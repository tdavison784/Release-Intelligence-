package verify

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// "The role cannot form" (PD-6, lane verify4, D-0027) on the synthetic Badge
// product. Default probe 1 is the in-role exposed arm (condition-absent,
// spec.signer = true) without spec.signer and with spec.signer = false: a
// Badge that does not sign cannot back its Signer, so the leaf the consumer
// verifies is never issued. Probe 2 is the role-absent arm's, where the
// values behave alike (no PD-5 field default).

// formationRunner builds its result from the fixture it is given, so the
// object observations name the arms' real objects.
type formationRunner struct {
	build func(runtime.FixtureSpec) *runtime.FixtureResult
}

func (r formationRunner) RunFixture(_ context.Context, spec runtime.FixtureSpec) (*runtime.FixtureResult, error) {
	return r.build(spec), nil
}

// shape sets what one observation phase saw of a stopped arm's objects;
// subject, signer and leaf are its objects' ids, out the verified output.
type shape func(phase, subject, signer, leaf, out string, objs map[string]runtime.ObjectState)

var phases = []string{runtime.PhaseFrom, runtime.PhaseFromExercised, runtime.PhaseTo, runtime.PhaseToExercised}

func ready() *bool { b := true; return &b }

func notReady() *bool { b := false; return &b }

// refused: the controller reports the leaf failed and never writes its
// Secret (the shape "the role cannot form").
func refused(_, _, _, leaf, out string, objs map[string]runtime.ObjectState) {
	objs[leaf] = runtime.ObjectState{Exists: true, Ready: notReady(), NotReady: "Ready=False (DoesNotExist): the leaf has not been issued",
		Conditions: []runtime.Condition{{Type: "Issuing", Status: "False", Reason: "Failed", Message: "the signer cannot sign: its Badge is not a signer"},
			{Type: "Ready", Status: "False", Reason: "DoesNotExist", Message: "the leaf has not been issued"}},
		Warnings: []string{"IssuanceFailed: the signer cannot sign"}, Logs: []string{"error: reconcile leaf: not a signer"}}
	objs[out] = runtime.ObjectState{}
}

// observedRun is refinementRun with object observations at every phase:
// every applied and observed object exists and is ready, except that each
// arm in stop has its consumer check stop at its leaf, and shape says what
// the phase saw of its objects.
func observedRun(spec runtime.FixtureSpec, outcomes map[string][2]string, stop map[string]shape) *runtime.FixtureResult {
	res := refinementRun(outcomes, nil)
	checks := map[string]runtime.ConsumerCheck{}
	for _, c := range spec.Consumers {
		checks[c.ID] = c
	}
	for i := range res.Run.Consumers {
		c := &res.Run.Consumers[i]
		chk := checks[c.Check]
		c.TrustCertificates = []string{"sha256:trust"}
		if _, ok := stop[c.Check]; ok {
			c.Outcome, c.Certificate, c.CertificateKey = runtime.ConsumerUndetermined, "", ""
			c.Reissued, c.Unanswered = chk.Reissue[:1], chk.Reissue[1:2]
			c.Detail = chk.Reissue[1] + " was not re-issued (status.revision did not change within 2m0s): the consumer observation cannot be made"
		}
	}
	var ids []string
	dec := yaml.NewDecoder(bytes.NewReader(spec.Objects))
	for {
		var obj map[string]any
		if err := dec.Decode(&obj); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			panic(err)
		}
		ids = append(ids, objectID(obj))
	}
	ids = append(ids, spec.Observe...)
	res.Observations = map[string]*runtime.Observation{}
	for _, phase := range phases {
		objs := map[string]runtime.ObjectState{}
		for _, id := range ids {
			st := runtime.ObjectState{Exists: true, Ready: ready(), Conditions: []runtime.Condition{{Type: "Ready", Status: "True", Reason: "Ready"}}}
			if strings.HasPrefix(id, "core/v1/Secret/") {
				st = runtime.ObjectState{Exists: true, Data: map[string]string{"ca.crt": "h1", "tls.crt": "h2"}}
			}
			objs[id] = st
		}
		for arm, sh := range stop {
			chk := checks[arm]
			subject, leaf := chk.Reissue[0], chk.Reissue[1]
			signer := strings.Replace(subject, "/Badge/", "/Signer/", 1) + "-signer"
			sh(phase, subject, signer, leaf, chk.Verify.Secret, objs)
		}
		res.Observations[phase] = &runtime.Observation{Phase: phase, Objects: objs}
	}
	return res
}

