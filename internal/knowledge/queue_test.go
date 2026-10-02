package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestDecideAcceptMintsFactAnchoredToEveryMember(t *testing.T) {
	s, q, c, item := seeded(t)
	ctx := context.Background()
	d := decision(item, "engineer-1", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))
	out, err := q.Decide(ctx, []domain.ReviewDecision{d})
	if err != nil {
		t.Fatal(err)
	}
	f := out[0].Fact
	if f == nil || len(out[0].OpenAspects) != 0 {
		t.Fatalf("outcome = %+v", out[0])
	}
	if f.AspectLevel(domain.AspectConsequence) != domain.VerifiedHuman || f.AspectLevel(domain.AspectSubject) != domain.VerifiedDeterministic {
		t.Fatalf("levels = %+v", f.Verification)
	}
	if f.Level() != domain.VerifiedHuman || len(f.Anchors) != 1 || f.Candidates[0] != c.ID {
		t.Fatalf("fact = %+v", f)
	}
	if out[0].Decision.ResultingFact != f.ID {
		t.Fatal("decision does not name its fact")
	}
	// the item is decided, and the stored fact passes basis verification (Put checked it)
	rec, _ := s.Get(ctx, item.ID)
	if rec.ReviewItem.Status != domain.ReviewDecided {
		t.Fatalf("status = %s", rec.ReviewItem.Status)
	}
	// retrying the same decision is idempotent
	again, err := q.Decide(ctx, []domain.ReviewDecision{d})
	if err != nil || again[0].Fact == nil || again[0].Fact.ID != f.ID {
		t.Fatalf("retry = %+v, %v", again, err)
	}
}

func TestCorrectionKeepsBothValues(t *testing.T) {
	_, q, _, item := seeded(t)
	ctx := context.Background()
	d := decision(item, "engineer-1", domain.ReviewerHuman, domain.ActionCorrect, t0.Add(time.Hour))
	corrected := rotationAssertion(domain.ConsequenceDeprecation)
	corrected = domain.SemanticAssertion{Consequence: corrected.Consequence, Statement: corrected.Statement}
	d.Corrected = &corrected
	d.Labels = []domain.FeedbackLabel{domain.LabelCorrected, domain.LabelWrongConsequence}
	d.Reason = "works today, scheduled for removal"
	out, err := q.Decide(ctx, []domain.ReviewDecision{d})
	if err != nil {
		t.Fatal(err)
	}
	got := out[0].Decision
	if got.Original == nil || got.Corrected == nil || got.Original.Consequence.Kind == got.Corrected.Consequence.Kind {
		t.Fatalf("decision lost a value: %+v", got)
	}
	if k := out[0].Fact.Assertion.Consequence.Kind; k != domain.ConsequenceDeprecation {
		t.Fatalf("fact consequence = %s, want the corrected value", k)
	}
	if out[0].Fact.Assertion.Consequence.ExposedClass != domain.ImpactReviewRequired {
		t.Fatal("class does not follow the corrected kind")
	}
}

func TestDecideValidatesBeforeWriting(t *testing.T) {
	s, q, _, item := seeded(t)
	ctx := context.Background()
	good := decision(item, "engineer-1", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))
	bad := decision(item, "engineer-2", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))
	other := rotationAssertion(domain.ConsequenceNone)
	bad.Original = &other // shows an assertion the item never proposed
	if _, err := q.Decide(ctx, []domain.ReviewDecision{good, bad}); err == nil {
		t.Fatal("a decision showing a different assertion was accepted")
	}
	snap, _ := s.Load(ctx, Query{Kinds: []domain.RecordKind{domain.RecordDecision}})
	if len(snap.Decisions) != 0 {
		t.Fatal("part of a failed call was written")
	}
	// a proxy cannot pass as human
	px := decision(item, "claude", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))
	p := aiProv("claude-sonnet-5-5")
	px.ProxyProvenance = &p
	if _, err := q.Decide(ctx, []domain.ReviewDecision{px}); err == nil || !strings.Contains(err.Error(), "AI provenance") {
		t.Fatalf("human decision with proxy provenance = %v", err)
	}
}

