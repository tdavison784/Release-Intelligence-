package gitsrc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/fetch"
)

func TestFetchFile(t *testing.T) {
	sha := strings.Repeat("c0", 20)
	tests := []struct {
		name          string
		loc           catalog.Locator
		wantURL       string
		wantImmutable bool
		wantURI       string
		wantFormat    string
	}{
		{
			name: "github branch is mutable",
			loc: catalog.Locator{Kind: "repo-file", Repository: "github.com/cert-manager/website", Ref: "master",
				Path: "content/docs/releases/release-notes/release-notes-1.18.md"},
			wantURL:       "https://raw.githubusercontent.com/cert-manager/website/master/content/docs/releases/release-notes/release-notes-1.18.md",
			wantImmutable: false,
			wantURI:       "https://github.com/cert-manager/website/blob/master/content/docs/releases/release-notes/release-notes-1.18.md",
			wantFormat:    "markdown",
		},
		{
			name: "github version tag is immutable",
			loc: catalog.Locator{Kind: "repo-file", Repository: "argoproj/argo-cd", Ref: "v3.0.0",
				Path: "docs/operator-manual/upgrading/2.14-3.0.md"},
			wantURL:       "https://raw.githubusercontent.com/argoproj/argo-cd/v3.0.0/docs/operator-manual/upgrading/2.14-3.0.md",
			wantImmutable: true,
			wantURI:       "https://github.com/argoproj/argo-cd/blob/v3.0.0/docs/operator-manual/upgrading/2.14-3.0.md",
			wantFormat:    "markdown",
		},
		{
			name: "bare version tag",
			loc: catalog.Locator{Kind: "repo-file", Repository: "github.com/istio/istio", Ref: "1.26.0",
				Path: "manifests/charts/base/Chart.yaml"},
			wantURL:       "https://raw.githubusercontent.com/istio/istio/1.26.0/manifests/charts/base/Chart.yaml",
			wantImmutable: true,
			wantURI:       "https://github.com/istio/istio/blob/1.26.0/manifests/charts/base/Chart.yaml",
			wantFormat:    "yaml",
		},
		{
			name: "chart tag with prefix",
			loc: catalog.Locator{Kind: "repo-file", Repository: "github.com/argoproj/argo-helm", Ref: "argo-cd-8.0.0",
				Path: "charts/argo-cd/values.yaml"},
			wantURL:       "https://raw.githubusercontent.com/argoproj/argo-helm/argo-cd-8.0.0/charts/argo-cd/values.yaml",
			wantImmutable: true,
			wantURI:       "https://github.com/argoproj/argo-helm/blob/argo-cd-8.0.0/charts/argo-cd/values.yaml",
			wantFormat:    "yaml",
		},
		{
			name: "release branch is mutable even though it contains digits and a dot",
			loc: catalog.Locator{Kind: "repo-file", Repository: "github.com/istio/istio.io", Ref: "release-1.31",
				Path: "data/compatibility/supportStatus.yml"},
			wantURL:       "https://raw.githubusercontent.com/istio/istio.io/release-1.31/data/compatibility/supportStatus.yml",
			wantImmutable: false,
			wantURI:       "https://github.com/istio/istio.io/blob/release-1.31/data/compatibility/supportStatus.yml",
			wantFormat:    "yaml",
		},
		{
			name: "commit sha is immutable",
			loc: catalog.Locator{Kind: "repo-file", Repository: "github.com/o/r", Ref: sha,
				Path: "data.json"},
			wantURL:       "https://raw.githubusercontent.com/o/r/" + sha + "/data.json",
			wantImmutable: true,
			wantURI:       "https://github.com/o/r/blob/" + sha + "/data.json",
			wantFormat:    "json",
		},
		{
			name: "missing ref defaults to HEAD and is mutable",
			loc: catalog.Locator{Kind: "repo-file", Repository: "github.com/o/r",
				Path: "LICENSE"},
			wantURL:       "https://raw.githubusercontent.com/o/r/HEAD/LICENSE",
			wantImmutable: false,
			wantURI:       "https://github.com/o/r/blob/HEAD/LICENSE",
			wantFormat:    "text",
		},
		{
			name: "gitlab",
			loc: catalog.Locator{Kind: "repo-file", Repository: "gitlab.com/group/sub/proj", Ref: "v2.1.0",
				Path: "CHANGELOG.md"},
			wantURL:       "https://gitlab.com/group/sub/proj/-/raw/v2.1.0/CHANGELOG.md",
			wantImmutable: true,
			wantURI:       "https://gitlab.com/group/sub/proj/-/blob/v2.1.0/CHANGELOG.md",
			wantFormat:    "markdown",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fc := &recordingFetch{docs: map[string]string{tc.wantURL: "# body\n"}}
			g := newTestGit(t, t.TempDir(), failRunner(t), fc, nil, Options{})
			doc, err := g.Files().FetchDocument(context.Background(), tc.loc)
			if err != nil {
				t.Fatal(err)
			}
			if len(fc.reqs) != 1 || fc.reqs[0].URL != tc.wantURL || fc.reqs[0].Immutable != tc.wantImmutable {
				t.Fatalf("requests = %+v, want one GET %s immutable=%v", fc.reqs, tc.wantURL, tc.wantImmutable)
			}
			if doc.URI != tc.wantURI || doc.FetchURL != tc.wantURL || doc.Format != tc.wantFormat ||
				string(doc.Content) != "# body\n" || doc.Locator != tc.loc || doc.Digest == "" || doc.RetrievedAt.IsZero() {
				t.Errorf("doc = %+v", doc)
			}
			if doc.Path != strings.Trim(tc.loc.Path, "/") {
				t.Errorf("doc.Path = %q", doc.Path)
			}
		})
	}
}

