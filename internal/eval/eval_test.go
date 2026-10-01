package eval

// Unit tests of the matching and scoring logic against fixture edges and
// reports (no network, no filesystem beyond tempdirs). The offline replay of
// a real dataset entry is in e2e_test.go; the live dataset runs via `ri eval`.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

func ch(id, title, detail, release string, subjects []string, cat domain.Category, breaking bool, ev ...domain.EvidenceID) domain.Change {
	return domain.Change{
		ID: id, Title: title, Detail: detail, Release: release,
		Subjects: subjects, Category: cat, Breaking: breaking,
		Evidence: ev,
	}
}

func ev(id, uri string) domain.Evidence {
	return domain.Evidence{ID: domain.EvidenceID(id), URI: uri}
}

func boolPtr(b bool) *bool { return &b }

// --- matchers ---------------------------------------------------------------

func TestMatcherAllFieldsMustMatch(t *testing.T) {
	c := ch("chg-1", "RotationPolicy default changed", "from Never to Always", "1.18.0",
		[]string{"spec.privateKey.rotationPolicy"}, domain.CategoryConfiguration, true, "ev-1")
	idx := EvidenceIndex{"ev-1": ev("ev-1", "https://example/upgrading-1.18.md")}

	m := &Matcher{Text: `(?i)rotationPolicy`}
	if !m.Matches(c, idx) {
		t.Error("text matcher must match title")
	}
	m = &Matcher{Text: "Never", Subject: "spec.privateKey.rotationPolicy"}
	if !m.Matches(c, idx) {
		t.Error("text+subject must match when both hold")
	}
	m = &Matcher{Text: "Never", Subject: "other.key"}
	if m.Matches(c, idx) {
		t.Error("subject mismatch must fail the matcher")
	}
	m = &Matcher{Category: "configuration", Breaking: boolPtr(true), ActionRequired: boolPtr(false)}
	if !m.Matches(c, idx) {
		t.Error("category+flags must match")
	}
	m = &Matcher{Evidence: `upgrading-1\.18`}
	if !m.Matches(c, idx) {
		t.Error("evidence matcher must match a cited URI")
	}
	m = &Matcher{Evidence: `notthere`}
	if m.Matches(c, idx) {
		t.Error("evidence matcher must not match uncited URIs")
	}
	m = &Matcher{Release: `^1\.18`}
	if !m.Matches(c, idx) {
		t.Error("release matcher must match")
	}
	m = &Matcher{Release: `^$`}
	if m.Matches(c, idx) {
		t.Error("release matcher must not match a non-empty release")
	}
}

// --- scoring ----------------------------------------------------------------

func fixtureCase() *Case {
	return &Case{
		ID: "fixture", Product: "fixture", From: "v1.0.0", To: "v2.0.0",
		Expected: []Expected{
			{ID: "E1", Title: "rotation policy", Kind: KindBehaviour, Importance: ImportanceCritical,
				Match: []Matcher{{Text: "(?i)rotationPolicy"}}},
			{ID: "E2", Title: "removed flag", Kind: KindRemoval, Importance: ImportanceImportant,
				Match: []Matcher{{Subject: "--old-flag"}}},
			{ID: "E3", Title: "not in the output", Kind: KindDeprecation, Importance: ImportanceMinor,
				Match: []Matcher{{Text: "(?i)consul"}}},
		},
		NotExpected: []NotExpected{
			{Title: "test-only bumps", Match: []Matcher{{Text: "(?i)bump .* in /test"}}},
		},
	}
}

