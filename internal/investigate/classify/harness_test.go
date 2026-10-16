package classify

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// The fixtures describe a FICTIONAL product ("gizmo", controller Deployment
// gizmo/gizmo-controller, API group gizmo.example.io) so the tests assert
// generic policy behaviour, never an evaluation case's answer.

var testNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

const controllerObj = "apps/v1/Deployment/gizmo/gizmo-controller"

type fx struct {
	t        *testing.T
	items    []domain.ChangeItem
	envItems []domain.ChangeItem
	arts     []domain.ArtifactRef
	ev       []domain.Evidence
	pairs    []domain.ManifestPair
	rel      []domain.ManifestPair
	inputs   []domain.SnapshotInput
	sources  map[domain.ChangeSourceKind]domain.ChangeSourceStatus
	up       domain.Evidence
	facts    domain.ArtifactRef
	envEv    domain.Evidence // an environment (local-file) record, the kind a pin cites
	n        int
	b        *domain.EvidenceBundle
}

func newFx(t *testing.T) *fx {
	t.Helper()
	f := &fx{t: t, sources: map[domain.ChangeSourceKind]domain.ChangeSourceStatus{}}
	f.up = domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.invalid/gizmo/releases/v2.0.0", "L1-L3",
		"the default of mode changes from safe to fast", domain.Digest([]byte("notes")), testNow)
	f.ev = append(f.ev, f.up)
	f.inputs = []domain.SnapshotInput{{Kind: domain.InputGitOpsRepo, Path: "./gitops", Status: domain.CollectionOK}}
	f.facts = f.art(domain.ArtifactEnvironmentFacts, domain.OriginDerived, "derived/environment-facts.json", "{}")
	f.envEv = domain.NewEvidence(domain.EvidenceLocalFile, "env", "gitops/values.yaml", "L4", "mode: safe", domain.Digest([]byte("values")), testNow)
	f.ev = append(f.ev, f.envEv)
	return f
}

func (f *fx) art(kind domain.ArtifactKind, origin domain.ArtifactOrigin, path, content string) domain.ArtifactRef {
	d := domain.Digest([]byte(content))
	a := domain.ArtifactRef{ID: domain.ArtifactID(kind, path, d), Kind: kind, MediaType: "application/yaml", Digest: d,
		Size: int64(len(content)), Path: path, Origin: origin, URI: "ri:test/" + path, Producer: "test@v1"}
	f.arts = append(f.arts, a)
	return a
}

// evidence adds an upstream evidence record with the excerpt (the text the
// classifier may read for setting names).
func (f *fx) evidence(excerpt string) domain.EvidenceID {
	e := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.invalid/gizmo/releases/v2.0.0", "L"+excerpt[:3], excerpt, domain.Digest([]byte(excerpt)), testNow)
	f.ev = append(f.ev, e)
	return e.ID
}

// item adds an in-scope change item anchored on a change id (default:
// family values, rendered-config, scope:breaking).
func (f *fx) item(mods ...func(*domain.ChangeItem)) string {
	f.n++
	it := domain.ChangeItem{ChangeID: "chg-" + string(rune('a'+f.n)), Family: domain.FamilyValues, Title: "the default of mode changes",
		Breaking: true, Evidence: []domain.EvidenceID{f.up.ID}, InScope: true, ScopeRule: "scope:breaking",
		Manifestation: domain.ManifestRenderedConfig, ManifestationRule: "manifest:computed-values"}
	for _, m := range mods {
		m(&it)
	}
	it.ID = domain.ChangeItemID(it.ChangeID, it.Platform, it.RenderedChange)
	f.items = append(f.items, it)
	return it.ID
}

func behaviour(it *domain.ChangeItem) {
	it.Family, it.Manifestation, it.ManifestationRule = domain.FamilyBehavior, domain.ManifestControllerBehaviour, "manifest:declared-behaviour"
}

func unknownManifest(it *domain.ChangeItem) {
	it.Family, it.Manifestation, it.ManifestationRule = domain.FamilyBehavior, domain.ManifestUnknown, "manifest:unknown"
}

func family(fam domain.ChangeItemFamily) func(*domain.ChangeItem) {
	return func(it *domain.ChangeItem) { it.Family = fam }
}

func outOfScope(it *domain.ChangeItem) { it.InScope, it.ScopeRule = false, "scope:not-breaking" }

func platform(p string) func(*domain.ChangeItem) {
	return func(it *domain.ChangeItem) {
		it.ChangeID, it.Platform, it.Family, it.ManifestationRule = "", p, domain.FamilyCompatibility, "manifest:computed-compat"
	}
}

