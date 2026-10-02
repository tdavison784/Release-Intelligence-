package knowledge

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// aspectState is one aspect of a candidate that has been verified: the value,
// how it was verified and by which records.
type aspectState struct {
	part   domain.SemanticAssertion // only this aspect is set
	digest string
	level  domain.VerificationLevel
	basis  []string
	// consensus labels a consensus-level aspect cross-model or same-model (PO-1).
	consensus domain.ConsensusScope
}

// AutoApproval is what an AutoApprovePolicy returns: aspects it verifies
// without a reviewer, with the level and basis records that justify them.
type AutoApproval struct {
	Aspects map[domain.Aspect]AutoApprovedAspect
}

// AutoApprovedAspect is one aspect verified by policy.
type AutoApprovedAspect struct {
	Part  domain.SemanticAssertion // only the aspect is set
	Level domain.VerificationLevel
	Basis []string
	// Consensus labels a consensus-level aspect (PO-1).
	Consensus domain.ConsensusScope
}

// AutoApprovePolicy is the SEAM for automatic approval (e.g. render-verifiable
// classes whose aspects are all deterministic or consensus-verified). It sees
// what routing sees, after validations have been folded in, and may verify
// further aspects. When the aspects it returns, together with the confirmed
// validations, cover all four, routing mints the fact (route auto-verify)
// instead of queueing items. The consensus level and the auto-approved fact
// marker are not part of the contract yet (p3ll/contract-2); until they are,
// no policy is installed and routing behaves as DESIGN.md §6 states. Sampling
// auto-approved facts into human review is a queue concern added with the
// marker.
type AutoApprovePolicy func(c domain.SemanticCandidate, ps []domain.SemanticProposal, vs []domain.ValidationResult, agreement []AspectAgreement) *AutoApproval

// Route routes one candidate (DESIGN.md §6) with no auto-approval policy.
func Route(c domain.SemanticCandidate, ps []domain.SemanticProposal, vs []domain.ValidationResult) RouteResult {
	return RouteWith(nil, c, ps, vs)
}

// RouteWith routes one candidate under an optional auto-approval policy. It
// is a pure function: the result is deterministic in its inputs (timestamps
// come from the records themselves).
func RouteWith(policy AutoApprovePolicy, c domain.SemanticCandidate, ps []domain.SemanticProposal, vs []domain.ValidationResult) RouteResult {
	agreement := Agreements(ps)
	state := stateFromValidations(vs)
	if policy != nil {
		// a policy auto-approves a candidate or nothing: its aspects count only
		// when, with the confirmed validations, they cover all four
		// CONTRACT-CHANGE(render): the policy sees the candidate's effective
		// renderability — its own assessment, else that of an assertion a
		// render confirmed (domain.EffectiveRenderability); never a model
		// claim alone. Only the policy's view changes; the stored candidate
		// is untouched.
		pc := c
		pc.Renderability = domain.EffectiveRenderability(c, vs)
		if ap := policy(pc, ps, vs, agreement); ap != nil {
			withPolicy := map[domain.Aspect]aspectState{}
			for x, st := range state {
				withPolicy[x] = st
			}
			for x, a := range ap.Aspects {
				if _, done := withPolicy[x]; !done && a.Part.Has(x) {
					withPolicy[x] = aspectState{part: a.Part, digest: a.Part.AspectDigest(x), level: a.Level, basis: a.Basis, consensus: a.Consensus}
				}
			}
			if len(withPolicy) == len(domain.Aspects) {
				state = withPolicy
			}
		}
	}
	now := latestTime(c, ps, vs)
	res := RouteResult{Agreement: agreement}
	if len(state) == len(domain.Aspects) {
		if f, err := buildFact(c, state, ps, vs, now); err == nil {
			res.Fact = f
			return res
		}
		// an inconsistent composition is not auto-verifiable: fall through to review
		state = map[domain.Aspect]aspectState{}
	}
	res.ReviewItems = buildItems(c, ps, vs, state, now)
	return res
}

func latestTime(c domain.SemanticCandidate, ps []domain.SemanticProposal, vs []domain.ValidationResult) time.Time {
	t := c.CreatedAt
	for _, p := range ps {
		if g := p.Provenance.GeneratedAt; g != nil && g.After(t) {
			t = *g
		}
	}
	for _, v := range vs {
		if v.CheckedAt.After(t) {
			t = v.CheckedAt
		}
	}
	return t
}

