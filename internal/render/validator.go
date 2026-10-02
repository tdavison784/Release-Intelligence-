package render

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// ValidatorName is the producer string of the rendered-diff validator.
const ValidatorName = "render.rendered-diff@v1"

// ReleasePairs serves release-level (chart-default) render pairs.
type ReleasePairs interface {
	ReleasePair(ctx context.Context, product, from, to string) *Pair
}

// ReleasePairs returns a memoizing release-pair source over the engine
// (chart defaults, scope release; the render cache makes repeats free).
func (e *Engine) ReleasePairs(kubeVersion string) ReleasePairs {
	return &releasePairs{e: e, kube: kubeVersion, m: map[string]*Pair{}}
}

type releasePairs struct {
	e    *Engine
	kube string
	mu   sync.Mutex
	m    map[string]*Pair
}

func (r *releasePairs) ReleasePair(ctx context.Context, product, from, to string) *Pair {
	k := product + "|" + from + "|" + to
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.m[k]; ok {
		return p
	}
	p := r.e.RenderPair(ctx, PairRequest{Product: product, From: from, To: to, Target: DefaultsTarget(product),
		Scope: domain.RenderRelease, KubeVersion: r.kube})
	r.m[k] = p
	return p
}

// Validator is the `rendered-diff` knowledge.Validator (DESIGN.md §2.3,
// RENDER-MISSION R6): it checks the subject and change aspects of an
// assertion against the release-level renders of the From and To charts
// with chart defaults, and records how the render relates to the claim:
//
//	confirmed-by-render    the renders show the asserted change (confirmed checks)
//	contradicted-by-render the renders show something else (a refuted check)
//	not-visible-in-render  the subject does not appear in either render, the
//	                       render failed, or its family has no rendered
//	                       representation (inconclusive; not wrong)
//	render-not-applicable  the change is not render-verifiable (runtime-only)
//
// It never checks applicability or consequence. Only release-scope renders
// back its evidence, so its results may enter knowledge/.
type Validator struct {
	Pairs ReleasePairs
}

var _ knowledge.Validator = (*Validator)(nil)

// NewValidator returns the validator over a release-pair source.
func NewValidator(p ReleasePairs) *Validator { return &Validator{Pairs: p} }

// Name implements knowledge.Validator.
func (v *Validator) Name() string { return ValidatorName }

func versionOf(r *domain.Release, edgeV domain.Version) string {
	if r != nil {
		edgeV = r.Version
	}
	if edgeV.Semver != "" {
		return edgeV.Semver
	}
	return edgeV.Tag
}

