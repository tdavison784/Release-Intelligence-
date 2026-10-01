package normalize

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func parseChartMetadata(chartYAML []byte) (*ChartMetadata, error) {
	// ChartMetadata only carries json tags, so decode into a yaml-tagged twin
	var raw struct {
		Name        string `yaml:"name"`
		Version     string `yaml:"version"`
		AppVersion  string `yaml:"appVersion"`
		KubeVersion string `yaml:"kubeVersion"`
	}
	if err := yaml.Unmarshal(chartYAML, &raw); err != nil {
		return nil, fmt.Errorf("normalize: parse Chart.yaml: %w", err)
	}
	if raw.Name == "" && raw.Version == "" {
		return nil, errors.New("normalize: Chart.yaml has neither name nor version")
	}
	return &ChartMetadata{Name: raw.Name, Version: raw.Version, AppVersion: raw.AppVersion, KubeVersion: raw.KubeVersion}, nil
}

// maxCommentLen bounds a values.yaml key comment.
const maxCommentLen = 300

// valuesSnapshot flattens a Helm values document.
//
// Paths: nested mapping keys are joined with "."; a key containing "." or
// whitespace (or quotes/brackets) is written as ["key"] without a leading dot,
// e.g. `ingress.annotations["nginx.ingress.kubernetes.io/rewrite-target"]`.
// Scalars and sequences are leaves holding their JSON encoding (mappings inside
// sequences are encoded with sorted keys); an empty mapping is the leaf "{}".
// Comments holds, per key (leaf or mapping), the comment block directly above
// it with "#", the helm-docs "-- " marker and "@param"/"@section"-style
// annotations stripped, joined into one line and bounded to 300 bytes.
func valuesSnapshot(chart, version string, valuesYAML []byte) (*domain.ValuesSnapshot, error) {
	snap := &domain.ValuesSnapshot{Chart: chart, Version: version, Entries: map[string]string{}}
	var doc yaml.Node
	if err := yaml.Unmarshal(valuesYAML, &doc); err != nil {
		return nil, fmt.Errorf("normalize: parse values.yaml: %w", err)
	}
	if len(doc.Content) == 0 {
		return snap, nil
	}
	root := resolveAlias(doc.Content[0])
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return snap, nil
	}
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("normalize: values.yaml root is not a mapping")
	}
	comments := map[string]string{}
	walkValues(root, "", snap.Entries, comments)
	if len(comments) > 0 {
		snap.Comments = comments
	}
	return snap, nil
}

func walkValues(m *yaml.Node, prefix string, entries, comments map[string]string) {
	for _, kv := range mappingPairs(m) {
		key := kv.key.Value
		path := joinValuePath(prefix, key)
		if c := cleanValueComment(kv.key.HeadComment); c != "" {
			comments[path] = c
		}
		v := resolveAlias(kv.val)
		if v.Kind == yaml.MappingNode {
			if len(mappingPairs(v)) == 0 {
				entries[path] = "{}"
				continue
			}
			walkValues(v, path, entries, comments)
			continue
		}
		entries[path] = encodeLeaf(v)
	}
}

type pair struct{ key, val *yaml.Node }

// mappingPairs returns the key/value pairs of a mapping in source order with
// YAML merge keys ("<<: *base") resolved; explicit keys win over merged ones.
func mappingPairs(m *yaml.Node) []pair {
	m = resolveAlias(m)
	var explicit, merged []pair
	seen := map[string]bool{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Tag == "!!merge" {
			vv := resolveAlias(v)
			switch vv.Kind {
			case yaml.MappingNode:
				merged = append(merged, mappingPairs(vv)...)
			case yaml.SequenceNode:
				for _, c := range vv.Content {
					if cc := resolveAlias(c); cc.Kind == yaml.MappingNode {
						merged = append(merged, mappingPairs(cc)...)
					}
				}
			}
			continue
		}
		explicit = append(explicit, pair{k, v})
		seen[k.Value] = true
	}
	for _, p := range merged {
		if !seen[p.key.Value] {
			seen[p.key.Value] = true
			explicit = append(explicit, p)
		}
	}
	return explicit
}

var simpleKeyRe = regexp.MustCompile(`^[^.\s"'\[\]]+$`)

func joinValuePath(prefix, key string) string {
	if simpleKeyRe.MatchString(key) {
		if prefix == "" {
			return key
		}
		return prefix + "." + key
	}
	q, _ := json.Marshal(key)
	return prefix + "[" + string(q) + "]"
}

// encodeLeaf JSON-encodes a scalar or sequence node.
func encodeLeaf(n *yaml.Node) string {
	v := nodeToValue(n)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		q, _ := json.Marshal(n.Value)
		return string(q)
	}
	return strings.TrimRight(buf.String(), "\n")
}

// nodeToValue converts a YAML node to a JSON-compatible Go value. Timestamps
// and binary scalars keep their source text.
func nodeToValue(n *yaml.Node) any {
	n = resolveAlias(n)
	if n == nil {
		return nil
	}
	switch n.Kind {
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!timestamp", "!!binary", "!!merge":
			return n.Value
		}
		var v any
		if err := n.Decode(&v); err != nil {
			return n.Value
		}
		switch x := v.(type) {
		case map[any]any, []any:
			return n.Value
		case float64:
			// NaN/Inf are not JSON; keep the source text
			if x != x || x > 1.7976931348623157e308 || x < -1.7976931348623157e308 {
				return n.Value
			}
		}
		return v
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			out = append(out, nodeToValue(c))
		}
		return out
	case yaml.MappingNode:
		out := map[string]any{}
		for _, p := range mappingPairs(n) {
			out[p.key.Value] = nodeToValue(p.val)
		}
		return out
	}
	return nil
}

var (
	commentLineRe  = regexp.MustCompile(`^\s*#+\s?`)
	helmDocsMarker = regexp.MustCompile(`^--(?:\s+|$)`)
	paramRe        = regexp.MustCompile(`^@param\s+\S+\s*(?:\[[^\]]*\]\s*)?`)
	annotationRe   = regexp.MustCompile(`^\+docs:|^@(?:section|default|notationType|skip|ignore|raw|extra|descriptionStart|descriptionEnd|type|example|enum|param-start|param-end)\b`)
)

// cleanValueComment turns a yaml head comment block into one descriptive line.
func cleanValueComment(c string) string {
	if strings.TrimSpace(c) == "" {
		return ""
	}
	var parts []string
	for _, line := range strings.Split(c, "\n") {
		t := commentLineRe.ReplaceAllString(line, "")
		t = strings.TrimSpace(t)
		t = helmDocsMarker.ReplaceAllString(t, "")
		switch {
		case t == "":
			continue
		case annotationRe.MatchString(t):
			continue
		case strings.HasPrefix(t, "@param"):
			t = strings.TrimSpace(paramRe.ReplaceAllString(t, ""))
		}
		if t != "" {
			parts = append(parts, t)
		}
	}
	return truncateText(collapseSpace(strings.Join(parts, " ")), maxCommentLen)
}
