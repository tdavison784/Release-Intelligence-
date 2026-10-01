package discovery

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// TagAnalysis is what the tag list says about a product's release scheme.
type TagAnalysis struct {
	Repository string `json:"repository"`
	Total      int    `json:"total"`
	Prefix     string `json:"prefix"`
	// TagPattern is a strict pattern (with a (?P<version>…) group) proposed
	// when the default pattern for Prefix would also accept junk tags, or a
	// component-group pattern (see Scheme) proposed when no dot-separated
	// semver family exists.
	TagPattern       string         `json:"tagPattern,omitempty"`
	PrereleaseStyles map[string]int `json:"prereleaseStyles,omitempty"` // e.g. "alpha.N": 65
	StableCount      int            `json:"stableCount"`
	PrereleaseCount  int            `json:"prereleaseCount"`
	JunkCount        int            `json:"junkCount"`
	Junk             []string       `json:"junk,omitempty"` // sample
	Latest           string         `json:"latest,omitempty"`
	Lines            []string       `json:"lines,omitempty"` // newest first (at most 8)
	Lineage          string         `json:"lineage"`
	// Scheme is "semver" (default) or "component-groups" when the pattern
	// assembles the semantic version from ?P<major>/<minor>/<patch> groups
	// (e.g. PostgreSQL REL_17_2, Ruby v3_3_0).
	Scheme string `json:"scheme,omitempty"`
	// PatternCoverage is how many of the listed tags the proposed pattern
	// matches ("612/693"); evidence that the scheme was tested against the
	// real tag list before being proposed.
	PatternCoverage string `json:"patternCoverage,omitempty"`
	// Families lists the dot-separated semver tag families with several
	// stable tags (prefix "v", "helm-chart-", "controller-v", …), most
	// numerous first. A repository with several families publishes several
	// version trains; only one is the product.
	Families []TagFamily `json:"families,omitempty"`
	// TrainSwitch records that the most numerous family was not chosen
	// because scan evidence pointed at another family (filled by the
	// pipeline, not by AnalyzeTags).
	TrainSwitch *TrainSwitch `json:"trainSwitch,omitempty"`
	ListedAt    time.Time    `json:"listedAt,omitempty"`

	stable []domain.Version // ascending
	all    []domain.Version // ascending, stable + conventional prereleases
	byTag  map[string]RemoteTag
	raw    []RemoteTag
	repo   RepoRef
}

// TagFamily is one prefix train of dot-separated semver tags.
type TagFamily struct {
	Prefix string `json:"prefix"`
	Count  int    `json:"stable"`
	Latest string `json:"latest"`
	// LatestVersion is the semver of Latest (no prefix).
	LatestVersion string `json:"latestVersion"`
	// TagPattern is the strict pattern for this family (when one was needed).
	TagPattern string `json:"tagPattern,omitempty"`
}

// TrainSwitch records evidence-based selection of the product tag train.
type TrainSwitch struct {
	From      TagFamily `json:"from"`
	To        TagFamily `json:"to"`
	Rationale string    `json:"rationale"`
}

// Stable returns the stable releases in ascending order.
func (t *TagAnalysis) Stable() []domain.Version { return append([]domain.Version(nil), t.stable...) }

// All returns stable releases and conventional prereleases, ascending.
func (t *TagAnalysis) All() []domain.Version { return append([]domain.Version(nil), t.all...) }

// LatestVersion returns the latest stable version.
func (t *TagAnalysis) LatestVersion() (domain.Version, bool) {
	if len(t.stable) == 0 {
		return domain.Version{}, false
	}
	return t.stable[len(t.stable)-1], true
}

// Find returns the parsed version for a tag.
func (t *TagAnalysis) Find(tag string) (domain.Version, bool) {
	for _, v := range t.all {
		if v.Tag == tag {
			return v, true
		}
	}
	return domain.Version{}, false
}

