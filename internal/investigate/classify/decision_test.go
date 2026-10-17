package classify

import (
	"bytes"
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/policy"
)

// --- §3.4 approval eligibility --------------------------------------------------------

func TestDecisionConjunction(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(c *clean, in *investigate.ClassifyInput)
		reason string
	}{
		{"agent not run", func(c *clean, in *investigate.ClassifyInput) { in.Agent = nil }, "agent stage did not run"},
		{"agent budget exhausted", func(c *clean, in *investigate.ClassifyInput) {
			in.Agent = agentOutcome(domain.AgentBudgetExhausted, nil)
		}, "did not complete"},
		{"agent failed", func(c *clean, in *investigate.ClassifyInput) { in.Agent = agentOutcome(domain.AgentFailed, nil) }, "did not complete"},
		{"tier policy review", func(c *clean, in *investigate.ClassifyInput) {
			in.Config.TierPolicy = "sha256:" + strings.Repeat("a", 64)
			in.Tier = &investigate.TierVerdict{Policy: "default", Digest: in.Config.TierPolicy, Tier: "review", Why: []string{"breaking change"}}
		}, "not auto-pass"},
		{"tier policy configured, verdict missing", func(c *clean, in *investigate.ClassifyInput) {
			in.Config.TierPolicy = "sha256:" + strings.Repeat("a", 64)
		}, "verdict is missing"},
		{"tier verdict of another policy", func(c *clean, in *investigate.ClassifyInput) {
			in.Config.TierPolicy = "sha256:" + strings.Repeat("a", 64)
			in.Tier = &investigate.TierVerdict{Policy: "other", Digest: "sha256:" + strings.Repeat("b", 64), Tier: "auto-pass"}
		}, "configured tier policy"},
		{"tier verdict block without configuration", func(c *clean, in *investigate.ClassifyInput) {
			in.Tier = &investigate.TierVerdict{Policy: "default", Digest: "sha256:" + strings.Repeat("b", 64), Tier: "block"}
		}, "not auto-pass"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newClean(t)
			in := c.f.in(c.tr, nil, c.pass)
			tc.mutate(c, &in)
			cls, _ := run(t, in)
			// every item is still not-affected and eligible: the per-finding
			// policy does not depend on the investigation-level conjuncts …
			wantClass(t, finding(t, cls, c.item), domain.FindingNotAffected, RuleValidatedClear)
			// … but the investigation is not eligible.
			if cls.Decision.Outcome != domain.PolicyHumanReview || !anyContains(cls.Decision.Reasons, tc.reason) {
				t.Fatalf("decision %+v, want human review with %q", cls.Decision, tc.reason)
			}
		})
	}
	t.Run("tier auto-pass", func(t *testing.T) {
		c := newClean(t)
		in := c.f.in(c.tr, nil, c.pass)
		in.Config.TierPolicy = "sha256:" + strings.Repeat("a", 64)
		in.Tier = &investigate.TierVerdict{Policy: "default", Digest: in.Config.TierPolicy, Tier: string(policy.AutoPass)}
		cls, _ := run(t, in)
		if cls.Decision.Outcome != domain.PolicyAutoApproveEligible {
			t.Fatalf("decision %+v", cls.Decision)
		}
	})
}

// Adversarial: an uncorrelated environment rendered change blocks
// eligibility even when every in-scope item is cleared.
func TestUncorrelatedRenderedChangeBlocksEligibility(t *testing.T) {
	c := newClean(t)
	c.f.pairs[0].Changes = append(c.f.pairs[0].Changes, rc("resource-added", "v1/ConfigMap/gizmo/surprise", ""))
	c.f.pairs[0].Changes[1].ID = domain.RenderedChangeID(domain.RenderEnvironment, c.f.pairs[0].Target, "v1/ConfigMap/gizmo/surprise", "", "resource-added")
	c.f.pairs[0].Changes[1].Evidence = c.f.pairs[0].Changes[0].Evidence
	sortChanges(c.f.pairs[0].Changes)
	cls, _ := run(t, c.f.in(c.tr, nil, c.pass))
	wantClass(t, finding(t, cls, c.item), domain.FindingNotAffected, RuleValidatedClear)
	if cls.Decision.Outcome != domain.PolicyHumanReview || !anyContains(cls.Decision.Reasons, "no change item explains") {
		t.Fatalf("decision %+v", cls.Decision)
	}
}

