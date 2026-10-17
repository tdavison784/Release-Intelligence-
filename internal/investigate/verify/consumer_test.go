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

// A synthetic product whose Badges are certificates: a Badge with
// spec.signer: true signs other Badges; spec.keyPolicy's default changes from
// Keep to Renew (a new key on every re-issuance).
const badgeFixtures = `apiVersion: ri.dev/v1alpha1
kind: ProductFixtures
product: acme-ctl
subjects:
- id: badge
  target: {group: acme.example, version: v1, kind: Badge}
  exercise: reissue
  namespace: ri-verify
  prerequisites:
  - {apiVersion: acme.example/v1, kind: Signer, metadata: {name: self}, spec: {self: {}}}
  template:
    apiVersion: acme.example/v1
    kind: Badge
    metadata: {name: placeholder}
    spec: {secretName: "{{name}}-tls", signerRef: self}
  references:
  - {url: "https://example.test/acme/docs/badge.md", quote: "a Badge is a certificate"}
  consumers:
  - id: signer-trust
    role: [{path: spec.signer, value: true}]
    objects:
    - {apiVersion: acme.example/v1, kind: Signer, metadata: {name: "{{name}}-signer"}, spec: {badge: "{{name}}"}}
    - {apiVersion: acme.example/v1, kind: Badge, metadata: {name: "{{name}}-leaf"}, spec: {secretName: "{{name}}-leaf-tls", signerRef: "{{name}}-signer"}}
    trust: {secret: "{{name}}-tls", key: ca.crt}
    reissue: ["{{name}}", "{{name}}-leaf"]
    verify: {secret: "{{name}}-leaf-tls", key: tls.crt}
    references:
    - {url: "https://example.test/acme/docs/signer.md", quote: "a signer Badge signs others"}
`

