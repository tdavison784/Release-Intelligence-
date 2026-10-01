package upgrade

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestDedupeKeys(t *testing.T) {
	a := alnumKey(normalizeNoteText("When configuring sidecar proxies if a hostname exists"))
	b := alnumKey(normalizeNoteText("When configuring sidecar proxies, if a hostname exists."))
	if a != b {
		t.Fatalf("punctuation must not matter: %q vs %q", a, b)
	}
	if headingTitle("Untaint controller: If you enabled the untaint controller") != headingTitle("Untaint controller: The `PILOT_ENABLE...` variable is now automatic") {
		t.Fatal("same heading title must produce the same key")
	}
	if headingTitle("Fix a bug. Then: more") != "" || headingTitle("Single: word") != "" {
		t.Fatal("non-heading prefixes must not produce a title key")
	}
}

func TestSectionLeaf(t *testing.T) {
	for in, want := range map[string]string{
		"":                                   "",
		"Untaint controller":                 "untaintcontroller",
		"Upgrade Notes › Untaint controller": "untaintcontroller",
		"A › B › `Untaint` Controller":       "untaintcontroller",
	} {
		if got := sectionLeaf(in); got != want {
			t.Errorf("sectionLeaf(%q) = %q, want %q", in, got, want)
		}
	}
}

// Items that merely share a "Prefix:" are different changes when they come
// from one source (they are not the same change published twice).
func TestTitlePrefixNeverMergesWithinOneSource(t *testing.T) {
	from := bare("v1.15.0")
	to := bare("v1.16.0").
		note("notes", domain.RoleReleaseNotes, "https://notes/1.16", "Potentially Breaking",
			"Potentially Breaking: ACME metrics label changes", domain.CategoryOther).
		note("notes", domain.RoleReleaseNotes, "https://notes/1.16", "Potentially Breaking",
			"Potentially Breaking: Challenge and Order RBAC restricted to the controller", domain.CategoryOther,
			refs(domain.Reference{Type: "advisory", ID: "GHSA-aaaa-bbbb-cccc"})).
		note("notes", domain.RoleReleaseNotes, "https://notes/1.16", "Potentially Breaking",
			"⚠️ Potentially BREAKING: Log messages changed format", domain.CategoryOther).
		note("notes", domain.RoleReleaseNotes, "https://notes/1.16", "Helm chart",
			"Helm chart: fixed the webhook selector", domain.CategoryBugfix).
		note("notes", domain.RoleReleaseNotes, "https://notes/1.16", "Helm chart",
			"Helm chart: added a podLabels value", domain.CategoryFeature)
	e := mustBuild(t, simpleInput(from, to))
	for _, sub := range []string{"ACME metrics", "Challenge and Order", "Log messages changed", "fixed the webhook", "added a podLabels"} {
		if findTitle(e, sub) == nil {
			t.Errorf("change %q was swallowed by a title merge", sub)
		}
	}
	if n := len(findChanges(e, "section:Potentially Breaking")) + len(findChanges(e, "section:Helm chart")); n != 5 {
		t.Errorf("want 5 distinct changes, got %d", n)
	}
	if acme := findTitle(e, "ACME metrics"); acme != nil && len(acme.References) != 0 {
		t.Errorf("ACME change inherited a reference from an unrelated item: %+v", acme.References)
	}
	if rbac := findTitle(e, "Challenge and Order"); rbac == nil || len(rbac.References) != 1 {
		t.Errorf("RBAC change must keep its own reference: %+v", rbac)
	}
}

// The same change published by two different sources (Istio: structured YAML
// filed under its area, and the upgrade-notes page with a "## Title" section)
// is still one change.
func TestTitleMergesSameChangeFromTwoSources(t *testing.T) {
	const title = "Untaint controller"
	from := bare("v1.22.0")
	to := bare("v1.23.0").
		note("release-notes", domain.RoleReleaseNotes, "https://notes/1.23.yaml", "traffic-management",
			title+": The `PILOT_ENABLE_UNTAINT` variable is now automatic", domain.CategoryOther).
		note("upgrade-notes", domain.RoleUpgradeGuide, "https://istio.io/upgrade-notes", "Upgrade Notes › "+title,
			title+": If you enabled the untaint controller, remove the variable", domain.CategoryMigration, action)
	e := mustBuild(t, simpleInput(from, to))
	cs := e.ChangesWhere(func(c domain.Change) bool { return strings.Contains(c.Title, title) })
	if len(cs) != 1 {
		t.Fatalf("same change from two sources must merge: %d changes: %+v", len(cs), cs)
	}
	if !cs[0].ActionRequired || len(cs[0].Evidence) != 2 {
		t.Errorf("merged change keeps both evidences and the action flag: %+v", cs[0])
	}

	// A second release-notes item with the same prefix must not be pulled
	// into the merged change: the group already holds an item of that source.
	to2 := bare("v1.23.0").
		note("release-notes", domain.RoleReleaseNotes, "https://notes/1.23.yaml", "traffic-management",
			title+": The `PILOT_ENABLE_UNTAINT` variable is now automatic", domain.CategoryOther).
		note("release-notes", domain.RoleReleaseNotes, "https://notes/1.23.yaml", "traffic-management",
			title+": Added a new metric for tainted nodes", domain.CategoryFeature).
		note("upgrade-notes", domain.RoleUpgradeGuide, "https://istio.io/upgrade-notes", "Upgrade Notes › "+title,
			title+": If you enabled the untaint controller, remove the variable", domain.CategoryMigration, action)
	e2 := mustBuild(t, simpleInput(from, to2))
	if n := len(e2.ChangesWhere(func(c domain.Change) bool { return strings.Contains(c.Title, title) })); n != 2 {
		t.Errorf("two release-notes items of one source must stay distinct: got %d changes", n)
	}
}

// Different sources are not enough: the title must also be the heading the
// change is filed under.
func TestTitleMergeRequiresMatchingSectionLeaf(t *testing.T) {
	from := bare("v1.15.0")
	to := bare("v1.16.0").
		note("release-notes", domain.RoleReleaseNotes, "https://notes/1.16", "Other",
			"Helm chart: fixed the webhook selector", domain.CategoryBugfix).
		note("upgrade-notes", domain.RoleUpgradeGuide, "https://upgrade/1.16", "Upgrading",
			"Helm chart: added a podLabels value", domain.CategoryFeature)
	e := mustBuild(t, simpleInput(from, to))
	if findTitle(e, "fixed the webhook") == nil || findTitle(e, "added a podLabels") == nil {
		t.Errorf("unrelated 'Helm chart:' items of two sources merged: %+v", e.Changes)
	}
}
