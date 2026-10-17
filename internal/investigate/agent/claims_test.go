package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
)

const promptV5 = "investigate-agent/v5"

func (f *fixture) v5session(t *testing.T) *Session {
	t.Helper()
	s, _, err := Prepare(f.st, f.input(investigate.AgentConfig{Enabled: true, Driver: DriverScripted, Model: "test-model", PromptVersion: promptV5}), nil, fixedNow)
	must(t, err)
	s.BeginItem(f.behaviour, ItemDefaultBudget, domain.Digest([]byte("v5 test session"))) // a per-item prompt: digests are per session
	s.SetCall(CallInfo{ID: "call-1", Model: "test-model", ModelVersion: "v", Provider: "test"})
	return s
}

func TestClaimsVerifiedOrRejectedWithTheContradiction(t *testing.T) {
	f := newFixture(t)
	s := f.v5session(t)
	claim := func(kind, setting, input, value string) map[string]any {
		m := map[string]any{"kind": kind, "setting": setting, "input": input}
		if value != "" {
			m["value"] = value
		}
		return m
	}
	for _, c := range []struct {
		args       map[string]any
		wantDetail string
	}{
		{claim("absent", "controller.unsetKey", ClaimInputEnvironment, ""), "occurs in none of 3 artifact(s)"},
		{claim("present", "controller.replicas", "env/apps/demo/values.yaml", ""), "present"},
		{claim("value", "controller.rotationPolicy", "apps/demo/values.yaml", `"Never"`), `has the value "Never"`},
		{claim("value", "controller.replicas", f.values.ID, "2"), "has the value 2"},
		{claim("present", "--rotation-policy=Never", f.toRender.ID, ""), "present"},
		{claim("absent", "--rotation-policy", ClaimInputEnvironment, ""), "occurs in none"},
	} {
		r := ok(t, s, investigate.ToolRecordClaim, c.args)
		if r["verified"] != true || !strings.Contains(r["detail"].(string), c.wantDetail) {
			t.Errorf("%v: %v", c.args, r)
		}
	}
	for _, c := range []struct {
		args map[string]any
		want string
	}{
		// The live-run-4 failure mode: "the environment does not set X" while it does.
		{claim("absent", "controller.rotationPolicy", ClaimInputEnvironment, ""), `L2: "rotationPolicy: Never"`},
		// A wrong or partial path is still caught: the last key occurs.
		{claim("absent", "spec.rotationPolicy", ClaimInputEnvironment, ""), `L2: "rotationPolicy: Never"`},
		{claim("absent", "rotationPolicy", f.values.ID, ""), "occurs in the input"},
		{claim("value", "controller.rotationPolicy", ClaimInputEnvironment, "Always"), `has another value — ` + f.values.ID},
		{claim("present", "controller.unsetKey", ClaimInputEnvironment, ""), "occurs nowhere"},
		{claim("value", "--rotation-policy=Never", f.toRender.ID, "x"), "a value claim needs a YAML key path"},
		{claim("absent", "x.y", f.notes.ID, ""), "not an environment file or an environment render"},
		{claim("absent", "x.y", "art-nope", ""), "unknown input"},
		{claim("value", "x.y", ClaimInputEnvironment, ""), "exactly for kind value"},
		{claim("missing", "x.y", ClaimInputEnvironment, ""), "want absent, present or value"},
	} {
		if _, err := call(t, s, investigate.ToolRecordClaim, c.args); !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: got %v, want a rejection mentioning %q", c.args, err, c.want)
		}
	}
	out, err := s.Outcome(context.Background(), f.st, DriverScripted, investigate.DriverResult{Model: "test-model", ModelVersion: "v", Stop: "error:x"})
	must(t, err)
	if len(out.Claims) != 6 {
		t.Fatalf("claims recorded: %d", len(out.Claims))
	}
	for _, c := range out.Claims {
		if c.Verifier != ClaimVerifier || len(c.Checked) == 0 || (c.Kind != domain.ClaimAbsent && len(c.Evidence) == 0) || c.Provenance.Method != domain.MethodAI {
			t.Fatalf("claim: %+v", c)
		}
	}
	if err := runRecord(t, f, out).ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
}

