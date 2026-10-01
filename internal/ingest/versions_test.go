package ingest

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

func semvers(vs []domain.Version) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Semver
	}
	return out
}

func stateList(sts []domain.SourceStatus) []string {
	out := make([]string, len(sts))
	for i, s := range sts {
		out[i] = s.SourceID + "=" + string(s.State)
	}
	return out
}

// gitTagsFixture is what the fallback git-tags source lists: a mono-repo
// with sub-module tags, a non-canonical duplicate, prereleases and a draft.
var gitTagsFixture = []sources.ReleaseRef{
	{Tag: "v1.1.0"},
	{Tag: "1.0.0"}, // non-canonical duplicate of v1.0.0
	{Tag: "cmd/ctl/v1.1.0"},
	{Tag: "v1.0.0", Commit: "deadbeef"},
	{Tag: "v1.2.0-alpha.0"},
	{Tag: "v1.1.1", Prerelease: true}, // flagged by the source itself
	{Tag: "v1.2.0", Draft: true},
	{Tag: "v0.9.0"},
	{Tag: "not-a-version"},
}

func TestListVersions(t *testing.T) {
	cases := []struct {
		name        string
		setup       func(w *world, def *catalog.ProductDefinition)
		wantSource  string
		wantVers    []string
		wantStates  []string
		wantErr     error
		wantTags    map[string]string // semver → tag
		checkDetail map[string]string // source → substring of detail
	}{
		{
			name:       "first source answers, fallback not consulted",
			wantSource: "gh-releases",
			wantVers:   []string{"1.0.0", "1.0.1", "1.1.0", "1.1.1", "1.2.0", "1.2.1"},
			wantStates: []string{"gh-releases=ok", "git-tags=skipped"},
			checkDetail: map[string]string{
				"gh-releases": "6 releases from 8 tags (ignored: 1 non-matching, 1 prereleases, 0 drafts, 0 duplicates)",
				"git-tags":    "not consulted: gh-releases answered",
			},
		},
		{
			name: "falls back when the first source is unavailable",
			setup: func(w *world, _ *catalog.ProductDefinition) {
				w.errs["github-releases:acme/operator"] = unavailable("api.github.com")
				w.releases["git-tags:github.com/acme/operator@:"] = gitTagsFixture
			},
			wantSource: "git-tags",
			wantVers:   []string{"0.9.0", "1.0.0", "1.1.0"},
			wantStates: []string{"gh-releases=unavailable", "git-tags=ok"},
			wantTags:   map[string]string{"1.0.0": "v1.0.0"},
			checkDetail: map[string]string{
				"git-tags": "ignored: 2 non-matching, 2 prereleases, 1 drafts, 1 duplicates",
			},
		},
		{
			name: "falls back when the first source lists no release",
			setup: func(w *world, _ *catalog.ProductDefinition) {
				w.releases["github-releases:acme/operator"] = []sources.ReleaseRef{{Tag: "chart-1.0.0"}}
				w.releases["git-tags:github.com/acme/operator@:"] = gitTagsFixture
			},
			wantSource: "git-tags",
			wantVers:   []string{"0.9.0", "1.0.0", "1.1.0"},
			wantStates: []string{"gh-releases=not-found", "git-tags=ok"},
		},
		{
			name: "prereleases kept when the definition includes them",
			setup: func(w *world, def *catalog.ProductDefinition) {
				def.Versioning.IncludePrereleases = true
				w.errs["github-releases:acme/operator"] = unavailable("api.github.com")
				w.releases["git-tags:github.com/acme/operator@:"] = gitTagsFixture
			},
			wantSource: "git-tags",
			wantVers:   []string{"0.9.0", "1.0.0", "1.1.0", "1.1.1", "1.2.0-alpha.0"},
			wantStates: []string{"gh-releases=unavailable", "git-tags=ok"},
		},
		{
			name: "no source answers",
			setup: func(w *world, _ *catalog.ProductDefinition) {
				w.down[catalog.LocatorGitHubReleases] = true
				w.down[catalog.LocatorGitTags] = true
			},
			wantStates: []string{"gh-releases=unavailable", "git-tags=unavailable"},
			wantErr:    ErrNoVersions,
		},
		{
			name: "missing adapter is skipped",
			setup: func(w *world, def *catalog.ProductDefinition) {
				def.Sources[0].Locator = catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: "https://charts.acme.example", Chart: "acme"}
				w.releases["git-tags:github.com/acme/operator@:"] = gitTagsFixture
			},
			wantSource:  "git-tags",
			wantVers:    []string{"0.9.0", "1.0.0", "1.1.0"},
			wantStates:  []string{"gh-releases=skipped", "git-tags=ok"},
			checkDetail: map[string]string{"gh-releases": "no version adapter registered"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, def := newWorld(), testDef()
			if tc.setup != nil {
				tc.setup(w, def)
			}
			ing := newTestIngester(w, &fakeParser{})
			vl, err := ing.ListVersions(context.Background(), def)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if vl == nil {
					t.Fatal("version list must carry statuses on error")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if got := stateList(vl.Sources); !reflect.DeepEqual(got, tc.wantStates) {
				t.Errorf("states = %v, want %v", got, tc.wantStates)
			}
			if tc.wantErr != nil {
				return
			}
			if vl.Source != tc.wantSource {
				t.Errorf("source = %q, want %q", vl.Source, tc.wantSource)
			}
			if got := semvers(vl.Versions); !reflect.DeepEqual(got, tc.wantVers) {
				t.Errorf("versions = %v, want %v", got, tc.wantVers)
			}
			for semver, tag := range tc.wantTags {
				if got := version(t, vl, semver).Tag; got != tag {
					t.Errorf("tag of %s = %q, want %q", semver, got, tag)
				}
				if vl.Refs[semver].Tag != tag {
					t.Errorf("ref tag of %s = %q, want %q", semver, vl.Refs[semver].Tag, tag)
				}
			}
			for src, want := range tc.checkDetail {
				for _, s := range vl.Sources {
					if s.SourceID == src && !strings.Contains(s.Detail, want) {
						t.Errorf("%s detail %q does not contain %q", src, s.Detail, want)
					}
				}
			}
			if len(vl.Evidence) != len(vl.Versions) {
				t.Errorf("want one evidence per version, got %d for %d", len(vl.Evidence), len(vl.Versions))
			}
			for _, v := range vl.Versions {
				ref, ok := vl.Refs[v.Semver]
				if !ok || ref.Evidence.ID == "" {
					t.Errorf("ref of %s missing or without evidence: %+v", v.Semver, ref)
				}
			}
		})
	}
}

func TestListVersionsKeepsAdapterEvidence(t *testing.T) {
	w := newWorld()
	ev := domain.NewEvidence(domain.EvidenceGitRef, "gh-releases", "https://github.com/acme/operator/releases/tag/v1.2.1", "tag v1.2.1", "v1.2.1", "", fetchedAt)
	refs := w.releases["github-releases:acme/operator"]
	refs[0].Evidence = ev
	ing := newTestIngester(w, &fakeParser{})
	vl := mustVersions(t, ing, testDef())
	if got := vl.Refs["1.2.1"].Evidence.ID; got != ev.ID {
		t.Fatalf("adapter evidence replaced: %s", got)
	}
	if got := vl.Refs["1.0.0"].Evidence; got.Kind != domain.EvidenceGitRef || got.Locator != "tag v1.0.0" {
		t.Fatalf("synthesised evidence: %+v", got)
	}
	if vl.Refs["1.2.1"].PublishedAt == nil || vl.Refs["1.2.1"].Commit != "abc123" {
		t.Fatalf("ref metadata lost: %+v", vl.Refs["1.2.1"])
	}
}
