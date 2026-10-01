package gitsrc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// lsRemoteFixture mimics "git ls-remote --tags" of a mono-repo: annotated
// tags (object + peeled commit), lightweight tags, tags of other components
// and a non-release tag.
const lsRemoteFixture = "" +
	"1111111111111111111111111111111111111111\trefs/tags/cmd/ctl/v1.12.0\n" +
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\trefs/tags/stable\n" +
	"2222222222222222222222222222222222222222\trefs/tags/v1.17.0\n" +
	"3333333333333333333333333333333333333333\trefs/tags/v1.17.0^{}\n" +
	"4444444444444444444444444444444444444444\trefs/tags/v1.18.0\n" +
	"5555555555555555555555555555555555555555\trefs/tags/v1.18.0-alpha.0\n" +
	"6666666666666666666666666666666666666666\trefs/tags/v1.18.0-alpha.0^{}\n" +
	"7777777777777777777777777777777777777777\trefs/tags/v1.18.1\n" +
	"8888888888888888888888888888888888888888\trefs/tags/v1.18.1^{}\n"

func TestParseLsRemote(t *testing.T) {
	tags, err := parseLsRemote([]byte(lsRemoteFixture))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	var order []string
	for _, tg := range tags {
		got[tg.Name] = tg.Commit
		order = append(order, tg.Name)
	}
	want := map[string]string{
		"cmd/ctl/v1.12.0": strings.Repeat("1", 40), // lightweight: the ref's own object
		"stable":          strings.Repeat("a", 40),
		"v1.17.0":         strings.Repeat("3", 40), // annotated: peeled commit, not the tag object
		"v1.18.0":         strings.Repeat("4", 40),
		"v1.18.0-alpha.0": strings.Repeat("6", 40),
		"v1.18.1":         strings.Repeat("8", 40),
	}
	if len(got) != len(want) {
		t.Fatalf("got %d tags %v, want %d", len(got), got, len(want))
	}
	for name, sha := range want {
		if got[name] != sha {
			t.Errorf("tag %s: commit %q, want %q", name, got[name], sha)
		}
	}
	if !sortedStrings(order) {
		t.Errorf("tags are not sorted by name: %v", order)
	}
}

func sortedStrings(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i-1] > s[i] {
			return false
		}
	}
	return true
}

func TestParseLsRemoteEdgeCases(t *testing.T) {
	// A peeled line that precedes its tag line must still win.
	out := "bb\trefs/tags/v1^{}\naa\trefs/tags/v1\n"
	tags, err := parseLsRemote([]byte(out))
	if err != nil || len(tags) != 1 || tags[0].Commit != "bb" {
		t.Fatalf("got %+v, %v", tags, err)
	}
	// Branches and other refs are ignored.
	tags, err = parseLsRemote([]byte("aa\trefs/heads/main\nbb\trefs/pull/1/head\ncc\trefs/tags/v2\n"))
	if err != nil || len(tags) != 1 || tags[0].Name != "v2" {
		t.Fatalf("got %+v, %v", tags, err)
	}
	// Empty output is a repository without tags.
	if tags, err := parseLsRemote(nil); err != nil || len(tags) != 0 {
		t.Fatalf("empty: %+v %v", tags, err)
	}
	if _, err := parseLsRemote([]byte("garbage without tab\n")); err == nil {
		t.Fatal("want error for malformed line")
	}
}

func lsRemoteRunner(t *testing.T, out string) *fakeRunner {
	return &fakeRunner{fn: func(c Command) ([]byte, error) {
		if subcommand(c.Args) != "ls-remote" {
			t.Errorf("unexpected git command %v", c.Args)
		}
		return []byte(out), nil
	}}
}

func tagLocator(repo, pattern string) catalog.Locator {
	return catalog.Locator{Kind: catalog.LocatorGitTags, Repository: repo, TagPattern: pattern}
}

