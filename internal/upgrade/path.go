package upgrade

import (
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Path step reasons recorded in domain.PathStep.Reason.
const (
	ReasonRelease         = "release"           // PolicyAll: every release is traversed
	ReasonPatch           = "patch"             // minor-lineage: patch of From's own line
	ReasonMajorRelease    = "major-release"     // minor-lineage: X.0.0 entering a new major line
	ReasonMinorRelease    = "minor-release"     // minor-lineage: X.Y.0 entering a new minor line
	ReasonLineEntry       = "line-entry"        // minor-lineage: lowest available release of a line that has no X.Y.0
	ReasonTargetLinePatch = "target-line-patch" // minor-lineage: patch of To's line up to To
)

// lineKey identifies a release line (major.minor).
type lineKey struct{ major, minor uint64 }

func lineOf(v domain.Version) lineKey { return lineKey{v.Major(), v.Minor()} }

func (a lineKey) less(b lineKey) bool {
	if a.major != b.major {
		return a.major < b.major
	}
	return a.minor < b.minor
}

func (a lineKey) String() string { return fmt.Sprintf("%d.%d", a.major, a.minor) }

// PolicyForLineage maps a definition lineage ("minor", "linear", "") to a path policy.
func PolicyForLineage(lineage string) string {
	switch lineage {
	case catalog.LineageMinor, PolicyMinorLineage:
		return PolicyMinorLineage
	default:
		return PolicyAll
	}
}

// uniqueSorted returns versions sorted ascending with semantic duplicates removed
// (the first occurrence wins).
func uniqueSorted(versions []domain.Version) []domain.Version {
	out := make([]domain.Version, 0, len(versions))
	out = append(out, versions...)
	domain.SortVersions(out)
	dedup := out[:0]
	for _, v := range out {
		if len(dedup) > 0 && dedup[len(dedup)-1].Equal(v) {
			continue
		}
		dedup = append(dedup, v)
	}
	return dedup
}

func indexOfVersion(vs []domain.Version, v domain.Version) int {
	for i := range vs {
		if vs[i].Equal(v) {
			return i
		}
	}
	return -1
}

func selectPath(versions []domain.Version, from, to domain.Version, lineage string) (*PathSelection, error) {
	policy := PolicyForLineage(lineage)
	all := uniqueSorted(versions)
	fi, ti := indexOfVersion(all, from), indexOfVersion(all, to)
	if fi < 0 {
		return nil, fmt.Errorf("upgrade: from version %s is not a known release", from)
	}
	if ti < 0 {
		return nil, fmt.Errorf("upgrade: to version %s is not a known release", to)
	}
	if fi >= ti {
		return nil, fmt.Errorf("upgrade: from version %s must be lower than to version %s", from, to)
	}
	from, to = all[fi], all[ti]
	between := all[fi+1 : ti+1] // (from, to]

	var include func(v domain.Version) bool
	switch policy {
	case PolicyMinorLineage:
		fromLine, toLine := lineOf(from), lineOf(to)
		// lowest stable release of every line strictly after from's line
		lowest := map[lineKey]domain.Version{}
		for _, v := range between {
			if v.IsPrerelease() {
				continue
			}
			k := lineOf(v)
			if _, ok := lowest[k]; !ok {
				lowest[k] = v // between is ascending, so the first one is the lowest
			}
		}
		include = func(v domain.Version) bool {
			if v.Equal(to) {
				return true
			}
			if v.IsPrerelease() {
				return false
			}
			k := lineOf(v)
			if fromLine == toLine {
				return true // same line: every patch in (from, to]
			}
			if k == fromLine {
				return false // backports on from's line after from
			}
			if k == toLine {
				return true // every release of to's line up to to
			}
			low, ok := lowest[k]
			return ok && low.Equal(v)
		}
	default:
		include = func(v domain.Version) bool { return v.Equal(to) || !v.IsPrerelease() }
	}

	sel := &PathSelection{Policy: policy, Path: []domain.Version{}, Skipped: []domain.Version{}}
	for _, v := range between {
		if include(v) {
			sel.Path = append(sel.Path, v)
		} else {
			sel.Skipped = append(sel.Skipped, v)
		}
	}
	return sel, nil
}

// stepReasons labels each path version with why it is traversed.
func stepReasons(policy string, from domain.Version, path []domain.Version) []string {
	out := make([]string, len(path))
	if policy != PolicyMinorLineage {
		for i := range out {
			out[i] = ReasonRelease
		}
		return out
	}
	fromLine := lineOf(from)
	seen := map[lineKey]bool{}
	for i, v := range path {
		k := lineOf(v)
		switch {
		case k == fromLine:
			out[i] = ReasonPatch
		case !seen[k]:
			switch {
			case v.Patch() == 0 && v.Minor() == 0:
				out[i] = ReasonMajorRelease
			case v.Patch() == 0:
				out[i] = ReasonMinorRelease
			default:
				out[i] = ReasonLineEntry
			}
		default:
			out[i] = ReasonTargetLinePatch
		}
		seen[k] = true
	}
	return out
}
