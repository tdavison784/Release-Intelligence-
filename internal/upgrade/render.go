package upgrade

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// DefaultMaxPerSection is used when RenderOptions.MaxPerSection is 0.
const DefaultMaxPerSection = 25

const (
	ansiBold    = "1"
	ansiDim     = "2"
	ansiRed     = "31"
	ansiGreen   = "32"
	ansiYellow  = "33"
	ansiMagenta = "35"
	ansiCyan    = "36"
)

type renderer struct {
	e        *domain.UpgradeEdge
	opts     RenderOptions
	max      int
	sb       strings.Builder
	tags     map[string]string
	cited    []domain.EvidenceID
	citedSet map[domain.EvidenceID]bool
	evByID   map[domain.EvidenceID]domain.Evidence
}

func renderText(w io.Writer, e *domain.UpgradeEdge, opts RenderOptions) error {
	if e == nil {
		return errors.New("upgrade: nil edge")
	}
	r := &renderer{e: e, opts: opts, max: opts.MaxPerSection, tags: map[string]string{}, citedSet: map[domain.EvidenceID]bool{}, evByID: map[domain.EvidenceID]domain.Evidence{}}
	if r.max == 0 {
		r.max = DefaultMaxPerSection
	}
	for _, v := range append([]domain.Version{e.From, e.To}, e.SkippedReleases...) {
		r.tags[v.Semver] = v.String()
	}
	for _, s := range e.Path {
		r.tags[s.Version.Semver] = s.Version.String()
	}
	for _, x := range e.Evidence {
		r.evByID[x.ID] = x
	}
	r.render()
	_, err := io.WriteString(w, r.sb.String())
	return err
}

