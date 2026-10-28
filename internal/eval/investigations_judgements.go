package eval

import (
	"fmt"
	"io"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// JudgementRow is one case's upgrade judgement (Milestone 3) in the ungated
// side table: what the judgement recommends and whether its required parts
// are present (PLAN-M3 §2, PD-1). It reads no expectation and feeds no gate;
// the scored M3 check is the commander's.
type JudgementRow struct {
	Case           string                 `json:"case"`
	Status         domain.JudgementStatus `json:"status,omitempty"` // "" = no judgement
	Recommendation domain.Recommendation  `json:"recommendation,omitempty"`
	Proposed       string                 `json:"proposed,omitempty"`
	Chosen         string                 `json:"chosen,omitempty"`
	Minimum        string                 `json:"minimum,omitempty"`
	Recommended    string                 `json:"recommended,omitempty"`
	Changes        int                    `json:"changes"`
	Located        int                    `json:"located"` // changes with a deterministic location
	Validation     int                    `json:"validation"`
	ResidualRisks  int                    `json:"residualRisks"`
	Findings       int                    `json:"findings"`
	Hard           int                    `json:"hard"`
	Interactions   int                    `json:"interactions"`
	// Missing names the required parts the judgement lacks.
	Missing []string `json:"missing,omitempty"`
}

// ScoreJudgements builds the side table over the cases' runs, in case order.
// Nil when no run carries a judgement.
func ScoreJudgements(cases []*InvCase, runs map[string]*InvRun) []JudgementRow {
	var rows []JudgementRow
	have := false
	for _, c := range cases {
		r := runs[c.ID]
		row := JudgementRow{Case: c.ID}
		if r == nil || r.Investigation == nil || r.Investigation.Judgement == nil {
			row.Missing = []string{"judgement"}
			rows = append(rows, row)
			continue
		}
		have = true
		j := r.Investigation.Judgement
		t := j.Target
		row.Status, row.Recommendation, row.Proposed = j.Status, j.Recommendation, t.Proposed.String()
		row.Chosen, row.Minimum, row.Recommended = versionText(t.Chosen), versionText(t.Minimum), versionText(t.Recommended)
		row.Changes, row.Validation, row.ResidualRisks = len(j.Changes), len(j.Validation), len(j.ResidualRisks)
		row.Findings, row.Hard, row.Interactions = len(j.Findings), len(j.HardFindings()), len(j.Interactions)
		for _, ch := range j.Changes {
			if !ch.Proposed {
				row.Located++
			}
		}
		miss := func(cond bool, part string) {
			if cond {
				row.Missing = append(row.Missing, part)
			}
		}
		if j.Status == domain.JudgementFailed {
			miss(true, "recommendation (synthesis failed)")
		} else {
			miss(strings.TrimSpace(j.Headline) == "", "headline")
			miss(len(j.Changes) == 0 && j.Recommendation != domain.RecommendDoNotProceed, "changes")
			miss(len(j.Validation) == 0 && j.Recommendation != domain.RecommendDoNotProceed, "validation")
			miss(len(j.Summary.Verified)+len(j.Summary.Inferred)+len(j.Summary.Unknown) == 0, "verified/inferred/unknown")
			miss(t.Changed && t.Chosen == nil, "chosen target")
		}
		miss(t.Rule == "", "target rule")
		miss(j.Backout.Statement == "", "backout")
		rows = append(rows, row)
	}
	if !have {
		return nil
	}
	return rows
}

func versionText(v *domain.Version) string {
	if v == nil {
		return ""
	}
	return v.String()
}

// WriteJudgementText writes the side table (nothing when it is empty).
func WriteJudgementText(w io.Writer, rows []JudgementRow) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(w, "\n── upgrade judgements (ungated side table; advisory AI judgements, not scored here) ──\n")
	for _, r := range rows {
		if r.Status == "" {
			fmt.Fprintf(w, "  %-44s no judgement\n", r.Case)
			continue
		}
		target := r.Chosen
		if target == "" {
			target = "none"
		}
		fmt.Fprintf(w, "  %-44s %-8s %-21s target %s (proposed %s, minimum %s, rule %s)  changes %d (%d located)  tests %d  risks %d  findings %d (%d hard)  interactions %d\n",
			r.Case, r.Status, orDash(string(r.Recommendation)), target, r.Proposed, orDash(r.Minimum), orDash(r.Recommended),
			r.Changes, r.Located, r.Validation, r.ResidualRisks, r.Findings, r.Hard, r.Interactions)
		if len(r.Missing) > 0 {
			fmt.Fprintf(w, "      missing: %s\n", strings.Join(r.Missing, ", "))
		}
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
