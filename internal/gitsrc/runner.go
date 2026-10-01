package gitsrc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Command describes one git invocation.
type Command struct {
	// Dir is the working directory of the process; empty means the current one.
	Dir string
	// Args are the git arguments, without the leading "git".
	Args []string
	// Stdin, when non-nil, is fed to the process on standard input.
	Stdin []byte
}

// Runner executes git. It exists so that tests (and callers with special
// needs, such as credential injection) can substitute the way git is run.
// Run returns the standard output of a successful command. A failed command
// should return an error that carries a *RunError so that the adapters can
// tell an unreachable remote from a missing ref.
type Runner interface {
	Run(ctx context.Context, cmd Command) ([]byte, error)
}

// RunError describes a failed git invocation.
type RunError struct {
	// Args are the git arguments of the failed command.
	Args []string
	// ExitCode is the process exit status, or -1 when the process did not
	// run to completion (not found, timed out, killed).
	ExitCode int
	// Stderr is the (truncated) standard error of the process.
	Stderr string
	// TimedOut reports that the runner's own timeout fired.
	TimedOut bool
	// NotInstalled reports that the git executable could not be started.
	NotInstalled bool
	// Err is the underlying error, if any.
	Err error
}

func (e *RunError) Error() string {
	return "git " + subcommand(e.Args) + ": " + e.detail()
}

// detail describes what went wrong without naming the command: the exit
// status or condition, then the first lines of standard error.
func (e *RunError) detail() string {
	var parts []string
	switch {
	case e.TimedOut:
		parts = append(parts, "timed out")
	case e.NotInstalled:
		parts = append(parts, "executable not found")
	case e.ExitCode > 0:
		parts = append(parts, fmt.Sprintf("exit status %d", e.ExitCode))
	}
	if s := strings.TrimSpace(e.Stderr); s != "" {
		parts = append(parts, firstLines(s, 3))
	} else if e.Err != nil && !e.TimedOut && e.ExitCode <= 0 {
		parts = append(parts, e.Err.Error())
	}
	if len(parts) == 0 {
		return "failed"
	}
	return strings.Join(parts, ": ")
}

// Unwrap returns the underlying error.
func (e *RunError) Unwrap() error { return e.Err }

// subcommand returns the git sub-command of an argument list, skipping global
// options such as "--git-dir <dir>" and "-c key=value". Only the sub-command
// is reported in errors; arguments may carry URLs and paths that callers
// report themselves.
func subcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-c" || a == "--git-dir" || a == "-C":
			i++
		case strings.HasPrefix(a, "-"):
		default:
			return a
		}
	}
	return ""
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "; ")
}

// DefaultTimeout bounds a single git command run by ExecRunner.
const DefaultTimeout = 5 * time.Minute

// maxStderr bounds how much standard error ExecRunner keeps.
const maxStderr = 16 << 10

// ExecRunner is the default Runner: it shells out to the git executable.
//
// Every command runs non-interactively (GIT_TERMINAL_PROMPT=0, so a remote
// that wants credentials fails instead of hanging), with a C locale (so that
// stderr is classifiable) and under a timeout. Cancelling the context kills
// the process. The inherited environment is otherwise preserved, which keeps
// proxy and credential-helper configuration working.
type ExecRunner struct {
	// Path is the git executable; empty means "git" from PATH.
	Path string
	// Timeout bounds one command; zero means DefaultTimeout.
	Timeout time.Duration
	// Env holds additional KEY=VALUE entries appended to the environment.
	Env []string
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, c Command) ([]byte, error) {
	path := r.Path
	if path == "" {
		path = "git"
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(tctx, path, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_LFS_SKIP_SMUDGE=1",
		// Abort transfers that stall instead of waiting for the timeout.
		"GIT_HTTP_LOW_SPEED_LIMIT=1000",
		"GIT_HTTP_LOW_SPEED_TIME=60",
		"LC_ALL=C",
		"LANGUAGE=C",
	)
	cmd.Env = append(cmd.Env, r.Env...)
	if c.Stdin != nil {
		cmd.Stdin = bytes.NewReader(c.Stdin)
	}
	var stdout bytes.Buffer
	stderr := &cappedBuffer{max: maxStderr}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = 10 * time.Second

	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}
	if ctx.Err() != nil {
		// The caller gave up; do not dress this up as a git failure.
		return nil, fmt.Errorf("git %s: %w", subcommand(c.Args), ctx.Err())
	}
	re := &RunError{Args: c.Args, ExitCode: -1, Stderr: stderr.String(), Err: err}
	var exitErr *exec.ExitError
	switch {
	case errors.Is(tctx.Err(), context.DeadlineExceeded):
		re.TimedOut = true
		re.Err = fmt.Errorf("timed out after %s", timeout)
	case errors.As(err, &exitErr):
		re.ExitCode = exitErr.ExitCode()
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission):
		re.NotInstalled = true
	}
	return nil, re
}

// cappedBuffer keeps at most max bytes and silently drops the rest.
type cappedBuffer struct {
	buf bytes.Buffer
	max int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		if len(p) > room {
			c.buf.Write(p[:room])
		} else {
			c.buf.Write(p)
		}
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }
