package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

const promptV6 = "investigate-agent/v6"

func v6Config() investigate.AgentConfig {
	return investigate.AgentConfig{Enabled: true, Driver: DriverScripted, Model: "test-model", PromptVersion: promptV6}
}

func widgetRun(t *testing.T, f *widgetFixture, d *Scripted, vals ...investigate.Validator) *investigate.AgentOutcome {
	t.Helper()
	cfg := v6Config()
	in := f.input(cfg)
	out, err := New(f.st, WithDriver(DriverScripted, d), WithValidators(vals...), WithClock(fixedNow)).Investigate(context.Background(), in)
	must(t, err)
	return out
}

// widgetRecord is the run record of an agent outcome on the widget fixture.
func widgetRecord(t *testing.T, f *widgetFixture, out *investigate.AgentOutcome, pre ...domain.ValidationRecord) *domain.Investigation {
	t.Helper()
	cfg := investigate.Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound, Agent: v6Config()}
	inv := &domain.Investigation{SchemaVersion: domain.InvestigationSchemaVersion, Bundle: f.b.ID, Snapshot: f.b.Snapshot.ID, ChangeRecord: f.b.Change.ID,
		Environment: f.b.Snapshot.Environment, Config: cfg.Digest(), Status: domain.InvestigationRunning, Traversal: f.trav, Agent: &out.Run,
		Hypotheses: out.Hypotheses, Assertions: out.Assertions, AgentNotes: out.Notes, AgentClaims: out.Claims, AgentVerdicts: out.Verdicts,
		EvidenceRequests: out.EvidenceRequests, ValidationRequests: out.ValidationRequests,
		Validations: append(append([]domain.ValidationRecord(nil), pre...), out.Validations...), Artifacts: out.Artifacts, Evidence: out.Evidence,
		Review: domain.HumanReview{State: domain.ReviewAwaiting}, CreatedAt: fixedNow()}
	inv.ID = domain.InvestigationID(inv.Bundle, inv.Config)
	inv.AppendAudit(domain.AuditEvent{At: fixedNow(), Stage: domain.StageInvestigate, Event: "completed", Actor: "agent:" + out.Run.Model, Refs: []string{out.Run.Transcript}})
	return inv
}

// widgetMitigation is a well-formed mitigation for the fixture's item: the
// rule is scoped to widgets that leave the field unset.
func widgetMitigation(basis ...string) map[string]any {
	return map[string]any{
		"proposedRule": map[string]any{
			"assertion": map[string]any{
				"subject": map[string]any{"family": "crd-field", "product": "widgets", "group": "example.dev", "kind": "Widget", "path": "spec.cleanup.policy"},
				"change":  map[string]any{"type": "default-changed", "before": `"Retain"`, "after": `"Purge"`},
				"applicability": map[string]any{"exposure": map[string]any{"op": "resource", "group": "example.dev", "kind": "Widget",
					"of": []any{map[string]any{"op": "field", "path": "spec.cleanup.policy", "state": "unset"}}}},
				"consequence": map[string]any{"kind": "behavior-change", "exposedClass": "review-required"},
				"statement":   "A Widget that sets spec.cleanup.policy keeps its behaviour; one that leaves it unset is purged on deletion.",
			},
			"release": map[string]any{"from": "1.4.0", "to": "1.5.0"},
		},
		"environmentBasis":      basis,
		"requestedVerification": "delete a Widget with and without spec.cleanup.policy: Retain after upgrading the controller; storage is kept only with it",
	}
}

func (f *widgetFixture) upstreamCite() []map[string]any {
	return []map[string]any{{"artifact": f.notes.ID, "locator": "## Breaking changes", "quote": "Set `spec.cleanup.policy: Retain` on a Widget to keep the previous behaviour."}}
}

func (f *widgetFixture) itemScript() map[string][]Step {
	good := f.upstreamCite()
	return map[string][]Step{
		f.item: {
			{Call: "call-w", Tool: investigate.ToolRecordClaim, Args: map[string]any{"changeItem": f.item, "kind": "present", "setting": "spec.cleanup.policy", "input": "env/apps/shop/widgets.yaml"}},
			{Call: "call-w", Tool: investigate.ToolRecordHypothesis, Args: map[string]any{"changeItem": f.item,
				"statement": "widgets that set the policy keep their storage", "citations": good, "mitigation": widgetMitigation("Widget/shop/orders")}},
			{Call: "call-w", Tool: investigate.ToolRecordHypothesis, Args: map[string]any{"changeItem": f.item,
				"statement": "the same, for a later release", "citations": good,
				"mitigation": func() map[string]any {
					m := widgetMitigation("Widget/shop/orders")
					m["proposedRule"].(map[string]any)["release"] = map[string]any{"from": "1.5.0", "to": "1.6.0"}
					return m
				}()}},
		},
	}
}