func fixtureEdge() *domain.UpgradeEdge {
	changes := []domain.Change{
		// covers E1
		ch("chg-a", "Default rotationPolicy changed to Always", "from Never", "1.18.0", nil, domain.CategoryConfiguration, true, "ev-1"),
		// duplicate of chg-a by title
		ch("chg-b", "default RotationPolicy changed to ALWAYS", "", "1.18.0", nil, domain.CategoryConfiguration, true, "ev-1"),
		// covers E2 (and is a subject-duplicate of chg-d)
		ch("chg-c", "Flag --old-flag removed", "", "1.18.0", []string{"--old-flag"}, domain.CategoryRemoval, false, "ev-1"),
		ch("chg-d", "The --old-flag option is gone", "", "1.18.0", []string{"--old-flag"}, domain.CategoryRemoval, false, "ev-2"),
		// false positive
		ch("chg-e", "Bump actions in /test", "", "1.18.1", nil, domain.CategoryDependency, false, "ev-1"),
		// unsupported: cites evidence that does not exist
		ch("chg-f", "Unsupported claim", "", "1.18.0", nil, domain.CategoryOther, false, "ev-nope"),
	}
	return &domain.UpgradeEdge{
		Product: domain.ProductRef{ID: "fixture"},
		From:    domain.Version{Semver: "1.0.0"},
		To:      domain.Version{Semver: "2.0.0"},
		Changes: changes,
		Evidence: []domain.Evidence{
			ev("ev-1", "https://example/notes.md"),
			ev("ev-2", "https://example/guide.md"),
		},
	}
}

func TestScoreEntryCounts(t *testing.T) {
	c, edge := fixtureCase(), fixtureEdge()
	res := ScoreEntry(c, edge, nil, nil)

	if res.Metrics.Expected != 3 || res.Metrics.Found != 2 {
		t.Errorf("found = %d/%d, want 2/3", res.Metrics.Found, res.Metrics.Expected)
	}
	if res.Metrics.MissedMinor != 1 || res.Metrics.MissedCritical != 0 || res.Metrics.MissedImportant != 0 {
		t.Errorf("missed distribution = %+v", res.Metrics)
	}
	if res.Metrics.Changes != 6 || res.Metrics.MatchedChanges != 4 {
		t.Errorf("changes/matched = %d/%d, want 6/4", res.Metrics.Changes, res.Metrics.MatchedChanges)
	}
	if res.Metrics.FalsePositives != 1 {
		t.Errorf("falsePositives = %d, want 1 (chg-e)", res.Metrics.FalsePositives)
	}
	if res.Metrics.Unsupported != 1 {
		t.Errorf("unsupported = %d, want 1 (chg-f)", res.Metrics.Unsupported)
	}
	if res.Metrics.DuplicateGroups == 0 {
		t.Error("duplicates not detected")
	}
	if got := res.Metrics.Recall(); got != 2.0/3.0 {
		t.Errorf("recall = %v", got)
	}

	// audit trail: E1 records which change matched, by which matcher and
	// with which evidence URI.
	e1 := matchByID(res.Matches, "E1")
	if !e1.Found || len(e1.Hits) == 0 {
		t.Fatalf("E1 audit = %+v", e1)
	}
	if e1.Hits[0].ChangeID != "chg-a" || e1.Hits[0].Evidence != "https://example/notes.md" {
		t.Errorf("E1 hit = %+v", e1.Hits[0])
	}
	// the miss is recorded
	if m := matchByID(res.Matches, "E3"); m.Found {
		t.Error("E3 must be missed")
	}
}

func matchByID(ms []MatchAudit, id string) *MatchAudit {
	for i := range ms {
		if ms[i].ExpectedID == id {
			return &ms[i]
		}
	}
	return nil
}

func TestScoreEntryPipelineErrorMissesEverything(t *testing.T) {
	res := ScoreEntry(fixtureCase(), nil, nil, context.Canceled)
	if res.Error == "" || res.Metrics.Found != 0 || res.Metrics.MissedMinor+res.Metrics.MissedImportant+res.Metrics.MissedCritical != 3 {
		t.Errorf("error run = %+v", res.Metrics)
	}
}