func TestDecisionRenders(t *testing.T) {
	c := newClean(t)
	c.f.brokenPair("helmfile:gizmo", "chart-unavailable")
	cls, _ := run(t, c.f.in(c.tr, nil, c.pass))
	if cls.Decision.Outcome != domain.PolicyHumanReview || !anyContains(cls.Decision.Reasons, "helmfile:gizmo") {
		t.Fatalf("decision %+v", cls.Decision)
	}
}

func TestDecisionNoInScopeItems(t *testing.T) {
	f := newFx(t)
	f.item(outOfScope)
	f.pair("argocd:apps/gizmo.yaml")
	cls, _ := run(t, f.in((&trav{}).result(), nil))
	if len(cls.Findings) != 0 || cls.Decision.Outcome != domain.PolicyHumanReview {
		t.Fatalf("no in-scope item ⇒ human review: %+v", cls.Decision)
	}
}

// AutoApprovalEnabled stays false in milestone 1, whatever the config says.
func TestAutoApprovalNeverEnacted(t *testing.T) {
	c := newClean(t)
	in := c.f.in(c.tr, nil, c.pass)
	in.Config.AutoApproval = true
	cls, inv := run(t, in)
	if cls.Decision.Outcome != domain.PolicyAutoApproveEligible || cls.Decision.AutoApprovalEnabled {
		t.Fatalf("decision %+v", cls.Decision)
	}
	if !anyContains(cls.Decision.Reasons, "shadow-only") || !anyContains(cls.Decision.Reasons, "stays disabled") {
		t.Fatalf("reasons %v", cls.Decision.Reasons)
	}
	if inv.Review.State != domain.ReviewAwaiting || inv.Status != domain.InvestigationAwaiting {
		t.Fatalf("an eligible run waits for a human: %s %s", inv.Review.State, inv.Status)
	}
}

func TestTierVerdictConversion(t *testing.T) {
	if TierVerdict(nil) != nil {
		t.Fatal("nil in, nil out")
	}
	v := &policy.Verdict{Policy: "p", Digest: "sha256:x", Tier: policy.Review,
		Why: []policy.Group{{Rule: "r1", Tier: policy.Review, Reason: "breaking", Count: 2}, {Rule: "ok", Tier: policy.AutoPass, Count: 1}}}
	got := TierVerdict(v)
	if got.Tier != "review" || got.Digest != "sha256:x" || len(got.Why) != 1 || !strings.Contains(got.Why[0], "r1 ×2") {
		t.Fatalf("%+v", got)
	}
}

// --- §3.5 ACTION REQUIRED is never weakened ---------------------------------------------

