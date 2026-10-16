package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// OverrideInspection establishes, from environment evidence, an explicit
// setting that pins the old (safe) behaviour of a change item — a values
// key, a field or annotation of the objects the change acts on, a
// controller flag, environment variable, feature gate or ConfigMap key — and
// records it as the pass's domain.PinnedSetting, citing the environment
// evidence that states it and, when the setting lands in the environment
// render, the rendered object (DESIGN.md §2.6, §3.1 R2.4). It is the only
// deterministic validation that clears a controller-behaviour item.
//
// Which setting pins which behaviour is never guessed: it comes from a
// declarative, release-level pin rule (RulePinJoin, RulePinDefaultChanged,
// RulePinExposure — see pins.go), whose source the record cites. The setting
// must hold everywhere it matters (every visible object of the kind, every
// render target), so the pass speaks for the whole item. A setting that is
// absent, set only on some objects, or set to a value that changes the
// behaviour is never a pass: the record is not-run with what was found.
//
// Facts come from the bundle's knowledge-fact artifacts related to the item
// (read through WithStore) and from WithFacts; only trusted (deterministic
// or human), active facts count.
func OverrideInspection(opts ...Option) investigate.Validator {
	return &overrideInspection{base{name: NameOverrideInspection, kind: domain.ValidationOverrideInspection, cfg: newConfig(opts)}}
}

type overrideInspection struct{ base }

func (v *overrideInspection) Validate(ctx context.Context, in investigate.ValidationInput) (*investigate.ValidationOutput, error) {
	item, err := prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	cov, err := coverageOf(in, item)
	if err != nil {
		return nil, err
	}
	out := newOutput(v.base, in)
	property := fmt.Sprintf("An explicit setting in the environment pins the old behaviour, so the item does not reach it (%s)", item.Title)
	var notes []string

	// 1. the join's pinned verdict on the item's change.
	rep := in.Report
	if rep != nil {
		if d, note, ok := v.joinPinned(in, item, rep, out); ok {
			d.subjects, d.paths, d.property = cov.subjects, cov.paths, property
			out.add(d)
			return out.result()
		} else if note != "" {
			notes = append(notes, note)
		}
	} else {
		notes = append(notes, "join: no impact report")
	}

	// 2. trusted facts → pin specs, evaluated against the environment.
	facts, factNotes, err := v.facts(ctx, in, item)
	if err != nil {
		return nil, err
	}
	notes = append(notes, factNotes...)
	var specs []pinSpec
	for _, f := range facts {
		s, note := specsFromFact(f.fact, f.artifact)
		specs = append(specs, s...)
		if note != "" {
			notes = append(notes, fmt.Sprintf("fact %s: %s", f.fact.ID, note))
		}
	}
	if len(specs) == 0 {
		out.notRun(cov, "pin:no-rule", property,
			"no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts):\n- "+strings.Join(notes, "\n- "))
		return out.result()
	}
	view, err := loadView(ctx, v.cfg, in.Bundle)
	if err != nil {
		return nil, err
	}
	for _, s := range specs {
		res := s.evaluate(view)
		label := fmt.Sprintf("%s from fact %s: %s %s", s.rule, s.fact.ID, s.setting(), s.acceptance())
		switch {
		case res.undecided != "":
			notes = append(notes, label+" — undecided: "+res.undecided)
			continue
		case !res.held:
			notes = append(notes, label+" — not pinned:\n    "+strings.Join(res.misses, "\n    "))
			continue
		}
		d := draft{rule: s.rule, outcome: domain.ValidationPass, property: property,
			subjects: cov.subjects, paths: cov.paths, inputs: append(res.inputs, s.factArtifact)}
		for _, h := range res.hits {
			d.evidence = append(d.evidence, out.cite(h.evidence))
		}
		for _, e := range s.fact.Evidence {
			d.evidence = append(d.evidence, out.cite(e))
		}
		d.pin = pinOf(s.kind, pinPath(s), res.hits)
		d.detail = fmt.Sprintf("pin rule %s, from fact %s (verification %s): %s\nPinning setting: %s, %s — it holds everywhere it matters:\n%s",
			s.rule, s.fact.ID, s.fact.Level(), factStatement(*s.fact), s.setting(), s.acceptance(), describeHits(res.hits))
		if d.pin.RenderedObject != "" {
			d.detail += "It lands in the environment render in " + d.pin.RenderedObject + "."
		}
		out.add(d)
		return out.result()
	}
	out.notRun(cov, "pin:not-established", property,
		"no pin rule holds in the environment (an absent setting, one set only on some objects, or a value that changes the behaviour is never a pin):\n- "+strings.Join(notes, "\n- "))
	return out.result()
}

