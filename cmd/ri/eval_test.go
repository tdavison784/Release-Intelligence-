package main

// `ri eval` end to end against the offline recording (the cert-manager case
// of the real dataset), plus the regression-mechanics and gate-mechanics a
// CI run relies on: -update writes snapshots, a stored snapshot is compared,
// a regression fails the command, an improvement does not; a failed hard
// gate fails the command; a permissive gates.yaml does not.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/eval"
)

func evalArgs(state, dataset string, extra ...string) []string {
	base := []string{
		"-products", filepath.Join("..", "..", "products"),
		"-offline", "-state", state,
		"eval", "-dir", dataset,
	}
	return append(base, extra...)
}

// permissiveGates writes a gates.yaml that cannot fail (thresholds the
// recording satisfies), so mechanics tests isolate the mechanic under test
// from the dataset's real gate outcomes. The real eval/gates.yaml carries the
// pre-registered thresholds and is deliberately NOT used here: gate failures
// on the honest numbers are findings, not test fixtures.
func permissiveGates(t *testing.T, dir string) {
	t.Helper()
	g := "criticalRecall: 0.0\nimportantRecall: 0.0\napplicabilityAccuracy: 0.0\n" +
		"falseActionRate: 1.0\nactionFindingEvidence: 0.0\nunsupportedMax: 1000000\npipelineFailuresMax: 1000000\n"
	if err := os.WriteFile(filepath.Join(dir, "gates.yaml"), []byte(g), 0o644); err != nil {
		t.Fatal(err)
	}
}

// copyCase copies one dataset entry (case.yaml + environment/) into a fresh
// dataset root.
func copyCase(t *testing.T, src, dataset string) {
	t.Helper()
	dst := filepath.Join(dataset, "cases", filepath.Base(src))
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	returned := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if returned != nil {
		t.Fatal(returned)
	}
}

func TestEvalCommandTextAndJSON(t *testing.T) {
	state := recordedState(t)
	dataset := t.TempDir()
	copyCase(t, filepath.Join("..", "..", "eval", "cases", "cert-manager-1.17-1.18"), dataset)
	permissiveGates(t, dataset)

	out, _, err := runCLI(t, evalArgs(state, dataset, "cert-manager-1.17-1.18")...)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"── cert-manager-1.17-1.18 ─ cert-manager v1.17.0 → v1.18.0",
		"recall ", "precision ",
		"environment:",
		"── aggregate ──",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	out, _, err = runCLI(t, evalArgs(state, dataset, "cert-manager-1.17-1.18", "-o", "json")...)
	if err != nil {
		t.Fatal(err)
	}
	var rep eval.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) != 1 || rep.Aggregate.Entries != 1 {
		t.Fatalf("report = %d results", len(rep.Results))
	}
	if rep.Results[0].Error != "" {
		t.Fatalf("entry errored: %s", rep.Results[0].Error)
	}
	if rep.Aggregate.Expected == 0 || rep.Aggregate.Changes == 0 {
		t.Errorf("aggregate = %+v", rep.Aggregate)
	}
	if len(rep.Gates) == 0 {
		t.Errorf("no gate panel in the report")
	}
}

func TestEvalCommandUnknownEntry(t *testing.T) {
	_, _, err := runCLI(t, evalArgs(t.TempDir(), filepath.Join("..", "..", "eval"), "no-such-entry")...)
	if err == nil || !strings.Contains(err.Error(), "no-such-entry") {
		t.Fatalf("got %v", err)
	}
}

// tinyCaseYAML is a minimal one-entry dataset body: an expectation the
// recording surely covers (rotationPolicy) and one it surely does not (consul).
const tinyCaseYAML = `id: tiny
product: cert-manager
from: v1.17.0
to: v1.18.0
expected:
  - {id: E1, title: rotation policy, kind: behaviour-change, importance: critical, match: [{text: '(?i)rotationPolicy'}]}
  - {id: E2, title: consul, kind: removal, importance: minor, match: [{text: '(?i)consul'}]
}
`

func writeTinyDataset(t *testing.T) string {
	t.Helper()
	dataset := t.TempDir()
	cdir := filepath.Join(dataset, "cases", "tiny")
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cdir, "case.yaml"), []byte(tinyCaseYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return dataset
}

