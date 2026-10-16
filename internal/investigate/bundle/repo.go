package bundle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// repoRef pins a GitOps repository: the git HEAD and dirty state when the
// path is a work tree, and always a tree digest over the collected files, so
// a directory without git is still pinned (DESIGN.md §2.3).
func repoRef(ctx context.Context, path string, files []fileDigest) *domain.RepoRef {
	ref := &domain.RepoRef{Path: path, TreeDigest: treeDigest(files)}
	if _, err := exec.LookPath("git"); err != nil {
		return ref
	}
	run := func(args ...string) (string, bool) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return "", false
		}
		return strings.TrimSpace(out.String()), true
	}
	if top, ok := run("rev-parse", "--show-toplevel"); !ok || top == "" {
		return ref
	}
	if c, ok := run("rev-parse", "HEAD"); ok {
		ref.Commit = c
	}
	// Dirty: tracked changes or untracked files below the repository path.
	if st, ok := run("status", "--porcelain", "--", "."); ok && st != "" {
		ref.Dirty = true
	}
	if u, ok := run("config", "--get", "remote.origin.url"); ok {
		ref.URL = stripCredentials(u)
	}
	return ref
}

// stripCredentials removes a userinfo section from a remote URL.
func stripCredentials(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		rest := u[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 && !strings.Contains(rest[:at], "/") {
			return u[:i+3] + rest[at+1:]
		}
	}
	return u
}

// fileDigest is one collected file: its logical path and the digest of the
// (redacted) bytes stored for it.
type fileDigest struct{ Path, Digest string }

// treeDigest is sha256 over the sorted (path, digest) list.
func treeDigest(files []fileDigest) string {
	fs := append([]fileDigest(nil), files...)
	sort.Slice(fs, func(i, j int) bool { return fs[i].Path < fs[j].Path })
	h := sha256.New()
	for _, f := range fs {
		h.Write([]byte(filepath.ToSlash(f.Path)))
		h.Write([]byte{0})
		h.Write([]byte(f.Digest))
		h.Write([]byte{'\n'})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
