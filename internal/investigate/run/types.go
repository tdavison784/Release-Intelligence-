package run

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Producer is the producer string of artifacts and audit detail written by
// the orchestration layer (component@vN).
const Producer = "orchestrate@v1"

// Phases distinguish the two runs of the traverse and validate stages
// (DESIGN.md §5.1): the deterministic pass before the agent, and the pass
// after it over the graph extended with the agent's proposed edges.
const (
	PhaseInitial = "initial"
	PhaseFinal   = "final"
)

// Outcomes of a whole run (Result.Outcome).
const (
	OutcomeAutoCompleted  = "auto-completed"   // automated approval enacted (never in milestone 1)
	OutcomeApproved       = "approved"         // a human approved
	OutcomeRejected       = "rejected"         // a human rejected (or the evidence rounds ran out)
	OutcomeReviewTimedOut = "review-timed-out" // fail closed: nothing is approved
	OutcomeCancelled      = "cancelled"
	OutcomeAwaitingReview = "awaiting-review" // in-process run with no review source: the record waits for `ri investigate review`
)

// MaxEvidenceRounds bounds request-more-evidence loops (DESIGN.md §5.3).
const MaxEvidenceRounds = 2

// Refs are the small handles the stages hand to each other (ids and
// digests only, never bodies; TEMPORAL.md payload cap).
type Refs struct {
	Change   string `json:"change,omitempty"`   // workspace digest of the ChangeResult
	Snapshot string `json:"snapshot,omitempty"` // workspace digest of the SnapshotResult
	// Bundle and Investigation exist once assemble-bundle ran.
	Bundle        string `json:"bundle,omitempty"`
	Investigation string `json:"investigation,omitempty"`
	// Previous is the investigation of the previous evidence round.
	Previous string `json:"previous,omitempty"`
	// Record is the content digest (no audit, no timestamps) of the run
	// record after the last stage: it feeds idempotency keys.
	Record string `json:"record,omitempty"`
	// Pending holds the audit events of stages that ran before the record
	// existed; assemble-bundle writes them into the new record, in order.
	Pending []domain.AuditEvent `json:"pending,omitempty"`
}

// StageInput is the payload of one stage activity.
type StageInput struct {
	Stage   domain.Stage        `json:"stage"`
	Phase   string              `json:"phase,omitempty"`
	Key     string              `json:"key"`
	Round   int                 `json:"round"`
	Attempt int                 `json:"attempt,omitempty"` // 1-based; >1 appends `retried`
	Request investigate.Request `json:"request"`
	// Definition is the digest of the product definition (ChangeKey input).
	Definition string `json:"definition,omitempty"`
	Refs       Refs   `json:"refs"`
	// Extra are the inputs a reviewer asked for (request-more-evidence).
	Extra []string `json:"extra,omitempty"`
}

// StageOutput is the payload a stage returns.
type StageOutput struct {
	Stage  domain.Stage `json:"stage"`
	Key    string       `json:"key"`
	Refs   Refs         `json:"refs"`
	Detail string       `json:"detail,omitempty"`
	// Digest is the digest of the run record, audit sequence included
	// (timestamps excluded), after the stage; "" before the record exists.
	Digest string `json:"digest,omitempty"`
	// After classify:
	Outcome domain.PolicyOutcome `json:"outcome,omitempty"`
	Review  domain.ReviewState   `json:"review,omitempty"`
}

// ReviewInput is the payload of the human-review activity.
type ReviewInput struct {
	Investigation string                     `json:"investigation"`
	Round         int                        `json:"round"`
	Decision      investigate.ReviewDecision `json:"decision"`
}

// ReviewOutput says what applying a decision did.
type ReviewOutput struct {
	State  domain.ReviewState         `json:"state"`
	Status domain.InvestigationStatus `json:"status"`
	Detail string                     `json:"detail,omitempty"`
	Digest string                     `json:"digest"`
}

// FinalizeInput ends a run that did not end in a decision.
type FinalizeInput struct {
	Investigation string                     `json:"investigation"`
	Stage         domain.Stage               `json:"stage"`
	Event         string                     `json:"event"` // timed-out | cancelled | exhausted
	Status        domain.InvestigationStatus `json:"status"`
	Detail        string                     `json:"detail,omitempty"`
}

