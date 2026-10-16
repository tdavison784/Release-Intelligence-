package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// Driver names (investigate.AgentConfig.Driver).
const (
	DriverClaudeCLI = "claude-cli"
	DriverExchange  = "exchange"
	DriverReplay    = "replay"
	DriverScripted  = "scripted"
)

// session is what drivers need from the tool surface beyond investigate.Tools.
type session interface {
	ToolHandler
	SetCall(CallInfo)
	Submitted() bool
	Exhausted() string
	RecordEnvelope(driver string, dr investigate.DriverResult, envelope json.RawMessage)
}

func asSession(tools investigate.Tools) (session, error) {
	s, ok := tools.(session)
	if !ok {
		return nil, fmt.Errorf("agent: driver needs the agent package's tool session, got %T", tools)
	}
	return s, nil
}

// stopAfter maps a tool error to the stop reason that ends a driven run
// (budget exhaustion ends it; other errors are returned to the model).
func stopAfter(err error, s session) (string, bool) {
	if errors.Is(err, investigate.ErrBudgetExhausted) {
		if e := s.Exhausted(); e != "" {
			return e, true
		}
		return "budget:exhausted", true
	}
	return "", false
}

// --- scripted (tests) ---------------------------------------------------------------------

// Step is one scripted tool call. Args is marshalled to JSON unless it is a
// json.RawMessage, []byte or string, which are sent verbatim (so tests can
// send malformed arguments).
type Step struct {
	Call string // call id; default "scripted-call-1"
	Tool string
	Args any
}

// StepResult is what one step returned.
type StepResult struct {
	Tool   string
	Result json.RawMessage
	Err    error
}

// Scripted plays a fixed list of tool calls (tests). Results are kept for
// inspection.
type Scripted struct {
	Model, ModelVersion, Provider string
	Steps                         []Step
	Turns, Tokens                 int
	Stop                          string // overrides the computed stop when set
	Results                       []StepResult
}

func (d *Scripted) Run(ctx context.Context, as investigate.AgentSession, tools investigate.Tools) (*investigate.DriverResult, error) {
	s, err := asSession(tools)
	if err != nil {
		return nil, err
	}
	model, version, provider := d.Model, d.ModelVersion, d.Provider
	if model == "" {
		model = "scripted"
	}
	if version == "" {
		version = "scripted-v1"
	}
	if provider == "" {
		provider = "scripted"
	}
	dr := &investigate.DriverResult{Model: model, ModelVersion: version, Provider: provider, Turns: d.Turns, Tokens: d.Tokens}
	stop := ""
	for _, st := range d.Steps {
		call := st.Call
		if call == "" {
			call = "scripted-call-1"
		}
		dr.CallID = call
		s.SetCall(CallInfo{ID: call, Model: model, ModelVersion: version, Provider: provider})
		var args json.RawMessage
		switch a := st.Args.(type) {
		case nil:
			args = json.RawMessage("{}")
		case json.RawMessage:
			args = a
		case []byte:
			args = a
		case string:
			args = json.RawMessage(a)
		default:
			if args, err = json.Marshal(a); err != nil {
				return nil, fmt.Errorf("agent: scripted step %s: %w", st.Tool, err)
			}
		}
		res, err := s.Dispatch(ctx, st.Tool, args)
		d.Results = append(d.Results, StepResult{Tool: st.Tool, Result: res, Err: err})
		if reason, ok := stopAfter(err, s); ok {
			stop = reason
			break
		}
	}
	switch {
	case d.Stop != "":
		dr.Stop = d.Stop
	case stop != "":
		dr.Stop = stop
	case s.Submitted():
		dr.Stop = "submitted"
	default:
		dr.Stop = "error:the script ended without submit_findings"
	}
	s.RecordEnvelope(DriverScripted, *dr, nil)
	return dr, nil
}

// --- replay (reproducibility) ---------------------------------------------------------------

// Transcript is a parsed agent transcript.
type Transcript struct {
	Header   map[string]json.RawMessage
	Calls    []TranscriptCall
	Envelope *TranscriptEnvelope
}

// TranscriptEnvelope is the transcript's closing line.
type TranscriptEnvelope struct {
	Type     string                   `json:"type"`
	Driver   string                   `json:"driver"`
	Result   investigate.DriverResult `json:"result"`
	Envelope json.RawMessage          `json:"envelope,omitempty"`
}

