package semvalidate

import (
	"encoding/json"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// The edge is built by the real differ, so the shapes this validator parses
// are the shapes internal/upgrade renders.
func restatementFixture(t *testing.T) (*rel, *rel, *domain.UpgradeEdge) {
	t.Helper()
	from := newRel("v1.0.0").
		values("chart", "demo", "logConfig.enabled", "true", "logConfig.level", `"info"`, "rotationPolicy", `"Never"`, "old", "1").
		crds("crds", crd("certificates.cert-manager.io", "cert-manager.io", "Certificate",
			crdVer("v1beta1", true, true, fld("spec.gone", "string", ""), fld("spec.mode", "string", `"a"`), fld("spec.opt", "string", "")))).
		images("manifest", "quay.io/acme/old:1", "quay.io/acme/keep:1")
	to := newRel("v1.1.0").
		values("chart", "demo", "rotationPolicy", `"Always"`, "fresh", "1").
		crds("crds", crd("certificates.cert-manager.io", "cert-manager.io", "Certificate",
			crdVer("v1beta1", true, true, fld("spec.mode", "string", `"b"`), withRequired(fld("spec.opt", "string", "")), fld("spec.born", "string", "")))).
		images("manifest", "quay.io/acme/new:1", "quay.io/acme/keep:1")
	return from, to, edgeOf(t, from, to)
}

func TestRestatementConfirmsAgainstComputedDiffs(t *testing.T) {
	from, to, edge := restatementFixture(t)
	O, R, I := domain.OutcomeConfirmed, domain.OutcomeRefuted, domain.OutcomeInconclusive
	img := func(n string) *domain.Subject {
		return &domain.Subject{Family: domain.SubjectImage, Product: "demo", Name: n}
	}
	cases := []struct {
		name            string
		subj            *domain.Subject
		chg             *domain.ChangeSpec
		subject, change domain.ValidationOutcome
	}{
		{"values section removed", helmSubject("logConfig"), change(domain.ChangeKindRemoved, "", ""), O, O},
		{"values key removed", helmSubject("old"), change(domain.ChangeKindRemoved, "", ""), O, O},
		{"values key added", helmSubject("fresh"), change(domain.ChangeKindAdded, "", ""), O, O},
		{"values default", helmSubject("rotationPolicy"), change(domain.ChangeKindDefaultChanged, `"Never"`, `"Always"`), O, O},
		{"values default, wrong before", helmSubject("rotationPolicy"), change(domain.ChangeKindDefaultChanged, `"Sometimes"`, `"Always"`), O, R},
		{"values: removed but the diff says default changed", helmSubject("rotationPolicy"), change(domain.ChangeKindRemoved, "", ""), O, R},
		{"values: deprecated is not contradicted by a removal", helmSubject("old"), change(domain.ChangeKindDeprecated, "", ""), O, I},
		{"crd field removed", crdSubject("cert-manager.io", "Certificate", "spec.gone"), change(domain.ChangeKindRemoved, "", ""), O, O},
		{"crd field added", crdSubject("cert-manager.io", "Certificate", "spec.born"), change(domain.ChangeKindAdded, "", ""), O, O},
		{"crd default", crdSubject("cert-manager.io", "Certificate", "spec.mode"), change(domain.ChangeKindDefaultChanged, `"a"`, `"b"`), O, O},
		{"crd default, wrong after", crdSubject("cert-manager.io", "Certificate", "spec.mode"), change(domain.ChangeKindDefaultChanged, `"a"`, `"c"`), O, R},
		{"crd now required", crdSubject("cert-manager.io", "Certificate", "spec.opt"), change(domain.ChangeKindNowRequired, "", ""), O, O},
		{"crd: another kind with the same path", crdSubject("cert-manager.io", "Issuer", "spec.gone"), change(domain.ChangeKindRemoved, "", ""), I, I},
		{"image removed", img("quay.io/acme/old"), change(domain.ChangeKindRemoved, "", ""), O, O},
		{"image added", img("quay.io/acme/new"), change(domain.ChangeKindAdded, "", ""), O, O},
		{"image unchanged", img("quay.io/acme/keep"), change(domain.ChangeKindRemoved, "", ""), I, I},
		{"not mentioned", helmSubject("nothing"), change(domain.ChangeKindRemoved, "", ""), I, I},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := input(from, to, assertion(tc.subj, tc.chg))
			in.Edge = edge
			got := run(t, restatementValidator{}, in)
			if got[domain.AspectSubject] != tc.subject || got[domain.AspectChange] != tc.change {
				t.Errorf("subject/change = %s/%s, want %s/%s", got[domain.AspectSubject], got[domain.AspectChange], tc.subject, tc.change)
			}
		})
	}
}

