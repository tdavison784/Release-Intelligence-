package normalize

import (
	"regexp"
	"strings"
	"testing"
)

// setextHeadings renders a document like Redis's 00-RELEASENOTES (7.2+): a
// decorative bar above the title, an underline below it, setext-H2
// subsections (hyphen underline) and ATX subsections.
func setextDoc() string {
	return strings.Join([]string{
		"Redis 8.2 release notes",           // 1 (setext H1, no bar)
		"=================================", // 2
		"",                                  // 3
		"=================================", // 4
		"Redis 8.2.10    Released Thu",      // 5 (setext H1 with bar)
		"=================================", // 6
		"",                                  // 7
		"Update urgency: `SECURITY`.",       // 8
		"",                                  // 9
		"Security fixes",                    // 10 (setext H2)
		"--------------",                    // 11
		"* (CVE-2026-1) something",          // 12
		"",                                  // 13
		"### Bug fixes",                     // 14 (ATX H3)
		"",                                  // 15
		"- fix one",                         // 16
		"```",                               // 17 (fence)
		"Not a heading",                     // 18
		"===================",               // 19
		"```",                               // 20
	}, "\n")
}

func TestScanSetextHeadings(t *testing.T) {
	d := scanDocument([]byte(setextDoc()))
	type want struct {
		line int // 1-based
		lvl  int
		head string
	}
	wants := []want{
		{1, 1, "Redis 8.2 release notes"},
		{5, 1, "Redis 8.2.10 Released Thu"},
		{10, 2, "Security fixes"},
		{14, 3, "Bug fixes"},
	}
	seen := 0
	for _, l := range d.lines {
		if l.kind != lkHeading {
			continue
		}
		if seen == len(wants) {
			t.Fatalf("unexpected extra heading L%d: %q", l.n, l.head)
		}
		w := wants[seen]
		seen++
		if l.n != w.line || l.level != w.lvl || l.head != w.head {
			t.Errorf("L%d: got level %d head %q, want L%d level %d head %q",
				l.n, l.level, l.head, w.line, w.lvl, w.head)
		}
	}
	if seen != len(wants) {
		t.Fatalf("got %d headings, want %d", seen, len(wants))
	}
	// decorative bar above the title is blank, not text
	if d.lines[3].kind != lkBlank {
		t.Errorf("L4 (decorative bar): kind %v, want lkBlank", d.lines[3].kind)
	}
	// the fenced pseudo-heading stays fenced content
	if d.lines[17].kind == lkHeading {
		t.Errorf("L18 inside fence became a heading")
	}
}

func TestSelectSectionSetext(t *testing.T) {
	sec, err := SelectSection([]byte(setextDoc()), regexp.MustCompile(`^Redis 8\.2\.10\s+Released`))
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if sec.StartLine != 5 || sec.Level != 1 {
		t.Errorf("got start %d level %d, want 5/1", sec.StartLine, sec.Level)
	}
	if !strings.Contains(sec.Body, "Security fixes") || !strings.Contains(sec.Body, "(CVE-2026-1)") {
		t.Errorf("body does not contain the setext-H2 subsection: %q", sec.Body)
	}
	// GA shape with parenthesised version and no "Redis" prefix
	ga := strings.Join([]string{
		"===========================================",
		"8.2 GA (v8.2.0)    Released Mon 4 Aug 2025",
		"===========================================",
		"",
		"- bundled modules",
	}, "\n")
	sec, err = SelectSection([]byte(ga), regexp.MustCompile(`^8\.2 GA`))
	if err != nil {
		t.Fatalf("select GA: %v", err)
	}
	if !strings.Contains(sec.Body, "bundled modules") {
		t.Errorf("GA body wrong: %q", sec.Body)
	}
}

func TestSetextNotInListOrTable(t *testing.T) {
	// a dash line after a blank is a thematic break, not an underline;
	// a paragraph directly above a table must not eat the table
	doc := strings.Join([]string{
		"Intro paragraph.", // 1
		"",                 // 2
		"---",              // 3 thematic break
		"",                 // 4
		"| a | b |",        // 5
		"|---|---|",        // 6
		"| 1 | 2 |",        // 7
	}, "\n")
	d := scanDocument([]byte(doc))
	for _, l := range d.lines {
		if l.kind == lkHeading {
			t.Errorf("L%d unexpectedly a heading: %q", l.n, l.head)
		}
	}
	// a single-dash underline right after text is a setext H2 (CommonMark)
	d = scanDocument([]byte("Title\n-"))
	if d.lines[0].kind != lkHeading || d.lines[0].level != 2 {
		t.Errorf("single-dash underline: kind %v level %d, want heading level 2", d.lines[0].kind, d.lines[0].level)
	}
}
