package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/verify/semantic"
)

func init() { investigateSubcommands["semcheck"] = (*cli).investigateSemcheck }

// semcheckResult is the report-only semantic check of one stored run.
type semcheckResult struct {
	Key           string                    `json:"key"`
	Investigation string                    `json:"investigation"`
	Checks        []semantic.BehaviourCheck `json:"checks"`
}

// investigateSemcheck is `ri investigate semcheck <key>...`: the report-only
// semantic check (Milestone 4, lane semverify) of every agent behaviour entry
// sourced upstream in stored runs: supported, unsupported or undetermined
// against the passages its citations quote. It reads only; it never changes
// a classification or a record.
func (c *cli) investigateSemcheck(args []string) error {
	fs := c.flags("investigate semcheck", "<key>...")
	dir := fs.String("store", "", "where the runs are: a state directory (default: the global -state), or stored evaluation runs (eval/investigations/results, or one run directory; keys <run-id>/<case>)")
	asJSON := fs.Bool("json", false, "print the checks as JSON")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		fs.Usage()
		return fmt.Errorf("%w: name at least one investigation key", app.ErrUsage)
	}
	src, err := c.reportSource(*dir)
	if err != nil {
		return err
	}
	var out []semcheckResult
	for _, key := range pos {
		rec, err := src.Load(c.ctx, key)
		if err != nil {
			return err
		}
		pool := map[domain.EvidenceID]domain.Evidence{}
		if rec.Bundle != nil {
			for _, e := range rec.Bundle.Evidence {
				pool[e.ID] = e
			}
		}
		for _, e := range rec.Investigation.Evidence {
			pool[e.ID] = e
		}
		out = append(out, semcheckResult{Key: key, Investigation: rec.Investigation.ID,
			Checks: semantic.CheckBehaviours(rec.Investigation.AgentAssessments, pool)})
	}
	if *asJSON {
		enc := json.NewEncoder(c.out)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	for _, r := range out {
		fmt.Fprintf(c.out, "%s (%s): %d upstream behaviour entries (report-only; never a classification input)\n", r.Key, r.Investigation, len(r.Checks))
		for _, bc := range r.Checks {
			fmt.Fprintf(c.out, "  %-12s %s  [%s]\n", bc.Verdict, bc.Behaviour, bc.ChangeItem)
			fmt.Fprintf(c.out, "               %s\n", bc.Detail)
			ids := make([]string, len(bc.Citations))
			for i, id := range bc.Citations {
				ids[i] = string(id)
			}
			fmt.Fprintf(c.out, "               cites %s", strings.Join(ids, ", "))
			if len(bc.Unresolved) > 0 {
				fmt.Fprintf(c.out, " (%d not in the run's evidence)", len(bc.Unresolved))
			}
			fmt.Fprintln(c.out)
		}
	}
	return nil
}
