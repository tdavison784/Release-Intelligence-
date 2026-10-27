package formalize

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/agent"
)

// Kinds of formalized hypothesis.
const (
	KindRule            = "rule"
	KindUnrepresentable = "unrepresentable"
)

// Answer is the model's structured answer (prompts/formalize/v1/schema.json).
type Answer struct {
	Hypotheses         []AnswerHypothesis `json:"hypotheses"`
	NoHypothesisReason string             `json:"noHypothesisReason"`
	// RoleDefaults (formalize/v2) are what unset consumer-role fields mean,
	// each established by citable upstream evidence (refine.go).
	RoleDefaults []AnswerRoleDefault `json:"roleDefaults,omitempty"`
}

// AnswerRoleDefault is the evidenced default of one consumer-role field.
type AnswerRoleDefault struct {
	Path    string              `json:"path"`
	Default domain.FieldDefault `json:"default"`
}

// AnswerHypothesis is one hypothesis of the answer.
type AnswerHypothesis struct {
	Kind                  string               `json:"kind"`
	Claim                 string               `json:"claim"`
	Citations             []string             `json:"citations"`
	Conditions            []string             `json:"conditions"`
	Rule                  *domain.ProposedRule `json:"rule"`
	EnvironmentBasis      []string             `json:"environmentBasis"`
	RequestedVerification string               `json:"requestedVerification"`
	Limitation            string               `json:"limitation"`
	ProposedVerification  string               `json:"proposedVerification"`
	Keeps                 []string             `json:"keeps"`
	// Discharges are the ids of the `default` uncertainties this
	// hypothesis' rule settles with an evidenced Condition.Default.
	Discharges []string `json:"discharges"`
}

// decode reads an answer strictly: an unknown field anywhere (also inside
// a rule) is a violation, not ignored.
func decode(text string) (*Answer, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(strings.TrimSpace(text))))
	dec.DisallowUnknownFields()
	var a Answer
	if err := dec.Decode(&a); err != nil {
		return nil, fmt.Errorf("the answer is not the schema's JSON document: %v", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("the answer has content after the JSON document")
	}
	return &a, nil
}

// checked is an answer hypothesis after the floor: its rule with the kept
// uncertainties written into its undecidable leaves, its citations, and for
// a rule the checked spec.
type checked struct {
	h     AnswerHypothesis
	rule  *domain.ProposedRule
	cites []domain.Evidence
	spec  *domain.MitigationSpec
	kept  []string // uncertainty ids kept by this hypothesis
	// discharged are the default uncertainties its rule settles with an
	// evidenced default; defaultCites the evidence of its defaults.
	discharged   []string
	defaultCites []domain.Evidence
}

// Floor checks an answer against the input (mission D4) and returns the
// violations; nothing is repaired. Every citation must be evidence the
// assessment established; every uncertainty must be kept (an undecidable
// leaf or an unrepresentable record), except a default uncertainty a rule
// settles with an evidenced default; a rule must pass the spec checks
// (agent.CheckMitigation: complete, decidable, a subject of the run's
// product found in the bundle, bounds within the run, a resolving basis, an
// upstream citation); a stated default cites upstream evidence the
// assessment established; no rule treats an unset field as a value without
// stating its default.
func Floor(ctx context.Context, p *Precomputed, a *Answer) []string {
	_, v := floor(ctx, p, a)
	return v
}

// floorOut is what the floor accepted: the hypotheses and the evidenced
// role-field defaults (by path).
type floorOut struct {
	hyps         []checked
	roleDefaults map[string]domain.FieldDefault
}

