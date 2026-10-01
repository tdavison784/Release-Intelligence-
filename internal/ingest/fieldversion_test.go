package ingest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// fieldWorld is a minimal monorepo world: one chart tag family, a Chart.yaml
// at each tag with pinned dependencies, and an OCI registry that has some of
// the pinned versions.
func fieldWorld(t *testing.T) (*world, *catalog.ProductDefinition) {
	t.Helper()
	pub := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	w := newWorld()
	w.releases["git-tags:example.org/acme/charts@:"] = []sources.ReleaseRef{
		{Tag: "stack-2.1.0", PublishedAt: &pub, Commit: "aaa111"},
		{Tag: "stack-2.0.0", PublishedAt: &pub, Commit: "bbb222"},
		{Tag: "other-1.0.0", PublishedAt: &pub, Commit: "ccc333"}, // sibling chart
	}
	chartAt := func(tag, ksm, grafana string) string {
		return "apiVersion: v2\nname: kube-prometheus-stack\nversion: " + tag + "\nappVersion: v0.94.1\n" +
			"dependencies:\n  - name: crds\n    version: \"0.0.0\"\n" +
			"  - name: kube-state-metrics\n    version: \"" + ksm + "\"\n" +
			"  - name: grafana\n    version: \"" + grafana + "\"\n"
	}
	w.docs["repo-file:example.org/acme/charts@stack-2.1.0:charts/stack/Chart.yaml"] = chartAt("2.1.0", "8.6.0", "13.2.7")
	w.docs["repo-file:example.org/acme/charts@stack-2.0.0:charts/stack/Chart.yaml"] = chartAt("2.0.0", "8.4.2", "13.2.5")
	// the registry has 8.6.0 but not 8.4.2 (a pin of an unpublished version)
	w.images["oci://ghcr.io/acme/charts/kube-state-metrics@8.6.0"] = "sha256:ksm-860"
	w.images["oci://ghcr.io/acme/grafana-community/grafana@13.2.7"] = "sha256:grafana-1327"

	def, err := catalog.Parse([]byte(`apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: stack
name: stack
versioning: {scheme: semver, tagPrefix: stack-}
sources:
  - {id: tags, roles: [versions], locator: {kind: git-tags, repository: example.org/acme/charts}}
artifacts:
  - id: ksm-chart
    type: helm-chart
    name: kube-state-metrics
    version:
      strategy: field
      field: dependencies[name=kube-state-metrics].version
      from: {kind: repo-file, repository: example.org/acme/charts, path: charts/stack/Chart.yaml}
    channels:
      - {kind: oci, repository: ghcr.io/acme/charts/kube-state-metrics}
  - id: grafana-chart
    type: helm-chart
    name: grafana
    version:
      strategy: field
      field: dependencies[name=grafana].version
      from: {kind: repo-file, repository: example.org/acme/charts, path: charts/stack/Chart.yaml}
    channels:
      - {kind: oci, repository: ghcr.io/acme/grafana-community/grafana}
  - id: operator-image
    type: container-image
    name: quay.io/acme/prometheus-operator
    version:
      strategy: field
      field: appVersion
      from: {kind: repo-file, repository: example.org/acme/charts, path: charts/stack/Chart.yaml}
    channels:
      - {kind: oci, repository: quay.io/acme/prometheus-operator}
    optional: true
`))
	if err != nil {
		t.Fatal(err)
	}
	return w, def
}

func ingestField(t *testing.T, semver string, setup func(w *world)) *domain.Release {
	t.Helper()
	w, def := fieldWorld(t)
	if setup != nil {
		setup(w)
	}
	ing := newTestIngester(w, &fakeParser{})
	vl := mustVersions(t, ing, def)
	rel, err := ing.IngestRelease(context.Background(), def, version(t, vl, semver), vl)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceIntegrity(t, rel)
	return rel
}

