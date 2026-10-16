package validate

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// RenderDiff validates a change item against the environment-scope render
// pairs of the bundle (the customer's own configuration rendered at from and
// to; DESIGN.md §2.4, the centerpiece evidence):
//
//   - fail: the item is breaking (§1.3) and an environment rendered change
//     is correlated to it by a rule that ties the change to the item's
//     effect (subject, values-default, artifact, or the item is itself a
//     rendered change) on a path that is not description text — the item's
//     effect demonstrably reaches the customer's rendered deployment. Only
//     complete pairs decide.
//   - pass: every environment pair is complete and every rendered change
//     correlated to the item is CRD description text (shown harmless, cited
//     in RenderedChanges), or there is none — and the item manifests as
//     rendered-config. Render silence never clears a controller-behaviour or
//     unknown item (§2.2.1).
//   - not-run otherwise: no or incomplete render, a non-breaking item with
//     correlated changes (a render cannot say they are harmless), changes
//     correlated only by a text mention, or a behaviour item.
//
// A requested set of rendered changes restricts the check to them.
func RenderDiff(opts ...Option) investigate.Validator {
	return &renderDiff{base{name: NameRenderDiff, kind: domain.ValidationRenderDiff, cfg: newConfig(opts)}}
}

type renderDiff struct{ base }

// strongCorrelations tie a rendered change to the item's effect (the same
// set DESIGN.md §2.2.1 uses for manifest:rendered-correlated). Text mentions
// (mentions-*) say the change text talks about the object, not that the
// rendered change is the item's effect.
// release-anchor (renderdiff) ties an environment change to the release-scope
// rendered change an item is anchored on. Correlation may list several
// rules, comma-separated; one strong rule suffices.
var strongCorrelations = map[string]bool{"subject": true, "values-default": true, "artifact": true, "release-anchor": true}

func strongCorrelation(rules string) bool {
	for _, r := range strings.Split(rules, ",") {
		if strongCorrelations[strings.TrimSpace(r)] {
			return true
		}
	}
	return false
}

