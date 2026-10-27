package classify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// --- R1 ----------------------------------------------------------------------------

func TestR1FailingValidationIsAffected(t *testing.T) {
	c := newClean(t)
	fail := c.f.vrec(c.item, domain.ValidationRenderDiff, domain.ValidationFail, rcs(c.rc), detail("the new default reaches the controller args"))
	cls, _ := run(t, c.f.in(c.tr, nil, c.pass, fail))
	f := finding(t, cls, c.item)
	wantClass(t, f, domain.FindingAffected, RuleValidatedExposure)
	if f.Basis != domain.LevelDeterministic || len(f.Validations) != 1 || f.Validations[0] != fail.ID {
		t.Fatalf("affected cites exactly the failing validation with its level: %+v basis %s", f.Validations, f.Basis)
	}
	if f.Approval.Eligible || !f.Approval.RequiresHumanReview || f.Remediation == "" {
		t.Fatalf("affected: approval %+v remediation %q", f.Approval, f.Remediation)
	}
	if !strings.Contains(f.Reasoning, "VALIDATED") || !strings.Contains(f.Reasoning, fail.ID) {
		t.Fatalf("reasoning does not name the validated failure: %s", f.Reasoning)
	}
}

func TestR1ObservedFailBasis(t *testing.T) {
	c := newClean(t, behaviour)
	fail := c.f.vrec(c.item, domain.ValidationRuntime, domain.ValidationFail, subjects(node(controllerObj)))
	cls, _ := run(t, c.f.in(c.tr, nil, fail))
	f := finding(t, cls, c.item)
	wantClass(t, f, domain.FindingAffected, RuleValidatedExposure)
	if f.Basis != domain.LevelObserved {
		t.Fatalf("basis %s, want observed", f.Basis)
	}
}

// R1 precedes R2: a failure is never outweighed by passes.
func TestR1BeatsPasses(t *testing.T) {
	c := newClean(t)
	obs := c.f.vrec(c.item, domain.ValidationRuntime, domain.ValidationPass, paths(c.path), rcs(c.rc))
	fail := c.f.vrec(c.item, domain.ValidationStaticReference, domain.ValidationFail, paths(c.path))
	cls, _ := run(t, c.f.in(c.tr, nil, c.pass, obs, fail))
	f := finding(t, cls, c.item)
	wantClass(t, f, domain.FindingAffected, RuleValidatedExposure)
	if f.Basis != domain.LevelDeterministic {
		t.Fatalf("basis must be the failing validation's level, got %s", f.Basis)
	}
}

// --- R2 ----------------------------------------------------------------------------

func TestR2ValidatedClear(t *testing.T) {
	c := newClean(t)
	cls, inv := run(t, c.f.in(c.tr, nil, c.pass))
	f := finding(t, cls, c.item)
	wantClass(t, f, domain.FindingNotAffected, RuleValidatedClear)
	if !f.Approval.Eligible || f.Basis != domain.LevelDeterministic || len(f.Gaps) > 0 || len(f.Assumptions) > 0 {
		t.Fatalf("not-affected finding: %+v", f)
	}
	if len(f.RenderedChanges) != 1 || f.RenderedChanges[0] != c.rc || len(f.Paths) != 1 || len(f.Components) != 2 {
		t.Fatalf("exposure not listed: rcs %v paths %v components %v", f.RenderedChanges, f.Paths, f.Components)
	}
	if cls.Decision.Outcome != domain.PolicyAutoApproveEligible || cls.Decision.AutoApprovalEnabled {
		t.Fatalf("decision %+v", cls.Decision)
	}
	if inv.Review.State != domain.ReviewAwaiting {
		t.Fatalf("an eligible run still waits for a human in milestone 1, got %s", inv.Review.State)
	}
}

