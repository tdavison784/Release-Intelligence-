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

const promptV12 = "investigate-agent/v12"

// v12 (docs/phase6/decision-intel/briefs/upstream2.md, ledger D-0013) is
// v11 plus structured behaviour dependencies: feature behaviour, the
// assessment's behaviour list in submit_findings, one system section and the
// closing bullet, and the item task's mention. Everything else is v11's; the
// default stays v9.
func TestPromptV12(t *testing.T) {
	v11, err := LoadPrompt(promptV11)
	must(t, err)
	v12, err := LoadPrompt(promptV12)
	must(t, err)
	want := v11.Features
	want.Behaviour = true
	if v12.Features != want || v11.Features.Behaviour {
		t.Fatalf("features: v11 %+v, v12 %+v", v11.Features, v12.Features)
	}
	if len(v12.Tools) != len(v11.Tools) {
		t.Fatalf("tools: v11 %d, v12 %d", len(v11.Tools), len(v12.Tools))
	}
	for i, x := range v12.Tools {
		y := v11.Tools[i]
		same := x.Name == y.Name && x.Description == y.Description && bytes.Equal(x.InputSchema, y.InputSchema)
		if x.Name != investigate.ToolSubmitFindings && !same {
			t.Errorf("v12 changes tool %s: only submit_findings gains behaviour", x.Name)
		}
		if x.Name == investigate.ToolSubmitFindings {
			for _, s := range []string{`"behaviour"`, `"upstream"`, `"change-text"`, `"unavailable"`, `"not-checked"`, `"request"`} {
				if !strings.Contains(string(x.InputSchema), s) {
					t.Errorf("v12 submit_findings schema lacks %s", s)
				}
			}
			if !strings.Contains(x.Description, "a not-affected verdict is rejected while any is unavailable or not-checked") {
				t.Error("v12 submit_findings description lacks the behaviour check")
			}
		}
	}
	for _, f := range []string{"task.md"} {
		a, _ := promptFS.ReadFile("prompts/" + promptV11 + "/" + f)
		b, _ := promptFS.ReadFile("prompts/" + promptV12 + "/" + f)
		if !bytes.Equal(a, b) {
			t.Errorf("v12 %s differs from v11's", f)
		}
	}
	if string(v12.MetaJSON) != strings.TrimSuffix(string(v11.MetaJSON), "\n}\n")+",\n  \"behaviour\": true\n}\n" {
		t.Errorf("v12 meta.json is v11's plus behaviour only:\n%s", v12.MetaJSON)
	}
	s11, s12 := systemSections(v11.System), systemSections(v12.System)
	for title, body := range s11 {
		if title == "Closing" {
			continue // one bullet added
		}
		if s12[title] != body {
			t.Errorf("v12 changes section %q", title)
		}
	}
	if len(s12) != len(s11)+1 || s12["Behaviour the verdict relies on"] == "" {
		t.Errorf("v12 sections: %d (v11 %d + the behaviour section)", len(s12), len(s11))
	}
	for _, s := range []string{"`upstream`", "`change-text`", "`unavailable`", "`not-checked`", "never the wording",
		"is rejected while any\nbehaviour is `unavailable` or `not-checked`", "is rejected once", `"behaviour <b> at <tag>: <source>"`} {
		if !strings.Contains(s12["Behaviour the verdict relies on"], s) {
			t.Errorf("v12 behaviour section lacks %q", s)
		}
	}
	if c := s12["Closing"]; strings.Replace(c, "  - `behaviour`: each upstream behaviour the verdict relies on, with its source (above);\n", "", 1) != s11["Closing"] {
		t.Error("v12 Closing is v11's plus the behaviour bullet only")
	}
	item, _ := promptFS.ReadFile("prompts/" + promptV12 + "/item.md")
	for _, s := range []string{"{{.Upstream}}", "List each such behaviour in the assessment's `behaviour`", "behaviour with\n   sources"} {
		if !strings.Contains(string(item), s) {
			t.Errorf("v12 item.md lacks %q", s)
		}
	}
	if DefaultPromptVersion != promptV13 {
		t.Fatalf("default %s: v13 is the default since the M2 gate", DefaultPromptVersion)
	}
	if v11.Digest("t") == v12.Digest("t") {
		t.Error("v11 and v12 share a prompt digest")
	}
	broken := *v12
	broken.Features.Upstream = UpstreamBounds{}
	if err := broken.checkSurface(); err == nil || !strings.Contains(err.Error(), "behaviour needs feature upstream") {
		t.Fatalf("behaviour without upstream: %v", err)
	}
}

