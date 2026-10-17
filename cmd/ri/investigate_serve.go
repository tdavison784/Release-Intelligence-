package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
	"github.com/tdavison784/release-intelligence/internal/investigate/reportui"
)

func init() { investigateSubcommands["serve"] = (*cli).investigateServe }

// investigateServe is `ri investigate serve`: the per-upgrade investigation
// view alone (`ri review serve` mounts the same pages under /investigations).
// It binds to localhost by default; there is no authentication, so do not
// expose it.
func (c *cli) investigateServe(args []string) error {
	fs := c.flags("investigate serve", "")
	addr := fs.String("addr", "127.0.0.1:8486", "listen address")
	dir := fs.String("store", "", "investigations to serve: a state directory (default: the global -state; reviews are recorded), or stored evaluation runs (eval/investigations/results, or one run directory; read-only)")
	reviewer := fs.String("reviewer", "", "pre-fill the reviewer name")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return fmt.Errorf("%w: unexpected arguments %v", app.ErrUsage, pos)
	}
	src, err := c.reportSource(*dir)
	if err != nil {
		return err
	}
	h := reportui.NewServer(src, reportui.Options{Reviewer: classify.NewReviewApplier(), DefaultReviewer: *reviewer})
	mux := http.NewServeMux()
	mux.Handle("/investigations", h)
	mux.Handle("/investigations/", h)
	mux.Handle("/{$}", http.RedirectHandler("/investigations", http.StatusSeeOther))
	return c.serveHTTP(*addr, mux, fmt.Sprintf("investigations on http://%%s/investigations (%s; Ctrl-C to stop)", src.Describe()))
}

// reportSource opens the investigations the report pages serve (default: the
// investigation store in the global state directory).
func (c *cli) reportSource(dir string) (reportui.Source, error) {
	if dir == "" {
		return reportui.NewStoreSource(c.g.state)
	}
	return reportui.Open(dir)
}

// serveHTTP listens on addr and serves h until the command's context ends.
// banner is printed with the bound address in place of %s.
func (c *cli) serveHTTP(addr string, h http.Handler, banner string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, banner+"\n", ln.Addr())
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
