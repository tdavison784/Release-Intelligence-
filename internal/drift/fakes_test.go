package drift

// An in-memory fake upstream, shaped after internal/ingest/fakes_test.go but
// slimmed to what drift needs: git-tags version listing, repo-file/http
// documents, oci/helm-repo probes and version indexes. Every retrieval is
// deterministic (fixed clock), so reports and evidence ids are reproducible.

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/ingest"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

var (
	testNow   = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	fetchedAt = time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
)

func unavailable(key string) error {
	return &fetch.Error{URL: key, Err: fetch.ErrUnavailable, Detail: "blocked by proxy"}
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
	case catalog.LocatorGitHubReleases:
		return l.Kind + ":" + l.Repository
	}
	return l.Kind + ":" + l.Repository + "@" + l.Ref + ":" + l.Path
}

type world struct {
	mu       sync.Mutex
	releases map[string][]sources.ReleaseRef
	docs     map[string]string
	images   map[string]string // probe key fkey@version → digest
	indexes  map[string][]sources.ArtifactVersion
	errs     map[string]error // per key (any capability)
}

func (w *world) enter(key string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err, ok := w.errs[key]; ok {
		return err
	}
	return nil
}

func (w *world) ListReleases(_ context.Context, loc catalog.Locator) ([]sources.ReleaseRef, error) {
	key := fkey(loc)
	if err := w.enter(key); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	refs, ok := w.releases[key]
	if !ok {
		return nil, &fetch.Error{URL: key, Status: 404, Err: fetch.ErrNotFound}
	}
	return append([]sources.ReleaseRef(nil), refs...), nil
}

func (w *world) FetchDocument(_ context.Context, loc catalog.Locator) (*sources.Document, error) {
	key := fkey(loc)
	if err := w.enter(key); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.docs[key]
	if !ok {
		return nil, &fetch.Error{URL: key, Status: 404, Err: fetch.ErrNotFound}
	}
	return &sources.Document{Locator: loc, URI: "https://fake.test/" + key, FetchURL: "https://fake.test/" + key,
		Content: []byte(c), Format: "markdown", RetrievedAt: fetchedAt}, nil
}

func (w *world) Probe(_ context.Context, ch catalog.Locator, version string) (*sources.ProbeResult, error) {
	key := fkey(ch) + "@" + version
	if err := w.enter(key); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	coord := version
	switch ch.Kind {
	case catalog.LocatorOCI:
		coord = ch.Repository + ":" + version
	case catalog.LocatorHelmRepo:
		coord = ch.URL + "/" + ch.Chart + ":" + version
	}
	digest, ok := w.images[key]
	res := &sources.ProbeResult{Exists: ok, Coordinate: coord, URI: "https://fake.test/" + key}
	if ok {
		res.Digest = digest
		res.Evidence = domain.NewEvidence(domain.EvidenceRegistry, "", res.URI, coord, coord, digest, fetchedAt)
	} else {
		// like the real adapters: a reachable registry saying "unknown" is
		// evidence for the absence
		res.Evidence = domain.NewEvidence(domain.EvidenceRegistry, "", res.URI, coord, "manifest unknown (HTTP 404)", "", fetchedAt)
	}
	return res, nil
}

func (w *world) ListArtifactVersions(_ context.Context, ch catalog.Locator) ([]sources.ArtifactVersion, error) {
	key := fkey(ch)
	if err := w.enter(key); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	entries, ok := w.indexes[key]
	if !ok {
		return nil, &fetch.Error{URL: key, Status: 404, Err: fetch.ErrNotFound}
	}
	return append([]sources.ArtifactVersion(nil), entries...), nil
}

func (w *world) registry() *sources.Registry {
	reg := sources.NewRegistry()
	reg.RegisterVersionLister(catalog.LocatorGitTags, w)
	reg.RegisterDocumentFetcher(catalog.LocatorRepoFile, w)
	reg.RegisterDocumentFetcher(catalog.LocatorHTTP, w)
	for _, k := range []string{catalog.LocatorOCI, catalog.LocatorHelmRepo} {
		reg.RegisterProbe(k, w)
		reg.RegisterVersionIndex(k, w)
	}
	return reg
}

// ---------------------------------------------------------------------------
// Fixture: acme operator, shaped after cert-manager / Argo CD.

var acmeTags = []string{"v1.0.0", "v1.0.1", "v1.1.0", "v1.1.1", "v1.2.0", "v1.2.1"}

