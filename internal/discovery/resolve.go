package discovery

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Element origins.
const (
	OriginDeterministic = "deterministic"
	OriginAI            = "ai"
)

// Element describes one source or artifact of a draft definition and how
// it was obtained.
type Element struct {
	Key        string            `json:"key"`  // "source:<id>" or "artifact:<id>"
	Kind       string            `json:"kind"` // "source" | "artifact"
	ID         string            `json:"id"`
	Origin     string            `json:"origin"`
	Rule       string            `json:"rule"`
	Confidence domain.Confidence `json:"confidence"`
	Candidates []string          `json:"candidates,omitempty"`
	ProposalID string            `json:"proposal,omitempty"`
	// Replaces names the deterministic element an AI variant would replace.
	Replaces string `json:"replaces,omitempty"`
	// Targets lists the discovery targets the element serves.
	Targets []string `json:"targets,omitempty"`
}

// Question is an ambiguity the deterministic resolver could not settle; the
// optional LLM resolver may answer it.
type Question struct {
	Kind       string      `json:"kind"`
	Subject    string      `json:"subject,omitempty"` // element key
	Summary    string      `json:"summary"`
	Candidates []Candidate `json:"-"`
	// Paths is extra context (e.g. a documentation repository listing).
	Paths []string          `json:"-"`
	Facts map[string]string `json:"-"`
}

// Question kinds.
const (
	QuestionRegistryRoles = "registry-roles"
	QuestionDocsSources   = "docs-sources"
	QuestionMainChart     = "main-chart"
	QuestionChartVersion  = "chart-version"
)

// Draft is the resolver output: a definition plus the bookkeeping needed to
// validate and explain it.
type Draft struct {
	Definition *catalog.ProductDefinition `json:"-"`
	Elements   map[string]*Element        `json:"elements"`
	Decisions  []Decision                 `json:"decisions"`
	Questions  []Question                 `json:"questions,omitempty"`
	Open       []string                   `json:"openQuestions,omitempty"`
	// AI holds AI-proposed elements awaiting validation, keyed like Elements.
	AISources   []catalog.Source   `json:"-"`
	AIArtifacts []catalog.Artifact `json:"-"`
}

// ResolveInput is everything the resolver needs.
type ResolveInput struct {
	Repo      RepoRef
	ProductID string
	Name      string
	Tags      *TagAnalysis
	// ScanRef is the release the main tree was scanned at (zero for a branch).
	ScanRef    domain.Version
	Candidates []Candidate
	// ChartRepoTags holds tags of external chart repositories, keyed by
	// candidate value (host/owner/name).
	ChartRepoTags map[string][]RemoteTag
	// DocsListings holds documentation repository paths worth showing to the
	// LLM, keyed by repository.
	DocsListings map[string][]string
	Clock        func() time.Time
}

// resolver carries state while building a draft.
type resolver struct {
	in    ResolveInput
	def   *catalog.ProductDefinition
	draft *Draft
	ids   map[string]bool
	byK   map[CandidateKind][]Candidate
	hints ProductHints
	now   time.Time
}

var idCleanRe = regexp.MustCompile(`[^a-z0-9-]+`)

// sanitizeID returns a lowercase DNS label.
func sanitizeID(s string) string {
	s = idCleanRe.ReplaceAllString(strings.ToLower(s), "-")
	s = strings.Trim(regexp.MustCompile(`-{2,}`).ReplaceAllString(s, "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	if s == "" {
		s = "x"
	}
	return s
}

func (r *resolver) uniqueID(base string) string {
	id := sanitizeID(base)
	if !r.ids[id] {
		r.ids[id] = true
		return id
	}
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s-%d", id, i)
		if !r.ids[c] {
			r.ids[c] = true
			return c
		}
	}
}

func (r *resolver) decide(element, action, rule, rationale string, conf domain.Confidence, method domain.Method, cands ...Candidate) {
	var ids []string
	for _, c := range cands {
		if c.ID != "" && !containsStr(ids, c.ID) {
			ids = append(ids, c.ID)
		}
	}
	if conf == "" {
		conf = domain.ConfidenceMedium
	}
	if method == "" {
		method = domain.MethodHeuristic
	}
	r.draft.Decisions = append(r.draft.Decisions, Decision{
		Element: element, Action: action, Rule: rule, Rationale: rationale, Candidates: ids,
		Provenance: domain.Provenance{Method: method, Producer: ProducerResolve, Rule: rule, Confidence: conf},
	})
}