func TestPromptV6(t *testing.T) {
	p, err := LoadPrompt(promptV6)
	must(t, err)
	if !p.Features.Dossier || !p.Features.Verdict || !p.Features.Mitigation || !p.Features.PerItem || len(p.Tools) != len(investigate.ToolNames)+2 {
		t.Fatalf("v6: %+v (%d tools)", p.Features, len(p.Tools))
	}
	for _, word := range []string{"cert-manager", "rotationPolicy", "payments", "ingress", "prometheus", "Certificate"} {
		for name, text := range map[string]string{"system": p.System, "tools": string(p.ToolsJSON)} {
			if strings.Contains(text, word) {
				t.Errorf("the v6 %s prompt names %q: prompts are generic", name, word)
			}
		}
	}
	f := newWidgetFixture(t, widgetOpts{})
	_, data, ref, err := itemDossier(context.Background(), f.st, f.input(v6Config()), f.item)
	must(t, err)
	agenda := ItemAgenda(f.b, nil)
	task, err := p.renderItemTask(f.input(v6Config()), agenda[0], 1, 2, 10, ItemDefaultBudget, data, ref.ID)
	must(t, err)
	for _, want := range []string{ref.ID, `"format": "ri.dev/item-dossier/v1"`, "inherited-default", "verdict"} {
		if !strings.Contains(task, want) {
			t.Errorf("v6 item task lacks %q", want)
		}
	}
	other, err := p.renderItemTask(f.input(v6Config()), agenda[0], 1, 2, 10, ItemDefaultBudget, append(append([]byte(nil), data...), ' ', 'x'), ref.ID)
	must(t, err)
	if p.Digest(task) == p.Digest(other) {
		t.Error("the prompt digest does not pin the dossier")
	}
}

func TestV6DossierVerdictMitigation(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{})
	script := f.itemScript()
	good := f.upstreamCite()
	bad := map[string]any{"artifact": f.notes.ID, "quote": "this sentence is not in the release notes"}
	d := &Scripted{Model: "test-model", ModelVersion: "test-model-1", Provider: "test", Turns: 3, Tokens: 1000, ByItem: script}
	// ids are content-derived: the closing call can name them up front
	var widgetsFile string
	for _, a := range f.b.Manifest.Artifacts {
		if a.Path == "env/apps/shop/widgets.yaml" {
			widgetsFile = a.ID
		}
	}
	claim := domain.SettingClaimID(f.item, domain.ClaimPresent, "spec.cleanup.policy", widgetsFile, "", "call-w")
	hyp := domain.AgentRecordID(domain.HypothesisIDPrefix, f.item, "widgets that set the policy keep their storage", "call-w")
	script[f.item] = append(script[f.item], Step{Call: "call-w", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.item, "disposition": "claims-mitigation",
		"summary": "explicitly set widgets keep their storage", "citations": good,
		"verdict": map[string]any{"class": "potentially-affected", "confidence": "medium",
			"mechanism":            "the controller's default for spec.cleanup.policy flips from Retain to Purge",
			"environmentReasoning": "two widgets set Retain explicitly; the cache widget inherits the new default",
			"citations":            append(append([]map[string]any(nil), good...), bad),
			"verifiedFacts":        []string{claim, "vr-doesnotexist"},
			"assumptions":          []string{"the cache widget's storage matters"},
			"mitigations":          []string{hyp, "hyp-doesnotexist"},
			"proposedValidations":  []string{"override-inspection of every Widget"},
			"wouldOverturn":        "every widget setting the policy explicitly"}})})
	script[f.other] = []Step{{Call: "call-v", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.other,
		"disposition": "not-examined", "summary": "out of time"})}}
	out := widgetRun(t, f, d)
	for _, r := range d.Results {
		if r.Err != nil {
			t.Fatalf("%s: %v", r.Tool, r.Err)
		}
	}
	if len(out.Hypotheses) != 2 {
		t.Fatalf("hypotheses: %+v", out.Hypotheses)
	}
	h, rej := out.Hypotheses[0], out.Hypotheses[1]
	if h.Mitigation == nil || h.Mitigation.Item != f.item || h.Mitigation.ProposedRule.Release.To != "1.5.0" || h.MitigationRejected != "" {
		t.Fatalf("well-formed mitigation: %+v", h)
	}
	if rej.Mitigation != nil || !strings.Contains(rej.MitigationRejected, "not within this run's transition") {
		t.Fatalf("out-of-bounds mitigation: %+v", rej)
	}
	if len(out.Verdicts) != 1 {
		t.Fatalf("verdicts: %+v", out.Verdicts)
	}
	v := out.Verdicts[0]
	if v.Class != domain.FindingPotentiallyAffected || v.DroppedCitations != 1 || v.DroppedVerifiedFacts != 1 || len(v.Citations) != 1 ||
		len(v.VerifiedFacts) != 1 || len(v.Mitigations) != 1 || v.Mitigations[0] != h.ID || len(v.Dropped) != 3 || v.Dossier == "" ||
		v.StatementKind != domain.StatementUnverifiedProse || v.Provenance.Model != "test-model" || v.Provenance.CallID != "call-w" {
		t.Fatalf("verdict: %+v", v)
	}
	r := out.Run
	if r.Sessions != 2 || r.DroppedCitations != 1 || r.DroppedVerifiedFacts != 1 || r.WallMillis < 0 {
		t.Fatalf("cost: %+v", r)
	}
	c := r.Coverage
	if c[0].ChangeItem != f.item || c[0].Verdict != v.ID || c[0].Dossier != v.Dossier || !c[0].Examined || c[1].Verdict != "" || c[1].Dossier == "" {
		t.Fatalf("coverage: %+v", c)
	}
	found := false
	for _, a := range out.Artifacts {
		found = found || (a.ID == v.Dossier && a.Kind == domain.ArtifactItemDossier)
	}
	if !found {
		t.Fatal("the dossier is not a run artifact")
	}
	inv := widgetRecord(t, f, out)
	if err := inv.ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
	// replay reproduces every result byte for byte, dossiers included
	rc, err := f.st.Open(context.Background(), out.Artifacts[0].Digest)
	must(t, err)
	original, _ := io.ReadAll(rc)
	rc.Close()
	rep := &Replay{Transcript: original}
	cfg := v6Config()
	cfg.Driver = DriverReplay
	again, err := New(f.st, WithDriver(DriverReplay, rep), WithClock(fixedNow)).Investigate(context.Background(), f.input(cfg))
	must(t, err)
	if len(rep.Mismatches) != 0 || again.Run.Transcript != out.Run.Transcript || again.Run.PromptDigest != out.Run.PromptDigest {
		t.Fatalf("replay: %+v", rep.Mismatches)
	}
	a, _ := json.Marshal(out.Verdicts)
	b, _ := json.Marshal(again.Verdicts)
	if !bytes.Equal(a, b) {
		t.Fatalf("replayed verdicts differ:\n%s\n%s", a, b)
	}
}

