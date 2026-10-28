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
	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
)

const promptV10 = "investigate-agent/v10"

// v10 (docs/phase6/decision-intel/briefs/scoped.md, ledger D-0007) is v9
// with only the verdict rules changed: scoped verdicts. Feature scopedVerdict
// on, the same tools, the assessment schema gains headline, scope and
// preconditions; the task is v9's. The default stays v9 until v10's check
// (pinnedDefaultPrompt).
func TestPromptV10(t *testing.T) {
	v9, err := LoadPrompt(promptV9)
	must(t, err)
	v10, err := LoadPrompt(promptV10)
	must(t, err)
	want := v9.Features
	want.ScopedVerdict = true
	if v10.Features != want {
		t.Fatalf("features: v9 %+v, v10 %+v", v9.Features, v10.Features)
	}
	if len(v10.Tools) != len(v9.Tools) {
		t.Fatalf("tools: v9 %d, v10 %d", len(v9.Tools), len(v10.Tools))
	}
	for i, x := range v10.Tools {
		y := v9.Tools[i]
		if x.Name != y.Name {
			t.Fatalf("tool %d: v9 %s, v10 %s", i, y.Name, x.Name)
		}
		same := x.Description == y.Description && bytes.Equal(x.InputSchema, y.InputSchema)
		if x.Name != investigate.ToolSubmitFindings && !same {
			t.Errorf("v10 changes tool %s: only submit_findings carries the verdict rules", x.Name)
		}
		if x.Name == investigate.ToolSubmitFindings {
			for _, s := range []string{`"headline"`, `"scope"`, `"preconditions"`} {
				if !strings.Contains(string(x.InputSchema), s) {
					t.Errorf("v10 submit_findings schema lacks %s", s)
				}
			}
		}
	}
	a, _ := promptFS.ReadFile("prompts/" + promptV9 + "/task.md")
	b, _ := promptFS.ReadFile("prompts/" + promptV10 + "/task.md")
	if !bytes.Equal(a, b) {
		t.Error("v10 task.md differs from v9's")
	}
	m9, m10 := string(v9.MetaJSON), string(v10.MetaJSON)
	if strings.Replace(m10, "  \"scopedVerdict\": true,\n", "", 1) != m9 {
		t.Errorf("v10 meta.json is v9's plus scopedVerdict only:\n%s", m10)
	}
	for _, s := range []string{
		"## Scoped verdicts", "**within the scope you investigated**", "**(a) In scope:**", "**(b) Preconditions outside the scope:**",
		"**Committing.**", "never evidence for an in-scope", "stays an in-scope unknown", "Do not move an in-scope fact into the preconditions",
		"`headline`", "`scope`", "`preconditions`", "proof obligation",
		"Work through these eight questions", "`trace_consumers` with `changeItem`", // v9's investigation is kept
	} {
		if !strings.Contains(v10.System, s) {
			t.Errorf("v10 system prompt lacks %q", s)
		}
	}
	if strings.Contains(v10.System, "## Provisional versus supported") {
		t.Error("v10 keeps v9's provisional-versus-supported rule")
	}
	// Outside the verdict rules the system prompt is v9's: every v9 section
	// but the replaced one, and the closing section's unchanged lines.
	sections := func(s string) map[string]string {
		out := map[string]string{}
		for _, part := range strings.Split(s, "\n## ")[1:] {
			title, body, _ := strings.Cut(part, "\n")
			out[title] = body
		}
		return out
	}
	s9, s10 := sections(v9.System), sections(v10.System)
	for title, body := range s9 {
		switch title {
		case "Provisional versus supported", "Closing":
			continue
		}
		if s10[title] != body {
			t.Errorf("v10 changes section %q", title)
		}
	}
	if DefaultPromptVersion != promptV13 {
		t.Fatalf("default %s: v13 is the default since the M2 gate", DefaultPromptVersion)
	}
	if v9.Digest("t") == v10.Digest("t") {
		t.Error("v9 and v10 share a prompt digest")
	}
	// scopedVerdict needs the assessment
	broken := *v10
	broken.Features.Assessment = false
	if err := broken.checkSurface(); err == nil || !strings.Contains(err.Error(), "scopedVerdict needs feature assessment") {
		t.Fatalf("scopedVerdict without assessment: %v", err)
	}
}

