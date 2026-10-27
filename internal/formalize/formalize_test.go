package formalize

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

func TestPrecompute(t *testing.T) {
	p := precompute(t, newFixture().input())
	var cited []string
	for _, e := range p.Evidence {
		cited = append(cited, string(e.ID))
		if up := e.ID != "ev-env"; e.Upstream != up {
			t.Errorf("%s upstream = %v", e.ID, e.Upstream)
		}
	}
	// verified + upstream behaviour + the item's claim; never another item's evidence
	if got := strings.Join(cited, ","); got != "ev-crd,ev-env,ev-note" {
		t.Errorf("citable evidence = %s", got)
	}
	kinds := map[string]int{}
	for _, u := range p.Uncertainties {
		kinds[u.Kind]++
		if !strings.HasPrefix(u.ID, UncertaintyIDPrefix) || u.Text == "" {
			t.Errorf("uncertainty %+v", u)
		}
	}
	want := map[string]int{UncertaintyUnknown: 1, UncertaintyBehaviour: 1, UncertaintyDefault: 1, UncertaintyPrecondition: 1}
	for k, n := range want {
		if kinds[k] != n {
			t.Errorf("%d uncertainties of kind %s, want %d (all: %+v)", kinds[k], k, n, p.Uncertainties)
		}
	}
	if len(p.Basis) != 2 || p.Basis[0].ID != clm1 || p.Basis[1].ID != vr1 {
		t.Errorf("basis = %+v", p.Basis)
	}
	if _, err := Precompute(Input{Investigation: newFixture().Inv, Bundle: newFixture().Bundle, Assessment: "aas-missing"}); err == nil {
		t.Error("an unknown assessment is accepted")
	}
}

func TestFloor(t *testing.T) {
	ctx := context.Background()
	p := precompute(t, newFixture().input())
	if v := Floor(ctx, p, validAnswer(p)); len(v) != 0 {
		t.Fatalf("the valid answer is rejected: %v", v)
	}
	cases := []struct {
		name   string
		mutate func(a *Answer)
		want   string
	}{
		{"a foreign citation", func(a *Answer) { a.Hypotheses[0].Citations = append(a.Hypotheses[0].Citations, "ev-foreign") }, "is not evidence the assessment established"},
		{"an invented citation", func(a *Answer) { a.Hypotheses[1].Citations = []string{"ev-invented"} }, "is not evidence the assessment established"},
		{"a dropped unknown", func(a *Answer) { a.Hypotheses[1].Keeps = a.Hypotheses[1].Keeps[1:] }, "is dropped"},
		{"a dropped precondition", func(a *Answer) { a.Hypotheses[1].Rule = nil }, "is dropped"},
		{"bounds beyond the run", func(a *Answer) { a.Hypotheses[0].Rule.Release.To = "1.2.0" }, "not within this run's transition"},
		{"partial-rule bounds beyond the run", func(a *Answer) { a.Hypotheses[1].Rule.Release.From = "0.9.0" }, "not within this run's transition"},
		{"another product", func(a *Answer) { a.Hypotheses[0].Rule.Assertion.Subject.Product = "other" }, "is not this investigation's product"},
		{"an unevidenced default", func(a *Answer) {
			ex := &a.Hypotheses[0].Rule.Assertion.Applicability.Exposure
			ex.Of = []domain.Condition{{Op: domain.OpAny, Of: []domain.Condition{
				{Op: domain.OpField, Path: "spec.mode", State: domain.StateUnset},
				{Op: domain.OpField, Path: "spec.mode", State: domain.StateEquals, Values: []string{`"strict"`}},
			}}}
		}, "treats an unset field as a value"},
		{"an unevidenced default via not(set)", func(a *Answer) {
			ex := &a.Hypotheses[0].Rule.Assertion.Applicability.Exposure
			ex.Of = []domain.Condition{{Op: domain.OpAny, Of: []domain.Condition{
				{Op: domain.OpNot, Of: []domain.Condition{{Op: domain.OpField, Path: "spec.mode", State: domain.StateSet}}},
				{Op: domain.OpField, Path: "spec.mode", State: domain.StateNotEquals, Values: []string{`"legacy"`}},
			}}}
		}, "treats an unset field as a value"},
		{"an undecidable rule", func(a *Answer) {
			ex := &a.Hypotheses[0].Rule.Assertion.Applicability.Exposure
			ex.Of = append(ex.Of, domain.Condition{Op: domain.OpUndecidable, Reason: domain.UnknownEvidenceGap, Needed: "x"})
		}, "undecidable"},
		{"a basis of another run", func(a *Answer) { a.Hypotheses[0].EnvironmentBasis = []string{"clm-ffffffffffff"} }, "environmentBasis"},
		{"a rule without upstream citation", func(a *Answer) { a.Hypotheses[0].Citations = []string{"ev-env"} }, "no citation is upstream text"},
		{"a rule keeping an uncertainty", func(a *Answer) { a.Hypotheses[0].Keeps = []string{p.Uncertainties[0].ID} }, "a rule takes no"},
		{"a record without limitation", func(a *Answer) { a.Hypotheses[1].Limitation = "" }, "states its grammar limitation"},
		{"a repeated claim", func(a *Answer) { a.Hypotheses[1].Claim = a.Hypotheses[0].Claim }, "repeats the claim"},
		{"an unknown kept id", func(a *Answer) { a.Hypotheses[1].Keeps = append(a.Hypotheses[1].Keeps, "unc-nope") }, "not an uncertainty id"},
		{"no hypothesis, no reason", func(a *Answer) { a.Hypotheses = nil }, "no noHypothesisReason"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := validAnswer(p)
			c.mutate(a)
			v := Floor(ctx, p, a)
			if !strings.Contains(strings.Join(v, "\n"), c.want) {
				t.Errorf("violations %q do not contain %q", v, c.want)
			}
		})
	}
	// strict decoding: a field the schema does not define, also inside a rule
	if _, err := decode(`{"hypotheses":[{"kind":"rule","claim":"x","rule":{"assertion":{"subject":{"family":"crd-field","product":"acme","default":"x"}}}}],"noHypothesisReason":""}`); err == nil {
		t.Error("an unknown field inside a rule is accepted")
	}
	if _, err := decode(`{"hypotheses":[],"noHypothesisReason":"x","verified":true}`); err == nil {
		t.Error("an unknown top-level field is accepted")
	}
}

