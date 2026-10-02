package ingest

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/normalize"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

var (
	testNow   = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	fetchedAt = time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
)

// ---------------------------------------------------------------------------
// Synthetic product definition (shaped after cert-manager / Argo CD).

// withFallbackGroup puts a source into a fallback group.
func withFallbackGroup(s catalog.Source, group string) catalog.Source {
	s.FallbackGroup = group
	return s
}

func hasExplicitFallbackGroups() bool { return true }

const website = "github.com/acme/website"

func testDef() *catalog.ProductDefinition {
	roles := func(r ...domain.SourceRole) []domain.SourceRole { return r }
	tmpl := func(t string) catalog.VersionRelation {
		return catalog.VersionRelation{Strategy: catalog.VersionTemplate, Template: t}
	}
	return &catalog.ProductDefinition{
		APIVersion: catalog.APIVersion,
		Kind:       catalog.Kind,
		ID:         "acme",
		Name:       "Acme Operator",
		Versioning: catalog.Versioning{Scheme: domain.SchemeSemver, TagPrefix: "v", Lineage: catalog.LineageMinor},
		Sources: []catalog.Source{
			{ID: "gh-releases", Roles: roles(domain.RoleVersions),
				Locator: catalog.Locator{Kind: catalog.LocatorGitHubReleases, Repository: "acme/operator"}},
			{ID: "git-tags", Roles: roles(domain.RoleVersions), Priority: 1,
				Locator: catalog.Locator{Kind: catalog.LocatorGitTags, Repository: "github.com/acme/operator",
					TagPattern: `^v?(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?)$`}},
			withFallbackGroup(catalog.Source{ID: "site-notes", Roles: roles(domain.RoleReleaseNotes),
				Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "main",
					Path: "content/release-notes/release-notes-{{.Line}}.md"},
				Extract:  &catalog.Extract{Type: catalog.ExtractMarkdownSection, Heading: `^v{{regexQuote .Version}}$`},
				Classify: []catalog.ClassifyRule{{Section: "(?i)breaking", Category: domain.CategoryRemoval}},
			}, "release-notes"),
			withFallbackGroup(catalog.Source{ID: "gh-notes", Roles: roles(domain.RoleReleaseNotes), Priority: 1,
				Locator: catalog.Locator{Kind: catalog.LocatorGitHubReleases, Repository: "acme/operator", Ref: "{{.Tag}}"},
			}, "release-notes"),
			{ID: "upgrade-guide", Roles: roles(domain.RoleUpgradeGuide), ReleaseKinds: []string{"minor", "major"},
				Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "main",
					Path: "content/upgrading/upgrading-{{.PrevLine}}-{{.Line}}.md"}},
			{ID: "support-matrix", Roles: roles(domain.RoleCompatibility),
				Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "main", Path: "content/releases/README.md"},
				Extract: &catalog.Extract{Type: catalog.ExtractMarkdownTable, KeyColumns: []string{"Release"},
					KeyMatch: `^{{regexQuote .Line}}$`,
					Columns:  []catalog.ColumnSpec{{Platform: "kubernetes", Headers: []string{"Supported Kubernetes versions"}}}}},
			{ID: "commit-log", Roles: roles(domain.RoleChangelog),
				Locator: catalog.Locator{Kind: catalog.LocatorGitLog, Repository: "github.com/acme/operator", Ref: "{{.PrevTag}}..{{.Tag}}"}},
			{ID: "legacy-changelog", Roles: roles(domain.RoleChangelog), Availability: "< 1.0.0",
				Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: "github.com/acme/operator", Path: "CHANGELOG.md"}},
			{ID: "advisories", Roles: roles(domain.RoleSecurity),
				Locator: catalog.Locator{Kind: catalog.LocatorGitHubAdvisories, Repository: "acme/operator"}},
			{ID: "bulletins", Roles: roles(domain.RoleSecurity),
				Locator: catalog.Locator{Kind: catalog.LocatorHTTP, URL: "https://acme.example/security/index.md"}},
		},
		Artifacts: []catalog.Artifact{
			{ID: "controller-image", Type: domain.ArtifactContainerImage, Name: "acme-controller", Version: tmpl("{{.Tag}}"),
				Channels:   []catalog.Locator{{Kind: catalog.LocatorOCI, Repository: "quay.io/acme/acme-controller"}},
				References: []catalog.ArtifactReference{{Artifact: "install-manifest", Pattern: "quay.io/acme/acme-controller:{{.Tag}}"}}},
			{ID: "install-manifest", Type: domain.ArtifactManifest, Name: "acme.yaml", Version: tmpl("{{.Tag}}"),
				Channels: []catalog.Locator{{Kind: catalog.LocatorHTTP, URL: "https://github.com/acme/operator/releases/download/{{.Tag}}/acme.yaml"}},
				Contents: []catalog.Content{{Kind: catalog.ContentImageRefs}}},
			{ID: "chart", Type: domain.ArtifactHelmChart, Name: "acme",
				Version: catalog.VersionRelation{Strategy: catalog.VersionLookup, Field: "appVersion", Match: "{{.Tag}}"},
				Channels: []catalog.Locator{
					{Kind: catalog.LocatorHelmRepo, URL: "https://charts.acme.example", Chart: "acme"},
					{Kind: catalog.LocatorOCI, Repository: "ghcr.io/acme/charts/acme"},
				},
				Contents: []catalog.Content{
					{Kind: catalog.ContentHelmValues, Locator: &catalog.Locator{Kind: catalog.LocatorRepoFile,
						Repository: "github.com/acme/helm-charts", Ref: "acme-{{.ArtifactVersion}}", Path: "charts/acme/values.yaml"}},
					{Kind: catalog.ContentChartMetadata, Locator: &catalog.Locator{Kind: catalog.LocatorRepoFile,
						Repository: "github.com/acme/helm-charts", Ref: "acme-{{.ArtifactVersion}}", Path: "charts/acme/Chart.yaml"}},
				}},
			{ID: "crds", Type: domain.ArtifactCRD, Name: "acme CRDs", Version: tmpl("{{.Tag}}"), Availability: ">= 1.1.0",
				Channels: []catalog.Locator{{Kind: catalog.LocatorRepoDir, Repository: "github.com/acme/operator", Path: "deploy/crds", Glob: "*.yaml"}},
				Contents: []catalog.Content{{Kind: catalog.ContentCRDs}}},
			{ID: "cli", Type: domain.ArtifactBinary, Name: "acmectl",
				Version:  catalog.VersionRelation{Strategy: catalog.VersionIndependent},
				Channels: []catalog.Locator{{Kind: catalog.LocatorHTTP, URL: "https://github.com/acme/cli/releases/latest/download/acmectl"}}},
		},
	}
}

