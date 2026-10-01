package normalize

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Regression tests for classification defects found in a review of live
// outputs (cert-manager 1.17 / 1.18 / 1.19 release notes, Argo CD commit
// logs, Argo CD upgrade guide). Texts are the live ones, shortened.

type defectCase struct {
	name  string
	md    string
	role  domain.SourceRole
	rules []catalog.ClassifyRule
	want  classWant
	// first, when set, is the rule that must be listed first in the provenance
	// (the decisive signal)
	first string
}

func runDefectCases(t *testing.T, tests []defectCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, _ := parseMD(t, tt.md, tt.role, tt.rules...)
			if len(items) != 1 {
				t.Fatalf("want 1 item, got %d: %q", len(items), texts(items))
			}
			checkClass(t, items[0], tt.want)
			if tt.first != "" {
				if first := strings.SplitN(items[0].Classification.Rule, ", ", 2)[0]; first != tt.first {
					t.Errorf("first rule = %q, want %q (rule %q)", first, tt.first, items[0].Classification.Rule)
				}
			}
		})
	}
}

// ---- 1. explicit CVE / GHSA ids make an item a security item -------------------

func TestClassifySecurityIdentifiers(t *testing.T) {
	runDefectCases(t, []defectCase{
		{name: "cert-manager 1.18: CVE under Bug or Regression",
			md:   "## Bug or Regression\n\n- Upgrade `golang.org/x/net` fixing `CVE-2025-22870`. ([#7619](https://github.com/cert-manager/cert-manager/pull/7619))\n",
			role: domain.RoleReleaseNotes, first: "id:cve",
			want: classWant{cat: domain.CategorySecurity, method: H, conf: hi, ruleHas: []string{"id:cve"}, ruleHasNot: []string{"section:/bug"}}},
		{name: "cert-manager 1.18: GHSA under Bug or Regression",
			md:   "## Bug or Regression\n\n- Bump `golang.org/x/crypto` to patch `GHSA-hcg3-q754-cr77`.\n",
			role: domain.RoleReleaseNotes, first: "id:ghsa",
			want: classWant{cat: domain.CategorySecurity, method: H, conf: hi}},
		{name: "CVE under a dependency section",
			md:   "## Dependency updates\n\n- Bump go-jose to address CVE-2025-27144\n",
			role: domain.RoleReleaseNotes, first: "id:cve",
			want: classWant{cat: domain.CategorySecurity, method: H, conf: hi}},
		{name: "CVE under a feature section",
			md:   "## Feature\n\n- Added a new check, see CVE-2025-1234\n",
			role: domain.RoleReleaseNotes, first: "id:cve",
			want: classWant{cat: domain.CategorySecurity, method: H, conf: hi}},
		{name: "Argo commit: fix: ... CVE-2025-22869",
			md:   "- fix: bump golang.org/x/crypto to resolve CVE-2025-22869 (#21999) ([abc1234](https://github.com/argoproj/argo-cd/commit/abc1234def5678abc1234def5678abc1234def56))\n",
			role: domain.RoleChangelog, first: "id:cve",
			want: classWant{cat: domain.CategorySecurity, method: H, conf: hi, ruleHasNot: []string{"cc:fix"}}},
		{name: "conventional commit with a dependency scope",
			md:   "- fix(deps): update golang.org/x/net to fix CVE-2025-22870 (#22000)\n",
			role: domain.RoleChangelog, first: "id:cve",
			want: classWant{cat: domain.CategorySecurity, method: H, conf: hi}},
		{name: "bold verb Fixed + GHSA",
			md:   "## Area\n\n- **Fixed** an authz bypass, GHSA-qm8v-g4f9-qhjx\n",
			role: domain.RoleReleaseNotes, first: "id:ghsa",
			want: classWant{cat: domain.CategorySecurity, method: H, conf: hi}},
		{name: "both id kinds are listed, the decisive signal first",
			md:   "## Bug Fixes\n\n- Bump x to fix CVE-2025-1111 (GHSA-hcg3-q754-cr77)\n",
			role: domain.RoleReleaseNotes, first: "id:cve",
			want: classWant{cat: domain.CategorySecurity, method: H, conf: hi, ruleHas: []string{"id:cve", "id:ghsa"}}},
		{name: "the id may only be in a link target",
			md:   "## Dependency updates\n\n- Bump `github.com/google/cel-go` to v0.30.0 to fix [a reported vulnerability](https://github.com/advisories/GHSA-gcjh-h69q-9w9g)\n",
			role: domain.RoleReleaseNotes, first: "id:ghsa",
			want: classWant{cat: domain.CategorySecurity, method: H, conf: hi}},
		{name: "the breaking flag is kept",
			md:   "- feat(api)!: new shape, mitigates CVE-2025-1111\n",
			role: domain.RoleChangelog, first: "id:cve",
			want: classWant{cat: domain.CategorySecurity, breaking: true, method: H, conf: hi, ruleHas: []string{"cc:feat!"}}},
		{name: "the action flag of an upgrade guide is kept",
			md:   "## Foo\n\nThe flag was disabled by default to address CVE-2025-1234.\n",
			role: domain.RoleUpgradeGuide,
			want: classWant{cat: domain.CategorySecurity, action: true}},
		{name: "a label that already says security stays declared",
			md:   "## Security Update\n\n- **Fixed** CVE-2025-0001 in envoy\n",
			role: domain.RoleReleaseNotes,
			want: classWant{cat: domain.CategorySecurity, method: D, conf: hi, ruleHasNot: []string{"id:"}}},
		{name: "malformed ids do not count",
			md:   "## Bug Fixes\n\n- Fix the CVE-2025-123 parser and the GHSA lookup\n",
			role: domain.RoleReleaseNotes,
			want: classWant{cat: domain.CategoryBugfix, method: D, conf: hi}},
		{name: "the word vulnerability without an id keeps the label",
			md:   "## Bug Fixes\n\n- Fix a vulnerability scanner false positive\n",
			role: domain.RoleReleaseNotes,
			want: classWant{cat: domain.CategoryBugfix, method: D, conf: hi}},
		// product rules always win
		{name: "product text rule beats the identifier",
			md:    "## Bug Fixes\n\n- Bump x to fix CVE-2025-1111\n",
			role:  domain.RoleReleaseNotes,
			rules: []catalog.ClassifyRule{{Text: "^Bump", Category: domain.CategoryDependency}},
			want:  classWant{cat: domain.CategoryDependency, method: H, conf: me, ruleHas: []string{"product:0"}, ruleHasNot: []string{"id:"}}},
		{name: "product section rule beats the identifier",
			md:    "## Bug Fixes\n\n- Bump x to fix CVE-2025-1111\n",
			role:  domain.RoleReleaseNotes,
			rules: []catalog.ClassifyRule{{Section: "Bug", Category: domain.CategoryAPI}},
			want:  classWant{cat: domain.CategoryAPI, method: D, conf: hi, ruleHasNot: []string{"id:"}}},
		{name: "a product flag rule does not stop the identifier",
			md:    "## Bug Fixes\n\n- Bump x to fix CVE-2025-1111\n",
			role:  domain.RoleReleaseNotes,
			rules: []catalog.ClassifyRule{{Section: "Bug", ActionRequired: boolPtr(true)}},
			want:  classWant{cat: domain.CategorySecurity, action: true, method: H, conf: hi, ruleHas: []string{"id:cve", "product:0"}}},
	})
}

