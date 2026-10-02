package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/impactenrich"
)

// impact runs `ri impact <product> <from> <to>`: the upgrade edge joined with
// the caller's environment. Every environment input is optional, but at least
// one must be given — the whole point of the command is the join. --repo
// enables directory mode: inputs are discovered in a repository tree by
// convention and the explicit flags compose with what was found.
//
// -enrich adds the optional AI step (mirroring `ri upgrade -enrich`): the
// deterministic report is built first, unchanged; the step then asks a
// bounded model about the unknown note-derived findings, duplicate groups and
// migration candidates, and attaches provenance-preserving enrichments plus
// at most review-required suggestions on unknown findings.
func (c *cli) impact(args []string) error {
	fs := c.flags("impact", "<product> <from> <to>")
	output := fs.String("o", "text", "output format: text|json")
	kubernetes := fs.String("kubernetes", "", "Kubernetes version of the cluster (e.g. 1.31 or 1.31.5)")
	repo := fs.String("repo", "", "directory mode: discover values/manifests/Argo CD/Flux inputs in a repository tree by convention (explicit flags compose with, and are applied after, the discovered files)")
	values := fs.String("values", "", "comma-separated Helm values files of the environment")
	manifests := fs.String("manifests", "", "comma-separated manifest files or directories (multi-document YAML)")
	crds := fs.String("crds", "", "comma-separated CustomResourceDefinition files or directories")
	images := fs.String("images", "", "comma-separated image references, or one file listing them (one per line)")
	inventory := fs.String("inventory", "", "declared product inventory: a YAML list of {product, version, note?} (which other products run here, e.g. ingress-nginx 1.12.1); in --repo mode <repo>/inventory.yaml is used when present")
	policy := fs.String("policy", "", "path policy override: minor-lineage|all")
	showNotAffected := fs.Bool("show-not-affected", false, "also list the not-affected verdicts with their evaluation records (the summary always counts them)")
	showUnknown := fs.Bool("show-unknown", false, "list every UNKNOWN finding instead of the collapsed per-reason summary (the summary always counts them; JSON always carries everything)")
	knowledgeDir := fs.String("knowledge", "", "directory of verified knowledge (the knowledge/ record tree): facts are evaluated against the environment (impact:knowledge-* findings)")
	minVerification := fs.String("min-verification", "", "with -knowledge: use facts verified at least at this level: deterministic|human|consensus|proxy (default human; consensus and proxy facts never clear a change)")
	doRender := fs.Bool("render", false, "render the source and target releases with the customer's configuration (--values/--repo, or chart defaults without them) and diff the manifests: what actually changes for this environment, each rendered change correlated with the changelog entries that mention it")
	rf := addRenderFlags(fs)
	ef := enrichFlagsFor("impact", fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 3 {
		fs.Usage()
		return fmt.Errorf("%w: expected <product> <from> <to>", app.ErrUsage)
	}
	if err := ef.check(); err != nil {
		return err
	}
	in := app.ImpactOptions{Policy: *policy}
	in.Environment.KubernetesVersion = *kubernetes
	in.Environment.Repo = *repo
	in.Environment.Inventory = *inventory
	in.Environment.ValuesFiles = splitList(*values)
	in.Environment.Manifests = splitList(*manifests)
	in.Environment.CRDs = splitList(*crds)
	in.Environment.Images, err = imageList(*images)
	if err != nil {
		return err
	}
	if *knowledgeDir != "" {
		lvl, err := app.ParseVerificationLevel(*minVerification)
		if err != nil {
			return err
		}
		ks, err := app.LoadKnowledge(*knowledgeDir)
		if err != nil {
			return err
		}
		for _, w := range ks.Warnings {
			fmt.Fprintf(c.err, "knowledge: %s\n", w)
		}
		in.Facts, in.MinVerification = ks.Facts, lvl
	} else if *minVerification != "" {
		return fmt.Errorf("%w: -min-verification needs -knowledge", app.ErrUsage)
	}
	if *doRender {
		in.Render = &app.RenderOptions{
			ValuesFiles: splitList(*values), Repo: *repo, Overlays: splitList(*rf.overlays),
			ReleaseName: *rf.releaseName, Namespace: *rf.namespace, KubeVersion: *kubernetes,
			APIVersions: splitList(*rf.apiVersions),
			Release:     *rf.chartDefaults || (*values == "" && *repo == "" && *rf.overlays == ""),
		}
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	irun, err := a.ImpactRun(c.ctx, pos[0], pos[1], pos[2], in)
	if err != nil {
		return err
	}
	rep, edge, e, rres := irun.Report, irun.Edge, irun.Env, irun.Render
	if *ef.candidates {
		printImpactCandidates(c.err, rep, edge, e, impactenrich.Candidates(rep, edge, impactenrich.CandidateOptions{}))
	}
	if *ef.enrich {
		res, err := c.impactEnrich(a, rep, edge, e, ef)
		if err != nil {
			return err
		}
		run := res.Run
		fmt.Fprintf(c.err, "impact enrich: %s; %d candidates, %d prompts, %d enrichments accepted, %d rejected, %d pending, %d failed; %d unknown(s) now carry an AI review suggestion\n",
			a.EnrichmentBackendForImpact(app.ImpactEnrichOptions{Model: *ef.model, ExchangeDir: *ef.exchange, APIKey: os.Getenv("ANTHROPIC_API_KEY"), BaseURL: os.Getenv("ANTHROPIC_BASE_URL"), Thinking: os.Getenv("ANTHROPIC_THINKING"), Max: *ef.maxGroups}),
			run.CandidateGroups, run.Requests, run.Accepted, len(run.Rejected), run.Pending, run.Failed, rep.Summary.SuggestedReview)
		if run.Pending > 0 {
			if *ef.exchange != "" {
				fmt.Fprintf(c.err, "impact enrich: %d prompts are waiting in %s; write a <digest>.response.json next to each request (docs/ENRICHMENT.md) and run again\n", run.Pending, *ef.exchange)
			} else {
				fmt.Fprintf(c.err, "impact enrich: %d prompts have no cached answer; set ANTHROPIC_API_KEY or use -llm-exchange DIR\n", run.Pending)
			}
		}
	}
	if *output == "json" {
		if rres != nil {
			// the impact report unchanged, plus the rendered delta beside it
			return c.writeJSON(struct {
				*domain.ImpactReport
				Render renderReport `json:"render"`
			}{rep, renderJSON(rres, *rf.showValues)})
		}
		return c.writeJSON(rep)
	}
	if err := impact.RenderText(c.out, rep, impact.RenderOptions{Color: isTerminal(c.out), ShowNotAffected: *showNotAffected, ShowUnknown: *showUnknown}); err != nil {
		return err
	}
	if rres != nil {
		fmt.Fprintln(c.out)
		writeRenderText(c.out, rres, *rf.showValues)
	}
	return nil
}

// impactEnrich runs the optional AI step over the deterministic report and
// attaches the result in place.
func (c *cli) impactEnrich(a *app.App, rep *domain.ImpactReport, edge *domain.UpgradeEdge, e *env.Environment, ef upgradeEnrichFlags) (*impactenrich.Result, error) {
	opts := app.ImpactEnrichOptions{Model: *ef.model, ExchangeDir: *ef.exchange, APIKey: os.Getenv("ANTHROPIC_API_KEY"), BaseURL: os.Getenv("ANTHROPIC_BASE_URL"), Thinking: os.Getenv("ANTHROPIC_THINKING"), Max: *ef.maxGroups}
	return a.EnrichImpact(c.ctx, rep, edge, e, opts)
}

// printImpactCandidates prints the deterministic candidates to stderr
// (debug); they are not conclusions.
func printImpactCandidates(w io.Writer, rep *domain.ImpactReport, edge *domain.UpgradeEdge, e *env.Environment, cs []impactenrich.Candidate) {
	titles := map[string]string{}
	for _, f := range rep.Findings {
		titles[f.ID] = f.Title
	}
	fmt.Fprintf(w, "Impact candidates (deterministic, not conclusions): %d\n", len(cs))
	for _, cand := range cs {
		fmt.Fprintf(w, "  %s  type %s\n", cand.ID, cand.Type)
		for _, id := range cand.Findings {
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

// imageList reads the --images flag: a comma-separated list, or the name of
// an existing file holding one reference per line (# comments allowed).
func imageList(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	if strings.Contains(s, ",") {
		return splitList(s), nil
	}
	if fi, err := os.Stat(s); err == nil && !fi.IsDir() {
		b, err := os.ReadFile(s)
		if err != nil {
			return nil, fmt.Errorf("read image list: %w", err)
		}
		var out []string
		for _, ln := range strings.Split(string(b), "\n") {
			if i := strings.IndexByte(ln, '#'); i >= 0 {
				ln = ln[:i]
			}
			if ln = strings.TrimSpace(ln); ln != "" {
				out = append(out, ln)
			}
		}
		return out, nil
	}
	return []string{s}, nil
}
