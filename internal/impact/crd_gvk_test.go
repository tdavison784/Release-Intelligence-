package impact

// Adversarial regression tests for the GVK-scoped CRD matching ladder
// (docs/IMPACT.md, "CRDs and API versions"): API identity — not field paths —
// decides. The cases come from the phase 3 plan's required list:
//
//  1. unrelated resource with an identical spec.* path → no CRD impact
//  2. same API group, different kind → review at most, never action
//  3. exact GVK + exact field → action-required, high confidence
//  4. CRD installed but no manifest uses the kind → not-affected with checks
//  5. manifests supplied, no CRD changes → nothing fabricated
//  6. upstream subject without a parsable GVK → unknown

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// schemaChange adds a crd:fields-removed change shaped like the differ's
// output (internal/upgrade/crds.go): the CRD identity the GVK-scoped matcher
// consumes lives in the title and detail, the subjects are the schema paths.
func (b *edgeBuilder) schemaChange(rule, title, detail string, subjects ...string) domain.Change {
	c := b.change(rule, title, subjects...)
	for i := range b.edge.Changes {
		if b.edge.Changes[i].ID == c.ID {
			b.edge.Changes[i].Detail = detail
			return b.edge.Changes[i]
		}
	}
	return c
}

// fieldsRemovedDetail renders the differ's detail line for one removed path.
func fieldsRemovedDetail(group, version, name, path string) string {
	return "Fields no longer in the " + group + "/" + version + " schema of " + name +
		" are pruned from stored objects and rejected or dropped in manifests. Remove them from your resources.\nRemoved paths:\n" + path
}

const certificateCRD = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: certificates.cert-manager.io
spec:
  group: cert-manager.io
  names: {kind: Certificate, plural: certificates}
  versions:
    - name: v1
      served: true
      storage: true
`

// --- 1. the adversarial case: a same-named path under another GVK is not a CRD impact

func TestGVKFieldPathUnderUnrelatedGVKIsNoCRDImpact(t *testing.T) {
	eb := newEdge()
	eb.schemaChange(upgrade.RuleCRDFieldsRemoved,
		"Certificate v1 schema: 1 field removed: `spec.foo`",
		fieldsRemovedDetail("cert-manager.io", "v1", "certificates.cert-manager.io", "spec.foo"),
		"spec.foo")
	dir := t.TempDir()
	crds := writeFile(t, dir, "crds.yaml", certificateCRD)
	manifests := writeFile(t, dir, "manifests.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  foo: bar
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-com
spec:
  secretName: example-com-tls
`)
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}, CRDs: []string{crds}}))

	if fs := findingsByRule(e, RuleCRDFieldRemoved); len(fs) != 0 {
		t.Fatalf("Deployment/web sets spec.foo, but it is not a Certificate: %+v", fs)
	}
	// checked with full manifests visibility and clear → not-affected
	fs := findingsByRule(e, RuleCRDFieldUnset)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactNotAffected {
		t.Fatalf("field-unset = %+v (all: %+v)", fs, e.Findings)
	}
	if len(fs[0].Checks) != 1 || fs[0].Checks[0].Dimension != domain.DimensionManifests {
		t.Errorf("evaluation record = %+v", fs[0].Checks)
	}
	if e.Summary.ActionRequired+e.Summary.ReviewRequired > 0 {
		t.Errorf("summary = %+v", e.Summary)
	}
}

// --- 2. same API group, different kind → review at most, never action

func TestGVKSameGroupDifferentKindIsReviewAtMost(t *testing.T) {
	eb := newEdge()
	eb.change(upgrade.RuleCRDVersionRemoved, "API version cert-manager.io/v1 of Issuer removed", "issuers.cert-manager.io/v1")
	dir := t.TempDir()
	// the environment runs ClusterIssuer only: same group, different CRD/kind
	crds := writeFile(t, dir, "crds.yaml", `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: clusterissuers.cert-manager.io
spec:
  group: cert-manager.io
  names: {kind: ClusterIssuer, plural: clusterissuers}
  versions:
    - name: v1
      served: true
      storage: true
`)
	manifests := writeFile(t, dir, "manifests.yaml", `apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
`)
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}, CRDs: []string{crds}}))

	if fs := findingsByClass(e, domain.ImpactActionRequired); len(fs) != 0 {
		t.Fatalf("a group-only match must never be ACTION REQUIRED: %+v", fs)
	}
	fs := findingsByRule(e, RuleCRDVersionRemoved)
	if len(fs) != 1 {
		t.Fatalf("version-removed findings = %+v (all: %+v)", fs, e.Findings)
	}
	if fs[0].Classification != domain.ImpactReviewRequired || fs[0].Provenance.Confidence != domain.ConfidenceMedium {
		t.Errorf("group/version-only match = %s/%s, want review-required/medium", fs[0].Classification, fs[0].Provenance.Confidence)
	}
	// the why-block says exactly what is missing to decide
	for _, want := range []string{"kind is not pinned", "issuers.cert-manager.io", "names.kind"} {
		if !strings.Contains(fs[0].Detail, want) {
			t.Errorf("detail must state what is missing (%q): %s", want, fs[0].Detail)
		}
	}
}

