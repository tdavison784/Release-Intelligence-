package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/tdavison784/release-intelligence/internal/env"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
)

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const secretManifest = `apiVersion: v1
kind: Secret
metadata:
  name: db-credentials
  namespace: shop
data:
  password: aHVudGVyMg==
`

// testRepo is a small GitOps repository: an Argo CD Application with inline
// values, a values file with a credential, and a Secret manifest.
func testRepo(t *testing.T) string {
	root := t.TempDir()
	write(t, root, "apps/cert-manager/values.yaml", "crds:\n  enabled: true\nwebhook:\n  password: hunter2\nreplicaCount: 2\n")
	write(t, root, "apps/shop/secret.yaml", secretManifest)
	write(t, root, "apps/shop/deploy.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: shop\n  namespace: shop\nspec:\n  replicas: 1\n")
	return root
}

func newSt(t *testing.T) *store.Store {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func read(t *testing.T, st *store.Store, digest string) string {
	t.Helper()
	rc, err := st.Open(context.Background(), digest)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	return string(b)
}

func TestSnapshotRepoRedactsAndPins(t *testing.T) {
	repo, st := testRepo(t), newSt(t)
	res, err := NewSnapshotBuilder(st).BuildSnapshot(context.Background(), investigate.EnvironmentInputs{Name: "shop-prod", Repo: repo, KubernetesVersion: "1.31"})
	if err != nil {
		t.Fatal(err)
	}
	snap := res.Snapshot
	if err := snap.Validate(); err != nil {
		t.Fatal(err)
	}
	if snap.Repo == nil || !strings.HasPrefix(snap.Repo.TreeDigest, "sha256:") || snap.KubernetesVersion != "1.31" {
		t.Fatalf("repo ref = %+v", snap.Repo)
	}
	for _, k := range []domain.SnapshotInputKind{domain.InputGitOpsRepo, domain.InputClusterVersion} {
		if s, ok := snap.InputStatus(k); !ok || s != domain.CollectionOK {
			t.Errorf("input %s: %s %v", k, s, ok)
		}
	}
	// No credential value is anywhere: not in any artifact, not in evidence.
	leaks := []string{"hunter2", "aHVudGVyMg=="}
	var redactedSecret, plainDeploy bool
	for _, a := range res.Artifacts {
		body := read(t, st, a.Digest)
		for _, l := range leaks {
			if strings.Contains(body, l) {
				t.Errorf("%q leaked into artifact %s", l, a.Path)
			}
		}
		switch a.Path {
		case "env/apps/shop/secret.yaml":
			redactedSecret = a.Redacted && strings.Contains(body, "<redacted sha256:") && strings.Contains(body, "password:")
		case "env/apps/shop/deploy.yaml":
			plainDeploy = !a.Redacted && strings.Contains(body, "replicas: 1")
		case "env/apps/cert-manager/values.yaml":
			if !a.Redacted || !strings.Contains(body, "replicaCount: 2") {
				t.Errorf("values file: redacted=%v\n%s", a.Redacted, body)
			}
		}
	}
	if !redactedSecret || !plainDeploy {
		t.Errorf("redaction flags wrong (secret=%v deploy=%v)", redactedSecret, plainDeploy)
	}
	raw, _ := json.Marshal(res.Evidence)
	for _, l := range leaks {
		if strings.Contains(string(raw), l) {
			t.Errorf("%q leaked into evidence", l)
		}
	}
	// The facts artifact is environment-facts and parses as JSON.
	facts, ok := findArtifact(res.Artifacts, snap.Facts)
	if !ok || facts.Kind != domain.ArtifactEnvironmentFacts {
		t.Fatalf("facts artifact missing: %+v", facts)
	}
	var tree any
	if err := json.Unmarshal([]byte(read(t, st, facts.Digest)), &tree); err != nil {
		t.Fatal(err)
	}
	for _, l := range leaks {
		if strings.Contains(read(t, st, facts.Digest), l) {
			t.Errorf("%q leaked into the facts", l)
		}
	}
}

func findArtifact(refs []domain.ArtifactRef, id string) (domain.ArtifactRef, bool) {
	for _, r := range refs {
		if r.ID == id {
			return r, true
		}
	}
	return domain.ArtifactRef{}, false
}

func TestSnapshotDeterministicAndContentSensitive(t *testing.T) {
	repo := testRepo(t)
	in := investigate.EnvironmentInputs{Name: "shop-prod", Repo: repo}
	a, err := NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if a.Snapshot.ID != b.Snapshot.ID {
		t.Fatalf("snapshot id is not reproducible: %s vs %s", a.Snapshot.ID, b.Snapshot.ID)
	}
	write(t, repo, "apps/shop/deploy.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: shop\n  namespace: shop\nspec:\n  replicas: 3\n")
	c, err := NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if c.Snapshot.ID == a.Snapshot.ID || c.Snapshot.Repo.TreeDigest == a.Snapshot.Repo.TreeDigest {
		t.Fatal("a changed file must change the tree digest and the snapshot id")
	}
}

func TestSnapshotGitCommitAndDirty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := testRepo(t)
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-q", "-m", "init")
	in := investigate.EnvironmentInputs{Name: "shop-prod", Repo: repo}
	clean, err := NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	// The enclosing commit is provenance, outside the identified snapshot.
	if r := clean.Snapshot.Repo; r.Commit != "" || r.Dirty || r.Path != "." {
		t.Fatalf("clean repo ref = %+v", r)
	}
	if p := clean.Provenance; p == nil || len(p.Commit) != 40 || p.Dirty {
		t.Fatalf("clean provenance = %+v", p)
	}
	write(t, repo, "apps/shop/deploy.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: shop\n  namespace: shop\nspec:\n  replicas: 5\n")
	dirty, err := NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if p := dirty.Provenance; p == nil || !p.Dirty || p.Commit != clean.Provenance.Commit {
		t.Fatalf("dirty provenance = %+v", p)
	}
	if dirty.Snapshot.ID == clean.Snapshot.ID {
		t.Fatal("changed content must change the snapshot id")
	}
}

