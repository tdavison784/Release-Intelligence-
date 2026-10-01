// Package normalize turns raw source bytes for ONE release into domain
// objects: classified release-note items, compatibility constraints and
// structured snapshots (Helm values, CRDs, image references).
//
// Everything here is deterministic and pure (no I/O). Cross-release reasoning
// (diffs, paths) lives in package upgrade.
//
// CONTRACT NOTE: the exported signatures in this file are the contract used by
// packages ingest and upgrade. Implementations may live in other files of this
// package; signatures must not change without updating the callers.
package normalize

import (
	"errors"
	"regexp"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ErrNotImplemented is returned by contract stubs not yet implemented.
var ErrNotImplemented = errors.New("normalize: not implemented")

// ErrNoMatch is returned when a section/row/table selector matches nothing.
var ErrNoMatch = errors.New("normalize: no match")

// Producer identifiers recorded in provenance / fact extractors.
const (
	ProducerNotes  = "normalize.notes@v1"
	ProducerTable  = "normalize.table@v1"
	ProducerHelm   = "normalize.helm@v1"
	ProducerCRD    = "normalize.crd@v1"
	ProducerImages = "normalize.images@v1"
)

// DocInput identifies a document (or a section of one) being normalised, so
// that every produced item can carry precise evidence.
type DocInput struct {
	SourceID    string
	Role        domain.SourceRole
	Release     string // semver of the release the document is about
	URI         string // human-facing URI of the whole document
	Digest      string // digest of the whole document
	RetrievedAt time.Time
	Content     []byte
	// LineOffset is added to line numbers when Content is a slice of a larger
	// document (e.g. a selected section), so locators point into the original.
	LineOffset int
	// Repository ("owner/name" on github.com, or host/owner/name) used to
	// build URLs for bare "#1234" references; optional.
	Repository string
}

// Section is a markdown section.
type Section struct {
	Heading   string   // heading text without leading #'s
	Level     int      // 1..6
	Path      []string // heading hierarchy including this heading
	StartLine int      // 1-based line of the heading
	EndLine   int      // 1-based last line of the section (inclusive)
	Body      string   // full text of the section including the heading line and subsections
}

// SelectSection returns the first section whose heading matches re, including
// its subsections. Returns ErrNoMatch when none matches.
func SelectSection(md []byte, re *regexp.Regexp) (*Section, error) {
	return selectSection(md, re)
}

// ParseNotes splits markdown release notes / changelog / upgrade-guide content
// into atomic items (bullets and paragraphs under headings) and classifies
// each one. Product-specific rules are evaluated first, then the generic
// defaults. Items matched via a section heading or an upstream label get
// MethodDeclared; keyword-based classification gets MethodHeuristic.
// Each item gets exactly one Evidence pointing at its line range.
func ParseNotes(in DocInput, rules []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error) {
	return parseNotes(in, rules)
}

// ParseReleaseNoteYAML parses structured release-note YAML files (one
// document per input, Istio "releasenotes/notes/*.yaml" style: kind, area,
// releaseNotes, upgradeNotes, securityNotes, ...).
func ParseReleaseNoteYAML(files []DocInput, rules []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error) {
	return parseReleaseNoteYAML(files, rules)
}

// TableRow is a selected markdown table row.
type TableRow struct {
	Headers []string          // all header cells (markdown stripped)
	Cells   map[string]string // header (markdown stripped) → raw cell value for the selected row
	Line    int               // 1-based line in the document (before LineOffset)
	Excerpt string            // header line + row line, verbatim
}

// TableSelector selects a row from markdown tables.
type TableSelector struct {
	// KeyColumns are alternative names of the key column (case-insensitive,
	// markdown links stripped). Every table having one is searched in order.
	KeyColumns []string
	// KeyRe is matched against the key cell after stripping markdown
	// links/emphasis (e.g. "[1.21][]" → "1.21").
	KeyRe *regexp.Regexp
	// TableHeading, when set, restricts the search to tables under a
	// heading matching it.
	TableHeading *regexp.Regexp
}

// ExtractTableRow returns the first row (across all qualifying tables) whose
// key cell matches. Returns ErrNoMatch when none does.
func ExtractTableRow(md []byte, sel TableSelector) (*TableRow, error) {
	return extractTableRow(md, sel)
}

// ExtractRecord is the YAML/JSON counterpart of ExtractTableRow: content is
// a list of mappings (or a mapping whose first list-valued field holds them);
// KeyColumns are field names. Headers/Cells use field names; Line is the
// record's line; Excerpt is the record's YAML. Scalars in list-valued fields
// are joined with ", ".
func ExtractRecord(content []byte, sel TableSelector) (*TableRow, error) {
	return extractRecord(content, sel)
}

// CompatibilityFromRow converts the selected columns of a table row into
// constraints with evidence (one Evidence for the row, shared). Cells are
// split by ColumnSpec.Separator when set and parsed with ParseVersionRange;
// unparsable values are kept verbatim in Raw with an empty Constraint.
// Provenance is MethodDeclared.
func CompatibilityFromRow(in DocInput, row *TableRow, columns []catalog.ColumnSpec) ([]domain.CompatibilityConstraint, []domain.Evidence) {
	return compatibilityFromRow(in, row, columns)
}

// ParseVersionRange parses human version ranges as they appear in support
// matrices and charts: "1.29 → 1.33", "1.22 - 1.27", "1.29, 1.30, 1.31",
// "1.31-1.33", ">= 1.22.0-0", "4.14 to 4.18". It returns a semver constraint
// string and, when the source enumerates versions, the explicit list.
func ParseVersionRange(raw string) (constraint string, versions []string, err error) {
	return parseVersionRange(raw)
}

// ChartMetadata is the subset of Chart.yaml we use.
type ChartMetadata struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	AppVersion  string `json:"appVersion,omitempty"`
	KubeVersion string `json:"kubeVersion,omitempty"`
}

// ParseChartMetadata parses Chart.yaml.
func ParseChartMetadata(chartYAML []byte) (*ChartMetadata, error) {
	return parseChartMetadata(chartYAML)
}

// ValuesSnapshot flattens a Helm values.yaml into dotted paths with JSON
// encoded leaf values (lists are leaves), keeping the comment directly above
// each key when present.
func ValuesSnapshot(chart, version string, valuesYAML []byte) (*domain.ValuesSnapshot, error) {
	return valuesSnapshot(chart, version, valuesYAML)
}

// CRDSnapshot summarises CustomResourceDefinitions found in one or more YAML
// streams (multi-document; non-CRD documents are ignored).
func CRDSnapshot(streams ...[]byte) (*domain.CRDSnapshot, error) {
	return crdSnapshot(streams...)
}

// ImageRefs extracts container image references from manifests or values
// (lines like `image: repo:tag`, `image: "repo@sha256:..."`), de-duplicated
// and sorted.
func ImageRefs(content []byte) []domain.ImageRef {
	return imageRefs(content)
}

// ParseImageRef parses "registry/repo[:tag][@digest]".
func ParseImageRef(s string) (domain.ImageRef, error) {
	return parseImageRef(s)
}

// ExtractReferences finds CVE ids, GHSA ids, and pull-request / issue
// references (GitHub URLs or "#1234" when repository is given).
func ExtractReferences(text, repository string) []domain.Reference {
	return extractReferences(text, repository)
}
