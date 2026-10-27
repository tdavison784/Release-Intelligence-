package domain

import (
	"strings"
	"testing"
)

// DESIGN.md §1.3 amendment (2026-10-16): rendered changes nothing explains
// become scope:rendered-unexplained items; self-artifact version bumps
// become scope:routine-self-artifact items. Environment-scope ones live in
// EvidenceBundle.EnvironmentItems.

// envItemFixture adds an environment-only rendered change anchored by an
// environment item to the fixture bundle.
func envItemFixture(t *testing.T, scope string) (*invFixture, *ChangeItem) {
	t.Helper()
	fx := newInvFixture(t)
	b := fx.b
	p := &b.Renders[0]
	rc := RenderedChange{Class: "field-removed", Object: "core/v1/ConfigMap/cert-manager/cm", Path: "data.x", Before: `"1"`,
		Evidence: p.Changes[0].Evidence}
	if scope == ScopeDocumentation {
		rc = RenderedChange{Class: RenderedClassDocumentation, Object: "apiextensions.k8s.io/v1/CustomResourceDefinition//widgets.example.io",
			Path: "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.description", Before: `"a"`, After: `"b"`,
			Evidence: p.Changes[0].Evidence}
	}
	rc.ID = RenderedChangeID(RenderEnvironment, p.Target, rc.Object, rc.Path, rc.Class)
	it := ChangeItem{RenderedChange: rc.ID, Family: FamilyRendered, Title: "field-removed ConfigMap/cm data.x (not explained by any change)",
		Evidence: rc.Evidence, Manifestation: ManifestRenderedConfig, ManifestationRule: "manifest:rendered-anchor",
		InScope: scope == ScopeRenderedUnexplained, ScopeRule: scope}
	it.ID = ChangeItemID("", "", rc.ID)
	rc.ChangeItems, rc.Correlation = []string{it.ID}, "rendered-anchor"
	p.Changes = append(p.Changes, rc)
	sortRendered(p.Changes)
	b.EnvironmentItems = []ChangeItem{it}
	b.ID = BundleID(b.Change.ID, b.Snapshot.ID, b.Manifest.Digest(), b.Renders)
	return fx, &b.EnvironmentItems[0]
}

func sortRendered(cs []RenderedChange) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j-1].ID > cs[j].ID; j-- {
			cs[j-1], cs[j] = cs[j], cs[j-1]
		}
	}
}

func TestEnvironmentItemsValidate(t *testing.T) {
	for _, scope := range []string{ScopeRenderedUnexplained, ScopeRoutineSelfArtifact, ScopeDocumentation} {
		fx, it := envItemFixture(t, scope)
		if err := fx.b.Validate(); err != nil {
			t.Fatalf("%s: %v", scope, err)
		}
		if _, ok := fx.b.Item(it.ID); !ok {
			t.Errorf("%s: Item() misses the environment item", scope)
		}
		in := strings.Join(fx.b.InScope(), ",")
		if strings.Contains(in, it.ID) != (scope == ScopeRenderedUnexplained) {
			t.Errorf("%s: InScope() = %s", scope, in)
		}
		if un := fx.b.Uncorrelated(); len(un) != 0 {
			t.Errorf("%s: the anchored change is still uncorrelated: %v", scope, un)
		}
	}
	cases := map[string]struct {
		mut  func(b *EvidenceBundle, it *ChangeItem)
		want string
	}{
		"scope rule": {func(_ *EvidenceBundle, it *ChangeItem) { it.ScopeRule = scopeCategoryForTest }, "environment items are scope:rendered-unexplained"},
		"anchor is environment-scope": {func(_ *EvidenceBundle, it *ChangeItem) {
			it.RenderedChange = "rc-nope"
			it.ID = ChangeItemID("", "", it.RenderedChange)
		}, "is not an environment-scope rendered change of this bundle"},
		"anchor correlates": {func(b *EvidenceBundle, it *ChangeItem) {
			for i := range b.Renders[0].Changes {
				if b.Renders[0].Changes[i].ID == it.RenderedChange {
					b.Renders[0].Changes[i].ChangeItems = []string{b.Change.Items[0].ID}
				}
			}
		}, "is not correlated to it"},
		"id derived": {func(_ *EvidenceBundle, it *ChangeItem) { it.ID = "ci-other" }, "id derived from it"},
		"in scope":   {func(_ *EvidenceBundle, it *ChangeItem) { it.InScope = false }, "requires inScope=true"},
		"family":     {func(_ *EvidenceBundle, it *ChangeItem) { it.Family = FamilyBehavior }, "requires family rendered"},
		"evidence":   {func(_ *EvidenceBundle, it *ChangeItem) { it.Evidence = nil }, "evidence are required"},
		"duplicate":  {func(b *EvidenceBundle, it *ChangeItem) { b.EnvironmentItems = append(b.EnvironmentItems, *it) }, "duplicate item id"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fx, it := envItemFixture(t, ScopeRenderedUnexplained)
			c.mut(fx.b, it)
			wantInvErr(t, fx.b.Validate(), c.want)
		})
	}
}

