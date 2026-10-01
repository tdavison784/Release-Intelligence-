package helm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// indexServer serves the trimmed real indexes of testdata/ at /istio and
// /argo and counts requests.
type indexServer struct {
	*httptest.Server
	hits atomic.Int32
}

func newIndexServer(t *testing.T) *indexServer {
	t.Helper()
	s := &indexServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		files := map[string]string{
			"/istio/index.yaml": "testdata/istio-index.yaml",
			"/argo/index.yaml":  "testdata/argo-index.yaml",
		}
		f, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile(filepath.FromSlash(f))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/x-yaml")
		w.Write(b)
	}))
	t.Cleanup(s.Close)
	return s
}

func newRepoAdapter(t *testing.T, opts ...Option) *RepoAdapter {
	t.Helper()
	return NewRepoAdapter(fetch.NewHTTPClient(nil, fetch.ModeOnline), opts...)
}

func repoLoc(url, chart string) catalog.Locator {
	return catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: url, Chart: chart}
}

func TestRepoListArtifactVersionsIstio(t *testing.T) {
	srv := newIndexServer(t)
	a := newRepoAdapter(t)
	vs, err := a.ListArtifactVersions(context.Background(), repoLoc(srv.URL+"/istio", "istiod"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range vs {
		got = append(got, v.Version+"="+v.Fields["appVersion"])
	}
	want := []string{
		"1.31.0-rc.0=1.31.0-rc.0", "1.26.2=1.26.2", "1.26.1=1.26.1",
		"1.26.0=1.26.0", "1.26.0-rc.0=1.26.0-rc.0", "1.25.5=1.25.5",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("versions = %v\nwant       %v", got, want)
	}

	var v126 sources.ArtifactVersion
	for _, v := range vs {
		if v.Version == "1.26.2" {
			v126 = v
		}
	}
	if v126.Fields["created"] == "" || !strings.HasPrefix(v126.Fields["created"], "20") {
		t.Errorf("created = %q", v126.Fields["created"])
	}
	if _, ok := v126.Fields["kubeVersion"]; ok {
		t.Error("kubeVersion must be omitted when the entry has none")
	}
	if !strings.HasPrefix(v126.Digest, "sha256:") || len(v126.Digest) != len("sha256:")+64 {
		t.Errorf("Digest = %q", v126.Digest)
	}
	if v126.URI != "https://istio-release.storage.googleapis.com/charts/istiod-1.26.2.tgz" {
		t.Errorf("URI = %q", v126.URI)
	}
	ev := v126.Evidence
	if ev.Kind != domain.EvidenceRegistry || ev.URI != srv.URL+"/istio/index.yaml" ||
		ev.Locator != "entries.istiod[version=1.26.2]" || ev.Excerpt != "version: 1.26.2, appVersion: 1.26.2" ||
		!strings.HasPrefix(ev.ContentDigest, "sha256:") || ev.ID == "" || ev.RetrievedAt.IsZero() {
		t.Errorf("evidence = %+v", ev)
	}

	// Another chart of the same index: kubeVersion is carried when present.
	amb, err := a.ListArtifactVersions(context.Background(), repoLoc(srv.URL+"/istio", "ambient"))
	if err != nil || len(amb) != 1 || amb[0].Fields["kubeVersion"] != ">= 1.23.0-0" {
		t.Fatalf("ambient = %+v, %v", amb, err)
	}
}

func TestRepoArgoAppVersionLookup(t *testing.T) {
	srv := newIndexServer(t)
	a := newRepoAdapter(t)
	// The "lookup" version relation: chart versions whose appVersion equals the release tag.
	vs, err := a.ListArtifactVersions(context.Background(), repoLoc(srv.URL+"/argo", "argo-cd"))
	if err != nil {
		t.Fatal(err)
	}
	var match []string
	for _, v := range vs {
		if v.Fields["appVersion"] == "v3.0.0" {
			match = append(match, v.Version)
		}
	}
	if !reflect.DeepEqual(match, []string{"8.0.1", "8.0.0"}) {
		t.Fatalf("charts with appVersion v3.0.0 = %v", match)
	}
	// Numeric-looking appVersion stays a string; a missing kubeVersion is absent.
	last := vs[len(vs)-1]
	if last.Version != "0.1.0" || last.Fields["appVersion"] != "0.11" {
		t.Errorf("oldest entry = %+v", last)
	}
	if _, ok := last.Fields["kubeVersion"]; ok {
		t.Errorf("kubeVersion present on %+v", last)
	}
	if got := vs[0].Fields["kubeVersion"]; got != ">=1.25.0-0" {
		t.Errorf("kubeVersion = %q", got)
	}
}

func TestRepoProbe(t *testing.T) {
	srv := newIndexServer(t)
	a := newRepoAdapter(t)
	ctx := context.Background()
	istio := srv.URL + "/istio/" // trailing slash must not matter
	tests := []struct {
		name       string
		loc        catalog.Locator
		version    string
		wantExists bool
		wantCoord  string
		wantExcerp string
	}{
		{"exact", repoLoc(istio, "istiod"), "1.26.0", true, srv.URL + "/istio istiod@1.26.0", "version: 1.26.0, appVersion: 1.26.0"},
		{"prerelease", repoLoc(istio, "istiod"), "1.26.0-rc.0", true, srv.URL + "/istio istiod@1.26.0-rc.0", ""},
		{"semantically equal", repoLoc(istio, "istiod"), "v1.26.0", true, srv.URL + "/istio istiod@1.26.0", ""},
		{"missing version", repoLoc(istio, "istiod"), "1.26.9", false, srv.URL + "/istio istiod@1.26.9", "no entry with version 1.26.9"},
		{"missing chart", repoLoc(istio, "nope"), "1.26.0", false, srv.URL + "/istio nope@1.26.0", "chart nope is not in the index"},
		{"other chart", repoLoc(srv.URL+"/argo", "argo-cd"), "8.0.1", true, srv.URL + "/argo argo-cd@8.0.1", "version: 8.0.1, appVersion: v3.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := a.Probe(ctx, tt.loc, tt.version)
			if err != nil {
				t.Fatal(err)
			}
			if res.Exists != tt.wantExists || res.Coordinate != tt.wantCoord {
				t.Fatalf("got exists=%v coordinate=%q; want %v %q", res.Exists, res.Coordinate, tt.wantExists, tt.wantCoord)
			}
			if tt.wantExcerp != "" && res.Evidence.Excerpt != tt.wantExcerp {
				t.Errorf("excerpt = %q, want %q", res.Evidence.Excerpt, tt.wantExcerp)
			}
			if res.Evidence.Kind != domain.EvidenceRegistry || res.Evidence.ID == "" || res.Evidence.ContentDigest == "" {
				t.Errorf("evidence = %+v", res.Evidence)
			}
			if tt.wantExists {
				if res.Digest == "" || !strings.HasSuffix(res.URI, ".tgz") {
					t.Errorf("digest/uri = %q / %q", res.Digest, res.URI)
				}
			} else if res.URI != srv.URL+"/istio/index.yaml" {
				t.Errorf("URI of a negative result = %q", res.URI)
			}
		})
	}
	if _, err := a.Probe(ctx, repoLoc(istio, "istiod"), ""); err == nil {
		t.Error("expected an error for an empty version")
	}
	if _, err := a.Probe(ctx, repoLoc(istio, ""), "1.0.0"); err == nil {
		t.Error("expected an error for a locator without chart")
	}
}

func TestRepoListReleases(t *testing.T) {
	srv := newIndexServer(t)
	a := newRepoAdapter(t)
	refs, err := a.ListReleases(context.Background(), repoLoc(srv.URL+"/argo", "argo-cd"))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 6 || refs[0].Tag != "10.9.5" {
		t.Fatalf("refs = %+v", refs)
	}
	want := time.Date(2026, 9, 30, 15, 34, 43, 176766731, time.UTC)
	if refs[0].PublishedAt == nil || !refs[0].PublishedAt.Equal(want) {
		t.Errorf("PublishedAt = %v, want %v", refs[0].PublishedAt, want)
	}
	if refs[0].URL != "https://github.com/argoproj/argo-helm/releases/download/argo-cd-10.9.5/argo-cd-10.9.5.tgz" {
		t.Errorf("URL = %q", refs[0].URL)
	}
	if refs[0].Prerelease || refs[0].Draft || refs[0].Evidence.Locator != "entries.argo-cd[version=10.9.5]" {
		t.Errorf("ref = %+v", refs[0])
	}
	if _, err := a.ListReleases(context.Background(), repoLoc(srv.URL+"/argo", "no-such-chart")); !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("unknown chart: err = %v, want ErrNotFound", err)
	}
}

