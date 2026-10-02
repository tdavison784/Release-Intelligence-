package impact

// GVK-scoped CRD matching (docs/IMPACT.md, section "CRDs and API versions").
//
// An upstream CRD/schema change is pinned to an API identity — CRD name,
// group, version, kind — and matched against the environment's GVK usage
// inventory (env.Environment.GVKUsage), never against bare field paths: a
// Deployment setting `spec.foo` must not turn a removed
// `cert-manager.io/v1 Certificate spec.foo` into a CRD impact.
//
// The confidence ladder (docs/IMPACT.md has the table):
//
//	exact GVK (+ exact field)     → high-confidence impact; action-required
//	                                when the upstream change is a removal
//	known CRD + compatible kind   → high (the CRD name matches an installed
//	                                CRD whose names.kind the manifests use)
//	                                or medium (kind only inferred); medium is
//	                                demoted to review-required by the contract
//	same API group (/version)     → review-required at most, never action:
//	                                another CRD of the group could serve the
//	                                kind in use
//	field path only               → insufficient evidence → unknown
//	                                (neededToDetermine)
//	no match, dimensions supplied → not-affected with the evaluation record
//
// Where the identity comes from — the differ's own deterministic output
// (internal/upgrade/crds.go), parsed, never guessed:
//
//	crd:removed         subject is the CRD name ("certificates.cert-manager.io")
//	crd:version-*       subject is "name/version"
//	crd:fields-removed  subjects are bare schema paths; the identity lives in
//	                    the change title ("<label> <version> schema: N field(s)
//	                    removed: …", the label being the CRD's kind when the
//	                    snapshot knows it, else its name) and detail ("… in
//	                    the cert-manager.io/v1alpha2 schema of
//	                    certificates.cert-manager.io are pruned …"). A change
//	                    whose title/detail yields no API group is UNKNOWN
//	                    (neededToDetermine), never path-matched.
//
// "Manifest usage" is read from the inventory: an installed CRD document
// stages a GVKUsage entry for every version it serves (its only resource name
// is the CRD itself) but sets no field paths, while every resource manifest
// sets at least the apiVersion/kind/metadata leaves — a non-empty FieldPaths
// list therefore marks manifest-backed usage, and "the CRD is installed"
// never masquerades as "a resource of this kind exists".

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// --- upstream identity ----------------------------------------------------------

// crdIdentity is the API identity an upstream CRD/schema change targets, as
// far as the change states it. Empty fields are unknown, never guessed.
type crdIdentity struct {
	Name    string // CRD resource name ("certificates.cert-manager.io")
	Group   string // API group ("cert-manager.io"); the scoping minimum
	Version string // API version ("v1")
	Kind    string // resource kind ("Certificate")
}

// parseCRDName parses a crd:removed subject: a CRD resource name whose API
// group is the first dot-suffix ("certificates.cert-manager.io" →
// "cert-manager.io"). A token without a dot has no group and cannot scope a
// GVK match.
func parseCRDName(s string) (name, group string, ok bool) {
	s = strings.TrimSpace(s)
	i := strings.IndexByte(s, '.')
	if s == "" || strings.ContainsAny(s, "/ \t") || i <= 0 || i == len(s)-1 {
		return "", "", false
	}
	return s, s[i+1:], true
}

// parseCRDNameVersion parses a crd:version-* subject ("name/version").
func parseCRDNameVersion(s string) (crdIdentity, bool) {
	s = strings.TrimSpace(s)
	i := strings.LastIndexByte(s, '/')
	if i <= 0 || i == len(s)-1 {
		return crdIdentity{}, false
	}
	name, group, ok := parseCRDName(s[:i])
	version := s[i+1:]
	if !ok || strings.ContainsAny(version, "/ \t") {
		return crdIdentity{}, false
	}
	return crdIdentity{Name: name, Group: group, Version: version}, true
}

// plausibleVersion guards the title parse against prose: a CRD version is a
// short DNS-label-like token without whitespace, slashes or colons.
func plausibleVersion(s string) bool {
	return s != "" && len(s) <= 63 && !strings.ContainsAny(s, " \t/:")
}

// identityFromSchemaChange recovers the CRD identity of a crd:fields-removed
// change from the differ's deterministic title and detail (the subjects are
// bare schema paths). ok is false when neither yields an API group: the paths
// then cannot be scoped to a GVK and the change is unknown rather than
// path-matched.
func identityFromSchemaChange(c domain.Change) (id crdIdentity, ok bool) {
	// Detail: "Fields no longer in the <group>/<version> schema of <name>
	// are pruned from stored objects and rejected or dropped in manifests. …"
	// — and the schema-attribute rules' "… changed in the <gv> schema of
	// <name> (old → new; …", "…; a removed value …", ":\n…": the name is the
	// token after " schema of ", ended by a space, ';', ':', '(' or newline.
	if i := strings.Index(c.Detail, " in the "); i >= 0 {
		rest := c.Detail[i+len(" in the "):]
		if j := strings.Index(rest, " schema of "); j >= 0 && !strings.ContainsAny(rest[:j], "\n") {
			gv := strings.TrimSpace(rest[:j])
			tail := rest[j+len(" schema of "):]
			end := strings.IndexAny(tail, " ;:(\n\t")
			if end < 0 {
				end = len(tail)
			}
			if name := strings.TrimRight(tail[:end], "."); name != "" && !strings.ContainsAny(gv, " \t") && !strings.Contains(name, "/") {
				group, version, hasGroup := strings.Cut(gv, "/")
				if !hasGroup {
					version, group = gv, "" // bare-version form; the name decides the group below
				}
				id.Name, id.Group, id.Version = name, group, version
			}
		}
	}
	// Title: "<label> <version> schema: N field(s) removed: …" — the label is
	// the kind when the snapshot knows it, else the CRD name.
	if i := strings.Index(c.Title, " schema: "); i > 0 {
		head := c.Title[:i]
		if sp := strings.LastIndexByte(head, ' '); sp > 0 {
			label, version := head[:sp], head[sp+1:]
			if plausibleVersion(version) {
				if id.Version == "" {
					id.Version = version
				}
				if strings.Contains(label, ".") {
					if id.Name == "" {
						id.Name = label
					}
				} else if id.Kind == "" {
					id.Kind = label
				}
			}
		}
	}
	if id.Name != "" && id.Group == "" {
		id.Group = groupOfCRDName(id.Name)
	}
	return id, id.Group != ""
}

