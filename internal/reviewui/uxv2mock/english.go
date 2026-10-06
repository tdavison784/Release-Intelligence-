// Command uxv2mock renders the step-1 "decision card" mockup for the reviewux
// lane from the review UI demo fixtures. Every string it shows is derived
// deterministically from the fixture data — no LLM, no hand-written prose.
// It is a throwaway: step 2 ports these renderers into internal/reviewui with
// tests and deletes this package.
package main

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// --- words for enums ------------------------------------------------------------------

var kindWords = map[domain.ConsequenceKind]string{
	domain.ConsequenceUpgradeBlocked:     "the upgrade is blocked",
	domain.ConsequenceResourceRejected:   "the resource is rejected",
	domain.ConsequenceSettingIgnored:     "the setting is ignored",
	domain.ConsequenceBehaviorChange:     "its behaviour changes",
	domain.ConsequencePermissionLost:     "permissions are lost",
	domain.ConsequenceWorkloadFailure:    "workloads fail",
	domain.ConsequenceMigrationRequired:  "a migration is required",
	domain.ConsequenceDeprecation:        "it is deprecated but still works",
	domain.ConsequenceSupersededUpstream: "it is superseded upstream",
	domain.ConsequenceNone:               "nothing needs to be done",
}

var changeWords = map[domain.ChangeKind]string{
	domain.ChangeKindAdded:               "was added",
	domain.ChangeKindRemoved:             "was removed",
	domain.ChangeKindRenamed:             "was renamed",
	domain.ChangeKindDefaultChanged:      "default changed",
	domain.ChangeKindValueChanged:        "value changed",
	domain.ChangeKindBehaviorChanged:     "behaviour changed",
	domain.ChangeKindDeprecated:          "was deprecated",
	domain.ChangeKindNowRequired:         "became required",
	domain.ChangeKindValidationTightened: "validation was tightened",
	domain.ChangeKindRequirementChanged:  "the requirement changed",
	domain.ChangeKindMigrationRequired:   "a migration became required",
}

var aspectWords = map[domain.Aspect]string{
	domain.AspectSubject:       "what the thing is",
	domain.AspectChange:        "what changed",
	domain.AspectApplicability: "who is exposed",
	domain.AspectConsequence:   "what happens",
}

func kindWord(k domain.ConsequenceKind) string {
	if w, ok := kindWords[k]; ok {
		return w
	}
	return strings.ReplaceAll(string(k), "-", " ")
}

func changeWord(k domain.ChangeKind) string {
	if w, ok := changeWords[k]; ok {
		return w
	}
	return strings.ReplaceAll(string(k), "-", " ")
}

func classLabel(c domain.ImpactClass) string {
	switch c {
	case domain.ImpactActionRequired:
		return "ACTION REQUIRED"
	case domain.ImpactReviewRequired:
		return "REVIEW REQUIRED"
	case domain.ImpactInformational:
		return "INFORMATIONAL"
	case domain.ImpactNotAffected:
		return "NOT AFFECTED"
	}
	return strings.ToUpper(strings.ReplaceAll(string(c), "-", " "))
}

func severityWord(s domain.ImpactSeverity) string {
	switch s {
	case domain.SeverityCritical:
		return "critical"
	case domain.SeverityHigh:
		return "high"
	case domain.SeverityMedium:
		return "medium"
	case domain.SeverityLow:
		return "low"
	}
	return string(s)
}

// modelShort turns a raw model id into a display name (claude-opus-5-5 →
// Opus 5.5, glm-5.3-flash → GLM 5.3 Flash); the raw id stays in tooltips
// and the details section.
var modelWords = map[string]string{
	"opus": "Opus", "sonnet": "Sonnet", "haiku": "Haiku", "glm": "GLM",
	"flash": "Flash", "mini": "Mini", "pro": "Pro",
}

