//go:build live

package helm

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// Manual published-artifact smoke checks against real chart repositories:
//
//	go test -tags live -run LivePackage -v ./internal/helm/
//
// Excluded from the normal test run (no network in unit tests).
func TestLivePackage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cache := fetch.NewCache(t.TempDir())
	a := NewPackageAdapter(fetch.NewHTTPClient(cache, fetch.ModeOnline))

	// kube-prometheus-stack: the shared gh-pages index (verified reachable
	// from this sandbox) and the raw.githubusercontent.com mirror of it.
	for _, repo := range []string{
		"https://prometheus-community.github.io/helm-charts",
		"https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages",
	} {
		idx, err := a.indexes.get(ctx, repo)
		if err != nil {
			t.Logf("INDEX %s -> error: %v (unavailable=%v)", repo, err, errors.Is(err, fetch.ErrUnavailable))
			continue
		}
		entries := idx.Entries["kube-prometheus-stack"]
		if len(entries) == 0 {
			t.Logf("INDEX %s -> no kube-prometheus-stack entries", repo)
			continue
		}
		e := entries[0]
		t.Logf("INDEX %s -> %d entries, newest kube-prometheus-stack %s (appVersion %s, digest %s, url %s)",
			repo, len(entries), e.Version, e.AppVersion, normalizeDigest(e.Digest), idx.ChartURL(e))

		pkg, err := a.ReadChartPackage(ctx, catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: repo, Chart: "kube-prometheus-stack"}, e.Version)
		if err != nil {
			t.Errorf("PACKAGE %s kube-prometheus-stack %s -> error: %v", repo, e.Version, err)
			continue
		}
		meta, _ := ParseChartYAML(pkg.ChartYAML)
		nCRDFiles := len(pkg.CRDs)
		nCRDBytes := 0
		for _, f := range pkg.CRDs {
			nCRDBytes += len(f.Content)
		}
		fmt.Fprintf(&logWriter{t}, "PACKAGE %s kube-prometheus-stack %s: representation=%s digest=%s\n"+
			"  Chart.yaml: name=%s version=%s appVersion=%s (kubeVersion set: %v)\n"+
			"  values.yaml: %d bytes; crds/**: %d files (%d bytes), first=%s; templates: %d files\n",
			repo, e.Version, pkg.Representation, pkg.Digest,
			meta.Name, meta.Version, meta.AppVersion, meta.KubeVersion != "",
			len(pkg.Values), nCRDFiles, nCRDBytes, firstPath(pkg.CRDs), len(pkg.Templates))
		for _, ev := range pkg.Evidence {
			t.Logf("  evidence %s kind=%s rep=%s digest=%s: %s", ev.ID, ev.Kind, ev.Representation, ev.ContentDigest, ev.Excerpt)
		}
		// The prometheus-community index entry URLs point at GitHub release
		// assets ("/releases/download/..."), so the honest representation of
		// these packages is release-asset; other repositories that host the
		// tarballs themselves yield published-chart-tgz. Both are valid.
		if pkg.Representation != domain.RepresentationChartTGZ && pkg.Representation != domain.RepresentationReleaseAsset {
			t.Errorf("representation = %q, want published-chart-tgz or release-asset", pkg.Representation)
		}
		if pkg.Digest != normalizeDigest(e.Digest) {
			t.Errorf("archive digest %s does not match the index digest %s", pkg.Digest, normalizeDigest(e.Digest))
		}
	}
}

type logWriter struct{ t *testing.T }

func (w *logWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", p)
	return len(p), nil
}

func firstPath(fs []sources.PackageFile) string {
	if len(fs) == 0 {
		return "-"
	}
	return fs[0].Path
}
