package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Exchange file formats.
const (
	ExchangeRequestFormat  = "ri.dev/llm-exchange/request/v1"
	ExchangeResponseFormat = "ri.dev/llm-exchange/response/v1"
)

// ErrPending is returned (wrapped) by Exchange when a request was written and
// no response has been supplied yet.
var ErrPending = errors.New("llm: request written to the exchange directory; waiting for a response")

// ErrExchangeMismatch is returned (wrapped) when a response file does not
// answer the request it is filed under.
var ErrExchangeMismatch = errors.New("llm: exchange response does not match its request")

// ExchangeRequest is written to <Dir>/<hex digest>.request.json. It holds the
// complete prompt and the JSON Schema the answer must conform to, so any
// model (or a human) can produce the response.
type ExchangeRequest struct {
	Format       string  `json:"format"`
	PromptDigest string  `json:"promptDigest"`
	Request      Request `json:"request"`
	// ResponseFile is the file name the answer must be written to.
	ResponseFile string `json:"responseFile"`
	Instructions string `json:"instructions"`
}

// ExchangeResponse is read from <Dir>/<hex digest>.response.json.
type ExchangeResponse struct {
	Format string `json:"format,omitempty"`
	// PromptDigest must equal the promptDigest of the request it answers.
	PromptDigest string `json:"promptDigest"`
	// Model and ModelVersion name the model that produced the answer; both
	// are required and are recorded in provenance as given.
	Model        string `json:"model"`
	ModelVersion string `json:"modelVersion"`
	// GeneratedAt is when the answer was produced (default: the response
	// file's modification time).
	GeneratedAt time.Time `json:"generatedAt,omitzero"`
	// Exactly one of Text (the raw answer) or Output (the structured answer
	// as a JSON value) is set.
	Text   string          `json:"text,omitempty"`
	Output json.RawMessage `json:"output,omitempty"`
}

// Exchange is a Client for batch or offline operation through a directory:
// Complete writes the pending request as <hex digest>.request.json and
// returns ErrPending; once a response file <hex digest>.response.json has been
// supplied (by any model, any tool, or a human), Complete returns it. A
// response whose promptDigest does not match the request is rejected with
// ErrExchangeMismatch. The recorded model is the one named in the response
// file.
type Exchange struct {
	Dir string
}

// RequestPath and ResponsePath name the exchange files of a request.
func (x *Exchange) RequestPath(req Request) string {
	return filepath.Join(x.Dir, digestHex(PromptDigest(req))+".request.json")
}

func (x *Exchange) ResponsePath(req Request) string {
	return filepath.Join(x.Dir, digestHex(PromptDigest(req))+".response.json")
}

const exchangeInstructions = "Answer `request` with any model: `request.system` is the system prompt, `request.messages` the conversation, " +
	"and the answer must be a JSON document conforming to `request.jsonSchema`. Write the file named by `responseFile` next to this one: " +
	`{"format":"` + ExchangeResponseFormat + `","promptDigest":"<copy promptDigest>","model":"<model id that answered>",` +
	`"modelVersion":"<exact model version>","generatedAt":"<RFC 3339>","output":<the JSON answer>}` +
	" (or \"text\":\"<the answer as a string>\" instead of output). Never edit this request file: its digest identifies the prompt."

// Complete implements Client.
func (x *Exchange) Complete(ctx context.Context, req Request) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if x.Dir == "" {
		return nil, errors.New("llm: exchange directory not set")
	}
	digest := PromptDigest(req)
	respPath := x.ResponsePath(req)
	b, err := os.ReadFile(respPath)
	if err == nil {
		return x.decode(req, digest, respPath, b)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(x.Dir, 0o755); err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(ExchangeRequest{Format: ExchangeRequestFormat, PromptDigest: digest, Request: req,
		ResponseFile: filepath.Base(respPath), Instructions: exchangeInstructions}, "", "  ")
	if err != nil {
		return nil, err
	}
	reqPath := x.RequestPath(req)
	if have, err := os.ReadFile(reqPath); err != nil || !bytes.Equal(have, append(out, '\n')) {
		if err := writeFileAtomic(reqPath, append(out, '\n')); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrPending, reqPath)
}

func (x *Exchange) decode(req Request, digest, path string, b []byte) (*Response, error) {
	var r ExchangeResponse
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("llm exchange %s: %w", path, err)
	}
	if r.Format != "" && r.Format != ExchangeResponseFormat {
		return nil, fmt.Errorf("llm exchange %s: unknown format %q", path, r.Format)
	}
	if r.PromptDigest != digest {
		return nil, fmt.Errorf("%w: %s answers prompt %q, the request has %q", ErrExchangeMismatch, path, r.PromptDigest, digest)
	}
	if strings.TrimSpace(r.Model) == "" || strings.TrimSpace(r.ModelVersion) == "" {
		return nil, fmt.Errorf("llm exchange %s: model and modelVersion are required", path)
	}
	text := r.Text
	switch {
	case len(r.Output) > 0 && text != "":
		return nil, fmt.Errorf("llm exchange %s: set either text or output, not both", path)
	case len(r.Output) > 0:
		var buf bytes.Buffer
		if err := json.Compact(&buf, r.Output); err != nil {
			return nil, fmt.Errorf("llm exchange %s: output: %w", path, err)
		}
		text = buf.String()
	case text == "":
		return nil, fmt.Errorf("llm exchange %s: empty answer", path)
	}
	if req.JSONSchema != nil && !json.Valid([]byte(strings.TrimSpace(text))) {
		return nil, fmt.Errorf("llm exchange %s: structured answer is not valid JSON", path)
	}
	gen := r.GeneratedAt
	if gen.IsZero() {
		if fi, err := os.Stat(path); err == nil {
			gen = fi.ModTime()
		}
	}
	return &Response{Text: text, Model: r.Model, ModelVersion: r.ModelVersion, GeneratedAt: gen.UTC().Truncate(time.Second), Origin: OriginExchange}, nil
}
