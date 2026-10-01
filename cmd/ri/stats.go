package main

import (
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/stats"
)

// stats measures how product onboarding scales: definition size and
// complexity, constructs introduced and reused, onboarding effort, discovery
// share and relationship validation, per product in onboarding order and per
// wave. It reads the definitions (global -products), the onboarding records
// and the saved relationship reports; it never touches the network.
func (c *cli) stats(args []string) error {
	fs := c.flags("stats", "[product ...]")
	output := fs.String("o", "text", "output format: text|json|markdown")
	records := fs.String("records", stats.DefaultRecordsDir, "directory of onboarding records")
	checks := fs.String("checks", stats.DefaultChecksDir, "directory of saved `ri check -o json` reports")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	switch *output {
	case stats.FormatText, stats.FormatJSON, stats.FormatMarkdown:
	default:
		return fmt.Errorf("%w: unknown output format %q (want text, json or markdown)", app.ErrUsage, *output)
	}
	in, err := stats.Collect(c.g.products, *records, *checks)
	if err != nil {
		return err
	}
	return stats.Render(c.out, stats.Build(in).Filter(pos), *output)
}
