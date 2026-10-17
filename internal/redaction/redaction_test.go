package redaction

import (
	"reflect"
	"testing"
)

const pem = "-----BEGIN RSA PRIVATE KEY-----\nMIIEow\n-----END RSA PRIVATE KEY-----\n"

func TestKeep(t *testing.T) {
	for _, c := range []struct {
		key   string
		kind  ScalarKind
		value string
		under bool
		want  bool
	}{
		{"replicas", Number, "2", false, true},
		{"host", String, "db.internal", false, true},
		{"password", String, "hunter2", false, false},
		{"apiToken", Number, "123456", false, false}, // own key: only bool/null, as before
		{"createSecret", Bool, "true", false, true},
		{"password", Null, "", false, true},
		{"ca.key", String, pem, false, false}, // a private key under any key
		{"note", String, "see " + pem, false, false},
		{"algorithm", String, "ECDSA", true, false}, // below a credential map: strings withheld
		{"size", Number, "256", true, true},
		{"enabled", Bool, "false", true, true},
		{"unset", Null, "", true, true},
		{"password", String, "hunter2", true, false},
		{"password", Number, "1234", true, false},
		{"", String, "item", true, false}, // a sequence item below a credential map
		// object-reference keys (rule c/d exception)
		{"secretName", String, "tls", false, true},
		{"name", String, "ca-key", true, true},
		{"key", String, "tls.key", true, true},
		{"namespace", String, "apps", true, true},
		{"issuerNamespace", String, "apps", true, true},
		{"secretNames", String, "a", false, false}, // not a reference key
		// the union of credential names
		{"secretAccessKey", String, "AKIA", false, false},
		{"accessKey", String, "AKID", false, false},
		{"clientSecret", String, "x", false, false},
		{"passphrase", String, "x", false, false},
		{"bearer", String, "x", false, false},
	} {
		if got := Keep(c.key, c.kind, c.value, c.under); got != c.want {
			t.Errorf("Keep(%q, %v, %q, under=%v) = %v, want %v", c.key, c.kind, c.value, c.under, got, c.want)
		}
	}
}

func TestKindOfTag(t *testing.T) {
	for tag, want := range map[string]ScalarKind{"!!bool": Bool, "!!null": Null, "!!int": Number, "!!float": Number, "!!str": String, "!!timestamp": String, "!!binary": String} {
		if got := KindOfTag(tag); got != want {
			t.Errorf("KindOfTag(%q) = %v, want %v", tag, got, want)
		}
	}
}

func TestMarker(t *testing.T) {
	m := Marker("Never")
	if m != Marker("Never") || m == Marker("Always") || !IsMarker(m) || len(m) != len(MarkerPrefix)+12+1 {
		t.Fatalf("bad marker %q", m)
	}
}

// gadgetCRD is a synthetic CRD: spec.credentials (credential-named, a map) has an
// enum field, a free-text field and a nested credential; spec.tokens is a
// credential-named list whose items are enum-constrained.
func gadgetCRD() map[string]any {
	str := func(enum ...any) map[string]any {
		m := map[string]any{"type": "string"}
		if len(enum) > 0 {
			m["enum"] = enum
		}
		return m
	}
	return map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
		"metadata": map[string]any{"name": "gadgets.example.test"},
		"spec": map[string]any{
			"group": "example.test", "names": map[string]any{"kind": "Gadget"},
			"versions": []any{map[string]any{"name": "v1", "schema": map[string]any{"openAPIV3Schema": map[string]any{
				"type": "object",
				"properties": map[string]any{"spec": map[string]any{"type": "object", "properties": map[string]any{
					"mode": str("Fast", "Slow"),
					"credentials": map[string]any{"type": "object", "properties": map[string]any{
						"rotation": str("Never", "Always"),
						"comment":  str(),
						"password": str("hunter2", "letmein"), // an enum on a credential-named field: never read back
						"usages":   map[string]any{"type": "array", "items": str("sign", "encrypt")},
						"pick":     str("Fast", "Slow", Marker("Slow")), // a marker in an enum is no candidate
					}},
					"tokens": map[string]any{"type": "array", "items": str("alpha", "beta")},
				}}},
			}}}},
		},
	}
}

