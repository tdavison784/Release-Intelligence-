package render

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

// Runner executes renderer binaries. The default (ExecRunner) runs real
// processes; tests substitute a fake to exercise failure classification
// without the tools.
type Runner interface {
	LookPath(name string) (string, error)
	Run(ctx context.Context, dir, name string, args ...string) (stdout, stderr []byte, err error)
}

// ExecRunner runs processes with a timeout and a scrubbed environment:
// renders must not depend on the caller's kube context or Helm repositories.
type ExecRunner struct {
	Timeout time.Duration // default 2 minutes
	// Home isolates HELM_* cache/config/data directories; "" = a temp dir per run.
	Home string
}

// LookPath implements Runner.
func (ExecRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	home := r.Home
	if home == "" {
		h, err := os.MkdirTemp("", "ri-render-home-")
		if err != nil {
			return nil, nil, err
		}
		defer os.RemoveAll(h)
		home = h
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	// No cluster access and no user configuration: an empty KUBECONFIG and
	// private Helm directories. PATH is kept so plugins/binaries resolve.
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"KUBECONFIG=" + home + "/no-kubeconfig",
		"HELM_CACHE_HOME=" + home + "/cache",
		"HELM_CONFIG_HOME=" + home + "/config",
		"HELM_DATA_HOME=" + home + "/data",
		"LANG=C",
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out.Bytes(), errb.Bytes(), errors.New("render timed out after " + timeout.String())
	}
	return out.Bytes(), errb.Bytes(), err
}