// R2.1: per-family collection. Each case breaks exactly one deciding input.
func TestR2_1CollectionPerFamily(t *testing.T) {
	cases := []struct {
		name  string
		fam   domain.ChangeItemFamily
		setup func(f *fx)
		gap   domain.GapKind
	}{
		{"values: chart source partial", domain.FamilyValues, func(f *fx) { f.source(domain.ChangeSourceChart, domain.CollectionPartial, "values.yaml missing") }, domain.GapCollection},
		{"values: repo input partial", domain.FamilyValues, func(f *fx) {
			f.inputs[0].Status, f.inputs[0].Reason = domain.CollectionPartial, "valuesFrom outside the repo"
		}, domain.GapCollection},
		{"values: no deciding input", domain.FamilyValues, func(f *fx) {
			f.inputs = []domain.SnapshotInput{{Kind: domain.InputClusterVersion, Value: "1.31", Status: domain.CollectionOK}}
		}, domain.GapExposure},
		{"values: an incomplete render target", domain.FamilyValues, func(f *fx) { f.brokenPair("kustomize:overlays/prod", "values-incomplete: layer in another repo") }, domain.GapCollection},
		{"image: images input unavailable", domain.FamilyImage, func(f *fx) { f.input(domain.InputImages, "images.txt", "", domain.CollectionUnavailable, "unreadable") }, domain.GapCollection},
		{"rendered: release renders unavailable", domain.FamilyRendered, func(f *fx) {
			f.source(domain.ChangeSourceRenderedManifests, domain.CollectionUnavailable, "chart-unavailable")
		}, domain.GapCollection},
		{"api-schema: CRD source unavailable", domain.FamilyAPISchema, func(f *fx) { f.source(domain.ChangeSourceAPISchema, domain.CollectionUnavailable, "no CRD bundle") }, domain.GapCollection},
		{"api-schema: crds input partial", domain.FamilyAPISchema, func(f *fx) { f.input(domain.InputCRDs, "crds/", "", domain.CollectionPartial, "one file unparsable") }, domain.GapCollection},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newClean(t, family(tc.fam))
			tc.setup(c.f)
			cls, _ := run(t, c.f.in(c.tr, nil, c.pass))
			f := finding(t, cls, c.item)
			if f.Class == domain.FindingNotAffected {
				t.Fatalf("%s: cleared despite a collection gap", tc.name)
			}
			if !hasGap(f, tc.gap, "") || len(f.NextSteps) == 0 {
				t.Fatalf("%s: want a %s gap and next steps, got %+v / %v", tc.name, tc.gap, f.Gaps, f.NextSteps)
			}
		})
	}
}

// R2.1 the other way: not-applicable sources and pairs, and gaps of sources
// that do not decide the family, never block.
func TestR2_1NotApplicableAndUnrelatedGapsDoNotBlock(t *testing.T) {
	c := newClean(t)
	c.f.naPair("kustomize:apps/consumer")
	c.f.source(domain.ChangeSourceSourceDiff, domain.CollectionUnavailable, "no source-diff collector")
	c.f.source(domain.ChangeSourceAPISchema, domain.CollectionUnavailable, "no CRD bundle")
	c.f.input(domain.InputLiveCluster, "", "kind-test", domain.CollectionUnavailable, "live observation disabled")
	cls, _ := run(t, c.f.in(c.tr, nil, c.pass))
	wantClass(t, finding(t, cls, c.item), domain.FindingNotAffected, RuleValidatedClear)
	if cls.Decision.Outcome != domain.PolicyAutoApproveEligible {
		t.Fatalf("a not-applicable pair must not block eligibility: %+v", cls.Decision)
	}
}

func TestR2_1CompatibilityNeedsTheClusterVersion(t *testing.T) {
	f := newFx(t)
	item := f.item(platform("kubernetes"))
	f.pair("argocd:apps/gizmo.yaml")
	tr := (&trav{}).result()
	pass := f.vrec(item, domain.ValidationSchemaCompat, domain.ValidationPass)
	cls, _ := run(t, f.in(tr, nil, pass))
	fd := finding(t, cls, item)
	wantClass(t, fd, domain.FindingInsufficientEvidence, RuleInsufficientEvidence)
	if !hasGap(fd, domain.GapExposure, "cluster-version") {
		t.Fatalf("want an exposure gap naming cluster-version: %+v", fd.Gaps)
	}

	f2 := newFx(t)
	item2 := f2.item(platform("kubernetes"))
	f2.input(domain.InputClusterVersion, "", "1.31", domain.CollectionOK, "")
	pass2 := f2.vrec(item2, domain.ValidationSchemaCompat, domain.ValidationPass)
	cls2, _ := run(t, f2.in((&trav{}).result(), nil, pass2))
	wantClass(t, finding(t, cls2, item2), domain.FindingNotAffected, RuleValidatedClear)
}

func TestR2_1RenderDecidedFamilyWithoutRender(t *testing.T) {
	f := newFx(t)
	item := f.item()
	pass := f.vrec(item, domain.ValidationOverrideInspection, domain.ValidationPass)
	cls, _ := run(t, f.in((&trav{}).result(), nil, pass))
	fd := finding(t, cls, item)
	if fd.Class == domain.FindingNotAffected || !hasGap(fd, domain.GapCollection, "render") {
		t.Fatalf("a values item cannot clear without an environment render: %s %+v", fd.Class, fd.Gaps)
	}
	if cls.Decision.Outcome != domain.PolicyHumanReview || !anyContains(cls.Decision.Reasons, "no complete environment render") {
		t.Fatalf("decision %+v", cls.Decision)
	}
}