func TestRepoIndexParsedOncePerURL(t *testing.T) {
	srv := newIndexServer(t)
	a := newRepoAdapter(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	idxs := make([]*Index, 16)
	for i := range idxs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			if idxs[i], err = a.Index(ctx, srv.URL+"/argo"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, ix := range idxs {
		if ix != idxs[0] {
			t.Fatal("index was parsed more than once")
		}
	}
	// Lookups for several charts and versions reuse it too.
	for _, chart := range []string{"argo-cd", "argo-workflows"} {
		if _, err := a.ListArtifactVersions(ctx, repoLoc(srv.URL+"/argo", chart)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Probe(ctx, repoLoc(srv.URL+"/argo", "argo-cd"), "10.9.4"); err != nil {
		t.Fatal(err)
	}
	if n := srv.hits.Load(); n != 1 {
		t.Errorf("index fetched %d times, want 1", n)
	}
	// A different repository URL is a different index.
	if _, err := a.Index(ctx, srv.URL+"/istio"); err != nil {
		t.Fatal(err)
	}
	if n := srv.hits.Load(); n != 2 {
		t.Errorf("after second repository: %d fetches, want 2", n)
	}
}

func TestRepoIndexUsesFetchCacheAndTTL(t *testing.T) {
	srv := newIndexServer(t)
	cache := fetch.NewCache(t.TempDir())
	ctx := context.Background()
	loc := repoLoc(srv.URL+"/istio", "istiod")

	if _, err := NewRepoAdapter(fetch.NewHTTPClient(cache, fetch.ModeOnline)).ListArtifactVersions(ctx, loc); err != nil {
		t.Fatal(err)
	}
	// A new process (new adapter) replays from the cache, also offline.
	offline := NewRepoAdapter(fetch.NewHTTPClient(cache, fetch.ModeOffline))
	vs, err := offline.ListArtifactVersions(ctx, loc)
	if err != nil || len(vs) != 6 {
		t.Fatalf("offline replay: %d versions, %v", len(vs), err)
	}
	if n := srv.hits.Load(); n != 1 {
		t.Errorf("network hits = %d, want 1", n)
	}

	// The configured TTL is passed to the fetch client (the index is mutable).
	var got fetch.Request
	rec := &stubClient{do: func(req fetch.Request) (*fetch.Document, error) {
		got = req
		return nil, &fetch.Error{URL: req.URL, Err: fetch.ErrUnavailable}
	}}
	_, _ = NewRepoAdapter(rec, WithIndexTTL(15*time.Minute)).Index(ctx, "https://charts.example.com/stable/")
	if got.TTL != 15*time.Minute || got.Immutable || got.URL != "https://charts.example.com/stable/index.yaml" {
		t.Errorf("request = %+v", got)
	}
	_, _ = NewRepoAdapter(rec).Index(ctx, "https://charts.example.com/stable")
	if got.TTL != DefaultIndexTTL {
		t.Errorf("default TTL = %v", got.TTL)
	}
}

func TestRepoUnavailable(t *testing.T) {
	ctx := context.Background()
	t.Run("connection refused", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		a := newRepoAdapter(t)
		if _, err := a.Probe(ctx, repoLoc(url, "x"), "1.0.0"); !errors.Is(err, fetch.ErrUnavailable) {
			t.Errorf("Probe err = %v", err)
		}
		if _, err := a.ListArtifactVersions(ctx, repoLoc(url, "x")); !errors.Is(err, fetch.ErrUnavailable) {
			t.Errorf("ListArtifactVersions err = %v", err)
		}
		if _, err := a.ListReleases(ctx, repoLoc(url, "x")); !errors.Is(err, fetch.ErrUnavailable) {
			t.Errorf("ListReleases err = %v", err)
		}
	})
	t.Run("blocked (403)", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "blocked", http.StatusForbidden)
		}))
		defer srv.Close()
		if _, err := newRepoAdapter(t).Probe(ctx, repoLoc(srv.URL, "x"), "1.0.0"); !errors.Is(err, fetch.ErrUnavailable) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("missing index is not found", func(t *testing.T) {
		srv := newIndexServer(t)
		_, err := newRepoAdapter(t).Probe(ctx, repoLoc(srv.URL+"/nowhere", "x"), "1.0.0")
		if !errors.Is(err, fetch.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("failures are not remembered", func(t *testing.T) {
		var fail atomic.Bool
		fail.Store(true)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if fail.Load() {
				http.Error(w, "down", http.StatusForbidden)
				return
			}
			w.Write([]byte("apiVersion: v1\nentries:\n  c:\n  - name: c\n    version: 1.0.0\n"))
		}))
		defer srv.Close()
		a := newRepoAdapter(t)
		if _, err := a.Index(ctx, srv.URL); err == nil {
			t.Fatal("expected failure")
		}
		fail.Store(false)
		if _, err := a.Index(ctx, srv.URL); err != nil {
			t.Fatalf("retry after failure: %v", err)
		}
	})
	t.Run("not an index", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("<html><body>hello</body></html>"))
		}))
		defer srv.Close()
		if _, err := newRepoAdapter(t).Index(ctx, srv.URL); err == nil {
			t.Fatal("expected a decode error")
		}
	})
}

