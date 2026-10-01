// Package llm defines the narrow interface the system uses to talk to a
// language model. LLMs are used only for discovery, ambiguous source
// resolution and optional enrichment — never in the deterministic ingestion
// path. Every call is described by a Request whose digest is recorded as
// provenance on anything derived from the response.
package llm

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Message is one conversational turn.
type Message struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// Request is a model invocation.
type Request struct {
	Model     string    `json:"model,omitempty"` // empty = client default
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"maxTokens,omitempty"`
	// JSONSchema, when set, asks the model to answer with a JSON document
	// conforming to the schema (structured output).
	JSONSchema json.RawMessage `json:"jsonSchema,omitempty"`
}

// Response is a model answer.
type Response struct {
	Text  string `json:"text"`
	Model string `json:"model"` // model that actually served the request
	// ModelVersion is the most precise version identifier reported with the
	// answer. Providers that report a single pinned model id (the Anthropic
	// Messages API) repeat it here; it is never filled in from the request.
	ModelVersion string `json:"modelVersion,omitempty"`
	// GeneratedAt is when the answer was produced, when the client knows it
	// (the cache and the exchange record it; zero otherwise).
	GeneratedAt time.Time `json:"generatedAt,omitzero"`
	// Origin says where the answer came from: OriginAPI, OriginCache,
	// OriginExchange or OriginFake.
	Origin string `json:"origin,omitempty"`
}

// Response origins.
const (
	OriginAPI      = "api"
	OriginCache    = "cache"
	OriginExchange = "exchange"
	OriginFake     = "fake"
)

// Client is implemented by model providers and by test fakes.
type Client interface {
	Complete(ctx context.Context, req Request) (*Response, error)
}

// PromptDigest returns a stable digest of the request, recorded in provenance.
func PromptDigest(req Request) string {
	b, _ := json.Marshal(req)
	return domain.Digest(b)
}

// digestHex returns the hex part of a PromptDigest ("sha256:<hex>"), used
// as a file name by the cache and the exchange.
func digestHex(digest string) string {
	return strings.TrimPrefix(digest, "sha256:")
}