// R2.2: every correlated environment rendered change must be shown harmless.
func TestR2_2UncoveredRenderedChange(t *testing.T) {
	c := newClean(t)
	partial := c.f.vrec(c.item, domain.ValidationRenderDiff, domain.ValidationPass, paths(c.path))
	cls, _ := run(t, c.f.in(c.tr, nil, partial))
	f := finding(t, cls, c.item)
	wantClass(t, f, domain.FindingPotentiallyAffected, RulePlausibleImpact)
	if !hasGap(f, domain.GapValidation, c.rc) {
		t.Fatalf("want a validation gap for %s: %+v", c.rc, f.Gaps)
	}
}

// R2.3: a truncated traversal or an unresolved edge on a path never clears.
func TestR2_3IncompleteTraversal(t *testing.T) {
	t.Run("truncated", func(t *testing.T) {
		c := newClean(t)
		tb := &trav{trunc: "max paths reached"}
		tb.root(c.item, node(controllerObj))
		p := tb.path(c.item, domain.EdgeEstablished, node(controllerObj), node("apps/v1/Deployment/app/consumer"))
		pass := c.f.vrec(c.item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(c.rc), paths(p))
		cls, _ := run(t, c.f.in(tb.result(), nil, pass))
		f := finding(t, cls, c.item)
		if f.Class == domain.FindingNotAffected || !hasGap(f, domain.GapTraversal, "") {
			t.Fatalf("truncated traversal cleared: %s %+v", f.Class, f.Gaps)
		}
		if !anyContains(cls.Decision.Reasons, "traversal is not complete") {
			t.Fatalf("decision reasons %v", cls.Decision.Reasons)
		}
	})
	for _, st := range []domain.EdgeStatus{domain.EdgeInferred, domain.EdgeMissing} {
		t.Run(string(st), func(t *testing.T) {
			c := newClean(t)
			tb := &trav{}
			tb.root(c.item, node(controllerObj))
			p := tb.path(c.item, st, node(controllerObj), node("v1/Secret/app/tls"))
			pass := c.f.vrec(c.item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(c.rc), paths(p))
			cls, _ := run(t, c.f.in(tb.result(), nil, pass))
			f := finding(t, cls, c.item)
			wantClass(t, f, domain.FindingPotentiallyAffected, RulePlausibleImpact)
			if !hasGap(f, domain.GapRelationship, "") {
				t.Fatalf("want a relationship gap: %+v", f.Gaps)
			}
		})
	}
	t.Run("no traversal", func(t *testing.T) {
		c := newClean(t)
		pass := c.f.vrec(c.item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(c.rc))
		cls, _ := run(t, c.f.in(nil, nil, pass))
		if f := finding(t, cls, c.item); f.Class == domain.FindingNotAffected || !hasGap(f, domain.GapTraversal, "traversal") {
			t.Fatalf("no traversal cleared: %s %+v", f.Class, f.Gaps)
		}
	})
}

// R2.4: paths, else roots, else the item itself need a passing validation.
func TestR2_4Coverage(t *testing.T) {
	t.Run("uncovered path", func(t *testing.T) {
		c := newClean(t)
		v := c.f.vrec(c.item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(c.rc))
		cls, _ := run(t, c.f.in(c.tr, nil, v))
		if f := finding(t, cls, c.item); f.Class == domain.FindingNotAffected || !hasGap(f, domain.GapValidation, c.path) {
			t.Fatalf("uncovered path cleared: %s %+v", f.Class, f.Gaps)
		}
	})
	t.Run("no roots: no match is never not-affected by itself", func(t *testing.T) {
		f := newFx(t)
		item := f.item()
		f.pair("argocd:apps/gizmo.yaml")
		cls, _ := run(t, f.in((&trav{}).result(), nil))
		fd := finding(t, cls, item)
		wantClass(t, fd, domain.FindingInsufficientEvidence, RuleInsufficientEvidence)
		if !hasGap(fd, domain.GapValidation, item) || len(fd.NextSteps) == 0 {
			t.Fatalf("want the 'no passing validation' gap and next steps: %+v %v", fd.Gaps, fd.NextSteps)
		}
	})
	t.Run("no roots: one passing validation clears a rendered-config item", func(t *testing.T) {
		f := newFx(t)
		item := f.item()
		f.pair("argocd:apps/gizmo.yaml")
		pass := f.vrec(item, domain.ValidationRenderDiff, domain.ValidationPass)
		cls, _ := run(t, f.in((&trav{}).result(), nil, pass))
		wantClass(t, finding(t, cls, item), domain.FindingNotAffected, RuleValidatedClear)
	})
	t.Run("roots without paths are covered through subjects", func(t *testing.T) {
		f := newFx(t)
		item := f.item()
		f.pair("argocd:apps/gizmo.yaml")
		tb := (&trav{}).root(item, node(controllerObj))
		tr := tb.result()
		cls, _ := run(t, f.in(tr, nil, f.vrec(item, domain.ValidationRenderDiff, domain.ValidationPass)))
		if fd := finding(t, cls, item); fd.Class == domain.FindingNotAffected {
			t.Fatal("an uncovered root cleared")
		}
		cls, _ = run(t, f.in(tr, nil, f.vrec(item, domain.ValidationRenderDiff, domain.ValidationPass, subjects(node(controllerObj)))))
		wantClass(t, finding(t, cls, item), domain.FindingNotAffected, RuleValidatedClear)
	})
}