func TestClassifySecurityLabelQualifiers(t *testing.T) {
	for _, label := range []string{
		"Security (HIGH):",
		"SECURITY (low risk):",
		"Security (LOW):",
		"security (Moderate severity):",
		"**Security (MEDIUM):**",
		"**SECURITY (low risk)**:",
		"__Security (critical):__",
		"Security:",
		"Security :",
	} {
		t.Run(label, func(t *testing.T) {
			items, _ := parseMD(t, "## Bug or Regression\n\n- "+label+" Limit maximum PEM size to 6KB\n", domain.RoleReleaseNotes)
			if len(items) != 1 {
				t.Fatalf("items = %q", texts(items))
			}
			checkClass(t, items[0], classWant{cat: domain.CategorySecurity, method: D, conf: hi, ruleHas: []string{"label:security"}})
		})
	}
	// cert-manager 1.17.0
	runDefectCases(t, []defectCase{
		{name: "cert-manager 1.17.0 SECURITY (low risk) without a section label",
			md:   "- SECURITY (low risk): Limit maximum PEM size to 6KB. ([#7400](https://github.com/cert-manager/cert-manager/pull/7400))\n",
			role: domain.RoleChangelog, first: "label:security",
			want: classWant{cat: domain.CategorySecurity, method: D, conf: hi}},
	})
	for _, notLabel := range []string{
		"Security considerations for the webhook",
		"Securityfix: x",
		"Security (a very long qualifier that is certainly not a severity, really not one):",
	} {
		items, _ := parseMD(t, "## Bug or Regression\n\n- "+notLabel+"\n", domain.RoleReleaseNotes)
		if len(items) != 1 || items[0].Category != domain.CategoryBugfix {
			t.Errorf("%q: %+v", notLabel, items)
		}
	}
}

