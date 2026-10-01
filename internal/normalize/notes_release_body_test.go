package normalize

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// A goreleaser/GitHub style release body as published by Argo CD: group
// headings, "* <sha>: <conventional commit subject> (@author)" bullets and
// installation boilerplate that a product definition skips with a rule.
const releaseBody = `## Quick Start

### Non-HA:

` + "```shell\nkubectl apply -n argocd -f https://example.com/install.yaml\n```" + `

## Upgrading

If upgrading from a different minor version, be sure to read the [upgrading](https://argo-cd.readthedocs.io/en/stable/operator-manual/upgrading/overview/) documentation.

## Changelog

### Breaking Changes

* 1a2b3c4d: fix!: remove legacy repo config (@alice)

### Features

* 2b3c4d5e: feat(appset): add foo generator (@bob)
* 3c4d5e6f: feat(cli): add --bar flag (#4567) (@carol)

### Bug fixes

* 4d5e6f70: fix(ui): crash on empty status (@dave)

### Documentation

* 5e6f7081: docs: clarify upgrade notes (@erin)

### Dependency updates

* 6f708192: chore(deps): bump foo from 1 to 2 (@dependabot[bot])

### Other work

* 708192a3: chore: tidy up (@frank)

**Full Changelog**: https://github.com/argoproj/argo-cd/compare/v3.4.0...v3.5.0
`

func TestParseNotesReleaseBody(t *testing.T) {
	rules := []catalog.ClassifyRule{{Section: "^(Quick Start|Upgrading)$", Skip: true}}
	items, evs := parseMD(t, releaseBody, domain.RoleReleaseNotes, rules...)
	if len(items) != 7 || len(evs) != 7 {
		t.Fatalf("items = %d: %q", len(items), texts(items))
	}
	want := []struct {
		cat      domain.Category
		breaking bool
	}{
		{domain.CategoryBugfix, true}, // fix!
		{domain.CategoryFeature, false},
		{domain.CategoryFeature, false},
		{domain.CategoryBugfix, false},
		{domain.CategoryOther, false},
		{domain.CategoryDependency, false},
		{domain.CategoryOther, false},
	}
	for i, w := range want {
		if items[i].Category != w.cat || items[i].Breaking != w.breaking {
			t.Errorf("item %d %q: %s breaking=%v, want %s %v (%s)", i, items[i].Text, items[i].Category, items[i].Breaking, w.cat, w.breaking, items[i].Classification.Rule)
		}
	}
	if items[0].Section != "Changelog › Breaking Changes" || !items[0].Breaking {
		t.Errorf("section/breaking: %+v", items[0])
	}
	// the trailing "Full Changelog" line is not an item
	for _, it := range items {
		if it.Text == "" || it.Section == "" {
			t.Errorf("unexpected item %+v", it)
		}
	}
	// without the skip rule the boilerplate sections show up as prose items
	items, _ = parseMD(t, releaseBody, domain.RoleReleaseNotes)
	if len(items) != 8 {
		t.Errorf("without rule: %d items: %q", len(items), texts(items))
	}
}
