package validate

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/redaction"
)

func kubeConstraint(kind, constraint string, ev domain.EvidenceID) *domain.CompatibilityConstraint {
	return &domain.CompatibilityConstraint{Platform: "kubernetes", Constraint: constraint, Raw: constraint, Kind: kind, SourceID: "compat",
		Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize@v1", Confidence: domain.ConfidenceHigh}, Evidence: []domain.EvidenceID{ev}}
}

func TestSchemaPlatformRange(t *testing.T) {
	cases := []struct {
		name, kube, kind string
		to               bool
		platform         string
		want             domain.ValidationOutcomeKind
		detail           string
	}{
		{"inside", "1.31", "supported", true, "kubernetes", domain.ValidationPass, "inside"},
		{"below", "1.27", "supported", true, "kubernetes", domain.ValidationFail, "outside"},
		{"outside tested only", "1.27", "tested", true, "kubernetes", domain.ValidationNotRun, "not a demonstrated incompatibility"},
		{"no cluster version", "", "supported", true, "kubernetes", domain.ValidationNotRun, "not supplied"},
		{"no target constraint", "1.31", "supported", false, "kubernetes", domain.ValidationNotRun, "states no"},
		{"other platform", "1.31", "supported", true, "openshift", domain.ValidationNotRun, "only the Kubernetes version"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFx(t)
			f.kube = c.kube
			cc := domain.CompatibilityChange{Platform: c.platform, Summary: "support narrowed", Narrowed: true,
				From: kubeConstraint(c.kind, ">=1.26.0-0 <1.34.0-0", f.up.ID)}
			if c.to {
				cc.To = kubeConstraint(c.kind, ">=1.29.0-0 <1.34.0-0", f.up.ID)
				cc.To.Platform = c.platform
			}
			item := f.platformItem(cc)
			b := f.bundle()
			r := one(t, run(t, SchemaCompat(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
			wantOutcome(t, r, c.want, c.detail)
		})
	}
}

func TestSchemaPlatformKindAnchor(t *testing.T) {
	f := newFx(t)
	f.kube = "1.27"
	f.platformItem(domain.CompatibilityChange{Platform: "kubernetes", Summary: "tested range moved",
		From: kubeConstraint("tested", ">=1.20.0-0", f.up.ID), To: kubeConstraint("tested", ">=1.29.0-0", f.up.ID)})
	item := f.platformItem(domain.CompatibilityChange{Platform: "kubernetes", Summary: "chart kubeVersion moved",
		From: kubeConstraint("chart-kubeVersion", ">=1.20.0-0", f.up.ID), To: kubeConstraint("chart-kubeVersion", ">=1.29.0-0", f.up.ID)})
	f.items[len(f.items)-1].Platform = "kubernetes/chart-kubeVersion"
	f.items[len(f.items)-1].ID = domain.ChangeItemID("", "kubernetes/chart-kubeVersion", "")
	item = f.items[len(f.items)-1].ID
	b := f.bundle()
	wantOutcome(t, one(t, run(t, SchemaCompat(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item})), domain.ValidationFail, "chart-kubeVersion")
}

func TestSchemaNoStoreIsNotRun(t *testing.T) {
	f := newFx(t)
	item := f.platformItem(domain.CompatibilityChange{Platform: "kubernetes", Summary: "x"})
	b := f.bundle()
	wantOutcome(t, one(t, run(t, SchemaCompat(), investigate.ValidationInput{Bundle: b, ChangeItem: item})), domain.ValidationNotRun, "cannot be read")
}

func apiItem(it *domain.ChangeItem) {
	it.Family, it.ManifestationRule = domain.FamilyAPISchema, "manifest:computed-crd-structure"
}

func widgetV(name, version, extra string) string {
	return "apiVersion: gizmo.example.io/" + version + "\nkind: Widget\nmetadata:\n  name: " + name + "\n  namespace: apps\nspec:\n  size: 3\n" + extra
}

func TestSchemaCRDObjects(t *testing.T) {
	type tc struct {
		name   string
		change domain.Change
		files  []string // widget documents in an environment file
		render string   // widget documents in the environment to-render
		want   domain.ValidationOutcomeKind
		detail string
	}
	fieldsRemoved := domain.Change{ID: "chg-f", Category: domain.CategoryCRDSchema, Breaking: true,
		Title:    "Widget v1 schema: 1 field removed: `spec.legacy`",
		Detail:   "Fields no longer in the gizmo.example.io/v1 schema of widgets.gizmo.example.io are pruned from stored objects and rejected or dropped in manifests. Remove them from your resources.\nRemoved paths:\nspec.legacy",
		Subjects: []string{"spec.legacy"}, Provenance: domain.Provenance{Rule: "crd:fields-removed"}}
	versionRemoved := domain.Change{ID: "chg-v", Category: domain.CategoryCRDSchema, Breaking: true,
		Title: "API version `gizmo.example.io/v1alpha1` of Widget removed", Subjects: []string{"widgets.gizmo.example.io/v1alpha1"},
		Provenance: domain.Provenance{Rule: "crd:version-removed"}}
	enum := domain.Change{ID: "chg-e", Category: domain.CategoryCRDSchema, Breaking: true,
		Title:    "Widget v1 schema: allowed values changed: `spec.rotation`",
		Detail:   "Enum values changed in the gizmo.example.io/v1 schema of widgets.gizmo.example.io; a removed value is rejected by the API server:\nspec.rotation: removed [\"Hourly\"], added []",
		Subjects: []string{"spec.rotation"}, Provenance: domain.Provenance{Rule: "crd:enum-changed"}}
	required := domain.Change{ID: "chg-r", Category: domain.CategoryCRDSchema, Breaking: true,
		Title:    "Widget v1 schema: 1 field now required: `spec.owner`",
		Detail:   "Fields newly required in the gizmo.example.io/v1 schema of widgets.gizmo.example.io; objects that omit them are rejected:\nspec.owner",
		Subjects: []string{"spec.owner"}, Provenance: domain.Provenance{Rule: "crd:field-required"}}
	crdRemoved := domain.Change{ID: "chg-x", Category: domain.CategoryCRDSchema, Breaking: true,
		Title: "CRD `widgets.gizmo.example.io` (Widget) removed", Subjects: []string{"widgets.gizmo.example.io"}, Provenance: domain.Provenance{Rule: "crd:removed"}}
	defaults := domain.Change{ID: "chg-d", Category: domain.CategoryCRDSchema,
		Title:    "Widget v1 schema: 1 default changed: `spec.rotation`",
		Detail:   "Schema defaults changed in the gizmo.example.io/v1 schema of widgets.gizmo.example.io (old → new; defaults apply to objects that leave the field unset):\nspec.rotation: \"Never\" → \"Always\"",
		Subjects: []string{"spec.rotation"}, Provenance: domain.Provenance{Rule: "crd:default-changed"}}
	declared := domain.Change{ID: "chg-n", Category: domain.CategoryAPI, Breaking: true, Title: "the v1alpha1 API is gone",
		Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize@v1", Rule: "section:breaking", Confidence: domain.ConfidenceHigh}}

	cases := []tc{
		{"removed field set in a file", fieldsRemoved, []string{widgetV("w1", "v1", "  legacy: true\n")}, "", domain.ValidationFail, "prunes"},
		{"removed field set only in the render", fieldsRemoved, []string{widgetV("w1", "v1", "")}, widgetV("w2", "v1", "  legacy: true\n"), domain.ValidationFail, "w2"},
		{"removed field unused", fieldsRemoved, []string{widgetV("w1", "v1", "")}, widgetV("w2", "v1", ""), domain.ValidationPass, "every visible Widget"},
		{"removed version used", versionRemoved, []string{widgetV("w1", "v1alpha1", "")}, "", domain.ValidationFail, "no longer serves"},
		{"removed version unused", versionRemoved, []string{widgetV("w1", "v1", "")}, "", domain.ValidationPass, ""},
		{"removed enum value used", enum, []string{widgetV("w1", "v1", "  rotation: Hourly\n")}, "", domain.ValidationFail, "no longer allows"},
		{"enum value still allowed", enum, []string{widgetV("w1", "v1", "  rotation: Never\n")}, "", domain.ValidationPass, ""},
		// a withheld (redacted) value may be the removed one: never compatible
		{"enum value withheld", enum, []string{widgetV("w1", "v1", "  rotation: \""+redaction.Marker("Hourly")+"\"\n")}, "", domain.ValidationNotRun, "withheld (redacted) in 1 Widget object(s)"},
		{"enum value withheld, another removed", enum, []string{widgetV("w1", "v1", "  rotation: \""+redaction.Marker("Never")+"\"\n"), widgetV("w2", "v1", "  rotation: Hourly\n")}, "", domain.ValidationFail, "w2"},
		{"newly required omitted", required, []string{widgetV("w1", "v1", "")}, "", domain.ValidationFail, "omits spec.owner"},
		{"newly required set", required, []string{widgetV("w1", "v1", "  owner: team\n")}, "", domain.ValidationPass, ""},
		{"removed CRD still used", crdRemoved, []string{widgetV("w1", "v1", "")}, "", domain.ValidationFail, "no longer ships"},
		{"no object visible", fieldsRemoved, nil, "", domain.ValidationNotRun, "absence in the repository is not proof"},
		{"default change", defaults, []string{widgetV("w1", "v1", "")}, "", domain.ValidationNotRun, "does not make existing objects invalid"},
		{"declared change", declared, []string{widgetV("w1", "v1", "")}, "", domain.ValidationNotRun, "no schema rule applies"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFx(t)
			item := f.change(c.change, apiItem)
			if len(c.files) > 0 {
				f.file("apps/widgets.yaml", stream(c.files...))
			}
			to := controller()
			if c.render != "" {
				to = stream(controller(), c.render)
			}
			f.pair("argocd:apps/gizmo.yaml", controller(), to)
			b := f.bundle()
			g := newGraph(t, b)
			w1 := g.node(domain.NodeCustomResource, "gizmo.example.io/v1/Widget/apps/w1", false)
			g.root(item, w1)
			g.path(item, w1)
			gr, tr := g.done()
			r := one(t, run(t, SchemaCompat(WithStore(f.st)), investigate.ValidationInput{Bundle: b, Graph: gr, Traversal: tr, ChangeItem: item}))
			wantOutcome(t, r, c.want, c.detail)
			if c.want == domain.ValidationPass && c.name == "removed field unused" && !contains(r.Subjects, w1) {
				t.Fatalf("a pass covers the checked objects' nodes: %+v", r)
			}
		})
	}
}

func TestSchemaIncompleteViewNeverPasses(t *testing.T) {
	f := newFx(t)
	item := f.change(domain.Change{ID: "chg-f", Category: domain.CategoryCRDSchema, Breaking: true,
		Title:    "Widget v1 schema: 1 field removed: `spec.legacy`",
		Detail:   "Fields no longer in the gizmo.example.io/v1 schema of widgets.gizmo.example.io are pruned.",
		Subjects: []string{"spec.legacy"}, Provenance: domain.Provenance{Rule: "crd:fields-removed"}}, apiItem)
	f.file("apps/widgets.yaml", widgetV("w1", "v1", ""))
	f.brokenPair("argocd:apps/gizmo.yaml", "chart-unavailable")
	b := f.bundle()
	wantOutcome(t, one(t, run(t, SchemaCompat(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item})),
		domain.ValidationNotRun, "not every object is visible")
}

func TestSchemaApplies(t *testing.T) {
	v := SchemaCompat()
	if v.Applies(domain.FamilyValues) || !v.Applies(domain.FamilyAPISchema) || !v.Applies(domain.FamilyCompatibility) {
		t.Fatal("schema-compat applies to api-schema and compatibility items only")
	}
}
