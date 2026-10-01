package discovery

import (
	"fmt"
	"regexp"
	"sort"
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
	// when the default pattern for Prefix would also accept junk tags.
	TagPattern       string         `json:"tagPattern,omitempty"`
	PrereleaseStyles map[string]int `json:"prereleaseStyles,omitempty"` // e.g. "alpha.N": 65
	StableCount      int            `json:"stableCount"`
	PrereleaseCount  int            `json:"prereleaseCount"`
	JunkCount        int            `json:"junkCount"`
	Junk             []string       `json:"junk,omitempty"` // sample
	Latest           string         `json:"latest,omitempty"`
	Lines            []string       `json:"lines,omitempty"` // newest first (at most 8)
	Lineage          string         `json:"lineage"`
	ListedAt         time.Time      `json:"listedAt,omitempty"`

	stable []domain.Version // ascending
	all    []domain.Version // ascending, stable + conventional prereleases
	byTag  map[string]RemoteTag
	repo   RepoRef
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
	ta := &TagAnalysis{Repository: repo.String(), Total: len(tags), PrereleaseStyles: map[string]int{}, byTag: map[string]RemoteTag{}, repo: repo}
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
	best, bestN := "", -1
	for p, n := range prefixes {
		if n > bestN || (n == bestN && p < best) {
			best, bestN = p, n
		}
	}
	ta.Prefix = best
	parser := domain.VersionParser{Scheme: domain.SchemeSemver, Pattern: domain.DefaultTagPattern(best)}
	junkParsed := false
	var conventional []string
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
	if junkParsed {
		ta.TagPattern = strictTagPattern(best, conventional)
	}
	if len(ta.stable) > 0 {
		ta.Latest = ta.stable[len(ta.stable)-1].Tag
	}
	seen := map[string]bool{}
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
	return ta
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

// Summary is a one-line description for the report.
func (ta *TagAnalysis) Summary() string {
	styles := make([]string, 0, len(ta.PrereleaseStyles))
	for s, n := range ta.PrereleaseStyles {
		styles = append(styles, fmt.Sprintf("%s×%d", s, n))
	}
	sort.Strings(styles)
	return fmt.Sprintf("%d tags: prefix %q, %d stable, %d prereleases (%s), %d junk; latest stable %s; lineage %s",
		ta.Total, ta.Prefix, ta.StableCount, ta.PrereleaseCount, strings.Join(styles, ", "), ta.JunkCount, ta.Latest, ta.Lineage)
}
