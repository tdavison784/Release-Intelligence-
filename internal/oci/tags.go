package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// noiseTagRes match tags that are registry by-products rather than releases.
var noiseTagRes = []*regexp.Regexp{
	// cosign signatures, attestations and SBOMs, and the OCI 1.1 referrers
	// fallback tag: sha256-<hex>[.sig|.att|.sbom|...]
	regexp.MustCompile(`^sha256-[0-9a-fA-F]{32,128}(?:\.[A-Za-z0-9]+)?$`),
	// tags that are (or end in) a full commit SHA: "9fceb02d0ae598e95dc970b74767f19372d61af8",
	// "1.31.0-alpha.9fceb02d0ae598e95dc970b74767f19372d61af8"
	regexp.MustCompile(`(?:^|[-._])[0-9a-f]{40}(?:[0-9a-f]{24})?$`),
}

// IsNoiseTag reports whether tag is registry noise rather than a release tag:
// cosign signature/attestation/SBOM tags ("sha256-<hex>.sig", ".att",
// ".sbom") and tags that are, or end in, a full 40-hex commit SHA (daily
// builds such as "1.31.0-alpha.<sha>").
func IsNoiseTag(tag string) bool {
	for _, re := range noiseTagRes {
		if re.MatchString(tag) {
			return true
		}
	}
	return false
}

// FilterNoiseTags returns tags without the noise tags (see IsNoiseTag),
// preserving order. The input slice is not modified.
func FilterNoiseTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if !IsNoiseTag(t) {
			out = append(out, t)
		}
	}
	return out
}

// tagPage is one fetched page of a tag listing.
type tagPage struct {
	URL       string
	Digest    string
	Retrieved time.Time
	Tags      []string
}

// listPages fetches the whole tag listing of repo, following Link
// pagination. Pages are returned in order.
func (a *Adapter) listPages(ctx context.Context, repo Repository) ([]tagPage, error) {
	var pages []tagPage
	next := repo.tagsURL(tagsPageSize)
	seen := map[string]bool{}
	for len(pages) < maxTagPages && next != "" && !seen[next] {
		seen[next] = true
		doc, err := a.get(ctx, repo, fetch.Request{
			URL:    next,
			Header: accept("application/json"),
			TTL:    a.tagsTTL,
		})
		if err != nil {
			return nil, fmt.Errorf("oci: list tags of %s: %w", repo.Given, err)
		}
		var body struct {
			Tags []string `json:"tags"`
		}
		if err := json.Unmarshal(doc.Body, &body); err != nil {
			return nil, fmt.Errorf("oci: list tags of %s: decode %s: %w", repo.Given, next, err)
		}
		pages = append(pages, tagPage{URL: next, Digest: doc.Digest, Retrieved: doc.RetrievedAt, Tags: body.Tags})
		next, err = nextLink(next, doc.Header.Get("Link"), repo.APIBase)
		if err != nil {
			return nil, fmt.Errorf("oci: list tags of %s: %w", repo.Given, err)
		}
	}
	return pages, nil
}

// linkNextRe extracts the target of a rel="next" Link header
// ("</v2/x/tags/list?last=b&n=1000>; rel=\"next\"").
var linkNextRe = regexp.MustCompile(`<([^>]*)>\s*;[^,]*\brel="?next"?`)

// nextLink resolves the rel="next" target of a Link header against the
// current page URL. It returns "" when there is no next page and refuses
// targets on another host than apiBase.
func nextLink(current, link, apiBase string) (string, error) {
	if link == "" {
		return "", nil
	}
	m := linkNextRe.FindStringSubmatch(link)
	if m == nil {
		return "", nil
	}
	base, err := url.Parse(current)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(m[1])
	if err != nil {
		return "", fmt.Errorf("bad Link header %q: %w", link, err)
	}
	abs := base.ResolveReference(ref)
	root, err := url.Parse(apiBase)
	if err == nil && abs.Host != root.Host {
		return "", fmt.Errorf("pagination link %q points to another host", m[1])
	}
	return abs.String(), nil
}

// ListTags returns every tag of repository, in the order the registry lists
// them, de-duplicated. No filtering is applied.
func (a *Adapter) ListTags(ctx context.Context, repository string) ([]string, error) {
	repo, err := a.repository(repository)
	if err != nil {
		return nil, err
	}
	pages, err := a.listPages(ctx, repo)
	if err != nil {
		return nil, err
	}
	var tags []string
	seen := map[string]bool{}
	for _, p := range pages {
		for _, t := range p.Tags {
			if !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
	}
	return tags, nil
}

// ListArtifactVersions implements sources.VersionIndex: one ArtifactVersion
// per tag of ch.Repository, with empty Fields. Noise tags (see IsNoiseTag)
// are dropped unless the adapter was built with WithNoiseFilter(false). When
// ch.TagPattern is set, only tags matching that regular expression are kept.
//
// Each entry's Evidence points at the tags/list page that listed the tag.
func (a *Adapter) ListArtifactVersions(ctx context.Context, ch catalog.Locator) ([]sources.ArtifactVersion, error) {
	repo, err := a.repository(ch.Repository)
	if err != nil {
		return nil, err
	}
	var include *regexp.Regexp
	if ch.TagPattern != "" {
		if include, err = regexp.Compile(ch.TagPattern); err != nil {
			return nil, fmt.Errorf("oci: tagPattern %q: %w", ch.TagPattern, err)
		}
	}
	pages, err := a.listPages(ctx, repo)
	if err != nil {
		return nil, err
	}
	var out []sources.ArtifactVersion
	seen := map[string]bool{}
	for _, p := range pages {
		for _, t := range p.Tags {
			if seen[t] || t == "" {
				continue
			}
			seen[t] = true
			if a.noiseFilter && IsNoiseTag(t) {
				continue
			}
			if include != nil && !include.MatchString(t) {
				continue
			}
			out = append(out, sources.ArtifactVersion{
				Version: t,
				URI:     repo.manifestURL(t),
				Evidence: domain.NewEvidence(domain.EvidenceRegistry, "", p.URL,
					"tags["+t+"]", "tag: "+t, p.Digest, p.Retrieved),
			})
		}
	}
	return out, nil
}