func gadget() map[string]any {
	return map[string]any{
		"apiVersion": "example.test/v1", "kind": "Gadget",
		"metadata": map[string]any{"name": "g"},
		"spec": map[string]any{
			"mode": Marker("Fast"), // not below a credential map: no collector withheld it there
			"credentials": map[string]any{
				"rotation": Marker("Never"),
				"comment":  Marker("rotate by hand, ask ops"),
				"password": Marker("hunter2"),
				"usages":   []any{Marker("sign"), Marker("custom")},
				"pick":     Marker("Slow"),
				"size":     256,
			},
			"tokens": []any{Marker("alpha")},
		},
	}
}

func TestRecoverReadsBackOnlyDeclaredEnumValues(t *testing.T) {
	ix := NewSchemas()
	ix.AddCRD(gadgetCRD())
	body := gadget()
	got := ix.Recover("example.test", "v1", "Gadget", body)
	want := []Recovered{
		{Path: "spec.credentials.pick", Marker: Marker("Slow"), Value: "Slow"},
		{Path: "spec.credentials.rotation", Marker: Marker("Never"), Value: "Never"},
		{Path: "spec.credentials.usages[]", Marker: Marker("sign"), Value: "sign"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recovered %#v\nwant %#v", got, want)
	}
	spec := body["spec"].(map[string]any)
	vault := spec["credentials"].(map[string]any)
	if vault["rotation"] != "Never" || vault["usages"].([]any)[0] != "sign" {
		t.Errorf("enum values not read back in place: %#v", vault)
	}
	// everything not provably a schema constant stays withheld
	for what, v := range map[string]any{
		"free text":              vault["comment"],
		"credential-named field": vault["password"],
		"value outside the enum": vault["usages"].([]any)[1],
		"credential-named list":  spec["tokens"].([]any)[0],
		"not below a credential": spec["mode"],
	} {
		if s, _ := v.(string); !IsMarker(s) {
			t.Errorf("%s was read back: %v", what, v)
		}
	}
}

func TestRecoverNeedsTheExactSchema(t *testing.T) {
	ix := NewSchemas()
	ix.AddCRD(gadgetCRD())
	for _, c := range []struct{ group, version, kind string }{
		{"example.test", "v2", "Gadget"}, // a version the CRD does not define
		{"other.test", "v1", "Gadget"},   // another group
		{"example.test", "v1", "Gizmo"},  // another kind
		{"", "v1", "Secret"},             // never a Secret
	} {
		if got := ix.Recover(c.group, c.version, c.kind, gadget()); len(got) != 0 {
			t.Errorf("%v: recovered %v without its schema", c, got)
		}
	}
	if got := NewSchemas().Recover("example.test", "v1", "Gadget", gadget()); len(got) != 0 {
		t.Errorf("no CRD: recovered %v", got)
	}
	var nilIx *Schemas
	if got := nilIx.Recover("example.test", "v1", "Gadget", gadget()); got != nil {
		t.Errorf("nil index: recovered %v", got)
	}
}

func TestRecoverDuplicateCRDsReadBackOnce(t *testing.T) {
	ix := NewSchemas()
	ix.AddCRD(gadgetCRD())
	ix.AddCRD(gadgetCRD()) // the same type from a second source (from/to CRDs)
	if got := ix.Recover("example.test", "v1", "Gadget", gadget()); len(got) != 3 {
		t.Fatalf("duplicate CRDs changed the read-back: %v", got)
	}
}

func TestAddCRDIgnoresOtherObjects(t *testing.T) {
	ix := NewSchemas()
	ix.AddCRD(map[string]any{"kind": "ConfigMap"})
	ix.AddCRD(map[string]any{"kind": "CustomResourceDefinition", "spec": map[string]any{"group": "g"}})
	if ix.Len() != 0 {
		t.Fatalf("indexed %d types", ix.Len())
	}
}

// The render model's markers are read back the same way.
func TestRecoverRenderMarkers(t *testing.T) {
	ix := NewSchemas()
	ix.AddCRD(gadgetCRD())
	body := gadget()
	creds := body["spec"].(map[string]any)["credentials"].(map[string]any)
	creds["rotation"], creds["comment"] = RenderMarker("Never"), RenderMarker("rotate by hand, ask ops")
	got := ix.Recover("example.test", "v1", "Gadget", body)
	if creds["rotation"] != "Never" || !IsMarker(creds["comment"].(string)) {
		t.Fatalf("render markers: %v %#v", got, creds)
	}
}
