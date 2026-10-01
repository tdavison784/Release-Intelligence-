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
//
// Only ATX headings ("#".."######") count; lines inside fenced code blocks
// (``` or ~~~), Hugo {{< text >}} blocks, HTML/MDX comments and a leading
// YAML/TOML front matter block are never headings. re is matched against the
// normalised heading text (leading/trailing '#', surrounding backticks and
// emphasis, a trailing "{#anchor}" and markdown links removed: "## `v1.18.0`"
// becomes "v1.18.0"); the heading text with backticks and the full heading
// line are tried as fallbacks. The section ends before the next heading of the
// same or a higher level (or at the end of the document) and trailing blank
// lines are not part of it. StartLine/EndLine are 1-based lines of md; Body is
// exactly those lines joined by "\n".
func SelectSection(md []byte, re *regexp.Regexp) (*Section, error) {
	return selectSection(md, re)
}

// ParseNotes splits markdown release notes / changelog / upgrade-guide content
// into atomic items (bullets and paragraphs under headings) and classifies
// each one. Product-specific rules are evaluated first, then the generic
// defaults. Items matched via a section heading or an upstream label get
// MethodDeclared; keyword-based classification gets MethodHeuristic.
// Each item gets exactly one Evidence pointing at its line range.
//
// Splitting: every top-level list item (with continuation lines, nested items
// and embedded code) is an item; a heading section that starts with prose
// instead of a list is ONE item "Heading: first paragraphs" (<= 1200 bytes)
// that folds in its lists and detail sub-sections ("Detection", "Option 1").
// Sections that contain category headings (Feature, Bug or Regression,
// Breaking Changes, ...), level-1 titles and grouping headings ("Major
// Themes") are containers whose sub-sections hold the items; their own prose
// becomes an introductory item only when it is substantive (level >= 2).
// Tables, comments, shortcode-only lines, Hugo {{< tip >}} asides, link
// reference definitions and front matter are never items, nor are the
// Community / Contributors / Next steps style sections (unless a product rule
// matches them). Items with identical text in one document are merged: the
// first classification wins and the item then references the evidence of every
// location.
//
// Classification order (every fired signal is listed in Provenance.Rule):
// product rules, upstream labels (section headings, bold verbs, "Security
// (HIGH):", conventional commits, breaking markers), keyword heuristics,
// role defaults (upgrade-guide => migration + ActionRequired), fallback
// "other". See classify.go for the exact tables.
func ParseNotes(in DocInput, rules []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error) {
	return parseNotes(in, rules)
}

// ParseReleaseNoteYAML parses structured release-note YAML files (one
// document per input, Istio "releasenotes/notes/*.yaml" style: kind, area,
// releaseNotes, upgradeNotes, securityNotes, ...). Every entry of
// releaseNotes / upgradeNotes / securityNotes is one item with its own
// Evidence (URI of the file, locator "<file name> L<first>-L<last>"); files of
// kind "test" are skipped. Keys are matched leniently ("upgradeNodes" typos,
// singular/plural). A file that does not parse is skipped and reported in the
// returned error, together with the items of the other files.
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
//
// Minor-level versions cover whole lines: "1.29 → 1.33" is
// ">=1.29.0-0, <1.34.0-0" with versions [1.29 1.30 1.31 1.32 1.33]; lists
// merge contiguous minors and join gaps with " || "; "1.25+" is
// ">=1.25.0-0"; strings with comparison operators are validated and passed
// through. The full grammar is documented in versionrange.go.
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
// each key when present. Keys containing "." or whitespace are written as
// ["key"], empty mappings are the leaf "{}"; comments are stripped of "#",
// helm-docs "-- " and "@param"-style markers and bounded to 300 bytes.
func ValuesSnapshot(chart, version string, valuesYAML []byte) (*domain.ValuesSnapshot, error) {
	return valuesSnapshot(chart, version, valuesYAML)
}

// CRDSnapshot summarises CustomResourceDefinitions found in one or more YAML
// streams (multi-document; non-CRD documents are ignored), sorted by name.
// SchemaPaths are sorted dotted property paths of the openAPIV3Schema below
// the root ("spec.issuerRef.name"; arrays as "spec.dnsNames[]" and
// "spec.foo[].bar"); x-kubernetes-* keys are skipped, additionalProperties is
// not descended into and at most MaxSchemaPaths paths are kept per version.
func CRDSnapshot(streams ...[]byte) (*domain.CRDSnapshot, error) {
	return crdSnapshot(streams...)
}

// ImageRefs extracts container image references from manifests or values
// (lines like `image: repo:tag`, `image: "repo@sha256:..."` and flags like
// `--foo-image=repo:tag`), de-duplicated and sorted. Templated values
// ("{{", "${", "$(") and empty values are skipped.
func ImageRefs(content []byte) []domain.ImageRef {
	return imageRefs(content)
}

// ParseImageRef parses "registry/repo[:tag][@digest]".
func ParseImageRef(s string) (domain.ImageRef, error) {
	return parseImageRef(s)
}

// ExtractReferences finds CVE ids, GHSA ids, and pull-request / issue
// references (GitHub URLs or "#1234" when repository is given), de-duplicated
// and in order of first appearance. Reference IDs of GitHub items are
// "owner/name#N"; a "#N" that is also present as a PR/issue URL of the same
// repository is folded into the URL reference. repository may be "owner/name",
// "github.com/owner/name" or a github.com URL; other hosts yield no bare
// references.
func ExtractReferences(text, repository string) []domain.Reference {
	return extractReferences(text, repository)
}
