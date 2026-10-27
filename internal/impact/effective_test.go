package impact

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// Lane join5 (D1): object-level evaluations judge the objects as deployed —
// the rendered objects of the environment's entrypoints — never a base file
// the render consumed. Synthetic product (gizmo.example.io Widget): a base
// declares the object, an overlay patches it.

const widgetBase = `apiVersion: gizmo.example.io/v1
kind: Widget
metadata:
  name: w
  namespace: apps
spec:
  size: 2
`

const widgetBasePinned = `apiVersion: gizmo.example.io/v1
kind: Widget
metadata:
  name: w
  namespace: apps
spec:
  size: 2
  rotation: Never
`

// an overlay strategic-merge patch that sets the field the base leaves unset
const widgetPatchSets = `apiVersion: gizmo.example.io/v1
kind: Widget
metadata:
  name: w
spec:
  rotation: Never
`

// an overlay JSON patch: not an object, so no file states the result
const widgetJSONPatch = `- op: remove
  path: /spec/rotation
`

// overlay is a base, an overlay patch and their kustomizations on disk; the
// rendered output is given (what kustomize build produced).
type overlay struct {
	dir                  string
	base, patch, overlay string
}

func newOverlay(t *testing.T, base, patch string) overlay {
	t.Helper()
	dir := t.TempDir()
	o := overlay{dir: dir}
	o.base = writeFile(t, dir, "base/widget.yaml", base)
	writeFile(t, dir, "base/kustomization.yaml", "resources:\n- widget.yaml\n")
	o.patch = writeFile(t, dir, "overlays/prod/patch.yaml", patch)
	o.overlay = writeFile(t, dir, "overlays/prod/kustomization.yaml", "resources:\n- ../../base\npatches:\n- path: patch.yaml\n")
	return o
}

func (o overlay) env(t *testing.T) *env.Environment {
	return loadEnv(t, env.Inputs{Manifests: []string{o.dir}})
}

// rendered is the view with the overlay's render (rendered nil: unavailable).
func (o overlay) rendered(t *testing.T, rendered []byte, why string) (*env.Environment, *env.Environment) {
	t.Helper()
	files := o.env(t)
	src := env.RenderedObjects{Target: "kustomize:overlays/prod", Docs: rendered, Digest: domain.Digest(rendered), Unavailable: why,
		Consumed: map[string]int{o.overlay: 0, o.patch: 0, filepath.Join(o.dir, "base/kustomization.yaml"): 3, o.base: 3}}
	return files, env.Effective(files, []env.RenderedObjects{src})
}

func widgetScope(of ...domain.Condition) domain.Condition {
	return domain.Condition{Op: domain.OpResource, Group: "gizmo.example.io", Kind: "Widget", Of: of}
}

func evidenceOf(e *env.Environment, id domain.EvidenceID) domain.Evidence {
	for _, x := range e.Evidence {
		if x.ID == id {
			return x
		}
	}
	return domain.Evidence{}
}

func TestEffectiveViewOverlaySetsAField(t *testing.T) {
	o := newOverlay(t, widgetBase, widgetPatchSets)
	files, eff := o.rendered(t, []byte(widgetBasePinned), "")
	unset := widgetScope(field("spec.rotation", domain.StateUnset))

	// the defect: the base file leaves the field unset, so the files say exposed
	if r := EvaluateCondition(unset, files, nil); r.Value != True {
		t.Fatalf("files view: %s, want the base file's (wrong) true", r.Value)
	}
	// the deployed object sets it
	if r := EvaluateCondition(unset, eff, nil); r.Value != False {
		t.Fatalf("effective view: %s %v, want false", r.Value, r.Needed)
	}
	// and the set value cites the overlay patch's line, not the base
	r := EvaluateCondition(widgetScope(field("spec.rotation", domain.StateEquals, `"Never"`)), eff, nil)
	if r.Value != True {
		t.Fatalf("equals Never: %s", r.Value)
	}
	cited := false
	for _, m := range r.Matches {
		for _, id := range m.Evidence {
			x := evidenceOf(eff, id)
			if x.URI == o.patch && x.Kind == domain.EvidenceLocalFile && strings.Contains(x.Locator, "(L6)") {
				cited = true
			}
			if x.URI == o.base {
				t.Errorf("the base file is cited for the patched field: %+v", x)
			}
		}
	}
	if !cited {
		t.Errorf("the overlay patch (line 6) is not cited: %+v", r.Matches)
	}
	// size comes from the base: its line is cited
	r = EvaluateCondition(widgetScope(field("spec.size", domain.StateEquals, "2")), eff, nil)
	if r.Value != True || evidenceOf(eff, r.Matches[len(r.Matches)-1].Evidence[0]).URI != o.base {
		t.Errorf("spec.size: %s %+v, want the base file cited", r.Value, r.Matches)
	}
	// neither the base nor the patch is an object of the view on its own
	for _, x := range eff.Resources {
		if x.Kind == "Widget" && x.Render == "" {
			t.Errorf("a consumed file is read as declared: %s:%d", x.Doc.File, x.Doc.StartLine)
		}
	}
}

