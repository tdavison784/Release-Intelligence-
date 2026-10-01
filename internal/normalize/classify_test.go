package normalize

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

const (
	D  = domain.MethodDeclared
	H  = domain.MethodHeuristic
	hi = domain.ConfidenceHigh
	me = domain.ConfidenceMedium
	lo = domain.ConfidenceLow
)

type classWant struct {
	cat        domain.Category
	breaking   bool
	action     bool
	method     domain.Method
	conf       domain.Confidence
	ruleHas    []string // substrings of Provenance.Rule
	ruleHasNot []string
}

func checkClass(t *testing.T, it domain.NoteItem, w classWant) {
	t.Helper()
	if it.Category != w.cat || it.Breaking != w.breaking || it.ActionRequired != w.action {
		t.Errorf("%q: got category=%s breaking=%v action=%v, want %s %v %v (rule %q)", it.Text, it.Category, it.Breaking, it.ActionRequired, w.cat, w.breaking, w.action, it.Classification.Rule)
	}
	if w.method != "" && it.Classification.Method != w.method {
		t.Errorf("%q: method = %s, want %s (rule %q)", it.Text, it.Classification.Method, w.method, it.Classification.Rule)
	}
	if w.conf != "" && it.Classification.Confidence != w.conf {
		t.Errorf("%q: confidence = %s, want %s", it.Text, it.Classification.Confidence, w.conf)
	}
	for _, s := range w.ruleHas {
		if !strings.Contains(it.Classification.Rule, s) {
			t.Errorf("%q: rule %q lacks %q", it.Text, it.Classification.Rule, s)
		}
	}
	for _, s := range w.ruleHasNot {
		if strings.Contains(it.Classification.Rule, s) {
			t.Errorf("%q: rule %q should not contain %q", it.Text, it.Classification.Rule, s)
		}
	}
	if it.Classification.Producer != ProducerNotes {
		t.Errorf("producer = %q", it.Classification.Producer)
	}
	if err := it.Classification.Validate(); err != nil {
		t.Errorf("provenance: %v", err)
	}
}

