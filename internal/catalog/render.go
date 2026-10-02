package catalog

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"text/template"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// RenderContext is the data available to templates in a definition.
//
//	{{.Product}}     product id
//	{{.Tag}}         canonical tag, e.g. "v1.18.0"
//	{{.Version}}     semantic version without prefix, e.g. "1.18.0"
//	{{.Major}} {{.Minor}} {{.Patch}} {{.Prerelease}}
//	{{.Line}}        "1.18"
//	{{.PrevLine}} {{.PrevMajor}} {{.PrevMinor}}  previous existing release line ("1.17")
//	{{.PrevTag}} {{.PrevVersion}}  previous release in lineage order: for a
//	                 patch release the previous patch of the same line, for an
//	                 X.Y.0 release the X'.Y'.0 release of the previous line
//	{{.ArtifactVersion}}  resolved artifact version (artifact contexts only)
//	{{.ArtifactVersionOf "id"}}  the version another artifact of the same
//	                 release resolved to (source templates only; see
//	                 ArtifactVersionOf)
//
// Functions: regexQuote, trimPrefix, trimSuffix, lower, upper, replace.
type RenderContext struct {
	Product         string
	Tag             string
	Version         string
	Major           uint64
	Minor           uint64
	Patch           uint64
	Prerelease      string
	Line            string
	PrevLine        string
	PrevMajor       uint64
	PrevMinor       uint64
	PrevTag         string
	PrevVersion     string
	ArtifactVersion string
	// ArtifactVersions holds the resolved versions of the artifacts whose
	// versions source templates follow (see ArtifactVersionOf). It is set
	// only when rendering source locators and extracts, after ingest
	// pre-resolved the referenced artifacts; nil everywhere else, so an
	// {{.ArtifactVersionOf}} in an artifact template fails loudly.
	ArtifactVersions map[string]string
	// anyArtifactVersion makes ArtifactVersionOf answer any non-empty id
	// with the sample version. Static validation only (sourceSampleContext):
	// rendering must not mask a reference error that sourceArtifactRefs
	// reports with a precise path and message. Never set during ingestion.
	anyArtifactVersion bool
}

// NewRenderContext builds a context for version v. known is the list of known
// release versions (any order) used to compute the previous release line; it
// may be nil, in which case PrevLine falls back to Minor-1 when Minor > 0. When
// known is not empty but holds no lower line (the first release of a product),
// PrevLine, PrevMajor, PrevMinor and PrevTag are left empty.
func NewRenderContext(product string, v domain.Version, known []domain.Version) RenderContext {
	rc := RenderContext{
		Product:    product,
		Tag:        v.Tag,
		Version:    v.Semver,
		Major:      v.Major(),
		Minor:      v.Minor(),
		Patch:      v.Patch(),
		Prerelease: v.Prerelease(),
		Line:       v.Line(),
	}
	// previous line: the highest line strictly lower than v's line.
	var best *domain.Version
	for i := range known {
		k := known[i]
		if k.IsPrerelease() {
			continue
		}
		if k.Major() > v.Major() || (k.Major() == v.Major() && k.Minor() >= v.Minor()) {
			continue
		}
		if best == nil || best.Less(k) {
			best = &known[i]
		}
	}
	switch {
	case best != nil:
		rc.PrevMajor, rc.PrevMinor = best.Major(), best.Minor()
		rc.PrevLine = best.Line()
	case v.Minor() > 0 && len(known) == 0:
		// Without any version list the previous line can only be guessed. With
		// one, no lower line means v is the product's first release (or its
		// first line): the Prev* fields stay empty instead of inventing a
		// line that never existed.
		rc.PrevMajor, rc.PrevMinor = v.Major(), v.Minor()-1
		rc.PrevLine = fmt.Sprintf("%d.%d", rc.PrevMajor, rc.PrevMinor)
	}
	rc.PrevTag, rc.PrevVersion = prevInLineage(v, known, rc)
	return rc
}