func TestEffectiveViewOverlayRemovesOrOverridesAField(t *testing.T) {
	// the fail-open direction: the base pins the value, the overlay removes it
	o := newOverlay(t, widgetBasePinned, widgetJSONPatch)
	files, eff := o.rendered(t, []byte(widgetBase), "")
	unset := widgetScope(field("spec.rotation", domain.StateUnset))
	if r := EvaluateCondition(unset, files, nil); r.Value != False {
		t.Fatalf("files view: %s, want the base file's (wrong) false", r.Value)
	}
	r := EvaluateCondition(unset, eff, nil)
	if r.Value != True {
		t.Fatalf("effective view: %s %v, want true: the deployed object leaves it unset", r.Value, r.Needed)
	}
	// the absence is read from the rendered object, which names its files
	var x domain.Evidence
	for _, m := range r.Matches {
		if m.Kind == domain.MatchAbsence {
			x = evidenceOf(eff, m.Evidence[0])
		}
	}
	if x.Kind != domain.EvidenceRendered || x.URI != "render:kustomize:overlays/prod" || !strings.Contains(x.Excerpt, o.base) {
		t.Errorf("absence evidence %+v, want the rendered object naming its base", x)
	}

	// an override: the base pins Never, the overlay replaces it with Always
	o = newOverlay(t, widgetBasePinned, "- op: replace\n  path: /spec/rotation\n  value: Always\n")
	files, eff = o.rendered(t, []byte(strings.Replace(widgetBasePinned, "Never", "Always", 1)), "")
	notPinned := widgetScope(field("spec.rotation", domain.StateNotEquals, `"Never"`))
	if r := EvaluateCondition(notPinned, files, nil); r.Value != False {
		t.Fatalf("files view: %s, want the base's (wrong) false", r.Value)
	}
	r = EvaluateCondition(notPinned, eff, nil)
	if r.Value != True {
		t.Fatalf("override: %s, want true", r.Value)
	}
	// no file states Always: the rendered output itself is cited
	for _, m := range r.Matches {
		for _, id := range m.Evidence {
			if e := evidenceOf(eff, id); e.URI == o.base {
				t.Errorf("the base's overridden value is cited: %+v", e)
			}
		}
	}
}

func TestEffectiveViewWithoutRenderIsUnknown(t *testing.T) {
	o := newOverlay(t, widgetBase, widgetPatchSets)
	_, eff := o.rendered(t, nil, "render failed: kustomize-dependency")
	for _, c := range []domain.Condition{
		widgetScope(field("spec.rotation", domain.StateUnset)),
		widgetScope(field("spec.rotation", domain.StateSet)),
		widgetScope(field("spec.size", domain.StateEquals, "2")),
	} {
		r := EvaluateCondition(c, eff, nil)
		if r.Value != Unknown || r.Reason != domain.UnknownEnvironmentVisibilityGap || !strings.Contains(strings.Join(r.Needed, " "), "deployed form") {
			t.Errorf("%s: %s %v, want unknown: the base is never read in place of the render", describe(c), r.Value, r.Needed)
		}
	}
	// a file no render consumed is still read as declared
	writeFile(t, o.dir, "other/gadget.yaml", "apiVersion: gizmo.example.io/v1\nkind: Gadget\nmetadata:\n  name: g\nspec:\n  rotation: Never\n")
	_, eff = o.rendered(t, nil, "render failed")
	g := domain.Condition{Op: domain.OpResource, Group: "gizmo.example.io", Kind: "Gadget", Of: []domain.Condition{field("spec.rotation", domain.StateSet)}}
	if r := EvaluateCondition(g, eff, nil); r.Value != True {
		t.Errorf("an unconsumed file: %s", r.Value)
	}
}

