package render

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Cache is a content-addressed render cache (R14): one directory per cache
// key (the digest over every provenance field that determines the output:
// tool + version, artifact digest, values digests, overrides, platform
// inputs, command options). A hit returns the recorded result, so a render
// is reproducible from its recorded inputs and never re-run needlessly.
//
//	<dir>/<key>/result.json   Result (status, failure, provenance, nondeterministic paths)
//	<dir>/<key>/output.yaml   the rendered stream (successful renders)
type Cache struct {
	Dir string
}

// NewCache returns a cache rooted at dir ("" disables caching).
func NewCache(dir string) *Cache {
	if dir == "" {
		return nil
	}
	return &Cache{Dir: dir}
}

// Get returns the cached result for key.
func (c *Cache) Get(key string) (*Result, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	dir := filepath.Join(c.Dir, key)
	b, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		return nil, false
	}
	var r Result
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, false
	}
	if r.Status == StatusSucceeded {
		out, err := os.ReadFile(filepath.Join(dir, "output.yaml"))
		if err != nil {
			return nil, false
		}
		objs, err := ParseObjects(out)
		if err != nil {
			return nil, false
		}
		r.Output, r.Objects = out, objs
	}
	return &r, true
}

// Put stores a result (best effort: a cache write failure never fails a render).
func (c *Cache) Put(r *Result) {
	if c == nil || r == nil || r.Provenance.CacheKey == "" {
		return
	}
	dir := filepath.Join(c.Dir, r.Provenance.CacheKey)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	if r.Status == StatusSucceeded {
		if err := writeAtomic(filepath.Join(dir, "output.yaml"), r.Output); err != nil {
			return
		}
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return
	}
	_ = writeAtomic(filepath.Join(dir, "result.json"), b)
}

func writeAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
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
