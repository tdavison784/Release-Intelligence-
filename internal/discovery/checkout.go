package discovery

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Runner runs an external command (git). It is injectable so tests never
// touch the network.
type Runner interface {
	// Run executes name with args in dir (current directory when ""),
	// feeding stdin when non-nil, and returns stdout.
	Run(ctx context.Context, dir string, stdin []byte, name string, args ...string) ([]byte, error)
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct {
	// Env is appended to the process environment.
	Env []string
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, dir string, stdin []byte, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "GIT_TERMINAL_PROMPT=0"), r.Env...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return stdout.Bytes(), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
	}
	return stdout.Bytes(), nil
}

// TreeInfo describes a scanned tree.
type TreeInfo struct {
	Repo RepoRef `json:"repo"`
	// Ref is the requested ref; for a default-branch checkout it is the
	// branch name.
	Ref    string `json:"ref"`
	Commit string `json:"commit,omitempty"`
	// PinRef is the ref used in evidence URIs: the tag when Ref is a tag,
	// otherwise the commit (branches move), otherwise Ref.
	PinRef      string    `json:"pinRef"`
	RefIsTag    bool      `json:"refIsTag,omitempty"`
	Partial     bool      `json:"partial,omitempty"` // contents fetched on demand
	Root        string    `json:"root,omitempty"`    // local directory (full trees)
	RetrievedAt time.Time `json:"retrievedAt"`
}

// FileEntry is one file of a tree.
type FileEntry struct {
	Path string `json:"path"` // slash-separated, relative to the tree root
	Size int64  `json:"size"`
}

// Tree is a snapshot of a repository at one ref.
type Tree interface {
	Info() TreeInfo
	// Files lists every regular file, sorted by path.
	Files() []FileEntry
	// ReadFiles returns the contents of the given paths (missing ones are
	// omitted). Implementations may fetch contents in bulk.
	ReadFiles(ctx context.Context, paths []string) (map[string][]byte, error)
}

// CheckoutOptions tunes a checkout.
type CheckoutOptions struct {
	// Partial asks for a tree whose contents are fetched on demand (used for
	// large external documentation repositories).
	Partial bool
}

// Checkout obtains trees.
type Checkout interface {
	// Checkout returns repo at ref; ref "" means the default branch.
	Checkout(ctx context.Context, repo RepoRef, ref string, opts CheckoutOptions) (Tree, error)
}

// RemoteTag is a tag as listed by git ls-remote.
type RemoteTag struct {
	Name   string `json:"name"`
	Object string `json:"object"`           // object the ref points at
	Commit string `json:"commit,omitempty"` // peeled commit for annotated tags
}

// TagLister lists remote tags and the default branch.
type TagLister interface {
	ListTags(ctx context.Context, repo RepoRef) ([]RemoteTag, error)
	DefaultBranch(ctx context.Context, repo RepoRef) (string, error)
}

// ParseLsRemote parses `git ls-remote --tags` output.
func ParseLsRemote(out []byte) []RemoteTag {
	byName := map[string]*RemoteTag{}
	var order []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 || !strings.HasPrefix(f[1], "refs/tags/") {
			continue
		}
		name := strings.TrimPrefix(f[1], "refs/tags/")
		peeled := strings.HasSuffix(name, "^{}")
		name = strings.TrimSuffix(name, "^{}")
		t, ok := byName[name]
		if !ok {
			t = &RemoteTag{Name: name}
			byName[name] = t
			order = append(order, name)
		}
		if peeled {
			t.Commit = f[0]
		} else {
			t.Object = f[0]
		}
	}
	out2 := make([]RemoteTag, 0, len(order))
	for _, n := range order {
		t := *byName[n]
		if t.Commit == "" {
			t.Commit = t.Object
		}
		out2 = append(out2, t)
	}
	return out2
}

// GitCheckout clones repositories with git into a cache directory: shallow
// (--depth 1) full clones for the product repository and blob-less partial
// clones for documentation repositories.
type GitCheckout struct {
	CacheDir string
	Runner   Runner
	Clock    func() time.Time
	// Refresh re-clones even when a cached clone exists.
	Refresh bool
}

// NewGitCheckout returns a GitCheckout using os/exec.
func NewGitCheckout(cacheDir string) *GitCheckout {
	return &GitCheckout{CacheDir: cacheDir, Runner: ExecRunner{}, Clock: time.Now}
}

var unsafePathChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeName(s string) string {
	s = unsafePathChars.ReplaceAllString(s, "_")
	if s == "" || s == "." || s == ".." {
		return "_"
	}
	return s
}

func (g *GitCheckout) now() time.Time {
	if g.Clock != nil {
		return g.Clock()
	}
	return time.Now()
}

func (g *GitCheckout) git(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error) {
	r := g.Runner
	if r == nil {
		r = ExecRunner{}
	}
	return r.Run(ctx, dir, stdin, "git", args...)
}

