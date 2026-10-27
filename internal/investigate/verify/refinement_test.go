package verify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// Scope refinements (lane verify2, D-0025) on the synthetic Badge product:
// keyPolicy's default changes from Keep to Renew; the behaviour rule exposes
// every Badge that leaves keyPolicy unset. The refinement candidate narrows
// the harm to signer Badges (role spec.signer = true); a non-signer Badge
// (spec.signer = false) is shielded. The catalog's role-absent observation:
// a non-signer Badge signed by a signer the arm declares, whose consumer
// trusts its ca.crt (the signer) and verifies its own tls.crt.

const badgeOutside = `    outside:
      summary: a client that trusts the signer verifies a non-signer Badge re-issued afterwards
      role: [{path: spec.signer, value: false}]
      subject: [{path: spec.signerRef, value: "{{name}}-root-signer"}]
      objects:
      - {apiVersion: acme.example/v1, kind: Badge, metadata: {name: "{{name}}-root"}, spec: {secretName: "{{name}}-root-tls", signerRef: self, signer: true}}
      - {apiVersion: acme.example/v1, kind: Signer, metadata: {name: "{{name}}-root-signer"}, spec: {badge: "{{name}}-root"}}
      trust: {secret: "{{name}}-tls", key: ca.crt}
      reissue: ["{{name}}"]
      verify: {secret: "{{name}}-tls", key: tls.crt}
      references:
      - {url: "https://example.test/acme/docs/signer.md", quote: "a Badge signed by a signer carries the signer in ca.crt"}
`