func modelShort(m string) string {
	var out []string
	for _, t := range strings.Split(strings.TrimPrefix(m, "claude-"), "-") {
		if w, ok := modelWords[t]; ok {
			out = append(out, w)
			continue
		}
		out = append(out, t)
	}
	// join numeric runs: 5 5 → 5.5
	var joined []string
	for i := 0; i < len(out); i++ {
		if i+1 < len(out) && isNum(out[i]) && isNum(out[i+1]) {
			joined = append(joined, out[i]+"."+out[i+1])
			i++
			continue
		}
		joined = append(joined, out[i])
	}
	return strings.Join(joined, " ")
}

func isNum(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// --- subjects, changes, consequences ----------------------------------------------------

func subjectPhrase(s *domain.Subject) string {
	if s == nil {
		return "this"
	}
	switch s.Family {
	case domain.SubjectCRDField:
		kind := s.Kind
		if kind == "" {
			kind = "resource"
		}
		return fmt.Sprintf("the %s field `%s`", kind, s.Path)
	case domain.SubjectCLIFlag:
		if s.Component != "" {
			return fmt.Sprintf("the %s flag `--%s`", s.Component, s.Name)
		}
		return fmt.Sprintf("the flag `--%s`", s.Name)
	case domain.SubjectHelmValue:
		return fmt.Sprintf("the Helm value `%s`", s.Path)
	case domain.SubjectConfigKey:
		return fmt.Sprintf("the config key `%s`", s.Path)
	case domain.SubjectProductRelationship:
		return fmt.Sprintf("how %s works together with %s", s.Product, s.Name)
	}
	var id []string
	for _, p := range []string{s.Group, s.Kind, s.Name, s.Path, s.Component} {
		if p != "" {
			id = append(id, p)
		}
	}
	if len(id) == 0 {
		return string(s.Family)
	}
	return fmt.Sprintf("%s `%s`", strings.ReplaceAll(string(s.Family), "-", " "), strings.Join(id, " / "))
}

// cleanVal strips JSON-ish quoting for inline display ("RSA" → RSA).
func cleanVal(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`)) {
		return strings.Trim(v, `"`)
	}
	return v
}

func vals(vs []string) string {
	var out []string
	for _, v := range vs {
		out = append(out, cleanVal(v))
	}
	return strings.Join(out, " or ")
}

func plural(kind string) string {
	if kind == "" {
		return "resources"
	}
	switch {
	case strings.HasSuffix(kind, "s"), strings.HasSuffix(kind, "ss"):
		return kind
	case strings.HasSuffix(kind, "y"):
		return kind[:len(kind)-1] + "ies"
	}
	return kind + "s"
}

// condEnglish renders a condition tree as one plain-English clause. scope is
// the noun the condition applies to ("Certificates", "environments"); it is
// only used at the root.
func condEnglish(c domain.Condition, root bool) string {
	switch c.Op {
	case domain.OpAll, domain.OpAny:
		var parts []string
		for _, o := range c.Of {
			parts = append(parts, condEnglish(o, false))
		}
		join := " or "
		if c.Op == domain.OpAll {
			join = " and "
		}
		body := strings.Join(parts, join)
		if len(parts) > 2 {
			body = strings.Join(parts, "; ")
			if c.Op == domain.OpAll {
				return "all of: " + body
			}
			return "any of: " + body
		}
		if root {
			return body
		}
		return "(" + body + ")"
	case domain.OpNot:
		if len(c.Of) == 1 {
			return "not (" + condEnglish(c.Of[0], false) + ")"
		}
	case domain.OpResource, domain.OpRef:
		noun := plural(c.Kind)
		if c.Kind == "" {
			noun = "resources"
		}
		if c.Op == domain.OpRef {
			noun = "the resource behind `" + c.Path + "`"
		}
		var parts []string
		for _, o := range c.Of {
			parts = append(parts, condEnglish(o, false))
		}
		if len(parts) == 0 {
			return "any " + noun
		}
		return noun + " where " + strings.Join(parts, " and ")
	case domain.OpField:
		return fieldClause(c)
	case domain.OpUndecidable:
		need := c.Needed
		if need == "" {
			need = "more environment input"
		}
		return "cannot be decided from the environment (needs " + need + ")"
	}
	return leafClause(c)
}

