package render

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// envGoldenPair is the golden pair dressed as an environment render (the
// customer's values): rendered-change conditions are evaluated against these.
func envGoldenPair(t *testing.T) *Pair {
	t.Helper()
	p := goldenReleasePair(t)
	p.Scope = domain.RenderEnvironment
	p.Target = Target{ID: "values:customer", Kind: TargetValuesFiles, Origin: "customer.yaml",
		Why: "test values", ReleaseName: "demo", Namespace: "demo", ValuesComplete: true}
	for _, r := range []*Result{p.FromResult, p.ToResult} {
		r.Provenance.Scope = domain.RenderEnvironment
		r.Provenance.ValuesDigest = "sha256:customer-values"
		r.Provenance.ReleaseName, r.Provenance.Namespace = "demo", "demo"
	}
	return p
}

func incompletePair(t *testing.T) *Pair {
	t.Helper()
	p := envGoldenPair(t)
	p.Target.ID = "flux:demo"
	p.Target.ValuesComplete = false
	p.Target.IncompleteReason = "valuesFrom references a secret"
	return p
}

func assumedNamesPair(t *testing.T) *Pair {
	t.Helper()
	p := envGoldenPair(t)
	p.Target.NamesAssumed = true
	return p
}

// TestEvaluateRenderedChangeAbsenceIsFalse: in complete renders, an object or
// path absent from both renders is not in the asked state — a false with an
// evaluation record (check + examined state records), never a bare false.
func TestEvaluateRenderedChangeAbsenceIsFalse(t *testing.T) {
	p := envGoldenPair(t)
	for _, c := range []domain.Condition{
		rcond("Gateway", "spec.listeners", domain.StateChanged),
		rcond("Deployment", "spec.template.spec.containers[].env[name=NOPE]", domain.StateAdded),
		rcond("Deployment", "spec.template.spec.containers[].image", domain.StateUnchanged),
	} {
		res := EvaluateRenderedChange(c, []*Pair{p})
		if res.Value != RenderedFalse {
			t.Fatalf("%s %s: %s (%s), want false", c.Kind, c.Path, res.Value, res.Detail)
		}
		if len(res.Checks) != 1 || res.Checks[0].Dimension != domain.DimensionRender || len(res.Examined) == 0 || len(res.Evidence) != len(res.Examined) {
			t.Errorf("%s %s: a false must carry a render check and examined records: %+v", c.Kind, c.Path, res)
		}
		for _, e := range res.Evidence {
			if e.Render == nil || e.Render.Scope != domain.RenderEnvironment {
				t.Errorf("examined record %s is not environment-scope render evidence", e.ID)
			}
		}
	}
	// a true carries one rendered-change match citing environment evidence
	res := EvaluateRenderedChange(rcond("Deployment", "spec.template.spec.containers[].image", domain.StateChanged), []*Pair{p})
	if res.Value != RenderedTrue || len(res.Matches) != 1 || res.Matches[0].Kind != domain.MatchRenderedChange || len(res.Matches[0].Evidence) == 0 {
		t.Fatalf("true without a rendered-change match: %+v", res)
	}
	have := map[domain.EvidenceID]bool{}
	for _, e := range res.Evidence {
		have[e.ID] = true
	}
	for _, id := range res.Matches[0].Evidence {
		if !have[id] {
			t.Errorf("match cites %s, which the result's records do not hold", id)
		}
	}
}

func failedPair() *Pair {
	return &Pair{Product: "demo", From: "1.0.0", To: "1.1.0", Status: PairFailed,
		Target:  Target{ID: "helmfile:demo", ValuesComplete: true},
		Failure: &Failure{Reason: FailTemplateError, Detail: "values do not satisfy the schema"}}
}

func rcond(kind, path string, state domain.FieldState, values ...string) domain.Condition {
	c := domain.Condition{Op: domain.OpRenderedChange, Kind: kind, Path: path, State: state}
	if len(values) > 0 {
		c.Values = values
	}
	return c
}

