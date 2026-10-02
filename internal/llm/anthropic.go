package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Anthropic Messages API constants.
const (
	// DefaultAnthropicModel is the Sonnet-class model used when neither the
	// client nor the request names one.
	DefaultAnthropicModel = "claude-sonnet-5-5"
	// DefaultAnthropicBaseURL is the public Claude API endpoint.
	DefaultAnthropicBaseURL = "https://api.anthropic.com"
	// AnthropicVersion is sent as the anthropic-version header.
	AnthropicVersion = "2023-06-01"
	// DefaultAnthropicMaxTokens bounds a non-streaming answer. Thinking tokens
	// count against it, so it is deliberately not tiny.
	DefaultAnthropicMaxTokens = 16000

	// serverFallbackBeta gates `"fallbacks": "default"`: when the model
	// declines a request, the API re-serves it on Anthropic's recommended
	// fallback model inside the same call. Response.Model then names the
	// model that actually answered, which is what provenance records.
	serverFallbackBeta = "server-side-fallback-2026-07-01"
)

// ErrRefusal is matched (errors.Is) by a *RefusalError.
var ErrRefusal = errors.New("llm: the model declined the request")

// ErrTruncated is returned when the answer stopped at max_tokens; a
// structured (JSON) answer is then incomplete and must not be used.
var ErrTruncated = errors.New("llm: answer truncated at max_tokens")

// RefusalError reports stop_reason "refusal".
type RefusalError struct {
	Model       string
	Category    string
	Explanation string
}

func (e *RefusalError) Error() string {
	s := "llm: " + e.Model + " declined the request"
	if e.Category != "" {
		s += " (category " + e.Category + ")"
	}
	if e.Explanation != "" {
		s += ": " + e.Explanation
	}
	return s
}

// Is makes errors.Is(err, ErrRefusal) work.
func (e *RefusalError) Is(target error) bool { return target == ErrRefusal }

