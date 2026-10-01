package llm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testRequest(q string) Request {
	return Request{System: "be exact", Messages: []Message{{Role: "user", Content: q}},
		JSONSchema: json.RawMessage(`{"type":"object","required":["a"],"properties":{"a":{"type":"string"}}}`)}
}

func TestCacheReplaysByteForByte(t *testing.T) {
	dir := t.TempDir()
	fake := &Fake{Model: "fake-model", ModelVersion: "fake-model-2026-01", Responses: []FakeResponse{{Text: `{"a":"x"}`}}}
	clock := func() time.Time { return time.Date(2026, 10, 1, 9, 30, 15, 123, time.UTC) }
	online := &Cache{Dir: dir, Inner: fake, Clock: clock}
	req := testRequest("q1")

	first, err := online.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Model != "fake-model" || first.ModelVersion != "fake-model-2026-01" || first.Origin != OriginFake ||
		!first.GeneratedAt.Equal(time.Date(2026, 10, 1, 9, 30, 15, 0, time.UTC)) {
		t.Fatalf("first answer: %+v", first)
	}
	entry := filepath.Join(dir, strings.TrimPrefix(PromptDigest(req), "sha256:")+".json")
	stored, err := os.ReadFile(entry)
	if err != nil {
		t.Fatalf("answer not stored under the prompt digest: %v", err)
	}

	// Offline replay: no inner client, the fake must not be asked again.
	offline := &Cache{Dir: dir}
	again, err := offline.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if again.Origin != OriginCache {
		t.Fatalf("origin %q", again.Origin)
	}
	again.Origin = first.Origin
	if !reflect.DeepEqual(first, again) {
		t.Fatalf("replay differs:\n first %+v\n again %+v", first, again)
	}
	if len(fake.Requests()) != 1 {
		t.Fatalf("inner asked %d times", len(fake.Requests()))
	}
	if b, _ := os.ReadFile(entry); string(b) != string(stored) {
		t.Fatal("a replay must not rewrite the entry")
	}

	// A different prompt is a miss; offline that is ErrNotCached.
	if _, err := offline.Complete(context.Background(), testRequest("q2")); !errors.Is(err, ErrNotCached) {
		t.Fatalf("want ErrNotCached, got %v", err)
	}
	// The requested model is part of the prompt digest.
	other := req
	other.Model = "another-model"
	if _, err := offline.Complete(context.Background(), other); !errors.Is(err, ErrNotCached) {
		t.Fatalf("a request for another model must not hit: %v", err)
	}
}

func TestCacheRejectsForeignEntry(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{Dir: dir, Inner: &Fake{Responses: []FakeResponse{{Text: `{"a":"x"}`}, {Text: `{"a":"y"}`}}}}
	r1, r2 := testRequest("q1"), testRequest("q2")
	if _, err := c.Complete(context.Background(), r1); err != nil {
		t.Fatal(err)
	}
	// Copy the entry of r1 to the file name of r2: the digest check catches it.
	b, _ := os.ReadFile(filepath.Join(dir, digestHex(PromptDigest(r1))+".json"))
	if err := os.WriteFile(filepath.Join(dir, digestHex(PromptDigest(r2))+".json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Cache{Dir: dir}).Complete(context.Background(), r2); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("want digest mismatch, got %v", err)
	}
}

func TestCacheDoesNotStoreErrorsOrUnattributedAnswers(t *testing.T) {
	dir := t.TempDir()
	boom := errors.New("boom")
	c := &Cache{Dir: dir, Inner: &Fake{Responses: []FakeResponse{{Err: boom}}}}
	if _, err := c.Complete(context.Background(), testRequest("q")); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("errors must not be cached: %v", entries)
	}
	anon := &Cache{Dir: dir, Inner: clientFunc(func(Request) (*Response, error) { return &Response{Text: "{}"}, nil })}
	if _, err := anon.Complete(context.Background(), testRequest("q")); err == nil {
		t.Fatal("an answer without model and model version must be refused")
	}
}

type clientFunc func(Request) (*Response, error)

func (f clientFunc) Complete(_ context.Context, r Request) (*Response, error) { return f(r) }
