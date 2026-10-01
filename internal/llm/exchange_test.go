package llm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExchangeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	x := &Exchange{Dir: dir}
	req := testRequest("explain")

	// 1. No response yet: the request is written and the call is pending.
	_, err := x.Complete(context.Background(), req)
	if !errors.Is(err, ErrPending) {
		t.Fatalf("want ErrPending, got %v", err)
	}
	b, err := os.ReadFile(x.RequestPath(req))
	if err != nil {
		t.Fatal(err)
	}
	var wr ExchangeRequest
	if err := json.Unmarshal(b, &wr); err != nil {
		t.Fatal(err)
	}
	if wr.Format != ExchangeRequestFormat || wr.PromptDigest != PromptDigest(req) || PromptDigest(wr.Request) != wr.PromptDigest ||
		string(wr.Request.JSONSchema) == "" || wr.Request.System == "" || !strings.HasSuffix(wr.ResponseFile, ".response.json") {
		t.Fatalf("request file must carry the full prompt, the schema and its digest: %s", b)
	}

	// 2. Some model answers through the file.
	gen := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	writeResponse(t, x.ResponsePath(req), ExchangeResponse{Format: ExchangeResponseFormat, PromptDigest: wr.PromptDigest,
		Model: "batch-model", ModelVersion: "batch-model-2026-09-01", GeneratedAt: gen, Output: json.RawMessage(`{ "a": "x" }`)})
	resp, err := x.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != `{"a":"x"}` || resp.Model != "batch-model" || resp.ModelVersion != "batch-model-2026-09-01" ||
		!resp.GeneratedAt.Equal(gen) || resp.Origin != OriginExchange {
		t.Fatalf("unexpected response %+v", resp)
	}

	// 3. Through the cache, the answer is kept and replays offline without the exchange.
	cacheDir := t.TempDir()
	if _, err := (&Cache{Dir: cacheDir, Inner: x}).Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	replay, err := (&Cache{Dir: cacheDir}).Complete(context.Background(), req)
	if err != nil || replay.Model != "batch-model" || replay.ModelVersion != "batch-model-2026-09-01" || !replay.GeneratedAt.Equal(gen) {
		t.Fatalf("offline replay: %+v %v", replay, err)
	}
}

func TestExchangeRejectsMismatchedDigest(t *testing.T) {
	dir := t.TempDir()
	x := &Exchange{Dir: dir}
	req, other := testRequest("one"), testRequest("two")
	writeResponse(t, x.ResponsePath(req), ExchangeResponse{PromptDigest: PromptDigest(other), Model: "m", ModelVersion: "m-1", Text: `{"a":"x"}`})
	_, err := x.Complete(context.Background(), req)
	if !errors.Is(err, ErrExchangeMismatch) {
		t.Fatalf("want ErrExchangeMismatch, got %v", err)
	}
}

func TestExchangeRejectsIncompleteResponses(t *testing.T) {
	cases := map[string]ExchangeResponse{
		"no model":         {Text: `{"a":"x"}`, ModelVersion: "v"},
		"no model version": {Text: `{"a":"x"}`, Model: "m"},
		"empty answer":     {Model: "m", ModelVersion: "v"},
		"text and output":  {Model: "m", ModelVersion: "v", Text: `{}`, Output: json.RawMessage(`{}`)},
		"not json":         {Model: "m", ModelVersion: "v", Text: `the answer is x`},
		"unknown format":   {Format: "something/v9", Model: "m", ModelVersion: "v", Text: `{}`},
	}
	for name, r := range cases {
		t.Run(name, func(t *testing.T) {
			x := &Exchange{Dir: t.TempDir()}
			req := testRequest("q")
			r.PromptDigest = PromptDigest(req)
			writeResponse(t, x.ResponsePath(req), r)
			if _, err := x.Complete(context.Background(), req); err == nil || errors.Is(err, ErrPending) {
				t.Fatalf("want a rejection, got %v", err)
			}
		})
	}
}

func writeResponse(t *testing.T, path string, r ExchangeResponse) {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
