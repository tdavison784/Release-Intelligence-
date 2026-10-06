package policy

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/render"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// Producer identifies the evaluator in verdicts.
const Producer = "policy@v1"

// Input is everything one upgrade verdict is decided from. Rendered diffs
// are consumed through internal/render's exported types; Edge and Report are
// optional (a verdict without them can only cover the rendered changes, and
// says so).
type Input struct {
	Pairs  []*render.Pair
	Edge   *domain.UpgradeEdge
	Report *domain.ImpactReport
}

// Item is one classified unit of the upgrade with the rule that decided it.
type Item struct {
	Subject Subject `json:"subject"`
	// Ref is a one-line, value-free description (a rendered change summary, a
	// change title, a finding title, a render state).
	Ref string `json:"ref"`
	// Target is the render target id (rendered and render-state items).
	Target string `json:"target,omitempty"`
	// Rule is the id of the rule that fired, "default" when none matched, or
	// a builtin: id (builtin:routine, builtin:cleared) for the two implicit
	// outcomes. First match wins.
	Rule string `json:"rule"`
	// PolicyTier is what the rules said; Tier is the outcome after the safety
	// invariants (never lower than PolicyTier, never auto-pass under a floor).
	PolicyTier Tier   `json:"policyTier"`
	Tier       Tier   `json:"tier"`
	Reason     string `json:"reason"`
	// Invariant names the safety invariant that applies to this item (a floor
	// no policy can lower); the tier is raised to it when the rules said less
	// (PolicyTier shows what they said). Empty when no invariant applies.
	Invariant string `json:"invariant,omitempty"`
	// Attrs are the derived attributes the predicates saw (for example
	// bump=minor, sameRepository=true); they never carry configuration values.
	Attrs map[string]string `json:"attrs,omitempty"`
	// Evidence cites where the item comes from: render artifact digests,
	// change and finding ids, upstream evidence ids.
	Evidence []string `json:"evidence,omitempty"`
}

// Verdict is the upgrade-level result.
type Verdict struct {
	Producer string `json:"producer"`
	Policy   string `json:"policy"`
	Digest   string `json:"policyDigest"`
	// Tier is the upgrade-level tier: auto-pass only when every item is
	// auto-pass, otherwise the strictest tier present.
	Tier Tier `json:"tier"`
	// Why groups the items that kept the upgrade out of auto-pass (or, for an
	// auto-pass verdict, the rules that covered it).
	Why   []Group `json:"why"`
	Items []Item  `json:"items"`
	// Counts per tier.
	Counts map[Tier]int `json:"counts"`
	// Covered counts what was accounted for without an item: routine upstream
	// changes (builtin:routine), non-routine changes deferred to their findings
	// and findings cleared as not-affected.
	Covered CoveredCounts `json:"covered"`
	Notes   []string      `json:"notes,omitempty"`
}

// Group is Why's entry: the items sharing a rule (and invariant).
type Group struct {
	Rule      string `json:"rule"`
	Tier      Tier   `json:"tier"`
	Reason    string `json:"reason"`
	Invariant string `json:"invariant,omitempty"`
	Count     int    `json:"count"`
}

// CoveredCounts is the bookkeeping of items that need no row of their own.
type CoveredCounts struct {
	RoutineChanges  int `json:"routineChanges"`
	DeferredChanges int `json:"deferredChanges"` // non-routine upstream changes decided through their findings
	ClearedFindings int `json:"clearedFindings"` // not-affected findings
}

// AutoPass reports whether the whole upgrade auto-passes.
func (v *Verdict) AutoPass() bool { return v != nil && v.Tier == AutoPass }

// Safety invariants (invariants.go) name the floors Evaluate applies.
const (
	InvActionRequired = "action-required-finding"
	InvAdvisory       = "security-advisory-affects-target"
	InvRenderState    = "render-not-complete"
	InvUnknown        = "unknown-on-non-routine-change"
	InvBreaking       = "breaking-or-directive-change"
)

