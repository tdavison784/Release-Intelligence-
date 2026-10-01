package helm

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// GitAdapter serves the "helm-git" locator kind: a chart that lives in a
// (mono-)repository and is released by git tags.
//
//	repository: github.com/argoproj/argo-helm
//	path:       charts/argo-cd
//	tagPattern: ^argo-cd-(?P<version>.+)$
//
// It depends only on the ports it composes: a sources.VersionLister serving
// the git-tags kind (to list the chart's tags) and a sources.DocumentFetcher
// serving repo-file (to read <path>/Chart.yaml at a tag). Chart.yaml at a
// release tag is immutable, which makes every read cacheable forever by the
// underlying fetcher.
//
// The tag list is memoized per (repository, tagPattern) for the life of the
// adapter. A GitAdapter is safe for concurrent use.
type GitAdapter struct {
	tags  sources.VersionLister
	files sources.DocumentFetcher
	cfg   config
	now   func() time.Time

	mu    sync.Mutex
	slots map[string]*tagSlot
}

type tagSlot struct {
	mu     sync.Mutex
	loaded bool
	cands  []candidate
}

// candidate is a git tag that carries a release of the chart.
type candidate struct {
	Tag     string // git tag, e.g. "argo-cd-10.9.5"
	Version string // chart version extracted with the tag pattern, e.g. "10.9.5"
	sv      *semver.Version
	Ref     sources.ReleaseRef
}

// NewGitAdapter returns a helm-git adapter over the given ports. Both must be
// non-nil. Options: WithMaxTags, WithConcurrency.
func NewGitAdapter(gitTags sources.VersionLister, repoFiles sources.DocumentFetcher, opts ...Option) *GitAdapter {
	return &GitAdapter{
		tags:  gitTags,
		files: repoFiles,
		cfg:   newConfig(opts),
		now:   time.Now,
		slots: map[string]*tagSlot{},
	}
}

var (
	_ sources.VersionIndex  = (*GitAdapter)(nil)
	_ sources.ArtifactProbe = (*GitAdapter)(nil)
)

// chartYAMLPath is the repo path of the chart's Chart.yaml.
func chartYAMLPath(dir string) string {
	dir = strings.Trim(strings.TrimSpace(dir), "/")
	if dir == "" || dir == "." {
		return "Chart.yaml"
	}
	return path.Join(dir, "Chart.yaml")
}

// repoURI turns a locator repository ("github.com/o/r" or a URL) into a URL.
func repoURI(repository string) string {
	r := strings.TrimSpace(repository)
	if strings.Contains(r, "://") {
		return r
	}
	return "https://" + r
}

// candidates lists the release tags of the chart, newest chart version first.
// Tags that do not match ch.TagPattern, are drafts, or whose extracted
// version is not a semantic version are dropped.
func (g *GitAdapter) candidates(ctx context.Context, ch catalog.Locator) ([]candidate, error) {
	if strings.TrimSpace(ch.Repository) == "" {
		return nil, fmt.Errorf("helm-git: locator has no repository")
	}
	if strings.TrimSpace(ch.TagPattern) == "" {
		return nil, fmt.Errorf("helm-git: locator has no tagPattern")
	}
	re, err := regexp.Compile(ch.TagPattern)
	if err != nil {
		return nil, fmt.Errorf("helm-git: tagPattern %q: %w", ch.TagPattern, err)
	}
	group := re.SubexpIndex("version")
	if group < 0 && re.NumSubexp() > 0 {
		group = 1
	}
	if group < 0 {
		group = 0
	}

	key := ch.Repository + "\x00" + ch.TagPattern
	g.mu.Lock()
	slot, ok := g.slots[key]
	if !ok {
		slot = &tagSlot{}
		g.slots[key] = slot
	}
	g.mu.Unlock()

	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.loaded {
		return slot.cands, nil
	}
	refs, err := g.tags.ListReleases(ctx, catalog.Locator{
		Kind: catalog.LocatorGitTags, Repository: ch.Repository, TagPattern: ch.TagPattern,
	})
	if err != nil {
		return nil, fmt.Errorf("helm-git: list tags of %s: %w", ch.Repository, err)
	}
	var cands []candidate
	seen := map[string]bool{}
	for _, ref := range refs {
		if ref.Draft || seen[ref.Tag] {
			continue
		}
		m := re.FindStringSubmatch(ref.Tag)
		if m == nil {
			continue
		}
		v := strings.TrimSpace(m[group])
		sv, ok := parseVersion(v)
		if !ok {
			continue
		}
		seen[ref.Tag] = true
		cands = append(cands, candidate{Tag: ref.Tag, Version: v, sv: sv, Ref: ref})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if c := cands[i].sv.Compare(cands[j].sv); c != 0 {
			return c > 0 // newest first
		}
		return cands[i].Tag < cands[j].Tag
	})
	slot.cands, slot.loaded = cands, true
	return cands, nil
}

