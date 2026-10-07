package policy

import (
	"fmt"
	"io"
	"strings"
)

// TextOptions tunes WriteText.
type TextOptions struct {
	// ShowAll lists auto-pass items too (default: only the items that keep the
	// upgrade out of auto-pass, plus counts).
	ShowAll bool
}

// WriteText renders the verdict as the report's "Upgrade policy" section.
func WriteText(w io.Writer, v *Verdict, opts TextOptions) {
	if v == nil {
		return
	}
	fmt.Fprintf(w, "Upgrade policy %q (%s): %s\n", v.Policy, shortDigest(v.Digest), strings.ToUpper(string(v.Tier)))
	fmt.Fprintf(w, "  %d auto-pass, %d review, %d block", v.Counts[AutoPass], v.Counts[Review], v.Counts[Block])
	c := v.Covered
	if c.RoutineChanges+c.DeferredChanges+c.ClearedFindings > 0 {
		fmt.Fprintf(w, "; also covered without a row: %d routine upstream change(s), %d change(s) decided through findings, %d finding(s) cleared as not-affected",
			c.RoutineChanges, c.DeferredChanges, c.ClearedFindings)
	}
	fmt.Fprintln(w)
	if v.Tier == AutoPass {
		fmt.Fprintln(w, "  Every rendered change, upstream change and finding is covered by an auto-pass rule.")
	} else {
		fmt.Fprintf(w, "  Why not auto-pass — the strictest tier present is %s:\n", v.Tier)
		for _, g := range v.Why {
			inv := ""
			if g.Invariant != "" {
				inv = " [safety invariant: " + g.Invariant + "]"
			}
			fmt.Fprintf(w, "    %d × %s%s — %s\n", g.Count, g.Rule, inv, g.Reason)
		}
	}
	for _, it := range v.Items {
		if it.Tier == AutoPass && !opts.ShowAll {
			continue
		}
		inv := ""
		if it.Invariant != "" && it.PolicyTier != it.Tier {
			inv = fmt.Sprintf(" (policy said %s; raised by safety invariant %s)", it.PolicyTier, it.Invariant)
		} else if it.Invariant != "" {
			inv = fmt.Sprintf(" (safety invariant %s applies)", it.Invariant)
		}
		fmt.Fprintf(w, "  [%s] %s: %s — rule %s%s\n", it.Tier, it.Subject, it.Ref, it.Rule, inv)
		if len(it.Evidence) > 0 {
			fmt.Fprintf(w, "        evidence: %s\n", strings.Join(it.Evidence, ", "))
		}
	}
	if !opts.ShowAll && v.Counts[AutoPass] > 0 {
		fmt.Fprintf(w, "  (%d auto-pass item(s) not listed; use --tier-policy-all)\n", v.Counts[AutoPass])
	}
	for _, n := range v.Notes {
		fmt.Fprintf(w, "  note: %s\n", n)
	}
}

func shortDigest(d string) string {
	if len(d) > 19 {
		return d[:19]
	}
	return d
}
