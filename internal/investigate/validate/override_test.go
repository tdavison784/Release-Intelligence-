package validate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

func str(s string) *string { return &s }

// fact builds a verified fact of the fictional product at one level.
func fact(id string, level domain.VerificationLevel, a domain.SemanticAssertion, ev domain.Evidence) domain.VerifiedFact {
	f := domain.VerifiedFact{ID: id, Product: "gizmo", Release: "2.0.0", Assertion: a, Evidence: []domain.Evidence{ev}, Status: domain.FactActive, CreatedAt: testNow}
	for _, x := range domain.Aspects {
		av := domain.AspectVerification{Aspect: x, Level: level, Basis: []string{"val-test"}}
		if level == domain.VerifiedConsensus {
			av.Consensus = domain.ConsensusCrossModel
		}
		f.Verification = append(f.Verification, av)
	}
	return f
}

func flagDefaultFact(id string, level domain.VerificationLevel, ev domain.Evidence) domain.VerifiedFact {
	return fact(id, level, domain.SemanticAssertion{
		Subject:   &domain.Subject{Family: domain.SubjectCLIFlag, Product: "gizmo", Name: "--mode", Component: "controller"},
		Change:    &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"safe"`), After: str(`"fast"`)},
		Statement: "The controller's --mode default changes from safe to fast.",
	}, ev)
}

func widgetDefaultFact(id string, ev domain.Evidence) domain.VerifiedFact {
	return fact(id, domain.VerifiedDeterministic, domain.SemanticAssertion{
		Subject:   &domain.Subject{Family: domain.SubjectCRDField, Product: "gizmo", Group: "gizmo.example.io", Kind: "Widget", Path: "spec.rotation"},
		Change:    &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"Never"`), After: str(`"Always"`)},
		Statement: "Widgets that leave spec.rotation unset now rotate always.",
	}, ev)
}

// overrideFx: a behaviour item whose fact sits in the bundle.
type overrideFx struct {
	f    *fx
	item string
}