// Validate implements knowledge.Validator.
func (v *Validator) Validate(ctx context.Context, in knowledge.ValidationInput) ([]domain.ValidationResult, error) {
	a := in.Assertion
	if a.Subject == nil || a.Change == nil {
		return nil, nil
	}
	var fromV, toV domain.Version
	if in.Edge != nil {
		fromV, toV = in.Edge.From, in.Edge.To
	}
	from, to := versionOf(in.From, fromV), versionOf(in.To, toV)
	if from == "" || to == "" {
		return nil, fmt.Errorf("rendered-diff: no from/to version for candidate %s", in.Candidate.ID)
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	res := domain.ValidationResult{CandidateID: in.Candidate.ID, ProposalID: in.ProposalID, Validator: ValidatorName,
		Assertion: a, CheckedAt: now.UTC().Truncate(time.Second)}
	rule := fmt.Sprintf("rendered-diff:%s/%s", a.Subject.Family, a.Change.Type)
	set := func(subj, chg domain.ValidationOutcome, rel domain.RenderRelation, detail string) {
		res.Checks = []domain.AspectCheck{
			{Aspect: domain.AspectSubject, Outcome: subj, Rule: rule, Detail: detail},
			{Aspect: domain.AspectChange, Outcome: chg, Rule: rule, Detail: detail},
		}
		res.RenderRelation = rel
	}
	finish := func() ([]domain.ValidationResult, error) {
		res.ID = domain.ValidationID(res.CandidateID, res.Validator, res.Assertion)
		if err := res.Validate(); err != nil {
			return nil, err
		}
		return []domain.ValidationResult{res}, nil
	}
	in2 := domain.RenderNotVerifiable
	if r := RenderabilityOf(a.Subject.Family, a.Change.Type); r != in2 {
		in2 = r
	}
	if in2 == domain.RenderNotVerifiable {
		set(domain.OutcomeInconclusive, domain.OutcomeInconclusive, domain.RenderNotApplicable,
			fmt.Sprintf("%s %s is not render-verifiable (runtime-only or no rendered shape)", a.Subject.Family, a.Change.Type))
		return finish()
	}
	p := v.Pairs.ReleasePair(ctx, string(in.Candidate.Product), from, to)
	if p == nil || p.Status != PairOK {
		why := "no release-level render"
		if p != nil && p.Failure != nil {
			why = fmt.Sprintf("release-level render failed (%s): %s", p.Failure.Reason, p.Failure.Detail)
		}
		set(domain.OutcomeInconclusive, domain.OutcomeInconclusive, domain.RenderNotVisible, why)
		return finish()
	}
	fromOcc, ok := Inventory(p.FromResult.Objects, *a.Subject)
	toOcc, _ := Inventory(p.ToResult.Objects, *a.Subject)
	if !ok {
		set(domain.OutcomeInconclusive, domain.OutcomeInconclusive, domain.RenderNotVisible,
			fmt.Sprintf("%s subjects have no rendered representation this validator reads", a.Subject.Family))
		return finish()
	}
	var replOcc []Occurrence
	if a.Change.ReplacedBy != nil {
		replOcc, _ = Inventory(p.ToResult.Objects, *a.Change.ReplacedBy)
	}
	subj, chg, detail := decide(*a.Change, *a.Subject, fromOcc, toOcc, replOcc)
	switch {
	case chg == domain.OutcomeRefuted:
		set(subj, chg, domain.RenderContradicted, detail)
	case chg == domain.OutcomeConfirmed:
		set(subj, chg, domain.RenderConfirmed, detail)
	case subj == domain.OutcomeConfirmed:
		// the subject exists but the change kind has no decidable rendered shape
		set(subj, chg, domain.RenderConfirmed, detail)
	default:
		set(subj, chg, domain.RenderNotVisible, detail)
	}
	if res.RenderRelation == domain.RenderConfirmed || res.RenderRelation == domain.RenderContradicted {
		res.Evidence = subjectEvidence(p, a.Subject.Key(), fromOcc, toOcc, replOcc)
	}
	return finish()
}

// decide maps the subject's occurrences in the From and To renders onto
// subject/change outcomes. Absence from both renders decides nothing (the
// subject may be behind a disabled default); only positive rendered
// evidence confirms or refutes.
func decide(c domain.ChangeSpec, s domain.Subject, from, to, repl []Occurrence) (subj, chg domain.ValidationOutcome, detail string) {
	inc := domain.OutcomeInconclusive
	fv, tv := values(from), values(to)
	desc := func() string {
		return fmt.Sprintf("%s: source render %s, target render %s", s.Key(), describe(fv, len(from)), describe(tv, len(to)))
	}
	if len(from) == 0 && len(to) == 0 {
		return inc, inc, desc() + " — not visible with chart defaults"
	}
	subj = domain.OutcomeConfirmed
	switch c.Type {
	case domain.ChangeKindAdded:
		switch {
		case len(from) == 0:
			return subj, domain.OutcomeConfirmed, desc()
		case len(to) > 0 && sameStrings(fv, tv):
			return subj, domain.OutcomeRefuted, desc() + " — present unchanged in both"
		}
		return subj, domain.OutcomeRefuted, desc() + " — already present in the source"
	case domain.ChangeKindRemoved:
		if len(to) == 0 {
			return subj, domain.OutcomeConfirmed, desc()
		}
		return subj, domain.OutcomeRefuted, desc() + " — still present in the target"
	case domain.ChangeKindRenamed:
		switch {
		case len(to) > 0:
			return subj, domain.OutcomeRefuted, desc() + " — the old subject is still present in the target"
		case len(repl) > 0:
			return subj, domain.OutcomeConfirmed, desc() + fmt.Sprintf("; replacement %s present in the target", c.ReplacedBy.Key())
		}
		return subj, inc, desc() + " — removed, but the replacement is not visible"
	case domain.ChangeKindValueChanged, domain.ChangeKindDefaultChanged:
		if len(from) == 0 || len(to) == 0 {
			return subj, inc, desc() + " — present on one side only (added/removed, not a value change)"
		}
		if sameStrings(fv, tv) {
			return subj, domain.OutcomeRefuted, desc() + " — the rendered value did not change"
		}
		if c.After != nil && !valueMatches(*c.After, tv) {
			return subj, domain.OutcomeRefuted, desc() + fmt.Sprintf(" — the target renders a value other than the asserted %s", *c.After)
		}
		if c.Before != nil && !valueMatches(*c.Before, fv) {
			return subj, domain.OutcomeRefuted, desc() + fmt.Sprintf(" — the source renders a value other than the asserted %s", *c.Before)
		}
		return subj, domain.OutcomeConfirmed, desc()
	}
	return subj, inc, desc() + fmt.Sprintf(" — %s has no rendered shape", c.Type)
}

func describe(vals []string, n int) string {
	if n == 0 {
		return "absent"
	}
	shown := make([]string, 0, len(vals))
	for _, v := range vals {
		if v == "" {
			v = "(present)"
		}
		shown = append(shown, v)
	}
	return fmt.Sprintf("%d× [%s]", n, strings.Join(shown, ", "))
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// valueMatches compares an asserted JSON-encoded value with rendered values:
// equal as JSON, equal as a string, or (images) equal to the reference's tag.
func valueMatches(asserted string, rendered []string) bool {
	var av any
	s := asserted
	if json.Unmarshal([]byte(asserted), &av) == nil {
		s = fmt.Sprint(av)
	}
	for _, r := range rendered {
		var rv any
		rs := r
		if json.Unmarshal([]byte(r), &rv) == nil {
			rs = fmt.Sprint(rv)
		}
		if r == asserted || rs == s || strings.HasSuffix(r, ":"+s) || strings.HasSuffix(r, "@"+s) {
			return true
		}
	}
	return false
}

// subjectEvidence cites the release-level renders: the rendered-diff records
// of every diff change on an object where the subject occurs, plus one state
// record per side naming the occurrences (so a refutation — a subject present
// unchanged in both renders — has evidence too).
func subjectEvidence(p *Pair, key string, from, to, repl []Occurrence) []domain.Evidence {
	var out []domain.Evidence
	seen := map[domain.EvidenceID]bool{}
	add := func(e domain.Evidence) {
		if !seen[e.ID] {
			seen[e.ID] = true
			out = append(out, e)
		}
	}
	objs := map[string]bool{}
	for _, o := range append(append(append([]Occurrence{}, from...), to...), repl...) {
		objs[o.Object.key()] = true
	}
	for _, c := range p.Diff.Changes {
		if objs[c.Object.key()] {
			add(p.Evidence(c, true))
		}
	}
	add(p.stateEvidence(p.FromResult, p.From, key, from))
	add(p.stateEvidence(p.ToResult, p.To, key, to))
	return out
}

func (p *Pair) stateEvidence(r *Result, version, key string, occ []Occurrence) domain.Evidence {
	pv := r.Provenance
	var locs []string
	for i, o := range occ {
		if i == 5 {
			locs = append(locs, fmt.Sprintf("… %d more", len(occ)-5))
			break
		}
		l := o.Object.String()
		if o.Path != "" {
			l += " " + o.Path
		}
		if o.Value != "" {
			l += " = " + o.Value
		}
		locs = append(locs, l)
	}
	excerpt := fmt.Sprintf("%s render of %s %s: %s absent", p.Scope, p.Product, version, key)
	if len(occ) > 0 {
		excerpt = fmt.Sprintf("%s render of %s %s: %s at %s", p.Scope, p.Product, version, key, strings.Join(locs, "; "))
	}
	uri := pv.ChartURI
	if uri == "" {
		uri = "render:" + p.Product + "@" + version
	}
	ev := domain.NewEvidence(domain.EvidenceRenderedDiff, Producer, uri, "state "+key, excerpt, pv.OutputDigest, pv.RenderedAt)
	rp := pv.Domain()
	rp.Change = "state"
	if len(occ) > 0 {
		rp.Object = occ[0].Object.String()
	}
	ev.Render = &rp
	ev.ID = domain.EvidenceID("ev-" + domain.ShortHash(string(ev.Kind), ev.URI, ev.Locator, ev.Excerpt, ev.ContentDigest, string(rp.Scope), rp.ValuesDigest, rp.ChartDigest))
	return ev
}
