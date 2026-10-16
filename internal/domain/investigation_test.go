package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// invFixture is a complete, valid investigation of a cert-manager-shaped
// upgrade: item A is affected (a failing render validation along
// api-type → Certificate → Secret → Deployment), item B is not affected
// (an override validated along a root-only path, with an agent concern that
// the validation answers), item C is out of scope.
type invFixture struct {
	b       *EvidenceBundle
	g       *RelationshipGraph
	inv     *Investigation
	itemA   string
	itemB   string
	itemC   string
	pathA   string
	pathB   string
	edgeRef string // secret → deployment
	rcA     string // the environment rendered change correlated to item A
}

var fixNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func digestOf(s string) string { return Digest([]byte(s)) }

func artifact(kind ArtifactKind, origin ArtifactOrigin, path, content string, items ...string) ArtifactRef {
	d := digestOf(content)
	return ArtifactRef{
		ID: ArtifactID(kind, path, d), Kind: kind, MediaType: "application/yaml", Digest: d,
		Size: int64(len(content)), Path: path, Origin: origin, URI: "file://" + path,
		Producer: "bundle.collect@v1", Tags: []string{string(kind)}, ChangeItems: items,
	}
}

func aiProv(callID string, input ...EvidenceID) Provenance {
	at := fixNow
	return Provenance{
		Method: MethodAI, Producer: "investigate.agent@v1", Confidence: ConfidenceMedium,
		Model: "claude-opus-5-5", ModelVersion: "claude-opus-5-5", PromptVersion: "investigate-agent/v1",
		PromptDigest: digestOf("prompt"), InputEvidence: input, GeneratedAt: &at, CallID: callID, Provider: "anthropic",
	}
}

func computed(producer string) Provenance {
	return Provenance{Method: MethodComputed, Producer: producer, Confidence: ConfidenceHigh}
}

