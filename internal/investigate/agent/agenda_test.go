package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

const promptV2 = "investigate-agent/v2"

func v2Config(b domain.AgentBudget) investigate.AgentConfig {
	return investigate.AgentConfig{Enabled: true, Driver: DriverScripted, Model: "test-model", PromptVersion: promptV2, Budget: b}
}

func (f *fixture) v2session(t *testing.T, in investigate.InvestigateInput, validators ...investigate.Validator) *Session {
	t.Helper()
	in.Config = v2Config(investigate.DefaultAgentBudget)
	s, _, err := Prepare(f.st, in, validators, fixedNow)
	must(t, err)
	s.SetCall(CallInfo{ID: "call-1", Model: "test-model", ModelVersion: "test-model-2026", Provider: "test"})
	return s
}

func rec(item string, outcome domain.ValidationOutcomeKind) domain.ValidationRecord {
	return domain.ValidationRecord{ID: "vr-" + item + string(outcome), ChangeItem: item, Outcome: outcome}
}

func TestPromptV2(t *testing.T) {
	p1, err := LoadPrompt(promptV1)
	must(t, err)
	p2, err := LoadPrompt("")
	must(t, err)
	if p2.Version != promptV2 || !p2.Features.Agenda || !p2.Features.ResolveSubjects || p1.Features.Agenda || p1.MetaJSON != nil {
		t.Fatalf("features: v1 %+v v2 %+v", p1.Features, p2.Features)
	}
	f := newFixture(t)
	in := f.input(v2Config(domain.AgentBudget{}))
	agenda := Agenda(f.b, nil)
	task, err := p2.renderTask(in, investigate.DefaultAgentBudget, agenda)
	must(t, err)
	for _, want := range []string{"## Agenda (2 items", "1. " + f.behaviour + " [agenda:behaviour]", "2. " + f.values2 + " [agenda:undecided]", "examined-no-concern"} {
		if !strings.Contains(task+p2.System, want) {
			t.Errorf("v2 prompt lacks %q", want)
		}
	}
	if p1.Digest("x") == p2.Digest("x") {
		t.Fatal("v1 and v2 share a digest")
	}
}

func TestAgendaRules(t *testing.T) {
	item := func(id string, m domain.Manifestation, scope string, in bool) domain.ChangeItem {
		return domain.ChangeItem{ID: id, Manifestation: m, ScopeRule: scope, InScope: in}
	}
	b := &domain.EvidenceBundle{Change: domain.ChangeEvidenceRecord{Items: []domain.ChangeItem{
		item("ci-a", domain.ManifestRenderedConfig, "scope:breaking", true),
		item("ci-b", domain.ManifestControllerBehaviour, "scope:breaking", true),
		item("ci-c", domain.ManifestRenderedConfig, "scope:rendered-unexplained", true),
		item("ci-d", domain.ManifestRenderedConfig, "scope:computed-breaking", true),
		item("ci-e", domain.ManifestUnknown, "scope:breaking", true),
		item("ci-f", domain.ManifestControllerBehaviour, "scope:not-breaking", false),
		item("ci-g", domain.ManifestControllerBehaviour, "scope:breaking", true),
	}}}
	got := Agenda(b, []domain.ValidationRecord{rec("ci-d", domain.ValidationPass), rec("ci-g", domain.ValidationFail), rec("ci-a", domain.ValidationNotRun), rec("ci-e", domain.ValidationPass)})
	var lines []string
	for _, e := range got {
		lines = append(lines, e.ChangeItem+"="+e.PriorityRule)
	}
	want := "ci-b=agenda:behaviour,ci-c=agenda:rendered-unexplained,ci-e=agenda:behaviour,ci-d=agenda:pass-to-scrutinise,ci-a=agenda:undecided"
	if strings.Join(lines, ",") != want {
		t.Fatalf("agenda\n got %s\nwant %s", strings.Join(lines, ","), want)
	}
}

