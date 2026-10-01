package ingest

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
)

// extract.labelParagraphs reaches the parser as DocInput.LabelPattern; sources
// without it get an empty pattern.
func TestIngestReleasePassesLabelPattern(t *testing.T) {
	const pattern = `^[A-Z][A-Z0-9 /&-]*:$`
	_, _, p := ingestFixture(t, "1.2.0", func(_ *world, def *catalog.ProductDefinition) {
		for i := range def.Sources {
			if def.Sources[i].ID == "site-notes" {
				def.Sources[i].Extract.LabelParagraphs = pattern
			}
		}
	})
	calls := p.notesFor("site-notes")
	if len(calls) != 1 || calls[0].LabelPattern != pattern {
		t.Fatalf("site-notes input: %+v", calls)
	}
	for _, c := range p.noteCalls {
		if c.SourceID != "site-notes" && c.LabelPattern != "" {
			t.Errorf("source %s must not get a label pattern: %q", c.SourceID, c.LabelPattern)
		}
	}
}
