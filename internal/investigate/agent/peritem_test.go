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
	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
)

const promptV4 = "investigate-agent/v4"

func v4Config(maxItems int) investigate.AgentConfig {
	return investigate.AgentConfig{Enabled: true, Driver: DriverScripted, Model: "test-model", PromptVersion: promptV4, MaxItems: maxItems}
}

func TestItemAgendaOrder(t *testing.T) {
	item := func(id string, m domain.Manifestation, scope string) domain.ChangeItem {
		return domain.ChangeItem{ID: id, Manifestation: m, ScopeRule: scope, InScope: true}
	}
	b := &domain.EvidenceBundle{Change: domain.ChangeEvidenceRecord{Items: []domain.ChangeItem{
		item("ci-d", domain.ManifestRenderedConfig, "scope:computed-breaking"),
		item("ci-e", domain.ManifestUnknown, "scope:breaking"),
		item("ci-c", domain.ManifestRenderedConfig, "scope:rendered-unexplained"),
		item("ci-a", domain.ManifestRenderedConfig, "scope:breaking"),
		item("ci-b", domain.ManifestControllerBehaviour, "scope:breaking"),
	}}, EnvironmentItems: []domain.ChangeItem{item("ci-0", domain.ManifestRenderedConfig, "scope:rendered-unexplained")}}
	var got []string
	for _, e := range ItemAgenda(b, []domain.ValidationRecord{rec("ci-d", domain.ValidationPass)}) {
		got = append(got, e.ChangeItem)
	}
	if strings.Join(got, ",") != "ci-b,ci-e,ci-0,ci-c,ci-a,ci-d" {
		t.Fatalf("order: %v", got)
	}
}

func TestPromptV4(t *testing.T) {
	p, err := LoadPrompt(promptV4)
	must(t, err)
	if p.Version != promptV4 || !p.Features.PerItem || !p.Features.DepthRules || p.Features.Claims || p.item == nil {
		t.Fatalf("v4: %+v", p.Features)
	}
	f := newFixture(t, true)
	agenda := ItemAgenda(f.b, nil)
	task, err := p.renderItemTask(f.input(investigate.AgentConfig{}), agenda[0], 1, 3, 10, ItemDefaultBudget)
	must(t, err)
	for _, want := range []string{f.behaviour, "item 1 of 3", f.notes.ID, f.rcItem, f.deploy, "exactly one entry"} {
		if !strings.Contains(task, want) {
			t.Errorf("item task lacks %q:\n%s", want, task)
		}
	}
	for _, other := range []string{f.values2, f.envItem} {
		if strings.Contains(task, other) {
			t.Errorf("item task mentions another item %s", other)
		}
	}
}

// itemScript: the behaviour item is examined properly; the environment item
// raises a concern on evidence it never retrieved (→ note); the values item
// raises a concern about another item (→ note) and is closed examined-no-concern.
func itemScript(f *fixture) map[string][]Step {
	values := []map[string]any{{"artifact": f.values.ID, "quote": "rotationPolicy: Never"}}
	notesCite := []map[string]any{{"artifact": f.notes.ID, "quote": "The values key `legacy.enabled` was removed."}}
	return map[string][]Step{
		f.behaviour: {
			{Call: "call-a", Tool: investigate.ToolGetChangeItem, Args: map[string]any{"id": f.behaviour}},
			{Call: "call-a", Tool: investigate.ToolReadArtifact, Args: map[string]any{"artifact": f.values.ID}},
			{Call: "call-a", Tool: investigate.ToolRecordAssertion, Args: map[string]any{"changeItem": f.behaviour, "stance": "raises-concern",
				"statement": "the default flips unless pinned", "citations": values}},
			{Call: "call-a", Tool: investigate.ToolRequestValidation, Args: map[string]any{"changeItem": f.behaviour, "kind": "override-inspection"}},
			{Call: "call-a", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.behaviour, "disposition": "raises-concern",
				"summary": "pinned? validation requested", "citations": values})},
		},
		f.envItem: {
			{Call: "call-b", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.envItem, "disposition": "raises-concern",
				"summary": "mode might matter", "citations": notesCite})},
		},
		f.values2: {
			{Call: "call-c", Tool: investigate.ToolGetChangeItem, Args: map[string]any{"id": f.values2}},
			{Call: "call-c", Tool: investigate.ToolRecordAssertion, Args: map[string]any{"changeItem": f.behaviour, "stance": "raises-concern",
				"statement": "also check the behaviour item", "citations": notesCite}},
			{Call: "call-c", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.values2, "disposition": "examined-no-concern",
				"summary": "the removed key is not set", "citations": notesCite})},
		},
	}
}

