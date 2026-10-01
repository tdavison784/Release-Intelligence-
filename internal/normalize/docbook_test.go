package normalize

import (
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestDocBookToMarkdownPreservesLines(t *testing.T) {
	src := fixture(t, "postgresql/release-excerpt.sgml")
	out := DocBookToMarkdown(src)
	if got, want := strings.Count(string(out), "\n"), strings.Count(string(src), "\n"); got != want {
		t.Fatalf("line count changed: %d lines in, %d lines out", want, got)
	}
	lines := strings.Split(string(out), "\n")
	at := func(n int) string { return lines[n-1] } // 1-based, as in the SGML source

	// headings: line N of the markdown is the <title> line N of the source
	for n, want := range map[int]string{
		5:   "# Release 17.2",
		19:  "## Migration to Version 17.2",
		32:  "## Changes",
		75:  "# Release 17",
		116: "### Server",
		119: "#### Optimizer",
	} {
		if at(n) != want {
			t.Errorf("line %d = %q, want %q", n, at(n), want)
		}
	}
	md := string(out)
	for _, want := range []string{
		"**Release date:**",
		"`release-17`.", // <xref/> becomes the linked id
		"- Restore functionality of `ALTER {ROLE|DATABASE} SET\n  role` (Tom Lane, Noah Misch)", // bullet + 2-space continuation
		"      ALTER ROLE app SET role = 'owner';",                                              // programlisting is indented code (in a list item)
		"re-apply them.",               // <link linkend>: text kept
		"(Álvaro Herrera)",             // entity decoded
		"*no*",                         // emphasis
		"`postgres_fdw` to handle SQL", // "</>" shorthand closes the code span
		"`postgres-fdw`)",              // void <xref> without end tag
		"  - Only affects servers with *no* WAL archive.", // nested list is indented
	} {
		if !strings.Contains(md, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	for _, bad := range []string{"&sect;", "commit_baseurl", "Author:", "<para>", "</", "&Aacute;"} {
		if strings.Contains(md, bad) {
			t.Errorf("output must not contain %q", bad)
		}
	}
}

func TestDocBookSectionSelection(t *testing.T) {
	md := DocBookToMarkdown(fixture(t, "postgresql/release-excerpt.sgml"))
	sec, err := SelectSection(md, mustRe(`^Release 17\.2$`))
	if err != nil {
		t.Fatal(err)
	}
	if sec.StartLine != 5 || !strings.Contains(sec.Body, "Restore functionality") || strings.Contains(sec.Body, "Release 17\n") {
		t.Fatalf("section 17.2: lines %d-%d\n%s", sec.StartLine, sec.EndLine, sec.Body)
	}
	if _, err := SelectSection(md, mustRe(`^Release 17$`)); err != nil {
		t.Fatalf("major release section: %v", err)
	}
	if _, err := SelectSection(md, mustRe(`^Release 9\.6\.1$`)); err != nil {
		t.Fatalf("legacy section: %v", err)
	}
}

// ListItems reads DocBook structure: every list item is one item even when
// prose precedes the list, prose-only sections are one item, nothing is folded.
func TestParseNotesListItems(t *testing.T) {
	md := DocBookToMarkdown(fixture(t, "postgresql/release-excerpt.sgml"))
	parse := func(heading string) []domain.NoteItem {
		sec, err := SelectSection(md, mustRe(heading))
		if err != nil {
			t.Fatal(err)
		}
		in := DocInput{SourceID: "release-notes", Role: domain.RoleReleaseNotes, Release: "17.0.0",
			URI: "https://example.test/release-17.sgml", Content: []byte(sec.Body), LineOffset: sec.StartLine - 1,
			RetrievedAt: time.Unix(0, 0), ListItems: true}
		items, _, err := ParseNotes(in, nil)
		if err != nil {
			t.Fatal(err)
		}
		return items
	}
	texts := func(items []domain.NoteItem) (out []string) {
		for _, it := range items {
			out = append(out, it.Section+" | "+it.Text)
		}
		return
	}

	minor := parse(`^Release 17\.2$`)
	got := strings.Join(texts(minor), "\n")
	for _, want := range []string{
		"Release 17.2 › Migration to Version 17.2 | Migration to Version 17.2: A dump/restore is not required",
		"Release 17.2 › Changes | Restore functionality of `ALTER {ROLE|DATABASE} SET role`",
		"Release 17.2 › Changes | Fix crash in `pg_rewind` (Álvaro Herrera)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("17.2 items lack %q:\n%s", want, got)
		}
	}
	if len(minor) != 3 {
		t.Errorf("17.2: %d items, want 3 (migration prose, two list items):\n%s", len(minor), got)
	}
	for _, it := range minor {
		// evidence line ranges point into the original SGML
		if len(it.Evidence) != 1 {
			t.Errorf("item %q has %d evidence", it.Text, len(it.Evidence))
		}
	}

	major := parse(`^Release 17$`)
	got = strings.Join(texts(major), "\n")
	for _, want := range []string{
		"Release 17 › Migration to Version 17 | Migration to Version 17: A dump/restore using `app-pg-dumpall` or use of `pgupgrade` is required",
		"Release 17 › Migration to Version 17 | Remove server variable `old_snapshot_threshold`",
		"Release 17 › Migration to Version 17 | Change `SET SESSION AUTHORIZATION` handling",
		"Release 17 › Changes › Server › Optimizer | Allow the optimizer to improve `IS [NOT] NULL` plans",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("17 items lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Below you will find") && strings.Contains(got, "Optimizer: -") {
		t.Errorf("sub-sections must not be folded into their parent:\n%s", got)
	}
}
