// Package httpsrc implements the "http" locator kind: plain HTTP(S) resources
// that are either read as documents (an install manifest, a values file, a
// compatibility table) or probed for existence (a release asset attached to
// a GitHub release, a chart tarball).
//
// All requests go through fetch.Client, so they are cached and can be
// replayed offline. Errors keep the semantics of package fetch: a 404 is
// fetch.ErrNotFound for documents and a negative result (not an error) for
// probes; an unreachable host wraps fetch.ErrUnavailable.
package httpsrc

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/gitsrc"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// Adapter implements sources.DocumentFetcher and sources.ArtifactProbe for
// the http locator kind.
type Adapter struct {
	fetch fetch.Client
	now   func() time.Time
}

// New returns an adapter reading through f.
func New(f fetch.Client) *Adapter {
	return &Adapter{fetch: f, now: time.Now}
}

// Register registers an adapter for the http locator kind as document
// fetcher and artifact probe.
func Register(reg *sources.Registry, f fetch.Client) {
	a := New(f)
	reg.RegisterDocumentFetcher(catalog.LocatorHTTP, a)
	reg.RegisterProbe(catalog.LocatorHTTP, a)
}

// IsImmutableURL reports whether the content behind rawURL can be assumed
// never to change, so that it may be cached without expiry. That is the case
// when
//
//   - the path contains "/releases/download/" (assets attached to a release
//     of a code host: .../releases/download/v1.18.0/cert-manager.yaml), or
//   - a directory segment of the path is a commit id or a version tag, as
//     judged by gitsrc.IsImmutableRef (.../argo-cd/v3.0.0/manifests/install.yaml,
//     .../releases/1.20.0/charts/index.yaml).
//
// The final path segment (the file name) and the query string are not
// considered. Everything else uses the fetch client's TTL.
func IsImmutableURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	p := u.Path
	if strings.Contains(p, "/releases/download/") {
		return true
	}
	segs := strings.Split(strings.Trim(p, "/"), "/")
	if len(segs) < 2 {
		return false
	}
	for _, s := range segs[:len(segs)-1] {
		if gitsrc.IsImmutableRef(s) {
			return true
		}
	}
	return false
}

// parseURL validates an absolute http(s) URL.
func parseURL(kind, raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%s: url is required", kind)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid url %q: %w", kind, raw, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%s: url %q must be an absolute http or https URL", kind, raw)
	}
	return u, nil
}

// FetchDocument implements sources.DocumentFetcher: it GETs loc.URL. The
// document's URI and FetchURL are the requested URL (not the final URL after
// redirects, which for release assets is a short-lived signed address).
// Format follows the file extension, then the response's Content-Type, and
// is "text" otherwise.
func (a *Adapter) FetchDocument(ctx context.Context, loc catalog.Locator) (*sources.Document, error) {
	u, err := parseURL("http", loc.URL)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(loc.URL)
	doc, err := a.fetch.Do(ctx, fetch.Request{URL: raw, Immutable: IsImmutableURL(raw)})
	if err != nil {
		return nil, err
	}
	return &sources.Document{
		Locator:     loc,
		URI:         raw,
		FetchURL:    raw,
		Content:     doc.Body,
		Format:      formatFor(u.Path, doc.Header.Get("Content-Type")),
		Digest:      doc.Digest,
		RetrievedAt: doc.RetrievedAt,
	}, nil
}

// formatFor derives the document format from the path's extension and, when
// that says nothing, from the media type.
func formatFor(p, contentType string) string {
	if f := gitsrc.FormatFromPath(p); f != "text" {
		return f
	}
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "text"
	}
	switch {
	case mt == "application/json", strings.HasSuffix(mt, "+json"):
		return "json"
	case strings.Contains(mt, "yaml"):
		return "yaml"
	case mt == "text/markdown", mt == "text/x-markdown":
		return "markdown"
	case mt == "text/html", mt == "application/xhtml+xml":
		return "html"
	}
	return "text"
}

// headFallbackStatuses are the statuses of a HEAD request that say "this
// server does not do HEAD (here)" rather than anything about the asset:
// servers and CDNs that reject HEAD answer 400, 403 (signed URLs are
// method-specific), 405, 406 or 501.
var headFallbackStatuses = map[int]bool{
	http.StatusBadRequest:       true,
	http.StatusForbidden:        true,
	http.StatusMethodNotAllowed: true,
	http.StatusNotAcceptable:    true,
	http.StatusNotImplemented:   true,
}

// Probe implements sources.ArtifactProbe: it checks whether the asset at
// ch.URL exists. The version has already been rendered into the URL and is
// informational only.
//
// The check is a HEAD request, repeated as GET when the server rejects HEAD.
// A 2xx answer yields Exists (with the body's Digest when a GET was needed);
// a 404 or 410 yields Exists == false and no error, because the channel
// answered and the artifact is absent; an unreachable server, an
// authentication failure or rate limiting is an error wrapping
// fetch.ErrUnavailable. In both cases the result carries release-asset
// evidence recording what the server said.
//
// A GET fallback reads the whole body, which the fetch client bounds
// (fetch.HTTPClient.MaxBody); an asset larger than that on a server without
// HEAD support fails with a plain error.
func (a *Adapter) Probe(ctx context.Context, ch catalog.Locator, version string) (*sources.ProbeResult, error) {
	if _, err := parseURL("http probe", ch.URL); err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(ch.URL)
	immutable := IsImmutableURL(raw)

	method := http.MethodHead
	doc, err := a.fetch.Do(ctx, fetch.Request{Method: http.MethodHead, URL: raw, Immutable: immutable})
	var fe *fetch.Error
	if err != nil && errors.As(err, &fe) && headFallbackStatuses[fe.Status] {
		method = http.MethodGet
		doc, err = a.fetch.Do(ctx, fetch.Request{URL: raw, Immutable: immutable})
	}

	switch {
	case err == nil:
		digest := ""
		if method == http.MethodGet {
			digest = doc.Digest
		}
		excerpt := fmt.Sprintf("%s %s -> HTTP %d", method, raw, doc.Status)
		if ct := doc.Header.Get("Content-Type"); ct != "" {
			excerpt += " " + ct
		}
		ev := domain.NewEvidence(domain.EvidenceReleaseAsset, "", raw, "", excerpt, digest, doc.RetrievedAt)
		return &sources.ProbeResult{Exists: true, Coordinate: raw, Digest: digest, URI: raw, Evidence: ev}, nil
	case errors.Is(err, fetch.ErrNotFound):
		status := http.StatusNotFound
		if errors.As(err, &fe) && fe.Status != 0 {
			status = fe.Status
		}
		excerpt := fmt.Sprintf("%s %s -> HTTP %d", method, raw, status)
		ev := domain.NewEvidence(domain.EvidenceReleaseAsset, "", raw, "", excerpt, "", a.now())
		return &sources.ProbeResult{Exists: false, Coordinate: raw, URI: raw, Evidence: ev}, nil
	default:
		return nil, fmt.Errorf("http probe %s: %w", raw, err)
	}
}
