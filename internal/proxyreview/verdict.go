package proxyreview

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/llm"
	"github.com/tdavison784/release-intelligence/internal/semantic"
)

// Provider is who serves the proxy model (the `claude -p` CLI's own login).
const Provider = "anthropic"

// Verdict is the proxy model's structured answer.
type Verdict struct {
	Action      string      `json:"action"`
	Labels      []string    `json:"labels,omitempty"`
	Reason      string      `json:"reason"`
	Citations   []string    `json:"citations"`
	Confidence  string      `json:"confidence"`
	DuplicateOf string      `json:"duplicateOf,omitempty"`
	Correction  *Correction `json:"correction,omitempty"`
}

// Correction carries the corrected aspects in the proposers' answer shape,
// so it goes through the same typed conversion as a proposal.
type Correction struct {
	Statement     string                 `json:"statement"`
	Subject       *semantic.AspectAnswer `json:"subject,omitempty"`
	Change        *semantic.AspectAnswer `json:"change,omitempty"`
	Applicability *semantic.AspectAnswer `json:"applicability,omitempty"`
	Consequence   *semantic.AspectAnswer `json:"consequence,omitempty"`
}

// Response is one answered request, written by scripts/proxy-review.sh:
// the verdict plus what the CLI envelope reported about the call.
type Response struct {
	Format       string `json:"format"`
	ItemID       string `json:"itemId"`
	PromptDigest string `json:"promptDigest"`
	// Model / ModelVersion as the CLI envelope reports them (modelUsage keys).
	Model        string `json:"model"`
	ModelVersion string `json:"modelVersion"`
	// CallID is the envelope's session id: one fresh session per call.
	CallID string `json:"callId"`
	// StartedAt / DecidedAt bracket the model call (the decision's time).
	StartedAt  time.Time       `json:"startedAt"`
	DecidedAt  time.Time       `json:"decidedAt"`
	DurationMs int64           `json:"durationMs,omitempty"`
	CostUSD    float64         `json:"costUSD,omitempty"`
	Output     json.RawMessage `json:"output"`
}

type obj = map[string]any

func enum(xs []string) obj { return obj{"type": "string", "enum": xs} }

var wrongLabels = []string{
	string(domain.LabelWrongSubject), string(domain.LabelWrongChangeType), string(domain.LabelWrongApplicability),
	string(domain.LabelWrongConsequence), string(domain.LabelWrongClassification),
}

// verdictSchema is the structured-output schema of one request: actions for
// the question type, citations narrowed to the evidence shown, duplicateOf
// narrowed to the facts shown, and the correction in the proposers' typed
// aspect shapes (every enum from the domain via semantic.AnswerSchema).
func verdictSchema(q domain.QuestionType, input []domain.EvidenceID, facts []string) (json.RawMessage, error) {
	cites := make([]string, len(input))
	for i, id := range input {
		cites[i] = string(id)
	}
	props := obj{
		"reason":     obj{"type": "string"},
		"citations":  obj{"type": "array", "items": enum(cites)},
		"confidence": enum([]string{string(domain.ConfidenceMedium), string(domain.ConfidenceLow)}),
	}
	s := obj{"type": "object", "additionalProperties": false, "required": []string{"action", "reason", "citations", "confidence"}, "properties": props}
	if q == domain.QuestionEvidenceSufficiency {
		props["action"] = enum([]string{ActionEvidenceSufficient, string(domain.ActionNeedMoreEvidence), string(domain.ActionDefer)})
		return json.Marshal(s)
	}
	props["action"] = enum([]string{string(domain.ActionAccept), string(domain.ActionCorrect), string(domain.ActionReject),
		string(domain.ActionNeedMoreEvidence), string(domain.ActionDefer)})
	props["labels"] = obj{"type": "array", "items": enum(wrongLabels)}
	if len(facts) > 0 {
		props["duplicateOf"] = enum(append(append([]string{}, facts...), "none"))
	}
	var full obj
	if err := json.Unmarshal(semantic.AnswerSchema(domain.TaskFull), &full); err != nil {
		return nil, fmt.Errorf("proxyreview: answer schema: %w", err)
	}
	fullProps, _ := full["properties"].(map[string]any)
	cprops := obj{"statement": obj{"type": "string"}}
	required := []string{"statement"}
	for _, x := range domain.QuestionAspects(q) {
		as, ok := fullProps[string(x)].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("proxyreview: no schema for aspect %s", x)
		}
		alts, _ := as["anyOf"].([]any)
		if len(alts) == 0 {
			return nil, fmt.Errorf("proxyreview: aspect %s schema has no asserted variant", x)
		}
		cprops[string(x)] = alts[0] // the asserted variant: a correction never abstains
		required = append(required, string(x))
	}
	props["correction"] = obj{"type": "object", "additionalProperties": false, "required": required, "properties": cprops}
	if defs, ok := full["$defs"]; ok && containsAspect(domain.QuestionAspects(q), domain.AspectApplicability) {
		s["$defs"] = defs
	}
	return json.Marshal(s)
}

