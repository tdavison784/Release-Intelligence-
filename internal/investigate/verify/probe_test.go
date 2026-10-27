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
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// Observed defaults (PD-5, lane verify3, D-0026) on the synthetic Badge
// product: the refinement compares spec.signer, which it states no default
// for. Default probe 1 is the exposed signer arm (condition-absent) without
// spec.signer and with spec.signer = false, in the role's observation;
// probe 2 is the role-absent arm without spec.signer and with spec.signer =
// true, in the role-absent observation.

const (
	pv, pf, pu = runtime.ConsumerVerified, runtime.ConsumerFailed, runtime.ConsumerUndetermined
	probe1u    = probePrefix + "1" + probeUnsetSuffix
	probe1o    = probePrefix + "1" + probeOtherSuffix
	probe2u    = probePrefix + "2" + probeUnsetSuffix
	probe2o    = probePrefix + "2" + probeOtherSuffix
)

var roleAbsentArm = domain.ArmConditionPresent + outsideRoleSuffix + "-1"

// refinementRun is a consumer run: the principal arms confirm the
// refinement (unless overridden) and the probes observe probes.
func refinementRun(probes map[string][2]string, override map[string][2]string) *runtime.FixtureResult {
	res := map[string][2]string{domain.ArmConditionPresent: {pv, pv}, domain.ArmConditionAbsent: {pv, pf}, roleAbsentArm: {pv, pv},
		domain.ArmConditionPresent + omittedSuffix + outsideRoleSuffix + "-1": {pv, pv}}
	for k, v := range probes {
		res[k] = v
	}
	for k, v := range override {
		res[k] = v
	}
	return consumerRun(res)
}

// an unset spec.signer behaves as false in both observations, and the two
// explicit values behave differently in each
var probesFalse = map[string][2]string{probe1u: {pv, pv}, probe1o: {pv, pv}, probe2u: {pv, pv}, probe2o: {pv, pf}}

func TestDefaultProbesBuilt(t *testing.T) {
	fx, _ := badgeOutsideSet(t).Subject("acme.example", "Badge")
	cf, _ := consumerFor(refinedRule(nil), fx)
	arms, _, why := BuildConsumerArms(refinedRule(nil), fx, cf)
	if why != "" {
		t.Fatal(why)
	}
	probes, lims := DefaultProbes(refinedRule(nil), fx, arms, cf, nil)
	if len(probes) != 4 || len(lims) != 0 {
		t.Fatalf("probes %d, limitations %v", len(probes), lims)
	}
	want := []struct {
		name, base string
		signer     any
		outside    bool
	}{{probe1u, domain.ArmConditionAbsent, nil, false}, {probe1o, domain.ArmConditionAbsent, false, false},
		{probe2u, roleAbsentArm, nil, true}, {probe2o, roleAbsentArm, true, true}}
	for i, w := range want {
		p := probes[i]
		spec := p.Object["spec"].(map[string]any)
		if p.Name != w.name || p.Probe == nil || p.Probe.Base != w.base || p.Probe.Field != "spec.signer" || spec["signer"] != w.signer ||
			p.outside != w.outside || p.Predicted != domain.BehaviourUndetermined {
			t.Errorf("probe %d: %+v %+v", i, p.Arm, p.Probe)
		}
		// the base arm's observation: the role-absent arm's subject fields stay
		if w.outside && !strings.HasSuffix(spec["signerRef"].(string), "-root-signer") {
			t.Errorf("probe %s lost the role-absent observation: %+v", p.Name, spec)
		}
	}
	// with an evidenced default there is nothing to observe
	if probes, _ := DefaultProbes(refinedRule(signerFieldDefault()), fx, arms, cf, rulePool([]domain.Evidence{signerEvidence})); len(probes) != 0 {
		t.Fatalf("probes despite an evidenced default: %d", len(probes))
	}
}

