package impact

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/normalize"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// relation describes how an environment key path and an upstream subject
// path overlap. Paths use the values-snapshot syntax and are compared
// segment-wise (so `a.bb` is never a prefix of `a.b.c`).
type relation int

const (
	relNone relation = iota
	relExact
	relSubjectAncestor   // the upstream path is an ancestor of the environment path
	relSubjectDescendant // the upstream path is below the environment path
)

// relate compares two values-style paths.
func relate(upstream, environment string) relation {
	a, b := env.SplitKeyPath(upstream), env.SplitKeyPath(environment)
	if len(a) == 0 || len(b) == 0 {
		return relNone
	}
	switch {
	case equalSegments(a, b):
		return relExact
	case len(a) < len(b) && equalSegments(a, b[:len(a)]):
		return relSubjectAncestor
	case len(b) < len(a) && equalSegments(b, a[:len(b)]):
		return relSubjectDescendant
	}
	return relNone
}

func equalSegments(a, b []string) bool {
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

// stripArrayMarkers normalises an upstream CRD schema path ("spec.foo[].bar"
// → "spec.foo.bar") so it can be compared with flattened manifest paths.
func stripArrayMarkers(p string) string {
	return strings.ReplaceAll(p, "[]", "")
}

// joinRules are the upstream diff rules whose subjects the join can compare:
// every other change is unknown (impact:not-joined) — applicability of a
// declared note change cannot be decided deterministically.
var joinRules = map[string]bool{
	upgrade.RuleValuesRemoved: true, upgrade.RuleValuesSectionRemoved: true,
	upgrade.RuleValuesDefaultChanged: true, upgrade.RuleValuesAdded: true,
	upgrade.RuleCRDRemoved: true, upgrade.RuleCRDVersionRemoved: true,
	upgrade.RuleCRDVersionUnserved: true, upgrade.RuleCRDVersionDeprecated: true,
	upgrade.RuleCRDFieldsRemoved: true,
	upgrade.RuleImageRemoved:     true, upgrade.RuleImageMoved: true, upgrade.RuleImageTagsChanged: true,
}

// unimplementedDiffRules are computed diff rules the join family knows about
// but has no join rule for (additions/flags that cannot be decided
// deterministically); they get a more specific neededToDetermine than
// note-derived changes.
var unimplementedDiffRules = map[string]bool{
	upgrade.RuleCRDAdded: true, upgrade.RuleCRDVersionAdded: true,
	upgrade.RuleCRDStorageChanged: true, upgrade.RuleCRDFieldsAdded: true,
	upgrade.RuleImageAdded: true,
}

// builder accumulates the report while Build runs.
type builder struct {
	edge *domain.UpgradeEdge
	env  *env.Environment
	rep  *domain.ImpactReport

	findings []domain.ImpactFinding
	seen     map[string]bool
	edgeEv   map[domain.EvidenceID]bool
	upCited  map[domain.EvidenceID]bool
	locCited map[domain.EvidenceID]bool
}

func build(in Input) (*domain.ImpactReport, error) {
	if in.Edge == nil || in.Env == nil {
		return nil, fmt.Errorf("impact: Input.Edge and Input.Env are required")
	}
	now := in.Now
	if now.IsZero() {
		now = time.Time{}
	}
	b := &builder{
		edge: in.Edge, env: in.Env,
		seen: map[string]bool{}, edgeEv: map[domain.EvidenceID]bool{},
		upCited: map[domain.EvidenceID]bool{}, locCited: map[domain.EvidenceID]bool{},
	}
	for _, e := range in.Edge.Evidence {
		b.edgeEv[e.ID] = true
	}
	b.rep = &domain.ImpactReport{
		SchemaVersion: domain.ImpactReportSchemaVersion,
		Product:       in.Edge.Product,
		From:          in.Edge.From,
		To:            in.Edge.To,
		GeneratedAt:   now.UTC(),
	}
	b.rep.Environment = environmentSummary(in.Env)
	b.rep.Warnings = append(append([]string{}, in.Edge.Warnings...), in.Env.Warnings...)
	b.rep.DefinitionDigest = in.Edge.DefinitionDigest

	b.valuesFamily()
	b.crdFamily()
	b.compatibilityChecks()
	b.imageFamily()
	b.unjoinedChanges()

	sortFindings(b.findings)
	b.rep.Findings = b.findings
	b.collectEvidence()
	b.summarize()
	if err := b.rep.Validate(); err != nil {
		return nil, fmt.Errorf("impact: assembled report is invalid: %w", err)
	}
	return b.rep, nil
}

func environmentSummary(e *env.Environment) domain.ImpactEnvironment {
	s := domain.ImpactEnvironment{
		ValuesKeys:    len(e.ValuesKeys),
		ManifestDocs:  e.ManifestDocCount,
		APIVersions:   len(e.APIVersions),
		CRDs:          len(e.CRDs),
		ManifestPaths: len(e.ManifestFields),
		Images:        len(e.Images),
		Warnings:      e.Warnings,
	}
	if e.Kubernetes != nil {
		s.Kubernetes = e.Kubernetes.Version
	}
	for _, f := range e.Files {
		s.Files = append(s.Files, domain.ImpactFile{Path: f.Path, Digest: f.Digest})
	}
	return s
}

// --- verdict emission -----------------------------------------------------------

// add appends an AFFECTED finding unless an identical one exists. Upstream
// evidence is filtered to ids that resolve in the edge; a finding without any
// (or without matches) is dropped — the join never states anything
// unevidenced. The contract's demotion rule is enforced here: an
// action-required finding with confidence below high is demoted to
// review-required, because ambiguity is never promoted into mandatory action.
func (b *builder) add(rule string, class domain.ImpactClass, sev domain.ImpactSeverity, conf domain.Confidence, title, detail string,
	change domain.Change, matches []domain.ImpactMatch, upstream ...domain.EvidenceID) {
	if class == domain.ImpactActionRequired && conf != domain.ConfidenceHigh {
		class = domain.ImpactReviewRequired
	}
	var up []domain.EvidenceID
	for _, id := range upstream {
		if b.edgeEv[id] {
			up = appendUnique(up, id)
		}
	}
	if len(up) == 0 || len(matches) == 0 {
		return
	}
	subjects := make([]string, len(matches))
	for i, m := range matches {
		subjects[i] = string(m.Kind) + ":" + m.Subject
	}
	id := "imp-" + domain.ShortHash(append([]string{rule, change.ID}, subjects...)...)
	if b.seen[id] {
		return
	}
	b.seen[id] = true

	var local []domain.EvidenceID
	for _, m := range matches {
		local = appendUnique(local, m.Evidence...)
	}
	f := domain.ImpactFinding{
		ID: id, Classification: class, Severity: sev, Rule: rule, Title: title, Detail: detail,
		Matches: matches, UpstreamEvidence: up, EnvironmentEvidence: local,
		Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: Producer, Rule: rule, Confidence: conf},
	}
	b.attachChange(&f, change)
	b.findings = append(b.findings, f)
	for _, id := range up {
		b.upCited[id] = true
	}
	for _, id := range local {
		b.locCited[id] = true
	}
}

