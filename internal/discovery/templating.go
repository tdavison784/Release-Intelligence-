package discovery

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Template placeholders (catalog.RenderContext fields).
const (
	tmplTag      = "{{.Tag}}"
	tmplVersion  = "{{.Version}}"
	tmplLine     = "{{.Major}}.{{.Minor}}"
	tmplPrevLine = "{{.PrevMajor}}.{{.PrevMinor}}"
)

// vtoken is a version-like token inside a string.
type vtoken struct {
	start, end   int // byte span, including a leading "v" when present
	vprefix      bool
	major, minor uint64
	patch        int64 // -1 for a line token ("1.21")
}

func (t vtoken) line() string { return fmt.Sprintf("%d.%d", t.major, t.minor) }

var versionTokenRe = regexp.MustCompile(`v?(\d+)\.(\d+)(?:\.(\d+))?`)

func isAlnumDot(b byte) bool {
	return b == '.' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// findVersionTokens returns version-like tokens with clean boundaries: not
// preceded by a letter, digit or dot, and not followed by a digit or by a dot
// and a digit (so "10.0.0.1" and "python3.11" do not match).
func findVersionTokens(s string) []vtoken {
	var out []vtoken
	for _, m := range versionTokenRe.FindAllStringSubmatchIndex(s, -1) {
		start, end := m[0], m[1]
		if start > 0 && isAlnumDot(s[start-1]) {
			continue
		}
		if end < len(s) {
			c := s[end]
			if c >= '0' && c <= '9' {
				continue
			}
			if c == '.' && end+1 < len(s) && s[end+1] >= '0' && s[end+1] <= '9' {
				continue
			}
		}
		major, _ := strconv.ParseUint(s[m[2]:m[3]], 10, 64)
		minor, _ := strconv.ParseUint(s[m[4]:m[5]], 10, 64)
		patch := int64(-1)
		if m[6] >= 0 {
			patch, _ = strconv.ParseInt(s[m[6]:m[7]], 10, 64)
		}
		out = append(out, vtoken{start: start, end: end, vprefix: s[start] == 'v', major: major, minor: minor, patch: patch})
	}
	return out
}

// pairSep matches the separator of a version pair such as "1.20-1.21",
// "2.14_3.0" or "1.20-to-1.21".
var pairSep = regexp.MustCompile(`^(?:-|_|-to-|_to_|\.\.)$`)

// templatizeRelease replaces occurrences of release v (tag, version, line and
// "previous line - line" pairs) in s by template placeholders. prevLine is
// the previous existing line ("" when unknown). For bare tags (no prefix)
// preferTag selects {{.Tag}} over {{.Version}}.
func templatizeRelease(s string, v domain.Version, prefix, prevLine string, preferTag bool) (string, bool) {
	toks := findVersionTokens(s)
	if len(toks) == 0 {
		return s, false
	}
	var b strings.Builder
	last := 0
	changed := false
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		repl := ""
		end := t.end
		switch {
		case t.patch >= 0 && t.major == v.Major() && t.minor == v.Minor() && uint64(t.patch) == v.Patch():
			switch {
			case t.vprefix && prefix == "v":
				repl = tmplTag
			case prefix == "" && preferTag:
				repl = tmplTag
			default:
				repl = tmplVersion
				if t.vprefix {
					repl = "v" + tmplVersion
				}
			}
		case t.patch < 0 && t.line() == v.Line():
			repl = tmplLine
			if t.vprefix {
				repl = "v" + repl
			}
		case t.patch < 0 && prevLine != "" && t.line() == prevLine && i+1 < len(toks):
			n := toks[i+1]
			if n.patch < 0 && n.line() == v.Line() && pairSep.MatchString(s[t.end:n.start]) {
				repl = tmplPrevLine + s[t.end:n.start] + tmplLine
				if t.vprefix {
					repl = "v" + repl
				}
				end = n.end
				i++
			}
		}
		if repl == "" {
			continue
		}
		b.WriteString(s[last:t.start])
		b.WriteString(repl)
		last = end
		changed = true
	}
	b.WriteString(s[last:])
	return b.String(), changed
}

// pathTemplate is a path generalised over the release it documents.
type pathTemplate struct {
	Template string
	Major    uint64
	Minor    uint64
	Patch    int64 // -1 when the path is per line
	Pair     bool  // contains a previous-line/line pair
}

// Anchor returns "M.m" or "M.m.p".
func (p pathTemplate) Anchor() string {
	if p.Patch >= 0 {
		return fmt.Sprintf("%d.%d.%d", p.Major, p.Minor, p.Patch)
	}
	return fmt.Sprintf("%d.%d", p.Major, p.Minor)
}