func fieldClause(c domain.Condition) string {
	path := "`" + c.Path + "`"
	switch c.State {
	case domain.StateUnset:
		return path + " is left unset"
	case domain.StateSet:
		return path + " is set (any value)"
	case domain.StateEquals:
		return path + " is " + vals(c.Values)
	case domain.StateNotEquals:
		return path + " is not " + vals(c.Values)
	case domain.StateMatches:
		return path + " matches /" + c.Pattern + "/"
	case domain.StateHasToken:
		return path + " contains the entry " + vals(c.Values)
	case domain.StateHasTokenKey:
		return path + " contains an entry keyed " + vals(c.Values)
	}
	return leafClause(c)
}

func leafClause(c domain.Condition) string {
	var what string
	switch c.Op {
	case domain.OpValuesKey:
		what = "the Helm values set `" + c.Path + "`"
	case domain.OpCLIFlag:
		what = "the flag `--" + c.Name + "`"
	case domain.OpEnvVar:
		what = "the environment variable `" + c.Name + "`"
	case domain.OpImageInUse:
		what = "the image `" + c.Name + "`"
	case domain.OpGVKInUse:
		what = strings.TrimPrefix(strings.Join([]string{c.Group, c.Kind}, " "), " ")
		what = "the resource kind " + what + " in use"
	case domain.OpProductVersion:
		return versionClause(c)
	default:
		what = string(c.Op)
		if c.Path != "" {
			what += " `" + c.Path + "`"
		}
		if c.Name != "" {
			what += " " + c.Name
		}
	}
	switch c.State {
	case domain.StateUnset:
		return what + " is left unset"
	case domain.StateSet:
		return what + " is set"
	case domain.StateInRange:
		return what + " is within " + c.Range
	case domain.StateOutOfRange:
		return what + " is outside " + c.Range
	case domain.StateExists:
		return what + " exists"
	case domain.StateNone:
		return what + " is absent"
	}
	if len(c.Values) > 0 {
		return what + " is " + vals(c.Values)
	}
	return what
}

func versionClause(c domain.Condition) string {
	name := c.Name
	r := strings.TrimSpace(c.Range)
	low := strings.TrimLeft(r, "<>=~^")
	switch {
	case strings.HasPrefix(r, ">="):
		return fmt.Sprintf("%s is older than %s", name, low)
	case strings.HasPrefix(r, ">"):
		return fmt.Sprintf("%s is %s or older", name, low)
	case strings.HasPrefix(r, "<="):
		return fmt.Sprintf("%s is newer than %s", name, low)
	case strings.HasPrefix(r, "<"):
		return fmt.Sprintf("%s is %s or newer", name, low)
	}
	return fmt.Sprintf("%s is outside %s", name, r)
}

// exposurePhrase renders an Applicability as who-is-exposed English.
func exposurePhrase(a *domain.Applicability) string {
	if a == nil {
		return ""
	}
	out := condEnglish(a.Exposure, true)
	// root all/any with several clauses reads better with a leading noun
	if !strings.HasPrefix(out, "any ") {
		switch a.Exposure.Op {
		case domain.OpAll, domain.OpAny:
			if !strings.HasPrefix(out, "all of") && !strings.HasPrefix(out, "any of") && !strings.HasPrefix(out, "(") {
				out = "environments where " + out
			}
		}
	}
	return out
}

func overlapPhrase(a *domain.Applicability) string {
	if a == nil || a.Overlap == nil {
		return ""
	}
	// when the overlap scopes the same resource kind as the exposure, the
	// bare clauses read better than repeating the noun ("Safe when
	// `spec.x` is set", not "Safe when Certificates where `spec.x` is set")
	if a.Overlap.Op == domain.OpResource && a.Exposure.Op == domain.OpResource && a.Overlap.Kind == a.Exposure.Kind && len(a.Overlap.Of) > 0 {
		var parts []string
		for _, o := range a.Overlap.Of {
			parts = append(parts, condEnglish(o, false))
		}
		return strings.Join(parts, " and ")
	}
	return condEnglish(*a.Overlap, true)
}

