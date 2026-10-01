package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/enrich"
)

// upgradeEnrichFlags are the AI enrichment flags of `ri upgrade`.
type upgradeEnrichFlags struct {
	enrich     *bool
	exchange   *string
	model      *string
	maxGroups  *int
	candidates *bool
}

func enrichFlags(fs *flag.FlagSet) upgradeEnrichFlags {
	return upgradeEnrichFlags{
		enrich: fs.Bool("enrich", false, "add AI enrichments (clusters, migration summaries, diff explanations, related changes) with provenance; "+
			"needs ANTHROPIC_API_KEY or -llm-exchange, or replays cached answers (-offline)"),
		exchange:   fs.String("llm-exchange", "", "answer enrichment prompts through files in `DIR` (<digest>.request.json → <digest>.response.json) instead of the API"),
		model:      fs.String("model", "", "model to request for enrichment (part of the prompt digest; default: the backend's)"),
		maxGroups:  fs.Int("enrich-max", 40, "maximum enrichment prompts per edge"),
		candidates: fs.Bool("enrich-candidates", false, "print the deterministic candidate groups to stderr (debug; they are not conclusions)"),
	}
}

// check rejects enrichment options given without -enrich.
func (f upgradeEnrichFlags) check() error {
	if !*f.enrich && (*f.exchange != "" || *f.model != "") {
		return fmt.Errorf("%w: -llm-exchange and -model require -enrich", app.ErrUsage)
	}
	return nil
}

// enrich runs the optional enrichment step of `ri upgrade`. Without -enrich
// the edge stays exactly as the deterministic pipeline built it.
func (c *cli) enrich(a *app.App, edge *domain.UpgradeEdge, f upgradeEnrichFlags) error {
	if *f.candidates {
		printCandidates(c.err, edge, enrich.Candidates(edge, enrich.CandidateOptions{}))
	}
	if !*f.enrich {
		return nil
	}
	opts := app.EnrichOptions{Model: *f.model, ExchangeDir: *f.exchange, APIKey: os.Getenv("ANTHROPIC_API_KEY"), BaseURL: os.Getenv("ANTHROPIC_BASE_URL"), Thinking: os.Getenv("ANTHROPIC_THINKING"), MaxGroups: *f.maxGroups}
	res, err := a.Enrich(c.ctx, edge, opts)
	if err != nil {
		return err
	}
	run := res.Run
	fmt.Fprintf(c.err, "enrich: %s; %d candidate groups, %d prompts, %d enrichments accepted, %d rejected, %d pending, %d failed\n",
		a.EnrichmentBackend(opts), run.CandidateGroups, run.Requests, run.Accepted, len(run.Rejected), run.Pending, run.Failed)
	if run.Pending > 0 {
		if *f.exchange != "" {
			fmt.Fprintf(c.err, "enrich: %d prompts are waiting in %s; write a <digest>.response.json next to each request (docs/ENRICHMENT.md) and run again\n", run.Pending, *f.exchange)
		} else {
			fmt.Fprintf(c.err, "enrich: %d prompts have no cached answer; set ANTHROPIC_API_KEY or use -llm-exchange DIR\n", run.Pending)
		}
	}
	return nil
}

func printCandidates(w interface{ Write([]byte) (int, error) }, e *domain.UpgradeEdge, cs []enrich.Candidate) {
	titles := map[string]string{}
	for _, ch := range e.Changes {
		titles[ch.ID] = ch.Title
	}
	fmt.Fprintf(w, "Candidate groups (deterministic, not conclusions): %d\n", len(cs))
	for _, cand := range cs {
		hints := make([]string, len(cand.Hints))
		for i, h := range cand.Hints {
			hints[i] = string(h)
		}
		fmt.Fprintf(w, "  %s  score %.2f  hints %s\n", cand.ID, cand.Score, strings.Join(hints, ","))
		for _, id := range cand.Changes {
			t := strings.Join(strings.Fields(titles[id]), " ")
			if r := []rune(t); len(r) > 110 {
				t = string(r[:110]) + "…"
			}
			fmt.Fprintf(w, "    %s  %s\n", id, t)
		}
		for _, s := range cand.Signals {
			fmt.Fprintf(w, "      · %s\n", s)
		}
	}
}
