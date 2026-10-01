package normalize

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Fixture tests over real documents (see testdata/): cert-manager website
// release notes and upgrade guides, Istio change/upgrade notes, Argo CD
// upgrade guides.

var locatorRe = regexp.MustCompile(`^L(\d+)-L(\d+)`)

func evidenceByID(evs []domain.Evidence) map[domain.EvidenceID]domain.Evidence {
	m := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range evs {
		m[e.ID] = e
	}
	return m
}

func countCat(items []domain.NoteItem, c domain.Category) int {
	n := 0
	for _, it := range items {
		if it.Category == c {
			n++
		}
	}
	return n
}

// checkEvidenceLines verifies that every item's evidence locator points at
// the right lines of the ORIGINAL document: the first line of the range must
// contain the start of the item's verbatim excerpt.
func checkEvidenceLines(t *testing.T, doc []byte, items []domain.NoteItem, evs []domain.Evidence) {
	t.Helper()
	lines := strings.Split(string(doc), "\n")
	byID := evidenceByID(evs)
	for _, it := range items {
		if len(it.Evidence) == 0 {
			t.Errorf("item %q has no evidence", it.Text)
			continue
		}
		ev, ok := byID[it.Evidence[0]]
		if !ok {
			t.Errorf("evidence %s of %q not returned", it.Evidence[0], it.Text)
			continue
		}
		m := locatorRe.FindStringSubmatch(ev.Locator)
		if m == nil {
			t.Errorf("bad locator %q", ev.Locator)
			continue
		}
		start, _ := strconv.Atoi(m[1])
		end, _ := strconv.Atoi(m[2])
		if start < 1 || end < start || end > len(lines) {
			t.Errorf("locator %q out of range (doc has %d lines)", ev.Locator, len(lines))
			continue
		}
		firstLine := strings.TrimSpace(strings.SplitN(ev.Excerpt, "\n", 2)[0])
		if len(firstLine) > 40 {
			firstLine = firstLine[:40]
		}
		if !strings.Contains(lines[start-1], firstLine) {
			t.Errorf("locator %q of %q: line %d is %q, excerpt starts %q", ev.Locator, it.Text, start, lines[start-1], firstLine)
		}
		if ev.URI != "https://example.com/notes.md" || ev.ContentDigest != "sha256:abc" || ev.Kind != domain.EvidenceDocument || ev.SourceID != "notes" {
			t.Errorf("evidence metadata wrong: %+v", ev)
		}
	}
}