func (r *renderer) paint(code, s string) string {
	if !r.opts.Color || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (r *renderer) line(format string, args ...any) {
	fmt.Fprintf(&r.sb, format, args...)
	r.sb.WriteByte('\n')
}

func (r *renderer) heading(code, title string) {
	r.sb.WriteByte('\n')
	r.line("%s", r.paint(code, title))
}

func (r *renderer) tag(semver string) string {
	if t, ok := r.tags[semver]; ok {
		return t
	}
	return semver
}

// cite records evidence ids as cited and returns a short label.
func (r *renderer) cite(ids []domain.EvidenceID) string {
	for _, id := range ids {
		if !r.citedSet[id] {
			r.citedSet[id] = true
			r.cited = append(r.cited, id)
		}
	}
	if len(ids) == 0 {
		return "no evidence"
	}
	const show = 3
	parts := make([]string, 0, show+1)
	for i, id := range ids {
		if i == show {
			parts = append(parts, fmt.Sprintf("+%d", len(ids)-show))
			break
		}
		parts = append(parts, string(id))
	}
	return strings.Join(parts, ", ")
}

func provLabel(p domain.Provenance) string {
	if p.Confidence != "" && p.Confidence != domain.ConfidenceHigh {
		return string(p.Method) + "/" + string(p.Confidence)
	}
	return string(p.Method)
}

func pad(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// limit returns how many of n items to show.
func (r *renderer) limit(n int) int {
	if r.max < 0 || n <= r.max {
		return n
	}
	return r.max
}

func (r *renderer) more(shown, total int) {
	if total > shown {
		r.line("  … and %d more", total-shown)
	}
}

func (r *renderer) render() {
	r.header()
	r.sources()
	r.changeSections()
	r.enrichments()
	r.warnings()
	r.evidence()
}

func (r *renderer) header() {
	e := r.e
	name := e.Product.Name
	if name == "" {
		name = string(e.Product.ID)
	}
	r.line("%s", r.paint(ansiBold, fmt.Sprintf("%s %s → %s", name, e.From, e.To)))

	steps := make([]string, len(e.Path))
	for i, s := range e.Path {
		steps[i] = s.Version.String()
	}
	if len(steps) > 12 {
		steps = append(append(append([]string{}, steps[:5]...), "…"), steps[len(steps)-5:]...)
	}
	path := strings.Join(steps, " → ")
	if path == "" {
		path = "(none)"
	}
	note := "policy " + e.PathPolicy
	if n := len(e.SkippedReleases); n > 0 {
		kind := "releases"
		if e.PathPolicy == PolicyMinorLineage {
			kind = "backport releases"
		}
		if n == 1 {
			kind = strings.TrimSuffix(kind, "s")
		}
		note += fmt.Sprintf("; %d %s skipped", n, kind)
	}
	if len(e.Path) > 12 {
		note += fmt.Sprintf("; %d releases traversed", len(e.Path))
	}
	r.line("Path: %s  (%s)", path, note)
	if r.opts.Verbose && len(e.SkippedReleases) > 0 {
		sk := make([]string, len(e.SkippedReleases))
		for i, v := range e.SkippedReleases {
			sk[i] = v.String()
		}
		r.line("Skipped: %s", strings.Join(sk, ", "))
	}
	breaking, action := 0, 0
	for _, c := range e.Changes {
		switch {
		case c.Breaking:
			breaking++
		case c.ActionRequired || c.Category == domain.CategoryMigration:
			action++
		}
	}
	summary := fmt.Sprintf("Summary: %d breaking · %d action required · %s · %s", breaking, action,
		plural(len(e.Changes), "change", "changes"), plural(len(e.Warnings), "warning", "warnings"))
	if len(e.Enrichments) > 0 {
		summary += " · " + plural(len(e.Enrichments), "AI enrichment", "AI enrichments")
	}
	r.line("%s", summary)
	if r.opts.Verbose {
		gen := "Generated " + e.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z")
		if e.DefinitionDigest != "" {
			gen += " · definition " + shorten(e.DefinitionDigest, 20)
		}
		r.line("%s", r.paint(ansiDim, gen))
	}
}

// --- sources ---

type srcAgg struct {
	id, kind string
	roles    []string
	entries  []domain.SourceStatus
}

func (r *renderer) sources() {
	var aggs []*srcAgg
	by := map[string]*srcAgg{}
	for _, s := range r.e.Sources {
		key := s.SourceID + "\x00" + s.Kind
		a := by[key]
		if a == nil {
			a = &srcAgg{id: s.SourceID, kind: s.Kind}
			by[key] = a
			aggs = append(aggs, a)
		}
		for _, role := range s.Roles {
			found := false
			for _, x := range a.roles {
				if x == string(role) {
					found = true
				}
			}
			if !found {
				a.roles = append(a.roles, string(role))
			}
		}
		a.entries = append(a.entries, s)
	}
	r.heading(ansiBold, "Sources:")
	if len(aggs) == 0 {
		r.line("  (no source status recorded)")
		return
	}
	wID, wKind, wRoles := 0, 0, 0
	for _, a := range aggs {
		wID = max(wID, min(utf8.RuneCountInString(a.id), 28))
		wKind = max(wKind, min(utf8.RuneCountInString(a.kind), 18))
		wRoles = max(wRoles, min(utf8.RuneCountInString(rolesLabel(a.roles)), 28))
	}
	for _, a := range aggs {
		sym, code := r.sourceSymbol(a.entries)
		r.line("  %s %s  %s  %s  %s", r.paint(code, sym), pad(a.id, wID), pad(a.kind, wKind), pad(rolesLabel(a.roles), wRoles), r.sourceState(a.entries))
	}
}

func rolesLabel(roles []string) string {
	if len(roles) == 0 {
		return "—"
	}
	return strings.Join(roles, ",")
}

// sourceSymbol summarises states; "skipped" (not applicable to a release,
// e.g. an upgrade guide for a patch release) is neutral.
func (r *renderer) sourceSymbol(es []domain.SourceStatus) (string, string) {
	allOK, allSkipped, anyOK, anyBad := true, true, false, false
	for _, s := range es {
		if s.State == domain.SourceSkipped {
			continue
		}
		allSkipped = false
		if s.State != domain.SourceOK {
			allOK = false
		}
		switch s.State {
		case domain.SourceOK, domain.SourcePartial:
			anyOK = true
		case domain.SourceUnavailable, domain.SourceError:
			anyBad = true
		}
	}
	switch {
	case allSkipped:
		return "·", ansiDim
	case allOK:
		return "✓", ansiGreen
	case !anyOK && anyBad:
		return "✗", ansiRed
	}
	return "~", ansiYellow
}

func (r *renderer) sourceState(es []domain.SourceStatus) string {
	type group struct {
		state    domain.SourceState
		versions []string
		detail   string
	}
	var groups []*group
	by := map[domain.SourceState]*group{}
	skippedOnly := true
	for _, s := range es {
		if s.State != domain.SourceSkipped {
			skippedOnly = false
		}
	}
	for _, s := range es {
		if s.State == domain.SourceSkipped && !skippedOnly && !r.opts.Verbose {
			continue
		}
		g := by[s.State]
		if g == nil {
			g = &group{state: s.State}
			by[s.State] = g
			groups = append(groups, g)
		}
		v := "product"
		if s.Version != "" {
			v = r.tag(s.Version)
		}
		dup := false
		for _, x := range g.versions {
			if x == v {
				dup = true
			}
		}
		if !dup {
			g.versions = append(g.versions, v)
		}
		if g.detail == "" && s.State != domain.SourceOK {
			g.detail = s.Detail
		}
	}
	if len(groups) == 1 {
		g := groups[0]
		out := string(g.state)
		if r.opts.Verbose && !(len(g.versions) == 1 && g.versions[0] == "product") {
			out += " (" + strings.Join(g.versions, ", ") + ")"
		}
		if g.detail != "" {
			out += ": " + g.detail
		}
		return out
	}
	parts := make([]string, len(groups))
	for i, g := range groups {
		p := string(g.state) + ": " + strings.Join(g.versions, ", ")
		if g.detail != "" {
			p += " (" + g.detail + ")"
		}
		parts[i] = p
	}
	return strings.Join(parts, "; ")
}

// --- changes ---

const (
	secBreaking = iota
	secAction
	secDeprecation
	secAPI
	secValues
	secConfig
	secArtifacts
	secDependencies
	secCompat
	secCompatNotes
	secSecurity
	secOther
	secHidden
	numSections
)

func isCompatComparison(c domain.Change) bool {
	return c.Provenance.Producer == Producer && strings.HasPrefix(c.Provenance.Rule, "compat:")
}

func sectionOf(c domain.Change, verbose bool) int {
	switch {
	case c.Breaking:
		return secBreaking
	case c.ActionRequired || c.Category == domain.CategoryMigration:
		return secAction
	}
	switch c.Category {
	case domain.CategoryDeprecation, domain.CategoryRemoval:
		return secDeprecation
	case domain.CategoryAPI, domain.CategoryCRDSchema:
		return secAPI
	case domain.CategoryHelmValues:
		return secValues
	case domain.CategoryConfiguration:
		return secConfig
	case domain.CategoryArtifact, domain.CategoryDependency:
		return secDependencies
	case domain.CategoryCompatibility:
		if isCompatComparison(c) {
			return secCompat
		}
		return secCompatNotes
	case domain.CategorySecurity:
		return secSecurity
	case domain.CategoryFeature, domain.CategoryBugfix:
		if verbose {
			return secOther
		}
		return secHidden
	}
	return secOther
}

func (r *renderer) changeSections() {
	var secs [numSections][]domain.Change
	for _, c := range r.e.Changes {
		s := sectionOf(c, r.opts.Verbose)
		secs[s] = append(secs[s], c)
	}
	r.changeList(ansiRed+";"+ansiBold, "Breaking changes", secs[secBreaking], true, true)
	r.changeList(ansiYellow+";"+ansiBold, "Migration requirements / action required", secs[secAction], true, true)
	r.changeList(ansiBold, "Deprecations & removals", secs[secDeprecation], false, false)
	r.changeList(ansiBold, "API & CRD schema changes", secs[secAPI], false, false)
	r.changeList(ansiBold, "Helm value changes", secs[secValues], false, false)
	r.changeList(ansiBold, "Configuration changes", secs[secConfig], false, false)
	r.artifacts()
	r.changeList(ansiBold, "Image & dependency changes", secs[secDependencies], false, false)
	r.compatibility()
	r.changeList(ansiBold, "Compatibility notes", secs[secCompatNotes], false, false)
	r.changeList(ansiBold, "Security changes", secs[secSecurity], false, false)
	r.other(secs[secOther], secs[secHidden])
}

func (r *renderer) changeList(code, title string, cs []domain.Change, always, hints bool) {
	if len(cs) == 0 && !always {
		return
	}
	r.heading(code, fmt.Sprintf("%s (%d):", title, len(cs)))
	if len(cs) == 0 {
		r.line("  none found")
		return
	}
	n := r.limit(len(cs))
	for _, c := range cs[:n] {
		r.changeLine(c, hints)
	}
	r.more(n, len(cs))
}

func (r *renderer) changeLine(c domain.Change, hint bool) {
	rel := "diff"
	if c.Release != "" {
		rel = r.tag(c.Release)
	}
	meta := fmt.Sprintf("(%s · %s · %s)", c.Category, provLabel(c.Provenance), r.cite(c.Evidence))
	r.line("  • [%s] %s  %s", rel, c.Title, r.paint(ansiDim, meta))
	if r.opts.Verbose {
		if d := strings.TrimSpace(c.Detail); d != "" {
			for _, l := range strings.Split(d, "\n") {
				r.line("      %s", strings.TrimRight(l, " "))
			}
		}
		if len(c.References) > 0 {
			refs := make([]string, len(c.References))
			for i, ref := range c.References {
				refs[i] = ref.ID
			}
			r.line("      refs: %s", strings.Join(refs, ", "))
		}
		return
	}
	if hint && c.Provenance.Producer == Producer && c.Detail != "" {
		r.line("      ↳ %s", shorten(firstLine(c.Detail), MaxTitle))
	}
}

func (r *renderer) other(shown, hidden []domain.Change) {
	if len(shown) == 0 && len(hidden) == 0 {
		return
	}
	r.heading(ansiBold, fmt.Sprintf("Other notable changes (%d):", len(shown)))
	n := r.limit(len(shown))
	for _, c := range shown[:n] {
		r.changeLine(c, false)
	}
	r.more(n, len(shown))
	if len(hidden) > 0 {
		features, fixes := 0, 0
		for _, c := range hidden {
			if c.Category == domain.CategoryFeature {
				features++
			} else {
				fixes++
			}
		}
		r.line("  (%s and %s not shown; use verbose output)", plural(features, "feature", "features"), plural(fixes, "bug fix", "bug fixes"))
	}
}

// --- artifacts ---

func statusLabel(s domain.ArtifactStatus) string {
	if s == "" {
		return "unknown"
	}
	return string(s)
}

func (r *renderer) statusPaint(s string) string {
	switch domain.ArtifactStatus(s) {
	case domain.ArtifactVerified:
		return r.paint(ansiGreen, s)
	case domain.ArtifactReferenced:
		return r.paint(ansiCyan, s)
	case domain.ArtifactExpected:
		return r.paint(ansiYellow, s)
	case domain.ArtifactMissing:
		return r.paint(ansiRed, s)
	}
	return s
}

func (r *renderer) artifactStatus(ac domain.ArtifactChange) string {
	fs, ts := "absent", "absent"
	if ac.From != nil {
		fs = statusLabel(ac.From.Status)
	}
	if ac.To != nil {
		ts = statusLabel(ac.To.Status)
	}
	switch ac.Change {
	case domain.ChangeAdded:
		if ac.From == nil {
			return r.statusPaint(ts)
		}
	case domain.ChangeUnchanged, domain.ChangeUpdated:
		if fs == ts {
			return r.statusPaint(ts)
		}
	}
	return r.statusPaint(fs) + " → " + r.statusPaint(ts)
}

func artifactDelta(ac domain.ArtifactChange) string {
	switch ac.Change {
	case domain.ChangeAdded:
		return "+ " + ac.To.Coordinate + "  (added)"
	case domain.ChangeRemoved:
		return "− " + ac.From.Coordinate + "  (removed)"
	case domain.ChangeUnchanged:
		return ac.To.Coordinate + "  (unchanged)"
	}
	fb, fv := splitCoordinate(ac.From.Coordinate)
	tb, tv := splitCoordinate(ac.To.Coordinate)
	if fb == tb && fv != "" && tv != "" {
		return fmt.Sprintf("%s  %s → %s", tb, fv, tv)
	}
	if ac.From.Coordinate == ac.To.Coordinate {
		return fmt.Sprintf("%s  %s → %s", ac.To.Coordinate, ac.From.Version, ac.To.Version)
	}
	return compactDelta(ac.From.Coordinate, ac.To.Coordinate)
}

func isDelim(c byte) bool { return strings.IndexByte("/:@=?& ", c) >= 0 }

// compactDelta renders two similar strings as prefix{a → b}suffix, splitting
// at delimiters, e.g. ".../download/{v1.17.0 → v1.18.0}/cert-manager.yaml".
func compactDelta(a, b string) string {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	for p > 0 && !isDelim(a[p-1]) {
		p--
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	for s > 0 && !isDelim(a[len(a)-s]) {
		s--
	}
	if p+s < 8 {
		return a + " → " + b
	}
	return a[:p] + "{" + a[p:len(a)-s] + " → " + b[p:len(b)-s] + "}" + a[len(a)-s:]
}

func (r *renderer) artifacts() {
	if len(r.e.Artifacts) == 0 {
		return
	}
	counts := map[domain.ChangeType]int{}
	var list, unchanged []domain.ArtifactChange
	for _, ac := range r.e.Artifacts {
		counts[ac.Change]++
		if ac.Change == domain.ChangeUnchanged && !r.opts.Verbose {
			unchanged = append(unchanged, ac)
			continue
		}
		list = append(list, ac)
	}
	var parts []string
	for _, ct := range []domain.ChangeType{domain.ChangeUpdated, domain.ChangeAdded, domain.ChangeRemoved, domain.ChangeUnchanged} {
		if counts[ct] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[ct], ct))
		}
	}
	r.heading(ansiBold, fmt.Sprintf("Artifact changes (%s):", strings.Join(parts, ", ")))
	w := 0
	for _, ac := range list {
		w = max(w, min(utf8.RuneCountInString(ac.ArtifactID), 24))
	}
	n := r.limit(len(list))
	for _, ac := range list[:n] {
		r.line("  %s  %s  [%s]  %s", pad(ac.ArtifactID, w), artifactDelta(ac), r.artifactStatus(ac), r.paint(ansiDim, "(computed · "+r.cite(ac.Evidence)+")"))
	}
	r.more(n, len(list))
	if len(unchanged) > 0 {
		ids := make([]string, len(unchanged))
		for i, ac := range unchanged {
			ids[i] = ac.ArtifactID
		}
		r.line("  (%d unchanged: %s)", len(unchanged), strings.Join(ids, ", "))
	}
}

