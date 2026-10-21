#!/bin/sh
# One-line toy fleet status: queue beads + MERGE beads + herdr agents in session fpilot.
. "$HOME/fleetpilot/env.sh"; cd "$HOME/fleetpilot/toy/city" || exit 1
B=$(gc bd --rig toy list --all --json 2>/dev/null | python3 -c '
import json,sys
for x in json.load(sys.stdin):
    if x["id"] in ("tw-7nf","tw-rp4","tw-ngf") or x.get("title","").startswith("MERGE"):
        print("%s:%s:%s" % (x["id"], x["status"], (x.get("assignee") or "")[:16]), end=" ")')
A=$(herdr --session fpilot agent list 2>/dev/null | python3 -c '
import json,sys
for a in json.load(sys.stdin)["result"]["agents"]:
    print("%s:%s" % (a["name"], a["agent_status"]), end=" ")')
echo "$(date +%T) beads[ $B] agents[ $A]"
