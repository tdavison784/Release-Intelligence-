// Package impact joins an UpgradeEdge with an environment (package env) and
// answers "which of these changes matter to THIS environment?". The join is
// pure and deterministic: no cluster access, no network, no LLM. Every
// finding cites two provenance chains — the upstream evidence of the change
// (copied from the edge) and the environment evidence of the local fact that
// matched — so no conclusion is unexplained. Rendering of reports for humans
// also lives here.
//
// CONTRACT NOTE: the exported API in this file is used by the CLI /
// orchestration layer.
package impact

import (
	"errors"
	"io"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// ErrNotImplemented is returned by contract stubs not yet implemented.
var ErrNotImplemented = errors.New("impact: not implemented")

// Producer identifier recorded in the provenance of findings.
const Producer = "impact@v1"

// Join rules. A rule names why a finding fired; the upstream diff rule that
// produced the joined change is visible through the finding's ChangeID.
const (
	// The customer's values file sets a key (or a key under/over it) that the
	// target release removed.
	RuleValuesRemoved = "impact:values-removed"
	// The customer pins exactly a key whose default changed: their value
	// keeps winning, nothing to do.
	RuleValuesPinned = "impact:values-pinned"
	// The customer sets a key adjacent to (ancestor/descendant of) one whose
	// default changed; merge semantics need a look.
	RuleValuesAdjacent = "impact:values-adjacent"
	// The customer sets a key that did not exist in From and is new in To:
	// it starts taking effect.
	RuleValuesNewKey = "impact:values-new-key"
	// The environment uses a CRD (installed or via manifests) that the
	// target release no longer ships.
	RuleCRDRemoved = "impact:crd-removed"
	// The environment uses an API version (manifest apiVersion or installed
	// CRD version) that the target removes or no longer serves.
	RuleCRDVersionRemoved = "impact:crd-version-removed"
	// The environment sets a field path that the target's CRD schema prunes.
	RuleCRDFieldRemoved = "impact:crd-field-removed"
	// The environment uses an API version the target deprecates.
	RuleCRDVersionDeprecated = "impact:crd-version-deprecated"
	// Cluster version checks against the target's Kubernetes constraints.
	RuleKubernetesInRange  = "impact:kubernetes-in-range"
	RuleKubernetesBelow    = "impact:kubernetes-below-range"
	RuleKubernetesAbove    = "impact:kubernetes-above-range"
	RuleKubernetesUntested = "impact:kubernetes-untested"
	RuleKubeVersionBlocked = "impact:kubeversion-blocked"
	// An image the environment references changed between the endpoints.
	RuleImageChanged = "impact:image-changed"
)

// Input is everything Build needs.
type Input struct {
	Edge *domain.UpgradeEdge
	Env  *env.Environment
	Now  time.Time
}

// Build assembles the ImpactReport. The result must pass
// domain.ImpactReport.Validate.
func Build(in Input) (*domain.ImpactReport, error) {
	return build(in)
}

// RenderOptions tunes human output.
type RenderOptions struct {
	// Color enables ANSI colours.
	Color bool
}

// RenderText writes a human-readable report: the funnel summary, the
// environment, then per-finding why-blocks citing both evidence chains.
func RenderText(w io.Writer, r *domain.ImpactReport, opts RenderOptions) error {
	return renderText(w, r, opts)
}
