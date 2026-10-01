package gitsrc

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// cacheVersion is bumped when the layout of cached results changes; entries
// of another version are ignored.
const cacheVersion = 1

// entry is one cached result. It is stored as gzip-compressed JSON under
// <root>/<kind>/... so it can be inspected with zcat.
type entry struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	// Repo is the clone URL the result was obtained from.
	Repo string `json:"repo"`
	// Request describes the request in human-readable form.
	Request string `json:"request"`
	// Immutable results never expire.
	Immutable   bool            `json:"immutable"`
	RetrievedAt time.Time       `json:"retrievedAt"`
	Payload     json.RawMessage `json:"payload"`
}

// fresh reports whether the entry may still be served online.
func (e *entry) fresh(now time.Time, ttl time.Duration) bool {
	return e.Immutable || now.Sub(e.RetrievedAt) < ttl
}

// readEntry loads an entry; ok is false when it is missing, unreadable or of
// another cache version (all of which just mean "not cached").
func readEntry(path string) (e *entry, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, false
	}
	defer zr.Close()
	b, err := io.ReadAll(zr)
	if err != nil {
		return nil, false
	}
	e = &entry{}
	if err := json.Unmarshal(b, e); err != nil || e.Version != cacheVersion {
		return nil, false
	}
	return e, true
}

// writeEntry stores an entry atomically (temp file + rename).
func writeEntry(path string, e *entry) error {
	e.Version = cacheVersion
	e.RetrievedAt = e.RetrievedAt.UTC()
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	zw.Header.ModTime = time.Time{} // keep the file content deterministic
	if _, err := zw.Write(raw); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// requestKey hashes the parts of a request into a short file name component.
func requestKey(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:20]
}

// keyedMutex serialises work per key (one repository directory) within the
// process. Operations of different processes on the same cache directory are
// not coordinated beyond what git's own locking provides.
type keyedMutex struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

// lock acquires the mutex of key and returns the function releasing it.
func (k *keyedMutex) lock(key string) func() {
	k.mu.Lock()
	if k.m == nil {
		k.m = map[string]*sync.Mutex{}
	}
	m, ok := k.m[key]
	if !ok {
		m = &sync.Mutex{}
		k.m[key] = m
	}
	k.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// exists reports whether a path exists.
func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
