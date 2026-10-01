package drift

import (
	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/ingest"
)

// BaselineCutoff returns the newest release the definition's relationships
// were validated against: the newest release of the saved baseline report
// (when given) or of the definition's validatedAgainst / provenance lists.
// Drift is judged on releases NEWER than this; "" means no baseline at all.
func BaselineCutoff(def *catalog.ProductDefinition, base *ingest.RelationshipReport) string {
	var releases []string
	if base != nil {
		releases = append(releases, base.Releases...)
	}
	releases = append(releases, provenanceReleases(def)...)
	releases = append(releases, validatedReleaseList(def)...)
	return maxSemver(releases)
}

// SelectReleases picks the n newest releases strictly newer than cutoff
// (all of them when cutoff is ""), ascending. Explicit known versions can be
// forced with force (used by -versions), which is passed through unchanged
// after validation by the caller.
func SelectReleases(all []domain.Version, cutoff string, n int) []domain.Version {
	if n <= 0 {
		n = 3
	}
	cut := domain.Version{Semver: cutoff}
	var newer []domain.Version
	for _, v := range all {
		if cutoff != "" && v.Compare(cut) <= 0 {
			continue
		}
		newer = append(newer, v)
	}
	if len(newer) > n {
		newer = newer[len(newer)-n:]
	}
	return newer
}

// provenanceReleases returns provenance.validatedReleases.
func provenanceReleases(def *catalog.ProductDefinition) []string {
	if def.Provenance == nil {
		return nil
	}
	return append([]string(nil), def.Provenance.ValidatedReleases...)
}

// validatedReleaseList returns the union of the validatedAgainst lists of
// every source and artifact.
func validatedReleaseList(def *catalog.ProductDefinition) []string {
	var out []string
	for _, s := range def.Sources {
		out = append(out, s.ValidatedAgainst...)
	}
	for _, a := range def.Artifacts {
		out = append(out, a.ValidatedAgainst...)
	}
	return out
}

// maxSemver returns the largest semver string in the list ("" when empty or
// nothing parses).
func maxSemver(list []string) string {
	best := ""
	var bestV *semver.Version
	for _, s := range list {
		sv, err := semver.NewVersion(s)
		if err != nil {
			continue
		}
		if bestV == nil || sv.Compare(bestV) > 0 {
			best, bestV = s, sv
		}
	}
	return best
}

// describeBaseline builds the report's Baseline section.
func describeBaseline(in Input) Baseline {
	b := Baseline{Cutoff: BaselineCutoff(in.Definition, in.Baseline)}
	switch {
	case in.Baseline != nil:
		b.Source = BaselineSavedCheck
		b.Path = in.BaselinePath
		b.Releases = append([]string(nil), in.Baseline.Releases...)
	case len(validatedReleaseList(in.Definition)) > 0 || len(provenanceReleases(in.Definition)) > 0:
		b.Source = BaselineDefinition
		b.Releases = provenanceReleases(in.Definition)
	default:
		b.Source = BaselineNone
	}
	return b
}