func TestFetchFileUsesConfiguredTTL(t *testing.T) {
	fc := &recordingFetch{docs: map[string]string{"https://raw.githubusercontent.com/o/r/main/a.md": "x"}}
	g := newTestGit(t, t.TempDir(), failRunner(t), fc, nil, Options{TTL: 90 * time.Minute})
	_, err := g.Files().FetchDocument(context.Background(), catalog.Locator{Repository: "o/r", Ref: "main", Path: "a.md"})
	if err != nil {
		t.Fatal(err)
	}
	if fc.reqs[0].TTL != 90*time.Minute {
		t.Errorf("request TTL = %v", fc.reqs[0].TTL)
	}
}

func TestFetchFileErrors(t *testing.T) {
	ctx := context.Background()
	fc := &recordingFetch{}
	g := newTestGit(t, t.TempDir(), failRunner(t), fc, nil, Options{})

	// Unsupported host: no raw endpoint known; never reaches the network.
	_, err := g.Files().FetchDocument(ctx, catalog.Locator{Kind: "repo-file", Repository: "git.example.org/team/tool", Ref: "v1.0.0", Path: "a.md"})
	if !errors.Is(err, fetch.ErrUnavailable) || !strings.Contains(err.Error(), "unsupported host") {
		t.Errorf("unsupported host: %v", err)
	}
	if len(fc.reqs) != 0 {
		t.Errorf("unsupported host must not hit the network: %+v", fc.reqs)
	}

	// 404 from the host maps to ErrNotFound.
	_, err = g.Files().FetchDocument(ctx, catalog.Locator{Kind: "repo-file", Repository: "github.com/o/r", Ref: "v1.0.0", Path: "missing.md"})
	if !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("missing file: %v", err)
	}

	// Transport failure passes through.
	fc.err = &fetch.Error{URL: "x", Err: fetch.ErrUnavailable}
	_, err = g.Files().FetchDocument(ctx, catalog.Locator{Kind: "repo-file", Repository: "github.com/o/r", Ref: "v1.0.0", Path: "a.md"})
	if !errors.Is(err, fetch.ErrUnavailable) {
		t.Errorf("unavailable: %v", err)
	}

	for name, loc := range map[string]catalog.Locator{
		"no path":         {Repository: "github.com/o/r", Ref: "v1"},
		"path traversal":  {Repository: "github.com/o/r", Ref: "v1", Path: "../etc/passwd"},
		"bad ref":         {Repository: "github.com/o/r", Ref: "--x", Path: "a.md"},
		"bad repository":  {Repository: "nope", Ref: "v1", Path: "a.md"},
		"ref with spaces": {Repository: "github.com/o/r", Ref: "a b", Path: "a.md"},
	} {
		if _, err := g.Files().FetchDocument(ctx, loc); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
