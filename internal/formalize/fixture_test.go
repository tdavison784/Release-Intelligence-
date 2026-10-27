package formalize

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// A synthetic investigated upgrade 1.0.0 → 1.1.0 of product "acme", generic
// by construction. Item it1: the default of a Widget field spec.mode changes
// from "legacy" to "strict". The agent's assessment (v13 shape) says the
// environment pins the old value explicitly (a verified setting claim), cites
// the upstream note and the CRD, relies on one upstream behaviour and one it
// did not check, leaves one consumer unknown, one setting to its default,
// and states one precondition. ev-foreign is upstream evidence of another
// item the assessment never cites.

var fixedNow = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

const (
	it1  = "ci-000000000001"
	it2  = "ci-000000000002"
	aas1 = "aas-000000000001"
	clm1 = "clm-000000000001"
	vr1  = "vr-000000000001"
)

var artifacts = map[string]string{
	"art-notes": "## 1.1.0\n- The default of spec.mode is now strict.\n",
	"art-crd":   "kind: CustomResourceDefinition\nspec:\n  names:\n    kind: Widget\n  versions:\n  - schema:\n      properties:\n        mode:\n          type: string\n",
	"art-env":   "apiVersion: example.test/v1\nkind: Widget\nspec:\n  mode: legacy\n",
}

type fixture struct {
	Inv    *domain.Investigation
	Bundle *domain.EvidenceBundle
}

func newFixture() fixture {
	ev := func(id, src, uri, excerpt string) domain.Evidence {
		return domain.Evidence{ID: domain.EvidenceID(id), Kind: "document", SourceID: src, URI: uri, Locator: "L2", Excerpt: excerpt}
	}
	b := &domain.EvidenceBundle{ID: "bnd-000000000001",
		Change: domain.ChangeEvidenceRecord{ID: "chr-000000000001", Product: domain.ProductRef{ID: "acme", Name: "Acme"},
			From: domain.Version{Tag: "v1.0.0", Semver: "1.0.0"}, To: domain.Version{Tag: "v1.1.0", Semver: "1.1.0"},
			Items: []domain.ChangeItem{
				{ID: it1, ChangeID: "c-1", Title: "spec.mode defaults to strict", Family: domain.FamilyBehavior, Evidence: []domain.EvidenceID{"ev-note"},
					Manifestation: domain.ManifestControllerBehaviour, InScope: true},
				{ID: it2, ChangeID: "c-2", Title: "another change", Family: domain.FamilyBehavior, Evidence: []domain.EvidenceID{"ev-foreign"},
					Manifestation: domain.ManifestControllerBehaviour, InScope: true},
			}},
		Manifest: domain.BundleManifest{Artifacts: []domain.ArtifactRef{
			{ID: "art-crd", Path: "upstream/crd.yaml", Origin: domain.OriginUpstream, Digest: "sha256:crd"},
			{ID: "art-env", Path: "env/widget.yaml", Origin: domain.OriginEnvironment, Digest: "sha256:env"},
			{ID: "art-notes", Path: "upstream/notes.md", Origin: domain.OriginUpstream, Digest: "sha256:notes"},
		}},
		Evidence: []domain.Evidence{
			ev("ev-note", "art-notes", "upstream/notes.md#L2", "The default of spec.mode is now strict."),
			ev("ev-crd", "art-crd", "upstream/crd.yaml#L8", "mode:"),
			ev("ev-default", "art-crd", "upstream/crd.yaml#L12", "other: default off"),
			ev("ev-env", "art-env", "env/widget.yaml#L4", "mode: legacy"),
			ev("ev-foreign", "art-notes", "upstream/notes.md#L9", "another change"),
		},
	}
	ai := domain.Provenance{Method: domain.MethodAI, Producer: "investigate.agent@v1", Confidence: domain.ConfidenceMedium, CallID: "agent-call-1",
		Model: "m", ModelVersion: "m"}
	inv := &domain.Investigation{ID: "inv-000000000001", Bundle: b.ID, Environment: "prod",
		Agent: &domain.AgentRun{},
		AgentClaims: []domain.SettingClaim{{ID: clm1, ChangeItem: it1, Kind: "value", Setting: "spec.mode", Input: "art-env", Value: `"legacy"`,
			Verifier: "claims@v1", Checked: []string{"art-env"}, Evidence: []domain.EvidenceID{"ev-env"}, Detail: "spec.mode is legacy", Provenance: ai}},
		Validations: []domain.ValidationRecord{{ID: vr1, ChangeItem: it1, Kind: "render", Property: "renders", Outcome: "pass"}},
		AgentAssessments: []domain.AgentAssessment{{ID: aas1, ChangeItem: it1,
			Headline:      "Not affected, provided the workloads reload certificates.",
			Preconditions: []string{"the workloads reload their configuration"},
			WhatChanged:   "The default of spec.mode changes from legacy to strict.",
			Mechanism:     "Objects that leave spec.mode unset switch to strict.",
			Consumers:     []string{"Deployment/ns/app"},
			EffectiveConfiguration: []domain.AssessmentSetting{
				{Setting: "spec.mode", State: domain.AssessmentExplicit, Detail: "legacy in env/widget.yaml"},
				{Setting: "spec.other", State: domain.AssessmentInheritedDefault, Detail: "unset"},
			},
			Verified: []domain.AssessmentFinding{{Statement: "The environment sets spec.mode to legacy.", Citations: []domain.EvidenceID{"ev-env"}}},
			Inferred: []string{"An explicit legacy keeps today's behaviour."},
			Unknown:  []string{"whether Deployment/ns/app depends on strict mode"},
			Behaviour: []domain.AssessmentBehaviour{
				{Behaviour: "the default changes to strict", Source: domain.BehaviourUpstream, Citations: []domain.EvidenceID{"ev-note", "ev-crd", "ev-default"}},
				{Behaviour: "an explicit value is honoured", Source: domain.BehaviourNotChecked},
			},
			LikelyImpact:     "none for this environment",
			RecommendedTests: []string{"render with and without spec.mode"},
			Provenance:       ai}},
	}
	return fixture{Inv: inv, Bundle: b}
}

