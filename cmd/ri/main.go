// Command ri is the Release Intelligence CLI.
//
//	ri products                         list product definitions
//	ri validate [product|file.yaml...]  statically validate definitions
//	ri versions <product>               list canonical releases
//	ri check <product> [-n N | -versions a,b,c]
//	                                    validate declared release↔artifact
//	                                    relationships against history
//	ri ingest <product> <version>       ingest one release (facts + evidence)
//	ri upgrade <product> <from> <to>    describe the upgrade edge
//	ri discover <repository>            propose a product definition
//
// Global flags (before the command): -products DIR, -state DIR, -offline,
// -refresh, -v.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/app"
)

const usage = `ri — software release intelligence (prototype)

Usage:
  ri [global flags] <command> [flags] [args]

Commands:
  products                          list product definitions
  validate [product|file.yaml ...]  statically validate product definitions
  versions <product>                list canonical releases and source status
  check <product>                   validate declared relationships against historical releases
  ingest <product> <version>        ingest one release and print its facts
  upgrade <product> <from> <to>     describe everything relevant to upgrading from → to
  discover <repository>             inspect an upstream repository and propose a definition

Global flags:
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		if errors.Is(err, app.ErrUsage) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

type globals struct {
	products string
	state    string
	offline  bool
	refresh  bool
	verbose  bool
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var g globals
	fs := flag.NewFlagSet("ri", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&g.products, "products", envOr("RI_PRODUCTS", "products"), "directory of product definitions")
	fs.StringVar(&g.state, "state", envOr("RI_STATE", ".ri"), "local state directory (cache + store)")
	fs.BoolVar(&g.offline, "offline", false, "use only cached upstream data (reproducible replay)")
	fs.BoolVar(&g.refresh, "refresh", false, "ignore caches and stored ingestions")
	fs.BoolVar(&g.verbose, "v", false, "log progress to stderr")
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return fmt.Errorf("%w: missing command", app.ErrUsage)
	}
	cmd, cmdArgs := rest[0], rest[1:]
	if cmd == "help" {
		fs.Usage()
		return nil
	}
	newApp := func() (*app.App, error) {
		logf := func(string, ...any) {}
		if g.verbose {
			logf = func(f string, a ...any) { fmt.Fprintf(stderr, "· "+f+"\n", a...) }
		}
		return app.New(app.Config{
			ProductsDir: g.products,
			StateDir:    g.state,
			Offline:     g.offline,
			Refresh:     g.refresh,
			GitHubToken: app.GitHubTokenFromEnv(),
			Logf:        logf,
		})
	}
	c := &cli{ctx: ctx, out: stdout, err: stderr, newApp: newApp, g: g}
	switch cmd {
	case "products":
		return c.products(cmdArgs)
	case "validate":
		return c.validate(cmdArgs)
	case "versions":
		return c.versions(cmdArgs)
	case "check":
		return c.check(cmdArgs)
	case "ingest":
		return c.ingest(cmdArgs)
	case "upgrade":
		return c.upgrade(cmdArgs)
	case "discover":
		return c.discover(cmdArgs)
	default:
		fs.Usage()
		return fmt.Errorf("%w: unknown command %q", app.ErrUsage, cmd)
	}
}

type cli struct {
	ctx    context.Context
	out    io.Writer
	err    io.Writer
	newApp func() (*app.App, error)
	g      globals
}

func (c *cli) flags(name, argsUsage string) *flag.FlagSet {
	fs := flag.NewFlagSet("ri "+name, flag.ContinueOnError)
	fs.SetOutput(c.err)
	fs.Usage = func() {
		fmt.Fprintf(c.err, "Usage: ri %s [flags] %s\n", name, argsUsage)
		fs.PrintDefaults()
	}
	return fs
}

func (c *cli) writeJSON(v any) error {
	enc := json.NewEncoder(c.out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// parse parses flags that may appear before, between or after positional
// arguments ("ri check cert-manager -n 4"). A literal "--" ends flag parsing.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		if len(args) > 0 && len(rest) < len(args) && args[len(args)-len(rest)-1] == "--" {
			return append(pos, rest...), nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