func (r *resolver) addSource(s catalog.Source, rule string, conf domain.Confidence, targets []string, rationale string, cands ...Candidate) string {
	s.ID = r.uniqueID(s.ID)
	r.def.Sources = append(r.def.Sources, s)
	key := "source:" + s.ID
	e := &Element{Key: key, Kind: "source", ID: s.ID, Origin: OriginDeterministic, Rule: rule, Confidence: conf, Targets: targets}
	for _, c := range cands {
		e.Candidates = append(e.Candidates, c.ID)
	}
	r.draft.Elements[key] = e
	r.decide(key, "include", rule, rationale, conf, domain.MethodHeuristic, cands...)
	return s.ID
}

func (r *resolver) addArtifact(a catalog.Artifact, rule string, conf domain.Confidence, targets []string, rationale string, cands ...Candidate) string {
	a.ID = r.uniqueID(a.ID)
	r.def.Artifacts = append(r.def.Artifacts, a)
	key := "artifact:" + a.ID
	e := &Element{Key: key, Kind: "artifact", ID: a.ID, Origin: OriginDeterministic, Rule: rule, Confidence: conf, Targets: targets}
	for _, c := range cands {
		e.Candidates = append(e.Candidates, c.ID)
	}
	r.draft.Elements[key] = e
	r.decide(key, "include", rule, rationale, conf, domain.MethodHeuristic, cands...)
	return a.ID
}

func (r *resolver) exclude(c Candidate, rule, rationale string) {
	r.decide("candidate:"+c.ID, "exclude", rule, rationale, c.Confidence, domain.MethodHeuristic, c)
}

// Resolve builds a draft definition from candidates with deterministic rules.
func Resolve(in ResolveInput) (*Draft, error) {
	if in.Tags == nil {
		in.Tags = AnalyzeTags(in.Repo, nil)
	}
	now := time.Now()
	if in.Clock != nil {
		now = in.Clock()
	}
	r := &resolver{in: in, ids: map[string]bool{}, byK: map[CandidateKind][]Candidate{}, now: now.UTC()}
	r.draft = &Draft{Elements: map[string]*Element{}}
	for _, c := range in.Candidates {
		r.byK[c.Kind] = append(r.byK[c.Kind], c)
	}
	id := in.ProductID
	if id == "" {
		id = in.Repo.Name
	}
	name := in.Name
	if name == "" {
		name = in.Repo.Name
	}
	r.hints = ProductHints{Owner: in.Repo.Owner, Name: in.Repo.Name, ID: id}
	r.def = &catalog.ProductDefinition{
		APIVersion: catalog.APIVersion, Kind: catalog.Kind, ID: sanitizeID(id), Name: name,
		Homepage: "https://" + in.Repo.String(),
	}
	r.draft.Definition = r.def
	r.versioning()
	r.versionSources()
	r.notesSources()
	r.changelogSources()
	r.upgradeSources()
	r.compatSources()
	r.securitySources()
	r.artifacts()
	r.questions()
	return r.draft, nil
}

func (r *resolver) versioning() {
	ta := r.in.Tags
	r.def.Versioning = catalog.Versioning{Scheme: domain.SchemeSemver, TagPrefix: ta.Prefix, TagPattern: ta.TagPattern, Lineage: ta.Lineage}
	if r.def.Versioning.Lineage == "" {
		r.def.Versioning.Lineage = catalog.LineageMinor
	}
	cands := r.byK[KindTagScheme]
	rationale := ta.Summary()
	if ta.TagPattern != "" {
		rationale += "; strict tagPattern excludes junk tags such as " + strings.Join(firstN(ta.Junk, 4), ", ")
	}
	for _, t := range r.byK[KindReleaseTrigger] {
		if ta.Prefix != "" && !strings.HasPrefix(t.Value, ta.Prefix) && !strings.HasPrefix(t.Value, "*") {
			r.draft.Open = append(r.draft.Open, fmt.Sprintf("Release workflow triggers on tags %q, which does not match the tag prefix %q.", t.Value, ta.Prefix))
		}
		cands = append(cands, t)
	}
	r.decide("versioning", "include", "tags.scheme", rationale, domain.ConfidenceHigh, domain.MethodComputed, cands...)
	if ta.StableCount == 0 {
		r.draft.Open = append(r.draft.Open, "No stable release tags were found; the tag scheme could not be inferred.")
	}
}