func TestActionRequiredContract(t *testing.T) {
	t.Run("action-required join ⇒ affected", func(t *testing.T) {
		c := newClean(t)
		in := c.f.in(c.tr, nil, c.pass)
		it, _ := in.Bundle.Item(c.item)
		join := domain.ImpactFinding{ID: "imf-act", ChangeID: it.ChangeID, Classification: domain.ImpactActionRequired, Rule: "impact:values-removed"}
		in.Report = &domain.ImpactReport{Findings: []domain.ImpactFinding{join}}
		// even with every R2 condition met by other validations, the wrapped
		// fail decides.
		wrap := c.f.vrec(c.item, domain.ValidationOverrideInspection, domain.ValidationFail, validator("validate.join@v1"), detail("ACTION REQUIRED by the deterministic join"))
		wrap.ImpactFinding = join.ID
		in.Validations = append(in.Validations, wrap)
		cls, inv := run(t, in)
		f := finding(t, cls, c.item)
		wantClass(t, f, domain.FindingAffected, RuleValidatedExposure)
		if len(f.ImpactFindings) != 1 || f.ImpactFindings[0] != (domain.ImpactFindingRef{ID: join.ID, Classification: domain.ImpactActionRequired, Rule: join.Rule}) {
			t.Fatalf("the join finding is referenced unchanged: %+v", f.ImpactFindings)
		}
		if in.Report.Findings[0].Classification != domain.ImpactActionRequired {
			t.Fatal("the classifier edited the join's finding")
		}
		var buf bytes.Buffer
		if err := WriteText(&buf, inv, in.Bundle); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "AFFECTED (validated, deterministic) · action: ACTION REQUIRED (deterministic join)") {
			t.Fatalf("text:\n%s", buf.String())
		}
	})
	t.Run("action-required join without its wrapped fail is an error", func(t *testing.T) {
		c := newClean(t)
		in := c.f.in(c.tr, nil, c.pass)
		it, _ := in.Bundle.Item(c.item)
		in.Report = &domain.ImpactReport{Findings: []domain.ImpactFinding{{ID: "imf-act", ChangeID: it.ChangeID, Classification: domain.ImpactActionRequired, Rule: "impact:values-removed"}}}
		_, err := New().Classify(context.Background(), in)
		if err == nil || !strings.Contains(err.Error(), "action-required") {
			t.Fatalf("want an error, got %v", err)
		}
	})
	t.Run("affected below action shows action: review", func(t *testing.T) {
		c := newClean(t)
		in := c.f.in(c.tr, nil, c.f.vrec(c.item, domain.ValidationRenderDiff, domain.ValidationFail, rcs(c.rc)))
		it, _ := in.Bundle.Item(c.item)
		in.Report = &domain.ImpactReport{Findings: []domain.ImpactFinding{{ID: "imf-inf", ChangeID: it.ChangeID, Classification: domain.ImpactInformational, Rule: "impact:values-default-changed"}}}
		cls, inv := run(t, in)
		wantClass(t, finding(t, cls, c.item), domain.FindingAffected, RuleValidatedExposure)
		var buf bytes.Buffer
		_ = WriteText(&buf, inv, in.Bundle)
		if !strings.Contains(buf.String(), "AFFECTED (validated, deterministic) · action: review") || strings.Contains(buf.String(), "ACTION REQUIRED") {
			t.Fatalf("the investigation never writes ACTION REQUIRED:\n%s", buf.String())
		}
	})
	t.Run("not-affected join is evidence, not sufficient alone", func(t *testing.T) {
		c := newClean(t)
		in := c.f.in(c.tr, nil, c.f.vrec(c.item, domain.ValidationOverrideInspection, domain.ValidationPass, validator("validate.join@v1")))
		it, _ := in.Bundle.Item(c.item)
		in.Report = &domain.ImpactReport{Findings: []domain.ImpactFinding{{ID: "imf-na", ChangeID: it.ChangeID, Classification: domain.ImpactNotAffected, Rule: "impact:values-set-no-effect"}}}
		cls, _ := run(t, in)
		if f := finding(t, cls, c.item); f.Class == domain.FindingNotAffected {
			t.Fatal("a join pass alone cleared an item with uncovered exposure")
		}
	})
	t.Run("unknown join is a gap with its reason", func(t *testing.T) {
		c := newClean(t)
		in := c.f.in(c.tr, nil, c.pass)
		it, _ := in.Bundle.Item(c.item)
		in.Report = &domain.ImpactReport{Findings: []domain.ImpactFinding{{ID: "imf-unk", ChangeID: it.ChangeID, Classification: domain.ImpactUnknown, Rule: "impact:values-unknown",
			UnknownReason: domain.UnknownEnvironmentVisibilityGap, NeededToDetermine: []string{"Helm values files (--values) not supplied"}}}}
		cls, _ := run(t, in)
		f := finding(t, cls, c.item)
		if f.Class == domain.FindingNotAffected || !hasGap(f, domain.GapValidation, "imf-unk") {
			t.Fatalf("%s %+v", f.Class, f.Gaps)
		}
		for _, g := range f.Gaps {
			if g.Subject == "imf-unk" && g.UnknownReason != domain.UnknownEnvironmentVisibilityGap {
				t.Fatalf("unknownReason not mapped: %+v", g)
			}
		}
		if !anyContains(f.NextSteps, "Helm values files") {
			t.Fatalf("steps %v", f.NextSteps)
		}
	})
}

