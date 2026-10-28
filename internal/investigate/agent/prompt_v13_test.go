package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
)

const promptV13 = "investigate-agent/v13"

// v13 (docs/phase6/decision-intel/briefs/upstream3.md, ledger D-0014) is v12
// plus feature fetchFirst: the not-checked check on submit_findings, the
// optional reason of a not-checked behaviour, and one sentence on the order
// (fetch first, then conclude). Everything else is v12's; the default stays
// v9.
func TestPromptV13(t *testing.T) {
	v12, err := LoadPrompt(promptV12)
	must(t, err)
	v13, err := LoadPrompt(promptV13)
	must(t, err)
	want := v12.Features
	want.FetchFirst = true
	if v13.Features != want || v12.Features.FetchFirst {
		t.Fatalf("features: v12 %+v, v13 %+v", v12.Features, v13.Features)
	}
	if len(v13.Tools) != len(v12.Tools) {
		t.Fatalf("tools: v12 %d, v13 %d", len(v12.Tools), len(v13.Tools))
	}
	for i, x := range v13.Tools {
		y := v12.Tools[i]
		same := x.Name == y.Name && x.Description == y.Description && bytes.Equal(x.InputSchema, y.InputSchema)
		if x.Name != investigate.ToolSubmitFindings && !same {
			t.Errorf("v13 changes tool %s: only submit_findings gains the reason", x.Name)
		}
		if x.Name != investigate.ToolSubmitFindings {
			continue
		}
		if !strings.Contains(string(x.InputSchema), `"reason": {`) ||
			!strings.Contains(string(x.InputSchema), `"description": "source not-checked: why the upstream source cannot answer this behaviour`) {
			t.Error("v13 submit_findings schema lacks the behaviour reason")
		}
		const sentence = " Fetch first: when upstream fetching is available for a behavioural item, a not-checked behaviour is rejected once while the item made no request_evidence request (request it, or give reason), and every time after a request (cite it with source upstream, or name the request recorded unavailable)."
		if strings.Replace(x.Description, sentence, "", 1) != y.Description {
			t.Error("v13 submit_findings description is v12's plus the fetch-first sentence only")
		}
	}
	for _, f := range []string{"task.md", "item.md"} {
		a, _ := promptFS.ReadFile("prompts/" + promptV12 + "/" + f)
		b, _ := promptFS.ReadFile("prompts/" + promptV13 + "/" + f)
		if !bytes.Equal(a, b) {
			t.Errorf("v13 %s differs from v12's", f)
		}
	}
	if string(v13.MetaJSON) != strings.TrimSuffix(string(v12.MetaJSON), "\n}\n")+",\n  \"fetchFirst\": true\n}\n" {
		t.Errorf("v13 meta.json is v12's plus fetchFirst only:\n%s", v13.MetaJSON)
	}
	s12, s13 := systemSections(v12.System), systemSections(v13.System)
	const section = "Behaviour the verdict relies on"
	for title, body := range s12 {
		if title != section && s13[title] != body {
			t.Errorf("v13 changes section %q", title)
		}
	}
	const order = "\nWhen upstream fetching is available, fetch first, then conclude: a behaviour left `not-checked` while the\n" +
		"item made no `request_evidence` request is rejected once (request it, or give in `reason` why the upstream\n" +
		"source cannot answer it), and after a request each behaviour is `upstream` or `unavailable`.\n"
	if len(s13) != len(s12) || strings.Replace(s13[section], order, "", 1) != s12[section] {
		t.Error("v13's system prompt is v12's plus the sentence on the order only")
	}
	if DefaultPromptVersion != promptV13 {
		t.Fatalf("default %s: v13 is the default since the M2 gate", DefaultPromptVersion)
	}
	if v12.Digest("t") == v13.Digest("t") {
		t.Error("v12 and v13 share a prompt digest")
	}
	broken := *v13
	broken.Features.Behaviour = false
	if err := broken.checkSurface(); err == nil || !strings.Contains(err.Error(), "fetchFirst needs feature behaviour") {
		t.Fatalf("fetchFirst without behaviour: %v", err)
	}
}

