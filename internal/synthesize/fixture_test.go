package synthesize

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// A synthetic classified upgrade 1.0.0 → 1.1.0 of product "acme" with three
// items, generic by construction:
//
//   - it1: affected (a failed validation);
//   - it2: potentially-affected; the agent says not-affected; its
//     assessment has an untraced consumer, a not-checked behaviour, an
//     unavailable request and a recommended test; a pin passes for it;
//   - it3: not-affected; the agent says affected.
//
// it1 and it2 change the same rendered object. GHSA-r regresses the target
// (fixed in 1.1.2); GHSA-i contradicts itself. The deployed version is
// stated by an Argo targetRevision line.

var fixedNow = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

func v(s string) domain.Version { return domain.Version{Tag: "v" + s, Semver: s} }

func vp(s string) *domain.Version { x := v(s); return &x }

func ev(id, kind, uri, loc, excerpt string) domain.Evidence {
	return domain.Evidence{ID: domain.EvidenceID(id), Kind: domain.EvidenceKind(kind), URI: uri, Locator: loc, Excerpt: excerpt}
}

type fixture struct {
	Inv      *domain.Investigation
	Bundle   *domain.EvidenceBundle
	Report   *domain.ImpactReport
	Releases *Releases
}

func (f fixture) input() Input {
	return Input{Investigation: f.Inv, Bundle: f.Bundle, Report: f.Report, Releases: f.Releases}
}

const (
	it1 = "chi-000000000001"
	it2 = "chi-000000000002"
	it3 = "chi-000000000003"
)

