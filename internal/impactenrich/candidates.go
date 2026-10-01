package impactenrich

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// CandidateType names which bounded question a candidate poses.
type CandidateType string

const (
	// CandApplicability: one unknown finding whose upstream change is
	// note-derived and not routine — could it apply to THIS environment?
	CandApplicability CandidateType = "applicability"
	// CandCluster: findings whose changes look like the same logical change
	// seen from different sources; the model adjudicates the semantics.
	CandCluster CandidateType = "cluster"
	// CandMigration: one affected finding eligible for a migration-steps
	// synthesis.
	CandMigration CandidateType = "migration"
)

// Candidate is a deterministic candidate: a set of findings of one report
// that a bounded prompt is built around. A candidate is NOT a conclusion;
// without a model nothing is derived from it.
type Candidate struct {
	ID       string        `json:"id"`
	Type     CandidateType `json:"type"`
	Findings []string      `json:"findings"` // finding ids, in report order
	// Signals say why the findings were selected, e.g.
	// "unknown note-derived change (impact:not-joined): imp-1".
	Signals []string `json:"signals"`
	Score   float64  `json:"score"`
}

// CandidateOptions bound candidate generation.
type CandidateOptions struct {
	// MaxGroupSize caps the findings per cluster candidate (default 5).
	MaxGroupSize int
	// NearDuplicate is the title-token Jaccard overlap two findings with the
	// same subject key need to be linked as near duplicates (default 0.75,
	// the threshold of the deterministic duplicate audit in internal/eval).
	NearDuplicate float64
}

func (o CandidateOptions) withDefaults() CandidateOptions {
	if o.MaxGroupSize <= 1 {
		o.MaxGroupSize = 5
	}
	if o.NearDuplicate <= 0 || o.NearDuplicate >= 1 {
		o.NearDuplicate = 0.75
	}
	return o
}

// Candidates selects what the enrichment step may ask about, purely
// deterministically:
//
//   - applicability: every UNKNOWN finding whose upstream change is
//     note-derived (declared, not computed) and not routine maintenance —
//     routine items are already handled deterministically, and computed diffs
//     have machine-comparable subjects the join did evaluate or will. The
//     change must carry evidence to ground the prompt.
//   - cluster: groups of ≥ 2 findings whose upstream changes look like the
//     same logical change (identical normalised title; identical
//     category + subject set; or a near-duplicate title with the same
//     non-empty subject set — the deterministic duplicate detector's shape).
//   - migration: affected findings (action-required / review-required) whose
//     change is a migration or breaking change and carries evidence.
//
// Applicability candidates come first (they are the block the step exists
// for), then clusters, then migrations; each group in report order. The
// caller caps how many are actually asked (Options.Max).
func Candidates(rep *domain.ImpactReport, edge *domain.UpgradeEdge, opts CandidateOptions) []Candidate {
	opts = opts.withDefaults()
	if rep == nil || edge == nil {
		return nil
	}
	changes := map[string]domain.Change{}
	for _, c := range edge.Changes {
		changes[c.ID] = c
	}
	findingOf := map[string]domain.ImpactFinding{}
	for _, f := range rep.Findings {
		findingOf[f.ID] = f
	}

	var out []Candidate
	add := func(typ CandidateType, fs []domain.ImpactFinding, signal func(f domain.ImpactFinding) string) {
		if len(fs) == 0 {
			return
		}
		cand := Candidate{Type: typ, Score: 1}
		for _, f := range fs {
			cand.Findings = append(cand.Findings, f.ID)
			if s := signal(f); s != "" {
				cand.Signals = append(cand.Signals, s)
			}
		}
		cand.ID = "cand-" + domain.ShortHash(string(typ), strings.Join(cand.Findings, "\x00"))
		out = append(out, cand)
	}

	// (a) applicability: unknown × note-derived × not-routine × evidenced.
	for _, f := range rep.Findings {
		if f.Classification != domain.ImpactUnknown || f.ChangeID == "" {
			continue
		}
		c, ok := changes[f.ChangeID]
		if !ok || len(c.Evidence) == 0 || c.Routine {
			continue
		}
		if c.Provenance.Method == domain.MethodComputed {
			continue // a computed diff has machine-comparable subjects
		}
		cf := f
		add(CandApplicability, []domain.ImpactFinding{cf}, func(f domain.ImpactFinding) string {
			return fmt.Sprintf("unknown note-derived change (%s): %s", f.Rule, f.ID)
		})
	}

	// (b) clusters: known duplicate groups of changes, mapped to the findings
	// that carry them (any class: duplicates occur among affected findings
	// just as among unknowns).
	for _, grp := range duplicateGroups(edge, changes, opts) {
		var fs []domain.ImpactFinding
		seen := map[string]bool{}
		for _, chID := range grp {
			for _, f := range rep.Findings {
				if f.ChangeID != chID || seen[f.ID] {
					continue
				}
				seen[f.ID] = true
				fs = append(fs, f)
			}
		}
		if len(fs) < 2 {
			continue
		}
		sort.Slice(fs, func(i, j int) bool { return fs[i].ID < fs[j].ID })
		if len(fs) > opts.MaxGroupSize {
			fs = fs[:opts.MaxGroupSize]
		}
		add(CandCluster, fs, func(f domain.ImpactFinding) string {
			return fmt.Sprintf("possible duplicate of the same logical change (%s): %s", f.Classification, f.ID)
		})
	}

	// (c) migration synthesis for affected findings.
	for _, f := range rep.Findings {
		if f.ChangeID == "" {
			continue
		}
		switch f.Classification {
		case domain.ImpactActionRequired, domain.ImpactReviewRequired:
		default:
			continue
		}
		c, ok := changes[f.ChangeID]
		if !ok || len(c.Evidence) == 0 {
			continue
		}
		if c.Category != domain.CategoryMigration && !c.Breaking {
			continue
		}
		cf := f
		add(CandMigration, []domain.ImpactFinding{cf}, func(f domain.ImpactFinding) string {
			return fmt.Sprintf("affected %s change (%s): %s", f.Classification, f.Rule, f.ID)
		})
	}
	return out
}

