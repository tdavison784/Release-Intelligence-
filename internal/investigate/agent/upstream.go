package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/gitsrc"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// Upstream behavioural evidence (prompt feature upstream; docs/phase6/
// decision-intel/PLAN-M2.md lane upstream). request_evidence kinds
// upstream-file and upstream-search are fulfilled during the item's session:
// one file, or a regex search over one directory, of the product's own code
// repositories at an immutable tag of the upgrade (the target's by default,
// the current version's for comparison). What is read is stored as an
// upstream-document run artifact (URI: the blob URL at the tag, producer
// evidence-fetch@v1), citable at once; the bundle is never changed. Anything
// else (another repository, another ref, an exhausted budget, a file that is
// not found or not in the recorded cache offline) is recorded as an
// unavailable request with its reason, and the item's assessment states it in
// unknown as "behaviour at <tag> unavailable: <reason>". Fetches go through
// the gitsrc / fetch caches, so a recorded state replays offline with the
// same bytes.

// UpstreamProducer is the producer of upstream evidence fetched mid-session.
const UpstreamProducer = "evidence-fetch@v1"

// upstreamGap prefixes the tool layer's unknown entry for an unfulfilled
// upstream request.
const upstreamGap = "investigation gap (recorded by the tool layer): behaviour at "

// Upstream is the fetch port of the upstream request kinds.
type Upstream struct {
	Files sources.DocumentFetcher  // repo-file (gitsrc Files)
	Dirs  sources.DirectoryFetcher // repo-dir (gitsrc Dirs)
	// Product returns the product's own code repositories ("owner/name";
	// linkedev.CodeReposOf) and its tag for a semantic version (catalog
	// TagFor); ok is false for an unknown product.
	Product func(id domain.ProductID) (repos []string, tagFor func(semver string) string, ok bool)
}

// upstreamState is the focus item's upstream budget and what it fetched.
type upstreamState struct {
	fetches int
	bytes   int64
	done    map[string][]string // query key -> stored artifact ids
}

// upstreamView is what the model receives for an upstream request.
type upstreamView struct {
	Request requestView          `json:"request"`
	Query   domain.UpstreamQuery `json:"query"`
	// File: the stored artifact and its first lines.
	File *upstreamFile `json:"file,omitempty"`
	// Search: what was scanned and the matching lines.
	Search *upstreamSearch `json:"search,omitempty"`
	Gap    string          `json:"gap,omitempty"`
	Budget string          `json:"budget"`
	Next   string          `json:"next,omitempty"`
}

type upstreamFile struct {
	Artifact string                  `json:"artifact"`
	URI      string                  `json:"uri"`
	Bytes    int                     `json:"bytes"`
	Lines    int                     `json:"lines"`
	Head     []investigate.LineMatch `json:"head,omitempty"`
	More     bool                    `json:"more,omitempty"`
}

type upstreamSearch struct {
	FilesListed  int             `json:"filesListed"`
	FilesScanned int             `json:"filesScanned"`
	Truncated    string          `json:"truncated,omitempty"`
	Matches      []upstreamMatch `json:"matches"`
	MoreMatches  bool            `json:"moreMatches,omitempty"`
	NotStored    string          `json:"notStored,omitempty"`
}

type upstreamMatch struct {
	Path     string                  `json:"path"`
	Artifact string                  `json:"artifact,omitempty"` // empty: the file was not stored (NotStored says why)
	URI      string                  `json:"uri"`
	Line     int                     `json:"line"`
	Text     string                  `json:"text"`
	Context  []investigate.LineMatch `json:"context,omitempty"`
}

// upstreamQuery is a parsed, structurally valid request.
type upstreamQuery struct {
	kind  domain.EvidenceRequestKind
	repo  gitsrc.Repo
	q     domain.UpstreamQuery
	re    *regexp.Regexp
	given string // the ref as given ("" means the target's tag)
}

func upstreamFieldsSet(in investigate.EvidenceRequestInput) bool {
	return in.Repo != "" || in.Ref != "" || in.Path != "" || in.Dir != "" || in.Pattern != "" || in.Glob != ""
}

