package run

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

func (f *fixture) runner(src ReviewSource) *Runner {
	return &Runner{P: New(f.Deps), Review: src, Sleep: func(time.Duration) {}}
}

func once(d *investigate.ReviewDecision) ReviewSource {
	return func(context.Context, *domain.Investigation) (*investigate.ReviewDecision, error) { return d, nil }
}

func TestRunAwaitsReviewAndValidates(t *testing.T) {
	f := newFixture(t)
	res, err := f.runner(nil).Run(context.Background(), f.request())
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != OutcomeAwaitingReview || res.Review != domain.ReviewAwaiting {
		t.Fatalf("result = %+v", res)
	}
	inv, err := f.St.GetInvestigation(context.Background(), res.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := f.St.GetBundle(context.Background(), inv.Bundle)
	if err := inv.ValidateAgainst(b); err != nil {
		t.Fatalf("record does not validate against its bundle: %v", err)
	}
	if inv.Status != domain.InvestigationAwaiting || inv.Agent == nil || inv.Agent.Status != domain.AgentNotRun {
		t.Fatalf("status/agent = %s / %+v", inv.Status, inv.Agent)
	}
	if inv.Traversal == nil || inv.Graph == "" || len(inv.Findings) == 0 {
		t.Fatalf("record is incomplete: %+v", inv)
	}
	// Every stage left started+completed, in pipeline order.
	var stages []domain.Stage
	for _, e := range inv.Audit {
		if e.Event == "completed" {
			stages = append(stages, e.Stage)
		}
	}
	want := []domain.Stage{domain.StageCollectChange, domain.StageBuildSnapshot, domain.StageAssemble, domain.StageBuildGraph,
		domain.StageTraverse, domain.StageValidate, domain.StageInvestigate, domain.StageTraverse, domain.StageValidate, domain.StageClassify}
	if len(stages) != len(want) {
		t.Fatalf("completed stages = %v", stages)
	}
	for i := range want {
		if stages[i] != want[i] {
			t.Fatalf("completed stages = %v, want %v", stages, want)
		}
	}
}

func TestReviewApproveRejectMoreEvidence(t *testing.T) {
	for _, tc := range []struct {
		action  investigate.ReviewAction
		outcome string
	}{{investigate.ReviewApprove, OutcomeApproved}, {investigate.ReviewReject, OutcomeRejected}} {
		f := newFixture(t)
		res, err := f.runner(once(human(tc.action))).Run(context.Background(), f.request())
		if err != nil {
			t.Fatal(err)
		}
		if res.Outcome != tc.outcome || res.Reviewer != "alice" {
			t.Errorf("%s: result = %+v", tc.action, res)
		}
		inv, _ := f.St.GetInvestigation(context.Background(), res.Investigation)
		if inv.Status != domain.InvestigationCompleted {
			t.Errorf("%s: status = %s", tc.action, inv.Status)
		}
	}
}

func TestMoreEvidenceIsBounded(t *testing.T) {
	f := newFixture(t)
	n := 0
	src := func(context.Context, *domain.Investigation) (*investigate.ReviewDecision, error) {
		n++
		d := human(investigate.ReviewRequestMoreEvidence)
		d.Requests = []string{"nonsense"}
		return d, nil
	}
	res, err := f.runner(src).Run(context.Background(), f.request())
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != OutcomeRejected || res.Rounds != MaxEvidenceRounds || n != MaxEvidenceRounds+1 {
		t.Fatalf("result = %+v after %d reviews", res, n)
	}
}

func TestProxyDecisionNeverApproves(t *testing.T) {
	f := newFixture(t)
	calls := 0
	src := func(context.Context, *domain.Investigation) (*investigate.ReviewDecision, error) {
		calls++
		if calls == 1 {
			return &investigate.ReviewDecision{Action: investigate.ReviewApprove, Reviewer: "claude", ReviewerKind: domain.ReviewerProxy, At: fixedNow}, nil
		}
		return nil, nil
	}
	res, err := f.runner(src).Run(context.Background(), f.request())
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome == OutcomeApproved || res.Review != domain.ReviewAwaiting {
		t.Fatalf("a proxy decision settled the review: %+v", res)
	}
}

func TestReviewTimeoutFailsClosed(t *testing.T) {
	f := newFixture(t)
	r := f.runner(func(ctx context.Context, _ *domain.Investigation) (*investigate.ReviewDecision, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	r.ReviewTimeout = 20 * time.Millisecond
	res, err := r.Run(context.Background(), f.request())
	if err != nil {
		t.Fatal(err)
	}
	inv, _ := f.St.GetInvestigation(context.Background(), res.Investigation)
	if res.Outcome != OutcomeReviewTimedOut || inv.Review.State == domain.ReviewApproved || inv.Status != domain.InvestigationFailed {
		t.Fatalf("result = %+v, status %s, review %s", res, inv.Status, inv.Review.State)
	}
	last := inv.Audit[len(inv.Audit)-1]
	if last.Event != "timed-out" {
		t.Fatalf("last audit event = %+v", last)
	}
}

type flakySnapshot struct {
	investigate.SnapshotBuilder
	fail, calls int
}

func (f *flakySnapshot) BuildSnapshot(ctx context.Context, in investigate.EnvironmentInputs) (*investigate.SnapshotResult, error) {
	f.calls++
	if f.calls <= f.fail {
		return nil, errors.New("registry timeout")
	}
	return f.SnapshotBuilder.BuildSnapshot(ctx, in)
}

func TestTransientRetryThenSuccessAndAudit(t *testing.T) {
	f := newFixture(t)
	fl := &flakySnapshot{SnapshotBuilder: f.Deps.Snapshot, fail: 2}
	f.Deps.Snapshot = fl
	res, err := f.runner(nil).Run(context.Background(), f.request())
	if err != nil {
		t.Fatal(err)
	}
	if fl.calls != 3 {
		t.Fatalf("snapshot attempts = %d", fl.calls)
	}
	inv, _ := f.St.GetInvestigation(context.Background(), res.Investigation)
	var seq []string
	for _, e := range inv.Audit {
		if e.Stage == domain.StageBuildSnapshot {
			seq = append(seq, e.Event)
		}
	}
	want := []string{"started", "failed", "retried", "failed", "retried", "completed"}
	if len(seq) != len(want) {
		t.Fatalf("snapshot audit = %v", seq)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("snapshot audit = %v, want %v", seq, want)
		}
	}
}

func TestTransientExhaustionFails(t *testing.T) {
	f := newFixture(t)
	f.Class.fail = 5 // classify allows one attempt
	_, err := f.runner(nil).Run(context.Background(), f.request())
	if err == nil || investigate.ClassOf(err) != investigate.Transient {
		t.Fatalf("err = %v", err)
	}
}

func TestPermanentFailureIsNotRetried(t *testing.T) {
	f := newFixture(t)
	f.Deps.Classifier = permanentClassifier{}
	_, err := f.runner(nil).Run(context.Background(), f.request())
	if err == nil || investigate.ClassOf(err) != investigate.Permanent {
		t.Fatalf("err = %v", err)
	}
}

type permanentClassifier struct{}

func (permanentClassifier) Classify(context.Context, investigate.ClassifyInput) (*investigate.Classification, error) {
	return nil, investigate.PermanentError(domain.StageClassify, errors.New("bad input"))
}

func TestResubmissionIsIdempotent(t *testing.T) {
	f := newFixture(t)
	a, err := f.runner(nil).Run(context.Background(), f.request())
	if err != nil {
		t.Fatal(err)
	}
	calls := f.Class.calls
	b, err := f.runner(nil).Run(context.Background(), f.request())
	if err != nil {
		t.Fatal(err)
	}
	if a.Investigation != b.Investigation || a.RecordDigest != b.RecordDigest || f.Class.calls != calls {
		t.Fatalf("resubmission recomputed: %+v vs %+v (classifier calls %d -> %d)", a, b, calls, f.Class.calls)
	}
}
