package gitsrc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
)

func TestParseLsTree(t *testing.T) {
	out := "100644 blob aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\tnotes/b.yaml\x00" +
		"100755 blob bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\tnotes/a script.sh\x00" +
		"120000 blob cccccccccccccccccccccccccccccccccccccccc\tnotes/link\x00" + // symlink: skipped
		"160000 commit dddddddddddddddddddddddddddddddddddddddd\tnotes/submodule\x00" + // gitlink: skipped
		"100644 blob eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\tnotes/sub/ü file.yaml\x00"
	got, err := parseLsTree([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, e := range got {
		paths = append(paths, e.Path)
	}
	want := []string{"notes/a script.sh", "notes/b.yaml", "notes/sub/ü file.yaml"}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("paths = %q, want %q", paths, want)
	}
	if got[1].OID != strings.Repeat("a", 40) {
		t.Errorf("oid = %q", got[1].OID)
	}
	if _, err := parseLsTree([]byte("garbage\x00")); err == nil {
		t.Error("want error for a malformed record")
	}
	if got, err := parseLsTree(nil); err != nil || len(got) != 0 {
		t.Errorf("empty: %v %v", got, err)
	}
}

func TestSelectEntries(t *testing.T) {
	at := func(paths ...string) []treeEntry {
		var out []treeEntry
		for i, p := range paths {
			out = append(out, treeEntry{Path: p, OID: fmt.Sprintf("%040d", i)})
		}
		return out
	}
	paths := func(es []treeEntry) string {
		var ps []string
		for _, e := range es {
			ps = append(ps, e.Path)
		}
		return strings.Join(ps, ",")
	}
	ref := at("n/a.yaml", "n/b.yaml", "n/c.txt", "n/sub/d.yaml", "n/sub/e.md")
	tests := []struct {
		name string
		base []treeEntry
		glob string
		want string
	}{
		{"default glob keeps everything", nil, "", "n/a.yaml,n/b.yaml,n/c.txt,n/sub/d.yaml,n/sub/e.md"},
		{"star", nil, "*", "n/a.yaml,n/b.yaml,n/c.txt,n/sub/d.yaml,n/sub/e.md"},
		{"glob matches the base name and recurses", nil, "*.yaml", "n/a.yaml,n/b.yaml,n/sub/d.yaml"},
		{"glob is not matched against the directory", nil, "sub*", ""},
		{"character class", nil, "[ab].yaml", "n/a.yaml,n/b.yaml"},
		{"base ref removes files that already existed", at("n/a.yaml", "n/c.txt", "n/old.yaml"), "", "n/b.yaml,n/sub/d.yaml,n/sub/e.md"},
		{"base ref and glob", at("n/a.yaml"), "*.yaml", "n/b.yaml,n/sub/d.yaml"},
		{"base ref identical to ref gives nothing", ref, "", ""},
		{"files only at the base are irrelevant", at("n/gone.yaml"), "*.md", "n/sub/e.md"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectEntries(ref, tc.base, tc.glob)
			if err != nil {
				t.Fatal(err)
			}
			if paths(got) != tc.want {
				t.Errorf("got %q, want %q", paths(got), tc.want)
			}
		})
	}
	if _, err := selectEntries(ref, nil, "[a"); err == nil {
		t.Error("want error for an invalid glob")
	}
}

