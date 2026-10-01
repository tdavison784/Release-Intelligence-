package eval

// Sample-based adjudication (G9): unmatched output changes are read by a human
// against the cited upstream docs and get a verdict. Adjudications are
// committed data under <dataset>/adjudications/<case>.yaml — never derived
// from the pipeline. `labeledPrecision` counts adjudicated false positives;
// raw and adjudicated numbers are reported side by side.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// AdjudicationsDirName is the subdirectory of the dataset root holding
// adjudication files.
const AdjudicationsDirName = "adjudications"

// Verdicts of an adjudication.
const (
	VerdictTruePositive  = "true-positive"  // real upgrade knowledge the must-find list did not anticipate (dataset gap)
	VerdictFalsePositive = "false-positive" // noise: an operator would not want this in an upgrade brief
	VerdictUncertain     = "uncertain"      // cannot be decided against the cited docs; excluded from both sides
)

var verdicts = map[string]bool{
	VerdictTruePositive: true, VerdictFalsePositive: true, VerdictUncertain: true,
}

// AdjudicationFile is one dataset-side adjudication record
// (eval/adjudications/<case>.yaml).
type AdjudicationFile struct {
	Case       string `json:"case" yaml:"case"`
	ReviewedAt string `json:"reviewedAt" yaml:"reviewedAt"`
	// Method records how the sample was drawn (audit trail).
	Method        string         `json:"method,omitempty" yaml:"method,omitempty"`
	Adjudications []Adjudication `json:"adjudications" yaml:"adjudications"`
}

// Adjudication is one human verdict about one generated change.
type Adjudication struct {
	ChangeID string `json:"changeId" yaml:"changeId"`
	// Verdict is true-positive | false-positive | uncertain.
	Verdict string `json:"verdict" yaml:"verdict"`
	// Reason is a one-line justification grounded in the case's cited sources.
	Reason string `json:"reason" yaml:"reason"`
}

// Validate enforces the vocabulary.
func (f *AdjudicationFile) Validate() error {
	var errs []error
	if f.Case == "" {
		errs = append(errs, fmt.Errorf("adjudications: case is empty"))
	}
	seen := map[string]bool{}
	for i, a := range f.Adjudications {
		if a.ChangeID == "" {
			errs = append(errs, fmt.Errorf("adjudications %s: adjudication[%d]: changeId is empty", f.Case, i))
		}
		if seen[a.ChangeID] {
			errs = append(errs, fmt.Errorf("adjudications %s: duplicate changeId %s", f.Case, a.ChangeID))
		}
		seen[a.ChangeID] = true
		if !verdicts[a.Verdict] {
			errs = append(errs, fmt.Errorf("adjudications %s: %s: unknown verdict %q", f.Case, a.ChangeID, a.Verdict))
		}
		if a.Reason == "" {
			errs = append(errs, fmt.Errorf("adjudications %s: %s: reason is required (a verdict without a reason is not auditable)", f.Case, a.ChangeID))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	return fmt.Errorf("%d validation error(s): %s", len(errs), strings.Join(msgs, "; "))
}

// AdjudicationStats is the adjudication outcome of one entry: every change
// the entry produced, labelled by dataset (matched/notExpected) or by human
// verdict. DatasetTrue/DatasetFalse already exclude overridden changes; the
// human buckets carry exactly the adjudicated ones, so an adjudicated change
// never counts on both sides.
type AdjudicationStats struct {
	// DatasetTrue are non-adjudicated changes covering ground truth (matched).
	DatasetTrue int `json:"datasetTrue"`
	// DatasetFalse are non-adjudicated changes matching notExpected entries.
	DatasetFalse int `json:"datasetFalse"`
	// HumanTrue / HumanFalse / Uncertain are adjudication verdicts (they
	// override the dataset label in either direction; uncertain moves a
	// change out of both sides — an undecided item must not sway a
	// precision number).
	HumanTrue    int      `json:"humanTrue"`
	HumanFalse   int      `json:"humanFalse"`
	Uncertain    int      `json:"uncertain"`
	TrueIDs      []string `json:"trueIds,omitempty"`
	FalseIDs     []string `json:"falseIds,omitempty"`
	UncertainIDs []string `json:"uncertainIds,omitempty"`
}

// LabeledTrue is the adjudicated true count.
func (s AdjudicationStats) LabeledTrue() int { return s.DatasetTrue + s.HumanTrue }

// LabeledFalse is the adjudicated false count (what labeledPrecision's
// denominator adds).
func (s AdjudicationStats) LabeledFalse() int { return s.DatasetFalse + s.HumanFalse }

// LabeledPrecision is the adjudicated precision: labeled true / (labeled true
// + labeled false), 0 when neither exists.
func (s AdjudicationStats) LabeledPrecision() float64 {
	den := s.LabeledTrue() + s.LabeledFalse()
	if den == 0 {
		return 0
	}
	return float64(s.LabeledTrue()) / float64(den)
}

// LoadAdjudications reads every adjudication file under
// <root>/adjudications/, keyed by case id. A missing directory is not an
// error (adjudication is opt-in data); a malformed file is.
func LoadAdjudications(root string) (map[string]*AdjudicationFile, error) {
	dir := filepath.Join(root, AdjudicationsDirName)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return map[string]*AdjudicationFile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("adjudications: %w", err)
	}
	out := map[string]*AdjudicationFile{}
	var names []string
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var f AdjudicationFile
		if err := yaml.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		f.Case = trimYamlExt(name)
		if err := f.Validate(); err != nil {
			return nil, err
		}
		if _, dup := out[f.Case]; dup {
			return nil, fmt.Errorf("%s: duplicate adjudication file for case %s", name, f.Case)
		}
		out[f.Case] = &f
	}
	return out, nil
}

func trimYamlExt(name string) string {
	name = strings.TrimSuffix(name, ".yaml")
	return strings.TrimSuffix(name, ".yml")
}
