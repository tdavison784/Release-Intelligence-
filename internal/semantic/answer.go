package semantic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ProposerProducer is the producer string of every proposal.
const ProposerProducer = "semantic.proposer@v1"

// Answer is the typed answer of one task, as the schema defines it. A typed
// model (a "System One" proposer) can produce it directly and go through
// ProposalFromAnswer, sharing every check with the LLM path.
type Answer struct {
	Statement      string        `json:"statement,omitempty"`
	SuggestedClass string        `json:"suggestedClass,omitempty"`
	Subject        *AspectAnswer `json:"subject,omitempty"`
	Change         *AspectAnswer `json:"change,omitempty"`
	Applicability  *AspectAnswer `json:"applicability,omitempty"`
	Consequence    *AspectAnswer `json:"consequence,omitempty"`
	DuplicateOf    string        `json:"duplicateOf,omitempty"`
	Reason         string        `json:"reason,omitempty"`
	Citations      []string      `json:"citations"`
	Confidence     string        `json:"confidence"`
}

// AspectAnswer is one aspect: asserted (with the aspect's typed fields) or
// undetermined (with a reason). One struct carries the union of the fields;
// the schema decides which belong to which aspect.
type AspectAnswer struct {
	Determination string `json:"determination"` // "asserted" | "undetermined"
	Reason        string `json:"reason,omitempty"`
	// subject identity (also replacedBy)
	SubjectIdentity
	// change
	Type string `json:"type,omitempty"`
	// Before/After keep the raw literal: an explicit null ("from nil") is a
	// value, an absent field is not stated.
	Before     json.RawMessage  `json:"before,omitempty"`
	After      json.RawMessage  `json:"after,omitempty"`
	ReplacedBy *SubjectIdentity `json:"replacedBy,omitempty"`
	// applicability
	Exposure *ConditionAnswer `json:"exposure,omitempty"`
	Overlap  *ConditionAnswer `json:"overlap,omitempty"`
	// consequence (its kind is SubjectIdentity.Kind: the same JSON key)
	Statement   string `json:"statement,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	Severity    string `json:"severity,omitempty"`
}

// SubjectIdentity is a subject without its product (the candidate's).
type SubjectIdentity struct {
	Family    string `json:"family,omitempty"`
	Group     string `json:"group,omitempty"`
	Version   string `json:"version,omitempty"`
	Kind      string `json:"kind,omitempty"` // also the consequence kind
	Path      string `json:"path,omitempty"`
	Name      string `json:"name,omitempty"`
	Component string `json:"component,omitempty"`
}

// ConditionAnswer is one condition node; values are literal scalars.
type ConditionAnswer struct {
	Op        string            `json:"op"`
	Of        []ConditionAnswer `json:"of,omitempty"`
	Group     string            `json:"group,omitempty"`
	Version   string            `json:"version,omitempty"`
	Kind      string            `json:"kind,omitempty"`
	Name      string            `json:"name,omitempty"`
	Path      string            `json:"path,omitempty"`
	Component string            `json:"component,omitempty"`
	State     string            `json:"state,omitempty"`
	Values    []json.RawMessage `json:"values,omitempty"`
	Pattern   string            `json:"pattern,omitempty"`
	Separator string            `json:"separator,omitempty"`
	Range     string            `json:"range,omitempty"`
	Reason    string            `json:"reason,omitempty"`
	Needed    string            `json:"needed,omitempty"`
}

var (
	schemaMu    sync.Mutex
	schemaCache = map[domain.ProposalTask]*jsonschema.Schema{}
)

func compiledSchema(task domain.ProposalTask) (*jsonschema.Schema, error) {
	schemaMu.Lock()
	defer schemaMu.Unlock()
	if s := schemaCache[task]; s != nil {
		return s, nil
	}
	raw := AnswerSchema(task)
	if raw == nil {
		return nil, fmt.Errorf("no answer schema for task %q", task)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	url := "https://ri.dev/schemas/semantic-answer/" + string(task) + "/v1.json"
	if err := c.AddResource(url, inst); err != nil {
		return nil, err
	}
	s, err := c.Compile(url)
	if err != nil {
		return nil, err
	}
	schemaCache[task] = s
	return s, nil
}

// DecodeAnswer checks the answer text against the task's schema (gateways
// that do not enforce structured output are covered here) and decodes it.
// Fenced JSON is tolerated.
func DecodeAnswer(task domain.ProposalTask, text string) (*Answer, error) {
	s, err := compiledSchema(task)
	if err != nil {
		return nil, fmt.Errorf("answer schema: %w", err)
	}
	t := strings.TrimSpace(text)
	if fenced, ok := unwrapFence(t); ok {
		t = fenced
	}
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(t))
	if err != nil {
		return nil, fmt.Errorf("answer is not JSON: %v", err)
	}
	if err := s.Validate(inst); err != nil {
		return nil, fmt.Errorf("answer does not match the schema: %s", shorten(oneLine(err.Error()), 600))
	}
	var a Answer
	dec := json.NewDecoder(strings.NewReader(t))
	if err := dec.Decode(&a); err != nil {
		return nil, fmt.Errorf("answer: %v", err)
	}
	return &a, nil
}

func unwrapFence(s string) (string, bool) {
	const fence = "```"
	if !strings.HasPrefix(s, fence) {
		return "", false
	}
	body := s[len(fence):]
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		body = body[i+1:]
	}
	if j := strings.LastIndex(body, fence); j >= 0 {
		body = body[:j]
	}
	return strings.TrimSpace(body), true
}

