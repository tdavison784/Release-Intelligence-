#!/usr/bin/env python3
"""PreToolUse guard for the toy fleet: deny any tool call that names SEALED.md, and any git push.
Exit 2 + stderr = Claude Code blocks the call and shows the reason to the model. Every decision is logged."""
import json, os, re, sys, time
LOG = os.path.expanduser("~/fleetpilot/logs/guard.jsonl")
ev = json.load(sys.stdin)
blob = json.dumps(ev.get("tool_input", {}))
why = None
if re.search(r"sealed", blob, re.I):
    why = "forbidden path: SEALED.md must never be read (fleetpilot rule)"
elif ev.get("tool_name") == "Bash" and re.search(r"\bgit\b[^|;&]*\bpush\b", ev.get("tool_input", {}).get("command", "")):
    why = "git push is forbidden without the user (fleetpilot rule)"
with open(LOG, "a") as f:
    f.write(json.dumps({"t": time.strftime("%FT%TZ", time.gmtime()), "agent": os.environ.get("GC_AGENT", ""),
                        "tool": ev.get("tool_name"), "deny": why, "input": blob[:300]}) + "\n")
if why:
    print(why, file=sys.stderr)
    sys.exit(2)