func TestPromptV12IsGeneric(t *testing.T) {
	assertGenericPrompt(t, promptV12, "nginx", "ledger", "ca.crt", "tls.crt", "tls.key", "trust bundle", "storefront", "catalog",
		"pathType", "ImplementationSpecific", "regex", "CVE", "GHSA", "strict")
}

func systemSections(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, "\n## ")[1:] {
		title, body, _ := strings.Cut(part, "\n")
		out[title] = body
	}
	return out
}

// --- fixtures -------------------------------------------------------------------------

func v12Config() investigate.AgentConfig {
	cfg := v9Config()
	cfg.PromptVersion = promptV12
	return cfg
}

// v12Run runs the fixture's item under v12 with the scripted steps.
func v12Run(t *testing.T, f *consumerFixture, up *Upstream, steps ...Step) (*Scripted, *investigate.AgentOutcome) {
	t.Helper()
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: steps}}
	opts := []Option{WithDriver(DriverScripted, d), WithClock(fixedNow)}
	if up != nil {
		opts = append(opts, WithUpstream(*up))
	}
	out, err := New(f.st, opts...).Investigate(context.Background(), f.input(v12Config()))
	must(t, err)
	return d, out
}

// fetchedName is the fixture's validation file fetched at the target's tag.
var fetchedName = domain.ArtifactID(domain.ArtifactUpstreamDocument, "upstream/keystore/v2.1.0/source/example/keystore/internal/validate/name.go",
	domain.Digest([]byte(upstreamSource["v2.1.0"]["internal/validate/name.go"])))

const validNameLine = "var validName = regexp.MustCompile(`^[a-z0-9-]+$`)"

// withBehaviour is the fixture's assessment with a behaviour list.
func (f *consumerFixture) withBehaviour(bs ...map[string]any) map[string]any {
	a := f.assessment()
	a["behaviour"] = bs
	return a
}

func upstreamBehaviour() map[string]any {
	return map[string]any{"behaviour": "which names the validation accepts at the target", "source": "upstream",
		"citations": []map[string]any{{"artifact": fetchedName, "locator": "internal/validate/name.go:6", "quote": validNameLine}}}
}

func notChecked() map[string]any {
	return map[string]any{"behaviour": "whether renewal keeps the key when spec.rotate is unset", "source": "not-checked"}
}

func submitErr(d *Scripted, i int) error { return d.Results[i].Err }

// --- checks ---------------------------------------------------------------------------

// A not-affected verdict resting on a not-checked behaviour is rejected every
// time; resubmitted as potentially-affected it is accepted, and the gap is in
// unknown.
func TestV12NotCheckedBehaviourBlocksNotAffected(t *testing.T) {
	f := newConsumerFixture(t)
	d, out := v12Run(t, f, nil, traceStep(f),
		f.v9Submit("not-affected", f.withBehaviour(notChecked())),
		f.v9Submit("not-affected", f.withBehaviour(notChecked())),
		f.v9Submit("potentially-affected", f.withBehaviour(notChecked())))
	for _, i := range []int{1, 2} {
		err := submitErr(d, i)
		if err == nil || !errors.Is(err, investigate.ErrRejected) ||
			!strings.Contains(err.Error(), `cannot rest on upstream behaviour that is not established; 1 behaviours are unavailable or not-checked: behaviour[0] "whether renewal keeps the key when spec.rotate is unset" (not-checked)`) ||
			!strings.Contains(err.Error(), "request_evidence kind upstream-search or upstream-file") || !strings.Contains(err.Error(), "potentially-affected, insufficient-evidence") {
			t.Fatalf("submission %d: %v", i, err)
		}
	}
	if out.Run.Status != domain.AgentSubmitted || len(out.Assessments) != 1 {
		t.Fatalf("run %s: %s", out.Run.Status, out.Run.Reason)
	}
	a := out.Assessments[0]
	if len(a.Behaviour) != 1 || a.Behaviour[0].Source != domain.BehaviourNotChecked ||
		a.Unknown[len(a.Unknown)-1] != `investigation gap (recorded by the tool layer): behaviour "whether renewal keeps the key when spec.rotate is unset" at v2.1.0: not-checked` {
		t.Fatalf("assessment %+v\n%v", a.Behaviour, a.Unknown)
	}
}

