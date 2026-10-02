package render

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
)

// permSetup renders the perm chart pair with the customer's values (live helm).
func permSetup(t *testing.T) (*Engine, *Pair) {
	t.Helper()
	needTool(t, "helm")
	e := testEngine(t)
	content, err := os.ReadFile("testdata/perm-customer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tgt := Target{ID: "values:perm-customer", Kind: TargetValuesFiles, Origin: "testdata/perm-customer.yaml", Why: "test",
		ReleaseName: "perm", Namespace: "perm", ValuesComplete: true,
		Values: []ValuesLayer{{Origin: "testdata/perm-customer.yaml", Content: content}}}
	p := e.RenderPair(context.Background(), PairRequest{Product: "perm", From: "1.0.0", To: "1.1.0", Target: tgt, Scope: domain.RenderEnvironment, KubeVersion: "1.31"})
	if p.Status != PairOK {
		t.Fatalf("pair: %s %+v", p.Status, p.Failure)
	}
	return e, p
}

func permUnset(e *Engine, pairs ...*Pair) *UnsetValues {
	return &UnsetValues{Engine: e, Product: "perm", From: "1.0.0", To: "1.1.0", Pairs: pairs, KubeVersion: "1.31", Ctx: context.Background()}
}

// PO-3 adversarial: per changed default / new key the customer leaves unset,
// the counterfactual decides — exposed only with an attributable rendered
// change in the customer's actual upgrade delta, clear only when the renders
// succeeded and nothing is attributable.
func TestUnsetValuesAttribution(t *testing.T) {
	e, p := permSetup(t)
	u := permUnset(e, p)
	cases := []struct {
		name, kind string
		keys       []string
		want       impact.Truth
		class      ChangeClass // an attributed change of this class must be cited
	}{
		// kyverno E1: the background controller silently loses its Secret grant
		{"permission loss by a shrunk default list", "default-changed", []string{"rbac.extraResources"}, impact.True, RBACPermissionRemoved},
		// kyverno E5: the cleanup hook pulls a new, unmirrored image tag
		{"image tag default bump", "default-changed", []string{"cleanup.image.tag"}, impact.True, ImageChanged},
		// kyverno E1 (replacement): a new key ships the view RoleBinding enabled
		{"new key that ships a resource", "added", []string{"viewRoleBinding.create"}, impact.True, ResourceAdded},
		// a default change no template reads renders nothing: clear, with a check
		{"default change that renders nothing", "default-changed", []string{"docs.url"}, impact.False, ""},
		// the counterfactual differs (no label without the key), but the source
		// already rendered the same label: no upgrade delta, so not attributable
		{"new key whose effect the source already rendered", "added", []string{"componentLabel"}, impact.False, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := u.EvaluateUnsetValues(domain.Change{ID: "chg-x", Subjects: tc.keys}, tc.kind)
			if r.Value != tc.want {
				t.Fatalf("value %s (needed %v, matches %v), want %s", r.Value, r.Needed, r.Matches, tc.want)
			}
			switch tc.want {
			case impact.True:
				if len(r.Matches) == 0 || len(r.Records) != len(r.Matches) {
					t.Fatalf("exposed needs rendered matches with records: %+v", r)
				}
				found := false
				for i, m := range r.Matches {
					if m.Kind != domain.MatchRenderedChange || len(m.Evidence) != 1 || m.Evidence[0] != r.Records[i].ID {
						t.Errorf("match %d: %+v", i, m)
					}
					rec := r.Records[i]
					if rec.Render == nil || rec.Render.Scope != domain.RenderEnvironment {
						t.Errorf("chain-2 evidence must be environment-scope render evidence: %+v", rec.Render)
					}
					found = found || rec.Render.Change == string(tc.class)
					if strings.Contains(m.Subject, "token") || rec.Render.Object == "core/v1/Secret/perm/perm-token" {
						t.Errorf("the random token was attributed: %s", m.Subject)
					}
				}
				if !found {
					t.Errorf("no attributed %s change among %v", tc.class, r.Matches)
				}
			case impact.False:
				if len(r.Checks) != 1 || r.Checks[0].Dimension != domain.DimensionRender || r.Checks[0].Facts == 0 {
					t.Errorf("clear needs a render check: %+v", r.Checks)
				}
			}
		})
	}
}

// Incomplete values, a failed render or no Helm deployment never clear.
func TestUnsetValuesUnavailable(t *testing.T) {
	e, p := permSetup(t)
	inc := *p
	inc.Target.ValuesComplete, inc.Target.IncompleteReason = false, "valuesFrom"
	failed := &Pair{Status: PairFailed, Target: Target{ID: "values:f", ValuesComplete: true}, Failure: &Failure{Reason: FailTemplateError, Detail: "boom"}}
	kz := &Pair{Status: PairOK, Target: Target{ID: "kustomize:x", Kind: TargetKustomize, ValuesComplete: true}}
	for name, pairs := range map[string][]*Pair{"incomplete values": {&inc}, "failed render": {failed}, "kustomize only": {kz}, "no pairs": nil} {
		r := permUnset(e, pairs...).EvaluateUnsetValues(domain.Change{Subjects: []string{"docs.url"}}, "default-changed")
		if r.Value != impact.Unknown || len(r.Needed) == 0 {
			t.Errorf("%s: %s %v, want unknown with a reason", name, r.Value, r.Needed)
		}
	}
	// but an attributable change in an incomplete render still exposes
	r := permUnset(e, &inc).EvaluateUnsetValues(domain.Change{Subjects: []string{"rbac.extraResources"}}, "default-changed")
	if r.Value != impact.True {
		t.Errorf("an evidence-backed exposure must stand under incomplete values: %s %v", r.Value, r.Needed)
	}
}

