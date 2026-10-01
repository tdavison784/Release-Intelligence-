package normalize

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Classification of note items.
//
// Evaluation order (first signal decides Provenance.Method/Confidence, every
// fired signal is listed in Provenance.Rule):
//
//  1. product rules (catalog.ClassifyRule)
//  2. upstream labels: item-level (bold verbs, "Security (HIGH):", conventional
//     commits, breaking markers) and section headings
//  3. keyword heuristics, only when 1 and 2 set neither category nor flag
//  4. role defaults (upgrade-guide => migration + ActionRequired)
//  5. fallback category "other"

type signal struct {
	method domain.Method
	conf   domain.Confidence
	rule   string
}

type verdict struct {
	skip           bool
	category       domain.Category
	breaking       bool
	actionRequired bool
	prov           domain.Provenance
}

// classInput is what the classifier looks at for one item.
type classInput struct {
	role  domain.SourceRole
	path  []string // heading path (YAML notes: [area])
	text  string   // cleaned item text
	raw   string   // markdown-intact text without code blocks
	label string   // markdown-intact first line without list marker (or heading for prose items)

	// release-note YAML only
	yamlKind string // the file's "kind"
	yamlList string // "releaseNotes" | "upgradeNotes" | "securityNotes"
}

type compiledRule struct {
	idx     int
	rule    catalog.ClassifyRule
	section *regexp.Regexp
	text    *regexp.Regexp
}

type classifier struct {
	rules []compiledRule
}

func newClassifier(rules []catalog.ClassifyRule) (*classifier, error) {
	c := &classifier{}
	for i, r := range rules {
		cr := compiledRule{idx: i, rule: r}
		var err error
		if r.Section != "" {
			if cr.section, err = regexp.Compile(r.Section); err != nil {
				return nil, fmt.Errorf("normalize: classify rule %d: section: %w", i, err)
			}
		}
		if r.Text != "" {
			if cr.text, err = regexp.Compile(r.Text); err != nil {
				return nil, fmt.Errorf("normalize: classify rule %d: text: %w", i, err)
			}
		}
		c.rules = append(c.rules, cr)
	}
	return c, nil
}

// match reports whether the rule matches the item; usedText tells whether the
// rule's Text regex took part (making the classification heuristic).
func (r compiledRule) match(path []string, text string) (matched, usedText bool) {
	if r.section == nil && r.text == nil {
		return false, false
	}
	if r.section != nil {
		ok := r.section.MatchString(strings.Join(path, " › "))
		for _, p := range path {
			if ok {
				break
			}
			ok = r.section.MatchString(p)
		}
		if !ok {
			return false, false
		}
	}
	if r.text != nil {
		if !r.text.MatchString(text) {
			return false, false
		}
		return true, true
	}
	return true, false
}

// ---- upstream labels ------------------------------------------------------

type labelHit struct {
	rule     string
	cat      domain.Category
	breaking bool
	action   bool
}

type headingRow struct {
	rule     string
	re       *regexp.Regexp
	not      *regexp.Regexp // exclusion
	cat      domain.Category
	breaking bool
	action   bool
}

func hrow(rule, re string, cat domain.Category, breaking, action bool) headingRow {
	return headingRow{rule: rule, re: regexp.MustCompile(re), cat: cat, breaking: breaking, action: action}
}

func hrowNot(rule, re, not string, cat domain.Category) headingRow {
	r := hrow(rule, re, cat, false, false)
	r.not = regexp.MustCompile(not)
	return r
}