func TestClassifySecurityIdentifierInYAMLNotes(t *testing.T) {
	yml := `apiVersion: release-notes/v2
kind: bug-fix
area: security
releaseNotes:
- |
  **Fixed** an unbounded read in the debug endpoint, see CVE-2026-31838.
`
	items, _, err := ParseReleaseNoteYAML([]DocInput{yamlInput("debug.yaml", yml)}, nil)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%v err=%v", texts(items), err)
	}
	checkClass(t, items[0], classWant{cat: domain.CategorySecurity, method: H, conf: hi, ruleHas: []string{"id:cve"}})
}

// ---- 2. keyword heuristics ---------------------------------------------------------

func TestClassifyFutureRemovalIsNotRemoval(t *testing.T) {
	other := classWant{cat: domain.CategoryOther, method: H, conf: lo}
	runDefectCases(t, []defectCase{
		{name: "cert-manager 1.19: new flag, value may be removed",
			md:   "- The Helm chart has a new (experimental) `global.hostUsers` flag which configures the Pods to use user namespaces. In the future, when Kubernetes 1.32 reaches its end-of-life, the Helm chart value may be removed (or become a no-op) and Pods will be configured to use user-namespaces by default.\n",
			role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryOther, method: H, conf: lo, ruleHasNot: []string{"kw:remov"}}},
		{name: "cert-manager 1.19: promoted to GA, will be removed in a future release",
			md:   "- The `CAInjectorMerging` feature gate was promoted to GA; the gate is now locked and will be removed in a future release.\n",
			role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryDeprecation, method: H, conf: me, ruleHas: []string{"kw:removal-notice"}, ruleHasNot: []string{"kw:removed"}}},
		{name: "will be removed, deprecated",
			md:   "- The `--legacy` flag is deprecated and will be removed in 3.0.\n",
			role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryDeprecation, method: H, conf: me, ruleHas: []string{"kw:deprecat"}}},
		{name: "will be removed, nothing else",
			md: "- The `--legacy` flag will be removed.\n", role: domain.RoleChangelog, want: other},
		{name: "may be removed", md: "- The value may be removed.\n", role: domain.RoleChangelog, want: other},
		{name: "to be removed", md: "- The `--legacy` flag is scheduled to be removed.\n", role: domain.RoleChangelog, want: other},
		{name: "could be removed", md: "- The shim could be removed.\n", role: domain.RoleChangelog, want: other},
		{name: "would be removed", md: "- The shim would be removed.\n", role: domain.RoleChangelog, want: other},
		{name: "might be removed", md: "- The shim might be removed.\n", role: domain.RoleChangelog, want: other},
		{name: "will soon be removed", md: "- The shim will soon be removed.\n", role: domain.RoleChangelog, want: other},
		{name: "will no longer be supported", md: "- Windows nodes will no longer be supported.\n", role: domain.RoleChangelog, want: other},
		{name: "will no longer be available in a future release",
			md:   "- The v1beta1 API will no longer be available in a future release.\n",
			role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryDeprecation, method: H, conf: me, ruleHas: []string{"kw:removal-notice"}}},
		{name: "Argo upgrade guide: can be overridden or removed",
			md:   "### Ignoring all status updates and high churn mutations\n\nArgo CD manifest now contains a default configuration for `resource.customizations.ignoreResourceUpdates` in the `argocd-cm`. The default configurations can be overridden or removed in the configMap to preserve the v2 behavior.\n",
			role: domain.RoleUpgradeGuide,
			want: classWant{cat: domain.CategoryMigration, action: true, method: D, conf: hi, ruleHasNot: []string{"kw:remov"}}},
		// controls: removals that happened are still removals
		{name: "control: was removed", md: "- The old API was removed.\n", role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryRemoval, method: H, conf: me, ruleHas: []string{"kw:removed"}}},
		{name: "control: has been removed", md: "- The `--legacy` flag has been removed in favour of `--modern`.\n", role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryRemoval, method: H, conf: me}},
		{name: "control: no longer supported", md: "- Windows is no longer supported.\n", role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryRemoval, method: H, conf: me}},
		{name: "control: a removal that happened next to a future one", md: "- The `--legacy` flag was removed; `--old` will be removed later.\n", role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryRemoval, method: H, conf: me}},
		{name: "control: the modal belongs to another sentence", md: "- Support may continue. The `--legacy` flag is removed.\n", role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryRemoval, method: H, conf: me}},
	})
}

