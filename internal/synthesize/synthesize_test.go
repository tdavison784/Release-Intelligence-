package synthesize

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

func violations(t *testing.T, p *Precomputed, a Answer) []string {
	t.Helper()
	dec, err := decode(answerJSON(t, a))
	if err != nil {
		t.Fatal(err)
	}
	return Floor(p, dec)
}

func wantViolation(t *testing.T, vs []string, substr string) {
	t.Helper()
	for _, v := range vs {
		if strings.Contains(v, substr) {
			return
		}
	}
	t.Errorf("no violation mentioning %q in %q", substr, vs)
}

func TestFloor(t *testing.T) {
	p := precompute(t, newFixture().input())
	if vs := violations(t, p, validAnswer(t, p)); len(vs) > 0 {
		t.Fatalf("the valid answer is rejected: %q", vs)
	}
	mod := func(f func(*Answer)) Answer {
		a := validAnswer(t, p)
		f(&a)
		return a
	}
	t.Run("proceed with a hard finding", func(t *testing.T) {
		a := mod(func(a *Answer) { a.Recommendation, a.TargetVersion = domain.RecommendProceed, "1.1.0" })
		wantViolation(t, violations(t, p, a), "proceed is impossible while")
	})
	t.Run("target with a regression", func(t *testing.T) {
		a := mod(func(a *Answer) { a.Recommendation, a.TargetVersion = domain.RecommendProceedWithChanges, "v1.1.0" })
		wantViolation(t, violations(t, p, a), "a target with a regression cannot be recommended")
	})
	t.Run("version outside the candidates", func(t *testing.T) {
		a := mod(func(a *Answer) { a.TargetVersion, a.Headline = "1.2.0", "Modify target to 1.2.0." })
		wantViolation(t, violations(t, p, a), "is not one of the computed options")
	})
	t.Run("modify-target keeping the target", func(t *testing.T) {
		a := mod(func(a *Answer) { a.TargetVersion = "" })
		wantViolation(t, violations(t, p, a), "modify-target takes a computed option")
	})
	t.Run("headline without the version", func(t *testing.T) {
		a := mod(func(a *Answer) { a.Headline = "Modify the target." })
		wantViolation(t, violations(t, p, a), "headline names the target version")
	})
	t.Run("unresolvable references", func(t *testing.T) {
		a := mod(func(a *Answer) {
			a.FindingComments = append(a.FindingComments, AnswerComment{ID: "jf-000000000000", Comment: "x"})
			a.Validation[0].Basis = []string{"tc-unknown"}
			a.Verified[0].Citations = []string{"ev-invented"}
			a.ResidualRisks[0].Refs = []string{"nothing"}
			a.Changes[1].ChangeItems = []string{"chi-unknown"}
		})
		vs := violations(t, p, a)
		for _, s := range []string{"findingComments[1]", "validation[0] basis", "verified[0] citations", "residualRisks[0] refs", "changes[1] changeItems"} {
			wantViolation(t, vs, s)
		}
	})
	t.Run("invented location", func(t *testing.T) {
		a := mod(func(a *Answer) { a.Changes[0].Location = "values/prod.yaml:7" })
		wantViolation(t, violations(t, p, a), "changes[0] location")
	})
	t.Run("a location written as a field", func(t *testing.T) {
		raw := strings.Replace(answerJSON(t, validAnswer(t, p)), `"location":""`, `"location":"","file":"values/prod.yaml"`, 1)
		if _, err := decode(raw); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("decode = %v, want an unknown-field rejection", err)
		}
	})
	t.Run("rewritten findings", func(t *testing.T) {
		raw := strings.TrimSuffix(answerJSON(t, validAnswer(t, p)), "}") + `,"findings":[]}`
		if _, err := decode(raw); err == nil {
			t.Error("an answer carrying its own findings list was accepted")
		}
	})
	t.Run("verified without citations", func(t *testing.T) {
		a := mod(func(a *Answer) { a.Verified[0].Citations = nil })
		wantViolation(t, violations(t, p, a), "otherwise it is inferred")
	})
	t.Run("no validation", func(t *testing.T) {
		a := mod(func(a *Answer) { a.Validation = nil })
		wantViolation(t, violations(t, p, a), "validation lists no test")
	})
}

func cacheOver(t *testing.T, inner llm.Client) *llm.Cache {
	t.Helper()
	return &llm.Cache{Dir: t.TempDir(), Inner: inner, Clock: func() time.Time { return fixedNow }}
}

