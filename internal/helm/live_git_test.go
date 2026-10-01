//go:build live

package helm

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// lsRemoteTags is a throwaway git-tags lister (`git ls-remote`) so that the
// helm-git adapter can be exercised live without the real gitsrc package.
type lsRemoteTags struct{}

func (lsRemoteTags) ListReleases(ctx context.Context, loc catalog.Locator) ([]sources.ReleaseRef, error) {
	out, err := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "https://"+loc.Repository).Output()
	if err != nil {
		return nil, &fetch.Error{URL: loc.Repository, Err: fetch.ErrUnavailable, Detail: err.Error()}
	}
	var refs []sources.ReleaseRef
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || strings.HasSuffix(f[1], "^{}") {
			continue
		}
		tag := strings.TrimPrefix(f[1], "refs/tags/")
		refs = append(refs, sources.ReleaseRef{Tag: tag, Commit: f[0]})
	}
	return refs, nil
}

// rawFiles is a throwaway repo-file fetcher over raw.githubusercontent.com.
type rawFiles struct{ f fetch.Client }

func (r rawFiles) FetchDocument(ctx context.Context, loc catalog.Locator) (*sources.Document, error) {
	repo := strings.TrimPrefix(loc.Repository, "github.com/")
	u := "https://raw.githubusercontent.com/" + repo + "/" + loc.Ref + "/" + loc.Path
	d, err := fetch.Get(ctx, r.f, u, true)
	if err != nil {
		return nil, err
	}
	return &sources.Document{Locator: loc, URI: "https://github.com/" + repo + "/blob/" + loc.Ref + "/" + loc.Path,
		FetchURL: u, Content: d.Body, Format: "yaml", Digest: d.Digest, RetrievedAt: d.RetrievedAt}, nil
}

func TestLiveHelmGit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	f := fetch.NewHTTPClient(fetch.NewCache(t.TempDir()), fetch.ModeOnline)
	g := NewGitAdapter(lsRemoteTags{}, rawFiles{f}, WithMaxTags(40))
	loc := catalog.Locator{Kind: catalog.LocatorHelmGit, Repository: "github.com/argoproj/argo-helm",
		Path: "charts/argo-cd", TagPattern: `^argo-cd-(?P<version>.+)$`}

	start := time.Now()
	vs, err := g.ListArtifactVersions(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}
	inspected := 0
	for _, v := range vs {
		if v.Fields["appVersion"] != "" {
			inspected++
		}
	}
	t.Logf("helm-git argo-cd: %d tags, %d inspected in %v; newest: %s appVersion=%s kubeVersion=%s",
		len(vs), inspected, time.Since(start), vs[0].Version, vs[0].Fields["appVersion"], vs[0].Fields["kubeVersion"])
	res, err := g.Probe(ctx, loc, "8.0.1")
	t.Logf("helm-git probe 8.0.1: exists=%v coord=%s uri=%s excerpt=%q err=%v", res.Exists, res.Coordinate, res.URI, res.Evidence.Excerpt, err)
	res, err = g.Probe(ctx, loc, "99.0.0")
	t.Logf("helm-git probe 99.0.0: exists=%v coord=%s err=%v", res.Exists, res.Coordinate, err)
}
