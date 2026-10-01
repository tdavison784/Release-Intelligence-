// Package oci implements the "oci" locator kind: container images and OCI
// artifacts (including Helm charts stored in OCI registries) reached through
// the OCI Distribution v2 API.
//
// An Adapter implements two ports from package sources:
//
//   - sources.ArtifactProbe: does the manifest of a tag exist? (HEAD, with a
//     GET fallback)
//   - sources.VersionIndex: which tags does the repository carry? (tags/list
//     with Link pagination, noise tags such as cosign signatures filtered)
//
// plus a few exported helpers for adjacent needs (ListTags, GetManifest,
// ChartConfig, IsNoiseTag).
//
// # Authentication
//
// Only anonymous access is supported. Registries that answer 401 with a
// "WWW-Authenticate: Bearer realm=...,service=...,scope=..." challenge (Docker
// Hub, ghcr.io, gcr.io for some paths, ...) are served by obtaining a pull
// token from the realm and repeating the request. Registries that allow
// anonymous access directly never take that path.
//
// Tokens are kept in an in-memory cache inside the Adapter and are NEVER
// written to the fetch cache: the token endpoint is called through a
// derived fetch client without a cache (see New). Registry API responses
// themselves (manifest HEADs, tag lists, config blobs) do go through the
// regular fetch client and cache; the cache key does not include
// Authorization, so an --offline replay of a cached run needs no token.
//
// # Caching
//
// Manifests addressed by tag are mutable (a tag can move) and get a short TTL
// (DefaultManifestTTL); manifests and blobs addressed by digest are immutable.
// Tag lists use DefaultTagsTTL. Note that fetch remembers 404 responses for
// fetch.NegativeTTL (1h) regardless of these TTLs.
//
// # Errors
//
// A registry that cannot be consulted (network policy, TLS, auth required,
// rate limiting, 5xx) yields an error wrapping fetch.ErrUnavailable. A
// missing manifest is NOT an error for Probe (ProbeResult.Exists is false); a
// missing repository is fetch.ErrNotFound for the listing calls.
package oci

import (
	"net/http"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// Default cache lifetimes.
const (
	// DefaultManifestTTL bounds how long a tag-addressed manifest probe is
	// reused. Tags are mutable, so this is deliberately short.
	DefaultManifestTTL = 30 * time.Minute
	// DefaultTagsTTL bounds how long a tags/list page is reused.
	DefaultTagsTTL = 1 * time.Hour
)

// tagsPageSize is the "n" parameter of tags/list requests.
const tagsPageSize = 1000

// maxTagPages bounds Link pagination (a safety net against loops).
const maxTagPages = 500

// manifestAccept is the Accept header sent for manifests: OCI index, OCI
// manifest, Docker manifest list and Docker v2 manifest.
const manifestAccept = "application/vnd.oci.image.index.v1+json, " +
	"application/vnd.oci.image.manifest.v1+json, " +
	"application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.docker.distribution.manifest.v2+json"

// Adapter serves the "oci" locator kind. The zero value is not usable; use
// New. An Adapter is safe for concurrent use.
type Adapter struct {
	f fetch.Client
	// tokenFetch, when set (WithTokenClient), is used for token endpoints
	// instead of the default described on tokenClient.
	tokenFetch fetch.Client

	manifestTTL time.Duration
	tagsTTL     time.Duration
	noiseFilter bool
	endpoints   map[string]string // registry host -> API base URL override
	now         func() time.Time

	mu     sync.Mutex
	tokens map[string]token
}

// Option configures an Adapter.
type Option func(*Adapter)

// WithManifestTTL sets the cache lifetime of tag-addressed manifest probes.
func WithManifestTTL(d time.Duration) Option { return func(a *Adapter) { a.manifestTTL = d } }

// WithTagsTTL sets the cache lifetime of tag listings.
func WithTagsTTL(d time.Duration) Option { return func(a *Adapter) { a.tagsTTL = d } }

// WithNoiseFilter switches the default filtering of noise tags (cosign
// signatures, commit-SHA tags) in ListArtifactVersions on or off. It is on by
// default.
func WithNoiseFilter(enabled bool) Option { return func(a *Adapter) { a.noiseFilter = enabled } }

// WithTokenClient overrides the client used for token endpoints. It must not
// persist responses.
func WithTokenClient(c fetch.Client) Option { return func(a *Adapter) { a.tokenFetch = c } }

// WithEndpoint maps a registry host as it appears in locators to the base URL
// of its Distribution API ("https://mirror.example.com"), for mirrors, proxies
// and tests.
func WithEndpoint(registry, baseURL string) Option {
	return func(a *Adapter) { a.endpoints[registry] = baseURL }
}

// New returns an Adapter that issues all HTTP requests through f.
func New(f fetch.Client, opts ...Option) *Adapter {
	a := &Adapter{
		f:           f,
		manifestTTL: DefaultManifestTTL,
		tagsTTL:     DefaultTagsTTL,
		noiseFilter: true,
		endpoints:   map[string]string{},
		now:         time.Now,
		tokens:      map[string]token{},
	}
	for _, o := range opts {
		o(a)
	}
	return a
}

// tokenClient returns the client used for token endpoints: the override from
// WithTokenClient, otherwise the adapter's client. Token requests are always
// sent with fetch.Request.NoStore so they are never cached or persisted.
func (a *Adapter) tokenClient() fetch.Client {
	if a.tokenFetch != nil {
		return a.tokenFetch
	}
	return a.f
}

// Register registers the adapter for the "oci" locator kind as both
// ArtifactProbe and VersionIndex.
func Register(reg *sources.Registry, f fetch.Client) {
	a := New(f)
	reg.RegisterProbe(catalog.LocatorOCI, a)
	reg.RegisterVersionIndex(catalog.LocatorOCI, a)
}

// repository parses a locator repository and applies endpoint overrides.
func (a *Adapter) repository(s string) (Repository, error) {
	r, err := ParseRepository(s)
	if err != nil {
		return Repository{}, err
	}
	if base, ok := a.endpoints[r.Registry]; ok {
		r.APIBase = base
	}
	return r, nil
}

// header returns a request header carrying the given Accept value.
func accept(v string) http.Header { return http.Header{"Accept": []string{v}} }

var (
	_ sources.ArtifactProbe = (*Adapter)(nil)
	_ sources.VersionIndex  = (*Adapter)(nil)
)