func newInvFixture(t *testing.T) *invFixture {
	t.Helper()
	fx := &invFixture{}
	itemA, itemB, itemC := ChangeItemID("chg-a", "", ""), ChangeItemID("chg-b", "", ""), ChangeItemID("chg-c", "", "")
	fx.itemA, fx.itemB, fx.itemC = itemA, itemB, itemC

	// Artifacts and evidence.
	edgeArt := artifact(ArtifactUpgradeEdge, OriginDerived, "derived/upgrade-edge.json", "edge")
	notesArt := artifact(ArtifactUpstreamDocument, OriginUpstream, "upstream/cert-manager/v1.18.0/release-notes.md", "notes", itemA, itemB)
	crdArt := artifact(ArtifactCRDDefinition, OriginUpstream, "upstream/cert-manager/v1.18.0/crds.yaml", "crds", itemA)
	factsArt := artifact(ArtifactEnvironmentFacts, OriginDerived, "derived/environment-facts.json", "facts")
	valuesArt := artifact(ArtifactEnvironmentFile, OriginEnvironment, "env/apps/cert-manager/values.yaml", "values")
	certArt := artifact(ArtifactEnvironmentFile, OriginEnvironment, "env/apps/web/certificate.yaml", "cert")
	upEv := NewEvidence(EvidenceDocument, "notes", "https://github.com/cert-manager/cert-manager/releases/tag/v1.18.0", "L10-L12", "rotationPolicy defaults to Always", notesArt.Digest, fixNow)
	upEvB := NewEvidence(EvidenceDocument, "notes", "https://github.com/cert-manager/cert-manager/releases/tag/v1.18.0", "L20", "pathType Exact", notesArt.Digest, fixNow)
	envEv := NewEvidence(EvidenceLocalFile, "", "apps/web/certificate.yaml", "$.spec", "secretName: web-tls", certArt.Digest, fixNow)

	items := []ChangeItem{
		{ChangeID: "chg-a", Family: FamilyAPISchema, Title: "Certificate privateKey.rotationPolicy default changes to Always", Breaking: true, Evidence: []EvidenceID{upEv.ID}, InScope: true, ScopeRule: "scope:breaking"},
		{ChangeID: "chg-b", Family: FamilyValues, Title: "ACME HTTP01 ingress pathType defaults to Exact", Breaking: true, Evidence: []EvidenceID{upEvB.ID}, InScope: true, ScopeRule: "scope:breaking"},
		{ChangeID: "chg-c", Family: FamilyBehavior, Title: "Bump Go to 1.24", Evidence: []EvidenceID{upEv.ID}, InScope: false, ScopeRule: "scope:routine-excluded"},
	}
	for i := range items {
		items[i].ID = ChangeItemID(items[i].ChangeID, items[i].Platform, items[i].RenderedChange)
	}
	var sources []ChangeSourceStatus
	for _, k := range ChangeSourceKinds {
		s := ChangeSourceStatus{Source: k, Status: CollectionNotApplicable, Reason: "not collected in the fixture"}
		switch k {
		case ChangeSourceReleaseNotes:
			s = ChangeSourceStatus{Source: k, Status: CollectionOK, Artifacts: []string{notesArt.ID}}
		case ChangeSourceAPISchema:
			s = ChangeSourceStatus{Source: k, Status: CollectionOK, Artifacts: []string{crdArt.ID}}
		case ChangeSourceSourceDiff:
			s = ChangeSourceStatus{Source: k, Status: CollectionUnavailable, Reason: "no source-diff collector yet"}
		}
		sources = append(sources, s)
	}
	cr := ChangeEvidenceRecord{
		Product: ProductRef{ID: "cert-manager", Name: "cert-manager"},
		From:    Version{Tag: "v1.17.0", Semver: "1.17.0"}, To: Version{Tag: "v1.18.0", Semver: "1.18.0"},
		Edge: edgeArt.ID, Items: items, Sources: sources, Producer: "bundle.change@v1",
	}
	cr.ID = cr.DerivedID()

	snap := EnvironmentSnapshot{
		Environment:       "prod-eu",
		Repo:              &RepoRef{Path: "./gitops", Commit: "0123abc", TreeDigest: digestOf("tree")},
		KubernetesVersion: "1.31",
		Inputs: []SnapshotInput{
			{Kind: InputGitOpsRepo, Path: "./gitops", Status: CollectionOK, Artifacts: []string{certArt.ID, valuesArt.ID}},
			{Kind: InputClusterVersion, Value: "1.31", Status: CollectionOK},
		},
		Facts: factsArt.ID, Producer: "bundle.snapshot@v1", CollectedAt: fixNow,
	}
	snap.ID = snap.DerivedID()

	fromArt := artifact(ArtifactRenderedManifests, OriginDerived, "derived/render/env/argocd-cert-manager/from.yaml", "from")
	toArt := artifact(ArtifactRenderedManifests, OriginDerived, "derived/render/env/argocd-cert-manager/to.yaml", "to")
	diffArt := artifact(ArtifactRenderDiff, OriginDerived, "derived/render/env/argocd-cert-manager/diff.json", "diff")
	rendEv := NewEvidence(EvidenceRenderedDiff, "render", "templates/deployment.yaml", "spec.template.spec.containers[0].args", "--default-private-key-rotation-policy", diffArt.Digest, fixNow)
	target := "argocd:apps/cert-manager.yaml"
	rcA := RenderedChange{Class: "container-arg-added", Object: "apps/v1/Deployment/cert-manager/cert-manager",
		Path: "spec.template.spec.containers[0].args", After: `"--default-private-key-rotation-policy=Always"`,
		ChangeItems: []string{itemA}, Correlation: "changelog-subject", Evidence: []EvidenceID{rendEv.ID}}
	rcA.ID = RenderedChangeID(RenderEnvironment, target, rcA.Object, rcA.Path, rcA.Class)
	fx.rcA = rcA.ID
	envPair := ManifestPair{Scope: RenderEnvironment, Target: target, From: fromArt.ID, To: toArt.ID, Diff: diffArt.ID,
		Status: CollectionOK, Changes: []RenderedChange{rcA}}

	man := BundleManifest{Artifacts: []ArtifactRef{edgeArt, notesArt, crdArt, factsArt, valuesArt, certArt, fromArt, toArt, diffArt}}
	sort.Slice(man.Artifacts, func(i, j int) bool { return man.Artifacts[i].Path < man.Artifacts[j].Path })
	b := &EvidenceBundle{
		SchemaVersion: EvidenceBundleSchemaVersion, Change: cr, Snapshot: snap, Manifest: man,
		Renders:  []ManifestPair{envPair},
		Evidence: []Evidence{upEv, upEvB, envEv, rendEv}, Producer: "bundle@v1", CreatedAt: fixNow,
	}
	b.ID = BundleID(cr.ID, snap.ID, man.Digest(), b.Renders)
	fx.b = b

	// Graph.
	node := func(k NodeKind, key string) GraphNode {
		return GraphNode{ID: NodeID(k, key), Kind: k, Key: key, Evidence: []EvidenceID{envEv.ID}}
	}
	nProd := node(NodeProductRelease, "cert-manager@v1.18.0")
	nAPI := node(NodeAPIType, "cert-manager.io/v1/Certificate")
	nCert := node(NodeCustomResource, "cert-manager.io/v1/Certificate/web/web-tls")
	nSecret := node(NodeConfigObject, "/v1/Secret/web/web-tls")
	nDeploy := node(NodeWorkload, "apps/v1/Deployment/web/web")
	edge := func(k EdgeKind, from, to GraphNode, detail string) GraphEdge {
		return GraphEdge{ID: EdgeID(k, from.ID, to.ID, detail), Kind: k, From: from.ID, To: to.ID, Status: EdgeEstablished,
			Rule: "graph.k8s-refs@v1", Detail: detail, Evidence: []EvidenceID{envEv.ID}}
	}
	eServes := edge(EdgeServesAPI, nProd, nAPI, "")
	eConsumes := edge(EdgeConsumesAPI, nAPI, nCert, "")
	eProduces := edge(EdgeProduces, nCert, nSecret, "spec.secretName")
	eRef := edge(EdgeReferences, nSecret, nDeploy, "spec.template.spec.volumes[0].secret.secretName")
	fx.edgeRef = eRef.ID
	g := &RelationshipGraph{Bundle: b.ID, Builder: "graph.build@v1",
		Nodes: []GraphNode{nProd, nAPI, nCert, nSecret, nDeploy}, Edges: []GraphEdge{eServes, eConsumes, eProduces, eRef}}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	sort.Slice(g.Edges, func(i, j int) bool { return g.Edges[i].ID < g.Edges[j].ID })
	g.ID = g.DerivedID()
	fx.g = g

	// Traversal.
	pA := ImpactPath{ChangeItem: itemA, Nodes: []string{nAPI.ID, nCert.ID, nSecret.ID, nDeploy.ID},
		Edges: []string{eConsumes.ID, eProduces.ID, eRef.ID}, Status: EdgeEstablished}
	pA.ID = ImpactPathID(pA.ChangeItem, pA.Nodes, pA.Edges)
	pB := ImpactPath{ChangeItem: itemB, Nodes: []string{nDeploy.ID}, Status: EdgeEstablished}
	pB.ID = ImpactPathID(pB.ChangeItem, pB.Nodes, pB.Edges)
	fx.pathA, fx.pathB = pA.ID, pB.ID
	visited := []string{nAPI.ID, nCert.ID, nSecret.ID, nDeploy.ID}
	sort.Strings(visited)
	tr := &TraversalResult{Graph: g.ID, Algorithm: "graph.dfs@v1", Bound: DefaultTraversalBound,
		Roots: []ExposureRoot{
			{ChangeItem: itemA, Node: nAPI.ID, Rule: "exposure:rendered-change", RenderedChange: rcA.ID},
			{ChangeItem: itemB, Node: nDeploy.ID, Rule: "exposure:values-key"},
		},
		DepthReached: 3, Visited: visited, Paths: []ImpactPath{pA, pB}, Complete: true}
	tr.ID = tr.DerivedID()

	// Run artifacts.
	graphArt := artifact(ArtifactGraph, OriginDerived, "derived/graph.json", "graph")
	renderArt := artifact(ArtifactRenderDiff, OriginDerived, "derived/render/env/diff.json", "diff")
	transcript := artifact(ArtifactAgentTranscript, OriginDerived, "derived/agent/transcript.jsonl", "transcript")
	reportArt := artifact(ArtifactImpactReport, OriginDerived, "derived/impact-report.json", "report")

	// Agent records.
	citeEv := NewEvidence(EvidenceLocalFile, "", "apps/web/certificate.yaml", "L8", "secretName: web-tls", certArt.Digest, fixNow)
	concern := AgentAssertion{ChangeItem: itemB, Stance: StanceRaisesConcern,
		Statement: "the web Deployment may serve the ACME solver path", Subjects: []string{nDeploy.ID}, Paths: []string{pB.ID},
		Citations: []EvidenceID{citeEv.ID}, Provenance: aiProv("session-1", citeEv.ID)}
	concern.ID = AgentRecordID(AssertionIDPrefix, concern.ChangeItem, string(concern.Stance)+"\x00"+concern.Statement, "session-1")
	explain := AgentAssertion{ChangeItem: itemA, Stance: StanceExplains,
		Statement: "web-tls is mounted by the web Deployment; rotation re-issues the key", Paths: []string{pA.ID},
		Citations: []EvidenceID{citeEv.ID}, Provenance: aiProv("session-1", citeEv.ID)}
	explain.ID = AgentRecordID(AssertionIDPrefix, explain.ChangeItem, string(explain.Stance)+"\x00"+explain.Statement, "session-1")
	vreq := ValidationRequest{ChangeItem: itemB, Kind: ValidationOverrideInspection, Subjects: []string{nDeploy.ID},
		Status: RequestFulfilled, Provenance: aiProv("session-1", citeEv.ID)}
	vreq.ID = AgentRecordID(ValidationRequestIDPrefix, vreq.ChangeItem, string(vreq.Kind)+"\x00"+strings.Join(vreq.Subjects, ","), "session-1")

	// Validations.
	vA := ValidationRecord{Validator: "validate.render-diff@v1", Kind: ValidationRenderDiff, Level: LevelDeterministic,
		ChangeItem: itemA, Subjects: []string{nCert.ID}, Paths: []string{pA.ID}, RenderedChanges: []string{rcA.ID},
		Property: "the customer's render is unchanged by the rotationPolicy default", Inputs: []string{certArt.ID},
		Outputs: []string{renderArt.ID}, Outcome: ValidationFail, Detail: "Certificate web-tls inherits rotationPolicy Always",
		Evidence: []EvidenceID{envEv.ID}, Provenance: computed("validate.render-diff@v1"), CheckedAt: fixNow}
	vA.ID = ValidationRecordID(vA.Validator, vA.ChangeItem, vA.Subjects, vA.Paths, vA.RenderedChanges, vA.Inputs)
	vB := ValidationRecord{Validator: "validate.override@v1", Kind: ValidationOverrideInspection, Level: LevelDeterministic,
		ChangeItem: itemB, Subjects: []string{nDeploy.ID}, Paths: []string{pB.ID},
		Property: "the values pin acmesolver pathType to Prefix", Inputs: []string{valuesArt.ID}, Outcome: ValidationPass,
		Evidence: []EvidenceID{envEv.ID}, Answers: []string{concern.ID}, Request: vreq.ID,
		Provenance: computed("validate.override@v1"), CheckedAt: fixNow}
	vB.ID = ValidationRecordID(vB.Validator, vB.ChangeItem, vB.Subjects, vB.Paths, vB.RenderedChanges, vB.Inputs)
	vreq.Result = vB.ID

	// Findings.
	fA := InvestigationFinding{ChangeItem: itemA, Class: FindingAffected, Rule: "classify:validated-exposure",
		Reasoning: "render-diff demonstrates the default reaches Certificate web-tls", Environment: "prod-eu",
		Components: []string{nCert.ID, nDeploy.ID}, RenderedChanges: []string{rcA.ID}, Paths: []string{pA.ID}, Validations: []string{vA.ID},
		Assertions:     []string{explain.ID},
		ImpactFindings: []ImpactFindingRef{{ID: "imp-a", Classification: ImpactReviewRequired, Rule: "impact:crd-attr-default"}},
		Evidence:       []EvidenceID{upEv.ID, envEv.ID}, Remediation: "set spec.privateKey.rotationPolicy: Never to keep today's behaviour",
		Basis: LevelDeterministic, Approval: ApprovalEligibility{RequiresHumanReview: true, Blockers: []string{"class affected"}},
		Provenance: computed("classify.policy@v1")}
	fA.ID = InvestigationFindingID(fA.Environment, fA.ChangeItem, fA.Class, fA.Rule)
	fB := InvestigationFinding{ChangeItem: itemB, Class: FindingNotAffected, Rule: "classify:validated-clear",
		Reasoning: "the values override pins pathType; validated", Environment: "prod-eu",
		Paths: []string{pB.ID}, Validations: []string{vB.ID}, Assertions: []string{concern.ID},
		Evidence: []EvidenceID{upEvB.ID}, Basis: LevelDeterministic,
		Approval: ApprovalEligibility{Eligible: true}, Provenance: computed("classify.policy@v1")}
	fB.ID = InvestigationFindingID(fB.Environment, fB.ChangeItem, fB.Class, fB.Rule)

	inv := &Investigation{
		SchemaVersion: InvestigationSchemaVersion, Bundle: b.ID, Snapshot: snap.ID, ChangeRecord: cr.ID,
		Environment: "prod-eu", Config: digestOf("config"), Status: InvestigationAwaiting,
		ImpactReport: reportArt.ID, Graph: graphArt.ID, Traversal: tr,
		Agent: &AgentRun{Status: AgentSubmitted, Model: "claude-opus-5-5", Provider: "anthropic",
			PromptVersion: "investigate-agent/v1", PromptDigest: digestOf("prompt"), CallID: "session-1",
			Budget: AgentBudget{MaxTurns: 30, MaxToolCalls: 60, MaxTokens: 400000, MaxReadBytes: 2 << 20, MaxSeconds: 900},
			Turns:  7, ToolCalls: 12, Tokens: 50000, Transcript: transcript.ID},
		Assertions:         []AgentAssertion{concern, explain},
		ValidationRequests: []ValidationRequest{vreq},
		Validations:        []ValidationRecord{vA, vB},
		Findings:           []InvestigationFinding{fA, fB},
		Decision:           &PolicyDecision{Policy: "classify.policy@v1", Outcome: PolicyHumanReview, Reasons: []string{"item affected"}},
		Review:             HumanReview{State: ReviewAwaiting},
		Artifacts:          []ArtifactRef{graphArt, renderArt, transcript, reportArt},
		Evidence:           []Evidence{citeEv},
		CreatedAt:          fixNow,
	}
	inv.ID = InvestigationID(inv.Bundle, inv.Config)
	inv.AppendAudit(AuditEvent{At: fixNow, Stage: StageCollectChange, Event: "completed", Actor: "system", Refs: []string{cr.ID}})
	inv.AppendAudit(AuditEvent{At: fixNow.Add(time.Minute), Stage: StageClassify, Event: "completed", Actor: "system"})
	fx.inv = inv
	return fx
}