func floor(ctx context.Context, p *Precomputed, a *Answer) (floorOut, []string) {
	var violations []string
	bad := func(format string, args ...any) { violations = append(violations, fmt.Sprintf(format, args...)) }
	out := floorOut{roleDefaults: p.roleDefaults(a.RoleDefaults, bad)}
	if len(a.Hypotheses) == 0 {
		if strings.TrimSpace(a.NoHypothesisReason) == "" {
			bad("no hypothesis and no noHypothesisReason: say why the assessment states no testable claim")
		}
		return out, violations
	}
	if strings.TrimSpace(a.NoHypothesisReason) != "" {
		bad("noHypothesisReason is only for an answer without hypotheses")
	}
	kept, discharged := map[string]bool{}, map[string]bool{}
	claims := map[string]int{}
	for i, h := range a.Hypotheses {
		at := func(format string, args ...any) { bad("hypotheses[%d]: %s", i, fmt.Sprintf(format, args...)) }
		c := checked{h: h}
		claim := strings.TrimSpace(h.Claim)
		if claim == "" {
			at("claim is required")
		} else if j, dup := claims[claim]; dup {
			at("repeats the claim of hypotheses[%d]", j)
		} else {
			claims[claim] = i
		}
		if len(h.Citations) == 0 {
			at("cites no evidence")
		}
		for _, id := range h.Citations {
			if !p.allowed[domain.EvidenceID(id)] {
				at("citation %q is not evidence the assessment established (its verified, behaviour or effective-configuration evidence): cite only ids from `evidence`", id)
				continue
			}
			c.cites = append(c.cites, p.pool[domain.EvidenceID(id)])
		}
		if h.Rule != nil {
			r := *h.Rule
			r.Assertion = keepLeaves(p, r.Assertion, &c.kept)
			c.rule = &r
			if why := p.roleNarrowing(r.Assertion); why != "" {
				at("%s", why)
			}
			for _, why := range unevidencedDefaults(r.Assertion) {
				at("%s", why)
			}
			var why []string
			c.defaultCites, why = p.defaults(r.Assertion)
			for _, w := range why {
				at("%s", w)
			}
		}
		for _, id := range h.Discharges {
			u, ok := p.uncertainty(id)
			switch {
			case !ok || u.Kind != UncertaintyDefault:
				at("discharges %q, which is not a default uncertainty id", id)
			case c.rule == nil || len(ruleDefaults(c.rule.Assertion)) == 0:
				at("discharges %s but its rule states no default (a value leaf with `default` citing the evidence that establishes it)", id)
			default:
				c.discharged = append(c.discharged, id)
			}
		}
		switch h.Kind {
		case KindRule:
			if strings.TrimSpace(h.Limitation) != "" || strings.TrimSpace(h.ProposedVerification) != "" || len(h.Keeps) != 0 {
				at("a rule takes no limitation, proposedVerification or keeps (an uncertainty the claim depends on makes it unrepresentable)")
			}
			if c.rule == nil {
				at("kind rule needs a rule")
				break
			}
			spec, why := agent.CheckMitigation(ctx, p.env(), p.item.ID, investigate.MitigationInput{ProposedRule: *c.rule,
				EnvironmentBasis: h.EnvironmentBasis, RequestedVerification: h.RequestedVerification}, c.cites)
			if why != "" {
				at("the rule fails the spec checks: %s", why)
			}
			c.spec = spec
		case KindUnrepresentable:
			if len(h.EnvironmentBasis) != 0 || strings.TrimSpace(h.RequestedVerification) != "" {
				at("an unrepresentable record takes no environmentBasis or requestedVerification")
			}
			if strings.TrimSpace(h.Limitation) == "" || strings.TrimSpace(h.ProposedVerification) == "" {
				at("an unrepresentable record states its grammar limitation and a proposedVerification")
			}
			for _, id := range h.Keeps {
				if _, ok := p.uncertainty(id); !ok {
					at("keeps %q, which is not an uncertainty id", id)
					continue
				}
				c.kept = append(c.kept, id)
			}
			if c.rule != nil {
				for _, why := range partialRule(p, *c.rule) {
					at("rule: %s", why)
				}
			}
		default:
			at("unknown kind %q (rule or unrepresentable)", h.Kind)
		}
		if h.Kind == KindUnrepresentable {
			for _, id := range c.kept {
				kept[id] = true
			}
		}
		for _, id := range c.discharged {
			discharged[id] = true
		}
		out.hyps = append(out.hyps, c)
	}
	for _, u := range p.Uncertainties {
		if !kept[u.ID] && !discharged[u.ID] {
			bad("uncertainty %s (%s: %q) is dropped: keep it as an undecidable leaf (needed = its id) in an unrepresentable record's rule, or in an unrepresentable record's keeps%s",
				u.ID, u.Kind, clip(u.Text, 200), map[bool]string{true: ", or settle it with an evidenced default in a rule (discharges)"}[u.Kind == UncertaintyDefault])
		}
	}
	return out, violations
}

// keepLeaves writes the uncertainty's text into every undecidable leaf
// whose needed names an uncertainty id, and records the id as kept.
func keepLeaves(p *Precomputed, a domain.SemanticAssertion, kept *[]string) domain.SemanticAssertion {
	if a.Applicability == nil {
		return a
	}
	var walk func(c domain.Condition) domain.Condition
	walk = func(c domain.Condition) domain.Condition {
		if c.Op == domain.OpUndecidable {
			if u, ok := p.uncertainty(strings.TrimSpace(c.Needed)); ok {
				c.Needed = u.Text
				*kept = append(*kept, u.ID)
			}
		}
		if len(c.Of) > 0 {
			of := make([]domain.Condition, len(c.Of))
			for i, x := range c.Of {
				of[i] = walk(x)
			}
			c.Of = of
		}
		return c
	}
	ap := domain.Applicability{Exposure: walk(a.Applicability.Exposure)}
	if a.Applicability.Overlap != nil {
		o := walk(*a.Applicability.Overlap)
		ap.Overlap = &o
	}
	a.Applicability = &ap
	return a
}