// chartForApp maps chart versions to the appVersion they ship.
var chartForApp = []struct{ chart, app string }{
	{"0.3.0", "v1.0.0"}, {"0.3.1", "v1.0.1"},
	{"0.4.0", "v1.1.0"}, {"0.4.1", "v1.1.1"},
	{"0.5.0", "v1.2.0"}, {"0.5.1", "v1.2.1"},
}

func notesDoc(line string, tags ...string) string {
	var out string
	for _, t := range tags {
		ver := t[1:]
		out += "## " + t + "\n\n### Feature\n\n- Added things in " + ver + "\n\n"
	}
	return "# Notes " + line + "\n\n" + out
}

func manifestFor(tag string, extraImages ...string) string {
	m := `apiVersion: v1
images:
`
	m += "  - image: quay.io/acme/controller:" + tag + "\n"
	for _, img := range extraImages {
		m += "  - image: " + img + ":" + tag + "\n"
	}
	return m
}

const (
	quayController = "oci://quay.io/acme/controller"
	ghcrChart      = "oci://ghcr.io/acme/charts/acme"
	helmIndex      = "https://charts.acme.example#acme"
	legacyRepo     = "oci://docker.io/acme/legacy"
)

func manifestURL(tag string) string {
	return "https://github.com/acme/operator/releases/download/" + tag + "/acme.yaml"
}

// newWorld builds the intact upstream: every release of the fixture
// definition is fully published.
func newWorld() *world {
	w := &world{
		releases: map[string][]sources.ReleaseRef{},
		docs:     map[string]string{},
		images:   map[string]string{},
		indexes:  map[string][]sources.ArtifactVersion{},
		errs:     map[string]error{},
	}
	pub := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var refs []sources.ReleaseRef
	for _, t := range acmeTags {
		refs = append(refs, sources.ReleaseRef{Tag: t, URL: "https://github.com/acme/operator/releases/tag/" + t, PublishedAt: &pub})
	}
	w.releases["git-tags:github.com/acme/operator@:"] = refs
	w.docs["repo-file:github.com/acme/site@main:content/notes-1.0.md"] = notesDoc("1.0", "v1.0.0", "v1.0.1")
	w.docs["repo-file:github.com/acme/site@main:content/notes-1.1.md"] = notesDoc("1.1", "v1.1.0", "v1.1.1")
	w.docs["repo-file:github.com/acme/site@main:content/notes-1.2.md"] = notesDoc("1.2", "v1.2.0", "v1.2.1")
	for k, t := range acmeTags {
		w.images[quayController+"@"+t] = fmt.Sprintf("sha256:%064d", k+1)
		w.docs[manifestURL(t)] = manifestFor(t)
		// the legacy image is retired from 1.2.0 (its availability window)
		if t == "v1.0.0" || t == "v1.0.1" || t == "v1.1.0" || t == "v1.1.1" {
			w.images[legacyRepo+"@"+t] = fmt.Sprintf("sha256:legacy%064d", k+1)
		}
	}
	w.indexes[helmIndex] = indexFor(chartForApp)
	// The OCI chart registry lists the same entries (a declared fallback).
	w.indexes[ghcrChart] = indexFor(chartForApp)
	for _, c := range chartForApp {
		w.images[helmIndex+"@"+c.chart] = "sha256:chart-" + c.chart
		w.images[ghcrChart+"@"+c.chart] = "sha256:chart-" + c.chart
	}
	return w
}

func indexFor(pairs []struct{ chart, app string }) []sources.ArtifactVersion {
	sorted := append([]struct{ chart, app string }{}, pairs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].chart < sorted[j].chart })
	var out []sources.ArtifactVersion
	for _, c := range sorted {
		out = append(out, sources.ArtifactVersion{
			Version: c.chart, Fields: map[string]string{"appVersion": c.app},
			URI: "https://charts.acme.example/acme-" + c.chart + ".tgz",
			Evidence: domain.NewEvidence(domain.EvidenceRegistry, "", "https://charts.acme.example/index.yaml",
				"acme-"+c.chart, "version: "+c.chart+" appVersion: "+c.app, "", fetchedAt),
		})
	}
	return out
}