// Checkout implements Checkout.
func (g *GitCheckout) Checkout(ctx context.Context, repo RepoRef, ref string, opts CheckoutOptions) (Tree, error) {
	if g.CacheDir == "" {
		return nil, errors.New("discovery: GitCheckout.CacheDir is required")
	}
	mode := "full"
	if opts.Partial {
		mode = "partial"
	}
	refName := ref
	if refName == "" {
		refName = "_default"
	}
	dir := filepath.Join(g.CacheDir, safeName(repo.Host), safeName(repo.Owner), safeName(repo.Name), safeName(refName)+"."+mode)
	reuse := false
	if !g.Refresh {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			if _, err := g.git(ctx, dir, nil, "rev-parse", "--verify", "HEAD"); err == nil {
				reuse = true
			}
		}
	}
	if !reuse {
		if err := os.RemoveAll(dir); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return nil, err
		}
		args := []string{"clone", "--quiet", "--depth", "1"}
		if opts.Partial {
			args = append(args, "--filter=blob:none", "--no-checkout")
		}
		if ref != "" {
			args = append(args, "--branch", ref)
		}
		args = append(args, repo.CloneURL(), dir)
		if _, err := g.git(ctx, "", nil, args...); err != nil {
			return nil, err
		}
	}
	commitOut, err := g.git(ctx, dir, nil, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	info := TreeInfo{Repo: repo, Ref: ref, Commit: strings.TrimSpace(string(commitOut)), Partial: opts.Partial, RetrievedAt: g.now().UTC()}
	if ref == "" {
		if b, err := g.git(ctx, dir, nil, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
			info.Ref = strings.TrimSpace(string(b))
		}
	} else if tags, err := g.git(ctx, dir, nil, "tag", "--points-at", "HEAD"); err == nil {
		for _, t := range strings.Fields(string(tags)) {
			if t == ref {
				info.RefIsTag = true
			}
		}
	}
	info.PinRef = pinRef(info)
	if !opts.Partial {
		info.Root = dir
		return NewDirTree(dir, info)
	}
	return g.partialTree(ctx, dir, info)
}

func pinRef(info TreeInfo) string {
	switch {
	case info.RefIsTag:
		return info.Ref
	case info.Commit != "":
		return info.Commit
	case info.Ref != "":
		return info.Ref
	}
	return "HEAD"
}

// ListTags implements TagLister.
func (g *GitCheckout) ListTags(ctx context.Context, repo RepoRef) ([]RemoteTag, error) {
	out, err := g.git(ctx, "", nil, "ls-remote", "--tags", repo.CloneURL())
	if err != nil {
		return nil, err
	}
	return ParseLsRemote(out), nil
}

// DefaultBranch implements TagLister.
func (g *GitCheckout) DefaultBranch(ctx context.Context, repo RepoRef) (string, error) {
	out, err := g.git(ctx, "", nil, "ls-remote", "--symref", repo.CloneURL(), "HEAD")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "ref: refs/heads/") {
			f := strings.Fields(strings.TrimPrefix(line, "ref: refs/heads/"))
			if len(f) > 0 {
				return f[0], nil
			}
		}
	}
	return "", fmt.Errorf("default branch of %s not found", repo)
}

// DirTree is a Tree backed by a local directory.
type DirTree struct {
	info  TreeInfo
	root  string
	files []FileEntry
}

// NewDirTree walks root (skipping .git and symlinks).
func NewDirTree(root string, info TreeInfo) (*DirTree, error) {
	t := &DirTree{info: info, root: root}
	if t.info.Root == "" {
		t.info.Root = root
	}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		t.files = append(t.files, FileEntry{Path: filepath.ToSlash(rel), Size: fi.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(t.files, func(i, j int) bool { return t.files[i].Path < t.files[j].Path })
	return t, nil
}

// Info implements Tree.
func (t *DirTree) Info() TreeInfo { return t.info }

// Files implements Tree.
func (t *DirTree) Files() []FileEntry { return t.files }

// ReadFiles implements Tree.
func (t *DirTree) ReadFiles(ctx context.Context, paths []string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(paths))
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		clean := filepath.Clean(filepath.FromSlash(p))
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(t.root, clean))
		if err != nil {
			continue
		}
		out[p] = b
	}
	return out, nil
}

// gitTree is a partial (blob-less) clone: the listing comes from ls-tree and
// contents are fetched in bulk on demand.
type gitTree struct {
	g     *GitCheckout
	dir   string
	info  TreeInfo
	files []FileEntry
	oids  map[string]string
}