// verdict appends a NOT AFFECTED or UNKNOWN record: no matches (nothing
// matched, or nothing could be checked), the upstream chain for context, the
// evaluation record (checks) and/or the missing-evidence list (needed).
// key deduplicates: the change id, artifact coordinates or a constraint tag.
func (b *builder) verdict(rule string, class domain.ImpactClass, key, title, detail string,
	change domain.Change, upstream []domain.EvidenceID, checks []domain.ImpactCheck, needed ...string) {
	if class != domain.ImpactNotAffected && class != domain.ImpactUnknown {
		panic(fmt.Sprintf("impact: verdict() is for not-affected/unknown, got %s", class))
	}
	var up []domain.EvidenceID
	for _, id := range upstream {
		if b.edgeEv[id] {
			up = appendUnique(up, id)
		}
	}
	if len(up) == 0 {
		return
	}
	id := "imp-" + domain.ShortHash(rule, key)
	if b.seen[id] {
		return
	}
	b.seen[id] = true
	f := domain.ImpactFinding{
		ID: id, Classification: class, Rule: rule, Title: title, Detail: detail,
		UpstreamEvidence: up, Checks: checks, NeededToDetermine: needed,
		Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: Producer, Rule: rule, Confidence: domain.ConfidenceHigh},
	}
	b.attachChange(&f, change)
	b.findings = append(b.findings, f)
	for _, id := range up {
		b.upCited[id] = true
	}
	for _, c := range checks {
		for _, id := range c.Evidence {
			b.locCited[id] = true
		}
	}
}

func (b *builder) attachChange(f *domain.ImpactFinding, change domain.Change) {
	if change.ID != "" {
		f.ChangeID = change.ID
		f.ChangeTitle = change.Title
		f.ChangeCategory = change.Category
		f.ChangeBreaking = change.Breaking
		f.ChangeActionRequired = change.ActionRequired
	}
}

// visibility helpers ---------------------------------------------------------

// imagesVisible reports whether any image-bearing input was supplied (--images,
// --values and --manifests all yield image facts).
func (b *builder) imagesVisible() bool {
	return b.env.Supplied.Images || b.env.Supplied.Values || b.env.Supplied.Manifests
}

// valuesCheck, crdsCheck, manifestsCheck, imagesCheck and clusterCheck build
// the evaluation records of not-affected (and partially-evaluated unknown)
// verdicts: what was checked against what.
func (b *builder) valuesCheck(subjects []string) domain.ImpactCheck {
	return domain.ImpactCheck{Dimension: domain.DimensionValues, Facts: len(b.env.ValuesKeys), Subjects: subjects}
}

func (b *builder) crdsCheck(subjects []string) domain.ImpactCheck {
	return domain.ImpactCheck{Dimension: domain.DimensionCRDs, Facts: len(b.env.CRDs), Subjects: subjects}
}

func (b *builder) manifestsCheck(subjects []string) domain.ImpactCheck {
	return domain.ImpactCheck{Dimension: domain.DimensionManifests, Facts: len(b.env.ManifestFields), Subjects: subjects}
}

func (b *builder) apiVersionsCheck(subjects []string) domain.ImpactCheck {
	return domain.ImpactCheck{Dimension: domain.DimensionManifests, Facts: len(b.env.APIVersions), Subjects: subjects}
}

func (b *builder) imagesCheck(subjects []string) domain.ImpactCheck {
	return domain.ImpactCheck{Dimension: domain.DimensionImages, Facts: len(b.env.Images), Subjects: subjects}
}

func (b *builder) clusterCheck(platform string, subjects []string) domain.ImpactCheck {
	c := domain.ImpactCheck{Dimension: domain.DimensionCluster, Platform: platform, Facts: 0, Subjects: subjects}
	if b.env.Kubernetes != nil && strings.EqualFold(platform, "kubernetes") {
		c.Facts = 1
		c.Evidence = b.env.Kubernetes.Evidence
	}
	return c
}

// subjectLess records the honest verdict for a computed change that carries
// no subjects at all: there is nothing to compare, so applicability is
// unknown rather than "not affected".
func (b *builder) subjectLess(c domain.Change) {
	b.verdict(RuleNotJoined, domain.ImpactUnknown, c.ID,
		fmt.Sprintf("Not evaluated for this environment: %s", c.Title),
		"The change carries no comparable subjects, so there is nothing to check against the environment.",
		c, c.Evidence, nil, "computed change carries no subjects to compare")
}

// --- rule 1: Helm values -----------------------------------------------------

// upstream values diff rules → join treatment.
func valuesDiffKind(rule string) string {
	switch rule {
	case upgrade.RuleValuesRemoved, upgrade.RuleValuesSectionRemoved:
		return "removed"
	case upgrade.RuleValuesDefaultChanged:
		return "default-changed"
	case upgrade.RuleValuesAdded:
		return "added"
	}
	return ""
}

