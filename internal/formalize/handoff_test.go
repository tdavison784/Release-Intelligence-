package formalize

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/agent"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// The hand-off (PLAN-M4 change 4): the formalized hypotheses feed
// agent.ExportHypotheses unchanged, through a view of the run whose
// hypotheses are the formalized ones. The rule becomes a proposal and a
// whole-fact review item proposing exactly its assertion; the
// unrepresentable record is skipped with its limitation; nothing becomes a
// fact, and the stored run record is not changed.
// handoffWorld stores the fixture's artifacts and upgrade edge and the
// knowledge candidate of the item's upstream change.
func handoffWorld(t *testing.T, ctx context.Context, f fixture) (*store.Store, knowledge.Store) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// the bundle's artifacts and the upgrade edge, content-addressed
	var arts []domain.ArtifactRef
	for _, a := range f.Bundle.Manifest.Artifacts {
		kind := domain.ArtifactUpstreamDocument
		if a.Origin == domain.OriginEnvironment {
			kind = domain.ArtifactEnvironmentFile
		}
		ref, err := st.Put(ctx, investigate.ArtifactSpec{Kind: kind, Path: a.Path, Origin: a.Origin, MediaType: "text/plain", URI: a.Path, Producer: "test@v1"},
			[]byte(artifacts[a.ID]))
		if err != nil {
			t.Fatal(err)
		}
		ref.ID = a.ID
		arts = append(arts, ref)
	}
	edge, _ := json.Marshal(domain.UpgradeEdge{Product: domain.ProductRef{ID: "acme"}})
	eref, err := st.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactUpgradeEdge, Path: "upstream/edge.json", Origin: domain.OriginUpstream, MediaType: "application/json",
		URI: "edge.json", Producer: "test@v1"}, edge)
	if err != nil {
		t.Fatal(err)
	}
	f.Bundle.Manifest.Artifacts = append(arts, eref)
	f.Bundle.Change.Edge = eref.ID
	// the knowledge candidate of the item's upstream change
	ks := knowledge.NewFileStore(t.TempDir())
	members := []domain.CandidateMember{{ChangeID: "c-1", Computed: true}}
	cand := domain.SemanticCandidate{ID: domain.CandidateID("acme", "v1.1.0", members), Product: "acme", Release: "v1.1.0", Members: members,
		Grouping: "single", Category: domain.CategoryFeature, Title: "spec.mode defaults to strict", Producer: "test@v1", CreatedAt: fixedNow,
		Evidence: []domain.Evidence{f.Bundle.Evidence[0]}}
	rec, err := domain.NewRecord(cand)
	if err != nil {
		t.Fatal(err)
	}
	if err := ks.Put(ctx, rec); err != nil {
		t.Fatal(err)
	}

	return st, ks
}

func TestFormalizedHypothesesFeedExportUnchanged(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	st, ks := handoffWorld(t, ctx, f)
	before, _ := json.Marshal(f.Inv)
	p := precompute(t, Input{Investigation: f.Inv, Bundle: f.Bundle, Assessment: aas1, Read: agent.StoreReader(st)})
	in := Input{Investigation: f.Inv, Bundle: f.Bundle, Assessment: aas1, Read: agent.StoreReader(st)}
	r, err := (&Formalizer{Client: &scripted{texts: []string{answerJSON(t, validAnswer(p))}}}).Formalize(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	view := *f.Inv
	view.Hypotheses = Hypotheses([]Record{*r})
	res, err := agent.ExportHypotheses(ctx, &view, f.Bundle, st, ks, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	var exported, skipped int
	for _, x := range res {
		switch x.Status {
		case agent.ExportExported:
			exported++
		case agent.ExportSkipped:
			skipped++
			if !strings.Contains(x.Reason, "not representable as a mitigation rule") {
				t.Errorf("skip reason = %q", x.Reason)
			}
		}
	}
	if exported != 1 || skipped != 1 {
		t.Fatalf("export = %+v", res)
	}
	snap, err := ks.Load(ctx, knowledge.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Facts) != 0 || len(snap.Decisions) != 0 || len(snap.Proposals) != 1 || len(snap.ReviewItems) != 1 {
		t.Fatalf("knowledge: %d facts, %d decisions, %d proposals, %d review items", len(snap.Facts), len(snap.Decisions), len(snap.Proposals), len(snap.ReviewItems))
	}
	var rule domain.Hypothesis
	for _, h := range r.Hypotheses {
		if h.Mitigation != nil {
			rule = h
		}
	}
	it := snap.ReviewItems[0]
	if it.Proposed.Digest() != rule.Mitigation.ProposedRule.Assertion.Digest() || !strings.Contains(it.Question, "kept by formalization") {
		t.Errorf("review item = %+v", it)
	}
	hyp, run, ok := domain.ParseHypothesisRule(snap.Proposals[0].Provenance.Rule)
	if !ok || hyp != rule.ID || run != f.Inv.ID || snap.Proposals[0].Provenance.Producer != ProducerV2 {
		t.Errorf("proposal provenance = %+v", snap.Proposals[0].Provenance)
	}
	if after, _ := json.Marshal(f.Inv); string(after) != string(before) {
		t.Error("the run record changed")
	}
}

// A scope-refinement candidate is exported like any rule: its own proposal
// and whole-fact review item proposing exactly the refined rule (another
// digest than the behaviour rule's), never a fact.
func TestRefinementCandidateIsExported(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	st, ks := handoffWorld(t, ctx, f)
	gref, err := st.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactGraph, Path: "derived/agent/consumers/to/rg-000000000001.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:" + agent.ConsumerGraphProducer, Producer: agent.ConsumerGraphProducer}, []byte(consumerGraph("ca.crt", domain.EdgeEstablished)))
	if err != nil {
		t.Fatal(err)
	}
	f.Inv.Artifacts = append(f.Inv.Artifacts, gref)
	in := Input{Investigation: f.Inv, Bundle: f.Bundle, Assessment: aas1, Read: agent.StoreReader(st), Roles: []ConsumerRole{signerRole}}
	r, err := (&Formalizer{Client: &scripted{texts: []string{answerJSON(t, validAnswer(precompute(t, in)))}}}).Formalize(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	_, refined := refinementsOf(r)
	if len(refined) != 1 {
		t.Fatalf("refined %+v notes %v", r.Hypotheses, r.RefinementNotes)
	}
	view := *f.Inv
	view.Hypotheses = Hypotheses([]Record{*r})
	res, err := agent.ExportHypotheses(ctx, &view, f.Bundle, st, ks, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	exported := map[string]bool{}
	for _, x := range res {
		if x.Status == agent.ExportExported {
			exported[x.Hypothesis] = true
		}
	}
	snap, _ := ks.Load(ctx, knowledge.Query{})
	if !exported[refined[0].ID] || len(exported) != 2 || len(snap.ReviewItems) != 2 || len(snap.Facts) != 0 {
		t.Fatalf("export %+v; %d review items, %d facts", res, len(snap.ReviewItems), len(snap.Facts))
	}
	found := false
	for _, it := range snap.ReviewItems {
		found = found || it.Proposed.Digest() == refined[0].Mitigation.ProposedRule.Assertion.Digest()
	}
	if !found {
		t.Error("no review item proposes the refined rule")
	}
}
