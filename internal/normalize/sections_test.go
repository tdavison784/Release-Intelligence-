package normalize

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSelectSection(t *testing.T) {
	doc := strings.Join([]string{
		"---",                        // 1
		"title: Release",             // 2
		"# not a heading",            // 3  (inside front matter)
		"---",                        // 4
		"",                           // 5
		"Intro text.",                // 6
		"",                           // 7
		"## Major Themes",            // 8
		"",                           // 9
		"### First theme",            // 10
		"text",                       // 11
		"```bash",                    // 12
		"## not a heading",           // 13  (inside fence)
		"```",                        // 14
		"### Second theme {#second}", // 15
		"more",                       // 16
		"",                           // 17
		"## `v1.2.3`",                // 18
		"",                           // 19
		"~~~",                        // 20
		"# nope",                     // 21
		"~~~",                        // 22
		"### **Feature**",            // 23
		"- a",                        // 24
		"",                           // 25
		"## `v1.2.2`  ##",            // 26
		"old",                        // 27
		"",                           // 28
	}, "\n")

	tests := []struct {
		name      string
		re        string
		wantErr   error
		heading   string
		level     int
		path      []string
		start     int
		end       int
		bodyHas   []string
		bodyHasNo []string
	}{
		{name: "plain h2", re: `^Major Themes$`, heading: "Major Themes", level: 2, path: []string{"Major Themes"}, start: 8, end: 16,
			bodyHas: []string{"### First theme", "## not a heading", "### Second theme {#second}"}, bodyHasNo: []string{"v1.2.3"}},
		{name: "backticks trimmed", re: `^v1\.2\.3$`, heading: "v1.2.3", level: 2, path: []string{"v1.2.3"}, start: 18, end: 24,
			bodyHas: []string{"~~~", "# nope", "- a"}},
		{name: "nested emphasis trimmed, path includes ancestors", re: `^Feature$`, heading: "Feature", level: 3, path: []string{"v1.2.3", "Feature"}, start: 23, end: 24},
		{name: "anchor suffix removed", re: `^Second theme$`, heading: "Second theme", level: 3, path: []string{"Major Themes", "Second theme"}, start: 15, end: 16},
		{name: "closing hashes removed", re: `^v1\.2\.2$`, heading: "v1.2.2", level: 2, path: []string{"v1.2.2"}, start: 26, end: 27},
		{name: "heading in front matter is ignored", re: `not a heading`, wantErr: ErrNoMatch},
		{name: "heading in fenced code is ignored", re: `^nope$`, wantErr: ErrNoMatch},
		{name: "no match", re: `^zzz$`, wantErr: ErrNoMatch},
		{name: "first match wins", re: `^v1\.2`, heading: "v1.2.3", level: 2, path: []string{"v1.2.3"}, start: 18, end: 24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := SelectSection([]byte(doc), mustRe(tt.re))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s.Heading != tt.heading || s.Level != tt.level || s.StartLine != tt.start || s.EndLine != tt.end {
				t.Errorf("got heading=%q level=%d lines=%d-%d, want %q %d %d-%d", s.Heading, s.Level, s.StartLine, s.EndLine, tt.heading, tt.level, tt.start, tt.end)
			}
			if !reflect.DeepEqual(s.Path, tt.path) {
				t.Errorf("path = %v, want %v", s.Path, tt.path)
			}
			for _, h := range tt.bodyHas {
				if !strings.Contains(s.Body, h) {
					t.Errorf("body lacks %q:\n%s", h, s.Body)
				}
			}
			for _, h := range tt.bodyHasNo {
				if strings.Contains(s.Body, h) {
					t.Errorf("body unexpectedly contains %q", h)
				}
			}
			// body is exactly the original lines start..end
			want := strings.Join(strings.Split(doc, "\n")[s.StartLine-1:s.EndLine], "\n")
			if s.Body != want {
				t.Errorf("body mismatch:\n got %q\nwant %q", s.Body, want)
			}
		})
	}
}

