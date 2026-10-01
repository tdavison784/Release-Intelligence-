package normalize

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

var testTime = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func testInput(md string, role domain.SourceRole) DocInput {
	return DocInput{
		SourceID:    "notes",
		Role:        role,
		Release:     "1.2.3",
		URI:         "https://example.com/notes.md",
		Digest:      "sha256:abc",
		RetrievedAt: testTime,
		Content:     []byte(md),
		Repository:  "example/repo",
	}
}

func parseMD(t *testing.T, md string, role domain.SourceRole, rules ...catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence) {
	t.Helper()
	items, evs, err := ParseNotes(testInput(md, role), rules)
	if err != nil {
		t.Fatalf("ParseNotes: %v", err)
	}
	return items, evs
}

func texts(items []domain.NoteItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Text)
	}
	return out
}

func boolPtr(b bool) *bool { return &b }

func TestParseNotesSplitsItems(t *testing.T) {
	md := `---
title: Release
---

<!-- a comment -->

## Changes

- first item
- second item
  continues here

  and a second paragraph

  - nested one
  - nested two
* star item
+ plus item
1. numbered item
2) paren item

{{< relnote >}}

| a | b |
|---|---|
| 1 | 2 |

- [ref link][r] and [inline](https://example.com/x "title") and ` + "`code [x](y)`" + ` and **bold**

[r]: https://example.com/r
`
	items, evs := parseMD(t, md, domain.RoleReleaseNotes)
	want := []string{
		"first item",
		"second item continues here and a second paragraph - nested one - nested two",
		"star item",
		"plus item",
		"numbered item",
		"paren item",
		"ref link and inline and `code [x](y)` and bold",
	}
	if got := texts(items); !reflect.DeepEqual(got, want) {
		t.Fatalf("items:\n got %q\nwant %q", got, want)
	}
	if len(evs) != len(items) {
		t.Errorf("want one evidence per item, got %d for %d items", len(evs), len(items))
	}
	for _, it := range items {
		if it.Section != "Changes" || it.Release != "1.2.3" || it.SourceID != "notes" || it.Role != domain.RoleReleaseNotes {
			t.Errorf("bad item metadata: %+v", it)
		}
		if len(it.Evidence) != 1 {
			t.Errorf("item %q evidence = %v", it.Text, it.Evidence)
		}
		if err := it.Classification.Validate(); err != nil {
			t.Errorf("provenance invalid: %v", err)
		}
	}
}

func TestParseNotesEvidenceAndIDs(t *testing.T) {
	md := "## Features\n\n- alpha\n- beta spans\n  two lines\n"
	in := testInput(md, domain.RoleReleaseNotes)
	in.LineOffset = 100
	items, evs, err := ParseNotes(in, nil)
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%d err=%v", len(items), err)
	}
	e0, e1 := evs[0], evs[1]
	if e0.Locator != "L103-L103 (Features)" || e1.Locator != "L104-L105 (Features)" {
		t.Errorf("locators: %q %q", e0.Locator, e1.Locator)
	}
	if e0.Excerpt != "- alpha" || e1.Excerpt != "- beta spans\n  two lines" {
		t.Errorf("excerpts: %q %q", e0.Excerpt, e1.Excerpt)
	}
	want := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.com/notes.md", "L103-L103 (Features)", "- alpha", "sha256:abc", testTime)
	if e0 != want {
		t.Errorf("evidence = %+v, want %+v", e0, want)
	}
	if items[0].Evidence[0] != e0.ID {
		t.Errorf("item evidence id %v != %v", items[0].Evidence[0], e0.ID)
	}
	if wantID := "note-" + domain.ShortHash("1.2.3", "notes", "alpha"); items[0].ID != wantID {
		t.Errorf("id = %q, want %q", items[0].ID, wantID)
	}
	// the ID depends on the release and source, not on the position
	in2 := testInput("## Other\n\n- alpha\n", domain.RoleReleaseNotes)
	items2, _, _ := ParseNotes(in2, nil)
	if items2[0].ID != items[0].ID {
		t.Errorf("ID should not depend on section: %q vs %q", items2[0].ID, items[0].ID)
	}
	in2.Release = "1.2.4"
	items3, _, _ := ParseNotes(in2, nil)
	if items3[0].ID == items[0].ID {
		t.Error("ID should depend on the release")
	}
}

