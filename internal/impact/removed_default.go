package impact

// Removed values keys the environment relied on (lane defaults). A removed
// key the environment does not set is not "nothing about this change takes
// effect on you" when the source chart gave it a non-empty default: that
// default was in effect for the environment, and the target no longer has
// it — whatever it configured now behaves as the target's templates decide.
// The environment that leaves the key unset is exactly the exposed one.

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// removedDefault emits RuleValuesRemovedDefault (review-required) for a
// removed values change none of whose keys the environment sets, when at
// least one key's source default is non-empty, or not recorded although the
// change records the others' (a section enumeration truncated by the diff:
// an unknown default is never read as empty). It reports whether it decided
// the change; false leaves today's values-unset verdict: every default
// empty, or a change that records no source default at all (not one the
// values diff computed, so there is no default to have relied on).
func (b *builder) removedDefault(c domain.Change) bool {
	defaults := upgrade.FromDefaults(c)
	if len(defaults) == 0 {
		return false
	}
	var relied, lines []string
	for _, k := range c.Subjects {
		v, known := defaults[k]
		if known && upgrade.EmptyDefault(v) {
			continue
		}
		relied = append(relied, k)
		if known {
			lines = append(lines, fmt.Sprintf("%s (default %s)", k, shorten(v, 120)))
		} else {
			lines = append(lines, k+" (default not recorded: the change lists only part of the removed keys)")
		}
	}
	if len(relied) == 0 {
		return false
	}
	ix := make(map[domain.EvidenceID]domain.Evidence, len(b.env.Evidence))
	for _, e := range b.env.Evidence {
		ix[e.ID] = e
	}
	var matches []domain.ImpactMatch
	for _, k := range relied {
		ms, recs := valuesAbsence(b.env, ix, k)
		for _, rec := range recs {
			if _, ok := b.extraLocal[rec.ID]; !ok {
				b.extraLocal[rec.ID] = rec
				b.extraLocalOrder = append(b.extraLocalOrder, rec.ID)
			}
		}
		matches = append(matches, ms...)
	}
	if len(matches) == 0 {
		return false // no record of the values that were read: nothing to cite (cannot happen with values supplied)
	}
	fromTag, toTag := b.edge.From.String(), b.edge.To.String()
	title := fmt.Sprintf("You rely on the default of %s, which %s removes", codeList(relied, 3), toTag)
	detail := fmt.Sprintf("Your values do not set %s, so the default of %s was in effect for you:\n  - %s\n%s removes the key: what that default configured now behaves as %s's templates decide, which may differ from what you run today. Review what the default controlled before upgrading (a render of your configuration shows whether it changes your deployment).",
		strings.Join(relied, ", "), fromTag, strings.Join(lines, "\n  - "), toTag, toTag)
	b.add(RuleValuesRemovedDefault, domain.ImpactReviewRequired, domain.SeverityMedium, domain.ConfidenceMedium, title, detail, c, matches, c.Evidence...)
	return true
}
