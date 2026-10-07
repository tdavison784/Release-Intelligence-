// Plain-English renderers for the ux-v2 decision card (reviewux lane). Every
// string here is derived deterministically from the review data — no LLM, no
// hand-written per-item prose. Condition trees (DESIGN §1.3), question types,
// routing signals and enum labels all get human wording; machine ids stay in
// the Details fold and the raw record.
package reviewui

import (
	"fmt"
	"sort"
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

func aspectWord(a domain.Aspect) string { return aspectWords[a] }

// classLabel is the impact class in words a human decides with.
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

// questionWord names the question type for the card header.
func questionWord(q domain.QuestionType) string {
	switch q {
	case domain.QuestionConsequence:
		return "Consequence question"
	case domain.QuestionEvidenceSufficiency:
		return "Evidence check"
	case domain.QuestionSemanticMapping:
		return "Mapping question"
	case domain.QuestionApplicability:
		return "Applicability question"
	case domain.QuestionClassification:
		return "Classification question"
	case domain.QuestionRelationship:
		return "Compatibility question"
	case domain.QuestionDuplicate:
		return "Duplicate check"
	}
	return strings.ReplaceAll(string(q), "-", " ")
}

// modelShort turns a raw model id into a display name (claude-opus-5-5 →
// Opus 5.5, glm-5.3-flash → GLM 5.3 Flash); the raw id stays in tooltips
// and the Details fold.
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
		out = append(out, titleCase(t))
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

// titleCase upper-cases the first letter of an unknown model token (codex →
// Codex); numbers and mixed tokens pass through untouched.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] = r[0] - 'a' + 'A'
	}
	return string(r)
}

// avatar is a model's visual token: initial in a circle whose hue comes
// deterministically from the model name.
type avatar struct {
	Name, Initial, Class string
}

