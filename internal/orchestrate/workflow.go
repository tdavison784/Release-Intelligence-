package orchestrate

import (
	"errors"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/run"
)

// WorkflowID returns the content-derived workflow id of a request, so
// re-submitting the same investigation attaches to the existing execution.
// The investigation id (bundle id + config digest) is not known before the
// evidence is collected, so the workflow id is derived from the request
// instead; the investigation id is in the state query and the result. A
// deliberate re-run adds a `-r<n>` suffix (rerun > 0).
func WorkflowID(req investigate.Request, rerun int) string {
	id := "invreq-" + domain.ShortHash(req.Product, req.From, req.To, req.Environment.Name,
		domain.Digest(mustJSON(req.Environment)), req.Config.Digest())
	if rerun > 0 {
		id += "-r" + itoa(rerun)
	}
	return id
}

type wf struct {
	ctx   workflow.Context
	in    Input
	state State
	// digest is the run record digest after the last review decision.
	digest string
}

func (w *wf) audit(stage, status, key, detail string) {
	w.state.Audit = append(w.state.Audit, AuditEntry{
		Seq: len(w.state.Audit) + 1, Stage: stage, Status: status, At: workflow.Now(w.ctx), Key: key, Detail: detail,
	})
}

func (w *wf) options(stage domain.Stage) workflow.Context {
	p := run.PolicyOf(stage, w.in.Request.Config)
	return workflow.WithActivityOptions(w.ctx, workflow.ActivityOptions{
		StartToCloseTimeout: p.StartToClose,
		HeartbeatTimeout:    p.Heartbeat,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval: p.Initial, BackoffCoefficient: p.Coefficient, MaximumInterval: p.Max,
			MaximumAttempts: int32(p.MaxAttempts), NonRetryableErrorTypes: []string{ErrTypePermanent},
		},
	})
}

func (w *wf) upsert() {
	if !w.in.SearchAttributes {
		return
	}
	_ = workflow.UpsertTypedSearchAttributes(w.ctx,
		temporal.NewSearchAttributeKeyKeyword(AttrDependency).ValueSet(w.in.Request.Product),
		temporal.NewSearchAttributeKeyKeyword(AttrEnvironment).ValueSet(w.in.Request.Environment.Name),
		temporal.NewSearchAttributeKeyKeyword(AttrClassification).ValueSet(string(w.state.Decision)),
		temporal.NewSearchAttributeKeyKeyword(AttrReviewState).ValueSet(reviewAttr(w.state)),
	)
}

func reviewAttr(s State) string {
	switch {
	case s.AwaitingReview:
		return "waiting"
	case s.Outcome == run.OutcomeReviewTimedOut:
		return "timed-out"
	case s.Review == "":
		return "none"
	}
	return string(s.Review)
}

// stage runs one stage activity with workflow audit entries.
func (w *wf) stage(s run.StageSpec, round int, refs run.Refs, extra []string) (run.StageOutput, error) {
	in := run.StageInput{
		Stage: s.Stage, Phase: s.Phase, Round: round, Request: w.in.Request, Definition: w.in.Definition,
		Refs: refs, Extra: extra,
	}
	in.Key = run.Key(in)
	w.state.CurrentStage = string(s.Stage)
	w.audit(string(s.Stage), "started", in.Key, s.Phase)
	var out run.StageOutput
	err := workflow.ExecuteActivity(w.options(s.Stage), string(s.Stage), in).Get(w.ctx, &out)
	if err != nil {
		status := "failed"
		if temporal.IsCanceledError(err) {
			status = "cancelled"
		}
		w.audit(string(s.Stage), status, in.Key, err.Error())
		return out, err
	}
	w.audit(string(s.Stage), "completed", in.Key, out.Detail)
	w.state.Investigation = out.Refs.Investigation
	return out, nil
}

func (w *wf) finalize(inv string, stage domain.Stage, event string, status domain.InvestigationStatus, detail string) (run.ReviewOutput, error) {
	if inv == "" {
		return run.ReviewOutput{}, nil // no record yet: the workflow history is the audit
	}
	dctx, cancel := workflow.NewDisconnectedContext(w.ctx)
	defer cancel()
	dctx = workflow.WithActivityOptions(dctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3, NonRetryableErrorTypes: []string{ErrTypePermanent}},
	})
	var out run.ReviewOutput
	err := workflow.ExecuteActivity(dctx, ActivityFinalize, run.FinalizeInput{
		Investigation: inv, Stage: stage, Event: event, Status: status, Detail: detail,
	}).Get(dctx, &out)
	return out, err
}

