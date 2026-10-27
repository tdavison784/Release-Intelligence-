package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// --- proof records (Phase 5 PROTOCOL §7–§8, lane verify) -------------------------------------
//
// A ProofRecord is the independent verification of one agent-proposed
// mitigation rule (a hypothesis-sourced fact-review item): which methods ran,
// what each established, at which level, within which version bounds, and
// what it did not cover. It is stored with the review item in the knowledge
// store and gates promotion: a human accept/correct of a hypothesis-sourced
// item needs a confirmed proof for the same proposed-rule digest
// (internal/knowledge). A proof never decides applicability to an
// environment — the deterministic validators do that at investigation time.

// ProofIDPrefix starts every proof record id.
const ProofIDPrefix = "pf-"

// HumanAuthoredRuleKind prefixes the provenance rule of a knowledge proposal
// taken in from a rule file written outside the agent (`ri knowledge
// intake-rule`, lane verify2): "human-authored:<hyp-id>", the hypothesis id
// derived from the file's digest. Such a proposal is gated, verified and
// reviewed exactly like an exported agent hypothesis (PROTOCOL amendment
// 2026-10-17 "fallback"). CONTRACT-CHANGE(verify2): additive.
const HumanAuthoredRuleKind = "human-authored:"

// HumanAuthoredHypothesisID is the hypothesis id of a human-authored rule
// file (its sha256 digest).
func HumanAuthoredHypothesisID(fileDigest string) string {
	return HypothesisIDPrefix + ShortHash("human-authored", fileDigest)
}

// ProofOutcome is what the verification concluded about the rule.
type ProofOutcome string

const (
	// ProofConfirmed: every method ran and passed.
	ProofConfirmed ProofOutcome = "confirmed"
	// ProofRefuted: at least one method failed (the evidence contradicts the rule).
	ProofRefuted ProofOutcome = "refuted"
	// ProofInconclusive: nothing failed, but not every method passed (one did
	// not run, could not decide, or the change itself was not demonstrated).
	ProofInconclusive ProofOutcome = "inconclusive"
)

// ProofMethodKind names a verification method.
type ProofMethodKind string

const (
	// ProofCitationCheck: the cited upstream texts resolve in the evidence
	// cache, contain the cited passage, and name a version inside the rule's
	// bounds (deterministic).
	ProofCitationCheck ProofMethodKind = "citation-check"
	// ProofRuleExercise: a synthetic fixture upgraded FROM → TO in a throwaway
	// cluster, the declared exercise run on both arms (condition present /
	// absent), both observed (observed).
	ProofRuleExercise ProofMethodKind = "rule-exercise"
	// ProofSemanticCheck: the value tokens the rule asserts (defaults,
	// allowed values, character classes, before/after values, booleans)
	// each occur in a cited passage with the same polarity (deterministic;
	// Phase 6 M4, mission E4, lane semverify). A token the passages lack, or
	// a flipped polarity, fails it; a claim it cannot decide is inconclusive
	// and left to the human reviewer, never to a model. It is optional: a
	// failure refutes the proof, a pass is recorded beside the required
	// methods and never replaces one. CONTRACT-CHANGE(semverify): additive.
	ProofSemanticCheck ProofMethodKind = "semantic-check"
)

// ProofMethodKinds lists the methods a confirmation requires (PROTOCOL §7.1:
// the upstream rule is confirmed only by a citation check and a two-arm
// runtime exercise).
var ProofMethodKinds = []ProofMethodKind{ProofCitationCheck, ProofRuleExercise}

// AllProofMethodKinds lists every method, the optional ones included.
var AllProofMethodKinds = []ProofMethodKind{ProofCitationCheck, ProofRuleExercise, ProofSemanticCheck}

// required reports whether a confirmation needs the method to pass.
func (k ProofMethodKind) required() bool {
	for _, r := range ProofMethodKinds {
		if k == r {
			return true
		}
	}
	return false
}

// MethodOutcome is the result of one method.
type MethodOutcome string