// A not-affected verdict whose behaviour is cited from a fetched upstream
// artifact is accepted at once; the citation becomes run evidence and the
// record validates.
func TestV12UpstreamBehaviourAccepted(t *testing.T) {
	f := newConsumerFixture(t)
	up := &fakeUpstream{src: upstreamSource}
	port := up.port()
	d, out := v12Run(t, f, &port, traceStep(f), fileStep(f, "internal/validate/name.go"),
		f.v9Submit("not-affected", f.withBehaviour(upstreamBehaviour())))
	if err := submitErr(d, 2); err != nil {
		t.Fatal(err)
	}
	a := out.Assessments[0]
	if len(a.Behaviour) != 1 || len(a.Behaviour[0].Citations) != 1 || strings.Contains(strings.Join(a.Unknown, "\n"), "behaviour \"") {
		t.Fatalf("assessment %+v %v", a.Behaviour, a.Unknown)
	}
	cited := false
	for _, e := range out.Evidence {
		cited = cited || (e.ID == a.Behaviour[0].Citations[0] && e.SourceID == fetchedName)
	}
	inputs := false
	for _, e := range a.Provenance.InputEvidence {
		inputs = inputs || e == a.Behaviour[0].Citations[0]
	}
	if !cited || !inputs {
		t.Fatalf("the behaviour citation is not run evidence (%v) or an input of the assessment (%v)", cited, inputs)
	}
	inv := widgetLikeRecord(t, f, out)
	must(t, inv.Validate())
	must(t, inv.ValidateAgainst(f.b))

	// An upstream source of the bundle counts too, when the quote goes
	// beyond the item's change text.
	f2 := newConsumerFixture(t)
	notes := map[string]any{"behaviour": "how the rotation default is restored", "source": "upstream",
		"citations": []map[string]any{{"artifact": f2.notes.ID, "quote": "to keep the key"}}}
	d2, _ := v12Run(t, f2, nil, traceStep(f2), f2.v9Submit("not-affected", f2.withBehaviour(notes)))
	// "Set `spec.rotate: false` to keep the key." is the change's detail: it
	// is change text, not upstream text beyond it.
	if err := submitErr(d2, 1); err == nil || !strings.Contains(err.Error(), "every citation quotes the item's own change text; use source change-text") {
		t.Fatalf("change text cited as upstream: %v", err)
	}
	f3 := newConsumerFixture(t)
	notes["citations"] = []map[string]any{{"artifact": f3.notes.ID, "quote": "## Breaking changes"}}
	d3, _ := v12Run(t, f3, nil, traceStep(f3), f3.v9Submit("not-affected", f3.withBehaviour(notes)))
	if err := submitErr(d3, 1); err != nil {
		t.Fatalf("bundle upstream source beyond the change text: %v", err)
	}
}

// An upstream source must cite upstream artifacts that resolve.
func TestV12UpstreamSourceIsChecked(t *testing.T) {
	f := newConsumerFixture(t)
	env := map[string]any{"behaviour": "b", "source": "upstream", "citations": []map[string]any{{"artifact": f.config.ID, "quote": "key_file = /etc/billing/tls/tls.key"}}}
	forged := map[string]any{"behaviour": "b", "source": "upstream", "citations": []map[string]any{{"artifact": f.notes.ID, "quote": "names are validated strictly"}}}
	none := map[string]any{"behaviour": "b", "source": "upstream"}
	mixed := map[string]any{"behaviour": "b", "source": "upstream", "citations": upstreamBehaviour()["citations"], "request": "erq-x"}
	bad := map[string]any{"behaviour": "b", "source": "remembered"}
	blank := map[string]any{"behaviour": " ", "source": "not-checked"}
	d, out := v12Run(t, f, nil, traceStep(f),
		f.v9Submit("potentially-affected", f.withBehaviour(env)), f.v9Submit("potentially-affected", f.withBehaviour(forged)),
		f.v9Submit("potentially-affected", f.withBehaviour(none)), f.v9Submit("potentially-affected", f.withBehaviour(mixed)),
		f.v9Submit("potentially-affected", f.withBehaviour(bad)), f.v9Submit("potentially-affected", f.withBehaviour(blank)),
		f.v9Submit("potentially-affected", f.assessment()))
	for i, want := range []string{
		"is not upstream evidence", "the quote does not occur in artifact", "source upstream needs citations", "source upstream takes citations, not quote or request",
		`source "remembered": want upstream, change-text, unavailable, not-checked`, "behaviour[0].behaviour (the upstream behaviour the verdict relies on) is required",
	} {
		if err := submitErr(d, i+1); err == nil || !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), want) {
			t.Errorf("case %d: %v, want %q", i, err, want)
		}
	}
	if out.Run.Status != domain.AgentSubmitted {
		t.Fatalf("run %s: %s", out.Run.Status, out.Run.Reason)
	}
}

