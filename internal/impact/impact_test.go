package impact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

var testNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// edgeBuilder assembles a minimal but valid UpgradeEdge for join tests.
type edgeBuilder struct {
	edge *domain.UpgradeEdge
}

func newEdge() *edgeBuilder {
	return &edgeBuilder{edge: &domain.UpgradeEdge{
		SchemaVersion: domain.UpgradeEdgeSchemaVersion,
		Product:       domain.ProductRef{ID: "example", Name: "Example"},
		From:          domain.MustVersion("v1.0.0", "1.0.0"),
		To:            domain.MustVersion("v1.1.0", "1.1.0"),
		Path:          []domain.PathStep{{Version: domain.MustVersion("v1.1.0", "1.1.0"), Reason: "minor-release"}},
	}}
}

func (b *edgeBuilder) ev(uri, excerpt string) domain.EvidenceID {
	e := domain.NewEvidence(domain.EvidenceStructured, "chart", uri, "L1-L9", excerpt, domain.Digest([]byte(uri)), testNow)
	for _, x := range b.edge.Evidence {
		if x.ID == e.ID {
			return e.ID
		}
	}
	b.edge.Evidence = append(b.edge.Evidence, e)
	return e.ID
}

func computed(rule string) domain.Provenance {
	return domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: rule, Confidence: domain.ConfidenceHigh}
}

func (b *edgeBuilder) change(rule, title string, subjects ...string) domain.Change {
	ev := b.ev("https://example/"+rule+".yaml", title)
	c := domain.Change{
		ID: "chg-" + domain.ShortHash(rule, title), Category: domain.CategoryOther, Title: title,
		Subjects: subjects, Provenance: computed(rule), Evidence: []domain.EvidenceID{ev},
	}
	for i := range b.edge.Changes {
		if b.edge.Changes[i].ID == c.ID {
			return b.edge.Changes[i]
		}
	}
	b.edge.Changes = append(b.edge.Changes, c)
	return c
}

