package gitsrc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/fetch"
)

// flavor identifies the URL layout of a hosting platform's web UI.
type flavor int

const (
	flavorOther flavor = iota
	flavorGitHub
	flavorGitLab
)

// Repo is a parsed repository reference.
type Repo struct {
	// Host is the lower-case host name ("github.com"); empty for file:// URLs.
	Host string
	// Path is the repository path on the host without ".git" ("cert-manager/cert-manager").
	// GitLab sub-groups yield more than two segments.
	Path string
	// CloneURL is what is handed to git.
	CloneURL string
	// WebURL is the human-facing base URL ("https://github.com/cert-manager/cert-manager").
	WebURL string

	flavor flavor
}

var (
	segmentRe = regexp.MustCompile(`^[A-Za-z0-9._~][A-Za-z0-9._~-]*$`)
	unsafeRe  = regexp.MustCompile(`[^A-Za-z0-9._-]`)
)

// validSegment reports whether s is acceptable as one path segment of a
// repository path. Dot segments are rejected; names such as ".github" are fine.
func validSegment(s string) bool {
	return s != "." && s != ".." && segmentRe.MatchString(s)
}

// ParseRepo normalises a repository string. It accepts
//
//	host/owner/name                 github.com/cert-manager/cert-manager
//	owner/name                      cert-manager/cert-manager (github.com is assumed)
//	https://host/owner/name[.git]   a full URL (also http, ssh, git and file schemes)
//
// The first segment of the host form must contain a dot (or be "localhost")
// to be recognised as a host. URLs that embed a password are rejected:
// credentials belong in a git credential helper, not in a catalog or a cache
// path.
func ParseRepo(s string) (Repo, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Repo{}, errors.New("empty repository")
	}
	if strings.ContainsAny(s, " \t\r\n\x00") {
		return Repo{}, fmt.Errorf("invalid repository %q: contains whitespace", s)
	}
	if strings.HasPrefix(s, "-") {
		return Repo{}, fmt.Errorf("invalid repository %q: must not start with '-'", s)
	}
	if strings.Contains(s, "://") {
		return parseRepoURL(s)
	}
	segs := strings.Split(strings.Trim(s, "/"), "/")
	for _, seg := range segs {
		if !validSegment(seg) {
			return Repo{}, fmt.Errorf("invalid repository %q: bad path segment %q", s, seg)
		}
	}
	host := "github.com"
	switch {
	case len(segs) == 2 && !strings.Contains(segs[0], "."):
		// owner/name
	case len(segs) >= 3 && (strings.Contains(segs[0], ".") || segs[0] == "localhost"):
		host, segs = strings.ToLower(segs[0]), segs[1:]
	default:
		return Repo{}, fmt.Errorf("invalid repository %q: want host/owner/name, owner/name or a URL", s)
	}
	return checkLayout(newRepo(host, strings.Join(segs, "/")), s)
}

func parseRepoURL(s string) (Repo, error) {
	u, err := url.Parse(s)
	if err != nil {
		return Repo{}, fmt.Errorf("invalid repository URL %q: %w", s, err)
	}
	if _, hasPw := u.User.Password(); hasPw {
		return Repo{}, fmt.Errorf("invalid repository URL %q: embedded passwords are not accepted; use a git credential helper", u.Redacted())
	}
	switch u.Scheme {
	case "https", "http", "ssh", "git":
		host := strings.ToLower(u.Host)
		p := strings.Trim(u.Path, "/")
		if host == "" || p == "" {
			return Repo{}, fmt.Errorf("invalid repository URL %q: want scheme://host/path", s)
		}
		for _, seg := range strings.Split(p, "/") {
			if !validSegment(seg) {
				return Repo{}, fmt.Errorf("invalid repository URL %q: bad path segment %q", s, seg)
			}
		}
		r := newRepo(host, p)
		r.CloneURL = strings.TrimRight(s, "/")
		return checkLayout(r, s)
	case "file":
		if u.Path == "" {
			return Repo{}, fmt.Errorf("invalid repository URL %q: missing path", s)
		}
		p := strings.TrimRight(u.Path, "/")
		return Repo{Path: strings.TrimSuffix(strings.TrimPrefix(p, "/"), ".git"), CloneURL: strings.TrimRight(s, "/"), WebURL: strings.TrimRight(s, "/")}, nil
	default:
		return Repo{}, fmt.Errorf("invalid repository URL %q: unsupported scheme %q", s, u.Scheme)
	}
}

