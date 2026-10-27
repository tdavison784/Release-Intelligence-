package eval

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestScoreJudgementsSideTable(t *testing.T) {
	v := func(s string) *domain.Version { return &domain.Version{Tag: "v" + s, Semver: s} }
	complete := &domain.UpgradeJudgement{Status: domain.JudgementComplete, Recommendation: domain.RecommendModifyTarget, Headline: "Modify target to 1.1.3.",
		Target: domain.JudgementTarget{Proposed: *v("1.1.0"), Chosen: v("1.1.3"), Minimum: v("1.1.2"), Recommended: v("1.1.3"), Changed: true, Rule: "latest patch"},
		Changes:  []domain.JudgementChange{{Order: 1, Kind: domain.StepPinBehaviour, Location: "loc-1"}, {Order: 2, Kind: domain.StepVersionBump, Proposed: true}},
		Findings: []domain.JudgementFinding{{Kind: domain.JFSecurityRegression, Hard: true}, {Kind: domain.JFUntracedConsumer}},
		Summary:  domain.JudgementSummary{Unknown: []string{"x"}}, Backout: domain.Backout{Statement: "restore"}}
	failed := &domain.UpgradeJudgement{Status: domain.JudgementFailed, Target: domain.JudgementTarget{Proposed: *v("1.1.0"), Rule: "minimum"},
		Backout: domain.Backout{Statement: "restore"}, Failure: &domain.JudgementFailure{Attempts: 3}}
	cases := []*InvCase{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	runs := map[string]*InvRun{
		"a": {Investigation: &domain.Investigation{Judgement: complete}},
		"b": {Investigation: &domain.Investigation{Judgement: failed}},
		"c": {Investigation: &domain.Investigation{}},
	}
	rows := ScoreJudgements(cases, runs)
	if len(rows) != 3 {
		t.Fatalf("%d rows", len(rows))
	}
	a := rows[0]
	if a.Recommendation != domain.RecommendModifyTarget || a.Chosen != "v1.1.3" || a.Minimum != "v1.1.2" || a.Changes != 2 || a.Located != 1 ||
		a.Hard != 1 || a.Findings != 2 || strings.Join(a.Missing, ",") != "validation" {
		t.Errorf("row a = %+v", a)
	}
	if rows[1].Status != domain.JudgementFailed || len(rows[1].Missing) != 1 || rows[2].Status != "" || rows[2].Missing[0] != "judgement" {
		t.Errorf("rows b, c = %+v %+v", rows[1], rows[2])
	}
	var b strings.Builder
	WriteJudgementText(&b, rows)
	if !strings.Contains(b.String(), "upgrade judgements (ungated") || !strings.Contains(b.String(), "no judgement") {
		t.Errorf("text:\n%s", b.String())
	}
	// no judgement anywhere: no table (earlier arms' output is unchanged)
	if ScoreJudgements(cases[2:], runs) != nil {
		t.Error("a table without any judgement")
	}
}
