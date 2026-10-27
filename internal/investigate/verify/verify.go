package verify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// Producer names the proof producer (component@vN).
const Producer = "verify.hypothesis@v1"

// Mitigation is what the run record states about a hypothesis' proposed rule
// (lane verdict's domain.MitigationSpec).
type Mitigation struct {
	Rule     *domain.SemanticAssertion
	Bounds   *domain.ProofBounds
	Rejected string // why the agent's spec was not stored ("" when it was)
}

// RunSource reads a hypothesis' mitigation from its investigation record.
type RunSource interface {
	Mitigation(ctx context.Context, investigation, hypothesis string) (*Mitigation, error)
}

// StateRuns reads run records from <state>/investigations/runs/<id>.json.
type StateRuns struct{ Dir string }

var recordID = regexp.MustCompile(`^[a-z]+-[0-9a-f]{6,64}$`)

// runView is the part of an investigation record verify reads: its
// hypotheses (lane verdict's domain.Hypothesis with its MitigationSpec).
type runView struct {
	ID         string              `json:"id"`
	Hypotheses []domain.Hypothesis `json:"hypotheses"`
}

func (r StateRuns) Mitigation(ctx context.Context, inv, hyp string) (*Mitigation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !recordID.MatchString(inv) {
		return nil, fmt.Errorf("verify: %q is not an investigation id", inv)
	}
	b, err := os.ReadFile(filepath.Join(r.Dir, inv+".json"))
	if err != nil {
		return nil, fmt.Errorf("verify: run %s: %w", inv, err)
	}
	var v runView
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("verify: run %s: %w", inv, err)
	}
	if v.ID != inv {
		return nil, fmt.Errorf("verify: file of run %s holds %s", inv, v.ID)
	}
	for _, h := range v.Hypotheses {
		if h.ID != hyp {
			continue
		}
		m := h.Mitigation
		if m == nil {
			return &Mitigation{Rejected: h.MitigationRejected}, nil
		}
		rule := m.ProposedRule.Assertion
		return &Mitigation{Rule: &rule, Bounds: &domain.ProofBounds{From: m.ProposedRule.Release.From, To: m.ProposedRule.Release.To}}, nil
	}
	return nil, fmt.Errorf("verify: run %s has no hypothesis %s", inv, hyp)
}

// Options configure a verification pass.
type Options struct {
	Knowledge knowledge.Store
	Resolver  Resolver
	Runs      RunSource
	// Runtime enables the rule exercise (opt-in: it creates a kind cluster).
	Runtime           bool
	Fixtures          map[string]*FixtureSet
	Exercises         []runtime.DeclaredExercise
	Relationships     func(product string) []catalog.Relationship
	KubernetesVersion string
	Renderer          ControllerRenderer
	Runner            FixtureRunner
	// Again re-verifies items that already have a decisive (confirmed or
	// refuted) proof of their rule; by default they are skipped.
	Again bool
	Now   func() time.Time
	Logf  func(format string, args ...any)
}

// Result is what a pass did for one item.
type Result struct {
	Item       string
	Hypothesis string
	Proof      *domain.ProofRecord
	// Skipped says why no proof was written ("" when one was).
	Skipped string
}

// baseLimitations are true of every proof this producer writes.
var baseLimitations = []string{
	"the rule exercise uses one synthetic subject object per arm in a fresh cluster: other consumers of the subject's outputs, other topologies, pre-existing state and other controller configurations are not covered",
	"the controller is installed from its chart with the fixture's declared values only; environment-specific controller settings are not exercised",
	"the citation check proves the passages exist in version-bounded upstream text and name the subject; whether they state the claimed behaviour is the human reviewer's judgement",
	"a proof never decides whether an environment meets the rule's conditions: the deterministic validators decide that at investigation time",
}

