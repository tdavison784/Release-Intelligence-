package orchestrate

import (
	"errors"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	DefaultReviewTimeout = 72 * time.Hour
	// MaxEvidenceRounds bounds request-more-evidence loops (bounded execution, P5).
	MaxEvidenceRounds = 2
)

// TaskQueue is the default task queue; see docs/phase4/env-impact/TEMPORAL.md.
const TaskQueue = "ri-investigation"

// WorkflowID returns the content-derived workflow ID, so re-submitting the same
// (dependency, versions, environment input) maps to the same execution.
func WorkflowID(in InvestigationInput) string {
	return "inv-" + IdempotencyKey("workflow", []ArtifactRef{
		{Digest: in.Dependency}, {Digest: in.FromVersion}, {Digest: in.ToVersion},
		{Digest: in.Environment}, in.EnvInputRef,
	})[:24]
}

type runner struct {
	ctx   workflow.Context
	state State
}

func (r *runner) audit(stage, status, key, detail string) {
	r.state.Audit = append(r.state.Audit, AuditEntry{
		Seq: len(r.state.Audit) + 1, Stage: stage, Status: status,
		At: workflow.Now(r.ctx), IdempotencyKey: key, Detail: detail,
	})
}

// stage runs one activity with audit entries before and after.
func (r *runner) stage(name string, inputs []ArtifactRef) (StageResult, error) {
	key := IdempotencyKey(name, inputs)
	r.state.CurrentStage = name
	r.audit(name, "started", key, "")
	var res StageResult
	err := workflow.ExecuteActivity(r.ctx, "Run", StageRequest{
		Stage: name, IdempotencyKey: key, Inputs: inputs, Round: r.state.Round,
	}).Get(r.ctx, &res)
	if err != nil {
		status := "failed"
		if temporal.IsCanceledError(err) {
			status = "cancelled"
		}
		r.audit(name, status, key, err.Error())
		return res, err
	}
	r.audit(name, "completed", key, res.Output.Digest)
	return res, nil
}

// InvestigationWorkflow runs the staged pipeline: collect change evidence, build env snapshot,
// build graph + traverse, investigate, validate, classify, then await human review if the
// (stub) deterministic policy requires it.
func InvestigationWorkflow(ctx workflow.Context, in InvestigationInput) (InvestigationResult, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:        time.Second,
			BackoffCoefficient:     2,
			MaximumInterval:        30 * time.Second,
			MaximumAttempts:        4,
			NonRetryableErrorTypes: []string{ErrTypeInvalidInput},
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)
	r := &runner{ctx: ctx}
	if err := workflow.SetQueryHandler(ctx, QueryState, func() (State, error) { return r.state, nil }); err != nil {
		return InvestigationResult{}, err
	}
	res := InvestigationResult{}
	finish := func(outcome string) (InvestigationResult, error) {
		res.Outcome, res.Classification = outcome, r.state.Classification
		res.Rounds, res.Audit = r.state.Round, r.state.Audit
		return res, nil
	}
	// cancelled records the cancellation on a context that survives it.
	cancelled := func(err error) (InvestigationResult, error) {
		dctx, _ := workflow.NewDisconnectedContext(ctx)
		rr := &runner{ctx: dctx, state: r.state}
		rr.audit(r.state.CurrentStage, "cancelled", "", "workflow cancelled")
		r.state = rr.state
		res.Outcome, res.Classification = OutcomeCancelled, r.state.Classification
		res.Rounds, res.Audit = r.state.Round, r.state.Audit
		return res, err
	}
	fail := func(err error) (InvestigationResult, error) {
		if temporal.IsCanceledError(err) || errors.Is(err, workflow.ErrCanceled) {
			return cancelled(temporal.NewCanceledError("cancelled in " + r.state.CurrentStage))
		}
		return res, err
	}
	keep := func(s StageResult) { res.Artifacts = append(res.Artifacts, s.Output) }

	base := []ArtifactRef{in.EnvInputRef, {Digest: in.Dependency + "@" + in.FromVersion + ".." + in.ToVersion}}
	change, err := r.stage(StageCollectChange, base)
	if err != nil {
		return fail(err)
	}
	keep(change)
	snap, err := r.stage(StageEnvSnapshot, []ArtifactRef{in.EnvInputRef})
	if err != nil {
		return fail(err)
	}
	keep(snap)
	graph, err := r.stage(StageGraph, []ArtifactRef{change.Output, snap.Output})
	if err != nil {
		return fail(err)
	}
	keep(graph)

	inputs := []ArtifactRef{change.Output, snap.Output, graph.Output}
	var cls StageResult
	for {
		inv, err := r.stage(StageInvestigate, inputs)
		if err != nil {
			return fail(err)
		}
		keep(inv)
		val, err := r.stage(StageValidate, []ArtifactRef{inv.Output})
		if err != nil {
			return fail(err)
		}
		keep(val)
		cls, err = r.stage(StageClassify, []ArtifactRef{inv.Output, val.Output})
		if err != nil {
			return fail(err)
		}
		keep(cls)
		r.state.Classification = cls.Classification
		if !cls.NeedsReview {
			return finish(OutcomeAutoCompleted)
		}

		d, outcome, err := r.awaitReview(in)
		if err != nil {
			return fail(err)
		}
		if outcome != "" {
			return finish(outcome)
		}
		res.Reviewer = d.Reviewer
		switch d.Decision {
		case DecisionApprove:
			return finish(OutcomeApproved)
		case DecisionReject:
			return finish(OutcomeRejected)
		case DecisionMoreEvidence:
			if r.state.Round >= MaxEvidenceRounds {
				r.audit(StageReview, "failed", "", "evidence round limit reached")
				return finish(OutcomeRejected)
			}
			r.state.Round++
			// New round => new idempotency keys, so side effects are not deduped away.
			inputs = append(inputs, ArtifactRef{Digest: fmt.Sprintf("round-%d:%s", r.state.Round, d.Note)})
		default:
			return res, temporal.NewNonRetryableApplicationError(
				"unknown review decision "+d.Decision, ErrTypeInvalidInput, nil)
		}
	}
}

