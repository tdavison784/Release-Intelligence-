package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/app"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for n, b := range files {
		p := filepath.Join(dir, n)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestKnowledgeTreeDigest(t *testing.T) {
	a := writeTree(t, map[string]string{"x/a.json": "{}", "b.json": "[]"})
	b := writeTree(t, map[string]string{"b.json": "[]", "x/a.json": "{}"})
	da, err := knowledgeTreeDigest(a)
	if err != nil {
		t.Fatal(err)
	}
	if db, _ := knowledgeTreeDigest(b); da != db {
		t.Fatalf("same content, different digest: %s vs %s", da, db)
	}
	for name, files := range map[string]map[string]string{
		"content changed": {"b.json": "[1]", "x/a.json": "{}"},
		"file renamed":    {"c.json": "[]", "x/a.json": "{}"},
		"file added":      {"b.json": "[]", "x/a.json": "{}", "x/new.json": "{}"},
		"boundary shift":  {"b.json": "[]{}", "x/a.json": ""},
	} {
		d, _ := knowledgeTreeDigest(writeTree(t, files))
		if d == da {
			t.Errorf("%s: digest unchanged", name)
		}
	}
	if d, err := knowledgeTreeDigest(""); err != nil || d != noKnowledgeDigest {
		t.Fatalf("empty = %q %v", d, err)
	}
	if _, err := knowledgeTreeDigest(filepath.Join(a, "missing")); err == nil {
		t.Fatal("a missing directory must be an error, not an empty digest")
	}
}

func TestCheckEvalKnowledgeRefusesImplicitAndLive(t *testing.T) {
	live := writeTree(t, map[string]string{"f.json": "{}"})
	snap := writeTree(t, map[string]string{"f.json": "{}"})
	roots := []string{live}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(live, link); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		explicit bool
		dir      string
		allow    bool
		refuse   bool
	}{
		{"default knowledge", false, "knowledge", false, true},
		{"explicit snapshot", true, snap, false, false},
		{"explicit live tree", true, live, false, true},
		{"live via symlink", true, link, false, true},
		{"live allowed", true, live, true, false},
		{"default allowed", false, "knowledge", true, false},
		{"knowledge switched off", true, "", false, false},
	} {
		err := checkEvalKnowledge(tc.explicit, tc.dir, tc.allow, roots)
		if (err != nil) != tc.refuse {
			t.Errorf("%s: err = %v, want refuse=%v", tc.name, err, tc.refuse)
		}
		if err != nil && !errors.Is(err, app.ErrUsage) {
			t.Errorf("%s: want a usage error, got %v", tc.name, err)
		}
	}
}
