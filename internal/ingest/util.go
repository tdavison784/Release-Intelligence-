package ingest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

// DefinitionDigest identifies the revision of a definition: the loader's
// digest of the raw document, or (for definitions built in memory) the digest
// of its canonical YAML encoding. Stored releases are reusable when their
// DefinitionDigest equals this value.
func DefinitionDigest(def *catalog.ProductDefinition) string {
	if def == nil {
		return ""
	}
	if d := def.Digest(); d != "" {
		return d
	}
	b, err := catalog.Marshal(def)
	if err != nil {
		return ""
	}
	return domain.Digest(b)
}

func (i *Ingester) now() time.Time {
	if i.Clock != nil {
		return i.Clock().UTC()
	}
	return time.Now().UTC()
}

func (i *Ingester) parser() Parser {
	if i.Parser != nil {
		return i.Parser
	}
	return DefaultParser
}

// parallel runs fn(0..n-1) with bounded concurrency and waits for all.
func (i *Ingester) parallel(n int, fn func(k int)) {
	limit := i.Concurrency
	if limit <= 0 {
		limit = DefaultConcurrency
	}
	if limit == 1 || n <= 1 {
		for k := 0; k < n; k++ {
			fn(k)
		}
		return
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for k := 0; k < n; k++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(k int) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(k)
		}(k)
	}
	wg.Wait()
}

// fetched is the outcome of retrieving a rendered locator.
type fetched struct {
	docs []sources.Document // one for document fetches; sorted by path for directories
	dir  bool
	err  error
}

// memo de-duplicates document retrievals within one ingestion (a channel
// body and a content of the same artifact are fetched once).
type memo struct {
	mu sync.Mutex
	m  map[string]*memoEntry
}

type memoEntry struct {
	once sync.Once
	res  fetched
}

func newMemo() *memo { return &memo{m: map[string]*memoEntry{}} }

func (m *memo) do(key string, fn func() fetched) fetched {
	m.mu.Lock()
	e, ok := m.m[key]
	if !ok {
		e = &memoEntry{}
		m.m[key] = e
	}
	m.mu.Unlock()
	e.once.Do(func() { e.res = fn() })
	return e.res
}

func locKey(l catalog.Locator) string {
	return strings.Join([]string{l.Kind, l.Repository, l.Ref, l.BaseRef, l.Path, l.Glob, l.URL, l.Chart, l.TagPattern}, "\x00")
}

// hasDocumentAdapter reports whether a rendered locator can be retrieved as
// a document (or directory) through the registry.
func (i *Ingester) hasDocumentAdapter(kind string) bool {
	if kind == catalog.LocatorRepoDir {
		_, err := i.Registry.DirectoryFetcher(kind)
		return err == nil
	}
	_, err := i.Registry.DocumentFetcher(kind)
	return err == nil
}

// fetch retrieves a rendered locator: repo-dir locators through the
// DirectoryFetcher, everything else through the DocumentFetcher. Results are
// memoised per ingestion and normalised (digest, retrieval time, URI).
func (i *Ingester) fetch(ctx context.Context, mm *memo, loc catalog.Locator, now time.Time) fetched {
	dir := loc.Kind == catalog.LocatorRepoDir
	return mm.do(locKey(loc), func() fetched {
		if dir {
			f, err := i.Registry.DirectoryFetcher(loc.Kind)
			if err != nil {
				return fetched{dir: true, err: err}
			}
			docs, err := f.FetchDirectory(ctx, loc)
			if err != nil {
				return fetched{dir: true, err: err}
			}
			out := make([]sources.Document, len(docs))
			copy(out, docs)
			for k := range out {
				normaliseDoc(&out[k], loc, now)
			}
			sort.SliceStable(out, func(a, b int) bool {
				if out[a].Path != out[b].Path {
					return out[a].Path < out[b].Path
				}
				return out[a].URI < out[b].URI
			})
			return fetched{docs: out, dir: true}
		}
		f, err := i.Registry.DocumentFetcher(loc.Kind)
		if err != nil {
			return fetched{err: err}
		}
		doc, err := f.FetchDocument(ctx, loc)
		if err != nil {
			return fetched{err: err}
		}
		if doc == nil {
			return fetched{err: errors.New("adapter returned no document")}
		}
		d := *doc
		normaliseDoc(&d, loc, now)
		return fetched{docs: []sources.Document{d}}
	})
}