func TestTestDefinitionIsValid(t *testing.T) {
	rep := catalog.Validate(testDef())
	if !rep.OK() {
		t.Fatalf("synthetic definition invalid: %v", rep.Issues)
	}
}

// ---------------------------------------------------------------------------
// Fake adapters.

func unavailable(key string) error {
	return &fetch.Error{URL: key, Err: fetch.ErrUnavailable, Detail: "blocked by proxy"}
}

func notFound(key string) error {
	return &fetch.Error{URL: key, Status: 404, Err: fetch.ErrNotFound}
}

// fkey is the fake world's address of a rendered locator.
func fkey(l catalog.Locator) string {
	switch l.Kind {
	case catalog.LocatorHTTP:
		return l.URL
	case catalog.LocatorHelmRepo:
		return l.URL + "#" + l.Chart
	case catalog.LocatorOCI:
		return "oci://" + l.Repository
	case catalog.LocatorChartTGZ:
		return l.URL
	case catalog.LocatorGitHubReleases, catalog.LocatorGitHubAdvisories:
		k := l.Kind + ":" + l.Repository
		if l.Ref != "" {
			k += "@" + l.Ref
		}
		return k
	}
	return l.Kind + ":" + l.Repository + "@" + l.Ref + ":" + l.Path
}

func fakeURI(key string) string { return "https://fake.test/" + key }

// world is an in-memory upstream implementing every source port.
type world struct {
	mu         sync.Mutex
	releases   map[string][]sources.ReleaseRef
	docs       map[string]string
	dirs       map[string]map[string]string
	images     map[string]string // probe key (fkey@version) → digest
	indexes    map[string][]sources.ArtifactVersion
	packages   map[string]*sources.ChartPackage // package key (fkey@version)
	imgConfigs map[string]*sources.Image        // image-manifest key (fkey@version)
	advisories map[string][]domain.Advisory
	advEv      map[string][]domain.Evidence
	errs       map[string]error // per key, any capability
	down       map[string]bool  // per locator kind
	calls      map[string]int
}

func (w *world) enter(kind, key string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls[key]++
	if w.down[kind] {
		return unavailable(key)
	}
	if err, ok := w.errs[key]; ok {
		return err
	}
	return nil
}

func (w *world) callCount(key string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls[key]
}

