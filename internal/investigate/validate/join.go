package validate

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// JoinVerdicts wraps the decisive verdicts of the deterministic join (the
// five-class domain.ImpactReport of the same upgrade and environment) as
// validation records (DESIGN.md §2.6, §3.5). At most one record per item:
//
//   - any action-required finding of the item's change (incl.
//     impact:render-target-rejects) → fail. A model-consensus fact may yield
//     ACTION REQUIRED (PO-2); the fail says so in its detail;
//   - every finding of the change not-affected, deterministic, with checks,
//     from a trusted fact when one is involved → pass;
//   - every finding not-affected but one rests on no render
//     (impact:values-default-unrendered, or a render check whose outcome is
//     unavailable) → not-run: a missing render is a gap, because approval
//     needs evidence;
//   - review-required / informational / unknown, or a mix → no record (not
//     decisive).
//
// report may be nil: the input's Report is used then. The records speak for
// the item as a whole (no subjects or paths): the join's verdict is about the
// change, not about one consumer.
func JoinVerdicts(report *domain.ImpactReport, opts ...Option) investigate.Validator {
	return &joinVerdicts{base: base{name: NameJoin, kind: domain.ValidationOverrideInspection, cfg: newConfig(opts)}, report: report}
}

type joinVerdicts struct {
	base
	report *domain.ImpactReport
}

const joinProperty = "The deterministic join (five-class impact report) finds that the change does not reach or break the environment"

func (v *joinVerdicts) Validate(ctx context.Context, in investigate.ValidationInput) (*investigate.ValidationOutput, error) {
	item, err := prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	out := newOutput(v.base, in)
	rep := v.report
	if rep == nil {
		rep = in.Report
	}
	if rep == nil {
		out.notRun(coverage{}, "join:no-report", joinProperty, "no impact report: the deterministic join did not run for this upgrade and environment")
		return out.result()
	}
	fs := findingsFor(rep, item)
	if len(fs) == 0 {
		return out.result()
	}
	pool := reportPool(rep)
	var actions, cleared, unrendered []domain.ImpactFinding
	decisive := true
	for _, f := range fs {
		switch {
		case f.Classification == domain.ImpactActionRequired:
			actions = append(actions, f)
		case f.Classification != domain.ImpactNotAffected:
			decisive = false
		case rendersNothing(f):
			unrendered = append(unrendered, f)
		default:
			cleared = append(cleared, f)
		}
	}
	switch {
	case len(actions) > 0:
		d := v.draft(in, actions, pool, out)
		d.outcome = domain.ValidationFail
		d.rule = "join:action-required"
		d.property = joinProperty
		d.detail = "ACTION REQUIRED by the deterministic join — the change demonstrably reaches the environment and breaks it if nothing is done:\n" + describeFindings(actions)
		out.add(uncited(d))
	case !decisive:
		// review-required / informational / unknown: no record.
	case len(unrendered) > 0:
		d := v.draft(in, unrendered, pool, out)
		d.outcome = domain.ValidationNotRun
		d.rule = "join:unrendered"
		d.property = joinProperty
		d.detail = "the join's not-affected verdict rests on no render of the environment (a gap, not evidence; approval needs a render):\n" + describeFindings(unrendered)
		d.evidence, d.inputs = nil, nil
		out.add(d)
	default:
		var weak []string
		for _, f := range cleared {
			if why := unwrappable(f); why != "" {
				weak = append(weak, f.ID+": "+why)
			}
		}
		d := v.draft(in, cleared, pool, out)
		d.property = joinProperty
		if len(weak) > 0 {
			d.outcome = domain.ValidationNotRun
			d.rule = "join:untrusted"
			d.detail = "the join's not-affected verdict cannot be wrapped as a validation: " + strings.Join(weak, "; ")
			d.evidence, d.inputs = nil, nil
		} else {
			d.outcome = domain.ValidationPass
			d.rule = "join:not-affected"
			d.detail = "NOT AFFECTED by the deterministic join (evaluated against the environment, with checks):\n" + describeFindings(cleared)
		}
		out.add(uncited(d))
	}
	return out.result()
}

// uncited turns a decisive draft whose evidence does not resolve into a
// not-run: a verdict that cannot be audited is not a validation.
func uncited(d draft) draft {
	if d.outcome.Decisive() && len(d.evidence) == 0 {
		d.outcome = domain.ValidationNotRun
		d.detail = "the join's verdict cites no evidence that resolves in the report or the bundle:\n" + d.detail
		d.inputs = nil
	}
	return d
}