func TestEvalRegressionMechanics(t *testing.T) {
	state := recordedState(t)
	dataset := writeTinyDataset(t)
	permissiveGates(t, dataset)

	// no snapshot yet: plain run, exit 0, no diff section
	out, _, err := runCLI(t, evalArgs(state, dataset, "tiny")...)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "vs stored results") {
		t.Fatalf("unexpected diff section without snapshots:\n%s", out)
	}

	// a tampered snapshot (E1 missed) makes the fresh run a regression:
	// exit non-zero and the diff names the field
	bad := eval.StoredResult{CaseID: "tiny", Metrics: eval.Metrics{Expected: 2, Found: 1, MissedCritical: 1}, MissedIDs: []string{"E1"}}
	if err := eval.WriteStored(dataset, bad); err != nil {
		t.Fatal(err)
	}
	out, _, err = runCLI(t, evalArgs(state, dataset, "tiny")...)
	if err == nil {
		t.Fatalf("regression must fail the command:\n%s", out)
	}
	if !strings.Contains(out, "✗ tiny missedMinor: 0 → 1 ← worse") || !strings.Contains(out, "REGRESSION") {
		t.Fatalf("diff not reported:\n%s", out)
	}

	// -update rewrites the snapshot from the fresh run (never the case)
	_, errOut, err := runCLI(t, evalArgs(state, dataset, "tiny", "-update")...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "updated stored results") {
		t.Errorf("stderr: %s", errOut)
	}
	good, err := eval.LoadStored(dataset, "tiny")
	if err != nil || good == nil {
		t.Fatal(err)
	}
	if good.Metrics.Found != 1 || len(good.MissedIDs) != 1 || good.MissedIDs[0] != "E2" {
		t.Errorf("stored = %+v", good)
	}

	// now the run compares clean: exit 0, no regression line
	out, _, err = runCLI(t, evalArgs(state, dataset, "tiny")...)
	if err != nil {
		t.Fatalf("clean comparison must pass:\n%s", out)
	}
	if !strings.Contains(out, "no regressions against eval/results") {
		t.Errorf("clean comparison output:\n%s", out)
	}
}

// TestEvalGateMechanics pins the hard-gate contract: the panel is rendered,
// a failed gate exits non-zero, and a satisfied gate does not.
func TestEvalGateMechanics(t *testing.T) {
	state := recordedState(t)
	dataset := writeTinyDataset(t)
	permissiveGates(t, dataset)

	// permissive gates: the panel shows every gate passing
	out, _, err := runCLI(t, evalArgs(state, dataset, "tiny")...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "── gates (pre-registered thresholds") {
		t.Fatalf("gate panel missing:\n%s", out)
	}
	if strings.Contains(out, "✗ ") && strings.Contains(out, "gates") && !strings.Contains(out, "✓ criticalRecall") {
		t.Fatalf("unexpected gate failure under permissive thresholds:\n%s", out)
	}

	// now demand perfect recall on both importances: the tiny case finds its
	// one critical item (passes), and its zero important items make the
	// important gate a vacuous pass (documented as proving nothing)…
	strict := "criticalRecall: 1.0\nimportantRecall: 1.0\nunsupportedMax: 1000000\npipelineFailuresMax: 1000000\n" +
		"applicabilityAccuracy: 0.0\nfalseActionRate: 1.0\nactionFindingEvidence: 0.0\n"
	if err := os.WriteFile(filepath.Join(dataset, "gates.yaml"), []byte(strict), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err = runCLI(t, evalArgs(state, dataset, "tiny")...)
	if err != nil {
		t.Fatalf("satisfied gate must not fail the command:\n%s", out)
	}

	// …while an impossible recall threshold (the consul item is a minor
	// miss, so demand 100% overall recall via a failing important gate with
	// an important item added) exits non-zero.
	strictCase := strings.Replace(tinyCaseYAML,
		"{id: E2, title: consul, kind: removal, importance: minor, match: [{text: '(?i)consul'}]\n}",
		"{id: E2, title: consul, kind: removal, importance: important, match: [{text: '(?i)consul'}]\n}", 1)
	if err := os.WriteFile(filepath.Join(dataset, "cases", "tiny", "case.yaml"), []byte(strictCase), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err = runCLI(t, evalArgs(state, dataset, "tiny")...)
	if err == nil {
		t.Fatalf("failed gate must exit non-zero:\n%s", out)
	}
	if !strings.Contains(out, "✗ importantRecall") || !strings.Contains(err.Error(), "hard gate") {
		t.Fatalf("gate failure not reported:\nout=%s err=%v", out, err)
	}
}