func (w *world) ListReleases(_ context.Context, loc catalog.Locator) ([]sources.ReleaseRef, error) {
	key := fkey(loc)
	if err := w.enter(loc.Kind, key); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	refs, ok := w.releases[key]
	if !ok {
		return nil, notFound(key)
	}
	return append([]sources.ReleaseRef(nil), refs...), nil
}

func (w *world) FetchDocument(_ context.Context, loc catalog.Locator) (*sources.Document, error) {
	key := fkey(loc)
	if err := w.enter(loc.Kind, key); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.docs[key]
	if !ok {
		return nil, notFound(key)
	}
	return &sources.Document{Locator: loc, URI: fakeURI(key), FetchURL: fakeURI(key), Content: []byte(c), Format: "markdown", RetrievedAt: fetchedAt}, nil
}

func (w *world) FetchDirectory(_ context.Context, loc catalog.Locator) ([]sources.Document, error) {
	key := fkey(loc)
	if err := w.enter(loc.Kind, key); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	files, ok := w.dirs[key]
	if !ok {
		return nil, notFound(key)
	}
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names))) // the pipeline must sort
	var out []sources.Document
	for _, n := range names {
		p := loc.Path + "/" + n
		out = append(out, sources.Document{Locator: loc, URI: fakeURI(key + "/" + n), Path: p, Content: []byte(files[n]), Format: "yaml", RetrievedAt: fetchedAt})
	}
	return out, nil
}

func probeCoordinate(ch catalog.Locator, version string) string {
	switch ch.Kind {
	case catalog.LocatorOCI:
		return ch.Repository + ":" + version
	case catalog.LocatorHelmRepo:
		return ch.URL + "/" + ch.Chart + ":" + version
	}
	return ch.URL
}

func (w *world) Probe(_ context.Context, ch catalog.Locator, version string) (*sources.ProbeResult, error) {
	key := fkey(ch) + "@" + version
	if err := w.enter(ch.Kind, key); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	coord := probeCoordinate(ch, version)
	digest, ok := w.images[key]
	res := &sources.ProbeResult{Exists: ok, Coordinate: coord, URI: fakeURI(key)}
	if ok {
		res.Digest = digest
		res.Evidence = domain.NewEvidence(domain.EvidenceRegistry, "", res.URI, coord, coord, digest, fetchedAt)
	}
	return res, nil
}

func (w *world) ListArtifactVersions(_ context.Context, ch catalog.Locator) ([]sources.ArtifactVersion, error) {
	key := fkey(ch)
	if err := w.enter(ch.Kind, key); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	entries, ok := w.indexes[key]
	if !ok {
		return nil, notFound(key)
	}
	return append([]sources.ArtifactVersion(nil), entries...), nil
}

