package github

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// clauseRe splits one comparison of a GitHub version range into operator and
// version: ">= 1.0.0", "<1.17.2", "= 1.2.3" or a bare "1.2.3".
var clauseRe = regexp.MustCompile(`^(<=|>=|==|=|<|>)?\s*(\S+)$`)

// pseudoRe matches the pre-release part of a Go pseudo-version: a 14-digit
// UTC timestamp, optionally followed by a 12-hex-digit commit
// ("20260319175635-5183b7d86137", "0.20260331202714-6800989bdc12").
var pseudoRe = regexp.MustCompile(`(^|[.-])\d{14}(-[0-9a-f]{12})?$`)

// ConvertVersionRange converts one GitHub advisory "vulnerable_version_range"
// into a constraint understood by github.com/Masterminds/semver/v3.
//
// GitHub writes a range as comma-separated comparisons that must all hold:
//
//	">= 1.0.0, < 1.17.2"   becomes ">= 1.0.0, < 1.17.2"
//	"<= 1.2.3"             becomes "<= 1.2.3"
//	"= 1.2.3"              becomes "= 1.2.3"
//	"1.2.3"                becomes "= 1.2.3"
//	">=1.0.0,<1.2"         becomes ">= 1.0.0, < 1.2"
//
// A leading "v" on a version is dropped. The result is validated with
// semver.NewConstraint; a range that cannot be expressed (unparsable
// versions, unknown operators) is an error.
//
// Two shapes are not an intersection of comparisons and are handled apart:
//
//   - An explicit version list (only bare or "=" versions, more than one) is
//     an OR of exact versions: "v1.2.0,v1.2.1" becomes "= 1.2.0 || = 1.2.1".
//     Read as an intersection it would match nothing, a silent "not
//     affected" for every version.
//   - A range with two lower bounds or two upper bounds
//     ("<v1.12.14,<v1.15.4,<v1.16.2", per-line fixes written as one list) is
//     an error: its intersection is narrower than what the author meant, so
//     it would declare affected versions unaffected. A list that mixes exact
//     versions with comparisons is an error for the same reason.
//
// Note that Masterminds constraints exclude pre-releases of a version unless
// the constraint itself names a pre-release, so "< 1.17.2" does not match
// "1.17.2-rc.1". A pre-release lower bound still matches the stable release
// it precedes: ">= 1.12.0-beta.0, < 1.12.1" matches 1.12.0.
//
// A Go pseudo-version bound ("< 0.0.0-20260319175635-5183b7d86137", how the
// Go database records a fix on a commit) is an error too. Compared as semver
// it sorts below every ordinary release, so it would match none of them, yet
// which releases contain the commit is unknown: reading it as "no release is
// affected" would be a false "not affected". It stays an advisory to check
// manually.
func ConvertVersionRange(r string) (string, error) {
	r = strings.TrimSpace(r)
	if r == "" {
		return "", fmt.Errorf("empty version range")
	}
	var clauses []string
	exact, lower, upper := 0, 0, 0
	for _, part := range strings.Split(r, ",") {
		part = strings.TrimSpace(part)
		m := clauseRe.FindStringSubmatch(part)
		if m == nil {
			return "", fmt.Errorf("version range %q: cannot parse comparison %q", r, part)
		}
		op, ver := m[1], strings.TrimPrefix(m[2], "v")
		switch op {
		case "", "==", "=":
			op = "="
			exact++
		case ">", ">=":
			lower++
		case "<", "<=":
			upper++
		}
		sv, err := semver.NewVersion(ver)
		if err != nil {
			return "", fmt.Errorf("version range %q: %w", r, err)
		}
		if pseudoRe.MatchString(sv.Prerelease()) {
			return "", fmt.Errorf("version range %q: %s is a Go pseudo-version (a commit, not a release)", r, ver)
		}
		clauses = append(clauses, op+" "+ver)
	}
	sep := ", "
	switch {
	case exact > 1 && lower+upper == 0:
		sep = " || "
	case exact > 0 && len(clauses) > 1:
		return "", fmt.Errorf("version range %q: mixes exact versions with comparisons", r)
	case lower > 1 || upper > 1:
		return "", fmt.Errorf("version range %q: more than one lower or upper bound (not an interval)", r)
	}
	out := strings.Join(clauses, sep)
	if _, err := semver.NewConstraint(out); err != nil {
		return "", fmt.Errorf("version range %q: %w", r, err)
	}
	return out, nil
}

// CombineVersionRanges converts several GitHub ranges (one per vulnerable
// package of an advisory) and unites them with "||". Duplicates are dropped
// and the order of first appearance is kept. Ranges that cannot be converted
// are skipped and returned in skipped, so that callers can report them; the
// result is empty when nothing is convertible. A non-empty result always
// parses with semver.NewConstraint.
func CombineVersionRanges(ranges []string) (constraint string, skipped []error) {
	var converted []string
	seen := map[string]bool{}
	for _, r := range ranges {
		if strings.TrimSpace(r) == "" {
			continue
		}
		c, err := ConvertVersionRange(r)
		if err != nil {
			skipped = append(skipped, err)
			continue
		}
		if !seen[c] {
			seen[c] = true
			converted = append(converted, c)
		}
	}
	out := strings.Join(converted, " || ")
	if out != "" {
		if _, err := semver.NewConstraint(out); err != nil {
			return "", append(skipped, err)
		}
	}
	return out, skipped
}

// ParsePatchedVersions splits GitHub's "patched_versions" string
// ("1.17.2" or "1.16.4, 1.17.2") into versions. Entries that are not
// versions (GitHub also stores free text) and duplicates are dropped; a
// leading "v" is kept as written.
func ParsePatchedVersions(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		v := strings.TrimSpace(part)
		if v == "" || seen[v] {
			continue
		}
		if _, err := semver.NewVersion(strings.TrimPrefix(v, "v")); err != nil {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