// partialRule checks the rule an unrepresentable record keeps: well-formed
// as far as it goes (undecidable leaves allowed), of the run's product,
// within the run's bounds.
func partialRule(p *Precomputed, r domain.ProposedRule) []string {
	var out []string
	if err := r.Assertion.Validate(false); err != nil {
		out = append(out, err.Error())
	}
	if s := r.Assertion.Subject; s != nil && s.Product != p.Transition.Product {
		out = append(out, fmt.Sprintf("subject product %q is not this investigation's product %q", s.Product, p.Transition.Product))
	}
	if why := agent.BoundsWithinRun(r.Release, p.Transition.From, p.Transition.To); why != "" {
		out = append(out, why)
	}
	return out
}

// valueStates are the leaf states that test a set value.
var valueStates = map[domain.FieldState]bool{domain.StateEquals: true, domain.StateNotEquals: true, domain.StateMatches: true,
	domain.StateHasToken: true, domain.StateHasTokenKey: true}

// unevidencedDefaults finds disjunctions that treat an unset field as a
// value: `any` of a leaf that tests the field unset (or not set) and a leaf
// that tests a value of the same field. What an unset field means is a
// default, and a default needs evidence the grammar cannot carry yet.
func unevidencedDefaults(a domain.SemanticAssertion) []string {
	if a.Applicability == nil {
		return nil
	}
	key := func(c domain.Condition) string {
		return strings.Join([]string{string(c.Op), c.Group, c.Kind, c.Name, c.Path, c.Component}, "\x00")
	}
	var out []string
	var walk func(c domain.Condition)
	walk = func(c domain.Condition) {
		if c.Op == domain.OpAny {
			unset, value := map[string]bool{}, map[string]bool{}
			for _, x := range c.Of {
				switch {
				case x.State == domain.StateUnset:
					unset[key(x)] = true
				case x.Op == domain.OpNot && len(x.Of) == 1 && x.Of[0].State == domain.StateSet:
					unset[key(x.Of[0])] = true
				case valueStates[x.State]:
					value[key(x)] = true
				}
			}
			var fields []string
			for k := range unset {
				if value[k] {
					parts := strings.Split(k, "\x00")
					fields = append(fields, strings.TrimSpace(parts[0]+" "+strings.Join(nonEmpty(parts[1:]), " ")))
				}
			}
			sort.Strings(fields)
			for _, f := range fields {
				out = append(out, fmt.Sprintf("the rule treats an unset field as a value (any of unset and a value of %s): that is a default; state it on the value leaf (`default` citing upstream evidence that establishes it) or write an undecidable leaf", f))
			}
		}
		for _, x := range c.Of {
			walk(x)
		}
	}
	walk(a.Applicability.Exposure)
	if a.Applicability.Overlap != nil {
		walk(*a.Applicability.Overlap)
	}
	return out
}

