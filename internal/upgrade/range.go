package upgrade

import (
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// versionRange is THE one semantic representation of a compatibility
// constraint: the set of platform minor lines it admits, plus explicit bound
// semantics for the sides the candidate list cannot show. Both consumers of
// a constraint — the endpoint diff (compareConstraints) and the environment
// check (EvaluatePlatformConstraint) — evaluate through versionRangeOf, so a
// fix to the meaning of one kind applies everywhere and the two never drift
// apart again.
//
// Semantics per constraint kind (domain.CompatibilityConstraint.Kind):
//
//   - "minimum": the stated version is a LOWER bound — "minimum: 1.30" means
//     ">= 1.30", not "== 1.30". Admits the bound line and everything above
//     (openAbove). Recognised when the value resolves to a single line (one
//     enumerated version, or a pure ">=X" constraint); any other value stated
//     in a minimum column (a full range, a list) is taken at face value.
//   - "maximum": the mirror — "maximum: 1.33" means "<= 1.33" (openBelow).
//   - "tested": the enumeration is taken literally: membership is exactly the
//     stated list/range. Being outside it means "untested", never
//     "unsupported" (informationalKind in compat.go).
//   - "supported" (the default for an empty kind) and "chart-kubeVersion":
//     the stated range itself. kubeVersion-style prerelease suffixes
//     (">=1.25.0-0") are handled by semver comparison at the patch bounds of
//     each line, so "1.25" is admitted by ">=1.25.0-0" (1.25.0 > 1.25.0-0).
type versionRange struct {
	// ok reports whether the constraint could be evaluated at all.
	ok bool
	// members are the admitted lines among the candidates given to
	// versionRangeOf.
	members map[lineKey]bool
	// openAbove: also admits every line above the largest candidate (no
	// upper bound). openBelow: also admits every line below the smallest
	// candidate (no lower bound — a pure maximum).
	openAbove, openBelow bool
}

// boundRe matches a constraint that states exactly one bound of a line:
// ">=1.25", ">= 1.25.0-0", "<=1.33". Direction and value are captured.
var boundRe = regexp.MustCompile(`^(>=|<=)\s*v?(\d+)(?:\.(\d+))?(?:\.\d+)?(?:-[0-9A-Za-z.\-]+)?$`)

// versionRangeOf evaluates a constraint over the given candidate lines. The
// candidates must contain every line the constraint itself mentions
// (candidateLines guarantees this); EvaluatePlatformConstraint additionally
// appends the checked line so admission is decided by the same code path as
// the diff.
func versionRangeOf(c *domain.CompatibilityConstraint, cands []lineKey) versionRange {
	if c == nil {
		return versionRange{}
	}
	if b, upper, ok := boundOf(c); ok {
		return boundRange(b, upper, cands)
	}
	// An explicit enumeration is taken literally (exact membership, no open
	// sides) — for "tested" lists this is what keeps "not tested" distinct
	// from "not supported".
	if len(c.Versions) > 0 {
		s := versionRange{ok: true, members: map[lineKey]bool{}}
		for _, v := range c.Versions {
			if k, ok := parseLine(v); ok {
				s.members[k] = true
			}
		}
		if len(s.members) > 0 {
			return s
		}
		// A list whose entries are not lines (e.g. full versions "1.2.3")
		// falls through to the constraint, which ParseVersionRange filled
		// in for exactly this purpose.
	}
	if strings.TrimSpace(c.Constraint) == "" {
		return versionRange{}
	}
	cs, err := semver.NewConstraint(c.Constraint)
	if err != nil {
		return versionRange{}
	}
	check := func(major, minor, patch uint64) bool {
		return cs.Check(semver.New(major, minor, patch, "", ""))
	}
	s := versionRange{ok: true, members: map[lineKey]bool{}}
	for _, k := range cands {
		// A bare line is admitted when any patch of it is; the probes use
		// the line's first and last conceivable patch. Prerelease suffixes
		// on the bound (">=1.25.0-0") compare correctly against both.
		if check(k.major, k.minor, 0) || check(k.major, k.minor, 999) {
			s.members[k] = true
		}
	}
	if len(cands) > 0 {
		last := cands[len(cands)-1]
		s.openAbove = check(last.major, last.minor+100, 0)
	}
	return s
}

// boundOf recognises a single-line bound for the directional kinds
// ("minimum"/"maximum"). It returns the bound line and whether it is an
// upper bound. Anything a minimum/maximum column states beyond a single
// bound (a full range, a list of lines) is left to face-value evaluation.
func boundOf(c *domain.CompatibilityConstraint) (line lineKey, upper, ok bool) {
	if c.Kind != "minimum" && c.Kind != "maximum" {
		return lineKey{}, false, false
	}
	if len(c.Versions) == 1 {
		if k, ok := parseLine(c.Versions[0]); ok {
			return k, c.Kind == "maximum", true
		}
		return lineKey{}, false, false
	}
	if len(c.Versions) > 0 {
		return lineKey{}, false, false
	}
	m := boundRe.FindStringSubmatch(strings.TrimSpace(c.Constraint))
	if m == nil {
		return lineKey{}, false, false
	}
	if (m[1] == "<=") != (c.Kind == "maximum") {
		return lineKey{}, false, false // direction contradicts the kind
	}
	return lineKey{mustUint(m[2]), mustUint("0" + m[3])}, c.Kind == "maximum", true
}

// boundRange builds the range of a directional bound: the bound line itself
// plus (for a minimum) every line above it, or (for a maximum) every line
// below it.
func boundRange(b lineKey, upper bool, cands []lineKey) versionRange {
	s := versionRange{ok: true, members: map[lineKey]bool{}}
	for _, k := range cands {
		if upper {
			if !b.less(k) { // k <= b
				s.members[k] = true
			}
		} else {
			if !k.less(b) { // k >= b
				s.members[k] = true
			}
		}
	}
	if upper {
		s.openBelow = true
	} else {
		s.openAbove = true
	}
	return s
}

func mustUint(s string) uint64 {
	var n uint64
	for _, d := range s {
		n = n*10 + uint64(d-'0')
	}
	return n
}

// display renders the admitted lines for humans, in the conventions of the
// edge summaries: runs of lines ("1.29–1.33"), an open top ("≥ 1.22") and an
// open bottom ("≤ 1.33").
func (s versionRange) display(cands []lineKey) string {
	rs := runs(s.members, cands)
	if s.openAbove && s.openBelow {
		return "any"
	}
	if len(rs) == 0 {
		if s.openAbove || s.openBelow {
			return "any"
		}
		return "none"
	}
	if s.openAbove && len(cands) > 0 && rs[len(rs)-1].to == cands[len(cands)-1] {
		head := formatRuns(rs[:len(rs)-1])
		tail := "≥ " + rs[len(rs)-1].from.String()
		if head == "" {
			return tail
		}
		return head + ", " + tail
	}
	if s.openBelow && len(cands) > 0 && rs[0].from == cands[0] {
		head := "≤ " + rs[0].to.String()
		if rest := formatRuns(rs[1:]); rest != "" {
			return head + ", " + rest
		}
		return head
	}
	return formatRuns(rs)
}

// run is a consecutive stretch of admitted lines within one major.
type run struct{ from, to lineKey }

// runs groups members into consecutive ranges over the candidate list.
func runs(members map[lineKey]bool, cands []lineKey) []run {
	var out []run
	for i, k := range cands {
		if !members[k] {
			continue
		}
		if n := len(out); n > 0 && i > 0 && cands[i-1] == out[n-1].to && members[cands[i-1]] && cands[i-1].major == k.major {
			out[n-1].to = k
			continue
		}
		out = append(out, run{k, k})
	}
	return out
}

func (r run) String() string {
	if r.from == r.to {
		return r.from.String()
	}
	return r.from.String() + "–" + r.to.String()
}

func formatRuns(rs []run) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = r.String()
	}
	return strings.Join(parts, ", ")
}

// minMember returns the smallest admitted line among the candidates ("" when
// nothing is admitted).
func minMember(s versionRange, cands []lineKey) string {
	for _, k := range cands {
		if s.members[k] {
			return k.String()
		}
	}
	return ""
}