// parseUpstream checks the shape of an upstream request; a malformed one
// rejects the call (nothing is recorded) so the model can fix it.
func parseUpstream(in investigate.EvidenceRequestInput, tags []string) (*upstreamQuery, error) {
	uq := &upstreamQuery{kind: in.Kind}
	repo, ref, p := trimmed(in.Repo), trimmed(in.Ref), trimmed(in.Path)
	target := trimmed(in.Target)
	switch in.Kind {
	case domain.RequestUpstreamFile:
		if in.Dir != "" || in.Pattern != "" || in.Glob != "" {
			return nil, rejectf("upstream-file takes repo, ref and path (or a blob URL in target); dir, pattern and glob are for upstream-search")
		}
		if target != "" {
			r, f, fp, err := parseBlobURL(target, tags)
			if err != nil {
				return nil, err
			}
			if (repo != "" && !sameRepo(repo, r)) || (ref != "" && ref != f) || (p != "" && strings.Trim(p, "/") != fp) {
				return nil, rejectf("target %s names another file than repo/ref/path; give one or the other", target)
			}
			repo, ref, p = r, f, fp
		}
		if repo == "" || p == "" {
			return nil, rejectf("upstream-file needs repo and path (or a blob URL at a tag in target)")
		}
		cp, err := cleanPath("path", p)
		if err != nil || cp == "" {
			return nil, rejectf("path %q: want a file path of the repository", p)
		}
		uq.q.Path = cp
	case domain.RequestUpstreamSearch:
		if target != "" || p != "" {
			return nil, rejectf("upstream-search takes repo, ref, dir, pattern and optionally glob; leave target and path empty")
		}
		if repo == "" || trimmed(in.Dir) == "" || in.Pattern == "" {
			return nil, rejectf("upstream-search needs repo, dir and pattern (a regular expression matched line by line)")
		}
		d, err := cleanPath("dir", in.Dir)
		if err != nil {
			return nil, err
		}
		if d == "" {
			return nil, rejectf("dir: name a directory of the repository; the repository root is not searched")
		}
		if len(in.Pattern) > MaxGrepPattern {
			return nil, rejectf("pattern is %d bytes (max %d)", len(in.Pattern), MaxGrepPattern)
		}
		if uq.re, err = regexp.Compile(in.Pattern); err != nil {
			return nil, rejectf("pattern: %v", err)
		}
		g := trimmed(in.Glob)
		if g != "" {
			if _, err := path.Match(g, ""); err != nil || strings.Contains(g, "/") {
				return nil, rejectf("glob %q: want a pattern on base names such as *.go", g)
			}
		}
		uq.q.Dir, uq.q.Pattern, uq.q.Glob = d, in.Pattern, g
	}
	r, err := gitsrc.ParseRepo(repo)
	if err != nil {
		return nil, rejectf("repo: %v", err)
	}
	uq.repo, uq.given = r, ref
	uq.q.Repository, uq.q.Ref = r.String(), ref
	if r.Host == "github.com" {
		uq.q.Repository = r.Path
	}
	return uq, nil
}

func cleanPath(what, p string) (string, error) {
	p = strings.Trim(strings.TrimSpace(p), "/")
	for _, seg := range strings.Split(p, "/") {
		if p != "" && (seg == "" || seg == "." || seg == "..") {
			return "", rejectf("%s %q: want a repository-relative path without . or .. segments", what, p)
		}
	}
	if strings.ContainsRune(p, 0) {
		return "", rejectf("%s: invalid path", what)
	}
	return p, nil
}

