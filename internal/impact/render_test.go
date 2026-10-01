package impact

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// rendered builds a report from the standard fixtures and renders it.
func rendered(t *testing.T) string {
	t.Helper()
	eb := newEdge()
	eb.change("values:removed", "Helm value `webhook.config` removed", "webhook.config")
	eb.change(upgrade.RuleValuesDefaultChanged, "Default of `replicaCount` changed: 1 → 3", "replicaCount")
	eb.constraint("supported", "1.29, 1.30, 1.31")
	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", "webhook:\n  config: true\nreplicaCount: 5\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}, KubernetesVersion: "1.31"}))
	var buf bytes.Buffer
	if err := RenderText(&buf, e, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestRenderText(t *testing.T) {
	out := rendered(t)
	for _, want := range []string{
		"Example v1.0.0 → v1.1.0 — impact on this environment",
		"2 upstream changes · 3 affect this environment · 1 action required · 0 review · 2 informational",
		"Action required (1)",
		"You set `webhook.config`, which v1.1.0 removed",
		"upstream change: Helm value `webhook.config` removed (other)",
		"environment: values-key webhook.config",
		"upstream evidence: ev-",
		"Environment",
		"kubernetes 1.31",
		"values: 2 keys set",
		"Evidence",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in rendered report:\n%s", want, out)
		}
	}
	// no findings section may appear for an empty class
	if strings.Contains(out, "Review (") {
		t.Errorf("empty sections must be omitted:\n%s", out)
	}
	if strings.Contains("\x1b[", out) {
		t.Error("colour must be off by default")
	}
}

func TestRenderTextColor(t *testing.T) {
	eb := newEdge()
	eb.change("values:removed", "Helm value `a` removed", "a")
	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", "a: 1\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}}))
	var buf bytes.Buffer
	if err := RenderText(&buf, e, RenderOptions{Color: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("\x1b[")) {
		t.Error("colour codes expected")
	}
}

func TestRenderNilReport(t *testing.T) {
	if err := RenderText(&bytes.Buffer{}, nil, RenderOptions{}); err == nil {
		t.Fatal("nil report must be an error")
	}
}