// headingRows is evaluated per heading; the first row with a category decides
// that heading's category, flag rows contribute independently.
var headingRows = []headingRow{
	hrow("section:/breaking/i", `(?i)\bbreaking\b|backwards?[ -]incompatible|incompatible changes?`, "", true, false),
	hrow("section:/deprecat/i", `(?i)deprecat`, domain.CategoryDeprecation, false, false),
	hrow("section:/remov/i", `(?i)remov`, domain.CategoryRemoval, false, false),
	hrow("section:/security|cve/i", `(?i)security|\bCVEs?\b|vulnerab`, domain.CategorySecurity, false, false),
	hrow("section:/api changes/i", `(?i)\bapi (?:changes?|versions?)\b`, domain.CategoryAPI, false, false),
	hrowNot("section:/dependenc|deps/i", `(?i)dependenc|\bdeps\b|\bupgraded\b|\bbump`, `(?i)\b(?:must|should|need(?:s)? to|has to|have to) be upgraded\b`, domain.CategoryDependency),
	hrow("section:/feature|enhancement/i", `(?i)\bfeatures?\b|\benhancements?\b|\badded\b`, domain.CategoryFeature, false, false),
	hrow("section:/bug|fix|regression/i", `(?i)\bbugs?\b|\bbug ?fix|\bfix(?:es|ed)?\b|regression`, domain.CategoryBugfix, false, false),
	hrow("section:/documentation|docs/i", `(?i)\bdocumentation\b|\bdocs?\b`, domain.CategoryOther, false, false),
	hrow("section:/other|cleanup|flake|misc/i", `(?i)\bother\b|clean-?up|\bflake|\bmisc|uncategori[sz]ed|\bchores?\b`, domain.CategoryOther, false, false),
	hrow("section:/action required|upgrade notes/i", `(?i)action[ -](?:required|needed)|\bupgrade notes?\b|\bimportant (?:upgrade )?notes?\b`, domain.CategoryMigration, false, true),
}

// sectionLabels evaluates the heading path from the leaf towards the root. The
// nearest heading carrying a category decides the category, flags accumulate
// over the whole path.
func sectionLabels(path []string) []labelHit {
	var hits []labelHit
	haveCat := false
	seenFlag := map[string]bool{}
	for i := len(path) - 1; i >= 0; i-- {
		h := path[i]
		catTaken := false
		for _, row := range headingRows {
			if !row.re.MatchString(h) || (row.not != nil && row.not.MatchString(h)) {
				continue
			}
			hit := labelHit{rule: row.rule}
			if row.cat != "" && !haveCat && !catTaken {
				hit.cat = row.cat
				catTaken = true
			}
			if row.breaking || row.action {
				if !seenFlag[row.rule] {
					seenFlag[row.rule] = true
					hit.breaking, hit.action = row.breaking, row.action
				}
			}
			if hit.cat != "" || hit.breaking || hit.action {
				hits = append(hits, hit)
			}
		}
		if catTaken {
			haveCat = true
		}
	}
	return hits
}

var (
	boldVerbRe = regexp.MustCompile(`^\s*(?:\*\*|__)\s*([A-Za-z]+)\s*:?\s*(?:\*\*|__)`)
	securityRe = regexp.MustCompile(`(?i)^\s*(?:\*\*|__)?\s*security\s*(?:\((?:critical|high|moderate|medium|low)\))?\s*:`)
	// conventional commit subject, optionally preceded by a commit SHA as in
	// goreleaser changelogs ("* a1b2c3d4: feat(x): ...")
	ccRe      = regexp.MustCompile(`(?i)^\s*(?:\*\*|__)?(?:[0-9a-f]{7,40}:?\s+)?(feat|fix|docs?|style|refactor|perf|tests?|build|ci|chore|revert)(?:\(([^)\s]*)\))?(!)?:\s`)
	ccDepsRe  = regexp.MustCompile(`(?i)^(?:deps?|dependenc(?:y|ies)|deps?-dev|dev-deps?)$`)
	breakCCRe = regexp.MustCompile(`BREAKING[ -]CHANGE`)
	markerRe  = regexp.MustCompile(`(?i)⚠\x{FE0F}?\s*(?:potentially\s+)?breaking|\bpotentially[ -]breaking\b|^\s*(?:\*\*|__)?\s*breaking(?: changes?)?\s*(?:\*\*|__)?\s*:`)
)

type verbInfo struct {
	cat    domain.Category
	action bool
}

var boldVerbs = map[string]verbInfo{
	"added":      {domain.CategoryFeature, false},
	"fixed":      {domain.CategoryBugfix, false},
	"removed":    {domain.CategoryRemoval, true},
	"deprecated": {domain.CategoryDeprecation, false},
	"promoted":   {domain.CategoryFeature, false},
	"improved":   {domain.CategoryFeature, false},
	"updated":    {domain.CategoryOther, false},
	"upgraded":   {domain.CategoryOther, false},
	"enabled":    {domain.CategoryFeature, false},
	"optimized":  {domain.CategoryFeature, false},
	"optimised":  {domain.CategoryFeature, false},
	"introduced": {domain.CategoryFeature, false},
	"changed":    {domain.CategoryOther, false},
}

