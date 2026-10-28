package policy

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/render"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// A security regression (the target is affected, the current version is
// not) is never treated less strictly than an advisory affecting both:
// same tier and same invariant under the default policy and under a policy
// that waves everything through, with or without a finding on the change,
// and even when the change is marked routine.
func TestAdvisoryRegressionAtLeastAsStrictAsAffectsTarget(t *testing.T) {
	yolo, err := Parse([]byte(autoPassEverything))
	if err != nil {
		t.Fatal(err)
	}
	good := pair(imageChange("a/b:1.0.0", "a/b:1.0.1"))
	mk := func(rule string, routine bool) domain.Change {
		c := chg("a", domain.CategorySecurity)
		c.Provenance.Rule, c.ActionRequired, c.Routine = rule, true, routine
		return c
	}
	reports := map[string]*domain.ImpactReport{
		"no finding":            nil,
		"informational finding": report(finding("f", "a", domain.ImpactInformational, "impact:x")),
		"unknown finding":       report(finding("f", "a", domain.ImpactUnknown, "impact:not-joined")),
		"not-affected finding":  report(finding("f", "a", domain.ImpactNotAffected, "impact:x")),
	}
	for _, p := range []*Policy{Default(), yolo} {
		for name, rep := range reports {
			for _, routine := range []bool{false, true} {
				eval := func(rule string) *Verdict {
					return Evaluate(p, Input{Pairs: []*render.Pair{good}, Edge: edge(mk(rule, routine)), Report: rep})
				}
				reg, aff := eval(upgrade.RuleAdvisoryRegression), eval(upgrade.RuleAdvisoryAffected)
				if reg.Tier != aff.Tier || reg.Tier == AutoPass {
					t.Errorf("%s/%s/routine=%v: regression tier %s, affects-target %s", p.Name, name, routine, reg.Tier, aff.Tier)
				}
				has := func(v *Verdict) bool {
					for _, it := range v.Items {
						if it.Invariant == InvAdvisory {
							return true
						}
					}
					return false
				}
				if !has(reg) || has(reg) != has(aff) {
					t.Errorf("%s/%s/routine=%v: invariant %s regression=%v affects-target=%v", p.Name, name, routine, InvAdvisory, has(reg), has(aff))
				}
			}
		}
	}
}