// InvestigationWorkflow runs RoundPlan's stages (DESIGN.md §5.1), then waits
// for the human-review signal when the policy requires it. request-more-
// evidence starts a new round (at most MaxEvidenceRounds); a review timeout
// fails closed (nothing is approved); cancellation records a `cancelled`
// audit event and returns a CanceledError.
func InvestigationWorkflow(ctx workflow.Context, in Input) (Result, error) {
	w := &wf{ctx: ctx, in: in}
	if err := workflow.SetQueryHandler(ctx, QueryState, func() (State, error) { return w.state, nil }); err != nil {
		return Result{}, err
	}
	res := run.Result{}
	done := func(outcome string) (Result, error) {
		res.Outcome, w.state.Outcome = outcome, outcome
		w.state.CurrentStage = string(domain.StageHumanReview)
		w.upsert()
		return Result{Result: res, Audit: w.state.Audit}, nil
	}
	fail := func(err error) (Result, error) {
		if temporal.IsCanceledError(err) || errors.Is(err, workflow.ErrCanceled) || ctx.Err() != nil {
			w.audit(w.state.CurrentStage, "cancelled", "", "workflow cancelled")
			if _, ferr := w.finalize(res.Investigation, domain.StageHumanReview, "cancelled", domain.InvestigationCancelled, "workflow cancelled in "+w.state.CurrentStage); ferr != nil {
				w.audit(w.state.CurrentStage, "failed", "", "could not record cancellation: "+ferr.Error())
			}
			res.Outcome, w.state.Outcome = run.OutcomeCancelled, run.OutcomeCancelled
			return Result{Result: res, Audit: w.state.Audit}, temporal.NewCanceledError("cancelled in " + w.state.CurrentStage)
		}
		w.audit(w.state.CurrentStage, "failed", "", err.Error())
		if _, ferr := w.finalize(res.Investigation, domain.Stage(w.state.CurrentStage), "failed", domain.InvestigationFailed, err.Error()); ferr != nil {
			w.audit(w.state.CurrentStage, "failed", "", "could not record failure: "+ferr.Error())
		}
		return Result{Result: res, Audit: w.state.Audit}, err
	}
	w.upsert()

	var refs run.Refs
	var extra []string
	for round := 0; ; round++ {
		w.state.Round, res.Rounds = round, round
		var last run.StageOutput
		for _, s := range run.RoundPlan(round == 0) {
			out, err := w.stage(s, round, refs, extra)
			if err != nil {
				return fail(err)
			}
			refs, last = out.Refs, out
			res.Investigation, res.Bundle, res.RecordDigest = refs.Investigation, refs.Bundle, out.Digest
		}
		res.Decision, res.Review = last.Outcome, last.Review
		w.state.Decision, w.state.Review = last.Outcome, last.Review
		w.upsert()
		if last.Review == domain.ReviewNotRequired {
			return done(run.OutcomeAutoCompleted)
		}
		dec, state, timedOut, err := w.awaitReview(res.Investigation, round)
		if err != nil {
			return fail(err)
		}
		if timedOut {
			out, err := w.finalize(res.Investigation, domain.StageHumanReview, "timed-out", domain.InvestigationFailed,
				"no decision before the review timeout; failing closed, nothing approved")
			if err != nil {
				return fail(err)
			}
			res.RecordDigest = out.Digest
			return done(run.OutcomeReviewTimedOut)
		}
		res.Review, res.Reviewer, res.RecordDigest = state, dec.Reviewer, w.digest
		w.state.Review = state
		switch state {
		case domain.ReviewApproved:
			return done(run.OutcomeApproved)
		case domain.ReviewRejected:
			return done(run.OutcomeRejected)
		case domain.ReviewMoreEvidenceWanted:
			if round >= run.MaxEvidenceRounds {
				w.audit(string(domain.StageHumanReview), "failed", "", "evidence round limit reached")
				out, err := w.finalize(res.Investigation, domain.StageHumanReview, "exhausted", domain.InvestigationCompleted,
					"evidence round limit reached; nothing approved")
				if err != nil {
					return fail(err)
				}
				res.RecordDigest = out.Digest
				return done(run.OutcomeRejected)
			}
			extra = dec.Requests
			// A new round: same change evidence, new snapshot/bundle (and therefore
			// a new investigation record), new idempotency keys.
			refs = run.Refs{Change: refs.Change, Previous: refs.Investigation}
		}
	}
}

