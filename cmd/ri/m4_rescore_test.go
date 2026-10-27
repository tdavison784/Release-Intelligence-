//go:build m4rescore

package main

// M4 lane applicability (PLAN-M4 §5, brief change 5): re-classify every
// stored B0, C1 and D1 run with the current classifier and compare each
// item's class and rule with the stored ones. Classifications must be
// identical, or stricter only. It reads only the runs' cases.tar.gz and
// run.json (records RI produced) — never report.txt, score.json.gz or any
// expected label. Run:
//
//	go test -tags m4rescore -run TestM4Rescore -v ./cmd/ri

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
)

var m4Runs = regexp.MustCompile(`^(B0|C1|D1)-`)

// looser reports whether moving from a to b relaxes the classification: a new
// not-affected, or a lost affected.
func looser(a, b domain.FindingClass) bool {
	return (b == domain.FindingNotAffected && a != domain.FindingNotAffected) || (a == domain.FindingAffected && b != domain.FindingAffected)
}

func TestM4Rescore(t *testing.T) {
	root := "../../eval/investigations/results"
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var runs, cases, items, same int
	transitions := map[string]int{}
	for _, e := range entries {
		if !e.IsDir() || !m4Runs.MatchString(e.Name()) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		var meta InvRunMeta
		raw, err := os.ReadFile(filepath.Join(dir, "run.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &meta); err != nil {
			t.Fatal(err)
		}
		rf, err := openInvRunFiles(dir)
		if err != nil {
			t.Fatal(err)
		}
		runs++
		for _, m := range meta.Cases {
			if m.Error != "" {
				continue
			}
			s, err := loadInvStored(rf, m.ID)
			if err != nil {
				t.Fatalf("%s %s: %v", e.Name(), m.ID, err)
			}
			cases++
			var ag *investigate.AgentOutcome
			if s.Inv.Agent != nil {
				ag = &investigate.AgentOutcome{Run: *s.Inv.Agent, Hypotheses: s.Inv.Hypotheses, Assertions: s.Inv.Assertions,
					EvidenceRequests: s.Inv.EvidenceRequests, ValidationRequests: s.Inv.ValidationRequests}
			}
			cls, err := classify.New().Classify(context.Background(), investigate.ClassifyInput{Bundle: s.Bundle, Traversal: s.Inv.Traversal,
				Report: s.Report, Agent: ag, Validations: s.Inv.Validations, Config: s.Request.Config})
			if err != nil {
				t.Fatalf("%s %s: re-classification failed: %v", e.Name(), m.ID, err)
			}
			now := map[string]domain.InvestigationFinding{}
			for _, f := range cls.Findings {
				now[f.ChangeItem] = f
			}
			for _, before := range s.Inv.Findings {
				items++
				after, ok := now[before.ChangeItem]
				if !ok {
					t.Errorf("%s %s %s: no finding after re-classification", e.Name(), m.ID, before.ChangeItem)
					continue
				}
				if before.Class == after.Class && before.Rule == after.Rule {
					same++
					continue
				}
				key := fmt.Sprintf("%s %s → %s %s", before.Class, before.Rule, after.Class, after.Rule)
				transitions[key]++
				if looser(before.Class, after.Class) {
					t.Errorf("%s %s %s: %s (looser)", e.Name(), m.ID, before.ChangeItem, key)
				}
			}
		}
	}
	if runs == 0 {
		t.Fatal("no stored B0/C1/D1 run found")
	}
	t.Logf("%d runs, %d cases, %d items: %d identical (class and rule)", runs, cases, items, same)
	var keys []string
	for k := range transitions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("changed: %s (%d items)", k, transitions[k])
	}
}
