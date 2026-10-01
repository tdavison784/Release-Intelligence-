package app

// Offline, replay-based tests of App.Drift against the recorded e2e fixture
// (see e2e_test.go): the full path catalog → version listing → exhaustive
// checks → drift.Analyze runs without network, from the cache.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/drift"
)

func TestDriftOfflineReplayNoDrift(t *testing.T) {
	a := newReplayApp(t, fixedNow)
	rep, err := a.Drift(context.Background(), "cert-manager", DriftOptions{
		Versions:     []string{"v1.18.0"},
		BaselinePath: "../../docs/onboarding/checks/cert-manager.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rep.Checked, ",") != "1.18.0" {
		t.Fatalf("checked: %v", rep.Checked)
	}
	if rep.Baseline.Source != drift.BaselineSavedCheck {
		t.Fatalf("baseline: %+v", rep.Baseline)
	}
	// Everything the recording captured passed at v1.18.0 when it was
	// ingested; hosts that were unreachable when recording are unavailable in
	// offline replay: those must be unverifiable, not drift.
	if rep.Summary.Drift != 0 {
		t.Fatalf("replaying a recorded release must not produce drift: %+v", rep.Events)
	}
	if rep.Summary.Unverifiable == 0 {
		t.Fatal("expected unverifiable events for the hosts that were unreachable when recording")
	}
	for _, ev := range rep.Events {
		if ev.Status == drift.StatusUnverifiable && ev.Kind != drift.KindUnverifiable {
			t.Fatalf("unverifiable status with kind %s: %+v", ev.Kind, ev)
		}
	}
}

func TestDriftUnknownProduct(t *testing.T) {
	a := newReplayApp(t, fixedNow)
	if _, err := a.Drift(context.Background(), "nope", DriftOptions{}); err == nil || !strings.Contains(err.Error(), `unknown product "nope"`) {
		t.Fatalf("got %v", err)
	}
}

// A baseline report for a different product is refused.
func TestDriftBaselineProductMismatch(t *testing.T) {
	a := newReplayApp(t, fixedNow)
	_, err := a.Drift(context.Background(), "cert-manager", DriftOptions{
		Versions:     []string{"v1.18.0"},
		BaselinePath: "../../docs/onboarding/checks/istio.json",
	})
	if err == nil || !strings.Contains(err.Error(), "report is for product") {
		t.Fatalf("got %v", err)
	}
}

// A product whose canonical channel cannot even be listed yields a report
// (not an error), classifying the versions sources honestly: unreachable is
// unverifiable, never drift.
func TestDriftVersionsSourceUnreachable(t *testing.T) {
	dir := t.TempDir()
	def := `apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: unreachable
name: Unreachable
versioning: {scheme: semver, tagPrefix: v}
sources:
  - id: tags
    roles: [versions]
    locator: {kind: git-tags, repository: github.com/example/unreachable}
artifacts:
  - id: image
    type: container-image
    name: unreachable
    version: {strategy: template, template: "{{.Tag}}"}
    channels:
      - {kind: oci, repository: docker.io/example/unreachable}
`
	if err := os.WriteFile(filepath.Join(dir, "unreachable.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
	// Offline with an empty cache and an empty PATH: git cannot run, so the
	// versions source is unavailable.
	t.Setenv("PATH", t.TempDir())
	a, err := New(Config{ProductsDir: dir, StateDir: t.TempDir(), Offline: true, Now: func() time.Time { return fixedNow }})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := a.Drift(context.Background(), "unreachable", DriftOptions{})
	if err != nil {
		t.Fatalf("an unreachable canonical channel is a report, not an error: %v", err)
	}
	if rep.Summary.Drift != 0 {
		t.Fatalf("unreachable must not be drift: %+v", rep.Events)
	}
	found := false
	for _, ev := range rep.Events {
		if ev.Subject == "tags" && ev.Kind == drift.KindUnverifiable && ev.Status == drift.StatusUnverifiable {
			found = true
		}
	}
	if !found {
		t.Fatalf("events: %+v", rep.Events)
	}
}
