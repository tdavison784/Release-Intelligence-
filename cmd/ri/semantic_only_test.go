package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func mkCand(id string) domain.SemanticCandidate {
	return domain.SemanticCandidate{ID: id}
}

func TestFilterCandidates(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "only.txt")
	list := "# the with-render set\nsc-two\nsc-one\nsc-one\n\nsc-missing\n"
	if err := os.WriteFile(file, []byte(list), 0o644); err != nil {
		t.Fatal(err)
	}
	cands := []domain.SemanticCandidate{mkCand("sc-one"), mkCand("sc-two"), mkCand("sc-other")}
	kept, missed, err := filterCandidates(cands, file)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 2 || string(kept[0].ID) != "sc-two" || string(kept[1].ID) != "sc-one" {
		t.Fatalf("kept = %+v, want [sc-two sc-one] in file order", kept)
	}
	if len(missed) != 1 || missed[0] != "sc-missing" {
		t.Fatalf("missed = %v, want [sc-missing]", missed)
	}
}

func TestFilterCandidatesNoFile(t *testing.T) {
	if _, _, err := filterCandidates(nil, filepath.Join(t.TempDir(), "absent.txt")); err == nil {
		t.Fatal("want an error for a missing -only file")
	}
}