func TestAgendaBudget(t *testing.T) {
	b, rule := AgendaBudget(domain.AgentBudget{}, 0)
	if b != investigate.DefaultAgentBudget || !strings.HasPrefix(rule, "agenda@v1 n=0") {
		t.Fatalf("n=0: %+v %s", b, rule)
	}
	b, _ = AgendaBudget(domain.AgentBudget{}, 27)
	if b.MaxToolCalls != 174 || b.MaxTurns != 91 || b.MaxTokens != 960_000 || b.MaxSeconds != 1920 || b.MaxReadBytes != 1<<20+27*192<<10 {
		t.Fatalf("n=27: %+v", b)
	}
	b, _ = AgendaBudget(domain.AgentBudget{MaxToolCalls: 1000}, 500)
	if b.MaxToolCalls != 1000 || b.MaxTurns != 200 || b.MaxTokens != 3_000_000 || b.MaxSeconds != 3600 || b.MaxReadBytes != 16<<20 {
		t.Fatalf("caps / configured above the formula: %+v", b)
	}
}

func submitArgs(entries ...map[string]any) map[string]any {
	items := []any{}
	for _, e := range entries {
		items = append(items, e)
	}
	return map[string]any{"items": items}
}

func TestV2SubmitClosesEveryAgendaItem(t *testing.T) {
	f := newFixture(t)
	s := f.v2session(t, f.input(investigate.AgentConfig{}))
	cite := []map[string]any{{"artifact": f.values.ID, "quote": "rotationPolicy: Never"}}
	noConcern := map[string]any{"changeItem": f.behaviour, "disposition": "examined-no-concern", "summary": "inspected the values file", "citations": cite}
	notExamined := map[string]any{"changeItem": f.values2, "disposition": "not-examined", "summary": "ran out of budget"}
	for _, c := range []struct {
		args any
		want string
	}{
		{submitArgs(noConcern), "1 of 2 have no entry: " + f.values2},
		{submitArgs(map[string]any{"changeItem": f.behaviour, "summary": "x", "citations": cite}, notExamined), "disposition is required"},
		{submitArgs(map[string]any{"changeItem": f.behaviour, "disposition": "cleared", "summary": "x", "citations": cite}, notExamined), "unknown disposition"},
		{submitArgs(map[string]any{"changeItem": f.behaviour, "disposition": "examined-no-concern", "summary": "x"}, notExamined), "at least one citation"},
		{submitArgs(noConcern, map[string]any{"changeItem": f.values2, "disposition": "not-examined", "summary": " "}), "the reason it was not examined"},
	} {
		if _, err := call(t, s, investigate.ToolSubmitFindings, c.args); !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: got %v, want %q", c.args, err, c.want)
		}
	}
	if s.Submitted() {
		t.Fatal("a rejected submission ended the run")
	}
	r := ok(t, s, investigate.ToolSubmitFindings, submitArgs(noConcern, notExamined))
	if r["agenda"].(map[string]any)["items"].(float64) != 2 {
		t.Fatalf("submit view: %v", r)
	}
	out, err := s.Outcome(context.Background(), f.st, DriverScripted, investigate.DriverResult{Model: "test-model", ModelVersion: "v", Stop: "submitted"})
	must(t, err)
	cov := out.Run.Coverage
	if len(cov) != 2 || cov[0].ChangeItem != f.behaviour || cov[0].Disposition != domain.CoverExaminedNoConcern || cov[0].Assertion == "" ||
		cov[1].Disposition != domain.CoverNotExamined || cov[1].Assertion != "" || cov[1].Summary != "ran out of budget" {
		t.Fatalf("coverage: %+v", cov)
	}
	if len(out.Assertions) != 1 || out.Assertions[0].Stance != domain.StanceExplains {
		t.Fatalf("examined-no-concern is recorded as an explains assertion, never a mitigation: %+v", out.Assertions)
	}
	if !strings.HasPrefix(out.Run.BudgetRule, "agenda@v1 n=2") {
		t.Fatalf("budget rule %q", out.Run.BudgetRule)
	}
	inv := runRecord(t, f, out)
	if err := inv.ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
	// The contract checks coverage: a closing assertion of the right stance,
	// none for not-examined, a known disposition, in-scope items.
	for name, mutate := range map[string]func(c []domain.AgendaCoverage){
		"wrong stance":            func(c []domain.AgendaCoverage) { c[0].Disposition = domain.CoverRaisesConcern },
		"not-examined with claim": func(c []domain.AgendaCoverage) { c[1].Assertion = c[0].Assertion },
		"unknown disposition":     func(c []domain.AgendaCoverage) { c[1].Disposition = "cleared" },
		"out-of-scope item":       func(c []domain.AgendaCoverage) { c[1].ChangeItem = f.outScope },
	} {
		bad := runRecord(t, f, out)
		run := *bad.Agent
		run.Coverage = append([]domain.AgendaCoverage(nil), out.Run.Coverage...)
		mutate(run.Coverage)
		bad.Agent = &run
		if err := bad.ValidateAgainst(f.b); err == nil {
			t.Errorf("%s: the run record validates", name)
		}
	}
}