func TestObservedDefaultRecordedOnlyFromAConfirmedDistinguishingObservation(t *testing.T) {
	rule := refinedRule(nil)
	in := func(res *runtime.FixtureResult) ExerciseInput {
		return refinementInput(t, rule, &fakeRunner{res: res})
	}
	derive := func(res *runtime.FixtureResult) (domain.ProofMethod, []domain.ObservedDefault, []string) {
		m, _ := ExerciseRule(context.Background(), in(res))
		ods, notes := ObservedDefaults(rule, m, ObservationFixture{Bounds: bounds, Fixtures: badgeOutsideSet(t)})
		return m, ods, notes
	}

	// both observations distinguish the values: unset behaves as false
	m, ods, notes := derive(refinementRun(probesFalse, nil))
	if m.Outcome != domain.MethodPass || len(ods) != 1 || len(notes) != 0 {
		t.Fatalf("distinguishing: %s %s %+v %v", m.Outcome, m.Detail, ods, notes)
	}
	d := ods[0]
	if d.Path != "spec.signer" || d.Value != "false" || d.Kind != "Badge" || d.Group != "acme.example" || d.Level != domain.LevelObserved ||
		d.Limitation != domain.ObservedDefaultLimitation || strings.Join(d.Versions, ",") != "1.9.4,2.0.0" || d.Fixture == "" || len(d.Evidence) == 0 ||
		d.RuleDigest != rule.Digest() || len(d.Arms) != 6 || d.Validate() != nil {
		t.Fatalf("observed default %+v (%v)", d, d.Validate())
	}
	// the probes never decide the method: a probe that fails at TO does not refute
	for _, a := range m.Arms {
		if isProbe(a.Name) && a.Probe == nil {
			t.Errorf("probe arm %s lost its probe", a.Name)
		}
	}

	// the explicit values behave alike: the observation shows nothing
	same := map[string][2]string{probe1u: {pv, pv}, probe1o: {pv, pf}, probe2u: {pv, pv}, probe2o: {pv, pv}}
	if _, ods, notes := derive(refinementRun(same, nil)); len(ods) != 0 || !strings.Contains(strings.Join(notes, "|"), "does not distinguish") {
		t.Fatalf("non-distinguishing: %+v %v", ods, notes)
	}
	// the observations contradict each other: nothing
	contra := map[string][2]string{probe1u: {pv, pf}, probe1o: {pv, pv}, probe2u: {pv, pv}, probe2o: {pv, pf}}
	if _, ods, notes := derive(refinementRun(contra, nil)); len(ods) != 0 || !strings.Contains(strings.Join(notes, "|"), "contradict") {
		t.Fatalf("contradicting: %+v %v", ods, notes)
	}
	// an unobserved probe leaves its observation out
	open := map[string][2]string{probe1u: {pu, pu}, probe1o: {pv, pv}, probe2u: {pv, pv}, probe2o: {pv, pf}}
	if m, ods, notes := derive(refinementRun(open, nil)); m.Outcome != domain.MethodPass || len(ods) != 1 || len(ods[0].Arms) != 3 ||
		!strings.Contains(strings.Join(notes, "|"), "not every arm was observed") {
		t.Fatalf("one probe unobserved: %s %+v %v", m.Outcome, ods, notes)
	}
}

// proofStore is a knowledge store with the refined rule's hypothesis item
// and a run record naming the refinement (as TestRefinementPromotionGateRequiresAHuman).
type proofStore struct {
	ks   knowledge.Store
	dir  string // the file store's directory
	item domain.ReviewItem
	opts func(*runtime.FixtureResult, time.Time) Options
}

func newProofStore(t *testing.T, rule domain.SemanticAssertion) *proofStore {
	t.Helper()
	dir := t.TempDir()
	ks := knowledge.NewFileStore(dir)
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
	return &proofStore{ks: ks, dir: dir, item: item, opts: func(res *runtime.FixtureResult, now time.Time) Options {
		return Options{Knowledge: ks, Resolver: mapResolver{ev.ContentDigest: "# Release 2.0\nThe default value of spec.keyPolicy changed from Keep to Renew."},
			Runs: StateRuns{Dir: runs}, Runtime: true, Again: true, Fixtures: map[string]*FixtureSet{"acme-ctl": badgeOutsideSet(t)},
			Exercises: []runtime.DeclaredExercise{reissueExercise()}, Relationships: func(string) []catalog.Relationship { return badgeProduces },
			Renderer: &fakeRenderer{}, Runner: &fakeRunner{res: res}, Now: func() time.Time { return now }}
	}}
}

func (s *proofStore) verify(t *testing.T, res *runtime.FixtureResult, now time.Time) *domain.ProofRecord {
	t.Helper()
	rs, err := Hypotheses(context.Background(), s.opts(res, now))
	if err != nil || len(rs) != 1 || rs[0].Proof == nil {
		t.Fatalf("results %+v %v", rs, err)
	}
	return rs[0].Proof
}

func (s *proofStore) promote(t *testing.T, when time.Time) error {
	orig := s.item.Proposed
	d := domain.ReviewDecision{ReviewItemID: s.item.ID, Action: domain.ActionAccept, Labels: []domain.FeedbackLabel{domain.LabelAccepted}, Original: &orig,
		Reviewer: "product-owner", ReviewerKind: domain.ReviewerHuman, StartedAt: when.Add(-time.Minute), DecidedAt: when}
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	_, err := knowledge.NewQueue(s.ks, nil).Decide(context.Background(), []domain.ReviewDecision{d})
	return err
}

