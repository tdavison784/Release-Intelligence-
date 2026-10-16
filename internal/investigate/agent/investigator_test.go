package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// fullScript exercises every record kind.
func fullScript(f *fixture) []Step {
	cite := func(a domain.ArtifactRef, q string) []map[string]any { return []map[string]any{{"artifact": a.ID, "locator": "L1", "quote": q}} }
	hypArgs := map[string]any{"changeItem": f.behaviour, "statement": "The environment pins the old rotation policy with controller.rotationPolicy: Never.",
		"subjects": []string{deployID.String()}, "citations": append(cite(f.values, "rotationPolicy: Never"), cite(f.notes, "Set `controller.rotationPolicy: Never` in your values")...)}
	return []Step{
		{Tool: investigate.ToolGetChangeItem, Args: map[string]any{"id": f.behaviour}},
		{Tool: investigate.ToolQueryDiff, Args: map[string]any{"changeItem": f.behaviour}},
		{Tool: investigate.ToolGetRenderedObject, Args: map[string]any{"target": "argocd:apps/demo.yaml", "side": "to", "object": deployID.String(), "path": "spec.template.spec.containers[name=controller].args"}},
		{Tool: investigate.ToolRecordHypothesis, Args: hypArgs},
		{Tool: investigate.ToolRecordAssertion, Args: map[string]any{"changeItem": f.behaviour, "stance": "claims-mitigation",
			"statement": "controller.rotationPolicy: Never keeps the previous behaviour.", "citations": cite(f.values, "rotationPolicy: Never"),
			"hypothesis": hypID(f, hypArgs["statement"].(string), "scripted-call-1")}},
		{Tool: investigate.ToolRecordAssertion, Args: map[string]any{"changeItem": f.behaviour, "stance": "raises-concern",
			"statement": "demo-config may be read by another consumer.", "subjects": []string{f.cfgMap}, "paths": []string{f.path}, "renderedChanges": []string{f.rcLoose},
			"citations": []map[string]any{{"artifact": f.toRender.ID, "quote": "mode: fast"}},
			"proposedEdge": map[string]any{"kind": "references", "from": f.cfgMap, "to": "workload:apps/v1/Deployment/demo/demo-reader", "reason": "the reader mounts demo-config (by convention)"}}},
		{Tool: investigate.ToolRequestValidation, Args: map[string]any{"changeItem": f.behaviour, "kind": "override-inspection", "subjects": []string{f.deploy},
			"inputs": []string{f.values.ID}, "hypothesis": hypID(f, hypArgs["statement"].(string), "scripted-call-1")}},
		{Tool: investigate.ToolRequestEvidence, Args: map[string]any{"changeItem": f.values2, "kind": "artifact", "target": "https://github.com/demo/demo/pull/7", "reason": "the removal PR"}},
		{Call: "call-2", Tool: investigate.ToolSubmitFindings, Args: map[string]any{"items": []any{
			map[string]any{"changeItem": f.behaviour, "summary": "Pinned by the values file; an override inspection was requested.", "citations": cite(f.values, "rotationPolicy: Never")},
			map[string]any{"changeItem": f.values2, "summary": "legacy.enabled is not set by the environment.", "citations": cite(f.notes, "was removed")},
		}}},
	}
}

func hypID(f *fixture, statement, call string) string {
	return domain.AgentRecordID(domain.HypothesisIDPrefix, f.behaviour, statement, call)
}

func runScripted(t *testing.T, f *fixture, steps []Step) (*investigate.AgentOutcome, *Scripted) {
	t.Helper()
	d := &Scripted{Model: "test-model", ModelVersion: "test-model-2026", Provider: "test", Turns: 5, Tokens: 1000, Steps: steps}
	inv := New(f.st, WithDriver(DriverScripted, d), WithValidators(&fakeOverride{f: f}), WithClock(fixedNow))
	out, err := inv.Investigate(context.Background(), f.input(investigate.AgentConfig{Enabled: true, Driver: DriverScripted, Model: "test-model",
		PromptVersion: DefaultPromptVersion, Budget: investigate.DefaultAgentBudget}))
	must(t, err)
	for i, r := range d.Results {
		if r.Err != nil {
			t.Fatalf("step %d %s: %v", i, r.Tool, r.Err)
		}
	}
	return out, d
}