// changeSentence: "the default of the Certificate field `spec.x` changed from `Never` to `Always`".
func changeSentence(a domain.SemanticAssertion) string {
	sub := subjectPhrase(a.Subject)
	ch := a.Change
	if ch == nil {
		return ""
	}
	var val string
	switch {
	case ch.Before != nil && ch.After != nil:
		val = fmt.Sprintf(" from `%s` to `%s`", cleanVal(*ch.Before), cleanVal(*ch.After))
	case ch.After != nil:
		val = fmt.Sprintf(" to `%s`", cleanVal(*ch.After))
	case ch.Before != nil:
		val = fmt.Sprintf(" (was `%s`)", cleanVal(*ch.Before))
	}
	w := changeWord(ch.Type)
	var out string
	switch ch.Type {
	case domain.ChangeKindDefaultChanged:
		out = fmt.Sprintf("the default of %s changed%s", sub, val)
	case domain.ChangeKindAdded:
		out = fmt.Sprintf("%s %s%s", sub, w, val)
	case domain.ChangeKindRemoved, domain.ChangeKindDeprecated, domain.ChangeKindRenamed:
		out = fmt.Sprintf("%s %s", sub, w)
		if ch.ReplacedBy != nil {
			out += fmt.Sprintf(" (its replacement is %s)", subjectPhrase(ch.ReplacedBy))
		}
	case domain.ChangeKindValidationTightened:
		out = fmt.Sprintf("%s has stricter validation now", sub)
	case domain.ChangeKindRequirementChanged:
		out = fmt.Sprintf("the requirement on %s changed%s", sub, val)
	default:
		out = fmt.Sprintf("%s: %s%s", sub, w, val)
	}
	return out
}

// consequenceSentence: kind + statement + severity, as one sentence.
func consequenceSentence(c *domain.Consequence) string {
	if c == nil {
		return ""
	}
	out := kindWord(c.Kind)
	if c.Statement != "" {
		out += " — " + strings.TrimSuffix(c.Statement, ".")
	}
	if c.Severity != "" {
		out += " (severity: " + severityWord(c.Severity) + ")"
	}
	return out
}

// aspectSentence is the one-sentence answer a proposal gives on one aspect.
func aspectSentence(a domain.SemanticAssertion, x domain.Aspect) string {
	switch x {
	case domain.AspectSubject:
		return subjectPhrase(a.Subject)
	case domain.AspectChange:
		s := changeSentence(a)
		if s == "" {
			return "no change stated"
		}
		return s
	case domain.AspectApplicability:
		out := "Exposed: " + exposurePhrase(a.Applicability)
		if ov := overlapPhrase(a.Applicability); ov != "" {
			out += ". Shielded when " + ov
		}
		return out
	case domain.AspectConsequence:
		s := consequenceSentence(a.Consequence)
		if s == "" {
			return "no consequence stated"
		}
		return strings.ToUpper(s[:1]) + s[1:]
	}
	return ""
}

// --- the card's question, answer and reasons ------------------------------------------

// plainQuestion composes the one-sentence plain-English question from the
// question type and the proposed assertion.
func plainQuestion(rc *knowledge.ReviewContext) string {
	it := rc.Item
	head := string(it.Product) + " " + it.Release
	a := it.Proposed
	switch it.QuestionType {
	case domain.QuestionConsequence:
		who := exposurePhrase(a.Applicability)
		if who == "" {
			who = "environments using " + subjectPhrase(a.Subject)
		}
		if !strings.HasPrefix(who, "environments") {
			// noun phrase like "Certificates where …" — use directly
			return fmt.Sprintf("Does %s change what happens to %s — and does that require action?", head, who)
		}
		return fmt.Sprintf("Does %s change what happens to %s — and does that require action?", head, who)
	case domain.QuestionEvidenceSufficiency:
		return fmt.Sprintf("Is the %s release note specific enough to record what actually changed?", head)
	case domain.QuestionSemanticMapping:
		return fmt.Sprintf("What exactly is %s in %s — and how did it change?", subjectPhrase(a.Subject), head)
	case domain.QuestionApplicability:
		return fmt.Sprintf("Who exactly is exposed to the %s change to %s?", head, subjectPhrase(a.Subject))
	case domain.QuestionClassification:
		return fmt.Sprintf("Is the %s change to %s something to act on, or just to know?", head, subjectPhrase(a.Subject))
	case domain.QuestionRelationship:
		if a.Subject != nil && a.Subject.Name != "" {
			return fmt.Sprintf("Does %s change how it works together with %s — and which environments are exposed?", head, a.Subject.Name)
		}
		return fmt.Sprintf("Does %s change a product relationship — and which environments are exposed?", head)
	}
	return it.Question
}

