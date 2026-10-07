package reviewui

import (
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// The panel maps the review context's render evidence: object label,
// change class, path, before/after, template, renderer provenance.
func TestDeltaFromContext(t *testing.T) {
	ev := domain.NewEvidence(domain.EvidenceRenderedDiff, "render@v1", "https://charts.example/demo-1.1.0.tgz",
		"demo/templates/rbac.yaml · rbac.authorization.k8s.io/v1/ClusterRole//demo-controller rules",
		"1.0.0 → 1.1.0: rbac-permission-removed ClusterRole/demo-controller rules demo.example.org/widgets:update", "sha256:out", time.Unix(0, 0))
	ev.Render = &domain.RenderProvenance{Scope: domain.RenderRelease, Tool: "helm", ToolVersion: "v3.16.1", ChartDigest: "sha256:to",
		FromArtifact: &domain.RenderArtifact{Name: "demo", Version: "1.0.0", Digest: "sha256:from"},
		ToArtifact:   &domain.RenderArtifact{Name: "demo", Version: "1.1.0", Digest: "sha256:to"},
		Object:       "rbac.authorization.k8s.io/v1/ClusterRole//demo-controller", Path: "rules", Change: "rbac-permission-removed",
		Before: `"demo.example.org/widgets:update"`}
	d := deltaFromContext(&knowledge.RenderEvidence{Validation: "val-1", Relation: domain.RenderConfirmed, Explanation: "source 1×, target absent", Evidence: []domain.Evidence{ev}})
	if d == nil || d.Relation != RelationConfirmed || d.Renderer != "helm" || d.FromChart != "sha256:from" || d.ToChart != "sha256:to" || len(d.Deltas) != 1 {
		t.Fatalf("delta: %+v", d)
	}
	c := d.Deltas[0]
	if c.Object != "rbac.authorization.k8s.io/v1 ClusterRole/demo-controller" || c.ChangeClass != "rbac-permission-removed" ||
		c.Source != `"demo.example.org/widgets:update"` || c.Target != "" || c.Template != "demo/templates/rbac.yaml" {
		t.Errorf("change: %+v", c)
	}
	if deltaFromContext(nil) != nil {
		t.Error("no render evidence must hide the panel")
	}
}