func (w *world) ListAdvisories(_ context.Context, loc catalog.Locator) ([]domain.Advisory, []domain.Evidence, error) {
	key := fkey(loc)
	if err := w.enter(loc.Kind, key); err != nil {
		return nil, nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	advs, ok := w.advisories[key]
	if !ok {
		return nil, nil, notFound(key)
	}
	return append([]domain.Advisory(nil), advs...), w.advEv[key], nil
}

// ReadChartPackage serves the chart-package port from the fixture world.
func (w *world) ReadChartPackage(_ context.Context, loc catalog.Locator, version string) (*sources.ChartPackage, error) {
	key := fkey(loc) + "@" + version
	if err := w.enter(loc.Kind, key); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	pkg, ok := w.packages[key]
	if !ok {
		return nil, notFound(key)
	}
	return pkg, nil
}

// ReadImage serves the image-manifest port from the fixture world.
func (w *world) ReadImage(_ context.Context, loc catalog.Locator, version string) (*sources.Image, error) {
	key := fkey(loc) + "@" + version
	if err := w.enter(loc.Kind, key); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	img, ok := w.imgConfigs[key]
	if !ok {
		return nil, notFound(key)
	}
	return img, nil
}

// registry registers the world for every kind it serves (git-log and
// helm-git deliberately have no adapter).
func (w *world) registry() *sources.Registry {
	reg := sources.NewRegistry()
	for _, k := range []string{catalog.LocatorGitHubReleases, catalog.LocatorGitTags} {
		reg.RegisterVersionLister(k, w)
	}
	for _, k := range []string{catalog.LocatorRepoFile, catalog.LocatorHTTP, catalog.LocatorGitHubReleases} {
		reg.RegisterDocumentFetcher(k, w)
	}
	reg.RegisterDirectoryFetcher(catalog.LocatorRepoDir, w)
	for _, k := range []string{catalog.LocatorOCI, catalog.LocatorHelmRepo, catalog.LocatorHTTP} {
		reg.RegisterProbe(k, w)
	}
	for _, k := range []string{catalog.LocatorOCI, catalog.LocatorHelmRepo} {
		reg.RegisterVersionIndex(k, w)
	}
	for _, k := range []string{catalog.LocatorOCI, catalog.LocatorHelmRepo, catalog.LocatorChartTGZ} {
		reg.RegisterChartPackageReader(k, w)
	}
	reg.RegisterImageManifestReader(catalog.LocatorOCI, w)
	reg.RegisterAdvisorySource(catalog.LocatorGitHubAdvisories, w)
	return reg
}

func websiteKey(path string) string { return "repo-file:" + website + "@main:" + path }

func manifestURL(tag string) string {
	return "https://github.com/acme/operator/releases/download/" + tag + "/acme.yaml"
}

func manifestFor(tag string) string {
	return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: acme-controller
spec:
  template:
    spec:
      containers:
      - name: controller
        image: "quay.io/acme/acme-controller:` + tag + `"
      - name: sidecar
        image: quay.io/acme/acme-sidecar:` + tag + `
`
}

const notes12 = "# Release 1.2\n\nIntro paragraph.\n\n## Major Themes\n\n- A big theme\n\n" +
	"## `v1.2.1`\n\n### Bug or Regression\n\n- Fixed crash in webhook (#12)\n\n" +
	"## `v1.2.0`\n\n### Feature\n\n- Added widget support\n\n### Breaking\n\n- Removed the legacy flag\n"

const notes11 = "# Release 1.1\n\n## v1.1.0\n\n### Feature\n\n- Added gadgets\n"

const notes10 = "# Release 1.0\n\n## v1.0.1\n\n- Fixed docs\n\n## v1.0.0\n\n- First release\n"

const supportMatrix = `# Supported releases

## Currently supported releases

| Release | Release Date | Supported Kubernetes versions |
|---------|--------------|-------------------------------|
| [1.2][] | Jan 01, 2026 | 1.30 → 1.33 |
| [1.1][] | Jun 01, 2025 | 1.29 → 1.32 |
| [1.0][] | Jan 01, 2025 | 1.28 → 1.31 |
`

const crdWidgets = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.acme.example
`

const crdGadgets = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: gadgets.acme.example
`

var acmeTags = []string{"v1.0.0", "v1.0.1", "v1.1.0", "v1.1.1", "v1.2.0", "v1.2.1"}

// chartForApp maps chart versions to the appVersion they ship. v1.2.1 is
// deliberately never shipped (as happens for Argo CD).
var chartForApp = []struct{ chart, app string }{
	{"0.3.0", "v1.0.0"}, {"0.3.1", "v1.0.1"}, {"0.4.0", "v1.1.0"}, {"0.4.1", "v1.1.1"}, {"0.5.1", "v1.2.0"}, {"0.5.0", "v1.2.0"},
}

func newWorld() *world {
	w := &world{
		releases:   map[string][]sources.ReleaseRef{},
		docs:       map[string]string{},
		dirs:       map[string]map[string]string{},
		images:     map[string]string{},
		indexes:    map[string][]sources.ArtifactVersion{},
		packages:   map[string]*sources.ChartPackage{},
		imgConfigs: map[string]*sources.Image{},
		advisories: map[string][]domain.Advisory{},
		advEv:      map[string][]domain.Evidence{},
		errs:       map[string]error{},
		down:       map[string]bool{},
		calls:      map[string]int{},
	}
	pub := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	w.releases["github-releases:acme/operator"] = []sources.ReleaseRef{
		{Tag: "v1.2.1", URL: "https://github.com/acme/operator/releases/tag/v1.2.1", PublishedAt: &pub, Commit: "abc123"},
		{Tag: "v1.0.0"}, {Tag: "v1.1.1"}, {Tag: "v1.3.0-rc.1", Prerelease: true}, {Tag: "v1.1.0"},
		{Tag: "v1.2.0"}, {Tag: "v1.0.1"}, {Tag: "cmd/ctl/v1.2.0"},
	}
	w.docs[websiteKey("content/release-notes/release-notes-1.2.md")] = notes12
	w.docs[websiteKey("content/release-notes/release-notes-1.1.md")] = notes11
	w.docs[websiteKey("content/release-notes/release-notes-1.0.md")] = notes10
	w.docs[websiteKey("content/upgrading/upgrading-1.1-1.2.md")] = "# Upgrading from 1.1 to 1.2\n\n- Remove the legacy flag before upgrading\n"
	w.docs[websiteKey("content/upgrading/upgrading-1.0-1.1.md")] = "# Upgrading from 1.0 to 1.1\n\n- Nothing special\n"
	w.docs[websiteKey("content/releases/README.md")] = supportMatrix
	for k, tag := range acmeTags {
		w.docs["github-releases:acme/operator@"+tag] = "## Changes\n\n- GitHub release notes for " + tag + "\n"
		w.docs[manifestURL(tag)] = manifestFor(tag)
		w.images["oci://quay.io/acme/acme-controller@"+tag] = fmt.Sprintf("sha256:%064d", k+1)
		w.dirs["repo-dir:github.com/acme/operator@"+tag+":deploy/crds"] = map[string]string{
			"widgets.yaml": crdWidgets, "gadgets.yaml": crdGadgets,
		}
	}
	var index []sources.ArtifactVersion
	for _, c := range chartForApp {
		uri := "https://charts.acme.example/acme-" + c.chart + ".tgz"
		index = append(index, sources.ArtifactVersion{
			Version: c.chart, Fields: map[string]string{"appVersion": c.app}, URI: uri,
			Evidence: domain.NewEvidence(domain.EvidenceRegistry, "", "https://charts.acme.example/index.yaml", "acme-"+c.chart, "version: "+c.chart+" appVersion: "+c.app, "", fetchedAt),
		})
		w.images["https://charts.acme.example#acme@"+c.chart] = "sha256:chart-" + c.chart
		hc := "repo-file:github.com/acme/helm-charts@acme-" + c.chart + ":charts/acme/"
		w.docs[hc+"values.yaml"] = "# Default values for acme.\nreplicaCount: 1\nimage: quay.io/acme/acme-controller\nlogLevel: info\n"
		w.docs[hc+"Chart.yaml"] = "apiVersion: v2\nname: acme\nversion: " + c.chart + "\nappVersion: " + c.app + "\nkubeVersion: \">= 1.25.0-0\"\n"
	}
	w.indexes["https://charts.acme.example#acme"] = index
	// The OCI chart registry lists tags only (no appVersion), like ghcr.
	w.indexes["oci://ghcr.io/acme/charts/acme"] = []sources.ArtifactVersion{{Version: "0.5.1"}, {Version: "0.5.0"}}
	ghsa := domain.NewEvidence(domain.EvidenceAdvisory, "", "https://github.com/acme/operator/security/advisories/GHSA-aaaa-bbbb-cccc", "GHSA-aaaa-bbbb-cccc", "Privilege escalation", "", fetchedAt)
	w.advisories["github-advisories:acme/operator"] = []domain.Advisory{
		{ID: "GHSA-aaaa-bbbb-cccc", Aliases: []string{"CVE-2026-0001"}, Summary: "Privilege escalation", Severity: "high",
			URL: "https://github.com/acme/operator/security/advisories/GHSA-aaaa-bbbb-cccc", Vulnerable: ">= 1.0.0, < 1.2.1", Patched: []string{"1.2.1"},
			Evidence: []domain.EvidenceID{ghsa.ID}},
		{ID: "GHSA-dddd-eeee-ffff", Summary: "Denial of service", URL: "https://github.com/acme/operator/security/advisories/GHSA-dddd-eeee-ffff"},
	}
	w.advEv["github-advisories:acme/operator"] = []domain.Evidence{ghsa}
	return w
}

func newTestIngester(w *world, p Parser) *Ingester {
	ing := New(w.registry())
	ing.Clock = func() time.Time { return testNow }
	ing.Parser = p
	return ing
}

// ---------------------------------------------------------------------------
// Fake parser: small, deterministic stand-ins for package normalize.

type fakeParser struct {
	mu        sync.Mutex
	noteCalls []normalize.DocInput
}

func (f *fakeParser) notesFor(sourceID string) []normalize.DocInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []normalize.DocInput
	for _, in := range f.noteCalls {
		if in.SourceID == sourceID {
			out = append(out, in)
		}
	}
	return out
}

func headingLevel(l string) int {
	n := len(l) - len(strings.TrimLeft(l, "#"))
	if n == 0 || n > 6 || !strings.HasPrefix(l[n:], " ") {
		return 0
	}
	return n
}

func (f *fakeParser) SelectSection(md []byte, re *regexp.Regexp) (*normalize.Section, error) {
	lines := strings.Split(string(md), "\n")
	for i, l := range lines {
		level := headingLevel(l)
		if level == 0 {
			continue
		}
		h := strings.Trim(strings.TrimSpace(strings.TrimLeft(l, "#")), "`")
		if !re.MatchString(h) {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if lv := headingLevel(lines[j]); lv > 0 && lv <= level {
				end = j
				break
			}
		}
		body := strings.TrimRight(strings.Join(lines[i:end], "\n"), "\n")
		return &normalize.Section{Heading: h, Level: level, Path: []string{h}, StartLine: i + 1,
			EndLine: i + 1 + strings.Count(body, "\n"), Body: body}, nil
	}
	return nil, normalize.ErrNoMatch
}