func (b *builder) valuesFamily() {
	toTag := b.edge.To.String()
	for _, c := range b.edge.Changes {
		kind := valuesDiffKind(c.Provenance.Rule)
		if kind == "" {
			continue
		}
		if len(c.Subjects) == 0 {
			b.subjectLess(c)
			continue
		}
		// deciding dimension: values. Without it the verdict is unknown —
		// "no match" must never be read as "not affected" (the contract's
		// example).
		if !b.env.Supplied.Values {
			b.verdict(RuleInsufficientVisibility, domain.ImpactUnknown, c.ID,
				fmt.Sprintf("Cannot tell whether this affects you: no Helm values were supplied (%s)", c.Title),
				"Applicability of a Helm values change can only be decided against the values you actually set; without a values file there is nothing to compare.",
				c, c.Evidence, nil, "Helm values files (--values) not supplied")
			continue
		}
		var matches []domain.ImpactMatch
		var exact, partial []string
		for _, s := range c.Subjects {
			for _, k := range b.env.ValuesKeys {
				rel := relate(s, k.Path)
				if rel == relNone {
					continue
				}
				matches = appendUniqueMatches(matches, domain.ImpactMatch{
					Kind: domain.MatchValuesKey, Subject: k.Path, Evidence: k.Evidence,
				})
				if rel == relExact {
					exact = appendUnique(exact, k.Path)
				} else {
					partial = appendUnique(partial, fmt.Sprintf("%s (upstream: %s)", k.Path, s))
				}
			}
		}
		if len(matches) == 0 {
			title := fmt.Sprintf("Your values do not touch %s", codeList(c.Subjects, 3))
			if len(c.Subjects) == 1 {
				title = fmt.Sprintf("Your values do not set %s", code(c.Subjects[0]))
			}
			detail := fmt.Sprintf("%s of %s changes these keys; your supplied values files set %d keys in total and none of them is the changed key or under it, so nothing about this change takes effect on you.",
				"The target release", toTag, len(b.env.ValuesKeys))
			b.verdict(RuleValuesUnset, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence,
				[]domain.ImpactCheck{b.valuesCheck(c.Subjects)})
			continue
		}
		switch kind {
		case "removed":
			title := fmt.Sprintf("You set %s that %s removed", pluralKeys(exact, partial), toTag)
			if len(exact)+len(partial) == 1 {
				title = fmt.Sprintf("You set %s, which %s removed", code(firstOf(exact, partial)), toTag)
			}
			detail := fmt.Sprintf("%s no longer has these values keys; keys you set here stop taking effect (or are rejected when the chart validates values against a schema). Remove them from your values and migrate the configuration they controlled.\nYou set: %s.",
				toTag, strings.Join(append(append([]string{}, exact...), partial...), ", "))
			b.add(RuleValuesRemoved, domain.ImpactActionRequired, domain.SeverityHigh, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
		case "default-changed":
			if len(exact) > 0 {
				title := fmt.Sprintf("You pin %s, so the new default of %s does not apply", codeList(exact, 3), toTag)
				if len(exact) == 1 && len(partial) == 0 {
					title = fmt.Sprintf("You pin %s; its default change in %s does not apply to you", code(exact[0]), toTag)
				}
				detail := fmt.Sprintf("You set %s explicitly. Helm keeps your value and discards the new default; your deployment does not change. Re-check the value against the new defaults when you next edit it.", strings.Join(exact, ", "))
				if len(partial) > 0 {
					detail += "\nAdjacent keys you also set: " + strings.Join(partial, ", ") + "."
				}
				b.add(RuleValuesPinned, domain.ImpactInformational, domain.SeverityLow, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
			}
			if len(partial) > 0 {
				title := fmt.Sprintf("Default changed under a section you set: %s", codeList(splitBeforeParen(partial), 3))
				detail := fmt.Sprintf("%s changed a default below or above keys you set (%s). Helm merges your mapping with the chart defaults; review the merged result after the upgrade.", toTag, strings.Join(partial, ", "))
				b.add(RuleValuesAdjacent, domain.ImpactReviewRequired, domain.SeverityMedium, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
			}
		case "added":
			title := fmt.Sprintf("Your values already set %s, new in %s", pluralKeys(exact, partial), toTag)
			if len(exact)+len(partial) == 1 {
				title = fmt.Sprintf("Your values already set %s, which %s introduces", firstOf(exact, partial), toTag)
			}
			detail := "A key your values file already set (previously ignored by the chart) becomes live with this release, or sits directly under/over a newly added section. Check that the value you set is what you intend the new key to have."
			b.add(RuleValuesNewKey, domain.ImpactReviewRequired, domain.SeverityMedium, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
		}
	}
}

// --- rule 2: CRDs and API versions --------------------------------------------

// crdNameAndVersion splits a "name/version" subject of the CRD diff rules.
func crdNameAndVersion(s string) (name, version string) {
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// groupOfCRDName derives the API group from a CRD resource name
// ("certificates.cert-manager.io" → "cert-manager.io").
func groupOfCRDName(name string) string {
	if i := strings.IndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return ""
}

func (b *builder) crdFamily() {
	toTag := b.edge.To.String()
	for _, c := range b.edge.Changes {
		if len(c.Subjects) == 0 {
			switch c.Provenance.Rule {
			case upgrade.RuleCRDRemoved, upgrade.RuleCRDVersionRemoved, upgrade.RuleCRDVersionUnserved,
				upgrade.RuleCRDVersionDeprecated, upgrade.RuleCRDFieldsRemoved:
				b.subjectLess(c)
				continue
			}
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

// crdMatches collects the environment facts that show the environment uses a
// CRD: the installed CRD (name match) and every manifest apiVersion of its
// group (+kind when known).
func (b *builder) crdMatches(name string) (matches []domain.ImpactMatch, installed *env.InstalledCRD, groupUsage []env.APIVersionUse) {
	group := groupOfCRDName(name)
	for i := range b.env.CRDs {
		if b.env.CRDs[i].Name == name {
			installed = &b.env.CRDs[i]
			matches = appendUniqueMatches(matches, domain.ImpactMatch{
				Kind: domain.MatchCRD, Subject: name, Evidence: b.env.CRDs[i].Evidence,
			})
			break
		}
	}
	for _, u := range b.env.APIVersions {
		g := u.GroupVersion
		if i := strings.IndexByte(g, '/'); i >= 0 {
			g = g[:i]
		}
		if g != group {
			continue
		}
		if installed != nil && installed.Kind != "" && u.Kind != installed.Kind {
			continue
		}
		groupUsage = append(groupUsage, u)
		matches = appendUniqueMatches(matches, domain.ImpactMatch{
			Kind: domain.MatchAPIVersion, Subject: u.GroupVersion + " " + u.Kind, Evidence: u.Evidence,
		})
	}
	return matches, installed, groupUsage
}

func (b *builder) crdRemoved(c domain.Change, toTag string) {
	// deciding dimension: installed CRDs (authoritative for what the cluster
	// runs). Manifests refine affected-path confidence; both are recorded.
	if !b.env.Supplied.CRDs {
		checks := b.partialChecks([]domain.ImpactCheck{b.apiVersionsCheck(c.Subjects)}, b.env.Supplied.Manifests)
		b.verdict(RuleInsufficientVisibility, domain.ImpactUnknown, c.ID,
			fmt.Sprintf("Cannot tell whether the removed CRD affects you: installed CRDs were not supplied (%s)", c.Title),
			"Applicability of a CRD removal is decided against the CustomResourceDefinitions your cluster actually has installed; without them the join cannot look.",
			c, c.Evidence, checks, "installed CustomResourceDefinitions (--crds) not supplied")
		return
	}
	var unmatched []string
	for _, s := range c.Subjects {
		matches, installed, usage := b.crdMatches(s)
		if len(matches) == 0 {
			unmatched = append(unmatched, s)
			continue
		}
		conf := domain.ConfidenceHigh
		detailWhere := fmt.Sprintf("manifests use %s", strings.Join(apiVersionStrings(usage), ", "))
		if installed != nil {
			detailWhere = fmt.Sprintf("%s is installed", installed.Name)
			if len(usage) > 0 {
				detailWhere += " and manifests use " + strings.Join(apiVersionStrings(usage), ", ")
			}
		} else {
			// matched on group alone: another kind of the same group could
			// be the one in use — below-high confidence is demoted to
			// review-required by the contract.
			conf = domain.ConfidenceMedium
		}
		title := fmt.Sprintf("You use the %s CRD that %s removes", code(s), toTag)
		detail := fmt.Sprintf("%s no longer ships this CustomResourceDefinition; existing resources stop being reconciled. %s. Migrate or delete the resources before upgrading.", toTag, detailWhere)
		b.add(RuleCRDRemoved, domain.ImpactActionRequired, domain.SeverityCritical, conf, title, detail, c, matches, c.Evidence...)
	}
	if len(unmatched) > 0 && len(unmatched) == len(c.Subjects) {
		// nothing matched at all: full CRD visibility → not affected
		title := fmt.Sprintf("The %s that %s removes %s not installed",
			codeList(unmatched, 3), toTag, pluralIs(unmatched))
		detail := fmt.Sprintf("Your environment does not have this CustomResourceDefinition installed, so no stored resources of it can stop being reconciled. Checked %d installed CRD(s).", len(b.env.CRDs))
		checks := []domain.ImpactCheck{b.crdsCheck(unmatched)}
		if b.env.Supplied.Manifests {
			checks = append(checks, b.apiVersionsCheck(unmatched))
		}
		b.verdict(RuleCRDUnused, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence, checks)
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

func (b *builder) crdVersionGone(c domain.Change, toTag string) {
	if !b.env.Supplied.CRDs {
		checks := b.partialChecks([]domain.ImpactCheck{b.apiVersionsCheck(c.Subjects)}, b.env.Supplied.Manifests)
		b.verdict(RuleInsufficientVisibility, domain.ImpactUnknown, c.ID,
			fmt.Sprintf("Cannot tell whether the removed API version affects you: installed CRDs were not supplied (%s)", c.Title),
			"Applicability is decided against the versions your installed CRDs declare and the apiVersions your manifests use; without the CRDs the join cannot look.",
			c, c.Evidence, checks, "installed CustomResourceDefinitions (--crds) not supplied")
		return
	}
	gone := "removed"
	if c.Provenance.Rule == upgrade.RuleCRDVersionUnserved {
		gone = "no longer served"
	}
	var unmatched []string
	for _, s := range c.Subjects {
		name, version := crdNameAndVersion(s)
		if name == "" || version == "" {
			continue
		}
		matches, installed, _ := b.crdMatches(name)
		// narrow the apiVersion matches to the removed version
		var narrowed []domain.ImpactMatch
		gv := groupOfCRDName(name) + "/" + version
		for _, m := range matches {
			if m.Kind == domain.MatchAPIVersion {
				if !strings.HasPrefix(m.Subject, gv+" ") {
					continue
				}
			}
			narrowed = append(narrowed, m)
		}
		if installed != nil {
			for _, v := range installed.Versions {
				if v.Name == version {
					narrowed = appendUniqueMatches(narrowed, domain.ImpactMatch{
						Kind: domain.MatchCRDVersion, Subject: name + "/" + version, Evidence: installed.Evidence,
					})
				}
			}
		}
		if len(narrowed) == 0 {
			unmatched = append(unmatched, s)
			continue
		}
		title := fmt.Sprintf("You use API version %s, which %s %s", code(gv), toTag, gone)
		detail := fmt.Sprintf("Manifests and clients using %s fail after the upgrade. Migrate them to a version the target still serves.", gv)
		if installed != nil {
			detail += fmt.Sprintf(" Your installed %s declares it.", name)
		}
		b.add(RuleCRDVersionRemoved, domain.ImpactActionRequired, domain.SeverityCritical, domain.ConfidenceHigh, title, detail, c, narrowed, c.Evidence...)
	}
	if len(unmatched) > 0 && len(unmatched) == len(c.Subjects) {
		title := fmt.Sprintf("API version(s) %s %s: not used in your environment", codeList(unmatched, 2), gone)
		detail := fmt.Sprintf("No installed CRD of your environment declares these versions and no manifest uses the apiVersion, so nothing fails. Checked %d installed CRD(s) and %d manifest apiVersion(s).", len(b.env.CRDs), len(b.env.APIVersions))
		b.verdict(RuleCRDVersionUnused, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence,
			[]domain.ImpactCheck{b.crdsCheck(unmatched), b.apiVersionsCheck(unmatched)})
	}
}

func (b *builder) crdVersionDeprecated(c domain.Change, toTag string) {
	if !b.env.Supplied.CRDs {
		checks := b.partialChecks([]domain.ImpactCheck{b.apiVersionsCheck(c.Subjects)}, b.env.Supplied.Manifests)
		b.verdict(RuleInsufficientVisibility, domain.ImpactUnknown, c.ID,
			fmt.Sprintf("Cannot tell whether the deprecated API version affects you: installed CRDs were not supplied (%s)", c.Title),
			"Applicability is decided against the versions your installed CRDs declare and the apiVersions your manifests use; without the CRDs the join cannot look.",
			c, c.Evidence, checks, "installed CustomResourceDefinitions (--crds) not supplied")
		return
	}
	var unmatched []string
	for _, s := range c.Subjects {
		name, version := crdNameAndVersion(s)
		if name == "" || version == "" {
			continue
		}
		matches, _, _ := b.crdMatches(name)
		gv := groupOfCRDName(name) + "/" + version
		var narrowed []domain.ImpactMatch
		for _, m := range matches {
			if m.Kind == domain.MatchAPIVersion && !strings.HasPrefix(m.Subject, gv+" ") {
				continue
			}
			if m.Kind == domain.MatchCRDVersion || m.Kind == domain.MatchAPIVersion {
				narrowed = append(narrowed, m)
			}
		}
		if installed := installedVersion(b.env, name, version); installed != nil {
			narrowed = appendUniqueMatches(narrowed, domain.ImpactMatch{
				Kind: domain.MatchCRDVersion, Subject: name + "/" + version, Evidence: installed,
			})
		}
		if len(narrowed) == 0 {
			unmatched = append(unmatched, s)
			continue
		}
		title := fmt.Sprintf("You use API version %s, deprecated as of %s", code(gv), toTag)
		detail := fmt.Sprintf("The API version still works, but %s marks it deprecated; plan the migration before a later release removes it.", toTag)
		b.add(RuleCRDVersionDeprecated, domain.ImpactReviewRequired, domain.SeverityMedium, domain.ConfidenceHigh, title, detail, c, narrowed, c.Evidence...)
	}
	if len(unmatched) > 0 && len(unmatched) == len(c.Subjects) {
		title := fmt.Sprintf("Deprecated API version(s) %s: not used in your environment", codeList(unmatched, 2))
		detail := fmt.Sprintf("No installed CRD of your environment declares these versions and no manifest uses the apiVersion, so the deprecation changes nothing for you today. Checked %d installed CRD(s) and %d manifest apiVersion(s).", len(b.env.CRDs), len(b.env.APIVersions))
		b.verdict(RuleCRDVersionUnused, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence,
			[]domain.ImpactCheck{b.crdsCheck(unmatched), b.apiVersionsCheck(unmatched)})
	}
}

func installedVersion(e *env.Environment, name, version string) []domain.EvidenceID {
	for _, crd := range e.CRDs {
		if crd.Name != name {
			continue
		}
		for _, v := range crd.Versions {
			if v.Name == version {
				return crd.Evidence
			}
		}
	}
	return nil
}

func (b *builder) crdFieldsRemoved(c domain.Change, toTag string) {
	// deciding dimension: manifests (field paths are manifest facts).
	if !b.env.Supplied.Manifests {
		b.verdict(RuleInsufficientVisibility, domain.ImpactUnknown, c.ID,
			fmt.Sprintf("Cannot tell whether the removed CRD field affects you: manifests were not supplied (%s)", c.Title),
			"Applicability of a schema field removal is decided against the field paths your manifests actually set; without them the join cannot look.",
			c, c.Evidence, nil, "Kubernetes manifests (--manifests) not supplied")
		return
	}
	var unmatched []string
	for _, s := range c.Subjects {
		up := stripArrayMarkers(s)
		var matches []domain.ImpactMatch
		var exact, under, over []string
		for _, f := range b.env.ManifestFields {
			rel := relate(up, f.Path)
			if rel == relNone {
				continue
			}
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
		if len(matches) == 0 {
			unmatched = append(unmatched, s)
			continue
		}
		if len(exact)+len(under) > 0 {
			title := fmt.Sprintf("Your manifests set %s, pruned from the CRD schema in %s", codeList(append(exact, under...), 3), toTag)
			detail := fmt.Sprintf("Fields no longer in the schema are pruned from stored objects and rejected or dropped in manifests. Remove them from your resources.\nYou set: %s.",
				strings.Join(append(exact, under...), ", "))
			b.add(RuleCRDFieldRemoved, domain.ImpactActionRequired, domain.SeverityHigh, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
			continue
		}
		title := fmt.Sprintf("You set a section above %s, pruned from the CRD schema in %s", code(s), toTag)
		detail := fmt.Sprintf("The removed path is below keys you set (%s); whether your resources are affected depends on which sub-fields they use. Compare your manifests with the new schema.", strings.Join(over, ", "))
		b.add(RuleCRDFieldRemoved, domain.ImpactReviewRequired, domain.SeverityMedium, domain.ConfidenceMedium, title, detail, c, matches, c.Evidence...)
	}
	if len(unmatched) > 0 && len(unmatched) == len(c.Subjects) {
		title := fmt.Sprintf("No manifest sets the removed field(s) %s", codeList(unmatched, 3))
		detail := fmt.Sprintf("Pruned schema fields only affect resources that set them; your manifests set %d field paths and none of them is the removed path or below it.", len(b.env.ManifestFields))
		b.verdict(RuleCRDFieldUnset, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence,
			[]domain.ImpactCheck{b.manifestsCheck(unmatched)})
	}
}

// --- rule 3: Kubernetes compatibility ------------------------------------------

func (b *builder) compatibilityChecks() {
	toTag, fromTag := b.edge.To.String(), b.edge.From.String()
	for _, cc := range b.edge.Compatibility {
		if cc.To == nil {
			continue
		}
		platform := cc.Platform
		// deciding dimension: the cluster version of this platform. Only the
		// kubernetes platform is collectable today (--kubernetes).
		supplied := b.env.Kubernetes != nil && strings.EqualFold(platform, "kubernetes")
		if !supplied {
			needed := fmt.Sprintf("%s cluster version not supplied", platform)
			hint := "no input collects it (ri impact supports --kubernetes)"
			if strings.EqualFold(platform, "kubernetes") {
				needed = "Kubernetes cluster version (--kubernetes) not supplied"
				hint = "supply it to evaluate support ranges"
			}
			b.verdict(RuleInsufficientVisibility, domain.ImpactUnknown, "compat:"+platform+"/"+compatKind(cc),
				fmt.Sprintf("Cannot evaluate the %s %s constraint without the cluster version", platform, compatKind(cc)),
				fmt.Sprintf("The target declares %s %s = %s. Applicability is decided against the running cluster version; %s.",
					platform, compatKind(cc), cc.To.Raw, hint),
				compatChangeFor(b.edge, cc), constraintEvidence(cc), nil,
				needed)
			continue
		}
		cluster := b.env.Kubernetes.Version
		kind := cc.To.Kind
		if kind == "" {
			kind = "supported"
		}
		chk := upgrade.EvaluatePlatformConstraint(cc.To, cluster)
		if !chk.Computable {
			b.verdict(RuleInsufficientVisibility, domain.ImpactUnknown, "compat:"+platform+"/"+kind,
				fmt.Sprintf("The %s %s constraint is not machine-readable", platform, kind),
				fmt.Sprintf("The target declares %s %s = %q, which cannot be evaluated against cluster %s. Check it manually.", platform, kind, cc.To.Raw, cluster),
				compatChangeFor(b.edge, cc), constraintEvidence(cc), []domain.ImpactCheck{b.clusterCheck(platform, []string{cc.To.Raw})},
				fmt.Sprintf("machine-readable version range for the %s %s constraint %q", platform, kind, cc.To.Raw))
			continue
		}
		var upstream []domain.EvidenceID
		upstream = append(upstream, cc.To.Evidence...)
		if cc.From != nil {
			upstream = append(upstream, cc.From.Evidence...)
		}
		wasAdmitted := false
		if cc.From != nil {
			wasAdmitted = upgrade.EvaluatePlatformConstraint(cc.From, cluster).Admits
		}
		match := domain.ImpactMatch{
			Kind: domain.MatchKubernetes, Subject: cluster, Evidence: b.env.Kubernetes.Evidence,
		}
		change := compatChangeFor(b.edge, cc)
		switch kind {
		case "supported":
			switch {
			case chk.Admits:
				b.add(RuleKubernetesInRange, domain.ImpactInformational, domain.SeverityLow, domain.ConfidenceHigh,
					fmt.Sprintf("Cluster Kubernetes %s is inside the range %s supports (%s)", cluster, toTag, chk.Display),
					fmt.Sprintf("%s supports Kubernetes %s; the cluster version is in range.", toTag, chk.Display),
					change, []domain.ImpactMatch{match}, upstream...)
			case chk.Below != "":
				b.add(RuleKubernetesBelow, domain.ImpactActionRequired, domain.SeverityHigh, domain.ConfidenceHigh,
					fmt.Sprintf("Cluster Kubernetes %s is below the supported range %s of %s", cluster, chk.Display, toTag),
					belowAboveDetail(toTag, fromTag, cluster, chk, wasAdmitted, "at or above "+chk.Below),
					change, []domain.ImpactMatch{match}, upstream...)
			case chk.Above != "":
				b.add(RuleKubernetesAbove, domain.ImpactActionRequired, domain.SeverityHigh, domain.ConfidenceHigh,
					fmt.Sprintf("Cluster Kubernetes %s is above the supported range %s of %s", cluster, chk.Display, toTag),
					belowAboveDetail(toTag, fromTag, cluster, chk, wasAdmitted, "at or below "+chk.Above),
					change, []domain.ImpactMatch{match}, upstream...)
			}
		case "minimum":
			if !chk.Admits {
				b.add(RuleKubernetesBelow, domain.ImpactActionRequired, domain.SeverityHigh, domain.ConfidenceHigh,
					fmt.Sprintf("Cluster Kubernetes %s is below the minimum %s requires (%s)", cluster, toTag, chk.Display),
					belowAboveDetail(toTag, fromTag, cluster, chk, wasAdmitted, chk.Display),
					change, []domain.ImpactMatch{match}, upstream...)
			} else {
				b.compatSatisfied(cc, platform, kind, cluster, chk, change, upstream)
			}
		case "maximum":
			// A cluster above a stated maximum is outside the support matrix
			// just as much as one below a minimum (karpenter-style
			// minK8sVersion/maxK8sVersion columns).
			if !chk.Admits {
				limit := chk.Above
				if limit == "" {
					limit = chk.Display
				}
				b.add(RuleKubernetesAbove, domain.ImpactActionRequired, domain.SeverityHigh, domain.ConfidenceHigh,
					fmt.Sprintf("Cluster Kubernetes %s is above the maximum %s supports (%s)", cluster, toTag, chk.Display),
					fmt.Sprintf("%s supports Kubernetes %s; the cluster runs %s, above the stated maximum. Plan a cluster version at or below %s, or stay on a release that admits it.", toTag, chk.Display, cluster, limit),
					change, []domain.ImpactMatch{match}, upstream...)
			}
		case "chart-kubeVersion":
			if !chk.Admits {
				b.add(RuleKubeVersionBlocked, domain.ImpactActionRequired, domain.SeverityCritical, domain.ConfidenceHigh,
					fmt.Sprintf("Chart kubeVersion %s excludes cluster Kubernetes %s", chk.Display, cluster),
					fmt.Sprintf("Helm refuses to install or upgrade the chart outside its kubeVersion constraint (%s). Upgrade the cluster or stay on a compatible release.", chk.Display),
					change, []domain.ImpactMatch{match}, upstream...)
			} else {
				b.compatSatisfied(cc, platform, kind, cluster, chk, change, upstream)
			}
		case "tested":
			if !chk.Admits {
				b.add(RuleKubernetesUntested, domain.ImpactInformational, domain.SeverityLow, domain.ConfidenceHigh,
					fmt.Sprintf("Cluster Kubernetes %s is not among the versions %s tests (%s)", cluster, toTag, chk.Display),
					fmt.Sprintf("The project's CI covers %s; other versions may work but are untested. No action required by the data.", chk.Display),
					change, []domain.ImpactMatch{match}, upstream...)
			} else {
				b.compatSatisfied(cc, platform, kind, cluster, chk, change, upstream)
			}
		}
	}
}

// compatSatisfied records a checked-and-clear compatibility verdict: the
// supplied cluster version satisfies the constraint, so it does not apply.
func (b *builder) compatSatisfied(cc domain.CompatibilityChange, platform, kind, cluster string, chk upgrade.PlatformVersionCheck, change domain.Change, upstream []domain.EvidenceID) {
	b.verdict(RuleCompatSatisfied, domain.ImpactNotAffected, "compat:"+platform+"/"+kind,
		fmt.Sprintf("Cluster Kubernetes %s satisfies the %s %s constraint (%s)", cluster, platform, kind, chk.Display),
		fmt.Sprintf("The cluster version is inside what %s declares (%s %s = %s), so this constraint requires nothing from you.", b.edge.To.String(), platform, kind, chk.Display),
		change, upstream, []domain.ImpactCheck{b.clusterCheck(platform, []string{cc.To.Raw})})
}

func compatKind(cc domain.CompatibilityChange) string {
	if cc.To != nil && cc.To.Kind != "" {
		return cc.To.Kind
	}
	return "supported"
}

func constraintEvidence(cc domain.CompatibilityChange) []domain.EvidenceID {
	var out []domain.EvidenceID
	out = append(out, cc.To.Evidence...)
	if cc.From != nil {
		out = append(out, cc.From.Evidence...)
	}
	return out
}

func belowAboveDetail(toTag, fromTag, cluster string, chk upgrade.PlatformVersionCheck, wasAdmitted bool, need string) string {
	detail := fmt.Sprintf("%s requires Kubernetes %s; the cluster runs %s. %s.", toTag, chk.Display, cluster, "Plan the cluster upgrade "+need+" before upgrading "+toTag)
	if wasAdmitted {
		detail = fmt.Sprintf("%s supported the cluster's version (%s); %s does not (%s). The support range narrowed under you: upgrade the cluster "+need+" before upgrading.", fromTag, cluster, toTag, chk.Display)
	}
	return detail
}

// compatChangeFor finds the edge change that states this compatibility
// comparison (subjects are "platform/kind"), so the finding can cite both
// the constraint and the change that aggregates it.
func compatChangeFor(e *domain.UpgradeEdge, cc domain.CompatibilityChange) domain.Change {
	kind := cc.To.Kind
	if kind == "" {
		kind = "supported"
	}
	subject := strings.ToLower(cc.Platform) + "/" + kind
	for _, c := range e.Changes {
		for _, s := range c.Subjects {
			if s == subject && c.Category == domain.CategoryCompatibility {
				return c
			}
		}
	}
	return domain.Change{}
}

// --- rule 4: images -------------------------------------------------------------

func (b *builder) imageFamily() {
	toTag := b.edge.To.String()
	for _, c := range b.edge.Changes {
		switch c.Provenance.Rule {
		case upgrade.RuleImageRemoved, upgrade.RuleImageMoved, upgrade.RuleImageTagsChanged:
		default:
			continue
		}
		if len(c.Subjects) == 0 {
			b.subjectLess(c)
			continue
		}
		if !b.imagesVisible() {
			b.verdict(RuleInsufficientVisibility, domain.ImpactUnknown, c.ID,
				fmt.Sprintf("Cannot tell whether the image change affects you: no image references were supplied (%s)", c.Title),
				"Applicability is decided against the images your environment references; none were supplied.",
				c, c.Evidence, nil, "no image references supplied (--images, --values or --manifests)")
			continue
		}
		var unmatched []string
		for _, s := range c.Subjects {
			uses := b.imagesForRepo(s)
			if len(uses) == 0 {
				unmatched = append(unmatched, s)
				continue
			}
			var matches []domain.ImpactMatch
			for _, u := range uses {
				matches = appendUniqueMatches(matches, domain.ImpactMatch{
					Kind: domain.MatchImage, Subject: u.Reference, Evidence: u.Evidence,
				})
			}
			title := fmt.Sprintf("Image you reference changed in %s: %s", toTag, c.Title)
			detail := fmt.Sprintf("Your environment references this repository (as %s). Update mirrors, allow-lists and pinned references as needed.", firstImageRef(uses))
			if strings.TrimSpace(c.Detail) != "" {
				detail = c.Detail + "\nYour environment references this repository (as " + firstImageRef(uses) + ")."
			}
			b.add(RuleImageChanged, domain.ImpactReviewRequired, domain.SeverityMedium, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
		}
		if len(unmatched) > 0 && len(unmatched) == len(c.Subjects) {
			title := fmt.Sprintf("Your environment does not reference %s", codeList(unmatched, 3))
			detail := fmt.Sprintf("The image repositories of this change are not among the %d image reference(s) your environment supplies, so the change does not reach you.", len(b.env.Images))
			b.verdict(RuleImageNotReferenced, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence,
				[]domain.ImpactCheck{b.imagesCheck(unmatched)})
		}
	}
	b.imageArtifacts()
}

// imageArtifacts evaluates container-image artifacts that moved between the
// endpoints (they carry no Change of their own).
func (b *builder) imageArtifacts() {
	for _, ac := range b.edge.Artifacts {
		if ac.Type != domain.ArtifactContainerImage || ac.Change != domain.ChangeUpdated || ac.From == nil || ac.To == nil {
			continue
		}
		fromRef, err1 := parseRef(ac.From.Coordinate)
		toRef, err2 := parseRef(ac.To.Coordinate)
		if err1 != nil || err2 != nil {
			continue
		}
		key := "artifact:" + ac.From.Coordinate + "→" + ac.To.Coordinate
		upstream := append(append([]domain.EvidenceID{}, ac.From.Evidence...), ac.To.Evidence...)
		if !b.imagesVisible() {
			b.verdict(RuleInsufficientVisibility, domain.ImpactUnknown, key,
				fmt.Sprintf("Cannot tell whether the image move affects you: no image references were supplied (%s → %s)", ac.From.Coordinate, ac.To.Coordinate),
				"Applicability is decided against the images your environment references; none were supplied.",
				domain.Change{}, upstream, nil, "no image references supplied (--images, --values or --manifests)")
			continue
		}
		uses := b.imagesForRepo(toRef.Repository)
		if len(uses) == 0 {
			uses = b.imagesForRepo(fromRef.Repository)
		}
		if len(uses) == 0 {
			repos := []string{fromRef.Repository}
			if toRef.Repository != fromRef.Repository {
				repos = append(repos, toRef.Repository)
			}
			b.verdict(RuleImageNotReferenced, domain.ImpactNotAffected, key,
				fmt.Sprintf("Your environment does not reference %s", codeList(repos, 3)),
				fmt.Sprintf("The release moves this image %s → %s; neither repository is among the %d image reference(s) your environment supplies.", ac.From.Coordinate, ac.To.Coordinate, len(b.env.Images)),
				domain.Change{}, upstream, []domain.ImpactCheck{b.imagesCheck([]string{ac.From.Coordinate, ac.To.Coordinate})})
			continue
		}
		var matches []domain.ImpactMatch
		for _, u := range uses {
			matches = appendUniqueMatches(matches, domain.ImpactMatch{
				Kind: domain.MatchImage, Subject: u.Reference, Evidence: u.Evidence,
			})
		}
		pinned := usesTag(uses, fromRef.Tag) || usesTag(uses, toRef.Tag)
		class := domain.ImpactReviewRequired
		sev := domain.SeverityMedium
		if usesTag(uses, toRef.Tag) && !usesTag(uses, fromRef.Tag) {
			class = domain.ImpactInformational
			sev = domain.SeverityLow
		}
		title := fmt.Sprintf("Image you reference moves %s → %s", ac.From.Coordinate, ac.To.Coordinate)
		detail := fmt.Sprintf("The release publishes %s (was %s). ", ac.To.Coordinate, ac.From.Coordinate)
		switch {
		case class == domain.ImpactInformational:
			detail += "Your environment already references the target tag; nothing to change."
		case pinned:
			detail += fmt.Sprintf("Your environment pins the old reference; update it (or your mirror) to pick up %s.", b.edge.To.String())
		default:
			detail += "Your environment references this repository with another tag; if you mirror or allow-list images, add the new reference."
		}
		b.add(RuleImageChanged, class, sev, domain.ConfidenceHigh, title, detail, domain.Change{}, matches, upstream...)
	}
}

func (b *builder) imagesForRepo(repo string) []env.ImageUse {
	var out []env.ImageUse
	for _, u := range b.env.Images {
		if u.Repository == repo {
			out = append(out, u)
		}
	}
	return out
}

func usesTag(uses []env.ImageUse, tag string) bool {
	if tag == "" {
		return false
	}
	for _, u := range uses {
		if u.Tag == tag {
			return true
		}
	}
	return false
}

func firstImageRef(uses []env.ImageUse) string {
	if len(uses) == 0 {
		return ""
	}
	return uses[0].Reference
}

// parseRef parses an artifact coordinate with the shared image parser.
func parseRef(coordinate string) (domain.ImageRef, error) {
	return normalize.ParseImageRef(coordinate)
}

// --- rule 5: everything the join cannot compare ---------------------------------

// unjoinedChanges records an unknown verdict for every change without a
// machine-comparable subject: applicability cannot be decided
// deterministically, and silence would read as "not affected".
func (b *builder) unjoinedChanges() {
	for _, c := range b.edge.Changes {
		if joinRules[c.Provenance.Rule] {
			continue
		}
		var needed string
		if unimplementedDiffRules[c.Provenance.Rule] {
			needed = fmt.Sprintf("diff rule %s has no join rule; whether the change applies to the environment cannot be decided deterministically", c.Provenance.Rule)
		} else {
			needed = "no machine-comparable subject (declared change); the deterministic join evaluates computed values/CRD/image diffs and compatibility constraints only"
		}
		b.verdict(RuleNotJoined, domain.ImpactUnknown, c.ID,
			fmt.Sprintf("Not evaluated for this environment: %s", c.Title),
			"The applicability of this change to your environment cannot be determined deterministically; see what is missing and check it against the upgrade notes yourself.",
			c, c.Evidence, nil, needed)
	}
}

// --- sorting and formatting helpers ---------------------------------------------

var classRank = map[domain.ImpactClass]int{
	domain.ImpactActionRequired: 0, domain.ImpactReviewRequired: 1, domain.ImpactInformational: 2,
	domain.ImpactUnknown: 3, domain.ImpactNotAffected: 4,
}

func sortFindings(fs []domain.ImpactFinding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if classRank[a.Classification] != classRank[b.Classification] {
			return classRank[a.Classification] < classRank[b.Classification]
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.ID < b.ID
	})
}

func appendUnique[T comparable](xs []T, ys ...T) []T {
	for _, y := range ys {
		dup := false
		for _, x := range xs {
			if x == y {
				dup = true
				break
			}
		}
		if !dup {
			xs = append(xs, y)
		}
	}
	return xs
}

// collectEvidence copies the cited subsets, in the order of their sources
// (the edge's pool and the environment's pool respectively).
func (b *builder) collectEvidence() {
	for _, e := range b.edge.Evidence {
		if b.upCited[e.ID] {
			b.rep.Evidence = append(b.rep.Evidence, e)
		}
	}
	for _, e := range b.env.Evidence {
		if b.locCited[e.ID] {
			b.rep.EnvironmentEvidence = append(b.rep.EnvironmentEvidence, e)
		}
	}
}

func (b *builder) summarize() {
	s := domain.ImpactSummary{UpstreamChanges: len(b.edge.Changes), AffectEnvironment: 0}
	for _, f := range b.findings {
		switch f.Classification {
		case domain.ImpactActionRequired:
			s.ActionRequired++
		case domain.ImpactReviewRequired:
			s.ReviewRequired++
		case domain.ImpactInformational:
			s.Informational++
		case domain.ImpactNotAffected:
			s.NotAffected++
		case domain.ImpactUnknown:
			s.Unknown++
		}
		if f.Classification.Affected() {
			s.AffectEnvironment++
		}
	}
	b.rep.Summary = s
}

func appendUniqueMatches(xs []domain.ImpactMatch, m domain.ImpactMatch) []domain.ImpactMatch {
	for _, x := range xs {
		if x.Kind == m.Kind && x.Subject == m.Subject {
			return xs
		}
	}
	return append(xs, m)
}

func apiVersionStrings(usage []env.APIVersionUse) []string {
	out := make([]string, 0, len(usage))
	for _, u := range usage {
		out = append(out, u.GroupVersion+" ("+u.Kind+")")
	}
	return out
}

func code(s string) string { return "`" + s + "`" }

func codeList(items []string, max int) string {
	if len(items) == 0 {
		return ""
	}
	if max > 0 && len(items) > max {
		return code(items[0]) + " and " + fmt.Sprint(len(items)-1) + " more"
	}
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = code(it)
	}
	return strings.Join(parts, ", ")
}

func pluralKeys(exact, partial []string) string {
	n := len(exact) + len(partial)
	if n == 1 {
		return code(firstOf(exact, partial))
	}
	return fmt.Sprintf("%d Helm values", n)
}

func firstOf(exact, partial []string) string {
	if len(exact) > 0 {
		return exact[0]
	}
	if len(partial) > 0 {
		return partial[0]
	}
	return ""
}

// splitBeforeParen strips the " (upstream: …)" annotation from partial keys.
func splitBeforeParen(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		if j := strings.Index(x, " ("); j >= 0 {
			x = x[:j]
		}
		out[i] = x
	}
	return out
}