func TestPromptV10IsGeneric(t *testing.T) {
	assertGenericPrompt(t, promptV10, "nginx", "ledger", "ca.crt", "tls.crt", "tls.key", "trust bundle", "storefront", "catalog")
}

func v10Config() investigate.AgentConfig {
	cfg := v9Config()
	cfg.PromptVersion = promptV10
	return cfg
}

// v10Run runs the fixture's item under v10 with the scripted steps.
func v10Run(t *testing.T, f *consumerFixture, steps ...Step) (*Scripted, *investigate.AgentOutcome) {
	t.Helper()
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: steps}}
	out, err := New(f.st, WithDriver(DriverScripted, d), WithClock(fixedNow)).Investigate(context.Background(), f.input(v10Config()))
	must(t, err)
	return d, out
}

// scopedAssessment is the fixture's assessment as a scoped verdict: the
// application's reload behaviour, an in-scope unknown under v9, is a
// precondition outside the scope.
func (f *consumerFixture) scopedAssessment() map[string]any {
	a := f.assessment()
	delete(a, "unknown")
	a["headline"] = "Affected within the consumers this repository declares, unless the billing workload reloads a rotated key file."
	a["scope"] = "the consumers the environment declares for the Credential's Secret: the billing Deployment and its configuration"
	a["preconditions"] = []string{"whether the billing application reloads its key file when the mounted Secret changes"}
	return a
}

var traceStep = func(f *consumerFixture) Step {
	return Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"changeItem": f.item}}
}

