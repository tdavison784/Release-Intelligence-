package app

import (
	"context"
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impactenrich"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// ImpactEnrichOptions configures EnrichImpact. It mirrors EnrichOptions (the
// upgrade-edge enricher) and shares its backend selection and response cache
// directory, so a recorded answer replays offline regardless of which step
// asked for it.
type ImpactEnrichOptions struct {
	// Model is the requested model ("" = the backend's default). It is part
	// of every prompt digest, so an offline replay must request the same one.
	Model string
	// ExchangeDir answers prompts through request/response files in this
	// directory instead of an API (batch or offline operation).
	ExchangeDir string
	// APIKey enables the Anthropic Messages API (not used offline or with
	// ExchangeDir).
	APIKey string
	// BaseURL overrides the Anthropic API endpoint (ANTHROPIC_BASE_URL),
	// e.g. an Anthropic-compatible gateway such as Z.AI's.
	BaseURL string
	// Thinking is sent as the thinking parameter ("enabled"/"disabled"; ""
	// omits it). ANTHROPIC_THINKING=disabled keeps reasoning-first gateway
	// models from spending the answer budget on thinking blocks.
	Thinking string
	// Client overrides the backend (tests). The response cache still wraps it.
	Client llm.Client
	// Max bounds the prompts per report (0 = impactenrich's default, 30).
	Max int
}

// EnrichmentBackend describes the model backend EnrichImpact will use; the
// same backend Enrich (the edge enricher) uses.
func (a *App) EnrichmentBackendForImpact(opts ImpactEnrichOptions) string {
	return a.EnrichmentBackend(EnrichOptions{
		Model: opts.Model, ExchangeDir: opts.ExchangeDir, APIKey: opts.APIKey,
		BaseURL: opts.BaseURL, Thinking: opts.Thinking, Client: opts.Client, MaxGroups: opts.Max,
	})
}

// EnrichImpact runs the impact enrichment step over a deterministic report
// (the ImpactParts tuple) and attaches the result: enrichments, run metadata
// and at most one review suggestion per unknown finding. The report's
// deterministic content is not changed; the report is NOT stored (the store
// holds releases and edges, not joins).
func (a *App) EnrichImpact(ctx context.Context, rep *domain.ImpactReport, edge *domain.UpgradeEdge, e *env.Environment, opts ImpactEnrichOptions) (*impactenrich.Result, error) {
	client, desc := a.enrichmentClient(EnrichOptions{
		Model: opts.Model, ExchangeDir: opts.ExchangeDir, APIKey: opts.APIKey,
		BaseURL: opts.BaseURL, Thinking: opts.Thinking, Client: opts.Client,
	})
	a.cfg.Logf("impact enrich: %s", desc)
	res, err := impactenrich.Run(ctx, rep, edge, e, impactenrich.Options{Client: client, Model: opts.Model, Max: opts.Max, Clock: a.now})
	if err != nil {
		return nil, err
	}
	for _, r := range res.Requests {
		a.cfg.Logf("impact enrich: %s %s %s %s", r.Group, r.Status, r.Origin, r.Detail)
	}
	if err := impactenrich.Apply(rep, res, e); err != nil {
		return nil, fmt.Errorf("impact enrich: %w", err)
	}
	return res, nil
}
