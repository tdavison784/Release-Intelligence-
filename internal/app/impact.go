package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
)

// ErrNoEnvironmentInput marks `ri impact` invocations without any environment
// input; the join is only meaningful against an environment.
var ErrNoEnvironmentInput = errors.New("at least one environment input is required (--repo, --kubernetes, --values, --manifests, --crds or --images)")

// ImpactOptions tunes Impact.
type ImpactOptions struct {
	// Policy overrides the upgrade path policy ("" = the definition's lineage).
	Policy string
	// Environment names the local inputs to parse (see env.Inputs). At least
	// one must be set.
	Environment env.Inputs
}

// Impact builds the UpgradeEdge for product from → to and joins it with the
// locally parsed environment: which changes matter to THAT environment, each
// explained by two provenance chains. Deterministic end to end; no LLM.
func (a *App) Impact(ctx context.Context, productID, from, to string, opts ImpactOptions) (*domain.ImpactReport, error) {
	rep, _, _, err := a.ImpactParts(ctx, productID, from, to, opts)
	return rep, err
}

// ImpactParts is Impact with the inputs the optional AI enrichment step
// (`EnrichImpact`) needs: the report, the edge it was built from and the
// parsed environment. The report alone is identical to Impact's.
func (a *App) ImpactParts(ctx context.Context, productID, from, to string, opts ImpactOptions) (*domain.ImpactReport, *domain.UpgradeEdge, *env.Environment, error) {
	if opts.Environment.Empty() {
		return nil, nil, nil, ErrNoEnvironmentInput
	}
	edge, err := a.Upgrade(ctx, productID, from, to, UpgradeOptions{Policy: opts.Policy})
	if err != nil {
		return nil, nil, nil, err
	}
	e, err := env.Load(opts.Environment)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("environment: %w", err)
	}
	rep, err := impact.Build(impact.Input{Edge: edge, Env: e, Now: a.now().UTC()})
	if err != nil {
		return nil, nil, nil, err
	}
	return rep, edge, e, nil
}
