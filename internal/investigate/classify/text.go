package classify

import (
	"fmt"
	"io"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// WriteText renders an investigation's findings and decision for a human.
// Every finding shows, in separate labelled sections, what is VALIDATED
// (deterministic or observed validation records) and what is NOT — agent
// assertions (AI output), unvalidated assumptions, evidence gaps — so the
// distinction holds on the face of the text, not only in the struct (AC6).
//
// The action axis of §3.5 is shown next to the class and is never raised by
// the investigation: "action: ACTION REQUIRED" only when the deterministic
// join says so; an affected finding the join classifies below action shows
// "action: review".
func WriteText(w io.Writer, inv *domain.Investigation, b *domain.EvidenceBundle) error {
	p := &printer{w: w}
	vals := map[string]domain.ValidationRecord{}
	for _, v := range inv.Validations {
		vals[v.ID] = v
	}
	asrts := map[string]domain.AgentAssertion{}
	for _, a := range inv.Assertions {
		asrts[a.ID] = a
	}
	p.f("Investigation %s — environment %s, bundle %s\n", inv.ID, inv.Environment, inv.Bundle)
	if d := inv.Decision; d != nil {
		p.f("Decision: %s (%s)\n", strings.ToUpper(string(d.Outcome)), d.Policy)
		for _, r := range d.Reasons {
			p.f("  - %s\n", r)
		}
	}
	p.f("Review: %s", inv.Review.State)
	if inv.Review.Reviewer != "" {
		p.f(" by %s:%s", inv.Review.ReviewerKind, inv.Review.Reviewer)
	}
	if inv.Review.ReviewerKind == domain.ReviewerProxy {
		p.f(" (proxy = AI reviewer: recorded, never an approval)")
	}
	p.f("\n")
	for _, f := range inv.Findings {
		title := f.ChangeItem
		if b != nil {
			if it, ok := b.Item(f.ChangeItem); ok {
				title = it.Title
			}
		}
		p.f("\n%s · action: %s — %s [%s]\n", classLabel(f), actionAxis(f), title, f.ChangeItem)
		p.f("  rule %s · manifestation %s · finding %s\n", f.Rule, f.Manifestation, f.ID)
		p.f("  Reasoning (deterministic policy): %s\n", f.Reasoning)

		var cited, other []domain.ValidationRecord
		citedSet := map[string]bool{}
		for _, id := range f.Validations {
			citedSet[id] = true
		}
		for _, v := range inv.Validations {
			if v.ChangeItem != f.ChangeItem {
				continue
			}
			if citedSet[v.ID] {
				cited = append(cited, v)
			} else {
				other = append(other, v)
			}
		}
		switch f.Class {
		case domain.FindingAffected, domain.FindingNotAffected:
			p.f("  VALIDATED — the class rests on these %s validation(s):\n", f.Basis)
			for _, v := range cited {
				p.validation(v)
			}
			if len(other) > 0 {
				p.f("  Other validations of the item (not what the class rests on):\n")
				for _, v := range other {
					p.validation(v)
				}
			}
		default:
			p.f("  NOT VALIDATED — no validation decides the item. Validation records:\n")
			if len(cited) == 0 {
				p.f("    (none)\n")
			}
			for _, v := range cited {
				p.validation(v)
				if v.Outcome == domain.ValidationPass && !v.DocumentationOnly && !f.Manifestation.RenderDecisive() && !v.ClearsBehaviour() {
					p.f("      ↳ not decisive for this item: it manifests as %s, and only a pinned setting or an observed outcome clears it\n", f.Manifestation)
				}
			}
		}
		assumed := map[string]bool{}
		for _, id := range f.Assumptions {
			assumed[id] = true
		}
		if len(f.Assumptions) > 0 {
			p.f("  UNVALIDATED ASSUMPTIONS (agent claims, AI output — no validation established them):\n")
			for _, id := range f.Assumptions {
				p.assertion(asrts[id], id)
			}
		}
		var concerns, explains, settled []string
		for _, id := range f.Assertions {
			if assumed[id] {
				continue
			}
			switch asrts[id].Stance {
			case domain.StanceRaisesConcern:
				concerns = append(concerns, id)
			case domain.StanceClaimsMitigation:
				settled = append(settled, id)
			default:
				explains = append(explains, id)
			}
		}
		if len(concerns) > 0 {
			p.f("  Agent concerns (AI output, not validations):\n")
			for _, id := range concerns {
				p.assertion(asrts[id], id)
			}
		}
		if len(settled) > 0 {
			p.f("  Agent mitigation claims a validation established (the validation, not the claim, is the evidence):\n")
			for _, id := range settled {
				p.assertion(asrts[id], id)
			}
		}
		if len(explains) > 0 {
			p.f("  Agent explanations (AI output, not validations; they change nothing):\n")
			for _, id := range explains {
				p.assertion(asrts[id], id)
			}
		}
		if len(f.Gaps) > 0 {
			p.f("  Evidence gaps:\n")
			for _, g := range f.Gaps {
				reason := ""
				if g.UnknownReason != "" {
					reason = " [" + string(g.UnknownReason) + "]"
				}
				p.f("    - %s %s%s: %s\n", g.Kind, g.Subject, reason, g.Detail)
			}
		}
		if len(f.RenderedChanges) > 0 {
			p.f("  Environment rendered changes: %s\n", strings.Join(f.RenderedChanges, ", "))
		}
		if len(f.Paths) > 0 {
			p.f("  Impact paths: %s\n", strings.Join(f.Paths, ", "))
		}
		for _, r := range f.ImpactFindings {
			p.f("  Deterministic join: %s %s (%s)\n", r.ID, r.Classification, r.Rule)
		}
		if f.Remediation != "" {
			p.f("  Remediation: %s\n", f.Remediation)
		}
		if len(f.NextSteps) > 0 {
			p.f("  Next steps:\n")
			for _, s := range f.NextSteps {
				p.f("    - %s\n", s)
			}
		}
		if f.Approval.Eligible {
			p.f("  Approval: eligible (shadow mode — a human still approves)\n")
		} else {
			p.f("  Approval: human review required — %s\n", strings.Join(f.Approval.Blockers, "; "))
		}
	}
	// CONTRACT-CHANGE(agent): investigation-level agent records (additive;
	// shown only when present).
	if len(inv.AgentClaims) > 0 {
		p.f("\nAgent environment claims (AI claims checked deterministically against the bundle — evidence for the agent's records, not validations):\n")
		for _, c := range inv.AgentClaims {
			v := ""
			if c.Value != "" {
				v = " = " + c.Value
			}
			p.f("  ✓ %s %s %s%s in %s (%s) — %s\n", c.ID, c.Kind, c.Setting, v, c.Input, c.Verifier, clip(c.Detail, 200))
		}
	}
	if len(inv.AgentNotes) > 0 {
		p.f("\nAgent notes (AI, investigation-level — never item concerns, no effect on any class):\n")
		for _, n := range inv.AgentNotes {
			p.f("    ? %s about %s (%s) — %s\n", n.ID, n.About, n.Reason, clip(n.Statement, 300))
		}
	}
	return p.err
}

func classLabel(f domain.InvestigationFinding) string {
	switch f.Class {
	case domain.FindingAffected:
		return fmt.Sprintf("AFFECTED (validated, %s)", f.Basis)
	case domain.FindingNotAffected:
		return fmt.Sprintf("NOT AFFECTED (validated, %s)", f.Basis)
	case domain.FindingPotentiallyAffected:
		return "POTENTIALLY AFFECTED (not validated)"
	}
	return "INSUFFICIENT EVIDENCE (not validated)"
}

// actionAxis is the five-class action axis (§3.5): the join's verdict, never
// raised by the investigation.
func actionAxis(f domain.InvestigationFinding) string {
	rank := map[domain.ImpactClass]int{domain.ImpactNotAffected: 1, domain.ImpactUnknown: 2, domain.ImpactInformational: 3, domain.ImpactReviewRequired: 4, domain.ImpactActionRequired: 5}
	best := domain.ImpactClass("")
	for _, r := range f.ImpactFindings {
		if rank[r.Classification] > rank[best] {
			best = r.Classification
		}
	}
	switch {
	case best == domain.ImpactActionRequired:
		return "ACTION REQUIRED (deterministic join)"
	case f.Class == domain.FindingAffected:
		return "review"
	case best == "":
		return "none (no join verdict)"
	}
	return string(best) + " (deterministic join)"
}

type printer struct {
	w   io.Writer
	err error
}

func (p *printer) f(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, args...)
	}
}