func TestParseNotesProseItems(t *testing.T) {
	long := strings.Repeat("word ", 400)
	md := `## Major Themes

### Theme one

First paragraph with [a link](https://example.com).

Second paragraph.

### Theme two

` + long + `

### Empty theme

### Theme with sub-sections

Intro text.

#### Detection

Run this command.

#### Remediation

Fix it.

- step one
- step two
`
	items, _ := parseMD(t, md, domain.RoleReleaseNotes)
	if len(items) != 3 {
		t.Fatalf("want 3 prose items, got %d: %q", len(items), texts(items))
	}
	if items[0].Text != "Theme one: First paragraph with a link. Second paragraph." {
		t.Errorf("item 0 text = %q", items[0].Text)
	}
	if items[0].Section != "Major Themes › Theme one" {
		t.Errorf("section = %q", items[0].Section)
	}
	if n := len(items[1].Text); n > maxItemText+4 || !strings.HasSuffix(items[1].Text, "…") {
		t.Errorf("long item not bounded: len=%d tail=%q", n, items[1].Text[len(items[1].Text)-10:])
	}
	if !strings.HasPrefix(items[2].Text, "Theme with sub-sections: Intro text. Detection: Run this command. Remediation: Fix it.") {
		t.Errorf("sub-sections should fold into the parent item: %q", items[2].Text)
	}
}

func TestParseNotesSkipsAndBuiltins(t *testing.T) {
	md := `Preamble prose that is not an item.

## Community

Thanks to everybody.

- [@alice](https://github.com/alice)
- [@bob](https://github.com/bob)

## What's Changed

- real change
- another change

## New Contributors

- @carol made their first contribution
`
	items, _ := parseMD(t, md, domain.RoleChangelog)
	if got := texts(items); !reflect.DeepEqual(got, []string{"real change", "another change"}) {
		t.Errorf("items = %q", got)
	}
	// contributor lists outside of a Community section are skipped as well
	items, _ = parseMD(t, "## Changes\n\n- real change\n\n### People\n\n- [`@alice`](https://github.com/alice)\n- @bob\n- @carol[bot]\n", domain.RoleChangelog)
	if got := texts(items); !reflect.DeepEqual(got, []string{"real change"}) {
		t.Errorf("handle-only items: %q", got)
	}
	// a product rule that matches overrides the built-in skip
	items, _ = parseMD(t, md, domain.RoleChangelog, catalog.ClassifyRule{Section: "Community", Category: domain.CategoryOther})
	if len(items) != 3 || !strings.HasPrefix(items[0].Text, "Community: Thanks to everybody.") {
		t.Errorf("product rule should override built-in skip, got %q", texts(items))
	}
}

func TestParseNotesNoHeadings(t *testing.T) {
	items, _ := parseMD(t, "- feat: one\n- fix: two\n", domain.RoleChangelog)
	if len(items) != 2 || items[0].Section != "" {
		t.Fatalf("items = %+v", items)
	}
	items, _ = parseMD(t, "This release only fixes a bug.\n", domain.RoleReleaseNotes)
	if len(items) != 1 || items[0].Text != "This release only fixes a bug." {
		t.Errorf("prose-only document: %+v", items)
	}
	items, evs, err := ParseNotes(testInput("", domain.RoleReleaseNotes), nil)
	if err != nil || len(items) != 0 || len(evs) != 0 {
		t.Errorf("empty document: %v %v %v", items, evs, err)
	}
}

func TestParseNotesContainerIntro(t *testing.T) {
	md := `# Title

Title intro that is not an item.

## v1.0.0

This release adds a lot of things that matter to everybody.

Changes since v0.9.0:

### Feature

- shiny thing

### Bug or Regression

- broken thing
`
	items, _ := parseMD(t, md, domain.RoleReleaseNotes)
	got := texts(items)
	want := []string{
		"v1.0.0: This release adds a lot of things that matter to everybody.",
		"shiny thing",
		"broken thing",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("items:\n got %q\nwant %q", got, want)
	}
}