func partOf(a domain.SemanticAssertion, x domain.Aspect) domain.SemanticAssertion {
	var p domain.SemanticAssertion
	switch x {
	case domain.AspectSubject:
		p.Subject = a.Subject
	case domain.AspectChange:
		p.Change = a.Change
	case domain.AspectApplicability:
		p.Applicability = a.Applicability
	case domain.AspectConsequence:
		p.Consequence = a.Consequence
	}
	return p
}

// stateFromValidations folds validator confirmations into per-aspect state.
// An aspect confirmed for two different values is in conflict and stays open.
func stateFromValidations(vs []domain.ValidationResult) map[domain.Aspect]aspectState {
	type conf struct {
		part domain.SemanticAssertion
		ids  []string
	}
	by := map[domain.Aspect]map[string]*conf{}
	for _, v := range vs {
		for _, ch := range v.Checks {
			if ch.Outcome != domain.OutcomeConfirmed {
				continue
			}
			d := v.Assertion.AspectDigest(ch.Aspect)
			if by[ch.Aspect] == nil {
				by[ch.Aspect] = map[string]*conf{}
			}
			if by[ch.Aspect][d] == nil {
				by[ch.Aspect][d] = &conf{part: partOf(v.Assertion, ch.Aspect)}
			}
			cf := by[ch.Aspect][d]
			cf.ids = appendUnique(cf.ids, v.ID)
		}
	}
	out := map[domain.Aspect]aspectState{}
	for x, m := range by {
		if len(m) != 1 {
			continue
		}
		for d, cf := range m {
			sort.Strings(cf.ids)
			out[x] = aspectState{part: cf.part, digest: d, level: domain.VerifiedDeterministic, basis: cf.ids}
		}
	}
	return out
}

func appendUnique(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}

// Agreements computes, per aspect, how proposals compare (digest → proposal
// ids) and who abstained.
func Agreements(ps []domain.SemanticProposal) []AspectAgreement {
	var out []AspectAgreement
	for _, x := range domain.Aspects {
		ag := AspectAgreement{Aspect: x, Groups: map[string][]string{}}
		for _, p := range ps {
			if p.Assertion.Has(x) {
				d := p.Assertion.AspectDigest(x)
				ag.Groups[d] = append(ag.Groups[d], p.ID)
			}
			for _, u := range p.Undetermined {
				if u == x {
					ag.Undetermined = append(ag.Undetermined, p.ID)
				}
			}
		}
		if len(ag.Groups) == 0 && len(ag.Undetermined) == 0 {
			continue
		}
		out = append(out, ag)
	}
	return out
}

func model(p domain.SemanticProposal) string { return p.Provenance.Model }

// callKey identifies the stateless model call behind a proposal. Agreement is
// counted across separate calls (PO-1: any models, including two calls of the
// same model); a proposal without a call id (not valid, but be safe) counts as its own call.
func callKey(p domain.SemanticProposal) string {
	if p.Provenance.CallID != "" {
		return p.Provenance.CallID
	}
	return p.ID
}

// signals derives the routing signals of the open aspects.
func signals(ps []domain.SemanticProposal, vs []domain.ValidationResult, open []domain.Aspect, confirmed bool) (sig []domain.RoutingSignal, agree, disagree, highImpact, allUndetermined bool) {
	models := map[string]bool{} // separate calls that asserted something
	asserted := false
	for _, p := range ps {
		if !p.Assertion.Empty() {
			models[callKey(p)] = true
			asserted = true
		}
		if c := p.Assertion.Consequence; c != nil && c.Kind.ActionEligible() {
			highImpact = true
		}
	}
	allUndetermined = !asserted
	refuted := false
	for _, v := range vs {
		for _, ch := range v.Checks {
			refuted = refuted || ch.Outcome == domain.OutcomeRefuted
		}
	}
	agree = true
	any := false
	for _, x := range open {
		groups := map[string]map[string]bool{} // digest → models
		for _, p := range ps {
			if p.Assertion.Has(x) {
				d := p.Assertion.AspectDigest(x)
				if groups[d] == nil {
					groups[d] = map[string]bool{}
				}
				groups[d][callKey(p)] = true
			}
		}
		if len(groups) == 0 {
			continue
		}
		any = true
		if len(groups) > 1 {
			disagree = true
		}
		if len(groups) == 1 {
			for _, m := range groups {
				if len(m) < 2 {
					agree = false
				}
			}
		}
	}
	agree = agree && any && !disagree
	switch {
	case allUndetermined:
		sig = append(sig, domain.SignalAllUndetermined)
	case agree:
		sig = append(sig, domain.SignalModelsAgree)
	case disagree || refuted:
		if disagree {
			sig = append(sig, domain.SignalModelsDisagree)
		}
	}
	if len(models) < 2 && !allUndetermined { // fewer than two separate calls answered
		sig = append(sig, domain.SignalSingleModel)
	}
	if confirmed {
		sig = append(sig, domain.SignalValidationConfirmed)
	}
	if refuted {
		sig = append(sig, domain.SignalValidationRefuted)
	}
	if highImpact {
		sig = append(sig, domain.SignalHighImpact)
	}
	return sig, agree, disagree || refuted, highImpact, allUndetermined
}