func badgeSet(t *testing.T) *FixtureSet {
	t.Helper()
	s, err := ParseFixtures([]byte(badgeFixtures))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func field(path string, state domain.FieldState, values ...string) domain.Condition {
	return domain.Condition{Op: domain.OpField, Path: path, State: state, Values: values}
}

func badges(of ...domain.Condition) domain.Condition {
	return domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Badge", Of: of}
}

// signerScopedRule: only a signer Badge that leaves keyPolicy unset changes
// observably; every other Badge is shielded.
func signerScopedRule() domain.SemanticAssertion {
	overlap := badges(domain.Condition{Op: domain.OpAny, Of: []domain.Condition{
		field("spec.keyPolicy", domain.StateSet), field("spec.signer", domain.StateUnset), field("spec.signer", domain.StateEquals, "false")}})
	return domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectCRDField, Product: "acme-ctl", Group: "acme.example", Kind: "Badge", Path: "spec.keyPolicy"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"Keep"`), After: str(`"Renew"`)},
		Applicability: &domain.Applicability{Exposure: badges(field("spec.keyPolicy", domain.StateUnset), field("spec.signer", domain.StateEquals, "true")),
			Overlap: &overlap},
		Consequence: &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired},
		Statement:   "Only a signer Badge that leaves keyPolicy unset changes observably.",
	}
}

// wrongRule declares the signer with keyPolicy unset shielded.
func wrongRule() domain.SemanticAssertion {
	r := signerScopedRule()
	overlap := badges(domain.Condition{Op: domain.OpAny, Of: []domain.Condition{field("spec.keyPolicy", domain.StateSet), field("spec.signer", domain.StateEquals, "true")}})
	r.Applicability = &domain.Applicability{Exposure: badges(field("spec.keyPolicy", domain.StateUnset), field("spec.signer", domain.StateUnset)), Overlap: &overlap}
	return r
}

func reissueExercise() runtime.DeclaredExercise {
	return runtime.DeclaredExercise{Product: "acme-ctl", Exercise: catalog.Exercise{ID: "reissue", Summary: "re-issue every Badge",
		Target: catalog.ObjectType{Group: "acme.example", Kind: "Badge"}, Action: catalog.ActionStatusCondition,
		Condition: &catalog.ExerciseCondition{Type: "Issuing", Status: "True", Reason: "Manual"},
		Reaction:  &catalog.ExerciseReaction{Kind: catalog.ReactionFieldChanged, Path: "status.revision"}, Families: []string{"behavior"},
		References: []catalog.ConfigReference{{URL: "https://example.test/acme/docs/reissue.md", Quote: "re-issue"}}}}
}

var badgeProduces = []catalog.Relationship{{ID: "badge-secret", Kind: catalog.RelProduces, From: catalog.ObjectType{Group: "acme.example", Kind: "Badge"},
	To: catalog.ObjectType{Kind: "Secret"}, Path: "spec.secretName"}}

func TestBuildConsumerArmsFromTheRule(t *testing.T) {
	fx, _ := badgeSet(t).Subject("acme.example", "Badge")
	cf, why := consumerFor(signerScopedRule(), fx)
	if cf == nil || why != "" {
		t.Fatalf("consumer observation not selected: %q", why)
	}
	arms, lims, why := BuildConsumerArms(signerScopedRule(), fx, cf)
	if why != "" || len(arms) != 2 {
		t.Fatalf("arms %+v why %q", arms, why)
	}
	spec := func(a consumerArm) map[string]any { return a.Object["spec"].(map[string]any) }
	present, absent := arms[0], arms[1]
	if present.Name != domain.ArmConditionPresent || spec(present)["keyPolicy"] != "Keep" || spec(present)["signer"] != true || present.Predicted != domain.BehaviourPrevious {
		t.Fatalf("shielded arm: %+v", present.Arm)
	}
	if _, set := spec(absent)["keyPolicy"]; set || absent.Name != domain.ArmConditionAbsent || spec(absent)["signer"] != true || absent.Predicted != domain.BehaviourNew {
		t.Fatalf("exposed arm: %+v", absent.Arm)
	}
	leaf := absent.consumers[1]
	if leaf["metadata"].(map[string]any)["name"] != "ri-arm-absent-leaf" || leaf["spec"].(map[string]any)["signerRef"] != "ri-arm-absent-signer" {
		t.Fatalf("consuming objects: %+v", absent.consumers)
	}
	// the two non-signer alternatives cannot hold in the signer role: listed, not exercised
	if len(lims) != 2 || !strings.Contains(lims[0], "spec.signer unset") {
		t.Fatalf("limitations: %v", lims)
	}
	c := consumerCheckOf(absent, cf, fx, "acme-ctl/reissue")
	if c.Trust.Secret != "core/v1/Secret/ri-verify/ri-arm-absent-tls" || c.Verify.Secret != "core/v1/Secret/ri-verify/ri-arm-absent-leaf-tls" ||
		strings.Join(c.Reissue, ",") != "acme.example/v1/Badge/ri-verify/ri-arm-absent,acme.example/v1/Badge/ri-verify/ri-arm-absent-leaf" {
		t.Fatalf("check: %+v", c)
	}

	// the deliberately wrong rule: both of its shielded alternatives are
	// signers (one keeps its key, one does not); no exposed signer exists
	arms, lims, why = BuildConsumerArms(wrongRule(), fx, cf)
	if why != "" || len(arms) != 2 || arms[0].Name != domain.ArmConditionPresent || arms[1].Name != domain.ArmConditionPresent+"-2" {
		t.Fatalf("wrong rule: arms %+v why %q", arms, why)
	}
	if _, set := spec(arms[1])["keyPolicy"]; set || spec(arms[1])["signer"] != true {
		t.Fatalf("wrong rule: the second shielded arm must be a signer with keyPolicy unset: %+v", arms[1].Arm)
	}
	if !strings.Contains(strings.Join(lims, "|"), "refuted by this exercise but not confirmed") {
		t.Fatalf("wrong rule: limitations %v", lims)
	}

	// a rule that names no role field keeps the produced-object observation
	plain := signerScopedRule()
	plain.Applicability = &domain.Applicability{Exposure: badges(field("spec.keyPolicy", domain.StateUnset))}
	if cf, why := consumerFor(plain, fx); cf != nil || why != "" {
		t.Fatalf("plain rule selected %v %q", cf, why)
	}
}

// consumerRun is a fixture run with one consumer result per arm and phase
// ("" = no result recorded).
func consumerRun(outcomes map[string][2]string) *runtime.FixtureResult {
	r := &runtime.Run{Product: "acme-ctl", From: "1.9.4", To: "2.0.0", Cluster: "ri-verify-00000000", Status: runtime.RunCompleted, UpgradeCompleted: true}
	for arm, o := range outcomes {
		for i, phase := range []string{"from", "to"} {
			if o[i] == "" {
				continue
			}
			c := runtime.ConsumerResult{Check: arm, Phase: phase, Outcome: o[i], Detail: o[i], Certificate: "sha256:c", CertificateKey: "sha256:k"}
			if o[i] != runtime.ConsumerUndetermined {
				c.Reissued = []string{"a", "b"}
			}
			r.Consumers = append(r.Consumers, c)
		}
	}
	return &runtime.FixtureResult{Run: r, Artifacts: []domain.ArtifactRef{{ID: "art-1", Path: "derived/runtime/fx/run.json", URI: "ri:runtime/run.json", Digest: "sha256:run"}}}
}

func consumerPlans(names ...string) []armPlan {
	var out []armPlan
	for _, n := range names {
		pred := domain.BehaviourNew
		if shielded(n) {
			pred = domain.BehaviourPrevious
		}
		c := runtime.ConsumerCheck{ID: n, Reissue: []string{"a", "b"}, Trust: runtime.SecretKey{Secret: "s", Key: "ca.crt"}, Verify: runtime.SecretKey{Secret: "l", Key: "tls.crt"}}
		out = append(out, armPlan{Arm: Arm{Name: n, Predicted: pred, Condition: n}, consumer: &c})
	}
	return out
}

func TestEvaluateConsumerArms(t *testing.T) {
	const v, f, u = runtime.ConsumerVerified, runtime.ConsumerFailed, runtime.ConsumerUndetermined
	present, absent, present2 := domain.ArmConditionPresent, domain.ArmConditionAbsent, domain.ArmConditionPresent+"-2"
	for name, tc := range map[string]struct {
		arms []string
		res  map[string][2]string
		want domain.MethodOutcome
		why  string
	}{
		"confirmed":                         {[]string{present, absent}, map[string][2]string{present: {v, v}, absent: {v, f}}, domain.MethodPass, "fails at TO"},
		"exposed arm quiet ⇒ not confirmed": {[]string{present, absent}, map[string][2]string{present: {v, v}, absent: {v, v}}, domain.MethodInconclusive, "still verifies at TO"},
		"shielded arm fails at TO":          {[]string{present, absent}, map[string][2]string{present: {v, f}, absent: {v, f}}, domain.MethodFail, "does not shield"},
		"shielded arm fails at FROM":        {[]string{present, absent}, map[string][2]string{present: {f, v}, absent: {v, f}}, domain.MethodFail, "does not shield"},
		"shielded fails, other undecided":   {[]string{present, present2, absent}, map[string][2]string{present: {v, f}, present2: {u, u}, absent: {v, f}}, domain.MethodFail, "does not shield"},
		"undetermined arm":                  {[]string{present, absent}, map[string][2]string{present: {v, u}, absent: {v, f}}, domain.MethodInconclusive, "quiet window"},
		"missing result":                    {[]string{present, absent}, map[string][2]string{present: {v, v}, absent: {v, ""}}, domain.MethodInconclusive, "quiet window"},
		"exposed arm fails at FROM":         {[]string{present, absent}, map[string][2]string{present: {v, v}, absent: {f, f}}, domain.MethodInconclusive, "no baseline"},
		"no exposed arm":                    {[]string{present, present2}, map[string][2]string{present: {v, v}, present2: {v, v}}, domain.MethodInconclusive, "no exposed arm"},
		"wrong rule refuted":                {[]string{present, present2}, map[string][2]string{present: {v, v}, present2: {v, f}}, domain.MethodFail, "does not shield"},
	} {
		m := EvaluateConsumerArms(consumerRun(tc.res), consumerPlans(tc.arms...))
		if m.Outcome != tc.want || !strings.Contains(m.Detail, tc.why) {
			t.Errorf("%s: %s %s", name, m.Outcome, m.Detail)
		}
		for _, a := range m.Arms {
			if a.Consumer == nil {
				t.Errorf("%s: arm %s carries no consumer observation", name, a.Name)
			}
		}
		if m.Outcome == domain.MethodPass {
			p := domain.ProofRecord{Hypothesis: hypID, ReviewItem: "ri-x", CandidateID: "sc-x", Product: "acme-ctl", ProposedRuleDigest: "d",
				Methods: []domain.ProofMethod{{Method: domain.ProofCitationCheck, Producer: CitationsProducer, Level: domain.LevelDeterministic, Outcome: domain.MethodPass,
					Detail: "ok", Evidence: []domain.Evidence{{ID: "ev-1"}}}, m}, VersionBounds: bounds, Limitations: []string{}, Level: domain.LevelObserved,
				Producer: Producer, CheckedAt: t0}
			p.Outcome = domain.ProofOutcomeOf(p.Methods)
			p.ID = domain.ProofRecordID(p)
			if err := p.Validate(); err != nil || p.Outcome != domain.ProofConfirmed {
				t.Errorf("%s: a passing consumer exercise does not make a valid confirmed proof: %v", name, err)
			}
		}
	}
	// a broken run is never decided by its consumer results
	broken := consumerRun(map[string][2]string{present: {v, v}, absent: {v, f}})
	broken.Run.UpgradeCompleted = false
	if m := EvaluateConsumerArms(broken, consumerPlans(present, absent)); m.Outcome != domain.MethodInconclusive {
		t.Errorf("upgrade incomplete: %s", m.Outcome)
	}
}

const ruleFile = `{
  "_provenance": "synthetic test rule",
  "statement": "Only a signer Badge that leaves keyPolicy unset changes observably.",
  "assertion": {
    "subject": {"family": "crd-field", "product": "acme-ctl", "group": "acme.example", "kind": "Badge", "path": "spec.keyPolicy"},
    "change": {"type": "default-changed", "before": "\"Keep\"", "after": "\"Renew\""},
    "applicability": {
      "exposure": {"op": "resource", "group": "acme.example", "kind": "Badge", "of": [
        {"op": "field", "path": "spec.keyPolicy", "state": "unset"},
        {"op": "field", "path": "spec.signer", "state": "equals", "values": ["true"]}]},
      "overlap": {"op": "resource", "group": "acme.example", "kind": "Badge", "of": [
        {"op": "any", "of": [
          {"op": "field", "path": "spec.keyPolicy", "state": "set"},
          {"op": "field", "path": "spec.signer", "state": "unset"},
          {"op": "field", "path": "spec.signer", "state": "equals", "values": ["false"]}]}]}
    },
    "consequence": {"kind": "behavior-change", "exposedClass": "review-required"},
    "release": {"from": "1.9.4", "to": "2.0.0", "bounds": "(from, to]"}
  },
  "citations": ["EVID (release-notes-2.0.md)"],
  "requestedVerification": "consumer observation, shielded and exposed arms"
}`

// The pre-registered path end to end on synthetic data: a human-authored
// rule file is taken in (proposal + whole-fact item + digest), verified by
// the consumer observation with the bounds the file states, and only then
// can a human promote it; the same rule with the signer declared shielded is
// refuted and can never be promoted.
func TestHumanAuthoredRuleVerifiedByConsumerObservation(t *testing.T) {
	w := newWorld(t, widgetRule(), bounds) // a store with a candidate citing the 2.0 notes
	cands, _ := w.ks.Load(context.Background(), knowledge.Query{Kinds: []domain.RecordKind{domain.RecordCandidate}})
	cand := cands.Candidates[0]
	notes := "# Release 2.0\nThe default value of spec.keyPolicy changed from Keep to Renew."
	ev := cite("https://example.test/acme/blob/main/docs/release-notes-2.0.md", "The default value of spec.keyPolicy changed from Keep to Renew.", notes)
	w.res[ev.ContentDigest] = notes
	if _, err := knowledge.ExtendCandidateEvidence(context.Background(), w.ks, cand.ID, []domain.Evidence{ev}); err != nil {
		t.Fatal(err)
	}
	data := []byte(strings.Replace(ruleFile, "EVID", string(ev.ID), 1))
	r, err := knowledge.ParseRuleFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if r.Assertion.Digest() != signerScopedRule().Digest() {
		t.Fatal("the rule file is not encoded as the rule it states")
	}
	res, err := knowledge.Intake(context.Background(), w.ks, r, knowledge.IntakeOptions{Author: "commander-drafted (test), label-aware", Now: t0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Candidate != cand.ID || res.FileDigest != domain.Digest(data) || res.RuleDigest != r.Assertion.Digest() || res.Status != knowledge.IntakeTaken {
		t.Fatalf("intake: %+v", res)
	}
	snap, _ := w.ks.Load(context.Background(), knowledge.Query{})
	var item domain.ReviewItem
	for _, it := range snap.ReviewItems {
		if it.ID == res.ReviewItem {
			item = it
		}
	}
	if hyp, inv, ok := knowledge.HypothesisSource(snap, item); !ok || hyp != res.Hypothesis || inv != "" {
		t.Fatalf("the item is not a hypothesis-sourced item: %q %q %v", hyp, inv, ok)
	}

	o := w.options(t, true, consumerRun(map[string][2]string{domain.ArmConditionPresent: {runtime.ConsumerVerified, runtime.ConsumerVerified},
		domain.ArmConditionAbsent: {runtime.ConsumerVerified, runtime.ConsumerFailed}}))
	o.Fixtures = map[string]*FixtureSet{"acme-ctl": badgeSet(t)}
	o.Exercises = []runtime.DeclaredExercise{reissueExercise()}
	o.Relationships = func(string) []catalog.Relationship { return badgeProduces }
	runner := o.Runner.(*fakeRunner)
	rs, err := Hypotheses(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	var p *domain.ProofRecord
	for _, x := range rs {
		if x.Item == res.ReviewItem {
			p = x.Proof
		}
	}
	if p == nil || p.Outcome != domain.ProofConfirmed || p.Hypothesis != res.Hypothesis || p.Investigation != "" ||
		p.VersionBounds == nil || *p.VersionBounds != *bounds || p.ProposedRuleDigest != res.RuleDigest {
		t.Fatalf("proof: %+v (results %+v)", p, rs)
	}
	if len(runner.spec.Consumers) != 2 || !strings.Contains(string(runner.spec.Objects), "name: ri-arm-absent-leaf") {
		t.Fatalf("fixture spec: %+v\n%s", runner.spec.Consumers, runner.spec.Objects)
	}
	b, _ := json.Marshal(p)
	if !strings.Contains(string(b), `"consumer"`) {
		t.Fatalf("the proof does not record the consumer observation: %s", b)
	}
	decide := func(it domain.ReviewItem, kind domain.ReviewerKind, at time.Time) error {
		orig := it.Proposed
		d := domain.ReviewDecision{ReviewItemID: it.ID, Action: domain.ActionAccept, Labels: []domain.FeedbackLabel{domain.LabelAccepted}, Original: &orig,
			Reviewer: "product-owner", ReviewerKind: kind, StartedAt: at.Add(-time.Minute), DecidedAt: at}
		if kind == domain.ReviewerProxy {
			at2 := at
			d.ProxyProvenance = &domain.Provenance{Method: domain.MethodAI, Producer: "proxy@v1", Confidence: domain.ConfidenceLow, Model: "m", ModelVersion: "m",
				PromptVersion: "p", PromptDigest: "sha256:p", InputEvidence: []domain.EvidenceID{w.ev.ID}, GeneratedAt: &at2, CallID: "c", Provider: "anthropic"}
		}
		d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
		_, err := knowledge.NewQueue(w.ks, nil).Decide(context.Background(), []domain.ReviewDecision{d})
		return err
	}
	if err := decide(item, domain.ReviewerProxy, t0.Add(2*time.Hour)); err == nil {
		t.Fatal("a proxy promoted a human-authored rule")
	}
	if err := decide(item, domain.ReviewerHuman, t0.Add(2*time.Hour)); err != nil {
		t.Fatalf("a human promotion with a confirmed proof was refused: %v", err)
	}

	// the deliberately wrong rule: refuted by the same method, never promoted
	wrong := strings.Replace(strings.Replace(string(data), `{"op": "field", "path": "spec.signer", "state": "equals", "values": ["true"]}]},`,
		`{"op": "field", "path": "spec.signer", "state": "unset"}]},`, 1),
		`{"op": "field", "path": "spec.signer", "state": "unset"},
          {"op": "field", "path": "spec.signer", "state": "equals", "values": ["false"]}]}]}`,
		`{"op": "field", "path": "spec.signer", "state": "equals", "values": ["true"]}]}]}`, 1)
	wr, err := knowledge.ParseRuleFile([]byte(wrong))
	if err != nil {
		t.Fatal(err)
	}
	if wr.Assertion.Digest() != wrongRule().Digest() {
		t.Fatalf("the wrong rule file does not state the wrong rule:\n%s", wrong)
	}
	wres, err := knowledge.Intake(context.Background(), w.ks, wr, knowledge.IntakeOptions{Author: "test: deliberately wrong", Now: t0})
	if err != nil {
		t.Fatal(err)
	}
	o.Runner = &fakeRunner{res: consumerRun(map[string][2]string{domain.ArmConditionPresent: {runtime.ConsumerVerified, runtime.ConsumerVerified},
		domain.ArmConditionPresent + "-2": {runtime.ConsumerVerified, runtime.ConsumerFailed}})}
	rs, _ = Hypotheses(context.Background(), o)
	for _, x := range rs {
		if x.Item == wres.ReviewItem && (x.Proof == nil || x.Proof.Outcome != domain.ProofRefuted) {
			t.Fatalf("wrong rule: %+v", x)
		}
	}
	snap, _ = w.ks.Load(context.Background(), knowledge.Query{})
	for _, it := range snap.ReviewItems {
		if it.ID == wres.ReviewItem {
			if err := decide(it, domain.ReviewerHuman, t0.Add(3*time.Hour)); err == nil || !strings.Contains(err.Error(), "refuted") {
				t.Fatalf("promotion of the refuted wrong rule: %v", err)
			}
		}
	}
}

// The proofs of the Phase 5 verify smoke run (produced-object observation,
// before the consumer observation existed) still decode, validate and
// re-derive their ids: the consumer fields are additive.
func TestStoredSmokeProofsStillVerify(t *testing.T) {
	paths, err := filepath.Glob("../../../docs/phase5/learning-loop/runs/verify-smoke/proofs/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no stored proofs: %v", err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var rec domain.KnowledgeRecord
		if err := json.Unmarshal(b, &rec); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if err := rec.Validate(); err != nil || rec.Proof == nil {
			t.Fatalf("%s: %v", p, err)
		}
		if err := rec.Proof.Validate(); err != nil || rec.Proof.ID != domain.ProofRecordID(*rec.Proof) {
			t.Fatalf("%s: proof %s no longer verifies: %v", p, rec.Proof.ID, err)
		}
		for _, m := range rec.Proof.Methods {
			for _, a := range m.Arms {
				if a.Consumer != nil {
					t.Fatalf("%s: a stored produced-object proof decodes with a consumer observation", p)
				}
			}
		}
		again, _ := json.Marshal(rec.Proof)
		var back domain.ProofRecord
		if err := json.Unmarshal(again, &back); err != nil || domain.ProofRecordID(back) != rec.Proof.ID {
			t.Fatalf("%s: re-encoding changes the proof id", p)
		}
	}
}
