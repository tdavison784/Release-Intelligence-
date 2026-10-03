package knowledge

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// IsProseOnlyCorrection reports whether d is a correction that changes only the
// consequence statement/remediation: every aspect digest is unchanged (digests
// ignore prose) but the prose differs (DESIGN §2.5, contract-5).
func IsProseOnlyCorrection(d domain.ReviewDecision) bool {
	if d.Action != domain.ActionCorrect || d.Original == nil || d.Corrected == nil ||
		d.Original.Digest() != d.Corrected.Digest() {
		return false
	}
	return consequenceProse(d.Original) != consequenceProse(d.Corrected)
}

func consequenceProse(a *domain.SemanticAssertion) string {
	if a == nil || a.Consequence == nil {
		return ""
	}
	return a.Consequence.Statement + "\x00" + a.Consequence.Remediation
}

// levelOf maps a reviewer kind to the verification level its decisions give.
func levelOf(k domain.ReviewerKind) domain.VerificationLevel {
	if k == domain.ReviewerProxy {
		return domain.VerifiedProxy
	}
	return domain.VerifiedHuman
}

// Minted is what FactFromDecision concluded from one decision.
type Minted struct {
	// Fact is the minted or updated fact (nil when aspects remain open, the
	// decision verifies nothing, or the fact is retracted/superseded).
	Fact *domain.VerifiedFact
	// Open lists the aspects of the candidate still unverified.
	Open []domain.Aspect
	// FollowUps are the review items newly needed for the open aspects.
	FollowUps []domain.ReviewItem
	// Conflict is set when the aspects of the candidate are all verified but
	// cannot compose into a valid fact; the decision is still recorded.
	Conflict string
	// Touched are other facts the decision changed: retracted, superseded, or
	// given a restatement anchor.
	Touched []domain.VerifiedFact
}

// candidateState derives the verified aspects of one candidate from its
// validations and its accept/correct decisions. Decisions apply in time
// order, so a later decision overrides an earlier one on the aspects its
// question verifies, with two guards: a value a validator confirmed stays
// deterministic when a reviewer decides the same value, and a proxy never
// overrides an aspect a trusted level already holds.
//
// The accumulated aspects must also compose: aspects verified independently
// (on different items of the same candidate) can each be sound alone yet
// conflict as a tuple — e.g. a change the domain allows only on another
// subject family (migration-required on a gvk subject). Such a tuple cannot
// become a fact, and refusing the decision would lose the reviewer's verdict
// on a question they did answer. The state is instead repaired: the least
// trusted non-trusted aspects go back to open (see repairState), so follow-up
// review re-asks them coherently against the surviving aspects.
func candidateState(seed map[domain.Aspect]aspectState, vs []domain.ValidationResult, items map[string]domain.ReviewItem, ds []domain.ReviewDecision) map[domain.Aspect]aspectState {
	validated := stateFromValidations(vs)
	state := map[domain.Aspect]aspectState{}
	for x, v := range seed {
		state[x] = v
	}
	for x, v := range validated {
		if cur, ok := state[x]; ok && cur.digest != v.digest {
			continue // a re-reviewed fact keeps its value unless a decision changes it
		}
		state[x] = v
	}
	ordered := append([]domain.ReviewDecision(nil), ds...)
	sort.Slice(ordered, func(i, j int) bool {
		return lessTimeID(ordered[i].DecidedAt.UnixNano(), ordered[i].ID, ordered[j].DecidedAt.UnixNano(), ordered[j].ID)
	})
	for _, d := range ordered {
		if d.Action != domain.ActionAccept && d.Action != domain.ActionCorrect {
			continue
		}
		item, ok := items[d.ReviewItemID]
		if !ok || item.Status == domain.ReviewSuperseded {
			continue
		}
		fin := d.Final()
		if fin == nil {
			continue
		}
		for _, x := range item.Aspects() {
			if !fin.Has(x) {
				continue
			}
			dg := fin.AspectDigest(x)
			if v, ok := validated[x]; ok && v.digest == dg {
				state[x] = v
				continue
			}
			lvl := levelOf(d.ReviewerKind)
			if cur, ok := state[x]; ok {
				if cur.digest == dg && levelNotWeaker(cur.level, lvl) {
					// a prose-only correction keeps the verified value but takes the
					// corrected statement/remediation, when the aspect rests on a
					// decision of the same level (a proxy never rewrites a trusted
					// reviewer's prose; a validator-confirmed value has no prose to edit)
					if x == domain.AspectConsequence && IsProseOnlyCorrection(d) && cur.level == lvl &&
						(lvl == domain.VerifiedHuman || lvl == domain.VerifiedProxy) {
						cur.part = partOf(*fin, x)
						cur.basis = appendUnique(append([]string(nil), cur.basis...), d.ID)
						state[x] = cur
					}
					continue
				}
				if cur.level.Trusted() && lvl == domain.VerifiedProxy {
					continue
				}
			}
			state[x] = aspectState{part: partOf(*fin, x), digest: dg, level: lvl, basis: []string{d.ID}}
		}
	}
	repairState(state)
	return state
}

