package upgrade

// Routine-maintenance classification: a deterministic detector that marks
// release-note changes as routine maintenance — dependency version bumps,
// UI-only work, CI/docs/test/build churn and observability-metric renames.
// Routine changes stay in the edge (facts are never dropped), but they leave
// the default breaking/action/migration narrative of the rendered brief and
// are summarised as a count + breakdown instead.
//
// Safety property (recall preservation): anything plausibly upgrade-relevant
// is NOT routine. The carve-outs below run before every pattern:
//
//   - breaking changes;
//   - security items — category security (upstream section, product rule or
//     keyword classification), a cited CVE/GHSA id, or vulnerability wording,
//     so "Bump golang.org/x/crypto to fix GHSA-…" stays a security change;
//   - items that state an operator directive ("you must", "before upgrading",
//     "manual migration"), even when a bump pattern also matches;
//   - items that remove or disable a feature flag (capability removal, not
//     churn);
//   - conventional-commit housekeeping on a significant category ("chore:
//     deprecate X" is upgrade work that happens to wear a chore label);
//   - computed diffs (values, CRDs, images, compatibility, advisories,
//     lifecycle) are never routine: only note-derived changes are eligible.
//
// The detector is generic: no product names, no LLM. When unsure, an item
// stays non-routine — a missed routine item only costs brief precision, a
// wrongly-routine critical item costs trust and recall.

