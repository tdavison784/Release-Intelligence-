package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/renderdiff"
)

// --- get_change_item ------------------------------------------------------------------

func (s *Session) upgradeEdge(ctx context.Context) (*domain.UpgradeEdge, error) {
	if s.edge != nil || s.edgeErr != nil {
		return s.edge, s.edgeErr
	}
	a, ok := s.artifact(s.cfg.Bundle.Change.Edge)
	if !ok {
		s.edgeErr = fmt.Errorf("%w: upgrade-edge artifact %s is not in the manifest", investigate.ErrNotFound, s.cfg.Bundle.Change.Edge)
		return nil, s.edgeErr
	}
	b, err := s.bytesOf(ctx, a)
	if err != nil {
		return nil, err // transient store failures are not cached
	}
	var e domain.UpgradeEdge
	if err := json.Unmarshal(b, &e); err != nil {
		s.edgeErr = fmt.Errorf("upgrade-edge artifact %s: %w", a.ID, err)
		return nil, s.edgeErr
	}
	s.edge = &e
	return s.edge, nil
}

func (s *Session) getChangeItem(ctx context.Context, id string) (*outcome, error) {
	it, err := s.item(id)
	if err != nil {
		return nil, err
	}
	d := &investigate.ChangeItemDetail{Item: it, Evidence: []domain.Evidence{}}
	edge, err := s.upgradeEdge(ctx)
	if err != nil {
		return nil, err
	}
	if it.ChangeID != "" {
		for i := range edge.Changes {
			if edge.Changes[i].ID == it.ChangeID {
				c := edge.Changes[i]
				d.Change = &c
				break
			}
		}
	}
	if it.Platform != "" {
		for i := range edge.Compatibility {
			if edge.Compatibility[i].Platform == it.Platform {
				c := edge.Compatibility[i]
				d.Compatibility = &c
				break
			}
		}
	}
	uris := map[string]bool{}
	for _, eid := range it.Evidence {
		if e, ok := s.pool[eid]; ok {
			d.Evidence = append(d.Evidence, e)
			uris[e.URI] = true
		}
	}
	for _, a := range s.cfg.Bundle.Manifest.Artifacts {
		if a.Origin == domain.OriginUpstream && (contains(a.ChangeItems, it.ID) || uris[a.URI] || contains(it.Artifacts, a.ID)) {
			d.Sources = append(d.Sources, a)
		}
	}
	for _, eid := range s.cfg.Bundle.Change.MigrationNotes {
		if e, ok := s.pool[eid]; ok {
			d.MigrationNotes = append(d.MigrationNotes, e)
		}
	}
	for _, ps := range [][]domain.ManifestPair{s.cfg.Bundle.Change.Renders, s.cfg.Bundle.Renders} {
		for _, p := range ps {
			for _, c := range p.Changes {
				if contains(c.ChangeItems, it.ID) || c.ID == it.RenderedChange {
					d.RenderedChanges = append(d.RenderedChanges, c)
				}
			}
		}
	}
	if r := s.cfg.Report; r != nil && it.ChangeID != "" {
		for _, f := range r.Findings {
			if f.ChangeID == it.ChangeID {
				d.ImpactFindings = append(d.ImpactFindings, domain.ImpactFindingRef{ID: f.ID, Classification: f.Classification, Rule: f.Rule})
			}
		}
	}
	return &outcome{value: d}, nil
}

// --- renders and diffs (renderdiff's query engine, DESIGN.md §7) ------------------------

func (s *Session) listRenders(context.Context) (*outcome, error) {
	sums := renderdiff.Summaries(s.cfg.Bundle)
	if sums == nil {
		sums = []investigate.RenderSummary{}
	}
	return &outcome{value: sums}, nil
}

func (s *Session) queryDiff(q investigate.DiffQuery) (*outcome, error) {
	page, err := renderdiff.QueryPage(s.cfg.Bundle, q)
	if err != nil {
		return nil, err
	}
	if page.Changes == nil {
		page.Changes = []domain.RenderedChange{}
	}
	return &outcome{value: page.Changes, view: page}, nil
}

func (s *Session) getRenderedObject(ctx context.Context, r investigate.ObjectRequest) (*outcome, error) {
	res, err := renderdiff.Object(ctx, s.cfg.Bundle, s.cfg.Store, r)
	if err != nil {
		return nil, err
	}
	return &outcome{value: res}, nil
}

