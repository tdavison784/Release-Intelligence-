package validate

import (
	"errors"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
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

func TestRenderDiffIgnoresNotApplicableTargets(t *testing.T) {
	f, item := renderFx(t)
	f.pair("argocd:apps/a.yaml", controller(), controller())
	f.naPair("kustomize:apps/other/overlays/prod")
	b := f.bundle()
	wantOutcome(t, one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item})), domain.ValidationPass, "")

	f2, item2 := renderFx(t)
	f2.naPair("kustomize:apps/other/overlays/prod")
	b2 := f2.bundle()
	wantOutcome(t, one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b2, ChangeItem: item2})), domain.ValidationNotRun, "no environment render")
}

func TestRenderDiffIncompleteIsNotRun(t *testing.T) {
	f, item := renderFx(t)
	f.pair("argocd:apps/a.yaml", controller(), controller())
	f.brokenPair("argocd:apps/b.yaml", "values-incomplete: valuesFrom outside the repository")
	b := f.bundle()
	r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationNotRun, "valuesFrom outside the repository")
}

// Exposure is not failure: a breaking item's effect reaching the rendered
// deployment, with no concrete failure, is inconclusive (error) — R3, never
// R1 — and the record names the exposure, its node and path.
func TestRenderDiffExposureIsInconclusive(t *testing.T) {
	for _, store := range []bool{false, true} {
		t.Run(map[bool]string{false: "no store", true: "store"}[store], func(t *testing.T) {
			f, item := renderFx(t)
			ids := f.pair("argocd:apps/a.yaml", controller("--mode=safe"), controller("--mode=fast"),
				rc("container-arg-changed", deployKey, "spec.template.spec.containers[name=controller].args", `"--mode=safe"`, `"--mode=fast"`, "values-default", item))
			f.brokenPair("argocd:apps/b.yaml", "chart-unavailable")
			b := f.bundle()
			g, tr, dep, p := withGraph(t, b, item)
			var opts []Option
			want := "could not run: no artifact store"
			if store {
				opts, want = append(opts, WithStore(f.st)), "no concrete failure found"
			}
			r := one(t, run(t, RenderDiff(opts...), investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: item}))
			wantOutcome(t, r, domain.ValidationError, "exposure: container-arg-changed spec.template.spec.containers[name=controller].args reaches "+deployKey)
			wantOutcome(t, r, domain.ValidationError, want)
			if len(r.RenderedChanges) != 1 || r.RenderedChanges[0] != ids[0] || !contains(r.Subjects, dep) || !contains(r.Paths, p) {
				t.Fatalf("the exposure record must cite the rendered change, its node and path: %+v", r)
			}
		})
	}
}

// Every shape of a strongly correlated effect is inconclusive without a
// concrete failure: behaviour items, non-breaking items, multi-rule
// correlations — the same rendered change gets the same outcome whatever the
// item's breaking flag (two environments are told apart by consequences).
func TestRenderDiffEffectShapesAreInconclusive(t *testing.T) {
	cases := []struct {
		name string
		mods []func(*domain.ChangeItem)
		corr string
	}{
		{"behaviour item", []func(*domain.ChangeItem){behaviour}, "subject"},
		{"non-breaking item", []func(*domain.ChangeItem){notBreaking}, "subject"},
		{"multi-rule correlation", nil, "mentions-field,values-default"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, item := renderFx(t, c.mods...)
			f.pair("argocd:apps/a.yaml", controller(), controller("--mode=fast"),
				rc("container-arg-added", deployKey, "spec.template.spec.containers[name=controller].args", "", `"--mode=fast"`, c.corr, item))
			b := f.bundle()
			wantOutcome(t, one(t, run(t, RenderDiff(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item})), domain.ValidationError, "exposure:")
		})
	}
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

// CRD description text (the declared render class) documents behaviour, it
// configures nothing: the validator passes it as harmless — correlated to the
// item by the field it documents (render/correlate documents-field), or as a
// requested check.
func TestRenderDiffDescriptionOnlyIsHarmless(t *testing.T) {
	for _, requested := range []bool{false, true} {
		t.Run(map[bool]string{false: "correlated", true: "requested"}[requested], func(t *testing.T) {
			f, item := renderFx(t)
			corr, items := "documents-field", []string{item}
			if requested {
				corr, items = "", nil
			}
			ids := f.pair("argocd:apps/a.yaml", controller(), controller(),
				rc(string(render.CRDDescriptionChanged), crdKey, "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.description",
					`"defaults to safe"`, `"defaults to fast"`, corr, items...))
			b := f.bundle()
			in := investigate.ValidationInput{Bundle: b, ChangeItem: item}
			if requested {
				in.RenderedChanges = ids
			}
			r := one(t, run(t, RenderDiff(), in))
			wantOutcome(t, r, domain.ValidationPass, "description text only")
			if len(r.RenderedChanges) != 1 || r.RenderedChanges[0] != ids[0] {
				t.Fatalf("the harmless description change must be covered: %+v", r)
			}
		})
	}
}

