#!/usr/bin/env bash
# Arm D (PROTOCOL §7): the product owner continues a recorded run interactively.
#
#   scripts/phase6/guided.sh <run-dir>
#
# Resumes the run's session as a fork (the recorded unaided session is left
# untouched) in the run's own live workspace, with the same model, isolation
# and tool limits as run-natural.sh, in an interactive terminal. The product
# owner types; exit the session (/exit or Ctrl-D twice) when done. Afterwards
# <run-dir>/guided/ holds the forked session's full transcript, the scratch
# directory, assessment.md (if the session rewrote it), meta.json, and
# interventions.md: one entry per human turn, verbatim, with the category
# left for the product owner to fill in.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
. "$here/lib.sh"
[ $# -eq 1 ] || { echo "usage: $0 <run-dir>" >&2; exit 2; }
run_dir=$(cd "$1" && pwd)
g=$run_dir/guided
[ ! -e "$g" ] || { echo "$g exists: one guided session per run" >&2; exit 1; }
sid=$(jq -r .sessionId "$run_dir/meta.json")
live=$(jq -r .workspace.live "$run_dir/meta.json")
[ -d "$live" ] || { echo "live workspace $live is gone; arm D needs it" >&2; exit 1; }
(cd "$live" && shasum -a 256 -c --quiet MANIFEST.sha256) || { echo "$live no longer matches its manifest" >&2; exit 1; }
cmp -s "$run_dir/assessment.md" "$live/scratch/assessment.md" || { echo "$live/scratch differs from the recorded run" >&2; exit 1; }

new=$(uuidgen | tr 'A-Z' 'a-z')
mkdir -p "$g" "$(dirname "$live")/tmp"
p6_env "$(dirname "$live")/tmp"
started=$(date -u +%Y-%m-%dT%H:%M:%SZ)
echo "Resuming session $sid as $new in $live (model $P6_INVESTIGATOR_MODEL). Exit with /exit when done." >&2
set +e
(cd "$live" && "${P6_ENV[@]}" "$(p6_claude_bin)" --model "$P6_INVESTIGATOR_MODEL" \
  --resume "$sid" --fork-session --session-id "$new" "${P6_ISOLATION_ARGS[@]}" \
  --allowedTools "${P6_ALLOWED[@]}" $(p6_ws_rules "$live") "$(p6_tool_results_rule "$live" "$sid")" "$(p6_tool_results_rule "$live" "$new")")
rc=$?
set -e
ended=$(date -u +%Y-%m-%dT%H:%M:%SZ)

session=$(p6_project_dir "$live")/$new.jsonl
[ -f "$session" ] || { echo "no session file at $session (nothing was sent?)" >&2; exit 1; }
cp "$session" "$g/transcript.jsonl"
cp -R "$live/scratch" "$g/scratch"
cmp -s "$run_dir/assessment.md" "$live/scratch/assessment.md" || cp "$live/scratch/assessment.md" "$g/assessment.md" 2>/dev/null || true
fixtures_ok=true; (cd "$live" && shasum -a 256 -c --quiet MANIFEST.sha256) || fixtures_ok=false

python3 - "$g" "$run_dir/prompt.txt" "$sid" "$new" "$started" "$ended" "$rc" "$fixtures_ok" "$(p6_claude_bin)" <<'EOF'
import json, os, subprocess, sys
g, prompt, parent, new, started, ended, rc, fixtures_ok, claude = sys.argv[1:10]
first = open(prompt).read().strip()
turns = []
for line in open(os.path.join(g, "transcript.jsonl")):
    try: e = json.loads(line)
    except ValueError: continue
    if e.get("type") != "user" or e.get("isMeta"): continue
    c = e.get("message", {}).get("content")
    if isinstance(c, list):
        if any(b.get("type") == "tool_result" for b in c if isinstance(b, dict)): continue
        c = "\n".join(b.get("text", "") for b in c if isinstance(b, dict) and b.get("type") == "text")
    if not isinstance(c, str) or not c.strip(): continue
    if c.strip() == first: continue  # the recorded unaided prompt
    turns.append((e.get("timestamp", ""), c))
with open(os.path.join(g, "interventions.md"), "w") as f:
    f.write(f"# Arm D interventions\n\nSession `{new}` (fork of `{parent}`), {started} to {ended}. "
            f"One entry per human turn, verbatim. The product owner fills in each category (PROTOCOL §7): "
            f"task clarification | pointer to existing evidence | newly supplied context | hint toward a conclusion | correction of a mistake.\n")
    for i, (ts, text) in enumerate(turns, 1):
        fence = "````" if "```" in text else "```"
        f.write(f"\n## Turn {i} ({ts})\n\n{fence}text\n{text}\n{fence}\n\nCategory: \n")
json.dump({"sessionId": new, "parentSessionId": parent, "started": started, "ended": ended, "exitCode": int(rc),
           "humanTurns": len(turns), "model": "claude-sonnet-5-5",
           "cliVersion": subprocess.run([claude, "--version"], capture_output=True, text=True).stdout.split()[0],
           "fixturesUnchanged": fixtures_ok == "true", "assessmentRewritten": os.path.exists(os.path.join(g, "assessment.md"))},
          open(os.path.join(g, "meta.json"), "w"), indent=2)
print(f"{len(turns)} human turn(s) recorded in {g}/interventions.md")
EOF