func TestV6VerdictRules(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{})
	good := f.upstreamCite()
	bad := []map[string]any{{"artifact": f.notes.ID, "quote": "not there"}}
	verdict := func(class string, cites []map[string]any) map[string]any {
		return map[string]any{"class": class, "confidence": "low", "mechanism": "m", "environmentReasoning": "r", "citations": cites, "wouldOverturn": "w"}
	}
	entry := func(v map[string]any) map[string]any {
		e := map[string]any{"changeItem": f.item, "disposition": "explains", "summary": "s", "citations": good}
		if v != nil {
			e["verdict"] = v
		}
		return submitArgs(e)
	}
	for _, c := range []struct {
		args any
		want string
	}{
		{entry(nil), "verdict is required"},
		{entry(verdict("safe", good)), "verdict.class"},
		{entry(map[string]any{"class": "affected", "confidence": "sure", "mechanism": "m", "environmentReasoning": "r", "citations": good, "wouldOverturn": "w"}), "verdict.confidence"},
		{entry(map[string]any{"class": "affected", "confidence": "low", "mechanism": "m", "environmentReasoning": "r", "citations": good}), "verdict.wouldOverturn"},
		{entry(verdict("not-affected", bad)), "needs at least one citation that resolves"},
	} {
		d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: {{Call: "call-x", Tool: investigate.ToolSubmitFindings, Args: c.args}}}}
		widgetRun(t, f, d)
		if err := d.Results[0].Err; !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v", c.want, err)
		}
	}
	// insufficient-evidence may rest on nothing that resolves; the drop is counted
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: {{Call: "call-x", Tool: investigate.ToolSubmitFindings, Args: entry(verdict("insufficient-evidence", bad))}}}}
	out := widgetRun(t, f, d)
	if d.Results[0].Err != nil || len(out.Verdicts) != 1 || out.Verdicts[0].DroppedCitations != 1 || out.Run.DroppedCitations != 1 {
		t.Fatalf("insufficient-evidence verdict: %v %+v", d.Results[0].Err, out.Verdicts)
	}
	// a v5 session takes no verdict
	s, _, err := Prepare(f.st, investigate.InvestigateInput{Bundle: f.b, Graph: f.g, Traversal: f.trav,
		Config: investigate.AgentConfig{Enabled: true, Driver: DriverScripted, PromptVersion: promptV5}}, nil, fixedNow)
	must(t, err)
	s.BeginItem(f.item, ItemDefaultBudget, "d")
	s.SetCall(CallInfo{ID: "call-5", Model: "m", ModelVersion: "v"})
	if _, err := s.Dispatch(context.Background(), investigate.ToolSubmitFindings, mustJSON(entry(verdict("affected", good)))); err == nil || !strings.Contains(err.Error(), "takes no verdict") {
		t.Fatalf("v5 verdict: %v", err)
	}
}