func firstN(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

func (r *resolver) versionSources() {
	repo := r.in.Repo
	s := catalog.Source{ID: "tags", Roles: []domain.SourceRole{domain.RoleVersions},
		Locator: catalog.Locator{Kind: catalog.LocatorGitTags, Repository: repo.String(), TagPattern: r.in.Tags.TagPattern}}
	r.addSource(s, "versions.git-tags", domain.ConfidenceHigh, []string{TargetReleaseSource},
		"Canonical versions come from the repository's git tags ("+r.in.Tags.Summary()+").", r.byK[KindTagScheme]...)
	if !repo.IsGitHub() {
		return
	}
	pubs := append(append([]Candidate{}, r.byK[KindReleasePublisher]...), r.ownAssets()...)
	if len(pubs) == 0 {
		return
	}
	fb := catalog.Source{ID: "github-releases", Roles: []domain.SourceRole{domain.RoleVersions}, Priority: 1, Fallback: true,
		Locator: catalog.Locator{Kind: catalog.LocatorGitHubReleases, Repository: repo.Slug()}}
	r.addSource(fb, "versions.github-releases-fallback", domain.ConfidenceMedium, []string{TargetReleaseSource},
		"Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list.", pubs...)
}

// ownAssets returns resolved release-asset candidates of the product repo.
func (r *resolver) ownAssets() []Candidate {
	var out []Candidate
	for _, c := range r.byK[KindReleaseAsset] {
		if c.Attr("unresolved") == "" {
			out = append(out, c)
		}
	}
	return out
}

// repoOf returns the repository a candidate was found in.
func (r *resolver) repoOf(c Candidate) (RepoRef, string, bool) {
	if v := c.Attr("repo"); v != "" {
		rr, err := ParseRepo(v)
		if err == nil {
			return rr, c.Attr("ref"), false
		}
	}
	return r.in.Repo, "", true
}

// repoFile builds a repo-file locator for a candidate path.
func (r *resolver) repoFile(c Candidate, p string) catalog.Locator {
	rr, ref, main := r.repoOf(c)
	l := catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: rr.String(), Path: p}
	if !main {
		l.Ref = ref
	}
	return l
}

// docScore ranks versioned-documentation path templates.
func (r *resolver) docScore(c Candidate, strong *regexp.Regexp) int {
	score := 0
	covered := strings.Split(c.Attr("covered"), "/")
	if len(covered) == 2 {
		score += 10 * atoiDefault(covered[0], 0)
	}
	if r.coversNewestLine(c) {
		score += 100
	}
	score += minInt(atoiDefault(c.Attr("instances"), 0), 30)
	base := path.Base(c.Value)
	if strings.Contains(base, "{{") {
		score += 15 // the version is in the file name, not a docs-snapshot directory
	}
	if strong != nil && strong.MatchString(c.Value) {
		score += 20
	}
	if strings.HasPrefix(base, "_index") {
		score -= 10
	}
	if archivedDocRe.MatchString(c.Value) {
		score -= 60 // archived copies of old docs sites
	}
	if strings.HasSuffix(strings.ToLower(c.Value), ".html") {
		score -= 15 // rendered output, prefer the source document
	}
	if _, _, main := r.repoOf(c); main {
		score += 5
	}
	if c.Confidence == domain.ConfidenceHigh {
		score += 5
	}
	return score
}

