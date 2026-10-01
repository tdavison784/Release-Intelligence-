package catalog

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Severity of a validation issue.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue is a single validation finding.
type Issue struct {
	Severity Severity `json:"severity"`
	Path     string   `json:"path"` // e.g. "sources[2].locator.path"
	Message  string   `json:"message"`
}

func (i Issue) String() string { return fmt.Sprintf("%s: %s: %s", i.Severity, i.Path, i.Message) }

// ValidationReport collects issues for a definition.
type ValidationReport struct {
	Product string  `json:"product"`        // "" when the file could not be loaded
	File    string  `json:"file,omitempty"` // source file, when known
	Issues  []Issue `json:"issues"`
}

// OK reports whether there are no errors (warnings allowed).
func (r ValidationReport) OK() bool {
	for _, i := range r.Issues {
		if i.Severity == SeverityError {
			return false
		}
	}
	return true
}

// Errors returns only error-level issues.
func (r ValidationReport) Errors() []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.Severity == SeverityError {
			out = append(out, i)
		}
	}
	return out
}

type validator struct {
	r ValidationReport
}

func (v *validator) errf(path, format string, args ...any) {
	v.r.Issues = append(v.r.Issues, Issue{SeverityError, path, fmt.Sprintf(format, args...)})
}

func (v *validator) warnf(path, format string, args ...any) {
	v.r.Issues = append(v.r.Issues, Issue{SeverityWarning, path, fmt.Sprintf(format, args...)})
}

var idRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

var knownRoles = map[domain.SourceRole]bool{
	domain.RoleVersions: true, domain.RoleReleaseNotes: true, domain.RoleChangelog: true,
	domain.RoleUpgradeGuide: true, domain.RoleCompatibility: true, domain.RoleSecurity: true,
}

var knownArtifactTypes = map[domain.ArtifactType]bool{
	domain.ArtifactSourceRelease: true, domain.ArtifactHelmChart: true, domain.ArtifactContainerImage: true,
	domain.ArtifactOperator: true, domain.ArtifactBinary: true, domain.ArtifactPackage: true,
	domain.ArtifactManifest: true, domain.ArtifactCRD: true, domain.ArtifactDocumentation: true,
}

// locatorSpec lists required fields per locator kind.
var locatorSpec = map[string][]string{
	LocatorGitHubReleases:   {"repository"},
	LocatorGitHubAdvisories: {"repository"},
	LocatorGitTags:          {"repository"},
	LocatorRepoFile:         {"repository", "path"},
	LocatorRepoDir:          {"repository", "path"},
	LocatorHTTP:             {"url"},
	LocatorHelmRepo:         {"url", "chart"},
	LocatorOCI:              {"repository"},
	LocatorHelmGit:          {"repository", "path", "tagPattern"},
	LocatorGitLog:           {"repository", "ref"},
}

// channel kinds allowed per artifact type.
var channelKinds = map[domain.ArtifactType][]string{
	domain.ArtifactContainerImage: {LocatorOCI},
	domain.ArtifactHelmChart:      {LocatorHelmRepo, LocatorOCI, LocatorHelmGit},
	domain.ArtifactManifest:       {LocatorHTTP, LocatorRepoFile, LocatorRepoDir},
	domain.ArtifactCRD:            {LocatorHTTP, LocatorRepoFile, LocatorRepoDir},
	domain.ArtifactBinary:         {LocatorHTTP, LocatorOCI},
	domain.ArtifactPackage:        {LocatorHTTP, LocatorOCI},
	domain.ArtifactOperator:       {LocatorOCI, LocatorHTTP, LocatorRepoDir},
	domain.ArtifactSourceRelease:  {LocatorGitHubReleases, LocatorGitTags},
	domain.ArtifactDocumentation:  {LocatorHTTP, LocatorRepoFile, LocatorRepoDir},
}

