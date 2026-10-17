package reviewui

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// The 2026-10-16 finding (lane rescore): a human accepted the audit items of
// pin-source facts, which verify the consequence only; change/applicability
// stayed consensus, so the facts stayed consensus (weakest aspect) while the
// default view showed nothing left. These tests pin the fact-level rules.

var flT0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func flStr(s string) *string { return &s }

func flEvidence() domain.Evidence {
	return domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example/p/releases/v1.2.0", "L1-L3",
		"--enable-x is now disabled by default", "sha256:notes", flT0)
}

func flCandidate(title string) domain.SemanticCandidate {
	e := flEvidence()
	a := domain.ChangeAnchor{Release: "1.2.0", EvidenceKeys: []string{domain.EvidenceKey(e)},
		StatementKeys: []string{domain.StatementKey(e)}, ChangeIDs: []string{"chg-" + title}}
	members := []domain.CandidateMember{{ChangeID: "chg-" + title, Anchor: &a}}
	return domain.SemanticCandidate{
		ID: domain.CandidateID("p", "1.2.0", members), Product: "p", Release: "1.2.0", Members: members,
		Grouping: "single", Category: domain.CategoryConfiguration, Title: title, Evidence: []domain.Evidence{e},
		Producer: "semantic.candidates@v1", CreatedAt: flT0,
	}
}

// flAssertion: a default change of a CLI flag (an OverrideInspection pin source).
func flAssertion(flag string) domain.SemanticAssertion {
	return domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectCLIFlag, Product: "p", Name: flag},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: flStr(`"true"`), After: flStr(`"false"`)},
		Applicability: &domain.Applicability{
			Exposure: domain.Condition{Op: domain.OpCLIFlag, Name: flag, State: domain.StateUnset},
			Overlap:  &domain.Condition{Op: domain.OpCLIFlag, Name: flag, State: domain.StateSet},
		},
		Consequence: &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ConsequenceBehaviorChange.ExposedClass(),
			Severity: domain.SeverityMedium, Statement: "the feature is off unless the flag is set"},
		Statement: flag + " is now disabled by default",
	}
}

func flProposal(c domain.SemanticCandidate, callID string, a domain.SemanticAssertion) domain.SemanticProposal {
	at := flT0.Add(time.Minute)
	p := domain.SemanticProposal{CandidateID: c.ID, Task: domain.TaskFull, Provider: "anthropic", Assertion: a,
		Citations: []domain.EvidenceID{flEvidence().ID},
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "semantic.propose@v1", Confidence: domain.ConfidenceMedium,
			Model: "claude-sonnet-5-5", ModelVersion: "claude-sonnet-5-5-1", PromptVersion: "semantic/v1", PromptDigest: "sha256:p",
			InputEvidence: []domain.EvidenceID{flEvidence().ID}, GeneratedAt: &at, CallID: callID, Provider: "anthropic"}}
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	return p
}

func flPut(t *testing.T, s knowledge.Store, e any) {
	t.Helper()
	rec, err := domain.NewRecord(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), rec); err != nil {
		t.Fatalf("put %T: %v", e, err)
	}
}

func flDecision(it domain.ReviewItem, kind domain.ReviewerKind, action domain.DecisionAction, at time.Time) domain.ReviewDecision {
	orig := it.Proposed
	d := domain.ReviewDecision{ReviewItemID: it.ID, Action: action, Original: &orig, Reviewer: "product-owner", ReviewerKind: kind,
		StartedAt: at.Add(-time.Minute), DecidedAt: at, Labels: []domain.FeedbackLabel{domain.LabelAccepted}}
	if kind == domain.ReviewerProxy {
		g := flT0
		d.Reviewer = "claude-proxy"
		d.ProxyProvenance = &domain.Provenance{Method: domain.MethodAI, Producer: "proxy@v1", Confidence: domain.ConfidenceMedium,
			Model: "claude-sonnet-5-5", ModelVersion: "x", PromptVersion: "proxy/v1", PromptDigest: "sha256:x",
			InputEvidence: []domain.EvidenceID{flEvidence().ID}, GeneratedAt: &g, Provider: "anthropic"}
	}
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	return d
}

