package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Producer is the component that records agent output (hypotheses,
// assertions, requests). Every such record carries AI provenance with this
// producer and the model call that produced it.
const Producer = "investigate.agent@v1"

// ProposedEdgeRule is the rule of an edge the agent proposed (always inferred).
const ProposedEdgeRule = "agent:proposed-edge@v1"

// TranscriptFormat names the agent-transcript JSONL format.
const TranscriptFormat = "ri.dev/agent-transcript/v1"

// Tool-surface bounds (DESIGN.md §4.1): every result is something a model can
// read and cite, never an unbounded dump.
const (
	MaxReadLines        = 400
	MaxReadWindow       = 64 << 10
	DefaultSearchLimit  = 20
	MaxSearchLimit      = 100
	MaxGrepPattern      = 512
	MaxGrepLines        = 20 // per artifact
	MaxGrepScan         = 64 << 20
	MaxLineText         = 300
	MaxExpandDepth      = 3
	MaxExpandNodes      = 300
	MaxQuote            = 4000
	MaxStatement        = 8000
	MaxCitations        = 20
	maxArgumentBytes    = 256 << 10
	maxTextCache        = 256 << 20
	unreportedModelInfo = "unreported"
)

// CallInfo identifies the model call the next tool calls belong to. ID is the
// provider's call/session id (claude -p session_id, an exchange response's
// callId), never invented; Model/ModelVersion are as reported, when the driver
// knows them at call time (otherwise they are filled in from the driver
// result when the run ends).
type CallInfo struct {
	ID           string `json:"callId"`
	Model        string `json:"model,omitempty"`
	ModelVersion string `json:"modelVersion,omitempty"`
	Provider     string `json:"provider,omitempty"`
}

// SessionConfig scopes a session to one investigation.
type SessionConfig struct {
	Bundle      *domain.EvidenceBundle
	Store       investigate.ArtifactStore
	Graph       *domain.RelationshipGraph
	Traversal   *domain.TraversalResult
	Report      *domain.ImpactReport
	Validations []domain.ValidationRecord // the deterministic pass before the agent
	Validators  []investigate.Validator
	// RunArtifacts are artifacts of the investigation beyond the bundle
	// (validator outputs, an earlier transcript) the session may serve.
	RunArtifacts []domain.ArtifactRef
	// ValidationArtifacts / ValidationEvidence are the deterministic pass's
	// outputs: readable and citable, resolved in the evidence pool, but not
	// part of the agent's outcome (the runner records them).
	ValidationArtifacts []domain.ArtifactRef
	ValidationEvidence  []domain.Evidence
	// Agenda is the agent's agenda (prompt feature agenda); BudgetRule the
	// formula that scaled Budget to it.
	Agenda     []domain.AgendaCoverage
	BudgetRule string
	// ItemCap is the per-item session cap (prompt feature perItem).
	ItemCap int
	Budget  domain.AgentBudget
	Prompt  *Prompt
	// PromptDigest pins the rendered prompt the session serves.
	PromptDigest string
	// Model is the requested model (the transcript header names it).
	Model string
	// ReadOnly serves the query tools only: record, request and submit tools
	// are rejected (an inspection session with no model call behind it).
	ReadOnly bool
	Now      func() time.Time
}

// Session is the agent's tool surface for one investigation (investigate.Tools).
// It is safe for concurrent use; calls are serialised so the transcript has
// one order. Every call — accepted or not — is counted against the budget and
// written to the transcript; a rejected call records nothing else.
type Session struct {
	cfg SessionConfig

	mu         sync.Mutex
	call       CallInfo
	toolCalls  int
	readBytes  int64
	exhausted  string // "budget:tool-calls" | "budget:read-bytes" when spent
	submitted  bool
	transcript bytes.Buffer
	seq        int

	arts     map[string]domain.ArtifactRef
	runArts  []domain.ArtifactRef
	text     map[string][]byte
	textSize int
	pool     map[domain.EvidenceID]domain.Evidence // bundle pool
	runEv    map[domain.EvidenceID]domain.Evidence
	runEvIDs []domain.EvidenceID
	rcs      map[string]domain.RenderedChange
	items    map[string]domain.ChangeItem
	nodes    map[string]domain.GraphNode
	nodeKey  map[string]string // key -> node id (first by id order)
	edges    map[string]domain.GraphEdge
	paths    map[string]domain.ImpactPath
	edge     *domain.UpgradeEdge
	edgeErr  error

	hyps        []domain.Hypothesis
	asserts     []domain.AgentAssertion
	evReqs      []domain.EvidenceRequest
	valReqs     []domain.ValidationRequest
	vals        []domain.ValidationRecord // run by agent requests
	valIDs      map[string]domain.ValidationRecord
	propNodes   []domain.GraphNode
	recordIndex map[string]bool
	passArts    []domain.ArtifactRef // deterministic-pass outputs (served, not the agent's)
	onAgenda    map[string]bool
	closing     map[string]investigate.ItemSummary // accepted submit entries by item (agenda mode)
	closingIDs  map[string]string                  // item -> closing assertion id
	closingEv   map[string][]domain.EvidenceID
	links       []investigate.AnswerLink
	examined    map[string]bool            // items retrieved with get_change_item
	inspections map[string]int             // accepted query calls naming the item
	citers      map[string]map[string]bool // citation key (artifact + quote) -> items whose records cite it

	// Per-item sessions (prompt feature perItem): the focus item, its own
	// counters, the artifacts retrieved in its session, its stats.
	focus     string
	itemCalls int
	itemRead  int64
	retrieved map[string]bool
	notes     []domain.AgentNote
	itemStats map[string]*itemStat
	callInfo  map[string]CallInfo // model info reported per call (per item session)
	rcArts    map[string][]string // rendered change -> the render artifacts of its pair
	sessions  []string            // items given a session, in order
	claims    []domain.SettingClaim
}

// itemStat is what one per-item session cost and how it ended.
type itemStat struct {
	CallID, Reason string
	Status         domain.AgentRunStatus
	Turns, Tokens  int
	ToolCalls      int
}

var (
	_ investigate.Tools = (*Session)(nil)
	_ ToolHandler       = (*Session)(nil)
)

// NewSession builds the tool surface for one investigation.
func NewSession(cfg SessionConfig) (*Session, error) {
	if cfg.Bundle == nil || cfg.Store == nil || cfg.Prompt == nil {
		return nil, errors.New("agent: session needs a bundle, an artifact store and a prompt")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	b := cfg.Bundle
	s := &Session{cfg: cfg, arts: map[string]domain.ArtifactRef{}, text: map[string][]byte{}, pool: b.EvidenceByID(),
		runEv: map[domain.EvidenceID]domain.Evidence{}, rcs: b.RenderedChanges(), items: map[string]domain.ChangeItem{},
		nodes: map[string]domain.GraphNode{}, nodeKey: map[string]string{}, edges: map[string]domain.GraphEdge{},
		paths: map[string]domain.ImpactPath{}, valIDs: map[string]domain.ValidationRecord{}, recordIndex: map[string]bool{},
		onAgenda: map[string]bool{}, examined: map[string]bool{}, inspections: map[string]int{}, citers: map[string]map[string]bool{}, itemStats: map[string]*itemStat{},
		callInfo: map[string]CallInfo{}, rcArts: map[string][]string{}, closing: map[string]investigate.ItemSummary{}, closingIDs: map[string]string{}, closingEv: map[string][]domain.EvidenceID{}}
	for _, a := range b.Manifest.Artifacts {
		s.arts[a.ID] = a
	}
	for _, a := range cfg.RunArtifacts {
		s.addRunArtifact(a)
	}
	for _, a := range cfg.ValidationArtifacts {
		if _, ok := s.arts[a.ID]; !ok {
			s.arts[a.ID] = a
			s.passArts = append(s.passArts, a)
		}
	}
	for _, e := range cfg.ValidationEvidence {
		if _, ok := s.pool[e.ID]; !ok {
			s.pool[e.ID] = e
		}
	}
	for _, e := range cfg.Agenda {
		s.onAgenda[e.ChangeItem] = true
	}
	for _, ps := range [][]domain.ManifestPair{b.Renders, b.Change.Renders} {
		for _, p := range ps {
			for _, c := range p.Changes {
				for _, a := range []string{p.Diff, p.From, p.To} {
					if a != "" {
						s.rcArts[c.ID] = append(s.rcArts[c.ID], a)
					}
				}
			}
		}
	}
	for _, it := range b.Items() {
		s.items[it.ID] = it
	}
	if g := cfg.Graph; g != nil {
		ns := append([]domain.GraphNode(nil), g.Nodes...)
		sort.Slice(ns, func(i, j int) bool { return ns[i].ID < ns[j].ID })
		for _, n := range ns {
			s.nodes[n.ID] = n
			if _, ok := s.nodeKey[n.Key]; !ok {
				s.nodeKey[n.Key] = n.ID
			}
		}
		for _, e := range g.Edges {
			s.edges[e.ID] = e
		}
	}
	if t := cfg.Traversal; t != nil {
		for _, p := range t.Paths {
			s.paths[p.ID] = p
		}
	}
	for _, v := range cfg.Validations {
		s.valIDs[v.ID] = v
	}
	header := map[string]any{"type": "session", "format": TranscriptFormat, "bundle": b.ID, "promptVersion": cfg.Prompt.Version,
		"promptDigest": cfg.PromptDigest, "model": cfg.Model, "budget": cfg.Budget, "readOnly": cfg.ReadOnly}
	if cfg.Prompt.Features.Agenda {
		var items []string
		for _, e := range cfg.Agenda {
			items = append(items, e.ChangeItem)
		}
		header["agenda"], header["budgetRule"] = items, cfg.BudgetRule
	}
	if cfg.Prompt.Features.PerItem {
		header["itemCap"] = cfg.ItemCap
	}
	s.writeLine(header)
	return s, nil
}

// SetCall names the model call subsequent tool calls belong to.
func (s *Session) SetCall(c CallInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.call = c
}

// Submitted reports whether submit_findings was accepted.
func (s *Session) Submitted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.submitted
}

// Exhausted returns which tool-layer budget was spent ("" when none).
func (s *Session) Exhausted() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exhausted
}

