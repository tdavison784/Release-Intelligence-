package agent

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
)

// Pod-spec paths (consumer rules v3, ledger D-0006): billing's container
// names the mounted key file in its command, not in configuration data.
// Rules v3 trace Secret → Deployment through that command line; v2 report
// the mount as unread.

const podSpecField = "spec.template.spec.containers[0:app].command[2]"

func TestTracePodSpecPaths(t *testing.T) {
	f := newConsumerFixtureWith(t, configPodSpec)
	d, out := v9Run(t, f,
		Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"changeItem": f.item}},
		f.v9Submit("potentially-affected", f.assessment()))
	ct := decodeTrace(t, d.Results[0])
	var r *TraceHop
	for i, h := range ct.Hops {
		if h.Kind == domain.EdgeReadsFile && h.From == cfSecret && h.To == cfDeployment {
			r = &ct.Hops[i]
		}
	}
	if r == nil || r.File == nil || r.Status != domain.EdgeEstablished || r.File.Key != "tls.key" || r.File.Via != "pod-spec" ||
		r.File.Field != podSpecField || r.File.MountPath != "/etc/billing/tls" || r.File.Container != "app" || r.File.ConfigKey != "" {
		t.Fatalf("pod-spec reads-file hop: %+v %+v", r, r.File)
	}
	// cited: the Deployment's file, the script line naming the key
	if r.Artifact != f.deploy.ID || r.Line != 20 || r.Text != `--tls-key=/etc/billing/tls/tls.key \` || r.Rule != "graph.reads-file@v3:pod-spec:command" {
		t.Fatalf("pod-spec citation: %+v", r)
	}
	gaps := gapText(ct)
	if strings.Contains(gaps, "names a path under it") {
		t.Errorf("the mount the command reads is reported unread:\n%s", gaps)
	}
	for _, want := range []string{
		cfDeployment + ": pod spec spec.template.spec.containers[0:app].env[0:LOG_FILE].value names /var/log/billing/app.log, read by container app: /var/log/billing/app.log is under no volume mount",
	} {
		if !strings.Contains(gaps, want) {
			t.Errorf("gaps lack %q:\n%s", want, gaps)
		}
	}
	if strings.Contains(gaps, "ledger.example.dev") || strings.Contains(gaps, "/bin/sh") {
		t.Errorf("a URL or the container's program taken for a file:\n%s", gaps)
	}
	dossier := string(runArtifact(t, f, out, "", domain.ArtifactItemDossier))
	for _, want := range []string{"reads-file: pod spec " + podSpecField + " names /etc/billing/tls/tls.key (key tls.key via mount /etc/billing/tls of container app)", f.deploy.ID + " L20"} {
		if !strings.Contains(dossier, want) {
			t.Errorf("dossier lacks %q", want)
		}
	}
	tr := runArtifact(t, f, out, out.Run.Transcript, "")
	if !bytes.Contains(tr, []byte(`"consumerRules":3`)) {
		t.Fatal("a v3 transcript does not name its consumer rules")
	}

	// Under rules v2 the same pod spec leaves the mount unread.
	d2, _ := v9RunWith(t, f, []Option{WithConsumerRules(relations.ConsumerRulesV2)},
		Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"changeItem": f.item}},
		f.v9Submit("potentially-affected", f.assessment()))
	if g := gapText(decodeTrace(t, d2.Results[0])); !strings.Contains(g, "container app mounts "+cfSecret+" at /etc/billing/tls; no configuration in the snapshot names a path under it") {
		t.Errorf("v2 gaps:\n%s", g)
	}
}

// A v3 recording replays under v3; a v2 recording of the same session
// replays under the v2 its header names, and not against a v3 graph.
func TestReplayPinsConsumerRulesV3(t *testing.T) {
	f := newConsumerFixtureWith(t, configPodSpec)
	steps := []Step{
		{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"changeItem": f.item}},
		f.v9Submit("potentially-affected", f.assessment()),
	}
	replay := func(transcript []byte, opts ...Option) (*Replay, *investigate.AgentOutcome) {
		return replayRun(t, f, transcript, opts...)
	}
	_, v3 := v9RunWith(t, f, nil, steps...)
	tr := runArtifact(t, f, v3, v3.Run.Transcript, "")
	if rep, again := replay(tr); len(rep.Mismatches) != 0 || again.Run.Status != domain.AgentSubmitted || again.Run.Transcript != v3.Run.Transcript {
		t.Fatalf("v3 replay: %+v %s %s", rep.Mismatches, again.Run.Status, again.Run.Reason)
	}

	_, v2 := v9RunWith(t, f, []Option{WithConsumerRules(relations.ConsumerRulesV2)}, steps...)
	tr = runArtifact(t, f, v2, v2.Run.Transcript, "")
	if !bytes.Contains(tr, []byte(`"consumerRules":2`)) {
		t.Fatal("a v2 transcript does not name its consumer rules")
	}
	if rep, again := replay(tr); len(rep.Mismatches) != 0 || again.Run.Status != domain.AgentSubmitted || again.Run.Transcript != v2.Run.Transcript {
		t.Fatalf("v2 replay: %+v %s %s", rep.Mismatches, again.Run.Status, again.Run.Reason)
	}
	// the pin matters: against a v3 graph the dossier, so the prompt digest,
	// differs and the replay stops
	if rep, out := replay(tr, WithConsumerRules(relations.ConsumerRulesV3)); len(rep.Mismatches) == 0 && out.Run.Status == domain.AgentSubmitted {
		t.Fatal("v2 and v3 graphs serve the same session: the fixture does not exercise the rules")
	} else if !strings.Contains(out.Run.Reason, "replay: the transcript was recorded under prompt") {
		t.Fatalf("v2 transcript against v3: %s %s", out.Run.Status, out.Run.Reason)
	}
}
