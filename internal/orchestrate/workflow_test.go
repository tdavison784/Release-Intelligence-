package orchestrate_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/run"
	"github.com/tdavison784/release-intelligence/internal/investigate/run/runtest"
	"github.com/tdavison784/release-intelligence/internal/orchestrate"
)

type harness struct {
	f   *runtest.Fixture
	env *testsuite.TestWorkflowEnvironment
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	f := runtest.New(t)
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	return &harness{f: f, env: env}
}

// start registers the activities over the fixture's current Deps and runs
// the workflow; cbs schedule signals and queries before it starts.
func (h *harness) start(in orchestrate.Input) {
	orchestrate.Register(h.env, &orchestrate.Activities{P: run.New(h.f.Deps)})
	h.env.ExecuteWorkflow(orchestrate.WorkflowType, in)
}

func (h *harness) input() orchestrate.Input {
	return orchestrate.Input{Request: h.f.Request(), ReviewTimeout: 24 * time.Hour}
}

func (h *harness) signalAt(d time.Duration, dec *investigate.ReviewDecision) {
	h.env.RegisterDelayedCallback(func() { h.env.SignalWorkflow(orchestrate.SignalReview, *dec) }, d)
}

func (h *harness) result(t *testing.T) orchestrate.Result {
	t.Helper()
	if !h.env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	var res orchestrate.Result
	if err := h.env.GetWorkflowResult(&res); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	return res
}

