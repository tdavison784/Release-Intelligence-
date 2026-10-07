package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// failAfterStore fails every Put after the first n (a disk error mid-write).
type failAfterStore struct {
	Store
	n int
}

func (f *failAfterStore) Put(ctx context.Context, rec domain.KnowledgeRecord) error {
	if f.n <= 0 {
		return errors.New("disk full")
	}
	f.n--
	return f.Store.Put(ctx, rec)
}

func batchOfTwo(t *testing.T) (Store, []domain.ReviewItem, []domain.ReviewDecision) {
	t.Helper()
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	mustPut(t, s, c)
	var items []domain.ReviewItem
	for _, a := range []domain.QuestionType{domain.QuestionSemanticMapping, domain.QuestionApplicability} {
		full := rotationAssertion(domain.ConsequenceBehaviorChange)
		prop := domain.SemanticAssertion{}
		for _, x := range domain.QuestionAspects(a) {
			mergeAspect(&prop, partOf(full, x), x)
		}
		it := newItem(c, a, prop, domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal}, nil, nil, t0)
		mustPut(t, s, it)
		items = append(items, it)
	}
	at := t0.Add(time.Hour)
	ids := []string{items[0].ID, items[1].ID}
	var ds []domain.ReviewDecision
	for _, it := range items {
		d := decision(it, "e", domain.ReviewerHuman, domain.ActionAccept, at)
		d.BatchID, d.BatchSize = domain.BatchID(ids, "e", at), 2
		ds = append(ds, d)
	}
	return s, items, ds
}

// CONTRACT-CHANGE(dashboard): the review UI's bulk action relies on Decide
// being all-or-nothing. Invalid input anywhere in the call writes nothing; a
// store failure mid-write cannot be rolled back, but a retry of the same
// decisions completes it exactly once (decisions are content-addressed).
func TestDecideRefusedBatchWritesNothing(t *testing.T) {
	s, items, ds := batchOfTwo(t)
	q := NewQueue(s, nil)
	ctx := context.Background()
	bad := ds[1]
	bad.ReviewItemID = "ri-unknown000000"
	bad.ID = domain.DecisionID(bad.ReviewItemID, bad.Reviewer, bad.DecidedAt)
	if _, err := q.Decide(ctx, []domain.ReviewDecision{ds[0], bad}); err == nil {
		t.Fatal("a batch with an unknown item was accepted")
	}
	snap, _ := s.Load(ctx, Query{})
	if len(snap.Decisions) != 0 {
		t.Fatalf("%d decisions written by a refused batch", len(snap.Decisions))
	}
	for _, it := range snap.ReviewItems {
		if it.Status != domain.ReviewPending {
			t.Errorf("item %s changed to %s", it.ID, it.Status)
		}
	}
	if len(snap.Facts) != 0 {
		t.Error("facts written by a refused batch")
	}
	_ = items
}

func TestDecideRetryAfterAStoreFailureCompletesOnce(t *testing.T) {
	s, items, ds := batchOfTwo(t)
	ctx := context.Background()
	flaky := NewQueue(&failAfterStore{Store: s, n: 1}, nil) // the first decision lands, then the disk fails
	if _, err := flaky.Decide(ctx, ds); err == nil {
		t.Fatal("the store failure was swallowed")
	}
	if _, err := NewQueue(s, nil).Decide(ctx, ds); err != nil {
		t.Fatalf("retry: %v", err)
	}
	snap, _ := s.Load(ctx, Query{})
	if len(snap.Decisions) != 2 {
		t.Fatalf("%d decisions after the retry, want 2 (no duplicates)", len(snap.Decisions))
	}
	for _, it := range snap.ReviewItems {
		for _, want := range items {
			if it.ID == want.ID && it.Status != domain.ReviewDecided {
				t.Errorf("item %s is %s after the retry", it.ID, it.Status)
			}
		}
	}
}

// CONTRACT-CHANGE(dashboard): InboxFilter.Priority and InboxCounts.HighPriority.
func TestInboxPriorityFilterAndHighCounter(t *testing.T) {
	s, _, _ := batchOfTwo(t) // two normal-priority items
	c := fixtureCandidate()
	full := rotationAssertion(domain.ConsequenceBehaviorChange)
	prop := domain.SemanticAssertion{}
	mergeAspect(&prop, partOf(full, domain.AspectConsequence), domain.AspectConsequence)
	high := newItem(c, domain.QuestionClassification, prop, domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityHigh}, nil, nil, t0)
	mustPut(t, s, high)
	q := NewQueue(s, nil)
	ctx := context.Background()
	for pri, want := range map[domain.ReviewPriority]int{domain.PriorityHigh: 1, domain.PriorityNormal: 2, domain.PriorityLow: 0, "": 3} {
		in, err := q.Inbox(ctx, InboxFilter{Priority: pri})
		if err != nil {
			t.Fatal(err)
		}
		if len(in.Items) != want || in.Matches != want {
			t.Errorf("priority %q: %d items (%d matches), want %d", pri, len(in.Items), in.Matches, want)
		}
		// the counters ignore the priority filter
		if in.Counts.Pending != 3 || in.Counts.HighPriority != 1 {
			t.Errorf("priority %q: counts %+v", pri, in.Counts)
		}
	}
}
