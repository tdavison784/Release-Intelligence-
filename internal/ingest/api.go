// Package ingest is the deterministic ingestion pipeline. Driven only by a
// catalog.ProductDefinition and a sources.Registry, it lists a product's
// releases, retrieves the documents and artifacts of one release, normalises
// them (via package normalize) and produces a domain.Release carrying facts,
// evidence and per-source status. It never calls an LLM.
//
// CONTRACT NOTE: the exported API in this file is used by the CLI, by package
// discovery (CheckRelationships) and by the upgrade orchestration.
package ingest

import (
	"context"
	"errors"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// ErrNotImplemented is returned by contract stubs not yet implemented.
var ErrNotImplemented = errors.New("ingest: not implemented")

// Producer identifier used as fact extractor.
const Producer = "ingest@v1"

// ErrNoVersions is returned by ListVersions when no versions source yields a
// release. The returned VersionList is still non-nil and carries the status
// of every source consulted.
var ErrNoVersions = errors.New("ingest: no versions source answered")

// DefaultConcurrency bounds parallel retrieval within one release.
const DefaultConcurrency = 4

// Ingester runs the deterministic pipeline.
type Ingester struct {
	Registry *sources.Registry
	// Clock returns the current time (overridable in tests).
	Clock func() time.Time
	// Parser performs the pure parsing steps; nil means DefaultParser
	// (package normalize). Tests substitute a fake.
	Parser Parser
	// Concurrency bounds how many sources / artifacts of one release are
	// retrieved in parallel (0 = DefaultConcurrency, 1 = sequential). Output
	// ordering never depends on it.
	Concurrency int
}

// New returns an Ingester over a registry.
func New(reg *sources.Registry) *Ingester {
	return &Ingester{Registry: reg, Clock: time.Now, Parser: DefaultParser}
}

// VersionList is the canonical list of releases of a product.
type VersionList struct {
	Product  domain.ProductID              `json:"product"`
	Source   string                        `json:"source,omitempty"` // id of the versions source that answered
	Versions []domain.Version              `json:"versions"`         // ascending, stable only unless the definition includes prereleases
	Refs     map[string]sources.ReleaseRef `json:"refs"`             // keyed by semver
	Sources  []domain.SourceStatus         `json:"sources"`
	Evidence []domain.Evidence             `json:"evidence,omitempty"`
}

// ListVersions queries the product's versions sources (in priority order,
// falling back when one is unavailable), parses tags with the definition's
// version parser and returns the sorted list. When no source yields a release
// the error wraps ErrNoVersions and the (empty) list still carries the source
// statuses.
func (i *Ingester) ListVersions(ctx context.Context, def *catalog.ProductDefinition) (*VersionList, error) {
	return i.listVersions(ctx, def)
}

// IngestRelease retrieves and normalises everything the definition declares
// for release v. known is the full version list (used for template context
// such as PrevLine). Missing or unreachable sources do not fail the call; they
// are reported in Release.Sources:
//
//   - one status per per-release source (SourceID = source id); members of a
//     satisfied fallback group are "skipped";
//   - one status per artifact channel consulted (SourceID = artifact id, Kind
//     = channel locator kind), for index lookups and probes;
//   - one status per artifact content (SourceID = "<artifact>/<content kind>").
//
// The call fails only for invalid input (nil definition, unparsable version)
// or a cancelled context.
func (i *Ingester) IngestRelease(ctx context.Context, def *catalog.ProductDefinition, v domain.Version, known *VersionList) (*domain.Release, error) {
	return i.ingestRelease(ctx, def, v, known)
}

// Advisories lists security advisories from the definition's security sources.
func (i *Ingester) Advisories(ctx context.Context, def *catalog.ProductDefinition) ([]domain.Advisory, []domain.Evidence, []domain.SourceStatus, error) {
	return i.advisories(ctx, def)
}

// RelationshipCheck is one cell of the historical validation matrix: whether
// a source or artifact relationship declared by the definition held for one
// release.
type RelationshipCheck struct {
	Subject     string              `json:"subject"`           // source id, artifact id or "<artifact>/<content kind>"
	SubjectKind string              `json:"subjectKind"`       // "source" | "artifact" | "content"
	Channel     string              `json:"channel,omitempty"` // locator kind used
	Release     string              `json:"release"`
	Outcome     string              `json:"outcome"` // "pass", "fail", "unverifiable", "not-applicable"
	Coordinate  string              `json:"coordinate,omitempty"`
	Detail      string              `json:"detail,omitempty"`
	Evidence    []domain.EvidenceID `json:"evidence,omitempty"`
}

// Relationship check outcomes.
const (
	OutcomePass          = "pass"
	OutcomeFail          = "fail"
	OutcomeUnverifiable  = "unverifiable"
	OutcomeNotApplicable = "not-applicable"
	// OutcomeCovered: a fallback-group member that did not answer while
	// another member of its group did.
	OutcomeCovered = "covered"
)

// Relationship check subject kinds.
const (
	SubjectSource   = "source"
	SubjectArtifact = "artifact"
	SubjectContent  = "content" // "<artifact>/<content kind>"
)

// Relationship summary verdicts.
const (
	VerdictValidated    = "validated"
	VerdictFailing      = "failing"
	VerdictInsufficient = "insufficient"
)

// RelationshipSummary aggregates checks per subject.
type RelationshipSummary struct {
	Subject     string `json:"subject"`
	SubjectKind string `json:"subjectKind"`
	// Passed, Failed and Unverifiable count releases (a release with several
	// instances of one artifact counts once: fail > pass > unverifiable).
	Passed       int `json:"passed"`
	Failed       int `json:"failed"`
	Unverifiable int `json:"unverifiable"`
	// Verdict: "validated" (>= MinValidations passes and no failures),
	// "failing" (any failure), "insufficient" (fewer passes than required).
	Verdict string `json:"verdict"`
}

// MinValidations is how many historical releases must confirm a relationship.
const MinValidations = 3

// RelationshipReport is the historical validation result for a definition.
type RelationshipReport struct {
	Product domain.ProductID `json:"product"`
	// DefinitionDigest identifies the definition revision the report was
	// produced from (DefinitionDigest()). A saved report whose digest differs
	// from the current definition is a stale drift baseline; reports saved
	// before this field existed leave it empty ("unrecorded").
	DefinitionDigest string                `json:"definitionDigest,omitempty"`
	Releases         []string              `json:"releases"`
	Checks           []RelationshipCheck   `json:"checks"`
	Summary          []RelationshipSummary `json:"summary"`
	Evidence         []domain.Evidence     `json:"evidence,omitempty"`
}

// CheckRelationships verifies, for each given release, that every declared
// source resolves and every artifact can be located through its channels (or
// cross-referenced), producing a validation matrix. Unlike IngestRelease it
// consults every member of a fallback group, so that alternatives are
// validated too.
func (i *Ingester) CheckRelationships(ctx context.Context, def *catalog.ProductDefinition, releases []domain.Version, known *VersionList) (*RelationshipReport, error) {
	rep, _, err := i.checkRelationships(ctx, def, releases, known)
	return rep, err
}

// IngestChecked is CheckRelationships plus the ingested releases of the same
// exhaustive run (in the same order as Report.Releases). Callers that need
// both the validation matrix and the per-release payloads — snapshots, facts,
// evidence — (for example drift) avoid ingesting every release twice.
func (i *Ingester) IngestChecked(ctx context.Context, def *catalog.ProductDefinition, releases []domain.Version, known *VersionList) (*RelationshipReport, []*domain.Release, error) {
	return i.checkRelationships(ctx, def, releases, known)
}
