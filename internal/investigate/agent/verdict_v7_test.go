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

const promptV7 = "investigate-agent/v7"

// v7 is v6 plus one behaviour (verdictInheritsCitations) and the tool-schema
// text that states it: system prompt, task and item templates are v6's
// byte for byte, and tools.json differs only in the two sentences.
func TestPromptV7(t *testing.T) {
	v6, err := LoadPrompt(promptV6)
	must(t, err)
	v7, err := LoadPrompt(promptV7)
	must(t, err)
	want := v6.Features
	want.VerdictInheritsCitations = true
	if v7.Features != want || v6.Features.VerdictInheritsCitations {
		t.Fatalf("features: v6 %+v, v7 %+v", v6.Features, v7.Features)
	}
	for _, name := range []string{"system.md", "task.md", "item.md"} {
		a, err := promptFS.ReadFile("prompts/" + promptV6 + "/" + name)
		must(t, err)
		b, err := promptFS.ReadFile("prompts/" + promptV7 + "/" + name)
		must(t, err)
		if !bytes.Equal(a, b) {
			t.Errorf("v7 %s differs from v6: no prompt change but the tool schema text", name)
		}
	}
	sentence := " A verdict that states no citations takes the entry's own citations, once they are checked."
	field := `
                    "description": "optional: when empty or omitted, the verdict takes this entry's checked citations (never unchecked ones); give your own to cite something else",`
	got := strings.Replace(strings.Replace(string(v7.ToolsJSON), sentence, "", 1), field, "", 1)
	if got != string(v6.ToolsJSON) {
		t.Error("v7 tools.json differs from v6 beyond the inheritance sentence and the verdict.citations description")
	}
	for _, word := range []string{"cert-manager", "rotationPolicy", "payments", "ingress", "prometheus", "Certificate"} {
		if strings.Contains(string(v7.ToolsJSON), word) {
			t.Errorf("the v7 tools name %q: prompts are generic", word)
		}
	}
	if v6.Digest("t") == v7.Digest("t") {
		t.Error("v6 and v7 share a prompt digest")
	}
	if _, err := LoadPrompt(promptV7); err != nil || DefaultPromptVersion != promptV7 {
		t.Fatalf("default %s: %v", DefaultPromptVersion, err)
	}
}

func v7Run(t *testing.T, f *widgetFixture, steps ...Step) (*Scripted, *investigate.AgentOutcome) {
	t.Helper()
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: steps}}
	cfg := v6Config()
	cfg.PromptVersion = promptV7
	out, err := New(f.st, WithDriver(DriverScripted, d), WithClock(fixedNow)).Investigate(context.Background(), f.input(cfg))
	must(t, err)
	return d, out
}

// The failure the live run hit (every first submit rejected because the
// verdict's citations were on the entry instead) is accepted under v7: the
// verdict takes the entry's checked citations, marked as such. It never takes
// an unchecked one, never replaces citations the verdict states, and v6 is
// unchanged.
func TestV7VerdictInheritsCheckedEntryCitations(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{})
	good := f.upstreamCite()
	bad := []map[string]any{{"artifact": f.notes.ID, "quote": "this sentence is not in the release notes"}}
	verdict := func(class string, cites []map[string]any) map[string]any {
		v := map[string]any{"class": class, "confidence": "medium", "mechanism": "the default flips", "environmentReasoning": "r", "wouldOverturn": "w"}
		if cites != nil {
			v["citations"] = cites
		}
		return v
	}
	entry := func(cites []map[string]any, v map[string]any) Step {
		return Step{Call: "call-7", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{
			"changeItem": f.item, "disposition": "explains", "summary": "s", "citations": cites, "verdict": v})}
	}

	// 1. no verdict citations, checked entry citations: accepted at once.
	d, out := v7Run(t, f, entry(good, verdict("not-affected", nil)))
	if err := d.Results[0].Err; err != nil {
		t.Fatalf("v7 inheritance rejected: %v", err)
	}
	if len(out.Assertions) != 1 || len(out.Assertions[0].Citations) == 0 {
		t.Fatalf("the entry's assertion: %+v", out.Assertions)
	}
	entryEv := out.Assertions[0].Citations // the entry's checked citations
	if len(out.Verdicts) != 1 || !out.Verdicts[0].CitationsFromEntry || !equalIDs(out.Verdicts[0].Citations, entryEv) || out.Verdicts[0].DroppedCitations != 0 {
		t.Fatalf("inherited verdict: %+v, want citations %v from the entry", out.Verdicts, entryEv)
	}
	// an empty list is "states none" too
	d, out = v7Run(t, f, entry(good, verdict("not-affected", []map[string]any{})))
	if d.Results[0].Err != nil || len(out.Verdicts) != 1 || !out.Verdicts[0].CitationsFromEntry {
		t.Fatalf("empty verdict.citations: %v %+v", d.Results[0].Err, out.Verdicts)
	}

	// 2. an unchecked entry citation is never inherited: the call is rejected.
	d, out = v7Run(t, f, entry(bad, verdict("not-affected", nil)))
	if err := d.Results[0].Err; !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), "does not occur in artifact") || len(out.Verdicts) != 0 {
		t.Fatalf("unchecked entry citation: %v %+v", err, out.Verdicts)
	}

	// 3. citations the verdict states are its own: kept, not replaced, and
	// if none of them resolves the verdict is still rejected (no fallback).
	other := []map[string]any{{"artifact": f.notes.ID, "quote": "Breaking changes"}}
	d, out = v7Run(t, f, entry(good, verdict("not-affected", other)))
	if d.Results[0].Err != nil || len(out.Verdicts) != 1 || out.Verdicts[0].CitationsFromEntry || equalIDs(out.Verdicts[0].Citations, entryEv) {
		t.Fatalf("own verdict citations: %v %+v", d.Results[0].Err, out.Verdicts)
	}
	d, _ = v7Run(t, f, entry(good, verdict("not-affected", bad)))
	if err := d.Results[0].Err; !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), "needs at least one citation that resolves") {
		t.Fatalf("unresolvable own verdict citations: %v", err)
	}

	// 4. v6 is unchanged: the same call is rejected as in the live run.
	d = &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: {entry(good, verdict("not-affected", nil))}}}
	widgetRun(t, f, d)
	if err := d.Results[0].Err; !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), "needs at least one citation that resolves") {
		t.Fatalf("v6 must still reject a verdict without citations: %v", err)
	}
}

