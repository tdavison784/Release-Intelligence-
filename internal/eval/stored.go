package eval

// Stored results and regression detection. `ri eval -update` writes one JSON
// file per case under <dataset>/results/ after a human reviews a change in
// behaviour; every later run compares fresh metrics against that snapshot
// and reports regressions and improvements. Expectations (case.yaml) are
// never written by tools.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ResultsDirName is the subdirectory of the dataset root holding snapshots.
const ResultsDirName = "results"

// StoredResult is the committed snapshot of one entry's metrics plus the
// audit trail needed to understand a diff (which items were missed, which
// changes were false positives). Volatile fields (evidence excerpts, times)
// are deliberately absent: a snapshot must diff cleanly.
type StoredResult struct {
	CaseID  string      `json:"caseId"`
	Metrics Metrics     `json:"metrics"`
	Env     *EnvMetrics `json:"env,omitempty"`
	// MissedIDs are the expected ids that were NOT found in the stored run.
	MissedIDs []string `json:"missedIds,omitempty"`
	// FPChangeIDs are the change ids that matched notExpected entries.
	FPChangeIDs []string `json:"falsePositiveChangeIds,omitempty"`
	// EnvFindingMissedIDs are expectedFinding ids not found in the stored run.
	EnvFindingMissedIDs []string `json:"envFindingMissedIds,omitempty"`
}

// StoredFromResult projects an entry result into its stable snapshot.
func StoredFromResult(r EntryResult) StoredResult {
	s := StoredResult{CaseID: r.CaseID, Metrics: r.Metrics, Env: r.Env}
	for _, m := range r.Matches {
		if !m.Found {
			s.MissedIDs = append(s.MissedIDs, m.ExpectedID)
		}
	}
	for _, fp := range r.FalsePos {
		s.FPChangeIDs = append(s.FPChangeIDs, fp.ChangeIDs...)
	}
	for _, f := range r.EnvFindings {
		if !f.Found {
			s.EnvFindingMissedIDs = append(s.EnvFindingMissedIDs, f.ID)
		}
	}
	return s
}

// ResultsPath returns the snapshot path of a case under root.
func ResultsPath(root, caseID string) string {
	return filepath.Join(root, ResultsDirName, caseID+".json")
}

// LoadStored reads the snapshot of one case (nil when none exists).
func LoadStored(root, caseID string) (*StoredResult, error) {
	b, err := os.ReadFile(ResultsPath(root, caseID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s StoredResult
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", ResultsPath(root, caseID), err)
	}
	return &s, nil
}

// WriteStored writes the snapshot of one case, creating the directory.
func WriteStored(root string, s StoredResult) error {
	dir := filepath.Join(root, ResultsDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(ResultsPath(root, s.CaseID), b, 0o644)
}

// Delta is one metric difference between a stored snapshot and a fresh run.
type Delta struct {
	CaseID string `json:"caseId"`
	Field  string `json:"field"`
	From   int    `json:"from"`
	To     int    `json:"to"`
	// Regression is true when the fresh run is worse for the operator.
	Regression bool `json:"regression"`
	// Improvement is true when the fresh run is better.
	Improvement bool `json:"improvement"`
	// Detail names the items behind the change (missed ids gained/lost...).
	Detail string `json:"detail,omitempty"`
}

// String renders the delta for text output.
func (d Delta) String() string {
	arrow := "="
	switch {
	case d.Regression:
		arrow = "← worse"
	case d.Improvement:
		arrow = "→ better"
	}
	return fmt.Sprintf("%s %s: %d → %d %s", d.CaseID, d.Field, d.From, d.To, arrow)
}

// worse is true when an increase of the field is a regression.
var regressionFields = []struct {
	field string
	worse bool // true: higher is worse; false: lower is worse
	get   func(Metrics) int
}{
	{"found", false, func(m Metrics) int { return m.Found }},
	{"missedCritical", true, func(m Metrics) int { return m.MissedCritical }},
	{"missedImportant", true, func(m Metrics) int { return m.MissedImportant }},
	{"missedMinor", true, func(m Metrics) int { return m.MissedMinor }},
	{"falsePositives", true, func(m Metrics) int { return m.FalsePositives }},
	{"duplicateGroups", true, func(m Metrics) int { return m.DuplicateGroups }},
	{"unsupported", true, func(m Metrics) int { return m.Unsupported }},
}

var envRegressionFields = []struct {
	field string
	worse bool
	get   func(EnvMetrics) int
}{
	{"impactLinksHit", false, func(m EnvMetrics) int { return m.ImpactLinksHit }},
	{"findingsFound", false, func(m EnvMetrics) int { return m.FindingsFound }},
	{"findingsFalsePositives", true, func(m EnvMetrics) int { return m.FindingsFP }},
	{"notAffectedViolations", true, func(m EnvMetrics) int { return m.NotAffectedViolations }},
}

// Diff compares a fresh result against its snapshot. Deltas with no change
// are omitted. New misses / resolved misses are called out in Detail using
// the ids stored in the snapshot.
func Diff(stored StoredResult, fresh EntryResult) []Delta {
	var out []Delta
	freshStored := StoredFromResult(fresh)
	push := func(field string, from, to int, worse bool, detail string) {
		if from == to {
			return
		}
		d := Delta{CaseID: fresh.CaseID, Field: field, From: from, To: to, Detail: detail}
		switch {
		case worse && to > from:
			d.Regression = true
		case !worse && to < from:
			d.Regression = true
		case worse && to < from:
			d.Improvement = true
		case !worse && to > from:
			d.Improvement = true
		}
		out = append(out, d)
	}
	for _, f := range regressionFields {
		push(f.field, f.get(stored.Metrics), f.get(freshStored.Metrics), f.worse, "")
	}
	// detail the found diff with the ids
	var gained, resolved []string
	freshMissed := map[string]bool{}
	for _, id := range freshStored.MissedIDs {
		freshMissed[id] = true
	}
	oldMissed := map[string]bool{}
	for _, id := range stored.MissedIDs {
		oldMissed[id] = true
	}
	for id := range freshMissed {
		if !oldMissed[id] {
			gained = append(gained, id)
		}
	}
	for id := range oldMissed {
		if !freshMissed[id] {
			resolved = append(resolved, id)
		}
	}
	sort.Strings(gained)
	sort.Strings(resolved)
	if len(gained) > 0 || len(resolved) > 0 {
		d := Delta{CaseID: fresh.CaseID, Field: "missedIds",
			From: len(stored.MissedIDs), To: len(freshStored.MissedIDs)}
		if len(gained) > 0 {
			d.Detail = "newly missed: " + joinIDs(gained)
			d.Regression = true
		}
		if len(resolved) > 0 {
			if d.Detail != "" {
				d.Detail += "; "
			}
			d.Detail += "now found: " + joinIDs(resolved)
			d.Improvement = true
		}
		out = append(out, d)
	}
	if fresh.Env != nil && stored.Env != nil {
		for _, f := range envRegressionFields {
			push(f.field, f.get(*stored.Env), f.get(*freshStored.Env), f.worse, "")
		}
	}
	return out
}

// HasRegression reports whether any delta is a regression.
func HasRegression(deltas []Delta) bool {
	for _, d := range deltas {
		if d.Regression {
			return true
		}
	}
	return false
}

func joinIDs(ids []string) string {
	s := ""
	for i, id := range ids {
		if i > 0 {
			s += ", "
		}
		s += id
	}
	return s
}
