// Package orchestrate is the Temporal edge of Release Intelligence (MISSION
// §A7, DESIGN.md §5). It is the only package (besides cmd/ri wiring) allowed
// to import go.temporal.io/*. The stages themselves are plain Go in
// internal/investigate/run; this package gives each one an activity with its
// §5.1 retry class and timeout, and runs them in a deterministic workflow
// with a human-review signal, a state query, cancellation and bounded rounds.
//
// Payloads carry ids, digests and small verdicts only: records live in the
// RecordStore, bytes in the ArtifactStore (Temporal payloads are capped at
// ~2 MB; TEMPORAL.md).
package orchestrate

import (
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/run"
)

// Names of the workflow's external interface.
const (
	// TaskQueue is the default task queue.
	TaskQueue = "ri-investigation"
	// WorkflowType is the registered workflow name.
	WorkflowType = "InvestigationWorkflow"
	// SignalReview carries an investigate.ReviewDecision.
	SignalReview = "review-decision"
	// QueryState returns State.
	QueryState = "state"
	// ActivityReview applies a review decision; ActivityFinalize ends a run
	// without one (timeout, cancellation, exhausted rounds).
	ActivityReview   = string(domain.StageHumanReview)
	ActivityFinalize = "finalize"
)

// Error types of application errors. Permanent is non-retryable.
const (
	ErrTypePermanent = "Permanent"
	ErrTypeTransient = "Transient"
)

// DefaultReviewTimeout is how long a run waits for a human (DESIGN.md §5.1).
const DefaultReviewTimeout = 7 * 24 * time.Hour

// Search attribute names (custom attributes must be registered on the
// cluster: `temporal operator search-attribute create`; see TEMPORAL.md).
const (
	AttrDependency     = "RiDependency"
	AttrEnvironment    = "RiEnvironment"
	AttrClassification = "RiClassification"
	AttrReviewState    = "RiReviewState"
)

// Input is the workflow input: the request plus orchestration knobs.
type Input struct {
	Request investigate.Request `json:"request"`
	// Definition is the digest of the product definition (ChangeKey input).
	Definition string `json:"definition,omitempty"`
	// ReviewTimeout bounds each wait for a human (0 = DefaultReviewTimeout).
	ReviewTimeout time.Duration `json:"reviewTimeout,omitempty"`
	// SearchAttributes upserts the Ri* search attributes (they must be
	// registered on the cluster).
	SearchAttributes bool `json:"searchAttributes,omitempty"`
}

// AuditEntry is one deterministic workflow-level transition (§5.4: the
// workflow keeps its own log; the run record's hash-chained trail is written
// by the activities). Times come from workflow.Now, so replays are identical.
type AuditEntry struct {
	Seq    int       `json:"seq"`
	Stage  string    `json:"stage"`
	Status string    `json:"status"` // started|completed|failed|waiting|signal|timed-out|cancelled
	At     time.Time `json:"at"`
	Key    string    `json:"key,omitempty"`
	Detail string    `json:"detail,omitempty"`
}

// State is returned by the QueryState query, while running and after.
type State struct {
	CurrentStage   string               `json:"currentStage"`
	Round          int                  `json:"round"`
	Investigation  string               `json:"investigation,omitempty"`
	Decision       domain.PolicyOutcome `json:"decision,omitempty"`
	Review         domain.ReviewState   `json:"review,omitempty"`
	AwaitingReview bool                 `json:"awaitingReview"`
	Outcome        string               `json:"outcome,omitempty"`
	Audit          []AuditEntry         `json:"audit"`
}

// Result is the workflow result: the same run.Result the in-process runner
// returns, plus the workflow's own log.
type Result struct {
	run.Result
	Audit []AuditEntry `json:"audit"`
}
