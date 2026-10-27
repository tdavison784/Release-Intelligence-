package applicability

import (
	"strings"
	"testing"
)

func TestDefaultAt(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{
		"spec": map[string]any{"type": "object", "properties": map[string]any{
			"a":    map[string]any{"type": "boolean", "default": false},
			"b":    map[string]any{"type": "string"},
			"obj":  map[string]any{"type": "object", "default": map[string]any{}, "properties": map[string]any{"c": map[string]any{"type": "string"}}},
			"list": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"d": map[string]any{"type": "string", "default": "x"}}}},
			"free": map[string]any{"type": "object", "x-kubernetes-preserve-unknown-fields": true},
		}},
	}}
	for _, c := range []struct {
		path, val string
		has       bool
		why       string
	}{
		{"spec.a", "false", true, ""},
		{"spec.b", "", false, ""},
		{"spec.obj.c", "", false, "ancestor"},
		{"spec.list[].d", `"x"`, true, ""},
		{"spec.free.e", "", false, "preserves unknown fields"},
		{"spec.nope", "", false, "does not describe"},
	} {
		v, has, why := DefaultAt(schema, c.path)
		if v != c.val || has != c.has || (c.why == "") != (why == "") || !strings.Contains(why, c.why) {
			t.Fatalf("%s: %q %v %q", c.path, v, has, why)
		}
	}
}