// R2.4 manifestation: render silence never clears a controller-behaviour or
// unknown item; only a pinned setting or an observed validation does.
func TestR2_4BehaviourItems(t *testing.T) {
	for _, m := range []func(*domain.ChangeItem){behaviour, unknownManifest} {
		c := newClean(t, m)
		silent := c.f.vrec(c.item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(c.rc), paths(c.path))
		joinPass := c.f.vrec(c.item, domain.ValidationOverrideInspection, domain.ValidationPass, validator("validate.join@v1"))
		cls, _ := run(t, c.f.in(c.tr, nil, silent, joinPass))
		f := finding(t, cls, c.item)
		if f.Class == domain.FindingNotAffected {
			t.Fatalf("%s item cleared by render silence", f.Manifestation)
		}
		if !hasGap(f, domain.GapValidation, c.item) || !strings.Contains(f.Reasoning, "render silence proves nothing") {
			t.Fatalf("the finding must say render silence is not decisive: %s %+v", f.Reasoning, f.Gaps)
		}

		// a field pin on exactly the changed field covers it (exact coverage:
		// a flag pin would cover no rendered change)
		pin := c.f.vrec(c.item, domain.ValidationOverrideInspection, domain.ValidationPass, rcs(c.rc), paths(c.path),
			withFieldPinAt(c.f.envEvidence(), controllerObj, "spec.template.spec.containers[].args"))
		cls, _ = run(t, c.f.in(c.tr, nil, silent, pin))
		f = finding(t, cls, c.item)
		wantClass(t, f, domain.FindingNotAffected, RuleValidatedClear)
		if len(f.Validations) != 1 || f.Validations[0] != pin.ID {
			t.Fatalf("a behaviour item's clear cites only the clearing validation: %v", f.Validations)
		}

		obs := c.f.vrec(c.item, domain.ValidationRuntime, domain.ValidationPass, rcs(c.rc), paths(c.path))
		cls, _ = run(t, c.f.in(c.tr, nil, silent, obs))
		f = finding(t, cls, c.item)
		wantClass(t, f, domain.FindingNotAffected, RuleValidatedClear)
		if f.Basis != domain.LevelObserved {
			t.Fatalf("basis %s", f.Basis)
		}
	}
}

// R2.5: an erroring validation contradicts a clear.
func TestR2_5ErrorBlocks(t *testing.T) {
	c := newClean(t)
	errRec := c.f.vrec(c.item, domain.ValidationStaticReference, domain.ValidationError, paths(c.path))
	cls, _ := run(t, c.f.in(c.tr, nil, c.pass, errRec))
	f := finding(t, cls, c.item)
	wantClass(t, f, domain.FindingPotentiallyAffected, RulePlausibleImpact)
	if !hasGap(f, domain.GapValidation, errRec.ID) || !anyContains(f.NextSteps, "Re-run validate.static-reference@v1") {
		t.Fatalf("error gap / step missing: %+v %v", f.Gaps, f.NextSteps)
	}
	// not-run is not a contradiction.
	nr := c.f.vrec(c.item, domain.ValidationOverrideInspection, domain.ValidationNotRun, detail("no pin rule applies"))
	cls, _ = run(t, c.f.in(c.tr, nil, c.pass, nr))
	wantClass(t, finding(t, cls, c.item), domain.FindingNotAffected, RuleValidatedClear)
}

// --- §3.3: what the agent can and cannot do -------------------------------------------

