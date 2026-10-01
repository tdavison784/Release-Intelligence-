package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *Anthropic {
	c := NewAnthropic("test-key")
	c.BaseURL = srv.URL
	c.HTTPClient = srv.Client()
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestAnthropicRequestShape(t *testing.T) {
	var got map[string]any
	var hdr http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		hdr = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("content-type", "application/json")
		io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5-5",
			"content":[{"type":"thinking","thinking":""},{"type":"text","text":"{\"answer\":"},{"type":"text","text":"42}"}],
			"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	schema := json.RawMessage(`{"type":"object","properties":{"answer":{"type":"integer"}},"required":["answer"],"additionalProperties":false}`)
	resp, err := c.Complete(context.Background(), Request{
		System:     "be terse",
		Messages:   []Message{{Role: "user", Content: "q"}},
		JSONSchema: schema,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != `{"answer":42}` || resp.Model != "claude-sonnet-5-5" {
		t.Fatalf("unexpected response %+v", resp)
	}
	if hdr.Get("x-api-key") != "test-key" || hdr.Get("anthropic-version") != AnthropicVersion {
		t.Fatalf("missing auth/version headers: %v", hdr)
	}
	if hdr.Get("anthropic-beta") != serverFallbackBeta {
		t.Fatalf("expected fallback beta header, got %q", hdr.Get("anthropic-beta"))
	}
	if got["model"] != DefaultAnthropicModel || got["system"] != "be terse" || got["fallbacks"] != "default" {
		t.Fatalf("unexpected body %v", got)
	}
	if int(got["max_tokens"].(float64)) != DefaultAnthropicMaxTokens {
		t.Fatalf("max_tokens = %v", got["max_tokens"])
	}
	oc := got["output_config"].(map[string]any)
	format := oc["format"].(map[string]any)
	if format["type"] != "json_schema" || format["schema"].(map[string]any)["type"] != "object" {
		t.Fatalf("unexpected output_config %v", oc)
	}
}

func TestAnthropicNoFallbackWhenDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "fallbacks") || r.Header.Get("anthropic-beta") != "" {
			t.Errorf("fallback sent although disabled: %s", b)
		}
		io.WriteString(w, `{"model":"m","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn"}`)
	}))
	defer srv.Close()
	c := newTestClient(srv)
	c.ServerFallback = false
	c.Model = "custom-model"
	resp, err := c.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}})
	if err != nil || resp.Text != "hi" {
		t.Fatalf("got %v %v", resp, err)
	}
}

func TestAnthropicErrorsAndRetries(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("retry-after", "1")
			w.WriteHeader(529)
			io.WriteString(w, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"},"request_id":"req_1"}`)
			return
		}
		io.WriteString(w, `{"model":"m","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`)
	}))
	defer srv.Close()
	c := newTestClient(srv)
	resp, err := c.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}})
	if err != nil || resp.Text != "ok" || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected retry then success, got %v %v calls=%d", resp, err, calls)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"bad schema"},"request_id":"req_2"}`)
	}))
	defer bad.Close()
	c = newTestClient(bad)
	_, err = c.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 || apiErr.Type != "invalid_request_error" || apiErr.RequestID != "req_2" || apiErr.Retryable() {
		t.Fatalf("unexpected error %#v", err)
	}
}

func TestAnthropicRefusalAndTruncation(t *testing.T) {
	answer := `{"model":"m","content":[],"stop_reason":"refusal","stop_details":{"type":"refusal","category":"cyber","explanation":"no"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, answer) }))
	defer srv.Close()
	c := newTestClient(srv)
	_, err := c.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}})
	var re *RefusalError
	if !errors.Is(err, ErrRefusal) || !errors.As(err, &re) || re.Category != "cyber" {
		t.Fatalf("expected refusal, got %v", err)
	}
	answer = `{"model":"m","content":[{"type":"text","text":"{\"a\":"}],"stop_reason":"max_tokens"}`
	_, err = c.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}, JSONSchema: json.RawMessage(`{"type":"object"}`)})
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("expected truncation error, got %v", err)
	}
}

func TestAnthropicRequiresKey(t *testing.T) {
	if _, err := NewAnthropic("").Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}}); err == nil {
		t.Fatal("expected error without API key")
	}
}

func TestFakeScriptsAndRecords(t *testing.T) {
	f := &Fake{Responses: []FakeResponse{{Text: "a"}, {Err: errors.New("boom")}}}
	r, err := f.Complete(context.Background(), Request{System: "s1"})
	if err != nil || r.Text != "a" || r.Model != "fake-model" {
		t.Fatalf("got %v %v", r, err)
	}
	if _, err := f.Complete(context.Background(), Request{System: "s2"}); err == nil {
		t.Fatal("expected scripted error")
	}
	if _, err := f.Complete(context.Background(), Request{}); !errors.Is(err, ErrFakeExhausted) {
		t.Fatalf("expected exhaustion, got %v", err)
	}
	if reqs := f.Requests(); len(reqs) != 3 || reqs[1].System != "s2" {
		t.Fatalf("requests not recorded: %+v", reqs)
	}
	g := &Fake{Model: "m", Respond: func(req Request) (string, error) { return req.System + "!", nil }}
	if r, _ := g.Complete(context.Background(), Request{System: "x"}); r.Text != "x!" || r.Model != "m" {
		t.Fatalf("respond func not used: %+v", r)
	}
	if PromptDigest(Request{System: "a"}) == PromptDigest(Request{System: "b"}) {
		t.Fatal("prompt digest must depend on the request")
	}
}