func TestRestatementCitesTheComputedChangeEvidence(t *testing.T) {
	from, to, edge := restatementFixture(t)
	in := input(from, to, assertion(helmSubject("logConfig"), change(domain.ChangeKindRemoved, "", "")))
	in.Edge = edge
	rs, err := restatementValidator{}.Validate(nil, in)
	if err != nil || len(rs) != 1 {
		t.Fatalf("%v %v", rs, err)
	}
	if len(rs[0].Evidence) != 2 {
		t.Errorf("a values diff cites the snapshots of both sides, got %d", len(rs[0].Evidence))
	}
	if rs[0].Validator != ProducerRestatement || rs[0].Checks[0].Rule != "restatement:values:section-removed" {
		t.Errorf("%+v", rs[0])
	}
}

func TestRestatementIgnoresNoteDerivedChanges(t *testing.T) {
	from, to, edge := restatementFixture(t)
	// a prose change naming the key proves nothing about the artifacts
	edge.Changes = append(edge.Changes, domain.Change{ID: "chg-note", Title: "Helm value `zzz` removed", Subjects: []string{"zzz"},
		Provenance: domain.Provenance{Method: domain.MethodDeclared, Rule: "values:removed"}})
	in := input(from, to, assertion(helmSubject("zzz"), change(domain.ChangeKindRemoved, "", "")))
	in.Edge = edge
	if got := run(t, restatementValidator{}, in); got[domain.AspectChange] != domain.OutcomeInconclusive {
		t.Errorf("note-derived change must not restate: %v", got)
	}
	in.Edge = nil
	rs, _ := restatementValidator{}.Validate(nil, in)
	if rs != nil {
		t.Errorf("without an edge the validator is not applicable")
	}
}

// Audit regressions (docs/phase3/learning-loop/VALIDATOR-AUDIT.md).

func TestRestatementDoesNotWidenBelowAKeyIntoItsParent(t *testing.T) {
	from := newRel("v1.0.0").
		values("chart", "demo", "a.b", "1", "a.c", "2", "gone.x", "1", "gone.y", "2").
		values("other", "other", "k", "1")
	to := newRel("v1.1.0").
		values("chart", "demo", "a.c", "2", "a.new", "1", "a.new2", "1").
		values("other", "other", "k", "1")
	edge := edgeOf(t, from, to)
	O, I := domain.OutcomeConfirmed, domain.OutcomeInconclusive
	for _, tc := range []struct {
		name   string
		path   string
		chg    domain.ChangeKind
		change domain.ValidationOutcome
	}{
		// a.b was removed, a itself still exists
		{"parent of a removed key is not removed", "a", domain.ChangeKindRemoved, I},
		{"the removed key", "a.b", domain.ChangeKindRemoved, O},
		{"a section that vanished as a whole", "gone", domain.ChangeKindRemoved, O},
		// keys added under an existing section do not add the section
		{"parent of added keys is not added", "a", domain.ChangeKindAdded, I},
		{"an added key", "a.new", domain.ChangeKindAdded, O},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := input(from, to, assertion(helmSubject(tc.path), change(tc.chg, "", "")))
			in.Edge = edge
			got := run(t, restatementValidator{}, in)
			if got[domain.AspectChange] != tc.change {
				t.Errorf("%s %s: %v, want %s", tc.chg, tc.path, got, tc.change)
			}
		})
	}
}

func TestRestatementDefaultAsStringEncodedList(t *testing.T) {
	from := newRel("v1.0.0").values("chart", "demo", "tolerations", `[{"key":"a"}]`)
	to := newRel("v1.1.0").values("chart", "demo", "tolerations", `[{"key":"b"}]`)
	enc := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	in := input(from, to, assertion(helmSubject("tolerations"), change(domain.ChangeKindDefaultChanged, enc(`[{"key":"a"}]`), enc(`[{"key":"b"}]`))))
	in.Edge = edgeOf(t, from, to)
	if got := run(t, restatementValidator{}, in); got[domain.AspectChange] != domain.OutcomeConfirmed {
		t.Errorf("string-encoded object default: %v", got)
	}
}

// a computed key diff of a chart whose keys changed root is not a restatement
func TestRestatementIgnoresReRootedCharts(t *testing.T) {
	wrapped := []string{"defaults.global.platform", `""`, "defaults.a", "1", "defaults.b", "2", "defaults.c", "3", "defaults.d", "4"}
	plain := []string{"global.platform", `""`, "a", "1", "b", "2", "c", "3", "d", "4"}
	from := newRel("v1.23.4").values("chart-base", "base", wrapped...)
	to := newRel("v1.24.0").values("chart-base", "base", plain...)
	in := input(from, to, assertion(helmSubject("global.platform"), change(domain.ChangeKindAdded, "", "")))
	in.Edge = edgeOf(t, from, to)
	if got := run(t, restatementValidator{}, in); got[domain.AspectChange] != domain.OutcomeInconclusive || got[domain.AspectSubject] != domain.OutcomeInconclusive {
		t.Errorf("re-rooted chart: %v", got)
	}
}