func parseSection(t *testing.T, doc []byte, heading string, role domain.SourceRole) ([]domain.NoteItem, []domain.Evidence) {
	t.Helper()
	sec, err := SelectSection(doc, mustRe(heading))
	if err != nil {
		t.Fatalf("SelectSection(%s): %v", heading, err)
	}
	in := testInput(sec.Body, role)
	in.LineOffset = sec.StartLine - 1
	items, evs, err := ParseNotes(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	return items, evs
}

func TestFixtureCertManager118PatchSection(t *testing.T) {
	doc := fixture(t, "certmanager/release-notes-1.18.md")
	items, evs := parseSection(t, doc, `^v1\.18\.0$`, domain.RoleReleaseNotes)

	if got := countCat(items, domain.CategoryFeature); got != 15 {
		t.Errorf("features = %d, want 15", got)
	}
	if got := countCat(items, domain.CategoryBugfix); got != 14 {
		t.Errorf("bugfixes = %d, want 14", got)
	}
	if got := countCat(items, domain.CategoryOther); got < 8 {
		t.Errorf("other = %d, want >= 8 (Documentation and Other (Cleanup or Flake))", got)
	}
	ids := map[string]bool{}
	for _, it := range items {
		if ids[it.ID] {
			t.Errorf("duplicate ID for %q", it.Text)
		}
		ids[it.ID] = true
		if !strings.HasPrefix(it.Section, "v1.18.0 › ") {
			t.Errorf("section path of %q = %q", it.Text, it.Section)
		}
		if it.Classification.Method != domain.MethodDeclared || it.Classification.Confidence != domain.ConfidenceHigh {
			t.Errorf("%q: %+v", it.Text, it.Classification)
		}
	}
	checkEvidenceLines(t, doc, items, evs)

	// kind headings map to the right categories
	find := func(substr string) domain.NoteItem {
		t.Helper()
		for _, it := range items {
			if strings.Contains(it.Text, substr) {
				return it
			}
		}
		t.Fatalf("no item containing %q", substr)
		return domain.NoteItem{}
	}
	if it := find("Vault issuer"); it.Category != domain.CategoryFeature || it.Section != "v1.18.0 › Feature" {
		t.Errorf("vault item: %+v", it)
	}
	if it := find("Quote nodeSelector"); it.Category != domain.CategoryBugfix {
		t.Errorf("nodeSelector item: %+v", it)
	}
	if it := find("Use `slices.Contains`"); it.Category != domain.CategoryOther || it.Section != "v1.18.0 › Other (Cleanup or Flake)" {
		t.Errorf("slices item: %+v", it)
	}
	// references: PR numbers resolve against the repository, CVE/GHSA ids are found
	cve := find("go-jose")
	var types []string
	for _, r := range cve.References {
		types = append(types, r.Type+":"+r.ID)
	}
	joined := strings.Join(types, " ")
	for _, want := range []string{"cve:CVE-2025-27144", "example/repo#7606"} {
		if !strings.Contains(joined, want) {
			t.Errorf("references of go-jose item %q lack %q", joined, want)
		}
	}
	if g := find("`golang.org/x/crypto` to patch"); len(g.References) == 0 || g.References[0].Type != "ghsa" {
		t.Errorf("GHSA reference missing: %+v", g.References)
	}
}

func TestFixtureCertManager118MajorThemes(t *testing.T) {
	doc := fixture(t, "certmanager/release-notes-1.18.md")
	items, evs := parseSection(t, doc, `^Major Themes$`, domain.RoleReleaseNotes)
	if len(items) != 6 {
		t.Fatalf("Major Themes: want 6 theme items, got %d: %q", len(items), texts(items))
	}
	var breaking []string
	for _, it := range items {
		if it.Breaking {
			breaking = append(breaking, it.Section)
		}
		if len(it.Text) > maxItemText+4 {
			t.Errorf("item not bounded: %d", len(it.Text))
		}
	}
	if len(breaking) != 3 {
		t.Errorf("breaking themes = %v, want PathType Exact, RotationPolicy and RevisionHistoryLimit", breaking)
	}
	var rot domain.NoteItem
	for _, it := range items {
		if strings.Contains(it.Section, "RotationPolicy") {
			rot = it
		}
	}
	if !rot.Breaking || rot.Classification.Method != domain.MethodDeclared || !strings.Contains(rot.Classification.Rule, "marker:breaking") {
		t.Errorf("RotationPolicy theme: %+v", rot)
	}
	if !strings.HasPrefix(rot.Text, "The default value of `Certificate.Spec.PrivateKey.RotationPolicy` is now `Always`: ") {
		t.Errorf("text = %q", rot.Text)
	}
	checkEvidenceLines(t, doc, items, evs)
}

func TestFixtureCertManager118WholeDocument(t *testing.T) {
	doc := fixture(t, "certmanager/release-notes-1.18.md")
	items, evs, err := ParseNotes(testInput(string(doc), domain.RoleReleaseNotes), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 60 {
		t.Errorf("items = %d", len(items))
	}
	ids := map[string]bool{}
	var sections = map[string]int{}
	for _, it := range items {
		if ids[it.ID] {
			t.Errorf("duplicate id %s (%q)", it.ID, it.Text)
		}
		ids[it.ID] = true
		sections[strings.SplitN(it.Section, " › ", 2)[0]]++
	}
	if sections["Community"] != 0 {
		t.Errorf("Community section must be skipped, got %d items", sections["Community"])
	}
	for _, v := range []string{"v1.18.0", "v1.18.1", "v1.18.2", "v1.18.3", "v1.18.4", "v1.18.5", "v1.18.6", "Major Themes"} {
		if sections[v] == 0 {
			t.Errorf("no items under %q (sections: %v)", v, sections)
		}
	}
	checkEvidenceLines(t, doc, items, evs)

	// "Security (MODERATE):" is an item level label
	var sec *domain.NoteItem
	for i := range items {
		if strings.HasPrefix(items[i].Text, "Security (MODERATE):") {
			sec = &items[i]
		}
	}
	if sec == nil || sec.Category != domain.CategorySecurity || sec.Section != "v1.18.5 › Changes by Kind › Bug or Regression" {
		t.Errorf("Security (MODERATE) item: %+v", sec)
	}
}

func TestFixtureCertManager119And120(t *testing.T) {
	doc := fixture(t, "certmanager/release-notes-1.19.md")
	items, evs, _ := ParseNotes(testInput(string(doc), domain.RoleReleaseNotes), nil)
	checkEvidenceLines(t, doc, items, evs)
	var upgrade, potentially *domain.NoteItem
	for i := range items {
		switch {
		case items[i].Section == "Important Upgrade Notes":
			upgrade = &items[i]
		case strings.HasPrefix(items[i].Text, "POTENTIALLY BREAKING:"):
			potentially = &items[i]
		}
	}
	if upgrade == nil || !upgrade.ActionRequired || upgrade.Category != domain.CategoryMigration {
		t.Errorf("Important Upgrade Notes: %+v", upgrade)
	}
	if potentially == nil || !potentially.Breaking || potentially.Section != "Major Themes › Observability, Reliability, and Maintenance" {
		t.Errorf("POTENTIALLY BREAKING bullet: %+v", potentially)
	}
	// 1.19.6: prose theme flagged "⚠️ Potentially breaking change"
	items196, _ := parseSection(t, doc, `^v1\.19\.6$`, domain.RoleReleaseNotes)
	var rbac *domain.NoteItem
	for i := range items196 {
		if strings.HasPrefix(items196[i].Text, "Restrict Challenge and Order RBAC") {
			rbac = &items196[i]
		}
	}
	if rbac == nil || !rbac.Breaking {
		t.Errorf("RBAC theme should be breaking: %+v", rbac)
	}

	// 1.20: patch sections with "### Changelog since vX" + "#### Kind" nesting
	doc20 := fixture(t, "certmanager/release-notes-1.20.md")
	items20, evs20 := parseSection(t, doc20, `^v1\.20\.0$`, domain.RoleReleaseNotes)
	if countCat(items20, domain.CategoryFeature) < 10 || countCat(items20, domain.CategoryBugfix) < 5 {
		t.Errorf("1.20.0: features=%d bugfixes=%d", countCat(items20, domain.CategoryFeature), countCat(items20, domain.CategoryBugfix))
	}
	for _, it := range items20 {
		if it.Category == domain.CategoryFeature && !strings.HasPrefix(it.Section, "v1.20.0 › Changelog since v1.19.0 › Feature") {
			t.Errorf("feature section = %q", it.Section)
		}
	}
	checkEvidenceLines(t, doc20, items20, evs20)
	// the security prose section with an embedded list is ONE item
	items204, _ := parseSection(t, doc20, `^v1\.20\.4$`, domain.RoleReleaseNotes)
	n := 0
	for _, it := range items204 {
		if strings.HasPrefix(it.Section, "v1.20.4 › Security scanners") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("security scanner explanation should be one item, got %d", n)
	}
}

func TestFixtureCertManagerUpgradeGuides(t *testing.T) {
	// 1.17 -> 1.18: numbered list without headings, role upgrade-guide
	doc := fixture(t, "certmanager/upgrading-1.17-1.18.md")
	items, evs, _ := ParseNotes(testInput(string(doc), domain.RoleUpgradeGuide), nil)
	if len(items) != 2 {
		t.Fatalf("1.17->1.18: %q", texts(items))
	}
	for _, it := range items {
		if it.Category != domain.CategoryMigration || !it.ActionRequired {
			t.Errorf("upgrade guide default: %+v", it)
		}
	}
	checkEvidenceLines(t, doc, items, evs)

	// 1.18 -> 1.19: "## Potentially Breaking: ..." headings, "## Next Steps" is skipped
	doc = fixture(t, "certmanager/upgrading-1.18-1.19.md")
	items, evs, _ = ParseNotes(testInput(string(doc), domain.RoleUpgradeGuide), nil)
	if len(items) != 3 {
		t.Fatalf("1.18->1.19: %q", texts(items))
	}
	if items[0].Breaking || !items[1].Breaking || !items[2].Breaking {
		t.Errorf("breaking flags: %v %v %v", items[0].Breaking, items[1].Breaking, items[2].Breaking)
	}
	if !strings.Contains(items[1].Text, "- Update any dashboards") {
		t.Errorf("numbered list should be folded into the prose item: %q", items[1].Text)
	}
	checkEvidenceLines(t, doc, items, evs)
}

func TestFixtureIstioChangeNotes(t *testing.T) {
	doc := fixture(t, "istio/change-notes-1.31.md")
	in := testInput(string(doc), domain.RoleReleaseNotes)
	in.Repository = "istio/istio"
	items, evs, err := ParseNotes(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 118 bullets in the real file (the research doc counted 118 notes files)
	if len(items) != 118 {
		t.Fatalf("items = %d, want 118", len(items))
	}
	want := map[domain.Category]int{
		domain.CategoryBugfix:  78, // **Fixed**
		domain.CategoryFeature: 37, // Added 32 + Improved 3 + Optimized + Enabled
		domain.CategoryRemoval: 1,  // **Removed**
		domain.CategoryOther:   2,  // Updated + Upgraded
	}
	for c, n := range want {
		if got := countCat(items, c); got != n {
			t.Errorf("category %s = %d, want %d", c, got, n)
		}
	}
	areas := map[string]int{}
	for _, it := range items {
		areas[it.Section]++
		if it.Classification.Method != domain.MethodDeclared {
			t.Errorf("%q should be declared: %+v", it.Text, it.Classification)
		}
	}
	for _, a := range []string{"Traffic Management", "Security", "Telemetry", "Installation", "istioctl"} {
		if areas[a] == 0 {
			t.Errorf("no items in area %q: %v", a, areas)
		}
	}
	for _, it := range items {
		if it.Category == domain.CategoryRemoval {
			if !it.ActionRequired || !strings.HasPrefix(it.Text, "Removed the `PILOT_SPAWN_UPSTREAM_SPAN_FOR_GATEWAY` feature flag") || it.Section != "Telemetry" {
				t.Errorf("removal item: %+v", it)
			}
			if !strings.Contains(it.Classification.Rule, "label:**Removed**") {
				t.Errorf("rule = %q", it.Classification.Rule)
			}
		}
	}
	checkEvidenceLines(t, doc, items, evs)

	// the item with an embedded {{< text yaml >}} block keeps its trailing issue link
	for _, it := range items {
		if strings.HasPrefix(it.Text, "Added support for excluding policy configuration from Istio") {
			if strings.Contains(it.Text, "apiVersion") {
				t.Errorf("shortcode code block leaked into the text: %q", it.Text)
			}
			if !strings.Contains(it.Text, "Example usage:") || !strings.Contains(it.Text, "(Issue #60122)") {
				t.Errorf("text = %q", it.Text)
			}
			// the "#60122" in the link text folds into the URL reference
			if len(it.References) != 1 || it.References[0].ID != "istio/istio#60122" || it.References[0].Type != "issue" {
				t.Errorf("references = %+v", it.References)
			}
		}
	}

	// 1.30 has deprecations/promotions in older minors; at least check it parses
	doc30 := fixture(t, "istio/change-notes-1.30.md")
	items30, evs30, _ := ParseNotes(testInput(string(doc30), domain.RoleReleaseNotes), nil)
	// 52 "**Fixed**" bullets; the ones in the Security area that describe
	// vulnerabilities are classified security
	if got := countCat(items30, domain.CategoryBugfix) + countCat(items30, domain.CategorySecurity); got != 52 || countCat(items30, domain.CategorySecurity) != 7 {
		t.Errorf("1.30 fixed = %d (security %d), want 52 (7)", got, countCat(items30, domain.CategorySecurity))
	}
	checkEvidenceLines(t, doc30, items30, evs30)
}

func TestFixtureIstioPatchAndUpgradeNotes(t *testing.T) {
	doc := fixture(t, "istio/announcing-1.30.5.md")
	items, evs, _ := ParseNotes(testInput(string(doc), domain.RoleReleaseNotes), nil)
	if len(items) != 11 || countCat(items, domain.CategoryBugfix) != 11 {
		t.Errorf("1.30.5: %d items, %d bugfix: %q", len(items), countCat(items, domain.CategoryBugfix), texts(items))
	}
	checkEvidenceLines(t, doc, items, evs)

	doc = fixture(t, "istio/upgrade-notes-1.31.md")
	items, evs, _ = ParseNotes(testInput(string(doc), domain.RoleUpgradeGuide), nil)
	if len(items) != 6 {
		t.Fatalf("upgrade notes 1.31: %d items: %q", len(items), texts(items))
	}
	cats := map[string]domain.Category{}
	for _, it := range items {
		if !it.ActionRequired {
			t.Errorf("upgrade-guide item without ActionRequired: %+v", it)
		}
		cats[it.Section] = it.Category
	}
	if cats["Deprecation of GCP infrastructure and hosting"] != domain.CategoryDeprecation {
		t.Errorf("GCP deprecation: %v", cats)
	}
	if cats["`PILOT_SPAWN_UPSTREAM_SPAN_FOR_GATEWAY` feature flag removed"] != domain.CategoryRemoval {
		t.Errorf("flag removed: %v", cats)
	}
	if cats["Default behavior for sending unhealthy endpoints"] != domain.CategoryMigration {
		t.Errorf("default: %v", cats)
	}
	// the section with prose AND a bullet list is one item
	if !strings.Contains(items[0].Text, "Docker images will still be available on Docker Hub") {
		t.Errorf("list should be folded into the item: %q", items[0].Text)
	}
	checkEvidenceLines(t, doc, items, evs)

	doc = fixture(t, "istio/upgrade-notes-1.30.md")
	items, _, _ = ParseNotes(testInput(string(doc), domain.RoleUpgradeGuide), nil)
	if len(items) != 6 {
		t.Errorf("upgrade notes 1.30: %d", len(items))
	}
	if items[0].Section != "Gateway API CRDs must be upgraded to `v1.5.x`" || items[0].Category != domain.CategoryMigration {
		t.Errorf("first 1.30 upgrade note: %+v", items[0])
	}
}

func TestFixtureArgoUpgradeGuides(t *testing.T) {
	doc := fixture(t, "argocd/2.14-3.0.md")
	items, evs, _ := ParseNotes(testInput(string(doc), domain.RoleUpgradeGuide), nil)
	var breaking, other int
	bySection := map[string]domain.NoteItem{}
	for _, it := range items {
		if !it.ActionRequired {
			t.Errorf("upgrade-guide item without ActionRequired: %+v", it)
		}
		if strings.HasPrefix(it.Section, "v2.14 to 3.0 › Breaking Changes › ") {
			breaking++
			if !it.Breaking {
				t.Errorf("item under Breaking Changes is not breaking: %q", it.Section)
			}
		} else {
			other++
			if it.Breaking {
				t.Errorf("item outside Breaking Changes is breaking: %q", it.Section)
			}
		}
		bySection[strings.TrimPrefix(it.Section, "v2.14 to 3.0 › ")] = it
	}
	if breaking != 9 || other != 6 {
		t.Errorf("breaking items = %d (want 9), other = %d (want 6): %v", breaking, other, sectionsOf(items))
	}
	if countCat(items, domain.CategoryMigration) < 5 {
		t.Errorf("migration items = %d", countCat(items, domain.CategoryMigration))
	}
	if it := bySection["Breaking Changes › Removal of `argocd_app_sync_status`, `argocd_app_health_status` and `argocd_app_created_time` Metrics"]; it.Category != domain.CategoryRemoval {
		t.Errorf("metrics removal: %+v", it)
	}
	// ### item with #### Detection / #### Remediation / ##### sub-sections: one item
	it := bySection["Breaking Changes › Fine-Grained RBAC for application `update` and `delete` sub-resources"]
	if it.Text == "" || !strings.HasPrefix(it.Text, "Fine-Grained RBAC for application `update` and `delete` sub-resources: The default behavior") {
		t.Errorf("fine-grained RBAC: %+v", it)
	}
	it = bySection["Breaking Changes › Use Annotation-Based Tracking by Default"]
	if !strings.Contains(it.Text, "Detection:") || !strings.Contains(it.Text, "Remediation:") && len(it.Text) < maxItemText {
		t.Errorf("sub-sections should be folded in: %q", it.Text)
	}
	if h := bySection["Breaking Changes › Upgraded Helm version with breaking changes"]; h.Category != domain.CategoryDependency || !h.Breaking {
		t.Errorf("helm upgrade: %+v", h)
	}
	checkEvidenceLines(t, doc, items, evs)

	doc = fixture(t, "argocd/3.4-3.5.md")
	items, evs, _ = ParseNotes(testInput(string(doc), domain.RoleUpgradeGuide), nil)
	if countCat(items, domain.CategoryDeprecation) != 4 || countCat(items, domain.CategoryDependency) != 2 {
		t.Errorf("3.4->3.5: deprecations=%d dependencies=%d: %v", countCat(items, domain.CategoryDeprecation), countCat(items, domain.CategoryDependency), sectionsOf(items))
	}
	nBreaking := 0
	for _, it := range items {
		if it.Breaking {
			nBreaking++
		}
	}
	if nBreaking != 3 {
		t.Errorf("breaking = %d, want 3", nBreaking)
	}
	checkEvidenceLines(t, doc, items, evs)
}

func sectionsOf(items []domain.NoteItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, string(it.Category)+" | "+it.Section)
	}
	return out
}

func TestFixtureArgoGitLog(t *testing.T) {
	doc := fixture(t, "argocd/git-log.md")
	in := testInput(string(doc), domain.RoleChangelog)
	in.Repository = "argoproj/argo-cd"
	items, evs, err := ParseNotes(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 15 {
		t.Fatalf("items = %d: %q", len(items), texts(items))
	}
	want := map[domain.Category]int{
		domain.CategoryFeature:     3,
		domain.CategoryBugfix:      2 + 0,
		domain.CategoryDependency:  3,
		domain.CategoryDeprecation: 1,
	}
	// feat, feat!, feat+BREAKING CHANGE; fix, fix!
	want[domain.CategoryBugfix] = 2
	for c, n := range want {
		if got := countCat(items, c); got != n {
			t.Errorf("category %s = %d, want %d", c, got, n)
		}
	}
	breaking := 0
	for _, it := range items {
		if it.Breaking {
			breaking++
		}
	}
	if breaking != 3 {
		t.Errorf("breaking = %d, want 3 (fix!, feat!, BREAKING CHANGE)", breaking)
	}
	// commit URLs are not references, PR numbers are
	first := items[0]
	if len(first.References) != 1 || first.References[0].ID != "argoproj/argo-cd#21234" || first.References[0].Type != "github-ref" {
		t.Errorf("references = %+v", first.References)
	}
	if first.References[0].URL != "https://github.com/argoproj/argo-cd/issues/21234" {
		t.Errorf("url = %q", first.References[0].URL)
	}
	checkEvidenceLines(t, doc, items, evs)
}
