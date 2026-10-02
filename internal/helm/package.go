package helm

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/httpsrc"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// PackageAdapter reads published Helm chart packages — the gzipped tarballs
// users install — for the locator kinds:
//
//   - "helm-repo" (url, chart): the tarball URL is taken from the
//     repository's index.yaml entry of the requested chart version, and the
//     downloaded bytes are verified against the entry's digest (helm's index
//     digests are the sha256 of the archive);
//   - "chart-tgz" (url): a direct tarball URL, typically a release asset
//     (".../releases/download/<tag>/<chart>-<version>.tgz").
//
// It implements sources.ChartPackageReader for both kinds and
// sources.ArtifactProbe for chart-tgz (a HEAD/GET existence check, the same
// semantics as the http adapter). All HTTP goes through fetch.Client, so
// packages are cached and replayable offline.
type PackageAdapter struct {
	f       fetch.Client
	indexes *indexCache
	probe   sources.ArtifactProbe
}

// NewPackageAdapter returns a package adapter issuing HTTP through f. f also
// serves the embedded chart-tgz probe.
func NewPackageAdapter(f fetch.Client, opts ...Option) *PackageAdapter {
	c := newConfig(opts)
	return &PackageAdapter{f: f, indexes: newIndexCache(f, c.indexTTL), probe: httpsrc.New(f)}
}

var _ sources.ChartPackageReader = (*PackageAdapter)(nil)

// Register registers the package adapter for the helm-repo and chart-tgz
// locator kinds (chart package reader), plus the chart-tgz probe.
func (a *PackageAdapter) Register(reg *sources.Registry) {
	reg.RegisterChartPackageReader(catalog.LocatorHelmRepo, a)
	reg.RegisterChartPackageReader(catalog.LocatorChartTGZ, a)
	reg.RegisterProbe(catalog.LocatorChartTGZ, a)
}

// Probe implements sources.ArtifactProbe for chart-tgz locators: the URL has
// the artifact version rendered into it already; the check is the http
// adapter's HEAD (with GET fallback), and the evidence records the URL.
func (a *PackageAdapter) Probe(ctx context.Context, ch catalog.Locator, version string) (*sources.ProbeResult, error) {
	if ch.Kind != catalog.LocatorChartTGZ {
		return nil, fmt.Errorf("chart-tgz: probe is not implemented for kind %q", ch.Kind)
	}
	return a.probe.Probe(ctx, catalog.Locator{Kind: catalog.LocatorHTTP, URL: ch.URL}, version)
}

// ReadChartPackage implements sources.ChartPackageReader. For helm-repo
// locators the chart version is looked up in the index (textual, then
// semantic equality); a version the reachable index does not list wraps
// fetch.ErrNotFound. For chart-tgz locators the URL is fetched directly.
//
// The archive digest is the sha256 of the downloaded bytes; for helm-repo it
// is verified against the index entry's digest when the index carries one
// (a mismatch is an error, not a silent difference). The representation is
// published-chart-tgz, except for release-asset URLs (URLs containing
// "/releases/download/"), which are labelled release-asset: the bytes are the
// same kind of package, but they reached users as a release attachment.
func (a *PackageAdapter) ReadChartPackage(ctx context.Context, loc catalog.Locator, version string) (*sources.ChartPackage, error) {
	switch loc.Kind {
	case catalog.LocatorHelmRepo:
		return a.fromIndex(ctx, loc, version)
	case catalog.LocatorChartTGZ:
		if strings.TrimSpace(loc.URL) == "" {
			return nil, fmt.Errorf("chart-tgz: locator has no url")
		}
		return a.download(ctx, loc.URL, version, "")
	default:
		return nil, fmt.Errorf("chart package: unsupported kind %q", loc.Kind)
	}
}