func (g *GitCheckout) partialTree(ctx context.Context, dir string, info TreeInfo) (*gitTree, error) {
	// No "-l": object sizes would make git fetch every missing blob one by
	// one. Sizes of partial trees are therefore unknown (0).
	out, err := g.git(ctx, dir, nil, "ls-tree", "-r", "-z", "HEAD")
	if err != nil {
		return nil, err
	}
	t := &gitTree{g: g, dir: dir, info: info, oids: map[string]string{}}
	for _, rec := range bytes.Split(out, []byte{0}) {
		// "<mode> SP <type> SP <oid> TAB <path>"
		tab := bytes.IndexByte(rec, '\t')
		if tab < 0 {
			continue
		}
		f := strings.Fields(string(rec[:tab]))
		if len(f) != 3 || f[1] != "blob" || f[0] == "120000" {
			continue
		}
		p := string(rec[tab+1:])
		t.files = append(t.files, FileEntry{Path: p})
		t.oids[p] = f[2]
	}
	sort.Slice(t.files, func(i, j int) bool { return t.files[i].Path < t.files[j].Path })
	return t, nil
}

func (t *gitTree) Info() TreeInfo     { return t.info }
func (t *gitTree) Files() []FileEntry { return t.files }

func (t *gitTree) ReadFiles(ctx context.Context, paths []string) (map[string][]byte, error) {
	var oids []string
	pathByOID := map[string][]string{}
	for _, p := range paths {
		oid, ok := t.oids[p]
		if !ok {
			continue
		}
		if _, dup := pathByOID[oid]; !dup {
			oids = append(oids, oid)
		}
		pathByOID[oid] = append(pathByOID[oid], p)
	}
	out := map[string][]byte{}
	if len(oids) == 0 {
		return out, nil
	}
	stdin := []byte(strings.Join(oids, "\n") + "\n")
	// Bulk prefetch, exactly as git's own promisor fetch does it; a failure
	// only means cat-file below falls back to lazy per-object fetches.
	_, _ = t.g.git(ctx, t.dir, stdin, "-c", "fetch.negotiationAlgorithm=noop", "fetch", "--quiet", "--no-tags",
		"--no-write-fetch-head", "--recurse-submodules=no", "--filter=blob:none", "--stdin", "origin")
	raw, err := t.g.git(ctx, t.dir, stdin, "cat-file", "--batch")
	if err != nil && len(raw) == 0 {
		return nil, err
	}
	for oid, data := range parseCatFileBatch(raw) {
		for _, p := range pathByOID[oid] {
			out[p] = data
		}
	}
	return out, nil
}

// parseCatFileBatch parses `git cat-file --batch` output.
func parseCatFileBatch(raw []byte) map[string][]byte {
	out := map[string][]byte{}
	for len(raw) > 0 {
		nl := bytes.IndexByte(raw, '\n')
		if nl < 0 {
			break
		}
		hdr := strings.Fields(string(raw[:nl]))
		raw = raw[nl+1:]
		if len(hdr) != 3 {
			continue // "<oid> missing"
		}
		size, err := strconv.Atoi(hdr[2])
		if err != nil || size > len(raw) {
			break
		}
		out[hdr[0]] = raw[:size]
		raw = raw[size:]
		if len(raw) > 0 && raw[0] == '\n' {
			raw = raw[1:]
		}
	}
	return out
}

// LocalCheckout serves trees from local directories (tests, or scanning an
// existing checkout). Keys are RepoRef.String().
type LocalCheckout struct {
	Dirs     map[string]string
	Tags     map[string][]string
	Branches map[string]string
	Clock    func() time.Time
}

// Checkout implements Checkout.
func (l *LocalCheckout) Checkout(ctx context.Context, repo RepoRef, ref string, opts CheckoutOptions) (Tree, error) {
	dir, ok := l.Dirs[repo.String()]
	if !ok {
		return nil, fmt.Errorf("discovery: no local directory for %s", repo)
	}
	now := time.Now()
	if l.Clock != nil {
		now = l.Clock()
	}
	info := TreeInfo{Repo: repo, Ref: ref, RetrievedAt: now.UTC(), Partial: opts.Partial}
	if ref == "" {
		info.Ref, _ = l.DefaultBranch(ctx, repo)
	}
	for _, t := range l.Tags[repo.String()] {
		if t == info.Ref {
			info.RefIsTag = true
		}
	}
	info.PinRef = pinRef(info)
	return NewDirTree(dir, info)
}

// ListTags implements TagLister.
func (l *LocalCheckout) ListTags(ctx context.Context, repo RepoRef) ([]RemoteTag, error) {
	tags, ok := l.Tags[repo.String()]
	if !ok {
		return nil, fmt.Errorf("discovery: no tags for %s", repo)
	}
	out := make([]RemoteTag, 0, len(tags))
	for _, t := range tags {
		out = append(out, RemoteTag{Name: t})
	}
	return out, nil
}

// DefaultBranch implements TagLister.
func (l *LocalCheckout) DefaultBranch(ctx context.Context, repo RepoRef) (string, error) {
	if b := l.Branches[repo.String()]; b != "" {
		return b, nil
	}
	return "main", nil
}
