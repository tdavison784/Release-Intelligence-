package agent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// MCP over stdio (newline-delimited JSON-RPC 2.0), the subset a tool server
// needs: initialize, tools/list, tools/call, ping. `ri investigate mcp`
// serves a session with it; the claude-cli driver serves the in-process
// session to the CLI's MCP client through a unix-socket bridge.

// MCPServerName is the server name in the MCP config; Claude Code exposes the
// tools as mcp__<server>__<tool>.
const MCPServerName = "ri"

// mcpProtocolVersions are the MCP revisions this server speaks (newest first).
var mcpProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpCallResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError"`
}

// ServeMCP serves h over one MCP stdio connection until r ends. Requests are
// handled one at a time, in order (the transcript has one order).
func ServeMCP(ctx context.Context, r io.Reader, w io.Writer, h ToolHandler) error {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReaderSize(r, 1<<20)
	}
	var wmu sync.Mutex
	send := func(resp rpcResponse) error {
		resp.JSONRPC = "2.0"
		if len(resp.ID) == 0 {
			resp.ID = json.RawMessage("null")
		}
		b, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		wmu.Lock()
		defer wmu.Unlock()
		_, err = w.Write(append(b, '\n'))
		return err
	}
	for {
		line, rerr := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			if err := handleRPC(ctx, line, h, send); err != nil {
				return err
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func handleRPC(ctx context.Context, line []byte, h ToolHandler, send func(rpcResponse) error) error {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return send(rpcResponse{Error: &rpcError{Code: -32700, Message: "parse error: " + err.Error()}})
	}
	notification := len(req.ID) == 0 || string(req.ID) == "null"
	if notification {
		return nil // notifications/initialized, notifications/cancelled, ...: nothing to answer
	}
	fail := func(code int, msg string) error {
		return send(rpcResponse{ID: req.ID, Error: &rpcError{Code: code, Message: msg}})
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := mcpProtocolVersions[0]
		for _, v := range mcpProtocolVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		return send(rpcResponse{ID: req.ID, Result: map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "ri-investigate", "version": Producer},
			"instructions":    "Release Intelligence investigation tools for one investigation. Every call is validated, budgeted and recorded.",
		}})
	case "ping":
		return send(rpcResponse{ID: req.ID, Result: map[string]any{}})
	case "tools/list":
		return send(rpcResponse{ID: req.ID, Result: map[string]any{"tools": h.Schemas()}})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return fail(-32602, "tools/call needs params.name")
		}
		res, err := h.Dispatch(ctx, p.Name, p.Arguments)
		if err != nil {
			return send(rpcResponse{ID: req.ID, Result: mcpCallResult{Content: []mcpContent{{Type: "text", Text: err.Error()}}, IsError: true}})
		}
		return send(rpcResponse{ID: req.ID, Result: mcpCallResult{Content: []mcpContent{{Type: "text", Text: string(res)}}}})
	default:
		return fail(-32601, "method not found: "+req.Method)
	}
}

// --- the unix-socket bridge ----------------------------------------------------------------

// attachHello is the first line a bridge sends: the token of the session it
// attaches to.
type attachHello struct {
	Attach string `json:"attach"`
}

// Attach connects stdio to a session served on a unix socket by the
// claude-cli driver (`ri investigate mcp --attach <socket> --token <t>`): it
// sends the handshake, then copies bytes both ways until either side ends.
func Attach(ctx context.Context, socket, token string, stdin io.Reader, stdout io.Writer) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socket)
	if err != nil {
		return fmt.Errorf("agent: attach %s: %w", socket, err)
	}
	defer conn.Close()
	hello, _ := json.Marshal(attachHello{Attach: token})
	if _, err := conn.Write(append(hello, '\n')); err != nil {
		return fmt.Errorf("agent: attach handshake: %w", err)
	}
	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(conn, stdin)
		if uc, ok := conn.(*net.UnixConn); ok {
			_ = uc.CloseWrite()
		}
		done <- err
	}()
	go func() {
		_, err := io.Copy(stdout, conn)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// socketServer serves a ToolHandler to bridges that present the token.
type socketServer struct {
	ln    net.Listener
	token string
	h     ToolHandler
	wg    sync.WaitGroup
	mu    sync.Mutex
	conns []net.Conn
	errs  []string
}

func newToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func serveSocket(ctx context.Context, path string, h ToolHandler) (*socketServer, error) {
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("agent: listen %s: %w", path, err)
	}
	s := &socketServer{ln: ln, token: newToken(), h: h}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns = append(s.conns, conn)
			s.mu.Unlock()
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				defer conn.Close()
				if err := s.serve(ctx, conn); err != nil {
					s.mu.Lock()
					s.errs = append(s.errs, err.Error())
					s.mu.Unlock()
				}
			}()
		}
	}()
	return s, nil
}

func (s *socketServer) serve(ctx context.Context, conn net.Conn) error {
	br := bufio.NewReaderSize(conn, 1<<20)
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	line, err := br.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("bridge handshake: %w", err)
	}
	var hello attachHello
	if json.Unmarshal(line, &hello) != nil || hello.Attach != s.token {
		return errors.New("bridge handshake: wrong token")
	}
	_ = conn.SetReadDeadline(time.Time{})
	return ServeMCP(ctx, br, conn, s.h)
}

// close stops accepting, closes open bridges and waits for the handlers.
func (s *socketServer) close() []string {
	_ = s.ln.Close()
	s.mu.Lock()
	for _, c := range s.conns {
		_ = c.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, e := range s.errs {
		if !strings.Contains(e, "use of closed network connection") {
			out = append(out, e)
		}
	}
	return out
}
