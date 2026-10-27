package domain

import (
	"strings"
	"testing"
	"time"
)

func testProof(citation, exercise MethodOutcome) ProofRecord {
	ev := NewEvidence(EvidenceDocument, "notes", "https://example.test/v2.0.0/notes.md", "", "quote", "sha256:x", time.Time{})
	ms := []ProofMethod{
		{Method: ProofCitationCheck, Producer: "verify.citations@v1", Level: LevelDeterministic, Outcome: citation, Detail: "d", Evidence: []Evidence{ev}},
		{Method: ProofRuleExercise, Producer: "verify.exercise@v1", Level: LevelObserved, Outcome: exercise, Detail: "d", Evidence: []Evidence{ev},
			Arms: []ProofArm{{Name: ArmConditionPresent, ReactedFrom: true, ReactedTo: true}, {Name: ArmConditionAbsent, ReactedFrom: true, ReactedTo: true, Changed: true}}},
	}
	p := ProofRecord{Hypothesis: "hyp-1", ReviewItem: "ri-1", CandidateID: "sc-1", Product: "acme-ctl", ProposedRuleDigest: "sha256:r",
		Methods: ms, VersionBounds: &ProofBounds{From: "1.0.0", To: "2.0.0"}, Limitations: []string{}, Producer: "verify.hypothesis@v1",
		CheckedAt: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)}
	p.Outcome = ProofOutcomeOf(ms)
	p.Level = LevelObserved
	if exercise == MethodNotRun {
		p.Level = LevelDeterministic
	}
	p.ID = ProofRecordID(p)
	return p
}

func TestProofOutcomeOf(t *testing.T) {
	for _, tc := range []struct {
		c, e MethodOutcome
		want ProofOutcome
	}{
		{MethodPass, MethodPass, ProofConfirmed},
		{MethodPass, MethodNotRun, ProofInconclusive},
		{MethodPass, MethodInconclusive, ProofInconclusive},
		{MethodFail, MethodPass, ProofRefuted},
		{MethodPass, MethodFail, ProofRefuted},
		{MethodFail, MethodNotRun, ProofRefuted},
	} {
		if got := ProofOutcomeOf(testProof(tc.c, tc.e).Methods); got != tc.want {
			t.Errorf("%s/%s = %s, want %s", tc.c, tc.e, got, tc.want)
		}
	}
	// a single passing method is never a confirmation
	if got := ProofOutcomeOf(testProof(MethodPass, MethodPass).Methods[:1]); got != ProofInconclusive {
		t.Errorf("citation only = %s", got)
	}
}

func TestProofValidateRejectsAnOutcomeTheMethodsDoNotSupport(t *testing.T) {
	if err := testProof(MethodPass, MethodPass).Validate(); err != nil {
		t.Fatal(err)
	}
	p := testProof(MethodPass, MethodInconclusive)
	p.Outcome = ProofConfirmed
	p.ID = ProofRecordID(p)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "does not follow") {
		t.Fatalf("hand-edited confirmation: %v", err)
	}
	p = testProof(MethodPass, MethodPass)
	p.Methods[1].Arms = p.Methods[1].Arms[:1]
	p.ID = ProofRecordID(p)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "both arms") {
		t.Fatalf("one-arm pass: %v", err)
	}
	// a non-discriminating condition-absent arm cannot carry a pass; an
	// omitted-field arm predicted to change can (lane verify2)
	p = testProof(MethodPass, MethodPass)
	p.Methods[1].Arms[1].NonDiscriminating = "it sets the field explicitly to the new default"
	p.ID = ProofRecordID(p)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "discriminating arm") {
		t.Fatalf("pass on a non-discriminating arm: %v", err)
	}
	p.Methods[1].Arms = append(p.Methods[1].Arms, ProofArm{Name: ArmConditionAbsent + "-omitted-1", Predicted: BehaviourNew, ReactedFrom: true, ReactedTo: true, Changed: true})
	p.ID = ProofRecordID(p)
	if err := p.Validate(); err != nil {
		t.Fatalf("pass on the omitted-field arm: %v", err)
	}
	p.Methods[1].Arms[2].Predicted = BehaviourPrevious
	p.ID = ProofRecordID(p)
	if err := p.Validate(); err == nil {
		t.Fatal("an omitted-field arm predicted to keep its behaviour cannot carry a pass")
	}
	p = testProof(MethodPass, MethodNotRun)
	p.Level = LevelObserved
	p.ID = ProofRecordID(p)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "level") {
		t.Fatalf("observed without an exercise: %v", err)
	}
}

// Observed defaults (PD-5, lane verify3) are recorded only on a confirmed
// proof, are the proof's own, name its exercise's arms, and are part of its
// id; earlier proofs (none recorded) keep their ids.
func TestProofObservedDefaults(t *testing.T) {
	p := testProof(MethodPass, MethodPass)
	before := ProofRecordID(p)
	od := ObservedDefault{Group: "acme.example", Kind: "Badge", Path: "spec.signer", Value: "false", Level: LevelObserved,
		Arms: []string{ArmConditionPresent, ArmConditionAbsent, ArmConditionAbsent}, Observation: "o", Versions: []string{"1.0.0", "2.0.0"},
		ControllerValues: "{}", Fixture: "sha256:fx", Evidence: p.Methods[1].Evidence, Limitation: ObservedDefaultLimitation, RuleDigest: p.ProposedRuleDigest}
	p.ObservedDefaults = []ObservedDefault{od}
	if ProofRecordID(p) == before {
		t.Fatal("the observed defaults are not part of the proof's id")
	}
	p.ID = ProofRecordID(p)
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ProofRecord){
		"on an inconclusive proof": func(p *ProofRecord) {
			p.Methods[1].Outcome = MethodInconclusive
			p.Outcome = ProofOutcomeOf(p.Methods)
		},
		"another rule":        func(p *ProofRecord) { p.ObservedDefaults[0].RuleDigest = "sha256:other" },
		"a proof reference":   func(p *ProofRecord) { p.ObservedDefaults[0].Proof = "pf-0123456789ab" },
		"an unknown arm":      func(p *ProofRecord) { p.ObservedDefaults[0].Arms = []string{"x", "y", "z"} },
		"without limitation":  func(p *ProofRecord) { p.ObservedDefaults[0].Limitation = "" },
		"not observed":        func(p *ProofRecord) { p.ObservedDefaults[0].Level = LevelDeterministic },
		"without evidence":    func(p *ProofRecord) { p.ObservedDefaults[0].Evidence = nil },
		"without the version": func(p *ProofRecord) { p.ObservedDefaults[0].Versions = nil },
	} {
		q := p
		q.Methods = append([]ProofMethod(nil), p.Methods...)
		q.ObservedDefaults = []ObservedDefault{od}
		mutate(&q)
		q.ID = ProofRecordID(q)
		if err := q.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
