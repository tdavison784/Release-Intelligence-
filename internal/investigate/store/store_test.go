package store

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func spec(path string) investigate.ArtifactSpec {
	return investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFile, Path: path, Origin: domain.OriginEnvironment,
		MediaType: "application/yaml", URI: path, Producer: "test@v1", Tags: []string{"B", "a", "b"}}
}

func TestPutOpenRoundTrip(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	ref, err := s.Put(ctx, spec("env/a.yaml"), []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if ref.Size != 5 || !strings.HasPrefix(ref.Digest, "sha256:") || ref.ID == "" {
		t.Fatalf("ref = %+v", ref)
	}
	if strings.Join(ref.Tags, ",") != "a,b" {
		t.Fatalf("tags not normalised: %v", ref.Tags)
	}
	again, err := s.Put(ctx, spec("env/a.yaml"), []byte("hello"))
	if err != nil || again.ID != ref.ID {
		t.Fatalf("Put is not idempotent: %v %v", again, err)
	}
	rc, err := s.Open(ctx, ref.Digest)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if b, _ := io.ReadAll(rc); string(b) != "hello" {
		t.Fatalf("read %q", b)
	}
	// the same bytes at another path share one CAS file
	other, err := s.Put(ctx, spec("env/b.yaml"), []byte("hello"))
	if err != nil || other.Digest != ref.Digest || other.ID == ref.ID {
		t.Fatalf("other = %+v %v", other, err)
	}
}

func TestPutRejectsBadSpec(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	for name, sp := range map[string]investigate.ArtifactSpec{
		"wrong origin prefix": {Kind: domain.ArtifactEnvironmentFile, Path: "upstream/x", Origin: domain.OriginEnvironment, MediaType: "x/y", URI: "u", Producer: "p@v1"},
		"traversal":           {Kind: domain.ArtifactEnvironmentFile, Path: "env/../x", Origin: domain.OriginEnvironment, MediaType: "x/y", URI: "u", Producer: "p@v1"},
		"unknown kind":        {Kind: "nope", Path: "env/x", Origin: domain.OriginEnvironment, MediaType: "x/y", URI: "u", Producer: "p@v1"},
		"no producer":         {Kind: domain.ArtifactEnvironmentFile, Path: "env/x", Origin: domain.OriginEnvironment, MediaType: "x/y", URI: "u"},
	} {
		if _, err := s.Put(ctx, sp, []byte("x")); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestOpenErrors(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	if _, err := s.Open(ctx, "sha256:"+strings.Repeat("0", 64)); !errors.Is(err, investigate.ErrNotFound) {
		t.Fatalf("missing digest: %v", err)
	}
	if _, err := s.Open(ctx, "../../etc/passwd"); err == nil {
		t.Fatal("a non-digest must be rejected")
	}
}

func TestOpenVerifiesDigest(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	ref, _ := s.Put(ctx, spec("env/a.yaml"), []byte("hello"))
	p, _ := s.casPath(ref.Digest)
	if err := os.WriteFile(p, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, ref.Digest); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("tampered bytes must fail verification, got %v", err)
	}
	// a Put of the original bytes repairs the file
	if _, err := s.Put(ctx, spec("env/a.yaml"), []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, ref.Digest); err != nil {
		t.Fatalf("repair failed: %v", err)
	}
}

func TestNoTempFilesLeft(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	if _, err := s.Put(ctx, spec("env/a.yaml"), []byte("hello")); err != nil {
		t.Fatal(err)
	}
	_ = filepath.WalkDir(s.Dir(), func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.HasPrefix(d.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", p)
		}
		return nil
	})
}

func sample[T any](t *testing.T, name string) *T {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "domain", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return &v
}

// putManifest stores bytes for every artifact of a bundle under the digest
// the manifest names, by writing the CAS file directly (the sample's bytes
// are not available).
func putManifest(t *testing.T, s *Store, b *domain.EvidenceBundle) {
	t.Helper()
	for _, a := range b.Manifest.Artifacts {
		p, err := s.casPath(a.Digest)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeAtomic(p, []byte("placeholder")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBundleImmutable(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	b := sample[domain.EvidenceBundle](t, "sample-evidence-bundle.json")
	if err := b.Validate(); err != nil {
		t.Skipf("sample bundle does not validate: %v", err)
	}
	if err := s.PutBundle(ctx, b); err == nil || !strings.Contains(err.Error(), "not in the artifact store") {
		t.Fatalf("a bundle whose artifacts are absent must be refused, got %v", err)
	}
	putManifest(t, s, b)
	if err := s.PutBundle(ctx, b); err != nil {
		t.Fatal(err)
	}
	// idempotent, even with another creation time
	b2 := *b
	b2.CreatedAt = b.CreatedAt.Add(time.Hour)
	if err := s.PutBundle(ctx, &b2); err != nil {
		t.Fatalf("re-writing the same bundle must be a no-op: %v", err)
	}
	got, err := s.GetBundle(ctx, b.ID)
	if err != nil || got.ID != b.ID || !got.CreatedAt.Equal(b.CreatedAt) {
		t.Fatalf("GetBundle = %v, %v", got, err)
	}
	// different content under the same id is refused
	b3 := *b
	b3.Producer = "someone-else@v9"
	if err := s.PutBundle(ctx, &b3); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("want immutability error, got %v", err)
	}
	if _, err := s.GetBundle(ctx, "bnd-missing"); !errors.Is(err, investigate.ErrNotFound) {
		t.Fatalf("missing bundle: %v", err)
	}
	// an invalid bundle is never written
	bad := *b
	bad.ID = "bnd-wrong"
	if err := s.PutBundle(ctx, &bad); err == nil {
		t.Fatal("invalid bundle accepted")
	}
	if _, err := s.GetBundle(ctx, "../x"); err == nil {
		t.Fatal("path-like id accepted")
	}
}

func TestInvestigationAuditOnlyGrows(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	inv := sample[domain.Investigation](t, "sample-investigation.json")
	if err := inv.Validate(); err != nil {
		t.Skipf("sample investigation does not validate: %v", err)
	}
	if err := s.PutInvestigation(ctx, inv); err != nil {
		t.Fatal(err)
	}
	// growing is fine
	grown := *inv
	grown.Audit = append([]domain.AuditEvent{}, inv.Audit...)
	grown.AppendAudit(domain.AuditEvent{At: inv.Audit[len(inv.Audit)-1].At.Add(time.Second), Stage: domain.StageHumanReview, Event: "requested", Actor: "system"})
	if err := s.PutInvestigation(ctx, &grown); err != nil {
		t.Fatalf("growing the audit trail: %v", err)
	}
	got, err := s.GetInvestigation(ctx, inv.ID)
	if err != nil || len(got.Audit) != len(grown.Audit) {
		t.Fatalf("GetInvestigation = %v, %v", got, err)
	}
	// shrinking is refused
	short := grown
	short.Audit = grown.Audit[:len(grown.Audit)-1]
	if err := s.PutInvestigation(ctx, &short); err == nil || !strings.Contains(err.Error(), "only grow") {
		t.Fatalf("shrinking the trail must be refused, got %v", err)
	}
	// rewriting history (with a re-chained, internally valid trail) is refused
	rewritten := grown
	rewritten.Audit = append([]domain.AuditEvent{}, grown.Audit...)
	rewritten.Audit[0].Detail = "rewritten"
	for i := 1; i < len(rewritten.Audit); i++ {
		rewritten.Audit[i].Prev = rewritten.Audit[i-1].Digest()
	}
	if err := s.PutInvestigation(ctx, &rewritten); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("altering history must be refused, got %v", err)
	}
	if _, err := s.GetInvestigation(ctx, "inv-missing"); !errors.Is(err, investigate.ErrNotFound) {
		t.Fatalf("missing investigation: %v", err)
	}
}