// stubClient is a fetch.Client backed by a function.
type stubClient struct {
	do func(fetch.Request) (*fetch.Document, error)
}

func (c *stubClient) Do(_ context.Context, req fetch.Request) (*fetch.Document, error) {
	return c.do(req)
}

func TestParseIndexQuirks(t *testing.T) {
	const body = `apiVersion: v1
generated: 2026-01-02T03:04:05Z
entries:
  demo:
  - name: demo
    version: 1.10
    appVersion: 2.0
    created: 2025-05-12T18:30:21.410929661Z
    digest: ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789
    urls:
    - charts/demo-1.10.tgz
    - https://mirror.example.com/demo-1.10.tgz
    deprecated: true
  - name: demo
    version: 1.9.0
    digest: sha256:deadbeef
    urls: []
`
	idx, err := ParseIndex([]byte(body), "https://charts.example.com/repo/index.yaml", "sha256:abc", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if idx.BaseURL != "https://charts.example.com/repo/" || idx.Generated != "2026-01-02T03:04:05Z" {
		t.Errorf("idx = %+v", idx)
	}
	e := idx.Entries["demo"][0]
	if e.Version != "1.10" || e.AppVersion != "2.0" || !e.Deprecated {
		t.Errorf("scalars must stay verbatim strings: %+v", e)
	}
	if e.Created != "2025-05-12T18:30:21.410929661Z" {
		t.Errorf("unquoted timestamp must decode as its text, got %q", e.Created)
	}
	if got := idx.ChartURL(e); got != "https://charts.example.com/repo/charts/demo-1.10.tgz" {
		t.Errorf("relative URL resolved to %q", got)
	}
	if got := idx.ChartURL(idx.Entries["demo"][1]); got != "" {
		t.Errorf("entry without urls: %q", got)
	}
	if got := normalizeDigest(e.Digest); got != "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" {
		t.Errorf("normalizeDigest = %q", got)
	}
	if got := normalizeDigest("sha256:deadbeef"); got != "sha256:deadbeef" {
		t.Errorf("already prefixed digest changed: %q", got)
	}
	if _, err := ParseIndex([]byte("apiVersion: v1\n"), "https://x/index.yaml", "", time.Time{}); err == nil {
		t.Error("an index without entries must be rejected")
	}
	if got := resolveURL("https://x/repo/", "https://other.example.com/a.tgz"); got != "https://other.example.com/a.tgz" {
		t.Errorf("absolute URL changed: %q", got)
	}
}

func TestParseCreated(t *testing.T) {
	for in, ok := range map[string]bool{
		"2025-05-12T18:30:21.410929661Z": true,
		"2025-05-12T18:30:21Z":           true,
		"2025-05-12T18:30:21+02:00":      true,
		"":                               false,
		"yesterday":                      false,
	} {
		if got := parseCreated(in); (got != nil) != ok {
			t.Errorf("parseCreated(%q) = %v", in, got)
		}
	}
}

func TestSameVersion(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"1.2.3", "1.2.3", true},
		{"v1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", false},
		{"1.2.3-rc.1", "1.2.3", false},
		{"1.2.3+a", "1.2.3+b", false},
		{"not-semver", "1.2.3", false},
		{"not-semver", "not-semver", true},
	} {
		if got := sameVersion(tt.a, tt.b); got != tt.want {
			t.Errorf("sameVersion(%q, %q) = %v", tt.a, tt.b, got)
		}
	}
}
