package orchestrate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/run"
	"github.com/tdavison784/release-intelligence/internal/investigate/run/runtest"
	"github.com/tdavison784/release-intelligence/internal/orchestrate"
)

// Replay determinism (TEMPORAL.md "Versioning workflow code"): histories
// captured from real executions must replay against the current workflow
// code. A failure here means a workflow change is not backward compatible:
// guard it with workflow.GetVersion or route new starts to a new workflow type.
func TestReplayRecordedHistories(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "history-*.json"))
	if len(files) == 0 {
		t.Skip("no recorded histories in testdata/")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			r := worker.NewWorkflowReplayer()
			r.RegisterWorkflowWithOptions(orchestrate.InvestigationWorkflow, workflow.RegisterOptions{Name: orchestrate.WorkflowType})
			if err := r.ReplayWorkflowHistoryFromJSONFile(nil, f); err != nil {
				t.Fatalf("replay %s: %v", f, err)
			}
		})
	}
}

// TestRecordHistory regenerates testdata/history-*.json from a real
// Temporal dev server (`temporal server start-dev --port 7299`):
//
//	RI_RECORD_HISTORY=1 RI_TEMPORAL_ADDR=localhost:7299 go test ./internal/orchestrate -run RecordHistory
//
// Re-record only when the workflow code changes on purpose, and keep the old
// files while in-flight executions of the old code may exist.
func TestRecordHistory(t *testing.T) { recordHistory(t, "history-approve.json", false) }

// TestRecordHistorySynthesize records testdata/history-synthesize.json: the
// same run with Input.Synthesize, which takes the GetVersion(VersionSynthesize)
// path (Milestone 3), so the versioned synthesis step is replay-tested too.
func TestRecordHistorySynthesize(t *testing.T) { recordHistory(t, "history-synthesize.json", true) }

func recordHistory(t *testing.T, name string, synthesize bool) {
	if os.Getenv("RI_RECORD_HISTORY") == "" {
		t.Skip("set RI_RECORD_HISTORY=1 and RI_TEMPORAL_ADDR to record a history from a dev server")
	}
	addr := os.Getenv("RI_TEMPORAL_ADDR")
	c, err := client.Dial(client.Options{HostPort: addr})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	f := runtest.New(t)
	if synthesize {
		client, _ := runtest.SynthClient(t)
		f.Deps.Synthesizer = runtest.Synth{Client: client}
	}
	queue := "ri-record-" + t.Name()
	w := worker.New(c, queue, worker.Options{})
	orchestrate.Register(w, &orchestrate.Activities{P: run.New(f.Deps)})
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	ctx := context.Background()
	wr, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: "record-" + name + "-" + time.Now().Format("150405"), TaskQueue: queue},
		orchestrate.WorkflowType, orchestrate.Input{Request: f.Request(), ReviewTimeout: time.Hour, Synthesize: synthesize})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	if _, err := orchestrate.GetState(ctx, c, wr.GetID()); err != nil {
		t.Fatal(err)
	}
	if err := orchestrate.SendReview(ctx, c, wr.GetID(), *runtest.Human(investigate.ReviewApprove)); err != nil {
		t.Fatal(err)
	}
	var res orchestrate.Result
	if err := wr.Get(ctx, &res); err != nil {
		t.Fatal(err)
	}
	iter := c.GetWorkflowHistory(ctx, wr.GetID(), wr.GetRunID(), false, 0)
	hist, err := historyJSON(iter)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("testdata", name), hist, 0o644); err != nil {
		t.Fatal(err)
	}
}