func TestAgentRaisesConcern(t *testing.T) {
	// blocks R2 until a passing validation answers it.
	c := newClean(t)
	concern := c.f.assertion(c.item, domain.StanceRaisesConcern, "the consumer may rely on the old mode")
	cls, _ := run(t, c.f.in(c.tr, agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{concern}), c.pass))
	f := finding(t, cls, c.item)
	wantClass(t, f, domain.FindingPotentiallyAffected, RulePlausibleImpact)
	if !hasGap(f, domain.GapAgent, concern.ID) {
		t.Fatalf("concern gap missing: %+v", f.Gaps)
	}
	answered := c.f.vrec(c.item, domain.ValidationStaticReference, domain.ValidationPass, paths(c.path), answers(concern.ID))
	cls, _ = run(t, c.f.in(c.tr, agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{concern}), c.pass, answered))
	f = finding(t, cls, c.item)
	wantClass(t, f, domain.FindingNotAffected, RuleValidatedClear)
	if len(f.Assertions) != 1 || len(f.Assumptions) != 0 {
		t.Fatalf("assertions %v assumptions %v", f.Assertions, f.Assumptions)
	}

	// can move R4 → R3.
	f2 := newFx(t)
	item := f2.item()
	f2.pair("argocd:apps/gizmo.yaml")
	cls, _ = run(t, f2.in((&trav{}).result(), nil))
	wantClass(t, finding(t, cls, item), domain.FindingInsufficientEvidence, RuleInsufficientEvidence)
	concern2 := f2.assertion(item, domain.StanceRaisesConcern, "a consumer outside the repo reads the mode")
	cls, _ = run(t, f2.in((&trav{}).result(), agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{concern2})))
	wantClass(t, finding(t, cls, item), domain.FindingPotentiallyAffected, RulePlausibleImpact)
}

// A concern about a behaviour item is lifted only by a clearing validation.
func TestAgentConcernOnBehaviourNeedsClearingAnswer(t *testing.T) {
	c := newClean(t, behaviour)
	concern := c.f.assertion(c.item, domain.StanceRaisesConcern, "the controller now rotates keys")
	pin := c.f.vrec(c.item, domain.ValidationOverrideInspection, domain.ValidationPass, rcs(c.rc), paths(c.path), withPin(c.f.envEvidence()))
	silentAnswer := c.f.vrec(c.item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(c.rc), answers(concern.ID))
	cls, _ := run(t, c.f.in(c.tr, agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{concern}), pin, silentAnswer))
	if f := finding(t, cls, c.item); f.Class == domain.FindingNotAffected {
		t.Fatal("render silence answered a behaviour concern")
	}
}

// Adversarial: an agent claims-mitigation with no validation never clears.
func TestAgentClaimsMitigationNeverClearsAlone(t *testing.T) {
	f := newFx(t)
	item := f.item(behaviour)
	f.pair("argocd:apps/gizmo.yaml")
	claim := f.assertion(item, domain.StanceClaimsMitigation, "the environment sets --mode=safe, so the new default does not apply")
	explain := f.assertion(item, domain.StanceExplains, "not affected: the setting pins the behaviour")
	agent := agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{claim, explain})
	cls, _ := run(t, f.in((&trav{}).result(), agent))
	fd := finding(t, cls, item)
	if fd.Class == domain.FindingNotAffected || fd.Approval.Eligible {
		t.Fatal("an unvalidated agent mitigation claim cleared the item")
	}
	if len(fd.Assumptions) != 1 || fd.Assumptions[0] != claim.ID || len(fd.Assertions) != 2 {
		t.Fatalf("the claim must be an unvalidated assumption: assumptions %v assertions %v", fd.Assumptions, fd.Assertions)
	}
	// Even with a passing render (rendered-config item), the claim blocks R2.
	c := newClean(t)
	claim2 := c.f.assertion(c.item, domain.StanceClaimsMitigation, "an overlay patches the mode back")
	cls, _ = run(t, c.f.in(c.tr, agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{claim2}), c.pass))
	fc := finding(t, cls, c.item)
	if fc.Class == domain.FindingNotAffected || len(fc.Assumptions) != 1 {
		t.Fatalf("claim must block R2 for its finding: %s %v", fc.Class, fc.Assumptions)
	}
	// Established by a validation: removed from the assumptions, then R2 holds.
	est := c.f.vrec(c.item, domain.ValidationOverrideInspection, domain.ValidationPass, paths(c.path), answers(claim2.ID))
	cls, _ = run(t, c.f.in(c.tr, agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{claim2}), c.pass, est))
	fc = finding(t, cls, c.item)
	wantClass(t, fc, domain.FindingNotAffected, RuleValidatedClear)
	if len(fc.Assumptions) != 0 {
		t.Fatalf("an established claim is not an assumption: %v", fc.Assumptions)
	}
}