func TestClaimsBackRecordsAndProseIsLabelled(t *testing.T) {
	f := newFixture(t)
	s := f.v5session(t)
	id := ok(t, s, investigate.ToolRecordClaim, map[string]any{"changeItem": f.behaviour, "kind": "value", "setting": "controller.rotationPolicy",
		"input": ClaimInputEnvironment, "value": "Never"})["id"].(string)
	other := ok(t, s, investigate.ToolRecordClaim, map[string]any{"changeItem": f.values2, "kind": "absent", "setting": "legacy.enabled", "input": ClaimInputEnvironment})["id"].(string)
	ok(t, s, investigate.ToolGetChangeItem, map[string]any{"id": f.behaviour})
	cite := []map[string]any{{"artifact": f.values.ID, "quote": "rotationPolicy: Never"}}
	ok(t, s, investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "claims-mitigation", "statement": "pinned to Never",
		"citations": cite, "claims": []string{id}})
	rejected(t, s, investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "explains", "statement": "x", "citations": cite,
		"claims": []string{other}}, "is about change item")
	rejected(t, s, investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "explains", "statement": "x", "citations": cite,
		"claims": []string{"clm-nope"}}, "unknown claim")
	out, err := s.Outcome(context.Background(), f.st, DriverScripted, investigate.DriverResult{Model: "test-model", ModelVersion: "v", Stop: "error:x"})
	must(t, err)
	a := out.Assertions[0]
	if a.StatementKind != domain.StatementUnverifiedProse || len(a.Claims) != 1 || a.Claims[0] != id {
		t.Fatalf("assertion: %+v", a)
	}
	inv := runRecord(t, f, out)
	if err := inv.ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
	inv.AgentClaims = nil
	if err := inv.Validate(); err == nil || !strings.Contains(err.Error(), "unknown setting claim") {
		t.Fatalf("an assertion naming a missing claim validates: %v", err)
	}
	// v4 has no claims and labels nothing.
	s4, _, err := Prepare(f.st, f.input(investigate.AgentConfig{Enabled: true, PromptVersion: promptV4}), nil, fixedNow)
	must(t, err)
	s4.SetCall(CallInfo{ID: "c"})
	rejected(t, s4, investigate.ToolRecordClaim, map[string]any{"kind": "absent", "setting": "x.y", "input": ClaimInputEnvironment}, "unknown tool")
}

func TestV5PerItemWithClaimsReplays(t *testing.T) {
	f := newFixture(t, true)
	values := []map[string]any{{"artifact": f.values.ID, "quote": "rotationPolicy: Never"}}
	notesCite := []map[string]any{{"artifact": f.notes.ID, "quote": "The values key `legacy.enabled` was removed."}}
	script := map[string][]Step{
		f.behaviour: {
			{Call: "a", Tool: investigate.ToolGetChangeItem, Args: map[string]any{"id": f.behaviour}},
			{Call: "a", Tool: investigate.ToolReadArtifact, Args: map[string]any{"artifact": f.values.ID}},
			{Call: "a", Tool: investigate.ToolRecordClaim, Args: map[string]any{"changeItem": f.behaviour, "kind": "value", "setting": "controller.rotationPolicy", "input": ClaimInputEnvironment, "value": "Never"}},
			{Call: "a", Tool: investigate.ToolRecordClaim, Args: map[string]any{"changeItem": f.behaviour, "kind": "absent", "setting": "controller.rotationPolicy", "input": ClaimInputEnvironment}},
			{Call: "a", Tool: investigate.ToolRequestValidation, Args: map[string]any{"changeItem": f.behaviour, "kind": "override-inspection"}},
			{Call: "a", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.behaviour, "disposition": "claims-mitigation",
				"summary": "pinned", "citations": values, "claims": []string{domain.SettingClaimID(f.behaviour, domain.ClaimValue, "controller.rotationPolicy", ClaimInputEnvironment, `"Never"`, "a")}})},
		},
		f.envItem: {{Call: "b", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.envItem, "disposition": "not-examined", "summary": "later"})}},
		f.values2: {
			{Call: "c", Tool: investigate.ToolGetChangeItem, Args: map[string]any{"id": f.values2}},
			{Call: "c", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.values2, "disposition": "examined-no-concern", "summary": "unset", "citations": notesCite})},
		},
	}
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: script}
	cfg := investigate.AgentConfig{Enabled: true, Driver: DriverScripted, Model: "test-model", PromptVersion: promptV5}
	out := runItems(t, f, cfg, d)
	if !errors.Is(d.Results[3].Err, investigate.ErrRejected) { // the false absence claim
		t.Fatalf("absence claim of a set key: %v", d.Results[3].Err)
	}
	if out.Run.Status != domain.AgentSubmitted || len(out.Claims) != 1 || out.Run.Coverage[0].SummaryKind != domain.StatementUnverifiedProse || len(out.Assertions[0].Claims) != 1 {
		t.Fatalf("v5 run: %+v claims %d", out.Run, len(out.Claims))
	}
	if err := runRecord(t, f, out).ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
	rc, err := f.st.Open(context.Background(), out.Artifacts[0].Digest)
	must(t, err)
	original, _ := io.ReadAll(rc)
	rc.Close()
	rep := &Replay{Transcript: original}
	cfg.Driver = DriverReplay
	again, err := New(f.st, WithDriver(DriverReplay, rep), WithValidators(&fakeOverride{f: f}), WithClock(fixedNow)).Investigate(context.Background(), f.input(cfg))
	must(t, err)
	if len(rep.Mismatches) != 0 || again.Run.Transcript != out.Run.Transcript || len(again.Claims) != 1 {
		t.Fatalf("v5 replay: %+v", rep.Mismatches)
	}
}

