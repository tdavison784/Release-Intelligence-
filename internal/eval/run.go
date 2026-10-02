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
	// Adjudications are the committed human verdicts
	// (eval/adjudications/<case>.yaml), applied after scoring. Optional.
	Adjudications map[string]*AdjudicationFile
	// Enriched asks pipelines implementing EnrichingPipeline for the
	// enriched ImpactReport (AI suggestions attached) so suggestion
	// precision/recall get scored. Deterministic content is unchanged.
	Enriched bool
	// EnrichedEnv, when set, replaces the case's environment inputs for the
	// enriched pass only: the committed answer caches replay only against the
	// environment (and path spelling) they were recorded with. The scored
	// expectations remain the case's; env-conditional expectations may
	// legitimately miss under the recorded inputs.
	EnrichedEnv *env.Inputs
	// Knowledge, when set, evaluates verified facts in the join (pipelines
	// implementing KnowledgePipeline); nil runs the knowledge-free join, so
	// results are unchanged without -knowledge.
	Knowledge *KnowledgeRun
}

// KnowledgeRun is the knowledge a run evaluates: the facts and the minimum
// verification level they must reach.
type KnowledgeRun struct {
	Facts           []domain.VerifiedFact
	MinVerification domain.VerificationLevel
}

// EnrichingPipeline is implemented by pipelines that can attach the AI
// enrichment layer (suggestions on unknown findings, provenance-preserving)
// to an impact report. The report's deterministic content is identical; only
// SuggestedClassification/enrichment fields differ.
type EnrichingPipeline interface {
	Pipeline
	// EnrichedImpact builds the impact report and enriches it. Implementations
	// replay cached model answers offline (the same cache `ri impact -enrich`
	// uses); a prompt without a cached answer is reported as pending, never
	// guessed.
	EnrichedImpact(ctx context.Context, product, from, to string, inputs env.Inputs) (*domain.ImpactReport, error)
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
	if c.Environment == nil {
		return env.Inputs{}, nil
	}
	return DirInputs(filepath.Join(c.Dir, EnvironmentDir), c.Environment.Kubernetes)
}

