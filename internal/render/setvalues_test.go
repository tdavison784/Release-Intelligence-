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

// rejTarget is the customer's values-files target for the rej chart.
func rejTarget(t *testing.T) Target {
	t.Helper()
	content, err := os.ReadFile("testdata/rej-customer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return Target{ID: "values:rej-customer", Kind: TargetValuesFiles, Origin: "testdata/rej-customer.yaml", Why: "test",
		ReleaseName: "rej", Namespace: "rej", ValuesComplete: true,
		Values: []ValuesLayer{{Origin: "testdata/rej-customer.yaml", Content: content}}}
}

// rejPair renders the customer's pair 1.0.0→to (live helm).
func rejPair(t *testing.T, to string) (*Engine, *Pair) {
	t.Helper()
	needTool(t, "helm")
	e := testEngine(t)
	p := e.RenderPair(context.Background(), PairRequest{Product: "rej", From: "1.0.0", To: to, Target: rejTarget(t), Scope: domain.RenderEnvironment, KubeVersion: "1.31"})
	return e, p
}

func rejSet(e *Engine, to string, pairs ...*Pair) *SetValues {
	return &SetValues{Engine: e, Product: "rej", From: "1.0.0", To: to, Pairs: pairs, KubeVersion: "1.31", Ctx: context.Background()}
}

func envScope(ids []domain.EvidenceID, pool map[domain.EvidenceID]domain.Evidence) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if x, ok := pool[id]; !ok || x.Render == nil || x.Render.Scope != domain.RenderEnvironment {
			return false
		}
	}
	return true
}

