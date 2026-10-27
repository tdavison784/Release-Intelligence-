package formalize

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/applicability"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/agent"
	"github.com/tdavison784/release-intelligence/internal/investigate/verify"
)

// Scope refinements (lane verify2, D-0025) on the synthetic fixture: the
// catalog declares a consumer role of Widgets (spec.signs = true: a signing
// Widget; outside it spec.signs = false) whose consumer reads key ca.crt; the
// run's consumer graph has a Widget producing a Secret whose ca.crt a
// Deployment reads (an established reads-file edge), or not.

var signerRole = ConsumerRole{ID: "signer-trust", Summary: "a client trusting what a signing Widget produces", Group: "example.test", Kind: "Widget",
	Role: []domain.RoleField{{Path: "spec.signs", Value: "true"}}, Outside: []domain.RoleField{{Path: "spec.signs", Value: "false"}}, TrustKey: "ca.crt"}

// consumerGraph is a TO-side consumer graph; key is the key the workload
// reads and status that edge's status.
func consumerGraph(key string, status domain.EdgeStatus) string {
	g := domain.RelationshipGraph{ID: "rg-000000000001", Builder: "relations.builder@v1+consumers@v3",
		Nodes: []domain.GraphNode{
			{ID: "gn-widget", Kind: domain.NodeKind("object"), Key: "example.test/v1/Widget/ns/w1"},
			{ID: "gn-secret", Kind: domain.NodeConfigObject, Key: "core/v1/Secret/ns/w1-tls", Placeholder: true},
			{ID: "gn-app", Kind: domain.NodeWorkload, Key: "apps/v1/Deployment/ns/app"},
		},
		Edges: []domain.GraphEdge{
			{ID: "ge-produces", Kind: domain.EdgeProduces, From: "gn-widget", To: "gn-secret", Status: domain.EdgeEstablished, Rule: "relationship:acme/widget-secret"},
			{ID: "ge-reads", Kind: domain.EdgeReadsFile, From: "gn-secret", To: "gn-app", Status: status, Rule: "graph.reads-file@v1:pod-spec",
				File: &domain.EdgeFile{Path: "/etc/tls/" + key, Workload: "apps/v1/Deployment/ns/app", Container: "app", Via: "pod-spec", Key: key},
				Line: &domain.EdgeLine{Artifact: "art-env", Line: 7, Text: "--ca=/etc/tls/" + key}},
		}}
	b, _ := json.Marshal(g)
	return string(b)
}

// withGraph adds a consumer graph artifact to the run (graph "" = none).
func withGraph(t *testing.T, f fixture, graph string) Input {
	t.Helper()
	in := f.input()
	in.Roles = []ConsumerRole{signerRole}
	if graph != "" {
		artifacts["art-cgraph"] = graph
		t.Cleanup(func() { delete(artifacts, "art-cgraph") })
		f.Inv.Artifacts = append(f.Inv.Artifacts, domain.ArtifactRef{ID: "art-cgraph", Kind: domain.ArtifactGraph, Path: "derived/agent/consumers/to/rg-000000000001.json",
			Origin: domain.OriginDerived, Producer: agent.ConsumerGraphProducer, Digest: "sha256:g"})
	}
	return in
}

func formalizeWith(t *testing.T, in Input, version string, a *Answer) *Record {
	t.Helper()
	c := &scripted{texts: []string{answerJSON(t, a)}}
	r, err := (&Formalizer{Client: c, Prompt: version}).Formalize(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.reqs[0].Messages[0].Content, `"consumerRoles"`) != !refinesPrompt(version) {
		t.Errorf("%s: the consumer roles are shown iff the prompt refines", version)
	}
	return r
}

func refinementsOf(r *Record) (base, refined []domain.Hypothesis) {
	for _, h := range r.Hypotheses {
		switch {
		case h.ProposedVerification != nil && h.ProposedVerification.Refinement != nil:
			refined = append(refined, h)
		case h.Mitigation != nil:
			base = append(base, h)
		}
	}
	return base, refined
}