// inferPathTemplate generalises a path that embeds versions, e.g.
// "docs/upgrading/2.14-3.0.md" → "docs/upgrading/{{.PrevMajor}}.{{.PrevMinor}}-{{.Major}}.{{.Minor}}.md"
// and "news/1.30.x/announcing-1.30.1/index.md" → "news/{{.Major}}.{{.Minor}}.x/announcing-{{.Version}}/index.md".
// The tokens must describe one release consistently.
func inferPathTemplate(p, tagPrefix string) (pathTemplate, bool) {
	toks := findVersionTokens(p)
	if len(toks) == 0 {
		return pathTemplate{}, false
	}
	type seg struct {
		t    vtoken
		pair *vtoken
	}
	var segs []seg
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.patch < 0 && i+1 < len(toks) {
			n := toks[i+1]
			if n.patch < 0 && pairSep.MatchString(p[t.end:n.start]) && lineLess(t, n) {
				nn := n
				segs = append(segs, seg{t: t, pair: &nn})
				i++
				continue
			}
		}
		segs = append(segs, seg{t: t})
	}
	pt := pathTemplate{Patch: -1}
	anchorSet := false
	for _, sg := range segs {
		a := sg.t
		if sg.pair != nil {
			a = *sg.pair
			pt.Pair = true
		}
		if !anchorSet {
			pt.Major, pt.Minor, anchorSet = a.major, a.minor, true
		} else if a.major != pt.Major || a.minor != pt.Minor {
			return pathTemplate{}, false
		}
		if a.patch >= 0 {
			if pt.Patch >= 0 && pt.Patch != a.patch {
				return pathTemplate{}, false
			}
			pt.Patch = a.patch
		}
	}
	var b strings.Builder
	last := 0
	for _, sg := range segs {
		b.WriteString(p[last:sg.t.start])
		switch {
		case sg.pair != nil:
			if sg.t.vprefix {
				b.WriteString("v")
			}
			b.WriteString(tmplPrevLine + p[sg.t.end:sg.pair.start])
			if sg.pair.vprefix {
				b.WriteString("v")
			}
			b.WriteString(tmplLine)
			last = sg.pair.end
			continue
		case sg.t.patch >= 0:
			if sg.t.vprefix && tagPrefix == "v" {
				b.WriteString(tmplTag)
			} else {
				if sg.t.vprefix {
					b.WriteString("v")
				}
				b.WriteString(tmplVersion)
			}
		default:
			if sg.t.vprefix {
				b.WriteString("v")
			}
			b.WriteString(tmplLine)
		}
		last = sg.t.end
	}
	b.WriteString(p[last:])
	pt.Template = b.String()
	return pt, true
}

func lineLess(a, b vtoken) bool {
	return a.major < b.major || (a.major == b.major && a.minor < b.minor)
}

// Placeholders used by build tooling that stand for "the release tag".
var (
	tagVarRe = regexp.MustCompile(`\$\{\{\s*github\.ref_name\s*\}\}|\$\{\{\s*github\.ref\s*\}\}|\$\{GITHUB_REF_NAME\}|\$GITHUB_REF_NAME|\$\{GITHUB_REF#refs/tags/\}|\{\{\s*\.Tag\s*\}\}`)
	// genericPlaceholderRe matches unresolved variables of shells, make,
	// GitHub Actions, goreleaser and docs templating engines.
	genericPlaceholderRe = regexp.MustCompile(`\$\{\{[^}]*\}\}|\$\{[A-Za-z_][A-Za-z0-9_]*(?:[:#%/][^}]*)?\}|\$\([A-Za-z_][A-Za-z0-9_]*\)|\$[A-Za-z_][A-Za-z0-9_]*|\{\{[^}]*\}\}|\[\[VAR::[^\]]*\]\]|<[A-Za-z_-]*(?:version|tag)[A-Za-z_-]*>`)
)

// isPlaceholder reports whether s is entirely one unresolved placeholder.
func isPlaceholder(s string) bool {
	m := genericPlaceholderRe.FindStringIndex(s)
	return m != nil && m[0] == 0 && m[1] == len(s)
}

// templatizeAssetURL turns a hosted-release download URL into a template:
// the path segment after /download/ is by definition the tag; the same
// placeholder (or the same concrete version) in the file name is replaced
// consistently. ok is false when unresolved placeholders remain.
func templatizeAssetURL(u, tagPrefix string) (string, bool) {
	const marker = "/releases/download/"
	i := strings.Index(u, marker)
	if i < 0 {
		return u, false
	}
	rest := u[i+len(marker):]
	slash := strings.Index(rest, "/")
	if slash <= 0 {
		return u, false
	}
	seg, file := rest[:slash], rest[slash+1:]
	if strings.Contains(file, "/") || file == "" {
		return u, false
	}
	switch {
	case isPlaceholder(seg):
		// the same placeholder in the file name means the tag as well; for
		// bare tags {{.Tag}} == {{.Version}}.
		repl := tmplTag
		if tagPrefix == "" {
			repl = tmplVersion
		}
		file = strings.ReplaceAll(file, seg, repl)
		if p := strings.TrimPrefix(seg, "v"); p != seg {
			file = strings.ReplaceAll(file, p, tmplVersion)
		}
	default:
		toks := findVersionTokens(seg)
		if len(toks) != 1 || toks[0].patch < 0 || toks[0].start != 0 || toks[0].end != len(seg) {
			return u, false
		}
		t := toks[0]
		v := domain.MustVersion(seg, fmt.Sprintf("%d.%d.%d", t.major, t.minor, t.patch))
		file, _ = templatizeRelease(file, v, tagPrefix, "", false)
	}
	file = tagVarRe.ReplaceAllString(file, tmplTag)
	out := u[:i+len(marker)] + tmplTag + "/" + file
	if genericPlaceholderRe.MatchString(strings.NewReplacer(tmplTag, "", tmplVersion, "", tmplLine, "").Replace(file)) {
		return out, false
	}
	return out, true
}