func TestEvaluateRenderedChangeTrue(t *testing.T) {
	p := envGoldenPair(t)
	cases := []struct {
		name  string
		cond  domain.Condition
		want  string // fragment of the detail
		evObj string // evidence must cite an object containing this
	}{
		{
			name: "container image changed",
			cond: rcond("Deployment", "spec.template.spec.containers[].image", domain.StateChanged),
			want: "is changed", evObj: "Deployment/demo-controller",
		},
		{
			name: "image changed to a value in Values",
			cond: rcond("Deployment", "spec.template.spec.containers[name=controller].image", domain.StateChanged, "v1.1.0"),
			want: "is changed", evObj: "Deployment/demo-controller",
		},
		{
			name: "env var added",
			cond: rcond("Deployment", "spec.template.spec.containers[].env[name=GOMAXPROCS].value", domain.StateAdded),
			want: "is added",
		},
		{
			name: "env element unchanged",
			cond: rcond("Deployment", "spec.template.spec.containers[].env[name=POD_NAMESPACE]", domain.StateUnchanged),
			want: "is unchanged",
		},
		{
			name: "args changed (ordered list)",
			cond: rcond("Deployment", "spec.template.spec.containers[name=controller].args", domain.StateChanged),
			want: "is changed",
		},
		{
			name: "field of an added object is added",
			cond: rcond("ConfigMap", "data.enabled", domain.StateAdded, "true"),
			want: "is added", evObj: "ConfigMap/demo-feature-x",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := EvaluateRenderedChange(tc.cond, []*Pair{p})
			if res.Value != RenderedTrue {
				t.Fatalf("value %s (%s), want true", res.Value, res.Detail)
			}
			if !strings.Contains(res.Detail, tc.want) {
				t.Errorf("detail %q does not say %q", res.Detail, tc.want)
			}
			if len(res.Targets) != 1 || res.Targets[0] != p.Target.ID {
				t.Errorf("targets %v, want [%s]", res.Targets, p.Target.ID)
			}
			if len(res.Evidence) == 0 {
				t.Fatal("a true result must cite rendered evidence")
			}
			if tc.evObj != "" {
				kind, name, _ := strings.Cut(tc.evObj, "/")
				found := false
				for _, e := range res.Evidence {
					// ObjectID.String() leaves an empty namespace as "//" —
					// kind and name are checked separately
					if e.Render != nil && strings.Contains(e.Render.Object+"/", kind+"/") && strings.Contains(e.Render.Object, name) {
						found = true
					}
				}
				if !found {
					t.Errorf("evidence does not cite %s", tc.evObj)
				}
			}
		})
	}
}

func TestEvaluateRenderedChangeFalse(t *testing.T) {
	p := envGoldenPair(t)
	// POD_NAMESPACE is unchanged, not added — with complete values that is a
	// decisive false
	res := EvaluateRenderedChange(rcond("Deployment", "spec.template.spec.containers[].env[name=POD_NAMESPACE]", domain.StateAdded), []*Pair{p})
	if res.Value != RenderedFalse {
		t.Fatalf("value %s (%s), want false", res.Value, res.Detail)
	}
	if !strings.Contains(res.Detail, "unchanged") {
		t.Errorf("the false detail must name what the render shows instead: %q", res.Detail)
	}
	// a Values filter the target does not satisfy is false too, not unknown
	res = EvaluateRenderedChange(
		rcond("Deployment", "spec.template.spec.containers[].image", domain.StateChanged, "v9.9.9"), []*Pair{p})
	if res.Value != RenderedFalse {
		t.Fatalf("changed-to-v9.9.9 = %s (%s), want false: the render shows the actual value", res.Value, res.Detail)
	}
}

