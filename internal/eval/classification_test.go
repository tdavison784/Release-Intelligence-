package eval

// Tests of the classification scoring (G9/G10/G11): expected-vs-actual
// classes, the confusion matrix and its severity weighting, adjudication
// folding, the false-action rate, suggestion scoring and the gates. All
// offline over in-code fixtures.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// classCase is a case with classification expectations.
func classCase() *Case {
	return &Case{
		ID: "cls", Product: "fixture", From: "v1.0.0", To: "v2.0.0",
		Expected: []Expected{
			// found by an action-required change; expected class
			// action-required → matched
			{ID: "E1", Title: "must act", Kind: KindRemoval, Importance: ImportanceCritical,
				Classification: ClassActionRequired,
				Match:          []Matcher{{Subject: "--must-act"}}},
			// found by a non-action change; expected review-required but the
			// change-level signal can only say "action" → scored as a mismatch
			{ID: "E2", Title: "needs a look", Kind: KindBehaviour, Importance: ImportanceImportant,
				Classification: ClassReviewRequired,
				Match:          []Matcher{{Subject: "--needs-look"}}},
			// found, but no class signal anywhere → NOT scored
			{ID: "E3", Title: "no signal", Kind: KindDeprecation, Importance: ImportanceMinor,
				Classification: ClassInformational,
				Match:          []Matcher{{Subject: "--no-signal"}}},
			// no expected class → legacy shape, never scored
			{ID: "E4", Title: "unclassified", Kind: KindAPI, Importance: ImportanceMinor,
				Match: []Matcher{{Subject: "--unclassified"}}},
		},
	}
}

func classEdge() *domain.UpgradeEdge {
	return &domain.UpgradeEdge{
		Product: domain.ProductRef{ID: "fixture"},
		From:    domain.Version{Semver: "1.0.0"}, To: domain.Version{Semver: "2.0.0"},
		Changes: []domain.Change{
			{ID: "chg-a", Title: "--must-act removed", Subjects: []string{"--must-act"},
				Category: domain.CategoryRemoval, ActionRequired: true,
				Evidence: []domain.EvidenceID{"ev-1"}},
			{ID: "chg-b", Title: "--needs-look default changed", Subjects: []string{"--needs-look"},
				Category: domain.CategoryConfiguration, ActionRequired: false,
				Evidence: []domain.EvidenceID{"ev-1"}},
			{ID: "chg-c", Title: "--no-signal touched", Subjects: []string{"--no-signal"},
				Category: domain.CategoryOther, ActionRequired: false,
				Evidence: []domain.EvidenceID{"ev-1"}},
			{ID: "chg-d", Title: "--unclassified touched", Subjects: []string{"--unclassified"},
				Category: domain.CategoryOther, ActionRequired: false,
				Evidence: []domain.EvidenceID{"ev-1"}},
		},
		Evidence: []domain.Evidence{ev("ev-1", "https://example/notes.md")},
	}
}

func TestClassificationScoredFromChangeActionAxis(t *testing.T) {
	res := ScoreEntry(classCase(), classEdge(), nil, nil)
	// scoreable: only E1 (its matching change carries the action flag).
	// E2's change is not action-required and the change vocabulary has no
	// other class signal → unlabelled, unscored. E3 likewise; E4 has no
	// expectation.
	if res.Metrics.ClassificationScored != 1 {
		t.Errorf("scored = %d, want 1 (only E1 shows a change-level class signal)", res.Metrics.ClassificationScored)
	}
	if res.Metrics.ClassificationMatched != 1 {
		t.Errorf("matched = %d, want 1 (E1)", res.Metrics.ClassificationMatched)
	}
	for _, m := range res.Matches {
		switch m.ExpectedID {
		case "E1":
			if m.ActualClass != ClassActionRequired {
				t.Errorf("E1 actual = %q", m.ActualClass)
			}
		case "E2", "E3":
			if m.ActualClass != "" {
				t.Errorf("%s actual = %q, want none (non-action changes carry no class signal)", m.ExpectedID, m.ActualClass)
			}
		}
	}
}

func classEnvCase() *Case {
	c := classCase()
	c.Environment = &Environment{
		ExpectedImpact: []ImpactLink{
			{Expected: "E1", Relevance: RelevanceActionRequired, Why: "removed key is set"},
			{Expected: "E2", Relevance: RelevanceReview, Why: "adjacent key set"},
		},
	}
	return c
}

