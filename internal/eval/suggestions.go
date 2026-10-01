package eval

import "github.com/tdavison784/release-intelligence/internal/domain"

// Scoring of enriched runs (G10): when a run attaches the AI layer's
// applicability suggestions (suggestedClassification on UNKNOWN findings —
// always review-required, never overwriting the deterministic verdict), the
// dataset scores the suggestions against the labels the fixtures state.
//
//	suggestionPrecision — of the labelled suggestions, how many point the
//	                     operator where the ground truth agrees
//	                     (label review-required or unknown);
//	suggestionRecall    — of the findings whose correct answer IS unknown,
//	                     how many carry a suggestion.
//
// A suggestion on an unlabelled finding is counted (visible) but scores
// nothing: the dataset claims no knowledge it does not have.

// SuggestionMetrics is the per-entry enriched-run score.
type SuggestionMetrics struct {
	// Suggestions counts findings carrying a suggestedClassification.
	Suggestions int `json:"suggestions"`
	// Labelled suggestions (a dataset label exists for their change).
	Labelled int `json:"labelled"`
	// LabeledCorrect: label is review-required or unknown — the suggestion
	// direction matches the ground truth.
	LabeledCorrect int `json:"labeledCorrect"`
	// LabeledWrong: label is action-required or informational — the
	// suggestion escalates a decidable finding into a look, or mislabels it.
	LabeledWrong int `json:"labeledWrong"`
	// Unlabeled suggestions (no dataset label; audit only).
	Unlabeled int `json:"unlabeled"`
	// UnknownLabeled counts labelled findings whose correct class IS unknown
	// (genuine suggestion opportunities); UnknownSuggested how many of them
	// carry a suggestion (recall).
	UnknownLabeled   int `json:"unknownLabeled"`
	UnknownSuggested int `json:"unknownSuggested"`
}

// Precision is labelledCorrect / labelled.
func (m SuggestionMetrics) Precision() float64 {
	if m.Labelled == 0 {
		return 0
	}
	return float64(m.LabeledCorrect) / float64(m.Labelled)
}

// Recall is unknownSuggested / unknownLabeled (0 when the fixture states no
// unknown-expected findings).
func (m SuggestionMetrics) Recall() float64 {
	if m.UnknownLabeled == 0 {
		return 0
	}
	return float64(m.UnknownSuggested) / float64(m.UnknownLabeled)
}

// Add pools two runs' suggestion metrics.
func (m *SuggestionMetrics) Add(o SuggestionMetrics) {
	m.Suggestions += o.Suggestions
	m.Labelled += o.Labelled
	m.LabeledCorrect += o.LabeledCorrect
	m.LabeledWrong += o.LabeledWrong
	m.Unlabeled += o.Unlabeled
	m.UnknownLabeled += o.UnknownLabeled
	m.UnknownSuggested += o.UnknownSuggested
}

// ActionFindingRecord is the audit record of one action-required finding's
// wrongness judgement, so a later adjudication pass (human verdicts are data
// the scorer reads after scoring) can adjust the false-action count.
type ActionFindingRecord struct {
	FindingID string
	ChangeID  string
	Wrong     bool
}

// applyAdjudications folds a case's human verdicts into a scored result: the
// adjudicated changes move out of their dataset bucket into the human bucket
// (or out of both, for uncertain), and action findings joined to
// adjudicated-false changes become false actions.
func applyAdjudications(res *EntryResult, f *AdjudicationFile) {
	if f == nil || len(f.Adjudications) == 0 {
		return
	}
	matched := map[string]bool{}
	for i := range res.Matches {
		for _, h := range res.Matches[i].Hits {
			matched[h.ChangeID] = true
		}
	}
	fp := map[string]bool{}
	for _, id := range res.FPChangeIDs {
		fp[id] = true
	}
	adj := map[string]Adjudication{}
	for _, a := range f.Adjudications {
		adj[a.ChangeID] = a
	}
	stats := AdjudicationStats{}
	newlyFalse := map[string]int{} // change id → action findings newly wrong
	for _, ch := range res.AllChangeIDs {
		a, adjudicated := adj[ch]
		switch {
		case adjudicated && a.Verdict == VerdictTruePositive:
			stats.HumanTrue++
			stats.TrueIDs = append(stats.TrueIDs, ch)
		case adjudicated && a.Verdict == VerdictFalsePositive:
			stats.HumanFalse++
			stats.FalseIDs = append(stats.FalseIDs, ch)
			if !fp[ch] {
				newlyFalse[ch] = 0
			}
		case adjudicated:
			stats.Uncertain++
			stats.UncertainIDs = append(stats.UncertainIDs, ch)
		case matched[ch]:
			stats.DatasetTrue++
		case fp[ch]:
			stats.DatasetFalse++
		}
	}
	// action findings joined to newly-false changes (that were not already
	// wrong for a structural reason) join the false-action count
	for _, r := range res.actionFindings {
		if !r.Wrong {
			if _, ok := newlyFalse[r.ChangeID]; ok {
				newlyFalse[r.ChangeID]++
			}
		}
	}
	for _, n := range newlyFalse {
		res.Metrics.FalseActionFindings += n
	}
	res.Adjudicated = stats
}

// scoreSuggestions scores the AI layer's applicability suggestions against
// the fixture's labels (G10). It runs inside scoreReport, where the
// change→expected-item and item→relevance maps are already built; on a
// deterministic report it is a no-op.
func scoreSuggestions(res *EntryResult, c *Case, report *domain.ImpactReport, expIDForChange map[string]string, relevanceFor map[string]string) {
	var m SuggestionMetrics
	labelFor := func(changeID string) string {
		if expID, ok := expIDForChange[changeID]; ok {
			return classForRelevance(relevanceFor[expID])
		}
		return ""
	}
	for _, f := range report.Findings {
		if f.SuggestedClassification == "" {
			if cls := labelFor(f.ChangeID); cls != "" && cls == ClassUnknown {
				// a genuine opportunity the enricher did not take still
				// counts against recall
				m.UnknownLabeled++
			}
			continue
		}
		m.Suggestions++
		label := labelFor(f.ChangeID)
		switch label {
		case "":
			m.Unlabeled++
		case ClassReviewRequired, ClassUnknown:
			m.Labelled++
			m.LabeledCorrect++
			if label == ClassUnknown {
				m.UnknownLabeled++
				m.UnknownSuggested++
			}
		default:
			m.Labelled++
			m.LabeledWrong++
		}
	}
	if m.Suggestions > 0 || m.UnknownLabeled > 0 {
		res.Suggestions = &m
	}
}
