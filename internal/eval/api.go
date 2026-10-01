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
