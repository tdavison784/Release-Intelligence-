// Command devcheck is a throwaway check of the workflow against a real
// `temporal server start-dev` (default localhost:7233). Not part of tests.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/tdavison784/release-intelligence/internal/orchestrate"
)

func main() {
	c, err := client.Dial(client.Options{HostPort: "localhost:17233"})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	w := worker.New(c, orchestrate.TaskQueue, worker.Options{})
	orchestrate.Register(w, &orchestrate.Activities{})
	if err := w.Start(); err != nil {
		log.Fatal(err)
	}
	defer w.Stop()

	in := orchestrate.InvestigationInput{
		Dependency: "example/dep", FromVersion: "1.0.0", ToVersion: "2.0.0",
		Environment: "prod", EnvInputRef: orchestrate.ArtifactRef{Digest: "sha256:env"},
		ReviewTimeout: time.Minute,
	}
	ctx := context.Background()
	id := orchestrate.WorkflowID(in)
	run, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: id, TaskQueue: orchestrate.TaskQueue}, orchestrate.InvestigationWorkflow, in)
	if err != nil {
		log.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	var st orchestrate.State
	q, err := c.QueryWorkflow(ctx, id, "", orchestrate.QueryState)
	if err != nil {
		log.Fatal(err)
	}
	_ = q.Get(&st)
	fmt.Println("stage:", st.CurrentStage, "awaitingReview:", st.AwaitingReview)
	if err := c.SignalWorkflow(ctx, id, "", orchestrate.SignalReview, orchestrate.ReviewDecision{Decision: orchestrate.DecisionApprove, Reviewer: "devcheck"}); err != nil {
		log.Fatal(err)
	}
	var res orchestrate.InvestigationResult
	if err := run.Get(ctx, &res); err != nil {
		log.Fatal(err)
	}
	fmt.Println("outcome:", res.Outcome, "audit entries:", len(res.Audit), "id:", id)
}