func (f *fakeParser) ParseNotes(in normalize.DocInput, _ []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error) {
	f.mu.Lock()
	f.noteCalls = append(f.noteCalls, in)
	f.mu.Unlock()
	var items []domain.NoteItem
	var evs []domain.Evidence
	section := ""
	for idx, l := range strings.Split(string(in.Content), "\n") {
		if headingLevel(l) > 0 {
			section = strings.TrimSpace(strings.TrimLeft(l, "#"))
			continue
		}
		if !strings.HasPrefix(l, "- ") {
			continue
		}
		line := idx + 1 + in.LineOffset
		ev := domain.NewEvidence(domain.EvidenceDocument, in.SourceID, in.URI, fmt.Sprintf("L%d", line), l, in.Digest, in.RetrievedAt)
		cat, breaking := domain.CategoryFeature, false
		if strings.Contains(strings.ToLower(section), "breaking") {
			cat, breaking = domain.CategoryRemoval, true
		}
		items = append(items, domain.NoteItem{
			ID: "note-" + domain.ShortHash(in.SourceID, in.Release, l), Release: in.Release, SourceID: in.SourceID, Role: in.Role,
			Section: section, Text: strings.TrimPrefix(l, "- "), Category: cat, Breaking: breaking,
			Classification: domain.Provenance{Method: domain.MethodDeclared, Producer: "fake", Confidence: domain.ConfidenceHigh},
			Evidence:       []domain.EvidenceID{ev.ID},
		})
		evs = append(evs, ev)
	}
	return items, evs, nil
}