// HasLine reports whether a stable release of major.minor exists.
func (t *TagAnalysis) HasLine(major, minor uint64) bool {
	for _, v := range t.stable {
		if v.Major() == major && v.Minor() == minor {
			return true
		}
	}
	return false
}

// PrevLine returns the previous existing stable line of major.minor.
func (t *TagAnalysis) PrevLine(major, minor uint64) (uint64, uint64, bool) {
	var best *domain.Version
	for i := range t.stable {
		v := t.stable[i]
		if v.Major() > major || (v.Major() == major && v.Minor() >= minor) {
			continue
		}
		if best == nil || best.Less(v) {
			best = &t.stable[i]
		}
	}
	if best == nil {
		return 0, 0, false
	}
	return best.Major(), best.Minor(), true
}

// Evidence returns git-ref evidence for a tag.
func (t *TagAnalysis) Evidence(tag string, retrieved time.Time) domain.Evidence {
	rt := t.byTag[tag]
	excerpt := "refs/tags/" + tag
	if rt.Commit != "" {
		excerpt = rt.Commit + " " + excerpt
	}
	return domain.NewEvidence(domain.EvidenceGitRef, "", t.repo.TagURL(tag), "refs/tags/"+tag, excerpt, "", retrieved)
}

var (
	semverCoreRe = regexp.MustCompile(`^([A-Za-z_-]*?)(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?$`)
	digitsRe     = regexp.MustCompile(`\d+`)
	// conventionalPre are the semver prerelease words every ecosystem uses.
	conventionalPre = regexp.MustCompile(`^(alpha|beta|rc|pre|preview)\.?N$`)
)

// prereleaseStyle abstracts digits away: "rc.4" → "rc.N", "beta1" → "betaN".
func prereleaseStyle(pre string) string { return digitsRe.ReplaceAllString(pre, "N") }

// AnalyzeTags infers the tag scheme from a remote tag list.
func AnalyzeTags(repo RepoRef, tags []RemoteTag) *TagAnalysis {
	ta := &TagAnalysis{Repository: repo.String(), Total: len(tags), PrereleaseStyles: map[string]int{}, byTag: map[string]RemoteTag{}, raw: tags, repo: repo}
	prefixes := map[string]int{}
	for _, t := range tags {
		ta.byTag[t.Name] = t
		if strings.Contains(t.Name, "/") {
			continue
		}
		if m := semverCoreRe.FindStringSubmatch(t.Name); m != nil {
			prefixes[m[1]]++
		}
	}
	families := tagFamilies(tags, prefixes)
	ta.Families = families
	best, bestN := "", -1
	for p, n := range prefixes {
		if n > bestN || (n == bestN && p < best) {
			best, bestN = p, n
		}
	}
	ta.Prefix = best
	ta.classifySemver(best, tags)
	// Fallback: no dot-separated semver family exists (or it is noise). Mine
	// the tag list for a component-group scheme (REL_17_2, v3_3_0, …) and
	// accept it only after testing it against the real tag list: it must
	// match a substantial share of tags (several times whatever the semver
	// family covered) and produce sane, deduplicable versions. The
	// postgresql failure mode (a scheme matching 0 of 693 tags) is what this
	// guards against.
	if ta.StableCount < minSemverFamilyStable {
		pat, matched := inferComponentPattern(tags)
		if pat != "" && matched >= minComponentShare(len(tags)) && matched > 3*ta.StableCount {
			probe := &TagAnalysis{Repository: repo.String(), Total: len(tags), byTag: ta.byTag, repo: repo}
			probe.classifyPattern(pat, tags)
			if probe.StableCount >= minComponentStable && !hasDuplicateSemvers(probe.stable) {
				families, sw, listedAt := ta.Families, ta.TrainSwitch, ta.ListedAt
				probe.Scheme, probe.PatternCoverage = SchemeComponentGroups, itoa(matched)+"/"+itoa(len(tags))
				*ta = *probe
				ta.Families, ta.TrainSwitch, ta.ListedAt = families, sw, listedAt
				ta.Prefix = ""
				ta.finish()
				return ta
			}
		}
	}
	ta.finish()
	return ta
}

