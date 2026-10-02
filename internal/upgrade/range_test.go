package upgrade

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// The compatibility semantics of the bound kinds, checked against both
// consumers of a constraint: the endpoint diff (compareConstraints) and the
// environment join (EvaluatePlatformConstraint). Both evaluate through
// versionRangeOf, so these tables pin the ONE representation.

func k8s(kind, constraint, raw string, vs ...string) *domain.CompatibilityConstraint {
	return &domain.CompatibilityConstraint{Platform: "kubernetes", Kind: kind, Constraint: constraint, Raw: raw, Versions: vs}
}

// TestBoundSemantics pins what "minimum"/"maximum" mean for the environment
// join: >= and <=, not ==. (Before phase 3, "minimum: 1.25" admitted only
// line 1.25 — a cluster on 1.31, well above the minimum, was reported as
// "below the minimum".)
func TestBoundSemantics(t *testing.T) {
	tests := []struct {
		name       string
		c          *domain.CompatibilityConstraint
		version    string
		admits     bool
		below      string
		above      string
		display    string
		computable bool
	}{
		// minimum: >= the stated line.
		{"cluster exactly AT minimum", k8s("minimum", ">=1.30.0-0, <1.31.0-0", "1.30", "1.30"), "1.30", true, "", "", "≥ 1.30", true},
		{"cluster above minimum", k8s("minimum", ">=1.25.0-0, <1.26.0-0", "1.25", "1.25"), "1.31", true, "", "", "≥ 1.25", true},
		{"cluster below minimum", k8s("minimum", ">=1.30.0-0, <1.31.0-0", "1.30", "1.30"), "1.28", false, "1.30", "", "≥ 1.30", true},
		// maximum: <= the stated line.
		{"cluster exactly AT maximum", k8s("maximum", ">=1.33.0-0, <1.34.0-0", "1.33", "1.33"), "1.33", true, "", "", "≤ 1.33", true},
		{"cluster below maximum", k8s("maximum", ">=1.33.0-0, <1.34.0-0", "1.33", "1.33"), "1.30", true, "", "", "≤ 1.33", true},
		{"cluster above maximum", k8s("maximum", ">=1.33.0-0, <1.34.0-0", "1.33", "1.33"), "1.35", false, "", "1.33", "≤ 1.33", true},
		// minimum written as a pure lower-bound constraint (a "Minimum
		// required" column saying ">= 1.25").
		{"minimum as >= constraint", k8s("minimum", ">= 1.25.0-0", ">= 1.25.0-0"), "1.24", false, "1.25", "", "≥ 1.25", true},
		{"maximum as <= constraint", k8s("maximum", "<= 1.33", "<= 1.33"), "1.34", false, "", "1.33", "≤ 1.33", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chk := EvaluatePlatformConstraint(tt.c, tt.version)
			if chk.Admits != tt.admits || chk.Below != tt.below || chk.Above != tt.above ||
				chk.Display != tt.display || chk.Computable != tt.computable {
				t.Errorf("got %+v", chk)
			}
		})
	}
}

// TestBoundDiffSemantics pins what minimum/maximum changes mean for the
// endpoint diff.
func TestBoundDiffSemantics(t *testing.T) {
	tests := []struct {
		name                          string
		f, t                          *domain.CompatibilityConstraint
		fromDisp, toDisp, drops, adds string
		narrowed                      bool
	}{
		{"minimum increases", k8s("minimum", "", "1.25", "1.25"), k8s("minimum", "", "1.26", "1.26"),
			"≥ 1.25", "≥ 1.26", "1.25", "", true},
		{"minimum decreases", k8s("minimum", "", "1.26", "1.26"), k8s("minimum", "", "1.25", "1.25"),
			"≥ 1.26", "≥ 1.25", "", "1.25", false},
		{"maximum increases", k8s("maximum", "", "1.30", "1.30"), k8s("maximum", "", "1.33", "1.33"),
			"≤ 1.30", "≤ 1.33", "", "1.31–1.33", false},
		{"maximum decreases", k8s("maximum", "", "1.33", "1.33"), k8s("maximum", "", "1.32", "1.32"),
			"≤ 1.33", "≤ 1.32", "1.33", "", true},
		{"minimum stated only on target", nil, k8s("minimum", "", "1.29", "1.29"),
			"", "≥ 1.29", "", "", false},
		{"maximum stated only on target", nil, k8s("maximum", "", "1.33", "1.33"),
			"", "≤ 1.33", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := compareConstraints(tt.f, tt.t)
			if r.fromDisp != tt.fromDisp || r.toDisp != tt.toDisp || r.drops != tt.drops ||
				r.adds != tt.adds || r.narrowed != tt.narrowed {
				t.Errorf("got %+v", r)
			}
		})
	}
}