// explains changes nothing.
func TestAgentExplainsChangesNothing(t *testing.T) {
	for _, withValidation := range []bool{true, false} {
		c := newClean(t)
		ex := c.f.assertion(c.item, domain.StanceExplains, "the change looks harmless")
		var vals []domain.ValidationRecord
		if withValidation {
			vals = append(vals, c.pass)
		}
		without, _ := run(t, c.f.in(c.tr, nil, vals...))
		with, _ := run(t, c.f.in(c.tr, agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{ex}), vals...))
		a, b := finding(t, without, c.item), finding(t, with, c.item)
		if a.Class != b.Class || a.Rule != b.Rule || len(b.Assumptions) != 0 || len(b.Assertions) != 1 {
			t.Fatalf("explains changed the finding: %s/%s → %s/%s, assumptions %v", a.Class, a.Rule, b.Class, b.Rule, b.Assumptions)
		}
	}
}

// A proposed edge reaches the policy only as an inferred edge in the
// re-run traversal: an unresolved path that blocks R2 (never completes it).
func TestAgentProposedEdgeOnlyAddsUnresolvedPaths(t *testing.T) {
	c := newClean(t)
	tb := &trav{}
	tb.root(c.item, node(controllerObj))
	p1 := tb.path(c.item, domain.EdgeEstablished, node(controllerObj), node("apps/v1/Deployment/app/consumer"))
	p2 := tb.path(c.item, domain.EdgeInferred, node(controllerObj), node("apps/v1/Deployment/other/reader"))
	pass := c.f.vrec(c.item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(c.rc), paths(p1, p2))
	cls, _ := run(t, c.f.in(tb.result(), nil, pass))
	f := finding(t, cls, c.item)
	wantClass(t, f, domain.FindingPotentiallyAffected, RulePlausibleImpact)
	if !hasGap(f, domain.GapRelationship, "") {
		t.Fatalf("gaps %+v", f.Gaps)
	}
}

// No agent record can produce affected: only validations can.
func TestAgentCannotProduceAffected(t *testing.T) {
	f := newFx(t)
	item := f.item()
	f.pair("argocd:apps/gizmo.yaml")
	var asrts []domain.AgentAssertion
	for i, s := range []string{"this definitely breaks the environment", "affected: the controller will crash", "action required"} {
		_ = i
		asrts = append(asrts, f.assertion(item, domain.StanceRaisesConcern, s))
	}
	cls, _ := run(t, f.in((&trav{}).result(), agentOutcome(domain.AgentSubmitted, asrts)))
	if fd := finding(t, cls, item); fd.Class == domain.FindingAffected {
		t.Fatal("agent concerns produced affected")
	}
}

// --- R3 / R4 -----------------------------------------------------------------------

func TestR3Signals(t *testing.T) {
	t.Run("correlated rendered change", func(t *testing.T) {
		c := newClean(t)
		cls, _ := run(t, c.f.in((&trav{}).result(), nil))
		f := finding(t, cls, c.item)
		wantClass(t, f, domain.FindingPotentiallyAffected, RulePlausibleImpact)
		if len(f.NextSteps) == 0 || !strings.Contains(f.Reasoning, "Plausible impact") {
			t.Fatalf("R3 needs next steps and states its signal: %s", f.Reasoning)
		}
	})
	t.Run("join review-required", func(t *testing.T) {
		f := newFx(t)
		item := f.item()
		f.pair("argocd:apps/gizmo.yaml")
		in := f.in((&trav{}).result(), nil)
		it, _ := in.Bundle.Item(item)
		in.Report = &domain.ImpactReport{Findings: []domain.ImpactFinding{{ID: "imf-1", ChangeID: it.ChangeID, Classification: domain.ImpactReviewRequired, Rule: "impact:values-set"}}}
		cls, _ := run(t, in)
		fd := finding(t, cls, item)
		wantClass(t, fd, domain.FindingPotentiallyAffected, RulePlausibleImpact)
		if len(fd.ImpactFindings) != 1 || fd.ImpactFindings[0].Classification != domain.ImpactReviewRequired {
			t.Fatalf("join refs %+v", fd.ImpactFindings)
		}
	})
	t.Run("inconclusive validation on an exposed subject", func(t *testing.T) {
		f := newFx(t)
		item := f.item()
		f.pair("argocd:apps/gizmo.yaml")
		tr := (&trav{}).root(item, node(controllerObj))
		errRec := f.vrec(item, domain.ValidationStaticReference, domain.ValidationError, subjects(node(controllerObj)))
		cls, _ := run(t, f.in(tr.result(), nil, errRec))
		if fd := finding(t, cls, item); !strings.Contains(fd.Reasoning, "inconclusive on an exposed subject") {
			t.Fatalf("reasoning: %s", fd.Reasoning)
		}
	})
}

