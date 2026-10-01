package domain

import (
	"regexp"
	"testing"
)

// A pattern with component groups (major, minor, patch, prerelease) maps tags
// that are not dot-separated or not three-part to semantic versions.
func TestVersionParseComponentGroups(t *testing.T) {
	cases := []struct {
		name, pattern string
		tag, semver   string // semver "" = must not parse
		line          string
	}{
		// PostgreSQL: two-part versions since 10 (major.patch), three-part before.
		{"pg 17.2", pgPattern, "REL_17_2", "17.0.2", "17.0"},
		{"pg 17.0", pgPattern, "REL_17_0", "17.0.0", "17.0"},
		{"pg 10.23", pgPattern, "REL_10_23", "10.0.23", "10.0"},
		{"pg 9.6.24", pgPattern, "REL9_6_24", "9.6.24", "9.6"},
		{"pg 17 rc", pgPattern, "REL_17_RC1", "17.0.0-RC1", "17.0"},
		{"pg 9.6 beta", pgPattern, "REL9_6_BETA3", "9.6.0-BETA3", "9.6"},
		{"pg ancient tag", pgPattern, "REL6_5", "", ""},
		{"pg 7.x two-part tag is ambiguous, not parsed", pgPattern, "REL7_4", "", ""},
		{"pg unrelated tag", pgPattern, "PG95-1_01", "", ""},
		{"pg branch-like", pgPattern, "REL_17_STABLE", "", ""},
		// Other products: two-part, underscore-separated, glued prerelease.
		{"go first release", `^go(?P<major>\d+)\.(?P<minor>\d+)(?:\.(?P<patch>\d+))?(?:rc(?P<prerelease>\d+))?$`, "go1.22", "1.22.0", "1.22"},
		{"go patch", `^go(?P<major>\d+)\.(?P<minor>\d+)(?:\.(?P<patch>\d+))?$`, "go1.22.3", "1.22.3", "1.22"},
		{"linux", `^v(?P<major>\d+)\.(?P<minor>\d+)(?:\.(?P<patch>\d+))?$`, "v6.7", "6.7.0", "6.7"},
		{"ruby", `^v(?P<major>\d+)_(?P<minor>\d+)_(?P<patch>\d+)$`, "v3_3_0", "3.3.0", "3.3"},
		{"python rc", `^v(?P<major>\d+)\.(?P<minor>\d+)\.(?P<patch>\d+)(?:(?P<prerelease>(?:a|b|rc)\d+))?$`, "v3.13.0rc1", "3.13.0-rc1", "3.13"},
		// "version" still wins when both kinds of group are present.
		{"version wins", `^v(?P<version>\d+\.\d+\.\d+)-(?P<major>\d+)$`, "v1.2.3-9", "1.2.3", "1.2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := VersionParser{Pattern: regexp.MustCompile(c.pattern)}
			v, err := p.Parse(c.tag)
			if c.semver == "" {
				if err == nil {
					t.Fatalf("%s parsed as %+v, want mismatch", c.tag, v)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if v.Semver != c.semver || v.Tag != c.tag || v.Line() != c.line {
				t.Fatalf("got tag=%q semver=%q line=%q; want %q %q %q", v.Tag, v.Semver, v.Line(), c.tag, c.semver, c.line)
			}
		})
	}
}

// PostgreSQL's line semantics: majors since 10 are lines of "N.0.x".
func TestVersionComponentOrdering(t *testing.T) {
	p := VersionParser{Pattern: regexp.MustCompile(pgPattern)}
	var vs []Version
	for _, tag := range []string{"REL_17_2", "REL9_6_24", "REL_17_RC1", "REL_10_0", "REL_17_0", "REL_17_10", "REL9_6_0", "REL_17_BETA1"} {
		v, err := p.Parse(tag)
		if err != nil {
			t.Fatal(err)
		}
		vs = append(vs, v)
	}
	SortVersions(vs)
	var got []string
	for _, v := range vs {
		got = append(got, v.Tag)
	}
	want := []string{"REL9_6_0", "REL9_6_24", "REL_10_0", "REL_17_BETA1", "REL_17_RC1", "REL_17_0", "REL_17_2", "REL_17_10"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestHasVersionGroups(t *testing.T) {
	for pat, want := range map[string]bool{
		`^v(?P<version>\d+)$`:            true,
		`^v(?P<major>\d+)_(\d+)$`:        true,
		`^v(\d+)\.(\d+)$`:                false,
		`^v(?P<minor>\d+)$`:              false,
		`^REL_(?P<major>\d+)_(\d+)$`:     true,
		`^(?P<patch>\d+)(?P<minor>\d+)$`: false,
	} {
		if got := HasVersionGroups(regexp.MustCompile(pat)); got != want {
			t.Errorf("HasVersionGroups(%s) = %v, want %v", pat, got, want)
		}
	}
	if HasVersionGroups(nil) {
		t.Error("nil pattern has no version groups")
	}
}

const pgPattern = `^REL_?(?P<major>[89]|\d{2})_(?:(?P<minor>\d+)_)?(?:(?P<patch>\d+)|(?P<prerelease>(?:ALPHA|BETA|RC)\d+))$`
