package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/enrich"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// EnrichOptions configures Enrich.
type EnrichOptions struct {
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
	// e.g. an Anthropic-compatible gateway such as Z.AI's. Server-side
	// fallback is disabled for non-default endpoints: gateways reject the
	// beta header.
	BaseURL string
	// Thinking is sent as the thinking parameter ("enabled"/"disabled"; ""
	// omits it). ANTHROPIC_THINKING=disabled keeps reasoning-first gateway
	// models from spending the answer budget on thinking blocks.
	Thinking string
	// Client overrides the backend (tests). The response cache still wraps it.
	Client llm.Client
	// MaxGroups bounds the prompts per edge (0 = enrich's default).
	MaxGroups int
}

// LLMCacheDir is where model answers are cached (keyed by prompt digest), so
// enrichment replays offline byte for byte. cfg.LLMCacheDir overrides the
// default (StateDir/llm-cache) — e.g. `ri eval -enriched` replaying a
// committed fixture cache instead of the user's state.
func (a *App) LLMCacheDir() string {
	if a.cfg.LLMCacheDir != "" {
		return a.cfg.LLMCacheDir
	}
	return filepath.Join(a.cfg.StateDir, "llm-cache")
}

// EnrichmentBackend describes the model backend Enrich will use.
func (a *App) EnrichmentBackend(opts EnrichOptions) string {
	_, desc := a.enrichmentClient(opts)
	return desc
}

func (a *App) enrichmentClient(opts EnrichOptions) (llm.Client, string) {
	var inner llm.Client
	desc := ""
	switch {
	case opts.Client != nil:
		inner, desc = opts.Client, "injected client"
	case opts.ExchangeDir != "":
		inner, desc = &llm.Exchange{Dir: opts.ExchangeDir}, "file exchange "+opts.ExchangeDir
	case a.cfg.Offline:
		desc = "offline: cached answers only"
	case opts.APIKey != "":
		c := llm.NewAnthropic(opts.APIKey)
		if opts.Model != "" {
			c.Model = opts.Model
		}
		if opts.BaseURL != "" {
			c.BaseURL = opts.BaseURL
			c.ServerFallback = false
		}
		c.Thinking = opts.Thinking
		inner, desc = c, "Anthropic Messages API"
	default:
		desc = "no model configured: cached answers only"
	}
	cache := &llm.Cache{Dir: a.LLMCacheDir(), Inner: inner, Refresh: a.cfg.Refresh && inner != nil, Clock: a.now}
	return cache, desc + " (cache " + cache.Dir + ")"
}

// Enrich runs the enrichment pipeline over an edge and attaches the result
// (Enrichments and EnrichmentRun). The deterministic content of the edge is
// not changed. The enriched edge is stored in place of the plain one.
func (a *App) Enrich(ctx context.Context, edge *domain.UpgradeEdge, opts EnrichOptions) (*enrich.Result, error) {
	client, desc := a.enrichmentClient(opts)
	a.cfg.Logf("enrich: %s", desc)
	res, err := enrich.Run(ctx, edge, enrich.Options{Client: client, Model: opts.Model, MaxGroups: opts.MaxGroups, Clock: a.now})
	if err != nil {
		return nil, err
	}
	for _, r := range res.Requests {
		a.cfg.Logf("enrich: %s %s %s %s", r.Group, r.Status, r.Origin, r.Detail)
	}
	if err := enrich.Apply(edge, res); err != nil {
		return nil, fmt.Errorf("enrich: %w", err)
	}
	if err := a.Store.SaveEdge(edge); err != nil {
		a.cfg.Logf("warning: could not store edge: %v", err)
	}
	return res, nil
}