// groupOfCRDName derives the API group from a CRD resource name
// ("certificates.cert-manager.io" → "cert-manager.io").
func groupOfCRDName(name string) string {
	if i := strings.IndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return ""
}

// --- the environment side: the GVK usage inventory ------------------------------

// crdByName returns the installed CRD with exactly this resource name.
func crdByName(e *env.Environment, name string) *env.InstalledCRD {
	for i := range e.CRDs {
		if e.CRDs[i].Name == name {
			return &e.CRDs[i]
		}
	}
	return nil
}

// installedDeclares reports whether the installed CRD declares the version.
func installedDeclares(crd *env.InstalledCRD, version string) bool {
	if crd == nil {
		return false
	}
	for _, v := range crd.Versions {
		if v.Name == version {
			return true
		}
	}
	return false
}

// groupVersionOf joins a GVK usage entry back into its manifest apiVersion
// form ("cert-manager.io/v1"; the core group is the bare version, "v1").
func groupVersionOf(u env.GVKUsage) string {
	if u.Group == "" {
		return u.Version
	}
	return u.Group + "/" + u.Version
}

// gvkUses returns the GVK usage entries that resource manifests back and that
// satisfy keep (see the file comment for the FieldPaths marker).
func (b *builder) gvkUses(keep func(env.GVKUsage) bool) []env.GVKUsage {
	var out []env.GVKUsage
	for _, u := range b.env.GVKUsage {
		if len(u.FieldPaths) == 0 {
			continue
		}
		if keep(u) {
			out = append(out, u)
		}
	}
	return out
}

// gvkMatches turns GVK usage entries into api-version matches citing the
// inventory evidence (one id per using document).
func gvkMatches(uses []env.GVKUsage) []domain.ImpactMatch {
	var out []domain.ImpactMatch
	for _, u := range uses {
		out = appendUniqueMatches(out, domain.ImpactMatch{
			Kind: domain.MatchAPIVersion, Subject: groupVersionOf(u) + " " + u.Kind, Evidence: u.Evidence,
		})
	}
	return out
}

// evIndex resolves environment evidence ids to their records (file + locator),
// so a why-block can tell the CRD definition document apart from the resource
// manifests that share a GVK usage entry with it.
type evIndex map[domain.EvidenceID]domain.Evidence

func envEvidenceIndex(e *env.Environment) evIndex {
	ix := make(evIndex, len(e.Evidence))
	for _, ev := range e.Evidence {
		ix[ev.ID] = ev
	}
	return ix
}

// gvkLines renders why-block lines for matched GVK usage entries. It knows the
// changed CRD's own staged identity — its resource name and its definition
// document — so the phrase names resource manifests, never the CRD definition
// that merely stages the served version in the same inventory entry.
type gvkLines struct {
	ix      evIndex
	crdName string                   // the changed CRD's resource name ("" when unknown)
	crdDocs map[env.DocumentRef]bool // the CRD definition document(s)
}

func newGVKLines(b *builder, crdName string, crd *env.InstalledCRD) gvkLines {
	g := gvkLines{ix: envEvidenceIndex(b.env), crdName: crdName, crdDocs: map[env.DocumentRef]bool{}}
	if crd == nil {
		return g
	}
	for _, id := range crd.Evidence {
		if e, ok := g.ix[id]; ok && e.Kind == domain.EvidenceLocalFile {
			line := 0
			fmt.Sscanf(e.Locator, "L%d", &line)
			g.crdDocs[env.DocumentRef{File: e.URI, StartLine: line}] = true
		}
	}
	return g
}

// gvkNamesPhrase renders the resource identities of a usage entry
// ("example-com", "istio-system/example-com", "example-com +2 more"), skipping
// the CRD's own staged name.
func (g gvkLines) names(u env.GVKUsage) string {
	var out []string
	for _, n := range u.Names {
		if n.Name == "" || (n.Namespace == "" && g.crdName != "" && n.Name == g.crdName) {
			continue
		}
		if n.Namespace != "" {
			out = append(out, n.Namespace+"/"+n.Name)
		} else {
			out = append(out, n.Name)
		}
	}
	if len(out) == 0 {
		return ""
	}
	if len(out) > 3 {
		return out[0] + " +" + fmt.Sprint(len(out)-1) + " more"
	}
	return strings.Join(out, ", ")
}

