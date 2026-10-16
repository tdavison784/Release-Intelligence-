package orchestrate

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
)

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func isErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func eq(t *testing.T, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func ne(t *testing.T, a, b any) {
	t.Helper()
	if reflect.DeepEqual(a, b) {
		t.Fatalf("expected values to differ, both %v", a)
	}
}

func isTrue(t *testing.T, v bool, msg string) {
	t.Helper()
	if !v {
		t.Fatal(msg)
	}
}

func empty[T any](t *testing.T, s []T) {
	t.Helper()
	if len(s) != 0 {
		t.Fatalf("expected empty, got %v", s)
	}
}

func testInput() InvestigationInput {
	return InvestigationInput{
		Dependency: "example/dep", FromVersion: "1.0.0", ToVersion: "2.0.0",
		Environment: "prod", EnvInputRef: ArtifactRef{Digest: "sha256:env"},
		ReviewTimeout: time.Hour,
	}
}

func newEnv(t *testing.T, a *Activities) *testsuite.TestWorkflowEnvironment {
	var s testsuite.WorkflowTestSuite
	env := s.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(InvestigationWorkflow)
	env.RegisterActivityWithOptions(a.Run, registerRun)
	return env
}

func signalAfter(env *testsuite.TestWorkflowEnvironment, d time.Duration, dec ReviewDecision) {
	env.RegisterDelayedCallback(func() { env.SignalWorkflow(SignalReview, dec) }, d)
}

func statuses(audit []AuditEntry, stage string) []string {
	var out []string
	for _, e := range audit {
		if e.Stage == stage {
			out = append(out, e.Status)
		}
	}
	return out
}

func TestHappyPathAutoCompletes(t *testing.T) {
	a := &Activities{Classification: ClassNotAffected}
	env := newEnv(t, a)
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	noErr(t, env.GetWorkflowError())
	var res InvestigationResult
	noErr(t, env.GetWorkflowResult(&res))
	eq(t, OutcomeAutoCompleted, res.Outcome)
	eq(t, ClassNotAffected, res.Classification)
	eq(t, 6, len(res.Artifacts))
	for _, s := range []string{StageCollectChange, StageEnvSnapshot, StageGraph, StageInvestigate, StageValidate, StageClassify} {
		eq(t, []string{"started", "completed"}, statuses(res.Audit, s))
	}
	empty(t, statuses(res.Audit, StageReview))
	for i, e := range res.Audit {
		eq(t, i+1, e.Seq)
	}
}

func TestIdempotencyKeysAreContentDerived(t *testing.T) {
	a := []ArtifactRef{{Digest: "x"}, {Digest: "y"}}
	eq(t, IdempotencyKey("s", a), IdempotencyKey("s", a))
	ne(t, IdempotencyKey("s", a), IdempotencyKey("t", a))
	ne(t, IdempotencyKey("s", a), IdempotencyKey("s", a[:1]))
	eq(t, WorkflowID(testInput()), WorkflowID(testInput()))
	other := testInput()
	other.ToVersion = "2.0.1"
	ne(t, WorkflowID(testInput()), WorkflowID(other))
}

func TestRetryThenSucceed(t *testing.T) {
	a := &Activities{Classification: ClassNotAffected, TransientFailures: map[string]int{StageEnvSnapshot: 2}}
	env := newEnv(t, a)
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	noErr(t, env.GetWorkflowError())
	eq(t, 3, a.Attempts(StageEnvSnapshot))
}

func TestTransientExhaustsRetries(t *testing.T) {
	a := &Activities{TransientFailures: map[string]int{StageGraph: 100}}
	env := newEnv(t, a)
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	isErr(t, env.GetWorkflowError())
	eq(t, 4, a.Attempts(StageGraph))
}

func TestNonRetryableFailure(t *testing.T) {
	a := &Activities{PermanentFailure: map[string]bool{StageCollectChange: true}}
	env := newEnv(t, a)
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	err := env.GetWorkflowError()
	isErr(t, err)
	eq(t, 1, a.Attempts(StageCollectChange))
	var appErr *temporal.ApplicationError
	isTrue(t, errors.As(err, &appErr), "want ApplicationError")
	eq(t, ErrTypeInvalidInput, appErr.Type())
}

func TestReviewApprove(t *testing.T) {
	a := &Activities{}
	env := newEnv(t, a)
	signalAfter(env, time.Minute, ReviewDecision{Decision: DecisionApprove, Reviewer: "alice"})
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	noErr(t, env.GetWorkflowError())
	var res InvestigationResult
	noErr(t, env.GetWorkflowResult(&res))
	eq(t, OutcomeApproved, res.Outcome)
	eq(t, "alice", res.Reviewer)
	eq(t, []string{"waiting", "signal"}, statuses(res.Audit, StageReview))
}

func TestReviewReject(t *testing.T) {
	env := newEnv(t, &Activities{})
	signalAfter(env, time.Minute, ReviewDecision{Decision: DecisionReject, Reviewer: "bob"})
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	var res InvestigationResult
	noErr(t, env.GetWorkflowResult(&res))
	eq(t, OutcomeRejected, res.Outcome)
}

func TestRequestMoreEvidenceThenApprove(t *testing.T) {
	a := &Activities{}
	env := newEnv(t, a)
	signalAfter(env, time.Minute, ReviewDecision{Decision: DecisionMoreEvidence, Reviewer: "a", Note: "check overlays"})
	signalAfter(env, 2*time.Minute, ReviewDecision{Decision: DecisionApprove, Reviewer: "a"})
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	noErr(t, env.GetWorkflowError())
	var res InvestigationResult
	noErr(t, env.GetWorkflowResult(&res))
	eq(t, OutcomeApproved, res.Outcome)
	eq(t, 1, res.Rounds)
	eq(t, 2, a.Attempts(StageInvestigate))
	eq(t, 1, a.Attempts(StageCollectChange))
}

func TestMoreEvidenceIsBounded(t *testing.T) {
	env := newEnv(t, &Activities{})
	for i := 1; i <= MaxEvidenceRounds+1; i++ {
		signalAfter(env, time.Duration(i)*time.Minute, ReviewDecision{Decision: DecisionMoreEvidence, Reviewer: "a"})
	}
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	var res InvestigationResult
	noErr(t, env.GetWorkflowResult(&res))
	eq(t, OutcomeRejected, res.Outcome)
	eq(t, MaxEvidenceRounds, res.Rounds)
}

func TestReviewTimeoutFailsClosed(t *testing.T) {
	env := newEnv(t, &Activities{})
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	noErr(t, env.GetWorkflowError())
	var res InvestigationResult
	noErr(t, env.GetWorkflowResult(&res))
	eq(t, OutcomeReviewTimedOut, res.Outcome)
	eq(t, []string{"waiting", "timed-out"}, statuses(res.Audit, StageReview))
}

func TestCancelWhileAwaitingReview(t *testing.T) {
	env := newEnv(t, &Activities{})
	env.RegisterDelayedCallback(func() {
		v, err := env.QueryWorkflow(QueryState)
		noErr(t, err)
		var st State
		noErr(t, v.Get(&st))
		isTrue(t, st.AwaitingReview, "want awaitingReview")
		eq(t, StageReview, st.CurrentStage)
		env.CancelWorkflow()
	}, time.Minute)
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	err := env.GetWorkflowError()
	isErr(t, err)
	isTrue(t, temporal.IsCanceledError(err), "want CanceledError, got "+err.Error())
}

func TestQueryStateAfterCompletion(t *testing.T) {
	env := newEnv(t, &Activities{Classification: ClassNotAffected})
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	noErr(t, env.GetWorkflowError())
	v, err := env.QueryWorkflow(QueryState)
	noErr(t, err)
	var st State
	noErr(t, v.Get(&st))
	eq(t, ClassNotAffected, st.Classification)
	isTrue(t, len(st.Audit) > 0, "want audit entries")
}
