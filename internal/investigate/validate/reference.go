package validate

import (
	"context"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// StaticReference checks the item's impact paths for references the upgrade
// breaks (MISSION §A5 "static dependency and reference checks"), on the
// relationship graph plus the environment renders:
//
//   - fail: an edge on a path whose provider object the target render no
//     longer renders (rendered change resource-removed) while its consumer
//     survives unchanged — the consumer's reference, selector or API use
//     dangles after the upgrade;
//   - pass: every edge on the path is established, no node is a placeholder,
//     the complete environment renders remove no provider on it — the
//     path's references resolve after the upgrade;
//   - not-run: no graph/traversal, a truncated traversal, no complete
//     environment render, or an unresolved edge / placeholder node on the
//     path (a relationship that is not established decides nothing).
//
// The property is about references only; it never clears a
// controller-behaviour item (DESIGN.md R2.4 counts only pinned settings and
// observed runs for those).
func StaticReference(opts ...Option) investigate.Validator {
	return &staticReference{base{name: NameStaticReference, kind: domain.ValidationStaticReference, cfg: newConfig(opts)}}
}

type staticReference struct{ base }

func (v *staticReference) Validate(ctx context.Context, in investigate.ValidationInput) (*investigate.ValidationOutput, error) {
	item, err := prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	cov, err := coverageOf(in, item)
	if err != nil {
		return nil, err
	}
	out := newOutput(v.base, in)
	property := "References along the item's impact paths still resolve after the upgrade"
	switch {
	case in.Graph == nil || in.Traversal == nil:
		out.notRun(cov, "reference:no-graph", property, "no relationship graph or traversal to check")
		return out.result()
	case in.Traversal.Truncated:
		out.notRun(cov, "reference:truncated", property, "the traversal was truncated ("+in.Traversal.TruncationReason+"): not every path is known")
		return out.result()
	case len(cov.paths) == 0:
		out.notRun(cov, "reference:no-paths", property, "the item has no impact paths to check")
		return out.result()
	}
	pairs, complete := envPairs(in.Bundle)
	if !complete {
		out.notRun(cov, "reference:incomplete", property, "what the target no longer renders is unknown — "+incompleteDetail(pairs))
		return out.result()
	}
	nodes := map[string]domain.GraphNode{}
	for _, n := range in.Graph.Nodes {
		nodes[n.ID] = n
	}
	edges := map[string]domain.GraphEdge{}
	for _, e := range in.Graph.Edges {
		edges[e.ID] = e
	}
	paths := map[string]domain.ImpactPath{}
	for _, p := range in.Traversal.Paths {
		paths[p.ID] = p
	}
	removed := map[string]domain.RenderedChange{} // object → its resource-removed change
	changed := map[string]bool{}                  // objects with any rendered change
	for _, p := range pairs {
		for _, c := range p.Changes {
			changed[c.Object] = true
			if c.Class == "resource-removed" {
				removed[c.Object] = c
			}
		}
	}

	var danglingPaths, danglingRCs, intact, undecided []string
	var danglingLines, undecidedLines []string
	var evidence []domain.EvidenceID
	for _, pid := range cov.paths {
		p := paths[pid]
		why := ""
		for _, nid := range p.Nodes {
			if n := nodes[nid]; n.Placeholder {
				why = fmt.Sprintf("node %s (%s) is referenced but absent from the snapshot", nid, n.Key)
			}
		}
		var dangles []string
		for _, eid := range p.Edges {
			e, ok := edges[eid]
			switch {
			case !ok:
				why = "edge " + eid + " is not in the graph"
			case !e.Status.Resolved():
				why = fmt.Sprintf("edge %s (%s) is %s: %s", eid, e.Kind, e.Status, e.Reason)
			default:
				from, to := nodes[e.From], nodes[e.To]
				rc, gone := removed[from.Key]
				if gone && removed[to.Key].ID == "" && !changed[to.Key] {
					dangles = append(dangles, fmt.Sprintf("%s %s → %s: %s is no longer rendered, %s still depends on it unchanged", e.Kind, from.Key, to.Key, from.Key, to.Key))
					danglingRCs = append(danglingRCs, rc.ID)
					evidence = append(evidence, rc.Evidence...)
					evidence = append(evidence, e.Evidence...)
				}
			}
		}
		if why == "" && len(p.Edges) == 0 {
			why = "the path is its root alone: there is no reference on it to check"
		}
		switch {
		case len(dangles) > 0:
			danglingPaths = append(danglingPaths, pid)
			danglingLines = append(danglingLines, pid+": "+strings.Join(dangles, "; "))
		case why != "":
			undecided = append(undecided, pid)
			undecidedLines = append(undecidedLines, pid+": "+why)
		default:
			intact = append(intact, pid)
			for _, eid := range p.Edges {
				evidence = append(evidence, edges[eid].Evidence...)
			}
		}
	}
	var inputs []string
	for _, p := range pairs {
		inputs = append(inputs, p.From, p.To, p.Diff)
	}
	known := func(ids []domain.EvidenceID) []domain.EvidenceID {
		var keep []domain.EvidenceID
		for _, id := range ids {
			if out.known(id) {
				keep = append(keep, id)
			}
		}
		return keep
	}
	roots := func(ids []string) []string {
		var rs []string
		for _, id := range ids {
			if p := paths[id]; len(p.Nodes) > 0 {
				rs = append(rs, p.Nodes[0])
			}
		}
		return rs
	}
	summary := func() domain.EvidenceID {
		a, _ := in.Bundle.Manifest.Lookup(pairs[0].Diff)
		return out.cite(artifactEvidence(domain.EvidenceRenderedDiff, a, "target "+pairs[0].Target,
			fmt.Sprintf("complete environment render %s: %d objects no longer rendered", pairs[0].Target, len(removed)), envRender("", "", "")))
	}
	if len(danglingPaths) > 0 {
		ev := append(known(evidence), summary())
		out.add(draft{rule: "reference:dangling", outcome: domain.ValidationFail, property: property,
			subjects: roots(danglingPaths), paths: danglingPaths, rcs: danglingRCs, inputs: inputs, evidence: ev,
			detail: "references the upgrade leaves dangling:\n- " + strings.Join(danglingLines, "\n- ")})
		return out.result()
	}
	if len(intact) > 0 && item.ScopeRule == scopeRenderedUnexplained {
		// Intact references do not show that a rendered change nothing
		// upstream explains is harmless (DESIGN.md §1.3 amendment).
		undecided = append(undecided, intact...)
		for _, pid := range intact {
			undecidedLines = append(undecidedLines, pid+": references resolve, but that does not show the unexplained rendered change harmless")
		}
		intact = nil
	}
	if len(intact) > 0 {
		ev := append(known(evidence), summary())
		out.add(draft{rule: "reference:intact", outcome: domain.ValidationPass, property: property,
			subjects: roots(intact), paths: intact, inputs: inputs, evidence: ev,
			detail: fmt.Sprintf("%s: every edge established, no placeholder, the complete environment renders (%s) remove no provider on them",
				plural(len(intact), "path", "paths"), targets(pairs))})
	}
	if len(undecided) > 0 {
		out.add(draft{rule: "reference:unresolved", outcome: domain.ValidationNotRun, property: property,
			subjects: roots(undecided), paths: undecided,
			detail: "an unresolved relationship decides nothing:\n- " + strings.Join(undecidedLines, "\n- ")})
	}
	return out.result()
}
