package linkedev

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/github"
)

var t0 = time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)

func ref(typ, id, url string) domain.Reference { return domain.Reference{Type: typ, ID: id, URL: url} }

func TestTargetsOf(t *testing.T) {
	sha := "618aae18515213bcf3fb820e6f8c234703d844b2"
	refs := []domain.Reference{
		ref("cve", "CVE-2026-1", ""),
		ref("ghsa", "GHSA-aaaa-bbbb-cccc", "https://github.com/advisories/GHSA-aaaa-bbbb-cccc"),
		ref("commit", "618aae1", "https://github.com/kubernetes/ingress-nginx/commit/"+sha),
		ref("issue", "kubernetes/ingress-nginx#11176", "https://github.com/kubernetes/ingress-nginx/issues/11176"),
		ref("github-ref", "cert-manager/cert-manager#7601", "https://github.com/cert-manager/cert-manager/issues/7601"),
		ref("pull-request", "kubernetes/ingress-nginx#11819", "https://github.com/kubernetes/ingress-nginx/pull/11819"),
		ref("pull-request", "Kubernetes/Ingress-Nginx#11819", "https://github.com/kubernetes/ingress-nginx/pull/11819"), // same item, other case
		ref("commit", "deadbee", "https://gitlab.com/o/r/-/commit/"+sha),                                                // not github
		ref("commit", "abcdef1", ""), // abbreviation only: unresolvable
		ref("pull-request", "bad id", ""),
		ref("pull-request", "o/r#0", ""),
	}
	got := TargetsOf(refs)
	var keys []string
	for _, tg := range got {
		keys = append(keys, tg.Kind+" "+tg.Key())
	}
	want := []string{
		"pull-request kubernetes/ingress-nginx#11819", // PRs first
		"github-ref cert-manager/cert-manager#7601",   // then bare #N
		"issue kubernetes/ingress-nginx#11176",
		"commit kubernetes/ingress-nginx@" + sha,
	}
	if strings.Join(keys, "|") != strings.Join(want, "|") {
		t.Errorf("targets = %v\nwant     %v", keys, want)
	}
	if TargetsOf(nil) != nil {
		t.Error("no references, no targets")
	}
}

// fakeFetcher serves canned items and records calls.
type fakeFetcher struct {
	calls   []string
	items   map[string]*github.LinkedItem
	err     map[string]error
	throttl string // key that answers throttled
}

func (f *fakeFetcher) LinkedItem(_ context.Context, owner, repo, kind string, n int, sha string) (*github.LinkedItem, error) {
	key := Target{Kind: kind, Owner: owner, Repo: repo, Number: n, SHA: sha}.Key()
	f.calls = append(f.calls, key)
	if key == f.throttl {
		return nil, &fetch.Error{URL: "x", Status: 429, Err: fetch.ErrThrottled, RetryAfter: 90 * time.Second}
	}
	if e := f.err[key]; e != nil {
		return nil, e
	}
	if it := f.items[key]; it != nil {
		return it, nil
	}
	return nil, fmt.Errorf("unexpected %s: %w", key, fetch.ErrNotFound)
}

func prItem(n int, title, body string) *github.LinkedItem {
	it := &github.LinkedItem{Kind: github.LinkedPullRequest, Owner: "o", Repo: "r", Number: n,
		URL: fmt.Sprintf("https://github.com/o/r/pull/%d", n), Title: title, State: "closed", Body: body,
		Files: []github.LinkedFile{{Name: "a.go", Status: "modified"}, {Name: "b.yaml", Status: "removed"}}}
	it.Digest = domain.Digest([]byte(title + body))
	return it
}

func chg(id string, refs ...domain.Reference) domain.Change {
	return domain.Change{ID: id, References: refs}
}
func pr(n int) domain.Reference {
	return ref("pull-request", fmt.Sprintf("o/r#%d", n), fmt.Sprintf("https://github.com/o/r/pull/%d", n))
}