func TestEffectiveViewRenderGap(t *testing.T) {
	o := newOverlay(t, widgetBase, widgetPatchSets)
	files := o.env(t)
	eff := env.Effective(files, []env.RenderedObjects{
		{Target: "kustomize:overlays/prod", Docs: []byte(widgetBasePinned), Consumed: map[string]int{o.overlay: 0, o.patch: 0, o.base: 3}},
		{Target: "helm:gizmo", Unavailable: "render failed: chart-unavailable"},
	})
	// a visible object that decides true stands; a false needs every object
	if r := EvaluateCondition(widgetScope(field("spec.rotation", domain.StateSet)), eff, nil); r.Value != True {
		t.Errorf("set: %s", r.Value)
	}
	r := EvaluateCondition(widgetScope(field("spec.rotation", domain.StateUnset)), eff, nil)
	if r.Value != Unknown || !strings.Contains(strings.Join(r.Needed, " "), "helm:gizmo") {
		t.Errorf("unset under a render gap: %s %v, want unknown", r.Value, r.Needed)
	}
	none := domain.Condition{Op: domain.OpResource, Group: "gizmo.example.io", Kind: "Gadget", Of: []domain.Condition{field("spec.rotation", domain.StateSet)}}
	if r := EvaluateCondition(none, eff, nil); r.Value != Unknown {
		t.Errorf("no object of the kind under a render gap: %s, want unknown", r.Value)
	}
}

// rotationWidgetAssertion: a default change of Widget.spec.rotation; a
// Widget that leaves it unset is exposed.
func rotationWidgetAssertion() domain.SemanticAssertion {
	return domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectCRDField, Product: "example", Group: "gizmo.example.io", Kind: "Widget", Path: "spec.rotation"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: ptr(`"Never"`), After: ptr(`"Always"`)},
		Applicability: &domain.Applicability{
			Exposure: widgetScope(field("spec.rotation", domain.StateUnset)),
		},
		Consequence: &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired},
		Statement:   "The default Widget.spec.rotation changed from Never to Always.",
	}
}

func TestKnowledgeJoinReadsRenderedObjects(t *testing.T) {
	eb := newEdge()
	note := eb.note("https://example/upgrade-guide.md", "Widget rotation now defaults to Always", "The default of spec.rotation is now Always.")
	f := fact(t, rotationWidgetAssertion(), domain.VerifiedHuman, domain.NewChangeAnchor(note, lookupOf(eb.edge)))
	knowledgeOf := func(objs *env.Environment, files *env.Environment) domain.ImpactFinding {
		t.Helper()
		r, err := Build(Input{Edge: eb.edge, Env: files, Objects: objs, Now: testNow, Facts: []domain.VerifiedFact{f}})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Validate(); err != nil {
			t.Fatalf("report does not validate: %v", err)
		}
		for _, x := range findingsOf(r, note.ID) {
			if x.Knowledge != nil {
				return x
			}
		}
		t.Fatalf("no knowledge finding: %+v", r.Findings)
		return domain.ImpactFinding{}
	}

	// overlay sets the field: the files say exposed (D1), the render clears
	o := newOverlay(t, widgetBase, widgetPatchSets)
	files, eff := o.rendered(t, []byte(widgetBasePinned), "")
	if k := knowledgeOf(nil, files); k.Rule != RuleKnowledgeExposed {
		t.Fatalf("files view: %s, want the defect's knowledge-exposed", k.Rule)
	}
	if k := knowledgeOf(eff, files); k.Rule != RuleKnowledgeClear || k.Classification != domain.ImpactNotAffected {
		t.Errorf("rendered: %s/%s, want knowledge-clear", k.Rule, k.Classification)
	}
	// overlay removes it: the files clear (fail-open), the render exposes,
	// citing the rendered object, which the report's pool holds
	o = newOverlay(t, widgetBasePinned, widgetJSONPatch)
	files, eff = o.rendered(t, []byte(widgetBase), "")
	if k := knowledgeOf(nil, files); k.Rule != RuleKnowledgeClear {
		t.Fatalf("files view: %s, want the fail-open knowledge-clear", k.Rule)
	}
	k := knowledgeOf(eff, files)
	if k.Rule != RuleKnowledgeExposed || len(k.EnvironmentEvidence) == 0 {
		t.Fatalf("rendered: %s %+v, want knowledge-exposed with the rendered object", k.Rule, k.EnvironmentEvidence)
	}
	// render unavailable: undecided, never the base file's view
	o = newOverlay(t, widgetBasePinned, widgetJSONPatch)
	files, eff = o.rendered(t, nil, "render failed")
	if k := knowledgeOf(eff, files); k.Rule != RuleKnowledgeUndecided || k.UnknownReason != domain.UnknownEnvironmentVisibilityGap {
		t.Errorf("unrendered: %s/%s, want knowledge-undecided", k.Rule, k.UnknownReason)
	}
}