func TestInvestigationFixtureValid(t *testing.T) {
	fx := newInvFixture(t)
	if err := fx.b.Validate(); err != nil {
		t.Fatalf("bundle: %v", err)
	}
	if err := fx.g.Validate(); err != nil {
		t.Fatalf("graph: %v", err)
	}
	if err := fx.inv.Traversal.ValidateAgainst(*fx.g); err != nil {
		t.Fatalf("traversal vs graph: %v", err)
	}
	if err := fx.inv.ValidateAgainst(fx.b); err != nil {
		t.Fatalf("investigation: %v", err)
	}
}

// TestInvestigationJSONRoundTrip pins that the records survive JSON (the
// bundle on disk, Temporal payloads) and still validate.
func TestInvestigationJSONRoundTrip(t *testing.T) {
	fx := newInvFixture(t)
	for name, v := range map[string]any{"bundle": fx.b, "graph": fx.g, "investigation": fx.inv} {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		switch name {
		case "bundle":
			var b EvidenceBundle
			if err := json.Unmarshal(raw, &b); err != nil {
				t.Fatal(err)
			}
			if err := b.Validate(); err != nil {
				t.Errorf("bundle after round trip: %v", err)
			}
		case "graph":
			var g RelationshipGraph
			if err := json.Unmarshal(raw, &g); err != nil {
				t.Fatal(err)
			}
			if err := g.Validate(); err != nil {
				t.Errorf("graph after round trip: %v", err)
			}
		case "investigation":
			var inv Investigation
			if err := json.Unmarshal(raw, &inv); err != nil {
				t.Fatal(err)
			}
			if err := inv.ValidateAgainst(fx.b); err != nil {
				t.Errorf("investigation after round trip: %v", err)
			}
		}
	}
}

func TestInvestigationIDsAreContentDerived(t *testing.T) {
	a, b := newInvFixture(t), newInvFixture(t)
	if a.b.ID != b.b.ID || a.inv.ID != b.inv.ID || a.g.ID != b.g.ID || a.inv.Traversal.ID != b.inv.Traversal.ID {
		t.Fatal("identical content must yield identical ids")
	}
	// Timestamps are not identity.
	c := newInvFixture(t)
	c.b.CreatedAt = fixNow.Add(time.Hour)
	c.b.Snapshot.CollectedAt = fixNow.Add(time.Hour)
	if err := c.b.Validate(); err != nil {
		t.Fatalf("collection time must not change identity: %v", err)
	}
	// Content is identity.
	if ArtifactID(ArtifactEnvironmentFile, "env/a.yaml", digestOf("x")) == ArtifactID(ArtifactEnvironmentFile, "env/a.yaml", digestOf("y")) {
		t.Fatal("artifact ids must depend on the digest")
	}
}

func wantInvErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want an error containing %q, got none", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("want an error containing %q, got:\n%v", want, err)
	}
}

func TestBundleValidateRules(t *testing.T) {
	cases := map[string]struct {
		mut  func(fx *invFixture)
		want string
	}{
		"schema version":         {func(fx *invFixture) { fx.b.SchemaVersion = "v0" }, "schemaVersion"},
		"bundle id not derived":  {func(fx *invFixture) { fx.b.ID = "bnd-000000000000" }, "id is not derived from change+snapshot+manifest"},
		"producer":               {func(fx *invFixture) { fx.b.Producer = "" }, "producer is required"},
		"duplicate evidence":     {func(fx *invFixture) { fx.b.Evidence = append(fx.b.Evidence, fx.b.Evidence[0]) }, "duplicate evidence"},
		"edge artifact missing":  {func(fx *invFixture) { fx.b.Change.Edge = "art-000000000000" }, "change.edge: artifact"},
		"edge artifact kind":     {func(fx *invFixture) { fx.b.Change.Edge = fx.b.Snapshot.Facts }, "has kind environment-facts"},
		"item evidence resolves": {func(fx *invFixture) { fx.b.Change.Items[0].Evidence = []EvidenceID{"ev-nope"} }, "does not resolve"},
		"artifact unknown item": {func(fx *invFixture) {
			fx.b.Manifest.Artifacts[0].ChangeItems = []string{"ci-unknown"}
		}, "relates to unknown change item"},
		"render artifacts": {func(fx *invFixture) {
			fx.b.Renders = []ManifestPair{{Scope: RenderEnvironment, Target: "argocd:apps/x.yaml", Status: CollectionOK}}
		}, "status ok requires from, to and diff"},
		"render scope": {func(fx *invFixture) {
			fx.b.Renders = []ManifestPair{{Scope: RenderRelease, Target: "chart-defaults", Status: CollectionUnavailable, Reason: "x"}}
		}, `want "environment"`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newInvFixture(t)
			c.mut(fx)
			wantInvErr(t, fx.b.Validate(), c.want)
		})
	}
}