// chartRead is the outcome of reading Chart.yaml at one tag.
type chartRead struct {
	doc     *sources.Document
	meta    *ChartMeta
	missing bool  // no Chart.yaml at that tag
	badYAML error // Chart.yaml exists but cannot be decoded
}

// readChart reads and decodes <path>/Chart.yaml at tag.
func (g *GitAdapter) readChart(ctx context.Context, ch catalog.Locator, tag string) (chartRead, error) {
	doc, err := g.files.FetchDocument(ctx, catalog.Locator{
		Kind: catalog.LocatorRepoFile, Repository: ch.Repository, Ref: tag, Path: chartYAMLPath(ch.Path),
	})
	switch {
	case errors.Is(err, fetch.ErrNotFound):
		return chartRead{missing: true}, nil
	case err != nil:
		return chartRead{}, fmt.Errorf("helm-git: read %s at %s: %w", chartYAMLPath(ch.Path), tag, err)
	}
	meta, perr := ParseChartYAML(doc.Content)
	return chartRead{doc: doc, meta: meta, badYAML: perr}, nil
}

// readAll reads Chart.yaml for every candidate with bounded concurrency. The
// first hard failure (anything but "no Chart.yaml at this tag") cancels the
// remaining reads and is returned.
func (g *GitAdapter) readAll(ctx context.Context, ch catalog.Locator, cands []candidate) ([]chartRead, error) {
	results := make([]chartRead, len(cands))
	if len(cands) == 0 {
		return results, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
		jobs     = make(chan int)
	)
	workers := g.cfg.concurrency
	if workers > len(cands) {
		workers = len(cands)
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					continue // drain
				}
				r, err := g.readChart(ctx, ch, cands[i].Tag)
				if err != nil {
					once.Do(func() { firstErr = err; cancel() })
					continue
				}
				results[i] = r
			}
		}()
	}
