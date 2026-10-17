package validate

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/redaction"
)

// Withheld values (redaction markers) in the effective view, on the
// fictional product: a Widget's settings below the credential-named map
// spec.credentials are stored redacted by the collector; a value the
// Widget CRD declares as an enum value is read back, nothing else is.

// widgetCredsCRD: rotation is an enum; tier is one when tierEnum.
func widgetCredsCRD(tierEnum bool) string {
	tier := "{type: string}"
	if tierEnum {
		tier = "{type: string, enum: [root, leaf]}"
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
                credentials:
                  type: object
                  properties:
                    rotation: {type: string, enum: [Never, Always]}
                    tier: ` + tier + `
`
}

// credWidget is a Widget as the collector stores it: credentials.* strings
// replaced by their markers.
func credWidget(name string, creds map[string]string) string {
	s := "apiVersion: gizmo.example.io/v1\nkind: Widget\nmetadata:\n  name: " + name + "\n  namespace: apps\nspec:\n  size: 1\n"
	if len(creds) > 0 {
		s += "  credentials:\n"
		for _, k := range []string{"rotation", "tier"} {
			if v, ok := creds[k]; ok {
				s += "    " + k + ": \"" + redaction.Marker(v) + "\"\n"
			}
		}
	}
	return s
}

var credRotation = "spec.credentials.rotation"

// credFact: spec.credentials.rotation's default changes Never → Always for
// Widgets of the scope (and the rotation unset).
func credFact(scope ...domain.Condition) domain.VerifiedFact {
	ops := append(scope, domain.Condition{Op: domain.OpField, Path: credRotation, State: domain.StateUnset})
	return fact("vf-cred", domain.VerifiedHuman, domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectCRDField, Product: "gizmo", Group: "gizmo.example.io", Kind: "Widget", Path: credRotation},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"Never"`), After: str(`"Always"`)},
		Applicability: &domain.Applicability{Exposure: widgetScope(ops...)},
		Consequence:   &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired},
		Statement:     "Widgets that leave spec.credentials.rotation unset now rotate always.",
	}, domain.Evidence{})
}

func credFx(t *testing.T, crd string, f domain.VerifiedFact, widgets ...string) domain.ValidationRecord {
	o := newOverrideFx(t)
	f.Evidence = []domain.Evidence{o.f.up}
	o.withFact(f)
	if crd != "" {
		o.f.crd(crd)
	}
	o.f.file("apps/w/widgets.yaml", stream(widgets...))
	o.f.pair("argocd:apps/gizmo.yaml", controller(), controller())
	return o.run(t)
}

// Positive control: the explicit setting, stored redacted, is read back
// from the CRD's enum and pins the old default — the rule applies.
func TestPinReadsBackWithheldEnumSetting(t *testing.T) {
	r := credFx(t, widgetCredsCRD(true), credFact(), credWidget("w", map[string]string{"rotation": "Never"}))
	wantOutcome(t, r, domain.ValidationPass, "stored redacted as "+redaction.Marker("Never"))
	if r.Pin == nil || r.Pin.Path != credRotation || r.Pin.Value != `"Never"` {
		t.Fatalf("pin %+v", r.Pin)
	}
}

// A scope condition on a withheld value: read back from the CRD's enum it
// decides (w-b is provably out of scope); without the enum, or with a value
// the enum does not declare, it stays withheld and is undecidable — the
// marker is not the value, so the object is never "provably out of scope".
// Comparing the marker as a literal would exclude w-b and clear.
func TestPinScopeOnWithheldValue(t *testing.T) {
	tierRoot := domain.Condition{Op: domain.OpField, Path: "spec.credentials.tier", State: domain.StateEquals, Values: []string{`"root"`}}
	widgets := func(bTier string) []string {
		return []string{
			credWidget("w-a", map[string]string{"rotation": "Never", "tier": "root"}),
			credWidget("w-b", map[string]string{"tier": bTier}),
		}
	}
	t.Run("read back", func(t *testing.T) {
		r := credFx(t, widgetCredsCRD(true), credFact(tierRoot), widgets("leaf")...)
		wantOutcome(t, r, domain.ValidationPass, "spec.credentials.tier is leaf (stored redacted as "+redaction.Marker("leaf"))
	})
	for name, c := range map[string]struct {
		enum bool
		tier string
	}{"no enum": {false, "leaf"}, "outside the enum": {true, "branch"}} {
		t.Run(name, func(t *testing.T) {
			r := credFx(t, widgetCredsCRD(c.enum), credFact(tierRoot), widgets(c.tier)...)
			if r.Outcome == domain.ValidationPass || r.ClearsBehaviour() {
				t.Fatalf("cleared although w-b's scope is withheld:\n%s", r.Detail)
			}
			for _, want := range []string{"gizmo.example.io/v1/Widget/apps/w-b", "spec.credentials.tier is withheld (redacted as " + redaction.Marker(c.tier)} {
				if !strings.Contains(r.Detail, want) {
					t.Fatalf("detail lacks %q:\n%s", want, r.Detail)
				}
			}
		})
	}
}
