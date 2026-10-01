package env

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// splitDocs splits a (possibly multi-document) YAML stream into documents,
// tracking the 1-based line where each document starts so evidence locators
// point into the original file. Documents that are empty (only comments and
// blank lines) yield a nil node.
func splitDocs(content []byte) ([]doc, error) {
	lines := strings.Split(string(content), "\n")
	type chunk struct {
		lines     []string
		startLine int // 1-based
	}
	var chunks []chunk
	cur := chunk{startLine: 1}
	flush := func() {
		chunks = append(chunks, cur)
	}
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "---" || strings.HasPrefix(t, "--- ") || t == "..." {
			flush()
			cur = chunk{startLine: i + 2}
			continue
		}
		cur.lines = append(cur.lines, ln)
	}
	flush()

	var out []doc
	for _, c := range chunks {
		if len(bytes.TrimSpace([]byte(strings.Join(c.lines, "\n")))) == 0 {
			out = append(out, doc{file: "", node: nil, startLine: c.startLine})
			continue
		}
		var node yaml.Node
		if err := yaml.Unmarshal([]byte(strings.Join(c.lines, "\n")), &node); err != nil {
			return nil, fmt.Errorf("parse YAML document at line %d: %w", c.startLine, err)
		}
		root := resolveAlias(node.Content[0])
		if root == nil || (root.Kind == yaml.ScalarNode && root.Tag == "!!null") {
			root = nil
		}
		// Chunk-internal line numbers are 1-based within the chunk; shift
		// them so every node line refers to the original file.
		offsetLines(root, c.startLine-1)
		out = append(out, doc{node: root, startLine: c.startLine})
	}
	return out, nil
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

// offsetLines shifts the line of every node by off (see splitDocs).
func offsetLines(n *yaml.Node, off int) {
	if n == nil || off == 0 {
		return
	}
	n.Line += off
	for _, c := range n.Content {
		offsetLines(c, off)
	}
	if n.Alias != nil {
		offsetLines(n.Alias, off)
	}
}

// fieldOf returns the value node of a (possibly nested) mapping path.
func fieldOf(n *yaml.Node, path ...string) *yaml.Node {
	cur := resolveAlias(n)
	for _, key := range path {
		if cur == nil || cur.Kind != yaml.MappingNode {
			return nil
		}
		next := (*yaml.Node)(nil)
		for i := 0; i+1 < len(cur.Content); i += 2 {
			if cur.Content[i].Value == key {
				next = resolveAlias(cur.Content[i+1])
				break
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
// sequences and mappings) in deterministic document order.
func walkMappings(n *yaml.Node, visit func(key string, val *yaml.Node, path string)) {
	var walk func(n *yaml.Node, prefix string)
	walk = func(n *yaml.Node, prefix string) {
		n = resolveAlias(n)
		if n == nil {
			return
		}
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				k, v := n.Content[i], resolveAlias(n.Content[i+1])
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
// with its values-style dotted path.
func walkFieldPaths(n *yaml.Node, prefix string, visit func(path string, val *yaml.Node)) {
	n = resolveAlias(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], resolveAlias(n.Content[i+1])
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