func (f *fx) source(k domain.ChangeSourceKind, s domain.CollectionStatus, reason string) {
	st := domain.ChangeSourceStatus{Source: k, Status: s, Reason: reason}
	if s == domain.CollectionOK {
		st.Reason = ""
		st.Artifacts = []string{f.art(domain.ArtifactUpstreamDocument, domain.OriginUpstream, "upstream/gizmo/v2.0.0/"+string(k), string(k)).ID}
	}
	f.sources[k] = st
}

func (f *fx) input(k domain.SnapshotInputKind, path, value string, s domain.CollectionStatus, reason string) {
	f.inputs = append(f.inputs, domain.SnapshotInput{Kind: k, Path: path, Value: value, Status: s, Reason: reason})
}

// rc is a rendered change correlated to the given items (id derived by pair).
func rc(class, object, path string, items ...string) domain.RenderedChange {
	sort.Strings(items)
	c := domain.RenderedChange{Class: class, Object: object, Path: path, ChangeItems: items}
	if len(items) > 0 {
		c.Correlation = "subject"
	}
	return c
}

// pair adds a complete environment render pair; it returns the rendered
// change ids in argument order.
func (f *fx) pair(target string, changes ...domain.RenderedChange) []string {
	dir := "derived/render/env/" + strings.NewReplacer(":", "-", "/", "-").Replace(target)
	fa := f.art(domain.ArtifactRenderedManifests, domain.OriginDerived, dir+"/from.yaml", "from "+target)
	ta := f.art(domain.ArtifactRenderedManifests, domain.OriginDerived, dir+"/to.yaml", "to "+target)
	da := f.art(domain.ArtifactRenderDiff, domain.OriginDerived, dir+"/diff.json", "diff "+target)
	var ids []string
	for i := range changes {
		changes[i].ID = domain.RenderedChangeID(domain.RenderEnvironment, target, changes[i].Object, changes[i].Path, changes[i].Class)
		ev := domain.NewEvidence(domain.EvidenceRenderedDiff, "render", "ri:renderdiff", "changes[id="+changes[i].ID+"]", "", da.Digest, testNow)
		f.ev = append(f.ev, ev)
		changes[i].Evidence = []domain.EvidenceID{ev.ID}
		ids = append(ids, changes[i].ID)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].ID < changes[j].ID })
	f.pairs = append(f.pairs, domain.ManifestPair{Scope: domain.RenderEnvironment, Target: target, From: fa.ID, To: ta.ID, Diff: da.ID,
		Status: domain.CollectionOK, Changes: changes})
	return ids
}

func (f *fx) naPair(target string) {
	f.pairs = append(f.pairs, domain.ManifestPair{Scope: domain.RenderEnvironment, Target: target, Status: domain.CollectionNotApplicable, Reason: "version-independent: does not deploy the product"})
}

func (f *fx) brokenPair(target, reason string) {
	f.pairs = append(f.pairs, domain.ManifestPair{Scope: domain.RenderEnvironment, Target: target, Status: domain.CollectionPartial, Reason: reason})
}

// envItem adds a scope:rendered-unexplained environment item anchored on the
// (already added) environment rendered change at pairs[p].Changes[i] and
// correlates the change to it.
func (f *fx) envItem(rcID string) string {
	it := domain.ChangeItem{RenderedChange: rcID, Family: domain.FamilyRendered, Title: "unexplained rendered change",
		Evidence: []domain.EvidenceID{f.up.ID}, InScope: true, ScopeRule: domain.ScopeRenderedUnexplained,
		Manifestation: domain.ManifestRenderedConfig, ManifestationRule: "manifest:rendered-anchor"}
	it.ID = domain.ChangeItemID("", "", rcID)
	for pi := range f.pairs {
		for ci := range f.pairs[pi].Changes {
			c := &f.pairs[pi].Changes[ci]
			if c.ID == rcID {
				c.ChangeItems = append(c.ChangeItems, it.ID)
				sort.Strings(c.ChangeItems)
				c.Correlation = "rendered-anchor"
			}
		}
	}
	f.envItems = append(f.envItems, it)
	return it.ID
}

