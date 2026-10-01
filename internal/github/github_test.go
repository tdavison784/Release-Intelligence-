package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// apiRequest is one request seen by the fake API.
type apiRequest struct {
	Path   string // path and query, e.g. /repos/o/r/releases?per_page=100
	Header http.Header
}

// fakeAPI serves recorded GitHub responses, with Link-header pagination.
type fakeAPI struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []apiRequest
	// handlers maps "path?query" to a response; unknown paths are 404.
	handlers map[string]func(w http.ResponseWriter, r *http.Request)
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	api := &fakeAPI{handlers: map[string]func(http.ResponseWriter, *http.Request){}}
	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		api.reqs = append(api.reqs, apiRequest{Path: r.URL.RequestURI(), Header: r.Header.Clone()})
		h := api.handlers[r.URL.RequestURI()]
		api.mu.Unlock()
		if h == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"Not Found","documentation_url":"https://docs.github.com/rest"}`)
			return
		}
		h(w, r)
	}))
	t.Cleanup(api.Close)
	return api
}

// serve registers a JSON response for a request URI.
func (a *fakeAPI) serve(uri, body string, link string) {
	a.handlers[uri] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if link != "" {
			w.Header().Set("Link", strings.ReplaceAll(link, "{base}", a.URL))
		}
		fmt.Fprint(w, body)
	}
}

func (a *fakeAPI) requests() []apiRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]apiRequest(nil), a.reqs...)
}

func (a *fakeAPI) paths() []string {
	var ps []string
	for _, r := range a.requests() {
		ps = append(ps, r.Path)
	}
	return ps
}

// spy records the requests made through a fetch.Client.
type spy struct {
	inner fetch.Client
	mu    sync.Mutex
	reqs  []fetch.Request
}

func (s *spy) Do(ctx context.Context, req fetch.Request) (*fetch.Document, error) {
	s.mu.Lock()
	s.reqs = append(s.reqs, req)
	s.mu.Unlock()
	return s.inner.Do(ctx, req)
}

func newClient(t *testing.T, api *fakeAPI, cacheDir, token string, mode fetch.Mode, opts ...Option) (*Client, *spy) {
	t.Helper()
	sp := &spy{inner: fetch.NewHTTPClient(fetch.NewCache(cacheDir), mode)}
	return NewClient(sp, token, append([]Option{WithBaseURL(api.URL)}, opts...)...), sp
}

const releasesURI = "/repos/cert-manager/cert-manager/releases?per_page=100"

// releasesAPI serves two pages of releases.
func releasesAPI(t *testing.T) *fakeAPI {
	api := newFakeAPI(t)
	api.serve(releasesURI, fixture(t, "releases_page1.json"),
		`<{base}/repos/cert-manager/cert-manager/releases?per_page=100&page=2>; rel="next", <{base}/repos/cert-manager/cert-manager/releases?per_page=100&page=2>; rel="last"`)
	api.serve(releasesURI+"&page=2", fixture(t, "releases_page2.json"),
		`<{base}/repos/cert-manager/cert-manager/releases?per_page=100&page=1>; rel="prev", <{base}/repos/cert-manager/cert-manager/releases?per_page=100&page=1>; rel="first"`)
	return api
}

func releasesLoc() catalog.Locator {
	return catalog.Locator{Kind: catalog.LocatorGitHubReleases, Repository: "cert-manager/cert-manager"}
}

func TestListReleases(t *testing.T) {
	api := releasesAPI(t)
	c, _ := newClient(t, api, t.TempDir(), "s3cret", fetch.ModeOnline)
	refs, err := NewReleases(c).ListReleases(context.Background(), releasesLoc())
	if err != nil {
		t.Fatal(err)
	}

	// Both pages were read, the draft and the duplicate were dropped.
	var tags []string
	for _, r := range refs {
		tags = append(tags, r.Tag)
	}
	want := []string{"v1.19.0-beta.0", "v1.18.1", "v1.18.0", "cmd/ctl/v1.17.0"}
	if strings.Join(tags, ",") != strings.Join(want, ",") {
		t.Fatalf("tags = %v, want %v", tags, want)
	}
	if got := api.paths(); len(got) != 2 || got[0] != releasesURI || got[1] != releasesURI+"&page=2" {
		t.Errorf("requests = %v", got)
	}

	beta, stable, plain, slash := refs[0], refs[1], refs[2], refs[3]
	if !beta.Prerelease || stable.Prerelease || plain.Prerelease {
		t.Errorf("prerelease flags: %v %v %v", beta.Prerelease, stable.Prerelease, plain.Prerelease)
	}
	if beta.Draft || stable.Draft {
		t.Error("drafts are skipped, never flagged")
	}
	if beta.PublishedAt == nil || !beta.PublishedAt.Equal(time.Date(2025, 9, 1, 9, 30, 0, 0, time.UTC)) {
		t.Errorf("PublishedAt = %v", beta.PublishedAt)
	}
	if beta.URL != "https://github.com/cert-manager/cert-manager/releases/tag/v1.19.0-beta.0" {
		t.Errorf("URL = %q", beta.URL)
	}
	// target_commitish is only a commit when it is a full SHA.
	if beta.Commit != "" || stable.Commit != "7f9d1a3c5b7e9f0a2c4e6a8b0d2f4a6c8e0b1d3f" {
		t.Errorf("commits: %q %q", beta.Commit, stable.Commit)
	}
	// published_at null: fall back to created_at; html_url missing: construct it.
	if slash.PublishedAt == nil || !slash.PublishedAt.Equal(time.Date(2025, 2, 4, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("fallback PublishedAt = %v", slash.PublishedAt)
	}
	if slash.URL != "https://github.com/cert-manager/cert-manager/releases/tag/cmd/ctl/v1.17.0" {
		t.Errorf("constructed URL = %q", slash.URL)
	}

	ev := stable.Evidence
	if ev.Kind != domain.EvidenceGitRef || ev.URI != stable.URL || ev.Locator != "refs/tags/v1.18.1" ||
		ev.Excerpt != "release v1.18.1 published 2025-06-10T10:15:30Z" || ev.ID == "" || ev.RetrievedAt.IsZero() {
		t.Errorf("evidence = %+v", ev)
	}
	if !strings.HasSuffix(beta.Evidence.Excerpt, "(prerelease)") {
		t.Errorf("prerelease evidence excerpt = %q", beta.Evidence.Excerpt)
	}
	if beta.Evidence.ID == stable.Evidence.ID {
		t.Error("evidence ids must differ per release")
	}
}

func TestRequestHeaders(t *testing.T) {
	api := releasesAPI(t)
	c, _ := newClient(t, api, t.TempDir(), "s3cret", fetch.ModeOnline)
	if _, err := NewReleases(c).ListReleases(context.Background(), releasesLoc()); err != nil {
		t.Fatal(err)
	}
	for _, r := range api.requests() {
		if got := r.Header.Get("Authorization"); got != "Bearer s3cret" {
			t.Errorf("%s: Authorization = %q", r.Path, got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("%s: Accept = %q", r.Path, got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
			t.Errorf("%s: X-GitHub-Api-Version = %q", r.Path, got)
		}
	}

	// Without a token no Authorization header is sent.
	api2 := releasesAPI(t)
	c2, _ := newClient(t, api2, t.TempDir(), "", fetch.ModeOnline)
	if _, err := NewReleases(c2).ListReleases(context.Background(), releasesLoc()); err != nil {
		t.Fatal(err)
	}
	for _, r := range api2.requests() {
		if _, ok := r.Header["Authorization"]; ok {
			t.Errorf("%s: unexpected Authorization header", r.Path)
		}
	}
}

func TestListReleasesCachingAndOfflineReplay(t *testing.T) {
	api := releasesAPI(t)
	dir := t.TempDir()
	c, sp := newClient(t, api, dir, "", fetch.ModeOnline)
	first, err := NewReleases(c).ListReleases(context.Background(), releasesLoc())
	if err != nil {
		t.Fatal(err)
	}
	// Nothing about a release listing is immutable.
	for _, r := range sp.reqs {
		if r.Immutable {
			t.Errorf("request %s must not be immutable", r.URL)
		}
	}
	// A second online run is answered by the cache (fresh within the TTL).
	if _, err := NewReleases(c).ListReleases(context.Background(), releasesLoc()); err != nil {
		t.Fatal(err)
	}
	if n := len(api.requests()); n != 2 {
		t.Errorf("server saw %d requests, want 2 (the second run is cached)", n)
	}
	api.Close()

	// Offline replay follows the persisted Link header through the cache.
	off, _ := newClient(t, api, dir, "", fetch.ModeOffline)
	replay, err := NewReleases(off).ListReleases(context.Background(), releasesLoc())
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != len(first) {
		t.Fatalf("replay has %d releases, first run %d", len(replay), len(first))
	}
	for i := range replay {
		if replay[i].Tag != first[i].Tag || replay[i].Evidence.ID != first[i].Evidence.ID {
			t.Errorf("replay[%d] differs: %+v vs %+v", i, replay[i], first[i])
		}
	}
	// A repository that was never fetched is not available offline.
	_, err = NewReleases(off).ListReleases(context.Background(), catalog.Locator{Repository: "other/repo"})
	if !errors.Is(err, fetch.ErrOffline) {
		t.Errorf("want ErrOffline, got %v", err)
	}
}

func TestListReleasesCustomTTL(t *testing.T) {
	api := releasesAPI(t)
	c, sp := newClient(t, api, t.TempDir(), "", fetch.ModeOnline, WithTTL(time.Minute))
	if _, err := NewReleases(c).ListReleases(context.Background(), releasesLoc()); err != nil {
		t.Fatal(err)
	}
	if sp.reqs[0].TTL != time.Minute {
		t.Errorf("TTL = %v", sp.reqs[0].TTL)
	}
}

func TestPaginationSafety(t *testing.T) {
	ctx := context.Background()

	t.Run("a link to another host is refused and the token is not sent there", func(t *testing.T) {
		evil := newFakeAPI(t)
		evil.serve("/steal", "[]", "")
		api := newFakeAPI(t)
		api.serve(releasesURI, "[]", `<`+evil.URL+`/steal>; rel="next"`)
		c, _ := newClient(t, api, t.TempDir(), "s3cret", fetch.ModeOnline)
		_, err := NewReleases(c).ListReleases(ctx, releasesLoc())
		if err == nil || !strings.Contains(err.Error(), "refusing to follow") {
			t.Fatalf("got %v", err)
		}
		if n := len(evil.requests()); n != 0 {
			t.Errorf("foreign host was contacted %d times", n)
		}
	})

	t.Run("page limit is an error, not a silent truncation", func(t *testing.T) {
		api := newFakeAPI(t)
		api.serve(releasesURI, "[]", `<{base}`+releasesURI+`&page=2>; rel="next"`)
		api.serve(releasesURI+"&page=2", "[]", `<{base}`+releasesURI+`&page=3>; rel="next"`)
		api.serve(releasesURI+"&page=3", "[]", "")
		c, _ := newClient(t, api, t.TempDir(), "", fetch.ModeOnline, WithMaxPages(2))
		_, err := NewReleases(c).ListReleases(ctx, releasesLoc())
		if err == nil || !strings.Contains(err.Error(), "more than 2 pages") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("relative links are resolved", func(t *testing.T) {
		api := newFakeAPI(t)
		api.serve(releasesURI, fixture(t, "releases_page1.json"), `</repos/cert-manager/cert-manager/releases?per_page=100&page=2>; rel="next"`)
		api.serve(releasesURI+"&page=2", fixture(t, "releases_page2.json"), "")
		c, _ := newClient(t, api, t.TempDir(), "", fetch.ModeOnline)
		refs, err := NewReleases(c).ListReleases(ctx, releasesLoc())
		if err != nil || len(refs) != 4 {
			t.Fatalf("%d releases, %v", len(refs), err)
		}
	})
}

func TestNextLink(t *testing.T) {
	tests := []struct {
		name, header, want string
	}{
		{"empty", "", ""},
		{"github style", `<https://api.github.com/repositories/1/releases?per_page=100&page=2>; rel="next", <https://api.github.com/repositories/1/releases?per_page=100&page=9>; rel="last"`,
			"https://api.github.com/repositories/1/releases?per_page=100&page=2"},
		{"next is not first", `<https://x/?page=1>; rel="prev", <https://x/?page=3>; rel="next", <https://x/?page=9>; rel="last"`, "https://x/?page=3"},
		{"last page", `<https://x/?page=1>; rel="prev", <https://x/?page=1>; rel="first"`, ""},
		{"unquoted rel", `<https://x/?page=2>; rel=next`, "https://x/?page=2"},
		{"extra params", `<https://x/?page=2>; title="Second"; rel="next"`, "https://x/?page=2"},
		{"several rel values", `<https://x/?page=2>; rel="next last"`, "https://x/?page=2"},
		{"upper case", `<https://x/?page=2>; REL="NEXT"`, "https://x/?page=2"},
		{"commas inside the target", `<https://x/?q=a,b&page=2>; rel="next"`, "https://x/?q=a,b&page=2"},
		{"garbage", `not a link header`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextLink(tc.header); got != tc.want {
				t.Errorf("nextLink(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestParseRepository(t *testing.T) {
	tests := []struct {
		in, owner, name string
		wantErr         bool
	}{
		{in: "cert-manager/cert-manager", owner: "cert-manager", name: "cert-manager"},
		{in: "github.com/istio/istio", owner: "istio", name: "istio"},
		{in: "https://github.com/argoproj/argo-cd", owner: "argoproj", name: "argo-cd"},
		{in: "https://github.com/argoproj/argo-cd.git", owner: "argoproj", name: "argo-cd"},
		{in: "argoproj/argo-cd/", owner: "argoproj", name: "argo-cd"},
		{in: " o/r ", owner: "o", name: "r"},
		{in: "o/.github", owner: "o", name: ".github"},
		{in: "", wantErr: true},
		{in: "justone", wantErr: true},
		{in: "a/b/c", wantErr: true},
		{in: "a/..", wantErr: true},
		{in: "../b", wantErr: true},
		{in: "a b/c", wantErr: true},
		{in: "o/r?x=1", wantErr: true},
	}
	for _, tc := range tests {
		owner, name, err := parseRepository(tc.in)
		if (err != nil) != tc.wantErr || owner != tc.owner || name != tc.name {
			t.Errorf("parseRepository(%q) = %q, %q, %v", tc.in, owner, name, err)
		}
	}
}

func TestListReleasesErrors(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		status    int
		body      string
		sentinel  error
		wantState domain.SourceState
	}{
		{"not found", http.StatusNotFound, `{"message":"Not Found"}`, fetch.ErrNotFound, domain.SourceNotFound},
		{"forbidden (blocked or no access)", http.StatusForbidden, `{"message":"Resource not accessible"}`, fetch.ErrUnavailable, domain.SourceUnavailable},
		{"rate limited", http.StatusTooManyRequests, `{"message":"API rate limit exceeded"}`, fetch.ErrUnavailable, domain.SourceUnavailable},
		{"bad credentials", http.StatusUnauthorized, `{"message":"Bad credentials"}`, fetch.ErrUnavailable, domain.SourceUnavailable},
		{"server error", http.StatusInternalServerError, `oops`, nil, domain.SourceError},
		{"malformed json", http.StatusOK, `{not json`, nil, domain.SourceError},
		{"unexpected json shape", http.StatusOK, `{"message":"not a list"}`, nil, domain.SourceError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := newFakeAPI(t)
			api.handlers[releasesURI] = func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}
			c, _ := newClient(t, api, t.TempDir(), "", fetch.ModeOnline)
			_, err := NewReleases(c).ListReleases(ctx, releasesLoc())
			if err == nil {
				t.Fatal("want error")
			}
			if tc.sentinel != nil && !errors.Is(err, tc.sentinel) {
				t.Errorf("errors.Is(%v, %v) = false", err, tc.sentinel)
			}
			if got := fetch.StateFor(err); got != tc.wantState {
				t.Errorf("state = %q, want %q (%v)", got, tc.wantState, err)
			}
		})
	}

	t.Run("unreachable API is unavailable", func(t *testing.T) {
		api := newFakeAPI(t)
		c, _ := newClient(t, api, t.TempDir(), "", fetch.ModeOnline)
		api.Close()
		_, err := NewReleases(c).ListReleases(ctx, releasesLoc())
		if !errors.Is(err, fetch.ErrUnavailable) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("invalid base URL", func(t *testing.T) {
		c := NewClient(fetch.NewHTTPClient(nil, fetch.ModeOnline), "", WithBaseURL("not a url"))
		if _, err := NewReleases(c).ListReleases(ctx, releasesLoc()); err == nil {
			t.Error("want error")
		}
	})
	t.Run("invalid repository", func(t *testing.T) {
		api := newFakeAPI(t)
		c, _ := newClient(t, api, t.TempDir(), "", fetch.ModeOnline)
		if _, err := NewReleases(c).ListReleases(ctx, catalog.Locator{Repository: "nope"}); err == nil {
			t.Error("want error")
		}
		if n := len(api.requests()); n != 0 {
			t.Errorf("no request must be made, got %d", n)
		}
	})
}

func TestFetchReleaseDocument(t *testing.T) {
	api := newFakeAPI(t)
	api.serve("/repos/cert-manager/cert-manager/releases/tags/v1.18.0", fixture(t, "release_by_tag.json"), "")
	api.serve("/repos/cert-manager/cert-manager/releases/tags/cmd/ctl/v1.17.0", `{"tag_name":"cmd/ctl/v1.17.0","body":"ctl notes","draft":false}`, "")
	api.serve("/repos/cert-manager/cert-manager/releases/tags/v0.0.1-draft", `{"tag_name":"v0.0.1-draft","body":"x","draft":true}`, "")
	api.serve("/repos/cert-manager/cert-manager/releases/tags/v0.0.2", `{"tag_name":"v0.0.2","body":null,"draft":false}`, "")
	api.serve("/repos/cert-manager/cert-manager/releases/tags/v0.0.3", `[]`, "")
	c, sp := newClient(t, api, t.TempDir(), "s3cret", fetch.ModeOnline)
	rel := NewReleases(c)
	ctx := context.Background()
	loc := func(ref string) catalog.Locator {
		return catalog.Locator{Kind: catalog.LocatorGitHubReleases, Repository: "cert-manager/cert-manager", Ref: ref}
	}

	doc, err := rel.FetchDocument(ctx, loc("v1.18.0"))
	if err != nil {
		t.Fatal(err)
	}
	wantBody := "## Release notes\r\n\r\ncert-manager 1.18 is a feature release.\r\n\r\n### Breaking Changes\r\n\r\n- The `foo` option was removed (#1234)\r\n"
	if string(doc.Content) != wantBody {
		t.Errorf("body must be verbatim, got %q", doc.Content)
	}
	if doc.Format != "markdown" || doc.URI != "https://github.com/cert-manager/cert-manager/releases/tag/v1.18.0" ||
		doc.FetchURL != api.URL+"/repos/cert-manager/cert-manager/releases/tags/v1.18.0" ||
		doc.Digest != domain.Digest([]byte(wantBody)) || doc.RetrievedAt.IsZero() || doc.Locator != loc("v1.18.0") {
		t.Errorf("doc = %+v", doc)
	}
	// Release bodies are edited after publication: never cached as immutable.
	if len(sp.reqs) != 1 || sp.reqs[0].Immutable {
		t.Errorf("requests = %+v", sp.reqs)
	}
	if got := api.requests()[0].Header.Get("Authorization"); got != "Bearer s3cret" {
		t.Errorf("Authorization = %q", got)
	}

	// Tags with a slash keep it in the path.
	doc, err = rel.FetchDocument(ctx, loc("cmd/ctl/v1.17.0"))
	if err != nil || string(doc.Content) != "ctl notes" {
		t.Fatalf("slash tag: %v %+v", err, doc)
	}
	// Without html_url the release page is derived.
	if doc.URI != "https://github.com/cert-manager/cert-manager/releases/tag/cmd/ctl/v1.17.0" {
		t.Errorf("URI = %q", doc.URI)
	}
	// A release without a body is an empty document.
	doc, err = rel.FetchDocument(ctx, loc("v0.0.2"))
	if err != nil || len(doc.Content) != 0 {
		t.Fatalf("empty body: %v %+v", err, doc)
	}
	// Drafts do not exist for readers.
	if _, err := rel.FetchDocument(ctx, loc("v0.0.1-draft")); !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("draft: %v", err)
	}
	if _, err := rel.FetchDocument(ctx, loc("v9.9.9")); !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("missing tag: %v", err)
	}
	if _, err := rel.FetchDocument(ctx, loc("v0.0.3")); err == nil || errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("malformed payload must be a plain error: %v", err)
	}
	if _, err := rel.FetchDocument(ctx, loc("")); err == nil {
		t.Error("missing ref: want error")
	}
}

