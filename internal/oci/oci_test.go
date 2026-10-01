package oci

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

const (
	imageIndexType = "application/vnd.oci.image.index.v1+json"
	imageIndexBody = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`
)

func newTestAdapter(t *testing.T, cache *fetch.Cache, mode fetch.Mode, opts ...Option) *Adapter {
	t.Helper()
	return New(fetch.NewHTTPClient(cache, mode), opts...)
}

func locator(repo string) catalog.Locator {
	return catalog.Locator{Kind: catalog.LocatorOCI, Repository: repo}
}

func TestParseRepository(t *testing.T) {
	tests := []struct {
		in      string
		want    Repository
		wantErr bool
	}{
		{in: "quay.io/jetstack/cert-manager-controller", want: Repository{
			Given: "quay.io/jetstack/cert-manager-controller", Registry: "quay.io",
			Name: "jetstack/cert-manager-controller", APIBase: "https://quay.io"}},
		{in: "docker.io/istio/pilot", want: Repository{
			Given: "docker.io/istio/pilot", Registry: "docker.io", Name: "istio/pilot",
			APIBase: "https://registry-1.docker.io"}},
		{in: "docker.io/nginx", want: Repository{
			Given: "docker.io/nginx", Registry: "docker.io", Name: "library/nginx",
			APIBase: "https://registry-1.docker.io"}},
		{in: "nginx", want: Repository{
			Given: "nginx", Registry: "docker.io", Name: "library/nginx",
			APIBase: "https://registry-1.docker.io"}},
		{in: "istio/pilot", want: Repository{
			Given: "istio/pilot", Registry: "docker.io", Name: "istio/pilot",
			APIBase: "https://registry-1.docker.io"}},
		{in: "index.docker.io/istio/pilot", want: Repository{
			Given: "index.docker.io/istio/pilot", Registry: "docker.io", Name: "istio/pilot",
			APIBase: "https://registry-1.docker.io"}},
		{in: "oci://ghcr.io/istio/release/charts/istiod", want: Repository{
			Given: "ghcr.io/istio/release/charts/istiod", Registry: "ghcr.io",
			Name: "istio/release/charts/istiod", APIBase: "https://ghcr.io"}},
		{in: "gcr.io/istio-release/pilot", want: Repository{
			Given: "gcr.io/istio-release/pilot", Registry: "gcr.io", Name: "istio-release/pilot",
			APIBase: "https://gcr.io"}},
		{in: "127.0.0.1:5000/team/app", want: Repository{
			Given: "127.0.0.1:5000/team/app", Registry: "127.0.0.1:5000", Name: "team/app",
			APIBase: "http://127.0.0.1:5000"}},
		{in: "localhost:5000/app", want: Repository{
			Given: "localhost:5000/app", Registry: "localhost:5000", Name: "app", APIBase: "http://localhost:5000"}},
		{in: "registry.example.com:8443/a/b", want: Repository{
			Given: "registry.example.com:8443/a/b", Registry: "registry.example.com:8443",
			Name: "a/b", APIBase: "https://registry.example.com:8443"}},
		{in: "", wantErr: true},
		{in: "quay.io/", wantErr: true},
		{in: "quay.io/jetstack/cert-manager-controller:v1.18.0", wantErr: true},
		{in: "quay.io/jetstack/x@sha256:abcd", wantErr: true},
		{in: "quay.io/Jetstack/x", wantErr: true},
		{in: "quay.io//x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseRepository(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestCoordinateAndNormalizeTag(t *testing.T) {
	r, _ := ParseRepository("docker.io/istio/pilot")
	if got := r.Coordinate("1.26.0"); got != "docker.io/istio/pilot:1.26.0" {
		t.Errorf("tag coordinate = %q", got)
	}
	d := "sha256:" + strings.Repeat("ab", 32)
	if got := r.Coordinate(d); got != "docker.io/istio/pilot@"+d {
		t.Errorf("digest coordinate = %q", got)
	}
	for _, tt := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"1.26.0", "1.26.0", true},
		{"v1.18.0-rc.1", "v1.18.0-rc.1", true},
		{"1.2.3+build.5", "1.2.3_build.5", true}, // Helm OCI convention
		{d, d, true},
		{"", "", false},
		{"bad tag", "bad tag", false},
		{"-leading", "-leading", false},
		{strings.Repeat("a", 129), strings.Repeat("a", 129), false},
	} {
		got, ok := NormalizeTag(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("NormalizeTag(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestParseChallenges(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []challenge
	}{
		{
			name: "docker hub",
			in:   []string{`Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:istio/pilot:pull"`},
			want: []challenge{{Scheme: "Bearer", Params: map[string]string{
				"realm": "https://auth.docker.io/token", "service": "registry.docker.io", "scope": "repository:istio/pilot:pull"}}},
		},
		{
			name: "comma inside quoted scope and spaces around commas",
			in:   []string{`bearer realm="https://r/token" , scope="repository:a/b:pull,push", service=svc`},
			want: []challenge{{Scheme: "Bearer", Params: map[string]string{
				"realm": "https://r/token", "scope": "repository:a/b:pull,push", "service": "svc"}}},
		},
		{
			name: "two challenges in one header",
			in:   []string{`Basic realm="registry", Bearer realm="https://r/token",service="s"`},
			want: []challenge{
				{Scheme: "Basic", Params: map[string]string{"realm": "registry"}},
				{Scheme: "Bearer", Params: map[string]string{"realm": "https://r/token", "service": "s"}},
			},
		},
		{
			name: "two header values and escaped quote",
			in:   []string{`Basic realm="a \"quoted\" realm"`, `Bearer realm="https://r/t"`},
			want: []challenge{
				{Scheme: "Basic", Params: map[string]string{"realm": `a "quoted" realm`}},
				{Scheme: "Bearer", Params: map[string]string{"realm": "https://r/t"}},
			},
		},
		{
			name: "bare values",
			in:   []string{`Bearer realm=https://r/token,service=s`},
			want: []challenge{{Scheme: "Bearer", Params: map[string]string{"realm": "https://r/token", "service": "s"}}},
		},
		{name: "empty", in: []string{""}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseChallenges(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

func TestIsNoiseTag(t *testing.T) {
	sha := strings.Repeat("9f", 20)
	tests := []struct {
		tag  string
		want bool
	}{
		{"1.26.0", false},
		{"v1.18.0-rc.1", false},
		{"1.26.0-distroless", false},
		{"1.26.0-debug", false},
		{"latest", false},
		{"deadbeef", false}, // short hex is not filtered: could be a numeric tag
		{"sha256-" + strings.Repeat("ab", 32) + ".sig", true},
		{"sha256-" + strings.Repeat("ab", 32) + ".att", true},
		{"sha256-" + strings.Repeat("ab", 32) + ".sbom", true},
		{"sha256-" + strings.Repeat("ab", 32), true}, // referrers fallback tag
		{sha, true},
		{"1.31.0-alpha." + sha, true},
		{"release-1.31-" + sha, true},
		{"v1.0.0-" + sha[:39], false},
	}
	for _, tt := range tests {
		if got := IsNoiseTag(tt.tag); got != tt.want {
			t.Errorf("IsNoiseTag(%q) = %v, want %v", tt.tag, got, tt.want)
		}
	}
	got := FilterNoiseTags([]string{"1.0.0", sha, "sha256-" + strings.Repeat("0", 64) + ".sig", "1.1.0"})
	if !reflect.DeepEqual(got, []string{"1.0.0", "1.1.0"}) {
		t.Errorf("FilterNoiseTags = %v", got)
	}
}

func TestProbeBearerTokenFlow(t *testing.T) {
	reg := newFakeRegistry(t)
	reg.RequireAuth = true
	reg.QuoteScope = true
	digest := reg.addManifest("istio/pilot", "1.26.0", imageIndexType, imageIndexBody)
	reg.addManifest("istio/pilot", "1.26.1", imageIndexType, imageIndexBody+" ")

	cacheDir := t.TempDir()
	a := newTestAdapter(t, fetch.NewCache(cacheDir), fetch.ModeOnline)
	ctx := context.Background()
	repo := reg.host() + "/istio/pilot"

	res, err := a.Probe(ctx, locator(repo), "1.26.0")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !res.Exists {
		t.Fatalf("expected the manifest to exist: %+v", res)
	}
	if res.Coordinate != repo+":1.26.0" {
		t.Errorf("Coordinate = %q", res.Coordinate)
	}
	if res.Digest != digest {
		t.Errorf("Digest = %q, want %q", res.Digest, digest)
	}
	wantURI := fmt.Sprintf("http://%s/v2/istio/pilot/manifests/%s", reg.host(), digest)
	if res.URI != wantURI {
		t.Errorf("URI = %q, want %q", res.URI, wantURI)
	}
	ev := res.Evidence
	if ev.Kind != domain.EvidenceRegistry || ev.URI != wantURI || ev.ContentDigest != digest || ev.ID == "" {
		t.Errorf("evidence = %+v", ev)
	}
	if want := "digest=" + digest + " mediaType=" + imageIndexType; ev.Excerpt != want {
		t.Errorf("excerpt = %q, want %q", ev.Excerpt, want)
	}
	if ev.Locator != "manifests/1.26.0" {
		t.Errorf("locator = %q", ev.Locator)
	}

	// The challenge scope (including its comma) is forwarded to the realm.
	if got := reg.tokenReqs[0]["scope"]; len(got) != 1 || got[0] != "repository:istio/pilot:pull,push" {
		t.Errorf("token scope = %v", got)
	}
	if got := reg.tokenReqs[0].Get("service"); got != "fake-registry" {
		t.Errorf("token service = %q", got)
	}

	// A second probe of the same repository reuses the in-memory token.
	if _, err := a.Probe(ctx, locator(repo), "1.26.1"); err != nil {
		t.Fatalf("second Probe: %v", err)
	}
	if n := reg.hitCount("token"); n != 1 {
		t.Errorf("token endpoint hit %d times, want 1", n)
	}

	// The token must never reach the on-disk cache.
	assertNoTokenOnDisk(t, cacheDir)
}

// assertNoTokenOnDisk fails when the fetch cache holds the token endpoint
// response or the bearer token itself.
func assertNoTokenOnDisk(t *testing.T, dir string) {
	t.Helper()
	n := 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		n++
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "/token") || strings.Contains(string(b), testToken) {
			t.Errorf("cache file %s contains token material", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected the registry responses to be cached")
	}
}

func TestTokenLifecycle(t *testing.T) {
	ctx := context.Background()
	newReg := func(t *testing.T) (*fakeRegistry, string) {
		reg := newFakeRegistry(t)
		reg.RequireAuth = true
		for _, tag := range []string{"a", "b", "c"} {
			reg.addManifest("app", tag, imageIndexType, imageIndexBody+strings.Repeat(" ", len(tag)))
		}
		return reg, reg.host() + "/app"
	}

	t.Run("expired token is renewed", func(t *testing.T) {
		reg, repo := newReg(t)
		reg.TokenExpiresIn = 60 // lifetime 60s minus the safety skew
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		now := time.Unix(1_700_000_000, 0)
		a.now = func() time.Time { return now }

		if _, err := a.Probe(ctx, locator(repo), "a"); err != nil {
			t.Fatal(err)
		}
		now = now.Add(30 * time.Second)
		if _, err := a.Probe(ctx, locator(repo), "b"); err != nil {
			t.Fatal(err)
		}
		if n := reg.hitCount("token"); n != 1 {
			t.Errorf("token still valid: %d token requests, want 1", n)
		}
		now = now.Add(2 * time.Minute)
		if _, err := a.Probe(ctx, locator(repo), "c"); err != nil {
			t.Fatal(err)
		}
		if n := reg.hitCount("token"); n != 2 {
			t.Errorf("token expired: %d token requests, want 2", n)
		}
	})
	t.Run("rejected token is replaced", func(t *testing.T) {
		reg, repo := newReg(t)
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		if _, err := a.Probe(ctx, locator(repo), "a"); err != nil {
			t.Fatal(err)
		}
		reg.setToken("tok-rotated") // the cached token is now refused
		res, err := a.Probe(ctx, locator(repo), "b")
		if err != nil || !res.Exists {
			t.Fatalf("Probe after rotation = %+v, %v", res, err)
		}
		if n := reg.hitCount("token"); n != 2 {
			t.Errorf("%d token requests, want 2", n)
		}
	})
	t.Run("a token the registry keeps rejecting ends as unavailable", func(t *testing.T) {
		reg, repo := newReg(t)
		reg.RequireAuth = true
		// The token endpoint issues a token the API never accepts.
		reg.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/token" {
				w.Write([]byte(`{"token":"useless"}`))
				return
			}
			reg.handle(w, req)
		})
		_, err := newTestAdapter(t, nil, fetch.ModeOnline).Probe(ctx, locator(repo), "a")
		if !errors.Is(err, fetch.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
	t.Run("access_token and default lifetime", func(t *testing.T) {
		reg, repo := newReg(t)
		reg.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/token" {
				w.Write([]byte(`{"access_token":"` + testToken + `"}`))
				return
			}
			reg.handle(w, req)
		})
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		if res, err := a.Probe(ctx, locator(repo), "a"); err != nil || !res.Exists {
			t.Fatalf("Probe = %+v, %v", res, err)
		}
		if tok := a.tokens[tokenKey(Repository{APIBase: "http://" + reg.host(), Name: "app"})]; tok.value != testToken {
			t.Errorf("cached token = %+v", tok)
		}
	})
}

