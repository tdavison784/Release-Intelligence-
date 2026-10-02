package impact

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// rendered builds a report from the standard fixtures and renders it.
func rendered(t *testing.T, opts RenderOptions) string {
	t.Helper()
	eb := newEdge()
	eb.change("values:removed", "Helm value `webhook.config` removed", "webhook.config")
	eb.change(upgrade.RuleValuesDefaultChanged, "Default of `replicaCount` changed: 1 → 3", "replicaCount")
	eb.change("section:/breaking/i", "A declared change without a comparable subject")
	eb.constraint("supported", "1.29, 1.30, 1.31")
	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", "webhook:\n  config: true\nreplicaCount: 5\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}, KubernetesVersion: "1.31"}))
	var buf bytes.Buffer
	if err := RenderText(&buf, e, opts); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestRenderText(t *testing.T) {
	out := rendered(t, RenderOptions{})
	for _, want := range []string{
		"Example v1.0.0 → v1.1.0 — impact on this environment",
		// the funnel: every upstream change analyzed, every verdict counted
		"3 upstream changes analyzed",
		"ACTION REQUIRED:   1",
		"REVIEW REQUIRED:   0",
		"INFORMATIONAL:   2",
		"NOT AFFECTED:   0",
		"UNKNOWN:   1",
		"Action required (1)",
		"You set `webhook.config`, which v1.1.0 removed",
		"upstream change: Helm value `webhook.config` removed (other)",
		"environment: values-key webhook.config",
		"upstream evidence: ev-",
		"Environment",
		"kubernetes 1.31",
		"values: 2 keys set",
		"Unknown — insufficient evidence (1)",
		// the collapsed UNKNOWN view: one line per missing-evidence family,
		// not one block per item
		"1 × note-derived changes the deterministic join cannot compare",
		"run -enrich for AI suggestions on these",
		"1 total — render with --show-unknown to list every item",
		// inline short-form citations in the why-blocks
		"evidence: upstream: compat L1-L9 · environment: flag:--kubernetes",
		"Environment",
		"kubernetes 1.31",
		"values: 2 keys set",
		"Evidence",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in rendered report:\n%s", want, out)
		}
	}
	// the collapsed view must not list the raw unknown item
	for _, absent := range []string{"Review required (", "Not affected (", "missing: no machine-comparable subject"} {
		if strings.Contains(out, absent) {
			t.Errorf("must be omitted from the collapsed view:\n%s", out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("colour must be off by default")
	}

	// --show-unknown lists the items again
	expanded := rendered(t, RenderOptions{ShowUnknown: true})
	for _, want := range []string{
		"Not evaluated for this environment",
		"missing: no machine-comparable subject",
	} {
		if !strings.Contains(expanded, want) {
			t.Errorf("--show-unknown missing %q:\n%s", want, expanded)
		}
	}
}

func TestRenderNotAffectedOnlyInVerbose(t *testing.T) {
	eb := newEdge()
	eb.change("values:removed", "Helm value `webhook.config` removed", "webhook.config")
	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", "unrelated: 1\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}}))

	var quiet, verbose bytes.Buffer
	if err := RenderText(&quiet, e, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := RenderText(&verbose, e, RenderOptions{ShowNotAffected: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(quiet.String(), "NOT AFFECTED:   1") {
		t.Errorf("the summary always counts not-affected:\n%s", quiet.String())
	}
	if strings.Contains(quiet.String(), "Not affected (") {
		t.Errorf("not-affected findings are verbose-only:\n%s", quiet.String())
	}
	for _, want := range []string{"Not affected (1)", "Your values do not set `webhook.config`", "checked: values, 1 fact(s)"} {
		if !strings.Contains(verbose.String(), want) {
			t.Errorf("verbose mode missing %q:\n%s", want, verbose.String())
		}
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
