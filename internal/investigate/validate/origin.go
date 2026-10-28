package validate

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// origin names the environment file and line that produced the value at
// path of a rendered object (lane join5, D1): among the files the object's
// kustomize render consumed, the one of the latest layer (an overlay patch
// before its base) whose declaration of the object states path with that
// value. "" when no consumed file states it (a transformer, a generator, a
// chart template) or the path crosses a list: the render is then the only
// source.
func (v *envView) origin(e effObj, path, value string) string {
	ranks := v.layers[e.src.target]
	if !e.rendered || len(ranks) == 0 || strings.Contains(path, "[") {
		return ""
	}
	files := make([]string, 0, len(ranks))
	for f := range ranks {
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool {
		if ranks[files[i]] != ranks[files[j]] {
			return ranks[files[i]] < ranks[files[j]]
		}
		return files[i] < files[j]
	})
	for _, f := range files {
		if line := declares(v.raw[f], e, path, value); line > 0 {
			return fmt.Sprintf("%s:L%d", f, line)
		}
	}
	return ""
}

// declares returns the line at which a document of data that declares e's
// object (same group and kind, its name, or the rendered name with a
// kustomize prefix/suffix) states path with value; 0 when none does.
func declares(data []byte, e effObj, path, value string) int {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) || err != nil {
			return 0
		}
		if len(doc.Content) == 0 {
			continue
		}
		root := doc.Content[0]
		apiVersion, kind := scalarAt(root, "apiVersion"), scalarAt(root, "kind")
		group := ""
		if i := strings.LastIndexByte(apiVersion, '/'); i >= 0 {
			group = apiVersion[:i]
		}
		name := ""
		if md := child(root, "metadata"); md != nil {
			name = scalarAt(md, "name")
		}
		if kind != e.obj.ID.Kind || group != e.obj.ID.Group || name == "" || !strings.Contains(e.obj.ID.Name, name) {
			continue
		}
		n := root
		for _, s := range segments(path) {
			if n = child(n, s.name); n == nil {
				break
			}
		}
		if n == nil || n.Kind != yaml.ScalarNode {
			continue
		}
		var x any
		if n.Decode(&x) == nil && sameValue(encodeJSON(x), value) {
			return n.Line
		}
	}
}

// child is the value node of key in a mapping node (nil when absent).
func child(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func scalarAt(n *yaml.Node, key string) string {
	if c := child(n, key); c != nil && c.Kind == yaml.ScalarNode {
		return c.Value
	}
	return ""
}