func normaliseDoc(d *sources.Document, loc catalog.Locator, now time.Time) {
	if d.Digest == "" {
		d.Digest = domain.Digest(d.Content)
	}
	if d.RetrievedAt.IsZero() {
		d.RetrievedAt = now
	}
	d.RetrievedAt = d.RetrievedAt.UTC()
	if d.URI == "" {
		d.URI = d.FetchURL
	}
	if d.URI == "" {
		d.URI = locatorURI(loc)
		if d.Path != "" && loc.Kind == catalog.LocatorRepoDir {
			d.URI += "#" + d.Path
		}
	}
}

// stateFor maps an adapter error to a source state. A missing adapter is
// "skipped" (the capability is not available in this build), noAdapter
// reports it so relationship checks can call it unverifiable.
func stateFor(err error) (state domain.SourceState, detail string, noAdapter bool) {
	var na *sources.ErrNoAdapter
	if errors.As(err, &na) {
		return domain.SourceSkipped, err.Error(), true
	}
	var ue *unresolvedError
	if errors.As(err, &ue) {
		return domain.SourceSkipped, err.Error(), false
	}
	return fetch.StateFor(err), err.Error(), false
}

// unresolvedError reports a locator that depends on a context value that is
// unknown for this release (e.g. the previous release of the first release).
type unresolvedError struct {
	field string
	need  string
}

func (e *unresolvedError) Error() string {
	return fmt.Sprintf("locator %s needs the %s, which is unknown for this release", e.field, e.need)
}

var contextFieldRe = regexp.MustCompile(`\.(PrevLine|PrevMajor|PrevMinor|PrevTag|PrevVersion|ArtifactVersion)\b`)

// renderLocator renders a locator, applies generic per-kind defaults and
// refuses locators whose templates depend on unknown context values.
func renderLocator(l catalog.Locator, rc catalog.RenderContext) (catalog.Locator, error) {
	fields := []struct {
		name, value string
	}{
		{"repository", l.Repository}, {"ref", l.Ref}, {"baseRef", l.BaseRef}, {"path", l.Path},
		{"glob", l.Glob}, {"url", l.URL}, {"chart", l.Chart},
	}
	for _, f := range fields {
		for _, m := range contextFieldRe.FindAllStringSubmatch(f.value, -1) {
			switch m[1] {
			case "PrevLine", "PrevMajor", "PrevMinor":
				if rc.PrevLine == "" {
					return catalog.Locator{}, &unresolvedError{f.name, "previous release line"}
				}
			case "PrevTag", "PrevVersion":
				if rc.PrevTag == "" {
					return catalog.Locator{}, &unresolvedError{f.name, "previous release"}
				}
			case "ArtifactVersion":
				if rc.ArtifactVersion == "" {
					return catalog.Locator{}, &unresolvedError{f.name, "artifact version"}
				}
			}
		}
	}
	out, err := catalog.RenderLocator(l, rc)
	if err != nil {
		return catalog.Locator{}, err
	}
	// A release document of a github-releases locator is addressed by tag.
	if out.Kind == catalog.LocatorGitHubReleases && out.Ref == "" && rc.Tag != "" {
		out.Ref = rc.Tag
	}
	return out, nil
}

