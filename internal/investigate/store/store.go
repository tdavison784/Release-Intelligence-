// Package store implements the investigation stores of DESIGN.md §2.3: a
// content-addressed artifact store and the bundle / run record store.
//
//	<state>/investigations/
//	  cas/sha256/<hex[0:2]>/<hex>   artifact bytes (immutable; written once; digest verified on read)
//	  bundles/<bnd-id>.json         EvidenceBundle (immutable)
//	  runs/<inv-id>.json            Investigation (rewritten as it progresses; audit only grows)
//
// Every write is atomic (temp file in the same directory + rename). The
// package depends only on internal/domain and the investigate ports.
package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Store satisfies investigate.ArtifactStore and investigate.RecordStore.
type Store struct {
	dir string // <state>/investigations
}

var (
	_ investigate.ArtifactStore = (*Store)(nil)
	_ investigate.RecordStore   = (*Store)(nil)
)

// New opens (creating it if needed) the investigation store below the state
// directory root.
func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("store: root (state directory) is required")
	}
	s := &Store{dir: filepath.Join(root, "investigations")}
	for _, d := range []string{filepath.Join(s.dir, "cas", "sha256"), filepath.Join(s.dir, "bundles"), filepath.Join(s.dir, "runs")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("store: %w", err)
		}
	}
	return s, nil
}

// Dir returns the store directory (<state>/investigations).
func (s *Store) Dir() string { return s.dir }

