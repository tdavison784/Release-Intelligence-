package formalize

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/agent"
)

// Scope refinements (Milestone 4 fix, lane verify2, D-0025; prompt
// formalize/v2). A behaviour rule says which subjects change; whether the
// change harms anything depends on how consumers use what a subject
// produces, which no upstream fetch of the product establishes. When the
// product's exercises catalog declares a consumer role R for the rule's
// subject kind (a role-field condition, the fields a subject holds outside
// the role, and the key a consumer in the role reads), and the
// investigation's consumer graph shows workloads reading that key from a
// Secret an object of the subject kind produces (established reads-file
// edges), the stage proposes a refinement candidate beside the rule: the
// same rule with its exposure narrowed to subjects in R and subjects outside
// R shielded. It is deterministic — the model neither proposes nor writes
// it; roles, conditions and keys come from the catalog only — and never a
// confirmed rule by itself: its ProposedVerification names the role's
// consumer exercise, which alone can establish it (internal/investigate/
// verify), and a human promotes it through the unchanged gate. A role field
// is compared by value, so what an unset role field means is a default: the
// refinement states it only when the model's answer gave it with upstream
// evidence the floor accepted (roleDefaults); otherwise objects that leave
// the field unset stay undecidable.

// ConsumerRole is a consumer role the product's exercises catalog declares
// for a subject kind (products/exercises/fixtures/<product>.yaml
// `consumers:` with its role-absent observation `outside:`).
type ConsumerRole struct {
	ID      string             `json:"id"`
	Summary string             `json:"summary,omitempty"`
	Group   string             `json:"group,omitempty"`
	Kind    string             `json:"kind"`
	Role    []domain.RoleField `json:"role"`
	Outside []domain.RoleField `json:"outside"`
	// TrustKey is the key of a subject's output that a consumer in the role
	// reads (the stage matches trace edges on it; the model does not see it).
	TrustKey string `json:"-"`
}

// paths are the role's field paths (in the role and outside it).
func (r ConsumerRole) paths() map[string]bool {
	out := map[string]bool{}
	for _, f := range append(append([]domain.RoleField(nil), r.Role...), r.Outside...) {
		out[f.Path] = true
	}
	return out
}

// RefinementCallID is the call id of a refinement candidate derived from a
// formalization call for a consumer role.
func RefinementCallID(call, role string) string { return call + "+refine:" + role }

// refinesPrompt reports whether a prompt version proposes refinements (and
// shows the model the consumer roles): every version after formalize/v1,
// which is frozen as it was run.
func refinesPrompt(version string) bool { return version != "formalize/v1" }

// ProducerFor is the record producer of a prompt version.
func ProducerFor(promptVersion string) string {
	if refinesPrompt(promptVersion) {
		return ProducerV2
	}
	return Producer
}

// consumerGraph reads the run's TO-side consumer graph (the trace's graph;
// nil when the run has none).
func (p *Precomputed) consumerGraph(ctx context.Context) (*domain.RelationshipGraph, domain.ArtifactRef, error) {
	for _, a := range p.in.Investigation.Artifacts {
		if a.Producer != agent.ConsumerGraphProducer || !strings.HasPrefix(a.Path, "derived/agent/consumers/to/") {
			continue
		}
		if p.in.Read == nil {
			return nil, a, fmt.Errorf("no artifact reader for the consumer graph %s", a.ID)
		}
		b, err := p.in.Read(ctx, a)
		if err != nil {
			return nil, a, fmt.Errorf("consumer graph %s: %w", a.ID, err)
		}
		var g domain.RelationshipGraph
		if err := json.Unmarshal(b, &g); err != nil {
			return nil, a, fmt.Errorf("consumer graph %s: %w", a.ID, err)
		}
		return &g, a, nil
	}
	return nil, domain.ArtifactRef{}, nil
}

// objectKind reads group and kind from a graph node key
// (<group>/<version>/<kind>/<namespace>/<name>; group "core" is "").
func objectKind(key string) (group, kind string, ok bool) {
	parts := strings.Split(key, "/")
	if len(parts) != 5 {
		return "", "", false
	}
	if parts[0] == "core" {
		parts[0] = ""
	}
	return parts[0], parts[2], true
}

