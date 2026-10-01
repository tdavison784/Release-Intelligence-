package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheReadThroughAndOffline(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		switch r.URL.Path {
		case "/doc":
			w.Header().Set("Content-Type", "text/markdown")
			w.Write([]byte("# hello"))
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cache := NewCache(t.TempDir())
	c := NewHTTPClient(cache, ModeOnline)
	ctx := context.Background()

	d, err := Get(ctx, c, srv.URL+"/doc", true)
	if err != nil || string(d.Body) != "# hello" || d.FromCache {
		t.Fatalf("first fetch: %v %+v", err, d)
	}
	d, err = Get(ctx, c, srv.URL+"/doc", true)
	if err != nil || !d.FromCache || string(d.Body) != "# hello" {
		t.Fatalf("second fetch should hit cache: %v", err)
	}
	if _, err := Get(ctx, c, srv.URL+"/missing", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
	if _, err := Get(ctx, c, srv.URL+"/missing", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want cached not found, got %v", err)
	}
	if _, err := Get(ctx, c, srv.URL+"/forbidden", false); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want unavailable, got %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Fatalf("expected 3 network hits, got %d", got)
	}

	off := NewHTTPClient(cache, ModeOffline)
	if d, err := Get(ctx, off, srv.URL+"/doc", false); err != nil || string(d.Body) != "# hello" {
		t.Fatalf("offline cached: %v", err)
	}
	if _, err := Get(ctx, off, srv.URL+"/other", false); !errors.Is(err, ErrOffline) {
		t.Fatalf("offline miss: %v", err)
	}
	if StateFor(&Error{Err: ErrOffline}) != "unavailable" {
		t.Fatal("state mapping")
	}
}

func TestNoStoreAndNegativeTTL(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.URL.Path == "/token" {
			w.Write([]byte(`{"token":"secret"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	cache := NewCache(t.TempDir())
	c := NewHTTPClient(cache, ModeOnline)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := c.Do(ctx, Request{URL: srv.URL + "/token", NoStore: true}); err != nil {
			t.Fatal(err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("NoStore requests must not be cached; hits=%d", got)
	}
	if _, err := NewHTTPClient(cache, ModeOffline).Do(ctx, Request{URL: srv.URL + "/token"}); !errors.Is(err, ErrOffline) {
		t.Fatalf("token must not be persisted, got %v", err)
	}
	// negative cache with a tiny TTL expires immediately
	req := Request{URL: srv.URL + "/missing", NegativeTTL: time.Nanosecond}
	c.Do(ctx, req)
	c.Do(ctx, req)
	if got := atomic.LoadInt32(&hits); got != 4 {
		t.Fatalf("expired negative entry should be refetched; hits=%d", got)
	}
}
