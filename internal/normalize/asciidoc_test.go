package normalize

import (
	"strings"
	"testing"
)

// The fixtures mirror the AsciiDoc of Elasticsearch's per-major migration
// guides (docs/reference/migration/migrate_9_0.asciidoc and
// migrate_8_0/*.asciidoc), the motivating product of format:adoc.

func TestASCIIDocToMarkdownPrefixHeadings(t *testing.T) {
	src := strings.Join([]string{
		"[[migrating-9.0]]",
		"== Migrating to 9.0",
		"++++",
		"<titleabbrev>9.0</titleabbrev>",
		"++++",
		"",
		"This section discusses the changes for {es} 9.0.",
		"",
		"coming::[9.0.0]",
		"",
		"[discrete]",
		"[[breaking-changes-9.0]]",
		"=== Breaking changes",
		"",
		"There are no notable breaking changes in {es} 9.0.",
	}, "\n")
	out := string(ASCIIDocToMarkdown([]byte(src)))
	lines := strings.Split(out, "\n")
	want := []string{
		"",                               // [[migrating-9.0]]
		"## Migrating to 9.0",            // == Migrating to 9.0
		"",                               // ++++
		"<titleabbrev>9.0</titleabbrev>", // pass-through content
		"",                               // ++++
		"",
		"This section discusses the changes for {es} 9.0.",
		"",
		"", // coming::[9.0.0]
		"",
		"", // [discrete]
		"", // [[breaking-changes-9.0]]
		"### Breaking changes",
		"",
		"There are no notable breaking changes in {es} 9.0.",
	}
	if len(lines) != len(want) {
		t.Fatalf("line count %d, want %d:\n%s", len(lines), len(want), out)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i+1, lines[i], want[i])
		}
	}
}