// Hypotheses verifies every open hypothesis-sourced fact-review item and
// stores a proof record for each one it could examine.
func Hypotheses(ctx context.Context, o Options) ([]Result, error) {
	if o.Knowledge == nil {
		return nil, errors.New("verify: a knowledge store is required")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	snap, err := o.Knowledge.Load(ctx, knowledge.Query{})
	if err != nil {
		return nil, err
	}
	cands := map[string]domain.SemanticCandidate{}
	for _, c := range snap.Candidates {
		cands[c.ID] = c
	}
	var out []Result
	for _, it := range snap.ReviewItems {
		if it.QuestionType != domain.QuestionFactReview || it.Status == domain.ReviewDecided || it.Status == domain.ReviewSuperseded {
			continue
		}
		hyp, inv, ok := knowledge.HypothesisSource(snap, it)
		if !ok {
			continue
		}
		res := Result{Item: it.ID, Hypothesis: hyp}
		if !o.Again {
			if p := decisive(snap, it); p != nil {
				res.Skipped = fmt.Sprintf("already %s by proof %s (use -again to re-verify)", p.Outcome, p.ID)
				out = append(out, res)
				continue
			}
		}
		p, why := verifyItem(ctx, o, snap, cands[it.CandidateID], it, hyp, inv)
		if why != "" {
			res.Skipped = why
			out = append(out, res)
			continue
		}
		rec, err := domain.NewRecord(*p)
		if err != nil {
			return out, err
		}
		if err := o.Knowledge.Put(ctx, rec); err != nil {
			return out, fmt.Errorf("verify: store proof of %s: %w", it.ID, err)
		}
		o.Logf("verify: %s (%s): %s", it.ID, hyp, p.Outcome)
		res.Proof = p
		out = append(out, res)
	}
	return out, nil
}

// humanAuthoredBounds are the release bounds a human-authored proposal of
// exactly the item's rule states (it has no run record). Proposals that
// disagree, or none, leave them unknown (with why).
func humanAuthoredBounds(snap *knowledge.Snapshot, it domain.ReviewItem) (*domain.ProofBounds, string) {
	var out *domain.ProofBounds
	for _, p := range snap.Proposals {
		if p.CandidateID != it.CandidateID || p.Assertion.Digest() != it.Proposed.Digest() || !knowledge.HumanAuthored(p.Provenance.Rule) || p.Release == nil {
			continue
		}
		b := &domain.ProofBounds{From: p.Release.From, To: p.Release.To}
		if out != nil && *out != *b {
			return nil, fmt.Sprintf("human-authored proposals of this rule state different release bounds ((%s, %s] and (%s, %s]), so they are unknown", out.From, out.To, b.From, b.To)
		}
		out = b
	}
	if out == nil {
		return nil, "the proposal names no investigation and states no release bounds, so the rule's release bounds are unknown"
	}
	return out, ""
}

func decisive(snap *knowledge.Snapshot, it domain.ReviewItem) *domain.ProofRecord {
	var last *domain.ProofRecord
	for i, p := range snap.Proofs {
		if p.ReviewItem == it.ID && p.ProposedRuleDigest == it.Proposed.Digest() && p.Outcome != domain.ProofInconclusive {
			last = &snap.Proofs[i]
		}
	}
	return last
}

// verifyItem builds the proof of one item ("" why) or says why it cannot.
func verifyItem(ctx context.Context, o Options, snap *knowledge.Snapshot, c domain.SemanticCandidate, it domain.ReviewItem, hyp, inv string) (*domain.ProofRecord, string) {
	rule := it.Proposed
	var bounds *domain.ProofBounds
	var limitations []string
	switch {
	case inv == "":
		b, why := humanAuthoredBounds(snap, it)
		if why != "" {
			limitations = append(limitations, why)
		}
		bounds = b
	case o.Runs == nil:
		limitations = append(limitations, "no run store configured, so the rule's release bounds are unknown")
	default:
		m, err := o.Runs.Mitigation(ctx, inv, hyp)
		switch {
		case err != nil:
			limitations = append(limitations, fmt.Sprintf("the rule's release bounds are unknown: %v", err))
		case m.Rule == nil:
			return nil, fmt.Sprintf("hypothesis %s has no stored mitigation spec (%s)", hyp, m.Rejected)
		case m.Rule.Digest() != rule.Digest():
			return nil, fmt.Sprintf("item %s proposes another rule than hypothesis %s's mitigation spec (digest %s vs %s)", it.ID, hyp, rule.Digest(), m.Rule.Digest())
		default:
			bounds = m.Bounds
		}
	}
	// the upstream citations of the hypothesis proposals of this rule
	evByID := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range c.Evidence {
		evByID[e.ID] = e
	}
	var evs []domain.Evidence
	seen := map[domain.EvidenceID]bool{}
	for _, p := range snap.Proposals {
		if p.CandidateID != it.CandidateID || p.Assertion.Digest() != rule.Digest() {
			continue
		}
		if _, _, ok := knowledge.ParseHypothesisRule(p.Provenance.Rule); !ok {
			continue
		}
		for _, id := range p.Citations {
			if seen[id] {
				continue
			}
			seen[id] = true
			e, ok := evByID[id]
			if !ok {
				e = domain.Evidence{ID: id, URI: "(evidence " + string(id) + " is not on the candidate)"}
			}
			evs = append(evs, e)
		}
	}
	res := o.Resolver
	if res == nil {
		res = Resolvers{}
	}
	methods := []domain.ProofMethod{CheckCitations(ctx, res, rule, bounds, evs)}
	var ex domain.ProofMethod
	if o.Runtime {
		var rels []catalog.Relationship
		if o.Relationships != nil {
			rels = o.Relationships(string(it.Product))
		}
		var notes []string
		ex, notes = ExerciseRule(ctx, ExerciseInput{Product: string(it.Product), Rule: rule, Bounds: bounds, Fixtures: o.Fixtures[string(it.Product)],
			Exercises: o.Exercises, Relationships: rels, KubernetesVersion: o.KubernetesVersion, Renderer: o.Renderer, Runner: o.Runner,
			Evidence: evs})
		limitations = append(limitations, notes...)
	} else {
		ex = domain.ProofMethod{Method: domain.ProofRuleExercise, Producer: ExerciseProducer, Level: domain.LevelObserved, Outcome: domain.MethodNotRun,
			Detail: "the rule exercise is opt-in (-runtime): without it the rule's behaviour is not observed, so the proof cannot be confirmed"}
	}
	methods = append(methods, ex)
	p := domain.ProofRecord{Hypothesis: hyp, Investigation: inv, ReviewItem: it.ID, CandidateID: it.CandidateID, Product: it.Product, Release: it.Release,
		ProposedRuleDigest: rule.Digest(), Methods: methods, VersionBounds: bounds, Conditions: rule.Applicability,
		Limitations: append(append([]string(nil), baseLimitations...), limitations...), Producer: Producer, CheckedAt: o.Now().UTC().Truncate(time.Second)}
	p.Outcome = domain.ProofOutcomeOf(methods)
	p.Level = domain.LevelDeterministic
	if ex.Outcome != domain.MethodNotRun {
		p.Level = domain.LevelObserved
	}
	p.ID = domain.ProofRecordID(p)
	if err := p.Validate(); err != nil {
		return nil, fmt.Sprintf("the proof does not validate (a bug, nothing stored): %v", err)
	}
	return &p, ""
}
