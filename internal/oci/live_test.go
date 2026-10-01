//go:build live

package oci

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/fetch"
)

// Manual sanity checks against real registries:
//
//	go test -tags live -run Live -v ./internal/oci/
//
// They are excluded from the normal test run (no network in unit tests).
func TestLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cache := fetch.NewCache(t.TempDir())
	a := New(fetch.NewHTTPClient(cache, fetch.ModeOnline))

	for _, tc := range []struct{ repo, tag string }{
		{"docker.io/istio/pilot", "1.26.0"},
		{"docker.io/istio/pilot", "0.0.0-nope"},
		{"gcr.io/istio-release/pilot", "1.26.0"},
		{"ghcr.io/istio/release/charts/istiod", "1.26.0"},
		{"ghcr.io/istio/release/charts/istiod", "0.0.0-nope"},
		{"docker.io/library/busybox", "1.36"},
		{"quay.io/jetstack/cert-manager-controller", "v1.18.0"},
	} {
		res, err := a.Probe(ctx, locator(tc.repo), tc.tag)
		switch {
		case err != nil:
			t.Logf("PROBE %s:%s -> error (unavailable=%v notfound=%v): %v",
				tc.repo, tc.tag, errors.Is(err, fetch.ErrUnavailable), errors.Is(err, fetch.ErrNotFound), err)
		default:
			t.Logf("PROBE %s:%s -> exists=%v coord=%s digest=%s uri=%s\n    evidence: %s | %s | %s",
				tc.repo, tc.tag, res.Exists, res.Coordinate, res.Digest, res.URI,
				res.Evidence.Locator, res.Evidence.Excerpt, res.Evidence.ID)
		}
	}

	for _, repo := range []string{"ghcr.io/istio/release/charts/istiod", "docker.io/istio/pilot"} {
		all, err := a.ListTags(ctx, repo)
		if err != nil {
			t.Logf("TAGS %s -> error: %v", repo, err)
			continue
		}
		vs, err := a.ListArtifactVersions(ctx, locator(repo))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("TAGS %s -> %d raw, %d after noise filter", repo, len(all), len(vs))
	}

	ch, err := a.ChartConfig(ctx, "ghcr.io/istio/release/charts/istiod", "1.26.0")
	t.Logf("CHART ghcr istiod:1.26.0 -> %+v err=%v (unavailable=%v)", ch, err, errors.Is(err, fetch.ErrUnavailable))
	if m, err := a.GetManifest(ctx, "ghcr.io/istio/release/charts/istiod", "1.26.0"); err == nil {
		t.Logf("MANIFEST annotations: %v config=%s", m.Annotations, m.Config.MediaType)
	}
}
