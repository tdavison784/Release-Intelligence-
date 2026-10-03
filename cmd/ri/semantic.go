package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/llm"
	"github.com/tdavison784/release-intelligence/internal/render"
	"github.com/tdavison784/release-intelligence/internal/semantic"
)

const semanticUsage = `Usage:
  ri semantic candidates <product> <from> <to> [-o text|json]
  ri semantic propose <product> <from> <to> -model M[,M2…] [-tasks full] [-llm-exchange DIR] [-out DIR]

candidates  print the deterministic restatement clusters of the edge (not conclusions)
propose     ask every model every task for every candidate (docs/SEMANTIC.md); proposals and
            candidates are written as knowledge records under -out; failures are recorded
`

// semanticCmd implements `ri semantic`.
func (c *cli) semanticCmd(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(c.err, semanticUsage)
		return fmt.Errorf("%w: missing subcommand", app.ErrUsage)
	}
	switch args[0] {
	case "candidates":
		return c.semanticCandidates(args[1:])
	case "propose":
		return c.semanticPropose(args[1:])
	}
	fmt.Fprint(c.err, semanticUsage)
	return fmt.Errorf("%w: unknown subcommand %q", app.ErrUsage, args[0])
}

func (c *cli) semanticCandidates(args []string) error {
	fs := c.flags("semantic candidates", "<product> <from> <to>")
	output := fs.String("o", "text", "output format: text|json")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 3 {
		fs.Usage()
		return fmt.Errorf("%w: expected <product> <from> <to>", app.ErrUsage)
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	edge, err := a.Upgrade(c.ctx, pos[0], pos[1], pos[2], app.UpgradeOptions{})
	if err != nil {
		return err
	}
	rep := semantic.BuildCandidates(edge, edge.GeneratedAt)
	if *output == "json" {
		return c.writeJSON(rep)
	}
	skips := map[string]int{}
	for _, s := range rep.Skipped {
		skips[s.Reason]++
	}
	fmt.Fprintf(c.out, "%d candidates from %d changes (%d members); skipped %v\n", len(rep.Candidates), len(edge.Changes), rep.Members, skips)
	for _, cand := range rep.Candidates {
		fmt.Fprintf(c.out, "%s  %-26s %d member(s)  %s\n", cand.ID, cand.Grouping, len(cand.Members), trimTitle(cand.Title, 100))
	}
	return nil
}

func (c *cli) semanticPropose(args []string) error {
	fs := c.flags("semantic propose", "<product> <from> <to>")
	models := fs.String("model", "", "comma-separated model ids; one proposer each (required)")
	provider := fs.String("provider", "anthropic", "provider serving the models (anthropic, zai, …)")
	tasks := fs.String("tasks", string(domain.TaskFull), "comma-separated tasks: "+strings.Join(taskNames(), ", "))
	exchange := fs.String("llm-exchange", "", "answer prompts through files in `DIR` (scripts/semantic-exchange.sh answers them)")
	cacheDir := fs.String("llm-cache", "", "answer cache `DIR` (default: <state>/llm-cache)")
	out := fs.String("out", "", "write candidate/proposal records under `DIR` (knowledge/ layout)")
	max := fs.Int("max", 0, "only the first N candidates (0 = all)")
	only := fs.String("only", "", "file of candidate ids to propose, one per line ('#' comments; default all)")
	parallel := fs.Int("parallel", 4, "concurrent requests per model")
	output := fs.String("o", "text", "output format: text|json")
	withRender := fs.Bool("render", false, "add release-level (chart-default) rendered-diff evidence to candidates (prompt version +rendered); use a separate -out store to compare with a run without it")
	kube := fs.String("kubernetes", "", "Kubernetes version for -render (chart kubeVersion gates)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 3 || *models == "" {
		fs.Usage()
		return fmt.Errorf("%w: expected <product> <from> <to> and -model", app.ErrUsage)
	}
	var ts []domain.ProposalTask
	for _, t := range splitList(*tasks) {
		if domain.TaskAspects(domain.ProposalTask(t)) == nil {
			return fmt.Errorf("%w: unknown task %q", app.ErrUsage, t)
		}
		ts = append(ts, domain.ProposalTask(t))
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	edge, err := a.Upgrade(c.ctx, pos[0], pos[1], pos[2], app.UpgradeOptions{})
	if err != nil {
		return err
	}
	toRel, err := c.release(a, pos[0], pos[2])
	if err != nil {
		return err
	}
	var copts semantic.CandidateOptions
	if *withRender {
		er, rerr := render.EdgeRenderedChanges(c.ctx, a.RenderEngine().ReleasePairs(*kube), edge)
		if rerr != nil {
			// never "no change": the run proceeds without render evidence, and says so
			fmt.Fprintf(c.err, "semantic: no rendered evidence for this edge: %v\n", rerr)
		} else {
			copts.RenderedEvidence = er.ForChanges
			fmt.Fprintf(c.err, "semantic: %d release-level rendered change(s) available as evidence\n", len(er.Evidence))
		}
	}
	cands := semantic.BuildCandidatesWith(edge, edge.GeneratedAt, copts).Candidates
	var hints []knowledge.ConfigSourceHint
	if def, derr := a.Product(pos[0]); derr == nil {
		hints = semantic.ConfigSourceHints(def.ConfigSources)
	}
	if *only != "" {
		var missed []string
		if cands, missed, err = filterCandidates(cands, *only); err != nil {
			return err
		}
		if len(missed) > 0 {
			// ids, not titles: the driver script pairs this list with its store
			fmt.Fprintf(c.err, "semantic: -only: %d id(s) not among this edge's candidates: %s\n", len(missed), strings.Join(missed, ", "))
		}
	}
	if *max > 0 && len(cands) > *max {
		cands = cands[:*max]
	}
	if *out != "" {
		var stored int
		if cands, stored, err = semantic.UseStored(*out, cands); err != nil {
			return err
		}
		if stored > 0 {
			fmt.Fprintf(c.err, "semantic: %d candidate(s) already stored under -out; proposing against the stored records\n", stored)
		}
	}

	var inner llm.Client
	backend := "cached answers only"
	switch {
	case *exchange != "":
		inner, backend = &llm.Exchange{Dir: *exchange}, "file exchange "+*exchange
	case c.g.offline:
	case os.Getenv("ANTHROPIC_API_KEY") != "":
		an := llm.NewAnthropic(os.Getenv("ANTHROPIC_API_KEY"))
		if base := os.Getenv("ANTHROPIC_BASE_URL"); base != "" {
			an.BaseURL, an.ServerFallback = base, false
		}
		an.Thinking = os.Getenv("ANTHROPIC_THINKING")
		inner, backend = an, "Messages API"
	}
	client := &llm.Cache{Dir: a.LLMCacheDir(), Inner: inner}
	if *cacheDir != "" {
		client.Dir = *cacheDir
	}
	var ps []knowledge.Proposer
	for _, m := range splitList(*models) {
		ps = append(ps, semantic.NewLLMProposer(client, *provider, m, semantic.ProposerOptions{}))
	}
	fmt.Fprintf(c.err, "semantic: %d candidates × %d task(s) × %d model(s) via %s (cache %s)\n", len(cands), len(ts), len(ps), backend, client.Dir)
	props, fails := semantic.ProposeAllWith(c.ctx, cands, ps, ts, semantic.ProposeOptions{
		Context: func(cand domain.SemanticCandidate) knowledge.ProposalContext {
			pctx := semantic.ArtifactContext(toRel, cand)
			pctx.ConfigSources = hints // the product's upstream-documented config channels (prompt v2)
			return pctx
		},
		Parallel: *parallel,
	})
	if *out != "" {
		if err := semantic.WriteRecords(*out, cands, props, fails); err != nil {
			return err
		}
	}
	rep := semantic.Summarize(len(cands), props, fails)
	if *output == "json" {
		return c.writeJSON(struct {
			Report    semantic.RunReport          `json:"report"`
			Proposals []domain.SemanticProposal   `json:"proposals"`
			Failures  []knowledge.ProposalFailure `json:"failures"`
		}{rep, props, fails})
	}
	rep.WriteText(c.out)
	return nil
}

// release loads one ingested release (the artifact context of prompts).
func (c *cli) release(a *app.App, productID, version string) (*domain.Release, error) {
	def, err := a.Product(productID)
	if err != nil {
		return nil, err
	}
	vl, err := a.Versions(c.ctx, def)
	if err != nil {
		return nil, err
	}
	v, err := app.ResolveVersion(vl, version)
	if err != nil {
		return nil, err
	}
	return a.Release(c.ctx, def, v, vl)
}

func taskNames() []string {
	out := make([]string, len(domain.ProposalTasks))
	for i, t := range domain.ProposalTasks {
		out[i] = string(t)
	}
	return out
}

func trimTitle(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// filterCandidates keeps the candidates whose id is listed in file (one per
// line, '#' comments). Missed lists the wanted ids this edge did not build, in
// file order, so a driver script can notice store/edge drift.
func filterCandidates(cands []domain.SemanticCandidate, file string) (kept []domain.SemanticCandidate, missed []string, err error) {
	ids, err := readIDList(file)
	if err != nil {
		return nil, nil, err
	}
	have := map[string]domain.SemanticCandidate{}
	for _, c := range cands {
		have[string(c.ID)] = c
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if c, ok := have[id]; ok {
			kept = append(kept, c)
		} else {
			missed = append(missed, id)
		}
	}
	return kept, missed, nil
}

func readIDList(file string) ([]string, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	var ids []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ids = append(ids, line)
	}
	return ids, nil
}
