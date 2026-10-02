package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Evidence returns the rendered-diff evidence record (RENDER-MISSION R5) for
// one change of the pair: the template (Helm "# Source:") or overlay as the
// URI/locator, both endpoints with their digests, the values digest, the
// object and path. Release-scope records carry the chart-default values in
// the excerpt; environment-scope records carry paths only unless showValues
// (they are environment evidence and never enter knowledge/ or a prompt).
func (p *Pair) Evidence(c Change, showValues bool) domain.Evidence {
	from, to := p.FromResult.Provenance, p.ToResult.Provenance
	uri := to.ChartURI
	if uri == "" && to.KustomizeDir != "" {
		uri = to.KustomizeRoot + "/" + to.KustomizeDir
	}
	if uri == "" {
		uri = "render:" + p.Product + "@" + p.To
	}
	loc := c.Object.String()
	if c.Path != "" {
		loc += " " + c.Path
	}
	if c.Source != "" {
		loc = c.Source + " · " + loc
	}
	show := showValues || p.Scope == domain.RenderRelease
	excerpt := fmt.Sprintf("%s → %s: %s", p.From, p.To, c.Summary(show))
	ev := domain.NewEvidence(domain.EvidenceRenderedDiff, Producer, uri, loc, excerpt, to.OutputDigest, to.RenderedAt)
	rp := to.Domain()
	rp.FromArtifact = &domain.RenderArtifact{Name: from.Chart, Version: from.ChartVersion, Digest: from.ArtifactDigest, OutputDigest: from.OutputDigest}
	rp.ToArtifact = &domain.RenderArtifact{Name: to.Chart, Version: to.ChartVersion, Digest: to.ArtifactDigest, OutputDigest: to.OutputDigest}
	if to.Tool == ToolKustomize {
		rp.FromArtifact.Name, rp.ToArtifact.Name = from.KustomizeDir, to.KustomizeDir
		rp.FromArtifact.Version, rp.ToArtifact.Version = p.From, p.To
	}
	rp.Object, rp.Path, rp.Change = c.Object.String(), c.Path, string(c.Class)
	if show && !c.Sensitive {
		if c.Before != nil {
			rp.Before = *c.Before
		}
		if c.After != nil {
			rp.After = *c.After
		}
	}
	ev.Render = &rp
	// the id covers the render scope and digests, so an environment record
	// can never collide with a release record of the same excerpt
	ev.ID = domain.EvidenceID("ev-" + domain.ShortHash(string(ev.Kind), ev.URI, ev.Locator, ev.Excerpt, ev.ContentDigest,
		string(rp.Scope), rp.ValuesDigest, rp.ChartDigest, from.ArtifactDigest))
	return ev
}

// RenderedEvidence returns one evidence record per rendered change of a
// successful pair, in diff order.
func (p *Pair) RenderedEvidence(showValues bool) []domain.Evidence {
	if p == nil || p.Status != PairOK {
		return nil
	}
	out := make([]domain.Evidence, 0, len(p.Diff.Changes))
	for _, c := range p.Diff.Changes {
		out = append(out, p.Evidence(c, showValues))
	}
	return out
}

// FailureEvidence is an input record stating that a render failed — the
// evidence of an UNKNOWN / render-failed gap.
func (p *Pair) FailureEvidence() domain.Evidence {
	reason, detail := "", ""
	if p.Failure != nil {
		reason, detail = string(p.Failure.Reason), p.Failure.Detail
	}
	return domain.NewEvidence(domain.EvidenceInput, Producer, "render:"+p.Target.ID,
		fmt.Sprintf("%s %s→%s", p.Status, p.From, p.To), strings.TrimSpace(reason+": "+detail), "", time.Time{})
}
