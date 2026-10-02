package impact

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// The changes below mirror the differ's wording (internal/upgrade/crds.go
// diffSchemaAttributes / storage-version change).

func attrEdge(rule, title, detail string, subjects ...string) (*edgeBuilder, domain.Change) {
	eb := newEdge()
	c := eb.schemaChange(rule, title, detail, subjects...)
	return eb, c
}

func onlyFinding(t *testing.T, r *domain.ImpactReport, changeID string) domain.ImpactFinding {
	t.Helper()
	fs := findingsOf(r, changeID)
	if len(fs) != 1 {
		t.Fatalf("want exactly one finding for the change, got %d: %+v", len(fs), fs)
	}
	return fs[0]
}

func TestCRDDefaultChangedJoin(t *testing.T) {
	eb, c := attrEdge(upgrade.RuleCRDDefaultChanged,
		"Certificate v1 schema: default changed: `spec.privateKey.rotationPolicy`",
		"Schema defaults changed in the cert-manager.io/v1 schema of certificates.cert-manager.io (old → new; defaults apply to objects that leave the field unset):\nspec.privateKey.rotationPolicy: \"Never\" → \"Always\"",
		"spec.privateKey.rotationPolicy")
	r := buildReport(t, eb.edge, condEnv(t, "", nil))
	var applies, pinned bool
	for _, f := range findingsOf(r, c.ID) {
		applies = applies || (f.Rule == RuleCRDDefaultApplies && f.Classification == domain.ImpactReviewRequired)
		pinned = pinned || (f.Rule == RuleCRDDefaultPinned && f.Classification == domain.ImpactInformational)
	}
	if !applies || !pinned {
		t.Errorf("default change: applies=%v pinned=%v, want both (Certificate a unset, b pinned)", applies, pinned)
	}
	// no Certificate in the manifests: checked and clear
	dir := t.TempDir()
	en := loadEnv(t, env.Inputs{Manifests: []string{writeFile(t, dir, "m.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: x}\n")}})
	if f := onlyFinding(t, buildReport(t, eb.edge, en), c.ID); f.Rule != RuleCRDAttributeClear || f.Classification != domain.ImpactNotAffected {
		t.Errorf("no Certificate: %s/%s", f.Rule, f.Classification)
	}
	// no manifests: unknown, environment-visibility-gap
	if f := onlyFinding(t, buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.30"})), c.ID); f.Classification != domain.ImpactUnknown || f.UnknownReason != domain.UnknownEnvironmentVisibilityGap {
		t.Errorf("no manifests: %s/%s", f.Classification, f.UnknownReason)
	}
}

func TestCRDEnumChangedJoin(t *testing.T) {
	eb, c := attrEdge(upgrade.RuleCRDEnumChanged,
		"Certificate v1 schema: allowed values changed: `spec.privateKey.rotationPolicy`",
		"Enum values changed in the cert-manager.io/v1 schema of certificates.cert-manager.io; a removed value is rejected by the API server:\nspec.privateKey.rotationPolicy: removed [\"Never\"], added [\"OnRenewal\"]",
		"spec.privateKey.rotationPolicy")
	f := onlyFinding(t, buildReport(t, eb.edge, condEnv(t, "", nil)), c.ID)
	if f.Rule != RuleCRDEnumValueRemoved || f.Classification != domain.ImpactActionRequired {
		t.Errorf("a resource uses the removed value: %s/%s", f.Rule, f.Classification)
	}
	// a removed value nobody uses
	eb2, c2 := attrEdge(upgrade.RuleCRDEnumChanged,
		"Certificate v1 schema: allowed values changed: `spec.privateKey.rotationPolicy`",
		"Enum values changed in the cert-manager.io/v1 schema of certificates.cert-manager.io; a removed value is rejected by the API server:\nspec.privateKey.rotationPolicy: removed [\"Sometimes\"], added []",
		"spec.privateKey.rotationPolicy")
	if f := onlyFinding(t, buildReport(t, eb2.edge, condEnv(t, "", nil)), c2.ID); f.Classification != domain.ImpactNotAffected {
		t.Errorf("unused removed value: %s/%s", f.Rule, f.Classification)
	}
	// kind not pinned (label is the CRD name, no installed CRD): review at most
	eb3, c3 := attrEdge(upgrade.RuleCRDEnumChanged,
		"certificates.cert-manager.io v1 schema: allowed values changed: `spec.privateKey.rotationPolicy`",
		"Enum values changed in the cert-manager.io/v1 schema of certificates.cert-manager.io; a removed value is rejected by the API server:\nspec.privateKey.rotationPolicy: removed [\"Never\"], added []",
		"spec.privateKey.rotationPolicy")
	if f := onlyFinding(t, buildReport(t, eb3.edge, condEnv(t, "", nil)), c3.ID); f.Classification != domain.ImpactReviewRequired {
		t.Errorf("kind unpinned: %s/%s, want review-required (demoted)", f.Rule, f.Classification)
	}
}