// classifySemver runs the dot-separated classification for one prefix.
func (ta *TagAnalysis) classifySemver(prefix string, tags []RemoteTag) {
	parser := domain.VersionParser{Scheme: domain.SchemeSemver, Pattern: domain.DefaultTagPattern(prefix)}
	conventional, junkParsed := ta.classifyWith(parser, tags)
	// A strict pattern is proposed when junk exists: demoted prereleases, or
	// the tags of another version train in the same repository.
	if junkParsed || (ta.JunkCount > 0 && len(ta.Families) > 1) {
		ta.TagPattern = strictTagPattern(prefix, conventional)
	}
	ta.Scheme, ta.PatternCoverage = "", ""
}

// classifyPattern runs the classification with an explicit pattern.
func (ta *TagAnalysis) classifyPattern(pattern string, tags []RemoteTag) {
	parser := domain.VersionParser{Scheme: domain.SchemeSemver, Pattern: regexp.MustCompile(pattern)}
	ta.classifyWith(parser, tags)
	ta.TagPattern = pattern
}

// classifyWith fills stable/all/counts using parser. It returns the
// conventional prerelease styles observed and whether a tag that parsed as a
// version was still demoted to junk (unconventional prerelease), which is
// when a strict pattern is worth proposing.
func (ta *TagAnalysis) classifyWith(parser domain.VersionParser, tags []RemoteTag) ([]string, bool) {
	ta.stable, ta.all = nil, nil
	ta.StableCount, ta.PrereleaseCount, ta.JunkCount, ta.Junk = 0, 0, 0, nil
	ta.PrereleaseStyles = map[string]int{}
	var conventional []string
	junkParsed := false
	for _, t := range tags {
		v, err := parser.Parse(t.Name)
		if err != nil {
			ta.addJunk(t.Name)
			continue
		}
		if !v.IsPrerelease() {
			ta.stable = append(ta.stable, v)
			ta.StableCount++
			continue
		}
		style := prereleaseStyle(v.Prerelease())
		if conventionalPre.MatchString(style) {
			ta.PrereleaseStyles[style]++
			ta.PrereleaseCount++
			ta.all = append(ta.all, v)
			if !containsStr(conventional, style) {
				conventional = append(conventional, style)
			}
			continue
		}
		ta.addJunk(t.Name)
		junkParsed = true
	}
	// a conventional style seen once among many prereleases is a legacy
	// outlier (e.g. a lone "v0.4.0-alpha1" among 130 "-rcN" tags)
	if ta.PrereleaseCount >= 20 {
		var keep []string
		var all []domain.Version
		for _, s := range conventional {
			if ta.PrereleaseStyles[s] == 1 {
				ta.PrereleaseCount--
				delete(ta.PrereleaseStyles, s)
				for _, v := range ta.all {
					if prereleaseStyle(v.Prerelease()) == s {
						ta.addJunk(v.Tag)
					}
				}
				junkParsed = true
				continue
			}
			keep = append(keep, s)
		}
		for _, v := range ta.all {
			if _, ok := ta.PrereleaseStyles[prereleaseStyle(v.Prerelease())]; ok {
				all = append(all, v)
			}
		}
		conventional, ta.all = keep, all
	}
	ta.all = append(ta.all, ta.stable...)
	domain.SortVersions(ta.stable)
	domain.SortVersions(ta.all)
	return conventional, junkParsed
}

// finish computes the derived fields (latest, lines, lineage).
func (ta *TagAnalysis) finish() {
	if len(ta.stable) > 0 {
		ta.Latest = ta.stable[len(ta.stable)-1].Tag
	}
	seen := map[string]bool{}
	ta.Lines = nil
	for i := len(ta.stable) - 1; i >= 0 && len(ta.Lines) < 8; i-- {
		l := ta.stable[i].Line()
		if !seen[l] {
			seen[l] = true
			ta.Lines = append(ta.Lines, l)
		}
	}
	ta.Lineage = catalog.LineageLinear
	patched := 0
	for i, l := range ta.Lines {
		if i >= 4 {
			break
		}
		for _, v := range ta.stable {
			if v.Line() == l && v.Patch() > 0 {
				patched++
				break
			}
		}
	}
	if patched >= 2 {
		ta.Lineage = catalog.LineageMinor
	}
}

