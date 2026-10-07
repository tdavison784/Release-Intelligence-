package impact

// Compatibility constraints on a platform other than the Kubernetes cluster —
// an operand or peer product such as Kafka for an operator ("Kafka support
// narrowed: 3.8–3.9 → 3.9, 4.0") — are decided against the product inventory
// (env.Environment.Products), with the product-version semantics of
// DESIGN.md §1.3: only application versions decide (a chart version is never
// read as the product's version), conflicting entries decide nothing, and a
// platform missing from the inventory is "not running" only when the
// inventory is declared complete. Generic over the platform name: the
// platform is looked up as an inventory product id.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

const (
	// The inventory runs the platform at a version outside the target's
	// supported set.
	RulePlatformOutOfRange = "impact:platform-out-of-range"
	// The inventory runs the platform at a version inside the supported set.
	RulePlatformInRange = "impact:platform-in-range"
	// The inventory, declared complete, does not run the platform at all.
	RulePlatformAbsent = "impact:platform-absent"
)

// productCompat evaluates one non-Kubernetes compatibility constraint.
func (b *builder) productCompat(cc domain.CompatibilityChange, platform string) {
	kind := compatKind(cc)
	key := "compat:" + platform + "/" + kind
	change := compatChangeFor(b.edge, cc)
	toTag := b.edge.To.String()
	up := constraintEvidence(cc)
	name := upgrade.PlatformName(platform)
	id := strings.ToLower(platform)
	health := b.env.Health(env.DimProducts)
	check := domain.ImpactCheck{Dimension: domain.DimensionProducts, Facts: len(b.env.Products), Subjects: []string{platform + " " + cc.To.Raw}}
	unknown := func(reason domain.UnknownReason, title, needed string) {
		b.unknown(reason, RuleInsufficientVisibility, key, title,
			fmt.Sprintf("The target declares %s %s = %s. Applicability is decided against the %s version this environment runs, read from the product inventory.", name, kind, cc.To.Raw, name),
			change, up, nil, needed)
	}
	if health == env.HealthAbsent {
		unknown(domain.UnknownCrossProductContextGap,
			fmt.Sprintf("Cannot evaluate the %s %s constraint without the %s version you run", name, kind, name),
			fmt.Sprintf("%s version: list it in the product inventory (--inventory)", name))
		return
	}
	all := b.env.ProductInstances(id)
	if len(all) == 0 {
		if b.env.InventoryComplete && health == env.HealthOK {
			check.Evidence = b.env.InventoryCompleteEvidence
			b.verdict(RulePlatformAbsent, domain.ImpactNotAffected, key,
				fmt.Sprintf("%s does not run here, so the %s %s constraint of %s does not apply", name, name, kind, toTag),
				fmt.Sprintf("Your product inventory is declared complete and does not list %s; the constraint (%s) requires nothing from you.", name, cc.To.Raw),
				change, up, []domain.ImpactCheck{check})
			return
		}
		unknown(domain.UnknownCrossProductContextGap,
			fmt.Sprintf("Cannot evaluate the %s %s constraint: %s is not in your product inventory", name, kind, name),
			fmt.Sprintf("%s is not listed in the product inventory, which is not declared complete (absence is not proof that it does not run)", name))
		return
	}
	verdicts := map[bool][]env.ProductInstance{}
	var uncomputable []string
	for _, p := range all {
		if p.VersionOf != env.VersionOfApp || p.Version == "" {
			continue
		}
		chk := upgrade.EvaluatePlatformConstraint(cc.To, p.Version)
		if !chk.Computable {
			uncomputable = append(uncomputable, p.RawVersion)
			continue
		}
		verdicts[chk.Admits] = append(verdicts[chk.Admits], p)
	}
	switch {
	case len(uncomputable) > 0 && len(verdicts) == 0:
		b.unknown(domain.UnknownEvidenceGap, RuleInsufficientVisibility, key,
			fmt.Sprintf("The %s %s constraint is not machine-readable", name, kind),
			fmt.Sprintf("The target declares %s %s = %q, which cannot be evaluated against %s %s. Check it manually.", name, kind, cc.To.Raw, name, strings.Join(uncomputable, ", ")),
			change, up, nil, fmt.Sprintf("machine-readable version range for the %s %s constraint %q", name, kind, cc.To.Raw))
		return
	case len(verdicts) == 2:
		var ns []string
		for _, ps := range verdicts {
			for _, p := range ps {
				ns = append(ns, p.RawVersion+" ("+p.Source+")")
			}
		}
		sort.Strings(ns)
		unknown(domain.UnknownCrossProductContextGap,
			fmt.Sprintf("Cannot evaluate the %s %s constraint: your inventory states conflicting %s versions", name, kind, name),
			fmt.Sprintf("the inventory entries of %s disagree about %s: %s", name, cc.To.Raw, strings.Join(ns, " vs ")))
		return
	case len(verdicts) == 0:
		unknown(domain.UnknownCrossProductContextGap,
			fmt.Sprintf("Cannot evaluate the %s %s constraint: your inventory states no %s application version", name, kind, name),
			fmt.Sprintf("%s application version (the inventory lists it only with a chart version or none)", name))
		return
	}
	var admits bool
	var ps []env.ProductInstance
	for k, v := range verdicts {
		admits, ps = k, v
	}
	var matches []domain.ImpactMatch
	var versions []string
	for _, p := range ps {
		matches = append(matches, domain.ImpactMatch{Kind: domain.MatchProduct, Subject: fmt.Sprintf("%s %s (%s)", p.Product, p.RawVersion, p.Source), Evidence: p.Evidence})
		versions = appendUnique(versions, p.RawVersion)
	}
	running := strings.Join(versions, ", ")
	display := upgrade.EvaluatePlatformConstraint(cc.To, ps[0].Version).Display
	switch {
	case admits && kind == "supported":
		b.add(RulePlatformInRange, domain.ImpactInformational, domain.SeverityLow, domain.ConfidenceHigh,
			fmt.Sprintf("%s %s is inside the range %s supports (%s)", name, running, toTag, display),
			fmt.Sprintf("%s supports %s %s; your inventory runs %s.", toTag, name, display, running),
			change, matches, up...)
	case admits:
		check.Evidence = nil
		for _, p := range ps {
			check.Evidence = appendUnique(check.Evidence, p.Evidence...)
		}
		b.verdict(RuleCompatSatisfied, domain.ImpactNotAffected, key,
			fmt.Sprintf("%s %s satisfies the %s %s constraint (%s)", name, running, name, kind, display),
			fmt.Sprintf("The %s version in your inventory is inside what %s declares, so this constraint requires nothing from you.", name, toTag),
			change, up, []domain.ImpactCheck{check})
	case kind == "tested":
		b.add(RuleKubernetesUntested, domain.ImpactInformational, domain.SeverityLow, domain.ConfidenceHigh,
			fmt.Sprintf("%s %s is not among the versions %s tests (%s)", name, running, toTag, display),
			fmt.Sprintf("The project's CI covers %s %s; other versions may work but are untested.", name, display),
			change, matches, up...)
	default:
		detail := fmt.Sprintf("%s supports %s %s; your inventory runs %s, outside that set. Move %s to a supported version before upgrading, or stay on a release that supports it.", toTag, name, display, running, name)
		if cc.From != nil {
			if chk := upgrade.EvaluatePlatformConstraint(cc.From, ps[0].Version); chk.Computable && chk.Admits {
				detail += fmt.Sprintf(" %s still supported it: the supported set narrowed under you.", b.edge.From.String())
			}
		}
		b.add(RulePlatformOutOfRange, domain.ImpactActionRequired, domain.SeverityHigh, domain.ConfidenceHigh,
			fmt.Sprintf("%s %s is outside the %s versions %s supports (%s)", name, running, name, toTag, display),
			detail, change, matches, up...)
	}
}
