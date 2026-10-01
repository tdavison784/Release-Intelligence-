package fetch

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Cache is a filesystem cache of HTTP responses. Layout:
//
//	<dir>/http/<host>/<key>.json     metadata (URL, status, headers, digest, time)
//	<dir>/http/<host>/<key>.body.gz  gzip-compressed body
//
// The cache is safe for concurrent use by multiple goroutines in one process
// (writes go through a temp file + rename).
type Cache struct {
	Dir string
}

// NewCache returns a cache rooted at dir.
func NewCache(dir string) *Cache { return &Cache{Dir: dir} }

type entry struct {
	Document
	Immutable bool   `json:"immutable"`
	Method    string `json:"method"`
	body      []byte
	dir       string
}

func cacheKey(req Request) string {
	h := sha256.New()
	h.Write([]byte(req.Method))
	h.Write([]byte{0})
	h.Write([]byte(req.URL))
	h.Write([]byte{0})
	h.Write([]byte(req.Header.Get("Accept")))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func (c *Cache) paths(key, rawURL string) (meta, body string) {
	host := "unknown"
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		host = u.Host
	}
	base := filepath.Join(c.Dir, "http", host, key)
	return base + ".json", base + ".body.gz"
}

func (c *Cache) load(key, rawURL string) (*entry, error) {
	metaPath, bodyPath := c.paths(key, rawURL)
	mb, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	var e entry
	if err := json.Unmarshal(mb, &e); err != nil {
		return nil, err
	}
	if e.Status >= 200 && e.Status < 300 && e.Method != http.MethodHead {
		f, err := os.Open(bodyPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		zr, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(zr)
		if err != nil {
			return nil, err
		}
		if domain.Digest(b) != e.Digest {
			return nil, errors.New("cache body digest mismatch")
		}
		e.body = b
	}
	return &e, nil
}

func (c *Cache) store(key string, req Request, doc *Document, status int) error {
	metaPath, bodyPath := c.paths(key, req.URL)
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		return err
	}
	e := entry{Document: *doc, Immutable: req.Immutable, Method: req.Method}
	e.Status = status
	if e.Digest == "" && doc.Body != nil {
		e.Digest = domain.Digest(doc.Body)
	}
	if status >= 200 && status < 300 && req.Method != http.MethodHead {
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		zw.Header.ModTime = time.Time{}
		if _, err := zw.Write(doc.Body); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		if err := writeAtomic(bodyPath, buf.Bytes()); err != nil {
			return err
		}
	}
	mb, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(metaPath, mb)
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
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

func (e *entry) fresh(req Request, now time.Time) bool {
	if e.Status == http.StatusNotFound || e.Status == http.StatusGone {
		ttl := req.NegativeTTL
		if ttl == 0 {
			ttl = NegativeTTL
		}
		return now.Sub(e.RetrievedAt) < ttl
	}
	if e.Immutable || req.Immutable {
		return true
	}
	ttl := req.TTL
	if ttl == 0 {
		ttl = DefaultTTL
	}
	return now.Sub(e.RetrievedAt) < ttl
}

func (e *entry) result(u string) (*Document, error) {
	if e.Status == http.StatusNotFound || e.Status == http.StatusGone {
		return nil, &Error{URL: u, Status: e.Status, Err: ErrNotFound, Detail: "cached"}
	}
	d := e.Document
	d.Body = e.body
	d.FromCache = true
	return &d, nil
}

// Clear removes all cached HTTP entries.
func (c *Cache) Clear() error {
	err := os.RemoveAll(filepath.Join(c.Dir, "http"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
