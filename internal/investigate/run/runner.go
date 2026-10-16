package run

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// ReviewSource supplies the human decision for a run awaiting review (the
// CLI flags, a prompt). Returning (nil, nil) means "no decision now": the
// run is left awaiting review. A deadline error means the review timed out.
type ReviewSource func(ctx context.Context, inv *domain.Investigation) (*investigate.ReviewDecision, error)

// Runner is the plain sequential in-process runner (`ri investigate`
// without --temporal). It drives the same Pipeline stages in the same order
// with the same retry policy as the Temporal workflow.
type Runner struct {
	P *Pipeline
	// Definition returns the digest of a product's definition (ChangeKey).
	Definition func(product string) string
	Review     ReviewSource
	// ReviewTimeout bounds each wait for a decision (0 = none in-process).
	ReviewTimeout time.Duration
	// Sleep waits between retries (default time.Sleep; tests pass a no-op).
	Sleep func(time.Duration)
	// Logf reports progress (optional).
	Logf func(format string, args ...any)
}

func (r *Runner) logf(f string, a ...any) {
	if r.Logf != nil {
		r.Logf(f, a...)
	}
}

// step runs one stage with the stage's retry policy: permanent errors stop
// at once, transient ones are retried with exponential backoff.
func (r *Runner) step(ctx context.Context, in StageInput) (StageOutput, error) {
	in.Key = Key(in)
	pol := PolicyOf(in.Stage, in.Request.Config)
	sleep := r.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	wait := pol.Initial
	var err error
	for attempt := 1; attempt <= pol.MaxAttempts; attempt++ {
		in.Attempt = attempt
		var out StageOutput
		out, err = r.P.Run(ctx, in)
		if err == nil {
			r.logf("%s %s: %s", in.Stage, in.Phase, out.Detail)
			return out, nil
		}
		if investigate.ClassOf(err) == investigate.Permanent || ctx.Err() != nil || attempt == pol.MaxAttempts {
			break
		}
		r.logf("%s failed (attempt %d/%d), retrying: %v", in.Stage, attempt, pol.MaxAttempts, err)
		sleep(wait)
		wait = time.Duration(float64(wait) * pol.Coefficient)
		if wait > pol.Max {
			wait = pol.Max
		}
	}
	return StageOutput{}, err
}

// Run investigates one request to the end of the review.
func (r *Runner) Run(ctx context.Context, req investigate.Request) (*Result, error) {
	def := ""
	if r.Definition != nil {
		def = r.Definition(req.Product)
	}
	var refs Refs
	var extra []string
	res := &Result{}
	for round := 0; ; round++ {
		var last StageOutput
		for _, s := range RoundPlan(round == 0) {
			in := StageInput{Stage: s.Stage, Phase: s.Phase, Round: round, Request: req, Definition: def, Refs: refs, Extra: extra}
			out, err := r.step(ctx, in)
			if err != nil {
				if ctx.Err() != nil && res.Investigation != "" {
					r.finalize(res.Investigation, domain.StageHumanReview, "cancelled", domain.InvestigationCancelled, "run cancelled")
				}
				return res, fmt.Errorf("%s: %w", s.Stage, err)
			}
			refs, last = out.Refs, out
			res.Investigation, res.Bundle, res.RecordDigest, res.Rounds = refs.Investigation, refs.Bundle, out.Digest, round
		}
		res.Decision, res.Review = last.Outcome, last.Review
		if last.Review == domain.ReviewNotRequired {
			res.Outcome = OutcomeAutoCompleted
			return res, nil
		}
		dec, state, err := r.awaitDecision(ctx, res)
		if err != nil {
			return res, err
		}
		if state != "" && state != "timed-out" {
			res.Review = state
		}
		switch {
		case dec == nil && state == "":
			res.Outcome = OutcomeAwaitingReview
			return res, nil
		case state == "timed-out":
			res.Outcome = OutcomeReviewTimedOut
			return res, nil
		}
		res.Reviewer = dec.Reviewer
		switch state {
		case domain.ReviewApproved:
			res.Outcome = OutcomeApproved
			return res, nil
		case domain.ReviewRejected:
			res.Outcome = OutcomeRejected
			return res, nil
		case domain.ReviewMoreEvidenceWanted:
			if round >= MaxEvidenceRounds {
				r.finalize(res.Investigation, domain.StageHumanReview, "exhausted", domain.InvestigationCompleted, "evidence round limit reached; nothing approved")
				res.Outcome = OutcomeRejected
				return res, nil
			}
			extra = dec.Requests
			refs = Refs{Change: refs.Change, Previous: refs.Investigation}
		}
	}
}

func (r *Runner) finalize(id string, stage domain.Stage, event string, status domain.InvestigationStatus, detail string) {
	_, _ = r.P.Finalize(context.WithoutCancel(context.Background()), FinalizeInput{Investigation: id, Stage: stage, Event: event, Status: status, Detail: detail})
}

// awaitDecision waits for decisions until one settles the review. A proxy
// decision is recorded and the wait continues. It returns the settled
// decision and the review state; ("", nil, nil) when there is no source.
func (r *Runner) awaitDecision(ctx context.Context, res *Result) (*investigate.ReviewDecision, domain.ReviewState, error) {
	if r.Review == nil {
		return nil, "", nil
	}
	wctx := ctx
	if r.ReviewTimeout > 0 {
		var cancel context.CancelFunc
		wctx, cancel = context.WithTimeout(ctx, r.ReviewTimeout)
		defer cancel()
	}
	for {
		inv, err := r.P.d.Records.GetInvestigation(ctx, res.Investigation)
		if err != nil {
			return nil, "", err
		}
		dec, err := r.Review(wctx, inv)
		switch {
		case errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil:
			r.finalize(res.Investigation, domain.StageHumanReview, "timed-out", domain.InvestigationFailed, "no decision before the review timeout; failing closed, nothing approved")
			return nil, "timed-out", nil
		case err != nil:
			return nil, "", err
		case dec == nil:
			return nil, "", nil
		}
		out, err := r.P.ApplyReview(ctx, ReviewInput{Investigation: res.Investigation, Round: res.Rounds, Decision: *dec})
		if err != nil {
			if errors.Is(err, investigate.ErrRejected) {
				r.logf("review decision rejected: %v", err)
				continue
			}
			return nil, "", err
		}
		res.RecordDigest = out.Digest
		if out.State != domain.ReviewAwaiting {
			return dec, out.State, nil
		}
	}
}