func (f *fx) bundle() *domain.EvidenceBundle {
	f.t.Helper()
	if f.b != nil {
		return f.b
	}
	edge := f.art(domain.ArtifactUpgradeEdge, domain.OriginDerived, "derived/upgrade-edge.json", "{}")
	facts := f.facts
	if _, ok := f.sources[domain.ChangeSourceReleaseNotes]; !ok {
		f.source(domain.ChangeSourceReleaseNotes, domain.CollectionOK, "")
	}
	var sources []domain.ChangeSourceStatus
	for _, k := range domain.ChangeSourceKinds {
		s, ok := f.sources[k]
		if !ok {
			s = domain.ChangeSourceStatus{Source: k, Status: domain.CollectionNotApplicable, Reason: "not in the fixture"}
		}
		sources = append(sources, s)
	}
	cr := domain.ChangeEvidenceRecord{Product: domain.ProductRef{ID: "gizmo", Name: "gizmo"}, From: domain.Version{Tag: "v1.0.0"}, To: domain.Version{Tag: "v2.0.0"},
		Edge: edge.ID, Items: f.items, Sources: sources, Renders: f.rel, Producer: "test.change@v1"}
	cr.ID = cr.DerivedID()
	snap := domain.EnvironmentSnapshot{Environment: "test-env", Repo: &domain.RepoRef{Path: "./gitops", TreeDigest: domain.Digest([]byte("tree"))},
		Inputs: f.inputs, Facts: facts.ID, Producer: "test.snapshot@v1", CollectedAt: testNow}
	snap.ID = snap.DerivedID()
	man := domain.BundleManifest{Artifacts: append([]domain.ArtifactRef(nil), f.arts...)}
	sort.Slice(man.Artifacts, func(i, j int) bool { return man.Artifacts[i].Path < man.Artifacts[j].Path })
	b := &domain.EvidenceBundle{SchemaVersion: domain.EvidenceBundleSchemaVersion, Change: cr, Snapshot: snap, Renders: f.pairs,
		EnvironmentItems: f.envItems, Manifest: man, Evidence: f.ev, Producer: "test.bundle@v1", CreatedAt: testNow}
	b.ID = domain.BundleID(cr.ID, snap.ID, man.Digest(), b.Renders)
	if err := b.Validate(); err != nil {
		f.t.Fatalf("fixture bundle invalid: %v", err)
	}
	f.b = b
	return b
}

// --- traversal --------------------------------------------------------------------

type trav struct {
	roots []domain.ExposureRoot
	paths []domain.ImpactPath
	un    []string
	trunc string
}

func node(key string) string { return domain.NodeID(domain.NodeWorkload, key) }

func (tr *trav) root(item, n string) *trav {
	tr.roots = append(tr.roots, domain.ExposureRoot{ChangeItem: item, Node: n, Rule: "exposure:test"})
	return tr
}

// path adds a path along nodes; status applies to the last edge.
func (tr *trav) path(item string, status domain.EdgeStatus, nodes ...string) string {
	p := domain.ImpactPath{ChangeItem: item, Nodes: nodes, Status: domain.EdgeEstablished}
	for i := 0; i+1 < len(nodes); i++ {
		e := domain.EdgeID(domain.EdgeReferences, nodes[i], nodes[i+1], "")
		p.Edges = append(p.Edges, e)
		if i+2 == len(nodes) && status != domain.EdgeEstablished {
			p.Status = status
			tr.un = append(tr.un, e)
		}
	}
	p.ID = domain.ImpactPathID(item, p.Nodes, p.Edges)
	tr.paths = append(tr.paths, p)
	return p.ID
}

func (tr *trav) result() *domain.TraversalResult {
	t := &domain.TraversalResult{Graph: "grf-test", Algorithm: "graph.dfs@v1", Bound: domain.DefaultTraversalBound, Roots: tr.roots, Paths: tr.paths}
	seen := map[string]bool{}
	for _, r := range tr.roots {
		seen[r.Node] = true
	}
	for _, p := range tr.paths {
		for _, n := range p.Nodes {
			seen[n] = true
		}
		if len(p.Edges) > t.DepthReached {
			t.DepthReached = len(p.Edges)
		}
	}
	for n := range seen {
		t.Visited = append(t.Visited, n)
	}
	sort.Strings(t.Visited)
	t.Unresolved = uniqSorted(tr.un)
	if tr.trunc != "" {
		t.Truncated, t.TruncationReason = true, tr.trunc
	}
	t.Complete = !t.Truncated && len(t.Unresolved) == 0
	t.ID = t.DerivedID()
	return t
}

// --- validation and agent records ----------------------------------------------------

type vmod func(*domain.ValidationRecord)

