package gitsrc

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/fetch"
)

// fakeRunner is a Runner driven by a function; it records every command.
type fakeRunner struct {
	mu    sync.Mutex
	calls []Command
	fn    func(c Command) ([]byte, error)
}

func (f *fakeRunner) Run(_ context.Context, c Command) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	if f.fn == nil {
		return nil, nil
	}
	return f.fn(c)
}

// subcommands returns the sub-command of every recorded call.
func (f *fakeRunner) subcommands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, subcommand(c.Args))
	}
	return out
}

func (f *fakeRunner) count(sub string) int {
	n := 0
	for _, s := range f.subcommands() {
		if s == sub {
			n++
		}
	}
	return n
}

// failRunner fails the test when git is run at all.
func failRunner(t *testing.T) Runner {
	t.Helper()
	return &fakeRunner{fn: func(c Command) ([]byte, error) {
		t.Errorf("git must not run, got %v", c.Args)
		return nil, &RunError{Args: c.Args, ExitCode: 1, Stderr: "must not run"}
	}}
}

// clock is a controllable time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// newTestGit returns an adapter set that caches in dir and uses the given
// runner, fetch client and clock.
func newTestGit(t *testing.T, dir string, r Runner, fc fetch.Client, clk *clock, opts Options) *Git {
	t.Helper()
	opts.CacheDir = dir
	opts.Runner = r
	g := New(fc, opts)
	if clk != nil {
		g.now = clk.Now
	}
	return g
}

// recordingFetch is a fetch.Client serving canned documents.
type recordingFetch struct {
	mu   sync.Mutex
	reqs []fetch.Request
	docs map[string]string // url -> body
	err  error
}

func (r *recordingFetch) Do(_ context.Context, req fetch.Request) (*fetch.Document, error) {
	r.mu.Lock()
	r.reqs = append(r.reqs, req)
	r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	body, ok := r.docs[req.URL]
	if !ok {
		return nil, &fetch.Error{URL: req.URL, Status: http.StatusNotFound, Err: fetch.ErrNotFound}
	}
	return &fetch.Document{
		URL: req.URL, Status: 200, Body: []byte(body),
		Digest:      "sha256:test-" + req.URL,
		RetrievedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
	}, nil
}

// requireGit skips the test when git is not installed.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

// testRepo is a throw-away git repository served over file://.
type testRepo struct {
	t    *testing.T
	Dir  string
	URL  string
	step int
}

// newTestRepo creates an empty repository that allows partial clones and
// fetching arbitrary objects, like the hosting platforms do.
func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	requireGit(t)
	dir := filepath.Join(t.TempDir(), "src-repo")
	r := &testRepo{t: t, Dir: dir, URL: "file://" + filepath.ToSlash(dir)}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r.git("init", "-q", "-b", "main")
	r.git("config", "uploadpack.allowFilter", "true")
	r.git("config", "uploadpack.allowAnySHA1InWant", "true")
	return r
}

// git runs git in the repository with a hermetic identity and no signing.
func (r *testRepo) git(args ...string) string {
	r.t.Helper()
	full := append([]string{
		"-c", "user.name=Test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false",
		"-c", "protocol.file.allow=always",
	}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = r.Dir
	r.step++
	// Distinct, increasing commit times make the history order unambiguous.
	ts := time.Date(2026, 1, 1, 0, 0, r.step, 0, time.UTC).Format(time.RFC3339)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+ts, "GIT_COMMITTER_DATE="+ts)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit writes the files (path -> content) and commits them.
func (r *testRepo) commit(msg string, files map[string]string) string {
	r.t.Helper()
	for p, content := range files {
		full := filepath.Join(r.Dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
	return r.git("rev-parse", "HEAD")
}
