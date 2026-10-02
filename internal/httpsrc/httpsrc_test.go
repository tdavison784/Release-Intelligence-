package httpsrc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

func TestIsImmutableURL(t *testing.T) {
	sha := strings.Repeat("ab", 20)
	tests := []struct {
		url  string
		want bool
	}{
		// release assets
		{"https://github.com/cert-manager/cert-manager/releases/download/v1.18.0/cert-manager.yaml", true},
		{"https://github.com/argoproj/argo-helm/releases/download/argo-cd-8.0.0/argo-cd-8.0.0.tgz", true},
		{"https://github.com/o/r/releases/download/nightly/asset.zip", true},
		// versioned directory segments
		{"https://raw.githubusercontent.com/argoproj/argo-cd/v3.0.0/manifests/install.yaml", true},
		{"https://raw.githubusercontent.com/istio/istio/1.26.0/manifests/charts/base/Chart.yaml", true},
		{"https://istio-release.storage.googleapis.com/charts/../releases/1.20.0/index.yaml", true},
		{"https://raw.githubusercontent.com/o/r/" + sha + "/data.json", true},
		{"https://example.com/argo-cd-8.0.0/values.yaml", true},
		// mutable
		{"https://raw.githubusercontent.com/cert-manager/website/master/content/docs/variables.json", false},
		{"https://raw.githubusercontent.com/argoproj/argo-helm/gh-pages/index.yaml", false},
		{"https://raw.githubusercontent.com/istio/istio.io/release-1.31/data/x.yml", false},
		{"https://github.com/o/r/releases/latest/download/x.yaml", false},
		{"https://github.com/o/r/releases/latest", false},
		// the file name alone does not make a URL immutable
		{"https://example.com/charts/argo-cd-8.0.0.tgz", false},
		{"https://example.com/v1.2.3", false},
		{"https://example.com/", false},
		{"https://example.com", false},
		{"https://example.com/index.yaml?ref=v1.2.3", false},
		{"%%%", false},
	}
	for _, tc := range tests {
		if got := IsImmutableURL(tc.url); got != tc.want {
			t.Errorf("IsImmutableURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestFormatFor(t *testing.T) {
	tests := []struct {
		path, ct, want string
	}{
		{"/a/install.yaml", "text/plain", "yaml"},
		{"/a/index.yml", "", "yaml"},
		{"/a/notes.md", "text/plain; charset=utf-8", "markdown"},
		{"/a/variables.json", "text/plain", "json"},
		{"/a/page.html", "", "html"},
		{"/a/noext", "application/json; charset=utf-8", "json"},
		{"/a/noext", "application/vnd.github+json", "json"},
		{"/a/noext", "application/x-yaml", "yaml"},
		{"/a/noext", "text/yaml", "yaml"},
		{"/a/noext", "text/markdown", "markdown"},
		{"/a/noext", "text/html; charset=utf-8", "html"},
		{"/a/noext", "text/plain", "text"},
		{"/a/noext", "application/octet-stream", "text"},
		{"/a/noext", "garbage;;;", "text"},
		{"/a/noext", "", "text"},
	}
	for _, tc := range tests {
		if got := formatFor(tc.path, tc.ct); got != tc.want {
			t.Errorf("formatFor(%q, %q) = %q, want %q", tc.path, tc.ct, got, tc.want)
		}
	}
}

// spy records the requests passing through a fetch.Client.
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

func (s *spy) methods() string {
	var ms []string
	for _, r := range s.reqs {
		m := r.Method
		if m == "" {
			m = "GET"
		}
		ms = append(ms, m)
	}
	return strings.Join(ms, ",")
}

// testServer serves a small release-asset layout.
type testServer struct {
	*httptest.Server
	mu   sync.Mutex
	hits map[string]int // "METHOD path" -> count
}

func newTestServer(t *testing.T) *testServer {
	ts := &testServer{hits: map[string]int{}}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		ts.hits[r.Method+" "+r.URL.Path]++
		ts.mu.Unlock()
		switch r.URL.Path {
		case "/releases/download/v1.18.0/cert-manager.yaml":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("kind: Deployment\n"))
		case "/no-head/asset.tgz": // rejects HEAD
			if r.Method == http.MethodHead {
				w.Header().Set("Allow", "GET")
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Write([]byte("tarball bytes"))
		case "/signed/asset.tgz": // HEAD forbidden, GET fine (method-specific signature)
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Write([]byte("signed bytes"))
		case "/head-501/asset":
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusNotImplemented)
				return
			}
			w.Write([]byte("x"))
		case "/forbidden/asset":
			w.WriteHeader(http.StatusForbidden)
		case "/limited/asset":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/gone/asset":
			w.WriteHeader(http.StatusGone)
		case "/docs/table.md":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("| a | b |\n"))
		case "/docs/data":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"k":"v"}`))
		case "/boom":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

func (ts *testServer) count(key string) int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.hits[key]
}

func newAdapter(t *testing.T, cacheDir string, mode fetch.Mode) (*Adapter, *spy) {
	t.Helper()
	hc := fetch.NewHTTPClient(fetch.NewCache(cacheDir), mode)
	hc.BackoffBase = time.Millisecond // fake 429s are retried; keep the test fast
	sp := &spy{inner: hc}
	a := New(sp)
	a.now = func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) }
	return a, sp
}

func httpLoc(u string) catalog.Locator { return catalog.Locator{Kind: catalog.LocatorHTTP, URL: u} }

func TestFetchDocument(t *testing.T) {
	ts := newTestServer(t)
	a, sp := newAdapter(t, t.TempDir(), fetch.ModeOnline)
	ctx := context.Background()

	doc, err := a.FetchDocument(ctx, httpLoc(ts.URL+"/docs/table.md"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.URI != ts.URL+"/docs/table.md" || doc.FetchURL != doc.URI || string(doc.Content) != "| a | b |\n" ||
		doc.Format != "markdown" || doc.Digest != domain.Digest(doc.Content) || doc.RetrievedAt.IsZero() {
		t.Errorf("doc = %+v", doc)
	}
	if sp.reqs[0].Immutable || sp.reqs[0].Method != "" {
		t.Errorf("a plain doc is a mutable GET: %+v", sp.reqs[0])
	}

	// Format from Content-Type when the URL has no useful extension.
	doc, err = a.FetchDocument(ctx, httpLoc(ts.URL+"/docs/data"))
	if err != nil || doc.Format != "json" {
		t.Fatalf("json: %+v %v", doc, err)
	}

	// Release assets are immutable.
	doc, err = a.FetchDocument(ctx, httpLoc(ts.URL+"/releases/download/v1.18.0/cert-manager.yaml"))
	if err != nil || doc.Format != "yaml" || string(doc.Content) != "kind: Deployment\n" {
		t.Fatalf("asset: %+v %v", doc, err)
	}
	if last := sp.reqs[len(sp.reqs)-1]; !last.Immutable {
		t.Errorf("release asset must be immutable: %+v", last)
	}

	// Errors keep fetch semantics.
	if _, err := a.FetchDocument(ctx, httpLoc(ts.URL+"/missing.md")); !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("404: %v", err)
	}
	if _, err := a.FetchDocument(ctx, httpLoc(ts.URL+"/forbidden/asset")); !errors.Is(err, fetch.ErrUnavailable) {
		t.Errorf("403: %v", err)
	}
	for _, bad := range []string{"", "ftp://example.com/x", "/relative", "https://"} {
		if _, err := a.FetchDocument(ctx, httpLoc(bad)); err == nil {
			t.Errorf("url %q: want error", bad)
		}
	}
}

func TestFetchDocumentOfflineReplay(t *testing.T) {
	ts := newTestServer(t)
	dir := t.TempDir()
	a, _ := newAdapter(t, dir, fetch.ModeOnline)
	loc := httpLoc(ts.URL + "/docs/table.md")
	first, err := a.FetchDocument(context.Background(), loc)
	if err != nil {
		t.Fatal(err)
	}
	ts.Close()

	off, _ := newAdapter(t, dir, fetch.ModeOffline)
	again, err := off.FetchDocument(context.Background(), loc)
	if err != nil || again.Digest != first.Digest || string(again.Content) != string(first.Content) {
		t.Fatalf("offline: %v %+v", err, again)
	}
	if _, err := off.FetchDocument(context.Background(), httpLoc(ts.URL+"/docs/other.md")); !errors.Is(err, fetch.ErrOffline) {
		t.Errorf("uncached offline: %v", err)
	}
}

func TestProbe(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()
	assetURL := ts.URL + "/releases/download/v1.18.0/cert-manager.yaml"

	tests := []struct {
		name       string
		path       string
		wantExists bool
		wantDigest bool
		wantMethod string
		wantErr    error
		wantExcerp string
	}{
		{name: "HEAD succeeds", path: "/releases/download/v1.18.0/cert-manager.yaml", wantExists: true, wantMethod: "HEAD",
			wantExcerp: "HEAD " + assetURL + " -> HTTP 200 application/octet-stream"},
		{name: "HEAD not allowed falls back to GET", path: "/no-head/asset.tgz", wantExists: true, wantDigest: true, wantMethod: "HEAD,GET"},
		{name: "HEAD forbidden falls back to GET", path: "/signed/asset.tgz", wantExists: true, wantDigest: true, wantMethod: "HEAD,GET"},
		{name: "HEAD not implemented falls back to GET", path: "/head-501/asset", wantExists: true, wantDigest: true, wantMethod: "HEAD,GET"},
		{name: "404 means absent, not an error", path: "/releases/download/v9.9.9/cert-manager.yaml", wantExists: false, wantMethod: "HEAD",
			wantExcerp: "HEAD " + ts.URL + "/releases/download/v9.9.9/cert-manager.yaml -> HTTP 404"},
		{name: "410 means absent", path: "/gone/asset", wantExists: false, wantMethod: "HEAD"},
		{name: "forbidden everywhere is unavailable", path: "/forbidden/asset", wantMethod: "HEAD,GET", wantErr: fetch.ErrUnavailable},
		{name: "rate limited is unavailable", path: "/limited/asset", wantMethod: "HEAD", wantErr: fetch.ErrThrottled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, sp := newAdapter(t, t.TempDir(), fetch.ModeOnline)
			url := ts.URL + tc.path
			res, err := a.Probe(ctx, httpLoc(url), "v1.18.0")
			if got := sp.methods(); got != tc.wantMethod {
				t.Errorf("requests = %s, want %s", got, tc.wantMethod)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || !errors.Is(err, fetch.ErrUnavailable) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.Exists != tc.wantExists || res.Coordinate != url || res.URI != url {
				t.Errorf("result = %+v", res)
			}
			if (res.Digest != "") != tc.wantDigest {
				t.Errorf("digest = %q, want present=%v", res.Digest, tc.wantDigest)
			}
			ev := res.Evidence
			if ev.Kind != domain.EvidenceReleaseAsset || ev.URI != url || ev.ID == "" || ev.ContentDigest != res.Digest || ev.RetrievedAt.IsZero() {
				t.Errorf("evidence = %+v", ev)
			}
			if tc.wantExcerp != "" && ev.Excerpt != tc.wantExcerp {
				t.Errorf("excerpt = %q, want %q", ev.Excerpt, tc.wantExcerp)
			}
		})
	}
}

func TestProbeIsImmutableForReleaseAssets(t *testing.T) {
	ts := newTestServer(t)
	a, sp := newAdapter(t, t.TempDir(), fetch.ModeOnline)
	if _, err := a.Probe(context.Background(), httpLoc(ts.URL+"/releases/download/v1.18.0/cert-manager.yaml"), "v1.18.0"); err != nil {
		t.Fatal(err)
	}
	if !sp.reqs[0].Immutable || sp.reqs[0].Method != http.MethodHead {
		t.Errorf("request = %+v", sp.reqs[0])
	}
	if _, err := a.Probe(context.Background(), httpLoc(ts.URL+"/forbidden/asset"), "v1"); err == nil {
		t.Fatal("want error")
	}
	if last := sp.reqs[len(sp.reqs)-1]; last.Immutable {
		t.Errorf("unversioned URL must not be immutable: %+v", last)
	}
}

func TestProbeCachesAndReplaysOffline(t *testing.T) {
	ts := newTestServer(t)
	dir := t.TempDir()
	a, _ := newAdapter(t, dir, fetch.ModeOnline)
	ctx := context.Background()
	present := httpLoc(ts.URL + "/releases/download/v1.18.0/cert-manager.yaml")
	absent := httpLoc(ts.URL + "/releases/download/v0.0.1/nothing.yaml")

	r1, err := a.Probe(ctx, present, "v1.18.0")
	if err != nil || !r1.Exists {
		t.Fatalf("present: %v %+v", err, r1)
	}
	r2, err := a.Probe(ctx, absent, "v0.0.1")
	if err != nil || r2.Exists {
		t.Fatalf("absent: %v %+v", err, r2)
	}
	// Second online probe is served from the cache.
	if _, err := a.Probe(ctx, present, "v1.18.0"); err != nil {
		t.Fatal(err)
	}
	if n := ts.count("HEAD /releases/download/v1.18.0/cert-manager.yaml"); n != 1 {
		t.Errorf("server saw %d HEAD requests, want 1", n)
	}
	ts.Close()

	off, _ := newAdapter(t, dir, fetch.ModeOffline)
	o1, err := off.Probe(ctx, present, "v1.18.0")
	if err != nil || !o1.Exists || o1.Evidence.ID != r1.Evidence.ID {
		t.Fatalf("offline present: %v %+v", err, o1)
	}
	o2, err := off.Probe(ctx, absent, "v0.0.1")
	if err != nil || o2.Exists || o2.Evidence.ID != r2.Evidence.ID {
		t.Fatalf("offline absent: %v %+v", err, o2)
	}
	if _, err := off.Probe(ctx, httpLoc(ts.URL+"/never/probed"), "v1"); !errors.Is(err, fetch.ErrOffline) {
		t.Errorf("uncached offline probe: %v", err)
	}
}

func TestProbeUnreachableServer(t *testing.T) {
	ts := newTestServer(t)
	url := ts.URL + "/releases/download/v1.18.0/cert-manager.yaml"
	ts.Close()
	a, _ := newAdapter(t, t.TempDir(), fetch.ModeOnline)
	_, err := a.Probe(context.Background(), httpLoc(url), "v1.18.0")
	if !errors.Is(err, fetch.ErrUnavailable) || fetch.StateFor(err) != domain.SourceUnavailable {
		t.Fatalf("got %v", err)
	}
}

func TestProbeOtherServerErrorsAreNotAbsence(t *testing.T) {
	ts := newTestServer(t)
	a, _ := newAdapter(t, t.TempDir(), fetch.ModeOnline)
	_, err := a.Probe(context.Background(), httpLoc(ts.URL+"/boom"), "v1")
	if err == nil || errors.Is(err, fetch.ErrNotFound) {
		t.Fatalf("a 500 is an error, not an absent artifact: %v", err)
	}
	if fetch.StateFor(err) != domain.SourceError {
		t.Errorf("state = %q", fetch.StateFor(err))
	}
}

func TestProbeInvalidURL(t *testing.T) {
	a, _ := newAdapter(t, t.TempDir(), fetch.ModeOnline)
	for _, bad := range []string{"", "oci://x/y", "not a url"} {
		if _, err := a.Probe(context.Background(), httpLoc(bad), "v1"); err == nil {
			t.Errorf("url %q: want error", bad)
		}
	}
}

func TestRegister(t *testing.T) {
	reg := sources.NewRegistry()
	Register(reg, fetch.NewHTTPClient(nil, fetch.ModeOffline))
	if _, err := reg.DocumentFetcher(catalog.LocatorHTTP); err != nil {
		t.Error(err)
	}
	if _, err := reg.Probe(catalog.LocatorHTTP); err != nil {
		t.Error(err)
	}
	if _, err := reg.VersionLister(catalog.LocatorHTTP); err == nil {
		t.Error("http must not claim a version lister")
	}
}
