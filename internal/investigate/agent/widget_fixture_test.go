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

// widgetFixture is a synthetic investigation for the Phase 5 lane verdict
// (dossier, verdicts, mitigations): an invented product "widgets" 1.4.0 →
// 1.5.0 whose controller changes the default of the custom resource field
// Widget.spec.cleanup.policy from Retain to Purge (a controller-behaviour
// item). The environment deploys widgets through a complete render: some set
// the field explicitly, some leave it unset. A consensus-level knowledge
// fact about the change is attached. No evaluation case is modelled.
type widgetFixture struct {
	st       *store.Store
	b        *domain.EvidenceBundle
	g        *domain.RelationshipGraph
	trav     *domain.TraversalResult
	item     string // the behaviour item
	other    string // a rendered-config values item
	notes    domain.ArtifactRef
	values   domain.ArtifactRef
	toRender domain.ArtifactRef
	fact     domain.VerifiedFact
	factArt  domain.ArtifactRef
	rc       string // rendered change correlated to the item
	explicit []string
	unset    []string
}

type widgetOpts struct {
	widgets          int  // extra widgets left unset (beyond the base three)
	renderIncomplete bool // the environment render pair is partial: widgets are seen only in files
	contradictFact   bool // a second active fact with another change for the same subject
	valuesConflict   bool // a second values file sets the values key to another value
	uncorrelated     bool // an environment rendered change nothing explains
}

const widgetNotes = `# widgets 1.5.0

## Breaking changes

- The widget controller now purges a widget's storage when the widget is deleted: the default of
  ` + "`spec.cleanup.policy`" + ` changed from Retain to Purge. Set ` + "`spec.cleanup.policy: Retain`" + ` on a Widget to keep the
  previous behaviour.
- The values key ` + "`controller.legacyMode`" + ` was removed.
`

var widgetCtl = render.ObjectID{Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "widgets", Name: "widget-controller"}

func widgetID(name string) render.ObjectID {
	return render.ObjectID{Group: "example.dev", Version: "v1", Kind: "Widget", Namespace: "shop", Name: name}
}

func widget(name, policy string) render.Object {
	spec := map[string]any{"size": 3}
	if policy != "" {
		spec["cleanup"] = map[string]any{"policy": policy}
	}
	return render.Object{ID: widgetID(name), Body: map[string]any{"apiVersion": "example.dev/v1", "kind": "Widget",
		"metadata": map[string]any{"name": name, "namespace": "shop"}, "spec": spec}}
}

