// Package drift detects source drift: whether the assumptions a trusted
// product definition makes about its upstream channels still hold for the
// NEWEST releases — those published after the releases the definition was
// validated against.
//
// Drift detection is deterministic: it re-validates every declared
// relationship (the same exhaustive machinery `ri check` uses) on the newest
// releases and reports structured differences. It never calls an LLM and
// never mutates products/*.yaml; the output is a report plus a proposed,
// annotated definition fragment that a human (or a reviewing agent) applies.
//
// The honesty rule of the rest of the pipeline applies with force here: an
// unreachable host is NOT drift. Events whose status is "unverifiable" say
// "I could not check"; only a channel that answered — and answered
// differently than the declaration expects — produces status "drift".
package drift

import (
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Event kinds (closed vocabulary).
const (
	// KindRelationshipBroken: a declared relationship (source, artifact or
	// content) that held historically fails on newer releases and no more
	// specific kind applies — tag convention change, renamed asset, dropped
	// content, broken chart/appVersion relation.
	KindRelationshipBroken = "relationship-broken"
	// KindArtifactMissing: a declared artifact is absent from every reachable
	// channel (not optional, not covered by exceptions).
	KindArtifactMissing = "artifact-missing"
	// KindArtifactAppeared: an artifact deterministically discoverable
	// through already-declared channels (e.g. an image carrying the release
	// tag referenced by a declared manifest/chart) that the definition does
	// not declare.
	KindArtifactAppeared = "artifact-appeared"
	// KindSourceMoved: something disappeared from its declared location while
	// a location already declared as an alternative answers (a document that
	// vanished, a chart that left its primary repository index but is present
	// at a declared fallback channel). No new host is ever invented.
	KindSourceMoved = "source-moved"
	// KindAvailabilityViolated: an artifact or source exists for a release
	// outside its declared availability window.
	KindAvailabilityViolated = "availability-violated"
	// KindUnverifiable: the check could not run — every channel was
	// unreachable. Explicitly NOT drift.
	KindUnverifiable = "unverifiable"
)

// Event statuses (closed vocabulary).
const (
	// StatusDrift: a channel was reachable and disagreed with the definition.
	StatusDrift = "drift"
	// StatusUnverifiable: could not be determined (host unreachable); the
	// opposite of drift.
	StatusUnverifiable = "unverifiable"
	// StatusNote: a real observation that is not a change — e.g. a
	// relationship that already failed at the baseline too.
	StatusNote = "note"
)

// Event severities (closed vocabulary).
const (
	SeverityHigh   = "high"   // a validated relationship is now broken
	SeverityMedium = "medium" // broken, but the historical evidence is weaker
	SeverityLow    = "low"    // informational (new artifact, stale window, lost verifiability)
)

// Subject kinds (closed vocabulary).
const (
	SubjectVersions = "versions" // a versions source (canonical channel)
	SubjectSource   = "source"
	SubjectArtifact = "artifact"
	SubjectContent  = "content"
	SubjectImage    = "image" // an image repository referenced but not declared
)

// Event is one drift observation: a structured difference between what the
// definition declares and what upstream answers for the checked releases.
type Event struct {
	Kind     string `json:"kind"`     // Kind* constant
	Status   string `json:"status"`   // Status* constant
	Severity string `json:"severity"` // Severity* constant
	// Subject identifies the definition element: source id, artifact id,
	// "<artifact>/<content kind>", or an image repository.
	Subject     string `json:"subject"`
	SubjectKind string `json:"subjectKind"` // Subject* constant
	// Releases are the checked releases where the observation was made,
	// ascending.
	Releases    []string `json:"releases"`
	Explanation string   `json:"explanation"` // human-readable, deterministic
	// Detail is the raw observation (the check's detail) the event is based on.
	Detail string `json:"detail,omitempty"`
	// Baseline records what held before and where that is documented, e.g.
	// "passed on 1.29.0, 1.30.0 (saved check report docs/onboarding/checks/x.json)".
	Baseline string `json:"baseline,omitempty"`
	// Channel/Coordinate locate the observation (locator kind / artifact coordinate).
	Channel    string `json:"channel,omitempty"`
	Coordinate string `json:"coordinate,omitempty"`
	// Evidence cites the records (Report.Evidence) backing the observation:
	// the release(s) and channel(s) that answered. Empty only when upstream
	// answered nothing at all (see DRIFT.md).
	Evidence []domain.EvidenceID `json:"evidence,omitempty"`
	// Proposal is the deterministic, annotated definition fragment for this
	// event (nil when no deterministic edit exists; the explanation then says
	// what to investigate).
	Proposal *Proposal `json:"proposal,omitempty"`
}

// Proposal is a suggested change to the product definition. YAML is an
// annotated fragment in definition syntax; it is never applied by a tool.
type Proposal struct {
	// Action is a short imperative description of the edit.
	Action string `json:"action"`
	// YAML is the annotated definition fragment ("" when the action cannot be
	// rendered deterministically).
	YAML string `json:"yaml,omitempty"`
}

// Baseline describes where "previously held" comes from.
type Baseline struct {
	// Source: "saved-check" (a `ri check -o json` report), "definition"
	// (validatedAgainst / provenance.validatedReleases) or "none".
	Source   string   `json:"source"`
	Path     string   `json:"path,omitempty"` // saved report the baseline was read from
	Releases []string `json:"releases,omitempty"`
	// Cutoff is the newest baseline release; checked releases are newer than
	// it ("" when there is no baseline at all).
	Cutoff string `json:"cutoff,omitempty"`
	// DefinitionDigest is the definition revision the saved report was
	// produced from, and DigestState compares it with the current definition:
	// "current" (same revision), "stale" (the definition changed since the
	// baseline was saved, so the baseline describes an older definition) or
	// "unrecorded" (a report saved before digests were recorded). Staleness
	// is a property of the baseline, not an event: it never counts as drift.
	DefinitionDigest string `json:"definitionDigest,omitempty"`
	DigestState      string `json:"digestState,omitempty"`
}

// Baseline digest states.
const (
	DigestCurrent    = "current"
	DigestStale      = "stale"
	DigestUnrecorded = "unrecorded"
)

// Baseline sources.
const (
	BaselineSavedCheck = "saved-check"
	BaselineDefinition = "definition"
	BaselineNone       = "none"
)

// Summary counts events by status.
type Summary struct {
	Drift        int            `json:"drift"`
	Unverifiable int            `json:"unverifiable"`
	Notes        int            `json:"notes"`
	ByKind       map[string]int `json:"byKind,omitempty"`
	Checked      int            `json:"checked"` // releases re-validated
}

// Report is the drift report for one product.
type Report struct {
	Product          domain.ProductID      `json:"product"`
	DefinitionPath   string                `json:"definitionPath,omitempty"`
	DefinitionDigest string                `json:"definitionDigest,omitempty"`
	GeneratedAt      time.Time             `json:"generatedAt"`
	Baseline         Baseline              `json:"baseline"`
	Checked          []string              `json:"checked"` // releases re-validated, ascending
	VersionsSources  []domain.SourceStatus `json:"versionsSources,omitempty"`
	Events           []Event               `json:"events"`
	Summary          Summary               `json:"summary"`
	Evidence         []domain.Evidence     `json:"evidence,omitempty"`
}

// HasDrift reports whether at least one event is confirmed drift.
func (r *Report) HasDrift() bool { return r.Summary.Drift > 0 }

func summarizeEvents(events []Event, checked int) Summary {
	s := Summary{Checked: checked, ByKind: map[string]int{}}
	for _, e := range events {
		s.ByKind[e.Kind]++
		switch e.Status {
		case StatusDrift:
			s.Drift++
		case StatusUnverifiable:
			s.Unverifiable++
		default:
			s.Notes++
		}
	}
	return s
}
