package renderdiff

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// memStore is an in-memory investigate.ArtifactStore.
type memStore struct {
	mu    sync.Mutex
	bytes map[string][]byte
	puts  int
}

func newMemStore() *memStore { return &memStore{bytes: map[string][]byte{}} }

func (s *memStore) Put(_ context.Context, spec investigate.ArtifactSpec, content []byte) (domain.ArtifactRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := domain.Digest(content)
	s.bytes[d] = append([]byte(nil), content...)
	s.puts++
	ref := domain.ArtifactRef{ID: domain.ArtifactID(spec.Kind, spec.Path, d), Kind: spec.Kind, MediaType: spec.MediaType, Digest: d,
		Size: int64(len(content)), Path: spec.Path, Origin: spec.Origin, URI: spec.URI, Producer: spec.Producer, Redacted: spec.Redacted,
		Tags: spec.Tags, ChangeItems: spec.ChangeItems}
	return ref, ref.Validate()
}

func (s *memStore) Open(_ context.Context, digest string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.bytes[digest]
	if !ok {
		return nil, investigate.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (s *memStore) text(t *testing.T, digest string) string {
	t.Helper()
	b, ok := s.bytes[digest]
	if !ok {
		t.Fatalf("digest %s not stored", digest)
	}
	return string(b)
}

// fakeHelm renders fixed streams per chart version. A values layer
// containing "reject: true" makes the 2.0.0 chart refuse the values; the
// release name "broken" fails both renders as if helm were missing.
type fakeHelm struct{ streams map[string]string }

func (h *fakeHelm) Tool() render.Tool                       { return render.ToolHelm }
func (h *fakeHelm) Version(context.Context) (string, error) { return "v3.test", nil }
func (h *fakeHelm) Render(_ context.Context, req render.Request) *render.Result {
	prov := render.Provenance{Scope: req.Scope, Tool: render.ToolHelm, ToolVersion: "v3.test",
		Command: []string{"helm", "template", req.ReleaseName, "chart.tgz", "--namespace", req.Namespace},
		Chart:   req.Chart.Name, ChartVersion: req.Chart.Version, ChartURI: req.Chart.URI, ArtifactDigest: req.Chart.Digest,
		ReleaseName: req.ReleaseName, Namespace: req.Namespace, ValuesComplete: req.ValuesComplete, IncompleteReason: req.IncompleteReason}
	for _, l := range req.Values {
		prov.Values = append(prov.Values, render.InputDigest{Origin: l.Origin, Digest: l.Digest()})
	}
	fail := func(r render.FailureReason, d string) *render.Result {
		return &render.Result{Status: render.StatusFailed, Failure: &render.Failure{Reason: r, Detail: d}, Provenance: prov}
	}
	if req.ReleaseName == "broken" {
		return fail(render.FailRendererUnavailable, "helm is not installed")
	}
	for _, l := range req.Values {
		if req.Chart.Version == "2.0.0" && bytes.Contains(l.Content, []byte("reject: true")) {
			return fail(render.FailInvalidValues, "values don't meet the specifications of the schema: reject is not allowed")
		}
	}
	out := []byte(h.streams[req.Chart.Version])
	objs, err := render.ParseObjects(out)
	if err != nil {
		return fail(render.FailOutputUnparsable, err.Error())
	}
	prov.OutputDigest = domain.Digest(out)
	return &render.Result{Status: render.StatusSucceeded, Provenance: prov, Output: out, Objects: objs}
}

// fakeKustomize renders a fixed stream per overlay directory (base name).
type fakeKustomize struct{ streams map[string]string }

func (k *fakeKustomize) Tool() render.Tool                       { return render.ToolKustomize }
func (k *fakeKustomize) Version(context.Context) (string, error) { return "v5.test", nil }
func (k *fakeKustomize) Render(_ context.Context, req render.Request) *render.Result {
	in := req.Kustomize
	out := []byte(k.streams[filepath.Base(in.Dir)])
	objs, err := render.ParseObjects(out)
	prov := render.Provenance{Scope: req.Scope, Tool: render.ToolKustomize, ToolVersion: "v5.test", Command: []string{"kustomize", "build", in.Dir},
		KustomizeRoot: in.Root, KustomizeDir: in.Dir, ValuesComplete: true, OutputDigest: domain.Digest(out)}
	if err != nil {
		return &render.Result{Status: render.StatusFailed, Failure: &render.Failure{Reason: render.FailOutputUnparsable, Detail: err.Error()}, Provenance: prov}
	}
	return &render.Result{Status: render.StatusSucceeded, Provenance: prov, Output: out, Objects: objs}
}

// fakeCharts resolves local chart directories (Chart.yaml + values.yaml).
type fakeCharts struct{ dirs map[string]string }

func (c *fakeCharts) ResolveChart(_ context.Context, product, version string) (*render.Chart, error) {
	d, ok := c.dirs[version]
	if !ok {
		return nil, &render.Failure{Reason: render.FailChartUnavailable, Detail: "no chart " + version}
	}
	return &render.Chart{Name: product, Version: version, URI: "https://charts.example.io/" + product + "-" + version + ".tgz",
		Digest: domain.Digest([]byte(product + version)), Path: d}, nil
}

const chartValues = `replicaCount: 1
image:
  repository: example.io/acme/controller
controller:
  args: []
service:
  type: ClusterIP
podLabels: {}
`

const chartYAML = `apiVersion: v2
name: acme
version: %s
dependencies:
  - name: redis
    version: 1.0.0
    repository: https://charts.example.io
`

const fromStream = `---
# Source: acme/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: acme
  namespace: acme
  labels:
    app.kubernetes.io/name: acme
spec:
  template:
    spec:
      containers:
        - name: controller
          image: example.io/acme/controller:1.0.0
          args:
            - --leader-elect
            - --log-level=info
          ports:
            - containerPort: 8080
              protocol: TCP
---
# Source: acme/templates/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: acme
  namespace: acme
  annotations:
    example.io/team: platform
spec:
  ports:
    - name: http
      port: 80
      targetPort: 8080
---
# Source: acme/templates/secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: acme-token
  namespace: acme
stringData:
  token: s3cr3t-from
---
# Source: acme/templates/rbac.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: acme
rules:
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list"]
`

const toStream = `---
# Source: acme/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: acme
  namespace: acme
  labels:
    app.kubernetes.io/name: acme
spec:
  template:
    spec:
      containers:
        - name: controller
          image: example.io/acme/controller:2.0.0
          args:
            - --leader-elect
            - --log-level=debug
            - --default-private-key-rotation-policy=Always
            - --enable-foo
          ports:
            - containerPort: 8080
              protocol: TCP
---
# Source: acme/templates/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: acme
  namespace: acme
  annotations:
    example.io/team: core
spec:
  ports:
    - name: http
      port: 80
      targetPort: http
---
# Source: acme/templates/secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: acme-token
  namespace: acme
stringData:
  token: s3cr3t-to
---
# Source: acme/templates/rbac.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: acme
rules:
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "watch"]
---
# Source: acme/templates/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: acme-config
  namespace: acme
data:
  mode: strict
`

// fixture is one renderer test setup.
type fixture struct {
	st      *memStore
	r       *Renderer
	change  *investigate.ChangeResult
	snap    *investigate.SnapshotResult
	base    *domain.EvidenceBundle
	targets []render.Target
	repo    string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	charts := &fakeCharts{dirs: map[string]string{}}
	for _, v := range []string{"1.0.0", "2.0.0"} {
		d := filepath.Join(dir, "charts", v)
		must(t, os.MkdirAll(d, 0o755))
		must(t, os.WriteFile(filepath.Join(d, "values.yaml"), []byte(chartValues), 0o644))
		must(t, os.WriteFile(filepath.Join(d, "Chart.yaml"), []byte(fmt.Sprintf(chartYAML, v)), 0o644))
		charts.dirs[v] = d
	}
	f := &fixture{st: newMemStore(), repo: filepath.Join(dir, "repo")}
	must(t, os.MkdirAll(f.repo, 0o755))
	eng := &render.Engine{Helm: &fakeHelm{streams: map[string]string{"1.0.0": fromStream, "2.0.0": toStream}},
		Kustomize: &fakeKustomize{streams: map[string]string{}}, Charts: charts}
	f.r = newRenderer(f.st, eng, func(string, app.RenderOptions) ([]render.Target, []string, error) {
		return f.targets, []string{"values file " + f.repo + "/other.yaml: skipped (its name does not name acme)"}, nil
	})
	f.loadBase(t)
	return f
}

// loadBase builds the change and snapshot results from the contract's
// sample bundle, with the fake upgrade edge stored as the edge artifact.
func (f *fixture) loadBase(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "domain", "testdata", "sample-evidence-bundle.json"))
	must(t, err)
	var b domain.EvidenceBundle
	must(t, json.Unmarshal(raw, &b))
	b.Renders = nil
	edge := fakeEdge()
	eb, err := json.Marshal(edge)
	must(t, err)
	ref, err := f.st.Put(context.Background(), investigate.ArtifactSpec{Kind: domain.ArtifactUpgradeEdge, Path: "derived/upgrade/acme/edge.json",
		Origin: domain.OriginDerived, MediaType: "application/json", URI: "ri:upgrade@v1", Producer: "upgrade@v1"}, eb)
	must(t, err)
	var arts []domain.ArtifactRef
	for _, a := range b.Manifest.Artifacts {
		if a.ID != b.Change.Edge && a.Kind != domain.ArtifactRenderDiff && a.Kind != domain.ArtifactRenderedManifests {
			arts = append(arts, a)
		}
	}
	b.Manifest.Artifacts = append(arts, ref)
	b.Change.Edge = ref.ID
	b.Change.ID = b.Change.DerivedID()
	f.base = &b
	f.change = &investigate.ChangeResult{Record: b.Change, Artifacts: []domain.ArtifactRef{ref}, Evidence: b.Evidence}
	snap := b.Snapshot
	snap.Repo = &domain.RepoRef{Path: f.repo, TreeDigest: snap.Repo.TreeDigest}
	f.snap = &investigate.SnapshotResult{Snapshot: snap}
}