// awaitReview blocks for a review signal, the timeout, or cancellation. A non-empty outcome
// means the wait ended without a decision.
func (r *runner) awaitReview(in InvestigationInput) (ReviewDecision, string, error) {
	timeout := in.ReviewTimeout
	if timeout <= 0 {
		timeout = DefaultReviewTimeout
	}
	r.state.CurrentStage = StageReview
	r.state.AwaitingReview = true
	defer func() { r.state.AwaitingReview = false }()
	r.audit(StageReview, "waiting", "", "timeout "+timeout.String())

	var d ReviewDecision
	ch := workflow.GetSignalChannel(r.ctx, SignalReview)
	timerCtx, cancelTimer := workflow.WithCancel(r.ctx)
	defer cancelTimer()
	timer := workflow.NewTimer(timerCtx, timeout)

	got, timedOut := false, false
	sel := workflow.NewSelector(r.ctx)
	sel.AddReceive(ch, func(c workflow.ReceiveChannel, _ bool) { c.Receive(r.ctx, &d); got = true })
	sel.AddFuture(timer, func(f workflow.Future) {
		if err := f.Get(r.ctx, nil); err == nil {
			timedOut = true
		}
	})
	sel.Select(r.ctx)
	switch {
	case r.ctx.Err() != nil:
		return d, "", r.ctx.Err()
	case got:
		r.audit(StageReview, "signal", "", d.Decision+" by "+d.Reviewer)
		return d, "", nil
	case timedOut:
		r.audit(StageReview, "timed-out", "", "no decision; failing closed")
		return d, OutcomeReviewTimedOut, nil
	}
	return d, "", errors.New("review wait ended unexpectedly")
}

// Register wires the workflow and activities onto a worker (or test environment registry).
func Register(w interface {
	RegisterWorkflow(any)
	RegisterActivityWithOptions(any, activity.RegisterOptions)
}, a *Activities) {
	w.RegisterWorkflow(InvestigationWorkflow)
	w.RegisterActivityWithOptions(a.Run, registerRun)
}