// impact:not-joined says the join does not evaluate the change (every
// declared change gets it): it never blocks R2 by itself — the item is
// cleared by the investigation's own validations — but it stays listed when
// the item is not cleared. Any other unknown join keeps blocking.
func TestNotJoinedJoinIsNoR2Blocker(t *testing.T) {
	notJoined := func(in *investigate.ClassifyInput, change string) {
		in.Report = &domain.ImpactReport{Findings: []domain.ImpactFinding{{ID: "imf-nj", ChangeID: change, Classification: domain.ImpactUnknown,
			Rule: "impact:not-joined", UnknownReason: domain.UnknownReleaseKnowledgeGap,
			NeededToDetermine: []string{"no machine-comparable subject (declared change)"}}}}
	}
	t.Run("cleared item", func(t *testing.T) {
		c := newClean(t)
		in := c.f.in(c.tr, nil, c.pass)
		it, _ := in.Bundle.Item(c.item)
		notJoined(&in, it.ChangeID)
		cls, _ := run(t, in)
		if f := finding(t, cls, c.item); f.Class != domain.FindingNotAffected || hasGap(f, domain.GapValidation, "imf-nj") {
			t.Fatalf("a not-joined join blocked an otherwise cleared item: %s %+v", f.Class, f.Gaps)
		}
	})
	t.Run("not cleared for another reason", func(t *testing.T) {
		c := newClean(t)
		in := c.f.in(c.tr, nil) // no passing validation
		it, _ := in.Bundle.Item(c.item)
		notJoined(&in, it.ChangeID)
		cls, _ := run(t, in)
		f := finding(t, cls, c.item)
		if f.Class == domain.FindingNotAffected || !hasGap(f, domain.GapValidation, "imf-nj") || !anyContains(f.NextSteps, "machine-comparable") {
			t.Fatalf("the not-joined gap must be listed on an uncleared item: %s %+v %v", f.Class, f.Gaps, f.NextSteps)
		}
	})
	t.Run("other unknown rule still blocks", func(t *testing.T) {
		c := newClean(t)
		in := c.f.in(c.tr, nil, c.pass)
		it, _ := in.Bundle.Item(c.item)
		notJoined(&in, it.ChangeID)
		in.Report.Findings[0].Rule = "impact:values-unknown"
		cls, _ := run(t, in)
		if f := finding(t, cls, c.item); f.Class == domain.FindingNotAffected {
			t.Fatal("an unknown join with another rule must keep blocking R2")
		}
	})
}

// --- reproducibility ---------------------------------------------------------------------