var knownContent = map[string]bool{
	ContentHelmValues: true, ContentChartMetadata: true, ContentCRDs: true, ContentImageRefs: true,
}

var knownCategories = func() map[domain.Category]bool {
	m := map[domain.Category]bool{}
	for _, c := range domain.AllCategories {
		m[c] = true
	}
	return m
}()

// sampleContext is used to check that templates render.
func sampleContext(d *ProductDefinition) RenderContext {
	v := domain.MustVersion(d.Versioning.TagPrefix+"1.2.3", "1.2.3")
	rc := NewRenderContext(d.ID, v, nil)
	rc.ArtifactVersion = "1.2.3"
	return rc
}

// Validate performs static validation of a definition. It does not touch the
// network; historical relationship checks live in package discovery/ingest.
func Validate(d *ProductDefinition) ValidationReport {
	v := &validator{r: ValidationReport{Product: d.ID, File: d.Path()}}
	if d.APIVersion != APIVersion {
		v.errf("apiVersion", "must be %q, got %q", APIVersion, d.APIVersion)
	}
	if d.Kind != Kind {
		v.errf("kind", "must be %q, got %q", Kind, d.Kind)
	}
	if !idRe.MatchString(d.ID) {
		v.errf("id", "must be a lowercase DNS label, got %q", d.ID)
	}
	if strings.TrimSpace(d.Name) == "" {
		v.errf("name", "required")
	}
	v.versioning(d)
	rc := sampleContext(d)

	sourceIDs := map[string]bool{}
	hasVersions := false
	for i, s := range d.Sources {
		p := fmt.Sprintf("sources[%d]", i)
		if !idRe.MatchString(s.ID) {
			v.errf(p+".id", "must be a lowercase DNS label, got %q", s.ID)
		}
		if sourceIDs[s.ID] {
			v.errf(p+".id", "duplicate source id %q", s.ID)
		}
		sourceIDs[s.ID] = true
		if len(s.Roles) == 0 {
			v.errf(p+".roles", "at least one role required")
		}
		for j, r := range s.Roles {
			if !knownRoles[r] {
				v.errf(fmt.Sprintf("%s.roles[%d]", p, j), "unknown role %q", r)
			}
			if r == domain.RoleVersions {
				hasVersions = true
			}
		}
		v.locator(p+".locator", s.Locator, rc)
		if s.Extract != nil {
			v.extract(p+".extract", *s.Extract, rc)
		}
		for j, cr := range s.Classify {
			v.classify(fmt.Sprintf("%s.classify[%d]", p, j), cr)
		}
		v.constraint(p+".availability", s.Availability)
		v.exceptions(p+".exceptions", s.Exceptions)
		for j, k := range s.ReleaseKinds {
			if k != "major" && k != "minor" && k != "patch" {
				v.errf(fmt.Sprintf("%s.releaseKinds[%d]", p, j), "must be major, minor or patch, got %q", k)
			}
		}
		if s.FallbackGroup != "" && !idRe.MatchString(s.FallbackGroup) {
			v.errf(p+".fallbackGroup", "must be a lowercase DNS label, got %q", s.FallbackGroup)
		}
		if s.HasRole(domain.RoleVersions) && s.Locator.Kind != LocatorGitHubReleases && s.Locator.Kind != LocatorGitTags && s.Locator.Kind != LocatorHelmRepo {
			v.errf(p+".locator.kind", "a versions source must use %s, %s or %s", LocatorGitHubReleases, LocatorGitTags, LocatorHelmRepo)
		}
		if s.HasRole(domain.RoleCompatibility) && (s.Extract == nil || (s.Extract.Type != ExtractMarkdownTable && s.Extract.Type != ExtractYAMLRecords)) {
			v.warnf(p+".extract", "compatibility sources should use a %s or %s extract to yield structured constraints", ExtractMarkdownTable, ExtractYAMLRecords)
		}
	}
	if !hasVersions {
		v.errf("sources", "at least one source must have role %q", domain.RoleVersions)
	}
	if len(d.SourcesWithRole(domain.RoleReleaseNotes))+len(d.SourcesWithRole(domain.RoleChangelog)) == 0 {
		v.warnf("sources", "no release-notes or changelog source declared")
	}

	artIDs := map[string]bool{}
	for _, a := range d.Artifacts {
		artIDs[a.ID] = true
	}
	seen := map[string]bool{}
	for i, a := range d.Artifacts {
		p := fmt.Sprintf("artifacts[%d]", i)
		if !idRe.MatchString(a.ID) {
			v.errf(p+".id", "must be a lowercase DNS label, got %q", a.ID)
		}
		if seen[a.ID] {
			v.errf(p+".id", "duplicate artifact id %q", a.ID)
		}
		seen[a.ID] = true
		if sourceIDs[a.ID] {
			v.warnf(p+".id", "artifact id %q also used by a source", a.ID)
		}
		if !knownArtifactTypes[a.Type] {
			v.errf(p+".type", "unknown artifact type %q", a.Type)
		}
		if strings.TrimSpace(a.Name) == "" {
			v.errf(p+".name", "required")
		}
		v.versionRelation(p+".version", a.Version, rc)
		if len(a.Channels) == 0 {
			v.errf(p+".channels", "at least one channel required")
		}
		allowed := channelKinds[a.Type]
		for j, ch := range a.Channels {
			cp := fmt.Sprintf("%s.channels[%d]", p, j)
			v.locator(cp, ch, rc)
			if len(allowed) > 0 && !contains(allowed, ch.Kind) {
				v.errf(cp+".kind", "channel kind %q not valid for artifact type %s (allowed: %s)", ch.Kind, a.Type, strings.Join(allowed, ", "))
			}
		}
		for j, ref := range a.References {
			rp := fmt.Sprintf("%s.references[%d]", p, j)
			if !artIDs[ref.Artifact] {
				v.errf(rp+".artifact", "unknown artifact %q", ref.Artifact)
			}
			if ref.Artifact == a.ID {
				v.errf(rp+".artifact", "artifact cannot reference itself")
			}
			if ref.Pattern == "" {
				v.errf(rp+".pattern", "required")
			} else if _, err := Render(ref.Pattern, rc); err != nil {
				v.errf(rp+".pattern", "%v", err)
			}
		}
		for j, c := range a.Contents {
			cp := fmt.Sprintf("%s.contents[%d]", p, j)
			if !knownContent[c.Kind] {
				v.errf(cp+".kind", "unknown content kind %q", c.Kind)
			}
			if c.Locator != nil {
				v.locator(cp+".locator", *c.Locator, rc)
			} else if !hasKind(a.Channels, LocatorHTTP) && !hasKind(a.Channels, LocatorRepoFile) && !hasKind(a.Channels, LocatorRepoDir) {
				v.errf(cp+".locator", "required when the artifact has no http/repo-file/repo-dir channel")
			}
			v.constraint(cp+".availability", c.Availability)
			if c.StripPrefix != "" && c.Kind != ContentHelmValues {
				v.errf(cp+".stripPrefix", "only supported for %s contents", ContentHelmValues)
			}
			if len(c.IgnoreKeys) > 0 && c.Kind != ContentHelmValues {
				v.errf(cp+".ignoreKeys", "only supported for %s contents", ContentHelmValues)
			}
			for k, key := range c.IgnoreKeys {
				kp := fmt.Sprintf("%s.ignoreKeys[%d]", cp, k)
				base := strings.TrimSuffix(key, ".*")
				switch {
				case strings.TrimSpace(key) == "" || strings.TrimSpace(key) != key:
					v.errf(kp, "must be a values key without surrounding whitespace, got %q", key)
				case base == "" || (base == key && strings.Contains(key, "*")) || strings.Contains(base, "*"):
					v.errf(kp, `a wildcard is only supported as a trailing ".*" after a key, got %q`, key)
				}
			}
		}
		v.constraint(p+".availability", a.Availability)
		v.exceptions(p+".exceptions", a.Exceptions)
	}
	return v.r
}