// itemLabels detects labels written on the item itself.
func itemLabels(ci classInput) []labelHit {
	var hits []labelHit
	label := ci.label
	switch {
	case securityRe.MatchString(label):
		hits = append(hits, labelHit{rule: "label:security", cat: domain.CategorySecurity})
	default:
		if m := boldVerbRe.FindStringSubmatch(label); m != nil {
			if vi, ok := boldVerbs[strings.ToLower(m[1])]; ok {
				hits = append(hits, labelHit{rule: "label:**" + m[1] + "**", cat: vi.cat, action: vi.action})
			}
		}
		if m := ccRe.FindStringSubmatch(label); m != nil && len(hits) == 0 {
			typ := strings.ToLower(m[1])
			scope := m[2]
			bang := m[3] == "!"
			var cat domain.Category
			switch typ {
			case "feat":
				cat = domain.CategoryFeature
			case "fix":
				cat = domain.CategoryBugfix
			default:
				cat = domain.CategoryOther
			}
			rule := "cc:" + typ
			if ccDepsRe.MatchString(scope) {
				cat = domain.CategoryDependency
				rule += "(deps)"
			}
			if bang {
				rule += "!"
			}
			hits = append(hits, labelHit{rule: rule, cat: cat, breaking: bang})
		}
	}
	if breakCCRe.MatchString(ci.raw) {
		hits = append(hits, labelHit{rule: "label:BREAKING CHANGE", breaking: true})
	}
	if markerRe.MatchString(ci.raw) || markerRe.MatchString(label) {
		hits = append(hits, labelHit{rule: "marker:breaking", breaking: true})
	}
	return hits
}

// yamlLabels handles the structured release-note YAML.
func yamlLabels(ci classInput) []labelHit {
	var hits []labelHit
	switch ci.yamlList {
	case "upgradeNotes":
		return []labelHit{{rule: "yaml:upgradeNotes", cat: domain.CategoryMigration, action: true}}
	case "securityNotes":
		return []labelHit{{rule: "yaml:securityNotes", cat: domain.CategorySecurity}}
	}
	kindCat := yamlKindCategory(ci.yamlKind)
	verbHits := itemLabels(ci)
	switch {
	case kindCat == domain.CategorySecurity:
		hits = append(hits, labelHit{rule: "kind:" + ci.yamlKind, cat: kindCat})
		for _, h := range verbHits {
			h.cat = ""
			if h.breaking || h.action {
				hits = append(hits, h)
			}
		}
	default:
		hits = append(hits, verbHits...)
		hasCat := false
		for _, h := range verbHits {
			if h.cat != "" {
				hasCat = true
			}
		}
		if !hasCat && kindCat != "" {
			hits = append(hits, labelHit{rule: "kind:" + ci.yamlKind, cat: kindCat})
		}
	}
	return hits
}

// yamlKindKey lower-cases kind and keeps letters and digits only
// ("bug-fix" => "bugfix").
func yamlKindKey(kind string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		}
		return -1
	}, kind)
}

// yamlKindCategory maps the "kind" of an Istio-style release note file.
func yamlKindCategory(kind string) domain.Category {
	k := yamlKindKey(kind)
	switch k {
	case "feature", "features", "enhancement", "promotion", "improvement":
		return domain.CategoryFeature
	case "bugfix", "bug", "fix", "bugs":
		return domain.CategoryBugfix
	case "securityfix", "security":
		return domain.CategorySecurity
	case "documentation", "docs", "doc", "cleanup", "chore", "other":
		return domain.CategoryOther
	}
	return ""
}

// ---- keyword heuristics ----------------------------------------------------