// Evaluate decides the verdict. It is deterministic and pure.
func Evaluate(p *Policy, in Input) *Verdict {
	e := &evaluator{p: p, in: in, changes: map[string]*domain.Change{}, findingsOf: map[string][]*domain.ImpactFinding{}}
	if in.Edge != nil {
		for i := range in.Edge.Changes {
			c := &in.Edge.Changes[i]
			e.changes[c.ID] = c
		}
	}
	if in.Report != nil {
		for i := range in.Report.Findings {
			f := &in.Report.Findings[i]
			if f.ChangeID != "" {
				e.findingsOf[f.ChangeID] = append(e.findingsOf[f.ChangeID], f)
			}
		}
	}
	e.renderStates()
	e.renderedChanges()
	e.upstream()
	e.findings()
	return e.verdict()
}

type evaluator struct {
	p          *Policy
	in         Input
	changes    map[string]*domain.Change
	findingsOf map[string][]*domain.ImpactFinding
	items      []Item
	covered    CoveredCounts
	notes      []string
}

// decide applies first-match-wins over the rules for one subject. ok is false
// when no rule matched (the caller then applies its implicit outcome or the
// policy default).
func (e *evaluator) decide(m func(Match) bool) (id string, o Outcome, ok bool) {
	for _, r := range e.p.Rules {
		if m(r.Match) {
			return r.ID, Outcome{Tier: r.Tier, Reason: r.Reason}, true
		}
	}
	return "default", e.p.Default, false
}

func (e *evaluator) add(it Item, floor Tier, invariant string) {
	it.PolicyTier = it.Tier
	if invariant != "" {
		// recorded whenever a floor applies; the tier only moves when the
		// rules said less than the floor
		it.Invariant = invariant
		it.Tier = Max(it.Tier, floor)
	}
	e.items = append(e.items, it)
}

// --- render state -------------------------------------------------------------

func (e *evaluator) renderStates() {
	if len(e.in.Pairs) == 0 {
		e.stateItem("", "no render was run: the rendered diff is the evidence that nothing else changes, and there is none", []string{StateNotRendered}, nil)
		return
	}
	for _, p := range e.in.Pairs {
		var states []string
		switch {
		case p.TargetRejects():
			states = []string{StateRejectsConfig, StateFailed}
		case p.Status == render.PairFailed:
			states = []string{StateFailed}
		case p.Status == render.PairNotApplicable:
			states = []string{StateNotApplicable}
		case !p.Complete():
			states = []string{StateIncompleteValues}
		}
		if len(states) == 0 {
			continue
		}
		ref := fmt.Sprintf("render %s: %s", p.Target.ID, states[0])
		switch {
		case p.Failure != nil:
			ref += " (" + string(p.Failure.Reason) + ")"
		case p.Target.IncompleteReason != "":
			ref += " (" + p.Target.IncompleteReason + ")"
		}
		e.stateItem(p.Target.ID, ref, states, p)
	}
}

func (e *evaluator) stateItem(target, ref string, states []string, p *render.Pair) {
	id, o, _ := e.decide(func(m Match) bool { return m.Subject == SubjectRenderState && anyIn(m.States, states) })
	it := Item{Subject: SubjectRenderState, Ref: ref, Target: target, Rule: id, Tier: o.Tier, Reason: o.Reason,
		Attrs: map[string]string{"state": states[0]}}
	if p != nil {
		it.Evidence = pairEvidence(p)
	}
	e.add(it, Review, InvRenderState)
}

func pairEvidence(p *render.Pair) []string {
	ev := []string{"render:" + p.Target.ID}
	if p.FromResult != nil && p.FromResult.Provenance.ArtifactDigest != "" {
		ev = append(ev, "from:"+p.FromResult.Provenance.ArtifactDigest)
	}
	if p.ToResult != nil && p.ToResult.Provenance.ArtifactDigest != "" {
		ev = append(ev, "to:"+p.ToResult.Provenance.ArtifactDigest)
	}
	return ev
}

// --- rendered changes ----------------------------------------------------------

func (e *evaluator) renderedChanges() {
	for _, p := range e.in.Pairs {
		if p.Diff == nil {
			continue
		}
		for _, c := range p.Diff.Changes {
			attrs := changeAttrs(c)
			id, o, _ := e.decide(func(m Match) bool { return m.Subject == SubjectRendered && matchRendered(m, c, attrs) })
			e.add(Item{Subject: SubjectRendered, Ref: c.Summary(false), Target: p.Target.ID, Rule: id, Tier: o.Tier, Reason: o.Reason,
				Attrs: attrs, Evidence: pairEvidence(p)}, AutoPass, "")
		}
	}
}