// Counters returns the tool calls made and the bytes returned so far.
func (s *Session) Counters() (toolCalls int, readBytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.toolCalls, s.readBytes
}

// Schemas returns the tool schemas the session serves.
func (s *Session) Schemas() []ToolSchema { return s.cfg.Prompt.Tools }

// Transcript returns the JSONL transcript so far.
func (s *Session) Transcript() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.transcript.Bytes()...)
}

// RecordEnvelope appends the driver's closing line: the driver result and the
// provider envelope as reported (DESIGN.md §4.5).
func (s *Session) RecordEnvelope(driver string, dr investigate.DriverResult, envelope json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	line := map[string]any{"type": "envelope", "driver": driver, "result": dr}
	if len(envelope) > 0 && json.Valid(envelope) {
		line["envelope"] = envelope
	}
	s.writeLine(line)
}

func (s *Session) writeLine(v any) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(fmt.Sprintf("agent: transcript: %v", err))
	}
	s.transcript.Write(b.Bytes())
}

// TranscriptCall is one tool call line of the transcript.
type TranscriptCall struct {
	Type         string          `json:"type"` // "call"
	Seq          int             `json:"seq"`
	Call         CallInfo        `json:"call"`
	Tool         string          `json:"tool"`
	Arguments    json.RawMessage `json:"arguments"`
	OK           bool            `json:"ok"`
	Error        string          `json:"error,omitempty"`
	ErrorClass   string          `json:"errorClass,omitempty"` // rejected | budget | not-found | error
	ResultDigest string          `json:"resultDigest"`         // sha256 of the bytes returned to the model (or of the error)
	ResultBytes  int             `json:"resultBytes"`
	ToolCalls    int             `json:"toolCalls"` // counters after the call
	ReadBytes    int64           `json:"readBytes"`
}

