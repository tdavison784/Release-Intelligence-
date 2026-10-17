package relations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// memStore is an in-memory ArtifactStore fake.
type memStore struct{ blobs map[string][]byte }

func newMemStore() *memStore { return &memStore{blobs: map[string][]byte{}} }

func (s *memStore) Put(_ context.Context, spec investigate.ArtifactSpec, content []byte) (domain.ArtifactRef, error) {
	sum := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	s.blobs[digest] = content
	return domain.ArtifactRef{
		ID: domain.ArtifactID(spec.Kind, spec.Path, digest), Kind: spec.Kind, MediaType: spec.MediaType, Digest: digest,
		Size: int64(len(content)), Path: spec.Path, Origin: spec.Origin, URI: spec.URI, Producer: spec.Producer,
	}, nil
}

func (s *memStore) Open(_ context.Context, digest string) (io.ReadCloser, error) {
	b, ok := s.blobs[digest]
	if !ok {
		return nil, investigate.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// fixture assembles a minimal bundle (only what the relations package reads).
type fixture struct {
	t       *testing.T
	st      *memStore
	b       *domain.EvidenceBundle
	reverse bool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	return &fixture{t: t, st: newMemStore(), b: &domain.EvidenceBundle{
		ID:     "bnd-test",
		Change: domain.ChangeEvidenceRecord{Product: domain.ProductRef{ID: "widget"}, To: domain.Version{Tag: "v2.0.0"}},
	}}
}

func (f *fixture) put(kind domain.ArtifactKind, origin domain.ArtifactOrigin, p, content string) domain.ArtifactRef {
	f.t.Helper()
	ref, err := f.st.Put(context.Background(), investigate.ArtifactSpec{Kind: kind, Path: p, Origin: origin, MediaType: "application/yaml", URI: p, Producer: "test@v1"}, []byte(content))
	if err != nil {
		f.t.Fatal(err)
	}
	f.b.Manifest.Artifacts = append(f.b.Manifest.Artifacts, ref)
	sort.Slice(f.b.Manifest.Artifacts, func(i, j int) bool { return f.b.Manifest.Artifacts[i].Path < f.b.Manifest.Artifacts[j].Path })
	return ref
}

// file adds an environment file at repo path p.
func (f *fixture) file(p, content string) domain.ArtifactRef {
	return f.put(domain.ArtifactEnvironmentFile, domain.OriginEnvironment, "env/"+p, content)
}

// render adds a complete environment pair whose TO render is `to`.
func (f *fixture) render(target, to string, changes ...domain.RenderedChange) {
	f.t.Helper()
	from := f.put(domain.ArtifactRenderedManifests, domain.OriginDerived, "derived/render/env/"+target+"/from.yaml", "# from\n")
	toRef := f.put(domain.ArtifactRenderedManifests, domain.OriginDerived, "derived/render/env/"+target+"/to.yaml", to)
	diff := f.put(domain.ArtifactRenderDiff, domain.OriginDerived, "derived/render/env/"+target+"/diff.json", "{}")
	for i := range changes {
		changes[i].ID = domain.RenderedChangeID(domain.RenderEnvironment, target, changes[i].Object, changes[i].Path, changes[i].Class)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].ID < changes[j].ID })
	f.b.Renders = append(f.b.Renders, domain.ManifestPair{Scope: domain.RenderEnvironment, Target: target, From: from.ID, To: toRef.ID, Diff: diff.ID, Status: domain.CollectionOK, Changes: changes})
}

func (f *fixture) build(opts ...Option) *domain.RelationshipGraph {
	f.t.Helper()
	if f.reverse {
		a := f.b.Manifest.Artifacts
		for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
			a[i], a[j] = a[j], a[i]
		}
	}
	g, err := NewBuilder(f.st, opts...).BuildGraph(context.Background(), f.b)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := g.Validate(); err != nil {
		f.t.Fatalf("graph does not validate: %v", err)
	}
	return g
}

// view makes graph assertions readable.
type view struct {
	t *testing.T
	g *domain.RelationshipGraph
}

func (v view) node(key string) (domain.GraphNode, bool) {
	for _, n := range v.g.Nodes {
		if n.Key == key {
			return n, true
		}
	}
	return domain.GraphNode{}, false
}

func (v view) mustNode(key string) domain.GraphNode {
	v.t.Helper()
	n, ok := v.node(key)
	if !ok {
		keys := []string{}
		for _, n := range v.g.Nodes {
			keys = append(keys, n.Key)
		}
		v.t.Fatalf("no node %q; nodes:\n  %s", key, strings.Join(keys, "\n  "))
	}
	return n
}

// edge finds an edge of kind from → to by node key.
func (v view) edge(kind domain.EdgeKind, from, to string) (domain.GraphEdge, bool) {
	f, t := v.mustNode(from).ID, v.mustNode(to).ID
	for _, e := range v.g.Edges {
		if e.Kind == kind && e.From == f && e.To == t {
			return e, true
		}
	}
	return domain.GraphEdge{}, false
}

func (v view) wantEdge(kind domain.EdgeKind, from, to string, status domain.EdgeStatus) domain.GraphEdge {
	v.t.Helper()
	e, ok := v.edge(kind, from, to)
	if !ok {
		var all []string
		names := map[string]string{}
		for _, n := range v.g.Nodes {
			names[n.ID] = n.Key
		}
		for _, e := range v.g.Edges {
			all = append(all, fmt.Sprintf("%s %s -> %s [%s]", e.Kind, names[e.From], names[e.To], e.Status))
		}
		v.t.Fatalf("no %s edge %s -> %s; edges:\n  %s", kind, from, to, strings.Join(all, "\n  "))
	}
	if e.Status != status {
		v.t.Fatalf("%s edge %s -> %s status %s, want %s (reason %q)", kind, from, to, e.Status, status, e.Reason)
	}
	return e
}

// loadEnvDir adds every file under dir to the fixture as env/<relative path>.
func (f *fixture) loadEnvDir(dir string) int {
	f.t.Helper()
	n := 0
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		f.file(filepath.ToSlash(rel), string(data))
		n++
		return nil
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return n
}
