package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/discovery"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// discover inspects an upstream repository, proposes a ProductDefinition and
// validates it against historical releases with the real ingestion pipeline.
func (c *cli) discover(args []string) error {
	fs := c.flags("discover", "<repository>")
	ref := fs.String("ref", "", "git ref to scan (default: latest stable tag)")
	id := fs.String("id", "", "product id for the proposed definition (default: repository name)")
	name := fs.String("name", "", "display name for the proposed definition")
	local := fs.String("dir", "", "scan an existing local checkout instead of cloning")
	noFollow := fs.Bool("no-follow", false, "do not scan referenced documentation repositories")
	useLLM := fs.Bool("llm", false, "resolve ambiguities with an LLM (needs ANTHROPIC_API_KEY); AI proposals only enter the definition when validated")
	model := fs.String("model", "", "LLM model override")
	out := fs.String("out", "", "write the proposed definition to this file (default: stdout)")
	report := fs.String("report", "", "write the full markdown discovery report to this file")
	noValidate := fs.Bool("no-validate", false, "skip validation against historical releases")
	releases := fs.Int("releases", 4, "number of historical releases to validate against (minimum 3)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		fs.Usage()
		return fmt.Errorf("%w: expected <repository>", app.ErrUsage)
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	d := &discovery.Discoverer{
		Checkout:           discovery.NewGitCheckout(filepath.Join(c.g.state, "discovery")),
		ValidationReleases: *releases,
		Model:              *model,
	}
	d.Tags = d.Checkout.(*discovery.GitCheckout)
	if !*noValidate {
		d.Checker = a.Ingester // real relationship checks against upstream releases
	}
	if *useLLM {
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return fmt.Errorf("-llm requires ANTHROPIC_API_KEY")
		}
		d.LLM = llm.NewAnthropic(key)
	}
	res, err := d.Run(c.ctx, discovery.Request{
		Repository: pos[0], Ref: *ref, ProductID: *id, Name: *name,
		LocalDir: *local, NoFollow: *noFollow, NoLLM: !*useLLM,
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(c.err, res.Report.Summary())
	if *report != "" {
		if err := os.WriteFile(*report, []byte(res.Report.Markdown()), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(c.err, "report written to %s\n", *report)
	}
	if *out != "" {
		if err := os.WriteFile(*out, res.YAML, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(c.err, "proposed definition written to %s\n", *out)
		return nil
	}
	_, err = c.out.Write(res.YAML)
	return err
}