func TestCRDFieldRequiredJoin(t *testing.T) {
	eb, c := attrEdge(upgrade.RuleCRDFieldRequired,
		"Certificate v1 schema: field now required: `spec.secretName`",
		"Fields newly required in the cert-manager.io/v1 schema of certificates.cert-manager.io; objects that omit them are rejected:\nspec.secretName",
		"spec.secretName")
	if f := onlyFinding(t, buildReport(t, eb.edge, condEnv(t, "", nil)), c.ID); f.Classification != domain.ImpactNotAffected {
		t.Errorf("every Certificate sets secretName: %s/%s", f.Rule, f.Classification)
	}
	eb2, c2 := attrEdge(upgrade.RuleCRDFieldRequired,
		"Certificate v1 schema: field now required: `spec.privateKey.algorithm`",
		"Fields newly required in the cert-manager.io/v1 schema of certificates.cert-manager.io; objects that omit them are rejected:\nspec.privateKey.algorithm",
		"spec.privateKey.algorithm")
	f := onlyFinding(t, buildReport(t, eb2.edge, condEnv(t, "", nil)), c2.ID)
	// only Certificate b has a privateKey object; it omits algorithm
	if f.Rule != RuleCRDFieldNowRequired || f.Classification != domain.ImpactActionRequired {
		t.Errorf("omitted required field: %s/%s", f.Rule, f.Classification)
	}
}

func TestCRDFieldTypeChangedJoin(t *testing.T) {
	eb, c := attrEdge(upgrade.RuleCRDFieldTypeChange,
		"Certificate v1 schema: field changed type: `spec.secretName`",
		"Field types changed in the cert-manager.io/v1 schema of certificates.cert-manager.io:\nspec.secretName: string → integer",
		"spec.secretName")
	if f := onlyFinding(t, buildReport(t, eb.edge, condEnv(t, "", nil)), c.ID); f.Classification != domain.ImpactActionRequired {
		t.Errorf("string value under an integer type: %s/%s", f.Rule, f.Classification)
	}
}

func TestCRDStorageChangedJoin(t *testing.T) {
	eb := newEdge()
	c := eb.change(upgrade.RuleCRDStorageChanged, "Storage version of `certificates.cert-manager.io` changes v1beta1 → v1", "certificates.cert-manager.io")
	for i := range eb.edge.Changes {
		if eb.edge.Changes[i].ID == c.ID {
			eb.edge.Changes[i].Detail = "Existing Certificate objects stay stored as v1beta1 until rewritten."
		}
	}
	// manifests use the kind → review
	if f := onlyFinding(t, buildReport(t, eb.edge, condEnv(t, "", nil)), c.ID); f.Rule != RuleCRDStorageMigration || f.Classification != domain.ImpactReviewRequired {
		t.Errorf("kind in use: %s/%s", f.Rule, f.Classification)
	}
	dir := t.TempDir()
	// CRDs supplied without it → not affected
	other := writeFile(t, dir, "crds/other.yaml", "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata: {name: issuers.cert-manager.io}\nspec:\n  group: cert-manager.io\n  names: {kind: Issuer}\n  versions: [{name: v1, served: true, storage: true}]\n")
	if f := onlyFinding(t, buildReport(t, eb.edge, loadEnv(t, env.Inputs{CRDs: []string{other}})), c.ID); f.Classification != domain.ImpactNotAffected {
		t.Errorf("CRD not installed: %s/%s", f.Rule, f.Classification)
	}
	// only a values file → unknown
	vals := writeFile(t, dir, "values.yaml", "a: 1\n")
	if f := onlyFinding(t, buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{vals}})), c.ID); f.Classification != domain.ImpactUnknown || f.UnknownReason != domain.UnknownEnvironmentVisibilityGap {
		t.Errorf("no CRDs/manifests: %s/%s", f.Classification, f.UnknownReason)
	}
}