// outcome is what a tool's implementation produced: the typed value the Go
// interface returns, the view the model receives (default: the value), and
// the state change to apply when the call is accepted.
type outcome struct {
	value  any
	view   any
	commit func()
}

// ResultDigest is the digest the transcript records for a result (the exact
// bytes returned to the model) or for an error.
func ResultDigest(result []byte, err error) string {
	if err != nil {
		return domain.Digest([]byte("error\x00" + err.Error()))
	}
	return domain.Digest(result)
}

// invoke runs one tool call: argument recording, budget accounting,
// implementation, the result-size budget, commit, transcript.
func (s *Session) invoke(ctx context.Context, tool string, args json.RawMessage, impl func(context.Context) (*outcome, error)) (any, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	entry := TranscriptCall{Type: "call", Seq: s.seq, Call: s.call, Tool: tool, Arguments: args}
	var (
		out *outcome
		res []byte
		err error
	)
	switch {
	case s.submitted:
		err = rejectf("the run was submitted; no further calls are accepted")
	case s.exhausted != "" && s.closingAllowed(tool):
		// With an agenda, the closing submit_findings is accepted once after
		// the budget is spent, so coverage is always reported; the run stays
		// budget-exhausted. It is not counted and its result not metered.
		if out, err = impl(ctx); err == nil {
			view := out.view
			if view == nil {
				view = out.value
			}
			if res, err = normalizedJSON(view); err == nil && out.commit != nil {
				out.commit()
			}
		}
	case s.exhausted != "":
		err = fmt.Errorf("%w: %s", investigate.ErrBudgetExhausted, s.exhausted)
	default:
		s.toolCalls++
		s.itemCalls++
		calls := s.toolCalls
		if s.focus != "" {
			calls = s.itemCalls // a per-item session has its own budget
		}
		if max := s.cfg.Budget.MaxToolCalls; max > 0 && calls > max {
			s.exhausted = "budget:tool-calls"
			err = fmt.Errorf("%w: %s (%d calls)", investigate.ErrBudgetExhausted, s.exhausted, max)
			break
		}
		if impl == nil {
			err = rejectf("unknown tool %q", tool)
			break
		}
		if out, err = impl(ctx); err != nil {
			break
		}
		view := out.view
		if view == nil {
			view = out.value
		}
		if res, err = normalizedJSON(view); err != nil {
			err = fmt.Errorf("agent: encode %s result: %w", tool, err)
			break
		}
		read := s.readBytes
		if s.focus != "" {
			read = s.itemRead
		}
		if max := s.cfg.Budget.MaxReadBytes; max > 0 && read+int64(len(res)) > max {
			s.exhausted = "budget:read-bytes"
			err = fmt.Errorf("%w: %s (%d of %d bytes returned; this result is %d bytes)", investigate.ErrBudgetExhausted, s.exhausted, read, max, len(res))
			res = nil
			break
		}
		s.readBytes += int64(len(res))
		s.itemRead += int64(len(res))
		if out.commit != nil {
			out.commit()
		}
	}
	entry.OK = err == nil
	if err != nil {
		entry.Error, entry.ErrorClass = err.Error(), errorClass(err)
		res = nil
	}
	entry.ResultDigest, entry.ResultBytes = ResultDigest(res, err), len(res)
	entry.ToolCalls, entry.ReadBytes = s.toolCalls, s.readBytes
	s.writeLine(entry)
	if err != nil {
		return nil, nil, err
	}
	return out.value, res, nil
}

