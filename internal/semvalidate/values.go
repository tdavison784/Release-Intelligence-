package semvalidate

import (
	"context"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// valuesValidator proves helm-value subjects and changes from the flattened
// Helm values snapshots of both releases (V1; the OCI chart values of V4 arrive
// through the same snapshots).
type valuesValidator struct{}

func (valuesValidator) Name() string { return ProducerValues }

// valuesPair is the values snapshot of one chart on both sides.
type valuesPair struct {
	chart    string
	from, to *domain.ValuesSnapshot
	evidence []domain.Evidence
}

// valuesPairs pairs the snapshots of the artifacts present on both sides; a
// subject Name (the chart) restricts the pairs to that chart or artifact.
func valuesPairs(in knowledge.ValidationInput, chart string) []valuesPair {
	if in.From == nil || in.To == nil {
		return nil
	}
	var out []valuesPair
	for _, ts := range in.To.Snapshots {
		if ts.Kind != domain.SnapshotHelmValues || ts.Values == nil {
			continue
		}
		fs := in.From.Snapshot(domain.SnapshotHelmValues, ts.ArtifactID)
		if fs == nil || fs.Values == nil {
			continue
		}
		if chart != "" && chart != ts.ArtifactID && chart != ts.Values.Chart {
			continue
		}
		ev := append(resolve(fs.Evidence, in.From.Evidence), resolve(ts.Evidence, in.To.Evidence)...)
		out = append(out, valuesPair{chart: ts.Values.Chart, from: fs.Values, to: ts.Values, evidence: ev})
	}
	return out
}

// hasPath reports whether the values contain the key or anything below it
// ("logConfig" is present when "logConfig.enabled" is).
func hasPath(entries map[string]string, p string) bool {
	if _, ok := entries[p]; ok {
		return true
	}
	for k := range entries {
		if strings.HasPrefix(k, p+".") || strings.HasPrefix(k, p+"[") {
			return true
		}
	}
	return false
}

func (valuesValidator) Validate(_ context.Context, in knowledge.ValidationInput) ([]domain.ValidationResult, error) {
	if !applies(in, domain.SubjectHelmValue) {
		return nil, nil
	}
	subj, chg := in.Assertion.Subject, in.Assertion.Change
	pairs := valuesPairs(in, subj.Name)
	if len(pairs) == 0 {
		why := "no Helm values snapshot on both sides"
		if subj.Name != "" {
			why += " for chart " + subj.Name
		}
		return build(in, ProducerValues, map[domain.Aspect]verdict{
			domain.AspectSubject: inconclusive("helm-values:snapshot", "%s", why),
			domain.AspectChange:  inconclusive("helm-values:snapshot", "%s", why),
		}, evidenceSet{}), nil
	}
	// Charts whose keys are rooted differently at the two releases (a wrapper
	// segment such as `defaults.` present at one release only) cannot be
	// compared key by key: a key that merely changed root would read as
	// removed/added. They never confirm or refute.
	var usable, drifted []valuesPair
	for _, p := range pairs {
		if seg := rootDrift(p.from.Entries, p.to.Entries); seg != "" {
			drifted = append(drifted, p)
			continue
		}
		usable = append(usable, p)
	}
	var holders []valuesPair
	for _, p := range usable {
		if hasPath(p.from.Entries, subj.Path) || hasPath(p.to.Entries, subj.Path) {
			holders = append(holders, p)
		}
	}
	if len(holders) == 0 && len(drifted) > 0 {
		why := fmt.Sprintf("the values of chart %s are rooted differently at the two releases (%s…): keys cannot be compared across them", drifted[0].chart, rootDrift(drifted[0].from.Entries, drifted[0].to.Entries)+".")
		v := inconclusive("helm-values:key-root", "%s", why)
		return build(in, ProducerValues, map[domain.Aspect]verdict{domain.AspectSubject: v, domain.AspectChange: v}, evidenceSet{}), nil
	}
	if len(holders) == 0 { // the key is in no chart's values
		pair := usable[0]
		var ev evidenceSet
		ev.add(pair.evidence...)
		vs := map[domain.Aspect]verdict{
			domain.AspectSubject: refuted("helm-values:exists", "%s is in the values of no chart of either release (e.g. %d keys at the source, %d at the target)", subj.Path, len(pair.from.Entries), len(pair.to.Entries)),
			domain.AspectChange:  valuesChange(pair, subj, chg),
		}
		return build(in, ProducerValues, vs, ev), nil
	}
	// the change must hold in every chart that has the key unless the
	// subject names its chart
	var ev evidenceSet
	vs := map[domain.Aspect]verdict{}
	pair := holders[0]
	ev.add(pair.evidence...)
	fHas, tHas := hasPath(pair.from.Entries, subj.Path), hasPath(pair.to.Entries, subj.Path)
	vs[domain.AspectSubject] = confirmed("helm-values:exists", "%s is a values key in %s of chart %s", subj.Path, sides(fHas, tHas), pair.chart)
	vs[domain.AspectChange] = valuesChange(pair, subj, chg)
	for _, other := range holders[1:] {
		o := valuesChange(other, subj, chg)
		if o.outcome != vs[domain.AspectChange].outcome {
			vs[domain.AspectChange] = inconclusive("helm-values:chart-ambiguous", "the change is %s in chart %s but %s in chart %s; name the chart in the subject", vs[domain.AspectChange].outcome, pair.chart, o.outcome, other.chart)
			ev = evidenceSet{}
			ev.add(pair.evidence...)
			ev.add(other.evidence...)
			break
		}
		ev.add(other.evidence...)
	}
	return build(in, ProducerValues, vs, ev), nil
}

// rootDrift returns the leading key segment that (nearly) all keys of one
// side share and the other side lacks — a packaging wrapper such as
// `defaults.` — or "" when the two sides are rooted alike.
func rootDrift(from, to map[string]string) string {
	if seg := dominantRoot(from); seg != "" && share(to, seg) < 0.2 {
		return seg
	}
	if seg := dominantRoot(to); seg != "" && share(from, seg) < 0.2 {
		return seg
	}
	return ""
}

func firstSegment(k string) string {
	if i := strings.IndexAny(k, ".["); i >= 0 {
		return k[:i]
	}
	return k
}

func share(e map[string]string, seg string) float64 {
	if len(e) == 0 {
		return 0
	}
	n := 0
	for k := range e {
		if firstSegment(k) == seg {
			n++
		}
	}
	return float64(n) / float64(len(e))
}

// dominantRoot is the first segment shared by at least 80% of at least five keys.
func dominantRoot(e map[string]string) string {
	if len(e) < 5 {
		return ""
	}
	count := map[string]int{}
	for k := range e {
		count[firstSegment(k)]++
	}
	for seg, n := range count {
		if float64(n) >= 0.8*float64(len(e)) {
			return seg
		}
	}
	return ""
}

func sides(from, to bool) string {
	switch {
	case from && to:
		return "both releases"
	case from:
		return "the source release only"
	}
	return "the target release only"
}

func valuesChange(p valuesPair, subj *domain.Subject, chg *domain.ChangeSpec) verdict {
	fe, te, path := p.from.Entries, p.to.Entries, subj.Path
	fHas, tHas := hasPath(fe, path), hasPath(te, path)
	switch chg.Type {
	case domain.ChangeKindRemoved:
		switch {
		case !fHas:
			return refuted("helm-values:removed", "%s was not in the source values, so nothing was removed", path)
		case tHas:
			return refuted("helm-values:removed", "%s is still in the target values", path)
		}
		return confirmed("helm-values:removed", "%s is in the source values and absent from the target values", path)
	case domain.ChangeKindAdded:
		switch {
		case fHas:
			return refuted("helm-values:added", "%s already exists in the source values", path)
		case !tHas:
			return refuted("helm-values:added", "%s is not in the target values", path)
		}
		return confirmed("helm-values:added", "%s is absent from the source values and present in the target values", path)
	case domain.ChangeKindDefaultChanged, domain.ChangeKindValueChanged:
		fv, fok := fe[path]
		tv, tok := te[path]
		switch {
		case !fHas || !tHas:
			return refuted("helm-values:default", "%s exists in %s only: it was %s, not %s", path, sides(fHas, tHas), addedOrRemoved(fHas), chg.Type)
		case !fok || !tok:
			return inconclusive("helm-values:default", "%s is a section, not a single value", path)
		}
		if !sameValue(ptr(chg.Before), fv) {
			return refuted("helm-values:default", "the source default of %s is %s, not %s", path, fv, ptr(chg.Before))
		}
		if !sameValue(ptr(chg.After), tv) {
			return refuted("helm-values:default", "the target default of %s is %s, not %s", path, tv, ptr(chg.After))
		}
		return confirmed("helm-values:default", "default of %s is %s at the source and %s at the target", path, fv, tv)
	case domain.ChangeKindRenamed:
		newPath := chg.ReplacedBy.Path
		switch {
		case !fHas:
			return refuted("helm-values:renamed", "%s is not in the source values", path)
		case tHas:
			return refuted("helm-values:renamed", "%s is still in the target values", path)
		case hasPath(fe, newPath):
			return refuted("helm-values:renamed", "%s already existed in the source values, so it is not a rename target", newPath)
		case !hasPath(te, newPath):
			return refuted("helm-values:renamed", "the replacement %s is not in the target values", newPath)
		}
		return confirmed("helm-values:renamed", "%s disappears and %s appears", path, newPath)
	case domain.ChangeKindDeprecated:
		if !fHas {
			return refuted("helm-values:deprecated", "%s is not in the source values", path)
		}
		if chg.ReplacedBy != nil && chg.ReplacedBy.Path != "" && !hasPath(te, chg.ReplacedBy.Path) {
			return refuted("helm-values:deprecated", "the replacement %s is not in the target values", chg.ReplacedBy.Path)
		}
		return inconclusive("helm-values:deprecated", "values snapshots cannot prove a deprecation")
	}
	return inconclusive("helm-values:"+string(chg.Type), "a %s change is not decidable from values snapshots", chg.Type)
}

func addedOrRemoved(inFrom bool) string {
	if inFrom {
		return "removed"
	}
	return "added"
}
