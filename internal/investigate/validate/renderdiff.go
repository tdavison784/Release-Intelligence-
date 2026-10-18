package validate

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// RenderDiff validates a change item against the environment-scope render
// pairs of the bundle (the customer's own configuration rendered at from and
// to; DESIGN.md §2.4, the centerpiece evidence). Exposure is not failure: a
// rendered change that carries the item's effect into the customer's
// deployment is a plausible impact (R3), and only a concrete failure it
// causes is a demonstrated one (R1):
//
//   - fail: an environment rendered change correlated to the item by a rule
//     that ties it to the item's effect (subject, values-default, artifact,
//     release-anchor, or the item is itself the rendered change) causes a
//     concrete failure in the TO render that the FROM render did not have —
//     a reference of (or to) the changed object that dangles (the object the
//     change removes, a name the change introduces that nothing provides), a
//     custom resource invalid against its rendered CRD schema, or an
//     apiVersion the rendered CRD no longer serves. Each failure is named.
//   - error (inconclusive): such an effect reaches the rendered deployment
//     and no concrete failure is found (or the checks cannot run) — never a
//     pass, never a fail by itself.
//   - pass: every environment pair is complete and every rendered change
//     correlated to the item is CRD description text (shown harmless, cited
//     in RenderedChanges), or there is none — and the item manifests as
//     rendered-config. Render silence never clears a controller-behaviour or
//     unknown item (§2.2.1).
//   - pass, documentation-only (controller-behaviour / unknown items): every
//     environment pair is complete and every rendered change correlated to
//     the item is CRD description text in a CRD that changes nothing else.
//     The record (DocumentationOnly) accounts for those rendered changes and
//     names no subject or path: it is never a behaviour clearance.
//   - not-run otherwise: no or incomplete render, changes correlated only by
//     a text mention, or a behaviour item with nothing documentation-only to
//     account for.
//
// A requested set of rendered changes restricts the check to them. The
// reference and schema checks need the artifact store (WithStore) and use
// the relationship rules given by WithRelations.
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

	if len(effects) > 0 {
		rcOf := func(hs []hit) []domain.RenderedChange {
			return effects2rc(hs, func(h hit) domain.RenderedChange { return h.rc })
		}
		cf := v.concreteFailures(ctx, b, effects2rc(effects, func(h hit) domain.RenderedChange { return h.rc }))
		if cf.failed() {
			var involved []hit
			for _, h := range effects {
				if cf.involves(h.rc.Object) {
					involved = append(involved, h)
				}
			}
			d := draft{rule: "render-diff:concrete-failure", outcome: domain.ValidationFail, property: property,
				rcs: rcIDs(involved), inputs: inputs(involved)}
			d.subjects, d.paths = exposedBy(in, cf.objects())
			for _, h := range involved {
				d.evidence = append(d.evidence, v.changeEvidence(out, h.pair, h.rc)...)
			}
			d.detail = fmt.Sprintf("the item's effect causes a concrete failure in the TO render that the FROM render did not have:\n%s\ncaused by:\n%s",
				cf.failures(), describeChanges(rcOf(involved)))
			out.add(d)
			return out.result()
		}
		d := draft{rule: "render-diff:exposure", outcome: domain.ValidationError, property: property,
			rcs: rcIDs(effects), inputs: inputs(effects)}
		d.subjects, d.paths = exposedBy(in, effects2objects(effects, func(h hit) string { return h.rc.Object }))
		for _, h := range effects {
			d.evidence = append(d.evidence, v.changeEvidence(out, h.pair, h.rc)...)
		}
		var lines []string
		for _, c := range rcOf(effects) {
			lines = append(lines, fmt.Sprintf("- exposure: %s %s reaches %s (%s)", c.Class, orNone(c.Path), c.Object, c.ID))
		}
		sort.Strings(lines)
		d.detail = fmt.Sprintf("inconclusive — the item's effect (%s) reaches the environment's rendered deployment, which by itself is exposure, not a demonstrated failure:\n%s\n%s",
			item.ScopeRule, strings.Join(lines, "\n"), cf.summary())
		out.add(d)
		return out.result()
	}
	if !complete {
		out.notRun(cov, "render-diff:incomplete", property, incompleteDetail(pairs))
		return out.result()
	}
	if !item.Manifestation.RenderDecisive() {
		behaviour := fmt.Sprintf(
			"the item manifests as %s (%s): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an observed run decides it",
			item.Manifestation, item.ManifestationRule)
		if len(harmless) > 0 && len(weak) == 0 {
			v.documentationOnly(out, cov, effects2rc(harmless, func(h hit) domain.RenderedChange { return h.rc }), func(id string) []domain.EvidenceID {
				for _, h := range harmless {
					if h.rc.ID == id {
						return v.changeEvidence(out, h.pair, h.rc)
					}
				}
				return nil
			}, inputs(harmless), behaviour)
			return out.result()
		}
		out.notRun(cov, "render-diff:behaviour", property, behaviour)
		return out.result()
	}
	if len(weak) > 0 {
		why := "rendered changes are correlated to the item only by a text mention, which does not show they are its effect (nor that they are harmless)"
		out.add(draft{rule: "render-diff:undecided", outcome: domain.ValidationNotRun, property: property,
			subjects: cov.subjects, paths: cov.paths, rcs: rcIDs(weak),
			detail: why + ":\n" + describeChanges(effects2rc(weak, func(h hit) domain.RenderedChange { return h.rc }))})
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

// documentationOnly records, for a behaviour item, the rendered changes
// correlated to it when every one is a documentation-only CRD change
// (domain EvidenceBundle.DocumentationOnlyChange: description text, and no
// schema, default, enum, required, served or storage change in the same
// CRD): a pass marked DocumentationOnly that accounts for them (R2.2) and
// for nothing else — no subject, no path, never a behaviour clearance. A
// CRD that also changes beyond description text leaves them all
// unaccounted for (not-run, naming why).
func (v *renderDiff) documentationOnly(out *output, cov coverage, rcs []domain.RenderedChange, evidence func(string) []domain.EvidenceID, inputs []string, behaviour string) {
	property := "The rendered changes correlated to the item are documentation only: CRD description text that configures nothing in the environment"
	var ids, disq []string
	for _, c := range rcs {
		ids = append(ids, c.ID)
		if ok, why := out.in.Bundle.DocumentationOnlyChange(c.ID); !ok {
			disq = append(disq, "- "+why)
		}
	}
	if len(disq) > 0 {
		sort.Strings(disq)
		out.add(draft{rule: "render-diff:behaviour", outcome: domain.ValidationNotRun, property: property,
			subjects: cov.subjects, paths: cov.paths, rcs: ids,
			detail: behaviour + "; the correlated CRD description changes are not documentation-only, so they are not accounted for:\n" + strings.Join(disq, "\n")})
		return
	}
	d := draft{rule: "render-diff:documentation-only", outcome: domain.ValidationPass, property: property,
		rcs: ids, inputs: inputs, docOnly: true}
	for _, id := range ids {
		d.evidence = append(d.evidence, evidence(id)...)
	}
	d.detail = "every rendered change correlated to the item is CRD description text, in CRDs that change nothing else (no schema, default, enum, required, served or storage change): documentation, not configuration. " +
		"This accounts for these rendered changes only; it is not a behaviour clearance — " + behaviour + ":\n" + describeChanges(rcs)
	out.add(d)
}

// concrete is what the concrete-failure checks found for a set of rendered
// changes.
type concrete struct {
	refs     refCheckList      // failing references (new or regressed)
	other    refCheckList      // pre-existing and unknown references (reported, decide nothing)
	resolved int               // references that resolve at TO
	schema   []schemaViolation // new schema violations
	checked  int               // custom resources checked against a rendered CRD
	err      error             // the checks could not run
}

func (c concrete) failed() bool { return len(c.refs) > 0 || len(c.schema) > 0 }

// objects are the objects a failure involves.
func (c concrete) objects() []string {
	out := c.refs.objects()
	for _, v := range c.schema {
		out = append(out, v.object, v.crd)
	}
	return uniq(out)
}

func (c concrete) involves(object string) bool {
	for _, o := range c.objects() {
		if o == object {
			return true
		}
	}
	for _, r := range c.refs {
		if r.ref.TargetMatches(object) {
			return true
		}
	}
	return false
}

func (c concrete) failures() string {
	var ls []string
	if len(c.refs) > 0 {
		ls = append(ls, "dangling references:\n"+c.refs.lines())
	}
	if len(c.schema) > 0 {
		var vs []string
		for _, v := range c.schema {
			vs = append(vs, "- "+v.line())
		}
		ls = append(ls, "invalid against the rendered CRD (new at TO):\n"+strings.Join(vs, "\n"))
	}
	return strings.Join(ls, "\n")
}

func (c concrete) summary() string {
	if c.err != nil {
		return "the concrete-failure checks (dangling references, CRD schema) could not run: " + c.err.Error()
	}
	s := fmt.Sprintf("no concrete failure found: %s of or to the changed objects checked at FROM and TO, none newly dangling (%d resolve); %s checked against the rendered CRD schema, no new violation",
		plural(c.resolved+len(c.other), "reference", "references"), c.resolved, plural(c.checked, "custom resource", "custom resources"))
	if len(c.other) > 0 {
		s += "; references that decide nothing:\n" + c.other.lines()
	}
	return s
}

// concreteFailures runs the reference and schema checks on the objects of
// the given environment rendered changes.
func (v *renderDiff) concreteFailures(ctx context.Context, b *domain.EvidenceBundle, rcs []domain.RenderedChange) concrete {
	from, to, err := v.cfg.sides(ctx, b)
	if err != nil {
		return concrete{err: err}
	}
	var keys []string
	for _, c := range rcs {
		keys = append(keys, c.Object)
	}
	keys = uniq(keys)
	var c concrete
	for _, r := range compareRefs(from, to, touching(keys)) {
		switch {
		case r.failing():
			c.refs = append(c.refs, r)
		case r.state == refResolved:
			c.resolved++
		default:
			c.other = append(c.other, r)
		}
	}
	var objs []relations.Object
	inKeys := map[string]bool{}
	for _, k := range keys {
		inKeys[k] = true
		objs = append(objs, customResourcesOf(to, k)...)
	}
	for _, o := range to.Objects {
		if inKeys[o.Key] {
			objs = append(objs, o)
		}
	}
	c.schema, c.checked = newViolations(from, to, objs)
	return c
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
// documents behaviour, it does not configure anything. The render layer
// declares the class (crd-description-changed); reading the class, not the
// path spelling, keeps this aligned with what the differ actually emitted.
func descriptionOnly(c domain.RenderedChange) bool {
	return c.Class == string(render.CRDDescriptionChanged)
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
