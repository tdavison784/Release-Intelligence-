package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// --- synthetic fixtures (no real case is read except the frozen gates file) ---

const synthInput = `id: demo-1.0-2.0--alpha
product: demo
from: {version: v1.0.0}
to: {version: v2.0.0}
environment: {id: alpha-prod, root: env/, kubernetes: "1.31"}
scope:
  - {id: B1, title: x}
`

const synthExpected = `case: demo-1.0-2.0--alpha
items:
  - item: B1
    classification: affected
    verification: observed
    match:
      - subject: spec.widget.mode
      - text: '(?i)widget mode'
    components:
      - ref: {file: env/apps/web.yaml, kind: Deployment, namespace: web, name: web}
        classification: affected
        requiredEvidence:
          - {file: env/apps/web.yaml, object: Deployment/web/web, field: spec.template.spec.containers}
          - {file: env/apps/other.yaml, field: data.x}
      - ref: {kind: Secret, namespace: web, name: web-tls}
        classification: not-affected
  - item: B2
    classification: not-affected
    verification: render
    match:
      - text: '(?i)gadget'
`

func writeInvCase(t *testing.T, root, input, expected string) string {
	t.Helper()
	dir := filepath.Join(root, "demo-1.0-2.0--alpha")
	if err := os.MkdirAll(filepath.Join(dir, "env", "apps"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"input.yaml": input, "expected.yaml": expected, "env/apps/web.yaml": "kind: Deployment\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func loadSynth(t *testing.T) *InvCase {
	t.Helper()
	root := t.TempDir()
	writeInvCase(t, root, synthInput, synthExpected)
	cs, err := LoadInvCases(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 {
		t.Fatalf("want 1 case, got %d", len(cs))
	}
	return cs[0]
}

// synthRun: B1 is realised by two in-scope items (one matched by subject,
// one by the change detail text) and an out-of-scope item that also matches
// (must be ignored); B2 matches nothing.
func synthRun(classA, classB domain.FindingClass) *InvRun {
	items := []domain.ChangeItem{
		{ID: "ci-a", ChangeID: "c1", Title: "Default changed", Subjects: []string{"Thing.spec.widget.mode"}, InScope: true, ScopeRule: "scope:breaking", Manifestation: domain.ManifestControllerBehaviour},
		{ID: "ci-b", ChangeID: "c2", Title: "Another note", InScope: true, ScopeRule: "scope:breaking", Manifestation: domain.ManifestRenderedConfig},
		{ID: "ci-c", ChangeID: "c3", Title: "widget mode docs", InScope: false},
	}
	rc := domain.RenderedChange{ID: "rc-1", Class: "field-changed", Object: "apps/v1/Deployment//web", Path: "spec.template.spec.containers[0].args",
		ChangeItems: []string{"ci-b"}, Correlation: "subject", Evidence: []domain.EvidenceID{"ev-rd"}}
	b := &domain.EvidenceBundle{ID: "bnd-x",
		Change:   domain.ChangeEvidenceRecord{Items: items},
		Renders:  []domain.ManifestPair{{Scope: domain.RenderScope("environment"), Target: "values-files", Status: domain.CollectionOK, Changes: []domain.RenderedChange{rc}}},
		Manifest: domain.BundleManifest{Artifacts: []domain.ArtifactRef{{ID: "art-web", Path: "env/apps/web.yaml", Digest: "sha256:web"}}},
		Evidence: []domain.Evidence{
			{ID: "ev-web", Kind: domain.EvidenceLocalFile, URI: "apps/web.yaml", Locator: "$.spec.template.spec.containers (L9)", Excerpt: "containers: […]"},
			{ID: "ev-rd", Kind: domain.EvidenceKind("rendered-diff"), URI: "ri:renderdiff@v1/derived/x/diff.json"},
		},
	}
	g := &domain.RelationshipGraph{Nodes: []domain.GraphNode{
		{ID: "gn-cert", Key: "cert-manager.io/v1/Certificate/web/web-tls"},
		{ID: "gn-web", Key: "apps/v1/Deployment/web/web"},
	}}
	inv := &domain.Investigation{ID: "inv-x", Bundle: "bnd-x",
		Traversal: &domain.TraversalResult{Complete: true, Paths: []domain.ImpactPath{{ID: "ip-1", ChangeItem: "ci-a", Nodes: []string{"gn-cert", "gn-web"}}}},
		Findings: []domain.InvestigationFinding{
			{ID: "inf-a", ChangeItem: "ci-a", Class: classA, Rule: "rule-a", Manifestation: domain.ManifestControllerBehaviour,
				Paths: []string{"ip-1"}, Evidence: []domain.EvidenceID{"ev-web"}, Approval: domain.ApprovalEligibility{RequiresHumanReview: true}},
			{ID: "inf-b", ChangeItem: "ci-b", Class: classB, Rule: "rule-b", Manifestation: domain.ManifestRenderedConfig, Basis: "deterministic",
				RenderedChanges: []string{"rc-1"}, Validations: []string{"vr-1"}, Approval: domain.ApprovalEligibility{Eligible: classB == domain.FindingNotAffected, RequiresHumanReview: classB != domain.FindingNotAffected}},
		},
		Validations: []domain.ValidationRecord{{ID: "vr-1", ChangeItem: "ci-b", Inputs: []string{"art-web", "art-missing"}}},
		Decision:    &domain.PolicyDecision{Outcome: domain.PolicyHumanReview},
	}
	ok := true
	return &InvRun{Case: "demo-1.0-2.0--alpha", Investigation: inv, Bundle: b, Graph: g, Reproduced: &ok,
		ChangeText: map[string]string{"c2": "The widget mode is now strict."}}
}

func TestLoadInvCases(t *testing.T) {
	c := loadSynth(t)
	if c.ID != "demo-1.0-2.0--alpha" || len(c.Expected.Items) != 2 || c.EnvDigest == "" || c.ExpectedDigest == "" {
		t.Fatalf("unexpected case: %+v", c)
	}
	req := c.Request(domainConfigZero())
	if req.Product != "demo" || req.From != "v1.0.0" || req.Environment.Name != "alpha-prod" ||
		req.Environment.Repo != filepath.Join(c.Dir, "env/") || req.Environment.KubernetesVersion != "1.31" {
		t.Fatalf("request does not carry the inputs: %+v", req)
	}
	// Selecting one case loads exactly that case, even when others sort after it.
	root := filepath.Dir(c.Dir)
	other := filepath.Join(root, "demo-1.0-2.0--zulu")
	if err := os.MkdirAll(filepath.Join(other, "env"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"input.yaml":    strings.ReplaceAll(synthInput, "--alpha", "--zulu"),
		"expected.yaml": strings.ReplaceAll(synthExpected, "--alpha", "--zulu"),
	} {
		if err := os.WriteFile(filepath.Join(other, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if sel, err := LoadInvCases(root, c.ID); err != nil || len(sel) != 1 || sel[0].ID != c.ID {
		t.Fatalf("-case selection must load only the named case: %v %d", err, len(sel))
	}
	if all, err := LoadInvCases(root); err != nil || len(all) != 2 {
		t.Fatalf("no selection loads every case: %v %d", err, len(all))
	}
	if _, err := LoadInvCases(root, "nope"); err == nil {
		t.Fatal("an unknown case id must be an error")
	}
}

func TestLoadInvCasesRejectsMalformed(t *testing.T) {
	for name, exp := range map[string]string{
		"class":        strings.Replace(synthExpected, "classification: affected\n    verification", "classification: broken\n    verification", 1),
		"verification": strings.Replace(synthExpected, "verification: render", "verification: vibes", 1),
		"match-both":   strings.Replace(synthExpected, "- text: '(?i)gadget'", "- {text: x, subject: y}", 1),
		"match-none":   strings.Replace(synthExpected, "    match:\n      - text: '(?i)gadget'\n", "", 1),
		"regex":        strings.Replace(synthExpected, "'(?i)gadget'", "'(unclosed'", 1),
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeInvCase(t, root, synthInput, exp)
			if _, err := LoadInvCases(root); err == nil {
				t.Fatal("malformed expected.yaml must not load")
			}
		})
	}
}

func TestScoreMatchingStrongestClassAndMiss(t *testing.T) {
	c := loadSynth(t)
	ds, rs := ScoreInvCase(c, synthRun(domain.FindingPotentiallyAffected, domain.FindingAffected))
	if len(ds) != 2 || rs.Error != "" {
		t.Fatalf("got %d decisions, run %+v", len(ds), rs)
	}
	b1 := ds[0]
	if len(b1.Matched) != 2 {
		t.Fatalf("B1 should match ci-a (subject) and ci-b (detail text), not out-of-scope ci-c: %+v", b1.Matched)
	}
	if b1.System != domain.FindingAffected || b1.Decider.ChangeItem != "ci-b" || b1.Basis != BasisDeterministic || !b1.Correct {
		t.Fatalf("strongest matched class decides: %+v", b1)
	}
	if b1.Matched[1].MatchedBy != "subject:spec.widget.mode" || !strings.HasPrefix(b1.Matched[0].MatchedBy, "text:") {
		t.Fatalf("matchedBy not recorded: %+v", b1.Matched)
	}
	b2 := ds[1]
	if !b2.MatchMiss || b2.System != domain.FindingInsufficientEvidence || b2.Manifestation != "unmatched" || !b2.ReviewRequired {
		t.Fatalf("unmatched item is insufficient-evidence + miss: %+v", b2)
	}
}

func TestScoreReachAndEvidence(t *testing.T) {
	c := loadSynth(t)
	ds, _ := ScoreInvCase(c, synthRun(domain.FindingPotentiallyAffected, domain.FindingAffected))
	comps := ds[0].Components
	// Deployment web/web: the rendered change's object has no namespace
	// (release namespace) and matches; the Secret is on no path.
	if !comps[0].Reached || !strings.HasPrefix(comps[0].Via, "rendered-change") {
		t.Fatalf("Deployment should be reached by the rendered change: %+v", comps[0])
	}
	if comps[1].Reached {
		t.Fatalf("Secret is reached by nothing: %+v", comps[1])
	}
	ev := ds[0].Evidence
	if len(ev) != 2 || !ev[0].Covered || !ev[0].FileCited || !ev[0].ObjectNamed || !ev[0].FieldNamed {
		t.Fatalf("local-file evidence on env/apps/web.yaml naming the Deployment and containers covers entry 1: %+v", ev)
	}
	if ev[1].FileCited || ev[1].Covered {
		t.Fatalf("env/apps/other.yaml is cited by nothing: %+v", ev[1])
	}
}

func TestScorePathEndReach(t *testing.T) {
	c := loadSynth(t)
	r := synthRun(domain.FindingPotentiallyAffected, domain.FindingAffected)
	r.Bundle.Renders[0].Changes = nil // only the path can reach now
	r.Investigation.Findings[1].RenderedChanges = nil
	ds, _ := ScoreInvCase(c, r)
	if got := ds[0].Components[0]; !got.Reached || got.Via != "path-end ip-1" {
		t.Fatalf("Deployment ends ip-1: %+v", got)
	}
}

func TestCitationResolutionCountsUnresolved(t *testing.T) {
	c := loadSynth(t)
	_, rs := ScoreInvCase(c, synthRun(domain.FindingPotentiallyAffected, domain.FindingAffected))
	if rs.Citations == rs.CitationsResolved || len(rs.Unresolved) != 1 || !strings.Contains(rs.Unresolved[0], "art-missing") {
		t.Fatalf("one unresolved artifact expected: %+v", rs)
	}
}

func TestRunFailureScoresInsufficient(t *testing.T) {
	c := loadSynth(t)
	s := ScoreInvestigations([]*InvCase{c}, map[string]*InvRun{c.ID: {Case: c.ID, Error: "boom"}}, nil)
	for _, d := range s.Decisions {
		if !d.RunFailed || d.System != domain.FindingInsufficientEvidence {
			t.Fatalf("failed run decision: %+v", d)
		}
	}
	if m := s.Metrics[MPipelineFailures]; m.Num != 1 || m.Den != 1 {
		t.Fatalf("pipelineFailures: %+v", m)
	}
	// A case with no run at all is a failure too.
	s = ScoreInvestigations([]*InvCase{c}, map[string]*InvRun{}, nil)
	if s.Metrics[MPipelineFailures].Num != 1 {
		t.Fatal("missing run must count as a pipeline failure")
	}
}

func invDec(exp, sys domain.FindingClass, verify string) ItemDecision {
	return ItemDecision{Expected: exp, System: sys, Verification: verify, Correct: exp == sys}
}

func TestDecisionMetrics(t *testing.T) {
	A, N, P, I := domain.FindingAffected, domain.FindingNotAffected, domain.FindingPotentiallyAffected, domain.FindingInsufficientEvidence
	ds := []ItemDecision{
		invDec(A, A, VerifyObserved),
		invDec(A, N, VerifyObserved), // the unsafe error
		invDec(A, P, VerifyRender),
		invDec(P, I, VerifyObserved),
		invDec(N, N, VerifyObserved),
		invDec(N, A, VerifyRender),
		invDec(N, I, VerifyObserved),
		invDec(I, I, VerifyObservedAlternatives),
	}
	ds[1].Eligible = true // eligible and wrong
	ds[4].Eligible = true // eligible and right
	ds[0].Components = []ComponentResult{{Classification: A, Reached: true}, {Classification: A}, {Classification: N}}
	ds[0].Evidence = []EvidenceResult{{Covered: true, FileCited: true}, {FileCited: true}, {}}
	ds[7].MatchMiss = true
	m := DecisionMetrics(ds)
	want := map[string][2]int{
		MFalseNegativeRate:       {1, 4}, // expected A/P: 4, system N: 1
		MUnsafeEligibilityRate:   {1, 2},
		MFalsePositiveRate:       {1, 3},
		MAffectedDetection:       {2, 3},
		MAffectedConfirmation:    {1, 3},
		MNotAffectedConfirmation: {1, 3},
		MExposureRecall:          {1, 2},
		MEvidenceCoverage:        {1, 3},
		MEvidenceFileCoverage:    {2, 3},
		MUnresolvedRate:          {1, 6}, // decisive expected (A/N): 6, system I: 1
		MObservedAgreement:       {2, 5},
		MAccuracy:                {3, 8},
		MItemMatchMisses:         {1, 8},
	}
	for name, w := range want {
		if got := m[name]; got.Num != w[0] || got.Den != w[1] {
			t.Errorf("%s = %d/%d, want %d/%d", name, got.Num, got.Den, w[0], w[1])
		}
	}
}

func TestGatesFileLoadsAndEveryGateIsComputed(t *testing.T) {
	g, err := LoadInvGates(filepath.Join("..", "..", "eval", "investigations", InvGatesFileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Gates) < 13 || g.Digest == "" || g.Autonomy.MinObservedAffected == 0 {
		t.Fatalf("gates not loaded: %+v", g)
	}
	m := DecisionMetrics(nil)
	for k, v := range RunMetrics(nil) {
		m[k] = v
	}
	for name := range g.Gates {
		if _, ok := m[name]; !ok {
			t.Errorf("gate %s has no computed metric", name)
		}
	}
}

func TestLoadInvGatesRejects(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"unknown-metric": "gates:\n  vibes: {min: 1, minSample: 1}\n",
		"both":           "gates:\n  accuracy: {min: 1, max: 1, minSample: 1}\n",
		"neither":        "gates:\n  accuracy: {minSample: 1}\n",
		"autonomy":       "gates:\n  accuracy: {min: 1, minSample: 1}\nautonomy:\n  requires: [falseNegativeRate]\n",
	} {
		p := filepath.Join(dir, name+".yaml")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadInvGates(p); err == nil {
			t.Errorf("%s: must not load", name)
		}
	}
}

func invF64(v float64) *float64 { return &v }

func TestEvaluateInvGatesStates(t *testing.T) {
	g := &InvGates{Gates: map[string]InvGate{
		MFalseNegativeRate:       {Max: invF64(0), MinSample: 4},
		MAffectedDetection:       {Min: invF64(0.75), MinSample: 2},
		MNotAffectedConfirmation: {Min: invF64(0.5), MinSample: 2},
		MUnsafeEligibilityRate:   {Max: invF64(0), MinSample: 1},
		MPipelineFailures:        {Max: invF64(0), MinSample: 0},
		MCitationResolution:      {Min: invF64(1), MinSample: 1},
	}}
	g.Autonomy.Requires = []string{MFalseNegativeRate, MPipelineFailures}
	g.Autonomy.MinObservedAffected, g.Autonomy.MinObservedNotAffected = 1, 1
	m := map[string]MetricValue{
		MFalseNegativeRate:       invRatio(MFalseNegativeRate, 0, 3), // below minSample, even though 0
		MAffectedDetection:       invRatio(MAffectedDetection, 3, 4),
		MNotAffectedConfirmation: invRatio(MNotAffectedConfirmation, 0, 5),
		MUnsafeEligibilityRate:   invRatio(MUnsafeEligibilityRate, 0, 0), // nothing eligible: never a pass
		MPipelineFailures:        {Name: MPipelineFailures, Num: 0, Den: 0, Count: true},
		MCitationResolution:      invRatio(MCitationResolution, 9, 10),
	}
	got := map[string]string{}
	for _, r := range EvaluateInvGates(g, m) {
		got[r.Name] = r.State
	}
	want := map[string]string{
		MFalseNegativeRate: GateInsufficientData, MAffectedDetection: GatePass, MNotAffectedConfirmation: GateFail,
		MUnsafeEligibilityRate: GateInsufficientData, MPipelineFailures: GatePass, MCitationResolution: GateFail,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: %s, want %s", k, got[k], w)
		}
	}
	m[MPipelineFailures] = MetricValue{Name: MPipelineFailures, Num: 1, Den: 8, Value: 1, Count: true}
	for _, r := range EvaluateInvGates(g, m) {
		if r.Name == MPipelineFailures && r.State != GateFail {
			t.Errorf("one pipeline failure must FAIL, got %s", r.State)
		}
	}
	gates := EvaluateInvGates(g, m)
	a := EvaluateAutonomy(g, gates, []ItemDecision{invDec(domain.FindingNotAffected, domain.FindingNotAffected, VerifyObserved)})
	if a.Eligible || len(a.Reasons) < 3 {
		t.Fatalf("INSUFFICIENT-DATA is a FAIL for autonomy, and no observed affected decision exists: %+v", a)
	}
	s := &InvScore{Gates: gates}
	if fail, insufficient := s.GateFailures(); fail != 3 || insufficient != 2 {
		t.Fatalf("GateFailures = %d, %d", fail, insufficient)
	}
}

func TestSplitsPartitionDecisions(t *testing.T) {
	A, N := domain.FindingAffected, domain.FindingNotAffected
	ds := []ItemDecision{invDec(A, A, VerifyObserved), invDec(N, N, VerifyRender), invDec(N, A, VerifyObserved)}
	ds[0].Basis, ds[1].Basis, ds[2].Basis = BasisDeterministic, BasisDeterministic, BasisNone
	ds[0].Manifestation, ds[1].Manifestation, ds[2].Manifestation = "rendered-config", "controller-behaviour", "rendered-config"
	total := map[string]int{}
	for _, sp := range Splits(ds) {
		total[sp.By] += sp.N
	}
	for _, by := range []string{"basis", "manifestation", "verification"} {
		if total[by] != len(ds) {
			t.Errorf("split %s covers %d of %d decisions", by, total[by], len(ds))
		}
	}
}

func TestWriteInvTextMarksInsufficientData(t *testing.T) {
	c := loadSynth(t)
	g := &InvGates{Gates: map[string]InvGate{MFalseNegativeRate: {Max: invF64(0), MinSample: 4}}}
	s := ScoreInvestigations([]*InvCase{c}, map[string]*InvRun{c.ID: synthRun(domain.FindingPotentiallyAffected, domain.FindingAffected)}, g)
	var b strings.Builder
	WriteInvText(&b, "t", s)
	out := b.String()
	for _, want := range []string{"INSUFFICIENT-DATA", "n = 2 item decisions", "never a pass", "B2"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q", want)
		}
	}
}

func domainConfigZero() investigate.Config { return investigate.Config{} }

func TestUnmatchedFindingsReported(t *testing.T) {
	c := loadSynth(t)
	r := synthRun(domain.FindingPotentiallyAffected, domain.FindingAffected)
	r.Bundle.Change.Items = append(r.Bundle.Change.Items, domain.ChangeItem{ID: "ci-d", Title: "RBAC split", InScope: true})
	r.Investigation.Findings = append(r.Investigation.Findings, domain.InvestigationFinding{ID: "inf-d", ChangeItem: "ci-d", Class: domain.FindingAffected})
	_, rs := ScoreInvCase(c, r)
	if rs.UnmatchedFindings != 1 || len(rs.UnmatchedAffected) != 1 || !strings.Contains(rs.UnmatchedAffected[0], "RBAC split") {
		t.Fatalf("unmatched affected finding not reported: %+v", rs)
	}
}
