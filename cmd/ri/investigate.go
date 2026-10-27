package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/agent"
	"github.com/tdavison784/release-intelligence/internal/investigate/bundle"
	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
	"github.com/tdavison784/release-intelligence/internal/investigate/renderdiff"
	runpkg "github.com/tdavison784/release-intelligence/internal/investigate/run"
	rtval "github.com/tdavison784/release-intelligence/internal/investigate/runtime"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/investigate/validate"
	"github.com/tdavison784/release-intelligence/internal/linkedev"
	"github.com/tdavison784/release-intelligence/internal/orchestrate"
)

const policyVersion = classify.Policy

// invFlags are the flags that describe an investigation request and its
// wiring; `ri investigate` and `ri worker` share them.
type invFlags struct {
	repo, values, manifests, crds, images, inventory, kubernetes, envName *string
	knowledge, minVerification                                            *string
	render                                                                *bool
	agent                                                                 *bool
	runtime                                                               *bool // opt-in observed validator (nil = off; `ri eval -investigations -runtime` sets it)
	driver, model, exchange, replay, script, claudeBin                    *string
	prompt                                                                *string // agent prompt version (nil or "" = agent.DefaultPromptVersion)
	synth                                                                 synthFlags
}

func addInvFlags(c *cli, fs interface {
	String(name, value, usage string) *string
	Bool(name string, value bool, usage string) *bool
}) invFlags {
	return invFlags{
		repo:       fs.String("repo", "", "environment repository (Argo CD / Flux / Helmfile / values / Kustomize inputs are discovered)"),
		values:     fs.String("values", "", "comma-separated Helm values files"),
		manifests:  fs.String("manifests", "", "comma-separated manifest files or directories"),
		crds:       fs.String("crds", "", "comma-separated CRD files or directories"),
		images:     fs.String("images", "", "comma-separated image references, or one file listing them"),
		inventory:  fs.String("inventory", "", "declared product inventory file"),
		kubernetes: fs.String("kubernetes", "", "Kubernetes version of the cluster"),
		envName:    fs.String("env-name", "", "environment name (default: the repository directory name)"),
		knowledge:  fs.String("knowledge", "knowledge", "directory of verified knowledge (facts clear changes only when trusted)"),
		render:     fs.Bool("render", true, "render both releases with the environment's configuration (the centerpiece evidence)"),
		agent:      fs.Bool("agent", false, "run the investigation agent (a sanctioned LLM stage; never decides a class)"),
		runtime:    fs.Bool("runtime", false, "also validate by an isolated upgrade in a throwaway kind cluster (observed level; needs docker, kind, kubectl; minutes per investigation)"),
		driver:     fs.String("agent-driver", "claude-cli", "agent driver: claude-cli|exchange|replay|scripted"),
		model:      fs.String("agent-model", "", "model for the agent driver"),
		exchange:   fs.String("agent-exchange", "", "exchange driver: directory for request/answer files"),
		replay:     fs.String("agent-replay", "", "replay driver: transcript file of an earlier session"),
		script:     fs.String("agent-script", "", "scripted driver: JSON file [{\"tool\":…,\"args\":{…}}, …] played as the model's tool calls (deterministic runs, live checks without a model)"),
		claudeBin:  fs.String("claude-bin", "", "claude CLI binary (claude-cli driver)"),
		prompt:     fs.String("agent-prompt", agent.DefaultPromptVersion, "agent prompt version (part of the config digest); an earlier one replays that version's transcripts"),
		synth:      addSynthFlags(fs),
	}
}

