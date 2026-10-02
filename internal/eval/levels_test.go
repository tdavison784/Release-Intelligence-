package eval

// The per-verification-level panel (DESIGN.md §7): `ri eval -knowledge` runs
// the dataset once per level and reports each separately, the human level is
// the gate, and the transfer subset excludes links decided by facts that were
// reviewed with that case's environment as context. These tests use a fake
// knowledge pipeline and hand-built facts, never eval expectations.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// fakeKnowledgePipeline records the knowledge runs it is asked for and marks
// its reports with one knowledge finding per fact.
type fakeKnowledgePipeline struct {
	fakePipeline
	runs []struct {
		facts []domain.VerifiedFact
		min   domain.VerificationLevel
	}
}

func (p *fakeKnowledgePipeline) ImpactWithKnowledge(ctx context.Context, product, from, to string, inputs env.Inputs, facts []domain.VerifiedFact, min domain.VerificationLevel) (*domain.ImpactReport, error) {
	p.runs = append(p.runs, struct {
		facts []domain.VerifiedFact
		min   domain.VerificationLevel
	}{facts, min})
	rep := &domain.ImpactReport{Findings: []domain.ImpactFinding{{
		ID: "f-1", Rule: "impact:values-pinned", Classification: domain.ImpactInformational,
		ChangeID: "chg-a",
		Matches:  []domain.ImpactMatch{{Subject: "a"}},
	}}}
	for i, f := range facts {
		rep.Findings = append(rep.Findings, domain.ImpactFinding{
			ID: "f-k" + string(rune('0'+i)), Rule: RuleKnowledgeExposedForTest,
			Classification: domain.ImpactReviewRequired, ChangeID: "chg-a",
			Knowledge: &domain.KnowledgeRef{Fact: f.ID, Verification: f.Level()},
		})
	}
	return rep, nil
}

// RuleKnowledgeExposedForTest is the knowledge rule constant of internal/impact
// (not importable from here without a cycle), spelled for the fake.
const RuleKnowledgeExposedForTest = "impact:knowledge-exposed"

// levelFact builds a fact whose every aspect is at level (Status active, so
// factsAt keeps it).
func levelFact(id string, level domain.VerificationLevel) domain.VerifiedFact {
	f := domain.VerifiedFact{ID: id, Product: "fixture", Release: "v2.0.0", Status: domain.FactActive,
		Assertion: domain.SemanticAssertion{Statement: "fixture fact " + id}}
	for _, x := range domain.Aspects {
		f.Verification = append(f.Verification, domain.AspectVerification{Aspect: x, Level: level, Basis: []string{"rd-" + string(x)}})
	}
	return f
}