// ParseTranscript reads an agent-transcript JSONL document.
func ParseTranscript(b []byte) (*Transcript, error) {
	t := &Transcript{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	n := 0
	for sc.Scan() {
		n++
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var kind struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &kind); err != nil {
			return nil, fmt.Errorf("agent: transcript line %d: %w", n, err)
		}
		switch kind.Type {
		case "session":
			if err := json.Unmarshal(line, &t.Header); err != nil {
				return nil, fmt.Errorf("agent: transcript line %d: %w", n, err)
			}
		case "call":
			var c TranscriptCall
			if err := json.Unmarshal(line, &c); err != nil {
				return nil, fmt.Errorf("agent: transcript line %d: %w", n, err)
			}
			t.Calls = append(t.Calls, c)
		case "envelope":
			var e TranscriptEnvelope
			if err := json.Unmarshal(line, &e); err != nil {
				return nil, fmt.Errorf("agent: transcript line %d: %w", n, err)
			}
			t.Envelope = &e
		default:
			return nil, fmt.Errorf("agent: transcript line %d: unknown type %q", n, kind.Type)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if t.Header == nil {
		return nil, errors.New("agent: transcript has no session line")
	}
	return t, nil
}

// PromptDigest returns the prompt digest the transcript was recorded under.
func (t *Transcript) PromptDigest() string {
	var d string
	_ = json.Unmarshal(t.Header["promptDigest"], &d)
	return d
}

// Mismatch is a replayed call whose result differs from the recording.
type Mismatch struct {
	Seq        int
	Tool       string
	Want, Got  string // result digests
	WantError  string
	GotError   string
	WantResult int // bytes
}

// Replay re-issues a recorded transcript's tool calls, in order and under
// the recorded call ids, against the same bundle, and checks every result
// digest (DESIGN.md §4.5). A mismatch is reported in Mismatches and in the
// stop reason — never hidden.
type Replay struct {
	Transcript []byte
	Mismatches []Mismatch
}

func (d *Replay) Run(ctx context.Context, as investigate.AgentSession, tools investigate.Tools) (*investigate.DriverResult, error) {
	s, err := asSession(tools)
	if err != nil {
		return nil, err
	}
	t, err := ParseTranscript(d.Transcript)
	if err != nil {
		return nil, investigate.PermanentError(domain.StageInvestigate, err)
	}
	if t.PromptDigest() != as.PromptDigest {
		dr := &investigate.DriverResult{Stop: fmt.Sprintf("error:replay: the transcript was recorded under prompt %s, this session's prompt is %s", t.PromptDigest(), as.PromptDigest)}
		s.RecordEnvelope(DriverReplay, *dr, nil)
		return dr, nil
	}
	d.Mismatches = nil
	for _, c := range t.Calls {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.SetCall(c.Call)
		res, err := s.Dispatch(ctx, c.Tool, c.Arguments)
		if got := ResultDigest(res, err); got != c.ResultDigest {
			m := Mismatch{Seq: c.Seq, Tool: c.Tool, Want: c.ResultDigest, Got: got, WantError: c.Error, WantResult: c.ResultBytes}
			if err != nil {
				m.GotError = err.Error()
			}
			d.Mismatches = append(d.Mismatches, m)
		}
	}
	dr := investigate.DriverResult{Stop: "error:replay: the transcript has no envelope line"}
	driver, envelope := DriverReplay, json.RawMessage(nil)
	if t.Envelope != nil {
		dr, driver, envelope = t.Envelope.Result, t.Envelope.Driver, t.Envelope.Envelope
	}
	if n := len(d.Mismatches); n > 0 {
		m := d.Mismatches[0]
		dr.Stop = fmt.Sprintf("error:replay: %d of %d results differ from the recording (first: call %d %s: recorded %s, got %s)", n, len(t.Calls), m.Seq, m.Tool, m.Want, m.Got)
	}
	s.RecordEnvelope(driver, dr, envelope)
	return &dr, nil
}

// --- exchange (any model, or a human, through the -llm-exchange directory) ---------------

// Exchange drives the session through the llm exchange directory protocol:
// one request file per tool round (system prompt + task + the rounds so
// far); the answer names the tool calls of the round. A round without an
// answer ends the run as pending; re-running replays the answered rounds
// (their tool results are deterministic, so the next request has the same
// digest) and continues.
type Exchange struct {
	Dir       string
	MaxTokens int // per-round answer cap stated in the request (default 8192)
}

const exchangeProtocol = `

## How to call tools in this session

You answer each round with one JSON document {"calls": [{"tool": "<name>", "arguments": {...}}, ...]}
naming the tool calls of this round (one or more). The next message gives their results, in order, as
[{"tool": ..., "ok": true, "result": ...} | {"tool": ..., "ok": false, "error": "..."}]. The tools and
their argument schemas are listed below. End the session with a round that calls submit_findings.

`

// roundSchema is the JSON Schema of one round's answer.
var roundSchema = json.RawMessage(`{"type":"object","properties":{"calls":{"type":"array","items":{"type":"object","properties":{"tool":{"type":"string"},"arguments":{"type":"object"}},"required":["tool","arguments"],"additionalProperties":false}}},"required":["calls"],"additionalProperties":false}`)

type roundAnswer struct {
	Calls []struct {
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"calls"`
}

type roundResult struct {
	Tool   string          `json:"tool"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func (d *Exchange) Run(ctx context.Context, as investigate.AgentSession, tools investigate.Tools) (*investigate.DriverResult, error) {
	s, err := asSession(tools)
	if err != nil {
		return nil, err
	}
	if d.Dir == "" {
		return nil, investigate.PermanentError(domain.StageInvestigate, errors.New("agent: exchange driver needs a directory"))
	}
	listing, err := json.MarshalIndent(s.Schemas(), "", "  ")
	if err != nil {
		return nil, err
	}
	system := as.SystemPrompt + exchangeProtocol + string(listing) + "\n"
	maxTokens := d.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}
	x := &llm.Exchange{Dir: d.Dir}
	msgs := []llm.Message{{Role: "user", Content: as.Task}}
	dr := &investigate.DriverResult{}
	finish := func(stop string) (*investigate.DriverResult, error) {
		dr.Stop = stop
		s.RecordEnvelope(DriverExchange, *dr, nil)
		return dr, nil
	}
	for turn := 1; ; turn++ {
		if as.Budget.MaxTurns > 0 && turn > as.Budget.MaxTurns {
			return finish("budget:turns")
		}
		req := llm.Request{Model: as.Model, System: system, Messages: msgs, MaxTokens: maxTokens, JSONSchema: roundSchema}
		resp, err := x.Complete(ctx, req)
		if errors.Is(err, llm.ErrPending) {
			return finish("pending:" + x.RequestPath(req))
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		dr.Turns = turn
		dr.Model, dr.ModelVersion = resp.Model, resp.ModelVersion
		call := resp.CallID
		if call == "" {
			// The exchange round itself identifies the call: its request digest.
			call = "exchange:" + strings.TrimPrefix(llm.PromptDigest(req), "sha256:")[:32]
		}
		dr.CallID = call
		s.SetCall(CallInfo{ID: call, Model: resp.Model, ModelVersion: resp.ModelVersion})
		var ans roundAnswer
		results := []roundResult{}
		stop := ""
		dec := json.NewDecoder(strings.NewReader(resp.Text))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&ans); err != nil {
			results = append(results, roundResult{Tool: "-", Error: "invalid round answer (want {\"calls\": [...]}): " + err.Error()})
		} else if len(ans.Calls) == 0 {
			results = append(results, roundResult{Tool: "-", Error: "the round named no tool calls; call tools, or end the session with submit_findings"})
		}
		for _, c := range ans.Calls {
			res, err := s.Dispatch(ctx, c.Tool, c.Arguments)
			r := roundResult{Tool: c.Tool, OK: err == nil, Result: res}
			if err != nil {
				r.Error = err.Error()
			}
			results = append(results, r)
			if reason, ok := stopAfter(err, s); ok {
				stop = reason
				break
			}
			if s.Submitted() {
				stop = "submitted"
				break
			}
		}
		if stop != "" {
			return finish(stop)
		}
		out, err := json.Marshal(results)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, llm.Message{Role: "assistant", Content: resp.Text}, llm.Message{Role: "user", Content: string(out)})
	}
}