feed:
	for i := range cands {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// ListArtifactVersions implements sources.VersionIndex: one ArtifactVersion
// per chart release tag, newest chart version first.
//
// For each inspected tag, Chart.yaml is read at that tag (concurrency bounded
// by WithConcurrency) and Fields carries "appVersion" and "kubeVersion" (when
// set), plus "tag", "commit" and "created" (the tag's date) when the git-tags
// lister provides them. Version is the version extracted from the tag with
// the locator's tagPattern; URI is the pinned Chart.yaml URL; Evidence is the
// Chart.yaml (structured) at that tag.
//
// With WithMaxTags(n), only the n newest tags are inspected; the remaining
// tags are still listed, but their Fields hold no appVersion/kubeVersion and
// their Evidence is the git ref. A tag without a Chart.yaml at the chart path
// is not a release of this chart and is left out; a Chart.yaml that cannot be
// decoded yields an entry without appVersion/kubeVersion.
func (g *GitAdapter) ListArtifactVersions(ctx context.Context, ch catalog.Locator) ([]sources.ArtifactVersion, error) {
	cands, err := g.candidates(ctx, ch)
	if err != nil {
		return nil, err
	}
	limit := len(cands)
	if g.cfg.maxTags > 0 && g.cfg.maxTags < limit {
		limit = g.cfg.maxTags
	}
	reads, err := g.readAll(ctx, ch, cands[:limit])
	if err != nil {
		return nil, err
	}
	out := make([]sources.ArtifactVersion, 0, len(cands))
	for i, c := range cands {
		if i >= limit {
			out = append(out, sources.ArtifactVersion{
				Version:  c.Version,
				Fields:   c.baseFields(),
				URI:      c.Ref.URL,
				Evidence: c.Ref.Evidence,
			})
			continue
		}
		r := reads[i]
		if r.missing {
			continue
		}
		fields := c.baseFields()
		if r.meta != nil && r.badYAML == nil {
			if r.meta.AppVersion != "" {
				fields["appVersion"] = r.meta.AppVersion
			}
			if r.meta.KubeVersion != "" {
				fields["kubeVersion"] = r.meta.KubeVersion
			}
		}
		out = append(out, sources.ArtifactVersion{
			Version:  c.Version,
			Fields:   fields,
			URI:      r.doc.URI,
			Evidence: chartEvidence(c, r),
		})
	}
	return out, nil
}

// baseFields are the fields known from the git tag alone.
func (c candidate) baseFields() map[string]string {
	f := map[string]string{"tag": c.Tag}
	if c.Ref.PublishedAt != nil {
		f["created"] = c.Ref.PublishedAt.UTC().Format(time.RFC3339)
	}
	if c.Ref.Commit != "" {
		f["commit"] = c.Ref.Commit
	}
	return f
}

// chartEvidence is the structured evidence of Chart.yaml at the candidate's tag.
func chartEvidence(c candidate, r chartRead) domain.Evidence {
	excerpt := "tag: " + c.Tag
	if r.meta != nil && r.badYAML == nil {
		excerpt = fmt.Sprintf("version: %s, appVersion: %s", r.meta.Version, r.meta.AppVersion)
	}
	return domain.NewEvidence(domain.EvidenceStructured, "", r.doc.URI, "$.appVersion", excerpt, r.doc.Digest, r.doc.RetrievedAt)
}

// Probe implements sources.ArtifactProbe: the chart release exists when a tag
// matching the locator's tagPattern carries the given chart version and
// <path>/Chart.yaml is readable (and valid) at that tag.
//
// Probe reads at most one Chart.yaml, regardless of WithMaxTags. A missing
// tag, or a tag without a usable Chart.yaml, yields Exists=false with a nil
// error; failures of the underlying ports are returned wrapped (so
// fetch.ErrUnavailable is preserved).
//
// Coordinate is "<repository> <path>@<tag>" ("@<version>" when no tag exists).
func (g *GitAdapter) Probe(ctx context.Context, ch catalog.Locator, version string) (*sources.ProbeResult, error) {
	if strings.TrimSpace(version) == "" {
		return nil, fmt.Errorf("helm-git: empty chart version")
	}
	cands, err := g.candidates(ctx, ch)
	if err != nil {
		return nil, err
	}
	dir := strings.Trim(ch.Path, "/ ")
	c, ok := findCandidate(cands, version)
	if !ok {
		return g.absent(fmt.Sprintf("%s %s@%s", ch.Repository, dir, version), repoURI(ch.Repository), "tags",
			fmt.Sprintf("no tag matching %s carries chart version %s", ch.TagPattern, version)), nil
	}
	coord := fmt.Sprintf("%s %s@%s", ch.Repository, dir, c.Tag)
	r, err := g.readChart(ctx, ch, c.Tag)
	if err != nil {
		return nil, err
	}
	refURI := c.Ref.URL
	if refURI == "" {
		refURI = repoURI(ch.Repository)
	}
	switch {
	case r.missing:
		return g.absent(coord, refURI, "refs/tags/"+c.Tag,
			fmt.Sprintf("tag %s exists but %s is missing", c.Tag, chartYAMLPath(ch.Path))), nil
	case r.badYAML != nil:
		excerpt := fmt.Sprintf("%s at tag %s is unusable: %v", chartYAMLPath(ch.Path), c.Tag, r.badYAML)
		return &sources.ProbeResult{
			Exists:     false,
			Coordinate: coord,
			URI:        r.doc.URI,
			Evidence:   domain.NewEvidence(domain.EvidenceStructured, "", r.doc.URI, "$", excerpt, r.doc.Digest, r.doc.RetrievedAt),
		}, nil
	}
	return &sources.ProbeResult{
		Exists:     true,
		Coordinate: coord,
		URI:        r.doc.URI,
		Evidence:   chartEvidence(c, r),
	}, nil
}

// absent builds a negative probe result.
func (g *GitAdapter) absent(coord, uri, locator, excerpt string) *sources.ProbeResult {
	return &sources.ProbeResult{
		Exists:     false,
		Coordinate: coord,
		URI:        uri,
		Evidence:   domain.NewEvidence(domain.EvidenceGitRef, "", uri, locator, excerpt, "", g.now()),
	}
}

// findCandidate finds the tag for a chart version: textual match first, then
// semantic equality.
func findCandidate(cands []candidate, version string) (candidate, bool) {
	for _, c := range cands {
		if c.Version == version {
			return c, true
		}
	}
	for _, c := range cands {
		if sameVersion(c.Version, version) {
			return c, true
		}
	}
	return candidate{}, false
}
