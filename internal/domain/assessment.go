package domain

import (
	"errors"
	"fmt"
	"strings"
)

// Phase 6 Milestone 1 (docs/phase6/decision-intel/PLAN-M1.md change 6, mission
// B5): the agent's natural technical assessment of one change item. It is AI
// output stored beside the provisional verdict, and like the verdict it is
// never an input of the classification policy (the runner keeps it out of the
// classifier's AgentOutcome). Only Verified entries are checked by the tool
// layer: each cites evidence that resolved. Everything else is unverified AI
// prose (StatementKind). CONTRACT-CHANGE(consumer): additive.

// AgentAssessmentIDPrefix prefixes agent assessment ids.
const AgentAssessmentIDPrefix = "aas-"

// Effective configuration states of an assessment setting (the dossier's
// vocabulary).
const (
	AssessmentExplicit         = "explicit"
	AssessmentInheritedDefault = "inherited-default"
	AssessmentUnknown          = "unknown"
)

// AgentAssessment is the natural assessment of one change item: what
// changed, the potential breaking mechanism, the relevant consumers, the
// effective environment configuration, what was verified (cited), what is
// inferred, what remains unknown, the likely impact and the recommended
// further investigation or tests.
type AgentAssessment struct {
	ID         string `json:"id"` // AgentAssessmentID(item, whatChanged, mechanism, callId)
	ChangeItem string `json:"changeItem"`
	// Verdict is the id of the provisional verdict of the same closing entry.
	Verdict string `json:"verdict,omitempty"`
	// Headline, Scope and Preconditions are the scoped verdict (prompt
	// feature scopedVerdict, docs/phase6/decision-intel/briefs/scoped.md):
	// one sentence with the class, the scope and the preconditions; what the
	// session examined; and the runtime or operational facts outside that
	// scope the verdict depends on. AI prose like the rest.
	// CONTRACT-CHANGE(scoped): additive.
	Headline               string              `json:"headline,omitempty"`
	Scope                  string              `json:"scope,omitempty"`
	Preconditions          []string            `json:"preconditions,omitempty"`
	WhatChanged            string              `json:"whatChanged"`
	Mechanism              string              `json:"mechanism"`
	Consumers              []string            `json:"consumers,omitempty"`
	EffectiveConfiguration []AssessmentSetting `json:"effectiveConfiguration,omitempty"`
	Verified               []AssessmentFinding `json:"verified,omitempty"`
	Inferred               []string            `json:"inferred,omitempty"`
	Unknown                []string            `json:"unknown,omitempty"`
	LikelyImpact           string              `json:"likelyImpact"`
	RecommendedTests       []string            `json:"recommendedTests,omitempty"`
	// UntracedConsumers are the graph consumers of the item's exposed objects
	// the session never retrieved (node keys), added by the tool layer, not
	// the model; each is also stated in Unknown as an investigation gap.
	UntracedConsumers []string `json:"untracedConsumers,omitempty"`
	// UnavailableEvidence are the item's upstream evidence requests that
	// could not be fulfilled (prompt feature upstream), added by the tool
	// layer; each is also stated in Unknown as "behaviour at <tag>
	// unavailable: <reason>". CONTRACT-CHANGE(upstream): additive.
	UnavailableEvidence []string `json:"unavailableEvidence,omitempty"`
	// Behaviour are the upstream behaviours the verdict relies on, each with
	// its source (prompt feature behaviour, docs/phase6/decision-intel/
	// briefs/upstream2.md). The tool layer checks each source; an entry
	// sourced unavailable or not-checked is also stated in Unknown as
	// "behaviour <b> at <tag>: <source>". CONTRACT-CHANGE(upstream2): additive.
	Behaviour     []AssessmentBehaviour `json:"behaviour,omitempty"`
	StatementKind string                `json:"statementKind,omitempty"` // StatementUnverifiedProse
	Provenance          Provenance `json:"provenance"`              // method ai
}

// AssessmentSetting is one setting the change touches and its effective
// state in the environment: explicit (set, and where), inherited-default
// (left unset: the shipped default applies) or unknown (and why).
type AssessmentSetting struct {
	Setting string `json:"setting"`
	State   string `json:"state"`
	Detail  string `json:"detail,omitempty"`
}

// Sources of a behaviour dependency.
const (
	// BehaviourUpstream: upstream evidence beyond the item's change text (a
	// fetched run artifact or an upstream source artifact of the bundle),
	// cited verbatim.
	BehaviourUpstream = "upstream"
	// BehaviourChangeText: a verbatim quote of the item's own change text,
	// which states the behaviour itself.
	BehaviourChangeText = "change-text"
	// BehaviourUnavailable: an upstream request_evidence request of the item
	// that was recorded unavailable.
	BehaviourUnavailable = "unavailable"
	// BehaviourNotChecked: honestly unestablished; with prompt feature
	// fetchFirst it may carry the reason the upstream source cannot answer it.
	BehaviourNotChecked = "not-checked"
)

