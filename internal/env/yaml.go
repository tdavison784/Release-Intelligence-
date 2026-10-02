package env

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// parseDocs decodes a (possibly multi-document) YAML stream with a real
// stream decoder (yaml.Decoder), so a "---" separator inside a block scalar
// (literal | or folded >) — or inside any other scalar — no longer splits
// documents. Decoded nodes carry their absolute line numbers, so evidence
// locators keep pointing into the original file without offset arithmetic.
//
// Documents that are empty (only comments and blank lines) yield a nil node.
// A stream that stops parsing midway returns the documents decoded so far
// plus a non-nil error: the caller keeps the prefix and warns about the
// remainder instead of dropping everything. onDup, when non-nil, is called
// for every duplicated mapping key with the 1-based line of the repeated
// key; extraction is last-wins (see pairsOf).
func parseDocs(content []byte, onDup func(key string, line int)) ([]doc, error) {
	// Tolerate a UTF-8 BOM and CRLF line endings; the decoder handles both,
	// but the BOM would otherwise surface in the first key of the first
	// document of some inputs.
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	var out []doc
	dec := yaml.NewDecoder(bytes.NewReader(content))
	for {
		var root yaml.Node
		err := dec.Decode(&root)
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, fmt.Errorf("document %d: %w", len(out)+1, err)
		}
		if len(root.Content) == 0 {
			out = append(out, doc{startLine: root.Line})
			continue
		}
		node := resolveAlias(root.Content[0])
		if node == nil || (node.Kind == yaml.ScalarNode && node.Tag == "!!null") {
			out = append(out, doc{startLine: root.Line})
			continue
		}
		if onDup != nil {
			checkDuplicateKeys(node, onDup)
		}
		// node.Line is the 1-based line of the document's first content in
		// the file — the provenance every fact of the document builds on.
		out = append(out, doc{node: node, startLine: node.Line})
	}
}

// countDocs counts the documents of a stream without keeping them.
func countDocs(content []byte) int {
	dec := yaml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))))
	n := 0
	var node yaml.Node
	for {
		if err := dec.Decode(&node); err != nil {
			return n
		}
		n++
	}
}

// checkDuplicateKeys warns about mapping keys repeated at the same level
// ("a: 1\na: 2"). yaml.v3 decodes such mappings silently, keeping the later
// pairs; extraction below matches that by being last-wins (pairsOf), and
// the warning keeps the ambiguity visible.
func checkDuplicateKeys(n *yaml.Node, onDup func(key string, line int)) {
	n = resolveAlias(n)
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.MappingNode:
		seen := map[string]int{} // key → line of first occurrence
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], resolveAlias(n.Content[i+1])
			if first, dup := seen[k.Value]; dup {
				onDup(fmt.Sprintf("%q (first at L%d)", k.Value, first), k.Line)
			} else {
				seen[k.Value] = k.Line
			}
			checkDuplicateKeys(v, onDup)
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			checkDuplicateKeys(c, onDup)
		}
	}
}

// pairsOf returns the key/value pairs of a mapping in source order, with
// duplicated keys collapsed to their LAST occurrence (the value a YAML
// processor would keep).
func pairsOf(n *yaml.Node) [][2]*yaml.Node {
	n = resolveAlias(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	last := map[string]int{}
	var pairs [][2]*yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if j, dup := last[k.Value]; dup {
			pairs[j][1] = v
			continue
		}
		last[k.Value] = len(pairs)
		pairs = append(pairs, [2]*yaml.Node{k, v})
	}
	return pairs
}

func resolveAlias(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		return resolveAlias(n.Alias)
	}
	return n
}

// fieldOf returns the value node of a (possibly nested) mapping path. A
// duplicated key resolves to its last occurrence.
func fieldOf(n *yaml.Node, path ...string) *yaml.Node {
	cur := resolveAlias(n)
	for _, key := range path {
		if cur == nil || cur.Kind != yaml.MappingNode {
			return nil
		}
		next := (*yaml.Node)(nil)
		for _, kv := range pairsOf(cur) {
			if kv[0].Value == key {
				next = resolveAlias(kv[1])
			}
		}
		cur = next
	}
	return cur
}

