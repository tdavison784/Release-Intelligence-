package eval

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestVerdictOutcome(t *testing.T) {
	A, N, P, I := domain.FindingAffected, domain.FindingNotAffected, domain.FindingPotentiallyAffected, domain.FindingInsufficientEvidence
	for _, tc := range []struct {
		agent, expected domain.FindingClass
		want            string
	}{
		{A, A, VerdictCorrect}, {N, N, VerdictCorrect}, {P, P, VerdictCorrect},
		{N, A, VerdictFalseSafe}, {N, P, VerdictFalseSafe}, {N, I, VerdictFalseSafe},
		{A, N, VerdictFalseAlarm}, {P, N, VerdictHedged}, {I, N, VerdictHedged}, {P, A, VerdictHedged}, {I, A, VerdictHedged},
		{A, P, VerdictOverstated}, {A, I, VerdictOverstated},
		{"", A, VerdictHedged}, // an unknown class is never credited as safe or correct
	} {
		if got := VerdictOutcome(tc.agent, tc.expected); got != tc.want {
			t.Errorf("agent %q vs expected %q: got %s want %s", tc.agent, tc.expected, got, tc.want)
		}
	}
}

// B1 (expected affected) is realised by ci-a and ci-b in the synthetic run;
// B2 (expected not-affected) matches nothing.
func TestScoreVerdictsSeparateFromPolicy(t *testing.T) {
	c := loadSynth(t)
	r := synthRun(domain.FindingAffected, domain.FindingNotAffected)
	policy, _ := ScoreInvCase(c, r)

	// No verdicts: no table.
	if got := ScoreVerdicts([]*InvCase{c}, map[string]*InvRun{c.ID: r}); got != nil {
		t.Fatalf("an arm with no verdicts must have no verdict table, got %v", got)
	}

	// The strongest agent class among the matched items counts; the weak
	// not-affected verdict on ci-b must not hide the affected one on ci-a,
	// and a false-safe verdict must be reported as such.
	r.Verdicts = []VerdictInput{{Item: "ci-a", Class: domain.FindingAffected}, {Item: "ci-b", Class: domain.FindingNotAffected}}
	vs := ScoreVerdicts([]*InvCase{c}, map[string]*InvRun{c.ID: r})
	if len(vs) != 2 || vs[0].Outcome != VerdictCorrect || vs[0].ChangeItem != "ci-a" {
		t.Fatalf("B1 should be correct via ci-a: %+v", vs)
	}
	if vs[1].Item != "B2" || vs[1].Outcome != VerdictNone {
		t.Fatalf("B2 matched nothing: %+v", vs[1])
	}

	r.Verdicts = []VerdictInput{{Item: "ci-a", Class: domain.FindingNotAffected}, {Item: "ci-b", Class: domain.FindingNotAffected}}
	vs = ScoreVerdicts([]*InvCase{c}, map[string]*InvRun{c.ID: r})
	if vs[0].Outcome != VerdictFalseSafe {
		t.Fatalf("agent not-affected for an expected-affected item is false-safe: %+v", vs[0])
	}

	// Verdicts never change the policy decisions.
	again, _ := ScoreInvCase(c, r)
	if len(again) != len(policy) {
		t.Fatal("policy decisions changed")
	}
	for i := range policy {
		if again[i].System != policy[i].System || again[i].Correct != policy[i].Correct {
			t.Fatalf("verdicts leaked into policy scoring: %+v vs %+v", again[i], policy[i])
		}
	}
	s := ScoreInvestigations([]*InvCase{c}, map[string]*InvRun{c.ID: r}, nil)
	var buf bytes.Buffer
	WriteInvText(&buf, "t", s)
	if !strings.Contains(buf.String(), "agent verdicts") || !strings.Contains(buf.String(), "false-safe") {
		t.Fatalf("verdict table missing from the text report:\n%s", buf.String())
	}
}

func TestScoreVerdictsFailedRun(t *testing.T) {
	c := loadSynth(t)
	good := synthRun(domain.FindingAffected, domain.FindingNotAffected)
	good.Verdicts = []VerdictInput{{Item: "ci-a", Class: domain.FindingAffected}}
	failed := &InvRun{Case: c.ID, Error: "boom", Verdicts: good.Verdicts}
	vs := ScoreVerdicts([]*InvCase{c}, map[string]*InvRun{c.ID: failed})
	for _, v := range vs {
		if v.Outcome != VerdictNone {
			t.Fatalf("a failed run has no verdicts: %+v", v)
		}
	}
}
