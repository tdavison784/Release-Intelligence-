package semvalidate

import (
	"encoding/json"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestValuesValidator(t *testing.T) {
	from := newRel("v1.0.0").values("chart", "demo",
		"logConfig.enabled", "true", "logConfig.level", `"info"`,
		"rotationPolicy", `"Never"`, "oldName", `"x"`, "keep", `"k"`, "tls.mode", `"a"`)
	to := newRel("v1.1.0").values("chart", "demo",
		"rotationPolicy", `"Always"`, "newName", `"x"`, "keep", `"k"`, "fresh", "1", "tls.mode", `"a"`)
	O, R, I := domain.OutcomeConfirmed, domain.OutcomeRefuted, domain.OutcomeInconclusive
	cases := []struct {
		name    string
		subj    string
		chg     *domain.ChangeSpec
		subject domain.ValidationOutcome
		change  domain.ValidationOutcome
	}{
		{"section removed", "logConfig", change(domain.ChangeKindRemoved, "", ""), O, O},
		{"leaf removed", "logConfig.level", change(domain.ChangeKindRemoved, "", ""), O, O},
		{"removed but still present", "keep", change(domain.ChangeKindRemoved, "", ""), O, R},
		{"removed but never existed", "ghost.key", change(domain.ChangeKindRemoved, "", ""), R, R},
		{"added", "fresh", change(domain.ChangeKindAdded, "", ""), O, O},
		{"added but already there", "keep", change(domain.ChangeKindAdded, "", ""), O, R},
		{"default changed", "rotationPolicy", change(domain.ChangeKindDefaultChanged, `"Never"`, `"Always"`), O, O},
		{"default changed, bare strings", "rotationPolicy", change(domain.ChangeKindDefaultChanged, `Never`, `Always`), O, O},
		{"wrong before default", "rotationPolicy", change(domain.ChangeKindDefaultChanged, `"Always"`, `"Never"`), O, R},
		{"wrong after default", "rotationPolicy", change(domain.ChangeKindDefaultChanged, `"Never"`, `"Sometimes"`), O, R},
		{"default of a key that did not change", "keep", change(domain.ChangeKindDefaultChanged, `"k"`, `"z"`), O, R},
		{"default of a key that was removed", "oldName", change(domain.ChangeKindDefaultChanged, `"x"`, `"y"`), O, R},
		{"default of a section", "tls", change(domain.ChangeKindDefaultChanged, `"a"`, `"b"`), O, I},
		{"behavior is not decidable", "rotationPolicy", change(domain.ChangeKindBehaviorChanged, "", ""), O, I},
		{"nonexistent plausible path", "rotationPolicyMode", change(domain.ChangeKindDefaultChanged, `"Never"`, `"Always"`), R, R},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := run(t, valuesValidator{}, input(from, to, assertion(helmSubject(tc.subj), tc.chg)))
			if got[domain.AspectSubject] != tc.subject || got[domain.AspectChange] != tc.change {
				t.Errorf("subject/change = %s/%s, want %s/%s", got[domain.AspectSubject], got[domain.AspectChange], tc.subject, tc.change)
			}
		})
	}
}

// A rename is proven only when the old key disappears and the new key appears;
// a key that exists under the new name only cannot be the renamed subject.
func TestValuesRename(t *testing.T) {
	from := newRel("v1.0.0").values("chart", "demo", "oldName", `"x"`, "both", "1", "kept", "1")
	to := newRel("v1.1.0").values("chart", "demo", "newName", `"x"`, "both", "1", "kept", "1")
	rename := func(old, repl string) domain.SemanticAssertion {
		c := change(domain.ChangeKindRenamed, "", "")
		c.ReplacedBy = helmSubject(repl)
		return assertion(helmSubject(old), c)
	}
	for _, tc := range []struct {
		name, old, repl string
		subject, change domain.ValidationOutcome
	}{
		{"real rename", "oldName", "newName", domain.OutcomeConfirmed, domain.OutcomeConfirmed},
		{"old name in neither release", "newName0", "newName", domain.OutcomeRefuted, domain.OutcomeRefuted},
		{"asserted old name exists only under the new name", "newName", "oldName", domain.OutcomeConfirmed, domain.OutcomeRefuted},
		{"replacement absent", "oldName", "nowhere", domain.OutcomeConfirmed, domain.OutcomeRefuted},
		{"old key still present", "both", "newName", domain.OutcomeConfirmed, domain.OutcomeRefuted},
		{"replacement already existed", "oldName", "kept", domain.OutcomeConfirmed, domain.OutcomeRefuted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := run(t, valuesValidator{}, input(from, to, rename(tc.old, tc.repl)))
			if got[domain.AspectSubject] != tc.subject || got[domain.AspectChange] != tc.change {
				t.Errorf("subject/change = %s/%s, want %s/%s", got[domain.AspectSubject], got[domain.AspectChange], tc.subject, tc.change)
			}
		})
	}
}

