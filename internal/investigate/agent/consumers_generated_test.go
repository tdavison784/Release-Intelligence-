package agent

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
)

// Generated configuration (ledger D-0005): billing's ConfigMap is generated
// by kustomize from a file the snapshot lacks. Consumer rules v2 read its data
// from the overlay's render, or say in the trace and the dossier that it is
// not known; a transcript recorded under v1 still replays.

// v9RunWith is v9Run with investigator options.
func v9RunWith(t *testing.T, f *consumerFixture, opts []Option, steps ...Step) (*Scripted, *investigate.AgentOutcome) {
	t.Helper()
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: steps}}
	opts = append([]Option{WithDriver(DriverScripted, d), WithClock(fixedNow)}, opts...)
	out, err := New(f.st, opts...).Investigate(context.Background(), f.input(v9Config()))
	must(t, err)
	return d, out
}

// runArtifact reads the outcome's artifact of id, or its first of kind.
func runArtifact(t *testing.T, f *consumerFixture, out *investigate.AgentOutcome, id string, kind domain.ArtifactKind) []byte {
	t.Helper()
	for _, a := range out.Artifacts {
		if a.ID == id || id == "" && a.Kind == kind {
			rc, err := f.st.Open(context.Background(), a.Digest)
			must(t, err)
			b, _ := io.ReadAll(rc)
			rc.Close()
			return b
		}
	}
	t.Fatalf("no artifact %s %s", id, kind)
	return nil
}

func gapText(ct *ConsumerTrace) string {
	var b strings.Builder
	for _, g := range ct.Gaps {
		b.WriteString(g.Subject + ": " + g.Detail + "\n")
	}
	return b.String()
}

func TestTraceGeneratedConfigFromRender(t *testing.T) {
	f := newConsumerFixtureWith(t, configGeneratedRendered)
	d, out := v9Run(t, f,
		Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"changeItem": f.item}},
		f.v9Submit("potentially-affected", f.assessment()))
	ct := decodeTrace(t, d.Results[0])
	var r *TraceHop
	for i, h := range ct.Hops {
		if h.Kind == domain.EdgeReadsFile && h.From == cfSecret && h.To == cfConfigMap {
			r = &ct.Hops[i]
		}
	}
	if r == nil || r.File == nil || r.File.Key != "tls.key" || r.File.ConfigKey != "app.ini" || r.Status != domain.EdgeEstablished {
		t.Fatalf("reads-file hop from the render: %+v", r)
	}
	// cited in the render: its artifact, line and text, and the object it read
	if r.Artifact != f.overlay.ID || r.Line != 5 || r.Text != "key_file = /etc/billing/tls/tls.key" ||
		!strings.Contains(r.File.DataFrom, "billing-config-5g7k9b2mdh") || !strings.Contains(r.File.DataFrom, "kustomize:apps/billing") {
		t.Fatalf("render citation: %+v %+v", r, r.File)
	}
	var consumers []string
	for _, c := range ct.Consumers {
		consumers = append(consumers, c.Key)
	}
	if strings.Join(consumers, ",") != cfDeployment+","+cfConfigMap {
		t.Fatalf("consumers %v", consumers)
	}
	gaps := gapText(ct)
	if strings.Contains(gaps, "is not known") || !strings.Contains(gaps, "/var/log/billing/app.log is under no volume mount") {
		t.Fatalf("gaps:\n%s", gaps)
	}
	dossier := string(runArtifact(t, f, out, "", domain.ArtifactItemDossier))
	for _, want := range []string{"reads-file: " + cfConfigMap + " data[app.ini] names /etc/billing/tls/tls.key", "the data is the rendered generated object's", f.overlay.ID + " L5"} {
		if !strings.Contains(dossier, want) {
			t.Errorf("dossier lacks %q", want)
		}
	}
}

