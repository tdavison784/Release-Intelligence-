package render

import (
	"bytes"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"gopkg.in/yaml.v3"
)

// RenderedCRDs is one render's worth of CustomResourceDefinition documents
// for env.Inputs.RenderedCRDs (PO-7a addendum item 6): what the FROM render
// of the customer's install — helm template with their values, honouring
// their CRD gates exactly as set, or kustomize build of their overlay —
// says they install.
type RenderedCRDs struct {
	// Label names the render in evidence URIs and warnings.
	Label string
	// Tool is the renderer ("helm" | "kustomize").
	Tool string
	// ChartDigest and ValuesDigest carry the render's artifact/values
	// provenance ("" when the renderer states none).
	ChartDigest  string
	ValuesDigest string
	// Docs is the CRD documents as a YAML stream.
	Docs []byte
}

// Empty reports whether the source carries no documents.
func (r RenderedCRDs) Empty() bool { return len(bytes.TrimSpace(r.Docs)) == 0 }

// RenderedCRDsOf extracts the CustomResourceDefinition documents from the
// FROM renders of the environment pairs (never the chart-default pair: the
// customer's gates decide whether their install renders CRDs at all). A CRD
// name rendered by several pairs is kept once, from the first pair that
// rendered it; a render without CRD documents yields nothing — a gated-off or
// separately installed CRD path says nothing (absence is not knowledge).
func RenderedCRDsOf(pairs []*Pair) []RenderedCRDs {
	var out []RenderedCRDs
	seen := map[string]bool{}
	for _, p := range pairs {
		if p == nil || p.Scope != domain.RenderEnvironment || !p.FromResult.OK() {
			continue
		}
		var docs []byte
		for _, o := range p.FromResult.Objects {
			if o.ID.Kind != "CustomResourceDefinition" || seen[o.ID.Name] {
				continue
			}
			b, err := yaml.Marshal(o.Body)
			if err != nil {
				continue // a CRD body that cannot re-serialize is skipped, never guessed
			}
			seen[o.ID.Name] = true
			if len(docs) > 0 {
				docs = append(docs, []byte("---\n")...)
			}
			docs = append(docs, b...)
		}
		if len(docs) == 0 {
			continue
		}
		out = append(out, RenderedCRDs{
			Label:        renderLabel(p),
			Tool:         string(p.FromResult.Provenance.Tool),
			ChartDigest:  p.FromResult.Provenance.ArtifactDigest,
			ValuesDigest: p.FromResult.Provenance.ValuesDigest,
			Docs:         docs,
		})
	}
	return out
}

// renderLabel names a pair's FROM render: the chart and version for helm,
// the kustomization directory for kustomize, else the target id.
func renderLabel(p *Pair) string {
	pr := p.FromResult.Provenance
	switch {
	case pr.Chart != "" && pr.ChartVersion != "":
		return "chart " + pr.Chart + "@" + pr.ChartVersion
	case pr.KustomizeDir != "":
		return "kustomize " + pr.KustomizeDir
	}
	return strings.TrimSpace(p.Target.ID)
}
