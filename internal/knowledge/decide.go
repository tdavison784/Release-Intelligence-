package knowledge

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

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
					continue
				}
				if cur.level.Trusted() && lvl == domain.VerifiedProxy {
					continue
				}
			}
			state[x] = aspectState{part: partOf(*fin, x), digest: dg, level: lvl, basis: []string{d.ID}}
		}
	}
	return state
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
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return f, nil
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
				out.FollowUps = append(out.FollowUps, it)
			}
		}
		return out, nil
	}
	f, err := buildFact(*cand, state, ps, vs, d.DecidedAt)
	if err != nil {
		return nil, fmt.Errorf("decision %s: %w", d.ID, err)
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
		var ver []domain.AspectVerification
		for _, v := range merged.Verification {
			if nv := f.Verification[verIndex(f.Verification, v.Aspect)]; levelNotWeaker(nv.Level, v.Level) {
				v = nv
			}
			ver = append(ver, v)
		}
		merged.Verification = ver
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
		out[v.Aspect] = aspectState{part: partOf(f.Assertion, v.Aspect), digest: f.Assertion.AspectDigest(v.Aspect), level: v.Level, basis: append([]string(nil), v.Basis...)}
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
