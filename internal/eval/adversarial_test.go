package eval

// The adversarial environment pack (G12): eval/adversarial/<id>/ holds
// environments designed to fool a simplistic join, with expected outcomes
// derived from the contract (docs/ACTION_CLASSIFICATION.md, docs/IMPACT.md).
// Each fixture declares a synthetic UpgradeEdge; the test builds it, joins it
// with the fixture's environment through the REAL join (internal/impact), and
// asserts what must and must not be concluded. Fully offline and
// deterministic.

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"gopkg.in/yaml.v3"
)

const adversarialDir = "../../eval/adversarial"

var adversarialNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// advFixture is the parsed adversarial.yaml.
type advFixture struct {
	ID          string `yaml:"id"`
	Attacks     string `yaml:"attacks"`
	Expectation string `yaml:"expectation"`
	Edge        struct {
		Changes []struct {
			Rule      string   `yaml:"rule"`
			Title     string   `yaml:"title"`
			Detail    string   `yaml:"detail"`
			Subjects  []string `yaml:"subjects"`
			Category  string   `yaml:"category"`
			ActionReq *bool    `yaml:"actionRequired"`
		} `yaml:"changes"`
		Compatibility []struct {
			Platform string   `yaml:"platform"`
			Kind     string   `yaml:"kind"`
			Versions []string `yaml:"versions"`
		} `yaml:"compatibility"`
	} `yaml:"edge"`
	Environment struct {
		Kubernetes string   `yaml:"kubernetes"`
		Files      []string `yaml:"files"`
	} `yaml:"environment"`
	Expect struct {
		Present []FindingMatcher `yaml:"present"`
		Absent  []FindingMatcher `yaml:"absent"`
		// DuplicateGroups: exact expected count of duplicate groups over the
		// fixture's changes (nil = not asserted).
		DuplicateGroups *int `yaml:"duplicateGroups"`
	} `yaml:"expect"`
}

// TestAdversarialPack runs every fixture under eval/adversarial and asserts
// the pack stays a pack: if a fixture stops parsing or a directory
// disappears, the test must say so rather than silently measure less.
func TestAdversarialPack(t *testing.T) {
	entries, err := os.ReadDir(adversarialDir)
	if err != nil {
		t.Fatalf("adversarial pack not found: %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(adversarialDir, e.Name(), "adversarial.yaml")); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	if len(names) < 10 {
		t.Fatalf("adversarial pack shrank to %d fixtures; investigate before lowering", len(names))
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			runAdversarialFixture(t, filepath.Join(adversarialDir, name))
		})
	}
}

func runAdversarialFixture(t *testing.T, dir string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "adversarial.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var f advFixture
	if err := yaml.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.ID == "" || f.Edge.Changes == nil {
		t.Fatalf("fixture %s: id and edge.changes are required", dir)
	}
	if f.ID != filepath.Base(dir) {
		t.Errorf("fixture id %q does not match directory %q", f.ID, filepath.Base(dir))
	}

	edge := buildAdversarialEdge(t, f)

	// environment from the fixture's files
	var inputs env.Inputs
	inputs.KubernetesVersion = f.Environment.Kubernetes
	for _, name := range f.Environment.Files {
		p := filepath.Join(dir, name)
		switch {
		case name == "values.yaml":
			inputs.ValuesFiles = append(inputs.ValuesFiles, p)
		case name == "images.txt":
			imgs, err := readImagesFile(p)
			if err != nil {
				t.Fatal(err)
			}
			inputs.Images = imgs
		case name == "crds" || strings.HasSuffix(name, "/crds"):
			inputs.CRDs = append(inputs.CRDs, p)
		default:
			inputs.Manifests = append(inputs.Manifests, p)
		}
	}
	e, err := env.Load(inputs)
	if err != nil {
		t.Fatalf("environment: %v", err)
	}

	rep, err := impact.Build(impact.Input{Edge: edge, Env: e, Now: adversarialNow})
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := rep.Validate(); err != nil {
		t.Fatalf("report invalid: %v", err)
	}

	// contract: every present matcher must match ≥1 finding; every absent
	// matcher must match none.
	matchCount := func(m FindingMatcher) int {
		n := 0
		for _, finding := range rep.Findings {
			if matcherMatchesFixture(m, finding) {
				n++
			}
		}
		return n
	}
	for _, m := range f.Expect.Present {
		if matchCount(m) == 0 {
			t.Errorf("expected finding %s not produced; got:\n%s", m.String(), renderFindings(rep))
		}
	}
	for _, m := range f.Expect.Absent {
		if n := matchCount(m); n > 0 {
			t.Errorf("FORBIDDEN finding %s produced %d time(s) — the join was fooled:\n%s", m.String(), n, renderFindings(rep))
		}
	}
	if f.Expect.DuplicateGroups != nil {
		if _, groups := findDuplicates(edge.Changes); groups != *f.Expect.DuplicateGroups {
			t.Errorf("duplicate groups = %d, want %d", groups, *f.Expect.DuplicateGroups)
		}
	}
}