// AnswerMeta is what the caller knows about the answer besides its content.
type AnswerMeta struct {
	Provider      string
	Model         string // the model that answered (never assumed from the request)
	ModelVersion  string
	PromptVersion string
	PromptDigest  string
	Input         []domain.EvidenceID // evidence ids shown
	KnownFacts    []string            // fact ids shown (duplicate task)
	GeneratedAt   time.Time
}

// ProposalFromAnswer turns a decoded answer into a proposal and validates it
// against its candidate. Every refusal is an error naming why; there is no
// path from an answer to an action-required or not-affected suggestion, a
// high confidence, an exposed class other than the one the consequence kind
// implies, or a citation the prompt did not show.
func ProposalFromAnswer(c domain.SemanticCandidate, task domain.ProposalTask, a *Answer, m AnswerMeta) (*domain.SemanticProposal, error) {
	if a == nil {
		return nil, fmt.Errorf("no answer")
	}
	if strings.TrimSpace(m.Model) == "" || strings.TrimSpace(m.ModelVersion) == "" {
		return nil, fmt.Errorf("the answer does not report the model and model version that produced it")
	}
	if strings.TrimSpace(m.Provider) == "" {
		return nil, fmt.Errorf("provider is required")
	}
	if m.GeneratedAt.IsZero() {
		return nil, fmt.Errorf("generation time unknown")
	}
	conf := domain.Confidence(a.Confidence)
	if conf != domain.ConfidenceMedium && conf != domain.ConfidenceLow {
		return nil, fmt.Errorf("confidence %q refused: a model reports low or medium", a.Confidence)
	}
	switch domain.ImpactClass(a.SuggestedClass) {
	case "", domain.ImpactReviewRequired, domain.ImpactInformational, domain.ImpactUnknown:
	default:
		return nil, fmt.Errorf("suggestedClass %q refused: a model may suggest review-required, informational or unknown only", a.SuggestedClass)
	}
	if err := checkLengths(a); err != nil {
		return nil, err
	}
	shown := map[string]bool{}
	for _, id := range m.Input {
		shown[string(id)] = true
	}
	var cites []domain.EvidenceID
	seen := map[string]bool{}
	for _, id := range a.Citations {
		if !shown[id] {
			return nil, fmt.Errorf("cites %q, which the prompt did not show (hallucinated or foreign citation)", id)
		}
		if seen[id] {
			continue // a repeated id carries no extra meaning
		}
		seen[id] = true
		cites = append(cites, domain.EvidenceID(id))
	}

	p := domain.SemanticProposal{
		CandidateID:    c.ID,
		Task:           task,
		Provider:       m.Provider,
		SuggestedClass: domain.ImpactClass(a.SuggestedClass),
		Citations:      cites,
	}
	var undet []string
	if task == domain.TaskDuplicate {
		switch {
		case a.DuplicateOf == "" || a.DuplicateOf == "none":
		case !containsString(m.KnownFacts, a.DuplicateOf):
			return nil, fmt.Errorf("duplicateOf %q is not one of the known facts shown", a.DuplicateOf)
		default:
			p.DuplicateOf = a.DuplicateOf
		}
		p.Assertion.Statement = strings.TrimSpace(a.Reason)
		if p.DuplicateOf == "" {
			p.Assertion.Statement = ""
			if len(p.Citations) == 0 {
				p.Citations = nil
			}
		}
	} else {
		p.Assertion.Statement = oneLine(a.Statement)
		parts := map[domain.Aspect]*AspectAnswer{
			domain.AspectSubject: a.Subject, domain.AspectChange: a.Change,
			domain.AspectApplicability: a.Applicability, domain.AspectConsequence: a.Consequence,
		}
		for _, x := range domain.TaskAspects(task) {
			aa := parts[x]
			if aa == nil {
				return nil, fmt.Errorf("aspect %s is missing (assert it or mark it undetermined)", x)
			}
			switch aa.Determination {
			case "undetermined":
				r := oneLine(aa.Reason)
				if r == "" {
					return nil, fmt.Errorf("aspect %s is undetermined without a reason", x)
				}
				p.Undetermined = append(p.Undetermined, x)
				undet = append(undet, string(x)+": "+r)
			case "asserted":
				if err := assertAspect(&p.Assertion, x, aa, c.Product); err != nil {
					return nil, fmt.Errorf("aspect %s: %w", x, err)
				}
			default:
				return nil, fmt.Errorf("aspect %s: unknown determination %q", x, aa.Determination)
			}
		}
		for k, aa := range parts {
			if aa != nil && !containsAspect(domain.TaskAspects(task), k) {
				return nil, fmt.Errorf("answers %s, outside task %s", k, task)
			}
		}
		if p.Assertion.Empty() {
			p.Assertion.Statement = ""
			if len(p.Citations) == 0 {
				p.Citations = nil
			}
		}
	}
	p.UndeterminedReason = strings.Join(undet, "; ")
	gen := m.GeneratedAt.UTC().Truncate(time.Second)
	p.Provenance = domain.Provenance{
		Method:        domain.MethodAI,
		Producer:      ProposerProducer,
		Rule:          "task:" + string(task),
		Confidence:    conf,
		Model:         m.Model,
		ModelVersion:  m.ModelVersion,
		PromptVersion: m.PromptVersion,
		PromptDigest:  m.PromptDigest,
		InputEvidence: append([]domain.EvidenceID(nil), m.Input...),
		GeneratedAt:   &gen,
	}
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	if err := p.ValidateAgainst(c); err != nil {
		return nil, fmt.Errorf("proposal refused: %s", shorten(oneLine(err.Error()), 800))
	}
	return &p, nil
}