func (s *Session) closingAllowed(tool string) bool {
	return s.cfg.Prompt.Features.Agenda && tool == investigate.ToolSubmitFindings
}

func errorClass(err error) string {
	switch {
	case errors.Is(err, investigate.ErrBudgetExhausted):
		return "budget"
	case errors.Is(err, investigate.ErrRejected):
		return "rejected"
	case errors.Is(err, investigate.ErrNotFound):
		return "not-found"
	}
	return "error"
}

func rejectf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", investigate.ErrRejected, fmt.Sprintf(format, args...))
}

// volatileKeys are wall-clock fields stripped from results: a result digest
// must depend on the evidence only, so a replay reproduces it.
var volatileKeys = map[string]bool{"checkedAt": true, "generatedAt": true, "retrievedAt": true, "createdAt": true, "collectedAt": true, "observedAt": true}

// normalizedJSON encodes v canonically (sorted keys, no HTML escaping) with
// wall-clock fields removed.
func normalizedJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil, err
	}
	tree = stripVolatile(tree)
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(tree); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

func stripVolatile(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			if volatileKeys[k] {
				delete(t, k)
				continue
			}
			t[k] = stripVolatile(x)
		}
	case []any:
		for i := range t {
			t[i] = stripVolatile(t[i])
		}
	}
	return v
}

// --- dispatch (the JSON path every driver and the MCP server use) -------------------

// ToolHandler is what the MCP server and the JSON drivers call.
type ToolHandler interface {
	Schemas() []ToolSchema
	Dispatch(ctx context.Context, tool string, args json.RawMessage) (json.RawMessage, error)
}

