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
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
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

	// The following refine ErrUnavailable: errors.Is(err, ErrUnavailable)
	// stays true for all of them (callers that only care about "could not be
	// checked" keep working), while errors.Is(err, ErrThrottled) etc. tell
	// WHY. The reason is also the first words of the error detail.

	// ErrThrottled: the server is reachable but is rate limiting us (HTTP 429,
	// 503 with Retry-After, GitHub's exhausted primary quota). Not "down":
	// retrying later is expected to work.
	ErrThrottled = &unavailableKind{"throttled"}
	// ErrAuthRequired: HTTP 401 - the host answered and wants credentials.
	ErrAuthRequired = &unavailableKind{"authentication required"}
	// ErrForbidden: HTTP 403 that is not a rate limit - the host answered and
	// refuses this client (an egress policy, a denied anonymous pull).
	ErrForbidden = &unavailableKind{"forbidden"}
	// ErrDNS: the host name does not resolve.
	ErrDNS = &unavailableKind{"DNS resolution failed"}
)

// unavailableKind is a reason for unavailability that is-a ErrUnavailable.
type unavailableKind struct{ reason string }

func (k *unavailableKind) Error() string { return "unavailable: " + k.reason }

func (k *unavailableKind) Is(target error) bool { return target == ErrUnavailable }

// Error wraps a failed fetch with its URL and status.
type Error struct {
	URL    string
	Status int
	Err    error // one of the sentinels, or a transport error
	Detail string
	// Header holds the response headers of an HTTP error response (e.g.
	// WWW-Authenticate on a 401), nil for transport errors and cache hits.
	Header http.Header
	// RetryAfter is the server-requested wait of a throttled response (zero
	// when none was given).
	RetryAfter time.Duration
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
	// MaxRetries is how many times a throttled (429, 503 with Retry-After,
	// exhausted GitHub quota) or transiently failing (502/503/504) idempotent
	// request is retried (default 3; negative disables retries).
	MaxRetries int
	// MaxRetryWait caps one wait, whether taken from Retry-After or from the
	// exponential backoff (default 30s). A longer Retry-After is not waited
	// out: the request fails as throttled and reports the server's value.
	MaxRetryWait time.Duration
	// BackoffBase is the first exponential backoff step (default 1s).
	BackoffBase time.Duration
	// HostConcurrency bounds in-flight requests per host (default 6);
	// HostLimits overrides it for specific hosts.
	HostConcurrency int
	HostLimits      map[string]int

	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error // tests replace it

	gateMu sync.Mutex
	gates  map[string]*hostGate
}

// DefaultHostLimits are per-host concurrency overrides for registries known
// to throttle bursts (public.ecr.aws answered 429 "Rate exceeded" to a
// handful of parallel manifest reads while sequential reads all succeed).
var DefaultHostLimits = map[string]int{"public.ecr.aws": 2}

// hostGate serialises access to one host: a concurrency semaphore plus a
// "not before" time that every request waits for after the host throttled.
type hostGate struct {
	sem       chan struct{}
	mu        sync.Mutex
	notBefore time.Time
}