// draft builds the shared part of a wrapped record: the deciding finding,
// the inputs the join read and the evidence both chains cite.
func (v *joinVerdicts) draft(in investigate.ValidationInput, fs []domain.ImpactFinding, pool map[domain.EvidenceID]domain.Evidence, out *output) draft {
	b := in.Bundle
	d := draft{kind: joinKind(fs), impactFinding: fs[0].ID}
	d.inputs = []string{b.Change.Edge, b.Snapshot.Facts, v.cfg.reportArtifact}
	renders := false
	for _, f := range fs {
		ids := append(append([]domain.EvidenceID{}, f.UpstreamEvidence...), f.EnvironmentEvidence...)
		for _, m := range f.Matches {
			ids = append(ids, m.Evidence...)
			renders = renders || m.Kind == domain.MatchRenderedChange
		}
		for _, c := range f.Checks {
			ids = append(ids, c.Evidence...)
			renders = renders || c.Dimension == domain.DimensionRender
		}
		for _, id := range ids {
			if e, ok := pool[id]; ok {
				d.evidence = append(d.evidence, out.cite(e))
			} else if out.known(id) {
				d.evidence = append(d.evidence, id)
			}
		}
	}
	if renders {
		for _, p := range b.Renders {
			d.inputs = append(d.inputs, p.From, p.To, p.Diff)
		}
	}
	return d
}

// findingsFor returns the report findings of the item's change, sorted by id.
// A compatibility item (anchored on a platform) takes the platform checks no
// change id names.
func findingsFor(rep *domain.ImpactReport, item domain.ChangeItem) []domain.ImpactFinding {
	var out []domain.ImpactFinding
	for _, f := range rep.Findings {
		switch {
		case item.ChangeID != "":
			if f.ChangeID == item.ChangeID {
				out = append(out, f)
			}
		case item.Platform != "":
			if f.ChangeID == "" && platformFinding(f, item.Platform) {
				out = append(out, f)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func platformFinding(f domain.ImpactFinding, platform string) bool {
	for _, c := range f.Checks {
		if c.Dimension == domain.DimensionCluster && strings.EqualFold(c.Platform, platform) {
			return true
		}
	}
	for _, m := range f.Matches {
		if m.Kind == domain.MatchKubernetes && strings.EqualFold(platform, "kubernetes") {
			return true
		}
	}
	return false
}

// rendersNothing: a not-affected verdict that rests on an unavailable render.
func rendersNothing(f domain.ImpactFinding) bool {
	if f.Rule == domain.RuleValuesDefaultUnrendered {
		return true
	}
	for _, c := range f.Checks {
		if c.Render != nil && c.Render.Outcome == domain.RenderUnavailable {
			return true
		}
	}
	return false
}

// unwrappable says why a not-affected finding cannot become a pass ("" when
// it can): only deterministic findings with an evaluation record, and only
// facts at a trusted level (the trust ladder; consensus and proxy facts
// never clear).
func unwrappable(f domain.ImpactFinding) string {
	if !f.Provenance.Deterministic() {
		return "not deterministic"
	}
	if len(f.Checks) == 0 {
		return "no evaluation record (checks)"
	}
	if k := f.Knowledge; k != nil && !k.Verification.Trusted() {
		return fmt.Sprintf("rests on fact %s verified only at level %s (consensus and proxy facts never clear)", k.Fact, k.Verification)
	}
	return ""
}

func describeFindings(fs []domain.ImpactFinding) string {
	var b strings.Builder
	for _, f := range fs {
		fmt.Fprintf(&b, "- %s %s (%s): %s", f.ID, f.Rule, f.Classification, f.Title)
		if k := f.Knowledge; k != nil {
			label := "verification " + string(k.Verification)
			if f.Classification == domain.ImpactActionRequired {
				label = k.ActionLabel()
			}
			fmt.Fprintf(&b, " [fact %s, %s]", k.Fact, label)
		}
		for _, c := range f.Checks {
			fmt.Fprintf(&b, "\n  checked %s: %s against %s", c.Dimension, plural(c.Facts, "fact", "facts"), strings.Join(c.Subjects, ", "))
			if c.Render != nil {
				fmt.Fprintf(&b, " (render %s for %s)", c.Render.Outcome, c.Render.Key)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// joinKind names the kind of validation a join verdict amounts to, from the
// evidence that decided it: a render, schema/API/cluster facts, or settings.
func joinKind(fs []domain.ImpactFinding) domain.ValidationKind {
	schema := false
	for _, f := range fs {
		for _, c := range f.Checks {
			switch c.Dimension {
			case domain.DimensionRender:
				return domain.ValidationRenderDiff
			case domain.DimensionCRDs, domain.DimensionManifests, domain.DimensionCluster:
				schema = true
			}
		}
		for _, m := range f.Matches {
			switch m.Kind {
			case domain.MatchRenderedChange:
				return domain.ValidationRenderDiff
			case domain.MatchAPIVersion, domain.MatchCRD, domain.MatchCRDVersion, domain.MatchKubernetes:
				schema = true
			}
		}
	}
	if schema {
		return domain.ValidationSchemaCompat
	}
	return domain.ValidationOverrideInspection
}

// reportPool indexes both evidence chains of the report.
func reportPool(rep *domain.ImpactReport) map[domain.EvidenceID]domain.Evidence {
	m := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range rep.Evidence {
		m[e.ID] = e
	}
	for _, e := range rep.EnvironmentEvidence {
		m[e.ID] = e
	}
	return m
}