// manifestRef renders the first resource-manifest document of the entry
// ("manifests/certificate.yaml:L1") — the raw document to re-inspect.
func (g gvkLines) manifestRef(u env.GVKUsage) string {
	for _, d := range u.Documents {
		if g.crdDocs[d] {
			continue
		}
		return fmt.Sprintf("%s:L%d", d.File, d.StartLine)
	}
	return ""
}

// line is one why-block line naming the matched resource the way
// docs/IMPACT.md promises: "Environment: Certificate/example-com
// (manifests/certificate.yaml:L1) sets spec.secretName (L7)".
func (g gvkLines) line(u env.GVKUsage, sets string) string {
	s := "Environment: " + u.Kind
	if n := g.names(u); n != "" {
		s += "/" + n
	}
	if d := g.manifestRef(u); d != "" {
		s += " (" + d + ")"
	}
	if sets != "" {
		s += " sets " + sets
	}
	return s
}

// lines renders one line per matched GVK usage entry.
func (g gvkLines) lines(uses []env.GVKUsage) []string {
	out := make([]string, 0, len(uses))
	for _, u := range uses {
		out = append(out, g.line(u, ""))
	}
	return out
}

// appendUniqueField appends f unless the same field fact (path, line,
// evidence) is already listed: several removed sub-paths can relate to one set
// field (a set list leaf `spec.resources` relates to every removed
// `spec.resources[].…` path), and the why-block must list it once.
func appendUniqueField(fs []env.ManifestField, f env.ManifestField) []env.ManifestField {
	for _, x := range fs {
		if x.Path == f.Path && x.Line == f.Line && sameEvidence(x.Evidence, f.Evidence) {
			return fs
		}
	}
	return append(fs, f)
}

func sameEvidence(a, b []domain.EvidenceID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// usageSettingFields narrows a GVK usage entry to the documents that contain
// the given field facts, so a why-block names the resources that actually set
// the matched fields, not every resource of the GVK. A field belongs to the
// resource document of its file with the greatest start line at or before the
// field's line. When no field can be placed (no line or file evidence), the
// entry is returned unchanged.
func usageSettingFields(e *env.Environment, ix evIndex, u env.GVKUsage, fs []env.ManifestField) env.GVKUsage {
	docs := map[env.DocumentRef]bool{}
	names := map[env.ResourceName]bool{}
	for _, f := range fs {
		file := ""
		for _, id := range f.Evidence {
			if ev, ok := ix[id]; ok && ev.Kind == domain.EvidenceLocalFile {
				file = ev.URI
				break
			}
		}
		if file == "" || f.Line <= 0 {
			return u
		}
		var owner *env.Resource
		for i := range e.Resources {
			r := &e.Resources[i]
			if r.Doc.File != file || r.Doc.StartLine > f.Line {
				continue
			}
			if owner == nil || r.Doc.StartLine > owner.Doc.StartLine {
				owner = r
			}
		}
		if owner == nil {
			return u
		}
		docs[owner.Doc] = true
		names[env.ResourceName{Name: owner.Name, Namespace: owner.Namespace}] = true
	}
	out := u
	out.Names, out.Documents = nil, nil
	for _, n := range u.Names {
		if names[n] {
			out.Names = append(out.Names, n)
		}
	}
	for _, d := range u.Documents {
		if docs[d] {
			out.Documents = append(out.Documents, d)
		}
	}
	if len(out.Documents) == 0 {
		return u
	}
	return out
}

// fieldSetsPhrase renders the matched field paths of one GVK with their lines
// ("spec.secretName (L7), spec.dnsNames (L9)"), capped like the titles.
func fieldSetsPhrase(fs []env.ManifestField) string {
	var parts []string
	for i, f := range fs {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("+%d more", len(fs)-3))
			break
		}
		if f.Line > 0 {
			parts = append(parts, fmt.Sprintf("%s (L%d)", f.Path, f.Line))
		} else {
			parts = append(parts, f.Path)
		}
	}
	return strings.Join(parts, ", ")
}

func apiVersionStrings(uses []env.GVKUsage) []string {
	out := make([]string, 0, len(uses))
	for _, u := range uses {
		out = append(out, groupVersionOf(u)+" ("+u.Kind+")")
	}
	return out
}

// --- rule 2: CRDs and API versions ----------------------------------------------

func (b *builder) crdFamily() {
	toTag := b.edge.To.String()
	for _, c := range b.edge.Changes {
		if len(c.Subjects) == 0 {
			switch c.Provenance.Rule {
			case upgrade.RuleCRDRemoved, upgrade.RuleCRDVersionRemoved, upgrade.RuleCRDVersionUnserved,
				upgrade.RuleCRDVersionDeprecated, upgrade.RuleCRDFieldsRemoved:
				b.subjectLess(c)
			}
			continue
		}
		switch c.Provenance.Rule {
		case upgrade.RuleCRDRemoved:
			b.crdRemoved(c, toTag)
		case upgrade.RuleCRDVersionRemoved, upgrade.RuleCRDVersionUnserved:
			b.crdVersionGone(c, toTag)
		case upgrade.RuleCRDVersionDeprecated:
			b.crdVersionDeprecated(c, toTag)
		case upgrade.RuleCRDFieldsRemoved:
			b.crdFieldsRemoved(c, toTag)
		}
	}
}