// DecodeVerdict checks the output against the request's schema and decodes it.
func DecodeVerdict(req *Request, output json.RawMessage) (*Verdict, error) {
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(req.Request.JSONSchema))
	if err != nil {
		return nil, fmt.Errorf("verdict schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	url := "https://ri.dev/schemas/proxy-verdict/" + strings.TrimPrefix(req.PromptDigest, "sha256:") + ".json"
	if err := c.AddResource(url, inst); err != nil {
		return nil, err
	}
	sch, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("verdict schema: %w", err)
	}
	out, err := jsonschema.UnmarshalJSON(bytes.NewReader(output))
	if err != nil {
		return nil, fmt.Errorf("verdict is not JSON: %v", err)
	}
	if err := sch.Validate(out); err != nil {
		return nil, fmt.Errorf("verdict does not match the schema: %s", shorten(oneLine(err.Error()), 600))
	}
	var v Verdict
	if err := json.Unmarshal(output, &v); err != nil {
		return nil, fmt.Errorf("verdict: %v", err)
	}
	if strings.TrimSpace(v.Reason) == "" {
		return nil, errors.New("verdict has no reason")
	}
	return &v, nil
}

// Reviewer is the reviewer name of the proxy's decisions.
func Reviewer(model string) string { return "proxy:" + model }