// composeValid reports whether the verified aspects, minus drop, compose into
// an assertion the domain accepts. Each aspect may be sound on its own while
// the tuple is not (a change the subject family forbids, a condition the
// subject cannot carry), because the aspects were verified by different items.
func composeValid(state map[domain.Aspect]aspectState, drop ...domain.Aspect) bool {
	skip := map[domain.Aspect]bool{}
	for _, x := range drop {
		skip[x] = true
	}
	var a domain.SemanticAssertion
	for _, x := range domain.Aspects {
		if skip[x] {
			continue
		}
		if st, ok := state[x]; ok {
			mergeAspect(&a, st.part, x)
		}
	}
	return a.Validate(false) == nil
}

// dropRank orders verification levels for conflict repair: the larger, the
// more giveable (deterministic and human are trusted and never dropped).
var dropRank = map[domain.VerificationLevel]int{
	domain.VerifiedDeterministic: 0,
	domain.VerifiedHuman:         0,
	domain.VerifiedConsensus:     1,
	domain.VerifiedProxy:         2,
}

func aspectIndex(x domain.Aspect) int {
	for i, a := range domain.Aspects {
		if a == x {
			return i
		}
	}
	return -1
}

// repairState reopens aspects when the accumulated tuple does not validate:
// it drops the aspects repairDrops chooses. Trusted (deterministic/human)
// aspects are never dropped — a conflict between them is an inconsistency
// that must surface through buildFact's error, not be silently re-asked.
func repairState(state map[domain.Aspect]aspectState) {
	if composeValid(state) {
		return
	}
	for _, x := range repairDrops(state) {
		delete(state, x)
	}
}

// repairDrops chooses the aspects to give up when the verified tuple does not
// validate: the smallest set of non-trusted aspects whose removal restores a
// valid composition. Among equally small sets it drops the least trusted (the
// larger sum of dropRank), and, still tied, the latest aspects of
// domain.Aspects (the subject is the fact's identity). nil when no non-trusted
// subset helps. domain.Aspects is small and fixed, so the subset scan is
// bounded by it.
func repairDrops(state map[domain.Aspect]aspectState) []domain.Aspect {
	var droppable []domain.Aspect
	for _, x := range domain.Aspects {
		if st, ok := state[x]; ok && !st.level.Trusted() {
			droppable = append(droppable, x)
		}
	}
	type dropSet struct {
		xs   []domain.Aspect
		rank int // sum of dropRank: larger = the dropped aspects were less trusted
		late int // sum of aspect indexes: larger = later aspects dropped
	}
	var valid []dropSet
	for mask := 1; mask < 1<<len(droppable); mask++ {
		var ds dropSet
		for i, x := range droppable {
			if mask&(1<<i) == 0 {
				continue
			}
			ds.xs = append(ds.xs, x)
			ds.rank += dropRank[state[x].level]
			ds.late += aspectIndex(x)
		}
		if composeValid(state, ds.xs...) {
			valid = append(valid, ds)
		}
	}
	sort.Slice(valid, func(i, j int) bool {
		a, b := valid[i], valid[j]
		if len(a.xs) != len(b.xs) {
			return len(a.xs) < len(b.xs)
		}
		if a.rank != b.rank {
			return a.rank > b.rank
		}
		return a.late > b.late
	})
	if len(valid) == 0 {
		return nil
	}
	return valid[0].xs
}