func TestPromptV13IsGeneric(t *testing.T) {
	assertGenericPrompt(t, promptV13, "nginx", "ledger", "ca.crt", "tls.crt", "tls.key", "trust bundle", "storefront", "catalog",
		"pathType", "ImplementationSpecific", "regex", "CVE", "GHSA", "strict")
}

// --- fixtures -------------------------------------------------------------------------

func v13Config() investigate.AgentConfig {
	cfg := v9Config()
	cfg.PromptVersion = promptV13
	return cfg
}

// v13Run runs the fixture's item under v13 with the scripted steps.
func v13Run(t *testing.T, f *consumerFixture, up *Upstream, steps ...Step) (*Scripted, *investigate.AgentOutcome) {
	t.Helper()
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: steps}}
	opts := []Option{WithDriver(DriverScripted, d), WithClock(fixedNow)}
	if up != nil {
		opts = append(opts, WithUpstream(*up))
	}
	out, err := New(f.st, opts...).Investigate(context.Background(), f.input(v13Config()))
	must(t, err)
	return d, out
}

func newPort() *Upstream {
	up := &fakeUpstream{src: upstreamSource}
	port := up.port()
	return &port
}

func notCheckedBecause(reason string) map[string]any {
	b := notChecked()
	b["reason"] = reason
	return b
}

const (
	noFetchMsg   = "Upstream fetching is available for example/keystore at v2.1.0 (target) and v2.0.0 (current): request the behaviour, or state in the entry why upstream source cannot answer it."
	afterFetch   = "so each behaviour is cited or reported unavailable; 1 are not-checked"
	notChecked0  = `behaviour[0] "whether renewal keeps the key when spec.rotate is unset"`
	notCheckedGp = `investigation gap (recorded by the tool layer): behaviour "whether renewal keeps the key when spec.rotate is unset" at v2.1.0: not-checked`
)

// --- checks ---------------------------------------------------------------------------

// With a port and no upstream request for the item, a not-checked behaviour
// is rejected once, whatever the verdict, naming the repositories and tags;
// the resubmission is accepted and the gap recorded.
func TestV13NotCheckedWithoutAttemptRejectedOnce(t *testing.T) {
	for _, class := range []string{"potentially-affected", "insufficient-evidence", "affected"} {
		f := newConsumerFixture(t)
		d, out := v13Run(t, f, newPort(), traceStep(f),
			f.v9Submit(class, f.withBehaviour(notChecked())), f.v9Submit(class, f.withBehaviour(notChecked())))
		err := submitErr(d, 1)
		if err == nil || !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), noFetchMsg) ||
			!strings.Contains(err.Error(), "1 behaviours are not-checked and this session made no upstream request for item "+f.item+": "+notChecked0) ||
			!strings.Contains(err.Error(), "A resubmission is accepted") {
			t.Fatalf("%s, first submission: %v", class, err)
		}
		if err := submitErr(d, 2); err != nil || out.Run.Status != domain.AgentSubmitted {
			t.Fatalf("%s, resubmission: %v %s", class, err, out.Run.Status)
		}
		a := out.Assessments[0]
		if len(a.Behaviour) != 1 || a.Behaviour[0].Reason != "" || a.Unknown[len(a.Unknown)-1] != notCheckedGp {
			t.Fatalf("%s: assessment %+v\n%v", class, a.Behaviour, a.Unknown)
		}
	}
	// not-affected: the fetch-first check speaks first; v12's verdict check
	// still rejects every time after it, and the gap's class is accepted.
	f := newConsumerFixture(t)
	d, out := v13Run(t, f, newPort(), traceStep(f),
		f.v9Submit("not-affected", f.withBehaviour(notChecked())), f.v9Submit("not-affected", f.withBehaviour(notChecked())),
		f.v9Submit("not-affected", f.withBehaviour(notCheckedBecause("r"))), f.v9Submit("potentially-affected", f.withBehaviour(notChecked())))
	for i, want := range []string{noFetchMsg, "cannot rest on upstream behaviour that is not established", "cannot rest on upstream behaviour that is not established"} {
		if err := submitErr(d, 1+i); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("not-affected, submission %d: %v, want %q", 1+i, err, want)
		}
	}
	if err := submitErr(d, 4); err != nil || out.Run.Status != domain.AgentSubmitted {
		t.Fatalf("potentially-affected after the once-check: %v", err)
	}
}