// Same inputs ⇒ same findings (digests), regardless of record order; a run
// record re-classified from what it stores reproduces its findings.
func TestReproducibility(t *testing.T) {
	c := newClean(t)
	item2 := c.f.item(behaviour)
	concern := c.f.assertion(c.item, domain.StanceRaisesConcern, "consumer may break")
	claim := c.f.assertion(item2, domain.StanceClaimsMitigation, "the flag --mode=safe is set")
	h := c.f.hypothesis(item2, "--mode=safe pins it")
	vals := []domain.ValidationRecord{
		c.pass,
		c.f.vrec(c.item, domain.ValidationStaticReference, domain.ValidationNotRun, detail("no paths")),
		c.f.vrec(item2, domain.ValidationRenderDiff, domain.ValidationPass),
		c.f.vrec(item2, domain.ValidationOverrideInspection, domain.ValidationNotRun, detail("no pin rule")),
	}
	agent := agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{concern, claim}, h)
	base, inv := run(t, c.f.in(c.tr, agent, vals...))
	want := mustJSON(t, base)

	r := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		v := append([]domain.ValidationRecord(nil), vals...)
		r.Shuffle(len(v), func(a, b int) { v[a], v[b] = v[b], v[a] })
		a := *agent
		a.Assertions = append([]domain.AgentAssertion(nil), agent.Assertions...)
		r.Shuffle(len(a.Assertions), func(x, y int) { a.Assertions[x], a.Assertions[y] = a.Assertions[y], a.Assertions[x] })
		got, err := New().Classify(context.Background(), c.f.in(c.tr, &a, v...))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(mustJSON(t, got), want) {
			t.Fatalf("order changed the classification")
		}
	}

	// Re-classification of the stored run (the §6 reproducibility metric).
	re := investigate.ClassifyInput{Bundle: c.f.bundle(), Traversal: inv.Traversal, Validations: inv.Validations,
		Agent:  &investigate.AgentOutcome{Run: *inv.Agent, Hypotheses: inv.Hypotheses, Assertions: inv.Assertions, Artifacts: inv.Artifacts, Evidence: inv.Evidence},
		Config: c.f.in(nil, nil).Config}
	got, err := New().Classify(context.Background(), re)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mustJSON(t, got.Findings), mustJSON(t, inv.Findings)) {
		t.Fatal("re-classifying the stored run does not reproduce its findings")
	}
	for i, f := range got.Findings {
		if f.ID != inv.Findings[i].ID {
			t.Fatalf("finding digest changed: %s vs %s", f.ID, inv.Findings[i].ID)
		}
	}
}

// --- AC6: validated vs assumed, on the face of the text ------------------------------------

func TestTextSeparatesValidatedFromAssumptions(t *testing.T) {
	c := newClean(t, behaviour)
	claim := c.f.assertion(c.item, domain.StanceClaimsMitigation, "UNIQUE-CLAIM the environment pins the old mode")
	ex := c.f.assertion(c.item, domain.StanceExplains, "UNIQUE-EXPLAIN the agent thinks it is fine")
	concern := c.f.assertion(c.item, domain.StanceRaisesConcern, "UNIQUE-CONCERN a reader of the mode exists")
	silent := c.f.vrec(c.item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(c.rc), paths(c.path), detail("UNIQUE-RENDER nothing attributable"))
	_, inv := run(t, c.f.in(c.tr, agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{claim, ex, concern}), silent))
	var buf bytes.Buffer
	if err := WriteText(&buf, inv, c.f.bundle()); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	section := func(header string) string {
		i := strings.Index(text, header)
		if i < 0 {
			t.Fatalf("missing section %q in:\n%s", header, text)
		}
		// up to the next two-space-indented section header
		lines := strings.Split(text[i+len(header):], "\n")
		var out []string
		for _, l := range lines[1:] {
			if strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "    ") {
				break
			}
			out = append(out, l)
		}
		return strings.Join(out, "\n")
	}
	if !strings.Contains(text, "POTENTIALLY AFFECTED (not validated)") && !strings.Contains(text, "INSUFFICIENT EVIDENCE (not validated)") {
		t.Fatalf("class label must say 'not validated':\n%s", text)
	}
	assumptions := section("UNVALIDATED ASSUMPTIONS")
	if !strings.Contains(assumptions, "UNIQUE-CLAIM") || strings.Contains(assumptions, "UNIQUE-RENDER") {
		t.Fatalf("assumptions section:\n%s", assumptions)
	}
	validated := section("NOT VALIDATED — no validation decides the item")
	if strings.Contains(validated, "UNIQUE-CLAIM") || strings.Contains(validated, "UNIQUE-EXPLAIN") || !strings.Contains(validated, "UNIQUE-RENDER") {
		t.Fatalf("validation section mixes agent output:\n%s", validated)
	}
	if !strings.Contains(section("Agent explanations (AI output, not validations"), "UNIQUE-EXPLAIN") {
		t.Fatal("explanations not labelled")
	}
	if !strings.Contains(section("Agent concerns (AI output"), "UNIQUE-CONCERN") {
		t.Fatal("concerns not labelled")
	}
	if !strings.Contains(text, "render silence proves nothing") {
		t.Fatal("behaviour reasoning missing from the text")
	}
	if testing.Verbose() {
		t.Log("\n" + text)
	}

	// A not-affected finding's text names the validations it rests on.
	c2 := newClean(t)
	_, inv2 := run(t, c2.f.in(c2.tr, nil, c2.pass))
	buf.Reset()
	_ = WriteText(&buf, inv2, c2.f.bundle())
	if !strings.Contains(buf.String(), "NOT AFFECTED (validated, deterministic)") || !strings.Contains(buf.String(), "VALIDATED — the class rests on these deterministic validation(s)") ||
		!strings.Contains(buf.String(), c2.pass.ID) {
		t.Fatalf("text:\n%s", buf.String())
	}
}

