package ingest

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// fixturePackage is a published chart package for the synthetic acme chart.
func fixturePackage(rep domain.Representation, uri, values string) *sources.ChartPackage {
	return &sources.ChartPackage{
		Representation: rep,
		URI:            uri,
		Coordinate:     uri,
		Digest:         "sha256:" + strings.Repeat("a", 64),
		RetrievedAt:    fetchedAt,
		ChartDir:       "acme",
		ChartYAML:      []byte("apiVersion: v2\nname: acme\nversion: 0.5.1\nappVersion: 1.2.0\nkubeVersion: \">= 1.26.0-0\"\n"),
		Values:         []byte(values),
		CRDs: []sources.PackageFile{
			{Path: "crds/widgets.yaml", Content: []byte(packageCRD)},
		},
		Evidence: []domain.Evidence{
			domain.NewEvidence(domain.EvidenceRegistry, "", uri, "archive:acme/Chart.yaml",
				"chart package acme 0.5.1", "sha256:"+strings.Repeat("a", 64), fetchedAt).WithRepresentation(rep),
		},
	}
}

const publishedValues = "replicaCount: 2\nimage: quay.io/acme/acme-controller\nlogLevel: info\n"

// packageCRD is a complete-enough CRD for the real parser's snapshot.
const packageCRD = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.acme.example
spec:
  group: acme.example
  names:
    kind: Widget
    plural: widgets
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
`

// chartOnlyDef keeps the synthetic chart artifact but routes its contents at
// the published package (the channel fallback: no content locators).
func chartOnlyDef() *catalog.ProductDefinition {
	def := testDef()
	def.Artifacts = []catalog.Artifact{def.Artifacts[2]} // the chart
	chart := &def.Artifacts[0]
	chart.Channels = []catalog.Locator{
		{Kind: catalog.LocatorHelmRepo, URL: "https://charts.acme.example", Chart: "acme"},
	}
	chart.Contents = []catalog.Content{
		{Kind: catalog.ContentHelmValues},
		{Kind: catalog.ContentChartMetadata},
		{Kind: catalog.ContentCRDs},
	}
	return def
}

func TestIngestContentsFromChartPackage(t *testing.T) {
	w := newWorld()
	w.packages["https://charts.acme.example#acme@0.5.1"] =
		fixturePackage(domain.RepresentationChartTGZ, "https://charts.acme.example/acme-0.5.1.tgz", publishedValues)
	ing := newTestIngester(w, nil)
	def := chartOnlyDef()
	rel, err := ing.IngestRelease(t.Context(), def, domain.MustVersion("v1.2.0", "1.2.0"), nil)
	if err != nil {
		t.Fatal(err)
	}

	values := rel.Snapshot(domain.SnapshotHelmValues, "chart")
	if values == nil || values.Values.Entries["replicaCount"] != "2" || values.Values.Entries["logLevel"] != `"info"` {
		t.Fatalf("values snapshot from the package: %+v", values)
	}
	ev, _ := evidenceByID(rel, values.Evidence[0])
	if ev.Representation != domain.RepresentationChartTGZ {
		t.Errorf("values evidence representation = %q", ev.Representation)
	}
	if !strings.Contains(ev.URI, "acme-0.5.1.tgz") || !strings.Contains(ev.Locator, "values.yaml") {
		t.Errorf("values evidence must point at the package member: %+v", ev)
	}
	if ev.ContentDigest != "sha256:"+strings.Repeat("a", 64) {
		t.Errorf("values evidence digest = %q, want the archive digest", ev.ContentDigest)
	}

	// chart-metadata: kubeVersion constraint from the packaged Chart.yaml
	var kube *domain.CompatibilityConstraint
	for k := range rel.Compat {
		if rel.Compat[k].SourceID == "chart" {
			kube = &rel.Compat[k]
		}
	}
	if kube == nil || kube.Raw != ">= 1.26.0-0" {
		t.Fatalf("chart constraint from the package: %+v", kube)
	}
	kev, _ := evidenceByID(rel, kube.Evidence[0])
	if kev.Representation != domain.RepresentationChartTGZ || !strings.Contains(kev.URI, "acme-0.5.1.tgz") {
		t.Fatalf("kubeVersion evidence: %+v", kev)
	}

	// CRDs from the package's crds/ member
	crds := rel.Snapshot(domain.SnapshotCRDs, "chart")
	if crds == nil || len(crds.CRDs.CRDs) != 1 {
		t.Fatalf("crd snapshot from the package: %+v", crds)
	}

	// every snapshot fact names the representation it came from
	for _, f := range factsOf(rel, domain.FactSnapshot) {
		if f.Attributes["representation"] != string(domain.RepresentationChartTGZ) {
			t.Errorf("fact %q representation = %q", f.Statement, f.Attributes["representation"])
		}
	}
	if s := statusOf(t, rel, "chart/helm-values"); s.State != domain.SourceOK || s.Kind != catalog.LocatorHelmRepo {
		t.Fatalf("values status: %+v", s)
	}
	// the retrieval evidence (index entry + archive) is recorded
	if !hasEvidenceWith(rel, func(e domain.Evidence) bool {
		return e.Kind == domain.EvidenceRegistry && strings.Contains(e.URI, "acme-0.5.1.tgz")
	}) {
		t.Error("no archive evidence recorded")
	}
}

func hasEvidenceWith(rel *domain.Release, pred func(domain.Evidence) bool) bool {
	for _, e := range rel.Evidence {
		if pred(e) {
			return true
		}
	}
	return false
}

func TestIngestContentsFromOCIChart(t *testing.T) {
	w := newWorld()
	w.packages["oci://ghcr.io/acme/charts/acme@0.5.1"] =
		fixturePackage(domain.RepresentationOCIChart, "https://ghcr.io/v2/acme/charts/acme/manifests/sha256:bbbb", publishedValues)
	ing := newTestIngester(w, nil)
	def := chartOnlyDef()
	def.Artifacts[0].Channels = []catalog.Locator{{Kind: catalog.LocatorOCI, Repository: "ghcr.io/acme/charts/acme"}}
	def.Artifacts[0].Version = catalog.VersionRelation{Strategy: catalog.VersionTemplate, Template: "0.5.1"}
	rel, err := ing.IngestRelease(t.Context(), def, domain.MustVersion("v1.2.0", "1.2.0"), nil)
	if err != nil {
		t.Fatal(err)
	}
	values := rel.Snapshot(domain.SnapshotHelmValues, "chart")
	if values == nil {
		t.Fatal("no values snapshot from the OCI chart")
	}
	ev, _ := evidenceByID(rel, values.Evidence[0])
	if ev.Representation != domain.RepresentationOCIChart {
		t.Errorf("representation = %q, want published-oci-chart", ev.Representation)
	}
	if s := statusOf(t, rel, "chart/helm-values"); !strings.Contains(s.Detail, "published-oci-chart") {
		t.Errorf("status detail: %+v", s)
	}
}

func TestIngestContentsPackageErrors(t *testing.T) {
	// the version is not in the index: an honest not-found, not an error
	w := newWorld()
	w.packages["https://charts.acme.example#acme@9.9.9"] = fixturePackage(domain.RepresentationChartTGZ, "u", publishedValues)
	w.errs["https://charts.acme.example#acme@0.5.1"] = notFound("pkg")
	ing := newTestIngester(w, nil)
	def := chartOnlyDef()
	def.Artifacts[0].Version = catalog.VersionRelation{Strategy: catalog.VersionTemplate, Template: "0.5.1"}
	rel, err := ing.IngestRelease(t.Context(), def, domain.MustVersion("v1.2.0", "1.2.0"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := statusOf(t, rel, "chart/helm-values"); s.State != domain.SourceNotFound {
		t.Fatalf("status = %+v, want not-found", s)
	}

	// an unreachable repository is unavailable, never "missing"
	w2 := newWorld()
	w2.down[catalog.LocatorHelmRepo] = true
	ing2 := newTestIngester(w2, nil)
	rel2, err := ing2.IngestRelease(t.Context(), def, domain.MustVersion("v1.2.0", "1.2.0"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := statusOf(t, rel2, "chart/helm-values"); s.State != domain.SourceUnavailable {
		t.Fatalf("status = %+v, want unavailable", s)
	}
}

func TestIngestContentsPackageUnresolvedVersion(t *testing.T) {
	// an optional lookup artifact that is not published for this release
	// (v1.2.1 is never an appVersion): its packaged contents are not
	// applicable, never unavailable/unverifiable — the same semantic the
	// template path gives a locator whose {{.ArtifactVersion}} is unknown.
	w := newWorld()
	w.packages["https://charts.acme.example#acme@0.5.1"] =
		fixturePackage(domain.RepresentationChartTGZ, "https://charts.acme.example/acme-0.5.1.tgz", publishedValues)
	ing := newTestIngester(w, nil)
	def := chartOnlyDef()
	rel, err := ing.IngestRelease(t.Context(), def, domain.MustVersion("v1.2.1", "1.2.1"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := statusOf(t, rel, "chart"); !strings.Contains(s.Detail, "none with appVersion v1.2.1") {
		t.Fatalf("chart status = %+v, want the none-with-appVersion detail", s)
	}
	for _, id := range []string{"chart/helm-values", "chart/chart-metadata", "chart/crds"} {
		s := statusOf(t, rel, id)
		if s.State != domain.SourceSkipped || !strings.Contains(s.Detail, "artifact version") {
			t.Fatalf("%s status = %+v, want skipped with the artifact-version detail", id, s)
		}
	}
}

func TestIngestContentsCompareWithDivergence(t *testing.T) {
	// primary: source-tree values (replicaCount 1); alternate: the published
	// package rewrote replicaCount to 2 and dropped logLevel.
	w := newWorld()
	w.packages["https://charts.acme.example#acme@0.5.1"] = fixturePackage(domain.RepresentationChartTGZ,
		"https://charts.acme.example/acme-0.5.1.tgz", "replicaCount: 2\nimage: quay.io/acme/acme-controller\n")
	w.docs["repo-file:github.com/acme/helm-charts@acme-0.5.1:charts/acme/values.yaml"] =
		"replicaCount: 1\nimage: quay.io/acme/acme-controller\nlogLevel: info\n"
	ing := newTestIngester(w, nil)
	def := chartOnlyDef()
	tgz := catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: "https://charts.acme.example", Chart: "acme"}
	def.Artifacts[0].Contents = []catalog.Content{{
		Kind:        catalog.ContentHelmValues,
		Locator:     &catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: "github.com/acme/helm-charts", Ref: "acme-0.5.1", Path: "charts/acme/values.yaml"},
		CompareWith: &tgz,
	}}
	rel, err := ing.IngestRelease(t.Context(), def, domain.MustVersion("v1.2.0", "1.2.0"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// the primary representation produced the snapshot
	values := rel.Snapshot(domain.SnapshotHelmValues, "chart")
	if values == nil || values.Values.Entries["replicaCount"] != "1" {
		t.Fatalf("snapshot must keep the primary representation: %+v", values)
	}
	ev, _ := evidenceByID(rel, values.Evidence[0])
	if ev.Representation != domain.RepresentationSourceTree {
		t.Errorf("primary evidence representation = %q", ev.Representation)
	}

	// the divergence is a fact, with both representations named
	divs := factsOf(rel, domain.FactRepresentationDivergence)
	if len(divs) != 1 {
		t.Fatalf("divergence facts: %+v", divs)
	}
	d := divs[0]
	if d.Attributes["representationA"] != string(domain.RepresentationSourceTree) ||
		d.Attributes["representationB"] != string(domain.RepresentationChartTGZ) {
		t.Errorf("divergence representations: %+v", d.Attributes)
	}
	if d.Attributes["changed"] != "1" || d.Attributes["removed"] != "1" || d.Attributes["added"] != "0" {
		t.Errorf("divergence counts: %+v", d.Attributes)
	}
	if !strings.Contains(d.Statement, "replicaCount") || !strings.Contains(d.Statement, "logLevel") {
		t.Errorf("divergence statement must name the keys: %q", d.Statement)
	}
	if len(d.Evidence) == 0 {
		t.Error("divergence fact must cite the alternate side's evidence")
	}
	if s := statusOf(t, rel, "chart/helm-values"); !strings.Contains(s.Detail, "representations differ") {
		t.Errorf("status detail: %+v", s)
	}
	// only one values snapshot is captured (the comparison is not a snapshot)
	if n := len(rel.Snapshots); n != 1 {
		t.Errorf("snapshots = %d, want 1", n)
	}
}

func TestIngestContentsCompareWithAgreement(t *testing.T) {
	w := newWorld()
	w.packages["https://charts.acme.example#acme@0.5.1"] = fixturePackage(domain.RepresentationChartTGZ,
		"https://charts.acme.example/acme-0.5.1.tgz", publishedValues)
	w.docs["repo-file:github.com/acme/helm-charts@acme-0.5.1:charts/acme/values.yaml"] = publishedValues
	ing := newTestIngester(w, nil)
	def := chartOnlyDef()
	tgz := catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: "https://charts.acme.example", Chart: "acme"}
	def.Artifacts[0].Contents = []catalog.Content{{
		Kind:        catalog.ContentHelmValues,
		Locator:     &catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: "github.com/acme/helm-charts", Ref: "acme-0.5.1", Path: "charts/acme/values.yaml"},
		CompareWith: &tgz,
	}}
	rel, err := ing.IngestRelease(t.Context(), def, domain.MustVersion("v1.2.0", "1.2.0"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := factsOf(rel, domain.FactRepresentationDivergence); len(got) != 0 {
		t.Fatalf("no divergence fact expected: %+v", got)
	}
	if s := statusOf(t, rel, "chart/helm-values"); !strings.Contains(s.Detail, "representations agree") {
		t.Errorf("status detail: %+v", s)
	}
}

func TestIngestContentsCompareWithUnavailable(t *testing.T) {
	// the alternate representation cannot be read: the content still
	// succeeds, with the honest gap in its status detail.
	w := newWorld()
	w.docs["repo-file:github.com/acme/helm-charts@acme-0.5.1:charts/acme/values.yaml"] = publishedValues
	ing := newTestIngester(w, nil)
	def := chartOnlyDef()
	tgz := catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: "https://charts.acme.example", Chart: "acme"}
	def.Artifacts[0].Contents = []catalog.Content{{
		Kind:        catalog.ContentHelmValues,
		Locator:     &catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: "github.com/acme/helm-charts", Ref: "acme-0.5.1", Path: "charts/acme/values.yaml"},
		CompareWith: &tgz,
	}}
	rel, err := ing.IngestRelease(t.Context(), def, domain.MustVersion("v1.2.0", "1.2.0"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if values := rel.Snapshot(domain.SnapshotHelmValues, "chart"); values == nil {
		t.Fatal("primary snapshot missing")
	}
	if s := statusOf(t, rel, "chart/helm-values"); s.State != domain.SourceOK || !strings.Contains(s.Detail, "comparison representation not-found") {
		t.Fatalf("status: %+v", s)
	}
}

func TestIngestContentsImageManifest(t *testing.T) {
	w := newWorld()
	w.imgConfigs["oci://quay.io/acme/acme-controller@v1.2.0"] = &sources.Image{
		Repository:  "quay.io/acme/acme-controller",
		Reference:   "v1.2.0",
		Digest:      "sha256:" + strings.Repeat("c", 64),
		URI:         "https://quay.io/v2/acme/acme-controller/manifests/sha256:ccc",
		Labels:      map[string]string{"org.opencontainers.image.version": "1.2.0"},
		RetrievedAt: fetchedAt,
		Evidence: []domain.Evidence{
			domain.NewEvidence(domain.EvidenceRegistry, "", "https://quay.io/v2/acme/acme-controller/manifests/sha256:ccc",
				"manifests/v1.2.0", "digest=sha256:ccc", "sha256:"+strings.Repeat("c", 64), fetchedAt).
				WithRepresentation(domain.RepresentationRegistryManifest),
		},
	}
	ing := newTestIngester(w, nil)
	def := testDef()
	def.Artifacts = []catalog.Artifact{def.Artifacts[0]} // the controller image
	def.Artifacts[0].Contents = []catalog.Content{{Kind: catalog.ContentImageRefs}}
	rel, err := ing.IngestRelease(t.Context(), def, domain.MustVersion("v1.2.0", "1.2.0"), nil)
	if err != nil {
		t.Fatal(err)
	}
	imgs := rel.Snapshot(domain.SnapshotImages, "controller-image")
	if imgs == nil || len(imgs.Images.Images) != 1 {
		t.Fatalf("image snapshot: %+v", imgs)
	}
	ref := imgs.Images.Images[0]
	if ref.Repository != "quay.io/acme/acme-controller" || ref.Tag != "v1.2.0" || ref.Digest != "sha256:"+strings.Repeat("c", 64) {
		t.Fatalf("image ref: %+v", ref)
	}
	facts := factsOf(rel, domain.FactSnapshot)
	if len(facts) != 1 {
		t.Fatalf("snapshot facts: %+v", facts)
	}
	if facts[0].Attributes["representation"] != string(domain.RepresentationRegistryManifest) {
		t.Errorf("fact representation: %+v", facts[0].Attributes)
	}
	if facts[0].Attributes["label.org_opencontainers_image_version"] != "" || facts[0].Attributes["label.org.opencontainers.image.version"] != "1.2.0" {
		t.Errorf("label attribute: %+v", facts[0].Attributes)
	}
	if s := statusOf(t, rel, "controller-image/image-refs"); s.State != domain.SourceOK || s.Kind != catalog.LocatorOCI {
		t.Fatalf("status: %+v", s)
	}
}

func TestDiffKeyValues(t *testing.T) {
	a := map[string]string{"x": "1", "y": "2", "z": "3"}
	b := map[string]string{"x": "1", "y": "9", "w": "4"}
	d := diffKeyValues(a, b)
	if len(d.changed) != 1 || d.changed[0] != "y: 2 -> 9" {
		t.Errorf("changed: %v", d.changed)
	}
	if len(d.removed) != 1 || d.removed[0] != "z" {
		t.Errorf("removed: %v", d.removed)
	}
	if len(d.added) != 1 || d.added[0] != "w" {
		t.Errorf("added: %v", d.added)
	}
	if d.empty() || d.summary() != "+1 -1 ~1 w, y: 2 -> 9, z" {
		t.Errorf("summary: %q", d.summary())
	}
	if e := (&representationDiff{}); !e.empty() {
		t.Error("empty diff")
	}
}

// A lines content keeps the lines of a document that match its pattern
// (trimmed, de-duplicated, sorted); a capture group keeps just the group.
func TestIngestContentsLines(t *testing.T) {
	w := newWorld()
	w.docs[manifestURL("v1.2.0")] = "# acme\nimage: quay.io/acme/acme-controller:v1.2.0\n" +
		"    Note: The `Updated` field has been removed. Use `Changed` instead.\n" +
		"  Deprecated: use spec.b instead. Will be removed in v2.\n" +
		"  Note: The `Updated` field has been removed. Use `Changed` instead.\n"
	ing := newTestIngester(w, nil)
	def := testDef()
	def.Artifacts[1].Contents = []catalog.Content{
		{Kind: catalog.ContentLines, Pattern: `\bhas been removed\b|^\s*Deprecated:`, Label: "manifest"},
		{Kind: catalog.ContentLines, Pattern: `^\s*Deprecated: (.+?)\.? Will`},
	}
	rel, err := ing.IngestRelease(t.Context(), def, domain.MustVersion("v1.2.0", "1.2.0"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := rel.Snapshot(domain.SnapshotLines, "install-manifest")
	if s == nil || s.Lines == nil {
		t.Fatalf("no lines snapshot: %+v", rel.Snapshots)
	}
	want := []string{"Deprecated: use spec.b instead. Will be removed in v2.", "Note: The `Updated` field has been removed. Use `Changed` instead."}
	if got := s.Lines.Lines; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("lines = %q, want %q", got, want)
	}
	if s.Lines.Source != "manifest" || len(s.Evidence) == 0 {
		t.Errorf("source/evidence: %+v", s)
	}
}
