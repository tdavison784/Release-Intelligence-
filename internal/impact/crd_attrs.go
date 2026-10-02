package impact

// Joins for the schema-attribute diffs (crd:default-changed, crd:enum-changed,
// crd:field-required, crd:field-type-changed) and the storage-version move
// (crd:storage-changed). Like crd:fields-removed they are scoped to the API
// identity the change names (identityFromSchemaChange; an installed CRD of
// the same name completes it) and evaluated per resource of that GVK against
// the resource facts (env.Environment.Resources), never by bare paths.
//
// Classes (docs/IMPACT.md has the table):
//
//	default-changed  a resource leaves the field unset (its parent set) → review-required
//	                 (the new default applies to it); every resource pins it → informational;
//	                 no resource of the GVK reaches the field → not-affected
//	enum-changed     a resource uses a removed value → action-required (rejected)
//	field-required   a resource omits the newly required field → action-required (rejected)
//	field-type       a resource's value no longer fits the type → action-required; it sets the
//	                 field (compatible value) → review-required
//	storage-changed  the CRD is installed, or manifests use its kind → review-required
//	                 (stored objects stay at the old version until migrated); CRD not
//	                 installed → not-affected
//
// A kind or version the change does not pin makes the finding medium confidence,
// so action is demoted to review (another CRD of the group could serve the kind).
// Negative verdicts need healthy manifests (absence is not knowledge).

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// Join rules of the schema-attribute and storage-version diffs.
const (
	// A resource of the changed GVK leaves the field unset: the new schema
	// default applies to it.
	RuleCRDDefaultApplies = "impact:crd-default-applies"
	// Every resource that reaches the field sets it explicitly.
	RuleCRDDefaultPinned = "impact:crd-default-pinned"
	// A resource uses an enum value the target removes.
	RuleCRDEnumValueRemoved = "impact:crd-enum-value-removed"
	// A resource omits a field the target makes required.
	RuleCRDFieldNowRequired = "impact:crd-field-now-required"
	// A resource sets a field whose type changed (action when the value no
	// longer fits the new type).
	RuleCRDFieldTypeChanged = "impact:crd-field-type-changed"
	// The CRD whose storage version moves is installed (or its kind is used):
	// stored objects stay at the old version until migrated.
	RuleCRDStorageMigration = "impact:crd-storage-migration"
	// Not-affected verdict of the attribute rules: no resource of the GVK is
	// touched by the attribute change.
	RuleCRDAttributeClear = "impact:crd-attribute-clear"
)

// crdAttributeFamily joins the schema-attribute and storage diffs.
func (b *builder) crdAttributeFamily() {
	toTag := b.edge.To.String()
	for _, c := range b.edge.Changes {
		switch c.Provenance.Rule {
		case upgrade.RuleCRDDefaultChanged, upgrade.RuleCRDEnumChanged, upgrade.RuleCRDFieldRequired, upgrade.RuleCRDFieldTypeChange:
			if len(c.Subjects) == 0 {
				b.subjectLess(c)
				continue
			}
			b.crdAttribute(c, toTag)
		case upgrade.RuleCRDStorageChanged:
			if len(c.Subjects) == 0 {
				b.subjectLess(c)
				continue
			}
			b.crdStorageChanged(c, toTag)
		}
	}
}

// attrScope is the resolved API scope of an attribute change.
type attrScope struct {
	id        crdIdentity
	resources []*env.Resource
	conf      domain.Confidence
	label     string
}

// scopeOf resolves the GVK of an attribute change and the resources of it.
func (b *builder) scopeOf(c domain.Change) (attrScope, bool) {
	id, ok := identityFromSchemaChange(c)
	var installed *env.InstalledCRD
	if id.Name != "" {
		installed = crdByName(b.env, id.Name)
	}
	if !ok && installed != nil && installed.Group != "" {
		id.Group, ok = installed.Group, true
	}
	if !ok {
		return attrScope{}, false
	}
	if id.Kind == "" && installed != nil {
		id.Kind = installed.Kind
	}
	s := attrScope{id: id, conf: domain.ConfidenceHigh}
	sel := env.GVKSelector{Group: id.Group, Version: id.Version, Kind: id.Kind}
	if id.Kind == "" {
		sel.Kind = "*"
		s.conf = domain.ConfidenceMedium
	}
	if id.Version == "" {
		s.conf = domain.ConfidenceMedium
	}
	s.resources = b.env.Select(sel)
	s.label = id.Group
	if id.Version != "" {
		s.label += "/" + id.Version
	}
	if id.Kind != "" {
		s.label += " " + id.Kind
	}
	return s, true
}