func TestSnapshotNonGitDirectoryIsStillPinned(t *testing.T) {
	res, err := NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), investigate.EnvironmentInputs{Name: "e", Repo: testRepo(t)})
	if err != nil {
		t.Fatal(err)
	}
	// t.TempDir is outside any repository: no commit, but a tree digest.
	if r := res.Snapshot.Repo; r.Commit != "" || r.TreeDigest == "" {
		t.Fatalf("repo ref = %+v", r)
	}
}

func TestSnapshotExplicitInputsAndStatuses(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "values.yaml", "replicaCount: 2\n")
	write(t, dir, "broken.yaml", "a: [unterminated\n")
	res, err := NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), investigate.EnvironmentInputs{
		Name: "e", ValuesFiles: []string{filepath.Join(dir, "values.yaml")}, Manifests: []string{filepath.Join(dir, "broken.yaml")},
		KubernetesVersion: "1.30", Live: &investigate.LiveOptions{Context: "prod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	if s, _ := res.Snapshot.InputStatus(domain.InputValues); s != domain.CollectionOK {
		t.Errorf("values: %s", s)
	}
	if s, _ := res.Snapshot.InputStatus(domain.InputManifests); !s.Gap() {
		t.Errorf("a manifest that does not parse must be a gap, got %s", s)
	}
	if s, ok := res.Snapshot.InputStatus(domain.InputLiveCluster); !ok || s != domain.CollectionUnavailable {
		t.Errorf("live observation is not implemented: %s %v", s, ok)
	}
	if res.Snapshot.Repo != nil {
		t.Errorf("no repo was given, none must be recorded")
	}
	for _, a := range res.Artifacts {
		if a.Kind == domain.ArtifactEnvironmentFile && !strings.HasPrefix(a.Path, "env/inputs/") {
			t.Errorf("explicit inputs live below env/inputs/, got %s", a.Path)
		}
	}
}

func TestSnapshotErrors(t *testing.T) {
	b := NewSnapshotBuilder(newSt(t))
	ctx := context.Background()
	if _, err := b.BuildSnapshot(ctx, investigate.EnvironmentInputs{Repo: t.TempDir()}); err == nil {
		t.Error("a missing environment name must be refused")
	}
	if _, err := b.BuildSnapshot(ctx, investigate.EnvironmentInputs{Name: "e"}); err == nil {
		t.Error("an environment with no input must be refused")
	}
	if _, err := b.BuildSnapshot(ctx, investigate.EnvironmentInputs{Name: "e", ValuesFiles: []string{"/definitely/not/here.yaml"}}); err == nil {
		t.Error("an unreadable input must be an error")
	}
}

