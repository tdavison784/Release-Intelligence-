// Package github implements the adapters backed by the GitHub REST API:
//
//	github-releases    sources.VersionLister   releases of a repository
//	github-releases    sources.DocumentFetcher the body of the release of a tag
//	github-advisories  sources.AdvisorySource  published repository security advisories
//
// All requests go through fetch.Client (cached, replayable offline) and carry
// the headers GitHub asks for: Accept: application/vnd.github+json,
// X-GitHub-Api-Version, and, when a token is configured, a Bearer
// Authorization header. The token is only ever sent to the configured API
// base URL.
//
// Failures follow package fetch: a 404 is fetch.ErrNotFound; an unreachable
// API, a 401/403 (including a blocked host) or rate limiting is
// fetch.ErrUnavailable.
package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/fetch"
)

const (
	// DefaultBaseURL is the public GitHub API.
	DefaultBaseURL = "https://api.github.com"
	// APIVersion is sent as X-GitHub-Api-Version.
	APIVersion = "2022-11-28"
	// acceptHeader is the media type GitHub recommends for REST requests.
	acceptHeader = "application/vnd.github+json"
	// perPage is the page size requested from list endpoints (the maximum).
	perPage = 100
	// DefaultMaxPages bounds how many pages of one listing are followed.
	DefaultMaxPages = 100
)

// Client is a small GitHub REST client. It is safe for concurrent use.
type Client struct {
	fetch    fetch.Client
	base     string
	baseURL  *url.URL
	baseErr  error
	token    string
	ttl      time.Duration
	maxPages int
}

// Option customises a Client.
type Option func(*Client)

// WithBaseURL sets the API base URL, for GitHub Enterprise Server
// ("https://ghe.example.com/api/v3") or for tests. The default is
// DefaultBaseURL.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.base = strings.TrimRight(strings.TrimSpace(u), "/") }
}

// WithTTL sets how long cached API responses stay fresh; the default is
// fetch.DefaultTTL. Responses are never treated as immutable: releases are
// edited after publication and listings grow.
func WithTTL(d time.Duration) Option {
	return func(c *Client) { c.ttl = d }
}

// WithMaxPages bounds how many pages of a listing are followed (default
// DefaultMaxPages); exceeding it is an error rather than a silent truncation.
func WithMaxPages(n int) Option {
	return func(c *Client) { c.maxPages = n }
}

// NewClient returns a client reading through f. token may be empty; callers
// typically pass os.Getenv("GITHUB_TOKEN") or GH_TOKEN.
func NewClient(f fetch.Client, token string, opts ...Option) *Client {
	c := &Client{fetch: f, base: DefaultBaseURL, token: strings.TrimSpace(token), maxPages: DefaultMaxPages}
	for _, o := range opts {
		o(c)
	}
	if c.maxPages <= 0 {
		c.maxPages = DefaultMaxPages
	}
	c.baseURL, c.baseErr = url.Parse(c.base)
	if c.baseErr == nil && (c.baseURL.Host == "" || (c.baseURL.Scheme != "https" && c.baseURL.Scheme != "http")) {
		c.baseErr = fmt.Errorf("github: invalid API base URL %q", c.base)
	}
	return c
}

// get performs one authenticated GET.
func (c *Client) get(ctx context.Context, rawURL string) (*fetch.Document, error) {
	if c.baseErr != nil {
		return nil, c.baseErr
	}
	h := http.Header{}
	h.Set("Accept", acceptHeader)
	h.Set("X-GitHub-Api-Version", APIVersion)
	if c.token != "" && c.sameOrigin(rawURL) {
		h.Set("Authorization", "Bearer "+c.token)
	}
	return c.fetch.Do(ctx, fetch.Request{URL: rawURL, Header: h, TTL: c.ttl})
}

// sameOrigin reports whether rawURL points at the API base's scheme and host.
func (c *Client) sameOrigin(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && c.baseURL != nil && u.Scheme == c.baseURL.Scheme && u.Host == c.baseURL.Host
}

// endpoint builds an API URL below the base URL.
func (c *Client) endpoint(path string, query url.Values) string {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

// paginate fetches first and every following page named by the Link header,
// calling each with every page. It fails when the API points to another
// origin (the token must not follow it) or when more than the configured
// maximum number of pages would be read.
func (c *Client) paginate(ctx context.Context, first string, each func(*fetch.Document) error) error {
	next := first
	for page := 1; next != ""; page++ {
		if page > c.maxPages {
			return fmt.Errorf("github: %s has more than %d pages", first, c.maxPages)
		}
		doc, err := c.get(ctx, next)
		if err != nil {
			return err
		}
		if err := each(doc); err != nil {
			return err
		}
		link := doc.Header.Get("Link")
		if link == "" {
			return nil
		}
		target := nextLink(link)
		if target == "" {
			return nil
		}
		base, err := url.Parse(next)
		if err != nil {
			return err
		}
		ref, err := url.Parse(target)
		if err != nil {
			return fmt.Errorf("github: bad Link header %q: %w", link, err)
		}
		next = base.ResolveReference(ref).String()
		if !c.sameOrigin(next) {
			return fmt.Errorf("github: refusing to follow pagination link to %s (not %s)", next, c.base)
		}
	}
	return nil
}

var linkRe = regexp.MustCompile(`<([^>]*)>((?:\s*;\s*[^;,<]+)*)`)

// nextLink returns the target of the rel="next" entry of a Link header
// (RFC 8288), or "".
//
//	<https://api.github.com/repositories/1/releases?per_page=100&page=2>; rel="next", <...>; rel="last"
func nextLink(header string) string {
	for _, m := range linkRe.FindAllStringSubmatch(header, -1) {
		for _, param := range strings.Split(m[2], ";") {
			name, value, ok := strings.Cut(strings.TrimSpace(param), "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(name), "rel") {
				continue
			}
			for _, rel := range strings.Fields(strings.Trim(strings.TrimSpace(value), `"`)) {
				if strings.EqualFold(rel, "next") {
					return m[1]
				}
			}
		}
	}
	return ""
}

// parseRepository extracts owner and name from "owner/name". For leniency it
// also accepts "github.com/owner/name" and "https://github.com/owner/name",
// each with an optional ".git" suffix.
func parseRepository(s string) (owner, name string, err error) {
	orig := s
	s = strings.TrimSpace(s)
	for _, p := range []string{"https://github.com/", "http://github.com/", "github.com/"} {
		if rest, ok := strings.CutPrefix(s, p); ok {
			s = rest
			break
		}
	}
	s = strings.TrimSuffix(strings.Trim(s, "/"), ".git")
	owner, name, ok := strings.Cut(s, "/")
	if !ok || !validRepoPart(owner) || !validRepoPart(name) {
		return "", "", fmt.Errorf("github: invalid repository %q: want owner/name", orig)
	}
	return owner, name, nil
}

var repoPartRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validRepoPart reports whether s can be an owner or repository name.
func validRepoPart(s string) bool {
	return s != "." && s != ".." && repoPartRe.MatchString(s)
}

// escapePath escapes each segment of a slash-separated path, keeping the
// slashes (tags such as "cmd/ctl/v1.12.0" contain some).
func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}
