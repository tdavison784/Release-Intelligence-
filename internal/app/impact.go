package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/render"
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
	// Facts is verified knowledge to evaluate against the environment
	// (LoadKnowledge); MinVerification keeps facts at or above the level
	// ("" = human). Without facts the report is the knowledge-free join.
	Facts           []domain.VerifiedFact
	MinVerification domain.VerificationLevel
	// Render, when set, renders the From and To releases with the
	// environment's configuration before the join (`ri impact --render`):
	// rendered-change conditions of facts are decided against those renders
	// (environment pairs only, never the chart-default pair), and the
	// rendered delta is returned beside the report (ImpactRun.Render).
	Render *RenderOptions
}

// ImpactRun is everything one `ri impact` run produced.
type ImpactRun struct {
	Report *domain.ImpactReport
	Edge   *domain.UpgradeEdge
	Env    *env.Environment
	Render *RenderDiffResult // nil without opts.Render
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
	r, err := a.ImpactRun(ctx, productID, from, to, opts)
	if err != nil {
		return nil, nil, nil, err
	}
	return r.Report, r.Edge, r.Env, nil
}

// ImpactRun is ImpactParts plus the rendered delta (opts.Render).
func (a *App) ImpactRun(ctx context.Context, productID, from, to string, opts ImpactOptions) (*ImpactRun, error) {
	if opts.Environment.Empty() {
		return nil, ErrNoEnvironmentInput
	}
	edge, err := a.Upgrade(ctx, productID, from, to, UpgradeOptions{Policy: opts.Policy})
	if err != nil {
		return nil, err
	}
	if opts.Environment.ProductHints == nil {
		opts.Environment.ProductHints = env.HintsFromCatalog(a.Catalog)
	}
	e, err := env.Load(opts.Environment)
	if err != nil {
		return nil, fmt.Errorf("environment: %w", err)
	}
	in := impact.Input{Edge: edge, Env: e, Now: a.now().UTC(), Facts: opts.Facts, MinVerification: opts.MinVerification}
	run := &ImpactRun{Edge: edge, Env: e}
	if opts.Render != nil {
		ro := *opts.Render
		ro.Env = e
		if run.Render, err = a.RenderDiffEdge(ctx, edge, ro); err != nil {
			return nil, err
		}
		in.Render = render.ConditionEvaluator{Pairs: run.Render.EnvironmentPairs()}
		// PO-3: changed defaults / new keys the customer leaves unset are
		// decided by the counterfactual render
		in.Unset = &render.UnsetValues{Engine: a.RenderEngine(), Product: productID,
			From: edge.From.String(), To: edge.To.String(), Pairs: run.Render.EnvironmentPairs(),
			KubeVersion: ro.KubeVersion, APIVersions: ro.APIVersions, Ctx: ctx}
		// PO-7a: values keys the customer sets whose key the target removes
		// are decided by their render (refusal / attribution / no effect)
		in.Set = &render.SetValues{Engine: a.RenderEngine(), Product: productID,
			From: edge.From.String(), To: edge.To.String(), Pairs: run.Render.EnvironmentPairs(),
			KubeVersion: ro.KubeVersion, APIVersions: ro.APIVersions, Ctx: ctx}
	}
	if run.Report, err = impact.Build(in); err != nil {
		return nil, err
	}
	return run, nil
}
