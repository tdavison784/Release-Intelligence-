package impact

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// Stubs and helpers for the PO-7a decision order (renderExposure): the
// customer's render decides exposure; knowledge only what rendering can't.

type stubSetEval struct{ r SetValuesResult }

func (s stubSetEval) EvaluateSetValues(domain.Change) SetValuesResult { return s.r }

type stubUnsetEval struct{ r ConditionResult }

func (u stubUnsetEval) EvaluateUnsetValues(domain.Change, string) ConditionResult { return u.r }

type stubImageEval struct{ r ImageResult }

func (s stubImageEval) EvaluateImage(domain.Change, string) ImageResult { return s.r }

func buildEval(t *testing.T, e *domain.UpgradeEdge, en *env.Environment, in Input) *domain.ImpactReport {
	t.Helper()
	in.Edge, in.Env, in.Now = e, en, testNow
	r, err := Build(in)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("report does not validate: %v", err)
	}
	return r
}

// renderRecord is an environment-scope render evidence record (chain-2 shape).
func renderRecord(excerpt string) domain.Evidence {
	ev := domain.NewEvidence(domain.EvidenceRenderedDiff, "render@v1", "render:example@1.1.0", "CronJob cleanup spec", excerpt, "sha256:d", testNow)
	rp := domain.RenderProvenance{Scope: domain.RenderEnvironment, Change: "image-changed"}
	ev.Render = &rp
	return ev
}

func noEffectCheck(key string, ev ...domain.Evidence) domain.ImpactCheck {
	c := domain.ImpactCheck{Dimension: domain.DimensionRender, Render: &domain.RenderCheck{Outcome: domain.RenderNoAttributableChange, Key: key}}
	for _, e := range ev {
		c.Evidence = append(c.Evidence, e.ID)
	}
	return c
}

// A decisive no-effect render stops a trusted fact's exposure claim: the
// render checked the customer's configuration and nothing is attributable, so
// the fact's values-key condition cannot re-expose the change.
func TestRenderNoEffectStopsKnowledgeExposure(t *testing.T) {
	eb := newEdge()
	removed := eb.change(upgrade.RuleValuesRemoved, "Helm value tls.secretsBackend removed", "tls.secretsBackend")
	a := domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectHelmValue, Product: "example", Path: "tls.secretsBackend"},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindRemoved},
		Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpValuesKey, Path: "tls.secretsBackend", State: domain.StateSet}},
		Consequence:   &domain.Consequence{Kind: domain.ConsequenceSettingIgnored, ExposedClass: domain.ImpactActionRequired, Statement: "The key stops taking effect."},
		Statement:     "tls.secretsBackend was removed and the setting is ignored.",
	}
	f := fact(t, a, domain.VerifiedHuman)
	en := condEnv(t, "", nil) // tls.secretsBackend IS set: the fact's own condition is True
	rec := renderRecord("1.0.0 → 1.1.0: key tls.secretsBackend set and unset — no attributable difference")
	set := stubSetEval{r: SetValuesResult{Outcome: SetValuesNoEffect,
		Checks: []domain.ImpactCheck{noEffectCheck("tls.secretsBackend", rec)}, Records: []domain.Evidence{rec}}}
	rep := buildEval(t, eb.edge, en, Input{Facts: []domain.VerifiedFact{f}, Set: set})
	fs := findingsOf(rep, removed.ID)
	if len(fs) != 1 || fs[0].Rule != RuleValuesSetNoEffect || fs[0].Classification != domain.ImpactNotAffected || fs[0].Knowledge != nil {
		t.Fatalf("the render's no-effect must stand alone: %+v", fs)
	}
	// an undecided render leaves the fact its say (today's composition)
	und := stubSetEval{r: SetValuesResult{Outcome: SetValuesUndecided, Needed: []string{"no render decided"}}}
	rep = buildEval(t, eb.edge, en, Input{Facts: []domain.VerifiedFact{f}, Set: und})
	// the fact (trusted, exposure true, ACTION consequence) may raise — but the
	// deterministic verdict is already ACTION, so composition adds nothing
	if fs := findingsOf(rep, removed.ID); len(fs) != 1 || fs[0].Rule != RuleValuesRemoved {
		t.Fatalf("undecided render: %+v", fs)
	}
}