func TestProbeAnonymousRegistryNeedsNoToken(t *testing.T) {
	reg := newFakeRegistry(t) // no auth
	reg.addManifest("app", "v1", imageIndexType, imageIndexBody)
	a := newTestAdapter(t, nil, fetch.ModeOnline)
	res, err := a.Probe(context.Background(), locator(reg.host()+"/app"), "v1")
	if err != nil || !res.Exists {
		t.Fatalf("Probe = %+v, %v", res, err)
	}
	if reg.hitCount("token") != 0 {
		t.Error("token endpoint must not be contacted by anonymous registries")
	}
}

func TestProbeMissing(t *testing.T) {
	reg := newFakeRegistry(t)
	reg.RequireAuth = true
	reg.addManifest("istio/pilot", "1.26.0", imageIndexType, imageIndexBody)
	a := newTestAdapter(t, nil, fetch.ModeOnline)
	ctx := context.Background()

	for name, repo := range map[string]string{
		"unknown tag":        reg.host() + "/istio/pilot",
		"unknown repository": reg.host() + "/does/not-exist",
	} {
		t.Run(name, func(t *testing.T) {
			res, err := a.Probe(ctx, locator(repo), "0.0.0-nope")
			if err != nil {
				t.Fatalf("a missing manifest must not be an error, got %v", err)
			}
			if res.Exists {
				t.Fatal("Exists = true")
			}
			if res.Coordinate != repo+":0.0.0-nope" {
				t.Errorf("Coordinate = %q", res.Coordinate)
			}
			if res.Evidence.ID == "" || res.Evidence.Kind != domain.EvidenceRegistry || !strings.Contains(res.Evidence.Excerpt, "404") {
				t.Errorf("evidence = %+v", res.Evidence)
			}
		})
	}
}

