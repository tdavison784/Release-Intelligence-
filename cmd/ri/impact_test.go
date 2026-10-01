package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// TestImpactCommand drives `ri impact` end to end against the offline
// recording, the way a user would, and checks both output formats.
func TestImpactCommand(t *testing.T) {
	state := recordedState(t)
	env := filepath.Join("..", "..", "internal", "app", "testdata", "e2e", "env", "cert-manager")
	base := []string{
		"-products", filepath.Join("..", "..", "products"), "-offline", "-state", state,
		"impact", "cert-manager", "v1.17.0", "v1.18.0",
		"--kubernetes", "1.28",
		"--values", filepath.Join(env, "values.yaml"),
		"--manifests", filepath.Join(env, "manifests"),
		"--crds", filepath.Join(env, "crds"),
		"--images", filepath.Join(env, "images.txt"),
	}

	out, _, err := runCLI(t, base...)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"cert-manager v1.17.0 → v1.18.0 — impact on this environment",
		// the funnel states every verdict class explicitly, unknowns included
		"51 upstream changes analyzed",
		"ACTION REQUIRED:    1",
		"REVIEW REQUIRED:    3",
		"INFORMATIONAL:    1",
		"NOT AFFECTED:    3",
		"UNKNOWN:   50",
		"Action required (1)",
		"Cluster Kubernetes 1.28 is below the supported range 1.29–1.33 of v1.18.0",
		"environment: values-key prometheus.servicemonitor.targetPort",
		"upstream evidence: ev-",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// NOT AFFECTED is counted but not listed per finding without the flag
	if strings.Contains(out, "Not affected (") {
		t.Errorf("not-affected findings are verbose-only:\n%s", out)
	}

	out, _, err = runCLI(t, append(base, "-o", "json")...)
	if err != nil {
		t.Fatal(err)
	}
	var rep domain.ImpactReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if err := rep.Validate(); err != nil {
		t.Fatal(err)
	}
	if rep.Summary.AffectEnvironment != 5 || rep.Summary.ActionRequired != 1 {
		t.Errorf("summary = %+v", rep.Summary)
	}

	out, _, err = runCLI(t, append(base, "--show-not-affected")...)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Not affected (3)",
		"checked: images, 2 fact(s)",
		"checked: cluster-version (kubernetes), 1 fact(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--show-not-affected output missing %q:\n%s", want, out)
		}
	}
}

// TestImpactRequiresEnvironmentInput: the join without an environment is a
// usage error, reported before any ingestion work.
func TestImpactRequiresEnvironmentInput(t *testing.T) {
	_, _, err := runCLI(t, "-products", t.TempDir(), "-state", t.TempDir(), "impact", "x", "1.0.0", "2.0.0")
	if err == nil || !strings.Contains(err.Error(), "at least one environment input") {
		t.Fatalf("got %v", err)
	}
}

func TestImageList(t *testing.T) {
	dir := t.TempDir()
	list := filepath.Join(dir, "images.txt")
	if err := writeFileForTest(t, list, "# comment\na.io/x:1\n\nb.io/y:2\n"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"", ""},
		{"a.io/x:1", "a.io/x:1"},
		{"a.io/x:1,b.io/y:2", "a.io/x:1|b.io/y:2"},
		{list, "a.io/x:1|b.io/y:2"},
	} {
		got, err := imageList(tc.in)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, "|") != tc.want {
			t.Errorf("imageList(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func writeFileForTest(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0o644)
}