// buildFact composes a fact from fully verified aspect state. The evidence is
// what the verified claim rests on: the validators' artifact evidence plus the
// candidate evidence cited by the proposals that agree with the verified
// value on at least one aspect (all candidate evidence when none does).
func buildFact(c domain.SemanticCandidate, state map[domain.Aspect]aspectState, ps []domain.SemanticProposal, vs []domain.ValidationResult, now time.Time) (*domain.VerifiedFact, error) {
	var a domain.SemanticAssertion
	var ver []domain.AspectVerification
	basisVal := map[string]bool{}
	for _, x := range domain.Aspects {
		st, ok := state[x]
		if !ok {
			return nil, fmt.Errorf("fact: aspect %s is not verified", x)
		}
		mergeAspect(&a, st.part, x)
		av := domain.AspectVerification{Aspect: x, Level: st.level, Basis: append([]string(nil), st.basis...)}
		if st.level == domain.VerifiedConsensus {
			// CONTRACT-CHANGE(contract-3): PO-1 labels consensus cross-model / same-model.
			av.Consensus = consensusScope(st.basis, ps)
		}
		ver = append(ver, av)
		for _, id := range st.basis {
			basisVal[id] = true
		}
	}
	a.Statement = statementOf(c, ps, a)
	cited := map[domain.EvidenceID]bool{}
	for _, p := range ps {
		for _, x := range domain.Aspects {
			if p.Assertion.Has(x) && p.Assertion.AspectDigest(x) == a.AspectDigest(x) {
				for _, id := range p.Citations {
					cited[id] = true
				}
				break
			}
		}
	}
	var ev []domain.Evidence
	seen := map[domain.EvidenceID]bool{}
	add := func(e domain.Evidence) {
		if !seen[e.ID] {
			seen[e.ID] = true
			ev = append(ev, e)
		}
	}
	for _, e := range c.Evidence {
		if cited[e.ID] {
			add(e)
		}
	}
	for _, v := range vs {
		if basisVal[v.ID] {
			for _, e := range v.Evidence {
				add(e)
			}
		}
	}
	if len(ev) == 0 {
		for _, e := range c.Evidence {
			add(e)
		}
	}
	auto := true
	for _, v := range ver {
		auto = auto && (v.Level == domain.VerifiedDeterministic || v.Level == domain.VerifiedConsensus)
	}
	f := &domain.VerifiedFact{
		AutoApproved: auto,
		ID:           domain.VerifiedFactID(c.Product, c.Release, a),
		Product:      c.Product,
		Release:      c.Release,
		Anchors:      c.Anchors(),
		Candidates:   []string{c.ID},
		Assertion:    a,
		Verification: ver,
		Evidence:     ev,
		Status:       domain.FactActive,
		CreatedAt:    now,
	}
	f.ConsensusAction = consensusAction(*f, ps, vs)
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return f, nil
}

// consensusAction decides PO-2: the fact may produce ACTION REQUIRED by model
// consensus when every aspect is at consensus or better (and at least one is
// consensus), the consequence is action-eligible, every agreeing proposal on a
// consensus-verified consequence requested action-required, and no validator
// refuted any aspect of it.
func consensusAction(f domain.VerifiedFact, ps []domain.SemanticProposal, vs []domain.ValidationResult) bool {
	if f.Level() != domain.VerifiedConsensus || f.Assertion.Consequence == nil || !f.Assertion.Consequence.Kind.ActionEligible() {
		return false
	}
	if f.AspectLevel(domain.AspectConsequence) == domain.VerifiedConsensus {
		byID := map[string]domain.SemanticProposal{}
		for _, p := range ps {
			byID[p.ID] = p
		}
		for _, v := range f.Verification {
			if v.Aspect != domain.AspectConsequence {
				continue
			}
			for _, id := range v.Basis {
				if strings.HasPrefix(id, domain.ProposalIDPrefix) {
					if p, ok := byID[id]; !ok || p.SuggestedClass != domain.ImpactActionRequired {
						return false
					}
				}
			}
		}
	}
	for _, v := range vs {
		for _, x := range domain.Aspects {
			if v.Assertion.AspectDigest(x) == f.Assertion.AspectDigest(x) && refutesAspect(v, x) {
				return false
			}
		}
	}
	return true
}

