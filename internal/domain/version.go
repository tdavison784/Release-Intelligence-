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
	// capture group) holding the semantic version portion of the tag.
	Pattern *regexp.Regexp
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
	return c.Check(sv), nil
}

// SortVersions sorts ascending in place.
func SortVersions(vs []Version) {
	sort.SliceStable(vs, func(i, j int) bool { return vs[i].Less(vs[j]) })
}
