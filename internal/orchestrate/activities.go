package orchestrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

// IdempotencyKey derives a stable key from a stage name and its input digests. Re-running a
// stage with the same inputs yields the same key, so activities can dedupe side effects
// (artifact writes, external calls) when Temporal retries or a workflow is re-submitted.
func IdempotencyKey(stage string, inputs []ArtifactRef) string {
	h := sha256.New()
	h.Write([]byte(stage))
	for _, in := range inputs {
		h.Write([]byte{0})
		h.Write([]byte(in.Digest))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func digestOf(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// Activities holds the stub stage implementations. The fields are test/behaviour knobs; in
// wave 2 this struct receives the real collaborators (plain Go packages) as dependencies.
type Activities struct {
	// TransientFailures makes the named stage fail with a retryable error this many times
	// before succeeding.
	TransientFailures map[string]int
	// PermanentFailure makes the named stage fail with a non-retryable error.
	PermanentFailure map[string]bool
	// Classification returned by the classify stage ("" => ClassPotentiallyAffected).
	Classification string

	mu       sync.Mutex
	attempts map[string]int
	seen     map[string]ArtifactRef // idempotency key -> output (dedupe)
}

// Run executes one stub stage.
func (a *Activities) Run(ctx context.Context, req StageRequest) (StageResult, error) {
	a.mu.Lock()
	if a.attempts == nil {
		a.attempts = map[string]int{}
		a.seen = map[string]ArtifactRef{}
	}
	a.attempts[req.Stage]++
	n := a.attempts[req.Stage]
	prior, dup := a.seen[req.IdempotencyKey]
	a.mu.Unlock()

	if a.PermanentFailure[req.Stage] {
		return StageResult{}, temporal.NewNonRetryableApplicationError(
			"stage "+req.Stage+": unsupported input", ErrTypeInvalidInput, nil)
	}
	if n <= a.TransientFailures[req.Stage] {
		return StageResult{}, temporal.NewApplicationError(
			"stage "+req.Stage+": transient failure", ErrTypeTransient)
	}
	activity.RecordHeartbeat(ctx, req.Stage)

	out := prior
	if !dup {
		out = ArtifactRef{Digest: digestOf(req.IdempotencyKey, req.Stage), URI: "artifact://" + req.Stage}
		a.mu.Lock()
		a.seen[req.IdempotencyKey] = out
		a.mu.Unlock()
	}
	res := StageResult{Output: out}
	if req.Stage == StageClassify {
		res.Classification = a.Classification
		if res.Classification == "" {
			res.Classification = ClassPotentiallyAffected
		}
		// Fail closed: only a definite Not Affected skips human review.
		res.NeedsReview = res.Classification != ClassNotAffected
	}
	return res, nil
}

// Attempts reports how many times a stage ran (for tests).
func (a *Activities) Attempts(stage string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.attempts[stage]
}

// registerRun registers Activities.Run under the stable activity name "Run".
var registerRun = activity.RegisterOptions{Name: "Run"}