func (f invFlags) request() (investigate.Request, error) {
	var r investigate.Request
	images, err := imageList(*f.images)
	if err != nil {
		return r, err
	}
	name := *f.envName
	if name == "" {
		abs, _ := filepath.Abs(*f.repo)
		name = filepath.Base(abs)
	}
	r.Environment = investigate.EnvironmentInputs{
		Name: name, Repo: *f.repo, ValuesFiles: splitList(*f.values), Manifests: splitList(*f.manifests),
		CRDs: splitList(*f.crds), Images: images, Inventory: *f.inventory, KubernetesVersion: *f.kubernetes,
	}
	if *f.repo == "" && len(r.Environment.ValuesFiles)+len(r.Environment.Manifests)+len(r.Environment.CRDs)+len(r.Environment.Images) == 0 {
		return r, fmt.Errorf("%w: an environment is required (--repo, or --values/--manifests/--crds/--images)", app.ErrUsage)
	}
	// An investigation is identified by its inputs: make paths absolute so the
	// request means the same thing in a worker with another working directory.
	abs := func(ps []string) []string {
		out := make([]string, len(ps))
		for i, p := range ps {
			out[i], _ = filepath.Abs(p)
		}
		return out
	}
	if r.Environment.Repo != "" {
		r.Environment.Repo, _ = filepath.Abs(r.Environment.Repo)
	}
	r.Environment.ValuesFiles, r.Environment.Manifests, r.Environment.CRDs = abs(r.Environment.ValuesFiles), abs(r.Environment.Manifests), abs(r.Environment.CRDs)
	r.Config = investigate.Config{
		Policy: policyVersion, Bound: domain.DefaultTraversalBound,
		Validators: []string{validate.NameJoin, validate.NameOverrideInspection, validate.NameRenderDiff, validate.NameSchemaCompat, validate.NameStaticReference},
		Render:     *f.render, KubeVersion: *f.kubernetes,
		Agent: investigate.AgentConfig{Enabled: *f.agent, Driver: *f.driver, Model: *f.model,
			PromptVersion: agent.DefaultPromptVersion, Budget: agent.Budget(domain.AgentBudget{})},
	}
	if f.prompt != nil && *f.prompt != "" {
		if _, err := agent.LoadPrompt(*f.prompt); err != nil {
			return r, fmt.Errorf("%w: -agent-prompt: %v", app.ErrUsage, err)
		}
		r.Config.Agent.PromptVersion = *f.prompt
	}
	if f.runtime != nil && *f.runtime {
		r.Config.Validators = append(r.Config.Validators, rtval.Name)
	}
	sort.Strings(r.Config.Validators)
	return r, nil
}

// pipeline wires the real packages into a runpkg.Pipeline over the state dir.
func (c *cli) pipeline(f invFlags) (*runpkg.Pipeline, *store.Store, error) {
	return c.pipelineAt(f, c.g.state)
}