func TestRefinementProposedOnlyWithTraceEvidence(t *testing.T) {
	f := newFixture()
	in := withGraph(t, f, consumerGraph("ca.crt", domain.EdgeEstablished))
	p := precompute(t, in)
	r := formalizeWith(t, in, PromptVersion, validAnswer(p))
	base, refined := refinementsOf(r)
	if len(base) != 1 || len(refined) != 1 || r.Provenance.Producer != ProducerV2 {
		t.Fatalf("hypotheses %+v notes %v", r.Hypotheses, r.RefinementNotes)
	}
	h, b := refined[0], base[0]
	ref := h.ProposedVerification.Refinement
	if ref.Refines != b.ID || ref.BaseRule != b.Mitigation.ProposedRule.Assertion.Digest() || ref.Consumer != "signer-trust" || ref.TrustKey != "ca.crt" ||
		len(ref.Trace) != 1 || ref.Trace[0].Edge != "ge-reads" || ref.Trace[0].Producer != "example.test/v1/Widget/ns/w1" || ref.Trace[0].Graph != "art-cgraph" ||
		ref.Trace[0].Artifact != "art-env" || ref.Trace[0].Line != 7 {
		t.Fatalf("refinement %+v", ref)
	}
	if h.ProposedVerification.Method != "consumer exercise (role signer-trust)" {
		t.Errorf("method %q", h.ProposedVerification.Method)
	}
	// the refined rule: exposure ∧ role, overlap ∨ outside; nothing else
	rule := h.Mitigation.ProposedRule.Assertion
	baseRule := b.Mitigation.ProposedRule.Assertion
	ap := rule.Applicability
	if ap.Exposure.Op != domain.OpAll || len(ap.Exposure.Of) != 2 || ap.Exposure.Of[1].Of[0].Path != "spec.signs" || ap.Exposure.Of[1].Of[0].Values[0] != "true" ||
		ap.Overlap.Op != domain.OpAny || ap.Overlap.Of[1].Of[0].Values[0] != "false" {
		t.Fatalf("refined applicability %+v", ap)
	}
	if rule.AspectDigest(domain.AspectChange) != baseRule.AspectDigest(domain.AspectChange) || rule.Subject.Key() != baseRule.Subject.Key() ||
		h.Mitigation.ProposedRule.Release != b.Mitigation.ProposedRule.Release {
		t.Error("a refinement changes only the applicability")
	}
	// no evidenced role default: objects that leave spec.signs unset stay
	// undecidable, and the uncertainty says so
	if ap.Exposure.Of[1].Of[0].Default != nil || !strings.Contains(strings.Join(h.ProposedVerification.Uncertainties, "|"), "unset spec.signs") {
		t.Errorf("role default / uncertainty: %+v %v", ap.Exposure.Of[1].Of[0], h.ProposedVerification.Uncertainties)
	}
	unknown := applicability.Unset(ap.Exposure.Of[1].Of[0], applicability.Resolve(applicability.Default{Why: "no schema"}, applicability.RuleDefault(ap.Exposure.Of[1].Of[0], nil)),
		func(string) bool { return false })
	if unknown.Known {
		t.Error("an unset role field without an evidenced default must stay undecidable")
	}
	// verification reads the refinement off the hypothesis
	if m := verify.MitigationOf(h); m.Refinement == nil || m.Refinement.Consumer != "signer-trust" || m.Rule.Digest() != rule.Digest() {
		t.Errorf("verify.MitigationOf = %+v", m)
	}
	if m := verify.MitigationOf(b); m.Refinement != nil {
		t.Error("the behaviour rule is not a refinement")
	}
}

func TestNoRefinementWithoutTraceEvidence(t *testing.T) {
	for name, graph := range map[string]string{
		"no consumer graph":        "",
		"another key read":         consumerGraph("tls.crt", domain.EdgeEstablished),
		"an inferred read":         consumerGraph("ca.crt", domain.EdgeInferred),
		"a read of a missing edge": consumerGraph("ca.crt", domain.EdgeMissing),
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			in := withGraph(t, f, graph)
			r := formalizeWith(t, in, PromptVersion, validAnswer(precompute(t, in)))
			if _, refined := refinementsOf(r); len(refined) != 0 || len(r.RefinementNotes) != 1 || !strings.Contains(r.RefinementNotes[0], "signer-trust") {
				t.Fatalf("refined %+v notes %v", refined, r.RefinementNotes)
			}
		})
	}
	// formalize/v1 is frozen: no roles shown, no refinement, the input as it was
	f := newFixture()
	in := withGraph(t, f, consumerGraph("ca.crt", domain.EdgeEstablished))
	plain := f.input()
	r := formalizeWith(t, in, "formalize/v1", validAnswer(precompute(t, in)))
	if _, refined := refinementsOf(r); len(refined) != 0 || r.RefinementNotes != nil || r.Provenance.Producer != Producer || r.Input != precompute(t, plain).Digest() {
		t.Fatalf("v1: refined %+v notes %v producer %s", refined, r.RefinementNotes, r.Provenance.Producer)
	}
}

