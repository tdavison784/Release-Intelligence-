package gitsrc

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

var (
	// commitSHARe matches a full SHA-1 or SHA-256 object name.
	commitSHARe = regexp.MustCompile(`^(?:[0-9A-Fa-f]{40}|[0-9A-Fa-f]{64})$`)
	// versionTagRe matches tags such as v1.18.0, 1.26.0, v3.6.0-rc1,
	// argo-cd-8.0.0 or cmd/ctl/v1.12.0: an optional prefix ending in a
	// separator, an optional "v", at least three numeric components and an
	// optional pre-release or build suffix.
	versionTagRe = regexp.MustCompile(`^(?:[A-Za-z0-9_./-]*[-_/])?[vV]?\d+\.\d+\.\d+(?:\.\d+)*(?:[-+][0-9A-Za-z.+-]*)?$`)
	// branchPrefixRe matches prefixes that conventionally name branches, such
	// as "release-1.31" or "release-1.30.4" (Istio has both a tag-looking
	// branch name and real tags), which must stay mutable.
	branchPrefixRe = regexp.MustCompile(`(?i)^(?:release|releases|rel|branch|hotfix|maint|maintenance|support|stable|dev|develop|feature|bugfix|backport)(?:[-_/]|$)`)
)

// IsImmutableRef reports whether ref can be assumed to name the same content
// forever, so that results addressed by it may be cached without expiry:
//
//   - a full commit SHA (40 or 64 hex digits), or
//   - a version tag: an optional prefix, an optional "v" and at least three
//     numeric components, e.g. "v1.18.0", "1.26.0", "v3.6.0-rc1",
//     "argo-cd-8.0.0".
//
// Branch names ("master", "main", "release-1.31", "release-1.30.4") and
// moving tags ("stable", "latest") are mutable. The test is deliberately
// conservative: calling an immutable ref mutable only costs a refetch after
// the TTL, whereas the opposite mistake would freeze a moving branch.
func IsImmutableRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	switch {
	case ref == "":
		return false
	case commitSHARe.MatchString(ref):
		return true
	case branchPrefixRe.MatchString(ref):
		return false
	}
	return versionTagRe.MatchString(ref)
}

// FormatFromPath derives a sources.Document format from a file name:
// ".md" gives "markdown", ".yaml"/".yml" "yaml", ".json" "json",
// ".html"/".htm" "html" and anything else "text".
func FormatFromPath(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".md", ".markdown":
		return "markdown"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".html", ".htm":
		return "html"
	}
	return "text"
}

// checkRef rejects revision strings that could be mistaken for command line
// options or that cannot be a ref name.
func checkRef(ref string) error {
	switch {
	case ref == "":
		return fmt.Errorf("empty ref")
	case strings.HasPrefix(ref, "-"):
		return fmt.Errorf("invalid ref %q: must not start with '-'", ref)
	case strings.ContainsAny(ref, " \t\r\n\x00~^:?*[\\") || strings.Contains(ref, ".."):
		return fmt.Errorf("invalid ref %q", ref)
	}
	return nil
}

// splitRange splits "A..B" into its endpoints. Symmetric ranges ("A...B")
// are rejected: the git-log adapter lists commits reachable from B but not
// from A.
func splitRange(rng string) (a, b string, err error) {
	rng = strings.TrimSpace(rng)
	if strings.Contains(rng, "...") {
		return "", "", fmt.Errorf("git-log ref %q: symmetric ranges (A...B) are not supported, use A..B", rng)
	}
	parts := strings.Split(rng, "..")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("git-log ref %q: want a range A..B", rng)
	}
	for _, p := range parts {
		if err := checkRef(p); err != nil {
			return "", "", fmt.Errorf("git-log ref %q: %w", rng, err)
		}
	}
	return parts[0], parts[1], nil
}

// cleanRepoPath normalises a repository-relative path: no leading or
// trailing slashes, no dot segments. The empty string and "." mean the
// repository root and yield "".
func cleanRepoPath(p string) (string, error) {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" || p == "." {
		return "", nil
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return "", fmt.Errorf("invalid repository path %q", p)
		}
	}
	if strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("invalid repository path %q", p)
	}
	return p, nil
}
