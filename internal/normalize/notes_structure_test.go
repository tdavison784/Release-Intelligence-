package normalize

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestParseNotesTitleAndGroupingHeadings(t *testing.T) {
	// a level-1 title with intro prose does not swallow its sub-sections
	// (Argo CD 3.0 -> 3.1 style: an H2 per change, no category headings)
	md := `# v3.0 to 3.1

Intro text about this upgrade that is not an item.

## Symlink protection in API ` + "`--staticassets`" + ` directory

Symlinks are now rejected.

### Details

More details.

## v1 Actions API Deprecated

The v1 actions API is deprecated.
`
	items, _ := parseMD(t, md, domain.RoleUpgradeGuide)
	got := texts(items)
	want := []string{
		"Symlink protection in API `--staticassets` directory: Symlinks are now rejected. Details: More details.",
		"v1 Actions API Deprecated: The v1 actions API is deprecated.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("items:\n got %q\nwant %q", got, want)
	}
	if items[1].Category != domain.CategoryDeprecation {
		t.Errorf("category = %s", items[1].Category)
	}

	// a single-section document keeps its prose item even at level 1
	items, _ = parseMD(t, "# Only title\n\nSome words here.\n", domain.RoleReleaseNotes)
	if len(items) != 1 || items[0].Text != "Only title: Some words here." {
		t.Errorf("%+v", items)
	}

	// grouping headings with an intro keep one item per theme
	md = "## Major Themes\n\nThis release has these themes.\n\n### One\n\nFirst.\n\n### Two\n\nSecond.\n"
	items, _ = parseMD(t, md, domain.RoleReleaseNotes)
	if got := texts(items); !reflect.DeepEqual(got, []string{"Major Themes: This release has these themes.", "One: First.", "Two: Second."}) {
		t.Errorf("themes: %q", got)
	}
}

func TestParseNotesListStyles(t *testing.T) {
	// list directly after the heading, with a trailing paragraph that is not an item
	items, _ := parseMD(t, "## What's Changed\n\n* feat: a by @u in https://github.com/o/r/pull/1\n* fix: b by @u in https://github.com/o/r/pull/2\n\n**Full Changelog**: https://github.com/o/r/compare/v1...v2\n", domain.RoleChangelog)
	if len(items) != 2 || items[0].Category != domain.CategoryFeature || items[1].Category != domain.CategoryBugfix {
		t.Fatalf("%+v", items)
	}
	if items[0].References[0].Type != "pull-request" || items[0].References[0].ID != "o/r#1" {
		t.Errorf("refs: %+v", items[0].References)
	}

	// lead-in sentence + list: the list items are the items (container "Changes since")
	items, _ = parseMD(t, "## v1.0.0\n\nChanges since `v0.9.0`:\n\n### Feature\n\n- one\n- two\n", domain.RoleReleaseNotes)
	if got := texts(items); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Errorf("%q", got)
	}

	// a code fence inside an item stays part of it (verbatim excerpt) but not of the text
	md := "## Notes\n\n- First item:\n\n  ```yaml\n  key: value\n  ```\n\n  after the code\n- Second item\n"
	items, evs := parseMD(t, md, domain.RoleReleaseNotes)
	if len(items) != 2 || items[0].Text != "First item: after the code" {
		t.Fatalf("%q", texts(items))
	}
	if !strings.Contains(evs[0].Excerpt, "key: value") || evs[0].Locator != "L3-L9 (Notes)" {
		t.Errorf("evidence: %+v", evs[0])
	}

	// a table directly after a list item ends the item; tables are never items
	items, _ = parseMD(t, "## T\n\n- item\n| a | b |\n|---|---|\n| 1 | 2 |\n", domain.RoleReleaseNotes)
	if got := texts(items); !reflect.DeepEqual(got, []string{"item"}) {
		t.Errorf("%q", got)
	}

	// loose and tight items mixed, tabs, deeper nesting
	items, _ = parseMD(t, "## L\n\n- a\n\n\t- nested with tab\n\n- b\n  continued\n    - deep\n- c\n", domain.RoleReleaseNotes)
	if got := texts(items); !reflect.DeepEqual(got, []string{"a - nested with tab", "b continued - deep", "c"}) {
		t.Errorf("%q", got)
	}
}

func TestParseNotesHugoAndComments(t *testing.T) {
	md := `---
title: Notes
---

{{< relnote >}}

<!--
## hidden heading
- hidden item
-->

## Changes

{{< tip >}}
Istio 1.31.0 is officially supported on Kubernetes versions 1.32 to 1.36.
{{< /tip >}}

- visible <!-- inline comment --> item {/* mdx comment */}
{/* BEGIN changelog v1.0.0 */}
- another item
{/* END changelog v1.0.0 */}
`
	items, evs := parseMD(t, md, domain.RoleReleaseNotes)
	want := []string{"visible item", "another item"}
	if got := texts(items); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
	// front matter and comment lines are counted: "- visible" is on line 18
	if evs[0].Locator != "L18-L18 (Changes)" {
		t.Errorf("locator = %q", evs[0].Locator)
	}
}
