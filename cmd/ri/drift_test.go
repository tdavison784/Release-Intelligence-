package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/drift"
)

const driftProduct = `apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: driftme
name: Drift Me
versioning: {scheme: semver, tagPrefix: v}
sources:
  - id: tags
    roles: [versions]
    locator: {kind: git-tags, repository: github.com/example/driftme}
artifacts:
  - id: image
    type: container-image
    name: driftme
    version: {strategy: template, template: "{{.Tag}}"}
    channels:
      - {kind: oci, repository: docker.io/example/driftme}
`

func driftProductsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "driftme.yaml"), []byte(driftProduct), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Offline with an empty cache and no git on PATH: the canonical channel is
// unreachable, which is a report (unverifiable, exit 0), not a failure.
func TestDriftCommandUnverifiableIsNotAnError(t *testing.T) {
	dir := driftProductsDir(t)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("RI_STATE", t.TempDir())
	out, _, err := runCLI(t, "-products", dir, "-offline", "drift", "driftme")
	if err != nil {
		t.Fatalf("unverifiable must not exit non-zero: %v\n%s", err, out)
	}
	for _, want := range []string{"Unverifiable", "not drift", "tags"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}

	// JSON output is the report.
	out, _, err = runCLI(t, "-products", dir, "-offline", "drift", "driftme", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var rep drift.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if rep.Product != "driftme" || rep.Summary.Unverifiable == 0 || rep.Summary.Drift != 0 {
		t.Fatalf("report: %+v", rep.Summary)
	}
}

func TestDriftCommandUsage(t *testing.T) {
	dir := driftProductsDir(t)
	_, _, err := runCLI(t, "-products", dir, "drift")
	if err == nil || !strings.Contains(err.Error(), "expected <product>") {
		t.Fatalf("got %v", err)
	}
	if _, _, err := runCLI(t, "-products", dir, "drift", "unknown-product", "-baseline", "-"); err == nil ||
		!strings.Contains(err.Error(), `unknown product "unknown-product"`) {
		t.Fatalf("got %v", err)
	}
}

// The proposal writer never writes into the products directory.
func TestWriteProposalRefusesProductsDir(t *testing.T) {
	dir := driftProductsDir(t)
	inside := dir + "/proposed.yaml"
	if err := writeProposal(dir, inside, "artifacts: []"); err == nil ||
		!strings.Contains(err.Error(), "refusing to write the proposal into the products directory") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(inside); err == nil {
		t.Fatal("nothing must have been written")
	}
	outside := filepath.Join(t.TempDir(), "proposed.yaml")
	if err := writeProposal(dir, outside, "artifacts: []"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(outside)
	if err != nil || string(b) != "artifacts: []" {
		t.Fatalf("proposal file: %v %q", err, b)
	}
	// an empty proposal writes nothing, wherever it points
	if err := writeProposal(dir, inside, ""); err != nil {
		t.Fatal(err)
	}
}