// pipelineAt is pipeline with the investigation store rooted at storeDir
// (the upstream cache stays in the state dir); `ri eval -investigations`
// keeps its runs out of the shared state this way.
func (c *cli) pipelineAt(f invFlags, storeDir string) (*runpkg.Pipeline, *store.Store, error) {
	a, err := c.newApp()
	if err != nil {
		return nil, nil, err
	}
	st, err := store.New(storeDir)
	if err != nil {
		return nil, nil, err
	}
	ws, err := runpkg.NewDirWorkspace(filepath.Join(st.Dir(), "work"))
	if err != nil {
		return nil, nil, err
	}
	cls, rev := classify.New(), classify.NewReviewApplier()
	var facts []domain.VerifiedFact
	if *f.knowledge != "" {
		if ks, err := app.LoadKnowledge(*f.knowledge); err == nil {
			facts = ks.Facts
		}
	}
	var rtv investigate.Validator // the observed validator, only when opted in
	if f.runtime != nil && *f.runtime {
		sets, err := catalog.LoadExercises(filepath.Join(c.g.products, "exercises"))
		if err != nil {
			return nil, nil, fmt.Errorf("--runtime: %w", err)
		}
		logf := func(string, ...any) {}
		if c.g.verbose {
			logf = func(f string, a ...any) { fmt.Fprintf(c.err, "  · "+f+"\n", a...) }
		}
		ar := rtval.NewAppRenderer(a)
		ar.Repo = *f.repo // the snapshot no longer carries the checkout path (55ac1cfa)
		rtv = rtval.New(rtval.WithStore(st), rtval.WithRenderer(ar),
			rtval.WithExercises(rtval.FromSets(sets...)...), rtval.WithLogf(logf))
	}
	validators := func(rep *domain.ImpactReport) []investigate.Validator {
		vs := []investigate.Validator{
			validate.JoinVerdicts(rep, validate.WithStore(st)),
			validate.RenderDiff(validate.WithStore(st), validate.WithRelations(relations.WithCatalog(a.Catalog))),
			validate.OverrideInspection(validate.WithStore(st), validate.WithFacts(facts...)),
			validate.SchemaCompat(validate.WithStore(st)),
			validate.StaticReference(validate.WithStore(st), validate.WithRelations(relations.WithCatalog(a.Catalog))),
		}
		if rtv != nil {
			vs = append(vs, rtv)
		}
		return vs
	}
	// The consumer graph of prompts with feature traceConsumers uses the
	// pipeline graph's relationship rules (relations.WithConsumers adds the rest).
	opts := []agent.Option{agent.WithValidators(validators(nil)...), agent.WithRelations(relations.WithCatalog(a.Catalog))}
	// Upstream evidence requests (prompt feature upstream) read through the
	// same cached repo-file / repo-dir adapters as ingestion, so -offline
	// replays a recorded state.
	if up, err := upstreamPort(a); err == nil {
		opts = append(opts, agent.WithUpstream(up))
	}
	if *f.exchange != "" {
		opts = append(opts, agent.WithDriver(agent.DriverExchange, &agent.Exchange{Dir: *f.exchange}))
	}
	if *f.replay != "" {
		raw, err := os.ReadFile(*f.replay)
		if err != nil {
			return nil, nil, err
		}
		opts = append(opts, agent.WithDriver(agent.DriverReplay, &agent.Replay{Transcript: raw}))
	}
	if *f.script != "" {
		raw, err := os.ReadFile(*f.script)
		if err != nil {
			return nil, nil, err
		}
		var steps []struct {
			Tool string `json:"tool"`
			Args any    `json:"args"`
		}
		if err := json.Unmarshal(raw, &steps); err != nil {
			return nil, nil, fmt.Errorf("--agent-script: %w", err)
		}
		sc := &agent.Scripted{Model: "scripted", ModelVersion: "scripted-1", Provider: "scripted", Turns: len(steps)}
		for i, st := range steps {
			sc.Steps = append(sc.Steps, agent.Step{Call: fmt.Sprintf("script-%d", i+1), Tool: st.Tool, Args: st.Args})
		}
		opts = append(opts, agent.WithDriver(agent.DriverScripted, sc))
	}
	if *f.claudeBin != "" {
		opts = append(opts, agent.WithDriver(agent.DriverClaudeCLI, &agent.ClaudeCLI{Binary: *f.claudeBin}))
	}
	d := runpkg.Deps{
		Artifacts: st, Records: st, Work: ws,
		Change:       bundle.NewChangeCollector(a, st, bundle.WithKnowledge(facts...)),
		Snapshot:     bundle.NewSnapshotBuilderWithHints(st, env.HintsFromCatalog(a.Catalog)),
		Renderer:     renderdiff.New(a, st),
		Assembler:    bundle.NewAssembler(st),
		Graph:        relations.NewBuilder(st, relations.WithCatalog(a.Catalog)),
		Exposure:     relations.NewExposure(relations.WithReconcilesFrom(a.Catalog)),
		Traverser:    relations.NewTraverser(),
		Validators:   validators,
		Investigator: agent.New(st, opts...),
		Classifier:   cls, Reviewer: rev,
		Synthesizer:  c.synthesizer(a, f.synth, *f.claudeBin),
		Report: func(ctx context.Context, r investigate.Request) (*domain.ImpactReport, error) {
			e := r.Environment
			return a.Impact(ctx, r.Product, r.From, r.To, app.ImpactOptions{
				Environment: env.Inputs{Repo: e.Repo, ValuesFiles: e.ValuesFiles, Manifests: e.Manifests, CRDs: e.CRDs,
					Images: e.Images, Inventory: e.Inventory, KubernetesVersion: e.KubernetesVersion},
				Facts: facts, MinVerification: domain.VerifiedHuman,
				Render: &app.RenderOptions{Repo: e.Repo, ValuesFiles: e.ValuesFiles, KubeVersion: e.KubernetesVersion},
			})
		},
	}
	return runpkg.New(d), st, nil
}

// upstreamPort is the agent's upstream fetch port: the product's code
// repositories and tags (linkedev.CodeReposOf, TagFor) and the registered
// repo-file / repo-dir adapters.
func upstreamPort(a *app.App) (agent.Upstream, error) {
	files, err := a.Registry.DocumentFetcher("repo-file")
	if err != nil {
		return agent.Upstream{}, err
	}
	dirs, err := a.Registry.DirectoryFetcher("repo-dir")
	if err != nil {
		return agent.Upstream{}, err
	}
	return agent.Upstream{Files: files, Dirs: dirs, Product: func(id domain.ProductID) ([]string, func(string) string, bool) {
		def, ok := a.Catalog.Get(string(id))
		if !ok {
			return nil, nil, false
		}
		return linkedev.CodeReposOf(def), def.TagFor, true
	}}, nil
}

