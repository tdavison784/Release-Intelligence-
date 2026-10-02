package upgrade

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// CRD diff rules.
const (
	RuleCRDRemoved           = "crd:removed"
	RuleCRDAdded             = "crd:added"
	RuleCRDVersionRemoved    = "crd:version-removed"
	RuleCRDVersionUnserved   = "crd:version-unserved"
	RuleCRDVersionAdded      = "crd:version-added"
	RuleCRDStorageChanged    = "crd:storage-changed"
	RuleCRDVersionDeprecated = "crd:version-deprecated"
	RuleCRDFieldsRemoved     = "crd:fields-removed"
	RuleCRDFieldsAdded       = "crd:fields-added"
	// schema-attribute diffs of paths present on both sides (aggregated per
	// CRD version, like the field add/remove rules)
	RuleCRDDefaultChanged  = "crd:default-changed"
	RuleCRDEnumChanged     = "crd:enum-changed"
	RuleCRDFieldRequired   = "crd:field-required"
	RuleCRDFieldTypeChange = "crd:field-type-changed"
)

func crdLabel(c domain.CRDSummary) string {
	if c.Kind != "" {
		return c.Kind
	}
	return c.Name
}

func groupVersion(c domain.CRDSummary, version string) string {
	if c.Group == "" {
		return version
	}
	return c.Group + "/" + version
}

func storageVersion(c domain.CRDSummary) string {
	for _, v := range c.Versions {
		if v.Storage {
			return v.Name
		}
	}
	return ""
}

func versionNames(c domain.CRDSummary) []string {
	out := make([]string, 0, len(c.Versions))
	for _, v := range c.Versions {
		out = append(out, v.Name)
	}
	return out
}

// pathRoots returns paths whose parent path is not itself in the list.
func pathRoots(paths []string) []string {
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	var out []string
	for _, p := range paths {
		root := true
		for i := strings.LastIndexByte(p, '.'); i > 0; i = strings.LastIndexByte(p[:i], '.') {
			if set[p[:i]] {
				root = false
				break
			}
		}
		if root {
			out = append(out, p)
		}
	}
	return out
}