const (
	MethodPass MethodOutcome = "pass"
	MethodFail MethodOutcome = "fail"
	// MethodInconclusive: the method ran but could not decide.
	MethodInconclusive MethodOutcome = "inconclusive"
	// MethodNotRun: the method could not run (opt-in not given, a missing
	// input or tool), with the reason.
	MethodNotRun MethodOutcome = "not-run"
)

var methodOutcomes = []MethodOutcome{MethodPass, MethodFail, MethodInconclusive, MethodNotRun}

// ProofBounds are the rule's release bounds: From exclusive (the last
// version with the previous behaviour), To inclusive.
type ProofBounds struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ProofCheck is one checked unit of a method (a citation, an arm).
type ProofCheck struct {
	// Subject names what was checked: an evidence id, an arm name.
	Subject string        `json:"subject"`
	Outcome MethodOutcome `json:"outcome"`
	Detail  string        `json:"detail"`
}

// ProofArm is one arm of a rule exercise: a synthetic subject object with the
// rule's mitigating condition present, or absent (the control).
type ProofArm struct {
	// Name is "condition-present" or "condition-absent".
	Name string `json:"name"`
	// Objects are the live object ids observed for the arm: the subject and
	// the objects its controller produces from it.
	Objects []string `json:"objects"`
	// Condition says how the arm's subject was built from the rule.
	Condition string `json:"condition"`
	// Predicted is what the hypothesis predicts for the arm across the
	// upgrade: "previous-behaviour" or "new-behaviour".
	Predicted string `json:"predicted"`
	// Reacted reports the declared exercise reaction on the subject at FROM
	// and at TO (an exercise that was not answered proves nothing).
	ReactedFrom bool `json:"reactedFrom"`
	ReactedTo   bool `json:"reactedTo"`
	// Changed: the arm's objects behaved differently at TO than at FROM
	// under the same exercise (Signatures lists the differences).
	Changed    bool     `json:"changed"`
	Signatures []string `json:"signatures,omitempty"`
	// Diagnostics are differences that never decide the arm: controller
	// log lines and warning events naming its objects, which are
	// nondeterministic between runs. Signatures (and Changed) are object
	// state only: the data, spec and status of the arm's objects and their
	// outputs (lane verify3, D-0026). CONTRACT-CHANGE(verify3): additive.
	Diagnostics []string `json:"diagnostics,omitempty"`
	// Observed is "previous-behaviour", "new-behaviour" or "undetermined".
	Observed string `json:"observed"`
	// Consumer is the arm's consumer observation when the exercise made one
	// (lane verify2): then it, not the data of the arm's objects, decides
	// the arm. CONTRACT-CHANGE(verify2): additive.
	Consumer *ProofConsumer `json:"consumer,omitempty"`
	// NonDiscriminating says why the arm cannot contrast FROM with TO by
	// construction (for example an explicit value of a field whose default
	// changed: it behaves the same at both versions): such an arm is run and
	// recorded but never decides the method (Milestone 4 fix, lane verify2,
	// D-0025). "" for every other arm. CONTRACT-CHANGE(verify2): additive.
	NonDiscriminating string `json:"nonDiscriminating,omitempty"`
	// Probe marks a default probe (PD-5, lane verify3, D-0026): a base arm's
	// object with one field omitted or set to another value, in the base
	// arm's observation. A probe is run and recorded and never decides the
	// method; it only establishes what an unset field behaves as
	// (ObservedDefault). CONTRACT-CHANGE(verify3): additive.
	Probe *ArmProbe `json:"probe,omitempty"`
}

// ArmProbe says how a default probe was built from its base arm.
type ArmProbe struct {
	// Field is the probed field (a path on the subject kind); Base the base
	// arm, which states it explicitly as BaseValue (JSON-encoded).
	Field     string `json:"field"`
	Base      string `json:"base"`
	BaseValue string `json:"baseValue"`
	// Omitted: the probe leaves Field unset; otherwise it sets Value.
	Omitted bool   `json:"omitted,omitempty"`
	Value   string `json:"value,omitempty"`
}

// ObservedDefaultLimitation is the limitation every observed default
// carries (PD-5).
const ObservedDefaultLimitation = "observed on the fixture's controller configuration"