// partialChecks appends a check only when its dimension was actually visible;
// an unknown verdict keeps the record of what COULD be checked.
func (b *builder) partialChecks(checks []domain.ImpactCheck, visible bool) []domain.ImpactCheck {
	if !visible && len(checks) > 0 {
		return nil
	}
	return checks
}

func pluralIs(subjects []string) string {
	if len(subjects) == 1 {
		return "is"
	}
	return "are"
}

// crdRemoved joins a crd:removed change. Ladder: installed CRD + manifest
// usage of its kind → action (high); group-only usage (kind not pinned) →
// review at most (medium, demoted); installed but no manifest of the kind →
// not-affected (nothing supplied stops being reconciled) or, without
// manifests, review; neither installed nor used → not-affected; a subject
// that does not even name a CRD → unknown.
func (b *builder) crdRemoved(c domain.Change, toTag string) {
	// deciding dimension: installed CRDs (authoritative for what the cluster
	// runs). Manifests refine the verdict; both are recorded.
	if !b.env.Supplied.CRDs {
		checks := b.partialChecks([]domain.ImpactCheck{b.apiVersionsCheck(c.Subjects)}, b.env.Supplied.Manifests)
		b.unknown(domain.UnknownEnvironmentVisibilityGap, RuleInsufficientVisibility, c.ID,
			fmt.Sprintf("Cannot tell whether the removed CRD affects you: installed CRDs were not supplied (%s)", c.Title),
			"Applicability of a CRD removal is decided against the CustomResourceDefinitions your cluster actually has installed; without them the join cannot look.",
			c, c.Evidence, checks, "installed CustomResourceDefinitions (--crds) not supplied")
		return
	}
	var unmatched []string
	parsed := 0
	for _, s := range c.Subjects {
		installed := crdByName(b.env, s)
		name, group, ok := parseCRDName(s)
		if installed != nil {
			// the exact name match pins the identity even when the raw
			// subject is not a well-formed name; the installed CRD supplies
			// group and kind.
			name, ok = s, true
			if group == "" {
				group = installed.Group
			}
		}
		if installed == nil && !ok {
			b.unknown(domain.UnknownSemanticAmbiguity, RuleNotJoined, c.ID+":"+s,
				fmt.Sprintf("Cannot tell whether the removed CRD affects you: %q does not name a CustomResourceDefinition", s),
				"The subject carries no CRD name with an API group, so the join can neither check the installed CRDs nor scope manifest usage to a group/version/kind.",
				c, c.Evidence, nil,
				fmt.Sprintf("the removed-CRD subject %q does not identify a CRD (name/group)", s))
			continue
		}
		parsed++
		kind := ""
		if installed != nil {
			kind = installed.Kind
		}
		g := newGVKLines(b, name, installed)
		uses := b.gvkUses(func(u env.GVKUsage) bool {
			return u.Group == group && (kind == "" || u.Kind == kind)
		})
		switch {
		case len(uses) > 0 && installed != nil:
			// known CRD + compatible kind: the name match pins the identity,
			// the manifests use its kind (any version — a removal drops all).
			conf := domain.ConfidenceHigh
			if kind == "" {
				conf = domain.ConfidenceMedium // the installed CRD states no kind; "compatible kind" is unproven
			}
			matches := []domain.ImpactMatch{{Kind: domain.MatchCRD, Subject: name, Evidence: installed.Evidence}}
			matches = append(matches, gvkMatches(uses)...)
			where := fmt.Sprintf("%s is installed and manifests use %s", installed.Name, strings.Join(apiVersionStrings(uses), ", "))
			detail := fmt.Sprintf("%s no longer ships this CustomResourceDefinition; existing resources stop being reconciled. %s. Migrate or delete the resources before upgrading.\n%s",
				toTag, where, strings.Join(g.lines(uses), "\n"))
			b.add(RuleCRDRemoved, domain.ImpactActionRequired, domain.SeverityCritical, conf,
				fmt.Sprintf("You use the %s CRD that %s removes", code(s), toTag), detail, c, matches, c.Evidence...)
		case len(uses) > 0:
			// same API group only: the change does not pin the kind, and
			// another CRD of the group could serve the kind in use — review at
			// most (medium confidence is demoted by the contract).
			detail := fmt.Sprintf("%s no longer ships a CustomResourceDefinition of the API group %s, which your manifests use — but the change does not pin the kind, and another CRD of the same group could serve it. To decide, the installed CRDs must contain %s (its names.kind then checks against your manifests).\n%s",
				toTag, code(group), code(s), strings.Join(g.lines(uses), "\n"))
			b.add(RuleCRDRemoved, domain.ImpactReviewRequired, domain.SeverityCritical, domain.ConfidenceMedium,
				fmt.Sprintf("Manifests use the API group of the removed CRD %s, kind unconfirmed", code(s)),
				detail, c, gvkMatches(uses), c.Evidence...)
		case installed != nil && b.env.Supplied.Manifests:
			// the CRD is installed, but no manifest declares a resource of the
			// kind: checked against both supplied dimensions and clear.
			what := "kind " + kind
			if kind == "" {
				what = "its API group " + code(group)
			}
			title := fmt.Sprintf("The %s CRD that %s removes is installed, but no manifest uses %s", code(installed.Name), toTag, what)
			detail := fmt.Sprintf("%s no longer ships this CustomResourceDefinition and it is installed in your environment, but none of the manifests you supplied declares a resource of it — nothing you supplied stops being reconciled. Checked %d installed CRD(s) and %d manifest apiVersion(s).",
				toTag, len(b.env.CRDs), len(b.env.APIVersions))
			b.verdict(RuleCRDUnused, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence,
				[]domain.ImpactCheck{b.crdsCheck([]string{s}), b.apiVersionsCheck([]string{s})})
		case installed != nil:
			// installed, but no manifests were supplied: whether resources of
			// the kind exist cannot be checked → credible overlap, unproven
			// necessity (never ACTION REQUIRED without usage evidence).
			detail := fmt.Sprintf("%s no longer ships this CustomResourceDefinition and it is installed in your environment. Whether resources of it exist cannot be checked: no manifests were supplied. If none exist, nothing stops being reconciled.", toTag)
			b.add(RuleCRDRemoved, domain.ImpactReviewRequired, domain.SeverityCritical, domain.ConfidenceMedium,
				fmt.Sprintf("The %s CRD that %s removes is installed", code(installed.Name), toTag),
				detail, c, []domain.ImpactMatch{{Kind: domain.MatchCRD, Subject: name, Evidence: installed.Evidence}}, c.Evidence...)
		default:
			unmatched = append(unmatched, s)
		}
	}
	if len(unmatched) > 0 && len(unmatched) == parsed {
		title := fmt.Sprintf("The %s that %s removes %s not installed",
			codeList(unmatched, 3), toTag, pluralIs(unmatched))
		detail := fmt.Sprintf("Your environment does not have this CustomResourceDefinition installed and no manifest uses its API group, so no stored resources of it can stop being reconciled. Checked %d installed CRD(s).", len(b.env.CRDs))
		checks := []domain.ImpactCheck{b.crdsCheck(unmatched)}
		if b.env.Supplied.Manifests {
			checks = append(checks, b.apiVersionsCheck(unmatched))
		}
		b.verdict(RuleCRDUnused, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence, checks)
	}
}

