package validate

import (
	"errors"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

const crdKey = "apiextensions.k8s.io/v1/CustomResourceDefinition//widgets.gizmo.example.io"

// renderFx: one values item, optionally with graph roots on the controller.
func renderFx(t *testing.T, mods ...func(*domain.ChangeItem)) (*fx, string) {
	f := newFx(t)
	item := f.change(domain.Change{ID: "chg-mode", Category: domain.CategoryHelmValues, Breaking: true, Title: "default of controller.mode changed",
		Subjects: []string{"controller.mode"}, Provenance: domain.Provenance{Rule: "values:default-changed"}}, mods...)
	return f, item
}

func withGraph(t *testing.T, b *domain.EvidenceBundle, item string) (*domain.RelationshipGraph, *domain.TraversalResult, string, string) {
	g := newGraph(t, b)
	dep := g.node(domain.NodeWorkload, deployKey, false)
	svc := g.node(domain.NodeService, "core/v1/Service/gizmo/gizmo", false)
	g.edge(domain.EdgeSelects, dep, svc, domain.EdgeEstablished)
	g.root(item, dep)
	p := g.path(item, dep, svc)
	gr, tr := g.done()
	return gr, tr, dep, p
}

func TestRenderDiffNoRenderIsNotRun(t *testing.T) {
	f, item := renderFx(t)
	b := f.bundle()
	r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationNotRun, "no environment render")
}

func TestRenderDiffIncompleteIsNotRun(t *testing.T) {
	f, item := renderFx(t)
	f.pair("argocd:apps/a.yaml", controller(), controller())
	f.brokenPair("argocd:apps/b.yaml", "values-incomplete: valuesFrom outside the repository")
	b := f.bundle()
	r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationNotRun, "valuesFrom outside the repository")
}

func TestRenderDiffBreakingEffectFails(t *testing.T) {
	f, item := renderFx(t)
	ids := f.pair("argocd:apps/a.yaml", controller("--mode=safe"), controller("--mode=fast"),
		rc("container-arg-changed", deployKey, "spec.template.spec.containers[name=controller].args", `"--mode=safe"`, `"--mode=fast"`, "values-default", item))
	f.brokenPair("argocd:apps/b.yaml", "chart-unavailable")
	b := f.bundle()
	g, tr, dep, p := withGraph(t, b, item)
	r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationFail, "reaches the environment's rendered deployment")
	if len(r.RenderedChanges) != 1 || r.RenderedChanges[0] != ids[0] || !contains(r.Subjects, dep) || !contains(r.Paths, p) {
		t.Fatalf("fail must cite the rendered change, its node and path: %+v", r)
	}
}

func TestRenderDiffBehaviourItemCanFail(t *testing.T) {
	f, item := renderFx(t, behaviour)
	f.pair("argocd:apps/a.yaml", controller(), controller("--mode=fast"),
		rc("container-arg-added", deployKey, "spec.template.spec.containers[name=controller].args", "", `"--mode=fast"`, "subject", item))
	b := f.bundle()
	wantOutcome(t, one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item})), domain.ValidationFail, "")
}

func TestRenderDiffSilentRenderedConfigPasses(t *testing.T) {
	f, item := renderFx(t)
	f.pair("argocd:apps/a.yaml", controller(), controller(),
		rc("label-changed", deployKey, "metadata.labels.version", `"1"`, `"2"`, "")) // uncorrelated: not this validator's business
	b := f.bundle()
	g, tr, dep, p := withGraph(t, b, item)
	r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationPass, "")
	if !contains(r.Subjects, dep) || !contains(r.Paths, p) || len(r.RenderedChanges) != 0 || r.ClearsBehaviour() {
		t.Fatalf("a silent complete render covers the rendered-config item's roots and paths: %+v", r)
	}
}

func TestRenderDiffDescriptionOnlyIsHarmless(t *testing.T) {
	f, item := renderFx(t)
	ids := f.pair("argocd:apps/a.yaml", controller(), controller(),
		rc("field-changed", crdKey, "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.description",
			`"defaults to safe"`, `"defaults to fast"`, "mentions-field", item))
	b := f.bundle()
	r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationPass, "description text only")
	if len(r.RenderedChanges) != 1 || r.RenderedChanges[0] != ids[0] {
		t.Fatalf("the harmless correlated change must be covered: %+v", r)
	}
}

