// Package catalog defines the machine-readable ProductDefinition format, loads
// definitions from disk and validates them.
//
// A ProductDefinition declares, for one logical product, its authoritative
// release channels (sources), the artifacts it publishes, how artifact
// versions relate to release versions and how to extract information from
// each source. Generic ingestion code is driven entirely by this data; product
// specific behaviour belongs here, not in Go code.
package catalog

import (
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// APIVersion is the current definition format version.
const APIVersion = "ri.dev/v1alpha1"

// Kind is the document kind.
const Kind = "ProductDefinition"

// ProductDefinition is the root document.
type ProductDefinition struct {
	APIVersion  string `yaml:"apiVersion" json:"apiVersion"`
	Kind        string `yaml:"kind" json:"kind"`
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Homepage    string `yaml:"homepage,omitempty" json:"homepage,omitempty"`

	Versioning Versioning `yaml:"versioning" json:"versioning"`
	Sources    []Source   `yaml:"sources" json:"sources"`
	Artifacts  []Artifact `yaml:"artifacts" json:"artifacts"`
	// Lifecycle states that the product (or some of its releases) is
	// deprecated or end-of-life; see Lifecycle.
	Lifecycle []Lifecycle `yaml:"lifecycle,omitempty" json:"lifecycle,omitempty"`

	// Provenance documents how this definition was produced and validated.
	Provenance *DefinitionProvenance `yaml:"provenance,omitempty" json:"provenance,omitempty"`

	// digest of the raw document, set by the loader.
	digest string
	path   string
}

// Digest returns the sha256 of the raw definition document.
func (d *ProductDefinition) Digest() string { return d.digest }

// Path returns the file the definition was loaded from ("" if not from disk).
func (d *ProductDefinition) Path() string { return d.path }

// Ref returns a domain product reference.
func (d *ProductDefinition) Ref() domain.ProductRef {
	return domain.ProductRef{ID: domain.ProductID(d.ID), Name: d.Name}
}

// Versioning describes how releases are identified.
type Versioning struct {
	Scheme domain.VersionScheme `yaml:"scheme" json:"scheme"`
	// TagPrefix is prepended to the semantic version in canonical tags ("v").
	TagPrefix string `yaml:"tagPrefix,omitempty" json:"tagPrefix,omitempty"`
	// TagPattern overrides the tag regex; it must contain a named group
	// "version" (the semantic version verbatim) or a named group "major",
	// optionally with "minor", "patch" and "prerelease": the semantic version
	// is then assembled from the components (see domain.HasVersionGroups).
	TagPattern string `yaml:"tagPattern,omitempty" json:"tagPattern,omitempty"`
	// Lineage describes how release lines branch: "minor" (X.Y.z lines are
	// maintained in parallel and X.Y.0 carries the changes since the previous
	// line) or "linear" (every release builds on the previous one).
	Lineage string `yaml:"lineage,omitempty" json:"lineage,omitempty"`
	// IncludePrereleases makes prereleases part of the canonical version list.
	IncludePrereleases bool `yaml:"includePrereleases,omitempty" json:"includePrereleases,omitempty"`
}

// Lineage values.
const (
	LineageMinor  = "minor"
	LineageLinear = "linear"
)

// Source is an information channel for the product (release notes, docs,
// compatibility matrices, advisories, the canonical version list).
type Source struct {
	ID      string              `yaml:"id" json:"id"`
	Roles   []domain.SourceRole `yaml:"roles" json:"roles"`
	Locator Locator             `yaml:"locator" json:"locator"`
	// Extract describes how to pull the relevant part out of the document.
	Extract *Extract `yaml:"extract,omitempty" json:"extract,omitempty"`
	// Classify adds product-specific classification rules for note items,
	// evaluated before the generic defaults.
	Classify []ClassifyRule `yaml:"classify,omitempty" json:"classify,omitempty"`
	// Availability is a semver constraint on the release version for which
	// this source exists (e.g. ">= 1.10.0").
	Availability string `yaml:"availability,omitempty" json:"availability,omitempty"`
	// ReleaseKinds limits the source to some release kinds: "major", "minor"
	// (X.Y.0 with Y>0 or X.0.0), "patch". Empty means all.
	ReleaseKinds []string `yaml:"releaseKinds,omitempty" json:"releaseKinds,omitempty"`
	// Priority orders sources sharing a role (lower first).
	Priority int `yaml:"priority,omitempty" json:"priority,omitempty"`
	// FallbackGroup names a set of alternative sources: sources with the same
	// non-empty group are tried in priority order and consultation stops at
	// the first one that answers (state ok). Sources without a group are
	// always consulted.
	FallbackGroup string `yaml:"fallbackGroup,omitempty" json:"fallbackGroup,omitempty"`
	// Exceptions are releases for which this source is known not to exist
	// upstream; they are treated as not applicable, with the reason recorded.
	Exceptions []Exception `yaml:"exceptions,omitempty" json:"exceptions,omitempty"`
	// ValidatedAgainst lists release versions for which this source was
	// verified to resolve (written by discovery / relationship checks).
	ValidatedAgainst []string `yaml:"validatedAgainst,omitempty" json:"validatedAgainst,omitempty"`
	Notes            string   `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// Exception records releases for which a declared relationship is known not
// to hold upstream (e.g. a release whose assets were never published). It is
// curated knowledge with a mandatory reason, so the deviation stays visible.
type Exception struct {
	Versions []string `yaml:"versions" json:"versions"` // semantic versions without prefix
	Reason   string   `yaml:"reason" json:"reason"`
}

// ExceptionFor returns the reason when v is listed in exceptions.
func ExceptionFor(v domain.Version, exceptions []Exception) (string, bool) {
	for _, e := range exceptions {
		for _, s := range e.Versions {
			if s == v.Semver || s == v.Tag {
				return e.Reason, true
			}
		}
	}
	return "", false
}

// HasRole reports whether the source has role r.
func (s Source) HasRole(r domain.SourceRole) bool {
	for _, x := range s.Roles {
		if x == r {
			return true
		}
	}
	return false
}

// Locator kinds understood by the generic ingestion code. Each kind is served
// by an adapter registered in package sources.
const (
	LocatorGitHubReleases   = "github-releases"   // repository: owner/name
	LocatorGitHubAdvisories = "github-advisories" // repository: owner/name
	LocatorGitTags          = "git-tags"          // repository: host/owner/name or URL; optional tagPattern
	LocatorRepoFile         = "repo-file"         // repository, ref, path
	LocatorRepoDir          = "repo-dir"          // repository, ref, path, glob
	LocatorHTTP             = "http"              // url
	LocatorHelmRepo         = "helm-repo"         // url, chart
	LocatorOCI              = "oci"               // repository: registry/name
	LocatorHelmGit          = "helm-git"          // repository, path (chart dir), tagPattern
	// LocatorGitLog renders the commit subjects in a revision range
	// (ref: "{{.PrevTag}}..{{.Tag}}") as a markdown bullet list, a
	// deterministic fallback when curated release notes are unreachable.
	LocatorGitLog = "git-log" // repository, ref (range)
)

// Locator says where and how to fetch something. Fields are interpreted per
// Kind; string fields may contain templates (see RenderContext).
type Locator struct {
	Kind string `yaml:"kind" json:"kind"`
	// Repository: "owner/name" for github-* kinds, "host/owner/name" (or a
	// URL) for git-based kinds, "registry/path" for oci.
	Repository string `yaml:"repository,omitempty" json:"repository,omitempty"`
	Ref        string `yaml:"ref,omitempty" json:"ref,omitempty"` // default "{{.Tag}}"
	// BaseRef (repo-dir only): when set, return only files that exist at Ref
	// but not at BaseRef — e.g. release-note files added since "{{.PrevTag}}".
	BaseRef    string `yaml:"baseRef,omitempty" json:"baseRef,omitempty"`
	Path       string `yaml:"path,omitempty" json:"path,omitempty"`
	Glob       string `yaml:"glob,omitempty" json:"glob,omitempty"`
	URL        string `yaml:"url,omitempty" json:"url,omitempty"`
	Chart      string `yaml:"chart,omitempty" json:"chart,omitempty"`
	TagPattern string `yaml:"tagPattern,omitempty" json:"tagPattern,omitempty"`
}

// Extract types.
const (
	ExtractWhole           = "whole"             // the full document
	ExtractMarkdownSection = "markdown-section"  // a section selected by heading regex
	ExtractMarkdownTable   = "markdown-table"    // a table row selected by key column
	ExtractReleaseNoteYAML = "release-note-yaml" // structured release-note YAML files (one item per file)
	ExtractYAMLRecords     = "yaml-records"      // a record selected from a YAML/JSON list (same fields as markdown-table; headers are field names)
)

// Document formats an Extract can read (Extract.Format).
const (
	FormatMarkdown = "markdown" // default
	// FormatDocBook: the document is DocBook (SGML/XML); it is rendered as
	// line-preserving markdown before extraction (normalize.DocBookToMarkdown),
	// so "markdown-section" selects <sectN> by title and "whole" parses it as notes.
	FormatDocBook = "docbook"
	// FormatRST: the document is reStructuredText; it is rendered as
	// line-preserving markdown before extraction (normalize.RSTToMarkdown):
	// section titles become headings and grid tables become pipe tables, so
	// "markdown-section" selects a section by title and "markdown-table" can
	// read converted grid tables.
	FormatRST = "rst"
)

// Extract configures how to extract information from a fetched document.
type Extract struct {
	// Format is the markup of the document ("markdown" when empty); "docbook"
	// and "rst" documents are converted to markdown first (docbook for the
	// types "whole" and "markdown-section"; rst also for "markdown-table",
	// because grid tables are converted to pipe tables).
	Format string `yaml:"format,omitempty" json:"format,omitempty"`
	Type   string `yaml:"type" json:"type"`
	// Heading is a regex template selecting the section (markdown-section).
	// Headings are matched after trimming '#', whitespace and backticks.
	Heading string `yaml:"heading,omitempty" json:"heading,omitempty"`
	// KeyColumns are alternative header names of the key column
	// (markdown-table). Every table in the document having one of these
	// columns is searched, in document order.
	KeyColumns []string `yaml:"keyColumns,omitempty" json:"keyColumns,omitempty"`
	// KeyMatch is a regex template matched against the key cell (after
	// stripping markdown links/emphasis), e.g. '^{{regexQuote .Line}}\b'.
	KeyMatch string `yaml:"keyMatch,omitempty" json:"keyMatch,omitempty"`
	// Columns selects values from the matched row.
	Columns []ColumnSpec `yaml:"columns,omitempty" json:"columns,omitempty"`
	// TableHeading optionally restricts the search to tables that appear
	// under a heading matching this regex.
	TableHeading string `yaml:"tableHeading,omitempty" json:"tableHeading,omitempty"`
	// LabelParagraphs (whole, markdown-section) is a regex matched against
	// standalone paragraph lines: a match is treated as a heading one level
	// below the last real heading. For changelogs that group bullets under
	// uppercase label paragraphs ("SECURITY:", "BUG FIXES:") rather than
	// markdown headings, e.g. '^[A-Z][A-Z0-9 /&-]*:$'.
	LabelParagraphs string `yaml:"labelParagraphs,omitempty" json:"labelParagraphs,omitempty"`
	// ListItems (whole, markdown-section) selects the structural reading that
	// docbook/rst conversions always use: every list item is one item of its
	// own and the substantive prose of a section is one item, instead of a
	// prose-led section (callout, intro) folding its whole list into a single
	// item. For documents like Karpenter's upgrade guide, whose per-version
	// sections open with a warning callout before the bullet list.
	ListItems bool `yaml:"listItems,omitempty" json:"listItems,omitempty"`
}

// ColumnSpec maps a table column to a platform constraint.
type ColumnSpec struct {
	Platform string `yaml:"platform" json:"platform"`             // "kubernetes", "openshift", ...
	Kind     string `yaml:"kind,omitempty" json:"kind,omitempty"` // "supported" (default), "tested", "minimum", "maximum"
	// Headers are alternative header names (case-insensitive, markdown
	// links stripped); the first present in the table is used.
	Headers []string `yaml:"headers" json:"headers"`
	// Separator splits a combined cell ("1.33 → 1.36 / 4.20 → 4.22") and
	// Part selects the 0-based piece.
	Separator string `yaml:"separator,omitempty" json:"separator,omitempty"`
	Part      int    `yaml:"part,omitempty" json:"part,omitempty"`
}

// ClassifyRule maps a note item to a category. Section and Text are regexes;
// a rule matches when every non-empty regex matches.
type ClassifyRule struct {
	Section        string          `yaml:"section,omitempty" json:"section,omitempty"`
	Text           string          `yaml:"text,omitempty" json:"text,omitempty"`
	Category       domain.Category `yaml:"category,omitempty" json:"category,omitempty"`
	Breaking       *bool           `yaml:"breaking,omitempty" json:"breaking,omitempty"`
	ActionRequired *bool           `yaml:"actionRequired,omitempty" json:"actionRequired,omitempty"`
	// Skip drops matching items (e.g. "Other (Cleanup or Flake)").
	Skip bool `yaml:"skip,omitempty" json:"skip,omitempty"`
}

// Artifact describes a deliverable published for each release.
type Artifact struct {
	ID          string              `yaml:"id" json:"id"`
	Type        domain.ArtifactType `yaml:"type" json:"type"`
	Name        string              `yaml:"name" json:"name"`
	Description string              `yaml:"description,omitempty" json:"description,omitempty"`
	// Version states how this artifact's version relates to the release version.
	Version VersionRelation `yaml:"version" json:"version"`
	// Channels are publication locations, in preference order.
	Channels []Locator `yaml:"channels" json:"channels"`
	// References are other artifacts of the same release that reference this
	// one (e.g. an install manifest containing the image tag). Used for
	// cross-reference verification when the channel itself is unreachable.
	References []ArtifactReference `yaml:"references,omitempty" json:"references,omitempty"`
	// Contents are structured views to snapshot for diffing.
	Contents []Content `yaml:"contents,omitempty" json:"contents,omitempty"`
	// Availability is a semver constraint on release versions that ship this artifact.
	Availability string `yaml:"availability,omitempty" json:"availability,omitempty"`
	// Optional marks artifacts that are not published for every release
	// (e.g. a community chart that skips some application versions). A
	// missing instance is still reported as missing, but it does not count
	// against the relationship.
	Optional bool `yaml:"optional,omitempty" json:"optional,omitempty"`
	// Exceptions are releases for which the artifact is known to be absent.
	Exceptions       []Exception `yaml:"exceptions,omitempty" json:"exceptions,omitempty"`
	ValidatedAgainst []string    `yaml:"validatedAgainst,omitempty" json:"validatedAgainst,omitempty"`
	Notes            string      `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// Version relation strategies.
const (
	VersionTemplate    = "template"    // artifact version = render(template)
	VersionLookup      = "lookup"      // search channel index for entries whose Field matches render(Match)
	VersionField       = "field"       // artifact version = a YAML field of the document at From (read at the release ref)
	VersionIndependent = "independent" // no derivable relationship
)

// VersionRelation describes how artifact versions relate to release versions.
// This exists because versions across artifacts frequently do not match
// (e.g. Argo CD 3.0.0 ships in Helm chart 8.0.x).
type VersionRelation struct {
	Strategy string `yaml:"strategy" json:"strategy"`
	Template string `yaml:"template,omitempty" json:"template,omitempty"`
	// Lookup: Field of the index entry (e.g. "appVersion") compared with Match.
	Field string `yaml:"field,omitempty" json:"field,omitempty"`
	Match string `yaml:"match,omitempty" json:"match,omitempty"`
	// Field: YAML path read out of the From document (e.g. "appVersion" or
	// "dependencies[name=kube-state-metrics].version").
	// From: locator of the document the field is read from, rendered with the
	// release context (e.g. Chart.yaml at "{{.Tag}}").
	From  *Locator `yaml:"from,omitempty" json:"from,omitempty"`
	Select string `yaml:"select,omitempty" json:"select,omitempty"` // lookup: "latest" (default), "earliest", "all"
}

// ArtifactReference declares that another artifact references this one.
type ArtifactReference struct {
	Artifact string `yaml:"artifact" json:"artifact"`
	// Pattern is a template of the literal string expected in the referencing
	// artifact (e.g. "quay.io/jetstack/cert-manager-controller:{{.Tag}}").
	Pattern string `yaml:"pattern" json:"pattern"`
}

// Content kinds map to domain snapshot kinds.
const (
	ContentHelmValues    = "helm-values"    // values.yaml of a chart
	ContentChartMetadata = "chart-metadata" // Chart.yaml (kubeVersion → compatibility)
	ContentCRDs          = "crds"           // CRD YAML (single or multi-doc, or a directory)
	ContentImageRefs     = "image-refs"     // image references inside manifests
)

// Content is a structured view of an artifact to capture per release.
type Content struct {
	Kind string `yaml:"kind" json:"kind"`
	// Locator to fetch the content from. When omitted the artifact's first
	// http channel is used.
	Locator *Locator `yaml:"locator,omitempty" json:"locator,omitempty"`
	// Availability narrows when this content can be captured.
	Availability string `yaml:"availability,omitempty" json:"availability,omitempty"`
	// StripPrefix (helm-values only) removes a wrapper key path from every
	// values key, so keys match what users actually set (e.g. Istio nests
	// chart defaults under "_internal_defaults_do_not_set").
	StripPrefix string `yaml:"stripPrefix,omitempty" json:"stripPrefix,omitempty"`
	// IgnoreKeys (helm-values only) lists dotted values keys, as they are
	// after StripPrefix, that are excluded from the snapshot. Intent: keys
	// rewritten by release tooling at build time (e.g. image hub/tag
	// placeholders in source-tree values), whose in-tree defaults differ from
	// release to release without meaning anything to a user. An entry matches
	// one key exactly; an entry ending in ".*" matches every key below it
	// ("global.image.*" matches "global.image.tag").
	IgnoreKeys []string `yaml:"ignoreKeys,omitempty" json:"ignoreKeys,omitempty"`
}

// DefinitionProvenance documents the origin of the definition itself.
type DefinitionProvenance struct {
	Method            string   `yaml:"method" json:"method"` // "manual", "discovery", "discovery+review"
	Author            string   `yaml:"author,omitempty" json:"author,omitempty"`
	Updated           string   `yaml:"updated,omitempty" json:"updated,omitempty"`
	ValidatedReleases []string `yaml:"validatedReleases,omitempty" json:"validatedReleases,omitempty"`
	Notes             string   `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// Source returns the source with the given id.
func (d *ProductDefinition) Source(id string) (Source, bool) {
	for _, s := range d.Sources {
		if s.ID == id {
			return s, true
		}
	}
	return Source{}, false
}

// SourcesWithRole returns the sources having role r, ordered by priority.
func (d *ProductDefinition) SourcesWithRole(r domain.SourceRole) []Source {
	var out []Source
	for _, s := range d.Sources {
		if s.HasRole(r) {
			out = append(out, s)
		}
	}
	// stable insertion sort by priority
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Priority < out[j-1].Priority; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Artifact returns the artifact with the given id.
func (d *ProductDefinition) Artifact(id string) (Artifact, bool) {
	for _, a := range d.Artifacts {
		if a.ID == id {
			return a, true
		}
	}
	return Artifact{}, false
}

// VersionParser returns the parser for this product's canonical tags.
func (d *ProductDefinition) VersionParser() (domain.VersionParser, error) {
	p := domain.VersionParser{Scheme: d.Versioning.Scheme}
	if d.Versioning.TagPattern != "" {
		re, err := compileRegex(d.Versioning.TagPattern)
		if err != nil {
			return p, err
		}
		p.Pattern = re
	} else {
		p.Pattern = domain.DefaultTagPattern(d.Versioning.TagPrefix)
	}
	return p, nil
}

// TagFor returns the canonical tag for a semantic version, using TagPrefix.
func (d *ProductDefinition) TagFor(semver string) string {
	return d.Versioning.TagPrefix + semver
}