// SchemeComponentGroups marks a tag scheme assembled from component groups.
const SchemeComponentGroups = "component-groups"

const (
	// minSemverFamilyStable is the smallest dot-separated family worth
	// keeping; below it the component-group fallback is tried.
	minSemverFamilyStable = 3
	// minComponentStable is the smallest component-group yield accepted.
	minComponentStable = 5
)

// minComponentShare is the fraction of tags a component pattern must match.
func minComponentShare(total int) int {
	share := total / 4
	if share < 4 {
		share = 4
	}
	return share
}

// hasDuplicateSemvers reports whether distinct tags map to the same version
// more than marginally (a pattern that collapses tags is wrong).
func hasDuplicateSemvers(vs []domain.Version) bool {
	seen := map[string]int{}
	dups := 0
	for _, v := range vs {
		seen[v.Semver]++
		if seen[v.Semver] == 2 {
			dups++
		}
	}
	return dups > len(vs)/20
}

// tagFamilies ranks dot-separated semver tag families with at least three
// stable tags, most numerous first.
func tagFamilies(tags []RemoteTag, prefixes map[string]int) []TagFamily {
	var out []TagFamily
	for prefix, n := range prefixes {
		if n < 3 {
			continue
		}
		parser := domain.VersionParser{Scheme: domain.SchemeSemver, Pattern: domain.DefaultTagPattern(prefix)}
		f := TagFamily{Prefix: prefix, Count: n}
		for _, t := range tags {
			if strings.Contains(t.Name, "/") {
				continue
			}
			// latest by version, not by listing order (ls-remote sorts
			// lexicographically: v1.9.0 > v1.15.0)
			if v, err := parser.Parse(t.Name); err == nil && !v.IsPrerelease() {
				if f.Latest == "" || domain.MustVersion(f.Latest, f.LatestVersion).Less(v) {
					f.Latest, f.LatestVersion = v.Tag, v.Semver
				}
			}
		}
		if f.Latest != "" {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Prefix < out[j].Prefix
	})
	return out
}

