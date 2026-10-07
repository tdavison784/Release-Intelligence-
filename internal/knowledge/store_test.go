package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestFileStoreIdempotentAndLayout(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	c := fixtureCandidate()
	mustPut(t, s, c)
	mustPut(t, s, c) // identical re-put is a no-op
	want := filepath.Join(dir, "cert-manager", "v1.18.0", "candidates", c.ID+".json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("layout: %v", err)
	}
	rec, err := s.Get(context.Background(), c.ID)
	if err != nil || rec.Candidate == nil || rec.Candidate.Title != c.Title {
		t.Fatalf("get = %+v, %v", rec, err)
	}
	if _, err := s.Get(context.Background(), "sc-missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing get = %v", err)
	}
	snap, err := s.Load(context.Background(), Query{Product: "cert-manager", Releases: []string{"v1.18.0"}})
	if err != nil || len(snap.Candidates) != 1 {
		t.Fatalf("load = %+v, %v", snap, err)
	}
	other, _ := s.Load(context.Background(), Query{Product: "argo-cd"})
	if len(other.Candidates) != 0 {
		t.Fatal("product filter ignored")
	}
}

func TestFileStoreRefusesInvalidAndOrphans(t *testing.T) {
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	p := proposal(c, "zai", "glm", rotationAssertion(domain.ConsequenceBehaviorChange))
	rec, _ := domain.NewRecord(p)
	if err := s.Put(context.Background(), rec); !errors.Is(err, ErrOrphan) {
		t.Fatalf("proposal before candidate = %v, want ErrOrphan", err)
	}
	bad := c
	bad.Title = ""
	rec, _ = domain.NewRecord(bad)
	if err := s.Put(context.Background(), rec); err == nil || !strings.Contains(err.Error(), "invalid record") {
		t.Fatalf("invalid candidate = %v", err)
	}
}

func TestFileStoreImmutability(t *testing.T) {
	s, _, c, item := seeded(t)
	// immutable kind: same id, different content
	changed := c
	changed.Title = "something else"
	rec, _ := domain.NewRecord(changed)
	if err := s.Put(context.Background(), rec); !errors.Is(err, ErrConflict) {
		t.Fatalf("candidate rewrite = %v, want ErrConflict", err)
	}
	// review items: status only
	it := item
	it.Status = domain.ReviewDeferred
	mustPut(t, s, it)
	it.Question = "different question"
	rec, _ = domain.NewRecord(it)
	if err := s.Put(context.Background(), rec); !errors.Is(err, ErrConflict) {
		t.Fatalf("item rewrite = %v, want ErrConflict", err)
	}
}

func TestSnapshotMinVerification(t *testing.T) {
	s, q, _, item := seeded(t)
	ctx := context.Background()
	d := decision(item, "proxy-claude", domain.ReviewerProxy, domain.ActionAccept, t0.Add(time.Hour))
	if _, err := q.Decide(ctx, []domain.ReviewDecision{d}); err != nil {
		t.Fatal(err)
	}
	all, _ := s.Load(ctx, Query{Kinds: []domain.RecordKind{domain.RecordFact}})
	if len(all.Facts) != 1 || all.Facts[0].Level() != domain.VerifiedProxy {
		t.Fatalf("facts = %+v", all.Facts)
	}
	trusted, _ := s.Load(ctx, Query{Kinds: []domain.RecordKind{domain.RecordFact}, MinVerification: domain.VerifiedHuman})
	if len(trusted.Facts) != 0 {
		t.Fatal("a proxy fact passed a human minimum")
	}
}

func TestEnvironmentContextLabelIsTheCaseIDOrAbsent(t *testing.T) {
	ctx := context.Background()
	route := func(env *domain.EnvironmentContext) []domain.ReviewItem {
		s := NewFileStore(t.TempDir())
		c := fixtureCandidate()
		mustPut(t, s, c)
		mustPut(t, s, proposal(c, "zai", "glm", rotationAssertion(domain.ConsequenceBehaviorChange)))
		if _, err := RouteStore(ctx, s, RouteOptions{Environment: env}, Query{}); err != nil {
			t.Fatal(err)
		}
		snap, _ := s.Load(ctx, Query{})
		return snap.ReviewItems
	}
	for _, it := range route(nil) {
		if it.Context != nil {
			t.Fatalf("environment-free routing set a context: %+v", it.Context)
		}
	}
	items := route(&domain.EnvironmentContext{Label: "cert-manager-1.17-1.18", Digest: "sha256:env"})
	if len(items) == 0 {
		t.Fatal("no items")
	}
	for _, it := range items {
		if it.Context == nil || it.Context.Label != "cert-manager-1.17-1.18" || it.Context.Digest != "sha256:env" {
			t.Fatalf("context = %+v", it.Context)
		}
	}
	for _, bad := range []string{"", " x", "a b", "eval/cases/x"} {
		if ValidateEnvironment(&domain.EnvironmentContext{Label: bad}) == nil {
			t.Errorf("label %q accepted", bad)
		}
	}
}

func TestFollowUpsInheritTheItemsEnvironment(t *testing.T) {
	_, q, c, _ := seeded(t) // consequence item only; build a mapping item with context
	s := NewFileStore(t.TempDir())
	q = NewQueue(s, nil)
	mustPut(t, s, c)
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	p := proposal(c, "zai", "glm", a)
	mustPut(t, s, p)
	prop := domain.SemanticAssertion{Subject: a.Subject, Change: a.Change}
	it := newItem(c, domain.QuestionSemanticMapping, prop, domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal}, []domain.SemanticProposal{p}, nil, t0)
	it.Context = &domain.EnvironmentContext{Label: "case-a"}
	mustPut(t, s, it)
	out, err := q.Decide(context.Background(), []domain.ReviewDecision{decision(it, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))})
	if err != nil || len(out[0].FollowUps) == 0 {
		t.Fatalf("follow-ups = %+v, %v", out, err)
	}
	for _, f := range out[0].FollowUps {
		if f.Context == nil || f.Context.Label != "case-a" {
			t.Fatalf("follow-up lost the environment: %+v", f.Context)
		}
	}
}
