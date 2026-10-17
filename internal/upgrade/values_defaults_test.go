package upgrade

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// FromDefaults reads back exactly the source defaults diffValues wrote, for
// every shape of values change it emits.
func TestFromDefaultsRoundTrip(t *testing.T) {
	from := bare("v1.0.0").values("chart", "demo", "https://x/v1.0.0/values.yaml",
		"a.x", `"/var/run/token"`, "a.y", "{}", // section removed
		"g.only", "null", // single-key section removed
		"b.keep", "1", "b.gone", `"path. Default in x: y"`, // per-key removal, a value holding the separators
		"b.sub.one", `["x","y"]`, "b.sub.two", `""`, // sub-section removed
		"d", `"a → b"`, // default changed, arrow inside the old value
		"e", "true",
	)
	from.r.Snapshots[0].Values.Comments = map[string]string{"b.gone": "documented: with a colon"}
	to := bare("v1.1.0").values("chart", "demo", "https://x/v1.1.0/values.yaml",
		"b.keep", "1", "d", `"c"`,
	)
	e := mustBuild(t, simpleInput(from, to))
	for title, want := range map[string]map[string]string{
		"Helm values section `a.*` removed (2 keys)":           {"a.x": `"/var/run/token"`, "a.y": "{}"},
		"Helm values section `g.*` removed (1 key)":            {"g.only": "null"},
		"Helm value `b.gone` removed":                          {"b.gone": `"path. Default in x: y"`},
		"Helm values `b.sub.*` removed (2 keys)":               {"b.sub.one": `["x","y"]`, "b.sub.two": `""`},
		"Helm value `e` removed":                               {"e": "true"},
		"Default of Helm value `d` changed: \"a → b\" → \"c\"": {"d": `"a → b"`},
	} {
		c := findTitle(e, title)
		if c == nil {
			t.Errorf("missing change %q", title)
			continue
		}
		if got := FromDefaults(*c); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: FromDefaults = %v, want %v", title, got, want)
		}
	}
	if got := FromDefaults(domain.Change{Provenance: computed(RuleValuesAdded), Subjects: []string{"x"}}); got != nil {
		t.Errorf("added keys have no source default, got %v", got)
	}
}

// A section enumeration truncated at maxDetailLines leaves the remaining
// defaults unknown (absent), never empty.
func TestFromDefaultsTruncatedSection(t *testing.T) {
	var kv []string
	for i := 0; i < maxDetailLines+5; i++ {
		kv = append(kv, fmt.Sprintf("s.k%03d", i), `"v"`)
	}
	from := bare("v1.0.0").values("chart", "demo", "https://x/v1.0.0/values.yaml", kv...)
	to := bare("v1.1.0").values("chart", "demo", "https://x/v1.1.0/values.yaml", "t", "1")
	e := mustBuild(t, simpleInput(from, to))
	cs := findChanges(e, RuleValuesSectionRemoved)
	if len(cs) != 1 {
		t.Fatalf("section changes = %d", len(cs))
	}
	got := FromDefaults(cs[0])
	if len(got) != maxDetailLines || got["s.k000"] != `"v"` {
		t.Errorf("recovered %d defaults (want %d): %v", len(got), maxDetailLines, got["s.k000"])
	}
	if _, ok := got[fmt.Sprintf("s.k%03d", maxDetailLines+1)]; ok {
		t.Error("a key past the enumeration bound must stay unknown")
	}
}

func TestEmptyDefault(t *testing.T) {
	for v, want := range map[string]bool{"null": true, `""`: true, "{}": true, "[]": true, "false": false, "0": false, `"x"`: false, `{"a":1}`: false} {
		if EmptyDefault(v) != want {
			t.Errorf("EmptyDefault(%s) = %v", v, !want)
		}
	}
}