// matcherMatchesFixture matches a finding against the set fields of a
// fixture matcher (rule, classification, subject).
func matcherMatchesFixture(m FindingMatcher, f domain.ImpactFinding) bool {
	if m.Rule != "" && f.Rule != m.Rule {
		return false
	}
	if m.Classification != "" && string(f.Classification) != m.Classification {
		return false
	}
	if m.Subject != "" {
		found := false
		for _, match := range f.Matches {
			if match.Subject == m.Subject {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// buildAdversarialEdge constructs a valid UpgradeEdge from the fixture
// declaration (computed provenance, one evidence record per change).
func buildAdversarialEdge(t *testing.T, f advFixture) *domain.UpgradeEdge {
	t.Helper()
	edge := &domain.UpgradeEdge{
		SchemaVersion: domain.UpgradeEdgeSchemaVersion,
		Product:       domain.ProductRef{ID: domain.ProductID("adversarial-" + f.ID), Name: f.ID},
		From:          domain.MustVersion("v1.0.0", "1.0.0"),
		To:            domain.MustVersion("v2.0.0", "2.0.0"),
		Path:          []domain.PathStep{{Version: domain.MustVersion("v2.0.0", "2.0.0"), Reason: "major-release"}},
	}
	for i, c := range f.Edge.Changes {
		uri := "https://adversarial.example/" + f.ID + "/" + c.Rule + "/" + string(rune('a'+i))
		e := domain.NewEvidence(domain.EvidenceStructured, "notes", uri, "L1", c.Title,
			domain.Digest([]byte(uri)), adversarialNow)
		edge.Evidence = append(edge.Evidence, e)
		ch := domain.Change{
			ID:       "chg-" + domain.ShortHash(c.Rule, c.Title),
			Title:    c.Title,
			Detail:   c.Detail,
			Subjects: c.Subjects,
			Category: domain.Category(c.Category),
			Provenance: domain.Provenance{
				Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: c.Rule,
				Confidence: domain.ConfidenceHigh,
			},
			Evidence: []domain.EvidenceID{e.ID},
		}
		if c.ActionReq != nil {
			ch.ActionRequired = *c.ActionReq
		}
		edge.Changes = append(edge.Changes, ch)
	}
	for _, cc := range f.Edge.Compatibility {
		e := domain.NewEvidence(domain.EvidenceStructured, "compat", "https://adversarial.example/"+f.ID+"/compat", "L2", strings.Join(cc.Versions, ", "),
			domain.Digest([]byte("compat:"+f.ID)), adversarialNow)
		edge.Evidence = append(edge.Evidence, e)
		mk := func() *domain.CompatibilityConstraint {
			return &domain.CompatibilityConstraint{
				Platform: cc.Platform, Kind: cc.Kind, Versions: cc.Versions,
				Raw: strings.Join(cc.Versions, ", "),
				Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.table@v1",
					Confidence: domain.ConfidenceHigh},
				Evidence: []domain.EvidenceID{e.ID},
			}
		}
		edge.Compatibility = append(edge.Compatibility, domain.CompatibilityChange{
			Platform: cc.Platform, From: mk(), To: mk(),
		})
	}
	return edge
}

// renderFindings dumps the report's findings for failure messages.
func renderFindings(rep *domain.ImpactReport) string {
	var b strings.Builder
	for _, f := range rep.Findings {
		subjects := make([]string, 0, len(f.Matches))
		for _, m := range f.Matches {
			subjects = append(subjects, m.Subject)
		}
		sort.Strings(subjects)
		b.WriteString("  " + string(f.Classification) + " " + f.Rule + " [" + strings.Join(subjects, ", ") + "] " + f.Title + "\n")
	}
	return b.String()
}

// TestUpstreamUnavailableIsExecutionFailure pins the pipeline-failure
// semantics (G10/G11): an edge that cannot be built is an execution error,
// counted in pipelineFailures — never a silent "found nothing" and never a
// reasoning miss.
func TestUpstreamUnavailableIsExecutionFailure(t *testing.T) {
	c := fixtureCase()
	res := ScoreEntry(c, nil, nil, context.DeadlineExceeded)
	if res.Error == "" {
		t.Fatal("no error recorded")
	}
	if res.Metrics.Found != 0 || res.Metrics.Found != res.Metrics.Expected-res.Metrics.Missed() {
		t.Errorf("a failed pipeline finds nothing: %+v", res.Metrics)
	}
	agg := AggregateResults([]EntryResult{res})
	if agg.PipelineFailures != 1 {
		t.Errorf("pipelineFailures = %d, want 1", agg.PipelineFailures)
	}
	// the failure keeps the entry's ground truth (the expectations are data)
	if agg.Expected != len(c.Expected) {
		t.Errorf("expected items lost on failure: %d", agg.Expected)
	}
}