func TestCollect(t *testing.T) {
	f := &fakeFetcher{items: map[string]*github.LinkedItem{
		"o/r#1": prItem(1, "one", "body one"), "o/r#2": prItem(2, "two", "body two"), "o/r#3": prItem(3, "three", "b3"),
		"o/r#4": prItem(4, "four", "b4"), "o/r#5": prItem(5, "five", "b5"),
	}, err: map[string]error{"o/r#6": fmt.Errorf("boom")}}
	changes := []domain.Change{
		chg("a", pr(1), pr(2)),
		chg("b", pr(2)), // shared with a: fetched once
		chg("c", pr(3), pr(4), pr(5), pr(6), pr(1)),
		chg("d"), // no references
	}
	res := Collect(context.Background(), f, changes, Options{MaxPerChange: 3, Now: t0})
	if got := strings.Join(f.calls, " "); got != "o/r#1 o/r#2 o/r#3 o/r#4 o/r#5" {
		t.Errorf("fetch order = %q (item 2 once; change c capped at 3 per change, so 6 and the repeated 1 are not requested for it)", got)
	}
	st := map[string]string{}
	for _, l := range res.Links {
		st[l.ChangeID+l.Target.Key()] = l.Status
	}
	if st["bo/r#2"] != StatusFetched || st["co/r#6"] != "" && st["co/r#6"] != StatusCapped {
		t.Errorf("statuses = %v", st)
	}
	by := res.ByChange()
	if len(by["a"]) == 0 || len(by["b"]) == 0 || len(by["d"]) != 0 {
		t.Errorf("by change = %v", by)
	}
	if len(res.Evidence) == 0 {
		t.Fatal("no evidence")
	}
	for _, e := range res.Evidence {
		if e.Kind != domain.EvidenceLinkedPR || !strings.HasPrefix(e.URI, "https://github.com/o/r/pull/") || e.SourceID != Producer || len(e.Excerpt) > domain.MaxExcerpt+4 {
			t.Errorf("evidence = %+v", e)
		}
	}
	if res.Fetches != 5 {
		t.Errorf("fetches = %d", res.Fetches)
	}

	// per-run cap
	f2 := &fakeFetcher{items: f.items}
	r2 := Collect(context.Background(), f2, changes, Options{MaxFetches: 2, Now: t0})
	if r2.Fetches != 2 || countStatus(r2, StatusCapped) == 0 {
		t.Errorf("run cap: fetches %d capped %d", r2.Fetches, countStatus(r2, StatusCapped))
	}

	// a missing and a failing item are recorded and skipped, never fatal
	f3 := &fakeFetcher{items: f.items, err: f.err}
	r3 := Collect(context.Background(), f3, []domain.Change{chg("x", pr(6), pr(99), pr(1))}, Options{Now: t0})
	if countStatus(r3, StatusError) != 1 || countStatus(r3, StatusNotFound) != 1 || countStatus(r3, StatusFetched) != 1 || r3.Throttled {
		t.Errorf("failure statuses: %+v", r3.Links)
	}
}

func countStatus(r *Result, s string) int {
	n := 0
	for _, l := range r.Links {
		if l.Status == s {
			n++
		}
	}
	return n
}

// A throttled response stops the run: nothing after it is requested, the
// targets are marked throttled, and what was already gathered is kept.
func TestCollectStopsWhenThrottled(t *testing.T) {
	f := &fakeFetcher{items: map[string]*github.LinkedItem{"o/r#1": prItem(1, "one", "b"), "o/r#3": prItem(3, "three", "b")}, throttl: "o/r#2"}
	res := Collect(context.Background(), f, []domain.Change{chg("a", pr(1)), chg("b", pr(2)), chg("c", pr(3))}, Options{Now: t0})
	if !res.Throttled || res.RetryAfter != 90*time.Second {
		t.Errorf("throttle state = %v / %v", res.Throttled, res.RetryAfter)
	}
	if strings.Join(f.calls, " ") != "o/r#1 o/r#2" {
		t.Errorf("calls after a throttle: %v", f.calls)
	}
	if countStatus(res, StatusThrottled) != 2 || countStatus(res, StatusFetched) != 1 || len(res.ByChange()["a"]) == 0 {
		t.Errorf("links = %+v", res.Links)
	}
}

