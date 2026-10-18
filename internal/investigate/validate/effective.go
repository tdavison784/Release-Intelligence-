package validate

import (
	"context"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/redaction"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// EffectiveObject is one object of the effective object view the pin
// validator decides on (envView.objects): the to side of every complete
// render of the product, every version-independent overlay render (a
// kustomize base merged with its patches), and the environment files no
// render consumed — with withheld enum values read back.
type EffectiveObject struct {
	Object render.Object
	// From is the same object before the upgrade (renders of the product
	// only; nil without a counterpart). SameSides: a file or overlay states
	// the object identically before and after.
	From      *render.Object
	SameSides bool
	// Rendered: seen through a render (product or overlay); Side is "to",
	// "overlay" or "file", Target the render target.
	Rendered bool
	Side     string
	Target   string
	Artifact domain.ArtifactRef
	// View names where the object comes from, as validation details do.
	View string
	// Unrendered: why its effective state is unknown (its file was consumed
	// by a render whose objects are not available); the object decides
	// nothing.
	Unrendered string
	ReadBack   []redaction.Recovered
}

// Effective is the effective object view of a bundle, as the pin validator
// sees it. Schemas are the CustomResourceDefinitions it may use (upstream
// CRD artifacts, and those the environment files and renders declare);
// Complete reports at least one environment render pair, all complete; Gaps
// names inputs not collected completely.
type Effective struct {
	Objects  []EffectiveObject
	Schemas  []render.Object
	Complete bool
	Gaps     []string
}

// EffectiveView loads the effective object view of a bundle.
func EffectiveView(ctx context.Context, st investigate.ArtifactStore, b *domain.EvidenceBundle) (*Effective, error) {
	v, err := loadView(ctx, config{store: st}, b)
	if err != nil {
		return nil, err
	}
	out := &Effective{Complete: v.complete, Gaps: v.gaps}
	for _, e := range v.objects() {
		out.Objects = append(out.Objects, EffectiveObject{
			Object: e.obj, From: e.from, SameSides: e.sameSides, Rendered: e.rendered,
			Side: e.src.side, Target: e.src.target, Artifact: e.src.artifact, View: e.src.label(),
			Unrendered: e.unrendered, ReadBack: e.src.readBack[e.obj.ID.String()],
		})
	}
	for _, s := range v.schemas {
		out.Schemas = append(out.Schemas, s.obj)
	}
	return out, nil
}
