package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// release is the subset of the GitHub release object used here.
type release struct {
	TagName         string     `json:"tag_name"`
	Name            string     `json:"name"`
	Body            string     `json:"body"`
	Draft           bool       `json:"draft"`
	Prerelease      bool       `json:"prerelease"`
	CreatedAt       *time.Time `json:"created_at"`
	PublishedAt     *time.Time `json:"published_at"`
	HTMLURL         string     `json:"html_url"`
	TargetCommitish string     `json:"target_commitish"`
}

var commitSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Releases implements sources.VersionLister and sources.DocumentFetcher for
// the github-releases locator kind.
type Releases struct {
	c *Client
}

// NewReleases returns the github-releases adapter.
func NewReleases(c *Client) *Releases { return &Releases{c: c} }

// ListReleases lists the published releases of loc.Repository ("owner/name"),
// all pages of GET /repos/{owner}/{repo}/releases?per_page=100, in the order
// the API returns them (newest first). Drafts are skipped; each ReleaseRef
// carries the prerelease flag, the publication time (the creation time when
// GitHub reports none), the release page as URL and a git-ref evidence record
// pointing at that page. Commit is set only when target_commitish is a full
// commit id, since it is usually a branch name.
func (r *Releases) ListReleases(ctx context.Context, loc catalog.Locator) ([]sources.ReleaseRef, error) {
	owner, name, err := parseRepository(loc.Repository)
	if err != nil {
		return nil, err
	}
	first := r.c.endpoint("/repos/"+owner+"/"+name+"/releases", url.Values{"per_page": {fmt.Sprint(perPage)}})

	var refs []sources.ReleaseRef
	seen := map[string]bool{}
	err = r.c.paginate(ctx, first, func(doc *fetch.Document) error {
		var page []release
		if err := json.Unmarshal(doc.Body, &page); err != nil {
			return fmt.Errorf("github-releases %s/%s: decoding %s: %w", owner, name, doc.URL, err)
		}
		for _, rel := range page {
			if rel.Draft || rel.TagName == "" || seen[rel.TagName] {
				continue
			}
			seen[rel.TagName] = true
			refs = append(refs, releaseRef(owner, name, rel, doc.RetrievedAt))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// releaseRef converts an API release into a ReleaseRef.
func releaseRef(owner, name string, rel release, retrievedAt time.Time) sources.ReleaseRef {
	published := rel.PublishedAt
	if published == nil {
		published = rel.CreatedAt
	}
	htmlURL := rel.HTMLURL
	if htmlURL == "" {
		htmlURL = "https://github.com/" + owner + "/" + name + "/releases/tag/" + escapePath(rel.TagName)
	}
	excerpt := "release " + rel.TagName
	if published != nil {
		excerpt += " published " + published.UTC().Format(time.RFC3339)
	}
	if rel.Prerelease {
		excerpt += " (prerelease)"
	}
	ref := sources.ReleaseRef{
		Tag:        rel.TagName,
		Prerelease: rel.Prerelease,
		URL:        htmlURL,
		Evidence: domain.NewEvidence(domain.EvidenceGitRef, "", htmlURL, "refs/tags/"+rel.TagName,
			excerpt, "", retrievedAt),
	}
	if published != nil {
		t := published.UTC()
		ref.PublishedAt = &t
	}
	if commitSHARe.MatchString(rel.TargetCommitish) {
		ref.Commit = rel.TargetCommitish
	}
	return ref
}

// FetchDocument returns the body of the release of the tag loc.Ref
// (GET /repos/{owner}/{repo}/releases/tags/{tag}) as a markdown document. The
// body is returned verbatim (GitHub bodies often use CRLF line endings), the
// URI is the release page and the digest covers the body, not the JSON
// envelope. The response is cached with the client's TTL and never as
// immutable: release bodies are edited after publication.
func (r *Releases) FetchDocument(ctx context.Context, loc catalog.Locator) (*sources.Document, error) {
	owner, name, err := parseRepository(loc.Repository)
	if err != nil {
		return nil, err
	}
	tag := strings.TrimSpace(loc.Ref)
	if tag == "" {
		return nil, fmt.Errorf("github-releases %s/%s: ref (the release tag) is required", owner, name)
	}
	api := r.c.endpoint("/repos/"+owner+"/"+name+"/releases/tags/"+escapePath(tag), nil)
	doc, err := r.c.get(ctx, api)
	if err != nil {
		return nil, err
	}
	var rel release
	if err := json.Unmarshal(doc.Body, &rel); err != nil {
		return nil, fmt.Errorf("github-releases %s/%s: decoding %s: %w", owner, name, api, err)
	}
	if rel.Draft {
		return nil, &fetch.Error{URL: api, Err: fetch.ErrNotFound, Detail: "release is a draft"}
	}
	uri := rel.HTMLURL
	if uri == "" {
		uri = "https://github.com/" + owner + "/" + name + "/releases/tag/" + escapePath(tag)
	}
	content := []byte(rel.Body)
	return &sources.Document{
		Locator:     loc,
		URI:         uri,
		FetchURL:    api,
		Content:     content,
		Format:      "markdown",
		Digest:      domain.Digest(content),
		RetrievedAt: doc.RetrievedAt,
	}, nil
}