// classReport builds findings whose classes are the interesting ones:
// E1's finding says review-required (under-classification of required
// action), E2's says review-required (matched). Evidence pools are
// populated so the findings pass the provenance audit.
func classReport() *domain.ImpactReport {
	return &domain.ImpactReport{
		Evidence:            []domain.Evidence{ev("ev-1", "https://example/notes.md")},
		EnvironmentEvidence: []domain.Evidence{ev("ev-local", "file://values.yaml")},
		Findings: []domain.ImpactFinding{
			{ID: "f-1", Rule: "impact:values-removed", Classification: domain.ImpactReviewRequired, ChangeID: "chg-a",
				UpstreamEvidence: []domain.EvidenceID{"ev-1"}, EnvironmentEvidence: []domain.EvidenceID{"ev-local"}},
			{ID: "f-2", Rule: "impact:values-adjacent", Classification: domain.ImpactReviewRequired, ChangeID: "chg-b",
				UpstreamEvidence: []domain.EvidenceID{"ev-1"}, EnvironmentEvidence: []domain.EvidenceID{"ev-local"}},
		},
	}
}

func TestClassificationStrengthenedByFindings(t *testing.T) {
	res := ScoreEntry(classEnvCase(), classEdge(), classReport(), nil)
	// E1: expected action-required; actual = strongest(finding review,
	// change-flag action) = action-required → matched at item level (the
	// change flag rescues it) even though the FINDING under-classifies —
	// the matrix below still records that ACTION→REVIEW cell.
	// E2: expected review-required, actual review-required → scored hit
	// E3: no finding joins chg-c → unscored
	if res.Metrics.ClassificationScored != 2 {
		t.Errorf("scored = %d, want 2", res.Metrics.ClassificationScored)
	}
	if res.Metrics.ClassificationMatched != 2 {
		t.Errorf("matched = %d, want 2", res.Metrics.ClassificationMatched)
	}
	// the confusion matrix labels both findings via expectedImpact
	if res.Confusion == nil {
		t.Fatal("no confusion matrix")
	}
	if got := res.Confusion.Rows[res.indexOf(t, ClassActionRequired)][res.indexOf(t, ClassReviewRequired)]; got != 1 {
		t.Errorf("ACTION→REVIEW cell = %d, want 1", got)
	}
	if got := res.Confusion.Rows[res.indexOf(t, ClassReviewRequired)][res.indexOf(t, ClassReviewRequired)]; got != 1 {
		t.Errorf("REVIEW→REVIEW cell = %d, want 1", got)
	}
	if w := res.Confusion.WeightedMiss; w != MissWeight(ClassActionRequired, ClassReviewRequired) {
		t.Errorf("weighted miss = %.1f, want the single ACTION→REVIEW weight", w)
	}
}

func (r *EntryResult) indexOf(t *testing.T, class string) int {
	t.Helper()
	for i, c := range r.Confusion.Classes {
		if c == class {
			return i
		}
	}
	t.Fatalf("class %q not in matrix", class)
	return -1
}

func TestMissWeightsMatchThePlan(t *testing.T) {
	// the four plan-mandated pairs
	if w := MissWeight(ClassActionRequired, ClassNotAffected); w != 10 {
		t.Errorf("ACTION→NOT AFFECTED weight = %.0f, want the catastrophic 10", w)
	}
	if w := MissWeight(ClassActionRequired, ClassUnknown); w != 5 {
		t.Errorf("ACTION→UNKNOWN weight = %.0f, want serious 5", w)
	}
	if w := MissWeight(ClassActionRequired, ClassReviewRequired); w != 1 {
		t.Errorf("ACTION→REVIEW weight = %.0f, want tolerable 1", w)
	}
	if w := MissWeight(ClassNotAffected, ClassActionRequired); w != 5 {
		t.Errorf("NOT AFFECTED→ACTION weight = %.0f, want serious 5", w)
	}
	if w := MissWeight(ClassReviewRequired, ClassReviewRequired); w != 0 {
		t.Errorf("diagonal weight = %.0f, want 0", w)
	}
}