// suggestedAnswer is the plain-English statement the primary button accepts.
func suggestedAnswer(rc *knowledge.ReviewContext) string {
	it := rc.Item
	if it.QuestionType == domain.QuestionEvidenceSufficiency {
		reason := "the note is too vague"
		for _, p := range rc.Proposals {
			if p.UndeterminedReason != "" {
				reason = p.UndeterminedReason
				break
			}
		}
		return "No — not specific enough to record: " + reason + "."
	}
	a := it.Proposed
	parts := []string{}
	if s := changeSentence(a); s != "" {
		parts = append(parts, strings.ToUpper(s[:1])+s[1:]+".")
	}
	if c := a.Consequence; c != nil {
		if c.Statement != "" {
			parts = append(parts, c.Statement)
		}
		if c.Remediation != "" {
			parts = append(parts, "Fix: "+c.Remediation)
		}
	}
	if len(parts) == 0 && a.Statement != "" {
		parts = append(parts, a.Statement)
	}
	ans := strings.Join(parts, " ")
	if c := a.Consequence; c != nil {
		ans += " → " + classLabel(c.ExposedClass)
		if c.Severity != "" {
			ans += " · " + severityWord(c.Severity)
		}
	}
	return ans
}

// callsOfModels gives the display names behind the item's proposals.
func modelNames(rc *knowledge.ReviewContext) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range rc.Proposals {
		if !seen[p.Provenance.Model] {
			seen[p.Provenance.Model] = true
			out = append(out, p.Provenance.Model)
		}
	}
	return out
}

func shortNames(ms []string) string {
	var out []string
	for _, m := range ms {
		out = append(out, modelShort(m))
	}
	return strings.Join(out, ", ")
}

// consensusAction mirrors itemView.noteConsequenceConsensus (PO-2): separate
// calls agree on an action-eligible consequence, every agreeing call requested
// action-required, nothing refuted.
func consensusAction(rc *knowledge.ReviewContext) bool {
	var agreeing []domain.SemanticProposal
	for _, ag := range rc.Agreement {
		if ag.Aspect != domain.AspectConsequence || len(ag.Groups) != 1 {
			continue
		}
		for _, ids := range ag.Groups {
			for _, id := range ids {
				for _, p := range rc.Proposals {
					if p.ID == id {
						agreeing = append(agreeing, p)
					}
				}
			}
		}
	}
	if len(agreeing) < 2 {
		return false
	}
	for _, p := range agreeing {
		if p.SuggestedClass != domain.ImpactActionRequired || p.Assertion.Consequence == nil || !p.Assertion.Consequence.Kind.ActionEligible() {
			return false
		}
	}
	for _, v := range rc.Validations {
		for _, c := range v.Checks {
			if c.Outcome == domain.OutcomeRefuted {
				return false
			}
		}
	}
	return true
}