func (p *printer) validation(v domain.ValidationRecord) {
	mark := map[domain.ValidationOutcomeKind]string{domain.ValidationPass: "pass", domain.ValidationFail: "FAIL", domain.ValidationError: "error", domain.ValidationNotRun: "not-run"}[v.Outcome]
	extra := ""
	if v.Pin != nil {
		extra = fmt.Sprintf(" · pinned setting %s %s=%s on %s", v.Pin.Kind, v.Pin.Path, v.Pin.Value, v.Pin.Subject)
	}
	if v.DocumentationOnly {
		extra = " · documentation only: accounts for the CRD description changes it names (R2.2), clears nothing"
	}
	detail := firstLine(v.Detail)
	if detail == "" {
		detail = v.Property
	}
	p.f("    [%s] %s %s (%s, %s)%s — %s\n", mark, v.Validator, v.ID, v.Kind, v.Level, extra, clip(detail, 300))
}

func (p *printer) assertion(a domain.AgentAssertion, id string) {
	if a.ID == "" {
		p.f("    ? %s (not in the run record)\n", id)
		return
	}
	label := "AI: " + a.Provenance.Model
	if a.StatementKind == domain.StatementUnverifiedProse { // CONTRACT-CHANGE(agent): prose labels (prompt v5)
		label += ", unverified prose"
	}
	p.f("    ? %s %s (%s) — %s\n", a.Stance, a.ID, label, clip(a.Statement, 300))
	if len(a.Claims) > 0 {
		p.f("      rests on verified claims: %s\n", strings.Join(a.Claims, ", "))
	}
}
