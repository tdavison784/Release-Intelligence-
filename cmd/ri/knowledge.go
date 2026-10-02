package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

const knowledgeUsage = `ri knowledge — the learning loop's store, queue, dataset and metrics

Subcommands:
  route [flags]                  route candidates: write review items and auto-verified facts
  decide [flags] <review-item>   record one decision (the only way proxy decisions enter)
  review-fact <fact-id>          open a re-review of a fact (reject retracts, correct supersedes)
  export [-o file]               the human-feedback dataset as JSONL
  metrics [-o text|json]         agreement, per-model accuracy, review cost, fact counts
  candidates | propose | validate  produced by the semantic and validate lanes (not wired in this build)
`

func (c *cli) knowledge(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(c.err, knowledgeUsage)
		return fmt.Errorf("%w: missing knowledge subcommand", app.ErrUsage)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "route":
		return c.knowledgeRoute(rest)
	case "decide":
		return c.knowledgeDecide(rest)
	case "review-fact":
		return c.knowledgeReviewFact(rest)
	case "export":
		return c.knowledgeExport(rest)
	case "metrics":
		return c.knowledgeMetrics(rest)
	case "candidates", "propose", "validate":
		return fmt.Errorf("ri knowledge %s is provided by the %s lane and is not wired in this build", sub,
			map[string]string{"candidates": "semantic", "propose": "semantic", "validate": "validate"}[sub])
	case "help":
		fmt.Fprint(c.err, knowledgeUsage)
		return nil
	}
	fmt.Fprint(c.err, knowledgeUsage)
	return fmt.Errorf("%w: unknown knowledge subcommand %q", app.ErrUsage, sub)
}

func (c *cli) knowledgeStore(dir string) knowledge.Store { return knowledge.NewFileStore(dir) }

func (c *cli) knowledgeRoute(args []string) error {
	fs := c.flags("knowledge route", "")
	dir := fs.String("dir", knowledge.DefaultDir, "knowledge directory")
	product := fs.String("product", "", "only this product")
	release := fs.String("release", "", "only this release")
	auto := fs.Bool("auto-approve", true, "auto-approve render-verifiable candidates whose open aspects have independent-family consensus")
	audit := fs.Int("audit-every", 5, "sample one in N auto-approved facts into human review (0 = none; action-eligible ones are always audited)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	opts := knowledge.RouteOptions{AuditEvery: *audit, Now: time.Now}
	if *auto {
		opts.Policy = knowledge.AutoApproveRenderVerifiable
	}
	q := knowledge.Query{Product: domain.ProductID(*product)}
	if *release != "" {
		q.Releases = []string{*release}
	}
	sum, err := knowledge.RouteStore(c.ctx, c.knowledgeStore(*dir), opts, q)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "routed %d candidates: %d review items created, %d facts auto-verified, %d audit items, %d already stored\n",
		sum.Candidates, sum.Items, sum.Facts, len(sum.Audits), sum.Existing)
	return nil
}