var (
	kwBreakingRe    = regexp.MustCompile(`(?i)\bbreaking[ -]changes?\b`)
	kwDeprecatedRe  = regexp.MustCompile(`(?i)deprecat`)
	kwRemovedRe     = regexp.MustCompile(`(?i)\b(?:removed|no longer (?:supported|available|served))\b`)
	kwSecurityRe    = regexp.MustCompile(`(?i)\bCVE-\d{4}-\d{4,}\b|\bGHSA-|vulnerab`)
	kwActionRe      = regexp.MustCompile(`(?i)\b(?:must|required|manually|migrat\w*)\b`)
	skipSectionsRe  = regexp.MustCompile(`(?i)^(?:community|contributors?|new contributors|special thanks|thanks(?: to)?|thank you|acknowledg(?:e)?ments?|credits?|maintainers|steering committee|full changelog|next steps|see also|further reading)\b`)
	categoryHeading = regexp.MustCompile(`(?i)^(?:the\s+)?(?:new\s+)?(?:` +
		`features?|enhancements?|improvements?|bug\s*fix(?:es)?|bugs?(?:\s+(?:or|and|&)\s+regressions?)?|fix(?:es|ed)?|regressions?|` +
		`documentation(?:\s+changes?)?|docs|other(?:\s+changes?)?(?:\s*\(.*\))?|cleanups?|uncategori[sz]ed|misc(?:ellaneous)?|chores?|` +
		`security(?:\s+(?:fix(?:es)?|updates?|changes?|notes?|advisor(?:y|ies)))?|breaking(?:\s+changes?)?|` +
		`backwards?[ -]incompatible(?:\s+changes?)?|deprecat(?:ions?|ed)(?:\s+(?:items?|features?|notices?))?|removals?|removed(?:\s+features?)?|` +
		`dependenc(?:y|ies)(?:\s+(?:updates?|changes?))?|api\s+changes?|added|changed|changes(?:\s+by\s+kind)?|what'?s\s+changed|` +
		`upgrade\s+notes?|action\s+required|important\s+(?:upgrade\s+)?notes?|` +
		`behavio(?:u)?ral\s+(?:improvements?|changes?)(?:\s*/\s*fix(?:es)?)?` +
		`)\s*:?$`)
)

// isCategoryHeading reports whether a heading is a plain category name
// (Feature, Bug or Regression, Breaking Changes, ...). Sections containing
// such headings are "containers": their sub-sections hold the items.
func isCategoryHeading(h string) bool {
	return categoryHeading.MatchString(strings.TrimSpace(h))
}

var groupingHeading = regexp.MustCompile(`(?i)^(?:major\s+)?themes?$|^highlights?$|^what'?s\s+new\??$|^overview$|^summary$|^notable\s+changes$|^release\s+highlights$`)

// isGroupingHeading reports whether a heading groups items rather than being
// an item itself ("Major Themes", "Highlights").
func isGroupingHeading(h string) bool {
	return groupingHeading.MatchString(strings.TrimSpace(h))
}

// handleOnlyRe matches items that are nothing but a user handle (contributor
// lists: "- [@alice](https://github.com/alice)").
var handleOnlyRe = regexp.MustCompile("^`?@[A-Za-z0-9][A-Za-z0-9-]*(?:\\[bot\\])?`?[.,;]?$")

func builtinSkip(path []string) bool {
	for _, p := range path {
		if skipSectionsRe.MatchString(p) {
			return true
		}
	}
	return false
}

// ---- main entry -------------------------------------------------------------

