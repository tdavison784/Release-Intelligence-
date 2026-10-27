package agent

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/gitsrc"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Reproducibility (briefs/upstream.md required 5): upstream fetches go through
// the gitsrc and fetch caches, so a session recorded online replays offline
// from the recorded state with identical bytes: the replay driver finds no
// mismatch, and the requests, artifacts and assessments are the same. Offline
// without the recorded state, the same requests are unavailable (and a replay
// of the recording does not match).

// gitHost answers the git commands of gitsrc's repo-dir flow for one
// repository whose tree is the same at every ref.
type gitHost struct {
	mu    sync.Mutex
	files map[string]string
	runs  int
}

func oidOf(content string) string {
	h := sha1.Sum([]byte("blob\x00" + content))
	return hex.EncodeToString(h[:])
}

func (g *gitHost) Run(_ context.Context, c gitsrc.Command) ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.runs++
	has := func(s string) int {
		for i, a := range c.Args {
			if a == s {
				return i
			}
		}
		return -1
	}
	switch {
	case has("rev-parse") >= 0:
		return []byte(strings.Repeat("12", 20) + "\n"), nil
	case has("ls-tree") >= 0:
		dir := ""
		if i := has("--"); i >= 0 && i+1 < len(c.Args) {
			dir = c.Args[i+1]
		}
		var paths []string
		for p := range g.files {
			if dir == "" || strings.HasPrefix(p, dir+"/") {
				paths = append(paths, p)
			}
		}
		sort.Strings(paths)
		var b bytes.Buffer
		for _, p := range paths {
			b.WriteString("100644 blob " + oidOf(g.files[p]) + "\t" + p + "\x00")
		}
		return b.Bytes(), nil
	case has("cat-file") >= 0:
		byOID := map[string]string{}
		for _, content := range g.files {
			byOID[oidOf(content)] = content
		}
		var b bytes.Buffer
		for _, oid := range strings.Fields(string(c.Stdin)) {
			content := byOID[oid]
			b.WriteString(oid + " blob " + strconv.Itoa(len(content)) + "\n" + content + "\n")
		}
		return b.Bytes(), nil
	}
	return nil, nil // init, config, the blob prefetch
}

// noGit fails the test when git runs at all (offline replay).
type noGit struct{ t *testing.T }

func (n noGit) Run(_ context.Context, c gitsrc.Command) ([]byte, error) {
	n.t.Errorf("git must not run offline: %v", c.Args)
	return nil, &gitsrc.RunError{Args: c.Args, ExitCode: 1, Stderr: "offline"}
}

// rawHost serves raw file content for one repository at one tag.
type rawHost struct {
	prefix string
	files  map[string]string
	hits   int
}