// The reasoning string alone (the struct field shown by any consumer) also
// separates the two.
func TestReasoningLabels(t *testing.T) {
	c := newClean(t)
	claim := c.f.assertion(c.item, domain.StanceClaimsMitigation, "an overlay patches it")
	cls, _ := run(t, c.f.in(c.tr, agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{claim}), c.pass))
	f := finding(t, cls, c.item)
	if !strings.Contains(f.Reasoning, "NOT VALIDATED") || !strings.Contains(f.Reasoning, "AI output, never a validation") || !strings.Contains(f.Reasoning, "1 unvalidated assumption") {
		t.Fatalf("reasoning: %s", f.Reasoning)
	}
	cls, _ = run(t, c.f.in(c.tr, nil, c.pass))
	if f := finding(t, cls, c.item); !strings.HasPrefix(f.Reasoning, "R2 classify:validated-clear → not-affected. VALIDATED") {
		t.Fatalf("reasoning: %s", f.Reasoning)
	}
}

// --- review ------------------------------------------------------------------------------

func classified(t *testing.T) *domain.Investigation {
	c := newClean(t)
	_, inv := run(t, c.f.in(c.tr, nil, c.pass))
	return inv
}

func human(action investigate.ReviewAction, at time.Time) investigate.ReviewDecision {
	return investigate.ReviewDecision{Action: action, Reviewer: "alice", ReviewerKind: domain.ReviewerHuman, At: at, Note: "looked at the diff"}
}

