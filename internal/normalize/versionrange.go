package normalize

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// ParseVersionRange grammar
//
//	input     := constraint | list
//	constraint:= any string containing a comparison operator, "~", "^", "!",
//	             "*" or "||" that is a valid Masterminds semver constraint
//	             (">= 1.22.0-0", "^1.2", "1.2.*"). It is returned unchanged
//	             (after unicode normalisation of ≥ and ≤); Versions is nil.
//	list      := item { sep item }            sep := "," | ";" | "&" | "and" | "or"
//	item      := range | open | single
//	range     := V arrow V                    arrow := "→" | "->" | "=>" | "-" | "–" | "—" | "to" | "through" | "..."
//	open      := V "+" | V ("and"|"or") ("above"|"later"|"newer"|"higher")
//	single    := V
//	V         := ["v"] N [ "." (N|"x") [ "." (N|"x") ] ] [ "-" prerelease ]  (prerelease only for 3-part singles)
//
// Granularity decides the meaning of a version: a 2-part version "1.29" is a
// whole minor line and includes all of its patches and prereleases
// (">=1.29.0-0, <1.30.0-0"), a 1-part version "4" a whole major, a 3-part
// version "1.29.3" exactly that version. The upper bound of a range is
// inclusive at the granularity it is written in: "1.29 → 1.33" is
// ">=1.29.0-0, <1.34.0-0", "1.29.0 - 1.29.5" is ">=1.29.0, <=1.29.5".
//
// Items that are 2-part minors of the same major are merged: contiguous runs
// become one range, gaps become alternatives joined by " || ". The resulting
// constraint always parses with semver.NewConstraint.
//
// versions enumerates the covered versions (ascending, without "v", as written
// at minor granularity: "1.29") when every item is enumerable: minor singles,
// minor ranges within one major (at most maxEnumerated lines), major ranges and
// 3-part singles. Open-ended items and ranges spanning majors at minor
// granularity yield versions == nil.
func parseVersionRange(raw string) (string, []string, error) {
	s := normalizeRangeInput(raw)
	if s == "" {
		return "", nil, fmt.Errorf("normalize: empty version range")
	}
	if strings.ContainsAny(s, "<>=~^!|*") {
		if _, err := semver.NewConstraint(s); err != nil {
			return "", nil, fmt.Errorf("normalize: version range %q: %w", raw, err)
		}
		return s, nil, nil
	}

	s = openPhraseRe.ReplaceAllString(s, "+")
	items := splitItemsRe.Split(s, -1)
	var (
		atoms []rangeAtom
	)
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		a, ok := parseAtom(it)
		if !ok {
			// last resort: a plain semver constraint such as "1.29.x"
			if len(items) == 1 {
				if _, err := semver.NewConstraint(it); err == nil && !strings.ContainsAny(it, " ") && strings.ContainsAny(it, "0123456789") {
					return it, nil, nil
				}
			}
			return "", nil, fmt.Errorf("normalize: cannot parse version range %q", raw)
		}
		atoms = append(atoms, a)
	}
	if len(atoms) == 0 {
		return "", nil, fmt.Errorf("normalize: cannot parse version range %q", raw)
	}
	constraint, versions := composeAtoms(atoms)
	if _, err := semver.NewConstraint(constraint); err != nil {
		return "", nil, fmt.Errorf("normalize: version range %q produced invalid constraint %q: %w", raw, constraint, err)
	}
	return constraint, versions, nil
}

// maxEnumerated bounds the number of minor versions listed for a range.
const maxEnumerated = 200

var (
	verRe        = `v?(\d+)(?:\.(\d+|[xX]))?(?:\.(\d+|[xX]))?`
	rangeRe      = regexp.MustCompile(`^` + verRe + `\s*(?:→|->|=>|–|—|-|\bto\b|\bthrough\b|\.\.\.?)\s*` + verRe + `$`)
	openRe       = regexp.MustCompile(`^` + verRe + `\s*\+$`)
	singleRe     = regexp.MustCompile(`^` + verRe + `$`)
	preRe        = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z][0-9A-Za-z.-]*)?(\+[0-9A-Za-z.-]+)?$`)
	openPhraseRe = regexp.MustCompile(`(?i)\s*\b(?:and|or)\s+(?:above|later|newer|higher)\b`)
	splitItemsRe = regexp.MustCompile(`(?i)\s*[,;&]\s*|\s+and\s+|\s+or\s+`)
)

func normalizeRangeInput(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.NewReplacer("\u00a0", " ", "\u2265", ">=", "\u2264", "<=", "->", "\u2192", "=>", "\u2192", "`", "").Replace(s)
	s = strings.Trim(s, " \t\"'")
	return collapseSpace(s)
}