// --- 3. exact GVK + exact field → action-required with high confidence

func TestGVKExactGVKAndExactFieldIsActionRequired(t *testing.T) {
	eb := newEdge()
	eb.schemaChange(upgrade.RuleCRDFieldsRemoved,
		"Certificate v1 schema: 1 field removed: `spec.privateKey`",
		fieldsRemovedDetail("cert-manager.io", "v1", "certificates.cert-manager.io", "spec.privateKey"),
		"spec.privateKey")
	dir := t.TempDir()
	crds := writeFile(t, dir, "crds.yaml", certificateCRD)
	manifests := writeFile(t, dir, "manifests.yaml", `apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-com
  namespace: istio-system
spec:
  secretName: example-com-tls
  privateKey: {}
`)
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}, CRDs: []string{crds}}))

	fs := findingsByRule(e, RuleCRDFieldRemoved)
	if len(fs) != 1 {
		t.Fatalf("field-removed findings = %+v (all: %+v)", fs, e.Findings)
	}
	f := fs[0]
	if f.Classification != domain.ImpactActionRequired || f.Provenance.Confidence != domain.ConfidenceHigh || f.Severity != domain.SeverityHigh {
		t.Errorf("exact GVK + exact field = %s/%s/%s, want action-required/high/high", f.Classification, f.Provenance.Confidence, f.Severity)
	}
	// both evidence chains: the GVK inventory evidence (and the matched field's)
	var field, gvk bool
	for _, m := range f.Matches {
		if m.Kind == domain.MatchManifestField && m.Subject == "spec.privateKey" {
			field = true
		}
		if m.Kind == domain.MatchAPIVersion && m.Subject == "cert-manager.io/v1 Certificate" {
			gvk = true
		}
	}
	if !field || !gvk {
		t.Errorf("matches = %+v (want the manifest field and the cert-manager.io/v1 Certificate GVK)", f.Matches)
	}
	if len(f.UpstreamEvidence) == 0 || len(f.EnvironmentEvidence) == 0 {
		t.Error("affected finding must cite both chains")
	}
	// the why-block names the matched resource: kind + (namespace/)name + document ref
	for _, want := range []string{"Environment: Certificate/istio-system/example-com", "manifests.yaml:L", "sets spec.privateKey (L"} {
		if !strings.Contains(f.Detail, want) {
			t.Errorf("detail must state the matched resource (%q): %s", want, f.Detail)
		}
	}
	var buf strings.Builder
	if err := RenderText(&buf, e, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Environment: Certificate/istio-system/example-com") {
		t.Errorf("rendered why-block must name the matched resource:\n%s", buf.String())
	}
}

// --- 4. CRD installed, but no manifest uses its kind → not-affected with checks

func TestGVKRemovedCRDInstalledButKindUnusedIsNotAffected(t *testing.T) {
	eb := newEdge()
	eb.change(upgrade.RuleCRDRemoved, "CRD issuers.cert-manager.io (Issuer) removed", "issuers.cert-manager.io")
	dir := t.TempDir()
	crds := writeFile(t, dir, "crds.yaml", `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: issuers.cert-manager.io
spec:
  group: cert-manager.io
  names: {kind: Issuer, plural: issuers}
  versions:
    - name: v1
      served: true
      storage: true
`)
	// same group, other kind — and a resource of yet another GVK entirely
	manifests := writeFile(t, dir, "manifests.yaml", `apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt
spec:
  acme: {}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg
data:
  a: "1"
`)
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}, CRDs: []string{crds}}))

	for _, class := range []domain.ImpactClass{domain.ImpactActionRequired, domain.ImpactReviewRequired} {
		if fs := findingsByClass(e, class); len(fs) != 0 {
			t.Errorf("installed CRD without manifest usage of its kind must not be %s: %+v", class, fs)
		}
	}
	fs := findingsByRule(e, RuleCRDUnused)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactNotAffected {
		t.Fatalf("crd-unused = %+v (all: %+v)", fs, e.Findings)
	}
	if len(fs[0].Checks) != 2 {
		t.Errorf("checks must record both consulted dimensions: %+v", fs[0].Checks)
	}
	dims := map[domain.EnvironmentDimension]bool{}
	for _, c := range fs[0].Checks {
		dims[c.Dimension] = true
	}
	if !dims[domain.DimensionCRDs] || !dims[domain.DimensionManifests] {
		t.Errorf("checks = %+v (want crds + manifests)", fs[0].Checks)
	}
}