// Decision turns a response into ONE proxy ReviewDecision for the item, after
// checking that the response answers this request and that the item is
// unchanged since the prompt was built. Every refusal is an error naming why
// (the caller records it; nothing is retried or invented).
func Decision(rc *knowledge.ReviewContext, req *Request, resp *Response) (domain.ReviewDecision, *Verdict, error) {
	var d domain.ReviewDecision
	it := rc.Item
	switch {
	case req.Format != RequestFormat:
		return d, nil, fmt.Errorf("request format %q", req.Format)
	case resp.Format != ResponseFormat:
		return d, nil, fmt.Errorf("response format %q", resp.Format)
	case req.PromptDigest != llm.PromptDigest(req.Request):
		return d, nil, errors.New("request file was edited: its prompt digest does not match its prompt")
	case resp.PromptDigest != req.PromptDigest:
		return d, nil, fmt.Errorf("response answers prompt %s, the request is %s", resp.PromptDigest, req.PromptDigest)
	case resp.ItemID != req.ItemID || req.ItemID != it.ID:
		return d, nil, fmt.Errorf("response/request/item ids differ (%s, %s, %s)", resp.ItemID, req.ItemID, it.ID)
	case it.Proposed.Digest() != req.ProposedDigest:
		return d, nil, fmt.Errorf("item %s changed since the prompt was built", it.ID)
	case it.Status != domain.ReviewPending:
		return d, nil, fmt.Errorf("item %s is %s, not pending", it.ID, it.Status)
	case strings.TrimSpace(resp.Model) == "" || strings.TrimSpace(resp.ModelVersion) == "":
		return d, nil, errors.New("response does not report the model and model version")
	case strings.TrimSpace(resp.CallID) == "":
		return d, nil, errors.New("response carries no call id")
	case resp.StartedAt.IsZero() || resp.DecidedAt.IsZero() || resp.DecidedAt.Before(resp.StartedAt):
		return d, nil, errors.New("response call times missing or inverted")
	}
	v, err := DecodeVerdict(req, resp.Output)
	if err != nil {
		return d, nil, err
	}
	gen := resp.DecidedAt.UTC()
	d = domain.ReviewDecision{
		ReviewItemID: it.ID, Reviewer: Reviewer(resp.Model), ReviewerKind: domain.ReviewerProxy,
		Reason: oneLine(v.Reason), StartedAt: resp.StartedAt.UTC(), DecidedAt: gen,
		ProxyProvenance: &domain.Provenance{
			Method: domain.MethodAI, Producer: Producer, Provider: Provider,
			Confidence: domain.Confidence(v.Confidence), Model: resp.Model, ModelVersion: resp.ModelVersion,
			PromptVersion: req.PromptVersion, PromptDigest: req.PromptDigest, CallID: resp.CallID,
			InputEvidence: append([]domain.EvidenceID(nil), req.InputEvidence...), GeneratedAt: &gen,
		},
	}
	orig := it.Proposed
	verify := domain.QuestionAspects(it.QuestionType)
	switch v.Action {
	case ActionEvidenceSufficient:
		if it.QuestionType != domain.QuestionEvidenceSufficiency {
			return d, v, fmt.Errorf("%s is a gate verdict, item is %s", v.Action, it.QuestionType)
		}
		d.Action = domain.ActionDefer
		d.Reason = "evidence sufficient (proxy gate verdict; the open aspects need proposals): " + d.Reason
	case string(domain.ActionNeedMoreEvidence):
		d.Action = domain.ActionNeedMoreEvidence
		d.Labels = []domain.FeedbackLabel{domain.LabelInsufficientEvidence}
	case string(domain.ActionDefer):
		d.Action = domain.ActionDefer
	case string(domain.ActionAccept):
		d.Action, d.Original = domain.ActionAccept, &orig
		d.Labels = []domain.FeedbackLabel{domain.LabelAccepted}
	case string(domain.ActionReject):
		d.Action, d.Original = domain.ActionReject, &orig
		if v.DuplicateOf != "" && v.DuplicateOf != "none" {
			d.DuplicateOf = v.DuplicateOf
			d.Labels = []domain.FeedbackLabel{domain.LabelDuplicate}
		} else {
			d.Labels = []domain.FeedbackLabel{domain.LabelRejected}
		}
		d.Labels = append(d.Labels, wrongOf(v.Labels, verify)...)
	case string(domain.ActionCorrect):
		corrected, err := correctedAssertion(rc, req, resp, v)
		if err != nil {
			return d, v, fmt.Errorf("correction refused: %w", err)
		}
		d.Action, d.Original, d.Corrected = domain.ActionCorrect, &orig, corrected
		if corrected.Digest() == orig.Digest() {
			// prose-only (contract-5): the typed assertion was right, so this is
			// not a model error and carries no wrong-* label
			d.Labels = []domain.FeedbackLabel{domain.LabelCorrected, domain.LabelImprovedStatement}
		} else {
			d.Labels = append([]domain.FeedbackLabel{domain.LabelCorrected}, mergeWrong(changedLabels(orig, *corrected, verify), wrongOf(v.Labels, verify))...)
		}
	default:
		return d, v, fmt.Errorf("unknown action %q", v.Action)
	}
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	if err := d.Validate(); err != nil {
		return d, v, err
	}
	return d, v, nil
}

// taskFor maps a question to the proposal task whose aspects it verifies.
func taskFor(q domain.QuestionType) domain.ProposalTask {
	switch q {
	case domain.QuestionSemanticMapping:
		return domain.TaskSemanticMapping
	case domain.QuestionApplicability:
		return domain.TaskApplicability
	case domain.QuestionConsequence, domain.QuestionClassification:
		return domain.TaskConsequence
	case domain.QuestionRelationship:
		return domain.TaskRelationship
	}
	return ""
}

