// Layered Helm values: one release configured by several values sources is
// the normal GitOps shape (an Argo CD helm.valueFiles list, Flux valuesFrom,
// helmfile layering, -f a.yaml -f b.yaml), not an ambiguity to warn about.
// This file implements the merge the loader applies across those sources, in
// application order: a later source overrides an earlier one per key,
// mappings deep-merge, and every other value replaces the subtree below its
// path (Helm's coalesce rules). The result keeps one effective key per path,
// with the evidence of the file and line that set it.

package env

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// valuesOrigin records which source set an effective values key.
type valuesOrigin struct {
	file string
	line int
}

// valuesMerge accumulates the layered values sources into the effective key
// set: one entry per path, from the source that last set it.
type valuesMerge struct {
	eff map[string]ValuesKey
	org map[string]valuesOrigin
}

func newValuesMerge() *valuesMerge {
	return &valuesMerge{eff: map[string]ValuesKey{}, org: map[string]valuesOrigin{}}
}

// apply merges one source: its flattened entries (sorted by path, as
// normalize.FlattenValues returns them) and the paths at which it holds
// mappings. A mapping at a path deep-merges with what earlier sources set
// below it (so an earlier leaf at that path is replaced by the mapping's own
// keys); any other entry takes the path itself and clears everything an
// earlier source set below it.
func (m *valuesMerge) apply(file string, entries []ValuesKey, containers []string) {
	for _, c := range containers {
		delete(m.eff, c)
		delete(m.org, c)
	}
	for _, k := range entries {
		for p := range m.eff {
			if strings.HasPrefix(p, k.Path+".") || strings.HasPrefix(p, k.Path+"[") {
				delete(m.eff, p)
				delete(m.org, p)
			}
		}
		m.eff[k.Path] = k
		m.org[k.Path] = valuesOrigin{file: file, line: k.Line}
	}
}

