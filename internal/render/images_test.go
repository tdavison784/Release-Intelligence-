package render

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
)

// permPairFor renders the perm chart pair with a named customer values file.
func permPairFor(t *testing.T, file string) (*Engine, *Pair) {
	t.Helper()
	needTool(t, "helm")
	e := testEngine(t)
	content, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	tgt := Target{ID: "values:" + file, Kind: TargetValuesFiles, Origin: "testdata/" + file, Why: "test",
		ReleaseName: "perm", Namespace: "perm", ValuesComplete: true,
		Values: []ValuesLayer{{Origin: "testdata/" + file, Content: content}}}
	p := e.RenderPair(context.Background(), PairRequest{Product: "perm", From: "1.0.0", To: "1.1.0", Target: tgt, Scope: domain.RenderEnvironment, KubeVersion: "1.31"})
	if p.Status != PairOK {
		t.Fatalf("pair: %s %+v", p.Status, p.Failure)
	}
	return e, p
}

func permImages(pairs ...*Pair) *RenderImages {
	return &RenderImages{Product: "perm", From: "1.0.0", To: "1.1.0", Pairs: pairs}
}

const kubectlRepo = "example.org/tools/kubectl"

// PO-7a adversarial, image family: a referenced repository whose image
// changes in the customer's rendered delta is attributable; a pin that keeps
// the chart's image change out of their render is no-effect — the reference
// alone decides nothing.
func TestRenderImagesAttribution(t *testing.T) {
	// repository set, tag left to the chart: the bump reaches them
	_, loose := permPairFor(t, "perm-customer-pinrepo.yaml")
	r := permImages(loose).EvaluateImage(domain.Change{ID: "chg-img", Subjects: []string{kubectlRepo}}, kubectlRepo)
	if r.Outcome != impact.ImageAttributable {
		t.Fatalf("loose pin: %s (needed %v)", r.Outcome, r.Needed)
	}
	if len(r.Matches) != len(r.Records) || len(r.Matches) == 0 {
		t.Fatalf("attributable needs rendered matches with records: %+v", r)
	}
	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, x := range r.Records {
		pool[x.ID] = x
	}
	for i, m := range r.Matches {
		if m.Kind != domain.MatchRenderedChange || m.Evidence[0] != r.Records[i].ID || !envScope(m.Evidence, pool) {
			t.Errorf("match %d: %+v", i, m)
		}
		if !strings.Contains(m.Subject, kubectlRepo) {
			t.Errorf("match subject must name the repository: %s", m.Subject)
		}
	}
	// repository and tag pinned: nothing changes in their render
	_, pinned := permPairFor(t, "perm-customer-pinned.yaml")
	r = permImages(pinned).EvaluateImage(domain.Change{ID: "chg-img", Subjects: []string{kubectlRepo}}, kubectlRepo)
	if r.Outcome != impact.ImageNoEffect {
		t.Fatalf("pinned: %s (needed %v)", r.Outcome, r.Needed)
	}
	pool = map[domain.EvidenceID]domain.Evidence{}
	for _, x := range r.Records {
		pool[x.ID] = x
	}
	if len(r.Checks) != 1 {
		t.Fatalf("clear needs exactly one render check: %+v", r.Checks)
	}
	c := r.Checks[0]
	if c.Dimension != domain.DimensionRender || c.Render == nil || c.Render.Outcome != domain.RenderNoAttributableChange ||
		c.Render.Key != kubectlRepo || c.Render.Counterfactual || !envScope(c.Evidence, pool) {
		t.Errorf("no-effect check: %+v", c)
	}
}