func TestProxyThenHumanUpgradesTheFactAndProxyNeverOverridesHuman(t *testing.T) {
	s, q, _, item := seeded(t)
	ctx := context.Background()
	out, err := q.Decide(ctx, []domain.ReviewDecision{decision(item, "claude", domain.ReviewerProxy, domain.ActionAccept, t0.Add(time.Hour))})
	if err != nil || out[0].Fact.Level() != domain.VerifiedProxy {
		t.Fatalf("proxy outcome = %+v, %v", out, err)
	}
	out, err = q.Decide(ctx, []domain.ReviewDecision{decision(item, "engineer-1", domain.ReviewerHuman, domain.ActionAccept, t0.Add(2*time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Fact.Level() != domain.VerifiedHuman {
		t.Fatalf("level after human = %s", out[0].Fact.Level())
	}
	rec, _ := s.Get(ctx, out[0].Fact.ID)
	if rec.Fact.Level() != domain.VerifiedHuman {
		t.Fatalf("stored level = %s", rec.Fact.Level())
	}
	// a later proxy decision does not weaken it
	out, err = q.Decide(ctx, []domain.ReviewDecision{decision(item, "claude2", domain.ReviewerProxy, domain.ActionAccept, t0.Add(3*time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Fact.Level() != domain.VerifiedHuman {
		t.Fatalf("a proxy weakened the fact to %s", out[0].Fact.Level())
	}
}

func TestRejectRetractsAFactReview(t *testing.T) {
	s, q, _, item := seeded(t)
	ctx := context.Background()
	out, _ := q.Decide(ctx, []domain.ReviewDecision{decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))})
	fid := out[0].Fact.ID
	rv, err := OpenFactReview(ctx, s, fid, t0.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Decide(ctx, []domain.ReviewDecision{decision(*rv, "e2", domain.ReviewerHuman, domain.ActionReject, t0.Add(3*time.Hour))}); err != nil {
		t.Fatal(err)
	}
	rec, _ := s.Get(ctx, fid)
	if rec.Fact.Status != domain.FactRetracted {
		t.Fatalf("status = %s", rec.Fact.Status)
	}
	// re-deriving never revives it
	if _, err := RouteStore(ctx, s, nil, Query{}); err != nil {
		t.Fatal(err)
	}
	rec, _ = s.Get(ctx, fid)
	if rec.Fact.Status != domain.FactRetracted {
		t.Fatal("routing revived a retracted fact")
	}
	snap, _ := s.Load(ctx, Query{})
	if m := ComputeMetrics(snap); m.Facts.Retracted != 1 || m.Facts.Active != 0 {
		t.Fatalf("fact metrics = %+v", m.Facts)
	}
}

func TestCorrectionOfAFactReviewSupersedes(t *testing.T) {
	s, q, _, item := seeded(t)
	ctx := context.Background()
	out, _ := q.Decide(ctx, []domain.ReviewDecision{decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))})
	old := out[0].Fact.ID
	rv, _ := OpenFactReview(ctx, s, old, t0.Add(2*time.Hour))
	d := decision(*rv, "e2", domain.ReviewerHuman, domain.ActionCorrect, t0.Add(3*time.Hour))
	nc := rotationAssertion(domain.ConsequenceDeprecation)
	corrected := *d.Original
	corrected.Consequence = nc.Consequence
	d.Corrected = &corrected
	d.Labels = []domain.FeedbackLabel{domain.LabelCorrected, domain.LabelWrongClassification}
	d.Reason = "keys are no longer honoured"
	res, err := q.Decide(ctx, []domain.ReviewDecision{d})
	if err != nil {
		t.Fatal(err)
	}
	nf := res[0].Fact
	if nf == nil || nf.ID == old || len(nf.Supersedes) != 1 || nf.Supersedes[0] != old {
		t.Fatalf("new fact = %+v", nf)
	}
	rec, _ := s.Get(ctx, old)
	if rec.Fact.Status != domain.FactSuperseded {
		t.Fatalf("old status = %s", rec.Fact.Status)
	}
	active, _ := s.Load(ctx, Query{FactStatus: []domain.FactStatus{domain.FactActive}, Kinds: []domain.RecordKind{domain.RecordFact}})
	if len(active.Facts) != 1 || active.Facts[0].ID != nf.ID {
		t.Fatalf("active facts = %+v", active.Facts)
	}
}

func TestRejectWithDuplicateAddsAnchor(t *testing.T) {
	s, q, _, item := seeded(t)
	ctx := context.Background()
	out, _ := q.Decide(ctx, []domain.ReviewDecision{decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))})
	f := out[0].Fact
	// a second candidate: another restatement of the same release
	e2 := domain.NewEvidence(domain.EvidenceDocument, "release", "https://example/release-1.18.md", "L1", "rotationPolicy now defaults to Always", "sha256:r", t0)
	an := domain.ChangeAnchor{Release: "v1.18.0", EvidenceKeys: []string{domain.EvidenceKey(e2)}, StatementKeys: []string{domain.StatementKey(e2)}, ChangeIDs: []string{"chg-x"}}
	members := []domain.CandidateMember{{ChangeID: "chg-x", Anchor: &an}}
	c2 := domain.SemanticCandidate{ID: domain.CandidateID("cert-manager", "v1.18.0", members), Product: "cert-manager", Release: "v1.18.0",
		Members: members, Grouping: "single", Category: domain.CategoryConfiguration, Title: "rotationPolicy now defaults to Always",
		Evidence: []domain.Evidence{e2}, Producer: "semantic.candidates@v1", CreatedAt: t0}
	mustPut(t, s, c2)
	dup := domain.SemanticAssertion{}
	it := domain.ReviewItem{CandidateID: c2.ID, Product: "cert-manager", Release: "v1.18.0", QuestionType: domain.QuestionDuplicate,
		Question: "Is this the same change as the known fact?", Proposed: dup,
		Routing: domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityLow}, Status: domain.ReviewPending, CreatedAt: t0}
	it.ID = domain.ReviewItemID(it.CandidateID, it.QuestionType, it.Proposed)
	mustPut(t, s, it)
	d := decision(it, "e", domain.ReviewerHuman, domain.ActionReject, t0.Add(4*time.Hour))
	d.Labels = []domain.FeedbackLabel{domain.LabelDuplicate}
	d.DuplicateOf = f.ID
	d.Reason = "same change, restated"
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	res, err := q.Decide(ctx, []domain.ReviewDecision{d})
	if err != nil {
		t.Fatal(err)
	}
	_ = res
	rec, _ := s.Get(ctx, f.ID)
	if len(rec.Fact.Anchors) != 2 || len(rec.Fact.Candidates) != 2 {
		t.Fatalf("anchors/candidates = %d/%d, want 2/2", len(rec.Fact.Anchors), len(rec.Fact.Candidates))
	}
}

func TestBatchDecisions(t *testing.T) {
	s := NewFileStore(t.TempDir())
	q := NewQueue(s, nil)
	ctx := context.Background()
	c := fixtureCandidate()
	mustPut(t, s, c)
	var items []domain.ReviewItem
	for i, a := range []domain.QuestionType{domain.QuestionSemanticMapping, domain.QuestionApplicability} {
		_ = i
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
	var ds []domain.ReviewDecision
	ids := []string{items[0].ID, items[1].ID}
	for _, it := range items {
		d := decision(it, "e", domain.ReviewerHuman, domain.ActionAccept, at)
		d.BatchID, d.BatchSize = domain.BatchID(ids, "e", at), 2
		ds = append(ds, d)
	}
	out, err := q.Decide(ctx, ds)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || len(out[1].OpenAspects) != 1 || out[1].OpenAspects[0] != domain.AspectConsequence {
		t.Fatalf("outcomes = %+v", out)
	}
	if len(out[1].FollowUps) != 0 { // nothing proposed the consequence: only evidence-sufficiency
		if out[1].FollowUps[0].QuestionType != domain.QuestionEvidenceSufficiency {
			t.Fatalf("follow-ups = %+v", out[1].FollowUps)
		}
	}
	// a batch that lies about its size is refused, and corrections cannot be batched (domain rule)
	lie := ds[0]
	lie.BatchSize = 3
	if _, err := q.Decide(ctx, []domain.ReviewDecision{lie}); err == nil {
		t.Fatal("bad batch accepted")
	}
	snap, _ := s.Load(ctx, Query{})
	m := ComputeMetrics(snap)
	if m.Review.Batch.Decisions != 2 || m.Review.Batch.Batches != 1 || m.Review.Individual.Decisions != 0 {
		t.Fatalf("batch metrics = %+v / %+v", m.Review.Batch, m.Review.Individual)
	}
}

func TestInboxCountsAndFilters(t *testing.T) {
	_, q, c, item := seeded(t)
	ctx := context.Background()
	in, err := q.Inbox(ctx, InboxFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if in.Counts.Pending != 1 || in.Counts.ModelDisagreement != 1 || len(in.Items) != 1 || in.Items[0].Title != c.Title {
		t.Fatalf("inbox = %+v", in.Counts)
	}
	if len(in.Items[0].Models) != 2 {
		t.Fatalf("models = %v", in.Items[0].Models)
	}
	yes, no := true, false
	for _, f := range []struct {
		f    InboxFilter
		want int
	}{
		{InboxFilter{Disagreement: &yes}, 1}, {InboxFilter{Disagreement: &no}, 0},
		{InboxFilter{Model: "glm-5.3-flash"}, 1}, {InboxFilter{Model: "nope"}, 0},
		{InboxFilter{Source: "upgrading-1.17"}, 1}, {InboxFilter{Source: "elsewhere"}, 0},
		{InboxFilter{QuestionType: domain.QuestionApplicability}, 0}, {InboxFilter{Release: "v1.18.0"}, 1},
		{InboxFilter{SubjectType: domain.SubjectCRDField}, 0}, // consequence items carry no subject
		{InboxFilter{Severity: domain.SeverityHigh}, 1}, {InboxFilter{Confidence: domain.ConfidenceMedium}, 1},
	} {
		got, err := q.Inbox(ctx, f.f)
		if err != nil || len(got.Items) != f.want {
			t.Errorf("%+v → %d items (%v), want %d", f.f, len(got.Items), err, f.want)
		}
	}
	d := decision(item, "e", domain.ReviewerHuman, domain.ActionDefer, t0.Add(time.Hour))
	d.Labels = nil
	if _, err := q.Decide(ctx, []domain.ReviewDecision{d}); err != nil {
		t.Fatal(err)
	}
	in, _ = q.Inbox(ctx, InboxFilter{})
	if in.Counts.Deferred != 1 || in.Counts.Pending != 0 {
		t.Fatalf("counts after defer = %+v", in.Counts)
	}
	got, _ := q.Inbox(ctx, InboxFilter{Reviewer: "e"})
	if len(got.Items) != 1 {
		t.Fatal("reviewer filter")
	}
}

func TestReviewContextIncludesPreviousRelatedDecisions(t *testing.T) {
	s, q, c, item := seeded(t)
	ctx := context.Background()
	out, _ := q.Decide(ctx, []domain.ReviewDecision{decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))})
	// a later candidate about the same subject, in another release
	c2 := c
	c2.Release = "v1.19.0"
	a := *c.Members[0].Anchor
	a.Release = "v1.19.0"
	c2.Members = []domain.CandidateMember{{ChangeID: "chg-later", Anchor: &a}}
	c2.ID = domain.CandidateID(c2.Product, c2.Release, c2.Members)
	mustPut(t, s, c2)
	p := proposal(c2, "zai", "glm-5.3-flash", rotationAssertion(domain.ConsequenceBehaviorChange))
	mustPut(t, s, p)
	it2 := newItem(c2, domain.QuestionConsequence, partOf(rotationAssertion(domain.ConsequenceBehaviorChange), domain.AspectConsequence),
		domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal}, []domain.SemanticProposal{p}, nil, t0)
	// the proposal asserts the subject, so the item is related by subject key
	mustPut(t, s, it2)
	rc, err := q.Item(ctx, it2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rc.Proposals) != 1 || len(rc.RelatedFacts) != 1 || rc.RelatedFacts[0].ID != out[0].Fact.ID || len(rc.RelatedDecisions) == 0 {
		t.Fatalf("context related = %d facts, %d decisions", len(rc.RelatedFacts), len(rc.RelatedDecisions))
	}
	if rc.SuggestedExposedClass != domain.ImpactReviewRequired {
		t.Fatalf("suggested class = %s", rc.SuggestedExposedClass)
	}
}

func TestDatasetAndMetrics(t *testing.T) {
	s, q, _, item := seeded(t)
	ctx := context.Background()
	// reject path first on a second item keeps negative examples
	if _, err := q.Decide(ctx, []domain.ReviewDecision{decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))}); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Load(ctx, Query{})
	var buf bytes.Buffer
	if err := ExportDataset(snap, &buf); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("dataset lines = %d", len(lines))
	}
	var ex domain.FeedbackExample
	if err := json.Unmarshal([]byte(lines[0]), &ex); err != nil {
		t.Fatal(err)
	}
	if ex.Fact == nil || len(ex.Proposals) != 2 || len(ex.Validations) != 1 || ex.Decision.Labels[0] != domain.LabelAccepted || ex.Candidate.ID == "" {
		t.Fatalf("example = %+v", ex)
	}

	m := ComputeMetrics(snap)
	if m.Facts.Active != 1 || m.Facts.ByLevel[domain.VerifiedHuman] != 1 || m.Facts.AnchorsPerFact != 1 {
		t.Fatalf("facts = %+v", m.Facts)
	}
	if m.Review.Items != 1 || m.Review.Individual.Accept != 1 || m.Review.Individual.MedianDuration != 90*time.Second {
		t.Fatalf("review = %+v", m.Review)
	}
	var glm, sonnet ModelMetric
	for _, mm := range m.Models {
		switch mm.Model {
		case "glm-5.3-flash":
			glm = mm
		case "claude-sonnet-5-5":
			sonnet = mm
		}
	}
	// the plurality tie goes to the smaller digest; whoever proposed what the item
	// showed was accepted as is, the other lost
	winner, loser := glm, sonnet
	if item.Proposed.Consequence.Kind == domain.ConsequenceSettingIgnored {
		winner, loser = sonnet, glm
	}
	if winner.AcceptedAsIs != 1 || loser.Rejected != 1 || winner.FalsePositive[domain.AspectConsequence] != 0 || loser.FalsePositive[domain.AspectConsequence] != 1 {
		t.Fatalf("model metrics winner=%+v loser=%+v", winner, loser)
	}
	if winner.ByTask[domain.TaskFull].AcceptedAsIs != 1 {
		t.Fatalf("per task = %+v", glm.ByTask)
	}
	var cons AgreementMetric
	for _, a := range m.Agreement {
		if a.Aspect == domain.AspectConsequence {
			cons = a
		}
		if a.Aspect == domain.AspectSubject && a.Agreement != 1 {
			t.Fatalf("subject agreement = %v", a.Agreement)
		}
	}
	if cons.Candidates != 1 || cons.Agreement != 0 || cons.Pairwise["claude-sonnet-5-5|glm-5.3-flash"] != 0 {
		t.Fatalf("consequence agreement = %+v", cons)
	}
	if m.Models[0].GroundedCitations != 1 {
		t.Fatalf("grounded = %v", m.Models[0].GroundedCitations)
	}
}
