package orchestrate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
)

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
	require.NoError(t, env.GetWorkflowError())
	var res InvestigationResult
	require.NoError(t, env.GetWorkflowResult(&res))
	require.Equal(t, OutcomeAutoCompleted, res.Outcome)
	require.Equal(t, ClassNotAffected, res.Classification)
	require.Len(t, res.Artifacts, 6)
	for _, s := range []string{StageCollectChange, StageEnvSnapshot, StageGraph, StageInvestigate, StageValidate, StageClassify} {
		require.Equal(t, []string{"started", "completed"}, statuses(res.Audit, s), s)
	}
	require.Empty(t, statuses(res.Audit, StageReview))
	for i, e := range res.Audit {
		require.Equal(t, i+1, e.Seq)
	}
}

func TestIdempotencyKeysAreContentDerived(t *testing.T) {
	a := []ArtifactRef{{Digest: "x"}, {Digest: "y"}}
	require.Equal(t, IdempotencyKey("s", a), IdempotencyKey("s", a))
	require.NotEqual(t, IdempotencyKey("s", a), IdempotencyKey("t", a))
	require.NotEqual(t, IdempotencyKey("s", a), IdempotencyKey("s", a[:1]))
	require.Equal(t, WorkflowID(testInput()), WorkflowID(testInput()))
	other := testInput()
	other.ToVersion = "2.0.1"
	require.NotEqual(t, WorkflowID(testInput()), WorkflowID(other))
}

func TestRetryThenSucceed(t *testing.T) {
	a := &Activities{Classification: ClassNotAffected, TransientFailures: map[string]int{StageEnvSnapshot: 2}}
	env := newEnv(t, a)
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, 3, a.Attempts(StageEnvSnapshot))
}

func TestTransientExhaustsRetries(t *testing.T) {
	a := &Activities{TransientFailures: map[string]int{StageGraph: 100}}
	env := newEnv(t, a)
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	require.Error(t, env.GetWorkflowError())
	require.Equal(t, 4, a.Attempts(StageGraph))
}

func TestNonRetryableFailure(t *testing.T) {
	a := &Activities{PermanentFailure: map[string]bool{StageCollectChange: true}}
	env := newEnv(t, a)
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	err := env.GetWorkflowError()
	require.Error(t, err)
	require.Equal(t, 1, a.Attempts(StageCollectChange))
	var appErr *temporal.ApplicationError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, ErrTypeInvalidInput, appErr.Type())
}

func TestReviewApprove(t *testing.T) {
	a := &Activities{}
	env := newEnv(t, a)
	signalAfter(env, time.Minute, ReviewDecision{Decision: DecisionApprove, Reviewer: "alice"})
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	require.NoError(t, env.GetWorkflowError())
	var res InvestigationResult
	require.NoError(t, env.GetWorkflowResult(&res))
	require.Equal(t, OutcomeApproved, res.Outcome)
	require.Equal(t, "alice", res.Reviewer)
	require.Equal(t, []string{"waiting", "signal"}, statuses(res.Audit, StageReview))
}

func TestReviewReject(t *testing.T) {
	env := newEnv(t, &Activities{})
	signalAfter(env, time.Minute, ReviewDecision{Decision: DecisionReject, Reviewer: "bob"})
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	var res InvestigationResult
	require.NoError(t, env.GetWorkflowResult(&res))
	require.Equal(t, OutcomeRejected, res.Outcome)
}

func TestRequestMoreEvidenceThenApprove(t *testing.T) {
	a := &Activities{}
	env := newEnv(t, a)
	signalAfter(env, time.Minute, ReviewDecision{Decision: DecisionMoreEvidence, Reviewer: "a", Note: "check overlays"})
	signalAfter(env, 2*time.Minute, ReviewDecision{Decision: DecisionApprove, Reviewer: "a"})
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	require.NoError(t, env.GetWorkflowError())
	var res InvestigationResult
	require.NoError(t, env.GetWorkflowResult(&res))
	require.Equal(t, OutcomeApproved, res.Outcome)
	require.Equal(t, 1, res.Rounds)
	require.Equal(t, 2, a.Attempts(StageInvestigate))
	require.Equal(t, 1, a.Attempts(StageCollectChange))
}

func TestMoreEvidenceIsBounded(t *testing.T) {
	env := newEnv(t, &Activities{})
	for i := 1; i <= MaxEvidenceRounds+1; i++ {
		signalAfter(env, time.Duration(i)*time.Minute, ReviewDecision{Decision: DecisionMoreEvidence, Reviewer: "a"})
	}
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	var res InvestigationResult
	require.NoError(t, env.GetWorkflowResult(&res))
	require.Equal(t, OutcomeRejected, res.Outcome)
	require.Equal(t, MaxEvidenceRounds, res.Rounds)
}

func TestReviewTimeoutFailsClosed(t *testing.T) {
	env := newEnv(t, &Activities{})
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	require.NoError(t, env.GetWorkflowError())
	var res InvestigationResult
	require.NoError(t, env.GetWorkflowResult(&res))
	require.Equal(t, OutcomeReviewTimedOut, res.Outcome)
	require.Equal(t, []string{"waiting", "timed-out"}, statuses(res.Audit, StageReview))
}

func TestCancelWhileAwaitingReview(t *testing.T) {
	env := newEnv(t, &Activities{})
	env.RegisterDelayedCallback(func() {
		v, err := env.QueryWorkflow(QueryState)
		require.NoError(t, err)
		var st State
		require.NoError(t, v.Get(&st))
		require.True(t, st.AwaitingReview)
		require.Equal(t, StageReview, st.CurrentStage)
		env.CancelWorkflow()
	}, time.Minute)
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	err := env.GetWorkflowError()
	require.Error(t, err)
	require.True(t, temporal.IsCanceledError(err), "got %v", err)
}

func TestQueryStateAfterCompletion(t *testing.T) {
	env := newEnv(t, &Activities{Classification: ClassNotAffected})
	env.ExecuteWorkflow(InvestigationWorkflow, testInput())
	require.NoError(t, env.GetWorkflowError())
	v, err := env.QueryWorkflow(QueryState)
	require.NoError(t, err)
	var st State
	require.NoError(t, v.Get(&st))
	require.Equal(t, ClassNotAffected, st.Classification)
	require.NotEmpty(t, st.Audit)
}