// whyYou picks the one strongest reason this needs a human.
func whyYou(rc *knowledge.ReviewContext) string {
	sigs := map[domain.RoutingSignal]bool{}
	for _, s := range rc.Item.Routing.Signals {
		sigs[s] = true
	}
	total := len(rc.Proposals)
	if sigs[domain.SignalConsensusAction] || consensusAction(rc) {
		names := shortNames(modelNames(rc))
		return fmt.Sprintf("High impact: separate model calls (%s) agree this is ACTION REQUIRED — policy sends every such consensus to a human audit. This is that audit.", names)
	}
	if sigs[domain.SignalModelsDisagree] {
		var aspects []string
		maxVariants := 0
		for _, ag := range rc.Agreement {
			if len(ag.Groups) >= 2 {
				if w, ok := aspectWords[ag.Aspect]; ok {
					aspects = append(aspects, w)
				}
				if len(ag.Groups) > maxVariants {
					maxVariants = len(ag.Groups)
				}
			}
		}
		what := "the answer"
		if len(aspects) > 0 {
			what = strings.Join(aspects, " and ")
		}
		return fmt.Sprintf("The models disagree on %s — up to %d different answers from %d calls. A human settles it.", what, maxVariants, total)
	}
	if sigs[domain.SignalAllUndetermined] {
		reason := "no model could pin down what changed"
		for _, p := range rc.Proposals {
			if p.UndeterminedReason != "" {
				reason = p.UndeterminedReason
				break
			}
		}
		return "No model could tell what changed: " + reason + "."
	}
	if sigs[domain.SignalValidationRefuted] {
		return "A deterministic check contradicted part of a proposal — the recorded answer may be wrong."
	}
	if sigs[domain.SignalSingleModel] {
		return "Only one model call has answered this — no second opinion yet."
	}
	if rc.Item.Routing.Priority == domain.PriorityHigh {
		return "Routed high priority: models see real impact for exposed environments."
	}
	return "Models agree; this is a routine confirmation."
}

// confLine is one "why to trust this" bullet; Tone: "" (support), "doubt", "bad".
type confLine struct {
	Text string
	Tone string
}

// whyConfident lists the honest reasons to trust (or doubt) the suggestion.
func whyConfident(rc *knowledge.ReviewContext) []confLine {
	var out []confLine
	asked := rc.Item.Aspects()
	total := len(rc.Proposals)
	support := map[domain.Aspect]int{}
	for _, ag := range rc.Agreement {
		if len(ag.Groups) == 1 {
			for _, ids := range ag.Groups {
				support[ag.Aspect] = len(ids)
			}
		}
	}
	for _, a := range asked {
		w := aspectWords[a]
		variants := 0
		for _, ag := range rc.Agreement {
			if ag.Aspect == a {
				variants = len(ag.Groups)
			}
		}
		switch {
		case variants >= 2:
			out = append(out, confLine{fmt.Sprintf("The %d calls give %d different answers on %s — see below.", total, variants, w), "doubt"})
		case support[a] == total && total >= 2:
			out = append(out, confLine{fmt.Sprintf("All %d calls give the same answer on %s.", total, w), ""})
		case support[a] > 0:
			out = append(out, confLine{fmt.Sprintf("%d of %d calls back this answer on %s; the rest did not commit.", support[a], total, w), "doubt"})
		case total > 0:
			out = append(out, confLine{fmt.Sprintf("No call committed to %s.", w), "bad"})
		}
	}
	scope := domain.ConsensusScopeOf(rc.Proposals)
	if consensusAction(rc) {
		out = append(out, confLine{fmt.Sprintf("The agreeing calls are separate and %s.", strings.ReplaceAll(string(scope), "cross-model", "from different models")), ""})
	}
	// validators, in honest words
	nVal := 0
	for _, v := range rc.Validations {
		for _, c := range v.Checks {
			nVal++
			switch c.Outcome {
			case domain.OutcomeConfirmed:
				out = append(out, confLine{fmt.Sprintf("Checked: %s (%s).", c.Detail, c.Rule), ""})
			case domain.OutcomeRefuted:
				out = append(out, confLine{fmt.Sprintf("Contradicted: %s (%s).", c.Detail, c.Rule), "bad"})
			case domain.OutcomeInconclusive:
				out = append(out, confLine{fmt.Sprintf("Not checkable: %s.", c.Detail), "doubt"})
			}
		}
	}
	if nVal == 0 {
		out = append(out, confLine{"No deterministic validator has checked this item.", "doubt"})
	}
	// evidence breadth
	evs := rc.Candidate.Evidence
	if len(evs) == 1 && len(evs[0].Excerpt) < 120 {
		out = append(out, confLine{"Evidence is thin: one source, a single line.", "doubt"})
	} else if len(evs) >= 2 {
		out = append(out, confLine{fmt.Sprintf("%d independent sources back the statement.", len(evs)), ""})
	}
	return out
}