func TestChangeRecordValidateRules(t *testing.T) {
	cases := map[string]struct {
		mut  func(r *ChangeEvidenceRecord)
		want string
	}{
		"id not derived":        {func(r *ChangeEvidenceRecord) { r.ID = "cer-x" }, "id is not derived"},
		"endpoints":             {func(r *ChangeEvidenceRecord) { r.From = Version{} }, "product, from and to are required"},
		"edge is artifact":      {func(r *ChangeEvidenceRecord) { r.Edge = "edge.json" }, "edge must be an artifact id"},
		"duplicate item":        {func(r *ChangeEvidenceRecord) { r.Items = append(r.Items, r.Items[0]) }, "duplicate item"},
		"changeId xor platform": {func(r *ChangeEvidenceRecord) { r.Items[0].Platform = "kubernetes" }, "exactly one of changeId, platform and renderedChange"},
		"item id derived":       {func(r *ChangeEvidenceRecord) { r.Items[0].ID = "ci-x" }, "id is not derived from its anchor"},
		"item family":           {func(r *ChangeEvidenceRecord) { r.Items[0].Family = "magic" }, "unknown family"},
		"item scope rule":       {func(r *ChangeEvidenceRecord) { r.Items[0].ScopeRule = "" }, "title and scopeRule are required"},
		"item evidence":         {func(r *ChangeEvidenceRecord) { r.Items[0].Evidence = nil }, "upstream evidence is required"},
		"source missing":        {func(r *ChangeEvidenceRecord) { r.Sources = r.Sources[1:] }, "must be stated exactly once"},
		"source twice":          {func(r *ChangeEvidenceRecord) { r.Sources = append(r.Sources, r.Sources[0]) }, "must be stated exactly once"},
		"gap needs reason": {func(r *ChangeEvidenceRecord) {
			r.Sources[2] = ChangeSourceStatus{Source: r.Sources[2].Source, Status: CollectionPartial}
		}, "requires a reason"},
		"ok needs artifacts": {func(r *ChangeEvidenceRecord) { r.Sources[0].Artifacts = nil }, "status ok requires the collected artifacts"},
		"unknown status": {func(r *ChangeEvidenceRecord) {
			r.Sources[2] = ChangeSourceStatus{Source: r.Sources[2].Source, Status: "maybe", Reason: "x"}
		}, "unknown collection status"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := newInvFixture(t).b.Change
			c.mut(&r)
			wantInvErr(t, r.Validate(), c.want)
		})
	}
}

func TestSnapshotValidateRules(t *testing.T) {
	cases := map[string]struct {
		mut  func(s *EnvironmentSnapshot)
		want string
	}{
		"id not derived":   {func(s *EnvironmentSnapshot) { s.ID = "snap-x" }, "id is not derived"},
		"environment name": {func(s *EnvironmentSnapshot) { s.Environment = "" }, "environment and producer are required"},
		"facts artifact":   {func(s *EnvironmentSnapshot) { s.Facts = "" }, "facts must be"},
		"no inputs":        {func(s *EnvironmentSnapshot) { s.Inputs = nil }, "at least one input"},
		"tree digest":      {func(s *EnvironmentSnapshot) { s.Repo.TreeDigest = "abc" }, "treeDigest"},
		"path xor value":   {func(s *EnvironmentSnapshot) { s.Inputs[1].Path = "x" }, "exactly one of path and value"},
		"input kind":       {func(s *EnvironmentSnapshot) { s.Inputs[0].Kind = "ftp" }, "unknown kind"},
		"input gap reason": {func(s *EnvironmentSnapshot) { s.Inputs[0].Status = CollectionPartial }, "requires a reason"},
		"live ok record": {func(s *EnvironmentSnapshot) {
			s.Inputs = append(s.Inputs, SnapshotInput{Kind: InputLiveCluster, Value: "kind-ri", Status: CollectionOK})
		}, "requires the live observation record"},
		"live fields": {func(s *EnvironmentSnapshot) { s.Live = &LiveObservation{Context: "kind-ri"} }, "live observation requires"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := newInvFixture(t).b.Snapshot
			c.mut(&s)
			wantInvErr(t, s.Validate(), c.want)
		})
	}
	s := newInvFixture(t).b.Snapshot
	s.Inputs = append(s.Inputs, SnapshotInput{Kind: InputValues, Path: "x.yaml", Status: CollectionUnavailable, Reason: "valuesFrom outside the repo"})
	if st, ok := s.InputStatus(InputValues); !ok || st != CollectionUnavailable {
		t.Errorf("InputStatus = %v %v", st, ok)
	}
	if _, ok := s.InputStatus(InputInventory); ok {
		t.Error("InputStatus must report an absent kind")
	}
}

func TestArtifactValidateRules(t *testing.T) {
	cases := map[string]struct {
		mut  func(a *ArtifactRef)
		want string
	}{
		"kind":            {func(a *ArtifactRef) { a.Kind = "blob" }, "unknown kind"},
		"origin":          {func(a *ArtifactRef) { a.Origin = "cloud" }, "unknown origin"},
		"path prefix":     {func(a *ArtifactRef) { a.Path = "upstream/x.yaml" }, "must start with"},
		"path traversal":  {func(a *ArtifactRef) { a.Path = "env/../../etc/passwd" }, "must not contain .."},
		"digest":          {func(a *ArtifactRef) { a.Digest = "md5:abc" }, "is not sha256"},
		"id derived":      {func(a *ArtifactRef) { a.ID = "art-x" }, "id is not derived"},
		"size":            {func(a *ArtifactRef) { a.Size = -1 }, "negative size"},
		"required fields": {func(a *ArtifactRef) { a.Producer = "" }, "mediaType, uri and producer are required"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			a := artifact(ArtifactEnvironmentFile, OriginEnvironment, "env/a.yaml", "a")
			c.mut(&a)
			wantInvErr(t, a.Validate(), c.want)
		})
	}
	m := BundleManifest{Artifacts: []ArtifactRef{
		artifact(ArtifactEnvironmentFile, OriginEnvironment, "env/b.yaml", "b"),
		artifact(ArtifactEnvironmentFile, OriginEnvironment, "env/a.yaml", "a"),
	}}
	wantInvErr(t, m.Validate(), "sorted by path")
	m.Artifacts[0] = m.Artifacts[1]
	wantInvErr(t, m.Validate(), "duplicate artifact id")
}

func TestGraphValidateRules(t *testing.T) {
	cases := map[string]struct {
		mut  func(g *RelationshipGraph)
		want string
	}{
		"id not derived":     {func(g *RelationshipGraph) { g.ID = "grf-x" }, "id is not derived"},
		"bundle and builder": {func(g *RelationshipGraph) { g.Builder = "" }, "bundle and builder are required"},
		"node id":            {func(g *RelationshipGraph) { g.Nodes[0].Key = "other" }, "id is not derived from kind+key"},
		"node kind": {func(g *RelationshipGraph) {
			g.Nodes[0].Kind = "lambda"
		}, "unknown kind"},
		"nodes sorted":          {func(g *RelationshipGraph) { g.Nodes[0], g.Nodes[1] = g.Nodes[1], g.Nodes[0] }, "nodes must be sorted"},
		"edges sorted":          {func(g *RelationshipGraph) { g.Edges[0], g.Edges[1] = g.Edges[1], g.Edges[0] }, "edges must be sorted"},
		"dangling edge":         {func(g *RelationshipGraph) { g.Nodes = g.Nodes[1:] }, "connects unknown nodes"},
		"edge id":               {func(g *RelationshipGraph) { g.Edges[0].Detail = "changed" }, "id is not derived from kind+from+to+detail"},
		"inferred needs reason": {func(g *RelationshipGraph) { g.Edges[0].Status = EdgeInferred }, "inferred edge requires a reason"},
		"missing needs reason":  {func(g *RelationshipGraph) { g.Edges[0].Status = EdgeMissing }, "missing edge requires a reason"},
		"established evidence": {func(g *RelationshipGraph) {
			g.Edges[0].Evidence, g.Edges[0].Artifacts = nil, nil
		}, "established edge requires evidence"},
		"mitigates only overrides": {func(g *RelationshipGraph) { g.Edges[0].Mitigates = []string{"ci-x"} }, "only overrides edges"},
		"agent edge inferred":      {func(g *RelationshipGraph) { g.Edges[0].Assertion = "asr-x" }, "always inferred"},
		"edge kind":                {func(g *RelationshipGraph) { g.Edges[0].Kind = "teleports" }, "unknown kind"},
		"edge rule":                {func(g *RelationshipGraph) { g.Edges[0].Rule = "" }, "rule is required"},
		"edge status":              {func(g *RelationshipGraph) { g.Edges[0].Status = "rumoured" }, "unknown status"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			g := *newInvFixture(t).g
			c.mut(&g)
			wantInvErr(t, g.Validate(), c.want)
		})
	}
	g := *newInvFixture(t).g
	e := g.Edges[0]
	e.To = e.From
	e.ID = EdgeID(e.Kind, e.From, e.To, e.Detail)
	wantInvErr(t, errorsJoin(e.validate()), "self-loop")
}

