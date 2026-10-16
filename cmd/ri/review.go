package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/reviewui"
)

// review dispatches `ri review <subcommand>`.
func (c *cli) review(args []string) error {
	if len(args) > 0 && args[0] == "open-audits" {
		return c.reviewOpenAudits(args[1:])
	}
	if len(args) == 0 || args[0] != "serve" {
		return fmt.Errorf("%w: usage: ri review serve [-addr host:port] [-demo | -knowledge DIR] | ri review open-audits -knowledge DIR [-apply]", app.ErrUsage)
	}
	return c.reviewServe(args[1:])
}

// reviewServe runs the engineering review UI (MISSION G7–G10). It binds to
// localhost by default; there is no authentication (a non-goal), so do not
// expose it.
func (c *cli) reviewServe(args []string) error {
	fs := c.flags("review serve", "")
	addr := fs.String("addr", "127.0.0.1:8484", "listen address")
	demo := fs.Bool("demo", false, "serve built-in fixture items (an in-memory queue; nothing is persisted)")
	dir := fs.String("knowledge", "", "knowledge store directory (the learning-loop store)")
	limit := fs.Int("limit", 0, "cap the inbox rows (default 200; try -limit 3 with -demo to see the truncation notice)")
	reviewer := fs.String("reviewer", "", "pre-fill the reviewer name")
	invDir := fs.String("investigations", "eval/investigations", "investigation case directory: its products' items that block investigations are listed first")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return fmt.Errorf("%w: unexpected arguments %v", app.ErrUsage, pos)
	}
	if *demo == (*dir != "") {
		return fmt.Errorf("%w: choose exactly one of -demo or -knowledge DIR", app.ErrUsage)
	}
	var (
		q    knowledge.Queue
		snap func(context.Context) (*knowledge.Snapshot, error)
	)
	source := "demo fixtures; nothing is persisted"
	if *dir != "" {
		if st, err := os.Stat(*dir); err == nil && !st.IsDir() { // a missing directory is an empty store
			return fmt.Errorf("-knowledge %s: not a directory", *dir)
		}
		st := knowledge.NewFileStore(*dir)
		q = knowledge.NewQueue(st, nil)
		snap = func(ctx context.Context) (*knowledge.Snapshot, error) { return st.Load(ctx, knowledge.Query{}) }
		source = "knowledge store " + *dir
	} else {
		q = reviewui.NewDemoQueue()
	}
	scope := reviewui.ScopeFromInvestigations(*invDir, c.g.products)
	h := reviewui.NewServer(q, reviewui.Options{DefaultReviewer: *reviewer, Demo: *demo, InboxLimit: *limit, Snapshot: snap, Scope: scope})
	var waiting string
	if snap != nil {
		if sn, err := snap(c.ctx); err == nil {
			pins, blocking, others := reviewui.CountViews(sn, scope)
			waiting = fmt.Sprintf("%d pin-source item(s) need your verification (listed); %d other blocking and %d other open item(s) are one link away", pins, blocking, others)
			if n := len(reviewui.UnreviewedPinFacts(sn, scope)); n > 0 {
				waiting += fmt.Sprintf("\n%d pin-source fact(s) hold only consensus/proxy verification and have no review item: run `ri review open-audits -knowledge %s -apply` to open them", n, *dir)
			}
		}
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "review UI on http://%s (%s; Ctrl-C to stop)\n", ln.Addr(), source)
	if waiting != "" {
		fmt.Fprintf(c.out, "%s\n", waiting)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-c.ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// reviewOpenAudits opens fact-review items for the pin-source facts that
// investigations need but no human can decide yet (consensus auto-approval
// left them without a review item). Dry run unless -apply: it writes review
// items into the knowledge store.
func (c *cli) reviewOpenAudits(args []string) error {
	fs := c.flags("review open-audits", "")
	dir := fs.String("knowledge", knowledge.DefaultDir, "knowledge store directory")
	invDir := fs.String("investigations", "eval/investigations", "investigation case directory (the investigated products and upgrades)")
	apply := fs.Bool("apply", false, "write the review items (default: list what would be opened)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	st := knowledge.NewFileStore(*dir)
	snap, err := st.Load(c.ctx, knowledge.Query{})
	if err != nil {
		return err
	}
	facts := reviewui.UnreviewedPinFacts(snap, reviewui.ScopeFromInvestigations(*invDir, c.g.products))
	for _, f := range facts {
		fmt.Fprintf(c.out, "%s %s %s: %s\n", f.ID, f.Product, f.Release, f.Assertion.Statement)
		if *apply {
			if _, err := knowledge.OpenFactReview(c.ctx, st, f.ID, time.Now()); err != nil {
				return err
			}
		}
	}
	verb := "would open"
	if *apply {
		verb = "opened"
	}
	fmt.Fprintf(c.out, "%s %d fact-review item(s)\n", verb, len(facts))
	return nil
}
