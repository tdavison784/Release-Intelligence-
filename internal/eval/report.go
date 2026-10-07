package eval

// Rendering of eval results: text for humans (`ri eval`), JSON for machines.
// The text output must let a reviewer audit every hit, miss, false positive
// and duplicate without re-running anything.

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Report is the machine-readable shape of a whole `ri eval` run.
type Report struct {
	Results   []EntryResult `json:"results"`
	Aggregate Aggregate     `json:"aggregate"`
	// Gates is the hard-gate panel (G11); nil when no gates were evaluated.
	Gates []GateResult `json:"gates,omitempty"`
	// Diffs against stored snapshots. Compared is true when at least one
	// stored snapshot existed (a clean comparison prints an explicit "no
	// regressions" line so CI logs say what was checked).
	Diffs    []Delta `json:"diffs,omitempty"`
	Compared bool    `json:"compared,omitempty"`
	// Levels is the per-verification-level panel of `ri eval -knowledge`
	// (DESIGN.md §7); absent without -knowledge.
	Levels []LevelReport `json:"levels,omitempty"`
	// KnowledgeWarnings lists knowledge records refused while loading.
	KnowledgeWarnings []string `json:"knowledgeWarnings,omitempty"`
}

// HasGateFailure reports whether any hard gate failed.
func (r Report) HasGateFailure() bool { return HasGateFailure(r.Gates) }