func TestParseCatFileBatch(t *testing.T) {
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 40)
	out := a + " blob 5\nhello\n" + b + " blob 12\nline1\nline2\n\n"
	got, err := parseCatFileBatch([]byte(out), []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if string(got[a]) != "hello" || string(got[b]) != "line1\nline2\n" {
		t.Errorf("got %q", got)
	}
	// Empty blob.
	got, err = parseCatFileBatch([]byte(a+" blob 0\n\n"), []string{a})
	if err != nil || len(got[a]) != 0 {
		t.Errorf("empty blob: %q %v", got, err)
	}
	for name, bad := range map[string]string{
		"missing object": a + " missing\n",
		"truncated":      a + " blob 50\nshort\n",
		"wrong oid":      b + " blob 1\nx\n",
		"bad size":       a + " blob x\n",
		"no output":      "",
	} {
		if _, err := parseCatFileBatch([]byte(bad), []string{a}); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

// istioLikeRepo builds a repository shaped like istio/istio's release notes
// directory: a directory that accumulates files over releases.
func istioLikeRepo(t *testing.T) *testRepo {
	r := newTestRepo(t)
	r.commit("first", map[string]string{
		"README.md":                     "# repo\n",
		"releasenotes/notes/a.yaml":     "kind: bug-fix\narea: a\n",
		"releasenotes/notes/c.txt":      "not yaml\n",
		"releasenotes/notes/sub/b.yaml": "kind: feature\narea: b\n",
		"other/x.yaml":                  "outside: true\n",
	})
	r.git("tag", "-a", "1.0.0", "-m", "release 1.0.0")
	r.commit("second", map[string]string{
		"releasenotes/notes/a.yaml":          "kind: bug-fix\narea: a\nmodified: true\n",
		"releasenotes/notes/d.yaml":          "kind: feature\narea: d\n",
		"releasenotes/notes/sub/e.yaml":      "kind: bug-fix\narea: e\n",
		"releasenotes/notes/with space.yaml": "kind: x\n",
	})
	r.git("tag", "1.1.0")
	return r
}

func dirLocator(r *testRepo, ref, base, glob string) catalog.Locator {
	return catalog.Locator{
		Kind: catalog.LocatorRepoDir, Repository: r.URL, Ref: ref, BaseRef: base,
		Path: "releasenotes/notes", Glob: glob,
	}
}

func pathsOf(t *testing.T, g *Git, loc catalog.Locator) (string, error) {
	t.Helper()
	docs, err := g.Dirs().FetchDirectory(context.Background(), loc)
	var ps []string
	for _, d := range docs {
		ps = append(ps, strings.TrimPrefix(d.Path, "releasenotes/notes/"))
	}
	return strings.Join(ps, ","), err
}

func TestFetchDirIntegration(t *testing.T) {
	r := istioLikeRepo(t)
	ctx := context.Background()
	dir := t.TempDir()
	g := newTestGit(t, dir, nil, nil, nil, Options{})

	tests := []struct {
		name string
		loc  catalog.Locator
		want string
	}{
		{"everything at the newer tag, recursing, sorted", dirLocator(r, "1.1.0", "", ""), "a.yaml,c.txt,d.yaml,sub/b.yaml,sub/e.yaml,with space.yaml"},
		{"glob on the base name", dirLocator(r, "1.1.0", "", "*.yaml"), "a.yaml,d.yaml,sub/b.yaml,sub/e.yaml,with space.yaml"},
		{"older tag", dirLocator(r, "1.0.0", "", ""), "a.yaml,c.txt,sub/b.yaml"},
		{"only files added since the base ref", dirLocator(r, "1.1.0", "1.0.0", ""), "d.yaml,sub/e.yaml,with space.yaml"},
		{"added files, glob applied", dirLocator(r, "1.1.0", "1.0.0", "[de].yaml"), "d.yaml,sub/e.yaml"},
		{"nothing added", dirLocator(r, "1.1.0", "1.1.0", ""), ""},
		{"branch ref", dirLocator(r, "main", "1.0.0", "*.yaml"), "d.yaml,sub/e.yaml,with space.yaml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pathsOf(t, g, tc.loc)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("files = %q, want %q", got, tc.want)
			}
		})
	}

	// Document details.
	docs, err := g.Dirs().FetchDirectory(ctx, dirLocator(r, "1.1.0", "1.0.0", "d.yaml"))
	if err != nil || len(docs) != 1 {
		t.Fatalf("%v %v", docs, err)
	}
	d := docs[0]
	if d.Path != "releasenotes/notes/d.yaml" || string(d.Content) != "kind: feature\narea: d\n" || d.Format != "yaml" ||
		d.Digest != domain.Digest(d.Content) || d.RetrievedAt.IsZero() || d.FetchURL != r.URL {
		t.Errorf("doc = %+v", d)
	}
	// The repository is hostless, so the blob URI degrades to the repository URL.
	if d.URI != r.URL {
		t.Errorf("URI = %q", d.URI)
	}

	// Modified files that exist at both refs carry the content of Ref.
	docs, err = g.Dirs().FetchDirectory(ctx, dirLocator(r, "1.1.0", "", "a.yaml"))
	if err != nil || len(docs) != 1 || !strings.Contains(string(docs[0].Content), "modified: true") {
		t.Fatalf("a.yaml at 1.1.0: %v %v", docs, err)
	}
	docs, err = g.Dirs().FetchDirectory(ctx, dirLocator(r, "1.0.0", "", "a.yaml"))
	if err != nil || len(docs) != 1 || strings.Contains(string(docs[0].Content), "modified") {
		t.Fatalf("a.yaml at 1.0.0: %v %v", docs, err)
	}
}