// ObservedDefault is a default for an unset field established by
// observation (PD-5, lane verify3, D-0026): in a rule exercise on the real
// controller, the object that omits Path behaved exactly as the base arm
// that sets it to Value, and differently from the same object with another
// value, so the observation distinguishes the values. It is recorded only
// on a confirmed proof; the applicability evaluator uses it only through a
// confirmed proof of the same rule digest, carried into the verified fact
// on human promotion. Level is always observed.
type ObservedDefault struct {
	// Group and Kind are the subject kind; Path the field; Value the
	// JSON-encoded value an unset Path behaves as.
	Group string          `json:"group,omitempty"`
	Kind  string          `json:"kind"`
	Path  string          `json:"path"`
	Value string          `json:"value"`
	Level ValidationLevel `json:"level"`
	// Arms are the arms that establish it (base, omitted probe, other-value
	// probe, per observation); Observation says what each observed.
	Arms        []string `json:"arms"`
	Observation string   `json:"observation"`
	// Versions are the controller versions observed (FROM, TO);
	// ControllerValues the fixture's controller values (JSON) and Fixture
	// the fixture set's digest.
	Versions         []string `json:"versions"`
	ControllerValues string   `json:"controllerValues"`
	Fixture          string   `json:"fixture"`
	// Evidence are the run's artifacts.
	Evidence   []Evidence `json:"evidence"`
	Limitation string     `json:"limitation"`
	// RuleDigest is the rule the proof verified; Proof the confirmed proof
	// (set when the default is carried into a fact; empty on the proof).
	RuleDigest string `json:"ruleDigest"`
	Proof      string `json:"proof,omitempty"`
}

// Key identifies the field an observed default is about.
func (d ObservedDefault) Key() string { return d.Group + "/" + d.Kind + "/" + d.Path }

// Same reports whether two observed defaults state the same default from
// the same observation (the proof reference aside).
func (d ObservedDefault) Same(o ObservedDefault) bool {
	d.Proof, o.Proof = "", ""
	a, _ := json.Marshal(d)
	b, _ := json.Marshal(o)
	return string(a) == string(b)
}

// Validate checks the default's shape (not where it comes from: see
// ObservedDefaultEstablished).
func (d ObservedDefault) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("observed default %s: %s", d.Key(), fmt.Sprintf(format, args...)))
	}
	var v any
	if strings.TrimSpace(d.Kind) == "" || strings.TrimSpace(d.Path) == "" || json.Unmarshal([]byte(d.Value), &v) != nil {
		bad("kind, path and a JSON value are required")
	}
	if d.Level != LevelObserved {
		bad("level %q: an observed default is observed", d.Level)
	}
	if len(d.Arms) < 3 || strings.TrimSpace(d.Observation) == "" {
		bad("the base arm, the omitted probe and the other-value probe, and what they observed, are required")
	}
	if len(d.Versions) == 0 || strings.TrimSpace(d.Fixture) == "" {
		bad("the controller versions and the fixture are required")
	}
	if len(d.Evidence) == 0 {
		bad("the exercise evidence is required")
	}
	if d.Limitation != ObservedDefaultLimitation {
		bad("the limitation must be %q", ObservedDefaultLimitation)
	}
	if strings.TrimSpace(d.RuleDigest) == "" {
		bad("ruleDigest is required")
	}
	if d.Proof != "" && !strings.HasPrefix(d.Proof, ProofIDPrefix) {
		bad("proof %q lacks the %s prefix", d.Proof, ProofIDPrefix)
	}
	return errors.Join(errs...)
}

// ObservedDefaultEstablished reports why a fact cannot use an observed
// default ("" when it can): it must name a confirmed proof of exactly the
// fact's rule (digest) that records the same default.
func ObservedDefaultEstablished(f VerifiedFact, d ObservedDefault, proofs map[string]ProofRecord) string {
	if why := d.usableBy(f); why != "" {
		return why
	}
	p, ok := proofs[d.Proof]
	switch {
	case !ok:
		return "proof " + d.Proof + " does not resolve"
	case p.Outcome != ProofConfirmed:
		return fmt.Sprintf("proof %s is %s, not confirmed", p.ID, p.Outcome)
	case p.ProposedRuleDigest != d.RuleDigest:
		return fmt.Sprintf("proof %s verified another rule (digest %s)", p.ID, p.ProposedRuleDigest)
	}
	for _, x := range p.ObservedDefaults {
		if x.Same(d) {
			return ""
		}
	}
	return "proof " + p.ID + " does not record this observed default"
}