// ver is a possibly partial version.
type ver struct {
	major, minor, patch uint64
	parts               int // 1, 2 or 3 numeric parts
	pre                 string
}

func (v ver) cmp(o ver) int {
	for _, p := range [][2]uint64{{v.major, o.major}, {v.minor, o.minor}, {v.patch, o.patch}} {
		if p[0] < p[1] {
			return -1
		}
		if p[0] > p[1] {
			return 1
		}
	}
	return 0
}

func (v ver) String() string {
	switch v.parts {
	case 1:
		return strconv.FormatUint(v.major, 10)
	case 2:
		return fmt.Sprintf("%d.%d", v.major, v.minor)
	}
	return fmt.Sprintf("%d.%d.%d%s", v.major, v.minor, v.patch, v.pre)
}

func verFromMatch(m []string) (ver, bool) {
	var v ver
	maj, err := strconv.ParseUint(m[1], 10, 32)
	if err != nil {
		return v, false
	}
	v.major, v.parts = maj, 1
	if m[2] != "" && !strings.EqualFold(m[2], "x") {
		min, err := strconv.ParseUint(m[2], 10, 32)
		if err != nil {
			return v, false
		}
		v.minor, v.parts = min, 2
		if m[3] != "" && !strings.EqualFold(m[3], "x") {
			p, err := strconv.ParseUint(m[3], 10, 32)
			if err != nil {
				return v, false
			}
			v.patch, v.parts = p, 3
		}
	}
	return v, true
}

type atomKind uint8

const (
	atomMinor atomKind = iota // single 2-part version, or a 2-part range within one major
	atomOther
)

type rangeAtom struct {
	kind     atomKind
	major    uint64
	minLo    uint64   // atomMinor: first minor
	minHi    uint64   // atomMinor: last minor
	terms    []string // atomOther: AND-ed constraint terms
	versions []string // atomOther: enumerated versions (nil when not enumerable)
	enum     bool     // atomOther: versions is complete
}

func (v ver) lower() string {
	switch v.parts {
	case 1:
		return fmt.Sprintf(">=%d.0.0-0", v.major)
	case 2:
		return fmt.Sprintf(">=%d.%d.0-0", v.major, v.minor)
	}
	return fmt.Sprintf(">=%d.%d.%d", v.major, v.minor, v.patch)
}

func (v ver) upper() string {
	switch v.parts {
	case 1:
		return fmt.Sprintf("<%d.0.0-0", v.major+1)
	case 2:
		return fmt.Sprintf("<%d.%d.0-0", v.major, v.minor+1)
	}
	return fmt.Sprintf("<=%d.%d.%d", v.major, v.minor, v.patch)
}

func parseAtom(it string) (rangeAtom, bool) {
	if m := openRe.FindStringSubmatch(it); m != nil {
		v, ok := verFromMatch(m)
		if !ok {
			return rangeAtom{}, false
		}
		return rangeAtom{kind: atomOther, terms: []string{v.lower()}}, true
	}
	if m := rangeRe.FindStringSubmatch(it); m != nil {
		lo, ok1 := verFromMatch(m[:4])
		hi, ok2 := verFromMatch(append([]string{m[0]}, m[4:7]...))
		if ok1 && ok2 && lo.cmp(hi) <= 0 {
			if lo.parts == 2 && hi.parts == 2 && lo.major == hi.major && hi.minor-lo.minor < maxEnumerated {
				return rangeAtom{kind: atomMinor, major: lo.major, minLo: lo.minor, minHi: hi.minor}, true
			}
			a := rangeAtom{kind: atomOther, terms: []string{lo.lower(), hi.upper()}}
			switch {
			case lo.parts == 1 && hi.parts == 1 && hi.major-lo.major < maxEnumerated:
				for x := lo.major; x <= hi.major; x++ {
					a.versions = append(a.versions, strconv.FormatUint(x, 10))
				}
				a.enum = true
			case lo.parts == 3 && hi.parts == 3 && lo.cmp(hi) == 0:
				a.versions, a.enum = []string{lo.String()}, true
			}
			return a, true
		}
		// not a range ("1.22.0-0" is a version with a prerelease): fall through
	}
	if m := singleRe.FindStringSubmatch(it); m != nil {
		v, ok := verFromMatch(m)
		if !ok {
			return rangeAtom{}, false
		}
		switch v.parts {
		case 2:
			return rangeAtom{kind: atomMinor, major: v.major, minLo: v.minor, minHi: v.minor}, true
		case 1:
			return rangeAtom{kind: atomOther, terms: []string{v.lower(), v.upper()}, versions: []string{v.String()}, enum: true}, true
		}
		return rangeAtom{kind: atomOther, terms: []string{v.String()}, versions: []string{v.String()}, enum: true}, true
	}
	if m := preRe.FindStringSubmatch(it); m != nil {
		s := strings.TrimPrefix(it, "v")
		return rangeAtom{kind: atomOther, terms: []string{s}, versions: []string{s}, enum: true}, true
	}
	return rangeAtom{}, false
}

