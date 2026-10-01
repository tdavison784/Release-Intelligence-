package gitsrc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// treeEntry is one regular file of a git tree.
type treeEntry struct {
	Path string
	OID  string
}

// parseLsTree parses the output of "git ls-tree -r -z": NUL-terminated
// records of "<mode> SP <type> SP <oid> TAB <path>". Only regular files
// (blobs with mode 100644 or 100755) are returned; symlinks and submodules
// are skipped. The result is sorted by path.
func parseLsTree(out []byte) ([]treeEntry, error) {
	var entries []treeEntry
	for _, rec := range bytes.Split(out, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		meta, p, ok := strings.Cut(string(rec), "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || p == "" {
			return nil, fmt.Errorf("malformed ls-tree record %q", rec)
		}
		mode, typ, oid := fields[0], fields[1], fields[2]
		if typ != "blob" || (mode != "100644" && mode != "100755") {
			continue
		}
		entries = append(entries, treeEntry{Path: p, OID: oid})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// selectEntries applies the repo-dir filters to the files found at Ref:
// files that also exist at the base ref (when given) are dropped, and the
// glob is matched against the base name of each remaining file.
func selectEntries(atRef, atBase []treeEntry, glob string) ([]treeEntry, error) {
	if glob == "" {
		glob = "*"
	}
	skip := make(map[string]bool, len(atBase))
	for _, e := range atBase {
		skip[e.Path] = true
	}
	var out []treeEntry
	for _, e := range atRef {
		if skip[e.Path] {
			continue
		}
		ok, err := path.Match(glob, path.Base(e.Path))
		if err != nil {
			return nil, fmt.Errorf("invalid glob %q: %w", glob, err)
		}
		if ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// parseCatFileBatch parses the output of "git cat-file --batch" for the
// given object ids (which must be the full ids that were fed to the command,
// in order): "<oid> SP <type> SP <size> LF <content> LF" per object.
func parseCatFileBatch(out []byte, oids []string) (map[string][]byte, error) {
	r := bufio.NewReader(bytes.NewReader(out))
	blobs := make(map[string][]byte, len(oids))
	for _, want := range oids {
		header, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("cat-file: reading header of %s: %w", want, err)
		}
		fields := strings.Fields(header)
		if len(fields) == 2 && fields[1] == "missing" {
			return nil, fmt.Errorf("cat-file: object %s is missing", want)
		}
		if len(fields) != 3 || fields[0] != want {
			return nil, fmt.Errorf("cat-file: unexpected header %q for %s", strings.TrimSpace(header), want)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 {
			return nil, fmt.Errorf("cat-file: bad size in header %q", strings.TrimSpace(header))
		}
		buf := make([]byte, size)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("cat-file: reading %s: %w", want, err)
		}
		if _, err := r.ReadByte(); err != nil { // the LF after the content
			return nil, fmt.Errorf("cat-file: reading %s: %w", want, err)
		}
		blobs[want] = buf
	}
	return blobs, nil
}

// refCandidates lists the remote ref names a user-supplied ref may denote, in
// the order in which they are tried. Version tags prefer tags and everything
// else prefers branches, which makes the outcome deterministic where git's
// own short-name resolution would warn about ambiguity.
func refCandidates(ref string) []string {
	switch {
	case commitSHARe.MatchString(ref), ref == "HEAD", strings.HasPrefix(ref, "refs/"):
		return []string{ref}
	case IsImmutableRef(ref):
		return []string{"refs/tags/" + ref, "refs/heads/" + ref}
	}
	return []string{"refs/heads/" + ref, "refs/tags/" + ref}
}

// dirFile is one cached file of a repo-dir result.
type dirFile struct {
	Path    string `json:"path"`
	OID     string `json:"oid"`
	Content []byte `json:"content"` // base64 in JSON
}

// dirPayload is the cached form of a repo-dir result.
type dirPayload struct {
	Commit     string    `json:"commit"`
	BaseCommit string    `json:"baseCommit,omitempty"`
	Files      []dirFile `json:"files"`
}

// fetchDir implements the repo-dir locator kind: every file below loc.Path
// whose base name matches loc.Glob (default "*"), at loc.Ref, recursing into
// subdirectories, sorted by path. When loc.BaseRef is set only files that
// exist at Ref but not at BaseRef are returned.
//
// Online, the files are read from a shallow, blobless repository kept under
// <CacheDir>/git/work: only the commit and trees of Ref (and BaseRef) are
// fetched, then exactly the blobs that are needed in one batch. The complete
// result, file contents included, is cached as one entry; this entry is what
// offline mode replays. It never expires when Ref (and BaseRef) are
// immutable, see IsImmutableRef.
func (g *Git) fetchDir(ctx context.Context, loc catalog.Locator) ([]sources.Document, error) {
	repo, err := ParseRepo(loc.Repository)
	if err != nil {
		return nil, fmt.Errorf("repo-dir: %w", err)
	}
	dirPath, err := cleanRepoPath(loc.Path)
	if err != nil {
		return nil, fmt.Errorf("repo-dir %s: %w", repo, err)
	}
	ref := strings.TrimSpace(loc.Ref)
	if ref == "" {
		ref = defaultRef
	}
	baseRef := strings.TrimSpace(loc.BaseRef)
	for _, r := range []string{ref, baseRef} {
		if r == "" {
			continue
		}
		if err := checkRef(r); err != nil {
			return nil, fmt.Errorf("repo-dir %s: %w", repo, err)
		}
	}
	glob := loc.Glob
	if glob == "" {
		glob = "*"
	}
	if _, err := path.Match(glob, ""); err != nil {
		return nil, fmt.Errorf("repo-dir %s: invalid glob %q: %w", repo, glob, err)
	}

	immutable := IsImmutableRef(ref) && (baseRef == "" || IsImmutableRef(baseRef))
	describe := fmt.Sprintf("dir %s ref=%s base=%s glob=%s", dirPath, ref, baseRef, glob)
	cachePath := filepath.Join(g.root, "dir", repo.slug(), requestKey(repo.CloneURL, ref, baseRef, dirPath, glob)+".json.gz")
	if e, ok := g.lookup(cachePath); ok {
		var p dirPayload
		if err := json.Unmarshal(e.Payload, &p); err == nil {
			return dirDocuments(repo, loc, ref, p, e.RetrievedAt), nil
		}
	}
	if g.opts.Offline {
		return nil, offlineErr(repo.CloneURL, "repo-dir result is not cached: "+describe)
	}

	p, err := g.readDir(ctx, repo, ref, baseRef, dirPath, glob)
	if err != nil {
		return nil, err
	}
	now := g.now().UTC()
	payload, err := json.Marshal(p)
	if err == nil {
		_ = writeEntry(cachePath, &entry{
			Kind: "dir", Repo: repo.CloneURL, Request: describe,
			Immutable: immutable, RetrievedAt: now, Payload: payload,
		})
	}
	return dirDocuments(repo, loc, ref, *p, now), nil
}

// dirDocuments turns a cached or fresh payload into documents with blob URIs
// pinned to ref.
func dirDocuments(repo Repo, loc catalog.Locator, ref string, p dirPayload, at time.Time) []sources.Document {
	docs := make([]sources.Document, 0, len(p.Files))
	for _, f := range p.Files {
		docs = append(docs, sources.Document{
			Locator:     loc,
			URI:         repo.BlobURL(ref, f.Path),
			FetchURL:    repo.CloneURL,
			Path:        f.Path,
			Content:     f.Content,
			Format:      FormatFromPath(f.Path),
			Digest:      domain.Digest(f.Content),
			RetrievedAt: at,
		})
	}
	return docs
}

// readDir performs the network side of fetchDir.
func (g *Git) readDir(ctx context.Context, repo Repo, ref, baseRef, dirPath, glob string) (*dirPayload, error) {
	dir := filepath.Join(g.root, "work", repo.slug()+".git")
	defer g.locks.lock(dir)()
	if err := g.ensureWork(ctx, repo, dir); err != nil {
		return nil, err
	}

	local, commit, err := g.fetchRef(ctx, repo, dir, ref)
	if err != nil {
		return nil, err
	}
	atRef, err := g.lsTree(ctx, repo, dir, local, dirPath)
	if err != nil {
		return nil, err
	}
	if len(atRef) == 0 {
		return nil, &fetch.Error{URL: repo.CloneURL, Err: fetch.ErrNotFound,
			Detail: fmt.Sprintf("repo-dir: no files under %q at %s", dirPath, ref)}
	}
	p := &dirPayload{Commit: commit}
	var atBase []treeEntry
	if baseRef != "" {
		baseLocal, baseCommit, err := g.fetchRef(ctx, repo, dir, baseRef)
		if err != nil {
			return nil, err
		}
		p.BaseCommit = baseCommit
		if atBase, err = g.lsTree(ctx, repo, dir, baseLocal, dirPath); err != nil {
			return nil, err
		}
	}
	selected, err := selectEntries(atRef, atBase, glob)
	if err != nil {
		return nil, fmt.Errorf("repo-dir %s: %w", repo, err)
	}
	blobs, err := g.readBlobs(ctx, repo, dir, selected)
	if err != nil {
		return nil, err
	}
	p.Files = make([]dirFile, 0, len(selected))
	for _, e := range selected {
		p.Files = append(p.Files, dirFile{Path: e.Path, OID: e.OID, Content: blobs[e.OID]})
	}
	return p, nil
}

// ensureWork makes sure dir is an initialised bare repository whose remote
// "origin" is the repository being read.
func (g *Git) ensureWork(ctx context.Context, repo Repo, dir string) error {
	if !exists(filepath.Join(dir, "HEAD")) {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return fmt.Errorf("repo-dir %s: %w", repo, err)
		}
		if _, err := g.run(ctx, repo.CloneURL, "git init", Command{
			Args: []string{"-c", "init.defaultBranch=main", "init", "--bare", "--quiet", dir},
		}); err != nil {
			return err
		}
	}
	_, err := g.run(ctx, repo.CloneURL, "git config", inRepo(dir, "config", "remote.origin.url", repo.CloneURL))
	return err
}

// fetchRef makes ref available as a local ref below refs/ri and returns that
// local name and the commit it points at. Only the commit and its trees are
// fetched (depth 1, no blobs). Immutable refs that are already present are
// not fetched again.
func (g *Git) fetchRef(ctx context.Context, repo Repo, dir, ref string) (local, commit string, err error) {
	local = "refs/ri/" + requestKey(ref)
	if IsImmutableRef(ref) {
		if sha, ok := g.revParse(ctx, repo, dir, local); ok {
			return local, sha, nil
		}
	}
	var lastErr error
	for _, src := range refCandidates(ref) {
		_, err := g.run(ctx, repo.CloneURL, "git fetch "+src, inRepo(dir,
			"fetch", "--quiet", "--no-tags", "--depth=1", "--filter=blob:none",
			"origin", "+"+src+":"+local))
		if err == nil {
			sha, ok := g.revParse(ctx, repo, dir, local)
			if !ok {
				return "", "", fmt.Errorf("repo-dir %s: fetched %s but cannot resolve it", repo, ref)
			}
			return local, sha, nil
		}
		lastErr = err
		if !errors.Is(err, fetch.ErrNotFound) {
			return "", "", err
		}
	}
	return "", "", lastErr
}

// revParse resolves a local ref to a commit id.
func (g *Git) revParse(ctx context.Context, repo Repo, dir, ref string) (string, bool) {
	out, err := g.run(ctx, repo.CloneURL, "git rev-parse", inRepo(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}"))
	if err != nil {
		return "", false
	}
	sha := strings.TrimSpace(string(out))
	return sha, sha != ""
}

// lsTree lists the regular files below dirPath at a local ref.
func (g *Git) lsTree(ctx context.Context, repo Repo, dir, local, dirPath string) ([]treeEntry, error) {
	args := []string{"ls-tree", "-r", "-z", local}
	if dirPath != "" {
		args = append(args, "--", dirPath)
	}
	out, err := g.run(ctx, repo.CloneURL, "git ls-tree", inRepo(dir, args...))
	if err != nil {
		return nil, err
	}
	entries, err := parseLsTree(out)
	if err != nil {
		return nil, fmt.Errorf("repo-dir %s: %w", repo, err)
	}
	return entries, nil
}

// prefetchChunk bounds how many blob ids are requested in one fetch.
const prefetchChunk = 500

// readBlobs returns the contents of the selected files keyed by blob id.
// Missing blobs are first requested in batches (one round trip per chunk
// instead of one per file); this is an optimisation only, "git cat-file"
// fetches whatever is still missing on demand.
func (g *Git) readBlobs(ctx context.Context, repo Repo, dir string, entries []treeEntry) (map[string][]byte, error) {
	seen := map[string]bool{}
	var oids []string
	for _, e := range entries {
		if !seen[e.OID] {
			seen[e.OID] = true
			oids = append(oids, e.OID)
		}
	}
	if len(oids) == 0 {
		return map[string][]byte{}, nil
	}
	for start := 0; start < len(oids); start += prefetchChunk {
		end := min(start+prefetchChunk, len(oids))
		// Mirrors the invocation git itself uses to fill in missing objects
		// of a partial clone. Failure is not an error, see above.
		_, _ = g.runner.Run(ctx, Command{
			Args: []string{"--git-dir", dir, "-c", "fetch.negotiationAlgorithm=noop", "fetch", "origin",
				"--quiet", "--no-tags", "--no-write-fetch-head", "--recurse-submodules=no",
				"--filter=blob:none", "--stdin"},
			Stdin: []byte(strings.Join(oids[start:end], "\n") + "\n"),
		})
	}
	out, err := g.run(ctx, repo.CloneURL, "git cat-file", Command{
		Args:  []string{"--git-dir", dir, "cat-file", "--batch"},
		Stdin: []byte(strings.Join(oids, "\n") + "\n"),
	})
	if err != nil {
		return nil, err
	}
	blobs, err := parseCatFileBatch(out, oids)
	if err != nil {
		return nil, fmt.Errorf("repo-dir %s: %w", repo, err)
	}
	return blobs, nil
}
