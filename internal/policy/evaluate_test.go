package policy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/render"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

func str(s string) *string {
	b, _ := json.Marshal(s)
	x := string(b)
	return &x
}

var deploy = render.ObjectID{Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "x", Name: "ctl"}

func imageChange(from, to string) render.Change {
	return render.Change{Class: render.ImageChanged, Object: deploy, Path: "spec.template.spec.containers[name=c].image",
		Name: imageRepositoryOf(to), Container: "c", Before: str(from), After: str(to)}
}

func imageRepositoryOf(ref string) string { return parseImage(ref).Repository }

func pair(changes ...render.Change) *render.Pair {
	return &render.Pair{Product: "p", Target: render.Target{ID: "t1", ValuesComplete: true}, Status: render.PairOK,
		Diff: &render.DiffResult{Changes: changes}}
}

func label() render.Change {
	return render.Change{Class: render.LabelChanged, Object: deploy, Path: `metadata.labels["a"]`, Name: "a", Before: str("1"), After: str("2")}
}

func rbacRemoved() render.Change {
	return render.Change{Class: render.RBACPermissionRemoved, Object: render.ObjectID{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole", Name: "r"},
		Permission: &render.Permission{Group: "", Resource: "secrets", Verb: "get"}}
}

func rbacAdded() render.Change {
	c := rbacRemoved()
	c.Class = render.RBACPermissionAdded
	return c
}

func TestDefaultPolicyLoads(t *testing.T) {
	p := Default()
	if p.Name != "default" || p.Digest == "" || len(p.Rules) == 0 {
		t.Fatalf("bad default policy: %+v", p)
	}
}

func TestEvaluateRendered(t *testing.T) {
	cases := []struct {
		name    string
		changes []render.Change
		want    Tier
	}{
		{"no change in a complete render", nil, AutoPass},
		{"image patch bump", []render.Change{imageChange("quay.io/x/ctl:v1.17.0", "quay.io/x/ctl:v1.17.2")}, AutoPass},
		{"image minor bump", []render.Change{imageChange("quay.io/x/ctl:v1.17.0", "quay.io/x/ctl:v1.18.0")}, AutoPass},
		{"image minor bump without v", []render.Change{imageChange("quay.io/x/ctl:1.17.0", "quay.io/x/ctl:1.18.0")}, AutoPass},
		{"image + label churn", []render.Change{imageChange("a/b:1.0.0", "a/b:1.0.1"), label()}, AutoPass},
		{"major image bump", []render.Change{imageChange("a/b:1.9.0", "a/b:2.0.0")}, Review},
		{"image downgrade", []render.Change{imageChange("a/b:1.9.0", "a/b:1.8.0")}, Review},
		{"image prerelease target", []render.Change{imageChange("a/b:1.9.0", "a/b:1.10.0-rc.1")}, Review},
		{"different repository", []render.Change{imageChange("a/b:1.0.0", "evil/b:1.0.1")}, Review},
		{"different registry same name", []render.Change{imageChange("quay.io/a/b:1.0.0", "docker.io/a/b:1.0.1")}, Review},
		{"non-semver tag", []render.Change{imageChange("a/b:latest", "a/b:stable")}, Review},
		{"loose two-part tag", []render.Change{imageChange("a/b:1.0", "a/b:1.1")}, Review},
		{"digest only", []render.Change{imageChange("a/b:1.0.0@sha256:aa", "a/b:1.0.0@sha256:bb")}, Review},
		{"image bump + hidden RBAC addition", []render.Change{imageChange("a/b:1.0.0", "a/b:1.0.1"), rbacAdded()}, Review},
		{"image bump + hidden RBAC removal", []render.Change{imageChange("a/b:1.0.0", "a/b:1.0.1"), rbacRemoved()}, Block},
		{"resource removed", []render.Change{{Class: render.ResourceRemoved, Object: deploy}}, Block},
		{"resource added", []render.Change{{Class: render.ResourceAdded, Object: deploy}}, Review},
		{"api version changed", []render.Change{{Class: render.APIVersionChanged, Object: deploy}}, Block},
		{"CRD storage flip", []render.Change{{Class: render.FieldChanged, Path: "spec.versions[name=v1].storage",
			Object: render.ObjectID{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition", Name: "c"}}}, Block},
		{"CRD description change", []render.Change{{Class: render.FieldChanged, Path: "spec.versions[name=v1].description",
			Object: render.ObjectID{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition", Name: "c"}}}, Review},
		{"env var change", []render.Change{{Class: render.EnvVarChanged, Object: deploy, Name: "X"}}, Review},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := Evaluate(Default(), Input{Pairs: []*render.Pair{pair(tc.changes...)}})
			if v.Tier != tc.want {
				t.Fatalf("tier = %s, want %s\n%s", v.Tier, tc.want, dump(v))
			}
			if v.AutoPass() != (tc.want == AutoPass) {
				t.Fatalf("AutoPass() = %v", v.AutoPass())
			}
		})
	}
}

func dump(v *Verdict) string {
	var sb strings.Builder
	WriteText(&sb, v, TextOptions{ShowAll: true})
	return sb.String()
}

func TestEvaluateRenderState(t *testing.T) {
	failed := &render.Pair{Target: render.Target{ID: "t1", ValuesComplete: true}, Status: render.PairFailed,
		Failure: &render.Failure{Reason: render.FailTemplateError, Detail: "boom"}}
	rejects := &render.Pair{Target: render.Target{ID: "t1", ValuesComplete: true}, Status: render.PairFailed,
		FromResult: &render.Result{Status: render.StatusSucceeded},
		ToResult:   &render.Result{Status: render.StatusFailed, Failure: &render.Failure{Reason: render.FailInvalidValues}}}
	incomplete := pair(imageChange("a/b:1.0.0", "a/b:1.0.1"))
	incomplete.Target.ValuesComplete = false
	incomplete.Target.IncompleteReason = "values from a secret"
	na := &render.Pair{Target: render.Target{ID: "t1"}, Status: render.PairNotApplicable}
	cases := []struct {
		name  string
		pairs []*render.Pair
		want  Tier
	}{
		{"render not run", nil, Review},
		{"render failed", []*render.Pair{failed}, Review},
		{"chart rejects config", []*render.Pair{rejects}, Block},
		{"incomplete values even with an image-only diff", []*render.Pair{incomplete}, Review},
		{"not applicable", []*render.Pair{na}, Review},
		{"one good pair, one failed pair", []*render.Pair{pair(), failed}, Review},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := Evaluate(Default(), Input{Pairs: tc.pairs})
			if v.Tier != tc.want {
				t.Fatalf("tier = %s, want %s\n%s", v.Tier, tc.want, dump(v))
			}
		})
	}
}

// --- upstream changes and findings ------------------------------------------------

func chg(id string, cat domain.Category) domain.Change {
	return domain.Change{ID: id, Category: cat, Title: id + " title", Evidence: []domain.EvidenceID{"ev-" + domain.EvidenceID(id)}}
}

func finding(id, changeID string, class domain.ImpactClass, rule string) domain.ImpactFinding {
	return domain.ImpactFinding{ID: id, ChangeID: changeID, Classification: class, Rule: rule, Title: id + " title"}
}

func edge(cs ...domain.Change) *domain.UpgradeEdge { return &domain.UpgradeEdge{Changes: cs} }
func report(fs ...domain.ImpactFinding) *domain.ImpactReport {
	return &domain.ImpactReport{Findings: fs}
}

func TestEvaluateUpstreamAndFindings(t *testing.T) {
	routine := chg("r", domain.CategoryDependency)
	routine.Routine, routine.RoutineKind = true, upgrade.RoutineDependency
	bugfix := chg("b", domain.CategoryBugfix)
	breakingBugfix := chg("bb", domain.CategoryBugfix)
	breakingBugfix.Breaking = true
	directive := chg("d", domain.CategoryBugfix)
	directive.ActionRequired = true
	valuesChange := chg("v", domain.CategoryHelmValues)
	security := chg("s", domain.CategorySecurity)
	advisory := chg("a", domain.CategorySecurity)
	advisory.Provenance.Rule = upgrade.RuleAdvisoryAffected
	good := pair(imageChange("a/b:1.0.0", "a/b:1.0.1"))

	cases := []struct {
		name string
		edge *domain.UpgradeEdge
		rep  *domain.ImpactReport
		want Tier
		inv  string
	}{
		{"routine change only", edge(routine), nil, AutoPass, ""},
		{"non-breaking bugfix", edge(bugfix), nil, AutoPass, ""},
		{"breaking bugfix with no findings", edge(breakingBugfix), nil, Review, InvBreaking},
		{"directive bugfix", edge(directive), nil, Review, InvBreaking},
		{"helm-values change with no finding", edge(valuesChange), nil, Review, ""},
		{"helm-values change cleared as not-affected", edge(valuesChange),
			report(finding("f1", "v", domain.ImpactNotAffected, "impact:values-unset")), AutoPass, ""},
		{"helm-values change with review finding", edge(valuesChange),
			report(finding("f1", "v", domain.ImpactReviewRequired, "impact:values-removed")), Review, ""},
		{"security fix shipped with target, informational", edge(security),
			report(finding("f1", "s", domain.ImpactInformational, "impact:security-fix")), AutoPass, ""},
		{"action-required finding", edge(valuesChange),
			report(finding("f1", "v", domain.ImpactActionRequired, "impact:values-removed")), Review, InvActionRequired},
		{"unknown on non-routine change", edge(bugfix),
			report(finding("f1", "b", domain.ImpactUnknown, "impact:not-joined")), Review, InvUnknown},
		{"unknown with no change at all", nil,
			report(finding("f1", "", domain.ImpactUnknown, "impact:x")), Review, InvUnknown},
		{"unknown on a routine change", edge(routine),
			report(finding("f1", "r", domain.ImpactUnknown, "impact:not-joined")), Review, ""}, // default policy: no auto-pass rule for unknown
		{"advisory affecting the target", edge(advisory), nil, Review, InvAdvisory},
		{"advisory affecting the target, even when its finding is informational", edge(advisory),
			report(finding("f1", "a", domain.ImpactInformational, "impact:x")), Review, InvAdvisory},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := Evaluate(Default(), Input{Pairs: []*render.Pair{good}, Edge: tc.edge, Report: tc.rep})
			if v.Tier != tc.want {
				t.Fatalf("tier = %s, want %s\n%s", v.Tier, tc.want, dump(v))
			}
			if tc.inv != "" {
				// the default policy may already say review; under a policy that
				// waves everything through the invariant itself must raise it
				yolo, err := Parse([]byte(autoPassEverything))
				if err != nil {
					t.Fatal(err)
				}
				y := Evaluate(yolo, Input{Pairs: []*render.Pair{good}, Edge: tc.edge, Report: tc.rep})
				found := false
				for _, it := range y.Items {
					found = found || it.Invariant == tc.inv
				}
				if !found || y.Tier == AutoPass {
					t.Fatalf("invariant %s did not hold under an auto-pass-everything policy\n%s", tc.inv, dump(y))
				}
			}
		})
	}
}

// --- hostile policies: the invariants hold whatever the rules say ------------------------

const autoPassEverything = `
apiVersion: ri.dev/upgrade-policy/v1alpha1
name: yolo
default: {tier: auto-pass, reason: ship it}
rules:
  - {id: all-rendered, tier: auto-pass, reason: x, match: {subject: rendered}}
  - {id: all-upstream, tier: auto-pass, reason: x, match: {subject: upstream}}
  - {id: all-findings, tier: auto-pass, reason: x, match: {subject: finding}}
  - {id: routine, tier: auto-pass, reason: x, match: {subject: upstream, routine: true}}
`

func TestInvariantsHoldUnderAutoPassEverything(t *testing.T) {
	p, err := Parse([]byte(autoPassEverything))
	if err != nil {
		t.Fatal(err)
	}
	advisory := chg("a", domain.CategorySecurity)
	advisory.Provenance.Rule = upgrade.RuleAdvisoryAffected
	bugfix := chg("b", domain.CategoryBugfix)
	good := pair(imageChange("a/b:1.0.0", "a/b:1.0.1"))
	failed := &render.Pair{Target: render.Target{ID: "t1"}, Status: render.PairFailed, Failure: &render.Failure{Reason: render.FailTemplateError}}
	cases := []struct {
		name string
		in   Input
		want Tier
	}{
		{"baseline: everything covered", Input{Pairs: []*render.Pair{good}, Edge: edge(bugfix), Report: report(finding("f", "b", domain.ImpactInformational, "r"))}, AutoPass},
		{"no render", Input{Edge: edge(bugfix)}, Review},
		{"render failure", Input{Pairs: []*render.Pair{failed}}, Review},
		{"incomplete values", Input{Pairs: []*render.Pair{func() *render.Pair { x := pair(); x.Target.ValuesComplete = false; return x }()}}, Review},
		{"action-required finding", Input{Pairs: []*render.Pair{good}, Report: report(finding("f", "", domain.ImpactActionRequired, "r"))}, Review},
		{"unknown finding", Input{Pairs: []*render.Pair{good}, Report: report(finding("f", "b", domain.ImpactUnknown, "r")), Edge: edge(bugfix)}, Review},
		{"advisory", Input{Pairs: []*render.Pair{good}, Edge: edge(advisory)}, Review},
		{"breaking upstream", Input{Pairs: []*render.Pair{good}, Edge: edge(func() domain.Change { c := bugfix; c.Breaking = true; return c }())}, Review},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := Evaluate(p, tc.in)
			if v.Tier != tc.want {
				t.Fatalf("tier = %s, want %s\n%s", v.Tier, tc.want, dump(v))
			}
		})
	}
}

func TestBlockNeverLowered(t *testing.T) {
	src := `
apiVersion: ri.dev/upgrade-policy/v1alpha1
name: strict
default: {tier: review, reason: d}
rules:
  - {id: block-actions, tier: block, reason: x, match: {subject: finding, findingClasses: [action-required]}}
  - {id: block-failed, tier: block, reason: x, match: {subject: render-state, states: [failed]}}
`
	p, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	v := Evaluate(p, Input{Pairs: []*render.Pair{pair()}, Report: report(finding("f", "", domain.ImpactActionRequired, "r"))})
	if v.Tier != Block {
		t.Fatalf("tier = %s, want block", v.Tier)
	}
	failed := &render.Pair{Target: render.Target{ID: "t1"}, Status: render.PairFailed, Failure: &render.Failure{Reason: render.FailTemplateError}}
	if v := Evaluate(p, Input{Pairs: []*render.Pair{failed}}); v.Tier != Block {
		t.Fatalf("tier = %s, want block", v.Tier)
	}
}

func TestFirstMatchWins(t *testing.T) {
	src := `
apiVersion: ri.dev/upgrade-policy/v1alpha1
name: ordered
default: {tier: review, reason: d}
rules:
  - {id: first, tier: review, reason: x, match: {subject: rendered, classes: [image-changed]}}
  - {id: second, tier: auto-pass, reason: x, match: {subject: rendered, classes: [image-changed], image: {bump: [patch]}}}
`
	p, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	v := Evaluate(p, Input{Pairs: []*render.Pair{pair(imageChange("a/b:1.0.0", "a/b:1.0.1"))}})
	if v.Tier != Review || v.Items[0].Rule != "first" {
		t.Fatalf("got %s via %s", v.Tier, v.Items[0].Rule)
	}
}

func TestPolicyRoutineRuleCanRaise(t *testing.T) {
	src := `
apiVersion: ri.dev/upgrade-policy/v1alpha1
name: careful
default: {tier: review, reason: d}
rules:
  - {id: deps-need-eyes, tier: review, reason: x, match: {subject: upstream, routine: true, routineKinds: [dependency]}}
`
	p, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	r := chg("r", domain.CategoryDependency)
	r.Routine, r.RoutineKind = true, upgrade.RoutineDependency
	v := Evaluate(p, Input{Pairs: []*render.Pair{pair()}, Edge: edge(r)})
	if v.Tier != Review || v.Covered.RoutineChanges != 0 {
		t.Fatalf("routine rule did not fire: %s", dump(v))
	}
}

// --- policy file validation ---------------------------------------------------------------

func TestParseRejects(t *testing.T) {
	hdr := "apiVersion: ri.dev/upgrade-policy/v1alpha1\nname: x\ndefault: {tier: review, reason: d}\n"
	cases := []struct{ name, src, want string }{
		{"bad tier", hdr + "rules:\n  - {id: a, tier: yes, reason: r, match: {subject: rendered}}\n", "schema"},
		{"missing reason", hdr + "rules:\n  - {id: a, tier: review, match: {subject: rendered}}\n", "schema"},
		{"unknown field", hdr + "rules:\n  - {id: a, tier: review, reason: r, match: {subject: rendered, bogus: 1}}\n", "schema"},
		{"unknown class", hdr + "rules:\n  - {id: a, tier: review, reason: r, match: {subject: rendered, classes: [nope]}}\n", "unknown rendered change class"},
		{"wrong-subject field", hdr + "rules:\n  - {id: a, tier: review, reason: r, match: {subject: rendered, categories: [bugfix]}}\n", "does not apply"},
		{"duplicate id", hdr + "rules:\n  - {id: a, tier: review, reason: r, match: {subject: rendered}}\n  - {id: a, tier: review, reason: r, match: {subject: upstream}}\n", "duplicate"},
		{"auto-pass render failure", hdr + "rules:\n  - {id: a, tier: auto-pass, reason: r, match: {subject: render-state, states: [failed]}}\n", "safety invariant"},
		{"auto-pass action-required", hdr + "rules:\n  - {id: a, tier: auto-pass, reason: r, match: {subject: finding, findingClasses: [action-required]}}\n", "safety invariant"},
		{"auto-pass unknown", hdr + "rules:\n  - {id: a, tier: auto-pass, reason: r, match: {subject: finding, findingClasses: [unknown]}}\n", "safety invariant"},
		{"image predicate on other class", hdr + "rules:\n  - {id: a, tier: review, reason: r, match: {subject: rendered, classes: [label-added], image: {bump: [patch]}}}\n", "image predicate"},
		{"wrong apiVersion", strings.Replace(hdr, "v1alpha1", "v9", 1) + "rules: []\n", "schema"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.src))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestImageBump(t *testing.T) {
	cases := []struct{ from, to, want string }{
		{"a/b:v1.2.3", "a/b:v1.2.4", BumpPatch},
		{"a/b:1.2.3", "a/b:1.3.0", BumpMinor},
		{"a/b:1.2.3", "a/b:2.0.0", BumpMajor},
		{"a/b:1.2.3", "a/b:1.2.2", BumpDowngrade},
		{"a/b:1.2.3", "a/b:1.3.0-rc.1", BumpPrerelease},
		{"a/b:1.2.3", "a/b:1.2.3+b2", BumpUnknown},
		{"a/b:1.2.3", "c/b:1.2.4", BumpUnknown},
		{"reg:5000/a/b:1.2.3", "reg:5000/a/b:1.2.4", BumpPatch},
		{"a/b", "a/b:1.2.4", BumpUnknown},
		{"a/b:1.2.3@sha256:1", "a/b:1.2.3@sha256:2", BumpDigest},
	}
	for _, tc := range cases {
		if got := imageBump(parseImage(tc.from), parseImage(tc.to)); got != tc.want {
			t.Errorf("%s -> %s: %s, want %s", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestVerdictJSONRoundTrip(t *testing.T) {
	v := Evaluate(Default(), Input{Pairs: []*render.Pair{pair(imageChange("a/b:1.0.0", "a/b:1.0.1"), rbacRemoved())}})
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var back Verdict
	if err := json.Unmarshal(b, &back); err != nil || back.Tier != Block {
		t.Fatalf("round trip: %v %+v", err, back.Tier)
	}
	// values never leak into the verdict
	if strings.Contains(string(b), "1.0.1") {
		t.Fatalf("verdict leaks rendered values: %s", b)
	}
}