// runRecord assembles the investigation run record the orchestrator would
// write from the agent outcome, so the contract's own checks judge it.
func runRecord(t *testing.T, f *fixture, out *investigate.AgentOutcome) *domain.Investigation {
	t.Helper()
	cfg := investigate.Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound, Agent: investigate.AgentConfig{Enabled: true}}
	g := MergeGraph(f.g, out)
	if err := g.Validate(); err != nil {
		t.Fatalf("merged graph: %v", err)
	}
	inv := &domain.Investigation{SchemaVersion: domain.InvestigationSchemaVersion, Bundle: f.b.ID, Snapshot: f.b.Snapshot.ID, ChangeRecord: f.b.Change.ID,
		Environment: f.b.Snapshot.Environment, Config: cfg.Digest(), Status: domain.InvestigationRunning, Traversal: f.trav, Agent: &out.Run,
		Hypotheses: out.Hypotheses, Assertions: out.Assertions, EvidenceRequests: out.EvidenceRequests, ValidationRequests: out.ValidationRequests,
		Validations: out.Validations, Artifacts: out.Artifacts, Evidence: out.Evidence, Review: domain.HumanReview{State: domain.ReviewAwaiting}, CreatedAt: fixedNow()}
	inv.ID = domain.InvestigationID(inv.Bundle, inv.Config)
	inv.AppendAudit(domain.AuditEvent{At: fixedNow(), Stage: domain.StageInvestigate, Event: "completed", Actor: "agent:" + out.Run.Model, Refs: []string{out.Run.Transcript}})
	return inv
}

func TestInvestigatorRecordsEverythingWithProvenance(t *testing.T) {
	f := newFixture(t)
	out, _ := runScripted(t, f, fullScript(f))
	r := out.Run
	if r.Status != domain.AgentSubmitted || r.Reason != "" || r.Model != "test-model" || r.PromptVersion != DefaultPromptVersion || r.Transcript == "" || r.ToolCalls != 9 {
		t.Fatalf("run: %+v", r)
	}
	if len(out.Hypotheses) != 1 || len(out.Assertions) != 4 || len(out.ValidationRequests) != 1 || len(out.EvidenceRequests) != 1 || len(out.Validations) != 1 || len(out.ProposedNodes) != 1 {
		t.Fatalf("records: %d hyp %d asr %d vrq %d erq %d val %d nodes", len(out.Hypotheses), len(out.Assertions), len(out.ValidationRequests),
			len(out.EvidenceRequests), len(out.Validations), len(out.ProposedNodes))
	}
	for _, a := range out.Assertions {
		p := a.Provenance
		if p.Method != domain.MethodAI || p.Model != "test-model" || p.ModelVersion != "test-model-2026" || p.CallID == "" || p.PromptDigest != r.PromptDigest || len(p.InputEvidence) == 0 {
			t.Fatalf("assertion provenance: %+v", p)
		}
	}
	if out.ValidationRequests[0].Result != out.Validations[0].ID || out.Validations[0].Provenance.Method == domain.MethodAI {
		t.Fatalf("validation request/result: %+v / %+v", out.ValidationRequests[0], out.Validations[0])
	}
	inv := runRecord(t, f, out)
	if err := inv.ValidateAgainst(f.b); err != nil {
		t.Fatalf("the run record built from the outcome does not validate against its bundle:\n%v", err)
	}
	// The transcript is stored, parses, and names every call.
	ref := out.Artifacts[0]
	rc, err := f.st.Open(context.Background(), ref.Digest)
	must(t, err)
	b, _ := io.ReadAll(rc)
	rc.Close()
	tr, err := ParseTranscript(b)
	must(t, err)
	if ref.Kind != domain.ArtifactAgentTranscript || len(tr.Calls) != 9 || tr.Envelope == nil || tr.Envelope.Result.Stop != "submitted" || tr.PromptDigest() != r.PromptDigest {
		t.Fatalf("transcript: %s %d calls, envelope %+v", ref.Kind, len(tr.Calls), tr.Envelope)
	}
	if bytes.Contains(b, []byte("2026-10-16T12:00:00Z")) {
		t.Errorf("the transcript carries wall-clock times; replay could not reproduce it")
	}
}

func TestNotEnabledSendsNothing(t *testing.T) {
	f := newFixture(t)
	d := &Scripted{}
	out, err := New(f.st, WithDriver(DriverScripted, d)).Investigate(context.Background(), f.input(investigate.AgentConfig{Driver: DriverScripted}))
	must(t, err)
	if out.Run.Status != domain.AgentNotRun || !strings.Contains(out.Run.Reason, "opt-in") || d.Results != nil {
		t.Fatalf("disabled agent: %+v", out.Run)
	}
	if _, err := New(f.st).Investigate(context.Background(), f.input(investigate.AgentConfig{Enabled: true, Driver: "telepathy"})); investigate.ClassOf(err) != investigate.Permanent {
		t.Fatalf("unknown driver: %v", err)
	}
}