// --- 5. manifests supplied but no CRD changes → nothing fabricated

func TestGVKNoCRDChangesFabricatesNothing(t *testing.T) {
	eb := newEdge()
	eb.change("values:removed", "Helm value `webhook.config` removed", "webhook.config")
	dir := t.TempDir()
	crds := writeFile(t, dir, "crds.yaml", certificateCRD)
	manifests := writeFile(t, dir, "manifests.yaml", "apiVersion: cert-manager.io/v1\nkind: Certificate\nmetadata:\n  name: c\nspec:\n  secretName: s\n")
	val := writeFile(t, dir, "values.yaml", "webhook:\n  config: true\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}, CRDs: []string{crds}, ValuesFiles: []string{val}}))

	for _, f := range e.Findings {
		if strings.HasPrefix(f.Rule, "impact:crd-") {
			t.Errorf("no CRD change on the edge, yet %s fired (%s)", f.Rule, f.Title)
		}
	}
	if fs := findingsByRule(e, RuleValuesRemoved); len(fs) != 1 {
		t.Errorf("the values rule must be untouched: %+v", fs)
	}
}

// --- 6. upstream subject without a parsable GVK → unknown, never guessed

func TestGVKUnparsableUpstreamIdentityIsUnknown(t *testing.T) {
	dir := t.TempDir()
	manifests := writeFile(t, dir, "manifests.yaml", "apiVersion: example.io/v1\nkind: Foo\nmetadata:\n  name: f\nspec:\n  old: 1\n")
	crds := writeFile(t, dir, "crds.yaml", "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: foos.example.io\nspec:\n  group: example.io\n  names: {kind: Foo, plural: foos}\n  versions:\n    - name: v1\n      served: true\n")

	// (a) a fields-removed change whose title/detail yield no API group: the
	// paths cannot be scoped, so they are never path-matched.
	eb := newEdge()
	eb.change(upgrade.RuleCRDFieldsRemoved, "Some schema fields changed", "spec.old")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}, CRDs: []string{crds}}))
	if fs := findingsByRule(e, RuleCRDFieldRemoved); len(fs) != 0 {
		t.Errorf("unparsable identity must not path-match: %+v", fs)
	}
	if fs := findingsByRule(e, RuleCRDFieldUnset); len(fs) != 0 {
		t.Errorf("an unscoped change cannot claim not-affected either: %+v", fs)
	}
	fs := findingsByRule(e, RuleNotJoined)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactUnknown {
		t.Fatalf("not-joined = %+v (all: %+v)", fs, e.Findings)
	}
	if len(fs[0].NeededToDetermine) == 0 || !strings.Contains(strings.Join(fs[0].NeededToDetermine, " "), "group/version/kind") {
		t.Errorf("neededToDetermine = %v", fs[0].NeededToDetermine)
	}

	// (b) a version change whose subject does not state "name/version"
	// (previously silently skipped).
	eb2 := newEdge()
	eb2.change(upgrade.RuleCRDVersionRemoved, "API version of Foo removed", "foos.example.io")
	e2 := buildReport(t, eb2.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}, CRDs: []string{crds}}))
	if fs := findingsByRule(e2, RuleCRDVersionUnused); len(fs) != 0 {
		t.Errorf("an unversioned subject cannot be checked-and-clear: %+v", fs)
	}
	fs = findingsByRule(e2, RuleNotJoined)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactUnknown {
		t.Fatalf("not-joined = %+v (all: %+v)", fs, e2.Findings)
	}
	if !strings.Contains(strings.Join(fs[0].NeededToDetermine, " "), "name and version") {
		t.Errorf("neededToDetermine = %v", fs[0].NeededToDetermine)
	}
}

// --- ladder tiers around the required cases ---------------------------------------