// A change-text source must quote the item's own change text: a forged quote
// is rejected; a genuine one is accepted and can support not-affected.
func TestV12ChangeTextQuote(t *testing.T) {
	f := newConsumerFixture(t)
	forged := map[string]any{"behaviour": "renewal keeps the key when spec.rotate is false", "source": "change-text", "quote": "spec.rotate: false keeps the key on every renewal"}
	other := map[string]any{"behaviour": "b", "source": "change-text", "quote": "## Breaking changes"} // in the notes, not in the item's text
	genuine := map[string]any{"behaviour": "renewal keeps the key when spec.rotate is false", "source": "change-text", "quote": "Set `spec.rotate: false` to keep the key."}
	excerpt := map[string]any{"behaviour": "the controller rotates on renewal", "source": "change-text", "quote": "rotates a Credential's private key"}
	cited := map[string]any{"behaviour": "b", "source": "change-text", "quote": "keep the key", "citations": f.upstreamCite()}
	d, out := v12Run(t, f, nil, traceStep(f),
		f.v9Submit("not-affected", f.withBehaviour(forged)), f.v9Submit("not-affected", f.withBehaviour(other)),
		f.v9Submit("not-affected", f.withBehaviour(cited)), f.v9Submit("not-affected", f.withBehaviour(genuine, excerpt)))
	for i, want := range []string{"the quote does not occur in the item's change text", "the quote does not occur in the item's change text", "source change-text takes quote, not citations or request"} {
		if err := submitErr(d, i+1); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if err := submitErr(d, 4); err != nil {
		t.Fatal(err)
	}
	if b := out.Assessments[0].Behaviour; len(b) != 2 || b[0].Quote != "Set `spec.rotate: false` to keep the key." || b[1].Source != domain.BehaviourChangeText {
		t.Fatalf("behaviour %+v", b)
	}
}

// An unavailable source names an upstream request of the item recorded
// unavailable; anything else is rejected. With a real one, potentially-
// affected is accepted with the gap, and not-affected is rejected.
func TestV12UnavailableNeedsTheRequest(t *testing.T) {
	f := newConsumerFixture(t)
	up := &fakeUpstream{src: upstreamSource}
	port := up.port()
	steps := []Step{traceStep(f), fileStep(f, "internal/validate/missing.go"), fileStep(f, "internal/validate/name.go")}
	d0, out0 := v12Run(t, f, &port, append(steps, f.v9Submit("potentially-affected", f.assessment()))...)
	if err := submitErr(d0, 3); err != nil {
		t.Fatal(err)
	}
	missing, fetched := out0.EvidenceRequests[0], out0.EvidenceRequests[1]
	if missing.Status != domain.RequestUnavailable || fetched.Status != domain.RequestFulfilled {
		t.Fatalf("requests %+v", out0.EvidenceRequests)
	}
	unavailable := func(id string) map[string]any {
		return map[string]any{"behaviour": "what the validation accepts", "source": "unavailable", "request": id}
	}
	f2 := newConsumerFixture(t)
	up2 := &fakeUpstream{src: upstreamSource}
	port2 := up2.port()
	d, out := v12Run(t, f2, &port2, append(steps,
		f2.v9Submit("potentially-affected", f2.withBehaviour(unavailable("erq-000000000000"))),
		f2.v9Submit("potentially-affected", f2.withBehaviour(unavailable(fetched.ID))),
		f2.v9Submit("not-affected", f2.withBehaviour(unavailable(missing.ID))),
		f2.v9Submit("potentially-affected", f2.withBehaviour(unavailable(missing.ID))))...)
	for i, want := range []string{
		`request "erq-000000000000" is not an upstream request_evidence of this item recorded unavailable`,
		`request "` + fetched.ID + `" is not an upstream request_evidence of this item recorded unavailable`,
		`behaviour[0] "what the validation accepts" (unavailable)`,
	} {
		if err := submitErr(d, 3+i); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("case %d: %v, want %q", i, err, want)
		}
	}
	if err := submitErr(d, 6); err != nil {
		t.Fatal(err)
	}
	a := out.Assessments[0]
	want := `investigation gap (recorded by the tool layer): behaviour "what the validation accepts" at v2.1.0: unavailable (request ` + missing.ID + `: not found at v2.1.0`
	if len(a.Behaviour) != 1 || a.Behaviour[0].Request != missing.ID || !strings.HasPrefix(a.Unknown[len(a.Unknown)-1], want) || len(a.UnavailableEvidence) != 1 {
		t.Fatalf("assessment %+v\n%v", a.Behaviour, a.Unknown)
	}
	inv := widgetLikeRecord(t, f2, out)
	must(t, inv.Validate())
	inv.AgentAssessments[0].Behaviour[0].Request = fetched.ID
	if err := inv.Validate(); err == nil || !strings.Contains(err.Error(), "not an unavailable evidence request of its item") {
		t.Fatalf("record naming a fulfilled request: %v", err)
	}
}