// A dropped disagreement is restored verbatim: the judgement's findings are
// the pre-computed ones whatever the answer says (it may only comment).
func TestFindingsAreCopiedVerbatim(t *testing.T) {
	f := newFixture()
	p := precompute(t, f.input())
	a := validAnswer(t, p)
	a.FindingComments = nil
	s := &Synthesizer{Client: cacheOver(t, &llm.Fake{Model: "m", Responses: []llm.FakeResponse{{Text: answerJSON(t, a)}}})}
	j, err := s.Synthesize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(j.Findings, p.Findings) || len(j.HardFindings()) == 0 {
		t.Errorf("findings are not the pre-computed ones")
	}
	if !reflect.DeepEqual(j.Interactions, p.Interactions) || !reflect.DeepEqual(j.Locations, p.Locations) || !reflect.DeepEqual(j.Backout.Version, p.Backout.Version) {
		t.Error("deterministic parts changed")
	}
}

// The call goes through llm.Cache over a scripted file exchange: the first
// run writes the request and is pending, the supplied answer completes it
// with complete provenance, and an offline replay is a cache hit with an
// identical judgement.
func TestCallThroughExchangeReplaysIdentically(t *testing.T) {
	f := newFixture()
	p := precompute(t, f.input())
	xdir, cdir := t.TempDir(), t.TempDir()
	s := &Synthesizer{Client: &llm.Cache{Dir: cdir, Inner: &llm.Exchange{Dir: xdir}}}
	if _, err := s.Synthesize(context.Background(), f.input()); !errors.Is(err, llm.ErrPending) {
		t.Fatalf("first call = %v, want pending", err)
	}
	reqs, _ := filepath.Glob(filepath.Join(xdir, "*.request.json"))
	if len(reqs) != 1 {
		t.Fatalf("%d exchange requests", len(reqs))
	}
	var xr llm.ExchangeRequest
	raw, _ := os.ReadFile(reqs[0])
	if err := json.Unmarshal(raw, &xr); err != nil {
		t.Fatal(err)
	}
	if xr.Request.Model != DefaultModel || len(xr.Request.Messages) != 1 || len(xr.Request.JSONSchema) == 0 {
		t.Errorf("request: model %q, %d messages, schema %d bytes", xr.Request.Model, len(xr.Request.Messages), len(xr.Request.JSONSchema))
	}
	resp := llm.ExchangeResponse{Format: llm.ExchangeResponseFormat, PromptDigest: xr.PromptDigest, Model: DefaultModel, ModelVersion: "claude-opus-5-5",
		GeneratedAt: fixedNow, CallID: "session-1", Output: json.RawMessage(answerJSON(t, validAnswer(t, p)))}
	b, _ := json.Marshal(resp)
	if err := os.WriteFile(strings.TrimSuffix(reqs[0], ".request.json")+".response.json", b, 0o644); err != nil {
		t.Fatal(err)
	}
	j, err := s.Synthesize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	pv := j.Provenance
	if j.Status != domain.JudgementComplete || pv.Model != DefaultModel || pv.ModelVersion == "" || pv.PromptVersion != PromptVersion ||
		!strings.HasPrefix(pv.PromptDigest, "sha256:") || len(pv.InputEvidence) == 0 || pv.GeneratedAt == nil || pv.CallID != "session-1" ||
		pv.Provider != "anthropic" || pv.Producer != Producer || j.StatementKind != domain.StatementUnverifiedProse || j.Input != p.Digest() {
		t.Errorf("provenance incomplete: %+v", pv)
	}
	if err := j.Validate(); err != nil {
		t.Error(err)
	}
	if !j.Target.Changed || j.Target.Chosen.Semver != "1.1.3" || j.Target.TransitionInvestigated {
		t.Errorf("target = %+v", j.Target)
	}
	var transition bool
	for _, x := range j.Validation {
		transition = transition || strings.Contains(x.Test, "v1.0.0 → v1.1.3")
	}
	if !transition {
		t.Error("a changed target does not require the new transition's investigation")
	}
	if c := j.Changes[0]; c.Proposed || c.Location == "" || !j.Changes[1].Proposed {
		t.Errorf("changes = %+v", j.Changes)
	}
	// replay offline: no exchange, the cache answers
	replay := &Synthesizer{Client: &llm.Cache{Dir: cdir}}
	j2, err := replay.Synthesize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(j)
	b2, _ := json.Marshal(j2)
	if string(a) != string(b2) {
		t.Error("the replayed judgement differs")
	}
}

