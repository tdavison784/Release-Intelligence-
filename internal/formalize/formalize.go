// Package formalize is the evidence-formalization stage (Phase 6 Milestone 4,
// Workstream D; docs/phase6/decision-intel/M4-IMPLEMENTATION.md): it turns
// one natural agent assessment into testable hypotheses — a cited mitigation
// rule in the fact grammar where the grammar can carry the claim, an
// unrepresentable record (the claim, its citations, intended conditions,
// the grammar limitation and a proposed verification) where it cannot.
//
// One structured model call through llm.Cache (a file exchange or a stateless
// `claude -p`), checked by a deterministic floor (non-invention: only the
// assessment's own evidence is cited, every uncertainty is kept, a rule
// passes the spec checks of agent.CheckMitigation and is never broader than
// the run), retried within MaxAttempts and otherwise recorded failed, never
// repaired. The output feeds agent.ExportHypotheses unchanged; formalization
// never changes the run record, a classification or knowledge.
package formalize

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// DefaultModel is the formalization model unless configured otherwise.
const DefaultModel = "claude-opus-5-5"

// MaxAttempts bounds the calls per formalization: the first answer and two
// retries, each told the previous answer's violations.
const MaxAttempts = 3

//go:embed prompts
var prompts embed.FS

// Prompt is one versioned formalization prompt.
type Prompt struct {
	Version string
	System  string
	Task    string
	Schema  json.RawMessage
}

// LoadPrompt loads a prompt version ("formalize/v1").
func LoadPrompt(version string) (*Prompt, error) {
	dir := "prompts/" + version
	read := func(name string) (string, error) {
		b, err := prompts.ReadFile(dir + "/" + name)
		return string(b), err
	}
	sys, err := read("system.md")
	if err != nil {
		return nil, fmt.Errorf("formalize: unknown prompt %q", version)
	}
	task, err := read("task.md")
	if err != nil {
		return nil, err
	}
	schema, err := read("schema.json")
	if err != nil {
		return nil, err
	}
	var compact json.RawMessage
	if err := json.Unmarshal([]byte(schema), &compact); err != nil {
		return nil, fmt.Errorf("formalize: prompt %s schema: %w", version, err)
	}
	return &Prompt{Version: version, System: sys, Task: task, Schema: compact}, nil
}

// Digest pins the prompt templates and schema (the rendered request's digest
// is recorded per call as the record's promptDigest).
func (p *Prompt) Digest() string {
	return domain.Digest([]byte(p.System + "\x00" + p.Task + "\x00" + string(p.Schema)))
}

// Formalizer formalizes assessments: one structured call through Client
// (normally llm.Cache over an exchange or `claude -p`), checked by the floor
// and retried within MaxAttempts.
type Formalizer struct {
	Client   llm.Client
	Model    string // default DefaultModel
	Provider string // default "anthropic"
	Prompt   string // default PromptVersion
	// MaxTokens bounds each answer (default 16000).
	MaxTokens int
}

// Formalize formalizes one assessment and returns its record: complete when
// an answer passed the floor, failed (recorded, never repaired) when none
// did within the bound. An error means no answer was obtained at all (a
// pending exchange, an unreachable model) or the input is not formalizable.
// The input is never changed.
func (f *Formalizer) Formalize(ctx context.Context, in Input) (*Record, error) {
	p, err := Precompute(in)
	if err != nil {
		return nil, err
	}
	return f.Run(ctx, p)
}

// Run runs the call and the floor over an already computed input.
func (f *Formalizer) Run(ctx context.Context, p *Precomputed) (*Record, error) {
	if f.Client == nil {
		return nil, errors.New("formalize: no model client is configured")
	}
	pv := f.Prompt
	if pv == "" {
		pv = PromptVersion
	}
	pr, err := LoadPrompt(pv)
	if err != nil {
		return nil, err
	}
	model := f.Model
	if model == "" {
		model = DefaultModel
	}
	maxTokens := f.MaxTokens
	if maxTokens == 0 {
		maxTokens = 16000
	}
	input, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	task := pr.Task + string(input) + "\n"
	var answers, violations []string
	var resp *llm.Response
	var req llm.Request
	var accepted []checked
	var acceptedAnswer *Answer
	for attempt := 1; attempt <= MaxAttempts && acceptedAnswer == nil; attempt++ {
		msg := task
		if attempt > 1 {
			msg = retryMessage(task, attempt, resp.Text, violations)
		}
		req = llm.Request{Model: model, System: pr.System, Messages: []llm.Message{{Role: "user", Content: msg}},
			MaxTokens: maxTokens, JSONSchema: pr.Schema}
		resp, err = f.Client.Complete(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("formalize: attempt %d: %w", attempt, err)
		}
		answers = append(answers, domain.Digest([]byte(resp.Text)))
		a, derr := decode(resp.Text)
		if derr != nil {
			violations = []string{derr.Error()}
			continue
		}
		var cs []checked
		if cs, violations = floor(ctx, p, a); len(violations) == 0 {
			accepted, acceptedAnswer = cs, a
		}
	}
	r, err := f.base(p, pr, req, resp)
	if err != nil {
		return nil, err
	}
	if acceptedAnswer == nil {
		r.Status = StatusFailed
		r.Failure = &Failure{Attempts: len(answers), Violations: violations, Answers: answers}
	} else {
		r.Status = StatusComplete
		r.NoHypothesisReason = strings.TrimSpace(acceptedAnswer.NoHypothesisReason)
		r.Hypotheses = hypotheses(p, r, accepted)
	}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("formalize: the record does not validate: %w", err)
	}
	return r, nil
}

