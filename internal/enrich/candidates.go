package enrich

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Candidate is a deterministic candidate group: changes of one edge that
// might describe the same thing, explain each other or be related. A
// candidate is NOT a conclusion. It only decides which changes are shown to
// the model together; without a model nothing is concluded from it.
type Candidate struct {
	ID      string   `json:"id"`
	Changes []string `json:"changes"` // change ids, in edge order
	// Signals say why the changes were grouped, e.g.
	// "same-key certificate.spec.privatekey.rotationpolicy: chg-1, chg-2".
	Signals []string `json:"signals"`
	// Hints are the enrichment kinds the signals suggest; the model decides.
	Hints []domain.EnrichmentKind `json:"hints"`
	Score float64                 `json:"score"`
}

// CandidateOptions bound candidate generation.
type CandidateOptions struct {
	// MaxGroupSize caps the changes per group, and so the prompt size (default 6).
	MaxGroupSize int
	// SimilarText is the IDF-weighted overlap two changes from different
	// sources need to be linked by wording alone (default 0.5).
	SimilarText float64
	// GuideNote is the (lower) overlap that links an upgrade-guide item with
	// a release-note item of the same release (default 0.45).
	GuideNote float64
	// WeakLinks is how many weak links (score below strongLink) may attach
	// other changes to one change (default 2). Weak links only ever attach a
	// single change to a group; they never merge two groups.
	WeakLinks int
}

// strongLink separates strong signals (shared key or reference, a diffed key
// named verbatim, very similar wording) from weak ones.
const strongLink = 2.5

// maxPartsSubjects: a computed change with more subjects than this (a schema
// diff adding dozens of fields) is only linked by verbatim key mentions, not
// by matching parts of its keys, which would match almost anything.
const maxPartsSubjects = 6

func (o CandidateOptions) withDefaults() CandidateOptions {
	if o.MaxGroupSize <= 1 {
		o.MaxGroupSize = 6
	}
	if o.SimilarText <= 0 {
		o.SimilarText = 0.5
	}
	if o.GuideNote <= 0 {
		o.GuideNote = 0.45
	}
	if o.WeakLinks <= 0 {
		o.WeakLinks = 2
	}
	return o
}

// profile is what candidate generation knows about one change.
type profile struct {
	c        domain.Change
	idx      int
	sources  map[string]bool
	guide    bool // evidence from an upgrade-guide source
	notes    bool // evidence from a release-notes / changelog source
	computed bool
	keys     map[string]bool // subjects + key-like code spans
	refs     map[string]bool // cve/ghsa/pr/issue/commit ids
	toks     map[string]bool
	text     string // lowercase title + detail
}

type link struct {
	a, b    int
	score   float64
	signals []string
}

