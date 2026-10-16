package validate

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

func impactProv() domain.Provenance {
	return domain.Provenance{Method: domain.MethodComputed, Producer: "impact@v1", Confidence: domain.ConfidenceHigh}
}

// joinFx is a bundle with one values change and a report about it.
type joinFx struct {
	f      *fx
	b      *domain.EvidenceBundle
	item   string
	envEv  domain.Evidence
	report *domain.ImpactReport
}

func newJoinFx(t *testing.T) *joinFx {
	f := newFx(t)
	item := f.change(domain.Change{ID: "chg-mode", Category: domain.CategoryHelmValues, Breaking: true, Title: "default of mode changed",
		Subjects: []string{"mode"}, Provenance: domain.Provenance{Rule: "values:default-changed"}})
	j := &joinFx{f: f, item: item}
	j.envEv = domain.NewEvidence(domain.EvidenceLocalFile, "", "apps/gizmo/values.yaml", "L3", "mode: safe", domain.Digest([]byte("values")), testNow)
	j.report = &domain.ImpactReport{SchemaVersion: domain.ImpactReportSchemaVersion, Product: f.edge.Product, From: f.edge.From, To: f.edge.To,
		Evidence: []domain.Evidence{f.up}, EnvironmentEvidence: []domain.Evidence{j.envEv}}
	return j
}

func (j *joinFx) finding(id string, class domain.ImpactClass, rule string, mods ...func(*domain.ImpactFinding)) {
	fd := domain.ImpactFinding{ID: id, Classification: class, Rule: rule, Title: rule + " title", ChangeID: "chg-mode",
		UpstreamEvidence: []domain.EvidenceID{j.f.up.ID}, Provenance: impactProv()}
	switch {
	case class.Affected():
		fd.Matches = []domain.ImpactMatch{{Kind: domain.MatchValuesKey, Subject: "mode", Evidence: []domain.EvidenceID{j.envEv.ID}}}
		fd.EnvironmentEvidence = []domain.EvidenceID{j.envEv.ID}
	case class == domain.ImpactNotAffected:
		fd.Checks = []domain.ImpactCheck{{Dimension: domain.DimensionValues, Facts: 4, Subjects: []string{"mode"}}}
	case class == domain.ImpactUnknown:
		fd.NeededToDetermine = []string{"values"}
	}
	for _, m := range mods {
		m(&fd)
	}
	j.report.Findings = append(j.report.Findings, fd)
}

func (j *joinFx) in() investigate.ValidationInput {
	if j.b == nil {
		j.b = j.f.bundle()
	}
	return investigate.ValidationInput{Bundle: j.b, ChangeItem: j.item}
}

func TestJoinNoReportIsNotRun(t *testing.T) {
	j := newJoinFx(t)
	r := one(t, run(t, JoinVerdicts(nil, WithClock(clock)), j.in()))
	wantOutcome(t, r, domain.ValidationNotRun, "no impact report")
}

func TestJoinNoFindingsNoRecord(t *testing.T) {
	j := newJoinFx(t)
	j.finding("imp-other", domain.ImpactNotAffected, "impact:values-unset", func(f *domain.ImpactFinding) { f.ChangeID = "chg-other" })
	out := run(t, JoinVerdicts(j.report), j.in())
	if len(out.Records) != 0 {
		t.Fatalf("a finding of another change must not be wrapped: %+v", out.Records)
	}
}

func TestJoinActionRequiredFails(t *testing.T) {
	j := newJoinFx(t)
	j.finding("imp-b", domain.ImpactActionRequired, domain.RuleRenderTargetRejects, func(f *domain.ImpactFinding) {
		f.Checks = nil
		f.Matches = append(f.Matches, domain.ImpactMatch{Kind: domain.MatchRenderedChange, Subject: "render", Evidence: []domain.EvidenceID{j.envEv.ID}})
	})
	j.finding("imp-a", domain.ImpactNotAffected, "impact:values-unset")
	j.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
	out := run(t, JoinVerdicts(j.report, WithReportArtifact("")), j.in())
	r := one(t, out)
	wantOutcome(t, r, domain.ValidationFail, "ACTION REQUIRED")
	if r.ImpactFinding != "imp-b" || r.Kind != domain.ValidationRenderDiff {
		t.Fatalf("impactFinding %s kind %s", r.ImpactFinding, r.Kind)
	}
	if len(r.Subjects)+len(r.Paths) != 0 {
		t.Fatalf("a wrapped verdict speaks for the item, not for subjects: %+v", r)
	}
	if !contains(r.Inputs, j.b.Change.Edge) || !contains(r.Inputs, j.b.Renders[0].To) {
		t.Fatalf("inputs %v must name the edge and the renders the join read", r.Inputs)
	}
	found := false
	for _, e := range out.Evidence {
		found = found || e.ID == j.envEv.ID
	}
	if !found {
		t.Fatal("environment evidence from the report must travel with the record")
	}
}

func TestJoinConsensusActionIsLabelled(t *testing.T) {
	j := newJoinFx(t)
	j.finding("imp-k", domain.ImpactActionRequired, "impact:knowledge-exposed", func(f *domain.ImpactFinding) {
		f.Knowledge = &domain.KnowledgeRef{Fact: "vf-1", Verification: domain.VerifiedConsensus, ConsensusAction: true}
	})
	r := one(t, run(t, JoinVerdicts(j.report), j.in()))
	wantOutcome(t, r, domain.ValidationFail, "model consensus")
}