// definitionDigest digests the product definition file (the ChangeKey input).
func (c *cli) definitionDigest(product string) string {
	for _, ext := range []string{".yaml", ".yml"} {
		if raw, err := os.ReadFile(filepath.Join(c.g.products, product+ext)); err == nil {
			sum := sha256.Sum256(raw)
			return "sha256:" + hex.EncodeToString(sum[:])
		}
	}
	return ""
}

func (c *cli) logger() *slog.Logger {
	lvl := slog.LevelWarn
	if c.g.verbose {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(c.err, &slog.HandlerOptions{Level: lvl}))
}

// investigateSubcommands lets other lanes' subcommands plug in.
var investigateSubcommands = map[string]func(*cli, []string) error{
	"review": (*cli).investigateReview,
	"state":  (*cli).investigateState,
}

// investigateCmd is `ri investigate`: the runner, `review` and `state`.
// (`ri investigate mcp` is dispatched by main before this.)
func (c *cli) investigateCmd(args []string) error {
	if len(args) > 0 {
		if sub, ok := investigateSubcommands[args[0]]; ok {
			return sub(c, args[1:])
		}
	}
	fs := c.flags("investigate", "<product> <from> <to>")
	f := addInvFlags(c, fs)
	output := fs.String("o", "text", "output format: text|json")
	useTemporal := fs.Bool("temporal", false, "run as a Temporal workflow (needs `ri worker`) instead of in-process")
	addr := fs.String("temporal-addr", envOr("RI_TEMPORAL_ADDR", "localhost:7233"), "Temporal frontend address")
	ns := fs.String("temporal-namespace", envOr("RI_TEMPORAL_NAMESPACE", "default"), "Temporal namespace")
	detach := fs.Bool("detach", false, "with --temporal: print the workflow id and return without waiting for the result")
	rerun := fs.Int("rerun", 0, "with --temporal: deliberate re-run number (workflow ids are content-derived; a finished result is not silently recomputed)")
	attrs := fs.Bool("search-attrs", false, "with --temporal: upsert the Ri* search attributes (register them first, TEMPORAL.md)")
	reviewAction := fs.String("review", "", "in-process: apply this review decision when the run reaches review: approve|reject|more")
	reviewer := fs.String("reviewer", "", "reviewer name for --review")
	note := fs.String("review-note", "", "note for --review")
	reviewTimeout := fs.Duration("review-timeout", 0, "how long to wait for a review decision (Temporal default 7d; in-process default: do not wait)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 3 {
		fs.Usage()
		return fmt.Errorf("%w: expected <product> <from> <to> (or: review|state|mcp ...)", app.ErrUsage)
	}
	req, err := f.request()
	if err != nil {
		return err
	}
	req.Product, req.From, req.To = pos[0], pos[1], pos[2]
	if *useTemporal {
		return c.investigateTemporal(req, *addr, *ns, *detach, *rerun, *attrs, *reviewTimeout, *output, f.synth.enabled())
	}
	p, st, err := c.pipeline(f)
	if err != nil {
		return err
	}
	r := &runpkg.Runner{P: p, Definition: c.definitionDigest, ReviewTimeout: *reviewTimeout, Synthesize: f.synth.enabled(),
		Logf: func(f string, a ...any) { fmt.Fprintf(c.err, "· "+f+"\n", a...) }}
	if *reviewAction != "" {
		d, err := reviewDecision(*reviewAction, *reviewer, *note, nil)
		if err != nil {
			return err
		}
		r.Review = func(context.Context, *domain.Investigation) (*investigate.ReviewDecision, error) { return &d, nil }
	}
	res, err := r.Run(c.ctx, req)
	if res != nil && res.Investigation != "" {
		_ = c.printInvestigation(st, res, *output)
	}
	return err
}

