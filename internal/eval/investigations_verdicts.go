package eval

// Agent-verdict scoring (docs/phase5/learning-loop/PROTOCOL.md §5, §8): the
// agent's provisional class per item is scored against the expected class in a
// table of its own. It is ungated and never feeds the policy metrics: the
// agent's reasoning is measured independently of what policy may approve.

import (
	"fmt"
	"io"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// VerdictInput is the part of a stored agent verdict (PROTOCOL §8
// Investigation.AgentVerdicts) the scorer needs. Item is the change item id,
// matched to expected items exactly like policy findings.
type VerdictInput struct {
	Item       string
	Class      domain.FindingClass
	Confidence string
	Mechanism  string
}

// Verdict outcomes. A verdict is "correct" when its class equals the expected
// one; otherwise the direction of the error is what matters.
const (
	VerdictCorrect    = "correct"
	VerdictFalseSafe  = "false-safe"  // agent not-affected, expected anything else
	VerdictFalseAlarm = "false-alarm" // agent affected, expected not-affected
	VerdictHedged     = "hedged"      // agent potentially-affected / insufficient-evidence, not equal to expected
	VerdictOverstated = "overstated"  // agent affected, expected potentially-affected / insufficient-evidence
	VerdictNone       = "no-verdict"  // the agent produced no verdict for the item
)

// VerdictOutcomes lists the outcomes in table order.
var VerdictOutcomes = []string{VerdictCorrect, VerdictFalseSafe, VerdictFalseAlarm, VerdictHedged, VerdictOverstated, VerdictNone}

// VerdictDecision is one (case, expected item) agent verdict score.
type VerdictDecision struct {
	Case       string              `json:"case"`
	Item       string              `json:"item"`
	Expected   domain.FindingClass `json:"expected"`
	Agent      domain.FindingClass `json:"agent,omitempty"`
	ChangeItem string              `json:"changeItem,omitempty"`
	Outcome    string              `json:"outcome"`
	Confidence string              `json:"confidence,omitempty"`
	Mechanism  string              `json:"mechanism,omitempty"`
}

// VerdictOutcome classifies an agent class against the expected class.
func VerdictOutcome(agent, expected domain.FindingClass) string {
	switch {
	case agent == expected:
		return VerdictCorrect
	case agent == domain.FindingNotAffected:
		return VerdictFalseSafe
	case agent == domain.FindingAffected && expected == domain.FindingNotAffected:
		return VerdictFalseAlarm
	case agent == domain.FindingAffected:
		return VerdictOverstated
	case agent == domain.FindingPotentiallyAffected || agent == domain.FindingInsufficientEvidence:
		return VerdictHedged
	}
	return VerdictHedged // an unknown class is never credited
}

// ScoreVerdicts scores the agent verdicts of every run against the case
// expectations. Items matching several change items take the strongest agent
// class among them, as policy classes do. Returns nil when no run carries any
// verdict (an arm without verdicts has nothing to show).
func ScoreVerdicts(cases []*InvCase, runs map[string]*InvRun) []VerdictDecision {
	any := false
	for _, r := range runs {
		if r != nil && len(r.Verdicts) > 0 {
			any = true
		}
	}
	if !any {
		return nil
	}
	var out []VerdictDecision
	for _, c := range cases {
		r := runs[c.ID]
		for _, exp := range c.Expected.Items {
			d := VerdictDecision{Case: c.ID, Item: exp.Item, Expected: exp.Classification, Outcome: VerdictNone}
			if r == nil || r.Error != "" || r.Investigation == nil || r.Bundle == nil {
				out = append(out, d)
				continue
			}
			ix := newRunIndex(r)
			byItem := map[string]VerdictInput{}
			for _, v := range r.Verdicts {
				if cur, ok := byItem[v.Item]; !ok || classRank[v.Class] > classRank[cur.Class] {
					byItem[v.Item] = v
				}
			}
			var best *VerdictInput
			for _, id := range r.Bundle.InScope() {
				it, ok := r.Bundle.Item(id)
				if !ok || ix.matchItem(exp.Match, it) == "" {
					continue
				}
				if v, ok := byItem[id]; ok && (best == nil || classRank[v.Class] > classRank[best.Class]) {
					v := v
					best = &v
				}
			}
			if best != nil {
				d.Agent, d.ChangeItem, d.Confidence, d.Mechanism = best.Class, best.Item, best.Confidence, best.Mechanism
				d.Outcome = VerdictOutcome(best.Class, exp.Classification)
			}
			out = append(out, d)
		}
	}
	return out
}

// VerdictCounts tallies outcomes (the confusion summary).
func VerdictCounts(ds []VerdictDecision) map[string]int {
	m := map[string]int{}
	for _, d := range ds {
		m[d.Outcome]++
	}
	return m
}

// WriteVerdictText prints the agent-verdict table (separate from the policy
// tables; ungated).
func WriteVerdictText(w io.Writer, ds []VerdictDecision) {
	if len(ds) == 0 {
		return
	}
	fmt.Fprintf(w, "\n── agent verdicts (provisional classes; ungated, never an input to policy) ──\n")
	for _, d := range ds {
		agent := string(d.Agent)
		if agent == "" {
			agent = "-"
		}
		fmt.Fprintf(w, "  %-44s %-3s expected %-21s agent %-21s %s\n", d.Case, d.Item, d.Expected, agent, d.Outcome)
	}
	cnt := VerdictCounts(ds)
	var keys []string
	for _, o := range VerdictOutcomes {
		keys = append(keys, fmt.Sprintf("%s=%d", o, cnt[o]))
	}
	fmt.Fprintf(w, "  outcomes: %v\n", keys)
}
