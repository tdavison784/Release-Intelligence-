package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/bundle"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
	"github.com/tdavison784/release-intelligence/internal/investigate/renderdiff"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// fixture is a small, valid investigation: a generic product "demo"
// 1.0.0 → 2.0.0 with a controller-behaviour item (a default compiled into
// the controller, the upstream text names the pinning values key), a
// rendered-config values item, an out-of-scope item, an environment with a
// values file that sets the key and a Secret, a complete environment render
// with one correlated and one uncorrelated rendered change, a graph and the
// real traversal. Assembled by the real bundle assembler, so it validates.
type fixture struct {
	st        *store.Store
	b         *domain.EvidenceBundle
	g         *domain.RelationshipGraph
	trav      *domain.TraversalResult
	report    *domain.ImpactReport
	notes     domain.ArtifactRef // upstream release notes
	values    domain.ArtifactRef // env values file
	secret    domain.ArtifactRef // env Secret (redacted)
	toRender  domain.ArtifactRef
	envEv     domain.Evidence // local-file evidence of the values file
	behaviour string          // in-scope controller-behaviour item
	values2   string          // in-scope rendered-config item
	outScope  string
	rcItem    string // env rendered change correlated to the behaviour item
	rcLoose   string // uncorrelated env rendered change
	deploy    string // node id of the controller Deployment
	cfgMap    string // node id of a ConfigMap reached by an inferred edge
	path      string // an impact path of the behaviour item
	appFile   domain.ArtifactRef
	appNode   string // the Argo CD Application node the env render target is declared in
	valsNode  string // the values-source node of the values file
}

const (
	notesText = `# demo 2.0.0

## Breaking changes

- The controller now rotates keys by default: the default rotation policy is Always.
  Set ` + "`controller.rotationPolicy: Never`" + ` in your values to keep the previous behaviour.
- The values key ` + "`legacy.enabled`" + ` was removed.
- Faster startup.
`
	valuesText = `controller:
  rotationPolicy: Never
  replicas: 2
image:
  tag: v2
`
	secretText = `apiVersion: v1
kind: Secret
metadata:
  name: demo-token
  namespace: demo
data:
  token: "<redacted sha256:0123>"
`
)

