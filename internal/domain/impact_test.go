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
