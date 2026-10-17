package validate

import (
	"sort"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Scoped pin rules and the effective object view, on the fictional product
// (harness_test.go): Widgets carry a role ("root" or "leaf") and a rotation
// policy whose default changes; a rule may say that only root Widgets are
// exposed. Nothing here is an evaluation case.

// gw renders a Widget with the given spec fields (sorted; values verbatim YAML).
func gw(name, ns string, spec map[string]string) string {
	s := "apiVersion: gizmo.example.io/v1\nkind: Widget\nmetadata:\n  name: " + name + "\n  namespace: " + ns + "\nspec:\n"
	keys := make([]string, 0, len(spec))
	for k := range spec {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s += "  " + k + ": " + spec[k] + "\n"
	}
	if len(spec) == 0 {
		s += "  size: 1\n"
	}
	return s
}

// scopedWidgetCRD is the Widget CRD; role carries a default when roleDefault is set.
func scopedWidgetCRD(roleDefault string) string {
	role := "{type: string}"
	if roleDefault != "" {
		role = "{type: string, default: " + roleDefault + "}"
	}
	return `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.gizmo.example.io
spec:
  group: gizmo.example.io
  names: {kind: Widget, plural: widgets}
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                role: ` + role + `
                rotation: {type: string}
                size: {type: integer}
                gadgetRef:
                  type: object
                  properties:
                    name: {type: string}
`
}

// overlayRender adds a version-independent overlay render: a kustomize
// target that does not deploy the product (not-applicable, from side only).
func (f *fx) overlayRender(target, rendered string) {
	dir := "derived/render/env/" + strings.NewReplacer(":", "-", "/", "-").Replace(target)
	fa := f.put(domain.ArtifactRenderedManifests, domain.OriginDerived, dir+"/from.yaml", rendered)
	f.pairs = append(f.pairs, domain.ManifestPair{Scope: domain.RenderEnvironment, Target: target, From: fa.ID, Status: domain.CollectionNotApplicable,
		Reason: "version-independent: the overlay neither pins nor deploys gizmo"})
}

// crd adds an upstream CRD artifact.
func (f *fx) crd(content string) {
	f.put(domain.ArtifactCRDDefinition, domain.OriginUpstream, "upstream/gizmo/v2.0.0/crds.yaml", content)
}

var rootRole = domain.Condition{Op: domain.OpField, Path: "spec.role", State: domain.StateEquals, Values: []string{`"root"`}}

func rotationUnset() domain.Condition {
	return domain.Condition{Op: domain.OpField, Path: "spec.rotation", State: domain.StateUnset}
}

// widgetScope is the exposure "some Widget such that all of ops".
func widgetScope(ops ...domain.Condition) domain.Condition {
	return domain.Condition{Op: domain.OpResource, Group: "gizmo.example.io", Kind: "Widget", Of: ops}
}

// scopedFact: rotation's default changes, but only Widgets of the scope are
// exposed (a behaviour change of root Widgets).
func scopedFact(id string, level domain.VerificationLevel, ev domain.Evidence, exposure domain.Condition) domain.VerifiedFact {
	return fact(id, level, domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectCRDField, Product: "gizmo", Group: "gizmo.example.io", Kind: "Widget", Path: "spec.rotation"},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"Never"`), After: str(`"Always"`)},
		Applicability: &domain.Applicability{Exposure: exposure},
		Consequence:   &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired},
		Statement:     "Widgets of the scope that leave spec.rotation unset now rotate always.",
	}, ev)
}

func rootScoped(id string, level domain.VerificationLevel, ev domain.Evidence) domain.VerifiedFact {
	return scopedFact(id, level, ev, widgetScope(rootRole, rotationUnset()))
}

const overlayTarget = "kustomize:apps/w/overlays/prod"

// baseAndPatch lays out a kustomize base (one root Widget, two leaf
// Widgets, none setting rotation) and an overlay whose patch sets the root
// Widget's rotation to rootRotation ("" = no patch).
func baseAndPatch(f *fx, rootRotation string) {
	f.file("apps/w/base/kustomization.yaml", "resources:\n  - widgets.yaml\n")
	f.file("apps/w/base/widgets.yaml", stream(
		gw("w-root", "apps", map[string]string{"role": "root"}),
		gw("w-a", "apps", map[string]string{"role": "leaf"}),
		gw("w-b", "apps", map[string]string{"role": "leaf"})))
	k := "resources:\n  - ../../base\n"
	if rootRotation != "" {
		k += "patches:\n  - path: rotation.yaml\n"
		f.file("apps/w/overlays/prod/rotation.yaml", gw("w-root", "apps", map[string]string{"rotation": rootRotation}))
	}
	f.file("apps/w/overlays/prod/kustomization.yaml", k)
}

// mergedRender is what `kustomize build` of baseAndPatch renders.
func mergedRender(rootRotation string) string {
	root := map[string]string{"role": "root"}
	if rootRotation != "" {
		root["rotation"] = rootRotation
	}
	return stream(gw("w-root", "apps", root),
		gw("w-a", "apps", map[string]string{"role": "leaf"}),
		gw("w-b", "apps", map[string]string{"role": "leaf"}))
}

// Positive control: a scoped rule, pinned by an overlay patch, clears — the
// leaf Widgets are provably out of scope, the root Widget is seen merged
// with its patch.
func TestPinScopedRulePinnedByOverlayPatchClears(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up))
	baseAndPatch(o.f, "Never")
	o.f.overlayRender(overlayTarget, mergedRender("Never"))
	o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
	r := o.run(t)
	wantOutcome(t, r, domain.ValidationPass, "vf-scoped")
	if r.Provenance.Rule != RulePinExposure || !r.ClearsBehaviour() {
		t.Fatalf("rule %s", r.Provenance.Rule)
	}
	p := r.Pin
	if p.Subject != "gizmo.example.io/v1/Widget/apps/w-root" || p.Path != "spec.rotation" || p.Value != `"Never"` || p.RenderedObject != "gizmo.example.io/v1/Widget/apps/w-root" {
		t.Fatalf("pin %+v", p)
	}
	for _, want := range []string{
		"in the rule's scope (Widget where spec.role equals \"root\" and spec.rotation unset)",
		"environment render " + overlayTarget + ", version-independent overlay",
		"Out of the rule's scope",
		"gizmo.example.io/v1/Widget/apps/w-a",
		"spec.role is leaf",
		"env/apps/w/base/widgets.yaml → " + overlayTarget,
		"env/apps/w/overlays/prod/rotation.yaml → " + overlayTarget,
	} {
		if !strings.Contains(r.Detail, want) {
			t.Fatalf("detail lacks %q:\n%s", want, r.Detail)
		}
	}
}

// The same environment against the fail-open alternatives: an unscoped rule
// (every Widget must pin) and the raw files on their own (the base's root
// Widget unpatched) must not clear.
func TestPinScopedControlsThatMustNotClear(t *testing.T) {
	t.Run("unscoped rule", func(t *testing.T) {
		o := newOverrideFx(t)
		o.withFact(scopedFact("vf-all", domain.VerifiedHuman, o.f.up, widgetScope(rotationUnset())))
		baseAndPatch(o.f, "Never")
		o.f.overlayRender(overlayTarget, mergedRender("Never"))
		o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
		r := o.run(t)
		wantOutcome(t, r, domain.ValidationNotRun, "gizmo.example.io/v1/Widget/apps/w-a")
		if r.Provenance.Rule != ReasonPinNotEstablished {
			t.Fatalf("rule %s", r.Provenance.Rule)
		}
	})
	t.Run("no render consumed the files", func(t *testing.T) {
		o := newOverrideFx(t)
		o.withFact(rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up))
		baseAndPatch(o.f, "Never")
		o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
		r := o.run(t)
		wantOutcome(t, r, domain.ValidationNotRun, "env/apps/w/base/widgets.yaml, no render consumed it): leaves spec.rotation unset")
	})
	t.Run("overlay render unavailable", func(t *testing.T) {
		o := newOverrideFx(t)
		o.withFact(rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up))
		baseAndPatch(o.f, "Never")
		o.f.naPair(overlayTarget) // not-applicable, but its render failed: no from side
		o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
		r := o.run(t)
		wantOutcome(t, r, domain.ValidationNotRun, "whose rendered objects are not available")
		if r.Provenance.Rule != ReasonPinApplicabilityUndecidable {
			t.Fatalf("rule %s:\n%s", r.Provenance.Rule, r.Detail)
		}
	})
}

// The known-affected counterpart: the scoped object leaves the setting
// unset in the effective view. The record names the object and the scope.
func TestPinScopedRuleAffectedObjectDoesNotClear(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up))
	baseAndPatch(o.f, "")
	o.f.overlayRender(overlayTarget, mergedRender(""))
	o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
	r := o.run(t)
	wantOutcome(t, r, domain.ValidationNotRun, "gizmo.example.io/v1/Widget/apps/w-root (environment render "+overlayTarget+", version-independent overlay): leaves spec.rotation unset")
	if !strings.Contains(r.Detail, "in the rule's scope (Widget where spec.role equals \"root\"") {
		t.Fatalf("the record must name the applicability condition: %s", r.Detail)
	}
	if r.Provenance.Rule != ReasonPinNotEstablished {
		t.Fatalf("rule %s", r.Provenance.Rule)
	}
}

// PROTOCOL §5 S3: every negative control leaves the item un-cleared, and the
// record says why. Each setup is the positive control's environment with
// one mutation of the knowledge (or of what the environment states).
func TestPinNegativeControls(t *testing.T) {
	env := func(o *overrideFx) {
		baseAndPatch(o.f, "Never")
		o.f.overlayRender(overlayTarget, mergedRender("Never"))
		o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
	}
	cases := []struct {
		name   string
		setup  func(o *overrideFx)
		rule   string
		detail string
	}{
		{"a: consensus level only", func(o *overrideFx) {
			o.withFact(rootScoped("vf-scoped", domain.VerifiedConsensus, o.f.up))
		}, ReasonPinNoRule, "never a pin rule"},
		{"a: proxy level only", func(o *overrideFx) {
			o.withFact(rootScoped("vf-scoped", domain.VerifiedProxy, o.f.up))
		}, ReasonPinNoRule, "never a pin rule"},
		{"a: weakest aspect below trusted", func(o *overrideFx) {
			f := rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up)
			for i := range f.Verification {
				if f.Verification[i].Aspect == domain.AspectApplicability {
					f.Verification[i].Level, f.Verification[i].Consensus = domain.VerifiedConsensus, domain.ConsensusCrossModel
				}
			}
			o.withFact(f)
		}, ReasonPinNoRule, "verified only at level consensus"},
		{"b: unverified rule (no verification recorded)", func(o *overrideFx) {
			f := rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up)
			f.Verification = nil
			o.withFact(f)
		}, ReasonPinNoRule, "never a pin rule"},
		{"c: release after the transition", func(o *overrideFx) {
			f := rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up)
			f.Release = "3.0.0"
			o.withFact(f)
		}, ReasonPinNoRule, "outside the transition 1.0.0 → 2.0.0"},
		{"c: release equal to the from version", func(o *overrideFx) {
			f := rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up)
			f.Release = "1.0.0"
			o.withFact(f)
		}, ReasonPinNoRule, "outside the transition"},
		{"c: no release stated", func(o *overrideFx) {
			f := rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up)
			f.Release = ""
			o.withFact(f)
		}, ReasonPinNoRule, "states no release"},
		{"c: another product", func(o *overrideFx) {
			f := rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up)
			f.Product = "doohickey"
			o.withFact(f)
		}, ReasonPinNoRule, "is about product doohickey"},
		{"d: wrong pinned value", func(o *overrideFx) {
			// the rule says only rotation Hourly pins; the environment sets Never
			o.withFact(fact("vf-value", domain.VerifiedHuman, domain.SemanticAssertion{Applicability: &domain.Applicability{
				Exposure: widgetScope(rootRole, domain.Condition{Op: domain.OpField, Path: "spec.rotation", State: domain.StateNotEquals, Values: []string{`"Hourly"`}})}}, o.f.up))
		}, ReasonPinNotEstablished, "not one of the pinning values \"Hourly\""},
		{"e: superseded", func(o *overrideFx) {
			f := rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up)
			f.Status = domain.FactSuperseded
			o.withFact(f)
		}, ReasonPinNoRule, "is superseded"},
		{"f: retracted", func(o *overrideFx) {
			f := rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up)
			f.Status = domain.FactRetracted
			o.withFact(f)
		}, ReasonPinNoRule, "is retracted"},
		{"g: scope not decidable on the environment", func(o *overrideFx) {
			o.withFact(scopedFact("vf-undecidable", domain.VerifiedHuman, o.f.up, widgetScope(
				domain.Condition{Op: domain.OpUndecidable, Reason: domain.UnknownRuntimeBehaviorGap, Needed: "whether the Widget is served at runtime"},
				rotationUnset())))
		}, ReasonPinApplicabilityUndecidable, "whether the Widget is served at runtime"},
		{"h: contradicting active fact on the same subject", func(o *overrideFx) {
			o.withFact(rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up))
			o.withFact(scopedFact("vf-other", domain.VerifiedConsensus, o.f.up, widgetScope(rotationUnset())))
		}, ReasonPinContradictingFacts, "vf-other"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := newOverrideFx(t)
			c.setup(o)
			env(o)
			r := o.run(t)
			wantOutcome(t, r, domain.ValidationNotRun, c.detail)
			if r.Pin != nil || r.ClearsBehaviour() {
				t.Fatalf("a negative control never clears: %+v", r)
			}
			if r.Provenance.Rule != c.rule {
				t.Fatalf("reason %s, want %s:\n%s", r.Provenance.Rule, c.rule, r.Detail)
			}
		})
	}
}

// Contradictions are about the same subject inside the transition: an
// identical restatement, a fact for another release, or an inactive one
// does not block the pin.
func TestPinNonContradictions(t *testing.T) {
	cases := map[string]func(o *overrideFx){
		"identical restatement": func(o *overrideFx) {
			f := rootScoped("vf-twin", domain.VerifiedConsensus, o.f.up)
			f.Assertion.Statement = "worded differently"
			o.withFact(f)
		},
		"other release": func(o *overrideFx) {
			f := scopedFact("vf-old", domain.VerifiedHuman, o.f.up, widgetScope(rotationUnset()))
			f.Release = "0.9.0"
			o.withFact(f)
		},
		"superseded other": func(o *overrideFx) {
			f := scopedFact("vf-gone", domain.VerifiedHuman, o.f.up, widgetScope(rotationUnset()))
			f.Status = domain.FactSuperseded
			o.withFact(f)
		},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			o := newOverrideFx(t)
			o.withFact(rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up))
			extra(o)
			baseAndPatch(o.f, "Never")
			o.f.overlayRender(overlayTarget, mergedRender("Never"))
			o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
			wantOutcome(t, o.run(t), domain.ValidationPass, "vf-scoped")
		})
	}
}

// A value predicate (equals / not-equals) on a scope field the object leaves
// unset is decided only by a schema default a CRD of the bundle states. A CRD
// that states no default does not decide it — the controller may default the
// field in code — so the object is undecidable; what an unset field means
// must be stated by the rule itself, with an explicit unset/set leaf.
func TestPinScopeUnsetFieldNeedsTheSchema(t *testing.T) {
	setup := func(o *overrideFx, exposure domain.Condition, plainRotation string) {
		o.withFact(scopedFact("vf-scoped", domain.VerifiedHuman, o.f.up, exposure))
		plain := map[string]string{"size": "2"} // role unset
		if plainRotation != "" {
			plain["rotation"] = plainRotation
		}
		o.f.file("apps/w/widgets.yaml", stream(
			gw("w-root", "apps", map[string]string{"role": "root", "rotation": "Never"}),
			gw("w-plain", "apps", plain)))
		o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
	}
	roleUnset := domain.Condition{Op: domain.OpField, Path: "spec.role", State: domain.StateUnset}
	roleSet := domain.Condition{Op: domain.OpField, Path: "spec.role", State: domain.StateSet}
	notLeaf := domain.Condition{Op: domain.OpField, Path: "spec.role", State: domain.StateNotEquals, Values: []string{`"leaf"`}}
	cases := []struct {
		name     string
		exposure domain.Condition
		crd      string // "" none; "-" a CRD stating no default; else the default
		inEnv    bool   // the CRD ships as an environment file
		plainRot string
		want     domain.ValidationOutcomeKind
		rule     string
		detail   string
	}{
		{"equals, no CRD", widgetScope(rootRole, rotationUnset()), "", false, "", domain.ValidationNotRun, ReasonPinApplicabilityUndecidable,
			"no CustomResourceDefinition of gizmo.example.io/Widget in the bundle"},
		// (1) the commander's fail-open: absence of a schema default is no proof
		{"equals, CRD states no default", widgetScope(rootRole, rotationUnset()), "-", false, "", domain.ValidationNotRun, ReasonPinApplicabilityUndecidable,
			"states no schema default for spec.role"},
		{"equals, environment CRD states no default", widgetScope(rootRole, rotationUnset()), "-", true, "", domain.ValidationNotRun, ReasonPinApplicabilityUndecidable,
			"a controller may default it in code"},
		// (2) not-equals likewise
		{"not-equals, CRD states no default", widgetScope(notLeaf, rotationUnset()), "-", false, "", domain.ValidationNotRun, ReasonPinApplicabilityUndecidable,
			"states no schema default for spec.role"},
		// a cited schema default decides it
		{"equals, CRD default puts it in scope", widgetScope(rootRole, rotationUnset()), "root", false, "", domain.ValidationNotRun, ReasonPinNotEstablished, "w-plain"},
		{"equals, CRD default keeps it out", widgetScope(rootRole, rotationUnset()), "leaf", false, "", domain.ValidationPass, "", "defaults to leaf"},
		{"CRDs disagree", widgetScope(rootRole, rotationUnset()), "disagree", false, "", domain.ValidationNotRun, ReasonPinApplicabilityUndecidable, "disagree"},
		// (3) the rule states what an unset role means: decided without any schema
		{"explicit: unset role is in scope (any{equals, unset}), not pinned", widgetScope(domain.Condition{Op: domain.OpAny, Of: []domain.Condition{rootRole, roleUnset}}, rotationUnset()),
			"", false, "", domain.ValidationNotRun, ReasonPinNotEstablished, "w-plain"},
		{"explicit: unset role is in scope (any{equals, unset}), pinned", widgetScope(domain.Condition{Op: domain.OpAny, Of: []domain.Condition{rootRole, roleUnset}}, rotationUnset()),
			"", false, "Never", domain.ValidationPass, "", ""},
		{"explicit: unset role is out of scope (all{not-equals, set})", widgetScope(notLeaf, roleSet, rotationUnset()),
			"", false, "", domain.ValidationPass, "", "spec.role is unset"},
		{"explicit: unset role is out of scope (all{equals, set})", widgetScope(rootRole, roleSet, rotationUnset()),
			"-", false, "", domain.ValidationPass, "", "spec.role is unset"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := newOverrideFx(t)
			setup(o, c.exposure, c.plainRot)
			switch {
			case c.crd == "-" && c.inEnv:
				o.f.file("crds/widgets.yaml", scopedWidgetCRD(""))
			case c.crd == "-":
				o.f.crd(scopedWidgetCRD(""))
			case c.crd == "disagree":
				o.f.crd(scopedWidgetCRD("root"))
				o.f.put(domain.ArtifactCRDDefinition, domain.OriginUpstream, "upstream/gizmo/v1.0.0/crds.yaml", scopedWidgetCRD("leaf"))
			case c.crd != "":
				o.f.crd(scopedWidgetCRD(c.crd))
			}
			r := o.run(t)
			wantOutcome(t, r, c.want, c.detail)
			if c.rule != "" && r.Provenance.Rule != c.rule {
				t.Fatalf("rule %s, want %s:\n%s", r.Provenance.Rule, c.rule, r.Detail)
			}
			if c.want != domain.ValidationPass && r.ClearsBehaviour() {
				t.Fatal("must not clear")
			}
		})
	}
}

// Scope operators: any, not and ref are decided per object; a reference
// that resolves to nothing is undecidable.
func TestPinScopeOperators(t *testing.T) {
	gadget := func(name, mode string) string {
		return "apiVersion: gizmo.example.io/v1\nkind: Gadget\nmetadata:\n  name: " + name + "\n  namespace: apps\nspec:\n  mode: " + mode + "\n"
	}
	strictGadget := domain.Condition{Op: domain.OpRef, Path: "spec.gadgetRef", Kind: "Gadget",
		Of: []domain.Condition{{Op: domain.OpField, Path: "spec.mode", State: domain.StateEquals, Values: []string{`"strict"`}}}}
	cases := []struct {
		name     string
		exposure domain.Condition
		objs     []string
		want     domain.ValidationOutcomeKind
		detail   string
	}{
		{"not excludes", widgetScope(domain.Condition{Op: domain.OpNot, Of: []domain.Condition{{Op: domain.OpField, Path: "spec.role", State: domain.StateEquals, Values: []string{`"leaf"`}}}}, rotationUnset()),
			[]string{gw("w-root", "apps", map[string]string{"role": "root", "rotation": "Never"}), gw("w-a", "apps", map[string]string{"role": "leaf"})},
			domain.ValidationPass, "not (spec.role is leaf)"},
		{"pin leaf under any: the other disjunct exposes", widgetScope(domain.Condition{Op: domain.OpAny, Of: []domain.Condition{rotationUnset(),
			{Op: domain.OpField, Path: "spec.size", State: domain.StateEquals, Values: []string{"9"}}}}),
			[]string{gw("w-big", "apps", map[string]string{"rotation": "Never", "size": "9"})},
			domain.ValidationNotRun, "in the rule's scope through another condition"},
		{"pin leaf under any: pinned", widgetScope(domain.Condition{Op: domain.OpAny, Of: []domain.Condition{rotationUnset(),
			{Op: domain.OpField, Path: "spec.size", State: domain.StateEquals, Values: []string{"9"}}}}),
			[]string{gw("w-small", "apps", map[string]string{"rotation": "Never", "size": "2"})},
			domain.ValidationPass, ""},
		{"ref to a non-strict gadget excludes", widgetScope(strictGadget, rotationUnset()),
			[]string{gw("w-r", "apps", map[string]string{"rotation": "Never", "gadgetRef": "{name: g1}"}),
				gw("w-x", "apps", map[string]string{"gadgetRef": "{name: g2}"}), gadget("g1", "strict"), gadget("g2", "lenient")},
			domain.ValidationPass, "spec.gadgetRef → gizmo.example.io/v1/Gadget/apps/g2"},
		{"ref to a strict gadget includes", widgetScope(strictGadget, rotationUnset()),
			[]string{gw("w-x", "apps", map[string]string{"gadgetRef": "{name: g1}"}), gw("w-r", "apps", map[string]string{"rotation": "Never"}), gadget("g1", "strict")},
			domain.ValidationNotRun, "w-x"},
		{"ref resolving to nothing is undecidable", widgetScope(strictGadget, rotationUnset()),
			[]string{gw("w-r", "apps", map[string]string{"rotation": "Never"}), gw("w-x", "apps", map[string]string{"gadgetRef": "{name: missing}"})},
			domain.ValidationNotRun, "absence is not knowledge"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := newOverrideFx(t)
			o.withFact(scopedFact("vf-op", domain.VerifiedHuman, o.f.up, c.exposure))
			o.f.file("apps/w/objects.yaml", stream(c.objs...))
			o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
			wantOutcome(t, o.run(t), c.want, c.detail)
		})
	}
}

// Every object out of scope is no pin: whether the environment is exposed
// at all is the join's decision.
func TestPinAllOutOfScopeIsNoPin(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up))
	o.f.file("apps/w/widgets.yaml", gw("w-a", "apps", map[string]string{"role": "leaf"}))
	o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
	wantOutcome(t, o.run(t), domain.ValidationNotRun, "outside the rule's scope: no setting pins anything")
}

// Objects of a render of the product are decided on their to side, with
// the from side for the value stated before the upgrade.
func TestPinScopedRenderedProductObjects(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up))
	leaf := gw("w-a", "gizmo", map[string]string{"role": "leaf"})
	o.f.pair("argocd:apps/gizmo.yaml",
		stream(controller(), gw("w-root", "gizmo", map[string]string{"role": "root", "rotation": "Never"}), leaf),
		stream(controller(), gw("w-root", "gizmo", map[string]string{"role": "root", "rotation": "Never"}), leaf))
	r := o.run(t)
	wantOutcome(t, r, domain.ValidationPass, "environment render argocd:apps/gizmo.yaml (to)")

	o = newOverrideFx(t)
	o.withFact(rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up))
	o.f.pair("argocd:apps/gizmo.yaml",
		stream(controller(), gw("w-root", "gizmo", map[string]string{"role": "root"}), leaf),
		stream(controller(), gw("w-root", "gizmo", map[string]string{"role": "root", "rotation": "Hourly"}), leaf))
	wantOutcome(t, o.run(t), domain.ValidationNotRun, "only after the upgrade")
}

func TestKustomizeConsumes(t *testing.T) {
	files := map[string][]byte{
		"env/a/base/kustomization.yaml":  []byte("resources:\n  - w.yaml\n  - https://example.invalid/remote.yaml\ncomponents:\n  - ../comp\n"),
		"env/a/base/w.yaml":              []byte("x: 1"),
		"env/a/comp/kustomization.yaml":  []byte("patchesStrategicMerge:\n  - p.yaml\n"),
		"env/a/comp/p.yaml":              []byte("x: 1"),
		"env/a/prod/kustomization.yml":   []byte("bases: [../base]\npatches:\n  - path: q.yaml\n  - patch: |-\n      inline\npatchesJson6902:\n  - path: j.yaml\n"),
		"env/a/prod/q.yaml":              []byte("x: 1"),
		"env/a/other/kustomization.yaml": []byte("resources: [x.yaml]\n"),
	}
	got, missing := kustomizeConsumes(files, "env/a/prod")
	want := []string{"env/a/base/kustomization.yaml", "env/a/base/w.yaml", "env/a/comp/kustomization.yaml", "env/a/comp/p.yaml", "env/a/prod/kustomization.yml", "env/a/prod/q.yaml"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("consumed %v, want %v", got, want)
	}
	if len(missing) != 1 || !strings.Contains(missing[0], "j.yaml") {
		t.Fatalf("missing %v", missing)
	}
	for target, want := range map[string]string{"kustomize:a/prod": "env/a/prod", "kustomize:a/prod#2": "env/a/prod", "kustomize:.": "env", "argocd:x": "", "kustomize:/abs": ""} {
		got, ok := kustomizeDir(target)
		if (want == "") == ok || got != want {
			t.Fatalf("kustomizeDir(%s) = %q, %v", target, got, ok)
		}
	}
}

func TestDefaultAt(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{
		"spec": map[string]any{"type": "object", "properties": map[string]any{
			"a":    map[string]any{"type": "boolean", "default": false},
			"b":    map[string]any{"type": "string"},
			"obj":  map[string]any{"type": "object", "default": map[string]any{}, "properties": map[string]any{"c": map[string]any{"type": "string"}}},
			"list": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"d": map[string]any{"type": "string", "default": "x"}}}},
			"free": map[string]any{"type": "object", "x-kubernetes-preserve-unknown-fields": true},
		}},
	}}
	for _, c := range []struct {
		path, val string
		has       bool
		why       string
	}{
		{"spec.a", "false", true, ""},
		{"spec.b", "", false, ""},
		{"spec.obj.c", "", false, "ancestor"},
		{"spec.list[].d", `"x"`, true, ""},
		{"spec.free.e", "", false, "preserves unknown fields"},
		{"spec.nope", "", false, "does not describe"},
	} {
		v, has, why := defaultAt(schema, c.path)
		if v != c.val || has != c.has || (c.why == "") != (why == "") || !strings.Contains(why, c.why) {
			t.Fatalf("%s: %q %v %q", c.path, v, has, why)
		}
	}
}

// A fact's applicability decides who is exposed: its default change alone
// never pins when the exposure names no setting that falsifies it, and an
// exposure leaf on the subject's own setting accepts the old default.
func TestPinDefaultChangeDefersToApplicability(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(scopedFact("vf-runtime", domain.VerifiedHuman, o.f.up, domain.Condition{Op: domain.OpUndecidable,
		Reason: domain.UnknownRuntimeBehaviorGap, Needed: "whether Widgets are reconciled"}))
	o.f.file("apps/w/widgets.yaml", gw("w1", "apps", map[string]string{"rotation": "Never"}))
	o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
	r := o.run(t)
	wantOutcome(t, r, domain.ValidationNotRun, "its default change alone is no pin")
	if r.Provenance.Rule != ReasonPinNoRule {
		t.Fatalf("rule %s", r.Provenance.Rule)
	}

	o = newOverrideFx(t)
	o.withFact(rootScoped("vf-scoped", domain.VerifiedHuman, o.f.up))
	o.f.pair("argocd:apps/gizmo.yaml",
		stream(controller(), gw("w-root", "gizmo", map[string]string{"role": "root"})),
		stream(controller(), gw("w-root", "gizmo", map[string]string{"role": "root", "rotation": "Never"})))
	r = o.run(t)
	wantOutcome(t, r, domain.ValidationPass, "the old default")
	if r.Provenance.Rule != RulePinExposure {
		t.Fatalf("rule %s", r.Provenance.Rule)
	}
}