// fromIndex reads the package of loc.Chart at version through the
// repository's index.
func (a *PackageAdapter) fromIndex(ctx context.Context, loc catalog.Locator, version string) (*sources.ChartPackage, error) {
	if strings.TrimSpace(version) == "" {
		return nil, fmt.Errorf("helm-repo: empty chart version")
	}
	if strings.TrimSpace(loc.Chart) == "" {
		return nil, fmt.Errorf("helm-repo: locator has no chart")
	}
	idx, err := a.indexes.get(ctx, loc.URL)
	if err != nil {
		return nil, err
	}
	e, ok := findEntry(idx.Entries[loc.Chart], version)
	if !ok {
		return nil, fmt.Errorf("helm-repo: %s lists no %s version %s: %w", idx.URL, loc.Chart, version, fetch.ErrNotFound)
	}
	url := idx.ChartURL(e)
	if url == "" {
		return nil, fmt.Errorf("helm-repo: index entry %s %s has no URL", loc.Chart, e.Version)
	}
	pkg, err := a.download(ctx, url, e.Version, normalizeDigest(e.Digest))
	if err != nil {
		return nil, err
	}
	// The index entry itself is evidence of where the URL and the digest
	// came from.
	idxEv := idx.evidence(loc.Chart, e)
	pkg.Evidence = append([]domain.Evidence{idxEv}, pkg.Evidence...)
	return pkg, nil
}

// download fetches and unpacks the tarball at url. expectDigest, when set, is
// the digest the archive must have (helm index digests are the sha256 of the
// archive; domain.Digest produces the same "sha256:<hex>" form).
func (a *PackageAdapter) download(ctx context.Context, url, version, expectDigest string) (*sources.ChartPackage, error) {
	doc, err := a.f.Do(ctx, fetch.Request{URL: url, Immutable: true})
	if err != nil {
		return nil, fmt.Errorf("chart package %s: %w", url, err)
	}
	if expectDigest != "" && doc.Digest != expectDigest {
		return nil, fmt.Errorf("chart package %s: digest mismatch (index says %s, downloaded %s)", url, expectDigest, doc.Digest)
	}
	arc, err := ReadArchive(bytes.NewReader(doc.Body))
	if err != nil {
		return nil, fmt.Errorf("chart package %s: %w", url, err)
	}
	rep := domain.RepresentationChartTGZ
	evKind := domain.EvidenceDocument
	if strings.Contains(url, "/releases/download/") {
		rep = domain.RepresentationReleaseAsset
		evKind = domain.EvidenceReleaseAsset
	}
	pkg := &sources.ChartPackage{
		Representation: rep,
		URI:            url,
		Coordinate:     url,
		Digest:         doc.Digest,
		RetrievedAt:    doc.RetrievedAt,
		ChartDir:       arc.ChartDir,
		ChartYAML:      arc.ChartYAML,
		Values:         arc.Values,
		Archive:        doc.Body,
	}
	for _, f := range arc.CRDs {
		pkg.CRDs = append(pkg.CRDs, sources.PackageFile{Path: f.Path, Content: f.Content})
	}
	for _, f := range arc.Templates {
		pkg.Templates = append(pkg.Templates, sources.PackageFile{Path: f.Path, Content: f.Content})
	}
	if arc.ChartMeta != nil {
		pkg.Evidence = append(pkg.Evidence, domain.NewEvidence(evKind, "", url, "archive:"+arc.ChartDir+"/Chart.yaml",
			fmt.Sprintf("chart package %s %s (name: %s, appVersion: %s)", arc.ChartMeta.Name, arc.ChartMeta.Version, arc.ChartMeta.Name, arc.ChartMeta.AppVersion),
			doc.Digest, doc.RetrievedAt).WithRepresentation(rep))
	} else {
		pkg.Evidence = append(pkg.Evidence, domain.NewEvidence(evKind, "", url, "archive",
			fmt.Sprintf("chart package version %s", version), doc.Digest, doc.RetrievedAt).WithRepresentation(rep))
	}
	return pkg, nil
}