// inferComponentPattern mines the tag list for a component-group pattern
// (PostgreSQL REL_17_2 / REL9_6_24, Ruby v3_3_0, Go go1.22, …). The returned
// pattern uses ?P<major>/<minor>/<patch> groups; matched is how many tags it
// matches. It returns "" when no dominant shape exists.
//
// When two sibling shapes coexist (same separator, prefixes differing only
// by a trailing separator, group counts differing by one), the digit roles
// are assigned from the tag list itself: when every 2-group major is newer
// than every 3-group major, the numbering dropped its middle component
// (PostgreSQL 9.6.24 → 10.23 is patch 23 of line 10.0) and the minor group
// is the optional one; otherwise the patch group is.
func inferComponentPattern(tags []RemoteTag) (string, int) {
	type shape struct {
		prefix        string
		sep           string
		groups        int
		tags          int
		minMajor      uint64
		maxMajor      uint64
		hasMajorZero  bool // a "<prefix>0"-only tag (REL_17_0 style anchors)
		sampleAnchors bool
	}
	shapes := map[string]*shape{}
	var order []string
	note := func(name string) (prefix, sep string, groups int, major uint64, ok bool) {
		m := componentTagRe.FindStringSubmatch(name)
		if m == nil {
			return "", "", 0, 0, false
		}
		g := 2
		if m[5] != "" {
			g = 3
		}
		major, _ = strconv.ParseUint(m[2], 10, 64)
		return m[1], m[3], g, major, true
	}
	for _, t := range tags {
		name := t.Name
		if strings.Contains(name, "/") {
			continue
		}
		prefix, sep, groups, major, ok := note(name)
		if !ok {
			continue
		}
		key := prefix + "\x00" + sep + "\x00" + itoa(groups)
		s, seen := shapes[key]
		if !seen {
			s = &shape{prefix: prefix, sep: sep, groups: groups, minMajor: ^uint64(0)}
			shapes[key] = s
			order = append(order, key)
		}
		s.tags++
		if major < s.minMajor {
			s.minMajor = major
		}
		if major > s.maxMajor {
			s.maxMajor = major
		}
	}
	if len(order) == 0 {
		return "", 0
	}
	sort.Slice(order, func(i, j int) bool { return shapes[order[i]].tags > shapes[order[j]].tags })
	top := shapes[order[0]]
	if top.tags < minComponentStable {
		return "", 0
	}
	prefix, groups, dropMiddle := top.prefix, top.groups, false
	siblingPrefix := top.prefix
	for _, k := range order[1:] {
		s := shapes[k]
		if s.sep != top.sep || s.groups+1 != top.groups && top.groups+1 != s.groups || !sepSiblingPrefix(top.prefix, s.prefix) {
			continue
		}
		if s.tags < maxInt(6, top.tags/6) {
			continue
		}
		prefix = commonPrefix(top.prefix, s.prefix)
		if len(s.prefix) > len(siblingPrefix) {
			siblingPrefix = s.prefix
		}
		two, three := top, s
		if three.groups == 2 {
			two, three = s, top
		}
		groups = 3
		if two.maxMajor < three.minMajor { // every 2-group major is older
			dropMiddle = false // 1_2 → 1_2_3: second number stays the minor
		} else if two.minMajor > three.maxMajor {
			dropMiddle = true // 9_6_24 → 10_23: the middle component was dropped
		}
		break
	}
	sepQ := regexp.QuoteMeta(top.sep)
	var b strings.Builder
	b.WriteString(`^`)
	if prefix != "" {
		b.WriteString(regexp.QuoteMeta(prefix))
	}
	// A merged sibling (REL vs REL_) leaves the separator inside the longer
	// prefix optional so both spellings match.
	if len(siblingPrefix) > len(prefix) && isAllSep(siblingPrefix[len(prefix):]) {
		b.WriteString(sepQ + `?`)
	}
	b.WriteString(`(?P<major>\d+)`)
	if groups == 3 && dropMiddle {
		// 2-group tags carry major.patch (PostgreSQL >= 10); the minor group
		// is optional and defaults to 0.
		b.WriteString(`(?:` + sepQ + `(?P<minor>\d+))?` + sepQ + `(?P<patch>\d+)`)
	} else {
		b.WriteString(`(?:` + sepQ + `(?P<minor>\d+))?`)
		if groups >= 3 {
			b.WriteString(`(?:` + sepQ + `(?P<patch>\d+))?`)
		}
	}
	b.WriteString(`$`)
	pat := b.String()
	matched := 0
	re := regexp.MustCompile(pat)
	for _, t := range tags {
		if !strings.Contains(t.Name, "/") && re.MatchString(t.Name) {
			matched++
		}
	}
	return pat, matched
}