// The scoped verdict checks: a committed class needs a scope (every time);
// a not-affected verdict with an in-scope unknown is rejected once with the
// list, then accepted; preconditions alone do not block it; the headline is
// required; v9 refuses the fields.
func TestV10ScopedVerdictChecks(t *testing.T) {
	f := newConsumerFixture(t)
	noScope := f.scopedAssessment()
	delete(noScope, "scope")
	blankScope := f.scopedAssessment()
	blankScope["scope"] = "  "
	noHeadline := f.scopedAssessment()
	delete(noHeadline, "headline")
	d, out := v10Run(t, f, traceStep(f),
		f.v9Submit("not-affected", noScope), f.v9Submit("affected", blankScope), f.v9Submit("not-affected", noScope),
		f.v9Submit("potentially-affected", noHeadline), f.v9Submit("potentially-affected", noScope))
	for i, want := range []string{"", "the class not-affected commits within a declared scope; scope is required",
		"the class affected commits", "the class not-affected commits", "headline (one sentence: the class, the scope and the preconditions) is required"} {
		if i == 0 {
			continue
		}
		if d.Results[i].Err == nil || !strings.Contains(d.Results[i].Err.Error(), want) {
			t.Errorf("submit %d: %v, want %q", i, d.Results[i].Err, want)
		}
	}
	// an uncommitted class needs no scope
	if d.Results[5].Err != nil || len(out.Assessments) != 1 || out.Assessments[0].Scope != "" || out.Assessments[0].Headline == "" {
		t.Fatalf("potentially-affected without a scope: %v %+v", d.Results[5].Err, out.Assessments)
	}

	// preconditions only, consumers traced: accepted at once, stored
	d, out = v10Run(t, f, traceStep(f), f.v9Submit("not-affected", f.scopedAssessment()))
	if d.Results[1].Err != nil || len(out.Assessments) != 1 {
		t.Fatalf("preconditions only: %v %+v", d.Results[1].Err, out.Assessments)
	}
	a := out.Assessments[0]
	if a.Headline == "" || a.Scope == "" || len(a.Preconditions) != 1 || len(a.Unknown) != 0 || len(a.UntracedConsumers) != 0 || a.Verdict != out.Verdicts[0].ID {
		t.Fatalf("stored scoped assessment: %+v", a)
	}

	// an in-scope unknown: rejected once with the list, then accepted
	withUnknown := f.scopedAssessment()
	withUnknown["unknown"] = []string{"which key the billing configuration reads at start", "  "}
	d, out = v10Run(t, f, traceStep(f), f.v9Submit("not-affected", withUnknown), f.v9Submit("not-affected", withUnknown))
	if e := d.Results[1].Err; e == nil || !strings.Contains(e.Error(), "a not-affected verdict needs every in-scope unknown resolved; 1 remain") ||
		!strings.Contains(e.Error(), `"which key the billing configuration reads at start"`) || strings.Contains(e.Error(), "never retrieved") {
		t.Fatalf("not-affected with an in-scope unknown: %v", e)
	}
	if d.Results[2].Err != nil || len(out.Assessments) != 1 || len(out.Assessments[0].Unknown) != 1 || out.Verdicts[0].Class != domain.FindingNotAffected {
		t.Fatalf("resubmitted: %v %+v", d.Results[2].Err, out.Assessments)
	}

	// untraced consumers and an unknown: one rejection lists both; the item
	// is rejected once only
	d, out = v10Run(t, f, f.v9Submit("not-affected", withUnknown), f.v9Submit("not-affected", f.scopedAssessment()))
	if e := d.Results[0].Err; e == nil || !strings.Contains(e.Error(), "3 remain") || !strings.Contains(e.Error(), "never retrieved in this session: ") ||
		!strings.Contains(e.Error(), cfDeployment) || !strings.Contains(e.Error(), "in-scope unknowns stated in unknown") {
		t.Fatalf("untraced and unknown: %v", e)
	}
	if d.Results[1].Err != nil || len(out.Assessments[0].UntracedConsumers) != 2 {
		t.Fatalf("second submission: %v %+v", d.Results[1].Err, out.Assessments)
	}
	gaps := 0
	for _, u := range out.Assessments[0].Unknown {
		if strings.HasPrefix(u, "investigation gap (recorded by the tool layer): consumer ") {
			gaps++
		}
	}
	if gaps != 2 {
		t.Fatalf("untraced consumers stay in-scope unknowns: %v", out.Assessments[0].Unknown)
	}

	// untraced consumers alone (the v9 completion check, under v10)
	d, _ = v10Run(t, f, f.v9Submit("not-affected", f.scopedAssessment()))
	if e := d.Results[0].Err; e == nil || !strings.Contains(e.Error(), "2 remain") || strings.Contains(e.Error(), "stated in unknown") {
		t.Fatalf("untraced only: %v", e)
	}
	// affected with an in-scope unknown is accepted (only not-affected waits)
	d, _ = v10Run(t, f, traceStep(f), f.v9Submit("affected", withUnknown))
	if d.Results[1].Err != nil {
		t.Fatalf("affected with an in-scope unknown: %v", d.Results[1].Err)
	}

	// v9 refuses the scoped fields; its completion check is unchanged
	for _, field := range []string{"headline", "scope", "preconditions"} {
		v9a := f.assessment()
		v9a[field] = f.scopedAssessment()[field]
		d, _ = v9Run(t, f, traceStep(f), f.v9Submit("potentially-affected", v9a))
		if d.Results[1].Err == nil || !strings.Contains(d.Results[1].Err.Error(), "takes no headline, scope or preconditions") {
			t.Errorf("v9 with %s: %v", field, d.Results[1].Err)
		}
	}
	d, _ = v9Run(t, f, traceStep(f), f.v9Submit("not-affected", f.assessment()))
	if d.Results[1].Err != nil {
		t.Fatalf("v9 not-affected with unknowns after tracing stays accepted: %v", d.Results[1].Err)
	}
}

