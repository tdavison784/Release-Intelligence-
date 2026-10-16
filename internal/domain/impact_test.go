package domain

import (
	"testing"
	"time"
)

func finding(id string) ImpactFinding {
	up := NewEvidence(EvidenceDocument, "notes", "https://example/notes.md", "L1", "upstream", "", time.Unix(0, 0))
	local := NewEvidence(EvidenceLocalFile, "", "values.yaml", "L3", "replicaCount: 2", "sha256:x", time.Time{})
	return ImpactFinding{
		ID: id, Classification: ImpactActionRequired, Rule: "impact:values-removed",
		Title:    "you set a removed value",
		ChangeID: "chg-x", ChangeTitle: "Helm value removed", ChangeCategory: CategoryHelmValues,
		Matches:             []ImpactMatch{{Kind: MatchValuesKey, Subject: "replicaCount", Evidence: []EvidenceID{local.ID}}},
		UpstreamEvidence:    []EvidenceID{up.ID},
		EnvironmentEvidence: []EvidenceID{local.ID},
		Provenance:          Provenance{Method: MethodComputed, Producer: "impact@v1", Rule: "impact:values-removed", Confidence: ConfidenceHigh},
	}
}

func validReport() *ImpactReport {
	up := NewEvidence(EvidenceDocument, "notes", "https://example/notes.md", "L1", "upstream", "", time.Unix(0, 0))
	local := NewEvidence(EvidenceLocalFile, "", "values.yaml", "L3", "replicaCount: 2", "sha256:x", time.Time{})
	f := finding("imp-1")
	return &ImpactReport{
		SchemaVersion:       ImpactReportSchemaVersion,
		Product:             ProductRef{ID: "p", Name: "P"},
		From:                MustVersion("v1.0.0", "1.0.0"),
		To:                  MustVersion("v1.1.0", "1.1.0"),
		Summary:             ImpactSummary{UpstreamChanges: 3, AffectEnvironment: 1, ActionRequired: 1},
		Findings:            []ImpactFinding{f},
		Evidence:            []Evidence{up},
		EnvironmentEvidence: []Evidence{local},
	}
}

func TestImpactReportValidate(t *testing.T) {
	r := validReport()
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestImpactReportValidateRejectsBrokenChains(t *testing.T) {
	cases := map[string]func(*ImpactReport){
		"no upstream evidence":  func(r *ImpactReport) { r.Findings[0].UpstreamEvidence = nil },
		"no environment match":  func(r *ImpactReport) { r.Findings[0].Matches = nil; r.Findings[0].EnvironmentEvidence = nil },
		"unknown upstream id":   func(r *ImpactReport) { r.Findings[0].UpstreamEvidence = []EvidenceID{"ev-nope"} },
		"unknown local id":      func(r *ImpactReport) { r.Findings[0].EnvironmentEvidence = []EvidenceID{"ev-nope"} },
		"bad classification":    func(r *ImpactReport) { r.Findings[0].Classification = ImpactClass("urgent") },
		"ai provenance":         func(r *ImpactReport) { r.Findings[0].Provenance.Method = MethodAI },
		"summary drift":         func(r *ImpactReport) { r.Summary.ActionRequired = 0 },
		"change title alone":    func(r *ImpactReport) { r.Findings[0].ChangeID = "" },
		"from not below to":     func(r *ImpactReport) { r.From, r.To = r.To, r.From },
		"schema version drift":  func(r *ImpactReport) { r.SchemaVersion = "ri.dev/impact-report/v9" },
		"duplicate local proof": func(r *ImpactReport) { r.EnvironmentEvidence = append(r.EnvironmentEvidence, r.EnvironmentEvidence[0]) },
	}
	for name, mutate := range cases {
		r := validReport()
		mutate(r)
		if err := r.Validate(); err == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

// IdentifiedContent strips the wall-clock stamps (GeneratedAt, evidence
// RetrievedAt) without touching anything the report is identified by:
// evidence ids stay (they never covered RetrievedAt), the caller's report is
// not mutated, and everything else is carried over verbatim.
func TestImpactReportIdentifiedContent(t *testing.T) {
	up := NewEvidence(EvidenceDocument, "notes", "https://example/notes.md", "L1", "upstream", "sha256:u", time.Unix(1700000000, 0))
	local := NewEvidence(EvidenceLocalFile, "", "values.yaml", "L3", "replicaCount: 2", "sha256:x", time.Unix(1700000099, 0))
	r := ImpactReport{
		SchemaVersion:       ImpactReportSchemaVersion,
		GeneratedAt:         time.Unix(1700000500, 0),
		Evidence:            []Evidence{up},
		EnvironmentEvidence: []Evidence{local},
	}
	c := r.IdentifiedContent()
	if !c.GeneratedAt.IsZero() || !c.Evidence[0].RetrievedAt.IsZero() || !c.EnvironmentEvidence[0].RetrievedAt.IsZero() {
		t.Fatalf("stamps survived: generatedAt=%v evidence=%v env=%v", c.GeneratedAt, c.Evidence[0].RetrievedAt, c.EnvironmentEvidence[0].RetrievedAt)
	}
	if c.Evidence[0].ID != up.ID || c.EnvironmentEvidence[0].ID != local.ID {
		t.Fatalf("evidence ids changed: %s / %s", c.Evidence[0].ID, c.EnvironmentEvidence[0].ID)
	}
	if !r.GeneratedAt.Equal(time.Unix(1700000500, 0)) || !r.Evidence[0].RetrievedAt.Equal(time.Unix(1700000000, 0)) || !r.EnvironmentEvidence[0].RetrievedAt.Equal(time.Unix(1700000099, 0)) {
		t.Fatal("IdentifiedContent mutated the original report")
	}
	// nil evidence stays nil (not an empty slice): the copy marshals the same.
	if r.Evidence == nil || r.EnvironmentEvidence == nil {
		t.Fatal("precondition: fixtures must carry evidence")
	}
	var zero ImpactReport
	if z := zero.IdentifiedContent(); z.Evidence != nil || z.EnvironmentEvidence != nil {
		t.Fatal("a report without evidence must not gain empty slices")
	}
}
