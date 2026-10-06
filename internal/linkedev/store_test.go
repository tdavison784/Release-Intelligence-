package linkedev

import (
	"context"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/github"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

func putRec(t *testing.T, s knowledge.Store, entity any) {
	t.Helper()
	rec, err := domain.NewRecord(entity)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func TestApplyAttachesEvidenceToStoredCandidates(t *testing.T) {
	ctx := context.Background()
	s := knowledge.NewFileStore(t.TempDir())
	note := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example/notes.md", "L1", "Change the default of rotationPolicy (#7)", "sha256:n", t0)
	anchor := domain.ChangeAnchor{Release: "1.18.0", EvidenceKeys: []string{domain.EvidenceKey(note)}, StatementKeys: []string{domain.StatementKey(note)}, ChangeIDs: []string{"chg-old"}}
	members := []domain.CandidateMember{{ChangeID: "chg-old", Anchor: &anchor}}
	cand := domain.SemanticCandidate{ID: domain.CandidateID("p", "1.18.0", members), Product: "p", Release: "1.18.0", Members: members,
		Grouping: "single", Category: domain.CategoryConfiguration, Title: "Change the default", Evidence: []domain.Evidence{note},
		Producer: "semantic.candidates@v1", CreatedAt: t0}
	putRec(t, s, cand)
	// an unrelated candidate of the same product must not be touched
	other := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example/notes.md", "L9", "Something else", "sha256:n", t0)
	oa := domain.ChangeAnchor{Release: "1.18.0", EvidenceKeys: []string{domain.EvidenceKey(other)}, StatementKeys: []string{domain.StatementKey(other)}, ChangeIDs: []string{"chg-other"}}
	om := []domain.CandidateMember{{ChangeID: "chg-other", Anchor: &oa}}
	oc := domain.SemanticCandidate{ID: domain.CandidateID("p", "1.18.0", om), Product: "p", Release: "1.18.0", Members: om,
		Grouping: "single", Category: domain.CategoryConfiguration, Title: "Something else", Evidence: []domain.Evidence{other},
		Producer: "semantic.candidates@v1", CreatedAt: t0}
	putRec(t, s, oc)
	// a review item closed as needs-evidence on the first candidate
	it := domain.ReviewItem{CandidateID: cand.ID, Product: "p", Release: "1.18.0", QuestionType: domain.QuestionDuplicate,
		Question: "q", Proposed: domain.SemanticAssertion{}, Routing: domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityLow},
		Status: domain.ReviewNeedsEvidence, CreatedAt: t0}
	it.ID = domain.ReviewItemID(it.CandidateID, it.QuestionType, it.Proposed)
	putRec(t, s, it)

	// today's edge: the change id shifted (note text edit), the anchor still matches
	edgeEv := []domain.Evidence{note}
	changes := []domain.Change{{ID: "chg-new", Evidence: []domain.EvidenceID{note.ID}, References: []domain.Reference{pr(7)}, Provenance: domain.Provenance{Method: domain.MethodDeclared}}}
	f := &fakeFetcher{items: map[string]*github.LinkedItem{"o/r#7": prItem(7, "Change the default", "Long description of the change.")}}
	res := Collect(ctx, f, changes, Options{Now: t0})

	dry, err := Apply(ctx, s, "p", changes, edgeEv, res, true)
	if err != nil {
		t.Fatal(err)
	}
	if dry.CandidatesMatched != 1 || len(dry.Gained()) != 1 || dry.Gained()[0].Added != 2 {
		t.Fatalf("dry run = %+v", dry)
	}
	if got, _ := s.Get(ctx, cand.ID); len(got.Candidate.Evidence) != 1 {
		t.Fatal("dry run wrote the store")
	}

	rep, err := Apply(ctx, s, "p", changes, edgeEv, res, false)
	if err != nil {
		t.Fatal(err)
	}
	g := rep.Gained()
	if len(g) != 1 || g[0].CandidateID != cand.ID || g[0].Added != 2 || g[0].NeedsEvidenceItems != 1 || g[0].Changes[0] != "chg-new" {
		t.Fatalf("report = %+v", rep)
	}
	got, _ := s.Get(ctx, cand.ID)
	if len(got.Candidate.Evidence) != 3 || got.Candidate.Evidence[0].ID != note.ID || got.Candidate.Evidence[1].Kind != domain.EvidenceLinkedPR {
		t.Errorf("candidate evidence = %+v", got.Candidate.Evidence)
	}
	if o, _ := s.Get(ctx, oc.ID); len(o.Candidate.Evidence) != 1 {
		t.Error("an unrelated candidate gained evidence")
	}

	// idempotent re-run (even at another fetch time): offered, but nothing added
	res2 := Collect(ctx, f, changes, Options{Now: t0.Add(time.Hour)})
	rep2, err := Apply(ctx, s, "p", changes, edgeEv, res2, false)
	if err != nil {
		t.Fatal(err)
	}
	if g2 := rep2.Gained(); len(g2) != 1 || g2[0].Added != 0 || g2[0].Offered != 2 {
		t.Errorf("re-run = %+v", rep2)
	}
	if got2, _ := s.Get(ctx, cand.ID); len(got2.Candidate.Evidence) != 3 {
		t.Error("re-run grew the candidate")
	}
	// the proposals' world is untouched: the id did not move and the item is still there
	if _, err := s.Get(ctx, it.ID); err != nil {
		t.Error(err)
	}
}
