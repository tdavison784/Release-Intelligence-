package gitsrc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// logSep separates the fields of one commit in the "git log" format; it
// cannot occur in a commit subject.
const logSep = "\x1f"

// commit is one commit of a rendered range.
type commit struct {
	SHA     string
	Subject string
}

// parseLog parses "git log --format=%H<US>%s" output, one commit per line.
func parseLog(out []byte) ([]commit, error) {
	var commits []commit
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		sha, subject, ok := strings.Cut(line, logSep)
		if !ok || sha == "" {
			return nil, fmt.Errorf("malformed git log line %q", line)
		}
		commits = append(commits, commit{SHA: sha, Subject: subject})
	}
	return commits, nil
}

// shortSHA is the abbreviation shown in link text.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// renderCommits renders the commits of the range a..b of repo as markdown,
// oldest commit first (the order of GoReleaser's "sort: asc" changelogs):
//
//	# Commits A..B
//
//	- feat(appset): add foo (#1234) ([abc1234](https://github.com/o/r/commit/<full sha>))
//
// One bullet per commit; the subject is kept verbatim (whitespace collapsed).
// Hosts whose URL layout is unknown get "(abc1234)" without a link. An empty
// range yields only the heading.
func renderCommits(repo Repo, a, b string, commits []commit) []byte {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Commits %s..%s\n", a, b)
	if len(commits) > 0 {
		sb.WriteString("\n")
	}
	for _, c := range commits {
		subject := strings.Join(strings.Fields(c.Subject), " ")
		if subject == "" {
			subject = "(no subject)"
		}
		if u := repo.CommitURL(c.SHA); u != "" {
			fmt.Fprintf(&sb, "- %s ([%s](%s))\n", subject, shortSHA(c.SHA), u)
		} else {
			fmt.Fprintf(&sb, "- %s (%s)\n", subject, shortSHA(c.SHA))
		}
	}
	return []byte(sb.String())
}

// logPayload is the cached form of a rendered range.
type logPayload struct {
	Markdown string `json:"markdown"`
	Commits  int    `json:"commits"`
}

// fetchLog implements the git-log locator kind. loc.Ref is a range "A..B":
// the document lists the non-merge commits reachable from B but not from A,
// oldest first, one markdown bullet each (see renderCommits). Its URI is the
// host's compare page A...B.
//
// Online, the commits are read from a treeless bare mirror kept under
// <CacheDir>/git/mirrors (history without trees or blobs is small: roughly
// 13 MB for Argo CD). The mirror is created on first use and updated with
// "git fetch" when an endpoint of the range is missing or mutable. The
// rendered document is cached; it never expires when both endpoints are
// immutable (version tags or commit ids, see IsImmutableRef), and it is what
// offline mode replays.
func (g *Git) fetchLog(ctx context.Context, loc catalog.Locator) (*sources.Document, error) {
	repo, err := ParseRepo(loc.Repository)
	if err != nil {
		return nil, fmt.Errorf("git-log: %w", err)
	}
	a, b, err := splitRange(loc.Ref)
	if err != nil {
		return nil, fmt.Errorf("git-log %s: %w", repo, err)
	}
	rng := a + ".." + b
	immutable := IsImmutableRef(a) && IsImmutableRef(b)
	cachePath := filepath.Join(g.root, "log", repo.slug(), requestKey(repo.CloneURL, rng)+".json.gz")

	doc := func(p logPayload, at time.Time) *sources.Document {
		content := []byte(p.Markdown)
		return &sources.Document{
			Locator:     loc,
			URI:         repo.CompareURL(a, b),
			FetchURL:    repo.CloneURL,
			Content:     content,
			Format:      "markdown",
			Digest:      domain.Digest(content),
			RetrievedAt: at,
		}
	}
	if e, ok := g.lookup(cachePath); ok {
		var p logPayload
		if err := json.Unmarshal(e.Payload, &p); err == nil {
			return doc(p, e.RetrievedAt), nil
		}
	}
	if g.opts.Offline {
		return nil, offlineErr(repo.CloneURL, "git-log result is not cached: "+rng)
	}

	commits, err := g.readLog(ctx, repo, a, b, immutable)
	if err != nil {
		return nil, err
	}
	p := logPayload{Markdown: string(renderCommits(repo, a, b, commits)), Commits: len(commits)}
	now := g.now().UTC()
	if payload, err := json.Marshal(p); err == nil {
		_ = writeEntry(cachePath, &entry{
			Kind: "log", Repo: repo.CloneURL, Request: "log " + rng,
			Immutable: immutable, RetrievedAt: now, Payload: payload,
		})
	}
	return doc(p, now), nil
}

// readLog performs the network side of fetchLog.
func (g *Git) readLog(ctx context.Context, repo Repo, a, b string, immutable bool) ([]commit, error) {
	dir := filepath.Join(g.root, "mirrors", repo.slug()+".git")
	defer g.locks.lock(dir)()

	if err := g.ensureMirror(ctx, repo, dir); err != nil {
		return nil, err
	}
	// Fetch only when needed: an endpoint that is not in the mirror yet, or
	// a mutable endpoint (a branch) whose cached result has expired.
	_, haveA := g.revParse(ctx, repo, dir, a)
	_, haveB := g.revParse(ctx, repo, dir, b)
	if !haveA || !haveB || !immutable {
		if _, err := g.run(ctx, repo.CloneURL, "git fetch", inRepo(dir, "fetch", "--quiet", "origin")); err != nil {
			return nil, err
		}
		for _, r := range []string{a, b} {
			if _, ok := g.revParse(ctx, repo, dir, r); !ok {
				return nil, &fetch.Error{URL: repo.CloneURL, Err: fetch.ErrNotFound,
					Detail: fmt.Sprintf("git-log: revision %q does not exist", r)}
			}
		}
	}
	out, err := g.run(ctx, repo.CloneURL, "git log", inRepo(dir,
		"log", "--no-merges", "--topo-order", "--reverse", "--encoding=UTF-8",
		"--format=%H"+logSep+"%s", a+".."+b, "--"))
	if err != nil {
		return nil, err
	}
	commits, err := parseLog(out)
	if err != nil {
		return nil, fmt.Errorf("git-log %s: %w", repo, err)
	}
	return commits, nil
}

// mirrorReady marks a completed mirror clone, so that an interrupted clone is
// redone instead of being mistaken for a mirror.
const mirrorReady = "ri-ready"

// ensureMirror creates the treeless bare mirror of repo in dir when it does
// not exist yet.
func (g *Git) ensureMirror(ctx context.Context, repo Repo, dir string) error {
	marker := filepath.Join(dir, mirrorReady)
	if exists(marker) {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("git-log %s: %w", repo, err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return fmt.Errorf("git-log %s: %w", repo, err)
	}
	if _, err := g.run(ctx, repo.CloneURL, "git clone", Command{
		Args: []string{"clone", "--quiet", "--bare", "--filter=tree:0", "--", repo.CloneURL, dir},
	}); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	// A bare clone has no fetch refspec; without these a later "git fetch"
	// would update nothing but FETCH_HEAD.
	for _, spec := range []string{"+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*"} {
		if _, err := g.run(ctx, repo.CloneURL, "git config", inRepo(dir, "config", "--add", "remote.origin.fetch", spec)); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(marker, []byte("treeless mirror of "+repo.CloneURL+"\n"), 0o644); err != nil {
		return fmt.Errorf("git-log %s: %w", repo, err)
	}
	return nil
}