// locatorURI returns a human-facing URI for a rendered locator (best effort,
// used when no document URI is available).
func locatorURI(l catalog.Locator) string {
	if l.URL != "" {
		return l.URL
	}
	switch l.Kind {
	case catalog.LocatorGitHubReleases:
		if l.Ref != "" {
			return "https://github.com/" + l.Repository + "/releases/tag/" + l.Ref
		}
		return "https://github.com/" + l.Repository + "/releases"
	case catalog.LocatorGitHubAdvisories:
		return "https://github.com/" + l.Repository + "/security/advisories"
	case catalog.LocatorOCI:
		return "oci://" + strings.TrimPrefix(l.Repository, "oci://")
	}
	if l.Repository == "" {
		return ""
	}
	s := l.Repository
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	return s
}

// describeLocator is a compact human description of a rendered locator.
func describeLocator(l catalog.Locator) string {
	if l.URL != "" {
		if l.Chart != "" {
			return strings.TrimSuffix(l.URL, "/") + " (chart " + l.Chart + ")"
		}
		return l.URL
	}
	s := l.Repository
	if l.Ref != "" {
		s += "@" + l.Ref
	}
	if l.Path != "" {
		s += ":" + l.Path
	}
	if l.Glob != "" {
		s += "/" + l.Glob
	}
	return s
}

// coordinateFor is the canonical coordinate of an artifact version at a
// rendered channel when the channel did not report one itself.
func coordinateFor(l catalog.Locator, av string) string {
	withVersion := func(base, sep string) string {
		if av == "" {
			return base
		}
		return base + sep + av
	}
	switch l.Kind {
	case catalog.LocatorOCI:
		return withVersion(strings.TrimPrefix(l.Repository, "oci://"), ":")
	case catalog.LocatorHelmRepo:
		return withVersion(strings.TrimSuffix(l.URL, "/")+"/"+l.Chart, ":")
	case catalog.LocatorHelmGit:
		return withVersion(trimRepo(l.Repository)+"/"+strings.Trim(l.Path, "/"), ":")
	case catalog.LocatorHTTP:
		return l.URL
	case catalog.LocatorGitHubReleases:
		return withVersion("github.com/"+l.Repository, "@")
	case catalog.LocatorGitTags:
		return withVersion(trimRepo(l.Repository), "@")
	}
	return describeLocator(l)
}

func trimRepo(r string) string {
	r = strings.TrimPrefix(r, "https://")
	r = strings.TrimPrefix(r, "http://")
	r = strings.TrimSuffix(r, "/")
	return strings.TrimSuffix(r, ".git")
}

// repositoryOf derives the repository used to resolve bare "#1234"
// references in documents of a locator.
func repositoryOf(l catalog.Locator) string {
	switch l.Kind {
	case catalog.LocatorGitHubReleases, catalog.LocatorGitHubAdvisories:
		return l.Repository
	case catalog.LocatorGitTags, catalog.LocatorRepoFile, catalog.LocatorRepoDir, catalog.LocatorGitLog, catalog.LocatorHelmGit:
		return trimRepo(l.Repository)
	}
	return ""
}

func renderRegex(tmpl string, rc catalog.RenderContext) (string, *regexp.Regexp, error) {
	s, err := catalog.Render(tmpl, rc)
	if err != nil {
		return "", nil, err
	}
	re, err := regexp.Compile(s)
	if err != nil {
		return s, nil, fmt.Errorf("invalid regex %q: %w", s, err)
	}
	return s, re, nil
}

// findLine returns the 1-based number and text of the first line containing needle.
func findLine(content []byte, needle string) (int, string, bool) {
	if needle == "" {
		return 0, "", false
	}
	n := 0
	for len(content) > 0 {
		n++
		line := content
		if k := bytes.IndexByte(content, '\n'); k >= 0 {
			line, content = content[:k], content[k+1:]
		} else {
			content = nil
		}
		if bytes.Contains(line, []byte(needle)) {
			return n, strings.TrimSpace(strings.TrimRight(string(line), "\r")), true
		}
	}
	return 0, "", false
}