// The resubmission may state why the upstream source cannot answer the
// behaviour; it is accepted and recorded with the gap. A reason is taken
// only on a not-checked entry.
func TestV13ReasonAcceptedOnResubmission(t *testing.T) {
	f := newConsumerFixture(t)
	why := "the behaviour is the cluster's API server admission, not the product's source"
	other := upstreamBehaviour()
	other["reason"] = "x"
	d, out := v13Run(t, f, newPort(), traceStep(f),
		f.v9Submit("potentially-affected", f.withBehaviour(notCheckedBecause(why))),
		f.v9Submit("potentially-affected", f.withBehaviour(other)),
		f.v9Submit("potentially-affected", f.withBehaviour(notCheckedBecause(why))))
	if err := submitErr(d, 1); err == nil || !strings.Contains(err.Error(), noFetchMsg) {
		t.Fatalf("a reason does not skip the once-check: %v", err)
	}
	if err := submitErr(d, 2); err == nil || !strings.Contains(err.Error(), "behaviour[0]: only source not-checked takes reason") {
		t.Fatalf("reason on source upstream: %v", err)
	}
	if err := submitErr(d, 3); err != nil {
		t.Fatal(err)
	}
	a := out.Assessments[0]
	if len(a.Behaviour) != 1 || a.Behaviour[0].Reason != why || a.Unknown[len(a.Unknown)-1] != notCheckedGp+" (reason: "+why+")" {
		t.Fatalf("assessment %+v\n%v", a.Behaviour, a.Unknown)
	}
	inv := widgetLikeRecord(t, f, out)
	must(t, inv.Validate())
	must(t, inv.ValidateAgainst(f.b))
	inv.AgentAssessments[0].Behaviour[0].Source = domain.BehaviourUnavailable
	inv.AgentAssessments[0].Behaviour[0].Request = "erq-000000000000"
	if err := inv.AgentAssessments[0].Validate(); err == nil || !strings.Contains(err.Error(), "only source not-checked takes a reason") {
		t.Fatalf("record with a reason on another source: %v", err)
	}
}

// After an upstream request for the item, a not-checked behaviour is
// rejected every time, a reason included; the entry is upstream (cited) or
// unavailable (the request id), under v12's rules.
func TestV13AfterAttemptUpstreamOrUnavailable(t *testing.T) {
	f := newConsumerFixture(t)
	d, out := v13Run(t, f, newPort(), traceStep(f), fileStep(f, "internal/validate/name.go"),
		f.v9Submit("potentially-affected", f.withBehaviour(notChecked())),
		f.v9Submit("potentially-affected", f.withBehaviour(notCheckedBecause("r"))),
		f.v9Submit("not-affected", f.withBehaviour(upstreamBehaviour())))
	for _, i := range []int{2, 3} {
		err := submitErr(d, i)
		if err == nil || !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), afterFetch+": "+notChecked0) ||
			!strings.Contains(err.Error(), "this session requested upstream evidence for item "+f.item+" (erq-") || !strings.Contains(err.Error(), "(fulfilled)") {
			t.Fatalf("submission %d: %v", i, err)
		}
	}
	if err := submitErr(d, 4); err != nil || out.Run.Status != domain.AgentSubmitted {
		t.Fatalf("cited: %v", err)
	}
	if b := out.Assessments[0].Behaviour; len(b) != 1 || b[0].Source != domain.BehaviourUpstream {
		t.Fatalf("behaviour %+v", b)
	}

	// An unavailable request is an attempt too; the entry names it.
	f2 := newConsumerFixture(t)
	d2, out2 := v13Run(t, f2, newPort(), traceStep(f2), fileStep(f2, "internal/validate/missing.go"),
		f2.v9Submit("potentially-affected", f2.withBehaviour(notChecked())))
	if err := submitErr(d2, 2); err == nil || !strings.Contains(err.Error(), afterFetch) || !strings.Contains(err.Error(), "(unavailable)") {
		t.Fatalf("not-checked after an unavailable request: %v", err)
	}
	missing := out2.EvidenceRequests[0]
	f3 := newConsumerFixture(t)
	d3, out3 := v13Run(t, f3, newPort(), traceStep(f3), fileStep(f3, "internal/validate/missing.go"),
		f3.v9Submit("potentially-affected", f3.withBehaviour(map[string]any{"behaviour": "what the validation accepts", "source": "unavailable", "request": missing.ID})))
	if err := submitErr(d3, 2); err != nil || len(out3.Assessments[0].Behaviour) != 1 || out3.Assessments[0].Behaviour[0].Request != missing.ID {
		t.Fatalf("unavailable after the request: %v", err)
	}
}

