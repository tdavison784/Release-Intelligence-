package policy

import (
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Image bump levels (ImageMatch.Bump).
const (
	BumpPatch      = "patch"
	BumpMinor      = "minor"
	BumpMajor      = "major"
	BumpDigest     = "digest"     // same tag (or no tag), different digest
	BumpPrerelease = "prerelease" // the target tag is a semver pre-release
	BumpDowngrade  = "downgrade"
	BumpUnknown    = "unknown" // a tag that is not semver, or no change that can be classified
)

// imageRef is a parsed image reference.
type imageRef struct {
	Repository string // registry/path, no tag or digest
	Tag        string
	Digest     string
}

func parseImage(ref string) imageRef {
	r := imageRef{}
	if i := strings.IndexByte(ref, '@'); i >= 0 {
		r.Digest = ref[i+1:]
		ref = ref[:i]
	}
	if i := strings.LastIndexByte(ref, ':'); i >= 0 && !strings.Contains(ref[i:], "/") {
		r.Tag = ref[i+1:]
		ref = ref[:i]
	}
	r.Repository = ref
	return r
}

// imageBump classifies the move from one image reference to another. The
// repository must be the same for any semver level to be reported: a move to a
// different repository is "unknown" here and fails every bump predicate (the
// sameRepository predicate reports it separately).
func imageBump(before, after imageRef) string {
	if before.Repository != after.Repository {
		return BumpUnknown
	}
	if before.Tag == after.Tag {
		if before.Digest != after.Digest {
			return BumpDigest
		}
		return BumpUnknown // identical reference; the diff should not have reported it
	}
	bv, bok := parseTag(before.Tag)
	av, aok := parseTag(after.Tag)
	if !bok || !aok {
		return BumpUnknown
	}
	if av.Prerelease() != "" {
		return BumpPrerelease
	}
	switch c := av.Compare(bv); {
	case c < 0:
		return BumpDowngrade
	case c == 0:
		return BumpUnknown // same version, different spelling: not provably a bump
	}
	switch {
	case av.Major() != bv.Major():
		return BumpMajor
	case av.Minor() != bv.Minor():
		return BumpMinor
	}
	return BumpPatch
}

// parseTag parses a strict semver tag (with an optional leading "v"). Loose
// forms ("1.2", "latest", date tags) are not semver: they classify as unknown
// and so never reach an auto-pass bump rule.
func parseTag(tag string) (*semver.Version, bool) {
	t := strings.TrimPrefix(tag, "v")
	if strings.Count(t, ".") < 2 {
		return nil, false
	}
	v, err := semver.StrictNewVersion(t)
	if err != nil {
		return nil, false
	}
	return v, true
}
