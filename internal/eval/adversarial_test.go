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
	"errors"
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
			Release   string   `yaml:"release"`
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
		Inventory  string   `yaml:"inventory"`
		Files      []string `yaml:"files"`
	} `yaml:"environment"`
	// Knowledge declares verified facts the join must evaluate (the knowledge
	// traps): each fact is hand-built from the contract's own shapes, never
	// from eval expectations.
	Knowledge struct {
		MinVerification string    `yaml:"minVerification"`
		Facts           []advFact `yaml:"facts"`
	} `yaml:"knowledge"`
	Expect struct {
		Present []FindingMatcher `yaml:"present"`
		Absent  []FindingMatcher `yaml:"absent"`
		// DuplicateGroups: exact expected count of duplicate groups over the
		// fixture's changes (nil = not asserted).
		DuplicateGroups *int `yaml:"duplicateGroups"`
	} `yaml:"expect"`
}

// advCondition mirrors domain.Condition in YAML (the applicability condition
// language of DESIGN.md §1.3).
type advCondition struct {
	Op        string         `yaml:"op"`
	Of        []advCondition `yaml:"of"`
	Group     string         `yaml:"group"`
	Version   string         `yaml:"version"`
	Kind      string         `yaml:"kind"`
	Name      string         `yaml:"name"`
	Path      string         `yaml:"path"`
	Component string         `yaml:"component"`
	State     string         `yaml:"state"`
	Values    []string       `yaml:"values"`
	Pattern   string         `yaml:"pattern"`
	Separator string         `yaml:"separator"`
	Range     string         `yaml:"range"`
}

func (a advCondition) toCondition() domain.Condition {
	c := domain.Condition{
		Op:        domain.ConditionOp(a.Op),
		Group:     a.Group,
		Version:   a.Version,
		Kind:      a.Kind,
		Name:      a.Name,
		Path:      a.Path,
		Component: a.Component,
		State:     domain.FieldState(a.State),
		Values:    a.Values,
		Pattern:   a.Pattern,
		Separator: a.Separator,
		Range:     a.Range,
	}
	for _, sub := range a.Of {
		c.Of = append(c.Of, sub.toCondition())
	}
	return c
}

