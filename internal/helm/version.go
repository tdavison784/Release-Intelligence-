package helm

import "github.com/Masterminds/semver/v3"

// parseVersion parses a chart version leniently ("v1.2.3" and "1.2.3" both
// parse); Helm itself requires semver chart versions, but several charts keep
// a "v" prefix.
func parseVersion(s string) (*semver.Version, bool) {
	v, err := semver.NewVersion(s)
	if err != nil {
		return nil, false
	}
	return v, true
}

// sameVersion reports whether a and b denote the same semantic version
// (including build metadata), ignoring a leading "v".
func sameVersion(a, b string) bool {
	if a == b {
		return true
	}
	va, ok := parseVersion(a)
	if !ok {
		return false
	}
	vb, ok := parseVersion(b)
	if !ok {
		return false
	}
	return va.Equal(vb) && va.Metadata() == vb.Metadata()
}