// flStore builds the real situation in a file store: an auto-approved
// pin-source fact (subject validator-confirmed, the rest consensus) whose
// audit item a human accepted (consequence human, fact still consensus).
func flStore(t *testing.T) (string, knowledge.Store, domain.VerifiedFact, domain.ReviewDecision) {
	t.Helper()
	dir := t.TempDir()
	s := knowledge.NewFileStore(dir)
	ctx := context.Background()
	c := flCandidate("x off by default")
	a := flAssertion("--enable-x")
	flPut(t, s, c)
	flPut(t, s, flProposal(c, "call-1", a))
	flPut(t, s, flProposal(c, "call-2", a))
	v := domain.ValidationResult{CandidateID: c.ID, Validator: "semvalidate.flags@v1", Assertion: a,
		Evidence: []domain.Evidence{flEvidence()}, CheckedAt: flT0.Add(2 * time.Minute),
		Checks: []domain.AspectCheck{{Aspect: domain.AspectSubject, Outcome: domain.OutcomeConfirmed, Rule: "test:flag"}}}
	v.ID = domain.ValidationID(v.CandidateID, v.Validator, v.Assertion)
	flPut(t, s, v)
	if _, err := knowledge.RouteStore(ctx, s, knowledge.RouteOptions{Policy: knowledge.AutoApproveGeneral, Now: func() time.Time { return flT0.Add(time.Hour) }}, knowledge.Query{}); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Load(ctx, knowledge.Query{})
	if len(snap.Facts) != 1 {
		t.Fatalf("fixture: %d facts", len(snap.Facts))
	}
	audit, err := knowledge.OpenFactReview(ctx, s, snap.Facts[0].ID, flT0.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	d := flDecision(*audit, domain.ReviewerHuman, domain.ActionAccept, flT0.Add(3*time.Hour))
	out, err := knowledge.NewQueue(s, nil).Decide(ctx, []domain.ReviewDecision{d})
	if err != nil {
		t.Fatal(err)
	}
	f := *out[0].Fact
	if f.Level() != domain.VerifiedConsensus || f.AspectLevel(domain.AspectConsequence) != domain.VerifiedHuman {
		t.Fatalf("fixture fact %s: %+v", f.Level(), f.Verification)
	}
	return dir, s, f, d
}

var flScope = BlockingScope{Products: []domain.ProductID{"p"}}

func load(t *testing.T, s knowledge.Store) *knowledge.Snapshot {
	t.Helper()
	snap, err := s.Load(context.Background(), knowledge.Query{})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestFactLevelTriage(t *testing.T) {
	_, s, f, _ := flStore(t)
	// the decided audit verified only the consequence: the fact is below
	// human, listed as a group with no open question yet
	gs := PinGroups(load(t, s), flScope)
	if len(gs) != 1 || gs[0].Key != f.ID || len(gs[0].Open) != 0 || gs[0].Human() {
		t.Fatalf("groups before open-audits: %+v", gs)
	}
	if !strings.Contains(gs[0].LevelLine(), "change consensus") || !strings.Contains(gs[0].LeftLine(), "open-audits") {
		t.Errorf("group lines: %q / %q", gs[0].LevelLine(), gs[0].LeftLine())
	}
	if got := UnreviewedPinFacts(load(t, s), flScope); len(got) != 1 {
		t.Fatalf("unreviewed pin facts: %d", len(got))
	}
	it, err := knowledge.OpenWholeFactReview(context.Background(), s, f.ID, flT0.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	snap := load(t, s)
	tr := triageItems(snap, flScope)
	if tt := tr[it.ID]; !tt.NeedsReview || !tt.PinSource || tt.Group != f.ID {
		t.Fatalf("whole-fact item triage: %+v", tt)
	}
	gs = PinGroups(snap, flScope)
	if len(gs) != 1 || !slices.Equal(gs[0].Open, []string{it.ID}) || gs[0].LeftLine() != "1 question left to verify this fact" {
		t.Fatalf("groups after open-audits: %+v", gs)
	}
	if pins, _, _ := CountViews(snap, flScope); pins != 1 {
		t.Errorf("pin count %d", pins)
	}
	if got := UnreviewedPinFacts(snap, flScope); len(got) != 0 {
		t.Errorf("a whole-fact item exists, still unreviewed: %d", len(got))
	}
	// out of scope: nothing is a pin source
	if gs := PinGroups(snap, BlockingScope{}); len(gs) != 0 {
		t.Errorf("unscoped groups: %d", len(gs))
	}
}

func TestSiblingQuestionsOfAPinSourceCandidateAreSurfaced(t *testing.T) {
	c := flCandidate("metrics off by default")
	a := flAssertion("--enable-metrics")
	mapping := domain.ReviewItem{CandidateID: c.ID, Product: "p", Release: "1.2.0", QuestionType: domain.QuestionSemanticMapping,
		Proposed: domain.SemanticAssertion{Subject: a.Subject, Change: a.Change}, Status: domain.ReviewDecided}
	mapping.ID = domain.ReviewItemID(c.ID, mapping.QuestionType, mapping.Proposed)
	app := domain.ReviewItem{CandidateID: c.ID, Product: "p", Release: "1.2.0", QuestionType: domain.QuestionApplicability,
		Proposed: domain.SemanticAssertion{Applicability: a.Applicability}, Status: domain.ReviewDecided}
	app.ID = domain.ReviewItemID(c.ID, app.QuestionType, app.Proposed)
	cons := domain.ReviewItem{CandidateID: c.ID, Product: "p", Release: "1.2.0", QuestionType: domain.QuestionConsequence,
		Proposed: domain.SemanticAssertion{Consequence: a.Consequence}, Status: domain.ReviewPending}
	cons.ID = domain.ReviewItemID(c.ID, cons.QuestionType, cons.Proposed)
	snap := &knowledge.Snapshot{
		Candidates:  []domain.SemanticCandidate{c},
		ReviewItems: []domain.ReviewItem{mapping, app, cons},
		Decisions: []domain.ReviewDecision{
			flDecision(mapping, domain.ReviewerHuman, domain.ActionAccept, flT0.Add(time.Hour)),
			flDecision(app, domain.ReviewerHuman, domain.ActionAccept, flT0.Add(2*time.Hour)),
		},
	}
	tr := triageItems(snap, flScope)
	// the consequence question alone is no pin source, but it is the last
	// question before the pin-source fact can compose
	if tt := tr[cons.ID]; !tt.NeedsReview || !tt.PinSource || tt.Group != c.ID || tt.Why != pinSourceWhy {
		t.Fatalf("sibling consequence question: %+v", tt)
	}
	gs := PinGroups(snap, flScope)
	if len(gs) != 1 || gs[0].Fact != nil || !slices.Equal(gs[0].Open, []string{cons.ID}) {
		t.Fatalf("groups: %+v", gs)
	}
	if got := gs[0].LevelLine(); got != "subject human · change human · applicability human · consequence not verified yet" {
		t.Errorf("levels: %q", got)
	}
	// a proxy decision on the sibling does not close it (proxy is never human)
	snap.Decisions = append(snap.Decisions, flDecision(cons, domain.ReviewerProxy, domain.ActionAccept, flT0.Add(3*time.Hour)))
	snap.ReviewItems[2].Status = domain.ReviewDecided
	if tt := triageItems(snap, flScope)[cons.ID]; !tt.PinSource || !tt.ProxyActed {
		t.Errorf("proxy-decided sibling: %+v", tt)
	}
	// a human decision does: nothing open, and with no fact the group leaves
	snap.Decisions = append(snap.Decisions, flDecision(cons, domain.ReviewerHuman, domain.ActionAccept, flT0.Add(4*time.Hour)))
	if gs := PinGroups(snap, flScope); len(gs) != 0 {
		t.Errorf("after the human decision: %+v", gs)
	}
}

func TestFactWithEveryAspectTrustedIsNotListed(t *testing.T) {
	c := flCandidate("x")
	a := flAssertion("--enable-x")
	f := domain.VerifiedFact{ID: domain.VerifiedFactID("p", "1.2.0", a), Product: "p", Release: "1.2.0", Candidates: []string{c.ID},
		Assertion: a, Status: domain.FactActive}
	for _, x := range domain.Aspects {
		f.Verification = append(f.Verification, domain.AspectVerification{Aspect: x, Level: domain.VerifiedHuman})
	}
	snap := &knowledge.Snapshot{Candidates: []domain.SemanticCandidate{c}, Facts: []domain.VerifiedFact{f}}
	if gs := PinGroups(snap, flScope); len(gs) != 0 {
		t.Fatalf("a human fact is listed: %+v", gs)
	}
	f.Verification[2].Level = domain.VerifiedProxy // applicability by a proxy only
	snap.Facts[0] = f
	if gs := PinGroups(snap, flScope); len(gs) != 1 {
		t.Fatalf("a proxy-verified aspect must keep the fact listed: %+v", gs)
	}
}

// The whole flow over HTTP against the real file store: the default view
// groups the whole-fact item under its fact, its page shows every aspect,
// accepting records the displayed aspects and makes the fact human; the
// earlier decision is untouched.
func TestWholeFactReviewThroughTheUI(t *testing.T) {
	dir, s, f, audit := flStore(t)
	ctx := context.Background()
	it, err := knowledge.OpenWholeFactReview(ctx, s, f.ID, flT0.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	auditPath := ""
	_ = filepath.WalkDir(dir, func(p string, _ os.DirEntry, _ error) error {
		if filepath.Base(p) == audit.ID+".json" {
			auditPath = p
		}
		return nil
	})
	before, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return flT0.Add(5 * time.Hour) }
	srv := httptest.NewServer(NewServer(knowledge.NewQueue(s, clock), Options{Now: clock, Logf: t.Logf, Scope: flScope,
		Snapshot: func(ctx context.Context) (*knowledge.Snapshot, error) { return s.Load(ctx, knowledge.Query{}) }}))
	defer srv.Close()
	r := &rig{t: t, srv: srv, c: newRig(t).c}

	_, body := r.get("/")
	if got := cardOrder(body); !slices.Equal(got, []string{it.ID}) {
		t.Fatalf("default view cards = %v", got)
	}
	contains(t, body, `class="grouphead" data-group="`+f.ID+`"`, "1 question left to verify this fact",
		"subject deterministic · change consensus · applicability consensus · consequence human")

	_, page := r.get("/items/" + it.ID)
	contains(t, page, `id="wholefact"`, "fact level now: consensus", "Yes — the whole fact is correct", "Whole-fact review",
		`data-aspect="subject"`, `data-aspect="change"`, `data-aspect="applicability"`, `data-aspect="consequence"`, "becomes human", "stays deterministic")

	// bulk accept of a whole fact needs its page opened first
	if g := bulkGuard(&knowledge.ReviewContext{Item: *it}); !strings.Contains(g, "whole fact") {
		t.Errorf("bulk guard = %q", g)
	}

	st, b, _ := r.post("/items/"+it.ID+"/decision", form("action", "accept", "reviewer", "product-owner",
		"started", flT0.Add(4*time.Hour+30*time.Minute).Format(time.RFC3339Nano), "return", "/"))
	if st != http.StatusSeeOther {
		t.Fatalf("decision: %d %s", st, b)
	}
	snap := load(t, s)
	var d domain.ReviewDecision
	for _, x := range snap.Decisions {
		if x.ReviewItemID == it.ID {
			d = x
		}
	}
	if d.ReviewerKind != domain.ReviewerHuman || !slices.Equal(d.Aspects, domain.Aspects) {
		t.Fatalf("recorded decision: kind %s aspects %v", d.ReviewerKind, d.Aspects)
	}
	var got domain.VerifiedFact
	for _, x := range snap.Facts {
		if x.ID == f.ID {
			got = x
		}
	}
	if !got.Level().Trusted() || got.AspectLevel(domain.AspectSubject) != domain.VerifiedDeterministic ||
		got.AspectLevel(domain.AspectChange) != domain.VerifiedHuman || got.AspectLevel(domain.AspectApplicability) != domain.VerifiedHuman {
		t.Fatalf("fact after accept: %s %+v", got.Level(), got.Verification)
	}
	after, _ := os.ReadFile(auditPath)
	if !bytes.Equal(before, after) {
		t.Error("the earlier decision changed")
	}
	_, body = r.get("/")
	contains(t, body, "Needs your verification: 0")
	if gs := PinGroups(snap, flScope); len(gs) != 0 {
		t.Errorf("groups after the whole-fact accept: %+v", gs)
	}
}