func TestV2ClosingSubmitAfterExhaustion(t *testing.T) {
	f := newFixture(t)
	steps := []Step{}
	for i := 0; i < 81; i++ { // the agenda budget for 2 items is the default 80 calls
		steps = append(steps, Step{Tool: investigate.ToolListRenders})
	}
	steps = append(steps, Step{Tool: investigate.ToolSubmitFindings, Args: submitArgs(
		map[string]any{"changeItem": f.behaviour, "disposition": "not-examined", "summary": "budget spent listing renders"},
		map[string]any{"changeItem": f.values2, "disposition": "not-examined", "summary": "budget spent listing renders"})})
	d := &Scripted{Model: "test-model", ModelVersion: "v", Steps: steps}
	out, err := New(f.st, WithDriver(DriverScripted, d)).Investigate(context.Background(), f.input(v2Config(domain.AgentBudget{})))
	must(t, err)
	if out.Run.Status != domain.AgentBudgetExhausted || out.Run.ToolCalls != 81 {
		t.Fatalf("run: %+v", out.Run)
	}
	if len(d.Results) != 81 {
		t.Fatalf("the scripted driver stopped at exhaustion after %d steps", len(d.Results))
	}
	// The driver stops at exhaustion; a model can still close the agenda.
	f2 := newFixture(t)
	s := f2.v2session(t, f2.input(investigate.AgentConfig{}))
	for i := 0; i < 81; i++ {
		_, _ = call(t, s, investigate.ToolListRenders, map[string]any{})
	}
	if _, err := call(t, s, investigate.ToolListRenders, map[string]any{}); !errors.Is(err, investigate.ErrBudgetExhausted) {
		t.Fatalf("after exhaustion: %v", err)
	}
	ok(t, s, investigate.ToolSubmitFindings, submitArgs(
		map[string]any{"changeItem": f2.behaviour, "disposition": "not-examined", "summary": "budget"},
		map[string]any{"changeItem": f2.values2, "disposition": "not-examined", "summary": "budget"}))
	o2, err := s.Outcome(context.Background(), f2.st, DriverScripted, investigate.DriverResult{Model: "m", ModelVersion: "v", Stop: "submitted"})
	must(t, err)
	if o2.Run.Status != domain.AgentBudgetExhausted || o2.Run.Coverage[0].Summary != "budget" {
		t.Fatalf("closing after exhaustion: %+v", o2.Run)
	}
	if err := runRecord(t, f2, o2).ValidateAgainst(f2.b); err != nil {
		t.Fatal(err)
	}
}

func TestV2CoverageWithoutSubmission(t *testing.T) {
	f := newFixture(t)
	d := &Scripted{Steps: []Step{{Tool: investigate.ToolRecordAssertion, Args: map[string]any{"changeItem": f.behaviour, "stance": "raises-concern",
		"statement": "unpinned?", "citations": []map[string]any{{"artifact": f.notes.ID, "quote": "default rotation policy is Always"}}}}}}
	out, err := New(f.st, WithDriver(DriverScripted, d)).Investigate(context.Background(), f.input(v2Config(domain.AgentBudget{})))
	must(t, err)
	c := out.Run.Coverage
	if out.Run.Status != domain.AgentFailed || len(c) != 2 || c[0].Disposition != domain.CoverNotExamined || !strings.Contains(c[0].Summary, "no closing entry") || len(c[0].Records) != 1 {
		t.Fatalf("coverage without submission: %+v", out.Run)
	}
	if err := runRecord(t, f, out).ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
}

