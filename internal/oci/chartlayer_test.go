package oci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// chartTgz builds a minimal packaged chart archive (deterministic bytes).
func chartTgz(t *testing.T) []byte {
	t.Helper()
	files := [][2]string{
		{"acme/Chart.yaml", "apiVersion: v2\nname: acme\nversion: 1.2.3\nappVersion: 3.4.5\n"},
		{"acme/values.yaml", "replicas: 1\n"},
		{"acme/crds/crd.yaml", "kind: CustomResourceDefinition\n"},
	}
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: f[0], Mode: 0o644, Size: int64(len(f[1])), Typeflag: tar.TypeReg,
			ModTime: time.Unix(0, 0).UTC(), Format: tar.FormatPAX,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	zw.ModTime = time.Unix(0, 0).UTC()
	if _, err := zw.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestOCIChartPackage(t *testing.T) {
	r := newFakeRegistry(t)
	a := New(fetch.NewHTTPClient(nil, fetch.ModeOnline), WithEndpoint(r.host(), r.srv.URL))

	body := chartTgz(t)
	layerDigest := r.addBlob("charts/acme", body)
	configDigest := r.addBlob("charts/acme", []byte(`{"name":"acme","version":"1.2.3","appVersion":"3.4.5"}`))
	manifest := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json",
		"config":{"mediaType":"%s","digest":"%s","size":40},
		"layers":[{"mediaType":"%s","digest":"%s","size":%d}]}`,
		HelmConfigMediaType, configDigest, HelmChartLayerMediaType, layerDigest, len(body))
	r.addManifest("charts/acme", "1.2.3", "application/vnd.oci.image.manifest.v1+json", manifest)

	pkg, err := a.ReadChartPackage(context.Background(),
		catalog.Locator{Kind: catalog.LocatorOCI, Repository: r.host() + "/charts/acme"}, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Representation != domain.RepresentationOCIChart {
		t.Errorf("Representation = %q", pkg.Representation)
	}
	if pkg.Digest != layerDigest {
		t.Errorf("Digest = %s, want layer digest %s", pkg.Digest, layerDigest)
	}
	if pkg.ChartDir != "acme" || pkg.Values == nil || len(pkg.CRDs) != 1 {
		t.Errorf("members: dir=%q values=%v crds=%d", pkg.ChartDir, pkg.Values, len(pkg.CRDs))
	}
	for _, ev := range pkg.Evidence {
		if ev.Representation != domain.RepresentationOCIChart {
			t.Errorf("evidence %s representation = %q", ev.Locator, ev.Representation)
		}
	}
}

func TestOCIChartPackageNotAChart(t *testing.T) {
	r := newFakeRegistry(t)
	a := New(fetch.NewHTTPClient(nil, fetch.ModeOnline), WithEndpoint(r.host(), r.srv.URL))

	configDigest := r.addBlob("app", []byte(`{"architecture":"amd64"}`))
	manifest := fmt.Sprintf(`{"schemaVersion":2,"config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"%s","size":20},"layers":[]}`,
		configDigest)
	r.addManifest("app", "v1.0.0", "application/vnd.oci.image.manifest.v1+json", manifest)

	_, err := a.ReadChartPackage(context.Background(),
		catalog.Locator{Kind: catalog.LocatorOCI, Repository: r.host() + "/app"}, "v1.0.0")
	if !errors.Is(err, ErrNotHelmChart) {
		t.Fatalf("error = %v, want ErrNotHelmChart", err)
	}
}