// coversNewestLine reports whether a path-template candidate has an
// instance for one of the two newest release lines.
func (r *resolver) coversNewestLine(c Candidate) bool {
	lines := r.in.Tags.Lines
	if len(lines) == 0 {
		return true
	}
	newest := c.Attr("newest")
	pt, ok := inferPathTemplate(newest, r.in.Tags.Prefix)
	if !ok {
		return false
	}
	for i, l := range lines {
		if i >= 2 {
			break
		}
		if lineOf(pt) == l {
			return true
		}
	}
	// documentation for the next, unreleased line also counts
	if lmaj, lmin, ok := splitLine(lines[0]); ok && (pt.Major > lmaj || (pt.Major == lmaj && pt.Minor > lmin)) {
		return true
	}
	return false
}

func splitLine(l string) (uint64, uint64, bool) {
	toks := findVersionTokens(l)
	if len(toks) != 1 {
		return 0, 0, false
	}
	return toks[0].major, toks[0].minor, true
}

func bestBy(cs []Candidate, score func(Candidate) int) (Candidate, int, bool) {
	best, bestScore, found := Candidate{}, -1<<31, false
	for _, c := range cs {
		s := score(c)
		if s > bestScore || (s == bestScore && c.Value < best.Value) {
			best, bestScore, found = c, s, true
		}
	}
	return best, bestScore, found
}

var archivedDocRe = regexp.MustCompile(`(?i)(^|/)(archives?|archived|old|legacy|attic)(/|$)`)

var strongNotesRe = regexp.MustCompile(`(?i)release[-_]?notes|change[-_]?notes|changelog|releasenotes`)