func (b *edgeBuilder) constraint(kind, versions string) *domain.CompatibilityChange {
	ev := b.ev("https://example/compat.md", versions)
	plat := "kubernetes"
	c := &domain.CompatibilityChange{Platform: plat,
		From: &domain.CompatibilityConstraint{Platform: plat, Kind: kind, Versions: splitVersions(versions), Raw: versions,
			Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.table@v1", Confidence: domain.ConfidenceHigh}, Evidence: []domain.EvidenceID{ev}},
		To: &domain.CompatibilityConstraint{Platform: plat, Kind: kind, Versions: splitVersions(versions), Raw: versions,
			Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.table@v1", Confidence: domain.ConfidenceHigh}, Evidence: []domain.EvidenceID{ev}}}
	b.edge.Compatibility = append(b.edge.Compatibility, *c)
	return &b.edge.Compatibility[len(b.edge.Compatibility)-1]
}

func splitVersions(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func loadEnv(t *testing.T, in env.Inputs) *env.Environment {
	t.Helper()
	e, err := env.Load(in)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func buildReport(t *testing.T, e *domain.UpgradeEdge, en *env.Environment) *domain.ImpactReport {
	t.Helper()
	r, err := Build(Input{Edge: e, Env: en, Now: testNow})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("report does not validate: %v", err)
	}
	return r
}

func findingsByRule(r *domain.ImpactReport, rule string) []domain.ImpactFinding {
	var out []domain.ImpactFinding
	for _, f := range r.Findings {
		if f.Rule == rule {
			out = append(out, f)
		}
	}
	return out
}

func findingsByClass(r *domain.ImpactReport, class domain.ImpactClass) []domain.ImpactFinding {
	var out []domain.ImpactFinding
	for _, f := range r.Findings {
		if f.Classification == class {
			out = append(out, f)
		}
	}
	return out
}

// --- class action-required / review-required / informational: the values rules ----

func TestValuesRemovedMatch(t *testing.T) {
	eb := newEdge()
	removed := eb.change("values:removed", "Helm value `webhook.config` removed", "webhook.config")

	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", `webhook:
  config: true
  keep: 1
`)
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}}))

	fs := findingsByRule(e, RuleValuesRemoved)
	if len(fs) != 1 {
		t.Fatalf("values-removed findings = %d (%+v)", len(fs), e.Findings)
	}
	f := fs[0]
	if f.Classification != domain.ImpactActionRequired || f.ChangeID != removed.ID {
		t.Errorf("finding = %+v", f)
	}
	if f.Severity != domain.SeverityHigh {
		t.Errorf("removed value must be high severity, got %q", f.Severity)
	}
	if len(f.Matches) != 1 || f.Matches[0].Subject != "webhook.config" || f.Matches[0].Kind != domain.MatchValuesKey {
		t.Errorf("matches = %+v", f.Matches)
	}
	if len(f.UpstreamEvidence) == 0 || len(f.EnvironmentEvidence) == 0 {
		t.Error("affected finding must cite both chains")
	}
	if e.Summary.ActionRequired != 1 || e.Summary.AffectEnvironment != 1 || e.Summary.UpstreamChanges != 1 {
		t.Errorf("summary = %+v", e.Summary)
	}
}

func TestValuesPinnedIsInformationalAndAdjacentIsReview(t *testing.T) {
	eb := newEdge()
	eb.change("values:default-changed", "Default of `replicaCount` changed: 1 → 3", "replicaCount")
	eb.change("values:default-changed", "Default of `ingress.timeout` changed: 30 → 60", "ingress.timeout")
	eb.change("values:added", "New Helm value `new.feature`", "new.feature")

	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", "replicaCount: 5\ningress: {}\nnew:\n  feature: on\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}}))

	pinned := findingsByRule(e, RuleValuesPinned)
	if len(pinned) != 1 || pinned[0].Matches[0].Subject != "replicaCount" {
		t.Fatalf("pinned = %+v", pinned)
	}
	if pinned[0].Classification != domain.ImpactInformational || pinned[0].Severity != domain.SeverityLow {
		t.Errorf("pinning the exact key is low-severity informational: %+v", pinned[0])
	}
	adjacent := findingsByRule(e, RuleValuesAdjacent)
	if len(adjacent) != 1 || !strings.Contains(adjacent[0].Detail, "ingress.timeout") {
		t.Fatalf("adjacent = %+v", adjacent)
	}
	if adjacent[0].Classification != domain.ImpactReviewRequired || adjacent[0].Severity != domain.SeverityMedium {
		t.Errorf("partial overlap is medium-severity review-required: %+v", adjacent[0])
	}
	if fs := findingsByRule(e, RuleValuesNewKey); len(fs) != 1 || fs[0].Classification != domain.ImpactReviewRequired {
		t.Errorf("new-key = %+v", fs)
	}
}

// --- class not-affected: full visibility, checked, no overlap ----------------------

func TestNoMatchWithValuesSuppliedIsNotAffected(t *testing.T) {
	eb := newEdge()
	eb.change("values:removed", "Helm value `webhook.config` removed", "webhook.config")

	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", "something:\n  else: 1\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}}))

	fs := findingsByRule(e, RuleValuesUnset)
	if len(fs) != 1 {
		t.Fatalf("values-unset findings = %+v (all: %+v)", fs, e.Findings)
	}
	f := fs[0]
	if f.Classification != domain.ImpactNotAffected {
		t.Fatalf("checked-but-clear must be not-affected, got %s", f.Classification)
	}
	if len(f.Matches) != 0 || len(f.Checks) != 1 {
		t.Fatalf("not-affected carries the evaluation record, not matches: %+v", f)
	}
	if f.Checks[0].Dimension != domain.DimensionValues || f.Checks[0].Facts != 1 || len(f.Checks[0].Subjects) != 1 {
		t.Errorf("check = %+v (1 values fact compared against the upstream subject)", f.Checks[0])
	}
	if len(f.UpstreamEvidence) == 0 {
		t.Error("not-affected keeps the upstream chain")
	}
	if e.Summary.NotAffected != 1 || e.Summary.AffectEnvironment != 0 {
		t.Errorf("summary = %+v", e.Summary)
	}
}

// --- class unknown: missing visibility beats silent assumptions ---------------------

func TestValuesChangeWithoutValuesFileIsUnknown(t *testing.T) {
	eb := newEdge()
	eb.change("values:removed", "Helm value `a` removed", "a")

	// only the cluster version is supplied: the values dimension is missing
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.31"}))
	fs := findingsByRule(e, RuleInsufficientVisibility)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactUnknown {
		t.Fatalf("findings = %+v", e.Findings)
	}
	if len(fs[0].NeededToDetermine) == 0 || !strings.Contains(fs[0].NeededToDetermine[0], "--values") {
		t.Errorf("neededToDetermine must name the missing input: %v", fs[0].NeededToDetermine)
	}
	if len(fs[0].Matches) != 0 || len(fs[0].Checks) != 0 {
		t.Errorf("unknown carries neither matches nor checks: %+v", fs[0])
	}
	if e.Summary.Unknown != 1 || e.Summary.NotAffected != 0 || e.Summary.AffectEnvironment != 0 {
		t.Errorf("summary = %+v", e.Summary)
	}
	// never silently promoted or dismissed
	for _, class := range []domain.ImpactClass{domain.ImpactActionRequired, domain.ImpactNotAffected} {
		if n := len(findingsByClass(e, class)); n != 0 {
			t.Errorf("UNKNOWN must never become %s: %d findings", class, n)
		}
	}
}

func TestValuesNestedKeyEdgeCases(t *testing.T) {
	eb := newEdge()
	// removed section: subjects are the removed leaves
	eb.change("values:section-removed", "Helm values section `webhook.*` removed (2 keys)", "webhook.timeout", "webhook.extra")
	// default changed on a key the customer pins and on one below a set section
	eb.change("values:default-changed", "Default of `replicaCount` changed: 1 → 3", "replicaCount")
	eb.change("values:default-changed", "Default of `ingress.timeout` changed: 30 → 60", "ingress.timeout")
	// new key that the customer already sets
	eb.change("values:added", "New Helm value `new.feature`", "new.feature")

	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", `replicaCount: 5
ingress: {}
new:
  feature: on
unrelated: x
`)
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}}))

	if fs := findingsByRule(e, RuleValuesRemoved); len(fs) != 0 {
		t.Errorf("customer sets nothing under webhook.*; got %+v", fs)
	}
	// the removed section was checked with full visibility and does not apply
	na := findingsByRule(e, RuleValuesUnset)
	if len(na) != 1 || !strings.Contains(na[0].Title, "webhook") {
		t.Errorf("values-unset = %+v", na)
	}
	// segment-wise matching: a.bb must not relate to a.b.c
	eb2 := newEdge()
	eb2.change("values:removed", "Helm value `a.b` removed", "a.b")
	val2 := writeFile(t, dir, "v2.yaml", "a:\n  bb: 1\n")
	e2 := buildReport(t, eb2.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val2}}))
	if fs := findingsByClass(e2, domain.ImpactActionRequired); len(fs) != 0 {
		t.Errorf("a.bb is not under a.b; got affected findings %+v", fs)
	}
	if fs := findingsByRule(e2, RuleValuesUnset); len(fs) != 1 {
		t.Errorf("checked and clear → not-affected: %+v", fs)
	}
}

// --- the demotion rule: ambiguity is never ACTION REQUIRED --------------------------

func TestLowConfidenceActionIsDemotedToReview(t *testing.T) {
	eb := newEdge()
	eb.change("crd:removed", "CRD foos.example.io removed", "foos.example.io")
	// the deciding dimension (installed CRDs) is supplied, but it holds a CRD
	// of another group; the manifest matches the removed CRD's group only →
	// group-only evidence, medium confidence → must not stay ACTION REQUIRED.
	dir := t.TempDir()
	manifests := writeFile(t, dir, "m.yaml", "apiVersion: example.io/v1\nkind: Foo\nspec:\n  a: 1\n")
	crds := writeFile(t, dir, "crds.yaml", "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: bars.other.io\nspec:\n  group: other.io\n  names: {kind: Bar, plural: bars}\n  versions:\n    - name: v1\n      served: true\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}, CRDs: []string{crds}}))
	fs := findingsByRule(e, RuleCRDRemoved)
	if len(fs) != 1 {
		t.Fatalf("findings = %+v", e.Findings)
	}
	if fs[0].Classification != domain.ImpactReviewRequired {
		t.Errorf("below-high confidence must demote to review-required, got %s", fs[0].Classification)
	}
	if fs[0].Provenance.Confidence != domain.ConfidenceMedium {
		t.Errorf("group-only match must stay medium confidence: %+v", fs[0].Provenance)
	}
}

func TestValidateRejectsActionRequiredWithoutHighConfidence(t *testing.T) {
	// the contract rule as data: an action-required finding with medium
	// confidence cannot validate
	eb := newEdge()
	eb.change("values:removed", "Helm value `a` removed", "a")
	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", "a: 1\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}}))
	e.Findings[0].Classification = domain.ImpactActionRequired
	e.Findings[0].Provenance.Confidence = domain.ConfidenceMedium
	if err := e.Validate(); err == nil || !strings.Contains(err.Error(), "high confidence") {
		t.Errorf("expected a demotion-contract error, got %v", err)
	}
}

// --- rule 2: CRDs -------------------------------------------------------------------

func TestCRDVersionRemovedMatch(t *testing.T) {
	eb := newEdge()
	eb.change("crd:version-removed", "API version example.io/v1beta1 of Foo removed", "foos.example.io/v1beta1")
	eb.change("crd:version-deprecated", "API version example.io/v2 of Foo deprecated", "foos.example.io/v2")
	eb.change("crd:removed", "CRD bats.other.io removed", "bats.other.io")
	// The two schema changes carry the differ's real output shape: the
	// GVK-scoped matcher parses the CRD identity (group/version/kind) from
	// title + detail; a change without it would be unknown, not path-matched.
	eb.schemaChange("crd:fields-removed", "Foo v1beta1 schema: 1 field removed: `spec.oldField`",
		"Fields no longer in the example.io/v1beta1 schema of foos.example.io are pruned from stored objects and rejected or dropped in manifests. Remove them from your resources.\nRemoved paths:\nspec.oldField",
		"spec.oldField")
	eb.schemaChange("crd:fields-removed", "Foo v2 schema: 1 field removed: `spec.arr[].gone`",
		"Fields no longer in the example.io/v2 schema of foos.example.io are pruned from stored objects and rejected or dropped in manifests. Remove them from your resources.\nRemoved paths:\nspec.arr[].gone",
		"spec.arr[].gone")

	dir := t.TempDir()
	manifests := writeFile(t, dir, "manifests.yaml", `apiVersion: example.io/v1beta1
kind: Foo
metadata:
  name: f1
spec:
  oldField: yes
---
apiVersion: example.io/v2
kind: Foo
metadata:
  name: f2
spec:
  arr:
    - name: a
      gone: 1
`)
	crd := writeFile(t, dir, "crds.yaml", `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: foos.example.io
spec:
  group: example.io
  names: {kind: Foo, plural: foos}
  versions:
    - name: v1beta1
      served: true
      storage: false
`)
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}, CRDs: []string{crd}}))

	if fs := findingsByRule(e, RuleCRDVersionRemoved); len(fs) != 1 {
		t.Fatalf("version-removed findings = %d: %+v", len(fs), e.Findings)
	} else {
		f := fs[0]
		if f.Classification != domain.ImpactActionRequired || f.Severity != domain.SeverityCritical {
			t.Errorf("classification/severity = %s/%s", f.Classification, f.Severity)
		}
		kinds := map[domain.ImpactMatchKind]bool{}
		for _, m := range f.Matches {
			kinds[m.Kind] = true
		}
		if !kinds[domain.MatchAPIVersion] || !kinds[domain.MatchCRDVersion] {
			t.Errorf("matches = %+v (want api-version and crd-version)", f.Matches)
		}
	}
	if fs := findingsByRule(e, RuleCRDVersionDeprecated); len(fs) != 1 || fs[0].Classification != domain.ImpactReviewRequired {
		t.Errorf("deprecated findings = %+v", findingsByRule(e, RuleCRDVersionDeprecated))
	}
	// bats.other.io: checked against the supplied CRDs and clear → not-affected
	if fs := findingsByRule(e, RuleCRDUnused); len(fs) != 1 || fs[0].Classification != domain.ImpactNotAffected {
		t.Fatalf("crd-unused = %+v", findingsByRule(e, RuleCRDUnused))
	}
	fs := findingsByRule(e, RuleCRDFieldRemoved)
	if len(fs) != 2 {
		t.Fatalf("field findings = %d: %+v", len(fs), e.Findings)
	}
	// spec.oldField set exactly → action-required; spec.arr[].gone matches the
	// manifest leaf spec.arr (set) — the manifest sets the array whose item
	// carried the removed field, so it must not be silently dropped either.
	var classes []domain.ImpactClass
	for _, f := range fs {
		classes = append(classes, f.Classification)
	}
	if classes[0] != domain.ImpactActionRequired {
		t.Errorf("classes = %v", classes)
	}
}

func TestCRDChangeWithoutCRDsInputIsUnknown(t *testing.T) {
	eb := newEdge()
	eb.change("crd:removed", "CRD foos.example.io removed", "foos.example.io")
	dir := t.TempDir()
	// manifests only: the deciding dimension (installed CRDs) is missing
	manifests := writeFile(t, dir, "m.yaml", "apiVersion: other.io/v1\nkind: Bar\nspec:\n  a: 1\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}}))
	fs := findingsByRule(e, RuleInsufficientVisibility)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactUnknown {
		t.Fatalf("findings = %+v", e.Findings)
	}
	if len(fs[0].NeededToDetermine) == 0 || !strings.Contains(fs[0].NeededToDetermine[0], "--crds") {
		t.Errorf("neededToDetermine = %v", fs[0].NeededToDetermine)
	}
	// the possible partial check (manifests were supplied) is recorded
	if len(fs[0].Checks) != 1 || fs[0].Checks[0].Dimension != domain.DimensionManifests {
		t.Errorf("partial checks = %+v", fs[0].Checks)
	}
}

// --- rule 3: Kubernetes ---------------------------------------------------------------

func newNarrowedEdge(eb *edgeBuilder) {
	ev := eb.ev("https://example/compat.md", "1.29, 1.30, 1.31, 1.32, 1.33")
	cc := domain.CompatibilityChange{Platform: "kubernetes", Narrowed: true,
		From: &domain.CompatibilityConstraint{Platform: "kubernetes", Kind: "supported",
			Versions: []string{"1.28", "1.29", "1.30"}, Raw: "1.28-1.30",
			Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.table@v1", Confidence: domain.ConfidenceHigh}, Evidence: []domain.EvidenceID{ev}},
		To: &domain.CompatibilityConstraint{Platform: "kubernetes", Kind: "supported",
			Versions: []string{"1.29", "1.30", "1.31", "1.32", "1.33"}, Raw: "1.29-1.33",
			Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.table@v1", Confidence: domain.ConfidenceHigh}, Evidence: []domain.EvidenceID{ev}}}
	eb.edge.Compatibility = append(eb.edge.Compatibility, cc)
}

func TestKubernetesFindings(t *testing.T) {
	eb := newEdge()
	newNarrowedEdge(eb)

	for _, tc := range []struct {
		cluster, rule string
		class         domain.ImpactClass
	}{
		{"1.28", RuleKubernetesBelow, domain.ImpactActionRequired},
		{"1.28.9", RuleKubernetesBelow, domain.ImpactActionRequired},
		{"1.31", RuleKubernetesInRange, domain.ImpactInformational},
		{"1.35", RuleKubernetesAbove, domain.ImpactActionRequired},
	} {
		e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: tc.cluster}))
		fs := findingsByRule(e, tc.rule)
		if len(fs) != 1 {
			t.Fatalf("cluster %s: rule %s findings = %+v (all: %+v)", tc.cluster, tc.rule, fs, e.Findings)
		}
		if fs[0].Classification != tc.class {
			t.Errorf("cluster %s: classification = %s", tc.cluster, fs[0].Classification)
		}
		if len(fs[0].EnvironmentEvidence) == 0 || fs[0].Matches[0].Kind != domain.MatchKubernetes {
			t.Errorf("cluster %s: matches = %+v", tc.cluster, fs[0].Matches)
		}
	}
	// a cluster outside even the old range is below without the "narrowed
	// under you" wording — and with the pre-existing exclusion stated: the
	// source range is known and also excluded 1.25, so the detail must not
	// imply this upgrade causes the exclusion
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.25"}))
	fs := findingsByRule(e, RuleKubernetesBelow)
	if len(fs) != 1 || strings.Contains(fs[0].Detail, "narrowed under you") {
		t.Errorf("1.25 detail = %q", fs[0].Detail)
	}
	if !strings.Contains(fs[0].Detail, "pre-existing") ||
		!strings.Contains(fs[0].Detail, "also already outside the source release's range") {
		t.Errorf("pre-existing exclusion must be stated: %q", fs[0].Detail)
	}
	if fs[0].Classification != domain.ImpactActionRequired {
		t.Errorf("pre-existing or not, the upgrade stays blocked: %s", fs[0].Classification)
	}
	e = buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.28"}))
	if !strings.Contains(findingsByRule(e, RuleKubernetesBelow)[0].Detail, "narrowed under you") {
		t.Error("1.28 was supported before and is dropped now; detail must say so")
	}
	// no From constraint: no pre-existing claim (nothing is known about the
	// source range)
	eb2 := newEdge()
	ev2 := eb2.ev("https://example/compat2.md", "1.29, 1.30, 1.31")
	eb2.edge.Compatibility = append(eb2.edge.Compatibility, domain.CompatibilityChange{Platform: "kubernetes",
		To: &domain.CompatibilityConstraint{Platform: "kubernetes", Kind: "supported",
			Versions: []string{"1.29", "1.30", "1.31"}, Raw: "1.29-1.31",
			Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.table@v1", Confidence: domain.ConfidenceHigh}, Evidence: []domain.EvidenceID{ev2}}})
	e2 := buildReport(t, eb2.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.25"}))
	if strings.Contains(findingsByRule(e2, RuleKubernetesBelow)[0].Detail, "pre-existing") {
		t.Error("without a From constraint the pre-existing claim would be invented")
	}
}

// TestKubeVersionAdmitsNote: when the chart's kubeVersion admits the cluster
// while the supported range does not, the detail must reconcile the two —
// "requires" alone overstates (the reviewers' case); class unchanged.
func TestKubeVersionAdmitsNote(t *testing.T) {
	eb := newEdge()
	eb.constraint("supported", "1.29, 1.30, 1.31")
	ev := eb.ev("https://example/chart", "kubeVersion: '>= 1.22.0-0'")
	mk := func() *domain.CompatibilityConstraint {
		return &domain.CompatibilityConstraint{Platform: "kubernetes", Kind: "chart-kubeVersion",
			Constraint: ">=1.22.0-0", Raw: ">= 1.22.0-0",
			Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.helm@v1", Confidence: domain.ConfidenceHigh},
			Evidence:   []domain.EvidenceID{ev}}
	}
	eb.edge.Compatibility = append(eb.edge.Compatibility, domain.CompatibilityChange{Platform: "kubernetes", From: mk(), To: mk()})
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.28"}))
	below := findingsByRule(e, RuleKubernetesBelow)
	if len(below) != 1 || below[0].Classification != domain.ImpactActionRequired {
		t.Fatalf("below findings = %+v (all %+v)", below, e.Findings)
	}
	for _, want := range []string{
		"The chart's kubeVersion constraint itself admits 1.28",
		"tested-matrix statement",
		"Helm will not refuse the install",
	} {
		if !strings.Contains(below[0].Detail, want) {
			t.Errorf("detail lacks %q: %q", want, below[0].Detail)
		}
	}
	// and the kubeVersion constraint itself is checked-and-clear
	if fs := findingsByRule(e, RuleCompatSatisfied); len(fs) != 1 {
		t.Errorf("compat-satisfied = %+v", fs)
	}
	// without a kubeVersion constraint the note must not appear
	eb2 := newEdge()
	eb2.constraint("supported", "1.29, 1.30, 1.31")
	e2 := buildReport(t, eb2.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.28"}))
	if strings.Contains(findingsByRule(e2, RuleKubernetesBelow)[0].Detail, "kubeVersion") {
		t.Error("kubeVersion note invented without a kubeVersion constraint")
	}
}

func TestKubernetesConstraintWithoutClusterVersionIsUnknown(t *testing.T) {
	eb := newEdge()
	newNarrowedEdge(eb)
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{mustValues(t)}}))
	fs := findingsByRule(e, RuleInsufficientVisibility)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactUnknown {
		t.Fatalf("findings = %+v", e.Findings)
	}
	if got := fs[0].NeededToDetermine; len(got) != 1 || !strings.Contains(got[0], "--kubernetes") {
		t.Errorf("neededToDetermine = %v", got)
	}
	if fs := findingsByClass(e, domain.ImpactActionRequired); len(fs) != 0 {
		t.Errorf("no cluster version, yet an action finding fired: %+v", fs)
	}
}

func TestUncomputableConstraintIsUnknown(t *testing.T) {
	eb := newEdge()
	ev := eb.ev("https://example/compat.md", "see the README")
	eb.edge.Compatibility = append(eb.edge.Compatibility, domain.CompatibilityChange{Platform: "kubernetes",
		To: &domain.CompatibilityConstraint{Platform: "kubernetes", Kind: "supported", Raw: "see the README",
			Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.table@v1", Confidence: domain.ConfidenceLow}, Evidence: []domain.EvidenceID{ev}}})
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.31"}))
	fs := findingsByRule(e, RuleInsufficientVisibility)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactUnknown {
		t.Fatalf("findings = %+v", e.Findings)
	}
	if !strings.Contains(strings.Join(fs[0].NeededToDetermine, " "), "machine-readable") {
		t.Errorf("neededToDetermine = %v", fs[0].NeededToDetermine)
	}
	// the cluster-version check IS recorded on the unknown record
	if len(fs[0].Checks) != 1 || fs[0].Checks[0].Dimension != domain.DimensionCluster || fs[0].Checks[0].Platform != "kubernetes" {
		t.Errorf("checks = %+v", fs[0].Checks)
	}
}

func TestSatisfiedMinimumConstraintIsNotAffected(t *testing.T) {
	eb := newEdge()
	ev := eb.ev("https://example/compat.md", ">=1.25.0-0")
	eb.edge.Compatibility = append(eb.edge.Compatibility, domain.CompatibilityChange{Platform: "kubernetes",
		To: &domain.CompatibilityConstraint{Platform: "kubernetes", Kind: "minimum", Constraint: ">=1.25.0-0", Raw: ">=1.25.0-0",
			Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.helm@v1", Confidence: domain.ConfidenceHigh}, Evidence: []domain.EvidenceID{ev}}})
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.31"}))
	fs := findingsByRule(e, RuleCompatSatisfied)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactNotAffected {
		t.Fatalf("compat-satisfied = %+v (all %+v)", fs, e.Findings)
	}
	if len(fs[0].Checks) != 1 || fs[0].Checks[0].Facts != 1 || len(fs[0].Checks[0].Evidence) == 0 {
		t.Errorf("check must record the supplied cluster version with its input evidence: %+v", fs[0].Checks)
	}
}

func TestChartKubeVersionBlocks(t *testing.T) {
	eb := newEdge()
	ev := eb.ev("https://example/chart", "kubeVersion")
	eb.edge.Compatibility = append(eb.edge.Compatibility, domain.CompatibilityChange{Platform: "kubernetes",
		To: &domain.CompatibilityConstraint{Platform: "kubernetes", Kind: "chart-kubeVersion",
			Constraint: ">=1.29.0-0", Raw: ">=1.29.0-0",
			Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.helm@v1", Confidence: domain.ConfidenceHigh}, Evidence: []domain.EvidenceID{ev}}})
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.28"}))
	fs := findingsByRule(e, RuleKubeVersionBlocked)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactActionRequired || fs[0].Severity != domain.SeverityCritical {
		t.Fatalf("findings = %+v", e.Findings)
	}
	e = buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.31"}))
	if fs := findingsByRule(e, RuleCompatSatisfied); len(fs) != 1 || fs[0].Classification != domain.ImpactNotAffected {
		t.Errorf("in-range kubeVersion = checked and clear: %+v", fs)
	}
}

// --- rule 4: images -----------------------------------------------------------------

func TestImageFindings(t *testing.T) {
	eb := newEdge()
	eb.change(upgrade.RuleImageTagsChanged, "Third-party image quay.io/other/sidecar: 1.0 → 1.2", "quay.io/other/sidecar")
	ev1, ev2 := eb.ev("https://example/manifest-1.yaml", "old"), eb.ev("https://example/manifest-2.yaml", "new")
	eb.edge.Artifacts = append(eb.edge.Artifacts, domain.ArtifactChange{
		ArtifactID: "image", Type: domain.ArtifactContainerImage, Name: "controller", Change: domain.ChangeUpdated,
		From: &domain.ArtifactInstance{ArtifactID: "image", Type: domain.ArtifactContainerImage, Coordinate: "quay.io/example/controller:v1.0.0", Evidence: []domain.EvidenceID{ev1}},
		To:   &domain.ArtifactInstance{ArtifactID: "image", Type: domain.ArtifactContainerImage, Coordinate: "quay.io/example/controller:v1.1.0", Evidence: []domain.EvidenceID{ev2}},
	})

	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", "image:\n  repository: quay.io/example/controller\n  tag: v1.0.0\n")
	images := writeFile(t, dir, "images.txt", "quay.io/other/sidecar:1.0\n")
	in := env.Inputs{ValuesFiles: []string{val}}
	in.Images = readList(t, images)
	e := buildReport(t, eb.edge, loadEnv(t, in))

	fs := findingsByRule(e, RuleImageChanged)
	if len(fs) != 2 {
		t.Fatalf("image findings = %d: %+v", len(fs), e.Findings)
	}
	var pinnedFound bool
	for _, f := range fs {
		for _, m := range f.Matches {
			if m.Subject == "quay.io/example/controller:v1.0.0" {
				pinnedFound = true
				if f.Classification != domain.ImpactReviewRequired {
					t.Errorf("pinning the old tag = review-required, got %s", f.Classification)
				}
			}
		}
	}
	if !pinnedFound {
		t.Error("the pinned controller image must match the artifact move")
	}
	// the sidecar change was checked (the mirror list references it)…
	if fs := findingsByRule(e, RuleImageChanged); len(fs) != 2 {
		t.Errorf("sidecar is referenced by the mirror list, so it is affected: %+v", fs)
	}
	// already at the target tag → informational
	val2 := writeFile(t, dir, "v2.yaml", "image:\n  repository: quay.io/example/controller\n  tag: v1.1.0\n")
	e2 := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val2}}))
	for _, f := range findingsByRule(e2, RuleImageChanged) {
		if strings.Contains(f.Title, "controller") && f.Classification != domain.ImpactInformational {
			t.Errorf("already at target = informational, got %s (%s)", f.Classification, f.Title)
		}
	}
	// unrelated images: checked with image facts supplied → not-affected,
	// never silently dropped
	val3 := writeFile(t, dir, "v3.yaml", "image:\n  repository: docker.io/library/nginx\n  tag: \"1.0\"\n")
	e3 := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val3}}))
	if fs := findingsByRule(e3, RuleImageNotReferenced); len(fs) != 2 {
		t.Fatalf("not-referenced records = %+v (all: %+v)", fs, e3.Findings)
	}
	for _, f := range findingsByRule(e3, RuleImageNotReferenced) {
		if f.Classification != domain.ImpactNotAffected || len(f.Checks) != 1 || f.Checks[0].Dimension != domain.DimensionImages {
			t.Errorf("record = %+v", f)
		}
	}
}

func TestImageChangeWithoutImageInputsIsUnknown(t *testing.T) {
	// CRDs only: no image facts of any kind
	eb := newEdge()
	eb.change(upgrade.RuleImageTagsChanged, "Third-party image quay.io/other/sidecar: 1.0 → 1.2", "quay.io/other/sidecar")
	dir := t.TempDir()
	crds := writeFile(t, dir, "crds.yaml", "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: foos.example.io\nspec:\n  group: example.io\n  names: {kind: Foo, plural: foos}\n  versions:\n    - name: v1\n      served: true\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{CRDs: []string{crds}}))
	fs := findingsByRule(e, RuleInsufficientVisibility)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactUnknown {
		t.Fatalf("findings = %+v", e.Findings)
	}
}

// --- unknown by construction: changes without a comparable subject -------------------

func TestNoteDerivedChangesAreUnknownNotSilent(t *testing.T) {
	eb := newEdge()
	eb.change("section:/breaking/i", "ACME HTTP01 path type is now Exact")
	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", "a: 1\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}}))
	fs := findingsByRule(e, RuleNotJoined)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactUnknown {
		t.Fatalf("not-joined = %+v (all %+v)", fs, e.Findings)
	}
	if len(fs[0].NeededToDetermine) == 0 || !strings.Contains(fs[0].NeededToDetermine[0], "machine-comparable subject") {
		t.Errorf("neededToDetermine = %v", fs[0].NeededToDetermine)
	}
	if fs[0].ChangeID == "" {
		t.Error("unknown records keep the upstream chain: ChangeID must be set")
	}
	// computed diff rules without a join rule get their own reason
	eb2 := newEdge()
	eb2.change("crd:fields-added", "Foo v1 schema: field added: spec.newThing", "spec.newThing")
	e2 := buildReport(t, eb2.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}}))
	fs2 := findingsByRule(e2, RuleNotJoined)
	if len(fs2) != 1 || !strings.Contains(strings.Join(fs2[0].NeededToDetermine, " "), "crd:fields-added") {
		t.Fatalf("unimplemented diff rule = %+v", fs2)
	}
}

func readList(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// --- report invariants ----------------------------------------------------------------

func TestBuildRejectsMissingParts(t *testing.T) {
	if _, err := Build(Input{}); err == nil {
		t.Fatal("Build requires Edge and Env")
	}
}

func TestDeterministicOutput(t *testing.T) {
	eb := newEdge()
	eb.change("values:removed", "Helm value `a` removed", "a")
	eb.change("section:/breaking/i", "A declared change")
	eb.constraint("supported", "1.29, 1.30, 1.31")
	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", "a: 1\n")
	en := loadEnv(t, env.Inputs{ValuesFiles: []string{val}})
	r1 := buildReport(t, eb.edge, en)
	r2 := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}}))
	if len(r1.Findings) != len(r2.Findings) || len(r1.Evidence) != len(r2.Evidence) {
		t.Fatal("Build must be deterministic")
	}
	for i := range r1.Findings {
		if r1.Findings[i].ID != r2.Findings[i].ID {
			t.Fatalf("finding %d differs: %s vs %s", i, r1.Findings[i].ID, r2.Findings[i].ID)
		}
	}
}

func mustValues(t *testing.T) string {
	t.Helper()
	return writeFile(t, t.TempDir(), "values.yaml", "a: 1\n")
}
