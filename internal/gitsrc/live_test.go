//go:build live

// Manual sanity checks against the real hosts. They are excluded from normal
// test runs; execute them with
//
//	go test -tags live -run Live -v -count=1 ./internal/gitsrc
//
// They need a working git, network access to github.com and
// raw.githubusercontent.com, and take a few seconds.
package gitsrc

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/fetch"
)

func liveGit(t *testing.T, offline bool) (*Git, string) {
	t.Helper()
	dir := os.Getenv("RI_LIVE_CACHE")
	if dir == "" {
		dir = t.TempDir()
	}
	mode := fetch.ModeOnline
	if offline {
		mode = fetch.ModeOffline
	}
	hc := fetch.NewHTTPClient(fetch.NewCache(dir), mode)
	hc.HTTP = &http.Client{Timeout: 60 * time.Second}
	return New(hc, Options{CacheDir: dir, Offline: offline}), dir
}

func dirSize(t *testing.T, dir string) int64 {
	t.Helper()
	var n int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}

func TestLiveCertManager(t *testing.T) {
	g, _ := liveGit(t, false)
	ctx := context.Background()

	start := time.Now()
	refs, err := g.Tags().ListReleases(ctx, catalog.Locator{
		Kind: catalog.LocatorGitTags, Repository: "github.com/cert-manager/cert-manager",
		TagPattern: `^v(?P<version>\d+\.\d+\.\d+)$`,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cert-manager: %d stable tags in %s; last=%s %s", len(refs), time.Since(start), refs[len(refs)-1].Tag, refs[len(refs)-1].URL)

	doc, err := g.Files().FetchDocument(ctx, catalog.Locator{
		Kind: catalog.LocatorRepoFile, Repository: "github.com/cert-manager/website", Ref: "master",
		Path: "content/docs/releases/release-notes/release-notes-1.18.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("release-notes-1.18.md: %d bytes, %s, uri=%s", len(doc.Content), doc.Format, doc.URI)
}

func TestLiveIstioDir(t *testing.T) {
	g, dir := liveGit(t, false)
	ctx := context.Background()
	loc := catalog.Locator{
		Kind: catalog.LocatorRepoDir, Repository: "github.com/istio/istio",
		Ref: "1.31.1", BaseRef: "1.31.0", Path: "releasenotes/notes", Glob: "*.yaml",
	}
	start := time.Now()
	docs, err := g.Dirs().FetchDirectory(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("istio notes added in 1.31.1: %d files in %s; work repo %d KiB", len(docs), time.Since(start), dirSize(t, filepath.Join(dir, "git", "work"))/1024)
	if len(docs) > 0 {
		t.Logf("first: %s (%d bytes) %s", docs[0].Path, len(docs[0].Content), docs[0].URI)
	}

	// Second call is served from the result cache.
	start = time.Now()
	if _, err := g.Dirs().FetchDirectory(ctx, loc); err != nil {
		t.Fatal(err)
	}
	t.Logf("cached repeat: %s", time.Since(start))

	// Offline replay through a fresh adapter.
	offline := New(g.fetch, Options{CacheDir: dir, Offline: true})
	again, err := offline.Dirs().FetchDirectory(ctx, loc)
	if err != nil || len(again) != len(docs) {
		t.Fatalf("offline replay: %v (%d files)", err, len(again))
	}
}

func TestLiveArgoLog(t *testing.T) {
	g, dir := liveGit(t, false)
	ctx := context.Background()
	loc := catalog.Locator{Kind: catalog.LocatorGitLog, Repository: "github.com/argoproj/argo-cd", Ref: "v3.0.0..v3.1.0"}
	start := time.Now()
	doc, err := g.Log().FetchDocument(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}
	bullets := strings.Count(string(doc.Content), "\n- ")
	t.Logf("argo-cd v3.0.0..v3.1.0: %d commits in %s (first run, includes mirror clone); mirror %d KiB; uri=%s",
		bullets, time.Since(start), dirSize(t, filepath.Join(dir, "git", "mirrors"))/1024, doc.URI)
	lines := strings.Split(string(doc.Content), "\n")
	for _, l := range lines[:min(6, len(lines))] {
		t.Log(l)
	}

	start = time.Now()
	loc.Ref = "v3.1.0..v3.2.0"
	doc2, err := g.Log().FetchDocument(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("argo-cd v3.1.0..v3.2.0: %d commits in %s (mirror reuse)", strings.Count(string(doc2.Content), "\n- "), time.Since(start))

	offline := New(g.fetch, Options{CacheDir: dir, Offline: true})
	loc.Ref = "v3.0.0..v3.1.0"
	again, err := offline.Log().FetchDocument(ctx, loc)
	if err != nil || again.Digest != doc.Digest {
		t.Fatalf("offline replay: %v", err)
	}
}
