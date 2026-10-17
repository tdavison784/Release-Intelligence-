package reviewui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/validate"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// Triage decides which review items the default inbox shows. The product
// owner verifies knowledge facts here and wants only what needs a human
// decision now; everything else stays reachable through one explicit
// "Show all staged data" filter and is never mixed into the default view.
//
// An item NEEDS REVIEW when all hold:
//   - its status is pending (needs-evidence / deferred / decided / superseded
//     items already carry a decision or were replaced),
//   - no decision of any kind (human or proxy) is recorded on it,
//   - its subject is not already verified at human or deterministic level by
//     an active fact (for the aspects the item asks about), and
//   - for a fact-review (audit) item, the audited fact is still active.
//
// Facts themselves (consensus-only or proxy-staged, retracted, superseded)
// and model proposals are not review items, so they never enter the list;
// they only matter here as a reason to hide an item.
//
// The default view is judged by FACT level, not by the aspect an item asks
// about: a pin-source fact is as trusted as its weakest aspect
// (VerifiedFact.Level), so it needs review while any aspect is below human
// (or while no fact is composed yet because a question is open). Every open
// item of such a fact's candidates is listed, siblings included, grouped per
// fact (PinGroups).

// Hidden reasons (why an item is outside the default view).
const (
	HiddenDecided    = "decided"    // a human decision is recorded
	HiddenProxy      = "proxy"      // an AI proxy acted on it (never counted as human)
	HiddenParked     = "parked"     // needs-evidence / deferred (a decision parked it)
	HiddenVerified   = "verified"   // an active fact already holds it at human/deterministic level
	HiddenSuperseded = "superseded" // replaced, or the fact it audits was retracted / superseded
)

// Triage is the verdict for one review item.
type Triage struct {
	NeedsReview bool
	// Hidden is the reason the item is outside the default view ("" when it
	// needs review); HiddenText is the plain sentence for it.
	Hidden, HiddenText string
	// Blocking marks a needs-review item of an investigated product whose
	// effect is controller behaviour (or unknown), so an investigation cannot
	// clear or flag it until a human verifies it. PinSource ranks first among
	// those: the proposed assertion, once verified, names a setting
	// OverrideInspection can look for. Why says it in one plain sentence.
	Blocking, PinSource, ProxyActed bool
	Why                             string
	// Group is the PinGroup key (fact id, or candidate id while no fact is
	// composed) of a pin-source item.
	Group string
}

// BlockingScope says what feeds investigations: an item is a candidate blocker
// only when its product is investigated, or its evidence comes from an
// upstream repository an investigated product reads (a chart that bundles an
// operator reads the operator's notes). It is derived from declarations
// (investigation inputs, product definitions), never a list of product names.
type BlockingScope struct {
	Products []domain.ProductID
	// Ranges are the upgrades investigated per product: an item of such a
	// product only blocks when its release lies in (From, To] of one of them.
	// An unparsable release, or a product without ranges, is never excluded.
	Ranges map[domain.ProductID][]VersionRange
	// Sources are repository paths ("prometheus-operator/prometheus-operator")
	// whose upstream statements investigations read.
	Sources []string
}

// VersionRange is one investigated upgrade, From exclusive, To inclusive.
type VersionRange struct{ From, To *semver.Version }

var reVersion = regexp.MustCompile(`\d+\.\d+\.\d+`)

func parseVersion(s string) *semver.Version {
	m := reVersion.FindString(s)
	if m == "" {
		return nil
	}
	v, err := semver.NewVersion(m)
	if err != nil {
		return nil
	}
	return v
}

func (b BlockingScope) inRange(product domain.ProductID, release string) bool {
	rs := b.Ranges[product]
	rel := parseVersion(release)
	if len(rs) == 0 || rel == nil {
		return true
	}
	for _, r := range rs {
		if rel.GreaterThan(r.From) && !rel.GreaterThan(r.To) {
			return true
		}
	}
	return false
}

// Empty reports whether nothing feeds investigations.
func (b BlockingScope) Empty() bool { return len(b.Products) == 0 && len(b.Sources) == 0 }

func (b BlockingScope) covers(product domain.ProductID, release string, c domain.SemanticCandidate) bool {
	if slices.Contains(b.Products, product) {
		return b.inRange(product, release)
	}
	for _, e := range c.Evidence {
		uri := strings.ToLower(e.URI)
		for _, s := range b.Sources {
			if strings.Contains(uri, strings.ToLower(s)) {
				return true
			}
		}
	}
	return false
}