func TestV2SubjectResolution(t *testing.T) {
	f := newFixture(t)
	fake := &fakeOverride{f: f}
	s := f.v2session(t, f.input(investigate.AgentConfig{}), fake)
	for _, c := range []struct {
		subject, rule, node, note string
	}{
		{"controller.rotationPolicy", SubjectValuesKey, f.valsNode, "set in"},
		{"values:controller.unsetKey", SubjectValuesKey, f.valsNode, "not set"},
		{"Deployment/demo/demo-controller", SubjectObjectRef, f.deploy, ""},
		{"deployment/demo-controller", SubjectObjectRef, f.deploy, ""},
		{f.rcItem, SubjectRenderedChange, f.deploy, ""},
		{"--rotation-policy=Never", SubjectRenderedSetting, f.deploy, ""},
		{"env/apps/demo/values.yaml", SubjectNodeKey, f.valsNode, ""},
		{"apps/demo/values.yaml", SubjectArtifact, f.valsNode, ""},
		{f.values.ID, SubjectArtifact, f.valsNode, ""},
		{deployID.String(), SubjectNodeKey, f.deploy, ""},
		{f.cfgMap, SubjectNodeID, f.cfgMap, ""},
	} {
		s.SetCall(CallInfo{ID: "call-" + c.subject, Model: "m", ModelVersion: "v"})
		r := ok(t, s, investigate.ToolRequestValidation, map[string]any{"changeItem": f.behaviour, "kind": "override-inspection", "subjects": []string{c.subject}})
		res := r["subjects"].([]any)[0].(map[string]any)
		nodes := res["nodes"].([]any)
		if res["rule"] != c.rule || len(nodes) != 1 || nodes[0] != c.node || !strings.Contains(stringOf(res["note"]), c.note) {
			t.Errorf("%s: %v", c.subject, res)
		}
		if c.rule == SubjectRenderedChange {
			if v, _ := r["validation"].(map[string]any); v == nil || !strings.Contains(string(mustJSON(v["renderedChanges"])), f.rcItem) {
				t.Errorf("a rendered change named as a subject joins the request's rendered changes: %v", r)
			}
		}
	}
	_, err := call(t, s, investigate.ToolRequestValidation, map[string]any{"changeItem": f.behaviour, "kind": "override-inspection", "subjects": []string{"the controller"}})
	if !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), "accepted subject forms") {
		t.Fatalf("unresolvable subject: %v", err)
	}
	// v1 keeps its contract: graph node ids or keys only.
	s1, _ := f.session(t, fake)
	if _, err := call(t, s1, investigate.ToolRequestValidation, map[string]any{"changeItem": f.behaviour, "kind": "override-inspection",
		"subjects": []string{"controller.rotationPolicy"}}); !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), "unknown subject") {
		t.Fatalf("v1 subject: %v", err)
	}
}

func stringOf(v any) string {
	s, _ := v.(string)
	return s
}