// scopeCategoryForTest is a record-only scope rule.
const scopeCategoryForTest = "scope:category"

func TestRecordRenderedScopeRules(t *testing.T) {
	cases := map[string]struct {
		mut  func(it *ChangeItem)
		want string
	}{
		"unexplained needs an anchor": {func(it *ChangeItem) { it.ScopeRule, it.InScope = ScopeRenderedUnexplained, true }, "requires a rendered-change anchor"},
		"self-artifact out of scope": {func(it *ChangeItem) {
			it.ScopeRule, it.InScope = ScopeRoutineSelfArtifact, true
		}, "scope:routine-self-artifact requires inScope=false"},
		"documentation out of scope": {func(it *ChangeItem) {
			it.ScopeRule, it.InScope = ScopeDocumentation, true
		}, "scope:documentation requires inScope=false"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := newInvFixture(t).b.Change
			r.Items = append([]ChangeItem(nil), r.Items...)
			c.mut(&r.Items[0])
			wantInvErr(t, r.Validate(), c.want)
		})
	}
}

// scope:documentation is only for schema documentation text: a real
// configuration change can never be filed away as documentation.
func TestDocumentationScopeOnlyForDescriptionText(t *testing.T) {
	fx, it := envItemFixture(t, ScopeDocumentation)
	for i := range fx.b.Renders[0].Changes {
		if c := &fx.b.Renders[0].Changes[i]; c.ID == it.RenderedChange {
			c.Class = "field-removed"
		}
	}
	wantInvErr(t, fx.b.Validate(), "scope:documentation is only for a crd-description-changed rendered change")
	fx, _ = envItemFixture(t, ScopeDocumentation)
	if err := fx.inv.ValidateAgainst(fx.b); err != nil && strings.Contains(err.Error(), "has no finding") {
		t.Errorf("an out-of-scope documentation item needs no finding: %v", err)
	}
}

func TestEnvironmentItemNeedsFinding(t *testing.T) {
	fx, _ := envItemFixture(t, ScopeRenderedUnexplained)
	wantInvErr(t, fx.inv.ValidateAgainst(fx.b), "has no finding")
	fx, _ = envItemFixture(t, ScopeRoutineSelfArtifact)
	if err := fx.inv.ValidateAgainst(fx.b); err != nil && strings.Contains(err.Error(), "has no finding") {
		t.Errorf("an out-of-scope self-artifact item needs no finding: %v", err)
	}
}

func TestAutoApprovalAcceptsNotApplicableRenders(t *testing.T) {
	fx := newInvFixture(t)
	na := ManifestPair{Scope: RenderEnvironment, Target: "kustomize:apps/payments", Status: CollectionNotApplicable, Reason: "version-independent: …"}
	before := fx.b.Uncorrelated()
	fx.b.Renders = append(fx.b.Renders, na)
	if err := fx.b.Validate(); err != nil && strings.Contains(err.Error(), "kustomize:apps/payments") {
		t.Errorf("a not-applicable pair is valid bundle content: %v", err)
	}
	if got := fx.b.Uncorrelated(); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Errorf("a not-applicable pair changes Uncorrelated(): %v → %v", before, got)
	}
	fx.inv.Decision.Outcome = PolicyAutoApproveEligible
	if err := fx.inv.ValidateAgainst(fx.b); err != nil && strings.Contains(err.Error(), "environment render") {
		t.Errorf("a not-applicable pair next to a complete one blocks: %v", err)
	}
	fx.b.Renders = []ManifestPair{na}
	wantInvErr(t, fx.inv.ValidateAgainst(fx.b), "every environment render is not-applicable")
}

func TestSchemaFieldPath(t *testing.T) {
	for _, c := range []struct {
		path, version, field string
		ok                   bool
	}{
		{"spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.privateKey.properties.rotationPolicy.description", "v1", "spec.privateKey.rotationPolicy", true},
		{"spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.ports.items.properties.name.description", "v1", "spec.ports[].name", true},
		{"spec.validation.openAPIV3Schema.properties.spec.properties.mode.description", "", "spec.mode", true},
		{"spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.type", "", "", false},
		{"spec.versions[name=v1].schema.openAPIV3Schema.description", "", "", false},
		{"spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.labels.additionalProperties.description", "", "", false},
		{"spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.oneOf[0].description", "", "", false},
		{"metadata.description", "", "", false},
	} {
		v, f, ok := SchemaFieldPath(c.path)
		if ok != c.ok || (ok && (v != c.version || f != c.field)) {
			t.Errorf("%s: %q %q %v", c.path, v, f, ok)
		}
	}
}