func (r *resolver) notesSources() {
	var line, patch []Candidate
	var body *Candidate
	for _, c := range r.byK[KindReleaseNotes] {
		c := c
		switch {
		case c.Value == "github-release-body":
			body = &c
		case c.Attr("level") == "patch":
			patch = append(patch, c)
		default:
			line = append(line, c)
		}
	}
	priority := 0
	if body != nil && r.in.Repo.IsGitHub() {
		s := catalog.Source{ID: "release-notes", Roles: []domain.SourceRole{domain.RoleReleaseNotes}, Priority: priority,
			Locator: catalog.Locator{Kind: catalog.LocatorGitHubReleases, Repository: r.in.Repo.Slug(), Ref: tmplTag},
			Extract: &catalog.Extract{Type: catalog.ExtractWhole},
			Notes:   "GitHub release body generated by " + body.Attr("generator") + " (groups: " + body.Attr("groups") + ")"}
		r.addSource(s, "notes.generated-release-body", domain.ConfidenceHigh, []string{TargetReleaseNotes},
			"The release pipeline generates the hosted release body from commit/PR titles.", *body)
		priority++
	}
	scoreFn := func(c Candidate) int { return r.docScore(c, strongNotesRe) }
	bestLine, ls, okLine := bestBy(line, scoreFn)
	bestPatch, ps, okPatch := bestBy(patch, scoreFn)
	if okLine && ls < 100 {
		okLine = false
	}
	if okPatch && ps < 100 {
		okPatch = false
	}
	for _, c := range line {
		if !okLine || c.ID != bestLine.ID {
			r.exclude(c, "notes.lower-ranked-template", "Another release-notes path template ranks higher (coverage of recent release lines, keyword strength).")
		}
	}
	for _, c := range patch {
		if !okPatch || c.ID != bestPatch.ID {
			r.exclude(c, "notes.lower-ranked-template", "Another per-release notes path template ranks higher.")
		}
	}
	fallback := priority > 0
	switch {
	case okLine && bestLine.Attr("sectionHeadings") == "true":
		s := catalog.Source{ID: "release-notes-docs", Roles: []domain.SourceRole{domain.RoleReleaseNotes}, Priority: priority, Fallback: fallback,
			Locator: r.repoFile(bestLine, bestLine.Value),
			Extract: &catalog.Extract{Type: catalog.ExtractMarkdownSection, Heading: `^v?{{regexQuote .Version}}$`}}
		r.addSource(s, "notes.per-line-file-with-release-sections", domain.ConfidenceHigh, []string{TargetReleaseNotes},
			fmt.Sprintf("One notes file per release line (%s instances) with one section per release (e.g. %q).", bestLine.Attr("instances"), bestLine.Attr("headingSample")), bestLine)
		priority++
	default:
		if okPatch {
			kinds := []string{"patch"}
			if bestPatch.Attr("zeroPatch") == "true" || !okLine {
				kinds = nil
			}
			s := catalog.Source{ID: "release-notes-docs", Roles: []domain.SourceRole{domain.RoleReleaseNotes}, Priority: priority, Fallback: fallback,
				Locator: r.repoFile(bestPatch, bestPatch.Value), ReleaseKinds: kinds}
			r.addSource(s, "notes.per-release-file", domain.ConfidenceHigh, []string{TargetReleaseNotes},
				fmt.Sprintf("One notes document per release (%s instances, newest %s).", bestPatch.Attr("instances"), bestPatch.Attr("newest")), bestPatch)
		}
		if okLine {
			s := catalog.Source{ID: "release-notes-line", Roles: []domain.SourceRole{domain.RoleReleaseNotes}, Priority: priority, Fallback: fallback,
				Locator: r.repoFile(bestLine, bestLine.Value), ReleaseKinds: []string{"minor", "major"}}
			r.addSource(s, "notes.per-line-file", domain.ConfidenceMedium, []string{TargetReleaseNotes},
				fmt.Sprintf("One notes document per release line (%s instances, newest %s), used for X.Y.0 releases.", bestLine.Attr("instances"), bestLine.Attr("newest")), bestLine)
		}
		if okPatch || okLine {
			priority++
		}
	}
	for _, c := range r.byK[KindNotesDir] {
		if _, _, main := r.repoOf(c); !main {
			continue
		}
		glob := c.Attr("glob")
		if glob == "" {
			glob = "*.yaml"
		}
		s := catalog.Source{ID: "release-note-files", Roles: []domain.SourceRole{domain.RoleReleaseNotes}, Priority: priority, Fallback: priority > 0,
			Locator: catalog.Locator{Kind: catalog.LocatorRepoDir, Repository: r.in.Repo.String(), Path: c.Value, Glob: glob, BaseRef: "{{.PrevTag}}"},
			Extract: &catalog.Extract{Type: catalog.ExtractReleaseNoteYAML},
			Notes:   "Structured per-change notes accumulate in this directory; a release's notes are the files added since the previous release."}
		conf := c.Confidence
		r.addSource(s, "notes.structured-dir", conf, []string{TargetReleaseNotes},
			fmt.Sprintf("Directory of %s structured note files (keys: %s).", c.Attr("fileCount"), c.Attr("keys")), c)
		priority++
		break
	}
	if body == nil && r.in.Repo.IsGitHub() {
		pubs := append(append([]Candidate{}, r.byK[KindReleasePublisher]...), r.ownAssets()...)
		if len(pubs) > 0 {
			s := catalog.Source{ID: "release-notes-github", Roles: []domain.SourceRole{domain.RoleReleaseNotes}, Priority: priority, Fallback: priority > 0,
				Locator: catalog.Locator{Kind: catalog.LocatorGitHubReleases, Repository: r.in.Repo.Slug(), Ref: tmplTag}}
			r.addSource(s, "notes.hosted-release-body", domain.ConfidenceMedium, []string{TargetReleaseNotes},
				"Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes.", pubs...)
		}
	}
}

func (r *resolver) changelogSources() {
	cs := r.byK[KindChangelog]
	sort.Slice(cs, func(i, j int) bool { return strings.Count(cs[i].Value, "/") < strings.Count(cs[j].Value, "/") })
	done := false
	for _, c := range cs {
		if _, _, main := r.repoOf(c); !main || done {
			r.exclude(c, "changelog.not-primary", "Only the top-level changelog of the product repository is used.")
			continue
		}
		newest := c.Attr("newest")
		if stale, why := r.staleChangelog(newest); stale {
			r.exclude(c, "changelog.stale", "Changelog is not maintained: "+why+".")
			r.draft.Open = append(r.draft.Open, fmt.Sprintf("%s stops at %s; confirm it is abandoned (release notes come from other sources).", c.Value, newest))
			continue
		}
		heading := `^\[?v?{{regexQuote .Version}}\]?(\s|$)`
		s := catalog.Source{ID: "changelog", Roles: []domain.SourceRole{domain.RoleChangelog},
			Locator: r.repoFile(c, c.Value), Extract: &catalog.Extract{Type: catalog.ExtractMarkdownSection, Heading: heading}}
		r.addSource(s, "changelog.file", domain.ConfidenceHigh, []string{TargetChangelog},
			fmt.Sprintf("Changelog with %s version sections, newest %s (heading %q).", c.Attr("entries"), newest, c.Attr("headingSample")), c)
		done = true
	}
}