func TestOCIImageConfig(t *testing.T) {
	r := newFakeRegistry(t)
	r.RequireAuth = true // exercise the token path too
	a := New(fetch.NewHTTPClient(nil, fetch.ModeOnline), WithEndpoint(r.host(), r.srv.URL))

	configBody := `{"architecture":"amd64","os":"linux","created":"2026-09-01T10:00:00Z",
		"config":{"Labels":{"org.opencontainers.image.version":"1.18.0"},"Env":["LANG=C"]}}`
	configDigest := r.addBlob("jetstack/app", []byte(configBody))
	manifest := fmt.Sprintf(`{"schemaVersion":2,"config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"%s","size":%d},"layers":[]}`,
		configDigest, len(configBody))
	r.addManifest("jetstack/app", "v1.18.0", "application/vnd.oci.image.manifest.v1+json", manifest)

	img, err := a.ReadImage(context.Background(),
		catalog.Locator{Kind: catalog.LocatorOCI, Repository: r.host() + "/jetstack/app"}, "v1.18.0")
	if err != nil {
		t.Fatal(err)
	}
	if img.Digest == "" || !strings.HasPrefix(img.Digest, "sha256:") {
		t.Errorf("Digest = %q", img.Digest)
	}
	if img.Labels["org.opencontainers.image.version"] != "1.18.0" {
		t.Errorf("Labels = %v", img.Labels)
	}
	if len(img.Evidence) != 2 {
		t.Fatalf("Evidence = %d records", len(img.Evidence))
	}
	for _, ev := range img.Evidence {
		if ev.Representation != domain.RepresentationRegistryManifest {
			t.Errorf("evidence representation = %q", ev.Representation)
		}
	}

	ic, err := a.ImageConfig(context.Background(), r.host()+"/jetstack/app", "v1.18.0")
	if err != nil {
		t.Fatal(err)
	}
	if ic.Architecture != "amd64" || ic.OS != "linux" || len(ic.Env) != 1 {
		t.Errorf("ImageConfig = %+v", ic)
	}
}

func TestOCIImageConfigDigestMismatch(t *testing.T) {
	r := newFakeRegistry(t)
	a := New(fetch.NewHTTPClient(nil, fetch.ModeOnline), WithEndpoint(r.host(), r.srv.URL))

	configDigest := r.addBlob("app", []byte(`{"architecture":"amd64"}`))
	manifest := fmt.Sprintf(`{"schemaVersion":2,"config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"%s","size":20},"layers":[]}`,
		configDigest)
	// replace the blob behind the manifest's back: the digest check must fire
	r.mu.Lock()
	r.blobs["app@"+configDigest] = []byte(`{"architecture":"arm64"}`)
	r.mu.Unlock()
	r.addManifest("app", "v2.0.0", "application/vnd.oci.image.manifest.v1+json", manifest)

	_, err := a.ImageConfig(context.Background(), r.host()+"/app", "v2.0.0")
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("error = %v, want digest mismatch", err)
	}
}

var _ sources.ChartPackageReader = (*Adapter)(nil)
var _ sources.ImageManifestReader = (*Adapter)(nil)

func TestOCIImageConfigManifestList(t *testing.T) {
	r := newFakeRegistry(t)
	a := New(fetch.NewHTTPClient(nil, fetch.ModeOnline), WithEndpoint(r.host(), r.srv.URL))

	amd := r.addBlob("multi/app", []byte(`{"architecture":"amd64","os":"linux","config":{"Labels":{"v":"amd"}}}`))
	arm := r.addBlob("multi/app", []byte(`{"architecture":"arm64","os":"linux","config":{"Labels":{"v":"arm"}}}`))
	amdManifest := fmt.Sprintf(`{"schemaVersion":2,"config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"%s","size":10},"layers":[]}`, amd)
	armManifest := fmt.Sprintf(`{"schemaVersion":2,"config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"%s","size":10},"layers":[]}`, arm)
	// register each platform manifest under its config digest key
	r.addManifest("multi/app", amd, "application/vnd.oci.image.manifest.v1+json", amdManifest)
	r.addManifest("multi/app", arm, "application/vnd.oci.image.manifest.v1+json", armManifest)
	index := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[
		{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"%s","size":10,"platform":{"architecture":"arm64","os":"linux"}},
		{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"%s","size":10,"platform":{"architecture":"amd64","os":"linux"}}]}`,
		arm, amd)
	r.addManifest("multi/app", "v2.0.0", "application/vnd.oci.image.index.v1+json", index)

	ic, err := a.ImageConfig(context.Background(), r.host()+"/multi/app", "v2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	// linux/amd64 selected deterministically even though arm64 is listed first
	if ic.Architecture != "amd64" || ic.Labels["v"] != "amd" {
		t.Fatalf("platform selection: %+v", ic)
	}
	if !strings.Contains(ic.Manifest.Evidence.Excerpt, "selected platform manifest") {
		t.Errorf("evidence excerpt: %q", ic.Manifest.Evidence.Excerpt)
	}
}
