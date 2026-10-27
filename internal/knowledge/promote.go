package knowledge

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// The promotion gate (Phase 5 PROTOCOL §7.3, lane verify). An agent may
// propose a reusable rule (exported by `ri investigate export-hypotheses` as an
// AI proposal plus a whole-fact review item); only a human can promote it, and
// only with a confirmed, independent proof record for exactly the rule being
// promoted. Everything else — a proxy or agent reviewer, a partial
// verification, a missing, inconclusive or refuted proof, a correction that
// changes the rule, a proof whose bounds do not cover the item's release, an
// active fact on the same subject the reviewer did not account for — refuses
// the decision with a reason before anything is written.

// hypothesisRulePrefix starts the provenance rule of a proposal exported from
// an agent hypothesis: "hypothesis:<hyp-id>@<inv-id>" (lane verdict's
// domain.HypothesisRule writes it; this parser also accepts a rule without
// the investigation).
const hypothesisRulePrefix = domain.HypothesisRuleKind

// ParseHypothesisRule splits a proposal's provenance rule
// "hypothesis:<hyp-id>@<inv-id>"; ok is false for any other rule. A
// human-authored rule "human-authored:<hyp-id>" (intake.go) is a hypothesis
// without an investigation: every consumer — routing, the gate, verify, the
// review UI — treats it exactly like an exported agent hypothesis.
func ParseHypothesisRule(rule string) (hyp, inv string, ok bool) {
	if rest, found := strings.CutPrefix(rule, domain.HumanAuthoredRuleKind); found {
		if !strings.HasPrefix(rest, domain.HypothesisIDPrefix) || strings.Contains(rest, "@") {
			return "", "", false
		}
		return rest, "", true
	}
	rest, found := strings.CutPrefix(rule, hypothesisRulePrefix)
	if !found {
		return "", "", false
	}
	hyp, inv, _ = strings.Cut(rest, "@")
	if !strings.HasPrefix(hyp, domain.HypothesisIDPrefix) || (inv != "" && !strings.HasPrefix(inv, domain.InvestigationIDPrefix)) {
		return "", "", false
	}
	return hyp, inv, true
}

// HumanAuthored reports whether a proposal's provenance rule is a
// human-authored rule's (not an agent hypothesis).
func HumanAuthored(rule string) bool {
	_, _, ok := ParseHypothesisRule(rule)
	return ok && strings.HasPrefix(rule, domain.HumanAuthoredRuleKind)
}

// HypothesisSource names the agent hypothesis a review item comes from: a
// proposal the item lists, or any AI proposal of its candidate asserting the
// item's rule, whose provenance rule is a hypothesis rule. ok is false for an
// ordinary item. Matching proposals by assertion too means an item cannot
// escape the gate by not listing the hypothesis proposal.
func HypothesisSource(snap *Snapshot, it domain.ReviewItem) (hyp, inv string, ok bool) {
	listed := map[string]bool{}
	for _, id := range it.Proposals {
		listed[id] = true
	}
	digest := it.Proposed.Digest()
	for _, p := range snap.Proposals {
		if p.CandidateID != it.CandidateID {
			continue
		}
		if !listed[p.ID] && p.Assertion.Digest() != digest {
			continue
		}
		if h, i, ok := ParseHypothesisRule(p.Provenance.Rule); ok {
			return h, i, true
		}
	}
	return "", "", false
}

// GateState is what the promotion gate concludes for one rule of one item:
// whether a human accept/correct with that rule may promote it, and why not.
type GateState struct {
	Item          string
	Hypothesis    string
	Investigation string
	// RuleDigest is the digest of the rule the decision would promote.
	RuleDigest string
	// Proof is the confirmed proof that opens the gate (nil when none).
	Proof *domain.ProofRecord
	// Proofs are every proof of the item, newest last (for display).
	Proofs []domain.ProofRecord
	// Refuting are refuted proofs of the same rule (any item): a refutation
	// is never outweighed by a later confirmation.
	Refuting []string
	Open     bool
	Reason   string
}