// Without a port (not configured), with a port that knows no repository for
// the product, and on a non-behavioural item, not-checked is accepted at once.
func TestV13NotCheckedAcceptedWhenTheCheckDoesNotApply(t *testing.T) {
	f := newConsumerFixture(t)
	d, out := v13Run(t, f, nil, traceStep(f), f.v9Submit("potentially-affected", f.withBehaviour(notChecked())))
	if err := submitErr(d, 1); err != nil || out.Assessments[0].Unknown[len(out.Assessments[0].Unknown)-1] != notCheckedGp {
		t.Fatalf("no port: %v", err)
	}
	f2 := newConsumerFixture(t)
	unknown := &Upstream{Files: &fakeUpstream{}, Dirs: &fakeUpstream{}, Product: func(domain.ProductID) ([]string, func(string) string, bool) { return nil, nil, false }}
	if d2, _ := v13Run(t, f2, unknown, traceStep(f2), f2.v9Submit("potentially-affected", f2.withBehaviour(notChecked()))); submitErr(d2, 1) != nil {
		t.Fatalf("no repository: %v", submitErr(d2, 1))
	}
	f3 := newConsumerFixture(t)
	it := &f3.b.Change.Items[0]
	it.Family, it.Manifestation, it.ManifestationRule = domain.FamilyValues, domain.ManifestRenderedConfig, "manifest:computed-values"
	if d3, _ := v13Run(t, f3, newPort(), traceStep(f3), f3.v9Submit("potentially-affected", f3.withBehaviour(notChecked()))); submitErr(d3, 1) != nil {
		t.Fatalf("non-behavioural item: %v", submitErr(d3, 1))
	}
}

// throughBudget is a driver that keeps calling after the budget is spent, as
// a model may: the closing submit_findings is accepted once then.
type throughBudget struct {
	steps   []Step
	results []StepResult
}

func (d *throughBudget) Run(ctx context.Context, _ investigate.AgentSession, tools investigate.Tools) (*investigate.DriverResult, error) {
	s, err := asSession(tools)
	if err != nil {
		return nil, err
	}
	dr := &investigate.DriverResult{Model: "test-model", ModelVersion: "v", Provider: "scripted", CallID: "call-9"}
	for _, st := range d.steps {
		s.SetCall(CallInfo{ID: "call-9", Model: "test-model", ModelVersion: "v", Provider: "scripted"})
		args, err := json.Marshal(st.Args)
		if err != nil {
			return nil, err
		}
		res, err := s.Dispatch(ctx, st.Tool, args)
		d.results = append(d.results, StepResult{Tool: st.Tool, Result: res, Err: err})
	}
	dr.Stop = s.Exhausted()
	s.RecordEnvelope(DriverScripted, *dr, nil)
	return dr, nil
}

// Once the session's budget is spent no request can be made, so the check
// does not apply to the closing submission.
func TestV13SpentBudgetAcceptsNotChecked(t *testing.T) {
	f := newConsumerFixture(t)
	d := &throughBudget{steps: []Step{traceStep(f), traceStep(f), traceStep(f), f.v9Submit("potentially-affected", f.withBehaviour(notChecked()))}}
	cfg := v13Config()
	cfg.ItemBudget = &domain.AgentBudget{MaxTurns: 40, MaxToolCalls: 2, MaxTokens: 600_000, MaxReadBytes: 1 << 20, MaxSeconds: 1200}
	out, err := New(f.st, WithDriver(DriverScripted, d), WithClock(fixedNow), WithUpstream(*newPort())).Investigate(context.Background(), f.input(cfg))
	must(t, err)
	if err := d.results[2].Err; !errors.Is(err, investigate.ErrBudgetExhausted) {
		t.Fatalf("third call: %v", err)
	}
	if err := d.results[3].Err; err != nil || len(out.Assessments) != 1 || out.Assessments[0].Unknown[len(out.Assessments[0].Unknown)-1] != notCheckedGp {
		t.Fatalf("closing submission after the budget: %v", err)
	}
}

