package classify

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// The deterministic texts of a finding: reasoning, remediation and next
// steps. They say on their face what is VALIDATED (by a deterministic or
// observed validator) and what is an AGENT assertion (AI, never a
// validation) — AC6.

// passWord names what counts as clearing for the item.
func (x *index) passWord(e *itemEval) string {
	if e.item.Manifestation.RenderDecisive() {
		return "passing validation"
	}
	return "clearing validation (a pinned setting or an observed outcome)"
}

func (x *index) validationReason(e *itemEval) domain.UnknownReason {
	if e.item.Manifestation.RenderDecisive() {
		return domain.UnknownEvidenceGap
	}
	return domain.UnknownRuntimeBehaviorGap
}

// suggest names the validators that can decide the item.
func (x *index) suggest(e *itemEval) string {
	if !e.item.Manifestation.RenderDecisive() {
		return "an override inspection establishing the setting that pins the old behaviour, or a runtime validation (observed); render silence proves nothing for this item"
	}
	vs := x.decidingFor(e.item).validators
	if len(vs) == 0 {
		return "an applicable validator"
	}
	return "request_validation with " + strings.Join(vs, " or ")
}

func (x *index) reasonAffected(e *itemEval, failing []domain.ValidationRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "R1 %s → affected. VALIDATED: %d failing validation(s) demonstrate that the change reaches the environment and breaks it:", RuleValidatedExposure, len(failing))
	for _, v := range failing {
		fmt.Fprintf(&b, " %s [%s, %s] — %s.", v.Validator, v.ID, v.Level, clip(firstLine(v.Detail), 300))
	}
	for _, j := range e.joins {
		if j.Classification == domain.ImpactActionRequired {
			fmt.Fprintf(&b, " The deterministic join classifies the change ACTION REQUIRED (%s %s).", j.ID, j.Rule)
		}
	}
	x.agentNote(&b, e)
	return b.String()
}

func (x *index) reasonClear(e *itemEval) string {
	var b strings.Builder
	var names []string
	for _, v := range e.counted {
		names = append(names, fmt.Sprintf("%s [%s, %s]", v.Validator, v.ID, v.Level))
	}
	fmt.Fprintf(&b, "R2 %s → not-affected. VALIDATED by %d passing validation(s): %s.", RuleValidatedClear, len(e.counted), strings.Join(names, "; "))
	fmt.Fprintf(&b, " R2.1 every source and input deciding a %s item is collected", e.item.Family)
	if x.decidingFor(e.item).render {
		b.WriteString(" and the environment render is complete")
	}
	fmt.Fprintf(&b, "; R2.2 %d correlated environment rendered change(s) shown harmless", len(e.envRCs))
	b.WriteString("; R2.3 the traversal is complete for the item")
	switch {
	case len(e.paths) > 0:
		fmt.Fprintf(&b, "; R2.4 %d impact path(s) covered", len(e.paths))
	case len(e.roots) > 0:
		fmt.Fprintf(&b, "; R2.4 %d exposure root(s) covered", len(e.roots))
	default:
		b.WriteString("; R2.4 no exposure root, and a passing validation of the item exists")
	}
	b.WriteString("; R2.5 no failing or erroring validation; R2.6 every agent concern answered by a validation; R2.7 no unvalidated assumption; R2.8 no action-required join verdict.")
	if !e.item.Manifestation.RenderDecisive() {
		fmt.Fprintf(&b, " The item manifests as %s: only validations that establish a pinned setting or observe the behaviour were counted.", e.item.Manifestation)
	}
	x.agentNote(&b, e)
	return b.String()
}