// version.strategy "field": the artifact version is the dependency pin read
// from Chart.yaml at the release tag, with the document as evidence.
func TestIngestVersionField(t *testing.T) {
	rel := ingestField(t, "2.1.0", nil)

	ksm := artifactOf(t, rel, "ksm-chart")
	if ksm.Status != domain.ArtifactVerified || ksm.Version != "8.6.0" || ksm.Coordinate != "ghcr.io/acme/charts/kube-state-metrics:8.6.0" {
		t.Fatalf("ksm-chart: %+v", ksm)
	}
	if len(ksm.Evidence) != 2 { // the Chart.yaml field + the registry probe
		t.Fatalf("ksm-chart evidence: %d records", len(ksm.Evidence))
	}
	ev, _ := evidenceByID(rel, ksm.Evidence[0])
	if ev.Kind != domain.EvidenceStructured || ev.SourceID != "ksm-chart" ||
		!strings.Contains(ev.URI, "charts/stack/Chart.yaml") || ev.Excerpt != "dependencies[name=kube-state-metrics].version: 8.6.0" {
		t.Fatalf("field evidence: %+v", ev)
	}
	if ev.Locator != "L9" { // the line of the pin in Chart.yaml
		t.Fatalf("field evidence locator: %q", ev.Locator)
	}
	st := statusOf(t, rel, "ksm-chart")
	if st.State != domain.SourceOK || !strings.Contains(st.Detail, "dependencies[name=kube-state-metrics].version = 8.6.0") {
		t.Fatalf("ksm-chart status: %+v", st)
	}

	op := artifactOf(t, rel, "operator-image")
	// the version resolves from Chart.yaml even though the registry answers
	// "absent" for it (a pin of a version this fake registry lacks)
	if op.Status != domain.ArtifactMissing || op.Version != "v0.94.1" {
		t.Fatalf("operator-image: %+v", op)
	}
	if !strings.Contains(op.Detail, "absent from quay.io/acme/prometheus-operator:v0.94.1") {
		t.Fatalf("operator-image detail: %q", op.Detail)
	}
}

// the pinned version is per release: 2.0.0 pinned another one (which the
// registry does not have → missing, a real upstream fact)
func TestIngestVersionFieldPerRelease(t *testing.T) {
	rel := ingestField(t, "2.0.0", nil)
	ksm := artifactOf(t, rel, "ksm-chart")
	if ksm.Version != "8.4.2" || ksm.Status != domain.ArtifactMissing {
		t.Fatalf("ksm-chart at 2.0.0: %+v", ksm)
	}
	if !strings.Contains(ksm.Detail, "absent from") {
		t.Fatalf("missing detail: %q", ksm.Detail)
	}
}

// the from document unreachable → the version is unknown, the artifact is
// expected (not missing: nothing said it is absent)
func TestIngestVersionFieldUnreachable(t *testing.T) {
	rel := ingestField(t, "2.1.0", func(w *world) {
		w.down[catalog.LocatorRepoFile] = true
	})
	ksm := artifactOf(t, rel, "ksm-chart")
	if ksm.Status != domain.ArtifactExpected || ksm.Version != "" {
		t.Fatalf("ksm-chart with the document unreachable: %+v", ksm)
	}
	if !strings.Contains(ksm.Detail, "cannot read") {
		t.Fatalf("detail: %q", ksm.Detail)
	}
}

// a release whose Chart.yaml has no such dependency → missing with the path
// in the detail (an artifact that release does not ship)
func TestIngestVersionFieldNoPath(t *testing.T) {
	rel := ingestField(t, "2.1.0", func(w *world) {
		w.docs["repo-file:example.org/acme/charts@stack-2.1.0:charts/stack/Chart.yaml"] =
			"apiVersion: v2\nname: kube-prometheus-stack\nversion: 2.1.0\ndependencies:\n  - name: grafana\n    version: \"13.2.7\"\n"
	})
	ksm := artifactOf(t, rel, "ksm-chart")
	if ksm.Status != domain.ArtifactMissing || !strings.Contains(ksm.Detail, "does not carry dependencies[name=kube-state-metrics].version") {
		t.Fatalf("ksm-chart without the dependency: %+v", ksm)
	}
}