// Prompts without feature fetchFirst refuse a behaviour's reason exactly as
// the strict decoder did before the field existed, and never apply the check.
func TestEarlierPromptsRefuseBehaviourReason(t *testing.T) {
	for _, key := range []string{"reason", "Reason"} {
		f := newConsumerFixture(t)
		b := notChecked()
		b[key] = "x"
		d, out := v12Run(t, f, newPort(), traceStep(f), f.v9Submit("potentially-affected", f.withBehaviour(b)),
			f.v9Submit("potentially-affected", f.withBehaviour(notChecked())))
		if err := submitErr(d, 1); err == nil || err.Error() != `investigate: rejected: invalid arguments for submit_findings: json: unknown field "`+key+`"` {
			t.Fatalf("v12 with %s: %v", key, err)
		}
		// v12 with a port and no request accepts not-checked at once.
		if err := submitErr(d, 2); err != nil || out.Run.Status != domain.AgentSubmitted {
			t.Fatalf("v12 not-checked: %v", err)
		}
		transcript := transcriptOf(t, f, out)
		if !bytes.Contains(transcript, []byte(`"`+key+`":"x"`)) {
			t.Fatal("the refused call's arguments are not in the transcript as sent")
		}
	}
	// The same entry is refused through the Go interface too.
	f := newConsumerFixture(t)
	b := notChecked()
	in := investigate.BehaviourInput{Behaviour: b["behaviour"].(string), Source: domain.BehaviourNotChecked, Reason: "x"}
	s, err := NewSession(SessionConfig{Bundle: f.b, Store: f.st, Graph: f.g, Traversal: f.trav, Budget: investigate.DefaultAgentBudget, Prompt: mustPrompt(t, promptV12)})
	must(t, err)
	if _, _, err := s.behaviourList(context.Background(), "a", f.b.Change.Items[0], []investigate.BehaviourInput{in}); err == nil ||
		!strings.Contains(err.Error(), "this prompt version (investigate-agent/v12) takes no reason") {
		t.Fatalf("v12 typed reason: %v", err)
	}
}

func mustPrompt(t *testing.T, v string) *Prompt {
	t.Helper()
	p, err := LoadPrompt(v)
	must(t, err)
	return p
}

// The fetch-first check, its reasons and gaps never change the
// classification.
func TestFetchFirstNeverReachesThePolicy(t *testing.T) {
	f := newConsumerFixture(t)
	_, out := v13Run(t, f, newPort(), traceStep(f),
		f.v9Submit("potentially-affected", f.withBehaviour(notChecked())),
		f.v9Submit("potentially-affected", f.withBehaviour(notCheckedBecause("not in the product's source"))))
	if out.Run.Status != domain.AgentSubmitted || out.Assessments[0].Behaviour[0].Reason == "" {
		t.Fatalf("run %s %+v", out.Run.Status, out.Assessments)
	}
	cfg := investigate.Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound, Agent: v13Config()}
	run := func(agent *investigate.AgentOutcome) []byte {
		cls, err := classify.New().Classify(context.Background(), investigate.ClassifyInput{Bundle: f.b, Traversal: f.trav, Agent: agent, Validations: out.Validations, Config: cfg})
		must(t, err)
		b, err := json.Marshal(cls)
		must(t, err)
		return b
	}
	stripped := *out
	stripped.Assessments = append([]domain.AgentAssessment(nil), out.Assessments...)
	stripped.Assessments[0].Behaviour = nil
	without := *out
	without.EvidenceRequests, without.Assessments, without.Verdicts = nil, nil, nil
	without.Artifacts, without.Evidence = nil, nil
	a, b, c := run(&without), run(&stripped), run(out)
	if !bytes.Equal(a, c) || !bytes.Equal(b, c) {
		t.Fatalf("the fetch-first check changed the classification:\nwithout: %s\nwith:    %s", a, c)
	}
}