func (v *validator) exceptions(path string, exs []Exception) {
	for i, e := range exs {
		p := fmt.Sprintf("%s[%d]", path, i)
		if len(e.Versions) == 0 {
			v.errf(p+".versions", "at least one version required")
		}
		for j, s := range e.Versions {
			if _, err := semver.StrictNewVersion(s); err != nil {
				v.errf(fmt.Sprintf("%s.versions[%d]", p, j), "must be a semantic version without prefix, got %q", s)
			}
		}
		if strings.TrimSpace(e.Reason) == "" {
			v.errf(p+".reason", "required: exceptions must explain themselves")
		}
	}
}

func (v *validator) versioning(d *ProductDefinition) {
	if d.Versioning.Scheme != domain.SchemeSemver {
		v.errf("versioning.scheme", "only %q is supported in this prototype, got %q", domain.SchemeSemver, d.Versioning.Scheme)
	}
	if d.Versioning.TagPattern != "" {
		re, err := regexp.Compile(d.Versioning.TagPattern)
		if err != nil {
			v.errf("versioning.tagPattern", "%v", err)
		} else if re.SubexpIndex("version") < 0 {
			v.errf("versioning.tagPattern", "must contain a named group (?P<version>...)")
		}
	}
	switch d.Versioning.Lineage {
	case "", LineageMinor, LineageLinear:
	default:
		v.errf("versioning.lineage", "must be %q or %q", LineageMinor, LineageLinear)
	}
	if p, err := d.VersionParser(); err == nil {
		sample := d.Versioning.TagPrefix + "1.2.3"
		if _, err := p.Parse(sample); err != nil {
			v.warnf("versioning", "sample tag %q does not parse: %v", sample, err)
		}
	}
}

