package verify

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// CitationsProducer names the citation check (component@vN).
const CitationsProducer = "verify.citations@v1"

// ErrUnresolved: no evidence source holds the cited document.
var ErrUnresolved = errors.New("verify: cited document not in the evidence cache")

// Document is a cited upstream text as an evidence source holds it.
type Document struct {
	// URL is where the bytes were fetched from ("" when the source does not say).
	URL    string
	Digest string
	Body   []byte
	// Source names the evidence source ("bundle-store", "http-cache").
	Source string
}

// Resolver finds the document a citation's evidence names. It never fetches:
// verification uses only what was collected (evidence caches).
type Resolver interface {
	Resolve(ctx context.Context, ev domain.Evidence) (*Document, error)
}

// Resolvers tries each resolver in turn; the first that holds the document wins.
type Resolvers []Resolver

func (rs Resolvers) Resolve(ctx context.Context, ev domain.Evidence) (*Document, error) {
	var errs []error
	for _, r := range rs {
		d, err := r.Resolve(ctx, ev)
		if err == nil {
			return d, nil
		}
		if !errors.Is(err, ErrUnresolved) {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%w: %v", ErrUnresolved, errors.Join(errs...))
	}
	return nil, ErrUnresolved
}

// StoreResolver resolves evidence by content digest in an investigation
// artifact store (the bundle CAS: agent citations carry the cited artifact's
// digest). The store verifies the digest of what it returns.
type StoreResolver struct {
	Store investigate.ArtifactStore
}

func (r StoreResolver) Resolve(ctx context.Context, ev domain.Evidence) (*Document, error) {
	if r.Store == nil || ev.ContentDigest == "" {
		return nil, ErrUnresolved
	}
	rc, err := r.Store.Open(ctx, ev.ContentDigest)
	if errors.Is(err, investigate.ErrNotFound) {
		return nil, ErrUnresolved
	}
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return &Document{Digest: ev.ContentDigest, Body: b, Source: "bundle-store"}, nil
}

// HTTPCacheResolver resolves evidence in the fetch cache
// (<state>/cache/http/<host>/<key>.{json,body.gz}, internal/fetch): by content
// digest first, else by URL (a GitHub blob URL also matches its raw form).
// A cached body whose digest does not match its metadata is never used.
type HTTPCacheResolver struct {
	Dir string // <state>/cache

	once     sync.Once
	byDigest map[string]cacheEntry
	byURL    map[string]cacheEntry
	err      error
}

type cacheEntry struct {
	URL, Digest, body string
}

func (r *HTTPCacheResolver) index() {
	r.byDigest, r.byURL = map[string]cacheEntry{}, map[string]cacheEntry{}
	metas, err := filepath.Glob(filepath.Join(r.Dir, "http", "*", "*.json"))
	if err != nil {
		r.err = err
		return
	}
	for _, m := range metas {
		b, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var meta struct {
			URL    string `json:"url"`
			Status int    `json:"status"`
			Digest string `json:"digest"`
			Method string `json:"method"`
		}
		if json.Unmarshal(b, &meta) != nil || meta.Status < 200 || meta.Status >= 300 || meta.Method == "HEAD" || meta.Digest == "" {
			continue
		}
		e := cacheEntry{URL: meta.URL, Digest: meta.Digest, body: strings.TrimSuffix(m, ".json") + ".body.gz"}
		r.byDigest[e.Digest] = e
		r.byURL[e.URL] = e
	}
}

func (r *HTTPCacheResolver) Resolve(ctx context.Context, ev domain.Evidence) (*Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.once.Do(r.index)
	if r.err != nil {
		return nil, r.err
	}
	e, ok := r.byDigest[ev.ContentDigest]
	if !ok || ev.ContentDigest == "" {
		if ev.ContentDigest != "" {
			// a digest was cited: only those exact bytes resolve it
			return nil, ErrUnresolved
		}
		for _, u := range []string{ev.URI, rawGitHub(ev.URI)} {
			if e, ok = r.byURL[u]; ok && u != "" {
				break
			}
		}
		if !ok {
			return nil, ErrUnresolved
		}
	}
	f, err := os.Open(e.body)
	if err != nil {
		return nil, ErrUnresolved
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	if domain.Digest(b) != e.Digest {
		return nil, fmt.Errorf("http cache: body of %s does not match its digest", e.URL)
	}
	return &Document{URL: e.URL, Digest: e.Digest, Body: b, Source: "http-cache"}, nil
}

// rawGitHub maps https://github.com/<o>/<r>/blob/<ref>/<path> to its
// raw.githubusercontent.com form ("" for any other URL).
func rawGitHub(u string) string {
	rest, ok := strings.CutPrefix(u, "https://github.com/")
	if !ok {
		return ""
	}
	parts := strings.SplitN(rest, "/", 4)
	if len(parts) != 4 || parts[2] != "blob" {
		return ""
	}
	return "https://raw.githubusercontent.com/" + parts[0] + "/" + parts[1] + "/" + parts[3]
}

// versionToken matches a version named in a URL or path: major.minor with an
// optional patch and prerelease, not part of a longer dotted number.
var versionToken = regexp.MustCompile(`(?:^|[^0-9.])v?(\d+)\.(\d+)(?:\.(\d+)(-[0-9A-Za-z]+(?:\.[0-9A-Za-z]+)*)?)?(?:$|[^0-9])`)

// VersionsNamed returns the versions a location names ("1.18", "1.18.0", …).
func VersionsNamed(locations ...string) []string {
	var out []string
	seen := map[string]bool{}
	for _, loc := range locations {
		for _, m := range versionToken.FindAllStringSubmatch(loc, -1) {
			v := m[1] + "." + m[2]
			if m[3] != "" {
				v += "." + m[3] + m[4]
			}
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	return out
}

// inBounds reports whether a named version lies inside the release bounds
// (From exclusive, To inclusive). A minor-only token names a release line: it
// is inside when the line overlaps [From's line, To].
func inBounds(token string, from, to *semver.Version) bool {
	if strings.Count(token, ".") == 1 {
		line, err := semver.NewVersion(token + ".0")
		if err != nil {
			return false
		}
		atLeastFrom := line.Major() > from.Major() || (line.Major() == from.Major() && line.Minor() >= from.Minor())
		return atLeastFrom && !line.GreaterThan(to)
	}
	v, err := semver.NewVersion(token)
	if err != nil {
		return false
	}
	return v.GreaterThan(from) && !v.GreaterThan(to)
}

// subjectTokens are the words a passage stating the rule's behaviour names:
// the last segment of the subject's path, else its name or kind.
func subjectTokens(s *domain.Subject) []string {
	if s == nil {
		return nil
	}
	var out []string
	if s.Path != "" {
		segs := strings.FieldsFunc(s.Path, func(r rune) bool { return r == '.' || r == '[' || r == ']' })
		if len(segs) > 0 {
			out = append(out, segs[len(segs)-1])
		}
	}
	if s.Name != "" {
		out = append(out, s.Name)
	}
	if len(out) == 0 && s.Kind != "" {
		out = append(out, s.Kind)
	}
	return out
}

func mentions(text string, tokens []string) bool {
	t := strings.ToLower(text)
	for _, tok := range tokens {
		if tok != "" && strings.Contains(t, strings.ToLower(tok)) {
			return true
		}
	}
	return false
}

// CheckCitations runs the citation check over a rule's upstream citations:
// each must resolve in the evidence cache (exact bytes when a digest is
// cited), contain its cited passage, and name only versions inside the
// rule's bounds (at least one). Any citation failing fails the method; with
// no bounds or no citation it does not run; when no passing passage names the
// rule's subject it is inconclusive (the passages do not state the rule).
func CheckCitations(ctx context.Context, r Resolver, rule domain.SemanticAssertion, bounds *domain.ProofBounds, evs []domain.Evidence) domain.ProofMethod {
	m := domain.ProofMethod{Method: domain.ProofCitationCheck, Producer: CitationsProducer, Level: domain.LevelDeterministic}
	if bounds == nil {
		m.Outcome, m.Detail = domain.MethodNotRun, "the rule states no release bounds, so no citation can be placed inside them"
		return m
	}
	from, err1 := semver.NewVersion(bounds.From)
	to, err2 := semver.NewVersion(bounds.To)
	if err := errors.Join(err1, err2); err != nil || !from.LessThan(to) {
		m.Outcome, m.Detail = domain.MethodNotRun, fmt.Sprintf("the rule's bounds (%s, %s] are not an increasing semver range", bounds.From, bounds.To)
		return m
	}
	if len(evs) == 0 {
		m.Outcome, m.Detail = domain.MethodNotRun, "the rule cites no upstream text"
		return m
	}
	failed, stating := 0, 0
	tokens := subjectTokens(rule.Subject)
	for _, ev := range evs {
		c := domain.ProofCheck{Subject: string(ev.ID)}
		quote := strings.TrimSuffix(strings.TrimSpace(ev.Excerpt), "…")
		doc, err := r.Resolve(ctx, ev)
		switch {
		case err != nil:
			c.Outcome, c.Detail = domain.MethodFail, fmt.Sprintf("%s does not resolve in the evidence cache (%v)", ev.URI, err)
		case strings.TrimSpace(quote) == "":
			c.Outcome, c.Detail = domain.MethodFail, fmt.Sprintf("%s: the citation carries no passage", ev.URI)
		case !investigate.QuoteOccurs(string(doc.Body), quote):
			c.Outcome, c.Detail = domain.MethodFail, fmt.Sprintf("%s (%s, %s): the cited passage does not occur in the document", ev.URI, doc.Source, doc.Digest)
		default:
			names := VersionsNamed(ev.URI, doc.URL)
			var outside []string
			for _, v := range names {
				if !inBounds(v, from, to) {
					outside = append(outside, v)
				}
			}
			switch {
			case len(names) == 0:
				c.Outcome, c.Detail = domain.MethodFail, fmt.Sprintf("%s names no version, so it cannot be placed inside (%s, %s]", ev.URI, bounds.From, bounds.To)
			case len(outside) > 0:
				c.Outcome, c.Detail = domain.MethodFail, fmt.Sprintf("%s is about %s, outside the rule's bounds (%s, %s]", ev.URI, strings.Join(outside, ", "), bounds.From, bounds.To)
			default:
				c.Outcome = domain.MethodPass
				c.Detail = fmt.Sprintf("%s (%s, %s) contains the passage and names %s, inside (%s, %s]", ev.URI, doc.Source, doc.Digest, strings.Join(names, ", "), bounds.From, bounds.To)
				if mentions(quote, tokens) {
					stating++
				}
				m.Evidence = append(m.Evidence, ev)
			}
		}
		if c.Outcome == domain.MethodFail {
			failed++
		}
		m.Checks = append(m.Checks, c)
	}
	switch {
	case failed > 0:
		m.Outcome = domain.MethodFail
		m.Detail = fmt.Sprintf("%d of %d citations fail: unresolvable, passage missing, or outside the rule's bounds (%s, %s]", failed, len(evs), bounds.From, bounds.To)
		m.Evidence = nil
	case stating == 0:
		m.Outcome = domain.MethodInconclusive
		m.Detail = fmt.Sprintf("every citation resolves inside (%s, %s], but no cited passage names the rule's subject (%s)", bounds.From, bounds.To, strings.Join(tokens, ", "))
		m.Evidence = nil
	default:
		m.Outcome = domain.MethodPass
		m.Detail = fmt.Sprintf("%d citations resolve in the evidence cache, contain their passages and lie inside (%s, %s]; %d name the subject", len(evs), bounds.From, bounds.To, stating)
	}
	return m
}