func newFixture() fixture {
	upstream := []domain.Evidence{
		ev("ev-up1", "document", "https://example.test/notes.md#L3", "L3", "a default changes"),
		ev("ev-up2", "document", "https://example.test/notes.md#L4", "L4", "a flag is removed"),
		ev("ev-up3", "document", "https://example.test/notes.md#L5", "L5", "a field is added"),
	}
	env := []domain.Evidence{
		ev("ev-pin", "local-file", "values/prod.yaml", "$.feature.mode (L7)", "mode: legacy"),
		ev("ev-set", "local-file", "values/prod.yaml#L9", "L9", "replicas: 2"),
	}
	b := &domain.EvidenceBundle{ID: "bnd-000000000001",
		Change: domain.ChangeEvidenceRecord{ID: "chr-000000000001", Product: domain.ProductRef{ID: "acme", Name: "Acme"}, From: v("1.0.0"), To: v("1.1.0"),
			Items: []domain.ChangeItem{
				{ID: it1, ChangeID: "c-1", Title: "a default changes", Evidence: []domain.EvidenceID{"ev-up1"}, Manifestation: "controller-behaviour", InScope: true},
				{ID: it2, ChangeID: "c-2", Title: "a flag is removed", Evidence: []domain.EvidenceID{"ev-up2"}, Manifestation: "values", InScope: true},
				{ID: it3, ChangeID: "c-3", Title: "a field is added", Evidence: []domain.EvidenceID{"ev-up3"}, Manifestation: "schema", InScope: true},
			}},
		Renders: []domain.ManifestPair{{Scope: domain.RenderEnvironment, Target: "argocd:apps/acme.yaml", Status: domain.CollectionOK, Changes: []domain.RenderedChange{
			{ID: "rc-1", Class: "field-changed", Object: "apps/v1/Deployment/ns/acme", Path: "spec.a", ChangeItems: []string{it1}},
			{ID: "rc-2", Class: "field-changed", Object: "apps/v1/Deployment/ns/acme", Path: "spec.b", ChangeItems: []string{it2}},
		}}},
		Manifest: domain.BundleManifest{Artifacts: []domain.ArtifactRef{{ID: "art-values", Path: "env/values/prod.yaml"}}},
		Evidence: append(upstream, env...),
	}
	det := domain.Provenance{Method: domain.MethodComputed, Producer: "classify.policy@v1", Confidence: domain.ConfidenceHigh}
	ai := func(call string) domain.Provenance {
		return domain.Provenance{Method: domain.MethodAI, Producer: "investigate.agent@v1", Confidence: domain.ConfidenceMedium, CallID: call}
	}
	inv := &domain.Investigation{ID: "inv-000000000001", Bundle: b.ID, Environment: "prod",
		Findings: []domain.InvestigationFinding{
			{ID: "inf-1", ChangeItem: it1, Class: domain.FindingAffected, Manifestation: "controller-behaviour", Rule: "classify:validated-affected",
				Reasoning: "a validation failed", Evidence: []domain.EvidenceID{"ev-up1"}, Validations: []string{"vr-fail"}, RenderedChanges: []string{"rc-1"}, Provenance: det},
			{ID: "inf-2", ChangeItem: it2, Class: domain.FindingPotentiallyAffected, Manifestation: "values", Rule: "classify:unvalidated",
				Reasoning: "no decisive validation", Evidence: []domain.EvidenceID{"ev-up2"}, RenderedChanges: []string{"rc-2"},
				NextSteps: []string{"render the environment with the flag unset"}, Provenance: det},
			{ID: "inf-3", ChangeItem: it3, Class: domain.FindingNotAffected, Manifestation: "schema", Rule: "classify:validated-clear",
				Reasoning: "schema compatible", Evidence: []domain.EvidenceID{"ev-up3"}, Provenance: det},
		},
		Decision: &domain.PolicyDecision{Policy: "classify.policy@v1", Outcome: domain.PolicyHumanReview, Reasons: []string{"affected item"}},
		Validations: []domain.ValidationRecord{
			{ID: "vr-fail", ChangeItem: it1, Kind: domain.ValidationRenderDiff, Level: domain.LevelDeterministic, Outcome: domain.ValidationFail,
				Property: "the rendered change does not reach the workload", Detail: "it does", Evidence: []domain.EvidenceID{"ev-up1"}},
			{ID: "vr-pin", ChangeItem: it2, Kind: domain.ValidationOverrideInspection, Level: domain.LevelDeterministic, Outcome: domain.ValidationPass,
				Property: "an override pins the old behaviour", Pin: &domain.PinnedSetting{Kind: domain.PinValuesKey, Subject: "values/prod.yaml",
					Path: "feature.mode", Value: `"legacy"`, Evidence: []domain.EvidenceID{"ev-pin"}}},
		},
		AgentVerdicts: []domain.AgentVerdict{
			{ID: "avd-2", ChangeItem: it2, Class: domain.FindingNotAffected, Provenance: ai("call-1")},
			{ID: "avd-3", ChangeItem: it3, Class: domain.FindingAffected, Provenance: ai("call-1")},
		},
		AgentAssessments: []domain.AgentAssessment{{ID: "aas-2", ChangeItem: it2, Verdict: "avd-2", Headline: "not affected provided the flag stays unset",
			WhatChanged: "a flag is removed", Mechanism: "startup fails with the flag", LikelyImpact: "none",
			EffectiveConfiguration: []domain.AssessmentSetting{{Setting: "flag", State: domain.AssessmentInheritedDefault}},
			Preconditions: []string{"the flag is not set outside the repository"}, Unknown: []string{"runtime arguments"},
			RecommendedTests: []string{"start the workload with the new version"}, UntracedConsumers: []string{"apps/v1/Deployment/ns/web"},
			UnavailableEvidence: []string{"evr-1"},
			Behaviour: []domain.AssessmentBehaviour{{Behaviour: "how the flag is parsed", Source: domain.BehaviourNotChecked, Reason: "no source"}},
			Provenance: ai("call-1")}},
		EvidenceRequests: []domain.EvidenceRequest{{ID: "evr-1", ChangeItem: it2, Status: domain.RequestUnavailable, Resolution: "not found at the tag"}},
		AgentClaims: []domain.SettingClaim{
			{ID: "clm-set", ChangeItem: it2, Kind: domain.ClaimValue, Setting: "replicas", Value: "2", Input: "art-values", Evidence: []domain.EvidenceID{"ev-set"}, Provenance: ai("call-1")},
			{ID: "clm-unset", ChangeItem: it2, Kind: domain.ClaimAbsent, Setting: "feature.flag", Input: "art-values", Provenance: ai("call-1")},
		},
	}
	adv := []domain.Evidence{
		ev("ev-adv-r", "advisory", "https://example.test/advisories/GHSA-r", "", "GHSA-r"),
		ev("ev-adv-i", "advisory", "https://example.test/advisories/GHSA-i", "", "GHSA-i"),
		ev("ev-chart", "registry", "https://example.test/index.yaml", "acme-1.1.2", "acme 1.1.2"),
	}
	rep := &domain.ImpactReport{
		Environment: domain.ImpactEnvironment{Products: []domain.ImpactProduct{{Product: "acme", Catalog: true, Version: "1.0.0", RawVersion: "v1.0.0",
			Source: "detected-argo", Evidence: []domain.EvidenceID{"ev-argo"}}}},
		EnvironmentEvidence: []domain.Evidence{ev("ev-argo", "local-file", "apps/acme.yaml", "$.spec.source.targetRevision (L12)", "targetRevision: v1.0.0")},
		Evidence:            adv,
		Security: &domain.EdgeSecurity{SchemaVersion: domain.EdgeSecuritySchemaVersion, Status: domain.SecurityComplete,
			Regressions: []domain.SecurityAdvisory{{ID: "GHSA-r", Aliases: []string{"CVE-2026-1"}, Vulnerable: ">=1.1.0, <1.1.2", Patched: []string{"1.1.2"},
				SourceID: "global", ChangeID: "c-1", Evidence: []domain.EvidenceID{"ev-adv-r"}}},
			Inconsistencies: []domain.AdvisoryInconsistency{{ID: "GHSA-i", Vulnerable: "<2.0.0", Patched: []string{"1.0.5"}, Contradicting: []string{"1.0.5"},
				SourceID: "global", Statement: "GHSA-i lists 1.0.5 as patched but its range <2.0.0 includes it; check the advisory.", Evidence: []domain.EvidenceID{"ev-adv-i"}}},
			Candidates: []domain.VersionCandidate{
				{Kind: domain.CandidateTargetLine, Version: vp("1.1.2"), Line: "1.1", Statement: "1.1.2 is the lowest known release on line 1.1 that removes every regression.",
					Removes: []domain.AdvisoryRef{{ID: "GHSA-r", Evidence: []domain.EvidenceID{"ev-adv-r"}}}, Evidence: []domain.EvidenceID{"ev-adv-r", "ev-chart"},
					Charts: []domain.CandidateChart{{Artifact: "chart", Name: "acme", Version: "1.1.2", Evidence: []domain.EvidenceID{"ev-chart"}}}},
				{Kind: domain.CandidateAnyLine, Version: vp("1.1.2"), Statement: "1.1.2 on any line.", Evidence: []domain.EvidenceID{"ev-adv-r"}},
				{Kind: domain.CandidateClean, Version: vp("1.1.2"), Statement: "1.1.2 is clean.", Evidence: []domain.EvidenceID{"ev-adv-r"}},
			}},
	}
	rel := &Releases{
		Versions: []domain.Version{v("1.0.0"), v("1.1.0"), v("1.1.1"), v("1.1.2"), v("1.1.3"), v("1.2.0"), {Tag: "v1.1.4-rc.1", Semver: "1.1.4-rc.1"}},
		Advisories: []domain.Advisory{
			{ID: "GHSA-r", Vulnerable: ">=1.1.0, <1.1.2", Evidence: []domain.EvidenceID{"ev-adv-r"}},
			{ID: "GHSA-i", Vulnerable: "<0.9.0", Evidence: []domain.EvidenceID{"ev-adv-i"}},
			{ID: "GHSA-u", Summary: "no range"},
		},
		Evidence: adv,
		Charts:   map[string][]domain.CandidateChart{"1.1.3": {{Artifact: "chart", Name: "acme", Version: "1.1.3", Evidence: []domain.EvidenceID{"ev-chart3"}}}},
		ChartEvidence: []domain.Evidence{ev("ev-chart3", "registry", "https://example.test/index.yaml", "acme-1.1.3", "acme 1.1.3")},
	}
	return fixture{Inv: inv, Bundle: b, Report: rep, Releases: rel}
}