// advFact is a verified fact declared for a knowledge trap. The harness
// derives id, product, release, exposed class and provenance so the fixture
// states only what the attack needs.
type advFact struct {
	Level           string `yaml:"level"` // deterministic|human|consensus|proxy
	ConsensusAction bool   `yaml:"consensusAction"`
	Subject         struct {
		Family    string `yaml:"family"`
		Group     string `yaml:"group"`
		Version   string `yaml:"version"`
		Kind      string `yaml:"kind"`
		Name      string `yaml:"name"`
		Path      string `yaml:"path"`
		Component string `yaml:"component"`
	} `yaml:"subject"`
	Change struct {
		Type   string  `yaml:"type"`
		Before *string `yaml:"before"`
		After  *string `yaml:"after"`
	} `yaml:"change"`
	Exposure    advCondition  `yaml:"exposure"`
	Overlap     *advCondition `yaml:"overlap"`
	Consequence struct {
		Kind        string `yaml:"kind"`
		Statement   string `yaml:"statement"`
		Remediation string `yaml:"remediation"`
	} `yaml:"consequence"`
	Statement string `yaml:"statement"`
	// Anchors are titles of the fixture's own changes (statement anchors).
	Anchors []string `yaml:"anchors"`
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
	if f.Environment.Inventory != "" {
		inputs.Inventory = filepath.Join(dir, f.Environment.Inventory)
	}
	e, err := env.Load(inputs)
	if err != nil {
		t.Fatalf("environment: %v", err)
	}

	in := impact.Input{Edge: edge, Env: e, Now: adversarialNow}
	if len(f.Knowledge.Facts) > 0 {
		in.Facts, in.MinVerification = buildAdvFacts(t, f, edge)
	}
	rep, err := impact.Build(in)
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

// advEdgeLookup resolves a fixture edge's evidence ids (for anchors).
func advEdgeLookup(edge *domain.UpgradeEdge) func(domain.EvidenceID) (domain.Evidence, bool) {
	ix := make(map[domain.EvidenceID]domain.Evidence, len(edge.Evidence))
	for _, e := range edge.Evidence {
		ix[e.ID] = e
	}
	return func(id domain.EvidenceID) (domain.Evidence, bool) {
		e, ok := ix[id]
		return e, ok
	}
}

// buildAdvFacts compiles the fixture's declared facts into valid
// domain.VerifiedFacts for the join: every aspect at the declared level,
// statement anchors resolved against the fixture's own changes, and the
// exposed class following the consequence kind. The facts are hand-built from
// the contract (DESIGN.md §1, §2.6), never from eval expectations.
func buildAdvFacts(t *testing.T, f advFixture, edge *domain.UpgradeEdge) ([]domain.VerifiedFact, domain.VerificationLevel) {
	t.Helper()
	level := domain.VerificationLevel("human") // Build's default gate level
	if f.Knowledge.MinVerification != "" {
		level = domain.VerificationLevel(f.Knowledge.MinVerification)
		if !level.Valid() {
			t.Fatalf("fixture %s: minVerification %q is not a level", f.ID, f.Knowledge.MinVerification)
		}
	}
	byTitle := map[string]domain.Change{}
	for _, c := range edge.Changes {
		byTitle[c.Title] = c
	}
	var facts []domain.VerifiedFact
	for i, af := range f.Knowledge.Facts {
		lvl := domain.VerificationLevel(af.Level)
		if !lvl.Valid() {
			t.Fatalf("fixture %s: fact %d level %q is not a level", f.ID, i, af.Level)
		}
		release := edge.To.Semver
		subject := &domain.Subject{Family: domain.SubjectFamily(af.Subject.Family), Product: edge.Product.ID,
			Group: af.Subject.Group, Version: af.Subject.Version, Kind: af.Subject.Kind,
			Name: af.Subject.Name, Path: af.Subject.Path, Component: af.Subject.Component}
		cons := &domain.Consequence{Kind: domain.ConsequenceKind(af.Consequence.Kind),
			ExposedClass: domain.ConsequenceKind(af.Consequence.Kind).ExposedClass(),
			Statement:    af.Consequence.Statement, Remediation: af.Consequence.Remediation}
		a := domain.SemanticAssertion{
			Subject: subject,
			Change:  &domain.ChangeSpec{Type: domain.ChangeKind(af.Change.Type), Before: af.Change.Before, After: af.Change.After},
			Applicability: &domain.Applicability{Exposure: af.Exposure.toCondition(), Overlap: func() *domain.Condition {
				if af.Overlap == nil {
					return nil
				}
				c := af.Overlap.toCondition()
				return &c
			}()},
			Consequence: cons,
			Statement:   af.Statement,
		}
		fact := domain.VerifiedFact{
			Product: edge.Product.ID, Release: release, Candidates: []string{"sc-000000000001"},
			Assertion: a, Status: domain.FactActive, CreatedAt: adversarialNow,
			Evidence: []domain.Evidence{domain.NewEvidence(domain.EvidenceDocument, "docs",
				"https://adversarial.example/"+f.ID+"/fact", "L1", af.Statement,
				domain.Digest([]byte(f.ID+af.Statement)), adversarialNow)},
		}
		for _, title := range af.Anchors {
			c, ok := byTitle[title]
			if !ok {
				t.Fatalf("fixture %s: fact %d anchors unknown change %q", f.ID, i, title)
			}
			fact.Anchors = append(fact.Anchors, domain.NewChangeAnchor(c, advEdgeLookup(edge)))
		}
		for _, x := range domain.Aspects {
			v := domain.AspectVerification{Aspect: x, Level: lvl}
			switch lvl {
			case domain.VerifiedDeterministic:
				v.Basis = []string{"val-" + string(x)}
			case domain.VerifiedConsensus:
				v.Basis = []string{"sp-a" + string(x), "sp-b" + string(x)}
				v.Consensus = domain.ConsensusCrossModel
			default: // human, proxy: an accept/correct decision
				v.Basis = []string{"rd-" + string(x)}
			}
			fact.Verification = append(fact.Verification, v)
		}
		fact.AutoApproved = lvl == domain.VerifiedDeterministic || lvl == domain.VerifiedConsensus
		fact.ConsensusAction = af.ConsensusAction
		fact.ID = domain.VerifiedFactID(fact.Product, fact.Release, fact.Assertion)
		if err := fact.Validate(); err != nil {
			t.Fatalf("fixture %s: fact %d is invalid: %v", f.ID, i, err)
		}
		facts = append(facts, fact)
	}
	return facts, level
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
		method, producer := domain.MethodComputed, "upgrade@v1"
		if c.Rule == "note" {
			// the pack's documented vocabulary: `note` declares a note-derived
			// change (docs/eval/adversarial/README.md), which statement
			// anchors attach to (DESIGN.md §2.6)
			method, producer = domain.MethodDeclared, "notes@v1"
		}
		ch := domain.Change{
			ID:       "chg-" + domain.ShortHash(c.Rule, c.Title),
			Title:    c.Title,
			Detail:   c.Detail,
			Release:  c.Release,
			Subjects: c.Subjects,
			Category: domain.Category(c.Category),
			Provenance: domain.Provenance{
				Method: method, Producer: producer, Rule: c.Rule,
				Confidence: domain.ConfidenceHigh,
			},
			Evidence: []domain.EvidenceID{e.ID},
		}
		if ch.Release == "" {
			ch.Release = edge.To.Semver // facts anchor changes at their release
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

// A report the pipeline could not produce must neither shrink the
// applicability denominator nor earn not-affected credit. Regression
// (trust-audit): in the proxy-incl-shadow view, kyverno's knowledge report
// failed validation; runCase recorded a nil report without an error, so the
// case's links vanished from every applicability count (71 → 62 links at the
// proxy level), its findings — including a false ACTION — disappeared, and no
// pipeline failure was reported. A missing report is an execution failure,
// and its links score as misses.
func TestMissingReportKeepsLinksAndFails(t *testing.T) {
	c := fixtureCase()
	c.Environment = &Environment{
		ExpectedImpact: []ImpactLink{
			{Expected: "E1", Relevance: RelevanceActionRequired},
			{Expected: "E2", Relevance: RelevanceReview},
			{Expected: "E3", Relevance: RelevanceNotAffected},
		},
	}
	res := ScoreEntry(c, fixtureEdge(), nil, errors.New("impact: assembled report is invalid"))
	agg := AggregateResults([]EntryResult{res})
	if agg.PipelineFailures != 1 {
		t.Errorf("pipelineFailures = %d, want 1: a missing report is an execution failure", agg.PipelineFailures)
	}
	if agg.ImpactLinks != 2 || agg.ImpactLinksHit != 0 {
		t.Errorf("affected links = %d hit %d, want 2 hit 0: the denominator must not shrink", agg.ImpactLinks, agg.ImpactLinksHit)
	}
	if agg.NotAffectedLinks != 1 || agg.NotAffectedViolations != 1 {
		t.Errorf("not-affected links = %d violations %d, want 1/1: a report that does not exist clears nothing", agg.NotAffectedLinks, agg.NotAffectedViolations)
	}
	if agg.ApplicabilityAccuracy != 0 {
		t.Errorf("applicabilityAccuracy = %v, want 0 for a case without a report", agg.ApplicabilityAccuracy)
	}
	// the same holds when the runner itself sees the failure
	root := t.TempDir()
	r := &Runner{Pipeline: failingImpactPipeline{}, CasesDir: root}
	cases := levelCase(t, r)
	cases[0].Environment.ExpectedImpact = []ImpactLink{{Expected: "E1", Relevance: RelevanceActionRequired}}
	got := AggregateResults(r.Run(context.Background(), cases))
	if got.PipelineFailures != 1 || got.ImpactLinks != 1 {
		t.Errorf("runner: pipelineFailures %d impactLinks %d, want 1/1", got.PipelineFailures, got.ImpactLinks)
	}
}

// failingImpactPipeline produces the edge but fails every impact join.
type failingImpactPipeline struct{ fakePipeline }

func (failingImpactPipeline) Impact(ctx context.Context, product, from, to string, inputs env.Inputs) (*domain.ImpactReport, error) {
	return nil, errors.New("impact: assembled report is invalid")
}
