package catalog

import (
	"fmt"
	"strings"
)

// YAMLPathSegment is one step of a version.field YAML path: a mapping key,
// optionally followed by list selectors that identify the element by one of
// its fields.
type YAMLPathSegment struct {
	Key       string
	Selectors []YAMLPathSelector
}

// YAMLPathSelector picks the first list element whose field equals value.
type YAMLPathSelector struct {
	Field, Value string
}

// SplitYAMLPath parses a YAML path such as
//
//	appVersion                                     Chart.yaml appVersion
//	dependencies[name=kube-state-metrics].version  a Helm dependency pin
//	prometheus.prometheusSpec.image.tag            a values.yaml default
//
// into segments and validates its shape: dotted mapping keys, each optionally
// followed by [field=value] selectors. The path is walked against a document
// by normalize.ReadYAMLPath.
func SplitYAMLPath(path string) ([]YAMLPathSegment, error) {
	bad := func(why string) error {
		return fmt.Errorf("yaml path %q: %s", path, why)
	}
	if strings.TrimSpace(path) == "" {
		return nil, bad("empty")
	}
	if strings.TrimSpace(path) != path {
		return nil, bad("surrounding whitespace")
	}
	var out []YAMLPathSegment
	for i, raw := range strings.Split(path, ".") {
		if raw == "" {
			return nil, bad("empty segment")
		}
		key := raw
		var sels []YAMLPathSelector
		for strings.Contains(key, "[") {
			j := strings.IndexByte(key, '[')
			k, bracket := key[:j], key[j:]
			e := strings.IndexByte(bracket, ']')
			if e < 0 {
				return nil, bad(fmt.Sprintf("unterminated [ in segment %d", i))
			}
			inner := bracket[1:e]
			f, val, ok := strings.Cut(inner, "=")
			if !ok || strings.TrimSpace(f) == "" || val == "" {
				return nil, bad(fmt.Sprintf("selector %q must be [field=value] in segment %d", inner, i))
			}
			rest := bracket[e+1:]
			if rest != "" && rest[0] != '[' {
				return nil, bad(fmt.Sprintf("misplaced bracket in segment %d (selectors must follow the key)", i))
			}
			sels = append(sels, YAMLPathSelector{Field: strings.TrimSpace(f), Value: val})
			key = k + rest
		}
		if key == "" {
			return nil, bad(fmt.Sprintf("empty key in segment %d", i))
		}
		if strings.ContainsAny(key, "[]") || strings.TrimSpace(key) != key {
			return nil, bad(fmt.Sprintf("invalid key %q in segment %d", key, i))
		}
		out = append(out, YAMLPathSegment{Key: key, Selectors: sels})
	}
	return out, nil
}