func TestFalseActionRateAndUnknownRate(t *testing.T) {
	c := classEnvCase()
	report := classReport()
	up := []domain.EvidenceID{"ev-1"}
	loc := []domain.EvidenceID{"ev-local"}
	// add: a supported but false action (joins a dataset FP change), an
	// unsupported action finding, and an unknown finding
	report.Findings = append(report.Findings,
		domain.ImpactFinding{ID: "f-fp", Rule: "impact:values-removed", Classification: domain.ImpactActionRequired, ChangeID: "chg-fp",
			UpstreamEvidence: up, EnvironmentEvidence: loc},
		domain.ImpactFinding{ID: "f-unsup", Rule: "impact:values-removed", Classification: domain.ImpactActionRequired, ChangeID: "chg-a"}, // cites no chains
		domain.ImpactFinding{ID: "f-unk", Rule: "impact:insufficient-visibility", Classification: domain.ImpactUnknown, ChangeID: "chg-c",
			UpstreamEvidence: up},
	)
	// chg-fp: a dataset false positive (matches the notExpected matcher)
	edge := classEdge()
	edge.Changes = append(edge.Changes,
		domain.Change{ID: "chg-fp", Title: "Bump actions in /test", Category: domain.CategoryDependency,
			Evidence: []domain.EvidenceID{"ev-1"}})
	c.NotExpected = []NotExpected{{Title: "test bumps", Match: []Matcher{{Text: "(?i)bump .* in /test"}}}}

	res := ScoreEntry(c, edge, report, nil)
	// action findings: f-fp (supported, wrong: dataset FP) and f-unsup
	// (unsupported: claims action-required without environment evidence)
	// — f-1 is review-required, not an action finding.
	if res.Metrics.ActionFindings != 2 {
		t.Errorf("action findings = %d, want 2", res.Metrics.ActionFindings)
	}
	if res.Metrics.FalseActionFindings != 2 {
		t.Errorf("false actions = %d, want 2 (dataset FP + unsupported)", res.Metrics.FalseActionFindings)
	}
	if res.Metrics.ActionFindingsUnsupported != 1 {
		t.Errorf("unsupported actions = %d, want 1", res.Metrics.ActionFindingsUnsupported)
	}
	if res.Metrics.UnknownFindings != 1 {
		t.Errorf("unknown findings = %d, want 1", res.Metrics.UnknownFindings)
	}
	agg := AggregateResults([]EntryResult{res})
	if agg.ActionFindings != 2 || agg.FalseActionFindings != 2 {
		t.Errorf("aggregate action accounting = %d/%d", agg.FalseActionFindings, agg.ActionFindings)
	}
	if agg.FalseActionRate != 1.0 {
		t.Errorf("false action rate = %.2f, want 1.0 for this fixture", agg.FalseActionRate)
	}
	if agg.UnknownRate == 0 {
		t.Error("unknown rate = 0 despite an unknown finding")
	}
}

func TestAdjudicationFoldingAndLabeledPrecision(t *testing.T) {
	res := ScoreEntry(classCase(), classEdge(), nil, nil)
	// matched: chg-a..chg-d cover E1..E4 → 4 dataset-true; 0 dataset-false
	f := &AdjudicationFile{Case: "cls", Adjudications: []Adjudication{
		{ChangeID: "chg-x", Verdict: VerdictFalsePositive, Reason: "metrics rename, monitoring maintenance"},
		{ChangeID: "chg-y", Verdict: VerdictTruePositive, Reason: "real removal the must-find list lacked"},
		{ChangeID: "chg-z", Verdict: VerdictUncertain, Reason: "cannot decide from the cited docs"},
		{ChangeID: "chg-a", Verdict: VerdictFalsePositive, Reason: "overrides the dataset: not upgrade work after all"},
	}}
	// AllChangeIDs must include the adjudicated changes for the fold
	res.AllChangeIDs = append(res.AllChangeIDs, "chg-x", "chg-y", "chg-z")
	applyAdjudications(&res, f)
	if res.Adjudicated.DatasetTrue != 3 { // chg-a moved out of the dataset bucket
		t.Errorf("dataset true = %d, want 3", res.Adjudicated.DatasetTrue)
	}
	if res.Adjudicated.HumanTrue != 1 || res.Adjudicated.HumanFalse != 2 || res.Adjudicated.Uncertain != 1 {
		t.Errorf("human buckets = %d/%d/%d, want 1/2/1",
			res.Adjudicated.HumanTrue, res.Adjudicated.HumanFalse, res.Adjudicated.Uncertain)
	}
	agg := AggregateResults([]EntryResult{res})
	// labeled true = 3 + 1, labeled false = 0 + 2 → 4/6
	if agg.LabeledPrecision == 0 || (agg.AdjudicatedTrue != 4 || agg.AdjudicatedFalse != 2) {
		t.Errorf("labeled precision %.2f (%d/%d), want 4/6", agg.LabeledPrecision, agg.AdjudicatedTrue, agg.AdjudicatedFalse)
	}
	// raw precision is unchanged by adjudication
	if agg.Precision != res.Metrics.Precision() {
		t.Errorf("raw precision moved: %v vs %v", agg.Precision, res.Metrics.Precision())
	}
}

