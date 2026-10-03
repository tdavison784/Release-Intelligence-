package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/proxyreview"
	"github.com/tdavison784/release-intelligence/internal/store"
)

// knowledgeProxyPrompt writes one proxy-review request per selected item
// (`ri knowledge proxy-prompt`). scripts/proxy-review.sh answers them with
// stateless `claude -p` calls and records each verdict with
// `ri knowledge decide -reviewer-kind proxy -proxy-response …`.
func (c *cli) knowledgeProxyPrompt(args []string) error {
	fs := c.flags("knowledge proxy-prompt", "")
	dir := fs.String("dir", knowledge.DefaultDir, "knowledge directory")
	out := fs.String("o", "", "exchange directory to write <item>.request.json files into (required)")
	model := fs.String("model", proxyreview.DefaultModel, "proxy model")
	prio := fs.String("priority", "non-high", "items by routing priority: non-high|high|all")
	question := fs.String("question", "", "only this question type")
	product := fs.String("product", "", "only this product")
	items := fs.String("items", "", "comma-separated item ids (overrides the filters except status)")
	noHuman := fs.Bool("no-human-context", false, "leave earlier human decisions out of the prompts (shadow pass)")
	limit := fs.Int("limit", 0, "at most N requests (0 = all)")
	noSections := fs.Bool("no-sections", false, "leave out the upstream section context (prompt v3 shows it from the ingested release store under -state)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("%w: -o <exchange-dir> is required", app.ErrUsage)
	}
	switch *prio {
	case "non-high", "high", "all":
	default:
		return fmt.Errorf("%w: -priority must be non-high, high or all", app.ErrUsage)
	}
	snap, err := c.loadSnapshot(*dir, *product)
	if err != nil {
		return err
	}
	only := map[string]bool{}
	for _, id := range splitList(*items) {
		only[id] = true
	}
	var sel []domain.ReviewItem
	for _, it := range snap.ReviewItems {
		if it.Status != domain.ReviewPending {
			continue
		}
		if len(only) > 0 {
			if only[it.ID] {
				sel = append(sel, it)
			}
			continue
		}
		high := it.Routing.Priority == domain.PriorityHigh
		if (*prio == "non-high" && high) || (*prio == "high" && !high) {
			continue
		}
		if *question != "" && string(it.QuestionType) != *question {
			continue
		}
		sel = append(sel, it)
	}
	sort.Slice(sel, func(i, j int) bool { return sel[i].ID < sel[j].ID })
	if *limit > 0 && len(sel) > *limit {
		sel = sel[:*limit]
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	var releases proxyreview.ReleaseSource
	if !*noSections {
		st := store.New(filepath.Join(c.g.state, "store"))
		releases = func(p domain.ProductID, v string) (*domain.Release, error) { return st.LoadRelease(string(p), v) }
	}
	written, same, skipped := 0, 0, 0
	for _, it := range sel {
		rc, err := knowledge.AssembleContext(snap, it.ID)
		if err != nil {
			return err
		}
		req, err := proxyreview.Build(rc, proxyreview.Options{Model: *model, NoHumanContext: *noHuman, Releases: releases})
		if errors.Is(err, proxyreview.ErrNotReviewable) {
			fmt.Fprintf(c.err, "skipped %s: %v\n", it.ID, err)
			skipped++
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", it.ID, err)
		}
		b, err := json.MarshalIndent(req, "", "  ")
		if err != nil {
			return err
		}
		b = append(b, '\n')
		path := filepath.Join(*out, it.ID+".request.json")
		if have, err := os.ReadFile(path); err == nil && bytes.Equal(have, b) {
			same++
			continue
		}
		if _, err := os.Stat(filepath.Join(*out, it.ID+".response.json")); err == nil {
			return fmt.Errorf("%s: a response exists for an older prompt of this item; move it away before re-prompting", it.ID)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return err
		}
		written++
	}
	fmt.Fprintf(c.out, "proxy-prompt: %d items selected, %d requests written, %d unchanged, %d skipped (%s)\n", len(sel), written, same, skipped, *out)
	return nil
}

// decideProxyResponse records one proxy verdict (`ri knowledge decide
// -reviewer-kind proxy -proxy-request R -proxy-response S`): the decision is
// built by proxyreview.Decision and recorded through the same queue as every
// other decision. Each outcome, success or refusal, is appended to the ledger.
func (c *cli) decideProxyResponse(store knowledge.Store, itemID, reqPath, respPath, ledger string) error {
	var req proxyreview.Request
	var resp proxyreview.Response
	if err := readJSONFile(reqPath, &req); err != nil {
		return err
	}
	if err := readJSONFile(respPath, &resp); err != nil {
		return err
	}
	entry := proxyreview.LedgerEntry{ItemID: itemID, ProxyModel: resp.Model, ModelVersion: resp.ModelVersion, CallID: resp.CallID,
		StartedAt: resp.StartedAt, DecidedAt: resp.DecidedAt, DurationMs: resp.DurationMs, CostUSD: resp.CostUSD}
	entry.Authors(&req, resp.Model)
	fail := func(stage string, err error) error {
		entry.Outcome, entry.Stage, entry.Error = proxyreview.OutcomeFailed, stage, err.Error()
		if ledger != "" {
			if lerr := proxyreview.AppendLedger(ledger, entry); lerr != nil {
				return errors.Join(err, lerr)
			}
		}
		return err
	}
	if req.ItemID != itemID {
		return fail(proxyreview.StageVerdict, fmt.Errorf("request %s is for item %s, not %s", reqPath, req.ItemID, itemID))
	}
	snap, err := store.Load(c.ctx, knowledge.Query{})
	if err != nil {
		return err
	}
	rc, err := knowledge.AssembleContext(snap, itemID)
	if err != nil {
		return fail(proxyreview.StageVerdict, err)
	}
	d, v, err := proxyreview.Decision(rc, &req, &resp)
	if v != nil {
		entry.Verdict, entry.Confidence = v.Action, v.Confidence
	}
	if err != nil {
		return fail(proxyreview.StageVerdict, err)
	}
	outs, err := knowledge.NewQueue(store, time.Now).Decide(c.ctx, []domain.ReviewDecision{d})
	if err != nil {
		return fail(proxyreview.StageRecord, err)
	}
	o := outs[0]
	entry.Outcome, entry.DecisionID, entry.Action, entry.Labels = proxyreview.OutcomeDecided, o.Decision.ID, o.Decision.Action, o.Decision.Labels
	if o.Fact != nil {
		entry.ResultingFact, entry.FactLevel = o.Fact.ID, string(o.Fact.Level())
	}
	for _, f := range o.FollowUps {
		entry.FollowUps = append(entry.FollowUps, f.ID)
	}
	if ledger != "" {
		if err := proxyreview.AppendLedger(ledger, entry); err != nil {
			return err
		}
	}
	fmt.Fprintf(c.out, "decision %s (%s, proxy %s)\n", o.Decision.ID, o.Decision.Action, resp.Model)
	if o.Fact != nil {
		fmt.Fprintf(c.out, "fact %s at level %s\n", o.Fact.ID, o.Fact.Level())
	}
	if len(o.FollowUps) > 0 {
		fmt.Fprintf(c.out, "open aspects: %v; %d follow-up items\n", o.OpenAspects, len(o.FollowUps))
	}
	return nil
}

// knowledgeProxyReport prints the proxy run report (`ri knowledge proxy-report`).
func (c *cli) knowledgeProxyReport(args []string) error {
	fs := c.flags("knowledge proxy-report", "")
	dir := fs.String("dir", knowledge.DefaultDir, "knowledge directory")
	ledger := fs.String("ledger", "", "run ledger (JSONL) written by decide -proxy-ledger and scripts/proxy-review.sh")
	output := fs.String("o", "text", "output format: text|json")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	snap, err := c.loadSnapshot(*dir, "")
	if err != nil {
		return err
	}
	var entries []proxyreview.LedgerEntry
	if *ledger != "" {
		if entries, err = proxyreview.ReadLedger(*ledger); err != nil {
			return err
		}
	}
	r := proxyreview.BuildReport(snap, entries)
	switch *output {
	case "text":
		proxyreview.WriteReport(c.out, r)
		return nil
	case "json":
		return c.writeJSON(r)
	}
	return fmt.Errorf("%w: unknown output format %q (want text or json)", app.ErrUsage, *output)
}

func readJSONFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// knowledgeProxyAgreement compares human decisions in the real store with the
// proxy's shadow-pass decisions (`ri knowledge proxy-agreement`).
func (c *cli) knowledgeProxyAgreement(args []string) error {
	fs := c.flags("knowledge proxy-agreement", "")
	dir := fs.String("dir", knowledge.DefaultDir, "the real knowledge directory (human decisions)")
	shadow := fs.String("shadow", "docs/phase3/learning-loop/proxy-shadow/knowledge", "the shadow store (proxy decisions on the high items)")
	output := fs.String("o", "text", "output format: text|json")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	real, err := c.loadSnapshot(*dir, "")
	if err != nil {
		return err
	}
	sh, err := c.loadSnapshot(*shadow, "")
	if err != nil {
		return err
	}
	a := proxyreview.CompareShadow(real, sh)
	switch *output {
	case "text":
		proxyreview.WriteAgreement(c.out, a)
		return nil
	case "json":
		return c.writeJSON(a)
	}
	return fmt.Errorf("%w: unknown output format %q (want text or json)", app.ErrUsage, *output)
}