func TestFetchReleaseDocumentOfflineReplay(t *testing.T) {
	api := newFakeAPI(t)
	api.serve("/repos/cert-manager/cert-manager/releases/tags/v1.18.0", fixture(t, "release_by_tag.json"), "")
	dir := t.TempDir()
	c, _ := newClient(t, api, dir, "", fetch.ModeOnline)
	loc := catalog.Locator{Repository: "cert-manager/cert-manager", Ref: "v1.18.0"}
	first, err := NewReleases(c).FetchDocument(context.Background(), loc)
	if err != nil {
		t.Fatal(err)
	}
	api.Close()
	off, _ := newClient(t, api, dir, "", fetch.ModeOffline)
	again, err := NewReleases(off).FetchDocument(context.Background(), loc)
	if err != nil || again.Digest != first.Digest || string(again.Content) != string(first.Content) {
		t.Fatalf("offline: %v %+v", err, again)
	}
}

func TestRegister(t *testing.T) {
	reg := sources.NewRegistry()
	Register(reg, fetch.NewHTTPClient(nil, fetch.ModeOffline), "tok", WithBaseURL("https://ghe.example.com/api/v3"))
	if _, err := reg.VersionLister(catalog.LocatorGitHubReleases); err != nil {
		t.Error(err)
	}
	if _, err := reg.DocumentFetcher(catalog.LocatorGitHubReleases); err != nil {
		t.Error(err)
	}
	if _, err := reg.AdvisorySource(catalog.LocatorGitHubAdvisories); err != nil {
		t.Error(err)
	}
	if _, err := reg.VersionLister(catalog.LocatorGitTags); err == nil {
		t.Error("github adapters must not claim git-tags")
	}
}

func TestEnterpriseBaseURLAndTokenScope(t *testing.T) {
	// The token is sent to the configured base only; endpoint() keeps the
	// base path of GitHub Enterprise Server installations.
	c := NewClient(fetch.NewHTTPClient(nil, fetch.ModeOffline), "tok", WithBaseURL("https://ghe.example.com/api/v3/"))
	got := c.endpoint("/repos/o/r/releases", url.Values{"per_page": {"100"}})
	if got != "https://ghe.example.com/api/v3/repos/o/r/releases?per_page=100" {
		t.Errorf("endpoint = %q", got)
	}
	if !c.sameOrigin("https://ghe.example.com/other") || c.sameOrigin("https://evil.example.com/x") || c.sameOrigin("http://ghe.example.com/x") {
		t.Error("sameOrigin")
	}
	def := NewClient(fetch.NewHTTPClient(nil, fetch.ModeOffline), "")
	if def.endpoint("/x", nil) != "https://api.github.com/x" {
		t.Errorf("default endpoint = %q", def.endpoint("/x", nil))
	}
}
