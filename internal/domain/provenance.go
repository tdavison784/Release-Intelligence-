package domain

import (
	"errors"
	"fmt"
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
	// MethodAI: produced by a language model. Must carry model, prompt digest
	// and input evidence.
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

	// AI-only fields.
	Model         string       `json:"model,omitempty"`
	PromptDigest  string       `json:"promptDigest,omitempty"`
	InputEvidence []EvidenceID `json:"inputEvidence,omitempty"`
	GeneratedAt   *time.Time   `json:"generatedAt,omitempty"`
}

// Deterministic reports whether the provenance is not AI-derived.
func (p Provenance) Deterministic() bool { return p.Method != MethodAI }

// Validate checks internal consistency of the provenance record.
func (p Provenance) Validate() error {
	var errs []error
	switch p.Method {
	case MethodDeclared, MethodComputed, MethodHeuristic:
		if p.Model != "" || p.PromptDigest != "" {
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