// --- compatibility ---

func compatKind(cc domain.CompatibilityChange) string {
	if cc.To != nil {
		return normKind(cc.To.Kind)
	}
	if cc.From != nil {
		return normKind(cc.From.Kind)
	}
	return "supported"
}

func (r *renderer) compatibility() {
	if len(r.e.Compatibility) == 0 {
		return
	}
	var platforms []string
	byPlatform := map[string][]domain.CompatibilityChange{}
	for _, cc := range r.e.Compatibility {
		if _, ok := byPlatform[cc.Platform]; !ok {
			platforms = append(platforms, cc.Platform)
		}
		byPlatform[cc.Platform] = append(byPlatform[cc.Platform], cc)
	}
	for _, p := range platforms {
		r.heading(ansiBold, PlatformName(p)+" compatibility:")
		for _, cc := range byPlatform[p] {
			kind := compatKind(cc)
			summary := strings.TrimPrefix(cc.Summary, fmt.Sprintf("%s (%s): ", PlatformName(p), kind))
			var ev []domain.EvidenceID
			if cc.From != nil {
				ev = append(ev, cc.From.Evidence...)
			}
			if cc.To != nil {
				ev = appendUniqueIDs(ev, cc.To.Evidence...)
			}
			flag := ""
			if cc.Narrowed {
				flag = "  " + r.paint(ansiYellow, "[narrowed]")
			}
			r.line("  %s: %s%s  %s", kind, summary, flag, r.paint(ansiDim, "(computed · "+r.cite(ev)+")"))
			if r.opts.Verbose {
				if cc.From != nil {
					r.line("      %s: %s", r.tag(r.e.From.Semver), strings.TrimSpace(cc.From.Raw))
				}
				if cc.To != nil {
					r.line("      %s: %s", r.tag(r.e.To.Semver), strings.TrimSpace(cc.To.Raw))
				}
			}
		}
	}
}

