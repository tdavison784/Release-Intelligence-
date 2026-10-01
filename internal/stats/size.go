package stats

import (
	"bytes"
	"os"
	"reflect"

	"github.com/tdavison784/release-intelligence/internal/catalog"
)

// Size is the size and complexity of one product definition.
type Size struct {
	// YAMLLines counts the physical lines of the definition file;
	// YAMLCodeLines excludes blank and comment-only lines.
	YAMLLines     int `json:"yamlLines"`
	YAMLCodeLines int `json:"yamlCodeLines"`

	Sources       int `json:"sources"`
	Artifacts     int `json:"artifacts"`
	Channels      int `json:"channels"`      // artifact publication channels
	Contents      int `json:"contents"`      // structured snapshots (helm-values, crds, ...)
	ClassifyRules int `json:"classifyRules"` // source classification rules
	TemplateExprs int `json:"templateExprs"` // {{ ... }} expressions
	Exceptions    int `json:"exceptions"`    // curated exceptions (sources and artifacts)

	OptionalArtifacts       int `json:"optionalArtifacts"`
	FallbackGroups          int `json:"fallbackGroups"`          // distinct named groups
	AvailabilityConstraints int `json:"availabilityConstraints"` // sources, artifacts, contents
}

// ProductStats is the computed view of one definition.
type ProductStats struct {
	Product string `json:"product"`
	Name    string `json:"name,omitempty"`
	Size
	// Constructs is the sorted set of catalog constructs the definition uses.
	Constructs []string `json:"constructs"`

	// SourceIDs and ArtifactIDs list the elements of the definition (used to
	// cross-check records).
	SourceIDs   []string `json:"-"`
	ArtifactIDs []string `json:"-"`
}

// Analyze computes the stats of a definition. raw is the definition file's
// content (for the line counts); when nil, the file the definition was loaded
// from is read, and when that is not possible the line counts are zero.
func Analyze(def *catalog.ProductDefinition, raw []byte) ProductStats {
	if raw == nil && def.Path() != "" {
		raw, _ = os.ReadFile(def.Path())
	}
	ps := ProductStats{Product: def.ID, Name: def.Name}
	ps.YAMLLines, ps.YAMLCodeLines = countLines(raw)

	ps.Sources = len(def.Sources)
	ps.Artifacts = len(def.Artifacts)
	groups := map[string]bool{}
	for _, s := range def.Sources {
		ps.SourceIDs = append(ps.SourceIDs, s.ID)
		ps.ClassifyRules += len(s.Classify)
		ps.Exceptions += len(s.Exceptions)
		if s.Availability != "" {
			ps.AvailabilityConstraints++
		}
		if s.FallbackGroup != "" {
			groups[s.FallbackGroup] = true
		}
	}
	for _, a := range def.Artifacts {
		ps.ArtifactIDs = append(ps.ArtifactIDs, a.ID)
		ps.Channels += len(a.Channels)
		ps.Contents += len(a.Contents)
		ps.Exceptions += len(a.Exceptions)
		if a.Optional {
			ps.OptionalArtifacts++
		}
		if a.Availability != "" {
			ps.AvailabilityConstraints++
		}
		for _, c := range a.Contents {
			if c.Availability != "" {
				ps.AvailabilityConstraints++
			}
		}
	}
	ps.FallbackGroups = len(groups)

	w := newWalker()
	w.walk(reflect.ValueOf(def), "", "", true)
	ps.TemplateExprs = w.tmplExprs
	ps.Constructs = w.constructs()
	return ps
}

// countLines returns the number of lines and of lines that are neither blank
// nor comment-only.
func countLines(raw []byte) (lines, code int) {
	if len(raw) == 0 {
		return 0, 0
	}
	for _, l := range bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n")) {
		lines++
		t := bytes.TrimSpace(l)
		if len(t) == 0 || t[0] == '#' {
			continue
		}
		code++
	}
	return lines, code
}
