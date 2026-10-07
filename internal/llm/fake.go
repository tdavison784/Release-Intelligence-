package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrFakeExhausted is returned by Fake when no scripted response is left.
var ErrFakeExhausted = errors.New("llm: fake has no scripted response left")

// FakeResponse is one scripted answer.
type FakeResponse struct {
	Text string
	Err  error
}

// Fake is a scripted Client for tests. It records every request. Respond,
// when set, answers each request; otherwise Responses are consumed in order.
type Fake struct {
	// Model is reported as the serving model (default "fake-model").
	Model string
	// ModelVersion is reported as the model version (default: Model).
	ModelVersion string
	Responses    []FakeResponse
	Respond      func(req Request) (string, error)

	mu       sync.Mutex
	requests []Request
	next     int
	calls    int
}

// Complete implements Client.
func (f *Fake) Complete(ctx context.Context, req Request) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	f.calls++
	callID := fmt.Sprintf("fake-call-%d", f.calls) // every Complete is a separate call
	model := f.Model
	if model == "" {
		model = "fake-model"
	}
	version := f.ModelVersion
	if version == "" {
		version = model
	}
	if f.Respond != nil {
		text, err := f.Respond(req)
		if err != nil {
			return nil, err
		}
		return &Response{Text: text, Model: model, ModelVersion: version, Origin: OriginFake, CallID: callID}, nil
	}
	if f.next >= len(f.Responses) {
		return nil, ErrFakeExhausted
	}
	r := f.Responses[f.next]
	f.next++
	if r.Err != nil {
		return nil, r.Err
	}
	return &Response{Text: r.Text, Model: model, ModelVersion: version, Origin: OriginFake, CallID: callID}, nil
}

// Requests returns a copy of the recorded requests.
func (f *Fake) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.requests...)
}