// PO-7a adversarial: for a values key the customer sets whose key the target
// removes, the render decides — attributable only when the key's removal
// traces to the customer's upgrade delta, no-effect only when every complete
// render attributes nothing, never a blanket guess.
func TestSetValuesAttribution(t *testing.T) {
	e, p := rejPair(t, "1.1.0")
	if p.Status != PairOK {
		t.Fatalf("pair: %s %+v", p.Status, p.Failure)
	}
	s := rejSet(e, "1.1.0", p)
	// the key the source chart read: its removal is attributable, with
	// environment-render evidence on chain 2
	r := s.EvaluateSetValues(domain.Change{ID: "chg-x", Subjects: []string{"legacy.feature"}})
	if r.Outcome != impact.SetValuesAttributable {
		t.Fatalf("feature: %s (needed %v, refusal %q)", r.Outcome, r.Needed, r.Refusal)
	}
	if len(r.Matches) == 0 || len(r.Matches) != len(r.Records) {
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
		if !strings.Contains(m.Subject, "legacy.feature") || !strings.Contains(m.Subject, "rej-customer") {
			t.Errorf("match subject must name the key and the deployment: %s", m.Subject)
		}
	}
	// the key no template ever read: nothing attributable, with a render check
	r = s.EvaluateSetValues(domain.Change{ID: "chg-y", Subjects: []string{"legacy.dead"}})
	if r.Outcome != impact.SetValuesNoEffect {
		t.Fatalf("dead: %s (needed %v)", r.Outcome, r.Needed)
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
		!c.Render.Counterfactual || c.Render.Key != "legacy.dead" || !envScope(c.Evidence, pool) {
		t.Errorf("no-effect check: %+v", c)
	}
}

// The target refuses the customer's values at render time and names the
// removed section: rejected, decisive standalone, with the refusal as
// environment-render evidence.
func TestSetValuesRejected(t *testing.T) {
	e, p := rejPair(t, "2.0.0")
	if !p.TargetRejects() {
		t.Fatalf("2.0.0 must refuse the customer's values: %s %+v", p.Status, p.Failure)
	}
	r := rejSet(e, "2.0.0", p).EvaluateSetValues(domain.Change{ID: "chg-x", Subjects: []string{"legacy.feature"}})
	if r.Outcome != impact.SetValuesRejected {
		t.Fatalf("%s (needed %v)", r.Outcome, r.Needed)
	}
	if !strings.Contains(r.Refusal, "legacy") {
		t.Errorf("refusal must name the removed key('s section): %q", r.Refusal)
	}
	if r.Rejection == nil || r.Rejection.Kind != domain.MatchRenderedChange || len(r.Rejection.Evidence) != 1 || len(r.Records) != 1 {
		t.Fatalf("rejection match: %+v records %d", r.Rejection, len(r.Records))
	}
	rec := r.Records[0]
	if r.Rejection.Evidence[0] != rec.ID || !envScope(r.Rejection.Evidence, map[domain.EvidenceID]domain.Evidence{rec.ID: rec}) {
		t.Errorf("rejection must cite environment-render evidence: %+v", rec)
	}
	if rec.Render == nil || rec.Render.Change != "render-rejected" {
		t.Errorf("rejection record: %+v", rec.Render)
	}
}

// No render decides: a refusal naming an unrelated key, keys set via --set,
// no Helm deployment — never a rejection, attribution or clear.
func TestSetValuesUndecided(t *testing.T) {
	rejecting := func(detail string) *Pair {
		return &Pair{Product: "rej", From: "1.0.0", To: "2.0.0", Scope: domain.RenderEnvironment, Status: PairFailed,
			Target:     Target{ID: "values:rej-customer", ValuesComplete: true},
			FromResult: &Result{Status: StatusSucceeded},
			ToResult:   &Result{Status: StatusFailed, Failure: &Failure{Reason: FailInvalidValues, Detail: detail}}}
	}
	cases := map[string][]*Pair{
		"refusal names an unrelated key": {rejecting("Additional property other is not allowed")},
		"refusal names nothing":          {rejecting("values don't meet the specifications of the schema")},
		"key set via --set": {{Status: PairOK, Scope: domain.RenderEnvironment,
			Target: Target{ID: "values:x", ValuesComplete: true, Set: []SetValue{{Path: "legacy.feature", Value: `"custom"`}}}}},
		"kustomize only": {{Status: PairOK, Target: Target{ID: "kz", Kind: TargetKustomize, ValuesComplete: true}}},
		"release only":   {{Status: PairOK, Scope: domain.RenderRelease, Target: Target{ID: "rel", ValuesComplete: true}}},
		"no pairs":       nil,
	}
	for name, pairs := range cases {
		r := rejSet(nil, "2.0.0", pairs...).EvaluateSetValues(domain.Change{ID: "chg-x", Subjects: []string{"legacy.feature"}})
		if r.Outcome != impact.SetValuesUndecided || len(r.Needed) == 0 || r.Rejection != nil || len(r.Matches) > 0 || len(r.Checks) > 0 {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

// The refusal matcher: dotted path or any ancestor prefix on word boundaries.
func TestRefusalNames(t *testing.T) {
	detail := `values don't meet the specifications of the schema(s): - (root): Additional property legacy is not allowed`
	cases := []struct {
		detail, key string
		want        bool
	}{
		{detail, "legacy.feature", true}, // ancestor named (root-level schema errors)
		{detail, "legacy", true},         // exact
		{detail, "legacy.dead", true},    // same section
		{detail, "other.key", false},     // unrelated
		{"execution error at (rej/templates/x.yaml): legacy.feature must go", "legacy.feature", true}, // chart-authored fail
		{"Additional property legac is not allowed", "legacy.feature", false},                         // no prefix slack
		{"Additional property mylegacy is not allowed", "legacy.feature", false},                      // word boundary
	}
	for _, tc := range cases {
		if got := refusalNames(tc.detail, []string{tc.key}); got != tc.want {
			t.Errorf("refusalNames(%q, %s) = %v, want %v", tc.detail, tc.key, got, tc.want)
		}
	}
}

func TestNullLayer(t *testing.T) {
	b, why := nullLayer([]string{"legacy.feature", "legacy.dead"})
	if why != "" || strings.TrimSpace(string(b)) != "legacy:\n    dead: null\n    feature: null" {
		t.Fatalf("nullLayer: %q %q", b, why)
	}
}

func TestPathOverlaps(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"legacy.feature", "legacy.feature", true},
		{"legacy", "legacy.feature", true},
		{"legacy.feature", "legacy", true},
		{"legacy.feature", "legacy.featureX", false},
		{"legacy.feature", "legacyx.feature", false},
		{"legacy", "legacy[0].feature", true},
		{"a.b", "a.b.c", true},
	}
	for _, tc := range cases {
		if got := pathOverlaps(tc.a, tc.b); got != tc.want {
			t.Errorf("pathOverlaps(%q, %q) = %v", tc.a, tc.b, got)
		}
	}
}

// The join (internal/impact, PO-7a hunk): rejected → render-target-rejects
// action-required citing the refusal; no-effect → values-set-no-effect
// not-affected with a render check; attributable → today's values-removed
// action-required with rendered matches on chain 2; undecided or no evaluator
// → a byte-identical report.
func TestSetValuesInTheJoin(t *testing.T) {
	ev := domain.NewEvidence(domain.EvidenceStructured, "helm-values", "https://example.org/rej/values.yaml", "values.yaml", "values diff", "sha256:v", fixedNow)
	mk := func(id string, keys ...string) domain.Change {
		return domain.Change{ID: id, Category: domain.CategoryHelmValues, Title: id, Subjects: keys,
			Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: "values:removed", Confidence: domain.ConfidenceHigh},
			Evidence:   []domain.EvidenceID{ev.ID}}
	}
	edge := &domain.UpgradeEdge{SchemaVersion: domain.UpgradeEdgeSchemaVersion, Product: domain.ProductRef{ID: "rej"},
		From: domain.Version{Tag: "1.0.0", Semver: "1.0.0"}, To: domain.Version{Tag: "1.1.0", Semver: "1.1.0"},
		Evidence: []domain.Evidence{ev},
		Changes: []domain.Change{
			mk("chg-feature", "legacy.feature"),
			mk("chg-dead", "legacy.dead"),
		}}
	envr, err := env.Load(env.Inputs{ValuesFiles: []string{"testdata/rej-customer.yaml"}})
	if err != nil {
		t.Fatal(err)
	}
	build := func(set impact.SetValuesEvaluator) *domain.ImpactReport {
		rep, err := impact.Build(impact.Input{Edge: edge, Env: envr, Now: fixedNow, Set: set})
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}
	find := func(rep *domain.ImpactReport, change string) domain.ImpactFinding {
		for _, f := range rep.Findings {
			if f.ChangeID == change {
				return f
			}
		}
		t.Fatalf("no finding for %s", change)
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

	// without --render: today's values-removed action-required
	base := build(nil)
	for _, id := range []string{"chg-feature", "chg-dead"} {
		if f := find(base, id); f.Classification != domain.ImpactActionRequired || f.Rule != impact.RuleValuesRemoved {
			t.Fatalf("baseline %s: %s %s", id, f.Classification, f.Rule)
		}
	}
	// undecided render: byte-identical report
	fake := stubSet{r: impact.SetValuesResult{Outcome: impact.SetValuesUndecided, Needed: []string{"no render decided"}}}
	und, err := json.Marshal(build(fake))
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

	e, p := rejPair(t, "1.1.0")
	if p.Status != PairOK {
		t.Fatalf("pair: %s %+v", p.Status, p.Failure)
	}
	rep := build(rejSet(e, "1.1.0", p))
	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, x := range rep.EnvironmentEvidence {
		pool[x.ID] = x
	}
	anyEnvScope := func(ids []domain.EvidenceID) bool {
		for _, id := range ids {
			if envScope([]domain.EvidenceID{id}, pool) {
				return true
			}
		}
		return false
	}
	// attributable: today's action-required, now with rendered matches
	f := find(rep, "chg-feature")
	if f.Classification != domain.ImpactActionRequired || f.Rule != impact.RuleValuesRemoved {
		t.Fatalf("attributable: %s %s", f.Classification, f.Rule)
	}
	if !anyEnvScope(f.EnvironmentEvidence) {
		t.Errorf("attributable needs environment-render evidence on chain 2: %d records", len(f.EnvironmentEvidence))
	}
	// no-effect: values-set-no-effect with a no-attributable-change render check
	d := find(rep, "chg-dead")
	c := renderCheck(d)
	if d.Classification != domain.ImpactNotAffected || d.Rule != impact.RuleValuesSetNoEffect || c == nil || c.Render == nil ||
		c.Render.Outcome != domain.RenderNoAttributableChange || c.Render.Key != "legacy.dead" || !envScope(c.Evidence, pool) {
		t.Errorf("no-effect: %s %s %+v", d.Classification, d.Rule, c)
	}

	// rejected: render-target-rejects action-required with both match kinds
	re, rp := rejPair(t, "2.0.0")
	if !rp.TargetRejects() {
		t.Fatalf("2.0.0 must refuse the customer's values: %s %+v", rp.Status, rp.Failure)
	}
	edge.To = domain.Version{Tag: "2.0.0", Semver: "2.0.0"}
	rej := build(rejSet(re, "2.0.0", rp))
	pool = map[domain.EvidenceID]domain.Evidence{}
	for _, x := range rej.EnvironmentEvidence {
		pool[x.ID] = x
	}
	f = find(rej, "chg-feature")
	if f.Classification != domain.ImpactActionRequired || f.Rule != impact.RuleRenderTargetRejects {
		t.Fatalf("rejected: %s %s", f.Classification, f.Rule)
	}
	keyed, rendered := false, false
	for _, m := range f.Matches {
		keyed = keyed || m.Kind == domain.MatchValuesKey
		rendered = rendered || (m.Kind == domain.MatchRenderedChange && envScope(m.Evidence, pool))
	}
	if !keyed || !rendered {
		t.Errorf("rejection needs a values-key match and a rendered match: %+v", f.Matches)
	}
	if !strings.Contains(f.Detail, "legacy") || !strings.Contains(f.Detail, "Additional property") {
		t.Errorf("rejection detail must quote the refusal: %s", f.Detail)
	}
}

// stubSet is a SetValuesEvaluator returning one canned result.
type stubSet struct{ r impact.SetValuesResult }

func (s stubSet) EvaluateSetValues(domain.Change) impact.SetValuesResult { return s.r }