// findReference is findLine for artifact references: an occurrence only
// counts when it is not part of a longer token, so the pattern
// "repo:v1.2.0" does not match "repo:v1.2.0-rc.7" (Argo CD v2.14.0 shipped
// an install manifest pointing at an rc image) or "mirror/repo:v1.2.0". A
// digest suffix ("repo:v1.2.0@sha256:...") still matches.
func findReference(content []byte, pattern string) (int, string, bool) {
	if pattern == "" {
		return 0, "", false
	}
	n := 0
	for _, line := range strings.Split(string(content), "\n") {
		n++
		for from := 0; ; {
			k := strings.Index(line[from:], pattern)
			if k < 0 {
				break
			}
			start, end := from+k, from+k+len(pattern)
			if referenceBoundary(line, pattern, start, end) {
				return n, strings.TrimSpace(strings.TrimRight(line, "\r")), true
			}
			from = start + 1
		}
	}
	return 0, "", false
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func referenceBoundary(line, pattern string, start, end int) bool {
	if start > 0 && isTokenByte(pattern[0]) {
		switch c := line[start-1]; {
		case isAlnum(c), c == '-', c == '_', c == '.', c == '/':
			return false
		}
	}
	if end < len(line) && isTokenByte(pattern[len(pattern)-1]) {
		switch c := line[end]; {
		case isAlnum(c), c == '-', c == '_', c == '+':
			return false
		case c == '.' && end+1 < len(line) && isAlnum(line[end+1]):
			return false
		}
	}
	return true
}

func isTokenByte(c byte) bool {
	return isAlnum(c) || c == '-' || c == '_' || c == '.' || c == '/' || c == ':'
}

// firstLine returns the first non-blank line, bounded for excerpts.
func firstLine(content []byte) string {
	for _, l := range strings.Split(string(content), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			if len(l) > 200 {
				l = domain.TruncateExcerpt(l[:200])
			}
			return l
		}
	}
	return ""
}

func lineCount(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	n := bytes.Count(content, []byte{'\n'})
	if content[len(content)-1] != '\n' {
		n++
	}
	return n
}

// compareArtifactVersions orders artifact versions: semver (leniently
// parsed, so "v1.2.3" works) before non-semver, non-semver by string.
func compareArtifactVersions(a, b string) int {
	va, ea := semver.NewVersion(a)
	vb, eb := semver.NewVersion(b)
	switch {
	case ea == nil && eb == nil:
		if c := va.Compare(vb); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	case ea == nil:
		return -1
	case eb == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func evidenceIDs(evs []domain.Evidence) []domain.EvidenceID {
	var out []domain.EvidenceID
	seen := map[domain.EvidenceID]bool{}
	for _, e := range evs {
		if e.ID == "" || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		out = append(out, e.ID)
	}
	return out
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

// outcomeForState maps a source state to a relationship check outcome.
func outcomeForState(s domain.SourceState, noAdapter bool) string {
	switch s {
	case domain.SourceOK, domain.SourcePartial:
		return OutcomePass
	case domain.SourceNotFound:
		return OutcomeFail
	case domain.SourceSkipped:
		if noAdapter {
			return OutcomeUnverifiable
		}
		return OutcomeNotApplicable
	default:
		return OutcomeUnverifiable
	}
}

// notApplicableDetail explains why a source/artifact does not apply to v.
func notApplicableDetail(v domain.Version, availability string, kinds []string) string {
	var parts []string
	if strings.TrimSpace(availability) != "" {
		parts = append(parts, fmt.Sprintf("availability %q", availability))
	}
	if len(kinds) > 0 {
		parts = append(parts, fmt.Sprintf("release kinds %s (this is a %s release)", strings.Join(kinds, ","), catalog.ReleaseKind(v)))
	}
	return fmt.Sprintf("not applicable to %s: %s", v.Semver, strings.Join(parts, ", "))
}
