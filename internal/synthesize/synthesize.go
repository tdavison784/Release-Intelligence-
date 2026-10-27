package synthesize

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// DefaultModel is the synthesis model unless configured otherwise.
const DefaultModel = "claude-opus-5-5"

// MaxAttempts bounds the calls per judgement: the first answer and two
// retries, each told the previous answer's violations.
const MaxAttempts = 3

//go:embed prompts
var prompts embed.FS

// Prompt is one versioned synthesis prompt.
type Prompt struct {
	Version string
	System  string
	Task    string
	Schema  json.RawMessage
}

// LoadPrompt loads a prompt version ("synthesize/v1").
func LoadPrompt(version string) (*Prompt, error) {
	dir := "prompts/" + version
	read := func(name string) (string, error) {
		b, err := prompts.ReadFile(dir + "/" + name)
		return string(b), err
	}
	sys, err := read("system.md")
	if err != nil {
		return nil, fmt.Errorf("synthesize: unknown prompt %q", version)
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
		return nil, fmt.Errorf("synthesize: prompt %s schema: %w", version, err)
	}
	return &Prompt{Version: version, System: sys, Task: task, Schema: compact}, nil
}

// Digest pins the prompt templates and schema (the rendered request's digest
// is recorded per call as the judgement's promptDigest).
func (p *Prompt) Digest() string {
	return domain.Digest([]byte(p.System + "\x00" + p.Task + "\x00" + string(p.Schema)))
}

// Synthesizer makes the judgement: one structured call through Client
// (normally llm.Cache over an exchange or `claude -p`), checked by the floor
// and retried within MaxAttempts.
type Synthesizer struct {
	Client   llm.Client
	Model    string // default DefaultModel
	Provider string // default "anthropic"
	Prompt   string // default PromptVersion
	// MaxTokens bounds each answer (default 16000).
	MaxTokens int
}

// Synthesize pre-computes the input, asks the model and returns the
// judgement: complete when an answer passed the floor, failed (recorded,
// never repaired) when none did within the bound. An error means no answer
// was obtained at all (a pending exchange, an unreachable model).
func (s *Synthesizer) Synthesize(ctx context.Context, in Input) (*domain.UpgradeJudgement, error) {
	pc, err := Precompute(in)
	if err != nil {
		return nil, err
	}
	return s.Judge(ctx, pc)
}

// Judge runs the call and the floor over an already computed input.
func (s *Synthesizer) Judge(ctx context.Context, pc *Precomputed) (*domain.UpgradeJudgement, error) {
	if s.Client == nil {
		return nil, errors.New("synthesize: no model client is configured")
	}
	pv := s.Prompt
	if pv == "" {
		pv = PromptVersion
	}
	pr, err := LoadPrompt(pv)
	if err != nil {
		return nil, err
	}
	model := s.Model
	if model == "" {
		model = DefaultModel
	}
	maxTokens := s.MaxTokens
	if maxTokens == 0 {
		maxTokens = 16000
	}
	input, err := json.MarshalIndent(pc, "", "  ")
	if err != nil {
		return nil, err
	}
	task := pr.Task + string(input) + "\n"
	var answers []string
	var violations []string
	var resp *llm.Response
	var req llm.Request
	var accepted *Answer
	for attempt := 1; attempt <= MaxAttempts && accepted == nil; attempt++ {
		msg := task
		if attempt > 1 {
			msg = retryMessage(task, attempt, resp.Text, violations)
		}
		req = llm.Request{Model: model, System: pr.System, Messages: []llm.Message{{Role: "user", Content: msg}},
			MaxTokens: maxTokens, JSONSchema: pr.Schema}
		resp, err = s.Client.Complete(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("synthesize: attempt %d: %w", attempt, err)
		}
		answers = append(answers, domain.Digest([]byte(resp.Text)))
		a, derr := decode(resp.Text)
		if derr != nil {
			violations = []string{derr.Error()}
			continue
		}
		if violations = Floor(pc, a); len(violations) == 0 {
			accepted = a
		}
	}
	j, err := s.base(pc, pr, req, resp)
	if err != nil {
		return nil, err
	}
	if accepted == nil {
		j.Status = domain.JudgementFailed
		j.Failure = &domain.JudgementFailure{Attempts: len(answers), Violations: violations, Answers: answers}
	} else {
		apply(j, pc, accepted)
	}
	if err := j.Validate(); err != nil {
		return nil, fmt.Errorf("synthesize: the judgement does not validate: %w", err)
	}
	return j, nil
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

// base is the judgement's deterministic part and provenance.
func (s *Synthesizer) base(pc *Precomputed, pr *Prompt, req llm.Request, resp *llm.Response) (*domain.UpgradeJudgement, error) {
	if resp.Model == "" || resp.ModelVersion == "" || resp.GeneratedAt.IsZero() || resp.CallID == "" {
		return nil, errors.New("synthesize: the answer does not report its model, model version, generation time and call id")
	}
	provider := s.Provider
	if provider == "" {
		provider = "anthropic"
	}
	var inputs []domain.EvidenceID
	for _, e := range pc.Evidence {
		inputs = append(inputs, e.ID)
	}
	if len(inputs) == 0 {
		return nil, errors.New("synthesize: the input cites no evidence")
	}
	at := resp.GeneratedAt.UTC()
	u := pc.Upgrade
	j := &domain.UpgradeJudgement{
		SchemaVersion: domain.UpgradeJudgementSchemaVersion, Investigation: u.Investigation,
		Product: u.Product, From: u.From, To: u.To, Environment: u.Environment,
		Target: pc.Target, Locations: pc.Locations, ResidualRisks: pc.ResidualRisks,
		Findings: pc.Findings, Interactions: pc.Interactions, Backout: pc.Backout,
		Evidence: pc.external, Input: pc.Digest(), StatementKind: domain.StatementUnverifiedProse,
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: Producer, Confidence: domain.ConfidenceLow,
			Model: resp.Model, ModelVersion: resp.ModelVersion, PromptVersion: pr.Version, PromptDigest: llm.PromptDigest(req),
			InputEvidence: inputs, GeneratedAt: &at, CallID: resp.CallID, Provider: provider},
	}
	j.ID = domain.UpgradeJudgementID(j.Investigation, j.Input, j.Provenance.CallID)
	// copies, so the judgement never aliases the input
	j.Findings = append([]domain.JudgementFinding(nil), pc.Findings...)
	j.Interactions = append([]domain.CrossChange(nil), pc.Interactions...)
	j.ResidualRisks = append([]domain.ResidualRisk(nil), pc.ResidualRisks...)
	return j, nil
}

