package impact

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// kafkaEdge: an operator's supported operand set narrows (3.8, 3.9 → 3.9, 4.0).
func kafkaEdge() *edgeBuilder {
	eb := newEdge()
	from := eb.ev("https://example/kafka-versions.yaml@from", "3.8, 3.9")
	to := eb.ev("https://example/kafka-versions.yaml@to", "3.9, 4.0")
	prov := domain.Provenance{Method: domain.MethodComputed, Producer: "normalize.table@v1", Confidence: domain.ConfidenceHigh}
	eb.edge.Compatibility = append(eb.edge.Compatibility, domain.CompatibilityChange{Platform: "kafka", Narrowed: true,
		From: &domain.CompatibilityConstraint{Platform: "kafka", Kind: "supported", Versions: []string{"3.8", "3.9"}, Raw: "3.8, 3.9", Provenance: prov, Evidence: []domain.EvidenceID{from}},
		To:   &domain.CompatibilityConstraint{Platform: "kafka", Kind: "supported", Versions: []string{"3.9", "4.0"}, Raw: "3.9, 4.0", Provenance: prov, Evidence: []domain.EvidenceID{to}}})
	return eb
}

func TestPlatformConstraintAgainstInventory(t *testing.T) {
	cases := []struct {
		name      string
		inventory string
		rule      string
		class     domain.ImpactClass
		reason    domain.UnknownReason
	}{
		{"no inventory", "", RuleInsufficientVisibility, domain.ImpactUnknown, domain.UnknownCrossProductContextGap},
		{"dropped version", "- product: kafka\n  version: \"3.8\"\n", RulePlatformOutOfRange, domain.ImpactActionRequired, ""},
		{"supported version", "- product: kafka\n  version: \"3.9\"\n", RulePlatformInRange, domain.ImpactInformational, ""},
		{"not listed, not complete", "- product: strimzi\n  version: 0.45.0\n", RuleInsufficientVisibility, domain.ImpactUnknown, domain.UnknownCrossProductContextGap},
		{"not listed, complete", "complete: true\nproducts:\n  - product: strimzi\n    version: 0.45.0\n", RulePlatformAbsent, domain.ImpactNotAffected, ""},
		{"conflicting versions", "- product: kafka\n  version: \"3.8\"\n- product: kafka\n  version: \"4.0\"\n", RuleInsufficientVisibility, domain.ImpactUnknown, domain.UnknownCrossProductContextGap},
		{"listed without version", "- product: kafka\n", RuleInsufficientVisibility, domain.ImpactUnknown, domain.UnknownCrossProductContextGap},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := env.Inputs{KubernetesVersion: "1.30"}
			if tc.inventory != "" {
				in.Inventory = writeFile(t, t.TempDir(), "inventory.yaml", tc.inventory)
			}
			r := buildReport(t, kafkaEdge().edge, loadEnv(t, in))
			var got []domain.ImpactFinding
			for _, f := range r.Findings {
				if len(f.UpstreamEvidence) > 0 && (f.Rule == tc.rule) {
					got = append(got, f)
				}
			}
			if len(got) != 1 || got[0].Classification != tc.class || got[0].UnknownReason != tc.reason {
				t.Fatalf("want one %s/%s/%q finding, got %+v", tc.rule, tc.class, tc.reason, r.Findings)
			}
		})
	}
}
