// Package helm implements the Helm-related locator kinds:
//
//   - "helm-repo": a classic HTTP chart repository (index.yaml). Served by
//     RepoAdapter as sources.VersionIndex, sources.ArtifactProbe and
//     sources.VersionLister.
//   - "helm-git": a chart that lives in a (mono-)repository and is released by
//     git tags such as "argo-cd-10.9.5". Served by GitAdapter as
//     sources.VersionIndex and sources.ArtifactProbe. It is built on the
//     git-tags and repo-file ports, not on concrete packages.
//
// Charts published to OCI registries are handled by package oci (locator kind
// "oci"), not here.
//
// Every HTTP request goes through a fetch.Client; the helm-git adapter never
// does HTTP itself.
package helm

import (
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// Defaults.
const (
	// DefaultIndexTTL is how long a fetched index.yaml is reused from the
	// fetch cache. Indexes are mutable (new releases are appended).
	DefaultIndexTTL = 1 * time.Hour
	// DefaultConcurrency bounds parallel Chart.yaml reads of the helm-git
	// adapter.
	DefaultConcurrency = 8
)

type config struct {
	indexTTL    time.Duration
	maxTags     int
	concurrency int
}

func newConfig(opts []Option) config {
	c := config{indexTTL: DefaultIndexTTL, concurrency: DefaultConcurrency}
	for _, o := range opts {
		o(&c)
	}
	if c.concurrency < 1 {
		c.concurrency = 1
	}
	return c
}

// Option configures the adapters of this package.
type Option func(*config)

// WithIndexTTL sets how long an index.yaml may be served from the fetch cache
// (helm-repo).
func WithIndexTTL(d time.Duration) Option { return func(c *config) { c.indexTTL = d } }

// WithMaxTags caps how many chart tags the helm-git adapter inspects in
// ListArtifactVersions: only the n newest tags (by semantic version) have
// their Chart.yaml read, older tags are listed without Fields. n <= 0 (the
// default) inspects every tag.
//
// Trade-off: argo-helm has ~790 argo-cd tags, so a full scan costs ~790
// Chart.yaml reads on a cold cache (they are immutable and cached forever
// afterwards). Capping bounds that cold cost, but a lookup such as "chart
// versions whose appVersion is v2.6.0" can then no longer find releases older
// than the cap. Probe is not affected by the cap: it reads exactly one
// Chart.yaml.
func WithMaxTags(n int) Option { return func(c *config) { c.maxTags = n } }

// WithConcurrency bounds the number of parallel Chart.yaml reads (helm-git).
// The default is DefaultConcurrency.
func WithConcurrency(n int) Option { return func(c *config) { c.concurrency = n } }

// Register registers the helm-repo adapter (version lister, version index and
// probe), the chart-package adapter (helm-repo and chart-tgz packages, plus
// the chart-tgz probe) and, when both ports are given, the helm-git adapter
// (version index and probe) in reg.
//
// f serves helm-repo and the packages. gitTags (a "git-tags" version lister)
// and repoFiles (a "repo-file" document fetcher) serve helm-git; when either
// is nil helm-git is not registered.
func Register(reg *sources.Registry, f fetch.Client, gitTags sources.VersionLister, repoFiles sources.DocumentFetcher, opts ...Option) {
	repo := NewRepoAdapter(f, opts...)
	reg.RegisterVersionLister(catalog.LocatorHelmRepo, repo)
	reg.RegisterVersionIndex(catalog.LocatorHelmRepo, repo)
	reg.RegisterProbe(catalog.LocatorHelmRepo, repo)

	NewPackageAdapter(f, opts...).Register(reg)

	if gitTags == nil || repoFiles == nil {
		return
	}
	git := NewGitAdapter(gitTags, repoFiles, opts...)
	reg.RegisterVersionIndex(catalog.LocatorHelmGit, git)
	reg.RegisterProbe(catalog.LocatorHelmGit, git)
}
