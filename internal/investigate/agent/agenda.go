package agent

import (
	"context"
	"fmt"
	"sort"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Agenda rules (declared, deterministic; the rule id is
// AgendaCoverage.PriorityRule). The agenda is every in-scope item the
// deterministic pass did not already decide, so coverage does not depend on
// the model's diligence (commander decision 2026-10-16).
//
// "Decided" means a failing deterministic or observed validation: such an
// item is affected by rule R1 whatever the agent records (DESIGN.md §3.1),
// so the agent cannot change its class. A passing validation decides
// nothing alone — R2 still needs every concern answered — so an item with a
// pass stays on the agenda: a concern there is exactly what prevents an
// unsafe clear.
const (
	// Priority 0: where render silence proves nothing, and changes to the
	// deployed configuration nothing upstream explains.
	AgendaBehaviour           = "agenda:behaviour"            // manifestation controller-behaviour or unknown
	AgendaRenderedUnexplained = "agenda:rendered-unexplained" // scope:rendered-unexplained
	// Priority 1: a passing validation exists; a concern could block a clear.
	AgendaPassToScrutinise = "agenda:pass-to-scrutinise"
	// Priority 2: everything else undecided.
	AgendaUndecided = "agenda:undecided"
)

// Agenda returns the agent's agenda for a bundle and the deterministic
// pass's validations, in priority order (then change-record order). Every
// entry's Disposition is still empty.
func Agenda(b *domain.EvidenceBundle, validations []domain.ValidationRecord) []domain.AgendaCoverage {
	failed, passed := map[string]bool{}, map[string]bool{}
	for _, v := range validations {
		switch v.Outcome {
		case domain.ValidationFail:
			failed[v.ChangeItem] = true // deterministic or observed: both decide R1
		case domain.ValidationPass:
			passed[v.ChangeItem] = true
		}
	}
	var out []domain.AgendaCoverage
	for _, it := range b.Items() {
		if !it.InScope || failed[it.ID] {
			continue
		}
		e := domain.AgendaCoverage{ChangeItem: it.ID}
		switch {
		case it.Manifestation != domain.ManifestRenderedConfig:
			e.Priority, e.PriorityRule = 0, AgendaBehaviour
		case it.ScopeRule == "scope:rendered-unexplained":
			e.Priority, e.PriorityRule = 0, AgendaRenderedUnexplained
		case passed[it.ID]:
			e.Priority, e.PriorityRule = 1, AgendaPassToScrutinise
		default:
			e.Priority, e.PriorityRule = 2, AgendaUndecided
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out
}

// Agenda budget formula, agenda@v1 (n = agenda size): each field is the
// larger of the configured budget (defaults for unset fields) and
//
//	tool calls 12+6n, turns 10+3n, tokens 150 000+30 000n, bytes 1 MiB+192 KiB·n, seconds 300+60n,
//
// capped at 400 calls, 200 turns, 3 000 000 tokens, 16 MiB and 3600 s. A
// configured budget above the formula is kept: the formula only ever raises.
const AgendaBudgetRule = "agenda@v1"

// AgendaBudget scales a budget to an agenda of n items and states the rule.
func AgendaBudget(configured domain.AgentBudget, n int) (domain.AgentBudget, string) {
	b := Budget(configured)
	raise := func(have *int, formula, max int) {
		if formula > max {
			formula = max
		}
		if formula > *have {
			*have = formula
		}
	}
	raise(&b.MaxToolCalls, 12+6*n, 400)
	raise(&b.MaxTurns, 10+3*n, 200)
	raise(&b.MaxTokens, 150_000+30_000*n, 3_000_000)
	raise(&b.MaxSeconds, 300+60*n, 3600)
	bytes := int64(1<<20) + int64(192<<10)*int64(n)
	if bytes > 16<<20 {
		bytes = 16 << 20
	}
	if bytes > b.MaxReadBytes {
		b.MaxReadBytes = bytes
	}
	rule := fmt.Sprintf("%s n=%d: max(configured, 12+6n calls, 10+3n turns, 150000+30000n tokens, 1MiB+192KiB·n bytes, 300+60n s), capped at 400 calls/200 turns/3000000 tokens/16MiB/3600 s → %d calls, %d turns, %d tokens, %d bytes, %d s",
		AgendaBudgetRule, n, b.MaxToolCalls, b.MaxTurns, b.MaxTokens, b.MaxReadBytes, b.MaxSeconds)
	return b, rule
}

// DeterministicPass runs every validator that applies over every in-scope
// item of the bundle (DESIGN.md §5.1, the pass before the agent) and returns
// the records (deduplicated by id, in item × validator order) with the
// artifacts and evidence they produced. A validator error aborts: validators
// report undecidable inputs as records, so an error is an invalid input or
// an infrastructure failure. The runner stores the output in the run record
// and passes it to the agent stage as InvestigateInput.Validations /
// ValidationArtifacts / ValidationEvidence.
func DeterministicPass(ctx context.Context, validators []investigate.Validator, base investigate.ValidationInput) (*investigate.ValidationOutput, error) {
	if base.Bundle == nil {
		return nil, fmt.Errorf("agent: deterministic pass: no bundle")
	}
	out := &investigate.ValidationOutput{}
	seenRec, seenArt, seenEv := map[string]bool{}, map[string]bool{}, map[domain.EvidenceID]bool{}
	for _, it := range base.Bundle.Items() {
		if !it.InScope {
			continue
		}
		for _, v := range validators {
			if !v.Applies(it.Family) {
				continue
			}
			in := base
			in.ChangeItem, in.Subjects, in.Paths, in.RenderedChanges, in.Request = it.ID, nil, nil, nil, nil
			vo, err := v.Validate(ctx, in)
			if err != nil {
				return nil, fmt.Errorf("agent: deterministic pass: %s on %s: %w", v.Name(), it.ID, err)
			}
			if vo == nil {
				continue
			}
			for _, r := range vo.Records {
				if !seenRec[r.ID] {
					seenRec[r.ID] = true
					out.Records = append(out.Records, r)
				}
			}
			for _, a := range vo.Artifacts {
				if !seenArt[a.ID] {
					seenArt[a.ID] = true
					out.Artifacts = append(out.Artifacts, a)
				}
			}
			for _, e := range vo.Evidence {
				if !seenEv[e.ID] {
					seenEv[e.ID] = true
					out.Evidence = append(out.Evidence, e)
				}
			}
		}
	}
	return out, nil
}

// ItemAgenda is the agenda in per-item session order (prompt feature
// perItem; commander decision 2026-10-16): manifestation
// controller-behaviour/unknown first, then scope:rendered-unexplained, then
// the rest; ties by item id. The membership and rules are Agenda's.
func ItemAgenda(b *domain.EvidenceBundle, validations []domain.ValidationRecord) []domain.AgendaCoverage {
	out := Agenda(b, validations)
	tier := func(e domain.AgendaCoverage) int {
		switch e.PriorityRule {
		case AgendaBehaviour:
			return 0
		case AgendaRenderedUnexplained:
			return 1
		}
		return 2
	}
	for i := range out {
		out[i].Priority = tier(out[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ChangeItem < out[j].ChangeItem
	})
	return out
}

// DefaultItemCap bounds the per-item sessions of one investigation.
const DefaultItemCap = 10

// ItemBudgetRule names the per-item session budget, item@v1: the configured
// AgentConfig.ItemBudget, its unset fields taken from the item defaults
// (16 tool calls, 14 turns, 150 000 tokens, 1 MiB returned, 360 s). The
// investigation-wide AgentConfig.Budget is for one long session and does not
// apply to per-item sessions.
const ItemBudgetRule = "item@v1"

// ItemDefaultBudget is the default per-item session budget.
var ItemDefaultBudget = domain.AgentBudget{MaxTurns: 14, MaxToolCalls: 16, MaxTokens: 150_000, MaxReadBytes: 1 << 20, MaxSeconds: 360}

// ItemBudgetV2Rule names the per-item budget of prompt versions that set
// their own item defaults (meta.json itemCalls / itemTurns; v9: 24 calls,
// 18 turns), the rest as item@v1.
const ItemBudgetV2Rule = "item@v2"

// ItemBudget returns the budget of one per-item session and states the rule.
func ItemBudget(configured *domain.AgentBudget, cap, agenda int) (domain.AgentBudget, string) {
	return itemBudget(ItemDefaultBudget, configured, cap, agenda)
}

// ItemBudgetFor is ItemBudget with the item defaults a prompt version sets
// (Features.ItemCalls / ItemTurns); without them it is ItemBudget, rule text
// included.
func ItemBudgetFor(f Features, configured *domain.AgentBudget, cap, agenda int) (domain.AgentBudget, string) {
	if f.ItemCalls == 0 && f.ItemTurns == 0 {
		return ItemBudget(configured, cap, agenda)
	}
	d := ItemDefaultBudget
	if f.ItemCalls > 0 {
		d.MaxToolCalls = f.ItemCalls
	}
	if f.ItemTurns > 0 {
		d.MaxTurns = f.ItemTurns
	}
	b, _ := itemBudget(d, configured, cap, agenda)
	sessions := agenda
	if sessions > cap {
		sessions = cap
	}
	rule := fmt.Sprintf("%s: per item %d calls, %d turns, %d tokens, %d bytes, %d s (prompt item defaults %d/%d/150000/1MiB/360 s unless AgentConfig.ItemBudget sets them); cap %d items: %d of %d agenda items get a session",
		ItemBudgetV2Rule, b.MaxToolCalls, b.MaxTurns, b.MaxTokens, b.MaxReadBytes, b.MaxSeconds, d.MaxToolCalls, d.MaxTurns, cap, sessions, agenda)
	return b, rule
}

func itemBudget(defaults domain.AgentBudget, configured *domain.AgentBudget, cap, agenda int) (domain.AgentBudget, string) {
	b := defaults
	if c := configured; c != nil {
		if c.MaxTurns > 0 {
			b.MaxTurns = c.MaxTurns
		}
		if c.MaxToolCalls > 0 {
			b.MaxToolCalls = c.MaxToolCalls
		}
		if c.MaxTokens > 0 {
			b.MaxTokens = c.MaxTokens
		}
		if c.MaxReadBytes > 0 {
			b.MaxReadBytes = c.MaxReadBytes
		}
		if c.MaxSeconds > 0 {
			b.MaxSeconds = c.MaxSeconds
		}
	}
	sessions := agenda
	if sessions > cap {
		sessions = cap
	}
	rule := fmt.Sprintf("%s: per item %d calls, %d turns, %d tokens, %d bytes, %d s (item defaults 16/14/150000/1MiB/360 s unless AgentConfig.ItemBudget sets them); cap %d items: %d of %d agenda items get a session",
		ItemBudgetRule, b.MaxToolCalls, b.MaxTurns, b.MaxTokens, b.MaxReadBytes, b.MaxSeconds, cap, sessions, agenda)
	return b, rule
}