func TestProbeHeadFallbacks(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*fakeRegistry)
		wantHEAD int
		wantGET  int
	}{
		{"HEAD rejected with 405", func(r *fakeRegistry) { r.HeadStatus = http.StatusMethodNotAllowed }, 1, 1},
		{"HEAD without Docker-Content-Digest", func(r *fakeRegistry) { r.OmitDigestOnHead = true }, 1, 1},
		{"HEAD works", func(r *fakeRegistry) {}, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := newFakeRegistry(t)
			tt.setup(reg)
			digest := reg.addManifest("app", "v1", "application/vnd.docker.distribution.manifest.v2+json", `{"schemaVersion":2}`)
			a := newTestAdapter(t, nil, fetch.ModeOnline)
			res, err := a.Probe(context.Background(), locator(reg.host()+"/app"), "v1")
			if err != nil || !res.Exists {
				t.Fatalf("Probe = %+v, %v", res, err)
			}
			if res.Digest != digest {
				t.Errorf("Digest = %q, want %q (computed from the body when the header is missing)", res.Digest, digest)
			}
			if !strings.Contains(res.Evidence.Excerpt, "mediaType=application/vnd.docker.distribution.manifest.v2+json") {
				t.Errorf("excerpt = %q", res.Evidence.Excerpt)
			}
			if got := reg.hitCount("HEAD manifests"); got != tt.wantHEAD {
				t.Errorf("HEAD hits = %d, want %d", got, tt.wantHEAD)
			}
			if got := reg.hitCount("GET manifests"); got != tt.wantGET {
				t.Errorf("GET hits = %d, want %d", got, tt.wantGET)
			}
		})
	}
}

