// Package sources defines the ports between the generic ingestion pipeline
// and external systems. Adapters (github, gitsrc, helm, oci, ...) implement
// these interfaces for one or more catalog locator kinds and are registered in
// a Registry. Ingestion code only ever talks to a Registry, never to a
// concrete adapter, so new hosting platforms (GitLab, other registries) can
// be added without touching the pipeline or the domain model.
//
// All locators passed to adapters are already rendered (no templates left).
package sources

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ReleaseRef is a release as listed by a versions source. Tag is raw; the
// pipeline parses it with the product's VersionParser and drops tags that do
// not match (mono-repos often carry tags of several components).
type ReleaseRef struct {
	Tag         string          `json:"tag"`
	PublishedAt *time.Time      `json:"publishedAt,omitempty"`
	Prerelease  bool            `json:"prerelease,omitempty"` // flagged as prerelease by the source itself
	Draft       bool            `json:"draft,omitempty"`
	Commit      string          `json:"commit,omitempty"`
	URL         string          `json:"url,omitempty"` // human-facing URL
	Evidence    domain.Evidence `json:"evidence"`
}

// VersionLister lists releases from a canonical versions source
// (github-releases, git-tags, helm-repo).
type VersionLister interface {
	ListReleases(ctx context.Context, loc catalog.Locator) ([]ReleaseRef, error)
}

// Document is a retrieved text document.
type Document struct {
	Locator     catalog.Locator `json:"locator"`
	URI         string          `json:"uri"`      // human-facing, stable URI (e.g. a blob URL pinned to ref)
	FetchURL    string          `json:"fetchUrl"` // URL actually fetched
	Path        string          `json:"path,omitempty"`
	Content     []byte          `json:"-"`
	Format      string          `json:"format"` // "markdown", "yaml", "json", "html", "text"
	Digest      string          `json:"digest"`
	RetrievedAt time.Time       `json:"retrievedAt"`
}

// DocumentFetcher retrieves single documents (repo-file, http,
// github-releases with Ref = tag → release body as markdown).
type DocumentFetcher interface {
	FetchDocument(ctx context.Context, loc catalog.Locator) (*Document, error)
}

// DirectoryFetcher retrieves every file under loc.Path matching loc.Glob at
// loc.Ref (repo-dir). Files are returned sorted by path.
type DirectoryFetcher interface {
	FetchDirectory(ctx context.Context, loc catalog.Locator) ([]Document, error)
}

// ProbeResult is the outcome of checking an artifact at a channel.
type ProbeResult struct {
	Exists     bool            `json:"exists"`
	Coordinate string          `json:"coordinate"` // canonical coordinate, e.g. "quay.io/jetstack/cert-manager-controller:v1.18.0"
	Digest     string          `json:"digest,omitempty"`
	URI        string          `json:"uri,omitempty"`
	Evidence   domain.Evidence `json:"evidence"`
}

// ArtifactProbe checks whether an artifact version exists at a channel
// (oci, helm-repo, helm-git, http).
type ArtifactProbe interface {
	Probe(ctx context.Context, ch catalog.Locator, version string) (*ProbeResult, error)
}

// ArtifactVersion is one entry of an artifact index.
type ArtifactVersion struct {
	Version  string            `json:"version"`
	Fields   map[string]string `json:"fields,omitempty"` // e.g. "appVersion": "v3.0.0", "created": RFC3339
	Digest   string            `json:"digest,omitempty"`
	URI      string            `json:"uri,omitempty"`
	Evidence domain.Evidence   `json:"evidence"`
}

// VersionIndex lists the versions available at a channel together with
// metadata, used by the "lookup" version relation (e.g. find the chart
// versions whose appVersion equals the release tag).
type VersionIndex interface {
	ListArtifactVersions(ctx context.Context, ch catalog.Locator) ([]ArtifactVersion, error)
}

// AdvisorySource lists advisories (github-advisories). The returned evidence
// records are those referenced by Advisory.Evidence.
type AdvisorySource interface {
	ListAdvisories(ctx context.Context, loc catalog.Locator) ([]domain.Advisory, []domain.Evidence, error)
}