// crdVersionGone joins crd:version-removed / crd:version-unserved. Ladder:
// exact GVK in use (kind pinned via the installed CRD) → action (high);
// group/version in use with the kind unpinnable → review at most; the
// installed CRD declares the version but no manifest uses it → not-affected
// (nothing supplied breaks) or, without manifests, review; nothing →
// not-affected; a subject without "name/version" → unknown (it used to be
// silently skipped).
func (b *builder) crdVersionGone(c domain.Change, toTag string) {
	if !b.env.Supplied.CRDs {
		checks := b.partialChecks([]domain.ImpactCheck{b.apiVersionsCheck(c.Subjects)}, b.env.Supplied.Manifests)
		b.unknown(domain.UnknownEnvironmentVisibilityGap, RuleInsufficientVisibility, c.ID,
			fmt.Sprintf("Cannot tell whether the removed API version affects you: installed CRDs were not supplied (%s)", c.Title),
			"Applicability is decided against the versions your installed CRDs declare and the apiVersions your manifests use; without the CRDs the join cannot look.",
			c, c.Evidence, checks, "installed CustomResourceDefinitions (--crds) not supplied")
		return
	}
	gone := "removed"
	if c.Provenance.Rule == upgrade.RuleCRDVersionUnserved {
		gone = "no longer served"
	}
	var unmatched, unparsed []string
	parsed := 0
	for _, s := range c.Subjects {
		id, ok := parseCRDNameVersion(s)
		if !ok {
			unparsed = append(unparsed, s)
			continue
		}
		parsed++
		installed := crdByName(b.env, id.Name)
		kind := ""
		if installed != nil {
			kind = installed.Kind
		}
		declared := installedDeclares(installed, id.Version)
		g := newGVKLines(b, id.Name, installed)
		uses := b.gvkUses(func(u env.GVKUsage) bool {
			return u.Group == id.Group && u.Version == id.Version && (kind == "" || u.Kind == kind)
		})
		gv := id.Group + "/" + id.Version
		switch {
		case len(uses) > 0 && kind != "":
			// exact GVK: group/version/kind all pinned and in manifest use.
			matches := gvkMatches(uses)
			matches = appendUniqueMatches(matches, domain.ImpactMatch{
				Kind: domain.MatchCRD, Subject: id.Name, Evidence: installed.Evidence,
			})
			if declared {
				matches = appendUniqueMatches(matches, domain.ImpactMatch{
					Kind: domain.MatchCRDVersion, Subject: id.Name + "/" + id.Version, Evidence: installed.Evidence,
				})
			}
			detail := fmt.Sprintf("Manifests and clients using %s fail after the upgrade. Migrate them to a version the target still serves.\n%s", gv, strings.Join(g.lines(uses), "\n"))
			if installed != nil {
				detail += fmt.Sprintf("\nYour installed %s declares it.", id.Name)
			}
			b.add(RuleCRDVersionRemoved, domain.ImpactActionRequired, domain.SeverityCritical, domain.ConfidenceHigh,
				fmt.Sprintf("You use API version %s, which %s %s", code(gv), toTag, gone), detail, c, matches, c.Evidence...)
		case len(uses) > 0:
			// group/version only: the kind in use cannot be tied to the
			// changed CRD — another CRD of the group could serve it.
			detail := fmt.Sprintf("Manifests use %s, but the kind is not pinned: your installed CRDs do not include %s, and another CRD of %s could serve %s. To decide, the installed CRDs must contain %s (its names.kind then checks against your manifests).\n%s",
				code(gv), code(id.Name), code(id.Group), strings.Join(apiVersionStrings(uses), ", "), code(id.Name), strings.Join(g.lines(uses), "\n"))
			b.add(RuleCRDVersionRemoved, domain.ImpactReviewRequired, domain.SeverityCritical, domain.ConfidenceMedium,
				fmt.Sprintf("Manifests use API version %s, which %s %s — kind unconfirmed", code(gv), toTag, gone),
				detail, c, gvkMatches(uses), c.Evidence...)
		case declared && !b.env.Supplied.Manifests:
			// the version survives in the installed CRDs, but whether any
			// manifest uses it cannot be checked.
			detail := fmt.Sprintf("Your installed %s declares %s, which %s %s. Whether manifests or clients use it cannot be checked: no manifests were supplied.",
				id.Name, gv, toTag, gone)
			b.add(RuleCRDVersionRemoved, domain.ImpactReviewRequired, domain.SeverityCritical, domain.ConfidenceMedium,
				fmt.Sprintf("Installed %s declares API version %s, which %s %s", id.Name, code(gv), toTag, gone),
				detail, c, []domain.ImpactMatch{{Kind: domain.MatchCRD, Subject: id.Name, Evidence: installed.Evidence}}, c.Evidence...)
		case declared:
			title := fmt.Sprintf("API version %s %s: declared by your installed %s, used by no manifest", code(gv), gone, id.Name)
			detail := fmt.Sprintf("Your installed %s declares %s, but none of the manifests you supplied uses that group/version/kind — nothing you supplied breaks. Checked %d installed CRD(s) and %d manifest apiVersion(s).",
				id.Name, gv, len(b.env.CRDs), len(b.env.APIVersions))
			b.verdict(RuleCRDVersionUnused, domain.ImpactNotAffected, c.ID+":"+s, title, detail, c, c.Evidence,
				[]domain.ImpactCheck{b.crdsCheck([]string{s}), b.apiVersionsCheck([]string{s})})
		default:
			unmatched = append(unmatched, s)
		}
	}
	if len(unparsed) > 0 {
		b.unknown(domain.UnknownSemanticAmbiguity, RuleNotJoined, c.ID+":unparsed",
			fmt.Sprintf("Cannot tell whether the removed API version affects you: %s does not name name/version", codeList(unparsed, 3)),
			"A version change is joined through its CRD name and API version; a subject without that shape cannot be scoped to a group/version/kind.",
			c, c.Evidence, nil,
			fmt.Sprintf("the API-version subject(s) %s do not state CRD name and version", codeList(unparsed, 3)))
	}
	if len(unmatched) > 0 && len(unmatched) == parsed {
		title := fmt.Sprintf("API version(s) %s %s: not used in your environment", codeList(unmatched, 2), gone)
		detail := fmt.Sprintf("No installed CRD of your environment declares these versions and no manifest uses the apiVersion, so nothing fails. Checked %d installed CRD(s) and %d manifest apiVersion(s).", len(b.env.CRDs), len(b.env.APIVersions))
		b.verdict(RuleCRDVersionUnused, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence,
			[]domain.ImpactCheck{b.crdsCheck(unmatched), b.apiVersionsCheck(unmatched)})
	} else {
		for _, s := range unmatched {
			b.verdict(RuleCRDVersionUnused, domain.ImpactNotAffected, c.ID+":"+s,
				fmt.Sprintf("API version %s %s: not used in your environment", code(s), gone),
				"No installed CRD of your environment declares this version and no manifest uses the apiVersion, so nothing fails.",
				c, c.Evidence, []domain.ImpactCheck{b.crdsCheck([]string{s}), b.apiVersionsCheck([]string{s})})
		}
	}
}

