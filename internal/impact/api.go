// Package impact joins an UpgradeEdge with an environment (package env) and
// answers "which of these changes matter to THIS environment?". The join is
// pure and deterministic: no cluster access, no network, no LLM. Applicability
// is decided first for every analyzed unit (AFFECTED / NOT_AFFECTED /
// UNKNOWN, per docs/ACTION_CLASSIFICATION.md); only an affected unit receives
// an action class. Every verdict is explained: affected findings cite two
// provenance chains — the upstream evidence of the change (copied from the
// edge) and the environment evidence of the local fact that matched — while
// not-affected records carry the evaluation record and unknown records carry
// the missing-evidence list. Rendering of reports for humans also lives here.
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

	// --- verdict rules (docs/ACTION_CLASSIFICATION.md) -----------------------

	// A not-affected verdict for a values change: the deciding dimension
	// (values) was supplied, the changed keys were checked, and the
	// environment does not set any of them.
	RuleValuesUnset = "impact:values-unset"
	// A not-affected verdict for a CRD-removal change: the CRD is not
	// installed in the environment.
	RuleCRDUnused = "impact:crd-unused"
	// A not-affected verdict for a CRD version change: no installed CRD
	// declares the version and no manifest uses it.
	RuleCRDVersionUnused = "impact:crd-version-unused"
	// A not-affected verdict for a removed CRD field: no manifest sets the
	// removed path (or anything below it).
	RuleCRDFieldUnset = "impact:crd-field-unset"
	// A not-affected verdict for an image change: the environment does not
	// reference the image repository.
	RuleImageNotReferenced = "impact:image-not-referenced"
	// A not-affected verdict for a compatibility constraint the supplied
	// cluster version satisfies (minimum admitted, kubeVersion admits, or a
	// tested-range hit). supported+admits produces the informational
	// RuleKubernetesInRange instead.
	RuleCompatSatisfied = "impact:compatibility-satisfied"
	// An unknown verdict: the change has no machine-comparable subject (or
	// its diff rule has no join rule), so the deterministic join cannot
	// evaluate its applicability. neededToDetermine says exactly that.
	RuleNotJoined = "impact:not-joined"
	// An unknown verdict: a deciding environment dimension was not supplied
	// (or the upstream constraint is not machine-readable), so applicability
	// cannot be determined. neededToDetermine names what is missing.
	RuleInsufficientVisibility = "impact:insufficient-visibility"
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
	// ShowNotAffected renders the not-affected section with its evaluation
	// records; the summary always counts them.
	ShowNotAffected bool
}

// RenderText writes a human-readable report: the funnel summary, the
// environment, then per-finding why-blocks citing both evidence chains (and,
// for unknown verdicts, the missing evidence; for not-affected verdicts the
// evaluation record, verbose mode only).
func RenderText(w io.Writer, r *domain.ImpactReport, opts RenderOptions) error {
	return renderText(w, r, opts)
}