// The join (internal/impact, PO-3 hunk): exposed → review-required with
// rendered evidence on chain 2; clear → not-affected with a render check;
// unavailable → not-affected as before, with "render unavailable" visible.
// Without an evaluator the verdict is today's.
func TestUnsetValuesInTheJoin(t *testing.T) {
	e, p := permSetup(t)
	ev := domain.NewEvidence(domain.EvidenceStructured, "helm-values", "https://example.org/perm/values.yaml", "values.yaml", "values diff", "sha256:v", fixedNow)
	mk := func(id, rule string, keys ...string) domain.Change {
		return domain.Change{ID: id, Category: domain.CategoryHelmValues, Title: id, Subjects: keys,
			Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: rule, Confidence: domain.ConfidenceHigh},
			Evidence:   []domain.EvidenceID{ev.ID}}
	}
	edge := &domain.UpgradeEdge{SchemaVersion: domain.UpgradeEdgeSchemaVersion, Product: domain.ProductRef{ID: "perm"},
		From: domain.Version{Tag: "1.0.0", Semver: "1.0.0"}, To: domain.Version{Tag: "1.1.0", Semver: "1.1.0"},
		Evidence: []domain.Evidence{ev},
		Changes: []domain.Change{
			mk("chg-rbac", "values:default-changed", "rbac.extraResources"),
			mk("chg-docs", "values:default-changed", "docs.url"),
		}}
	envr, err := env.Load(env.Inputs{ValuesFiles: []string{"testdata/perm-customer.yaml"}})
	if err != nil {
		t.Fatal(err)
	}
	build := func(u impact.UnsetValuesEvaluator) *domain.ImpactReport {
		rep, err := impact.Build(impact.Input{Edge: edge, Env: envr, Now: fixedNow, Unset: u})
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
	// without the evaluator: today's not-affected for both
	base := build(nil)
	if f := find(base, "chg-rbac"); f.Classification != domain.ImpactNotAffected || f.Rule != impact.RuleValuesUnset {
		t.Fatalf("baseline: %s %s", f.Classification, f.Rule)
	}
	rep := build(permUnset(e, p))
	f := find(rep, "chg-rbac")
	if f.Classification != domain.ImpactReviewRequired || f.Rule != impact.RuleValuesDefaultRendered || len(f.EnvironmentEvidence) == 0 {
		t.Fatalf("exposed: %s %s env=%d", f.Classification, f.Rule, len(f.EnvironmentEvidence))
	}
	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, x := range rep.EnvironmentEvidence {
		pool[x.ID] = x
	}
	for _, id := range f.EnvironmentEvidence {
		if x, ok := pool[id]; !ok || x.Render == nil {
			t.Errorf("chain-2 record %s is not rendered evidence in the report pool", id)
		}
	}
	d := find(rep, "chg-docs")
	if d.Classification != domain.ImpactNotAffected || !hasDim(d.Checks, domain.DimensionRender) {
		t.Errorf("clear: %s checks %+v", d.Classification, d.Checks)
	}
	// render unavailable: today's verdict, the check says so
	failed := &Pair{Status: PairFailed, Target: Target{ID: "values:f", ValuesComplete: true}, Failure: &Failure{Reason: FailChartUnavailable, Detail: "offline"}}
	un := build(permUnset(e, failed))
	for _, id := range []string{"chg-rbac", "chg-docs"} {
		x := find(un, id)
		if x.Classification != domain.ImpactNotAffected || !hasDim(x.Checks, domain.DimensionRender) || !strings.Contains(strings.Join(x.Checks[len(x.Checks)-1].Subjects, " "), "render unavailable") {
			t.Errorf("%s unavailable: %s %+v", id, x.Classification, x.Checks)
		}
	}
}

func hasDim(cs []domain.ImpactCheck, d domain.EnvironmentDimension) bool {
	for _, c := range cs {
		if c.Dimension == d {
			return true
		}
	}
	return false
}

// For an UNSET key the chart-default attribution is the customer's effective
// change and carries no customer values: release-scope records, valid as
// knowledge evidence, offerable to proposal prompts (PO-3 note).
func TestUnsetKeyEvidenceIsReleaseLevel(t *testing.T) {
	e, _ := permSetup(t)
	evs, why := UnsetKeyEvidence(context.Background(), e, e.ReleasePairs("1.31"), "perm", "1.0.0", "1.1.0", []string{"rbac.extraResources"}, "default-changed")
	if why != "" || len(evs) == 0 {
		t.Fatalf("release attribution: %q %d", why, len(evs))
	}
	for _, x := range evs {
		if x.Render == nil || x.Render.Scope != domain.RenderRelease || x.Render.ValuesDigest != "" { // chart defaults: no customer values
			t.Errorf("record %s: %+v", x.ID, x.Render)
		}
	}
	cand := domain.SemanticCandidate{Product: "perm", Release: "1.1.0", Grouping: "single", Category: domain.CategoryHelmValues,
		Title: "rbac.extraResources default", Producer: "test@v1", CreatedAt: fixedNow,
		Members: []domain.CandidateMember{{ChangeID: "chg-rbac", Computed: true}}, Evidence: evs}
	cand.ID = domain.CandidateID(cand.Product, cand.Release, cand.Members)
	if err := cand.Validate(); err != nil {
		t.Fatalf("release-level attribution must be valid knowledge evidence: %v", err)
	}
	if none, why := UnsetKeyEvidence(context.Background(), e, e.ReleasePairs("1.31"), "perm", "1.0.0", "1.1.0", []string{"docs.url"}, "default-changed"); why != "" || len(none) != 0 {
		t.Errorf("a default that renders nothing has no release evidence: %q %d", why, len(none))
	}
}
