package discovery

import (
	"fmt"
	"net/url"
	"strings"
)

// RepoRef identifies a git repository hosted at Host/Owner/Name.
type RepoRef struct {
	Host  string `json:"host"`
	Owner string `json:"owner"`
	Name  string `json:"name"`
	// URL overrides the clone URL (e.g. a file:// URL in tests).
	URL string `json:"url,omitempty"`
}

// ParseRepo accepts "owner/name" (GitHub), "host/owner/name",
// "https://host/owner/name(.git)" and "git@host:owner/name.git".
func ParseRepo(s string) (RepoRef, error) {
	s = strings.TrimSpace(s)
	orig := s
	if strings.HasPrefix(s, "git@") {
		s = strings.TrimPrefix(s, "git@")
		s = strings.Replace(s, ":", "/", 1)
	}
	if u, err := url.Parse(s); err == nil && u.Scheme != "" && u.Host != "" {
		s = u.Host + u.Path
	}
	s = strings.TrimSuffix(strings.Trim(s, "/"), ".git")
	parts := strings.Split(s, "/")
	switch {
	case len(parts) == 2 && !strings.Contains(parts[0], "."):
		return RepoRef{Host: "github.com", Owner: parts[0], Name: parts[1]}, nil
	case len(parts) >= 3 && strings.Contains(parts[0], "."):
		return RepoRef{Host: strings.ToLower(parts[0]), Owner: strings.Join(parts[1:len(parts)-1], "/"), Name: parts[len(parts)-1]}, nil
	}
	return RepoRef{}, fmt.Errorf("cannot parse repository %q (want owner/name or host/owner/name)", orig)
}

// IsZero reports whether the reference is unset.
func (r RepoRef) IsZero() bool { return r.Owner == "" && r.Name == "" }

// String returns host/owner/name, the form used by git-based locators.
func (r RepoRef) String() string { return r.Host + "/" + r.Owner + "/" + r.Name }

// Slug returns owner/name, the form used by github-* locators.
func (r RepoRef) Slug() string { return r.Owner + "/" + r.Name }

// IsGitHub reports whether the repository lives on github.com.
func (r RepoRef) IsGitHub() bool { return r.Host == "github.com" }

// CloneURL returns the URL used by git.
func (r RepoRef) CloneURL() string {
	if r.URL != "" {
		return r.URL
	}
	return "https://" + r.String()
}

// BlobURL returns a human-facing URL of path at ref.
func (r RepoRef) BlobURL(ref, path string) string {
	switch {
	case r.IsGitHub():
		return fmt.Sprintf("https://github.com/%s/blob/%s/%s", r.Slug(), ref, path)
	case strings.Contains(r.Host, "gitlab"):
		return fmt.Sprintf("https://%s/-/blob/%s/%s", r.String(), ref, path)
	case r.Host == "" || r.Host == "local":
		return "file://" + r.URL + "/" + path
	}
	return fmt.Sprintf("https://%s/blob/%s/%s", r.String(), ref, path)
}

// TagURL returns a human-facing URL of a tag.
func (r RepoRef) TagURL(tag string) string {
	if r.IsGitHub() {
		return fmt.Sprintf("https://github.com/%s/releases/tag/%s", r.Slug(), tag)
	}
	return r.CloneURL() + "#refs/tags/" + tag
}

// Same reports whether two references name the same repository.
func (r RepoRef) Same(o RepoRef) bool {
	return strings.EqualFold(r.Host, o.Host) && strings.EqualFold(r.Owner, o.Owner) && strings.EqualFold(r.Name, o.Name)
}