func TestListTagsPeelsFiltersAndAttachesEvidence(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		pattern string
		want    []string
	}{
		{"no pattern lists every tag", "", []string{"cmd/ctl/v1.12.0", "stable", "v1.17.0", "v1.18.0", "v1.18.0-alpha.0", "v1.18.1"}},
		{"stable semver only", `^v(?P<version>\d+\.\d+\.\d+)$`, []string{"v1.17.0", "v1.18.0", "v1.18.1"}},
		{"with prerelease", `^v(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$`, []string{"v1.17.0", "v1.18.0", "v1.18.0-alpha.0", "v1.18.1"}},
		{"sub-module tags", `^cmd/ctl/v(?P<version>\d+\.\d+\.\d+)$`, []string{"cmd/ctl/v1.12.0"}},
		{"matches nothing", `^nothing$`, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGit(t, t.TempDir(), lsRemoteRunner(t, lsRemoteFixture), nil, newClock(), Options{})
			refs, err := g.Tags().ListReleases(ctx, tagLocator("github.com/cert-manager/cert-manager", tc.pattern))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range refs {
				got = append(got, r.Tag)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("tags = %v, want %v", got, tc.want)
			}
		})
	}

	g := newTestGit(t, t.TempDir(), lsRemoteRunner(t, lsRemoteFixture), nil, newClock(), Options{})
	refs, err := g.Tags().ListReleases(ctx, tagLocator("cert-manager/cert-manager", `^v(?P<version>\d+\.\d+\.\d+)$`))
	if err != nil {
		t.Fatal(err)
	}
	r := refs[0] // v1.17.0, annotated
	if r.Tag != "v1.17.0" || r.Commit != strings.Repeat("3", 40) {
		t.Fatalf("ref = %+v", r)
	}
	if r.URL != "https://github.com/cert-manager/cert-manager/releases/tag/v1.17.0" || r.PublishedAt != nil || r.Prerelease || r.Draft {
		t.Errorf("ref metadata = %+v", r)
	}
	ev := r.Evidence
	if ev.Kind != domain.EvidenceGitRef || ev.URI != r.URL || ev.Locator != "refs/tags/v1.17.0" ||
		ev.Excerpt != strings.Repeat("3", 40)+" refs/tags/v1.17.0" || ev.ID == "" || ev.ContentDigest == "" {
		t.Errorf("evidence = %+v", ev)
	}
	if !ev.RetrievedAt.Equal(newClock().Now()) {
		t.Errorf("evidence RetrievedAt = %v", ev.RetrievedAt)
	}
	// Evidence is stable across runs (idempotent ingestion).
	refs2, _ := newTestGit(t, t.TempDir(), lsRemoteRunner(t, lsRemoteFixture), nil, newClock(), Options{}).
		Tags().ListReleases(ctx, tagLocator("cert-manager/cert-manager", `^v(?P<version>\d+\.\d+\.\d+)$`))
	if refs2[0].Evidence.ID != ev.ID {
		t.Errorf("evidence id changed between runs: %s vs %s", refs2[0].Evidence.ID, ev.ID)
	}
	// A different tag has a different evidence id.
	if refs[1].Evidence.ID == ev.ID {
		t.Error("evidence ids must differ per tag")
	}
}

func TestListTagsGitLabEvidenceURI(t *testing.T) {
	g := newTestGit(t, t.TempDir(), lsRemoteRunner(t, lsRemoteFixture), nil, newClock(), Options{})
	refs, err := g.Tags().ListReleases(context.Background(), tagLocator("gitlab.com/group/proj", `^v1\.18\.1$`))
	if err != nil || len(refs) != 1 {
		t.Fatalf("%v %v", refs, err)
	}
	if refs[0].URL != "https://gitlab.com/group/proj/-/tags/v1.18.1" {
		t.Errorf("URL = %q", refs[0].URL)
	}
}