// NewHTTPClient returns a client using http.DefaultTransport (which honours
// HTTPS_PROXY) and the given cache.
func NewHTTPClient(cache *Cache, mode Mode) *HTTPClient {
	return &HTTPClient{
		HTTP:       &http.Client{Timeout: 60 * time.Second},
		Cache:      cache,
		Mode:       mode,
		UserAgent:  "release-intelligence/0.1 (+https://github.com/tdavison784/release-intelligence)",
		MaxBody:    64 << 20,
		HostLimits: DefaultHostLimits,
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
	doc, status, err := c.doRetrying(ctx, req)
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

func (c *HTTPClient) doOnce(ctx context.Context, req Request) (*Document, int, error) {
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
		var dns *net.DNSError
		if errors.As(err, &dns) {
			return nil, 0, &Error{URL: req.URL, Err: ErrDNS, Detail: err.Error()}
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
	case resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode == http.StatusServiceUnavailable && resp.Header.Get("Retry-After") != "") || githubQuotaExhausted(resp):
		ra := retryAfter(resp.Header, c.clock())
		return nil, resp.StatusCode, &Error{URL: req.URL, Status: resp.StatusCode, Err: ErrThrottled, Detail: throttleDetail(ra, body), Header: resp.Header.Clone(), RetryAfter: ra}
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, resp.StatusCode, &Error{URL: req.URL, Status: resp.StatusCode, Err: ErrAuthRequired, Detail: authDetail(resp.Header, body), Header: resp.Header.Clone()}
	case resp.StatusCode == http.StatusForbidden:
		return nil, resp.StatusCode, &Error{URL: req.URL, Status: resp.StatusCode, Err: ErrForbidden, Detail: snippet(body), Header: resp.Header.Clone()}
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

// githubQuotaExhausted reports GitHub's primary rate limit, which answers 403
// (not 429) with X-RateLimit-Remaining: 0, and its secondary limit, a 403
// carrying Retry-After.
func githubQuotaExhausted(resp *http.Response) bool {
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	return resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != ""
}

// retryAfter parses Retry-After (delta seconds or an HTTP date) and, failing
// that, GitHub's X-RateLimit-Reset (epoch seconds). Zero when absent.
func retryAfter(h http.Header, now time.Time) time.Duration {
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
		if at, err := http.ParseTime(v); err == nil {
			if d := at.Sub(now); d > 0 {
				return d
			}
			return 0
		}
	}
	if h.Get("X-RateLimit-Remaining") == "0" {
		if epoch, err := strconv.ParseInt(strings.TrimSpace(h.Get("X-RateLimit-Reset")), 10, 64); err == nil {
			if d := time.Unix(epoch, 0).Sub(now); d > 0 {
				return d
			}
		}
	}
	return 0
}

func throttleDetail(ra time.Duration, body []byte) string {
	s := snippet(body)
	if ra > 0 {
		s = fmt.Sprintf("retry after %s: %s", ra.Round(time.Second), s)
	}
	return strings.TrimSuffix(s, ": ")
}

// authDetail names the authentication challenge, so a registry that wants a
// token reads differently from a host that refuses outright.
func authDetail(h http.Header, body []byte) string {
	if ch := h.Get("Www-Authenticate"); ch != "" {
		return "challenge " + snippet([]byte(ch))
	}
	return snippet(body)
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// Reason names why a resource was unavailable: "throttled", "authentication
// required", "forbidden", "DNS resolution failed" or "" (plain unreachable,
// not applicable). It reads either an error or the text of one (a stored
// SourceStatus.Detail), so reports can say WHY without keeping the error.
func Reason(v any) string {
	var s string
	switch x := v.(type) {
	case error:
		if x == nil {
			return ""
		}
		for _, k := range []*unavailableKind{ErrThrottled, ErrAuthRequired, ErrForbidden, ErrDNS} {
			if errors.Is(x, k) {
				return k.reason
			}
		}
		s = x.Error()
	case string:
		s = x
	}
	for _, k := range []*unavailableKind{ErrThrottled, ErrAuthRequired, ErrForbidden, ErrDNS} {
		if strings.Contains(s, "unavailable: "+k.reason) {
			return k.reason
		}
	}
	return ""
}

// StateFor maps a fetch error to a source state.
func StateFor(err error) domain.SourceState {
	switch {
	case err == nil:
		return domain.SourceOK
	case errors.Is(err, ErrNotFound):
		return domain.SourceNotFound
	case errors.Is(err, ErrThrottled):
		return domain.SourceThrottled
	case errors.Is(err, ErrUnavailable), errors.Is(err, ErrOffline):
		return domain.SourceUnavailable
	default:
		return domain.SourceError
	}
}

// doRetrying wraps doOnce with per-host concurrency limiting and bounded
// retries. Only GET/HEAD are retried (they are idempotent); throttled
// responses wait for Retry-After (capped by MaxRetryWait) or back off
// exponentially, and a throttled host makes later requests to it wait too.
func (c *HTTPClient) doRetrying(ctx context.Context, req Request) (*Document, int, error) {
	host := hostOf(req.URL)
	gate := c.gate(host)
	retries := c.MaxRetries
	if c.MaxRetries == 0 {
		retries = 3
	} else if retries < 0 {
		retries = 0
	}
	maxWait := c.MaxRetryWait
	if maxWait <= 0 {
		maxWait = 30 * time.Second
	}
	base := c.BackoffBase
	if base <= 0 {
		base = time.Second
	}
	for attempt := 0; ; attempt++ {
		if err := gate.acquire(ctx, c); err != nil {
			return nil, 0, &Error{URL: req.URL, Err: err}
		}
		doc, status, err := c.doOnce(ctx, req)
		gate.release()
		var fe *Error
		if err == nil || !errors.As(err, &fe) || ctx.Err() != nil {
			return doc, status, err
		}
		throttled := errors.Is(err, ErrThrottled)
		transient := status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
		if !(throttled || transient) || (req.Method != http.MethodGet && req.Method != http.MethodHead) {
			return doc, status, err
		}
		wait := fe.RetryAfter
		if wait == 0 {
			wait = base << attempt
		}
		if wait > maxWait {
			if throttled {
				gate.pause(c.clock().Add(wait)) // later requests respect the server's ask
			}
			return doc, status, err
		}
		if throttled {
			gate.pause(c.clock().Add(wait))
		}
		if attempt >= retries {
			return doc, status, err
		}
		if err := c.pause(ctx, wait); err != nil {
			return nil, 0, &Error{URL: req.URL, Err: err}
		}
	}
}

func hostOf(rawurl string) string {
	s := rawurl
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}

func (c *HTTPClient) gate(host string) *hostGate {
	c.gateMu.Lock()
	defer c.gateMu.Unlock()
	if c.gates == nil {
		c.gates = map[string]*hostGate{}
	}
	g := c.gates[host]
	if g == nil {
		n := c.HostConcurrency
		if n <= 0 {
			n = 6
		}
		if l, ok := c.HostLimits[host]; ok && l > 0 {
			n = l
		}
		g = &hostGate{sem: make(chan struct{}, n)}
		c.gates[host] = g
	}
	return g
}

func (g *hostGate) acquire(ctx context.Context, c *HTTPClient) error {
	select {
	case g.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	g.mu.Lock()
	wait := g.notBefore.Sub(c.clock())
	g.mu.Unlock()
	if wait > 0 {
		if err := c.pause(ctx, wait); err != nil {
			<-g.sem
			return err
		}
	}
	return nil
}

func (g *hostGate) release() { <-g.sem }

func (g *hostGate) pause(until time.Time) {
	g.mu.Lock()
	if until.After(g.notBefore) {
		g.notBefore = until
	}
	g.mu.Unlock()
}

func (c *HTTPClient) pause(ctx context.Context, d time.Duration) error {
	if c.sleep != nil {
		return c.sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
