package ingest

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// A source with extract.format html is rendered as markdown before the notes
// are parsed, in list-item mode. The fixture mirrors Go's published notes
// pages (go.dev/doc/go1.N) and the in-tree doc/go1.N.html copies.
func TestIngestReleaseHTMLFormat(t *testing.T) {
	const page = `<!DOCTYPE html>
<html><head><title>Go 1.27 Release Notes</title></head>
<body>
<h2 id="introduction">Introduction to Go 1.27</h2>
<p>The latest Go release, version 1.27, arrives six months after Go 1.26.</p>
<h2 id="library">Standard library</h2>
<h3 id="uuid">New uuid package</h3>
<p>Go 1.27 adds the new <a href="/pkg/uuid/"><code>uuid</code></a> package.</p>
<dl id="bytes"><dt><a href="/pkg/bytes/">bytes</a></dt>
  <dd>
    <p>The <code>Buffer</code> type now has a <code>Clear</code> method.</p>
  </dd>
</dl>
</body>
</html>
`
	rel, w, p := ingestFixture(t, "1.2.0", func(w *world, def *catalog.ProductDefinition) {
		w.docs["repo-file:"+website+"@v1.2.0:doc/go1.27.html"] = page
		def.Sources = append(def.Sources, catalog.Source{ID: "release-notes", Roles: []domain.SourceRole{domain.RoleReleaseNotes},
			Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "{{.Tag}}", Path: "doc/go1.27.html"},
			Extract: &catalog.Extract{Format: catalog.FormatHTML, Type: catalog.ExtractWhole}})
	})
	_ = rel
	s := statusOf(t, rel, "release-notes")
	if s.State != domain.SourceOK {
		t.Fatalf("release-notes: %+v", s)
	}
	calls := p.notesFor("release-notes")
	if len(calls) != 1 {
		t.Fatalf("expected one ParseNotes call, got %d", len(calls))
	}
	in := calls[0]
	if !in.ListItems {
		t.Error("html content must be parsed in list-item mode")
	}
	body := string(in.Content)
	for _, want := range []string{
		"## Introduction to Go 1.27",
		"## Standard library",
		"### New uuid package",
		"Go 1.27 adds the new [`uuid`](/pkg/uuid/) package.",
		"**[bytes](/pkg/bytes/)**",
		"The `Buffer` type now has a `Clear` method.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	for _, banned := range []string{"<h2", "<code>", "</p>", "<title>", "uuid</a>"} {
		if strings.Contains(body, banned) {
			t.Errorf("%q not converted:\n%s", banned, body)
		}
	}
	// the memoised document of other sources is not converted in place
	if !strings.Contains(w.docs["repo-file:"+website+"@v1.2.0:doc/go1.27.html"], "<h2") {
		t.Error("source document must stay HTML")
	}
}

func TestValidateHTMLFormat(t *testing.T) {
	def := testDef()
	def.Sources = append(def.Sources, catalog.Source{ID: "html-bad", Roles: []domain.SourceRole{domain.RoleCompatibility},
		Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "main", Path: "x.html"},
		Extract: &catalog.Extract{Format: catalog.FormatHTML, Type: catalog.ExtractMarkdownTable,
			KeyColumns: []string{"v"}, KeyMatch: "^1$", Columns: []catalog.ColumnSpec{{Platform: "p", Headers: []string{"h"}}}}})
	rep := catalog.Validate(def)
	found := false
	for _, i := range rep.Errors() {
		if strings.HasSuffix(i.Path, "extract.format") {
			found = true
		}
	}
	if !found {
		t.Fatalf("html with markdown-table must be rejected: %v", rep.Issues)
	}
	// whole and markdown-section are fine for html
	def.Sources[len(def.Sources)-1].Extract = &catalog.Extract{Format: catalog.FormatHTML, Type: catalog.ExtractMarkdownSection, Heading: "^Go "}
	if rep := catalog.Validate(def); !rep.OK() {
		t.Fatalf("html with markdown-section must validate: %v", rep.Issues)
	}
	// the converter is exposed through the normalize API used by ingest
	if !strings.Contains(string(normalize.HTMLToMarkdown([]byte("<h3>Go</h3>"))), "### Go") {
		t.Error("HTMLToMarkdown not wired")
	}
}
