package proxyreview

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ReleaseSource returns an ingested release (nil, nil when absent). The proxy
// reads only upstream, release-level data from it: the notes around the cited
// text (prompt v3, lever L2: decide from the whole upstream section instead of
// closing aspects for lack of evidence).
type ReleaseSource func(product domain.ProductID, version string) (*domain.Release, error)

// Section-context bounds.
const (
	maxSectionChars      = 9000  // per section
	maxSectionTotal      = 18000 // all sections of one prompt
	maxNoteChars         = 1200  // one note
	maxGuideSections     = 2     // related upgrade-guide sections
	minTermLen           = 5     // a subject term must be this distinctive to pull in an upgrade-guide section
	sectionPathSeparator = " › "
)

// SectionRef records one section shown (on the request, for audit).
type SectionRef struct {
	SourceID  string            `json:"sourceId"`
	Role      domain.SourceRole `json:"role"`
	Section   string            `json:"section"`
	Why       string            `json:"why"` // "cited" | "upgrade-guide mentions <term>"
	Notes     int               `json:"notes"`
	Shown     int               `json:"shown"`
	Truncated bool              `json:"truncated,omitempty"`
}

type sectionBlock struct {
	ref   SectionRef
	notes []domain.NoteItem
	cited map[string]bool
}

type sectionKey struct{ source, section string }

func inSection(n domain.NoteItem, k sectionKey) bool {
	return n.SourceID == k.source && (n.Section == k.section || strings.HasPrefix(n.Section, k.section+sectionPathSeparator))
}

// sectionContext finds the sections of rel the candidate's evidence comes
// from (every note citing a candidate evidence id), plus up to
// maxGuideSections upgrade-guide sections that mention a distinctive term of
// the proposed subject, when the cited text is not itself an upgrade guide.
func sectionContext(rel *domain.Release, c domain.SemanticCandidate, proposed domain.SemanticAssertion) []sectionBlock {
	if rel == nil || len(rel.Notes) == 0 {
		return nil
	}
	ids := map[domain.EvidenceID]bool{}
	for _, e := range c.Evidence {
		ids[e.ID] = true
	}
	var keys []sectionKey
	cited := map[string]bool{}
	seenKey := map[sectionKey]bool{}
	citedGuide := false
	for _, n := range rel.Notes {
		hit := false
		for _, id := range n.Evidence {
			hit = hit || ids[id]
		}
		if !hit {
			continue
		}
		cited[n.ID] = true
		citedGuide = citedGuide || n.Role == domain.RoleUpgradeGuide
		k := sectionKey{n.SourceID, n.Section}
		if !seenKey[k] {
			seenKey[k] = true
			keys = append(keys, k)
		}
	}
	var out []sectionBlock
	for _, k := range keys {
		b := sectionBlock{cited: cited, ref: SectionRef{SourceID: k.source, Section: k.section, Why: "cited"}}
		for _, n := range rel.Notes {
			if inSection(n, k) {
				b.notes = append(b.notes, n)
				b.ref.Role = n.Role
			}
		}
		out = append(out, b)
	}
	if !citedGuide {
		for _, term := range subjectTerms(proposed, c) {
			if countWhy(out, "upgrade-guide") >= maxGuideSections {
				break
			}
			lt := strings.ToLower(term)
			for _, n := range rel.Notes {
				if n.Role != domain.RoleUpgradeGuide || !strings.Contains(strings.ToLower(n.Text), lt) {
					continue
				}
				k := sectionKey{n.SourceID, n.Section}
				if seenKey[k] {
					break
				}
				seenKey[k] = true
				b := sectionBlock{cited: map[string]bool{n.ID: true}, ref: SectionRef{SourceID: k.source, Role: n.Role, Section: k.section,
					Why: fmt.Sprintf("upgrade-guide mentions %q", term)}}
				for _, m := range rel.Notes {
					if inSection(m, k) {
						b.notes = append(b.notes, m)
					}
				}
				out = append(out, b)
				break
			}
		}
	}
	return out
}

func countWhy(bs []sectionBlock, prefix string) int {
	n := 0
	for _, b := range bs {
		if strings.HasPrefix(b.ref.Why, prefix) {
			n++
		}
	}
	return n
}