func TestReviewHuman(t *testing.T) {
	ra := NewReviewApplier()
	at := testNow.Add(time.Hour)
	for _, tc := range []struct {
		action investigate.ReviewAction
		state  domain.ReviewState
		status domain.InvestigationStatus
	}{
		{investigate.ReviewApprove, domain.ReviewApproved, domain.InvestigationCompleted},
		{investigate.ReviewReject, domain.ReviewRejected, domain.InvestigationCompleted},
		{investigate.ReviewRequestMoreEvidence, domain.ReviewMoreEvidenceWanted, domain.InvestigationRunning},
	} {
		inv := classified(t)
		n := len(inv.Audit)
		d := human(tc.action, at)
		d.Requests = []string{"the overlay values of prod"}
		if err := ra.ApplyReview(inv, d); err != nil {
			t.Fatalf("%s: %v", tc.action, err)
		}
		if inv.Review.State != tc.state || inv.Status != tc.status || inv.Review.Reviewer != "alice" || inv.Review.DecidedAt == nil {
			t.Fatalf("%s: review %+v status %s", tc.action, inv.Review, inv.Status)
		}
		last := inv.Audit[len(inv.Audit)-1]
		if len(inv.Audit) != n+1 || last.Actor != "human:alice" || last.Stage != domain.StageHumanReview || last.Event != "decided" {
			t.Fatalf("%s: audit %+v", tc.action, last)
		}
		if err := inv.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReviewProxyNeverApproves(t *testing.T) {
	inv := classified(t)
	at := testNow.Add(time.Hour)
	p := aiProv("proxy-call", "ev-x")
	for _, a := range []investigate.ReviewAction{investigate.ReviewApprove, investigate.ReviewReject} {
		d := investigate.ReviewDecision{Action: a, Reviewer: "claude-proxy", ReviewerKind: domain.ReviewerProxy, At: at, ProxyProvenance: &p}
		if err := NewReviewApplier().ApplyReview(inv, d); err != nil {
			t.Fatal(err)
		}
		if inv.Review.State != domain.ReviewAwaiting || inv.Review.ReviewerKind != domain.ReviewerProxy || inv.Review.ProxyProvenance == nil {
			t.Fatalf("a proxy decision is recorded and keeps waiting: %+v", inv.Review)
		}
		if last := inv.Audit[len(inv.Audit)-1]; last.Actor != "proxy:claude-proxy" || !strings.Contains(last.Detail, "not enacted") {
			t.Fatalf("audit %+v", last)
		}
	}
	// A human then approves.
	if err := NewReviewApplier().ApplyReview(inv, human(investigate.ReviewApprove, at.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if inv.Review.State != domain.ReviewApproved || inv.Review.ProxyProvenance != nil {
		t.Fatalf("%+v", inv.Review)
	}
}

func TestReviewRejectsInvalidDecisions(t *testing.T) {
	at := testNow.Add(time.Hour)
	p := aiProv("proxy-call", "ev-x")
	bad := map[string]investigate.ReviewDecision{
		"unknown action":       {Action: "merge", Reviewer: "alice", ReviewerKind: domain.ReviewerHuman, At: at},
		"no reviewer":          {Action: investigate.ReviewApprove, ReviewerKind: domain.ReviewerHuman, At: at},
		"no time":              {Action: investigate.ReviewApprove, Reviewer: "alice", ReviewerKind: domain.ReviewerHuman},
		"unknown kind":         {Action: investigate.ReviewApprove, Reviewer: "alice", ReviewerKind: "robot", At: at},
		"proxy w/o provenance": {Action: investigate.ReviewApprove, Reviewer: "m", ReviewerKind: domain.ReviewerProxy, At: at},
		"human with AI prov":   {Action: investigate.ReviewApprove, Reviewer: "alice", ReviewerKind: domain.ReviewerHuman, At: at, ProxyProvenance: &p},
		"more evidence, none":  {Action: investigate.ReviewRequestMoreEvidence, Reviewer: "alice", ReviewerKind: domain.ReviewerHuman, At: at},
		"time goes back":       {Action: investigate.ReviewApprove, Reviewer: "alice", ReviewerKind: domain.ReviewerHuman, At: testNow.Add(-time.Hour)},
	}
	for name, d := range bad {
		inv := classified(t)
		before := mustJSON(t, inv)
		if err := NewReviewApplier().ApplyReview(inv, d); err == nil {
			t.Errorf("%s: want an error", name)
		}
		if !bytes.Equal(before, mustJSON(t, inv)) {
			t.Errorf("%s: a rejected decision changed the record", name)
		}
	}
	// Final states are final; an unclassified run cannot be reviewed.
	inv := classified(t)
	if err := NewReviewApplier().ApplyReview(inv, human(investigate.ReviewReject, at)); err != nil {
		t.Fatal(err)
	}
	if err := NewReviewApplier().ApplyReview(inv, human(investigate.ReviewApprove, at)); err == nil {
		t.Fatal("approved a rejected investigation")
	}
	inv = classified(t)
	inv.Decision = nil
	if err := NewReviewApplier().ApplyReview(inv, human(investigate.ReviewApprove, at)); err == nil {
		t.Fatal("reviewed an unclassified investigation")
	}
}

// A human may approve a human-review-required investigation (AC8: the human
// is the decision); the audit says the policy did not find it eligible.
func TestReviewHumanApprovesNonEligible(t *testing.T) {
	f := newFx(t)
	f.item()
	f.pair("argocd:apps/gizmo.yaml")
	_, inv := run(t, f.in((&trav{}).result(), nil))
	if inv.Decision.Outcome != domain.PolicyHumanReview {
		t.Fatal("fixture should need review")
	}
	if err := NewReviewApplier().ApplyReview(inv, human(investigate.ReviewApprove, testNow.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inv.Audit[len(inv.Audit)-1].Detail, "did not find it eligible") {
		t.Fatalf("audit %+v", inv.Audit[len(inv.Audit)-1])
	}
}

func sortChanges(cs []domain.RenderedChange) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j].ID < cs[j-1].ID; j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}
