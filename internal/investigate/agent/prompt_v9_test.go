package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

const promptV9 = "investigate-agent/v9"

// v9 (docs/phase6/decision-intel/PLAN-M1.md change 5): per-item sessions kept,
// mitigation off, consumer tracing and the natural assessment on, a larger
// item budget; trace_consumers is the one tool v8 does not have and
// record_hypothesis no longer takes a mitigation. It is selected explicitly:
// v9 is the default since the M1 gate (TestDefaultPromptIsTheNewest).
func TestPromptV9(t *testing.T) {
	v8, err := LoadPrompt(promptV8)
	must(t, err)
	v9, err := LoadPrompt(promptV9)
	must(t, err)
	want := v8.Features
	want.Mitigation, want.TraceConsumers, want.Assessment, want.ItemCalls, want.ItemTurns = false, true, true, 24, 18
	if v9.Features != want {
		t.Fatalf("features: v8 %+v, v9 %+v", v8.Features, v9.Features)
	}
	names := func(p *Prompt) map[string]bool {
		out := map[string]bool{}
		for _, x := range p.Tools {
			out[x.Name] = true
		}
		return out
	}
	n8, n9 := names(v8), names(v9)
	if len(n9) != len(n8)+1 || !n9[investigate.ToolTraceConsumers] || n8[investigate.ToolTraceConsumers] {
		t.Fatalf("tools: v8 %v, v9 %v", n8, n9)
	}
	for _, x := range v9.Tools {
		if x.Name == investigate.ToolRecordHypothesis && strings.Contains(string(x.InputSchema), `"mitigation"`) {
			t.Error("v9 record_hypothesis still takes a mitigation")
		}
		if x.Name == investigate.ToolSubmitFindings && !strings.Contains(string(x.InputSchema), `"assessment"`) {
			t.Error("v9 submit_findings takes no assessment")
		}
	}
	// the tool-count check: a surface without the feature's tool is refused
	broken := *v9
	broken.Tools = nil
	for _, x := range v9.Tools {
		if x.Name != investigate.ToolTraceConsumers {
			broken.Tools = append(broken.Tools, x)
		}
	}
	if err := broken.checkSurface(); err == nil || !strings.Contains(err.Error(), "traceConsumers needs tool trace_consumers") {
		t.Fatalf("a v9 surface without trace_consumers: %v", err)
	}
	extra := *v8
	extra.Tools = append(append([]ToolSchema(nil), v8.Tools...), ToolSchema{Name: investigate.ToolTraceConsumers})
	if err := extra.checkSurface(); err == nil {
		t.Fatal("a v8 surface with trace_consumers (feature off) must be refused")
	}
	// the mission's instructions are present; the mitigation and consistency instructions are gone
	for _, s := range []string{
		"Work through these eight questions", "`trace_consumers` with `changeItem`", "never evidence",
		"**supported**", "**provisional**", "proof obligations", "`effectiveConfiguration`", "`inherited-default`",
		"`verified`", "`inferred`", "`unknown`", "`likelyImpact`", "`recommendedTests`",
	} {
		if !strings.Contains(v9.System, s) {
			t.Errorf("v9 system prompt lacks %q", s)
		}
	}
	for _, s := range []string{"record it with `record_hypothesis` and a structured `mitigation`", "Consistency:", "fact grammar"} {
		if strings.Contains(v9.System, s) {
			t.Errorf("v9 system prompt still has %q", s)
		}
	}
	if DefaultPromptVersion != promptV9 {
		t.Fatalf("default %s, want %s since the M1 gate", DefaultPromptVersion, promptV9)
	}
	if p, err := LoadPrompt(""); err != nil || p.Version != promptV9 {
		t.Fatalf("LoadPrompt(\"\"): %v %v", p, err)
	}
	if v8.Digest("t") == v9.Digest("t") {
		t.Error("v8 and v9 share a prompt digest")
	}
}

func TestPromptV9IsGeneric(t *testing.T) {
	assertGenericPrompt(t, promptV9, "nginx", "ledger", "ca.crt", "tls.crt", "tls.key", "trust bundle", "storefront", "catalog")
}

