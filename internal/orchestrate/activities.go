package orchestrate

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/run"
)

// Activities are the stage activities over a run.Pipeline: one activity per
// domain.Stage (named by the stage), plus human-review and finalize.
type Activities struct{ P *run.Pipeline }

// toTemporal maps the retry class (DESIGN.md §5.2) to an application error:
// permanent is non-retryable; transient (the default) is retried within the
// stage's policy. Cancellation passes through.
func toTemporal(ctx context.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return ctx.Err()
	case investigate.ClassOf(err) == investigate.Permanent:
		return temporal.NewNonRetryableApplicationError(err.Error(), ErrTypePermanent, err)
	}
	return temporal.NewApplicationError(err.Error(), ErrTypeTransient, err)
}

// heartbeating runs f while heartbeating, so a long single call into a port
// (a render, a model session) still shows liveness and a lost worker is
// detected within the heartbeat timeout.
func heartbeating(ctx context.Context, f func(ctx context.Context) error) error {
	hb := func(detail string) { activity.RecordHeartbeat(ctx, detail) }
	ctx = run.WithHeartbeat(ctx, hb)
	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				hb("working")
			}
		}
	}()
	return f(ctx)
}

// Stage returns the activity function of one stage.
func (a *Activities) Stage(stage domain.Stage) func(context.Context, run.StageInput) (run.StageOutput, error) {
	return func(ctx context.Context, in run.StageInput) (run.StageOutput, error) {
		in.Stage = stage
		in.Attempt = int(activity.GetInfo(ctx).Attempt)
		var out run.StageOutput
		err := heartbeating(ctx, func(ctx context.Context) (err error) {
			out, err = a.P.Run(ctx, in)
			return err
		})
		return out, toTemporal(ctx, err)
	}
}

// Review applies a human-review decision (stage human-review).
func (a *Activities) Review(ctx context.Context, in run.ReviewInput) (run.ReviewOutput, error) {
	out, err := a.P.ApplyReview(ctx, in)
	return out, toTemporal(ctx, err)
}

// Finalize ends a run without a decision. It must succeed even when the
// workflow was cancelled, so the workflow calls it on a disconnected context.
func (a *Activities) Finalize(ctx context.Context, in run.FinalizeInput) (run.ReviewOutput, error) {
	out, err := a.P.Finalize(ctx, in)
	return out, toTemporal(ctx, err)
}
