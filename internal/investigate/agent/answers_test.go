package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
)

const promptV3 = "investigate-agent/v3"

func (f *fixture) v3session(t *testing.T, in investigate.InvestigateInput, validators ...investigate.Validator) *Session {
	t.Helper()
	in.Config = investigate.AgentConfig{Enabled: true, Driver: DriverScripted, Model: "test-model", PromptVersion: promptV3, Budget: investigate.DefaultAgentBudget}
	s, _, err := Prepare(f.st, in, validators, fixedNow)
	must(t, err)
	s.SetCall(CallInfo{ID: "call-1", Model: "test-model", ModelVersion: "test-model-2026", Provider: "test"})
	return s
}

func concern(t *testing.T, s *Session, f *fixture, statement string, extra map[string]any) string {
	t.Helper()
	args := map[string]any{"changeItem": f.behaviour, "stance": "raises-concern", "statement": statement,
		"citations": []map[string]any{{"artifact": f.notes.ID, "quote": "the default rotation policy is Always"}}}
	for k, v := range extra {
		args[k] = v
	}
	return ok(t, s, investigate.ToolRecordAssertion, args)["id"].(string)
}

// unanswered reports whether the classifier still lists the concern as an
// unanswered agent concern on the item's finding.
func unanswered(t *testing.T, f *fixture, out *investigate.AgentOutcome, vals []domain.ValidationRecord, assertion string) bool {
	t.Helper()
	c, err := classify.New().Classify(context.Background(), investigate.ClassifyInput{Bundle: f.b, Traversal: f.trav, Agent: out, Validations: vals,
		Config: investigate.Config{Policy: "classify.policy@v1"}})
	must(t, err)
	for _, fd := range c.Findings {
		if fd.ChangeItem != f.behaviour {
			continue
		}
		for _, g := range fd.Gaps {
			if g.Kind == domain.GapAgent && g.Subject == assertion {
				return true
			}
		}
		return false
	}
	t.Fatal("no finding for the behaviour item")
	return false
}

func TestV3RequestedValidationAnswersTheConcern(t *testing.T) {
	f := newFixture(t)
	s := f.v3session(t, f.input(investigate.AgentConfig{}), &fakeOverride{f: f})
	asr := concern(t, s, f, "the controller rotation default reaches demo-controller", map[string]any{"subjects": []string{"Deployment/demo/demo-controller"}})
	r := ok(t, s, investigate.ToolRequestValidation, map[string]any{"changeItem": f.behaviour, "kind": "override-inspection",
		"subjects": []string{"Deployment/demo/demo-controller"}, "answers": []string{asr}})
	links := r["answers"].([]any)
	if len(links) != 1 || links[0].(map[string]any)["linked"] != true {
		t.Fatalf("answer link: %v", r)
	}
	if !strings.Contains(string(mustJSON(r["validation"].(map[string]any)["answers"])), asr) {
		t.Fatalf("the record does not name the assertion it answers: %v", r["validation"])
	}
	out, err := s.Outcome(context.Background(), f.st, DriverScripted, investigate.DriverResult{Model: "test-model", ModelVersion: "v", Stop: "error:x"})
	must(t, err)
	if len(out.Validations) != 1 || !contains(out.Validations[0].Answers, asr) || len(out.AnswerLinks) != 1 || len(out.ValidationRequests[0].Answers) != 1 {
		t.Fatalf("outcome: %+v / %+v", out.Validations, out.AnswerLinks)
	}
	if err := runRecord(t, f, out).ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
	if unanswered(t, f, out, out.Validations, asr) {
		t.Fatal("the classifier still treats the answered concern as open")
	}
	// Control: the same concern and validation without the link stays open.
	f2 := newFixture(t)
	s2 := f2.v3session(t, f2.input(investigate.AgentConfig{}), &fakeOverride{f: f2})
	asr2 := concern(t, s2, f2, "the controller rotation default reaches demo-controller", map[string]any{"subjects": []string{"Deployment/demo/demo-controller"}})
	ok(t, s2, investigate.ToolRequestValidation, map[string]any{"changeItem": f2.behaviour, "kind": "override-inspection", "subjects": []string{"Deployment/demo/demo-controller"}})
	out2, err := s2.Outcome(context.Background(), f2.st, DriverScripted, investigate.DriverResult{Model: "test-model", ModelVersion: "v", Stop: "error:x"})
	must(t, err)
	if !unanswered(t, f2, out2, out2.Validations, asr2) {
		t.Fatal("an unlinked validation lifted the concern")
	}
}

