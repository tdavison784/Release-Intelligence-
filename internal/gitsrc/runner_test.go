package gitsrc

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
)

func TestExecRunnerRunsGit(t *testing.T) {
	requireGit(t)
	out, err := ExecRunner{}.Run(context.Background(), Command{Args: []string{"--version"}})
	if err != nil || !strings.HasPrefix(string(out), "git version") {
		t.Fatalf("out=%q err=%v", out, err)
	}
	// Stdin reaches the process.
	out, err = ExecRunner{}.Run(context.Background(), Command{Args: []string{"hash-object", "--stdin"}, Stdin: []byte("hello\n")})
	if err != nil || strings.TrimSpace(string(out)) != "ce013625030ba8dba906f756967f9e9ca394464a" {
		t.Fatalf("hash-object: %q %v", out, err)
	}
}

func TestExecRunnerFailureCarriesStderrAndExitCode(t *testing.T) {
	requireGit(t)
	_, err := ExecRunner{}.Run(context.Background(), Command{Args: []string{"rev-parse", "--verify", "refs/heads/nope"}, Dir: t.TempDir()})
	var re *RunError
	if !errors.As(err, &re) {
		t.Fatalf("want *RunError, got %T %v", err, err)
	}
	if re.ExitCode <= 0 || re.TimedOut || re.NotInstalled {
		t.Errorf("run error = %+v", re)
	}
	if !strings.Contains(err.Error(), "git rev-parse") {
		t.Errorf("message %q does not name the sub-command", err)
	}
}

func TestExecRunnerIsNonInteractive(t *testing.T) {
	requireGit(t)
	// A server that wants credentials must fail fast, not wait for a tty.
	// The repository is a local path, so no network is involved; we only
	// check that the environment variables are set for the child.
	out, err := ExecRunner{Path: "sh"}.Run(context.Background(), Command{Args: []string{"-c", "echo $GIT_TERMINAL_PROMPT $LC_ALL"}})
	if err != nil || strings.TrimSpace(string(out)) != "0 C" {
		t.Fatalf("env = %q %v", out, err)
	}
	// Caller supplied environment wins.
	out, err = ExecRunner{Path: "sh", Env: []string{"LC_ALL=de_DE.UTF-8"}}.Run(context.Background(), Command{Args: []string{"-c", "echo $LC_ALL"}})
	if err != nil || strings.TrimSpace(string(out)) != "de_DE.UTF-8" {
		t.Fatalf("env override = %q %v", out, err)
	}
}

func TestExecRunnerNotInstalled(t *testing.T) {
	_, err := ExecRunner{Path: "definitely-not-a-git-binary"}.Run(context.Background(), Command{Args: []string{"--version"}})
	var re *RunError
	if !errors.As(err, &re) || !re.NotInstalled {
		t.Fatalf("got %v", err)
	}
	wrapped := wrapErr("https://github.com/o/r", "git ls-remote", err)
	if !errors.Is(wrapped, fetch.ErrUnavailable) || fetch.StateFor(wrapped) != domain.SourceUnavailable {
		t.Errorf("missing git must be unavailable: %v", wrapped)
	}
}

func TestExecRunnerTimeout(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep is not available")
	}
	start := time.Now()
	_, err := ExecRunner{Path: "sleep", Timeout: 100 * time.Millisecond}.Run(context.Background(), Command{Args: []string{"30"}})
	var re *RunError
	if !errors.As(err, &re) || !re.TimedOut {
		t.Fatalf("got %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("timeout took %s", time.Since(start))
	}
	if !errors.Is(wrapErr("u", "op", err), fetch.ErrUnavailable) {
		t.Errorf("a timeout is an unavailable source")
	}
}

func TestExecRunnerContextCancellation(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep is not available")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := ExecRunner{Path: "sleep"}.Run(ctx, Command{Args: []string{"30"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	var re *RunError
	if errors.As(err, &re) {
		t.Errorf("a cancelled context is not a git failure: %v", err)
	}
	if got := fetch.StateFor(wrapErr("u", "op", err)); got != domain.SourceError {
		t.Errorf("state = %q", got)
	}
}

