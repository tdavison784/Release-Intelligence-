package renderdiff

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// Query bounds (DESIGN §4.1: results are something an agent can cite, never
// an unbounded dump).
const (
	DefaultLimit = 50
	MaxLimit     = 200
	// MaxPageBytes bounds one page's JSON size; a page holds at least one change.
	MaxPageBytes = 256 << 10
	// MaxObjectBytes bounds one Object result; larger objects must be read by field path.
	MaxObjectBytes = 64 << 10
)

// DiffPage is one bounded page of a diff query.
type DiffPage struct {
	Changes []domain.RenderedChange `json:"changes"`
	// Total is the number of matching changes across all pages.
	Total int `json:"total"`
	// Next, when set, continues the query: pass it as DiffQuery.After.
	Next string `json:"next,omitempty"`
}

// Query returns the first page of rendered changes matching q (see
// QueryPage). The store is not needed — changes are served from the
// bundle's index; it is accepted for the DESIGN §7 signature.
func Query(b *domain.EvidenceBundle, _ investigate.ArtifactStore, q investigate.DiffQuery) ([]domain.RenderedChange, error) {
	p, err := QueryPage(b, q)
	if err != nil {
		return nil, err
	}
	return p.Changes, nil
}

// QueryPage filters the bundle's rendered changes. Every set field of q must
// match; list fields match any element. Results are ordered by (target,
// object, path, class, id) — a fixed order, so q.After (the id of the last
// change of the previous page) continues without overlap or gaps. Filters
// that name something the bundle does not have (an unknown scope, target,
// change item or class, an unknown cursor) are rejected rather than
// answered with an empty page: an empty result must mean "no such change",
// never "you mistyped".
func QueryPage(b *domain.EvidenceBundle, q investigate.DiffQuery) (*DiffPage, error) {
	if b == nil {
		return nil, fmt.Errorf("%w: no bundle", investigate.ErrNotFound)
	}
	pairs, err := pairsOf(b, q.Scope)
	if err != nil {
		return nil, err
	}
	if q.Target != "" && findPair(pairs, q.Target) == nil {
		return nil, rejectf("unknown %s render target %q (targets: %s)", scopeName(q.Scope), q.Target, strings.Join(targetNames(pairs), ", "))
	}
	if q.ChangeItem != "" {
		if _, ok := b.Change.Item(q.ChangeItem); !ok {
			return nil, rejectf("unknown change item %q", q.ChangeItem)
		}
	}
	for _, c := range q.Classes {
		if !knownClass(c) {
			return nil, rejectf("unknown change class %q", c)
		}
	}
	limit := q.Limit
	switch {
	case limit < 0:
		return nil, rejectf("limit %d is negative", limit)
	case limit == 0:
		limit = DefaultLimit
	case limit > MaxLimit:
		limit = MaxLimit
	}
	type hit struct {
		target string
		c      domain.RenderedChange
	}
	var hits []hit
	for _, p := range pairs {
		if q.Target != "" && p.Target != q.Target {
			continue
		}
		for _, c := range p.Changes {
			if matches(c, q) {
				hits = append(hits, hit{p.Target, c})
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		for _, k := range [][2]string{{a.target, b.target}, {a.c.Object, b.c.Object}, {a.c.Path, b.c.Path}, {a.c.Class, b.c.Class}, {a.c.ID, b.c.ID}} {
			if k[0] != k[1] {
				return k[0] < k[1]
			}
		}
		return false
	})
	start := 0
	if q.After != "" {
		start = -1
		for i, h := range hits {
			if h.c.ID == q.After {
				start = i + 1
				break
			}
		}
		if start < 0 {
			return nil, rejectf("continuation %q is not a change of this query's result", q.After)
		}
	}
	page := &DiffPage{Changes: []domain.RenderedChange{}, Total: len(hits)}
	size := 0
	for i := start; i < len(hits); i++ {
		if len(page.Changes) == limit {
			page.Next = page.Changes[len(page.Changes)-1].ID
			break
		}
		n := jsonSize(hits[i].c)
		if len(page.Changes) > 0 && size+n > MaxPageBytes {
			page.Next = page.Changes[len(page.Changes)-1].ID
			break
		}
		size += n
		page.Changes = append(page.Changes, hits[i].c)
	}
	return page, nil
}

func matches(c domain.RenderedChange, q investigate.DiffQuery) bool {
	if q.ChangeItem != "" && !contains(c.ChangeItems, q.ChangeItem) {
		return false
	}
	if q.Uncorrelated && len(c.ChangeItems) > 0 {
		return false
	}
	if len(q.Classes) > 0 && !contains(q.Classes, c.Class) {
		return false
	}
	_, _, kind, ns, _ := splitObject(c.Object)
	if len(q.Kinds) > 0 && !containsFold(q.Kinds, kind) {
		return false
	}
	if ns == "" {
		ns = "-"
	}
	if len(q.Namespaces) > 0 && !contains(q.Namespaces, ns) {
		return false
	}
	return q.PathPrefix == "" || strings.HasPrefix(c.Path, q.PathPrefix)
}

// Object reads one object, or one field of it, from the from or to render of
// a pair (environment targets first, then release targets such as
// "chart-defaults"). The YAML is re-encoded from the stored (redacted)
// render, so its text occurs in the artifact bytes up to whitespace: cite
// it with Citation{Artifact: result.Artifact.ID, Locator: path, Quote}. An
// object or path absent on that side is Found=false (a fact, not an error);
// a side that was not rendered is ErrNotFound with the pair's reason.
func Object(ctx context.Context, b *domain.EvidenceBundle, st investigate.ArtifactStore, r investigate.ObjectRequest) (*investigate.ObjectResult, error) {
	if b == nil || st == nil {
		return nil, fmt.Errorf("%w: no bundle or artifact store", investigate.ErrNotFound)
	}
	p := findPair(b.Renders, r.Target)
	if p == nil {
		p = findPair(b.Change.Renders, r.Target)
	}
	if p == nil {
		return nil, rejectf("unknown render target %q", r.Target)
	}
	var id string
	switch r.Side {
	case "from":
		id = p.From
	case "to":
		id = p.To
	default:
		return nil, rejectf("side %q: want from or to", r.Side)
	}
	if id == "" {
		return nil, fmt.Errorf("%w: target %s has no %s render (status %s: %s)", investigate.ErrNotFound, p.Target, r.Side, p.Status, p.Reason)
	}
	ref, ok := b.Manifest.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("%w: render artifact %s is not in the manifest", investigate.ErrNotFound, id)
	}
	var segs []pathSeg
	if r.Path != "" {
		var err error
		if segs, err = parsePath(r.Path); err != nil {
			return nil, rejectf("%v", err)
		}
	}
	rc, err := st.Open(ctx, ref.Digest)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	stream, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	obj, err := lookupObject(stream, r.Object)
	if err != nil {
		return nil, err
	}
	res := &investigate.ObjectResult{Artifact: ref}
	if obj == nil {
		return res, nil
	}
	v, found := resolve(obj.Body, segs)
	if !found {
		return res, nil
	}
	out, err := marshalYAML(v)
	if err != nil {
		return nil, err
	}
	if len(out) > MaxObjectBytes {
		return nil, rejectf("%s %s is %d bytes (max %d): read a field of it by path (top-level keys: %s)",
			r.Object, orWhole(r.Path), len(out), MaxObjectBytes, strings.Join(keysOf(v), ", "))
	}
	res.YAML, res.Found = string(out), true
	return res, nil
}