// crdVersionDeprecated joins crd:version-deprecated. Same ladder as
// crdVersionGone, but every affected verdict is review-required (a
// deprecation breaks nothing today) and a declared-but-unused version with
// manifests supplied is not-affected.
func (b *builder) crdVersionDeprecated(c domain.Change, toTag string) {
	if !b.env.Supplied.CRDs {
		checks := b.partialChecks([]domain.ImpactCheck{b.apiVersionsCheck(c.Subjects)}, b.env.Supplied.Manifests)
		b.unknown(domain.UnknownEnvironmentVisibilityGap, RuleInsufficientVisibility, c.ID,
			fmt.Sprintf("Cannot tell whether the deprecated API version affects you: installed CRDs were not supplied (%s)", c.Title),
			"Applicability is decided against the versions your installed CRDs declare and the apiVersions your manifests use; without the CRDs the join cannot look.",
			c, c.Evidence, checks, "installed CustomResourceDefinitions (--crds) not supplied")
		return
	}
	var unmatched, unparsed []string
	parsed := 0
	for _, s := range c.Subjects {
		id, ok := parseCRDNameVersion(s)
		if !ok {
			unparsed = append(unparsed, s)
			continue
		}
		parsed++
		installed := crdByName(b.env, id.Name)
		kind := ""
		if installed != nil {
			kind = installed.Kind
		}
		declared := installedDeclares(installed, id.Version)
		g := newGVKLines(b, id.Name, installed)
		uses := b.gvkUses(func(u env.GVKUsage) bool {
			return u.Group == id.Group && u.Version == id.Version && (kind == "" || u.Kind == kind)
		})
		gv := id.Group + "/" + id.Version
		switch {
		case len(uses) > 0:
			matches := gvkMatches(uses)
			if declared {
				matches = appendUniqueMatches(matches, domain.ImpactMatch{
					Kind: domain.MatchCRDVersion, Subject: id.Name + "/" + id.Version, Evidence: installed.Evidence,
				})
			}
			detail := fmt.Sprintf("The API version still works, but %s marks it deprecated; plan the migration before a later release removes it.\n%s", toTag, strings.Join(g.lines(uses), "\n"))
			b.add(RuleCRDVersionDeprecated, domain.ImpactReviewRequired, domain.SeverityMedium, domain.ConfidenceHigh,
				fmt.Sprintf("You use API version %s, deprecated as of %s", code(gv), toTag), detail, c, matches, c.Evidence...)
		case declared && !b.env.Supplied.Manifests:
			detail := fmt.Sprintf("Your installed %s declares %s, which %s marks deprecated. Whether manifests or clients use it cannot be checked: no manifests were supplied. Nothing breaks today; a later release removing the version would.",
				id.Name, gv, toTag)
			b.add(RuleCRDVersionDeprecated, domain.ImpactReviewRequired, domain.SeverityMedium, domain.ConfidenceMedium,
				fmt.Sprintf("Installed %s declares API version %s, deprecated as of %s", id.Name, code(gv), toTag),
				detail, c, []domain.ImpactMatch{{Kind: domain.MatchCRD, Subject: id.Name, Evidence: installed.Evidence}}, c.Evidence...)
		case declared:
			title := fmt.Sprintf("API version %s is deprecated: declared by your installed %s, used by no manifest", code(gv), id.Name)
			detail := fmt.Sprintf("Your installed %s declares %s and %s marks it deprecated, but none of the manifests you supplied uses that group/version/kind — nothing you supplied needs migrating today. Checked %d installed CRD(s) and %d manifest apiVersion(s).",
				id.Name, gv, toTag, len(b.env.CRDs), len(b.env.APIVersions))
			b.verdict(RuleCRDVersionUnused, domain.ImpactNotAffected, c.ID+":"+s, title, detail, c, c.Evidence,
				[]domain.ImpactCheck{b.crdsCheck([]string{s}), b.apiVersionsCheck([]string{s})})
		default:
			unmatched = append(unmatched, s)
		}
	}
	if len(unparsed) > 0 {
		b.unknown(domain.UnknownSemanticAmbiguity, RuleNotJoined, c.ID+":unparsed",
			fmt.Sprintf("Cannot tell whether the deprecated API version affects you: %s does not name name/version", codeList(unparsed, 3)),
			"A version change is joined through its CRD name and API version; a subject without that shape cannot be scoped to a group/version/kind.",
			c, c.Evidence, nil,
			fmt.Sprintf("the API-version subject(s) %s do not state CRD name and version", codeList(unparsed, 3)))
	}
	if len(unmatched) > 0 && len(unmatched) == parsed {
		title := fmt.Sprintf("Deprecated API version(s) %s: not used in your environment", codeList(unmatched, 2))
		detail := fmt.Sprintf("No installed CRD of your environment declares these versions and no manifest uses the apiVersion, so the deprecation changes nothing for you today. Checked %d installed CRD(s) and %d manifest apiVersion(s).", len(b.env.CRDs), len(b.env.APIVersions))
		b.verdict(RuleCRDVersionUnused, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence,
			[]domain.ImpactCheck{b.crdsCheck(unmatched), b.apiVersionsCheck(unmatched)})
	} else {
		for _, s := range unmatched {
			b.verdict(RuleCRDVersionUnused, domain.ImpactNotAffected, c.ID+":"+s,
				fmt.Sprintf("Deprecated API version %s: not used in your environment", code(s)),
				"No installed CRD of your environment declares this version and no manifest uses the apiVersion, so the deprecation changes nothing for you today.",
				c, c.Evidence, []domain.ImpactCheck{b.crdsCheck([]string{s}), b.apiVersionsCheck([]string{s})})
		}
	}
}

