package ingest

import (
	"regexp"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// Parser is the set of pure parsing functions the pipeline relies on. It
// mirrors the corresponding functions of package normalize one to one and
// exists so that the pipeline can be tested with a fake parser.
type Parser interface {
	SelectSection(md []byte, re *regexp.Regexp) (*normalize.Section, error)
	ParseNotes(in normalize.DocInput, rules []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error)
	ParseReleaseNoteYAML(files []normalize.DocInput, rules []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error)
	ExtractTableRow(md []byte, sel normalize.TableSelector) (*normalize.TableRow, error)
	ExtractRecord(content []byte, sel normalize.TableSelector) (*normalize.TableRow, error)
	CompatibilityFromRow(in normalize.DocInput, row *normalize.TableRow, columns []catalog.ColumnSpec) ([]domain.CompatibilityConstraint, []domain.Evidence)
	ParseVersionRange(raw string) (constraint string, versions []string, err error)
	ReadYAMLPath(content []byte, path string) (value string, line int, err error)
	ParseChartMetadata(chartYAML []byte) (*normalize.ChartMetadata, error)
	ValuesSnapshot(chart, version string, valuesYAML []byte) (*domain.ValuesSnapshot, error)
	CRDSnapshot(streams ...[]byte) (*domain.CRDSnapshot, error)
	ImageRefs(content []byte) []domain.ImageRef
}

// DefaultParser delegates every call to package normalize.
var DefaultParser Parser = normalizeParser{}

type normalizeParser struct{}

func (normalizeParser) SelectSection(md []byte, re *regexp.Regexp) (*normalize.Section, error) {
	return normalize.SelectSection(md, re)
}

func (normalizeParser) ParseNotes(in normalize.DocInput, rules []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error) {
	return normalize.ParseNotes(in, rules)
}

func (normalizeParser) ParseReleaseNoteYAML(files []normalize.DocInput, rules []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error) {
	return normalize.ParseReleaseNoteYAML(files, rules)
}

func (normalizeParser) ExtractTableRow(md []byte, sel normalize.TableSelector) (*normalize.TableRow, error) {
	return normalize.ExtractTableRow(md, sel)
}

func (normalizeParser) ExtractRecord(content []byte, sel normalize.TableSelector) (*normalize.TableRow, error) {
	return normalize.ExtractRecord(content, sel)
}

func (normalizeParser) CompatibilityFromRow(in normalize.DocInput, row *normalize.TableRow, columns []catalog.ColumnSpec) ([]domain.CompatibilityConstraint, []domain.Evidence) {
	return normalize.CompatibilityFromRow(in, row, columns)
}

func (normalizeParser) ParseVersionRange(raw string) (string, []string, error) {
	return normalize.ParseVersionRange(raw)
}

func (normalizeParser) ReadYAMLPath(content []byte, path string) (string, int, error) {
	return normalize.ReadYAMLPath(content, path)
}

func (normalizeParser) ParseChartMetadata(chartYAML []byte) (*normalize.ChartMetadata, error) {
	return normalize.ParseChartMetadata(chartYAML)
}

func (normalizeParser) ValuesSnapshot(chart, version string, valuesYAML []byte) (*domain.ValuesSnapshot, error) {
	return normalize.ValuesSnapshot(chart, version, valuesYAML)
}

func (normalizeParser) CRDSnapshot(streams ...[]byte) (*domain.CRDSnapshot, error) {
	return normalize.CRDSnapshot(streams...)
}

func (normalizeParser) ImageRefs(content []byte) []domain.ImageRef {
	return normalize.ImageRefs(content)
}