// bundle assembles a bundle from the base and a render result, the way the
// bundle lane's assembler will (manifest = every artifact, sorted by path).
func (f *fixture) bundle(t *testing.T, res *investigate.RenderResult) *domain.EvidenceBundle {
	t.Helper()
	b := *f.base
	b.Change = f.change.Record
	b.Renders = res.Pairs
	b.EnvironmentItems = res.Items
	arts := append(append([]domain.ArtifactRef(nil), f.base.Manifest.Artifacts...), res.Artifacts...)
	sort.Slice(arts, func(i, j int) bool { return arts[i].Path < arts[j].Path })
	b.Manifest = domain.BundleManifest{Artifacts: arts}
	b.Evidence = append(append([]domain.Evidence(nil), f.base.Evidence...), res.Evidence...)
	b.ID = domain.BundleID(b.Change.ID, b.Snapshot.ID, b.Manifest.Digest(), b.Renders)
	return &b
}

func fakeEdge() *domain.UpgradeEdge {
	img := func(tag string) *domain.ArtifactInstance {
		return &domain.ArtifactInstance{ArtifactID: "controller", Type: domain.ArtifactContainerImage, Name: "controller",
			Coordinate: "example.io/acme/controller:" + tag, Status: domain.ArtifactVerified}
	}
	return &domain.UpgradeEdge{SchemaVersion: domain.UpgradeEdgeSchemaVersion, Product: domain.ProductRef{ID: "acme", Name: "Acme"},
		From: domain.MustVersion("1.0.0", "1.0.0"), To: domain.MustVersion("2.0.0", "2.0.0"),
		Changes: []domain.Change{
			{ID: "chg-a", Category: domain.CategoryConfiguration, Title: "Private key rotation policy defaults to Always",
				Subjects: []string{"--default-private-key-rotation-policy"}},
			{ID: "chg-b", Category: domain.CategoryConfiguration, Title: "Service targets the named port", Subjects: []string{"http"}},
			{ID: "chg-c", Category: domain.CategoryOther, Title: "Bump Go to 1.24"},
			{ID: "chg-routine", Category: domain.CategoryDependency, Title: "Bump deps", Subjects: []string{"--enable-foo"}, Routine: true},
		},
		Artifacts: []domain.ArtifactChange{{ArtifactID: "controller", Type: domain.ArtifactContainerImage, Name: "controller",
			Change: domain.ChangeUpdated, From: img("1.0.0"), To: img("2.0.0")}},
	}
}

func helmTarget(id string, kind render.TargetKind, origin, values string) render.Target {
	t := render.Target{ID: id, Kind: kind, Origin: origin, Why: "test", ReleaseName: "acme", Namespace: "acme", ValuesComplete: true}
	if values != "" {
		t.Values = []render.ValuesLayer{{Origin: origin, Content: []byte(values)}}
	}
	return t
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func renderOK(t *testing.T, f *fixture) *investigate.RenderResult {
	t.Helper()
	res, err := f.r.RenderEnvironment(context.Background(), f.change, f.snap, investigate.Config{Render: true})
	must(t, err)
	return res
}

func pairByPrefix(t *testing.T, res *investigate.RenderResult, prefix string) domain.ManifestPair {
	t.Helper()
	for _, p := range res.Pairs {
		if strings.HasPrefix(p.Target, prefix) {
			return p
		}
	}
	t.Fatalf("no pair %s* in %v", prefix, targetNames(res.Pairs))
	return domain.ManifestPair{}
}
