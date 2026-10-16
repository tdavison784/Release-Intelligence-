package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/render"
)

const (
	notesURL  = "https://github.com/acme/web/blob/main/notes.md"
	notesRaw  = "https://raw.githubusercontent.com/acme/web/main/notes.md"
	crdsURL   = "https://github.com/acme/app/releases/download/v2.0.0/app.crds.yaml"
	valuesURL = "https://github.com/acme/app/blob/v2.0.0/chart/values.yaml"
	valuesRaw = "https://raw.githubusercontent.com/acme/app/v2.0.0/chart/values.yaml"
)

var docs = map[string]string{
	notesRaw:  "# Notes\n## Breaking\n- --enable-thing is on by default\n",
	crdsURL:   "kind: CustomResourceDefinition\n",
	valuesRaw: "replicaCount: 1\n",
}

type fakeUpstream struct {
	edge      *domain.UpgradeEdge
	pair      *render.Pair
	renderErr error
	fetchFail map[string]bool
	fetched   []string
}

func (f *fakeUpstream) Upgrade(context.Context, string, string, string) (*domain.UpgradeEdge, error) {
	cp := *f.edge
	cp.GeneratedAt = time.Now() // wall clock must not leak into the bundle
	return &cp, nil
}

func (f *fakeUpstream) RenderRelease(context.Context, *domain.UpgradeEdge, string) (*render.Pair, error) {
	return f.pair, f.renderErr
}

func (f *fakeUpstream) Fetch(_ context.Context, u string) ([]byte, string, error) {
	f.fetched = append(f.fetched, u)
	if f.fetchFail[u] {
		return nil, "", fmt.Errorf("fetch %s: not in cache", u)
	}
	d, ok := docs[u]
	if !ok {
		return nil, "", fmt.Errorf("fetch %s: 404", u)
	}
	return []byte(d), "text/plain", nil
}

func ev(kind domain.EvidenceKind, source, uri, locator, excerpt, body string) domain.Evidence {
	return domain.NewEvidence(kind, source, uri, locator, excerpt, domain.Digest([]byte(body)), time.Now())
}

func computed(rule string) domain.Provenance {
	return domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: rule, Confidence: domain.ConfidenceHigh}
}

func declared(rule string) domain.Provenance {
	return domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.notes@v1", Rule: rule, Confidence: domain.ConfidenceHigh}
}

func constraint(platform, c, raw, kind string, e domain.EvidenceID) *domain.CompatibilityConstraint {
	return &domain.CompatibilityConstraint{Platform: platform, Constraint: c, Raw: raw, Kind: kind, SourceID: "matrix",
		Provenance: declared("table"), Evidence: []domain.EvidenceID{e}}
}