// Failed renders, incomplete values or no deployment never clear.
func TestRenderImagesUndecided(t *testing.T) {
	failed := &Pair{Product: "perm", From: "1.0.0", To: "1.1.0", Scope: domain.RenderEnvironment, Status: PairFailed,
		Target:     Target{ID: "values:f", ValuesComplete: true},
		FromResult: &Result{Status: StatusSucceeded},
		Failure:    &Failure{Reason: FailChartUnavailable, Detail: "offline"}}
	incomplete := &Pair{Status: PairOK, Scope: domain.RenderEnvironment, Diff: &DiffResult{},
		Target: Target{ID: "values:i", ValuesComplete: false, IncompleteReason: "valuesFrom"}}
	for name, pairs := range map[string][]*Pair{
		"failed render": {failed}, "incomplete values": {incomplete},
		"release only": {{Status: PairOK, Scope: domain.RenderRelease, Target: Target{ID: "rel", ValuesComplete: true}}},
		"no pairs":     nil,
	} {
		r := permImages(pairs...).EvaluateImage(domain.Change{ID: "chg-img", Subjects: []string{kubectlRepo}}, kubectlRepo)
		if r.Outcome != impact.ImageUndecided || len(r.Needed) == 0 || len(r.Matches) > 0 || len(r.Checks) > 0 {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

// The join (internal/impact, PO-7a image hunk): attributable keeps today's
// review-required image-changed with rendered evidence on chain 2; pinned →
// image-render-unchanged not-affected with a render check; undecided or no
// evaluator → a byte-identical report.
func TestImagesInTheJoin(t *testing.T) {
	ev := domain.NewEvidence(domain.EvidenceStructured, "chart-diff", "https://example.org/perm/diff", "images", "image tag diff", "sha256:v", fixedNow)
	edge := &domain.UpgradeEdge{SchemaVersion: domain.UpgradeEdgeSchemaVersion, Product: domain.ProductRef{ID: "perm"},
		From: domain.Version{Tag: "1.0.0", Semver: "1.0.0"}, To: domain.Version{Tag: "1.1.0", Semver: "1.1.0"},
		Evidence: []domain.Evidence{ev},
		Changes: []domain.Change{{
			ID: "chg-img", Category: domain.CategoryArtifact, Title: "cleanup image 1.28.5 → 1.29.0", Subjects: []string{kubectlRepo},
			Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: "images:tag-changed", Confidence: domain.ConfidenceHigh},
			Evidence:   []domain.EvidenceID{ev.ID},
		}}}
	load := func(file string) *env.Environment {
		e, err := env.Load(env.Inputs{ValuesFiles: []string{"testdata/" + file}})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	build := func(set impact.ImageRenderEvaluator, envr *env.Environment) *domain.ImpactReport {
		rep, err := impact.Build(impact.Input{Edge: edge, Env: envr, Now: fixedNow, Image: set})
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}
	find := func(rep *domain.ImpactReport) domain.ImpactFinding {
		for _, f := range rep.Findings {
			if f.ChangeID == "chg-img" {
				return f
			}
		}
		t.Fatalf("no finding for chg-img: %d findings", len(rep.Findings))
		return domain.ImpactFinding{}
	}
	renderCheck := func(f domain.ImpactFinding) *domain.ImpactCheck {
		for i := range f.Checks {
			if f.Checks[i].Dimension == domain.DimensionRender {
				return &f.Checks[i]
			}
		}
		return nil
	}
	pinnedEnv := load("perm-customer-pinned.yaml")
	if len(pinnedEnv.Images) == 0 {
		t.Fatal("the pinned customer file must yield an image fact")
	}

	// without --render: today's review-required image-changed
	base := build(nil, pinnedEnv)
	if f := find(base); f.Classification != domain.ImpactReviewRequired || f.Rule != impact.RuleImageChanged {
		t.Fatalf("baseline: %s %s", f.Classification, f.Rule)
	}
	// undecided render: byte-identical report
	fake := stubImage{r: impact.ImageResult{Outcome: impact.ImageUndecided, Needed: []string{"no render decided"}}}
	und, err := json.Marshal(build(fake, pinnedEnv))
	if err != nil {
		t.Fatal(err)
	}
	baseJSON, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if string(und) != string(baseJSON) {
		t.Error("an undecided render must leave the report byte-identical")
	}

	// pinned: image-render-unchanged not-affected with a render check
	_, pinned := permPairFor(t, "perm-customer-pinned.yaml")
	rep := build(permImages(pinned), pinnedEnv)
	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, x := range rep.EnvironmentEvidence {
		pool[x.ID] = x
	}
	f := find(rep)
	c := renderCheck(f)
	if f.Classification != domain.ImpactNotAffected || f.Rule != impact.RuleImageRenderUnchanged || c == nil || c.Render == nil ||
		c.Render.Outcome != domain.RenderNoAttributableChange || c.Render.Key != kubectlRepo || !envScope(c.Evidence, pool) {
		t.Errorf("pinned: %s %s %+v", f.Classification, f.Rule, c)
	}

	// repository set, tag loose: attributable — today's class with rendered
	// evidence on chain 2
	looseEnv := load("perm-customer-pinrepo.yaml")
	_, loose := permPairFor(t, "perm-customer-pinrepo.yaml")
	rep = build(permImages(loose), looseEnv)
	pool = map[domain.EvidenceID]domain.Evidence{}
	for _, x := range rep.EnvironmentEvidence {
		pool[x.ID] = x
	}
	f = find(rep)
	if f.Classification != domain.ImpactReviewRequired || f.Rule != impact.RuleImageChanged {
		t.Fatalf("attributable: %s %s", f.Classification, f.Rule)
	}
	anyRender := false
	for _, id := range f.EnvironmentEvidence {
		if x, ok := pool[id]; ok && x.Render != nil && x.Render.Scope == domain.RenderEnvironment {
			anyRender = true
		}
	}
	if !anyRender {
		t.Errorf("attributable needs environment-render evidence on chain 2: %d records", len(f.EnvironmentEvidence))
	}
}

// stubImage is an ImageRenderEvaluator returning one canned result.
type stubImage struct{ r impact.ImageResult }

func (s stubImage) EvaluateImage(domain.Change, string) impact.ImageResult { return s.r }