// checkLayout rejects paths that cannot name a repository on the host: on
// github.com a repository is exactly owner/name, so a pasted ".../tree/main"
// or ".../issues/1" URL is an error rather than a silently wrong clone URL.
func checkLayout(r Repo, orig string) (Repo, error) {
	if r.Host == "github.com" && strings.Count(r.Path, "/") != 1 {
		return Repo{}, fmt.Errorf("invalid repository %q: github.com repositories are owner/name", orig)
	}
	return r, nil
}

// newRepo builds a Repo for an https-reachable host.
func newRepo(host, p string) Repo {
	p = strings.TrimSuffix(p, ".git")
	r := Repo{Host: host, Path: p, WebURL: "https://" + host + "/" + p, CloneURL: "https://" + host + "/" + p}
	switch {
	case host == "github.com" || strings.HasPrefix(host, "github."):
		r.flavor = flavorGitHub
	case host == "gitlab.com" || strings.HasPrefix(host, "gitlab."):
		r.flavor = flavorGitLab
	}
	return r
}

// String returns "host/path" (or the clone URL for hostless repositories).
func (r Repo) String() string {
	if r.Host == "" {
		return r.CloneURL
	}
	return r.Host + "/" + r.Path
}

// slug is a file-system friendly, collision-free identifier of the repository,
// used for cache paths.
func (r Repo) slug() string {
	sum := sha256.Sum256([]byte(r.CloneURL))
	name := r.Host + "_" + strings.ReplaceAll(r.Path, "/", "_")
	name = unsafeRe.ReplaceAllString(name, "_")
	if len(name) > 60 {
		name = name[len(name)-60:]
	}
	return strings.Trim(name, "._") + "-" + hex.EncodeToString(sum[:])[:10]
}

// TagURL returns the human-facing URL of a tag. For hosts whose layout is
// unknown it is the repository's web URL.
func (r Repo) TagURL(tag string) string {
	switch r.flavor {
	case flavorGitHub:
		return r.WebURL + "/releases/tag/" + escapePath(tag)
	case flavorGitLab:
		return r.WebURL + "/-/tags/" + escapePath(tag)
	}
	return r.WebURL
}

// BlobURL returns the human-facing URL of a file pinned to a ref. For hosts
// whose layout is unknown it is the repository's web URL.
func (r Repo) BlobURL(ref, p string) string {
	switch r.flavor {
	case flavorGitHub:
		return r.WebURL + "/blob/" + escapePath(ref) + "/" + escapePath(p)
	case flavorGitLab:
		return r.WebURL + "/-/blob/" + escapePath(ref) + "/" + escapePath(p)
	}
	return r.WebURL
}

// CommitURL returns the human-facing URL of a commit, or "" when the host's
// layout is unknown.
func (r Repo) CommitURL(sha string) string {
	switch r.flavor {
	case flavorGitHub:
		return r.WebURL + "/commit/" + sha
	case flavorGitLab:
		return r.WebURL + "/-/commit/" + sha
	}
	return ""
}

// CompareURL returns the human-facing URL comparing two refs (a...b). For
// hosts whose layout is unknown it is the repository's web URL.
func (r Repo) CompareURL(a, b string) string {
	switch r.flavor {
	case flavorGitHub:
		return r.WebURL + "/compare/" + escapePath(a) + "..." + escapePath(b)
	case flavorGitLab:
		return r.WebURL + "/-/compare/" + escapePath(a) + "..." + escapePath(b)
	}
	return r.WebURL
}

// RawURL returns the URL serving the raw bytes of a file at a ref. Only
// github.com and gitlab.com are supported; any other host yields an error
// wrapping fetch.ErrUnavailable.
func (r Repo) RawURL(ref, p string) (string, error) {
	switch r.Host {
	case "github.com":
		return "https://raw.githubusercontent.com/" + r.Path + "/" + escapePath(ref) + "/" + escapePath(p), nil
	case "gitlab.com":
		return "https://gitlab.com/" + r.Path + "/-/raw/" + escapePath(ref) + "/" + escapePath(p), nil
	}
	return "", fmt.Errorf("repo-file: unsupported host %q for raw file access: %w", r.Host, fetch.ErrUnavailable)
}

// escapePath escapes every segment of a slash-separated path and keeps the
// slashes themselves.
func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}