func reviewDecision(action, reviewer, note string, requests []string) (investigate.ReviewDecision, error) {
	var a investigate.ReviewAction
	switch action {
	case "approve":
		a = investigate.ReviewApprove
	case "reject":
		a = investigate.ReviewReject
	case "more", "more-evidence", string(investigate.ReviewRequestMoreEvidence):
		a = investigate.ReviewRequestMoreEvidence
	default:
		return investigate.ReviewDecision{}, fmt.Errorf("%w: review action must be approve|reject|more", app.ErrUsage)
	}
	if reviewer == "" {
		reviewer = os.Getenv("USER")
	}
	if reviewer == "" {
		return investigate.ReviewDecision{}, fmt.Errorf("%w: --reviewer is required", app.ErrUsage)
	}
	// A decision from this CLI is a human's: a proxy (AI) reviewer is recorded
	// by tooling that sets ReviewerKind itself, and can never approve.
	return investigate.ReviewDecision{Action: a, Reviewer: reviewer, ReviewerKind: domain.ReviewerHuman, Note: note, At: time.Now().UTC(), Requests: requests}, nil
}

func (c *cli) printInvestigation(st *store.Store, res *runpkg.Result, output string) error {
	if output == "json" {
		return c.writeJSON(res)
	}
	inv, err := st.GetInvestigation(c.ctx, res.Investigation)
	if err != nil {
		return err
	}
	classes := map[domain.FindingClass]int{}
	for _, f := range inv.Findings {
		classes[f.Class]++
	}
	fmt.Fprintf(c.out, "investigation %s  (bundle %s)\n", inv.ID, inv.Bundle)
	fmt.Fprintf(c.out, "  outcome   %s\n  status    %s\n  review    %s", res.Outcome, inv.Status, inv.Review.State)
	if inv.Review.Reviewer != "" {
		fmt.Fprintf(c.out, " by %s (%s)", inv.Review.Reviewer, inv.Review.ReviewerKind)
	}
	if inv.Decision != nil {
		fmt.Fprintf(c.out, "\n  decision  %s", inv.Decision.Outcome)
	}
	fmt.Fprintf(c.out, "\n  findings ")
	for _, k := range domain.FindingClasses {
		fmt.Fprintf(c.out, " %s=%d", k, classes[k])
	}
	fmt.Fprintf(c.out, "\n  validations %d  agent %s  audit events %d  rounds %d\n  record digest %s\n",
		len(inv.Validations), agentStatus(inv), len(inv.Audit), res.Rounds+1, res.RecordDigest)
	if res.Outcome == runpkg.OutcomeAwaitingReview {
		fmt.Fprintf(c.out, "  awaiting a human: ri investigate review %s approve|reject|more --reviewer NAME\n", inv.ID)
	}
	return c.printJudgement(c.out, st, inv)
}

func agentStatus(inv *domain.Investigation) string {
	if inv.Agent == nil {
		return "none"
	}
	return string(inv.Agent.Status)
}

// investigateReview is `ri investigate review <id> approve|reject|more`:
// with --temporal it signals the workflow <id>; otherwise it applies the
// decision to the stored run record <id> (an in-process run awaiting review).
func (c *cli) investigateReview(args []string) error {
	fs := c.flags("investigate review", "<id> approve|reject|more")
	reviewer := fs.String("reviewer", "", "reviewer name (default $USER)")
	note := fs.String("note", "", "note recorded with the decision")
	req := fs.String("request", "", "with `more`: comma-separated inputs wanted (values:<path>, manifest:<path>, crd:<path>, image:<ref>)")
	useTemporal := fs.Bool("temporal", false, "signal a Temporal workflow")
	addr := fs.String("temporal-addr", envOr("RI_TEMPORAL_ADDR", "localhost:7233"), "Temporal frontend address")
	ns := fs.String("temporal-namespace", envOr("RI_TEMPORAL_NAMESPACE", "default"), "Temporal namespace")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		fs.Usage()
		return fmt.Errorf("%w: expected <id> approve|reject|more", app.ErrUsage)
	}
	d, err := reviewDecision(pos[1], *reviewer, *note, splitList(*req))
	if err != nil {
		return err
	}
	if *useTemporal {
		tc, err := orchestrate.Dial(*addr, *ns, c.logger())
		if err != nil {
			return err
		}
		defer tc.Close()
		if err := orchestrate.SendReview(c.ctx, tc, pos[0], d); err != nil {
			return err
		}
		fmt.Fprintf(c.out, "sent %s from %s to workflow %s\n", d.Action, d.Reviewer, pos[0])
		return nil
	}
	rev := classify.NewReviewApplier()
	st, err := store.New(c.g.state)
	if err != nil {
		return err
	}
	ws, err := runpkg.NewDirWorkspace(filepath.Join(st.Dir(), "work"))
	if err != nil {
		return err
	}
	p := runpkg.New(runpkg.Deps{Records: st, Artifacts: st, Work: ws, Reviewer: rev})
	inv, err := st.GetInvestigation(c.ctx, pos[0])
	if err != nil {
		return err
	}
	out, err := p.ApplyReview(c.ctx, runpkg.ReviewInput{Investigation: pos[0], Round: 0, Decision: d})
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "investigation %s: review %s -> %s (status %s)\n", inv.ID, inv.Review.State, out.State, out.Status)
	if out.State == domain.ReviewMoreEvidenceWanted {
		fmt.Fprintln(c.out, "  more evidence was requested: re-run `ri investigate` with the extra inputs to start a new round")
	}
	return nil
}