func checkLengths(a *Answer) error {
	if len(a.Statement) > maxStatementLen {
		return fmt.Errorf("statement longer than %d characters", maxStatementLen)
	}
	if len(a.Reason) > maxReasonLen*2 {
		return fmt.Errorf("reason longer than %d characters", maxReasonLen*2)
	}
	if len(a.Citations) > maxCitations {
		return fmt.Errorf("%d citations (at most %d)", len(a.Citations), maxCitations)
	}
	for _, aa := range []*AspectAnswer{a.Subject, a.Change, a.Applicability, a.Consequence} {
		if aa == nil {
			continue
		}
		if len(aa.Reason) > maxReasonLen {
			return fmt.Errorf("undetermined reason longer than %d characters", maxReasonLen)
		}
		if len(aa.Statement) > maxConsequenceLen || len(aa.Remediation) > maxConsequenceLen {
			return fmt.Errorf("consequence statement/remediation longer than %d characters", maxConsequenceLen)
		}
	}
	return nil
}

func assertAspect(as *domain.SemanticAssertion, x domain.Aspect, aa *AspectAnswer, product domain.ProductID) error {
	switch x {
	case domain.AspectSubject:
		s := subjectOf(aa.SubjectIdentity, product)
		as.Subject = &s
	case domain.AspectChange:
		ch := domain.ChangeSpec{Type: domain.ChangeKind(aa.Type)}
		raw := ch.Type == domain.ChangeKindRequirementChanged
		var err error
		if ch.Before, err = encodeValue(aa.Before, raw); err != nil {
			return fmt.Errorf("before: %w", err)
		}
		if ch.After, err = encodeValue(aa.After, raw); err != nil {
			return fmt.Errorf("after: %w", err)
		}
		if aa.ReplacedBy != nil {
			s := subjectOf(*aa.ReplacedBy, product)
			ch.ReplacedBy = &s
		}
		as.Change = &ch
	case domain.AspectApplicability:
		if aa.Exposure == nil {
			return fmt.Errorf("exposure is required")
		}
		exp, err := conditionOf(*aa.Exposure)
		if err != nil {
			return fmt.Errorf("exposure: %w", err)
		}
		ap := domain.Applicability{Exposure: exp}
		if aa.Overlap != nil {
			ov, err := conditionOf(*aa.Overlap)
			if err != nil {
				return fmt.Errorf("overlap: %w", err)
			}
			ap.Overlap = &ov
		}
		as.Applicability = &ap
	case domain.AspectConsequence:
		k := domain.ConsequenceKind(aa.Kind)
		if !k.Valid() {
			return fmt.Errorf("unknown consequence kind %q", aa.Kind)
		}
		// the class follows the kind (DESIGN.md §1.4); the model never states it
		as.Consequence = &domain.Consequence{
			Kind: k, ExposedClass: k.ExposedClass(),
			Statement: oneLine(aa.Statement), Remediation: oneLine(aa.Remediation),
			Severity: domain.ImpactSeverity(aa.Severity),
		}
	}
	return nil
}