// prevInLineage computes the previous release in lineage order (see RenderContext).
func prevInLineage(v domain.Version, known []domain.Version, rc RenderContext) (string, string) {
	prefix := strings.TrimSuffix(v.Tag, v.Semver)
	if v.Patch() > 0 {
		var best *domain.Version
		for i := range known {
			k := known[i]
			if k.IsPrerelease() || k.Major() != v.Major() || k.Minor() != v.Minor() || !k.Less(v) {
				continue
			}
			if best == nil || best.Less(k) {
				best = &known[i]
			}
		}
		if best != nil {
			return best.Tag, best.Semver
		}
		if len(known) == 0 {
			s := fmt.Sprintf("%d.%d.%d", v.Major(), v.Minor(), v.Patch()-1)
			return prefix + s, s
		}
		return "", ""
	}
	if rc.PrevLine == "" {
		return "", ""
	}
	want := fmt.Sprintf("%d.%d.0", rc.PrevMajor, rc.PrevMinor)
	for _, k := range known {
		if k.Semver == want {
			return k.Tag, k.Semver
		}
	}
	if len(known) == 0 {
		return prefix + want, want
	}
	return "", ""
}

// WithArtifactVersion returns a copy with ArtifactVersion set.
func (rc RenderContext) WithArtifactVersion(av string) RenderContext {
	rc.ArtifactVersion = av
	return rc
}

// WithArtifactVersions returns a copy with the versions of followed
// artifacts set (source rendering; see ArtifactVersionOf).
func (rc RenderContext) WithArtifactVersions(m map[string]string) RenderContext {
	rc.ArtifactVersions = m
	return rc
}

// ArtifactVersionOf returns the version another artifact of the same release
// resolved to: in a SOURCE template (locator or extract),
//
//	{{.ArtifactVersionOf "prometheus-operator-image"}}
//
// renders the pinned component version behind the product — the operator an
// aggregating chart deploys, a controller a manifest pins — so the source can
// read the component's own release notes at its pinned tag. Ingest resolves
// the referenced artifact's version BEFORE sources are consulted (the
// artifact's own version relation reads the release's files or is pure
// templating, so this introduces no circularity); only ids that resolved are
// in the map. A missing or unresolved id is an error, which makes the source
// skipped/unavailable — never a wrong locator. The argument must be a
// double-quoted literal so the reference is statically checkable (Validate
// rejects unknown ids and unresolvable strategies).
func (rc RenderContext) ArtifactVersionOf(id string) (string, error) {
	if v, ok := rc.ArtifactVersions[id]; ok && v != "" {
		return v, nil
	}
	if rc.anyArtifactVersion && id != "" {
		return SampleArtifactVersion, nil
	}
	return "", fmt.Errorf("the version of artifact %q is not available for this release", id)
}

var funcs = template.FuncMap{
	"regexQuote": regexp.QuoteMeta,
	"trimPrefix": func(prefix, s string) string { return strings.TrimPrefix(s, prefix) },
	"trimSuffix": func(suffix, s string) string { return strings.TrimSuffix(s, suffix) },
	"lower":      strings.ToLower,
	"upper":      strings.ToUpper,
	"replace":    func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
}

var (
	tmplCache   = map[string]*template.Template{}
	tmplCacheMu sync.Mutex
)

func parseTemplate(s string) (*template.Template, error) {
	tmplCacheMu.Lock()
	defer tmplCacheMu.Unlock()
	if t, ok := tmplCache[s]; ok {
		return t, nil
	}
	t, err := template.New("").Funcs(funcs).Option("missingkey=error").Parse(s)
	if err != nil {
		return nil, err
	}
	tmplCache[s] = t
	return t, nil
}

