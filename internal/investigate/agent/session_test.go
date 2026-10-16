package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

func TestPromptLoadsAndDigests(t *testing.T) {
	p, err := LoadPrompt("")
	must(t, err)
	if p.Version != DefaultPromptVersion || len(p.Tools) != len(investigate.ToolNames) {
		t.Fatalf("prompt %s with %d tools", p.Version, len(p.Tools))
	}
	if _, err := LoadPrompt("investigate-agent/v0"); err == nil {
		t.Fatal("an unknown prompt version loads")
	}
	if _, err := LoadPrompt("../x"); err == nil {
		t.Fatal("a path-escaping prompt version loads")
	}
	f := newFixture(t)
	task, err := p.RenderTask(f.input(investigate.AgentConfig{PromptVersion: promptV1, }), investigate.DefaultAgentBudget)
	must(t, err)
	for _, want := range []string{f.behaviour, f.values2, f.rcItem, f.rcLoose, "controller-behaviour", "manifest:declared-behaviour",
		"https://example.com/demo/releases/v2.0.0", "argocd:apps/demo.yaml", "1 further change items are out of scope", "80 tool calls"} {
		if !strings.Contains(task, want) {
			t.Errorf("task lacks %q:\n%s", want, task)
		}
	}
	if strings.Contains(task, f.outScope) {
		t.Errorf("task lists the out-of-scope item")
	}
	again, _ := p.RenderTask(f.input(investigate.AgentConfig{PromptVersion: promptV1, }), investigate.DefaultAgentBudget)
	if again != task || p.Digest(task) != p.Digest(again) {
		t.Fatal("task rendering is not deterministic")
	}
	if p.Digest(task) == p.Digest(task+" ") {
		t.Fatal("the digest ignores the task")
	}
}

func call(t *testing.T, s *Session, tool string, args any) (json.RawMessage, error) {
	t.Helper()
	b, err := json.Marshal(args)
	must(t, err)
	return s.Dispatch(context.Background(), tool, b)
}

func ok(t *testing.T, s *Session, tool string, args any) map[string]any {
	t.Helper()
	res, err := call(t, s, tool, args)
	if err != nil {
		t.Fatalf("%s %v: %v", tool, args, err)
	}
	var v any
	must(t, json.Unmarshal(res, &v))
	if m, isMap := v.(map[string]any); isMap {
		return m
	}
	return map[string]any{"list": v}
}

