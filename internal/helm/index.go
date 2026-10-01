package helm

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/fetch"
)

// IndexEntry is one chart version of a repository index. Only the fields the
// release-intelligence pipeline needs are decoded.
type IndexEntry struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	AppVersion  string   `yaml:"appVersion"`
	KubeVersion string   `yaml:"kubeVersion"`
	Created     string   `yaml:"created"`
	Digest      string   `yaml:"digest"` // hex sha256 of the archive, as written by helm
	Description string   `yaml:"description"`
	URLs        []string `yaml:"urls"`
	Deprecated  bool     `yaml:"deprecated"`
}

// Index is a parsed repository index.yaml.
type Index struct {
	// URL is the index.yaml URL that was fetched.
	URL string
	// BaseURL is the repository URL, with a trailing slash; relative chart
	// URLs resolve against it.
	BaseURL string
	// Generated is the "generated" timestamp of the index, verbatim.
	Generated string
	// Digest is the sha256 of the exact bytes retrieved.
	Digest string
	// RetrievedAt is when those bytes were retrieved (original retrieval
	// time when served from the cache).
	RetrievedAt time.Time
	// Entries maps chart name to its versions in index order (helm writes
	// newest first).
	Entries map[string][]IndexEntry
}

// ChartURL returns the first download URL of e resolved against the
// repository URL, or "" when the entry has none.
func (idx *Index) ChartURL(e IndexEntry) string {
	if len(e.URLs) == 0 {
		return ""
	}
	return resolveURL(idx.BaseURL, e.URLs[0])
}

// resolveURL resolves ref against base (relative chart URLs are relative to
// the repository URL, which is treated as a directory).
func resolveURL(base, ref string) string {
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	if r.IsAbs() {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

// normalizeDigest renders a helm index digest ("<hex>") as "sha256:<hex>".
func normalizeDigest(d string) string {
	d = strings.TrimSpace(d)
	if hexSHA256.MatchString(d) {
		return "sha256:" + strings.ToLower(d)
	}
	return d
}

var hexSHA256 = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// ParseIndex decodes the bytes of an index.yaml. indexURL and the digest are
// recorded in the result; the repository base URL is derived from indexURL.
func ParseIndex(body []byte, indexURL, digest string, retrievedAt time.Time) (*Index, error) {
	var raw struct {
		APIVersion string                  `yaml:"apiVersion"`
		Generated  string                  `yaml:"generated"`
		Entries    map[string][]IndexEntry `yaml:"entries"`
	}
	if err := yaml.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode %s: %w", indexURL, err)
	}
	if raw.Entries == nil {
		return nil, fmt.Errorf("decode %s: no \"entries\" (not a Helm repository index)", indexURL)
	}
	base := strings.TrimSuffix(indexURL, "index.yaml")
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return &Index{
		URL:         indexURL,
		BaseURL:     base,
		Generated:   raw.Generated,
		Digest:      digest,
		RetrievedAt: retrievedAt,
		Entries:     raw.Entries,
	}, nil
}

// indexCache fetches and parses index.yaml files, once per process and URL.
type indexCache struct {
	f   fetch.Client
	ttl time.Duration

	mu    sync.Mutex
	slots map[string]*indexSlot
}

type indexSlot struct {
	mu  sync.Mutex
	idx *Index
}

func newIndexCache(f fetch.Client, ttl time.Duration) *indexCache {
	return &indexCache{f: f, ttl: ttl, slots: map[string]*indexSlot{}}
}

// indexURL returns the index.yaml URL of a repository URL.
func indexURL(repoURL string) string {
	return strings.TrimRight(strings.TrimSpace(repoURL), "/") + "/index.yaml"
}

// get returns the parsed index of repoURL. A successful parse is kept for the
// life of the cache, so a 1.6 MB index is decoded once no matter how many
// charts and versions are looked up. Failures are not remembered: concurrent
// callers share one attempt (the slot lock), later callers retry.
func (c *indexCache) get(ctx context.Context, repoURL string) (*Index, error) {
	u := indexURL(repoURL)
	c.mu.Lock()
	slot, ok := c.slots[u]
	if !ok {
		slot = &indexSlot{}
		c.slots[u] = slot
	}
	c.mu.Unlock()

	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.idx != nil {
		return slot.idx, nil
	}
	doc, err := c.f.Do(ctx, fetch.Request{URL: u, TTL: c.ttl})
	if err != nil {
		return nil, fmt.Errorf("helm-repo: fetch index: %w", err)
	}
	idx, err := ParseIndex(doc.Body, u, doc.Digest, doc.RetrievedAt)
	if err != nil {
		return nil, fmt.Errorf("helm-repo: %w", err)
	}
	slot.idx = idx
	return idx, nil
}