func TestR4InsufficientEvidence(t *testing.T) {
	f := newFx(t)
	item := f.item()
	f.brokenPair("argocd:apps/gizmo.yaml", "values-incomplete: value files in another repo")
	cls, _ := run(t, f.in((&trav{}).result(), agentOutcome(domain.AgentBudgetExhausted, nil)))
	fd := finding(t, cls, item)
	wantClass(t, fd, domain.FindingInsufficientEvidence, RuleInsufficientEvidence)
	if !hasGap(fd, domain.GapCollection, "render:argocd:apps/gizmo.yaml") || !hasGap(fd, domain.GapAgent, "agent") {
		t.Fatalf("gaps must name the incomplete render and the unfinished agent: %+v", fd.Gaps)
	}
	if !anyContains(fd.NextSteps, "Complete the environment render of argocd:apps/gizmo.yaml") || !anyContains(fd.NextSteps, "agent") {
		t.Fatalf("next steps %v", fd.NextSteps)
	}
}

// scope:rendered-unexplained items are treated like scope:category: never
// affected without a failing validation, cleared only by a covering pass.
func TestRenderedUnexplainedItem(t *testing.T) {
	f := newFx(t)
	ids := f.pair("argocd:apps/gizmo.yaml", rc("resource-added", "rbac.authorization.k8s.io/v1/ClusterRole/-/gizmo-extra", ""))
	item := f.envItem(ids[0])
	cls, _ := run(t, f.in((&trav{}).result(), nil))
	fd := finding(t, cls, item)
	wantClass(t, fd, domain.FindingPotentiallyAffected, RulePlausibleImpact)
	if len(fd.RenderedChanges) != 1 || fd.RenderedChanges[0] != ids[0] {
		t.Fatalf("finding lists its rendered change: %v", fd.RenderedChanges)
	}
	pass := f.vrec(item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(ids[0]))
	cls, _ = run(t, f.in((&trav{}).result(), nil, pass))
	wantClass(t, finding(t, cls, item), domain.FindingNotAffected, RuleValidatedClear)
}

// --- NextSteps name the setting (§3.2) ----------------------------------------------------

func TestNextStepsNameTheSetting(t *testing.T) {
	f := newFx(t)
	ev := f.evidence("The controller now defaults to rotating keys. Set `privateKey.rotationPolicy: Never` or pass --legacy-key-rotation=true to keep the old behaviour.")
	item := f.item(behaviour, func(it *domain.ChangeItem) { it.Evidence = []domain.EvidenceID{ev} })
	f.pair("argocd:apps/gizmo.yaml")
	cls, _ := run(t, f.in((&trav{}).result(), nil))
	fd := finding(t, cls, item)
	if len(fd.NextSteps) == 0 || !strings.Contains(fd.NextSteps[0], "--legacy-key-rotation") || !strings.Contains(fd.NextSteps[0], "privateKey.rotationPolicy") {
		t.Fatalf("first next step must name the settings: %v", fd.NextSteps)
	}
	if !strings.Contains(fd.NextSteps[0], "override inspection") {
		t.Fatalf("next step must ask for an override inspection: %s", fd.NextSteps[0])
	}

	// Hypotheses contribute setting names too.
	f2 := newFx(t)
	item2 := f2.item(behaviour)
	f2.pair("argocd:apps/gizmo.yaml")
	h := f2.hypothesis(item2, "the annotation gizmo.example.io/legacy-mode on the Widget restores the old default")
	cls, _ = run(t, f2.in((&trav{}).result(), agentOutcome(domain.AgentSubmitted, nil, h)))
	if fd := finding(t, cls, item2); !strings.Contains(fd.NextSteps[0], "gizmo.example.io/legacy-mode") {
		t.Fatalf("hypothesis setting missing: %v", fd.NextSteps)
	}

	// Upstream names none: observe.
	f3 := newFx(t)
	ev3 := f3.evidence("The controller behaves differently now.")
	item3 := f3.item(behaviour, func(it *domain.ChangeItem) { it.Evidence = []domain.EvidenceID{ev3}; it.Title = "behaviour change" })
	f3.pair("argocd:apps/gizmo.yaml")
	cls, _ = run(t, f3.in((&trav{}).result(), nil))
	if fd := finding(t, cls, item3); !strings.Contains(fd.NextSteps[0], "isolated cluster") || !strings.Contains(fd.NextSteps[0], "names no setting") {
		t.Fatalf("want the observe step: %v", fd.NextSteps)
	}
}

