package main

// `ri eval` end to end against the offline recording (the cert-manager case
// of the real dataset), plus the regression-mechanics a CI run relies on:
// -update writes snapshots, a stored snapshot is compared, a regression
// fails the command, an improvement does not.

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

func TestEvalCommandTextAndJSON(t *testing.T) {
	state := recordedState(t)
	dataset := filepath.Join("..", "..", "eval")

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
}

func TestEvalCommandUnknownEntry(t *testing.T) {
	_, _, err := runCLI(t, evalArgs(t.TempDir(), filepath.Join("..", "..", "eval"), "no-such-entry")...)
	if err == nil || !strings.Contains(err.Error(), "no-such-entry") {
		t.Fatalf("got %v", err)
	}
}

func TestEvalRegressionMechanics(t *testing.T) {
	state := recordedState(t)
	dataset := t.TempDir()
	// a minimal one-entry dataset with an expectation the recording surely
	// covers (rotationPolicy) and one it surely does not (consul)
	cdir := filepath.Join(dataset, "cases", "tiny")
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		t.Fatal(err)
	}
	caseYAML := `id: tiny
product: cert-manager
from: v1.17.0
to: v1.18.0
expected:
  - {id: E1, title: rotation policy, kind: behaviour-change, importance: critical, match: [{text: '(?i)rotationPolicy'}]}
  - {id: E2, title: consul, kind: removal, importance: minor, match: [{text: '(?i)consul'}]}
`
	if err := os.WriteFile(filepath.Join(cdir, "case.yaml"), []byte(caseYAML), 0o644); err != nil {
		t.Fatal(err)
	}

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