// usableBy reports why the default cannot serve fact f on its face (no
// confirmed proof named, another rule) — the records check is
// ObservedDefaultEstablished.
func (d ObservedDefault) usableBy(f VerifiedFact) string {
	switch {
	case d.Validate() != nil:
		return d.Validate().Error()
	case d.Proof == "":
		return "it names no confirmed proof (a default that is merely proposed is ignored)"
	case d.RuleDigest != f.Assertion.Digest():
		return fmt.Sprintf("it was observed for another rule (digest %s, the fact's is %s)", d.RuleDigest, f.Assertion.Digest())
	}
	return ""
}

// UsableObservedDefaults are the fact's observed defaults that name a
// confirmed proof of exactly its rule (on their face; impact.VerifyFacts
// checks them against the proof records). ignored says why each other one
// is not used.
func (f VerifiedFact) UsableObservedDefaults() (usable []ObservedDefault, ignored []string) {
	for _, d := range f.ObservedDefaults {
		if why := d.usableBy(f); why != "" {
			ignored = append(ignored, fmt.Sprintf("observed default %s = %s ignored: %s", d.Key(), d.Value, why))
			continue
		}
		usable = append(usable, d)
	}
	return usable, ignored
}

// Answered reports whether the arm's exercise (or consumer observation) was
// answered at FROM and at TO.
func (a ProofArm) Answered() bool { return a.ReactedFrom && a.ReactedTo }

// Contrast reports whether the arm can decide that the behaviour changed:
// the principal condition-absent arm, or another condition-absent arm (a
// numbered alternative, an omitted-field arm) predicted to take the new
// behaviour — never a non-discriminating one.
func (a ProofArm) Contrast() bool {
	switch {
	case a.NonDiscriminating != "":
		return false
	case a.Name == ArmConditionAbsent:
		return true
	}
	return strings.HasPrefix(a.Name, ArmConditionAbsent+"-") && a.Predicted == BehaviourNew
}

// ProofConsumer is a consumer observation of one arm: at each version a
// consumer captured the Trust certificates, the Reissue objects were
// re-issued in order, and the Verified certificate chain was checked against
// the captured trust with x509 (outcome and digests only: never key
// material).
type ProofConsumer struct {
	Check    string               `json:"check"`
	Trust    string               `json:"trust"`
	Reissue  []string             `json:"reissue"`
	Verified string               `json:"verified"`
	From     *ProofConsumerResult `json:"from,omitempty"`
	To       *ProofConsumerResult `json:"to,omitempty"`
}

// ProofConsumerResult is the consumer observation at one version.
type ProofConsumerResult struct {
	// Outcome is "verified", "failed" (the chain does not reach the
	// captured trust) or "undetermined".
	Outcome           string   `json:"outcome"`
	Detail            string   `json:"detail"`
	TrustCertificates []string `json:"trustCertificates,omitempty"`
	TrustKeys         []string `json:"trustKeys,omitempty"`
	Certificate       string   `json:"certificate,omitempty"`
	CertificateKey    string   `json:"certificateKey,omitempty"`
}

// Arm names and behaviours.
const (
	ArmConditionPresent   = "condition-present"
	ArmConditionAbsent    = "condition-absent"
	BehaviourPrevious     = "previous-behaviour"
	BehaviourNew          = "new-behaviour"
	BehaviourUndetermined = "undetermined"
)

// ProofMethod is one method's result.
type ProofMethod struct {
	Method ProofMethodKind `json:"method"`
	// Producer is the component that ran it (component@vN).
	Producer string `json:"producer"`
	// Level is deterministic (citation check) or observed (rule exercise).
	Level   ValidationLevel `json:"level"`
	Outcome MethodOutcome   `json:"outcome"`
	Detail  string          `json:"detail"`
	Checks  []ProofCheck    `json:"checks,omitempty"`
	Arms    []ProofArm      `json:"arms,omitempty"`
	// Evidence are the records the method rests on: the cited upstream
	// evidence, or the run's artifacts (URI ri:…, contentDigest).
	Evidence []Evidence `json:"evidence,omitempty"`
}

