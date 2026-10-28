package reviewui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// A synthetic product ("acme-ctl": Widget.spec.retention.mode default Keep →
// Purge); never an evaluation case.

var hT0 = time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)

func hstr(s string) *string { return &s }

func hRule(overlap bool) domain.SemanticAssertion {
	field := func(st domain.FieldState, vs ...string) domain.Condition {
		return domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Widget",
			Of: []domain.Condition{{Op: domain.OpField, Path: "spec.retention.mode", State: st, Values: vs}}}
	}
	a := domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectCRDField, Product: "acme-ctl", Group: "acme.example", Kind: "Widget", Path: "spec.retention.mode"},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: hstr(`"Keep"`), After: hstr(`"Purge"`)},
		Applicability: &domain.Applicability{Exposure: field(domain.StateUnset)},
		Consequence:   &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired, Statement: "data is purged on rollover"},
		Statement:     "Widgets that keep the old retention explicitly are not affected",
	}
	if overlap {
		o := field(domain.StateEquals, `"Keep"`)
		a.Applicability.Overlap = &o
	}
	return a
}

func hPut(t *testing.T, s knowledge.Store, e any) {
	t.Helper()
	rec, err := domain.NewRecord(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func hDecision(it domain.ReviewItem, at time.Time) domain.ReviewDecision {
	orig := it.Proposed
	d := domain.ReviewDecision{ReviewItemID: it.ID, Action: domain.ActionAccept, Labels: []domain.FeedbackLabel{domain.LabelAccepted}, Original: &orig,
		Reviewer: "product-owner", ReviewerKind: domain.ReviewerHuman, StartedAt: at.Add(-time.Minute), DecidedAt: at}
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	return d
}

// hStore: a human-verified broad fact and an exported agent hypothesis
// (proposal + whole-fact item) refining it.
func hStore(t *testing.T) (knowledge.Store, domain.VerifiedFact, domain.ReviewItem) {
	t.Helper()
	s := knowledge.NewFileStore(t.TempDir())
	ev := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.test/acme/v2.0.0/notes.md", "L3", "retention.mode default is now Purge", "sha256:n", hT0)
	an := domain.ChangeAnchor{Release: "2.0.0", EvidenceKeys: []string{domain.EvidenceKey(ev)}, StatementKeys: []string{domain.StatementKey(ev)}, ChangeIDs: []string{"chg-ret"}}
	members := []domain.CandidateMember{{ChangeID: "chg-ret", Anchor: &an}}
	c := domain.SemanticCandidate{ID: domain.CandidateID("acme-ctl", "2.0.0", members), Product: "acme-ctl", Release: "2.0.0", Members: members, Grouping: "single",
		Category: domain.CategoryConfiguration, Title: "retention default", Evidence: []domain.Evidence{ev}, Producer: "semantic.candidates@v1", CreatedAt: hT0}
	hPut(t, s, c)
	v := domain.ValidationResult{CandidateID: c.ID, Validator: "semvalidate.crd@v1", Assertion: hRule(false), Evidence: []domain.Evidence{ev}, CheckedAt: hT0.Add(time.Minute)}
	for _, x := range []domain.Aspect{domain.AspectSubject, domain.AspectChange, domain.AspectApplicability} {
		v.Checks = append(v.Checks, domain.AspectCheck{Aspect: x, Outcome: domain.OutcomeConfirmed, Rule: "test"})
	}
	v.ID = domain.ValidationID(v.CandidateID, v.Validator, v.Assertion)
	hPut(t, s, v)
	prov := func(rule, call string) domain.Provenance {
		at := hT0.Add(2 * time.Minute)
		return domain.Provenance{Method: domain.MethodAI, Producer: "investigate.agent@v1", Rule: rule, Confidence: domain.ConfidenceMedium, Model: "claude-opus-5-5",
			ModelVersion: "claude-opus-5-5", PromptVersion: "investigate/v6", PromptDigest: "sha256:p", InputEvidence: []domain.EvidenceID{ev.ID}, GeneratedAt: &at, CallID: call, Provider: "anthropic"}
	}
	sem := domain.SemanticProposal{CandidateID: c.ID, Task: domain.TaskFull, Provider: "anthropic", Assertion: hRule(false), Citations: []domain.EvidenceID{ev.ID}, Provenance: prov("", "call-sem")}
	sem.ID = domain.ProposalID(sem.CandidateID, sem.Task, sem.Provider, sem.Provenance)
	hPut(t, s, sem)
	res := knowledge.Route(c, []domain.SemanticProposal{sem}, []domain.ValidationResult{v})
	hPut(t, s, res.ReviewItems[0])
	out, err := knowledge.NewQueue(s, nil).Decide(context.Background(), []domain.ReviewDecision{hDecision(res.ReviewItems[0], hT0.Add(10*time.Minute))})
	if err != nil {
		t.Fatal(err)
	}
	broad := *out[0].Fact
	hyp := domain.SemanticProposal{CandidateID: c.ID, Task: domain.TaskFull, Provider: "anthropic", Assertion: hRule(true), Citations: []domain.EvidenceID{ev.ID},
		Provenance: prov("hypothesis:hyp-0123456789ab@inv-0123456789ab", "call-hyp")}
	hyp.ID = domain.ProposalID(hyp.CandidateID, hyp.Task, hyp.Provider, hyp.Provenance)
	hPut(t, s, hyp)
	it := domain.ReviewItem{CandidateID: c.ID, Product: "acme-ctl", Release: "2.0.0", QuestionType: domain.QuestionFactReview, Question: "Is this agent-proposed rule correct as a whole?",
		Proposed: hRule(true), Proposals: []string{hyp.ID}, Routing: domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityHigh}, Status: domain.ReviewPending, CreatedAt: hT0.Add(20 * time.Minute)}
	it.ID = domain.ReviewItemID(it.CandidateID, it.QuestionType, it.Proposed)
	hPut(t, s, it)
	return s, broad, it
}

func hProof(it domain.ReviewItem) domain.ProofRecord {
	ev := domain.NewEvidence(domain.EvidenceStructured, "art-1", "ri:runtime/run.json", "derived/runtime/fx/run.json", "runtime run", "sha256:run", time.Time{})
	up := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.test/acme/v2.0.0/notes.md", "L3", "retention.mode default is now Purge", "sha256:n", hT0)
	p := domain.ProofRecord{Hypothesis: "hyp-0123456789ab", Investigation: "inv-0123456789ab", ReviewItem: it.ID, CandidateID: it.CandidateID, Product: it.Product,
		Release: it.Release, ProposedRuleDigest: it.Proposed.Digest(), VersionBounds: &domain.ProofBounds{From: "1.9.4", To: "2.0.0"},
		Methods: []domain.ProofMethod{
			{Method: domain.ProofCitationCheck, Producer: "verify.citations@v1", Level: domain.LevelDeterministic, Outcome: domain.MethodPass, Detail: "1 citation resolves", Evidence: []domain.Evidence{up}},
			{Method: domain.ProofRuleExercise, Producer: "verify.exercise@v1", Level: domain.LevelObserved, Outcome: domain.MethodPass, Detail: "with the condition the behaviour at TO matched FROM", Evidence: []domain.Evidence{ev},
				Arms: []domain.ProofArm{
					{Name: domain.ArmConditionPresent, Condition: `spec.retention.mode = "Keep"`, Predicted: domain.BehaviourPrevious, Observed: domain.BehaviourPrevious, ReactedFrom: true, ReactedTo: true},
					{Name: domain.ArmConditionAbsent, Condition: "spec.retention.mode unset", Predicted: domain.BehaviourNew, Observed: domain.BehaviourNew, ReactedFrom: true, ReactedTo: true, Changed: true,
						Signatures: []string{"core/v1/ConfigMap/ri-verify/ri-arm-absent-data data:payload"}},
				}},
		},
		Limitations: []string{"one synthetic object per arm"}, Producer: "verify.hypothesis@v1", CheckedAt: hT0.Add(30 * time.Minute)}
	p.Outcome, p.Level = domain.ProofOutcomeOf(p.Methods), domain.LevelObserved
	p.ID = domain.ProofRecordID(p)
	return p
}

func TestAgentProposedRuleThroughTheUI(t *testing.T) {
	s, broad, it := hStore(t)
	scope := BlockingScope{Products: []domain.ProductID{"acme-ctl"}}
	clock := func() time.Time { return hT0.Add(time.Hour) }
	srv := httptest.NewServer(NewServer(knowledge.NewQueue(s, clock), Options{Now: clock, Logf: t.Logf, Scope: scope,
		Snapshot: func(ctx context.Context) (*knowledge.Snapshot, error) { return s.Load(ctx, knowledge.Query{}) }}))
	defer srv.Close()
	r := &rig{t: t, srv: srv, c: newRig(t).c}
	accept := func(extra ...string) (int, string) {
		st, b, _ := r.post("/items/"+it.ID+"/decision", form(append([]string{"action", "accept", "reviewer", "product-owner",
			"started", hT0.Add(50 * time.Minute).Format(time.RFC3339Nano), "return", "/"}, extra...)...))
		return st, b
	}

	// without a proof: out of the default view, the page says why, the gate refuses
	_, body := r.get("/")
	if slices.Contains(cardOrder(body), it.ID) {
		t.Fatal("an unverified agent rule is in the default view")
	}
	_, other := r.get("/?scope=other")
	contains(t, other, "agent-proposed rules awaiting a confirmed proof")
	_, page := r.get("/items/" + it.ID)
	contains(t, page, `id="hypothesis"`, "promotion gate: closed", "No proof record yet", "AI — not verified",
		"Widgets that keep the old retention explicitly are not affected", "Preserved (the proposed mitigation)", "Not promotable yet")
	if st, b := accept("relation-"+broad.ID, "supersedes"); st != http.StatusUnprocessableEntity {
		t.Fatalf("accept without a proof: %d", st)
	} else {
		contains(t, b, "no proof record")
	}

	// a confirmed proof: in the default view; the page shows the proof
	hPut(t, s, hProof(it))
	_, body = r.get("/")
	if !slices.Contains(cardOrder(body), it.ID) {
		t.Fatalf("a confirmed agent rule is not in the default view: %v", cardOrder(body))
	}
	_, page = r.get("/items/" + it.ID)
	contains(t, page, "promotion gate: open", "citation-check", "rule-exercise", "observed", "condition-present", "condition-absent",
		"data:payload", "Limitations", "Yes — promote this verified rule", `name="relation-`+broad.ID+`"`)

	// the reviewer must say how the rule relates to the broad fact
	if st, b := accept(); st != http.StatusUnprocessableEntity {
		t.Fatalf("accept leaving the broad fact unaccounted for: %d", st)
	} else {
		contains(t, b, "never silently coexists")
	}
	if st, b := accept("relation-"+broad.ID, "supersedes"); st != http.StatusSeeOther {
		t.Fatalf("promotion: %d %s", st, b)
	}
	snap := load(t, s)
	status := map[string]domain.FactStatus{}
	for _, f := range snap.Facts {
		status[f.ID] = f.Status
	}
	narrow := domain.VerifiedFactID("acme-ctl", "2.0.0", hRule(true))
	if status[broad.ID] != domain.FactSuperseded || status[narrow] != domain.FactActive {
		t.Fatalf("facts after promotion: %v", status)
	}
}

// A confirmed proof's role-membership default (PD-6, lane verify4) is shown
// to the reviewer with its role, values and limitation before the decision,
// and said not to be a default of the field.
func TestRoleMembershipDefaultShownToTheReviewer(t *testing.T) {
	s, _, it := hStore(t)
	clock := func() time.Time { return hT0.Add(time.Hour) }
	srv := httptest.NewServer(NewServer(knowledge.NewQueue(s, clock), Options{Now: clock, Logf: t.Logf,
		Snapshot: func(ctx context.Context) (*knowledge.Snapshot, error) { return s.Load(ctx, knowledge.Query{}) }}))
	defer srv.Close()
	r := &rig{t: t, srv: srv, c: newRig(t).c}
	p := hProof(it)
	ex := p.Methods[1]
	p.RoleMembershipDefaults = []domain.RoleMembershipDefault{{Group: "acme.example", Kind: "Widget", Path: "spec.archiver", Role: "archive-trust",
		RoleValue: "true", OutsideValue: "false", Level: domain.LevelObserved, Arms: []string{domain.ArmConditionAbsent, domain.ArmConditionPresent, domain.ArmConditionAbsent},
		Observation: "with spec.archiver unset the relationship could not form", Versions: []string{"1.9.4", "2.0.0"}, ControllerValues: "{}",
		Fixture: "sha256:fx", Evidence: ex.Evidence, Limitation: domain.ObservedDefaultLimitation, RuleDigest: p.ProposedRuleDigest}}
	p.ID = domain.ProofRecordID(p)
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	hPut(t, s, p)
	_, page := r.get("/items/" + it.ID)
	contains(t, page, "Role-membership defaults", "spec.archiver</code> unset on Widget ⇒ outside role <b>archive-trust</b>",
		"Limitation: observed on the fixture", "They are not defaults of the field", "the relationship could not form")
}

// Lane backdoor (D-0029): an ordinary item offers the same choice. A second
// candidate restating the broad fact's change (another severity) must name
// the broad fact; compatible facts may be kept.
func TestOrdinaryItemOffersTheRelationChoice(t *testing.T) {
	s, broad, _ := hStore(t)
	ev := domain.NewEvidence(domain.EvidenceDocument, "guide", "https://example.test/acme/v2.0.0/upgrade.md", "L9", "retention.mode now defaults to Purge", "sha256:g", hT0)
	an := domain.ChangeAnchor{Release: "2.0.0", EvidenceKeys: []string{domain.EvidenceKey(ev)}, StatementKeys: []string{domain.StatementKey(ev)}, ChangeIDs: []string{"chg-ret-guide"}}
	members := []domain.CandidateMember{{ChangeID: "chg-ret-guide", Anchor: &an}}
	c := domain.SemanticCandidate{ID: domain.CandidateID("acme-ctl", "2.0.0", members), Product: "acme-ctl", Release: "2.0.0", Members: members, Grouping: "single",
		Category: domain.CategoryConfiguration, Title: "retention default (guide)", Evidence: []domain.Evidence{ev}, Producer: "semantic.candidates@v1", CreatedAt: hT0}
	hPut(t, s, c)
	rule := hRule(false)
	rule.Consequence.Severity = domain.SeverityMedium
	v := domain.ValidationResult{CandidateID: c.ID, Validator: "semvalidate.crd@v1", Assertion: rule, Evidence: []domain.Evidence{ev}, CheckedAt: hT0.Add(time.Minute)}
	for _, x := range []domain.Aspect{domain.AspectSubject, domain.AspectChange, domain.AspectApplicability} {
		v.Checks = append(v.Checks, domain.AspectCheck{Aspect: x, Outcome: domain.OutcomeConfirmed, Rule: "test"})
	}
	v.ID = domain.ValidationID(v.CandidateID, v.Validator, v.Assertion)
	hPut(t, s, v)
	at := hT0.Add(2 * time.Minute)
	sem := domain.SemanticProposal{CandidateID: c.ID, Task: domain.TaskFull, Provider: "anthropic", Assertion: rule, Citations: []domain.EvidenceID{ev.ID},
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "semantic.propose@v1", Confidence: domain.ConfidenceMedium, Model: "claude-opus-5-5",
			ModelVersion: "claude-opus-5-5", PromptVersion: "semantic/v1", PromptDigest: "sha256:p", InputEvidence: []domain.EvidenceID{ev.ID}, GeneratedAt: &at, CallID: "call-guide", Provider: "anthropic"}}
	sem.ID = domain.ProposalID(sem.CandidateID, sem.Task, sem.Provider, sem.Provenance)
	hPut(t, s, sem)
	res := knowledge.Route(c, []domain.SemanticProposal{sem}, []domain.ValidationResult{v})
	if len(res.ReviewItems) != 1 {
		t.Fatalf("route: %+v", res)
	}
	it := res.ReviewItems[0]
	hPut(t, s, it)

	clock := func() time.Time { return hT0.Add(time.Hour) }
	srv := httptest.NewServer(NewServer(knowledge.NewQueue(s, clock), Options{Now: clock, Logf: t.Logf, Scope: BlockingScope{Products: []domain.ProductID{"acme-ctl"}},
		Snapshot: func(ctx context.Context) (*knowledge.Snapshot, error) { return s.Load(ctx, knowledge.Query{}) }}))
	defer srv.Close()
	r := &rig{t: t, srv: srv, c: newRig(t).c}
	_, page := r.get("/items/" + it.ID)
	contains(t, page, `name="relation-`+broad.ID+`"`, "the fact this decision records")
	accept := func(extra ...string) (int, string) {
		st, b, _ := r.post("/items/"+it.ID+"/decision", form(append([]string{"action", "accept", "reviewer", "product-owner",
			"started", hT0.Add(50 * time.Minute).Format(time.RFC3339Nano), "return", "/"}, extra...)...))
		return st, b
	}
	if st, b := accept(); st != http.StatusUnprocessableEntity {
		t.Fatalf("accept leaving the broad fact unnamed: %d", st)
	} else {
		contains(t, b, "no fact silently coexists", broad.ID)
	}
	if st, b := accept("relation-"+broad.ID, "coexists"); st != http.StatusSeeOther {
		t.Fatalf("accept keeping a compatible fact: %d %s", st, b)
	}
	active := 0
	for _, f := range load(t, s).Facts {
		if f.Status == domain.FactActive {
			active++
		}
	}
	if active != 2 {
		t.Fatalf("want the broad fact and the restatement both active, got %d", active)
	}
}
