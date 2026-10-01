//go:build live

package helm

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/fetch"
)

// Manual sanity checks against the real indexes:
//
//	go test -tags live -run Live -v ./internal/helm/
//
// They are excluded from the normal test run (no network in unit tests).
func TestLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	a := NewRepoAdapter(fetch.NewHTTPClient(fetch.NewCache(t.TempDir()), fetch.ModeOnline))

	start := time.Now()
	vs, err := a.ListArtifactVersions(ctx, repoLoc("https://istio-release.storage.googleapis.com/charts", "istiod"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("istio istiod: %d versions (first fetch+parse %v)", len(vs), time.Since(start))
	for _, v := range vs {
		if strings.HasPrefix(v.Version, "1.26.") {
			t.Logf("  istiod %-16s appVersion=%-16s created=%s", v.Version, v.Fields["appVersion"], v.Fields["created"])
		}
	}
	res, err := a.Probe(ctx, repoLoc("https://istio-release.storage.googleapis.com/charts", "istiod"), "1.26.0")
	t.Logf("istio probe 1.26.0: %+v err=%v", res, err)

	start = time.Now()
	argoURL := "https://raw.githubusercontent.com/argoproj/argo-helm/gh-pages"
	av, err := a.ListArtifactVersions(ctx, repoLoc(argoURL, "argo-cd"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("argo argo-cd: %d versions (first fetch+parse %v)", len(av), time.Since(start))
	var match []string
	for _, v := range av {
		if v.Fields["appVersion"] == "v3.0.0" {
			match = append(match, v.Version)
		}
	}
	sort.Strings(match)
	t.Logf("argo-cd chart versions with appVersion v3.0.0: %v", match)

	start = time.Now()
	for i := 0; i < 1000; i++ {
		if _, err := a.Probe(ctx, repoLoc(argoURL, "argo-cd"), "10.9.5"); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("1000 probes against the memoized index: %v", time.Since(start))
	res, err = a.Probe(ctx, repoLoc(argoURL, "argo-cd"), "10.9.5")
	t.Logf("argo probe 10.9.5: %+v err=%v", res, err)
}