// ProofRecord is the verification of one proposed rule (PROTOCOL §8).
type ProofRecord struct {
	ID string `json:"id"`
	// Hypothesis and Investigation identify the agent hypothesis the rule
	// came from (the exported proposal's provenance rule).
	Hypothesis    string `json:"hypothesis"`
	Investigation string `json:"investigation,omitempty"`
	// ReviewItem is the hypothesis-sourced fact-review item; CandidateID its
	// candidate (where the record is stored).
	ReviewItem  string    `json:"reviewItem"`
	CandidateID string    `json:"candidateId"`
	Product     ProductID `json:"product"`
	Release     string    `json:"release,omitempty"`
	// ProposedRuleDigest is SemanticAssertion.Digest() of the rule verified:
	// a correction that changes the rule changes the digest and needs a new proof.
	ProposedRuleDigest string        `json:"proposedRuleDigest"`
	Methods            []ProofMethod `json:"methods"`
	Outcome            ProofOutcome  `json:"outcome"`
	VersionBounds      *ProofBounds  `json:"versionBounds,omitempty"`
	// Conditions are the rule's applicability conditions the proof exercised
	// (the environment conditions under which the mitigation is claimed).
	Conditions *Applicability `json:"conditions,omitempty"`
	// Limitations lists what the proof does not cover (other consumers,
	// topologies, versions, conditions that could not be synthesised).
	Limitations []string `json:"limitations"`
	// Level is observed when the rule exercise ran (whatever it concluded),
	// deterministic otherwise.
	Level     ValidationLevel `json:"level"`
	Producer  string          `json:"producer"`
	CheckedAt time.Time       `json:"checkedAt"`
	// ObservedDefaults are defaults of unset fields the rule exercise
	// established by observation (PD-5): only on a confirmed proof.
	// CONTRACT-CHANGE(verify3): additive.
	ObservedDefaults []ObservedDefault `json:"observedDefaults,omitempty"`
}

// ProofRecordID derives a proof id from its content.
func ProofRecordID(p ProofRecord) string {
	m, _ := json.Marshal(p.Methods)
	parts := []string{p.ReviewItem, p.ProposedRuleDigest, string(p.Outcome), p.CheckedAt.UTC().Format(time.RFC3339Nano), Digest(m)}
	if len(p.ObservedDefaults) > 0 {
		// only when present, so the ids of earlier proofs are unchanged
		d, _ := json.Marshal(p.ObservedDefaults)
		parts = append(parts, Digest(d))
	}
	return ProofIDPrefix + ShortHash(parts...)
}

// ProofOutcomeOf derives the outcome from the methods: confirmed only when
// every required method ran and passed; any failure (of any method)
// refutes; otherwise inconclusive. An optional method (semantic-check) that
// did not pass and did not fail leaves the outcome to the required ones.
func ProofOutcomeOf(ms []ProofMethod) ProofOutcome {
	passed := map[ProofMethodKind]bool{}
	for _, m := range ms {
		if m.Outcome == MethodFail {
			return ProofRefuted
		}
	}
	for _, m := range ms {
		if m.Outcome != MethodPass {
			if !m.Method.required() {
				continue
			}
			return ProofInconclusive
		}
		passed[m.Method] = true
	}
	for _, k := range ProofMethodKinds {
		if !passed[k] {
			return ProofInconclusive
		}
	}
	return ProofConfirmed
}

