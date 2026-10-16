package orchestrate

import (
	"context"
	"fmt"
	"log/slog"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/worker"

	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Dial connects to a Temporal frontend. The SDK's chatty default logger is
// replaced by a leveled slog adapter (warnings and errors only).
func Dial(hostPort, namespace string, l *slog.Logger) (client.Client, error) {
	if l == nil {
		l = slog.New(slog.NewTextHandler(discard{}, nil))
	}
	return client.Dial(client.Options{HostPort: hostPort, Namespace: namespace, Logger: log.NewStructuredLogger(l)})
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// Start submits an investigation, or attaches to the execution that already
// has the request's workflow id (a running one, or a completed one: a result
// is never silently recomputed; pass rerun > 0 for a deliberate re-run).
func Start(ctx context.Context, c client.Client, in Input, rerun int) (client.WorkflowRun, error) {
	return c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:                       WorkflowID(in.Request, rerun),
		TaskQueue:                TaskQueue,
		WorkflowIDReusePolicy:    enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY,
		WorkflowIDConflictPolicy: enumspb.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
	}, WorkflowType, in)
}

// Attach returns the latest run of a workflow id.
func Attach(c client.Client, workflowID string) client.WorkflowRun {
	return c.GetWorkflow(context.Background(), workflowID, "")
}

// SendReview delivers a review decision to a waiting investigation.
func SendReview(ctx context.Context, c client.Client, workflowID string, d investigate.ReviewDecision) error {
	return c.SignalWorkflow(ctx, workflowID, "", SignalReview, d)
}

// GetState returns the workflow's state (running or completed).
func GetState(ctx context.Context, c client.Client, workflowID string) (State, error) {
	var st State
	v, err := c.QueryWorkflow(ctx, workflowID, "", QueryState)
	if err != nil {
		return st, fmt.Errorf("query %s: %w", workflowID, err)
	}
	return st, v.Get(&st)
}

// NewWorker returns a worker on the investigation task queue with the
// workflow and every activity registered.
func NewWorker(c client.Client, a *Activities) worker.Worker {
	w := worker.New(c, TaskQueue, worker.Options{})
	Register(w, a)
	return w
}
