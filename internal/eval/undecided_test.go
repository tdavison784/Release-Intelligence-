package eval

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// undecidedCase: E1/E2/E3 are undecided links (the fixture cannot decide
// them); E4 is an ordinary affected link the engine misses.
func undecidedCase() *Case {
	c := classCase()
	c.Environment = &Environment{
		ExpectedImpact: []ImpactLink{{Expected: "E4", Relevance: RelevanceReview, Why: "touched"}},
		UndecidedImpact: []UndecidedLink{
			{Expected: "E1", Reason: domain.UnknownEnvironmentVisibilityGap, Needed: "values"},
			{Expected: "E2", Reason: domain.UnknownRuntimeBehaviorGap, Needed: "live state"},
			{Expected: "E3", Reason: domain.UnknownCrossProductContextGap, Needed: "inventory"},
		},
	}
	return c
}

func undecidedReport() *domain.ImpactReport {
	f := func(id, change string, class domain.ImpactClass, k *domain.KnowledgeRef) domain.ImpactFinding {
		return domain.ImpactFinding{ID: id, Rule: "impact:test", Classification: class, ChangeID: change,
			UpstreamEvidence: []domain.EvidenceID{"ev-1"}, EnvironmentEvidence: []domain.EvidenceID{"ev-local"}, Knowledge: k}
	}
	return &domain.ImpactReport{
		Evidence:            []domain.Evidence{ev("ev-1", "https://example/notes.md")},
		EnvironmentEvidence: []domain.Evidence{ev("ev-local", "file://values.yaml")},
		Findings: []domain.ImpactFinding{
			// E1 honest: the engine says UNKNOWN
			f("f-1", "chg-a", domain.ImpactUnknown, nil),
			// E2 dishonest-affected: claims REVIEW on an undecidable link
			f("f-2", "chg-b", domain.ImpactReviewRequired, &domain.KnowledgeRef{Fact: "vf-reviewed-here", Verification: domain.VerifiedHuman}),
			// E3 dishonest-clear: claims NOT AFFECTED on an undecidable link
			f("f-3", "chg-c", domain.ImpactNotAffected, nil),
		},
	}
}

func TestUnknownHonesty(t *testing.T) {
	res := ScoreEntry(undecidedCase(), classEdge(), undecidedReport(), nil)
	if res.Env.UndecidedLinks != 3 || res.Env.UndecidedHonest != 1 {
		t.Fatalf("undecided %d / honest %d, want 3 / 1", res.Env.UndecidedLinks, res.Env.UndecidedHonest)
	}
	want := map[string]bool{"E1": true, "E2": false, "E3": false}
	for _, a := range res.EnvUndecided {
		if a.Honest != want[a.ExpectedID] {
			t.Errorf("%s honest = %v, want %v (overclaims %v)", a.ExpectedID, a.Honest, want[a.ExpectedID], a.Overclaims)
		}
	}
	// undecided links never enter the gated applicability accuracy
	if res.Env.ImpactLinks != 1 || res.Env.NotAffectedLinks != 0 {
		t.Errorf("links %d / not-affected %d: undecided links leaked into applicability", res.Env.ImpactLinks, res.Env.NotAffectedLinks)
	}

	// no finding at all is honest too (and vacuous)
	empty := ScoreEntry(undecidedCase(), classEdge(), &domain.ImpactReport{}, nil)
	if empty.Env.UnknownHonesty() != 1 {
		t.Errorf("an engine that emits nothing: honesty %.2f, want 1.00 (the vacuous case)", empty.Env.UnknownHonesty())
	}

	agg := AggregateResults([]EntryResult{res, empty})
	if agg.UndecidedLinks != 6 || agg.UndecidedHonest != 4 || agg.UnknownHonesty < 0.66 || agg.UnknownHonesty > 0.67 {
		t.Errorf("aggregate %d/%d = %.3f, want 4/6", agg.UndecidedHonest, agg.UndecidedLinks, agg.UnknownHonesty)
	}

	// the text report shows it next to applicability with the vacuity note
	var b strings.Builder
	if err := RenderText(&b, Report{Results: []EntryResult{empty}, Aggregate: AggregateResults([]EntryResult{empty})}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"unknown honesty 1.00 (3/3", "reported, not gated", "vacuous — no affected link was decided"} {
		if !strings.Contains(b.String(), s) {
			t.Errorf("report misses %q:\n%s", s, b.String())
		}
	}

	// transfer subset: the overclaim resting on a fact reviewed with this
	// environment is excluded; the other two remain
	tr := Transfer([]EntryResult{res}, map[string][]string{"vf-reviewed-here": {res.CaseID}})
	if tr.UndecidedLinks != 2 || tr.UndecidedHonest != 1 || tr.UnknownHonesty != 0.5 {
		t.Errorf("transfer undecided %d/%d = %.2f, want 1/2", tr.UndecidedHonest, tr.UndecidedLinks, tr.UnknownHonesty)
	}
}