// BehaviourSources lists every behaviour source.
var BehaviourSources = []string{BehaviourUpstream, BehaviourChangeText, BehaviourUnavailable, BehaviourNotChecked}

// AssessmentBehaviour is one upstream behaviour (validation, defaulting,
// parsing, a feature gate, API semantics) an assessment's verdict relies on,
// and where it is established: Citations (upstream), Quote (change-text),
// Request (unavailable), or nowhere (not-checked, optionally with the Reason
// the upstream source cannot answer it). CONTRACT-CHANGE(upstream3): Reason
// is additive.
type AssessmentBehaviour struct {
	Behaviour string       `json:"behaviour"`
	Source    string       `json:"source"`
	Citations []EvidenceID `json:"citations,omitempty"`
	Quote     string       `json:"quote,omitempty"`
	Request   string       `json:"request,omitempty"`
	Reason    string       `json:"reason,omitempty"`
}

// Gap reports whether the behaviour is left unestablished.
func (b AssessmentBehaviour) Gap() bool {
	return b.Source == BehaviourUnavailable || b.Source == BehaviourNotChecked
}

// AssessmentFinding is one verified statement with the evidence its
// citations became (each resolved in the run's pool).
type AssessmentFinding struct {
	Statement string       `json:"statement"`
	Citations []EvidenceID `json:"citations"`
}

// AgentAssessmentID derives an assessment id.
func AgentAssessmentID(changeItem, whatChanged, mechanism, callID string) string {
	return AgentRecordID(AgentAssessmentIDPrefix, changeItem, whatChanged+"\x00"+mechanism, callID)
}

// Validate checks the assessment's shape.
func (a AgentAssessment) Validate() error {
	errs := validateAI("agent assessment "+a.ID, a.Provenance)
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("agent assessment %s: %s", a.ID, fmt.Sprintf(format, args...)))
	}
	if a.ID != AgentAssessmentID(a.ChangeItem, a.WhatChanged, a.Mechanism, a.Provenance.CallID) {
		bad("id is not derived")
	}
	if !strings.HasPrefix(a.ChangeItem, ChangeItemIDPrefix) {
		bad("changeItem is required")
	}
	if a.Verdict != "" && !strings.HasPrefix(a.Verdict, AgentVerdictIDPrefix) {
		bad("verdict %q is not a verdict id", a.Verdict)
	}
	if strings.TrimSpace(a.WhatChanged) == "" || strings.TrimSpace(a.Mechanism) == "" || strings.TrimSpace(a.LikelyImpact) == "" {
		bad("whatChanged, mechanism and likelyImpact are required")
	}
	for i, s := range a.EffectiveConfiguration {
		if strings.TrimSpace(s.Setting) == "" {
			bad("effectiveConfiguration[%d]: setting is required", i)
		}
		switch s.State {
		case AssessmentExplicit, AssessmentInheritedDefault, AssessmentUnknown:
		default:
			bad("effectiveConfiguration[%d]: unknown state %q", i, s.State)
		}
	}
	for i, p := range a.Preconditions {
		if strings.TrimSpace(p) == "" {
			bad("preconditions[%d] is empty", i)
		}
	}
	for i, f := range a.Verified {
		if strings.TrimSpace(f.Statement) == "" || len(f.Citations) == 0 {
			bad("verified[%d]: a verified statement cites evidence", i)
		}
	}
	for i, b := range a.Behaviour {
		if strings.TrimSpace(b.Behaviour) == "" {
			bad("behaviour[%d]: behaviour is required", i)
		}
		ok := false
		switch b.Source {
		case BehaviourUpstream:
			ok = len(b.Citations) > 0 && b.Quote == "" && b.Request == ""
		case BehaviourChangeText:
			ok = len(b.Citations) == 0 && strings.TrimSpace(b.Quote) != "" && b.Request == ""
		case BehaviourUnavailable:
			ok = len(b.Citations) == 0 && b.Quote == "" && strings.HasPrefix(b.Request, EvidenceRequestIDPrefix)
		case BehaviourNotChecked:
			ok = len(b.Citations) == 0 && b.Quote == "" && b.Request == ""
		default:
			bad("behaviour[%d]: unknown source %q", i, b.Source)
			continue
		}
		if !ok {
			bad("behaviour[%d]: source %s takes citations (upstream), a quote (change-text), a request id (unavailable) or nothing (not-checked)", i, b.Source)
		}
		if b.Reason != "" && b.Source != BehaviourNotChecked {
			bad("behaviour[%d]: only source not-checked takes a reason", i)
		}
	}
	return errors.Join(errs...)
}
