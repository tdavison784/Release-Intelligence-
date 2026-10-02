package llm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The call id travels from the exchange response through the cache and is
// replayed unchanged (a replay is the same call, not a new one).
func TestCallIDThroughExchangeAndCache(t *testing.T) {
	dir := t.TempDir()
	x := &Exchange{Dir: filepath.Join(dir, "x")}
	c := &Cache{Dir: filepath.Join(dir, "cache"), Inner: x}
	req := Request{Model: "m", Messages: []Message{{Role: "user", Content: "q"}}, JSONSchema: json.RawMessage(`{"type":"object"}`)}
	if _, err := c.Complete(context.Background(), req); !errors.Is(err, ErrPending) {
		t.Fatalf("want pending, got %v", err)
	}
	resp := ExchangeResponse{PromptDigest: PromptDigest(req), Model: "m", ModelVersion: "m-1", CallID: "session-123",
		GeneratedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Output: json.RawMessage(`{"a":1}`)}
	b, _ := json.Marshal(resp)
	if err := os.WriteFile(x.ResponsePath(req), b, 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // first from the exchange, then from the cache
		got, err := c.Complete(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if got.CallID != "session-123" {
			t.Fatalf("pass %d: call id = %q", i, got.CallID)
		}
	}
	f := &Fake{Responses: []FakeResponse{{Text: "a"}, {Text: "b"}}}
	r1, _ := f.Complete(context.Background(), req)
	r2, _ := f.Complete(context.Background(), req)
	if r1.CallID == "" || r1.CallID == r2.CallID {
		t.Fatalf("every fake call has its own id: %q %q", r1.CallID, r2.CallID)
	}
}