// Render renders a template string with the context. Strings without "{{"
// are returned unchanged.
func Render(s string, rc RenderContext) (string, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	t, err := parseTemplate(s)
	if err != nil {
		return "", fmt.Errorf("template %q: %w", s, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, rc); err != nil {
		return "", fmt.Errorf("template %q: %w", s, err)
	}
	return buf.String(), nil
}

// RenderLocator renders every templated field of a locator. Ref defaults to
// "{{.Tag}}" for repository-backed kinds.
func RenderLocator(l Locator, rc RenderContext) (Locator, error) {
	out := l
	if out.Ref == "" && (l.Kind == LocatorRepoFile || l.Kind == LocatorRepoDir) {
		out.Ref = "{{.Tag}}"
	}
	fields := []*string{&out.Repository, &out.Ref, &out.BaseRef, &out.Path, &out.Glob, &out.URL, &out.Chart}
	for _, f := range fields {
		r, err := Render(*f, rc)
		if err != nil {
			return Locator{}, err
		}
		*f = r
	}
	return out, nil
}

// LocatorTemplateFields returns the locator fields that are rendered as
// templates (the field set of RenderLocator), in a fixed order. Shared by the
// reference scanner and the validator so both see exactly what ingest renders.
func LocatorTemplateFields(l Locator) []string {
	return []string{l.Repository, l.Ref, l.BaseRef, l.Path, l.Glob, l.URL, l.Chart}
}

// artifactVersionRefRe matches the {{.ArtifactVersionOf "id"}} references in
// a template. The argument must be a double-quoted literal: the id is data
// (a definition field), not something computed at render time.
var artifactVersionRefRe = regexp.MustCompile(`\.ArtifactVersionOf\s+"([^"]*)"`)

// ReferencedArtifacts returns the artifact ids s references through
// {{.ArtifactVersionOf "id"}}, in order of first reference, without
// duplicates. Empty ids (a malformed reference) are kept so validation can
// reject them.
func ReferencedArtifacts(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range artifactVersionRefRe.FindAllStringSubmatch(s, -1) {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	return out
}

// SourceArtifactRefs returns the artifact ids a source's templates reference
// through {{.ArtifactVersionOf "id"}}: the locator fields and the extract's
// rendered regex templates — exactly the strings ingest renders with the
// release context when consulting the source.
func SourceArtifactRefs(src Source) []string {
	var out []string
	for _, s := range LocatorTemplateFields(src.Locator) {
		out = append(out, ReferencedArtifacts(s)...)
	}
	if src.Extract != nil {
		for _, s := range []string{src.Extract.Heading, src.Extract.KeyMatch, src.Extract.TableHeading} {
			out = append(out, ReferencedArtifacts(s)...)
		}
	}
	seen := map[string]bool{}
	var uniq []string
	for _, id := range out {
		if seen[id] {
			continue
		}
		seen[id] = true
		uniq = append(uniq, id)
	}
	return uniq
}

// AppliesTo reports whether a source/artifact with the given availability
// constraint and release kinds applies to version v. A prerelease is judged by
// its release version (see domain.Version.Satisfies), so 1.19.0-alpha.0
// satisfies ">= 1.5.0".
func AppliesTo(v domain.Version, availability string, releaseKinds []string) (bool, error) {
	ok, err := v.Satisfies(availability)
	if err != nil || !ok {
		return false, err
	}
	if len(releaseKinds) == 0 {
		return true, nil
	}
	kind := ReleaseKind(v)
	for _, k := range releaseKinds {
		if k == kind {
			return true, nil
		}
	}
	return false, nil
}

// ReleaseKind classifies v as "major" (X.0.0), "minor" (X.Y.0) or "patch".
func ReleaseKind(v domain.Version) string {
	switch {
	case v.Patch() != 0:
		return "patch"
	case v.Minor() == 0:
		return "major"
	default:
		return "minor"
	}
}

func compileRegex(s string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(s)
	if err != nil {
		return nil, fmt.Errorf("invalid regex %q: %w", s, err)
	}
	return re, nil
}