func errorsJoin(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	var b strings.Builder
	for _, e := range errs {
		fmt.Fprintln(&b, e)
	}
	return fmt.Errorf("%s", b.String())
}

func TestTraversalValidateRules(t *testing.T) {
	cases := map[string]struct {
		mut  func(fx *invFixture, tr *TraversalResult)
		want string
	}{
		"id not derived":      {func(_ *invFixture, tr *TraversalResult) { tr.ID = "trv-x" }, "id is not derived"},
		"graph and algorithm": {func(_ *invFixture, tr *TraversalResult) { tr.Algorithm = "" }, "graph and algorithm are required"},
		"positive bounds":     {func(_ *invFixture, tr *TraversalResult) { tr.Bound.MaxPaths = 0 }, "every bound must be positive"},
		"exceeds bound":       {func(_ *invFixture, tr *TraversalResult) { tr.Bound.MaxDepth = 2 }, "exceeds its bound"},
		"truncated reason":    {func(_ *invFixture, tr *TraversalResult) { tr.Truncated = true; tr.Complete = false }, "truncated and truncationReason go together"},
		"truncated not complete": {func(_ *invFixture, tr *TraversalResult) {
			tr.Truncated, tr.TruncationReason = true, "maxPaths reached"
		}, "complete = true, want false"},
		"visited sorted": {func(_ *invFixture, tr *TraversalResult) {
			tr.Visited[0], tr.Visited[1] = tr.Visited[1], tr.Visited[0]
		}, "visited must be sorted"},
		"root fields": {func(_ *invFixture, tr *TraversalResult) { tr.Roots[0].Rule = "" }, "root needs changeItem, node and rule"},
		"path id":     {func(_ *invFixture, tr *TraversalResult) { tr.Paths[0].ChangeItem = tr.Paths[1].ChangeItem }, "id is not derived from item+nodes+edges"},
		"path shape": {func(_ *invFixture, tr *TraversalResult) {
			tr.Paths[0].Edges = tr.Paths[0].Edges[:1]
		}, "needs n nodes and n-1 edges"},
		"path starts at root": {func(_ *invFixture, tr *TraversalResult) {
			p := &tr.Paths[1]
			p.Nodes = []string{tr.Roots[0].Node}
			p.ID = ImpactPathID(p.ChangeItem, p.Nodes, p.Edges)
		}, "does not start at an exposure root"},
		"duplicate path": {func(_ *invFixture, tr *TraversalResult) { tr.Paths = append(tr.Paths, tr.Paths[0]) }, "duplicate path"},
		"unresolved listed": {func(_ *invFixture, tr *TraversalResult) {
			tr.Paths[0].Status = EdgeInferred
		}, "no unresolved edge is listed"},
		"complete rule": {func(_ *invFixture, tr *TraversalResult) {
			tr.Paths[0].Status = EdgeInferred
			tr.Unresolved = []string{tr.Paths[0].Edges[0]}
		}, "complete = true, want false"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newInvFixture(t)
			tr := *fx.inv.Traversal
			tr.Visited = append([]string(nil), tr.Visited...)
			tr.Paths = append([]ImpactPath(nil), tr.Paths...)
			tr.Roots = append([]ExposureRoot(nil), tr.Roots...)
			c.mut(fx, &tr)
			wantInvErr(t, tr.Validate(), c.want)
		})
	}
	if tr := newInvFixture(t).inv.Traversal; !tr.CompleteFor(newInvFixture(t).itemA) || len(tr.PathsOf("ci-none")) != 0 {
		t.Error("CompleteFor/PathsOf helpers")
	}
}

func TestTraversalValidateAgainstGraph(t *testing.T) {
	cases := map[string]struct {
		mut  func(fx *invFixture, tr *TraversalResult, g *RelationshipGraph)
		want string
	}{
		"other graph": {func(_ *invFixture, tr *TraversalResult, _ *RelationshipGraph) { tr.Graph = "grf-other" }, "want grf-"},
		"root node": {func(_ *invFixture, tr *TraversalResult, _ *RelationshipGraph) {
			tr.Roots = append(tr.Roots, ExposureRoot{ChangeItem: "ci-x", Node: "gn-x", Rule: "r"})
		}, "root node gn-x is not in the graph"},
		"visited node": {func(_ *invFixture, tr *TraversalResult, _ *RelationshipGraph) {
			tr.Visited = append(tr.Visited, "gn-zzz")
		}, "visited node gn-zzz"},
		"edge connects": {func(_ *invFixture, tr *TraversalResult, _ *RelationshipGraph) {
			tr.Paths[0].Edges = []string{tr.Paths[0].Edges[1], tr.Paths[0].Edges[0], tr.Paths[0].Edges[2]}
		}, "does not connect"},
		"weakest status": {func(fx *invFixture, tr *TraversalResult, g *RelationshipGraph) {
			for i := range g.Edges {
				if g.Edges[i].ID == fx.edgeRef {
					g.Edges[i].Status, g.Edges[i].Reason = EdgeMissing, "secret not in repo"
				}
			}
		}, "want missing (weakest edge)"},
		"unresolved exact": {func(fx *invFixture, tr *TraversalResult, _ *RelationshipGraph) {
			tr.Unresolved = []string{fx.edgeRef}
		}, "is on no path"},
		"mitigation edge": {func(fx *invFixture, tr *TraversalResult, _ *RelationshipGraph) {
			tr.Paths[0].Mitigations = []string{fx.edgeRef}
		}, "is not an overrides edge into the path"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newInvFixture(t)
			tr := *fx.inv.Traversal
			tr.Paths = append([]ImpactPath(nil), tr.Paths...)
			g := *fx.g
			g.Edges = append([]GraphEdge(nil), g.Edges...)
			c.mut(fx, &tr, &g)
			wantInvErr(t, tr.ValidateAgainst(g), c.want)
		})
	}
}

func TestValidationRecordRules(t *testing.T) {
	cases := map[string]struct {
		mut  func(v *ValidationRecord)
		want string
	}{
		"id":               {func(v *ValidationRecord) { v.Inputs = append(v.Inputs, "art-x") }, "id is not derived"},
		"validator format": {func(v *ValidationRecord) { v.Validator = "render" }, "must be component@vN"},
		"kind":             {func(v *ValidationRecord) { v.Kind = "vibes" }, "unknown kind"},
		"level":            {func(v *ValidationRecord) { v.Level = "human" }, "unknown level"},
		"runtime observed": {func(v *ValidationRecord) { v.Kind = ValidationRuntime }, "runtime validations are observed"},
		"observed runtime": {func(v *ValidationRecord) { v.Level = LevelObserved }, "runtime validations are observed"},
		"outcome":          {func(v *ValidationRecord) { v.Outcome = "maybe" }, "unknown outcome"},
		"property":         {func(v *ValidationRecord) { v.Property = " " }, "changeItem and property are required"},
		"detail":           {func(v *ValidationRecord) { v.Detail = "" }, "outcome fail requires detail"},
		"inputs":           {func(v *ValidationRecord) { v.Inputs = nil }, "requires the input artifacts"},
		"evidence": {func(v *ValidationRecord) {
			v.Evidence, v.Outputs = nil, nil
		}, "requires evidence or output artifacts"},
		"never ai":         {func(v *ValidationRecord) { v.Provenance = aiProv("c", "ev-x") }, "never AI output"},
		"producer matches": {func(v *ValidationRecord) { v.Provenance.Producer = "other@v1" }, "must equal validator"},
		"bad provenance":   {func(v *ValidationRecord) { v.Provenance.Confidence = "certain" }, "invalid confidence"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			v := newInvFixture(t).inv.Validations[0]
			c.mut(&v)
			wantInvErr(t, v.Validate(), c.want)
		})
	}
	// not-run needs no inputs but a reason.
	v := newInvFixture(t).inv.Validations[0]
	v.Outcome, v.Inputs, v.Evidence, v.Outputs, v.Detail = ValidationNotRun, nil, nil, nil, "helm binary missing"
	v.ID = ValidationRecordID(v.Validator, v.ChangeItem, v.Subjects, v.Paths, v.RenderedChanges, v.Inputs)
	if err := v.Validate(); err != nil {
		t.Errorf("a not-run record with a reason is valid: %v", err)
	}
}