func TestReplayReproducesResultDigests(t *testing.T) {
	f := newFixture(t)
	out, _ := runScripted(t, f, fullScript(f))
	rc, err := f.st.Open(context.Background(), out.Artifacts[0].Digest)
	must(t, err)
	original, _ := io.ReadAll(rc)
	rc.Close()

	rep := &Replay{Transcript: original}
	inv := New(f.st, WithDriver(DriverReplay, rep), WithValidators(&fakeOverride{f: f}), WithClock(func() time.Time { return fixedNow().Add(time.Hour) }))
	again, err := inv.Investigate(context.Background(), f.input(investigate.AgentConfig{Enabled: true, Driver: DriverReplay, Model: "test-model",
		PromptVersion: DefaultPromptVersion, Budget: investigate.DefaultAgentBudget}))
	must(t, err)
	if len(rep.Mismatches) != 0 {
		t.Fatalf("replay mismatches: %+v", rep.Mismatches)
	}
	if again.Run.Status != domain.AgentSubmitted || again.Run.Transcript != out.Run.Transcript {
		t.Fatalf("replay run %+v; transcript %s vs %s", again.Run, again.Run.Transcript, out.Run.Transcript)
	}
	ids := func(o *investigate.AgentOutcome) string {
		var s []string
		for _, h := range o.Hypotheses {
			s = append(s, h.ID)
		}
		for _, a := range o.Assertions {
			s = append(s, a.ID)
		}
		for _, r := range o.ValidationRequests {
			s = append(s, r.ID, r.Result)
		}
		for _, r := range o.EvidenceRequests {
			s = append(s, r.ID)
		}
		for _, e := range o.Evidence {
			s = append(s, string(e.ID))
		}
		return strings.Join(s, ",")
	}
	if ids(out) != ids(again) {
		t.Fatalf("replayed records differ:\n%s\n%s", ids(out), ids(again))
	}

	// A tampered recording: the mismatch is reported, never hidden.
	var lines [][]byte
	for i, l := range bytes.Split(bytes.TrimSpace(original), []byte("\n")) {
		if i == 2 {
			l = bytes.Replace(l, []byte(`"resultDigest":"sha256:`), []byte(`"resultDigest":"sha256:00`), 1)
		}
		lines = append(lines, l)
	}
	rep2 := &Replay{Transcript: append(bytes.Join(lines, []byte("\n")), '\n')}
	bad, err := New(f.st, WithDriver(DriverReplay, rep2), WithValidators(&fakeOverride{f: f})).Investigate(context.Background(),
		f.input(investigate.AgentConfig{Enabled: true, Driver: DriverReplay, Model: "test-model", Budget: investigate.DefaultAgentBudget}))
	must(t, err)
	if len(rep2.Mismatches) != 1 || bad.Run.Status != domain.AgentFailed || !strings.Contains(bad.Run.Reason, "1 of 9 results differ") {
		t.Fatalf("tampered replay: %+v %+v", rep2.Mismatches, bad.Run)
	}

	// A different prompt is not the same run.
	rep3 := &Replay{Transcript: original}
	other, err := New(f.st, WithDriver(DriverReplay, rep3)).Investigate(context.Background(),
		f.input(investigate.AgentConfig{Enabled: true, Driver: DriverReplay, Budget: domain.AgentBudget{MaxToolCalls: 7}}))
	must(t, err)
	if other.Run.Status != domain.AgentFailed || !strings.Contains(other.Run.Reason, "recorded under prompt") {
		t.Fatalf("replay under another prompt: %+v", other.Run)
	}
}

// --- MCP --------------------------------------------------------------------------------

func mcpExchange(t *testing.T, h ToolHandler, reqs ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	must(t, ServeMCP(context.Background(), strings.NewReader(strings.Join(reqs, "\n")+"\n"), &out, h))
	var resps []map[string]any
	for _, l := range bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n")) {
		var m map[string]any
		must(t, json.Unmarshal(l, &m))
		resps = append(resps, m)
	}
	return resps
}

func TestMCPServer(t *testing.T) {
	f := newFixture(t)
	s, _ := f.session(t)
	resps := mcpExchange(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_change_item","arguments":{"id":"`+f.behaviour+`"}}}`,
		`{"jsonrpc":"2.0","id":"four","method":"tools/call","params":{"name":"get_change_item","arguments":{"id":"ci-nope"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":6,"method":"ping"}`,
		`not json`,
	)
	if len(resps) != 7 {
		t.Fatalf("%d responses: %v", len(resps), resps)
	}
	if v := resps[0]["result"].(map[string]any)["protocolVersion"]; v != "2025-03-26" {
		t.Errorf("negotiated %v", v)
	}
	if tools := resps[1]["result"].(map[string]any)["tools"].([]any); len(tools) != len(investigate.ToolNames) {
		t.Errorf("tools/list: %d tools", len(tools))
	}
	ok3 := resps[2]["result"].(map[string]any)
	if ok3["isError"] != false || !strings.Contains(string(mustJSON(ok3["content"])), "rotationPolicy") {
		t.Errorf("tools/call: %v", ok3)
	}
	bad := resps[3]["result"].(map[string]any)
	if resps[3]["id"] != "four" || bad["isError"] != true || !strings.Contains(string(mustJSON(bad["content"])), "unknown change item") {
		t.Errorf("rejected tools/call: %v", resps[3])
	}
	if resps[4]["error"].(map[string]any)["code"].(float64) != -32601 || resps[6]["error"].(map[string]any)["code"].(float64) != -32700 {
		t.Errorf("errors: %v %v", resps[4], resps[6])
	}
}

