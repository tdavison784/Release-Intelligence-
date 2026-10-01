//go:build live

package gitsrc

import (
	"context"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/fetch"
)

// TestLiveIstioWholeDir reads the whole accumulated notes directory (no
// baseRef) to show what an unfiltered repo-dir costs.
func TestLiveIstioWholeDir(t *testing.T) {
	g, _ := liveGit(t, false)
	start := time.Now()
	docs, err := g.Dirs().FetchDirectory(context.Background(), catalog.Locator{
		Kind: catalog.LocatorRepoDir, Repository: "github.com/istio/istio", Ref: "1.31.1",
		Path: "releasenotes/notes", Glob: "*.yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	var bytes int
	for _, d := range docs {
		bytes += len(d.Content)
	}
	t.Logf("istio 1.31.1 releasenotes/notes (no baseRef): %d files, %d KiB in %s", len(docs), bytes/1024, time.Since(start))
}

// TestLiveErrors prints how failures of the real hosts are classified.
func TestLiveErrors(t *testing.T) {
	g, _ := liveGit(t, false)
	ctx := context.Background()
	for _, repo := range []string{
		"github.com/this-org-does-not-exist-0xdead/none",
		"bitbucket.org/atlassian/does-not-exist",
		"gitlab.com/does-not-exist-0xdead/none",
		"example.invalid/o/r",
	} {
		_, err := g.Tags().ListReleases(ctx, catalog.Locator{Kind: catalog.LocatorGitTags, Repository: repo})
		t.Logf("%-55s state=%-12s err=%v", repo, fetch.StateFor(err), err)
	}
	_, err := g.Log().FetchDocument(ctx, catalog.Locator{Kind: catalog.LocatorGitLog, Repository: "github.com/argoproj/argo-cd", Ref: "v3.0.0..v99.0.0"})
	t.Logf("missing tag: state=%s err=%v", fetch.StateFor(err), err)
	_, err = g.Dirs().FetchDirectory(ctx, catalog.Locator{Kind: catalog.LocatorRepoDir, Repository: "github.com/istio/istio", Ref: "99.0.0", Path: "releasenotes/notes"})
	t.Logf("missing ref: state=%s err=%v", fetch.StateFor(err), err)
	_, err = g.Files().FetchDocument(ctx, catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: "github.com/istio/istio", Ref: "1.31.1", Path: "no/such/file.md"})
	t.Logf("missing file: state=%s err=%v", fetch.StateFor(err), err)
}