func TestAgentRecordRules(t *testing.T) {
	fx := newInvFixture(t)
	a := fx.inv.Assertions[0]
	cases := map[string]struct {
		mut  func(a *AgentAssertion)
		want string
	}{
		"ai provenance": {func(a *AgentAssertion) { a.Provenance = computed("x@v1") }, "agent records carry ai provenance"},
		"call id":       {func(a *AgentAssertion) { a.Provenance.CallID = "" }, "requires the callId"},
		"id derived":    {func(a *AgentAssertion) { a.Statement = "other" }, "id is not derived"},
		"stance":        {func(a *AgentAssertion) { a.Stance = "approves" }, "unknown stance"},
		"citations":     {func(a *AgentAssertion) { a.Citations = nil }, "must cite evidence"},
		"proposed edge inferred": {func(a *AgentAssertion) {
			e := GraphEdge{Kind: EdgeReferences, From: "gn-a", To: "gn-b", Status: EdgeEstablished, Rule: "agent", Evidence: a.Citations}
			e.ID = EdgeID(e.Kind, e.From, e.To, e.Detail)
			a.ProposedEdge = &e
		}, "a proposed edge is inferred and names this assertion"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			x := a
			c.mut(&x)
			wantInvErr(t, x.Validate(), c.want)
		})
	}
	ok := a
	e := GraphEdge{Kind: EdgeReferences, From: "gn-a", To: "gn-b", Status: EdgeInferred, Rule: "agent", Reason: "naming convention", Assertion: a.ID}
	e.ID = EdgeID(e.Kind, e.From, e.To, e.Detail)
	ok.ProposedEdge = &e
	if err := ok.Validate(); err != nil {
		t.Errorf("an inferred proposed edge is valid: %v", err)
	}

	h := Hypothesis{ChangeItem: fx.itemA, Statement: "s", Citations: a.Citations, Provenance: aiProv("c1", a.Citations...)}
	h.ID = AgentRecordID(HypothesisIDPrefix, h.ChangeItem, h.Statement, "c1")
	if err := h.Validate(); err != nil {
		t.Errorf("hypothesis: %v", err)
	}
	h.Citations = nil
	wantInvErr(t, h.Validate(), "statement and citations are required")

	r := EvidenceRequest{ChangeItem: fx.itemA, Kind: RequestArtifact, Target: "https://github.com/x/y/pull/1", Reason: "PR body has the default", Status: RequestRejected, Provenance: aiProv("c1", a.Citations...)}
	r.ID = AgentRecordID(EvidenceRequestIDPrefix, r.ChangeItem, string(r.Kind)+"\x00"+r.Target, "c1")
	wantInvErr(t, r.Validate(), "requires a resolution")
	r.Status = RequestFulfilled
	wantInvErr(t, r.Validate(), "a result is set exactly when fulfilled")
	r.Status, r.Kind = RequestOpen, "telepathy"
	wantInvErr(t, r.Validate(), "unknown kind")

	vr := fx.inv.ValidationRequests[0]
	vr.Result = ""
	wantInvErr(t, vr.Validate(), "a result is set exactly when fulfilled")
}

func TestFindingRules(t *testing.T) {
	cases := map[string]struct {
		idx  int // 0 = affected, 1 = not-affected
		mut  func(f *InvestigationFinding)
		want string
	}{
		"id derived": {0, func(f *InvestigationFinding) { f.Rule = "other" }, "id is not derived"},
		"class":      {0, func(f *InvestigationFinding) { f.Class = "affected-ish" }, "unknown class"},
		"reasoning":  {0, func(f *InvestigationFinding) { f.Reasoning = "" }, "reasoning is required"},
		"evidence":   {0, func(f *InvestigationFinding) { f.Evidence = nil }, "evidence is required"},
		"never ai":   {0, func(f *InvestigationFinding) { f.Provenance = aiProv("c", "ev-x") }, "never by a model"},
		"basis":      {0, func(f *InvestigationFinding) { f.Basis = "human" }, "unknown basis"},
		"gap shape":  {0, func(f *InvestigationFinding) { f.Gaps = []EvidenceGap{{Kind: "vibes"}} }, "gap needs a known kind"},
		"gap reason": {0, func(f *InvestigationFinding) {
			f.Gaps = []EvidenceGap{{Kind: GapAgent, Subject: "s", Detail: "d", UnknownReason: "x"}}
		}, "unknown unknownReason"},
		"action never weakened": {1, func(f *InvestigationFinding) {
			f.ImpactFindings = []ImpactFindingRef{{ID: "imp-x", Classification: ImpactActionRequired, Rule: "impact:crd-removed"}}
		}, "must be affected"},
		"affected needs validation": {0, func(f *InvestigationFinding) { f.Validations = nil }, "affected requires a failing"},
		"not-affected no gaps": {1, func(f *InvestigationFinding) {
			f.Gaps = []EvidenceGap{{Kind: GapCollection, Subject: "source-diff", Detail: "unavailable"}}
		}, "not-affected carries no evidence gaps"},
		"not-affected validations": {1, func(f *InvestigationFinding) { f.Validations = nil }, "requires deterministic or observed validations"},
		"not-affected assumptions": {1, func(f *InvestigationFinding) { f.Assumptions = f.Assertions }, "no unvalidated assumption"},
		"potentially next steps": {0, func(f *InvestigationFinding) {
			f.Class, f.Validations, f.Basis = FindingPotentiallyAffected, nil, ""
		}, "potentially-affected requires the next investigation steps"},
		"insufficient gaps": {0, func(f *InvestigationFinding) {
			f.Class, f.NextSteps = FindingInsufficientEvidence, []string{"supply values"}
		}, "insufficient-evidence names its evidence gaps"},
		"insufficient next": {0, func(f *InvestigationFinding) {
			f.Class, f.Gaps = FindingInsufficientEvidence, []EvidenceGap{{Kind: GapExposure, Subject: "values", Detail: "not supplied"}}
		}, "insufficient-evidence requires the next investigation steps"},
		"eligible only not-affected": {0, func(f *InvestigationFinding) {
			f.Approval = ApprovalEligibility{Eligible: true}
		}, "only a not-affected finding can be eligible"},
		"review negation": {1, func(f *InvestigationFinding) { f.Approval.RequiresHumanReview = true }, "negation of eligible"},
		"blockers": {1, func(f *InvestigationFinding) {
			f.Approval = ApprovalEligibility{RequiresHumanReview: true}
		}, "blockers are listed exactly when not eligible"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newInvFixture(t).inv.Findings[c.idx]
			c.mut(&f)
			wantInvErr(t, f.Validate(), c.want)
		})
	}
}

// rederive refreshes a finding's id after a mutation, so a test isolates
// the rule under test from the identity check.
func rederive(f *InvestigationFinding) {
	f.ID = InvestigationFindingID(f.Environment, f.ChangeItem, f.Class, f.Rule)
}