// DirInputs builds env.Inputs from a directory laid out like a case
// environment (values.yaml, manifests/, crds/, images.txt, inventory.yaml).
func DirInputs(dir, kubernetes string) (env.Inputs, error) {
	var in env.Inputs
	in.KubernetesVersion = kubernetes
	for _, n := range envFileNames {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		switch n {
		case "values.yaml":
			in.ValuesFiles = append(in.ValuesFiles, p)
		case "manifests":
			in.Manifests = append(in.Manifests, p)
		case "crds":
			in.CRDs = append(in.CRDs, p)
		case "inventory.yaml":
			in.Inventory = p
		case "images.txt":
			imgs, err := readImagesFile(p)
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
		res := r.runCase(ctx, c)
		if c.TransferOf != "" {
			res.environmentOnly() // transfer.go: the base entry scores the edge
		}
		out = append(out, res)
	}
	return out
}

func (r *Runner) runCase(ctx context.Context, c *Case) EntryResult {
	edge, err := r.Pipeline.Upgrade(ctx, c.Product, c.From, c.To)
	if err != nil {
		res := ScoreEntry(c, nil, nil, fmt.Errorf("upgrade: %w", err))
		applyAdjudications(&res, r.Adjudications[c.adjudicationKey()])
		return res
	}
	var report *domain.ImpactReport
	if c.Environment != nil {
		inputs, ierr := environmentInputs(c)
		if ierr != nil {
			res := ScoreEntry(c, edge, nil, nil) // note: env load failures surface as env misses
			applyAdjudications(&res, r.Adjudications[c.adjudicationKey()])
			return res
		}
		if r.Enriched {
			if r.EnrichedEnv != nil {
				inputs = *r.EnrichedEnv
			}
			if ep, ok := r.Pipeline.(EnrichingPipeline); ok {
				report, err = ep.EnrichedImpact(ctx, c.Product, c.From, c.To, inputs)
			} else {
				err = fmt.Errorf("pipeline does not support -enriched (no EnrichingPipeline)")
			}
		} else if r.Knowledge != nil {
			if kp, ok := r.Pipeline.(KnowledgePipeline); ok {
				report, err = kp.ImpactWithKnowledge(ctx, c.Product, c.From, c.To, inputs, r.Knowledge.Facts, r.Knowledge.MinVerification)
			} else {
				err = fmt.Errorf("pipeline does not support -knowledge (no KnowledgePipeline)")
			}
		} else {
			report, err = r.Pipeline.Impact(ctx, c.Product, c.From, c.To, inputs)
		}
		if err != nil {
			// The edge still counts; the join failure is recorded as a nil
			// report (all environment expectations miss).
			report = nil
		}
	}
	res := ScoreEntry(c, edge, report, nil)
	applyAdjudications(&res, r.Adjudications[c.adjudicationKey()])
	return res
}

// Aggregate pools the per-entry metrics. The named fields are the G10
// metrics; the deprecated aliases keep old consumers working for one release.
type Aggregate struct {
	Entries int `json:"entries"`
	// PipelineFailures counts entries whose pipeline could not run at all
	// (execution errors). They are never conflated with reasoning misses: a
	// missed item because the pipeline errored is still a missed item (the
	// entry scores zero), but the cause is recorded here.
	PipelineFailures int `json:"pipelineFailures"`
	Expected         int `json:"expected"`
	Found            int `json:"found"`
	MissedCritical   int `json:"missedCritical"`
	MissedImportant  int `json:"missedImportant"`
	MissedMinor      int `json:"missedMinor"`
	Changes          int `json:"changes"`
	MatchedChanges   int `json:"matchedChanges"`
	FalsePositives   int `json:"falsePositives"`
	DuplicateGroups  int `json:"duplicateGroups"`
	Unsupported      int `json:"unsupported"`

	// --- G10 metrics -------------------------------------------------------

	// Recall is found/expected over every importance (kept name).
	Recall float64 `json:"recall"`
	// CriticalRecall is found/expected over critical items only: the work
	// that breaks production if missed (FoundCritical pools the count).
	CriticalRecall float64 `json:"criticalRecall"`
	FoundCritical  int     `json:"foundCritical"`
	// ImportantRecall is found/expected over important items only.
	ImportantRecall float64 `json:"importantRecall"`
	FoundImportant  int     `json:"foundImportant"`
	// Precision is the raw labeled precision (deprecated alias of
	// LabeledPrecision, kept one release; identical before adjudication).
	Precision float64 `json:"precision"`
	// LabeledPrecision is the precision over adjudicated output: dataset
	// labels plus human verdicts (see eval/adjudications/).
	LabeledPrecision float64 `json:"labeledPrecision"`
	// AdjudicatedTrue / AdjudicatedFalse are the adjudicated label counts
	// behind LabeledPrecision.
	AdjudicatedTrue  int `json:"adjudicatedTrue"`
	AdjudicatedFalse int `json:"adjudicatedFalse"`
	// FalseActionRate: wrong action-required findings / all action-required
	// findings (0 when none were produced).
	FalseActionRate float64 `json:"falseActionRate"`
	ActionFindings  int     `json:"actionFindings"`
	// FalseActionFindings is the wrong subset; ActionFindingsUnsupported the
	// subset failing the provenance audit (the both-chain evidence gate).
	FalseActionFindings       int `json:"falseActionFindings"`
	ActionFindingsUnsupported int `json:"actionFindingsUnsupported"`
	// ApplicabilityAccuracy: correct applicability decisions / all decisions
	// over environment entries — affected links hit plus not-affected links
	// left clear, over all links.
	ApplicabilityAccuracy float64 `json:"applicabilityAccuracy"`
	// ClassificationAccuracy: expected items with an expected class whose
	// observed class matched / observable class expectations.
	ClassificationScored   int     `json:"classificationScored"`
	ClassificationAccuracy float64 `json:"classificationAccuracy"`
	// UnknownRate: unknown findings / all findings over environment entries.
	UnknownRate     float64 `json:"unknownRate"`
	UnknownFindings int     `json:"unknownFindings"`
	FindingsTotal   int     `json:"findingsTotal"`
	// EvidenceCoverage: matched changes with resolving evidence / matched.
	EvidenceCoverage float64 `json:"evidenceCoverage"`
	EvidenceCovered  int     `json:"evidenceCovered"`
	// DuplicateRate: changes inside a duplicate group / all changes.
	DuplicateRate     float64 `json:"duplicateRate"`
	DuplicatedChanges int     `json:"duplicatedChanges"`
	// ConfusionMatrix over labelled findings (nil when nothing was labelled).
	Confusion *ConfusionMatrix `json:"confusion,omitempty"`
	// Suggestions pools the enriched-run suggestion scoring.
	Suggestions         *SuggestionMetrics `json:"suggestions,omitempty"`
	SuggestionPrecision float64            `json:"suggestionPrecision"`
	SuggestionRecall    float64            `json:"suggestionRecall"`

	// --- environment (kept names) -----------------------------------------
	EnvEntries            int     `json:"envEntries"`
	ImpactLinks           int     `json:"impactLinks"`
	ImpactLinksHit        int     `json:"impactLinksHit"`
	ImpactAccuracy        float64 `json:"impactAccuracy"`
	NotAffectedLinks      int     `json:"notAffectedLinks"`
	NotAffectedViolations int     `json:"notAffectedViolations"`
	FindingsExpected      int     `json:"findingsExpected"`
	FindingsFound         int     `json:"findingsFound"`
	FindingsFP            int     `json:"findingsFalsePositives"`
	// UnknownHonesty (reported, never gated): undecided links answered
	// honestly / undecided links — vacuous if nothing is decided, so it is
	// always shown next to ApplicabilityAccuracy and ImpactAccuracy.
	UndecidedLinks  int     `json:"undecidedLinks"`
	UndecidedHonest int     `json:"undecidedHonest"`
	UnknownHonesty  float64 `json:"unknownHonesty"`
}

// Aggregate computes the pooled numbers over the entry results.
func AggregateResults(rs []EntryResult) Aggregate {
	var a Aggregate
	a.Entries = len(rs)
	dupChanges := map[string]bool{}
	// accumulators without a direct pooled field
	var (
		criticalFound, importantFound int
		classificationMatched         int
	)
	for _, r := range rs {
		if r.Error != "" {
			a.PipelineFailures++
		}
		a.Expected += r.Metrics.Expected
		a.Found += r.Metrics.Found
		criticalFound += r.Metrics.ExpectedCritical - r.Metrics.MissedCritical
		importantFound += r.Metrics.ExpectedImportant - r.Metrics.MissedImportant
		a.FoundCritical = criticalFound
		a.FoundImportant = importantFound
		a.MissedCritical += r.Metrics.MissedCritical
		a.MissedImportant += r.Metrics.MissedImportant
		a.MissedMinor += r.Metrics.MissedMinor
		a.Changes += r.Metrics.Changes
		a.MatchedChanges += r.Metrics.MatchedChanges
		a.FalsePositives += r.Metrics.FalsePositives
		a.DuplicateGroups += r.Metrics.DuplicateGroups
		a.Unsupported += r.Metrics.Unsupported
		a.ClassificationScored += r.Metrics.ClassificationScored
		classificationMatched += r.Metrics.ClassificationMatched
		a.EvidenceCovered += r.Metrics.EvidenceCovered
		a.ActionFindings += r.Metrics.ActionFindings
		a.FalseActionFindings += r.Metrics.FalseActionFindings
		a.ActionFindingsUnsupported += r.Metrics.ActionFindingsUnsupported
		for _, d := range r.Duplicates {
			for _, id := range d.ChangeIDs {
				dupChanges[id] = true
			}
		}
		if r.Confusion != nil {
			if a.Confusion == nil {
				a.Confusion = NewConfusionMatrix()
			}
			for i, row := range r.Confusion.Rows {
				for j, n := range row {
					for k := 0; k < n; k++ {
						a.Confusion.Add(r.Confusion.Classes[i], r.Confusion.Classes[j])
					}
				}
			}
		}
		if r.Suggestions != nil {
			if a.Suggestions == nil {
				a.Suggestions = &SuggestionMetrics{}
			}
			a.Suggestions.Add(*r.Suggestions)
		}
		if r.Env != nil {
			a.EnvEntries++
			a.ImpactLinks += r.Env.ImpactLinks
			a.ImpactLinksHit += r.Env.ImpactLinksHit
			a.NotAffectedLinks += r.Env.NotAffectedLinks
			a.NotAffectedViolations += r.Env.NotAffectedViolations
			a.FindingsExpected += r.Env.FindingsExpected
			a.FindingsFound += r.Env.FindingsFound
			a.FindingsFP += r.Env.FindingsFP
			a.UndecidedLinks += r.Env.UndecidedLinks
			a.UndecidedHonest += r.Env.UndecidedHonest
			a.UnknownFindings += r.Metrics.UnknownFindings
			a.FindingsTotal += r.Env.Findings
		}
		a.AdjudicatedTrue += r.Adjudicated.LabeledTrue()
		a.AdjudicatedFalse += r.Adjudicated.LabeledFalse()
	}
	a.Recall = Metrics{Expected: a.Expected, Found: a.Found}.Recall()
	a.CriticalRecall = ratio(criticalFound+a.MissedCritical, criticalFound)
	a.ImportantRecall = ratio(importantFound+a.MissedImportant, importantFound)
	a.Precision = Metrics{MatchedChanges: a.MatchedChanges, FalsePositives: a.FalsePositives}.Precision()
	a.LabeledPrecision = AdjudicationStats{DatasetTrue: a.AdjudicatedTrue, DatasetFalse: a.AdjudicatedFalse}.LabeledPrecision()
	a.ImpactAccuracy = EnvMetrics{ImpactLinks: a.ImpactLinks, ImpactLinksHit: a.ImpactLinksHit}.ImpactAccuracy()
	a.UnknownHonesty = ratio(a.UndecidedLinks, a.UndecidedHonest)
	a.ApplicabilityAccuracy = applicabilityAccuracy(a.ImpactLinks, a.ImpactLinksHit, a.NotAffectedLinks, a.NotAffectedViolations)
	a.ClassificationAccuracy = ratio(a.ClassificationScored, classificationMatched)
	a.FalseActionRate = ratio(a.ActionFindings, a.FalseActionFindings)
	a.UnknownRate = ratio(a.FindingsTotal, a.UnknownFindings)
	a.EvidenceCoverage = ratio(a.MatchedChanges, a.EvidenceCovered)
	a.DuplicatedChanges = len(dupChanges)
	a.DuplicateRate = ratio(a.Changes, a.DuplicatedChanges)
	if a.Suggestions != nil {
		a.SuggestionPrecision = a.Suggestions.Precision()
		a.SuggestionRecall = a.Suggestions.Recall()
	}
	return a
}

// ratio is num/den with the honest zero for an empty denominator: a metric
// nothing in the dataset could observe is reported as 0 (and the gates treat
// the vacuous case explicitly).
func ratio(den, num int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

// applicabilityAccuracy pools affected-link hits and clean not-affected
// links over all applicability decisions.
func applicabilityAccuracy(links, linksHit, notAffected, violations int) float64 {
	total := links + notAffected
	if total == 0 {
		return 0
	}
	return float64(linksHit+notAffected-violations) / float64(total)
}
