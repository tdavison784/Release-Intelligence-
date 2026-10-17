package run

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Workspace holds what stages hand to each other that is not (yet) part of
// the run record: the intermediate change/snapshot results, per-key memos
// and the audit events of stages that ran before the record exists. Payloads
// between stages carry only the digests it returns (TEMPORAL.md: ~2 MB cap).
//
// It is scratch, not evidence: everything in it is derived from the stores
// and recomputable; the bundle, the artifacts and the run record are the
// reproducible truth.
type Workspace interface {
	// Put stores v (canonical JSON) and returns its sha256:<hex> digest.
	Put(v any) (string, error)
	// Get decodes the value stored under digest; investigate.ErrNotFound when absent.
	Get(digest string, v any) error
	// Memo returns the value saved under key; ok is false when none.
	Memo(key string, v any) (ok bool, err error)
	// SetMemo saves v under key, replacing any earlier value.
	SetMemo(key string, v any) error
}

func encode(v any) ([]byte, string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return raw, "sha256:" + hex.EncodeToString(sum[:]), nil
}

// MemWorkspace is an in-memory Workspace (tests, throwaway runs).
type MemWorkspace struct {
	mu    sync.Mutex
	blobs map[string][]byte
	memos map[string][]byte
}

// NewMemWorkspace returns an empty in-memory workspace.
func NewMemWorkspace() *MemWorkspace {
	return &MemWorkspace{blobs: map[string][]byte{}, memos: map[string][]byte{}}
}

func (w *MemWorkspace) Put(v any) (string, error) {
	raw, d, err := encode(v)
	if err != nil {
		return "", err
	}
	w.mu.Lock()
	w.blobs[d] = raw
	w.mu.Unlock()
	return d, nil
}

func (w *MemWorkspace) Get(d string, v any) error {
	w.mu.Lock()
	raw, ok := w.blobs[d]
	w.mu.Unlock()
	if !ok {
		return fmt.Errorf("workspace blob %s: %w", d, investigate.ErrNotFound)
	}
	return json.Unmarshal(raw, v)
}

func (w *MemWorkspace) Memo(key string, v any) (bool, error) {
	w.mu.Lock()
	raw, ok := w.memos[key]
	w.mu.Unlock()
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(raw, v)
}

func (w *MemWorkspace) SetMemo(key string, v any) error {
	raw, _, err := encode(v)
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.memos[key] = raw
	w.mu.Unlock()
	return nil
}

// DirWorkspace is a Workspace on disk, shared by every worker of one state
// directory: <dir>/blobs/<hex> and <dir>/memos/<sha256(key)>.
type DirWorkspace struct{ dir string }

// NewDirWorkspace opens (creating) a workspace under dir, conventionally
// <state>/investigations/work.
func NewDirWorkspace(dir string) (*DirWorkspace, error) {
	for _, sub := range []string{"blobs", "memos"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("workspace: %w", err)
		}
	}
	return &DirWorkspace{dir: dir}, nil
}

func writeFileAtomic(path string, raw []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(raw)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (w *DirWorkspace) Put(v any) (string, error) {
	raw, d, err := encode(v)
	if err != nil {
		return "", err
	}
	return d, writeFileAtomic(filepath.Join(w.dir, "blobs", d[len("sha256:"):]), raw)
}

func (w *DirWorkspace) Get(d string, v any) error {
	if len(d) != len("sha256:")+64 {
		return fmt.Errorf("workspace: %q is not a digest: %w", d, investigate.ErrNotFound)
	}
	raw, err := os.ReadFile(filepath.Join(w.dir, "blobs", d[len("sha256:"):]))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("workspace blob %s: %w", d, investigate.ErrNotFound)
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func (w *DirWorkspace) memoPath(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(w.dir, "memos", hex.EncodeToString(sum[:]))
}

func (w *DirWorkspace) Memo(key string, v any) (bool, error) {
	raw, err := os.ReadFile(w.memoPath(key))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(raw, v)
}

func (w *DirWorkspace) SetMemo(key string, v any) error {
	raw, _, err := encode(v)
	if err != nil {
		return err
	}
	return writeFileAtomic(w.memoPath(key), raw)
}
