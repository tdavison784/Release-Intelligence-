package normalize

import (
	"regexp"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

const labelRe = `^[A-Z][A-Z0-9 /&-]*:$`

// hashicorpChangelog mimics one release section of a HashiCorp-style
// CHANGELOG.md: a version heading, a date heading, then uppercase label
// paragraphs followed by bullet lists.
const hashicorpChangelog = `## 2.1.1
### September 16, 2026

SECURITY:

* core: Update golang.org/x/crypto to v0.56.0 to fix a vulnerability.

BREAKING CHANGES:

* containers: Packages gnupg, openssl have been dropped from images.

CHANGES:

* core: Bump Go version to 1.26.8

IMPROVEMENTS:

* ui: Bump dompurify to 3.4.15

FEATURES:

* **Agent Registry UI**: Adds a new page.

DEPRECATIONS:

* The legacy endpoint is deprecated.

BUG FIXES:

* ui: Fix agent registry ceiling policies not displaying.
* auth/token: Fix 403 on lookup.
`

func parseLabels(t *testing.T, md, pattern string, rules ...catalog.ClassifyRule) []domain.NoteItem {
	t.Helper()
	in := testInput(md, domain.RoleReleaseNotes)
	in.LabelPattern = pattern
	items, _, err := ParseNotes(in, rules)
	if err != nil {
		t.Fatalf("ParseNotes: %v", err)
	}
	return items
}

func findItem(t *testing.T, items []domain.NoteItem, substr string) domain.NoteItem {
	t.Helper()
	for _, it := range items {
		if strings.Contains(it.Text, substr) {
			return it
		}
	}
	t.Fatalf("no item containing %q in %v", substr, texts(items))
	return domain.NoteItem{}
}

func TestLabelParagraphsClassifyBullets(t *testing.T) {
	items := parseLabels(t, hashicorpChangelog, labelRe)
	if len(items) != 8 {
		t.Fatalf("want 8 items, got %d: %v", len(items), texts(items))
	}
	cases := []struct {
		substr   string
		category domain.Category
		breaking bool
		section  string
	}{
		{"golang.org/x/crypto", domain.CategorySecurity, false, "2.1.1 › September 16, 2026 › SECURITY"},
		{"gnupg", "", true, "2.1.1 › September 16, 2026 › BREAKING CHANGES"},
		{"Agent Registry UI", domain.CategoryFeature, false, "2.1.1 › September 16, 2026 › FEATURES"},
		{"legacy endpoint", domain.CategoryDeprecation, false, "2.1.1 › September 16, 2026 › DEPRECATIONS"},
		{"agent registry ceiling", domain.CategoryBugfix, false, "2.1.1 › September 16, 2026 › BUG FIXES"},
		{"auth/token", domain.CategoryBugfix, false, "2.1.1 › September 16, 2026 › BUG FIXES"},
	}
	for _, c := range cases {
		it := findItem(t, items, c.substr)
		if c.category != "" && it.Category != c.category {
			t.Errorf("%s: category %q, want %q (rule %s)", c.substr, it.Category, c.category, it.Classification.Rule)
		}
		if it.Breaking != c.breaking {
			t.Errorf("%s: breaking=%v, want %v", c.substr, it.Breaking, c.breaking)
		}
		if it.Section != c.section {
			t.Errorf("%s: section %q, want %q", c.substr, it.Section, c.section)
		}
		if it.Classification.Method != domain.MethodDeclared {
			t.Errorf("%s: label-derived classification must be declared, got %s", c.substr, it.Classification.Method)
		}
	}
}

func TestLabelParagraphsProductRulesSeeTheLabel(t *testing.T) {
	rules := []catalog.ClassifyRule{{Section: `^IMPROVEMENTS$`, Category: domain.CategoryFeature}}
	items := parseLabels(t, hashicorpChangelog, labelRe, rules...)
	if it := findItem(t, items, "dompurify"); it.Category != domain.CategoryFeature {
		t.Errorf("IMPROVEMENTS bullet: category %q, want feature", it.Category)
	}
}

func TestLabelParagraphsOffByDefault(t *testing.T) {
	items := parseLabels(t, hashicorpChangelog, "")
	for _, it := range items {
		if strings.Contains(it.Section, "SECURITY") || strings.Contains(it.Section, "BUG FIXES") {
			t.Fatalf("labels must not be headings without a pattern: %q", it.Section)
		}
	}
	if it := findItem(t, items, "Fix 403"); it.Category == domain.CategoryBugfix && it.Classification.Method == domain.MethodDeclared {
		t.Errorf("without the pattern the bug fix label must not classify the bullet (got %+v)", it.Classification)
	}
}

func TestLabelParagraphsAreHeadingsOneLevelBelow(t *testing.T) {
	d := scanDocument([]byte(hashicorpChangelog))
	d.promoteLabels(regexp.MustCompile(labelRe))
	want := map[string]int{"2.1.1": 2, "September 16, 2026": 3, "SECURITY": 4, "BUG FIXES": 4, "BREAKING CHANGES": 4}
	got := map[string]int{}
	for _, l := range d.lines {
		if l.kind == lkHeading {
			got[l.head] = l.level
		}
	}
	for h, lvl := range want {
		if got[h] != lvl {
			t.Errorf("heading %q level %d, want %d (all: %v)", h, got[h], lvl, got)
		}
	}
	// a label right below a level 2 heading is level 3
	d = scanDocument([]byte("## 1.0.0\n\nBUG FIXES:\n\n* x\n"))
	d.promoteLabels(regexp.MustCompile(labelRe))
	if l := d.lines[2]; l.kind != lkHeading || l.level != 3 || l.head != "BUG FIXES" {
		t.Errorf("label under ## heading: %+v", l)
	}
	// no heading at all: level 1
	d = scanDocument([]byte("BUG FIXES:\n\n* x\n"))
	d.promoteLabels(regexp.MustCompile(labelRe))
	if l := d.lines[0]; l.kind != lkHeading || l.level != 1 {
		t.Errorf("label without heading: %+v", l)
	}
}

func TestLabelParagraphsIgnoreLookalikes(t *testing.T) {
	md := "## 1.0.0\n\n" +
		"* an item that wraps and\n" +
		"  NOTE:\n" + // indented continuation of a list item
		"\n" +
		"```\n" +
		"BUG FIXES:\n" + // inside a fenced code block
		"```\n" +
		"\n" +
		"The text before a label-like line\n" +
		"WARNING:\n" + // wrapped paragraph: previous line is text, not blank
		"\n" +
		"    SECURITY:\n" + // indented code
		"\n" +
		"Not a label: it has trailing words\n"
	d := scanDocument([]byte(md))
	d.promoteLabels(regexp.MustCompile(labelRe))
	for _, l := range d.lines {
		if l.label {
			t.Errorf("line %d %q must not be a label", l.n, l.raw)
		}
	}
}

func TestLabelParagraphsInvalidPattern(t *testing.T) {
	in := testInput(hashicorpChangelog, domain.RoleReleaseNotes)
	in.LabelPattern = `[`
	if _, _, err := ParseNotes(in, nil); err == nil {
		t.Fatal("an invalid label pattern must be an error")
	}
}
