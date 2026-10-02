package normalize

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
)

// ReadYAMLPath reads one scalar value out of a YAML document.
//
// The path addresses nested mappings with dotted keys and elements of a
// sequence with bracket selectors naming the identifying field (see
// catalog.SplitYAMLPath for the syntax):
//
//	appVersion                                     Chart.yaml appVersion
//	dependencies[name=kube-state-metrics].version  a Helm dependency pin
//	prometheus.prometheusSpec.image.tag            a values.yaml default
//
// The addressed node must be a scalar; its value is returned verbatim
// together with its 1-based line. An error wrapping ErrNoMatch is returned
// when the path addresses nothing (missing key, no such list element, or a
// non-scalar node); other errors mean the document or the path is unusable.
//
// This is the reader behind version.strategy "field": an artifact whose
// version is pinned per release inside a file of the release (a sub-component
// of an aggregating chart), rather than derived from the release version.
func ReadYAMLPath(content []byte, path string) (string, int, error) {
	segs, err := catalog.SplitYAMLPath(path)
	if err != nil {
		return "", 0, fmt.Errorf("normalize: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return "", 0, fmt.Errorf("normalize: parse document for path %q: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return "", 0, fmt.Errorf("normalize: yaml path %q: %w (empty document)", path, ErrNoMatch)
	}
	node := resolveAlias(doc.Content[0])
	for _, seg := range segs {
		node, err = stepYAMLPath(node, seg, path)
		if err != nil {
			return "", 0, err
		}
	}
	if node == nil || node.Kind != yaml.ScalarNode {
		return "", 0, fmt.Errorf("normalize: yaml path %q: %w (not a scalar)", path, ErrNoMatch)
	}
	return node.Value, node.Line, nil
}

// stepYAMLPath advances from node (a mapping) along one segment: first the
// mapping key, then any list selectors on the value
// ("dependencies[name=x]" = key "dependencies", then element with name=x).
func stepYAMLPath(node *yaml.Node, seg catalog.YAMLPathSegment, path string) (*yaml.Node, error) {
	cur := resolveAlias(node)
	if cur.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("normalize: yaml path %q: %w (%q is not a mapping)", path, ErrNoMatch, seg.Key)
	}
	next := mappingLookup(cur, seg.Key)
	if next == nil {
		return nil, fmt.Errorf("normalize: yaml path %q: %w (no key %q)", path, ErrNoMatch, seg.Key)
	}
	cur = next
	for _, sel := range seg.Selectors {
		cur = resolveAlias(cur)
		if cur.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("normalize: yaml path %q: %w (%q is not a list)", path, ErrNoMatch, seg.Key)
		}
		var found *yaml.Node
		for _, el := range cur.Content {
			el = resolveAlias(el)
			if el.Kind != yaml.MappingNode {
				continue
			}
			if f := mappingLookup(el, sel.Field); f != nil && f.Value == sel.Value {
				found = el
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("normalize: yaml path %q: %w (no element with %s=%q)", path, ErrNoMatch, sel.Field, sel.Value)
		}
		cur = found
	}
	return cur, nil
}

// mappingLookup returns the value node of key in a mapping (merge keys
// resolved), or nil.
func mappingLookup(m *yaml.Node, key string) *yaml.Node {
	for _, p := range mappingPairs(m) {
		if p.key.Value == key {
			return p.val
		}
	}
	return nil
}
