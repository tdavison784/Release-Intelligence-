// Package redaction is the credential policy of collected evidence (DESIGN.md
// §2.3), shared by everything the bundle stores, and its one schema-aware
// counterpart that may read a withheld value back.
//
// Collection (before hashing, no schema at hand):
//
//   - Kubernetes Secret data/stringData values are withheld (the caller
//     applies this; it is a document-level rule);
//   - a scalar whose own key names a credential is withheld unless it is a
//     boolean or null (`createSecret: true` stays readable);
//   - any scalar holding private-key material (a PEM private-key block) is
//     withheld, whatever its key;
//   - below a credential-named key whose value is a mapping (`privateKey:`,
//     `credentials:`), the mapping is walked: a scalar is kept only when it
//     is provably not a credential — a boolean, null or number. Every string
//     below it stays withheld.
//
// A withheld value is replaced by Marker(value): "<redacted sha256:…>", the
// first 12 hex digits of the SHA-256 of its text.
//
// Read-back (Schemas.Recover): a marker below a credential-named mapping, at
// a field whose own key does not name a credential, is replaced by the one
// string enum value a CustomResourceDefinition of the bundle declares for
// that exact group/version/kind and field path whose marker equals it.
// Only a constant the schema itself lists can ever be shown, so nothing the
// collector withheld that is not provably a schema constant is re-exposed:
// free text, names and anything without a declared enum stay withheld.
package redaction

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

// SensitiveKey matches mapping keys that name a credential.
var SensitiveKey = regexp.MustCompile(`(?i)password|passwd|secret|token|apikey|api_key|privatekey|private_key|credentials?`)

var privateKeyMark = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)

// MarkerPrefix starts every marker.
const MarkerPrefix = "<redacted sha256:"

// Marker is the stored replacement of a withheld value.
func Marker(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return MarkerPrefix + hex.EncodeToString(sum[:])[:12] + ">"
}

// IsMarker reports whether s is a marker.
func IsMarker(s string) bool { return strings.HasPrefix(s, MarkerPrefix) }

// HoldsPrivateKey reports whether s contains a PEM private-key block.
func HoldsPrivateKey(s string) bool { return privateKeyMark.MatchString(s) }

// ScalarKind is what a scalar is, as its YAML tag says.
type ScalarKind int

const (
	String ScalarKind = iota // strings, timestamps, binary: anything else
	Bool
	Null
	Number
)

// KindOfTag classifies a YAML short tag (yaml.Node.ShortTag()).
func KindOfTag(tag string) ScalarKind {
	switch tag {
	case "!!bool":
		return Bool
	case "!!null":
		return Null
	case "!!int", "!!float":
		return Number
	}
	return String
}

// Keep reports whether a scalar may be stored as is. key is the nearest
// mapping key above it (its own key, or the key of the sequence it is an
// item of); underCredential reports that a key further up names a
// credential and holds a mapping.
func Keep(key string, kind ScalarKind, value string, underCredential bool) bool {
	switch {
	case kind == String && HoldsPrivateKey(value):
		return false
	case key != "" && SensitiveKey.MatchString(key):
		return kind == Bool || kind == Null
	case underCredential:
		return kind != String
	}
	return true
}

// Schemas indexes the string enums CustomResourceDefinitions declare: by
// "group/version/kind", by field path ("spec.a.b", "[]" for a sequence
// item, as CRD SchemaPaths), the declared values.
type Schemas struct {
	enums map[string]map[string][]string
}

// NewSchemas returns an empty index.
func NewSchemas() *Schemas { return &Schemas{enums: map[string]map[string][]string{}} }

// Len is the number of group/version/kinds indexed.
func (s *Schemas) Len() int { return len(s.enums) }

// AddCRD indexes one apiextensions.k8s.io/v1 CustomResourceDefinition (its
// parsed body). Anything else is ignored.
func (s *Schemas) AddCRD(body map[string]any) {
	if body["kind"] != "CustomResourceDefinition" {
		return
	}
	spec, _ := body["spec"].(map[string]any)
	group, _ := spec["group"].(string)
	names, _ := spec["names"].(map[string]any)
	kind, _ := names["kind"].(string)
	if group == "" || kind == "" {
		return
	}
	versions, _ := spec["versions"].([]any)
	for _, v := range versions {
		vm, _ := v.(map[string]any)
		name, _ := vm["name"].(string)
		schema, _ := vm["schema"].(map[string]any)
		root, _ := schema["openAPIV3Schema"].(map[string]any)
		if name == "" || root == nil {
			continue
		}
		gvk := group + "/" + name + "/" + kind
		if s.enums[gvk] == nil {
			s.enums[gvk] = map[string][]string{}
		}
		s.walk(s.enums[gvk], root, "", 0)
	}
}

func (s *Schemas) walk(into map[string][]string, node map[string]any, path string, depth int) {
	if node == nil || depth > 64 {
		return
	}
	if enum, ok := node["enum"].([]any); ok && path != "" {
		for _, e := range enum {
			if str, ok := e.(string); ok && !IsMarker(str) {
				into[path] = append(into[path], str)
			}
		}
	}
	if props, ok := node["properties"].(map[string]any); ok {
		for k, p := range props {
			pm, _ := p.(map[string]any)
			s.walk(into, pm, join(path, k), depth+1)
		}
	}
	if items, ok := node["items"].(map[string]any); ok {
		s.walk(into, items, path+"[]", depth+1)
	}
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// Recovered is one marker read back.
type Recovered struct {
	Path   string // field path, as Schemas indexes it
	Marker string
	Value  string
}

// Recover reads back, in body (a parsed object of group/version/kind), every
// marker that the collector put below a credential-named mapping at a field
// whose own key names no credential, when exactly one enum value the schemas
// declare for that exact field path has that marker. It changes body in
// place and returns what it read back, by path. A core Secret is never read
// back.
func (s *Schemas) Recover(group, version, kind string, body map[string]any) []Recovered {
	if s == nil || (group == "" && kind == "Secret") {
		return nil
	}
	enums := s.enums[group+"/"+version+"/"+kind]
	if len(enums) == 0 {
		return nil
	}
	var out []Recovered
	var walk func(v any, path, key string, under bool, set func(any), depth int)
	walk = func(v any, path, key string, under bool, set func(any), depth int) {
		if depth > 64 {
			return
		}
		switch t := v.(type) {
		case map[string]any:
			for k, c := range t {
				k, c := k, c
				_, isMap := c.(map[string]any)
				walk(c, join(path, k), k, under || (isMap && SensitiveKey.MatchString(k)), func(n any) { t[k] = n }, depth+1)
			}
		case []any:
			for i, c := range t {
				i := i
				walk(c, path+"[]", key, under, func(n any) { t[i] = n }, depth+1)
			}
		case string:
			if !under || !IsMarker(t) || (key != "" && SensitiveKey.MatchString(key)) {
				return
			}
			match := ""
			n := 0
			for _, c := range uniq(enums[path]) {
				if Marker(c) == t {
					match, n = c, n+1
				}
			}
			if n == 1 && !HoldsPrivateKey(match) {
				set(match)
				out = append(out, Recovered{Path: path, Marker: t, Value: match})
			}
		}
	}
	walk(body, "", "", false, func(any) {}, 0)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