// validAnswer is an answer the floor accepts on the fixture.
func validAnswer(t *testing.T, p *Precomputed) Answer {
	t.Helper()
	var pin, test string
	for _, l := range p.Locations {
		if l.Kind == domain.LocationPinnedSetting {
			pin = l.ID
		}
	}
	for _, c := range p.Tests {
		if c.ChangeItem == it2 {
			test = c.ID
			break
		}
	}
	var hard string
	for _, f := range p.Findings {
		if f.Hard {
			hard = f.ID
			break
		}
	}
	if pin == "" || test == "" || hard == "" {
		t.Fatalf("fixture lacks a pin location, a test or a hard finding: %+v", p)
	}
	rec := p.Target.Recommended
	return Answer{
		Recommendation: domain.RecommendModifyTarget,
		Headline:       "Modify target to " + rec.Semver + "; keep the pinned mode, then upgrade; provided the flag stays unset; start test required before production.",
		Scope:          "the repository's configuration", Preconditions: []string{"the flag is not set outside the repository"},
		TargetVersion: rec.Semver, Rationale: "the proposed target regresses security",
		Changes: []AnswerChange{
			{Kind: domain.StepPinBehaviour, Summary: "keep the pinned mode", ChangeItems: []string{it2}, Location: pin},
			{Kind: domain.StepVersionBump, Summary: "bump to " + rec.Semver},
			{Kind: domain.StepValidate, Summary: "run the start test"},
		},
		Validation:      []AnswerTest{{Test: "start the workload", Expected: "it starts", Basis: []string{test, hard}, ChangeItems: []string{it2}}},
		ResidualRisks:   []AnswerRisk{{Statement: "runtime arguments are not in the repository", ChangeItem: it2, Refs: []string{test}}},
		FindingComments: []AnswerComment{{ID: hard, Comment: "removed by the target change"}},
		Verified:        []AnswerVerified{{Statement: "the advisory range covers the proposed target", Citations: []string{"ev-adv-r"}}},
		Inferred:        []string{"the workload does not set the flag at runtime"},
		Unknown:         []string{"runtime arguments"},
		BackoutNotes:    "revert the pin last",
	}
}

func answerJSON(t *testing.T, a any) string {
	t.Helper()
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