// APIError is a non-2xx answer of the Messages API. Its body has the shape
// {"type":"error","error":{"type":"...","message":"..."},"request_id":"..."}.
type APIError struct {
	StatusCode int
	Type       string // e.g. "invalid_request_error", "rate_limit_error", "overloaded_error"
	Message    string
	RequestID  string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("anthropic: HTTP %d", e.StatusCode)
	if e.Type != "" {
		msg += " " + e.Type
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if e.RequestID != "" {
		msg += " (request " + e.RequestID + ")"
	}
	return msg
}

// Retryable reports whether the request may succeed when repeated
// (timeouts, conflicts, rate limits, overload and server errors).
func (e *APIError) Retryable() bool {
	switch {
	case e.StatusCode == http.StatusRequestTimeout, e.StatusCode == http.StatusConflict,
		e.StatusCode == http.StatusTooManyRequests, e.StatusCode >= 500:
		return true
	}
	return false
}

// Anthropic is a Client for the Anthropic Messages API (POST /v1/messages)
// implemented with net/http. Construct it with NewAnthropic; callers read the
// API key from ANTHROPIC_API_KEY themselves.
type Anthropic struct {
	APIKey  string
	BaseURL string // default DefaultAnthropicBaseURL; tests point it at httptest
	Model   string // default DefaultAnthropicModel; Request.Model overrides
	// MaxTokens is used when Request.MaxTokens is 0.
	MaxTokens int
	// Effort, when set, is sent as output_config.effort ("low" … "max").
	Effort string
	// Thinking is sent as the thinking parameter ("enabled"/"disabled"; ""
	// omits it). Disabling it keeps reasoning-first gateway models (GLM
	// served as Anthropic-compatible) from spending the answer budget on
	// thinking blocks.
	Thinking string
	// ServerFallback sends `"fallbacks": "default"` (beta header
	// server-side-fallback-2026-07-01) so that a declined request is re-served
	// by a fallback model. Only the first-party Claude API accepts it; turn it
	// off when BaseURL points at a gateway that rejects it.
	ServerFallback bool
	// MaxRetries is the number of retries for retryable failures (default 2).
	MaxRetries int
	HTTPClient *http.Client

	sleep func(context.Context, time.Duration) error
}

// NewAnthropic returns a client with the documented defaults.
func NewAnthropic(apiKey string) *Anthropic {
	return &Anthropic{
		APIKey:         apiKey,
		BaseURL:        DefaultAnthropicBaseURL,
		Model:          DefaultAnthropicModel,
		MaxTokens:      DefaultAnthropicMaxTokens,
		ServerFallback: true,
		MaxRetries:     2,
		HTTPClient:     &http.Client{Timeout: 10 * time.Minute},
	}
}

// wire types -----------------------------------------------------------------

type anthropicRequest struct {
	Model        string             `json:"model"`
	MaxTokens    int                `json:"max_tokens"`
	System       string             `json:"system,omitempty"`
	Messages     []anthropicMessage `json:"messages"`
	OutputConfig *outputConfig      `json:"output_config,omitempty"`
	Fallbacks    string             `json:"fallbacks,omitempty"`
	// Thinking explicitly enables/disables extended thinking ("enabled"/
	// "disabled"; "" omits the field). Reasoning-first models served through
	// Anthropic-compatible gateways otherwise spend the token budget on
	// thinking before answering.
	Thinking *anthropicThinking `json:"thinking,omitempty"`
}

type anthropicThinking struct {
	Type string `json:"type"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type outputConfig struct {
	Effort string        `json:"effort,omitempty"`
	Format *outputFormat `json:"format,omitempty"`
}

type outputFormat struct {
	Type   string          `json:"type"` // "json_schema"
	Schema json.RawMessage `json:"schema"`
}

type anthropicResponse struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopDetails *struct {
		Category    string `json:"category"`
		Explanation string `json:"explanation"`
	} `json:"stop_details"`
}

type anthropicErrorBody struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

// Complete implements Client.
func (a *Anthropic) Complete(ctx context.Context, req Request) (*Response, error) {
	if strings.TrimSpace(a.APIKey) == "" {
		return nil, errors.New("anthropic: no API key configured")
	}
	if len(req.Messages) == 0 {
		return nil, errors.New("anthropic: request has no messages")
	}
	body, err := a.buildBody(req)
	if err != nil {
		return nil, err
	}
	retries := a.MaxRetries
	if retries < 0 {
		retries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<(attempt-1)) * time.Second
			var apiErr *APIError
			if errors.As(lastErr, &apiErr) && apiErr.RetryAfter > 0 {
				wait = apiErr.RetryAfter
			}
			if err := a.wait(ctx, wait); err != nil {
				return nil, err
			}
		}
		resp, err := a.once(ctx, body, req.JSONSchema != nil)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		var apiErr *APIError
		switch {
		case errors.As(err, &apiErr):
			if !apiErr.Retryable() {
				return nil, err
			}
		case errors.Is(err, ErrRefusal), errors.Is(err, ErrTruncated), ctx.Err() != nil:
			return nil, err
		case isTransport(err):
			// network failure: retry
		default:
			return nil, err
		}
	}
	return nil, lastErr
}

func (a *Anthropic) buildBody(req Request) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = a.Model
	}
	if model == "" {
		model = DefaultAnthropicModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = a.MaxTokens
	}
	if maxTokens <= 0 {
		maxTokens = DefaultAnthropicMaxTokens
	}
	ar := anthropicRequest{Model: model, MaxTokens: maxTokens, System: req.System}
	for _, m := range req.Messages {
		ar.Messages = append(ar.Messages, anthropicMessage{Role: m.Role, Content: m.Content})
	}
	if req.JSONSchema != nil || a.Effort != "" {
		ar.OutputConfig = &outputConfig{Effort: a.Effort}
		if req.JSONSchema != nil {
			if !json.Valid(req.JSONSchema) {
				return nil, errors.New("anthropic: request JSONSchema is not valid JSON")
			}
			ar.OutputConfig.Format = &outputFormat{Type: "json_schema", Schema: req.JSONSchema}
		}
	}
	if a.ServerFallback {
		ar.Fallbacks = "default"
	}
	if a.Thinking != "" {
		ar.Thinking = &anthropicThinking{Type: a.Thinking}
	}
	return json.Marshal(ar)
}

type transportError struct{ err error }

func (e *transportError) Error() string { return "anthropic: " + e.err.Error() }
func (e *transportError) Unwrap() error { return e.err }

func isTransport(err error) bool {
	var te *transportError
	return errors.As(err, &te)
}

func (a *Anthropic) once(ctx context.Context, body []byte, structured bool) (*Response, error) {
	base := strings.TrimRight(a.BaseURL, "/")
	if base == "" {
		base = DefaultAnthropicBaseURL
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("content-type", "application/json")
	hreq.Header.Set("x-api-key", a.APIKey)
	hreq.Header.Set("anthropic-version", AnthropicVersion)
	if a.ServerFallback {
		hreq.Header.Set("anthropic-beta", serverFallbackBeta)
	}
	hc := a.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	hresp, err := hc.Do(hreq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &transportError{err}
	}
	defer hresp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(hresp.Body, 32<<20))
	if err != nil {
		return nil, &transportError{err}
	}
	if hresp.StatusCode < 200 || hresp.StatusCode > 299 {
		return nil, parseAPIError(hresp, raw)
	}
	var ar anthropicResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		return nil, fmt.Errorf("anthropic: decode response: %w", err)
	}
	switch ar.StopReason {
	case "refusal":
		re := &RefusalError{Model: ar.Model}
		if ar.StopDetails != nil {
			re.Category, re.Explanation = ar.StopDetails.Category, ar.StopDetails.Explanation
		}
		return nil, re
	case "max_tokens":
		if structured {
			return nil, ErrTruncated
		}
	}
	var text strings.Builder
	for _, c := range ar.Content {
		// thinking / fallback / other blocks are skipped: only the visible
		// answer is returned.
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	// The Messages API reports one model identifier, which names the pinned
	// model that answered (after any server-side fallback); it is recorded
	// as both the model and its version.
	out := &Response{Text: text.String(), Model: ar.Model, ModelVersion: ar.Model, Origin: OriginAPI, CallID: ar.ID}
	if structured {
		// Models behind Anthropic-compatible gateways may ignore the
		// structured-output config and answer with a fenced or prose-wrapped
		// JSON document; the caller's schema validation still applies to
		// whatever is extracted.
		out.Text = normalizeStructuredJSON(out.Text)
		if !json.Valid([]byte(strings.TrimSpace(out.Text))) {
			return nil, fmt.Errorf("anthropic: structured answer is not valid JSON (stop_reason %q)", ar.StopReason)
		}
	}
	return out, nil
}

// normalizeStructuredJSON returns the outermost JSON document in s (a
// ```json fence, or the first balanced top-level object/array) when s itself
// is not already valid JSON; otherwise it returns s unchanged, leaving the
// caller to report the honest "not valid JSON" failure.
func normalizeStructuredJSON(s string) string {
	t := strings.TrimSpace(s)
	if json.Valid([]byte(t)) {
		return s
	}
	if cand := extractFencedJSON(t); json.Valid([]byte(cand)) {
		return cand
	}
	if cand := extractBalancedJSON(t); json.Valid([]byte(cand)) {
		return cand
	}
	return s
}

func extractFencedJSON(s string) string {
	for _, marker := range []string{"```json", "```JSON", "```"} {
		i := strings.Index(s, marker)
		if i < 0 {
			continue
		}
		rest := s[i+len(marker):]
		if j := strings.Index(rest, "```"); j >= 0 {
			return strings.TrimSpace(rest[:j])
		}
	}
	return ""
}

func extractBalancedJSON(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != '{' && s[i] != '[' {
			continue
		}
		open, depth := s[i], 0
		close := byte('}')
		if open == '[' {
			close = ']'
		}
		inStr, esc := false, false
		for j := i; j < len(s); j++ {
			c := s[j]
			if inStr {
				if esc {
					esc = false
				} else if c == '\\' {
					esc = true
				} else if c == '"' {
					inStr = false
				}
				continue
			}
			switch c {
			case '"':
				inStr = true
			case open:
				depth++
			case close:
				depth--
				if depth == 0 {
					return strings.TrimSpace(s[i : j+1])
				}
			}
		}
		return ""
	}
	return ""
}

func parseAPIError(hresp *http.Response, raw []byte) error {
	e := &APIError{StatusCode: hresp.StatusCode, RequestID: hresp.Header.Get("request-id")}
	var body anthropicErrorBody
	if json.Unmarshal(raw, &body) == nil && body.Error.Type != "" {
		e.Type, e.Message = body.Error.Type, body.Error.Message
		if body.RequestID != "" {
			e.RequestID = body.RequestID
		}
	} else {
		e.Message = strings.TrimSpace(string(raw))
		if len(e.Message) > 300 {
			e.Message = e.Message[:300] + "…"
		}
	}
	if ra := hresp.Header.Get("retry-after"); ra != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(ra)); err == nil && secs >= 0 {
			e.RetryAfter = time.Duration(secs) * time.Second
		}
	}
	return e
}

func (a *Anthropic) wait(ctx context.Context, d time.Duration) error {
	if a.sleep != nil {
		return a.sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