func (f *fakeParser) ParseReleaseNoteYAML(files []normalize.DocInput, _ []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error) {
	var items []domain.NoteItem
	var evs []domain.Evidence
	for _, in := range files {
		ev := domain.NewEvidence(domain.EvidenceDocument, in.SourceID, in.URI, "L1", firstLine(in.Content), in.Digest, in.RetrievedAt)
		items = append(items, domain.NoteItem{ID: "note-" + domain.ShortHash(in.URI), Release: in.Release, SourceID: in.SourceID, Role: in.Role,
			Text: firstLine(in.Content), Category: domain.CategoryFeature,
			Classification: domain.Provenance{Method: domain.MethodDeclared, Producer: "fake", Confidence: domain.ConfidenceHigh},
			Evidence:       []domain.EvidenceID{ev.ID}})
		evs = append(evs, ev)
	}
	return items, evs, nil
}

var mdLinkRe = regexp.MustCompile(`\[([^\]]*)\](\[[^\]]*\]|\([^)]*\))?`)

func stripMD(s string) string {
	return strings.TrimSpace(mdLinkRe.ReplaceAllString(strings.TrimSpace(s), "$1"))
}

func tableCells(l string) []string {
	l = strings.Trim(strings.TrimSpace(l), "|")
	parts := strings.Split(l, "|")
	for k := range parts {
		parts[k] = strings.TrimSpace(parts[k])
	}
	return parts
}

func (f *fakeParser) ExtractTableRow(md []byte, sel normalize.TableSelector) (*normalize.TableRow, error) {
	lines := strings.Split(string(md), "\n")
	for i := 0; i+1 < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "|") || !strings.HasPrefix(lines[i+1], "|-") {
			continue
		}
		headers := tableCells(lines[i])
		key := -1
		for k, h := range headers {
			headers[k] = stripMD(h)
			for _, kc := range sel.KeyColumns {
				if strings.EqualFold(headers[k], kc) {
					key = k
				}
			}
		}
		if key < 0 {
			continue
		}
		for j := i + 2; j < len(lines) && strings.HasPrefix(lines[j], "|"); j++ {
			row := tableCells(lines[j])
			if key < len(row) && sel.KeyRe.MatchString(stripMD(row[key])) {
				cells := map[string]string{}
				for k, h := range headers {
					if k < len(row) {
						cells[h] = row[k]
					}
				}
				return &normalize.TableRow{Headers: headers, Cells: cells, Line: j + 1, Excerpt: lines[i] + "\n" + lines[j]}, nil
			}
		}
	}
	return nil, normalize.ErrNoMatch
}