// --- search_manifest -------------------------------------------------------------------

func (s *Session) allArtifacts() []domain.ArtifactRef {
	out := append(append([]domain.ArtifactRef(nil), s.cfg.Bundle.Manifest.Artifacts...), s.runArts...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (s *Session) searchManifest(ctx context.Context, q investigate.SearchQuery) (*outcome, error) {
	limit := q.Limit
	switch {
	case limit == 0:
		limit = DefaultSearchLimit
	case limit < 0 || limit > MaxSearchLimit:
		return nil, rejectf("limit %d: want 1..%d", limit, MaxSearchLimit)
	}
	if q.ChangeItem != "" {
		if _, err := s.item(q.ChangeItem); err != nil {
			return nil, err
		}
	}
	for _, k := range q.Kinds {
		if !oneOfKinds(k) {
			return nil, rejectf("unknown artifact kind %q", k)
		}
	}
	var re *regexp.Regexp
	if q.Grep != "" {
		if len(q.Grep) > MaxGrepPattern {
			return nil, rejectf("grep pattern is %d bytes (max %d)", len(q.Grep), MaxGrepPattern)
		}
		var err error
		if re, err = regexp.Compile(q.Grep); err != nil {
			return nil, rejectf("grep: %v", err)
		}
	}
	terms := make([]string, 0, len(q.Terms))
	for _, t := range q.Terms {
		if t = strings.ToLower(trimmed(t)); t != "" {
			terms = append(terms, t)
		}
	}
	hits := []investigate.SearchHit{}
	scanned := 0
	for _, a := range s.allArtifacts() {
		if len(hits) == limit {
			break
		}
		if len(q.Kinds) > 0 && !kindIn(a.Kind, q.Kinds) {
			continue
		}
		if q.ChangeItem != "" && !contains(a.ChangeItems, q.ChangeItem) {
			continue
		}
		if !matchesTerms(a, terms) {
			continue
		}
		hit := investigate.SearchHit{Artifact: a}
		if re != nil {
			if scanned >= MaxGrepScan {
				break
			}
			b, err := s.bytesOf(ctx, a)
			if err != nil {
				return nil, err
			}
			if isBinary(b) {
				continue
			}
			scanned += len(b)
			for i, line := range strings.Split(string(b), "\n") {
				if re.MatchString(line) {
					hit.Lines = append(hit.Lines, investigate.LineMatch{Line: i + 1, Text: clipLine(line)})
					if len(hit.Lines) == MaxGrepLines {
						break
					}
				}
			}
			if len(hit.Lines) == 0 {
				continue
			}
		}
		hits = append(hits, hit)
	}
	return &outcome{value: hits}, nil
}

func matchesTerms(a domain.ArtifactRef, terms []string) bool {
	path := strings.ToLower(a.Path)
	for _, t := range terms {
		ok := strings.Contains(path, t) || strings.EqualFold(string(a.Kind), t)
		for _, tag := range a.Tags {
			if ok {
				break
			}
			ok = strings.Contains(strings.ToLower(tag), t)
		}
		if !ok {
			return false
		}
	}
	return true
}

func kindIn(k domain.ArtifactKind, ks []domain.ArtifactKind) bool {
	for _, x := range ks {
		if x == k {
			return true
		}
	}
	return false
}

func oneOfKinds(k domain.ArtifactKind) bool { return kindIn(k, domain.ArtifactKinds) }

func clipLine(l string) string {
	if len(l) <= MaxLineText {
		return l
	}
	return clip(l, MaxLineText)
}

func isBinary(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return !utf8.Valid(b)
}

// --- read_artifact ---------------------------------------------------------------------

func (s *Session) readArtifact(ctx context.Context, r investigate.ReadRequest) (*outcome, error) {
	a, ok := s.artifact(r.Artifact)
	if !ok {
		return nil, rejectf("unknown artifact %q", r.Artifact)
	}
	text, err := s.textOf(ctx, a)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // a trailing newline ends the last line
	}
	total := len(lines)
	from, to := r.FromLine, r.ToLine
	if r.YAMLPath != "" {
		if from != 0 || to != 0 {
			return nil, rejectf("give either a line window or yamlPath, not both")
		}
		if from, to, err = yamlSpan(text, r.YAMLPath); err != nil {
			return nil, err
		}
	}
	if from == 0 {
		from = 1
	}
	if from < 1 || from > total {
		return nil, rejectf("fromLine %d: artifact %s has %d lines", from, a.ID, total)
	}
	if to == 0 {
		to = from + MaxReadLines - 1
	}
	if to < from {
		return nil, rejectf("toLine %d is before fromLine %d", to, from)
	}
	res := &investigate.ReadResult{Artifact: a, FromLine: from, TotalLines: total}
	if to > total {
		to = total
	}
	if to-from+1 > MaxReadLines {
		to, res.Truncated = from+MaxReadLines-1, true
	}
	var b strings.Builder
	end := from - 1
	for i := from; i <= to; i++ {
		l := lines[i-1]
		if b.Len()+len(l)+1 > MaxReadWindow {
			if end < from { // a single line longer than the window: its head
				cut := MaxReadWindow
				for cut > 0 && (l[cut]&0xC0) == 0x80 {
					cut--
				}
				b.WriteString(l[:cut])
				end = i
			}
			res.Truncated = true
			break
		}
		if i > from {
			b.WriteByte('\n')
		}
		b.WriteString(l)
		end = i
	}
	if end < to && !res.Truncated {
		res.Truncated = true
	}
	res.ToLine, res.Text = end, b.String()
	return &outcome{value: res}, nil
}

// yamlSpan resolves a YAML path to the line span of its node in the
// original text (so the returned text occurs verbatim and can be quoted).
// Syntax: an optional leading [n] selects the n-th document (0-based); then
// dot-separated keys, each optionally followed by [i] sequence indices.
func yamlSpan(text, path string) (int, int, error) {
	docIdx := 0
	p := path
	if strings.HasPrefix(p, "[") {
		end := strings.IndexByte(p, ']')
		if end < 0 {
			return 0, 0, rejectf("yamlPath %q: unterminated [", path)
		}
		n, err := strconv.Atoi(p[1:end])
		if err != nil || n < 0 {
			return 0, 0, rejectf("yamlPath %q: bad document index", path)
		}
		docIdx, p = n, strings.TrimPrefix(p[end+1:], ".")
	}
	dec := yaml.NewDecoder(strings.NewReader(text))
	var doc yaml.Node
	for i := 0; ; i++ {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			return 0, 0, rejectf("yamlPath %q: document %d not found (the artifact has %d)", path, docIdx, i)
		}
		if err != nil {
			return 0, 0, rejectf("yamlPath %q: the artifact is not YAML: %v", path, err)
		}
		if i == docIdx {
			doc = n
			break
		}
	}
	cur := &doc
	if cur.Kind == yaml.DocumentNode && len(cur.Content) > 0 {
		cur = cur.Content[0]
	}
	start := cur.Line
	if p != "" {
		for _, seg := range strings.Split(p, ".") {
			name, idx, err := splitSeg(seg)
			if err != nil {
				return 0, 0, rejectf("yamlPath %q: %v", path, err)
			}
			if name != "" {
				if cur.Kind != yaml.MappingNode {
					return 0, 0, rejectf("yamlPath %q: %q is not a mapping key here", path, name)
				}
				var next *yaml.Node
				for i := 0; i+1 < len(cur.Content); i += 2 {
					if cur.Content[i].Value == name {
						start, next = cur.Content[i].Line, cur.Content[i+1]
						break
					}
				}
				if next == nil {
					return 0, 0, rejectf("yamlPath %q: key %q not found", path, name)
				}
				cur = next
			}
			for _, i := range idx {
				if cur.Kind != yaml.SequenceNode || i >= len(cur.Content) {
					return 0, 0, rejectf("yamlPath %q: index [%d] out of range", path, i)
				}
				cur = cur.Content[i]
				start = cur.Line
			}
		}
	}
	return start, maxLine(cur), nil
}