func TestValuesDeprecatedNeedsAReplacementThatExists(t *testing.T) {
	from := newRel("v1.0.0").values("chart", "demo", "old", "1")
	to := newRel("v1.1.0").values("chart", "demo", "old", "1", "new", "1")
	dep := func(repl string) domain.SemanticAssertion {
		c := change(domain.ChangeKindDeprecated, "", "")
		c.ReplacedBy = helmSubject(repl)
		return assertion(helmSubject("old"), c)
	}
	if got := run(t, valuesValidator{}, input(from, to, dep("new"))); got[domain.AspectChange] != domain.OutcomeInconclusive {
		t.Errorf("deprecation with an existing replacement: %v (a deprecation is not provable from values)", got)
	}
	if got := run(t, valuesValidator{}, input(from, to, dep("missing"))); got[domain.AspectChange] != domain.OutcomeRefuted {
		t.Errorf("deprecation with a missing replacement: %v", got)
	}
}

func TestValuesNoSnapshotsIsInconclusiveAndOneChartCanBeSelected(t *testing.T) {
	bare1, bare2 := newRel("v1.0.0"), newRel("v1.1.0")
	got := run(t, valuesValidator{}, input(bare1, bare2, assertion(helmSubject("a"), change(domain.ChangeKindRemoved, "", ""))))
	if got[domain.AspectSubject] != domain.OutcomeInconclusive || got[domain.AspectChange] != domain.OutcomeInconclusive {
		t.Errorf("no snapshots: %v", got)
	}
	from := newRel("v1.0.0").values("base", "base", "k", "1").values("istiod", "istiod", "k", "2")
	to := newRel("v1.1.0").values("base", "base", "k", "1").values("istiod", "istiod", "k", "3")
	s := helmSubject("k")
	s.Name = "istiod"
	got = run(t, valuesValidator{}, input(from, to, assertion(s, change(domain.ChangeKindDefaultChanged, "2", "3"))))
	if got[domain.AspectChange] != domain.OutcomeConfirmed {
		t.Errorf("chart-selected default: %v", got)
	}
	s.Name = "base"
	got = run(t, valuesValidator{}, input(from, to, assertion(s, change(domain.ChangeKindDefaultChanged, "2", "3"))))
	if got[domain.AspectChange] != domain.OutcomeRefuted {
		t.Errorf("the other chart's default must not confirm: %v", got)
	}
}

func TestValuesEvidenceCitesBothSides(t *testing.T) {
	from := newRel("v1.0.0").values("chart", "demo", "a", "1")
	to := newRel("v1.1.0").values("chart", "demo", "a", "2")
	rs, _ := valuesValidator{}.Validate(nil, input(from, to, assertion(helmSubject("a"), change(domain.ChangeKindDefaultChanged, "1", "2"))))
	if len(rs) != 1 || len(rs[0].Evidence) != 2 {
		t.Fatalf("evidence = %+v", rs)
	}
	uris := map[string]bool{}
	for _, e := range rs[0].Evidence {
		uris[e.URI] = true
	}
	if !uris["https://x/v1.0.0/chart"] || !uris["https://x/v1.1.0/chart"] {
		t.Errorf("both releases' snapshots must be cited: %v", uris)
	}
}

// Audit regressions (docs/phase3/learning-loop/VALIDATOR-AUDIT.md).