// awaitReview waits for a decision that settles the review. A proxy decision
// is recorded and the wait continues; a decision the applier rejects is
// logged and ignored. The timeout covers the whole wait.
func (w *wf) awaitReview(inv string, round int) (investigate.ReviewDecision, domain.ReviewState, bool, error) {
	timeout := w.in.ReviewTimeout
	if timeout <= 0 {
		timeout = DefaultReviewTimeout
	}
	w.state.CurrentStage = string(domain.StageHumanReview)
	w.state.AwaitingReview = true
	w.upsert()
	defer func() { w.state.AwaitingReview = false }()
	w.audit(string(domain.StageHumanReview), "waiting", "", "timeout "+timeout.String())

	ch := workflow.GetSignalChannel(w.ctx, SignalReview)
	timerCtx, cancelTimer := workflow.WithCancel(w.ctx)
	defer cancelTimer()
	timer := workflow.NewTimer(timerCtx, timeout)
	for {
		var d investigate.ReviewDecision
		got, timedOut := false, false
		sel := workflow.NewSelector(w.ctx)
		sel.AddReceive(ch, func(c workflow.ReceiveChannel, _ bool) { c.Receive(w.ctx, &d); got = true })
		sel.AddFuture(timer, func(f workflow.Future) {
			if f.Get(w.ctx, nil) == nil {
				timedOut = true
			}
		})
		sel.Select(w.ctx)
		switch {
		case w.ctx.Err() != nil:
			return d, "", false, w.ctx.Err()
		case timedOut:
			w.audit(string(domain.StageHumanReview), "timed-out", "", "no decision; failing closed")
			return d, "", true, nil
		case !got:
			continue
		}
		if d.At.IsZero() {
			d.At = workflow.Now(w.ctx)
		}
		w.audit(string(domain.StageHumanReview), "signal", "", string(d.Action)+" by "+d.Reviewer+" ("+string(d.ReviewerKind)+")")
		var out run.ReviewOutput
		err := workflow.ExecuteActivity(w.reviewOptions(), ActivityReview, run.ReviewInput{Investigation: inv, Round: round, Decision: d}).Get(w.ctx, &out)
		switch {
		case err != nil && (temporal.IsCanceledError(err) || w.ctx.Err() != nil):
			return d, "", false, err
		case err != nil:
			w.audit(string(domain.StageHumanReview), "failed", "", "decision not applied: "+err.Error())
			continue
		}
		w.state.Review, w.digest = out.State, out.Digest
		res := out.State
		if res == domain.ReviewAwaiting {
			w.audit(string(domain.StageHumanReview), "waiting", "", "decision recorded; still awaiting a human")
			continue
		}
		return d, res, false, nil
	}
}

func (w *wf) reviewOptions() workflow.Context {
	return workflow.WithActivityOptions(w.ctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3, NonRetryableErrorTypes: []string{ErrTypePermanent}},
	})
}

type registrar interface {
	RegisterWorkflowWithOptions(any, workflow.RegisterOptions)
	RegisterActivityWithOptions(any, activity.RegisterOptions)
}

// Register wires the workflow and every activity onto a worker (or a test
// environment).
func Register(r registrar, a *Activities) {
	r.RegisterWorkflowWithOptions(InvestigationWorkflow, workflow.RegisterOptions{Name: WorkflowType})
	for _, s := range domain.Stages {
		if s == domain.StageHumanReview {
			continue
		}
		r.RegisterActivityWithOptions(a.Stage(s), activity.RegisterOptions{Name: string(s)})
	}
	r.RegisterActivityWithOptions(a.Review, activity.RegisterOptions{Name: ActivityReview})
	r.RegisterActivityWithOptions(a.Finalize, activity.RegisterOptions{Name: ActivityFinalize})
}
