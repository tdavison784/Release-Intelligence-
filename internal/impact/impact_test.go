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

// --- rule 1: values -------------------------------------------------------------

func TestValuesRemovedMatch(t *testing.T) {
	eb := newEdge()
	removed := eb.change("values:removed", "Helm value `webhook.config` removed", "webhook.config")
	eb.change("values:removed", "Helm value `unrelated.key` removed", "unrelated.key")
	eb.change("crd:added", "New CRD", "foos.example.io")

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
	if len(f.Matches) != 1 || f.Matches[0].Subject != "webhook.config" || f.Matches[0].Kind != domain.MatchValuesKey {
		t.Errorf("matches = %+v", f.Matches)
	}
	if len(f.UpstreamEvidence) == 0 || len(f.EnvironmentEvidence) == 0 {
		t.Error("finding must cite both chains")
	}
	if e.Summary.AffectEnvironment != 1 || e.Summary.UpstreamChanges != 3 {
		t.Errorf("summary = %+v", e.Summary)
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

	removed := findingsByRule(e, RuleValuesRemoved)
	if len(removed) != 0 {
		t.Errorf("customer sets nothing under webhook.*; got %+v", removed)
	}
	pinned := findingsByRule(e, RuleValuesPinned)
	if len(pinned) != 1 || pinned[0].Matches[0].Subject != "replicaCount" {
		t.Errorf("pinned = %+v", pinned)
	}
	if pinned[0].Classification != domain.ImpactInformational {
		t.Errorf("pinning the exact key is informational: %+v", pinned[0])
	}
	adjacent := findingsByRule(e, RuleValuesAdjacent)
	if len(adjacent) != 1 || !strings.Contains(adjacent[0].Detail, "ingress.timeout") {
		t.Errorf("adjacent = %+v", adjacent)
	}
	if adjacent[0].Classification != domain.ImpactReview {
		t.Errorf("partial overlap is review: %+v", adjacent[0])
	}
	newKey := findingsByRule(e, RuleValuesNewKey)
	if len(newKey) != 1 {
		t.Fatalf("new-key = %+v", newKey)
	}
	// segment-wise matching: a.bb must not relate to a.b.c
	eb2 := newEdge()
	eb2.change("values:removed", "Helm value `a.b` removed", "a.b")
	val2 := writeFile(t, dir, "v2.yaml", "a:\n  bb: 1\n")
	e2 := buildReport(t, eb2.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val2}}))
	if n := len(e2.Findings); n != 0 {
		t.Errorf("a.bb is not under a.b; got %d findings", n)
	}
}

// --- rule 2: CRDs and APIs --------------------------------------------------------

func TestCRDVersionRemovedMatch(t *testing.T) {
	eb := newEdge()
	eb.change("crd:version-removed", "API version example.io/v1beta1 of Foo removed", "foos.example.io/v1beta1")
	eb.change("crd:version-deprecated", "API version example.io/v2 of Foo deprecated", "foos.example.io/v2")
	eb.change("crd:removed", "CRD bats.other.io removed", "bats.other.io")
	eb.change("crd:fields-removed", "Foo v1 schema: field removed: spec.oldField", "spec.oldField")
	eb.change("crd:fields-removed", "Foo v1 schema: field removed: spec.arr[].gone", "spec.arr[].gone")

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
		if f.Classification != domain.ImpactActionRequired {
			t.Errorf("classification = %s", f.Classification)
		}
		kinds := map[domain.ImpactMatchKind]bool{}
		for _, m := range f.Matches {
			kinds[m.Kind] = true
		}
		if !kinds[domain.MatchAPIVersion] || !kinds[domain.MatchCRDVersion] {
			t.Errorf("matches = %+v (want api-version and crd-version)", f.Matches)
		}
	}
	if fs := findingsByRule(e, RuleCRDVersionDeprecated); len(fs) != 1 {
		t.Errorf("deprecated findings = %d", len(fs))
	}
	if fs := findingsByRule(e, RuleCRDRemoved); len(fs) != 0 {
		t.Errorf("other.io is a group the environment never uses; got %+v", fs)
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

func TestCRDRemovedGroupOnlyMatchIsMediumConfidence(t *testing.T) {
	eb := newEdge()
	eb.change("crd:removed", "CRD foos.example.io removed", "foos.example.io")
	dir := t.TempDir()
	manifests := writeFile(t, dir, "m.yaml", "apiVersion: example.io/v1\nkind: Foo\nspec:\n  a: 1\n")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{manifests}}))
	fs := findingsByRule(e, RuleCRDRemoved)
	if len(fs) != 1 {
		t.Fatalf("findings = %+v", e.Findings)
	}
	if fs[0].Provenance.Confidence != domain.ConfidenceMedium {
		t.Errorf("group-only match must be medium confidence: %+v", fs[0].Provenance)
	}
	if fs[0].Classification != domain.ImpactActionRequired {
		t.Errorf("classification = %s", fs[0].Classification)
	}
}

