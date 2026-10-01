//go:build live

package oci

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/fetch"
)

// Manual published-artifact smoke checks against real registries:
//
//	go test -tags live -run LiveChartLayer -v ./internal/oci/
func TestLiveChartLayer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cache := fetch.NewCache(t.TempDir())
	a := New(fetch.NewHTTPClient(cache, fetch.ModeOnline))

	// Helm charts published as OCI artifacts. The chart layer is a blob
	// download; registries that redirect blobs to a blocked host fail with
	// fetch.ErrUnavailable (reported honestly, not as "missing").
	for _, tc := range []struct{ repo, tag string }{
		{"ghcr.io/prometheus-community/charts/kube-prometheus-stack", "91.8.2"},
		{"ghcr.io/istio/release/charts/istiod", "1.26.0"},
	} {
		pkg, err := a.ReadChartPackage(ctx, catalog.Locator{Kind: catalog.LocatorOCI, Repository: tc.repo}, tc.tag)
		switch {
		case err != nil:
			t.Logf("CHART LAYER %s:%s -> error (unavailable=%v notfound=%v notahelmchart=%v): %v",
				tc.repo, tc.tag, errors.Is(err, fetch.ErrUnavailable), errors.Is(err, fetch.ErrNotFound), errors.Is(err, ErrNotHelmChart), err)
		default:
			t.Logf("CHART LAYER %s:%s -> representation=%s digest=%s chartDir=%s values=%dB crds=%d templates=%d evidence=%d",
				tc.repo, tc.tag, pkg.Representation, pkg.Digest, pkg.ChartDir, len(pkg.Values), len(pkg.CRDs), len(pkg.Templates), len(pkg.Evidence))
		}
	}

	// Container image configs: labels and digests as registry evidence.
	for _, tc := range []struct{ repo, tag string }{
		{"quay.io/prometheus/prometheus", "v3.15.0"},
		{"quay.io/prometheus-operator/prometheus-operator", "v0.94.1"},
	} {
		ic, err := a.ImageConfig(ctx, tc.repo, tc.tag)
		switch {
		case err != nil:
			t.Logf("IMAGE CONFIG %s:%s -> error (unavailable=%v): %v", tc.repo, tc.tag, errors.Is(err, fetch.ErrUnavailable), err)
		default:
			ver := ic.Labels["org.opencontainers.image.version"]
			t.Logf("IMAGE CONFIG %s:%s -> digest=%s arch=%s/%s labels=%d (version=%q) config=%s",
				tc.repo, tc.tag, ic.Manifest.Digest, ic.Architecture, ic.OS, len(ic.Labels), ver, ic.ConfigDigest)
		}
	}
}