func TestRoleDefaults(t *testing.T) {
	answer := func(p *Precomputed, rds ...AnswerRoleDefault) *Answer {
		a := validAnswer(p)
		a.RoleDefaults = rds
		return a
	}
	def := func(path, value string, cites ...domain.EvidenceID) AnswerRoleDefault {
		return AnswerRoleDefault{Path: path, Default: domain.FieldDefault{Value: value, Source: domain.DefaultController, Citations: cites}}
	}
	f := newFixture()
	in := withGraph(t, f, consumerGraph("ca.crt", domain.EdgeEstablished))
	p := precompute(t, in)
	p.ConsumerRoles = in.Roles

	// an evidenced default goes onto the refined rule's role leaves, and its
	// evidence travels with the refinement
	r := formalizeWith(t, in, PromptVersion, answer(p, def("spec.signs", "false", "ev-default")))
	_, refined := refinementsOf(r)
	if len(refined) != 1 {
		t.Fatalf("refined %+v notes %v", r.Hypotheses, r.RefinementNotes)
	}
	ap := refined[0].Mitigation.ProposedRule.Assertion.Applicability
	leaf := ap.Exposure.Of[1].Of[0]
	if leaf.Default == nil || leaf.Default.Value != "false" || ap.Overlap.Of[1].Of[0].Default == nil {
		t.Fatalf("role leaves %+v", ap)
	}
	cited := false
	for _, id := range refined[0].Citations {
		cited = cited || id == "ev-default"
	}
	if !cited || strings.Contains(strings.Join(refined[0].ProposedVerification.Uncertainties, "|"), "unset spec.signs") {
		t.Errorf("citations %v uncertainties %v", refined[0].Citations, refined[0].ProposedVerification.Uncertainties)
	}
	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range f.Bundle.Evidence {
		pool[e.ID] = e
	}
	if u := applicability.Unset(leaf, applicability.Resolve(applicability.Default{Why: "no schema"}, applicability.RuleDefault(leaf, pool)),
		func(v string) bool { return applicability.SameValue(v, "true") }); !u.Known || u.Holds {
		t.Errorf("with the evidenced default an unset spec.signs is outside the role: %+v", u)
	}

	// rejected role defaults
	for name, tc := range map[string]struct {
		rd   AnswerRoleDefault
		want string
	}{
		"environment evidence": {def("spec.signs", "false", "ev-env"), "not upstream text"},
		"foreign evidence":     {def("spec.signs", "false", "ev-foreign"), "not evidence the assessment established"},
		"not a role field":     {def("spec.mode", `"legacy"`, "ev-default"), "not a field of a consumer role"},
		"not JSON":             {def("spec.signs", "nope", "ev-default"), "not JSON-encoded"},
	} {
		v := Floor(context.Background(), p, answer(p, tc.rd))
		if !strings.Contains(strings.Join(v, "|"), tc.want) {
			t.Errorf("%s: violations %v", name, v)
		}
	}
	if v := Floor(context.Background(), p, answer(p, def("spec.signs", "false", "ev-default"), def("spec.signs", "false", "ev-default"))); !strings.Contains(strings.Join(v, "|"), "repeats") {
		t.Errorf("repeated role default: %v", v)
	}
	// with formalize/v1 no role is offered, so none may be stated
	p1 := precompute(t, in)
	if v := Floor(context.Background(), p1, answer(p1, def("spec.signs", "false", "ev-default"))); !strings.Contains(strings.Join(v, "|"), "no consumer role was offered") {
		t.Errorf("v1 role default: %v", v)
	}
}

// The model must not narrow a rule to a consumer role itself: with roles
// offered, a rule naming a role field is rejected (the stage proposes that
// narrowing, only with trace evidence).
func TestModelRoleNarrowingRejected(t *testing.T) {
	f := newFixture()
	in := withGraph(t, f, "")
	p := precompute(t, in)
	p.ConsumerRoles = in.Roles
	a := validAnswer(p)
	rule := a.Hypotheses[0].Rule
	rule.Assertion.Applicability.Exposure.Of = append(rule.Assertion.Applicability.Exposure.Of,
		domain.Condition{Op: domain.OpField, Path: "spec.signs", State: domain.StateEquals, Values: []string{"true"}})
	if v := Floor(context.Background(), p, a); !strings.Contains(strings.Join(v, "|"), "narrows its scope to consumer role signer-trust") {
		t.Errorf("violations %v", v)
	}
	// without roles offered (formalize/v1) the same rule is not judged on it
	p1 := precompute(t, in)
	if v := Floor(context.Background(), p1, a); strings.Contains(strings.Join(v, "|"), "narrows") {
		t.Errorf("v1 violations %v", v)
	}
}

// A refinement whose base is not a rule of the record does not validate.
func TestRefinementMustRefineARuleOfTheRecord(t *testing.T) {
	f := newFixture()
	in := withGraph(t, f, consumerGraph("ca.crt", domain.EdgeEstablished))
	r := formalizeWith(t, in, PromptVersion, validAnswer(precompute(t, in)))
	_, refined := refinementsOf(r)
	bad := *r
	bad.Hypotheses = nil
	for _, h := range r.Hypotheses {
		if h.ID == refined[0].ProposedVerification.Refinement.Refines {
			continue
		}
		bad.Hypotheses = append(bad.Hypotheses, h)
	}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "does not refine") {
		t.Fatalf("validate: %v", err)
	}
	// a refinement without trace evidence is not a valid proposal
	h := refined[0]
	pv := *h.ProposedVerification
	ref := *pv.Refinement
	ref.Trace = nil
	pv.Refinement = &ref
	h.ProposedVerification = &pv
	if err := h.Validate(); err == nil || !strings.Contains(err.Error(), "trace evidence") {
		t.Fatalf("hypothesis validate: %v", err)
	}
}
