package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/eval"
)

// invCompare prints the Phase 5 comparison tables (Markdown) for stored runs:
// spec is "dir,dir,…" with an optional "label=" before a dir; the first is
// the baseline. Only stored scores are read, nothing is run or rescored.
func (c *cli) invCompare(spec string) error {
	var runs []eval.ComparedRun
	for _, part := range splitList(spec) {
		label, dir := "", part
		if i := strings.Index(part, "="); i > 0 {
			label, dir = part[:i], part[i+1:]
		}
		meta, err := readInvRunMeta(dir)
		if err != nil {
			return err
		}
		raw, err := readInvScore(dir)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		var score eval.InvScore
		if err := json.Unmarshal(raw, &score); err != nil {
			return fmt.Errorf("%s: score: %w", dir, err)
		}
		if label == "" {
			label = meta.Arm
		}
		secs := 0
		for _, m := range meta.Cases {
			secs += m.Seconds
		}
		k := meta.KnowledgeDigest
		if k == "" {
			k = "(not recorded)"
		}
		runs = append(runs, eval.ComparedRun{Label: label, RunID: filepath.Base(dir), Arm: meta.Arm, Commit: meta.Commit, Knowledge: k, Seconds: secs, Score: &score})
	}
	if len(runs) == 0 {
		return fmt.Errorf("%w: -compare needs at least one stored run directory", app.ErrUsage)
	}
	eval.WriteInvComparison(c.out, runs)
	return nil
}