func TestV3AnswerGuards(t *testing.T) {
	f := newFixture(t)
	s := f.v3session(t, f.input(investigate.AgentConfig{}), &fakeOverride{f: f})
	onDeploy := concern(t, s, f, "concern on the Deployment", map[string]any{"subjects": []string{f.deploy}})
	itemWide := concern(t, s, f, "concern on the item", nil)
	expl := ok(t, s, investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "explains", "statement": "reasoning",
		"citations": []map[string]any{{"artifact": f.notes.ID, "quote": "Faster startup."}}})["id"].(string)
	other := ok(t, s, investigate.ToolRecordAssertion, map[string]any{"changeItem": f.values2, "stance": "raises-concern", "statement": "legacy",
		"citations": []map[string]any{{"artifact": f.notes.ID, "quote": "was removed"}}})["id"].(string)
	for _, c := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"answers": []string{"asr-nope"}}, "unknown assertion"},
		{map[string]any{"answers": []string{expl}}, "only raises-concern and claims-mitigation"},
		{map[string]any{"answers": []string{other}}, "is about change item"},
		{map[string]any{"subjects": []string{f.cfgMap}, "answers": []string{onDeploy}}, "subject " + f.deploy + " is not covered"},
		{map[string]any{"subjects": []string{f.deploy}, "answers": []string{itemWide}}, "only an item-wide validation"},
	} {
		args := map[string]any{"changeItem": f.behaviour, "kind": "override-inspection"}
		for k, v := range c.args {
			args[k] = v
		}
		if _, err := call(t, s, investigate.ToolRequestValidation, args); !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: got %v, want %q", c.args, err, c.want)
		}
	}
	// An item-wide request answers the item-wide concern (the validator covers every root and path).
	r := ok(t, s, investigate.ToolRequestValidation, map[string]any{"changeItem": f.behaviour, "kind": "override-inspection", "answers": []string{itemWide}})
	if r["answers"].([]any)[0].(map[string]any)["linked"] != true {
		t.Fatalf("item-wide: %v", r)
	}
	// Prompt v2 has no answer links.
	s2 := f.v2session(t, f.input(investigate.AgentConfig{}), &fakeOverride{f: f})
	if _, err := call(t, s2, investigate.ToolRequestValidation, map[string]any{"changeItem": f.behaviour, "kind": "override-inspection", "answers": []string{"asr-x"}}); !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), "does not link answers") {
		t.Fatalf("v2 answers: %v", err)
	}
}

func TestV3HypothesisLinksAndTheGuardRefusesNarrowRecords(t *testing.T) {
	f := newFixture(t)
	s := f.v3session(t, f.input(investigate.AgentConfig{}), &fakeOverride{f: f})
	hyp := ok(t, s, investigate.ToolRecordHypothesis, map[string]any{"changeItem": f.behaviour, "statement": "the values file pins the policy",
		"citations": []map[string]any{{"artifact": f.values.ID, "quote": "rotationPolicy: Never"}}})["id"].(string)
	wide := concern(t, s, f, "item-wide concern", map[string]any{"hypothesis": hyp})
	// A narrow request with the hypothesis: the item-wide concern is not linked (reason given).
	r := ok(t, s, investigate.ToolRequestValidation, map[string]any{"changeItem": f.behaviour, "kind": "override-inspection", "subjects": []string{f.deploy}, "hypothesis": hyp})
	l := r["answers"].([]any)[0].(map[string]any)
	if l["linked"] != false || !strings.Contains(l["reason"].(string), "of the item") {
		t.Fatalf("narrow record linked an item-wide concern: %v", r)
	}
	// An item-wide request with the hypothesis links it implicitly.
	s.SetCall(CallInfo{ID: "call-2", Model: "test-model", ModelVersion: "v"})
	r = ok(t, s, investigate.ToolRequestValidation, map[string]any{"changeItem": f.behaviour, "kind": "override-inspection", "hypothesis": hyp})
	if l := r["answers"].([]any)[0].(map[string]any); l["assertion"] != wide || l["linked"] != true {
		t.Fatalf("hypothesis link: %v", r)
	}
}

func TestV3AnswerLinkToADeterministicPassRecord(t *testing.T) {
	f := newFixture(t)
	fake := &fakeOverride{f: f}
	pass, err := DeterministicPass(context.Background(), []investigate.Validator{fake}, investigate.ValidationInput{Bundle: f.b, Graph: f.g, Traversal: f.trav})
	must(t, err)
	in := f.input(investigate.AgentConfig{})
	in.Validations, in.ValidationEvidence = pass.Records, pass.Evidence
	s := f.v3session(t, in, fake)
	asr := concern(t, s, f, "item-wide concern", nil)
	r := ok(t, s, investigate.ToolRequestValidation, map[string]any{"changeItem": f.behaviour, "kind": "override-inspection", "answers": []string{asr}})
	if r["request"].(map[string]any)["status"] != "rejected" || r["answers"].([]any)[0].(map[string]any)["linked"] != true {
		t.Fatalf("duplicate of the pass record: %v", r)
	}
	out, err := s.Outcome(context.Background(), f.st, DriverScripted, investigate.DriverResult{Model: "test-model", ModelVersion: "v", Stop: "error:x"})
	must(t, err)
	if len(out.Validations) != 0 || len(out.AnswerLinks) != 1 || out.AnswerLinks[0].Validation != pass.Records[0].ID {
		t.Fatalf("links: %+v", out.AnswerLinks)
	}
	if !unanswered(t, f, out, pass.Records, asr) {
		t.Fatal("the link lifted the concern before the runner applied it")
	}
	applied := ApplyAnswerLinks(pass.Records, out.AnswerLinks)
	if unanswered(t, f, out, applied, asr) {
		t.Fatal("the applied link does not lift the concern")
	}
	if len(pass.Records[0].Answers) != 0 {
		t.Fatal("ApplyAnswerLinks mutated its input")
	}
	if err := runRecord(t, f, out, applied...).ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
}
