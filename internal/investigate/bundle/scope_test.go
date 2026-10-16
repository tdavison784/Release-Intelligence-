package bundle

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/render"
)

func TestScopeRules(t *testing.T) {
	computed := func(rule string) changeFacts { return changeFacts{Computed: true, Rule: rule} }
	cases := []struct {
		name  string
		f     changeFacts
		scope bool
		rule  string
	}{
		{"routine wins over breaking", changeFacts{Routine: true, Breaking: true}, false, ScopeRoutineExcluded},
		{"declared breaking", changeFacts{Breaking: true}, true, ScopeBreaking},
		{"declared action required", changeFacts{ActionRequired: true}, true, ScopeBreaking},
		{"computed values removed", computed("values:removed"), true, ScopeComputedBreaking},
		{"computed values default changed", computed("values:default-changed"), true, ScopeComputedBreaking},
		{"computed crd default changed", computed("crd:default-changed"), true, ScopeComputedBreaking},
		{"computed image removed", computed("images:removed"), true, ScopeComputedBreaking},
		{"computed image moved", computed("images:moved"), true, ScopeComputedBreaking},
		{"computed compat narrowed", computed("compat:narrowed"), true, ScopeComputedBreaking},
		{"narrowed platform item", changeFacts{Platform: true, Narrowed: true}, true, ScopeComputedBreaking},
		{"extended platform item", changeFacts{Platform: true}, false, ScopeNotBreaking},
		{"computed values added", computed("values:added"), false, ScopeNotBreaking},
		{"computed crd fields added", computed("crd:fields-added"), false, ScopeNotBreaking},
		{"computed image tag changed", computed("images:tag-changed"), false, ScopeNotBreaking},
		{"rendered anchor", changeFacts{RenderedAnchored: true}, true, ScopeRenderedStructural},
		{"rendered unexplained", changeFacts{RenderedAnchored: true, RenderedScope: ScopeRenderedUnexplained}, true, ScopeRenderedUnexplained},
		{"rendered self-artifact", changeFacts{RenderedAnchored: true, RenderedScope: ScopeRoutineSelfArtifact}, false, ScopeRoutineSelfArtifact},
		{"removal category", changeFacts{Category: domain.CategoryRemoval}, true, ScopeCategory},
		{"deprecation category", changeFacts{Category: domain.CategoryDeprecation}, true, ScopeCategory},
		{"migration category", changeFacts{Category: domain.CategoryMigration}, true, ScopeCategory},
		{"feature", changeFacts{Category: domain.CategoryFeature}, false, ScopeNotBreaking},
		{"bugfix", changeFacts{Category: domain.CategoryBugfix}, false, ScopeNotBreaking},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in, rule := scopeOf(c.f)
			if in != c.scope || rule != c.rule {
				t.Fatalf("scopeOf = (%v, %s), want (%v, %s)", in, rule, c.scope, c.rule)
			}
		})
	}
}

