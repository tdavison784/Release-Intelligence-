package ingest

import (
	"sort"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// fallbackGroup returns the fallback group of a source: sources sharing the
// same non-empty group are alternatives, tried in Priority order until one is
// ok; sources without a group are always consulted.
func fallbackGroup(src catalog.Source) string { return src.FallbackGroup }

// sourceGroup is a unit of per-release source processing: either a single
// always-consulted source or the members of one fallback group.
type sourceGroup struct {
	name    string // fallback group name; "" for a standalone source
	members []catalog.Source
}

// groupSources groups srcs into processing units using groupOf (normally
// fallbackGroup). Units appear in the order of their first member in srcs;
// members of a fallback group are sorted by Priority (stable, so ties keep
// definition order).
func groupSources(srcs []catalog.Source, groupOf func(catalog.Source) string) []sourceGroup {
	var groups []sourceGroup
	index := map[string]int{}
	for _, s := range srcs {
		g := groupOf(s)
		if g == "" {
			groups = append(groups, sourceGroup{members: []catalog.Source{s}})
			continue
		}
		if k, ok := index[g]; ok {
			groups[k].members = append(groups[k].members, s)
			continue
		}
		index[g] = len(groups)
		groups = append(groups, sourceGroup{name: g, members: []catalog.Source{s}})
	}
	for k := range groups {
		m := groups[k].members
		sort.SliceStable(m, func(a, b int) bool { return m[a].Priority < m[b].Priority })
	}
	return groups
}

// primaryRole is the first role of a source that is consulted per release
// (anything but versions and security).
func primaryRole(src catalog.Source) (domain.SourceRole, bool) {
	for _, r := range src.Roles {
		if r != domain.RoleVersions && r != domain.RoleSecurity {
			return r, true
		}
	}
	return "", false
}