func TestQueryToolsServeTheBundle(t *testing.T) {
	f := newFixture(t)
	s, _ := f.session(t)
	d := ok(t, s, investigate.ToolGetChangeItem, map[string]any{"id": f.behaviour})
	js, _ := json.Marshal(d)
	for _, want := range []string{"Set controller.rotationPolicy: Never", f.notes.ID, f.rcItem, "manifest:declared-behaviour"} {
		if !strings.Contains(string(js), want) {
			t.Errorf("get_change_item lacks %q: %s", want, js)
		}
	}
	if r := ok(t, s, investigate.ToolListRenders, map[string]any{}); len(r["list"].([]any)) != 1 {
		t.Errorf("list_renders: %v", r)
	}
	page := ok(t, s, investigate.ToolQueryDiff, map[string]any{"uncorrelated": true})
	if cs := page["changes"].([]any); len(cs) != 1 || cs[0].(map[string]any)["id"] != f.rcLoose {
		t.Errorf("query_diff uncorrelated: %v", page)
	}
	obj := ok(t, s, investigate.ToolGetRenderedObject, map[string]any{"target": "argocd:apps/demo.yaml", "side": "to",
		"object": deployID.String(), "path": "spec.template.spec.containers[name=controller].args"})
	if obj["found"] != true || !strings.Contains(obj["yaml"].(string), "--rotation-policy=Never") {
		t.Errorf("get_rendered_object: %v", obj)
	}
	hits := ok(t, s, investigate.ToolSearchManifest, map[string]any{"terms": []string{"values"}, "grep": "rotationPolicy"})
	hl := hits["list"].([]any)
	if len(hl) != 1 || !strings.Contains(string(mustJSON(hl[0])), `"line":2`) {
		t.Errorf("search_manifest: %v", hits)
	}
	rd := ok(t, s, investigate.ToolReadArtifact, map[string]any{"artifact": f.values.ID, "fromLine": 2, "toLine": 2})
	if rd["text"] != "  rotationPolicy: Never" || rd["totalLines"].(float64) != 5 {
		t.Errorf("read_artifact window: %v", rd)
	}
	rd = ok(t, s, investigate.ToolReadArtifact, map[string]any{"artifact": f.values.ID, "yamlPath": "controller.rotationPolicy"})
	if rd["text"] != "  rotationPolicy: Never" {
		t.Errorf("read_artifact yamlPath: %v", rd)
	}
	rd = ok(t, s, investigate.ToolReadArtifact, map[string]any{"artifact": f.toRender.ID, "yamlPath": "[1].spec.template"})
	if !strings.Contains(rd["text"].(string), "rotation-policy") {
		t.Errorf("read_artifact yamlPath into document 1: %v", rd)
	}
	sec := ok(t, s, investigate.ToolReadArtifact, map[string]any{"artifact": f.secret.ID})
	if !strings.Contains(sec["text"].(string), "<redacted sha256:") {
		t.Errorf("secret not served redacted: %v", sec)
	}
	sub := ok(t, s, investigate.ToolExpandGraph, map[string]any{"node": deployID.String()})
	if len(sub["nodes"].([]any)) != 2 || len(sub["edges"].([]any)) != 1 {
		t.Errorf("expand_graph: %v", sub)
	}
	paths := ok(t, s, investigate.ToolListPaths, map[string]any{"changeItem": f.behaviour, "unresolved": true})
	if len(paths["list"].([]any)) != 1 {
		t.Errorf("list_paths unresolved: %v", paths)
	}
	n, _ := s.Counters()
	if n != 11 {
		t.Errorf("tool calls counted: %d", n)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// TestRejectionPaths: every rejection returns ErrRejected to the model,
// records nothing, and is still counted and written to the transcript.
func TestRejectionPaths(t *testing.T) {
	f := newFixture(t)
	quote := func(a domain.ArtifactRef, q string) []map[string]any {
		return []map[string]any{{"artifact": a.ID, "quote": q}}
	}
	good := quote(f.notes, "the default rotation policy is Always")
	cases := []struct {
		name, tool string
		args       any
		want       string
	}{
		{"unknown change item", investigate.ToolGetChangeItem, map[string]any{"id": "ci-nope"}, "unknown change item"},
		{"unknown artifact", investigate.ToolReadArtifact, map[string]any{"artifact": "art-nope"}, "unknown artifact"},
		{"line past the end", investigate.ToolReadArtifact, map[string]any{"artifact": f.values.ID, "fromLine": 99}, "has 5 lines"},
		{"unknown yaml key", investigate.ToolReadArtifact, map[string]any{"artifact": f.values.ID, "yamlPath": "controller.nope"}, "not found"},
		{"unknown node", investigate.ToolExpandGraph, map[string]any{"node": "gn-nope"}, "unknown node"},
		{"depth too deep", investigate.ToolExpandGraph, map[string]any{"node": f.deploy, "depth": 4}, "depth 4"},
		{"unknown render target", investigate.ToolQueryDiff, map[string]any{"target": "argocd:nope"}, "unknown"},
		{"bad grep", investigate.ToolSearchManifest, map[string]any{"grep": "("}, "grep"},
		{"unknown path", investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "raises-concern", "statement": "x",
			"paths": []string{"ip-nope"}, "citations": good}, "unknown impact path"},
		{"unknown rendered change", investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "raises-concern", "statement": "x",
			"renderedChanges": []string{"rc-nope"}, "citations": good}, "unknown rendered change"},
		{"unknown hypothesis", investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "raises-concern", "statement": "x",
			"hypothesis": "hyp-nope", "citations": good}, "unknown hypothesis"},
		{"unknown subject", investigate.ToolRecordHypothesis, map[string]any{"changeItem": f.behaviour, "statement": "x", "subjects": []string{"core/v1/Pod/x/y"},
			"citations": good}, "unknown subject"},
		{"uncited assertion", investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "raises-concern", "statement": "x",
			"citations": []any{}}, "at least one citation"},
		{"uncited hypothesis", investigate.ToolRecordHypothesis, map[string]any{"changeItem": f.behaviour, "statement": "x", "citations": []any{}}, "at least one citation"},
		{"quote not in artifact", investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "explains", "statement": "x",
			"citations": quote(f.values, "rotationPolicy: Always")}, "does not occur"},
		{"citation of unknown artifact", investigate.ToolRecordHypothesis, map[string]any{"changeItem": f.behaviour, "statement": "x",
			"citations": quote(domain.ArtifactRef{ID: "art-nope"}, "x")}, "unknown artifact"},
		{"uncited summary", investigate.ToolSubmitFindings, map[string]any{"items": []any{map[string]any{"changeItem": f.behaviour, "summary": "s", "citations": []any{}}}}, "at least one citation"},
		{"invalid stance", investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "clears", "statement": "x", "citations": good}, "stance"},
		{"empty statement", investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "explains", "statement": "  ", "citations": good}, "statement is required"},
		{"unknown argument field", investigate.ToolGetChangeItem, map[string]any{"id": f.behaviour, "class": "not-affected"}, "unknown field"},
		{"malformed arguments", investigate.ToolQueryDiff, json.RawMessage(`{"limit": "many"}`), "invalid arguments"},
		{"unknown tool", "set_class", map[string]any{}, "unknown tool"},
		{"proposed edge kind", investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "raises-concern", "statement": "x", "citations": good,
			"proposedEdge": map[string]any{"kind": "clears", "from": f.deploy, "to": f.cfgMap, "reason": "r"}}, "unknown edge kind"},
		{"proposed edge without reason", investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "raises-concern", "statement": "x", "citations": good,
			"proposedEdge": map[string]any{"kind": "references", "from": f.cfgMap, "to": f.deploy, "reason": " "}}, "reason is required"},
		{"proposed edge already established", investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "raises-concern", "statement": "x", "citations": good,
			"proposedEdge": map[string]any{"kind": "references", "from": f.deploy, "to": f.cfgMap, "detail": "env[0].valueFrom", "reason": "r"}}, "already has this edge"},
		{"out-of-scope validation", investigate.ToolRequestValidation, map[string]any{"changeItem": f.outScope, "kind": "override-inspection"}, "out of scope"},
		{"validation path of another item", investigate.ToolRequestValidation, map[string]any{"changeItem": f.values2, "kind": "render-diff", "paths": []string{f.path}}, "belongs to change item"},
		{"submit out-of-scope item", investigate.ToolSubmitFindings, map[string]any{"items": []any{map[string]any{"changeItem": f.outScope, "summary": "s", "citations": good}}}, "out of scope"},
		{"submit an item twice", investigate.ToolSubmitFindings, map[string]any{"items": []any{
			map[string]any{"changeItem": f.behaviour, "summary": "a", "citations": good}, map[string]any{"changeItem": f.behaviour, "summary": "b", "citations": good}}}, "twice"},
		{"evidence already present", investigate.ToolRequestEvidence, map[string]any{"kind": "environment-input", "target": "env/apps/demo/values.yaml", "reason": "r"}, "already in the investigation"},
		{"evidence source unknown", investigate.ToolRequestEvidence, map[string]any{"kind": "source", "target": "blog-posts", "reason": "r"}, "not a change source kind"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := f.session(t)
			var raw []byte
			switch a := c.args.(type) {
			case json.RawMessage:
				raw = a
			default:
				raw, _ = json.Marshal(a)
			}
			_, err := s.Dispatch(context.Background(), c.tool, raw)
			if !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want a rejection mentioning %q", err, c.want)
			}
			out, err := s.Outcome(context.Background(), f.st, DriverScripted, investigate.DriverResult{Model: "m", ModelVersion: "v", Stop: "error:test"})
			must(t, err)
			if n := len(out.Hypotheses) + len(out.Assertions) + len(out.EvidenceRequests) + len(out.ValidationRequests) + len(out.Validations) + len(out.Evidence); n != 0 {
				t.Fatalf("a rejected call recorded %d records: %+v", n, out)
			}
			if s.Submitted() || out.Run.ToolCalls != 1 {
				t.Fatalf("submitted=%v toolCalls=%d", s.Submitted(), out.Run.ToolCalls)
			}
			tr, err := ParseTranscript(s.Transcript())
			must(t, err)
			if len(tr.Calls) != 1 || tr.Calls[0].OK || tr.Calls[0].ErrorClass != "rejected" {
				t.Fatalf("transcript: %+v", tr.Calls)
			}
		})
	}
}