func newWidgetFixture(t *testing.T, o widgetOpts) *widgetFixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.New(t.TempDir())
	must(t, err)
	f := &widgetFixture{st: st}
	put := func(spec investigate.ArtifactSpec, b []byte) domain.ArtifactRef {
		ref, err := st.Put(ctx, spec, b)
		must(t, err)
		return ref
	}
	notesURI := "https://example.dev/widgets/releases/v1.5.0"
	f.notes = put(investigate.ArtifactSpec{Kind: domain.ArtifactUpstreamDocument, Path: "upstream/widgets/v1.5.0/release-notes.md", Origin: domain.OriginUpstream,
		MediaType: "text/markdown", URI: notesURI, Producer: "test.collect@v1", Tags: []string{"release-notes", "widgets"}}, []byte(widgetNotes))
	ev1 := domain.NewEvidence(domain.EvidenceDocument, f.notes.ID, notesURI, "## Breaking changes", "The widget controller now purges a widget's storage when the widget is deleted", f.notes.Digest, time.Time{})
	ev2 := domain.NewEvidence(domain.EvidenceDocument, f.notes.ID, notesURI, "## Breaking changes", "The values key `controller.legacyMode` was removed.", f.notes.Digest, time.Time{})
	edge := domain.UpgradeEdge{Product: domain.ProductRef{ID: "widgets"}, From: domain.Version{Tag: "v1.4.0"}, To: domain.Version{Tag: "v1.5.0"},
		Changes: []domain.Change{
			{ID: "chg-cleanup", Category: domain.Category("behavior"), Breaking: true, Title: "Default cleanup policy is Purge", Release: "v1.5.0",
				Detail: "Set spec.cleanup.policy: Retain on a Widget to keep the previous behaviour.", Subjects: []string{"Widget.spec.cleanup.policy"}, Evidence: []domain.EvidenceID{ev1.ID},
				Provenance: domain.Provenance{Method: domain.MethodHeuristic, Producer: "normalize.releasenotes@v1", Rule: "section:/breaking/i", Confidence: domain.ConfidenceMedium}},
			{ID: "chg-legacy", Category: domain.Category("removal"), Breaking: true, Title: "values key controller.legacyMode removed", Release: "v1.5.0",
				Subjects: []string{"controller.legacyMode"}, Evidence: []domain.EvidenceID{ev2.ID}},
		}, Evidence: []domain.Evidence{ev1, ev2}}
	eb, err := json.MarshalIndent(edge, "", "  ")
	must(t, err)
	edgeRef := put(investigate.ArtifactSpec{Kind: domain.ArtifactUpgradeEdge, Path: "derived/upgrade/edge.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:upgrade@v1", Producer: "upgrade@v1"}, eb)

	behaviour := domain.ChangeItem{ID: domain.ChangeItemID("chg-cleanup", "", ""), ChangeID: "chg-cleanup", Family: domain.FamilyBehavior,
		Title: "Default cleanup policy is Purge", Breaking: true, Subjects: []string{"Widget.spec.cleanup.policy"}, Evidence: []domain.EvidenceID{ev1.ID},
		Manifestation: domain.ManifestControllerBehaviour, ManifestationRule: "manifest:declared-behaviour", InScope: true, ScopeRule: "scope:breaking"}
	legacy := domain.ChangeItem{ID: domain.ChangeItemID("chg-legacy", "", ""), ChangeID: "chg-legacy", Family: domain.FamilyValues,
		Title: "values key controller.legacyMode removed", Breaking: true, Subjects: []string{"controller.legacyMode"}, Evidence: []domain.EvidenceID{ev2.ID},
		Manifestation: domain.ManifestRenderedConfig, ManifestationRule: "manifest:computed-values", InScope: true, ScopeRule: "scope:computed-breaking"}
	f.item, f.other = behaviour.ID, legacy.ID

	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	mkFact := func(after string) domain.VerifiedFact {
		a := domain.SemanticAssertion{
			Subject: &domain.Subject{Family: domain.SubjectCRDField, Product: "widgets", Group: "example.dev", Kind: "Widget", Path: "spec.cleanup.policy"},
			Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: strp(`"Retain"`), After: strp(after)},
			Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpResource, Group: "example.dev", Kind: "Widget",
				Of: []domain.Condition{{Op: domain.OpField, Path: "spec.cleanup.policy", State: domain.StateUnset}}}},
			Consequence: &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired},
			Statement:   "Widgets that leave spec.cleanup.policy unset are purged on deletion after the upgrade.",
		}
		vf := domain.VerifiedFact{Product: "widgets", Release: "v1.5.0", Candidates: []string{"cand-x"}, Assertion: a, Status: domain.FactActive, CreatedAt: at}
		for _, x := range domain.Aspects {
			vf.Verification = append(vf.Verification, domain.AspectVerification{Aspect: x, Level: domain.VerifiedConsensus, Basis: []string{"val-x"}, Consensus: domain.ConsensusCrossModel})
		}
		vf.ID = domain.VerifiedFactID(vf.Product, vf.Release, a)
		return vf
	}
	f.fact = mkFact(`"Purge"`)
	facts := []domain.VerifiedFact{f.fact}
	if o.contradictFact {
		facts = append(facts, mkFact(`"Archive"`))
	}
	var changeArts = []domain.ArtifactRef{edgeRef, f.notes}
	for i, vf := range facts {
		data, _ := json.MarshalIndent(vf, "", "  ")
		ref := put(investigate.ArtifactSpec{Kind: domain.ArtifactKnowledgeFact, Path: "derived/knowledge/widgets/" + vf.ID + ".json", Origin: domain.OriginDerived,
			MediaType: "application/json", URI: "knowledge:" + vf.ID, Producer: "test.collect@v1", ChangeItems: []string{behaviour.ID}}, data)
		behaviour.Artifacts = append(behaviour.Artifacts, ref.ID)
		changeArts = append(changeArts, ref)
		if i == 0 {
			f.factArt = ref
		}
	}
	sort.Strings(behaviour.Artifacts)
	items := []domain.ChangeItem{behaviour, legacy}
	rec := domain.ChangeEvidenceRecord{Product: edge.Product, From: edge.From, To: edge.To, Edge: edgeRef.ID, Items: items, Producer: "test.collect@v1"}
	for _, k := range domain.ChangeSourceKinds {
		s := domain.ChangeSourceStatus{Source: k, Status: domain.CollectionNotApplicable, Reason: "not part of this fixture"}
		if k == domain.ChangeSourceReleaseNotes {
			s = domain.ChangeSourceStatus{Source: k, Status: domain.CollectionOK, Artifacts: []string{f.notes.ID}}
		}
		rec.Sources = append(rec.Sources, s)
	}
	rec.ID = rec.DerivedID()
	change := &investigate.ChangeResult{Record: rec, Artifacts: changeArts, Evidence: edge.Evidence}

	// Environment: the widgets as declared in the repository, a values file.
	explicitW := []render.Object{widget("orders", "Retain"), widget("ledger", "Retain")}
	unsetW := []render.Object{widget("cache", "")}
	for i := 0; i < o.widgets; i++ {
		unsetW = append(unsetW, widget(fmt.Sprintf("scratch-%03d", i), ""))
	}
	for _, w := range explicitW {
		f.explicit = append(f.explicit, w.ID.String())
	}
	for _, w := range unsetW {
		f.unset = append(f.unset, w.ID.String())
	}
	declared, _ := renderdiff.EncodeObjects(append(append([]render.Object(nil), explicitW...), unsetW...))
	widgetsFile := put(investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFile, Path: "env/apps/shop/widgets.yaml", Origin: domain.OriginEnvironment,
		MediaType: "application/yaml", URI: "apps/shop/widgets.yaml", Producer: "test.snapshot@v1", Tags: []string{"manifests"}}, declared)
	f.values = put(investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFile, Path: "env/apps/widgets/values.yaml", Origin: domain.OriginEnvironment,
		MediaType: "application/yaml", URI: "apps/widgets/values.yaml", Producer: "test.snapshot@v1", Tags: []string{"values"}},
		[]byte("controller:\n  legacyMode: true\n  replicas: 2\n"))
	envArts := []domain.ArtifactRef{widgetsFile, f.values}
	if o.valuesConflict {
		envArts = append(envArts, put(investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFile, Path: "env/apps/widgets/values-prod.yaml", Origin: domain.OriginEnvironment,
			MediaType: "application/yaml", URI: "apps/widgets/values-prod.yaml", Producer: "test.snapshot@v1", Tags: []string{"values"}},
			[]byte("controller:\n  legacyMode: false\n")))
	}
	factsArt := put(investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFacts, Path: "derived/env/facts.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:env@v1", Producer: "test.snapshot@v1"}, []byte(`{"name":"shop"}`))
	var envIDs []string
	for _, a := range envArts {
		envIDs = append(envIDs, a.ID)
	}
	sort.Strings(envIDs)
	snap := domain.EnvironmentSnapshot{Environment: "shop", Repo: &domain.RepoRef{Path: "env", TreeDigest: domain.Digest([]byte("widgets-tree"))},
		Inputs: []domain.SnapshotInput{{Kind: domain.InputGitOpsRepo, Path: "env", Status: domain.CollectionOK, Artifacts: envIDs}},
		Facts:  factsArt.ID, Producer: "test.snapshot@v1"}
	snap.ID = snap.DerivedID()
	snapRes := &investigate.SnapshotResult{Snapshot: snap, Artifacts: append(envArts, factsArt)}

	// The environment render: the controller and the widgets, from and to.
	ctl := func(tag string) render.Object {
		return render.Object{ID: widgetCtl, Body: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]any{"name": "widget-controller", "namespace": "widgets"},
			"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
				"containers": []any{map[string]any{"name": "controller", "image": "example.dev/widget-controller:" + tag}}}}}}}
	}
	objs := func(tag string) []render.Object {
		return append(append([]render.Object{ctl(tag)}, explicitW...), unsetW...)
	}
	fromB, _ := renderdiff.EncodeObjects(objs("v1.4.0"))
	toB, _ := renderdiff.EncodeObjects(objs("v1.5.0"))
	fromRef := put(investigate.ArtifactSpec{Kind: domain.ArtifactRenderedManifests, Path: "derived/render/env/argocd-widgets/from.yaml", Origin: domain.OriginDerived,
		MediaType: "application/yaml", URI: "ri:renderdiff@v1", Producer: "test.render@v1"}, fromB)
	f.toRender = put(investigate.ArtifactSpec{Kind: domain.ArtifactRenderedManifests, Path: "derived/render/env/argocd-widgets/to.yaml", Origin: domain.OriginDerived,
		MediaType: "application/yaml", URI: "ri:renderdiff@v1", Producer: "test.render@v1"}, toB)
	diffRef := put(investigate.ArtifactSpec{Kind: domain.ArtifactRenderDiff, Path: "derived/render/env/argocd-widgets/diff.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:renderdiff@v1", Producer: "test.render@v1"}, []byte(`{"changes":1}`))
	target := "argocd:apps/widgets.yaml"
	rc := domain.RenderedChange{Class: "image-changed", Object: widgetCtl.String(), Path: "spec.template.spec.containers[name=controller].image",
		Before: `"example.dev/widget-controller:v1.4.0"`, After: `"example.dev/widget-controller:v1.5.0"`, ChangeItems: []string{behaviour.ID}, Correlation: "subject"}
	changes := []*domain.RenderedChange{&rc}
	var loose domain.RenderedChange
	if o.uncorrelated {
		loose = domain.RenderedChange{Class: "field-changed", Object: widgetCtl.String(), Path: "spec.replicas", Before: "1", After: "2"}
		changes = append(changes, &loose)
	}
	pair := domain.ManifestPair{Scope: domain.RenderEnvironment, Target: target, From: fromRef.ID, To: f.toRender.ID, Diff: diffRef.ID, Status: domain.CollectionOK}
	for _, c := range changes {
		c.ID = domain.RenderedChangeID(domain.RenderEnvironment, target, c.Object, c.Path, c.Class)
		e := domain.NewEvidence(domain.EvidenceRenderedDiff, diffRef.ID, "ri:renderdiff@v1", "changes[id="+c.ID+"]", c.Class+" "+c.Object, diffRef.Digest, time.Time{})
		c.Evidence = []domain.EvidenceID{e.ID}
		snapRes.Evidence = append(snapRes.Evidence, e)
		pair.Changes = append(pair.Changes, *c)
	}
	sort.Slice(pair.Changes, func(i, j int) bool { return pair.Changes[i].ID < pair.Changes[j].ID })
	f.rc = rc.ID
	if o.renderIncomplete {
		pair.Status, pair.Reason, pair.Changes = domain.CollectionPartial, "values incomplete: a values file outside the repository", nil
	}
	renders := &investigate.RenderResult{Pairs: []domain.ManifestPair{pair}, Artifacts: []domain.ArtifactRef{fromRef, f.toRender, diffRef}}
	b, err := bundle.NewAssembler(st).Assemble(ctx, change, snapRes, renders)
	must(t, err)
	must(t, st.PutBundle(ctx, b))
	f.b = b

	// Graph: the controller and each widget; the unset widgets are the item's roots.
	var nodes []domain.GraphNode
	var roots []domain.ExposureRoot
	cn := domain.GraphNode{ID: domain.NodeID(domain.NodeWorkload, widgetCtl.String()), Kind: domain.NodeWorkload, Key: widgetCtl.String(), Artifacts: []string{f.toRender.ID}}
	nodes = append(nodes, cn)
	for _, w := range append(append([]render.Object(nil), explicitW...), unsetW...) {
		n := domain.GraphNode{ID: domain.NodeID(domain.NodeCustomResource, w.ID.String()), Kind: domain.NodeCustomResource, Key: w.ID.String(), Artifacts: []string{f.toRender.ID}}
		nodes = append(nodes, n)
		roots = append(roots, domain.ExposureRoot{ChangeItem: behaviour.ID, Node: n.ID, Rule: "exposure:join-match"})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	g := domain.RelationshipGraph{Bundle: b.ID, Builder: "test.graph@v1", Nodes: nodes}
	g.ID = g.DerivedID()
	must(t, g.Validate())
	f.g = &g
	trav, err := relations.NewTraverser().Traverse(ctx, f.g, roots, domain.DefaultTraversalBound)
	must(t, err)
	f.trav = trav
	return f
}

func strp(s string) *string { return &s }

func (f *widgetFixture) dossierInput() DossierInput {
	return DossierInput{Bundle: f.b, Graph: f.g, Traversal: f.trav, Read: StoreReader(f.st)}
}

func (f *widgetFixture) input(cfg investigate.AgentConfig) investigate.InvestigateInput {
	return investigate.InvestigateInput{Bundle: f.b, Graph: f.g, Traversal: f.trav, Config: cfg}
}