// parseBlobURL reads https://github.com/<owner>/<name>/blob/<ref>/<path>. A
// ref with slashes is recognised when it is one of tags.
func parseBlobURL(s string, tags []string) (repo, ref, p string, err error) {
	u, perr := url.Parse(s)
	if perr != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", "", "", rejectf("target %q: want a blob URL (https://<host>/<owner>/<name>/blob/<tag>/<path>) or leave it empty and give repo, ref and path", s)
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	i := -1
	for k, seg := range segs {
		if seg == "blob" && k >= 2 {
			i = k
			break
		}
	}
	if i < 0 || i+2 >= len(segs) {
		return "", "", "", rejectf("target %q: want a blob URL (…/<owner>/<name>/blob/<tag>/<path>)", s)
	}
	repo = u.Host + "/" + strings.Join(segs[:i], "/")
	repo = strings.TrimSuffix(strings.TrimSuffix(repo, "/-"), "/")
	rest := strings.Join(segs[i+1:], "/")
	for _, t := range tags {
		if t != "" && strings.HasPrefix(rest, t+"/") {
			return repo, t, strings.TrimPrefix(rest, t+"/"), nil
		}
	}
	return repo, segs[i+1], strings.Join(segs[i+2:], "/"), nil
}

func sameRepo(a, b string) bool {
	ra, ea := gitsrc.ParseRepo(a)
	rb, eb := gitsrc.ParseRepo(b)
	return ea == nil && eb == nil && strings.EqualFold(ra.CloneURL, rb.CloneURL)
}

// upstreamScope is what the session may read: the product's code
// repositories and the upgrade's two tags.
type upstreamScope struct {
	repos           []string
	target, current string
}

func (s *Session) upstreamScope() upstreamScope {
	ch := s.cfg.Bundle.Change
	sc := upstreamScope{target: ch.To.Tag, current: ch.From.Tag}
	if u := s.cfg.Upstream; u != nil && u.Product != nil {
		if repos, tagFor, ok := u.Product(ch.Product.ID); ok {
			sc.repos = repos
			if tagFor != nil {
				if ch.To.Semver != "" {
					sc.target = tagFor(ch.To.Semver)
				}
				if ch.From.Semver != "" {
					sc.current = tagFor(ch.From.Semver)
				}
			}
		}
	}
	return sc
}

// refusal says why a well-formed request is outside what may be read ("" =
// allowed); it is recorded, not rejected.
func (sc upstreamScope) refusal(uq *upstreamQuery) string {
	allowed := false
	for _, r := range sc.repos {
		allowed = allowed || sameRepo(r, uq.repo.String())
	}
	if !allowed {
		known := "none is known for this product"
		if len(sc.repos) > 0 {
			known = strings.Join(sc.repos, ", ")
		}
		return fmt.Sprintf("refused: %s is not one of the product's own code repositories (%s)", uq.q.Repository, known)
	}
	ref := uq.q.Ref
	if !gitsrc.IsImmutableRef(ref) {
		return fmt.Sprintf("refused: ref %q is not an immutable ref (a version tag or a commit); only the target's tag %s, or the current version's tag %s for comparison, is read", ref, sc.target, sc.current)
	}
	if ref != sc.target && ref != sc.current {
		return fmt.Sprintf("refused: ref %s is neither the target's tag %s nor the current version's tag %s", ref, sc.target, sc.current)
	}
	return ""
}

func (uq *upstreamQuery) key() string {
	return strings.Join([]string{string(uq.kind), strings.ToLower(uq.repo.CloneURL), uq.q.Ref, uq.q.Path, uq.q.Dir, uq.q.Pattern, uq.q.Glob}, "\x00")
}

// target is the request's canonical target: the blob URL of the file, or the
// tree URL of the directory with the pattern.
func (uq *upstreamQuery) target() string {
	ref := uq.q.Ref
	if uq.kind == domain.RequestUpstreamFile {
		return uq.repo.BlobURL(ref, uq.q.Path)
	}
	t := uq.repo.TreeURL(ref, uq.q.Dir) + "?pattern=" + url.QueryEscape(uq.q.Pattern)
	if uq.q.Glob != "" {
		t += "&glob=" + url.QueryEscape(uq.q.Glob)
	}
	return t
}

// requestUpstream fulfils an upstream-file / upstream-search request (caller
// holds s.mu; state changes only in the commit).
func (s *Session) requestUpstream(ctx context.Context, in investigate.EvidenceRequestInput) (*outcome, error) {
	item := in.ChangeItem
	if item == "" {
		item = s.focus
	}
	it, err := s.item(item)
	if err != nil {
		return nil, err
	}
	if s.focus != "" && it.ID != s.focus {
		return nil, rejectf("upstream evidence is fetched for this session's item %s, not %s", s.focus, it.ID)
	}
	reason := trimmed(in.Reason)
	if reason == "" {
		return nil, rejectf("reason is required: the question the upstream text should settle")
	}
	if len(reason) > MaxStatement {
		return nil, rejectf("reason is too long (max %d bytes)", MaxStatement)
	}
	sc := s.upstreamScope()
	uq, err := parseUpstream(in, []string{sc.target, sc.current})
	if err != nil {
		return nil, err
	}
	if uq.q.Ref == "" {
		uq.q.Ref = sc.target
	}
	target := uq.target()
	id := domain.AgentRecordID(domain.EvidenceRequestIDPrefix, it.ID, string(uq.kind)+"\x00"+target, s.call.ID)
	q := uq.q
	r := domain.EvidenceRequest{ID: id, ChangeItem: it.ID, Kind: uq.kind, Target: target, Reason: reason, Upstream: &q,
		Provenance: s.aiProvenance(it.Evidence)}
	b := s.cfg.Prompt.Features.Upstream
	maxBytes := b.Bytes
	if m := s.cfg.Budget.MaxReadBytes; m > 0 && m < maxBytes {
		maxBytes = m
	}
	st := s.upState
	budget := func(fetches int, stored int64) string {
		return fmt.Sprintf("%d of %d upstream fetches and %d of %d stored bytes used for this item", fetches, b.Fetches, stored, maxBytes)
	}
	for i := range s.evReqs {
		if s.evReqs[i].ID == id {
			x := s.evReqs[i]
			return &outcome{value: &x, view: upstreamView{Request: requestView{ID: id, Status: x.Status, Resolution: x.Resolution}, Query: q,
				Budget: budget(st.fetches, st.bytes), Next: "already requested in this call"}}, nil
		}
	}
	view := upstreamView{Query: q}
	var stored []domain.ArtifactRef
	fetches, used := st.fetches, st.bytes
	unavailable := func(why string) {
		r.Status, r.Resolution = domain.RequestUnavailable, why
		view.Gap = upstreamGap + q.Ref + " unavailable: " + why
	}
	switch why := sc.refusal(uq); {
	case s.cfg.Upstream == nil:
		unavailable("no upstream fetcher is configured for this run")
	case why != "":
		unavailable(why)
	case st.done[uq.key()] != nil:
		r.Fulfilled = st.done[uq.key()] // read earlier in this item's session: not fetched or counted again
	case fetches >= b.Fetches:
		unavailable(fmt.Sprintf("budget: the item's %d upstream fetches are spent", b.Fetches))
	case used >= maxBytes:
		unavailable(fmt.Sprintf("budget: the item's %d upstream bytes are spent", maxBytes))
	default:
		fetches++
		var why string
		if uq.kind == domain.RequestUpstreamFile {
			stored, view.File, why = s.fetchUpstreamFile(ctx, uq, maxBytes-used, b)
		} else {
			stored, view.Search, why = s.searchUpstream(ctx, uq, maxBytes-used, b)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		for _, a := range stored {
			used += a.Size
			r.Fulfilled = append(r.Fulfilled, a.ID)
		}
		if why != "" {
			r.Fulfilled = nil
			unavailable(why)
		}
	}
	if len(r.Fulfilled) > 0 {
		r.Status = domain.RequestFulfilled
		view.Next = "cite it verbatim like any artifact; read further with read_artifact (line windows) or search_manifest (grep) on the artifact id"
	} else if view.Gap != "" {
		view.Next = "recorded as unavailable; the item's assessment will state the gap in unknown. Do not answer it from memory: fetch what the question needs within the allowed repositories and tags, or keep it unknown"
	}
	if err := invalid(func() error { c := r; c.Provenance = placeholder(c.Provenance); return c.Validate() }()); err != nil {
		return nil, err
	}
	view.Request = requestView{ID: id, Status: r.Status, Resolution: r.Resolution}
	view.Budget = budget(fetches, used)
	key := uq.key()
	ids := r.Fulfilled
	return &outcome{value: &r, view: view, commit: s.retrieve(func() {
		s.evReqs = append(s.evReqs, r)
		s.upState.fetches, s.upState.bytes = fetches, used
		if len(stored) > 0 {
			s.upState.done[key] = ids
		}
		for _, a := range stored {
			s.addRunArtifact(a)
		}
	}, ids...)}, nil
}

// fetchUpstreamFile reads one file and stores it ("" reason = stored).
func (s *Session) fetchUpstreamFile(ctx context.Context, uq *upstreamQuery, room int64, b UpstreamBounds) ([]domain.ArtifactRef, *upstreamFile, string) {
	if s.cfg.Upstream.Files == nil {
		return nil, nil, "no upstream file fetcher is configured for this run"
	}
	doc, err := s.cfg.Upstream.Files.FetchDocument(ctx, catalog.Locator{Kind: "repo-file", Repository: uq.repo.CloneURL, Ref: uq.q.Ref, Path: uq.q.Path})
	if err != nil {
		return nil, nil, fetchReason(err, uq.q.Ref)
	}
	if why := unreadable(doc.Content); why != "" {
		return nil, nil, why
	}
	if int64(len(doc.Content)) > room {
		return nil, nil, fmt.Sprintf("budget: the file is %d bytes and %d of the item's upstream bytes remain", len(doc.Content), room)
	}
	a, err := s.storeUpstream(ctx, uq, uq.q.Path, doc.Content)
	if err != nil {
		return nil, nil, "storing the fetched file failed: " + short(err.Error())
	}
	lines := splitLines(string(doc.Content))
	f := &upstreamFile{Artifact: a.ID, URI: a.URI, Bytes: len(doc.Content), Lines: len(lines)}
	for i := 0; i < len(lines) && i < b.HeadLines; i++ {
		f.Head = append(f.Head, investigate.LineMatch{Line: i + 1, Text: short(lines[i])})
	}
	f.More = len(lines) > b.HeadLines
	return []domain.ArtifactRef{a}, f, ""
}

// searchUpstream lists a directory at the tag, matches the pattern line by
// line and stores the matching files ("" reason = something was stored).
func (s *Session) searchUpstream(ctx context.Context, uq *upstreamQuery, room int64, b UpstreamBounds) ([]domain.ArtifactRef, *upstreamSearch, string) {
	if s.cfg.Upstream.Dirs == nil {
		return nil, nil, "no upstream directory fetcher is configured for this run"
	}
	docs, err := s.cfg.Upstream.Dirs.FetchDirectory(ctx, catalog.Locator{Kind: "repo-dir", Repository: uq.repo.CloneURL, Ref: uq.q.Ref, Path: uq.q.Dir, Glob: uq.q.Glob})
	if err != nil {
		return nil, nil, fetchReason(err, uq.q.Ref)
	}
	res := &upstreamSearch{FilesListed: len(docs), Matches: []upstreamMatch{}}
	var scanned int64
	type hit struct {
		doc   int
		lines []string
	}
	var files []hit
	for i, d := range docs {
		if res.FilesScanned == b.SearchFiles || scanned+int64(len(d.Content)) > MaxGrepScan {
			res.Truncated = fmt.Sprintf("scanned %d of %d files (cap %d files, %d bytes); narrow dir or glob", res.FilesScanned, len(docs), b.SearchFiles, MaxGrepScan)
			break
		}
		res.FilesScanned++
		scanned += int64(len(d.Content))
		if unreadable(d.Content) != "" {
			continue
		}
		lines := splitLines(string(d.Content))
		matched := false
		for n, l := range lines {
			if !uq.re.MatchString(l) {
				continue
			}
			if len(res.Matches) == b.SearchMatches {
				res.MoreMatches = true
				break
			}
			m := upstreamMatch{Path: d.Path, URI: uq.repo.BlobURL(uq.q.Ref, d.Path), Line: n + 1, Text: short(l)}
			for k := max(0, n-b.SearchContext); k <= min(len(lines)-1, n+b.SearchContext); k++ {
				if k != n {
					m.Context = append(m.Context, investigate.LineMatch{Line: k + 1, Text: short(lines[k])})
				}
			}
			res.Matches = append(res.Matches, m)
			matched = true
		}
		if matched {
			files = append(files, hit{doc: i, lines: lines})
		}
		if res.MoreMatches {
			break
		}
	}
	if len(res.Matches) == 0 {
		return nil, res, fmt.Sprintf("no match: pattern %q matched no line in the %d files scanned under %s", uq.q.Pattern, res.FilesScanned, uq.q.Dir)
	}
	var stored []domain.ArtifactRef
	byPath := map[string]string{}
	var skipped []string
	for _, h := range files {
		d := docs[h.doc]
		if len(stored) == b.SearchStore || int64(len(d.Content)) > room {
			skipped = append(skipped, d.Path)
			continue
		}
		a, err := s.storeUpstream(ctx, uq, d.Path, d.Content)
		if err != nil {
			skipped = append(skipped, d.Path)
			continue
		}
		room -= a.Size
		stored = append(stored, a)
		byPath[d.Path] = a.ID
	}
	for i := range res.Matches {
		res.Matches[i].Artifact = byPath[res.Matches[i].Path]
	}
	if len(skipped) > 0 {
		res.NotStored = fmt.Sprintf("%d matching files were not stored (at most %d per search, within the item's upstream bytes), so their lines cannot be cited: %s", len(skipped), b.SearchStore, listKeys(skipped, 10))
	}
	if len(stored) == 0 {
		return nil, res, "budget: no matching file fits the item's remaining upstream bytes or the per-search store cap"
	}
	return stored, res, ""
}

// storeUpstream stores fetched bytes as an upstream-document artifact; the
// artifact is the same for the same file at the same tag.
func (s *Session) storeUpstream(ctx context.Context, uq *upstreamQuery, p string, content []byte) (domain.ArtifactRef, error) {
	product := string(s.cfg.Bundle.Change.Product.ID)
	repoPath := uq.repo.Path
	if uq.repo.Host != "" && uq.repo.Host != "github.com" {
		repoPath = uq.repo.Host + "/" + repoPath
	}
	return s.cfg.Store.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactUpstreamDocument, Origin: domain.OriginUpstream,
		Path:      "upstream/" + product + "/" + uq.q.Ref + "/source/" + repoPath + "/" + p,
		MediaType: mediaTypeOf(p), URI: uq.repo.BlobURL(uq.q.Ref, p), Producer: UpstreamProducer,
		Tags: []string{"upstream-fetch", strings.ToLower(uq.q.Repository), strings.ToLower(uq.q.Ref)}}, content)
}

