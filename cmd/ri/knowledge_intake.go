package main

import (
	"fmt"
	"os"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// knowledgeIntakeRule is `ri knowledge intake-rule -dir DIR -author TEXT
// <rule.json>`: a human-authored rule file becomes a proposal and a
// whole-fact review item (never a fact), gated and verified exactly like an
// exported agent hypothesis (internal/knowledge/intake.go). Idempotent.
func (c *cli) knowledgeIntakeRule(args []string) error {
	fs := c.flags("knowledge intake-rule", "<rule.json>")
	dir := fs.String("dir", "", "knowledge directory to write the proposal and review item into (required; use a copy, not the live knowledge/ tree, for experiments)")
	author := fs.String("author", "", "who wrote the rule, recorded verbatim (required)")
	cand := fs.String("candidate", "", "knowledge candidate to anchor the rule on (default: the one inside the bounds holding every cited evidence)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *dir == "" || *author == "" {
		fs.Usage()
		return fmt.Errorf("%w: give the rule file, -dir and -author", app.ErrUsage)
	}
	data, err := os.ReadFile(pos[0])
	if err != nil {
		return err
	}
	r, err := knowledge.ParseRuleFile(data)
	if err != nil {
		return err
	}
	res, err := knowledge.Intake(c.ctx, c.knowledgeStore(*dir), r, knowledge.IntakeOptions{Author: *author, Candidate: *cand, Now: time.Now()})
	if err != nil {
		return err
	}
	res.File = pos[0]
	return c.writeJSON(res)
}