// --- rule 3: Kubernetes -------------------------------------------------------------

func TestKubernetesFindings(t *testing.T) {
	eb := newEdge()
	// narrowed support: 1.28–1.30 → 1.29–1.33
	ev := eb.ev("https://example/compat.md", "1.29, 1.30, 1.31, 1.32, 1.33")
	cc := domain.CompatibilityChange{Platform: "kubernetes", Narrowed: true,
		From: &domain.CompatibilityConstraint{Platform: "kubernetes", Kind: "supported",
			Versions: []string{"1.28", "1.29", "1.30"}, Raw: "1.28-1.30",
			Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.table@v1", Confidence: domain.ConfidenceHigh}, Evidence: []domain.EvidenceID{ev}},
		To: &domain.CompatibilityConstraint{Platform: "kubernetes", Kind: "supported",
			Versions: []string{"1.29", "1.30", "1.31", "1.32", "1.33"}, Raw: "1.29-1.33",
			Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.table@v1", Confidence: domain.ConfidenceHigh}, Evidence: []domain.EvidenceID{ev}}}
	eb.edge.Compatibility = append(eb.edge.Compatibility, cc)

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
	// under you" wording
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.25"}))
	fs := findingsByRule(e, RuleKubernetesBelow)
	if len(fs) != 1 || strings.Contains(fs[0].Detail, "narrowed under you") {
		t.Errorf("1.25 detail = %q", fs[0].Detail)
	}
	e = buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.28"}))
	if !strings.Contains(findingsByRule(e, RuleKubernetesBelow)[0].Detail, "narrowed under you") {
		t.Error("1.28 was supported before and is dropped now; detail must say so")
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
	if fs := findingsByRule(e, RuleKubeVersionBlocked); len(fs) != 1 || fs[0].Classification != domain.ImpactActionRequired {
		t.Fatalf("findings = %+v", e.Findings)
	}
	e = buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.31"}))
	if fs := findingsByRule(e, RuleKubeVersionBlocked); len(fs) != 0 {
		t.Errorf("in-range kubeVersion must not block: %+v", fs)
	}
}

// --- rule 4: images -------------------------------------------------------------

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
				if f.Classification != domain.ImpactReview {
					t.Errorf("pinning the old tag = review, got %s", f.Classification)
				}
			}
		}
	}
	if !pinnedFound {
		t.Error("the pinned controller image must match the artifact move")
	}
	// already at the target tag → informational
	val2 := writeFile(t, dir, "v2.yaml", "image:\n  repository: quay.io/example/controller\n  tag: v1.1.0\n")
	e2 := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val2}}))
	for _, f := range findingsByRule(e2, RuleImageChanged) {
		if strings.Contains(f.Title, "controller") && f.Classification != domain.ImpactInformational {
			t.Errorf("already at target = informational, got %s (%s)", f.Classification, f.Title)
		}
	}
	// unrelated images never match
	val3 := writeFile(t, dir, "v3.yaml", "image:\n  repository: docker.io/library/nginx\n  tag: \"1.0\"\n")
	e3 := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val3}}))
	if n := len(e3.Findings); n != 0 {
		t.Errorf("unrelated image: %d findings", n)
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

// --- report invariants ---------------------------------------------------------

func TestBuildRejectsMissingParts(t *testing.T) {
	if _, err := Build(Input{}); err == nil {
		t.Fatal("Build requires Edge and Env")
	}
}

func TestNoEnvironmentInputYieldsNoFindings(t *testing.T) {
	eb := newEdge()
	eb.change("values:removed", "Helm value `a` removed", "a")
	e := buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: "1.31"}))
	if len(e.Findings) != 0 || e.Summary.AffectEnvironment != 0 {
		t.Fatalf("findings = %+v", e.Findings)
	}
}

func TestDeterministicOutput(t *testing.T) {
	eb := newEdge()
	eb.change("values:removed", "Helm value `a` removed", "a")
	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", "a: 1\n")
	en := loadEnv(t, env.Inputs{ValuesFiles: []string{val}})
	r1 := buildReport(t, eb.edge, en)
	r2 := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}}))
	if r1.Findings[0].ID != r2.Findings[0].ID || len(r1.Evidence) != len(r2.Evidence) {
		t.Fatal("Build must be deterministic")
	}
}
