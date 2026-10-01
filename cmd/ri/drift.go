package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/drift"
)

// drift re-validates the product definition's declared relationships against
// the NEWEST upstream releases (newer than the validated baseline) and
// reports structured drift events with a proposed, annotated definition
// fragment. It never writes to products/: proposals go to stdout or -out.
func (c *cli) drift(args []string) error {
	fs := c.flags("drift", "<product>")
	n := fs.Int("n", 3, "number of newest releases newer than the baseline to re-validate")
	versions := fs.String("versions", "", "comma-separated releases to check instead of -n")
	baseline := fs.String("baseline", "", `saved "ri check -o json" report used as the baseline (default: docs/onboarding/checks/<id>.json when present; "-" for none)`)
	output := fs.String("o", "text", "output format: text|json")
	out := fs.String("out", "", "write the YAML proposal to this file instead of stdout")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		fs.Usage()
		return fmt.Errorf("%w: expected <product>", app.ErrUsage)
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	rep, err := a.Drift(c.ctx, pos[0], app.DriftOptions{
		N:            *n,
		Versions:     splitList(*versions),
		BaselinePath: *baseline,
	})
	if err != nil {
		return err
	}
	proposal := drift.RenderProposal(rep)
	if *output == "json" {
		return c.writeJSON(rep)
	}
	if err := drift.RenderText(c.out, rep); err != nil {
		return err
	}
	if *out != "" {
		if err := writeProposal(c.g.products, *out, proposal); err != nil {
			return err
		}
	} else if proposal != "" {
		fmt.Fprintln(c.out, proposal)
	}
	if rep.Summary.Drift > 0 {
		return fmt.Errorf("%d drift event(s) for %s (unverifiable events are not drift)", rep.Summary.Drift, rep.Product)
	}
	return nil
}

// writeProposal writes the proposal to path, refusing anything inside the
// products directory: definitions are only ever edited by hand.
func writeProposal(productsDir, path, proposal string) error {
	if proposal == "" {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		if pd, err := filepath.Abs(productsDir); err == nil {
			if rel, err := filepath.Rel(pd, abs); err == nil && !strings.HasPrefix(rel, "..") {
				return fmt.Errorf("refusing to write the proposal into the products directory (%s): proposals must be reviewed, not applied", path)
			}
		}
	}
	return os.WriteFile(path, []byte(proposal), 0o644)
}
