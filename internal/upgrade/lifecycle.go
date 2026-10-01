package upgrade

import (
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Rules of lifecycle changes.
const (
	RuleLifecycleEndOfLife  = "lifecycle:end-of-life"
	RuleLifecycleDeprecated = "lifecycle:deprecated"
	RuleLifecycleLeavesEOL  = "lifecycle:leaves-end-of-life"
	RuleLifecycleLeavesDepr = "lifecycle:leaves-deprecated"
)

// lifecycleProvenance: the statement is curated in the definition (declared),
// its evidence is the retrieved text of the cited sources.
func lifecycleProvenance(rule string) domain.Provenance {
	return domain.Provenance{Method: domain.MethodDeclared, Producer: Producer, Rule: rule, Confidence: domain.ConfidenceHigh}
}

// lifecycleEvidence returns the evidence of the note items that the sources
// of l produced, taken from the first release that has any.
func lifecycleEvidence(l catalog.Lifecycle, rs []*domain.Release) []domain.EvidenceID {
	for _, r := range rs {
		var ids []domain.EvidenceID
		for _, it := range r.Notes {
			if l.HasSource(it.SourceID) {
				ids = appendUniqueIDs(ids, it.Evidence...)
			}
		}
		if len(ids) > 0 {
			return ids
		}
	}
	return nil
}

// lifecycleChanges reports the lifecycle statements of the definition that
// concern the upgrade: the target is deprecated / end-of-life (action
// required for end-of-life), or the upgrade leaves a deprecated / end-of-life
// release. The note items of the statements' sources are not reported as
// per-release notes (see noteChanges).
func (b *builder) lifecycleChanges() {
	def := b.in.Definition
	if def == nil {
		return
	}
	fromTag, toTag := b.from.Version.String(), b.to.Version.String()
	name := b.productRef().Name
	// evidence is looked up at the target first, then along the path, then at From
	toFirst := append(append([]*domain.Release{b.to}, b.releases...), b.from)
	fromFirst := append([]*domain.Release{b.from}, toFirst...)
	for i, l := range def.Lifecycle {
		fromAff, err1 := l.Affects(b.from.Version)
		toAff, err2 := l.Affects(b.to.Version)
		if err1 != nil || err2 != nil {
			b.warnf("Lifecycle statement %d of the definition has an unusable versions constraint %q", i+1, l.Versions)
			continue
		}
		if !fromAff && !toAff {
			continue
		}
		since := ""
		if l.Since != "" {
			since = " since " + l.Since
		}
		eol := l.State == catalog.LifecycleEndOfLife
		state := "deprecated"
		if eol {
			state = "end-of-life"
		}
		if toAff {
			rule, detail := RuleLifecycleDeprecated, l.Summary
			if eol {
				rule = RuleLifecycleEndOfLife
				detail += " No further releases, bug fixes or security fixes are to be expected for the affected releases: plan the migration instead of relying on upstream fixes."
			}
			b.warnf("Target %s is %s%s: %s", toTag, state, since, shorten(firstLine(l.Summary), 200))
			b.addChange(domain.Change{
				Category:       domain.CategoryDeprecation,
				ActionRequired: eol,
				Title:          fmt.Sprintf("%s %s is %s%s", name, toTag, state, since),
				Detail:         detail,
				Provenance:     lifecycleProvenance(rule),
				Evidence:       lifecycleEvidence(l, toFirst),
			}, rule, fmt.Sprint(i))
			continue
		}
		rule := RuleLifecycleLeavesDepr
		if eol {
			rule = RuleLifecycleLeavesEOL
		}
		b.addChange(domain.Change{
			Category:   domain.CategoryDeprecation,
			Title:      fmt.Sprintf("%s %s is %s%s; the upgrade target %s is not", name, fromTag, state, since, toTag),
			Detail:     l.Summary,
			Provenance: lifecycleProvenance(rule),
			Evidence:   lifecycleEvidence(l, fromFirst),
		}, rule, fmt.Sprint(i))
	}
}