import (
	"regexp"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// noteProducer is the provenance producer of note-derived Changes (the value
// of normalize.ProducerNotes). A literal, so that upgrade keeps its documented
// dependency set (domain + catalog only); routine_test.go asserts the two
// never drift apart.
const noteProducer = "normalize.notes@v1"

// Routine kinds recorded in Change.RoutineKind.
const (
	RoutineDependency   = "dependency"   // version bumps of third-party components and build inputs
	RoutineUI           = "ui"           // UI-only improvements, fixes and reverts
	RoutineHousekeeping = "housekeeping" // CI/docs/test/build churn, image-build bumps, release tooling
	RoutineMetrics      = "metrics"      // metric renames, additions, deprecations
)

// routineKindLabels labels the kinds in the routine summary, in display order.
var routineKindLabels = []struct {
	kind, one, many string
}{
	{RoutineDependency, "dependency bump", "dependency bumps"},
	{RoutineUI, "UI update", "UI updates"},
	{RoutineHousekeeping, "UI/docs/CI churn", "UI/docs/CI churn"},
	{RoutineMetrics, "metrics rename", "metrics renames"},
}

var (
	// security identifiers and wording (searched in title, detail and
	// reference ids — a CVE may only be cited in a link target)
	routineSecIDRe   = regexp.MustCompile(`(?i)\bCVE-\d{4}-\d{4,}\b|\bGHSA-[a-z0-9]{4}-[a-z0-9]{4,5}-[a-z0-9]{3,5}\b`)
	routineSecWordRe = regexp.MustCompile(`(?i)\b(?:vulnerab\w*|exploit|advisories?)\b|\bsecurity (?:fix(?:es)?|patch(?:es)?|advisory|advisories|release)\b|(?:address|fix(?:es|ed)?|fixing)\s+(?:a\s+)?security\b`)
	// an operator directive in the item itself ("you must", "action required",
	// "before upgrading") — the phrasing the keyword classifier treats as
	// action-required
	routineDirectiveRe = regexp.MustCompile(`(?i)\b(?:you|users?|operators?|admins?|administrators?)\s+(?:must|need(?:s)?\s+to|(?:have|has)\s+to)\b|` +
		`\bmust\s+be\b|\b(?:is|are)\s+now\s+required\b|\baction\s+(?:is\s+)?required\b|` +
		`\bmanual(?:ly)?\s+(?:steps?|interventions?|migrations?)\b|\bmigrate\s+(?:your|existing)\b|` +
		`\bbefore\s+(?:you\s+)?upgrad(?:e|ing)\b`)
	// a removed/disabled feature flag is a capability change, not churn
	routineFlagRe = regexp.MustCompile(`(?i)\bfeature[ -]?flags?\b`)
)

// routineMetricName is a Prometheus-style metric identifier: lowercase, at
// least three underscore-separated segments; `<placeholders>` inside segment
// names are allowed ("envoy_cilium_policymap_<node-ip>_<node-id>_version").
const routineMetricName = `[a-z][a-z0-9]*(?:_[a-z0-9<>!=:.\[\]-]+){2,}`

var (
	// "`old_metric` -> `new_metric`" (a listed rename)
	routineRenameRe = regexp.MustCompile(`^[` + "`" + `"']?(` + routineMetricName + `)[` + "`" + `"']?\s*(?:->|→|=>)\s*[` + "`" + `"']?(` + routineMetricName + `)[` + "`" + `"']?\s*$`)
	// an item that is nothing but a metric name (listed added/renamed metrics)
	routineBareMetricRe = regexp.MustCompile(`^[` + "`" + `"']?(` + routineMetricName + `)[` + "`" + `"']?\s*[.,;:]?\s*$`)
	// "`some_metric_name` is now deprecated"
	routineMetricDepRe = regexp.MustCompile(`^[` + "`" + `"']?(` + routineMetricName + `)[` + "`" + `"']?\s+is\s+now\s+deprecated\b`)
	// a "Changed Metrics:" / "Added Metrics:" / "Removed Metrics:" entry
	routineMetricListRe = regexp.MustCompile(`(?i)^\s*(?:added|new|changed|renamed|removed)\s+metrics?\b\s*[-–—:]`)

	// "ui: …", "UI: …", "ui/pki: …", "ui/activity (enterprise): …"
	routineUIRe = regexp.MustCompile(`(?i)^\s*ui\b\s*[/:]`)

	// dependency bumps: dependabot/renovate shapes ("Bump x from 1.2.3 to
	// 1.4.5", optionally after a short "Label: " prefix) and "deps:" /
	// "chore(deps):" subjects
	routineBumpRe   = regexp.MustCompile(`(?i)^\s*(?:[a-z][a-z0-9 +._/-]{0,40}:\s*)?(?:deps?[(:! ]|bumps?\b)`)
	routineDepsCCRe = regexp.MustCompile(`(?i)^\s*(?:[0-9a-f]{7,40}:?\s+)?[a-z]+\((?:deps|dependenc(?:y|ies))[^)]*\)!?:\s`)
	routineUpdateRe = regexp.MustCompile(`(?i)\b(?:update[sd]?|upgrade[sd]?)\b([^.\n]{0,80}?)\bto\s+v\d[\w.+-]*`)
	// a leading "Label: " prefix of a changelog line ("auth/alicloud: Update
	// plugin to v0.23.1", "database/redis-elasticache: Update plugin to v0.9.1")
	routineLabelPrefixRe = regexp.MustCompile(`(?i)^\s*[a-z][a-z0-9 +._/-]{0,40}:\s*`)

	// conventional-commit housekeeping types (an optional leading commit SHA
	// is allowed, as in goreleaser compare-log entries "a1b2c3d: chore(x): …")
	routineCCRe = regexp.MustCompile(`(?i)^\s*(?:[0-9a-f]{7,40}:?\s+)?(?:docs?|documentation|ci|chore|build|tests?)\s*(?:\([^)]*\))?!?:\s`)
	// "Docs: …", "CI: …" label paragraphs
	routineLabelRe = regexp.MustCompile(`(?i)^\s*(?:docs?|documentation|ci|build|release)\s*[-:]\s`)
	// image-build churn ("Images: Bump Alpine to v3.21", "Images: Remove NGINX
	// v1.21", "Images: Build s390x controller", "Images: Use latest …")
	routineImagesRe = regexp.MustCompile(`(?i)^\s*images?\s*[-:]\s*(?:bump|build|use|update|upgrade|rebuild|remove|drop|add|delete|switch|pin|move)\b`)
	// release bots ("Prepare release v1.2.3", "Prepare v1.2.3")
	routinePrepareRe = regexp.MustCompile(`(?i)^\s*prepare(?:\s+for)?\s+(?:release\b|v?\d)`)

	// "default" in a bump phrase marks a configuration change ("Bump default
	// max_connections to 500"), not a dependency bump
	routineDefaultRe = regexp.MustCompile(`(?i)\bdefaults?\b`)
)

// routineSigCat reports whether the category marks upgrade-relevant content,
// so that a housekeeping commit prefix does not wash it out ("chore: deprecate
// the --redis-compress flag" is a deprecation first).
func routineSigCat(c domain.Category) bool {
	switch c {
	case domain.CategoryDeprecation, domain.CategoryRemoval, domain.CategoryAPI,
		domain.CategoryConfiguration, domain.CategoryHelmValues, domain.CategoryCRDSchema,
		domain.CategoryCompatibility, domain.CategoryMigration:
		return true
	}
	return false
}

// ClassifyRoutine reports whether a change is routine maintenance and, if so,
// its routine kind ("" when not routine). It is deterministic and pure; the
// pipeline applies it to note-derived changes only, and the evaluator applies
// it to recorded edges, so both see identical results.
func ClassifyRoutine(c domain.Change) (routine bool, kind string) {
	// Only release-note items are eligible. Computed diffs (Helm values, CRDs,
	// images, compatibility, advisories, lifecycle tables) describe the actual
	// upgrade surface and are never routine.
	if c.Provenance.Producer != noteProducer {
		return false, ""
	}
	// Carve-outs first: safety before brevity.
	if c.Breaking || c.Category == domain.CategorySecurity {
		return false, ""
	}
	refs := make([]string, 0, 2*len(c.References))
	for _, r := range c.References {
		refs = append(refs, r.Type, r.ID)
	}
	head := c.Title + "\n" + c.Detail
	if routineSecIDRe.MatchString(head) || hasAny(refs, routineSecIDRe) ||
		routineSecWordRe.MatchString(head) || hasAny(refs, routineSecWordRe) {
		return false, ""
	}
	if c.ActionRequired && routineDirectiveRe.MatchString(head) {
		return false, ""
	}
	if routineFlagRe.MatchString(head) {
		return false, ""
	}
	title := strings.TrimLeft(c.Title, " \t*#")
	switch {
	case routineRenameRe.MatchString(title) || routineBareMetricRe.MatchString(title) ||
		routineMetricDepRe.MatchString(head) || routineMetricListRe.MatchString(title):
		return true, RoutineMetrics
	case routineUIRe.MatchString(title):
		return true, RoutineUI
	case routineDep(head):
		return true, RoutineDependency
	case routineCCRe.MatchString(title):
		if routineSigCat(c.Category) {
			return false, ""
		}
		return true, RoutineHousekeeping
	case routineImagesRe.MatchString(title), routineLabelRe.MatchString(title), routinePrepareRe.MatchString(title):
		return true, RoutineHousekeeping
	}
	return false, ""
}

// routineDep reports a dependency-bump statement, and only a bare one: the
// statement must be nothing but the bump (a leading "Label: " prefix of a
// changelog line and reference/version tokens in the detail are allowed; any
// further sentence — a review pointer, a behaviour change — keeps the item in
// the narrative). "This version upgrades Prometheus-Operator to v0.94.0; the
// operator's ClusterRole no longer grants wildcard verbs" is upgrade work, not
// churn.
func routineDep(head string) bool {
	title := head
	if i := strings.IndexByte(head, '\n'); i >= 0 {
		title = head[:i]
	}
	t := strings.TrimLeft(title, " \t*#")
	// shape 1: the whole statement is a leading "Bump …" / "deps: …"
	if routineDepsCCRe.MatchString(t) || routineBumpRe.MatchString(t) {
		if routineDefaultRe.MatchString(t) {
			return false // "Bump default max_connections to 500" is configuration
		}
		detail := ""
		if i := strings.IndexByte(head, '\n'); i >= 0 {
			detail = head[i+1:]
			// noteTitle keeps the whole text as detail when it differs from the
			// title only by punctuation/refs (or the title was truncated) — the
			// restatement is not extra content
			if strings.HasPrefix(detail, title) {
				detail = detail[len(title):]
			} else if base := strings.TrimSuffix(title, "…"); strings.HasPrefix(detail, base) {
				detail = detail[len(base):]
			}
		}
		return routineOnlyRefs(detail)
	}
	// shape 2: "Update/upgrade <dep> to vN", possibly mid-sentence; the
	// remainder (after the leading label) must carry no further content
	m := routineUpdateRe.FindStringSubmatchIndex(head)
	if m == nil || routineDefaultRe.MatchString(head[m[2]:m[3]]) {
		return false
	}
	rest := head[:m[0]] + head[m[1]:]
	rest = routineLabelPrefixRe.ReplaceAllString(rest, "")
	return routineOnlyRefs(rest)
}

// routineOnlyRefs reports whether s carries no content beyond reference
// tokens ("#22300", "(GH-42)"), version tokens, commit/digest hex and
// punctuation.
func routineOnlyRefs(s string) bool {
	s = refsRe.ReplaceAllString(s, " ")
	s = versionRe.ReplaceAllString(s, " ")
	// hex-shaped tokens that contain a digit are commit SHAs or digests;
	// all-letter hex words ("defaced") are left alone
	s = hexishRe.ReplaceAllStringFunc(s, func(tok string) string {
		if strings.IndexFunc(tok, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0 {
			return " "
		}
		return tok
	})
	return !wordRe.MatchString(s)
}

var (
	refsRe    = regexp.MustCompile(`\(?#[0-9]+\)?|\b(?:fix(?:es|ed)?|closes?|refs?|see)\s+#[0-9]+\b|\b(?:GH|PR)-[0-9]+\b|\bsha(?:1|256|512):[0-9a-f]+\b`)
	versionRe = regexp.MustCompile(`\bv?[0-9]+(?:\.[0-9]+)+(?:\+[a-z0-9]+)?\b`)
	hexishRe  = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)
	wordRe    = regexp.MustCompile(`[A-Za-z]{2,}`)
)

func hasAny(xs []string, re *regexp.Regexp) bool {
	for _, x := range xs {
		if re.MatchString(x) {
			return true
		}
	}
	return false
}

// routineSummary aggregates the routine changes of an edge; nil when none.
func routineSummary(cs []domain.Change) *domain.RoutineSummary {
	var s domain.RoutineSummary
	byKind := map[string]int{}
	for _, c := range cs {
		if !c.Routine {
			continue
		}
		s.Count++
		byKind[c.RoutineKind]++
	}
	if s.Count == 0 {
		return nil
	}
	var parts []string
	for _, l := range routineKindLabels {
		if n := byKind[l.kind]; n > 0 {
			parts = append(parts, plural(n, l.one, l.many))
		}
	}
	s.ByKind = byKind
	s.Summary = strings.Join(parts, ", ")
	return &s
}
