package impactenrich

import (
	"encoding/json"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// TestForbiddenTransitions: there is no answer the model can give that
// produces an action-required suggestion, an affected classification, a
// deleted or downgraded finding, or AI provenance on a finding.
func TestForbiddenTransitions(t *testing.T) {
	f := newFixture(t)
	cand := firstApplicability(t, f)
	fd := findingByID(t, f, cand.Findings[0])
	ch := f.changes[fd.ChangeID]
	in := newInputs(f.edge, f.report)
	p := buildPrompt(f.report, cand, "", in, f.env)
	g := newAnswerContext(cand, []domain.ImpactFinding{fd}, in, p.input)

	changeCite := string(ch.Evidence[0])

	// 1. the schema cannot express an action-required verdict
	for _, body := range []string{
		`{"verdict":"action-required","content":"x","citations":["` + changeCite + `"],"confidence":"low"}`,
		`{"verdict":"not-affected","content":"x","citations":["` + changeCite + `"],"confidence":"low"}`,
		`{"verdict":"upgrade-to-action-required","content":"x","citations":[],"confidence":"low"}`,
	} {
		if _, err := decode(CandApplicability, body); err == nil {
			t.Errorf("schema accepted a forbidden answer: %s", body)
		}
	}

	// 2. an answer that names a finding to reclassify cannot: applicability
	// answers carry no finding field at all
	var raw map[string]any
	if err := json.Unmarshal([]byte(applicabilityAnswerSchema), &raw); err != nil {
		t.Fatal(err)
	}
	props := raw["properties"].(map[string]any)
	if _, has := props["finding"]; has {
		t.Error("the applicability schema has a finding field: an answer could try to reclassify another finding")
	}
	if _, has := props["classification"]; has {
		t.Error("the applicability schema has a classification field")
	}

	// 3. plausibly-applies without a citation of the change's own evidence is
	// refused (no grounding, no suggestion)
	if _, err := g.checkApplicability(applicabilityAnswer{Verdict: "plausibly-applies", Content: "maybe",
		Citations: []string{"ev-not-shown"}, Confidence: "low"}); err == nil {
		t.Error("citation outside the input evidence was accepted")
	}
	if _, err := g.checkApplicability(applicabilityAnswer{Verdict: "plausibly-applies", Content: "maybe",
		Citations: nil, Confidence: "low"}); err == nil {
		t.Error("plausibly-applies without any cited change evidence was accepted")
	}
	// 4. and the only class an accepted plausibly-applies can produce is the
	// constant: review-required
	if suggestedClass != domain.ImpactReviewRequired {
		t.Errorf("suggestedClass = %q, want review-required", suggestedClass)
	}
	for _, eff := range verdictEffects {
		if eff.kind == domain.EnrichmentPlausiblyApplies && suggestedClass != domain.ImpactReviewRequired {
			t.Error("plausibly-applies does not map onto the review-required constant")
		}
	}
	// 5. empty content is refused in every kind
	if _, err := g.checkApplicability(applicabilityAnswer{Verdict: "undetermined", Content: "   ",
		Citations: []string{changeCite}, Confidence: "low"}); err == nil {
		t.Error("empty content accepted")
	}
	if _, err := g.checkCluster(clusterAnswer{SameChange: true, Content: " "}); err == nil {
		t.Error("cluster with empty content accepted")
	}
}

func TestValidatorClusterRules(t *testing.T) {
	f := newFixture(t)
	var cand Candidate
	for _, c := range Candidates(f.report, f.edge, CandidateOptions{}) {
		if c.Type == CandCluster {
			cand = c
		}
	}
	in := newInputs(f.edge, f.report)
	p := buildPrompt(f.report, cand, "", in, f.env)
	fs := make([]domain.ImpactFinding, 0, len(cand.Findings))
	for _, id := range cand.Findings {
		fs = append(fs, findingByID(t, f, id))
	}
	g := newAnswerContext(cand, fs, in, p.input)

	// a consolidation must cite evidence of every member
	var citeA, citeB string
	for _, id := range cand.Findings {
		ev := f.changes[findingByID(t, f, id).ChangeID].Evidence[0]
		if citeA == "" {
			citeA = string(ev)
			continue
		}
		citeB = string(ev)
	}
	if _, err := g.checkCluster(clusterAnswer{SameChange: true, Content: "one change",
		Citations: []string{citeA}}); err == nil {
		t.Error("cluster with an uncited member was accepted")
	}
	same, err := g.checkCluster(clusterAnswer{SameChange: true, Title: "t", Content: "one change",
		Citations: []string{citeA, citeB}})
	if err != nil || !same {
		t.Fatalf("valid cluster answer rejected: same=%v err=%v", same, err)
	}
	// "not duplicates" is a valid answer that produces no enrichment
	same, err = g.checkCluster(clusterAnswer{SameChange: false, Content: "different things", Citations: []string{citeA}})
	if err != nil || same {
		t.Fatalf("not-duplicates answer rejected: same=%v err=%v", same, err)
	}
}

func TestValidatorMigrationRules(t *testing.T) {
	f := newFixture(t)
	var cand Candidate
	for _, c := range Candidates(f.report, f.edge, CandidateOptions{}) {
		if c.Type == CandMigration {
			cand = c
		}
	}
	in := newInputs(f.edge, f.report)
	p := buildPrompt(f.report, cand, "", in, f.env)
	fd := findingByID(t, f, cand.Findings[0])
	g := newAnswerContext(cand, []domain.ImpactFinding{fd}, in, p.input)
	changeCite := string(f.changes[fd.ChangeID].Evidence[0])

	// steps must cite the finding's own change
	if _, err := g.checkMigration(migrationAnswer{Steps: []string{"do it"}, Citations: []string{"ev-nope"}}); err == nil {
		t.Error("migration citing unknown evidence was accepted")
	}
	if _, err := g.checkMigration(migrationAnswer{Steps: []string{"do it"}, Citations: nil}); err == nil {
		t.Error("migration citing nothing was accepted")
	}
	steps, err := g.checkMigration(migrationAnswer{Steps: []string{"step 1", "step 2"}, Citations: []string{changeCite}})
	if err != nil {
		t.Fatalf("valid migration rejected: %v", err)
	}
	if steps != "1. step 1\n2. step 2" {
		t.Errorf("steps joined = %q", steps)
	}
}

func TestValidateUnusedHelper(t *testing.T) {
	if impactConfidence() != domain.ConfidenceMedium {
		t.Error("AI confidence is not capped at medium")
	}
}