func TestInvestigationPolicyRules(t *testing.T) {
	cases := map[string]struct {
		mut  func(fx *invFixture)
		want string
	}{
		"schema version": {func(fx *invFixture) { fx.inv.SchemaVersion = "x" }, "schemaVersion"},
		"id derived":     {func(fx *invFixture) { fx.inv.Config = digestOf("other") }, "id is not derived from bundle+config"},
		"config digest":  {func(fx *invFixture) { fx.inv.Config = "abc" }, "config (a digest)"},
		"status":         {func(fx *invFixture) { fx.inv.Status = "paused" }, "unknown status"},
		"duplicate finding": {func(fx *invFixture) {
			f := fx.inv.Findings[0]
			f.Rule = "classify:other"
			rederive(&f)
			fx.inv.Findings = append(fx.inv.Findings, f)
		}, "has more than one finding"},
		"finding environment": {func(fx *invFixture) {
			fx.inv.Findings[0].Environment = "staging"
			rederive(&fx.inv.Findings[0])
		}, `is for environment "staging"`},
		"affected cites failing": {func(fx *invFixture) {
			fx.inv.Findings[0].Validations = []string{fx.inv.Validations[1].ID}
		}, "is not a validation of"},
		"basis is strongest": {func(fx *invFixture) {
			fx.inv.Findings[1].Basis = LevelObserved
		}, "basis observed"},
		"not-affected complete traversal": {func(fx *invFixture) {
			tr := fx.inv.Traversal
			tr.Truncated, tr.TruncationReason, tr.Complete = true, "maxNodes reached", false
			tr.ID = tr.DerivedID()
		}, "requires a complete traversal for the item"},
		"not-affected contradicted": {func(fx *invFixture) {
			v := fx.inv.Validations[0]
			v.ChangeItem, v.Paths = fx.itemB, []string{fx.pathB}
			v.ID = ValidationRecordID(v.Validator, v.ChangeItem, v.Subjects, v.Paths, v.RenderedChanges, v.Inputs)
			fx.inv.Validations = append(fx.inv.Validations, v)
		}, "not-affected contradicts validation"},
		"not-affected path coverage": {func(fx *invFixture) {
			v := &fx.inv.Validations[1]
			v.Paths = nil
			v.ID = ValidationRecordID(v.Validator, v.ChangeItem, v.Subjects, v.Paths, v.RenderedChanges, v.Inputs)
			fx.inv.Findings[1].Validations = []string{v.ID}
			fx.inv.ValidationRequests[0].Result = v.ID
		}, "is covered by no passing validation"},
		"concern answered": {func(fx *invFixture) {
			fx.inv.Validations[1].Answers = nil
		}, "is not answered by a passing validation"},
		"assumption listed": {func(fx *invFixture) {
			fx.inv.Findings[0].Assumptions = []string{fx.inv.Assertions[1].ID}
			fx.inv.Findings[0].Assertions = nil
		}, "must also be listed in assertions"},
		"assertion of item": {func(fx *invFixture) {
			fx.inv.Findings[0].Assertions = []string{fx.inv.Assertions[0].ID}
		}, "is not an assertion about"},
		"path of item": {func(fx *invFixture) {
			fx.inv.Findings[0].Paths = []string{fx.pathB}
		}, "is not a path of"},
		"validation path": {func(fx *invFixture) {
			fx.inv.Validations[0].Paths = []string{fx.pathB}
		}, "covers unknown path"},
		"validation answers": {func(fx *invFixture) {
			fx.inv.Validations[0].Answers = []string{"asr-nope"}
		}, "answers unknown assertion"},
		"validation request": {func(fx *invFixture) {
			fx.inv.Validations[1].Request = "vrq-nope"
		}, "names unknown request"},
		"request result": {func(fx *invFixture) {
			fx.inv.ValidationRequests[0].Result = fx.inv.Validations[0].ID
		}, "is not a validation run for it"},
		"assertion hypothesis": {func(fx *invFixture) {
			fx.inv.Assertions[1].Hypothesis = "hyp-nope"
		}, "names unknown hypothesis"},
		"decision contradicts": {func(fx *invFixture) {
			fx.inv.Decision.Outcome = PolicyAutoApproveEligible
		}, "contradicts the findings"},
		"decision enabled": {func(fx *invFixture) {
			fx.inv.Decision.AutoApprovalEnabled = true
		}, "can only be enabled for an eligible outcome"},
		"decision shape": {func(fx *invFixture) { fx.inv.Decision.Reasons = nil }, "decision needs policy"},
		"eligible needs agent": {func(fx *invFixture) {
			fx.inv.Findings = fx.inv.Findings[1:]
			fx.inv.Decision.Outcome = PolicyAutoApproveEligible
			fx.inv.Agent.Status, fx.inv.Agent.Reason = AgentBudgetExhausted, "maxToolCalls"
			fx.inv.Review.State = ReviewNotRequired
		}, "requires a completed agent stage"},
		"agent reason":              {func(fx *invFixture) { fx.inv.Agent.Status = AgentFailed }, "requires a reason"},
		"agent record":              {func(fx *invFixture) { fx.inv.Agent.Transcript = "" }, "records model, promptVersion"},
		"agent transcript artifact": {func(fx *invFixture) { fx.inv.Agent.Transcript = "art-nope" }, "is not a run artifact"},
		"review required":           {func(fx *invFixture) { fx.inv.Review.State = ReviewNotRequired }, "cannot be not-required"},
		"review decided":            {func(fx *invFixture) { fx.inv.Review.State = ReviewRejected }, "records reviewer and decidedAt"},
		"proxy never approves": {func(fx *invFixture) {
			at := fixNow
			p := aiProv("proxy-1", "ev-x")
			fx.inv.Review = HumanReview{State: ReviewApproved, Reviewer: "opus-proxy", ReviewerKind: ReviewerProxy, DecidedAt: &at, ProxyProvenance: &p}
		}, "a proxy (AI) reviewer can never approve"},
		"proxy provenance": {func(fx *invFixture) {
			at := fixNow
			fx.inv.Review = HumanReview{State: ReviewRejected, Reviewer: "opus-proxy", ReviewerKind: ReviewerProxy, DecidedAt: &at}
		}, "a proxy reviewer carries ai provenance"},
		"approval human": {func(fx *invFixture) {
			at := fixNow
			fx.inv.Review = HumanReview{State: ReviewApproved, Reviewer: "tommy", DecidedAt: &at}
		}, "approval requires a human reviewer"},
		"audit empty": {func(fx *invFixture) { fx.inv.Audit = nil }, "must not be empty"},
		"audit chain": {func(fx *invFixture) { fx.inv.Audit[0].Detail = "rewritten" }, "breaks the hash chain"},
		"audit seq":   {func(fx *invFixture) { fx.inv.Audit[1].Seq = 5 }, "must be contiguous"},
		"audit time": {func(fx *invFixture) {
			fx.inv.Audit = nil
			fx.inv.AppendAudit(AuditEvent{At: fixNow, Stage: StageTraverse, Event: "completed", Actor: "system"})
			fx.inv.AppendAudit(AuditEvent{At: fixNow.Add(-time.Second), Stage: StageClassify, Event: "completed", Actor: "system"})
		}, "goes back in time"},
		"audit stage": {func(fx *invFixture) {
			fx.inv.Audit = nil
			fx.inv.AppendAudit(AuditEvent{At: fixNow, Stage: "deploy", Event: "completed", Actor: "system"})
		}, "needs a known stage"},
		"run evidence": {func(fx *invFixture) { fx.inv.Evidence = append(fx.inv.Evidence, fx.inv.Evidence[0]) }, "unique id"},
		"fulfilled artifact": {func(fx *invFixture) {
			r := EvidenceRequest{ChangeItem: fx.itemA, Kind: RequestArtifact, Target: "t", Reason: "r", Status: RequestFulfilled,
				Fulfilled: []string{"art-nope"}, Provenance: aiProv("session-1", "ev-x")}
			r.ID = AgentRecordID(EvidenceRequestIDPrefix, r.ChangeItem, string(r.Kind)+"\x00"+r.Target, "session-1")
			fx.inv.EvidenceRequests = []EvidenceRequest{r}
		}, "is not a run artifact"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newInvFixture(t)
			c.mut(fx)
			wantInvErr(t, fx.inv.Validate(), c.want)
		})
	}
}