// correctedAssertion converts the correction through the proposers' typed
// path (semantic.ProposalFromAnswer: enum checks, exposed class from the
// kind, citation checks) and applies it to the proposed assertion.
func correctedAssertion(rc *knowledge.ReviewContext, req *Request, resp *Response, v *Verdict) (*domain.SemanticAssertion, error) {
	c := v.Correction
	if c == nil {
		return nil, errors.New("action correct without a correction")
	}
	task := taskFor(rc.Item.QuestionType)
	if task == "" {
		return nil, fmt.Errorf("question %s cannot be corrected", rc.Item.QuestionType)
	}
	candEv := map[string]bool{}
	for _, id := range req.CandidateEvidence {
		candEv[string(id)] = true
	}
	var cites []string
	for _, id := range v.Citations {
		if candEv[id] {
			cites = append(cites, id)
		}
	}
	ans := &semantic.Answer{Statement: c.Statement, Subject: c.Subject, Change: c.Change, Applicability: c.Applicability,
		Consequence: c.Consequence, Citations: cites, Confidence: v.Confidence}
	if ans.Citations == nil {
		ans.Citations = []string{}
	}
	p, err := semantic.ProposalFromAnswer(rc.Candidate, task, ans, semantic.AnswerMeta{
		Provider: Provider, Model: resp.Model, ModelVersion: resp.ModelVersion, PromptVersion: req.PromptVersion,
		PromptDigest: req.PromptDigest, CallID: resp.CallID, Input: req.CandidateEvidence, GeneratedAt: resp.DecidedAt,
	})
	if err != nil {
		return nil, err
	}
	out := rc.Item.Proposed
	for _, x := range domain.QuestionAspects(rc.Item.QuestionType) {
		if !p.Assertion.Has(x) {
			return nil, fmt.Errorf("the correction does not assert %s", x)
		}
		switch x {
		case domain.AspectSubject:
			out.Subject = p.Assertion.Subject
		case domain.AspectChange:
			out.Change = p.Assertion.Change
		case domain.AspectApplicability:
			out.Applicability = p.Assertion.Applicability
		case domain.AspectConsequence:
			out.Consequence = p.Assertion.Consequence
		}
	}
	if s := strings.TrimSpace(p.Assertion.Statement); s != "" {
		out.Statement = s
	}
	// contract-5: a correction may change only the consequence prose
	// (statement/remediation), which aspect digests ignore; anything else that
	// changes no digest is a no-op.
	if out.Digest() == rc.Item.Proposed.Digest() && consequenceProse(out) == consequenceProse(rc.Item.Proposed) {
		return nil, errors.New("the correction changes no aspect and no consequence statement/remediation")
	}
	if err := out.Validate(false); err != nil {
		return nil, err
	}
	return &out, nil
}

// changedLabels derives the wrong-* labels from what the correction changed.
func changedLabels(orig, corr domain.SemanticAssertion, verify []domain.Aspect) []domain.FeedbackLabel {
	var out []domain.FeedbackLabel
	for _, x := range verify {
		if orig.AspectDigest(x) == corr.AspectDigest(x) {
			continue
		}
		switch x {
		case domain.AspectSubject:
			out = append(out, domain.LabelWrongSubject)
		case domain.AspectChange:
			out = append(out, domain.LabelWrongChangeType)
		case domain.AspectApplicability:
			out = append(out, domain.LabelWrongApplicability)
		case domain.AspectConsequence:
			out = append(out, domain.LabelWrongConsequence)
			if orig.Consequence == nil || corr.Consequence == nil || orig.Consequence.ExposedClass != corr.Consequence.ExposedClass {
				out = append(out, domain.LabelWrongClassification)
			}
		}
	}
	return out
}

// wrongOf keeps the model's wrong-* labels that name an aspect the question verifies.
func wrongOf(ls []string, verify []domain.Aspect) []domain.FeedbackLabel {
	var out []domain.FeedbackLabel
	for _, l := range ls {
		var x domain.Aspect
		switch domain.FeedbackLabel(l) {
		case domain.LabelWrongSubject:
			x = domain.AspectSubject
		case domain.LabelWrongChangeType:
			x = domain.AspectChange
		case domain.LabelWrongApplicability:
			x = domain.AspectApplicability
		case domain.LabelWrongConsequence, domain.LabelWrongClassification:
			x = domain.AspectConsequence
		default:
			continue
		}
		if containsAspect(verify, x) {
			out = mergeWrong(out, []domain.FeedbackLabel{domain.FeedbackLabel(l)})
		}
	}
	return out
}

func mergeWrong(a, b []domain.FeedbackLabel) []domain.FeedbackLabel {
	seen := map[domain.FeedbackLabel]bool{}
	var out []domain.FeedbackLabel
	for _, l := range append(append([]domain.FeedbackLabel(nil), a...), b...) {
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}

// consequenceProse is the consequence's free text, which aspect digests ignore
// (the same notion as the domain's prose-only correction check, contract-5).
func consequenceProse(a domain.SemanticAssertion) string {
	if a.Consequence == nil {
		return ""
	}
	return a.Consequence.Statement + "\x00" + a.Consequence.Remediation
}
