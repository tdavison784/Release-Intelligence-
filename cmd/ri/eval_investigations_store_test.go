package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/eval"
)

func TestInvArchiveDeterministicRoundTrip(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{invCasePath("b", "x.json"): []byte("{}\n"), invCasePath("a", "y.json"): bytes.Repeat([]byte("z"), 5000)}
	p1, p2 := filepath.Join(dir, "1.tar.gz"), filepath.Join(dir, "2.tar.gz")
	if err := writeInvArchive(p1, files); err != nil {
		t.Fatal(err)
	}
	if err := writeInvArchive(p2, files); err != nil {
		t.Fatal(err)
	}
	b1, _ := os.ReadFile(p1)
	b2, _ := os.ReadFile(p2)
	if !bytes.Equal(b1, b2) {
		t.Fatal("same files must give the same archive bytes")
	}
	back, err := readInvArchive(p1)
	if err != nil || len(back) != 2 || !bytes.Equal(back[invCasePath("a", "y.json")], files[invCasePath("a", "y.json")]) {
		t.Fatalf("round trip: %v %v", err, back)
	}
}

func TestPackLegacyRunAndReadBothLayouts(t *testing.T) {
	dir := t.TempDir()
	inv := []byte(`{"id":"inv-1"}` + "\n")
	if err := os.MkdirAll(filepath.Join(dir, "cases", "c1"), 0o755); err != nil {
		t.Fatal(err)
	}
	must := func(p string, b []byte) {
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(dir, "cases", "c1", "investigation.json"), inv)
	must(filepath.Join(dir, "run.json"), mustJSONBytes(t, InvRunMeta{RunID: "A-x", Arm: "A"}))
	must(filepath.Join(dir, "score.json"), mustJSONBytes(t, eval.InvScore{}))

	legacy, err := openInvRunFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := legacy.read("c1", "investigation.json"); err != nil || !bytes.Equal(got, inv) {
		t.Fatalf("legacy read: %v", err)
	}
	if err := packInvRun(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cases")); !os.IsNotExist(err) {
		t.Fatal("cases/ must be removed after packing")
	}
	if _, err := os.Stat(filepath.Join(dir, "score.json")); !os.IsNotExist(err) {
		t.Fatal("plain score.json must be replaced by score.json.gz")
	}
	if raw, err := readInvScore(dir); err != nil || !bytes.Equal(raw, mustJSONBytes(t, eval.InvScore{})) {
		t.Fatalf("compressed score differs: %v", err)
	}
	for _, n := range []string{invCasesArchive, invScoreFile, "gates.json", "run.json"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Fatalf("%s missing after pack: %v", n, err)
		}
	}
	packed, err := openInvRunFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := packed.read("c1", "investigation.json"); err != nil || !bytes.Equal(got, inv) {
		t.Fatalf("packed read differs: %v", err)
	}
	if _, err := packed.read("c1", "transcript.jsonl"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file must be ErrNotExist, got %v", err)
	}
	if err := packInvRun(dir); err == nil {
		t.Fatal("packing twice must fail (nothing to pack)")
	}
}

func TestSaveInvRunArchiveLoads(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	s := &invStored{Inv: &domain.Investigation{ID: "inv-1"}, Bundle: &domain.EvidenceBundle{ID: "bnd-1"}, ChangeText: map[string]string{"c": "d"},
		Graph: &domain.RelationshipGraph{ID: "grf-1"}, Transcript: []byte("{\"type\":\"session\"}\n")}
	if err := saveInvRun(dir, InvRunMeta{RunID: "A-x"}, &eval.InvScore{}, map[string]*invStored{"case-1": s}, []byte("report\n"),
		map[string][]byte{invCasePath("case-0", "investigation.json"): []byte("{}\n")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cases")); !os.IsNotExist(err) {
		t.Fatal("new runs store no uncompressed cases/ directory")
	}
	if _, err := readInvScore(dir); err != nil {
		t.Fatalf("score.json.gz: %v", err)
	}
	rf, err := openInvRunFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := rf.read("case-0", "investigation.json"); err != nil || string(raw) != "{}\n" {
		t.Fatalf("earlier case records must be carried over unchanged: %v", err)
	}
	got, err := loadInvStored(rf, "case-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Inv.ID != "inv-1" || got.Bundle.ID != "bnd-1" || got.Graph == nil || got.Graph.ID != "grf-1" || got.Report != nil ||
		got.ChangeText["c"] != "d" || !bytes.Equal(got.Transcript, s.Transcript) {
		t.Fatalf("stored case does not load back: %+v", got)
	}
}

func mustJSONBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, err := invFileBytes(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