func TestEvidenceFor(t *testing.T) {
	long := "<!-- template hint -->\n## Motivation\n\n\n" + strings.Repeat("The default changes and every consumer pinning it breaks. ", 40) + "\n```release-note\nNONE\n```"
	it := prItem(7, "Change the default", long)
	evs := EvidenceFor(it, t0)
	var locs []string
	for _, e := range evs {
		locs = append(locs, e.Locator)
		if len(e.Excerpt) > domain.MaxExcerpt+len("…") {
			t.Errorf("excerpt %q exceeds the store bound (%d)", e.Locator, len(e.Excerpt))
		}
		if strings.Contains(e.Excerpt, "template hint") {
			t.Error("html comment kept")
		}
	}
	if strings.Join(locs, "|") != "title and description|description (part 2)|changed files" {
		t.Errorf("locators = %v", locs)
	}
	if !strings.HasPrefix(evs[0].Excerpt, "PR: Change the default [closed]\n## Motivation") || !strings.HasSuffix(evs[1].Excerpt, "…") {
		t.Errorf("first = %q\nsecond tail = %q", evs[0].Excerpt, evs[1].Excerpt)
	}
	files := evs[2].Excerpt
	if files != "2 changed file(s): a.go, b.yaml (removed)" {
		t.Errorf("files = %q", files)
	}
	// ids are content-derived: same content, same ids, regardless of fetch time
	again := EvidenceFor(it, t0.Add(48*time.Hour))
	for i := range evs {
		if evs[i].ID != again[i].ID {
			t.Errorf("record %d id moved with the fetch time", i)
		}
	}
	// changed content, new ids
	if other := EvidenceFor(prItem(7, "Change the default", long+" edited"), t0); other[0].ID == evs[0].ID {
		t.Error("edited description kept the old id")
	}
	// a short body is one description record; no files, no files record
	short := prItem(8, "t", "short")
	short.Files = nil
	if e := EvidenceFor(short, t0); len(e) != 1 {
		t.Errorf("short item records = %d", len(e))
	}
	// commits and issues use their own kinds/labels
	cm := &github.LinkedItem{Kind: github.LinkedCommit, Owner: "o", Repo: "r", SHA: strings.Repeat("a", 40), URL: "https://github.com/o/r/commit/x", Title: "subject", Digest: "d"}
	if e := EvidenceFor(cm, t0); e[0].Kind != domain.EvidenceLinkedCommit || !strings.HasPrefix(e[0].Excerpt, "commit: subject") {
		t.Errorf("commit evidence = %+v", e[0])
	}
	if e := EvidenceFor(&github.LinkedItem{Kind: github.LinkedIssue, URL: "https://github.com/o/r/issues/1", Title: "i", Digest: "d"}, t0); !strings.HasPrefix(e[0].Excerpt, "issue: i") {
		t.Errorf("issue evidence = %+v", e[0])
	}
}

func TestChunkingHandlesHostileText(t *testing.T) {
	for _, s := range []string{strings.Repeat("é", 2000), strings.Repeat("x", 3000), strings.Repeat("word ", 500), ""} {
		for _, c := range chunk(s, 520, 2) {
			if len(c) > 600 {
				t.Errorf("chunk of %d bytes", len(c))
			}
			if strings.ContainsRune(c, '�') {
				t.Error("chunk split a rune")
			}
		}
	}
}