// Render silence never clears a behaviour item — not even when the only
// correlated trace is the reworded documentation of its field (that text is
// often the only trace of a controller default: it is accounted for as
// documentation, never as a clearance), nor a text-mentioned schema change.
func TestRenderDiffSilenceNeverClearsBehaviour(t *testing.T) {
	traces := map[string]func(item string) domain.RenderedChange{
		"description of its field": func(item string) domain.RenderedChange {
			return rc(string(render.CRDDescriptionChanged), crdKey, "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.description",
				`"defaults to safe"`, `"defaults to fast"`, "documents-field", item)
		},
		"text-mentioned schema change": func(item string) domain.RenderedChange {
			return rc("field-changed", crdKey, "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.type",
				`"string"`, `"integer"`, "mentions-field", item)
		},
	}
	for _, m := range []domain.Manifestation{domain.ManifestControllerBehaviour, domain.ManifestUnknown} {
		for name, trace := range traces {
			t.Run(string(m)+"/"+name, func(t *testing.T) {
				f, item := renderFx(t, func(it *domain.ChangeItem) { it.Manifestation, it.ManifestationRule = m, "manifest:test" })
				f.pair("argocd:apps/a.yaml", controller(), controller(), trace(item))
				b := f.bundle()
				r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
				if r.ClearsBehaviour() || len(r.Subjects)+len(r.Paths) > 0 {
					t.Fatalf("render silence cleared a %s item: %+v", m, r)
				}
				if r.Outcome == domain.ValidationPass && !r.DocumentationOnly {
					t.Fatalf("a pass on a %s item that is not documentation-only coverage: %+v", m, r)
				}
				if name == "description of its field" {
					wantOutcome(t, r, domain.ValidationPass, "not a behaviour clearance")
					return
				}
				wantOutcome(t, r, domain.ValidationNotRun, "render silence proves nothing")
			})
		}
	}
}

func TestRenderDiffUndecidedShapes(t *testing.T) {
	cases := []struct {
		name   string
		mods   []func(*domain.ChangeItem)
		corr   string
		detail string
	}{
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
		rc(string(render.CRDDescriptionChanged), crdKey, "spec.x.description", `"a"`, `"b"`, ""),
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
		wantOutcome(t, r, domain.ValidationError, "exposure: rbac-permission-added rules reaches")
	})
}

// Regression (classify follow-up): an ENVIRONMENT-scope item —
// scope:rendered-unexplained anchored on an environment rendered change, in
// EvidenceBundle.EnvironmentItems — is a known item to every validator: each
// returns real records about it (never ErrNotFound), the records conform, and
// no validator fails it without a concrete failure.
func TestEnvironmentScopeItemGetsRecords(t *testing.T) {
	f := newFx(t)
	ids := f.pair("argocd:apps/a.yaml", controller(), controller("--extra=1"),
		rc("container-arg-added", deployKey, "spec.template.spec.containers[name=controller].args[=--extra]", "", `"--extra=1"`, ""))
	item := f.envUnexplained(ids[0])
	b := f.bundle()
	if _, inRecord := b.Change.Item(item); inRecord {
		t.Fatal("fixture: the item must live in EnvironmentItems only")
	}
	total := 0
	for _, v := range allValidators(f.st) {
		if !v.Applies(domain.FamilyRendered) {
			continue
		}
		out := run(t, v, investigate.ValidationInput{Bundle: b, ChangeItem: item})
		for _, r := range out.Records {
			if r.Outcome == domain.ValidationFail {
				t.Fatalf("%s failed a rendered-unexplained item without a concrete failure: %s", v.Name(), r.Detail)
			}
		}
		total += len(out.Records)
		if v.Name() == NameRenderDiff {
			r := one(t, out)
			if len(r.RenderedChanges) != 1 || r.RenderedChanges[0] != ids[0] {
				t.Fatalf("render-diff must speak for the anchoring change: %+v", r.RenderedChanges)
			}
			wantOutcome(t, r, domain.ValidationError, "no concrete failure found")
		}
	}
	if total == 0 {
		t.Fatal("no validator produced a record for the environment-scope item")
	}
	// A requested rendered change of the item resolves too.
	run(t, RenderDiff(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item, RenderedChanges: ids})
}

const (
	descPath   = "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.description"
	otherCRD   = "apiextensions.k8s.io/v1/CustomResourceDefinition//sprockets.gizmo.example.io"
	schemaType = "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.type"
)

func descChange(item string) domain.RenderedChange {
	return rc(string(render.CRDDescriptionChanged), crdKey, descPath, `"defaults to safe"`, `"defaults to fast"`, "documents-field", item)
}

