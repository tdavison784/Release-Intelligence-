package bundle

import (
	"context"
	"encoding/json"
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
	if r := clean.Snapshot.Repo; len(r.Commit) != 40 || r.Dirty {
		t.Fatalf("clean repo ref = %+v", r)
	}
	write(t, repo, "apps/shop/deploy.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: shop\n  namespace: shop\nspec:\n  replicas: 5\n")
	dirty, err := NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if r := dirty.Snapshot.Repo; !r.Dirty || r.Commit != clean.Snapshot.Repo.Commit {
		t.Fatalf("dirty repo ref = %+v", r)
	}
	if dirty.Snapshot.ID == clean.Snapshot.ID {
		t.Fatal("dirty state must change the snapshot id")
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