func TestSuggestionScoring(t *testing.T) {
	c := classEnvCase()
	// E2 label becomes unknown: the fixture says the correct answer IS unknown
	c.Environment.ExpectedImpact[1].Relevance = RelevanceReview
	report := &domain.ImpactReport{
		Findings: []domain.ImpactFinding{
			// suggestion on an E1 finding (label action-required) → wrong
			{ID: "f-1", Classification: domain.ImpactUnknown, ChangeID: "chg-a", SuggestedClassification: domain.ImpactReviewRequired},
			// suggestion on an E2 finding (label review-required) → correct
			{ID: "f-2", Classification: domain.ImpactUnknown, ChangeID: "chg-b", SuggestedClassification: domain.ImpactReviewRequired},
			// suggestion on an unlabelled change → counted, scores nothing
			{ID: "f-3", Classification: domain.ImpactUnknown, ChangeID: "chg-d", SuggestedClassification: domain.ImpactReviewRequired},
		},
	}
	// a genuinely-unknown-labelled finding the enricher did NOT suggest → recall miss
	report.Findings = append(report.Findings,
		domain.ImpactFinding{ID: "f-4", Classification: domain.ImpactUnknown, ChangeID: "chg-c"})
	c.Environment.ExpectedImpact = append(c.Environment.ExpectedImpact,
		ImpactLink{Expected: "E3", Relevance: RelevanceInformational}) // not unknown; f-4 unlabelled then
	// make E3's change label unknown by a dedicated link? Relevance has no
	// unknown; recall opportunities come from expectedImpact relevance — the
	// dataset vocabulary expresses "correct answer unknown" through
	// expectedFindings with classification: unknown. Add one:
	c.Environment.ExpectedFindings = []ExpectedFinding{{
		ID:    "F9",
		Match: FindingMatcher{Subject: "values.key", Classification: ClassUnknown},
	}}
	// f-4 must match that subject to be a recall opportunity
	report.Findings[len(report.Findings)-1].Matches = []domain.ImpactMatch{{Subject: "values.key"}}

	res := ScoreEntry(c, classEdge(), report, nil)
	if res.Suggestions == nil {
		t.Fatal("no suggestion metrics")
	}
	s := *res.Suggestions
	if s.Suggestions != 3 {
		t.Errorf("suggestions = %d, want 3", s.Suggestions)
	}
	if s.Labelled != 2 || s.LabeledCorrect != 1 || s.LabeledWrong != 1 || s.Unlabeled != 1 {
		t.Errorf("labelled = %d (%d correct, %d wrong), unlabelled = %d; want 2 (1/1), 1",
			s.Labelled, s.LabeledCorrect, s.LabeledWrong, s.Unlabeled)
	}
	if s.UnknownLabeled != 1 || s.UnknownSuggested != 0 {
		t.Errorf("unknown opportunities = %d/%d suggested, want 1 labelled, 0 suggested (recall miss)",
			s.UnknownSuggested, s.UnknownLabeled)
	}
	if s.Precision() != 0.5 || s.Recall() != 0 {
		t.Errorf("precision %.2f recall %.2f, want 0.5/0", s.Precision(), s.Recall())
	}
}

func TestGateEvaluationVacuousAndFailure(t *testing.T) {
	cfg := DefaultGateConfig()
	// an aggregate with nothing observed: every gate vacuous or zero-failure
	empty := Aggregate{Entries: 1}
	for _, g := range EvaluateGates(cfg, empty) {
		if !g.Pass {
			t.Errorf("gate %s failed on an empty aggregate", g.Name)
		}
		if !g.Vacuous && (g.Name == "applicabilityAccuracy" || g.Name == "falseActionRate") {
			t.Errorf("gate %s should be vacuous on an empty aggregate", g.Name)
		}
	}
	// a failing recall
	bad := Aggregate{
		Entries: 3, PipelineFailures: 1, Expected: 10, Found: 5,
		FoundCritical: 1, MissedCritical: 2, FoundImportant: 2, MissedImportant: 2,
	}
	var failed int
	for _, g := range EvaluateGates(cfg, bad) {
		if !g.Pass {
			failed++
		}
	}
	if failed == 0 {
		t.Fatal("no gate failed on a 50%-recall aggregate with a pipeline failure")
	}
	if HasGateFailure(EvaluateGates(cfg, Aggregate{})) {
		t.Error("empty aggregate must not fail gates")
	}
}