func diffStrings(a, b []string) (onlyA, onlyB []string) {
	inA, inB := map[string]bool{}, map[string]bool{}
	for _, x := range a {
		inA[x] = true
	}
	for _, x := range b {
		inB[x] = true
	}
	for _, x := range a {
		if !inB[x] {
			onlyA = append(onlyA, x)
		}
	}
	for _, x := range b {
		if !inA[x] {
			onlyB = append(onlyB, x)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	return dedupStrings(onlyA), dedupStrings(onlyB)
}

func dedupStrings(xs []string) []string {
	out := xs[:0]
	for i, x := range xs {
		if i > 0 && xs[i-1] == x {
			continue
		}
		out = append(out, x)
	}
	return out
}

// crdChanges diffs the CRD snapshots of every artifact present on both sides.
func (b *builder) crdChanges() {
	ids, from, to := b.snapshotPairs(domain.SnapshotCRDs, "CRDs", func(s *domain.Snapshot) bool { return s.CRDs != nil })
	for _, id := range ids {
		b.diffCRDs(id, from[id], to[id])
	}
}

func (b *builder) diffCRDs(artifactID string, f, t *domain.Snapshot) {
	evidence := append(append([]domain.EvidenceID{}, f.Evidence...), t.Evidence...)
	toTag := b.to.Version.String()
	fromBy, toBy := map[string]domain.CRDSummary{}, map[string]domain.CRDSummary{}
	for _, c := range f.CRDs.CRDs {
		fromBy[c.Name] = c
	}
	for _, c := range t.CRDs.CRDs {
		toBy[c.Name] = c
	}
	add := func(c domain.Change, rule string, parts ...string) {
		c.Provenance = computed(rule)
		c.Evidence = evidence
		b.addChange(c, append([]string{rule, artifactID}, parts...)...)
	}

	for _, name := range sortedKeys(fromBy) {
		if _, ok := toBy[name]; ok {
			continue
		}
		c := fromBy[name]
		add(domain.Change{
			Category: domain.CategoryCRDSchema,
			Breaking: true,
			Title:    fmt.Sprintf("CRD %s (%s) removed", code(name), crdLabel(c)),
			Detail: fmt.Sprintf("%s no longer ships this CustomResourceDefinition (versions %s). Existing %s resources are no longer reconciled; migrate or delete them before upgrading.",
				toTag, strings.Join(versionNames(c), ", "), crdLabel(c)),
			Subjects: []string{name},
		}, RuleCRDRemoved, name)
	}
	for _, name := range sortedKeys(toBy) {
		if _, ok := fromBy[name]; ok {
			continue
		}
		c := toBy[name]
		add(domain.Change{
			Category: domain.CategoryCRDSchema,
			Title:    fmt.Sprintf("New CRD %s (%s, %s)", code(name), crdLabel(c), strings.Join(versionNames(c), ", ")),
			Detail:   "Install or update CRDs before upgrading if you manage them separately from the release.",
			Subjects: []string{name},
		}, RuleCRDAdded, name)
	}

	for _, name := range sortedKeys(toBy) {
		fc, ok := fromBy[name]
		if !ok {
			continue
		}
		tc := toBy[name]
		label := crdLabel(tc)
		fv := map[string]domain.CRDVersionInfo{}
		for _, v := range fc.Versions {
			fv[v.Name] = v
		}
		tv := map[string]domain.CRDVersionInfo{}
		for _, v := range tc.Versions {
			tv[v.Name] = v
		}

		for _, v := range fc.Versions {
			gv := groupVersion(tc, v.Name)
			subject := name + "/" + v.Name
			nv, still := tv[v.Name]
			switch {
			case !still && v.Served:
				add(domain.Change{
					Category:       domain.CategoryAPI,
					Breaking:       true,
					ActionRequired: true,
					Title:          fmt.Sprintf("API version %s of %s removed", code(gv), label),
					Detail:         fmt.Sprintf("Manifests, Helm charts and clients using %s for %s fail after the upgrade; migrate them to %s and make sure no stored objects remain at %s.", gv, label, strings.Join(versionNames(tc), ", "), v.Name),
					Subjects:       []string{subject},
				}, RuleCRDVersionRemoved, name, v.Name)
			case !still:
				add(domain.Change{
					Category: domain.CategoryAPI,
					Title:    fmt.Sprintf("Unserved API version %s of %s removed", code(gv), label),
					Subjects: []string{subject},
				}, RuleCRDVersionRemoved, name, v.Name)
			case v.Served && !nv.Served:
				add(domain.Change{
					Category:       domain.CategoryAPI,
					Breaking:       true,
					ActionRequired: true,
					Title:          fmt.Sprintf("API version %s of %s no longer served", code(gv), label),
					Detail:         fmt.Sprintf("Requests using %s for %s are rejected after the upgrade; migrate manifests and clients to a served version.", gv, label),
					Subjects:       []string{subject},
				}, RuleCRDVersionUnserved, name, v.Name)
			}
			if still && !v.Deprecated && nv.Deprecated {
				detail := fmt.Sprintf("%s for %s is deprecated as of %s.", gv, label, toTag)
				if nv.DeprecationWarning != "" {
					detail += " Warning: " + nv.DeprecationWarning
				}
				add(domain.Change{
					Category: domain.CategoryDeprecation,
					Title:    fmt.Sprintf("API version %s of %s deprecated", code(gv), label),
					Detail:   detail,
					Subjects: []string{subject},
				}, RuleCRDVersionDeprecated, name, v.Name)
			}
			if still {
				b.diffSchemaPaths(add, name, label, gv, v, nv)
			}
		}
		for _, v := range tc.Versions {
			if _, ok := fv[v.Name]; ok {
				continue
			}
			gv := groupVersion(tc, v.Name)
			title := fmt.Sprintf("New API version %s of %s", code(gv), label)
			if !v.Served {
				title += " (not served)"
			}
			add(domain.Change{
				Category: domain.CategoryAPI,
				Title:    title,
				Subjects: []string{name + "/" + v.Name},
			}, RuleCRDVersionAdded, name, v.Name)
		}
		if os, ns := storageVersion(fc), storageVersion(tc); os != "" && ns != "" && os != ns {
			add(domain.Change{
				Category:       domain.CategoryAPI,
				ActionRequired: true,
				Title:          fmt.Sprintf("Storage version of %s changes %s → %s", code(name), os, ns),
				Detail:         fmt.Sprintf("Existing %s objects stay stored as %s until rewritten. Run a storage version migration (re-write all objects) and update status.storedVersions before a later release removes %s.", label, os, os),
				Subjects:       []string{name},
			}, RuleCRDStorageChanged, name)
		}
	}
}

func (b *builder) diffSchemaPaths(add func(domain.Change, string, ...string), name, label, gv string, fv, tv domain.CRDVersionInfo) {
	if len(fv.SchemaPaths) == 0 {
		return
	}
	if len(tv.SchemaPaths) == 0 {
		b.warnf("Schema of %s %s was not captured for %s; field diff skipped", name, fv.Name, b.to.Version)
		return
	}
	defer b.diffSchemaAttributes(add, name, label, gv, fv, tv)
	removed, added := diffStrings(fv.SchemaPaths, tv.SchemaPaths)
	if len(removed) > 0 {
		roots := pathRoots(removed)
		add(domain.Change{
			Category: domain.CategoryCRDSchema,
			Breaking: true,
			Title:    fmt.Sprintf("%s %s schema: %s removed: %s", label, fv.Name, plural(len(roots), "field", "fields"), codeList(roots, 3)),
			Detail: fmt.Sprintf("Fields no longer in the %s schema of %s are pruned from stored objects and rejected or dropped in manifests. Remove them from your resources.\nRemoved paths:\n%s",
				gv, name, strings.Join(removed, "\n")),
			Subjects: removed,
		}, RuleCRDFieldsRemoved, name, fv.Name)
	}
	if len(added) > 0 {
		roots := pathRoots(added)
		add(domain.Change{
			Category: domain.CategoryCRDSchema,
			Title:    fmt.Sprintf("%s %s schema: %s added: %s", label, fv.Name, plural(len(roots), "field", "fields"), codeList(roots, 3)),
			Detail:   fmt.Sprintf("New paths in the %s schema of %s:\n%s", gv, name, strings.Join(added, "\n")),
			Subjects: added,
		}, RuleCRDFieldsAdded, name, fv.Name)
	}
}

// diffSchemaAttributes diffs the per-path schema facts (default, enum,
// required, type) of paths present in both versions. One aggregated change per
// rule and CRD version, with the changed paths as subjects, so a CRD with many
// adjusted defaults is one line, not many. Identity wording mirrors the
// field-removed change ("<label> <version> schema: …", "in the <gv> schema of
// <name>") so consumers recover the GVK the same way.
func (b *builder) diffSchemaAttributes(add func(domain.Change, string, ...string), name, label, gv string, fv, tv domain.CRDVersionInfo) {
	if len(fv.Fields) == 0 || len(tv.Fields) == 0 {
		return
	}
	old := make(map[string]domain.CRDFieldSchema, len(fv.Fields))
	for _, f := range fv.Fields {
		old[f.Path] = f
	}
	var defaults, enums, required, types []string
	var defaultLines, enumLines, typeLines []string
	for _, n := range tv.Fields {
		o, ok := old[n.Path]
		if !ok {
			continue
		}
		if o.Default != n.Default {
			defaults = append(defaults, n.Path)
			defaultLines = append(defaultLines, fmt.Sprintf("%s: %s → %s", n.Path, orNone(o.Default), orNone(n.Default)))
		}
		if gone, added := diffStrings(o.Enum, n.Enum); len(gone)+len(added) > 0 && len(o.Enum)+len(n.Enum) > 0 {
			enums = append(enums, n.Path)
			enumLines = append(enumLines, fmt.Sprintf("%s: removed [%s], added [%s]", n.Path, strings.Join(gone, ", "), strings.Join(added, ", ")))
		}
		if n.Required && !o.Required {
			required = append(required, n.Path)
		}
		if o.Type != "" && n.Type != "" && o.Type != n.Type {
			types = append(types, n.Path)
			typeLines = append(typeLines, fmt.Sprintf("%s: %s → %s", n.Path, o.Type, n.Type))
		}
	}
	if len(defaults) > 0 {
		add(domain.Change{
			Category: domain.CategoryCRDSchema,
			Title:    fmt.Sprintf("%s %s schema: %s changed: %s", label, fv.Name, plural(len(defaults), "default", "defaults"), codeList(defaults, 3)),
			Detail: fmt.Sprintf("Schema defaults changed in the %s schema of %s (old → new; defaults apply to objects that leave the field unset):\n%s",
				gv, name, strings.Join(defaultLines, "\n")),
			Subjects: defaults,
		}, RuleCRDDefaultChanged, name, fv.Name)
	}
	if len(enums) > 0 {
		add(domain.Change{
			Category: domain.CategoryCRDSchema,
			Breaking: true,
			Title:    fmt.Sprintf("%s %s schema: allowed values changed: %s", label, fv.Name, codeList(enums, 3)),
			Detail: fmt.Sprintf("Enum values changed in the %s schema of %s; a removed value is rejected by the API server:\n%s",
				gv, name, strings.Join(enumLines, "\n")),
			Subjects: enums,
		}, RuleCRDEnumChanged, name, fv.Name)
	}
	if len(required) > 0 {
		add(domain.Change{
			Category: domain.CategoryCRDSchema,
			Breaking: true,
			Title:    fmt.Sprintf("%s %s schema: %s now required: %s", label, fv.Name, plural(len(required), "field", "fields"), codeList(required, 3)),
			Detail: fmt.Sprintf("Fields newly required in the %s schema of %s; objects that omit them are rejected:\n%s",
				gv, name, strings.Join(required, "\n")),
			Subjects: required,
		}, RuleCRDFieldRequired, name, fv.Name)
	}
	if len(types) > 0 {
		add(domain.Change{
			Category: domain.CategoryCRDSchema,
			Breaking: true,
			Title:    fmt.Sprintf("%s %s schema: %s changed type: %s", label, fv.Name, plural(len(types), "field", "fields"), codeList(types, 3)),
			Detail: fmt.Sprintf("Field types changed in the %s schema of %s:\n%s",
				gv, name, strings.Join(typeLines, "\n")),
			Subjects: types,
		}, RuleCRDFieldTypeChange, name, fv.Name)
	}
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