// An attributable render supplies the exposure evidence, a trusted fact the
// consequence: together an ACTION the render alone could never state (trust
// ladder unchanged). Without the render the same fact cannot clear the bar.
func TestRenderExposureWithTrustedConsequenceRaisesAction(t *testing.T) {
	eb := newEdge()
	def := eb.change(upgrade.RuleValuesDefaultChanged, "Default of cleanup.image.tag changed", "cleanup.image.tag")
	a := domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectHelmValue, Product: "example", Path: "cleanup.image.tag"},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: ptr(`"1.28.5"`), After: ptr(`"1.29.0"`)},
		Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpValuesKey, Path: "cleanup.image.tag", State: domain.StateSet}},
		Consequence:   &domain.Consequence{Kind: domain.ConsequenceWorkloadFailure, ExposedClass: domain.ImpactActionRequired, Statement: "The new default tag is not on any mirror: the cleanup CronJob's image pull fails and it never runs."},
		Statement:     "The default cleanup image tag changed to one that is not mirrored.",
	}
	f := fact(t, a, domain.VerifiedHuman)
	en := condEnv(t, "", nil) // cleanup.image.tag is NOT set: the fact's own condition is False
	// baseline: no render — the fact clears nothing (not-affected, weaker than
	// the deterministic verdict), exactly today's composition
	base := buildEval(t, eb.edge, en, Input{Facts: []domain.VerifiedFact{f}})
	for _, x := range findingsOf(base, def.ID) {
		if x.Knowledge != nil {
			t.Fatalf("baseline must not carry a knowledge finding: %+v", x)
		}
	}
	// with the render attributing the default to the customer's upgrade, the
	// trusted consequence raises it to ACTION citing the rendered change
	rec := renderRecord("1.0.0 → 1.1.0: image example.org/tools/kubectl 1.28.5 → 1.29.0")
	unset := stubUnsetEval{r: ConditionResult{Value: True,
		Matches: []domain.ImpactMatch{{Kind: domain.MatchRenderedChange, Subject: "cleanup.image.tag → image changed", Evidence: []domain.EvidenceID{rec.ID}}},
		Records: []domain.Evidence{rec}}}
	rep := buildEval(t, eb.edge, en, Input{Facts: []domain.VerifiedFact{f}, Unset: unset})
	var action *domain.ImpactFinding
	for i, x := range findingsOf(rep, def.ID) {
		if x.Knowledge != nil && x.Classification == domain.ImpactActionRequired {
			action = &findingsOf(rep, def.ID)[i]
		}
	}
	if action == nil {
		t.Fatalf("render exposure + trusted consequence must yield an ACTION: %+v", findingsOf(rep, def.ID))
	}
	if action.Rule != domain.KnowledgeRulePrefix+"exposed" || action.Knowledge.Fact != f.ID {
		t.Errorf("knowledge finding: %s %+v", action.Rule, action.Knowledge)
	}
	found := false
	for _, m := range action.Matches {
		found = found || (m.Kind == domain.MatchRenderedChange && m.Evidence[0] == rec.ID)
	}
	if !found {
		t.Errorf("the ACTION must cite the rendered change on chain 2: %+v", action.Matches)
	}
}

// The renderExposure dispatch itself: image aggregation (any attributable
// repo → true, every referenced repo no-effect → false, one undecided repo →
// not at all) and the values guards (unmatched removed key, unset-key family
// with a match).
func TestRenderExposureDispatch(t *testing.T) {
	eb := newEdge()
	img := eb.change(upgrade.RuleImageTagsChanged, "cleanup image tag changed", "example.org/tools/kubectl", "example.org/other/tool")
	dir := t.TempDir()
	en := loadEnv(t, env.Inputs{ValuesFiles: []string{writeFile(t, dir, "values.yaml", "cleanup:\n  image:\n    repository: example.org/tools/kubectl\n    tag: \"1.28.5\"\n")}})
	rec := renderRecord("image changed")
	match := domain.ImpactMatch{Kind: domain.MatchRenderedChange, Subject: "example.org/tools/kubectl → image changed", Evidence: []domain.EvidenceID{rec.ID}}
	newBuilder := func(image ImageRenderEvaluator) *builder {
		return &builder{edge: eb.edge, env: en, image: image}
	}
	// every referenced repo attributable → true with the rendered matches
	b := newBuilder(stubImageEval{r: ImageResult{Outcome: ImageAttributable,
		Matches: []domain.ImpactMatch{match}, Records: []domain.Evidence{rec}}})
	r, ok := b.renderExposure(img)
	if !ok || r.Value != True || len(r.Matches) != 1 || r.Records[0].ID != rec.ID {
		t.Fatalf("attributable: ok=%v %+v", ok, r)
	}
	// every referenced repo no-effect → false with the render checks
	chk := noEffectCheck("example.org/tools/kubectl", rec)
	b = newBuilder(stubImageEval{r: ImageResult{Outcome: ImageNoEffect, Checks: []domain.ImpactCheck{chk}}})
	if r, ok = b.renderExposure(img); !ok || r.Value != False || len(r.Checks) != 1 {
		t.Fatalf("no-effect: ok=%v %+v", ok, r)
	}
	// one referenced repo undecided → the render decides the change not at all
	b = newBuilder(stubImageEval{r: ImageResult{Outcome: ImageUndecided, Needed: []string{"render failed"}}})
	if _, ok = b.renderExposure(img); ok {
		t.Fatal("an undecided repo must leave the change to the fact's condition")
	}
	// a values:removed key the customer does not set: not renderExposure's
	// question (the deterministic join decided values-unset)
	removed := eb.change(upgrade.RuleValuesRemoved, "Helm value docs.url removed", "docs.url")
	b2 := &builder{edge: eb.edge, env: en, set: stubSetEval{r: SetValuesResult{Outcome: SetValuesRejected}}}
	if _, ok := b2.renderExposure(removed); ok {
		t.Fatal("an unmatched removed key must not consult the set-values render")
	}
	// a change visible only at runtime (no render-verifiable family): the
	// render abstains and the knowledge/deterministic fallback decides
	runtime := eb.change(upgrade.RuleCompatChanged, "supported Kubernetes range narrowed", "1.29")
	b3 := &builder{edge: eb.edge, env: en, set: stubSetEval{r: SetValuesResult{Outcome: SetValuesRejected}}}
	if _, ok := b3.renderExposure(runtime); ok {
		t.Fatal("a runtime-visible change must fall back to the knowledge layer")
	}
}
