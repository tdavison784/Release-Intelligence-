package classify

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Policy is the policy version this package implements (Config.Policy and
// every finding's provenance producer). v2: a documentation-only render-diff
// pass accounts for the CRD description changes it names (R2.2) for any
// manifestation — and for nothing else (never R2.4, R2.6 or a clearance).
const Policy = "classify.policy@v2"

// Rule ids (InvestigationFinding.Rule).
const (
	RuleValidatedExposure    = "classify:validated-exposure"
	RuleValidatedClear       = "classify:validated-clear"
	RulePlausibleImpact      = "classify:plausible-impact"
	RuleInsufficientEvidence = "classify:insufficient-evidence"
)

// New returns the §3 classifier.
func New() investigate.Classifier { return classifier{} }

type classifier struct{}

// Classify computes one finding per in-scope change item and the
// investigation-level decision. Errors are invariant violations of the input
// (an invalid or duplicated record, a record about an unknown item, an
// action-required join verdict nothing wraps) — a broken input is never
// classified around.
func (classifier) Classify(ctx context.Context, in investigate.ClassifyInput) (*investigate.Classification, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	x, err := newIndex(in)
	if err != nil {
		return nil, investigate.PermanentError(domain.StageClassify, err)
	}
	out := &investigate.Classification{Findings: []domain.InvestigationFinding{}}
	for _, id := range in.Bundle.InScope() {
		f, err := x.classify(id)
		if err != nil {
			return nil, investigate.PermanentError(domain.StageClassify, err)
		}
		out.Findings = append(out.Findings, f)
	}
	out.Decision = x.decide(out.Findings)
	return out, nil
}

// --- input index --------------------------------------------------------------------------

type index struct {
	in     investigate.ClassifyInput
	b      *domain.EvidenceBundle
	ev     map[domain.EvidenceID]domain.Evidence
	vals   map[string][]domain.ValidationRecord // by change item, sorted by id
	asrts  map[string][]domain.AgentAssertion
	hyps   map[string][]domain.Hypothesis
	envRCs map[string][]string // change item → env-scope rendered changes correlated to it, sorted
	rcs    map[string]domain.RenderedChange
	rcPair map[string]domain.ManifestPair // env rendered change → its pair
}

func newIndex(in investigate.ClassifyInput) (*index, error) {
	b := in.Bundle
	if b == nil {
		return nil, errors.New("classify: no bundle")
	}
	if p := in.Config.Policy; p != "" && p != Policy {
		return nil, fmt.Errorf("classify: unsupported policy %q (this classifier implements %s)", p, Policy)
	}
	x := &index{in: in, b: b, ev: b.EvidenceByID(), vals: map[string][]domain.ValidationRecord{},
		asrts: map[string][]domain.AgentAssertion{}, hyps: map[string][]domain.Hypothesis{},
		envRCs: map[string][]string{}, rcs: b.RenderedChanges(), rcPair: map[string]domain.ManifestPair{}}
	known := func(what, item string) error {
		if _, ok := b.Item(item); !ok {
			return fmt.Errorf("classify: %s is about unknown change item %q", what, item)
		}
		return nil
	}
	seen := map[string]bool{}
	for _, v := range in.Validations {
		if err := v.Validate(); err != nil {
			return nil, fmt.Errorf("classify: invalid validation record: %w", err)
		}
		if seen[v.ID] {
			return nil, fmt.Errorf("classify: duplicate validation record %s", v.ID)
		}
		seen[v.ID] = true
		if err := known("validation "+v.ID, v.ChangeItem); err != nil {
			return nil, err
		}
		x.vals[v.ChangeItem] = append(x.vals[v.ChangeItem], v)
	}
	if a := in.Agent; a != nil {
		for _, e := range a.Evidence {
			x.ev[e.ID] = e
		}
		for _, h := range a.Hypotheses {
			if err := h.Validate(); err != nil {
				return nil, fmt.Errorf("classify: invalid hypothesis: %w", err)
			}
			if seen[h.ID] {
				return nil, fmt.Errorf("classify: duplicate hypothesis %s", h.ID)
			}
			seen[h.ID] = true
			if err := known("hypothesis "+h.ID, h.ChangeItem); err != nil {
				return nil, err
			}
			x.hyps[h.ChangeItem] = append(x.hyps[h.ChangeItem], h)
		}
		for _, as := range a.Assertions {
			if err := as.Validate(); err != nil {
				return nil, fmt.Errorf("classify: invalid agent assertion: %w", err)
			}
			if seen[as.ID] {
				return nil, fmt.Errorf("classify: duplicate agent assertion %s", as.ID)
			}
			seen[as.ID] = true
			if err := known("assertion "+as.ID, as.ChangeItem); err != nil {
				return nil, err
			}
			x.asrts[as.ChangeItem] = append(x.asrts[as.ChangeItem], as)
		}
	}
	for k := range x.vals {
		sort.Slice(x.vals[k], func(i, j int) bool { return x.vals[k][i].ID < x.vals[k][j].ID })
	}
	for k := range x.asrts {
		sort.Slice(x.asrts[k], func(i, j int) bool { return x.asrts[k][i].ID < x.asrts[k][j].ID })
	}
	for k := range x.hyps {
		sort.Slice(x.hyps[k], func(i, j int) bool { return x.hyps[k][i].ID < x.hyps[k][j].ID })
	}
	for _, p := range b.Renders {
		for _, c := range p.Changes {
			x.rcPair[c.ID] = p
			for _, ci := range c.ChangeItems {
				x.envRCs[ci] = append(x.envRCs[ci], c.ID)
			}
		}
	}
	for k := range x.envRCs {
		x.envRCs[k] = uniqSorted(x.envRCs[k])
	}
	return x, nil
}

// --- per-item evaluation ------------------------------------------------------------------

// itemEval is everything the rules read about one change item.
type itemEval struct {
	item    domain.ChangeItem
	envRCs  []string
	roots   []domain.ExposureRoot
	paths   []domain.ImpactPath
	vals    []domain.ValidationRecord
	asrts   []domain.AgentAssertion
	hyps    []domain.Hypothesis
	joins   []domain.ImpactFinding
	counted []domain.ValidationRecord // passing validations that count for R2 (R2.4 manifestation rule)
	// docs are documentation-only passes that account for at least one
	// rendered change (R2.2 only; never counted for R2.4 or R2.6).
	docs    []domain.ValidationRecord
	covered map[string]bool // "path:<id>", "rc:<id>", "root:<node>", "answer:<assertion>"
	gaps    []domain.EvidenceGap
	steps   []string
	assume  []string // claims-mitigation assertions no counting validation established
	// scope gaps say what the deterministic join does not evaluate (an
	// impact:not-joined verdict). They are listed when the item is not
	// cleared for another reason, and never block R2 by themselves.
	scope      []domain.EvidenceGap
	scopeSteps []string
}

// counts reports whether a validation counts for the item's R2 checks:
// a pass, and — for an item that does not manifest in rendered configuration
// — one that clears behaviour (an established pin or an observed outcome).
func (e *itemEval) counts(v domain.ValidationRecord) bool {
	if v.Outcome != domain.ValidationPass || v.DocumentationOnly {
		return false
	}
	return e.item.Manifestation.RenderDecisive() || v.ClearsBehaviour()
}

func (e *itemEval) gap(kind domain.GapKind, subject, detail string, reason domain.UnknownReason, step string) {
	e.gaps = append(e.gaps, domain.EvidenceGap{Kind: kind, Subject: subject, Detail: detail, UnknownReason: reason})
	if step != "" {
		e.steps = append(e.steps, step)
	}
}

func (x *index) classify(id string) (domain.InvestigationFinding, error) {
	it, _ := x.b.Item(id)
	e := &itemEval{item: it, envRCs: x.envRCs[id], vals: x.vals[id], asrts: x.asrts[id], hyps: x.hyps[id], covered: map[string]bool{}}
	if t := x.in.Traversal; t != nil {
		e.roots, e.paths = t.RootsOf(id), t.PathsOf(id)
	}
	if x.in.Report != nil {
		e.joins = joinFindings(x.in.Report, it)
	}
	for _, v := range e.vals {
		if v.Outcome == domain.ValidationPass && v.DocumentationOnly {
			// R2.2 only, and only for what the bundle itself shows is
			// documentation-only (the record's claim is re-checked).
			n := 0
			for _, rc := range v.RenderedChanges {
				if ok, _ := x.b.DocumentationOnlyChange(rc); ok {
					e.covered["rc:"+rc] = true
					n++
				}
			}
			if n > 0 {
				e.docs = append(e.docs, v)
			}
			continue
		}
		if !e.counts(v) {
			continue
		}
		e.counted = append(e.counted, v)
		for _, p := range v.Paths {
			e.covered["path:"+p] = true
		}
		for _, rc := range v.RenderedChanges {
			e.covered["rc:"+rc] = true
		}
		for _, s := range v.Subjects {
			e.covered["root:"+s] = true
		}
		for _, a := range v.Answers {
			e.covered["answer:"+a] = true
		}
	}

	var failing []domain.ValidationRecord
	for _, v := range e.vals {
		if v.Outcome == domain.ValidationFail {
			failing = append(failing, v)
		}
	}
	actionRequired := false
	for _, f := range e.joins {
		actionRequired = actionRequired || f.Classification == domain.ImpactActionRequired
	}
	if actionRequired && len(failing) == 0 {
		// §3.5: ACTION REQUIRED is never weakened. The join's verdict reaches
		// the policy as a wrapped failing validation (validate.join); without
		// it the item cannot be classified honestly.
		return domain.InvestigationFinding{}, fmt.Errorf("classify: change item %s: the deterministic join classifies its change action-required, but no failing validation wraps that verdict (is the join validator enabled?)", id)
	}

	x.r2(e) // collects gaps, steps and assumptions for every class
	if len(failing) > 0 || len(e.gaps) > 0 || len(e.assume) > 0 {
		e.gaps, e.steps = append(e.gaps, e.scope...), append(e.steps, e.scopeSteps...)
	}
	f := x.base(e)
	switch {
	case len(failing) > 0: // R1
		f.Class, f.Rule = domain.FindingAffected, RuleValidatedExposure
		f.Validations = ids(failing)
		f.Basis = strongest(failing)
		f.Gaps = contextGaps(e.gaps)
		f.Reasoning = x.reasonAffected(e, failing)
		f.Remediation = x.remediation(e, failing)
		f.NextSteps = x.nextSteps(e, false)
	case len(e.gaps) == 0 && len(e.assume) == 0: // R2
		f.Class, f.Rule = domain.FindingNotAffected, RuleValidatedClear
		cited := append(append([]domain.ValidationRecord(nil), e.counted...), e.docs...)
		f.Validations = ids(cited)
		f.Basis = strongest(cited)
		f.Reasoning = x.reasonClear(e)
	default:
		if signals := x.plausible(e); len(signals) > 0 { // R3
			f.Class, f.Rule = domain.FindingPotentiallyAffected, RulePlausibleImpact
			f.Reasoning = x.reasonOpen(e, "R3", RulePlausibleImpact, f.Class, signals)
		} else { // R4
			f.Class, f.Rule = domain.FindingInsufficientEvidence, RuleInsufficientEvidence
			f.Reasoning = x.reasonOpen(e, "R4", RuleInsufficientEvidence, f.Class, nil)
		}
		f.Validations = ids(e.vals)
		f.Gaps = append(append([]domain.EvidenceGap(nil), e.gaps...), x.agentGaps()...)
		f.NextSteps = x.nextSteps(e, true)
		f.Remediation = x.remediation(e, nil)
	}
	f.Assumptions = e.assume
	f.ID = domain.InvestigationFindingID(f.Environment, f.ChangeItem, f.Class, f.Rule)
	f.Provenance = domain.Provenance{Method: domain.MethodComputed, Producer: Policy, Rule: f.Rule, Confidence: confidence(f.Class)}
	f.Evidence = x.findingEvidence(e, f.Validations)
	f.Artifacts = x.findingArtifacts(e, f.Validations)
	f.Approval = approval(f)
	return f, nil
}

// base fills what every class carries.
func (x *index) base(e *itemEval) domain.InvestigationFinding {
	f := domain.InvestigationFinding{
		ChangeItem:      e.item.ID,
		Manifestation:   e.item.Manifestation,
		Environment:     x.b.Snapshot.Environment,
		RenderedChanges: e.envRCs,
	}
	var nodes []string
	for _, r := range e.roots {
		nodes = append(nodes, r.Node)
	}
	for _, p := range e.paths {
		f.Paths = append(f.Paths, p.ID)
		nodes = append(nodes, p.Nodes...)
	}
	f.Components = uniqSorted(nodes)
	f.Paths = uniqSorted(f.Paths)
	for _, a := range e.asrts {
		f.Assertions = append(f.Assertions, a.ID)
	}
	for _, j := range e.joins {
		f.ImpactFindings = append(f.ImpactFindings, domain.ImpactFindingRef{ID: j.ID, Classification: j.Classification, Rule: j.Rule})
	}
	return f
}

// r2 evaluates R2.1–R2.7 for the item, recording every unmet condition as an
// evidence gap with the next step that would meet it (R2.8 is the
// action-required check in classify, which precedes it).
func (x *index) r2(e *itemEval) {
	x.collection(e) // R2.1

	// R2.2: every environment rendered change correlated to the item is shown
	// harmless by a passing (counting) validation.
	for _, id := range e.envRCs {
		if e.covered["rc:"+id] {
			continue
		}
		c := x.rcs[id]
		e.gap(domain.GapValidation, id,
			fmt.Sprintf("environment rendered change %s (%s %s %s) is shown harmless by no %s", id, c.Class, c.Object, c.Path, x.passWord(e)),
			x.validationReason(e),
			fmt.Sprintf("Show environment rendered change %s (%s on %s%s) harmless for the environment — %s.", id, c.Class, c.Object, pathSuffix(c.Path), x.suggest(e)))
	}

	// R2.3: the traversal is complete for the item.
	t := x.in.Traversal
	switch {
	case t == nil:
		e.gap(domain.GapTraversal, "traversal", "no relationship traversal ran: consumers of the item are unknown", domain.UnknownEvidenceGap,
			"Build the relationship graph and run the bounded traversal for the item.")
	case t.Truncated:
		e.gap(domain.GapTraversal, t.ID, "the traversal was truncated: "+t.TruncationReason, domain.UnknownEvidenceGap,
			fmt.Sprintf("Re-run the traversal with a larger bound (truncated: %s).", t.TruncationReason))
	}
	if t != nil {
		unresolved := map[string]bool{}
		for _, id := range t.Unresolved {
			unresolved[id] = true
		}
		done := map[string]bool{}
		for _, p := range e.paths {
			if p.Status.Resolved() {
				continue
			}
			for _, eid := range p.Edges {
				if !unresolved[eid] || done[eid] {
					continue
				}
				done[eid] = true
				e.gap(domain.GapRelationship, eid, fmt.Sprintf("edge %s on impact path %s is not established (path status %s): an inferred or missing relationship", eid, p.ID, p.Status),
					domain.UnknownEnvironmentVisibilityGap,
					fmt.Sprintf("Establish relationship %s from environment evidence (supply the object at its other end) or validate impact path %s directly.", eid, p.ID))
			}
		}
	}

	// R2.4: paths (else roots, else the item) covered by a counting pass.
	for _, p := range e.paths {
		if !e.covered["path:"+p.ID] {
			e.gap(domain.GapValidation, p.ID, fmt.Sprintf("impact path %s (%d node(s)) is covered by no %s", p.ID, len(p.Nodes), x.passWord(e)),
				x.validationReason(e), fmt.Sprintf("Validate impact path %s — %s.", p.ID, x.suggest(e)))
		}
	}
	if len(e.paths) == 0 {
		for _, r := range e.roots {
			if !e.covered["root:"+r.Node] {
				e.gap(domain.GapValidation, r.Node, fmt.Sprintf("exposure root %s is covered by no %s", r.Node, x.passWord(e)),
					x.validationReason(e), fmt.Sprintf("Validate exposure root %s — %s.", r.Node, x.suggest(e)))
			}
		}
	}
	if len(e.counted) == 0 {
		detail := "no " + x.passWord(e) + " of the item exists"
		if !e.item.Manifestation.RenderDecisive() {
			detail += fmt.Sprintf(" (the item manifests as %s: a render that shows nothing proves nothing; only an override inspection that establishes a pinned setting, or an observed validation, clears it)", e.item.Manifestation)
		}
		if nr := notRun(e.vals); nr != "" {
			detail += "; validators that could not decide: " + nr
		}
		e.gap(domain.GapValidation, e.item.ID, detail, x.validationReason(e), "")
	}

	// R2.5: no failing (R1 took those) or erroring validation.
	for _, v := range e.vals {
		if v.Outcome == domain.ValidationError {
			e.gap(domain.GapValidation, v.ID, fmt.Sprintf("validator %s errored: %s", v.Validator, firstLine(v.Detail)), domain.UnknownEvidenceGap,
				fmt.Sprintf("Re-run %s (record %s errored: %s).", v.Validator, v.ID, firstLine(v.Detail)))
		}
	}

	// R2.6 / R2.7: agent records (§3.3).
	for _, a := range e.asrts {
		answered := e.covered["answer:"+a.ID]
		switch a.Stance {
		case domain.StanceRaisesConcern:
			if !answered {
				e.gap(domain.GapAgent, a.ID, "unanswered agent concern (AI, not a validation): "+clip(a.Statement, 240), "",
					fmt.Sprintf("Answer agent concern %s with a %s that names it (request_validation): %q.", a.ID, x.passWord(e), clip(a.Statement, 160)))
			}
		case domain.StanceClaimsMitigation:
			if !answered {
				e.assume = append(e.assume, a.ID)
				e.gap(domain.GapAgent, a.ID, "unvalidated assumption — the agent claims a mitigation no validation established (AI, not a validation): "+clip(a.Statement, 240), "",
					fmt.Sprintf("Establish or refute the agent's mitigation claim %s with a validation that names it — for a setting, an override inspection: %q.", a.ID, clip(a.Statement, 160)))
			}
		}
	}

	// §3.5: an unknown join verdict is a gap signal.
	for _, j := range e.joins {
		if j.Classification != domain.ImpactUnknown {
			continue
		}
		need := strings.Join(j.NeededToDetermine, "; ")
		if need == "" {
			need = firstLine(j.Detail)
		}
		step := ""
		if need != "" {
			step = fmt.Sprintf("Supply what the deterministic join needs to decide %s: %s.", j.ID, need)
		}
		g := domain.EvidenceGap{Kind: domain.GapValidation, Subject: j.ID, UnknownReason: j.UnknownReason,
			Detail: fmt.Sprintf("the deterministic join could not decide the change (%s): %s", j.Rule, orDash(need))}
		if j.Rule == ruleJoinNotJoined {
			// The join does not evaluate this kind of change at all (a declared
			// change, a diff rule without a join rule, a change without
			// subjects): a statement about the join's scope, not missing
			// environment evidence. The investigation decides such items with
			// its own evidence — R2.1–R2.8 still hold in full (for a behaviour
			// item: pinned settings or observed passes covering every path,
			// root and rendered change) — so this does not block R2.
			e.scope = append(e.scope, g)
			if step != "" {
				e.scopeSteps = append(e.scopeSteps, step)
			}
			continue
		}
		e.gap(g.Kind, g.Subject, g.Detail, g.UnknownReason, step)
	}
}

// ruleJoinNotJoined is impact.RuleNotJoined: the join's verdict for a change
// it does not evaluate.
const ruleJoinNotJoined = "impact:not-joined"

// plausible lists the R3 plausible-impact signals of the item.
func (x *index) plausible(e *itemEval) []string {
	var s []string
	if n := len(e.envRCs); n > 0 {
		s = append(s, fmt.Sprintf("%d environment rendered change(s) correlated to the item", n))
	}
	if n := len(e.paths); n > 0 {
		s = append(s, fmt.Sprintf("%d impact path(s) from %d exposure root(s)", n, len(e.roots)))
	} else if n := len(e.roots); n > 0 {
		s = append(s, fmt.Sprintf("%d exposure root(s)", n))
	}
	for _, j := range e.joins {
		switch {
		case j.Classification == domain.ImpactReviewRequired || j.Classification == domain.ImpactInformational:
			s = append(s, fmt.Sprintf("join finding %s is %s (%s)", j.ID, j.Classification, j.Rule))
		case j.Classification == domain.ImpactUnknown && len(j.Matches) > 0:
			s = append(s, fmt.Sprintf("join finding %s is unknown with %d environment match(es)", j.ID, len(j.Matches)))
		}
	}
	exposed := map[string]bool{}
	for _, r := range e.roots {
		exposed[r.Node] = true
	}
	for _, p := range e.paths {
		exposed["path:"+p.ID] = true
		for _, n := range p.Nodes {
			exposed[n] = true
		}
	}
	for _, rc := range e.envRCs {
		exposed["rc:"+rc] = true
	}
	for _, v := range e.vals {
		if v.Outcome != domain.ValidationError {
			continue
		}
		hit := false
		for _, n := range v.Subjects {
			hit = hit || exposed[n]
		}
		for _, p := range v.Paths {
			hit = hit || exposed["path:"+p]
		}
		for _, rc := range v.RenderedChanges {
			hit = hit || exposed["rc:"+rc]
		}
		if hit {
			s = append(s, fmt.Sprintf("validation %s (%s) is inconclusive on an exposed subject", v.ID, v.Validator))
		}
	}
	for _, a := range e.asrts {
		if a.Stance == domain.StanceRaisesConcern && !e.covered["answer:"+a.ID] {
			s = append(s, fmt.Sprintf("unanswered agent concern %s (AI)", a.ID))
		}
	}
	return s
}

// agentGaps states, on an open finding, that the agent stage did not
// complete (nobody looked for the consumers and mechanisms the
// deterministic layers miss). Not an R2 condition (§3.4 is), so only R3/R4
// findings carry it.
func (x *index) agentGaps() []domain.EvidenceGap {
	a := x.in.Agent
	switch {
	case a == nil || a.Run.Status == "":
		return []domain.EvidenceGap{{Kind: domain.GapAgent, Subject: "agent", Detail: "the investigation agent did not run (agent stage disabled or not reached)"}}
	case a.Run.Status != domain.AgentSubmitted:
		return []domain.EvidenceGap{{Kind: domain.GapAgent, Subject: "agent", Detail: fmt.Sprintf("the investigation agent did not complete (%s: %s)", a.Run.Status, orDash(a.Run.Reason))}}
	}
	return nil
}

// contextGaps keeps, on an affected finding, the gaps that describe
// uncertainty about the environment (collection, traversal, relationships);
// "not cleared" gaps are moot once a failure is demonstrated.
func contextGaps(gs []domain.EvidenceGap) []domain.EvidenceGap {
	var out []domain.EvidenceGap
	for _, g := range gs {
		switch g.Kind {
		case domain.GapCollection, domain.GapExposure, domain.GapTraversal, domain.GapRelationship:
			out = append(out, g)
		}
	}
	return out
}

func approval(f domain.InvestigationFinding) domain.ApprovalEligibility {
	if f.Class == domain.FindingNotAffected {
		return domain.ApprovalEligibility{Eligible: true}
	}
	a := domain.ApprovalEligibility{RequiresHumanReview: true,
		Blockers: []string{fmt.Sprintf("class %s (%s): only a not-affected finding is approval-eligible", f.Class, f.Rule)}}
	for _, g := range f.Gaps {
		a.Blockers = append(a.Blockers, fmt.Sprintf("gap %s: %s", g.Kind, g.Subject))
	}
	return a
}

func confidence(c domain.FindingClass) domain.Confidence {
	switch c {
	case domain.FindingAffected, domain.FindingNotAffected:
		return domain.ConfidenceHigh
	case domain.FindingPotentiallyAffected:
		return domain.ConfidenceMedium
	}
	return domain.ConfidenceLow
}

// strongest is the strongest level among decisive validations (observed >
// deterministic), as Investigation.Validate computes a finding's basis.
func strongest(vs []domain.ValidationRecord) domain.ValidationLevel {
	var out domain.ValidationLevel
	for _, v := range vs {
		if v.Outcome.Decisive() && (out == "" || v.Level == domain.LevelObserved) {
			out = v.Level
		}
	}
	return out
}

// findingEvidence: the item's upstream chain, the rendered-diff evidence of
// its environment rendered changes and the evidence of the validations the
// finding cites (both resolve in the bundle or the run pool).
func (x *index) findingEvidence(e *itemEval, validations []string) []domain.EvidenceID {
	var out []string
	for _, id := range e.item.Evidence {
		out = append(out, string(id))
	}
	for _, rc := range e.envRCs {
		for _, id := range x.rcs[rc].Evidence {
			out = append(out, string(id))
		}
	}
	cited := map[string]bool{}
	for _, id := range validations {
		cited[id] = true
	}
	for _, v := range e.vals {
		if cited[v.ID] {
			for _, id := range v.Evidence {
				out = append(out, string(id))
			}
		}
	}
	var ev []domain.EvidenceID
	for _, id := range uniqSorted(out) {
		ev = append(ev, domain.EvidenceID(id))
	}
	return ev
}

// findingArtifacts: the item's artifacts, the diff of every pair holding one
// of its environment rendered changes, and the outputs of cited validations.
func (x *index) findingArtifacts(e *itemEval, validations []string) []string {
	out := append([]string(nil), e.item.Artifacts...)
	for _, rc := range e.envRCs {
		if p, ok := x.rcPair[rc]; ok && p.Diff != "" {
			out = append(out, p.Diff)
		}
	}
	cited := map[string]bool{}
	for _, id := range validations {
		cited[id] = true
	}
	for _, v := range e.vals {
		if cited[v.ID] {
			out = append(out, v.Outputs...)
		}
	}
	return uniqSorted(out)
}

// joinFindings returns the report findings of the item's change, sorted by
// id. A compatibility item (anchored on a platform) takes the platform checks
// no change id names. (Same mapping as validate.join; duplicated because
// lanes never import each other's packages.)
func joinFindings(rep *domain.ImpactReport, item domain.ChangeItem) []domain.ImpactFinding {
	var out []domain.ImpactFinding
	for _, f := range rep.Findings {
		switch {
		case item.ChangeID != "":
			if f.ChangeID == item.ChangeID {
				out = append(out, f)
			}
		case item.Platform != "":
			if f.ChangeID == "" && platformFinding(f, item.Platform) {
				out = append(out, f)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func platformFinding(f domain.ImpactFinding, anchor string) bool {
	platform, _, _ := strings.Cut(anchor, "/")
	for _, c := range f.Checks {
		if c.Dimension == domain.DimensionCluster && strings.EqualFold(c.Platform, platform) {
			return true
		}
	}
	for _, m := range f.Matches {
		if m.Kind == domain.MatchKubernetes && strings.EqualFold(platform, "kubernetes") {
			return true
		}
	}
	return false
}

// --- small helpers ------------------------------------------------------------------------

func ids(vs []domain.ValidationRecord) []string {
	var out []string
	for _, v := range vs {
		out = append(out, v.ID)
	}
	return uniqSorted(out)
}

func uniqSorted(xs []string) []string {
	if len(xs) == 0 {
		return nil
	}
	s := append([]string(nil), xs...)
	sort.Strings(s)
	out := s[:0]
	for i, v := range s {
		if v != "" && (i == 0 || v != s[i-1]) {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func uniqKeepOrder(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func notRun(vs []domain.ValidationRecord) string {
	var parts []string
	for _, v := range vs {
		if v.Outcome == domain.ValidationNotRun {
			parts = append(parts, fmt.Sprintf("%s (%s)", v.Validator, clip(firstLine(v.Detail), 160)))
		}
	}
	return strings.Join(parts, "; ")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return strings.TrimSuffix(s, ":")
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func pathSuffix(p string) string {
	if p == "" {
		return ""
	}
	return " at " + p
}
