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

	b.valuesFindings()
	b.crdFindings()
	b.kubernetesFindings()
	b.imageFindings()

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

// add appends a finding unless an identical one exists. Upstream evidence is
// filtered to ids that resolve in the edge; a finding without any (or
// without matches) is dropped — the join never states anything unevidenced.
func (b *builder) add(rule string, class domain.ImpactClass, conf domain.Confidence, title, detail string,
	change domain.Change, matches []domain.ImpactMatch, upstream ...domain.EvidenceID) {
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
		ID: id, Classification: class, Rule: rule, Title: title, Detail: detail,
		Matches: matches, UpstreamEvidence: up, EnvironmentEvidence: local,
		Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: Producer, Rule: rule, Confidence: conf},
	}
	if change.ID != "" {
		f.ChangeID = change.ID
		f.ChangeTitle = change.Title
		f.ChangeCategory = change.Category
		f.ChangeBreaking = change.Breaking
		f.ChangeActionRequired = change.ActionRequired
	}
	b.findings = append(b.findings, f)
	for _, id := range up {
		b.upCited[id] = true
	}
	for _, id := range local {
		b.locCited[id] = true
	}
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
	s := domain.ImpactSummary{UpstreamChanges: len(b.edge.Changes), AffectEnvironment: len(b.findings)}
	for _, f := range b.findings {
		switch f.Classification {
		case domain.ImpactActionRequired:
			s.ActionRequired++
		case domain.ImpactReview:
			s.Review++
		case domain.ImpactInformational:
			s.Informational++
		}
	}
	b.rep.Summary = s
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

