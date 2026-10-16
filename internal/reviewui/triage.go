package reviewui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

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
	Blocking, PinSource bool
	Why                 string
}

// BlockingScope says what feeds investigations: an item is a candidate blocker
// only when its product is investigated, or its evidence comes from an
// upstream repository an investigated product reads (a chart that bundles an
// operator reads the operator's notes). It is derived from declarations
// (investigation inputs, product definitions), never a list of product names.
type BlockingScope struct {
	Products []domain.ProductID
	// Sources are repository paths ("prometheus-operator/prometheus-operator")
	// whose upstream statements investigations read.
	Sources []string
}

// Empty reports whether nothing feeds investigations.
func (b BlockingScope) Empty() bool { return len(b.Products) == 0 && len(b.Sources) == 0 }

func (b BlockingScope) covers(product domain.ProductID, c domain.SemanticCandidate) bool {
	if slices.Contains(b.Products, product) {
		return true
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
		if m == nil || seenProduct[string(m[1])] {
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
	out := make(map[string]Triage, len(snap.ReviewItems))
	for _, it := range snap.ReviewItems {
		out[it.ID] = triageOne(it, cands[it.CandidateID], decs[it.ID], snap.Facts, scope)
	}
	return out
}

func hide(reason, text string) Triage { return Triage{Hidden: reason, HiddenText: text} }

func triageOne(it domain.ReviewItem, cand domain.SemanticCandidate, decs []domain.ReviewDecision, facts []domain.VerifiedFact, scope BlockingScope) Triage {
	for _, d := range decs {
		if d.ReviewerKind != domain.ReviewerHuman {
			return hide(HiddenProxy, fmt.Sprintf("an AI proxy (%s) acted on it; that is not a human decision", d.Reviewer))
		}
	}
	if n := len(decs); n > 0 {
		last := decs[n-1]
		switch it.Status {
		case domain.ReviewNeedsEvidence, domain.ReviewDeferred:
			return hide(HiddenParked, fmt.Sprintf("%s by %s", strings.ReplaceAll(string(it.Status), "-", " "), last.Reviewer))
		}
		return hide(HiddenDecided, fmt.Sprintf("decided by %s (%s)", last.Reviewer, last.Action))
	}
	switch it.Status {
	case domain.ReviewPending:
	case domain.ReviewNeedsEvidence, domain.ReviewDeferred:
		return hide(HiddenParked, strings.ReplaceAll(string(it.Status), "-", " "))
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
		audit := it.QuestionType == domain.QuestionClassification && f.Assertion.Digest() == it.Proposed.Digest()
		if audit && f.Status != domain.FactActive {
			return hide(HiddenSuperseded, "the fact it audits is "+string(f.Status))
		}
		if f.Status == domain.FactActive && trustedAspects(f, aspects) {
			return hide(HiddenVerified, "already verified at human or deterministic level by fact "+f.ID)
		}
	}
	t := Triage{NeedsReview: true}
	if scope.covers(it.Product, cand) && controllerBehaviour(cand) {
		t.Blocking, t.Why = true, blockingWhy
		if validate.CanBePinSource(it.Proposed) {
			t.PinSource, t.Why = true, pinSourceWhy
		}
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

// CountNeedsReview returns how many items the default view lists and how many
// of those block investigations (what `ri review serve` prints at start).
func CountNeedsReview(snap *knowledge.Snapshot, scope BlockingScope) (needs, blocking int) {
	for _, t := range triageItems(snap, scope) {
		if t.NeedsReview {
			needs++
			if t.Blocking {
				blocking++
			}
		}
	}
	return needs, blocking
}
