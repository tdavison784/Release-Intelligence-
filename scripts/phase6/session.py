#!/usr/bin/env python3
"""Run one `claude -p --output-format stream-json` session and capture it.

    session.py <run-dir> <budget-tokens> <prompt-file> -- <command...>

Streams stdout to <run-dir>/transcript.jsonl and stderr to stderr.txt, saves
the init event as init.json, and stops the session (SIGTERM to its process
group) once the tokens it has processed exceed the budget (PROTOCOL §9).
Tokens are counted as RI's agent driver counts them (claudecli.go): uncached
input + cache writes + output, cache reads excluded. While streaming, output
is estimated (the CLI streams partial output counts) from the characters of
the emitted blocks; the result event's totals replace the estimate when the
session ends on its own. Writes <run-dir>/session.json for run-natural.sh.
"""
import json, os, signal, subprocess, sys, time

run_dir, budget, prompt_file = sys.argv[1], int(sys.argv[2]), sys.argv[3]
cmd = sys.argv[sys.argv.index("--") + 1:]

msgs = {}        # message id -> [input + cache writes, estimated output chars]
result = init = None
stopped = None
tool_calls = 0
start = time.time()

with open(prompt_file, "rb") as stdin, \
     open(os.path.join(run_dir, "transcript.jsonl"), "wb") as tr, \
     open(os.path.join(run_dir, "stderr.txt"), "wb") as err:
    p = subprocess.Popen(cmd, stdin=stdin, stdout=subprocess.PIPE, stderr=err, start_new_session=True)
    for line in p.stdout:
        tr.write(line)
        tr.flush()
        try:
            ev = json.loads(line)
        except ValueError:
            continue
        t = ev.get("type")
        if t == "system" and ev.get("subtype") == "init" and init is None:
            init = ev
            with open(os.path.join(run_dir, "init.json"), "w") as f:
                json.dump(ev, f, indent=2)
        elif t == "assistant":
            m = ev.get("message", {})
            u = m.get("usage") or {}
            rec = msgs.setdefault(m.get("id"), [0, 0])
            rec[0] = max(rec[0], u.get("input_tokens", 0) + u.get("cache_creation_input_tokens", 0))
            for b in m.get("content", []):
                rec[1] += len(json.dumps(b))
                tool_calls += b.get("type") == "tool_use"
        elif t == "result":
            result = ev
        estimate = sum(a + c // 4 for a, c in msgs.values())
        if stopped is None and result is None and estimate > budget:
            stopped = f"budget: ~{estimate} tokens processed > {budget}"
            os.killpg(p.pid, signal.SIGTERM)
    code = p.wait()

wall = time.time() - start
if result is not None:
    u = result.get("usage", {})
    tokens = u.get("input_tokens", 0) + u.get("cache_creation_input_tokens", 0) + u.get("output_tokens", 0)
    tokens_basis = "result event"
else:
    tokens = sum(a + c // 4 for a, c in msgs.values())
    tokens_basis = "stream estimate (no result event)"
if stopped is None and tokens > budget:
    stopped = f"budget: {tokens} tokens processed > {budget} (exceeded in the final turn)"
status = "budget-stop" if stopped else ("completed" if result and not result.get("is_error") and code == 0 else "error")
json.dump({
    "status": status, "stop": stopped, "exitCode": code, "wallSeconds": round(wall, 1),
    "tokens": tokens, "tokensBasis": tokens_basis, "budgetTokens": budget,
    "toolCalls": tool_calls, "apiMessages": len(msgs), "result": {k: v for k, v in (result or {}).items() if k != "result"},
    "init": {k: (init or {}).get(k) for k in ("model", "claude_code_version", "cwd", "tools", "mcp_servers", "plugins", "skills", "permissionMode", "apiKeySource", "session_id")},
}, open(os.path.join(run_dir, "session.json"), "w"), indent=2)
sys.exit(0 if status == "completed" else 1)
