package classify

import (
	"context"
	"sort"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
)

// The real traverser (lane relations) feeding the classifier: an inferred
// edge anywhere below an item's root keeps it out of R2 even when every path
// the validators looked at passes; once the edge is established the same
// validation coverage clears it.
func TestRealTraverserFeedsThePolicy(t *testing.T) {
	for _, st := range []domain.EdgeStatus{domain.EdgeInferred, domain.EdgeEstablished} {
		t.Run(string(st), func(t *testing.T) {
			f := newFx(t)
			item := f.item()
			rcID := f.pair("argocd:apps/gizmo.yaml", rc("field-changed", controllerObj, "spec.template.spec.containers[0].args", item))[0]
			b := f.bundle()

			ctrl := domain.GraphNode{ID: domain.NodeID(domain.NodeWorkload, controllerObj), Kind: domain.NodeWorkload, Key: controllerObj}
			cm := domain.GraphNode{ID: domain.NodeID(domain.NodeConfigObject, "v1/ConfigMap/app/settings"), Kind: domain.NodeConfigObject, Key: "v1/ConfigMap/app/settings"}
			e := domain.GraphEdge{ID: domain.EdgeID(domain.EdgeConfigures, ctrl.ID, cm.ID, ""), Kind: domain.EdgeConfigures, From: ctrl.ID, To: cm.ID, Status: st, Rule: "test.rule@v1"}
			if st == domain.EdgeEstablished {
				e.Evidence = []domain.EvidenceID{f.up.ID}
			} else {
				e.Reason = "naming convention only"
			}
			g := &domain.RelationshipGraph{Bundle: b.ID, Builder: "test.graph@v1", Nodes: []domain.GraphNode{ctrl, cm}, Edges: []domain.GraphEdge{e}}
			sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
			g.ID = g.DerivedID()
			roots := []domain.ExposureRoot{{ChangeItem: item, Node: ctrl.ID, Rule: "exposure:rendered-change", RenderedChange: rcID}}
			tr, err := relations.NewTraverser().Traverse(context.Background(), g, roots, domain.DefaultTraversalBound)
			if err != nil {
				t.Fatal(err)
			}
			var ps []string
			for _, p := range tr.PathsOf(item) {
				ps = append(ps, p.ID)
			}
			pass := f.vrec(item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(rcID), paths(ps...))
			cls, _ := run(t, f.in(tr, nil, pass))
			fd := finding(t, cls, item)
			if st == domain.EdgeEstablished {
				wantClass(t, fd, domain.FindingNotAffected, RuleValidatedClear)
			} else {
				wantClass(t, fd, domain.FindingPotentiallyAffected, RulePlausibleImpact)
				if !hasGap(fd, domain.GapRelationship, e.ID) {
					t.Fatalf("gaps %+v", fd.Gaps)
				}
			}
		})
	}
}