// A v6 session shaped like the live run (first submit rejected for an empty
// verdict.citations, the second accepted) replays byte-identically under v6,
// and is refused under v7 (another prompt digest; v7 would accept the first
// submit): a transcript replays only under the version it was recorded with. A v7 session with an
// inherited verdict replays byte-identically under v7.
func TestV6AndV7TranscriptsReplayUnderTheirVersion(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{})
	good := f.upstreamCite()
	submit := func(v map[string]any) Step {
		return Step{Call: "call-r", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{
			"changeItem": f.item, "disposition": "explains", "summary": "s", "citations": good, "verdict": v})}
	}
	noCites := map[string]any{"class": "not-affected", "confidence": "medium", "mechanism": "m", "environmentReasoning": "r", "wouldOverturn": "w"}
	withCites := map[string]any{"class": "not-affected", "confidence": "medium", "mechanism": "m", "environmentReasoning": "r", "wouldOverturn": "w", "citations": good}
	record := func(version string, steps ...Step) (*investigate.AgentOutcome, []byte) {
		d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: steps}}
		cfg := v6Config()
		cfg.PromptVersion = version
		out, err := New(f.st, WithDriver(DriverScripted, d), WithClock(fixedNow)).Investigate(context.Background(), f.input(cfg))
		must(t, err)
		for _, a := range out.Artifacts {
			if a.ID == out.Run.Transcript {
				rc, err := f.st.Open(context.Background(), a.Digest)
				must(t, err)
				raw, _ := io.ReadAll(rc)
				rc.Close()
				return out, raw
			}
		}
		t.Fatal("no transcript artifact")
		return nil, nil
	}
	replay := func(version string, transcript []byte) (*Replay, *investigate.AgentOutcome) {
		rep := &Replay{Transcript: transcript}
		cfg := v6Config()
		cfg.PromptVersion, cfg.Driver = version, DriverReplay
		again, err := New(f.st, WithDriver(DriverReplay, rep), WithClock(fixedNow)).Investigate(context.Background(), f.input(cfg))
		must(t, err)
		return rep, again
	}
	same := func(a, b *investigate.AgentOutcome) bool {
		x, _ := json.Marshal(a.Verdicts)
		y, _ := json.Marshal(b.Verdicts)
		return a.Run.Transcript == b.Run.Transcript && a.Run.PromptDigest == b.Run.PromptDigest && bytes.Equal(x, y)
	}

	out6, t6 := record(promptV6, submit(noCites), submit(withCites))
	if out6.Run.ToolCalls < 2 || len(out6.Verdicts) != 1 || out6.Verdicts[0].CitationsFromEntry {
		t.Fatalf("v6 recording: %+v %+v", out6.Run, out6.Verdicts)
	}
	if rep, again := replay(promptV6, t6); len(rep.Mismatches) != 0 || !same(out6, again) {
		t.Fatalf("v6 transcript under v6: %+v", rep.Mismatches)
	}
	if _, again := replay(promptV7, t6); again.Run.Status == domain.AgentSubmitted || !strings.Contains(again.Run.Reason, "recorded under prompt") {
		t.Fatalf("a v6 transcript under v7 must be refused (another prompt digest): %s %s", again.Run.Status, again.Run.Reason)
	}

	out7, t7 := record(promptV7, submit(noCites))
	if len(out7.Verdicts) != 1 || !out7.Verdicts[0].CitationsFromEntry {
		t.Fatalf("v7 recording: %+v", out7.Verdicts)
	}
	if rep, again := replay(promptV7, t7); len(rep.Mismatches) != 0 || !same(out7, again) {
		t.Fatalf("v7 transcript under v7: %+v", rep.Mismatches)
	}
}

func equalIDs(a, b []domain.EvidenceID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