// --- enrichments, warnings, evidence ---

func (r *renderer) enrichments() {
	if len(r.e.Enrichments) == 0 {
		return
	}
	r.heading(ansiMagenta+";"+ansiBold, fmt.Sprintf("AI enrichments (%d) — AI-derived, not deterministic; verify before acting:", len(r.e.Enrichments)))
	n := r.limit(len(r.e.Enrichments))
	for _, en := range r.e.Enrichments[:n] {
		head := en.Title
		content := strings.TrimSpace(en.Content)
		if head == "" {
			head = shorten(firstLine(content), MaxTitle)
		}
		meta := fmt.Sprintf("(AI-derived · %s · %s)", en.Provenance.Model, r.cite(en.Provenance.InputEvidence))
		r.line("  • [%s] %s  %s", en.Kind, head, r.paint(ansiDim, meta))
		if r.opts.Verbose && content != "" {
			for _, l := range strings.Split(content, "\n") {
				r.line("      %s", strings.TrimRight(l, " "))
			}
		}
	}
	r.more(n, len(r.e.Enrichments))
}

func (r *renderer) warnings() {
	if len(r.e.Warnings) == 0 {
		return
	}
	r.heading(ansiYellow+";"+ansiBold, fmt.Sprintf("Warnings (%d):", len(r.e.Warnings)))
	for _, w := range r.e.Warnings {
		r.line("  %s %s", r.paint(ansiYellow, "!"), w)
	}
}