func TestProbeUnavailable(t *testing.T) {
	ctx := context.Background()

	t.Run("registry down", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		host := strings.TrimPrefix(srv.URL, "http://")
		srv.Close() // connection refused
		_, err := newTestAdapter(t, nil, fetch.ModeOnline).Probe(ctx, locator(host+"/x/y"), "1.0.0")
		if !errors.Is(err, fetch.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
	t.Run("blocked by proxy (403)", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "blocked by policy", http.StatusForbidden)
		}))
		defer srv.Close()
		_, err := newTestAdapter(t, nil, fetch.ModeOnline).Probe(ctx, locator(strings.TrimPrefix(srv.URL, "http://")+"/x/y"), "1.0.0")
		if !errors.Is(err, fetch.ErrUnavailable) || errors.Is(err, fetch.ErrNotFound) {
			t.Fatalf("err = %v, want ErrUnavailable only", err)
		}
	})
	t.Run("401 without challenge", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()
		_, err := newTestAdapter(t, nil, fetch.ModeOnline).Probe(ctx, locator(strings.TrimPrefix(srv.URL, "http://")+"/x/y"), "1.0.0")
		if !errors.Is(err, fetch.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
	t.Run("token endpoint refuses", func(t *testing.T) {
		reg := newFakeRegistry(t)
		reg.RequireAuth = true
		reg.TokenStatus = http.StatusNotFound // must not be mistaken for "missing"
		_, err := newTestAdapter(t, nil, fetch.ModeOnline).Probe(ctx, locator(reg.host()+"/x/y"), "1.0.0")
		if !errors.Is(err, fetch.ErrUnavailable) || errors.Is(err, fetch.ErrNotFound) {
			t.Fatalf("err = %v, want ErrUnavailable only", err)
		}
	})
	t.Run("server error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusBadGateway)
		}))
		defer srv.Close()
		_, err := newTestAdapter(t, nil, fetch.ModeOnline).Probe(ctx, locator(strings.TrimPrefix(srv.URL, "http://")+"/x/y"), "1.0.0")
		if !errors.Is(err, fetch.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
	t.Run("offline and not cached", func(t *testing.T) {
		reg := newFakeRegistry(t)
		_, err := newTestAdapter(t, fetch.NewCache(t.TempDir()), fetch.ModeOffline).Probe(ctx, locator(reg.host()+"/x/y"), "1.0.0")
		if !errors.Is(err, fetch.ErrOffline) || fetch.StateFor(err) != domain.SourceUnavailable {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("invalid input", func(t *testing.T) {
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		if _, err := a.Probe(ctx, locator("quay.io/a/b"), "not a tag"); err == nil {
			t.Error("expected error for an invalid tag")
		}
		if _, err := a.Probe(ctx, locator(""), "1.0.0"); err == nil {
			t.Error("expected error for an empty repository")
		}
	})
}

func TestProbeOfflineReplayNeedsNoToken(t *testing.T) {
	reg := newFakeRegistry(t)
	reg.RequireAuth = true
	digest := reg.addManifest("istio/pilot", "1.26.0", imageIndexType, imageIndexBody)
	cache := fetch.NewCache(t.TempDir())
	repo := reg.host() + "/istio/pilot"
	ctx := context.Background()

	if _, err := newTestAdapter(t, cache, fetch.ModeOnline).Probe(ctx, locator(repo), "1.26.0"); err != nil {
		t.Fatal(err)
	}
	reg.srv.Close() // nothing is reachable any more

	// A brand-new adapter (empty token cache) replays from the cache.
	res, err := newTestAdapter(t, cache, fetch.ModeOffline).Probe(ctx, locator(repo), "1.26.0")
	if err != nil || !res.Exists || res.Digest != digest {
		t.Fatalf("offline replay = %+v, %v", res, err)
	}
}

// recordingClient is a fetch.Client capturing requests and answering from a
// fixed function.
type recordingClient struct {
	reqs []fetch.Request
	do   func(fetch.Request) (*fetch.Document, error)
}

func (c *recordingClient) Do(_ context.Context, req fetch.Request) (*fetch.Document, error) {
	c.reqs = append(c.reqs, req)
	return c.do(req)
}

func TestProbeMutabilityAndAccept(t *testing.T) {
	digest := "sha256:" + strings.Repeat("ab", 32)
	c := &recordingClient{do: func(req fetch.Request) (*fetch.Document, error) {
		return &fetch.Document{
			URL: req.URL, Status: 200, RetrievedAt: time.Unix(1700000000, 0),
			Header: http.Header{"Docker-Content-Digest": {digest}, "Content-Type": {imageIndexType}},
		}, nil
	}}
	a := New(c, WithManifestTTL(7*time.Minute))
	ctx := context.Background()

	if _, err := a.Probe(ctx, locator("quay.io/jetstack/cert-manager-controller"), "v1.18.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Probe(ctx, locator("quay.io/jetstack/cert-manager-controller"), digest); err != nil {
		t.Fatal(err)
	}
	byTag, byDigest := c.reqs[0], c.reqs[1]
	if byTag.Immutable || byTag.TTL != 7*time.Minute {
		t.Errorf("tag-addressed probe must be mutable with the short TTL: %+v", byTag)
	}
	if !byDigest.Immutable {
		t.Errorf("digest-addressed probe must be immutable: %+v", byDigest)
	}
	if byTag.Method != http.MethodHead || byTag.URL != "https://quay.io/v2/jetstack/cert-manager-controller/manifests/v1.18.0" {
		t.Errorf("request = %+v", byTag)
	}
	for _, mt := range []string{
		"application/vnd.oci.image.index.v1+json", "application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json", "application/vnd.docker.distribution.manifest.v2+json",
	} {
		if !strings.Contains(byTag.Header.Get("Accept"), mt) {
			t.Errorf("Accept %q lacks %s", byTag.Header.Get("Accept"), mt)
		}
	}
}

func TestDockerHubMapping(t *testing.T) {
	reg := newFakeRegistry(t)
	reg.addManifest("istio/pilot", "1.26.0", imageIndexType, imageIndexBody)
	reg.addManifest("library/nginx", "1.27", imageIndexType, imageIndexBody)
	a := newTestAdapter(t, nil, fetch.ModeOnline, WithEndpoint("docker.io", reg.srv.URL))
	ctx := context.Background()

	res, err := a.Probe(ctx, locator("docker.io/istio/pilot"), "1.26.0")
	if err != nil || !res.Exists {
		t.Fatalf("istio/pilot: %+v, %v", res, err)
	}
	if res.Coordinate != "docker.io/istio/pilot:1.26.0" {
		t.Errorf("Coordinate = %q (must keep the user-facing form)", res.Coordinate)
	}
	for _, repo := range []string{"docker.io/nginx", "nginx"} {
		res, err = a.Probe(ctx, locator(repo), "1.27")
		if err != nil || !res.Exists {
			t.Fatalf("%s: %+v, %v", repo, res, err)
		}
		if res.Coordinate != repo+":1.27" {
			t.Errorf("Coordinate = %q", res.Coordinate)
		}
	}
}

func tagList(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("1.%03d.0", i)
	}
	return out
}

func TestListArtifactVersions(t *testing.T) {
	sha := strings.Repeat("9f", 20)
	sig := "sha256-" + strings.Repeat("ab", 32) + ".sig"
	all := append(tagList(5), sig, "1.31.0-alpha."+sha, sha, "1.005.0-distroless")

	setup := func(t *testing.T, pageSize int) (*fakeRegistry, string) {
		reg := newFakeRegistry(t)
		reg.RequireAuth = true
		reg.PageSize = pageSize
		reg.tags["istio/release/charts/istiod"] = all
		return reg, reg.host() + "/istio/release/charts/istiod"
	}
	versions := func(avs []sources.ArtifactVersion) []string {
		var out []string
		for _, av := range avs {
			out = append(out, av.Version)
		}
		return out
	}
	ctx := context.Background()

	t.Run("paginated, noise filtered by default", func(t *testing.T) {
		reg, repo := setup(t, 3)
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		got, err := a.ListArtifactVersions(ctx, locator(repo))
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"1.000.0", "1.001.0", "1.002.0", "1.003.0", "1.004.0", "1.005.0-distroless"}
		if !reflect.DeepEqual(versions(got), want) {
			t.Fatalf("versions = %v, want %v", versions(got), want)
		}
		if n := reg.hitCount("GET tags"); n != 3 { // 9 tags, 3 per page
			t.Errorf("tags/list requests = %d, want 3", n)
		}
		if n := reg.hitCount("token"); n != 1 {
			t.Errorf("token requests = %d, want 1", n)
		}
		for _, av := range got {
			if len(av.Fields) != 0 {
				t.Errorf("Fields must be empty, got %v", av.Fields)
			}
			if av.URI != fmt.Sprintf("http://%s/v2/istio/release/charts/istiod/manifests/%s", reg.host(), av.Version) {
				t.Errorf("URI = %q", av.URI)
			}
			ev := av.Evidence
			if ev.Kind != domain.EvidenceRegistry || ev.Locator != "tags["+av.Version+"]" ||
				!strings.Contains(ev.URI, "/tags/list") || ev.ContentDigest == "" || ev.ID == "" {
				t.Errorf("evidence = %+v", ev)
			}
		}
		// The first tag lives on the first page, the last one on the third.
		if got[0].Evidence.URI == got[len(got)-1].Evidence.URI {
			t.Error("evidence should reference the page that listed the tag")
		}
	})
	t.Run("noise filter off", func(t *testing.T) {
		_, repo := setup(t, 0)
		a := newTestAdapter(t, nil, fetch.ModeOnline, WithNoiseFilter(false))
		got, err := a.ListArtifactVersions(ctx, locator(repo))
		if err != nil || len(got) != len(all) {
			t.Fatalf("got %d versions (%v), want %d", len(got), err, len(all))
		}
	})
	t.Run("tagPattern includes", func(t *testing.T) {
		_, repo := setup(t, 0)
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		loc := locator(repo)
		loc.TagPattern = `^1\.00[0-2]\.0$`
		got, err := a.ListArtifactVersions(ctx, loc)
		if err != nil || !reflect.DeepEqual(versions(got), []string{"1.000.0", "1.001.0", "1.002.0"}) {
			t.Fatalf("got %v, %v", versions(got), err)
		}
		loc.TagPattern = "("
		if _, err := a.ListArtifactVersions(ctx, loc); err == nil {
			t.Error("expected error for an invalid tagPattern")
		}
	})
	t.Run("ListTags is unfiltered", func(t *testing.T) {
		_, repo := setup(t, 4)
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		got, err := a.ListTags(ctx, repo)
		if err != nil || !reflect.DeepEqual(got, all) {
			t.Fatalf("got %v, %v", got, err)
		}
	})
	t.Run("unknown repository", func(t *testing.T) {
		reg, _ := setup(t, 0)
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		_, err := a.ListArtifactVersions(ctx, locator(reg.host()+"/nope/nope"))
		if !errors.Is(err, fetch.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("unreachable", func(t *testing.T) {
		reg, repo := setup(t, 0)
		reg.srv.Close()
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		if _, err := a.ListArtifactVersions(ctx, locator(repo)); !errors.Is(err, fetch.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
}

func TestNextLink(t *testing.T) {
	tests := []struct {
		name, link, want string
		wantErr          bool
	}{
		{name: "none", link: "", want: ""},
		{name: "relative", link: `</v2/a/tags/list?last=x&n=10>; rel="next"`, want: "https://r.example/v2/a/tags/list?last=x&n=10"},
		{name: "absolute same host", link: `<https://r.example/v2/a/tags/list?last=x>; rel="next"`, want: "https://r.example/v2/a/tags/list?last=x"},
		{name: "other rel", link: `</v2/a>; rel="prev"`, want: ""},
		{name: "other host refused", link: `<https://evil.example/v2/a>; rel="next"`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nextLink("https://r.example/v2/a/tags/list?n=10", tt.link, "https://r.example")
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("nextLink = %q, %v; want %q (err %v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

const (
	helmManifestType = "application/vnd.oci.image.manifest.v1+json"
	chartConfigJSON  = `{"name":"istiod","version":"1.26.0","description":"Helm chart for istio control plane","apiVersion":"v2","appVersion":"1.26.0","type":"application"}`
)

func helmManifest(configDigest string, size int) string {
	return fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,`+
		`"config":{"mediaType":%q,"digest":%q,"size":%d},`+
		`"layers":[{"mediaType":%q,"digest":"sha256:%s","size":1234}],`+
		`"annotations":{"org.opencontainers.image.version":"1.26.0","org.opencontainers.image.created":"2025-05-01T10:00:00Z"}}`,
		helmManifestType, HelmConfigMediaType, configDigest, size, HelmChartLayerMediaType, strings.Repeat("c", 64))
}

func TestChartConfig(t *testing.T) {
	ctx := context.Background()
	reg := newFakeRegistry(t)
	reg.RequireAuth = true
	name := "istio/release/charts/istiod"
	cfgDigest := reg.addBlob(name, []byte(chartConfigJSON))
	reg.addManifest(name, "1.26.0", helmManifestType, helmManifest(cfgDigest, len(chartConfigJSON)))
	repo := reg.host() + "/" + name

	t.Run("reads name, version and appVersion", func(t *testing.T) {
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		ch, err := a.ChartConfig(ctx, repo, "1.26.0")
		if err != nil {
			t.Fatal(err)
		}
		want := ChartConfig{APIVersion: "v2", Name: "istiod", Version: "1.26.0", AppVersion: "1.26.0",
			Description: "Helm chart for istio control plane", Type: "application"}
		if ch.Config != want {
			t.Errorf("Config = %+v\nwant     %+v", ch.Config, want)
		}
		if ch.ConfigDigest != cfgDigest || ch.Manifest.Config.MediaType != HelmConfigMediaType {
			t.Errorf("digest/manifest = %q / %+v", ch.ConfigDigest, ch.Manifest.Config)
		}
		if got := ch.Manifest.Annotations["org.opencontainers.image.version"]; got != "1.26.0" {
			t.Errorf("annotations = %v", ch.Manifest.Annotations)
		}
		if len(ch.Manifest.Layers) != 1 || ch.Manifest.Layers[0].MediaType != HelmChartLayerMediaType {
			t.Errorf("layers = %+v", ch.Manifest.Layers)
		}
		ev := ch.Evidence
		if ev.Kind != domain.EvidenceStructured || ev.ContentDigest != cfgDigest ||
			!strings.HasSuffix(ev.URI, "/blobs/"+cfgDigest) || ev.Excerpt != "name: istiod, version: 1.26.0, appVersion: 1.26.0" {
			t.Errorf("evidence = %+v", ev)
		}
	})
	t.Run("not a helm chart", func(t *testing.T) {
		reg.addManifest(name, "image", helmManifestType,
			`{"schemaVersion":2,"config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"sha256:`+strings.Repeat("d", 64)+`","size":1}}`)
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		if _, err := a.ChartConfig(ctx, repo, "image"); !errors.Is(err, ErrNotHelmChart) {
			t.Fatalf("err = %v, want ErrNotHelmChart", err)
		}
	})
	t.Run("blob digest mismatch", func(t *testing.T) {
		bad := "sha256:" + strings.Repeat("e", 64)
		reg.mu.Lock()
		reg.blobs[name+"@"+bad] = []byte(chartConfigJSON)
		reg.mu.Unlock()
		reg.addManifest(name, "corrupt", helmManifestType, helmManifest(bad, 10))
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		if _, err := a.ChartConfig(ctx, repo, "corrupt"); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
			t.Fatalf("err = %v, want digest mismatch", err)
		}
	})
	t.Run("blob unavailable keeps the sentinel", func(t *testing.T) {
		// ghcr.io: blob downloads redirect to a host that is unreachable.
		blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "blocked", http.StatusForbidden)
		}))
		defer blocked.Close()
		r2 := newFakeRegistry(t)
		d := "sha256:" + strings.Repeat("a", 64)
		r2.addManifest(name, "1.26.0", helmManifestType, helmManifest(d, 10))
		r2.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if strings.Contains(req.URL.Path, "/blobs/") {
				http.Redirect(w, req, blocked.URL+"/blob", http.StatusTemporaryRedirect)
				return
			}
			r2.handle(w, req)
		})
		a := newTestAdapter(t, nil, fetch.ModeOnline)
		_, err := a.ChartConfig(ctx, r2.host()+"/"+name, "1.26.0")
		if !errors.Is(err, fetch.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		// The manifest itself (with its annotations) is still readable.
		m, err := a.GetManifest(ctx, r2.host()+"/"+name, "1.26.0")
		if err != nil || m.Annotations["org.opencontainers.image.version"] != "1.26.0" {
			t.Fatalf("GetManifest = %+v, %v", m, err)
		}
	})
}

func TestRegister(t *testing.T) {
	reg := sources.NewRegistry()
	Register(reg, fetch.NewHTTPClient(nil, fetch.ModeOnline))
	if _, err := reg.Probe(catalog.LocatorOCI); err != nil {
		t.Errorf("probe not registered: %v", err)
	}
	if _, err := reg.VersionIndex(catalog.LocatorOCI); err != nil {
		t.Errorf("version index not registered: %v", err)
	}
}