func TestUnrepresentableClaimIsKept(t *testing.T) {
	f := newFixture()
	p := precompute(t, f.input())
	c := &scripted{texts: []string{answerJSON(t, validAnswer(p))}}
	r, err := (&Formalizer{Client: c}).Formalize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusComplete || len(r.Hypotheses) != 2 {
		t.Fatalf("record %s with %d hypotheses", r.Status, len(r.Hypotheses))
	}
	var rule, rec *domain.Hypothesis
	for i := range r.Hypotheses {
		if r.Hypotheses[i].Mitigation != nil {
			rule = &r.Hypotheses[i]
		} else {
			rec = &r.Hypotheses[i]
		}
	}
	if rule == nil || rec == nil {
		t.Fatalf("hypotheses = %+v", r.Hypotheses)
	}
	pv := rec.ProposedVerification
	if !strings.HasPrefix(rec.MitigationRejected, "not representable as a mitigation rule: a consumer's reload behaviour") ||
		rec.Statement != "The consumer keeps working only if it reloads its configuration." || len(rec.Citations) != 1 ||
		pv == nil || pv.Assessment != aas1 || pv.Method == "" || len(pv.Conditions) != 1 || pv.Rule == nil ||
		len(pv.Uncertainties) != len(p.Uncertainties) {
		t.Errorf("record = %+v, proposed verification = %+v", rec, pv)
	}
	// the precondition is kept verbatim in the partial rule's undecidable leaf
	leaf := pv.Rule.Assertion.Applicability.Exposure.Of[1]
	if leaf.Op != domain.OpUndecidable || leaf.Needed != "the workloads reload their configuration" {
		t.Errorf("undecidable leaf = %+v", leaf)
	}
	// the rule carries every uncertainty, for the reviewer too
	if len(rule.ProposedVerification.Uncertainties) != len(p.Uncertainties) || rule.ProposedVerification.Rule != nil ||
		!strings.Contains(rule.Mitigation.RequestedVerification, "kept by formalization") ||
		!strings.Contains(rule.Mitigation.RequestedVerification, "whether Deployment/ns/app depends on strict mode") {
		t.Errorf("rule = %+v / %+v", rule.Mitigation, rule.ProposedVerification)
	}
	if rule.Mitigation.Item != it1 || len(rule.Mitigation.EnvironmentBasis) != 1 {
		t.Errorf("spec = %+v", rule.Mitigation)
	}
	for _, h := range r.Hypotheses {
		if err := h.Validate(); err != nil {
			t.Error(err)
		}
	}
}

