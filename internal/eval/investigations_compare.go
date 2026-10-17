package eval

// Phase 5 comparison report (docs/phase5/learning-loop/PROTOCOL.md §5): the
// same cases scored under several arms, side by side. Accuracy and
// coverage/risk-detection are separate tables; safety rows come first.

import (
	"fmt"
	"io"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ComparedRun is one stored run in a comparison. Seconds is the wall time of
// its case runs (run.json), the rest identifies the run.
type ComparedRun struct {
	Label     string
	RunID     string
	Arm       string
	Commit    string
	Knowledge string
	Seconds   int
	Score     *InvScore
}

// Clearance counts: decisions the system answered not-affected.
type Clearances struct {
	Cleared  int // system not-affected
	Correct  int // …and expected not-affected
	Unsafe   int // …and expected anything else (false-safe policy decisions)
	Eligible int // approval-eligible decisions
}

// CountClearances tallies the clearances in ds.
func CountClearances(ds []ItemDecision) Clearances {
	var c Clearances
	for _, d := range ds {
		if d.Eligible {
			c.Eligible++
		}
		if d.System != domain.FindingNotAffected || d.RunFailed {
			continue
		}
		c.Cleared++
		if d.Expected == domain.FindingNotAffected {
			c.Correct++
		} else {
			c.Unsafe++
		}
	}
	return c
}

// accuracyRows and coverageRows are the metric names of the two tables.
// Accuracy: did the class match. Coverage/risk detection: did it see the
// risk, how much was left to humans, how well is it evidenced.
var (
	accuracyRows = []string{MAccuracy, MNotAffectedConfirmation, MAffectedConfirmation, MObservedAgreement}
	coverageRows = []string{MAffectedDetection, MExposureRecall, MEvidenceCoverage, MUnresolvedRate, MHumanReviewRate, MAutoApprovalEligibilityRate}
	safetyRows   = []string{MFalseNegativeRate, MUnsafeEligibilityRate}
)

func decisionKey(d ItemDecision) string { return d.Case + "\x00" + d.Item }

// WriteInvComparison writes the report tables for runs[0] (the baseline)
// against the others, as Markdown.
func WriteInvComparison(w io.Writer, runs []ComparedRun) {
	if len(runs) == 0 {
		return
	}
	row := func(cells ...string) { fmt.Fprintf(w, "| %s |\n", strings.Join(cells, " | ")) }
	sep := func(n int) { fmt.Fprintf(w, "|%s\n", strings.Repeat("---|", n)) }
	head := func(first string) []string {
		h := []string{first}
		for _, r := range runs {
			h = append(h, r.Label)
		}
		return h
	}

	fmt.Fprintf(w, "### Runs\n\n")
	row("label", "run", "arm", "binary", "knowledge digest")
	sep(5)
	for _, r := range runs {
		row(r.Label, r.RunID, r.Arm, shortRef(r.Commit), r.Knowledge)
	}

	fmt.Fprintf(w, "\n### Policy class per item (expected → system)\n\n")
	row(append(head("case · item"), "expected")...)
	sep(len(runs) + 2)
	index := make([]map[string]ItemDecision, len(runs))
	for i, r := range runs {
		index[i] = map[string]ItemDecision{}
		for _, d := range r.Score.Decisions {
			index[i][decisionKey(d)] = d
		}
	}
	for _, d := range runs[0].Score.Decisions {
		cells := []string{fmt.Sprintf("%s · %s", d.Case, d.Item)}
		for i := range runs {
			x, ok := index[i][decisionKey(d)]
			switch {
			case !ok:
				cells = append(cells, "—")
			default:
				mark := ""
				if x.System == domain.FindingNotAffected && x.Expected != domain.FindingNotAffected {
					mark = " **UNSAFE**"
				}
				cells = append(cells, string(x.System)+" ["+x.Basis+"]"+mark)
			}
		}
		cells = append(cells, string(d.Expected))
		row(cells...)
	}

	metric := func(r ComparedRun, name string) string { return r.Score.Metrics[name].String() }
	table := func(title string, names []string) {
		fmt.Fprintf(w, "\n### %s\n\n", title)
		row(head("metric")...)
		sep(len(runs) + 1)
		for _, n := range names {
			cells := []string{n}
			for _, r := range runs {
				cells = append(cells, metric(r, n))
			}
			row(cells...)
		}
	}

	fmt.Fprintf(w, "\n### Safety\n\n")
	row(head("row")...)
	sep(len(runs) + 1)
	for _, n := range safetyRows {
		cells := []string{n}
		for _, r := range runs {
			cells = append(cells, metric(r, n))
		}
		row(cells...)
	}
	for _, spec := range []struct {
		name string
		get  func(Clearances) int
	}{{"clearances (system not-affected)", func(c Clearances) int { return c.Cleared }},
		{"…of which expected not-affected", func(c Clearances) int { return c.Correct }},
		{"unsafe clearances (not expected not-affected)", func(c Clearances) int { return c.Unsafe }},
		{"approval-eligible decisions", func(c Clearances) int { return c.Eligible }}} {
		cells := []string{spec.name}
		for _, r := range runs {
			cells = append(cells, fmt.Sprint(spec.get(CountClearances(r.Score.Decisions))))
		}
		row(cells...)
	}
	// clearances gained relative to the baseline, by item
	for i := 1; i < len(runs); i++ {
		var gained, lost []string
		for _, d := range runs[i].Score.Decisions {
			b, ok := index[0][decisionKey(d)]
			if !ok {
				continue
			}
			tag := fmt.Sprintf("%s·%s (expected %s)", d.Case, d.Item, d.Expected)
			switch {
			case d.System == domain.FindingNotAffected && b.System != domain.FindingNotAffected:
				gained = append(gained, tag)
			case d.System != domain.FindingNotAffected && b.System == domain.FindingNotAffected:
				lost = append(lost, tag)
			}
		}
		fmt.Fprintf(w, "\n%s vs %s — clearances gained: %s; clearances lost: %s\n", runs[i].Label, runs[0].Label, listOrNone(gained), listOrNone(lost))
	}

	table("Accuracy (class equals expected)", accuracyRows)
	table("Coverage and risk detection (reported separately from accuracy)", coverageRows)

	fmt.Fprintf(w, "\n### Human review and cost\n\n")
	row(head("measure")...)
	sep(len(runs) + 1)
	for _, spec := range []struct {
		name string
		get  func(ComparedRun) int
	}{
		{"item decisions needing human review", func(r ComparedRun) int {
			n := 0
			for _, d := range r.Score.Decisions {
				if d.ReviewRequired {
					n++
				}
			}
			return n
		}},
		{"findings requiring human review (all items)", func(r ComparedRun) int { return sumRuns(r, func(x RunScore) int { return x.ReviewFindings }) }},
		{"agent tokens", func(r ComparedRun) int { return sumRuns(r, func(x RunScore) int { return x.AgentTokens }) }},
		{"hypotheses proposed", func(r ComparedRun) int { return sumRuns(r, func(x RunScore) int { return x.Hypotheses }) }},
		{"wall seconds (all cases)", func(r ComparedRun) int { return r.Seconds }},
	} {
		cells := []string{spec.name}
		for _, r := range runs {
			cells = append(cells, fmt.Sprint(spec.get(r)))
		}
		row(cells...)
	}

	if hasVerdicts(runs) {
		fmt.Fprintf(w, "\n### Agent verdicts (ungated; never an input to policy)\n\n")
		row(head("outcome")...)
		sep(len(runs) + 1)
		for _, o := range VerdictOutcomes {
			cells := []string{o}
			for _, r := range runs {
				cells = append(cells, fmt.Sprint(VerdictCounts(r.Score.Verdicts)[o]))
			}
			row(cells...)
		}
	}
}

func hasVerdicts(runs []ComparedRun) bool {
	for _, r := range runs {
		if len(r.Score.Verdicts) > 0 {
			return true
		}
	}
	return false
}

func sumRuns(r ComparedRun, f func(RunScore) int) int {
	n := 0
	for _, x := range r.Score.Runs {
		n += f(x)
	}
	return n
}

func listOrNone(xs []string) string {
	if len(xs) == 0 {
		return "none"
	}
	return strings.Join(xs, "; ")
}

func shortRef(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}