// PromotionGate evaluates the gate for promoting rule through item (the
// item's own rule for an accept, the corrected rule for a correct). It
// reports the proof side only; the reviewer side (human, whole fact) and the
// coexistence rule are checked per decision (gateDecision).
func PromotionGate(snap *Snapshot, it domain.ReviewItem, rule domain.SemanticAssertion) GateState {
	g := GateState{Item: it.ID, RuleDigest: rule.Digest()}
	g.Hypothesis, g.Investigation, _ = HypothesisSource(snap, it)
	for _, p := range snap.Proofs {
		if p.ReviewItem == it.ID {
			g.Proofs = append(g.Proofs, p)
		}
		if p.Product == it.Product && p.ProposedRuleDigest == g.RuleDigest && p.Outcome == domain.ProofRefuted {
			g.Refuting = append(g.Refuting, p.ID)
		}
	}
	var latest *domain.ProofRecord
	for i := range g.Proofs {
		p := g.Proofs[i]
		if p.ProposedRuleDigest != g.RuleDigest {
			continue
		}
		if latest == nil || p.CheckedAt.After(latest.CheckedAt) || (p.CheckedAt.Equal(latest.CheckedAt) && p.ID > latest.ID) {
			latest = &g.Proofs[i]
		}
		if p.Outcome == domain.ProofConfirmed && g.Proof == nil {
			g.Proof = &g.Proofs[i]
		}
	}
	switch {
	case len(g.Refuting) > 0:
		g.Proof, g.Reason = nil, fmt.Sprintf("the rule was refuted by proof %s: a refuted rule cannot be promoted", strings.Join(g.Refuting, ", "))
	case g.Proof == nil && rule.Digest() != it.Proposed.Digest():
		g.Reason = "the corrected rule differs from the verified one (digest " + g.RuleDigest + "): a changed rule needs its own proof — re-export it as a new hypothesis item and verify it"
	case g.Proof == nil && latest != nil:
		g.Reason = fmt.Sprintf("the latest proof %s is %s; promotion needs a confirmed proof (`ri verify hypotheses -runtime`)", latest.ID, latest.Outcome)
	case g.Proof == nil:
		g.Reason = "no proof record for this rule: run `ri verify hypotheses -runtime` first"
	default:
		if why := boundsCover(g.Proof.VersionBounds, it.Release); why != "" {
			g.Proof, g.Reason = nil, why
			break
		}
		g.Open = true
		g.Reason = fmt.Sprintf("confirmed by proof %s (%s)", g.Proof.ID, g.Proof.Level)
	}
	return g
}

// boundsCover reports why a proof's bounds do not cover the release the rule
// is promoted for ("" when they do).
func boundsCover(b *domain.ProofBounds, release string) string {
	if b == nil {
		return "the proof states no version bounds"
	}
	from, err1 := semver.NewVersion(b.From)
	to, err2 := semver.NewVersion(b.To)
	rel, err3 := semver.NewVersion(release)
	if err := errors.Join(err1, err2, err3); err != nil {
		return fmt.Sprintf("the proof's bounds (%s, %s] or the item's release %q do not parse: %v", b.From, b.To, release, err)
	}
	if !rel.GreaterThan(from) || rel.GreaterThan(to) {
		return fmt.Sprintf("the item's release %s lies outside the proof's bounds (%s, %s]", release, b.From, b.To)
	}
	return ""
}

// gateDecisions refuses decisions the promotion gate does not allow. Ordinary
// (non-hypothesis) items are untouched, except that any supersedes/coexists
// they name must be active facts on the same subject.
func gateDecisions(snap *Snapshot, ds []domain.ReviewDecision) error {
	items := map[string]domain.ReviewItem{}
	for _, it := range snap.ReviewItems {
		items[it.ID] = it
	}
	var errs []error
	for _, d := range ds {
		it, ok := items[d.ReviewItemID]
		if !ok {
			continue // validateDecisions reports it
		}
		if err := gateDecision(snap, it, d); err != nil {
			errs = append(errs, fmt.Errorf("knowledge: decision %s on %s refused: %w", d.ID, it.ID, err))
		}
	}
	return errors.Join(errs...)
}

// ErrPromotionRefused marks a decision the promotion gate refused.
var ErrPromotionRefused = errors.New("promotion gate")

func gateDecision(snap *Snapshot, it domain.ReviewItem, d domain.ReviewDecision) error {
	if d.Action != domain.ActionAccept && d.Action != domain.ActionCorrect {
		return nil // reject / need-more-evidence / defer never promote
	}
	fin := d.Final()
	if fin == nil {
		return nil // validateDecisions reports it
	}
	_, _, hyp := HypothesisSource(snap, it)
	named, err := namedFacts(snap, it, *fin, d)
	if err != nil {
		return err
	}
	if !hyp {
		return nil
	}
	if d.ReviewerKind != domain.ReviewerHuman {
		return fmt.Errorf("%w: only a human can promote an agent-proposed rule (reviewer kind %s)", ErrPromotionRefused, d.ReviewerKind)
	}
	if it.QuestionType != domain.QuestionFactReview {
		return fmt.Errorf("%w: an agent-proposed rule is promoted only through a whole-fact review item, not a %s question", ErrPromotionRefused, it.QuestionType)
	}
	for _, x := range domain.Aspects {
		if !slices.Contains(d.Verifies(it), x) {
			return fmt.Errorf("%w: the reviewer must verify the whole rule (aspect %s is not verified by this decision)", ErrPromotionRefused, x)
		}
	}
	g := PromotionGate(snap, it, *fin)
	if !g.Open {
		return fmt.Errorf("%w: %s", ErrPromotionRefused, g.Reason)
	}
	if fin.Subject == nil {
		return fmt.Errorf("%w: the rule names no subject", ErrPromotionRefused)
	}
	var unnamed []string
	self := domain.VerifiedFactID(it.Product, it.Release, *fin)
	for _, f := range snap.Facts {
		if f.Status != domain.FactActive || f.Product != it.Product || f.ID == self || f.Assertion.Subject == nil ||
			f.Assertion.Subject.Key() != fin.Subject.Key() || named[f.ID] {
			continue
		}
		unnamed = append(unnamed, f.ID)
	}
	if len(unnamed) > 0 {
		return fmt.Errorf("%w: active fact(s) %s state(s) the same subject (%s); the decision must say whether the promoted rule supersedes each or coexists with it (supersedes / coexists) — a refining rule never silently coexists",
			ErrPromotionRefused, strings.Join(unnamed, ", "), fin.Subject.Key())
	}
	return nil
}

