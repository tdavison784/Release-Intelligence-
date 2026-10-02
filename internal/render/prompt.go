package render

import (
	"context"
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// EdgeRendered is the release-level rendered view of an edge, for the
// semantic lane's prompts and the dashboard: the chart-default diff, its
// correlation with the edge's changes, and one release-scope rendered-diff
// evidence record per rendered change (aligned with Correlation.Changes).
// Release-scope records carry chart defaults only, so they may be cited by
// proposals and enter knowledge/; environment renders never reach here.
type EdgeRendered struct {
	Pair        *Pair
	Correlation Correlation
	Evidence    []domain.Evidence
}

// EdgeRenderedChanges renders the edge's From and To charts with chart
// defaults (through the release-pair source) and returns the rendered
// changes as evidence. A failed render is returned as an error naming the
// failure (the caller proceeds without render evidence; it is never "no
// change"). It refuses any pair that is not release scope.
func EdgeRenderedChanges(ctx context.Context, rp ReleasePairs, edge *domain.UpgradeEdge) (*EdgeRendered, error) {
	if edge == nil {
		return nil, fmt.Errorf("render: no edge")
	}
	from, to := versionOf(nil, edge.From), versionOf(nil, edge.To)
	p := rp.ReleasePair(ctx, string(edge.Product.ID), from, to)
	return releaseRendered(p, edge)
}

func releaseRendered(p *Pair, edge *domain.UpgradeEdge) (*EdgeRendered, error) {
	if p == nil {
		return nil, fmt.Errorf("render: no release-level render")
	}
	if p.Scope != domain.RenderRelease {
		return nil, fmt.Errorf("render: a %s render carries customer configuration and never enters a prompt or knowledge", p.Scope)
	}
	for _, r := range []*Result{p.FromResult, p.ToResult} {
		if r != nil && r.Provenance.Scope != domain.RenderRelease {
			return nil, fmt.Errorf("render: a %s render carries customer configuration and never enters a prompt or knowledge", r.Provenance.Scope)
		}
	}
	if p.Status != PairOK {
		reason := "unknown"
		if p.Failure != nil {
			reason = string(p.Failure.Reason) + ": " + p.Failure.Detail
		}
		return nil, fmt.Errorf("render: release-level render %s (%s)", p.Status, reason)
	}
	out := &EdgeRendered{Pair: p, Correlation: Correlate(p.Diff, edge)}
	for _, cc := range out.Correlation.Changes {
		out.Evidence = append(out.Evidence, p.Evidence(cc.Change, true))
	}
	return out, nil
}

// ForChanges returns the evidence of the rendered changes that the given edge
// changes restate (a candidate's member change ids) — the deterministic
// annotation a prompt may show next to the candidate's own evidence.
func (er *EdgeRendered) ForChanges(changeIDs []string) []domain.Evidence {
	if er == nil {
		return nil
	}
	want := map[string]bool{}
	for _, id := range changeIDs {
		want[id] = true
	}
	var out []domain.Evidence
	for i, cc := range er.Correlation.Changes {
		for _, l := range cc.Links {
			if l.ChangeID != "" && want[l.ChangeID] {
				out = append(out, er.Evidence[i])
				break
			}
		}
	}
	return out
}

// Undocumented returns the evidence of rendered changes no edge change
// restates (review items: an upstream effect the changelog does not mention).
func (er *EdgeRendered) Undocumented() []domain.Evidence {
	if er == nil {
		return nil
	}
	var out []domain.Evidence
	for i, cc := range er.Correlation.Changes {
		if len(cc.Links) == 0 {
			out = append(out, er.Evidence[i])
		}
	}
	return out
}