var deployID = render.ObjectID{Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "demo", Name: "demo-controller"}
var cmID = render.ObjectID{Version: "v1", Kind: "ConfigMap", Namespace: "demo", Name: "demo-config"}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.New(t.TempDir())
	must(t, err)
	f := &fixture{st: st}
	put := func(spec investigate.ArtifactSpec, b []byte) domain.ArtifactRef {
		ref, err := st.Put(ctx, spec, b)
		must(t, err)
		return ref
	}
	notesURI := "https://example.com/demo/releases/v2.0.0"
	f.notes = put(investigate.ArtifactSpec{Kind: domain.ArtifactUpstreamDocument, Path: "upstream/demo/v2.0.0/release-notes.md", Origin: domain.OriginUpstream,
		MediaType: "text/markdown", URI: notesURI, Producer: "test.collect@v1", Tags: []string{"release-notes", "demo"}}, []byte(notesText))
	ev1 := domain.NewEvidence(domain.EvidenceDocument, "notes", notesURI, "## Breaking changes", "The controller now rotates keys by default", "", time.Time{})
	ev2 := domain.NewEvidence(domain.EvidenceDocument, "notes", notesURI, "## Breaking changes", "The values key `legacy.enabled` was removed.", "", time.Time{})
	ev3 := domain.NewEvidence(domain.EvidenceDocument, "notes", notesURI, "## Breaking changes", "Faster startup.", "", time.Time{})
	edge := domain.UpgradeEdge{Product: domain.ProductRef{ID: "demo"}, From: domain.Version{Tag: "v1.0.0"}, To: domain.Version{Tag: "v2.0.0"},
		Changes: []domain.Change{
			{ID: "chg-rotation", Category: domain.Category("behavior"), Breaking: true, Title: "Default rotation policy is Always",
				Detail: "Set controller.rotationPolicy: Never in your values to keep the previous behaviour.", Evidence: []domain.EvidenceID{ev1.ID}},
			{ID: "chg-legacy", Category: domain.Category("removal"), Breaking: true, Title: "values key legacy.enabled removed", Subjects: []string{"legacy.enabled"}, Evidence: []domain.EvidenceID{ev2.ID}},
			{ID: "chg-startup", Category: domain.Category("feature"), Title: "Faster startup", Evidence: []domain.EvidenceID{ev3.ID}},
		}, Evidence: []domain.Evidence{ev1, ev2, ev3}}
	eb, err := json.MarshalIndent(edge, "", "  ")
	must(t, err)
	edgeRef := put(investigate.ArtifactSpec{Kind: domain.ArtifactUpgradeEdge, Path: "derived/upgrade/edge.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:upgrade@v1", Producer: "upgrade@v1"}, eb)

	item := func(changeID string, fam domain.ChangeItemFamily, title string, m domain.Manifestation, mrule string, in bool, srule string, ev domain.EvidenceID) domain.ChangeItem {
		return domain.ChangeItem{ID: domain.ChangeItemID(changeID, "", ""), ChangeID: changeID, Family: fam, Title: title, Breaking: in,
			Evidence: []domain.EvidenceID{ev}, Manifestation: m, ManifestationRule: mrule, InScope: in, ScopeRule: srule}
	}
	items := []domain.ChangeItem{
		item("chg-rotation", domain.FamilyBehavior, "Default rotation policy is Always", domain.ManifestControllerBehaviour, "manifest:declared-behaviour", true, "scope:breaking", ev1.ID),
		item("chg-legacy", domain.FamilyValues, "values key legacy.enabled removed", domain.ManifestRenderedConfig, "manifest:computed-values", true, "scope:computed-breaking", ev2.ID),
		item("chg-startup", domain.FamilyBehavior, "Faster startup", domain.ManifestControllerBehaviour, "manifest:declared-behaviour", false, "scope:not-breaking", ev3.ID),
	}
	items[1].Subjects = []string{"legacy.enabled"}
	f.behaviour, f.values2, f.outScope = items[0].ID, items[1].ID, items[2].ID
	rec := domain.ChangeEvidenceRecord{Product: edge.Product, From: edge.From, To: edge.To, Edge: edgeRef.ID, Items: items, Producer: "test.collect@v1"}
	for _, k := range domain.ChangeSourceKinds {
		s := domain.ChangeSourceStatus{Source: k, Status: domain.CollectionUnavailable, Reason: "not collected by this fixture"}
		if k == domain.ChangeSourceReleaseNotes {
			s = domain.ChangeSourceStatus{Source: k, Status: domain.CollectionOK, Artifacts: []string{f.notes.ID}}
		}
		rec.Sources = append(rec.Sources, s)
	}
	rec.ID = rec.DerivedID()
	change := &investigate.ChangeResult{Record: rec, Artifacts: []domain.ArtifactRef{edgeRef, f.notes}, Evidence: edge.Evidence}

	f.values = put(investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFile, Path: "env/apps/demo/values.yaml", Origin: domain.OriginEnvironment,
		MediaType: "application/yaml", URI: "apps/demo/values.yaml", Producer: "test.snapshot@v1", Tags: []string{"values", "demo"}}, []byte(valuesText))
	f.secret = put(investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFile, Path: "env/apps/demo/secret.yaml", Origin: domain.OriginEnvironment,
		MediaType: "application/yaml", URI: "apps/demo/secret.yaml", Producer: "test.snapshot@v1", Redacted: true, Tags: []string{"secret"}}, []byte(secretText))
	f.appFile = put(investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFile, Path: "env/apps/demo.yaml", Origin: domain.OriginEnvironment,
		MediaType: "application/yaml", URI: "apps/demo.yaml", Producer: "test.snapshot@v1", Tags: []string{"application"}},
		[]byte("apiVersion: argoproj.io/v1alpha1\nkind: Application\nmetadata:\n  name: demo\n  namespace: argocd\n"))
	facts := put(investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFacts, Path: "derived/env/facts.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:env@v1", Producer: "test.snapshot@v1"}, []byte(`{"name":"prod"}`))
	f.envEv = domain.NewEvidence(domain.EvidenceLocalFile, "values", "apps/demo/values.yaml", "L2", "rotationPolicy: Never", f.values.Digest, time.Time{})
	snap := domain.EnvironmentSnapshot{Environment: "prod", Repo: &domain.RepoRef{Path: "env", TreeDigest: domain.Digest([]byte("tree"))},
		Inputs: []domain.SnapshotInput{{Kind: domain.InputGitOpsRepo, Path: "env", Status: domain.CollectionOK, Artifacts: []string{f.appFile.ID, f.secret.ID, f.values.ID}}},
		Facts:  facts.ID, Producer: "test.snapshot@v1"}
	snap.ID = snap.DerivedID()
	snapRes := &investigate.SnapshotResult{Snapshot: snap, Artifacts: []domain.ArtifactRef{f.values, f.secret, f.appFile, facts}, Evidence: []domain.Evidence{f.envEv}}

	deploy := func(arg string) render.Object {
		return render.Object{ID: deployID, Body: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]any{"name": "demo-controller", "namespace": "demo"},
			"spec": map[string]any{"replicas": 2, "template": map[string]any{"spec": map[string]any{
				"containers": []any{map[string]any{"name": "controller", "image": "demo/controller:" + arg, "args": []any{"--rotation-policy=Never"}}}}}}}}
	}
	cm := render.Object{ID: cmID, Body: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "demo-config", "namespace": "demo"},
		"data": map[string]any{"mode": "fast"}}}
	fromB, _ := renderdiff.EncodeObjects([]render.Object{cm, deploy("v1")})
	toB, _ := renderdiff.EncodeObjects([]render.Object{cm, deploy("v2")})
	fromRef := put(investigate.ArtifactSpec{Kind: domain.ArtifactRenderedManifests, Path: "derived/render/env/argocd-demo/from.yaml", Origin: domain.OriginDerived,
		MediaType: "application/yaml", URI: "ri:renderdiff@v1", Producer: "test.render@v1", Tags: []string{"deployment", "demo-controller"}}, fromB)
	f.toRender = put(investigate.ArtifactSpec{Kind: domain.ArtifactRenderedManifests, Path: "derived/render/env/argocd-demo/to.yaml", Origin: domain.OriginDerived,
		MediaType: "application/yaml", URI: "ri:renderdiff@v1", Producer: "test.render@v1", Tags: []string{"deployment", "demo-controller"}}, toB)
	diffRef := put(investigate.ArtifactSpec{Kind: domain.ArtifactRenderDiff, Path: "derived/render/env/argocd-demo/diff.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:renderdiff@v1", Producer: "test.render@v1"}, []byte(`{"changes":2}`))
	target := "argocd:apps/demo.yaml"
	rcA := domain.RenderedChange{Class: "image-changed", Object: deployID.String(), Path: "spec.template.spec.containers[name=controller].image",
		Before: `"demo/controller:v1"`, After: `"demo/controller:v2"`, ChangeItems: []string{f.behaviour}, Correlation: "subject"}
	rcB := domain.RenderedChange{Class: "field-changed", Object: cmID.String(), Path: "data.mode", Before: `"fast"`, After: `"fast"`}
	for _, c := range []*domain.RenderedChange{&rcA, &rcB} {
		c.ID = domain.RenderedChangeID(domain.RenderEnvironment, target, c.Object, c.Path, c.Class)
		e := domain.NewEvidence(domain.EvidenceRenderedDiff, diffRef.ID, "ri:renderdiff@v1", "changes[id="+c.ID+"]", c.Class+" "+c.Object, diffRef.Digest, time.Time{})
		c.Evidence = []domain.EvidenceID{e.ID}
		snapRes.Evidence = append(snapRes.Evidence, e)
	}
	f.rcItem, f.rcLoose = rcA.ID, rcB.ID
	changes := []domain.RenderedChange{rcA, rcB}
	if changes[0].ID > changes[1].ID {
		changes[0], changes[1] = changes[1], changes[0]
	}
	pair := domain.ManifestPair{Scope: domain.RenderEnvironment, Target: target, From: fromRef.ID, To: f.toRender.ID, Diff: diffRef.ID, Status: domain.CollectionOK, Changes: changes}
	renders := &investigate.RenderResult{Pairs: []domain.ManifestPair{pair}, Artifacts: []domain.ArtifactRef{fromRef, f.toRender, diffRef}}

	b, err := bundle.NewAssembler(st).Assemble(ctx, change, snapRes, renders)
	must(t, err)
	must(t, st.PutBundle(ctx, b))
	f.b = b

	// Graph: ConfigMap → Deployment is inferred (a name-only match), so the
	// behaviour item has an unresolved path.
	dn := domain.GraphNode{ID: domain.NodeID(domain.NodeWorkload, deployID.String()), Kind: domain.NodeWorkload, Key: deployID.String(), Artifacts: []string{f.toRender.ID}}
	cn := domain.GraphNode{ID: domain.NodeID(domain.NodeConfigObject, cmID.String()), Kind: domain.NodeConfigObject, Key: cmID.String(), Artifacts: []string{f.toRender.ID}}
	f.deploy, f.cfgMap = dn.ID, cn.ID
	e := domain.GraphEdge{Kind: domain.EdgeReferences, From: dn.ID, To: cn.ID, Status: domain.EdgeInferred, Rule: "test:name-match", Detail: "env[0].valueFrom", Reason: "matched by name only"}
	e.ID = domain.EdgeID(e.Kind, e.From, e.To, e.Detail)
	an := domain.GraphNode{ID: domain.NodeID(domain.NodeGitOpsApplication, "argoproj.io/v1alpha1/Application/argocd/demo"), Kind: domain.NodeGitOpsApplication,
		Key: "argoproj.io/v1alpha1/Application/argocd/demo", Artifacts: []string{f.appFile.ID}}
	vn := domain.GraphNode{ID: domain.NodeID(domain.NodeValuesSource, "env/apps/demo/values.yaml"), Kind: domain.NodeValuesSource, Key: "env/apps/demo/values.yaml",
		Artifacts: []string{f.values.ID}}
	f.appNode, f.valsNode = an.ID, vn.ID
	ve := domain.GraphEdge{Kind: domain.EdgeConfigures, From: vn.ID, To: an.ID, Status: domain.EdgeEstablished, Rule: "test:values-file", Detail: "helm.valueFiles", Artifacts: []string{f.appFile.ID}}
	ve.ID = domain.EdgeID(ve.Kind, ve.From, ve.To, ve.Detail)
	g := domain.RelationshipGraph{Bundle: b.ID, Builder: "test.graph@v1", Nodes: []domain.GraphNode{dn, cn, an, vn}, Edges: []domain.GraphEdge{e, ve}}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	sort.Slice(g.Edges, func(i, j int) bool { return g.Edges[i].ID < g.Edges[j].ID })
	g.ID = g.DerivedID()
	must(t, g.Validate())
	f.g = &g
	roots := []domain.ExposureRoot{{ChangeItem: f.behaviour, Node: dn.ID, Rule: "exposure:rendered-change", RenderedChange: rcA.ID}}
	trav, err := relations.NewTraverser().Traverse(ctx, f.g, roots, domain.DefaultTraversalBound)
	must(t, err)
	f.trav = trav
	if len(trav.Paths) == 0 {
		t.Fatal("fixture: no impact path")
	}
	f.path = trav.Paths[0].ID
	return f
}