func TestSettingExtraction(t *testing.T) {
	x := &index{ev: map[domain.EvidenceID]domain.Evidence{"e1": {ID: "e1", Excerpt: "Use `--feature-gates=Foo=true`, set controller.config.legacyMode, the env var GIZMO_LEGACY_MODE, or the annotation gizmo.example.io/mode. See https://example.invalid/docs.md and v1.2.3."}}}
	e := &itemEval{item: domain.ChangeItem{Title: "x", Evidence: []domain.EvidenceID{"e1"}}}
	got := strings.Join(x.settings(e), " ")
	for _, want := range []string{"--feature-gates", "controller.config.legacyMode", "GIZMO_LEGACY_MODE", "gizmo.example.io/mode"} {
		if !strings.Contains(got, want) {
			t.Errorf("settings %q lack %q", got, want)
		}
	}
	for _, not := range []string{"https", "v1.2.3", "docs.md"} {
		if strings.Contains(got, not) {
			t.Errorf("settings %q contain %q", got, not)
		}
	}
}

// --- input errors --------------------------------------------------------------------

func TestInvalidInputIsAnError(t *testing.T) {
	c := newClean(t)
	ai := c.pass
	ai.Provenance = aiProv("call-x", c.f.up.ID)
	bad := []struct {
		name string
		in   investigate.ClassifyInput
	}{
		{"no bundle", investigate.ClassifyInput{}},
		{"unknown policy", func() investigate.ClassifyInput {
			in := c.f.in(c.tr, nil)
			in.Config.Policy = "classify.policy@v9"
			return in
		}()},
		{"AI validation", c.f.in(c.tr, nil, ai)},
		{"duplicate validation", c.f.in(c.tr, nil, c.pass, c.pass)},
		{"validation of an unknown item", c.f.in(c.tr, nil, c.f.vrec("ci-unknown", domain.ValidationRenderDiff, domain.ValidationPass))},
	}
	for _, tc := range bad {
		if _, err := New().Classify(context.Background(), tc.in); err == nil {
			t.Errorf("%s: want an error", tc.name)
		} else if investigate.ClassOf(err) != investigate.Permanent {
			t.Errorf("%s: error must be permanent: %v", tc.name, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New().Classify(ctx, c.f.in(c.tr, nil)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

// Out-of-scope items get no finding.
func TestOutOfScopeItemsAreNotClassified(t *testing.T) {
	c := newClean(t)
	out := c.f.item(outOfScope)
	cls, _ := run(t, c.f.in(c.tr, nil, c.pass))
	for _, f := range cls.Findings {
		if f.ChangeItem == out {
			t.Fatal("out-of-scope item classified")
		}
	}
	if len(cls.Findings) != 1 {
		t.Fatalf("findings %d", len(cls.Findings))
	}
}

// A compatibility item takes the join's platform checks no change id names.
func TestPlatformJoinFindings(t *testing.T) {
	f := newFx(t)
	item := f.item(platform("kubernetes"))
	f.input(domain.InputClusterVersion, "", "1.31", domain.CollectionOK, "")
	in := f.in((&trav{}).result(), nil)
	in.Report = &domain.ImpactReport{Findings: []domain.ImpactFinding{
		{ID: "imf-k8s", Classification: domain.ImpactReviewRequired, Rule: "impact:kubernetes-range", Checks: []domain.ImpactCheck{{Dimension: domain.DimensionCluster, Platform: "Kubernetes"}}},
		{ID: "imf-peer", Classification: domain.ImpactReviewRequired, Rule: "impact:peer", Checks: []domain.ImpactCheck{{Dimension: domain.DimensionCluster, Platform: "istio"}}},
		{ID: "imf-chg", ChangeID: "chg-x", Classification: domain.ImpactReviewRequired, Rule: "impact:values-set"},
	}}
	cls, _ := run(t, in)
	fd := finding(t, cls, item)
	if len(fd.ImpactFindings) != 1 || fd.ImpactFindings[0].ID != "imf-k8s" {
		t.Fatalf("join refs %+v", fd.ImpactFindings)
	}
	wantClass(t, fd, domain.FindingPotentiallyAffected, RulePlausibleImpact)
}

func TestSettingExtractionCamelCaseInBackticks(t *testing.T) {
	x := &index{ev: map[domain.EvidenceID]domain.Evidence{"e1": {ID: "e1", Excerpt: "set `rotationPolicy` to keep it; `Never` is a value; `the old way` is prose"}}}
	got := x.settings(&itemEval{item: domain.ChangeItem{Title: "x", Evidence: []domain.EvidenceID{"e1"}}})
	if len(got) != 1 || got[0] != "rotationPolicy" {
		t.Fatalf("settings %v", got)
	}
}