// retryMessage is the next attempt's single user message: the task again,
// the rejected answer and its violations (a single message, so any exchange
// answerer can serve it).
func retryMessage(task string, attempt int, previous string, violations []string) string {
	var b strings.Builder
	b.WriteString(task)
	fmt.Fprintf(&b, "\n## Attempt %d: your previous answer was rejected\n\nIt violated these rules of the deterministic floor:\n\n", attempt)
	for _, v := range violations {
		b.WriteString("- " + v + "\n")
	}
	b.WriteString("\nAnswer again, as a complete JSON document, without these violations. The rejected answer was:\n\n")
	b.WriteString(previous)
	b.WriteString("\n")
	return b.String()
}

// base is the record's deterministic part and provenance.
func (f *Formalizer) base(p *Precomputed, pr *Prompt, req llm.Request, resp *llm.Response) (*Record, error) {
	if resp.Model == "" || resp.ModelVersion == "" || resp.GeneratedAt.IsZero() || resp.CallID == "" {
		return nil, errors.New("formalize: the answer does not report its model, model version, generation time and call id")
	}
	provider := f.Provider
	if provider == "" {
		provider = "anthropic"
	}
	inputs := append([]domain.EvidenceID(nil), p.item.Evidence...)
	for _, e := range p.Evidence {
		inputs = append(inputs, e.ID)
	}
	inputs = uniqEvidence(inputs)
	if len(inputs) == 0 {
		return nil, errors.New("formalize: the input cites no evidence")
	}
	at := resp.GeneratedAt.UTC()
	r := &Record{SchemaVersion: SchemaVersion, Investigation: p.Investigation, Assessment: p.Assessment.ID, ChangeItem: p.item.ID,
		Input: p.Digest(), Uncertainties: append([]Uncertainty(nil), p.Uncertainties...), StatementKind: domain.StatementUnverifiedProse,
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: Producer, Confidence: domain.ConfidenceLow,
			Model: resp.Model, ModelVersion: resp.ModelVersion, PromptVersion: pr.Version, PromptDigest: llm.PromptDigest(req),
			InputEvidence: inputs, GeneratedAt: &at, CallID: resp.CallID, Provider: provider}}
	r.ID = RecordID(r.Investigation, r.Assessment, r.Input, r.Provenance.CallID)
	return r, nil
}

// hypotheses turns the accepted answer into hypotheses. A rule keeps its
// checked spec; every uncertainty of the assessment travels with it (in
// ProposedVerification and, for the human reviewer, appended to the
// requested verification). An unrepresentable record keeps its claim,
// citations, conditions, limitation, proposed verification, the
// uncertainties it keeps and its partial rule.
func hypotheses(p *Precomputed, r *Record, cs []checked) []domain.Hypothesis {
	var all []string
	for _, u := range p.Uncertainties {
		all = append(all, u.Text)
	}
	var out []domain.Hypothesis
	for _, c := range cs {
		claim := strings.TrimSpace(c.h.Claim)
		var cites []domain.EvidenceID
		// a stated default's evidence travels with the hypothesis, so the
		// exported proposal (and a promoted fact) carries what establishes it
		for _, e := range append(append([]domain.Evidence(nil), c.cites...), c.defaultCites...) {
			cites = append(cites, e.ID)
		}
		cites = uniqEvidence(cites)
		prov := r.Provenance
		prov.InputEvidence = uniqEvidence(append(append([]domain.EvidenceID(nil), cites...), p.item.Evidence...))
		h := domain.Hypothesis{ID: domain.AgentRecordID(domain.HypothesisIDPrefix, p.item.ID, claim, prov.CallID), ChangeItem: p.item.ID,
			Statement: claim, Citations: cites, StatementKind: domain.StatementUnverifiedProse, Provenance: prov}
		pv := &domain.ProposedVerification{Assessment: p.Assessment.ID, Conditions: trimAll(c.h.Conditions)}
		if c.h.Kind == KindRule {
			spec := *c.spec
			open := unresolved(p, c.discharged)
			if len(open) > 0 {
				spec.RequestedVerification += " Unresolved in the investigation (kept by formalization): " + strings.Join(open, "; ") + "."
			}
			if len(c.discharged) > 0 {
				spec.RequestedVerification += " The rule states evidenced defaults; verify them with the field unset."
			}
			h.Mitigation = &spec
			pv.Method, pv.Uncertainties = spec.RequestedVerification, open
		} else {
			h.MitigationRejected = "not representable as a mitigation rule: " + strings.TrimSpace(c.h.Limitation)
			pv.Method, pv.Rule = strings.TrimSpace(c.h.ProposedVerification), c.rule
			for _, id := range uniqStrings(c.kept) {
				u, _ := p.uncertainty(id)
				pv.Uncertainties = append(pv.Uncertainties, u.Text)
			}
		}
		h.ProposedVerification = pv
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// unresolved are the uncertainties' texts, less the defaults a rule settles.
func unresolved(p *Precomputed, discharged []string) []string {
	settled := map[string]bool{}
	for _, id := range discharged {
		settled[id] = true
	}
	var out []string
	for _, u := range p.Uncertainties {
		if !settled[u.ID] {
			out = append(out, u.Text)
		}
	}
	return out
}

func trimAll(xs []string) []string {
	var out []string
	for _, x := range xs {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func uniqStrings(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func uniqEvidence(xs []domain.EvidenceID) []domain.EvidenceID {
	seen := map[domain.EvidenceID]bool{}
	var out []domain.EvidenceID
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