// Candidates finds candidate groups in an edge deterministically, from:
//   - shared subjects or key-like code spans (`Certificate.Spec.X`);
//   - shared CVE / GHSA / pull-request / issue / commit references;
//   - normalised, IDF-weighted token similarity between changes from
//     different sources;
//   - a computed diff whose key (or the specific parts of it) is mentioned
//     in a release-note or upgrade-guide item;
//   - upgrade-guide ↔ release-note pairs of the same release.
//
// Linked changes are merged greedily, strongest links first, into groups of
// at most MaxGroupSize changes. Groups are returned strongest first.
func Candidates(e *domain.UpgradeEdge, opts CandidateOptions) []Candidate {
	opts = opts.withDefaults()
	ps := profiles(e)
	idf := idfOf(ps)
	var links []link
	for i := range ps {
		for j := i + 1; j < len(ps); j++ {
			if l, ok := pair(&ps[i], &ps[j], idf, opts); ok {
				links = append(links, l)
			}
		}
	}
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].score != links[j].score {
			return links[i].score > links[j].score
		}
		if links[i].a != links[j].a {
			return links[i].a < links[j].a
		}
		return links[i].b < links[j].b
	})

	parent := make([]int, len(ps))
	size := make([]int, len(ps))
	for i := range parent {
		parent[i], size[i] = i, 1
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	weak := make([]int, len(ps))
	for _, l := range links {
		ra, rb := find(l.a), find(l.b)
		if ra == rb || size[ra]+size[rb] > opts.MaxGroupSize {
			continue
		}
		if l.score < strongLink {
			if size[ra] > 1 && size[rb] > 1 || weak[l.a] >= opts.WeakLinks || weak[l.b] >= opts.WeakLinks {
				continue
			}
			weak[l.a]++
			weak[l.b]++
		}
		if rb < ra {
			ra, rb = rb, ra
		}
		parent[rb] = ra
		size[ra] += size[rb]
	}

	members := map[int][]int{}
	for i := range ps {
		r := find(i)
		members[r] = append(members[r], i)
	}
	groupLinks := map[int][]link{}
	for _, l := range links {
		if r := find(l.a); r == find(l.b) {
			groupLinks[r] = append(groupLinks[r], l)
		}
	}
	var out []Candidate
	for r, idx := range members {
		if len(idx) < 2 {
			continue
		}
		sort.Ints(idx)
		cand := Candidate{}
		var ids []string
		for _, i := range idx {
			cand.Changes = append(cand.Changes, ps[i].c.ID)
			ids = append(ids, ps[i].c.ID)
		}
		cand.ID = "cand-" + domain.ShortHash(ids...)
		for _, l := range groupLinks[r] {
			cand.Score = math.Max(cand.Score, l.score)
			for _, s := range l.signals {
				cand.Signals = append(cand.Signals, fmt.Sprintf("%s: %s, %s", s, ps[l.a].c.ID, ps[l.b].c.ID))
			}
		}
		cand.Hints = hints(ps, idx)
		out = append(out, cand)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func profiles(e *domain.UpgradeEdge) []profile {
	roles := map[string]map[domain.SourceRole]bool{}
	for _, s := range e.Sources {
		if roles[s.SourceID] == nil {
			roles[s.SourceID] = map[domain.SourceRole]bool{}
		}
		for _, r := range s.Roles {
			roles[s.SourceID][r] = true
		}
	}
	evSource := map[domain.EvidenceID]string{}
	for _, x := range e.Evidence {
		evSource[x.ID] = x.SourceID
	}
	ps := make([]profile, 0, len(e.Changes))
	for i, c := range e.Changes {
		p := profile{c: c, idx: i, sources: map[string]bool{}, keys: map[string]bool{}, refs: map[string]bool{},
			computed: c.Provenance.Method == domain.MethodComputed}
		for _, id := range c.Evidence {
			src := evSource[id]
			p.sources[src] = true
			if roles[src][domain.RoleUpgradeGuide] {
				p.guide = true
			}
			if roles[src][domain.RoleReleaseNotes] || roles[src][domain.RoleChangelog] {
				p.notes = true
			}
		}
		text := c.Title + "\n" + c.Detail
		p.text = strings.ToLower(text)
		for _, s := range c.Subjects {
			if k := normKey(s); keyLike(k, s) || p.computed && len(k) >= 3 {
				p.keys[k] = true
			}
		}
		for _, k := range codeKeys(text) {
			p.keys[k] = true
		}
		for _, r := range c.References {
			switch r.Type {
			case "cve", "ghsa":
				p.refs[r.Type+":"+strings.ToUpper(r.ID)] = true
			case "pull-request", "issue", "commit":
				p.refs[r.Type+":"+strings.ToLower(r.ID)] = true
			}
		}
		if !p.computed {
			p.toks = tokens(text)
		}
		ps = append(ps, p)
	}
	return ps
}

// idfOf weighs tokens by inverse document frequency over the note-derived changes.
func idfOf(ps []profile) func(string) float64 {
	df := map[string]int{}
	n := 0
	for _, p := range ps {
		if p.toks == nil {
			continue
		}
		n++
		for t := range p.toks {
			df[t]++
		}
	}
	return func(t string) float64 { return math.Log(1 + float64(n+1)/float64(df[t]+1)) }
}

func disjoint(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return false
		}
	}
	return true
}

