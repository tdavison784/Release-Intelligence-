package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// ErrNotCached is returned by a Cache without an Inner client (offline
// replay) when no answer is stored for the request.
var ErrNotCached = errors.New("llm: no cached answer for this prompt")

// CacheEntry is one stored answer: <Dir>/<hex digest>.json. It keeps the full
// request so that an entry can be audited (and its digest re-checked) on its
// own.
type CacheEntry struct {
	PromptDigest string    `json:"promptDigest"`
	Request      Request   `json:"request"`
	Model        string    `json:"model"`
	ModelVersion string    `json:"modelVersion"`
	GeneratedAt  time.Time `json:"generatedAt"`
	// Origin is where the answer originally came from (api, exchange, fake).
	Origin string `json:"origin"`
	Text   string `json:"text"`
}

// Cache is a Client that answers repeated requests from disk, keyed by
// PromptDigest. A miss is forwarded to Inner and the answer is stored, so a
// later run (including `-offline`, where Inner is nil) replays the same
// answer byte for byte: same text, same model and model version, same
// generation time. Errors are never cached.
type Cache struct {
	Dir   string
	Inner Client // nil = replay only; a miss returns ErrNotCached
	// Refresh ignores stored answers (they are re-requested and overwritten).
	Refresh bool
	// Clock stamps answers whose client did not report a generation time
	// (default time.Now).
	Clock func() time.Time
}

func (c *Cache) path(digest string) string {
	return filepath.Join(c.Dir, digestHex(digest)+".json")
}

// Lookup returns the stored entry for req, or (nil, nil) when there is none.
func (c *Cache) Lookup(req Request) (*CacheEntry, error) {
	digest := PromptDigest(req)
	b, err := os.ReadFile(c.path(digest))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var e CacheEntry
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("llm cache %s: %w", c.path(digest), err)
	}
	if e.PromptDigest != digest || PromptDigest(e.Request) != digest {
		return nil, fmt.Errorf("llm cache %s: entry does not belong to this prompt (digest mismatch)", c.path(digest))
	}
	if e.Model == "" || e.ModelVersion == "" || e.GeneratedAt.IsZero() {
		return nil, fmt.Errorf("llm cache %s: entry lacks model, model version or generation time", c.path(digest))
	}
	return &e, nil
}

// Complete implements Client.
func (c *Cache) Complete(ctx context.Context, req Request) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !c.Refresh {
		e, err := c.Lookup(req)
		if err != nil {
			return nil, err
		}
		if e != nil {
			return &Response{Text: e.Text, Model: e.Model, ModelVersion: e.ModelVersion, GeneratedAt: e.GeneratedAt, Origin: OriginCache}, nil
		}
	}
	if c.Inner == nil {
		return nil, ErrNotCached
	}
	resp, err := c.Inner.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Model == "" || resp.ModelVersion == "" {
		return nil, errors.New("llm: the answer does not report the model and model version that produced it")
	}
	out := *resp
	if out.GeneratedAt.IsZero() {
		now := time.Now
		if c.Clock != nil {
			now = c.Clock
		}
		out.GeneratedAt = now().UTC().Truncate(time.Second)
	}
	e := CacheEntry{PromptDigest: PromptDigest(req), Request: req, Model: out.Model, ModelVersion: out.ModelVersion,
		GeneratedAt: out.GeneratedAt.UTC(), Origin: out.Origin, Text: out.Text}
	if err := c.store(e); err != nil {
		return nil, err
	}
	// Return exactly what a replay will return.
	out.GeneratedAt = e.GeneratedAt
	return &out, nil
}

func (c *Cache) store(e CacheEntry) error {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(c.path(e.PromptDigest), append(b, '\n'))
}

// writeFileAtomic writes via a temporary file and a rename, so a reader never
// sees a partial file.
func writeFileAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}
