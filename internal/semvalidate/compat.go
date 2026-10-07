package semvalidate

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// compatValidator proves compatibility-boundary and product-relationship
// requirements from the release's compatibility rows (the Kubernetes matrix,
// operand tables of V5, a chart's kubeVersion). Ranges are compared as sets of
// version lines through upgrade.EvaluatePlatformConstraint, the one semantic
// representation of a constraint, so "≥1.30" and ">=1.30.0-0" are equal and
// "1.29–1.33" is not "≥1.29".
type compatValidator struct{}

func (compatValidator) Name() string { return ProducerCompat }

// rowsOf returns the rows of a platform that define the supported set
// (supported, minimum, maximum). "tested" rows say what was exercised, not
// what is required; "chart-kubeVersion" is a static install guard, used only
// when it is the sole row.
func rowsOf(r *domain.Release, platform string) (rows []domain.CompatibilityConstraint, evidence []domain.EvidenceID) {
	if r == nil {
		return nil, nil
	}
	var guard []domain.CompatibilityConstraint
	for _, c := range r.Compat {
		if !strings.EqualFold(c.Platform, platform) {
			continue
		}
		switch c.Kind {
		case "", "supported", "minimum", "maximum":
			rows = append(rows, c)
		case "chart-kubeVersion":
			guard = append(guard, c)
		}
	}
	if len(rows) == 0 {
		rows = guard
	}
	for _, c := range rows {
		evidence = append(evidence, c.Evidence...)
	}
	return rows, evidence
}

var lineRe = regexp.MustCompile(`\d+\.\d+`)

type line struct{ major, minor int }

func (l line) String() string { return fmt.Sprintf("%d.%d", l.major, l.minor) }

func linesIn(texts ...string) []line {
	var out []line
	for _, t := range texts {
		for _, m := range lineRe.FindAllString(t, -1) {
			a, b, _ := strings.Cut(m, ".")
			ma, _ := strconv.Atoi(a)
			mi, _ := strconv.Atoi(b)
			out = append(out, line{ma, mi})
		}
	}
	return out
}

// grid is every line between the smallest and largest mentioned, per major,
// padded by two lines on each side, so open-ended sides compare too.
func grid(ls []line) []line {
	span := map[int][2]int{}
	for _, l := range ls {
		s, ok := span[l.major]
		if !ok {
			s = [2]int{l.minor, l.minor}
		}
		if l.minor < s[0] {
			s[0] = l.minor
		}
		if l.minor > s[1] {
			s[1] = l.minor
		}
		span[l.major] = s
	}
	var out []line
	for major, s := range span {
		lo := s[0] - 2
		if lo < 0 {
			lo = 0
		}
		for m := lo; m <= s[1]+2 && m-lo <= 400; m++ {
			out = append(out, line{major, m})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].major != out[j].major {
			return out[i].major < out[j].major
		}
		return out[i].minor < out[j].minor
	})
	return out
}

// admits evaluates the AND of the rows for one line; ok is false when a row
// cannot be evaluated.
func admits(rows []domain.CompatibilityConstraint, l line) (admit, ok bool) {
	admit = true
	for i := range rows {
		r := upgrade.EvaluatePlatformConstraint(&rows[i], l.String())
		if !r.Computable {
			return false, false
		}
		admit = admit && r.Admits
	}
	return admit, true
}

func textsOf(rows []domain.CompatibilityConstraint) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Raw, r.Constraint)
		out = append(out, r.Versions...)
	}
	return out
}

// sameRange compares two row sets over the grid of every line either mentions
// and returns the lines they disagree on.
func sameRange(a, b []domain.CompatibilityConstraint, extra ...string) (equal, computable bool, diff []string) {
	g := grid(linesIn(append(append(textsOf(a), textsOf(b)...), extra...)...))
	equal, computable = true, true
	for _, l := range g {
		x, ok1 := admits(a, l)
		y, ok2 := admits(b, l)
		if !ok1 || !ok2 {
			return false, false, nil
		}
		if x != y {
			equal = false
			diff = append(diff, l.String())
		}
	}
	return equal, computable, diff
}

func (compatValidator) Validate(_ context.Context, in knowledge.ValidationInput) ([]domain.ValidationResult, error) {
	if !applies(in, domain.SubjectCompatibilityBoundary, domain.SubjectProductRelationship) {
		return nil, nil
	}
	subj, chg := in.Assertion.Subject, in.Assertion.Change
	fRows, fEv := rowsOf(in.From, subj.Name)
	tRows, tEv := rowsOf(in.To, subj.Name)
	var ev evidenceSet
	ev.add(resolve(fEv, releaseEvidence(in.From))...)
	ev.add(resolve(tEv, releaseEvidence(in.To))...)

	vs := map[domain.Aspect]verdict{}
	if len(fRows)+len(tRows) > 0 {
		vs[domain.AspectSubject] = confirmed("compatibility:row", "%s has compatibility rows in %s", subj.Name, sides(len(fRows) > 0, len(tRows) > 0))
	} else {
		// prose-only boundaries exist: no row is not a refutation
		vs[domain.AspectSubject] = inconclusive("compatibility:row", "no compatibility row for %s in either release", subj.Name)
	}
	vs[domain.AspectChange] = compatChange(subj, chg, fRows, tRows)
	return build(in, ProducerCompat, vs, ev), nil
}

func compatChange(subj *domain.Subject, chg *domain.ChangeSpec, fRows, tRows []domain.CompatibilityConstraint) verdict {
	if chg.Type != domain.ChangeKindRequirementChanged {
		return inconclusive("compatibility:"+string(chg.Type), "only a requirement-changed change is decidable from compatibility rows")
	}
	if len(tRows) == 0 {
		return inconclusive("compatibility:requirement", "no %s row at the target release", subj.Name)
	}
	after := domain.CompatibilityConstraint{Platform: subj.Name, Kind: "supported", Constraint: ptr(chg.After), Raw: ptr(chg.After)}
	afterRows := []domain.CompatibilityConstraint{after}
	eq, ok, diff := sameRange(tRows, afterRows)
	if !ok {
		return inconclusive("compatibility:requirement", "the target row or %q cannot be evaluated as a version range", ptr(chg.After))
	}
	if !eq {
		return refuted("compatibility:requirement", "the target %s rows admit a different set than %s (they differ on lines %s)", subj.Name, ptr(chg.After), strings.Join(first(diff, 6), ", "))
	}
	if len(fRows) == 0 {
		return inconclusive("compatibility:requirement", "the target requirement is %s, but no source row shows that it moved", ptr(chg.After))
	}
	if chg.Before != nil {
		before := []domain.CompatibilityConstraint{{Platform: subj.Name, Kind: "supported", Constraint: *chg.Before, Raw: *chg.Before}}
		if eq, ok, diff := sameRange(fRows, before); ok && !eq {
			return refuted("compatibility:requirement", "the source %s rows admit a different set than the asserted before %s (lines %s)", subj.Name, *chg.Before, strings.Join(first(diff, 6), ", "))
		}
	}
	unchanged, ok, diff := sameRange(fRows, tRows)
	if !ok {
		return inconclusive("compatibility:requirement", "the source row cannot be evaluated as a version range")
	}
	if unchanged {
		return refuted("compatibility:requirement", "the %s requirement is identical at both releases", subj.Name)
	}
	return confirmed("compatibility:requirement", "the %s requirement moves and the target admits exactly %s (changed lines %s)", subj.Name, ptr(chg.After), strings.Join(first(diff, 6), ", "))
}

func first(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}
