package normalize

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// The fixture is condensed from Cilium's Documentation/operations/upgrade.rst
// at v1.19.x: overline+underline page title, "=" sections, "-" the
// "1.19 Upgrade Notes" subsection, "~" and "#" levels below it, a grid
// compatibility table, directive lines and inline roles.

func TestRSTToMarkdownPreservesLines(t *testing.T) {
	src := fixture(t, "cilium/upgrade-notes.rst")
	out := RSTToMarkdown(src)
	count := func(b []byte) int { return len(strings.Split(strings.TrimRight(string(b), "\n"), "\n")) }
	if got, want := count(out), count(src); got != want {
		t.Fatalf("line count changed: %d lines in, %d lines out", want, got)
	}
	lines := strings.Split(string(out), "\n")
	at := func(n int) string { return lines[n-1] } // 1-based, as in the rst source

	// heading levels follow rst first-use order; the overline+underline
	// title style is distinct from the underline styles
	for n, want := range map[int]string{
		10: "# Upgrade Guide", // overline style: first used -> level 1
		15: "## Running pre-flight check (Required)",
		28: "## Version Specific Notes",
		33: "### Kubernetes Compatibility",
		46: "### 1.19 Upgrade Notes",
		49: "#### Action Required",
		59: "#### Informational Notes",
		73: "##### Deprecated Options",
		81: "##### Removed Options",
		87: "## Advanced",
	} {
		if at(n) != want {
			t.Errorf("line %d = %q, want %q", n, at(n), want)
		}
	}
	// adornment lines are blanked (title over/underline 8-9, section 12)
	for _, n := range []int{8, 9, 12, 16, 17, 34, 35, 47, 48} {
		if at(n) != "" {
			t.Errorf("line %d = %q, want blank (adornment)", n, at(n))
		}
	}
	// directive lines are blanked
	if at(1) != "" || at(21) != "" {
		t.Errorf("directive lines must be blanked: %q / %q", at(1), at(21))
	}
	// directive content (indented) remains, roles flatten, literals convert
	for _, want := range []string{
		"| 1.29, 1.30, 1.31, 1.32 | * networking.k8s.io/v1 | CustomResourceDefinition |",
		"* The `v2alpha1` version of the `CiliumNodeConfig` CRD is deprecated.",
		"* The previously deprecated `--k8s-api-server` agent flag has been removed in",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if got := inlineRST(":ref:`DNS based <dns_policy>` and :ref:`gs_mutual` and `networking.k8s.io/v1`_"); got != "DNS based and gs_mutual and networking.k8s.io/v1" {
		t.Errorf("titled references: %q", got)
	}
	for _, bad := range []string{".. only::", ".. tabs::", ":term:", "``cilium", "`networking.k8s.io/v1`_"} {
		if strings.Contains(string(out), bad) {
			t.Errorf("output must not contain %q", bad)
		}
	}
}

func TestRSTSectionSelection(t *testing.T) {
	md := RSTToMarkdown(fixture(t, "cilium/upgrade-notes.rst"))
	sec, err := SelectSection(md, mustRe(`^1\.19 Upgrade Notes$`))
	if err != nil {
		t.Fatal(err)
	}
	if sec.StartLine != 46 {
		t.Fatalf("section starts at line %d, want 46", sec.StartLine)
	}
	for _, want := range []string{"Action Required", "Gateway API v1.6.1", "Removed Options"} {
		if !strings.Contains(sec.Body, want) {
			t.Errorf("section body lacks %q", want)
		}
	}
	if strings.Contains(sec.Body, "e2e tested") {
		t.Error("section body must not contain the preceding section")
	}
	if _, err := SelectSection(md, mustRe(`^Upgrade Guide$`)); err != nil {
		t.Fatalf("overline title: %v", err)
	}
}

// The grid compatibility table becomes a pipe table the markdown-table
// extractor can read.
func TestRSTGridTable(t *testing.T) {
	md := RSTToMarkdown(fixture(t, "cilium/upgrade-notes.rst"))
	row, err := ExtractTableRow(md, TableSelector{KeyColumns: []string{"k8s Version"}, KeyRe: mustRe(`1\.29`)})
	if err != nil {
		t.Fatal(err)
	}
	if row.Cells["k8s Version"] != "1.29, 1.30, 1.31, 1.32" {
		t.Fatalf("k8s cell = %q", row.Cells["k8s Version"])
	}
	if row.Line == 0 {
		t.Error("row must carry its line number")
	}
}

// Notes parse with their sub-sections: Action Required items are separate
// items carrying the section path, ready for classify rules.
func TestRSTNotes(t *testing.T) {
	md := RSTToMarkdown(fixture(t, "cilium/upgrade-notes.rst"))
	sec, err := SelectSection(md, mustRe(`^1\.19 Upgrade Notes$`))
	if err != nil {
		t.Fatal(err)
	}
	items, _, err := ParseNotes(DocInput{SourceID: "upgrade-notes", Role: domain.RoleUpgradeGuide, Release: "1.19.0",
		Content: []byte(sec.Body), LineOffset: sec.StartLine - 1, ListItems: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var action, deprecated []domain.NoteItem
	for _, it := range items {
		if strings.HasSuffix(it.Section, "› Action Required") {
			action = append(action, it)
		}
		if strings.HasSuffix(it.Section, "› Changes to Features › Deprecated Options") {
			deprecated = append(deprecated, it)
		}
	}
	// structural reading: every bullet is an item; a plain intro paragraph is
	// an item of its own, a colon lead-in ("The following options ...:") is not
	if len(action) != 3 || !strings.Contains(action[0].Text, "If you are using") {
		t.Fatalf("Action Required items = %d, want intro + 2 bullets (got %v)", len(action), sectionTexts(items))
	}
	if len(deprecated) != 1 || !strings.Contains(deprecated[0].Text, "preferIpv6") {
		t.Fatalf("Deprecated Options items = %d, want the bullet only (got %v)", len(deprecated), sectionTexts(items))
	}
}

func sectionTexts(items []domain.NoteItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Section + " :: " + it.Text
	}
	return out
}

// Text followed by a shorter ruler is not a heading (rst underlines must be
// at least as long as the title); list lines are never titles.
func TestRSTNonHeadings(t *testing.T) {
	src := "" +
		"Summary of Changes\n" +
		"------\n" +
		"* bullet\n" +
		"====\n" +
		"\n" +
		"1.29 - 1.32\n" +
		"============\n"
	out := string(RSTToMarkdown([]byte(src)))
	for _, bad := range []string{"# Summary", "# bullet", "# ===="} {
		if strings.Contains(out, bad) {
			t.Errorf("%q must not become a heading:\n%s", bad, out)
		}
	}
	if !strings.Contains(out, "# 1.29 - 1.32") {
		t.Errorf("a full-width underline is a heading:\n%s", out)
	}
}
