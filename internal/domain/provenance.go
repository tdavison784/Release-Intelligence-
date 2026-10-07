package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Method states how a piece of knowledge was derived.
type Method string

const (
	// MethodDeclared: upstream explicitly stated it in a structured or
	// labelled way (e.g. an item under a "Breaking Changes" heading, a
	// release-note YAML with "action required", a published support matrix).
	MethodDeclared Method = "declared"
	// MethodComputed: deterministic computation over source data
	// (e.g. a diff of two values.yaml files or two CRD sets).
	MethodComputed Method = "computed"
	// MethodHeuristic: deterministic but pattern-based interpretation
	// (e.g. keyword matching "deprecated" inside a bug-fix bullet).
	MethodHeuristic Method = "heuristic"
	// MethodAI: produced by a language model. Must carry the model and model
	// version that answered, the prompt version and digest, the input
	// evidence and the generation time.
	MethodAI Method = "ai"
)

// Confidence is a coarse confidence level.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Provenance records how a Change or Enrichment was produced.
type Provenance struct {
	Method     Method     `json:"method"`
	Producer   string     `json:"producer"`       // component@version, e.g. "normalize.releasenotes@v1"
	Rule       string     `json:"rule,omitempty"` // rule identifier, e.g. "section:/breaking/i"
	Confidence Confidence `json:"confidence"`

	// AI-only fields. Model and ModelVersion are as reported by whoever
	// produced the answer (the API response or an exchange response file),
	// never assumed from the request; ModelVersion is the most precise
	// version identifier reported. PromptVersion names the prompt templates
	// (e.g. "enrich/v1"); PromptDigest pins the exact rendered prompt.
	Model         string       `json:"model,omitempty"`
	ModelVersion  string       `json:"modelVersion,omitempty"`
	PromptVersion string       `json:"promptVersion,omitempty"`
	PromptDigest  string       `json:"promptDigest,omitempty"`
	InputEvidence []EvidenceID `json:"inputEvidence,omitempty"`
	GeneratedAt   *time.Time   `json:"generatedAt,omitempty"`
	// CallID identifies the single stateless model call that produced the
	// answer — the request/message id or CLI session id from the provider's
	// response envelope, never invented. It makes "separate calls" checkable
	// (PO-1, docs/phase3/learning-loop/DECISIONS.md). Optional in general;
	// required on SemanticProposals.
	CallID string `json:"callId,omitempty"`
	// Provider names who served the model ("anthropic", "zai", …; MISSION
	// Goal 2). AI-only; set by every writer going forward. Older records
	// carried it as Rule "provider:<name>" — read it with ProviderName.
	Provider string `json:"provider,omitempty"`
}

// ProviderName returns the provider that served the model: the Provider
// field, or — for records written before it existed — the name in a legacy
// Rule "provider:<name>". "" when neither states it.
func (p Provenance) ProviderName() string {
	if p.Provider != "" {
		return p.Provider
	}
	if name, ok := strings.CutPrefix(p.Rule, "provider:"); ok {
		return strings.TrimSpace(name)
	}
	return ""
}

// Deterministic reports whether the provenance is not AI-derived.
func (p Provenance) Deterministic() bool { return p.Method != MethodAI }

// Validate checks internal consistency of the provenance record.
func (p Provenance) Validate() error {
	var errs []error
	switch p.Method {
	case MethodDeclared, MethodComputed, MethodHeuristic:
		if p.Model != "" || p.PromptDigest != "" || p.ModelVersion != "" || p.PromptVersion != "" || len(p.InputEvidence) > 0 || p.GeneratedAt != nil || p.CallID != "" || p.Provider != "" {
			errs = append(errs, fmt.Errorf("deterministic provenance (%s) must not carry model/prompt fields", p.Method))
		}
	case MethodAI:
		if p.Model == "" {
			errs = append(errs, errors.New("ai provenance requires model"))
		}
		if p.PromptDigest == "" {
			errs = append(errs, errors.New("ai provenance requires promptDigest"))
		}
		if len(p.InputEvidence) == 0 {
			errs = append(errs, errors.New("ai provenance requires inputEvidence"))
		}
		if p.ModelVersion == "" {
			errs = append(errs, errors.New("ai provenance requires modelVersion"))
		}
		if p.PromptVersion == "" {
			errs = append(errs, errors.New("ai provenance requires promptVersion"))
		}
		if p.GeneratedAt == nil || p.GeneratedAt.IsZero() {
			errs = append(errs, errors.New("ai provenance requires generatedAt"))
		}
	default:
		errs = append(errs, fmt.Errorf("unknown provenance method %q", p.Method))
	}
	if p.Producer == "" {
		errs = append(errs, errors.New("provenance requires producer"))
	}
	switch p.Confidence {
	case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
	default:
		errs = append(errs, fmt.Errorf("invalid confidence %q", p.Confidence))
	}
	return errors.Join(errs...)
}