// duplicateGroups groups the edge's changes that restate the same fact:
// identical normalised title, identical category+subject set, or ≥ 0.75
// title-token Jaccard overlap with identical non-empty subject sets (the
// deterministic duplicate audit of internal/eval, reused as a candidate
// signal only — the model adjudicates whether the members really are one
// logical change).
func duplicateGroups(edge *domain.UpgradeEdge, changes map[string]domain.Change, opts CandidateOptions) [][]string {
	byTitle := map[string][]string{}
	bySubject := map[string][]string{}
	for _, c := range edge.Changes {
		byTitle[normalizeTitle(c.Title)] = append(byTitle[normalizeTitle(c.Title)], c.ID)
		if k := subjectKey(c); k != "" {
			bySubject[k] = append(bySubject[k], c.ID)
		}
	}
	groups := map[string][]string{}
	for k, ids := range byTitle {
		if len(ids) > 1 {
			groups["title:"+k] = ids
		}
	}
	for k, ids := range bySubject {
		if len(ids) < 2 {
			continue
		}
		groups["subjects:"+k] = mergeIDs(groups["subjects:"+k], ids...)
		for a := 0; a < len(ids); a++ {
			for b := a + 1; b < len(ids); b++ {
				if jaccard(tokens(changes[ids[a]].Title), tokens(changes[ids[b]].Title)) >= opts.NearDuplicate {
					key := "near:" + k
					groups[key] = mergeIDs(groups[key], ids[a], ids[b])
				}
			}
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// the same member set can be found by two signals (identical title AND
	// identical subjects); one candidate per unique member set
	byMembers := map[string]bool{}
	var out [][]string
	for _, k := range keys {
		ids := groups[k]
		sort.Strings(ids)
		if len(ids) < 2 {
			continue
		}
		key := strings.Join(ids, "\x00")
		if byMembers[key] {
			continue
		}
		byMembers[key] = true
		out = append(out, ids)
	}
	return out
}

// subjectKey identifies a change by its category and its subject set.
func subjectKey(c domain.Change) string {
	if len(c.Subjects) == 0 {
		return ""
	}
	ss := append([]string(nil), c.Subjects...)
	sort.Strings(ss)
	return string(c.Category) + "|" + strings.Join(ss, "\x00")
}

func mergeIDs(in []string, ids ...string) []string {
	seen := map[string]bool{}
	for _, id := range in {
		seen[id] = true
	}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			in = append(in, id)
		}
	}
	return in
}
