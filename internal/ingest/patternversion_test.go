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

// patternWorld is a minimal aggregator world: one umbrella tag family whose
// release refs pin sub-components inside kustomize remote-resource URLs (the
// flux2 shape), and an OCI registry that has some of the pinned versions.
func patternWorld(t *testing.T) (*world, *catalog.ProductDefinition) {
	t.Helper()
	pub := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	w := newWorld()
	w.releases["git-tags:example.org/acme/fleet@:"] = []sources.ReleaseRef{
		{Tag: "v2.1.0", PublishedAt: &pub, Commit: "aaa111"},
		{Tag: "v2.0.0", PublishedAt: &pub, Commit: "bbb222"},
	}
	baseAt := func(source, kustomize string) string {
		return "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n" +
			"- https://github.com/acme/source-controller/releases/download/" + source + "/source-controller.crds.yaml\n" +
			"- https://github.com/acme/source-controller/releases/download/" + source + "/source-controller.deployment.yaml\n" +
			"---\napiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n" +
			"- https://github.com/acme/kustomize-controller/releases/download/" + kustomize + "/kustomize-controller.crds.yaml\n"
	}
	w.docs["repo-file:example.org/acme/fleet@v2.1.0:manifests/bases/source-controller/kustomization.yaml"] = baseAt("v1.2.0", "v1.2.0")
	w.docs["repo-file:example.org/acme/fleet@v2.0.0:manifests/bases/source-controller/kustomization.yaml"] = baseAt("v1.1.0", "v1.1.0")
	w.docs["repo-file:example.org/acme/fleet@v2.1.0:manifests/bases/kustomize-controller/kustomization.yaml"] = baseAt("v1.2.0", "v1.2.0")
	w.docs["repo-file:example.org/acme/fleet@v2.0.0:manifests/bases/kustomize-controller/kustomization.yaml"] = baseAt("v1.1.0", "v1.1.0")
	w.images["oci://ghcr.io/acme/source-controller@v1.2.0"] = "sha256:source-120"
	w.images["oci://ghcr.io/acme/source-controller@v1.1.0"] = "sha256:source-110"

	def, err := catalog.Parse([]byte(`apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: fleet
name: fleet
versioning: {scheme: semver, tagPrefix: v}
sources:
  - {id: tags, roles: [versions], locator: {kind: git-tags, repository: example.org/acme/fleet}}
artifacts:
  - id: source-controller-image
    type: container-image
    name: ghcr.io/acme/source-controller
    version:
      strategy: pattern
      pattern: 'source-controller/releases/download/(?P<version>v\d+\.\d+\.\d+)/source-controller\.crds\.yaml'
      from: {kind: repo-file, repository: example.org/acme/fleet, path: manifests/bases/source-controller/kustomization.yaml}
    channels:
      - {kind: oci, repository: ghcr.io/acme/source-controller}
  - id: kustomize-controller-image
    type: container-image
    name: ghcr.io/acme/kustomize-controller
    version:
      strategy: pattern
      pattern: 'kustomize-controller/releases/download/(?P<version>v\d+\.\d+\.\d+)/'
      from: {kind: repo-file, repository: example.org/acme/fleet, path: manifests/bases/kustomize-controller/kustomization.yaml}
    channels:
      - {kind: oci, repository: ghcr.io/acme/kustomize-controller}
`))
	if err != nil {
		t.Fatal(err)
	}
	return w, def
}

func ingestPattern(t *testing.T, semver string, setup func(w *world)) *domain.Release {
	t.Helper()
	w, def := patternWorld(t)
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

// version.strategy "pattern": the artifact version is the (?P<version>…)
// capture of a text document fetched at the release tag, with the matching
// line as evidence.
func TestIngestVersionPattern(t *testing.T) {
	rel := ingestPattern(t, "2.1.0", nil)

	sc := artifactOf(t, rel, "source-controller-image")
	if sc.Status != domain.ArtifactVerified || sc.Version != "v1.2.0" || sc.Coordinate != "ghcr.io/acme/source-controller:v1.2.0" {
		t.Fatalf("source-controller-image: %+v", sc)
	}
	if len(sc.Evidence) != 2 { // the matched line + the registry probe
		t.Fatalf("source-controller-image evidence: %d records", len(sc.Evidence))
	}
	ev, _ := evidenceByID(rel, sc.Evidence[0])
	if ev.Kind != domain.EvidenceStructured || ev.SourceID != "source-controller-image" ||
		!strings.Contains(ev.URI, "manifests/bases/source-controller/kustomization.yaml") ||
		!strings.Contains(ev.Excerpt, "releases/download/v1.2.0") {
		t.Fatalf("pattern evidence: %+v", ev)
	}
	if ev.Locator != "L4" { // the line of the pin in the kustomization
		t.Fatalf("pattern evidence locator: %q", ev.Locator)
	}
	st := statusOf(t, rel, "source-controller-image")
	if st.State != domain.SourceOK || !strings.Contains(st.Detail, "pattern capture version = v1.2.0") {
		t.Fatalf("source-controller-image status: %+v", st)
	}
}

// the pin is per release: 2.0.0 pinned 1.1.0, verified at its own registry
// tag; a controller whose registry lacks the pin is missing (a real fact).
func TestIngestVersionPatternPerRelease(t *testing.T) {
	rel := ingestPattern(t, "2.0.0", nil)
	sc := artifactOf(t, rel, "source-controller-image")
	if sc.Version != "v1.1.0" || sc.Status != domain.ArtifactVerified {
		t.Fatalf("source-controller-image at 2.0.0: %+v", sc)
	}
	kc := artifactOf(t, rel, "kustomize-controller-image")
	if kc.Version != "v1.1.0" || kc.Status != domain.ArtifactMissing {
		t.Fatalf("kustomize-controller-image at 2.0.0: %+v", kc)
	}
	if !strings.Contains(kc.Detail, "absent from ghcr.io/acme/kustomize-controller:v1.1.0") {
		t.Fatalf("missing detail: %q", kc.Detail)
	}
}

// the from document unreachable → the version is unknown, the artifact is
// expected (not missing: nothing said it is absent)
func TestIngestVersionPatternUnreachable(t *testing.T) {
	rel := ingestPattern(t, "2.1.0", func(w *world) {
		w.down[catalog.LocatorRepoFile] = true
	})
	sc := artifactOf(t, rel, "source-controller-image")
	if sc.Status != domain.ArtifactExpected || sc.Version != "" {
		t.Fatalf("source-controller-image with the document unreachable: %+v", sc)
	}
	if !strings.Contains(sc.Detail, "cannot read") {
		t.Fatalf("detail: %q", sc.Detail)
	}
}

// a release whose document has no match → missing with the document in the
// detail (that release does not pin the component)
func TestIngestVersionPatternNoMatch(t *testing.T) {
	rel := ingestPattern(t, "2.1.0", func(w *world) {
		w.docs["repo-file:example.org/acme/fleet@v2.1.0:manifests/bases/source-controller/kustomization.yaml"] =
			"apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n- account.yaml\n"
	})
	sc := artifactOf(t, rel, "source-controller-image")
	if sc.Status != domain.ArtifactMissing || !strings.Contains(sc.Detail, "does not match the version pattern") {
		t.Fatalf("source-controller-image without a match: %+v", sc)
	}
}
