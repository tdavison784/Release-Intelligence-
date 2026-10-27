// Package upgrade computes UpgradeEdges. It is pure: given already-ingested
// releases (domain.Release), the product definition and advisories, it
// selects the release path, aggregates release-note items, diffs structured
// snapshots (Helm values, CRDs, images), compares compatibility constraints
// and assembles an evidence-backed domain.UpgradeEdge. Rendering of edges for
// humans also lives here.
//
// CONTRACT NOTE: the exported API in this file is used by the CLI /
// orchestration layer.
package upgrade

import (
	"errors"
	"io"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ErrNotImplemented is returned by contract stubs not yet implemented.
var ErrNotImplemented = errors.New("upgrade: not implemented")

// Producer identifier recorded in provenance of computed changes.
const Producer = "upgrade@v1"

// Path policies.
const (
	// PolicyMinorLineage: for products whose minor lines branch, traverse the
	// X.Y.0 release of every line after From's line up to To's line, plus the
	// patch releases of To's line up to To (or, when From and To share a line,
	// the patches in (From, To]). Backport patches of intermediate lines are
	// reported as skipped.
	PolicyMinorLineage = "minor-lineage"
	// PolicyAll: every release in (From, To] in semver order.
	PolicyAll = "all"
)

// PathSelection is the result of SelectPath.
type PathSelection struct {
	Policy  string           `json:"policy"`
	Path    []domain.Version `json:"path"`    // ascending, excludes From, includes To
	Skipped []domain.Version `json:"skipped"` // releases in (From, To) not traversed
}

// SelectPath chooses the releases traversed when upgrading from → to, given
// all known stable versions and the product lineage ("minor" → PolicyMinorLineage,
// otherwise PolicyAll). from and to must be present in versions and from < to.
func SelectPath(versions []domain.Version, from, to domain.Version, lineage string) (*PathSelection, error) {
	return selectPath(versions, from, to, lineage)
}

// Input is everything Build needs.
type Input struct {
	Definition *catalog.ProductDefinition
	From       *domain.Release   // ingested source release
	To         *domain.Release   // ingested target release
	Path       []*domain.Release // ingested releases of PathSelection.Path (ascending; last == To)
	Selection  *PathSelection
	Advisories []domain.Advisory
	// AdvisoryEvidence holds the evidence records referenced by Advisories.
	AdvisoryEvidence []domain.Evidence
	// ExtraSources are product-level source statuses (e.g. versions listing, advisories).
	ExtraSources []domain.SourceStatus
	// Versions are every known stable release of the product (the version
	// listing), from which the security section's candidate versions are
	// chosen. Empty: no candidates (a gap when the target is affected).
	Versions []domain.Version
	// Charts maps a candidate's semver to the chart versions that ship it
	// (CandidateVersions lists which versions to look up).
	Charts map[string]ChartLookup
	Now    time.Time
}

// ChartLookup is the result of looking up which chart versions ship one
// release: the charts found, with their evidence, and why a lookup found
// nothing (an unread index, no chart with that appVersion).
type ChartLookup struct {
	Charts []ChartVersion
	Gaps   []string
}

// ChartVersion is one chart version shipping a release.
type ChartVersion struct {
	Artifact string // definition artifact id
	Name     string // chart name
	Version  string
	Evidence []domain.Evidence
}

// CandidateVersions returns the candidate versions Build would put in the
// edge's security section for in (ignoring in.Charts), so that a caller can
// look up their chart versions before building. It is pure.
func CandidateVersions(in Input) []domain.Version {
	in.Charts = nil
	e, err := build(in)
	if err != nil || e.Security == nil {
		return nil
	}
	var out []domain.Version
	seen := map[string]bool{}
	for _, c := range e.Security.Candidates {
		if c.Version != nil && !seen[c.Version.Semver] {
			seen[c.Version.Semver] = true
			out = append(out, *c.Version)
		}
	}
	return out
}

// Build assembles the UpgradeEdge. The result must pass domain.UpgradeEdge.Validate.
func Build(in Input) (*domain.UpgradeEdge, error) {
	return build(in)
}

// RenderText writes a human-readable report of the edge (the `ri upgrade`
// default output), grouping changes by section and citing evidence ids.
func RenderText(w io.Writer, e *domain.UpgradeEdge, opts RenderOptions) error {
	return renderText(w, e, opts)
}

// RenderOptions tunes human output.
type RenderOptions struct {
	// Verbose includes features/bugfixes and full evidence excerpts.
	Verbose bool
	// MaxPerSection truncates long sections (0 = default 25; <0 = unlimited).
	MaxPerSection int
	// Color enables ANSI colours.
	Color bool
}

// PlatformVersionCheck is the result of evaluating one platform version
// against a compatibility constraint; it is how the environment join asks
// "is this cluster inside what the release supports?".
type PlatformVersionCheck struct {
	// Computable is false when the constraint can neither be read as an
	// explicit version list nor parsed as a semver range.
	Computable bool
	// Admits reports whether the version satisfies the constraint. A version
	// given as a line ("1.31") is admitted when any patch of the line is.
	// A "minimum" kind admits the bound line and everything above it
	// ("minimum: 1.30" means ">= 1.30"); a "maximum" kind the bound line and
	// everything below.
	Admits bool
	// Display is the human form of the constraint's range ("1.29–1.33",
	// "≥ 1.22"), or its raw text when not computable.
	Display string
	// Below/Above name the nearest admitted line when the version falls
	// outside a closed range ("1.30" when the range starts there and the
	// cluster is older). Empty when the constraint is open-ended on that
	// side or the position could not be determined.
	Below, Above string
}

// EvaluatePlatformConstraint checks a single platform version (e.g. "1.31" or
// "1.31.5") against one constraint, using the same range semantics as the
// endpoint diff (compat.go).
func EvaluatePlatformConstraint(c *domain.CompatibilityConstraint, version string) PlatformVersionCheck {
	return evaluatePlatformConstraint(c, version)
}
