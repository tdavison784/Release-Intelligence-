package catalog

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestValidateLabelParagraphs(t *testing.T) {
	mk := func(e Extract) *ProductDefinition {
		return &ProductDefinition{
			APIVersion: APIVersion, Kind: Kind, ID: "x", Name: "x",
			Versioning: Versioning{Scheme: "semver"},
			Sources: []Source{
				{ID: "tags", Roles: []domain.SourceRole{domain.RoleVersions},
					Locator: Locator{Kind: LocatorGitTags, Repository: "github.com/o/r"}},
				{ID: "notes", Roles: []domain.SourceRole{domain.RoleReleaseNotes},
					Locator: Locator{Kind: LocatorRepoFile, Repository: "github.com/o/r", Path: "CHANGELOG.md"}, Extract: &e},
			},
		}
	}
	has := func(rep ValidationReport, path string) bool {
		for _, i := range rep.Errors() {
			if i.Path == path {
				return true
			}
		}
		return false
	}
	if rep := Validate(mk(Extract{Type: ExtractMarkdownSection, Heading: "^1$", LabelParagraphs: `^[A-Z ]+:$`})); !rep.OK() {
		t.Fatalf("valid labelParagraphs rejected: %v", rep.Issues)
	}
	if rep := Validate(mk(Extract{Type: ExtractWhole, LabelParagraphs: `^[A-Z ]+:$`})); !rep.OK() {
		t.Fatalf("labelParagraphs on a whole extract rejected: %v", rep.Issues)
	}
	if rep := Validate(mk(Extract{Type: ExtractMarkdownSection, Heading: "^1$", LabelParagraphs: `[`})); !has(rep, "sources[1].extract.labelParagraphs") {
		t.Errorf("invalid regex not reported: %v", rep.Issues)
	}
	rep := Validate(mk(Extract{Type: ExtractMarkdownTable, KeyColumns: []string{"a"}, KeyMatch: "x",
		Columns: []ColumnSpec{{Platform: "kubernetes", Headers: []string{"k"}}}, LabelParagraphs: `^X:$`}))
	if !has(rep, "sources[1].extract.labelParagraphs") {
		t.Errorf("labelParagraphs on a table extract not reported: %v", rep.Issues)
	}
}