func mediaTypeOf(p string) string {
	switch gitsrc.FormatFromPath(p) {
	case "markdown":
		return "text/markdown"
	case "yaml":
		return "application/yaml"
	case "json":
		return "application/json"
	case "html":
		return "text/html"
	}
	return "text/plain"
}

// fetchReason is the recorded reason of a failed fetch. It names the kind of
// failure first; the error text is deterministic for a cached or offline
// replay.
func fetchReason(err error, ref string) string {
	switch {
	case errors.Is(err, fetch.ErrOffline):
		return "offline: not in the recorded cache (" + short(err.Error()) + ")"
	case errors.Is(err, fetch.ErrNotFound):
		return "not found at " + ref + " (" + short(err.Error()) + ")"
	case errors.Is(err, fetch.ErrUnavailable):
		return "upstream unavailable (" + short(err.Error()) + ")"
	}
	return "fetch failed (" + short(err.Error()) + ")"
}

func unreadable(b []byte) string {
	if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
		return "binary file: it cannot be read or quoted"
	}
	return ""
}

func short(s string) string {
	if len(s) > MaxLineText {
		return s[:MaxLineText] + "…"
	}
	return s
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// upstreamGaps are the unknown entries of an item's unfulfilled upstream
// requests, and their ids. A refused request whose question a later request
// of the item answered (same file, or same search) is not a gap.
func (s *Session) upstreamGaps(item string) ([]string, []string) {
	if !s.cfg.Prompt.Features.Upstream.On() {
		return nil, nil
	}
	question := func(r domain.EvidenceRequest) string {
		q := r.Upstream
		return strings.Join([]string{string(r.Kind), strings.ToLower(q.Repository), q.Path, q.Dir, q.Pattern, q.Glob}, "\x00")
	}
	answered := map[string]bool{}
	for _, r := range s.evReqs {
		if r.ChangeItem == item && r.Upstream != nil && r.Status == domain.RequestFulfilled {
			answered[question(r)] = true
		}
	}
	var gaps, ids []string
	for _, r := range s.evReqs {
		if r.ChangeItem != item || r.Upstream == nil || r.Status != domain.RequestUnavailable {
			continue
		}
		if strings.HasPrefix(r.Resolution, "refused:") && answered[question(r)] {
			continue
		}
		gaps = append(gaps, fmt.Sprintf("%s%s unavailable: %s (%s %s, request %s)", upstreamGap, r.Upstream.Ref, r.Resolution, r.Kind, r.Target, r.ID))
		ids = append(ids, r.ID)
	}
	return gaps, ids
}
