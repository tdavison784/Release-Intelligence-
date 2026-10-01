package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// VersionScheme names how a product numbers its releases.
type VersionScheme string

const (
	// SchemeSemver is semantic versioning (optionally with a tag prefix such as "v").
	SchemeSemver VersionScheme = "semver"
)

// Version is a release version of a product. Tag is the identifier exactly as
// published by the canonical release channel (e.g. "v1.18.0"); Semver is the
// normalised semantic version without prefix (e.g. "1.18.0").
type Version struct {
	Tag    string `json:"tag"`
	Semver string `json:"version"`

	sv *semver.Version
}

// VersionParser converts published tags into Versions for one product.
type VersionParser struct {
	Scheme VersionScheme
	// Pattern must contain a capture group named "version" (or a first
	// capture group) holding the semantic version portion of the tag, or the
	// component groups described at HasVersionGroups.
	Pattern *regexp.Regexp
}

// HasVersionGroups reports whether a tag pattern says how to read a version:
// a named group "version" (the semantic version verbatim) or a named group
// "major". With "major" the version is assembled from the named groups
// major (required), minor and patch (default 0) and prerelease (optional,
// without its leading "-"), so tags whose numbers are not dot-separated or
// not three-part still map to semver: PostgreSQL REL_17_2 (major=17,
// patch=2), Ruby v3_3_0, Go go1.22, Linux v6.7, Python v3.13.0rc1.
func HasVersionGroups(re *regexp.Regexp) bool {
	return re != nil && (re.SubexpIndex("version") > 0 || re.SubexpIndex("major") > 0)
}

// componentVersion assembles a semantic version from the named groups of m.
func componentVersion(pat *regexp.Regexp, m []string) string {
	get := func(name string, def string) string {
		if i := pat.SubexpIndex(name); i > 0 && m[i] != "" {
			return m[i]
		}
		return def
	}
	raw := get("major", "") + "." + get("minor", "0") + "." + get("patch", "0")
	if pre := get("prerelease", ""); pre != "" {
		raw += "-" + pre
	}
	return raw
}

// DefaultTagPattern builds the conventional pattern for a prefix such as "v" or "".
func DefaultTagPattern(prefix string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$`)
}

// Parse parses a tag. It returns an error when the tag is not a release tag of
// this product (e.g. a chart tag in a mono-repo).
func (p VersionParser) Parse(tag string) (Version, error) {
	tag = strings.TrimSpace(tag)
	pat := p.Pattern
	if pat == nil {
		pat = DefaultTagPattern("")
	}
	m := pat.FindStringSubmatch(tag)
	if m == nil {
		return Version{}, fmt.Errorf("tag %q does not match pattern %s", tag, pat)
	}
	raw := ""
	if i := pat.SubexpIndex("version"); i > 0 {
		raw = m[i]
	} else if pat.SubexpIndex("major") > 0 {
		raw = componentVersion(pat, m)
	} else if len(m) > 1 {
		raw = m[1]
	} else {
		raw = m[0]
	}
	sv, err := semver.StrictNewVersion(raw)
	if err != nil {
		return Version{}, fmt.Errorf("tag %q: %w", tag, err)
	}
	return Version{Tag: tag, Semver: sv.String(), sv: sv}, nil
}

// MustVersion builds a Version from a tag and semver string; it panics on
// invalid input and is intended for tests and literals.
func MustVersion(tag, version string) Version {
	sv := semver.MustParse(version)
	return Version{Tag: tag, Semver: sv.String(), sv: sv}
}

func (v Version) parsed() *semver.Version {
	if v.sv != nil {
		return v.sv
	}
	sv, err := semver.NewVersion(v.Semver)
	if err != nil {
		return nil
	}
	return sv
}

// IsZero reports whether v is unset.
func (v Version) IsZero() bool { return v.Tag == "" && v.Semver == "" }

// String returns the published tag.
func (v Version) String() string {
	if v.Tag != "" {
		return v.Tag
	}
	return v.Semver
}

// Major, Minor and Patch return the numeric components (0 when unparsable).
func (v Version) Major() uint64 {
	if sv := v.parsed(); sv != nil {
		return sv.Major()
	}
	return 0
}
func (v Version) Minor() uint64 {
	if sv := v.parsed(); sv != nil {
		return sv.Minor()
	}
	return 0
}
func (v Version) Patch() uint64 {
	if sv := v.parsed(); sv != nil {
		return sv.Patch()
	}
	return 0
}

// Prerelease returns the prerelease suffix ("rc.1"), or "".
func (v Version) Prerelease() string {
	if sv := v.parsed(); sv != nil {
		return sv.Prerelease()
	}
	return ""
}

// IsPrerelease reports whether the version carries a prerelease suffix.
func (v Version) IsPrerelease() bool { return v.Prerelease() != "" }

// Line returns the minor release line, e.g. "1.18".
func (v Version) Line() string { return fmt.Sprintf("%d.%d", v.Major(), v.Minor()) }

// Compare returns -1, 0 or +1 comparing semantic versions.
func (v Version) Compare(o Version) int {
	a, b := v.parsed(), o.parsed()
	switch {
	case a == nil && b == nil:
		return strings.Compare(v.Semver, o.Semver)
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return a.Compare(b)
}

// Less reports whether v sorts before o.
func (v Version) Less(o Version) bool { return v.Compare(o) < 0 }

// Equal reports whether both versions are semantically equal.
func (v Version) Equal(o Version) bool { return v.Compare(o) == 0 }

// Satisfies reports whether v satisfies a semver constraint such as ">= 1.6.0".
// An empty constraint is always satisfied.
//
// A prerelease version is evaluated as its release version: 1.21.0-alpha.1
// satisfies ">= 1.21.0" and does not satisfy "< 1.21.0". (Plain semver
// constraint checking never matches a prerelease against a constraint that
// has no prerelease part, so ">= 1.5.0" would exclude 1.19.0-alpha.0.) This
// is what availability constraints need: a prerelease of a release belongs to
// the same generation as the release itself.
func (v Version) Satisfies(constraint string) (bool, error) {
	if strings.TrimSpace(constraint) == "" {
		return true, nil
	}
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return false, err
	}
	sv := v.parsed()
	if sv == nil {
		return false, fmt.Errorf("version %q is not semver", v.Semver)
	}
	if sv.Prerelease() != "" || sv.Metadata() != "" {
		sv = semver.New(sv.Major(), sv.Minor(), sv.Patch(), "", "")
	}
	return c.Check(sv), nil
}

// SortVersions sorts ascending in place.
func SortVersions(vs []Version) {
	sort.SliceStable(vs, func(i, j int) bool { return vs[i].Less(vs[j]) })
}