func TestDeterministicPassAndItsOutputs(t *testing.T) {
	f := newFixture(t)
	fake := &fakeOverride{f: f}
	pass, err := DeterministicPass(context.Background(), []investigate.Validator{fake, fake}, investigate.ValidationInput{Bundle: f.b, Graph: f.g, Traversal: f.trav})
	must(t, err)
	if len(pass.Records) != 1 || pass.Records[0].ChangeItem != f.behaviour || fake.calls != 2 {
		t.Fatalf("pass: %d records, %d validator calls", len(pass.Records), fake.calls)
	}
	// A pass output artifact is readable and citable, and is not the agent's.
	out, err := f.st.Put(context.Background(), investigate.ArtifactSpec{Kind: domain.ArtifactValidationOutput, Path: "derived/validate/override.txt",
		Origin: domain.OriginDerived, MediaType: "text/plain", URI: "ri:validate", Producer: "fake.override@v1"}, []byte("pinned: controller.rotationPolicy=Never\n"))
	must(t, err)
	in := f.input(investigate.AgentConfig{})
	in.Validations, in.ValidationArtifacts, in.ValidationEvidence = pass.Records, []domain.ArtifactRef{out}, pass.Evidence
	s := f.v2session(t, in)
	if task := Agenda(f.b, pass.Records); len(task) != 2 || task[0].PriorityRule != AgendaBehaviour {
		t.Fatalf("agenda with a pass: %+v", task)
	}
	ok(t, s, investigate.ToolReadArtifact, map[string]any{"artifact": out.ID})
	ok(t, s, investigate.ToolRecordHypothesis, map[string]any{"changeItem": f.behaviour, "statement": "the validator saw the pin",
		"citations": []map[string]any{{"artifact": out.ID, "quote": "controller.rotationPolicy=Never"}}})
	o, err := s.Outcome(context.Background(), f.st, DriverScripted, investigate.DriverResult{Model: "m", ModelVersion: "v", Stop: "error:x"})
	must(t, err)
	for _, a := range o.Artifacts {
		if a.ID == out.ID {
			t.Fatal("a deterministic-pass artifact is reported as the agent's")
		}
	}
	b, _ := json.Marshal(o.Hypotheses)
	if !strings.Contains(string(b), "ev-") {
		t.Fatal("no citation evidence")
	}
}

func TestV2ReplayReproduces(t *testing.T) {
	f := newFixture(t)
	cite := []map[string]any{{"artifact": f.values.ID, "quote": "rotationPolicy: Never"}}
	steps := []Step{
		{Tool: investigate.ToolGetChangeItem, Args: map[string]any{"id": f.behaviour}},
		{Tool: investigate.ToolRequestValidation, Args: map[string]any{"changeItem": f.behaviour, "kind": "override-inspection", "subjects": []string{"controller.rotationPolicy"}}},
		{Tool: investigate.ToolSubmitFindings, Args: submitArgs(
			map[string]any{"changeItem": f.behaviour, "disposition": "claims-mitigation", "summary": "pinned by the values file", "citations": cite},
			map[string]any{"changeItem": f.values2, "disposition": "examined-no-concern", "summary": "the key is not set", "citations": cite})},
	}
	d := &Scripted{Model: "test-model", ModelVersion: "v", Steps: steps}
	out, err := New(f.st, WithDriver(DriverScripted, d), WithValidators(&fakeOverride{f: f})).Investigate(context.Background(), f.input(v2Config(domain.AgentBudget{})))
	must(t, err)
	for _, r := range d.Results {
		must(t, r.Err)
	}
	rc, err := f.st.Open(context.Background(), out.Artifacts[0].Digest)
	must(t, err)
	var buf strings.Builder
	_, _ = io.Copy(&buf, rc)
	rc.Close()
	rep := &Replay{Transcript: []byte(buf.String())}
	again, err := New(f.st, WithDriver(DriverReplay, rep), WithValidators(&fakeOverride{f: f})).Investigate(context.Background(), f.input(investigate.AgentConfig{
		Enabled: true, Driver: DriverReplay, Model: "test-model", PromptVersion: promptV2}))
	must(t, err)
	if len(rep.Mismatches) > 0 || again.Run.Transcript != out.Run.Transcript || again.Run.Status != domain.AgentSubmitted {
		t.Fatalf("v2 replay: %+v %+v", rep.Mismatches, again.Run)
	}
	a, _ := json.Marshal(out.Run.Coverage)
	b, _ := json.Marshal(again.Run.Coverage)
	if string(a) != string(b) || out.Run.Coverage[0].Disposition != domain.CoverClaimsMitigation {
		t.Fatalf("coverage differs on replay:\n%s\n%s", a, b)
	}
	if err := runRecord(t, f, out).ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
}