// subjectTerms are distinctive identity terms of the proposed subject (the
// last path segment, the name), then the candidate's hints: tokens an
// upgrade guide would use to talk about the same thing.
func subjectTerms(a domain.SemanticAssertion, c domain.SemanticCandidate) []string {
	var terms []string
	seen := map[string]bool{}
	add := func(t string) {
		t = strings.Trim(strings.TrimSpace(t), "-`'\".,")
		if len(t) < minTermLen || seen[strings.ToLower(t)] {
			return
		}
		seen[strings.ToLower(t)] = true
		terms = append(terms, t)
	}
	if s := a.Subject; s != nil {
		if s.Path != "" {
			segs := strings.Split(s.Path, ".")
			add(segs[len(segs)-1])
		}
		add(s.Name)
		add(s.Kind)
	}
	for _, h := range c.Hints {
		add(h)
	}
	return terms
}

// renderSections writes the section context and returns the evidence ids it
// showed (citable). Each section is cut to a window around its cited notes.
func renderSections(b *strings.Builder, rel *domain.Release, blocks []sectionBlock) ([]domain.EvidenceID, []SectionRef) {
	if len(blocks) == 0 {
		return nil, nil
	}
	evByID := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range rel.Evidence {
		evByID[e.ID] = e
	}
	b.WriteString("\nUPSTREAM SECTION CONTEXT (the whole upstream section the cited text comes from, and upgrade-guide sections that\n" +
		"mention the subject; release-level text from the ingested release, not a customer environment. ► marks the cited item.)\n")
	var ids []domain.EvidenceID
	var refs []SectionRef
	total := 0
	for _, blk := range blocks {
		if total >= maxSectionTotal {
			break
		}
		budget := maxSectionChars
		if rest := maxSectionTotal - total; rest < budget {
			budget = rest
		}
		lines := make([]string, len(blk.notes))
		for i, n := range blk.notes {
			mark := "  "
			if blk.cited[n.ID] {
				mark = "► "
			}
			id := ""
			if len(n.Evidence) > 0 {
				id = "[" + string(n.Evidence[0]) + "] "
			}
			lines[i] = mark + id + shorten(oneLine(n.Text), maxNoteChars)
		}
		keep := window(lines, blk, budget)
		ref := blk.ref
		ref.Notes, ref.Shown = len(blk.notes), len(keep)
		ref.Truncated = len(keep) < len(blk.notes)
		uri := ""
		if len(blk.notes) > 0 && len(blk.notes[0].Evidence) > 0 {
			if e, ok := evByID[blk.notes[0].Evidence[0]]; ok {
				uri = " uri=" + e.URI
			}
		}
		fmt.Fprintf(b, "--- %s · source %s · section %q (%s; %d items", ref.Role, ref.SourceID, ref.Section, ref.Why, ref.Notes)
		if ref.Truncated {
			fmt.Fprintf(b, ", %d shown around the cited item", ref.Shown)
		}
		fmt.Fprintf(b, ")%s\n", uri)
		for _, i := range keep {
			b.WriteString(lines[i])
			b.WriteString("\n")
			total += len(lines[i]) + 1
			for _, id := range blk.notes[i].Evidence {
				ids = append(ids, id)
				break
			}
		}
		refs = append(refs, ref)
	}
	return ids, refs
}

// window keeps the cited lines and grows outward from them, alternating
// before/after, until the budget is spent; indices are returned in order.
func window(lines []string, blk sectionBlock, budget int) []int {
	var cited []int
	for i, n := range blk.notes {
		if blk.cited[n.ID] {
			cited = append(cited, i)
		}
	}
	if len(cited) == 0 {
		cited = []int{0}
	}
	keep := map[int]bool{}
	used := 0
	for _, i := range cited {
		if !keep[i] {
			keep[i] = true
			used += len(lines[i]) + 1
		}
	}
	lo, hi := cited[0]-1, cited[len(cited)-1]+1
	for i := cited[0]; i <= cited[len(cited)-1]; i++ { // the span between cited items first
		if !keep[i] && used+len(lines[i])+1 <= budget {
			keep[i] = true
			used += len(lines[i]) + 1
		}
	}
	for lo >= 0 || hi < len(lines) {
		grew := false
		if hi < len(lines) && used+len(lines[hi])+1 <= budget {
			keep[hi] = true
			used += len(lines[hi]) + 1
			hi++
			grew = true
		} else {
			hi = len(lines)
		}
		if lo >= 0 && used+len(lines[lo])+1 <= budget {
			keep[lo] = true
			used += len(lines[lo]) + 1
			lo--
			grew = true
		} else {
			lo = -1
		}
		if !grew {
			break
		}
	}
	out := make([]int, 0, len(keep))
	for i := range keep {
		out = append(out, i)
	}
	sort.Ints(out)
	return out
}