func TestDuplicateDetection(t *testing.T) {
	changes := []domain.Change{
		ch("a", "Same title", "", "1", nil, domain.CategoryOther, false, "e"),
		ch("b", "same   TITLE", "", "1", nil, domain.CategoryOther, false, "e"),
		ch("c", "One flag removed", "", "1", []string{"x"}, domain.CategoryRemoval, false, "e"),
		ch("d", "Flag removed", "", "1", []string{"x"}, domain.CategoryRemoval, false, "e"),
		ch("e2", "Totally different", "", "1", []string{"y"}, domain.CategoryFeature, false, "e"),
	}
	groups, n := findDuplicates(changes)
	if n != 2 {
		t.Errorf("groups = %d, want 2 (title dup + subject dup): %+v", n, groups)
	}
}

func TestEnvironmentScoring(t *testing.T) {
	c := fixtureCase()
	c.Environment = &Environment{
		Kubernetes: "1.29",
		ExpectedImpact: []ImpactLink{
			{Expected: "E1", Relevance: RelevanceReview},
			{Expected: "E2", Relevance: RelevanceActionRequired},
			{Expected: "E3", Relevance: RelevanceNotAffected},
		},
		ExpectedFindings: []ExpectedFinding{
			{ID: "F1", Match: FindingMatcher{Subject: "values.key", Rule: "impact:values-pinned", Classification: "informational"}},
			{ID: "F2", Match: FindingMatcher{Subject: "missing.key"}},
		},
		NotExpectedFindings: []NotExpectedFinding{
			{Title: "no CRD removed", Match: FindingMatcher{Rule: "impact:crd-removed"}},
		},
	}
	edge := fixtureEdge()
	up := []domain.EvidenceID{"ev-1"}
	loc := []domain.EvidenceID{"ev-local"}
	report := &domain.ImpactReport{
		Evidence:            []domain.Evidence{ev("ev-1", "https://example/notes.md")},
		EnvironmentEvidence: []domain.Evidence{ev("ev-local", "file://values.yaml")},
		Findings: []domain.ImpactFinding{
			{ID: "f-1", Rule: "impact:values-pinned", Classification: domain.ImpactInformational,
				ChangeID: "chg-a", Matches: []domain.ImpactMatch{{Subject: "values.key"}},
				UpstreamEvidence: up, EnvironmentEvidence: loc},
			// joins chg-c (E2) via a different rule
			{ID: "f-2", Rule: "impact:values-removed", Classification: domain.ImpactActionRequired,
				ChangeID: "chg-c", UpstreamEvidence: up, EnvironmentEvidence: loc},
			// joins a change that does not exist; and matches the
			// notExpectedFindings rule (crd-removed)
			{ID: "f-3", Rule: "impact:crd-removed", Classification: domain.ImpactReviewRequired,
				ChangeID: "chg-nope", UpstreamEvidence: up, EnvironmentEvidence: loc},
		},
	}
	res := ScoreEntry(c, edge, report, nil)
	em := res.Env
	if em == nil {
		t.Fatal("env metrics missing")
	}
	if em.ImpactLinks != 2 || em.ImpactLinksHit != 2 {
		t.Errorf("impact links = %d hit %d, want 2/2 (E1 via f-1, E2 via f-2)", em.ImpactLinks, em.ImpactLinksHit)
	}
	if em.NotAffectedLinks != 1 || em.NotAffectedViolations != 0 {
		t.Errorf("not-affected = %d violations %d, want 1/0", em.NotAffectedLinks, em.NotAffectedViolations)
	}
	if em.FindingsExpected != 2 || em.FindingsFound != 1 {
		t.Errorf("findings = %d/%d, want 1/2", em.FindingsFound, em.FindingsExpected)
	}
	if em.FindingsFP != 1 {
		t.Errorf("finding FPs = %d, want 1 (f-3 matches the notExpectedFindings rule impact:crd-removed)", em.FindingsFP)
	}
	// f-3 joins a change that does not exist in the edge: unsupported
	if em.Unsupported != 1 {
		t.Errorf("unsupported findings = %d, want 1", em.Unsupported)
	}
}

// --- regression diff ----------------------------------------------------------