func TestClassifyStderr(t *testing.T) {
	tests := []struct {
		stderr string
		want   error
	}{
		// not found
		{"fatal: couldn't find remote ref refs/tags/v9", fetch.ErrNotFound},
		{"fatal: Remote branch release-9 not found in upstream origin", fetch.ErrNotFound},
		{"remote: Repository not found.\nfatal: repository 'https://github.com/o/x/' not found", fetch.ErrNotFound},
		{"fatal: unable to access 'https://h/o/r/': The requested URL returned error: 404", fetch.ErrNotFound},
		{"fatal: ambiguous argument 'v9': unknown revision or path not in the working tree.", fetch.ErrNotFound},
		{"fatal: bad revision 'v9..v10'", fetch.ErrNotFound},
		// unavailable
		{"fatal: unable to access 'https://github.com/o/r/': Could not resolve host: github.com", fetch.ErrUnavailable},
		{"fatal: unable to access 'https://x/': Failed to connect to x port 443 after 1 ms: Connection refused", fetch.ErrUnavailable},
		{"fatal: unable to access 'https://x/': CONNECT tunnel failed, response 403", fetch.ErrUnavailable},
		{"fatal: unable to access 'https://x/': The requested URL returned error: 403", fetch.ErrUnavailable},
		{"fatal: unable to access 'https://x/': The requested URL returned error: 503", fetch.ErrUnavailable},
		{"fatal: unable to access 'https://x/': SSL certificate problem: self-signed certificate", fetch.ErrUnavailable},
		{"fatal: could not read Username for 'https://github.com': terminal prompts disabled", fetch.ErrUnavailable},
		{"fatal: Authentication failed for 'https://x/'", fetch.ErrUnavailable},
		{"error: RPC failed; curl 56 OpenSSL SSL_read: Connection reset by peer\nfatal: early EOF", fetch.ErrUnavailable},
		{"git@github.com: Permission denied (publickey).", fetch.ErrUnavailable},
		// unclassified
		{"fatal: something else entirely", nil},
		{"", nil},
	}
	for _, tc := range tests {
		got := classify(&RunError{Args: []string{"fetch"}, ExitCode: 128, Stderr: tc.stderr})
		if got != tc.want {
			t.Errorf("classify(%q) = %v, want %v", tc.stderr, got, tc.want)
		}
	}
}

func TestWrapErrKeepsSentinelsAndAnnotates(t *testing.T) {
	// Already a fetch error: returned unchanged.
	fe := &fetch.Error{URL: "u", Err: fetch.ErrNotFound}
	if got := wrapErr("x", "op", fe); got != error(fe) {
		t.Errorf("fetch.Error must pass through, got %v", got)
	}
	// A plain sentinel gets annotated, not hidden.
	got := wrapErr("https://h/o/r", "git fetch", fetch.ErrOffline)
	if !errors.Is(got, fetch.ErrOffline) || !strings.Contains(got.Error(), "https://h/o/r") {
		t.Errorf("got %v", got)
	}
	// Unclassified errors stay errors but keep their cause.
	cause := &RunError{Args: []string{"log"}, ExitCode: 1, Stderr: "fatal: odd"}
	got = wrapErr("https://h/o/r", "git log", cause)
	var re *RunError
	if !errors.As(got, &re) || errors.Is(got, fetch.ErrNotFound) || errors.Is(got, fetch.ErrUnavailable) {
		t.Errorf("got %v", got)
	}
	if wrapErr("u", "op", nil) != nil {
		t.Error("nil must stay nil")
	}
}

func TestRunErrorMessage(t *testing.T) {
	e := &RunError{Args: []string{"--git-dir", "/x/y.git", "-c", "a=b", "fetch", "origin"}, ExitCode: 128,
		Stderr: "fatal: first\nfatal: second\nfatal: third\nfatal: fourth"}
	msg := e.Error()
	if !strings.HasPrefix(msg, "git fetch: exit status 128: fatal: first; fatal: second; fatal: third") || strings.Contains(msg, "fourth") {
		t.Errorf("message = %q", msg)
	}
	if strings.Contains(msg, "/x/y.git") {
		t.Errorf("message leaks the arguments: %q", msg)
	}
}