// parentPath drops the last segment of a schema path ("spec.a[].b" → "spec.a[]").
func parentPath(p string) string {
	if i := strings.LastIndexByte(p, '.'); i > 0 {
		return p[:i]
	}
	return ""
}

// reach counts how often a resource reaches the field's parent and sets the
// field: "omitted" means the parent occurs (an object the schema would
// default/require the field in) more often than the field is set.
//
// strict (required): the parent must be stated — a required field binds only
// inside an existing parent object. Otherwise (defaults): an absent plain
// object parent still counts once, because defaulted parents are common and
// "not affected" must not rest on that guess; list elements must exist.
func reach(r *env.Resource, path string, strict bool) (parents, set int, facts []env.FieldFact) {
	facts = fieldFacts(r, path)
	set = len(facts)
	pp := parentPath(path)
	switch {
	case pp == "":
		parents = 1
	case len(fieldFacts(r, pp)) > 0:
		parents = len(fieldFacts(r, pp))
	case !strict && !strings.Contains(pp, "[]"):
		parents = 1
	}
	return parents, set, facts
}

var (
	enumLine = regexp.MustCompile(`^(\S+): removed \[(.*)\], added \[(.*)\]$`)
	typeLine = regexp.MustCompile(`^(\S+): (\S+) → (\S+)$`)
)

// detailLines maps "path: …" detail lines of an attribute change by path.
func detailLines(detail string, re *regexp.Regexp) map[string][]string {
	out := map[string][]string{}
	for _, ln := range strings.Split(detail, "\n") {
		if m := re.FindStringSubmatch(strings.TrimSpace(ln)); m != nil {
			out[m[1]] = m[2:]
		}
	}
	return out
}

func splitEnumList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ", ") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// jsonType names the JSON type of an encoded value.
func jsonType(v string) string {
	var x any
	if json.Unmarshal([]byte(v), &x) != nil {
		return ""
	}
	switch t := x.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		if t == float64(int64(t)) && !strings.ContainsAny(v, ".eE") {
			return "integer"
		}
		return "number"
	case nil:
		return "null"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return ""
}

// fitsType reports whether a value of JSON type got is valid for the schema type want.
func fitsType(got, want string) bool {
	switch {
	case got == want:
		return true
	case want == "number" && got == "integer":
		return true
	case got == "null":
		return true // nullable is a separate schema attribute; never claimed here
	}
	return false
}