// fixtureEdge is a small upgrade edge with one of everything the collector
// distinguishes.
func fixtureEdge() *domain.UpgradeEdge {
	evNotes := ev(domain.EvidenceDocument, "notes", notesURL+"#L3", "L3", "--enable-thing is on by default", docs[notesRaw])
	evCRD := ev(domain.EvidenceStructured, "crds", crdsURL, "", "crd bundle", docs[crdsURL])
	evVals := ev(domain.EvidenceStructured, "chart", valuesURL, "chart/values.yaml", "values", docs[valuesRaw])
	evOther := ev(domain.EvidenceDocument, "notes", notesURL+"#L99", "L99", "something unrelated", docs[notesRaw])
	evFrom := ev(domain.EvidenceDocument, "matrix", notesURL+"#L10", "L10", "k8s 1.26-1.30", docs[notesRaw])
	evTo := ev(domain.EvidenceDocument, "matrix", notesURL+"#L11", "L11", "k8s 1.28-1.32", docs[notesRaw])
	chg := func(id string, cat domain.Category, p domain.Provenance, mut func(*domain.Change)) domain.Change {
		c := domain.Change{ID: id, Category: cat, Title: id + " title", Provenance: p, Evidence: []domain.EvidenceID{evOther.ID}}
		if mut != nil {
			mut(&c)
		}
		return c
	}
	return &domain.UpgradeEdge{
		SchemaVersion: domain.UpgradeEdgeSchemaVersion,
		Product:       domain.ProductRef{ID: "acme", Name: "Acme"},
		From:          domain.Version{Tag: "v1.0.0", Semver: "1.0.0"}, To: domain.Version{Tag: "v2.0.0", Semver: "2.0.0"},
		Sources: []domain.SourceStatus{
			{SourceID: "notes", Roles: []domain.SourceRole{domain.RoleReleaseNotes}, State: domain.SourceOK},
			{SourceID: "matrix", Roles: []domain.SourceRole{domain.RoleCompatibility}, State: domain.SourceOK},
		},
		Artifacts: []domain.ArtifactChange{
			{ArtifactID: "crds", Type: domain.ArtifactCRD, Name: "crds", Change: domain.ChangeUpdated},
			{ArtifactID: "chart", Type: domain.ArtifactHelmChart, Name: "chart", Change: domain.ChangeUpdated},
			{ArtifactID: "ctl", Type: domain.ArtifactContainerImage, Name: "ctl", Change: domain.ChangeUpdated,
				From: &domain.ArtifactInstance{Type: domain.ArtifactContainerImage, Coordinate: "example.io/acme/ctl:v1.0.0"},
				To:   &domain.ArtifactInstance{Type: domain.ArtifactContainerImage, Coordinate: "example.io/acme/ctl:v2.0.0"}},
		},
		Changes: []domain.Change{
			chg("c-breaking", domain.CategoryConfiguration, declared("marker:breaking"), func(c *domain.Change) {
				c.Breaking, c.Subjects = true, []string{"--enable-thing"}
				c.Evidence = []domain.EvidenceID{evNotes.ID}
			}),
			chg("c-routine", domain.CategoryOther, declared("section:other"), func(c *domain.Change) { c.Routine, c.RoutineKind = true, "dependency" }),
			chg("c-feature", domain.CategoryFeature, declared("section:feature"), nil),
			chg("c-removal", domain.CategoryRemoval, declared("kw:remove"), nil),
			chg("c-valrm", domain.CategoryHelmValues, computed("values:removed"), func(c *domain.Change) {
				c.Evidence = []domain.EvidenceID{evVals.ID}
				c.Subjects = []string{"webhook.legacy"}
			}),
			chg("c-valadd", domain.CategoryHelmValues, computed("values:added"), nil),
			chg("c-crddef", domain.CategoryCRDSchema, computed("crd:default-changed"), func(c *domain.Change) { c.Evidence = []domain.EvidenceID{evCRD.ID} }),
			chg("c-compat", domain.CategoryCompatibility, computed("compat:narrowed"), func(c *domain.Change) { c.Subjects = []string{"kubernetes:supported"} }),
		},
		Compatibility: []domain.CompatibilityChange{
			{Platform: "kubernetes", From: constraint("kubernetes", ">=1.26 <1.31", "1.26-1.30", "supported", evFrom.ID),
				To: constraint("kubernetes", ">=1.28 <1.33", "1.28-1.32", "supported", evTo.ID), Summary: "Kubernetes 1.26–1.30 → 1.28–1.32", Narrowed: true},
			{Platform: "openshift", From: constraint("openshift", ">=4.16", "4.16+", "supported", evFrom.ID),
				To: constraint("openshift", ">=4.16", "4.16+", "supported", evTo.ID), Summary: "OpenShift unchanged"},
		},
		Evidence: []domain.Evidence{evNotes, evOther, evCRD, evVals, evFrom, evTo},
	}
}

func objID(kind, name string) render.ObjectID {
	return render.ObjectID{Group: "apps", Version: "v1", Kind: kind, Namespace: "acme", Name: name}
}

func s(v string) *string { return &v }

