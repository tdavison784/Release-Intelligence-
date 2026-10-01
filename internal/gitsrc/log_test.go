package gitsrc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
)

func TestParseLog(t *testing.T) {
	out := "aaa" + logSep + "feat(appset): add foo (#1234)\n" +
		"bbb" + logSep + "fix: has " + logSep + "-looking text? no, only the first separator splits\n" +
		"\n" +
		"ccc" + logSep + "\n"
	got, err := parseLog([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].SHA != "aaa" || got[0].Subject != "feat(appset): add foo (#1234)" || got[2].Subject != "" {
		t.Fatalf("got %+v", got)
	}
	if _, err := parseLog([]byte("no separator here\n")); err == nil {
		t.Error("want error for a malformed line")
	}
	if got, err := parseLog(nil); err != nil || len(got) != 0 {
		t.Errorf("empty: %v %v", got, err)
	}
}

func TestRenderCommits(t *testing.T) {
	sha1 := "0123456789abcdef0123456789abcdef01234567"
	sha2 := "fedcba9876543210fedcba9876543210fedcba98"
	commits := []commit{
		{SHA: sha1, Subject: "feat(appset): add foo (#1234)"},
		{SHA: sha2, Subject: "  fix(ui):   tabs\tand   spaces  "},
		{SHA: "abc", Subject: ""},
	}
	tests := []struct {
		name string
		repo string
		want string
	}{
		{
			name: "github",
			repo: "github.com/argoproj/argo-cd",
			want: "# Commits v3.0.0..v3.1.0\n\n" +
				"- feat(appset): add foo (#1234) ([0123456](https://github.com/argoproj/argo-cd/commit/" + sha1 + "))\n" +
				"- fix(ui): tabs and spaces ([fedcba9](https://github.com/argoproj/argo-cd/commit/" + sha2 + "))\n" +
				"- (no subject) ([abc](https://github.com/argoproj/argo-cd/commit/abc))\n",
		},
		{
			name: "gitlab",
			repo: "gitlab.com/g/p",
			want: "# Commits v3.0.0..v3.1.0\n\n" +
				"- feat(appset): add foo (#1234) ([0123456](https://gitlab.com/g/p/-/commit/" + sha1 + "))\n" +
				"- fix(ui): tabs and spaces ([fedcba9](https://gitlab.com/g/p/-/commit/" + sha2 + "))\n" +
				"- (no subject) ([abc](https://gitlab.com/g/p/-/commit/abc))\n",
		},
		{
			name: "unknown host has no links",
			repo: "git.example.org/team/tool",
			want: "# Commits v3.0.0..v3.1.0\n\n" +
				"- feat(appset): add foo (#1234) (0123456)\n" +
				"- fix(ui): tabs and spaces (fedcba9)\n" +
				"- (no subject) (abc)\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, err := ParseRepo(tc.repo)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(renderCommits(repo, "v3.0.0", "v3.1.0", commits)); got != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
	repo, _ := ParseRepo("github.com/o/r")
	if got := string(renderCommits(repo, "v1", "v2", nil)); got != "# Commits v1..v2\n" {
		t.Errorf("empty range = %q", got)
	}
}

// logFlow is a fake git for the git-log flow: a mirror that learns about a
// tag only after "git fetch".
type logFlow struct {
	t         *testing.T
	fetched   bool
	missing   string // revision known only after a fetch
	logOutput string
	fetchErr  error
	cloneErr  error
}

func (f *logFlow) run(c Command) ([]byte, error) {
	switch subcommand(c.Args) {
	case "clone":
		return nil, f.cloneErr
	case "config":
		return nil, nil
	case "rev-parse":
		rev := strings.TrimSuffix(c.Args[len(c.Args)-1], "^{commit}")
		if rev == f.missing && !f.fetched {
			return nil, &RunError{Args: c.Args, ExitCode: 1}
		}
		if rev == "never" {
			return nil, &RunError{Args: c.Args, ExitCode: 1}
		}
		return []byte(strings.Repeat("9", 40) + "\n"), nil
	case "fetch":
		f.fetched = true
		return nil, f.fetchErr
	case "log":
		return []byte(f.logOutput), nil
	}
	f.t.Errorf("unexpected git command %v", c.Args)
	return nil, nil
}

func TestFetchLogRendersCachesAndReplays(t *testing.T) {
	ctx := context.Background()
	sha1 := "0123456789abcdef0123456789abcdef01234567"
	sha2 := "fedcba9876543210fedcba9876543210fedcba98"
	flow := &logFlow{t: t, logOutput: sha1 + logSep + "feat(appset): add foo (#1234)\n" + sha2 + logSep + "fix: bar (#1235)\n"}
	runner := &fakeRunner{fn: flow.run}
	cacheDir := t.TempDir()
	clk := newClock()
	g := newTestGit(t, cacheDir, runner, nil, clk, Options{})
	loc := catalog.Locator{Kind: catalog.LocatorGitLog, Repository: "github.com/argoproj/argo-cd", Ref: "v3.0.0..v3.1.0"}

	doc, err := g.Log().FetchDocument(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}
	wantBody := "# Commits v3.0.0..v3.1.0\n\n" +
		"- feat(appset): add foo (#1234) ([0123456](https://github.com/argoproj/argo-cd/commit/" + sha1 + "))\n" +
		"- fix: bar (#1235) ([fedcba9](https://github.com/argoproj/argo-cd/commit/" + sha2 + "))\n"
	if string(doc.Content) != wantBody {
		t.Fatalf("content:\n%s\nwant:\n%s", doc.Content, wantBody)
	}
	if doc.URI != "https://github.com/argoproj/argo-cd/compare/v3.0.0...v3.1.0" || doc.Format != "markdown" ||
		doc.Digest != domain.Digest(doc.Content) || doc.Locator != loc || doc.FetchURL != "https://github.com/argoproj/argo-cd" ||
		!doc.RetrievedAt.Equal(clk.Now()) {
		t.Errorf("doc = %+v", doc)
	}

	// The mirror is created once; both refs are present, so there is no fetch.
	if runner.count("clone") != 1 || runner.count("fetch") != 0 || runner.count("log") != 1 {
		t.Errorf("git calls = %v", runner.subcommands())
	}
	var logArgs string
	for _, c := range runner.calls {
		if subcommand(c.Args) == "log" {
			logArgs = strings.Join(c.Args, " ")
		}
	}
	for _, want := range []string{"--no-merges", "--reverse", "v3.0.0..v3.1.0"} {
		if !strings.Contains(logArgs, want) {
			t.Errorf("git log args %q lack %q", logArgs, want)
		}
	}

	// Immutable range: served from the cache forever, no git at all.
	clk.Advance(5000 * time.Hour)
	again, err := newTestGit(t, cacheDir, failRunner(t), nil, clk, Options{TTL: time.Minute}).Log().FetchDocument(ctx, loc)
	if err != nil || again.Digest != doc.Digest || !again.RetrievedAt.Equal(doc.RetrievedAt) {
		t.Fatalf("cached: %v %+v", err, again)
	}

	// Offline replay is identical.
	off, err := newTestGit(t, cacheDir, failRunner(t), nil, clk, Options{Offline: true}).Log().FetchDocument(ctx, loc)
	if err != nil || off.Digest != doc.Digest {
		t.Fatalf("offline: %v", err)
	}
	// Not cached while offline.
	other := loc
	other.Ref = "v3.1.0..v3.2.0"
	_, err = newTestGit(t, cacheDir, failRunner(t), nil, clk, Options{Offline: true}).Log().FetchDocument(ctx, other)
	if !errors.Is(err, fetch.ErrOffline) {
		t.Fatalf("want ErrOffline, got %v", err)
	}
}

func TestFetchLogFetchesWhenARefIsMissing(t *testing.T) {
	flow := &logFlow{t: t, missing: "v3.2.0", logOutput: ""}
	runner := &fakeRunner{fn: flow.run}
	g := newTestGit(t, t.TempDir(), runner, nil, nil, Options{})
	doc, err := g.Log().FetchDocument(context.Background(), catalog.Locator{
		Kind: "git-log", Repository: "github.com/o/r", Ref: "v3.1.0..v3.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if string(doc.Content) != "# Commits v3.1.0..v3.2.0\n" {
		t.Errorf("empty range content = %q", doc.Content)
	}
	if runner.count("fetch") != 1 {
		t.Errorf("want one fetch after the miss, got %v", runner.subcommands())
	}
	// Second range: the mirror exists, nothing is cloned again.
	if _, err := g.Log().FetchDocument(context.Background(), catalog.Locator{
		Kind: "git-log", Repository: "github.com/o/r", Ref: "v3.0.0..v3.1.0"}); err != nil {
		t.Fatal(err)
	}
	if runner.count("clone") != 1 {
		t.Errorf("mirror cloned %d times", runner.count("clone"))
	}
}

func TestFetchLogMutableRangeRefetchesAfterTTL(t *testing.T) {
	flow := &logFlow{t: t, logOutput: "aa" + logSep + "one\n"}
	runner := &fakeRunner{fn: flow.run}
	clk := newClock()
	g := newTestGit(t, t.TempDir(), runner, nil, clk, Options{TTL: time.Hour})
	loc := catalog.Locator{Kind: "git-log", Repository: "github.com/o/r", Ref: "v1.0.0..release-1.1"}

	if _, err := g.Log().FetchDocument(context.Background(), loc); err != nil {
		t.Fatal(err)
	}
	// A branch endpoint forces a fetch even though both refs exist.
	if runner.count("fetch") != 1 {
		t.Fatalf("mutable range must fetch: %v", runner.subcommands())
	}
	clk.Advance(time.Minute)
	if _, err := g.Log().FetchDocument(context.Background(), loc); err != nil {
		t.Fatal(err)
	}
	if runner.count("log") != 1 {
		t.Errorf("within the TTL the cached document is served: %v", runner.subcommands())
	}
	flow.logOutput = "aa" + logSep + "one\nbb" + logSep + "two\n"
	clk.Advance(2 * time.Hour)
	doc, err := g.Log().FetchDocument(context.Background(), loc)
	if err != nil || !strings.Contains(string(doc.Content), "two") {
		t.Fatalf("after the TTL: %v %s", err, doc.Content)
	}
}

func TestFetchLogErrors(t *testing.T) {
	ctx := context.Background()
	loc := catalog.Locator{Kind: "git-log", Repository: "github.com/o/r", Ref: "v1.0.0..v2.0.0"}

	t.Run("unknown revision is not found", func(t *testing.T) {
		flow := &logFlow{t: t}
		l := loc
		l.Ref = "v1.0.0..never"
		g := newTestGit(t, t.TempDir(), &fakeRunner{fn: flow.run}, nil, nil, Options{})
		_, err := g.Log().FetchDocument(ctx, l)
		if !errors.Is(err, fetch.ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("clone failure is unavailable and leaves no half mirror", func(t *testing.T) {
		flow := &logFlow{t: t, cloneErr: &RunError{Args: []string{"clone"}, ExitCode: 128,
			Stderr: "fatal: unable to access 'https://github.com/o/r/': Could not resolve host: github.com"}}
		dir := t.TempDir()
		g := newTestGit(t, dir, &fakeRunner{fn: flow.run}, nil, nil, Options{})
		_, err := g.Log().FetchDocument(ctx, loc)
		if !errors.Is(err, fetch.ErrUnavailable) {
			t.Fatalf("got %v", err)
		}
		if entries, _ := os.ReadDir(filepath.Join(dir, "git", "mirrors")); len(entries) != 0 {
			t.Errorf("failed clone left %v behind", entries)
		}
	})
	t.Run("fetch failure is unavailable", func(t *testing.T) {
		flow := &logFlow{t: t, missing: "v2.0.0", fetchErr: &RunError{Args: []string{"fetch"}, ExitCode: 128,
			Stderr: "fatal: unable to access: Connection refused"}}
		g := newTestGit(t, t.TempDir(), &fakeRunner{fn: flow.run}, nil, nil, Options{})
		_, err := g.Log().FetchDocument(ctx, loc)
		if !errors.Is(err, fetch.ErrUnavailable) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("invalid ranges", func(t *testing.T) {
		g := newTestGit(t, t.TempDir(), failRunner(t), nil, nil, Options{})
		for _, ref := range []string{"", "v1", "v1...v2", "..v2", "-x..v2", "v1..v2..v3"} {
			l := loc
			l.Ref = ref
			if _, err := g.Log().FetchDocument(ctx, l); err == nil {
				t.Errorf("ref %q: want error", ref)
			}
		}
		l := loc
		l.Repository = "nope"
		if _, err := g.Log().FetchDocument(ctx, l); err == nil {
			t.Error("bad repository: want error")
		}
	})
}

func TestEnsureMirrorRedoesAnInterruptedClone(t *testing.T) {
	flow := &logFlow{t: t}
	runner := &fakeRunner{fn: flow.run}
	dir := t.TempDir()
	g := newTestGit(t, dir, runner, nil, nil, Options{})
	repo, _ := ParseRepo("github.com/o/r")
	mirror := filepath.Join(dir, "git", "mirrors", repo.slug()+".git")
	// A directory without the completion marker is what a killed clone leaves.
	if err := os.MkdirAll(filepath.Join(mirror, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := g.ensureMirror(context.Background(), repo, mirror); err != nil {
		t.Fatal(err)
	}
	if runner.count("clone") != 1 {
		t.Fatalf("incomplete mirror must be re-cloned: %v", runner.subcommands())
	}
	if _, err := os.Stat(filepath.Join(mirror, mirrorReady)); err != nil {
		t.Errorf("marker missing: %v", err)
	}
	// With the marker present nothing happens.
	if err := g.ensureMirror(context.Background(), repo, mirror); err != nil || runner.count("clone") != 1 {
		t.Errorf("complete mirror must be kept: %v %v", err, runner.subcommands())
	}
}

func TestFetchLogIntegration(t *testing.T) {
	r := newTestRepo(t)
	r.commit("feat(core): first (#1)", map[string]string{"a.txt": "1"})
	r.git("tag", "-a", "v1.0.0", "-m", "v1.0.0")
	r.git("checkout", "-q", "-b", "side")
	r.commit("chore: side work (#3)", map[string]string{"side.txt": "s"})
	r.git("checkout", "-q", "main")
	r.commit("fix(core): second (#2)", map[string]string{"b.txt": "2"})
	r.git("merge", "-q", "--no-ff", "-m", "Merge branch 'side'", "side")
	r.git("tag", "v1.1.0")

	ctx := context.Background()
	cacheDir := t.TempDir()
	g := newTestGit(t, cacheDir, nil, nil, nil, Options{})
	loc := catalog.Locator{Kind: catalog.LocatorGitLog, Repository: r.URL, Ref: "v1.0.0..v1.1.0"}

	doc, err := g.Log().FetchDocument(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(doc.Content)), "\n")
	if lines[0] != "# Commits v1.0.0..v1.1.0" || lines[1] != "" {
		t.Fatalf("header = %q", lines[:2])
	}
	// Oldest first, merges excluded, hostless repository: no links.
	bullets := lines[2:]
	if len(bullets) != 2 {
		t.Fatalf("want 2 bullets (no merge commit), got %q", bullets)
	}
	if strings.Contains(string(doc.Content), "Merge branch") {
		t.Errorf("merge commit listed: %s", doc.Content)
	}
	wantOrder := strings.Split(r.git("log", "--no-merges", "--topo-order", "--reverse", "--format=%s (%h)", "v1.0.0..v1.1.0"), "\n")
	for i, b := range bullets {
		if want := "- " + wantOrder[i]; !strings.HasPrefix(b, want[:len(want)-len(" (abcdefg)")]) || !strings.HasSuffix(b, ")") {
			t.Errorf("bullet %d = %q, want subject of %q", i, b, wantOrder[i])
		}
	}
	// The side-branch commit is part of the range and carries its short id.
	sideSHA := r.git("rev-parse", "side")
	if !strings.Contains(string(doc.Content), "- chore: side work (#3) ("+sideSHA[:7]+")") {
		t.Errorf("side commit missing: %s", doc.Content)
	}

	// A tag that appears after the mirror was cloned is picked up by a fetch.
	r.commit("feat(core): third (#4)", map[string]string{"c.txt": "3"})
	r.git("tag", "v1.2.0")
	doc2, err := g.Log().FetchDocument(ctx, catalog.Locator{Kind: catalog.LocatorGitLog, Repository: r.URL, Ref: "v1.1.0..v1.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc2.Content), "feat(core): third (#4)") || strings.Count(string(doc2.Content), "\n- ") != 1 {
		t.Errorf("doc2 = %s", doc2.Content)
	}

	// Offline replay of both ranges without git.
	off := newTestGit(t, cacheDir, failRunner(t), nil, nil, Options{Offline: true})
	again, err := off.Log().FetchDocument(ctx, loc)
	if err != nil || again.Digest != doc.Digest {
		t.Fatalf("offline replay: %v", err)
	}

	// A revision that does not exist.
	_, err = g.Log().FetchDocument(ctx, catalog.Locator{Kind: catalog.LocatorGitLog, Repository: r.URL, Ref: "v1.0.0..v9.9.9"})
	if !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("missing revision: %v", err)
	}
}