func readFrom(t *testing.T, st *store.Store, digest string) string { return read(t, st, digest) }

func kustomizeRepo(t *testing.T) string {
	root := t.TempDir()
	write(t, root, "base/deploy.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: shop\nspec:\n  replicas: 1\n")
	write(t, root, "base/kustomization.yaml", "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - deploy.yaml\n")
	write(t, root, "apps/cert-manager/values.yaml", "replicaCount: 2\n")
	return root
}

func repoStatus(t *testing.T, res *investigate.SnapshotResult) (domain.CollectionStatus, string) {
	t.Helper()
	for _, in := range res.Snapshot.Inputs {
		if in.Kind == domain.InputGitOpsRepo {
			return in.Status, in.Reason
		}
	}
	t.Fatal("no repo input")
	return "", ""
}

// A kustomization the snapshot builds is not a collection gap: the loader's
// "not built" warning is about the loader.
func TestSnapshotBuiltKustomizationIsComplete(t *testing.T) {
	repo := kustomizeRepo(t)
	b := NewSnapshotBuilder(newSt(t)).(*SnapshotBuilder)
	var built []string
	b.kbuild = func(_ context.Context, root, dir string) error { built = append(built, dir); return nil }
	res, err := b.BuildSnapshot(context.Background(), investigate.EnvironmentInputs{Name: "e", Repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	if len(built) != 1 || !strings.HasSuffix(built[0], "base") {
		t.Fatalf("built = %v", built)
	}
	if st, why := repoStatus(t, res); st != domain.CollectionOK {
		t.Fatalf("repo status = %s (%s), want ok", st, why)
	}
}

// A kustomization that cannot be built stays partial, with the reason.
func TestSnapshotUnbuildableKustomizationStaysPartial(t *testing.T) {
	repo := kustomizeRepo(t)
	b := NewSnapshotBuilder(newSt(t)).(*SnapshotBuilder)
	b.kbuild = func(context.Context, string, string) error { return errors.New("accumulating resources: missing base") }
	res, err := b.BuildSnapshot(context.Background(), investigate.EnvironmentInputs{Name: "e", Repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	st, why := repoStatus(t, res)
	if st != domain.CollectionPartial || !strings.Contains(why, "cannot be built") || !strings.Contains(why, "missing base") {
		t.Fatalf("repo status = %s (%s)", st, why)
	}
}

// With the real kustomize: a good kustomization is ok, a broken one partial.
func TestSnapshotRealKustomize(t *testing.T) {
	if _, err := exec.LookPath("kustomize"); err != nil {
		if _, err := exec.LookPath("kubectl"); err != nil {
			t.Skip("neither kustomize nor kubectl is on PATH")
		}
	}
	good := kustomizeRepo(t)
	res, err := NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), investigate.EnvironmentInputs{Name: "e", Repo: good})
	if err != nil {
		t.Fatal(err)
	}
	if st, why := repoStatus(t, res); st != domain.CollectionOK {
		t.Fatalf("good kustomization: %s (%s)", st, why)
	}
	bad := kustomizeRepo(t)
	write(t, bad, "base/kustomization.yaml", "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - deploy.yaml\npatches:\n  - path: nope.yaml\n")
	res, err = NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), investigate.EnvironmentInputs{Name: "e", Repo: bad})
	if err != nil {
		t.Fatal(err)
	}
	if st, why := repoStatus(t, res); st != domain.CollectionPartial || !strings.Contains(why, "cannot be built") {
		t.Fatalf("broken kustomization: %s (%s)", st, why)
	}
}