// levelCase loads a one-case dataset whose case declares an environment (the
// knowledge branch of runCase runs only for environment-bearing cases).
func levelCase(t *testing.T, r *Runner) []*Case {
	t.Helper()
	cdir := filepath.Join(r.CasesDir, CasesDirName, "c1")
	if err := os.MkdirAll(filepath.Join(cdir, EnvironmentDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cdir, "case.yaml"), []byte(`id: c1
product: fixture
from: v1.0.0
to: v2.0.0
environment:
  kubernetes: "1.30"
expected:
  - {id: E1, title: rotation, kind: behaviour-change, importance: critical, match: [{text: '(?i)rotationPolicy'}]}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cdir, EnvironmentDir, "values.yaml"), []byte("replicas: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases, err := r.LoadAll()
	if err != nil || len(cases) != 1 {
		t.Fatalf("LoadAll: %v %d", err, len(cases))
	}
	return cases
}

func TestRunLevelsRunsEachLevelOnce(t *testing.T) {
	p := &fakeKnowledgePipeline{}
	r := &Runner{Pipeline: p, CasesDir: t.TempDir()}
	cases := levelCase(t, r)
	facts := []domain.VerifiedFact{
		levelFact("vf-det", domain.VerifiedDeterministic),
		levelFact("vf-prx", domain.VerifiedProxy),
	}
	levels := r.RunLevels(context.Background(), cases, facts, nil)
	if len(levels) != len(EvalLevels) {
		t.Fatalf("levels = %d, want %d", len(levels), len(EvalLevels))
	}
	for i, want := range EvalLevels {
		if levels[i].Report.Level != want {
			t.Errorf("level[%d] = %s, want %s", i, levels[i].Report.Level, want)
		}
		if levels[i].Report.Gate != (want == string(GateLevel)) {
			t.Errorf("level %s: gate = %v", want, levels[i].Report.Gate)
		}
	}
	// none uses no facts; each level above uses the facts at or above it
	wantFacts := map[string]int{LevelNone: 0, "deterministic": 1, "human": 1, "consensus": 1, "proxy": 2}
	for _, l := range levels {
		if l.Report.FactsUsed != wantFacts[l.Report.Level] {
			t.Errorf("level %s: factsUsed = %d, want %d", l.Report.Level, l.Report.FactsUsed, wantFacts[l.Report.Level])
		}
	}
	// the none level runs the plain pipeline (no knowledge run), and levels
	// with the same fact set reuse results: only the fact-set changes
	// (none→deterministic, deterministic→proxy) trigger new knowledge runs
	if len(p.runs) != 2 {
		t.Fatalf("knowledge runs = %d, want 2 (none never runs it; equal fact sets are reused)", len(p.runs))
	}
	if got := len(p.runs[0].facts); got != 1 {
		t.Errorf("run 1 facts = %d, want 1", got)
	}
	if p.runs[0].min != domain.VerifiedDeterministic {
		t.Errorf("run 1 min = %s, want deterministic", p.runs[0].min)
	}
	if got := len(p.runs[1].facts); got != 2 {
		t.Errorf("run 2 facts = %d, want 2", got)
	}
	if p.runs[1].min != domain.VerifiedProxy {
		t.Errorf("run 2 min = %s, want proxy", p.runs[1].min)
	}
	// the reused levels still report their own knowledge findings
	if levels[3].Report.KnowledgeFindings != 1 || levels[4].Report.KnowledgeFindings != 2 {
		t.Errorf("knowledge findings by level = %d, %d; want 1, 2", levels[3].Report.KnowledgeFindings, levels[4].Report.KnowledgeFindings)
	}
	// facts below the level never reach the pipeline (non-active facts included)
	retired := levelFact("vf-old", domain.VerifiedHuman)
	retired.Status = domain.FactSuperseded
	got := factsAt(append(facts, retired), "human")
	if len(got) != 1 || got[0].ID != "vf-det" {
		t.Errorf("factsAt human = %v, want [vf-det] (retired and below-level facts drop)", got)
	}
}

func TestRunLevelsWithoutFactsIsTheBaseline(t *testing.T) {
	p := &fakeKnowledgePipeline{}
	r := &Runner{Pipeline: p, CasesDir: t.TempDir()}
	levels := r.RunLevels(context.Background(), levelCase(t, r), nil, nil)
	for _, l := range levels {
		if l.Report.FactsUsed != 0 || l.Report.KnowledgeFindings != 0 {
			t.Errorf("level %s: without facts every level is the baseline (%d facts, %d findings)", l.Report.Level, l.Report.FactsUsed, l.Report.KnowledgeFindings)
		}
	}
	if len(p.runs) != 0 {
		t.Errorf("no fact set ever differs, so the knowledge pipeline never runs: %d runs", len(p.runs))
	}
	// the none level's results are byte-for-byte the plain Run's
	plain := r.Run(context.Background(), levelCase(t, r))
	if len(plain) != len(levels[0].Results) {
		t.Fatalf("none level results = %d, plain run = %d", len(levels[0].Results), len(plain))
	}
	if plain[0].Metrics.Found != levels[0].Results[0].Metrics.Found {
		t.Errorf("none level found = %d, plain run = %d", levels[0].Results[0].Metrics.Found, plain[0].Metrics.Found)
	}
}

func TestKnowledgeCounts(t *testing.T) {
	if got := knowledgeCounts(nil); got != nil {
		t.Errorf("knowledgeCounts(nil) = %v, want nil", got)
	}
	rep := &domain.ImpactReport{Findings: []domain.ImpactFinding{
		{ID: "f-1", Rule: "impact:values-pinned", Classification: domain.ImpactInformational},
		{ID: "f-2", Rule: RuleKnowledgeExposedForTest, Classification: domain.ImpactReviewRequired,
			Knowledge: &domain.KnowledgeRef{Fact: "vf-1", Verification: domain.VerifiedHuman}},
		{ID: "f-3", Rule: RuleKnowledgeExposedForTest, Classification: domain.ImpactActionRequired,
			Knowledge: &domain.KnowledgeRef{Fact: "vf-2", Verification: domain.VerifiedConsensus, ConsensusAction: true}},
		{ID: "f-4", Rule: "impact:knowledge-undecided", Classification: domain.ImpactUnknown,
			Knowledge: &domain.KnowledgeRef{Fact: "vf-3", Verification: domain.VerifiedProxy}},
	}}
	k := knowledgeCounts(rep)
	if k.Findings != 3 {
		t.Errorf("findings = %d, want 3", k.Findings)
	}
	if k.ByClass[string(domain.ImpactReviewRequired)] != 1 || k.ByClass[string(domain.ImpactActionRequired)] != 1 || k.ByClass[string(domain.ImpactUnknown)] != 1 {
		t.Errorf("byClass = %v", k.ByClass)
	}
	if k.ConsensusAction != 1 {
		t.Errorf("consensusAction = %d, want 1 (PO-2: counted separately)", k.ConsensusAction)
	}
}

func TestTransferSubset(t *testing.T) {
	link := func(hit bool, facts ...string) EnvImpactAudit {
		return EnvImpactAudit{Hit: hit, Facts: facts}
	}
	rs := []EntryResult{
		{CaseID: "c1", EnvImpact: []EnvImpactAudit{
			link(true, "vf-reviewed-c1"),  // reviewed with c1's environment: excluded
			link(true, "vf-fresh"),        // transfer: hit
		}},
		{CaseID: "c2", EnvImpact: []EnvImpactAudit{
			link(false, "vf-fresh"),       // transfer: miss
			{Relevance: RelevanceNotAffected, Hit: true}, // a not-affected violation
		}},
	}
	contexts := map[string][]string{"vf-reviewed-c1": {"c1"}, "vf-fresh": nil}
	tm := Transfer(rs, contexts)
	if tm.Excluded != 1 {
		t.Errorf("excluded = %d, want 1", tm.Excluded)
	}
	if tm.ImpactLinks != 2 || tm.ImpactLinksHit != 1 {
		t.Errorf("transfer links = %d/%d, want 1/2", tm.ImpactLinksHit, tm.ImpactLinks)
	}
	if tm.NotAffectedLinks != 1 || tm.NotAffectedViolations != 1 {
		t.Errorf("not-affected = %d links, %d violations; want 1, 1", tm.NotAffectedLinks, tm.NotAffectedViolations)
	}
	// accuracy keeps the not-affected link as a decidable outcome:
	// (1 hit + 1 not-affected kept - 1 violated) / (2 links + 1 not-affected)
	if want := 1.0 / 3.0; tm.ApplicabilityAccuracy != want {
		t.Errorf("transfer applicability = %.3f, want %.3f", tm.ApplicabilityAccuracy, want)
	}
	// a fact reviewed with a DIFFERENT case's environment still transfers
	contexts["vf-fresh"] = []string{"c9"}
	if tm := Transfer(rs, contexts); tm.Excluded != 1 || tm.ImpactLinks != 2 {
		t.Errorf("context of another case must not exclude: %+v", tm)
	}
}
