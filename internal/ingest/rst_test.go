package ingest

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// A source with extract.format rst is rendered as markdown before the
// section is selected, and its notes are parsed in list-item mode (every
// bullet is one item; rst grids become pipe tables for markdown-table).
func TestIngestReleaseRSTFormat(t *testing.T) {
	rst := strings.Join([]string{
		".. _upgrade:",
		"",
		"1.2 Upgrade Notes",
		"-----------------",
		"",
		"Action Required",
		"~~~~~~~~~~~~~~~",
		"",
		"If you are using the following features, read carefully.",
		"",
		"* The ``v2alpha1`` CRD version is deprecated. Use ``cilium.io/v2``.",
		"",
		"Removed Options",
		"###############",
		"",
		"* The ``--k8s-api-server`` agent flag has been removed.",
		"",
	}, "\n")
	rel, w, p := ingestFixture(t, "1.2.0", func(w *world, def *catalog.ProductDefinition) {
		w.docs["repo-file:"+website+"@v1.2.0:Documentation/operations/upgrade.rst"] = rst
		def.Sources = append(def.Sources, catalog.Source{ID: "upgrade-notes", Roles: []domain.SourceRole{domain.RoleUpgradeGuide},
			Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "{{.Tag}}", Path: "Documentation/operations/upgrade.rst"},
			Extract: &catalog.Extract{Format: catalog.FormatRST, Type: catalog.ExtractMarkdownSection, Heading: `^{{.Major}}.{{.Minor}} Upgrade Notes$`}})
	})
	_ = w
	s := statusOf(t, rel, "upgrade-notes")
	if s.State != domain.SourceOK || !strings.Contains(s.Detail, `section "1.2 Upgrade Notes"`) {
		t.Fatalf("upgrade-notes: %+v", s)
	}
	calls := p.notesFor("upgrade-notes")
	if len(calls) != 1 {
		t.Fatalf("expected one ParseNotes call, got %d", len(calls))
	}
	in := calls[0]
	if !in.ListItems {
		t.Error("rst content must be parsed in list-item mode")
	}
	body := string(in.Content)
	if !strings.Contains(body, "## Action Required") || !strings.Contains(body, "* The `v2alpha1` CRD version is deprecated.") {
		t.Errorf("content was not rendered as markdown:\n%s", body)
	}
	// the memoised document of other sources is not converted in place
	if !strings.Contains(w.docs["repo-file:"+website+"@v1.2.0:Documentation/operations/upgrade.rst"], "~~~~") {
		t.Error("source document must stay rst")
	}
}

// rst + markdown-table reads the grid compatibility table of the converted
// document (e.g. Cilium's compatibility.rst).
func TestIngestReleaseRSTCompatibility(t *testing.T) {
	const rst = `Kubernetes Compatibility
========================

+----------------+------------+
| k8s Version    | Policy API |
+----------------+------------+
| 1.29, 1.30     | v1         |
+----------------+------------+
`
	rel, _, _ := ingestFixture(t, "1.2.0", func(w *world, def *catalog.ProductDefinition) {
		w.docs["repo-file:"+website+"@v1.2.0:compatibility.rst"] = rst
		def.Sources = append(def.Sources, catalog.Source{ID: "compat", Roles: []domain.SourceRole{domain.RoleCompatibility},
			Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "{{.Tag}}", Path: "compatibility.rst"},
			Extract: &catalog.Extract{Format: catalog.FormatRST, Type: catalog.ExtractMarkdownTable,
				KeyColumns: []string{"k8s Version"}, KeyMatch: ".",
				Columns: []catalog.ColumnSpec{{Platform: "kubernetes", Kind: "tested", Headers: []string{"k8s Version"}}}}})
	})
	s := statusOf(t, rel, "compat")
	if s.State != domain.SourceOK {
		t.Fatalf("compat: %+v", s)
	}
	found := false
	for _, cc := range rel.Compat {
		if cc.SourceID == "compat" && cc.Platform == "kubernetes" && cc.Raw == "1.29, 1.30" && cc.Kind == "tested" {
			found = true
		}
	}
	if !found {
		t.Fatalf("compatibility not extracted: %+v", rel.Compat)
	}
}

func TestValidateRSTFormat(t *testing.T) {
	def := testDef()
	def.Sources = append(def.Sources, catalog.Source{ID: "rst-bad", Roles: []domain.SourceRole{domain.RoleCompatibility},
		Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "main", Path: "x.rst"},
		Extract: &catalog.Extract{Format: catalog.FormatRST, Type: catalog.ExtractReleaseNoteYAML}})
	rep := catalog.Validate(def)
	found := false
	for _, i := range rep.Errors() {
		if strings.HasSuffix(i.Path, "extract.format") {
			found = true
		}
	}
	if !found {
		t.Fatalf("rst with release-note-yaml must be rejected: %v", rep.Issues)
	}
	// markdown-table is fine for rst (grid tables are converted)
	def.Sources[len(def.Sources)-1].Extract = &catalog.Extract{Format: catalog.FormatRST, Type: catalog.ExtractMarkdownTable,
		KeyColumns: []string{"v"}, KeyMatch: "^1$", Columns: []catalog.ColumnSpec{{Platform: "p", Headers: []string{"h"}}}}
	if rep := catalog.Validate(def); !rep.OK() {
		t.Fatalf("rst with markdown-table must validate: %v", rep.Issues)
	}
	// the converter is exposed through the normalize API used by ingest
	if !strings.Contains(string(normalize.RSTToMarkdown([]byte("Title\n=====\n"))), "# Title") {
		t.Error("RSTToMarkdown not wired")
	}
}