// investigateState is `ri investigate state <id>`: the workflow's state
// query (--temporal) or the stored run record's summary.
func (c *cli) investigateState(args []string) error {
	fs := c.flags("investigate state", "<id>")
	useTemporal := fs.Bool("temporal", false, "query a Temporal workflow")
	addr := fs.String("temporal-addr", envOr("RI_TEMPORAL_ADDR", "localhost:7233"), "Temporal frontend address")
	ns := fs.String("temporal-namespace", envOr("RI_TEMPORAL_NAMESPACE", "default"), "Temporal namespace")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		fs.Usage()
		return fmt.Errorf("%w: expected <id>", app.ErrUsage)
	}
	if *useTemporal {
		tc, err := orchestrate.Dial(*addr, *ns, c.logger())
		if err != nil {
			return err
		}
		defer tc.Close()
		s, err := orchestrate.GetState(c.ctx, tc, pos[0])
		if err != nil {
			return err
		}
		return c.writeJSON(s)
	}
	st, err := store.New(c.g.state)
	if err != nil {
		return err
	}
	inv, err := st.GetInvestigation(c.ctx, pos[0])
	if err != nil {
		return err
	}
	return c.writeJSON(map[string]any{"id": inv.ID, "status": inv.Status, "review": inv.Review, "decision": inv.Decision,
		"findings": len(inv.Findings), "validations": len(inv.Validations), "audit": inv.Audit})
}

func (c *cli) investigateTemporal(req investigate.Request, addr, ns string, detach bool, rerun int, attrs bool, reviewTimeout time.Duration, output string, synthesize bool) error {
	tc, err := orchestrate.Dial(addr, ns, c.logger())
	if err != nil {
		return err
	}
	defer tc.Close()
	wr, err := orchestrate.Start(c.ctx, tc, orchestrate.Input{
		Request: req, Definition: c.definitionDigest(req.Product), ReviewTimeout: reviewTimeout, SearchAttributes: attrs, Synthesize: synthesize,
	}, rerun)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.err, "workflow %s (run %s)\n", wr.GetID(), wr.GetRunID())
	if detach {
		fmt.Fprintf(c.out, "%s\n", wr.GetID())
		return nil
	}
	fmt.Fprintf(c.err, "waiting; review with: ri investigate review %s approve|reject|more --temporal --reviewer NAME\n", wr.GetID())
	var res orchestrate.Result
	if err := wr.Get(c.ctx, &res); err != nil {
		return err
	}
	if output == "json" {
		return c.writeJSON(res)
	}
	st, err := store.New(c.g.state)
	if err != nil {
		return err
	}
	return c.printInvestigation(st, &res.Result, output)
}

// worker is `ri worker`: serves the investigation task queue.
func (c *cli) worker(args []string) error {
	fs := c.flags("worker", "")
	f := addInvFlags(c, fs)
	addr := fs.String("temporal-addr", envOr("RI_TEMPORAL_ADDR", "localhost:7233"), "Temporal frontend address")
	ns := fs.String("temporal-namespace", envOr("RI_TEMPORAL_NAMESPACE", "default"), "Temporal namespace")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	p, _, err := c.pipeline(f)
	if err != nil {
		return err
	}
	tc, err := orchestrate.Dial(*addr, *ns, c.logger())
	if err != nil {
		return err
	}
	defer tc.Close()
	w := orchestrate.NewWorker(tc, &orchestrate.Activities{P: p})
	fmt.Fprintf(c.err, "worker on task queue %s (%s, namespace %s); state %s\n", orchestrate.TaskQueue, *addr, *ns, c.g.state)
	if err := w.Start(); err != nil {
		return err
	}
	<-c.ctx.Done()
	w.Stop()
	return nil
}