func pair(a, b *profile, idf func(string) float64, opts CandidateOptions) (link, bool) {
	l := link{a: a.idx, b: b.idx}
	add := func(score float64, format string, args ...any) {
		l.score += score
		l.signals = append(l.signals, fmt.Sprintf(format, args...))
	}
	for _, k := range sortedKeys(a.keys) {
		if b.keys[k] {
			add(3, "same-key %s", k)
			break
		}
	}
	for _, r := range sortedKeys(a.refs) {
		if b.refs[r] {
			add(3, "same-reference %s", r)
			break
		}
	}
	switch {
	case a.computed && !b.computed:
		diffMention(a, b, add)
	case b.computed && !a.computed:
		diffMention(b, a, add)
	case !a.computed && !b.computed && disjoint(a.sources, b.sources):
		sim, shared := weightedOverlap(a.toks, b.toks, idf)
		pairGuide := (a.guide && b.notes || a.notes && b.guide) && (a.c.Release == b.c.Release || a.c.Release == "" || b.c.Release == "")
		switch {
		case sim >= opts.SimilarText && len(shared) >= 3:
			add(2*sim, "similar-text %.2f", sim)
		case pairGuide && sim >= opts.GuideNote && len(shared) >= 2:
			add(1+sim, "upgrade-guide/release-note %.2f", sim)
		}
	}
	return l, len(l.signals) > 0
}

// diffMention links a computed diff with a note that names its key: the full
// key, its specific last segment, or at least two of its specific parts.
func diffMention(diff, note *profile, add func(float64, string, ...any)) {
	best := 0.0
	label := ""
	set := func(score float64, l string) {
		if score > best {
			best, label = score, l
		}
	}
	for _, s := range diff.c.Subjects {
		k := normKey(s)
		if len(k) < 3 {
			continue
		}
		if keyLike(k, s) && strings.Contains(note.text, k) {
			set(3, "diff-key-mentioned "+k)
			continue
		}
		if !keyLike(k, s) && containsWord(note.text, k) {
			set(2, "diff-key-mentioned "+k) // a plain word such as an image name: weaker
			continue
		}
		raw := lastSegment(s)
		leaf := strings.ToLower(raw)
		if leaf != k && len(leaf) >= 8 && keyLike(leaf, raw) && containsWord(note.text, leaf) {
			set(2, "diff-key-leaf "+leaf)
			continue
		}
		if len(diff.c.Subjects) > maxPartsSubjects {
			continue
		}
		leafParts := toSet(keyTokens(raw)...)
		var matched []string
		long, onLeaf := false, false
		for _, kt := range keyTokens(s) {
			for nt := range note.toks {
				if prefixMatch(kt, nt) {
					matched = append(matched, kt)
					long = long || len(kt) >= 6
					onLeaf = onLeaf || leafParts[kt]
					break
				}
			}
		}
		if len(matched) >= 2 && long && onLeaf {
			set(1.5, "diff-key-parts "+strings.Join(matched, "+"))
		}
	}
	if best > 0 {
		add(best, "%s", label)
	}
}

func lastSegment(s string) string {
	if i := strings.LastIndexAny(s, ".[/"); i >= 0 {
		return strings.Trim(s[i+1:], "[]\"")
	}
	return s
}

// hints derives the kinds the signals suggest for a group.
func hints(ps []profile, idx []int) []domain.EnrichmentKind {
	var computed, notes, migration bool
	sources := map[string]bool{}
	for _, i := range idx {
		p := ps[i]
		if p.computed {
			computed = true
		} else {
			notes = true
		}
		if p.c.ActionRequired || p.c.Breaking || p.c.Category == domain.CategoryMigration {
			migration = true
		}
		for s := range p.sources {
			sources[s] = true
		}
	}
	var out []domain.EnrichmentKind
	if len(sources) > 1 {
		out = append(out, domain.EnrichmentCluster)
	}
	if migration {
		out = append(out, domain.EnrichmentMigrationSummary)
	}
	if computed && notes {
		out = append(out, domain.EnrichmentDiffExplanation)
	}
	return append(out, domain.EnrichmentRelated)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
