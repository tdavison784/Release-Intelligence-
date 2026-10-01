package discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// renderPath renders a repo-file path template for a release and checks the
// file exists in the fixture tree — the templates must point at real files.
func renderPath(t *testing.T, f fixture, ta *TagAnalysis, l catalog.Locator, tag string) string {
	t.Helper()
	v, ok := ta.Find(tag)
	if !ok {
		t.Fatalf("unknown tag %s", tag)
	}
	rc := catalog.NewRenderContext("x", v, ta.Stable())
	rl, err := catalog.RenderLocator(l, rc)
	if err != nil {
		t.Fatal(err)
	}
	dir, ok := f.dirs[rl.Repository]
	if !ok {
		t.Fatalf("repository %s not in fixture", rl.Repository)
	}
	if _, err := os.Stat(filepath.Join(dir, rl.Path)); err != nil {
		t.Errorf("rendered %s for %s does not exist: %v", rl.Path, tag, err)
	}
	return rl.Path
}

func TestPipelineCertLike(t *testing.T) {
	res := runFixture(t, certFixture, nil, Request{})
	def, ta := res.Definition, certFixture.tagAnalysis(t)
	if def.Versioning.TagPrefix != "v" || def.Versioning.TagPattern != "" || def.Versioning.Lineage != catalog.LineageMinor {
		t.Errorf("versioning: %+v", def.Versioning)
	}
	notes := source(t, def, "release-notes-docs")
	if notes.Locator.Repository != "github.com/example/website" || notes.Locator.Ref != "master" ||
		notes.Extract == nil || notes.Extract.Type != catalog.ExtractMarkdownSection || len(notes.ReleaseKinds) != 0 {
		t.Errorf("release notes source: %+v", notes)
	}
	if p := renderPath(t, certFixture, ta, notes.Locator, "v1.1.2"); p != "content/docs/releases/release-notes/release-notes-1.1.md" {
		t.Errorf("rendered notes path %s", p)
	}
	up := source(t, def, "upgrade-guide")
	if p := renderPath(t, certFixture, ta, up.Locator, "v1.2.0"); p != "content/docs/releases/upgrading/upgrading-1.1-1.2.md" {
		t.Errorf("rendered upgrade path %s", p)
	}
	compat := source(t, def, "compatibility")
	if compat.Extract.Type != catalog.ExtractMarkdownTable || strings.Join(compat.Extract.KeyColumns, ",") != "Release" || len(compat.Extract.Columns) != 3 {
		t.Errorf("compat extract: %+v", compat.Extract)
	}
	source(t, def, "advisories")
	if _, ok := def.Source("changelog"); ok {
		t.Error("no changelog exists in this fixture")
	}

	chart := artifact(t, def, "certmgr-chart")
	if chart.Version.Template != tmplTag || chart.Channels[0].Kind != catalog.LocatorOCI || chart.Channels[0].Repository != "quay.io/examplecorp/charts/certmgr" ||
		chart.Channels[1].Kind != catalog.LocatorHelmRepo || chart.Channels[1].URL != "https://charts.example.io" {
		t.Errorf("chart: %+v", chart)
	}
	for _, id := range []string{"certmgr-controller", "certmgr-webhook"} {
		a := artifact(t, def, id)
		if a.Type != domain.ArtifactContainerImage || a.Version.Template != tmplTag || a.Channels[0].Repository != "quay.io/examplecorp/"+id {
			t.Errorf("image %s: %+v", id, a)
		}
	}
	if a := artifact(t, def, "certmgr-crds"); a.Type != domain.ArtifactCRD || a.Channels[0].URL != "https://github.com/example/certmgr/releases/download/{{.Tag}}/certmgr.crds.yaml" {
		t.Errorf("crd asset: %+v", a)
	}
	if a := artifact(t, def, "certmgr-manifest"); a.Type != domain.ArtifactManifest || a.Contents[0].Kind != catalog.ContentImageRefs {
		t.Errorf("manifest asset: %+v", a)
	}
	for _, a := range def.Artifacts {
		if strings.Contains(a.Name, "sample") || strings.Contains(a.Name, "distroless") || strings.Contains(a.Name, "vault") || strings.Contains(a.Name, "legacy") {
			t.Errorf("unexpected artifact %s", a.ID)
		}
	}
	if def.Provenance == nil || def.Provenance.Method != "discovery" || len(def.Provenance.ValidatedReleases) != 0 || !strings.Contains(def.Provenance.Notes, "NOT validated") {
		t.Errorf("provenance: %+v", def.Provenance)
	}

	rep := res.Report
	for _, c := range rep.Coverage {
		want := "found"
		if c.Target == TargetChangelog {
			want = "not-found"
		}
		if c.Status != want {
			t.Errorf("coverage %s = %s, want %s", c.Target, c.Status, want)
		}
	}
	if len(rep.Scans) != 2 || rep.Scans[1].Tree.Repo.String() != "github.com/example/website" {
		t.Errorf("the referenced docs repository should be scanned: %+v", rep.Scans)
	}
	for _, d := range rep.Decisions {
		if d.Rule == "" || d.Provenance.Producer == "" || d.Provenance.Validate() != nil {
			t.Errorf("decision without rule/provenance: %+v", d)
		}
	}
	md := rep.Markdown()
	for _, want := range []string{"# Discovery report: certmgr", "## Coverage", "## Proposed definition", "## Decisions", "helm.version-from-build", "## Candidates"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
	if _, err := json.Marshal(rep); err != nil {
		t.Errorf("report must be JSON serialisable: %v", err)
	}
}

func TestPipelineArgoLike(t *testing.T) {
	res := runFixture(t, argoFixture, nil, Request{})
	def, ta := res.Definition, argoFixture.tagAnalysis(t)
	if def.Versioning.TagPattern != `^v(?P<version>\d+\.\d+\.\d+(?:-(?:rc\d+))?)$` {
		t.Errorf("strict tag pattern expected (junk v2.1.2-hf1): %q", def.Versioning.TagPattern)
	}
	notes := source(t, def, "release-notes")
	if notes.Locator.Kind != catalog.LocatorGitHubReleases || notes.Locator.Ref != tmplTag || notes.Priority != 0 {
		t.Errorf("generated release body: %+v", notes)
	}
	if _, ok := def.Source("changelog"); ok {
		t.Error("stale CHANGELOG.md must be excluded")
	}
	if !hasDecision(res.Report, "changelog.stale") {
		t.Error("stale changelog decision missing")
	}
	up := source(t, def, "upgrade-guide")
	if up.Locator.Ref != "" || up.Locator.Repository != "github.com/example/deployer" {
		t.Errorf("in-repo upgrade docs are read at the tag: %+v", up.Locator)
	}
	// 3.0's previous line is 2.9 (not 3.-1): the template uses the previous existing line
	if p := renderPath(t, argoFixture, ta, up.Locator, "v3.0.0"); p != "docs/operator-manual/upgrading/2.9-3.0.md" {
		t.Errorf("rendered %s", p)
	}
	compat := source(t, def, "compatibility")
	if compat.Extract.Columns[0].Kind != "tested" {
		t.Errorf("tested-kubernetes-versions means tested: %+v", compat.Extract.Columns)
	}
	img := artifact(t, def, "deployer")
	if len(img.Channels) != 1 || img.Channels[0].Repository != "quay.io/example/deployer" || img.Version.Template != tmplTag {
		t.Errorf("release image (no ghcr snapshot channel): %+v", img)
	}
	if len(img.References) != 2 || img.References[0].Pattern != "quay.io/example/deployer:{{.Tag}}" {
		t.Errorf("image should be cross-referenced by the install manifests: %+v", img.References)
	}
	chart := artifact(t, def, "deployer-chart")
	if chart.Version.Strategy != catalog.VersionLookup || chart.Version.Field != "appVersion" || chart.Version.Match != tmplTag {
		t.Errorf("external chart lookup: %+v", chart.Version)
	}
	if g := chart.Channels[1]; g.Kind != catalog.LocatorHelmGit || g.TagPattern != `^deployer-(?P<version>\d+\.\d+\.\d+)$` || g.Path != "charts/deployer" {
		t.Errorf("helm-git channel: %+v", g)
	}
	if c := artifact(t, def, "crds"); c.Channels[0].Kind != catalog.LocatorRepoDir || c.Channels[0].Glob != "*-crd.yaml" {
		t.Errorf("crds: %+v", c)
	}
	if b := artifact(t, def, "deployer-binary"); b.Channels[0].URL != "https://github.com/example/deployer/releases/download/{{.Tag}}/deployer-linux-amd64" {
		t.Errorf("binary: %+v", b)
	}
	artifact(t, def, "manifests-install")
	for _, a := range def.Artifacts {
		if strings.Contains(a.ID, "ci-builder") || strings.Contains(a.ID, "base") {
			t.Errorf("dev image / non-release manifest included: %s", a.ID)
		}
	}
	if !hasDecision(res.Report, "image.dev-registry") && !hasDecision(res.Report, "image.non-release-tags") {
		t.Error("the snapshot image exclusion must be recorded")
	}
}

func TestPipelineIstioLike(t *testing.T) {
	res := runFixture(t, istioFixture, nil, Request{})
	def, ta := res.Definition, istioFixture.tagAnalysis(t)
	if def.Versioning.TagPrefix != "" || !strings.Contains(def.Versioning.TagPattern, `rc\.\d+`) {
		t.Errorf("bare tags with a strict pattern (junk snapshot tags): %+v", def.Versioning)
	}
	patch := source(t, def, "release-notes-docs")
	if strings.Join(patch.ReleaseKinds, ",") != "patch" || strings.HasPrefix(patch.Locator.Path, "archive/") {
		t.Errorf("patch notes must come from the source docs, not the HTML archive: %+v", patch)
	}
	if p := renderPath(t, istioFixture, ta, patch.Locator, "1.30.2"); p != "content/en/news/releases/1.30.x/announcing-1.30.2/index.md" {
		t.Errorf("rendered %s", p)
	}
	line := source(t, def, "release-notes-line")
	if strings.Join(line.ReleaseKinds, ",") != "minor,major" {
		t.Errorf("minor notes: %+v", line)
	}
	renderPath(t, istioFixture, ta, line.Locator, "1.31.0")
	files := source(t, def, "release-note-files")
	if files.Locator.Kind != catalog.LocatorRepoDir || files.Locator.BaseRef != "{{.PrevTag}}" || files.Extract.Type != catalog.ExtractReleaseNoteYAML || !files.Fallback {
		t.Errorf("structured notes: %+v", files)
	}
	up := source(t, def, "upgrade-guide")
	renderPath(t, istioFixture, ta, up.Locator, "1.30.0")
	if c := source(t, def, "compatibility"); c.Extract.Type != catalog.ExtractYAMLRecords {
		t.Errorf("compat: %+v", c.Extract)
	}
	for _, id := range []string{"base-chart", "meshd-chart"} {
		c := artifact(t, def, id)
		if c.Version.Template != tmplVersion || len(c.Channels) != 2 {
			t.Errorf("%s: %+v", id, c)
		}
	}
	if _, ok := def.Artifact("mesh-egress-chart"); ok {
		t.Error("unpublished chart must be excluded")
	}
	for _, id := range []string{"proxy", "meshd"} {
		img := artifact(t, def, id)
		if img.Channels[0].Repository != "docker.io/meshproj/"+id {
			t.Errorf("image %s should be published to the HUB registry first: %+v", id, img.Channels)
		}
		for _, ch := range img.Channels {
			if strings.Contains(ch.Repository, "testing") {
				t.Errorf("testing registry used: %s", ch.Repository)
			}
		}
	}
	if _, ok := def.Artifact("app"); ok {
		t.Error("test image included")
	}
	if a := artifact(t, def, "mesh-binary"); a.Channels[0].URL != "https://github.com/meshproj/mesh/releases/download/{{.Tag}}/mesh-{{.Version}}-osx.tar.gz" {
		t.Errorf("binary: %+v", a)
	}
	found := false
	for _, o := range res.Report.Open {
		if strings.Contains(o, "Security bulletins live in github.com/meshproj/mesh.io:content/en/news/security") {
			found = true
		}
	}
	if !found {
		t.Errorf("bulletins should be an open question: %v", res.Report.Open)
	}
}

func TestPipelineLocalDirWithoutTags(t *testing.T) {
	lc := &LocalCheckout{Dirs: map[string]string{}, Clock: clock}
	d := &Discoverer{Checkout: lc, Tags: lc, Clock: clock}
	res, err := d.Run(context.Background(), Request{Repository: "example/certmgr", LocalDir: "testdata/certlike", NoFollow: true})
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.Validate(res.Definition).OK() {
		t.Fatalf("invalid definition:\n%s", res.YAML)
	}
	if len(res.Report.Errors) == 0 {
		t.Error("missing tags should be reported")
	}
	if !strings.HasPrefix(res.Report.Scans[0].Tree.Repo.String(), "github.com/example/certmgr") {
		t.Errorf("scan: %+v", res.Report.Scans[0].Tree)
	}
}

func hasDecision(r *Report, rule string) bool {
	for _, d := range r.Decisions {
		if d.Rule == rule {
			return true
		}
	}
	return false
}
