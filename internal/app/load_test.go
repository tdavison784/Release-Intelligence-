package app

import (
	"fmt"
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

func loadTestProductsDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A malformed product file is logged by New, does not hide the other
// products, and is named when its product is requested.
func TestNewToleratesMalformedProductFile(t *testing.T) {
	dir := loadTestProductsDir(t, map[string]string{
		"good.yaml": goodProduct,
		"bad.yaml":  "id: [unterminated\n",
	})
	var logs []string
	a, err := New(Config{
		ProductsDir: dir, StateDir: t.TempDir(), Offline: true,
		Logf: func(f string, args ...any) { logs = append(logs, fmt.Sprintf(f, args...)) },
	})
	if err != nil {
		t.Fatalf("New must not fail because of one bad file: %v", err)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], filepath.Join(dir, "bad.yaml")) || !strings.Contains(logs[0], "not loaded") {
		t.Fatalf("load errors must be logged once per file: %q", logs)
	}
	if _, err := a.Product("good"); err != nil {
		t.Fatalf("the valid product must stay usable: %v", err)
	}
	_, err = a.Product("bad")
	if err == nil || !strings.Contains(err.Error(), `unknown product "bad"`) || !strings.Contains(err.Error(), "known: good") ||
		!strings.Contains(err.Error(), "failed to load") || !strings.Contains(err.Error(), filepath.Join(dir, "bad.yaml")) {
		t.Fatalf("the error for an absent id must mention the file that failed to load: %v", err)
	}
}

func TestProductUnknownWithoutLoadErrors(t *testing.T) {
	a, err := New(Config{ProductsDir: loadTestProductsDir(t, map[string]string{"good.yaml": goodProduct}), StateDir: t.TempDir(), Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Product("nope")
	if err == nil || err.Error() != `unknown product "nope" (known: good)` {
		t.Fatalf("got %v", err)
	}
}