var (
	reProduct = regexp.MustCompile(`(?m)^product:\s*(\S+)`)
	reFromTo  = regexp.MustCompile(`(?ms)^from:\s*\n\s+version:\s*(\S+).*?^to:\s*\n\s+version:\s*(\S+)`)
	reRepo    = regexp.MustCompile(`(?m)^\s*repository:\s*['"]?([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)['"]?\s*$`)
)

// ScopeFromInvestigations reads the product of every investigation case input
// (<dir>/*/input.yaml; inputs only, never expected.yaml) and the upstream
// repositories its product definition (<productsDir>/<product>.yaml) reads
// release statements from. Missing directories give an empty scope.
func ScopeFromInvestigations(investigationsDir, productsDir string) BlockingScope {
	var sc BlockingScope
	seenProduct := map[string]bool{}
	inputs, _ := filepath.Glob(filepath.Join(investigationsDir, "*", "input.yaml"))
	for _, in := range inputs {
		b, err := os.ReadFile(in)
		if err != nil {
			continue
		}
		m := reProduct.FindSubmatch(b)
		if m == nil {
			continue
		}
		if ft := reFromTo.FindSubmatch(b); ft != nil {
			// kube-prometheus-stack-style chart versions do not match the release
			// ids of the knowledge store, which parse independently; both are kept
			from, to := parseVersion(string(ft[1])), parseVersion(string(ft[2]))
			if from != nil && to != nil {
				if sc.Ranges == nil {
					sc.Ranges = map[domain.ProductID][]VersionRange{}
				}
				p := domain.ProductID(m[1])
				if r := (VersionRange{from, to}); !slices.ContainsFunc(sc.Ranges[p], func(x VersionRange) bool { return x.From.Equal(r.From) && x.To.Equal(r.To) }) {
					sc.Ranges[p] = append(sc.Ranges[p], r)
				}
			}
		}
		if seenProduct[string(m[1])] {
			continue
		}
		seenProduct[string(m[1])] = true
		sc.Products = append(sc.Products, domain.ProductID(m[1]))
		pb, err := os.ReadFile(filepath.Join(productsDir, string(m[1])+".yaml"))
		if err != nil {
			continue
		}
		for _, r := range reRepo.FindAllSubmatch(pb, -1) {
			if s := string(r[1]); !slices.Contains(sc.Sources, s) {
				sc.Sources = append(sc.Sources, s)
			}
		}
	}
	return sc
}

const (
	pinSourceWhy = "Verifying this lets investigations clear environments that pin the old default."
	blockingWhy  = "Investigations of this product cannot rely on this behaviour change until a human verifies it."
)

// triageItems classifies every review item of the snapshot.
func triageItems(snap *knowledge.Snapshot, scope BlockingScope) map[string]Triage {
	cands := map[string]domain.SemanticCandidate{}
	for _, c := range snap.Candidates {
		cands[c.ID] = c
	}
	decs := map[string][]domain.ReviewDecision{}
	for _, d := range snap.Decisions {
		decs[d.ReviewItemID] = append(decs[d.ReviewItemID], d)
	}
	groups := pinGroupOf(snap, scope)
	out := make(map[string]Triage, len(snap.ReviewItems))
	for _, it := range snap.ReviewItems {
		t := triageOne(it, cands[it.CandidateID], decs[it.ID], snap.Facts, scope)
		// a pin source by the fact it would verify, not by its own question:
		// a sibling question (e.g. the consequence of a default-change
		// candidate) is needed to make the pin-source fact human as well
		if g := groups[it.CandidateID]; g != nil && t.NeedsReview && t.Blocking && !g.Human() {
			t.PinSource, t.Group = true, g.Key
			t.Why = strings.Replace(t.Why, blockingWhy, pinSourceWhy, 1)
		}
		out[it.ID] = t
	}
	return out
}

func hide(reason, text string) Triage { return Triage{Hidden: reason, HiddenText: text} }