// Principle 6: the same environment content in another checkout, at another
// path and another commit, is the same snapshot: same id, same bundle id and
// bytes. Paths and evidence URIs are repo-relative; the checkout is provenance.
func TestSnapshotAndBundleIdenticalAcrossCheckouts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx := context.Background()
	build := func(commitMsg string) (*investigate.SnapshotResult, *domain.EvidenceBundle) {
		repo := testRepo(t)
		for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"commit", "-q", "-m", commitMsg}} {
			cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		st := newSt(t)
		change, err := NewChangeCollectorOver(&fakeUpstream{edge: fixtureEdge(), pair: fixturePair()}, st).CollectChange(ctx,
			investigate.ChangeRequest{Product: "acme", From: "v1.0.0", To: "v2.0.0", Render: true})
		if err != nil {
			t.Fatal(err)
		}
		snap, err := NewSnapshotBuilder(st).BuildSnapshot(ctx, investigate.EnvironmentInputs{Name: "shop-prod", Repo: repo, Product: "acme"})
		if err != nil {
			t.Fatal(err)
		}
		b, err := NewAssembler(st).Assemble(ctx, change, snap, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range b.Evidence {
			if strings.Contains(e.URI, repo) || strings.HasPrefix(e.URI, "/") && !strings.HasPrefix(e.URI, "/acme") {
				t.Errorf("evidence %s has a checkout-specific URI %q", e.ID, e.URI)
			}
		}
		for _, a := range b.Manifest.Artifacts {
			if strings.Contains(a.URI, repo) {
				t.Errorf("artifact %s has a checkout-specific URI %q", a.Path, a.URI)
			}
		}
		raw, _ := json.Marshal(b)
		if strings.Contains(string(raw), repo) {
			t.Errorf("the bundle mentions the checkout path %s", repo)
		}
		return snap, b
	}
	s1, b1 := build("first commit")
	s2, b2 := build("another commit, another checkout")
	if s1.Provenance.Commit == s2.Provenance.Commit || s1.Provenance.Path == s2.Provenance.Path {
		t.Fatal("the test needs two different checkouts and commits")
	}
	if s1.Snapshot.ID != s2.Snapshot.ID || b1.ID != b2.ID {
		t.Fatalf("same content, different identity: snapshot %s/%s bundle %s/%s", s1.Snapshot.ID, s2.Snapshot.ID, b1.ID, b2.ID)
	}
	j1, _ := json.Marshal(b1)
	j2, _ := json.Marshal(b2)
	if !bytes.Equal(j1, j2) {
		t.Fatal("same content, different bundle bytes")
	}
}

const argoCertManager = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
  namespace: argocd
spec:
  project: platform
  destination:
    server: https://kubernetes.default.svc
    namespace: cert-manager
  sources:
    - repoURL: https://charts.jetstack.io
      chart: cert-manager
      targetRevision: v1.17.2
      helm:
        valueFiles:
          - $values/platform/cert-manager/values.yaml
    - repoURL: https://git.example.com/platform/gitops.git
      targetRevision: main
      ref: values
`

const argoOther = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: ingress-nginx
  namespace: argocd
spec:
  project: platform
  destination:
    server: https://kubernetes.default.svc
    namespace: ingress
  source:
    repoURL: https://kubernetes.github.io/ingress-nginx
    chart: ingress-nginx
    targetRevision: main
`

func argoRepo(t *testing.T, files map[string]string) string {
	root := t.TempDir()
	for p, body := range files {
		write(t, root, p, body)
	}
	return root
}

func unparsableWarnings(t *testing.T, repo string) []string {
	t.Helper()
	e, err := env.Load(env.Inputs{Repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, w := range e.Warnings {
		if strings.Contains(w, "not a parsable version") {
			out = append(out, w)
		}
	}
	return out
}

// A non-semver targetRevision on an Application that does not deploy the
// product is a recorded note, not a gap.
func TestSnapshotOtherProductsBranchIsANoteNotAGap(t *testing.T) {
	repo := argoRepo(t, map[string]string{"apps/ingress.yaml": argoOther, "platform/cert-manager/values.yaml": "replicaCount: 2\n"})
	ws := unparsableWarnings(t, repo)
	if len(ws) == 0 {
		t.Skip("the env loader raised no unparsable-version warning for the fixture")
	}
	res, err := NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), investigate.EnvironmentInputs{Name: "e", Repo: repo, Product: "cert-manager"})
	if err != nil {
		t.Fatal(err)
	}
	st, why := repoStatus(t, res)
	if st != domain.CollectionOK || !strings.Contains(why, "note:") {
		t.Fatalf("other product's branch: %s (%q), want ok with a note", st, why)
	}
}