func (c *cli) knowledgeDecide(args []string) error {
	fs := c.flags("knowledge decide", "<review-item-id>")
	dir := fs.String("dir", knowledge.DefaultDir, "knowledge directory")
	action := fs.String("action", "accept", "accept|reject|correct|need-more-evidence|defer")
	reviewer := fs.String("reviewer", "", "reviewer name (required)")
	kind := fs.String("reviewer-kind", "human", "human|proxy (a proxy requires the -proxy-* provenance flags)")
	reason := fs.String("reason", "", "reason (required for reject, correct, need-more-evidence)")
	labels := fs.String("labels", "", "comma-separated feedback labels (default follows the action; correct needs wrong-* labels)")
	correctedFile := fs.String("corrected", "", "JSON file with the corrected SemanticAssertion (action correct)")
	duplicateOf := fs.String("duplicate-of", "", "fact id this item restates (action reject, label duplicate)")
	started := fs.String("started-at", "", "RFC3339 time the reviewer started (default: now)")
	pModel := fs.String("proxy-model", "", "proxy: model that acted as reviewer")
	pVersion := fs.String("proxy-model-version", "", "proxy: model version as reported")
	pPromptV := fs.String("proxy-prompt-version", "", "proxy: prompt template version")
	pPromptD := fs.String("proxy-prompt-digest", "", "proxy: digest of the rendered prompt")
	pInput := fs.String("proxy-input-evidence", "", "proxy: comma-separated evidence ids the proxy was shown")
	pConf := fs.String("proxy-confidence", "medium", "proxy: low|medium (never high)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("%w: ri knowledge decide <review-item-id>", app.ErrUsage)
	}
	if strings.TrimSpace(*reviewer) == "" {
		return fmt.Errorf("%w: -reviewer is required", app.ErrUsage)
	}
	store := c.knowledgeStore(*dir)
	rec, err := store.Get(c.ctx, pos[0])
	if err != nil {
		return err
	}
	if rec.ReviewItem == nil {
		return fmt.Errorf("%s is not a review item", pos[0])
	}
	now := time.Now().UTC()
	start := now
	if *started != "" {
		if start, err = time.Parse(time.RFC3339, *started); err != nil {
			return fmt.Errorf("%w: -started-at: %v", app.ErrUsage, err)
		}
	}
	orig := rec.ReviewItem.Proposed
	d := domain.ReviewDecision{
		ReviewItemID: rec.ReviewItem.ID, Action: domain.DecisionAction(*action), Original: &orig,
		Reviewer: *reviewer, ReviewerKind: domain.ReviewerKind(*kind), Reason: *reason,
		StartedAt: start, DecidedAt: now, DuplicateOf: *duplicateOf,
	}
	switch d.Action {
	case domain.ActionNeedMoreEvidence, domain.ActionDefer:
		d.Original = nil
	}
	for _, l := range splitList(*labels) {
		d.Labels = append(d.Labels, domain.FeedbackLabel(l))
	}
	if len(d.Labels) == 0 {
		switch d.Action {
		case domain.ActionAccept:
			d.Labels = []domain.FeedbackLabel{domain.LabelAccepted}
		case domain.ActionReject:
			d.Labels = []domain.FeedbackLabel{domain.LabelRejected}
		case domain.ActionNeedMoreEvidence:
			d.Labels = []domain.FeedbackLabel{domain.LabelInsufficientEvidence}
		}
	}
	if *correctedFile != "" {
		b, err := os.ReadFile(*correctedFile)
		if err != nil {
			return err
		}
		var a domain.SemanticAssertion
		if err := json.Unmarshal(b, &a); err != nil {
			return fmt.Errorf("-corrected: %w", err)
		}
		d.Corrected = &a
	}
	switch d.ReviewerKind {
	case domain.ReviewerProxy:
		if *pConf == string(domain.ConfidenceHigh) {
			return fmt.Errorf("%w: a proxy's confidence is capped at medium", app.ErrUsage)
		}
		gen := now
		prov := domain.Provenance{Method: domain.MethodAI, Producer: "knowledge.proxy@v1", Confidence: domain.Confidence(*pConf),
			Model: *pModel, ModelVersion: *pVersion, PromptVersion: *pPromptV, PromptDigest: *pPromptD, GeneratedAt: &gen}
		for _, id := range splitList(*pInput) {
			prov.InputEvidence = append(prov.InputEvidence, domain.EvidenceID(id))
		}
		d.ProxyProvenance = &prov
	case domain.ReviewerHuman:
	default:
		return fmt.Errorf("%w: -reviewer-kind must be human or proxy", app.ErrUsage)
	}
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	out, err := knowledge.NewQueue(store, time.Now).Decide(c.ctx, []domain.ReviewDecision{d})
	if err != nil {
		return err
	}
	o := out[0]
	fmt.Fprintf(c.out, "decision %s (%s, %s)\n", o.Decision.ID, o.Decision.Action, o.Decision.ReviewerKind)
	if o.Fact != nil {
		fmt.Fprintf(c.out, "fact %s at level %s\n", o.Fact.ID, o.Fact.Level())
	}
	if len(o.OpenAspects) > 0 {
		fmt.Fprintf(c.out, "open aspects: %v; %d follow-up items\n", o.OpenAspects, len(o.FollowUps))
	}
	return nil
}

func (c *cli) knowledgeReviewFact(args []string) error {
	fs := c.flags("knowledge review-fact", "<fact-id>")
	dir := fs.String("dir", knowledge.DefaultDir, "knowledge directory")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("%w: ri knowledge review-fact <fact-id>", app.ErrUsage)
	}
	it, err := knowledge.OpenFactReview(c.ctx, c.knowledgeStore(*dir), pos[0], time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "review item %s opened for %s\n", it.ID, pos[0])
	return nil
}

func (c *cli) loadSnapshot(dir, product string) (*knowledge.Snapshot, error) {
	return c.knowledgeStore(dir).Load(c.ctx, knowledge.Query{Product: domain.ProductID(product)})
}

func (c *cli) knowledgeExport(args []string) error {
	fs := c.flags("knowledge export", "")
	dir := fs.String("dir", knowledge.DefaultDir, "knowledge directory")
	product := fs.String("product", "", "only this product")
	out := fs.String("o", "", "write JSONL to this file (default stdout)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	snap, err := c.loadSnapshot(*dir, *product)
	if err != nil {
		return err
	}
	w := c.out
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	return knowledge.ExportDataset(snap, w)
}

func (c *cli) knowledgeMetrics(args []string) error {
	fs := c.flags("knowledge metrics", "")
	dir := fs.String("dir", knowledge.DefaultDir, "knowledge directory")
	product := fs.String("product", "", "only this product")
	output := fs.String("o", "text", "output format: text|json")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	snap, err := c.loadSnapshot(*dir, *product)
	if err != nil {
		return err
	}
	m := knowledge.ComputeMetrics(snap)
	switch *output {
	case "text":
		knowledge.WriteMetricsReport(c.out, m)
		return nil
	case "json":
		return c.writeJSON(m)
	}
	return fmt.Errorf("%w: unknown output format %q (want text or json)", app.ErrUsage, *output)
}