func triageOne(it domain.ReviewItem, cand domain.SemanticCandidate, decs []domain.ReviewDecision, facts []domain.VerifiedFact, scope BlockingScope) Triage {
	var human []domain.ReviewDecision
	var proxy *domain.ReviewDecision
	for i, d := range decs {
		if d.ReviewerKind == domain.ReviewerHuman {
			human = append(human, d)
		} else {
			proxy = &decs[i]
		}
	}
	if n := len(human); n > 0 {
		last := human[n-1]
		switch it.Status {
		case domain.ReviewNeedsEvidence, domain.ReviewDeferred:
			return hide(HiddenParked, fmt.Sprintf("%s by %s", strings.ReplaceAll(string(it.Status), "-", " "), last.Reviewer))
		}
		return hide(HiddenDecided, fmt.Sprintf("decided by %s (%s)", last.Reviewer, last.Action))
	}
	// no human decision: a proxy decision settles nothing (FLEET: a proxy is
	// never human; DESIGN: proxy-verified facts are capped and never pin
	// sources), so the item stays open when it blocks investigations
	switch it.Status {
	case domain.ReviewPending:
	case domain.ReviewNeedsEvidence, domain.ReviewDeferred, domain.ReviewDecided:
		if proxy == nil {
			if it.Status == domain.ReviewDecided {
				return hide(HiddenDecided, "status decided")
			}
			return hide(HiddenParked, strings.ReplaceAll(string(it.Status), "-", " "))
		}
	case domain.ReviewSuperseded:
		return hide(HiddenSuperseded, "superseded by a newer item")
	default:
		return hide(HiddenDecided, "status "+string(it.Status))
	}
	aspects := it.Aspects()
	for _, f := range facts {
		if !slices.Contains(f.Candidates, it.CandidateID) {
			continue
		}
		// an audit (classification) or whole-fact (fact-review) item re-asks
		// this fact; each is judged by the aspects its accept would verify (an
		// audit verifies the consequence only, a whole-fact review all four)
		audit := (it.QuestionType == domain.QuestionClassification || it.QuestionType == domain.QuestionFactReview) &&
			f.Assertion.Digest() == it.Proposed.Digest()
		if audit && f.Status != domain.FactActive {
			return hide(HiddenSuperseded, "the fact it audits is "+string(f.Status))
		}
		if f.Status == domain.FactActive && trustedAspects(f, aspects) {
			return hide(HiddenVerified, "already verified at human or deterministic level by fact "+f.ID)
		}
	}
	t := Triage{NeedsReview: true}
	if scope.covers(it.Product, it.Release, cand) && controllerBehaviour(cand) {
		t.Blocking, t.Why = true, blockingWhy
	}
	if proxy != nil {
		t.ProxyActed = true
		if !t.Blocking {
			return hide(HiddenProxy, fmt.Sprintf("an AI proxy (%s) acted on it; that is not a human decision", proxy.Reviewer))
		}
		t.Why += " An AI proxy already acted on it; that is not human verification."
	}
	return t
}

var reComputedRule = regexp.MustCompile(`computed artifact diff \(([a-z-]+:[a-z-]+)\)`)

// controllerBehaviour reports whether the candidate manifests as controller
// behaviour (DESIGN §2.2.1): every member is an upstream statement
// (manifest:declared-behaviour), or a computed diff of rule crd:default-changed
// (manifest:crd-default). The rules of computed members are read from the
// "computed artifact diff (<rule>)" lines the candidate text records; any other
// computed rule is decided by a render, not by a human-verified fact.
func controllerBehaviour(c domain.SemanticCandidate) bool {
	computed := 0
	for _, m := range c.Members {
		if m.Computed {
			computed++
		}
	}
	if len(c.Members) == 0 {
		return false
	}
	if computed == 0 {
		return true
	}
	for _, m := range reComputedRule.FindAllStringSubmatch(c.Text, -1) {
		if m[1] == "crd:default-changed" {
			return true
		}
	}
	return false
}

// trustedAspects reports whether f verifies every aspect at human or
// deterministic level (false for a question that verifies no aspect).
func trustedAspects(f domain.VerifiedFact, aspects []domain.Aspect) bool {
	if len(aspects) == 0 {
		return false
	}
	for _, a := range aspects {
		ok := false
		for _, v := range f.Verification {
			ok = ok || (v.Aspect == a && v.Level.Trusted())
		}
		if !ok {
			return false
		}
	}
	return true
}

// SnapshotSource is what the default view needs beyond knowledge.Queue: the
// records (decisions, facts, candidates) that say whether an item still needs
// a human. The demo queue implements it; the file-backed queue gets it
// through Options.Snapshot.
type SnapshotSource interface {
	Snapshot(ctx context.Context) (*knowledge.Snapshot, error)
}