// Result is what a whole run returns (in-process and Temporal alike).
type Result struct {
	Outcome       string               `json:"outcome"`
	Investigation string               `json:"investigation,omitempty"`
	Bundle        string               `json:"bundle,omitempty"`
	Decision      domain.PolicyOutcome `json:"decision,omitempty"`
	Review        domain.ReviewState   `json:"review,omitempty"`
	Reviewer      string               `json:"reviewer,omitempty"`
	Rounds        int                  `json:"rounds"`
	// RecordDigest identifies the run record: the same inputs, config and
	// (replay/scripted) agent yield the same digest on every path.
	RecordDigest string `json:"recordDigest,omitempty"`
}

// Heartbeat reports liveness from inside a long stage (Temporal heartbeat).
type Heartbeat func(detail string)

type hbKey struct{}

// WithHeartbeat attaches a heartbeat function to ctx.
func WithHeartbeat(ctx context.Context, f Heartbeat) context.Context {
	return context.WithValue(ctx, hbKey{}, f)
}

func beat(ctx context.Context, detail string) {
	if f, ok := ctx.Value(hbKey{}).(Heartbeat); ok && f != nil {
		f(detail)
	}
}

// StageSpec is one step of a round.
type StageSpec struct {
	Stage domain.Stage
	Phase string
}

// RoundPlan is the stage sequence of one round: the first round collects the
// change evidence, later rounds reuse it (DESIGN.md §5.1/§5.3).
func RoundPlan(first bool) []StageSpec {
	var plan []StageSpec
	if first {
		plan = append(plan, StageSpec{Stage: domain.StageCollectChange})
	}
	return append(plan,
		StageSpec{Stage: domain.StageBuildSnapshot},
		StageSpec{Stage: domain.StageAssemble},
		StageSpec{Stage: domain.StageBuildGraph},
		StageSpec{Stage: domain.StageTraverse, Phase: PhaseInitial},
		StageSpec{Stage: domain.StageValidate, Phase: PhaseInitial},
		StageSpec{Stage: domain.StageInvestigate},
		StageSpec{Stage: domain.StageTraverse, Phase: PhaseFinal},
		StageSpec{Stage: domain.StageValidate, Phase: PhaseFinal},
		StageSpec{Stage: domain.StageClassify},
	)
}

// Policy is the retry/timeout policy of one stage (DESIGN.md §5.1).
type Policy struct {
	MaxAttempts  int
	Initial      time.Duration
	Max          time.Duration
	Coefficient  float64
	StartToClose time.Duration
	Heartbeat    time.Duration // 0 = none
}

// PolicyOf returns the stage's policy. Investigate's timeout follows the
// agent budget's wall clock plus five minutes.
func PolicyOf(stage domain.Stage, cfg investigate.Config) Policy {
	base := Policy{MaxAttempts: 1, Initial: time.Second, Max: 30 * time.Second, Coefficient: 2}
	switch stage {
	case domain.StageCollectChange:
		base.MaxAttempts, base.Initial, base.Max = 5, 2*time.Second, 2*time.Minute
		base.StartToClose, base.Heartbeat = 10*time.Minute, 30*time.Second
	case domain.StageBuildSnapshot:
		base.MaxAttempts, base.StartToClose = 3, 5*time.Minute
	case domain.StageAssemble:
		base.MaxAttempts, base.StartToClose, base.Heartbeat = 3, 15*time.Minute, 30*time.Second
	case domain.StageBuildGraph:
		base.StartToClose = 2 * time.Minute
	case domain.StageTraverse:
		base.StartToClose = time.Minute
	case domain.StageValidate:
		// 10 minutes per validator; the stage runs them in one activity.
		n := len(cfg.Validators)
		if n < 1 {
			n = 1
		}
		base.MaxAttempts, base.StartToClose, base.Heartbeat = 3, time.Duration(n)*10*time.Minute, 30*time.Second
	case domain.StageInvestigate:
		base.StartToClose = time.Duration(cfg.Agent.Budget.MaxSeconds)*time.Second + 5*time.Minute
		base.Heartbeat = 30 * time.Second
	case domain.StageClassify:
		base.StartToClose = time.Minute
	default:
		base.StartToClose = time.Minute
	}
	return base
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
