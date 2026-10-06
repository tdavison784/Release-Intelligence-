package linkedev

import (
	"context"
	"sort"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// CandidateLink is the outcome for one stored candidate.
type CandidateLink struct {
	CandidateID string
	Release     string
	// Changes are the edge changes the candidate's members resolved to that
	// carry fetched evidence.
	Changes []string
	// Evidence is how many records the candidate gained (0 on a re-run).
	Added int
	// Offered is how many distinct records the links offered it.
	Offered int
	// NeedsEvidenceItems counts the candidate's review items whose status is
	// needs-evidence (status only; decisions are not read).
	NeedsEvidenceItems int
}

// StoreReport summarises one edge's application to the store.
type StoreReport struct {
	CandidatesMatched int // stored candidates with at least one member resolved to an edge change
	Candidates        []CandidateLink
}

// Gained returns the candidates that have linked evidence on offer (their
// snapshot holds it after Apply, whether added now or by an earlier run).
func (r StoreReport) Gained() []CandidateLink {
	var out []CandidateLink
	for _, c := range r.Candidates {
		if c.Offered > 0 {
			out = append(out, c)
		}
	}
	return out
}

// Apply adds the evidence of res to the stored candidates of the product that
// the edge's changes belong to. A candidate member is matched to a change by
// id, falling back to the member's statement anchor (change ids shift with
// note text). With dryRun the store is read but not written. The operation is
// additive and idempotent (knowledge.ExtendCandidateEvidence).
func Apply(ctx context.Context, store knowledge.Store, product domain.ProductID, changes []domain.Change, edgeEvidence []domain.Evidence, res *Result, dryRun bool) (StoreReport, error) {
	var rep StoreReport
	snap, err := store.Load(ctx, knowledge.Query{Product: product, Kinds: []domain.RecordKind{domain.RecordCandidate, domain.RecordReviewItem}})
	if err != nil {
		return rep, err
	}
	needs := map[string]int{}
	for _, it := range snap.ReviewItems {
		if it.Status == domain.ReviewNeedsEvidence {
			needs[it.CandidateID]++
		}
	}
	byID := map[string]domain.Change{}
	for _, c := range changes {
		byID[c.ID] = c
	}
	evByID := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range edgeEvidence {
		evByID[e.ID] = e
	}
	lookup := func(id domain.EvidenceID) (domain.Evidence, bool) { e, ok := evByID[id]; return e, ok }
	built := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range res.Evidence {
		built[e.ID] = e
	}
	perChange := res.ByChange()

	for _, cand := range snap.Candidates {
		cl := CandidateLink{CandidateID: cand.ID, Release: cand.Release, NeedsEvidenceItems: needs[cand.ID]}
		matched := false
		var offered []domain.Evidence
		seen := map[domain.EvidenceID]bool{}
		for _, m := range cand.Members {
			ch, ok := byID[m.ChangeID]
			if !ok && m.Anchor != nil {
				for _, c := range changes {
					if m.Anchor.Matches(c, lookup) {
						ch, ok = c, true
						break
					}
				}
			}
			if !ok {
				continue
			}
			matched = true
			if ids := perChange[ch.ID]; len(ids) > 0 {
				cl.Changes = append(cl.Changes, ch.ID)
				for _, id := range ids {
					if !seen[id] {
						seen[id] = true
						offered = append(offered, built[id])
					}
				}
			}
		}
		if !matched {
			continue
		}
		rep.CandidatesMatched++
		cl.Offered = len(offered)
		if len(offered) > 0 {
			sort.Strings(cl.Changes)
			if !dryRun {
				n, err := knowledge.ExtendCandidateEvidence(ctx, store, cand.ID, offered)
				if err != nil {
					return rep, err
				}
				cl.Added = n
			} else {
				have := map[domain.EvidenceID]bool{}
				for _, e := range cand.Evidence {
					have[e.ID] = true
				}
				for _, e := range offered {
					if !have[e.ID] {
						cl.Added++
					}
				}
			}
		}
		rep.Candidates = append(rep.Candidates, cl)
	}
	return rep, nil
}