func TestClassifyRemovalVerb(t *testing.T) {
	removal := classWant{cat: domain.CategoryRemoval, method: H, conf: me}
	runDefectCases(t, []defectCase{
		{name: "cert-manager: Remove deprecated feature gate",
			md:   "- Remove deprecated feature gate `ValidateCAA`. Setting this feature gate is now a no-op. ([#7553](https://github.com/cert-manager/cert-manager/pull/7553))\n",
			role: domain.RoleChangelog, first: "kw:remove-verb", want: removal},
		{name: "Removed deprecated", md: "- Removed the deprecated `--foo` flag\n", role: domain.RoleChangelog, first: "kw:remove-verb", want: removal},
		{name: "Drop", md: "- Drop the deprecated `--foo` flag\n", role: domain.RoleChangelog, first: "kw:remove-verb", want: removal},
		{name: "Dropped", md: "- Dropped support for Kubernetes 1.29\n", role: domain.RoleChangelog, first: "kw:remove-verb", want: removal},
		{name: "label prefix", md: "- Vault: Remove deprecated `token` auth\n", role: domain.RoleChangelog, first: "kw:remove-verb", want: removal},
		{name: "bold verb that is not a known label", md: "- **Remove** deprecated `token` auth\n", role: domain.RoleChangelog, first: "kw:remove-verb", want: removal},
		{name: "weak section is refined", md: "## Other (Cleanup or Flake)\n\n- Remove deprecated feature gate `ValidateCAA`.\n", role: domain.RoleReleaseNotes, first: "kw:remove-verb",
			want: classWant{cat: domain.CategoryRemoval, method: H, conf: me, ruleHas: []string{"section:/other"}}},
		{name: "a deprecation verb is still a deprecation", md: "- Deprecate the v1 actions API (#21607)\n", role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryDeprecation, method: H, conf: me, ruleHas: []string{"kw:deprecat"}}},
		{name: "deprecated, then removed, not starting with the verb", md: "- The `--foo` flag was deprecated in 2.1 and removed in 3.0\n", role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryDeprecation, method: H, conf: me}},
		{name: "Drop-in is not a verb", md: "- Drop-in replacement for the `foo` helper\n", role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryOther, method: H, conf: lo}},
		{name: "Dropped connections is a bug description", md: "- Dropped connections are now retried\n", role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryOther, method: H, conf: lo}},
		{name: "a label still beats the verb", md: "## Bug Fixes\n\n- Remove duplicate calls to the API\n", role: domain.RoleReleaseNotes,
			want: classWant{cat: domain.CategoryBugfix, method: D, conf: hi, ruleHasNot: []string{"kw:"}}},
	})

	for text, want := range map[string]bool{
		"Remove deprecated feature gate": true,
		"Removed foo":                    true,
		"Removes the need for foo":       false,
		"Dropped support for X":          true,
		"**Removed** foo":                true,
		"Vault: Remove foo":              true,
		"feat(api)!: remove foo":         true,
		"a1b2c3d4e5: chore: drop foo":    true,
		"chore: Remove foo":              true,
		"Foo removed":                    false,
		"Do not remove foo":              false,
		"Removal of foo":                 false,
		"Drop-in":                        false,
		"Dropped packets are counted":    false,
		"Some prefix that is much too long to be a label for the item at hand: Remove foo": false,
	} {
		if got := startsWithRemovalVerb(text); got != want {
			t.Errorf("startsWithRemovalVerb(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestClassifyWeakSectionHousekeepingNotRefined(t *testing.T) {
	cleanup := "## v1.19.0\n\n### Other (Cleanup or Flake)\n\n"
	other := classWant{cat: domain.CategoryOther, method: D, conf: hi, ruleHasNot: []string{"kw:"}}
	runDefectCases(t, []defectCase{
		{name: "cert-manager: migrate E2E tests from a deprecated dependency",
			md:   cleanup + "- Vault: Migrate Vault E2E add-on tests from deprecated `vault-client-go` to the new `vault/api` client. ([#8059](https://github.com/cert-manager/cert-manager/pull/8059))\n",
			role: domain.RoleReleaseNotes, want: other},
		{name: "tests", md: cleanup + "- Remove the deprecated test helpers\n", role: domain.RoleReleaseNotes, want: other},
		{name: "CI", md: cleanup + "- Pin CI actions; the deprecated runner image was removed\n", role: domain.RoleReleaseNotes, want: other},
		{name: "lint", md: cleanup + "- Fix lint warnings about removed fields\n", role: domain.RoleReleaseNotes, want: other},
		{name: "makefile", md: cleanup + "- Makefile: remove deprecated target\n", role: domain.RoleReleaseNotes, want: other},
		{name: "tooling", md: cleanup + "- Drop the deprecated release tooling\n", role: domain.RoleReleaseNotes, want: other},
		// controls
		{name: "control: product code is still refined", md: cleanup + "- Remove deprecated feature gate `ValidateCAA`.\n", role: domain.RoleReleaseNotes,
			want: classWant{cat: domain.CategoryRemoval, method: H, conf: me, ruleHas: []string{"kw:remove-verb"}}},
		{name: "control: product deprecation is still refined", md: cleanup + "- The `--legacy` flag is deprecated.\n", role: domain.RoleReleaseNotes,
			want: classWant{cat: domain.CategoryDeprecation, method: H, conf: me}},
		{name: "control: a CVE in test housekeeping is still security", md: cleanup + "- Bump the e2e test image to fix CVE-2025-1111\n", role: domain.RoleReleaseNotes,
			want: classWant{cat: domain.CategorySecurity, method: H, conf: hi}},
		{name: "control: words that merely contain the tokens", md: cleanup + "- Remove the deprecated `contest` and `acid` packages\n", role: domain.RoleReleaseNotes,
			want: classWant{cat: domain.CategoryRemoval, method: H, conf: me}},
	})
}

func TestClassifyActionRequiredIsDirective(t *testing.T) {
	prose := classWant{cat: domain.CategoryOther, method: H, conf: lo}
	directive := classWant{cat: domain.CategoryOther, action: true, method: H, conf: lo}
	runDefectCases(t, []defectCase{
		// ordinary prose is not an instruction
		{name: "a password is required by the keystore", md: "- A password is required by the keystore when the JKS format is selected.\n", role: domain.RoleChangelog, want: prose},
		{name: "manually triggered", md: "- Jobs can be manually triggered from the UI.\n", role: domain.RoleChangelog, want: prose},
		{name: "migrating as a feature", md: "- Add support for migrating Certificates between issuers.\n", role: domain.RoleChangelog, want: prose},
		{name: "required as a field name", md: "- The `required` field is now validated.\n", role: domain.RoleChangelog, want: prose},
		{name: "must in a sentence about the code", md: "- The reconciler must retry on conflict.\n", role: domain.RoleChangelog, want: prose},
		// directive phrasing
		{name: "you must", md: "- You must set `foo` before the first start.\n", role: domain.RoleChangelog,
			want: classWant{cat: domain.CategoryOther, action: true, method: H, conf: lo, ruleHas: []string{"kw:must"}}},
		{name: "users must", md: "- Users must set the flag\n", role: domain.RoleChangelog, want: directive},
		{name: "must be", md: "- The value must be a string.\n", role: domain.RoleChangelog, want: directive},
		{name: "is now required", md: "- The `issuerRef` is now required.\n", role: domain.RoleChangelog, want: directive},
		{name: "action required", md: "- Action required: rotate the key.\n", role: domain.RoleChangelog, want: directive},
		{name: "manual step", md: "- This needs a manual step on every cluster.\n", role: domain.RoleChangelog, want: directive},
		{name: "manual intervention", md: "- Requires manual intervention.\n", role: domain.RoleChangelog, want: directive},
		{name: "manual migration", md: "- Perform a manual migration of the CRDs.\n", role: domain.RoleChangelog, want: directive},
		{name: "migrate your", md: "- Please migrate your configuration to the new format.\n", role: domain.RoleChangelog, want: directive},
		{name: "migrate existing", md: "- Migrate existing Certificates to the new issuer.\n", role: domain.RoleChangelog, want: directive},
		{name: "before upgrading", md: "- Back up the database before upgrading.\n", role: domain.RoleChangelog, want: directive},
		{name: "need to migrate", md: "- You need to migrate your config\n", role: domain.RoleChangelog, want: directive},
		// the role defaults are unchanged
		{name: "upgrade guide prose is still action required", md: "## Some change\n\nA password is required by the keystore.\n", role: domain.RoleUpgradeGuide,
			want: classWant{cat: domain.CategoryMigration, action: true, method: D, conf: hi, ruleHas: []string{"role:upgrade-guide"}, ruleHasNot: []string{"kw:"}}},
	})
}

// ---- helpers --------------------------------------------------------------------------

func TestRemovalPhrases(t *testing.T) {
	tests := []struct {
		text         string
		done, future bool
	}{
		{"nothing here", false, false},
		{"was removed", true, false},
		{"will be removed", false, true},
		{"will then also be removed", false, true},
		{"is going to be removed", false, true},
		{"may be deprecated and removed", false, true},
		{"can be marked as deprecated, renamed or removed", false, true},
		{"can be overridden or removed in the configMap", false, true},
		{"will no longer be supported", false, true},
		{"may no longer be available", false, true},
		{"is no longer supported", true, false},
		{"was deprecated and removed", true, false},
		{"it will be fine. The flag was removed", true, false},
		{"it will be fine; the flag was removed", true, false},
		{"the old flag was removed and the new one will be removed", true, true},
		{"has been removed so that it can be reused", true, false},
	}
	for _, tt := range tests {
		done, future := removalPhrases(tt.text)
		if done != tt.done || future != tt.future {
			t.Errorf("removalPhrases(%q) = %v, %v; want %v, %v", tt.text, done, future, tt.done, tt.future)
		}
	}
}