// Render silence never clears a behaviour item — not even with only
// description text correlated (that text is often the only trace).
func TestRenderDiffSilenceNeverClearsBehaviour(t *testing.T) {
	for _, m := range []domain.Manifestation{domain.ManifestControllerBehaviour, domain.ManifestUnknown} {
		t.Run(string(m), func(t *testing.T) {
			f, item := renderFx(t, func(it *domain.ChangeItem) { it.Manifestation, it.ManifestationRule = m, "manifest:test" })
			f.pair("argocd:apps/a.yaml", controller(), controller(),
				rc("field-changed", crdKey, "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.description",
					`"a"`, `"b"`, "mentions-field", item))
			b := f.bundle()
			wantOutcome(t, one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item})), domain.ValidationNotRun, "render silence proves nothing")
		})
	}
}

func TestRenderDiffUndecidedShapes(t *testing.T) {
	cases := []struct {
		name   string
		mods   []func(*domain.ChangeItem)
		corr   string
		detail string
	}{
		{"non-breaking item with an effect", []func(*domain.ChangeItem){notBreaking}, "subject", "not breaking"},
		{"text-mention correlation", nil, "mentions-name", "text mention"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, item := renderFx(t, c.mods...)
			ids := f.pair("argocd:apps/a.yaml", controller(), controller("--mode=fast"),
				rc("container-arg-added", deployKey, "spec.template.spec.containers[name=controller].args", "", `"--mode=fast"`, c.corr, item))
			b := f.bundle()
			r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
			wantOutcome(t, r, domain.ValidationNotRun, c.detail)
			if !contains(r.RenderedChanges, ids[0]) {
				t.Fatalf("the undecided rendered change must be named: %+v", r)
			}
		})
	}
}

func TestRenderDiffRequestedChanges(t *testing.T) {
	f, item := renderFx(t)
	ids := f.pair("argocd:apps/a.yaml", controller(), controller("--mode=fast"),
		rc("field-changed", crdKey, "spec.x.description", `"a"`, `"b"`, "mentions-field", item),
		rc("container-arg-added", deployKey, "spec.template.spec.containers[name=controller].args", "", `"--mode=fast"`, "mentions-name", item))
	b := f.bundle()
	var desc string
	for _, c := range b.Renders[0].Changes {
		if c.Object == crdKey {
			desc = c.ID
		}
	}
	r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item, RenderedChanges: []string{desc}}))
	wantOutcome(t, r, domain.ValidationPass, "requested rendered changes")
	if len(r.Subjects)+len(r.Paths) != 0 || len(r.RenderedChanges) != 1 {
		t.Fatalf("a requested check covers only the requested changes: %+v", r)
	}
	_ = ids
	_, err := RenderDiff().Validate(t.Context(), investigate.ValidationInput{Bundle: b, ChangeItem: item, RenderedChanges: []string{"rc-unknown"}})
	if !errors.Is(err, investigate.ErrRejected) {
		t.Fatalf("an unknown rendered change is rejected, got %v", err)
	}
	_, err = RenderDiff().Validate(t.Context(), investigate.ValidationInput{Bundle: b, ChangeItem: item, Paths: []string{"ip-unknown"}})
	if !errors.Is(err, investigate.ErrRejected) {
		t.Fatalf("a path without traversal is rejected, got %v", err)
	}
}

func TestRenderDiffMultiRuleCorrelation(t *testing.T) {
	f, item := renderFx(t)
	f.pair("argocd:apps/a.yaml", controller("--mode=safe"), controller("--mode=fast"),
		rc("container-arg-changed", deployKey, "spec.template.spec.containers[name=controller].args", `"--mode=safe"`, `"--mode=fast"`, "mentions-field,values-default", item))
	b := f.bundle()
	wantOutcome(t, one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item})), domain.ValidationFail, "")
}

// scope:rendered-unexplained (DESIGN §1.3 amendment): pass only when the
// environment render positively shows the change does not land; never fail
// without a concrete failure; otherwise not-run.
func TestRenderDiffRenderedUnexplained(t *testing.T) {
	t.Run("does not reach the environment", func(t *testing.T) {
		f := newFx(t)
		item, _ := f.unexplained("rbac-permission-added", "rbac.authorization.k8s.io/v1/ClusterRole//gizmo", "rules")
		f.pair("argocd:apps/a.yaml", controller(), controller())
		b := f.bundle()
		r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
		wantOutcome(t, r, domain.ValidationPass, "nothing in it is attributable")
	})
	t.Run("reaches the environment", func(t *testing.T) {
		f := newFx(t)
		item, _ := f.unexplained("rbac-permission-added", "rbac.authorization.k8s.io/v1/ClusterRole//gizmo", "rules")
		f.pair("argocd:apps/a.yaml", controller(), controller(),
			rc("rbac-permission-added", "rbac.authorization.k8s.io/v1/ClusterRole//gizmo", "rules", "", `"secrets/get"`, "release-anchor", item))
		b := f.bundle()
		r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
		wantOutcome(t, r, domain.ValidationNotRun, "concrete failure")
	})
}
