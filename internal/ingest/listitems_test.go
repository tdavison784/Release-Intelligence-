package ingest

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// extract.listItems must reach the parser as DocInput.ListItems for markdown
// documents (the structural reading docbook/rst conversions always use).
func TestIngestReleaseListItemsExtract(t *testing.T) {
	guide := "### Upgrading to `1.2.0`+\n\n{{% alert title=\"Warning\" %}}\nKarpenter `1.1.0` drops the support for `v1beta1` APIs.\n{{% /alert %}}\n\n* This version adds a Balanced consolidation policy.\n* No breaking changes\n"
	rel, _, p := ingestFixture(t, "1.2.0", func(w *world, def *catalog.ProductDefinition) {
		w.docs["repo-file:"+website+"@v1.2.0:upgrading/upgrade-guide.md"] = guide
		def.Sources = append(def.Sources, catalog.Source{
			ID: "guide", Roles: []domain.SourceRole{domain.RoleUpgradeGuide},
			Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "{{.Tag}}", Path: "upgrading/upgrade-guide.md"},
			Extract: &catalog.Extract{Type: catalog.ExtractMarkdownSection, Heading: `^Upgrading to `, ListItems: true},
		})
	})
	if s := statusOf(t, rel, "guide"); s.State != domain.SourceOK {
		t.Fatalf("guide: %+v", s)
	}
	calls := p.notesFor("guide")
	if len(calls) != 1 {
		t.Fatalf("parsed %d times", len(calls))
	}
	if !calls[0].ListItems {
		t.Fatalf("ListItems not passed through: %+v", calls[0])
	}
	// the shared fixture's plain markdown-section source stays prose-folded
	for _, in := range p.notesFor("site-notes") {
		if in.ListItems {
			t.Fatalf("site-notes must not use the structural reading: %+v", in)
		}
	}
}