func (c *classifier) classify(ci classInput) verdict {
	var v verdict
	var sigs []signal
	var rules []string
	add := func(m domain.Method, conf domain.Confidence, rule string) {
		sigs = append(sigs, signal{m, conf, rule})
		rules = append(rules, rule)
	}

	// 1. product rules
	matchedAny := false
	catSig := -1 // index in sigs of the rule that decided the category
	for _, r := range c.rules {
		ok, usedText := r.match(ci.path, ci.text)
		if !ok {
			continue
		}
		matchedAny = true
		if r.rule.Skip {
			v.skip = true
			return v
		}
		method, conf := domain.MethodDeclared, domain.ConfidenceHigh
		if usedText {
			method, conf = domain.MethodHeuristic, domain.ConfidenceMedium
		}
		fired := false
		if r.rule.Category != "" && v.category == "" {
			v.category = r.rule.Category
			fired = true
			catSig = len(sigs)
		}
		if r.rule.Breaking != nil && *r.rule.Breaking && !v.breaking {
			v.breaking, fired = true, true
		}
		if r.rule.ActionRequired != nil && *r.rule.ActionRequired && !v.actionRequired {
			v.actionRequired, fired = true, true
		}
		if fired {
			add(method, conf, "product:"+strconv.Itoa(r.idx))
		}
	}
	if catSig > 0 {
		// the rule that decided the category determines method and confidence
		sigs[0], sigs[catSig] = sigs[catSig], sigs[0]
		rules[0], rules[catSig] = rules[catSig], rules[0]
	}
	if !matchedAny && (builtinSkip(ci.path) || handleOnlyRe.MatchString(ci.text)) {
		v.skip = true
		return v
	}

	// 2. upstream labels
	var itemHits []labelHit
	if ci.yamlList != "" || ci.yamlKind != "" {
		itemHits = yamlLabels(ci)
	} else {
		itemHits = itemLabels(ci)
	}
	var secHits []labelHit
	if ci.yamlList == "" { // YAML notes are classified by kind/verb, not by area
		secHits = sectionLabels(ci.path)
	}
	var secCat, itemCat domain.Category
	for _, h := range itemHits {
		if h.cat != "" && itemCat == "" {
			itemCat = h.cat
		}
	}
	for _, h := range secHits {
		if h.cat != "" && secCat == "" {
			secCat = h.cat
		}
	}
	var chosen domain.Category
	switch {
	case itemCat == domain.CategorySecurity:
		chosen = itemCat
	case secCat == domain.CategorySecurity && kwSecurityRe.MatchString(ci.text):
		chosen = secCat
	case itemCat != "":
		chosen = itemCat
	default:
		chosen = secCat
	}
	productSetCat := v.category != ""
	if !productSetCat {
		v.category = chosen
	}
	catAttributed := productSetCat
	for _, group := range [][]labelHit{itemHits, secHits} {
		for _, h := range group {
			used := false
			if h.cat != "" && h.cat == chosen && !catAttributed {
				used, catAttributed = true, true
			}
			if h.breaking {
				v.breaking, used = true, true
			}
			if h.action {
				v.actionRequired, used = true, true
			}
			if used {
				add(domain.MethodDeclared, domain.ConfidenceHigh, h.rule)
			}
		}
	}

	// 3. keyword heuristics
	if v.category == "" && !v.breaking && !v.actionRequired {
		txt := ci.text
		var kwRules []string
		conf := domain.ConfidenceLow
		if kwBreakingRe.MatchString(txt) {
			v.breaking = true
			kwRules = append(kwRules, "kw:breaking change")
			conf = domain.ConfidenceMedium
		}
		switch {
		case kwSecurityRe.MatchString(txt):
			v.category = domain.CategorySecurity
			kwRules = append(kwRules, "kw:cve")
			conf = domain.ConfidenceMedium
		case kwDeprecatedRe.MatchString(txt):
			v.category = domain.CategoryDeprecation
			kwRules = append(kwRules, "kw:deprecat")
			conf = domain.ConfidenceMedium
		case kwRemovedRe.MatchString(txt):
			v.category = domain.CategoryRemoval
			kwRules = append(kwRules, "kw:removed")
			conf = domain.ConfidenceMedium
		}
		if ci.role != domain.RoleUpgradeGuide && kwActionRe.MatchString(txt) { // implied by the role otherwise
			v.actionRequired = true
			kwRules = append(kwRules, "kw:must|required|manually|migrat")
		}
		for _, r := range kwRules {
			add(domain.MethodHeuristic, conf, r)
		}
	}

	// 4. role defaults
	switch ci.role {
	case domain.RoleUpgradeGuide:
		changed := false
		if !v.actionRequired {
			v.actionRequired, changed = true, true
		}
		if v.category == "" {
			v.category, changed = domain.CategoryMigration, true
		}
		if changed {
			add(domain.MethodDeclared, domain.ConfidenceHigh, "role:upgrade-guide")
		}
	case domain.RoleSecurity:
		if v.category == "" {
			v.category = domain.CategorySecurity
			add(domain.MethodDeclared, domain.ConfidenceHigh, "role:security")
		}
	}

	// 5. fallback
	if v.category == "" {
		v.category = domain.CategoryOther
		if len(sigs) == 0 {
			add(domain.MethodHeuristic, domain.ConfidenceLow, "fallback:other")
		}
	}

	v.prov = domain.Provenance{
		Method:     sigs[0].method,
		Producer:   ProducerNotes,
		Rule:       strings.Join(rules, ", "),
		Confidence: sigs[0].conf,
	}
	return v
}
