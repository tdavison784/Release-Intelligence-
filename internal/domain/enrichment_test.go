package domain

import (
	"strings"
	"testing"
	"time"
)

// enrichedEdge is a small valid edge: a release note, an upgrade-guide
// statement and a computed Helm diff, consolidated by one cluster.
func enrichedEdge() *UpgradeEdge {
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	note := NewEvidence(EvidenceDocument, "release-notes", "https://example/notes.md", "L10", "Switched the ServiceMonitor to the named port http-metrics.", "", t0)
	guide := NewEvidence(EvidenceDocument, "upgrade-guide", "https://example/upgrade.md", "L3-L5", "prometheus.servicemonitor.targetPort now defaults to http-metrics.", "", t0)
	values := NewEvidence(EvidenceStructured, "chart", "https://example/values.yaml", "values.yaml", "targetPort: http-metrics", "", t0)
	prov := func(m Method) Provenance { return Provenance{Method: m, Producer: "test", Confidence: ConfidenceHigh} }
	gen := t0.Add(time.Hour)
	return &UpgradeEdge{
		From: MustVersion("v1.0.0", "1.0.0"), To: MustVersion("v1.1.0", "1.1.0"),
		Evidence: []Evidence{note, guide, values},
		Changes: []Change{
			{ID: "chg-note", Category: CategoryFeature, Title: "Switched to named port", Provenance: prov(MethodDeclared), Evidence: []EvidenceID{note.ID}},
			{ID: "chg-guide", Category: CategoryMigration, Title: "targetPort default changed", Provenance: prov(MethodDeclared), Evidence: []EvidenceID{guide.ID}},
			{ID: "chg-diff", Category: CategoryHelmValues, Title: "Default of targetPort changed", Subjects: []string{"prometheus.servicemonitor.targetPort"},
				Provenance: prov(MethodComputed), Evidence: []EvidenceID{values.ID}},
		},
		Enrichments: []Enrichment{{
			ID: "enr-1", Kind: EnrichmentCluster, Content: "One change, three sources.",
			RelatesTo: []string{"chg-note", "chg-guide", "chg-diff"},
			Citations: []EvidenceID{note.ID, guide.ID, values.ID},
			Provenance: Provenance{Method: MethodAI, Producer: "enrich@v1", Confidence: ConfidenceMedium, Model: "fake-model", ModelVersion: "fake-model-1",
				PromptVersion: "enrich/v1", PromptDigest: "sha256:abc", InputEvidence: []EvidenceID{note.ID, guide.ID, values.ID}, GeneratedAt: &gen},
		}},
		EnrichmentRun: &EnrichmentRun{Producer: "enrich@v1", PromptVersion: "enrich/v1", CandidateGroups: 1, Requests: 1, Accepted: 1,
			Clusters: 1, ClusteredChanges: 3, DuplicatesConsolidated: 2},
	}
}

func TestEnrichedEdgeValidates(t *testing.T) {
	if err := enrichedEdge().Validate(); err != nil {
		t.Fatal(err)
	}
	c, n := ClusterMetrics(enrichedEdge().Enrichments)
	if c != 1 || n != 3 {
		t.Fatalf("cluster metrics = %d, %d", c, n)
	}
}

func TestEnrichmentInvariants(t *testing.T) {
	cases := []struct {
		name string
		edit func(e *UpgradeEdge)
		want string
	}{
		{"citation outside the input evidence", func(e *UpgradeEdge) {
			e.Enrichments[0].Provenance.InputEvidence = e.Enrichments[0].Provenance.InputEvidence[:2]
		}, "was not part of its input evidence"},
		{"input evidence outside the edge", func(e *UpgradeEdge) {
			p := &e.Enrichments[0].Provenance
			p.InputEvidence = append(p.InputEvidence, "ev-elsewhere")
		}, "input evidence ev-elsewhere is not in the edge"},
		{"citation outside the edge", func(e *UpgradeEdge) {
			e.Enrichments[0].Citations = append(e.Enrichments[0].Citations, "ev-hallucinated")
		}, "cites ev-hallucinated"},
		{"unknown change id", func(e *UpgradeEdge) {
			e.Enrichments[0].RelatesTo = append(e.Enrichments[0].RelatesTo, "chg-hallucinated")
		}, "unknown change chg-hallucinated"},
		{"no citations", func(e *UpgradeEdge) { e.Enrichments[0].Citations = nil }, "cites no evidence"},
		{"blank content", func(e *UpgradeEdge) { e.Enrichments[0].Content = "  " }, "empty content"},
		{"unknown kind", func(e *UpgradeEdge) { e.Enrichments[0].Kind = "risk" }, "unknown kind"},
		{"cluster of one change", func(e *UpgradeEdge) {
			e.Enrichments[0].RelatesTo = e.Enrichments[0].RelatesTo[:1]
			e.EnrichmentRun.ClusteredChanges, e.EnrichmentRun.DuplicatesConsolidated = 1, 0
		}, "at least 2 change(s)"},
		{"related not marked unverified", func(e *UpgradeEdge) {
			e.Enrichments[0].Kind = EnrichmentRelated
			e.EnrichmentRun = nil
		}, "unverified must be set"},
		{"diff explanation without a computed change", func(e *UpgradeEdge) {
			e.Enrichments[0].Kind = EnrichmentDiffExplanation
			e.Enrichments[0].RelatesTo = []string{"chg-note", "chg-guide"}
			e.EnrichmentRun = nil
		}, "at least one computed change"},
		{"change in two clusters", func(e *UpgradeEdge) {
			second := e.Enrichments[0]
			second.ID = "enr-2"
			second.RelatesTo = []string{"chg-note", "chg-diff"}
			e.Enrichments = append(e.Enrichments, second)
			e.EnrichmentRun = nil
		}, "already consolidated by cluster enr-1"},
		{"enrichment id collides with a change", func(e *UpgradeEdge) { e.Changes[0].ID = "enr-1"; e.Enrichments[0].RelatesTo[0] = "enr-1" }, "enrichments never appear as Changes"},
		{"enrichment id without prefix", func(e *UpgradeEdge) { e.Enrichments[0].ID = "x-1" }, "must start with"},
		{"duplicate enrichment id", func(e *UpgradeEdge) {
			e.Enrichments = append(e.Enrichments, e.Enrichments[0])
			e.Enrichments[1].Kind = EnrichmentMigrationSummary
			e.EnrichmentRun = nil
		}, "duplicate id"},
		{"missing model version", func(e *UpgradeEdge) { e.Enrichments[0].Provenance.ModelVersion = "" }, "requires modelVersion"},
		{"missing prompt version", func(e *UpgradeEdge) { e.Enrichments[0].Provenance.PromptVersion = "" }, "requires promptVersion"},
		{"missing generation time", func(e *UpgradeEdge) { e.Enrichments[0].Provenance.GeneratedAt = nil }, "requires generatedAt"},
		{"run metadata out of sync", func(e *UpgradeEdge) { e.EnrichmentRun.DuplicatesConsolidated = 0 }, "does not match"},
		{"AI provenance on a change", func(e *UpgradeEdge) {
			e.Changes[0].Provenance = e.Enrichments[0].Provenance
		}, "must be an Enrichment, not a Change"},
		{"deterministic change with AI fields", func(e *UpgradeEdge) {
			e.Changes[0].Provenance.PromptVersion = "enrich/v1"
		}, "must not carry model/prompt fields"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := enrichedEdge()
			tc.edit(e)
			err := e.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}