// A bare #N in notes read from a docs repository points at the code
// repository: it is looked up there first, falls back to the notes' own
// repository, and the evidence says it was inferred. Explicit references are
// never re-targeted.
func TestBareReferencesResolveInTheCodeRepository(t *testing.T) {
	bare := func(owner, repo string, n int) domain.Reference {
		return ref("github-ref", fmt.Sprintf("%s/%s#%d", owner, repo, n), fmt.Sprintf("https://github.com/%s/%s/issues/%d", owner, repo, n))
	}
	code := &github.LinkedItem{Kind: github.LinkedPullRequest, Owner: "o", Repo: "code", Number: 7, URL: "https://github.com/o/code/pull/7", Title: "the PR", Digest: "d1"}
	docs := &github.LinkedItem{Kind: github.LinkedIssue, Owner: "o", Repo: "docs", Number: 8, URL: "https://github.com/o/docs/issues/8", Title: "docs issue", Digest: "d2"}
	f := &fakeFetcher{items: map[string]*github.LinkedItem{"o/code#7": code, "o/docs#8": docs}}
	changes := []domain.Change{
		chg("a", bare("o", "docs", 7)), // exists only in the code repo
		chg("b", bare("o", "docs", 8)), // exists only in the docs repo: the fallback
		chg("c", ref("pull-request", "o/docs#7", "https://github.com/o/docs/pull/7")), // explicit: stays in docs, not found
		chg("d", bare("o", "docs", 99)),
	}
	res := Collect(context.Background(), f, changes, Options{Now: t0, CodeRepos: []string{"o/code"}})
	if got := strings.Join(f.calls, " "); got != "o/code#7 o/code#8 o/docs#8 o/docs#7 o/code#99 o/docs#99" {
		t.Errorf("lookup order = %q", got)
	}
	by := map[string]Link{}
	for _, l := range res.Links {
		by[l.ChangeID] = l
	}
	if by["a"].Status != StatusFetched || by["a"].Target.Repo != "code" || !strings.Contains(by["a"].Detail, "resolved in o/code") {
		t.Errorf("a = %+v", by["a"])
	}
	if by["b"].Status != StatusFetched || by["b"].Target.Repo != "docs" || by["b"].Detail != "" {
		t.Errorf("b = %+v", by["b"])
	}
	if by["c"].Status != StatusNotFound || by["d"].Status != StatusNotFound {
		t.Errorf("c/d = %s / %s", by["c"].Status, by["d"].Status)
	}
	var first string
	for _, e := range res.Evidence {
		if e.URI == "https://github.com/o/code/pull/7" && e.Locator == "title and description" {
			first = e.Excerpt
		}
	}
	if first != "PR (cited in the notes as #7): the PR" {
		t.Errorf("excerpt = %q", first)
	}
	// without code repositories nothing changes
	f2 := &fakeFetcher{items: f.items}
	Collect(context.Background(), f2, changes[:1], Options{Now: t0})
	if strings.Join(f2.calls, " ") != "o/docs#7" {
		t.Errorf("calls = %v", f2.calls)
	}
}

func TestCodeReposOf(t *testing.T) {
	def := &catalog.ProductDefinition{Sources: []catalog.Source{
		{ID: "notes", Roles: []domain.SourceRole{domain.RoleReleaseNotes}, Locator: catalog.Locator{Kind: "repo-file", Repository: "github.com/o/website"}},
		{ID: "rel", Roles: []domain.SourceRole{domain.RoleVersions}, Priority: 0, Locator: catalog.Locator{Kind: catalog.LocatorGitHubReleases, Repository: "o/code"}},
		{ID: "tags", Roles: []domain.SourceRole{domain.RoleVersions}, Priority: 1, Locator: catalog.Locator{Kind: "git-tags", Repository: "github.com/O/Code.git"}},
		{ID: "gl", Roles: []domain.SourceRole{domain.RoleVersions}, Priority: 2, Locator: catalog.Locator{Kind: "git-tags", Repository: "gitlab.example.com/a/b/c"}},
	}}
	if got := CodeReposOf(def); len(got) != 1 || got[0] != "o/code" {
		t.Errorf("code repos = %v (the docs repo, the duplicate and the non-github host are not code repositories)", got)
	}
	if CodeReposOf(nil) != nil {
		t.Error("nil definition")
	}
}
