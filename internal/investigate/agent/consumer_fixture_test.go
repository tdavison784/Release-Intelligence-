package agent

import (
	"context"
	"encoding/json"
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

// consumerFixture is a synthetic investigation for prompt v9 (lane consumer):
// an invented product "keystore" 2.0.0 → 2.1.0 whose controller now rotates a
// Credential's private key on every renewal (a controller-behaviour item). In
// the environment a Credential produces a Secret at runtime (not in the
// snapshot); the "billing" Deployment mounts it and a ConfigMap whose
// configuration names the mounted key file — the chain
// Credential → Secret → Deployment → ConfigMap. The configuration also names
// a log path under no mount. No evaluation case is modelled.
type consumerFixture struct {
	st       *store.Store
	b        *domain.EvidenceBundle
	g        *domain.RelationshipGraph
	trav     *domain.TraversalResult
	item     string
	notes    domain.ArtifactRef
	config   domain.ArtifactRef // env file holding the ConfigMap
	deploy   domain.ArtifactRef // env file holding the Deployment
	toRender domain.ArtifactRef
	overlay  domain.ArtifactRef // configGeneratedRendered: the overlay's render
}

const (
	cfCredential = "example.dev/v1/Credential/shop/billing-tls"
	cfSecret     = "core/v1/Secret/shop/billing-tls"
	cfDeployment = "apps/v1/Deployment/shop/billing"
	cfConfigMap  = "core/v1/ConfigMap/shop/billing-config"
)

const keystoreNotes = `# keystore 2.1.0

## Breaking changes

- The controller now rotates a Credential's private key on every renewal: the default of
  ` + "`spec.rotate`" + ` changed from false to true. Set ` + "`spec.rotate: false`" + ` to keep the key.
`

const consumerConfig = `apiVersion: v1
kind: ConfigMap
metadata:
  name: billing-config
  namespace: shop
data:
  app.ini: |
    [tls]
    key_file = /etc/billing/tls/tls.key
    log = /var/log/billing/app.log
`

const consumerDeploy = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: billing
  namespace: shop
spec:
  template:
    spec:
      volumes:
        - name: tls
          secret:
            secretName: billing-tls
        - name: conf
          configMap:
            name: billing-config
      containers:
        - name: app
          volumeMounts:
            - name: tls
              mountPath: /etc/billing/tls
            - name: conf
              mountPath: /etc/billing/conf
`

const consumerCredential = `apiVersion: example.dev/v1
kind: Credential
metadata:
  name: billing-tls
  namespace: shop
spec:
  secretName: billing-tls
`

// fixtureConfig is how the fixture declares billing's configuration.
type fixtureConfig int

const (
	// configObject: a ConfigMap manifest (config.yaml).
	configObject fixtureConfig = iota
	// configGenerated: a kustomize configMapGenerator whose source file
	// (app.ini) the snapshot lacks, as it lacks every non-YAML file.
	configGenerated
	// configGeneratedRendered: configGenerated, and the overlay's render (a
	// version-independent overlay: one render, both sides) holds the
	// generated object under its hash-suffixed name.
	configGeneratedRendered
)

const consumerKustomization = `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: shop
resources:
  - credential.yaml
  - deployment.yaml
configMapGenerator:
  - name: billing-config
    files:
      - app.ini
`

// consumerOverlayRender is kustomize's output for apps/billing.
const consumerOverlayRender = `apiVersion: v1
data:
  app.ini: |
    [tls]
    key_file = /etc/billing/tls/tls.key
    log = /var/log/billing/app.log
kind: ConfigMap
metadata:
  name: billing-config-5g7k9b2mdh
  namespace: shop
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: billing
  namespace: shop
spec:
  template:
    spec:
      containers:
      - name: app
        volumeMounts:
        - mountPath: /etc/billing/tls
          name: tls
        - mountPath: /etc/billing/conf
          name: conf
      volumes:
      - name: tls
        secret:
          secretName: billing-tls
      - configMap:
          name: billing-config-5g7k9b2mdh
        name: conf
`

func newConsumerFixture(t *testing.T) *consumerFixture {
	t.Helper()
	return newConsumerFixtureWith(t, configObject)
}

func newConsumerFixtureWith(t *testing.T, mode fixtureConfig) *consumerFixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.New(t.TempDir())
	must(t, err)
	f := &consumerFixture{st: st}
	put := func(spec investigate.ArtifactSpec, b []byte) domain.ArtifactRef {
		ref, err := st.Put(ctx, spec, b)
		must(t, err)
		return ref
	}
	notesURI := "https://example.dev/keystore/releases/v2.1.0"
	f.notes = put(investigate.ArtifactSpec{Kind: domain.ArtifactUpstreamDocument, Path: "upstream/keystore/v2.1.0/release-notes.md", Origin: domain.OriginUpstream,
		MediaType: "text/markdown", URI: notesURI, Producer: "test.collect@v1", Tags: []string{"release-notes", "keystore"}}, []byte(keystoreNotes))
	ev := domain.NewEvidence(domain.EvidenceDocument, f.notes.ID, notesURI, "## Breaking changes", "The controller now rotates a Credential's private key on every renewal", f.notes.Digest, time.Time{})
	edge := domain.UpgradeEdge{Product: domain.ProductRef{ID: "keystore"}, From: domain.Version{Tag: "v2.0.0"}, To: domain.Version{Tag: "v2.1.0"},
		Changes: []domain.Change{{ID: "chg-rotate", Category: domain.Category("behavior"), Breaking: true, Title: "Private keys rotate on renewal by default",
			Release: "v2.1.0", Detail: "Set `spec.rotate: false` to keep the key.", Subjects: []string{"Credential.spec.rotate"}, Evidence: []domain.EvidenceID{ev.ID},
			Provenance: domain.Provenance{Method: domain.MethodHeuristic, Producer: "normalize.releasenotes@v1", Rule: "section:/breaking/i", Confidence: domain.ConfidenceMedium}}},
		Evidence: []domain.Evidence{ev}}
	eb, err := json.MarshalIndent(edge, "", "  ")
	must(t, err)
	edgeRef := put(investigate.ArtifactSpec{Kind: domain.ArtifactUpgradeEdge, Path: "derived/upgrade/edge.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:upgrade@v1", Producer: "upgrade@v1"}, eb)
	it := domain.ChangeItem{ID: domain.ChangeItemID("chg-rotate", "", ""), ChangeID: "chg-rotate", Family: domain.FamilyBehavior,
		Title: "Private keys rotate on renewal by default", Breaking: true, Subjects: []string{"Credential.spec.rotate"}, Evidence: []domain.EvidenceID{ev.ID},
		Manifestation: domain.ManifestControllerBehaviour, ManifestationRule: "manifest:declared-behaviour", InScope: true, ScopeRule: "scope:breaking"}
	f.item = it.ID
	rec := domain.ChangeEvidenceRecord{Product: edge.Product, From: edge.From, To: edge.To, Edge: edgeRef.ID, Items: []domain.ChangeItem{it}, Producer: "test.collect@v1"}
	for _, k := range domain.ChangeSourceKinds {
		s := domain.ChangeSourceStatus{Source: k, Status: domain.CollectionNotApplicable, Reason: "not part of this fixture"}
		if k == domain.ChangeSourceReleaseNotes {
			s = domain.ChangeSourceStatus{Source: k, Status: domain.CollectionOK, Artifacts: []string{f.notes.ID}}
		}
		rec.Sources = append(rec.Sources, s)
	}
	rec.ID = rec.DerivedID()
	change := &investigate.ChangeResult{Record: rec, Artifacts: []domain.ArtifactRef{edgeRef, f.notes}, Evidence: edge.Evidence}

	envFile := func(p, content string) domain.ArtifactRef {
		return put(investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFile, Path: "env/" + p, Origin: domain.OriginEnvironment,
			MediaType: "application/yaml", URI: p, Producer: "test.snapshot@v1", Tags: []string{"manifests"}}, []byte(content))
	}
	cred := envFile("apps/billing/credential.yaml", consumerCredential)
	f.deploy = envFile("apps/billing/deployment.yaml", consumerDeploy)
	if mode == configObject {
		f.config = envFile("apps/billing/config.yaml", consumerConfig)
	} else {
		f.config = envFile("apps/billing/kustomization.yaml", consumerKustomization)
	}
	envArts := []domain.ArtifactRef{cred, f.deploy, f.config}
	factsArt := put(investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFacts, Path: "derived/env/facts.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:env@v1", Producer: "test.snapshot@v1"}, []byte(`{"name":"shop"}`))
	var envIDs []string
	for _, a := range envArts {
		envIDs = append(envIDs, a.ID)
	}
	sort.Strings(envIDs)
	snap := domain.EnvironmentSnapshot{Environment: "shop", Repo: &domain.RepoRef{Path: "env", TreeDigest: domain.Digest([]byte("keystore-tree"))},
		Inputs: []domain.SnapshotInput{{Kind: domain.InputGitOpsRepo, Path: "env", Status: domain.CollectionOK, Artifacts: envIDs}},
		Facts:  factsArt.ID, Producer: "test.snapshot@v1"}
	snap.ID = snap.DerivedID()
	snapRes := &investigate.SnapshotResult{Snapshot: snap, Artifacts: append(envArts, factsArt)}

	// The environment render: the product's controller, from and to.
	ctlID := render.ObjectID{Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "keystore", Name: "keystore-controller"}
	ctl := func(tag string) []render.Object {
		return []render.Object{{ID: ctlID, Body: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]any{"name": "keystore-controller", "namespace": "keystore"},
			"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
				"containers": []any{map[string]any{"name": "controller", "image": "example.dev/keystore:" + tag}}}}}}}}
	}
	fromB, _ := renderdiff.EncodeObjects(ctl("v2.0.0"))
	toB, _ := renderdiff.EncodeObjects(ctl("v2.1.0"))
	fromRef := put(investigate.ArtifactSpec{Kind: domain.ArtifactRenderedManifests, Path: "derived/render/env/argocd-keystore/from.yaml", Origin: domain.OriginDerived,
		MediaType: "application/yaml", URI: "ri:renderdiff@v1", Producer: "test.render@v1"}, fromB)
	f.toRender = put(investigate.ArtifactSpec{Kind: domain.ArtifactRenderedManifests, Path: "derived/render/env/argocd-keystore/to.yaml", Origin: domain.OriginDerived,
		MediaType: "application/yaml", URI: "ri:renderdiff@v1", Producer: "test.render@v1"}, toB)
	diffRef := put(investigate.ArtifactSpec{Kind: domain.ArtifactRenderDiff, Path: "derived/render/env/argocd-keystore/diff.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:renderdiff@v1", Producer: "test.render@v1"}, []byte(`{"changes":1}`))
	target := "argocd:apps/keystore.yaml"
	rc := domain.RenderedChange{Class: "image-changed", Object: ctlID.String(), Path: "spec.template.spec.containers[name=controller].image",
		Before: `"example.dev/keystore:v2.0.0"`, After: `"example.dev/keystore:v2.1.0"`, ChangeItems: []string{it.ID}, Correlation: "subject"}
	rc.ID = domain.RenderedChangeID(domain.RenderEnvironment, target, rc.Object, rc.Path, rc.Class)
	rcEv := domain.NewEvidence(domain.EvidenceRenderedDiff, diffRef.ID, "ri:renderdiff@v1", "changes[id="+rc.ID+"]", rc.Class+" "+rc.Object, diffRef.Digest, time.Time{})
	rc.Evidence = []domain.EvidenceID{rcEv.ID}
	snapRes.Evidence = append(snapRes.Evidence, rcEv)
	pair := domain.ManifestPair{Scope: domain.RenderEnvironment, Target: target, From: fromRef.ID, To: f.toRender.ID, Diff: diffRef.ID,
		Status: domain.CollectionOK, Changes: []domain.RenderedChange{rc}}
	renders := &investigate.RenderResult{Pairs: []domain.ManifestPair{pair}, Artifacts: []domain.ArtifactRef{fromRef, f.toRender, diffRef}}
	if mode == configGeneratedRendered {
		f.overlay = put(investigate.ArtifactSpec{Kind: domain.ArtifactRenderedManifests, Path: "derived/render/env/kustomize-apps-billing/from.yaml", Origin: domain.OriginDerived,
			MediaType: "application/yaml", URI: "ri:renderdiff@v1", Producer: "test.render@v1"}, []byte(consumerOverlayRender))
		renders.Pairs = append(renders.Pairs, domain.ManifestPair{Scope: domain.RenderEnvironment, Target: "kustomize:apps/billing", From: f.overlay.ID,
			Status: domain.CollectionNotApplicable, Reason: "version-independent: the overlay does not deploy keystore, so the upgrade does not change what it renders"})
		renders.Artifacts = append(renders.Artifacts, f.overlay)
	}
	b, err := bundle.NewAssembler(st).Assemble(ctx, change, snapRes, renders)
	must(t, err)
	must(t, st.PutBundle(ctx, b))
	f.b = b

	// The stored graph (no consumer mode) and the item's root: the Credential.
	g, err := relations.NewBuilder(st).BuildGraph(ctx, b)
	must(t, err)
	must(t, g.Validate())
	f.g = g
	root := domain.ExposureRoot{ChangeItem: it.ID, Node: domain.NodeID(domain.NodeCustomResource, cfCredential), Rule: "exposure:join-match"}
	trav, err := relations.NewTraverser().Traverse(ctx, g, []domain.ExposureRoot{root}, domain.DefaultTraversalBound)
	must(t, err)
	f.trav = trav
	return f
}

func (f *consumerFixture) input(cfg investigate.AgentConfig) investigate.InvestigateInput {
	return investigate.InvestigateInput{Bundle: f.b, Graph: f.g, Traversal: f.trav, Config: cfg}
}

func (f *consumerFixture) upstreamCite() []map[string]any {
	return []map[string]any{{"artifact": f.notes.ID, "locator": "## Breaking changes", "quote": "The controller now rotates a Credential's private key on every renewal"}}
}

func v9Config() investigate.AgentConfig {
	return investigate.AgentConfig{Enabled: true, Driver: DriverScripted, Model: "test-model", PromptVersion: promptV9}
}

// v9Run runs the fixture's item under v9 with the scripted steps.
func v9Run(t *testing.T, f *consumerFixture, steps ...Step) (*Scripted, *investigate.AgentOutcome) {
	t.Helper()
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: steps}}
	out, err := New(f.st, WithDriver(DriverScripted, d), WithClock(fixedNow)).Investigate(context.Background(), f.input(v9Config()))
	must(t, err)
	return d, out
}

// v9Submit is a closing step with a verdict of class and an assessment.
func (f *consumerFixture) v9Submit(class string, assessment map[string]any) Step {
	cite := f.upstreamCite()
	return Step{Call: "call-9", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.item,
		"disposition": "explains", "summary": "the key rotates; the billing app reads the mounted key file", "citations": cite,
		"verdict": map[string]any{"class": class, "confidence": "medium", "mechanism": "the key rotates on renewal",
			"environmentReasoning": "billing mounts the Secret and its configuration names the key file", "wouldOverturn": "a preserving setting"},
		"assessment": assessment})}
}

func (f *consumerFixture) assessment() map[string]any {
	return map[string]any{
		"whatChanged": "the controller rotates the private key on every renewal by default",
		"mechanism":   "a consumer that caches the key, or pins it, sees a new key after renewal",
		"consumers":   []string{"Deployment shop/billing: Credential → Secret billing-tls → mounted at /etc/billing/tls → app.ini key_file"},
		"effectiveConfiguration": []map[string]any{
			{"setting": "Credential.spec.rotate", "state": "inherited-default", "detail": "unset in credential.yaml; the new default applies"}},
		"verified": []map[string]any{{"statement": "upstream changed the rotation default", "citations": f.upstreamCite()}},
		"inferred": []string{"the application reads the key file at start"},
		"unknown":  []string{"whether the application reloads the key"},
		"likelyImpact":     "provisional: billing may keep a stale key until restarted",
		"recommendedTests": []string{"renew the Credential in a test cluster and watch billing's TLS handshakes"},
	}
}
