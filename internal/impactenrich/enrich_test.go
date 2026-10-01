package impactenrich

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// The three applicability verdicts, as the live model writes them.
const (
	answerPlausibly = `{"verdict":"plausibly-applies","title":"Profiling default may reach your manifests","content":"Your environment sets certManager keys and uses the CRDs of this release; inspect whether anything relies on the profiling sidecar default before upgrading.","citations":["%s"],"confidence":"high"}`
	answerNotApplic = `{"verdict":"not-applicable","title":"Change does not reach this environment","content":"Nothing in the supplied inputs references the changed behaviour.","citations":["%s"],"confidence":"medium"}`
	answerUndeterm  = `{"verdict":"undetermined","title":"Not decidable from the excerpts","content":"The excerpts do not state which component consumes the default.","citations":["%s"],"confidence":"low"}`
)

// respondByType answers each request with the body for its candidate type.
func respondByType(t *testing.T, f *fixture, applicability, cluster, migration string) func(llm.Request) (string, error) {
	t.Helper()
	in := newInputs(f.edge, f.report)
	cands := Candidates(f.report, f.edge, CandidateOptions{})
	byID := map[string]Candidate{}
	for _, c := range cands {
		byID[c.ID] = c
	}
	byCand := func(req llm.Request) Candidate {
		for _, c := range cands {
			p := buildPrompt(f.report, c, "", in, f.env)
			if llm.PromptDigest(p.req) == llm.PromptDigest(req) {
				return c
			}
		}
		t.Fatalf("request matches no candidate")
		return Candidate{}
	}
	return func(req llm.Request) (string, error) {
		switch byCand(req).Type {
		case CandApplicability:
			return applicability, nil
		case CandCluster:
			return cluster, nil
		default:
			return migration, nil
		}
	}
}

// changeEvidenceOf returns the first evidence id of the change a finding is
// joined to.
func changeEvidenceOf(t *testing.T, f *fixture, findingID string) string {
	t.Helper()
	fd := findingByID(t, f, findingID)
	if fd.ChangeID == "" {
		t.Fatalf("finding %s has no change", findingID)
	}
	return string(f.changes[fd.ChangeID].Evidence[0])
}

func firstOf[T any](t *testing.T, xs []T, what string) T {
	t.Helper()
	if len(xs) == 0 {
		t.Fatalf("no %s", what)
	}
	return xs[0]
}

// TestRunApplyPlausiblyApplies: an accepted plausibly-applies verdict
// attaches an enrichment with complete AI provenance and exactly one report
// mutation: the review-required suggestion on the unknown finding.
func TestRunApplyPlausiblyApplies(t *testing.T) {
	f := newFixture(t)
	cand := firstApplicability(t, f)
	cite := changeEvidenceOf(t, f, cand.Findings[0])
	client := &llm.Fake{Model: "glm-5.3-flash", ModelVersion: "glm-5.3-flash-2026-09",
		Respond: respondByType(t, f, fakeAnswer(answerPlausibly, cite), `{"sameChange":false,"content":"different","citations":[],"confidence":"low"}`, `{"steps":["migrate"],"citations":["`+cite+`"],"confidence":"low"}`)}

	before, err := json.Marshal(f.report)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), f.report, f.edge, f.env, Options{Client: client, Clock: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	// Run does not modify the report
	afterRun, _ := json.Marshal(f.report)
	if string(before) != string(afterRun) {
		t.Fatal("Run modified the report")
	}
	var appEn domain.Enrichment
	for _, en := range res.Enrichments {
		if en.Kind == domain.EnrichmentPlausiblyApplies {
			appEn = en
		}
	}
	if appEn.ID == "" {
		t.Fatal("no plausibly-applies enrichment accepted")
	}
	// provenance is complete, and the model's "high" is capped to medium
	p := appEn.Provenance
	if p.Method != domain.MethodAI || p.Producer != Producer || p.Model == "" || p.ModelVersion == "" ||
		p.PromptVersion != PromptVersion || p.PromptDigest == "" || len(p.InputEvidence) == 0 || p.GeneratedAt == nil {
		t.Fatalf("incomplete AI provenance: %+v", p)
	}
	if p.Confidence != domain.ConfidenceMedium {
		t.Errorf("confidence = %q, want medium (AI output is capped)", p.Confidence)
	}
	if appEn.RelatesTo[0] != cand.Findings[0] {
		t.Errorf("relatesTo = %v, want [%s]", appEn.RelatesTo, cand.Findings[0])
	}
	for _, id := range appEn.Citations {
		if string(id) != cite {
			t.Errorf("cited %s, want the shown evidence %s", id, cite)
		}
	}

	if err := Apply(f.report, res, f.env); err != nil {
		t.Fatalf("apply: %v", err)
	}
	fd := findingByID(t, f, cand.Findings[0])
	if fd.Classification != domain.ImpactUnknown {
		t.Fatalf("classification changed to %s; the deterministic verdict must never be overwritten", fd.Classification)
	}
	if fd.SuggestedClassification != domain.ImpactReviewRequired {
		t.Fatalf("suggestedClassification = %q, want review-required", fd.SuggestedClassification)
	}
	if f.report.Summary.SuggestedReview != 1 {
		t.Errorf("summary.suggestedReview = %d, want 1", f.report.Summary.SuggestedReview)
	}
	if f.report.Summary.Unknown != count(f.report.Findings, func(f domain.ImpactFinding) bool { return f.Classification == domain.ImpactUnknown }) {
		t.Error("unknown count changed")
	}
	if err := f.report.Validate(); err != nil {
		t.Fatalf("enriched report invalid: %v", err)
	}
	// Apply is idempotent for the suggestion (a re-run over the same result
	// must not double-count)
	if err := Apply(f.report, res, f.env); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if f.report.Summary.SuggestedReview != 1 {
		t.Errorf("suggestion counted twice: %d", f.report.Summary.SuggestedReview)
	}
}