func badgeOutsideSet(t *testing.T) *FixtureSet {
	t.Helper()
	s, err := ParseFixtures([]byte(badgeFixtures + badgeOutside))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// refinedRule is the behaviour rule narrowed to the signer role (the shape
// formalize.RefineRule writes); def, when set, states what an unset
// spec.signer means.
func refinedRule(def *domain.FieldDefault) domain.SemanticAssertion {
	signer := func(v string) domain.Condition {
		c := field("spec.signer", domain.StateEquals, v)
		if def != nil {
			d := *def
			c.Default = &d
		}
		return c
	}
	return domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectCRDField, Product: "acme-ctl", Group: "acme.example", Kind: "Badge", Path: "spec.keyPolicy"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"Keep"`), After: str(`"Renew"`)},
		Applicability: &domain.Applicability{
			Exposure: badges(field("spec.keyPolicy", domain.StateUnset), signer("true")),
			Overlap: &domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Badge", Of: []domain.Condition{{Op: domain.OpAny, Of: []domain.Condition{
				{Op: domain.OpAll, Of: []domain.Condition{field("spec.keyPolicy", domain.StateEquals, `"Keep"`)}},
				{Op: domain.OpAll, Of: []domain.Condition{signer("false")}}}}}}},
		Consequence: &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired},
		Statement:   "Badges that leave keyPolicy unset get a new key; only signer Badges' consumers notice.",
	}
}

var signerEvidence = cite("https://example.test/acme/v2.0.0/api/badge_types.go", "Signer defaults to false when omitted.", "// Signer defaults to false when omitted.")

func signerFieldDefault() *domain.FieldDefault {
	return &domain.FieldDefault{Value: "false", Source: domain.DefaultController, Citations: []domain.EvidenceID{signerEvidence.ID}}
}

var refinement = &domain.ScopeRefinement{Refines: "hyp-00000000base", BaseRule: "d", Consumer: "signer-trust",
	Role: []domain.RoleField{{Path: "spec.signer", Value: "true"}}, Outside: []domain.RoleField{{Path: "spec.signer", Value: "false"}}, TrustKey: "ca.crt",
	Trace: []domain.TraceRead{{Graph: "art-g", Edge: "ge-1", Producer: "acme.example/v1/Badge/ns/b", Secret: "core/v1/Secret/ns/b-tls", Key: "ca.crt", Workload: "apps/v1/Deployment/ns/app"}}}

func refinementInput(t *testing.T, rule domain.SemanticAssertion, run *fakeRunner) ExerciseInput {
	return ExerciseInput{Product: "acme-ctl", Rule: rule, Bounds: bounds, Fixtures: badgeOutsideSet(t), Exercises: []runtime.DeclaredExercise{reissueExercise()},
		Relationships: badgeProduces, Renderer: &fakeRenderer{}, Runner: run, Evidence: []domain.Evidence{signerEvidence}, Refinement: refinement}
}

func TestRoleAbsentObservation(t *testing.T) {
	fx, _ := badgeOutsideSet(t).Subject("acme.example", "Badge")
	cf, _ := consumerFor(refinedRule(nil), fx)
	if cf == nil || cf.Outside == nil {
		t.Fatal("the role-absent observation is not declared")
	}
	arms, _, why := BuildConsumerArms(refinedRule(nil), fx, cf)
	if why != "" {
		t.Fatal(why)
	}
	var outside *consumerArm
	for i := range arms {
		if isOutsideRole(arms[i].Name) {
			outside = &arms[i]
		}
	}
	if outside == nil {
		t.Fatalf("no outside-role arm: %+v", arms)
	}
	spec := outside.Object["spec"].(map[string]any)
	name := outside.Object["metadata"].(map[string]any)["name"].(string)
	if spec["signer"] != false || spec["signerRef"] != name+"-root-signer" || spec["keyPolicy"] != nil || outside.Predicted != domain.BehaviourPrevious {
		t.Fatalf("role-absent arm: %+v", outside.Object)
	}
	if len(outside.consumers) != 2 || outside.consumers[0]["metadata"].(map[string]any)["name"] != name+"-root" {
		t.Fatalf("role-absent consumers: %+v", outside.consumers)
	}
	c := consumerCheckOf(*outside, cf, fx, "acme-ctl/reissue")
	if c.Verify.Secret != "core/v1/Secret/ri-verify/"+name+"-tls" || c.Verify.Key != "tls.crt" || len(c.Reissue) != 1 {
		t.Fatalf("role-absent check: %+v", c)
	}
	// an invalid declaration: outside values equal to the role's
	if _, err := ParseFixtures([]byte(badgeFixtures + strings.Replace(badgeOutside, "path: spec.signer, value: false", "path: spec.signer, value: true", 1))); err == nil ||
		!strings.Contains(err.Error(), "other than the role's") {
		t.Fatalf("outside = role accepted: %v", err)
	}
}

func TestRefinementExercise(t *testing.T) {
	const v, f, u = runtime.ConsumerVerified, runtime.ConsumerFailed, runtime.ConsumerUndetermined
	present, absent, outside := domain.ArmConditionPresent, domain.ArmConditionAbsent, domain.ArmConditionPresent+outsideRoleSuffix+"-1"
	for name, tc := range map[string]struct {
		res  map[string][2]string
		want domain.MethodOutcome
		why  string
	}{
		"confirmed: harm in the role only":            {map[string][2]string{present: {v, v}, absent: {v, f}, outside: {v, v}}, domain.MethodPass, "confined to the role"},
		"refuted: the role-absent consumer breaks":    {map[string][2]string{present: {v, v}, absent: {v, f}, outside: {v, f}}, domain.MethodFail, "does play it"},
		"refuted: the shielded consumer breaks":       {map[string][2]string{present: {v, f}, absent: {v, f}, outside: {v, v}}, domain.MethodFail, "does not shield"},
		"inconclusive: role-absent not observed":      {map[string][2]string{present: {v, v}, absent: {v, f}, outside: {u, u}}, domain.MethodInconclusive, "role-absent arm"},
		"inconclusive: no harm in the role either":    {map[string][2]string{present: {v, v}, absent: {v, v}, outside: {v, v}}, domain.MethodInconclusive, "still verifies at TO"},
		"inconclusive: role-absent never established": {map[string][2]string{present: {v, v}, absent: {v, f}, outside: {f, f}}, domain.MethodInconclusive, "role-absent arm"},
	} {
		run := &fakeRunner{res: consumerRun(tc.res)}
		m, lims := ExerciseRule(context.Background(), refinementInput(t, refinedRule(nil), run))
		if m.Outcome != tc.want || !strings.Contains(m.Detail, tc.why) || m.Producer != ConsumerExerciseProducer {
			t.Errorf("%s: %s %s", name, m.Outcome, m.Detail)
		}
		// no evidenced default for spec.signer: objects that omit it stay
		// undecidable (a limitation), the refinement is not refuted for it
		if !strings.Contains(strings.Join(lims, "|"), "leave the role field spec.signer unset stay undecidable") {
			t.Errorf("%s: limitations %v", name, lims)
		}
		// shielded, role-absent, exposed, and the shielded arm without
		// spec.signer (decidable: its keyPolicy shields it in any role) on
		// the role-absent observation; and the default probes of spec.signer
		// (PD-5, lane verify3): the exposed arm and the role-absent arm each
		// without spec.signer and with its other value, in their own
		// observation
		if len(run.spec.Consumers) != 8 || run.spec.Consumers[6].Trust.Key != "ca.crt" || len(run.spec.Consumers[6].Reissue) != 1 || len(run.spec.Consumers[4].Reissue) != 2 {
			t.Errorf("%s: consumer checks %+v", name, run.spec.Consumers)
		}
	}
	// the same exercise without a refinement keeps the earlier rule: the
	// undecidable omitted role field fails the boundary fixtures, and an
	// unobserved outside-role arm does not block a pass
	in := refinementInput(t, refinedRule(nil), &fakeRunner{res: consumerRun(map[string][2]string{present: {v, v}, absent: {v, f}})})
	in.Refinement = nil
	if m, _ := ExerciseRule(context.Background(), in); m.Outcome != domain.MethodFail || !strings.Contains(m.Detail, "boundary fixtures") {
		t.Errorf("plain consumer rule: %s %s", m.Outcome, m.Detail)
	}
	in.Rule = refinedRule(signerFieldDefault())
	in.Runner = &fakeRunner{res: consumerRun(map[string][2]string{present: {v, v}, absent: {v, f}})}
	if m, _ := ExerciseRule(context.Background(), in); m.Outcome != domain.MethodPass {
		t.Errorf("plain consumer rule with a default: %s %s", m.Outcome, m.Detail)
	}

	// with an evidenced default the omitted role field is exercised outside
	// the role, on the role-absent observation
	run := &fakeRunner{res: consumerRun(map[string][2]string{present: {v, v}, absent: {v, f}, outside: {v, v},
		present + omittedSuffix + outsideRoleSuffix + "-1": {v, v}, present + omittedSuffix + outsideRoleSuffix + "-2": {v, v}})}
	m, lims := ExerciseRule(context.Background(), refinementInput(t, refinedRule(signerFieldDefault()), run))
	if m.Outcome != domain.MethodPass || strings.Contains(strings.Join(lims, "|"), "stay undecidable") {
		t.Fatalf("with a default: %s %s %v", m.Outcome, m.Detail, lims)
	}
	objs := string(run.spec.Objects)
	if !strings.Contains(objs, "name: ri-arm-omit-1-root") || !strings.Contains(objs, "signerRef: ri-arm-omit-1-root-signer") {
		t.Fatalf("the omitted role-field arm does not use the role-absent observation:\n%s", objs)
	}

	// a refinement whose conditions select no (or another) consumer, or a
	// catalog without the role-absent observation, cannot be exercised
	in = refinementInput(t, refinedRule(nil), &fakeRunner{})
	in.Refinement = &domain.ScopeRefinement{Consumer: "other"}
	if m, _ := ExerciseRule(context.Background(), in); m.Outcome != domain.MethodNotRun {
		t.Errorf("another consumer: %s", m.Outcome)
	}
	in = refinementInput(t, refinedRule(nil), &fakeRunner{})
	in.Fixtures = badgeSet(t)
	if m, _ := ExerciseRule(context.Background(), in); m.Outcome != domain.MethodNotRun || !strings.Contains(m.Detail, "role-absent") {
		t.Errorf("no role-absent observation: %s %s", m.Outcome, m.Detail)
	}
}

// The resulting rule goes through the existing promotion gate unchanged: a
// confirmed proof of exactly its digest and a human decision; a proxy never
// promotes it, an inconclusive proof never opens the gate.
func TestRefinementPromotionGateRequiresAHuman(t *testing.T) {
	const v, f, u = runtime.ConsumerVerified, runtime.ConsumerFailed, runtime.ConsumerUndetermined
	present, absent, outside := domain.ArmConditionPresent, domain.ArmConditionAbsent, domain.ArmConditionPresent+outsideRoleSuffix+"-1"
	rule := refinedRule(nil)
	ks := knowledge.NewFileStore(t.TempDir())
	runs := t.TempDir()
	ev := cite("https://example.test/acme/blob/main/docs/release-notes-2.0.md", "The default value of spec.keyPolicy changed from Keep to Renew.",
		"# Release 2.0\nThe default value of spec.keyPolicy changed from Keep to Renew.")
	a := domain.ChangeAnchor{Release: "2.0.0", EvidenceKeys: []string{domain.EvidenceKey(ev)}, StatementKeys: []string{domain.StatementKey(ev)}, ChangeIDs: []string{"chg-key"}}
	members := []domain.CandidateMember{{ChangeID: "chg-key", Anchor: &a}}
	c := domain.SemanticCandidate{ID: domain.CandidateID("acme-ctl", "2.0.0", members), Product: "acme-ctl", Release: "2.0.0", Members: members,
		Grouping: "single", Category: domain.CategoryConfiguration, Title: "key policy default", Evidence: []domain.Evidence{ev},
		Producer: "semantic.candidates@v1", CreatedAt: t0}
	put(t, ks, c)
	at := t0.Add(time.Minute)
	p := domain.SemanticProposal{CandidateID: c.ID, Task: domain.TaskFull, Provider: "anthropic", Assertion: rule, Citations: []domain.EvidenceID{ev.ID},
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "formalize@v2", Rule: "hypothesis:" + hypID + "@" + invID, Confidence: domain.ConfidenceLow,
			Model: "claude-opus-5-5", ModelVersion: "claude-opus-5-5", PromptVersion: "formalize/v2", PromptDigest: "sha256:p", InputEvidence: []domain.EvidenceID{ev.ID},
			GeneratedAt: &at, CallID: "call-1", Provider: "anthropic"}}
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	put(t, ks, p)
	item := domain.ReviewItem{CandidateID: c.ID, Product: "acme-ctl", Release: "2.0.0", QuestionType: domain.QuestionFactReview, Question: "whole rule?",
		Proposed: rule, Proposals: []string{p.ID}, Routing: domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityHigh},
		Status: domain.ReviewPending, CreatedAt: t0.Add(2 * time.Minute)}
	item.ID = domain.ReviewItemID(item.CandidateID, item.QuestionType, item.Proposed)
	put(t, ks, item)
	h := map[string]any{"id": hypID, "mitigation": map[string]any{"item": "ci-1", "proposedRule": map[string]any{"assertion": rule, "release": bounds}},
		"proposedVerification": map[string]any{"assessment": "aas-1", "method": "consumer exercise (role signer-trust)", "refinement": refinement}}
	b, _ := json.Marshal(map[string]any{"id": invID, "hypotheses": []any{h}})
	if err := os.WriteFile(filepath.Join(runs, invID+".json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	opts := func(res map[string][2]string) Options {
		return Options{Knowledge: ks, Resolver: mapResolver{ev.ContentDigest: "# Release 2.0\nThe default value of spec.keyPolicy changed from Keep to Renew."},
			Runs: StateRuns{Dir: runs}, Runtime: true, Again: true, Fixtures: map[string]*FixtureSet{"acme-ctl": badgeOutsideSet(t)},
			Exercises: []runtime.DeclaredExercise{reissueExercise()}, Relationships: func(string) []catalog.Relationship { return badgeProduces },
			Renderer: &fakeRenderer{}, Runner: &fakeRunner{res: consumerRun(res)}, Now: func() time.Time { return t0.Add(time.Hour) }}
	}
	decide := func(kind domain.ReviewerKind, when time.Time) error {
		orig := item.Proposed
		d := domain.ReviewDecision{ReviewItemID: item.ID, Action: domain.ActionAccept, Labels: []domain.FeedbackLabel{domain.LabelAccepted}, Original: &orig,
			Reviewer: "product-owner", ReviewerKind: kind, StartedAt: when.Add(-time.Minute), DecidedAt: when}
		if kind == domain.ReviewerProxy {
			d.ProxyProvenance = &domain.Provenance{Method: domain.MethodAI, Producer: "proxy@v1", Confidence: domain.ConfidenceLow, Model: "m", ModelVersion: "m",
				PromptVersion: "p", PromptDigest: "sha256:p", InputEvidence: []domain.EvidenceID{ev.ID}, GeneratedAt: &when, CallID: "c", Provider: "anthropic"}
		}
		d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
		_, err := knowledge.NewQueue(ks, nil).Decide(context.Background(), []domain.ReviewDecision{d})
		return err
	}

	// role-absent arm unobserved: inconclusive, the gate stays closed
	rs, err := Hypotheses(context.Background(), opts(map[string][2]string{present: {v, v}, absent: {v, f}, outside: {u, u}}))
	if err != nil || len(rs) != 1 || rs[0].Proof == nil || rs[0].Proof.Outcome != domain.ProofInconclusive {
		t.Fatalf("results %+v %v", rs, err)
	}
	if !strings.Contains(strings.Join(rs[0].Proof.Limitations, "|"), "scope refinement") {
		t.Errorf("limitations %v", rs[0].Proof.Limitations)
	}
	if err := decide(domain.ReviewerHuman, t0.Add(2*time.Hour)); err == nil {
		t.Fatal("an inconclusive refinement was promoted")
	}
	// observed: confirmed; a proxy cannot promote it, a human can
	rs, err = Hypotheses(context.Background(), opts(map[string][2]string{present: {v, v}, absent: {v, f}, outside: {v, v}}))
	if err != nil || len(rs) != 1 || rs[0].Proof == nil || rs[0].Proof.Outcome != domain.ProofConfirmed {
		t.Fatalf("results %+v %v", rs, err)
	}
	if err := decide(domain.ReviewerProxy, t0.Add(3*time.Hour)); err == nil {
		t.Fatal("a proxy promoted a refinement")
	}
	if err := decide(domain.ReviewerHuman, t0.Add(3*time.Hour)); err != nil {
		t.Fatalf("a human promotion with a confirmed proof was refused: %v", err)
	}
}

// Stored proofs are not re-judged: the Phase 5 consumer-observation proofs
// (real kind runs, producer verify.exercise-consumer@v1) still decode,
// validate and re-derive their ids, with no arm classified after the fact.
func TestStoredConsumerProofsStillVerify(t *testing.T) {
	paths, err := filepath.Glob("../../../docs/phase5/learning-loop/runs/verify2-smoke/proofs/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no stored proofs: %v", err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var rec domain.KnowledgeRecord
		if err := json.Unmarshal(b, &rec); err != nil || rec.Proof == nil {
			t.Fatalf("%s: %v", p, err)
		}
		if err := rec.Proof.Validate(); err != nil || rec.Proof.ID != domain.ProofRecordID(*rec.Proof) {
			t.Fatalf("%s: proof %s no longer verifies: %v", p, rec.Proof.ID, err)
		}
		consumer := false
		for _, m := range rec.Proof.Methods {
			for _, a := range m.Arms {
				consumer = consumer || a.Consumer != nil
				if a.NonDiscriminating != "" {
					t.Fatalf("%s: a stored arm decodes as non-discriminating", p)
				}
			}
		}
		if !consumer {
			t.Fatalf("%s: not a consumer-observation proof", p)
		}
	}
}