// priority applies the DESIGN.md §6 table.
func priority(agree, disagree, highImpact, validated bool) domain.ReviewPriority {
	switch {
	case highImpact:
		return domain.PriorityHigh
	case disagree:
		return domain.PriorityNormal
	case agree && validated:
		return domain.PriorityLow
	}
	return domain.PriorityNormal
}

// refutedDigests lists aspect digests a validator refuted.
func refutedDigests(vs []domain.ValidationResult) map[domain.Aspect]map[string]bool {
	out := map[domain.Aspect]map[string]bool{}
	for _, v := range vs {
		for _, ch := range v.Checks {
			if ch.Outcome == domain.OutcomeRefuted {
				if out[ch.Aspect] == nil {
					out[ch.Aspect] = map[string]bool{}
				}
				out[ch.Aspect][v.Assertion.AspectDigest(ch.Aspect)] = true
			}
		}
	}
	return out
}

// pluralityValue picks the proposed value for an aspect: the digest asserted
// by the most distinct models (ties: the smallest digest), ignoring values a
// validator refuted unless nothing else is proposed. ok is false when no
// proposal asserts the aspect.
func pluralityValue(ps []domain.SemanticProposal, x domain.Aspect, refuted map[domain.Aspect]map[string]bool) (part domain.SemanticAssertion, statement string, ok bool) {
	type g struct {
		models map[string]bool
		first  domain.SemanticProposal
	}
	groups := map[string]*g{}
	for _, p := range ps {
		if !p.Assertion.Has(x) {
			continue
		}
		d := p.Assertion.AspectDigest(x)
		if refuted[x][d] {
			continue
		}
		if groups[d] == nil {
			groups[d] = &g{models: map[string]bool{}, first: p}
		}
		groups[d].models[callKey(p)] = true
	}
	var best string
	for d, gr := range groups {
		if best == "" || len(gr.models) > len(groups[best].models) || (len(gr.models) == len(groups[best].models) && d < best) {
			best = d
		}
	}
	if best == "" {
		return part, "", false
	}
	p := groups[best].first
	return partOf(p.Assertion, x), p.Assertion.Statement, true
}

// questionFor builds the review question for a question type and proposal.
func questionFor(q domain.QuestionType, a domain.SemanticAssertion, c domain.SemanticCandidate) string {
	subj := "this change"
	if a.Subject != nil {
		subj = a.Subject.Key()
	}
	switch q {
	case domain.QuestionSemanticMapping:
		ch := ""
		if a.Change != nil {
			ch = " (" + string(a.Change.Type) + ")"
		}
		return fmt.Sprintf("Does %q mean a change to %s%s?", c.Title, subj, ch)
	case domain.QuestionApplicability:
		return fmt.Sprintf("For %q, which environments are exposed? Confirm or correct the exposure condition.", c.Title)
	case domain.QuestionConsequence:
		k := ""
		if a.Consequence != nil {
			k = " proposed: " + string(a.Consequence.Kind)
		}
		return fmt.Sprintf("If an exposed environment does nothing about %q, what happens, and is that review or action?%s", c.Title, k)
	case domain.QuestionClassification:
		return fmt.Sprintf("Is %q an action-eligible failure, a deprecation or a no-op?", c.Title)
	case domain.QuestionRelationship:
		return fmt.Sprintf("Does %q impose a cross-product requirement on %s? Confirm subject, change and exposure.", c.Title, subj)
	}
	return fmt.Sprintf("Is the cited evidence enough to decide what %q means?", c.Title)
}

type itemGroup struct {
	q       domain.QuestionType
	aspects []domain.Aspect
}

