#!/usr/bin/env python3
"""Sum Claude Code transcript usage per session and price it at Haiku 4.5 list rates (USD/MTok)."""
import glob, json, os, sys
P = {"in": 1.0, "out": 5.0, "w5m": 1.25, "w1h": 2.0, "read": 0.10}
tot = 0.0
for f in sorted(glob.glob(os.path.expanduser("~/.claude/projects/-Users-tommydavison-fleetpilot-*/*.jsonl"))):
    seen, u, models = set(), dict(inp=0, out=0, w5m=0, w1h=0, read=0), set()
    for line in open(f):
        d = json.loads(line)
        m = d.get("message") or {}
        if d.get("type") != "assistant" or "usage" not in m: continue
        if m.get("id") in seen: continue          # streamed chunks repeat the same message id
        seen.add(m.get("id")); models.add(m.get("model"))
        x = m["usage"]; cc = x.get("cache_creation") or {}
        u["inp"] += x.get("input_tokens", 0); u["out"] += x.get("output_tokens", 0); u["read"] += x.get("cache_read_input_tokens", 0)
        w1h = cc.get("ephemeral_1h_input_tokens", 0); w5 = cc.get("ephemeral_5m_input_tokens", x.get("cache_creation_input_tokens", 0) - w1h)
        u["w5m"] += w5; u["w1h"] += w1h
    usd = (u["inp"]*P["in"] + u["out"]*P["out"] + u["w5m"]*P["w5m"] + u["w1h"]*P["w1h"] + u["read"]*P["read"]) / 1e6
    tot += usd
    print(f"{os.path.basename(os.path.dirname(f))[34:]:38} {len(seen):3} msgs  {u}  models={sorted(filter(None, models))}  ${usd:.3f}")
print(f"TOTAL ${tot:.3f}")