var (
	digestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	idRE     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

func (s *Store) casPath(digest string) (string, error) {
	if !digestRE.MatchString(digest) {
		return "", fmt.Errorf("store: %q is not a sha256:<hex> digest", digest)
	}
	h := strings.TrimPrefix(digest, "sha256:")
	return filepath.Join(s.dir, "cas", "sha256", h[:2], h), nil
}

// Put stores content and returns its ref. The same bytes are stored once; the
// same spec and bytes yield the same ref.
func (s *Store) Put(ctx context.Context, spec investigate.ArtifactSpec, content []byte) (domain.ArtifactRef, error) {
	if err := ctx.Err(); err != nil {
		return domain.ArtifactRef{}, err
	}
	sum := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	ref := domain.ArtifactRef{
		Kind: spec.Kind, MediaType: spec.MediaType, Digest: digest, Size: int64(len(content)),
		Path: spec.Path, Origin: spec.Origin, URI: spec.URI, Producer: spec.Producer,
		Redacted: spec.Redacted, Tags: normTags(spec.Tags), ChangeItems: sortedUnique(spec.ChangeItems),
	}
	ref.ID = domain.ArtifactID(ref.Kind, ref.Path, ref.Digest)
	if err := ref.Validate(); err != nil {
		return domain.ArtifactRef{}, fmt.Errorf("store: %w", err)
	}
	p, err := s.casPath(digest)
	if err != nil {
		return domain.ArtifactRef{}, err
	}
	if existing, err := os.ReadFile(p); err == nil {
		if bytes.Equal(existing, content) {
			return ref, nil
		}
		// The stored file does not match its own name: repair it below.
	}
	if err := writeAtomic(p, content); err != nil {
		return domain.ArtifactRef{}, fmt.Errorf("store: %w", err)
	}
	return ref, nil
}

// Open returns the bytes of a digest, verifying them first.
func (s *Store) Open(ctx context.Context, digest string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := s.casPath(digest)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("artifact %s: %w", digest, investigate.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	sum := sha256.Sum256(b)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != digest {
		return nil, fmt.Errorf("store: artifact %s is corrupt (content hashes to %s)", digest, got)
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// Has reports whether the digest is stored (without verifying it).
func (s *Store) Has(digest string) bool {
	p, err := s.casPath(digest)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// PutBundle validates and stores a bundle. A bundle is immutable: writing it
// again is a no-op (its creation time is not identity), writing different
// content under an existing id is an error. Every manifest artifact must be
// in the artifact store.
func (s *Store) PutBundle(ctx context.Context, b *domain.EvidenceBundle) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b == nil {
		return errors.New("store: nil bundle")
	}
	if err := b.Validate(); err != nil {
		return fmt.Errorf("store: invalid bundle: %w", err)
	}
	for _, a := range b.Manifest.Artifacts {
		if !s.Has(a.Digest) {
			return fmt.Errorf("store: bundle %s lists artifact %s (%s) which is not in the artifact store", b.ID, a.ID, a.Path)
		}
	}
	p, err := s.recordPath("bundles", b.ID)
	if err != nil {
		return err
	}
	if old, err := s.GetBundle(ctx, b.ID); err == nil {
		a, b2 := *old, *b
		a.CreatedAt, b2.CreatedAt = time.Time{}, time.Time{}
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b2)
		if !bytes.Equal(ja, jb) {
			return fmt.Errorf("store: bundle %s already exists with different content (bundles are immutable)", b.ID)
		}
		return nil
	} else if !errors.Is(err, investigate.ErrNotFound) {
		return err
	}
	return writeJSON(p, b)
}

// GetBundle reads and validates a bundle.
func (s *Store) GetBundle(ctx context.Context, id string) (*domain.EvidenceBundle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := s.recordPath("bundles", id)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("bundle %s: %w", id, investigate.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	var b domain.EvidenceBundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("store: bundle %s: %w", id, err)
	}
	if b.ID != id {
		return nil, fmt.Errorf("store: file for bundle %s holds bundle %s", id, b.ID)
	}
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("store: stored bundle %s is invalid: %w", id, err)
	}
	return &b, nil
}

// PutInvestigation validates and stores (or rewrites) a run record. The
// audit trail may only grow: the stored events must be a prefix of the new
// ones, byte for byte.
func (s *Store) PutInvestigation(ctx context.Context, inv *domain.Investigation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if inv == nil {
		return errors.New("store: nil investigation")
	}
	if err := inv.Validate(); err != nil {
		return fmt.Errorf("store: invalid investigation: %w", err)
	}
	p, err := s.recordPath("runs", inv.ID)
	if err != nil {
		return err
	}
	old, err := s.GetInvestigation(ctx, inv.ID)
	switch {
	case err == nil:
		if len(inv.Audit) < len(old.Audit) {
			return fmt.Errorf("store: investigation %s: audit trail shrank (%d -> %d events); it may only grow", inv.ID, len(old.Audit), len(inv.Audit))
		}
		for i, e := range old.Audit {
			if e.Digest() != inv.Audit[i].Digest() {
				return fmt.Errorf("store: investigation %s: audit event %d was altered; the trail is append-only", inv.ID, i+1)
			}
		}
	case errors.Is(err, investigate.ErrNotFound):
	default:
		return err
	}
	return writeJSON(p, inv)
}

// GetInvestigation reads a run record.
func (s *Store) GetInvestigation(ctx context.Context, id string) (*domain.Investigation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := s.recordPath("runs", id)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("investigation %s: %w", id, investigate.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	var inv domain.Investigation
	if err := json.Unmarshal(raw, &inv); err != nil {
		return nil, fmt.Errorf("store: investigation %s: %w", id, err)
	}
	if inv.ID != id {
		return nil, fmt.Errorf("store: file for investigation %s holds investigation %s", id, inv.ID)
	}
	return &inv, nil
}

func (s *Store) recordPath(kind, id string) (string, error) {
	if !idRE.MatchString(id) {
		return "", fmt.Errorf("store: %q is not a valid record id", id)
	}
	return filepath.Join(s.dir, kind, id+".json"), nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	if err := writeAtomic(path, append(b, '\n')); err != nil {
		return fmt.Errorf("store: %w", err)
	}
	return nil
}

// writeAtomic writes data to path via a temp file in the same directory.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func normTags(in []string) []string {
	var out []string
	for _, t := range in {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			out = append(out, t)
		}
	}
	return sortedUnique(out)
}

func sortedUnique(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	m := map[string]bool{}
	var out []string
	for _, s := range in {
		if !m[s] {
			m[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
