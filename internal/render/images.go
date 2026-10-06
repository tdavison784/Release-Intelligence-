package render

// PO-7a (product-owner decision, briefs/renderfirst.md), image family: for an
// image repository the environment references, the customer's own From→To
// render with their configuration decides what a reference cannot — a pin
// keeps the chart's image change out of their rendered delta (no-effect),
// anything else lets it through (attributable). No counterfactual is needed:
// the pair delta IS the customer's upgrade.

import (
	"fmt"
	"strings"
	"sync"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/impact"
)

// RenderImages implements impact.ImageRenderEvaluator over the environment's
// render pairs (`ri impact --render`), Helm and Kustomize alike (images reach
// both).
type RenderImages struct {
	Product, From, To string
	// Pairs are the environment pairs (customer configuration); release-scope
	// pairs are ignored (they carry no customer configuration).
	Pairs []*Pair

	mu   sync.Mutex
	memo map[string]impact.ImageResult
}

var _ impact.ImageRenderEvaluator = (*RenderImages)(nil)

// EvaluateImage implements impact.ImageRenderEvaluator.
func (s *RenderImages) EvaluateImage(c domain.Change, repo string) impact.ImageResult {
	key := c.ID + "|" + repo
	s.mu.Lock()
	if r, ok := s.memo[key]; ok {
		s.mu.Unlock()
		return r
	}
	s.mu.Unlock()
	r := s.evaluate(repo)
	s.mu.Lock()
	if s.memo == nil {
		s.memo = map[string]impact.ImageResult{}
	}
	s.memo[key] = r
	s.mu.Unlock()
	return r
}

func imageUndecided(needed ...string) impact.ImageResult {
	return impact.ImageResult{Outcome: impact.ImageUndecided, Needed: needed}
}

func (s *RenderImages) evaluate(repo string) impact.ImageResult {
	var pairs []*Pair
	for _, p := range s.Pairs {
		if p.Scope != domain.RenderRelease {
			pairs = append(pairs, p)
		}
	}
	if len(pairs) == 0 {
		return imageUndecided("no deployment of " + s.Product + " was rendered with your configuration")
	}
	var (
		matches      []domain.ImpactMatch
		records      []domain.Evidence
		needed       []string
		cleared      int
		compared     int
		clearRecords []domain.Evidence
	)
	for _, p := range pairs {
		switch {
		case p.Status != PairOK || p.Diff == nil:
			reason := string(p.Status)
			if p.Failure != nil {
				reason = string(p.Failure.Reason) + ": " + p.Failure.Detail
			}
			needed = append(needed, p.Target.ID+": render failed ("+reason+")")
		default:
			var imageChanges []Change
			for _, ch := range p.Diff.Changes {
				if imageChangeTouches(ch, repo) {
					imageChanges = append(imageChanges, ch)
				}
			}
			if len(imageChanges) > 0 {
				for _, ch := range imageChanges {
					ev := p.Evidence(ch, false)
					records = append(records, ev)
					matches = append(matches, domain.ImpactMatch{Kind: domain.MatchRenderedChange,
						Subject: repo + " → " + ch.Summary(false) + " (" + p.Target.ID + ")", Evidence: []domain.EvidenceID{ev.ID}})
				}
			} else if !p.Target.ValuesComplete {
				needed = append(needed, p.Target.ID+": values incomplete ("+p.Target.IncompleteReason+"), so the absence of an image change decides nothing")
			} else {
				cleared++
				compared += len(p.ToResult.Objects)
				subject := "the image " + repo
				clearRecords = append(clearRecords,
					p.stateEvidence(p.FromResult, p.From, subject, objOcc(p.FromResult.Objects)),
					p.stateEvidence(p.ToResult, p.To, subject, objOcc(p.ToResult.Objects)))
			}
		}
	}
	if len(matches) > 0 {
		return impact.ImageResult{Outcome: impact.ImageAttributable, Matches: matches, Records: records}
	}
	if len(needed) > 0 || cleared == 0 {
		return imageUndecided(needed...)
	}
	ids := make([]domain.EvidenceID, 0, len(clearRecords))
	for _, e := range clearRecords {
		ids = append(ids, e.ID)
	}
	return impact.ImageResult{Outcome: impact.ImageNoEffect, Records: clearRecords, Checks: []domain.ImpactCheck{{
		Dimension: domain.DimensionRender, Facts: compared, Evidence: ids,
		Subjects: []string{fmt.Sprintf("rendered %s → %s with your configuration: no change to %s in %d deployment(s)", s.From, s.To, repo, cleared)},
		Render:   &domain.RenderCheck{Outcome: domain.RenderNoAttributableChange, Key: repo},
	}}}
}

// imageChangeTouches reports whether a rendered change is an image change of
// the repository (the change names the repository, or its before/after
// references resolve to it).
func imageChangeTouches(c Change, repo string) bool {
	if c.Class != ImageChanged {
		return false
	}
	if c.Name == repo {
		return true
	}
	for _, v := range []*string{c.Before, c.After} {
		if v != nil && imageRepository(strings.Trim(*v, `"`)) == repo {
			return true
		}
	}
	return false
}
