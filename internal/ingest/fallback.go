package ingest

import (
	"reflect"
	"sort"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// fallbackGroup returns the fallback group of a source: sources sharing the
// same non-empty group are alternatives, tried in Priority order until one is
// ok; sources without a group are always consulted.
//
// MERGE NOTE: the contract moved from `Fallback bool` (alternatives grouped
// by role) to `FallbackGroup string` (explicit, role-independent groups). The
// field is read by name so this package compiles against both revisions of
// package catalog; once the new contract is merged this can become a plain
// `return src.FallbackGroup`. With the legacy field, the group is the
// source's primary role.
func fallbackGroup(src catalog.Source) string {
	rv := reflect.ValueOf(src)
	if f := rv.FieldByName("FallbackGroup"); f.IsValid() && f.Kind() == reflect.String {
		return f.String()
	}
	if f := rv.FieldByName("Fallback"); f.IsValid() && f.Kind() == reflect.Bool && f.Bool() {
		if role, ok := primaryRole(src); ok {
			return string(role)
		}
		if len(src.Roles) > 0 {
			return string(src.Roles[0])
		}
	}
	return ""
}

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
