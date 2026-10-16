package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Depth rules (prompt feature depthRules; commander decision 2026-10-16,
// generic, enforced here rather than asked of the model):
//
//   - D1: an explains assertion, and an explains / examined-no-concern
//     closing entry, is accepted only for an item the agent retrieved with
//     get_change_item earlier in the session, and only with at least one
//     citation that no record about another item shares. A citation is the
//     cited artifact plus the whitespace-normalised quote, so a changed
//     locator does not make a shared quote unique.
//   - D2: submit_findings is rejected while a raises-concern about a
//     controller-behaviour or unknown item has neither a recorded
//     request_validation for that item in this session nor a stated
//     noValidationReason.
//
// Both are checked when the record is made: a later record about another
// item does not retroactively invalidate an accepted one.
const (
	DepthRuleExamined = "depth:examined-after-get-change-item"
	DepthRuleUnique   = "depth:unshared-citation"
	DepthRuleConcern  = "depth:behaviour-concern-validated-or-reasoned"
)

// citeKey identifies a citation by what it quotes, not where it says it is.
func citeKey(e domain.Evidence) string {
	return e.SourceID + "\x00" + normalizeText(e.Excerpt)
}

func normalizeText(s string) string { return strings.Join(strings.Fields(s), " ") }

// cite registers the citations of a committed record about an item.
func (s *Session) cite(item string, evs []domain.Evidence) {
	for _, e := range evs {
		k := citeKey(e)
		if s.citers[k] == nil {
			s.citers[k] = map[string]bool{}
		}
		s.citers[k][item] = true
	}
}

// depthExplains enforces D1 for an explains-kind record about item. pending
// holds the citations of other entries of the same call (submit_findings).
func (s *Session) depthExplains(what, item string, evs []domain.Evidence, pending map[string]map[string]bool) error {
	if !s.cfg.Prompt.Features.DepthRules {
		return nil
	}
	if !s.examined[item] {
		return rejectf("%s: depth rule %s: an explanation or examined-no-concern of %s needs get_change_item on that item first", what, DepthRuleExamined, item)
	}
	sharedWith := map[string]bool{}
	for _, e := range evs {
		k := citeKey(e)
		others := false
		for _, set := range []map[string]bool{s.citers[k], pending[k]} {
			for it := range set {
				if it != item {
					others = true
					sharedWith[it] = true
				}
			}
		}
		if !others {
			return nil
		}
	}
	return rejectf("%s: depth rule %s: every citation for %s is also cited for another item (%s); cite at least one passage specific to this item",
		what, DepthRuleUnique, item, strings.Join(sortedKeys(sharedWith), ", "))
}

// depthConcerns enforces D2 over the session's and the call's raises-concern
// assertions.
func (s *Session) depthConcerns(made []domain.AgentAssertion) error {
	if !s.cfg.Prompt.Features.DepthRules {
		return nil
	}
	requested := map[string]bool{}
	for _, r := range s.valReqs {
		requested[r.ChangeItem] = true
	}
	var open []string
	for _, a := range append(append([]domain.AgentAssertion(nil), s.asserts...), made...) {
		it, ok := s.items[a.ChangeItem]
		if !ok || a.Stance != domain.StanceRaisesConcern || it.Manifestation == domain.ManifestRenderedConfig || (s.focus != "" && a.ChangeItem != s.focus) {
			continue
		}
		if !requested[a.ChangeItem] && strings.TrimSpace(a.NoValidationReason) == "" {
			open = append(open, fmt.Sprintf("%s (%s)", a.ID, a.ChangeItem))
		}
	}
	if len(open) > 0 {
		return rejectf("depth rule %s: each raises-concern on a controller-behaviour/unknown item needs a request_validation for the item or a noValidationReason; open: %s",
			DepthRuleConcern, strings.Join(open, ", "))
	}
	return nil
}

func (s *Session) noValidationReason(stanceIsConcern bool, reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return "", nil
	case !s.cfg.Prompt.Features.DepthRules:
		return "", rejectf("this prompt version (%s) takes no noValidationReason", s.cfg.Prompt.Version)
	case !stanceIsConcern:
		return "", rejectf("noValidationReason applies to raises-concern only")
	case len(reason) > MaxStatement:
		return "", rejectf("noValidationReason is %d bytes (max %d)", len(reason), MaxStatement)
	}
	return reason, nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