// Validate checks identity, references, the method shapes and that the
// outcome follows from the methods (a hand-edited "confirmed" over an
// undecided method is rejected).
func (p ProofRecord) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("proof %s: %s", p.ID, fmt.Sprintf(format, args...)))
	}
	if p.ID != ProofRecordID(p) {
		bad("id is not derived from its content")
	}
	if !strings.HasPrefix(p.ReviewItem, ReviewItemIDPrefix) || !strings.HasPrefix(p.CandidateID, CandidateIDPrefix) || p.Product == "" {
		bad("reviewItem, candidateId and product are required")
	}
	if !strings.HasPrefix(p.Hypothesis, HypothesisIDPrefix) {
		bad("hypothesis %q lacks the %s prefix", p.Hypothesis, HypothesisIDPrefix)
	}
	if p.Investigation != "" && !strings.HasPrefix(p.Investigation, InvestigationIDPrefix) {
		bad("investigation %q lacks the %s prefix", p.Investigation, InvestigationIDPrefix)
	}
	if strings.TrimSpace(p.ProposedRuleDigest) == "" {
		bad("proposedRuleDigest is required")
	}
	if p.Producer == "" || p.CheckedAt.IsZero() {
		bad("producer and checkedAt are required")
	}
	if len(p.Methods) == 0 {
		bad("at least one method is required")
	}
	seen := map[ProofMethodKind]bool{}
	observed := false
	for i, m := range p.Methods {
		w := fmt.Sprintf("method %d (%s)", i, m.Method)
		switch m.Method {
		case ProofCitationCheck:
			if m.Level != LevelDeterministic {
				bad("%s: a citation check is deterministic", w)
			}
		case ProofSemanticCheck:
			if m.Level != LevelDeterministic {
				bad("%s: a semantic check is deterministic", w)
			}
		case ProofRuleExercise:
			if m.Level != LevelObserved {
				bad("%s: a rule exercise is observed", w)
			}
			observed = observed || m.Outcome != MethodNotRun
			if m.Outcome == MethodPass {
				// the condition arm, and an arm that can show the change
				// (a non-discriminating condition-absent arm cannot)
				present, contrast := false, false
				for _, a := range m.Arms {
					present = present || (a.Name == ArmConditionPresent && a.Answered())
					contrast = contrast || (a.Contrast() && a.Answered())
				}
				if !present || !contrast {
					bad("%s: a passing exercise needs both arms observed with the exercise answered at both versions (the condition-absent side by a discriminating arm)", w)
				}
			}
		default:
			bad("%s: unknown method", w)
		}
		if seen[m.Method] {
			bad("%s: repeated method", w)
		}
		seen[m.Method] = true
		if !oneOf(m.Outcome, methodOutcomes) {
			bad("%s: unknown outcome %q", w, m.Outcome)
		}
		if m.Producer == "" || strings.TrimSpace(m.Detail) == "" {
			bad("%s: producer and detail are required", w)
		}
		if m.Outcome == MethodPass && len(m.Evidence) == 0 {
			bad("%s: a passing method must cite its evidence", w)
		}
	}
	if want := ProofOutcomeOf(p.Methods); p.Outcome != want {
		bad("outcome %q does not follow from the methods (%q)", p.Outcome, want)
	}
	want := LevelDeterministic
	if observed {
		want = LevelObserved
	}
	if p.Level != want {
		bad("level %q, want %q (observed exactly when the rule exercise ran)", p.Level, want)
	}
	if p.Outcome == ProofConfirmed && p.VersionBounds == nil {
		bad("a confirmed proof states its version bounds")
	}
	if p.Limitations == nil {
		bad("limitations are required (an empty list says nothing is known to be uncovered)")
	}
	if len(p.ObservedDefaults) > 0 && p.Outcome != ProofConfirmed {
		bad("observed defaults are recorded only on a confirmed proof")
	}
	arms := map[string]bool{}
	for _, m := range p.Methods {
		if m.Method == ProofRuleExercise {
			for _, a := range m.Arms {
				arms[a.Name] = true
			}
		}
	}
	for _, d := range p.ObservedDefaults {
		if err := d.Validate(); err != nil {
			bad("%v", err)
		}
		if d.RuleDigest != p.ProposedRuleDigest || d.Proof != "" {
			bad("observed default %s: it is the proof's own (ruleDigest = proposedRuleDigest, no proof reference)", d.Key())
		}
		for _, a := range d.Arms {
			if !arms[a] {
				bad("observed default %s: arm %s is not an arm of the rule exercise", d.Key(), a)
			}
		}
	}
	return errors.Join(errs...)
}