func TestInvestigationValidateAgainstBundle(t *testing.T) {
	cases := map[string]struct {
		mut  func(fx *invFixture)
		want string
	}{
		"other bundle":        {func(fx *invFixture) { fx.inv.Environment = "staging" }, "does not belong to bundle"},
		"citation resolves":   {func(fx *invFixture) { fx.inv.Evidence = nil }, "does not resolve"},
		"validation artifact": {func(fx *invFixture) { fx.inv.Validations[0].Outputs = []string{"art-nope"} }, "artifact art-nope does not resolve"},
		"report artifact":     {func(fx *invFixture) { fx.inv.ImpactReport = "art-nope" }, "impactReport: artifact"},
		"unknown item": {func(fx *invFixture) {
			fx.inv.EvidenceRequests = []EvidenceRequest{{ChangeItem: "ci-nope"}}
		}, "unknown change item ci-nope"},
		"in-scope item finding": {func(fx *invFixture) {
			fx.inv.Findings = fx.inv.Findings[:1]
		}, "has no finding"},
		"out-of-scope finding": {func(fx *invFixture) {
			f := fx.inv.Findings[0]
			f.ChangeItem = fx.itemC
			rederive(&f)
			fx.inv.Findings = append(fx.inv.Findings, f)
		}, "classifies out-of-scope item"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newInvFixture(t)
			c.mut(fx)
			wantInvErr(t, fx.inv.ValidateAgainst(fx.b), c.want)
		})
	}
}

// TestAgentCannotLowerAClass pins the central fail-closed property: no
// agent record can make a finding not-affected. A claims-mitigation
// assertion alone (no validation) leaves the item unprovable, and the
// not-affected shape is rejected.
func TestAgentCannotLowerAClass(t *testing.T) {
	fx := newInvFixture(t)
	// Replace B's validated clearing with an agent's mitigation claim only.
	claim := AgentAssertion{ChangeItem: fx.itemB, Stance: StanceClaimsMitigation, Statement: "the override neutralises it",
		Citations: fx.inv.Assertions[0].Citations, Provenance: aiProv("session-1", fx.inv.Assertions[0].Citations...)}
	claim.ID = AgentRecordID(AssertionIDPrefix, claim.ChangeItem, string(claim.Stance)+"\x00"+claim.Statement, "session-1")
	fx.inv.Assertions = append(fx.inv.Assertions, claim)
	fx.inv.Validations = fx.inv.Validations[:1]
	fx.inv.ValidationRequests = nil
	f := &fx.inv.Findings[1]
	f.Validations, f.Assertions = nil, []string{claim.ID}
	wantInvErr(t, fx.inv.Validate(), "requires deterministic or observed validations")
}

// TestInvestigationSamples pins the checked-in sample documents to the
// fixture. They are example documents for the next lanes and the input of
// the schema test in internal/domain/schemagen. Rewrite them with
// RI_UPDATE_SAMPLES=1 go test ./internal/domain -run TestInvestigationSamples.
func TestInvestigationSamples(t *testing.T) {
	fx := newInvFixture(t)
	for file, v := range map[string]any{
		"testdata/sample-evidence-bundle.json":     fx.b,
		"testdata/sample-investigation.json":       fx.inv,
		"testdata/sample-investigation-graph.json": fx.g,
	} {
		want, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, '\n')
		if os.Getenv("RI_UPDATE_SAMPLES") != "" {
			if err := os.MkdirAll("testdata", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%v (rewrite with RI_UPDATE_SAMPLES=1)", err)
		}
		if string(got) != string(want) {
			t.Errorf("%s is stale; rewrite with RI_UPDATE_SAMPLES=1 go test ./internal/domain -run TestInvestigationSamples", file)
		}
	}
}

func TestRenderedChangeRules(t *testing.T) {
	pairCases := map[string]struct {
		mut  func(p *ManifestPair)
		want string
	}{
		"id derived":  {func(p *ManifestPair) { p.Changes[0].Path = "other" }, "id is not derived from scope+target+object+path+class"},
		"class":       {func(p *ManifestPair) { p.Changes[0].Class = "" }, "needs class and object"},
		"sensitive":   {func(p *ManifestPair) { p.Changes[0].Sensitive = true }, "is sensitive; its values are never recorded"},
		"correlation": {func(p *ManifestPair) { p.Changes[0].Correlation = "" }, "correlation is set exactly when change items are"},
		"items sorted": {func(p *ManifestPair) {
			p.Changes[0].ChangeItems = []string{"ci-b", "ci-a"}
		}, "changeItems must be sorted"},
		"sorted unique": {func(p *ManifestPair) { p.Changes = append(p.Changes, p.Changes[0]) }, "must be sorted by id and unique"},
		"incomplete render has no diff": {func(p *ManifestPair) {
			p.Status, p.Reason = CollectionPartial, "template-error"
		}, "an incomplete render is an evidence gap"},
	}
	for name, c := range pairCases {
		t.Run(name, func(t *testing.T) {
			fx := newInvFixture(t)
			p := fx.b.Renders[0]
			p.Changes = append([]RenderedChange(nil), p.Changes...)
			c.mut(&p)
			wantInvErr(t, p.validate(RenderEnvironment), c.want)
		})
	}
	bundleCases := map[string]struct {
		mut  func(fx *invFixture)
		want string
	}{
		"unknown correlated item": {func(fx *invFixture) {
			fx.b.Renders[0].Changes[0].ChangeItems = []string{"ci-nope"}
		}, "is correlated to unknown change item ci-nope"},
		"rendered evidence": {func(fx *invFixture) {
			fx.b.Renders[0].Changes[0].Evidence = []EvidenceID{"ev-nope"}
		}, "evidence ev-nope does not resolve"},
		"release anchor": {func(fx *invFixture) {
			it := &fx.b.Change.Items[2]
			it.ChangeID, it.RenderedChange, it.Family = "", "rc-nope", FamilyRendered
			it.ID = ChangeItemID(it.ChangeID, it.Platform, it.RenderedChange)
		}, "is not a release-scope rendered change of this record"},
		"rendered family": {func(fx *invFixture) {
			it := &fx.b.Change.Items[2]
			it.ChangeID, it.RenderedChange = "", "rc-nope"
			it.ID = ChangeItemID(it.ChangeID, it.Platform, it.RenderedChange)
		}, "a rendered-change item has family rendered"},
	}
	for name, c := range bundleCases {
		t.Run(name, func(t *testing.T) {
			fx := newInvFixture(t)
			c.mut(fx)
			wantInvErr(t, fx.b.Validate(), c.want)
		})
	}
	invCases := map[string]struct {
		mut  func(fx *invFixture)
		want string
	}{
		"finding lists correlated changes": {func(fx *invFixture) {
			fx.inv.Findings[0].RenderedChanges = nil
		}, "the item's correlated environment changes are"},
		"unknown rendered change": {func(fx *invFixture) {
			fx.inv.Assertions[0].RenderedChanges = []string{"rc-nope"}
		}, "rendered change rc-nope is not in the bundle"},
		"uncorrelated blocks auto-approval": {func(fx *invFixture) {
			fx.b.Renders[0].Changes[0].ChangeItems, fx.b.Renders[0].Changes[0].Correlation = nil, ""
			fx.inv.Decision.Outcome = PolicyAutoApproveEligible
		}, "rendered changes no change item explains"},
		"auto-approval needs a render": {func(fx *invFixture) {
			fx.b.Renders = nil
			fx.inv.Decision.Outcome = PolicyAutoApproveEligible
		}, "requires a complete environment render"},
		"auto-approval needs complete renders": {func(fx *invFixture) {
			fx.b.Renders[0].Status, fx.b.Renders[0].Reason, fx.b.Renders[0].Changes = CollectionUnavailable, "chart-unavailable", nil
			fx.inv.Decision.Outcome = PolicyAutoApproveEligible
		}, "requires every environment render to be complete"},
	}
	for name, c := range invCases {
		t.Run(name, func(t *testing.T) {
			fx := newInvFixture(t)
			c.mut(fx)
			wantInvErr(t, fx.inv.ValidateAgainst(fx.b), c.want)
		})
	}
	// A not-affected item whose render shows a correlated change needs a
	// passing validation that covers that change.
	fx := newInvFixture(t)
	f := &fx.inv.Findings[1]
	f.RenderedChanges = []string{fx.rcA}
	wantInvErr(t, fx.inv.Validate(), "is shown harmless by no passing validation")
	// Root grounding is single-sourced.
	tr := *fx.inv.Traversal
	tr.Roots = append([]ExposureRoot(nil), tr.Roots...)
	tr.Roots[0].ImpactFinding = "imp-a"
	wantInvErr(t, tr.Validate(), "at most one of renderedChange and impactFinding")
}