func (v *renderDiff) Validate(ctx context.Context, in investigate.ValidationInput) (*investigate.ValidationOutput, error) {
	item, err := prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	cov, err := coverageOf(in, item)
	if err != nil {
		return nil, err
	}
	out := newOutput(v.base, in)
	b := in.Bundle
	property := fmt.Sprintf("The item's effect does not reach the environment's rendered configuration (%s)", item.Title)
	pairs, complete := envPairs(b)

	type hit struct {
		pair domain.ManifestPair
		rc   domain.RenderedChange
	}
	requested := map[string]bool{}
	for _, id := range in.RenderedChanges {
		requested[id] = true
	}
	var correlated []hit
	found := map[string]bool{}
	for _, p := range pairs {
		for _, c := range p.Changes {
			if len(requested) > 0 {
				if requested[c.ID] {
					found[c.ID] = true
					correlated = append(correlated, hit{p, c})
				}
				continue
			}
			// an item anchored on this environment change (scope
			// rendered-unexplained) is that change, correlated or not
			if contains(c.ChangeItems, item.ID) || c.ID == item.RenderedChange {
				correlated = append(correlated, hit{p, c})
			}
		}
	}
	for id := range requested {
		if !found[id] {
			return nil, fmt.Errorf("validate: %w: %s is not an environment rendered change of the bundle", investigate.ErrRejected, id)
		}
	}

	var effects, harmless, weak []hit
	for _, h := range correlated {
		switch {
		case descriptionOnly(h.rc):
			harmless = append(harmless, h)
		case strongCorrelation(h.rc.Correlation) || (item.RenderedChange != "" && (contains(h.rc.ChangeItems, item.ID) || h.rc.ID == item.RenderedChange)):
			effects = append(effects, h)
		default:
			weak = append(weak, h)
		}
	}

	inputs := func(hs []hit) []string {
		var ids []string
		seen := map[string]bool{}
		add := func(p domain.ManifestPair) {
			if !seen[p.Target] {
				seen[p.Target] = true
				ids = append(ids, p.From, p.To, p.Diff)
			}
		}
		for _, h := range hs {
			add(h.pair)
		}
		if len(hs) == 0 {
			for _, p := range pairs {
				add(p)
			}
		}
		return ids
	}
	rcIDs := func(hs []hit) []string {
		var ids []string
		for _, h := range hs {
			ids = append(ids, h.rc.ID)
		}
		return ids
	}

	if len(effects) > 0 && breaking(item) {
		d := draft{rule: "render-diff:breaking-effect", outcome: domain.ValidationFail, property: property,
			rcs: rcIDs(effects), inputs: inputs(effects)}
		d.subjects, d.paths = exposedBy(in, effects2objects(effects, func(h hit) string { return h.rc.Object }))
		for _, h := range effects {
			d.evidence = append(d.evidence, v.changeEvidence(out, h.pair, h.rc)...)
		}
		d.detail = fmt.Sprintf("breaking item (%s) whose effect reaches the environment's rendered deployment:\n%s",
			item.ScopeRule, describeChanges(effects2rc(effects, func(h hit) domain.RenderedChange { return h.rc })))
		out.add(d)
		return out.result()
	}
	if !complete {
		out.notRun(cov, "render-diff:incomplete", property, incompleteDetail(pairs))
		return out.result()
	}
	if !item.Manifestation.RenderDecisive() {
		out.notRun(cov, "render-diff:behaviour", property, fmt.Sprintf(
			"the item manifests as %s (%s): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an observed run decides it",
			item.Manifestation, item.ManifestationRule))
		return out.result()
	}
	if len(effects)+len(weak) > 0 {
		var why string
		switch {
		case len(effects) > 0:
			why = fmt.Sprintf("the item is not breaking (%s), but its effect reaches the rendered deployment; a render cannot say whether that is harmless, nor name a concrete failure", item.ScopeRule)
		default:
			why = "rendered changes are correlated to the item only by a text mention, which does not show they are its effect (nor that they are harmless)"
		}
		hs := append(append([]hit{}, effects...), weak...)
		out.add(draft{rule: "render-diff:undecided", outcome: domain.ValidationNotRun, property: property,
			subjects: cov.subjects, paths: cov.paths, rcs: rcIDs(hs),
			detail: why + ":\n" + describeChanges(effects2rc(hs, func(h hit) domain.RenderedChange { return h.rc }))})
		return out.result()
	}
	// Complete render, rendered-config item, nothing attributable (or only
	// description text): render silence is decisive for this manifestation.
	d := draft{rule: "render-diff:nothing-attributable", outcome: domain.ValidationPass, property: property,
		rcs: rcIDs(harmless), inputs: inputs(nil)}
	if len(requested) == 0 {
		// item-wide: nothing attributable anywhere in the complete renders.
		d.subjects, d.paths = cov.subjects, cov.paths
	}
	for _, p := range pairs {
		if a, ok := b.Manifest.Lookup(p.Diff); ok {
			n := 0
			for _, c := range p.Changes {
				// an item anchored on this environment change (scope
			// rendered-unexplained) is that change, correlated or not
			if contains(c.ChangeItems, item.ID) || c.ID == item.RenderedChange {
					n++
				}
			}
			d.evidence = append(d.evidence, out.cite(artifactEvidence(domain.EvidenceRenderedDiff, a, "target "+p.Target,
				fmt.Sprintf("complete environment render %s: %s, %d correlated to %s, none an effect on rendered configuration",
					p.Target, plural(len(p.Changes), "rendered change", "rendered changes"), n, item.ID), envRender("", "", ""))))
		}
	}
	for _, h := range harmless {
		d.evidence = append(d.evidence, v.changeEvidence(out, h.pair, h.rc)...)
	}
	d.detail = fmt.Sprintf("every environment render is complete (%s) and nothing in it is attributable to the item", targets(pairs))
	if len(requested) > 0 {
		d.detail = fmt.Sprintf("every environment render is complete (%s); the requested rendered changes are checked, not the whole render", targets(pairs))
	}
	if len(harmless) > 0 {
		d.detail += "; the correlated changes are CRD description text only, which has no effect on rendered configuration:\n" +
			describeChanges(effects2rc(harmless, func(h hit) domain.RenderedChange { return h.rc }))
	}
	out.add(d)
	return out.result()
}