func TestLiveObservationIsRejectedNotAnError(t *testing.T) {
	f := newFixture(t)
	s, _ := f.session(t)
	r := ok(t, s, investigate.ToolRequestEvidence, map[string]any{"changeItem": f.behaviour, "kind": "live-observation", "target": "apps/v1/Deployment", "reason": "see the running args"})
	if r["status"] != "rejected" || !strings.Contains(r["resolution"].(string), "disabled") {
		t.Fatalf("live request: %v", r)
	}
	r = ok(t, s, investigate.ToolRequestEvidence, map[string]any{"changeItem": f.behaviour, "kind": "artifact", "target": "https://github.com/demo/demo/pull/1", "reason": "the PR"})
	if r["status"] != "open" {
		t.Fatalf("artifact request: %v", r)
	}
}

func TestReadOnlySessionRefusesRecords(t *testing.T) {
	f := newFixture(t)
	p, _ := LoadPrompt(promptV1)
	s, err := NewSession(SessionConfig{Bundle: f.b, Store: f.st, Graph: f.g, Traversal: f.trav, Budget: investigate.DefaultAgentBudget, Prompt: p, ReadOnly: true})
	must(t, err)
	s.SetCall(CallInfo{ID: "c"})
	if _, err := call(t, s, investigate.ToolRecordHypothesis, map[string]any{"changeItem": f.behaviour, "statement": "x",
		"citations": []map[string]any{{"artifact": f.notes.ID, "quote": "Faster startup."}}}); !errors.Is(err, investigate.ErrRejected) {
		t.Fatalf("read-only session recorded: %v", err)
	}
	ok(t, s, investigate.ToolGetChangeItem, map[string]any{"id": f.behaviour})
}

