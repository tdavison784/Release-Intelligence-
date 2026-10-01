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
	// Diffs against stored snapshots. Compared is true when at least one
	// stored snapshot existed (a clean comparison prints an explicit "no
	// regressions" line so CI logs say what was checked).
	Diffs    []Delta `json:"diffs,omitempty"`
	Compared bool    `json:"compared,omitempty"`
}

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
	fmt.Fprintf(w, "%d entries (%d errored): expected %d, found %d, missed %d (critical %d, important %d, minor %d)\n",
		agg.Entries, agg.Errored, agg.Expected, agg.Found, agg.Expected-agg.Found,
		agg.MissedCritical, agg.MissedImportant, agg.MissedMinor)
	fmt.Fprintf(w, "recall %.2f   precision %.2f   (changes %d, matched %d, false positives %d, duplicate groups %d, unsupported %d)\n",
		agg.Recall, agg.Precision, agg.Changes, agg.MatchedChanges, agg.FalsePositives, agg.DuplicateGroups, agg.Unsupported)
	if agg.EnvEntries > 0 {
		fmt.Fprintf(w, "environment (%d entries): impact links %d/%d hit (accuracy %.2f), findings %d/%d, false findings %d\n",
			agg.EnvEntries, agg.ImpactLinksHit, agg.ImpactLinks, agg.ImpactAccuracy,
			agg.FindingsFound, agg.FindingsExpected, agg.FindingsFP)
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