// v9's per-item budget is 24 calls and 18 turns (rule item@v2); v8's is
// unchanged, rule text included.
func TestV9ItemBudget(t *testing.T) {
	v8, _ := LoadPrompt(promptV8)
	v9, _ := LoadPrompt(promptV9)
	b8, r8 := ItemBudgetFor(v8.Features, nil, 10, 3)
	if old, oldRule := ItemBudget(nil, 10, 3); b8 != old || r8 != oldRule || b8.MaxToolCalls != 16 || b8.MaxTurns != 14 {
		t.Fatalf("v8 budget changed: %+v %q", b8, r8)
	}
	b9, r9 := ItemBudgetFor(v9.Features, nil, 10, 3)
	if b9.MaxToolCalls != 24 || b9.MaxTurns != 18 || b9.MaxTokens != b8.MaxTokens || b9.MaxSeconds != b8.MaxSeconds || b9.MaxReadBytes != b8.MaxReadBytes {
		t.Fatalf("v9 budget: %+v", b9)
	}
	if !strings.HasPrefix(r9, ItemBudgetV2Rule+":") {
		t.Fatalf("v9 rule %q", r9)
	}
	c, _ := ItemBudgetFor(v9.Features, &domain.AgentBudget{MaxToolCalls: 30}, 10, 3)
	if c.MaxToolCalls != 30 || c.MaxTurns != 18 {
		t.Fatalf("configured item budget: %+v", c)
	}
}

func decodeTrace(t *testing.T, r StepResult) *ConsumerTrace {
	t.Helper()
	if r.Err != nil {
		t.Fatalf("%s: %v", r.Tool, r.Err)
	}
	var ct ConsumerTrace
	must(t, json.Unmarshal(r.Result, &ct))
	return &ct
}

// trace_consumers on the fixture: Credential → Secret (produced at runtime)
// → Deployment (mount detail) and → ConfigMap (the configuration line naming
// the key file) → Deployment; every hop cites an artifact and a line; the
// Secret's absence and the log path under no mount are gaps.
func TestV9TraceConsumers(t *testing.T) {
	f := newConsumerFixture(t)
	d, out := v9Run(t, f,
		Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"changeItem": f.item}},
		Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"node": cfSecret, "side": "from"}},
		Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"node": "core/v1/Secret/shop/nope"}},
		f.v9Submit("potentially-affected", f.assessment()))
	ct := decodeTrace(t, d.Results[0])
	if ct.Side != "to" || len(ct.Start) != 1 || ct.Start[0].Key != cfCredential {
		t.Fatalf("start: %+v", ct.Start)
	}
	type hk struct {
		kind     domain.EdgeKind
		from, to string
	}
	hops := map[hk]TraceHop{}
	for _, h := range ct.Hops {
		if h.Artifact == "" || h.Line < 1 || h.Text == "" {
			t.Errorf("hop %s %s → %s cites no artifact line: %+v", h.Kind, h.From, h.To, h)
		}
		hops[hk{h.Kind, h.From, h.To}] = h
	}
	if _, ok := hops[hk{domain.EdgeProduces, cfCredential, cfSecret}]; !ok {
		t.Fatalf("no produces hop: %+v", ct.Hops)
	}
	m, ok := hops[hk{domain.EdgeReferences, cfSecret, cfDeployment}]
	if !ok || m.Mount == nil || len(m.Mount.Mounts) != 1 || m.Mount.Mounts[0].MountPath != "/etc/billing/tls" || m.Artifact != f.deploy.ID {
		t.Fatalf("mount hop: %+v", m)
	}
	r, ok := hops[hk{domain.EdgeReadsFile, cfSecret, cfConfigMap}]
	if !ok || r.File == nil || r.File.Key != "tls.key" || r.Artifact != f.config.ID || r.Text != "key_file = /etc/billing/tls/tls.key" || r.Line != 9 {
		t.Fatalf("reads-file hop: %+v", r)
	}
	if _, ok := hops[hk{domain.EdgeReferences, cfConfigMap, cfDeployment}]; !ok {
		t.Fatal("the configuration's own mount is not a hop")
	}
	var consumers []string
	for _, c := range ct.Consumers {
		consumers = append(consumers, c.Key)
	}
	if strings.Join(consumers, ",") != cfDeployment+","+cfConfigMap {
		t.Fatalf("consumers %v", consumers)
	}
	gaps := ""
	for _, g := range ct.Gaps {
		gaps += g.Subject + ": " + g.Detail + "\n"
	}
	for _, want := range []string{"is inferred", "/var/log/billing/app.log is under no volume mount"} {
		if !strings.Contains(gaps, want) {
			t.Errorf("gaps lack %q:\n%s", want, gaps)
		}
	}
	if from := decodeTrace(t, d.Results[1]); from.Side != "from" || len(from.Hops) == 0 || from.Graph == ct.Graph {
		t.Fatalf("side from: %+v", from)
	}
	if d.Results[2].Err == nil || !strings.Contains(d.Results[2].Err.Error(), "unknown node") {
		t.Fatalf("an unknown node: %v", d.Results[2].Err)
	}
	// both consumer graphs are run artifacts
	graphs := 0
	for _, a := range out.Artifacts {
		if a.Kind == domain.ArtifactGraph && a.Producer == ConsumerGraphProducer {
			graphs++
		}
	}
	if graphs != 2 || len(out.Assessments) != 1 || len(out.Assessments[0].UntracedConsumers) != 0 {
		t.Fatalf("graphs %d, assessments %+v", graphs, out.Assessments)
	}
	// The dossier lists the chains.
	dossier := ""
	for _, a := range out.Artifacts {
		if a.Kind == domain.ArtifactItemDossier {
			rc, err := f.st.Open(context.Background(), a.Digest)
			must(t, err)
			b, _ := io.ReadAll(rc)
			rc.Close()
			dossier = string(b)
		}
	}
	for _, want := range []string{DossierConsumersProducer, `"consumers"`, "reads-file: " + cfConfigMap + " data[app.ini] names /etc/billing/tls/tls.key"} {
		if !strings.Contains(dossier, want) {
			t.Errorf("dossier lacks %q", want)
		}
	}
}

