package semvalidate

import (
	"context"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// crdValidator proves crd-field and gvk subjects and changes from the CRD
// snapshots of both releases, including the per-path default/enum/required/
// type facts the capture lane records (V3, V6).
type crdValidator struct{}

func (crdValidator) Name() string { return ProducerCRD }

// crdSide is the CRDs of one release, with the evidence of the snapshot that
// held each.
type crdSide struct {
	crds     []domain.CRDSummary
	evidence map[string][]domain.EvidenceID // CRD name → snapshot evidence
	pool     []domain.Evidence
	present  bool // at least one CRD snapshot
}

func crdsOf(r *domain.Release) crdSide {
	s := crdSide{evidence: map[string][]domain.EvidenceID{}, pool: releaseEvidence(r)}
	if r == nil {
		return s
	}
	for _, sn := range r.Snapshots {
		if sn.Kind != domain.SnapshotCRDs || sn.CRDs == nil {
			continue
		}
		s.present = true
		for _, c := range sn.CRDs.CRDs {
			s.crds = append(s.crds, c)
			s.evidence[c.Name] = append(s.evidence[c.Name], sn.Evidence...)
		}
	}
	return s
}

func (s crdSide) find(group, kind string) *domain.CRDSummary {
	for i := range s.crds {
		if s.crds[i].Group == group && s.crds[i].Kind == kind {
			return &s.crds[i]
		}
	}
	return nil
}

func (s crdSide) hasGroup(group string) bool {
	for _, c := range s.crds {
		if c.Group == group {
			return true
		}
	}
	return false
}

func (s crdSide) cite(ev *evidenceSet, c *domain.CRDSummary) {
	if c != nil {
		ev.add(resolve(s.evidence[c.Name], s.pool)...)
	}
}

func version(c *domain.CRDSummary, name string) *domain.CRDVersionInfo {
	if c == nil {
		return nil
	}
	for i := range c.Versions {
		if c.Versions[i].Name == name {
			return &c.Versions[i]
		}
	}
	return nil
}

func storage(c *domain.CRDSummary) string {
	if c == nil {
		return ""
	}
	for _, v := range c.Versions {
		if v.Storage {
			return v.Name
		}
	}
	return ""
}

// versionNames lists the version names to examine: the subject's version when
// stated, else the storage version of the target first, then every other
// version of either release.
func versionNames(subjVersion string, f, t *domain.CRDSummary) []string {
	if subjVersion != "" {
		return []string{subjVersion}
	}
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	add(storage(t))
	var rest []string
	for _, c := range []*domain.CRDSummary{t, f} {
		if c == nil {
			continue
		}
		for _, v := range c.Versions {
			if !seen[v.Name] {
				rest = append(rest, v.Name)
			}
		}
	}
	sort.Strings(rest)
	for _, n := range rest {
		add(n)
	}
	return out
}

// resolvePath finds the schema path in a version: exact first, then ignoring
// the "[]" list markers when that is unambiguous.
func resolvePath(v *domain.CRDVersionInfo, p string) (string, bool) {
	if v == nil {
		return "", false
	}
	for _, sp := range v.SchemaPaths {
		if sp == p {
			return sp, true
		}
	}
	strip := func(s string) string { return strings.ReplaceAll(s, "[]", "") }
	want, hit, n := strip(p), "", 0
	for _, sp := range v.SchemaPaths {
		if strip(sp) == want {
			hit, n = sp, n+1
		}
	}
	return hit, n == 1
}

func fieldOf(v *domain.CRDVersionInfo, path string) (domain.CRDFieldSchema, bool) {
	if v == nil {
		return domain.CRDFieldSchema{}, false
	}
	for _, f := range v.Fields {
		if f.Path == path {
			return f, true
		}
	}
	return domain.CRDFieldSchema{}, false
}

func (crdValidator) Validate(_ context.Context, in knowledge.ValidationInput) ([]domain.ValidationResult, error) {
	if !applies(in, domain.SubjectCRDField, domain.SubjectGVK) {
		return nil, nil
	}
	from, to := crdsOf(in.From), crdsOf(in.To)
	subj, chg := in.Assertion.Subject, in.Assertion.Change
	if !from.present || !to.present {
		v := inconclusive("crd-schema:snapshot", "no CRD snapshot on both sides")
		return build(in, ProducerCRD, map[domain.Aspect]verdict{domain.AspectSubject: v, domain.AspectChange: v}, evidenceSet{}), nil
	}
	fc, tc := from.find(subj.Group, subj.Kind), to.find(subj.Group, subj.Kind)
	var ev evidenceSet
	from.cite(&ev, fc)
	to.cite(&ev, tc)
	vs := map[domain.Aspect]verdict{}
	if subj.Family == domain.SubjectGVK {
		vs[domain.AspectSubject], vs[domain.AspectChange] = gvkVerdicts(from, to, fc, tc, subj, chg)
	} else {
		vs[domain.AspectSubject], vs[domain.AspectChange] = fieldVerdicts(from, to, fc, tc, subj, chg)
	}
	return build(in, ProducerCRD, vs, ev), nil
}

// --- gvk ---------------------------------------------------------------------------

func gvkVerdicts(from, to crdSide, fc, tc *domain.CRDSummary, subj *domain.Subject, chg *domain.ChangeSpec) (verdict, verdict) {
	fv, tv := version(fc, subj.Version), version(tc, subj.Version)
	id := subj.Group + "/" + subj.Version + " " + subj.Kind
	var sub verdict
	switch {
	case fv != nil || tv != nil:
		sub = confirmed("crd-schema:gvk", "%s is an API version in %s", id, sides(fv != nil, tv != nil))
	case fc != nil || tc != nil:
		sub = refuted("crd-schema:gvk", "%s %s has no version %s in either release", subj.Group, subj.Kind, subj.Version)
	default:
		// a CRD snapshot is not known to be the group's complete set of kinds
		// (some products generate CRDs at runtime), so a kind it lacks is a gap
		sub = inconclusive("crd-schema:gvk", "%s is not defined by any CRD of the snapshots (the snapshot may not hold every kind of the group)", id)
	}
	if sub.outcome == domain.OutcomeInconclusive {
		return sub, inconclusive("crd-schema:"+string(chg.Type), "the API version is not in the snapshots")
	}
	servedF, servedT := fv != nil && fv.Served, tv != nil && tv.Served
	var chv verdict
	switch chg.Type {
	case domain.ChangeKindRemoved:
		switch {
		case fv == nil:
			chv = refuted("crd-schema:version", "%s does not exist in the source release", id)
		case tv != nil && servedT:
			chv = refuted("crd-schema:version", "%s is still served by the target release", id)
		case tv != nil:
			chv = confirmed("crd-schema:version", "%s is still defined but no longer served", id)
		default:
			chv = confirmed("crd-schema:version", "%s is defined at the source and gone at the target", id)
		}
	case domain.ChangeKindAdded:
		switch {
		case fv != nil:
			chv = refuted("crd-schema:version", "%s already exists in the source release", id)
		case tv == nil:
			chv = refuted("crd-schema:version", "%s does not exist in the target release", id)
		default:
			chv = confirmed("crd-schema:version", "%s is new in the target release", id)
		}
	case domain.ChangeKindDeprecated:
		switch {
		case fv == nil || tv == nil:
			chv = refuted("crd-schema:deprecated", "%s must exist on both sides to be deprecated", id)
		case fv.Deprecated:
			chv = refuted("crd-schema:deprecated", "%s was already deprecated at the source", id)
		case !tv.Deprecated:
			chv = refuted("crd-schema:deprecated", "%s is not deprecated at the target", id)
		default:
			chv = confirmed("crd-schema:deprecated", "%s becomes deprecated", id)
		}
	case domain.ChangeKindValueChanged:
		// the storage version moves: before and after name versions
		fs, ts := storage(fc), storage(tc)
		switch {
		case fc == nil || tc == nil:
			chv = refuted("crd-schema:storage", "%s %s must exist on both sides", subj.Group, subj.Kind)
		case canonJSON(ptr(chg.Before)) != canonJSON(fs):
			chv = refuted("crd-schema:storage", "the source storage version is %q, not %s", fs, ptr(chg.Before))
		case canonJSON(ptr(chg.After)) != canonJSON(ts):
			chv = refuted("crd-schema:storage", "the target storage version is %q, not %s", ts, ptr(chg.After))
		default:
			chv = confirmed("crd-schema:storage", "storage version moves %s → %s", fs, ts)
		}
	default:
		chv = inconclusive("crd-schema:"+string(chg.Type), "a %s change of an API version is not decidable from CRD snapshots", chg.Type)
	}
	_ = servedF
	return sub, chv
}

// --- crd-field -----------------------------------------------------------------------

type fieldView struct {
	name     string
	fv, tv   *domain.CRDVersionInfo
	fp, tp   string // resolved schema paths ("" = absent)
	fOK, tOK bool
}

func fieldViews(subj *domain.Subject, fc, tc *domain.CRDSummary) []fieldView {
	var out []fieldView
	for _, n := range versionNames(subj.Version, fc, tc) {
		v := fieldView{name: n, fv: version(fc, n), tv: version(tc, n)}
		v.fp, v.fOK = resolvePath(v.fv, subj.Path)
		v.tp, v.tOK = resolvePath(v.tv, subj.Path)
		out = append(out, v)
	}
	return out
}

func fieldVerdicts(from, to crdSide, fc, tc *domain.CRDSummary, subj *domain.Subject, chg *domain.ChangeSpec) (verdict, verdict) {
	id := subj.Group + " " + subj.Kind + " " + subj.Path
	views := fieldViews(subj, fc, tc)
	anyF, anyT := false, false
	for _, v := range views {
		anyF, anyT = anyF || v.fOK, anyT || v.tOK
	}
	var sub verdict
	switch {
	case anyF || anyT:
		sub = confirmed("crd-schema:field", "%s is in the schema of %s", id, sides(anyF, anyT))
	case fc != nil || tc != nil:
		sub = refuted("crd-schema:field", "%s %s has no schema path %s in any version of either release", subj.Group, subj.Kind, subj.Path)
	default:
		// a CRD snapshot is not known to be the group's complete set of kinds
		// (some products generate CRDs at runtime), so a kind it lacks is a gap
		sub = inconclusive("crd-schema:field", "%s %s is not defined by any CRD of the snapshots (the snapshot may not hold every kind of the group)", subj.Group, subj.Kind)
	}
	if sub.outcome == domain.OutcomeInconclusive {
		return sub, inconclusive("crd-schema:"+string(chg.Type), "the subject is not in the snapshots")
	}
	return sub, fieldChange(views, id, subj, chg, anyF, anyT, fc, tc)
}

func fieldChange(views []fieldView, id string, subj *domain.Subject, chg *domain.ChangeSpec, anyF, anyT bool, fc, tc *domain.CRDSummary) verdict {
	// Attribute claims are about one schema: the subject's version, else the
	// target's storage version (the one stored objects and `kubectl` default to).
	var both *fieldView
	for i := range views {
		if views[i].fOK && views[i].tOK {
			if subj.Version == "" && views[i].name != storage(tc) {
				continue
			}
			both = &views[i]
			break
		}
	}
	switch chg.Type {
	case domain.ChangeKindRemoved:
		var dropped, kept []string
		for _, v := range views {
			switch {
			case v.fOK && v.tv != nil && !v.tOK:
				dropped = append(dropped, v.name)
			case v.fOK && v.tv == nil:
				dropped = append(dropped, v.name)
			case v.tOK:
				kept = append(kept, v.name)
			}
		}
		switch {
		case len(dropped) > 0 && len(kept) == 0:
			return confirmed("crd-schema:field-removed", "%s is in the %s schema at the source and gone at the target", id, strings.Join(dropped, ", "))
		case len(dropped) > 0:
			return inconclusive("crd-schema:field-removed", "%s is dropped from %s but still in the target schema of %s; name the version", id, strings.Join(dropped, ", "), strings.Join(kept, ", "))
		case !anyF:
			return refuted("crd-schema:field-removed", "%s was not in the source schema", id)
		}
		return refuted("crd-schema:field-removed", "%s is still in the target schema", id)
	case domain.ChangeKindAdded:
		var born []string
		for _, v := range views {
			if v.tOK && !v.fOK {
				if v.fv == nil {
					continue // the whole version is new: that is a new API version, not a new field
				}
				born = append(born, v.name)
			}
		}
		switch {
		case len(born) > 0 && subj.Version == "" && anyF:
			return inconclusive("crd-schema:field-added", "%s is new in %s but already in the source schema of another version; name the version", id, strings.Join(born, ", "))
		case len(born) > 0:
			return confirmed("crd-schema:field-added", "%s is new in the %s schema", id, strings.Join(born, ", "))
		case !anyT:
			return refuted("crd-schema:field-added", "%s is not in the target schema", id)
		}
		for _, v := range views {
			if v.tOK && v.fv == nil {
				return inconclusive("crd-schema:field-added", "%s appears with the new API version %s, not as a new field of an existing version", id, v.name)
			}
		}
		return refuted("crd-schema:field-added", "%s was already in the source schema", id)
	case domain.ChangeKindRenamed:
		newPath := chg.ReplacedBy.Path
		for _, v := range views {
			if !v.fOK || v.tOK {
				continue
			}
			if _, ok := resolvePath(v.fv, newPath); ok {
				continue // the replacement already existed
			}
			if _, ok := resolvePath(v.tv, newPath); ok {
				return confirmed("crd-schema:field-renamed", "%s disappears from %s and %s appears", subj.Path, v.name, newPath)
			}
		}
		// a move between API versions (the old path stays in the old version,
		// the new path exists in the new one) is real but not a per-schema rename
		oldKept, newOnly := "", ""
		for _, v := range views {
			if v.tOK && oldKept == "" {
				oldKept = v.name
			}
			if _, ok := resolvePath(v.tv, newPath); ok && !v.tOK {
				newOnly = v.name
			}
		}
		if oldKept != "" && newOnly != "" && oldKept != newOnly {
			return inconclusive("crd-schema:field-renamed", "%s stays in %s while %s appears in %s: a move between API versions, not a rename inside one schema", subj.Path, oldKept, newPath, newOnly)
		}
		return refuted("crd-schema:field-renamed", "no version drops %s while gaining %s", subj.Path, newPath)
	case domain.ChangeKindDefaultChanged, domain.ChangeKindNowRequired, domain.ChangeKindValidationTightened, domain.ChangeKindValueChanged:
		if both == nil {
			if subj.Version == "" && (anyF && anyT) {
				return inconclusive("crd-schema:"+string(chg.Type), "%s is not in the target storage version on both sides; name the version", id)
			}
			return refuted("crd-schema:"+string(chg.Type), "%s must exist on both sides for a %s change", id, chg.Type)
		}
		ff, fok := fieldOf(both.fv, both.fp)
		tf, tok := fieldOf(both.tv, both.tp)
		if !fok || !tok {
			return inconclusive("crd-schema:"+string(chg.Type), "a CRD snapshot carries no per-field schema facts (captured before the capture lane)")
		}
		return attributeChange(chg, id, both.name, ff, tf, fc)
	}
	return inconclusive("crd-schema:"+string(chg.Type), "a %s change is not decidable from CRD snapshots", chg.Type)
}

func attributeChange(chg *domain.ChangeSpec, id, ver string, ff, tf domain.CRDFieldSchema, fc *domain.CRDSummary) verdict {
	switch chg.Type {
	case domain.ChangeKindDefaultChanged:
		return defaultChange(chg, id, ver, ff, tf)
	case domain.ChangeKindNowRequired:
		switch {
		case ff.Required:
			return refuted("crd-schema:required", "%s was already required in %s", id, ver)
		case !tf.Required:
			return refuted("crd-schema:required", "%s is not required at the target in %s", id, ver)
		}
		return confirmed("crd-schema:required", "%s becomes required in the %s schema", id, ver)
	case domain.ChangeKindValidationTightened:
		var why []string
		if gone, _ := diffSets(ff.Enum, tf.Enum); len(gone) > 0 {
			why = append(why, "enum lost "+strings.Join(gone, ", "))
		}
		if !ff.Required && tf.Required {
			why = append(why, "now required")
		}
		if ff.Type != "" && tf.Type != "" && ff.Type != tf.Type {
			why = append(why, "type "+ff.Type+" → "+tf.Type)
		}
		if len(why) > 0 {
			return confirmed("crd-schema:tightened", "%s in %s: %s", id, ver, strings.Join(why, "; "))
		}
		return inconclusive("crd-schema:tightened", "enum, required and type of %s are unchanged; patterns and bounds are not captured", id)
	case domain.ChangeKindValueChanged:
		// an enum value renamed: before leaves the enum, after enters it
		gone, added := diffSets(ff.Enum, tf.Enum)
		if len(ff.Enum) == 0 && len(tf.Enum) == 0 {
			return inconclusive("crd-schema:enum", "%s has no enum in %s", id, ver)
		}
		before, after := canonJSON(ptr(chg.Before)), canonJSON(ptr(chg.After))
		if !contains(gone, before) || !contains(added, after) {
			if enumElsewhere(fc, before) {
				return inconclusive("crd-schema:enum", "%s is an allowed value of %s in another API version, but the %s enum does not change: a rename between versions, not inside one schema", ptr(chg.Before), id, ver)
			}
			if !contains(ff.Enum, before) {
				return refuted("crd-schema:enum", "%s is not an allowed value of %s at the source", ptr(chg.Before), id)
			}
			return refuted("crd-schema:enum", "the enum of %s does not drop %s and gain %s", id, ptr(chg.Before), ptr(chg.After))
		}
		return confirmed("crd-schema:enum", "enum of %s in %s: %s replaced by %s", id, ver, before, after)
	}
	return inconclusive("crd-schema:"+string(chg.Type), "not decidable")
}

// enumElsewhere reports whether any version of the source CRD allows the value.
func enumElsewhere(c *domain.CRDSummary, val string) bool {
	if c == nil {
		return false
	}
	for _, v := range c.Versions {
		for _, f := range v.Fields {
			if contains(f.Enum, val) {
				return true
			}
		}
	}
	return false
}

// defaultChange compares the asserted default before→after with the schema
// defaults. An asserted `null` means "no default": at the target that is
// proven by a schema that no longer states one (the schema default was
// removed); at the source it is not provable (a default may live in code).
func defaultChange(chg *domain.ChangeSpec, id, ver string, ff, tf domain.CRDFieldSchema) verdict {
	switch {
	case ff.Default != "" && !isNull(chg.Before) && !sameValue(ptr(chg.Before), ff.Default):
		return refuted("crd-schema:default", "the %s source default of %s is %s, not %s", ver, id, ff.Default, ptr(chg.Before))
	case tf.Default != "" && !isNull(chg.After) && !sameValue(ptr(chg.After), tf.Default):
		return refuted("crd-schema:default", "the %s target default of %s is %s, not %s", ver, id, tf.Default, ptr(chg.After))
	case ff.Default != "" && isNull(chg.Before):
		return refuted("crd-schema:default", "the %s source schema of %s states the default %s, not none", ver, id, ff.Default)
	case tf.Default != "" && isNull(chg.After):
		return refuted("crd-schema:default", "the %s target schema of %s states the default %s, not none", ver, id, tf.Default)
	case ff.Default == "" && tf.Default == "":
		return inconclusive("crd-schema:default", "the %s schema states no default at either release (it may be applied in code)", ver)
	case ff.Default == "" || (tf.Default == "" && !isNull(chg.After)):
		return inconclusive("crd-schema:default", "only one release's %s schema states a default of %s (%s → %s)", ver, id, orUnset(ff.Default), orUnset(tf.Default))
	}
	return confirmed("crd-schema:default", "the %s schema default of %s is %s at the source and %s at the target", ver, id, orUnset(ff.Default), orUnset(tf.Default))
}

func orUnset(s string) string {
	if s == "" {
		return "unset"
	}
	return s
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// diffSets returns the members only in a, and only in b.
func diffSets(a, b []string) (onlyA, onlyB []string) {
	for _, x := range a {
		if !contains(b, x) {
			onlyA = append(onlyA, x)
		}
	}
	for _, x := range b {
		if !contains(a, x) {
			onlyB = append(onlyB, x)
		}
	}
	return
}