func runItems(t *testing.T, f *fixture, cfg investigate.AgentConfig, d *Scripted) *investigate.AgentOutcome {
	t.Helper()
	out, err := New(f.st, WithDriver(DriverScripted, d), WithValidators(&fakeOverride{f: f}), WithClock(fixedNow)).Investigate(context.Background(), f.input(cfg))
	must(t, err)
	return out
}

func TestPerItemSessions(t *testing.T) {
	f := newFixture(t, true)
	d := &Scripted{Model: "test-model", ModelVersion: "v", Provider: "test", Turns: 2, Tokens: 100, ByItem: itemScript(f)}
	out := runItems(t, f, v4Config(0), d)
	for _, r := range d.Results {
		if r.Err != nil {
			t.Fatalf("%s: %v", r.Tool, r.Err)
		}
	}
	r := out.Run
	if r.Status != domain.AgentSubmitted || r.ItemCap != DefaultItemCap || r.Tokens != 300 || r.Turns != 6 || r.ToolCalls != 9 || !strings.HasPrefix(r.BudgetRule, "item@v1") || r.Budget != ItemDefaultBudget {
		t.Fatalf("run: %+v", r)
	}
	c := r.Coverage
	if len(c) != 3 || c[0].ChangeItem != f.behaviour || c[1].ChangeItem != f.envItem || c[2].ChangeItem != f.values2 {
		t.Fatalf("order: %+v", c)
	}
	if c[0].Disposition != domain.CoverRaisesConcern || c[0].CallID != "call-a" || c[0].ToolCalls != 5 || c[0].Tokens != 100 || c[0].SessionStatus != domain.AgentSubmitted || !c[0].Examined {
		t.Fatalf("behaviour item: %+v", c[0])
	}
	if c[1].Disposition != domain.CoverNotExamined || !strings.Contains(c[1].Summary, "investigation-level note") || c[1].Assertion != "" {
		t.Fatalf("concern without item evidence: %+v", c[1])
	}
	if c[2].Disposition != domain.CoverExaminedNoConcern || c[2].CallID != "call-c" {
		t.Fatalf("values item: %+v", c[2])
	}
	if len(out.Notes) != 2 || out.Notes[0].About != f.envItem || out.Notes[1].About != f.behaviour || !strings.Contains(out.Notes[1].Reason, "session of") {
		t.Fatalf("notes: %+v", out.Notes)
	}
	for _, a := range out.Assertions {
		if a.Stance == domain.StanceRaisesConcern && a.ChangeItem != f.behaviour {
			t.Fatalf("a concern without item evidence became an item concern: %+v", a)
		}
	}
	// Each record carries its own session's prompt digest.
	if out.Assertions[0].Provenance.PromptDigest == r.PromptDigest || out.Assertions[0].Provenance.CallID != "call-a" {
		t.Fatalf("per-item provenance: %+v", out.Assertions[0].Provenance)
	}
	if err := runRecord(t, f, out).ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
	// Notes never reach the classifier as concerns.
	cls, err := classify.New().Classify(context.Background(), investigate.ClassifyInput{Bundle: f.b, Traversal: f.trav, Agent: out, Validations: out.Validations,
		Config: investigate.Config{Policy: classify.Policy}})
	must(t, err)
	for _, fd := range cls.Findings {
		for _, g := range fd.Gaps {
			for _, n := range out.Notes {
				if g.Subject == n.ID {
					t.Fatalf("a note reached the policy: %+v", g)
				}
			}
		}
	}
}

