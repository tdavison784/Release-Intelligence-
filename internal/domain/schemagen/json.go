package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// kv is one key/value pair of an ordered JSON object.
type kv struct {
	K string
	V any
}

// obj is a JSON object that keeps its keys in insertion order, so the
// generated files read like hand-written schemas (type, required, properties,
// ...) and are byte-for-byte reproducible.
type obj []kv

// o builds an obj from alternating key, value arguments.
func o(pairs ...any) obj {
	if len(pairs)%2 != 0 {
		panic("schemagen: o() needs key/value pairs")
	}
	out := make(obj, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, kv{pairs[i].(string), pairs[i+1]})
	}
	return out
}

// get returns the value stored under k.
func (x obj) get(k string) (any, bool) {
	for _, p := range x {
		if p.K == k {
			return p.V, true
		}
	}
	return nil, false
}

// with returns a copy of x with k set: an existing key keeps its position,
// a new key is appended.
func (x obj) with(k string, v any) obj {
	out := make(obj, len(x), len(x)+1)
	copy(out, x)
	for i := range out {
		if out[i].K == k {
			out[i].V = v
			return out
		}
	}
	return append(out, kv{k, v})
}

// merge applies every pair of patch onto x (see with).
func (x obj) merge(patch obj) obj {
	for _, p := range patch {
		x = x.with(p.K, p.V)
	}
	return x
}

// scalar encodes a JSON scalar without HTML escaping (descriptions contain
// characters such as '>' and '<' that must stay readable).
func scalar(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(fmt.Sprintf("schemagen: cannot encode %T: %v", v, err))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// maxInline is the widest a nested value may be to stay on one line.
const maxInline = 96

// inline renders v on a single line, or reports false when it cannot be
// rendered that way.
func inline(v any) (string, bool) {
	switch x := v.(type) {
	case obj:
		if len(x) == 0 {
			return "{}", true
		}
		parts := make([]string, len(x))
		for i, p := range x {
			s, ok := inline(p.V)
			if !ok {
				return "", false
			}
			parts[i] = scalar(p.K) + ": " + s
		}
		return "{" + strings.Join(parts, ", ") + "}", true
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			s, ok := inline(e)
			if !ok {
				return "", false
			}
			parts[i] = s
		}
		return "[" + strings.Join(parts, ", ") + "]", true
	case []string:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = scalar(e)
		}
		return "[" + strings.Join(parts, ", ") + "]", true
	default:
		return scalar(v), true
	}
}

// expandedKeys name objects whose members are themselves schemas; they are
// always laid out one member per line.
var expandedKeys = map[string]bool{"properties": true, "$defs": true}

// pretty renders v with two-space indentation. Short sub-schemas such as
// {"type": "string"} or {"$ref": "#/$defs/Evidence"} stay on one line.
func pretty(v any) []byte {
	var b strings.Builder
	write(&b, v, 0, "")
	b.WriteString("\n")
	return []byte(b.String())
}

func write(b *strings.Builder, v any, depth int, key string) {
	pad := strings.Repeat("  ", depth)
	if !expandedKeys[key] {
		if s, ok := inline(v); ok && len(pad)+len(s) <= maxInline {
			b.WriteString(s)
			return
		}
	}
	switch x := v.(type) {
	case obj:
		b.WriteString("{\n")
		for i, p := range x {
			b.WriteString(pad + "  " + scalar(p.K) + ": ")
			write(b, p.V, depth+1, p.K)
			if i < len(x)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(pad + "}")
	case []any:
		b.WriteString("[\n")
		for i, e := range x {
			b.WriteString(pad + "  ")
			write(b, e, depth+1, "")
			if i < len(x)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(pad + "]")
	case []string:
		arr := make([]any, len(x))
		for i, e := range x {
			arr[i] = e
		}
		write(b, arr, depth, key)
	default:
		b.WriteString(scalar(v))
	}
}
