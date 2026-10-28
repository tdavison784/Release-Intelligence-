package validate

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Lane join5 (D1) on the validator side: the pin scope, the override
// inspection and the schema check judge the objects as deployed (the
// effective view) and cite the file that produced a rendered value.

// The pinned value is cited at the overlay patch's line, not only as a
// rendered value.
func TestPinCitesTheOverlayPatch(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up))
	baseAndPatch(o.f, "Never")
	o.f.overlayRender(overlayTarget, mergedRender("Never"))
	o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
	r := o.run(t)
	wantOutcome(t, r, domain.ValidationPass, "set at env/apps/w/overlays/prod/rotation.yaml:L7")
	if strings.Contains(r.Detail, "set at env/apps/w/base/") {
		t.Fatalf("the base is cited for the patched value:\n%s", r.Detail)
	}
}

// The fail-open direction: the base pins the value, an overlay JSON patch
// removes it. The base file on its own would pin; the deployed object does
// not, so nothing clears.
func TestPinOverlayRemovingTheBaseSettingDoesNotClear(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(scopedFact("vf-all", domain.VerifiedHuman, o.f.up, widgetScope(rotationUnset())))
	o.f.file("apps/w/base/kustomization.yaml", "resources:\n  - widgets.yaml\n")
	o.f.file("apps/w/base/widgets.yaml", gw("w-root", "apps", map[string]string{"role": "root", "rotation": "Never"}))
	o.f.file("apps/w/overlays/prod/kustomization.yaml", "resources:\n  - ../../base\npatches:\n  - path: unpin.yaml\n    target: {kind: Widget, name: w-root}\n")
	o.f.file("apps/w/overlays/prod/unpin.yaml", "- op: remove\n  path: /spec/rotation\n")
	o.f.overlayRender(overlayTarget, gw("w-root", "apps", map[string]string{"role": "root"}))
	o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
	r := o.run(t)
	if r.Outcome == domain.ValidationPass || r.ClearsBehaviour() {
		t.Fatalf("cleared on the base's setting: %s\n%s", r.Outcome, r.Detail)
	}
	wantOutcome(t, r, r.Outcome, "leaves spec.rotation unset")
}

// The schema check judges the deployed object: a removed enum value the base
// states but the overlay replaces is not a violation; the base's object is
// not judged on its own; without the render nothing is judged.
func TestSchemaJudgesTheDeployedObject(t *testing.T) {
	enum := domain.Change{ID: "chg-e", Category: domain.CategoryCRDSchema, Breaking: true,
		Title:    "Widget v1 schema: allowed values changed: `spec.rotation`",
		Detail:   "Enum values changed in the gizmo.example.io/v1 schema of widgets.gizmo.example.io; a removed value is rejected by the API server:\nspec.rotation: removed [\"Hourly\"], added []",
		Subjects: []string{"spec.rotation"}, Provenance: domain.Provenance{Rule: "crd:enum-changed"}}
	for _, c := range []struct {
		name     string
		rendered bool
		want     domain.ValidationOutcomeKind
		detail   string
	}{
		{"rendered", true, domain.ValidationPass, "Widget/apps/w1 (derived/render/env/kustomize-apps-w-overlays-prod/from.yaml)"},
		// the base and the patch: both declarations, neither judged
		{"render unavailable", false, domain.ValidationNotRun, "deployed form of 2 Widget object(s) is unknown"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFx(t)
			item := f.change(enum, apiItem)
			f.file("apps/w/base/kustomization.yaml", "resources:\n  - widgets.yaml\n")
			f.file("apps/w/base/widgets.yaml", widgetV("w1", "v1", "  rotation: Hourly\n"))
			f.file("apps/w/overlays/prod/kustomization.yaml", "resources:\n  - ../../base\npatches:\n  - path: rotation.yaml\n")
			f.file("apps/w/overlays/prod/rotation.yaml", widgetV("w1", "v1", "  rotation: Never\n"))
			if c.rendered {
				f.overlayRender(overlayTarget, widgetV("w1", "v1", "  rotation: Never\n"))
			} else {
				f.naPair(overlayTarget)
			}
			f.pair("argocd:apps/gizmo.yaml", controller(), controller())
			b := f.bundle()
			r := one(t, run(t, SchemaCompat(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
			wantOutcome(t, r, c.want, c.detail)
			if strings.Contains(r.Detail, "no longer allows") {
				t.Fatalf("the base file's value was judged:\n%s", r.Detail)
			}
		})
	}
}