func TestPerItemCapAndSubmissionScope(t *testing.T) {
	f := newFixture(t, true)
	script := itemScript(f)
	script[f.behaviour] = append([]Step{
		{Call: "call-a", Tool: investigate.ToolSubmitFindings, Args: submitArgs(
			map[string]any{"changeItem": f.behaviour, "disposition": "not-examined", "summary": "x"},
			map[string]any{"changeItem": f.values2, "disposition": "not-examined", "summary": "x"})},
		{Call: "call-a", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.envItem, "disposition": "not-examined", "summary": "x"})},
	}, script[f.behaviour]...)
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: script}
	out := runItems(t, f, v4Config(2), d)
	for _, r := range d.Results[:2] {
		if !errors.Is(r.Err, investigate.ErrRejected) || !strings.Contains(r.Err.Error(), "closes exactly one item") {
			t.Fatalf("a submission outside the session's item: %v", r.Err)
		}
	}
	c := out.Run.Coverage
	if out.Run.ItemCap != 2 || len(c) != 3 || c[2].Disposition != domain.CoverNotExamined || c[2].Summary != "beyond cap 2: no session (closed by the runner, not by the model)" || c[2].CallID != "" {
		t.Fatalf("cap: %+v", c)
	}
	if !strings.Contains(out.Run.BudgetRule, "2 of 3 agenda items get a session") {
		t.Fatalf("budget rule: %s", out.Run.BudgetRule)
	}
	if err := runRecord(t, f, out).ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
}

func TestPerItemBudgetsAreIndependent(t *testing.T) {
	f := newFixture(t, true)
	var spend []Step
	for i := 0; i < 17; i++ { // one more than the item default
		spend = append(spend, Step{Call: "call-a", Tool: investigate.ToolListRenders})
	}
	script := itemScript(f)
	script[f.behaviour] = spend
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: script}
	out := runItems(t, f, v4Config(0), d)
	c := out.Run.Coverage
	if out.Run.Status != domain.AgentBudgetExhausted || !strings.Contains(out.Run.Reason, f.behaviour+": budget:tool-calls") {
		t.Fatalf("run: %+v", out.Run)
	}
	if c[0].SessionStatus != domain.AgentBudgetExhausted || c[0].ToolCalls != 17 || c[2].SessionStatus != domain.AgentSubmitted || c[2].Disposition != domain.CoverExaminedNoConcern {
		t.Fatalf("per-item budgets: %+v", c)
	}
	if !strings.HasPrefix(c[0].Summary, "no closing entry: the item's session ended budget-exhausted") {
		t.Fatalf("exhausted item: %+v", c[0])
	}
}

func TestPerItemReplayIsByteIdentical(t *testing.T) {
	f := newFixture(t, true)
	out := runItems(t, f, v4Config(0), &Scripted{Model: "test-model", ModelVersion: "v", Tokens: 50, ByItem: itemScript(f)})
	rc, err := f.st.Open(context.Background(), out.Artifacts[0].Digest)
	must(t, err)
	original, _ := io.ReadAll(rc)
	rc.Close()
	tr, err := ParseTranscript(original)
	must(t, err)
	if len(tr.Segments) != 3 || tr.Segments[1].Item != f.envItem || tr.Segments[1].PromptDigest == "" {
		t.Fatalf("segments: %+v", tr.Segments)
	}
	rep := &Replay{Transcript: original}
	cfg := v4Config(0)
	cfg.Driver = DriverReplay
	again2, err := New(f.st, WithDriver(DriverReplay, rep), WithValidators(&fakeOverride{f: f}), WithClock(fixedNow)).Investigate(context.Background(), f.input(cfg))
	must(t, err)
	if len(rep.Mismatches) != 0 || again2.Run.Transcript != out.Run.Transcript {
		t.Fatalf("replay: %+v %s vs %s", rep.Mismatches, again2.Run.Transcript, out.Run.Transcript)
	}
	a, _ := json.Marshal(out.Run.Coverage)
	b, _ := json.Marshal(again2.Run.Coverage)
	if string(a) != string(b) || len(again2.Notes) != 2 {
		t.Fatalf("replayed coverage differs:\n%s\n%s", a, b)
	}
}
