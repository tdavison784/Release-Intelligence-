package validate

import (
	"context"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// StaticReference checks the references the upgrade breaks (MISSION §A5
// "static dependency and reference checks"), in the environment renders
// with the relationship rules of the graph (WithRelations) and its namespace
// semantics, and along the item's impact paths:
//
//   - fail: a reference of (or to) an object the item's rendered changes
//     touch dangles in the TO render where it resolved in the FROM render,
//     or is new at TO and nothing provides its target (not rendered, not an
//     environment file, no declared producer, not provided by Kubernetes) —
//     named by referrer, field path and target kind/namespace/name; or an
//     edge on a path whose provider object the TO render no longer renders
//     while its consumer survives unchanged;
//   - pass: every edge on the path is established, no node is a placeholder,
//     the complete environment renders remove no provider on it, and no
//     reference of a node on it is newly dangling or undecidable — the
//     path's references resolve after the upgrade;
//   - not-run: no graph/traversal, a truncated traversal, no complete
//     environment render, an unresolved edge / placeholder node on the path,
//     or a reference whose target cannot be decided (matched by name only,
//     namespace unknown, optional, a controller only presumed to create it).
//
// A reference that dangled at FROM too is not the upgrade's doing (the
// environment provides it outside the snapshot, or it was already missing).
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
	pairs, complete := envPairs(in.Bundle)
	if !complete {
		out.notRun(cov, "reference:incomplete", property, "what the target no longer renders is unknown — "+incompleteDetail(pairs))
		return out.result()
	}
	rv, err := v.renderRefs(ctx, in, item, cov)
	if err != nil {
		return nil, err
	}
	if len(rv.failing) > 0 {
		v.renderFail(in, out, cov, property, pairs, rv)
		return out.result()
	}
	switch {
	case in.Graph == nil || in.Traversal == nil:
		out.notRun(cov, "reference:no-graph", property, "no relationship graph or traversal to check"+rv.note())
		return out.result()
	case in.Traversal.Truncated:
		out.notRun(cov, "reference:truncated", property, "the traversal was truncated ("+in.Traversal.TruncationReason+"): not every path is known"+rv.note())
		return out.result()
	case len(cov.paths) == 0:
		out.notRun(cov, "reference:no-paths", property, "the item has no impact paths to check"+rv.note())
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
		for _, nid := range p.Nodes {
			if b := rv.blocked[nodes[nid].Key]; b != "" && why == "" {
				why = b
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

// renderRefs is the render-level reference check of an item.
type renderRefs struct {
	ran     bool
	err     error
	failing refCheckList      // new or regressed dangles touching the item's rendered changes
	effects []domain.RenderedChange
	blocked map[string]string // object key → why paths through it decide nothing
	other   refCheckList      // pre-existing dangles and undecidable references (reported)
	checked int
}

func (r renderRefs) note() string {
	switch {
	case r.err != nil:
		return "; the render-level reference check could not run: " + r.err.Error()
	case !r.ran:
		return ""
	}
	s := fmt.Sprintf("; render-level check: %s of or to the item's objects compared at FROM and TO, none newly dangling", plural(r.checked, "reference", "references"))
	if len(r.other) > 0 {
		s += "; references that decide nothing:\n" + r.other.lines()
	}
	return s
}

// renderRefs compares the references of the TO render with the FROM render
// for the objects of the item's rendered changes (failures attributable to
// the item) and the objects on its impact paths (what blocks a pass).
func (v *staticReference) renderRefs(ctx context.Context, in investigate.ValidationInput, item domain.ChangeItem, cov coverage) (renderRefs, error) {
	rv := renderRefs{blocked: map[string]string{}}
	from, to, err := v.cfg.sides(ctx, in.Bundle)
	if err != nil {
		if ctx.Err() != nil {
			return rv, ctx.Err()
		}
		rv.err = err
		return rv, nil
	}
	rv.ran = true
	rv.effects = itemEffects(in, item)
	var mine []string
	for _, c := range rv.effects {
		mine = append(mine, c.Object)
	}
	onPaths := pathObjects(in, cov)
	attributable := touching(mine)
	for _, c := range compareRefs(from, to, touching(append(append([]string{}, mine...), onPaths...))) {
		rv.checked++
		switch {
		case c.failing() && attributable(c.ref):
			rv.failing = append(rv.failing, c)
		case c.failing():
			rv.blocked[c.ref.Referrer] = "a reference on the path dangles at TO, caused by no rendered change of this item: " + c.line()
		case c.state == refUnknown:
			rv.blocked[c.ref.Referrer] = "a reference on the path cannot be decided: " + c.line()
			rv.other = append(rv.other, c)
		case c.state == refPreexisting:
			rv.other = append(rv.other, c)
		}
	}
	return rv, nil
}

func (v *staticReference) renderFail(in investigate.ValidationInput, out *output, cov coverage, property string, pairs []domain.ManifestPair, rv renderRefs) {
	objs := rv.failing.objects()
	var rcs []string
	var ev []domain.EvidenceID
	for _, c := range rv.effects {
		hit := false
		for _, r := range rv.failing {
			hit = hit || r.ref.Referrer == c.Object || r.ref.TargetMatches(c.Object)
		}
		if hit {
			rcs = append(rcs, c.ID)
			for _, id := range c.Evidence {
				if out.known(id) {
					ev = append(ev, id)
				}
			}
		}
	}
	var inputs []string
	for _, p := range pairs {
		inputs = append(inputs, p.From, p.To, p.Diff)
		if a, ok := in.Bundle.Manifest.Lookup(p.To); ok {
			ev = append(ev, out.cite(artifactEvidence(domain.EvidenceRenderedDiff, a, "target "+p.Target,
				fmt.Sprintf("TO render of %s: %s", p.Target, plural(len(rv.failing), "dangling reference", "dangling references")), envRender("", "", ""))))
		}
	}
	nodes := nodesByKey(in.Graph)
	var subjects []string
	for _, o := range objs {
		subjects = append(subjects, nodes[o]...)
	}
	out.add(draft{rule: "reference:dangling-at-to", outcome: domain.ValidationFail, property: property,
		subjects: subjects, paths: pathsThrough(in, cov, subjects), rcs: rcs, inputs: inputs, evidence: ev,
		detail: "references the upgrade leaves dangling in the TO render:\n" + rv.failing.lines()})
}

// itemEffects are the environment rendered changes that carry the item's
// effect: the requested ones, else those correlated by a strong rule or the
// one the item is anchored on (as in RenderDiff).
func itemEffects(in investigate.ValidationInput, item domain.ChangeItem) []domain.RenderedChange {
	requested := map[string]bool{}
	for _, id := range in.RenderedChanges {
		requested[id] = true
	}
	var out []domain.RenderedChange
	for _, p := range in.Bundle.Renders {
		for _, c := range p.Changes {
			switch {
			case len(requested) > 0:
				if requested[c.ID] {
					out = append(out, c)
				}
			case c.ID == item.RenderedChange && item.RenderedChange != "":
				out = append(out, c)
			case contains(c.ChangeItems, item.ID) && strongCorrelation(c.Correlation) && !descriptionOnly(c):
				out = append(out, c)
			}
		}
	}
	return out
}

// pathObjects are the keys of the nodes on the covered paths and subjects.
func pathObjects(in investigate.ValidationInput, cov coverage) []string {
	if in.Graph == nil {
		return nil
	}
	keys := map[string]string{}
	for _, n := range in.Graph.Nodes {
		keys[n.ID] = n.Key
	}
	var out []string
	for _, s := range cov.subjects {
		out = append(out, keys[s])
	}
	if in.Traversal != nil {
		want := map[string]bool{}
		for _, p := range cov.paths {
			want[p] = true
		}
		for _, p := range in.Traversal.Paths {
			if want[p.ID] {
				for _, n := range p.Nodes {
					out = append(out, keys[n])
				}
			}
		}
	}
	return uniq(out)
}

// pathsThrough are the covered paths that contain one of the nodes.
func pathsThrough(in investigate.ValidationInput, cov coverage, nodes []string) []string {
	if in.Traversal == nil {
		return nil
	}
	set := map[string]bool{}
	for _, n := range nodes {
		set[n] = true
	}
	want := map[string]bool{}
	for _, p := range cov.paths {
		want[p] = true
	}
	var out []string
	for _, p := range in.Traversal.Paths {
		if !want[p.ID] {
			continue
		}
		for _, n := range p.Nodes {
			if set[n] {
				out = append(out, p.ID)
				break
			}
		}
	}
	return uniq(out)
}