func nonEmpty(xs []string) []string {
	var out []string
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// ruleDefaults are the field defaults a rule states (exposure and overlap).
func ruleDefaults(a domain.SemanticAssertion) []domain.FieldDefault {
	if a.Applicability == nil {
		return nil
	}
	ds := a.Applicability.Exposure.Defaults()
	if a.Applicability.Overlap != nil {
		ds = append(ds, a.Applicability.Overlap.Defaults()...)
	}
	return ds
}

// defaults checks every default a rule states (mission D4): each citation is
// evidence the assessment established, upstream text, and establishes the
// default as the evaluators read it (domain.FieldDefault.Unevidenced over the
// citable evidence). It returns the cited evidence and the violations.
func (p *Precomputed) defaults(a domain.SemanticAssertion) ([]domain.Evidence, []string) {
	var cites []domain.Evidence
	var out []string
	for _, d := range ruleDefaults(a) {
		c, why := p.checkDefault(d)
		cites, out = append(cites, c...), append(out, why...)
	}
	return cites, uniqStrings(out)
}

// checkDefault checks one stated default (see defaults).
func (p *Precomputed) checkDefault(d domain.FieldDefault) ([]domain.Evidence, []string) {
	citable := map[domain.EvidenceID]domain.Evidence{}
	for id := range p.allowed {
		citable[id] = p.pool[id]
	}
	var cites []domain.Evidence
	var out []string
	{
		for _, id := range d.Citations {
			e, ok := citable[id]
			switch {
			case !ok:
				out = append(out, fmt.Sprintf("the default %s cites %q, which is not evidence the assessment established: a default cites only upstream ids from `evidence`", d.Value, id))
			case !p.upstream(e):
				out = append(out, fmt.Sprintf("the default %s cites %s, which is not upstream text: a default is established by the upstream schema or code, never by the environment", d.Value, id))
			default:
				cites = append(cites, e)
			}
		}
		if why := d.Unevidenced(citable); why != "" && len(d.Citations) > 0 {
			out = append(out, fmt.Sprintf("the default %s is not established: %s", d.Value, why))
		}
	}
	return cites, out
}

// roleDefaults checks the answer's role-field defaults (formalize/v2): each
// names a field of a consumer role the input offered, once, and is a default
// established by upstream evidence the assessment holds (checkDefault). It
// returns the accepted ones by path.
func (p *Precomputed) roleDefaults(rds []AnswerRoleDefault, bad func(format string, args ...any)) map[string]domain.FieldDefault {
	out := map[string]domain.FieldDefault{}
	paths := map[string]bool{}
	for _, r := range p.ConsumerRoles {
		for k := range r.paths() {
			paths[k] = true
		}
	}
	for i, rd := range rds {
		at := func(format string, args ...any) { bad("roleDefaults[%d]: %s", i, fmt.Sprintf(format, args...)) }
		switch {
		case len(p.ConsumerRoles) == 0:
			at("no consumer role was offered: leave roleDefaults empty")
			continue
		case !paths[rd.Path]:
			at("%q is not a field of a consumer role in `consumerRoles`", rd.Path)
			continue
		case out[rd.Path].Value != "":
			at("repeats the default of %s", rd.Path)
			continue
		}
		if err := rd.Default.Validate(); err != nil {
			at("%v", err)
			continue
		}
		if len(rd.Default.Citations) == 0 {
			at("a default cites the upstream evidence that establishes it")
			continue
		}
		if _, why := p.checkDefault(rd.Default); len(why) > 0 {
			for _, w := range why {
				at("%s", w)
			}
			continue
		}
		out[rd.Path] = rd.Default
	}
	return out
}

// roleNarrowing says why a rule narrows its scope to a consumer role ("" if
// it does not): with consumer roles offered (formalize/v2), a rule on a role's
// subject kind must not name the role's fields — the stage proposes that
// narrowing itself, only with trace evidence (refine.go) — unless the change
// is about the role field itself.
func (p *Precomputed) roleNarrowing(a domain.SemanticAssertion) string {
	if a.Subject == nil || a.Applicability == nil {
		return ""
	}
	for _, r := range p.ConsumerRoles {
		if a.Subject.Group != r.Group || a.Subject.Kind != r.Kind {
			continue
		}
		paths := r.paths()
		delete(paths, a.Subject.Path)
		if names(a.Applicability.Exposure, paths) || (a.Applicability.Overlap != nil && names(*a.Applicability.Overlap, paths)) {
			return fmt.Sprintf("the rule narrows its scope to consumer role %s (it names %s): write the behaviour rule as the assessment establishes it; the stage proposes a role refinement itself, only with consumer-trace evidence", r.ID, fieldsText(r.Role))
		}
	}
	return ""
}

// env is what the spec checks read about the stored run.
func (p *Precomputed) env() agent.MitigationEnv {
	b := p.in.Bundle
	read := p.in.Read
	if read == nil {
		read = func(context.Context, domain.ArtifactRef) ([]byte, error) {
			return nil, fmt.Errorf("formalize: no artifact reader")
		}
	}
	return agent.MitigationEnv{Product: b.Change.Product.ID, From: b.Change.From.Tag, To: b.Change.To.Tag,
		Artifacts: b.Manifest.Artifacts, Read: read,
		Artifact: func(id string) (domain.ArtifactRef, bool) { a, ok := p.arts[id]; return a, ok },
		Basis:    func(_ context.Context, item, ref string) string { return p.basis(ref) }}
}

// basis says why a reference is not a verified fact of this environment
// for the item ("" = it is): one of the listed setting claims or
// validation records.
func (p *Precomputed) basis(ref string) string {
	for _, b := range p.Basis {
		if b.ID == ref {
			return ""
		}
	}
	switch {
	case strings.HasPrefix(ref, domain.SettingClaimIDPrefix):
		return "no verified setting claim of this run about the item has that id"
	case strings.HasPrefix(ref, domain.ValidationRecordIDPrefix):
		return "no validation record of this run for the item has that id"
	}
	return "formalization names only the listed setting claims (clm-…) and validation records (vr-…) as environment basis"
}