// Summaries describes every render pair of the bundle (environment scope,
// then release scope), each sorted by target.
func Summaries(b *domain.EvidenceBundle) []investigate.RenderSummary {
	if b == nil {
		return nil
	}
	var out []investigate.RenderSummary
	for _, ps := range [][]domain.ManifestPair{b.Renders, b.Change.Renders} {
		sorted := append([]domain.ManifestPair(nil), ps...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Target < sorted[j].Target })
		for _, p := range sorted {
			s := investigate.RenderSummary{Scope: p.Scope, Target: p.Target, Status: p.Status, Reason: p.Reason, Changes: len(p.Changes)}
			for _, c := range p.Changes {
				if s.ByClass == nil {
					s.ByClass = map[string]int{}
				}
				s.ByClass[c.Class]++
				if len(c.ChangeItems) == 0 {
					s.Uncorrelated++
				}
			}
			out = append(out, s)
		}
	}
	return out
}

func pairsOf(b *domain.EvidenceBundle, scope domain.RenderScope) ([]domain.ManifestPair, error) {
	switch scope {
	case "", domain.RenderEnvironment:
		return b.Renders, nil
	case domain.RenderRelease:
		return b.Change.Renders, nil
	}
	return nil, rejectf("unknown scope %q (want %s or %s)", scope, domain.RenderEnvironment, domain.RenderRelease)
}

func scopeName(s domain.RenderScope) string {
	if s == "" {
		return string(domain.RenderEnvironment)
	}
	return string(s)
}

func findPair(ps []domain.ManifestPair, target string) *domain.ManifestPair {
	for i := range ps {
		if ps[i].Target == target {
			return &ps[i]
		}
	}
	return nil
}

func targetNames(ps []domain.ManifestPair) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Target)
	}
	sort.Strings(out)
	return out
}

func knownClass(c string) bool {
	for _, k := range render.ChangeClasses {
		if string(k) == c {
			return true
		}
	}
	return false
}

func rejectf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", investigate.ErrRejected, fmt.Sprintf(format, args...))
}

func jsonSize(v any) int {
	b, _ := json.Marshal(v)
	return len(b)
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func containsFold(xs []string, s string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func keysOf(v any) []string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func orWhole(path string) string {
	if path == "" {
		return "(whole object)"
	}
	return path
}