// ExtractRecord understands "- key: value" records (one level).
func (f *fakeParser) ExtractRecord(content []byte, sel normalize.TableSelector) (*normalize.TableRow, error) {
	lines := strings.Split(string(content), "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "- ") {
			continue
		}
		cells := map[string]string{}
		var excerpt []string
		for j := i; j < len(lines) && (j == i || strings.HasPrefix(lines[j], "  ")); j++ {
			kv := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(lines[j], "- ")), ":", 2)
			if len(kv) == 2 {
				cells[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), `"`)
			}
			excerpt = append(excerpt, lines[j])
		}
		for _, kc := range sel.KeyColumns {
			if v, ok := cells[kc]; ok && sel.KeyRe.MatchString(v) {
				var headers []string
				for h := range cells {
					headers = append(headers, h)
				}
				sort.Strings(headers)
				return &normalize.TableRow{Headers: headers, Cells: cells, Line: i + 1, Excerpt: strings.Join(excerpt, "\n")}, nil
			}
		}
	}
	return nil, normalize.ErrNoMatch
}

// CompatibilityFromRow leaves SourceID empty so the pipeline must fill it.
func (f *fakeParser) CompatibilityFromRow(in normalize.DocInput, row *normalize.TableRow, columns []catalog.ColumnSpec) ([]domain.CompatibilityConstraint, []domain.Evidence) {
	ev := domain.NewEvidence(domain.EvidenceDocument, in.SourceID, in.URI, fmt.Sprintf("L%d", row.Line+in.LineOffset), row.Excerpt, in.Digest, in.RetrievedAt)
	var out []domain.CompatibilityConstraint
	for _, c := range columns {
		for _, h := range c.Headers {
			for name, v := range row.Cells {
				if !strings.EqualFold(name, h) {
					continue
				}
				kind := c.Kind
				if kind == "" {
					kind = "supported"
				}
				out = append(out, domain.CompatibilityConstraint{Platform: c.Platform, Kind: kind, Raw: v, Constraint: "range(" + v + ")",
					Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "fake", Confidence: domain.ConfidenceHigh},
					Evidence:   []domain.EvidenceID{ev.ID}})
			}
		}
	}
	return out, []domain.Evidence{ev}
}

func (f *fakeParser) ParseVersionRange(raw string) (string, []string, error) {
	return strings.ReplaceAll(raw, " ", ""), nil, nil
}

func (f *fakeParser) ReadYAMLPath(b []byte, path string) (string, int, error) {
	return normalize.ReadYAMLPath(b, path)
}

func (f *fakeParser) ParseChartMetadata(b []byte) (*normalize.ChartMetadata, error) {
	md := &normalize.ChartMetadata{}
	for _, l := range strings.Split(string(b), "\n") {
		kv := strings.SplitN(l, ":", 2)
		if len(kv) != 2 {
			continue
		}
		v := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		switch strings.TrimSpace(kv[0]) {
		case "name":
			md.Name = v
		case "version":
			md.Version = v
		case "appVersion":
			md.AppVersion = v
		case "kubeVersion":
			md.KubeVersion = v
		}
	}
	return md, nil
}

func (f *fakeParser) ValuesSnapshot(chart, version string, b []byte) (*domain.ValuesSnapshot, error) {
	vs := &domain.ValuesSnapshot{Chart: chart, Version: version, Entries: map[string]string{}}
	for _, l := range strings.Split(string(b), "\n") {
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, " ") {
			continue
		}
		kv := strings.SplitN(l, ":", 2)
		if len(kv) == 2 {
			vs.Entries[kv[0]] = strconv.Quote(strings.TrimSpace(kv[1]))
		}
	}
	return vs, nil
}

func (f *fakeParser) CRDSnapshot(streams ...[]byte) (*domain.CRDSnapshot, error) {
	snap := &domain.CRDSnapshot{}
	for _, s := range streams {
		for _, doc := range strings.Split(string(s), "\n---") {
			if !strings.Contains(doc, "kind: CustomResourceDefinition") {
				continue
			}
			for _, l := range strings.Split(doc, "\n") {
				if t := strings.TrimSpace(l); strings.HasPrefix(t, "name:") {
					snap.CRDs = append(snap.CRDs, domain.CRDSummary{Name: strings.TrimSpace(strings.TrimPrefix(t, "name:"))})
					break
				}
			}
		}
	}
	sort.Slice(snap.CRDs, func(a, b int) bool { return snap.CRDs[a].Name < snap.CRDs[b].Name })
	return snap, nil
}

