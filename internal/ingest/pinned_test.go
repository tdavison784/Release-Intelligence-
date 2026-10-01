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

// pinnedWorld is an aggregating chart (like kube-prometheus-stack) whose
// Chart.yaml pins an operator via appVersion, plus that operator's own
// release notes at the pinned tag: the source follows the pin with
// {{.ArtifactVersionOf "operator-image"}}.
func pinnedWorld(t *testing.T, sourceYAML string) (*world, *catalog.ProductDefinition) {
	t.Helper()
	pub := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	w := newWorld()
	w.releases["git-tags:example.org/acme/charts@:"] = []sources.ReleaseRef{
		{Tag: "stack-2.1.0", PublishedAt: &pub, Commit: "aaa111"},
		{Tag: "stack-2.0.0", PublishedAt: &pub, Commit: "bbb222"},
	}
	chartAt := func(tag, appVersion string) string {
		return "apiVersion: v2\nname: stack\nversion: " + tag + "\nappVersion: " + appVersion + "\n"
	}
	w.docs["repo-file:example.org/acme/charts@stack-2.1.0:charts/stack/Chart.yaml"] = chartAt("2.1.0", "v0.94.1")
	w.docs["repo-file:example.org/acme/charts@stack-2.0.0:charts/stack/Chart.yaml"] = chartAt("2.0.0", "v0.93.0")
	// the operator's own release bodies at the pinned tags
	w.docs["github-releases:acme/operator@v0.94.1"] = "## v0.94.1 Changes\n\n- Discard zero-value duration fields in Alertmanager resources\n"
	w.docs["github-releases:acme/operator@v0.93.0"] = "## v0.93.0 Changes\n\n- Unrelated older change\n"
	w.images["oci://quay.io/acme/prometheus-operator@v0.94.1"] = "sha256:op-0941"

	def, err := catalog.Parse([]byte(`apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: stack
name: stack
versioning: {scheme: semver, tagPrefix: stack-}
sources:
  - {id: tags, roles: [versions], locator: {kind: git-tags, repository: example.org/acme/charts}}
` + sourceYAML + `
artifacts:
  - id: operator-image
    type: container-image
    name: quay.io/acme/prometheus-operator
    version:
      strategy: field
      field: appVersion
      from: {kind: repo-file, repository: example.org/acme/charts, path: charts/stack/Chart.yaml}
    channels:
      - {kind: oci, repository: quay.io/acme/prometheus-operator}
`))
	if err != nil {
		t.Fatal(err)
	}
	if rep := catalog.Validate(def); !rep.OK() {
		t.Fatalf("definition invalid: %v", rep.Issues)
	}
	return w, def
}

// the source that follows the pin
const followSource = `  - id: operator-notes
    roles: [release-notes]
    locator:
      kind: github-releases
      repository: acme/operator
      ref: '{{.ArtifactVersionOf "operator-image"}}'`

func ingestPinned(t *testing.T, sourceYAML, semver string, setup func(w *world)) (*domain.Release, *fakeParser) {
	t.Helper()
	w, def := pinnedWorld(t, sourceYAML)
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
	return rel, ing.Parser.(*fakeParser)
}

// A source locator rendered with {{.ArtifactVersionOf}} addresses the pinned
// component's own channel at the pinned tag: the operator's release body for
// v0.94.1 (the pin this release's Chart.yaml carries), not the release tag.
func TestIngestSourceFollowsPinnedVersion(t *testing.T) {
	rel, fp := ingestPinned(t, followSource, "2.1.0", nil)

	st := statusOf(t, rel, "operator-notes")
	if st.State != domain.SourceOK {
		t.Fatalf("operator-notes: %+v", st)
	}
	inputs := fp.notesFor("operator-notes")
	if len(inputs) != 1 || !strings.Contains(inputs[0].URI, "github.com/acme/operator/releases/tag/v0.94.1") &&
		!strings.Contains(inputs[0].URI, "v0.94.1") {
		t.Fatalf("notes parsed from the wrong document: %+v", inputs)
	}
	if !strings.Contains(string(inputs[0].Content), "zero-value duration") {
		t.Fatalf("expected the v0.94.1 body, got %q", inputs[0].Content)
	}
	// the pin is per release: the same source at 2.0.0 reads v0.93.0
	rel20, fp20 := ingestPinned(t, followSource, "2.0.0", nil)
	if st := statusOf(t, rel20, "operator-notes"); st.State != domain.SourceOK {
		t.Fatalf("operator-notes at 2.0.0: %+v", st)
	}
	if in := fp20.notesFor("operator-notes"); len(in) != 1 || !strings.Contains(string(in[0].Content), "older change") {
		t.Fatalf("expected the v0.93.0 body, got %+v", in)
	}
}

// The followed artifact is resolved once per release: the resolution status
// (the Chart.yaml read) and its evidence appear exactly once even though the
// source forced an early resolution.
func TestIngestPinnedResolutionRecordedOnce(t *testing.T) {
	rel, _ := ingestPinned(t, followSource, "2.1.0", nil)

	op := artifactOf(t, rel, "operator-image")
	if op.Status != domain.ArtifactVerified || op.Version != "v0.94.1" {
		t.Fatalf("operator-image: %+v", op)
	}
	n := 0
	for _, st := range statusesOf(rel, "operator-image") {
		if strings.Contains(st.Detail, "appVersion = v0.94.1") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("resolution status recorded %d times: %+v", n, statusesOf(rel, "operator-image"))
	}
	pins := 0
	for _, ev := range rel.Evidence {
		if ev.Kind == domain.EvidenceStructured && ev.Excerpt == "appVersion: v0.94.1" {
			pins++
		}
	}
	if pins != 1 {
		t.Fatalf("pin evidence recorded %d times", pins)
	}
}