// JSON renders the report as indented JSON.
func (r Report) JSON() ([]byte, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// RenderText writes the per-entry results, the aggregate and (when present)
// the regression diff. Deterministic: no timestamps, no wall clock.
func RenderText(w io.Writer, rep Report) error {
	for i := range rep.Results {
		if err := renderEntry(w, &rep.Results[i]); err != nil {
			return err
		}
	}
	agg := rep.Aggregate
	fmt.Fprintf(w, "── aggregate ─────────────────────────────────────────────\n")
	fmt.Fprintf(w, "%d entries (%d pipeline failures): expected %d, found %d, missed %d (critical %d, important %d, minor %d)\n",
		agg.Entries, agg.PipelineFailures, agg.Expected, agg.Found, agg.Expected-agg.Found,
		agg.MissedCritical, agg.MissedImportant, agg.MissedMinor)
	fmt.Fprintf(w, "recall %.2f (critical %.2f, important %.2f)   raw precision %.2f   labeled precision %.2f (true %d / false %d, adjudicated)\n",
		agg.Recall, agg.CriticalRecall, agg.ImportantRecall, agg.Precision, agg.LabeledPrecision, agg.AdjudicatedTrue, agg.AdjudicatedFalse)
	fmt.Fprintf(w, "(changes %d, matched %d, false positives %d, duplicate rate %.2f (%d groups), unsupported %d, evidence coverage %.2f)\n",
		agg.Changes, agg.MatchedChanges, agg.FalsePositives, agg.DuplicateRate, agg.DuplicateGroups, agg.Unsupported, agg.EvidenceCoverage)
	fmt.Fprintf(w, "classification accuracy %.2f (%d scored)   false-action rate %.2f (%d/%d ACTION findings)   unknown rate %.2f   applicability accuracy %.2f\n",
		agg.ClassificationAccuracy, agg.ClassificationScored, agg.FalseActionRate, agg.FalseActionFindings, agg.ActionFindings,
		agg.UnknownRate, agg.ApplicabilityAccuracy)
	if agg.Suggestions != nil {
		fmt.Fprintf(w, "suggestions: %d (%d labelled: %d correct, %d wrong, %d unlabelled) precision %.2f, recall %.2f (unknown-labelled %d)\n",
			agg.Suggestions.Suggestions, agg.Suggestions.Labelled, agg.Suggestions.LabeledCorrect, agg.Suggestions.LabeledWrong,
			agg.Suggestions.Unlabeled, agg.SuggestionPrecision, agg.SuggestionRecall, agg.Suggestions.UnknownLabeled)
	}
	if agg.EnvEntries > 0 {
		fmt.Fprintf(w, "environment (%d entries): impact links %d/%d hit (accuracy %.2f), findings %d/%d, false findings %d\n",
			agg.EnvEntries, agg.ImpactLinksHit, agg.ImpactLinks, agg.ImpactAccuracy,
			agg.FindingsFound, agg.FindingsExpected, agg.FindingsFP)
		if agg.UndecidedLinks > 0 {
			fmt.Fprintf(w, "unknown honesty %.2f (%d/%d undecided links answered UNKNOWN or not at all; reported, not gated) — read next to applicability accuracy %.2f and affected links hit %d/%d: %s\n",
				agg.UnknownHonesty, agg.UndecidedHonest, agg.UndecidedLinks, agg.ApplicabilityAccuracy, agg.ImpactLinksHit, agg.ImpactLinks,
				honestyNote(agg.ImpactLinksHit, agg.ImpactLinks))
		}
	}
	if agg.Confusion != nil && agg.Confusion.Labelled > 0 {
		renderConfusion(w, agg.Confusion)
	}
	RenderLevels(w, rep.Levels)
	for _, kw := range rep.KnowledgeWarnings {
		fmt.Fprintf(w, "knowledge: %s\n", kw)
	}
	if len(rep.Gates) > 0 {
		if err := RenderGates(w, rep.Gates); err != nil {
			return err
		}
	}
	if len(rep.Diffs) > 0 || rep.Compared {
		fmt.Fprintf(w, "── vs stored results ─────────────────────────────────────\n")
		for _, d := range rep.Diffs {
			mark := "·"
			switch {
			case d.Regression:
				mark = "✗"
			case d.Improvement:
				mark = "✓"
			}
			fmt.Fprintf(w, "%s %s", mark, d)
			if d.Detail != "" {
				fmt.Fprintf(w, "  [%s]", d.Detail)
			}
			fmt.Fprintln(w)
		}
		if HasRegression(rep.Diffs) {
			fmt.Fprintf(w, "REGRESSION against eval/results (review, then `ri eval -update`)\n")
		} else {
			fmt.Fprintf(w, "no regressions against eval/results\n")
		}
	}
	return nil
}

func renderEntry(w io.Writer, r *EntryResult) error {
	fmt.Fprintf(w, "── %s ─ %s %s → %s\n", r.CaseID, r.Product, r.From, r.To)
	if r.Error != "" {
		fmt.Fprintf(w, "  ✗ pipeline error: %s\n", r.Error)
	}
	m := r.Metrics
	fmt.Fprintf(w, "  expected %d, found %d, missed %d (critical %d, important %d, minor %d) — recall %.2f, precision %.2f\n",
		m.Expected, m.Found, m.Missed(), m.MissedCritical, m.MissedImportant, m.MissedMinor, m.Recall(), m.Precision())
	fmt.Fprintf(w, "  changes %d, matched %d (coverage %.2f), false positives %d, duplicate groups %d, unsupported %d\n",
		m.Changes, m.MatchedChanges, m.Coverage(), m.FalsePositives, m.DuplicateGroups, m.Unsupported)
	for i := range r.Matches {
		a := &r.Matches[i]
		if a.Found {
			fmt.Fprintf(w, "  ✓ %s (%s/%s) %s\n", a.ExpectedID, a.Importance, a.Kind, clip(a.Title, 90))
			for _, h := range a.Hits {
				fmt.Fprintf(w, "      ← %s %q [%s]", h.ChangeID, clip(h.ChangeTitle, 70), h.MatchedBy)
				if h.Evidence != "" {
					fmt.Fprintf(w, " %s", clip(h.Evidence, 80))
				}
				fmt.Fprintln(w)
			}
		} else {
			fmt.Fprintf(w, "  ✗ %s (%s/%s) %s — MISSED\n", a.ExpectedID, a.Importance, a.Kind, clip(a.Title, 90))
		}
	}
	for _, fp := range r.FalsePos {
		fmt.Fprintf(w, "  ✗ FP %s — %s\n", strings.Join(fp.ChangeIDs, ", "), clip(fp.Title, 90))
	}
	for _, d := range r.Duplicates {
		fmt.Fprintf(w, "  ≈ dup %s: %s\n", clip(d.Key, 72), strings.Join(d.ChangeIDs, ", "))
	}
	for _, u := range r.UnsupportedConclusions {
		fmt.Fprintf(w, "  ? unsupported %s %s: %s\n", u.Kind, u.ID, u.Reason)
	}
	if r.Env != nil {
		e := r.Env
		fmt.Fprintf(w, "  environment: links %d hit %d (accuracy %.2f), findings %d/%d expected, false findings %d (%d total)\n",
			e.ImpactLinks, e.ImpactLinksHit, e.ImpactAccuracy(), e.FindingsFound, e.FindingsExpected, e.FindingsFP, e.Findings)
		for _, a := range r.EnvImpact {
			mark := "✓"
			if !a.Hit {
				mark = "✗"
			}
			if a.Relevance == RelevanceNotAffected && a.Hit {
				mark = "!"
			}
			fmt.Fprintf(w, "    %s %s → %s (%s)%s\n", mark, a.ExpectedID, a.Relevance, strings.Join(a.FindingIDs, ", "), clip(a.Why, 80))
		}
		for _, a := range r.EnvFindings {
			mark := "✓"
			if !a.Found {
				mark = "✗"
			}
			fmt.Fprintf(w, "    %s finding %s [%s] %s\n", mark, firstNonEmpty(a.ID, "-"), a.Matcher, strings.Join(a.FindingIDs, ", "))
		}
		for _, fp := range r.EnvFalsePos {
			fmt.Fprintf(w, "    ✗ FP finding %s — %s\n", strings.Join(fp.ChangeIDs, ", "), clip(fp.Title, 90))
		}
	}
	return nil
}

func renderConfusion(w io.Writer, m *ConfusionMatrix) {
	fmt.Fprintf(w, "confusion matrix (expected rows × actual columns, %d labelled findings; severity weighting per docs/ACTION_CLASSIFICATION.md):\n", m.Labelled)
	short := map[string]string{
		ClassActionRequired: "ACTION", ClassReviewRequired: "REVIEW", ClassInformational: "INFO",
		ClassNotAffected: "NOTAFF", ClassUnknown: "UNKNOWN",
	}
	fmt.Fprintf(w, "  %-17s", "expected\\actual")
	for _, c := range m.Classes {
		fmt.Fprintf(w, " %7s", short[c])
	}
	fmt.Fprintln(w)
	for i, row := range m.Rows {
		fmt.Fprintf(w, "  %-17s", short[m.Classes[i]])
		for _, n := range row {
			fmt.Fprintf(w, " %7d", n)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "  weighted miss %.1f (ACTION→NOTAFF ×10, ACTION→UNKNOWN/INFO ×5, NOTAFF→ACTION ×5, ACTION→REVIEW ×1, …)\n", m.WeightedMiss)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// honestyNote qualifies unknown honesty: it is vacuous when the engine
// decides nothing (an engine that emits nothing scores 1.0), so the note says
// whether anything was decided at all.
func honestyNote(hit, links int) string {
	if hit == 0 {
		return fmt.Sprintf("vacuous — no affected link was decided (0/%d hit), so honesty here proves nothing", links)
	}
	return "honesty counts only alongside decided links"
}