// staleChangelog reports whether a changelog whose newest entry is newest
// lags the tags by more than one release line.
func (r *resolver) staleChangelog(newest string) (bool, string) {
	latest, ok := r.in.Tags.LatestVersion()
	if !ok || newest == "" {
		return false, ""
	}
	toks := findVersionTokens(newest)
	if len(toks) == 0 {
		return false, ""
	}
	t := toks[0]
	if len(r.in.Tags.Lines) >= 2 {
		pm, pmin, _ := splitLine(r.in.Tags.Lines[1])
		if t.major > pm || (t.major == pm && t.minor >= pmin) {
			return false, ""
		}
	} else if t.major == latest.Major() && t.minor == latest.Minor() {
		return false, ""
	}
	return true, fmt.Sprintf("newest entry %s while the latest release is %s", newest, latest.Tag)
}

var strongUpgradeRe = regexp.MustCompile(`(?i)upgrad`)

func (r *resolver) upgradeSources() {
	cs := r.byK[KindUpgradeGuide]
	score := func(c Candidate) int {
		s := r.docScore(c, strongUpgradeRe)
		if c.Attr("level") == "pair" {
			s += 20
		}
		return s
	}
	best, bs, ok := bestBy(cs, score)
	for _, c := range cs {
		if !ok || bs < 100 || c.ID != best.ID {
			r.exclude(c, "upgrade.lower-ranked-template", "Another upgrade-guide path template ranks higher or this one does not cover the newest release lines.")
		}
	}
	if !ok || bs < 100 {
		return
	}
	s := catalog.Source{ID: "upgrade-guide", Roles: []domain.SourceRole{domain.RoleUpgradeGuide}, ReleaseKinds: []string{"minor", "major"},
		Locator: r.repoFile(best, best.Value)}
	why := fmt.Sprintf("One upgrade document per release line (%s instances, newest %s)", best.Attr("instances"), best.Attr("newest"))
	if best.Attr("level") == "pair" {
		why += "; the file name names the previous and the new line, so {{.PrevMajor}}.{{.PrevMinor}} (the previous existing line) is used"
	}
	r.addSource(s, "upgrade.versioned-doc", best.Confidence, []string{TargetUpgradeDocs}, why+".", best)
}