func (v *validator) locator(path string, l Locator, rc RenderContext) {
	req, ok := locatorSpec[l.Kind]
	if !ok {
		v.errf(path+".kind", "unknown locator kind %q", l.Kind)
		return
	}
	get := map[string]string{
		"repository": l.Repository, "path": l.Path, "url": l.URL, "chart": l.Chart, "tagPattern": l.TagPattern, "ref": l.Ref,
	}
	for _, f := range req {
		if strings.TrimSpace(get[f]) == "" {
			v.errf(path+"."+f, "required for kind %s", l.Kind)
		}
	}
	if _, err := RenderLocator(l, rc); err != nil {
		v.errf(path, "%v", err)
	}
	if l.TagPattern != "" {
		re, err := regexp.Compile(l.TagPattern)
		if err != nil {
			v.errf(path+".tagPattern", "%v", err)
		} else if re.SubexpIndex("version") < 0 {
			v.errf(path+".tagPattern", "must contain a named group (?P<version>...)")
		}
	}
	if l.BaseRef != "" && l.Kind != LocatorRepoDir {
		v.errf(path+".baseRef", "only supported for kind %s", LocatorRepoDir)
	}
	if l.URL != "" && !strings.HasPrefix(l.URL, "https://") && !strings.HasPrefix(l.URL, "http://") {
		v.errf(path+".url", "must be an http(s) URL")
	}
	if (l.Kind == LocatorGitHubReleases || l.Kind == LocatorGitHubAdvisories) && strings.Count(l.Repository, "/") != 1 {
		v.errf(path+".repository", "must be owner/name for %s", l.Kind)
	}
}

