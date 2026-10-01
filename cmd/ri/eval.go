package main

import (
	"context"
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/eval"
)

// appPipeline adapts the composition root to the evaluator's Pipeline port,
// keeping internal/eval independent of internal/app.
type appPipeline struct{ a *app.App }

func (p appPipeline) Upgrade(ctx context.Context, product, from, to string) (*domain.UpgradeEdge, error) {
	return p.a.Upgrade(ctx, product, from, to, app.UpgradeOptions{})
}

func (p appPipeline) Impact(ctx context.Context, product, from, to string, inputs env.Inputs) (*domain.ImpactReport, error) {
	return p.a.Impact(ctx, product, from, to, app.ImpactOptions{Environment: inputs})
}

// eval runs `ri eval [entries...] [-o text|json] [-update]`: the validation
// dataset (eval/cases, format in eval/FORMAT.md) scored against the real
// pipeline, with regression detection against eval/results.
func (c *cli) eval(args []string) error {
	fs := c.flags("eval", "[entries...]  # case ids under eval/cases")
	output := fs.String("o", "text", "output format: text|json")
	update := fs.Bool("update", false, "rewrite the stored results under eval/results after review (never the expectations)")
	dataset := fs.String("dir", envOr("RI_EVAL", "eval"), "dataset root (holds cases/ and results/)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	r := &eval.Runner{Pipeline: appPipeline{a}, CasesDir: *dataset}
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