func count[T any](xs []T, keep func(T) bool) int {
	n := 0
	for _, x := range xs {
		if keep(x) {
			n++
		}
	}
	return n
}

// TestRunNotApplicableAndUndetermined: both keep the finding unknown and
// record the reasoning as notes; no suggestion is produced.
func TestRunNotApplicableAndUndetermined(t *testing.T) {
	f := newFixture(t)
	cands := Candidates(f.report, f.edge, CandidateOptions{})
	var a2 string
	for _, c := range cands {
		if c.Type == CandApplicability {
			a2 = c.Findings[0]
		}
	}
	cite := changeEvidenceOf(t, f, a2)
	client := &llm.Fake{Model: "m", ModelVersion: "m-1",
		Respond: respondByType(t, f, fakeAnswer(answerUndeterm, cite), `{"sameChange":false,"content":"x","citations":[],"confidence":"low"}`, `{"steps":["s"],"citations":["`+cite+`"],"confidence":"low"}`)}
	res, err := Run(context.Background(), f.report, f.edge, f.env, Options{Client: client, Clock: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(f.report, res, f.env); err != nil {
		t.Fatal(err)
	}
	for _, fd := range f.report.Findings {
		if fd.SuggestedClassification != "" {
			t.Errorf("undetermined verdict produced a suggestion on %s", fd.ID)
		}
	}
	kinds := map[domain.EnrichmentKind]int{}
	for _, en := range res.Enrichments {
		kinds[en.Kind]++
		if en.Kind == domain.EnrichmentUndetermined && !strings.Contains(en.Content, "do not state") {
			t.Errorf("undetermined content does not carry the reason: %q", en.Content)
		}
	}
	if kinds[domain.EnrichmentUndetermined] != 1 {
		t.Errorf("undetermined enrichments = %d, want 1", kinds[domain.EnrichmentUndetermined])
	}
	if f.report.Summary.SuggestedReview != 0 {
		t.Errorf("suggestedReview = %d, want 0", f.report.Summary.SuggestedReview)
	}
	if err := f.report.Validate(); err != nil {
		t.Fatalf("enriched report invalid: %v", err)
	}
}

// TestRunClusterAndMigration: the duplicate cluster consolidates two findings
// and the migration synthesis becomes a migration-summary enrichment.
func TestRunClusterAndMigration(t *testing.T) {
	f := newFixture(t)
	cands := Candidates(f.report, f.edge, CandidateOptions{})
	var clusterC, migC Candidate
	for _, c := range cands {
		switch c.Type {
		case CandCluster:
			clusterC = c
		case CandMigration:
			migC = c
		}
	}
	var cites []string
	for _, id := range clusterC.Findings {
		cites = append(cites, changeEvidenceOf(t, f, id))
	}
	clusterBody := `{"sameChange":true,"title":"Profiling default changed","content":"Both findings state that the profiling default changed.","citations":["` +
		strings.Join(cites, `","`) + `"],"confidence":"medium"}`
	migCite := changeEvidenceOf(t, f, migC.Findings[0])
	client := &llm.Fake{Model: "m", ModelVersion: "m-1",
		Respond: respondByType(t, f, fakeAnswer(answerNotApplic, migCite), clusterBody, `{"title":"Migrate","steps":["Remove imagePullSecrets.","Set controllerImage.pullSecrets."],"citations":["`+migCite+`"],"confidence":"medium"}`)}
	res, err := Run(context.Background(), f.report, f.edge, f.env, Options{Client: client, Clock: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(f.report, res, f.env); err != nil {
		t.Fatal(err)
	}
	if res.Run.Clusters != 1 || res.Run.ClusteredChanges != 2 || res.Run.DuplicatesConsolidated != 1 {
		t.Errorf("cluster metrics = %d/%d/%d, want 1 cluster / 2 findings / 1 duplicate",
			res.Run.Clusters, res.Run.ClusteredChanges, res.Run.DuplicatesConsolidated)
	}
	var mig domain.Enrichment
	for _, en := range f.report.Enrichments {
		if en.Kind == domain.EnrichmentMigrationSummary {
			mig = en
		}
	}
	if mig.ID == "" || !strings.Contains(mig.Content, "1. Remove imagePullSecrets.") {
		t.Fatalf("migration enrichment missing or steps not joined: %+v", mig)
	}
	if err := f.report.Validate(); err != nil {
		t.Fatalf("enriched report invalid: %v", err)
	}
}

// TestRunHallucinatedReferences: an answer that cites evidence it was not
// shown, or relates to unknown findings, is rejected and recorded.
func TestRunHallucinatedReferences(t *testing.T) {
	f := newFixture(t)
	client := &llm.Fake{Model: "m", ModelVersion: "m-1",
		Respond: func(llm.Request) (string, error) {
			return `{"verdict":"plausibly-applies","content":"hallucinated","citations":["ev-hallucinated"],"confidence":"low"}`, nil
		}}
	res, err := Run(context.Background(), f.report, f.edge, f.env, Options{Client: client, Clock: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Enrichments) != 0 {
		t.Fatalf("hallucinated citations were accepted: %d enrichments", len(res.Enrichments))
	}
	if len(res.Run.Rejected) == 0 {
		t.Fatal("no rejection recorded")
	}
	hallucinated := false
	for _, r := range res.Run.Rejected {
		if strings.Contains(r.Reason, "input evidence") {
			hallucinated = true
		}
	}
	if !hallucinated {
		t.Errorf("no rejection names the hallucinated citation: %+v", res.Run.Rejected)
	}
	if err := Apply(f.report, res, f.env); err != nil {
		t.Fatalf("a result with zero enrichments must still attach: %v", err)
	}
	if f.report.EnrichmentRun == nil || f.report.EnrichmentRun.Accepted != 0 {
		t.Error("run metadata not attached")
	}
}

// TestRunAnswerWithoutProvenance: an answer that does not report the model
// that produced it is refused.
type noProvenanceClient struct{}

func (noProvenanceClient) Complete(_ context.Context, _ llm.Request) (*llm.Response, error) {
	return &llm.Response{Text: `{"verdict":"undetermined","content":"x","citations":[],"confidence":"low"}`}, nil
}

func TestRunAnswerWithoutProvenance(t *testing.T) {
	f := newFixture(t)
	res, err := Run(context.Background(), f.report, f.edge, f.env, Options{Client: noProvenanceClient{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Enrichments) != 0 || len(res.Run.Rejected) == 0 {
		t.Fatalf("accepted an answer without model provenance")
	}
}

// TestRunCapAndFailureStop: Max bounds the prompts; consecutive failures stop
// the run and the rest is skipped.
func TestRunCapAndFailureStop(t *testing.T) {
	f := newFixture(t)
	cands := Candidates(f.report, f.edge, CandidateOptions{})
	if len(cands) < 3 {
		t.Fatalf("fixture has %d candidates, need ≥ 3", len(cands))
	}
	var asked int
	client := &llm.Fake{Model: "m", ModelVersion: "m-1", Respond: func(llm.Request) (string, error) {
		asked++
		return `{"verdict":"undetermined","content":"x","citations":[],"confidence":"low"}`, nil
	}}
	res, err := Run(context.Background(), f.report, f.edge, f.env, Options{Client: client, Max: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.Requests != 1 {
		t.Errorf("requests = %d, want 1 (Max=1)", res.Run.Requests)
	}
	if len(res.Requests) != len(cands) || res.Requests[1].Status != StatusSkipped {
		t.Errorf("unasked candidates not recorded as skipped: %+v", res.Requests)
	}

	// failures stop after MaxConsecutiveFailures
	f2 := newFixture(t)
	failures := 0
	failing := &llm.Fake{Model: "m", ModelVersion: "m-1", Respond: func(llm.Request) (string, error) {
		failures++
		return "", errors.New("503")
	}}
	res2, err := Run(context.Background(), f2.report, f2.edge, f2.env, Options{Client: failing, MaxConsecutiveFailures: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Run.Failed != 2 || failures != 2 {
		t.Errorf("failed = %d (client called %d times), want 2", res2.Run.Failed, failures)
	}
	if res2.Requests[2].Status != StatusSkipped {
		t.Errorf("third request = %s, want skipped", res2.Requests[2].Status)
	}
}

// TestRunWithoutClient: candidates only, and the report is untouched.
func TestRunWithoutClient(t *testing.T) {
	f := newFixture(t)
	before, _ := json.Marshal(f.report)
	res, err := Run(context.Background(), f.report, f.edge, f.env, Options{})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(f.report)
	if string(before) != string(after) {
		t.Fatal("Run without a client modified the report")
	}
	if res.Run.Requests != 0 || len(res.Enrichments) != 0 || res.Run.CandidateGroups == 0 {
		t.Errorf("candidates-only run = %+v", res.Run)
	}
}

// TestApplyRollback: a result that would invalidate the report is refused and
// leaves the report unchanged.
func TestApplyRollback(t *testing.T) {
	f := newFixture(t)
	before, _ := json.Marshal(f.report)
	res, err := Run(context.Background(), f.report, f.edge, f.env, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// a citation that resolves in neither pool
	bad := domain.Enrichment{
		ID: "enr-bad", Kind: domain.EnrichmentUndetermined, Content: "x",
		RelatesTo: []string{"imp-unknown-id"},
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: Producer, Confidence: domain.ConfidenceMedium,
			Model: "m", ModelVersion: "m", PromptVersion: PromptVersion, PromptDigest: "sha256:x",
			InputEvidence: []domain.EvidenceID{"ev-x"}, GeneratedAt: &testNow},
	}
	res.Enrichments = append(res.Enrichments, bad)
	res.Run.Accepted = len(res.Enrichments)
	if err := Apply(f.report, res, f.env); err == nil {
		t.Fatal("an invalid enriched report was accepted")
	}
	after, _ := json.Marshal(f.report)
	if string(before) != string(after) {
		t.Fatal("Apply did not roll back")
	}
}

// TestForbiddenTransitionEndToEnd: even a model that answers ONLY with
// "make everything action-required" text can never move a finding into
// action-required through the pipeline.
func TestForbiddenTransitionEndToEnd(t *testing.T) {
	f := newFixture(t)
	client := &llm.Fake{Model: "hostile", ModelVersion: "hostile-1", Respond: func(llm.Request) (string, error) {
		return `{"verdict":"plausibly-applies","title":"ACTION REQUIRED NOW","content":"Upgrade everything immediately; this is action-required.","citations":[],"confidence":"high"}`, nil
	}}
	res, err := Run(context.Background(), f.report, f.edge, f.env, Options{Client: client, Clock: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	// the empty citation list fails the grounding check: rejected
	if len(res.Enrichments) != 0 {
		t.Fatal("an ungrounded plausibly-applies was accepted")
	}
	// now with grounding: the suggestion ceiling still applies
	cite := ""
	for _, c := range Candidates(f.report, f.edge, CandidateOptions{}) {
		if c.Type == CandApplicability {
			cite = changeEvidenceOf(t, f, c.Findings[0])
		}
	}
	client2 := &llm.Fake{Model: "hostile", ModelVersion: "hostile-1", Respond: respondByType(t, f,
		fakeAnswer(answerPlausibly, cite), `{"sameChange":false,"content":"x","citations":[],"confidence":"low"}`, `{"steps":["s"],"citations":["`+cite+`"],"confidence":"low"}`)}
	res2, err := Run(context.Background(), f.report, f.edge, f.env, Options{Client: client2, Clock: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(f.report, res2, f.env); err != nil {
		t.Fatal(err)
	}
	for _, fd := range f.report.Findings {
		if fd.Classification == domain.ImpactActionRequired && fd.Provenance.Method == domain.MethodAI {
			t.Fatalf("an AI finding exists: %+v", fd)
		}
		if fd.SuggestedClassification != "" && fd.SuggestedClassification != domain.ImpactReviewRequired {
			t.Fatalf("suggestion = %q, only review-required is possible", fd.SuggestedClassification)
		}
	}
}