// groupsFor decides which questions the open aspects need.
func groupsFor(open map[domain.Aspect]bool, ps []domain.SemanticProposal) []itemGroup {
	var gs []itemGroup
	relationship := false
	for _, p := range ps {
		if p.Task == domain.TaskRelationship && !p.Assertion.Empty() {
			relationship = true
		}
	}
	appDone := false
	if open[domain.AspectSubject] || open[domain.AspectChange] {
		if relationship && open[domain.AspectApplicability] {
			gs = append(gs, itemGroup{domain.QuestionRelationship, domain.QuestionAspects(domain.QuestionRelationship)})
			appDone = true
		} else {
			gs = append(gs, itemGroup{domain.QuestionSemanticMapping, domain.QuestionAspects(domain.QuestionSemanticMapping)})
		}
	}
	if open[domain.AspectApplicability] && !appDone {
		gs = append(gs, itemGroup{domain.QuestionApplicability, domain.QuestionAspects(domain.QuestionApplicability)})
	}
	if open[domain.AspectConsequence] {
		gs = append(gs, itemGroup{domain.QuestionConsequence, domain.QuestionAspects(domain.QuestionConsequence)})
	}
	return gs
}

// buildItems creates the review items for the aspects not yet verified
// (state). Open aspects with no value anyone proposed become one
// evidence-sufficiency item on the missing-evidence route. Shared by Route
// and by the follow-ups of Queue.Decide.
func buildItems(c domain.SemanticCandidate, ps []domain.SemanticProposal, vs []domain.ValidationResult, state map[domain.Aspect]aspectState, now time.Time) []domain.ReviewItem {
	open := map[domain.Aspect]bool{}
	var openList []domain.Aspect
	for _, x := range domain.Aspects {
		if _, ok := state[x]; !ok {
			open[x] = true
			openList = append(openList, x)
		}
	}
	if len(open) == 0 {
		return nil
	}
	sig, agree, disagree, high, allUnd := signals(ps, vs, openList, len(state) > 0)
	refuted := refutedDigests(vs)
	var items []domain.ReviewItem
	missing := false
	for _, g := range groupsFor(open, ps) {
		var proposed domain.SemanticAssertion
		complete := true
		for _, x := range g.aspects {
			var part domain.SemanticAssertion
			var stmt string
			ok := false
			if st, done := state[x]; done {
				part, ok = st.part, true
			} else {
				part, stmt, ok = pluralityValue(ps, x, refuted)
			}
			if !ok {
				complete = false
				break
			}
			mergeAspect(&proposed, part, x)
			if proposed.Statement == "" {
				proposed.Statement = stmt
			}
		}
		if !complete {
			missing = true
			continue
		}
		items = append(items, newItem(c, g.q, proposed, domain.Routing{
			Route:    domain.RouteReview,
			Priority: priority(agree, disagree, high, len(state) > 0),
			Signals:  sig,
		}, ps, vs, now))
	}
	if missing || (len(items) == 0 && allUnd) {
		items = append(items, newItem(c, domain.QuestionEvidenceSufficiency, domain.SemanticAssertion{}, domain.Routing{
			Route:    domain.RouteMissingEvidence,
			Priority: domain.PriorityLow,
			Signals:  sig,
		}, ps, vs, now))
	}
	return items
}

func mergeAspect(dst *domain.SemanticAssertion, part domain.SemanticAssertion, x domain.Aspect) {
	switch x {
	case domain.AspectSubject:
		dst.Subject = part.Subject
	case domain.AspectChange:
		dst.Change = part.Change
	case domain.AspectApplicability:
		dst.Applicability = part.Applicability
	case domain.AspectConsequence:
		dst.Consequence = part.Consequence
	}
}

func newItem(c domain.SemanticCandidate, q domain.QuestionType, proposed domain.SemanticAssertion, r domain.Routing, ps []domain.SemanticProposal, vs []domain.ValidationResult, now time.Time) domain.ReviewItem {
	it := domain.ReviewItem{
		ID:           domain.ReviewItemID(c.ID, q, proposed),
		CandidateID:  c.ID,
		Product:      c.Product,
		Release:      c.Release,
		QuestionType: q,
		Question:     questionFor(q, proposed, c),
		Proposed:     proposed,
		Routing:      r,
		Status:       domain.ReviewPending,
		CreatedAt:    now,
	}
	for _, p := range ps {
		it.Proposals = append(it.Proposals, p.ID)
	}
	for _, v := range vs {
		it.Validations = append(it.Validations, v.ID)
	}
	sort.Strings(it.Proposals)
	sort.Strings(it.Validations)
	it.Routing.Signals = dedupeSignals(it.Routing.Signals)
	return it
}

func dedupeSignals(s []domain.RoutingSignal) []domain.RoutingSignal {
	seen := map[domain.RoutingSignal]bool{}
	var out []domain.RoutingSignal
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// statementOf builds the fact's one-line statement.
func statementOf(c domain.SemanticCandidate, ps []domain.SemanticProposal, a domain.SemanticAssertion) string {
	if a.Statement != "" {
		return a.Statement
	}
	return strings.TrimSpace(c.Title)
}