func TestDiffClassifiesRegressionsAndImprovements(t *testing.T) {
	stored := StoredResult{
		CaseID:    "x",
		Metrics:   Metrics{Expected: 4, Found: 3, FalsePositives: 1, MissedImportant: 1},
		Env:       &EnvMetrics{ImpactLinks: 2, ImpactLinksHit: 2},
		MissedIDs: []string{"E4"},
	}
	worse := EntryResult{CaseID: "x", Metrics: Metrics{Expected: 4, Found: 2, MissedCritical: 1, FalsePositives: 1}}
	// Matches the way ScoreEntry would leave them (2 found of 4)
	worse.Matches = []MatchAudit{
		{ExpectedID: "E1"}, {ExpectedID: "E2"}, {ExpectedID: "E3", Found: true},
		{ExpectedID: "E4", Importance: ImportanceCritical},
	}
	deltas := Diff(stored, worse)
	if !HasRegression(deltas) {
		t.Fatalf("deltas must contain a regression: %+v", deltas)
	}
	found := false
	for _, d := range deltas {
		// stored run missed E4 only; the fresh run additionally misses E1/E2
		if d.Field == "missedIds" && d.Regression &&
			strings.Contains(d.Detail, "newly missed: E1, E2") &&
			!strings.Contains(d.Detail, "now found") {
			found = true
		}
	}
	if !found {
		t.Errorf("missedIds regression not reported: %+v", deltas)
	}

	better := EntryResult{CaseID: "x", Metrics: Metrics{Expected: 4, Found: 4, FalsePositives: 0}}
	deltas = Diff(stored, better)
	if HasRegression(deltas) {
		t.Errorf("no regression expected: %+v", deltas)
	}
	for _, d := range deltas {
		if d.Field == "falsePositives" && !d.Improvement {
			t.Errorf("fewer FPs must be an improvement: %+v", d)
		}
	}
}