func TestHappyPathApprove(t *testing.T) {
	h := newHarness(t)
	var mid orchestrate.State
	h.env.RegisterDelayedCallback(func() {
		v, err := h.env.QueryWorkflow(orchestrate.QueryState)
		if err != nil {
			t.Error(err)
			return
		}
		_ = v.Get(&mid)
	}, time.Hour)
	h.signalAt(2*time.Hour, runtest.Human(investigate.ReviewApprove))
	h.start(h.input())
	res := h.result(t)
	if res.Outcome != run.OutcomeApproved || res.Reviewer != "alice" || res.Review != domain.ReviewApproved {
		t.Fatalf("result = %+v", res.Result)
	}
	if !mid.AwaitingReview || mid.CurrentStage != string(domain.StageHumanReview) || mid.Investigation != res.Investigation {
		t.Fatalf("state while waiting = %+v", mid)
	}
	inv, err := h.f.St.GetInvestigation(context.Background(), res.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != domain.InvestigationCompleted || inv.Review.State != domain.ReviewApproved {
		t.Fatalf("record = %s / %s", inv.Status, inv.Review.State)
	}
	if run.Digest(inv) != res.RecordDigest {
		t.Fatal("result digest is not the record's digest")
	}
	// The workflow log has every stage started+completed.
	done := 0
	for _, a := range res.Audit {
		if a.Status == "completed" {
			done++
		}
	}
	if done != len(run.RoundPlan(true)) {
		t.Fatalf("completed entries = %d: %+v", done, res.Audit)
	}
}

func TestReject(t *testing.T) {
	h := newHarness(t)
	h.signalAt(time.Hour, runtest.Human(investigate.ReviewReject))
	h.start(h.input())
	if res := h.result(t); res.Outcome != run.OutcomeRejected {
		t.Fatalf("result = %+v", res.Result)
	}
}

func TestMoreEvidenceStartsNewRoundsAndIsBounded(t *testing.T) {
	h := newHarness(t)
	for i := 1; i <= run.MaxEvidenceRounds+1; i++ {
		d := runtest.Human(investigate.ReviewRequestMoreEvidence)
		d.Requests = []string{"not-fulfillable"}
		h.signalAt(time.Duration(i)*time.Hour, d)
	}
	h.start(h.input())
	res := h.result(t)
	if res.Outcome != run.OutcomeRejected || res.Rounds != run.MaxEvidenceRounds {
		t.Fatalf("result = %+v", res.Result)
	}
}

func TestMoreEvidenceThenApprove(t *testing.T) {
	h := newHarness(t)
	h.signalAt(time.Hour, runtest.Human(investigate.ReviewRequestMoreEvidence))
	h.signalAt(2*time.Hour, runtest.Human(investigate.ReviewApprove))
	h.start(h.input())
	res := h.result(t)
	if res.Outcome != run.OutcomeApproved || res.Rounds != 1 {
		t.Fatalf("result = %+v", res.Result)
	}
	// Nothing new was supplied, so round 1 reuses the record and re-runs the later stages on it.
	inv, _ := h.f.St.GetInvestigation(context.Background(), res.Investigation)
	classified := 0
	for _, e := range inv.Audit {
		if e.Stage == domain.StageClassify && e.Event == "completed" {
			classified++
		}
	}
	if classified != 2 || inv.Review.State != domain.ReviewApproved {
		t.Fatalf("classify ran %d times, review %s", classified, inv.Review.State)
	}
}

func TestProxyDecisionDoesNotApprove(t *testing.T) {
	h := newHarness(t)
	h.signalAt(time.Hour, &investigate.ReviewDecision{Action: investigate.ReviewApprove, Reviewer: "claude", ReviewerKind: domain.ReviewerProxy, At: runtest.FixedNow})
	h.start(h.input())
	res := h.result(t)
	if res.Outcome != run.OutcomeReviewTimedOut {
		t.Fatalf("a proxy approval settled the review: %+v", res.Result)
	}
}

func TestInvalidDecisionIsIgnored(t *testing.T) {
	h := newHarness(t)
	h.signalAt(time.Hour, &investigate.ReviewDecision{Action: investigate.ReviewApprove, ReviewerKind: domain.ReviewerHuman}) // no reviewer
	h.signalAt(2*time.Hour, runtest.Human(investigate.ReviewReject))
	h.start(h.input())
	if res := h.result(t); res.Outcome != run.OutcomeRejected {
		t.Fatalf("result = %+v", res.Result)
	}
}

func TestReviewTimeoutFailsClosed(t *testing.T) {
	h := newHarness(t)
	h.start(h.input())
	res := h.result(t)
	if res.Outcome != run.OutcomeReviewTimedOut || res.Review == domain.ReviewApproved {
		t.Fatalf("result = %+v", res.Result)
	}
	inv, _ := h.f.St.GetInvestigation(context.Background(), res.Investigation)
	if inv.Status != domain.InvestigationFailed || inv.Audit[len(inv.Audit)-1].Event != "timed-out" {
		t.Fatalf("record = %s, last event %+v", inv.Status, inv.Audit[len(inv.Audit)-1])
	}
}

func TestCancelWhileAwaitingReview(t *testing.T) {
	h := newHarness(t)
	h.env.RegisterDelayedCallback(h.env.CancelWorkflow, time.Hour)
	h.start(h.input())
	if !h.env.IsWorkflowCompleted() || h.env.GetWorkflowError() == nil {
		t.Fatal("expected a cancelled workflow")
	}
	var st orchestrate.State
	v, err := h.env.QueryWorkflow(orchestrate.QueryState)
	if err != nil {
		t.Fatal(err)
	}
	_ = v.Get(&st)
	inv, err := h.f.St.GetInvestigation(context.Background(), st.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != domain.InvestigationCancelled || inv.Audit[len(inv.Audit)-1].Event != "cancelled" {
		t.Fatalf("record = %s, last event %+v", inv.Status, inv.Audit[len(inv.Audit)-1])
	}
	if st.Outcome != run.OutcomeCancelled {
		t.Fatalf("state = %+v", st)
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

func TestTransientFailureIsRetried(t *testing.T) {
	h := newHarness(t)
	fl := &flakySnapshot{SnapshotBuilder: h.f.Deps.Snapshot, fail: 2}
	h.f.Deps.Snapshot = fl
	h.signalAt(time.Hour, runtest.Human(investigate.ReviewApprove))
	h.start(h.input())
	if res := h.result(t); res.Outcome != run.OutcomeApproved || fl.calls != 3 {
		t.Fatalf("outcome %s after %d snapshot attempts", res.Outcome, fl.calls)
	}
}

func TestTransientExhaustionFailsWorkflow(t *testing.T) {
	h := newHarness(t)
	fl := &flakySnapshot{SnapshotBuilder: h.f.Deps.Snapshot, fail: 99}
	h.f.Deps.Snapshot = fl
	h.start(h.input())
	if h.env.GetWorkflowError() == nil || fl.calls != run.PolicyOf(domain.StageBuildSnapshot, investigate.Config{}).MaxAttempts {
		t.Fatalf("error %v after %d attempts", h.env.GetWorkflowError(), fl.calls)
	}
}

type permanentClassifier struct{ calls int }

func (c *permanentClassifier) Classify(context.Context, investigate.ClassifyInput) (*investigate.Classification, error) {
	c.calls++
	return nil, investigate.PermanentError(domain.StageClassify, errors.New("unsupported input"))
}

func TestNonRetryableFailure(t *testing.T) {
	h := newHarness(t)
	pc := &permanentClassifier{}
	h.f.Deps.Classifier = pc
	h.start(h.input())
	if h.env.GetWorkflowError() == nil || pc.calls != 1 {
		t.Fatalf("error %v after %d calls", h.env.GetWorkflowError(), pc.calls)
	}
	var st orchestrate.State
	v, _ := h.env.QueryWorkflow(orchestrate.QueryState)
	_ = v.Get(&st)
	inv, _ := h.f.St.GetInvestigation(context.Background(), st.Investigation)
	if inv == nil || inv.Status != domain.InvestigationFailed {
		t.Fatalf("record = %+v", inv)
	}
}

func TestWorkflowIDIsContentDerived(t *testing.T) {
	f := runtest.New(t)
	a, b := f.Request(), f.Request()
	if orchestrate.WorkflowID(a, 0) != orchestrate.WorkflowID(b, 0) || orchestrate.WorkflowID(a, 1) == orchestrate.WorkflowID(a, 0) {
		t.Fatal("workflow id is not stable per request")
	}
	b.To = "v2.0.1"
	if orchestrate.WorkflowID(a, 0) == orchestrate.WorkflowID(b, 0) {
		t.Fatal("different request, same workflow id")
	}
}

// The in-process runner and the Temporal path produce the same run record
// digest for the same inputs (both with the scripted agent; fixed clock).
func TestInProcessAndTemporalRecordDigestsMatch(t *testing.T) {
	approve := runtest.Human(investigate.ReviewApprove)
	// In-process.
	f1 := runtest.New(t)
	r := &run.Runner{P: run.New(f1.Deps), Sleep: func(time.Duration) {},
		Review: func(context.Context, *domain.Investigation) (*investigate.ReviewDecision, error) { return approve, nil }}
	a, err := r.Run(context.Background(), f1.Request())
	if err != nil {
		t.Fatal(err)
	}
	// Temporal.
	h := newHarness(t)
	h.f.Repo = f1.Repo // the repo path is part of the snapshot, hence of the ids
	h.signalAt(time.Hour, approve)
	h.start(h.input())
	b := h.result(t)
	if a.Investigation != b.Investigation {
		t.Fatalf("investigation ids differ: %s vs %s", a.Investigation, b.Investigation)
	}
	if a.RecordDigest == "" || a.RecordDigest != b.RecordDigest {
		t.Fatalf("record digests differ: %s vs %s", a.RecordDigest, b.RecordDigest)
	}
}

// Parity with the in-process runner (run.TestAgentRequestsKeepAuditMonotonic):
// an agent that makes requests yields a valid, time-monotonic record here too.
func TestAgentRequestsKeepAuditMonotonic(t *testing.T) {
	h := newHarness(t)
	h.f.Deps.Now = runtest.TickClock()
	h.f.Deps.Investigator = &runtest.RequestingAgent{St: h.f.St}
	dec := runtest.Human(investigate.ReviewApprove)
	dec.At = runtest.FixedNow.Add(time.Hour) // after every event the ticking clock stamps
	h.signalAt(time.Hour, dec)
	in := h.input()
	in.Request.Config.Agent.Enabled = true
	h.start(in)
	res := h.result(t)
	inv, err := h.f.St.GetInvestigation(context.Background(), res.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtest.CheckAuditMonotonic(inv); err != nil {
		t.Fatal(err)
	}
}