func TestGatesLoadFromDatasetRoot(t *testing.T) {
	root := t.TempDir()
	cfg, err := LoadGates(root) // no file → defaults
	if err != nil {
		t.Fatal(err)
	}
	if *cfg.CriticalRecall != 0.95 || *cfg.ApplicabilityAccuracy != 0.80 {
		t.Errorf("defaults not applied: %+v", cfg)
	}
	if err := os.WriteFile(filepath.Join(root, GatesFileName), []byte("criticalRecall: 1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadGates(root)
	if err != nil {
		t.Fatal(err)
	}
	if *cfg.CriticalRecall != 1.0 || *cfg.ImportantRecall != 0.90 {
		t.Errorf("override not merged: %+v", cfg)
	}
	if err := os.WriteFile(filepath.Join(root, GatesFileName), []byte("criticalRecall: [1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadGates(root); err == nil {
		t.Error("malformed gates.yaml must error")
	}
}

func TestAdjudicationFileValidationAndLoad(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, AdjudicationsDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `case: cls
reviewedAt: "2026-10-01"
method: sample
adjudications:
  - {changeId: chg-x, verdict: false-positive, reason: noise}
  - {changeId: chg-y, verdict: true-positive, reason: dataset gap}
`
	if err := os.WriteFile(filepath.Join(root, AdjudicationsDirName, "cls.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := LoadAdjudications(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || len(files["cls"].Adjudications) != 2 {
		t.Fatalf("loaded = %+v", files)
	}
	if files["cls"].Adjudications[0].Verdict != VerdictFalsePositive {
		t.Errorf("verdict = %q", files["cls"].Adjudications[0].Verdict)
	}
	bad := `case: cls
adjudications:
  - {changeId: chg-x, verdict: maybe, reason: ""}
`
	if err := os.WriteFile(filepath.Join(root, AdjudicationsDirName, "cls.yaml"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAdjudications(root); err == nil {
		t.Error("malformed adjudications must error")
	} else if !strings.Contains(err.Error(), "verdict") {
		t.Errorf("error should name the verdict: %v", err)
	}
	// a missing directory is not an error
	if _, err := LoadAdjudications(t.TempDir()); err != nil {
		t.Errorf("missing adjudications dir: %v", err)
	}
}

// enrichingFake implements both Pipeline and EnrichingPipeline. The enriched
// report's finding joins E1's change with the marker id, so the test can see
// which path ran through the match audit.
type enrichingFake struct{}

func (enrichingFake) Upgrade(ctx context.Context, product, from, to string) (*domain.UpgradeEdge, error) {
	return classEdge(), nil
}

func (enrichingFake) Impact(ctx context.Context, product, from, to string, inputs env.Inputs) (*domain.ImpactReport, error) {
	return markerReport("plain-marker"), nil
}

func (enrichingFake) EnrichedImpact(ctx context.Context, product, from, to string, inputs env.Inputs) (*domain.ImpactReport, error) {
	return markerReport("enriched-marker"), nil
}

func markerReport(id string) *domain.ImpactReport {
	return &domain.ImpactReport{
		Findings: []domain.ImpactFinding{
			{ID: id, Rule: "impact:values-removed", Classification: domain.ImpactActionRequired, ChangeID: "chg-a"},
		},
	}
}

func TestEnrichedRunnerUsesEnrichingPipeline(t *testing.T) {
	c := classEnvCase()
	r := &Runner{Pipeline: enrichingFake{}, Enriched: true}
	res := r.Run(context.Background(), []*Case{c})
	if len(res) != 1 || len(res[0].Matches) == 0 {
		t.Fatalf("results = %+v", res)
	}
	if got := res[0].Matches[0].FindingIDs; len(got) != 1 || got[0] != "enriched-marker" {
		t.Errorf("enriched path not used: %v", got)
	}
	r2 := &Runner{Pipeline: enrichingFake{}}
	res2 := r2.Run(context.Background(), []*Case{c})
	if got := res2[0].Matches[0].FindingIDs; len(got) != 1 || got[0] != "plain-marker" {
		t.Errorf("deterministic path must not enrich: %v", got)
	}
}