// apply copies an accepted answer into the judgement. The deterministic
// parts stay as computed; the answer adds the committed recommendation, the
// chosen target, the steps, tests, model-named risks, comments and summary.
func apply(j *domain.UpgradeJudgement, pc *Precomputed, a *Answer) {
	j.Status = domain.JudgementComplete
	j.Recommendation, j.Headline, j.Scope, j.Rationale = a.Recommendation, strings.TrimSpace(a.Headline), strings.TrimSpace(a.Scope), strings.TrimSpace(a.Rationale)
	j.Preconditions = trimAll(a.Preconditions)
	if o, _ := pc.option(a.TargetVersion); o != nil {
		v := o.Version
		j.Target.Chosen = &v
		j.Target.Changed = !v.Equal(j.Target.Proposed)
	}
	for i, c := range a.Changes {
		j.Changes = append(j.Changes, domain.JudgementChange{Order: i + 1, Kind: c.Kind, Summary: strings.TrimSpace(c.Summary),
			ChangeItems: c.ChangeItems, Location: c.Location, Proposed: c.Location == ""})
	}
	seen := map[string]bool{}
	addTest := func(t domain.JudgementTest) {
		t.ID = domain.JudgementTestID(t.Test, t.Expected)
		if !seen[t.ID] {
			seen[t.ID] = true
			j.Validation = append(j.Validation, t)
		}
	}
	for _, t := range a.Validation {
		addTest(domain.JudgementTest{Test: strings.TrimSpace(t.Test), Expected: strings.TrimSpace(t.Expected), Basis: t.Basis, ChangeItems: t.ChangeItems})
	}
	if j.Target.Changed {
		// The transition a changed target needs is deterministic: it is
		// always required (mission C2), whatever the answer listed.
		test := transitionTest(j.From, *j.Target.Chosen)
		for _, c := range pc.Tests {
			if c.Test == test {
				addTest(domain.JudgementTest{Test: test, Expected: "Every change of the new transition is classified, with no new hard finding.", Basis: []string{c.ID}})
			}
		}
	}
	for _, r := range a.ResidualRisks {
		st := strings.TrimSpace(r.Statement)
		j.ResidualRisks = append(j.ResidualRisks, domain.ResidualRisk{ID: domain.ResidualRiskID(domain.RiskModel, r.ChangeItem, st),
			Source: domain.RiskModel, ChangeItem: r.ChangeItem, Statement: st, Refs: r.Refs})
	}
	comments := map[string]string{}
	for _, c := range a.FindingComments {
		comments[c.ID] = joinComment(comments[c.ID], c.Comment)
	}
	for i := range j.Findings {
		j.Findings[i].Comment = comments[j.Findings[i].ID]
	}
	xc := map[string]string{}
	for _, c := range a.InteractionComments {
		xc[c.ID] = joinComment(xc[c.ID], c.Comment)
	}
	for i := range j.Interactions {
		j.Interactions[i].Comment = xc[j.Interactions[i].ID]
	}
	for _, v := range a.Verified {
		var cites []domain.EvidenceID
		for _, c := range v.Citations {
			cites = append(cites, domain.EvidenceID(c))
		}
		j.Summary.Verified = append(j.Summary.Verified, domain.AssessmentFinding{Statement: strings.TrimSpace(v.Statement), Citations: cites})
	}
	j.Summary.Inferred, j.Summary.Unknown = trimAll(a.Inferred), trimAll(a.Unknown)
	j.Backout.Notes = strings.TrimSpace(a.BackoutNotes)
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

func joinComment(a, b string) string {
	b = strings.TrimSpace(b)
	if a == "" {
		return b
	}
	return a + " " + b
}
