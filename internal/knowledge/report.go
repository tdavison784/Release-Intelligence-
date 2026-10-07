package knowledge

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func pct(x float64) string { return fmt.Sprintf("%.0f%%", x*100) }

func dur(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	return d.Round(time.Second).String()
}

// WriteMetricsReport prints LoopMetrics as a readable report. Batch and
// individual decisions are shown separately, as DESIGN §7 requires.
func WriteMetricsReport(w io.Writer, m LoopMetrics) {
	p := func(f string, a ...any) { fmt.Fprintf(w, f+"\n", a...) }
	p("Learning-loop metrics")
	p("")
	p("Facts: %d active, %d retracted; anchors/fact %.1f", m.Facts.Active, m.Facts.Retracted, m.Facts.AnchorsPerFact)
	p("  by level:  %s", countList(m.Facts.ByLevel))
	p("  by family: %s", countList(m.Facts.ByFamily))
	p("  auto-validation rate %s; auto-approved %d, audited %d, agreement %s",
		pct(m.Review.AutoValidationRate), m.Facts.AutoApproved, m.Facts.AutoApprovedAudited, pct(m.Facts.AutoApprovalAgreement))
	for fam, a := range m.Facts.AutoApprovalAgreementBy {
		p("    %-24s %s", fam, pct(a))
	}
	p("  consensus ACTION: %d facts, %d audited, agreement %s", m.Facts.ConsensusAction, m.Facts.ConsensusActionAudited, pct(m.Facts.ConsensusActionAgreement))
	for sc, a := range m.Facts.ConsensusAgreementByScope {
		p("    consensus %-12s audit agreement %s", sc, pct(a))
	}
	p("")
	p("Review: %d items; repeat-pattern rate %s", m.Review.Items, pct(m.Review.RepeatPatternRate))
	p("  per question: %s", countList(m.Review.ItemsPerQuestion))
	p("  per product:  %s", countList(m.Review.ItemsPerProduct))
	p("  per release:  %s", countList(m.Review.ItemsPerRelease))
	for _, row := range []struct {
		name string
		s    DecisionStats
	}{{"individual", m.Review.Individual}, {"batch", m.Review.Batch}} {
		s := row.s
		p("  %-10s %3d decisions: accept %d, correct %d (+%d prose edits), reject %d, need-evidence %d, defer %d; median %s, p90 %s%s",
			row.name, s.Decisions, s.Accept, s.Correct, s.ProseEdits, s.Reject, s.NeedMoreEvidence, s.Defer, dur(s.MedianDuration), dur(s.P90Duration),
			map[bool]string{true: fmt.Sprintf(", %d batches", s.Batches), false: ""}[row.name == "batch"])
	}
	p("")
	p("Agreement across distinct models:")
	for _, a := range m.Agreement {
		p("  %-14s %-16s %3d candidates, agreement %s", a.Aspect, a.Task, a.Candidates, pct(a.Agreement))
		keys := make([]string, 0, len(a.Pairwise))
		for k := range a.Pairwise {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p("      %s  %s", k, pct(a.Pairwise[k]))
		}
	}
	p("")
	p("Per model (decisions vs proposals; FP/FN vs final facts):")
	for _, mm := range m.Models {
		p("  %s/%s: %d proposals, as-is %d, corrected %d, rejected %d, insufficient %d, abstained %d, grounded %s",
			mm.Provider, mm.Model, mm.Proposals, mm.AcceptedAsIs, mm.AcceptedCorrected, mm.Rejected, mm.InsufficientEvidence, mm.Abstentions, pct(mm.GroundedCitations))
		for _, x := range domain.Aspects {
			if mm.FalsePositive[x] > 0 || mm.FalseNegative[x] > 0 {
				p("      %-14s FP %d  FN %d", x, mm.FalsePositive[x], mm.FalseNegative[x])
			}
		}
		tasks := make([]string, 0, len(mm.ByTask))
		for t := range mm.ByTask {
			tasks = append(tasks, string(t))
		}
		sort.Strings(tasks)
		for _, t := range tasks {
			tm := mm.ByTask[domain.ProposalTask(t)]
			p("      task %-16s %d proposals, as-is %d, corrected %d, rejected %d", t, tm.Proposals, tm.AcceptedAsIs, tm.AcceptedCorrected, tm.Rejected)
		}
	}
}

func countList[K ~string](m map[K]int) string {
	if len(m) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, m[K(k)]))
	}
	return strings.Join(parts, ", ")
}
