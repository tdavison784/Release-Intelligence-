// Package orchestrate is the Temporal edge of Release Intelligence (MISSION §A7).
//
// It is the only package (besides cmd/ri wiring) allowed to import go.temporal.io/*.
// Everything in this wave-0 spike uses placeholder payload types: the real evidence,
// snapshot, graph and finding types come from the contract lane. Large data must travel as
// artifact references (ArtifactRef), never inline, because Temporal payloads are capped
// (~2 MB per payload, 4 MB per history event batch) and every payload is stored in history.
package orchestrate

import "time"

// Stage names, in pipeline order. They double as audit-log stages and activity names.
const (
	StageCollectChange = "collect-change-evidence"
	StageEnvSnapshot   = "build-env-snapshot"
	StageGraph         = "build-graph-traverse"
	StageInvestigate   = "investigate"
	StageValidate      = "validate"
	StageClassify      = "classify"
	StageReview        = "await-human-review"
)

// Names of the workflow's external interface.
const (
	SignalReview = "review-decision"
	QueryState   = "state"
)

// Error types. ErrTypeInvalidInput is registered as non-retryable in the retry policy;
// ErrTypeTransient is retried with backoff.
const (
	ErrTypeInvalidInput = "InvalidInput"
	ErrTypeTransient    = "Transient"
)

// Classification mirrors the four MISSION §A6 outcomes (placeholder strings).
const (
	ClassAffected             = "Affected"
	ClassNotAffected          = "Not Affected"
	ClassPotentiallyAffected  = "Potentially Affected"
	ClassInsufficientEvidence = "Insufficient Evidence"
)

// Review decisions accepted by the review signal.
const (
	DecisionApprove      = "approve"
	DecisionReject       = "reject"
	DecisionMoreEvidence = "request-more-evidence"
)

// Final outcomes of a workflow run.
const (
	OutcomeAutoCompleted  = "auto-completed"   // no review needed (deterministic policy)
	OutcomeApproved       = "approved"         // human approved
	OutcomeRejected       = "rejected"         // human rejected
	OutcomeReviewTimedOut = "review-timed-out" // fail closed: nothing is approved
	OutcomeCancelled      = "cancelled"
)

// ArtifactRef points at content-addressed data held outside Temporal history.
type ArtifactRef struct {
	Digest string `json:"digest"` // sha256:<hex>
	URI    string `json:"uri,omitempty"`
}

// InvestigationInput is the placeholder workflow input. Only refs and digests, never bodies.
type InvestigationInput struct {
	Dependency    string        `json:"dependency"`
	FromVersion   string        `json:"fromVersion"`
	ToVersion     string        `json:"toVersion"`
	Environment   string        `json:"environment"`
	EnvInputRef   ArtifactRef   `json:"envInputRef"`
	ReviewTimeout time.Duration `json:"reviewTimeout"` // 0 => DefaultReviewTimeout
}

// StageRequest is the input of every stub activity.
type StageRequest struct {
	Stage          string        `json:"stage"`
	IdempotencyKey string        `json:"idempotencyKey"` // sha256 of stage + input digests
	Inputs         []ArtifactRef `json:"inputs"`
	Round          int           `json:"round"` // investigation round (>0 after request-more-evidence)
}

// StageResult is the output of every stub activity.
type StageResult struct {
	Output         ArtifactRef `json:"output"`
	Classification string      `json:"classification,omitempty"` // classify stage only
	NeedsReview    bool        `json:"needsReview,omitempty"`    // classify stage only
}

// ReviewDecision is the review signal payload.
type ReviewDecision struct {
	Decision string `json:"decision"`
	Reviewer string `json:"reviewer"`
	Note     string `json:"note,omitempty"`
}

// AuditEntry is one deterministic stage transition. Timestamps come from workflow.Now, so
// replays produce an identical log.
type AuditEntry struct {
	Seq            int       `json:"seq"`
	Stage          string    `json:"stage"`
	Status         string    `json:"status"` // started|completed|failed|waiting|signal|timed-out|cancelled
	At             time.Time `json:"at"`
	IdempotencyKey string    `json:"idempotencyKey,omitempty"`
	Detail         string    `json:"detail,omitempty"`
}

// State is returned by the QueryState query.
type State struct {
	CurrentStage   string       `json:"currentStage"`
	Classification string       `json:"classification,omitempty"`
	Round          int          `json:"round"`
	AwaitingReview bool         `json:"awaitingReview"`
	Audit          []AuditEntry `json:"audit"`
}

// InvestigationResult is the workflow result.
type InvestigationResult struct {
	Outcome        string        `json:"outcome"`
	Classification string        `json:"classification"`
	Reviewer       string        `json:"reviewer,omitempty"`
	Rounds         int           `json:"rounds"`
	Artifacts      []ArtifactRef `json:"artifacts"`
	Audit          []AuditEntry  `json:"audit"`
}