// probeOutcomes: the in-role probes undetermined at the runtime, the
// role-absent probes alike for both values.
var probeOutcomes = map[string][2]string{probe1u: {pu, pu}, probe1o: {pu, pu}, probe2u: {pv, pv}, probe2o: {pv, pv}}

func exerciseBuilt(t *testing.T, rule domain.SemanticAssertion, build func(runtime.FixtureSpec) *runtime.FixtureResult) domain.ProofMethod {
	t.Helper()
	in := refinementInput(t, rule, nil)
	in.Runner = formationRunner{build: build}
	m, _ := ExerciseRule(context.Background(), in)
	return m
}

func armNamed(m domain.ProofMethod, name string) domain.ProofArm {
	for _, a := range m.Arms {
		if a.Name == name {
			return a
		}
	}
	return domain.ProofArm{}
}

func TestRoleCannotFormVersusUndetermined(t *testing.T) {
	rule := refinedRule(nil)
	both := func(sh shape) map[string]shape { return map[string]shape{probe1u: sh, probe1o: sh} }

	// the relationship cannot form: observed, with object-state evidence
	m := exerciseBuilt(t, rule, func(s runtime.FixtureSpec) *runtime.FixtureResult { return observedRun(s, probeOutcomes, both(refused)) })
	if m.Outcome != domain.MethodPass || m.Producer != ConsumerExerciseProducer {
		t.Fatalf("the probes decided the method: %s %s", m.Outcome, m.Detail)
	}
	for _, name := range []string{probe1u, probe1o} {
		a := armNamed(m, name)
		if a.Observed != domain.BehaviourRoleCannotForm || a.Consumer.From.Outcome != domain.ConsumerRoleCannotForm || a.Consumer.To.Outcome != domain.ConsumerRoleCannotForm {
			t.Fatalf("%s: observed %s, consumer %s", name, a.Observed, consumerOutcomes(a))
		}
		ev := strings.Join(append(append([]string(nil), a.Consumer.From.Formation...), a.Consumer.To.Formation...), "|")
		for _, want := range []string{"ri-arm-probe-1-", "the dependent output was not produced: ", "-leaf was not re-issued", "subject acme.example/v1/Badge/", "ready; conditions Ready=True",
			"reports a failure, condition Issuing=False (Failed)", "Issuing=False (Failed): the signer cannot sign", "warning events IssuanceFailed",
			"-leaf-tls[tls.crt] does not exist", runtime.PhaseFromExercised + ":", runtime.PhaseToExercised + ":"} {
			if !strings.Contains(ev, want) {
				t.Errorf("%s: formation evidence lacks %q: %s", name, want, ev)
			}
		}
		if strings.Contains(ev, "reconcile leaf") {
			t.Errorf("%s: a log line is evidence: %s", name, ev)
		}
		if !strings.Contains(a.Signatures[0], "role-cannot-form") {
			t.Errorf("%s: signature %v", name, a.Signatures)
		}
	}
	// role-absent probes are never role-cannot-form (their consumer is the
	// role-absent observation's)
	if a := armNamed(m, probe2u); a.Observed != domain.BehaviourPrevious {
		t.Fatalf("role-absent probe: %s", a.Observed)
	}

	// undetermined: everything else that leaves the consumer without an answer
	undetermined := map[string]struct {
		sh     shape
		mutate func(*runtime.FixtureResult)
		why    string
	}{
		"timeout: the leaf never reported": {sh: func(_, _, _, leaf, out string, objs map[string]runtime.ObjectState) {
			objs[leaf] = runtime.ObjectState{Exists: true}
			objs[out] = runtime.ObjectState{}
		}, why: "may only be late"},
		"timeout: not ready, still working on it": {sh: func(_, _, _, leaf, out string, objs map[string]runtime.ObjectState) {
			objs[leaf] = runtime.ObjectState{Exists: true, Ready: notReady(), NotReady: "Ready=False (DoesNotExist)",
				Conditions: []runtime.Condition{{Type: "Issuing", Status: "True", Reason: "DoesNotExist"}, {Type: "Ready", Status: "False", Reason: "DoesNotExist"}}}
			objs[out] = runtime.ObjectState{}
		}, why: "may only be late"},
		"timeout at TO only": {sh: func(phase, s, g, leaf, out string, objs map[string]runtime.ObjectState) {
			if phase == runtime.PhaseFrom || phase == runtime.PhaseFromExercised {
				refused(phase, s, g, leaf, out, objs)
				return
			}
			objs[leaf] = runtime.ObjectState{Exists: true}
		}, why: "may only be late"},
		"a consuming object is missing": {sh: func(phase, s, signer, leaf, out string, objs map[string]runtime.ObjectState) {
			refused(phase, s, signer, leaf, out, objs)
			objs[signer] = runtime.ObjectState{}
		}, why: "-signer does not exist"},
		"a consuming object was not applied": {sh: func(phase, s, signer, leaf, out string, objs map[string]runtime.ObjectState) {
			refused(phase, s, signer, leaf, out, objs)
			objs[signer] = runtime.ObjectState{ApplyError: "admission webhook denied the request"}
		}, why: "-signer does not exist"},
		"the subject is not ready (controller failure)": {sh: func(phase, subject, g, leaf, out string, objs map[string]runtime.ObjectState) {
			refused(phase, subject, g, leaf, out, objs)
			objs[subject] = runtime.ObjectState{Exists: true, Ready: notReady(), NotReady: "Ready=False (Pending)"}
		}, why: "is not ready"},
		"the subject is missing": {sh: func(phase, subject, g, leaf, out string, objs map[string]runtime.ObjectState) {
			refused(phase, subject, g, leaf, out, objs)
			delete(objs, subject)
		}, why: "does not exist"},
		"only logs complain": {sh: func(_, _, _, leaf, out string, objs map[string]runtime.ObjectState) {
			st := objs[leaf]
			st.Logs = []string{"error: reconcile leaf: not a signer"}
			objs[leaf] = st
			objs[out] = runtime.ObjectState{}
		}, why: "may only be late"},
		"the subject did not answer": {sh: refused, mutate: func(res *runtime.FixtureResult) {
			for i := range res.Run.Consumers {
				if c := &res.Run.Consumers[i]; c.Check == probe1u {
					c.Unanswered, c.Reissued = []string{strings.Replace(c.Unanswered[0], "-leaf", "", 1)}, nil
				}
			}
		}, why: "the subject itself did not answer"},
		"the trust output was not captured": {sh: refused, mutate: func(res *runtime.FixtureResult) {
			for i := range res.Run.Consumers {
				if c := &res.Run.Consumers[i]; c.Check == probe1u {
					c.TrustCertificates = nil
				}
			}
		}, why: "trust output was not captured"},
		"the output was read, x509 undecided": {sh: refused, mutate: func(res *runtime.FixtureResult) {
			for i := range res.Run.Consumers {
				if c := &res.Run.Consumers[i]; c.Check == probe1u {
					c.Unanswered, c.Certificate, c.Detail = nil, "sha256:c", "x509 verification could not decide (not an authority mismatch): expired"
				}
			}
		}, why: "x509 could not decide"},
		"the sandbox could not run it": {sh: refused, mutate: func(res *runtime.FixtureResult) {
			for i := range res.Run.Consumers {
				if c := &res.Run.Consumers[i]; c.Check == probe1u {
					res.Run.Degradations = append(res.Run.Degradations, runtime.Degradation{Object: c.Unanswered[0], Infrastructure: true, Signature: "image-pull"})
				}
			}
		}, why: "the sandbox could not run"},
		"no object observation stored": {sh: refused, mutate: func(res *runtime.FixtureResult) { res.Observations = nil }, why: "no object observation"},
	}
	for name, tc := range undetermined {
		m := exerciseBuilt(t, rule, func(s runtime.FixtureSpec) *runtime.FixtureResult {
			res := observedRun(s, probeOutcomes, map[string]shape{probe1u: tc.sh})
			if tc.mutate != nil {
				tc.mutate(res)
			}
			return res
		})
		a := armNamed(m, probe1u)
		if a.Observed != domain.BehaviourUndetermined || a.Consumer.To.Outcome != runtime.ConsumerUndetermined || len(a.Consumer.To.Formation) != 0 {
			t.Errorf("%s: observed %s, consumer %s", name, a.Observed, consumerOutcomes(a))
			continue
		}
		var detail string
		for _, c := range m.Checks {
			if c.Subject == probe1u {
				detail = c.Detail
			}
		}
		if !strings.Contains(detail, "undetermined, not role-cannot-form") || !strings.Contains(detail, tc.why) {
			t.Errorf("%s: detail lacks %q: %s", name, tc.why, detail)
		}
	}

	// a controller that does not come up at TO: no arm is decided at all
	m = exerciseBuilt(t, rule, func(s runtime.FixtureSpec) *runtime.FixtureResult {
		res := observedRun(s, probeOutcomes, both(refused))
		res.Run.UpgradeCompleted, res.Run.UpgradeProblems = false, []string{"apps/v1/Deployment/ri-system/controller not ready"}
		return res
	})
	if m.Outcome != domain.MethodInconclusive || armNamed(m, probe1u).Observed == domain.BehaviourRoleCannotForm {
		t.Fatalf("controller failure: %s %+v", m.Outcome, armNamed(m, probe1u))
	}
}