func refutesAspect(v domain.ValidationResult, x domain.Aspect) bool {
	for _, c := range v.Checks {
		if c.Aspect == x && c.Outcome == domain.OutcomeRefuted {
			return true
		}
	}
	return false
}

// factReviewTarget returns the id of the fact an item re-reviews: an item
// whose proposed assertion is complete and is exactly an existing fact.
func factReviewTarget(it domain.ReviewItem, facts map[string]domain.VerifiedFact) (string, bool) {
	if it.Proposed.Validate(true) != nil {
		return "", false
	}
	id := domain.VerifiedFactID(it.Product, it.Release, it.Proposed)
	_, ok := facts[id]
	return id, ok
}

// FactFromDecision applies one decision to the snapshot and says what it
// produced (DESIGN.md §3). snap must hold the decision's review item, its
// candidate and everything known about it; the decision itself may or may not
// already be in snap. Nothing is written.
//
//   - accept/correct: verify the aspects the item's question covers (a
//     correction keeps Original and Corrected both on the decision); when all
//     four aspects of the candidate are then verified, mint the fact (or merge
//     into the existing fact of that identity: anchors, candidates, and any
//     verification upgrade); otherwise list the open aspects and the follow-up
//     items. A correction of a fact-review item supersedes the old fact.
//   - reject: a negative example; on a fact-review item it retracts the fact;
//     with the duplicate label it adds this candidate's anchors to the named fact.
//   - need-more-evidence / defer: no fact.
func FactFromDecision(snap *Snapshot, d domain.ReviewDecision) (*Minted, error) {
	items := map[string]domain.ReviewItem{}
	for _, it := range snap.ReviewItems {
		items[it.ID] = it
	}
	item, ok := items[d.ReviewItemID]
	if !ok {
		return nil, fmt.Errorf("%w: review item %s", ErrNotFound, d.ReviewItemID)
	}
	var cand *domain.SemanticCandidate
	for i := range snap.Candidates {
		if snap.Candidates[i].ID == item.CandidateID {
			cand = &snap.Candidates[i]
		}
	}
	if cand == nil {
		return nil, fmt.Errorf("%w: candidate %s of item %s", ErrOrphan, item.CandidateID, item.ID)
	}
	facts := map[string]domain.VerifiedFact{}
	for _, f := range snap.Facts {
		facts[f.ID] = f
	}
	out := &Minted{}
	switch d.Action {
	case domain.ActionNeedMoreEvidence, domain.ActionDefer:
		return out, nil
	case domain.ActionReject:
		if target, ok := factReviewTarget(item, facts); ok {
			f := facts[target]
			f.Status = domain.FactRetracted
			out.Touched = append(out.Touched, f)
		}
		if d.DuplicateOf != "" {
			f, ok := facts[d.DuplicateOf]
			if !ok {
				return nil, fmt.Errorf("%w: duplicate of fact %s", ErrNotFound, d.DuplicateOf)
			}
			if f.Product != cand.Product {
				return nil, fmt.Errorf("decision %s: fact %s belongs to %s, not %s", d.ID, f.ID, f.Product, cand.Product)
			}
			for _, an := range cand.Anchors() {
				if an.Release != f.Release {
					return nil, fmt.Errorf("decision %s: candidate release %q differs from fact release %q", d.ID, an.Release, f.Release)
				}
				f.Anchors = addAnchor(f.Anchors, an)
			}
			f.Candidates = appendUnique(f.Candidates, cand.ID)
			out.Touched = append(out.Touched, f)
		}
		return out, nil
	}

	// accept / correct
	var vs []domain.ValidationResult
	for _, v := range snap.Validations {
		if v.CandidateID == cand.ID {
			vs = append(vs, v)
		}
	}
	var ps []domain.SemanticProposal
	for _, p := range snap.Proposals {
		if p.CandidateID == cand.ID {
			ps = append(ps, p)
		}
	}
	candItems := map[string]domain.ReviewItem{}
	for id, it := range items {
		if it.CandidateID == cand.ID {
			candItems[id] = it
		}
	}
	var ds []domain.ReviewDecision
	have := false
	for _, x := range snap.Decisions {
		if _, ok := candItems[x.ReviewItemID]; ok {
			ds = append(ds, x)
			have = have || x.ID == d.ID
		}
	}
	if !have {
		ds = append(ds, d)
	}
	var seed map[domain.Aspect]aspectState
	target, isReview := factReviewTarget(item, facts)
	if isReview {
		seed = seedFromFact(facts[target])
	}
	state := candidateState(seed, vs, candItems, ds)
	for _, x := range domain.Aspects {
		if _, ok := state[x]; !ok {
			out.Open = append(out.Open, x)
		}
	}
	if len(out.Open) > 0 {
		now := d.DecidedAt
		for _, it := range buildItems(*cand, ps, vs, state, now) {
			if prev, exists := items[it.ID]; !exists || prev.Status == domain.ReviewSuperseded {
				it.Context = item.Context // the same reviewer session saw the same environment
				out.FollowUps = append(out.FollowUps, it)
			}
		}
		return out, nil
	}
	f, err := buildFact(*cand, state, ps, vs, d.DecidedAt)
	if err != nil {
		// Every aspect is verified yet they do not compose (repairState only
		// reopens non-trusted aspects, so this is a conflict between trusted
		// ones, e.g. two validators confirming incompatible parts). The
		// reviewer's verdict stands and is recorded; no fact is assembled, and
		// the conflict is reported so a person can resolve it.
		out.Conflict = fmt.Sprintf("decision %s: verified aspects do not compose into a fact: %v", d.ID, err)
		return out, nil
	}
	if old, exists := facts[f.ID]; exists {
		if old.Status != domain.FactActive {
			return out, nil // a retracted or superseded fact is never revived
		}
		merged := old
		for _, an := range f.Anchors {
			merged.Anchors = addAnchor(merged.Anchors, an)
		}
		merged.Candidates = appendUnique(merged.Candidates, cand.ID)
		// the corrected consequence prose (same id: digests ignore prose)
		if oc, nc := merged.Assertion.Consequence, f.Assertion.Consequence; oc != nil && nc != nil &&
			(oc.Statement != nc.Statement || oc.Remediation != nc.Remediation) && state[domain.AspectConsequence].level != domain.VerifiedDeterministic &&
			state[domain.AspectConsequence].level != domain.VerifiedConsensus {
			c := *oc
			c.Statement, c.Remediation = nc.Statement, nc.Remediation
			merged.Assertion.Consequence = &c
		}
		var ver []domain.AspectVerification
		for _, v := range merged.Verification {
			if nv := f.Verification[verIndex(f.Verification, v.Aspect)]; levelNotWeaker(nv.Level, v.Level) {
				v = nv
			}
			ver = append(ver, v)
		}
		merged.Verification = ver
		merged.ConsensusAction = old.ConsensusAction && merged.Level() == domain.VerifiedConsensus
		// the auto-approved marker is history: it stays after a human audit
		// upgrades the aspects the reviewer verified. The audit itself is the
		// decision record (written before the fact), which is what the R19
		// agreement metric reads.
		auto := true
		for _, v := range ver {
			auto = auto && (v.Level == domain.VerifiedDeterministic || v.Level == domain.VerifiedConsensus)
		}
		merged.AutoApproved = old.AutoApproved || auto
		f = &merged
	}
	if target, ok := factReviewTarget(item, facts); ok && d.Action == domain.ActionCorrect && target != f.ID {
		f.Supersedes = appendUnique(f.Supersedes, target)
		old := facts[target]
		old.Status = domain.FactSuperseded
		out.Touched = append(out.Touched, old)
	}
	out.Fact = f
	return out, nil
}