func TestClassifyItems(t *testing.T) {
	tests := []struct {
		name string
		md   string
		role domain.SourceRole
		want classWant
	}{
		// section headings (declared)
		{"section breaking", "## Breaking Changes\n\n- Dropped thing.\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryOther, breaking: true, method: D, conf: hi, ruleHas: []string{"section:/breaking/i"}}},
		{"section backwards incompatible", "## Backwards-incompatible changes\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryOther, breaking: true, method: D, conf: hi}},
		{"section action required", "## Action Required\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryMigration, action: true, method: D, conf: hi, ruleHas: []string{"action required"}}},
		{"section upgrade notes", "## Important Upgrade Notes\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryMigration, action: true, method: D, conf: hi}},
		{"section deprecations", "## Deprecations\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryDeprecation, method: D, conf: hi, ruleHas: []string{"section:/deprecat/i"}}},
		{"section removed", "## Removed features\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryRemoval, method: D, conf: hi}},
		{"section security", "## Security\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategorySecurity, method: D, conf: hi}},
		{"section features", "## New Features\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryFeature, method: D, conf: hi}},
		{"section enhancements", "## Enhancements\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryFeature, method: D, conf: hi}},
		{"section bug or regression", "## Bug or Regression\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryBugfix, method: D, conf: hi}},
		{"section bug fixes", "## Bug Fixes\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryBugfix, method: D, conf: hi}},
		{"section dependencies", "## Dependency updates\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryDependency, method: D, conf: hi}},
		{"section deps", "## Deps\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryDependency, method: D, conf: hi}},
		{"section api changes", "## API Changes\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryAPI, method: D, conf: hi}},
		{"section documentation", "## Documentation\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryOther, method: D, conf: hi, ruleHas: []string{"documentation"}}},
		{"section other cleanup flake", "## Other (Cleanup or Flake)\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryOther, method: D, conf: hi}},
		{"nearest heading decides the category, flags accumulate", "## Breaking Changes\n\n### Removed\n\n- x\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryRemoval, breaking: true, method: D, conf: hi}},

		// item-level labels
		{"istio Added", "## Area\n\n- **Added** a thing\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryFeature, method: D, conf: hi, ruleHas: []string{"label:**Added**"}}},
		{"istio Fixed", "## Area\n\n- **Fixed** a thing\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryBugfix, method: D, conf: hi}},
		{"istio Removed => removal + action", "## Area\n\n- **Removed** a thing\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryRemoval, action: true, method: D, conf: hi, ruleHas: []string{"label:**Removed**"}}},
		{"istio Deprecated", "## Area\n\n- **Deprecated** a thing\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryDeprecation, method: D, conf: hi}},
		{"istio Promoted", "## Area\n\n- **Promoted** a thing\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryFeature, method: D, conf: hi}},
		{"istio Improved", "## Area\n\n- **Improved** a thing\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryFeature, method: D, conf: hi}},
		{"istio Updated", "## Area\n\n- **Updated** a thing\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryOther, method: D, conf: hi}},
		{"istio Upgraded", "## Area\n\n- **Upgraded** a thing\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryOther, method: D, conf: hi}},
		{"istio Enabled", "## Area\n\n- **Enabled** a thing\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryFeature, method: D, conf: hi}},
		{"verb beats weak section label", "## Bug Fixes\n\n- **Added** a thing\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryFeature, method: D, conf: hi}},
		{"cert-manager Security(HIGH) beats the section", "## Bug or Regression\n\n- Security (HIGH): remove verbs\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategorySecurity, method: D, conf: hi, ruleHas: []string{"label:security"}}},
		{"security section + CVE text", "## Security Update\n\n- **Fixed** CVE-2025-0001 in envoy\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategorySecurity, method: D, conf: hi}},
		{"breaking marker blockquote", "## Themes\n\n### A theme\n\n> ⚠️ Breaking change\n\nWe changed the default.\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryOther, breaking: true, method: D, conf: hi, ruleHas: []string{"marker:breaking"}}},
		{"potentially breaking marker", "## Themes\n\n- POTENTIALLY BREAKING: a label was removed\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryOther, breaking: true, method: D, conf: hi}},
		{"marker keeps the section category", "## Feature\n\n- Added x ⚠️ Breaking change\n", domain.RoleReleaseNotes,
			classWant{cat: domain.CategoryFeature, breaking: true, method: D, conf: hi}},

		// conventional commits
		{"cc feat", "- feat(appset): add foo (#1)\n", domain.RoleChangelog, classWant{cat: domain.CategoryFeature, method: D, conf: hi, ruleHas: []string{"cc:feat"}}},
		{"cc with commit sha prefix", "* a1b2c3d4e5: feat(appset)!: add foo (@user)\n", domain.RoleChangelog, classWant{cat: domain.CategoryFeature, breaking: true, method: D, conf: hi, ruleHas: []string{"cc:feat!"}}},
		{"cc fix", "- fix: something\n", domain.RoleChangelog, classWant{cat: domain.CategoryBugfix, method: D, conf: hi}},
		{"cc feat!", "- feat(api)!: new shape\n", domain.RoleChangelog, classWant{cat: domain.CategoryFeature, breaking: true, method: D, conf: hi, ruleHas: []string{"cc:feat!"}}},
		{"cc fix!", "- fix!: something\n", domain.RoleChangelog, classWant{cat: domain.CategoryBugfix, breaking: true, method: D, conf: hi}},
		{"cc BREAKING CHANGE footer", "- feat: x\n\n  BREAKING CHANGE: y\n", domain.RoleChangelog,
			classWant{cat: domain.CategoryFeature, breaking: true, method: D, conf: hi, ruleHas: []string{"BREAKING CHANGE"}}},
		{"cc chore(deps)", "- chore(deps): bump a\n", domain.RoleChangelog, classWant{cat: domain.CategoryDependency, method: D, conf: hi}},
		{"cc build(deps)", "- build(deps): bump a\n", domain.RoleChangelog, classWant{cat: domain.CategoryDependency, method: D, conf: hi}},
		{"cc fix(deps)", "- fix(deps): bump a\n", domain.RoleChangelog, classWant{cat: domain.CategoryDependency, method: D, conf: hi}},
		{"cc docs", "- docs: words\n", domain.RoleChangelog, classWant{cat: domain.CategoryOther, method: D, conf: hi}},
		{"cc refactor", "- refactor(x): words\n", domain.RoleChangelog, classWant{cat: domain.CategoryOther, method: D, conf: hi}},
		{"cc test", "- test: words\n", domain.RoleChangelog, classWant{cat: domain.CategoryOther, method: D, conf: hi}},
		{"cc ci", "- ci: words\n", domain.RoleChangelog, classWant{cat: domain.CategoryOther, method: D, conf: hi}},
		{"cc chore", "- chore: words\n", domain.RoleChangelog, classWant{cat: domain.CategoryOther, method: D, conf: hi}},
		{"cc perf", "- perf: words\n", domain.RoleChangelog, classWant{cat: domain.CategoryOther, method: D, conf: hi}},
		{"cc build", "- build: words\n", domain.RoleChangelog, classWant{cat: domain.CategoryOther, method: D, conf: hi}},

		// keyword heuristics (only when nothing declared)
		{"kw breaking change", "- This is a breaking change for users\n", domain.RoleChangelog,
			classWant{cat: domain.CategoryOther, breaking: true, method: H, conf: me, ruleHas: []string{"kw:breaking change"}}},
		{"kw deprecat", "- The flag is deprecated and will go\n", domain.RoleChangelog,
			classWant{cat: domain.CategoryDeprecation, method: H, conf: me, ruleHas: []string{"kw:deprecat"}}},
		{"kw removed", "- The old API was removed\n", domain.RoleChangelog,
			classWant{cat: domain.CategoryRemoval, method: H, conf: me, ruleHas: []string{"kw:removed"}}},
		{"kw no longer supported", "- Windows is no longer supported\n", domain.RoleChangelog,
			classWant{cat: domain.CategoryRemoval, method: H, conf: me}},
		{"kw CVE", "- Addresses CVE-2025-1234\n", domain.RoleChangelog,
			classWant{cat: domain.CategorySecurity, method: H, conf: me}},
		{"kw GHSA", "- Addresses GHSA-hcg3-q754-cr77\n", domain.RoleChangelog,
			classWant{cat: domain.CategorySecurity, method: H, conf: me}},
		{"kw vulnerability", "- Fixes a vulnerability\n", domain.RoleChangelog,
			classWant{cat: domain.CategorySecurity, method: H, conf: me}},
		{"kw must => action required (low)", "- Users must set the flag\n", domain.RoleChangelog,
			classWant{cat: domain.CategoryOther, action: true, method: H, conf: lo, ruleHas: []string{"kw:must"}}},
		{"kw migrate", "- You need to migrate your config\n", domain.RoleChangelog,
			classWant{cat: domain.CategoryOther, action: true, method: H, conf: lo}},
		{"kw does not run when a label fired", "## Bug Fixes\n\n- The API was removed\n", domain.RoleChangelog,
			classWant{cat: domain.CategoryBugfix, method: D, conf: hi, ruleHasNot: []string{"kw:"}}},

		// role defaults and fallback
		{"upgrade guide default", "## Some change\n\nThe behaviour is different now.\n", domain.RoleUpgradeGuide,
			classWant{cat: domain.CategoryMigration, action: true, method: D, conf: hi, ruleHas: []string{"role:upgrade-guide"}}},
		{"upgrade guide keeps a more specific category and ActionRequired", "## Deprecations\n\nThe flag is going away.\n", domain.RoleUpgradeGuide,
			classWant{cat: domain.CategoryDeprecation, action: true, method: D, conf: hi, ruleHas: []string{"role:upgrade-guide"}}},
		{"upgrade guide + breaking section", "## Breaking Changes\n\n### Foo changed\n\nbar\n", domain.RoleUpgradeGuide,
			classWant{cat: domain.CategoryMigration, breaking: true, action: true, method: D, conf: hi}},
		{"upgrade guide keyword refines the category", "## Foo\n\nThe flag was removed.\n", domain.RoleUpgradeGuide,
			classWant{cat: domain.CategoryRemoval, action: true, method: H, conf: me, ruleHas: []string{"kw:removed", "role:upgrade-guide"}}},
		{"fallback", "- Something happened\n", domain.RoleChangelog,
			classWant{cat: domain.CategoryOther, method: H, conf: lo, ruleHas: []string{"fallback:other"}}},
		{"security role default", "- Something happened\n", domain.RoleSecurity,
			classWant{cat: domain.CategorySecurity, method: D, conf: hi}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, _ := parseMD(t, tt.md, tt.role)
			if len(items) != 1 {
				t.Fatalf("want 1 item, got %d: %q", len(items), texts(items))
			}
			checkClass(t, items[0], tt.want)
		})
	}
}

func TestClassifyProductRules(t *testing.T) {
	md := `## Feature

- Alpha: something new
- Beta: something else
- Cleanup chores here

## Misc

- Gamma
- Delta
`
	rules := []catalog.ClassifyRule{
		{Section: "^Misc$", Skip: true},
		{Text: "^Alpha", Category: domain.CategoryAPI},
		{Text: "(?i)^Alpha", Category: domain.CategoryConfiguration, Breaking: boolPtr(true)},
		{Section: "Feature", ActionRequired: boolPtr(true)},
		{Text: "chores", Category: domain.CategoryDependency},
	}
	items, _ := parseMD(t, md, domain.RoleReleaseNotes, rules...)
	if len(items) != 3 {
		t.Fatalf("Misc section should be skipped: %q", texts(items))
	}
	// Alpha: first rule with a category decides it; Breaking is OR-ed from a later
	// rule; the Text regex took part => heuristic; rule lists what fired
	a := items[0]
	checkClass(t, a, classWant{cat: domain.CategoryAPI, breaking: true, action: true, method: H, conf: me, ruleHas: []string{"product:1", "product:2", "product:3"}})
	// Beta only matches the section-only rule that sets ActionRequired: declared;
	// the category then comes from the label "Feature"
	b := items[1]
	checkClass(t, b, classWant{cat: domain.CategoryFeature, action: true, method: D, conf: hi, ruleHas: []string{"product:3"}})
	// "chores": text rule decides the category (heuristic)
	c := items[2]
	checkClass(t, c, classWant{cat: domain.CategoryDependency, action: true, method: H, conf: me, ruleHas: []string{"product:4"}})

	// section-only rule that sets the category is declared
	items, _ = parseMD(t, "## Feature\n\n- x\n", domain.RoleReleaseNotes, catalog.ClassifyRule{Section: "Feature", Category: domain.CategoryAPI})
	checkClass(t, items[0], classWant{cat: domain.CategoryAPI, method: D, conf: hi, ruleHas: []string{"product:0"}})

	// a Skip rule drops the item even when an earlier rule matched
	items, _ = parseMD(t, "## Feature\n\n- Alpha\n- Beta\n", domain.RoleReleaseNotes,
		catalog.ClassifyRule{Section: "Feature", Category: domain.CategoryAPI},
		catalog.ClassifyRule{Text: "^Beta", Skip: true})
	if len(items) != 1 || items[0].Text != "Alpha" {
		t.Errorf("skip rule: %q", texts(items))
	}

	// rules see the full heading path, any element of it matches
	items, _ = parseMD(t, "## Parent\n\n### Child\n\n- x\n", domain.RoleReleaseNotes, catalog.ClassifyRule{Section: "^Parent$", Category: domain.CategoryAPI})
	if len(items) != 1 || items[0].Category != domain.CategoryAPI {
		t.Errorf("path element match: %+v", items)
	}
	items, _ = parseMD(t, "## Parent\n\n### Child\n\n- x\n", domain.RoleReleaseNotes, catalog.ClassifyRule{Section: "Parent › Child$", Category: domain.CategoryAPI})
	if len(items) != 1 || items[0].Category != domain.CategoryAPI {
		t.Errorf("joined path match: %+v", items)
	}
}