// scalarOf returns the scalar string at a mapping path ("" when absent).
func scalarOf(n *yaml.Node, path ...string) string {
	v := fieldOf(n, path...)
	if v == nil || v.Kind != yaml.ScalarNode {
		return ""
	}
	return v.Value
}

// itemsOf returns the elements of a sequence node.
func itemsOf(n *yaml.Node) []*yaml.Node {
	n = resolveAlias(n)
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	out := make([]*yaml.Node, 0, len(n.Content))
	for _, c := range n.Content {
		if c := resolveAlias(c); c != nil {
			out = append(out, c)
		}
	}
	return out
}

// walkMappings visits every mapping pair of a document (recursing into
// sequences and mappings) in deterministic document order. Duplicated keys
// are visited once, at their last occurrence.
func walkMappings(n *yaml.Node, visit func(key string, val *yaml.Node, path string)) {
	var walk func(n *yaml.Node, prefix string)
	walk = func(n *yaml.Node, prefix string) {
		n = resolveAlias(n)
		if n == nil {
			return
		}
		switch n.Kind {
		case yaml.MappingNode:
			for _, kv := range pairsOf(n) {
				k, v := kv[0], resolveAlias(kv[1])
				path := joinPath(prefix, k.Value)
				visit(k.Value, v, path)
				walk(v, path)
			}
		case yaml.SequenceNode:
			for _, c := range n.Content {
				walk(c, prefix)
			}
		}
	}
	walk(n, "")
}

// walkFieldPaths visits every leaf (scalar or sequence) of a mapping tree
// with its values-style dotted path. Duplicated keys collapse to their last
// occurrence.
func walkFieldPaths(n *yaml.Node, prefix string, visit func(path string, val *yaml.Node)) {
	n = resolveAlias(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return
	}
	for _, kv := range pairsOf(n) {
		k, v := kv[0], resolveAlias(kv[1])
		path := joinPath(prefix, k.Value)
		if v != nil && v.Kind == yaml.MappingNode && len(v.Content) > 0 {
			walkFieldPaths(v, path, visit)
			continue
		}
		visit(path, v)
	}
}

var simpleKeyRe = regexp.MustCompile(`^[^.\s"'\[\]]+$`)

// joinPath joins a key into a dotted values-style path; keys that are not
// plain identifiers are written as ["key"], matching the syntax of
// domain.ValuesSnapshot entries.
func joinPath(prefix, key string) string {
	if simpleKeyRe.MatchString(key) {
		if prefix == "" {
			return key
		}
		return prefix + "." + key
	}
	q := fmt.Sprintf("%q", key)
	return prefix + "[" + q + "]"
}

// lastSegment returns the segment after the final "." or the whole
// bracket-quoted key.
func lastSegment(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[i+1:]
	}
	return path
}

// siblingPath replaces the last segment of a dotted path.
func siblingPath(path, key string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[:i+1] + key
	}
	return key
}

// SplitKeyPath splits a values-style dotted path into its segments,
// understanding the ["key"] escaping (a key may contain dots). It is the
// shared vocabulary of the impact join: environment paths and upstream
// snapshot paths use the same syntax.
func SplitKeyPath(path string) []string {
	var out []string
	var cur strings.Builder
	i := 0
	for i < len(path) {
		switch c := path[i]; {
		case c == '.':
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			i++
		case c == '[' && i+1 < len(path) && path[i+1] == '"':
			// quoted segment; find the closing quote (no escapes in YAML keys here)
			j := i + 2
			for j < len(path) && path[j] != '"' {
				j++
			}
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			if j < len(path) {
				out = append(out, path[i+2:j])
			}
			i = j + 1
			// skip a trailing ] and any separator
			for i < len(path) && (path[i] == ']' || path[i] == '.') {
				i++
			}
		default:
			cur.WriteByte(c)
			i++
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
