package classify

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/policy"
)

// decide is §3.4: the investigation is auto-approve-eligible exactly when
// every in-scope item is eligible (not-affected) AND the agent stage
// completed AND the traversal is complete AND at least one complete
// environment render exists, every environment render target of the product
// is complete, and no environment rendered change is uncorrelated AND — when
// a tier policy is configured — its verdict is auto-pass. Automated approval
// is never enacted in milestone 1 (AutoApprovalEnabled stays false): an
// eligible investigation still waits for a human, and the eligibility is
// recorded for measurement.
func (x *index) decide(findings []domain.InvestigationFinding) domain.PolicyDecision {
	var blockers, ok []string

	var notEligible []string
	for _, f := range findings {
		if !f.Approval.Eligible {
			notEligible = append(notEligible, fmt.Sprintf("%s (%s)", f.ChangeItem, f.Class))
		}
	}
	switch {
	case len(findings) == 0:
		blockers = append(blockers, "no in-scope change item was classified: nothing supports an automatic safe determination")
	case len(notEligible) > 0:
		blockers = append(blockers, fmt.Sprintf("%d of %d in-scope change item(s) are not approval-eligible: %s", len(notEligible), len(findings), strings.Join(notEligible, ", ")))
	default:
		ok = append(ok, fmt.Sprintf("all %d in-scope change item(s) are not-affected (validated)", len(findings)))
	}

	switch a := x.in.Agent; {
	case a == nil || a.Run.Status == "":
		blockers = append(blockers, "the investigation agent stage did not run (automated approval requires a completed agent stage)")
	case a.Run.Status != domain.AgentSubmitted:
		blockers = append(blockers, fmt.Sprintf("the investigation agent stage did not complete (%s: %s)", a.Run.Status, orDash(a.Run.Reason)))
	default:
		ok = append(ok, "the agent stage completed (submitted)")
	}

	switch t := x.in.Traversal; {
	case t == nil:
		blockers = append(blockers, "no relationship traversal ran")
	case !t.Complete:
		why := fmt.Sprintf("%d unresolved edge(s)", len(t.Unresolved))
		if t.Truncated {
			why = "truncated: " + t.TruncationReason
		}
		blockers = append(blockers, "the traversal is not complete ("+why+")")
	default:
		ok = append(ok, "the traversal is complete")
	}

	complete, incomplete := 0, []string{}
	for _, p := range x.b.Renders {
		switch p.Status {
		case domain.CollectionOK:
			complete++
		case domain.CollectionNotApplicable:
		default:
			incomplete = append(incomplete, fmt.Sprintf("%s (%s)", p.Target, p.Status))
		}
	}
	switch {
	case complete == 0:
		blockers = append(blockers, "no complete environment render exists: absence of change is only evidence from a render")
	case len(incomplete) > 0:
		blockers = append(blockers, "environment render(s) not complete: "+strings.Join(incomplete, ", "))
	default:
		ok = append(ok, fmt.Sprintf("%d complete environment render(s)", complete))
	}
	if un := x.b.Uncorrelated(); len(un) > 0 {
		blockers = append(blockers, fmt.Sprintf("%d environment rendered change(s) no change item explains: %s", len(un), strings.Join(un, ", ")))
	} else if complete > 0 {
		ok = append(ok, "every environment rendered change is correlated to a change item")
	}

	cfgTier, tv := x.in.Config.TierPolicy, x.in.Tier
	switch {
	case cfgTier != "" && tv == nil:
		blockers = append(blockers, "a tier policy is configured ("+cfgTier+") but its verdict is missing")
	case tv != nil && cfgTier != "" && tv.Digest != cfgTier:
		blockers = append(blockers, fmt.Sprintf("the tier verdict is for policy %s, but the configured tier policy is %s", tv.Digest, cfgTier))
	case tv != nil && tv.Tier != string(policy.AutoPass):
		why := ""
		if len(tv.Why) > 0 {
			why = ": " + strings.Join(tv.Why, "; ")
		}
		blockers = append(blockers, fmt.Sprintf("the tier policy %s verdict is %s, not auto-pass%s", tv.Policy, tv.Tier, why))
	case tv != nil:
		ok = append(ok, "the tier policy "+tv.Policy+" verdict is auto-pass")
	default:
		ok = append(ok, "no tier policy is configured")
	}

	d := domain.PolicyDecision{Policy: x.policy}
	if len(blockers) == 0 {
		d.Outcome = domain.PolicyAutoApproveEligible
		d.Reasons = ok
	} else {
		d.Outcome = domain.PolicyHumanReview
		d.Reasons = blockers
	}
	shadow := "automated approval is shadow-only in milestone 1 (never enacted): a human reviews and approves"
	if x.in.Config.AutoApproval {
		shadow += "; Config.AutoApproval was requested but stays disabled until the pre-registered gates allow it (DESIGN.md §3.4, §6.3)"
	}
	d.Reasons = append(d.Reasons, shadow)
	d.AutoApprovalEnabled = false
	return d
}

// TierVerdict reduces a tier policy verdict (docs/POLICY.md) to the
// classifier's input. nil in, nil out.
func TierVerdict(v *policy.Verdict) *investigate.TierVerdict {
	if v == nil {
		return nil
	}
	out := &investigate.TierVerdict{Policy: v.Policy, Digest: v.Digest, Tier: string(v.Tier)}
	for _, g := range v.Why {
		if g.Tier == policy.AutoPass {
			continue
		}
		out.Why = append(out.Why, fmt.Sprintf("%s ×%d (%s): %s", g.Rule, g.Count, g.Tier, g.Reason))
	}
	return out
}
