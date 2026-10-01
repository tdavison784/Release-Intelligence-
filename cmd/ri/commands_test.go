package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodProduct = `apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: good
name: Good
versioning: {scheme: semver, tagPrefix: v, lineage: minor}
sources:
  - id: tags
    roles: [versions]
    locator: {kind: git-tags, repository: github.com/example/good}
`

// productsDirWithBadFile returns a products directory holding one valid
// definition and one that does not decode.
func productsDirWithBadFile(t *testing.T) (dir, bad string) {
	t.Helper()
	dir = t.TempDir()
	bad = filepath.Join(dir, "bad.yaml")
	for p, content := range map[string]string{
		filepath.Join(dir, "good.yaml"): goodProduct,
		bad:                             "id: [unterminated\n",
	} {
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, bad
}

func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err = run(context.Background(), args, &out, &errOut)
	return out.String(), errOut.String(), err
}

func TestProductsReportsLoadErrorsPerFile(t *testing.T) {
	dir, bad := productsDirWithBadFile(t)
	out, _, err := runCLI(t, "-products", dir, "products")
	if err != nil {
		t.Fatalf("products lists what it can: %v", err)
	}
	if !strings.Contains(out, "good") || !strings.Contains(out, "1 product file(s) could not be loaded") || !strings.Contains(out, "✗ "+bad+":") {
		t.Fatalf("output:\n%s", out)
	}

	// JSON keeps stdout a clean array; the failure goes to stderr.
	out, errOut, err := runCLI(t, "-products", dir, "products", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var defs []map[string]any
	if err := json.Unmarshal([]byte(out), &defs); err != nil || len(defs) != 1 || defs[0]["id"] != "good" {
		t.Fatalf("stdout must be the JSON array of loaded products: %v\n%s", err, out)
	}
	if !strings.Contains(errOut, bad) {
		t.Fatalf("stderr must name the file that failed to load: %q", errOut)
	}
}

func TestValidateFailsOnLoadErrors(t *testing.T) {
	dir, bad := productsDirWithBadFile(t)
	out, _, err := runCLI(t, "-products", dir, "validate")
	if err == nil || !strings.Contains(err.Error(), "1 definition(s) invalid") {
		t.Fatalf("validate must exit non-zero: %v", err)
	}
	if !strings.Contains(out, "✓ valid  good") || !strings.Contains(out, "✗ invalid  (could not be loaded) ("+bad+")") || !strings.Contains(out, "error file:") {
		t.Fatalf("each file is reported:\n%s", out)
	}

	out, _, err = runCLI(t, "-products", dir, "validate", "-o", "json")
	if err == nil {
		t.Fatal("validate -o json must exit non-zero too")
	}
	var reps []struct {
		Product string `json:"product"`
		File    string `json:"file"`
		Issues  []struct {
			Severity string `json:"severity"`
			Message  string `json:"message"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &reps); err != nil || len(reps) != 2 {
		t.Fatalf("json: %v\n%s", err, out)
	}
	failed := 0
	for _, r := range reps {
		if r.File == bad && r.Product == "" && len(r.Issues) == 1 && r.Issues[0].Severity == "error" && r.Issues[0].Message != "" {
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("load failure missing from the JSON report: %+v", reps)
	}

	// An explicit file target that does not load is reported, not fatal to the others.
	out, _, err = runCLI(t, "-products", dir, "validate", filepath.Join(dir, "good.yaml"), bad)
	if err == nil || !strings.Contains(out, "✓ valid  good") || !strings.Contains(out, "(could not be loaded)") {
		t.Fatalf("file targets: %v\n%s", err, out)
	}

	// A healthy directory still validates cleanly.
	if err := os.Remove(bad); err != nil {
		t.Fatal(err)
	}
	if out, _, err := runCLI(t, "-products", dir, "validate"); err != nil || !strings.Contains(out, "✓ valid  good") {
		t.Fatalf("clean directory: %v\n%s", err, out)
	}
}

func TestValidateUnknownProductMentionsLoadErrors(t *testing.T) {
	dir, _ := productsDirWithBadFile(t)
	_, _, err := runCLI(t, "-products", dir, "validate", "bad")
	if err == nil || !strings.Contains(err.Error(), `unknown product "bad"`) || !strings.Contains(err.Error(), "failed to load") {
		t.Fatalf("got %v", err)
	}
}
