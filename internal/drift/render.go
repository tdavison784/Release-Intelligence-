package drift

import (
	"fmt"
	"io"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// RenderText prints the human-readable drift report. The layout mirrors
// `ri check`: status lines for the versions sources, then the events grouped
// by status, most severe first. Unverifiable events get their own section so
// "cannot check" is never mistaken for "changed".
func RenderText(w io.Writer, rep *Report) error {
	baseline := rep.Baseline.Source
	switch baseline {
	case BaselineSavedCheck:
		baseline = "saved check report"
		if rep.Baseline.Path != "" {
			baseline += " " + rep.Baseline.Path
		}
	case BaselineDefinition:
		baseline = "validatedAgainst lists in the definition"
	default:
		baseline = "none (no saved check report, no validatedAgainst)"
	}
	cutoff := rep.Baseline.Cutoff
	if cutoff == "" {
		cutoff = "(no baseline)"
	}
	fmt.Fprintf(w, "%s: drift check — newest releases vs the baseline\n", rep.Product)
	fmt.Fprintf(w, "Baseline: %s, cutoff %s\n", baseline, cutoff)
	switch rep.Baseline.DigestState {
	case DigestStale:
		fmt.Fprintf(w, "  ! stale baseline: the definition changed since this report was saved (baseline %s, current %s); "+
			"it describes an older definition. Re-run `ri check -o json` and replace it.\n",
			shortDigest(rep.Baseline.DefinitionDigest), shortDigest(rep.DefinitionDigest))
	case DigestUnrecorded:
		fmt.Fprintf(w, "  · baseline records no definition digest: whether it still matches the definition is unknown.\n")
	}
	if len(rep.Checked) == 0 {
		fmt.Fprintf(w, "Checked: no releases newer than the baseline; nothing to re-validate.\n")
	} else {
		fmt.Fprintf(w, "Checked: %s\n", strings.Join(rep.Checked, ", "))
	}
	if len(rep.VersionsSources) > 0 {
		fmt.Fprintf(w, "\nVersion sources:\n")
		for _, s := range rep.VersionsSources {
			fmt.Fprintf(w, "  %s %-24s %-14s %s\n", stateMark(s.State), s.SourceID, s.Kind, oneLine(s.Detail))
		}
	}

	sections := []struct {
		status string
		header string
		mark   string
	}{
		{StatusDrift, "Drift — reachable channels disagree with the definition:", "✗"},
		{StatusUnverifiable, "Unverifiable — could NOT be checked (unreachable); not drift:", "?"},
		{StatusNote, "Notes — observations that are not new (e.g. already failing at baseline):", "·"},
	}
	for _, sec := range sections {
		var events []Event
		for _, ev := range rep.Events {
			if ev.Status == sec.status {
				events = append(events, ev)
			}
		}
		if len(events) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s (%d)\n", sec.header, len(events))
		for _, ev := range events {
			fmt.Fprintf(w, "  %s [%s] %s %s\n", sec.mark, ev.Severity, ev.Kind, ev.Subject)
			fmt.Fprintf(w, "      %s\n", ev.Explanation)
			if len(ev.Releases) > 0 {
				fmt.Fprintf(w, "      releases: %s\n", strings.Join(ev.Releases, ", "))
			}
			if ev.Detail != "" {
				fmt.Fprintf(w, "      detail: %s\n", oneLine(ev.Detail))
			}
			if ev.Baseline != "" {
				fmt.Fprintf(w, "      baseline: %s\n", ev.Baseline)
			}
			if len(ev.Evidence) > 0 {
				ids := make([]string, len(ev.Evidence))
				for i, id := range ev.Evidence {
					ids[i] = string(id)
				}
				fmt.Fprintf(w, "      evidence: %s\n", strings.Join(ids, ", "))
			}
			if ev.Proposal != nil {
				fmt.Fprintf(w, "      proposal: %s\n", ev.Proposal.Action)
			}
		}
	}
	if len(rep.Events) == 0 {
		fmt.Fprintf(w, "\nNo drift: every declared relationship held on the checked releases.\n")
	}
	fmt.Fprintf(w, "\nSummary: %d drift · %d unverifiable · %d notes (%d releases checked, %d evidence records)\n",
		rep.Summary.Drift, rep.Summary.Unverifiable, rep.Summary.Notes, rep.Summary.Checked, len(rep.Evidence))
	return nil
}

func stateMark(s domain.SourceState) string {
	switch s {
	case domain.SourceOK, domain.SourcePartial:
		return "✓"
	case domain.SourceSkipped:
		return "·"
	default:
		return "✗"
	}
}

// oneLine collapses a detail string onto one line, bounded.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		return s[:157] + "…"
	}
	return s
}

func shortDigest(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		d = d[:12]
	}
	return d
}
