package eval

// The recall-preservation gate and the false-positive coverage of the
// routine-maintenance classifier, measured against the STORED eval run.
//
// The fixtures in testdata/stored-edges are the trimmed Changes and evidence
// pools of the edges that produced eval/results/*.json (recorded live on
// 2026-10-01 against the real upstreams, commit 0cf815f). The routine
// detector (upgrade.ClassifyRoutine) is a pure function of a Change, so
// running it here reproduces exactly what the pipeline computes when it
// builds these edges today:
//
//   - TestRoutineNeverCoversExpectedItems: for EVERY dataset entry, no change
//     matched by an expected item's matchers classifies routine. This is the
//     hard safety gate: routine classification may cost precision points,
//     never recall.
//   - TestRoutineCoversStoredFalsePositives: of the changes the stored run
//     counted as false positives, the accepted share is routine (Vault ≥ 90%,
//     ingress-nginx ≥ 80%, Cilium's metric renames 100% — see REPORT.md).
//
// These tests are fully offline; `go test ./internal/eval` needs no network
// and no .ri state.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

type storedEdgeFixture struct {
	CaseID   string            `json:"caseId"`
	Recorded string            `json:"recordedFrom"`
	// MissedAtRecording lists expected items no recorded change matched (recall
	// misses of the recorded run). The routine gate is vacuous for them; the
	// list is checked both ways so it cannot go stale silently.
	MissedAtRecording []string `json:"missedAtRecording,omitempty"`
	Changes  []domain.Change   `json:"changes"`
	Evidence []domain.Evidence `json:"evidence"`
}

func loadRoutineFixture(t *testing.T, caseID string) (*storedEdgeFixture, EvidenceIndex) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "stored-edges", caseID+".json"))
	if err != nil {
		t.Fatalf("stored-edge fixture: %v", err)
	}
	var fx storedEdgeFixture
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatalf("%s: %v", caseID, err)
	}
	idx := make(EvidenceIndex, len(fx.Evidence))
	for _, x := range fx.Evidence {
		idx[x.ID] = x
	}
	return &fx, idx
}

func routineIDs(fx *storedEdgeFixture) map[string]bool {
	out := map[string]bool{}
	for _, c := range fx.Changes {
		if routine, _ := upgrade.ClassifyRoutine(c); routine {
			out[c.ID] = true
		}
	}
	return out
}

func TestRoutineNeverCoversExpectedItems(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(datasetRoot, CasesDirName))
	if err != nil {
		t.Fatal(err)
	}
	gated := 0
	for _, e := range entries {
		dir := filepath.Join(datasetRoot, CasesDirName, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "case.yaml")); err != nil {
			continue
		}
		c, err := LoadCase(dir)
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if c.TransferOf != "" {
			continue // shares the base case's expected items and edge (gated there)
		}
		if _, err := os.Stat(filepath.Join("testdata", "stored-edges", c.ID+".json")); os.IsNotExist(err) {
			// Cases added after the stored run (2026-10-01, 17 entries) have no
			// recorded edge yet; recording one is a separate, reviewed step.
			t.Logf("%s: no stored-edge fixture; not gated", c.ID)
			continue
		}
		fx, ev := loadRoutineFixture(t, c.ID)
		routine := routineIDs(fx)
		gated++
		for _, exp := range c.Expected {
			hits, routineHits := 0, 0
			for _, m := range exp.Match {
				for i := range fx.Changes {
					if !m.Matches(fx.Changes[i], ev) {
						continue
					}
					hits++
					if routine[fx.Changes[i].ID] {
						routineHits++
					}
				}
			}
			knownMiss := false
			for _, id := range fx.MissedAtRecording {
				knownMiss = knownMiss || id == exp.ID
			}
			if knownMiss {
				if hits > 0 {
					t.Errorf("%s %s: listed in missedAtRecording but %d stored changes match — update the fixture", c.ID, exp.ID, hits)
				} else {
					t.Logf("%s %s: recall miss at recording; routine gate vacuous for it", c.ID, exp.ID)
				}
				continue
			}
			if hits == 0 {
				// The gate is only meaningful while the stored edges still
				// match the ground truth; a silent divergence would turn it
				// vacuous.
				t.Errorf("%s %s: no stored change matches any matcher — fixture is out of sync with the case", c.ID, exp.ID)
				continue
			}
			// An expected item loses recall only when EVERY change covering it
			// classifies routine. A routine change that merely shares a keyword
			// with an expectation is fine: e.g. ingress-nginx E4 ("s390x images
			// dropped") is covered by the ⚠️ breaking "Images: Drop `s390x`"
			// change (never routine), while the dataset itself lists the bare
			// "Images: Build `s390x` controller" changelog entry as a false
			// positive.
			if hits == routineHits {
				t.Errorf("%s %s: all %d matching changes classify routine — the expected item would drop out of the narrative", c.ID, exp.ID, hits)
			}
		}
	}
	if gated == 0 {
		t.Fatal("no dataset entries found")
	}
	if gated < 17 {
		t.Errorf("expected to gate at least the 17 recorded dataset entries, got %d", gated)
	}
}

// TestRoutineCoversStoredFalsePositives measures the routine share of the
// changes the stored run counted as false positives. The thresholds are the
// acceptance criteria of the routine-classification goal; everything above
// them is reported, not asserted.
func TestRoutineCoversStoredFalsePositives(t *testing.T) {
	cases := map[string]float64{
		"vault-1.21-2.0":          0.90, // 55 FPs: per-plugin dependency bumps + UI-only items
		"ingress-nginx-1.11-1.12": 0.80, // 14 FPs: Images:/docs: commit firehose
		"cilium-1.16-1.17":        1.00, // 16 FPs: metric renames/deprecations (see REPORT.md for the verdict)
	}
	for caseID, minShare := range cases {
		t.Run(caseID, func(t *testing.T) {
			stored, err := LoadStored(datasetRoot, caseID)
			if err != nil || stored == nil {
				t.Fatalf("stored result: %v", err)
			}
			if len(stored.FPChangeIDs) == 0 {
				t.Fatal("stored result has no false-positive ids")
			}
			fx, _ := loadRoutineFixture(t, caseID)
			byID := map[string]domain.Change{}
			for _, c := range fx.Changes {
				byID[c.ID] = c
			}
			routine := 0
			var missed []string
			for _, id := range stored.FPChangeIDs {
				c, ok := byID[id]
				if !ok {
					t.Fatalf("stored false positive %s not in fixture (fixture out of sync)", id)
				}
				if r, kind := upgrade.ClassifyRoutine(c); r {
					routine++
				} else if c.Category != domain.CategorySecurity {
					// security carve-outs are expected misses; anything else is reported
					missed = append(missed, kind+"|"+c.Title)
				}
			}
			share := float64(routine) / float64(len(stored.FPChangeIDs))
			t.Logf("%s: %d/%d stored false positives classify routine (%.2f, threshold %.2f); non-security misses: %v",
				caseID, routine, len(stored.FPChangeIDs), share, minShare, missed)
			if share < minShare {
				t.Errorf("routine coverage %.2f < %.2f", share, minShare)
			}
		})
	}
}
