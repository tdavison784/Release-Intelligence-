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
	p = testProof(MethodPass, MethodNotRun)
	p.Level = LevelObserved
	p.ID = ProofRecordID(p)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "level") {
		t.Fatalf("observed without an exercise: %v", err)
	}
}