// avatarOf gives a model its visual token: the initial of its display name in
// a circle whose hue is a stable hash of the raw model id.
func avatarOf(model string) avatar {
	short := modelShort(model)
	initial := "·"
	for _, r := range short {
		initial = strings.ToUpper(string(r))
		break
	}
	h := 0
	for _, b := range []byte(model) {
		h += int(b)
	}
	return avatar{Name: short, Initial: initial, Class: fmt.Sprintf("av%d", h%6)}
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

// condEnglish renders a condition tree as one plain-English clause. root is
// true at the tree root, where a bare clause list needs no parentheses.
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
	case domain.StateInRange:
		return path + " is within " + c.Range
	case domain.StateOutOfRange:
		return path + " is outside " + c.Range
	case domain.StateExists:
		return path + " exists"
	case domain.StateNone:
		return path + " is absent"
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
	// root all/any or a bare leaf clause reads better with a leading noun
	if !strings.HasPrefix(out, "any ") {
		switch a.Exposure.Op {
		case domain.OpAll, domain.OpAny:
			if !strings.HasPrefix(out, "all of") && !strings.HasPrefix(out, "any of") && !strings.HasPrefix(out, "(") {
				out = "environments where " + out
			}
		case domain.OpResource, domain.OpRef:
			// already carries its noun ("Certificates where …")
		default:
			out = "environments where " + out
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
// question type and the proposed assertion; it needs only the item, so the
// inbox rows can ask it too. Anything it cannot phrase falls back to the
// recorded question.
func plainQuestion(it domain.ReviewItem) string {
	head := string(it.Product) + " " + it.Release
	a := it.Proposed
	switch it.QuestionType {
	case domain.QuestionConsequence:
		who := exposurePhrase(a.Applicability)
		if who == "" {
			who = "environments using " + subjectPhrase(a.Subject)
		}
		return fmt.Sprintf("Does %s change what happens to %s — and does that require action?", head, who)
	case domain.QuestionEvidenceSufficiency:
		return fmt.Sprintf("Is the %s release note specific enough to record what actually changed?", head)
	case domain.QuestionSemanticMapping:
		if a.Subject == nil {
			return fmt.Sprintf("What exactly changed in %s?", head)
		}
		return fmt.Sprintf("What exactly is %s in %s — and how did it change?", subjectPhrase(a.Subject), head)
	case domain.QuestionApplicability:
		if a.Subject == nil {
			return fmt.Sprintf("Who exactly is exposed to the %s change?", head)
		}
		return fmt.Sprintf("Who exactly is exposed to the %s change to %s?", head, subjectPhrase(a.Subject))
	case domain.QuestionClassification:
		if a.Subject == nil {
			return fmt.Sprintf("Is the %s change something to act on, or just to know?", head)
		}
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
// gateReason is the undetermined reason shown on evidence-sufficiency items
// ("" falls back to a generic phrasing).
func suggestedAnswer(it domain.ReviewItem, gateReason string) string {
	if it.QuestionType == domain.QuestionEvidenceSufficiency {
		if gateReason == "" {
			gateReason = "the note is too vague"
		}
		return "No — not specific enough to record: " + gateReason + "."
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

// gateReason picks the first undetermined reason among the proposals.
func gateReason(rc *knowledge.ReviewContext) string {
	for _, p := range rc.Proposals {
		if p.UndeterminedReason != "" {
			return p.UndeterminedReason
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// whyYou picks the one strongest reason this needs a human (the card).
// consensusAction is itemView's own PO-2 computation, passed in so this stays
// a pure function of the context.
func whyYou(rc *knowledge.ReviewContext, consensusAction bool) string {
	sigs := map[domain.RoutingSignal]bool{}
	for _, s := range rc.Item.Routing.Signals {
		sigs[s] = true
	}
	total := len(rc.Proposals)
	if sigs[domain.SignalConsensusAction] || consensusAction {
		return fmt.Sprintf("High impact: separate model calls (%s) agree this is ACTION REQUIRED — policy sends every such consensus to a human audit. This is that audit.", shortNames(modelNames(rc)))
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

// whyYouBrief is the inbox row's why-line: it knows only the item's routing
// signals and the call count, not the full agreement data.
func whyYouBrief(it domain.ReviewItem, calls int) string {
	sigs := map[domain.RoutingSignal]bool{}
	for _, s := range it.Routing.Signals {
		sigs[s] = true
	}
	switch {
	case sigs[domain.SignalConsensusAction]:
		return "High impact: separate model calls agree this is ACTION REQUIRED — this is the human audit policy requires."
	case sigs[domain.SignalModelsDisagree]:
		return "The models disagree — open the item to see the answers side by side."
	case sigs[domain.SignalAllUndetermined]:
		return "No model could tell what changed."
	case sigs[domain.SignalValidationRefuted]:
		return "A deterministic check contradicted part of a proposal."
	case sigs[domain.SignalSingleModel] || calls <= 1:
		return "Only one model call has answered this — no second opinion yet."
	case it.Routing.Priority == domain.PriorityHigh:
		return "High priority: models see real impact for exposed environments."
	}
	return "Models agree; this is a routine confirmation."
}

// whyTone colors the why-line: "" support, "warn", "danger".
func whyTone(it domain.ReviewItem, consensusAction bool) string {
	if consensusAction {
		return "danger"
	}
	for _, s := range it.Routing.Signals {
		if s == domain.SignalModelsDisagree || s == domain.SignalAllUndetermined || s == domain.SignalValidationRefuted {
			return "warn"
		}
	}
	return ""
}

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

// confLine is one "why to trust this" bullet; Tone: "" (support), "doubt", "bad".
type confLine struct {
	Text string
	Tone string
}

// whyConfident lists the honest reasons to trust (or doubt) the suggestion.
func whyConfident(rc *knowledge.ReviewContext, consensusAction bool) []confLine {
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
	if consensusAction {
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

// --- §5: where the models differ --------------------------------------------------------

// differCell is one answer group on a differing aspect: which calls gave it
// (avatars + names), the group's consensus label (PO-1), the answer in one
// sentence, and whether it is the suggested assertion's answer.
type differCell struct {
	Who        string
	Avatars    []avatar
	Text       string
	Class      string // g0..g3 group color
	Consensus  string // "same-model consensus · 2 calls" | "single call"
	AsProposed bool
}

type differRow struct {
	Label string
	Cells []differCell
}

// agreeChip collapses a single-group aspect: "who is exposed — consensus ·
// cross-model · 3 calls". Tone is ok for a real consensus, muted for a
// single call.
type agreeChip struct {
	Label, Text, Tone string
}

// buildDiffer builds §5 from the agreement digests: one row per aspect with
// ≥2 answer groups (biggest group first, then model name), a chip per
// agreeing aspect, and the undetermined reason when no call committed.
func buildDiffer(rc *knowledge.ReviewContext) (rows []differRow, agrees []agreeChip, undetermined string) {
	propsByID := map[string]domain.SemanticProposal{}
	for _, p := range rc.Proposals {
		propsByID[p.ID] = p
	}
	byAspect := map[domain.Aspect]knowledge.AspectAgreement{}
	for _, ag := range rc.Agreement {
		byAspect[ag.Aspect] = ag
	}
	for _, a := range domain.Aspects {
		ag := byAspect[a]
		w, ok := aspectWords[a]
		if !ok {
			continue
		}
		switch {
		case len(ag.Groups) >= 2:
			rows = append(rows, buildDifferRow(rc, a, ag, w))
		case len(ag.Groups) == 1 && len(rc.Proposals) >= 2:
			var g []domain.SemanticProposal
			for _, ids := range ag.Groups {
				for _, id := range ids {
					if p, ok := propsByID[id]; ok {
						g = append(g, p)
					}
				}
			}
			if n := distinctCalls(g); n >= 2 {
				agrees = append(agrees, agreeChip{w, fmt.Sprintf("consensus · %s · %d calls", domain.ConsensusScopeOf(g), n), "ok"})
			} else if n == 1 && len(rc.Proposals) >= 2 {
				agrees = append(agrees, agreeChip{w, "single call", "muted"})
			}
		case len(ag.Undetermined) > 0:
			for _, id := range ag.Undetermined {
				if p, ok := propsByID[id]; ok && p.UndeterminedReason != "" && undetermined == "" {
					undetermined = p.UndeterminedReason
				}
			}
		}
	}
	return rows, agrees, undetermined
}

func buildDifferRow(rc *knowledge.ReviewContext, a domain.Aspect, ag knowledge.AspectAgreement, label string) differRow {
	propsByID := map[string]domain.SemanticProposal{}
	for _, p := range rc.Proposals {
		propsByID[p.ID] = p
	}
	digests := make([]string, 0, len(ag.Groups))
	for d := range ag.Groups {
		digests = append(digests, d)
	}
	// deterministic, reader-friendly order: biggest group first, then model name
	sort.Slice(digests, func(i, j int) bool {
		if len(ag.Groups[digests[i]]) != len(ag.Groups[digests[j]]) {
			return len(ag.Groups[digests[i]]) > len(ag.Groups[digests[j]])
		}
		return firstModel(rc, ag.Groups[digests[i]]) < firstModel(rc, ag.Groups[digests[j]])
	})
	row := differRow{Label: label}
	for gi, d := range digests {
		var group []domain.SemanticProposal
		for _, pid := range ag.Groups[d] {
			if p, ok := propsByID[pid]; ok {
				group = append(group, p)
			}
		}
		if len(group) == 0 {
			continue
		}
		var models []string
		var avatars []avatar
		for _, p := range group {
			models = append(models, modelShort(p.Provenance.Model))
			avatars = append(avatars, avatarOf(p.Provenance.Model))
		}
		consensus := "single call"
		if n := distinctCalls(group); n >= 2 {
			consensus = fmt.Sprintf("%s consensus · %d calls", domain.ConsensusScopeOf(group), n)
		}
		asProposed := rc.Item.Proposed.Has(a) && rc.Item.Proposed.AspectDigest(a) == group[0].Assertion.AspectDigest(a)
		row.Cells = append(row.Cells, differCell{
			Who: strings.Join(models, " + "), Avatars: avatars,
			Text:  aspectSentence(group[0].Assertion, a),
			Class: fmt.Sprintf("g%d", gi%4), Consensus: consensus, AsProposed: asProposed,
		})
	}
	return row
}

// firstModel is the alphabetically-first model name behind a group of
// proposals (a deterministic tie-break for ordering groups).
func firstModel(rc *knowledge.ReviewContext, ids []string) string {
	best := ""
	for _, pid := range ids {
		for _, p := range rc.Proposals {
			if p.ID == pid && (best == "" || p.Provenance.Model < best) {
				best = p.Provenance.Model
			}
		}
	}
	return best
}