// fixturePair is an OK chart-default pair with: a container arg the breaking
// change names (correlated), a removed ClusterRole nothing mentions (an
// unexplained structural change), unexplained non-structural changes, and
// the version bump of the product's own image.
func fixturePair() *render.Pair {
	diff := &render.DiffResult{Changes: []render.Change{
		{Class: render.ContainerArgAdded, Object: objID("Deployment", "ctl"), Path: "spec.template.spec.containers[name=ctl].args[--enable-thing]", Pattern: "spec.template.spec.containers[].args", Name: "--enable-thing", Container: "ctl", After: s(`"--enable-thing=true"`)},
		{Class: render.ResourceRemoved, Object: render.ObjectID{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole", Name: "acme-old"}},
		{Class: render.FieldChanged, Object: objID("Deployment", "ctl"), Path: "spec.minReadySeconds", Before: s("1"), After: s("5")},
		{Class: render.ImageChanged, Object: objID("Deployment", "ctl"), Path: "spec.template.spec.containers[name=ctl].image", Name: "example.io/acme/ctl",
			Before: s(`"example.io/acme/ctl:v1.0.0"`), After: s(`"example.io/acme/ctl:v2.0.0"`)},
		{Class: render.FieldChanged, Object: render.ObjectID{Version: "v1", Kind: "Secret", Namespace: "acme", Name: "s"}, Path: "data.token", Sensitive: true, Before: s("sha256:aa"), After: s("sha256:bb")},
	}}
	return &render.Pair{Product: "acme", From: "v1.0.0", To: "v2.0.0", Scope: domain.RenderRelease, Target: render.DefaultsTarget("acme"), Status: render.PairOK,
		FromResult: &render.Result{Status: render.StatusSucceeded, Output: []byte("kind: ConfigMap\n")},
		ToResult:   &render.Result{Status: render.StatusSucceeded, Output: []byte("apiVersion: v1\nkind: Secret\ndata:\n  token: c2VjcmV0\n")},
		Diff:       diff}
}

func collect(t *testing.T, up *fakeUpstream, renderOn bool) (*investigate.ChangeResult, *store.Store) {
	t.Helper()
	fs := newSt(t)
	res, err := NewChangeCollectorOver(up, fs).CollectChange(context.Background(), investigate.ChangeRequest{Product: "acme", From: "v1.0.0", To: "v2.0.0", Render: renderOn})
	if err != nil {
		t.Fatal(err)
	}
	return res, fs
}

func itemByChange(r domain.ChangeEvidenceRecord, changeID string) (domain.ChangeItem, bool) {
	return r.Item(domain.ChangeItemID(changeID, "", ""))
}

func TestChangeItemsScopeAndManifestation(t *testing.T) {
	res, _ := collect(t, &fakeUpstream{edge: fixtureEdge(), pair: fixturePair()}, true)
	rec := res.Record
	if err := rec.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, ok := itemByChange(rec, "c-routine"); ok {
		t.Error("a routine change has no item")
	}
	if _, ok := itemByChange(rec, "c-compat"); ok {
		t.Error("a computed compat change is represented by its platform item")
	}
	want := map[string]struct {
		scope         bool
		scopeRule     string
		manifestation domain.Manifestation
		mRule         string
		family        domain.ChangeItemFamily
	}{
		"c-breaking": {true, ScopeBreaking, domain.ManifestRenderedConfig, ManifestRuleRenderedCorrelated, domain.FamilyBehavior},
		"c-feature":  {false, ScopeNotBreaking, domain.ManifestControllerBehaviour, ManifestRuleDeclaredBehaviour, domain.FamilyBehavior},
		"c-removal":  {true, ScopeCategory, domain.ManifestControllerBehaviour, ManifestRuleDeclaredBehaviour, domain.FamilyBehavior},
		"c-valrm":    {true, ScopeComputedBreaking, domain.ManifestRenderedConfig, ManifestRuleComputedValues, domain.FamilyValues},
		"c-valadd":   {false, ScopeNotBreaking, domain.ManifestRenderedConfig, ManifestRuleComputedValues, domain.FamilyValues},
		"c-crddef":   {true, ScopeComputedBreaking, domain.ManifestControllerBehaviour, ManifestRuleCRDDefault, domain.FamilyAPISchema},
	}
	for id, w := range want {
		it, ok := itemByChange(rec, id)
		if !ok {
			t.Errorf("%s: no item", id)
			continue
		}
		if it.InScope != w.scope || it.ScopeRule != w.scopeRule || it.Manifestation != w.manifestation || it.ManifestationRule != w.mRule || it.Family != w.family {
			t.Errorf("%s: inScope=%v %s, %s/%s, family %s; want %+v", id, it.InScope, it.ScopeRule, it.Manifestation, it.ManifestationRule, it.Family, w)
		}
	}
	// Platform items: only the moved constraint.
	var platforms []domain.ChangeItem
	for _, it := range rec.Items {
		if it.Platform != "" {
			platforms = append(platforms, it)
		}
	}
	if len(platforms) != 1 || platforms[0].Platform != "kubernetes" {
		t.Fatalf("platform items = %+v", platforms)
	}
	if p := platforms[0]; !p.InScope || p.ScopeRule != ScopeComputedBreaking || p.Manifestation != domain.ManifestRenderedConfig || p.Family != domain.FamilyCompatibility {
		t.Errorf("platform item = %+v", p)
	}
	// Every item cites upstream evidence that resolves in the pool.
	pool := map[domain.EvidenceID]bool{}
	for _, e := range res.Evidence {
		pool[e.ID] = true
	}
	for _, it := range rec.Items {
		if len(it.Evidence) == 0 {
			t.Errorf("%s: no evidence", it.ID)
		}
		for _, id := range it.Evidence {
			if !pool[id] {
				t.Errorf("%s: evidence %s not in the pool", it.ID, id)
			}
		}
	}
}

func TestChangeReleaseRenderCorrelationAndAnchors(t *testing.T) {
	res, st := collect(t, &fakeUpstream{edge: fixtureEdge(), pair: fixturePair()}, true)
	rec := res.Record
	if len(rec.Renders) != 1 {
		t.Fatalf("renders = %d", len(rec.Renders))
	}
	p := rec.Renders[0]
	if p.Scope != domain.RenderRelease || p.Target != "chart-defaults" || p.Status != domain.CollectionOK || len(p.Changes) != 5 {
		t.Fatalf("pair = %+v", p)
	}
	breaking, _ := itemByChange(rec, "c-breaking")
	var arg, role, field, secret, img *domain.RenderedChange
	for i := range p.Changes {
		c := &p.Changes[i]
		switch {
		case c.Class == "container-arg-added":
			arg = c
		case c.Class == "resource-removed":
			role = c
		case c.Object == "core/v1/Secret/acme/s":
			secret = c
		case c.Class == "image-changed":
			img = c
		default:
			field = c
		}
	}
	if arg == nil || len(arg.ChangeItems) != 1 || arg.ChangeItems[0] != breaking.ID || arg.Correlation == "" {
		t.Fatalf("the arg change must be attributed to the breaking item: %+v", arg)
	}
	// DESIGN §1.3 amendment: every other unexplained change anchors its own
	// in-scope item; the product's own image bump an out-of-scope one
	for _, c := range []struct {
		rc      *domain.RenderedChange
		rule    string
		inScope bool
	}{{field, ScopeRenderedUnexplained, true}, {secret, ScopeRenderedUnexplained, true}, {img, ScopeRoutineSelfArtifact, false}} {
		if c.rc == nil || len(c.rc.ChangeItems) != 1 || c.rc.Correlation != "rendered-anchor" {
			t.Fatalf("an unexplained change is anchored on its own item: %+v", c.rc)
		}
		it, ok := rec.Item(c.rc.ChangeItems[0])
		if !ok || it.RenderedChange != c.rc.ID || it.ScopeRule != c.rule || it.InScope != c.inScope || it.Family != domain.FamilyRendered ||
			it.Manifestation != domain.ManifestRenderedConfig {
			t.Errorf("%s: item = %+v", c.rc.ID, it)
		}
	}
	if role == nil || len(role.ChangeItems) != 1 || role.Correlation != "rendered-anchor" {
		t.Fatalf("an unexplained structural change is anchored on its own item: %+v", role)
	}
	anchor, ok := rec.Item(role.ChangeItems[0])
	if !ok || anchor.RenderedChange != role.ID || anchor.Family != domain.FamilyRendered || !anchor.InScope || anchor.ScopeRule != ScopeRenderedStructural ||
		anchor.Manifestation != domain.ManifestRenderedConfig || anchor.ManifestationRule != ManifestRuleRenderedAnchor {
		t.Errorf("anchor item = %+v", anchor)
	}
	if secret == nil || !secret.Sensitive || secret.Before != "" || secret.After != "" {
		t.Errorf("a sensitive change records no values: %+v", secret)
	}
	// The renders are artifacts; Secret values never reach them.
	for _, id := range []string{p.From, p.To, p.Diff} {
		a, ok := findArtifact(res.Artifacts, id)
		if !ok {
			t.Fatalf("artifact %s missing", id)
		}
		if body := readFrom(t, st, a.Digest); strings.Contains(body, "c2VjcmV0") {
			t.Errorf("secret value in %s", a.Path)
		}
	}
	if a, _ := findArtifact(res.Artifacts, p.To); !a.Redacted || a.Kind != domain.ArtifactRenderedManifests {
		t.Errorf("to render = %+v", a)
	}
	if s, _ := rec.SourceStatus(domain.ChangeSourceRenderedManifests); s.Status != domain.CollectionOK || len(s.Artifacts) != 3 {
		t.Errorf("rendered-manifests source = %+v", s)
	}
}

func TestChangeIncompleteReleaseRenderIsAGap(t *testing.T) {
	failed := fixturePair()
	failed.Status, failed.Diff, failed.ToResult = render.PairFailed, nil, &render.Result{Status: render.StatusFailed}
	failed.Failure = &render.Failure{Reason: render.FailChartUnavailable, Detail: "no chart"}
	res, _ := collect(t, &fakeUpstream{edge: fixtureEdge(), pair: failed}, true)
	p := res.Record.Renders[0]
	if p.Status != domain.CollectionUnavailable || len(p.Changes) != 0 || !strings.Contains(p.Reason, "chart-unavailable") {
		t.Fatalf("a failed render is an evidence gap with its reason, never a diff: %+v", p)
	}
	if s, _ := res.Record.SourceStatus(domain.ChangeSourceRenderedManifests); s.Status != domain.CollectionUnavailable || s.Reason == "" {
		t.Errorf("source = %+v", s)
	}
	// No release render ⇒ no anchored items.
	for _, it := range res.Record.Items {
		if it.RenderedChange != "" {
			t.Errorf("anchored item without a render: %+v", it)
		}
	}
	// A render that could not even be attempted.
	res, _ = collect(t, &fakeUpstream{edge: fixtureEdge(), renderErr: fmt.Errorf("helm not found")}, true)
	if p := res.Record.Renders[0]; p.Status != domain.CollectionUnavailable || !strings.Contains(p.Reason, "helm not found") {
		t.Errorf("pair = %+v", p)
	}
}

func TestChangeRenderNotRequested(t *testing.T) {
	up := &fakeUpstream{edge: fixtureEdge(), pair: fixturePair()}
	res, _ := collect(t, up, false)
	if len(res.Record.Renders) != 0 {
		t.Error("no renders were requested")
	}
	if s, _ := res.Record.SourceStatus(domain.ChangeSourceRenderedManifests); s.Status != domain.CollectionUnavailable {
		t.Errorf("source = %+v", s)
	}
}

func TestChangeEverySourceKindIsStated(t *testing.T) {
	res, _ := collect(t, &fakeUpstream{edge: fixtureEdge(), pair: fixturePair()}, true)
	got := map[domain.ChangeSourceKind]domain.ChangeSourceStatus{}
	for _, s := range res.Record.Sources {
		got[s.Source] = s
	}
	want := map[domain.ChangeSourceKind]domain.CollectionStatus{
		domain.ChangeSourceReleaseNotes:      domain.CollectionOK,
		domain.ChangeSourceMigrationGuide:    domain.CollectionUnavailable, // the definition has no upgrade guide
		domain.ChangeSourceSourceDiff:        domain.CollectionUnavailable,
		domain.ChangeSourceAPISchema:         domain.CollectionOK,
		domain.ChangeSourceChart:             domain.CollectionOK,
		domain.ChangeSourceKustomize:         domain.CollectionNotApplicable,
		domain.ChangeSourceRenderedManifests: domain.CollectionOK,
		domain.ChangeSourceCompatibility:     domain.CollectionOK,
		domain.ChangeSourceAdvisories:        domain.CollectionUnavailable,
	}
	if len(res.Record.Sources) != len(domain.ChangeSourceKinds) {
		t.Fatalf("sources = %d, want %d", len(res.Record.Sources), len(domain.ChangeSourceKinds))
	}
	for k, w := range want {
		s := got[k]
		if s.Status != w {
			t.Errorf("%s: %s (%s), want %s", k, s.Status, s.Reason, w)
		}
		if s.Status != domain.CollectionOK && s.Reason == "" {
			t.Errorf("%s: a non-ok status states its reason", k)
		}
	}
	if !strings.Contains(got[domain.ChangeSourceMigrationGuide].Reason, "upgrade guide") {
		t.Errorf("reason = %q", got[domain.ChangeSourceMigrationGuide].Reason)
	}
}

func TestChangeFetchFailureIsPartialNotFatal(t *testing.T) {
	up := &fakeUpstream{edge: fixtureEdge(), pair: fixturePair(), fetchFail: map[string]bool{crdsURL: true}}
	res, _ := collect(t, up, true)
	s, _ := res.Record.SourceStatus(domain.ChangeSourceAPISchema)
	if s.Status != domain.CollectionUnavailable || !strings.Contains(s.Reason, "not in cache") {
		t.Fatalf("api-schema = %+v", s)
	}
	if err := res.Record.Validate(); err != nil {
		t.Fatal(err)
	}
	// One of two documents failing leaves the source partial.
	up = &fakeUpstream{edge: fixtureEdge(), pair: fixturePair(), fetchFail: map[string]bool{notesRaw: true}}
	e := up.edge
	e.Evidence = append(e.Evidence, ev(domain.EvidenceDocument, "notes", "https://github.com/acme/web/blob/main/other.md", "", "x", "x"))
	res, _ = collect(t, up, true)
	if s, _ := res.Record.SourceStatus(domain.ChangeSourceReleaseNotes); s.Status != domain.CollectionUnavailable && s.Status != domain.CollectionPartial {
		t.Errorf("release-notes = %+v", s)
	}
}

func TestChangeDocumentDriftIsFlagged(t *testing.T) {
	e := fixtureEdge()
	for i := range e.Evidence { // every ingested citation of the notes carries another digest
		if e.Evidence[i].SourceID == "notes" {
			e.Evidence[i].ContentDigest = "sha256:" + strings.Repeat("0", 64)
		}
	}
	res, _ := collect(t, &fakeUpstream{edge: e, pair: fixturePair()}, true)
	s, _ := res.Record.SourceStatus(domain.ChangeSourceReleaseNotes)
	if s.Status != domain.CollectionPartial || !strings.Contains(s.Reason, "changed since it was ingested") {
		t.Fatalf("a document whose digest differs from the ingested evidence is partial: %+v", s)
	}
}

func TestChangeNoArtifactsIsNotApplicable(t *testing.T) {
	e := fixtureEdge()
	e.Artifacts = nil
	res, _ := collect(t, &fakeUpstream{edge: e, pair: fixturePair()}, false)
	for _, k := range []domain.ChangeSourceKind{domain.ChangeSourceAPISchema, domain.ChangeSourceChart} {
		if s, _ := res.Record.SourceStatus(k); s.Status != domain.CollectionNotApplicable {
			t.Errorf("%s = %+v", k, s)
		}
	}
}

func TestChangeEdgeArtifactAndDeterminism(t *testing.T) {
	a, st := collect(t, &fakeUpstream{edge: fixtureEdge(), pair: fixturePair()}, true)
	b, _ := collect(t, &fakeUpstream{edge: fixtureEdge(), pair: fixturePair()}, true)
	if a.Record.ID != b.Record.ID {
		t.Fatalf("record id depends on wall-clock time: %s vs %s", a.Record.ID, b.Record.ID)
	}
	edge, ok := findArtifact(a.Artifacts, a.Record.Edge)
	if !ok || edge.Kind != domain.ArtifactUpgradeEdge {
		t.Fatalf("edge artifact = %+v", edge)
	}
	var back domain.UpgradeEdge
	if err := json.Unmarshal([]byte(readFrom(t, st, edge.Digest)), &back); err != nil {
		t.Fatal(err)
	}
	if !back.GeneratedAt.IsZero() || back.Product.ID != "acme" || len(back.Changes) != len(fixtureEdge().Changes) {
		t.Errorf("stored edge = %+v", back)
	}
	// Items never copy the edge's change bodies, only reference them.
	for _, it := range a.Record.Items {
		if it.ChangeID != "" {
			if _, ok := edgeChange(&back, it.ChangeID); !ok {
				t.Errorf("item %s references unknown change %s", it.ID, it.ChangeID)
			}
		}
	}
}

func edgeChange(e *domain.UpgradeEdge, id string) (domain.Change, bool) {
	for _, c := range e.Changes {
		if c.ID == id {
			return c, true
		}
	}
	return domain.Change{}, false
}

func TestChangeDocumentArtifactsRelateToItems(t *testing.T) {
	res, _ := collect(t, &fakeUpstream{edge: fixtureEdge(), pair: fixturePair()}, false)
	var notes *domain.ArtifactRef
	for i, a := range res.Artifacts {
		if a.URI == notesRaw {
			notes = &res.Artifacts[i]
		}
	}
	if notes == nil || notes.Origin != domain.OriginUpstream || !strings.HasPrefix(notes.Path, "upstream/acme/") {
		t.Fatalf("notes artifact = %+v", notes)
	}
	breaking, _ := itemByChange(res.Record, "c-breaking")
	found := false
	for _, id := range notes.ChangeItems {
		found = found || id == breaking.ID
	}
	if !found {
		t.Errorf("the notes artifact relates to the items citing it: %v", notes.ChangeItems)
	}
}

func TestChangeUpgradeErrorIsAnError(t *testing.T) {
	_, err := NewChangeCollectorOver(&errUpstream{}, newSt(t)).CollectChange(context.Background(), investigate.ChangeRequest{Product: "x", From: "1", To: "2"})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}

type errUpstream struct{ fakeUpstream }

func (*errUpstream) Upgrade(context.Context, string, string, string) (*domain.UpgradeEdge, error) {
	return nil, fmt.Errorf("boom")
}

func TestDocURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/o/r/blob/v1/dir/f.md#L3-L9":      "https://raw.githubusercontent.com/o/r/v1/dir/f.md",
		"https://github.com/o/r/releases/download/v1/x.yaml": "https://github.com/o/r/releases/download/v1/x.yaml",
		"https://example.com/a/b.md#frag":                    "https://example.com/a/b.md",
	} {
		if got, ok := docURL(in); !ok || got != want {
			t.Errorf("docURL(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "ri:render@release", "file:///etc/passwd", "refs/tags/v1"} {
		if _, ok := docURL(bad); ok {
			t.Errorf("docURL(%q) must be refused", bad)
		}
	}
}

var _ = io.EOF

// TestChangeStoresAttachedKnowledgeFacts: facts the wiring supplies are stored
// as knowledge-fact artifacts when they attach to a change that has an item
// (Principle 6: validators read them from the bundle), and only then.
func TestChangeStoresAttachedKnowledgeFacts(t *testing.T) {
	edge := fixtureEdge()
	lookup := func(id domain.EvidenceID) (domain.Evidence, bool) {
		for _, e := range edge.Evidence {
			if e.ID == id {
				return e, true
			}
		}
		return domain.Evidence{}, false
	}
	var breaking domain.Change
	for _, c := range edge.Changes {
		if c.ID == "c-breaking" {
			breaking = c
		}
	}
	attached := domain.VerifiedFact{ID: "fact-attached", Product: "acme", Status: domain.FactActive,
		Anchors: []domain.ChangeAnchor{domain.NewChangeAnchor(breaking, lookup)}}
	otherProduct := attached
	otherProduct.ID, otherProduct.Product = "fact-other", "other"
	retracted := attached
	retracted.ID, retracted.Status = "fact-retracted", domain.FactRetracted
	fs := newSt(t)
	res, err := NewChangeCollectorOver(&fakeUpstream{edge: edge, pair: fixturePair()}, fs, WithKnowledge(retracted, otherProduct, attached, attached)).
		CollectChange(context.Background(), investigate.ChangeRequest{Product: "acme", From: "v1.0.0", To: "v2.0.0", Render: true})
	if err != nil {
		t.Fatal(err)
	}
	var facts []domain.ArtifactRef
	for _, a := range res.Artifacts {
		if a.Kind == domain.ArtifactKnowledgeFact {
			facts = append(facts, a)
		}
	}
	item, _ := itemByChange(res.Record, "c-breaking")
	if len(facts) != 1 || facts[0].Path != "derived/knowledge/acme/fact-attached.json" || len(facts[0].ChangeItems) != 1 || facts[0].ChangeItems[0] != item.ID {
		t.Fatalf("knowledge artifacts = %+v", facts)
	}
	if len(item.Artifacts) != 1 || item.Artifacts[0] != facts[0].ID {
		t.Errorf("item artifacts = %v", item.Artifacts)
	}
	var back domain.VerifiedFact
	if err := json.Unmarshal([]byte(readFrom(t, fs, facts[0].Digest)), &back); err != nil || back.ID != "fact-attached" {
		t.Errorf("stored fact = %+v (%v)", back, err)
	}
	if err := res.Record.Validate(); err != nil {
		t.Fatal(err)
	}
}

// A CRD description-text change documents the API; it configures nothing: it
// stays a visible rendered change of the pair but correlates to no item and
// anchors none — reworded schema docs must not flood the investigation with
// scope:rendered-unexplained items.
func TestChangeCRDDescriptionAnchorsNoItem(t *testing.T) {
	pair := fixturePair()
	pair.Diff.Changes = append(pair.Diff.Changes, render.Change{
		Class:  render.CRDDescriptionChanged,
		Object: render.ObjectID{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition", Name: "widgets.acme.io"},
		Path:   "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.description", Name: "description",
		Before: s(`"defaults to safe"`), After: s(`"defaults to fast"`)})
	res, _ := collect(t, &fakeUpstream{edge: fixtureEdge(), pair: pair}, true)
	rec := res.Record
	var found *domain.RenderedChange
	for i := range rec.Renders[0].Changes {
		c := &rec.Renders[0].Changes[i]
		if c.Class == string(render.CRDDescriptionChanged) {
			found = c
		}
	}
	if found == nil {
		t.Fatal("the description change must stay visible in the pair")
	}
	if len(found.ChangeItems) != 0 || found.Correlation != "" {
		t.Fatalf("description text must correlate to no item: %+v", found)
	}
	for _, it := range rec.Items {
		if it.RenderedChange == found.ID {
			t.Fatalf("description text anchored an item: %+v", it)
		}
	}
	if err := rec.Validate(); err != nil {
		t.Fatal(err)
	}
}
