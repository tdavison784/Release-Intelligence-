package discovery

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestScanCertLikeRepo(t *testing.T) {
	res := scanFixture(t, certFixture, certFixture.repo, ProfileSource)
	cs := res.Candidates

	ctrl := mustCand(t, cs, KindImage, "quay.io/examplecorp/certmgr-controller")
	if ctrl.Attr("classes") != "chart-appversion" || !containsStr(ctrl.Rules, "helm.values-image") {
		t.Errorf("controller image composed from imageRegistry/imageNamespace/name: %+v", ctrl)
	}
	// evidence: repo-file, blob URI pinned to the scanned tag, line locator, line excerpt
	ev := ctrl.Evidence[0]
	if ev.Kind != domain.EvidenceRepoFile || ev.URI != "https://github.com/example/certmgr/blob/v1.2.1/deploy/charts/certmgr/values.yaml" ||
		ev.Locator != "L8" || !strings.Contains(ev.Excerpt, "name: certmgr-controller") || ev.ContentDigest == "" {
		t.Errorf("unexpected evidence %+v", ev)
	}
	mustCand(t, cs, KindImage, "quay.io/examplecorp/certmgr-webhook")

	chart := mustCand(t, cs, KindHelmChart, "certmgr@deploy/charts/certmgr")
	if chart.Attr("placeholder") != "true" || chart.Attr("versionPrefix") != "v" {
		t.Errorf("placeholder chart version not detected: %v", chart.Attributes)
	}
	mustCand(t, cs, KindHelmOCI, "quay.io/examplecorp/charts/certmgr")
	rel := mustCand(t, cs, KindVersionRelation, "chart.version = {{.Tag}}")
	if !containsStr(rel.Rules, "make.helm-package") {
		t.Errorf("chart version relation should come from `helm package --version=$(VERSION)`: %v", rel.Rules)
	}
	mustCand(t, cs, KindVersionRelation, "binary.version = {{.Tag}}")
	mustCand(t, cs, KindVersionRelation, "build.VERSION = {{.Tag}}")
	mustCand(t, cs, KindReleaseAsset, "https://github.com/example/certmgr/releases/download/{{.Tag}}/certmgr.yaml")
	mustCand(t, cs, KindReleaseAsset, "https://github.com/example/certmgr/releases/download/{{.Tag}}/certmgr.crds.yaml")
	mustCand(t, cs, KindDocsRepo, "github.com/example/website")
	mustCand(t, cs, KindSecurityPolicy, "SECURITY.md")
	mustCand(t, cs, KindCRD, "deploy/crds/certmgr.example.io_widgets.yaml")
	if c, ok := findCand(cs, KindImage, "gcr.io/distroless/static-debian13"); !ok || c.Attr("classes") != tagPinned {
		t.Errorf("base image should be found as a pinned image: %+v", c)
	}
	if _, ok := findCand(cs, KindHelmChart, "sample-webhook@make/config/sample/chart"); !ok {
		t.Error("helper chart should still be a candidate (excluded later by the resolver)")
	}
}

func TestScanCertLikeWebsite(t *testing.T) {
	res := scanFixture(t, certFixture, "github.com/example/website", ProfileDocs)
	cs := res.Candidates
	notes := mustCand(t, cs, KindReleaseNotes, "content/docs/releases/release-notes/release-notes-{{.Major}}.{{.Minor}}.md")
	if notes.Attr("sectionHeadings") != "true" || notes.Attr("repo") != "github.com/example/website" || notes.Attr("ref") != "master" {
		t.Errorf("per-line notes with release sections expected: %v", notes.Attributes)
	}
	up := mustCand(t, cs, KindUpgradeGuide, "content/docs/releases/upgrading/upgrading-{{.PrevMajor}}.{{.PrevMinor}}-{{.Major}}.{{.Minor}}.md")
	if up.Attr("level") != "pair" {
		t.Errorf("upgrade guide level: %v", up.Attributes)
	}
	compat := mustCand(t, cs, KindCompatibility, "content/docs/releases/README.md")
	if compat.Attr("keyColumns") != "Release" {
		t.Errorf("the OpenShift-keyed table must not contribute key columns: %v", compat.Attributes)
	}
	if !strings.Contains(compat.Attr("testedHeaders"), "Tested Kubernetes Versions") || compat.Attr("separatorParts") != "Supported Kubernetes / OpenShift Versions=0" {
		t.Errorf("compat headers: %v", compat.Attributes)
	}
	repo := mustCand(t, cs, KindHelmRepo, "https://charts.example.io")
	if repo.Attr("charts") != "certmgr" {
		t.Errorf("helm repo should know the installed chart: %v", repo.Attributes)
	}
	mustCand(t, cs, KindReleaseAsset, "https://github.com/example/certmgr/releases/download/{{.Tag}}/certmgr.yaml")
	legacy := mustCand(t, cs, KindReleaseAsset, "https://github.com/example/certmgr/releases/download/{{.Tag}}/certmgr-legacy.yaml")
	if !strings.Contains(legacy.Attr("files"), "v1.0-docs") {
		t.Errorf("legacy asset should be attributed to versioned docs: %v", legacy.Attributes)
	}
	// evidence of a docs-repo file is pinned to the docs branch
	if !strings.HasPrefix(compat.Evidence[0].URI, "https://github.com/example/website/blob/master/") {
		t.Errorf("docs evidence URI: %s", compat.Evidence[0].URI)
	}
}

