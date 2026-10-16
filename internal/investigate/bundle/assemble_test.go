package bundle

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
)

// envRender stores an environment-scope render pair the way the renderdiff
// lane does (a fake of that port), with one correlated and, optionally, one
// uncorrelated change.
func envRender(t *testing.T, st *store.Store, item string, uncorrelated bool) *investigate.RenderResult {
	t.Helper()
	ctx := context.Background()
	put := func(name string, kind domain.ArtifactKind, body string) domain.ArtifactRef {
		ref, err := st.Put(ctx, investigate.ArtifactSpec{Kind: kind, Path: "derived/render/env/argocd_app/" + name, Origin: domain.OriginDerived,
			MediaType: "application/yaml", URI: "ri:renderdiff@v1", Producer: "renderdiff@v1"}, []byte(body))
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	from, to := put("from.yaml", domain.ArtifactRenderedManifests, "a: 1\n"), put("to.yaml", domain.ArtifactRenderedManifests, "a: 2\n")
	diff := put("diff.json", domain.ArtifactRenderDiff, "{}\n")
	evd := domain.NewEvidence(domain.EvidenceRenderedDiff, "", "ri:renderdiff@v1/argocd:app", "", "env diff", diff.Digest, time.Time{})
	mk := func(obj, path string, items ...string) domain.RenderedChange {
		rc := domain.RenderedChange{ID: domain.RenderedChangeID(domain.RenderEnvironment, "argocd:app", obj, path, "field-changed"), Class: "field-changed", Object: obj, Path: path,
			Evidence: []domain.EvidenceID{evd.ID}, ChangeItems: items}
		if len(items) > 0 {
			rc.Correlation = "subject"
		}
		return rc
	}
	changes := []domain.RenderedChange{mk("apps/v1/Deployment/ns/a", "spec.x", item)}
	if uncorrelated {
		changes = append(changes, mk("apps/v1/Deployment/ns/b", "spec.y"))
	}
	if changes[0].ID > changes[len(changes)-1].ID {
		changes[0], changes[len(changes)-1] = changes[len(changes)-1], changes[0]
	}
	return &investigate.RenderResult{
		Pairs:     []domain.ManifestPair{{Scope: domain.RenderEnvironment, Target: "argocd:app", From: from.ID, To: to.ID, Diff: diff.ID, Status: domain.CollectionOK, Changes: changes}},
		Artifacts: []domain.ArtifactRef{from, to, diff}, Evidence: []domain.Evidence{evd},
	}
}

type parts struct {
	st     *store.Store
	change *investigate.ChangeResult
	snap   *investigate.SnapshotResult
	repo   string
}

func newParts(t *testing.T) parts {
	t.Helper()
	st := newSt(t)
	change, err := NewChangeCollectorOver(&fakeUpstream{edge: fixtureEdge(), pair: fixturePair()}, st).CollectChange(context.Background(),
		investigate.ChangeRequest{Product: "acme", From: "v1.0.0", To: "v2.0.0", Render: true})
	if err != nil {
		t.Fatal(err)
	}
	repo := testRepo(t)
	snap, err := NewSnapshotBuilder(st).BuildSnapshot(context.Background(), investigate.EnvironmentInputs{Name: "shop-prod", Repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	return parts{st, change, snap, repo}
}

func TestAssembleBundleValidatesAndStores(t *testing.T) {
	p := newParts(t)
	item, _ := itemByChange(p.change.Record, "c-breaking")
	b, err := NewAssembler(p.st).Assemble(context.Background(), p.change, p.snap, envRender(t, p.st, item.ID, true))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	if b.SchemaVersion != domain.EvidenceBundleSchemaVersion || len(b.Renders) != 1 {
		t.Fatalf("bundle = %+v", b)
	}
	// Manifest: sorted by path, unique, covering edge + snapshot facts + renders.
	paths := map[string]bool{}
	for i, a := range b.Manifest.Artifacts {
		paths[a.Path] = true
		if i > 0 && b.Manifest.Artifacts[i-1].Path >= a.Path {
			t.Fatalf("manifest not sorted at %s", a.Path)
		}
	}
	for _, want := range []string{"derived/change/upgrade-edge.json", "derived/snapshot/environment-facts.json", "derived/render/env/argocd_app/diff.json", "derived/render/release/chart-defaults/diff.json", "env/apps/shop/deploy.yaml"} {
		if !paths[want] {
			t.Errorf("manifest lacks %s", want)
		}
	}
	// Evidence pool resolves upstream, environment and render records.
	pool := b.EvidenceByID()
	if len(pool) == 0 || len(b.Uncorrelated()) != 1 {
		t.Fatalf("pool=%d uncorrelated=%v", len(pool), b.Uncorrelated())
	}
	if err := p.st.PutBundle(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	got, err := p.st.GetBundle(context.Background(), b.ID)
	if err != nil || got.ID != b.ID {
		t.Fatalf("round trip: %v", err)
	}
}

func TestAssembleWithoutEnvironmentRenders(t *testing.T) {
	p := newParts(t)
	b, err := NewAssembler(p.st).Assemble(context.Background(), p.change, p.snap, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Renders) != 0 {
		t.Fatal("no environment render was given")
	}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAssembleDeterministic(t *testing.T) {
	build := func(repo string) string {
		st := newSt(t)
		change, err := NewChangeCollectorOver(&fakeUpstream{edge: fixtureEdge(), pair: fixturePair()}, st).CollectChange(context.Background(),
			investigate.ChangeRequest{Product: "acme", From: "v1.0.0", To: "v2.0.0", Render: true})
		if err != nil {
			t.Fatal(err)
		}
		snap, err := NewSnapshotBuilder(st).BuildSnapshot(context.Background(), investigate.EnvironmentInputs{Name: "shop-prod", Repo: repo})
		if err != nil {
			t.Fatal(err)
		}
		a := NewAssembler(st).(*Assembler)
		a.now = func() time.Time { return time.Now().Add(time.Duration(len(repo)) * time.Hour) } // creation time is not identity
		b, err := a.Assemble(context.Background(), change, snap, nil)
		if err != nil {
			t.Fatal(err)
		}
		return b.ID
	}
	repo := testRepo(t)
	first, second := build(repo), build(repo)
	if first != second {
		t.Fatalf("two runs, two bundle ids: %s vs %s", first, second)
	}
	write(t, repo, "apps/shop/deploy.yaml", "kind: Deployment\nspec:\n  replicas: 9\n")
	if third := build(repo); third == first {
		t.Fatal("a changed environment must change the bundle id")
	}
}

func TestAssembleRefusals(t *testing.T) {
	ctx := context.Background()
	p := newParts(t)
	a := NewAssembler(p.st)
	if _, err := a.Assemble(ctx, nil, p.snap, nil); err == nil {
		t.Error("missing change evidence must be refused")
	}
	if _, err := a.Assemble(ctx, p.change, nil, nil); err == nil {
		t.Error("missing snapshot must be refused")
	}

	// A render pair citing an artifact the bundle does not list.
	item, _ := itemByChange(p.change.Record, "c-breaking")
	r := envRender(t, p.st, item.ID, false)
	r.Artifacts = r.Artifacts[:1]
	if _, err := a.Assemble(ctx, p.change, p.snap, r); err == nil || !strings.Contains(err.Error(), "not in the manifest") {
		t.Errorf("unlisted render artifact: %v", err)
	}

	// A render correlated to a change item that does not exist.
	if _, err := a.Assemble(ctx, p.change, p.snap, envRender(t, p.st, "ci-doesnotexist", false)); err == nil || !strings.Contains(err.Error(), "unknown change item") {
		t.Errorf("unknown item: %v", err)
	}

	// Two different artifacts claiming one path.
	dup := *p.snap
	other, err := p.st.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFile, Path: p.snap.Artifacts[0].Path, Origin: p.snap.Artifacts[0].Origin,
		MediaType: "text/plain", URI: "x", Producer: "t@v1"}, []byte("different content"))
	if err != nil {
		t.Fatal(err)
	}
	dup.Artifacts = append(append([]domain.ArtifactRef{}, p.snap.Artifacts...), other)
	if _, err := a.Assemble(ctx, p.change, &dup, nil); err == nil || !strings.Contains(err.Error(), "claim the path") {
		t.Errorf("path collision: %v", err)
	}

	// An artifact whose bytes are not in the store.
	ghost := p.change.Artifacts[0]
	ghost.Path, ghost.Digest = "derived/change/ghost.json", "sha256:"+strings.Repeat("a", 64)
	ghost.ID = domain.ArtifactID(ghost.Kind, ghost.Path, ghost.Digest)
	cp := *p.change
	cp.Artifacts = append(append([]domain.ArtifactRef{}, p.change.Artifacts...), ghost)
	if _, err := a.Assemble(ctx, &cp, p.snap, nil); err == nil || !strings.Contains(err.Error(), "not retrievable") {
		t.Errorf("unstored artifact: %v", err)
	}
}