// namedFacts checks the facts a decision supersedes or keeps: each is an
// active fact of the item's product on the decided rule's subject, and not the
// fact the decision itself mints.
func namedFacts(snap *Snapshot, it domain.ReviewItem, fin domain.SemanticAssertion, d domain.ReviewDecision) (map[string]bool, error) {
	facts := map[string]domain.VerifiedFact{}
	for _, f := range snap.Facts {
		facts[f.ID] = f
	}
	self := domain.VerifiedFactID(it.Product, it.Release, fin)
	out := map[string]bool{}
	for _, id := range append(append([]string(nil), d.Supersedes...), d.Coexists...) {
		f, ok := facts[id]
		switch {
		case !ok:
			return nil, fmt.Errorf("%w: names fact %s, which does not exist", ErrNotFound, id)
		case f.Status != domain.FactActive:
			return nil, fmt.Errorf("names fact %s, which is %s (only an active fact can be superseded or kept)", id, f.Status)
		case f.Product != it.Product:
			return nil, fmt.Errorf("names fact %s of %s, not %s", id, f.Product, it.Product)
		case id == self:
			return nil, fmt.Errorf("names fact %s, which is the fact this decision mints", id)
		case fin.Subject == nil || f.Assertion.Subject == nil || f.Assertion.Subject.Key() != fin.Subject.Key():
			return nil, fmt.Errorf("names fact %s, which states another subject than the decided rule", id)
		}
		out[id] = true
	}
	return out, nil
}

// accountFor refuses a decision that mints or changes fact f (accept,
// correct, duplicate; ordinary and hypothesis-sourced items alike) while
// another active fact of f's product, release and subject is left unnamed,
// or is kept beside f although the two contradict each other (lane backdoor,
// D-0029). implied is the fact the route supersedes by itself (a correction
// of that fact's own review item), "" when none. The hypothesis gate's wider
// rule (every release of the product, gateDecision) is unchanged.
func accountFor(facts map[string]domain.VerifiedFact, it domain.ReviewItem, d domain.ReviewDecision, f domain.VerifiedFact, implied string) error {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("knowledge: decision %s on %s refused: %w: %s", d.ID, it.ID, ErrPromotionRefused, fmt.Sprintf(format, args...))
	}
	named := map[string]bool{}
	for _, id := range d.Supersedes {
		named[id] = true
	}
	kept := map[string]bool{}
	for _, id := range d.Coexists {
		named[id], kept[id] = true, true
	}
	for id := range named {
		g, ok := facts[id]
		switch {
		case !ok:
			return refuse("names fact %s, which does not exist", id)
		case g.Status != domain.FactActive:
			return refuse("names fact %s, which is %s (only an active fact can be superseded or kept)", id, g.Status)
		case id == f.ID:
			return refuse("names fact %s, which is the fact this decision mints or changes", id)
		case g.Product != f.Product || g.Assertion.Subject == nil || f.Assertion.Subject == nil || g.Assertion.Subject.Key() != f.Assertion.Subject.Key():
			return refuse("names fact %s, which states another subject than fact %s", id, f.ID)
		}
	}
	var unnamed, clash []string
	for _, g := range facts {
		if g.Status != domain.FactActive || g.ID == f.ID || g.ID == implied || !domain.SameFactScope(f, g) {
			continue
		}
		switch {
		case !named[g.ID]:
			unnamed = append(unnamed, g.ID)
		case kept[g.ID]:
			if why := domain.FactIncompatibility(f, g); why != "" {
				clash = append(clash, g.ID+" ("+why+")")
			}
		}
	}
	sort.Strings(unnamed)
	sort.Strings(clash)
	if len(unnamed) > 0 {
		how := "supersedes or coexists"
		if d.Action == domain.ActionReject {
			how = "coexists; to replace one, correct or reject its own review item"
		}
		return refuse("active fact(s) %s state(s) the same subject (%s) at %s %s as fact %s; the decision must name each (%s) — no fact silently coexists",
			strings.Join(unnamed, ", "), f.Assertion.Subject.Key(), f.Product, f.Release, f.ID, how)
	}
	if len(clash) > 0 {
		return refuse("fact %s cannot coexist with %s: they contradict each other; supersede it, or retract one",
			f.ID, strings.Join(clash, ", "))
	}
	return nil
}