func TestStoredRoundTrip(t *testing.T) {
	res := ScoreEntry(fixtureCase(), fixtureEdge(), nil, nil)
	s := StoredFromResult(res)
	if len(s.MissedIDs) != 1 || s.MissedIDs[0] != "E3" {
		t.Errorf("missedIDs = %v", s.MissedIDs)
	}
	dir := t.TempDir()
	if err := WriteStored(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadStored(dir, s.CaseID)
	if err != nil || got == nil {
		t.Fatalf("load: %v %v", got, err)
	}
	if got.Metrics.Found != s.Metrics.Found || len(got.MissedIDs) != 1 {
		t.Errorf("round trip = %+v", got)
	}
	none, err := LoadStored(dir, "other")
	if err != nil || none != nil {
		t.Errorf("absent snapshot = %v, %v; want nil, nil", none, err)
	}
}

// --- loading ----------------------------------------------------------------

func TestLoadCaseValidates(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		if err := os.WriteFile(filepath.Join(dir, "case.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	good := `id: t1
product: p
from: v1
to: v2
expected:
  - id: E1
    title: t
    kind: removal
    importance: critical
    match: [{text: 'x'}]
`
	if _, err := LoadCase(write(good)); err != nil {
		t.Fatalf("good case: %v", err)
	}
	badKind := `id: t1
product: p
from: v1
to: v2
expected:
  - id: E1
    title: t
    kind: nonsense
    importance: critical
    match: [{text: 'x'}]
`
	if _, err := LoadCase(write(badKind)); err == nil {
		t.Error("unknown kind must fail")
	}
	badRegex := `id: t1
product: p
from: v1
to: v2
expected:
  - id: E1
    title: t
    kind: removal
    importance: critical
    match: [{text: '(unclosed'}]
`
	if _, err := LoadCase(write(badRegex)); err == nil {
		t.Error("malformed regex must fail")
	}
	badLink := good + "\nenvironment:\n  expectedImpact:\n    - {expected: NOPE, relevance: review}\n"
	if _, err := LoadCase(write(badLink)); err == nil {
		t.Error("unknown expected-impact link must fail")
	}
	envNoFiles := good + "\nenvironment:\n  kubernetes: \"1.29\"\n"
	if _, err := LoadCase(write(envNoFiles)); err == nil {
		t.Error("environment without files must fail")
	}
}

func TestRunnerEnvironmentInputsAndImages(t *testing.T) {
	root := t.TempDir()
	cdir := filepath.Join(root, CasesDirName, "c1")
	edir := filepath.Join(cdir, EnvironmentDir)
	if err := os.MkdirAll(filepath.Join(edir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	for p, body := range map[string]string{
		filepath.Join(cdir, "case.yaml"): `id: c1
product: p
from: v1
to: v2
expected:
  - {id: E1, title: t, kind: removal, importance: critical, match: [{text: x}]}
environment:
  kubernetes: "1.30"
`,
		filepath.Join(edir, "values.yaml"):         "a: 1\n",
		filepath.Join(edir, "images.txt"):          "# c\nreg/x:1\n\nreg/y:2\n",
		filepath.Join(edir, "manifests", "m.yaml"): "apiVersion: v1\nkind: ConfigMap\n",
	} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := &Runner{Pipeline: fakePipeline{}, CasesDir: root}
	cases, err := r.LoadAll()
	if err != nil || len(cases) != 1 {
		t.Fatalf("LoadAll: %v %d", err, len(cases))
	}
	in, err := environmentInputs(cases[0])
	if err != nil {
		t.Fatal(err)
	}
	if in.KubernetesVersion != "1.30" || len(in.ValuesFiles) != 1 || len(in.Manifests) != 1 {
		t.Errorf("inputs = %+v", in)
	}
	if len(in.Images) != 2 || in.Images[0] != "reg/x:1" {
		t.Errorf("images = %v", in.Images)
	}
	if in.Empty() {
		t.Error("inputs must not be empty")
	}
}

// fakePipeline returns a tiny edge/report without touching the network, so
// the runner logic is testable offline.
type fakePipeline struct{}

func (fakePipeline) Upgrade(ctx context.Context, product, from, to string) (*domain.UpgradeEdge, error) {
	return fixtureEdge(), nil
}

func (fakePipeline) Impact(ctx context.Context, product, from, to string, inputs env.Inputs) (*domain.ImpactReport, error) {
	return &domain.ImpactReport{
		Findings: []domain.ImpactFinding{{
			ID: "f-1", Rule: "impact:values-pinned", Classification: domain.ImpactInformational,
			ChangeID: "chg-a",
			Matches:  []domain.ImpactMatch{{Subject: "a"}},
		}},
	}, nil
}

func TestRunnerRunScoresWithoutNetwork(t *testing.T) {
	root := t.TempDir()
	cdir := filepath.Join(root, CasesDirName, "c1")
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cdir, "case.yaml"), []byte(`id: c1
product: fixture
from: v1.0.0
to: v2.0.0
expected:
  - {id: E1, title: rotation, kind: behaviour-change, importance: critical, match: [{text: '(?i)rotationPolicy'}]}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{Pipeline: fakePipeline{}, CasesDir: root}
	cases, err := r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	results := r.Run(context.Background(), cases)
	if len(results) != 1 || results[0].Metrics.Found != 1 {
		t.Fatalf("results = %+v", results)
	}
	agg := AggregateResults(results)
	if agg.Recall != 1 || agg.Entries != 1 {
		t.Errorf("aggregate = %+v", agg)
	}
}

// Compile-time check that report rendering runs on a report with diffs.
func TestRenderTextSmoke(t *testing.T) {
	res := ScoreEntry(fixtureCase(), fixtureEdge(), nil, nil)
	rep := Report{Results: []EntryResult{res}, Aggregate: AggregateResults([]EntryResult{res})}
	if err := RenderText(&nopWriter{}, rep); err != nil {
		t.Fatal(err)
	}
	if _, err := rep.JSON(); err != nil {
		t.Fatal(err)
	}
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
