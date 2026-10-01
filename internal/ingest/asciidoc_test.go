package ingest

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// A source with extract.format adoc is rendered as markdown before the notes
// are parsed, in list-item mode (every bullet and topic paragraph is one
// item). The fixture mirrors Elasticsearch's migrate_9_0.asciidoc.
func TestIngestReleaseAsciiDocFormat(t *testing.T) {
	const adoc = `[[migrating-9.0]]
== Migrating to 9.0

coming::[9.0.0]

[discrete]
[[breaking-changes-9.0]]
=== Breaking changes

[[drop_tls_rsa_cipher_support_for_jdk_24]]
.Drop ` + "`TLS_RSA`" + ` cipher support for JDK 24
[%collapsible]
====
*Details* +
This change removes ` + "`TLS_RSA`" + ` ciphers.

*Impact* +
TLS connections using these ciphers will no longer work.
====

[discrete]
[[deprecated-9.0]]
=== Deprecations

* Behavioral Analytics CRUD APIs are deprecated and will be removed.
`
	rel, w, p := ingestFixture(t, "1.2.0", func(w *world, def *catalog.ProductDefinition) {
		w.docs["repo-file:"+website+"@v1.2.0:docs/reference/migration/migrate_9_0.asciidoc"] = adoc
		def.Sources = append(def.Sources, catalog.Source{ID: "migration-9.0", Roles: []domain.SourceRole{domain.RoleUpgradeGuide},
			Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "{{.Tag}}", Path: "docs/reference/migration/migrate_9_0.asciidoc"},
			Extract: &catalog.Extract{Format: catalog.FormatAsciiDoc, Type: catalog.ExtractWhole}})
	})
	_ = w
	s := statusOf(t, rel, "migration-9.0")
	if s.State != domain.SourceOK {
		t.Fatalf("migration-9.0: %+v", s)
	}
	calls := p.notesFor("migration-9.0")
	if len(calls) != 1 {
		t.Fatalf("expected one ParseNotes call, got %d", len(calls))
	}
	in := calls[0]
	if !in.ListItems {
		t.Error("adoc content must be parsed in list-item mode")
	}
	body := string(in.Content)
	for _, want := range []string{
		"## Migrating to 9.0",
		"### Breaking changes",
		"### Deprecations",
		"**Drop `TLS_RSA` cipher support for JDK 24**",
		"TLS connections using these ciphers will no longer work.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	for _, banned := range []string{"[[", "[discrete]", "====", "coming::", " +"} {
		if strings.Contains(body, banned) {
			t.Errorf("%q not converted:\n%s", banned, body)
		}
	}
	// the memoised document of other sources is not converted in place
	if !strings.Contains(w.docs["repo-file:"+website+"@v1.2.0:docs/reference/migration/migrate_9_0.asciidoc"], "[%collapsible]") {
		t.Error("source document must stay AsciiDoc")
	}
}

func TestValidateAsciiDocFormat(t *testing.T) {
	def := testDef()
	def.Sources = append(def.Sources, catalog.Source{ID: "adoc-bad", Roles: []domain.SourceRole{domain.RoleCompatibility},
		Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "main", Path: "x.adoc"},
		Extract: &catalog.Extract{Format: catalog.FormatAsciiDoc, Type: catalog.ExtractReleaseNoteYAML}})
	rep := catalog.Validate(def)
	found := false
	for _, i := range rep.Errors() {
		if strings.HasSuffix(i.Path, "extract.format") {
			found = true
		}
	}
	if !found {
		t.Fatalf("adoc with release-note-yaml must be rejected: %v", rep.Issues)
	}
	// whole and markdown-table are fine for adoc (pipe tables are converted)
	def.Sources[len(def.Sources)-1].Extract = &catalog.Extract{Format: catalog.FormatAsciiDoc, Type: catalog.ExtractMarkdownTable,
		KeyColumns: []string{"v"}, KeyMatch: "^1$", Columns: []catalog.ColumnSpec{{Platform: "p", Headers: []string{"h"}}}}
	if rep := catalog.Validate(def); !rep.OK() {
		t.Fatalf("adoc with markdown-table must validate: %v", rep.Issues)
	}
	// the converter is exposed through the normalize API used by ingest
	if !strings.Contains(string(normalize.ASCIIDocToMarkdown([]byte("== Title\n"))), "## Title") {
		t.Error("ASCIIDocToMarkdown not wired")
	}
}