func TestScanArgoLikeRepo(t *testing.T) {
	res := scanFixture(t, argoFixture, argoFixture.repo, ProfileSource)
	cs := res.Candidates
	mustCand(t, cs, KindReleaseTrigger, "v*")
	if _, ok := findCand(cs, KindReleaseTrigger, "!v1.*"); ok {
		t.Error("negated tag patterns are not triggers")
	}
	mustCand(t, cs, KindReleasePublisher, "goreleaser")
	body := mustCand(t, cs, KindReleaseNotes, "github-release-body")
	if !strings.Contains(body.Attr("groups"), "Breaking Changes") {
		t.Errorf("changelog groups: %v", body.Attributes)
	}
	img := mustCand(t, cs, KindImage, "quay.io/example/deployer")
	classes := setOf(img.Attr("classes"))
	if !classes[tagRelease] || !setOf(img.Attr("triggers"))[triggerTag] || img.Attr("tagTemplate") != tmplTag {
		t.Errorf("release image from the tag-triggered workflow (shell defaults resolved): %v", img.Attributes)
	}
	snap := mustCand(t, cs, KindImage, "ghcr.io/example/deployer/deployer")
	if snap.Attr("classes") != tagSnapshot {
		t.Errorf("branch-triggered sha-tagged image must be a snapshot: %v", snap.Attributes)
	}
	mustCand(t, cs, KindRegistry, "quay.io")
	for _, a := range []string{"deployer-linux-amd64", "deployer-darwin-arm64", "deployer-windows-amd64.exe", "cli_checksums.txt"} {
		mustCand(t, cs, KindReleaseAsset, "https://github.com/example/deployer/releases/download/{{.Tag}}/"+a)
	}
	if _, ok := findCand(cs, KindReleaseAsset, "https://github.com/example/deployer/releases/download/{{.Tag}}/deployer-windows-arm64.exe"); ok {
		t.Error("goreleaser ignore rules must be honoured")
	}
	m := mustCand(t, cs, KindManifest, "manifests/install.yaml")
	if m.Attr("served") != "raw-at-tag" || !strings.Contains(m.Attr("images"), "quay.io/example/deployer:v3.1.2") {
		t.Errorf("install manifest: %v", m.Attributes)
	}
	ch := mustCand(t, cs, KindChangelog, "CHANGELOG.md")
	if ch.Attr("newest") != "1.4.8" {
		t.Errorf("changelog newest entry: %v", ch.Attributes)
	}
	cr := mustCand(t, cs, KindChartRepo, "github.com/example/deployer-helm")
	if cr.Attr("paths") != "charts/deployer" {
		t.Errorf("chart path: %v", cr.Attributes)
	}
	mustCand(t, cs, KindUpgradeGuide, "docs/operator-manual/upgrading/{{.PrevMajor}}.{{.PrevMinor}}-{{.Major}}.{{.Minor}}.md")
	mustCand(t, cs, KindCompatibility, "docs/operator-manual/tested-kubernetes-versions.md")
	mustCand(t, cs, KindAdvisories, "example/deployer")
	mustCand(t, cs, KindCRD, "manifests/crds/app-crd.yaml")
	mustCand(t, cs, KindVersionRelation, "VERSION file = {{.Version}}")
}

