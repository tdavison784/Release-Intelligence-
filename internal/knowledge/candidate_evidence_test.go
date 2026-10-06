package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func linkedEv(n string) domain.Evidence {
	return domain.NewEvidence(domain.EvidenceLinkedPR, "linkedev@v1", "https://github.com/o/r/pull/"+n, "title and description",
		"PR: change "+n, "sha256:"+n, t0)
}

func TestExtendCandidateEvidence(t *testing.T) {
	ctx := context.Background()
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	mustPut(t, s, c)
	before, _ := s.Get(ctx, c.ID)

	n, err := ExtendCandidateEvidence(ctx, s, c.ID, []domain.Evidence{linkedEv("1"), linkedEv("2")})
	if err != nil || n != 2 {
		t.Fatalf("added %d, err %v", n, err)
	}
	got, _ := s.Get(ctx, c.ID)
	if len(got.Candidate.Evidence) != len(c.Evidence)+2 {
		t.Fatalf("evidence = %d", len(got.Candidate.Evidence))
	}
	for i, e := range before.Candidate.Evidence { // the stored records keep their order
		if got.Candidate.Evidence[i].ID != e.ID {
			t.Errorf("record %d moved", i)
		}
	}
	if got.Candidate.ID != c.ID {
		t.Error("the candidate id must not move")
	}

	// idempotent: the same evidence (even re-fetched later) adds nothing and writes nothing
	again := linkedEv("1")
	again.RetrievedAt = again.RetrievedAt.Add(72 * 3600 * 1e9)
	if n, err := ExtendCandidateEvidence(ctx, s, c.ID, []domain.Evidence{again, linkedEv("2")}); err != nil || n != 0 {
		t.Errorf("re-run added %d, err %v", n, err)
	}
	// ... and one new record is appended after the others
	if n, _ := ExtendCandidateEvidence(ctx, s, c.ID, []domain.Evidence{linkedEv("3")}); n != 1 {
		t.Errorf("third added %d", n)
	}

	// other mutations of a candidate stay refused (title, shrinking, reordering, dropping)
	cur, _ := s.Get(ctx, c.ID)
	for name, mut := range map[string]func(*domain.SemanticCandidate){
		"title":   func(x *domain.SemanticCandidate) { x.Title = "something else" },
		"drop":    func(x *domain.SemanticCandidate) { x.Evidence = x.Evidence[:len(x.Evidence)-1] },
		"reorder": func(x *domain.SemanticCandidate) { x.Evidence[0], x.Evidence[1] = x.Evidence[1], x.Evidence[0] },
		"replace": func(x *domain.SemanticCandidate) { x.Evidence[0] = linkedEv("9") },
	} {
		cp := *cur.Candidate
		cp.Evidence = append([]domain.Evidence(nil), cp.Evidence...)
		mut(&cp)
		rec, err := domain.NewRecord(cp)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Put(ctx, rec); !errors.Is(err, ErrConflict) {
			t.Errorf("%s: Put = %v, want ErrConflict", name, err)
		}
	}

	// environment evidence never enters a candidate (release-level knowledge)
	local := domain.NewEvidence(domain.EvidenceLocalFile, "", "values.yaml", "L1", "x", "d", t0)
	if _, err := ExtendCandidateEvidence(ctx, s, c.ID, []domain.Evidence{local}); err == nil {
		t.Error("environment evidence accepted")
	}
	if _, err := ExtendCandidateEvidence(ctx, s, "sc-missing", []domain.Evidence{linkedEv("1")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown candidate: %v", err)
	}
}