func (x *index) reasonOpen(e *itemEval, r, rule string, class domain.FindingClass, signals []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s → %s. NOT VALIDATED: no validation demonstrates a failure, and the item is not cleared.", r, rule, class)
	if len(signals) > 0 {
		fmt.Fprintf(&b, " Plausible impact: %s.", strings.Join(signals, "; "))
	} else {
		b.WriteString(" No plausible-impact signal exists, but there is not enough evidence to clear the item.")
	}
	fmt.Fprintf(&b, " Not cleared because of %s.", gapSummary(e.gaps))
	if !e.item.Manifestation.RenderDecisive() {
		fmt.Fprintf(&b, " The item manifests as %s: render silence proves nothing; it clears only through an explicit setting that pins the old behaviour (override inspection) or an observed validation.", e.item.Manifestation)
	}
	x.agentNote(&b, e)
	return b.String()
}

// agentNote labels the agent records attached to the finding.
func (x *index) agentNote(b *strings.Builder, e *itemEval) {
	if len(e.asrts) == 0 {
		return
	}
	n := map[domain.AssertionStance]int{}
	for _, a := range e.asrts {
		n[a.Stance]++
	}
	fmt.Fprintf(b, " AGENT (AI output, never a validation): %d assertion(s) attached — %d concern(s), %d mitigation claim(s), %d explanation(s); %d unvalidated assumption(s).",
		len(e.asrts), n[domain.StanceRaisesConcern], n[domain.StanceClaimsMitigation], n[domain.StanceExplains], len(e.assume))
}

func gapSummary(gs []domain.EvidenceGap) string {
	n := map[domain.GapKind]int{}
	for _, g := range gs {
		n[g.Kind]++
	}
	var parts []string
	for _, k := range domain.GapKinds {
		if n[k] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s gap(s)", n[k], k))
		}
	}
	if len(parts) == 0 {
		return "no gap"
	}
	return strings.Join(parts, ", ")
}

func (x *index) remediation(e *itemEval, failing []domain.ValidationRecord) string {
	settings := x.settings(e)
	if len(failing) > 0 {
		s := fmt.Sprintf("Before upgrading, resolve what %s demonstrates (%s)", failing[0].ID, clip(firstLine(failing[0].Detail), 200))
		if len(settings) > 0 {
			s += "; upstream names the setting(s) " + quoteList(settings) + " — set the old behaviour explicitly or adapt the affected configuration"
		} else {
			s += "; adapt the affected configuration to the new release (see the upstream evidence of the change item)"
		}
		return s + "."
	}
	if !e.item.Manifestation.RenderDecisive() && len(settings) > 0 {
		return "If the environment relies on the old behaviour, set it explicitly before upgrading via " + quoteList(settings) + " (named by upstream), then re-run the investigation so an override inspection can establish the pin."
	}
	return ""
}

// nextSteps orders the steps: for an item that does not manifest in rendered
// configuration, first the setting to look for (§3.2), then one step per gap.
func (x *index) nextSteps(e *itemEval, open bool) []string {
	var steps []string
	if open && !e.item.Manifestation.RenderDecisive() {
		steps = append(steps, x.behaviourStep(e)...)
	}
	steps = append(steps, e.steps...)
	if open {
		for _, h := range e.hyps {
			if !e.covered["answer:"+h.ID] {
				steps = append(steps, fmt.Sprintf("Test agent hypothesis %s with a deterministic validator (AI, not yet tested): %q.", h.ID, clip(h.Statement, 160)))
			}
		}
		if a := x.in.Agent; a == nil || a.Run.Status != domain.AgentSubmitted {
			steps = append(steps, "Run the investigation agent to completion to look for consumers and mechanisms the deterministic layers miss.")
		}
	}
	if open && len(steps) == 0 {
		steps = append(steps, "Run an applicable validator for the item: "+x.suggest(e)+".")
	}
	return uniqKeepOrder(steps)
}