func TestJoinNotAffectedPasses(t *testing.T) {
	j := newJoinFx(t)
	j.finding("imp-a", domain.ImpactNotAffected, "impact:values-unset")
	j.finding("imp-c", domain.ImpactNotAffected, "impact:knowledge-clear", func(f *domain.ImpactFinding) {
		f.Knowledge = &domain.KnowledgeRef{Fact: "vf-2", Verification: domain.VerifiedHuman}
	})
	r := one(t, run(t, JoinVerdicts(nil), investigate.ValidationInput{Bundle: j.in().Bundle, ChangeItem: j.item, Report: j.report}))
	wantOutcome(t, r, domain.ValidationPass, "")
	if r.ImpactFinding != "imp-a" || r.Kind != domain.ValidationOverrideInspection || r.Pin != nil || r.ClearsBehaviour() {
		t.Fatalf("pass %+v", r)
	}
}

// "Cannot decide ⇒ never pass": every non-decisive or untrusted join shape.
func TestJoinNeverPassesWhenUndecided(t *testing.T) {
	cases := []struct {
		name     string
		add      func(j *joinFx)
		noRecord bool
		detail   string
	}{
		{"review-required", func(j *joinFx) { j.finding("imp-r", domain.ImpactReviewRequired, "impact:values-adjacent") }, true, ""},
		{"informational", func(j *joinFx) { j.finding("imp-i", domain.ImpactInformational, joinValuesPinned) }, true, ""},
		{"unknown", func(j *joinFx) { j.finding("imp-u", domain.ImpactUnknown, "impact:insufficient-visibility") }, true, ""},
		{"mixed not-affected and review", func(j *joinFx) {
			j.finding("imp-a", domain.ImpactNotAffected, "impact:values-unset")
			j.finding("imp-r", domain.ImpactReviewRequired, "impact:values-adjacent")
		}, true, ""},
		{"unrendered default", func(j *joinFx) {
			j.finding("imp-d", domain.ImpactNotAffected, domain.RuleValuesDefaultUnrendered, func(f *domain.ImpactFinding) {
				f.Checks = []domain.ImpactCheck{{Dimension: domain.DimensionRender, Subjects: []string{"mode"},
					Render: &domain.RenderCheck{Outcome: domain.RenderUnavailable, Key: "mode", Reason: "helm missing"}}}
			})
		}, false, "no render"},
		{"render check unavailable", func(j *joinFx) {
			j.finding("imp-d", domain.ImpactNotAffected, domain.RuleValuesDefaultNoEffect, func(f *domain.ImpactFinding) {
				f.Checks = []domain.ImpactCheck{{Dimension: domain.DimensionRender, Subjects: []string{"mode"},
					Render: &domain.RenderCheck{Outcome: domain.RenderUnavailable, Key: "mode", Reason: "chart unavailable"}}}
			})
		}, false, "no render"},
		{"proxy fact", func(j *joinFx) {
			j.finding("imp-p", domain.ImpactNotAffected, "impact:knowledge-clear", func(f *domain.ImpactFinding) {
				f.Knowledge = &domain.KnowledgeRef{Fact: "vf-3", Verification: domain.VerifiedProxy}
			})
		}, false, "never clear"},
		{"consensus fact", func(j *joinFx) {
			j.finding("imp-c", domain.ImpactNotAffected, "impact:knowledge-clear", func(f *domain.ImpactFinding) {
				f.Knowledge = &domain.KnowledgeRef{Fact: "vf-4", Verification: domain.VerifiedConsensus}
			})
		}, false, "never clear"},
		{"no checks", func(j *joinFx) {
			j.finding("imp-n", domain.ImpactNotAffected, "impact:values-unset", func(f *domain.ImpactFinding) { f.Checks = nil })
		}, false, "checks"},
		{"uncited", func(j *joinFx) {
			j.finding("imp-x", domain.ImpactNotAffected, "impact:values-unset", func(f *domain.ImpactFinding) {
				f.UpstreamEvidence = []domain.EvidenceID{"ev-nowhere"}
			})
		}, false, "cites no evidence"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := newJoinFx(t)
			c.add(j)
			out := run(t, JoinVerdicts(j.report), j.in())
			for _, r := range out.Records {
				if r.Outcome == domain.ValidationPass {
					t.Fatalf("undecided join verdict wrapped as pass: %s", r.Detail)
				}
			}
			if c.noRecord {
				if len(out.Records) != 0 {
					t.Fatalf("not decisive ⇒ no record, got %+v", out.Records)
				}
				return
			}
			wantOutcome(t, one(t, out), domain.ValidationNotRun, c.detail)
		})
	}
}

func TestJoinPlatformItem(t *testing.T) {
	j := newJoinFx(t)
	item := j.f.platformItem(domain.CompatibilityChange{Platform: "kubernetes", Summary: "Kubernetes support narrowed", Narrowed: true})
	j.item = item
	j.finding("imp-k8s", domain.ImpactActionRequired, "impact:kubernetes-below-range", func(f *domain.ImpactFinding) {
		f.ChangeID = ""
		f.Matches = []domain.ImpactMatch{{Kind: domain.MatchKubernetes, Subject: "1.27", Evidence: []domain.EvidenceID{j.envEv.ID}}}
	})
	r := one(t, run(t, JoinVerdicts(j.report), j.in()))
	wantOutcome(t, r, domain.ValidationFail, "")
	if r.Kind != domain.ValidationSchemaCompat {
		t.Fatalf("a cluster verdict is a schema/platform check, got %s", r.Kind)
	}
}

func TestJoinRejectsUnknownItem(t *testing.T) {
	j := newJoinFx(t)
	in := j.in()
	in.ChangeItem = "ci-nope"
	if _, err := JoinVerdicts(j.report).Validate(t.Context(), in); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown item must be an error, got %v", err)
	}
}