func (f *fakeParser) ImageRefs(b []byte) []domain.ImageRef {
	var out []domain.ImageRef
	for _, l := range strings.Split(string(b), "\n") {
		k := strings.Index(l, "image:")
		if k < 0 {
			continue
		}
		ref := strings.Trim(strings.TrimSpace(l[k+len("image:"):]), `"`)
		img := domain.ImageRef{Repository: ref}
		if c := strings.LastIndex(ref, ":"); c > strings.LastIndex(ref, "/") {
			img = domain.ImageRef{Repository: ref[:c], Tag: ref[c+1:]}
		}
		out = append(out, img)
	}
	return out
}

// ---------------------------------------------------------------------------
// Assertion helpers.

// assertEvidenceIntegrity checks that evidence ids are unique and that
// everything in the release references evidence that it carries.
func assertEvidenceIntegrity(t *testing.T, rel *domain.Release) {
	t.Helper()
	have := map[domain.EvidenceID]bool{}
	for _, e := range rel.Evidence {
		if e.ID == "" {
			t.Errorf("evidence without id: %+v", e)
		}
		if have[e.ID] {
			t.Errorf("duplicate evidence %s", e.ID)
		}
		have[e.ID] = true
	}
	check := func(what string, ids []domain.EvidenceID) {
		for _, id := range ids {
			if !have[id] {
				t.Errorf("%s references unknown evidence %s", what, id)
			}
		}
	}
	for _, n := range rel.Notes {
		check("note "+n.ID, n.Evidence)
	}
	for _, c := range rel.Compat {
		check("compat "+c.SourceID, c.Evidence)
		if len(c.Evidence) == 0 {
			t.Errorf("compat %s without evidence", c.SourceID)
		}
	}
	for _, s := range rel.Snapshots {
		check("snapshot "+s.ArtifactID, s.Evidence)
		if len(s.Evidence) == 0 {
			t.Errorf("snapshot %s without evidence", s.ArtifactID)
		}
	}
	for _, f := range rel.Facts {
		check("fact "+string(f.Kind), f.Evidence)
		if len(f.Evidence) == 0 {
			t.Errorf("fact %s (%s) without evidence", f.ID, f.Statement)
		}
	}
	for _, a := range rel.Artifacts {
		check("artifact "+a.ArtifactID, a.Evidence)
		if (a.Status == domain.ArtifactVerified || a.Status == domain.ArtifactReferenced) && len(a.Evidence) == 0 {
			t.Errorf("artifact %s is %s without evidence", a.ArtifactID, a.Status)
		}
	}
}

func statusOf(t *testing.T, rel *domain.Release, sourceID string) domain.SourceStatus {
	t.Helper()
	for _, s := range rel.Sources {
		if s.SourceID == sourceID {
			return s
		}
	}
	t.Fatalf("no status for %s in %+v", sourceID, rel.Sources)
	return domain.SourceStatus{}
}

func statusesOf(rel *domain.Release, sourceID string) []domain.SourceStatus {
	var out []domain.SourceStatus
	for _, s := range rel.Sources {
		if s.SourceID == sourceID {
			out = append(out, s)
		}
	}
	return out
}

func artifactsOf(rel *domain.Release, id string) []domain.ArtifactInstance {
	var out []domain.ArtifactInstance
	for _, a := range rel.Artifacts {
		if a.ArtifactID == id {
			out = append(out, a)
		}
	}
	return out
}

func artifactOf(t *testing.T, rel *domain.Release, id string) domain.ArtifactInstance {
	t.Helper()
	as := artifactsOf(rel, id)
	if len(as) != 1 {
		t.Fatalf("expected one instance of %s, got %+v", id, as)
	}
	return as[0]
}

func evidenceByID(rel *domain.Release, id domain.EvidenceID) (domain.Evidence, bool) {
	for _, e := range rel.Evidence {
		if e.ID == id {
			return e, true
		}
	}
	return domain.Evidence{}, false
}

func factsOf(rel *domain.Release, kind domain.FactKind) []domain.Fact {
	var out []domain.Fact
	for _, f := range rel.Facts {
		if f.Kind == kind {
			out = append(out, f)
		}
	}
	return out
}

// mustVersions lists versions from the default world.
func mustVersions(t *testing.T, ing *Ingester, def *catalog.ProductDefinition) *VersionList {
	t.Helper()
	vl, err := ing.ListVersions(context.Background(), def)
	if err != nil {
		t.Fatal(err)
	}
	return vl
}

func version(t *testing.T, vl *VersionList, semver string) domain.Version {
	t.Helper()
	for _, v := range vl.Versions {
		if v.Semver == semver {
			return v
		}
	}
	t.Fatalf("version %s not in %v", semver, vl.Versions)
	return domain.Version{}
}
