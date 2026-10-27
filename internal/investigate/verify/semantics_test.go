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

	m := CheckSemantics(rule, evs, cit, nil)
	if m.Outcome != domain.MethodPass || len(m.Evidence) != 1 || m.Evidence[0].ID != "ev-ok" || m.Level != domain.LevelDeterministic {
		t.Fatalf("consistent rule: %s %s (evidence %v)", m.Outcome, m.Detail, m.Evidence)
	}

	// a statement adding a token the established passage lacks
	added := rule
	added.Statement = "The default becomes `Renew`; the rotation policy is `Always`."
	if m := CheckSemantics(added, evs, cit, nil); m.Outcome != domain.MethodFail || !strings.Contains(m.Detail, "`Always`") {
		t.Errorf("added token: %s %s", m.Outcome, m.Detail)
	}
	// only established passages count: the failing citation's text is never read
	wrong := "Never"
	other := domain.SemanticAssertion{Change: &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: &before, After: &wrong}}
	if m := CheckSemantics(other, evs, cit, nil); m.Outcome != domain.MethodFail {
		t.Errorf("token only in a failed citation: %s %s", m.Outcome, m.Detail)
	}
	// a polarity flip
	flip := domain.SemanticAssertion{Statement: "The webhook accepts `Renew` keys."}
	flipEv := []domain.Evidence{{ID: "ev-ok", Excerpt: "The webhook rejects `Renew` keys."}}
	if m := CheckSemantics(flip, flipEv, cit, nil); m.Outcome != domain.MethodFail || !strings.Contains(m.Detail, "opposite polarity") {
		t.Errorf("flip: %s %s", m.Outcome, m.Detail)
	}
	// nothing to compare: routed to the human reviewer
	prose := domain.SemanticAssertion{Statement: "Keys are handled differently after the upgrade."}
	if m := CheckSemantics(prose, evs, cit, nil); m.Outcome != domain.MethodInconclusive || !strings.Contains(m.Detail, "human reviewer") {
		t.Errorf("no token: %s %s", m.Outcome, m.Detail)
	}
	// no established passage
	none := domain.ProofMethod{Method: domain.ProofCitationCheck, Checks: []domain.ProofCheck{{Subject: "ev-ok", Outcome: domain.MethodFail}}}
	if m := CheckSemantics(rule, evs, none, nil); m.Outcome != domain.MethodNotRun {
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

// A scope refinement's statement names the catalog role's condition values
// (lane verify3, D-0026): they come from the catalog and are established by
// the consumer exercise, so they are excluded from the semantic check and
// recorded with that method; every other token is checked as before.
func TestSemanticCheckExcludesRoleTokens(t *testing.T) {
	before, after := "Keep", "Renew"
	base := domain.SemanticAssertion{Change: &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: &before, After: &after},
		Statement: "Badges that leave keyPolicy unset get a new key."}
	evs := []domain.Evidence{{ID: "ev-ok", Excerpt: "The default value of spec.keyPolicy changed from Keep to Renew."}}
	cit := domain.ProofMethod{Method: domain.ProofCitationCheck, Checks: []domain.ProofCheck{{Subject: "ev-ok", Outcome: domain.MethodPass}}}
	r := &domain.ScopeRefinement{Consumer: "signer-trust", Role: []domain.RoleField{{Path: "spec.signer", Value: "true"}},
		Outside: []domain.RoleField{{Path: "spec.signer", Value: "false"}}}
	refined := base
	refined.Statement = strings.TrimSpace(base.Statement + r.Clause())

	// without the exclusion the role's booleans are refuted (the development-run shape)
	if m := CheckSemantics(refined, evs, cit, nil); m.Outcome != domain.MethodFail || !strings.Contains(m.Detail, "boolean `true`") {
		t.Fatalf("no refinement given: %s %s", m.Outcome, m.Detail)
	}
	m := CheckSemantics(refined, evs, cit, r)
	if m.Outcome != domain.MethodPass {
		t.Fatalf("role tokens not excluded: %s %s", m.Outcome, m.Detail)
	}
	var excluded []string
	for _, c := range m.Checks {
		if c.Outcome == domain.MethodNotRun {
			excluded = append(excluded, c.Subject)
			if !strings.Contains(c.Detail, "consumer exercise (role signer-trust)") || !strings.Contains(c.Detail, "spec.signer = ") {
				t.Errorf("exclusion without its condition and method: %s", c.Detail)
			}
		}
	}
	if strings.Join(excluded, ",") != "boolean `true`,boolean `false`" || !strings.Contains(m.Detail, "2 token(s) excluded") {
		t.Fatalf("exclusions %v: %s", excluded, m.Detail)
	}

	// a non-role token in the base statement still fails
	bad := refined
	bad.Statement = strings.TrimSpace("Badges that leave keyPolicy unset get a new key; the rotation policy is `Always`." + r.Clause())
	if m := CheckSemantics(bad, evs, cit, r); m.Outcome != domain.MethodFail || !strings.Contains(m.Detail, "`Always`") {
		t.Fatalf("non-role token: %s %s", m.Outcome, m.Detail)
	}
	// a role value also asserted outside the clause is checked there
	also := refined
	also.Statement = strings.TrimSpace("Badges that leave keyPolicy unset are enabled: true." + r.Clause())
	if m := CheckSemantics(also, evs, cit, r); m.Outcome != domain.MethodFail || !strings.Contains(m.Detail, "boolean `true`") {
		t.Fatalf("a role value in the base statement: %s %s", m.Outcome, m.Detail)
	}
	// a statement that does not carry the clause verbatim excludes nothing
	edited := refined
	edited.Statement = "Badges that leave keyPolicy unset get a new key; only spec.signer = true is harmed, spec.signer = false is shielded."
	if m := CheckSemantics(edited, evs, cit, r); m.Outcome != domain.MethodFail {
		t.Fatalf("edited statement: %s %s", m.Outcome, m.Detail)
	}
}