// The schema-attribute join (crd:default-changed) reads the same view.
func TestDefaultChangedJoinReadsRenderedObjects(t *testing.T) {
	eb := newEdge()
	c := domain.Change{
		ID: "chg-widget-default", Category: domain.CategoryCRDSchema,
		Title:      "Widget v1 schema: default changed: `spec.rotation`",
		Detail:     "Schema defaults changed in the gizmo.example.io/v1 schema of widgets.gizmo.example.io (old → new; defaults apply to objects that leave the field unset):\nspec.rotation: \"Never\" → \"Always\"",
		Subjects:   []string{"spec.rotation"},
		Provenance: computed(upgrade.RuleCRDDefaultChanged),
		Evidence:   []domain.EvidenceID{eb.ev("https://example/crds.yaml", "crd schema")},
	}
	eb.edge.Changes = append(eb.edge.Changes, c)
	rules := func(objs, files *env.Environment) map[string]bool {
		t.Helper()
		r, err := Build(Input{Edge: eb.edge, Env: files, Objects: objs, Now: testNow})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Validate(); err != nil {
			t.Fatalf("report does not validate: %v", err)
		}
		out := map[string]bool{}
		for _, f := range findingsOf(r, c.ID) {
			out[f.Rule] = true
		}
		return out
	}
	o := newOverlay(t, widgetBase, widgetPatchSets)
	files, eff := o.rendered(t, []byte(widgetBasePinned), "")
	if got := rules(nil, files); !got[RuleCRDDefaultApplies] {
		t.Fatalf("files view: %v, want the base's default-applies", got)
	}
	if got := rules(eff, files); got[RuleCRDDefaultApplies] || !got[RuleCRDDefaultPinned] {
		t.Errorf("rendered: %v, want pinned only", got)
	}
	o = newOverlay(t, widgetBasePinned, widgetJSONPatch)
	files, eff = o.rendered(t, []byte(widgetBase), "")
	if got := rules(eff, files); !got[RuleCRDDefaultApplies] || got[RuleCRDDefaultPinned] {
		t.Errorf("overlay removes the pin: %v, want default-applies only", got)
	}
	_, eff = o.rendered(t, nil, "render failed")
	if got := rules(eff, files); !got[RuleInsufficientVisibility] || len(got) != 1 {
		t.Errorf("unrendered: %v, want unknown only", got)
	}
}

// Workload-wide leaves under a render gap: the exposed direction keeps its
// finding (a true stands, as under partial health), a false is unknown, and
// a true resting on absence is never negated into a false.
func TestEffectiveViewRenderGapOnFlags(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  template:
    spec:
      containers:
        - name: app
          args: ["--foo=1"]
`)
	files := loadEnv(t, env.Inputs{Manifests: []string{dir}})
	eff := env.Effective(files, []env.RenderedObjects{{Target: "helm:gizmo", Unavailable: "rendered with incomplete values"}})
	flag := func(name string, st domain.FieldState) domain.Condition {
		return domain.Condition{Op: domain.OpCLIFlag, Name: name, State: st}
	}
	if r := EvaluateCondition(flag("--bar", domain.StateUnset), eff, nil); r.Value != True {
		t.Errorf("absence true under a gap: %s, want it to stand", r.Value)
	}
	if r := EvaluateCondition(flag("--foo", domain.StateUnset), eff, nil); r.Value != Unknown {
		t.Errorf("false under a gap: %s, want unknown", r.Value)
	}
	if r := EvaluateCondition(flag("--foo", domain.StateUnset), files, nil); r.Value != False {
		t.Errorf("control without the view: %s", r.Value)
	}
	not := domain.Condition{Op: domain.OpNot, Of: []domain.Condition{flag("--bar", domain.StateUnset)}}
	if r := EvaluateCondition(not, eff, nil); r.Value != Unknown {
		t.Errorf("negated absence under a gap: %s, want unknown", r.Value)
	}
	if r := EvaluateCondition(not, files, nil); r.Value != False {
		t.Errorf("negated absence without a gap: %s", r.Value)
	}
}
