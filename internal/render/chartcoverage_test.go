package render

import (
	"context"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// normalizeFlatten flattens a values document to path → JSON (test helper).
func normalizeFlatten(raw []byte) (map[string]string, error) {
	flat, err := normalize.FlattenValues(raw)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(flat))
	for _, f := range flat {
		out[f.Path] = f.Value
	}
	return out, nil
}

// The coverage guard's path rule: a default covers a key it names exactly or
// lies UNDER; a default above the key (the chart's parent map exists, the
// customer's leaf does not) covers nothing.
func TestUncoveredKeys(t *testing.T) {
	defaults := map[string]string{
		"replicas":         "1",
		"legacy.feature":   `"on"`,
		"legacy.dead":      `"unused"`,
		"tls.list[0].name": `"x"`,
	}
	cases := []struct {
		key  string
		want bool // covered
	}{
		{"legacy.feature", true},          // exact
		{"legacy.dead", true},             // exact
		{"legacy", true},                  // defaults under it: nulling the section deletes them
		{"tls.list[0]", true},             // default under the list element
		{"tls", true},                     // defaults under it
		{"replicas.deeper", false},        // default above the key — the chart's scalar is not the leaf
		{"replicasX", false},              // shared prefix, no segment boundary
		{"cni.ambient.dnsCapture", false}, // another chart's key entirely
	}
	for _, tc := range cases {
		if got := len(uncoveredKeys(defaults, []string{tc.key})) == 0; got != tc.want {
			t.Errorf("uncoveredKeys(%q) covered = %v, want %v", tc.key, got, tc.want)
		}
	}
	// mixed: only the uncovered ones are named
	if unc := uncoveredKeys(defaults, []string{"legacy.feature", "cni.ambient.dnsCapture"}); len(unc) != 1 || unc[0] != "cni.ambient.dnsCapture" {
		t.Errorf("mixed keys must name only the uncovered: %v", unc)
	}
}

// Istio's charts wrap every real key under one top-level `defaults` map; the
// lift unwraps exactly that shape and nothing else.
func TestLiftDefaultsWrap(t *testing.T) {
	cases := []struct {
		name, in string
		want     string // expected flattened top key, "" = unchanged
	}{
		{"istio wrap", "defaults:\n  pilot:\n    replicas: 1\n", "pilot.replicas"},
		{"two top keys", "defaults:\n  a: 1\nother: 2\n", ""},
		{"defaults scalar", "defaults: gone\n", ""},
		{"no wrap", "pilot:\n  replicas: 1\n", ""},
	}
	for _, tc := range cases {
		out, ok := liftDefaultsWrap([]byte(tc.in))
		if tc.want == "" {
			if ok {
				t.Errorf("%s: must not lift", tc.name)
			}
			continue
		}
		if !ok {
			t.Fatalf("%s: must lift", tc.name)
		}
		d, why := chartDefaults(nil, nil, "", "")
		if why != "no chart resolver configured" || d != nil {
			t.Fatalf("resolver guard: %v %q", d, why)
		}
		flat, err := normalizeFlatten(out)
		if err != nil {
			t.Fatalf("%s: flatten: %v", tc.name, err)
		}
		if _, hit := flat[tc.want]; !hit {
			t.Errorf("%s: lifted keys %v lack %s", tc.name, flat, tc.want)
		}
	}
}
func TestChartDefaults(t *testing.T) {
	e := testEngine(t)
	d, why := chartDefaults(context.Background(), e, "rej", "1.0.0")
	if why != "" {
		t.Fatalf("chartDefaults: %s", why)
	}
	for _, k := range []string{"replicas", "legacy.feature", "legacy.dead"} {
		if _, ok := d[k]; !ok {
			t.Errorf("missing default %s in %v", k, d)
		}
	}
	if _, ok := d["cni.ambient.dnsCapture"]; ok {
		t.Error("the rej chart does not define cni keys")
	}
}
