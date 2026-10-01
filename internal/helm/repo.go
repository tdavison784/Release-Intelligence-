package helm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// RepoAdapter serves the "helm-repo" locator kind (loc.URL = repository base
// URL, loc.Chart = chart name) from the repository's index.yaml.
//
// The index is fetched through the fetch client with a TTL (it is mutable)
// and parsed once per process and URL; see Index. A RepoAdapter is safe for
// concurrent use.
type RepoAdapter struct {
	indexes *indexCache
}

// NewRepoAdapter returns a helm-repo adapter issuing HTTP through f.
func NewRepoAdapter(f fetch.Client, opts ...Option) *RepoAdapter {
	c := newConfig(opts)
	return &RepoAdapter{indexes: newIndexCache(f, c.indexTTL)}
}

var (
	_ sources.VersionIndex  = (*RepoAdapter)(nil)
	_ sources.ArtifactProbe = (*RepoAdapter)(nil)
	_ sources.VersionLister = (*RepoAdapter)(nil)
)

// Index returns the parsed index of the repository at repoURL.
func (a *RepoAdapter) Index(ctx context.Context, repoURL string) (*Index, error) {
	if strings.TrimSpace(repoURL) == "" {
		return nil, fmt.Errorf("helm-repo: locator has no url")
	}
	return a.indexes.get(ctx, repoURL)
}

// entries returns the index and the entries of the locator's chart. A chart
// that the (reachable) index does not know is fetch.ErrNotFound.
func (a *RepoAdapter) entries(ctx context.Context, loc catalog.Locator) (*Index, []IndexEntry, error) {
	if strings.TrimSpace(loc.Chart) == "" {
		return nil, nil, fmt.Errorf("helm-repo: locator has no chart")
	}
	idx, err := a.Index(ctx, loc.URL)
	if err != nil {
		return nil, nil, err
	}
	es, ok := idx.Entries[loc.Chart]
	if !ok || len(es) == 0 {
		return idx, nil, fmt.Errorf("helm-repo: chart %q is not in %s: %w", loc.Chart, idx.URL, fetch.ErrNotFound)
	}
	return idx, es, nil
}

// evidence builds the registry evidence for one index entry.
func (idx *Index) evidence(chart string, e IndexEntry) domain.Evidence {
	return domain.NewEvidence(domain.EvidenceRegistry, "", idx.URL,
		fmt.Sprintf("entries.%s[version=%s]", chart, e.Version),
		fmt.Sprintf("version: %s, appVersion: %s", e.Version, e.AppVersion),
		idx.Digest, idx.RetrievedAt)
}

// ListArtifactVersions implements sources.VersionIndex: one ArtifactVersion
// per index entry of loc.Chart, in index order.
//
// Fields carries "appVersion", "created" and "kubeVersion" (keys with empty
// values are omitted). Digest is the archive digest as "sha256:<hex>", URI
// the first archive URL resolved against the repository URL. A chart missing
// from a reachable index yields an error wrapping fetch.ErrNotFound.
func (a *RepoAdapter) ListArtifactVersions(ctx context.Context, loc catalog.Locator) ([]sources.ArtifactVersion, error) {
	idx, es, err := a.entries(ctx, loc)
	if err != nil {
		return nil, err
	}
	out := make([]sources.ArtifactVersion, 0, len(es))
	for _, e := range es {
		fields := map[string]string{}
		for k, v := range map[string]string{"appVersion": e.AppVersion, "created": e.Created, "kubeVersion": e.KubeVersion} {
			if v != "" {
				fields[k] = v
			}
		}
		out = append(out, sources.ArtifactVersion{
			Version:  e.Version,
			Fields:   fields,
			Digest:   normalizeDigest(e.Digest),
			URI:      idx.ChartURL(e),
			Evidence: idx.evidence(loc.Chart, e),
		})
	}
	return out, nil
}

// Probe implements sources.ArtifactProbe: it reports whether the index lists
// chart loc.Chart at exactly the given version. If no entry is textually
// equal, an entry that is semantically equal ("1.2.3" vs "v1.2.3") counts.
//
// A reachable index without the version (or without the chart) is not an
// error: the result has Exists=false. An unreachable repository yields an
// error wrapping fetch.ErrUnavailable.
func (a *RepoAdapter) Probe(ctx context.Context, loc catalog.Locator, version string) (*sources.ProbeResult, error) {
	if strings.TrimSpace(version) == "" {
		return nil, fmt.Errorf("helm-repo: empty chart version")
	}
	if strings.TrimSpace(loc.Chart) == "" {
		return nil, fmt.Errorf("helm-repo: locator has no chart")
	}
	idx, err := a.Index(ctx, loc.URL)
	if err != nil {
		return nil, err
	}
	coord := fmt.Sprintf("%s %s@%s", strings.TrimRight(loc.URL, "/"), loc.Chart, version)

	e, ok := findEntry(idx.Entries[loc.Chart], version)
	if !ok {
		excerpt := fmt.Sprintf("no entry with version %s", version)
		if len(idx.Entries[loc.Chart]) == 0 {
			excerpt = fmt.Sprintf("chart %s is not in the index", loc.Chart)
		}
		return &sources.ProbeResult{
			Exists:     false,
			Coordinate: coord,
			URI:        idx.URL,
			Evidence: domain.NewEvidence(domain.EvidenceRegistry, "", idx.URL,
				"entries."+loc.Chart, excerpt, idx.Digest, idx.RetrievedAt),
		}, nil
	}
	return &sources.ProbeResult{
		Exists:     true,
		Coordinate: fmt.Sprintf("%s %s@%s", strings.TrimRight(loc.URL, "/"), loc.Chart, e.Version),
		Digest:     normalizeDigest(e.Digest),
		URI:        idx.ChartURL(e),
		Evidence:   idx.evidence(loc.Chart, e),
	}, nil
}

// findEntry returns the entry for version: an exact textual match first, then
// a semantic-version match.
func findEntry(es []IndexEntry, version string) (IndexEntry, bool) {
	for _, e := range es {
		if e.Version == version {
			return e, true
		}
	}
	for _, e := range es {
		if sameVersion(e.Version, version) {
			return e, true
		}
	}
	return IndexEntry{}, false
}

// ListReleases implements sources.VersionLister for products whose canonical
// versions come from a chart repository: one ReleaseRef per chart version,
// Tag being the chart version and PublishedAt the entry's "created" time.
func (a *RepoAdapter) ListReleases(ctx context.Context, loc catalog.Locator) ([]sources.ReleaseRef, error) {
	idx, es, err := a.entries(ctx, loc)
	if err != nil {
		return nil, err
	}
	out := make([]sources.ReleaseRef, 0, len(es))
	for _, e := range es {
		out = append(out, sources.ReleaseRef{
			Tag:         e.Version,
			PublishedAt: parseCreated(e.Created),
			URL:         idx.ChartURL(e),
			Evidence:    idx.evidence(loc.Chart, e),
		})
	}
	return out, nil
}

// parseCreated parses an index "created" timestamp (RFC 3339 with optional
// fractional seconds); it returns nil when absent or unparsable.
func parseCreated(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05.999999999 -0700 MST"} {
		if t, err := time.Parse(layout, s); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}