func TestParseNotesShortcodeCodeBlockInsideItem(t *testing.T) {
	md := `## Traffic

- **Added** support for X.

  Example usage:

{{< text yaml >}}
apiVersion: v1
kind: Foo
{{< /text >}}

  ([Issue #60122](https://github.com/istio/istio/issues/60122))

- **Fixed** Y.
`
	items, evs := parseMD(t, md, domain.RoleReleaseNotes)
	if len(items) != 2 {
		t.Fatalf("items = %q", texts(items))
	}
	if want := "Added support for X. Example usage: (Issue #60122)"; items[0].Text != want {
		t.Errorf("text = %q, want %q", items[0].Text, want)
	}
	if !strings.Contains(evs[0].Excerpt, "apiVersion: v1") {
		t.Errorf("excerpt should keep the verbatim code: %q", evs[0].Excerpt)
	}
	if evs[0].Locator != "L3-L12 (Traffic)" {
		t.Errorf("locator = %q", evs[0].Locator)
	}
	if items[0].Category != domain.CategoryFeature || items[1].Category != domain.CategoryBugfix {
		t.Errorf("categories = %v %v", items[0].Category, items[1].Category)
	}
}

func TestParseNotesReferences(t *testing.T) {
	md := "## Fixes\n\n" +
		"- Fixed a thing ([`#7810`](https://github.com/example/repo/pull/7810), [`@alice`](https://github.com/alice)) for CVE-2025-27144 and GHSA-hcg3-q754-cr77.\n"
	items, _ := parseMD(t, md, domain.RoleReleaseNotes)
	if len(items) != 1 {
		t.Fatalf("items: %q", texts(items))
	}
	got := items[0].References
	want := []domain.Reference{
		{Type: "pull-request", ID: "example/repo#7810", URL: "https://github.com/example/repo/pull/7810"},
		{Type: "cve", ID: "CVE-2025-27144", URL: "https://www.cve.org/CVERecord?id=CVE-2025-27144"},
		{Type: "ghsa", ID: "GHSA-hcg3-q754-cr77", URL: "https://github.com/advisories/GHSA-hcg3-q754-cr77"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("references:\n got %+v\nwant %+v", got, want)
	}
}

func TestParseNotesDuplicateItemsMerge(t *testing.T) {
	md := "## A\n\n- same text\n\n## B\n\n- same text\n- other\n"
	items, evs := parseMD(t, md, domain.RoleReleaseNotes)
	if len(items) != 2 {
		t.Fatalf("duplicate should be merged: %q", texts(items))
	}
	seen := map[string]bool{}
	for _, it := range items {
		if seen[it.ID] {
			t.Errorf("duplicate id %s", it.ID)
		}
		seen[it.ID] = true
	}
	if len(items[0].Evidence) != 2 || len(evs) != 3 {
		t.Errorf("first item should reference both locations: %v (evidence %d)", items[0].Evidence, len(evs))
	}
	for _, it := range items {
		for _, id := range it.Evidence {
			found := false
			for _, e := range evs {
				if e.ID == id {
					found = true
				}
			}
			if !found {
				t.Errorf("evidence %s not returned", id)
			}
		}
	}
}

func TestParseNotesDeterministic(t *testing.T) {
	md := string(fixture(t, "certmanager/release-notes-1.18.md"))
	in := testInput(md, domain.RoleReleaseNotes)
	a, ea, _ := ParseNotes(in, nil)
	for i := 0; i < 3; i++ {
		b, eb, _ := ParseNotes(in, nil)
		if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(ea, eb) {
			t.Fatal("ParseNotes output differs between runs")
		}
	}
}

func TestParseNotesInvalidRule(t *testing.T) {
	if _, _, err := ParseNotes(testInput("- a\n", domain.RoleReleaseNotes), []catalog.ClassifyRule{{Section: "("}}); err == nil {
		t.Error("invalid section regex should fail")
	}
	if _, _, err := ParseNotes(testInput("- a\n", domain.RoleReleaseNotes), []catalog.ClassifyRule{{Text: "["}}); err == nil {
		t.Error("invalid text regex should fail")
	}
}