func (r *renderer) evidence() {
	total := len(r.e.Evidence)
	r.heading(ansiBold, fmt.Sprintf("Evidence (%d records, %d cited above):", total, len(r.cited)))
	ids := append([]domain.EvidenceID{}, r.cited...)
	if r.opts.Verbose {
		for _, x := range r.e.Evidence {
			if !r.citedSet[x.ID] {
				ids = append(ids, x.ID)
			}
		}
	}
	w := 0
	for _, id := range ids {
		w = max(w, utf8.RuneCountInString(string(id)))
	}
	n := r.limit(len(ids))
	for _, id := range ids[:n] {
		x, ok := r.evByID[id]
		if !ok {
			r.line("  %s  (not in edge)", pad(string(id), w))
			continue
		}
		loc := ""
		if x.Locator != "" {
			loc = "  " + x.Locator
		}
		r.line("  %s  %s%s", r.paint(ansiDim, pad(string(id), w)), x.URI, loc)
		if r.opts.Verbose && x.Excerpt != "" {
			for _, l := range strings.Split(x.Excerpt, "\n") {
				r.line("      │ %s", strings.TrimRight(l, " "))
			}
		}
	}
	r.more(n, len(ids))
	if !r.opts.Verbose && total > len(r.cited) {
		r.line("  (%d uncited records not shown; use verbose or JSON output)", total-len(r.cited))
	}
}
