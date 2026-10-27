package verify

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestCheckSemantics(t *testing.T) {
	before, after := "Keep", "Renew"
	rule := domain.SemanticAssertion{Change: &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: &before, After: &after}}
	evs := []domain.Evidence{
		{ID: "ev-ok", Excerpt: "The default value of spec.keyPolicy changed from Keep to Renew."},
		{ID: "ev-bad", Excerpt: "The default value of spec.keyPolicy is Never."},
	}
	cit := domain.ProofMethod{Method: domain.ProofCitationCheck, Checks: []domain.ProofCheck{
		{Subject: "ev-ok", Outcome: domain.MethodPass}, {Subject: "ev-bad", Outcome: domain.MethodFail}}}

	m := CheckSemantics(rule, evs, cit)
	if m.Outcome != domain.MethodPass || len(m.Evidence) != 1 || m.Evidence[0].ID != "ev-ok" || m.Level != domain.LevelDeterministic {
		t.Fatalf("consistent rule: %s %s (evidence %v)", m.Outcome, m.Detail, m.Evidence)
	}

	// a statement adding a token the established passage lacks
	added := rule
	added.Statement = "The default becomes `Renew`; the rotation policy is `Always`."
	if m := CheckSemantics(added, evs, cit); m.Outcome != domain.MethodFail || !strings.Contains(m.Detail, "`Always`") {
		t.Errorf("added token: %s %s", m.Outcome, m.Detail)
	}
	// only established passages count: the failing citation's text is never read
	wrong := "Never"
	other := domain.SemanticAssertion{Change: &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: &before, After: &wrong}}
	if m := CheckSemantics(other, evs, cit); m.Outcome != domain.MethodFail {
		t.Errorf("token only in a failed citation: %s %s", m.Outcome, m.Detail)
	}
	// a polarity flip
	flip := domain.SemanticAssertion{Statement: "The webhook accepts `Renew` keys."}
	flipEv := []domain.Evidence{{ID: "ev-ok", Excerpt: "The webhook rejects `Renew` keys."}}
	if m := CheckSemantics(flip, flipEv, cit); m.Outcome != domain.MethodFail || !strings.Contains(m.Detail, "opposite polarity") {
		t.Errorf("flip: %s %s", m.Outcome, m.Detail)
	}
	// nothing to compare: routed to the human reviewer
	prose := domain.SemanticAssertion{Statement: "Keys are handled differently after the upgrade."}
	if m := CheckSemantics(prose, evs, cit); m.Outcome != domain.MethodInconclusive || !strings.Contains(m.Detail, "human reviewer") {
		t.Errorf("no token: %s %s", m.Outcome, m.Detail)
	}
	// no established passage
	none := domain.ProofMethod{Method: domain.ProofCitationCheck, Checks: []domain.ProofCheck{{Subject: "ev-ok", Outcome: domain.MethodFail}}}
	if m := CheckSemantics(rule, evs, none); m.Outcome != domain.MethodNotRun {
		t.Errorf("no established passage: %s %s", m.Outcome, m.Detail)
	}
}

func TestProofOutcomeWithSemanticCheck(t *testing.T) {
	pass := func(k domain.ProofMethodKind) domain.ProofMethod { return domain.ProofMethod{Method: k, Outcome: domain.MethodPass} }
	with := func(o domain.MethodOutcome) []domain.ProofMethod {
		return []domain.ProofMethod{pass(domain.ProofCitationCheck), {Method: domain.ProofSemanticCheck, Outcome: o}, pass(domain.ProofRuleExercise)}
	}
	for o, want := range map[domain.MethodOutcome]domain.ProofOutcome{
		domain.MethodPass:         domain.ProofConfirmed,
		domain.MethodInconclusive: domain.ProofConfirmed, // left to the human reviewer, who must accept anyway
		domain.MethodNotRun:       domain.ProofConfirmed,
		domain.MethodFail:         domain.ProofRefuted,
	} {
		if got := domain.ProofOutcomeOf(with(o)); got != want {
			t.Errorf("semantic-check %s: outcome %s, want %s", o, got, want)
		}
	}
	// it never replaces a required method
	only := []domain.ProofMethod{pass(domain.ProofCitationCheck), pass(domain.ProofSemanticCheck)}
	if got := domain.ProofOutcomeOf(only); got != domain.ProofInconclusive {
		t.Errorf("without the rule exercise: %s, want inconclusive", got)
	}
}
