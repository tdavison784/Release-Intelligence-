package reviewui

import (
	"context"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// RenderRelation is how rendering relates to a prose-derived proposal
// (RENDER-MISSION.md goal 6): absence from a render is not refutation.
type RenderRelation string

const (
	RelationConfirmed     RenderRelation = "confirmed-by-render"
	RelationNotVisible    RenderRelation = "not-visible-in-render"
	RelationContradicted  RenderRelation = "contradicted-by-render"
	RelationNotApplicable RenderRelation = "render-not-applicable"
)

// RenderedDelta is the render evidence shown on the "Rendered delta" panel
// (RENDER-MISSION.md goal 18). It comes from knowledge.ReviewContext.Render
// (the candidate's rendered-diff validations, deltaFromContext); a queue
// implementing RenderedDeltaSource (the demo fixtures) overrides it. The
// panel is hidden when there is neither.
type RenderedDelta struct {
	Relation RenderRelation
	Deltas   []RenderedChange
	// Provenance of the render(s) the delta comes from.
	Renderer    string // e.g. "helm"
	Version     string // renderer version
	FromChart   string // chart digest of the source release
	ToChart     string // chart digest of the target release
	Provenance  *domain.RenderProvenance
	Explanation string
}

// RenderedChange is one changed field of a rendered object.
type RenderedChange struct {
	Object      string // object identity: "rbac.authorization.k8s.io/v1 ClusterRole/cert-manager-controller-issuers"
	ChangeClass string // e.g. "rbac-permission-removed"
	Path        string
	Source      string // value in the source release render ("" = absent)
	Target      string // value in the target release render ("" = absent)
	Template    string // template file (Helm "# Source:")
}

// RenderedDeltaSource is optionally implemented by a knowledge.Queue that can
// attach render evidence to an item.
type RenderedDeltaSource interface {
	RenderedDelta(ctx context.Context, itemID string) (*RenderedDelta, error)
}

// RenderedDelta implements RenderedDeltaSource for the fixtures.
func (q *DemoQueue) RenderedDelta(_ context.Context, itemID string) (*RenderedDelta, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.renders[itemID], nil
}

// deltaFromContext maps the review context's render evidence
// (knowledge.ReviewContext.Render, filled by the queue from the candidate's
// rendered-diff validations) onto the panel. CONTRACT-CHANGE(render).
func deltaFromContext(re *knowledge.RenderEvidence) *RenderedDelta {
	if re == nil {
		return nil
	}
	d := &RenderedDelta{Relation: RenderRelation(re.Relation), Explanation: re.Explanation}
	var states []RenderedChange
	for _, e := range re.Evidence {
		r := e.Render
		if r == nil {
			continue
		}
		if d.Provenance == nil {
			rp := *r
			d.Provenance = &rp
			d.Renderer, d.Version = r.Tool, r.ToolVersion
			if r.FromArtifact != nil {
				d.FromChart = r.FromArtifact.Digest
			}
			if r.ToArtifact != nil {
				d.ToChart = r.ToArtifact.Digest
			}
		}
		template := ""
		if i := strings.Index(e.Locator, " · "); i >= 0 {
			template = e.Locator[:i]
		}
		ch := RenderedChange{Object: objectLabel(r.Object), ChangeClass: r.Change, Path: r.Path,
			Source: r.Before, Target: r.After, Template: template}
		if r.Change == "state" { // what one render shows (no diff record): its excerpt
			ch.Target = e.Excerpt
			states = append(states, ch)
			continue
		}
		d.Deltas = append(d.Deltas, ch)
	}
	if len(d.Deltas) == 0 {
		d.Deltas = states
	}
	return d
}

// objectLabel renders "<group>/<version>/<kind>/<namespace>/<name>" as
// "<apiVersion> <Kind>/<name>" (+ namespace).
func objectLabel(id string) string {
	p := strings.Split(id, "/")
	if len(p) != 5 {
		return id
	}
	av := p[1]
	if p[0] != "core" && p[0] != "" {
		av = p[0] + "/" + p[1]
	}
	s := av + " " + p[2] + "/" + p[4]
	if p[3] != "" {
		s += " (ns " + p[3] + ")"
	}
	return s
}