// Registry maps locator kinds to adapters.
type Registry struct {
	mu          sync.RWMutex
	versions    map[string]VersionLister
	documents   map[string]DocumentFetcher
	directories map[string]DirectoryFetcher
	probes      map[string]ArtifactProbe
	indexes     map[string]VersionIndex
	advisories  map[string]AdvisorySource
	packages    map[string]ChartPackageReader
	images      map[string]ImageManifestReader
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		versions:    map[string]VersionLister{},
		documents:   map[string]DocumentFetcher{},
		directories: map[string]DirectoryFetcher{},
		probes:      map[string]ArtifactProbe{},
		indexes:     map[string]VersionIndex{},
		advisories:  map[string]AdvisorySource{},
		packages:    map[string]ChartPackageReader{},
		images:      map[string]ImageManifestReader{},
	}
}

// ErrNoAdapter is returned when no adapter serves a locator kind.
type ErrNoAdapter struct {
	Capability string
	Kind       string
}

func (e *ErrNoAdapter) Error() string {
	return fmt.Sprintf("no %s adapter registered for locator kind %q", e.Capability, e.Kind)
}

func (r *Registry) RegisterVersionLister(kind string, a VersionLister) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.versions[kind] = a
}

func (r *Registry) RegisterDocumentFetcher(kind string, a DocumentFetcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.documents[kind] = a
}

func (r *Registry) RegisterDirectoryFetcher(kind string, a DirectoryFetcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.directories[kind] = a
}

func (r *Registry) RegisterProbe(kind string, a ArtifactProbe) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.probes[kind] = a
}

func (r *Registry) RegisterVersionIndex(kind string, a VersionIndex) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.indexes[kind] = a
}

func (r *Registry) RegisterAdvisorySource(kind string, a AdvisorySource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.advisories[kind] = a
}

func (r *Registry) RegisterChartPackageReader(kind string, a ChartPackageReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.packages[kind] = a
}

func (r *Registry) RegisterImageManifestReader(kind string, a ImageManifestReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.images[kind] = a
}

func (r *Registry) VersionLister(kind string) (VersionLister, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a, ok := r.versions[kind]; ok {
		return a, nil
	}
	return nil, &ErrNoAdapter{"version", kind}
}

func (r *Registry) DocumentFetcher(kind string) (DocumentFetcher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a, ok := r.documents[kind]; ok {
		return a, nil
	}
	return nil, &ErrNoAdapter{"document", kind}
}

func (r *Registry) DirectoryFetcher(kind string) (DirectoryFetcher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a, ok := r.directories[kind]; ok {
		return a, nil
	}
	return nil, &ErrNoAdapter{"directory", kind}
}

func (r *Registry) Probe(kind string) (ArtifactProbe, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a, ok := r.probes[kind]; ok {
		return a, nil
	}
	return nil, &ErrNoAdapter{"probe", kind}
}

func (r *Registry) VersionIndex(kind string) (VersionIndex, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a, ok := r.indexes[kind]; ok {
		return a, nil
	}
	return nil, &ErrNoAdapter{"version-index", kind}
}

func (r *Registry) AdvisorySource(kind string) (AdvisorySource, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a, ok := r.advisories[kind]; ok {
		return a, nil
	}
	return nil, &ErrNoAdapter{"advisory", kind}
}

// ChartPackageReader returns the chart-package reader serving locator kind.
func (r *Registry) ChartPackageReader(kind string) (ChartPackageReader, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a, ok := r.packages[kind]; ok {
		return a, nil
	}
	return nil, &ErrNoAdapter{"chart-package", kind}
}

// ImageManifestReader returns the image-manifest reader serving locator kind.
func (r *Registry) ImageManifestReader(kind string) (ImageManifestReader, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a, ok := r.images[kind]; ok {
		return a, nil
	}
	return nil, &ErrNoAdapter{"image-manifest", kind}
}

// Kinds returns the registered locator kinds per capability (for diagnostics).
func (r *Registry) Kinds() map[string][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string][]string{}
	add := func(cap string, keys []string) {
		sort.Strings(keys)
		out[cap] = keys
	}
	add("version", keysOf(r.versions))
	add("document", keysOf(r.documents))
	add("directory", keysOf(r.directories))
	add("probe", keysOf(r.probes))
	add("version-index", keysOf(r.indexes))
	add("advisory", keysOf(r.advisories))
	add("chart-package", keysOf(r.packages))
	add("image-manifest", keysOf(r.images))
	return out
}

func keysOf[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
