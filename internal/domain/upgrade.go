package domain

import (
	"errors"
	"fmt"
	"time"
)

// Category is the primary classification of a change.
type Category string

const (
	CategoryFeature       Category = "feature"
	CategoryBugfix        Category = "bugfix"
	CategoryDeprecation   Category = "deprecation"
	CategoryRemoval       Category = "removal"       // removed functionality / APIs
	CategoryMigration     Category = "migration"     // explicit migration / upgrade step
	CategoryConfiguration Category = "configuration" // flags, config files, defaults
	CategoryHelmValues    Category = "helm-values"
	CategoryCRDSchema     Category = "crd-schema"
	CategoryAPI           Category = "api" // API versions served/stored, API behaviour
	CategoryCompatibility Category = "compatibility"
	CategorySecurity      Category = "security"
	CategoryArtifact      Category = "artifact"
	CategoryDependency    Category = "dependency"
	CategoryOther         Category = "other"
)

// AllCategories lists every category in display order.
var AllCategories = []Category{
	CategoryMigration, CategoryRemoval, CategoryDeprecation, CategoryAPI, CategoryCRDSchema,
	CategoryHelmValues, CategoryConfiguration, CategoryCompatibility, CategorySecurity,
	CategoryArtifact, CategoryDependency, CategoryFeature, CategoryBugfix, CategoryOther,
}

// Change is a deterministic conclusion about an upgrade edge. It may come from
// a release note item (declared/heuristic) or from a computed diff.
type Change struct {
	ID             string       `json:"id"`
	Category       Category     `json:"category"`
	Breaking       bool         `json:"breaking,omitempty"`
	ActionRequired bool         `json:"actionRequired,omitempty"`
	Title          string       `json:"title"`
	Detail         string       `json:"detail,omitempty"`
	Release        string       `json:"release,omitempty"`  // release that introduced it; "" for endpoint diffs
	Subjects       []string     `json:"subjects,omitempty"` // affected keys, CRDs, images, APIs
	References     []Reference  `json:"references,omitempty"`
	Provenance     Provenance   `json:"provenance"`
	Facts          []FactID     `json:"facts,omitempty"`
	Evidence       []EvidenceID `json:"evidence"`
}

// ChangeType for artifact deltas.
type ChangeType string

const (
	ChangeAdded     ChangeType = "added"
	ChangeRemoved   ChangeType = "removed"
	ChangeUpdated   ChangeType = "updated"
	ChangeUnchanged ChangeType = "unchanged"
)

// ArtifactChange compares an artifact between the two endpoints of an edge.
type ArtifactChange struct {
	ArtifactID string            `json:"artifactId"`
	Type       ArtifactType      `json:"type"`
	Name       string            `json:"name"`
	Change     ChangeType        `json:"change"`
	From       *ArtifactInstance `json:"from,omitempty"`
	To         *ArtifactInstance `json:"to,omitempty"`
	Evidence   []EvidenceID      `json:"evidence,omitempty"`
}

// CompatibilityChange compares a platform constraint between endpoints.
type CompatibilityChange struct {
	Platform string                   `json:"platform"`
	From     *CompatibilityConstraint `json:"from,omitempty"`
	To       *CompatibilityConstraint `json:"to,omitempty"`
	Summary  string                   `json:"summary"`
	// Narrowed is true when the target supports platform versions that are a
	// strict subset of the source's (e.g. dropped support for an old Kubernetes).
	Narrowed bool `json:"narrowed,omitempty"`
}

// PathStep is a release traversed by an upgrade edge.
type PathStep struct {
	Version     Version    `json:"version"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	Reason      string     `json:"reason"` // e.g. "minor-release", "target-line-patch"
}

// UpgradeEdgeSchemaVersion is the current serialisation version.
const UpgradeEdgeSchemaVersion = "ri.dev/upgrade-edge/v1alpha1"

// UpgradeEdge describes everything relevant to upgrading Product from From to To.
type UpgradeEdge struct {
	SchemaVersion string     `json:"schemaVersion"`
	Product       ProductRef `json:"product"`
	From          Version    `json:"from"`
	To            Version    `json:"to"`
	PathPolicy    string     `json:"pathPolicy"`
	Path          []PathStep `json:"path"`
	// SkippedReleases are releases between the endpoints deliberately not
	// traversed (e.g. backport patches on intermediate lines).
	SkippedReleases []Version `json:"skippedReleases,omitempty"`

	Sources       []SourceStatus        `json:"sources"`
	Changes       []Change              `json:"changes"`
	Compatibility []CompatibilityChange `json:"compatibility,omitempty"`
	Artifacts     []ArtifactChange      `json:"artifacts,omitempty"`
	Enrichments   []Enrichment          `json:"enrichments,omitempty"`
	Facts         []Fact                `json:"facts,omitempty"`
	Evidence      []Evidence            `json:"evidence"`
	Warnings      []string              `json:"warnings,omitempty"`

	GeneratedAt      time.Time `json:"generatedAt"`
	DefinitionDigest string    `json:"definitionDigest,omitempty"`

	// EnrichmentRun describes the AI run that produced Enrichments (absent
	// when no enrichment was attempted). See enrichment.go.
	EnrichmentRun *EnrichmentRun `json:"enrichmentRun,omitempty"`
}

// ChangesWhere returns changes matching pred.
func (e *UpgradeEdge) ChangesWhere(pred func(Change) bool) []Change {
	var out []Change
	for _, c := range e.Changes {
		if pred(c) {
			out = append(out, c)
		}
	}
	return out
}

// Validate enforces the provenance invariants of an edge:
//   - every Change has deterministic provenance and at least one evidence ID
//     that resolves within the edge;
//   - every Enrichment has complete AI provenance, relates to existing
//     Changes and cites only evidence it was given, which resolves within the
//     edge (see validateEnrichments);
//   - every fact's evidence resolves.
func (e *UpgradeEdge) Validate() error {
	var errs []error
	ev := map[EvidenceID]bool{}
	for _, x := range e.Evidence {
		if ev[x.ID] {
			errs = append(errs, fmt.Errorf("duplicate evidence id %s", x.ID))
		}
		ev[x.ID] = true
	}
	facts := map[FactID]bool{}
	for _, f := range e.Facts {
		facts[f.ID] = true
		for _, id := range f.Evidence {
			if !ev[id] {
				errs = append(errs, fmt.Errorf("fact %s references unknown evidence %s", f.ID, id))
			}
		}
	}
	for _, c := range e.Changes {
		if err := c.Provenance.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("change %s: %w", c.ID, err))
		}
		if !c.Provenance.Deterministic() {
			errs = append(errs, fmt.Errorf("change %s: AI-derived content must be an Enrichment, not a Change", c.ID))
		}
		if len(c.Evidence) == 0 {
			errs = append(errs, fmt.Errorf("change %s (%q) has no evidence", c.ID, c.Title))
		}
		for _, id := range c.Evidence {
			if !ev[id] {
				errs = append(errs, fmt.Errorf("change %s references unknown evidence %s", c.ID, id))
			}
		}
		for _, id := range c.Facts {
			if !facts[id] {
				errs = append(errs, fmt.Errorf("change %s references unknown fact %s", c.ID, id))
			}
		}
	}
	errs = append(errs, e.validateEnrichments(ev)...)
	if e.From.Compare(e.To) >= 0 {
		errs = append(errs, errors.New("edge 'from' must be lower than 'to'"))
	}
	return errors.Join(errs...)
}
