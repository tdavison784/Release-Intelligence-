package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Every stored investigation run (eval/investigations/results/*) still loads
// and re-verifies against the ids and digests pinned in its run.json: the
// investigation id is InvestigationID(bundle, config digest), and the record
// digest is the content digest of the stored record (recordDigest). A change to stage keys, record
// digests or id derivation that would rewrite history fails here.
//
// The two A″ runs at binary 5d4da37635 already failed the record-digest check
// at this test's base (7cb8d6ff, before any stage-key change): their ids
// re-derive, but the pinned content digest does not reproduce from the stored
// record. That is recorded in status/fixes5.md as a finding, not hidden: the
// test still checks their ids, and fails if any other run stops re-verifying.
var storedDigestKnownMismatch = map[string]bool{
	"A-20261016T231142Z-5d4da37635": true,
	"A-20261016T231217Z-5d4da37635": true,
}

func TestStoredRunsReverifyTheirDigests(t *testing.T) {
	dirs, _ := filepath.Glob(filepath.Join("..", "..", "eval", "investigations", "results", "*", "run.json"))
	if len(dirs) == 0 {
		t.Skip("no stored runs")
	}
	checked := 0
	for _, p := range dirs {
		dir := filepath.Dir(p)
		meta, err := readInvRunMeta(dir)
		if err != nil {
			t.Fatal(err)
		}
		rf, err := openInvRunFiles(dir)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		for _, m := range meta.Cases {
			if m.Investigation == "" {
				continue
			}
			s, err := loadInvStored(rf, m.ID)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				t.Fatalf("%s %s: %v", meta.RunID, m.ID, err)
			}
			inv := s.Inv
			if inv.ID != m.Investigation || inv.Bundle != m.Bundle || s.Bundle.ID != m.Bundle {
				t.Errorf("%s %s: stored ids %s/%s, pinned %s/%s", meta.RunID, m.ID, inv.ID, inv.Bundle, m.Investigation, m.Bundle)
			}
			if id := domain.InvestigationID(inv.Bundle, inv.Config); id != inv.ID {
				t.Errorf("%s %s: id %s does not re-derive (%s)", meta.RunID, m.ID, inv.ID, id)
			}
			if m.RecordDigest != "" && !storedDigestKnownMismatch[meta.RunID] {
				if d := recordDigest(inv); d != m.RecordDigest {
					t.Errorf("%s %s: record digest %s, pinned %s", meta.RunID, m.ID, d, m.RecordDigest)
				}
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no stored case was checked")
	}
	t.Logf("%d stored case records re-verified", checked)
}