func TestV6MitigationRules(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{})
	good := f.upstreamCite()
	envOnly := []map[string]any{{"artifact": f.values.ID, "quote": "legacyMode: true"}}
	mut := func(edit func(rule, assertion map[string]any)) map[string]any {
		m := widgetMitigation("Widget/shop/orders")
		rule := m["proposedRule"].(map[string]any)
		edit(rule, rule["assertion"].(map[string]any))
		return m
	}
	cases := []struct {
		name  string
		m     map[string]any
		cites []map[string]any
		want  string
	}{
		{"other product", mut(func(_, a map[string]any) { a["subject"].(map[string]any)["product"] = "gadgets" }), good, "not this investigation's product"},
		{"subject absent", mut(func(_, a map[string]any) { a["subject"].(map[string]any)["kind"] = "Gizmo" }), good, "does not occur in the bundle"},
		{"undecidable", mut(func(_, a map[string]any) {
			a["applicability"] = map[string]any{"exposure": map[string]any{"op": "undecidable", "reason": "runtime-behavior-gap", "needed": "a live read"}}
		}), good, "undecidable"},
		{"incomplete", mut(func(_, a map[string]any) { delete(a, "consequence") }), good, "consequence is required"},
		{"bounds unparsable", mut(func(r, _ map[string]any) { r["release"] = map[string]any{"from": "old", "to": "1.5.0"} }), good, "release bounds"},
		{"basis unresolved", widgetMitigation("Widget/shop/nonexistent"), good, "environmentBasis"},
		{"basis claim unknown", widgetMitigation("clm-doesnotexist"), good, "no verified setting claim"},
		{"no upstream citation", widgetMitigation("Widget/shop/orders"), envOnly, "no citation is upstream text"},
	}
	for i, c := range cases {
		d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: {{Call: "call-m", Tool: investigate.ToolRecordHypothesis,
			Args: map[string]any{"changeItem": f.item, "statement": "hypothesis " + c.name + string(rune('a'+i)), "citations": c.cites, "mitigation": c.m}}}}}
		out := widgetRun(t, f, d)
		if d.Results[0].Err != nil {
			t.Errorf("%s: a malformed mitigation must keep the hypothesis as free text, got %v", c.name, d.Results[0].Err)
			continue
		}
		if len(out.Hypotheses) != 1 || out.Hypotheses[0].Mitigation != nil || !strings.Contains(out.Hypotheses[0].MitigationRejected, c.want) {
			t.Errorf("%s: %+v", c.name, out.Hypotheses)
		}
		if !strings.Contains(string(d.Results[0].Result), "not recorded") {
			t.Errorf("%s: the model is not told: %s", c.name, d.Results[0].Result)
		}
	}
	// a prompt without the feature rejects a mitigation outright
	s, _, err := Prepare(f.st, investigate.InvestigateInput{Bundle: f.b, Graph: f.g, Traversal: f.trav,
		Config: investigate.AgentConfig{Enabled: true, Driver: DriverScripted, PromptVersion: promptV5}}, nil, fixedNow)
	must(t, err)
	s.BeginItem(f.item, ItemDefaultBudget, "d")
	s.SetCall(CallInfo{ID: "call-5", Model: "m", ModelVersion: "v"})
	args := mustJSON(map[string]any{"changeItem": f.item, "statement": "x", "citations": good, "mitigation": widgetMitigation("Widget/shop/orders")})
	if _, err := s.Dispatch(context.Background(), investigate.ToolRecordHypothesis, args); err == nil || !strings.Contains(err.Error(), "takes no structured mitigation") {
		t.Fatalf("v5 mitigation: %v", err)
	}
}

func TestGetDossierTool(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{})
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: {
		{Call: "call-d", Tool: investigate.ToolGetDossier, Args: map[string]any{"changeItem": f.other}},
		{Call: "call-d", Tool: investigate.ToolGetDossier, Args: map[string]any{"changeItem": "ci-nope"}},
	}}}
	widgetRun(t, f, d)
	var got Dossier
	if err := json.Unmarshal(d.Results[0].Result, &got); err != nil || got.Item != f.other || got.Settings[0].Kind != "values-key" {
		t.Fatalf("get_dossier: %v %+v", err, got)
	}
	if !errors.Is(d.Results[1].Err, investigate.ErrRejected) {
		t.Fatalf("unknown item: %v", d.Results[1].Err)
	}
}