// The product's own deployment tracking a branch leaves its deployed version
// unknown: the gap stays (and without a product everything is a gap).
func TestSnapshotOwnProductsBranchStaysAGap(t *testing.T) {
	repo := argoRepo(t, map[string]string{"apps/ingress.yaml": argoOther})
	if len(unparsableWarnings(t, repo)) == 0 {
		t.Skip("the env loader raised no unparsable-version warning for the fixture")
	}
	for _, product := range []string{"ingress-nginx", ""} {
		res, err := NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), investigate.EnvironmentInputs{Name: "e", Repo: repo, Product: product})
		if err != nil {
			t.Fatal(err)
		}
		if st, why := repoStatus(t, res); st != domain.CollectionPartial || !strings.Contains(why, "not a parsable version") {
			t.Fatalf("product %q: %s (%q), want partial", product, st, why)
		}
	}
}

// An environment that layers several values files for one release (the
// normal Argo helm.valueFiles / Flux valuesFrom / helmfile shape) is fully
// collected: the loader merges the layers, so the repository input is ok —
// not a partial collection that would block every values item — and the
// facts carry the merged keys applied in the Application's list order (the
// later list entry wins even when an alphabetical walk would load it first).
func TestSnapshotLayeredValuesFilesAreComplete(t *testing.T) {
	root := t.TempDir()
	write(t, root, "apps/cert-manager.yaml", argoValuesApp)
	write(t, root, "platform/cert-manager/values.yaml", "replicaCount: 2\nwebhook:\n  timeoutSeconds: 10\n")
	write(t, root, "platform/cert-manager/values-prod.yaml", "replicaCount: 3\nprometheus:\n  servicemonitor:\n    enabled: true\n")
	st := newSt(t)
	res, err := NewSnapshotBuilder(st).BuildSnapshot(context.Background(), investigate.EnvironmentInputs{Name: "e", Repo: root})
	if err != nil {
		t.Fatal(err)
	}
	if s, why := repoStatus(t, res); s != domain.CollectionOK {
		t.Fatalf("layered values files: %s (%q), want ok", s, why)
	}
	facts, ok := findArtifact(res.Artifacts, res.Snapshot.Facts)
	if !ok {
		t.Fatal("facts artifact missing")
	}
	body := read(t, st, facts.Digest)
	for _, want := range []string{
		`"Path": "replicaCount"`, `"Value": "3"`, // the later list entry wins
		`"Path": "webhook.timeoutSeconds"`,            // deep-merge keeps the base key
		`"Path": "prometheus.servicemonitor.enabled"`, // the overlay adds its key
		`values-prod.yaml`,                            // provenance of the effective value
	} {
		if !strings.Contains(body, want) {
			t.Errorf("facts missing %s in merged values:\n%s", want, body)
		}
	}
	if strings.Count(body, `"Path": "replicaCount"`) != 1 {
		t.Errorf("replicaCount appears more than once in the effective values:\n%s", body)
	}
}

// argoValuesApp: a multi-source Application whose second source only carries
// values — the platform gitops shape. The valueFiles list order is Helm's -f
// order: values.yaml applies before values-prod.yaml even though
// "values-prod.yaml" sorts first alphabetically.
const argoValuesApp = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
spec:
  destination:
    namespace: cert-manager
  sources:
    - repoURL: https://charts.jetstack.io
      chart: cert-manager
      targetRevision: v1.17.0
      helm:
        valueFiles:
          - $values/platform/cert-manager/values.yaml
          - $values/platform/cert-manager/values-prod.yaml
    - repoURL: https://git.example.com/platform/gitops.git
      ref: values
`

func TestSplitWarnings(t *testing.T) {
	ws := []string{
		`detected-argo "cert-manager": version "main" is not a parsable version; kept as stated, it cannot be range-checked`,
		`detected-argo "prod": version "main" is not a parsable version; kept as stated, it cannot be range-checked`,
		`kustomization x references y, which is not present`,
	}
	gaps, notes := splitWarnings(ws, "cert-manager")
	if len(gaps) != 2 || len(notes) != 1 || !strings.Contains(notes[0], `"prod"`) {
		t.Fatalf("gaps %v notes %v", gaps, notes)
	}
	if gaps, notes := splitWarnings(ws, ""); len(gaps) != 3 || len(notes) != 0 {
		t.Fatalf("no product must fail closed: gaps %v notes %v", gaps, notes)
	}
}