// Istio 1.23 snapshots its chart values under a `defaults.` wrapper, 1.24 does
// not: a key that merely changed root must not read as removed or added.
func TestValuesRootedDifferentlyAtTheTwoReleasesNeverConfirmsOrRefutes(t *testing.T) {
	wrapped := []string{"defaults.global.platform", `""`, "defaults.seLinuxOptions", "{}", "defaults.a", "1", "defaults.b", "2", "defaults.c", "3", "defaults.d", "4"}
	plain := []string{"global.platform", `""`, "seLinuxOptions", "{}", "a", "1", "b", "2", "c", "3", "d", "4"}
	from := newRel("v1.23.4").values("chart-base", "base", wrapped...)
	to := newRel("v1.24.0").values("chart-base", "base", plain...)
	for _, c := range []*domain.ChangeSpec{
		change(domain.ChangeKindAdded, "", ""),
		change(domain.ChangeKindRemoved, "", ""),
		change(domain.ChangeKindDefaultChanged, `"x"`, `"y"`),
	} {
		for _, path := range []string{"global.platform", "seLinuxOptions", "defaults.a", "nowhere"} {
			got := run(t, valuesValidator{}, input(from, to, assertion(helmSubject(path), c)))
			if got[domain.AspectSubject] != domain.OutcomeInconclusive || got[domain.AspectChange] != domain.OutcomeInconclusive {
				t.Errorf("%s %s: %v", c.Type, path, got)
			}
		}
	}
	// the same chart rooted alike on both sides is compared normally
	to2 := newRel("v1.24.0").values("chart-base", "base", wrapped...)
	if got := run(t, valuesValidator{}, input(from, to2, assertion(helmSubject("defaults.a"), change(domain.ChangeKindRemoved, "", "")))); got[domain.AspectChange] != domain.OutcomeRefuted {
		t.Errorf("alike-rooted: %v", got)
	}
}

// A key present in several charts is only confirmed when the charts agree,
// unless the subject names the chart.
func TestValuesMultiChartKeyNeedsAgreement(t *testing.T) {
	from := newRel("v1.0.0").values("cni", "cni", "env", "1").values("ztunnel", "ztunnel", "k", "1")
	to := newRel("v1.1.0").values("cni", "cni", "env", "1", "seLinuxOptions", "{}").values("ztunnel", "ztunnel", "k", "1", "env", "1", "seLinuxOptions", "{}")
	added := change(domain.ChangeKindAdded, "", "")
	// seLinuxOptions is new in both charts: agreement confirms
	if got := run(t, valuesValidator{}, input(from, to, assertion(helmSubject("seLinuxOptions"), added))); got[domain.AspectChange] != domain.OutcomeConfirmed {
		t.Errorf("both charts add it: %v", got)
	}
	// env existed in cni, is new in ztunnel
	if got := run(t, valuesValidator{}, input(from, to, assertion(helmSubject("env"), added))); got[domain.AspectChange] != domain.OutcomeInconclusive {
		t.Errorf("charts disagree: %v", got)
	}
	named := helmSubject("env")
	named.Name = "ztunnel"
	if got := run(t, valuesValidator{}, input(from, to, assertion(named, added))); got[domain.AspectChange] != domain.OutcomeConfirmed {
		t.Errorf("named chart: %v", got)
	}
	named.Name = "cni"
	if got := run(t, valuesValidator{}, input(from, to, assertion(named, added))); got[domain.AspectChange] != domain.OutcomeRefuted {
		t.Errorf("named chart that had it: %v", got)
	}
}

func TestValuesDefaultAsStringEncodedList(t *testing.T) {
	from := newRel("v1.0.0").values("chart", "demo", "tolerations", `[{"key":"a"}]`)
	to := newRel("v1.1.0").values("chart", "demo", "tolerations", `[{"key":"b"}]`)
	enc := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	got := run(t, valuesValidator{}, input(from, to, assertion(helmSubject("tolerations"), change(domain.ChangeKindDefaultChanged, enc(`[{"key":"a"}]`), enc(`[{"key":"b"}]`)))))
	if got[domain.AspectChange] != domain.OutcomeConfirmed {
		t.Errorf("string-encoded object: %v", got)
	}
	got = run(t, valuesValidator{}, input(from, to, assertion(helmSubject("tolerations"), change(domain.ChangeKindDefaultChanged, enc(`[{"key":"z"}]`), enc(`[{"key":"b"}]`)))))
	if got[domain.AspectChange] != domain.OutcomeRefuted {
		t.Errorf("wrong object: %v", got)
	}
}
