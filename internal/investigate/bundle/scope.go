// Package bundle implements P1 of the Phase 4 design (DESIGN.md §2.2, §2.3,
// §7): change-evidence collection, environment snapshots and the evidence
// bundle assembler. It builds on internal/app (Upgrade, RenderDiffEdge),
// internal/env (Load) and internal/render, and writes through the
// investigate.ArtifactStore port.
//
// This file holds the declared scope (§1.3) and manifestation (§2.2.1) rule
// tables. They are pure functions of the evidence — no product-specific
// logic anywhere.
package bundle

import (
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// Scope rule names (DESIGN.md §1.3).
const (
	ScopeRoutineExcluded    = "scope:routine-excluded"
	ScopeBreaking           = "scope:breaking"
	ScopeComputedBreaking   = "scope:computed-breaking"
	ScopeRenderedStructural = domain.ScopeRenderedStructural
	// DESIGN.md §1.3 amendment (2026-10-16).
	ScopeRenderedUnexplained = domain.ScopeRenderedUnexplained
	ScopeRoutineSelfArtifact = domain.ScopeRoutineSelfArtifact
	ScopeCategory            = "scope:category"
	ScopeNotBreaking         = "scope:not-breaking"
)

// Manifestation rule names (DESIGN.md §2.2.1).
const (
	ManifestRuleRenderedAnchor     = "manifest:rendered-anchor"
	ManifestRuleComputedValues     = "manifest:computed-values"
	ManifestRuleComputedImage      = "manifest:computed-image"
	ManifestRuleComputedCompat     = "manifest:computed-compat"
	ManifestRuleComputedCRDStruct  = "manifest:computed-crd-structure"
	ManifestRuleCRDDefault         = "manifest:crd-default"
	ManifestRuleRenderedCorrelated = "manifest:rendered-correlated"
	ManifestRuleDeclaredBehaviour  = "manifest:declared-behaviour"
	ManifestRuleUnknown            = "manifest:unknown"
)

// computedBreakingRules are the computed diffs that remove or narrow
// (scope:computed-breaking). The rule ids are the upgrade package's
// Provenance.Rule values.
var computedBreakingRules = map[string]bool{
	"values:removed": true, "values:section-removed": true, "values:default-changed": true,
	"crd:removed": true, "crd:version-removed": true, "crd:version-unserved": true,
	"crd:version-deprecated": true, "crd:storage-changed": true, "crd:fields-removed": true,
	"crd:default-changed": true, "crd:enum-changed": true, "crd:field-required": true, "crd:field-type-changed": true,
	"images:removed": true, "images:moved": true, "artifacts:removed": true,
	"compat:narrowed": true, "compat:changed-raw": true, // the latter cannot be compared automatically: assume it narrows
}

// crdStructureRules are the CRD diffs about schema structure, decided against
// the manifests (manifest:computed-crd-structure).
var crdStructureRules = map[string]bool{
	"crd:removed": true, "crd:version-removed": true, "crd:version-unserved": true, "crd:version-deprecated": true,
	"crd:version-added": true, "crd:storage-changed": true, "crd:fields-removed": true,
	"crd:field-type-changed": true, "crd:field-required": true, "crd:enum-changed": true,
}

// structuralRenderClasses are the render change classes the default tier
// policy blocks (docs/POLICY.md): an unexplained release-scope change of one
// of them is its own in-scope item (scope:rendered-structural).
var structuralRenderClasses = map[render.ChangeClass]bool{
	render.RBACPermissionRemoved: true,
	render.ResourceRemoved:       true,
	render.APIVersionChanged:     true,
}

// isStructuralRender reports whether a release-scope rendered change is one
// the tier policy blocks, including CRD storage/served moves.
func isStructuralRender(c render.Change) bool {
	if structuralRenderClasses[c.Class] {
		return true
	}
	if c.Object.Kind == "CustomResourceDefinition" {
		p := c.Pattern
		if p == "" {
			p = c.Path
		}
		return strings.HasSuffix(p, ".served") || strings.HasSuffix(p, ".storage")
	}
	return false
}

// changeFacts is what the rules read about one item.
type changeFacts struct {
	Category       domain.Category
	Breaking       bool
	ActionRequired bool
	Routine        bool
	Computed       bool   // Provenance.Method == computed
	Rule           string // Provenance.Rule (the computed diff rule, or the note-parsing rule)
	// Platform marks a compatibility (platform) item; Narrowed is its
	// CompatibilityChange.Narrowed.
	Platform bool
	Narrowed bool
	// RenderedAnchored marks an item anchored on a release-scope rendered
	// change; RenderedScope is its scope rule (structural, unexplained or
	// self-artifact; "" = structural).
	RenderedAnchored bool
	RenderedScope    string
	// CorrelatedPaths are the paths of the release-scope rendered changes
	// correlated to the item by rule subject, values-default or artifact.
	CorrelatedPaths []string
}

func factsOf(c domain.Change, correlatedPaths []string) changeFacts {
	return changeFacts{
		Category: c.Category, Breaking: c.Breaking, ActionRequired: c.ActionRequired, Routine: c.Routine,
		Computed: c.Provenance.Method == domain.MethodComputed, Rule: c.Provenance.Rule, CorrelatedPaths: correlatedPaths,
	}
}

// scopeOf applies the §1.3 rules, first match wins.
func scopeOf(f changeFacts) (bool, string) {
	switch {
	case f.Routine:
		return false, ScopeRoutineExcluded
	case f.Breaking || f.ActionRequired:
		return true, ScopeBreaking
	case f.RenderedAnchored && f.RenderedScope == ScopeRoutineSelfArtifact:
		return false, ScopeRoutineSelfArtifact
	case f.RenderedAnchored && f.RenderedScope == ScopeRenderedUnexplained:
		return true, ScopeRenderedUnexplained
	case f.RenderedAnchored:
		return true, ScopeRenderedStructural
	case f.Platform && f.Narrowed:
		return true, ScopeComputedBreaking
	case f.Computed && computedBreakingRules[f.Rule]:
		return true, ScopeComputedBreaking
	case f.Category == domain.CategoryRemoval || f.Category == domain.CategoryDeprecation || f.Category == domain.CategoryMigration:
		return true, ScopeCategory
	}
	return false, ScopeNotBreaking
}

// manifestationOf applies the §2.2.1 rules, first match wins.
func manifestationOf(f changeFacts) (domain.Manifestation, string) {
	has := func(prefix string) bool { return strings.HasPrefix(f.Rule, prefix) }
	switch {
	case f.RenderedAnchored:
		return domain.ManifestRenderedConfig, ManifestRuleRenderedAnchor
	case f.Platform || (f.Computed && has("compat:")):
		return domain.ManifestRenderedConfig, ManifestRuleComputedCompat
	case f.Computed && has("values:"):
		return domain.ManifestRenderedConfig, ManifestRuleComputedValues
	case f.Computed && (has("images:") || has("artifacts:")):
		return domain.ManifestRenderedConfig, ManifestRuleComputedImage
	case f.Computed && crdStructureRules[f.Rule]:
		return domain.ManifestRenderedConfig, ManifestRuleComputedCRDStruct
	case f.Computed && f.Rule == "crd:default-changed":
		return domain.ManifestControllerBehaviour, ManifestRuleCRDDefault
	case !f.Computed && hasNonDescriptionPath(f.CorrelatedPaths):
		return domain.ManifestRenderedConfig, ManifestRuleRenderedCorrelated
	case !f.Computed:
		return domain.ManifestControllerBehaviour, ManifestRuleDeclaredBehaviour
	}
	return domain.ManifestUnknown, ManifestRuleUnknown
}

func hasNonDescriptionPath(paths []string) bool {
	for _, p := range paths {
		if !strings.HasSuffix(p, "description") {
			return true
		}
	}
	return false
}

// familyOf groups an item by how its exposure is decided (DESIGN.md §2.2).
func familyOf(f changeFacts) domain.ChangeItemFamily {
	has := func(prefix string) bool { return strings.HasPrefix(f.Rule, prefix) }
	switch {
	case f.RenderedAnchored:
		return domain.FamilyRendered
	case f.Platform || f.Category == domain.CategoryCompatibility || (f.Computed && has("compat:")):
		return domain.FamilyCompatibility
	case f.Computed && has("values:"), f.Category == domain.CategoryHelmValues:
		return domain.FamilyValues
	case f.Computed && (has("images:") || has("artifacts:")), f.Category == domain.CategoryArtifact:
		return domain.FamilyImage
	case f.Computed && has("crd:"), f.Category == domain.CategoryCRDSchema:
		return domain.FamilyAPISchema
	case f.Category == domain.CategoryMigration:
		return domain.FamilyProcedure
	}
	return domain.FamilyBehavior
}