func TestListTagsPassesRepositoryToGit(t *testing.T) {
	r := lsRemoteRunner(t, lsRemoteFixture)
	g := newTestGit(t, t.TempDir(), r, nil, newClock(), Options{})
	if _, err := g.Tags().ListReleases(context.Background(), tagLocator("github.com/istio/istio", "")); err != nil {
		t.Fatal(err)
	}
	args := strings.Join(r.calls[0].Args, " ")
	if args != "ls-remote --tags -- https://github.com/istio/istio" {
		t.Errorf("git args = %q", args)
	}
}

func TestListTagsCacheTTLAndOffline(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	clk := newClock()
	runner := lsRemoteRunner(t, lsRemoteFixture)
	loc := tagLocator("github.com/cert-manager/cert-manager", `^v\d+\.\d+\.\d+$`)

	g := newTestGit(t, dir, runner, nil, clk, Options{TTL: time.Hour})
	first, err := g.Tags().ListReleases(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}
	// Within the TTL the cache answers.
	clk.Advance(30 * time.Minute)
	if _, err := g.Tags().ListReleases(ctx, loc); err != nil {
		t.Fatal(err)
	}
	if runner.count("ls-remote") != 1 {
		t.Fatalf("ls-remote ran %d times within the TTL, want 1", runner.count("ls-remote"))
	}
	// After the TTL it is refreshed.
	clk.Advance(2 * time.Hour)
	if _, err := g.Tags().ListReleases(ctx, loc); err != nil {
		t.Fatal(err)
	}
	if runner.count("ls-remote") != 2 {
		t.Fatalf("ls-remote ran %d times after the TTL, want 2", runner.count("ls-remote"))
	}
	// Refresh bypasses a fresh entry.
	gr := newTestGit(t, dir, runner, nil, clk, Options{Refresh: true})
	if _, err := gr.Tags().ListReleases(ctx, loc); err != nil {
		t.Fatal(err)
	}
	if runner.count("ls-remote") != 3 {
		t.Fatalf("Refresh did not go to the network: %d runs", runner.count("ls-remote"))
	}

	// Offline replays any age without running git, with the original
	// retrieval time in the evidence.
	clk.Advance(1000 * time.Hour)
	off := newTestGit(t, dir, failRunner(t), nil, clk, Options{Offline: true, TTL: time.Minute})
	replay, err := off.Tags().ListReleases(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != len(first) {
		t.Fatalf("replay has %d refs, first run %d", len(replay), len(first))
	}
	for i := range replay {
		if replay[i].Tag != first[i].Tag || replay[i].Evidence.ID != first[i].Evidence.ID {
			t.Errorf("replay[%d] = %+v differs from %+v", i, replay[i], first[i])
		}
	}

	// Offline and uncached: ErrOffline, and no git.
	_, err = off.Tags().ListReleases(ctx, tagLocator("github.com/other/repo", ""))
	if !errors.Is(err, fetch.ErrOffline) || fetch.StateFor(err) != domain.SourceUnavailable {
		t.Fatalf("want ErrOffline, got %v", err)
	}
}

func TestListTagsErrors(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		loc       catalog.Locator
		runErr    error
		sentinel  error
		wantState domain.SourceState
		msg       string
	}{
		{
			name: "unreachable host",
			loc:  tagLocator("github.com/o/r", ""),
			runErr: &RunError{Args: []string{"ls-remote"}, ExitCode: 128,
				Stderr: "fatal: unable to access 'https://github.com/o/r/': Could not resolve host: github.com"},
			sentinel: fetch.ErrUnavailable, wantState: domain.SourceUnavailable,
		},
		{
			name: "blocked by proxy",
			loc:  tagLocator("github.com/o/r", ""),
			runErr: &RunError{Args: []string{"ls-remote"}, ExitCode: 128,
				Stderr: "fatal: unable to access 'https://github.com/o/r/': CONNECT tunnel failed, response 403"},
			sentinel: fetch.ErrUnavailable, wantState: domain.SourceUnavailable,
		},
		{
			name:     "git not installed",
			loc:      tagLocator("github.com/o/r", ""),
			runErr:   &RunError{Args: []string{"ls-remote"}, ExitCode: -1, NotInstalled: true, Err: errors.New("exec: git: not found")},
			sentinel: fetch.ErrUnavailable, wantState: domain.SourceUnavailable,
		},
		{
			name:     "timeout",
			loc:      tagLocator("github.com/o/r", ""),
			runErr:   &RunError{Args: []string{"ls-remote"}, ExitCode: -1, TimedOut: true},
			sentinel: fetch.ErrUnavailable, wantState: domain.SourceUnavailable,
		},
		{
			name: "authentication required",
			loc:  tagLocator("github.com/o/r", ""),
			runErr: &RunError{Args: []string{"ls-remote"}, ExitCode: 128,
				Stderr: "fatal: could not read Username for 'https://github.com': terminal prompts disabled"},
			sentinel: fetch.ErrUnavailable, wantState: domain.SourceUnavailable,
		},
		{
			name: "repository does not exist",
			loc:  tagLocator("github.com/o/r", ""),
			runErr: &RunError{Args: []string{"ls-remote"}, ExitCode: 128,
				Stderr: "remote: Repository not found.\nfatal: repository 'https://github.com/o/r/' not found"},
			sentinel: fetch.ErrNotFound, wantState: domain.SourceNotFound,
		},
		{
			name:      "unclassifiable failure stays an error",
			loc:       tagLocator("github.com/o/r", ""),
			runErr:    &RunError{Args: []string{"ls-remote"}, ExitCode: 1, Stderr: "fatal: something odd"},
			wantState: domain.SourceError,
		},
		{
			name:      "invalid repository",
			loc:       tagLocator("nonsense", ""),
			wantState: domain.SourceError,
			msg:       "want host/owner/name",
		},
		{
			name:      "invalid pattern",
			loc:       tagLocator("github.com/o/r", "("),
			wantState: domain.SourceError,
			msg:       "invalid tagPattern",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{fn: func(Command) ([]byte, error) { return nil, tc.runErr }}
			g := newTestGit(t, t.TempDir(), runner, nil, newClock(), Options{})
			_, err := g.Tags().ListReleases(ctx, tc.loc)
			if err == nil {
				t.Fatal("want error")
			}
			if tc.sentinel != nil && !errors.Is(err, tc.sentinel) {
				t.Errorf("errors.Is(%v, %v) = false", err, tc.sentinel)
			}
			if got := fetch.StateFor(err); got != tc.wantState {
				t.Errorf("state = %q, want %q (err: %v)", got, tc.wantState, err)
			}
			if tc.msg != "" && !strings.Contains(err.Error(), tc.msg) {
				t.Errorf("error %q does not mention %q", err, tc.msg)
			}
		})
	}
}

func TestListTagsContextCancellationIsNotAnUnavailableSource(t *testing.T) {
	runner := &fakeRunner{fn: func(Command) ([]byte, error) { return nil, context.Canceled }}
	g := newTestGit(t, t.TempDir(), runner, nil, newClock(), Options{})
	_, err := g.Tags().ListReleases(context.Background(), tagLocator("github.com/o/r", ""))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestRegister(t *testing.T) {
	reg := sources.NewRegistry()
	Register(reg, &recordingFetch{}, Options{CacheDir: t.TempDir(), Runner: failRunner(t)})
	if _, err := reg.VersionLister(catalog.LocatorGitTags); err != nil {
		t.Error(err)
	}
	if _, err := reg.DocumentFetcher(catalog.LocatorRepoFile); err != nil {
		t.Error(err)
	}
	if _, err := reg.DocumentFetcher(catalog.LocatorGitLog); err != nil {
		t.Error(err)
	}
	if _, err := reg.DirectoryFetcher(catalog.LocatorRepoDir); err != nil {
		t.Error(err)
	}
	if _, err := reg.VersionLister(catalog.LocatorGitHubReleases); err == nil {
		t.Error("git adapters must not claim github-releases")
	}
}
