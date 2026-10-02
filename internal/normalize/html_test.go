package normalize

import (
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// The fixture condenses the two real shapes this converter exists for: the
// published notes page (go.dev/doc/go1.N: h2/h3/h4 sections, p prose with
// inline code/em/a, comments, a head with style/script) and the in-tree
// doc/go1.N.html copy of the pre-1.21 era (the DRAFT banner, per-package
// dl/dt/dd blocks, ul/li lists, pre terminals).
func TestHTMLToMarkdownPreservesLines(t *testing.T) {
	src := fixture(t, "golang/release-notes-excerpt.html")
	out := HTMLToMarkdown(src)
	if got, want := strings.Count(string(out), "\n"), strings.Count(string(src), "\n"); got != want {
		t.Fatalf("line count changed: %d lines in, %d lines out", want, got)
	}
	lines := strings.Split(string(out), "\n")
	at := func(n int) string { return lines[n-1] } // 1-based, as in the HTML source

	for n, want := range map[int]string{
		13: "## DRAFT RELEASE NOTES — Introduction to Go 1.20",
		22: "## Changes to the language",
		30: "### Cover",
		42: "  - a `percent` function",
		48: "    $ go tool cover -func",
		54: "**[archive/tar](/pkg/archive/tar/)**",
		57: "  When the `GODEBUG=tarinsecurepath=0` environment variable is set,",
		71: "#### unsafe package & generics",
	} {
		if at(n) != want {
			t.Errorf("line %d = %q, want %q", n, at(n), want)
		}
	}
	md := string(out)
	for _, want := range []string{
		"- Coverage summaries can be collected for programs that are not tests.",
		"- The `coverage` command added:",
		"before [exiting](/pkg/go/...). It’s documented in", // entity decoded
		"*the command documentation*.",
		"[`Reader.Next`](/pkg/archive/tar/#Reader.Next) method", // code inside a link
	} {
		if !strings.Contains(md, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	// head, style and script contents, comments and tags are dropped
	for _, bad := range []string{
		"<", "window.something", "main ul li", "Content-Type", "https://go.dev/issue/55356",
	} {
		if strings.Contains(md, bad) {
			t.Errorf("output must not contain %q", bad)
		}
	}
}

func TestHTMLSectionSelection(t *testing.T) {
	md := HTMLToMarkdown(fixture(t, "golang/release-notes-excerpt.html"))
	sec, err := SelectSection(md, mustRe(`^## Core library$`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sec.Body, "**[archive/tar]") || !strings.Contains(sec.Body, "ErrInsecurePath") {
		t.Fatalf("core library section:\n%s", sec.Body)
	}
	if _, err := SelectSection(md, mustRe(`^### Cover$`)); err != nil {
		t.Fatalf("cover section: %v", err)
	}
}

// Converted HTML is parsed in list-item mode (ingest sets DocInput.ListItems
// for every converted format): every bullet is one item, prose paragraphs are
// items of their own, dt/dd blocks are not folded.
func TestParseNotesHTMLListItems(t *testing.T) {
	md := HTMLToMarkdown(fixture(t, "golang/release-notes-excerpt.html"))
	sec, err := SelectSection(md, mustRe(`^### Cover$`))
	if err != nil {
		t.Fatal(err)
	}
	in := DocInput{SourceID: "release-notes", Role: domain.RoleReleaseNotes, Release: "1.20.0",
		URI: "https://go.dev/doc/go1.20", Content: []byte(sec.Body), LineOffset: sec.StartLine - 1,
		RetrievedAt: time.Unix(0, 0), ListItems: true}
	items, _, err := ParseNotes(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	// the parser normalizes the bullet prefix away: each flat top-level list
	// block is one item, the prose intro is one item (nested lists fold into
	// their parent item, the shared docbook/rst behavior)
	var texts []string
	for _, it := range items {
		texts = append(texts, it.Text)
	}
	got := strings.Join(texts, "\n")
	if len(items) != 3 {
		for _, it := range items {
			t.Logf("item %q section %q", it.Text, it.Section)
		}
		t.Fatalf("got %d items, want 3 (prose intro + 2 flat bullets)", len(items))
	}
	if !strings.Contains(got, "Coverage summaries can be collected for programs that are not tests.") {
		t.Errorf("items lack the flat bullet:\n%s", got)
	}
	if !strings.Contains(items[0].Text, "go test -cover") || items[0].Section != "Cover" {
		t.Errorf("first item is not the prose intro: %+v", items[0])
	}
}

// TestHTMLToMarkdownHeadingsCloseAcrossLines: a heading whose text spans
// lines is written back onto its opening line, and unclosed inner tags
// (HTML allows omitting </li>, </p>, </dd>) close at their parent's end tag.
func TestHTMLToMarkdownUnclosedElements(t *testing.T) {
	src := "<ul>\n<li>one\n<li>two\n</ul>\n<p>a</p>\n"
	out := string(HTMLToMarkdown([]byte(src)))
	for _, want := range []string{"- one", "- two", "a"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q lacks %q", out, want)
		}
	}
	if strings.Count(out, "- ") != 2 {
		t.Errorf("output %q: want exactly 2 bullets", out)
	}
}
