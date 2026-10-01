package normalize

// Contract stubs. Replace each with a real implementation (and delete it from
// this file) — see api.go for the documented behaviour.

import (
	"regexp"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

func selectSection(md []byte, re *regexp.Regexp) (*Section, error) { return nil, ErrNotImplemented }

func parseNotes(in DocInput, rules []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error) {
	return nil, nil, ErrNotImplemented
}

func parseReleaseNoteYAML(files []DocInput, rules []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error) {
	return nil, nil, ErrNotImplemented
}

func extractTableRow(md []byte, sel TableSelector) (*TableRow, error) {
	return nil, ErrNotImplemented
}

func compatibilityFromRow(in DocInput, row *TableRow, columns []catalog.ColumnSpec) ([]domain.CompatibilityConstraint, []domain.Evidence) {
	return nil, nil
}

func parseVersionRange(raw string) (string, []string, error) { return "", nil, ErrNotImplemented }

func parseChartMetadata(chartYAML []byte) (*ChartMetadata, error) { return nil, ErrNotImplemented }

func valuesSnapshot(chart, version string, valuesYAML []byte) (*domain.ValuesSnapshot, error) {
	return nil, ErrNotImplemented
}

func crdSnapshot(streams ...[]byte) (*domain.CRDSnapshot, error) { return nil, ErrNotImplemented }

func imageRefs(content []byte) []domain.ImageRef { return nil }

func parseImageRef(s string) (domain.ImageRef, error) { return domain.ImageRef{}, ErrNotImplemented }

func extractReferences(text, repository string) []domain.Reference { return nil }
