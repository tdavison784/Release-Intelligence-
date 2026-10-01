package normalize

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCleanInline(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain   text\n with\tspaces", "plain text with spaces"},
		{"a [link](https://example.com/x) b", "a link b"},
		{"a [link](https://example.com/x \"title\") b", "a link b"},
		{"a [link](<https://example.com/x>) b", "a link b"},
		{"a [link](https://en.wikipedia.org/wiki/Foo_(bar)) b", "a link b"},
		{"[`#7810`](https://github.com/o/r/pull/7810)", "`#7810`"},
		{"[ref link][r] and [empty ref][] and [shortcut]", "ref link and empty ref and [shortcut]"},
		{"![alt text](img.png) after", "alt text after"},
		{"<https://example.com/auto> and <mailto:a@b.c>", "https://example.com/auto and mailto:a@b.c"},
		{"keep `code [x](y) **z**` intact", "keep `code [x](y) **z**` intact"},
		{"``double `tick` code`` here", "``double `tick` code`` here"},
		{"**Added** support, __Fixed__ too, and 2*3*4 and snake_case_name", "Added support, Fixed too, and 2*3*4 and snake_case_name"},
		{"line<br>break and <details><summary>S</summary>body</details>", "linebreak and Sbody"},
		{"a &lt;namespace&gt; &amp; b", "a <namespace> & b"},
		{`escaped \_ and \* and \[x\]`, "escaped _ and * and [x]"},
		{"an unclosed `backtick stays", "an unclosed `backtick stays"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := cleanInline(tt.in); got != tt.want {
			t.Errorf("cleanInline(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStripMarkdownAndHeadings(t *testing.T) {
	for in, want := range map[string]string{
		"[1.21][]":                 "1.21",
		"[1.12 LTS][]":             "1.12 LTS",
		"[Supported K8s][s]":       "Supported K8s",
		"**Bold**":                 "Bold",
		"`1.21`":                   "1.21",
		"_italic_":                 "italic",
		"[1.2](./notes.md#x) beta": "1.2 beta",
	} {
		if got := stripMarkdown(in); got != want {
			t.Errorf("stripMarkdown(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"`v1.18.0`":                     "v1.18.0",
		"**Feature**":                   "Feature",
		"*Feature*":                     "Feature",
		"Heading {#anchor}":             "Heading",
		"Heading {#a} {#b}":             "Heading",
		"[1.2.3](http://x/y) - 2024-01": "1.2.3 - 2024-01",
		"Default `tokenrequest` RBAC":   "Default `tokenrequest` RBAC",
		"`a` and `b`":                   "`a` and `b`",
		"  spaced   out  ":              "spaced out",
		"Using `cluster.inClusterEnabled: \"false\"`": "Using `cluster.inClusterEnabled: \"false\"`",
	} {
		if got := normalizeHeading(in); got != want {
			t.Errorf("normalizeHeading(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitTableRow(t *testing.T) {
	for in, want := range map[string][]string{
		"| a | b | c |":  {"a", "b", "c"},
		"a | b | c":      {"a", "b", "c"},
		"| a | b |":      {"a", "b"},
		"|a|b|":          {"a", "b"},
		`| a \| b | c |`: {"a | b", "c"},
		"| a |  | c |":   {"a", "", "c"},
		"| only |":       {"only"},
		"  | x |  ":      {"x"},
		"| a | b":        {"a", "b"},
		"|---|:-:|--:|":  {"---", ":-:", "--:"},
	} {
		if got := splitTableRow(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitTableRow(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncateText(t *testing.T) {
	if got := truncateText("short", 10); got != "short" {
		t.Errorf("%q", got)
	}
	long := strings.Repeat("word ", 50)
	got := truncateText(long, 100)
	if len(got) > 104 || !strings.HasSuffix(got, "…") || strings.HasSuffix(strings.TrimSuffix(got, "…"), " ") {
		t.Errorf("%q", got)
	}
	// never cuts inside a multi-byte rune
	multi := strings.Repeat("é", 100)
	got = truncateText(multi, 51)
	if !utf8.ValidString(got) {
		t.Errorf("invalid utf-8: %q", got)
	}
	// a single long token is cut hard
	if got := truncateText(strings.Repeat("x", 500), 100); len(got) > 104 {
		t.Errorf("len = %d", len(got))
	}
}

func TestScanDocumentKinds(t *testing.T) {
	md := "---\ntitle: x\n---\n<!-- c1 -->\n# H1 <!-- inline -->\n\n```go\n# not heading\n```\ntext {{< warning >}}inline{{< /warning >}}\n{{< relnote >}}\n[ref]: https://x\n<br>\n***\n{/* mdx */}\n{{< text yaml >}}\n# in shortcode\n{{< /text >}}\n    ## indented code is not a heading\n"
	d := scanDocument([]byte(md))
	want := []lineKind{
		lkFrontMatter, lkFrontMatter, lkFrontMatter, // front matter
		lkBlank,                   // comment only
		lkHeading,                 // heading with inline comment
		lkBlank,                   // blank
		lkFence, lkFence, lkFence, // fence
		lkText,  // text with inline shortcode
		lkBlank, // shortcode only
		lkBlank, // reference definition
		lkBlank, // html tag only
		lkBlank, // thematic break
		lkBlank, // mdx comment
		lkShortcodeCode, lkShortcodeCode, lkShortcodeCode,
		lkText, // 4-space indented: not a heading
	}
	if len(d.lines) != len(want) {
		t.Fatalf("lines = %d, want %d", len(d.lines), len(want))
	}
	for i, l := range d.lines {
		if l.kind != want[i] {
			t.Errorf("line %d (%q): kind %d, want %d", i+1, l.raw, l.kind, want[i])
		}
	}
	if d.lines[4].head != "H1" || d.lines[4].level != 1 {
		t.Errorf("heading: %+v", d.lines[4])
	}
	if strings.TrimSpace(d.lines[9].vis) != "text inline" {
		t.Errorf("shortcode tags should be removed from text: %q", d.lines[9].vis)
	}
}

func TestNoPanicOnMalformedInputs(t *testing.T) {
	// truncate every fixture at many offsets and feed it to the markdown and
	// YAML entry points: none may panic
	for _, f := range []string{
		"certmanager/release-notes-1.18.md", "certmanager/releases-README.md", "certmanager/upgrading-1.18-1.19.md",
		"istio/change-notes-1.31.md", "istio/upgrade-notes-1.31.md", "argocd/2.14-3.0.md", "argocd/3.4-3.5.md", "argocd/git-log.md",
	} {
		doc := fixture(t, f)
		for cut := 0; cut <= len(doc); cut += 1499 {
			piece := doc[:cut]
			_, _ = SelectSection(piece, mustRe(`.`))
			if _, _, err := ParseNotes(testInput(string(piece), "upgrade-guide"), nil); err != nil {
				t.Fatalf("%s@%d: %v", f, cut, err)
			}
			_, _ = ExtractTableRow(piece, TableSelector{KeyColumns: []string{"Release", "Argo CD version"}, KeyRe: mustRe(`.`)})
		}
	}
	for _, f := range []string{"istio/supportStatus.yml", "certmanager/values.yaml", "certmanager/crds.yaml", "certmanager/install-deployments.yaml", "istio/releasenotes/sni-dnat-default.yaml"} {
		doc := fixture(t, f)
		for cut := 0; cut <= len(doc); cut += 2999 {
			piece := doc[:cut]
			_, _ = ExtractRecord(piece, TableSelector{KeyColumns: []string{"version"}, KeyRe: mustRe(`.`)})
			_, _ = ValuesSnapshot("c", "v", piece)
			_, _ = CRDSnapshot(piece)
			_ = ImageRefs(piece)
			_, _, _ = ParseReleaseNoteYAML([]DocInput{testInput(string(piece), "release-notes")}, nil)
		}
	}
	// pathological shapes
	for _, s := range []string{
		"", "\n\n\n", "#", "# ", "-", "- ", "1.", ">", "> >", "|", "|\n|", "| a |\n|---|", "```", "~~~~\n", "<!--", "{/*", "{{<", "---\n", "---\n---\n",
		"- a\n  - b\n    - c\n      - d\n", strings.Repeat("#", 50) + " x\n", strings.Repeat("- x\n", 2000), strings.Repeat("`", 999),
	} {
		_, _ = SelectSection([]byte(s), mustRe(`.`))
		if _, _, err := ParseNotes(testInput(s, "release-notes"), nil); err != nil {
			t.Errorf("%q: %v", s, err)
		}
		_, _ = ExtractTableRow([]byte(s), TableSelector{KeyColumns: []string{"a"}, KeyRe: mustRe(`.`)})
		_ = ExtractReferences(s, "o/r")
	}
}
