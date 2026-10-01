// Package eval runs the validation dataset (eval/cases, format in
// eval/FORMAT.md) against the REAL pipeline and scores the output against
// hand-curated ground truth. It is deterministic (no LLM, no network of its
// own): the pipeline under test is injected as a Pipeline, so the same
// package scores both live runs (`ri eval`) and offline replays (tests).
//
// The contract of this file is used by the CLI and by tests.
package eval

import (
	"context"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// Pipeline is the real upgrade/impact pipeline, injected so that this package
// never imports the composition root (dependency direction stays
// domain ← … ← app ← cmd; eval sits beside app, not below it).
type Pipeline interface {
	// Upgrade builds the UpgradeEdge for product from → to.
	Upgrade(ctx context.Context, product, from, to string) (*domain.UpgradeEdge, error)
	// Impact builds the ImpactReport for product from → to against inputs.
	// Implementations that only need the edge can return an error that
	// IsNoEnvironment reports false for; the runner calls Impact only for
	// cases with an environment directory anyway.
	Impact(ctx context.Context, product, from, to string, inputs env.Inputs) (*domain.ImpactReport, error)
}

// Kinds of expected items (vocabulary of eval/FORMAT.md).
const (
	KindBreaking      = "breaking"
	KindRemoval       = "removal"
	KindDeprecation   = "deprecation"
	KindBehaviour     = "behaviour-change"
	KindHelmValues    = "helm-values"
	KindCRDSchema     = "crd-schema"
	KindAPI           = "api"
	KindCompatibility = "compatibility"
	KindSecurity      = "security"
	KindArtifact      = "artifact"
	KindMigrationStep = "migration-step"
	KindDependency    = "dependency"
)

// kinds is the closed vocabulary enforced at load time (typos in case.yaml
// must fail loudly, not silently never match).
var kinds = map[string]bool{
	KindBreaking: true, KindRemoval: true, KindDeprecation: true, KindBehaviour: true,
	KindHelmValues: true, KindCRDSchema: true, KindAPI: true, KindCompatibility: true,
	KindSecurity: true, KindArtifact: true, KindMigrationStep: true, KindDependency: true,
}

// Importances of expected items.
const (
	ImportanceCritical  = "critical"
	ImportanceImportant = "important"
	ImportanceMinor     = "minor"
)

var importances = map[string]bool{
	ImportanceCritical: true, ImportanceImportant: true, ImportanceMinor: true,
}

// Relevance values of environment expectedImpact links.
const (
	RelevanceActionRequired = "action-required"
	RelevanceReview         = "review"
	RelevanceInformational  = "informational"
	RelevanceNotAffected    = "not-affected"
)

var relevances = map[string]bool{
	RelevanceActionRequired: true, RelevanceReview: true,
	RelevanceInformational: true, RelevanceNotAffected: true,
}

// Expected classifications (G9): the class a correct system should output for
// an expected item — the five-class contract of docs/ACTION_CLASSIFICATION.md,
// dataset vocabulary. Mirrors domain.ImpactClass; declared locally so the
// evaluator stays below the domain of the join (matching the package's
// dependency direction) and so case.yaml can be validated without a domain
// import in the vocabulary itself.
const (
	ClassActionRequired = "action-required"
	ClassReviewRequired = "review-required"
	ClassInformational  = "informational"
	ClassNotAffected    = "not-affected"
	ClassUnknown        = "unknown"
)

// classes is the closed vocabulary of Expected.Classification and
// NotExpected.Classification. Empty is allowed (the field is optional; legacy
// cases predate it and score on presence only).
var classes = map[string]bool{
	ClassActionRequired: true, ClassReviewRequired: true, ClassInformational: true,
	ClassNotAffected: true, ClassUnknown: true,
}

// classOrder ranks classes by how bad a miss they are (docs/ACTION_CLASSIFICATION.md
// severity weighting): when several findings join one expected item, the
// item's actual class is the strongest one — a miss that denies required
// action is worse than an over-report, and unknown (cannot tell) is worse
// than a checked-and-clear not-affected.
var classOrder = map[string]int{
	ClassActionRequired: 4, ClassReviewRequired: 3, ClassInformational: 2,
	ClassUnknown: 1, ClassNotAffected: 0,
}

// StrongestClass returns the class that ranks higher under the miss-severity
// order (b when equal or either is empty).
func StrongestClass(a, b string) string {
	if b == "" {
		return a
	}
	if a == "" {
		return b
	}
	if classOrder[b] > classOrder[a] {
		return b
	}
	return a
}
