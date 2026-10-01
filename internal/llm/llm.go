// Package llm defines the narrow interface the system uses to talk to a
// language model. LLMs are used only for discovery, ambiguous source
// resolution and optional enrichment — never in the deterministic ingestion
// path. Every call is described by a Request whose digest is recorded as
// provenance on anything derived from the response.
package llm

import (
	"context"
	"encoding/json"

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
}

// Client is implemented by model providers and by test fakes.
type Client interface {
	Complete(ctx context.Context, req Request) (*Response, error)
}

// PromptDigest returns a stable digest of the request, recorded in provenance.
func PromptDigest(req Request) string {
	b, _ := json.Marshal(req)
	return domain.Digest(b)
}