func testDef() *catalog.ProductDefinition {
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
			{ID: "git-tags", Roles: []domain.SourceRole{domain.RoleVersions},
				Locator: catalog.Locator{Kind: catalog.LocatorGitTags, Repository: "github.com/acme/operator"}},
			{ID: "notes", Roles: []domain.SourceRole{domain.RoleReleaseNotes},
				Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: "github.com/acme/site", Ref: "main",
					Path: "content/notes-{{.Line}}.md"},
				Extract: &catalog.Extract{Type: catalog.ExtractMarkdownSection, Heading: `^v{{regexQuote .Version}}$`}},
		},
		Artifacts: []catalog.Artifact{
			{ID: "controller-image", Type: domain.ArtifactContainerImage, Name: "controller",
				Version: tmpl("{{.Tag}}"), ValidatedAgainst: []string{"1.0.0", "1.1.0"},
				Channels: []catalog.Locator{{Kind: catalog.LocatorOCI, Repository: "quay.io/acme/controller"}}},
			{ID: "chart", Type: domain.ArtifactHelmChart, Name: "acme",
				Version:          catalog.VersionRelation{Strategy: catalog.VersionLookup, Field: "appVersion", Match: "{{.Tag}}"},
				ValidatedAgainst: []string{"1.0.0", "1.1.0"},
				Channels: []catalog.Locator{
					{Kind: catalog.LocatorHelmRepo, URL: "https://charts.acme.example", Chart: "acme"},
					{Kind: catalog.LocatorOCI, Repository: "ghcr.io/acme/charts/acme"},
				}},
			{ID: "legacy-image", Type: domain.ArtifactContainerImage, Name: "legacy",
				Version: tmpl("{{.Tag}}"), Availability: "< 1.2.0",
				ValidatedAgainst: []string{"1.0.0", "1.1.0"},
				Channels:         []catalog.Locator{{Kind: catalog.LocatorOCI, Repository: "docker.io/acme/legacy"}}},
			{ID: "install-manifest", Type: domain.ArtifactManifest, Name: "acme.yaml",
				Version: tmpl("{{.Tag}}"), ValidatedAgainst: []string{"1.0.0", "1.1.0"},
				Channels: []catalog.Locator{{Kind: catalog.LocatorHTTP, URL: manifestURL("{{.Tag}}")}},
				Contents: []catalog.Content{{Kind: catalog.ContentImageRefs}}},
		},
		Provenance: &catalog.DefinitionProvenance{Method: "manual", ValidatedReleases: []string{"1.0.0", "1.0.1", "1.1.0", "1.1.1"}},
	}
}

// runCheck ingests the given releases exhaustively (like `ri check`).
func runCheck(w *world, def *catalog.ProductDefinition, semvers []string) (*ingest.RelationshipReport, []*domain.Release, *ingest.VersionList) {
	ing := ingest.New(w.registry())
	ing.Clock = func() time.Time { return testNow }
	vl, err := ing.ListVersions(context.Background(), def)
	if err != nil {
		panic(err)
	}
	var rels []domain.Version
	for _, s := range semvers {
		v, err := app_resolve(vl, s)
		if err != nil {
			panic(err)
		}
		rels = append(rels, v)
	}
	rep, releases, err := ing.IngestChecked(context.Background(), def, rels, vl)
	if err != nil {
		panic(err)
	}
	return rep, releases, vl
}

func app_resolve(vl *ingest.VersionList, s string) (domain.Version, error) {
	for _, v := range vl.Versions {
		if v.Tag == s || v.Semver == s {
			return v, nil
		}
	}
	return domain.Version{}, fmt.Errorf("version %s not in list", s)
}

// analyze runs the full drift analysis over the world for the given releases
// (checking semvers, baseline semvers; an empty baseline list means no saved
// report).
func analyze(w *world, def *catalog.ProductDefinition, baselinePath string, check, baseline []string) *Report {
	var baseRep *ingest.RelationshipReport
	if len(baseline) > 0 {
		baseRep, _, _ = runCheck(w, def, baseline)
	}
	cur, rels, vl := runCheck(w, def, check)
	rep, err := Analyze(context.Background(), Input{
		Definition:   def,
		Versions:     vl,
		Current:      cur,
		Releases:     rels,
		Baseline:     baseRep,
		BaselinePath: baselinePath,
		Registry:     w.registry(),
		Now:          testNow,
	})
	if err != nil {
		panic(err)
	}
	return rep
}

func eventsOf(rep *Report, kind string) []Event {
	var out []Event
	for _, ev := range rep.Events {
		if ev.Kind == kind {
			out = append(out, ev)
		}
	}
	return out
}

func (w *world) removeChartFromHelmIndex(appVersions ...string) {
	var kept []sources.ArtifactVersion
	for _, e := range w.indexes[helmIndex] {
		drop := false
		for _, app := range appVersions {
			if e.Fields["appVersion"] == app {
				drop = true
			}
		}
		if !drop {
			kept = append(kept, e)
		}
	}
	w.indexes[helmIndex] = kept
}