func (b *builder) valuesFindings() {
	if len(b.env.ValuesKeys) == 0 {
		return
	}
	toTag := b.edge.To.String()
	for _, c := range b.edge.Changes {
		kind := valuesDiffKind(c.Provenance.Rule)
		if kind == "" {
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
			b.add(RuleValuesRemoved, domain.ImpactActionRequired, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
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
				b.add(RuleValuesPinned, domain.ImpactInformational, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
			}
			if len(partial) > 0 {
				title := fmt.Sprintf("Default changed under a section you set: %s", codeList(splitBeforeParen(partial), 3))
				detail := fmt.Sprintf("%s changed a default below or above keys you set (%s). Helm merges your mapping with the chart defaults; review the merged result after the upgrade.", toTag, strings.Join(partial, ", "))
				b.add(RuleValuesAdjacent, domain.ImpactReview, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
			}
		case "added":
			title := fmt.Sprintf("Your values already set %s, new in %s", pluralKeys(exact, partial), toTag)
			if len(exact)+len(partial) == 1 {
				title = fmt.Sprintf("Your values already set %s, which %s introduces", firstOf(exact, partial), toTag)
			}
			detail := "A key your values file already set (previously ignored by the chart) becomes live with this release, or sits directly under/over a newly added section. Check that the value you set is what you intend the new key to have."
			b.add(RuleValuesNewKey, domain.ImpactReview, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
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

func (b *builder) crdFindings() {
	if len(b.env.APIVersions) == 0 && len(b.env.CRDs) == 0 && len(b.env.ManifestFields) == 0 {
		return
	}
	toTag := b.edge.To.String()
	for _, c := range b.edge.Changes {
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
	for _, s := range c.Subjects {
		matches, installed, usage := b.crdMatches(s)
		if len(matches) == 0 {
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
			// be the one in use
			conf = domain.ConfidenceMedium
		}
		title := fmt.Sprintf("You use the %s CRD that %s removes", code(s), toTag)
		detail := fmt.Sprintf("%s no longer ships this CustomResourceDefinition; existing resources stop being reconciled. %s. Migrate or delete the resources before upgrading.", toTag, detailWhere)
		b.add(RuleCRDRemoved, domain.ImpactActionRequired, conf, title, detail, c, matches, c.Evidence...)
	}
}

func (b *builder) crdVersionGone(c domain.Change, toTag string) {
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
			continue
		}
		gone := "removed"
		if c.Provenance.Rule == upgrade.RuleCRDVersionUnserved {
			gone = "no longer served"
		}
		title := fmt.Sprintf("You use API version %s, which %s %s", code(gv), toTag, gone)
		detail := fmt.Sprintf("Manifests and clients using %s fail after the upgrade. Migrate them to a version the target still serves.", gv)
		if installed != nil {
			detail += fmt.Sprintf(" Your installed %s declares it.", name)
		}
		b.add(RuleCRDVersionRemoved, domain.ImpactActionRequired, domain.ConfidenceHigh, title, detail, c, narrowed, c.Evidence...)
	}
}

func (b *builder) crdVersionDeprecated(c domain.Change, toTag string) {
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
			continue
		}
		title := fmt.Sprintf("You use API version %s, deprecated as of %s", code(gv), toTag)
		detail := fmt.Sprintf("The API version still works, but %s marks it deprecated; plan the migration before a later release removes it.", toTag)
		b.add(RuleCRDVersionDeprecated, domain.ImpactReview, domain.ConfidenceHigh, title, detail, c, narrowed, c.Evidence...)
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
			continue
		}
		if len(exact)+len(under) > 0 {
			title := fmt.Sprintf("Your manifests set %s, pruned from the CRD schema in %s", codeList(append(exact, under...), 3), toTag)
			detail := fmt.Sprintf("Fields no longer in the schema are pruned from stored objects and rejected or dropped in manifests. Remove them from your resources.\nYou set: %s.",
				strings.Join(append(exact, under...), ", "))
			b.add(RuleCRDFieldRemoved, domain.ImpactActionRequired, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
			continue
		}
		title := fmt.Sprintf("You set a section above %s, pruned from the CRD schema in %s", code(s), toTag)
		detail := fmt.Sprintf("The removed path is below keys you set (%s); whether your resources are affected depends on which sub-fields they use. Compare your manifests with the new schema.", strings.Join(over, ", "))
		b.add(RuleCRDFieldRemoved, domain.ImpactReview, domain.ConfidenceMedium, title, detail, c, matches, c.Evidence...)
	}
}

// --- rule 3: Kubernetes compatibility ------------------------------------------

func (b *builder) kubernetesFindings() {
	if b.env.Kubernetes == nil {
		return
	}
	cluster := b.env.Kubernetes.Version
	toTag, fromTag := b.edge.To.String(), b.edge.From.String()
	for _, cc := range b.edge.Compatibility {
		if !strings.EqualFold(cc.Platform, "kubernetes") || cc.To == nil {
			continue
		}
		kind := cc.To.Kind
		if kind == "" {
			kind = "supported"
		}
		chk := upgrade.EvaluatePlatformConstraint(cc.To, cluster)
		if !chk.Computable {
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
				b.add(RuleKubernetesInRange, domain.ImpactInformational, domain.ConfidenceHigh,
					fmt.Sprintf("Cluster Kubernetes %s is inside the range %s supports (%s)", cluster, toTag, chk.Display),
					fmt.Sprintf("%s supports Kubernetes %s; the cluster version is in range.", toTag, chk.Display),
					change, []domain.ImpactMatch{match}, upstream...)
			case chk.Below != "":
				b.add(RuleKubernetesBelow, domain.ImpactActionRequired, domain.ConfidenceHigh,
					fmt.Sprintf("Cluster Kubernetes %s is below the supported range %s of %s", cluster, chk.Display, toTag),
					belowAboveDetail(toTag, fromTag, cluster, chk, wasAdmitted, "at or above "+chk.Below),
					change, []domain.ImpactMatch{match}, upstream...)
			case chk.Above != "":
				b.add(RuleKubernetesAbove, domain.ImpactActionRequired, domain.ConfidenceHigh,
					fmt.Sprintf("Cluster Kubernetes %s is above the supported range %s of %s", cluster, chk.Display, toTag),
					belowAboveDetail(toTag, fromTag, cluster, chk, wasAdmitted, "at or below "+chk.Above),
					change, []domain.ImpactMatch{match}, upstream...)
			}
		case "minimum":
			if !chk.Admits {
				b.add(RuleKubernetesBelow, domain.ImpactActionRequired, domain.ConfidenceHigh,
					fmt.Sprintf("Cluster Kubernetes %s is below the minimum %s requires (%s)", cluster, toTag, chk.Display),
					belowAboveDetail(toTag, fromTag, cluster, chk, wasAdmitted, chk.Display),
					change, []domain.ImpactMatch{match}, upstream...)
			}
		case "chart-kubeVersion":
			if !chk.Admits {
				b.add(RuleKubeVersionBlocked, domain.ImpactActionRequired, domain.ConfidenceHigh,
					fmt.Sprintf("Chart kubeVersion %s excludes cluster Kubernetes %s", chk.Display, cluster),
					fmt.Sprintf("Helm refuses to install or upgrade the chart outside its kubeVersion constraint (%s). Upgrade the cluster or stay on a compatible release.", chk.Display),
					change, []domain.ImpactMatch{match}, upstream...)
			}
		case "tested":
			if !chk.Admits {
				b.add(RuleKubernetesUntested, domain.ImpactInformational, domain.ConfidenceHigh,
					fmt.Sprintf("Cluster Kubernetes %s is not among the versions %s tests (%s)", cluster, toTag, chk.Display),
					fmt.Sprintf("The project's CI covers %s; other versions may work but are untested. No action required by the data.", chk.Display),
					change, []domain.ImpactMatch{match}, upstream...)
			}
		}
	}
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

func (b *builder) imageFindings() {
	if len(b.env.Images) == 0 {
		return
	}
	toTag := b.edge.To.String()
	for _, c := range b.edge.Changes {
		switch c.Provenance.Rule {
		case upgrade.RuleImageRemoved, upgrade.RuleImageMoved, upgrade.RuleImageTagsChanged:
		default:
			continue
		}
		for _, s := range c.Subjects {
			uses := b.imagesForRepo(s)
			if len(uses) == 0 {
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
			b.add(RuleImageChanged, domain.ImpactReview, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
		}
	}
	// container-image artifacts moving between the endpoints
	for _, ac := range b.edge.Artifacts {
		if ac.Type != domain.ArtifactContainerImage || ac.Change != domain.ChangeUpdated || ac.From == nil || ac.To == nil {
			continue
		}
		fromRef, err1 := parseRef(ac.From.Coordinate)
		toRef, err2 := parseRef(ac.To.Coordinate)
		if err1 != nil || err2 != nil {
			continue
		}
		uses := b.imagesForRepo(toRef.Repository)
		if len(uses) == 0 {
			uses = b.imagesForRepo(fromRef.Repository)
		}
		if len(uses) == 0 {
			continue
		}
		var matches []domain.ImpactMatch
		for _, u := range uses {
			matches = appendUniqueMatches(matches, domain.ImpactMatch{
				Kind: domain.MatchImage, Subject: u.Reference, Evidence: u.Evidence,
			})
		}
		pinned := usesTag(uses, fromRef.Tag) || usesTag(uses, toRef.Tag)
		class := domain.ImpactReview
		rule := RuleImageChanged
		if usesTag(uses, toRef.Tag) && !usesTag(uses, fromRef.Tag) {
			class = domain.ImpactInformational
		}
		title := fmt.Sprintf("Image you reference moves %s → %s", ac.From.Coordinate, ac.To.Coordinate)
		detail := fmt.Sprintf("The release publishes %s (was %s). ", ac.To.Coordinate, ac.From.Coordinate)
		switch {
		case class == domain.ImpactInformational:
			detail += "Your environment already references the target tag; nothing to change."
		case pinned:
			detail += fmt.Sprintf("Your environment pins the old reference; update it (or your mirror) to pick up %s.", toTag)
		default:
			detail += "Your environment references this repository with another tag; if you mirror or allow-list images, add the new reference."
		}
		b.add(rule, class, domain.ConfidenceHigh, title, detail, domain.Change{}, matches, append(append([]domain.EvidenceID{}, ac.From.Evidence...), ac.To.Evidence...)...)
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

// --- sorting and formatting helpers ---------------------------------------------

var classRank = map[domain.ImpactClass]int{
	domain.ImpactActionRequired: 0, domain.ImpactReview: 1, domain.ImpactInformational: 2,
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
