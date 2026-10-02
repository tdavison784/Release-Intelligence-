package impact

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// A "maximum" constraint of the target (a karpenter-style maxK8sVersion
// column) must exclude clusters above it, exactly as a "minimum" excludes
// clusters below it. Before phase 3 the join had no case for "maximum", so
// cluster-above-maximum was silently ignored; "minimum" admitted only the
// bound line itself, so clusters ABOVE the minimum were reported as below it.
func TestMinMaxConstraintsJoin(t *testing.T) {
	tests := []struct {
		name        string
		kind        string
		versions    string
		cluster     string
		wantRule    string
		wantClass   domain.ImpactClass
		wantFirable bool
	}{
		{"cluster at minimum", "minimum", "1.25", "1.25", "", domain.ImpactInformational, false},
		{"cluster above minimum is fine", "minimum", "1.25", "1.31", "", domain.ImpactInformational, false},
		{"cluster below minimum", "minimum", "1.25", "1.24", RuleKubernetesBelow, domain.ImpactActionRequired, true},
		{"cluster at maximum", "maximum", "1.33", "1.33", "", domain.ImpactInformational, false},
		{"cluster below maximum is fine", "maximum", "1.33", "1.30", "", domain.ImpactInformational, false},
		{"cluster above maximum", "maximum", "1.33", "1.35", RuleKubernetesAbove, domain.ImpactActionRequired, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eb := newEdge()
			ev := eb.ev("https://example/compat.md", tt.versions)
			cc := domain.CompatibilityChange{Platform: "kubernetes",
				To: &domain.CompatibilityConstraint{Platform: "kubernetes", Kind: tt.kind,
					Versions: splitVersions(tt.versions), Raw: tt.versions,
					Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.table@v1", Confidence: domain.ConfidenceHigh},
					Evidence:   []domain.EvidenceID{ev}}}
			eb.edge.Compatibility = append(eb.edge.Compatibility, cc)
			rep := buildReport(t, eb.edge, loadEnv(t, env.Inputs{KubernetesVersion: tt.cluster}))
			fs := findingsByRule(rep, tt.wantRule)
			if !tt.wantFirable {
				if len(fs) != 0 {
					t.Fatalf("cluster %s vs %s %s: unexpected findings %+v", tt.cluster, tt.kind, tt.versions, fs)
				}
				return
			}
			if len(fs) != 1 {
				t.Fatalf("cluster %s vs %s %s: findings for %s = %+v (all %+v)", tt.cluster, tt.kind, tt.versions, tt.wantRule, fs, rep.Findings)
			}
			if fs[0].Classification != tt.wantClass {
				t.Errorf("classification = %s, want %s", fs[0].Classification, tt.wantClass)
			}
		})
	}
}
