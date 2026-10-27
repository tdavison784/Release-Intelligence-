package synthesize

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/llm"
)

// ClaudePrint is an llm.Client that answers one request with one stateless
// `claude -p` call (no API key: the CLI's own login), in the style of
// scripts/semantic-exchange.sh:
//
//	claude -p --model <m> --system-prompt <system> --tools "" --json-schema <schema> \
//	       --output-format json --no-session-persistence --setting-sources "" < message
//
// It runs in an empty directory (no project CLAUDE.md or settings). Model and
// model version are read from the envelope's modelUsage, the call id is the
// envelope's session_id; nothing is assumed. Wrap it in llm.Cache so a
// replay is a cache hit.
type ClaudePrint struct {
	Binary string // default "claude" on PATH
	Extra  []string
}

type printEnvelope struct {
	Type             string                     `json:"type"`
	Subtype          string                     `json:"subtype"`
	IsError          bool                       `json:"is_error"`
	Result           string                     `json:"result"`
	SessionID        string                     `json:"session_id"`
	StructuredOutput json.RawMessage            `json:"structured_output"`
	ModelUsage       map[string]json.RawMessage `json:"modelUsage"`
}

// Complete implements llm.Client.
func (c *ClaudePrint) Complete(ctx context.Context, req llm.Request) (*llm.Response, error) {
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		return nil, errors.New("claude -p: one user message is required")
	}
	bin := c.Binary
	if bin == "" {
		bin = "claude"
	}
	bin, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("claude -p: the claude CLI is not available: %w", err)
	}
	dir, err := os.MkdirTemp("", "ri-synth-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	args := []string{"-p", "--output-format", "json", "--tools", "", "--no-session-persistence", "--setting-sources", "",
		"--system-prompt", req.System}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if len(req.JSONSchema) > 0 {
		args = append(args, "--json-schema", string(req.JSONSchema))
	}
	args = append(args, c.Extra...)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(req.Messages[0].Content)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 10 * time.Second
	runErr := cmd.Run()
	var env printEnvelope
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &env); err != nil || env.Type != "result" {
		return nil, fmt.Errorf("claude -p: no result envelope (%v): %s", runErr, clip(stderr.String(), 400))
	}
	if env.IsError || runErr != nil {
		return nil, fmt.Errorf("claude -p: %s: %s", env.Subtype, clip(env.Result, 400))
	}
	text := env.Result
	if len(env.StructuredOutput) > 0 && string(env.StructuredOutput) != "null" {
		text = string(env.StructuredOutput)
	}
	keys := make([]string, 0, len(env.ModelUsage))
	for k := range env.ModelUsage {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return nil, errors.New("claude -p: the envelope reports no model")
	}
	model := keys[0]
	for _, k := range keys {
		if k == req.Model {
			model = k
		}
	}
	return &llm.Response{Text: text, Model: model, ModelVersion: strings.Join(keys, ","), Origin: llm.OriginAPI,
		CallID: env.SessionID, GeneratedAt: time.Now().UTC().Truncate(time.Second)}, nil
}