// A behavioural item's not-affected verdict with an empty behaviour list is
// rejected once; the second submission is accepted. A non-behavioural item
// (rendered configuration, family values) is accepted at once.
func TestV12EmptyBehaviourList(t *testing.T) {
	f := newConsumerFixture(t)
	d, out := v12Run(t, f, nil, traceStep(f), f.v9Submit("not-affected", f.assessment()), f.v9Submit("not-affected", f.assessment()))
	err := submitErr(d, 1)
	if err == nil || !strings.Contains(err.Error(), "is a behavioural change (manifestation controller-behaviour, family behavior) and the not-affected verdict declares no behaviour") ||
		!strings.Contains(err.Error(), "A second submission is accepted") {
		t.Fatalf("first submission: %v", err)
	}
	if err := submitErr(d, 2); err != nil || out.Run.Status != domain.AgentSubmitted || len(out.Assessments[0].Behaviour) != 0 {
		t.Fatalf("second submission: %v %s", err, out.Run.Status)
	}
	// Other classes declare no behaviour freely.
	f2 := newConsumerFixture(t)
	if d2, _ := v12Run(t, f2, nil, traceStep(f2), f2.v9Submit("potentially-affected", f2.assessment())); submitErr(d2, 1) != nil {
		t.Fatal(submitErr(d2, 1))
	}
	// A non-behavioural item.
	f3 := newConsumerFixture(t)
	it := &f3.b.Change.Items[0]
	it.Family, it.Manifestation, it.ManifestationRule = domain.FamilyValues, domain.ManifestRenderedConfig, "manifest:computed-values"
	d3, out3 := v12Run(t, f3, nil, traceStep(f3), f3.v9Submit("not-affected", f3.assessment()))
	if err := submitErr(d3, 1); err != nil || out3.Run.Status != domain.AgentSubmitted {
		t.Fatalf("non-behavioural item: %v %s", err, out3.Run.Status)
	}
	for _, c := range []struct {
		fam  domain.ChangeItemFamily
		man  domain.Manifestation
		want bool
	}{
		{domain.FamilyBehavior, domain.ManifestControllerBehaviour, true},
		{domain.FamilyBehavior, domain.ManifestRenderedConfig, true}, // a behaviour note correlated to a rendered change
		{domain.FamilyAPISchema, domain.ManifestControllerBehaviour, true},
		{domain.FamilyValues, domain.ManifestUnknown, true},
		{domain.FamilyValues, domain.ManifestRenderedConfig, false},
		{domain.FamilyImage, domain.ManifestRenderedConfig, false},
		{domain.FamilyRendered, domain.ManifestRenderedConfig, false},
		{domain.FamilyCompatibility, domain.ManifestRenderedConfig, false},
	} {
		if got := behaviouralChange(domain.ChangeItem{Family: c.fam, Manifestation: c.man}); got != c.want {
			t.Errorf("%s/%s: behavioural %v", c.fam, c.man, got)
		}
	}
}

// The behaviour once-check has its own flag: an item rejected once by the
// consumer completion check is still rejected once for an empty list.
func TestV12BehaviourAndConsumerChecksRejectOnceEach(t *testing.T) {
	f := newConsumerFixture(t)
	d, out := v12Run(t, f, nil, f.v9Submit("not-affected", f.assessment()), f.v9Submit("not-affected", f.assessment()), f.v9Submit("not-affected", f.assessment()))
	if err := submitErr(d, 0); err == nil || !strings.Contains(err.Error(), "never retrieved in this session") {
		t.Fatalf("consumer check: %v", err)
	}
	if err := submitErr(d, 1); err == nil || !strings.Contains(err.Error(), "declares no behaviour") {
		t.Fatalf("behaviour check: %v", err)
	}
	if err := submitErr(d, 2); err != nil || len(out.Assessments[0].UntracedConsumers) == 0 {
		t.Fatalf("third submission: %v", err)
	}
}

