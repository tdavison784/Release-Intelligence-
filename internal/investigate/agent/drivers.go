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
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
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
	// Steps are consumed in order across runs: each run (one per-item
	// session, or the single session) plays steps up to and including an
	// accepted submit_findings. ByItem, when set, gives each per-item
	// session its own steps instead.
	Steps         []Step
	ByItem        map[string][]Step
	Turns, Tokens int
	Stop          string // overrides the computed stop when set
	Results       []StepResult
	next          int
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
	steps := d.Steps[d.next:]
	if d.ByItem != nil {
		steps = d.ByItem[as.ChangeItem]
	}
	for _, st := range steps {
		if d.ByItem == nil {
			d.next++
		}
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
		if reason, ok := stopAfter(err, s); ok && !(err != nil && st.Tool == investigate.ToolSubmitFindings) {
			stop = reason
			break
		}
		if st.Tool == investigate.ToolSubmitFindings && err == nil {
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

// Transcript is a parsed agent transcript. Calls and Envelope cover the
// whole run (the last envelope); Segments split it per session: one per
// per-item session ("item" lines), or a single segment otherwise.
type Transcript struct {
	Header   map[string]json.RawMessage
	Calls    []TranscriptCall
	Envelope *TranscriptEnvelope
	Segments []TranscriptSegment
}

// TranscriptSegment is one driver session of the transcript.
type TranscriptSegment struct {
	Item         string // "" for a single-session run
	PromptDigest string
	Calls        []TranscriptCall
	Envelope     *TranscriptEnvelope
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
	seg := func() *TranscriptSegment {
		if len(t.Segments) == 0 {
			t.Segments = append(t.Segments, TranscriptSegment{})
		}
		return &t.Segments[len(t.Segments)-1]
	}
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
		case "item":
			var it struct {
				Item         string `json:"item"`
				PromptDigest string `json:"promptDigest"`
			}
			if err := json.Unmarshal(line, &it); err != nil {
				return nil, fmt.Errorf("agent: transcript line %d: %w", n, err)
			}
			if len(t.Segments) == 1 && t.Segments[0].Item == "" && len(t.Segments[0].Calls) == 0 && t.Segments[0].Envelope == nil {
				t.Segments = t.Segments[:0]
			}
			t.Segments = append(t.Segments, TranscriptSegment{Item: it.Item, PromptDigest: it.PromptDigest})
		case "call":
			var c TranscriptCall
			if err := json.Unmarshal(line, &c); err != nil {
				return nil, fmt.Errorf("agent: transcript line %d: %w", n, err)
			}
			t.Calls = append(t.Calls, c)
			seg().Calls = append(seg().Calls, c)
		case "envelope":
			var e TranscriptEnvelope
			if err := json.Unmarshal(line, &e); err != nil {
				return nil, fmt.Errorf("agent: transcript line %d: %w", n, err)
			}
			t.Envelope = &e
			seg().Envelope = &e
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
	if len(t.Segments) == 1 && t.Segments[0].Item == "" {
		t.Segments[0].PromptDigest = t.PromptDigest()
	}
	return t, nil
}

// ConsumerRules returns the consumer rules the transcript was recorded under:
// its header's, else relations.ConsumerRulesV1 (transcripts recorded before
// the rules were versioned, ledger D-0005).
func (t *Transcript) ConsumerRules() int {
	var n int
	if err := json.Unmarshal(t.Header["consumerRules"], &n); err != nil || n == 0 {
		return relations.ConsumerRulesV1
	}
	return n
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
	parsed     *Transcript
	next       int // next segment (per-item sessions)
}

func (d *Replay) Run(ctx context.Context, as investigate.AgentSession, tools investigate.Tools) (*investigate.DriverResult, error) {
	s, err := asSession(tools)
	if err != nil {
		return nil, err
	}
	if d.parsed == nil {
		t, err := ParseTranscript(d.Transcript)
		if err != nil {
			return nil, investigate.PermanentError(domain.StageInvestigate, err)
		}
		d.parsed, d.next, d.Mismatches = t, 0, nil
	}
	t := d.parsed
	if d.next >= len(t.Segments) {
		dr := &investigate.DriverResult{Stop: fmt.Sprintf("error:replay: the transcript has no session %d (item %s)", d.next+1, as.ChangeItem)}
		s.RecordEnvelope(DriverReplay, *dr, nil)
		return dr, nil
	}
	sg := t.Segments[d.next]
	d.next++
	if sg.Item != as.ChangeItem || sg.PromptDigest != as.PromptDigest {
		dr := &investigate.DriverResult{Stop: fmt.Sprintf("error:replay: the transcript was recorded under prompt %s (item %q), this session's prompt is %s (item %q)",
			sg.PromptDigest, sg.Item, as.PromptDigest, as.ChangeItem)}
		s.RecordEnvelope(DriverReplay, *dr, nil)
		return dr, nil
	}
	first := len(d.Mismatches)
	for _, c := range sg.Calls {
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
	if sg.Envelope != nil {
		dr, driver, envelope = sg.Envelope.Result, sg.Envelope.Driver, sg.Envelope.Envelope
	}
	if n := len(d.Mismatches) - first; n > 0 {
		m := d.Mismatches[first]
		dr.Stop = fmt.Sprintf("error:replay: %d of %d results differ from the recording (first: call %d %s: recorded %s, got %s)", n, len(sg.Calls), m.Seq, m.Tool, m.Want, m.Got)
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