// A scripted v13 run (the once-rejection without a request, a fetch, the
// rejection after it, the cited resubmission) replays with no mismatch, and
// so does one accepted with a reason.
func TestV13Replay(t *testing.T) {
	for name, steps := range map[string]func(f *consumerFixture) []Step{
		"fetched": func(f *consumerFixture) []Step {
			return []Step{traceStep(f), f.v9Submit("potentially-affected", f.withBehaviour(notChecked())), fileStep(f, "internal/validate/name.go"),
				f.v9Submit("potentially-affected", f.withBehaviour(notChecked())), f.v9Submit("not-affected", f.withBehaviour(upstreamBehaviour()))}
		},
		"reason": func(f *consumerFixture) []Step {
			return []Step{traceStep(f), f.v9Submit("insufficient-evidence", f.withBehaviour(notChecked())),
				f.v9Submit("insufficient-evidence", f.withBehaviour(notCheckedBecause("not in the product's source")))}
		},
	} {
		f := newConsumerFixture(t)
		port := newPort()
		_, out := v13Run(t, f, port, steps(f)...)
		if out.Run.Status != domain.AgentSubmitted {
			t.Fatalf("%s: run %s: %s", name, out.Run.Status, out.Run.Reason)
		}
		transcript := transcriptOf(t, f, out)
		if !bytes.Contains(transcript, []byte("Upstream fetching is available for")) {
			t.Fatalf("%s: the transcript lacks the once-rejection", name)
		}
		rep := &Replay{Transcript: transcript}
		cfg := v13Config()
		cfg.Driver = DriverReplay
		again, err := New(f.st, WithDriver(DriverReplay, rep), WithClock(fixedNow), WithUpstream(*port)).Investigate(context.Background(), f.input(cfg))
		must(t, err)
		if len(rep.Mismatches) != 0 || again.Run.Status != domain.AgentSubmitted {
			t.Fatalf("%s: replay: %+v %s %s", name, rep.Mismatches, again.Run.Status, again.Run.Reason)
		}
		j := func(v any) string { b, _ := json.Marshal(v); return string(b) }
		if again.Run.Transcript != out.Run.Transcript || again.Run.PromptVersion != promptV13 || again.Run.PromptDigest != out.Run.PromptDigest ||
			j(again.Assessments) != j(out.Assessments) {
			t.Fatalf("%s: the replay differs from the recording", name)
		}
	}
}

// A v12 transcript replays unchanged: a not-checked behaviour after no
// request, and after a request, is accepted as v12 always accepted it, so
// the check runs only under fetchFirst.
func TestV12ReplayUnchangedByFetchFirst(t *testing.T) {
	f := newConsumerFixture(t)
	port := newPort()
	d, out := v12Run(t, f, port, traceStep(f), f.v9Submit("potentially-affected", f.withBehaviour(notChecked())))
	if err := submitErr(d, 1); err != nil {
		t.Fatalf("v12 not-checked with a port and no request: %v", err)
	}
	f2 := newConsumerFixture(t)
	d2, _ := v12Run(t, f2, newPort(), traceStep(f2), fileStep(f2, "internal/validate/name.go"), f2.v9Submit("potentially-affected", f2.withBehaviour(notChecked())))
	if err := submitErr(d2, 2); err != nil {
		t.Fatalf("v12 not-checked after a request: %v", err)
	}
	rep := &Replay{Transcript: transcriptOf(t, f, out)}
	cfg := v12Config()
	cfg.Driver = DriverReplay
	again, err := New(f.st, WithDriver(DriverReplay, rep), WithClock(fixedNow), WithUpstream(*port)).Investigate(context.Background(), f.input(cfg))
	must(t, err)
	if len(rep.Mismatches) != 0 || again.Run.Transcript != out.Run.Transcript {
		t.Fatalf("v12 replay: %+v", rep.Mismatches)
	}
}