func (b *builder) crdAttribute(c domain.Change, toTag string) {
	if !b.env.Supplied.Manifests {
		b.unknown(domain.UnknownEnvironmentVisibilityGap, RuleInsufficientVisibility, c.ID,
			fmt.Sprintf("Cannot tell whether the CRD schema change affects you: manifests were not supplied (%s)", c.Title),
			"Applicability of a schema attribute change is decided against the resources your manifests declare; without them the join cannot look.",
			c, c.Evidence, nil, "Kubernetes manifests (--manifests) not supplied")
		return
	}
	s, ok := b.scopeOf(c)
	if !ok {
		b.unknown(domain.UnknownSemanticAmbiguity, RuleNotJoined, c.ID,
			fmt.Sprintf("Cannot tell whether the CRD schema change affects you: the change does not identify the CRD (%s)", c.Title),
			"The changed schema paths carry no group/version/kind, so they cannot be scoped to the resources of one CRD.",
			c, c.Evidence, nil,
			fmt.Sprintf("the change does not state the CRD identity (group/version/kind) of the schema paths %s", codeList(c.Subjects, 3)))
		return
	}
	var enums, types map[string][]string
	switch c.Provenance.Rule {
	case upgrade.RuleCRDEnumChanged:
		enums = detailLines(c.Detail, enumLine)
	case upgrade.RuleCRDFieldTypeChange:
		types = detailLines(c.Detail, typeLine)
	}
	var hits, soft, pinned []domain.ImpactMatch
	var lines []string
	var withheld, unparsed []string
	for _, r := range s.resources {
		head := domain.ImpactMatch{Kind: domain.MatchAPIVersion, Subject: resourceLabel(r), Evidence: []domain.EvidenceID{r.Evidence}}
		var rHits, rSoft, rPinned []domain.ImpactMatch
		for _, p := range c.Subjects {
			parents, set, facts := reach(r, p, c.Provenance.Rule == upgrade.RuleCRDFieldRequired)
			switch c.Provenance.Rule {
			case upgrade.RuleCRDDefaultChanged, upgrade.RuleCRDFieldRequired:
				if parents > set {
					rHits = append(rHits, domain.ImpactMatch{Kind: domain.MatchAbsence, Subject: fmt.Sprintf("%s leaves %s unset", resourceLabel(r), p), Evidence: []domain.EvidenceID{r.Evidence}})
				} else if set > 0 {
					for _, f := range facts {
						rPinned = append(rPinned, domain.ImpactMatch{Kind: domain.MatchManifestField, Subject: resourceLabel(r) + ": " + f.Element, Evidence: f.Evidence})
					}
				}
			case upgrade.RuleCRDEnumChanged:
				parts, ok := enums[p]
				if !ok {
					unparsed = appendUnique(unparsed, p)
					continue
				}
				removed := splitEnumList(parts[0])
				for _, f := range facts {
					if f.Container {
						continue
					}
					if f.Withheld != "" {
						withheld = append(withheld, fmt.Sprintf("value withheld (%s) at %s", f.Withheld, resourceLabel(r)+": "+f.Element))
						continue
					}
					for _, v := range removed {
						if jsonEqual(v, f.Value) {
							rHits = append(rHits, domain.ImpactMatch{Kind: domain.MatchManifestField, Subject: fmt.Sprintf("%s: %s = %s", resourceLabel(r), f.Element, f.Value), Evidence: f.Evidence})
						}
					}
				}
			case upgrade.RuleCRDFieldTypeChange:
				parts, ok := types[p]
				if !ok {
					unparsed = appendUnique(unparsed, p)
					continue
				}
				for _, f := range facts {
					m := domain.ImpactMatch{Kind: domain.MatchManifestField, Subject: fmt.Sprintf("%s: %s", resourceLabel(r), f.Element), Evidence: f.Evidence}
					got := "object"
					switch {
					case f.Withheld != "":
						got = ""
					case !f.Container:
						got = jsonType(f.Value)
					case strings.HasSuffix(f.Path, "[]") || len(fieldFacts(r, f.Path+"[]")) > 0:
						got = "array"
					}
					if got != "" && !fitsType(got, parts[1]) {
						m.Subject += fmt.Sprintf(" (%s, the schema now wants %s)", got, parts[1])
						rHits = append(rHits, m)
					} else {
						rSoft = append(rSoft, m)
					}
				}
			}
		}
		if len(rHits)+len(rSoft)+len(rPinned) == 0 {
			continue
		}
		if len(rHits) > 0 {
			hits = append(append(hits, head), rHits...)
		}
		if len(rSoft) > 0 {
			soft = append(append(soft, head), rSoft...)
		}
		if len(rPinned) > 0 {
			pinned = append(append(pinned, head), rPinned...)
		}
		lines = append(lines, "Environment: "+resourceLabel(r)+" ("+r.Doc.File+fmt.Sprintf(":L%d", r.Doc.StartLine)+")")
	}
	paths := codeList(c.Subjects, 3)
	envLines := strings.Join(lines, "\n")
	switch c.Provenance.Rule {
	case upgrade.RuleCRDDefaultChanged:
		if len(hits) > 0 {
			b.add(RuleCRDDefaultApplies, domain.ImpactReviewRequired, domain.SeverityMedium, s.conf,
				fmt.Sprintf("The new schema default of %s in %s applies to your resources that leave it unset", paths, toTag),
				fmt.Sprintf("%s changes the default of %s in the %s schema; resources that leave the field unset take the new default on their next write. Verify the new behaviour, or pin the old value explicitly.\n%s\n%s",
					toTag, paths, s.label, strings.TrimSpace(c.Detail), envLines),
				c, hits, c.Evidence...)
		}
		if len(pinned) > 0 {
			b.add(RuleCRDDefaultPinned, domain.ImpactInformational, domain.SeverityLow, s.conf,
				fmt.Sprintf("You set %s explicitly, so its new schema default in %s does not apply", paths, toTag),
				fmt.Sprintf("Your resources set the field themselves; the API server keeps your value and the default change does not reach them.\n%s", envLines),
				c, pinned, c.Evidence...)
		}
		if len(hits)+len(pinned) > 0 {
			return
		}
	case upgrade.RuleCRDEnumChanged:
		if len(hits) > 0 {
			b.add(RuleCRDEnumValueRemoved, domain.ImpactActionRequired, domain.SeverityHigh, s.conf,
				fmt.Sprintf("Your resources use a value of %s that %s no longer allows", paths, toTag),
				fmt.Sprintf("The %s schema of %s removes allowed values; the API server rejects resources that still use them. Change them to an allowed value before upgrading.\n%s\n%s",
					s.label, toTag, strings.TrimSpace(c.Detail), envLines),
				c, hits, c.Evidence...)
			return
		}
	case upgrade.RuleCRDFieldRequired:
		if len(hits) > 0 {
			b.add(RuleCRDFieldNowRequired, domain.ImpactActionRequired, domain.SeverityHigh, s.conf,
				fmt.Sprintf("Your resources omit %s, which %s makes required", paths, toTag),
				fmt.Sprintf("The %s schema of %s requires the field; the API server rejects resources that omit it on their next write. Set it before upgrading.\n%s", s.label, toTag, envLines),
				c, hits, c.Evidence...)
			return
		}
	case upgrade.RuleCRDFieldTypeChange:
		if len(hits) > 0 {
			b.add(RuleCRDFieldTypeChanged, domain.ImpactActionRequired, domain.SeverityHigh, s.conf,
				fmt.Sprintf("Your resources set %s to a value that no longer fits its type in %s", paths, toTag),
				fmt.Sprintf("The %s schema of %s changes the field's type; values of the old type are rejected.\n%s\n%s", s.label, toTag, strings.TrimSpace(c.Detail), envLines),
				c, hits, c.Evidence...)
		}
		if len(soft) > 0 {
			b.add(RuleCRDFieldTypeChanged, domain.ImpactReviewRequired, domain.SeverityMedium, s.conf,
				fmt.Sprintf("You set %s, whose type changes in %s", paths, toTag),
				fmt.Sprintf("Your value fits the new type (or could not be checked); verify it against the new schema.\n%s\n%s", strings.TrimSpace(c.Detail), envLines),
				c, soft, c.Evidence...)
		}
		if len(hits)+len(soft) > 0 {
			return
		}
	}
	// nothing matched: unknown when a value or the change itself could not be
	// read, not-affected only against healthy manifests
	switch {
	case len(withheld) > 0:
		b.unknown(domain.UnknownEnvironmentVisibilityGap, RuleInsufficientVisibility, c.ID,
			fmt.Sprintf("Cannot tell whether your resources use a removed value of %s", paths),
			"Some values of the field are withheld (sensitive), so they cannot be compared with the removed values.",
			c, c.Evidence, nil, withheld...)
	case len(unparsed) > 0:
		b.unknown(domain.UnknownEvidenceGap, RuleNotJoined, c.ID,
			fmt.Sprintf("Cannot tell whether %s affects you: the change does not state its values machine-readably", c.Title),
			"The change's detail does not list the old/new values per path, so resource values cannot be compared with them.",
			c, c.Evidence, nil, fmt.Sprintf("machine-readable old/new values for %s", codeList(unparsed, 3)))
	case b.env.Health(env.DimManifests) != env.HealthOK:
		b.unknown(domain.UnknownEnvironmentVisibilityGap, RuleInsufficientVisibility, c.ID,
			fmt.Sprintf("Cannot rule out that %s affects you: manifests were only partially parsed", c.Title),
			"No parsed resource is affected, but some manifests could not be parsed; an affected resource may be among them.",
			c, c.Evidence, nil, "Kubernetes manifests were only partially parsed")
	default:
		b.verdict(RuleCRDAttributeClear, domain.ImpactNotAffected, c.ID,
			fmt.Sprintf("No resource of %s is affected by %s", code(s.label), c.Title),
			fmt.Sprintf("Checked %d resource(s) of %s against the changed schema paths %s; none reaches a changed attribute.", len(s.resources), s.label, paths),
			c, c.Evidence, []domain.ImpactCheck{{Dimension: domain.DimensionManifests, Facts: len(s.resources), Subjects: c.Subjects}})
	}
}