// TraceReads are the established reads-file edges of a consumer graph that
// read the role's trust key from a Secret an object of the role's subject
// kind produces (an established produces edge), sorted by edge id.
func TraceReads(g *domain.RelationshipGraph, graph string, r ConsumerRole) []domain.TraceRead {
	if g == nil || r.TrustKey == "" {
		return nil
	}
	nodes := map[string]domain.GraphNode{}
	for _, n := range g.Nodes {
		nodes[n.ID] = n
	}
	producer := map[string]string{} // Secret node id -> producing object key
	for _, e := range g.Edges {
		if e.Kind != domain.EdgeProduces || e.Status != domain.EdgeEstablished {
			continue
		}
		fg, fk, ok1 := objectKind(nodes[e.From].Key)
		tg, tk, ok2 := objectKind(nodes[e.To].Key)
		if ok1 && ok2 && fg == r.Group && fk == r.Kind && tg == "" && tk == "Secret" {
			producer[e.To] = nodes[e.From].Key
		}
	}
	var out []domain.TraceRead
	for _, e := range g.Edges {
		if e.Kind != domain.EdgeReadsFile || e.Status != domain.EdgeEstablished || e.File == nil || e.File.Key != r.TrustKey || producer[e.From] == "" {
			continue
		}
		t := domain.TraceRead{Graph: graph, Edge: e.ID, Producer: producer[e.From], Secret: nodes[e.From].Key, Key: e.File.Key,
			Workload: e.File.Workload, Container: e.File.Container, Path: e.File.Path}
		if e.Line != nil {
			t.Artifact, t.Line = e.Line.Artifact, e.Line.Line
		} else if len(e.Artifacts) > 0 {
			t.Artifact = e.Artifacts[0]
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Edge < out[j].Edge })
	return out
}

// names reports whether a condition names one of the paths.
func names(c domain.Condition, paths map[string]bool) bool {
	if c.Op == domain.OpField && paths[c.Path] {
		return true
	}
	for _, x := range c.Of {
		if names(x, paths) {
			return true
		}
	}
	return false
}

// RefineRule narrows a behaviour rule to the subjects in a consumer role.
// Both conditions must be one resource condition of the subject kind (a
// resource condition holds when some object of the kind meets its operands,
// so the role fields must join the same scope to speak of the same object):
// exposure resource(K, E…, role fields), overlap resource(K, any(all(O…),
// all(fields outside the role))). defaults are evidenced role-field defaults
// (by path). why is set when the rule cannot be refined.
func RefineRule(rule domain.SemanticAssertion, r ConsumerRole, defaults map[string]domain.FieldDefault) (domain.SemanticAssertion, string) {
	ap := rule.Applicability
	switch {
	case rule.Subject == nil || ap == nil:
		return rule, "the rule has no subject or applicability"
	case rule.Subject.Group != r.Group || rule.Subject.Kind != r.Kind:
		return rule, "the role is of another subject kind"
	case len(r.Role) == 0 || len(r.Outside) == 0:
		return rule, "the catalog declares no fields outside the role"
	case names(ap.Exposure, r.paths()) || (ap.Overlap != nil && names(*ap.Overlap, r.paths())):
		return rule, "the rule already names a field of the role"
	}
	scoped := func(c domain.Condition) bool {
		return c.Op == domain.OpResource && c.Group == r.Group && c.Kind == r.Kind && c.Name == "" && len(c.Of) > 0
	}
	e := ap.Exposure
	if !scoped(e) {
		return rule, "its exposure is not one condition on an object of the subject kind, so the role cannot be added to the same object"
	}
	var o domain.Condition
	switch {
	case ap.Overlap != nil && scoped(*ap.Overlap):
		o = *ap.Overlap
	case ap.Overlap != nil:
		return rule, "its mitigating (overlap) condition is not one condition on an object of the subject kind"
	case len(e.Of) == 1 && e.Of[0].Op == domain.OpField && e.Of[0].State == domain.StateUnset:
		// "exposed while unset": the shielded state is the field set (the
		// rule exercise reads such a rule the same way)
		o = domain.Condition{Op: domain.OpResource, Group: e.Group, Kind: e.Kind, Version: e.Version,
			Of: []domain.Condition{{Op: domain.OpField, Path: e.Of[0].Path, State: domain.StateSet}}}
	default:
		return rule, "the rule states no mitigating (overlap) condition and its exposure is not a single unset field"
	}
	leaves := func(fs []domain.RoleField) []domain.Condition {
		var out []domain.Condition
		for _, f := range fs {
			c := domain.Condition{Op: domain.OpField, Path: f.Path, State: domain.StateEquals, Values: []string{f.Value}}
			if d, ok := defaults[f.Path]; ok {
				d.Citations = append([]domain.EvidenceID(nil), d.Citations...)
				c.Default = &d
			}
			out = append(out, c)
		}
		return out
	}
	exposure := e
	exposure.Of = append(append([]domain.Condition(nil), e.Of...), leaves(r.Role)...)
	overlap := o
	overlap.Of = []domain.Condition{{Op: domain.OpAny, Of: []domain.Condition{
		{Op: domain.OpAll, Of: append([]domain.Condition(nil), o.Of...)},
		{Op: domain.OpAll, Of: leaves(r.Outside)}}}}
	out := rule
	out.Applicability = &domain.Applicability{Exposure: exposure, Overlap: &overlap}
	out.Statement = strings.TrimSpace(rule.Statement + domain.ScopeRefinement{Consumer: r.ID, Role: r.Role, Outside: r.Outside}.Clause())
	return out, ""
}