func paths(ids ...string) vmod    { return func(v *domain.ValidationRecord) { v.Paths = ids } }
func rcs(ids ...string) vmod      { return func(v *domain.ValidationRecord) { v.RenderedChanges = ids } }
func subjects(ids ...string) vmod { return func(v *domain.ValidationRecord) { v.Subjects = ids } }
func answers(ids ...string) vmod  { return func(v *domain.ValidationRecord) { v.Answers = ids } }
func detail(s string) vmod        { return func(v *domain.ValidationRecord) { v.Detail = s } }
func validator(name string) vmod  { return func(v *domain.ValidationRecord) { v.Validator = name } }
func withPin(ev domain.EvidenceID) vmod {
	return func(v *domain.ValidationRecord) {
		v.Pin = &domain.PinnedSetting{Kind: domain.PinFlag, Subject: controllerObj, Path: "--mode", Value: `"safe"`, Evidence: []domain.EvidenceID{ev}}
	}
}

// vrec builds a valid validation record of the given kind and outcome.
func (f *fx) vrec(item string, kind domain.ValidationKind, outcome domain.ValidationOutcomeKind, mods ...vmod) domain.ValidationRecord {
	v := domain.ValidationRecord{Validator: "validate." + string(kind) + "@v1", Kind: kind, Level: domain.LevelDeterministic, ChangeItem: item,
		Property: "the change does not reach or break the subjects", Outcome: outcome, CheckedAt: testNow}
	if kind == domain.ValidationRuntime {
		v.Level = domain.LevelObserved
	}
	if outcome != domain.ValidationPass {
		v.Detail = string(outcome) + " in the fixture"
	}
	for _, m := range mods {
		m(&v)
	}
	if outcome.Decisive() {
		v.Inputs = []string{f.facts.ID}
		v.Evidence = []domain.EvidenceID{f.up.ID}
	}
	v.Provenance = domain.Provenance{Method: domain.MethodComputed, Producer: v.Validator, Confidence: domain.ConfidenceHigh}
	v.ID = domain.ValidationRecordID(v.Validator, v.ChangeItem, v.Subjects, v.Paths, v.RenderedChanges, v.Inputs)
	if err := v.Validate(); err != nil {
		f.t.Fatalf("fixture validation invalid: %v", err)
	}
	return v
}

func (f *fx) envEvidence() domain.EvidenceID { return f.envEv.ID }

func aiProv(callID string, ev domain.EvidenceID) domain.Provenance {
	at := testNow
	return domain.Provenance{Method: domain.MethodAI, Producer: "agent@v1", Confidence: domain.ConfidenceLow, Model: "test-model", ModelVersion: "test-model-1",
		PromptVersion: "investigate-agent/v1", PromptDigest: domain.Digest([]byte("prompt")), InputEvidence: []domain.EvidenceID{ev}, GeneratedAt: &at, CallID: callID, Provider: "test"}
}

func (f *fx) assertion(item string, stance domain.AssertionStance, statement string) domain.AgentAssertion {
	a := domain.AgentAssertion{ChangeItem: item, Stance: stance, Statement: statement, Citations: []domain.EvidenceID{f.up.ID}, Provenance: aiProv("call-1", f.up.ID)}
	a.ID = domain.AgentRecordID(domain.AssertionIDPrefix, item, string(stance)+"\x00"+statement, "call-1")
	return a
}

func (f *fx) hypothesis(item, statement string) domain.Hypothesis {
	h := domain.Hypothesis{ChangeItem: item, Statement: statement, Citations: []domain.EvidenceID{f.up.ID}, Provenance: aiProv("call-1", f.up.ID)}
	h.ID = domain.AgentRecordID(domain.HypothesisIDPrefix, item, statement, "call-1")
	return h
}

var transcript = domain.ArtifactRef{Kind: domain.ArtifactAgentTranscript, MediaType: "application/jsonl", Digest: domain.Digest([]byte("t")),
	Size: 1, Path: "derived/agent/transcript.jsonl", Origin: domain.OriginDerived, URI: "ri:agent@v1", Producer: "agent@v1"}

func init() { transcript.ID = domain.ArtifactID(transcript.Kind, transcript.Path, transcript.Digest) }

func agentOutcome(status domain.AgentRunStatus, asrts []domain.AgentAssertion, hyps ...domain.Hypothesis) *investigate.AgentOutcome {
	run := domain.AgentRun{Status: status, Model: "test-model", Provider: "test", PromptVersion: "investigate-agent/v1",
		PromptDigest: domain.Digest([]byte("prompt")), CallID: "call-1", Budget: investigate.DefaultAgentBudget, Transcript: transcript.ID}
	if status != domain.AgentSubmitted {
		run.Reason = "stopped in the fixture"
	}
	return &investigate.AgentOutcome{Run: run, Assertions: asrts, Hypotheses: hyps, Artifacts: []domain.ArtifactRef{transcript}}
}