// seedFromFact turns a fact's verification back into per-aspect state.
func seedFromFact(f domain.VerifiedFact) map[domain.Aspect]aspectState {
	out := map[domain.Aspect]aspectState{}
	for _, v := range f.Verification {
		out[v.Aspect] = aspectState{part: partOf(f.Assertion, v.Aspect), digest: f.Assertion.AspectDigest(v.Aspect), level: v.Level, basis: append([]string(nil), v.Basis...), consensus: v.Consensus}
	}
	return out
}

func verIndex(vs []domain.AspectVerification, x domain.Aspect) int {
	for i, v := range vs {
		if v.Aspect == x {
			return i
		}
	}
	return 0
}

func addAnchor(as []domain.ChangeAnchor, a domain.ChangeAnchor) []domain.ChangeAnchor {
	keys := map[string]bool{}
	for _, x := range as {
		for _, k := range x.StatementKeys {
			keys[k] = true
		}
	}
	for _, k := range a.StatementKeys {
		if keys[k] {
			return as
		}
	}
	return append(as, a)
}

// validateDecisions checks the decisions of one Decide call before anything is
// written: shape, the item they answer, the assertion they were shown, that a
// corrected assertion covers what the question verifies, and the batch.
func validateDecisions(snap *Snapshot, ds []domain.ReviewDecision) error {
	if len(ds) == 0 {
		return errors.New("knowledge: no decisions")
	}
	items := map[string]domain.ReviewItem{}
	for _, it := range snap.ReviewItems {
		items[it.ID] = it
	}
	var errs []error
	seenItem := map[string]bool{}
	batches := map[string][]domain.ReviewDecision{}
	for _, d := range ds {
		if err := d.Validate(); err != nil {
			errs = append(errs, err)
			continue
		}
		it, ok := items[d.ReviewItemID]
		if !ok {
			errs = append(errs, fmt.Errorf("%w: decision %s answers unknown review item %s", ErrNotFound, d.ID, d.ReviewItemID))
			continue
		}
		if seenItem[it.ID] {
			errs = append(errs, fmt.Errorf("knowledge: item %s is decided twice in one call", it.ID))
		}
		seenItem[it.ID] = true
		if it.Status == domain.ReviewSuperseded {
			errs = append(errs, fmt.Errorf("knowledge: item %s is superseded", it.ID))
		}
		if d.Original != nil && d.Original.Digest() != it.Proposed.Digest() {
			errs = append(errs, fmt.Errorf("knowledge: decision %s shows a different assertion than item %s proposes", d.ID, it.ID))
		}
		if d.Action == domain.ActionAccept || d.Action == domain.ActionCorrect {
			fin := d.Final()
			for _, x := range it.Aspects() {
				if fin == nil || !fin.Has(x) {
					errs = append(errs, fmt.Errorf("knowledge: decision %s: %s question needs the %s aspect", d.ID, it.QuestionType, x))
				}
			}
			if len(it.Aspects()) == 0 {
				errs = append(errs, fmt.Errorf("knowledge: decision %s: a %s item verifies nothing; reject, defer or need-more-evidence", d.ID, it.QuestionType))
			}
		}
		if d.BatchID != "" {
			batches[d.BatchID] = append(batches[d.BatchID], d)
		}
	}
	for id, b := range batches {
		if len(b) != b[0].BatchSize {
			errs = append(errs, fmt.Errorf("knowledge: batch %s declares %d decisions, call has %d", id, b[0].BatchSize, len(b)))
		}
		for _, d := range b {
			if d.Reviewer != b[0].Reviewer || d.BatchSize != b[0].BatchSize {
				errs = append(errs, fmt.Errorf("knowledge: batch %s mixes reviewers or sizes", id))
				break
			}
		}
	}
	return errors.Join(errs...)
}

// consensusScope labels the agreeing proposals of a consensus basis
// (same-model when they cannot be resolved: the conservative label).
func consensusScope(basis []string, ps []domain.SemanticProposal) domain.ConsensusScope {
	ids := map[string]bool{}
	for _, id := range basis {
		ids[id] = true
	}
	var agreeing []domain.SemanticProposal
	for _, p := range ps {
		if ids[p.ID] {
			agreeing = append(agreeing, p)
		}
	}
	return domain.ConsensusScopeOf(agreeing)
}
