// Package fetch provides an HTTP client with a local filesystem cache.
//
// Every external read in the system goes through a Client so that ingestion
// is reproducible: a run made online populates the cache, and the same run can
// later be replayed with ModeOffline (used by tests and golden fixtures).
// Content addressed by immutable coordinates (a tag, a commit, a digest) is
// cached forever; mutable indexes are cached with a TTL.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Sentinel errors. Adapters map these to domain.SourceState values.
var (
	// ErrNotFound: the server answered and the resource does not exist (404/410).
	ErrNotFound = errors.New("not found")
	// ErrUnavailable: the resource could not be reached (network policy,
	// DNS, TLS, 401/403 auth or rate limiting).
	ErrUnavailable = errors.New("unavailable")
	// ErrOffline: offline mode and the resource is not cached.
	ErrOffline = errors.New("not in cache (offline mode)")
)

// Error wraps a failed fetch with its URL and status.
type Error struct {
	URL    string
	Status int
	Err    error // one of the sentinels, or a transport error
	Detail string
	// Header holds the response headers of an HTTP error response (e.g.
	// WWW-Authenticate on a 401), nil for transport errors and cache hits.
	Header http.Header
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("fetch %s: %v", e.URL, e.Err)
	if e.Status != 0 {
		msg += fmt.Sprintf(" (HTTP %d)", e.Status)
	}
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// Mode controls cache behaviour.
type Mode int

const (
	// ModeOnline reads through the cache (fresh entries served from cache).
	ModeOnline Mode = iota
	// ModeOffline serves only from cache, regardless of age.
	ModeOffline
	// ModeRefresh always goes to the network and updates the cache.
	ModeRefresh
)

// Request describes one fetch.
type Request struct {
	URL    string
	Method string // GET (default) or HEAD
	// Header is sent with the request. Accept participates in the cache key;
	// Authorization is never cached or persisted.
	Header http.Header
	// Immutable marks content that never changes for this URL (tag, commit or
	// digest addressed). Immutable entries never expire.
	Immutable bool
	// TTL for mutable content; zero means DefaultTTL.
	TTL time.Duration
	// NegativeTTL bounds how long a cached 404 is honoured; zero means
	// NegativeTTL (the package default). Probes for artifacts that may be
	// published soon should use a short value.
	NegativeTTL time.Duration
	// NoStore disables both reading and writing the cache for this request
	// (e.g. registry token exchanges, which must never be persisted).
	NoStore bool
}

// DefaultTTL for mutable resources.
const DefaultTTL = 6 * time.Hour

// NegativeTTL bounds how long a 404 is remembered.
const NegativeTTL = 1 * time.Hour

// Document is a fetched resource.
type Document struct {
	URL         string      `json:"url"`
	FinalURL    string      `json:"finalUrl,omitempty"`
	Status      int         `json:"status"`
	Header      http.Header `json:"header,omitempty"` // persisted subset only
	Body        []byte      `json:"-"`
	Digest      string      `json:"digest"`
	RetrievedAt time.Time   `json:"retrievedAt"`
	FromCache   bool        `json:"-"`
}

// Client fetches documents.
type Client interface {
	Do(ctx context.Context, req Request) (*Document, error)
}

// Get is a convenience wrapper for a GET request.
func Get(ctx context.Context, c Client, url string, immutable bool) (*Document, error) {
	return c.Do(ctx, Request{URL: url, Immutable: immutable})
}

// persistedHeaders are kept in the cache.
var persistedHeaders = []string{"Content-Type", "Etag", "Last-Modified", "Docker-Content-Digest", "Link", "Location", "Www-Authenticate"}

// HTTPClient is a Client backed by net/http with an optional cache.
type HTTPClient struct {
	HTTP      *http.Client
	Cache     *Cache // may be nil
	Mode      Mode
	UserAgent string
	// MaxBody bounds response sizes (default 64 MiB).
	MaxBody int64
	now     func() time.Time
}

// NewHTTPClient returns a client using http.DefaultTransport (which honours
// HTTPS_PROXY) and the given cache.
func NewHTTPClient(cache *Cache, mode Mode) *HTTPClient {
	return &HTTPClient{
		HTTP:      &http.Client{Timeout: 60 * time.Second},
		Cache:     cache,
		Mode:      mode,
		UserAgent: "release-intelligence/0.1 (+https://github.com/tdavison784/release-intelligence)",
		MaxBody:   64 << 20,
	}
}

func (c *HTTPClient) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// Do implements Client.
func (c *HTTPClient) Do(ctx context.Context, req Request) (*Document, error) {
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	key := cacheKey(req)
	useCache := c.Cache != nil && !req.NoStore
	if useCache && c.Mode != ModeRefresh {
		if ent, err := c.Cache.load(key, req.URL); err == nil && ent != nil {
			fresh := c.Mode == ModeOffline || ent.fresh(req, c.clock())
			if fresh {
				return ent.result(req.URL)
			}
		}
	}
	if c.Mode == ModeOffline && !req.NoStore {
		return nil, &Error{URL: req.URL, Err: ErrOffline}
	}
	if c.Mode == ModeOffline {
		return nil, &Error{URL: req.URL, Err: ErrOffline, Detail: "uncacheable request in offline mode"}
	}
	doc, status, err := c.do(ctx, req)
	if useCache {
		switch {
		case err == nil:
			_ = c.Cache.store(key, req, doc, status)
		case errors.Is(err, ErrNotFound):
			_ = c.Cache.store(key, req, &Document{URL: req.URL, Status: status, RetrievedAt: c.clock().UTC()}, status)
		}
	}
	return doc, err
}

func (c *HTTPClient) do(ctx context.Context, req Request) (*Document, int, error) {
	hr, err := http.NewRequestWithContext(ctx, req.Method, req.URL, nil)
	if err != nil {
		return nil, 0, &Error{URL: req.URL, Err: err}
	}
	for k, vs := range req.Header {
		for _, v := range vs {
			hr.Header.Add(k, v)
		}
	}
	if hr.Header.Get("User-Agent") == "" {
		hr.Header.Set("User-Agent", c.UserAgent)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(hr)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, &Error{URL: req.URL, Err: ctx.Err()}
		}
		return nil, 0, &Error{URL: req.URL, Err: ErrUnavailable, Detail: err.Error()}
	}
	defer resp.Body.Close()
	max := c.MaxBody
	if max <= 0 {
		max = 64 << 20
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, resp.StatusCode, &Error{URL: req.URL, Status: resp.StatusCode, Err: ErrUnavailable, Detail: "read body: " + err.Error()}
	}
	if int64(len(body)) > max {
		return nil, resp.StatusCode, &Error{URL: req.URL, Status: resp.StatusCode, Err: fmt.Errorf("response exceeds %d bytes", max)}
	}
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return nil, resp.StatusCode, &Error{URL: req.URL, Status: resp.StatusCode, Err: ErrNotFound, Header: resp.Header.Clone()}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return nil, resp.StatusCode, &Error{URL: req.URL, Status: resp.StatusCode, Err: ErrUnavailable, Detail: snippet(body), Header: resp.Header.Clone()}
	case resp.StatusCode >= 400:
		return nil, resp.StatusCode, &Error{URL: req.URL, Status: resp.StatusCode, Err: fmt.Errorf("http error"), Detail: snippet(body), Header: resp.Header.Clone()}
	}
	h := http.Header{}
	for _, k := range persistedHeaders {
		if v := resp.Header.Values(k); len(v) > 0 {
			h[k] = v
		}
	}
	doc := &Document{
		URL:         req.URL,
		FinalURL:    resp.Request.URL.String(),
		Status:      resp.StatusCode,
		Header:      h,
		Body:        body,
		Digest:      domain.Digest(body),
		RetrievedAt: c.clock().UTC(),
	}
	if doc.FinalURL == req.URL {
		doc.FinalURL = ""
	}
	return doc, resp.StatusCode, nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// StateFor maps a fetch error to a source state.
func StateFor(err error) domain.SourceState {
	switch {
	case err == nil:
		return domain.SourceOK
	case errors.Is(err, ErrNotFound):
		return domain.SourceNotFound
	case errors.Is(err, ErrUnavailable), errors.Is(err, ErrOffline):
		return domain.SourceUnavailable
	default:
		return domain.SourceError
	}
}