func subjectOf(s SubjectIdentity, product domain.ProductID) domain.Subject {
	// the product is implied: a component that only repeats it carries no
	// identity (deterministic normalisation, so spelling never splits agreement)
	if strings.EqualFold(strings.TrimSpace(s.Component), string(product)) {
		s.Component = ""
	}
	return domain.Subject{
		Family: domain.SubjectFamily(s.Family), Product: product,
		Group: strings.TrimSpace(s.Group), Version: strings.TrimSpace(s.Version), Kind: strings.TrimSpace(s.Kind),
		Path: strings.TrimSpace(s.Path), Name: strings.TrimSpace(s.Name), Component: strings.TrimSpace(s.Component),
	}
}

// encodeValue turns a literal answer value into the domain's encoding:
// canonical JSON for values, the raw string for a requirement range.
// Deterministic: "Never" → "\"Never\"", 30 → "30", null → "null".
func encodeValue(v json.RawMessage, raw bool) (*string, error) {
	if len(v) == 0 {
		return nil, nil
	}
	var x any
	if err := json.Unmarshal(v, &x); err != nil {
		return nil, err
	}
	if raw {
		s, ok := x.(string)
		if !ok {
			return nil, fmt.Errorf("a requirement is a version constraint string, got %s", string(v))
		}
		s = strings.TrimSpace(s)
		return &s, nil
	}
	switch x.(type) {
	case map[string]any, []any:
		return nil, fmt.Errorf("a value must be a scalar")
	}
	b, err := json.Marshal(x)
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}

func conditionOf(c ConditionAnswer) (domain.Condition, error) {
	out := domain.Condition{
		Op: domain.ConditionOp(c.Op), Group: c.Group, Version: c.Version, Kind: c.Kind, Name: c.Name, Path: c.Path,
		Component: c.Component, State: domain.FieldState(c.State), Pattern: c.Pattern, Separator: c.Separator,
		Range: c.Range, Reason: domain.UnknownReason(c.Reason), Needed: c.Needed,
	}
	if len(c.Values) > maxValues {
		return out, fmt.Errorf("%d values (at most %d)", len(c.Values), maxValues)
	}
	tokens := out.State == domain.StateHasToken || out.State == domain.StateHasTokenKey
	for _, v := range c.Values {
		var x any
		if err := json.Unmarshal(v, &x); err != nil {
			return out, err
		}
		if tokens {
			// tokens are plain strings ("Gate=true"), never JSON-encoded
			out.Values = append(out.Values, fmt.Sprint(x))
			continue
		}
		b, _ := json.Marshal(x)
		out.Values = append(out.Values, string(b))
	}
	for _, sub := range c.Of {
		sc, err := conditionOf(sub)
		if err != nil {
			return out, err
		}
		out.Of = append(out.Of, sc)
	}
	return out, nil
}

func containsString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