// The default prompt is the newest embedded version, so a runner that
// passes agent.DefaultPromptVersion (or none) gets it.
func TestDefaultPromptIsTheNewest(t *testing.T) {
	entries, err := promptFS.ReadDir("prompts/investigate-agent")
	must(t, err)
	newest := 0
	for _, e := range entries {
		var n int
		if _, err := fmt.Sscanf(e.Name(), "v%d", &n); err == nil && n > newest {
			newest = n
		}
	}
	if want := fmt.Sprintf("investigate-agent/v%d", newest); DefaultPromptVersion != want {
		t.Fatalf("DefaultPromptVersion = %s, newest embedded = %s", DefaultPromptVersion, want)
	}
	p, err := LoadPrompt("")
	must(t, err)
	if p.Version != DefaultPromptVersion {
		t.Fatalf("LoadPrompt(\"\") = %s", p.Version)
	}
}

// The rendered text labels agent prose and shows verified claims and
// investigation-level notes (commander decision: prose is labelled in
// records and rendered text).
func TestRenderedTextLabelsProseAndShowsClaims(t *testing.T) {
	f := newFixture(t, true)
	values := []map[string]any{{"artifact": f.values.ID, "quote": "rotationPolicy: Never"}}
	claimID := domain.SettingClaimID(f.behaviour, domain.ClaimValue, "controller.rotationPolicy", ClaimInputEnvironment, `"Never"`, "a")
	script := map[string][]Step{
		f.behaviour: {
			{Call: "a", Tool: investigate.ToolGetChangeItem, Args: map[string]any{"id": f.behaviour}},
			{Call: "a", Tool: investigate.ToolReadArtifact, Args: map[string]any{"artifact": f.values.ID}},
			{Call: "a", Tool: investigate.ToolRecordClaim, Args: map[string]any{"changeItem": f.behaviour, "kind": "value", "setting": "controller.rotationPolicy", "input": ClaimInputEnvironment, "value": "Never"}},
			{Call: "a", Tool: investigate.ToolRequestValidation, Args: map[string]any{"changeItem": f.behaviour, "kind": "override-inspection"}},
			{Call: "a", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.behaviour, "disposition": "raises-concern",
				"summary": "the default flips", "citations": values, "claims": []string{claimID}})},
		},
		f.envItem: {{Call: "b", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.envItem, "disposition": "raises-concern",
			"summary": "unexplained mode change", "citations": values})}}, // no item evidence → note
	}
	out := runItems(t, f, investigate.AgentConfig{Enabled: true, Driver: DriverScripted, Model: "test-model", PromptVersion: promptV5, MaxItems: 2},
		&Scripted{Model: "test-model", ModelVersion: "v", ByItem: script})
	inv := runRecord(t, f, out)
	c, err := classify.New().Classify(context.Background(), investigate.ClassifyInput{Bundle: f.b, Traversal: f.trav, Agent: out, Validations: out.Validations,
		Config: investigate.Config{Policy: classify.Policy}})
	must(t, err)
	inv.Findings = c.Findings
	var buf strings.Builder
	must(t, classify.WriteText(&buf, inv, f.b))
	text := buf.String()
	for _, want := range []string{"unverified prose", "rests on verified claims: " + claimID, "Agent environment claims", "✓ " + claimID + " value controller.rotationPolicy",
		"Agent notes (AI, investigation-level", out.Notes[0].ID} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered text lacks %q:\n%s", want, text)
		}
	}
}