// paths returns the effective key paths in deterministic order.
func (m *valuesMerge) paths() []string {
	out := make([]string, 0, len(m.eff))
	for p := range m.eff {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// valuesContainersOf lists the mapping paths of a values document's bytes
// (the first document of the stream, as FlattenValues reads it).
func valuesContainersOf(b []byte) []string {
	docs, _ := parseDocs(b, nil)
	for _, d := range docs {
		if d.node != nil {
			return valuesContainerPaths(d.node)
		}
	}
	return nil
}

// valuesContainerPaths lists the paths at which a values mapping has nested
// keys (non-empty mappings), with the same key resolution the flattened
// leaves get (merge keys resolved, explicit keys winning; duplicated keys
// collapsed to their last occurrence), so container and leaf paths agree.
func valuesContainerPaths(root *yaml.Node) []string {
	var out []string
	var walk func(n *yaml.Node, prefix string)
	walk = func(n *yaml.Node, prefix string) {
		for _, kv := range valuesPairs(n) {
			v := resolveAlias(kv[1])
			if v == nil || v.Kind != yaml.MappingNode || len(v.Content) == 0 {
				continue // a leaf: scalars, sequences and empty mappings
			}
			path := joinValuesPath(prefix, kv[0].Value)
			out = append(out, path)
			walk(v, path)
		}
	}
	if root = resolveAlias(root); root != nil && root.Kind == yaml.MappingNode {
		walk(root, "")
	}
	sort.Strings(out)
	return out
}

// valuesPairs returns the key/value pairs of a values mapping with YAML merge
// keys ("<<: *base") resolved (explicit keys win over merged ones) and
// duplicated keys collapsed to their last occurrence — the resolution the
// flattened leaves get.
func valuesPairs(m *yaml.Node) [][2]*yaml.Node {
	m = resolveAlias(m)
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	var explicit, merged [][2]*yaml.Node
	seen := map[string]int{} // key → pair index in explicit
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Tag == "!!merge" {
			vv := resolveAlias(v)
			switch vv.Kind {
			case yaml.MappingNode:
				merged = append(merged, valuesPairs(vv)...)
			case yaml.SequenceNode:
				for _, c := range vv.Content {
					if cc := resolveAlias(c); cc != nil && cc.Kind == yaml.MappingNode {
						merged = append(merged, valuesPairs(cc)...)
					}
				}
			}
			continue
		}
		if j, dup := seen[k.Value]; dup {
			explicit[j][1] = v
			continue
		}
		seen[k.Value] = len(explicit)
		explicit = append(explicit, [2]*yaml.Node{k, v})
	}
	for _, p := range merged {
		if _, ok := seen[p[0].Value]; !ok {
			seen[p[0].Value] = len(explicit)
			explicit = append(explicit, p)
		}
	}
	return explicit
}

// joinValuesPath joins a key into a values-style dotted path with exactly the
// syntax of normalize's flattened entries (["key"] escaping), so merge paths
// and leaf paths always align.
func joinValuesPath(prefix, key string) string {
	if simpleKeyRe.MatchString(key) {
		if prefix == "" {
			return key
		}
		return prefix + "." + key
	}
	q, _ := json.Marshal(key)
	return prefix + "[" + string(q) + "]"
}

// applyValuesFile merges one values file (already read and registered by the
// loader) into the layered-values merge. Per-file parse problems are warnings
// on the values dimension, exactly as before layering was a merge.
func (l *loader) applyValuesFile(f string, b []byte) {
	flat, err := normalize.FlattenValues(b)
	if err != nil {
		l.warnf(DimValues, "%s was not parsed as a Helm values mapping: %v", f, err)
		return
	}
	// yaml.Unmarshal decodes only a stream's FIRST document and silently
	// drops the rest; a Helm values file must be a single document, so
	// make any extra ones an explicit, dimension-degrading warning.
	if n := countDocs(b); n > 1 {
		l.warnf(DimValues, "%s holds %d YAML documents; a Helm values file is one document — the rest was ignored", f, n)
	}
	entries := make([]ValuesKey, 0, len(flat))
	for _, kv := range flat {
		entries = append(entries, ValuesKey{
			Path: kv.Path, Value: kv.Value, Line: kv.Line,
			Evidence: []domain.EvidenceID{l.ev(f, fmt.Sprintf("$.%s (L%d)", kv.Path, kv.Line), kv.Path+": "+kv.Value)},
		})
	}
	l.values.apply(f, entries, valuesContainersOf(b))
}

// finalizeValues materializes the layered-values merge into the environment:
// one effective key per path — the source that last set it, with its
// provenance — under the global cap, and the image references the effective
// values pin (repository/tag siblings may come from different layers: Helm
// merges them the same way).
func (l *loader) finalizeValues() {
	paths := l.values.paths()
	capped := false
	for _, p := range paths {
		if !l.addValuesKey(l.values.eff[p]) {
			capped = true
			break
		}
	}
	if capped {
		l.warnf(DimValues, "more than %d values keys; the remaining effective keys were ignored", maxValuesKeys)
	}
	byPath := make(map[string]string, len(paths))
	for _, p := range paths {
		byPath[p] = l.values.eff[p].Value
	}
	for _, p := range paths {
		base := lastSegment(p)
		if base != "repository" && base != "hub" && base != "image" {
			continue
		}
		ref := strings.Trim(strings.Trim(l.values.eff[p].Value, `"`), `'`)
		if !strings.Contains(ref, "/") {
			continue // a bare name or a non-reference value
		}
		tag := ""
		if base != "image" {
			if sib, ok := byPath[siblingPath(p, "tag")]; ok {
				tag = strings.Trim(strings.Trim(sib, `"`), `'`)
			}
		}
		full := ref
		if tag != "" && base != "image" {
			full = ref + ":" + tag
		}
		if _, err := normalize.ParseImageRef(full); err != nil {
			continue
		}
		o := l.values.org[p]
		l.addImageAt(full, "values", o.file, o.line)
	}
}