func (h *rawHost) RoundTrip(r *http.Request) (*http.Response, error) {
	h.hits++
	body, ok := h.files[strings.TrimPrefix(r.URL.String(), h.prefix)]
	status := http.StatusOK
	if !ok {
		status, body = http.StatusNotFound, "404: Not Found"
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"text/plain"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

// cachedPort is the production port shape: gitsrc's Files and Dirs over a
// fetch client and cache directories.
func cachedPort(git *gitsrc.Git) Upstream {
	return Upstream{Files: git.Files(), Dirs: git.Dirs(), Product: func(domain.ProductID) ([]string, func(string) string, bool) {
		return []string{upstreamRepo}, nil, true
	}}
}

func TestUpstreamReplaysOfflineFromTheRecordedState(t *testing.T) {
	src := upstreamSource["v2.1.0"]
	httpCache, gitCache := t.TempDir(), t.TempDir()

	// Record online (the network is faked; the caches are real).
	raw := &rawHost{prefix: "https://raw.githubusercontent.com/example/keystore/v2.1.0/", files: src}
	online := fetch.NewHTTPClient(fetch.NewCache(httpCache), fetch.ModeOnline)
	online.HTTP = &http.Client{Transport: raw}
	host := &gitHost{files: src}
	port := cachedPort(gitsrc.New(online, gitsrc.Options{CacheDir: gitCache, Runner: host}))

	f := newConsumerFixture(t)
	a := f.assessment()
	steps := []Step{
		fileStep(f, "internal/validate/name.go"),
		fileStep(f, "internal/validate/absent.go"),
		f.upstreamStep("call-9", map[string]any{"kind": "upstream-search", "repo": upstreamRepo, "dir": "internal/validate", "pattern": "MustCompile|CheckName", "glob": "*_test.go"}),
		f.upstreamStep("call-9", map[string]any{"kind": "upstream-search", "repo": upstreamRepo, "dir": "nowhere", "pattern": "x"}),
		f.v9Submit("potentially-affected", a),
	}
	_, out := v11Run(t, f, &port, steps...)
	if out.Run.Status != domain.AgentSubmitted || raw.hits != 2 || host.runs == 0 {
		t.Fatalf("recording: %s %s (raw hits %d, git runs %d)", out.Run.Status, out.Run.Reason, raw.hits, host.runs)
	}
	statuses := func(o *investigate.AgentOutcome) string {
		var s []string
		for _, r := range o.EvidenceRequests {
			s = append(s, string(r.Status))
		}
		return strings.Join(s, ",")
	}
	if got := statuses(out); got != "fulfilled,unavailable,fulfilled,unavailable" {
		t.Fatalf("recorded statuses %s: %+v", got, out.EvidenceRequests)
	}
	transcript := transcriptOf(t, f, out)

	// Replay offline from the recorded state: no network, no git.
	offline := fetch.NewHTTPClient(fetch.NewCache(httpCache), fetch.ModeOffline)
	offline.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("network used offline: %s", r.URL)
		return nil, errors.New("offline")
	})}
	offPort := cachedPort(gitsrc.New(offline, gitsrc.Options{CacheDir: gitCache, Offline: true, Runner: noGit{t}}))
	replay := func(p Upstream) (*Replay, *investigate.AgentOutcome) {
		rep := &Replay{Transcript: transcript}
		cfg := v11Config()
		cfg.Driver = DriverReplay
		again, err := New(f.st, WithDriver(DriverReplay, rep), WithClock(fixedNow), WithUpstream(p)).Investigate(context.Background(), f.input(cfg))
		must(t, err)
		return rep, again
	}
	rep, again := replay(offPort)
	if len(rep.Mismatches) != 0 || again.Run.Status != domain.AgentSubmitted {
		t.Fatalf("offline replay: %+v %s %s", rep.Mismatches, again.Run.Status, again.Run.Reason)
	}
	j := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	ids := func(o *investigate.AgentOutcome) string { // the transcript's tags name the driver; ids do not
		var s []string
		for _, a := range o.Artifacts {
			s = append(s, a.ID)
		}
		return strings.Join(s, ",")
	}
	if again.Run.Transcript != out.Run.Transcript || again.Run.PromptDigest != out.Run.PromptDigest ||
		j(again.EvidenceRequests) != j(out.EvidenceRequests) || j(again.Assessments) != j(out.Assessments) || ids(again) != ids(out) {
		t.Fatal("the offline replay differs from the recording")
	}
	if !strings.Contains(strings.Join(again.Assessments[0].Unknown, "\n"), "behaviour at v2.1.0 unavailable: not found at v2.1.0") {
		t.Fatalf("replayed gaps: %v", again.Assessments[0].Unknown)
	}

	// Offline without the recorded state: every fetch is unavailable, and the
	// recording no longer replays.
	emptyFetch := fetch.NewHTTPClient(fetch.NewCache(t.TempDir()), fetch.ModeOffline)
	emptyPort := cachedPort(gitsrc.New(emptyFetch, gitsrc.Options{CacheDir: t.TempDir(), Offline: true, Runner: noGit{t}}))
	f2 := newConsumerFixture(t)
	_, cold := v11Run(t, f2, &emptyPort, steps[0], steps[2], f2.v9Submit("potentially-affected", f2.assessment()))
	for _, r := range cold.EvidenceRequests {
		if r.Status != domain.RequestUnavailable || !strings.HasPrefix(r.Resolution, "offline: not in the recorded cache") {
			t.Errorf("cold offline request %+v", r)
		}
	}
	if len(cold.Assessments[0].UnavailableEvidence) != 2 {
		t.Fatalf("cold offline gaps: %v", cold.Assessments[0].Unknown)
	}
	if rep, _ := replay(emptyPort); len(rep.Mismatches) == 0 {
		t.Fatal("a recording replayed against a state without its upstream fetches must not match")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