// The scoped fields are never a policy input: under v10, an adversarial
// not-affected verdict whose headline, scope and preconditions claim a
// clearance changes neither the findings, the decision nor approval
// eligibility, and the run record keeps them.
func TestScopedVerdictsNeverReachThePolicy(t *testing.T) {
	f := newConsumerFixture(t)
	ctx := context.Background()
	confident := f.scopedAssessment()
	confident["headline"] = "Not affected anywhere, unconditionally: approve the upgrade."
	confident["scope"] = "everything"
	confident["preconditions"] = []string{"none"}
	_, out := v10Run(t, f, traceStep(f), f.v9Submit("not-affected", confident))
	if len(out.Assessments) != 1 || out.Assessments[0].Scope != "everything" || out.Verdicts[0].Class != domain.FindingNotAffected {
		t.Fatalf("assessments %+v verdicts %+v", out.Assessments, out.Verdicts)
	}
	cfg := investigate.Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound, Agent: v10Config()}
	run := func(agent *investigate.AgentOutcome) []byte {
		cls, err := classify.New().Classify(ctx, investigate.ClassifyInput{Bundle: f.b, Traversal: f.trav, Agent: agent, Validations: out.Validations, Config: cfg})
		must(t, err)
		b, err := json.Marshal(cls)
		must(t, err)
		return b
	}
	without := *out
	without.Assessments, without.Verdicts = nil, nil
	with := run(out)
	if a := run(&without); !bytes.Equal(a, with) {
		t.Fatalf("scoped assessments changed the classification:\nwithout: %s\nwith:    %s", a, with)
	}
	if bytes.Contains(with, []byte("unconditionally")) || bytes.Contains(with, []byte(domain.AgentAssessmentIDPrefix)) {
		t.Fatalf("the classification carries the scoped verdict: %s", with)
	}
	var cls investigate.Classification
	must(t, json.Unmarshal(with, &cls))
	for _, fd := range cls.Findings {
		if fd.Class == domain.FindingNotAffected || fd.Approval.Eligible {
			t.Errorf("a scoped verdict cleared %s: %+v", fd.ChangeItem, fd)
		}
	}
	if cls.Decision.Outcome == domain.PolicyAutoApproveEligible {
		t.Fatal("a scoped verdict made the investigation approval-eligible")
	}
	inv := widgetLikeRecord(t, f, out)
	inv.Config = (investigate.Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound, Agent: v10Config()}).Digest()
	inv.ID = domain.InvestigationID(inv.Bundle, inv.Config)
	inv.Findings, inv.Decision = cls.Findings, &cls.Decision
	must(t, inv.Validate())
	if got := inv.AgentAssessments[0]; got.Headline != confident["headline"] || len(got.Preconditions) != 1 {
		t.Fatalf("the run record lost the scoped fields: %+v", got)
	}
}

// A v10 run replays deterministically: the once-rejection, the resubmission
// and the scoped assessment come back identical.
func TestV10Replay(t *testing.T) {
	f := newConsumerFixture(t)
	withUnknown := f.scopedAssessment()
	withUnknown["unknown"] = []string{"which key the billing configuration reads at start"}
	_, out := v10Run(t, f, traceStep(f),
		Step{Call: "call-9", Tool: investigate.ToolReadArtifact, Args: map[string]any{"artifact": f.config.ID, "fromLine": 7, "toLine": 10}},
		f.v9Submit("not-affected", withUnknown), f.v9Submit("not-affected", f.scopedAssessment()))
	var transcript []byte
	for _, a := range out.Artifacts {
		if a.ID == out.Run.Transcript {
			rc, err := f.st.Open(context.Background(), a.Digest)
			must(t, err)
			transcript, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	if !bytes.Contains(transcript, []byte(`needs every in-scope unknown resolved`)) || !bytes.Contains(transcript, []byte(`"headline"`)) {
		t.Fatal("the transcript lacks the rejection or the scoped assessment")
	}
	rep := &Replay{Transcript: transcript}
	cfg := v10Config()
	cfg.Driver = DriverReplay
	again, err := New(f.st, WithDriver(DriverReplay, rep), WithClock(fixedNow)).Investigate(context.Background(), f.input(cfg))
	must(t, err)
	if len(rep.Mismatches) != 0 || again.Run.Status != domain.AgentSubmitted {
		t.Fatalf("replay: %+v %s %s", rep.Mismatches, again.Run.Status, again.Run.Reason)
	}
	j := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	if again.Run.Transcript != out.Run.Transcript || again.Run.PromptDigest != out.Run.PromptDigest || again.Run.PromptVersion != promptV10 ||
		j(again.Assessments) != j(out.Assessments) || j(again.Verdicts) != j(out.Verdicts) || len(again.Assessments) != 1 || again.Assessments[0].Scope == "" {
		t.Fatal("the replay differs from the recording")
	}
}