// Dispatch decodes the arguments strictly (unknown fields are rejected) and
// runs the tool; it returns the JSON the model receives.
func (s *Session) Dispatch(ctx context.Context, tool string, args json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage("{}")
	}
	var res []byte
	var err error
	bad := func(e error) {
		_, res, err = s.invoke(ctx, tool, argsForTranscript(args), func(context.Context) (*outcome, error) { return nil, e })
	}
	if len(args) > maxArgumentBytes {
		bad(rejectf("arguments are %d bytes (max %d)", len(args), maxArgumentBytes))
		return nil, err
	}
	decode := func(v any) bool {
		dec := json.NewDecoder(bytes.NewReader(args))
		dec.DisallowUnknownFields()
		if e := dec.Decode(v); e != nil {
			bad(rejectf("invalid arguments for %s: %v", tool, e))
			return false
		}
		if dec.More() {
			bad(rejectf("invalid arguments for %s: trailing data", tool))
			return false
		}
		return true
	}
	switch tool {
	case investigate.ToolGetChangeItem:
		var in getChangeItemArgs
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.getChangeItem(ctx, in.ID) })
		}
	case investigate.ToolListRenders:
		var in struct{}
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, s.listRenders)
		}
	case investigate.ToolQueryDiff:
		var in investigate.DiffQuery
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.queryDiff(in) })
		}
	case investigate.ToolGetRenderedObject:
		var in investigate.ObjectRequest
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.getRenderedObject(ctx, in) })
		}
	case investigate.ToolSearchManifest:
		var in investigate.SearchQuery
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.searchManifest(ctx, in) })
		}
	case investigate.ToolReadArtifact:
		var in investigate.ReadRequest
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.readArtifact(ctx, in) })
		}
	case investigate.ToolExpandGraph:
		var in investigate.ExpandRequest
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.expandGraph(in) })
		}
	case investigate.ToolListPaths:
		var in investigate.PathQuery
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.listPaths(in) })
		}
	case investigate.ToolRequestEvidence:
		var in investigate.EvidenceRequestInput
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.requestEvidence(in) })
		}
	case investigate.ToolRequestValidation:
		var in investigate.ValidationRequestInput
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.requestValidation(ctx, in) })
		}
	case investigate.ToolRecordHypothesis:
		var in investigate.HypothesisInput
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.recordHypothesis(ctx, in) })
		}
	case investigate.ToolRecordAssertion:
		var in investigate.AssertionInput
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.recordAssertion(ctx, in) })
		}
	case investigate.ToolRecordClaim:
		if !s.cfg.Prompt.Features.Claims {
			_, res, err = s.invoke(ctx, tool, argsForTranscript(args), nil)
			break
		}
		var in investigate.ClaimInput
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.recordClaim(ctx, in) })
		}
	case investigate.ToolSubmitFindings:
		var in investigate.SubmitInput
		if decode(&in) {
			res, err = s.dispatch(ctx, tool, in, func(ctx context.Context) (*outcome, error) { return s.submitFindings(ctx, in) })
		}
	default:
		_, res, err = s.invoke(ctx, tool, argsForTranscript(args), nil)
	}
	return res, err
}

// dispatch records the decoded (canonical) arguments and runs the tool.
func (s *Session) dispatch(ctx context.Context, tool string, in any, impl func(context.Context) (*outcome, error)) ([]byte, error) {
	_, res, err := s.invoke(ctx, tool, canonicalArgs(in), impl)
	return res, err
}

// typed runs a tool from the Go interface: the same path as Dispatch.
func typed[T any](s *Session, ctx context.Context, tool string, in any, impl func(context.Context) (*outcome, error)) (T, error) {
	var zero T
	v, _, err := s.invoke(ctx, tool, canonicalArgs(in), impl)
	if err != nil {
		return zero, err
	}
	t, _ := v.(T)
	return t, nil
}

func canonicalArgs(in any) json.RawMessage {
	b, err := normalizedJSON(in)
	if err != nil {
		panic(fmt.Sprintf("agent: encode arguments: %v", err))
	}
	return b
}

// argsForTranscript keeps undecodable arguments: valid JSON as is, anything
// else as a JSON string.
func argsForTranscript(args json.RawMessage) json.RawMessage {
	if json.Valid(args) {
		if b, err := normalizedJSON(json.RawMessage(args)); err == nil {
			return b
		}
		return args
	}
	b, _ := json.Marshal(string(args))
	return b
}