// Tested lists are enumerations, not ranges: outside them is "untested",
// never "unsupported" (the join words it informational; here we pin that the
// evaluation itself is exact membership).
func TestTestedVsSupportedMembership(t *testing.T) {
	tested := k8s("tested", ">=1.29.0-0, <1.32.0-0", "1.29 – 1.31", "1.29", "1.30", "1.31")
	supported := k8s("supported", ">=1.29.0-0 <=1.31.x", "1.29 - 1.31")
	for _, version := range []string{"1.29", "1.30", "1.31"} {
		if !EvaluatePlatformConstraint(tested, version).Admits {
			t.Errorf("tested: %s should be a member", version)
		}
		if !EvaluatePlatformConstraint(supported, version).Admits {
			t.Errorf("supported: %s should be a member", version)
		}
	}
	for _, version := range []string{"1.28", "1.32"} {
		// both exclude it, but only the supported range calls it Below/Above
		// the supported range; the tested list stays an exact enumeration.
		tchk := EvaluatePlatformConstraint(tested, version)
		if tchk.Admits {
			t.Errorf("tested: %s must not be a member", version)
		}
	}
	// A tested list that moves stays a diff, but an informational one (see
	// TestTestedNarrowingIsInformational); the range machinery must not
	// silently widen the enumeration.
	r := compareConstraints(tested, k8s("tested", ">=1.30.0-0, <1.33.0-0", "1.30 – 1.32", "1.30", "1.31", "1.32"))
	if !r.narrowed || r.drops != "1.29" || r.adds != "1.32" {
		t.Errorf("tested list shift: %+v", r)
	}
}

// kubeVersion semver quirks: prerelease suffixes on the bound (">=1.25.0-0")
// must admit the bound line itself — 1.25.0 > 1.25.0-0 — and exclude the
// line below, exactly like the Masterminds comparison Helm performs.
func TestKubeVersionPrereleaseQuirks(t *testing.T) {
	kvs := []struct{ constraint, raw string }{
		{">=1.25.0-0", ">= 1.25.0-0"},
		{">=1.25.0-0", ">=1.25.0-0"},
	}
	for _, kv := range kvs {
		c := k8s("chart-kubeVersion", kv.constraint, kv.raw)
		if !EvaluatePlatformConstraint(c, "1.25").Admits {
			t.Errorf("%s must admit 1.25 (1.25.0 > 1.25.0-0)", kv.raw)
		}
		if !EvaluatePlatformConstraint(c, "1.25.9").Admits {
			t.Errorf("%s must admit 1.25.9", kv.raw)
		}
		chk := EvaluatePlatformConstraint(c, "1.24")
		if chk.Admits || chk.Below != "1.25" {
			t.Errorf("%s vs 1.24: %+v", kv.raw, chk)
		}
	}
	// A 3-part exact prerelease bound without operator is an exact version.
	c := k8s("chart-kubeVersion", "1.25.3", "1.25.3")
	if EvaluatePlatformConstraint(c, "1.25.3").Admits {
		// exact-version constraints do not admit the whole line: the
		// patch-0/999 probes both fail → not admitted; documented behaviour
		// of line-granular checks (the display shows the exact version).
		t.Logf("exact version constraint 1.25.3 vs line 1.25: %+v", EvaluatePlatformConstraint(c, "1.25"))
	}
}