// The completion check: a not-affected verdict with untraced consumers is
// rejected once; resubmitted, it is accepted with the consumers recorded as
// gaps; after tracing it is accepted at once; other classes record the gaps.
func TestV9CompletionCheck(t *testing.T) {
	f := newConsumerFixture(t)
	d, out := v9Run(t, f, f.v9Submit("not-affected", f.assessment()), f.v9Submit("not-affected", f.assessment()))
	if d.Results[0].Err == nil || !strings.Contains(d.Results[0].Err.Error(), "were never retrieved in this session") ||
		!strings.Contains(d.Results[0].Err.Error(), cfDeployment) {
		t.Fatalf("first not-affected submit: %v", d.Results[0].Err)
	}
	if d.Results[1].Err != nil || len(out.Assessments) != 1 {
		t.Fatalf("second submit: %v %+v", d.Results[1].Err, out.Assessments)
	}
	a := out.Assessments[0]
	if strings.Join(a.UntracedConsumers, ",") != cfConfigMap+","+cfDeployment && strings.Join(a.UntracedConsumers, ",") != cfDeployment+","+cfConfigMap {
		t.Fatalf("untraced: %v", a.UntracedConsumers)
	}
	gaps := 0
	for _, u := range a.Unknown {
		if strings.HasPrefix(u, "investigation gap (recorded by the tool layer): consumer ") {
			gaps++
		}
	}
	if gaps != 2 || a.Verdict != out.Verdicts[0].ID || out.Verdicts[0].Class != domain.FindingNotAffected {
		t.Fatalf("gaps %d in %v; verdict %s", gaps, a.Unknown, a.Verdict)
	}

	// traced first: accepted at once, no gaps
	d, out = v9Run(t, f, Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"changeItem": f.item}},
		f.v9Submit("not-affected", f.assessment()))
	if d.Results[1].Err != nil || len(out.Assessments[0].UntracedConsumers) != 0 {
		t.Fatalf("after tracing: %v %+v", d.Results[1].Err, out.Assessments)
	}
	// reading the consumers' own objects counts as well
	d, out = v9Run(t, f,
		Step{Call: "call-9", Tool: investigate.ToolReadArtifact, Args: map[string]any{"artifact": f.config.ID, "fromLine": 1, "toLine": 10}},
		Step{Call: "call-9", Tool: investigate.ToolReadArtifact, Args: map[string]any{"artifact": f.deploy.ID, "fromLine": 1, "toLine": 25}},
		f.v9Submit("not-affected", f.assessment()))
	if d.Results[2].Err != nil || len(out.Assessments[0].UntracedConsumers) != 0 {
		t.Fatalf("after reading: %v %+v", d.Results[2].Err, out.Assessments)
	}
	// another class: accepted at once, the gaps recorded
	d, out = v9Run(t, f, f.v9Submit("potentially-affected", f.assessment()))
	if d.Results[0].Err != nil || len(out.Assessments[0].UntracedConsumers) != 2 {
		t.Fatalf("potentially-affected: %v %+v", d.Results[0].Err, out.Assessments)
	}
}