func (v *validator) extract(path string, e Extract, rc RenderContext) {
	switch e.Type {
	case ExtractWhole, ExtractReleaseNoteYAML:
	case ExtractMarkdownSection:
		if e.Heading == "" {
			v.errf(path+".heading", "required for %s", e.Type)
		} else {
			v.regexTemplate(path+".heading", e.Heading, rc)
		}
	case ExtractMarkdownTable, ExtractYAMLRecords:
		if len(e.KeyColumns) == 0 {
			v.errf(path+".keyColumns", "required for %s", e.Type)
		}
		if e.KeyMatch == "" {
			v.errf(path+".keyMatch", "required for %s", e.Type)
		} else {
			v.regexTemplate(path+".keyMatch", e.KeyMatch, rc)
		}
		if len(e.Columns) == 0 {
			v.errf(path+".columns", "at least one column required for %s", e.Type)
		}
		for j, c := range e.Columns {
			cp := fmt.Sprintf("%s.columns[%d]", path, j)
			if c.Platform == "" {
				v.errf(cp+".platform", "required")
			}
			if len(c.Headers) == 0 {
				v.errf(cp+".headers", "at least one header required")
			}
			if c.Part < 0 || (c.Part > 0 && c.Separator == "") {
				v.errf(cp+".part", "part requires a separator and must be >= 0")
			}
			switch c.Kind {
			case "", "supported", "tested", "minimum":
			default:
				v.errf(cp+".kind", "must be supported, tested or minimum")
			}
		}
		if e.TableHeading != "" {
			v.regexTemplate(path+".tableHeading", e.TableHeading, rc)
		}
	default:
		v.errf(path+".type", "unknown extract type %q", e.Type)
	}
}

func (v *validator) regexTemplate(path, s string, rc RenderContext) {
	r, err := Render(s, rc)
	if err != nil {
		v.errf(path, "%v", err)
		return
	}
	if _, err := regexp.Compile(r); err != nil {
		v.errf(path, "rendered regex invalid: %v", err)
	}
}

func (v *validator) classify(path string, cr ClassifyRule) {
	if cr.Section == "" && cr.Text == "" {
		v.errf(path, "at least one of section or text is required")
	}
	for f, re := range map[string]string{"section": cr.Section, "text": cr.Text} {
		if re == "" {
			continue
		}
		if _, err := regexp.Compile(re); err != nil {
			v.errf(path+"."+f, "%v", err)
		}
	}
	if !cr.Skip && cr.Category == "" && cr.Breaking == nil && cr.ActionRequired == nil {
		v.errf(path, "rule must set category, breaking, actionRequired or skip")
	}
	if cr.Category != "" && !knownCategories[cr.Category] {
		v.errf(path+".category", "unknown category %q", cr.Category)
	}
}

func (v *validator) versionRelation(path string, vr VersionRelation, rc RenderContext) {
	switch vr.Strategy {
	case VersionTemplate:
		if vr.Template == "" {
			v.errf(path+".template", "required for strategy template")
		} else if _, err := Render(vr.Template, rc); err != nil {
			v.errf(path+".template", "%v", err)
		}
	case VersionLookup:
		if vr.Field == "" {
			v.errf(path+".field", "required for strategy lookup")
		}
		if vr.Match == "" {
			v.errf(path+".match", "required for strategy lookup")
		} else if _, err := Render(vr.Match, rc); err != nil {
			v.errf(path+".match", "%v", err)
		}
		switch vr.Select {
		case "", "latest", "earliest", "all":
		default:
			v.errf(path+".select", "must be latest, earliest or all")
		}
	case VersionIndependent:
	default:
		v.errf(path+".strategy", "must be template, lookup or independent, got %q", vr.Strategy)
	}
}

func (v *validator) constraint(path, c string) {
	if strings.TrimSpace(c) == "" {
		return
	}
	if _, err := semver.NewConstraint(c); err != nil {
		v.errf(path, "invalid semver constraint %q: %v", c, err)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func hasKind(ls []Locator, kind string) bool {
	for _, l := range ls {
		if l.Kind == kind {
			return true
		}
	}
	return false
}