var storageTitle = regexp.MustCompile("^Storage version of `([^`]+)` changes (\\S+) → (\\S+)$")
var storageLabel = regexp.MustCompile(`^Existing (\S+) objects stay stored`)

// crdStorageChanged joins crd:storage-changed: the CRD's storage version
// moves, so objects stored at the old version stay there until rewritten (a
// storage version migration), and a later release that stops serving the old
// version breaks them. Exposure: the CRD is installed, or manifests use its
// kind (any version). Not-affected: installed CRDs were supplied (healthy)
// and do not include it, and no manifest uses the kind.
func (b *builder) crdStorageChanged(c domain.Change, toTag string) {
	name := c.Subjects[0]
	_, group, ok := parseCRDName(name)
	m := storageTitle.FindStringSubmatch(c.Title)
	if !ok || m == nil {
		b.unknown(domain.UnknownSemanticAmbiguity, RuleNotJoined, c.ID,
			fmt.Sprintf("Cannot tell whether the storage-version change affects you: %s", c.Title),
			"The change does not name a CRD and its old/new storage versions in the form the join reads.",
			c, c.Evidence, nil, fmt.Sprintf("the CRD name and storage versions of %q", c.Title))
		return
	}
	oldV, newV := m[2], m[3]
	installed := crdByName(b.env, name)
	kind := ""
	if installed != nil {
		kind = installed.Kind
	} else if lm := storageLabel.FindStringSubmatch(c.Detail); lm != nil && !strings.Contains(lm[1], ".") {
		kind = lm[1]
	}
	var matches []domain.ImpactMatch
	if installed != nil {
		matches = append(matches, domain.ImpactMatch{Kind: domain.MatchCRD, Subject: name, Evidence: installed.Evidence})
	}
	conf := domain.ConfidenceHigh
	if kind == "" {
		conf = domain.ConfidenceMedium
	}
	uses := b.gvkUses(func(u env.GVKUsage) bool { return u.Group == group && (kind == "" || u.Kind == kind) })
	matches = append(matches, gvkMatches(uses)...)
	if len(matches) > 0 {
		g := newGVKLines(b, name, installed)
		detail := fmt.Sprintf("%s stores %s objects as %s instead of %s. Objects already stored stay at %s until they are rewritten: run a storage version migration (re-write every object) and remove %s from status.storedVersions before a release stops serving %s.",
			toTag, firstNonEmpty(kind, name), newV, oldV, oldV, oldV, oldV)
		if installed != nil {
			detail += fmt.Sprintf("\nYour installed CRD %s may hold objects stored at %s.", name, oldV)
		}
		if len(uses) > 0 {
			detail += "\n" + strings.Join(g.lines(uses), "\n")
		}
		b.add(RuleCRDStorageMigration, domain.ImpactReviewRequired, domain.SeverityMedium, conf,
			fmt.Sprintf("Storage version of %s moves %s → %s: migrate stored objects", code(name), oldV, newV), detail, c, matches, c.Evidence...)
		return
	}
	switch {
	case b.env.Supplied.CRDs && b.env.Health(env.DimCRDs) == env.HealthOK && (!b.env.Supplied.Manifests || b.env.Health(env.DimManifests) == env.HealthOK):
		checks := []domain.ImpactCheck{b.crdsCheck([]string{name})}
		if b.env.Supplied.Manifests {
			checks = append(checks, b.apiVersionsCheck([]string{name}))
		}
		b.verdict(RuleCRDUnused, domain.ImpactNotAffected, c.ID,
			fmt.Sprintf("The %s CRD whose storage version moves is not installed", code(name)),
			fmt.Sprintf("No installed CRD of your environment is %s, so no objects of it are stored and nothing needs migrating. Checked %d installed CRD(s).", name, len(b.env.CRDs)),
			c, c.Evidence, checks)
	default:
		checks := b.partialChecks([]domain.ImpactCheck{b.apiVersionsCheck([]string{name})}, b.env.Supplied.Manifests)
		b.unknown(domain.UnknownEnvironmentVisibilityGap, RuleInsufficientVisibility, c.ID,
			fmt.Sprintf("Cannot tell whether the storage-version change affects you: installed CRDs were not supplied (%s)", c.Title),
			"Objects stored in the cluster are only visible through the installed CRDs; manifests that do not use the kind do not prove that no objects are stored.",
			c, c.Evidence, checks, "installed CustomResourceDefinitions (--crds) not supplied")
	}
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}
