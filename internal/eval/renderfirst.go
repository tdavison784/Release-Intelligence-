package eval

// Render-first measurement (PO-7a, docs/RENDER-FIRST.md): which
// applicability links rendering could decide at all, and which ones a
// render-backed finding actually decided. Reported, never gated; it reads
// the links and the item's semantic labels only to bucket the outcome — it
// never feeds the pipeline.

import (
	"fmt"
	"io"
	"sort"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// RenderabilityUnlabelled buckets links whose item carries no semantic label.
const RenderabilityUnlabelled = "unlabelled"

// LinkRenderability classifies an expected item by the R12 renderability of
// its labelled subjects (family × change type, domain.RenderabilityOf): the
// most renderable subject wins (an item bundling a removed RBAC permission
// and a behaviour change is render-decidable in part, and rendering can hit
// it through that part). Items without labels are "unlabelled".
func LinkRenderability(e *Expected) string {
	if e == nil || len(e.Semantics) == 0 {
		return RenderabilityUnlabelled
	}
	best := domain.RenderNotVerifiable
	for _, s := range e.Semantics {
		if s.Subject == nil || s.Change == nil {
			continue
		}
		switch domain.RenderabilityOf(s.Subject.Family, s.Change.Type) {
		case domain.RenderVerifiable:
			best = domain.RenderVerifiable
		case domain.RenderPartiallyVerifiable:
			if best != domain.RenderVerifiable {
				best = domain.RenderPartiallyVerifiable
			}
		}
	}
	return string(best)
}

// RenderBacked reports whether a finding was decided by a customer render:
// a rendered-change match (an affected finding's environment chain) or a
// render-dimension check (a not-affected or unknown finding's record).
func RenderBacked(f domain.ImpactFinding) bool {
	for _, m := range f.Matches {
		if m.Kind == domain.MatchRenderedChange {
			return true
		}
	}
	for _, c := range f.Checks {
		if c.Dimension == domain.DimensionRender && c.Render != nil && c.Render.Outcome != domain.RenderUnavailable {
			return true
		}
	}
	return false
}

// RenderFirstBucket counts the links of one renderability class.
type RenderFirstBucket struct {
	Renderability string `json:"renderability"`
	// Affected links (relevance != not-affected): hit at all, and hit by a
	// render-backed affected finding.
	Affected         int `json:"affected"`
	AffectedHit      int `json:"affectedHit"`
	AffectedByRender int `json:"affectedHitByRender"`
	// Not-affected links: left clean, and cleared by a render-backed
	// not-affected finding on the item's change (with no affected finding).
	NotAffected         int `json:"notAffected"`
	NotAffectedClean    int `json:"notAffectedClean"`
	NotAffectedByRender int `json:"notAffectedClearedByRender"`
	// Violations: not-affected links a render-backed affected finding hit
	// (render made a wrong exposure call).
	RenderViolations int `json:"renderViolations"`
}

// Links is all decided links of the bucket.
func (b RenderFirstBucket) Links() int { return b.Affected + b.NotAffected }

// Correct is affected hits plus clean not-affected links.
func (b RenderFirstBucket) Correct() int { return b.AffectedHit + b.NotAffectedClean }

// ByRender is the links a render-backed finding decided correctly.
func (b RenderFirstBucket) ByRender() int { return b.AffectedByRender + b.NotAffectedByRender }

// RenderFirstStats is the render-first panel.
type RenderFirstStats struct {
	Buckets []RenderFirstBucket `json:"buckets"`
	Total   RenderFirstBucket   `json:"total"`
}

// RenderFirst aggregates the per-link render audits of a run.
func RenderFirst(results []EntryResult) RenderFirstStats {
	by := map[string]*RenderFirstBucket{}
	add := func(b *RenderFirstBucket, a EnvImpactAudit) {
		if a.Relevance == RelevanceNotAffected {
			b.NotAffected++
			if !a.Hit {
				b.NotAffectedClean++
				if a.RenderCleared {
					b.NotAffectedByRender++
				}
			} else if a.RenderDecided {
				b.RenderViolations++
			}
			return
		}
		b.Affected++
		if a.Hit {
			b.AffectedHit++
			if a.RenderDecided {
				b.AffectedByRender++
			}
		}
	}
	var st RenderFirstStats
	st.Total.Renderability = "all"
	for _, r := range results {
		for _, a := range r.EnvImpact {
			k := a.Renderability
			if k == "" {
				k = RenderabilityUnlabelled
			}
			if by[k] == nil {
				by[k] = &RenderFirstBucket{Renderability: k}
			}
			add(by[k], a)
			add(&st.Total, a)
		}
	}
	order := map[string]int{string(domain.RenderVerifiable): 0, string(domain.RenderPartiallyVerifiable): 1, string(domain.RenderNotVerifiable): 2, RenderabilityUnlabelled: 3}
	for _, b := range by {
		st.Buckets = append(st.Buckets, *b)
	}
	sort.Slice(st.Buckets, func(i, j int) bool { return order[st.Buckets[i].Renderability] < order[st.Buckets[j].Renderability] })
	return st
}

// WriteRenderFirst prints the panel.
func WriteRenderFirst(w io.Writer, title string, st RenderFirstStats) {
	fmt.Fprintf(w, "\nRender-first (%s): links by R12 renderability of the item's labelled subject\n", title)
	fmt.Fprintf(w, "  %-28s %6s %10s %16s %10s %18s %10s %16s\n", "renderability", "links", "affected", "hit (by render)", "not-aff.", "clean (by render)", "accuracy", "render-alone")
	row := func(b RenderFirstBucket) {
		acc, alone := 0.0, 0.0
		if n := b.Links(); n > 0 {
			acc = float64(b.Correct()) / float64(n)
			alone = float64(b.ByRender()) / float64(n)
		}
		viol := ""
		if b.RenderViolations > 0 {
			viol = fmt.Sprintf("  (%d not-affected link(s) wrongly hit by render)", b.RenderViolations)
		}
		fmt.Fprintf(w, "  %-28s %6d %10d %9d (%3d) %10d %11d (%3d) %10.3f %9.3f (%d)%s\n", b.Renderability, b.Links(), b.Affected, b.AffectedHit, b.AffectedByRender,
			b.NotAffected, b.NotAffectedClean, b.NotAffectedByRender, acc, alone, b.ByRender(), viol)
	}
	for _, b := range st.Buckets {
		row(b)
	}
	row(st.Total)
}

// auditRender fills a link audit's render fields from the report: its
// renderability bucket, whether a render-backed affected finding hits it,
// and whether a render-backed not-affected finding clears it.
func auditRender(a *EnvImpactAudit, c *Case, report *domain.ImpactReport, itemOf func(changeID string) string) {
	a.Renderability = LinkRenderability(expectedByID(c, a.ExpectedID))
	for _, f := range report.Findings {
		if f.ChangeID == "" || itemOf(f.ChangeID) != a.ExpectedID || !RenderBacked(f) {
			continue
		}
		switch {
		case f.Classification.Affected():
			a.RenderDecided = true
		case f.Classification == domain.ImpactNotAffected:
			a.RenderCleared = true
		}
	}
}