// A verified entry needs citations that resolve; the assessment is required.
func TestV9AssessmentChecks(t *testing.T) {
	f := newConsumerFixture(t)
	bad := f.assessment()
	bad["verified"] = []map[string]any{{"statement": "the default is documented", "citations": []map[string]any{{"artifact": f.notes.ID, "quote": "a sentence the notes do not hold"}}}}
	uncited := f.assessment()
	uncited["verified"] = []map[string]any{{"statement": "remembered from an earlier release", "citations": []map[string]any{}}}
	state := f.assessment()
	state["effectiveConfiguration"] = []map[string]any{{"setting": "spec.rotate", "state": "probably-unset"}}
	none := f.v9Submit("potentially-affected", nil)
	delete(none.Args.(map[string]any)["items"].([]any)[0].(map[string]any), "assessment")
	d, out := v9Run(t, f, f.v9Submit("potentially-affected", bad), f.v9Submit("potentially-affected", uncited),
		f.v9Submit("potentially-affected", state), none, f.v9Submit("potentially-affected", f.assessment()))
	for i, want := range []string{"the quote does not occur", "needs a citation that resolves", "want explicit, inherited-default or unknown", "assessment is required"} {
		if d.Results[i].Err == nil || !strings.Contains(d.Results[i].Err.Error(), want) {
			t.Errorf("submit %d: %v, want %q", i, d.Results[i].Err, want)
		}
	}
	if d.Results[4].Err != nil || len(out.Assessments) != 1 || len(out.Assessments[0].Verified) != 1 || len(out.Assessments[0].Verified[0].Citations) != 1 {
		t.Fatalf("a well-formed assessment: %v %+v", d.Results[4].Err, out.Assessments)
	}
	inv := widgetLikeRecord(t, f, out)
	must(t, inv.Validate())
	// v8 takes no assessment
	cfg := v9Config()
	cfg.PromptVersion = promptV8
	d8 := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: {f.v9Submit("potentially-affected", f.assessment())}}}
	if _, err := New(f.st, WithDriver(DriverScripted, d8), WithClock(fixedNow)).Investigate(context.Background(), f.input(cfg)); err != nil {
		t.Fatal(err)
	}
	if d8.Results[0].Err == nil || !strings.Contains(d8.Results[0].Err.Error(), "takes no assessment") {
		t.Fatalf("v8 with an assessment: %v", d8.Results[0].Err)
	}
}

// A v9 run replays deterministically (-agent-driver replay): the same
// results, transcript, verdicts and assessments.
func TestV9Replay(t *testing.T) {
	f := newConsumerFixture(t)
	_, out := v9Run(t, f,
		Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"changeItem": f.item}},
		Step{Call: "call-9", Tool: investigate.ToolReadArtifact, Args: map[string]any{"artifact": f.config.ID, "fromLine": 7, "toLine": 10}},
		f.v9Submit("not-affected", f.assessment()))
	var transcript []byte
	for _, a := range out.Artifacts {
		if a.ID == out.Run.Transcript {
			rc, err := f.st.Open(context.Background(), a.Digest)
			must(t, err)
			transcript, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	if !bytes.Contains(transcript, []byte(`"tool":"trace_consumers"`)) {
		t.Fatal("the transcript lacks the trace")
	}
	rep := &Replay{Transcript: transcript}
	cfg := v9Config()
	cfg.Driver = DriverReplay
	again, err := New(f.st, WithDriver(DriverReplay, rep), WithClock(fixedNow)).Investigate(context.Background(), f.input(cfg))
	must(t, err)
	if len(rep.Mismatches) != 0 || again.Run.Status != domain.AgentSubmitted {
		t.Fatalf("replay: %+v %s %s", rep.Mismatches, again.Run.Status, again.Run.Reason)
	}
	j := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	if again.Run.Transcript != out.Run.Transcript || again.Run.PromptDigest != out.Run.PromptDigest ||
		j(again.Assessments) != j(out.Assessments) || j(again.Verdicts) != j(out.Verdicts) || len(again.Assessments) != 1 {
		t.Fatal("the replay differs from the recording")
	}
}

// widgetLikeRecord is the run record of a v9 outcome on the consumer fixture.
func widgetLikeRecord(t *testing.T, f *consumerFixture, out *investigate.AgentOutcome) *domain.Investigation {
	t.Helper()
	cfg := investigate.Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound, Agent: v9Config()}
	inv := &domain.Investigation{SchemaVersion: domain.InvestigationSchemaVersion, Bundle: f.b.ID, Snapshot: f.b.Snapshot.ID, ChangeRecord: f.b.Change.ID,
		Environment: f.b.Snapshot.Environment, Config: cfg.Digest(), Status: domain.InvestigationRunning, Traversal: f.trav, Agent: &out.Run,
		Hypotheses: out.Hypotheses, Assertions: out.Assertions, AgentNotes: out.Notes, AgentClaims: out.Claims, AgentVerdicts: out.Verdicts,
		AgentAssessments: out.Assessments, EvidenceRequests: out.EvidenceRequests, ValidationRequests: out.ValidationRequests,
		Validations: out.Validations, Artifacts: out.Artifacts, Evidence: out.Evidence,
		Review: domain.HumanReview{State: domain.ReviewAwaiting}, CreatedAt: fixedNow()}
	inv.ID = domain.InvestigationID(inv.Bundle, inv.Config)
	inv.AppendAudit(domain.AuditEvent{At: fixedNow(), Stage: domain.StageInvestigate, Event: "completed", Actor: "agent:" + out.Run.Model, Refs: []string{out.Run.Transcript}})
	return inv
}