func pinPath(s pinSpec) string {
	if s.container != nil {
		return s.container.Name
	}
	return s.path
}

func factStatement(f domain.VerifiedFact) string {
	if s := strings.TrimSpace(f.Assertion.Statement); s != "" {
		return s
	}
	if c := f.Assertion.Consequence; c != nil {
		return c.Statement
	}
	return "(no statement)"
}

// joinPinned turns the join's pinned verdict into a pass draft. It holds only
// when the join's whole verdict on the change is "pinned": a pinned finding
// and no finding that says the default reaches something (applies, adjacent,
// unknown), and — for values — every changed key of the item pinned exactly.
func (v *overrideInspection) joinPinned(in investigate.ValidationInput, item domain.ChangeItem, rep *domain.ImpactReport, out *output) (draft, string, bool) {
	fs := findingsFor(rep, item)
	var pinned []domain.ImpactFinding
	var other []string
	for _, f := range fs {
		switch {
		case f.Rule == joinValuesPinned || f.Rule == joinCRDDefaultPinned:
			pinned = append(pinned, f)
		case f.Classification != domain.ImpactNotAffected:
			other = append(other, fmt.Sprintf("%s %s (%s)", f.ID, f.Rule, f.Classification))
		}
	}
	if len(pinned) == 0 {
		return draft{}, "", false
	}
	if len(other) > 0 {
		return draft{}, "join: the change is pinned only in part — other findings: " + strings.Join(other, ", "), false
	}
	pool := reportPool(rep)
	var hits []pinHit
	keys := map[string]bool{}
	kind := domain.PinField
	path := ""
	for _, f := range pinned {
		for _, m := range f.Matches {
			var h pinHit
			switch {
			case f.Rule == joinValuesPinned && m.Kind == domain.MatchValuesKey:
				kind, path = domain.PinValuesKey, m.Subject
				keys[m.Subject] = true
				h.path = m.Subject
			case f.Rule == joinCRDDefaultPinned && m.Kind == domain.MatchManifestField:
				label, element, ok := strings.Cut(m.Subject, ": ")
				if !ok {
					continue
				}
				obj, ok := objectFromLabel(label)
				if !ok {
					return draft{}, "join: cannot read the pinned resource from " + m.Subject, false
				}
				h.object, h.path, path = obj, element, element
				if strings.HasPrefix(element, "metadata.annotations.") {
					kind = domain.PinAnnotation
				}
			default:
				continue
			}
			for _, id := range m.Evidence {
				e, ok := pool[id]
				if !ok || e.Kind != domain.EvidenceLocalFile {
					continue
				}
				if h.object == "" {
					h.object = e.URI // a values key is set in an environment file
				}
				h.value = valueFromExcerpt(e.Excerpt)
				h.evidence = e
				if h.value != "" {
					hits = append(hits, h)
				}
				break
			}
		}
	}
	if len(hits) == 0 {
		return draft{}, "join: the pinned verdict cites no environment record stating the value", false
	}
	if kind == domain.PinValuesKey {
		var missing []string
		for _, s := range item.Subjects {
			if !keys[s] {
				missing = append(missing, s)
			}
		}
		if len(missing) > 0 {
			return draft{}, "join: values keys of the change not pinned: " + strings.Join(missing, ", "), false
		}
	}
	d := draft{rule: RulePinJoin, outcome: domain.ValidationPass, impactFinding: pinned[0].ID}
	d.inputs = []string{in.Bundle.Change.Edge, in.Bundle.Snapshot.Facts, v.cfg.reportArtifact}
	for _, h := range hits {
		d.evidence = append(d.evidence, out.cite(h.evidence))
	}
	for _, f := range pinned {
		for _, id := range f.UpstreamEvidence {
			if e, ok := pool[id]; ok {
				d.evidence = append(d.evidence, out.cite(e))
			}
		}
	}
	d.pin = pinOf(kind, path, hits)
	if kind != domain.PinValuesKey {
		d.pin.Path = hits[0].path
	}
	d.detail = fmt.Sprintf("pin rule %s: the upstream change is a computed default change and the deterministic join found every relevant environment record setting it explicitly:\n%s", RulePinJoin, describeFindings(pinned))
	return d, "", true
}

// objectFromLabel converts the join's resource label
// ("<apiVersion> <Kind> [<namespace>/]<name>") into a render object id.
func objectFromLabel(label string) (string, bool) {
	parts := strings.Fields(label)
	if len(parts) != 3 {
		return "", false
	}
	group, version := "core", parts[0]
	if g, ver, ok := strings.Cut(parts[0], "/"); ok {
		group, version = g, ver
	}
	ns, name := "", parts[2]
	if a, b, ok := strings.Cut(parts[2], "/"); ok {
		ns, name = a, b
	}
	return strings.Join([]string{group, version, parts[1], ns, name}, "/"), true
}

