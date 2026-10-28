package synthesize

import (
	"fmt"
	"io"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// RenderJudgement writes the judgement of inv as a change-ticket section
// (PD-0c, mission F4): summary and recommendation, disagreements and hard
// findings, impact (with the agent's assessments), implementation,
// validation, backout and audit. b (optional) names the change items.
// Nothing is written when inv has no judgement.
func RenderJudgement(w io.Writer, inv *domain.Investigation, b *domain.EvidenceBundle) error {
	j := inv.Judgement
	if j == nil {
		return nil
	}
	r := &renderer{w: w, titles: map[string]string{}}
	if b != nil {
		for _, it := range b.Items() {
			r.titles[it.ID] = it.Title
		}
	}
	r.summary(j)
	r.findings(j)
	r.impact(j, inv)
	r.implementation(j)
	r.validation(j)
	r.backout(j)
	r.audit(j)
	return r.err
}

type renderer struct {
	w      io.Writer
	titles map[string]string
	err    error
}

func (r *renderer) f(format string, args ...any) {
	if r.err == nil {
		_, r.err = fmt.Fprintf(r.w, format, args...)
	}
}

func (r *renderer) item(id string) string {
	if t := r.titles[id]; t != "" {
		return clip(t, 90) + " [" + id + "]"
	}
	return id
}

func (r *renderer) summary(j *domain.UpgradeJudgement) {
	r.f("\n══ upgrade judgement (advisory; AI judgement for an engineer to approve or deny) ══\n")
	r.f("%s %s → %s · environment %s\n", j.Product.ID, j.From, j.To, j.Environment)
	if j.Status == domain.JudgementFailed {
		r.f("SYNTHESIS FAILED after %d attempt(s): no answer passed the deterministic floor (recorded, not repaired).\n", j.Failure.Attempts)
		for _, v := range j.Failure.Violations {
			r.f("  ✗ %s\n", v)
		}
	} else {
		r.f("Recommendation  %s\n", strings.ToUpper(string(j.Recommendation)))
		r.f("Headline        %s\n", j.Headline)
		if j.Scope != "" {
			r.f("Scope           %s\n", j.Scope)
		}
		for _, p := range j.Preconditions {
			r.f("Precondition    %s\n", p)
		}
	}
	t := j.Target
	r.f("\n── target version ──\n")
	r.f("  current %s · proposed %s", t.Current, t.Proposed)
	if t.Minimum != nil {
		r.f(" · minimum safe %s", t.Minimum)
	}
	if t.Recommended != nil {
		r.f(" · recommended by rule %s", t.Recommended)
	}
	if t.Chosen != nil {
		r.f(" · CHOSEN %s", t.Chosen)
	}
	r.f("\n  rule: %s\n", t.Rule)
	for _, o := range t.Options {
		safe := "safe"
		if !o.Safe {
			safe = "NOT SAFE"
		}
		r.f("  - %-13s %-10s %s. %s\n", o.Role, o.Version, safe, o.Statement)
		for _, c := range o.Charts {
			r.f("      chart %s %s\n", c.Name, c.Version)
		}
		if len(o.Leaves) > 0 {
			r.f("      still affected by: %s\n", strings.Join(o.Leaves, ", "))
		}
		if len(o.Unassessed) > 0 {
			r.f("      not assessable: %s\n", strings.Join(o.Unassessed, ", "))
		}
	}
	if t.Changed && !t.TransitionInvestigated {
		r.f("  ! the %s → %s transition has not been investigated; that investigation is required (see validation).\n", t.Current, t.Chosen)
	}
	if j.Rationale != "" {
		r.f("\n  rationale: %s\n", j.Rationale)
	}
}

func (r *renderer) findings(j *domain.UpgradeJudgement) {
	r.f("\n── disagreements and hard findings (deterministic; an approval against a HARD one is an override) ──\n")
	if len(j.Findings) == 0 {
		r.f("  none\n")
	}
	for _, f := range j.Findings {
		mark := "  "
		if f.Hard {
			mark = "HARD "
		}
		r.f("  %s[%s] %s %s\n", mark, f.ID, f.Kind, f.Statement)
		if len(f.Evidence) > 0 {
			r.f("      evidence %s\n", joinIDs(f.Evidence))
		}
		if f.Comment != "" {
			r.f("      model: %s\n", f.Comment)
		}
	}
	if len(j.Interactions) > 0 {
		r.f("\n── cross-change interactions ──\n")
		for _, x := range j.Interactions {
			r.f("  [%s] %s\n", x.ID, x.Statement)
			if x.Comment != "" {
				r.f("      model: %s\n", x.Comment)
			}
		}
	}
	r.f("\n── residual risks an approval accepts ──\n")
	if len(j.ResidualRisks) == 0 {
		r.f("  none listed\n")
	}
	for _, x := range j.ResidualRisks {
		r.f("  [%s] (%s) %s\n", x.ID, x.Source, x.Statement)
	}
}

func (r *renderer) impact(j *domain.UpgradeJudgement, inv *domain.Investigation) {
	r.f("\n── impact ──\n")
	cls := map[string]domain.InvestigationFinding{}
	for _, f := range inv.Findings {
		cls[f.ChangeItem] = f
	}
	for _, f := range inv.Findings {
		r.f("  %-21s %s (%s)\n", f.Class, r.item(f.ChangeItem), f.Rule)
	}
	if len(inv.AgentAssessments) > 0 {
		r.f("  agent assessments (unverified AI prose):\n")
	}
	for _, a := range inv.AgentAssessments {
		r.f("  · %s\n", r.item(a.ChangeItem))
		if a.Headline != "" {
			r.f("      headline: %s\n", clip(a.Headline, 300))
		}
		r.f("      mechanism: %s\n      likely impact: %s\n", clip(a.Mechanism, 300), clip(a.LikelyImpact, 300))
		for _, s := range a.EffectiveConfiguration {
			r.f("      setting %s: %s%s\n", s.Setting, s.State, prefixed(" — ", clip(s.Detail, 160)))
		}
		for _, p := range a.Preconditions {
			r.f("      precondition: %s\n", clip(p, 200))
		}
	}
	s := j.Summary
	if len(s.Verified)+len(s.Inferred)+len(s.Unknown) > 0 {
		r.f("  upgrade level:\n")
	}
	for _, v := range s.Verified {
		r.f("    verified  %s (%s)\n", v.Statement, joinIDs(v.Citations))
	}
	for _, x := range s.Inferred {
		r.f("    inferred  %s\n", x)
	}
	for _, x := range s.Unknown {
		r.f("    unknown   %s\n", x)
	}
}

func (r *renderer) implementation(j *domain.UpgradeJudgement) {
	r.f("\n── implementation (ordered, isolated steps) ──\n")
	if len(j.Changes) == 0 {
		r.f("  none\n")
	}
	for _, c := range j.Changes {
		where := "proposed (no deterministic location)"
		if l, ok := j.Location(c.Location); ok {
			where = fmt.Sprintf("%s%s %s", l.File, lineSuffix(l.Line), l.Key)
			if l.Value != "" {
				where += " (now " + l.Value + ")"
			}
		}
		r.f("  %d. %-17s %s\n       where: %s\n", c.Order, c.Kind, c.Summary, where)
	}
}

func (r *renderer) validation(j *domain.UpgradeJudgement) {
	r.f("\n── validation before proceeding ──\n")
	if len(j.Validation) == 0 {
		r.f("  none listed\n")
	}
	for _, t := range j.Validation {
		r.f("  - %s\n      expected: %s\n", t.Test, t.Expected)
	}
}

func (r *renderer) backout(j *domain.UpgradeJudgement) {
	r.f("\n── backout ──\n  %s\n", j.Backout.Statement)
	if j.Backout.Notes != "" {
		r.f("  model: %s\n", j.Backout.Notes)
	}
}

func (r *renderer) audit(j *domain.UpgradeJudgement) {
	p := j.Provenance
	r.f("\n── audit ──\n  judgement %s (%s) · %s · model %s (%s) · prompt %s %s\n  call %s · input %s\n",
		j.ID, j.Status, p.Producer, p.Model, p.ModelVersion, p.PromptVersion, p.PromptDigest, p.CallID, j.Input)
}

func prefixed(prefix, s string) string {
	if s == "" {
		return ""
	}
	return prefix + s
}

func joinIDs(ids []domain.EvidenceID) string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return strings.Join(out, ", ")
}