func TestScanIstioLikeRepo(t *testing.T) {
	res := scanFixture(t, istioFixture, istioFixture.repo, ProfileSource)
	cs := res.Candidates
	for _, n := range []string{"proxy", "meshd"} {
		c := mustCand(t, cs, KindImageName, n)
		if c.Confidence == domain.ConfidenceLow {
			t.Errorf("%s should be a production image: %+v", n, c)
		}
	}
	if c := mustCand(t, cs, KindImageName, "app"); c.Confidence != domain.ConfidenceLow {
		t.Errorf("test image should be low confidence: %+v", c)
	}
	if _, ok := findCand(cs, KindImageName, "base"); ok {
		t.Error("base images are not product images")
	}
	if _, ok := findCand(cs, KindImageName, "helloworld"); ok {
		t.Error("example entries are not product images")
	}
	hub := mustCand(t, cs, KindRegistry, "docker.io/meshproj")
	if hub.Attr("variable") != "HUB" || hub.Attr("implicitDockerHub") != "true" {
		t.Errorf("HUB ?= meshproj means Docker Hub: %v", hub.Attributes)
	}
	mustCand(t, cs, KindRegistry, "ghcr.io/meshproj/testing")
	if _, ok := findCand(cs, KindImage, "ghcr.io/meshproj/testing"); ok {
		t.Error("a registry variable assignment is not an image")
	}
	dir := mustCand(t, cs, KindNotesDir, "releasenotes/notes")
	if dir.Confidence != domain.ConfidenceHigh || !strings.Contains(dir.Attr("docs"), "releasenotes/template.yaml") {
		t.Errorf("structured notes dir: %+v", dir)
	}
	base := mustCand(t, cs, KindHelmChart, "base@manifests/charts/base")
	if base.Attr("placeholder") != "true" {
		t.Errorf("'never actually shipped' comment marks a placeholder: %v", base.Attributes)
	}
	repo := mustCand(t, cs, KindHelmRepo, "https://mesh-release.example.com/charts")
	if !setOf(repo.Attr("charts"))["meshd"] || !setOf(repo.Attr("charts"))["base"] {
		t.Errorf("helm repo charts: %v", repo.Attributes)
	}
	mustCand(t, cs, KindHelmOCI, "registry.mesh.example/release/charts")
	asset := mustCand(t, cs, KindReleaseAsset, "https://github.com/meshproj/mesh/releases/download/{{.Tag}}/mesh-{{.Version}}-osx.tar.gz")
	if asset.Attr("unresolved") != "" {
		t.Errorf("OSEXT resolves to its first literal value: %v", asset.Attributes)
	}
	mustCand(t, cs, KindDocsRepo, "github.com/meshproj/mesh.io")
	mustCand(t, cs, KindSecurityPolicy, ".github/SECURITY.md")
	mustCand(t, cs, KindVersionRelation, "VERSION file = {{.Major}}.{{.Minor}}")
}

func TestScanIstioLikeDocs(t *testing.T) {
	res := scanFixture(t, istioFixture, "github.com/meshproj/mesh.io", ProfileDocs)
	cs := res.Candidates
	patch := mustCand(t, cs, KindReleaseNotes, "content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Version}}/index.md")
	if patch.Attr("level") != "patch" || patch.Attr("zeroPatch") == "true" {
		t.Errorf("patch notes: %v", patch.Attributes)
	}
	mustCand(t, cs, KindReleaseNotes, "content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Major}}.{{.Minor}}/change-notes/index.md")
	mustCand(t, cs, KindUpgradeGuide, "content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Major}}.{{.Minor}}/upgrade-notes/index.md")
	for _, c := range cs {
		if strings.Contains(c.Value, "content/zh/") || strings.Contains(c.Attr("newest"), "content/zh/") {
			t.Errorf("localized copies must be ignored: %s", c.Value)
		}
	}
	compat := mustCand(t, cs, KindCompatibility, "data/compatibility/supportStatus.yml")
	if compat.Attr("format") != "yaml-records" || compat.Attr("keyColumns") != "version" || compat.Attr("testedHeaders") != "testedK8sVersions" {
		t.Errorf("yaml support matrix: %v", compat.Attributes)
	}
	b := mustCand(t, cs, KindSecurityDocs, "content/en/news/security")
	if b.Attr("entries") != "6" {
		t.Errorf("bulletins: %v", b.Attributes)
	}
	mustCand(t, cs, KindHelmOCI, "ghcr.io/meshproj/release/charts/ambient")
}

func TestPathContexts(t *testing.T) {
	cases := map[string]string{
		"make/e2e-setup.mk":                ctxTest,
		"test/e2e/values.yaml":             ctxTest,
		"samples/bookinfo/x.yaml":          ctxSample,
		"vendor/x/y.go":                    ctxVendor,
		"manifests/dev-tilt/ui.yaml":       ctxTest,
		"docs/operator-manual/upgrade.md":  ctxDocs,
		"deploy/charts/x/values.yaml":      ctxNone,
		"pkg/test/echo/docker/Dockerfile":  ctxTest,
		"manifests/charts/base/Chart.yaml": ctxNone,
	}
	for p, want := range cases {
		if got := pathContext(p); got != want {
			t.Errorf("pathContext(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestFilterLocalizations(t *testing.T) {
	in := []FileEntry{{Path: "content/en/a.md"}, {Path: "content/zh/a.md"}, {Path: "content/uk/a.md"}, {Path: "data/x.yml"}, {Path: "docs/en.md"}}
	out := filterLocalizations(in)
	var got []string
	for _, f := range out {
		got = append(got, f.Path)
	}
	if strings.Join(got, " ") != "content/en/a.md data/x.yml docs/en.md" {
		t.Fatalf("got %v", got)
	}
}