func TestASCIIDocToMarkdownUnderlineHeadings(t *testing.T) {
	// AsciiDoc underline levels are fixed: "=" is 1, "-" is 2, "~" is 3.
	src := "Title\n=====\n\nSub\n-----\n\nText\n"
	out := string(ASCIIDocToMarkdown([]byte(src)))
	for _, want := range []string{"# Title", "## Sub", "Text"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// the underline lines are blanked (line-preserving)
	if lines := strings.Split(out, "\n"); lines[1] != "" || lines[4] != "" {
		t.Errorf("underlines not consumed:\n%s", out)
	}
}

func TestASCIIDocToMarkdownCollapsibleBlock(t *testing.T) {
	src := strings.Join([]string{
		"[[drop_tls_rsa_cipher_support_for_jdk_24]]",
		".Drop `TLS_RSA` cipher support for JDK 24",
		"[%collapsible]",
		"====",
		"*Details* +",
		"This change removes `TLS_RSA` ciphers for Elasticsearch deployments running on JDK 24.",
		"",
		"*Impact* +",
		"TLS connections using these ciphers will no longer work. See <<deprecation-logging,deprecation logging>>.",
		"====",
	}, "\n")
	out := string(ASCIIDocToMarkdown([]byte(src)))
	for _, want := range []string{
		"**Drop `TLS_RSA` cipher support for JDK 24**",
		"*Details*",
		"This change removes `TLS_RSA` ciphers",
		"*Impact*",
		"See deprecation logging.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, " +") {
		t.Errorf("hard line breaks not stripped:\n%s", out)
	}
	if strings.Contains(out, "[[drop_") || strings.Contains(out, "[%collapsible]") || strings.Contains(out, "====") {
		t.Errorf("structural lines not blanked:\n%s", out)
	}
}

func TestASCIIDocToMarkdownCommentsIncludesTags(t *testing.T) {
	src := strings.Join([]string{
		"//NOTE: The notable-breaking-changes tagged regions are re-used",
		"//tag::notable-breaking-changes[]",
		".Some title",
		"====",
		"Body text.",
		"====",
		"//end::notable-breaking-changes[]",
		"include::migrate_8_0/rest-api-changes.asciidoc[]",
		"ifdef::enterprise[]",
		"endif::[]",
	}, "\n")
	out := string(ASCIIDocToMarkdown([]byte(src)))
	if strings.Contains(out, "//") || strings.Contains(out, "include::") || strings.Contains(out, "ifdef") {
		t.Errorf("comments/macros not blanked:\n%s", out)
	}
	if !strings.Contains(out, "**Some title**") || !strings.Contains(out, "Body text.") {
		t.Errorf("content lost:\n%s", out)
	}
}

func TestASCIIDocToMarkdownPipeTable(t *testing.T) {
	src := strings.Join([]string{
		`[options="header",cols="<1,<3,<1"]`,
		`|====`,
		`| API   | Typed API endpoint  | Typeless API endpoint`,
		``,
		`| {ref}/docs-bulk.html[Bulk]`,
		"| `<target>/<type>/_bulk`",
		"| `<target>/_bulk`",
		``,
		`| {ref}/search-count.html[Count]`,
		"| `<target>/<type>/_count`",
		"| `<target>/_count`",
		``,
		`|====`,
	}, "\n")
	out := string(ASCIIDocToMarkdown([]byte(src)))
	lines := strings.Split(out, "\n")
	// line 2 (header), line 3 (separator on the blank), line 4 (joined first row)
	if lines[2] != "| API | Typed API endpoint | Typeless API endpoint |" {
		t.Errorf("header row = %q", lines[2])
	}
	if lines[3] != "|---|---|---|" {
		t.Errorf("separator = %q", lines[3])
	}
	if lines[4] != "| [Bulk]({ref}/docs-bulk.html) | `<target>/<type>/_bulk` | `<target>/_bulk` |" {
		t.Errorf("data row = %q", lines[4])
	}
	if lines[8] != "| [Count]({ref}/search-count.html) | `<target>/<type>/_count` | `<target>/_count` |" {
		t.Errorf("second data row = %q", lines[8])
	}
	if strings.Contains(out, "|====") {
		t.Errorf("borders not blanked:\n%s", out)
	}
}

func TestASCIIDocToMarkdownDefinitionLists(t *testing.T) {
	// the AsciiDoc-flavored definition lists of the .md release-notes export
	src := strings.Join([]string{
		"Allocation",
		":   * Increase minimum threshold in shard balancer [#115831](https://github.com/elastic/elasticsearch/pull/115831)",
		"* Remove `cluster.routing.allocation.disk.watermark.enable_for_single_data_node` setting [#114207](url)",
	}, "\n")
	out := string(ASCIIDocToMarkdown([]byte(src)))
	lines := strings.Split(out, "\n")
	if lines[1] != "* Increase minimum threshold in shard balancer [#115831](https://github.com/elastic/elasticsearch/pull/115831)" {
		t.Errorf("definition entry = %q", lines[1])
	}
	if lines[2] != "* Remove `cluster.routing.allocation.disk.watermark.enable_for_single_data_node` setting [#114207](url)" {
		t.Errorf("plain bullet changed = %q", lines[2])
	}
}

func TestASCIIDocToMarkdownFloatBlocks(t *testing.T) {
	// 7.0-era style: [float] blocks with ==== topic headings
	src := strings.Join([]string{
		"[float]",
		"[[breaking_70_analysis_changes]]",
		"=== Analysis changes",
		"",
		"[float]",
		"==== `standard` filter has been removed",
		"",
		"The `standard` token filter has been removed because it doesn't change anything in the stream.",
	}, "\n")
	out := string(ASCIIDocToMarkdown([]byte(src)))
	for _, want := range []string{"### Analysis changes", "#### `standard` filter has been removed", "The `standard` token filter has been removed"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestASCIIDocToMarkdownInlineMacros(t *testing.T) {
	src := "See https://www.elastic.co/guide[the guide] and kbd:[Ctrl+C] and pass:[*] and btn:[Save] today."
	out := string(ASCIIDocToMarkdown([]byte(src)))
	for _, want := range []string{"[the guide](https://www.elastic.co/guide)", "Ctrl+C", "*", "Save"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, banned := range []string{"pass:[", "kbd:[", "btn:["} {
		if strings.Contains(out, banned) {
			t.Errorf("%s not flattened:\n%s", banned, out)
		}
	}
}

func TestASCIIDocToMarkdownLinePreserving(t *testing.T) {
	// every input line keeps its index: evidence locators stay valid
	src := strings.Join([]string{
		"== Heading",
		"",
		"* first",
		"[discrete]",
		"* second",
	}, "\n")
	lines := strings.Split(string(ASCIIDocToMarkdown([]byte(src))), "\n")
	if lines[0] != "## Heading" || lines[2] != "* first" || lines[3] != "" || lines[4] != "* second" {
		t.Errorf("not line-preserving: %q", lines)
	}
}