// changeEvidence returns the evidence of a rendered change: its own
// rendered-diff records, or a record citing the pair's diff artifact.
func (v *renderDiff) changeEvidence(out *output, p domain.ManifestPair, c domain.RenderedChange) []domain.EvidenceID {
	var ids []domain.EvidenceID
	for _, id := range c.Evidence {
		if out.known(id) {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		return ids
	}
	a, ok := out.in.Bundle.Manifest.Lookup(p.Diff)
	if !ok {
		return nil
	}
	excerpt := fmt.Sprintf("%s %s %s", c.Class, c.Object, c.Path)
	if !c.Sensitive && (c.Before != "" || c.After != "") {
		excerpt += fmt.Sprintf(": %s → %s", orNone(c.Before), orNone(c.After))
	}
	return []domain.EvidenceID{out.cite(artifactEvidence(domain.EvidenceRenderedDiff, a, c.Object+" "+c.Path, excerpt, envRender(c.Object, c.Path, c.After)))}
}

// descriptionOnly: a change of OpenAPI description text inside a CRD — it
// documents behaviour, it does not configure anything.
func descriptionOnly(c domain.RenderedChange) bool {
	parts := strings.Split(c.Object, "/")
	if len(parts) < 3 || parts[2] != "CustomResourceDefinition" {
		return false
	}
	last := c.Path
	if i := strings.LastIndexByte(last, '.'); i >= 0 {
		last = last[i+1:]
	}
	return last == "description"
}

// exposedBy returns the graph nodes of the given objects that the item
// reaches, and the item's paths rooted at them.
func exposedBy(in investigate.ValidationInput, objects []string) (subjects, paths []string) {
	byKey := nodesByKey(in.Graph)
	var nodes []string
	for _, o := range objects {
		nodes = append(nodes, byKey[o]...)
	}
	if in.Traversal == nil {
		return uniq(nodes), nil
	}
	keep := map[string]bool{}
	for _, n := range nodes {
		keep[n] = true
	}
	for _, p := range in.Traversal.PathsOf(in.ChangeItem) {
		if len(p.Nodes) > 0 && keep[p.Nodes[0]] {
			paths = append(paths, p.ID)
		}
	}
	return uniq(nodes), uniq(paths)
}

func effects2objects[T any](hs []T, f func(T) string) []string {
	var out []string
	for _, h := range hs {
		out = append(out, f(h))
	}
	return out
}

func effects2rc[T any](hs []T, f func(T) domain.RenderedChange) []domain.RenderedChange {
	var out []domain.RenderedChange
	for _, h := range hs {
		out = append(out, f(h))
	}
	return out
}

func describeChanges(cs []domain.RenderedChange) string {
	sort.Slice(cs, func(i, j int) bool { return cs[i].ID < cs[j].ID })
	var b strings.Builder
	for _, c := range cs {
		fmt.Fprintf(&b, "- %s %s %s %s", c.ID, c.Class, c.Object, c.Path)
		switch {
		case c.Sensitive:
			b.WriteString(" (sensitive; values withheld)")
		case c.Before != "" || c.After != "":
			fmt.Fprintf(&b, ": %s → %s", orNone(c.Before), orNone(c.After))
		}
		if c.Correlation != "" {
			fmt.Fprintf(&b, " [correlation %s]", c.Correlation)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func targets(pairs []domain.ManifestPair) string {
	var ts []string
	for _, p := range pairs {
		ts = append(ts, p.Target)
	}
	return strings.Join(ts, ", ")
}

func orNone(s string) string {
	if s == "" {
		return "(absent)"
	}
	return s
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