func TestRoleMembershipDefaultRecordedOnlyWhenTheRoleFormed(t *testing.T) {
	rule := refinedRule(nil)
	fx := ObservationFixture{Bounds: bounds, Fixtures: badgeOutsideSet(t)}
	derive := func(outcomes map[string][2]string, stop map[string]shape) (domain.ProofMethod, []domain.RoleMembershipDefault, []string) {
		m := exerciseBuilt(t, rule, func(s runtime.FixtureSpec) *runtime.FixtureResult { return observedRun(s, outcomes, stop) })
		mds, notes := RoleMembershipDefaults(rule, m, fx, refinement)
		return m, mds, notes
	}
	stopBoth := map[string]shape{probe1u: refused, probe1o: refused}

	// the role-value arm formed the relationship; unset and the other value cannot
	m, mds, notes := derive(probeOutcomes, stopBoth)
	if m.Outcome != domain.MethodPass || len(mds) != 1 || len(notes) != 0 {
		t.Fatalf("recorded: %s %+v %v", m.Outcome, mds, notes)
	}
	d := mds[0]
	if d.Path != "spec.signer" || d.Role != "signer-trust" || d.RoleValue != "true" || d.OutsideValue != "false" || d.Kind != "Badge" ||
		d.Group != "acme.example" || d.Level != domain.LevelObserved || d.Limitation != domain.ObservedDefaultLimitation ||
		strings.Join(d.Arms, ",") != strings.Join([]string{domain.ArmConditionAbsent, probe1u, probe1o}, ",") || d.RuleDigest != rule.Digest() ||
		len(d.Evidence) == 0 || d.Fixture == "" || strings.Join(d.Versions, ",") != "1.9.4,2.0.0" || d.Validate() != nil {
		t.Fatalf("membership default %+v (%v)", d, d.Validate())
	}
	if d.Statement() != "spec.signer unset ⇒ outside role signer-trust (observed)" ||
		!strings.Contains(d.Observation, "Issuing=False (Failed)") || !strings.Contains(d.Observation, "the relationship formed (FROM verified, TO failed)") {
		t.Fatalf("statement %q, observation %s", d.Statement(), d.Observation)
	}
	// role-cannot-form is membership only: no PD-5 field default comes of it
	if ods, _ := ObservedDefaults(rule, m, fx); len(ods) != 0 {
		t.Fatalf("a field default from role-cannot-form: %+v", ods)
	}

	// the role-value arm did not form the relationship: nothing (its not
	// forming without the field shows nothing)
	noRole := map[string][2]string{domain.ArmConditionAbsent: {pu, pu}}
	for k, v := range probeOutcomes {
		noRole[k] = v
	}
	if _, mds, notes := derive(noRole, stopBoth); len(mds) != 0 || !strings.Contains(strings.Join(notes, "|"), "did not form the relationship at both versions") {
		t.Fatalf("role arm not formed: %+v %v", mds, notes)
	}
	// the other value forms the relationship: the field does not decide membership as declared
	otherForms := map[string][2]string{probe1o: {pv, pf}}
	for k, v := range probeOutcomes {
		if k != probe1o {
			otherForms[k] = v
		}
	}
	if _, mds, notes := derive(otherForms, map[string]shape{probe1u: refused}); len(mds) != 0 || !strings.Contains(strings.Join(notes, "|"), "does not decide membership") {
		t.Fatalf("other value formed: %+v %v", mds, notes)
	}
	// unset could not form at FROM only (a timeout at TO): nothing
	fromOnly := func(phase, s, g, leaf, out string, objs map[string]runtime.ObjectState) {
		if phase == runtime.PhaseFrom || phase == runtime.PhaseFromExercised {
			refused(phase, s, g, leaf, out, objs)
			return
		}
		objs[leaf] = runtime.ObjectState{Exists: true}
	}
	if _, mds, notes := derive(probeOutcomes, map[string]shape{probe1u: fromOnly, probe1o: refused}); len(mds) != 0 ||
		!strings.Contains(strings.Join(notes, "|"), "was not observed not to form at both versions") {
		t.Fatalf("one version only: %+v %v", mds, notes)
	}
	// the other value undetermined (not formed) still lets unset establish it
	if _, mds, _ := derive(probeOutcomes, map[string]shape{probe1u: refused}); len(mds) != 1 {
		t.Fatalf("other value undetermined: %+v", mds)
	}
	// without a refinement there is no role: nothing
	if mds, notes := RoleMembershipDefaults(rule, m, fx, nil); len(mds) != 0 || len(notes) != 0 {
		t.Fatalf("no refinement: %+v %v", mds, notes)
	}
}

