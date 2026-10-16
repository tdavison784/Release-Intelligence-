package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// The test binary doubles as a fake `claude` CLI and as the `ri investigate
// mcp --attach` bridge the MCP config starts, so the claude-cli driver is
// tested end to end (flags, MCP config, socket bridge, envelope) without the
// real CLI.
func TestMain(m *testing.M) {
	switch os.Getenv("AGENT_HELPER") {
	case "claude":
		os.Exit(fakeClaude())
	case "bridge":
		os.Exit(fakeBridge())
	}
	os.Exit(m.Run())
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func fakeBridge() int {
	args := os.Args[1:]
	if err := Attach(context.Background(), argAfter(args, "--attach"), argAfter(args, "--token"), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func fakeClaude() int {
	args := os.Args[1:]
	_, _ = io.ReadAll(os.Stdin)
	mode := os.Getenv("AGENT_TEST_MODE")
	if mode == "no-envelope" {
		fmt.Fprintln(os.Stderr, "fake claude: not logged in")
		return 1
	}
	for _, want := range []string{"-p", "--strict-mcp-config", "--no-session-persistence"} {
		found := false
		for _, a := range args {
			found = found || a == want
		}
		if !found {
			fmt.Fprintln(os.Stderr, "missing flag", want)
			return 2
		}
	}
	if argAfter(args, "--tools") != "" || argAfter(args, "--allowedTools") != "mcp__ri" || argAfter(args, "--permission-mode") != "dontAsk" {
		fmt.Fprintln(os.Stderr, "unsafe tool flags", args)
		return 2
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	raw, err := os.ReadFile(argAfter(args, "--mcp-config"))
	if err != nil || json.Unmarshal(raw, &cfg) != nil {
		fmt.Fprintln(os.Stderr, "bad mcp config", err)
		return 2
	}
	srv := cfg.MCPServers[MCPServerName]
	cmd := exec.Command(srv.Command, srv.Args...)
	cmd.Env = append(os.Environ(), "AGENT_HELPER=bridge")
	in, _ := cmd.StdinPipe()
	outPipe, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	out := bufio.NewReader(outPipe)
	id := 0
	rpc := func(method string, params any) map[string]any {
		id++
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		fmt.Fprintf(in, "%s\n", b)
		line, _ := out.ReadBytes('\n')
		var m map[string]any
		_ = json.Unmarshal(line, &m)
		return m
	}
	rpc("initialize", map[string]any{"protocolVersion": "2025-06-18"})
	fmt.Fprintln(in, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	tools := rpc("tools/list", nil)
	if n := len(tools["result"].(map[string]any)["tools"].([]any)); n != len(investigate.ToolNames) {
		fmt.Fprintln(os.Stderr, "tools/list", n)
		return 2
	}
	var assertion map[string]any
	_ = json.Unmarshal([]byte(os.Getenv("AGENT_TEST_ASSERT")), &assertion)
	rpc("tools/call", map[string]any{"name": "record_assertion", "arguments": assertion})
	subtype := "success"
	if mode == "max-turns" {
		subtype = "error_max_turns"
	} else {
		rpc("tools/call", map[string]any{"name": "submit_findings", "arguments": map[string]any{"items": []any{}}})
	}
	in.Close()
	_ = cmd.Wait()
	sid := argAfter(args, "--session-id")
	if mode == "other-session" {
		sid = "00000000-0000-4000-8000-000000000000"
	}
	env, _ := json.Marshal(map[string]any{"type": "result", "subtype": subtype, "is_error": subtype != "success", "result": "done", "session_id": sid,
		"num_turns": 3, "usage": map[string]any{"input_tokens": 10, "output_tokens": 20, "cache_creation_input_tokens": 30, "cache_read_input_tokens": 4000},
		"modelUsage": map[string]any{argAfter(args, "--model"): map[string]any{"costUSD": 0.01}}})
	fmt.Println(string(env))
	return 0
}

func TestClaudeCLIDriver(t *testing.T) {
	f := newFixture(t)
	exe, err := os.Executable()
	must(t, err)
	t.Setenv("AGENT_HELPER", "claude")
	assert, _ := json.Marshal(map[string]any{"changeItem": f.behaviour, "stance": "raises-concern", "statement": "the policy default changed",
		"citations": []map[string]any{{"artifact": f.notes.ID, "quote": "the default rotation policy is Always"}}})
	t.Setenv("AGENT_TEST_ASSERT", string(assert))
	run := func(mode string) *investigate.AgentOutcome {
		t.Setenv("AGENT_TEST_MODE", mode)
		inv := New(f.st, WithDriver(DriverClaudeCLI, &ClaudeCLI{Binary: exe, RIBinary: exe}), WithClock(fixedNow))
		out, err := inv.Investigate(context.Background(), f.input(investigate.AgentConfig{PromptVersion: promptV1, Enabled: true, Driver: DriverClaudeCLI,
			Model: "claude-sonnet-5-5", Budget: investigate.DefaultAgentBudget}))
		must(t, err)
		return out
	}
	out := run("ok")
	r := out.Run
	if r.Status != domain.AgentSubmitted || r.Model != "claude-sonnet-5-5" || r.Provider != "anthropic" || r.Turns != 3 || r.Tokens != 60 || r.ToolCalls != 2 {
		t.Fatalf("run: %+v", r)
	}
	if len(out.Assertions) != 1 {
		t.Fatalf("assertions: %d", len(out.Assertions))
	}
	p := out.Assertions[0].Provenance
	if p.CallID != r.CallID || p.Model != "claude-sonnet-5-5" || p.ModelVersion != "claude-sonnet-5-5" || p.Provider != "anthropic" {
		t.Fatalf("records carry the envelope's model and the session id: %+v (run call %s)", p, r.CallID)
	}
	if err := runRecord(t, f, out).ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}

	if out := run("max-turns"); out.Run.Status != domain.AgentBudgetExhausted || out.Run.Reason != "budget:turns" || len(out.Assertions) != 1 {
		t.Fatalf("max turns: %+v", out.Run)
	}
	if out := run("no-envelope"); out.Run.Status != domain.AgentFailed || !strings.Contains(out.Run.Reason, "not logged in") {
		t.Fatalf("no envelope: %+v", out.Run)
	}
	if out := run("other-session"); out.Run.Status != domain.AgentFailed || !strings.Contains(out.Run.Reason, "not the requested") {
		t.Fatalf("other session: %+v", out.Run)
	}

	_, err = New(f.st, WithDriver(DriverClaudeCLI, &ClaudeCLI{Binary: "/nonexistent/claude"})).Investigate(context.Background(),
		f.input(investigate.AgentConfig{PromptVersion: promptV1, Enabled: true, Driver: DriverClaudeCLI}))
	if investigate.ClassOf(err) != investigate.Permanent {
		t.Fatalf("missing CLI: %v", err)
	}
}