func TestFetchDirBlobURIsAreHostSpecific(t *testing.T) {
	// A fake runner that answers exactly the commands of the repo-dir flow
	// is enough to check URI construction for a GitHub repository.
	oid := strings.Repeat("ab", 20)
	runner := &fakeRunner{fn: func(c Command) ([]byte, error) {
		switch subcommand(c.Args) {
		case "rev-parse":
			return []byte(strings.Repeat("12", 20) + "\n"), nil
		case "ls-tree":
			return []byte("100644 blob " + oid + "\treleasenotes/notes/x.yaml\x00"), nil
		case "cat-file":
			return []byte(oid + " blob 3\nhi\n\n"), nil
		}
		return nil, nil
	}}
	g := newTestGit(t, t.TempDir(), runner, nil, nil, Options{})
	docs, err := g.Dirs().FetchDirectory(context.Background(), catalog.Locator{
		Kind: "repo-dir", Repository: "github.com/istio/istio", Ref: "1.31.1", Path: "releasenotes/notes",
	})
	if err != nil || len(docs) != 1 {
		t.Fatalf("%v %v", docs, err)
	}
	if docs[0].URI != "https://github.com/istio/istio/blob/1.31.1/releasenotes/notes/x.yaml" ||
		docs[0].FetchURL != "https://github.com/istio/istio" || string(docs[0].Content) != "hi\n" {
		t.Errorf("doc = %+v", docs[0])
	}
	// An immutable ref that is already in the work repository is not fetched again.
	if runner.count("fetch") != 1 { // only the blob prefetch
		t.Errorf("fetch ran %d times: %v", runner.count("fetch"), runner.subcommands())
	}
}

func TestFetchDirCachingAndOffline(t *testing.T) {
	r := istioLikeRepo(t)
	ctx := context.Background()
	cacheDir := t.TempDir()
	clk := newClock()
	g := newTestGit(t, cacheDir, nil, nil, clk, Options{TTL: time.Hour})

	loc := dirLocator(r, "1.1.0", "1.0.0", "*.yaml")
	first, err := pathsOf(t, g, loc)
	if err != nil {
		t.Fatal(err)
	}

	// Offline replay: no git, identical documents, original retrieval time.
	off := newTestGit(t, cacheDir, failRunner(t), nil, clk, Options{Offline: true})
	docsOnline, _ := g.Dirs().FetchDirectory(ctx, loc)
	docsOffline, err := off.Dirs().FetchDirectory(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}
	if len(docsOnline) != len(docsOffline) {
		t.Fatalf("offline %d files, online %d", len(docsOffline), len(docsOnline))
	}
	for i := range docsOnline {
		a, b := docsOnline[i], docsOffline[i]
		if a.Path != b.Path || a.Digest != b.Digest || string(a.Content) != string(b.Content) || !a.RetrievedAt.Equal(b.RetrievedAt) {
			t.Errorf("doc %d differs: %+v vs %+v", i, a, b)
		}
	}
	if got, _ := pathsOf(t, off, loc); got != first {
		t.Errorf("offline files %q != online %q", got, first)
	}

	// A request that was never made is not available offline.
	_, err = off.Dirs().FetchDirectory(ctx, dirLocator(r, "1.0.0", "", ""))
	if !errors.Is(err, fetch.ErrOffline) {
		t.Fatalf("want ErrOffline, got %v", err)
	}

	// Immutable refs never expire, even when the TTL is long past.
	clk.Advance(1000 * time.Hour)
	runner := &fakeRunner{}
	again := newTestGit(t, cacheDir, runner, nil, clk, Options{TTL: time.Hour})
	if _, err := again.Dirs().FetchDirectory(ctx, loc); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("immutable result must come from the cache, git ran: %v", runner.subcommands())
	}
}

