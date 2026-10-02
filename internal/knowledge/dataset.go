package knowledge

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ExportDataset writes the human-feedback dataset (G13) as JSONL, one
// domain.FeedbackExample per decision in decision order: the decision with its
// labels, original and corrected assertions, the item shown, the candidate,
// the proposals and validations the item cites, and the fact the decision
// helped verify. Rejections, duplicates and insufficient-evidence decisions
// are exported like any other: they are the negative examples (G11).
func ExportDataset(s *Snapshot, w io.Writer) error {
	items := map[string]domain.ReviewItem{}
	for _, it := range s.ReviewItems {
		items[it.ID] = it
	}
	cands := map[string]domain.SemanticCandidate{}
	for _, c := range s.Candidates {
		cands[c.ID] = c
	}
	props := map[string]domain.SemanticProposal{}
	for _, p := range s.Proposals {
		props[p.ID] = p
	}
	vals := map[string]domain.ValidationResult{}
	for _, v := range s.Validations {
		vals[v.ID] = v
	}
	facts := map[string]domain.VerifiedFact{}
	for _, f := range s.Facts {
		facts[f.ID] = f
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, d := range s.Decisions {
		it, ok := items[d.ReviewItemID]
		if !ok {
			return fmt.Errorf("dataset: decision %s: review item %s missing", d.ID, d.ReviewItemID)
		}
		c, ok := cands[it.CandidateID]
		if !ok {
			return fmt.Errorf("dataset: item %s: candidate %s missing", it.ID, it.CandidateID)
		}
		ex := domain.FeedbackExample{Decision: d, Item: it, Candidate: c}
		for _, id := range it.Proposals {
			if p, ok := props[id]; ok {
				ex.Proposals = append(ex.Proposals, p)
			}
		}
		for _, id := range it.Validations {
			if v, ok := vals[id]; ok {
				ex.Validations = append(ex.Validations, v)
			}
		}
		if f := factOf(d, facts); f != nil {
			ex.Fact = f
		}
		if err := enc.Encode(ex); err != nil {
			return err
		}
	}
	return nil
}

// factOf finds the fact a decision verified an aspect of: the one it names,
// else any fact whose verification rests on it.
func factOf(d domain.ReviewDecision, facts map[string]domain.VerifiedFact) *domain.VerifiedFact {
	if f, ok := facts[d.ResultingFact]; ok && d.ResultingFact != "" {
		return &f
	}
	var found *domain.VerifiedFact
	for _, f := range facts {
		for _, v := range f.Verification {
			for _, b := range v.Basis {
				if b == d.ID && (found == nil || f.ID < found.ID) {
					f := f
					found = &f
				}
			}
		}
	}
	return found
}