// composeAtoms merges atoms into one constraint string and, when possible,
// the enumerated version list.
func composeAtoms(atoms []rangeAtom) (string, []string) {
	minors := map[uint64]map[uint64]bool{}
	var others []rangeAtom
	enumerable := true
	var listed []string
	for _, a := range atoms {
		if a.kind == atomMinor {
			set := minors[a.major]
			if set == nil {
				set = map[uint64]bool{}
				minors[a.major] = set
			}
			for m := a.minLo; m <= a.minHi; m++ {
				set[m] = true
			}
			continue
		}
		others = append(others, a)
		if !a.enum {
			enumerable = false
		}
		listed = append(listed, a.versions...)
	}

	majors := make([]uint64, 0, len(minors))
	for m := range minors {
		majors = append(majors, m)
	}
	sort.Slice(majors, func(i, j int) bool { return majors[i] < majors[j] })

	var alternatives []string
	var versions []string
	for _, maj := range majors {
		var ms []uint64
		for m := range minors[maj] {
			ms = append(ms, m)
		}
		sort.Slice(ms, func(i, j int) bool { return ms[i] < ms[j] })
		for i := 0; i < len(ms); {
			j := i
			for j+1 < len(ms) && ms[j+1] == ms[j]+1 {
				j++
			}
			alternatives = append(alternatives, fmt.Sprintf(">=%d.%d.0-0, <%d.%d.0-0", maj, ms[i], maj, ms[j]+1))
			i = j + 1
		}
		if enumerable {
			for _, m := range ms {
				versions = append(versions, fmt.Sprintf("%d.%d", maj, m))
			}
		}
	}
	for _, a := range others {
		alternatives = append(alternatives, strings.Join(a.terms, ", "))
	}
	if !enumerable {
		return strings.Join(alternatives, " || "), nil
	}
	versions = append(versions, listed...)
	versions = sortUniqueVersions(versions)
	if len(versions) > maxEnumerated*5 {
		versions = nil
	}
	return strings.Join(alternatives, " || "), versions
}

// sortUniqueVersions sorts partial versions numerically and removes duplicates.
func sortUniqueVersions(vs []string) []string {
	type pv struct {
		s   string
		key [3]uint64
		n   int
		pre string
	}
	seen := map[string]bool{}
	var list []pv
	for _, s := range vs {
		if seen[s] {
			continue
		}
		seen[s] = true
		x := pv{s: s}
		core := s
		if i := strings.IndexAny(core, "-+"); i >= 0 {
			x.pre = core[i:]
			core = core[:i]
		}
		for i, part := range strings.Split(core, ".") {
			if i > 2 {
				break
			}
			n, _ := strconv.ParseUint(part, 10, 64)
			x.key[i] = n
			x.n = i + 1
		}
		list = append(list, x)
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		for k := 0; k < 3; k++ {
			if a.key[k] != b.key[k] {
				return a.key[k] < b.key[k]
			}
		}
		if a.n != b.n {
			return a.n < b.n
		}
		return a.pre < b.pre
	})
	out := make([]string, len(list))
	for i, x := range list {
		out[i] = x.s
	}
	return out
}