func splitSeg(seg string) (string, []int, error) {
	name := seg
	var idx []int
	if i := strings.IndexByte(seg, '['); i >= 0 {
		name = seg[:i]
		rest := seg[i:]
		for rest != "" {
			if rest[0] != '[' {
				return "", nil, fmt.Errorf("bad segment %q", seg)
			}
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				return "", nil, fmt.Errorf("unterminated [ in %q", seg)
			}
			n, err := strconv.Atoi(rest[1:end])
			if err != nil || n < 0 {
				return "", nil, fmt.Errorf("bad index in %q", seg)
			}
			idx = append(idx, n)
			rest = rest[end+1:]
		}
	}
	return name, idx, nil
}

func maxLine(n *yaml.Node) int {
	m := n.Line
	if n.Kind == yaml.ScalarNode && (n.Style == yaml.LiteralStyle || n.Style == yaml.FoldedStyle) {
		m += strings.Count(strings.TrimRight(n.Value, "\n"), "\n") + 1
	}
	for _, c := range n.Content {
		if l := maxLine(c); l > m {
			m = l
		}
	}
	return m
}

// --- expand_graph / list_paths -----------------------------------------------------------

func (s *Session) expandGraph(r investigate.ExpandRequest) (*outcome, error) {
	if s.cfg.Graph == nil {
		return nil, rejectf("this investigation has no relationship graph")
	}
	start, ok := s.node(r.Node)
	if !ok {
		return nil, rejectf("unknown node %q (not a node id or key of the relationship graph)", r.Node)
	}
	depth := r.Depth
	switch {
	case depth == 0:
		depth = 1
	case depth < 0 || depth > MaxExpandDepth:
		return nil, rejectf("depth %d: want 1..%d", depth, MaxExpandDepth)
	}
	dir := r.Direction
	if dir == "" {
		dir = "out"
	}
	if dir != "out" && dir != "in" && dir != "both" {
		return nil, rejectf("direction %q: want out, in or both", dir)
	}
	for _, k := range r.EdgeKinds {
		if !edgeKindKnown(k) {
			return nil, rejectf("unknown edge kind %q", k)
		}
	}
	edges := s.sortedEdges()
	seen := map[string]bool{start: true}
	inSub := map[string]bool{}
	frontier := []string{start}
	for d := 0; d < depth && len(frontier) > 0; d++ {
		var next []string
		for _, n := range frontier {
			for _, e := range edges {
				if len(r.EdgeKinds) > 0 && !edgeKindIn(e.Kind, r.EdgeKinds) {
					continue
				}
				var other string
				switch {
				case (dir == "out" || dir == "both") && e.From == n:
					other = e.To
				case (dir == "in" || dir == "both") && e.To == n:
					other = e.From
				default:
					continue
				}
				inSub[e.ID] = true
				if !seen[other] {
					seen[other] = true
					next = append(next, other)
				}
			}
		}
		if len(seen) > MaxExpandNodes {
			return nil, rejectf("the neighbourhood of %s at depth %d has more than %d nodes: reduce depth or filter edgeKinds", start, d+1, MaxExpandNodes)
		}
		frontier = next
	}
	sub := &investigate.Subgraph{Nodes: []domain.GraphNode{}, Edges: []domain.GraphEdge{}}
	for id := range seen {
		sub.Nodes = append(sub.Nodes, s.nodes[id])
	}
	sort.Slice(sub.Nodes, func(i, j int) bool { return sub.Nodes[i].ID < sub.Nodes[j].ID })
	for _, e := range edges {
		if inSub[e.ID] {
			sub.Edges = append(sub.Edges, e)
		}
	}
	return &outcome{value: sub}, nil
}

func (s *Session) sortedEdges() []domain.GraphEdge {
	out := make([]domain.GraphEdge, 0, len(s.edges))
	for _, e := range s.edges {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func edgeKindKnown(k domain.EdgeKind) bool { return edgeKindIn(k, domain.EdgeKinds) }

func edgeKindIn(k domain.EdgeKind, ks []domain.EdgeKind) bool {
	for _, x := range ks {
		if x == k {
			return true
		}
	}
	return false
}

func (s *Session) listPaths(q investigate.PathQuery) (*outcome, error) {
	if s.cfg.Traversal == nil {
		return nil, rejectf("this investigation has no traversal")
	}
	if q.ChangeItem != "" {
		if _, err := s.item(q.ChangeItem); err != nil {
			return nil, err
		}
	}
	out := []domain.ImpactPath{}
	for _, p := range s.cfg.Traversal.Paths {
		if q.ChangeItem != "" && p.ChangeItem != q.ChangeItem {
			continue
		}
		if q.Unresolved && p.Status.Resolved() {
			continue
		}
		out = append(out, p)
	}
	return &outcome{value: out}, nil
}
