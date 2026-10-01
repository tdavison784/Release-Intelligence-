package helm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
)

// packageServer serves a chart repository: an index.yaml whose single entry
// points at a tarball, plus the tarball itself, plus a release-asset URL of
// the same archive.
type packageServer struct {
	*httptest.Server
	tgzBody   []byte
	tgzDigest string
}

func newPackageServer(t *testing.T) *packageServer {
	t.Helper()
	s := &packageServer{}
	s.tgzBody = buildArchive(t, [][2]string{
		{"acme/Chart.yaml", "apiVersion: v2\nname: acme\nversion: 1.2.3\nappVersion: 3.4.5\nkubeVersion: \">= 1.29\"\n"},
		{"acme/values.yaml", "replicas: 2\nimage:\n  repository: quay.io/acme/app\n  tag: 3.4.5\n"},
		{"acme/crds/crd.yaml", "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\n"},
		{"acme/templates/deploy.yaml", "image: quay.io/acme/app:3.4.5\n"},
	})
	s.tgzDigest = domain.Digest(s.tgzBody)
	idx := "apiVersion: v1\nentries:\n  acme:\n" +
		"    - name: acme\n      version: 1.2.3\n      appVersion: 3.4.5\n      digest: " +
		strings.TrimPrefix(s.tgzDigest, "sha256:") + "\n      urls:\n        - acme-1.2.3.tgz\n"
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.yaml":
			w.Write([]byte(idx))
		case "/acme-1.2.3.tgz":
			w.Header().Set("Content-Type", "application/gzip")
			w.Write(s.tgzBody)
		case "/releases/download/v3.4.5/acme-1.2.3.tgz":
			w.Write(s.tgzBody)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func newPackageAdapter() *PackageAdapter {
	return NewPackageAdapter(fetch.NewHTTPClient(nil, fetch.ModeOnline))
}

func TestPackageAdapterHelmRepo(t *testing.T) {
	srv := newPackageServer(t)
	a := newPackageAdapter()
	pkg, err := a.ReadChartPackage(context.Background(),
		catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: srv.URL, Chart: "acme"}, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Representation != domain.RepresentationChartTGZ {
		t.Errorf("Representation = %q", pkg.Representation)
	}
	if pkg.Digest != srv.tgzDigest {
		t.Errorf("Digest = %s, want the archive digest %s", pkg.Digest, srv.tgzDigest)
	}
	if pkg.ChartDir != "acme" || pkg.ChartYAML == nil || pkg.Values == nil {
		t.Errorf("members not unpacked: dir=%q chart=%v values=%v", pkg.ChartDir, pkg.ChartYAML, pkg.Values)
	}
	if len(pkg.CRDs) != 1 || pkg.CRDs[0].Path != "crds/crd.yaml" {
		t.Errorf("CRDs = %+v", pkg.CRDs)
	}
	if len(pkg.Templates) != 1 {
		t.Errorf("Templates = %+v", pkg.Templates)
	}
	// evidence: the index entry plus the archive, both with representation.
	if len(pkg.Evidence) != 2 {
		t.Fatalf("Evidence = %d records, want 2 (index entry + archive)", len(pkg.Evidence))
	}
	if pkg.Evidence[0].Kind != domain.EvidenceRegistry || pkg.Evidence[0].ContentDigest == "" {
		t.Errorf("index evidence = %+v", pkg.Evidence[0])
	}
	if pkg.Evidence[1].Representation != domain.RepresentationChartTGZ {
		t.Errorf("archive evidence representation = %q", pkg.Evidence[1].Representation)
	}
	// a semantically equal version ("v1.2.3") resolves too
	pkg2, err := a.ReadChartPackage(context.Background(),
		catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: srv.URL, Chart: "acme"}, "v1.2.3")
	if err != nil || pkg2 == nil {
		t.Fatalf("v-prefixed version: %v", err)
	}
}

func TestPackageAdapterHelmRepoMissingVersion(t *testing.T) {
	srv := newPackageServer(t)
	a := newPackageAdapter()
	_, err := a.ReadChartPackage(context.Background(),
		catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: srv.URL, Chart: "acme"}, "9.9.9")
	if !errors.Is(err, fetch.ErrNotFound) {
		t.Fatalf("error = %v, want fetch.ErrNotFound", err)
	}
}

func TestPackageAdapterDigestMismatch(t *testing.T) {
	// An index that lies about the digest: the download must be rejected.
	s := newPackageServer(t)
	idx := "apiVersion: v1\nentries:\n  acme:\n    - name: acme\n      version: 1.2.3\n" +
		"      digest: 0000000000000000000000000000000000000000000000000000000000000000\n      urls:\n        - acme-1.2.3.tgz\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.yaml":
			w.Write([]byte(idx))
		case "/acme-1.2.3.tgz":
			w.Write(s.tgzBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	a := newPackageAdapter()
	_, err := a.ReadChartPackage(context.Background(),
		catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: srv.URL, Chart: "acme"}, "1.2.3")
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("error = %v, want digest mismatch", err)
	}
}

func TestPackageAdapterChartTGZ(t *testing.T) {
	srv := newPackageServer(t)
	a := newPackageAdapter()
	loc := catalog.Locator{Kind: catalog.LocatorChartTGZ, URL: srv.URL + "/releases/download/v3.4.5/acme-1.2.3.tgz"}
	pkg, err := a.ReadChartPackage(context.Background(), loc, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	// release-asset URL shape => release-asset representation
	if pkg.Representation != domain.RepresentationReleaseAsset {
		t.Errorf("Representation = %q, want release-asset", pkg.Representation)
	}
	if pkg.Evidence[0].Kind != domain.EvidenceReleaseAsset {
		t.Errorf("evidence kind = %q", pkg.Evidence[0].Kind)
	}

	// a plain URL (no /releases/download/) is a published chart archive
	loc.URL = srv.URL + "/acme-1.2.3.tgz"
	pkg, err = a.ReadChartPackage(context.Background(), loc, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Representation != domain.RepresentationChartTGZ {
		t.Errorf("Representation = %q, want published-chart-tgz", pkg.Representation)
	}

	// the probe is the http HEAD/GET semantics
	res, err := a.Probe(context.Background(), loc, "1.2.3")
	if err != nil || !res.Exists {
		t.Fatalf("probe: exists=%v err=%v", res != nil && res.Exists, err)
	}
	loc.URL = srv.URL + "/missing.tgz"
	res, err = a.Probe(context.Background(), loc, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if res.Exists {
		t.Error("missing archive must not exist")
	}
}

func TestPackageAdapterCacheReplay(t *testing.T) {
	// Reading the same package twice is served from the fetch cache: the
	// server sees the tarball request once and the retrieval time is the
	// original one.
	srv := newPackageServer(t)
	var hits atomic.Int32
	inner := srv.Config.Handler
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/acme-1.2.3.tgz" {
			hits.Add(1)
		}
		inner.ServeHTTP(w, r)
	})
	cache := fetch.NewCache(t.TempDir())
	a := NewPackageAdapter(fetch.NewHTTPClient(cache, fetch.ModeOnline))
	loc := catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: srv.URL, Chart: "acme"}
	p1, err := a.ReadChartPackage(context.Background(), loc, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := a.ReadChartPackage(context.Background(), loc, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if n := int(hits.Load()); n != 1 {
		t.Errorf("tarball served %d times, want 1 (second read from cache)", n)
	}
	if !p1.RetrievedAt.Equal(p2.RetrievedAt) {
		t.Errorf("retrievedAt changed between reads: %v vs %v", p1.RetrievedAt, p2.RetrievedAt)
	}
}