func TestSelectSectionEdgeCases(t *testing.T) {
	// nil regexp and empty input never panic
	if _, err := SelectSection([]byte("# a"), nil); !errors.Is(err, ErrNoMatch) {
		t.Errorf("nil regexp: %v", err)
	}
	if _, err := SelectSection(nil, mustRe(".")); !errors.Is(err, ErrNoMatch) {
		t.Errorf("empty doc: %v", err)
	}
	// CRLF line endings, tab-indented fence, BOM
	crlf := "\xef\xbb\xbf# One\r\n\r\ntext\r\n\r\n## Two\r\nbody\r\n"
	s, err := SelectSection([]byte(crlf), mustRe(`^Two$`))
	if err != nil || s.StartLine != 5 || s.EndLine != 6 || s.Body != "## Two\nbody" {
		t.Errorf("crlf: %+v err=%v", s, err)
	}
	// a section without trailing newline and a heading that is the last line
	s, err = SelectSection([]byte("## a\ntext\n## b"), mustRe(`^b$`))
	if err != nil || s.StartLine != 3 || s.EndLine != 3 {
		t.Errorf("last heading: %+v err=%v", s, err)
	}
	// "#hashtag" and 7 hashes are not headings
	if _, err := SelectSection([]byte("#hashtag\n####### seven\n"), mustRe(`.`)); !errors.Is(err, ErrNoMatch) {
		t.Errorf("non headings matched: %v", err)
	}
	// trailing blank lines and comments are not part of the section
	s, err = SelectSection([]byte("## a\ntext\n\n<!-- c -->\n\n## b\n"), mustRe(`^a$`))
	if err != nil || s.EndLine != 2 {
		t.Errorf("trailing blanks: %+v err=%v", s, err)
	}
	// headings are found inside HTML comments only when outside of them
	s, err = SelectSection([]byte("<!--\n## hidden\n-->\n## shown\n"), mustRe(`hidden|shown`))
	if err != nil || s.Heading != "shown" {
		t.Errorf("comment: %+v err=%v", s, err)
	}
}

func TestSelectSectionFixtures(t *testing.T) {
	md := fixture(t, "certmanager/release-notes-1.18.md")
	// the research doc: per-patch headings are "## `v1.18.x`"; line numbers of
	// the real file
	s, err := SelectSection(md, mustRe(`^v1\.18\.0$`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Level != 2 || s.Heading != "v1.18.0" || s.StartLine != 319 {
		t.Errorf("v1.18.0: %+v", s)
	}
	if !strings.Contains(s.Body, "### Feature") {
		t.Errorf("subsections missing from body")
	}
	if strings.Contains(s.Body, "v1.18.1") {
		t.Errorf("section leaked into previous patch")
	}
	// the last section of the file runs to the last content line
	last := strings.Split(strings.TrimRight(string(md), "\n"), "\n")
	if s.EndLine != len(last) {
		t.Errorf("v1.18.0 should end at EOF line %d, got %d", len(last), s.EndLine)
	}
	// "Major Themes" ends before "Community"
	mt, err := SelectSection(md, mustRe(`^Major Themes$`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(mt.Body, "## Community") || !strings.Contains(mt.Body, "### OperatorHub Packages Discontinued") {
		t.Errorf("Major Themes body wrong: lines %d-%d", mt.StartLine, mt.EndLine)
	}
	// code fences containing "# values.yaml" comments do not create sections
	if _, err := SelectSection(md, mustRe(`^values\.yaml$`)); !errors.Is(err, ErrNoMatch) {
		t.Errorf("fenced comment became a heading: %v", err)
	}

	// Argo upgrade guide: "# v2.14 to 3.0" is the title, YAML comments in code
	// fences must not be headings
	argo := fixture(t, "argocd/2.14-3.0.md")
	bc, err := SelectSection(argo, mustRe(`^Breaking Changes$`))
	if err != nil || bc.Level != 2 {
		t.Fatalf("breaking changes: %+v err=%v", bc, err)
	}
	if strings.Contains(bc.Body, "## Other changes") {
		t.Error("Breaking Changes leaked into Other changes")
	}
	if _, err := SelectSection(argo, mustRe(`^Policies based on`)); !errors.Is(err, ErrNoMatch) {
		t.Error("YAML comment in fence matched as heading")
	}
}