func TestToolCallBudget(t *testing.T) {
	f := newFixture(t)
	b := investigate.DefaultAgentBudget
	b.MaxToolCalls = 3
	inv := New(f.st, WithDriver(DriverScripted, &Scripted{Steps: []Step{
		{Tool: investigate.ToolRecordHypothesis, Args: map[string]any{"changeItem": f.behaviour, "statement": "the environment pins the old policy",
			"citations": []map[string]any{{"artifact": f.values.ID, "quote": "rotationPolicy: Never"}}}},
		{Tool: investigate.ToolListRenders},
		{Tool: investigate.ToolListRenders},
		{Tool: investigate.ToolListRenders}, // the 4th call exhausts the budget
		{Tool: investigate.ToolSubmitFindings, Args: map[string]any{"items": []any{}}},
	}}), WithClock(fixedNow))
	out, err := inv.Investigate(context.Background(), f.input(investigate.AgentConfig{PromptVersion: promptV1, Enabled: true, Driver: DriverScripted, Budget: b}))
	must(t, err)
	if out.Run.Status != domain.AgentBudgetExhausted || out.Run.Reason != "budget:tool-calls" {
		t.Fatalf("run: %+v", out.Run)
	}
	if len(out.Hypotheses) != 1 {
		t.Fatalf("records made before exhaustion are kept: %d hypotheses", len(out.Hypotheses))
	}
	if out.Run.ToolCalls != 4 {
		t.Fatalf("tool calls: %d", out.Run.ToolCalls)
	}
}

func TestReadByteBudget(t *testing.T) {
	f := newFixture(t)
	s, _, err := Prepare(f.st, f.input(investigate.AgentConfig{Enabled: true, PromptVersion: promptV1, Budget: domain.AgentBudget{MaxReadBytes: 300}}), nil, fixedNow)
	must(t, err)
	s.SetCall(CallInfo{ID: "c", Model: "m", ModelVersion: "v"})
	if _, err := call(t, s, investigate.ToolReadArtifact, map[string]any{"artifact": f.notes.ID}); !errors.Is(err, investigate.ErrBudgetExhausted) {
		t.Fatalf("an oversized read: %v", err)
	}
	if s.Exhausted() != "budget:read-bytes" {
		t.Fatalf("exhausted = %q", s.Exhausted())
	}
	if _, err := call(t, s, investigate.ToolListRenders, map[string]any{}); !errors.Is(err, investigate.ErrBudgetExhausted) {
		t.Fatalf("a call after exhaustion: %v", err)
	}
	out, err := s.Outcome(context.Background(), f.st, DriverScripted, investigate.DriverResult{Model: "m", ModelVersion: "v", Stop: "submitted"})
	must(t, err)
	if out.Run.Status != domain.AgentBudgetExhausted {
		t.Fatalf("run status %s", out.Run.Status)
	}
}