// behaviourStep names the setting to look for: the flags, keys, fields,
// annotations and variables the item's upstream text (and the agent's
// hypotheses about it) mention — or, when upstream names none, observation.
func (x *index) behaviourStep(e *itemEval) []string {
	settings := x.settings(e)
	observe := "Observe the behaviour after the upgrade in an isolated cluster (a runtime validation, level observed)"
	if len(settings) == 0 {
		return []string{"Upstream names no setting that restores the old behaviour: " + strings.ToLower(observe[:1]) + observe[1:] + ", or have a human review the upstream text."}
	}
	return []string{
		fmt.Sprintf("Look for an explicit setting that pins the old behaviour — upstream names %s (from the item's upstream evidence) — in the environment's values, manifests and rendered controller configuration; if it is set, request an override inspection (request_validation kind override-inspection) to establish the pinned setting.", quoteList(settings)),
		observe + " if no such setting is set.",
	}
}

var (
	reBacktick = regexp.MustCompile("`([^`\\n]{2,100})`")
	reFlag     = regexp.MustCompile(`(?:^|[\s("'\x60=,])(--[a-z0-9][a-z0-9.-]*[a-z0-9])`)
	reAnnot    = regexp.MustCompile(`\b((?:[a-z0-9-]+\.)+[a-z]{2,}/[A-Za-z0-9][A-Za-z0-9._-]*)`)
	reField    = regexp.MustCompile(`\b((?:spec|data|metadata|status|controller|global)\.[A-Za-z][A-Za-z0-9]*(?:\.[A-Za-z][A-Za-z0-9]*)+)`)
	reEnvVar   = regexp.MustCompile(`\b([A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+)\b`)
	reGate     = regexp.MustCompile(`\b([A-Z][A-Za-z0-9]+=(?:true|false))\b`)
	reSpaceful = regexp.MustCompile(`\s`)
	reURLish   = regexp.MustCompile(`^(?:https?:|www\.)|\.(?:md|html|yaml|yml|json|go)$`)
)

// settings extracts setting names from the item's upstream texts (title,
// subjects, evidence excerpts) and the agent's hypotheses about it. Purely
// lexical and generic: never product code.
func (x *index) settings(e *itemEval) []string {
	texts := []string{e.item.Title}
	for _, id := range e.item.Evidence {
		if ev, ok := x.ev[id]; ok {
			texts = append(texts, ev.Excerpt)
		}
	}
	for _, h := range e.hyps {
		texts = append(texts, h.Statement)
	}
	var out []string
	for _, s := range e.item.Subjects {
		if looksLikeSetting(s) {
			out = append(out, s)
		}
	}
	for _, t := range texts {
		for _, m := range reBacktick.FindAllStringSubmatch(t, -1) {
			if v := strings.TrimSpace(m[1]); looksLikeSetting(v) {
				out = append(out, v)
			}
		}
		for _, re := range []*regexp.Regexp{reFlag, reAnnot, reField, reEnvVar, reGate} {
			for _, m := range re.FindAllStringSubmatch(t, -1) {
				if v := strings.Trim(m[1], ".,;:)"); looksLikeSetting(v) {
					out = append(out, v)
				}
			}
		}
	}
	out = uniqKeepOrder(out)
	// A flag with a value ("--x=y") and the bare flag: keep the bare name once.
	var keep []string
	seen := map[string]bool{}
	for _, s := range out {
		name, _, _ := strings.Cut(s, "=")
		if strings.HasPrefix(s, "--") {
			s = name
		}
		if !seen[s] {
			seen[s] = true
			keep = append(keep, s)
		}
	}
	if len(keep) > 8 {
		keep = keep[:8]
	}
	return keep
}

func looksLikeSetting(s string) bool {
	if len(s) < 3 || len(s) > 100 || reSpaceful.MatchString(s) || reURLish.MatchString(s) {
		return false
	}
	// A bare version or number is not a setting.
	if strings.Trim(s, "v0123456789.-") == "" {
		return false
	}
	return strings.ContainsAny(s, ".-_/=") || hasUpperInside(s)
}

func hasUpperInside(s string) bool {
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func quoteList(xs []string) string {
	q := make([]string, len(xs))
	for i, s := range xs {
		q[i] = "`" + s + "`"
	}
	return strings.Join(q, ", ")
}
