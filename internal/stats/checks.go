package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/ingest"
)

// CheckSummary condenses one saved `ri check <id> -o json` report
// (docs/onboarding/checks/<id>.json).
type CheckSummary struct {
	Product  string   `json:"product"`
	Releases []string `json:"releases,omitempty"`

	// Subject verdicts (one per source, artifact or artifact content).
	Subjects     int `json:"subjects"`
	Validated    int `json:"validated"`    // >= 3 passes and no failure
	Failing      int `json:"failing"`      // at least one failure
	Insufficient int `json:"insufficient"` // fewer passes than required
	// Unverifiable counts subjects with at least one release that could not
	// be verified (an unreachable host); OnlyUnverifiable those where no
	// release could be verified at all.
	Unverifiable     int `json:"unverifiable"`
	OnlyUnverifiable int `json:"onlyUnverifiable"`

	// Outcomes counts individual checks by outcome (pass, fail,
	// unverifiable, not-applicable, covered).
	Outcomes map[string]int `json:"outcomes,omitempty"`

	FailingSubjects      []string `json:"failingSubjects,omitempty"`
	UnverifiableSubjects []string `json:"unverifiableSubjects,omitempty"`

	// File is the path the report was read from.
	File string `json:"-"`
}

// SummarizeCheck condenses a relationship report.
func SummarizeCheck(rep *ingest.RelationshipReport) CheckSummary {
	cs := CheckSummary{
		Product:  string(rep.Product),
		Releases: rep.Releases,
		Subjects: len(rep.Summary),
		Outcomes: map[string]int{},
	}
	for _, s := range rep.Summary {
		switch s.Verdict {
		case ingest.VerdictValidated:
			cs.Validated++
		case ingest.VerdictFailing:
			cs.Failing++
			cs.FailingSubjects = append(cs.FailingSubjects, s.Subject)
		case ingest.VerdictInsufficient:
			cs.Insufficient++
		}
		if s.Unverifiable > 0 {
			cs.Unverifiable++
			cs.UnverifiableSubjects = append(cs.UnverifiableSubjects, s.Subject)
			if s.Passed == 0 && s.Failed == 0 {
				cs.OnlyUnverifiable++
			}
		}
	}
	for _, c := range rep.Checks {
		cs.Outcomes[c.Outcome]++
	}
	sort.Strings(cs.FailingSubjects)
	sort.Strings(cs.UnverifiableSubjects)
	return cs
}

// LoadChecks reads every *.json file in dir. A file that is not a
// relationship report is skipped with a warning. The result is keyed by
// product id (the report's own product, else the file name). A missing
// directory yields no reports.
func LoadChecks(dir string) (map[string]*CheckSummary, []string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	out := map[string]*CheckSummary{}
	var warns []string
	for _, e := range entries {
		if e.IsDir() || strings.ToLower(filepath.Ext(e.Name())) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			warns = append(warns, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		var rep ingest.RelationshipReport
		if err := json.Unmarshal(data, &rep); err != nil {
			warns = append(warns, fmt.Sprintf("%s: not a relationship report, skipped: %v", path, err))
			continue
		}
		if len(rep.Summary) == 0 && len(rep.Checks) == 0 {
			warns = append(warns, fmt.Sprintf("%s: report has no summary and no checks, skipped", path))
			continue
		}
		cs := SummarizeCheck(&rep)
		cs.File = path
		fileID := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if cs.Product == "" {
			cs.Product = fileID
		} else if cs.Product != fileID {
			warns = append(warns, fmt.Sprintf("%s: report is for product %q, file name says %q; using %q", path, cs.Product, fileID, cs.Product))
		}
		if prev, dup := out[cs.Product]; dup {
			warns = append(warns, fmt.Sprintf("%s: second report for %q (first: %s), ignored", path, cs.Product, prev.File))
			continue
		}
		out[cs.Product] = &cs
	}
	return out, warns, nil
}