// CountViews returns how many items each needs-review view lists: pin sources
// (the default), the other blocking items, and the remaining open items.
func CountViews(snap *knowledge.Snapshot, scope BlockingScope) (pins, blocking, others int) {
	for _, t := range triageItems(snap, scope) {
		switch {
		case !t.NeedsReview:
		case t.PinSource:
			pins++
		case t.Blocking:
			blocking++
		default:
			others++
		}
	}
	return pins, blocking, others
}

// UnreviewedPinFacts lists active facts that would be OverrideInspection pin
// sources, belong to an investigated upgrade, rest on upstream behaviour
// statements, and are not verified at human or deterministic level on every
// aspect, yet have no whole-fact review item. Consensus auto-approval
// supersedes the questions, and an audit (classification) item verifies only
// the consequence, so such a fact can stay below human after every item a
// human could decide is decided. `ri review open-audits` opens one whole-fact
// (fact-review) item per fact (knowledge.OpenWholeFactReview).
func UnreviewedPinFacts(snap *knowledge.Snapshot, scope BlockingScope) []domain.VerifiedFact {
	cands := map[string]domain.SemanticCandidate{}
	for _, c := range snap.Candidates {
		cands[c.ID] = c
	}
	have := map[string]bool{}
	for _, it := range snap.ReviewItems {
		have[it.ID] = true
	}
	var out []domain.VerifiedFact
	for _, f := range snap.Facts {
		if !pinFact(f, cands, scope) || trustedAspects(f, domain.Aspects) {
			continue
		}
		if have[domain.ReviewItemID(f.Candidates[0], domain.QuestionFactReview, f.Assertion)] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// pinFact reports whether f is an active pin-source fact of an investigated
// upgrade that rests on upstream behaviour statements.
func pinFact(f domain.VerifiedFact, cands map[string]domain.SemanticCandidate, scope BlockingScope) bool {
	if f.Status != domain.FactActive || len(f.Candidates) == 0 {
		return false
	}
	cand := cands[f.Candidates[0]]
	return scope.covers(f.Product, f.Release, cand) && controllerBehaviour(cand) && validate.CanBePinSource(f.Assertion)
}

// PinGroup is one pin-source fact the default view asks a human to verify:
// an active fact, or, while no fact is composed, the candidate whose questions
// would compose one (some item of it proposes a pin-source assertion).
type PinGroup struct {
	Key       string // the fact id, or the candidate id while no fact is composed
	Fact      *domain.VerifiedFact
	Candidate domain.SemanticCandidate
	Product   domain.ProductID
	Release   string
	Statement string
	// Levels is the verification level per aspect ("" = not verified yet).
	Levels map[domain.Aspect]domain.VerificationLevel
	// Open are the open review items (ids) of the group, in the default view.
	Open []string
}

// Human reports whether every aspect is verified at human or deterministic
// level (the fact is trusted; nothing is left to verify).
func (g PinGroup) Human() bool {
	for _, x := range domain.Aspects {
		if !g.Levels[x].Trusted() {
			return false
		}
	}
	return true
}

// LevelLine reads "subject deterministic · change consensus · …".
func (g PinGroup) LevelLine() string {
	var out []string
	for _, x := range domain.Aspects {
		l := string(g.Levels[x])
		if l == "" {
			l = "not verified yet"
		}
		out = append(out, string(x)+" "+l)
	}
	return strings.Join(out, " · ")
}

// LeftLine is the one-line "N questions left to verify this fact".
func (g PinGroup) LeftLine() string {
	n := len(g.Open)
	what := "this fact"
	if g.Fact == nil {
		what = "this change (no fact composed yet)"
	}
	switch n {
	case 0:
		return "No question open for " + what + ": run `ri review open-audits -apply` to open its whole-fact review"
	case 1:
		return "1 question left to verify " + what
	}
	return fmt.Sprintf("%d questions left to verify %s", n, what)
}

// pinGroupOf maps every candidate of a pin-source fact or pin-source
// candidate (in scope, controller behaviour) to its group.
func pinGroupOf(snap *knowledge.Snapshot, scope BlockingScope) map[string]*PinGroup {
	cands := map[string]domain.SemanticCandidate{}
	for _, c := range snap.Candidates {
		cands[c.ID] = c
	}
	out := map[string]*PinGroup{}
	facts := slices.Clone(snap.Facts)
	slices.SortFunc(facts, func(a, b domain.VerifiedFact) int { return strings.Compare(a.ID, b.ID) })
	hasFact := map[string]bool{}
	for i := range facts {
		f := facts[i]
		if f.Status != domain.FactActive {
			continue
		}
		for _, c := range f.Candidates {
			hasFact[c] = true
		}
		if !pinFact(f, cands, scope) {
			continue
		}
		g := &PinGroup{Key: f.ID, Fact: &f, Candidate: cands[f.Candidates[0]], Product: f.Product, Release: f.Release,
			Statement: f.Assertion.Statement, Levels: map[domain.Aspect]domain.VerificationLevel{}}
		for _, x := range domain.Aspects {
			g.Levels[x] = f.AspectLevel(x)
		}
		for _, c := range f.Candidates {
			if out[c] == nil {
				out[c] = g
			}
		}
	}
	// candidates with no active fact: a pin source when one of their items
	// proposes a pin-source assertion (the consequence question of a
	// default-change candidate is its sibling and belongs to the same group)
	items := map[string]domain.ReviewItem{}
	for _, it := range snap.ReviewItems {
		items[it.ID] = it
		c, ok := cands[it.CandidateID]
		if !ok || hasFact[c.ID] || out[c.ID] != nil || it.Status == domain.ReviewSuperseded {
			continue
		}
		if !validate.CanBePinSource(it.Proposed) || !scope.covers(c.Product, c.Release, c) || !controllerBehaviour(c) {
			continue
		}
		out[c.ID] = &PinGroup{Key: c.ID, Candidate: c, Product: c.Product, Release: c.Release, Statement: c.Title,
			Levels: map[domain.Aspect]domain.VerificationLevel{}}
	}
	// the aspects a composing candidate already holds: validator-confirmed,
	// then human (or proxy) accept/correct decisions on its live items
	for _, v := range snap.Validations {
		if g := out[v.CandidateID]; g != nil && g.Fact == nil {
			for _, x := range domain.Aspects {
				if v.Confirms(x) {
					g.Levels[x] = domain.VerifiedDeterministic
				}
			}
		}
	}
	for _, d := range snap.Decisions {
		it, ok := items[d.ReviewItemID]
		g := out[it.CandidateID]
		if !ok || g == nil || g.Fact != nil || it.Status == domain.ReviewSuperseded ||
			(d.Action != domain.ActionAccept && d.Action != domain.ActionCorrect) {
			continue
		}
		lvl := domain.VerifiedHuman
		if d.ReviewerKind == domain.ReviewerProxy {
			lvl = domain.VerifiedProxy
		}
		for _, x := range d.Verifies(it) {
			if cur := g.Levels[x]; !cur.Trusted() {
				g.Levels[x] = lvl
			}
		}
	}
	return out
}

// PinGroups lists the pin-source facts (and composing candidates) below human
// with their open items in the default view, ordered by product, release and
// key. A group with no open item is still listed: its fact needs a whole-fact
// review item first (`ri review open-audits`).
func PinGroups(snap *knowledge.Snapshot, scope BlockingScope) []PinGroup {
	return pinGroupsFrom(snap, scope, triageItems(snap, scope))
}

func pinGroupsFrom(snap *knowledge.Snapshot, scope BlockingScope, tr map[string]Triage) []PinGroup {
	seen := map[string]*PinGroup{}
	var keys []string
	for _, g := range pinGroupOf(snap, scope) {
		if g.Human() || seen[g.Key] != nil {
			continue
		}
		seen[g.Key] = g
		keys = append(keys, g.Key)
	}
	for _, it := range snap.ReviewItems {
		if t := tr[it.ID]; t.PinSource && seen[t.Group] != nil {
			seen[t.Group].Open = append(seen[t.Group].Open, it.ID)
		}
	}
	out := make([]PinGroup, 0, len(keys))
	for _, k := range keys {
		g := seen[k]
		if g.Fact == nil && len(g.Open) == 0 {
			continue // nothing composes it any more (e.g. its mapping was rejected)
		}
		slices.Sort(g.Open)
		out = append(out, *g)
	}
	slices.SortFunc(out, func(a, b PinGroup) int {
		if c := strings.Compare(string(a.Product), string(b.Product)); c != 0 {
			return c
		}
		if c := strings.Compare(a.Release, b.Release); c != 0 {
			return c
		}
		return strings.Compare(a.Key, b.Key)
	})
	return out
}