type getChangeItemArgs struct {
	ID string `json:"id"`
}

// --- the investigate.Tools interface ------------------------------------------------

func (s *Session) GetChangeItem(ctx context.Context, id string) (*investigate.ChangeItemDetail, error) {
	return typed[*investigate.ChangeItemDetail](s, ctx, investigate.ToolGetChangeItem, getChangeItemArgs{ID: id},
		func(ctx context.Context) (*outcome, error) { return s.getChangeItem(ctx, id) })
}

func (s *Session) ListRenders(ctx context.Context) ([]investigate.RenderSummary, error) {
	return typed[[]investigate.RenderSummary](s, ctx, investigate.ToolListRenders, struct{}{}, s.listRenders)
}

func (s *Session) QueryDiff(ctx context.Context, q investigate.DiffQuery) ([]domain.RenderedChange, error) {
	return typed[[]domain.RenderedChange](s, ctx, investigate.ToolQueryDiff, q, func(context.Context) (*outcome, error) { return s.queryDiff(q) })
}

func (s *Session) GetRenderedObject(ctx context.Context, r investigate.ObjectRequest) (*investigate.ObjectResult, error) {
	return typed[*investigate.ObjectResult](s, ctx, investigate.ToolGetRenderedObject, r,
		func(ctx context.Context) (*outcome, error) { return s.getRenderedObject(ctx, r) })
}

func (s *Session) SearchManifest(ctx context.Context, q investigate.SearchQuery) ([]investigate.SearchHit, error) {
	return typed[[]investigate.SearchHit](s, ctx, investigate.ToolSearchManifest, q,
		func(ctx context.Context) (*outcome, error) { return s.searchManifest(ctx, q) })
}

func (s *Session) ReadArtifact(ctx context.Context, r investigate.ReadRequest) (*investigate.ReadResult, error) {
	return typed[*investigate.ReadResult](s, ctx, investigate.ToolReadArtifact, r,
		func(ctx context.Context) (*outcome, error) { return s.readArtifact(ctx, r) })
}

func (s *Session) ExpandGraph(ctx context.Context, r investigate.ExpandRequest) (*investigate.Subgraph, error) {
	return typed[*investigate.Subgraph](s, ctx, investigate.ToolExpandGraph, r, func(context.Context) (*outcome, error) { return s.expandGraph(r) })
}

func (s *Session) ListPaths(ctx context.Context, q investigate.PathQuery) ([]domain.ImpactPath, error) {
	return typed[[]domain.ImpactPath](s, ctx, investigate.ToolListPaths, q, func(context.Context) (*outcome, error) { return s.listPaths(q) })
}

func (s *Session) RequestEvidence(ctx context.Context, in investigate.EvidenceRequestInput) (*domain.EvidenceRequest, error) {
	return typed[*domain.EvidenceRequest](s, ctx, investigate.ToolRequestEvidence, in, func(context.Context) (*outcome, error) { return s.requestEvidence(in) })
}

func (s *Session) RequestValidation(ctx context.Context, in investigate.ValidationRequestInput) (*domain.ValidationRequest, *domain.ValidationRecord, error) {
	v, err := typed[validationResult](s, ctx, investigate.ToolRequestValidation, in,
		func(ctx context.Context) (*outcome, error) { return s.requestValidation(ctx, in) })
	if err != nil {
		return nil, nil, err
	}
	return v.Request, v.Validation, nil
}

func (s *Session) RecordHypothesis(ctx context.Context, in investigate.HypothesisInput) (*domain.Hypothesis, error) {
	return typed[*domain.Hypothesis](s, ctx, investigate.ToolRecordHypothesis, in,
		func(ctx context.Context) (*outcome, error) { return s.recordHypothesis(ctx, in) })
}

func (s *Session) RecordAssertion(ctx context.Context, in investigate.AssertionInput) (*domain.AgentAssertion, error) {
	return typed[*domain.AgentAssertion](s, ctx, investigate.ToolRecordAssertion, in,
		func(ctx context.Context) (*outcome, error) { return s.recordAssertion(ctx, in) })
}

