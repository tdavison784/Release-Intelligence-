package impact

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// removedWithDefault is a values:removed change whose Detail records the
// source default the way internal/upgrade writes it.
func (b *edgeBuilder) removedWithDefault(key, def string) domain.Change {
	b.change("values:removed", "Helm value `"+key+"` removed", key)
	c := &b.edge.Changes[len(b.edge.Changes)-1]
	c.Detail = "Values set at this key no longer take effect after the upgrade (or are rejected when the chart validates values against a schema). Default in v1.0.0: " + def + ".\nDocumentation in v1.0.0: the file holding the token"
	return *c
}

func valuesEnv(t *testing.T, content string) *env.Environment {
	t.Helper()
	return loadEnv(t, env.Inputs{ValuesFiles: []string{writeFile(t, t.TempDir(), "values.yaml", content)}})
}

// A removed key the environment leaves unset with a non-empty source default:
// the environment relied on the default — review-required, never not-affected,
// never action-required, citing the values files that do not set it.
func TestRemovedKeyWithDefaultLeftUnsetIsReview(t *testing.T) {
	eb := newEdge()
	eb.removedWithDefault("exporter.serviceMonitor.bearerTokenFile", `"/var/run/secrets/kubernetes.io/serviceaccount/token"`)
	r := buildReport(t, eb.edge, valuesEnv(t, "exporter:\n  enabled: true\n"))

	if fs := findingsByRule(r, RuleValuesUnset); len(fs) != 0 {
		t.Fatalf("relied-on default must not be values-unset: %+v", fs)
	}
	fs := findingsByRule(r, RuleValuesRemovedDefault)
	if len(fs) != 1 {
		t.Fatalf("values-removed-default = %+v (all %+v)", fs, r.Findings)
	}
	f := fs[0]
	if f.Classification != domain.ImpactReviewRequired {
		t.Errorf("class = %s, want review-required (never action-required from this rule)", f.Classification)
	}
	if len(f.Matches) != 1 || f.Matches[0].Kind != domain.MatchAbsence || !strings.Contains(f.Matches[0].Subject, "exporter.serviceMonitor.bearerTokenFile") {
		t.Errorf("matches = %+v (the values files' absence of the key is the environment chain)", f.Matches)
	}
	if len(f.EnvironmentEvidence) == 0 || len(f.UpstreamEvidence) == 0 {
		t.Errorf("both chains required: %+v", f)
	}
	if !strings.Contains(f.Detail, `/var/run/secrets/kubernetes.io/serviceaccount/token`) {
		t.Errorf("detail must state the default relied on: %s", f.Detail)
	}
	if r.Summary.NotAffected != 0 || r.Summary.ActionRequired != 0 {
		t.Errorf("summary = %+v", r.Summary)
	}
}

// An empty source default ("", null, {}, []) gave the environment nothing to
// rely on: today's values-unset verdict stays.
func TestRemovedKeyWithEmptyDefaultStaysNotAffected(t *testing.T) {
	for _, def := range []string{`""`, "null", "{}", "[]"} {
		eb := newEdge()
		eb.removedWithDefault("exporter.extraArgs", def)
		r := buildReport(t, eb.edge, valuesEnv(t, "exporter:\n  enabled: true\n"))
		if fs := findingsByRule(r, RuleValuesUnset); len(fs) != 1 || fs[0].Classification != domain.ImpactNotAffected {
			t.Errorf("default %s: values-unset = %+v (all %+v)", def, fs, r.Findings)
		}
		if fs := findingsByRule(r, RuleValuesRemovedDefault); len(fs) != 0 {
			t.Errorf("default %s: no reliance on an empty default: %+v", def, fs)
		}
	}
}

// A key the environment sets keeps today's logic (the removed key's value no
// longer takes effect), whatever its default.
func TestRemovedKeyWithDefaultSetByEnvironmentUnchanged(t *testing.T) {
	eb := newEdge()
	eb.removedWithDefault("exporter.serviceMonitor.bearerTokenFile", `"/var/run/token"`)
	r := buildReport(t, eb.edge, valuesEnv(t, "exporter:\n  serviceMonitor:\n    bearerTokenFile: /custom\n"))
	if fs := findingsByRule(r, RuleValuesRemoved); len(fs) != 1 {
		t.Errorf("values-removed = %+v (all %+v)", fs, r.Findings)
	}
	if fs := findingsByRule(r, RuleValuesRemovedDefault); len(fs) != 0 {
		t.Errorf("a set key is not a relied-on default: %+v", fs)
	}
}

// Section removals: only keys with a non-empty default are relied on; a key
// past the diff's enumeration bound has an unknown default, never an empty
// one; all-empty sections stay not-affected.
func TestRemovedSectionDefaults(t *testing.T) {
	section := func(eb *edgeBuilder, detail string, keys ...string) {
		eb.change("values:removed", "Helm values `x.*` removed", keys...)
		eb.edge.Changes[len(eb.edge.Changes)-1].Detail = detail
	}
	head := "`x` of chart demo no longer exists in v1.1.0. Remove these keys from your values and migrate the configuration they controlled; values set here no longer take effect (or are rejected when the chart validates values against a schema).\nRemoved keys:\n"

	eb := newEdge()
	section(eb, head+"x.a (default \"\")\nx.b (default {})", "x.a", "x.b")
	r := buildReport(t, eb.edge, valuesEnv(t, "y: 1\n"))
	if fs := findingsByRule(r, RuleValuesUnset); len(fs) != 1 {
		t.Errorf("all-empty section: values-unset = %+v (all %+v)", fs, r.Findings)
	}

	eb = newEdge()
	section(eb, head+"x.a (default \"\")\nx.b (default 8080)", "x.a", "x.b")
	r = buildReport(t, eb.edge, valuesEnv(t, "y: 1\n"))
	fs := findingsByRule(r, RuleValuesRemovedDefault)
	if len(fs) != 1 || len(fs[0].Matches) != 1 || !strings.Contains(fs[0].Matches[0].Subject, "x.b") {
		t.Errorf("only x.b is relied on: %+v (all %+v)", fs, r.Findings)
	}

	eb = newEdge()
	section(eb, head+"x.a (default \"\")\n… and 1 more", "x.a", "x.z")
	r = buildReport(t, eb.edge, valuesEnv(t, "y: 1\n"))
	fs = findingsByRule(r, RuleValuesRemovedDefault)
	if len(fs) != 1 || !strings.Contains(fs[0].Detail, "x.z (default not recorded") {
		t.Errorf("an unrecorded default is never read as empty: %+v (all %+v)", fs, r.Findings)
	}
}

// Default-changed keys left unset stay with PO-3 (docs/ACTION_CLASSIFICATION.md):
// rendering decides; without a render the product owner's not-affected stands,
// visibly unrendered (the investigation's validate.join treats that verdict as
// a gap, never as a clearing pass).
func TestDefaultChangedUnsetStaysPO3(t *testing.T) {
	eb := newEdge()
	eb.change("values:default-changed", "Default of Helm value `exporter.port` changed: 9100 → 9200", "exporter.port")
	eb.edge.Changes[0].Detail = "9100 → 9200"
	r := buildReport(t, eb.edge, valuesEnv(t, "y: 1\n"))
	if fs := findingsByRule(r, RuleValuesDefaultUnrendered); len(fs) != 1 {
		t.Errorf("values-default-unrendered = %+v (all %+v)", fs, r.Findings)
	}
	if fs := findingsByRule(r, RuleValuesRemovedDefault); len(fs) != 0 {
		t.Errorf("the removed-default rule is for removed keys: %+v", fs)
	}
}