func TestEvaluateRenderedChangeUnknown(t *testing.T) {
	p := envGoldenPair(t)
	cases := []struct {
		name  string
		pairs []*Pair
		cond  domain.Condition
		want  string // fragment of detail or reason
	}{
		{
			name:  "no renders at all",
			pairs: nil,
			cond:  rcond("Deployment", "spec.template.spec.containers[].image", domain.StateChanged),
			want:  "render unavailable",
		},
		{
			name:  "failed render",
			pairs: []*Pair{failedPair()},
			cond:  rcond("Deployment", "spec.template.spec.containers[].image", domain.StateChanged),
			want:  "values do not satisfy",
		},
		{
			name:  "incomplete values keep an absence of change silent",
			pairs: []*Pair{incompletePair(t)},
			cond:  rcond("Deployment", "spec.template.spec.containers[].env[name=POD_NAMESPACE]", domain.StateAdded),
			want:  "values incomplete",
		},
		{
			name:  "named object absent under an assumed release name",
			pairs: []*Pair{assumedNamesPair(t)},
			cond:  domain.Condition{Op: domain.OpRenderedChange, Kind: "Deployment", Name: "cert-manager", Path: "spec.replicas", State: domain.StateChanged},
			want:  "release name was assumed",
		},
		{
			name:  "one undecidable pair keeps the leaf unknown",
			pairs: []*Pair{p, failedPair()},
			cond:  rcond("Deployment", "spec.template.spec.containers[].env[name=POD_NAMESPACE]", domain.StateAdded),
			want:  "helmfile:demo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := EvaluateRenderedChange(tc.cond, tc.pairs)
			if res.Value != RenderedUnknown {
				t.Fatalf("value %s (%s), want unknown", res.Value, res.Detail)
			}
			if res.Reason != domain.UnknownEnvironmentVisibilityGap {
				t.Errorf("reason %q, want environment-visibility-gap", res.Reason)
			}
			if !strings.Contains(res.Detail, tc.want) {
				t.Errorf("detail %q does not mention %q", res.Detail, tc.want)
			}
		})
	}
	// a true pair decides even when another pair failed: the customer IS affected
	res := EvaluateRenderedChange(rcond("Deployment", "spec.template.spec.containers[].image", domain.StateChanged),
		[]*Pair{failedPair(), p})
	if res.Value != RenderedTrue {
		t.Fatalf("one pair showing the change is true despite the other failing: %s (%s)", res.Value, res.Detail)
	}
	// a non-rendered-change op is not evaluated
	res = EvaluateRenderedChange(domain.Condition{Op: domain.OpGVKInUse, Kind: "Deployment"}, []*Pair{p})
	if res.Value != RenderedUnknown || res.Reason != domain.UnknownEnvironmentVisibilityGap {
		t.Fatalf("wrong op: %+v", res)
	}
}

func TestEvaluateRenderedChangeMultiplePairs(t *testing.T) {
	a := envGoldenPair(t)
	b := incompletePair(t)
	// both complete-and-unchanged would be false; one incomplete is unknown
	res := EvaluateRenderedChange(rcond("Deployment", "spec.template.spec.containers[].env[name=POD_NAMESPACE]", domain.StateAdded), []*Pair{a, b})
	if res.Value != RenderedUnknown {
		t.Fatalf("value %s, want unknown (the incomplete pair may still differ)", res.Value)
	}
	if len(res.Targets) != 2 {
		t.Errorf("targets %v, want both", res.Targets)
	}
}

// R5/R11 boundary: a customer render delta is environment evidence — it may
// back an impact finding, but its records are environment-scoped renders the
// contract rejects for knowledge (only release-level renders may back facts),
// and the leaf predicate itself concludes nothing about consequences or
// ACTION REQUIRED — that classification is the trust ladder's, above it.
func TestRenderedChangeEvidenceStaysEnvironmental(t *testing.T) {
	res := EvaluateRenderedChange(
		rcond("ClusterRole", "rules[]", domain.StateChanged), []*Pair{envGoldenPair(t)})
	if len(res.Evidence) == 0 {
		t.Fatal("expected evidence records")
	}
	for _, e := range res.Evidence {
		if e.Render == nil || e.Render.Scope != domain.RenderEnvironment {
			t.Errorf("evidence %s: rendered-change evidence must be environment scope", e.ID)
		}
	}
}