func fieldsText(fs []domain.RoleField) string { return domain.RoleFieldsText(fs) }

// refinements proposes the scope-refinement candidates of the accepted
// rule hypotheses (see the comment above). notes say, per rule and role,
// why no candidate was proposed.
func refinements(ctx context.Context, p *Precomputed, base []domain.Hypothesis, cs map[string]checked, roleDefaults map[string]domain.FieldDefault) (out []domain.Hypothesis, notes []string) {
	if len(p.ConsumerRoles) == 0 {
		return nil, nil
	}
	g, art, gerr := p.consumerGraph(ctx)
	for _, h := range base {
		c, ok := cs[h.ID]
		if !ok || h.Mitigation == nil {
			continue
		}
		rule := h.Mitigation.ProposedRule.Assertion
		for _, r := range p.ConsumerRoles {
			if rule.Subject == nil || rule.Subject.Group != r.Group || rule.Subject.Kind != r.Kind {
				continue
			}
			note := func(format string, args ...any) {
				notes = append(notes, fmt.Sprintf("no refinement of %s to consumer role %s: %s", h.ID, r.ID, fmt.Sprintf(format, args...)))
			}
			switch {
			case gerr != nil:
				note("%v", gerr)
				continue
			case g == nil:
				note("the investigation has no consumer graph (no consumer trace)")
				continue
			}
			reads := TraceReads(g, art.ID, r)
			if len(reads) == 0 {
				note("the consumer trace shows no workload reading %s from a Secret a %s produces", r.TrustKey, r.Kind)
				continue
			}
			refined, why := RefineRule(rule, r, roleDefaults)
			if why != "" {
				note("%s", why)
				continue
			}
			cites := append(append([]domain.Evidence(nil), c.cites...), c.defaultCites...)
			var undecided []string
			for _, f := range r.Role {
				if d, ok := roleDefaults[f.Path]; ok {
					// a stated default's evidence travels with the hypothesis
					for _, id := range d.Citations {
						cites = append(cites, p.pool[id])
					}
				} else {
					undecided = append(undecided, fmt.Sprintf("what an unset %s means is not established by the investigation's upstream evidence: objects that leave it unset stay undecidable under this refinement", f.Path))
				}
			}
			verification := fmt.Sprintf("Run the consumer exercise of role %s: in the role the consumer must verify with the mitigating condition and fail at TO without it; outside the role (%s) it must keep verifying at FROM and TO. %s",
				r.ID, fieldsText(r.Outside), h.Mitigation.RequestedVerification)
			spec, why := agent.CheckMitigation(ctx, p.env(), p.item.ID, investigate.MitigationInput{ProposedRule: domain.ProposedRule{Assertion: refined,
				Release: h.Mitigation.ProposedRule.Release}, EnvironmentBasis: h.Mitigation.EnvironmentBasis, RequestedVerification: verification}, cites)
			if why != "" {
				note("the refined rule fails the spec checks: %s", why)
				continue
			}
			claim := fmt.Sprintf("Scope refinement of %s: its harmful consequence occurs only for %s objects in consumer role %s (%s), whose output workloads read (%s); objects outside the role (%s) are shielded.",
				h.ID, r.Kind, r.ID, fieldsText(r.Role), r.TrustKey, fieldsText(r.Outside))
			var ids []domain.EvidenceID
			for _, e := range cites {
				ids = append(ids, e.ID)
			}
			ids = uniqEvidence(append(ids, h.Citations...))
			// derived from the base rule's call by this stage: its own call
			// id, so it is exported as its own proposal (one per call and
			// candidate)
			prov := h.Provenance
			prov.CallID = RefinementCallID(h.Provenance.CallID, r.ID)
			prov.InputEvidence = uniqEvidence(append(append([]domain.EvidenceID(nil), ids...), p.item.Evidence...))
			x := domain.Hypothesis{ID: domain.AgentRecordID(domain.HypothesisIDPrefix, p.item.ID, claim, prov.CallID), ChangeItem: p.item.ID,
				Statement: claim, Citations: ids, StatementKind: domain.StatementUnverifiedProse, Mitigation: spec, Provenance: prov,
				ProposedVerification: &domain.ProposedVerification{Assessment: p.Assessment.ID, Method: domain.RefinementMethodPrefix + r.ID + ")",
					Conditions: append(append([]string(nil), h.ProposedVerification.Conditions...),
						fmt.Sprintf("the subject plays consumer role %s (%s)", r.ID, fieldsText(r.Role))),
					Uncertainties: append(append([]string(nil), h.ProposedVerification.Uncertainties...), undecided...),
					Refinement: &domain.ScopeRefinement{Refines: h.ID, BaseRule: rule.Digest(), Consumer: r.ID,
						Role: r.Role, Outside: r.Outside, TrustKey: r.TrustKey, Trace: reads}}}
			out = append(out, x)
		}
	}
	return out, notes
}