// Prompts without feature behaviour refuse the field.
func TestEarlierPromptsRefuseBehaviour(t *testing.T) {
	f := newConsumerFixture(t)
	d, out := v11Run(t, f, nil, f.v9Submit("potentially-affected", f.withBehaviour(notChecked())), f.v9Submit("potentially-affected", f.assessment()))
	if err := submitErr(d, 0); err == nil || err.Error() != "investigate: rejected: items[0].assessment: this prompt version (investigate-agent/v11) takes no behaviour" {
		t.Fatalf("v11 with behaviour: %v", err)
	}
	if out.Run.Status != domain.AgentSubmitted || out.Assessments[0].Behaviour != nil {
		t.Fatalf("v11 outcome %s %+v", out.Run.Status, out.Assessments[0].Behaviour)
	}
	// v11's not-affected on the behavioural item is not asked for behaviour.
	f2 := newConsumerFixture(t)
	if d2, _ := v11Run(t, f2, nil, traceStep(f2), f2.v9Submit("not-affected", f2.assessment())); submitErr(d2, 1) != nil {
		t.Fatal(submitErr(d2, 1))
	}
}

// Behaviour dependencies and their gaps never change the classification.
func TestBehaviourNeverReachesThePolicy(t *testing.T) {
	f := newConsumerFixture(t)
	up := &fakeUpstream{src: upstreamSource}
	port := up.port()
	confident := f.withBehaviour(upstreamBehaviour(), map[string]any{"behaviour": "the key is kept", "source": "change-text", "quote": "to keep the key"})
	confident["likelyImpact"] = "none: the upstream validation accepts every name this environment uses"
	_, out := v12Run(t, f, &port, traceStep(f), fileStep(f, "internal/validate/name.go"), f.v9Submit("not-affected", confident))
	if out.Run.Status != domain.AgentSubmitted || len(out.Assessments[0].Behaviour) != 2 {
		t.Fatalf("run %s %+v", out.Run.Status, out.Assessments)
	}
	cfg := investigate.Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound, Agent: v12Config()}
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
		t.Fatalf("behaviour changed the classification:\nwithout: %s\nwith:    %s", a, c)
	}
}

// A scripted v12 run (a fetch, the not-checked rejection, the empty-list
// rejection, the accepted resubmission) replays with no mismatch.
func TestV12Replay(t *testing.T) {
	f := newConsumerFixture(t)
	up := &fakeUpstream{src: upstreamSource}
	port := up.port()
	_, out := v12Run(t, f, &port, traceStep(f), fileStep(f, "internal/validate/name.go"),
		f.v9Submit("not-affected", f.withBehaviour(notChecked())), f.v9Submit("not-affected", f.assessment()),
		f.v9Submit("not-affected", f.withBehaviour(upstreamBehaviour())))
	transcript := transcriptOf(t, f, out)
	for _, s := range []string{"cannot rest on upstream behaviour that is not established", "declares no behaviour", `"source":"upstream"`} {
		if !bytes.Contains(transcript, []byte(s)) {
			t.Fatalf("the transcript lacks %q", s)
		}
	}
	rep := &Replay{Transcript: transcript}
	cfg := v12Config()
	cfg.Driver = DriverReplay
	again, err := New(f.st, WithDriver(DriverReplay, rep), WithClock(fixedNow), WithUpstream(port)).Investigate(context.Background(), f.input(cfg))
	must(t, err)
	if len(rep.Mismatches) != 0 || again.Run.Status != domain.AgentSubmitted {
		t.Fatalf("replay: %+v %s %s", rep.Mismatches, again.Run.Status, again.Run.Reason)
	}
	j := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	if again.Run.Transcript != out.Run.Transcript || again.Run.PromptVersion != promptV12 || again.Run.PromptDigest != out.Run.PromptDigest ||
		j(again.Assessments) != j(out.Assessments) || len(again.Assessments[0].Behaviour) != 1 {
		t.Fatal("the replay differs from the recording")
	}
}
