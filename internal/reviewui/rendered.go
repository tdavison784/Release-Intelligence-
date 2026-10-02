package reviewui

import (
	"context"

	"github.com/tdavison784/release-intelligence/internal/domain"
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
// (RENDER-MISSION.md goal 18). CONTRACT-CHANGE(dashboard): until the render lane adds
// this to knowledge.ReviewContext, the UI reads it through the optional
// RenderedDeltaSource port; the panel is hidden when there is none. When the
// field lands, replace the port by the field (the view code only needs this struct).
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