func (s *Session) RecordClaim(ctx context.Context, in investigate.ClaimInput) (*domain.SettingClaim, error) {
	if !s.cfg.Prompt.Features.Claims {
		_, _, err := s.invoke(ctx, investigate.ToolRecordClaim, canonicalArgs(in), nil)
		return nil, err
	}
	return typed[*domain.SettingClaim](s, ctx, investigate.ToolRecordClaim, in, func(ctx context.Context) (*outcome, error) { return s.recordClaim(ctx, in) })
}

func (s *Session) SubmitFindings(ctx context.Context, in investigate.SubmitInput) error {
	_, err := typed[any](s, ctx, investigate.ToolSubmitFindings, in, func(ctx context.Context) (*outcome, error) { return s.submitFindings(ctx, in) })
	return err
}

// --- shared lookups -----------------------------------------------------------------

// artifact resolves an artifact id in the manifest or the run.
func (s *Session) artifact(id string) (domain.ArtifactRef, bool) {
	a, ok := s.arts[id]
	return a, ok
}

func (s *Session) addRunArtifact(a domain.ArtifactRef) {
	if _, ok := s.arts[a.ID]; ok {
		return
	}
	s.arts[a.ID] = a
	s.runArts = append(s.runArts, a)
}

func (s *Session) addRunEvidence(e domain.Evidence) {
	if _, ok := s.pool[e.ID]; ok {
		return
	}
	if _, ok := s.runEv[e.ID]; ok {
		return
	}
	s.runEv[e.ID] = e
	s.runEvIDs = append(s.runEvIDs, e.ID)
}

// bytesOf reads an artifact's bytes (digest-verified by the store).
func (s *Session) bytesOf(ctx context.Context, a domain.ArtifactRef) ([]byte, error) {
	if b, ok := s.text[a.Digest]; ok {
		return b, nil
	}
	rc, err := s.cfg.Store.Open(ctx, a.Digest)
	if err != nil {
		return nil, fmt.Errorf("artifact %s (%s): %w", a.ID, a.Path, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("artifact %s (%s): %w", a.ID, a.Path, err)
	}
	if s.textSize+len(b) <= maxTextCache {
		s.text[a.Digest] = b
		s.textSize += len(b)
	}
	return b, nil
}

// textOf reads an artifact as text; binary artifacts (chart archives) are
// not readable by the model.
func (s *Session) textOf(ctx context.Context, a domain.ArtifactRef) (string, error) {
	b, err := s.bytesOf(ctx, a)
	if err != nil {
		return "", err
	}
	if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
		return "", rejectf("artifact %s (%s) is binary (%s); it cannot be read or quoted", a.ID, a.Path, a.MediaType)
	}
	return string(b), nil
}

// node resolves a node id, or a node key, to a node id.
func (s *Session) node(ref string) (string, bool) {
	if _, ok := s.nodes[ref]; ok {
		return ref, true
	}
	id, ok := s.nodeKey[ref]
	return id, ok
}