// A removed field matching under group/version WITHOUT a pinned kind (the
// change labels the CRD by name, none installed) is review at most: another
// CRD of the group could serve the kind that sets the path.
func TestGVKFieldsRemovedKindUnpinnedIsReview(t *testing.T) {
	eb := newEdge()
	eb.schemaChange(upgrade.RuleCRDFieldsRemoved,
		"foos.example.io v1 schema: 1 field removed: `spec.old`",
		fieldsRemovedDetail("example.io", "v1", "foos.example.io", "spec.old"),
		"spec.old")
	dir := t.TempDir()
	manifests := writeFile(t, dir, "manifests.yaml", "apiVersion: example.io/v1\nkind: Foo\nmetadata:\n  name: f\nspec:\n  old: 1\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}}))
	fs := findingsByRule(e, RuleCRDFieldRemoved)
	if len(fs) != 1 {
		t.Fatalf("field-removed findings = %+v (all: %+v)", fs, e.Findings)
	}
	if fs[0].Classification != domain.ImpactReviewRequired || fs[0].Provenance.Confidence != domain.ConfidenceMedium {
		t.Errorf("kind-unpinned match = %s/%s, want review-required/medium", fs[0].Classification, fs[0].Provenance.Confidence)
	}
}

// The installed CRD declares the removed version, the manifests were supplied
// and show no usage of the exact GVK → not-affected; without the manifests the
// same shape stays review (usage unverifiable), never action.
func TestGVKVersionDeclaredButUnused(t *testing.T) {
	edge := func() *edgeBuilder {
		eb := newEdge()
		eb.change(upgrade.RuleCRDVersionRemoved, "API version example.io/v1beta1 of Foo removed", "foos.example.io/v1beta1")
		return eb
	}
	dir := t.TempDir()
	crds := writeFile(t, dir, "crds.yaml", "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: foos.example.io\nspec:\n  group: example.io\n  names: {kind: Foo, plural: foos}\n  versions:\n    - name: v1beta1\n      served: true\n")
	// manifests exist but use a different version of the kind: the removed
	// version's GVK is unused
	manifests := writeFile(t, dir, "manifests.yaml", "apiVersion: example.io/v2\nkind: Foo\nmetadata:\n  name: f\nspec:\n  a: 1\n")

	e := buildReport(t, edge().edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}, CRDs: []string{crds}}))
	fs := findingsByRule(e, RuleCRDVersionUnused)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactNotAffected {
		t.Fatalf("declared-but-unused with manifests supplied = %+v (all: %+v)", fs, e.Findings)
	}
	if fs := findingsByClass(e, domain.ImpactActionRequired); len(fs) != 0 {
		t.Errorf("no usage, yet action: %+v", fs)
	}

	// without manifests the usage cannot be checked → review-required
	e2 := buildReport(t, edge().edge, loadEnv(t, env.Inputs{CRDs: []string{crds}}))
	fs = findingsByRule(e2, RuleCRDVersionRemoved)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactReviewRequired || fs[0].Provenance.Confidence != domain.ConfidenceMedium {
		t.Fatalf("declared-but-unverifiable = %+v (all: %+v)", fs, e2.Findings)
	}
	if len(fs[0].Matches) == 0 || fs[0].Matches[0].Kind != domain.MatchCRD {
		t.Errorf("the installed CRD is the environment evidence: %+v", fs[0].Matches)
	}
}

// --- 7. trustfix: an ACTION finding's why-block names only the resources that
// set the removed field, and each matched field once.
//
// Regression (crossplane-1.20-2.0): a removed list field (`spec.resources[]`)
// is reported with dozens of removed sub-paths; each sub-path related to the
// same set list leaf, so the explanation repeated "spec.resources (L18)" for
// every sub-path, and it named every resource of the GVK — including the
// converted Composition that does not set the field at all. The evidence
// chain was right; the prose blamed a resource that is not exposed.
func TestCRDFieldRemovedWhyNamesOnlyTheExposedResources(t *testing.T) {
	eb := newEdge()
	eb.schemaChange(upgrade.RuleCRDFieldsRemoved,
		"Composition v1 schema: 1 field removed: `spec.resources[]`",
		fieldsRemovedDetail("apiextensions.example.io", "v1", "compositions.apiextensions.example.io",
			"spec.resources[]\nspec.resources[].base\nspec.resources[].patches[]"),
		"spec.resources[]", "spec.resources[].base", "spec.resources[].patches[]")
	dir := t.TempDir()
	manifests := writeFile(t, dir, "compositions.yaml", `apiVersion: apiextensions.example.io/v1
kind: Composition
metadata:
  name: legacy
spec:
  mode: Resources
  resources:
    - name: bucket
      base: {kind: Bucket}
---
apiVersion: apiextensions.example.io/v1
kind: Composition
metadata:
  name: converted
spec:
  mode: Pipeline
  pipeline:
    - step: one
`)
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}}))
	fs := findingsByRule(e, RuleCRDFieldRemoved)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactActionRequired {
		t.Fatalf("exact GVK + set removed field must stay one action-required finding: %+v", e.Findings)
	}
	d := fs[0].Detail
	if strings.Contains(d, "converted") {
		t.Errorf("why-block names a resource that does not set the removed field:\n%s", d)
	}
	if !strings.Contains(d, "Composition/legacy") {
		t.Errorf("why-block must name the exposed resource:\n%s", d)
	}
	if n := strings.Count(d, "spec.resources (L"); n != 1 {
		t.Errorf("the matched field must be listed once, got %d times:\n%s", n, d)
	}
}