// input assembles a ClassifyInput (agent submitted unless given).
func (f *fx) in(tr *domain.TraversalResult, agent *investigate.AgentOutcome, vals ...domain.ValidationRecord) investigate.ClassifyInput {
	if agent == nil {
		agent = agentOutcome(domain.AgentSubmitted, nil)
	}
	return investigate.ClassifyInput{Bundle: f.bundle(), Traversal: tr, Agent: agent, Validations: vals,
		Config: investigate.Config{Policy: Policy, Bound: domain.DefaultTraversalBound}}
}

// --- running and conformance ---------------------------------------------------------

// run classifies and checks the output against the contract: it is recorded
// on a run record (Apply validates it) and the record ValidatesAgainst the
// bundle. It also checks reproducibility (a second run is byte-identical).
func run(t *testing.T, in investigate.ClassifyInput) (*investigate.Classification, *domain.Investigation) {
	t.Helper()
	c, err := New().Classify(context.Background(), in)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	again, err := New().Classify(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := mustJSON(t, c), mustJSON(t, again); !bytes.Equal(a, b) {
		t.Fatalf("classification is not reproducible:\n%s\n%s", a, b)
	}
	inv := record(t, in)
	if err := Apply(inv, c, testNow.Add(time.Minute)); err != nil {
		t.Fatalf("classification does not conform to the run record contract: %v", err)
	}
	if err := inv.ValidateAgainst(in.Bundle); err != nil {
		t.Fatalf("classification does not ValidateAgainst its bundle: %v", err)
	}
	return c, inv
}

// record builds the run record the classification is recorded on.
func record(t *testing.T, in investigate.ClassifyInput) *domain.Investigation {
	t.Helper()
	b := in.Bundle
	inv := &domain.Investigation{SchemaVersion: domain.InvestigationSchemaVersion, Bundle: b.ID, Snapshot: b.Snapshot.ID,
		ChangeRecord: b.Change.ID, Environment: b.Snapshot.Environment, Config: in.Config.Digest(),
		Status: domain.InvestigationRunning, Traversal: in.Traversal, Validations: in.Validations,
		Review: domain.HumanReview{State: domain.ReviewNotRequired}, CreatedAt: testNow}
	if a := in.Agent; a != nil {
		run := a.Run
		inv.Agent = &run
		inv.Hypotheses, inv.Assertions, inv.Artifacts, inv.Evidence = a.Hypotheses, a.Assertions, a.Artifacts, a.Evidence
	}
	inv.ID = domain.InvestigationID(inv.Bundle, inv.Config)
	inv.AppendAudit(domain.AuditEvent{At: testNow, Stage: domain.StageValidate, Event: "completed", Actor: "system"})
	return inv
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func finding(t *testing.T, c *investigate.Classification, item string) domain.InvestigationFinding {
	t.Helper()
	for _, f := range c.Findings {
		if f.ChangeItem == item {
			return f
		}
	}
	t.Fatalf("no finding for %s", item)
	return domain.InvestigationFinding{}
}

func wantClass(t *testing.T, f domain.InvestigationFinding, class domain.FindingClass, rule string) {
	t.Helper()
	if f.Class != class || f.Rule != rule {
		t.Fatalf("class %s (%s), want %s (%s)\nreasoning: %s\ngaps: %+v", f.Class, f.Rule, class, rule, f.Reasoning, f.Gaps)
	}
}

func hasGap(f domain.InvestigationFinding, kind domain.GapKind, subject string) bool {
	for _, g := range f.Gaps {
		if g.Kind == kind && (subject == "" || g.Subject == subject) {
			return true
		}
	}
	return false
}

func anyContains(xs []string, sub string) bool {
	for _, x := range xs {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}

// clean is the happy fixture: one values item, rendered-config, a complete
// environment render with one correlated rendered change, one established
// path from the controller, and a render-diff pass covering both.
type clean struct {
	f    *fx
	item string
	rc   string
	path string
	tr   *domain.TraversalResult
	pass domain.ValidationRecord
}

func newClean(t *testing.T, mods ...func(*domain.ChangeItem)) *clean {
	f := newFx(t)
	item := f.item(mods...)
	ids := f.pair("argocd:apps/gizmo.yaml", rc("field-changed", controllerObj, "spec.template.spec.containers[0].args", item))
	tb := &trav{}
	tb.root(item, node(controllerObj))
	p := tb.path(item, domain.EdgeEstablished, node(controllerObj), node("apps/v1/Deployment/app/consumer"))
	tr := tb.result()
	c := &clean{f: f, item: item, rc: ids[0], path: p, tr: tr}
	c.pass = f.vrec(item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(c.rc), paths(p))
	return c
}
