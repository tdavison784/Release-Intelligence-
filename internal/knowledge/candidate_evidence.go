package knowledge

import (
	"context"
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ExtendCandidateEvidence appends evidence to a stored candidate's snapshot
// (CONTRACT-CHANGE(prtext)). It is additive and idempotent: records already
// present (by content id) are skipped, existing records keep their order, and
// re-running with the same evidence writes nothing. It returns how many
// records were added. The candidate keeps its id (derived from its members),
// so proposals, review items and facts that name it stay valid; proposals
// cite evidence ids they were shown, which the growth never removes.
func ExtendCandidateEvidence(ctx context.Context, s Store, candidateID string, evs []domain.Evidence) (int, error) {
	rec, err := s.Get(ctx, candidateID)
	if err != nil {
		return 0, err
	}
	if rec.Candidate == nil {
		return 0, fmt.Errorf("knowledge: %s is not a candidate", candidateID)
	}
	c := *rec.Candidate
	have := map[domain.EvidenceID]bool{}
	for _, e := range c.Evidence {
		have[e.ID] = true
	}
	added := 0
	ev := append([]domain.Evidence(nil), c.Evidence...)
	for _, e := range evs {
		if have[e.ID] {
			continue
		}
		have[e.ID] = true
		ev = append(ev, e)
		added++
	}
	if added == 0 {
		return 0, nil
	}
	c.Evidence = ev
	next, err := domain.NewRecord(c)
	if err != nil {
		return 0, err
	}
	if err := s.Put(ctx, next); err != nil {
		return 0, err
	}
	return added, nil
}