func TestFetchDirMutableRefExpires(t *testing.T) {
	r := istioLikeRepo(t)
	cacheDir := t.TempDir()
	clk := newClock()
	g := newTestGit(t, cacheDir, nil, nil, clk, Options{TTL: time.Hour})
	loc := dirLocator(r, "main", "", "*.yaml")

	before, err := pathsOf(t, g, loc)
	if err != nil {
		t.Fatal(err)
	}
	r.commit("third", map[string]string{"releasenotes/notes/new.yaml": "kind: x\n"})

	clk.Advance(10 * time.Minute)
	if got, _ := pathsOf(t, g, loc); got != before {
		t.Fatalf("within the TTL the cached listing is served: %q", got)
	}
	clk.Advance(2 * time.Hour)
	after, err := pathsOf(t, g, loc)
	if err != nil {
		t.Fatal(err)
	}
	if after == before || !strings.Contains(after, "new.yaml") {
		t.Fatalf("after the TTL the branch is re-read: %q", after)
	}

	// Refresh bypasses a fresh entry.
	r.commit("fourth", map[string]string{"releasenotes/notes/newer.yaml": "kind: x\n"})
	gr := newTestGit(t, cacheDir, nil, nil, clk, Options{TTL: time.Hour, Refresh: true})
	if got, _ := pathsOf(t, gr, loc); !strings.Contains(got, "newer.yaml") {
		t.Fatalf("Refresh must re-read: %q", got)
	}
}

func TestFetchDirBySHAAndErrors(t *testing.T) {
	r := istioLikeRepo(t)
	ctx := context.Background()
	g := newTestGit(t, t.TempDir(), nil, nil, nil, Options{})

	sha := r.git("rev-parse", "1.0.0^{commit}")
	got, err := pathsOf(t, g, dirLocator(r, sha, "", "*.yaml"))
	if err != nil || got != "a.yaml,sub/b.yaml" {
		t.Fatalf("by sha: %q %v", got, err)
	}

	// A path that does not exist at Ref is "not found", not an empty listing.
	loc := dirLocator(r, "1.1.0", "", "")
	loc.Path = "no/such/dir"
	if _, err := g.Dirs().FetchDirectory(ctx, loc); !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("missing path: %v", err)
	}
	// A ref that does not exist is "not found".
	if _, err := g.Dirs().FetchDirectory(ctx, dirLocator(r, "9.9.9", "", "")); !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("missing ref: %v", err)
	}
	if _, err := g.Dirs().FetchDirectory(ctx, dirLocator(r, "1.1.0", "9.9.9", "")); !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("missing base ref: %v", err)
	}
	// An unreachable repository is "unavailable".
	missing := "file://" + filepath.Join(t.TempDir(), "does-not-exist")
	_, err = g.Dirs().FetchDirectory(ctx, catalog.Locator{Kind: "repo-dir", Repository: missing, Ref: "1.0.0", Path: "x"})
	if err == nil || fetch.StateFor(err) == domain.SourceOK {
		t.Errorf("missing repository: %v", err)
	}
	// Invalid input.
	for name, l := range map[string]catalog.Locator{
		"bad glob":       {Repository: r.URL, Ref: "1.1.0", Path: "releasenotes", Glob: "[a"},
		"bad path":       {Repository: r.URL, Ref: "1.1.0", Path: "../x"},
		"bad ref":        {Repository: r.URL, Ref: "-x", Path: "x"},
		"bad base ref":   {Repository: r.URL, Ref: "1.1.0", BaseRef: "a b", Path: "x"},
		"bad repository": {Repository: "nope", Ref: "1.1.0", Path: "x"},
	} {
		if _, err := g.Dirs().FetchDirectory(ctx, l); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestFetchDirWholeRepositoryRoot(t *testing.T) {
	r := istioLikeRepo(t)
	g := newTestGit(t, t.TempDir(), nil, nil, nil, Options{})
	docs, err := g.Dirs().FetchDirectory(context.Background(), catalog.Locator{
		Kind: "repo-dir", Repository: r.URL, Ref: "1.0.0", Glob: "*.md",
	})
	if err != nil || len(docs) != 1 || docs[0].Path != "README.md" || docs[0].Format != "markdown" {
		t.Fatalf("%+v %v", docs, err)
	}
}

func TestWorkRepoIsReusedAcrossRefs(t *testing.T) {
	r := istioLikeRepo(t)
	cacheDir := t.TempDir()
	g := newTestGit(t, cacheDir, nil, nil, nil, Options{})
	for _, ref := range []string{"1.0.0", "1.1.0"} {
		if _, err := pathsOf(t, g, dirLocator(r, ref, "", "")); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(cacheDir, "git", "work"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("want exactly one work repository, got %v (%v)", entries, err)
	}
}