// valueFromExcerpt reads "key: value" from an environment evidence excerpt
// (the line of the file) as a JSON-encoded scalar; "" when it is not one.
func valueFromExcerpt(excerpt string) string {
	line := strings.TrimSpace(excerpt)
	line = strings.TrimPrefix(line, "- ")
	var m map[string]any
	if yaml.Unmarshal([]byte(line), &m) != nil || len(m) != 1 {
		return ""
	}
	for _, x := range m {
		switch x.(type) {
		case string, bool, int, float64, nil:
			b, _ := json.Marshal(x)
			return string(b)
		}
	}
	return ""
}

type factRef struct {
	fact     domain.VerifiedFact
	artifact string
}

// facts returns the trusted, active facts attached to the item: the bundle's
// knowledge-fact artifacts related to it, and WithFacts facts the report or
// the upgrade edge attaches to its change.
func (v *overrideInspection) facts(ctx context.Context, in investigate.ValidationInput, item domain.ChangeItem) ([]factRef, []string, error) {
	var out []factRef
	var notes []string
	seen := map[string]bool{}
	take := func(f domain.VerifiedFact, artifact string) {
		if seen[f.ID] {
			return
		}
		seen[f.ID] = true
		switch {
		case f.Status != domain.FactActive:
			notes = append(notes, fmt.Sprintf("fact %s is %s", f.ID, f.Status))
		case !f.Level().Trusted():
			notes = append(notes, fmt.Sprintf("fact %s is verified only at level %s: a consensus or proxy fact is never a pin rule", f.ID, orAny(string(f.Level()))))
		default:
			out = append(out, factRef{fact: f, artifact: artifact})
		}
	}
	b := in.Bundle
	for _, a := range b.Manifest.Artifacts {
		if a.Kind != domain.ArtifactKnowledgeFact || !contains(a.ChangeItems, item.ID) {
			continue
		}
		data, _, err := v.cfg.read(ctx, b, a.ID)
		if err != nil {
			notes = append(notes, fmt.Sprintf("knowledge fact %s unreadable: %v", a.Path, err))
			continue
		}
		f, err := parseFact(data)
		if err != nil {
			notes = append(notes, fmt.Sprintf("knowledge fact %s: %v", a.Path, err))
			continue
		}
		take(f, a.ID)
	}
	if len(v.cfg.facts) > 0 && item.ChangeID != "" {
		attached := map[string]bool{}
		if in.Report != nil {
			for _, f := range in.Report.Findings {
				if f.Knowledge != nil && f.ChangeID == item.ChangeID {
					attached[f.Knowledge.Fact] = true
				}
			}
		}
		edge, err := v.edge(ctx, b)
		if err != nil && len(attached) == 0 {
			notes = append(notes, "supplied facts: attachment to the change not decidable: "+err.Error())
		}
		for _, f := range v.cfg.facts {
			if !attached[f.ID] && edge != nil {
				for _, c := range impact.AttachedChanges(f, edge) {
					attached[f.ID] = attached[f.ID] || c.ID == item.ChangeID
				}
			}
			if attached[f.ID] {
				take(f, "")
			}
		}
	}
	if len(out)+len(notes) == 0 {
		notes = append(notes, "no verified fact is attached to the item")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].fact.ID < out[j].fact.ID })
	return out, notes, nil
}

// edge reads the upgrade edge the change record references.
func (v *overrideInspection) edge(ctx context.Context, b *domain.EvidenceBundle) (*domain.UpgradeEdge, error) {
	return readEdge(ctx, v.cfg, b)
}

func readEdge(ctx context.Context, c config, b *domain.EvidenceBundle) (*domain.UpgradeEdge, error) {
	data, _, err := c.read(ctx, b, b.Change.Edge)
	if err != nil {
		return nil, err
	}
	var e domain.UpgradeEdge
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("upgrade edge: %w", err)
	}
	return &e, nil
}

// parseFact reads a verified fact: the knowledge/ record form
// ({"kind":"fact","fact":{...}}) or a bare domain.VerifiedFact.
func parseFact(data []byte) (domain.VerifiedFact, error) {
	var rec struct {
		Kind string               `json:"kind"`
		Fact *domain.VerifiedFact `json:"fact"`
	}
	if err := json.Unmarshal(data, &rec); err == nil && rec.Fact != nil {
		return *rec.Fact, nil
	}
	var f domain.VerifiedFact
	if err := json.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("not a verified fact: %w", err)
	}
	if f.ID == "" {
		return f, fmt.Errorf("not a verified fact (no id)")
	}
	return f, nil
}