// The whole path: an inconclusive or refuted proof records no observed
// default; a confirmed one does, and a human promotion carries it into the
// fact, which the impact layer then accepts through the proof — and ignores
// when it names a proof that is not a confirmed proof of the same rule.
func TestObservedDefaultThroughPromotion(t *testing.T) {
	s := newProofStore(t, refinedRule(nil))

	// inconclusive (no role-absent arm is observed): none recorded
	p := s.verify(t, refinementRun(probesFalse, map[string][2]string{roleAbsentArm: {pu, pu},
		domain.ArmConditionPresent + omittedSuffix + outsideRoleSuffix + "-1": {pu, pu}}), t0.Add(time.Hour))
	if p.Outcome != domain.ProofInconclusive || len(p.ObservedDefaults) != 0 ||
		!strings.Contains(strings.Join(p.Limitations, "|"), "so no observed default is recorded") {
		t.Fatalf("inconclusive: %s %+v %v", p.Outcome, p.ObservedDefaults, p.Limitations)
	}
	inconclusive := *p
	// refuted (the shielded consumer breaks): none recorded
	p = s.verify(t, refinementRun(probesFalse, map[string][2]string{domain.ArmConditionPresent: {pv, pf}}), t0.Add(2*time.Hour))
	if p.Outcome != domain.ProofRefuted || len(p.ObservedDefaults) != 0 {
		t.Fatalf("refuted: %s %+v", p.Outcome, p.ObservedDefaults)
	}

	// a refuted rule can never be promoted: start again with a fresh store
	s = newProofStore(t, refinedRule(nil))
	p = s.verify(t, refinementRun(probesFalse, nil), t0.Add(time.Hour))
	if p.Outcome != domain.ProofConfirmed || len(p.ObservedDefaults) != 1 || p.ObservedDefaults[0].Value != "false" || p.Validate() != nil {
		t.Fatalf("confirmed: %s %+v %v", p.Outcome, p.ObservedDefaults, p.Validate())
	}
	if !strings.Contains(strings.Join(p.Limitations, "|"), "decided by the default spec.signer = false observed by this proof") {
		t.Errorf("limitations %v", p.Limitations)
	}
	confirmed := *p
	// a tampered proof (an observed default added afterwards) no longer validates its id
	tampered := confirmed
	tampered.ObservedDefaults = append([]domain.ObservedDefault(nil), confirmed.ObservedDefaults...)
	tampered.ObservedDefaults[0].Value = "true"
	if tampered.Validate() == nil {
		t.Error("a proof whose observed default was edited still validates")
	}

	if err := s.promote(t, t0.Add(3*time.Hour)); err != nil {
		t.Fatalf("promotion refused: %v", err)
	}
	snap, err := s.ks.Load(context.Background(), knowledge.Query{})
	if err != nil {
		t.Fatal(err)
	}
	var fact *domain.VerifiedFact
	for i := range snap.Facts {
		if snap.Facts[i].Assertion.Digest() == refinedRule(nil).Digest() {
			fact = &snap.Facts[i]
		}
	}
	if fact == nil || len(fact.ObservedDefaults) != 1 {
		t.Fatalf("the promoted fact does not carry the observed default: %+v", fact)
	}
	od := fact.ObservedDefaults[0]
	if od.Proof != confirmed.ID || od.Limitation != domain.ObservedDefaultLimitation || od.Value != "false" {
		t.Fatalf("carried default %+v", od)
	}
	facts, rejected := impact.VerifyFacts(snap)
	if len(facts) != 1 || len(facts[0].ObservedDefaults) != 1 || len(rejected) != 0 {
		t.Fatalf("verified facts %d (%+v), rejected %v", len(facts), facts, rejected)
	}

	// the same default naming the inconclusive proof, no proof, or carried
	// onto another rule is ignored: the fact stays, its field stays unknown
	for name, mutate := range map[string]func(*domain.ObservedDefault){
		"inconclusive proof": func(d *domain.ObservedDefault) { d.Proof = inconclusive.ID },
		"merely proposed":    func(d *domain.ObservedDefault) { d.Proof = "" },
		"another rule":       func(d *domain.ObservedDefault) { d.RuleDigest = "sha256:other" },
		"not recorded":       func(d *domain.ObservedDefault) { d.Value = "true" },
	} {
		bad := *snap
		f := *fact
		f.ObservedDefaults = append([]domain.ObservedDefault(nil), fact.ObservedDefaults...)
		mutate(&f.ObservedDefaults[0])
		bad.Facts = []domain.VerifiedFact{f}
		bad.Proofs = append(append([]domain.ProofRecord(nil), snap.Proofs...), inconclusive)
		facts, rejected := impact.VerifyFacts(&bad)
		if len(facts) != 1 || len(facts[0].ObservedDefaults) != 0 || !strings.Contains(strings.Join(rejected, "|"), "observed default acme.example/Badge/spec.signer = ") {
			t.Errorf("%s: facts %d, defaults kept %v, rejected %v", name, len(facts), facts, rejected)
		}
	}
}