// A violating answer is retried with its violations, within the bound; a
// failure is recorded with every rejected answer, never repaired.
func TestBoundedRetryAndRecordedFailure(t *testing.T) {
	f := newFixture()
	p := precompute(t, f.input())
	bad := validAnswer(t, p)
	bad.Recommendation, bad.TargetVersion = domain.RecommendProceed, "1.1.0"
	good := validAnswer(t, p)
	fake := &llm.Fake{Model: "m", Responses: []llm.FakeResponse{{Text: answerJSON(t, bad)}, {Text: "not json"}, {Text: answerJSON(t, good)}}}
	j, err := (&Synthesizer{Client: cacheOver(t, fake)}).Synthesize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	reqs := fake.Requests()
	if j.Status != domain.JudgementComplete || len(reqs) != 3 {
		t.Fatalf("status %s after %d calls", j.Status, len(reqs))
	}
	retry := reqs[1].Messages[0].Content
	if len(reqs[1].Messages) != 1 || !strings.Contains(retry, "proceed is impossible") || !strings.Contains(retry, `"recommendation":"proceed"`) {
		t.Errorf("the retry does not carry the violations and the rejected answer")
	}
	if !strings.Contains(reqs[2].Messages[0].Content, "not the schema's JSON document") {
		t.Error("the third attempt does not carry the decode violation")
	}

	fake = &llm.Fake{Model: "m", Respond: func(llm.Request) (string, error) { return answerJSON(t, bad), nil }}
	j, err = (&Synthesizer{Client: cacheOver(t, fake)}).Synthesize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	if j.Status != domain.JudgementFailed || j.Recommendation != "" || j.Target.Chosen != nil || len(fake.Requests()) != MaxAttempts {
		t.Fatalf("status %s, recommendation %q after %d calls", j.Status, j.Recommendation, len(fake.Requests()))
	}
	if j.Failure.Attempts != MaxAttempts || len(j.Failure.Answers) != MaxAttempts || len(j.Failure.Violations) == 0 || len(j.Findings) != len(p.Findings) {
		t.Errorf("failure = %+v", j.Failure)
	}
	if err := j.Validate(); err != nil {
		t.Error(err)
	}
}

// The prompt is versioned with a digest and names no product, case, file or
// field.
func TestPromptIsGenericAndVersioned(t *testing.T) {
	pr, err := LoadPrompt(PromptVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pr.Digest(), "sha256:") || !json.Valid(pr.Schema) {
		t.Error("prompt digest or schema invalid")
	}
	text := strings.ToLower(pr.System + pr.Task + string(pr.Schema))
	for _, w := range []string{"cert-manager", "ingress", "nginx", "payments", "docs-eu", "ca.crt", "ledger", "isca", "strict", "pathtype",
		"1.12", "1.11", "1.20", "1.19", "chart 4", "ghsa-", "cve-", "secret", "configmap", "deployment", "argo", "flux", "helm", "values.yaml"} {
		if strings.Contains(text, w) {
			t.Errorf("the prompt names %q", w)
		}
	}
	if _, err := LoadPrompt("synthesize/v0"); err == nil {
		t.Error("an unknown prompt version loaded")
	}
}

// RenderJudgement's text is golden (testdata/judgement.golden; regenerate
// with RI_UPDATE_GOLDEN=1).
func TestRenderGolden(t *testing.T) {
	f := newFixture()
	p := precompute(t, f.input())
	s := &Synthesizer{Client: cacheOver(t, &llm.Fake{Model: "claude-opus-5-5", Responses: []llm.FakeResponse{{Text: answerJSON(t, validAnswer(t, p))}}})}
	j, err := s.Synthesize(context.Background(), f.input())
	if err != nil {
		t.Fatal(err)
	}
	f.Inv.Judgement = j
	var buf strings.Builder
	if err := RenderJudgement(&buf, f.Inv, f.Bundle); err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "judgement.golden")
	if os.Getenv("RI_UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(buf.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != string(want) {
		t.Errorf("rendered judgement differs from %s:\n%s", golden, buf.String())
	}
	var empty strings.Builder
	f.Inv.Judgement = nil
	if err := RenderJudgement(&empty, f.Inv, f.Bundle); err != nil || empty.Len() != 0 {
		t.Error("a run without a judgement renders something")
	}
}
