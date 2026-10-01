package eval

// The runner: loads dataset entries, drives the injected Pipeline per entry
// (the REAL app for `ri eval`, a replay in tests) and scores the output.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// Runner executes dataset entries against a Pipeline.
type Runner struct {
	// Pipeline under test (the real app, or an offline replay).
	Pipeline Pipeline
	// CasesDir is the dataset root (default "eval", entries below "cases").
	CasesDir string
}

// CasesDirName is the subdirectory of the dataset root holding the entries.
const CasesDirName = "cases"

// LoadAll lists and loads every entry of the dataset, sorted by id.
func (r *Runner) LoadAll() ([]*Case, error) {
	dir := filepath.Join(r.datasetRoot(), CasesDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("eval dataset: %w", err)
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "case.yaml")); err == nil {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return r.Load(ids)
}

// Load loads the given entries by id (or path). An unknown id is an error:
// a mistyped selection must not silently measure nothing.
func (r *Runner) Load(ids []string) ([]*Case, error) {
	var out []*Case
	for _, id := range ids {
		dir := id
		if !strings.HasPrefix(dir, "/") && !strings.HasPrefix(dir, ".") {
			dir = filepath.Join(r.datasetRoot(), CasesDirName, id)
		} else {
			dir = filepath.Clean(dir)
		}
		c, err := LoadCase(dir)
		if err != nil {
			return nil, err
		}
		if c.ID != id && !strings.HasPrefix(id, "/") && !strings.HasPrefix(id, ".") {
			return nil, fmt.Errorf("case directory %s holds id %q", id, c.ID)
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *Runner) datasetRoot() string {
	if r.CasesDir != "" {
		return r.CasesDir
	}
	return "eval"
}

// environmentInputs builds the env.Inputs of a case from its environment/
// directory. Paths are used as given (relative to the dataset root, exactly
// how the CLI would pass them), so evidence URIs stay stable.
func environmentInputs(c *Case) (env.Inputs, error) {
	var in env.Inputs
	if c.Environment == nil {
		return in, nil
	}
	in.KubernetesVersion = c.Environment.Kubernetes
	for _, f := range c.EnvironmentFiles() {
		switch name := filepath.Base(f); name {
		case "values.yaml":
			in.ValuesFiles = append(in.ValuesFiles, f)
		case "manifests":
			in.Manifests = append(in.Manifests, f)
		case "crds":
			in.CRDs = append(in.CRDs, f)
		case "images.txt":
			imgs, err := readImagesFile(f)
			if err != nil {
				return in, err
			}
			in.Images = append(in.Images, imgs...)
		}
	}
	return in, nil
}

// readImagesFile parses an images.txt (one reference per line, # comments).
func readImagesFile(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, ln := range strings.Split(string(b), "\n") {
		if i := strings.IndexByte(ln, '#'); i >= 0 {
			ln = ln[:i]
		}
		if ln = strings.TrimSpace(ln); ln != "" {
			out = append(out, ln)
		}
	}
	return out, nil
}

// Run executes every case and returns the scored results (same order).
// A pipeline failure is recorded per entry, never aborting the run: an
// environment where a product's upstream is unreachable must show up as
// misses, not as a crash.
func (r *Runner) Run(ctx context.Context, cases []*Case) []EntryResult {
	out := make([]EntryResult, 0, len(cases))
	for _, c := range cases {
		out = append(out, r.runCase(ctx, c))
	}
	return out
}

func (r *Runner) runCase(ctx context.Context, c *Case) EntryResult {
	edge, err := r.Pipeline.Upgrade(ctx, c.Product, c.From, c.To)
	if err != nil {
		return ScoreEntry(c, nil, nil, fmt.Errorf("upgrade: %w", err))
	}
	var report *domain.ImpactReport
	if c.Environment != nil {
		inputs, ierr := environmentInputs(c)
		if ierr != nil {
			return ScoreEntry(c, edge, nil, nil) // note: env load failures surface as env misses
		}
		report, err = r.Pipeline.Impact(ctx, c.Product, c.From, c.To, inputs)
		if err != nil {
			// The edge still counts; the join failure is recorded as a nil
			// report (all environment expectations miss).
			report = nil
		}
	}
	return ScoreEntry(c, edge, report, nil)
}

// Aggregate pools the per-entry metrics.
type Aggregate struct {
	Entries          int     `json:"entries"`
	Errored          int     `json:"errored"`
	Expected         int     `json:"expected"`
	Found            int     `json:"found"`
	MissedCritical   int     `json:"missedCritical"`
	MissedImportant  int     `json:"missedImportant"`
	MissedMinor      int     `json:"missedMinor"`
	Changes          int     `json:"changes"`
	MatchedChanges   int     `json:"matchedChanges"`
	FalsePositives   int     `json:"falsePositives"`
	DuplicateGroups  int     `json:"duplicateGroups"`
	Unsupported      int     `json:"unsupported"`
	Recall           float64 `json:"recall"`
	Precision        float64 `json:"precision"`
	EnvEntries       int     `json:"envEntries"`
	ImpactLinks      int     `json:"impactLinks"`
	ImpactLinksHit   int     `json:"impactLinksHit"`
	ImpactAccuracy   float64 `json:"impactAccuracy"`
	FindingsExpected int     `json:"findingsExpected"`
	FindingsFound    int     `json:"findingsFound"`
	FindingsFP       int     `json:"findingsFalsePositives"`
}

// Aggregate computes the pooled numbers over the entry results.
func AggregateResults(rs []EntryResult) Aggregate {
	var a Aggregate
	a.Entries = len(rs)
	for _, r := range rs {
		if r.Error != "" {
			a.Errored++
		}
		a.Expected += r.Metrics.Expected
		a.Found += r.Metrics.Found
		a.MissedCritical += r.Metrics.MissedCritical
		a.MissedImportant += r.Metrics.MissedImportant
		a.MissedMinor += r.Metrics.MissedMinor
		a.Changes += r.Metrics.Changes
		a.MatchedChanges += r.Metrics.MatchedChanges
		a.FalsePositives += r.Metrics.FalsePositives
		a.DuplicateGroups += r.Metrics.DuplicateGroups
		a.Unsupported += r.Metrics.Unsupported
		if r.Env != nil {
			a.EnvEntries++
			a.ImpactLinks += r.Env.ImpactLinks
			a.ImpactLinksHit += r.Env.ImpactLinksHit
			a.FindingsExpected += r.Env.FindingsExpected
			a.FindingsFound += r.Env.FindingsFound
			a.FindingsFP += r.Env.FindingsFP
		}
	}
	a.Recall = Metrics{Expected: a.Expected, Found: a.Found}.Recall()
	a.Precision = Metrics{MatchedChanges: a.MatchedChanges, FalsePositives: a.FalsePositives}.Precision()
	a.ImpactAccuracy = EnvMetrics{ImpactLinks: a.ImpactLinks, ImpactLinksHit: a.ImpactLinksHit}.ImpactAccuracy()
	return a
}
