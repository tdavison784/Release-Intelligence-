package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// ClaudeCLI drives the session with the Claude Code CLI in print mode — no
// API keys: the CLI is logged in (DESIGN.md §4.2):
//
//	claude -p --model <m> --session-id <uuid> --mcp-config <dir>/mcp.json --strict-mcp-config \
//	       --tools "" --allowedTools mcp__ri --permission-mode dontAsk --max-turns <n> \
//	       --system-prompt <system> --setting-sources "" --no-session-persistence --output-format json < task
//
// The MCP config starts `ri investigate mcp --attach <socket> --token <t>`,
// a byte bridge to this process, which serves the in-process session — so
// every tool call is validated, budgeted and recorded here. The session id
// is chosen up front (a fresh UUID) so records made during the session carry
// the call id the envelope then reports; model and model version are read
// from the envelope (`modelUsage`), never assumed.
type ClaudeCLI struct {
	Binary   string   // default "claude" on PATH
	RIBinary string   // the ri executable the MCP config starts (default: this executable)
	Extra    []string // extra CLI arguments
	// Stderr receives the CLI's stderr as it runs (default: kept for error detail only).
	Stderr io.Writer
	// KeepDir keeps the session directory (mcp.json, socket) for inspection.
	KeepDir bool
}

// cliEnvelope is the part of the `--output-format json` result the driver reads.
type cliEnvelope struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	IsError   bool   `json:"is_error"`
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
	NumTurns  int    `json:"num_turns"`
	Usage     struct {
		InputTokens         int `json:"input_tokens"`
		OutputTokens        int `json:"output_tokens"`
		CacheCreationTokens int `json:"cache_creation_input_tokens"`
		CacheReadTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	ModelUsage map[string]json.RawMessage `json:"modelUsage"`
}

func (d *ClaudeCLI) Run(ctx context.Context, as investigate.AgentSession, tools investigate.Tools) (*investigate.DriverResult, error) {
	s, err := asSession(tools)
	if err != nil {
		return nil, err
	}
	bin := d.Binary
	if bin == "" {
		bin = "claude"
	}
	if bin, err = exec.LookPath(bin); err != nil {
		return nil, investigate.PermanentError(domain.StageInvestigate, fmt.Errorf("agent: the claude CLI is not available: %w", err))
	}
	ri := d.RIBinary
	if ri == "" {
		if ri, err = os.Executable(); err != nil {
			return nil, investigate.PermanentError(domain.StageInvestigate, fmt.Errorf("agent: locate the ri executable: %w", err))
		}
	}
	callID := newUUID()
	s.SetCall(CallInfo{ID: callID, Provider: "anthropic"})

	dir, err := os.MkdirTemp("", "ri-agent-")
	if err != nil {
		return nil, err
	}
	if !d.KeepDir {
		defer os.RemoveAll(dir)
	}
	sock := filepath.Join(dir, "mcp.sock")
	if len(sock) > 100 {
		return nil, investigate.PermanentError(domain.StageInvestigate, fmt.Errorf("agent: socket path %s is too long for a unix socket; set TMPDIR to a shorter directory", sock))
	}
	srv, err := serveSocket(ctx, sock, s)
	if err != nil {
		return nil, err
	}
	cfg := map[string]any{"mcpServers": map[string]any{MCPServerName: map[string]any{
		"type": "stdio", "command": ri, "args": []string{"investigate", "mcp", "--attach", sock, "--token", srv.token}}}}
	cfgJSON, _ := json.MarshalIndent(cfg, "", "  ")
	mcpPath := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(mcpPath, cfgJSON, 0o600); err != nil {
		srv.close()
		return nil, err
	}
	args := []string{"-p", "--output-format", "json", "--session-id", callID,
		"--mcp-config", mcpPath, "--strict-mcp-config", "--tools", "", "--allowedTools", "mcp__" + MCPServerName,
		"--permission-mode", "dontAsk", "--system-prompt", as.SystemPrompt, "--setting-sources", "", "--no-session-persistence"}
	if as.Model != "" {
		args = append(args, "--model", as.Model)
	}
	if as.Budget.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(as.Budget.MaxTurns))
	}
	args = append(args, d.Extra...)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir // neutral: no project CLAUDE.md or settings
	cmd.Stdin = strings.NewReader(as.Task)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if d.Stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, d.Stderr)
	}
	cmd.WaitDelay = 10 * time.Second
	runErr := cmd.Run()
	bridgeErrs := srv.close()

	dr := &investigate.DriverResult{Provider: "anthropic", CallID: callID}
	var env cliEnvelope
	raw := lastJSONObject(stdout.Bytes())
	haveEnv := raw != nil && json.Unmarshal(raw, &env) == nil && env.Type == "result"
	if haveEnv {
		dr.Turns = env.NumTurns
		// Tokens processed for this session: uncached input, cache writes and
		// output. Cache reads (the same context re-read every turn) are in
		// the envelope but not counted against the budget.
		dr.Tokens = env.Usage.InputTokens + env.Usage.CacheCreationTokens + env.Usage.OutputTokens
		dr.Model, dr.ModelVersion = reportedModel(env.ModelUsage, as.Model)
		if env.SessionID != "" && env.SessionID != callID {
			dr.CallID = env.SessionID
		}
	}
	switch {
	case ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded):
		dr.Stop = "budget:seconds"
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case haveEnv && env.SessionID != "" && env.SessionID != callID:
		dr.Stop = fmt.Sprintf("error:the CLI reported session %s, not the requested %s; records made in the session carry the requested id", env.SessionID, callID)
	case s.Submitted():
		dr.Stop = "submitted" // a turn limit hit by the closing message after submit_findings does not undo it
	case haveEnv && env.Subtype == "error_max_turns":
		dr.Stop = "budget:turns"
	case !haveEnv:
		dr.Stop = fmt.Sprintf("error:the claude CLI produced no result envelope (%v): %s", runErr, clip(stderr.String(), 400))
	case env.IsError || env.Subtype != "success":
		dr.Stop = fmt.Sprintf("error:%s: %s", env.Subtype, clip(env.Result, 400))
	default:
		dr.Stop = "error:the session ended without submit_findings"
	}
	if len(bridgeErrs) > 0 && dr.Stop != "submitted" {
		dr.Stop += " (bridge: " + clip(strings.Join(bridgeErrs, "; "), 300) + ")"
	}
	s.RecordEnvelope(DriverClaudeCLI, *dr, raw)
	return dr, nil
}

// reportedModel reads the model(s) the envelope reports: the requested model
// when it is among them, else the first; the version lists every reported
// model id (as the raw arm of the model comparison does).
func reportedModel(usage map[string]json.RawMessage, requested string) (string, string) {
	if len(usage) == 0 {
		return "", ""
	}
	keys := make([]string, 0, len(usage))
	for k := range usage {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	model := keys[0]
	for _, k := range keys {
		if k == requested {
			model = k
		}
	}
	return model, strings.Join(keys, ",")
}

// lastJSONObject returns the last line of out that is a JSON object.
func lastJSONObject(out []byte) json.RawMessage {
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		l := bytes.TrimSpace(lines[i])
		if len(l) > 0 && l[0] == '{' && json.Valid(l) {
			return json.RawMessage(l)
		}
	}
	return nil
}

// newUUID returns a random (version 4) UUID.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