func TestTraceGeneratedConfigUnknown(t *testing.T) {
	f := newConsumerFixtureWith(t, configGenerated)
	d, out := v9Run(t, f,
		Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"changeItem": f.item}},
		Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"node": cfConfigMap}},
		f.v9Submit("potentially-affected", f.assessment()))
	ct := decodeTrace(t, d.Results[0])
	for _, h := range ct.Hops {
		if h.Kind == domain.EdgeReadsFile && h.Status == domain.EdgeEstablished {
			t.Fatalf("a resolved reads-file hop without the data: %+v", h)
		}
	}
	gaps := gapText(ct)
	for _, want := range []string{
		"container app reads " + cfConfigMap + " data[app.ini], which is not known",
		"from file env/apps/billing/app.ini, which is not in the environment snapshot; no render of the to side holds the generated object",
		"whether it names a path under /etc/billing/tls is not established",
	} {
		if !strings.Contains(gaps, want) {
			t.Errorf("item trace gaps lack %q:\n%s", want, gaps)
		}
	}
	// traced from the configuration itself, the unknown data is its gap
	if g := gapText(decodeTrace(t, d.Results[1])); !strings.Contains(g, cfConfigMap+": data[app.ini], read by container app of "+cfDeployment+", is not known") {
		t.Errorf("node trace gaps:\n%s", g)
	}
	dossier := string(runArtifact(t, f, out, "", domain.ArtifactItemDossier))
	if !strings.Contains(dossier, "data[app.ini], which is not known") {
		t.Error("the dossier does not report the unknown data")
	}
}

// A transcript recorded under consumer rules v1 (no consumerRules in its
// header: every transcript before D-0005) replays against a v1 graph; a v2
// recording names its rules and replays under them.
func TestReplayPinsConsumerRules(t *testing.T) {
	f := newConsumerFixtureWith(t, configGenerated)
	steps := []Step{
		{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"changeItem": f.item}},
		f.v9Submit("potentially-affected", f.assessment()),
	}
	replay := func(transcript []byte, opts ...Option) (*Replay, *investigate.AgentOutcome) {
		rep := &Replay{Transcript: transcript}
		cfg := v9Config()
		cfg.Driver = DriverReplay
		out, err := New(f.st, append([]Option{WithDriver(DriverReplay, rep), WithClock(fixedNow)}, opts...)...).Investigate(context.Background(), f.input(cfg))
		must(t, err)
		return rep, out
	}

	_, v1 := v9RunWith(t, f, []Option{WithConsumerRules(relations.ConsumerRulesV1)}, steps...)
	tr := runArtifact(t, f, v1, v1.Run.Transcript, "")
	if bytes.Contains(tr, []byte(`"consumerRules"`)) {
		t.Fatal("a v1 transcript names consumer rules")
	}
	rep, again := replay(tr)
	if len(rep.Mismatches) != 0 || again.Run.Status != domain.AgentSubmitted || again.Run.Transcript != v1.Run.Transcript {
		t.Fatalf("v1 replay: %+v %s %s", rep.Mismatches, again.Run.Status, again.Run.Reason)
	}
	// the pin matters: against a v2 graph the item's dossier, so its prompt
	// digest, differs and the replay stops
	if rep, out := replay(tr, WithConsumerRules(relations.ConsumerRulesV2)); len(rep.Mismatches) == 0 && out.Run.Status == domain.AgentSubmitted {
		t.Fatal("v1 and v2 graphs serve the same session: the fixture does not exercise the rules")
	} else if !strings.Contains(out.Run.Reason, "replay: the transcript was recorded under prompt") {
		t.Fatalf("v1 transcript against v2: %s %s", out.Run.Status, out.Run.Reason)
	}

	_, v2 := v9RunWith(t, f, nil, steps...)
	tr = runArtifact(t, f, v2, v2.Run.Transcript, "")
	if !bytes.Contains(tr, []byte(`"consumerRules":2`)) {
		t.Fatal("a v2 transcript does not name its consumer rules")
	}
	if rep, again := replay(tr); len(rep.Mismatches) != 0 || again.Run.Transcript != v2.Run.Transcript {
		t.Fatalf("v2 replay: %+v", rep.Mismatches)
	}
}
