// Package redaction is the one credential policy of everything Release
// Intelligence stores or shows from an environment (DESIGN.md §2.3): the
// files and renders the investigation bundle collects (marker "<redacted
// sha256:…>") and the render model (internal/render, "<sensitive
// sha256:…>"), plus the one schema-aware step that may read a withheld value
// back.
//
// The rule (redaction@v2; v1 was the collector's and the render model's separate rules):
//
//	(a) Kubernetes Secret data/stringData values are withheld (a
//	    document-level rule the caller applies);
//	(b) a string holding private-key material (a PEM private-key block) is
//	    withheld under any key;
//	(c) a scalar whose own key names a credential (SensitiveKey) is withheld
//	    unless it is a boolean or null — except an object-reference key
//	    (ReferenceKey: name, namespace, key, …Name, …Namespace): its value
//	    names an object or a data key, not secret material (secretName,
//	    privateKeySecretRef.name, tokenSecretRef.key);
//	(d) below a credential-named key holding a mapping or a list
//	    (privateKey:, credentials:, imagePullSecrets:) a scalar is kept only
//	    when it is a boolean, null or number, or sits at a reference key;
//	    every other string stays withheld;
//	(e) the value of an environment variable whose name names a credential
//	    (- name: DB_PASSWORD, value: …) is withheld.
//
// A CustomResourceDefinition is a schema, not settings: its property names
// (spec.versions[].schema.openAPIV3Schema.properties.privateKey) name
// fields, so only (b) applies to it (CRDDocument).
//
// Read-back (Schemas.Recover): a marker below a credential-named mapping or
// list, at a field whose own key does not name a credential, is replaced by
// the one string enum value a CustomResourceDefinition of the bundle
// declares for that exact group/version/kind and field path whose marker
// (either form) equals it. Only a constant the schema itself lists can ever
// be shown: free text and anything without a declared enum stay withheld.
package redaction

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// SensitiveKey matches keys that name a credential: the union of the
// collector's and the render model's former expressions, plus accessKey.
var SensitiveKey = regexp.MustCompile(`(?i)password|passwd|passphrase|secret|token|api[-_]?key|private[-_]?key|access[-_]?key|credential|bearer`)

// ReferenceKey reports whether a key holds an object reference's name,
// namespace or data-key name rather than secret material.
func ReferenceKey(key string) bool {
	switch key {
	case "name", "namespace", "key":
		return true
	}
	return strings.HasSuffix(key, "Name") || strings.HasSuffix(key, "Namespace")
}

// Credential reports whether a key names a credential and is not a
// reference key: its scalar value is withheld (rule c).
func Credential(key string) bool {
	return key != "" && SensitiveKey.MatchString(key) && !ReferenceKey(key)
}

// CredentialContainer reports whether a key holding a mapping or list
// makes everything below it subject to rule (d).
func CredentialContainer(key string) bool { return SensitiveKey.MatchString(key) }

// CRDDocument reports whether a document (apiVersion, kind) is a
// CustomResourceDefinition: only rule (b) applies inside it.
func CRDDocument(apiVersion, kind string) bool {
	return kind == "CustomResourceDefinition" && strings.HasPrefix(apiVersion, "apiextensions.k8s.io/")
}

var privateKeyMark = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)

// MarkerPrefix starts every collector marker.
const MarkerPrefix = "<redacted sha256:"

// RenderMarkerPrefix starts every render-model marker.
const RenderMarkerPrefix = "<sensitive sha256:"

// Marker is the collector's replacement of a withheld value: the first 12
// hex digits of the SHA-256 of its text.
func Marker(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return MarkerPrefix + hex.EncodeToString(sum[:])[:12] + ">"
}

// RenderMarker is the render model's replacement of a withheld value: the
// first 16 hex digits of the SHA-256 of its JSON encoding.
func RenderMarker(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return RenderMarkerPrefix + hex.EncodeToString(sum[:])[:16] + ">"
}

// IsMarker reports whether s is a marker of either form.
func IsMarker(s string) bool {
	return strings.HasPrefix(s, MarkerPrefix) || strings.HasPrefix(s, RenderMarkerPrefix)
}

// HoldsPrivateKey reports whether s contains a PEM private-key block.
func HoldsPrivateKey(s string) bool { return privateKeyMark.MatchString(s) }

// ScalarKind is what a scalar is.
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

// KindOf classifies a decoded value (YAML or JSON into any).
func KindOf(v any) ScalarKind {
	switch v.(type) {
	case bool:
		return Bool
	case nil:
		return Null
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return Number
	}
	return String
}

// Keep reports whether a scalar may be stored as is (rules b–d). key is the
// nearest mapping key above it (its own key, or the key of the list it is
// an item of); under reports that a key further up names a credential and
// holds a mapping or list.
func Keep(key string, kind ScalarKind, value string, under bool) bool {
	switch {
	case kind == String && HoldsPrivateKey(value):
		return false
	case Credential(key):
		return kind == Bool || kind == Null
	case under:
		return kind != String || ReferenceKey(key)
	}
	return true
}

// CredentialEnvVar reports whether an environment variable's name names a
// credential, so its value is withheld (rule e).
func CredentialEnvVar(name string) bool { return SensitiveKey.MatchString(name) }

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
// marker (either form) put below a credential-named mapping or list at a
// field whose own key names no credential, when exactly one enum value the schemas
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
				_, isList := c.([]any)
				walk(c, join(path, k), k, under || ((isMap || isList) && CredentialContainer(k)), func(n any) { t[k] = n }, depth+1)
			}
		case []any:
			for i, c := range t {
				i := i
				walk(c, path+"[]", key, under, func(n any) { t[i] = n }, depth+1)
			}
		case string:
			if !under || !IsMarker(t) || Credential(key) {
				return
			}
			match := ""
			n := 0
			for _, c := range uniq(enums[path]) {
				if Marker(c) == t || RenderMarker(c) == t {
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