func (f *fixture) input(cfg investigate.AgentConfig) investigate.InvestigateInput {
	return investigate.InvestigateInput{Bundle: f.b, Graph: f.g, Traversal: f.trav, Report: f.report, Config: cfg}
}

func (f *fixture) session(t *testing.T, validators ...investigate.Validator) (*Session, investigate.AgentSession) {
	t.Helper()
	s, as, err := Prepare(f.st, f.input(investigate.AgentConfig{Enabled: true, Driver: DriverScripted, PromptVersion: promptV1,
		Budget: investigate.DefaultAgentBudget}), validators, fixedNow)
	must(t, err)
	s.SetCall(CallInfo{ID: "call-1", Model: "test-model", ModelVersion: "test-model-2026", Provider: "test"})
	return s, as
}

// promptV1 pins the tests of v1 behaviour (its transcripts must keep replaying).
const promptV1 = "investigate-agent/v1"

func fixedNow() time.Time { return time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC) }

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// --- fake validators -----------------------------------------------------------------

// fakeOverride is an override-inspection validator: it passes with a pinned
// setting when the environment's values file states the key, citing the
// bundle's local-file evidence.
type fakeOverride struct {
	f     *fixture
	calls int
}

func (v *fakeOverride) Name() string                             { return "fake.override@v1" }
func (v *fakeOverride) Kind() domain.ValidationKind              { return domain.ValidationOverrideInspection }
func (v *fakeOverride) Applies(fam domain.ChangeItemFamily) bool { return fam == domain.FamilyBehavior }
func (v *fakeOverride) Validate(ctx context.Context, in investigate.ValidationInput) (*investigate.ValidationOutput, error) {
	v.calls++
	inputs := []string{v.f.values.ID}
	rec := domain.ValidationRecord{Validator: v.Name(), Kind: v.Kind(), Level: domain.LevelDeterministic, ChangeItem: in.ChangeItem,
		Subjects: in.Subjects, Paths: in.Paths, RenderedChanges: in.RenderedChanges, Property: "the environment pins controller.rotationPolicy",
		Inputs: inputs, Outcome: domain.ValidationPass, Evidence: []domain.EvidenceID{v.f.envEv.ID},
		Pin:        &domain.PinnedSetting{Kind: domain.PinValuesKey, Subject: "env/apps/demo/values.yaml", Path: "controller.rotationPolicy", Value: `"Never"`, Evidence: []domain.EvidenceID{v.f.envEv.ID}},
		Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: v.Name(), Confidence: domain.ConfidenceHigh}, CheckedAt: time.Now()}
	if in.Request != nil {
		rec.Request = in.Request.ID
	}
	rec.ID = domain.ValidationRecordID(rec.Validator, rec.ChangeItem, rec.Subjects, rec.Paths, rec.RenderedChanges, rec.Inputs)
	return &investigate.ValidationOutput{Records: []domain.ValidationRecord{rec}}, nil
}

// brokenValidator fails (schema-compat) or misbehaves (static-reference: a
// record for another change item).
type brokenValidator struct {
	kind  domain.ValidationKind
	other string
}

func (v *brokenValidator) Name() string                         { return "fake.broken-" + string(v.kind) + "@v1" }
func (v *brokenValidator) Kind() domain.ValidationKind          { return v.kind }
func (v *brokenValidator) Applies(domain.ChangeItemFamily) bool { return true }
func (v *brokenValidator) Validate(ctx context.Context, in investigate.ValidationInput) (*investigate.ValidationOutput, error) {
	if v.other == "" {
		return nil, fmt.Errorf("cannot read the CRDs")
	}
	rec := domain.ValidationRecord{Validator: v.Name(), Kind: v.kind, Level: domain.LevelDeterministic, ChangeItem: v.other, Property: "x",
		Outcome: domain.ValidationNotRun, Detail: "no input", Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: v.Name(), Confidence: domain.ConfidenceHigh}}
	rec.ID = domain.ValidationRecordID(rec.Validator, rec.ChangeItem, nil, nil, nil, nil)
	return &investigate.ValidationOutput{Records: []domain.ValidationRecord{rec}}, nil
}