// The pin document unreachable → the version is unknown → the following
// source is unavailable (it cannot address the component's channel), never a
// crash and never a half-rendered locator.
func TestIngestSourceFollowUnreachablePin(t *testing.T) {
	rel, fp := ingestPinned(t, followSource, "2.1.0", func(w *world) {
		w.down[catalog.LocatorRepoFile] = true
	})
	st := statusOf(t, rel, "operator-notes")
	if st.State != domain.SourceUnavailable {
		t.Fatalf("operator-notes with the pin unreachable: %+v", st)
	}
	if !strings.Contains(st.Detail, "cannot follow artifact operator-image") {
		t.Fatalf("detail: %q", st.Detail)
	}
	if len(fp.notesFor("operator-notes")) != 0 {
		t.Fatal("notes parsed although the pin did not resolve")
	}
}

// A release whose Chart.yaml carries no pin → the artifact is missing and the
// following source is not-found (the component has no release at any version
// for this release).
func TestIngestSourceFollowMissingPin(t *testing.T) {
	rel, _ := ingestPinned(t, followSource, "2.1.0", func(w *world) {
		w.docs["repo-file:example.org/acme/charts@stack-2.1.0:charts/stack/Chart.yaml"] =
			"apiVersion: v2\nname: stack\nversion: 2.1.0\n"
	})
	if st := statusOf(t, rel, "operator-notes"); st.State != domain.SourceNotFound {
		t.Fatalf("operator-notes without a pin: %+v", st)
	}
	if op := artifactOf(t, rel, "operator-image"); op.Status != domain.ArtifactMissing {
		t.Fatalf("operator-image: %+v", op)
	}
}

// The followed artifact outside its availability window → the source is
// skipped exactly like the artifact (not applicable to this release).
func TestIngestSourceFollowNotApplicablePin(t *testing.T) {
	src := `  - id: operator-notes
    roles: [release-notes]
    availability: ">= 99.0.0"
    locator:
      kind: github-releases
      repository: acme/operator
      ref: '{{.ArtifactVersionOf "operator-image"}}'`
	rel, _ := ingestPinned(t, src, "2.1.0", nil)
	if st := statusOf(t, rel, "operator-notes"); st.State != domain.SourceSkipped {
		t.Fatalf("operator-notes outside availability: %+v", st)
	}
}

// An extract template can follow the pin too (the heading selects the pinned
// version's section), composing it with a template function via assignment.
func TestIngestSourceFollowsInExtract(t *testing.T) {
	src := `  - id: operator-notes
    roles: [release-notes]
    locator:
      kind: repo-file
      repository: example.org/acme/charts
      ref: stack-2.1.0
      path: docs/operator-notes.md
    extract:
      type: markdown-section
      heading: '{{$v := .ArtifactVersionOf "operator-image"}}^operator {{regexQuote $v}}$'`
	rel, fp := ingestPinned(t, src, "2.1.0", func(w *world) {
		w.docs["repo-file:example.org/acme/charts@stack-2.1.0:docs/operator-notes.md"] =
			"# operator notes\n\n## operator v0.93.0\n\n- stale\n\n## operator v0.94.1\n\n- Discard zero-value duration fields\n"
	})
	if st := statusOf(t, rel, "operator-notes"); st.State != domain.SourceOK {
		t.Fatalf("operator-notes: %+v", st)
	}
	in := fp.notesFor("operator-notes")
	if len(in) != 1 || !strings.Contains(string(in[0].Content), "zero-value duration") {
		t.Fatalf("expected only the pinned section, got %+v", in)
	}
}

// A followed artifact with strategy template resolves without any document:
// the source follows the templated version.
func TestIngestSourceFollowsTemplatedArtifact(t *testing.T) {
	src := followSource + "\n"
	def, err := catalog.Parse([]byte(`apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: stack
name: stack
versioning: {scheme: semver, tagPrefix: stack-}
sources:
  - {id: tags, roles: [versions], locator: {kind: git-tags, repository: example.org/acme/charts}}
` + src + `artifacts:
  - id: operator-image
    type: container-image
    name: quay.io/acme/prometheus-operator
    version: {strategy: template, template: "v0.94.{{.Patch}}"}
    channels:
      - {kind: oci, repository: quay.io/acme/prometheus-operator}
`))
	if err != nil {
		t.Fatal(err)
	}
	w := newWorld()
	w.releases["git-tags:example.org/acme/charts@:"] = w.releases["git-tags:example.org/acme/charts@:"][:0]
	w.releases["git-tags:example.org/acme/charts@:"] = []sources.ReleaseRef{{Tag: "stack-2.1.3"}}
	w.docs["github-releases:acme/operator@v0.94.3"] = "## Changes\n\n- templated pin change\n"
	ing := newTestIngester(w, &fakeParser{})
	vl := mustVersions(t, ing, def)
	rel, err := ing.IngestRelease(context.Background(), def, version(t, vl, "2.1.3"), vl)
	if err != nil {
		t.Fatal(err)
	}
	if st := statusOf(t, rel, "operator-notes"); st.State != domain.SourceOK {
		t.Fatalf("operator-notes over a template pin: %+v", st)
	}
}
