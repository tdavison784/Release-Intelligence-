package main

import (
	"context"
	"fmt"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/eval"
)

// appPipeline adapts the composition root to the evaluator's Pipeline port,
// keeping internal/eval independent of internal/app. EnrichedImpact
// implements the evaluator's EnrichingPipeline: the same report, with the AI
// layer's suggestions attached (replayed from the answer cache; a prompt
// without a cached answer stays pending — it is never guessed).
type appPipeline struct {
	a     *app.App
	model string
}

func (p appPipeline) Upgrade(ctx context.Context, product, from, to string) (*domain.UpgradeEdge, error) {
	return p.a.Upgrade(ctx, product, from, to, app.UpgradeOptions{})
}

func (p appPipeline) Impact(ctx context.Context, product, from, to string, inputs env.Inputs) (*domain.ImpactReport, error) {
	return p.a.Impact(ctx, product, from, to, app.ImpactOptions{Environment: inputs})
}

func (p appPipeline) EnrichedImpact(ctx context.Context, product, from, to string, inputs env.Inputs) (*domain.ImpactReport, error) {
	rep, edge, e, err := p.a.ImpactParts(ctx, product, from, to, app.ImpactOptions{Environment: inputs})
	if err != nil {
		return nil, err
	}
	if _, err := p.a.EnrichImpact(ctx, rep, edge, e, app.ImpactEnrichOptions{Model: p.model}); err != nil {
		return nil, err
	}
	return rep, nil
}

// eval runs `ri eval [entries...] [-o text|json] [-update] [-enriched]`:
// the validation dataset (eval/cases, format in eval/FORMAT.md) scored
// against the real pipeline, with regression detection against eval/results,
// hard gates (eval/gates.yaml) and — opt-in — AI-suggestion scoring.
func (c *cli) eval(args []string) error {
	fs := c.flags("eval", "[entries...]  # case ids under eval/cases")
	output := fs.String("o", "text", "output format: text|json")
	update := fs.Bool("update", false, "rewrite the stored results under eval/results after review (never the expectations)")
	dataset := fs.String("dir", envOr("RI_EVAL", "eval"), "dataset root (holds cases/, results/, adjudications/, gates.yaml)")
	enriched := fs.Bool("enriched", false, "score the AI layer's suggestions against the fixtures (replays cached answers; offline-safe)")
	llmCache := fs.String("llm-cache", "internal/app/testdata/impact-llm-cache", "enrichment answer cache for -enriched (the committed replay fixtures)")
	enrichedState := fs.String("enriched-state", "internal/app/testdata/e2e/state", "recorded state dir for -enriched (the fixtures were recorded from it; keeps prompt digests stable)")
	enrichedEnvDir := fs.String("enriched-env", "internal/app/testdata/e2e/env/cert-manager", "environment inputs for -enriched, spelled exactly as the recording harness passed them (paths are part of the prompt digest)")
	enrichedKubernetes := fs.String("enriched-kubernetes", "1.28", "cluster version for -enriched-env (the recorded fixture's)")
	model := fs.String("model", "glm-5.3-flash", "model requested for -enriched (part of the prompt digest; the fixtures were recorded from this one)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	if *enriched {
		// The committed fixtures were recorded with a fixed state, a fixed
		// clock and the recorded fixture environment (the prompt digest
		// covers the environment inputs and their path spelling); rebuild the
		// app offline against exactly those, or every prompt misses the
		// cache and the suggestion scoring silently degrades to zero.
		recordingClock := func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) }
		logf := func(string, ...any) {}
		if c.g.verbose {
			logf = func(f string, a ...any) { fmt.Fprintf(c.err, "· "+f+"\n", a...) }
		}
		a2, err := app.New(app.Config{
			ProductsDir: c.g.products, StateDir: *enrichedState, Offline: true,
			LLMCacheDir: *llmCache, GitHubToken: app.GitHubTokenFromEnv(),
			Now: recordingClock, Logf: logf,
		})
		if err != nil {
			return err
		}
		a = a2
	}
	adj, err := eval.LoadAdjudications(*dataset)
	if err != nil {
		return err
	}
	gates, err := eval.LoadGates(*dataset)
	if err != nil {
		return err
	}
	var enrichedEnv *env.Inputs
	if *enriched {
		envInputs, enverr := eval.DirInputs(*enrichedEnvDir, *enrichedKubernetes)
		if enverr != nil {
			return enverr
		}
		enrichedEnv = &envInputs
	}
	r := &eval.Runner{Pipeline: appPipeline{a, *model}, CasesDir: *dataset, Adjudications: adj, Enriched: *enriched, EnrichedEnv: enrichedEnv}
	var cases []*eval.Case
	if len(pos) == 0 {
		cases, err = r.LoadAll()
	} else {
		cases, err = r.Load(pos)
	}
	if err != nil {
		return err
	}
	results := r.Run(c.ctx, cases)
	rep := eval.Report{Results: results, Aggregate: eval.AggregateResults(results)}
	rep.Gates = eval.EvaluateGates(gates, rep.Aggregate)

	// -update rewrites the stored snapshot of every (selected) entry after
	// human review. Expectations are never touched.
	if *update {
		for i := range results {
			if err := eval.WriteStored(*dataset, eval.StoredFromResult(results[i])); err != nil {
				return err
			}
		}
		fmt.Fprintf(c.err, "updated stored results for %d entries under %s/%s\n", len(results), *dataset, eval.ResultsDirName)
	}

	// Compare against stored snapshots (skip when updating).
	if !*update {
		for i := range results {
			stored, err := eval.LoadStored(*dataset, results[i].CaseID)
			if err != nil {
				return err
			}
			if stored == nil {
				continue
			}
			rep.Compared = true
			rep.Diffs = append(rep.Diffs, eval.Diff(*stored, results[i])...)
		}
	}

	if *output == "json" {
		b, err := rep.JSON()
		if err != nil {
			return err
		}
		_, werr := c.out.Write(b)
		if werr != nil {
			return werr
		}
	} else if err := eval.RenderText(c.out, rep); err != nil {
		return err
	}

	if eval.HasRegression(rep.Diffs) {
		// Non-zero exit on regression, so CI fails loudly. The diff above
		// says what changed; accept it with `ri eval -update`.
		return fmt.Errorf("%d regression(s) against %s/%s (review, then re-run with -update)",
			countRegressions(rep.Diffs), *dataset, eval.ResultsDirName)
	}
	if rep.HasGateFailure() {
		// Non-zero exit on a failed hard gate. The thresholds are
		// pre-registered (eval/gates.yaml); a failure is a finding about the
		// pipeline, reported honestly — not something to tune against here.
		return fmt.Errorf("%d hard gate(s) failed (eval/gates.yaml) — see the gate panel above", countGateFailures(rep.Gates))
	}
	return nil
}

func countRegressions(deltas []eval.Delta) int {
	n := 0
	for _, d := range deltas {
		if d.Regression {
			n++
		}
	}
	return n
}

func countGateFailures(gates []eval.GateResult) int {
	n := 0
	for _, g := range gates {
		if !g.Pass {
			n++
		}
	}
	return n
}