// A behaviour item whose only correlated rendered change is CRD description
// text: a documentation-only pass that accounts for exactly that change and
// for nothing else (no subject, no path, no answered hypothesis) — never a
// behaviour clearance. Correlated, requested, or requested by an agent
// request testing a hypothesis.
func TestRenderDiffDocumentationOnlyAccountsForBehaviourItem(t *testing.T) {
	for _, mode := range []string{"correlated", "requested", "agent request"} {
		t.Run(mode, func(t *testing.T) {
			f, item := renderFx(t, behaviour)
			ids := f.pair("argocd:apps/a.yaml", controller(), controller(), descChange(item))
			b := f.bundle()
			g, tr, dep, _ := withGraph(t, b, item)
			in := investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: item}
			switch mode {
			case "requested":
				in.RenderedChanges = ids
			case "agent request":
				in.RenderedChanges = ids
				in.Request = testRequest(b, item, domain.ValidationRenderDiff, nil)
				in.Request.Hypothesis = domain.AgentRecordID(domain.HypothesisIDPrefix, item, "a hypothesis", "call-1")
			}
			r := one(t, run(t, RenderDiff(), in))
			wantOutcome(t, r, domain.ValidationPass, "documentation, not configuration")
			if !r.DocumentationOnly || r.ClearsBehaviour() {
				t.Fatalf("want documentation-only coverage that clears nothing: %+v", r)
			}
			if len(r.RenderedChanges) != 1 || r.RenderedChanges[0] != ids[0] || len(r.Subjects)+len(r.Paths)+len(r.Answers) != 0 {
				t.Fatalf("documentation-only coverage names exactly the description change: %+v", r)
			}
			if contains(r.Subjects, dep) {
				t.Fatalf("documentation-only coverage must not cover the exposure root: %+v", r)
			}
		})
	}
}

// The opposite (fail-open) implementation would account for a description
// change whatever else its CRD does. A schema, default, enum, required,
// served or storage change in the same CRD — correlated to the item or to
// nothing — leaves the description change unaccounted for; a change in
// another CRD does not.
func TestRenderDiffDocumentationOnlyNeedsAnOtherwiseUnchangedCRD(t *testing.T) {
	cases := []struct {
		name    string
		extra   func(item string) domain.RenderedChange
		outcome domain.ValidationOutcomeKind
		detail  string
	}{
		{"uncorrelated schema change in the same CRD", func(string) domain.RenderedChange {
			return rc("field-changed", crdKey, schemaType, `"string"`, `"integer"`, "")
		}, domain.ValidationNotRun, "also changes beyond description text"},
		{"schema default change in the same CRD, correlated to nothing", func(string) domain.RenderedChange {
			return rc("field-added", crdKey, "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.default", "", `"fast"`, "")
		}, domain.ValidationNotRun, "also changes beyond description text"},
		{"served flag change in the same CRD", func(string) domain.RenderedChange {
			return rc("field-changed", crdKey, "spec.versions[name=v1].served", "true", "false", "")
		}, domain.ValidationNotRun, "also changes beyond description text"},
		{"text-mentioned schema change in the same CRD", func(item string) domain.RenderedChange {
			return rc("field-changed", crdKey, schemaType, `"string"`, `"integer"`, "mentions-field", item)
		}, domain.ValidationNotRun, "render silence proves nothing"},
		{"schema change of the item in the same CRD", func(item string) domain.RenderedChange {
			return rc("field-changed", crdKey, schemaType, `"string"`, `"integer"`, "subject", item)
		}, domain.ValidationError, "inconclusive"},
		{"schema change in another CRD", func(string) domain.RenderedChange {
			return rc("field-changed", otherCRD, schemaType, `"string"`, `"integer"`, "")
		}, domain.ValidationPass, "documentation, not configuration"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, item := renderFx(t, behaviour)
			ids := f.pair("argocd:apps/a.yaml", controller(), controller(), descChange(item), c.extra(item))
			b := f.bundle()
			r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
			wantOutcome(t, r, c.outcome, c.detail)
			if r.ClearsBehaviour() {
				t.Fatalf("cleared a behaviour item: %+v", r)
			}
			if c.outcome != domain.ValidationPass {
				if r.DocumentationOnly {
					t.Fatalf("documentation-only coverage of a CRD that changes beyond description text: %+v", r)
				}
				return
			}
			if !r.DocumentationOnly || len(r.RenderedChanges) != 1 || r.RenderedChanges[0] != ids[0] {
				t.Fatalf("want documentation-only coverage of the description change only: %+v", r)
			}
		})
	}
}

// An incomplete environment render accounts for nothing, documentation or not.
func TestRenderDiffDocumentationOnlyNeedsCompleteRenders(t *testing.T) {
	f, item := renderFx(t, behaviour)
	f.pair("argocd:apps/a.yaml", controller(), controller(), descChange(item))
	f.brokenPair("argocd:apps/b.yaml", "chart-unavailable")
	b := f.bundle()
	r := one(t, run(t, RenderDiff(), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationNotRun, "chart-unavailable")
	if r.DocumentationOnly {
		t.Fatalf("documentation-only coverage from an incomplete render: %+v", r)
	}
}

// The validate package mirrors the render class the domain predicate reads.
func TestDocumentationOnlyClassMatchesRender(t *testing.T) {
	if domain.RenderedClassCRDDescription != string(render.CRDDescriptionChanged) {
		t.Fatalf("domain.RenderedClassCRDDescription %q != render.CRDDescriptionChanged %q", domain.RenderedClassCRDDescription, render.CRDDescriptionChanged)
	}
}