func TestCallThroughExchangeReplaysIdentically(t *testing.T) {
	f := newFixture()
	p := precompute(t, f.input())
	xdir, cdir := t.TempDir(), t.TempDir()
	fz := &Formalizer{Client: &llm.Cache{Dir: cdir, Inner: &llm.Exchange{Dir: xdir}}}
	if _, err := fz.Formalize(context.Background(), f.input()); !errors.Is(err, llm.ErrPending) {
		t.Fatalf("first call = %v, want pending", err)
	}
	reqs, _ := filepath.Glob(filepath.Join(xdir, "*.request.json"))
	if len(reqs) != 1 {
		t.Fatalf("%d exchange requests", len(reqs))
	}
	var xr llm.ExchangeRequest
	raw, _ := os.ReadFile(reqs[0])
	if err := json.Unmarshal(raw, &xr); err != nil {
		t.Fatal(err)
	}
	if xr.Request.Model != DefaultModel || len(xr.Request.Messages) != 1 || len(xr.Request.JSONSchema) == 0 {
		t.Errorf("request: model %q, %d messages, schema %d bytes", xr.Request.Model, len(xr.Request.Messages), len(xr.Request.JSONSchema))
	}
	resp := llm.ExchangeResponse{Format: llm.ExchangeResponseFormat, PromptDigest: xr.PromptDigest, Model: DefaultModel, ModelVersion: "claude-opus-5-5",
		GeneratedAt: fixedNow, CallID: "session-1", Output: json.RawMessage(answerJSON(t, validAnswer(p)))}
	b, _ := json.Marshal(resp)
	if err := os.WriteFile(strings.TrimSuffix(reqs[0], ".request.json")+".response.json", b, 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := fz.Formalize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	pv := r.Provenance
	if r.Status != StatusComplete || pv.Model != DefaultModel || pv.ModelVersion == "" || pv.PromptVersion != PromptVersion ||
		!strings.HasPrefix(pv.PromptDigest, "sha256:") || len(pv.InputEvidence) == 0 || pv.GeneratedAt == nil || pv.CallID != "session-1" ||
		pv.Provider != "anthropic" || pv.Producer != Producer || r.StatementKind != domain.StatementUnverifiedProse || r.Input != p.Digest() {
		t.Errorf("provenance incomplete: %+v", pv)
	}
	for _, h := range r.Hypotheses {
		if h.Provenance.CallID != "session-1" || h.Provenance.PromptDigest != pv.PromptDigest || h.Provenance.Producer != Producer {
			t.Errorf("hypothesis provenance = %+v", h.Provenance)
		}
	}
	// replay offline: no exchange, the cache answers
	r2, err := (&Formalizer{Client: &llm.Cache{Dir: cdir}}).Formalize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	a1, _ := json.Marshal(r)
	a2, _ := json.Marshal(r2)
	if string(a1) != string(a2) {
		t.Error("the replayed record differs")
	}
}

func TestBoundedRetryAndRecordedFailure(t *testing.T) {
	f := newFixture()
	p := precompute(t, f.input())
	bad := validAnswer(p)
	bad.Hypotheses[1].Keeps = nil // drops the uncertainties
	good := answerJSON(t, validAnswer(p))

	// a violating answer, then a decode failure, then a valid one
	c := &scripted{texts: []string{answerJSON(t, bad), "not json", good}}
	r, err := (&Formalizer{Client: c}).Formalize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusComplete || len(c.reqs) != 3 {
		t.Fatalf("status %s after %d calls", r.Status, len(c.reqs))
	}
	retry := c.reqs[1].Messages[0].Content
	if !strings.Contains(retry, "Attempt 2: your previous answer was rejected") || !strings.Contains(retry, "is dropped") ||
		!strings.Contains(retry, answerJSON(t, bad)) {
		t.Error("the retry does not carry the violations and the rejected answer")
	}

	// three violating answers: a recorded failure, never repaired
	c = &scripted{texts: []string{answerJSON(t, bad), answerJSON(t, bad), answerJSON(t, bad), good}}
	r, err = (&Formalizer{Client: c}).Formalize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusFailed || r.Failure.Attempts != MaxAttempts || len(r.Failure.Answers) != MaxAttempts || len(r.Hypotheses) != 0 ||
		len(c.reqs) != MaxAttempts || !strings.Contains(strings.Join(r.Failure.Violations, "\n"), "is dropped") {
		t.Errorf("failed record = %+v after %d calls", r, len(c.reqs))
	}
	if err := r.Validate(); err != nil {
		t.Error(err)
	}

	// no answer at all is an error, not a record
	if _, err := (&Formalizer{Client: &scripted{}}).Formalize(context.Background(), f.input()); err == nil {
		t.Error("no answer gave a record")
	}
}

func TestNoHypothesisIsRecorded(t *testing.T) {
	f := newFixture()
	c := &scripted{texts: []string{`{"hypotheses":[],"noHypothesisReason":"the assessment states no testable claim"}`}}
	r, err := (&Formalizer{Client: c}).Formalize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusComplete || len(r.Hypotheses) != 0 || r.NoHypothesisReason == "" {
		t.Errorf("record = %+v", r)
	}
}

// Formalization reads the run record and never changes it: the findings,
// decision, hypotheses and every other field (what the classifier reads)
// are byte-identical afterwards, and the formalized hypotheses live only in
// the formalization record.
func TestPolicyIsolation(t *testing.T) {
	f := newFixture()
	f.Inv.Findings = []domain.InvestigationFinding{{ID: "fnd-000000000001", ChangeItem: it1, Class: domain.FindingPotentiallyAffected}}
	f.Inv.Decision = &domain.PolicyDecision{}
	before, _ := json.Marshal(f.Inv)
	p := precompute(t, f.input())
	r, err := (&Formalizer{Client: &scripted{texts: []string{answerJSON(t, validAnswer(p))}}}).Formalize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(f.Inv)
	if string(before) != string(after) {
		t.Error("formalization changed the run record")
	}
	if len(f.Inv.Hypotheses) != 0 || len(r.Hypotheses) == 0 {
		t.Error("formalized hypotheses reached the run record")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	f := newFixture()
	p := precompute(t, f.input())
	r, err := (&Formalizer{Client: &scripted{texts: []string{answerJSON(t, validAnswer(p))}}}).Formalize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	st := Store{Dir: t.TempDir()}
	if err := st.Put(r); err != nil {
		t.Fatal(err)
	}
	if err := st.Put(r); err != nil { // content-addressed: idempotent
		t.Fatal(err)
	}
	rs, err := st.List(context.Background(), f.Inv.ID)
	if err != nil || len(rs) != 1 || rs[0].ID != r.ID {
		t.Fatalf("list = %v, %v", rs, err)
	}
	if hs := Hypotheses(rs); len(hs) != 2 {
		t.Errorf("%d hypotheses", len(hs))
	}
	bad := *r
	bad.Status = "maybe"
	if err := st.Put(&bad); err == nil {
		t.Error("an invalid record is stored")
	}
}

func TestPromptIsGenericAndVersioned(t *testing.T) {
	pr, err := LoadPrompt(PromptVersion)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Version != "formalize/v1" || !strings.HasPrefix(pr.Digest(), "sha256:") {
		t.Errorf("prompt %s digest %s", pr.Version, pr.Digest())
	}
	text := strings.ToLower(pr.System + pr.Task + string(pr.Schema))
	for _, banned := range []string{"cert-manager", "ingress-nginx", "isca", "ca.crt", "ledger", "payments", "docs-eu", "strict-path", "pathtype", "privatekey", "rotationpolicy"} {
		if strings.Contains(text, banned) {
			t.Errorf("the prompt names %q", banned)
		}
	}
	if regexp.MustCompile(`\bv?\d+\.\d+\.\d+\b`).MatchString(text) {
		t.Error("the prompt names a version")
	}
	if _, err := LoadPrompt("formalize/v0"); err == nil {
		t.Error("an unknown prompt loads")
	}
}