func TestManifestationRules(t *testing.T) {
	computed := func(rule string) changeFacts { return changeFacts{Computed: true, Rule: rule} }
	cases := []struct {
		name string
		f    changeFacts
		want domain.Manifestation
		rule string
	}{
		{"rendered anchor", changeFacts{RenderedAnchored: true}, domain.ManifestRenderedConfig, ManifestRuleRenderedAnchor},
		{"computed values", computed("values:default-changed"), domain.ManifestRenderedConfig, ManifestRuleComputedValues},
		{"computed image", computed("images:removed"), domain.ManifestRenderedConfig, ManifestRuleComputedImage},
		{"computed artifact", computed("artifacts:removed"), domain.ManifestRenderedConfig, ManifestRuleComputedImage},
		{"computed compat", computed("compat:narrowed"), domain.ManifestRenderedConfig, ManifestRuleComputedCompat},
		{"platform item", changeFacts{Platform: true}, domain.ManifestRenderedConfig, ManifestRuleComputedCompat},
		{"crd removed", computed("crd:removed"), domain.ManifestRenderedConfig, ManifestRuleComputedCRDStruct},
		{"crd fields removed", computed("crd:fields-removed"), domain.ManifestRenderedConfig, ManifestRuleComputedCRDStruct},
		{"crd version unserved", computed("crd:version-unserved"), domain.ManifestRenderedConfig, ManifestRuleComputedCRDStruct},
		{"crd required", computed("crd:field-required"), domain.ManifestRenderedConfig, ManifestRuleComputedCRDStruct},
		{"crd default is server-side behaviour", computed("crd:default-changed"), domain.ManifestControllerBehaviour, ManifestRuleCRDDefault},
		{"declared with a rendered subject match", changeFacts{CorrelatedPaths: []string{"spec.template.spec.containers[name=a].args"}}, domain.ManifestRenderedConfig, ManifestRuleRenderedCorrelated},
		// description-only paths no longer reach CorrelatedPaths at all (a
		// crd-description-changed render change correlates to no item), so
		// every correlated path is a real one; see render's correlate tests.
		{"declared with no render match", changeFacts{}, domain.ManifestControllerBehaviour, ManifestRuleDeclaredBehaviour},
		{"computed rule nobody classified fails closed", computed("something:new"), domain.ManifestUnknown, ManifestRuleUnknown},
		{"computed crd fields added fails closed", computed("crd:fields-added"), domain.ManifestUnknown, ManifestRuleUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, rule := manifestationOf(c.f)
			if got != c.want || rule != c.rule {
				t.Fatalf("manifestationOf = (%s, %s), want (%s, %s)", got, rule, c.want, c.rule)
			}
		})
	}
	// The unknown manifestation is never render-decisive.
	if m, _ := manifestationOf(computed("something:new")); m.RenderDecisive() {
		t.Fatal("unknown must not be render-decisive")
	}
}

func TestFamilies(t *testing.T) {
	cases := map[string]struct {
		f    changeFacts
		want domain.ChangeItemFamily
	}{
		"values":    {changeFacts{Computed: true, Rule: "values:removed"}, domain.FamilyValues},
		"image":     {changeFacts{Computed: true, Rule: "images:moved"}, domain.FamilyImage},
		"crd":       {changeFacts{Computed: true, Rule: "crd:removed"}, domain.FamilyAPISchema},
		"compat":    {changeFacts{Platform: true}, domain.FamilyCompatibility},
		"rendered":  {changeFacts{RenderedAnchored: true}, domain.FamilyRendered},
		"migrate":   {changeFacts{Category: domain.CategoryMigration}, domain.FamilyProcedure},
		"prose":     {changeFacts{Category: domain.CategoryConfiguration}, domain.FamilyBehavior},
		"noted CRD": {changeFacts{Category: domain.CategoryCRDSchema}, domain.FamilyAPISchema},
	}
	for name, c := range cases {
		if got := familyOf(c.f); got != c.want {
			t.Errorf("%s: family = %s, want %s", name, got, c.want)
		}
	}
}

func TestStructuralRenders(t *testing.T) {
	crd := render.ObjectID{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition", Name: "x"}
	cases := []struct {
		c    render.Change
		want bool
	}{
		{render.Change{Class: render.RBACPermissionRemoved}, true},
		{render.Change{Class: render.ResourceRemoved}, true},
		{render.Change{Class: render.APIVersionChanged}, true},
		{render.Change{Class: render.FieldChanged, Object: crd, Pattern: "spec.versions[].storage"}, true},
		{render.Change{Class: render.FieldChanged, Object: crd, Pattern: "spec.versions[].served"}, true},
		{render.Change{Class: render.FieldChanged, Object: crd, Pattern: "spec.versions[].schema.description"}, false},
		{render.Change{Class: render.ImageChanged}, false},
		{render.Change{Class: render.RBACPermissionAdded}, false},
	}
	for i, c := range cases {
		if got := isStructuralRender(c.c); got != c.want {
			t.Errorf("case %d (%s): %v, want %v", i, c.c.Class, got, c.want)
		}
	}
}
