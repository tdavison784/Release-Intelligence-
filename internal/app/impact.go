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
var ErrNoEnvironmentInput = errors.New("at least one environment input is required (--kubernetes, --values, --manifests, --crds or --images)")

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
	if opts.Environment.Empty() {
		return nil, ErrNoEnvironmentInput
	}
	edge, err := a.Upgrade(ctx, productID, from, to, UpgradeOptions{Policy: opts.Policy})
	if err != nil {
		return nil, err
	}
	e, err := env.Load(opts.Environment)
	if err != nil {
		return nil, fmt.Errorf("environment: %w", err)
	}
	rep, err := impact.Build(impact.Input{Edge: edge, Env: e, Now: a.now().UTC()})
	if err != nil {
		return nil, err
	}
	return rep, nil
}
