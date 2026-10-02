package eval

// Measuring verified knowledge honestly (docs/phase3/learning-loop/DESIGN.md
// §7): `ri eval -knowledge <dir>` runs the dataset once per verification
// level and reports each level separately. The human level (deterministic ∪
// human facts) is the gate number; consensus (PO-1/PO-2) and proxy levels are
// reported and labelled, never presented as the gate. Per level the eval also
// reports the transfer subset: links whose deciding facts were never reviewed
// with that case's environment as context.

import (
	"context"
	"fmt"
	"io"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// LevelNone is the knowledge-free baseline level.
const LevelNone = "none"

// EvalLevels are the levels `ri eval -knowledge` reports, in order.
var EvalLevels = []string{LevelNone, string(domain.VerifiedDeterministic), string(domain.VerifiedHuman), string(domain.VerifiedConsensus), string(domain.VerifiedProxy)}

// GateLevel is the level whose numbers are the gate (DESIGN.md §7).
const GateLevel = domain.VerifiedHuman

// KnowledgeCounts counts the knowledge findings of one entry (nil without any).
type KnowledgeCounts struct {
	Findings int            `json:"findings"`
	ByClass  map[string]int `json:"byClass"`
	// ConsensusAction counts ACTION REQUIRED findings resting on model
	// consensus (PO-2), labelled "model consensus".
	ConsensusAction int `json:"consensusAction,omitempty"`
}

// knowledgeCounts tallies a report's knowledge findings.
func knowledgeCounts(r *domain.ImpactReport) *KnowledgeCounts {
	if r == nil {
		return nil
	}
	var k *KnowledgeCounts
	for _, f := range r.Findings {
		if f.Knowledge == nil {
			continue
		}
		if k == nil {
			k = &KnowledgeCounts{ByClass: map[string]int{}}
		}
		k.Findings++
		k.ByClass[string(f.Classification)]++
		if f.Classification == domain.ImpactActionRequired && f.Knowledge.Verification == domain.VerifiedConsensus {
			k.ConsensusAction++
		}
	}
	return k
}

// TransferMetrics is applicability accuracy over the transfer subset.
type TransferMetrics struct {
	ImpactLinks           int     `json:"impactLinks"`
	ImpactLinksHit        int     `json:"impactLinksHit"`
	NotAffectedLinks      int     `json:"notAffectedLinks"`
	NotAffectedViolations int     `json:"notAffectedViolations"`
	ApplicabilityAccuracy float64 `json:"applicabilityAccuracy"`
	// Excluded counts links decided by a fact that was reviewed with this
	// case's environment as context (not transfer).
	Excluded int `json:"excluded"`
	// Undecided links on the transfer subset (an overclaim resting on a fact
	// reviewed with this environment is excluded like a decided link).
	UndecidedLinks  int     `json:"undecidedLinks"`
	UndecidedHonest int     `json:"undecidedHonest"`
	UnknownHonesty  float64 `json:"unknownHonesty"`
}

// Transfer computes the transfer subset: contexts maps a fact id to the
// environment labels it was reviewed with (impact.ReviewContexts); a link is
// excluded when one of its deciding facts was reviewed with the case's id as
// context.
func Transfer(rs []EntryResult, contexts map[string][]string) TransferMetrics {
	var t TransferMetrics
	for _, r := range rs {
		for _, a := range r.EnvImpact {
			seen := false
			for _, f := range a.Facts {
				for _, label := range contexts[f] {
					seen = seen || label == r.CaseID
				}
			}
			if seen {
				t.Excluded++
				continue
			}
			if a.Relevance == RelevanceNotAffected {
				t.NotAffectedLinks++
				if a.Hit {
					t.NotAffectedViolations++
				}
				continue
			}
			t.ImpactLinks++
			if a.Hit {
				t.ImpactLinksHit++
			}
		}
	}
	t.ApplicabilityAccuracy = applicabilityAccuracy(t.ImpactLinks, t.ImpactLinksHit, t.NotAffectedLinks, t.NotAffectedViolations)
	for _, r := range rs {
		for _, u := range r.EnvUndecided {
			if reviewedHere(u.Facts, contexts, r.CaseID) {
				t.Excluded++
				continue
			}
			t.UndecidedLinks++
			if u.Honest {
				t.UndecidedHonest++
			}
		}
	}
	t.UnknownHonesty = ratio(t.UndecidedLinks, t.UndecidedHonest)
	return t
}

// LevelReport is the result of one verification level.
type LevelReport struct {
	Level string `json:"level"`
	// Gate marks the level the gates were evaluated on (human by default,
	// the DESIGN.md §7 gate number; `-min-verification` names another).
	Gate      bool `json:"gate,omitempty"`
	FactsUsed int  `json:"factsUsed"`
	// Applicability and class quality.
	ApplicabilityAccuracy  float64 `json:"applicabilityAccuracy"`
	ImpactLinks            int     `json:"impactLinks"`
	ImpactLinksHit         int     `json:"impactLinksHit"`
	NotAffectedViolations  int     `json:"notAffectedViolations"`
	ClassificationAccuracy float64 `json:"classificationAccuracy"`
	UnknownRate            float64 `json:"unknownRate"`
	// ACTION REQUIRED quality (PO-2: reported per level, consensus included).
	FalseActionRate           float64 `json:"falseActionRate"`
	ActionFindings            int     `json:"actionFindings"`
	FalseActionFindings       int     `json:"falseActionFindings"`
	ActionFindingsUnsupported int     `json:"actionFindingsUnsupported"`
	// Knowledge findings of the level.
	KnowledgeFindings int             `json:"knowledgeFindings"`
	KnowledgeByClass  map[string]int  `json:"knowledgeByClass,omitempty"`
	ConsensusAction   int             `json:"consensusAction,omitempty"`
	Transfer          TransferMetrics `json:"transfer"`
	// UnknownHonesty (reported, never gated): undecided links answered
	// honestly / undecided links; vacuous when nothing is decided.
	UndecidedLinks  int     `json:"undecidedLinks"`
	UndecidedHonest int     `json:"undecidedHonest"`
	UnknownHonesty  float64 `json:"unknownHonesty"`
}

// LevelRun is one level's report plus its entry results.
type LevelRun struct {
	Report  LevelReport
	Results []EntryResult
}

// factsAt returns the active facts at or above the level ("none": no facts).
func factsAt(facts []domain.VerifiedFact, level string) []domain.VerifiedFact {
	if level == LevelNone {
		return nil
	}
	var out []domain.VerifiedFact
	for _, f := range facts {
		if f.Status == domain.FactActive && f.Level().AtLeast(domain.VerificationLevel(level)) {
			out = append(out, f)
		}
	}
	return out
}

// RunLevels runs the cases once per verification level. A level that uses
// no more facts than the previous one reuses its results (identical input,
// identical output).
func (r *Runner) RunLevels(ctx context.Context, cases []*Case, facts []domain.VerifiedFact, contexts map[string][]string) []LevelRun {
	var out []LevelRun
	var prev []EntryResult
	prevN := -1
	for _, level := range EvalLevels {
		used := factsAt(facts, level)
		var results []EntryResult
		if len(used) == prevN && prev != nil {
			results = prev
		} else {
			rr := *r
			rr.Knowledge = nil
			if level != LevelNone {
				rr.Knowledge = &KnowledgeRun{Facts: used, MinVerification: domain.VerificationLevel(level)}
			}
			results = rr.Run(ctx, cases)
		}
		prev, prevN = results, len(used)
		out = append(out, LevelRun{Report: levelReport(level, len(used), results, contexts), Results: results})
	}
	return out
}

func levelReport(level string, used int, rs []EntryResult, contexts map[string][]string) LevelReport {
	agg := AggregateResults(rs)
	lr := LevelReport{
		Level: level, Gate: level == string(GateLevel), FactsUsed: used,
		ApplicabilityAccuracy: agg.ApplicabilityAccuracy, ImpactLinks: agg.ImpactLinks, ImpactLinksHit: agg.ImpactLinksHit,
		NotAffectedViolations: agg.NotAffectedViolations, ClassificationAccuracy: agg.ClassificationAccuracy, UnknownRate: agg.UnknownRate,
		FalseActionRate: agg.FalseActionRate, ActionFindings: agg.ActionFindings, FalseActionFindings: agg.FalseActionFindings,
		ActionFindingsUnsupported: agg.ActionFindingsUnsupported,
		Transfer:                  Transfer(rs, contexts),
		UndecidedLinks:            agg.UndecidedLinks, UndecidedHonest: agg.UndecidedHonest, UnknownHonesty: agg.UnknownHonesty,
	}
	for _, r := range rs {
		if r.Knowledge == nil {
			continue
		}
		if lr.KnowledgeByClass == nil {
			lr.KnowledgeByClass = map[string]int{}
		}
		lr.KnowledgeFindings += r.Knowledge.Findings
		lr.ConsensusAction += r.Knowledge.ConsensusAction
		for c, n := range r.Knowledge.ByClass {
			lr.KnowledgeByClass[c] += n
		}
	}
	return lr
}

// RenderLevels writes the per-level panel.
func RenderLevels(w io.Writer, levels []LevelReport) {
	if len(levels) == 0 {
		return
	}
	fmt.Fprintf(w, "── verified knowledge, per verification level ────────────\n")
	fmt.Fprintf(w, "%-14s %5s  %-18s %-16s %7s %7s  %-24s %s\n", "level", "facts", "applicability", "transfer", "class", "unknown", "ACTION (false/unsupp.)", "knowledge findings")
	for _, l := range levels {
		name := l.Level
		switch {
		case l.Gate:
			name += " (gate)"
		}
		if l.Level == string(domain.VerifiedConsensus) || l.Level == string(domain.VerifiedProxy) {
			name += " (*)"
		}
		kf := fmt.Sprintf("%d", l.KnowledgeFindings)
		if l.ConsensusAction > 0 {
			kf += fmt.Sprintf(" (%d ACTION · model consensus)", l.ConsensusAction)
		}
		fmt.Fprintf(w, "%-14s %5d  %.3f (%2d/%2d)     %.3f (-%d)      %.2f    %.2f   %3d (%d/%d)               %s\n",
			name, l.FactsUsed, l.ApplicabilityAccuracy, l.ImpactLinksHit, l.ImpactLinks,
			l.Transfer.ApplicabilityAccuracy, l.Transfer.Excluded, l.ClassificationAccuracy, l.UnknownRate,
			l.ActionFindings, l.FalseActionFindings, l.ActionFindingsUnsupported, kf)
	}
	fmt.Fprintf(w, "(*) consensus and proxy levels are reported and labelled; the gate number is the human level (deterministic ∪ human facts) unless -min-verification names another.\n")
	fmt.Fprintf(w, "unknown honesty per level (undecided links answered UNKNOWN or not at all; reported, not gated; vacuous if nothing is decided — read with applicability above):\n")
	for _, l := range levels {
		if l.UndecidedLinks == 0 && l.Transfer.UndecidedLinks == 0 {
			continue
		}
		fmt.Fprintf(w, "  %-14s %.2f (%d/%d)   transfer %.2f (%d/%d)\n", l.Level, l.UnknownHonesty, l.UndecidedHonest, l.UndecidedLinks,
			l.Transfer.UnknownHonesty, l.Transfer.UndecidedHonest, l.Transfer.UndecidedLinks)
	}
}

// reviewedHere reports whether one of the facts was reviewed with the case's
// environment as context.
func reviewedHere(facts []string, contexts map[string][]string, caseID string) bool {
	for _, f := range facts {
		for _, label := range contexts[f] {
			if label == caseID {
				return true
			}
		}
	}
	return false
}