// componentTagRe parses a component-group tag: a literal prefix, then 2–3
// digit groups separated by one consistent separator.
var componentTagRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_-]*?)(\d+)([_\-.])(\d+)(?:[_\-.](\d+))?$`)

func sepSiblingPrefix(a, b string) bool {
	// REL_ vs REL, v vs v_: the longer extends the shorter with separator
	// characters only.
	if strings.HasPrefix(a, b) {
		return isAllSep(a[len(b):])
	}
	if strings.HasPrefix(b, a) {
		return isAllSep(b[len(a):])
	}
	return false
}

func isAllSep(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c != '_' && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

func commonPrefix(a, b string) string {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return a[:n]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (ta *TagAnalysis) addJunk(name string) {
	ta.JunkCount++
	if len(ta.Junk) < 15 {
		ta.Junk = append(ta.Junk, name)
	}
}

// strictTagPattern accepts the prefix, a semantic version and only the
// conventional prerelease styles observed.
func strictTagPattern(prefix string, styles []string) string {
	sort.Strings(styles)
	var alts []string
	for _, s := range styles {
		alts = append(alts, strings.ReplaceAll(regexp.QuoteMeta(s), "N", `\d+`))
	}
	pre := ""
	if len(alts) > 0 {
		pre = `(?:-(?:` + strings.Join(alts, "|") + `))?`
	}
	return `^` + regexp.QuoteMeta(prefix) + `(?P<version>\d+\.\d+\.\d+` + pre + `)$`
}

// TagRegex returns the effective tag regex (strict pattern or default).
func (ta *TagAnalysis) TagRegex() string {
	if ta.TagPattern != "" {
		return ta.TagPattern
	}
	return domain.DefaultTagPattern(ta.Prefix).String()
}

// SelectValidationReleases picks n recent stable releases: the newest patch
// of each of the newest lines, plus the X.Y.0 of the newest line so that
// minor-only sources are exercised too.
func (ta *TagAnalysis) SelectValidationReleases(n int) []domain.Version {
	if n < 1 {
		n = 1
	}
	var out []domain.Version
	add := func(v domain.Version) {
		for _, x := range out {
			if x.Tag == v.Tag {
				return
			}
		}
		out = append(out, v)
	}
	newestOfLine := func(line string) (domain.Version, bool) {
		for i := len(ta.stable) - 1; i >= 0; i-- {
			if ta.stable[i].Line() == line {
				return ta.stable[i], true
			}
		}
		return domain.Version{}, false
	}
	for _, l := range ta.Lines {
		if len(out) >= n-1 {
			break
		}
		if v, ok := newestOfLine(l); ok {
			add(v)
		}
	}
	// Each sampled line's X.Y.0 release too: sources that only exist for
	// minor releases (upgrade guides, minor announcements) need them.
	lines := map[string]bool{}
	for _, v := range out {
		lines[v.Line()] = true
	}
	for _, v := range ta.stable {
		if lines[v.Line()] && v.Patch() == 0 {
			add(v)
		}
	}
	// fall back to more patches when there are fewer lines than needed
	for i := len(ta.stable) - 1; i >= 0 && len(out) < n; i-- {
		add(ta.stable[i])
	}
	domain.SortVersions(out)
	return out
}

// SwitchFamily returns the analysis of another tag family of the same tag
// list (the same versions the train switch re-selects).
func (ta *TagAnalysis) SwitchFamily(f TagFamily, tags []RemoteTag) *TagAnalysis {
	nt := &TagAnalysis{Repository: ta.Repository, Total: ta.Total, byTag: ta.byTag, raw: ta.raw, repo: ta.repo,
		Prefix: f.Prefix, Families: ta.Families, TrainSwitch: ta.TrainSwitch, ListedAt: ta.ListedAt}
	nt.classifySemver(f.Prefix, tags)
	nt.finish()
	return nt
}

// Summary is a one-line description for the report.
func (ta *TagAnalysis) Summary() string {
	styles := make([]string, 0, len(ta.PrereleaseStyles))
	for s, n := range ta.PrereleaseStyles {
		styles = append(styles, fmt.Sprintf("%s×%d", s, n))
	}
	sort.Strings(styles)
	if ta.Scheme == SchemeComponentGroups {
		return fmt.Sprintf("%d tags: component tag scheme %s matching %s tags, %d stable, %d junk; latest stable %s; lineage %s",
			ta.Total, ta.TagPattern, ta.PatternCoverage, ta.StableCount, ta.JunkCount, ta.Latest, ta.Lineage)
	}
	return fmt.Sprintf("%d tags: prefix %q, %d stable, %d prereleases (%s), %d junk; latest stable %s; lineage %s",
		ta.Total, ta.Prefix, ta.StableCount, ta.PrereleaseCount, strings.Join(styles, ", "), ta.JunkCount, ta.Latest, ta.Lineage)
}
