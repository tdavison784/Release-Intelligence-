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
// Note that Masterminds constraints exclude pre-releases of a version unless
// the constraint itself names a pre-release, so "< 1.17.2" does not match
// "1.17.2-rc.1".
func ConvertVersionRange(r string) (string, error) {
	r = strings.TrimSpace(r)
	if r == "" {
		return "", fmt.Errorf("empty version range")
	}
	var clauses []string
	for _, part := range strings.Split(r, ",") {
		part = strings.TrimSpace(part)
		m := clauseRe.FindStringSubmatch(part)
		if m == nil {
			return "", fmt.Errorf("version range %q: cannot parse comparison %q", r, part)
		}
		op, ver := m[1], strings.TrimPrefix(m[2], "v")
		if op == "" || op == "==" {
			op = "="
		}
		if _, err := semver.NewVersion(ver); err != nil {
			return "", fmt.Errorf("version range %q: %w", r, err)
		}
		clauses = append(clauses, op+" "+ver)
	}
	out := strings.Join(clauses, ", ")
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