func readFixture(_ context.Context, a domain.ArtifactRef) ([]byte, error) {
	s, ok := artifacts[a.ID]
	if !ok {
		return nil, fmt.Errorf("no artifact %s", a.ID)
	}
	return []byte(s), nil
}

func (f fixture) input() Input {
	return Input{Investigation: f.Inv, Bundle: f.Bundle, Assessment: aas1, Read: readFixture}
}

func precompute(t *testing.T, in Input) *Precomputed {
	t.Helper()
	p, err := Precompute(in)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func s(v string) *string { return &v }

// validRule is a decidable mitigation rule of the fixture: objects that
// leave spec.mode unset are exposed; an explicit legacy is shielded.
func validRule() *domain.ProposedRule {
	widget := func(leaf domain.Condition) domain.Condition {
		return domain.Condition{Op: domain.OpResource, Group: "example.test", Kind: "Widget", Of: []domain.Condition{leaf}}
	}
	overlap := widget(domain.Condition{Op: domain.OpField, Path: "spec.mode", State: domain.StateEquals, Values: []string{`"legacy"`}})
	return &domain.ProposedRule{Release: domain.ReleaseBounds{From: "1.0.0", To: "1.1.0"}, Assertion: domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectCRDField, Product: "acme", Group: "example.test", Kind: "Widget", Path: "spec.mode"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: s(`"legacy"`), After: s(`"strict"`)},
		Applicability: &domain.Applicability{
			Exposure: widget(domain.Condition{Op: domain.OpField, Path: "spec.mode", State: domain.StateUnset}),
			Overlap:  &overlap},
		Consequence: &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired},
		Statement:   "Widgets that leave spec.mode unset switch to strict; an explicit legacy keeps the old behaviour."}}
}

// validAnswer formalizes the fixture: the rule, and one unrepresentable
// record that keeps every uncertainty (the precondition as an undecidable
// leaf of its partial rule, the rest in keeps).
func validAnswer(p *Precomputed) *Answer {
	var keeps []string
	var pre string
	for _, u := range p.Uncertainties {
		if u.Kind == UncertaintyPrecondition {
			pre = u.ID
			continue
		}
		keeps = append(keeps, u.ID)
	}
	partial := validRule()
	partial.Assertion.Applicability = &domain.Applicability{Exposure: domain.Condition{Op: domain.OpAll, Of: []domain.Condition{
		partial.Assertion.Applicability.Exposure,
		{Op: domain.OpUndecidable, Reason: domain.UnknownRuntimeBehaviorGap, Needed: pre},
	}}}
	return &Answer{Hypotheses: []AnswerHypothesis{
		{Kind: KindRule, Claim: "An explicit spec.mode legacy preserves the pre-upgrade behaviour.", Citations: []string{"ev-note", "ev-env"},
			Conditions: []string{"spec.mode is set to legacy"}, Rule: validRule(), EnvironmentBasis: []string{clm1},
			RequestedVerification: "Exercise a Widget with spec.mode legacy and one with it unset on 1.1.0."},
		{Kind: KindUnrepresentable, Claim: "The consumer keeps working only if it reloads its configuration.", Citations: []string{"ev-env"},
			Conditions: []string{"the consumer reloads"}, Rule: partial,
			Limitation:           "a consumer's reload behaviour is a runtime fact the condition grammar cannot decide",
			ProposedVerification: "Observe the consumer across a configuration change in a test cluster.", Keeps: keeps},
	}}
}

func answerJSON(t *testing.T, a *Answer) string {
	t.Helper()
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// scripted answers each call with the next text, as a model would report it.
type scripted struct {
	texts []string
	reqs  []llm.Request
}

func (c *scripted) Complete(_ context.Context, req llm.Request) (*llm.Response, error) {
	c.reqs = append(c.reqs, req)
	if len(c.reqs) > len(c.texts) {
		return nil, llm.ErrFakeExhausted
	}
	return &llm.Response{Text: c.texts[len(c.reqs)-1], Model: DefaultModel, ModelVersion: DefaultModel, Origin: llm.OriginFake,
		CallID: fmt.Sprintf("call-%d", len(c.reqs)), GeneratedAt: fixedNow}, nil
}