func TestPatternPathResolver(t *testing.T) {
	from, _ := goldenPair(t)
	var dep Object
	for _, o := range from {
		if o.ID.Kind == "Deployment" {
			dep = o
			continue
		}
	}
	if dep.ID.Kind == "" {
		t.Fatal("no Deployment in the golden stream")
	}
	cases := []struct {
		path string
		want []string // len + fragments
	}{
		{path: "spec.template.spec.containers[name=controller].image", want: []string{`example.org/demo/controller:v1.0.0`}},
		{path: "spec.template.spec.containers[].image", want: []string{`example.org/demo/controller:v1.0.0`}},
		{path: "spec.template.spec.containers[name=other].image", want: nil},
		{path: "spec.template.spec.containers[name=controller].args", want: []string{`["--v=2","--leader-election-namespace=kube-system","--enable-certificate-owner-ref=false"]`}},
		{path: "spec.template.spec.containers[].env[name=POD_NAMESPACE].valueFrom.fieldRef.fieldPath", want: []string{`"metadata.namespace"`}},
		{path: "metadata.labels[\"app.kubernetes.io/name\"]", want: []string{`"demo"`}},
		{path: "spec.template.spec.containers[].ports[].containerPort", want: []string{`9402`}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			segs, err := parsePatternPath(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			got := resolveSegs(dep.Body, segs)
			if tc.want == nil {
				if len(got) != 0 {
					t.Fatalf("got %v, want nothing", got)
				}
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], strings.Trim(w, `"`)) && got[i] != w {
					t.Errorf("value %q, want %q", got[i], w)
				}
			}
		})
	}
	for _, bad := range []string{"a..b", "a[b", "a.", `a["b`, ""} {
		if _, err := parsePatternPath(bad); err == nil {
			t.Errorf("path %q must not parse", bad)
		}
	}
	if got := patternOfPath("spec.template.spec.containers[name=controller].env[name=X].value"); got != "spec.template.spec.containers[].env[].value" {
		t.Errorf("patternOfPath = %q", got)
	}
}

// A decisive false must carry its evaluation record in the shape the report
// contract enforces (domain.ImpactReport.Validate: a render-dimension check
// carries exactly a render record, no-attributable-change citing
// environment-render evidence, naming what it evaluated). Regression
// (trust-audit, kyverno-1.12-1.13 in the proxy-incl-shadow view): a fact whose
// exposure held a rendered-change leaf that decided false produced a check
// without the record; the whole kyverno report failed validation and the
// evaluator silently dropped the case.
func TestEvaluateRenderedChangeFalseCarriesRenderRecord(t *testing.T) {
	p := envGoldenPair(t)
	res := EvaluateRenderedChange(rcond("Deployment", "spec.template.spec.containers[].env[name=POD_NAMESPACE]", domain.StateAdded), []*Pair{p})
	if res.Value != RenderedFalse {
		t.Fatalf("value %s (%s), want false", res.Value, res.Detail)
	}
	if len(res.Checks) == 0 {
		t.Fatal("a decisive false carries its evaluation record")
	}
	for _, c := range res.Checks {
		if c.Dimension != domain.DimensionRender || c.Render == nil {
			t.Fatalf("render check without a render record: %+v", c)
		}
		if c.Render.Outcome != domain.RenderNoAttributableChange || strings.TrimSpace(c.Render.Key) == "" || len(c.Evidence) == 0 {
			t.Errorf("render record shape: %+v (evidence %v)", c.Render, c.Evidence)
		}
	}
}