func newOverrideFx(t *testing.T) *overrideFx {
	f := newFx(t)
	item := f.change(domain.Change{ID: "chg-mode", Category: domain.CategoryConfiguration, Breaking: true, Title: "controller default mode becomes fast",
		Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize@v1", Rule: "section:breaking", Confidence: domain.ConfidenceHigh}}, behaviour)
	return &overrideFx{f: f, item: item}
}

func (o *overrideFx) withFact(f domain.VerifiedFact) {
	data, _ := json.Marshal(map[string]any{"schemaVersion": "ri.dev/knowledge/v1alpha1", "kind": "fact", "fact": f})
	o.f.put(domain.ArtifactKnowledgeFact, domain.OriginUpstream, "upstream/gizmo/v2.0.0/facts/"+f.ID+".json", string(data), o.item)
}

func (o *overrideFx) run(t *testing.T, opts ...Option) domain.ValidationRecord {
	t.Helper()
	b := o.f.bundle()
	opts = append([]Option{WithStore(o.f.st), WithClock(clock)}, opts...)
	return one(t, run(t, OverrideInspection(opts...), investigate.ValidationInput{Bundle: b, ChangeItem: o.item}))
}

func TestOverrideNoRuleIsNotRun(t *testing.T) {
	o := newOverrideFx(t)
	o.f.pair("argocd:apps/gizmo.yaml", controller("--mode=safe"), controller("--mode=safe"))
	r := o.run(t)
	wantOutcome(t, r, domain.ValidationNotRun, "no declarative pin rule")
	if r.Pin != nil {
		t.Fatal("no rule ⇒ no pin, even though the render sets the flag")
	}
}

func TestOverrideFlagPinnedToOldDefault(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(flagDefaultFact("vf-flag", domain.VerifiedDeterministic, o.f.up))
	o.f.pair("argocd:apps/gizmo.yaml", controller(), controller("--mode=safe"))
	r := o.run(t)
	wantOutcome(t, r, domain.ValidationPass, "the old default")
	p := r.Pin
	if p == nil || p.Kind != domain.PinFlag || p.Path != "--mode" || p.Value != `"safe"` || p.RenderedObject != deployKey || p.Subject != deployKey {
		t.Fatalf("pin %+v", p)
	}
	if !r.ClearsBehaviour() || r.Provenance.Rule != RulePinDefaultChanged {
		t.Fatalf("a pinned flag clears a behaviour item: %+v", r)
	}
	if !strings.Contains(r.Detail, "vf-flag") || !strings.Contains(r.Detail, "lands in the environment render") {
		t.Fatalf("detail must cite the rule source and the landing: %s", r.Detail)
	}
}

func TestOverrideFlagSameValueBothSides(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(flagDefaultFact("vf-flag", domain.VerifiedHuman, o.f.up))
	o.f.pair("argocd:apps/gizmo.yaml", controller("--mode=custom"), controller("--mode=custom"))
	wantOutcome(t, o.run(t), domain.ValidationPass, "both sides")
}

func TestOverrideFactViaOption(t *testing.T) {
	o := newOverrideFx(t)
	o.f.pair("argocd:apps/gizmo.yaml", controller("--mode=safe"), controller("--mode=safe"))
	b := o.f.bundle()
	fv := flagDefaultFact("vf-opt", domain.VerifiedDeterministic, o.f.up)
	rep := &domain.ImpactReport{Findings: []domain.ImpactFinding{{ID: "imp-k", ChangeID: "chg-mode", Classification: domain.ImpactReviewRequired,
		Rule: "impact:knowledge-exposed", Knowledge: &domain.KnowledgeRef{Fact: "vf-opt", Verification: domain.VerifiedDeterministic}}}}
	in := investigate.ValidationInput{Bundle: b, ChangeItem: o.item, Report: rep}
	r := one(t, run(t, OverrideInspection(WithStore(o.f.st), WithFacts(fv)), in))
	wantOutcome(t, r, domain.ValidationPass, "vf-opt")
}

func TestOverrideWidgetFieldsInEnvironmentFiles(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(widgetDefaultFact("vf-widget", o.f.up))
	o.f.file("apps/a/widgets.yaml", stream(widget("w1", "apps", "Never"), widget("w2", "apps", "Hourly")))
	o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
	r := o.run(t)
	wantOutcome(t, r, domain.ValidationPass, "on every gizmo.example.io Widget")
	p := r.Pin
	if p.Kind != domain.PinField || p.Path != "spec.rotation" || p.Subject != "gizmo.example.io/v1/Widget/apps/w1" || p.Value != `"Never"` || len(p.Evidence) != 2 {
		t.Fatalf("pin %+v", p)
	}
	if p.RenderedObject != "" {
		t.Fatalf("the widgets are not in the product render: %+v", p)
	}
}

// A setting that is merely absent — on some or all objects — is never a pass.
func TestOverrideAbsentSettingNeverPasses(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(o *overrideFx)
		detail string
	}{
		{"flag absent", func(o *overrideFx) {
			o.withFact(flagDefaultFact("vf-flag", domain.VerifiedDeterministic, o.f.up))
			o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
		}, "does not set"},
		{"flag set to the new default only after", func(o *overrideFx) {
			o.withFact(flagDefaultFact("vf-flag", domain.VerifiedDeterministic, o.f.up))
			o.f.pair("argocd:apps/gizmo.yaml", controller(), controller("--mode=fast"))
		}, "behaviour changes"},
		{"flag changed across the upgrade", func(o *overrideFx) {
			o.withFact(flagDefaultFact("vf-flag", domain.VerifiedDeterministic, o.f.up))
			o.f.pair("argocd:apps/gizmo.yaml", controller("--mode=custom"), controller("--mode=other"))
		}, "before it"},
		{"one install of two lacks it", func(o *overrideFx) {
			o.withFact(flagDefaultFact("vf-flag", domain.VerifiedDeterministic, o.f.up))
			o.f.pair("argocd:apps/a.yaml", controller("--mode=safe"), controller("--mode=safe"))
			o.f.pair("argocd:apps/b.yaml", controller(), controller())
		}, "apps/b.yaml"},
		{"one widget leaves it unset", func(o *overrideFx) {
			o.withFact(widgetDefaultFact("vf-widget", o.f.up))
			o.f.file("apps/a/widgets.yaml", stream(widget("w1", "apps", "Never"), widget("w2", "apps", "")))
			o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
		}, "w2"},
		{"no widget visible", func(o *overrideFx) {
			o.withFact(widgetDefaultFact("vf-widget", o.f.up))
			o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
		}, "absent is never a pin"},
		{"unparsed file may declare widgets", func(o *overrideFx) {
			o.withFact(widgetDefaultFact("vf-widget", o.f.up))
			o.f.file("apps/a/widgets.yaml", widget("w1", "apps", "Never"))
			o.f.file("charts/x/templates/w.yaml", "apiVersion: gizmo.example.io/v1\nkind: Widget\nmetadata:\n  name: {{ .Values.name }\n  labels: [a\n")
			o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
		}, "could not be parsed"},
		{"environment input partial", func(o *overrideFx) {
			o.withFact(widgetDefaultFact("vf-widget", o.f.up))
			o.f.file("apps/a/widgets.yaml", widget("w1", "apps", "Never"))
			o.f.inputs = append(o.f.inputs, domain.SnapshotInput{Kind: domain.InputManifests, Path: "./extra", Status: domain.CollectionPartial, Reason: "2 documents unparsable"})
			o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
		}, "not collected completely"},
		{"render incomplete", func(o *overrideFx) {
			o.withFact(flagDefaultFact("vf-flag", domain.VerifiedDeterministic, o.f.up))
			o.f.pair("argocd:apps/a.yaml", controller("--mode=safe"), controller("--mode=safe"))
			o.f.brokenPair("argocd:apps/b.yaml", "chart-unavailable")
		}, "needs every environment render complete"},
		{"no render", func(o *overrideFx) {
			o.withFact(widgetDefaultFact("vf-widget", o.f.up))
			o.f.file("apps/a/widgets.yaml", widget("w1", "apps", "Never"))
		}, "no environment render"},
		{"proxy fact", func(o *overrideFx) {
			o.withFact(flagDefaultFact("vf-flag", domain.VerifiedProxy, o.f.up))
			o.f.pair("argocd:apps/gizmo.yaml", controller("--mode=safe"), controller("--mode=safe"))
		}, "never a pin rule"},
		{"consensus fact", func(o *overrideFx) {
			o.withFact(flagDefaultFact("vf-flag", domain.VerifiedConsensus, o.f.up))
			o.f.pair("argocd:apps/gizmo.yaml", controller("--mode=safe"), controller("--mode=safe"))
		}, "never a pin rule"},
		{"retracted fact", func(o *overrideFx) {
			fv := flagDefaultFact("vf-flag", domain.VerifiedDeterministic, o.f.up)
			fv.Status = domain.FactRetracted
			o.withFact(fv)
			o.f.pair("argocd:apps/gizmo.yaml", controller("--mode=safe"), controller("--mode=safe"))
		}, "retracted"},
		{"values subject", func(o *overrideFx) {
			o.withFact(fact("vf-values", domain.VerifiedDeterministic, domain.SemanticAssertion{
				Subject: &domain.Subject{Family: domain.SubjectHelmValue, Product: "gizmo", Path: "controller.mode"},
				Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"safe"`), After: str(`"fast"`)}}, o.f.up))
			o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
		}, "values default change"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := newOverrideFx(t)
			c.setup(o)
			r := o.run(t)
			wantOutcome(t, r, domain.ValidationNotRun, c.detail)
			if r.Pin != nil || r.ClearsBehaviour() {
				t.Fatalf("an undecided inspection never carries a pin: %+v", r)
			}
		})
	}
}

func TestOverrideNoStoreIsNotRun(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(flagDefaultFact("vf-flag", domain.VerifiedDeterministic, o.f.up))
	o.f.pair("argocd:apps/gizmo.yaml", controller("--mode=safe"), controller("--mode=safe"))
	b := o.f.bundle()
	r := one(t, run(t, OverrideInspection(), investigate.ValidationInput{Bundle: b, ChangeItem: o.item}))
	wantOutcome(t, r, domain.ValidationNotRun, "no artifact store")
}

func TestOverrideExposureDerivedConfigKey(t *testing.T) {
	exposure := domain.Condition{Op: domain.OpResource, Kind: "ConfigMap", Name: "gizmo-config",
		Of: []domain.Condition{{Op: domain.OpField, Path: `data["sync.mode"]`, State: domain.StateUnset}}}
	for _, c := range []struct {
		name       string
		from, to   string
		want       domain.ValidationOutcomeKind
		wantDetail string
	}{
		{"set on both sides", "strict", "strict", domain.ValidationPass, "config-key sync.mode of ConfigMap gizmo-config"},
		{"set only after", "", "strict", domain.ValidationNotRun, "only after the upgrade"},
	} {
		t.Run(c.name, func(t *testing.T) {
			o := newOverrideFx(t)
			o.withFact(fact("vf-cm", domain.VerifiedDeterministic, domain.SemanticAssertion{
				Applicability: &domain.Applicability{Exposure: exposure}, Statement: "sync.mode unset now means lenient"}, o.f.up))
			cm := func(v string) string {
				if v == "" {
					return configMap("gizmo-config", "gizmo", map[string]string{"other": "x"})
				}
				return configMap("gizmo-config", "gizmo", map[string]string{"sync.mode": v})
			}
			o.f.pair("argocd:apps/gizmo.yaml", stream(controller(), cm(c.from)), stream(controller(), cm(c.to)))
			r := o.run(t)
			wantOutcome(t, r, c.want, c.wantDetail)
			if c.want == domain.ValidationPass {
				if r.Pin.Kind != domain.PinConfigKey || r.Pin.Path != "sync.mode" || r.Pin.RenderedObject != "core/v1/ConfigMap/gizmo/gizmo-config" || r.Provenance.Rule != RulePinExposure {
					t.Fatalf("pin %+v", r.Pin)
				}
			}
		})
	}
}

func TestOverrideExposureNotEquals(t *testing.T) {
	exposure := domain.Condition{Op: domain.OpAll, Of: []domain.Condition{
		{Op: domain.OpCLIFlag, Name: "--tls-min", Component: "controller", State: domain.StateNotEquals, Values: []string{`"1.2"`}},
	}}
	for _, c := range []struct {
		val  string
		want domain.ValidationOutcomeKind
	}{{"1.2", domain.ValidationPass}, {"1.0", domain.ValidationNotRun}} {
		t.Run(c.val, func(t *testing.T) {
			o := newOverrideFx(t)
			o.withFact(fact("vf-tls", domain.VerifiedDeterministic, domain.SemanticAssertion{Applicability: &domain.Applicability{Exposure: exposure}}, o.f.up))
			o.f.pair("argocd:apps/gizmo.yaml", controller("--tls-min="+c.val), controller("--tls-min="+c.val))
			wantOutcome(t, o.run(t), c.want, "")
		})
	}
}

// --- the join's pinned verdicts ------------------------------------------------

func joinPinnedFx(t *testing.T, extra ...domain.ImpactFinding) (*fx, string, *domain.ImpactReport) {
	f := newFx(t)
	item := f.change(domain.Change{ID: "chg-crd", Category: domain.CategoryCRDSchema, Title: "Widget v1 schema: 1 default changed: `spec.rotation`",
		Detail:   "Schema defaults changed in the gizmo.example.io/v1 schema of widgets.gizmo.example.io (old → new; …):\nspec.rotation: \"Never\" → \"Always\"",
		Subjects: []string{"spec.rotation"}, Provenance: domain.Provenance{Rule: "crd:default-changed"}},
		func(it *domain.ChangeItem) {
			it.Family, it.Manifestation, it.ManifestationRule = domain.FamilyAPISchema, domain.ManifestControllerBehaviour, "manifest:crd-default"
		})
	envEv := domain.NewEvidence(domain.EvidenceLocalFile, "", "apps/a/widgets.yaml", "L8", "  rotation: Never", domain.Digest([]byte("w")), testNow)
	rep := &domain.ImpactReport{SchemaVersion: domain.ImpactReportSchemaVersion, Evidence: []domain.Evidence{f.up}, EnvironmentEvidence: []domain.Evidence{envEv},
		Findings: append([]domain.ImpactFinding{{ID: "imp-pin", ChangeID: "chg-crd", Classification: domain.ImpactInformational, Rule: joinCRDDefaultPinned,
			Title: "You set spec.rotation explicitly", UpstreamEvidence: []domain.EvidenceID{f.up.ID}, EnvironmentEvidence: []domain.EvidenceID{envEv.ID},
			Matches: []domain.ImpactMatch{
				{Kind: domain.MatchAPIVersion, Subject: "gizmo.example.io/v1 Widget apps/w1", Evidence: []domain.EvidenceID{envEv.ID}},
				{Kind: domain.MatchManifestField, Subject: "gizmo.example.io/v1 Widget apps/w1: spec.rotation", Evidence: []domain.EvidenceID{envEv.ID}},
			}, Provenance: impactProv()}}, extra...)}
	return f, item, rep
}

func TestOverrideJoinPinned(t *testing.T) {
	f, item, rep := joinPinnedFx(t)
	b := f.bundle()
	r := one(t, run(t, OverrideInspection(WithClock(clock)), investigate.ValidationInput{Bundle: b, ChangeItem: item, Report: rep}))
	wantOutcome(t, r, domain.ValidationPass, RulePinJoin)
	p := r.Pin
	if p.Kind != domain.PinField || p.Subject != "gizmo.example.io/v1/Widget/apps/w1" || p.Path != "spec.rotation" || p.Value != `"Never"` || r.ImpactFinding != "imp-pin" {
		t.Fatalf("pin %+v (finding %s)", p, r.ImpactFinding)
	}
	if !r.ClearsBehaviour() {
		t.Fatal("the join's pinned verdict on a CRD default clears the behaviour item")
	}
}

func TestOverrideJoinPinnedOnlyInPart(t *testing.T) {
	f, item, rep := joinPinnedFx(t, domain.ImpactFinding{ID: "imp-applies", ChangeID: "chg-crd", Classification: domain.ImpactReviewRequired,
		Rule: "impact:crd-default-applies", Title: "applies", UpstreamEvidence: []domain.EvidenceID{"x"}, Provenance: impactProv()})
	b := f.bundle()
	r := one(t, run(t, OverrideInspection(), investigate.ValidationInput{Bundle: b, ChangeItem: item, Report: rep}))
	wantOutcome(t, r, domain.ValidationNotRun, "pinned only in part")
}

func TestOverrideJoinValuesPinnedNeedsEveryKey(t *testing.T) {
	f := newFx(t)
	item := f.change(domain.Change{ID: "chg-v", Category: domain.CategoryHelmValues, Title: "defaults changed", Subjects: []string{"a.x", "a.y"},
		Provenance: domain.Provenance{Rule: "values:default-changed"}})
	envEv := domain.NewEvidence(domain.EvidenceLocalFile, "", "apps/gizmo/values.yaml", "L2", "x: 1", domain.Digest([]byte("v")), testNow)
	rep := &domain.ImpactReport{EnvironmentEvidence: []domain.Evidence{envEv}, Evidence: []domain.Evidence{f.up},
		Findings: []domain.ImpactFinding{{ID: "imp-v", ChangeID: "chg-v", Classification: domain.ImpactInformational, Rule: joinValuesPinned,
			UpstreamEvidence: []domain.EvidenceID{f.up.ID}, Matches: []domain.ImpactMatch{{Kind: domain.MatchValuesKey, Subject: "a.x", Evidence: []domain.EvidenceID{envEv.ID}}},
			Provenance: impactProv()}}}
	b := f.bundle()
	in := investigate.ValidationInput{Bundle: b, ChangeItem: item, Report: rep}
	r := one(t, run(t, OverrideInspection(), in))
	wantOutcome(t, r, domain.ValidationNotRun, "not pinned: a.y")

	rep.Findings[0].Matches = append(rep.Findings[0].Matches, domain.ImpactMatch{Kind: domain.MatchValuesKey, Subject: "a.y", Evidence: []domain.EvidenceID{envEv.ID}})
	r = one(t, run(t, OverrideInspection(), in))
	wantOutcome(t, r, domain.ValidationPass, "")
	if r.Pin.Kind != domain.PinValuesKey || r.Pin.Subject != "apps/gizmo/values.yaml" || r.Pin.Value != "1" {
		t.Fatalf("pin %+v", r.Pin)
	}
}

func TestOverrideRequestAnswersHypothesis(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(flagDefaultFact("vf-flag", domain.VerifiedDeterministic, o.f.up))
	o.f.pair("argocd:apps/gizmo.yaml", controller("--mode=safe"), controller("--mode=safe"))
	b := o.f.bundle()
	at := testNow
	hyp := domain.AgentRecordID(domain.HypothesisIDPrefix, o.item, "a hypothesis", "call-1")
	req := &domain.ValidationRequest{ChangeItem: o.item, Kind: domain.ValidationOverrideInspection, Hypothesis: hyp,
		Status: domain.RequestFulfilled, Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "test.agent@v1", Confidence: domain.ConfidenceLow,
			Model: "m", ModelVersion: "m", PromptVersion: "p/v1", PromptDigest: domain.Digest([]byte("p")), InputEvidence: []domain.EvidenceID{o.f.up.ID},
			GeneratedAt: &at, CallID: "call-1"}}
	req.ID = domain.AgentRecordID(domain.ValidationRequestIDPrefix, o.item, string(req.Kind)+"\x00", "call-1")
	out, err := OverrideInspection(WithStore(o.f.st)).Validate(t.Context(), investigate.ValidationInput{Bundle: b, ChangeItem: o.item, Request: req})
	if err != nil {
		t.Fatal(err)
	}
	r := one(t, out)
	if r.Request != req.ID || len(r.Answers) != 1 || r.Answers[0] != hyp {
		t.Fatalf("a requested validation names its request and the hypothesis it answers: %+v", r)
	}
	req.Result = r.ID
	conform(t, investigate.ValidationInput{Bundle: b, ChangeItem: o.item, Request: req}, out)
}