// crdFieldsRemoved joins a crd:fields-removed change. The removed schema paths
// are matched only inside the GVK the change names (exact path, or a set path
// below/above it — the same at/below semantics as before, now scoped):
// exact GVK + exact field → action-required high; group/version without a
// pinned kind (or version) → review at most; no manifest of that GVK sets the
// paths → not-affected; a change that yields no API group at all → unknown
// (it used to path-match, flagging unrelated resources).
func (b *builder) crdFieldsRemoved(c domain.Change, toTag string) {
	// deciding dimension: manifests (field paths are manifest facts).
	if !b.env.Supplied.Manifests {
		b.unknown(domain.UnknownEnvironmentVisibilityGap, RuleInsufficientVisibility, c.ID,
			fmt.Sprintf("Cannot tell whether the removed CRD field affects you: manifests were not supplied (%s)", c.Title),
			"Applicability of a schema field removal is decided against the field paths your manifests actually set; without them the join cannot look.",
			c, c.Evidence, nil, "Kubernetes manifests (--manifests) not supplied")
		return
	}
	id, ok := identityFromSchemaChange(c)
	// An installed CRD of the named resource completes the identity: it pins
	// the group when the title/detail did not, and its names.kind pins the kind.
	var installed *env.InstalledCRD
	if id.Name != "" {
		installed = crdByName(b.env, id.Name)
	}
	if !ok && installed != nil && installed.Group != "" {
		id.Group = installed.Group
		ok = true
	}
	if !ok {
		b.unknown(domain.UnknownSemanticAmbiguity, RuleNotJoined, c.ID,
			fmt.Sprintf("Cannot tell whether the removed CRD fields affect you: the change does not identify the CRD (%s)", c.Title),
			"The removed schema paths carry no group/version/kind, so matching them by path alone would flag every resource that sets a same-named path — a Deployment setting spec.foo is not a CRD impact. Check the paths against your manifests manually.",
			c, c.Evidence, nil,
			fmt.Sprintf("the change does not state the CRD identity (group/version/kind) of the removed schema paths %s", codeList(c.Subjects, 3)))
		return
	}
	kindPinned := id.Kind != ""
	if !kindPinned && installed != nil && installed.Kind != "" {
		id.Kind = installed.Kind
		kindPinned = true
	}
	g := newGVKLines(b, id.Name, installed)
	// candidate GVKs: manifests of the identity's group (+version/+kind when
	// the change or an installed CRD of the same name states them).
	var uses []env.GVKUsage
	for _, u := range b.env.GVKUsage {
		if len(u.FieldPaths) == 0 || u.Group != id.Group {
			continue
		}
		if id.Version != "" && u.Version != id.Version {
			continue
		}
		if kindPinned && u.Kind != id.Kind {
			continue
		}
		uses = append(uses, u)
	}
	uncertain := !kindPinned || id.Version == ""
	var matches []domain.ImpactMatch
	var exact, under, over []string
	var lines []string
	for _, u := range uses {
		var hit []env.ManifestField
		for _, s := range c.Subjects {
			up := stripArrayMarkers(s)
			for _, f := range b.env.ManifestFields {
				if f.APIVersion != groupVersionOf(u) || f.Kind != u.Kind {
					continue // same-named path under another GVK is not this CRD's field
				}
				rel := relate(up, f.Path)
				if rel == relNone {
					continue
				}
				hit = appendUniqueField(hit, f)
				matches = appendUniqueMatches(matches, domain.ImpactMatch{
					Kind: domain.MatchManifestField, Subject: f.Path, Evidence: f.Evidence,
				})
				switch rel {
				case relExact:
					exact = appendUnique(exact, f.Path)
				case relSubjectAncestor:
					under = appendUnique(under, f.Path)
				case relSubjectDescendant:
					over = appendUnique(over, f.Path)
				}
			}
		}
		if len(hit) > 0 {
			matches = appendUniqueMatches(matches, domain.ImpactMatch{
				Kind: domain.MatchAPIVersion, Subject: groupVersionOf(u) + " " + u.Kind, Evidence: u.Evidence,
			})
			lines = append(lines, g.line(usageSettingFields(b.env, g.ix, u, hit), fieldSetsPhrase(hit)))
		}
	}
	if len(matches) == 0 {
		scope := id.Group
		if id.Version != "" {
			scope += "/" + id.Version
		}
		if kindPinned {
			scope += " " + id.Kind
		}
		title := fmt.Sprintf("No manifest of %s sets the removed field(s) %s", code(scope), codeList(c.Subjects, 3))
		detail := fmt.Sprintf("Pruned schema fields only affect resources that set them. Your manifests set %d field paths and none of them is the removed path or below it within %s.", len(b.env.ManifestFields), code(scope))
		b.verdict(RuleCRDFieldUnset, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence,
			[]domain.ImpactCheck{b.manifestsCheck(c.Subjects)})
		return
	}
	if len(exact)+len(under) > 0 {
		paths := append(append([]string{}, exact...), under...)
		detail := fmt.Sprintf("Fields no longer in the schema are pruned from stored objects and rejected or dropped in manifests. Remove them from your resources.\nYou set: %s.\n%s",
			strings.Join(paths, ", "), strings.Join(lines, "\n"))
		conf := domain.ConfidenceHigh
		if uncertain {
			// group/version match without a pinned kind/version: another CRD
			// of the group could serve the kind that sets the path — review
			// at most (the contract demotes medium confidence).
			conf = domain.ConfidenceMedium
		}
		b.add(RuleCRDFieldRemoved, domain.ImpactActionRequired, domain.SeverityHigh, conf,
			fmt.Sprintf("Your manifests set %s, pruned from the CRD schema in %s", codeList(paths, 3), toTag),
			detail, c, matches, c.Evidence...)
		return
	}
	title := fmt.Sprintf("You set a section above the removed path(s) %s, pruned from the CRD schema in %s", codeList(c.Subjects, 3), toTag)
	detail := fmt.Sprintf("The removed path is below keys you set (%s); whether your resources are affected depends on which sub-fields they use. Compare your manifests with the new schema.\n%s",
		strings.Join(over, ", "), strings.Join(lines, "\n"))
	b.add(RuleCRDFieldRemoved, domain.ImpactReviewRequired, domain.SeverityMedium, domain.ConfidenceMedium,
		title, detail, c, matches, c.Evidence...)
}