func TestDriverBudgets(t *testing.T) {
	f := newFixture(t)
	for _, c := range []struct {
		dr   investigate.DriverResult
		want string
	}{
		{investigate.DriverResult{Stop: "submitted", Tokens: 700_000}, "budget:tokens"},
		{investigate.DriverResult{Stop: "submitted", Turns: 42}, "budget:turns"},
		{investigate.DriverResult{Stop: "budget:seconds"}, "budget:seconds"},
		{investigate.DriverResult{Stop: "pending:/x/1.request.json"}, "exchange-pending"},
		{investigate.DriverResult{Stop: "error:boom"}, "boom"},
	} {
		s, _ := f.session(t)
		ok(t, s, investigate.ToolSubmitFindings, map[string]any{"items": []any{}})
		st, reason := runStatus(c.dr, s)
		if st == domain.AgentSubmitted || !strings.HasPrefix(reason, c.want) && !strings.Contains(reason, c.want) {
			t.Errorf("%+v: %s %q", c.dr, st, reason)
		}
	}
}

func TestValidationRequests(t *testing.T) {
	f := newFixture(t)
	fake := &fakeOverride{f: f}
	s, _ := f.session(t, fake, &brokenValidator{kind: domain.ValidationSchemaCompat}, &brokenValidator{kind: domain.ValidationStaticReference, other: "ci-other"})
	r := ok(t, s, investigate.ToolRequestValidation, map[string]any{"changeItem": f.behaviour, "kind": "override-inspection",
		"subjects": []string{deployID.String()}, "renderedChanges": []string{f.rcItem}, "inputs": []string{f.values.ID}})
	req := r["request"].(map[string]any)
	val := r["validation"].(map[string]any)
	if req["status"] != "fulfilled" || req["result"] != val["id"] || val["request"] != req["id"] || val["outcome"] != "pass" {
		t.Fatalf("fulfilled request: %v", r)
	}
	if _, has := val["checkedAt"]; has {
		t.Errorf("volatile checkedAt returned to the model")
	}
	// The same validation again (a new call): not recorded twice.
	s.SetCall(CallInfo{ID: "call-2", Model: "test-model", ModelVersion: "test-model-2026"})
	r = ok(t, s, investigate.ToolRequestValidation, map[string]any{"changeItem": f.behaviour, "kind": "override-inspection",
		"subjects": []string{f.deploy}, "renderedChanges": []string{f.rcItem}})
	if r["request"].(map[string]any)["status"] != "rejected" || !strings.Contains(r["request"].(map[string]any)["resolution"].(string), "already validated") {
		t.Fatalf("duplicate validation: %v", r)
	}
	for _, c := range []struct{ kind, item, want string }{
		{"render-diff", f.behaviour, "no render-diff validator applies"},
		{"override-inspection", f.values2, "no override-inspection validator applies to family values"},
		{"schema-compat", f.values2, "cannot read the CRDs"},
		{"static-reference", f.values2, "another item"},
	} {
		r := ok(t, s, investigate.ToolRequestValidation, map[string]any{"changeItem": c.item, "kind": c.kind})
		req := r["request"].(map[string]any)
		if req["status"] != "unavailable" || !strings.Contains(req["resolution"].(string), c.want) {
			t.Errorf("%s/%s: %v", c.kind, c.item, r)
		}
	}
	out, err := s.Outcome(context.Background(), f.st, DriverScripted, investigate.DriverResult{Model: "m", ModelVersion: "v", Stop: "error:x"})
	must(t, err)
	if len(out.Validations) != 1 || len(out.ValidationRequests) != 6 || fake.calls != 2 {
		t.Fatalf("validations %d requests %d validator calls %d", len(out.Validations), len(out.ValidationRequests), fake.calls)
	}
}