// The whole path: an inconclusive proof records no membership default; a
// confirmed one does; human promotion carries it into the fact; the impact
// layer accepts it through the proof and ignores it otherwise.
func TestRoleMembershipDefaultThroughPromotion(t *testing.T) {
	rule := refinedRule(nil)
	stop := map[string]shape{probe1u: refused, probe1o: refused}
	verifyWith := func(s *proofStore, outcomes map[string][2]string, now time.Time) *domain.ProofRecord {
		t.Helper()
		o := s.opts(nil, now)
		o.Runner = formationRunner{build: func(spec runtime.FixtureSpec) *runtime.FixtureResult { return observedRun(spec, outcomes, stop) }}
		rs, err := Hypotheses(context.Background(), o)
		if err != nil || len(rs) != 1 || rs[0].Proof == nil {
			t.Fatalf("results %+v %v", rs, err)
		}
		return rs[0].Proof
	}

	// inconclusive (the role-absent arms are not observed): none recorded
	s := newProofStore(t, rule)
	open := map[string][2]string{roleAbsentArm: {pu, pu}, domain.ArmConditionPresent + omittedSuffix + outsideRoleSuffix + "-1": {pu, pu}}
	for k, v := range probeOutcomes {
		open[k] = v
	}
	p := verifyWith(s, open, t0.Add(time.Hour))
	if p.Outcome != domain.ProofInconclusive || len(p.RoleMembershipDefaults) != 0 ||
		!strings.Contains(strings.Join(p.Limitations, "|"), "so no role-membership default is recorded") {
		t.Fatalf("inconclusive: %s %+v %v", p.Outcome, p.RoleMembershipDefaults, p.Limitations)
	}
	inconclusive := *p

	// confirmed: recorded, validated, in the proof id
	p = verifyWith(s, probeOutcomes, t0.Add(2*time.Hour))
	if p.Outcome != domain.ProofConfirmed || len(p.RoleMembershipDefaults) != 1 || len(p.ObservedDefaults) != 0 || p.Validate() != nil {
		t.Fatalf("confirmed: %s %+v %v", p.Outcome, p.RoleMembershipDefaults, p.Validate())
	}
	if !strings.Contains(strings.Join(p.Limitations, "|"), "for this rule's membership conditions only; it is not a default of spec.signer") {
		t.Errorf("limitations %v", p.Limitations)
	}
	confirmed := *p
	tampered := confirmed
	tampered.RoleMembershipDefaults = append([]domain.RoleMembershipDefault(nil), confirmed.RoleMembershipDefaults...)
	tampered.RoleMembershipDefaults[0].OutsideValue = "true"
	if tampered.Validate() == nil {
		t.Error("a proof whose membership default was edited still validates")
	}
	onInconclusive := inconclusive
	onInconclusive.RoleMembershipDefaults = confirmed.RoleMembershipDefaults
	onInconclusive.ID = domain.ProofRecordID(onInconclusive)
	if err := onInconclusive.Validate(); err == nil || !strings.Contains(err.Error(), "only on a confirmed proof") {
		t.Errorf("a membership default on an inconclusive proof validates: %v", err)
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
		if snap.Facts[i].Assertion.Digest() == rule.Digest() {
			fact = &snap.Facts[i]
		}
	}
	if fact == nil || len(fact.RoleMembershipDefaults) != 1 || len(fact.ObservedDefaults) != 0 {
		t.Fatalf("the promoted fact does not carry the membership default: %+v", fact)
	}
	md := fact.RoleMembershipDefaults[0]
	if md.Proof != confirmed.ID || md.Limitation != domain.ObservedDefaultLimitation || md.Role != "signer-trust" {
		t.Fatalf("carried default %+v", md)
	}
	facts, rejected := impact.VerifyFacts(snap)
	if len(facts) != 1 || len(facts[0].RoleMembershipDefaults) != 1 || len(rejected) != 0 {
		t.Fatalf("verified facts %d (%+v), rejected %v", len(facts), facts, rejected)
	}
	// naming the inconclusive proof, no proof, another rule, or a default
	// the proof does not record: ignored, the fact stays
	for name, mutate := range map[string]func(*domain.RoleMembershipDefault){
		"inconclusive proof": func(d *domain.RoleMembershipDefault) { d.Proof = inconclusive.ID },
		"merely proposed":    func(d *domain.RoleMembershipDefault) { d.Proof = "" },
		"another rule":       func(d *domain.RoleMembershipDefault) { d.RuleDigest = "sha256:other" },
		"not recorded":       func(d *domain.RoleMembershipDefault) { d.Role = "another-role" },
	} {
		bad := *snap
		f := *fact
		f.RoleMembershipDefaults = append([]domain.RoleMembershipDefault(nil), fact.RoleMembershipDefaults...)
		mutate(&f.RoleMembershipDefaults[0])
		bad.Facts = []domain.VerifiedFact{f}
		bad.Proofs = append(append([]domain.ProofRecord(nil), snap.Proofs...), inconclusive)
		facts, rejected := impact.VerifyFacts(&bad)
		if len(facts) != 1 || len(facts[0].RoleMembershipDefaults) != 0 || !strings.Contains(strings.Join(rejected, "|"), "role-membership default acme.example/Badge/spec.signer") {
			t.Errorf("%s: facts %d, kept %v, rejected %v", name, len(facts), facts, rejected)
		}
	}
}