func (s *Session) nodeIDs(what string, refs []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, r := range refs {
		id, ok := s.node(r)
		if !ok {
			if s.cfg.Graph == nil {
				return nil, rejectf("%s %q: this investigation has no relationship graph", what, r)
			}
			return nil, rejectf("unknown %s %q (not a node id or key of the relationship graph)", what, r)
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s *Session) item(id string) (domain.ChangeItem, error) {
	if id == "" {
		return domain.ChangeItem{}, rejectf("changeItem is required")
	}
	it, ok := s.items[id]
	if !ok {
		return domain.ChangeItem{}, rejectf("unknown change item %q", id)
	}
	return it, nil
}

func (s *Session) renderedIDs(ids []string) ([]string, error) {
	out := uniqSorted(ids)
	for _, id := range out {
		if _, ok := s.rcs[id]; !ok {
			return nil, rejectf("unknown rendered change %q", id)
		}
	}
	return out, nil
}

// itemEvidence is the upstream evidence the task gave the model for the
// items: the input evidence of records not tied to one item.
func (s *Session) taskEvidence() []domain.EvidenceID {
	var out []domain.EvidenceID
	for _, it := range s.cfg.Bundle.Items() {
		if it.InScope {
			out = append(out, it.Evidence...)
		}
	}
	return uniqEvidence(out)
}

// aiProvenance is the provenance of a record made in the current call.
func (s *Session) aiProvenance(inputs []domain.EvidenceID) domain.Provenance {
	t := s.cfg.Now().UTC().Truncate(time.Second)
	return domain.Provenance{Method: domain.MethodAI, Producer: Producer, Confidence: domain.ConfidenceLow,
		Model: s.call.Model, ModelVersion: s.call.ModelVersion, PromptVersion: s.cfg.Prompt.Version, PromptDigest: s.cfg.PromptDigest,
		InputEvidence: uniqEvidence(inputs), GeneratedAt: &t, CallID: s.call.ID, Provider: s.call.Provider}
}

func (s *Session) writable() error {
	if s.cfg.ReadOnly {
		return rejectf("read-only session: records and requests are accepted only from a driven agent run (ri investigate --agent)")
	}
	if s.call.ID == "" {
		return rejectf("no model call is active: records need the provider's call id")
	}
	return nil
}

func uniqSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	j := 0
	for i, x := range out {
		if i == 0 || x != out[j-1] {
			out[j] = x
			j++
		}
	}
	return out[:j]
}

func uniqEvidence(in []domain.EvidenceID) []domain.EvidenceID {
	seen := map[domain.EvidenceID]bool{}
	var out []domain.EvidenceID
	for _, e := range in {
		if e != "" && !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func trimmed(s string) string { return strings.TrimSpace(s) }

// --- per-item sessions (prompt feature perItem) -------------------------------------------

// BeginItem starts the session of one agenda item: its budget and prompt
// digest, fresh per-session counters, a transcript "item" line.
func (s *Session) BeginItem(item string, budget domain.AgentBudget, promptDigest string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.focus, s.cfg.Budget, s.cfg.PromptDigest = item, budget, promptDigest
	s.exhausted, s.submitted, s.itemCalls, s.itemRead = "", false, 0, 0
	s.retrieved = map[string]bool{}
	s.sessions = append(s.sessions, item)
	s.writeLine(map[string]any{"type": "item", "item": item, "promptDigest": promptDigest, "budget": budget})
}

// EndItem records how the focus item's session ended.
func (s *Session) EndItem(dr investigate.DriverResult) {
	status, reason := runStatus(dr, s)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.itemStats[s.focus] = &itemStat{CallID: dr.CallID, Status: status, Reason: reason, Turns: dr.Turns, Tokens: dr.Tokens, ToolCalls: s.itemCalls}
	if dr.CallID != "" {
		s.callInfo[dr.CallID] = CallInfo{ID: dr.CallID, Model: dr.Model, ModelVersion: dr.ModelVersion, Provider: dr.Provider}
	}
}

// retrieve returns a commit that records artifacts as retrieved in the
// focus item's session (composed with another commit).
func (s *Session) retrieve(then func(), ids ...string) func() {
	if !s.cfg.Prompt.Features.PerItem || s.focus == "" {
		return then
	}
	return func() {
		for _, id := range ids {
			if id != "" {
				s.retrieved[id] = true
			}
		}
		if then != nil {
			then()
		}
	}
}

// itemEvidence reports whether a raises-concern about item may stand as an
// item concern (rule: it is about the session's item and cites an artifact
// retrieved in that session), and why not.
func (s *Session) itemEvidence(item string, evs []domain.Evidence) (bool, string) {
	if !s.cfg.Prompt.Features.PerItem {
		return true, ""
	}
	if item != s.focus {
		return false, fmt.Sprintf("raised in the session of %s, not of %s", s.focus, item)
	}
	for _, e := range evs {
		if s.retrieved[e.SourceID] {
			return true, ""
		}
	}
	return false, "it cites no artifact retrieved for the item in its own session"
}

// note builds an investigation-level note from a concern without item evidence.
func (s *Session) note(about, statement, reason string, evs []domain.Evidence) (domain.AgentNote, error) {
	cites := evidenceIDs(evs)
	n := domain.AgentNote{ID: domain.AgentRecordID(domain.AgentNoteIDPrefix, about, statement, s.call.ID), About: about, Statement: statement,
		Reason: reason, Citations: cites, StatementKind: s.proseKind(), Provenance: s.aiProvenance(cites)}
	if len(n.Provenance.InputEvidence) == 0 {
		n.Provenance.InputEvidence = s.taskEvidence()
	}
	return n, invalid(func() error { c := n; c.Provenance = placeholder(c.Provenance); return c.Validate() }())
}