func (r *resolver) compatSources() {
	var content, paths []Candidate
	for _, c := range r.byK[KindCompatibility] {
		if c.Attr("format") != "" {
			content = append(content, c)
		} else {
			paths = append(paths, c)
		}
	}
	score := func(c Candidate) int {
		s := 0
		if len(findVersionTokens(c.Value)) == 0 {
			s += 50 // current docs, not a per-version snapshot
		}
		if c.Attr("pathRole") == "true" || c.Attr("format") == "yaml-records" {
			s += 20
		}
		s += minInt(atoiDefault(c.Attr("rows"), 0), 30)
		if _, _, main := r.repoOf(c); main {
			s += 5
		}
		return s
	}
	best, _, ok := bestBy(content, score)
	for _, c := range content {
		if ok && c.ID != best.ID {
			r.exclude(c, "compat.lower-ranked", "Another compatibility table ranks higher (current docs, rows keyed by product release lines).")
		}
	}
	for _, c := range paths {
		r.exclude(c, "compat.no-structured-table", "Path suggests compatibility information but no table keyed by release lines with a Kubernetes column was found.")
	}
	if !ok {
		return
	}
	var ex *catalog.Extract
	tested := strings.Contains(strings.ToLower(best.Value), "tested")
	switch best.Attr("format") {
	case "yaml-records":
		ex = &catalog.Extract{Type: catalog.ExtractYAMLRecords, KeyColumns: splitList(best.Attr("keyColumns")), KeyMatch: `^{{regexQuote .Line}}$`}
	default:
		ex = &catalog.Extract{Type: catalog.ExtractMarkdownTable, KeyColumns: splitList(best.Attr("keyColumns")), KeyMatch: `^v?{{regexQuote .Line}}\b`}
	}
	ex.Columns = compatColumns(best, tested)
	if len(ex.Columns) == 0 {
		r.exclude(best, "compat.no-kubernetes-column", "No Kubernetes column usable.")
		return
	}
	s := catalog.Source{ID: "compatibility", Roles: []domain.SourceRole{domain.RoleCompatibility}, Locator: r.repoFile(best, best.Value), Extract: ex}
	r.addSource(s, "compat."+best.Attr("format"), best.Confidence, []string{TargetCompatibility},
		fmt.Sprintf("Support matrix keyed by release line (%s rows; key column %s; Kubernetes columns %s).", best.Attr("rows"), best.Attr("keyColumns"),
			strings.Trim(best.Attr("supportedHeaders")+","+best.Attr("testedHeaders"), ",")), best)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// compatColumns builds column specs grouped by kind and separator part.
func compatColumns(c Candidate, allTested bool) []catalog.ColumnSpec {
	parts := map[string]int{}
	for _, kv := range strings.Split(c.Attr("separatorParts"), ";") {
		if i := strings.LastIndex(kv, "="); i > 0 {
			parts[kv[:i]] = atoiDefault(kv[i+1:], 0)
		}
	}
	type key struct {
		kind string
		part int
		sep  bool
	}
	groups := map[key][]string{}
	var order []key
	add := func(kind string, headers []string) {
		for _, h := range headers {
			p, sep := parts[h]
			k := key{kind, p, sep}
			if _, ok := groups[k]; !ok {
				order = append(order, k)
			}
			groups[k] = append(groups[k], h)
		}
	}
	supKind := "supported"
	if allTested {
		supKind = "tested"
	}
	add(supKind, splitList(c.Attr("supportedHeaders")))
	add("tested", splitList(c.Attr("testedHeaders")))
	var out []catalog.ColumnSpec
	for _, k := range order {
		cs := catalog.ColumnSpec{Platform: "kubernetes", Kind: k.kind, Headers: groups[k]}
		if k.sep {
			cs.Separator, cs.Part = "/", k.part
		}
		out = append(out, cs)
	}
	return out
}

func (r *resolver) securitySources() {
	repo := r.in.Repo
	var policy []string
	for _, c := range r.byK[KindSecurityPolicy] {
		if _, _, main := r.repoOf(c); main && strings.Count(c.Value, "/") <= 1 {
			policy = append(policy, c.Value)
			if l := c.Attr("links"); l != "" {
				policy = append(policy, "→ "+l)
			}
		}
	}
	if b, _, ok := bestBy(r.byK[KindSecurityDocs], func(c Candidate) int { return atoiDefault(c.Attr("entries"), 0) }); ok {
		where := b.Value
		if repo := b.Attr("repo"); repo != "" {
			where = repo + ":" + b.Value
		}
		r.draft.Open = append(r.draft.Open, fmt.Sprintf("Security bulletins live in %s (%s entries, newest %s); no advisory adapter reads such pages, so they are not part of the definition.",
			where, b.Attr("entries"), b.Attr("newest")))
	}
	if !repo.IsGitHub() {
		if len(policy) == 0 {
			r.draft.Open = append(r.draft.Open, "No security advisory feed was found.")
		}
		return
	}
	cands := r.byK[KindAdvisories]
	conf, rule := domain.ConfidenceMedium, "security.github-hosted-default"
	why := "The repository is hosted on GitHub, whose security advisories are the default advisory feed."
	if len(cands) > 0 {
		conf, rule = domain.ConfidenceHigh, "security.advisories-referenced"
		why = "The security policy states that advisories are published as GitHub Security Advisories."
	}
	s := catalog.Source{ID: "advisories", Roles: []domain.SourceRole{domain.RoleSecurity},
		Locator: catalog.Locator{Kind: catalog.LocatorGitHubAdvisories, Repository: repo.Slug()}}
	if len(policy) > 0 {
		s.Notes = "Security policy: " + strings.Join(policy, " ")
	}
	r.addSource(s, rule, conf, []string{TargetSecurity}, why, append(cands, r.byK[KindSecurityPolicy]...)...)
}