func TestSocketBridge(t *testing.T) {
	f := newFixture(t)
	s, _ := f.session(t)
	dir, err := os.MkdirTemp("", "ri-t-")
	must(t, err)
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "s")
	srv, err := serveSocket(context.Background(), sock, s)
	must(t, err)
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_renders","arguments":{}}}` + "\n")
	var out bytes.Buffer
	must(t, Attach(context.Background(), sock, srv.token, in, &out))
	if !strings.Contains(out.String(), "argocd:apps/demo.yaml") {
		t.Fatalf("bridged call: %s", out.String())
	}
	var out2 bytes.Buffer
	_ = Attach(context.Background(), sock, "wrong-token", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"), &out2)
	if out2.Len() != 0 {
		t.Fatalf("a bridge with the wrong token was served: %s", out2.String())
	}
	errs := srv.close()
	if len(errs) != 1 || !strings.Contains(errs[0], "wrong token") {
		t.Fatalf("bridge errors: %v", errs)
	}
	if n, _ := s.Counters(); n != 1 {
		t.Fatalf("calls through the bridge: %d", n)
	}
}

// --- exchange ---------------------------------------------------------------------------

func TestExchangeDriver(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	cfg := investigate.AgentConfig{Enabled: true, Driver: DriverExchange, Model: "any-model", Budget: investigate.DefaultAgentBudget}
	inv := New(f.st, WithDriver(DriverExchange, &Exchange{Dir: dir}), WithClock(fixedNow))
	answer := func(output string) {
		reqs, _ := filepath.Glob(filepath.Join(dir, "*.request.json"))
		for _, rp := range reqs {
			resp := strings.TrimSuffix(rp, ".request.json") + ".response.json"
			if _, err := os.Stat(resp); err == nil {
				continue
			}
			var req llm.ExchangeRequest
			b, _ := os.ReadFile(rp)
			must(t, json.Unmarshal(b, &req))
			r, _ := json.Marshal(llm.ExchangeResponse{Format: llm.ExchangeResponseFormat, PromptDigest: req.PromptDigest, Model: "human", ModelVersion: "human-1",
				CallID: "", GeneratedAt: fixedNow(), Output: json.RawMessage(output)})
			must(t, os.WriteFile(resp, r, 0o644))
		}
	}
	out, err := inv.Investigate(context.Background(), f.input(cfg))
	must(t, err)
	if out.Run.Status != domain.AgentFailed || !strings.HasPrefix(out.Run.Reason, "exchange-pending") {
		t.Fatalf("round 1: %+v", out.Run)
	}
	answer(`{"calls":[{"tool":"get_change_item","arguments":{"id":"` + f.behaviour + `"}},{"tool":"get_change_item","arguments":{"id":"ci-nope"}}]}`)
	out, err = inv.Investigate(context.Background(), f.input(cfg))
	must(t, err)
	if !strings.HasPrefix(out.Run.Reason, "exchange-pending") || out.Run.Turns != 1 {
		t.Fatalf("round 2: %+v", out.Run)
	}
	answer(`{"calls":[{"tool":"record_assertion","arguments":{"changeItem":"` + f.behaviour + `","stance":"raises-concern","statement":"check the policy","citations":[{"artifact":"` + f.notes.ID + `","quote":"default rotation policy is Always"}]}},{"tool":"submit_findings","arguments":{"items":[]}}]}`)
	out, err = inv.Investigate(context.Background(), f.input(cfg))
	must(t, err)
	if out.Run.Status != domain.AgentSubmitted || out.Run.Turns != 2 || out.Run.ToolCalls != 4 || len(out.Assertions) != 1 {
		t.Fatalf("round 3: %+v (%d assertions)", out.Run, len(out.Assertions))
	}
	a := out.Assertions[0].Provenance
	if a.Model != "human" || a.ModelVersion != "human-1" || !strings.HasPrefix(a.CallID, "exchange:") {
		t.Fatalf("exchange provenance: %+v", a)
	}
	if err := runRecord(t, f, out).ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
}
