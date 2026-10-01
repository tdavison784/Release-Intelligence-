package ingest

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// A source with extract.format docbook is rendered as markdown before the
// section is selected, and its notes are parsed in list-item mode.
func TestIngestReleaseDocBookFormat(t *testing.T) {
	const sgml = ` <sect1 id="release-1-1-0">
  <title>Release 1.1.0</title>
  <sect2>
   <title>Changes</title>
   <itemizedlist>
    <listitem>
     <para>Fix <literal>vacuum</literal> crash</para>
    </listitem>
   </itemizedlist>
  </sect2>
 </sect1>

 <sect1 id="release-1-0-0">
  <title>Release 1.0.0</title>
 </sect1>
`
	rel, w, p := ingestFixture(t, "1.1.0", func(w *world, def *catalog.ProductDefinition) {
		w.docs[websiteKey("doc/release-1.sgml")] = sgml
		def.Sources = append(def.Sources, catalog.Source{ID: "sgml-notes", Roles: []domain.SourceRole{domain.RoleChangelog},
			Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "main", Path: "doc/release-{{.Major}}.sgml"},
			Extract: &catalog.Extract{Format: catalog.FormatDocBook, Type: catalog.ExtractMarkdownSection, Heading: `^Release {{regexQuote .Version}}$`}})
	})
	_ = w
	s := statusOf(t, rel, "sgml-notes")
	if s.State != domain.SourceOK || !strings.Contains(s.Detail, `section "Release 1.1.0"`) {
		t.Fatalf("sgml-notes: %+v", s)
	}
	calls := p.notesFor("sgml-notes")
	if len(calls) != 1 {
		t.Fatalf("expected one ParseNotes call, got %d", len(calls))
	}
	in := calls[0]
	if !in.ListItems {
		t.Error("docbook content must be parsed in list-item mode")
	}
	body := string(in.Content)
	if !strings.HasPrefix(body, "# Release 1.1.0") || !strings.Contains(body, "- Fix `vacuum` crash") || strings.Contains(body, "<") {
		t.Errorf("content was not rendered as markdown:\n%s", body)
	}
	if in.LineOffset != 1 {
		t.Errorf("line offset = %d, want 1 (the heading is line 2 of the SGML)", in.LineOffset)
	}
	// the memoised document of other sources is not converted in place
	if !strings.Contains(w.docs[websiteKey("doc/release-1.sgml")], "<sect1") {
		t.Error("source document must stay SGML")
	}
}

func TestValidateDocBookFormat(t *testing.T) {
	def := testDef()
	def.Sources = append(def.Sources, catalog.Source{ID: "sgml-bad", Roles: []domain.SourceRole{domain.RoleCompatibility},
		Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "main", Path: "x.sgml"},
		Extract: &catalog.Extract{Format: catalog.FormatDocBook, Type: catalog.ExtractMarkdownTable, KeyColumns: []string{"v"}, KeyMatch: "^1$",
			Columns: []catalog.ColumnSpec{{Platform: "p", Headers: []string{"h"}}}}})
	rep := catalog.Validate(def)
	found := false
	for _, i := range rep.Errors() {
		if strings.HasSuffix(i.Path, "extract.format") {
			found = true
		}
	}
	if !found {
		t.Fatalf("docbook with markdown-table must be rejected: %v", rep.Issues)
	}
	def.Sources[len(def.Sources)-1].Extract = &catalog.Extract{Format: "asciidoc", Type: catalog.ExtractWhole}
	rep = catalog.Validate(def)
	found = false
	for _, i := range rep.Errors() {
		if strings.HasSuffix(i.Path, "extract.format") {
			found = true
		}
	}
	if !found {
		t.Fatalf("unknown format must be rejected: %v", rep.Issues)
	}
}