// changeAttrs derives the predicate attributes of a rendered change. Values
// never leave this function: only classifications do.
func changeAttrs(c render.Change) map[string]string {
	a := map[string]string{"class": string(c.Class), "kind": c.Object.Kind}
	if c.Class == render.ImageChanged {
		b, af := parseImage(asString(render.Unquote(c.Before))), parseImage(asString(render.Unquote(c.After)))
		a["sameRepository"] = fmt.Sprint(b.Repository == af.Repository)
		a["bump"] = imageBump(b, af)
		a["repository"] = af.Repository
	}
	return a
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func matchRendered(m Match, c render.Change, attrs map[string]string) bool {
	if len(m.Classes) > 0 && !in(m.Classes, string(c.Class)) {
		return false
	}
	if len(m.Kinds) > 0 && !in(m.Kinds, c.Object.Kind) {
		return false
	}
	if len(m.Namespaces) > 0 {
		ns := c.Object.Namespace
		if ns == "" {
			ns = "-"
		}
		if !in(m.Namespaces, ns) {
			return false
		}
	}
	if len(m.PathSuffixes) > 0 {
		ok := false
		for _, s := range m.PathSuffixes {
			if strings.HasSuffix(c.Path, s) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	if im := m.Image; im != nil {
		if c.Class != render.ImageChanged {
			return false
		}
		if im.SameRepository != nil && (attrs["sameRepository"] == "true") != *im.SameRepository {
			return false
		}
		if len(im.Repositories) > 0 && !globIn(im.Repositories, attrs["repository"]) {
			return false
		}
		if len(im.Bump) > 0 && !in(im.Bump, attrs["bump"]) {
			return false
		}
	}
	return true
}

// --- upstream changes ------------------------------------------------------------

func (e *evaluator) upstream() {
	if e.in.Edge == nil {
		e.notes = append(e.notes, "no upstream edge supplied: upstream changes were not part of this verdict")
		return
	}
	for i := range e.in.Edge.Changes {
		c := &e.in.Edge.Changes[i]
		sec := upgrade.IsSecurityItem(*c)
		id, o, matched := e.decide(func(m Match) bool { return m.Subject == SubjectUpstream && matchUpstream(m, c, sec) })
		advisory := isAdvisoryAffecting(c)
		inv := ""
		floor := AutoPass
		if advisory {
			inv, floor = InvAdvisory, Review
		}
		attrs := map[string]string{"category": string(c.Category)}
		ev := append([]string{"change:" + c.ID}, evidenceIDs(c.Evidence)...)
		if !matched {
			switch {
			case c.Routine:
				if advisory {
					break // an advisory change is never routine churn; fall through to the default
				}
				e.covered.RoutineChanges++
				continue
			case len(e.findingsOf[c.ID]) > 0 && !advisory:
				// decided through its findings: they carry the tier
				e.covered.DeferredChanges++
				continue
			}
		}
		if !(matched && c.Routine && !advisory) && (c.Breaking || c.ActionRequired) && floor == AutoPass {
			inv, floor = InvBreaking, Review
		}
		e.add(Item{Subject: SubjectUpstream, Ref: oneLine(c.Title), Rule: id, Tier: o.Tier, Reason: o.Reason, Attrs: attrs, Evidence: ev}, floor, inv)
	}
}

func matchUpstream(m Match, c *domain.Change, sec bool) bool {
	explicitRoutine := m.Routine != nil || len(m.RoutineKinds) > 0
	if c.Routine && !explicitRoutine {
		return false // only a rule that names routine changes decides them
	}
	if m.Routine != nil && *m.Routine != c.Routine {
		return false
	}
	if len(m.RoutineKinds) > 0 && !in(m.RoutineKinds, c.RoutineKind) {
		return false
	}
	if len(m.Categories) > 0 && !in(m.Categories, string(c.Category)) {
		return false
	}
	if m.Breaking != nil && *m.Breaking != c.Breaking {
		return false
	}
	if m.ActionReq != nil && *m.ActionReq != c.ActionRequired {
		return false
	}
	if m.Security != nil && *m.Security != sec {
		return false
	}
	return true
}

// isAdvisoryAffecting reports a computed advisory change saying the TARGET
// version is itself affected by an advisory.
func isAdvisoryAffecting(c *domain.Change) bool {
	return c.Provenance.Rule == upgrade.RuleAdvisoryAffected
}

// --- findings ----------------------------------------------------------------------

func (e *evaluator) findings() {
	if e.in.Report == nil {
		e.notes = append(e.notes, "no impact report supplied: findings were not part of this verdict")
		return
	}
	for i := range e.in.Report.Findings {
		f := &e.in.Report.Findings[i]
		if f.Classification == domain.ImpactNotAffected {
			e.covered.ClearedFindings++
			continue
		}
		ver := "none"
		if f.Knowledge != nil {
			ver = string(f.Knowledge.Verification)
		}
		id, o, _ := e.decide(func(m Match) bool { return m.Subject == SubjectFinding && matchFinding(m, f, ver) })
		inv, floor := "", AutoPass
		ch := e.changes[f.ChangeID]
		switch {
		case f.Classification == domain.ImpactActionRequired:
			inv, floor = InvActionRequired, Review
		case ch != nil && isAdvisoryAffecting(ch):
			inv, floor = InvAdvisory, Review
		case f.Classification == domain.ImpactUnknown && (ch == nil || !ch.Routine):
			inv, floor = InvUnknown, Review
		}
		ev := append([]string{"finding:" + f.ID}, evidenceIDs(f.UpstreamEvidence)...)
		e.add(Item{Subject: SubjectFinding, Ref: oneLine(f.Title), Rule: id, Tier: o.Tier, Reason: o.Reason,
			Attrs: map[string]string{"class": string(f.Classification), "findingRule": f.Rule, "verification": ver}, Evidence: ev}, floor, inv)
	}
}

func matchFinding(m Match, f *domain.ImpactFinding, ver string) bool {
	if len(m.FindingClasses) > 0 && !in(m.FindingClasses, string(f.Classification)) {
		return false
	}
	if len(m.FindingRules) > 0 && !in(m.FindingRules, f.Rule) {
		return false
	}
	if len(m.Verification) > 0 && !in(m.Verification, ver) {
		return false
	}
	return true
}

// --- verdict --------------------------------------------------------------------------

func (e *evaluator) verdict() *Verdict {
	v := &Verdict{Producer: Producer, Policy: e.p.Name, Digest: e.p.Digest, Tier: AutoPass, Items: e.items,
		Counts: map[Tier]int{AutoPass: 0, Review: 0, Block: 0}, Covered: e.covered, Notes: e.notes}
	if v.Items == nil {
		v.Items = []Item{}
	}
	type key struct{ rule, inv string }
	groups := map[key]*Group{}
	var order []key
	for _, it := range e.items {
		v.Counts[it.Tier]++
		v.Tier = Max(v.Tier, it.Tier)
	}
	// Why: the items at the verdict's tier (the strictest present); for an
	// auto-pass verdict, every covering rule.
	for _, it := range e.items {
		if it.Tier != v.Tier {
			continue
		}
		k := key{it.Rule, it.Invariant}
		g, ok := groups[k]
		if !ok {
			g = &Group{Rule: it.Rule, Tier: it.Tier, Reason: it.Reason, Invariant: it.Invariant}
			groups[k] = g
			order = append(order, k)
		}
		g.Count++
	}
	for _, k := range order {
		v.Why = append(v.Why, *groups[k])
	}
	sort.SliceStable(v.Why, func(i, j int) bool { return v.Why[i].Count > v.Why[j].Count })
	if v.Why == nil {
		v.Why = []Group{}
	}
	if len(e.items) == 0 {
		v.Notes = append(v.Notes, "no rendered change, uncovered upstream change or open finding: a complete render found nothing to decide")
	}
	// stable item order: strictest first, then subject, then ref
	sort.SliceStable(v.Items, func(i, j int) bool {
		a, b := v.Items[i], v.Items[j]
		if a.Tier != b.Tier {
			return a.Tier.rank() > b.Tier.rank()
		}
		if a.Subject != b.Subject {
			return a.Subject < b.Subject
		}
		return a.Ref < b.Ref
	})
	return v
}

// --- helpers ----------------------------------------------------------------------------

func in(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func anyIn(list, vals []string) bool {
	for _, v := range vals {
		if in(list, v) {
			return true
		}
	}
	return false
}

func globIn(patterns []string, s string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, s); ok {
			return true
		}
	}
	return false
}

func evidenceIDs(ids []domain.EvidenceID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 140 {
		s = string(r[:140]) + "…"
	}
	return s
}
