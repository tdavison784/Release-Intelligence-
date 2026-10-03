package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

func rotationCandidate(t *testing.T) domain.SemanticCandidate {
	t.Helper()
	return candidateFor(t, Candidates(rotationEdge(), t0), "chg-guide")
}

// fullAnswer is a correct full answer citing the candidate's first evidence.
func fullAnswer(c domain.SemanticCandidate, mod func(m map[string]any)) string {
	m := map[string]any{
		"statement": "Widget.spec.rotation.policy defaults to Always instead of Never.",
		"subject":   map[string]any{"determination": "asserted", "family": "crd-field", "group": "example.io", "kind": "Widget", "path": "spec.rotation.policy"},
		"change":    map[string]any{"determination": "asserted", "type": "default-changed", "before": "Never", "after": "Always"},
		"applicability": map[string]any{"determination": "asserted",
			"exposure": map[string]any{"op": "resource", "group": "example.io", "kind": "Widget", "of": []any{map[string]any{"op": "field", "path": "spec.rotation.policy", "state": "unset"}}},
			"overlap":  map[string]any{"op": "resource", "group": "example.io", "kind": "Widget", "of": []any{map[string]any{"op": "field", "path": "spec.rotation.policy", "state": "set"}}}},
		"consequence":    map[string]any{"determination": "asserted", "kind": "behavior-change", "statement": "Private keys are rotated on every re-issuance.", "severity": "medium"},
		"suggestedClass": "review-required",
		"citations":      []any{string(c.Evidence[0].ID)},
		"confidence":     "medium",
	}
	if mod != nil {
		mod(m)
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func propose(t *testing.T, c domain.SemanticCandidate, task domain.ProposalTask, answer string) (*domain.SemanticProposal, error) {
	t.Helper()
	f := &llm.Fake{Model: "claude-sonnet-5-5", Responses: []llm.FakeResponse{{Text: answer}}}
	p := NewLLMProposer(f, "anthropic", "claude-sonnet-5-5", ProposerOptions{Clock: func() time.Time { return t0 }})
	return p.Propose(context.Background(), knowledge.ProposalRequest{Candidate: c, Task: task})
}

func TestProposeFullAnswer(t *testing.T) {
	c := rotationCandidate(t)
	p, err := propose(t, c, domain.TaskFull, fullAnswer(c, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.ValidateAgainst(c); err != nil {
		t.Fatal(err)
	}
	a := p.Assertion
	if a.Subject.Product != c.Product || a.Subject.Key() != "crd-field:widget-operator|group=example.io|kind=Widget|path=spec.rotation.policy" {
		t.Errorf("subject = %+v", a.Subject)
	}
	if *a.Change.Before != `"Never"` || *a.Change.After != `"Always"` {
		t.Errorf("values are JSON-encoded literals, got %s → %s", *a.Change.Before, *a.Change.After)
	}
	if a.Consequence.ExposedClass != domain.ImpactReviewRequired {
		t.Errorf("the class follows the kind: %s", a.Consequence.ExposedClass)
	}
	pv := p.Provenance
	if pv.Method != domain.MethodAI || pv.Model != "claude-sonnet-5-5" || pv.PromptVersion != "semantic-full/v2" ||
		pv.PromptDigest == "" || len(pv.InputEvidence) != len(c.Evidence) || pv.GeneratedAt == nil || p.Provider != "anthropic" {
		t.Errorf("incomplete provenance %+v", pv)
	}
}

func TestProposeRefusals(t *testing.T) {
	c := rotationCandidate(t)
	cases := map[string]struct {
		answer string
		want   string
	}{
		"hallucinated citation":                                          {fullAnswer(c, func(m map[string]any) { m["citations"] = []any{"ev-000000000000"} }), "did not show"},
		"no citation for an assertion":                                   {fullAnswer(c, func(m map[string]any) { m["citations"] = []any{} }), "must cite evidence"},
		"action-required request without an action-eligible consequence": {fullAnswer(c, func(m map[string]any) { m["suggestedClass"] = "action-required" }), "action-eligible consequence"},
		"model emits not-affected":                                       {fullAnswer(c, func(m map[string]any) { m["suggestedClass"] = "not-affected" }), "schema"},
		"model states an exposed class": {fullAnswer(c, func(m map[string]any) {
			m["consequence"].(map[string]any)["exposedClass"] = "action-required"
		}), "schema"},
		"high confidence":  {fullAnswer(c, func(m map[string]any) { m["confidence"] = "high" }), "schema"},
		"missing aspect":   {fullAnswer(c, func(m map[string]any) { delete(m, "consequence") }), "schema"},
		"not json":         {"The change is a default change.", "not JSON"},
		"unknown family":   {fullAnswer(c, func(m map[string]any) { m["subject"].(map[string]any)["family"] = "helm-chart" }), "schema"},
		"family field mix": {fullAnswer(c, func(m map[string]any) { m["subject"].(map[string]any)["name"] = "x" }), "not part of this family"},
		"before equals after": {fullAnswer(c, func(m map[string]any) {
			m["change"].(map[string]any)["after"] = "Never"
		}), "before equals after"},
		"scoped leaf outside scope": {fullAnswer(c, func(m map[string]any) {
			m["applicability"].(map[string]any)["exposure"] = map[string]any{"op": "field", "path": "spec.x", "state": "unset"}
			delete(m["applicability"].(map[string]any), "overlap")
		}), "only valid inside"},
		"failure kind without statement": {fullAnswer(c, func(m map[string]any) {
			m["consequence"] = map[string]any{"determination": "asserted", "kind": "workload-failure", "statement": ""}
		}), "statement"},
		"undetermined without reason": {fullAnswer(c, func(m map[string]any) {
			m["consequence"] = map[string]any{"determination": "undetermined", "reason": " "}
		}), "without a reason"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := propose(t, c, domain.TaskFull, tc.answer)
			if err == nil {
				t.Fatalf("accepted: %+v", p)
			}
			var pe *ProposalError
			if !errors.As(err, &pe) || pe.Kind != "rejected" || pe.PromptDigest == "" {
				t.Fatalf("want a rejected ProposalError with the prompt digest, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("reason %q lacks %q", err, tc.want)
			}
		})
	}
}

func TestProposeAbstentionIsAProposal(t *testing.T) {
	c := rotationCandidate(t)
	ans := `{"statement":"","subject":{"determination":"undetermined","reason":"the text names no field"},` +
		`"change":{"determination":"undetermined","reason":"no before/after"},"confidence":"low","citations":[]}`
	p, err := propose(t, c, domain.TaskSemanticMapping, ans)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Assertion.Empty() || len(p.Undetermined) != 2 || !strings.Contains(p.UndeterminedReason, "subject: the text names no field") {
		t.Errorf("abstention = %+v", p)
	}
}

func TestProposeValueEncodingAndRelationship(t *testing.T) {
	c := rotationCandidate(t)
	ans := map[string]any{
		"statement": "Requires ingress-nginx >= 1.12.0 for path handling.",
		"subject":   map[string]any{"determination": "asserted", "family": "product-relationship", "name": "ingress-nginx"},
		"change":    map[string]any{"determination": "asserted", "type": "requirement-changed", "after": ">=1.12.0"},
		"applicability": map[string]any{"determination": "asserted", "exposure": map[string]any{"op": "all", "of": []any{
			map[string]any{"op": "product-version", "name": "ingress-nginx", "state": "out-of-range", "range": ">=1.12.0"},
			map[string]any{"op": "feature-gate", "name": "ValidateThing", "state": "enabled"},
			map[string]any{"op": "values-key", "path": "extraArgs", "state": "has-token-key", "values": []any{"ValidateThing"}},
			map[string]any{"op": "values-key", "path": "replicas", "state": "equals", "values": []any{3, "3"}},
		}}},
		"citations":  []any{string(c.Evidence[0].ID)},
		"confidence": "low",
	}
	b, _ := json.Marshal(ans)
	p, err := propose(t, c, domain.TaskRelationship, string(b))
	if err != nil {
		t.Fatal(err)
	}
	if *p.Assertion.Change.After != ">=1.12.0" {
		t.Errorf("a requirement stays a raw constraint: %q", *p.Assertion.Change.After)
	}
	ops := p.Assertion.Applicability.Exposure.Of
	if ops[2].Values[0] != "ValidateThing" {
		t.Errorf("tokens are plain: %q", ops[2].Values[0])
	}
	if ops[3].Values[0] != "3" || ops[3].Values[1] != `"3"` {
		t.Errorf("equals values are JSON literals: %q", ops[3].Values)
	}
}

func TestProposeDuplicateTask(t *testing.T) {
	c := rotationCandidate(t)
	known := []knowledge.FactSummary{{ID: "vf-aaaaaaaaaaaa", Subject: "crd-field:widget-operator|kind=Widget", Statement: "rotation default"}}
	run := func(answer string) (*domain.SemanticProposal, error) {
		f := &llm.Fake{Model: "m", Responses: []llm.FakeResponse{{Text: answer}}}
		p := NewLLMProposer(f, "anthropic", "m", ProposerOptions{})
		return p.Propose(context.Background(), knowledge.ProposalRequest{Candidate: c, Task: domain.TaskDuplicate, KnownFacts: known})
	}
	cite := string(c.Evidence[0].ID)
	p, err := run(`{"duplicateOf":"vf-aaaaaaaaaaaa","reason":"same default change","citations":["` + cite + `"],"confidence":"medium"}`)
	if err != nil || p.DuplicateOf != "vf-aaaaaaaaaaaa" {
		t.Fatalf("duplicate = %+v, %v", p, err)
	}
	p, err = run(`{"duplicateOf":"none","reason":"different","citations":[],"confidence":"low"}`)
	if err != nil || p.DuplicateOf != "" {
		t.Fatalf("none = %+v, %v", p, err)
	}
	if _, err := run(`{"duplicateOf":"vf-bbbbbbbbbbbb","reason":"x","citations":["` + cite + `"],"confidence":"low"}`); err == nil {
		t.Fatal("a fact id that was not shown must be refused")
	}
}

func TestProposeTransportOutcomes(t *testing.T) {
	c := rotationCandidate(t)
	dir := t.TempDir()
	p := NewLLMProposer(&llm.Exchange{Dir: dir}, "anthropic", "claude-haiku-4-5", ProposerOptions{})
	req := knowledge.ProposalRequest{Candidate: c, Task: domain.TaskFull}
	_, err := p.Propose(context.Background(), req)
	var pe *ProposalError
	if !errors.As(err, &pe) || pe.Kind != "pending" {
		t.Fatalf("want pending, got %v", err)
	}
	// answer the exchange request like the runner does, then replay
	reqs, _ := filepath.Glob(filepath.Join(dir, "*.request.json"))
	if len(reqs) != 1 {
		t.Fatalf("want one request file, got %v", reqs)
	}
	var xr llm.ExchangeRequest
	b, _ := os.ReadFile(reqs[0])
	if err := json.Unmarshal(b, &xr); err != nil {
		t.Fatal(err)
	}
	resp := llm.ExchangeResponse{Format: llm.ExchangeResponseFormat, PromptDigest: xr.PromptDigest, Model: "claude-haiku-4-5",
		ModelVersion: "claude-haiku-4-5-20251001", GeneratedAt: t0, CallID: "session-abc", Output: json.RawMessage(fullAnswer(c, nil))}
	out, _ := json.Marshal(resp)
	if err := os.WriteFile(filepath.Join(dir, xr.ResponseFile), out, 0o644); err != nil {
		t.Fatal(err)
	}
	prop, err := p.Propose(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if prop.Provenance.ModelVersion != "claude-haiku-4-5-20251001" || !prop.Provenance.GeneratedAt.Equal(t0) || prop.Provenance.CallID != "session-abc" {
		t.Errorf("provenance must come from the response: %+v", prop.Provenance)
	}

	// the model declined
	f := &llm.Fake{Responses: []llm.FakeResponse{{Err: &llm.RefusalError{Model: "m"}}}}
	_, err = NewLLMProposer(f, "anthropic", "m", ProposerOptions{}).Propose(context.Background(), req)
	if !errors.As(err, &pe) || pe.Kind != "refused" {
		t.Fatalf("want refused, got %v", err)
	}
}

func TestProposeAllRecordsEveryOutcome(t *testing.T) {
	cands := Candidates(rotationEdge(), t0)
	good := &llm.Fake{Model: "claude-opus-5-5", Respond: func(req llm.Request) (string, error) {
		// abstain on everything: always valid
		return `{"statement":"","subject":{"determination":"undetermined","reason":"r"},"change":{"determination":"undetermined","reason":"r"},"confidence":"low","citations":[]}`, nil
	}}
	broken := &llm.Fake{Model: "glm-5.3-flash", Respond: func(llm.Request) (string, error) { return "", errors.New("connection reset") }}
	ps := []knowledge.Proposer{
		NewLLMProposer(good, "anthropic", "claude-opus-5-5", ProposerOptions{}),
		NewLLMProposer(broken, "zai", "glm-5.3-flash", ProposerOptions{}),
	}
	props, fails := ProposeAllWith(context.Background(), cands, ps, []domain.ProposalTask{domain.TaskSemanticMapping, domain.TaskDuplicate},
		ProposeOptions{Parallel: 1, Clock: func() time.Time { return t0 }})
	if len(props) != len(cands) {
		t.Errorf("want one proposal per candidate from the working model, got %d", len(props))
	}
	if len(fails) != len(cands) {
		t.Fatalf("every broken attempt is recorded, got %d", len(fails))
	}
	transport, notAsked := 0, 0
	for _, f := range fails {
		switch {
		case strings.HasPrefix(f.Reason, "transport:"):
			transport++
		case strings.HasPrefix(f.Reason, "not asked:"):
			notAsked++
		}
		if f.Provider != "zai" || f.Model != "glm-5.3-flash" || f.CandidateID == "" {
			t.Errorf("failure lacks attribution: %+v", f)
		}
	}
	if transport != 3 || notAsked != len(cands)-3 {
		t.Errorf("transport=%d notAsked=%d: stop after 3 failures in a row", transport, notAsked)
	}
	// deterministic order: candidate order
	for i := range props {
		if props[i].CandidateID != cands[i].ID {
			t.Errorf("proposal %d out of order", i)
		}
	}
}

func TestProposeNullIsAValue(t *testing.T) {
	c := rotationCandidate(t)
	p, err := propose(t, c, domain.TaskFull, fullAnswer(c, func(m map[string]any) {
		m["change"] = map[string]any{"determination": "asserted", "type": "default-changed", "before": nil, "after": 1}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if *p.Assertion.Change.Before != "null" || *p.Assertion.Change.After != "1" {
		t.Errorf("nil → 1 must encode as null → 1, got %v → %v", p.Assertion.Change.Before, p.Assertion.Change.After)
	}
}

// PO-2: a model may REQUEST action-required with an action-eligible
// consequence; the request is recorded, never a classification. Outside a
// consequence task the request is not even expressible.
func TestProposeActionRequest(t *testing.T) {
	c := rotationCandidate(t)
	p, err := propose(t, c, domain.TaskFull, fullAnswer(c, func(m map[string]any) {
		m["suggestedClass"] = "action-required"
		m["consequence"] = map[string]any{"determination": "asserted", "kind": "workload-failure", "statement": "Challenges fail."}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if p.SuggestedClass != domain.ImpactActionRequired || p.Assertion.Consequence.ExposedClass != domain.ImpactActionRequired || p.Provenance.Confidence == domain.ConfidenceHigh {
		t.Errorf("request = %+v", p)
	}
	ans := `{"statement":"","subject":{"determination":"undetermined","reason":"r"},"change":{"determination":"undetermined","reason":"r"},` +
		`"confidence":"low","citations":[],"suggestedClass":"action-required"}`
	if _, err := propose(t, c, domain.TaskSemanticMapping, ans); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Errorf("semantic-mapping cannot request action-required: %v", err)
	}
}

// Every Complete is a separate call: two answers from one model have
// distinct call ids, hence distinct proposals (PO-1).
func TestSeparateCallsAreDistinctProposals(t *testing.T) {
	c := rotationCandidate(t)
	f := &llm.Fake{Model: "claude-opus-5-5", Respond: func(llm.Request) (string, error) { return fullAnswer(c, nil), nil }}
	p := NewLLMProposer(f, "anthropic", "claude-opus-5-5", ProposerOptions{})
	a, err1 := p.Propose(context.Background(), knowledge.ProposalRequest{Candidate: c, Task: domain.TaskFull})
	b, err2 := p.Propose(context.Background(), knowledge.ProposalRequest{Candidate: c, Task: domain.TaskFull})
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	if a.ID == b.ID || !domain.SeparateCalls(*a, *b) {
		t.Errorf("two calls must be two proposals: %s %s", a.ID, b.ID)
	}
}

// L1: an undecidable leaf may only state a runtime/evidence/identity gap.
// "configuration not visible" is the engine's verdict, never a model's.
func TestAvoidableUndecidableIsRefused(t *testing.T) {
	c := rotationCandidate(t)
	withLeaf := func(reason string) string {
		return fullAnswer(c, func(m map[string]any) {
			m["applicability"] = map[string]any{"determination": "asserted", "exposure": map[string]any{"op": "all", "of": []any{
				map[string]any{"op": "values-key", "path": "a.b", "state": "set"},
				map[string]any{"op": "undecidable", "reason": reason, "needed": "whether the config file sets x"},
			}}}
		})
	}
	// through a non-enforcing transport the schema refusal is the first line…
	if _, err := propose(t, c, domain.TaskFull, withLeaf("environment-visibility-gap")); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("environment-visibility-gap must be refused: %v", err)
	}
	// …and the validator is the second, for typed answers that skip the schema
	a, err := DecodeAnswer(domain.TaskFull, withLeaf("runtime-behavior-gap"))
	if err != nil {
		t.Fatal(err)
	}
	a.Applicability.Exposure.Of[1].Reason = "cross-product-context-gap"
	meta := AnswerMeta{Provider: "typesafe", Model: "m", ModelVersion: "m", PromptVersion: "semantic-full/v2", PromptDigest: "sha256:x",
		CallID: "c1", Input: []domain.EvidenceID{c.Evidence[0].ID}, GeneratedAt: t0}
	if _, err := ProposalFromAnswer(c, domain.TaskFull, a, meta); !errors.Is(err, ErrAvoidableUndecidable) {
		t.Fatalf("want ErrAvoidableUndecidable, got %v", err)
	}
	// a genuinely runtime condition stays expressible
	if p, err := propose(t, c, domain.TaskFull, withLeaf("runtime-behavior-gap")); err != nil || p.Assertion.Applicability == nil {
		t.Fatalf("runtime-behavior-gap must be accepted: %v", err)
	}
}
