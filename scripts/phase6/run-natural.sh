#!/usr/bin/env bash
# One natural Claude Code investigation (PROTOCOL §5, §6, §8, §9).
#
#   scripts/phase6/run-natural.sh <workspace> <C-whole|C-item> <run-dir> [<section-file>]
#
# <workspace> is a build-workspace.sh output; it is never modified. The run
# works in a fresh copy under $P6_LIVE (kept: arm D resumes it). C-item needs
# <section-file>, the verbatim upstream section the commander provides. The
# arm "canary" (canary.sh) takes its whole prompt from <section-file>.
# <run-dir> (new or empty) receives prompt.txt, transcript.jsonl, init.json,
# stderr.txt, assessment.md (when written), scratch/, tool-calls.tsv,
# workspace-MANIFEST.sha256 and meta.json. A run that errors or hits the
# budget is kept as it is (never retried here).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
. "$here/lib.sh"

[ $# -ge 3 ] || { echo "usage: $0 <workspace> <C-whole|C-item> <run-dir> [<section-file>]" >&2; exit 2; }
ws=$(cd "$1" && pwd); arm=$2; run_dir=$3; section=${4:-}
case "$arm" in
  C-whole) [ -z "$section" ] || { echo "C-whole takes no section file" >&2; exit 2; } ;;
  C-item|canary) [ -f "$section" ] || { echo "$arm needs <section-file>" >&2; exit 2; } ;;
  *) echo "arm must be C-whole or C-item" >&2; exit 2 ;;
esac
[ -f "$ws/MANIFEST.sha256" ] || { echo "$ws is not a built workspace" >&2; exit 1; }
(cd "$ws" && shasum -a 256 -c --quiet MANIFEST.sha256) || { echo "$ws does not match its MANIFEST.sha256" >&2; exit 1; }
[ -z "$(ls -A "$ws/scratch")" ] || { echo "$ws/scratch is not empty" >&2; exit 1; }
if [ -e "$run_dir" ] && [ -n "$(ls -A "$run_dir")" ]; then echo "$run_dir is not empty" >&2; exit 1; fi
mkdir -p "$run_dir"; run_dir=$(cd "$run_dir" && pwd)
repo=$(git -C "$here" rev-parse --show-toplevel)

# Fresh pristine copy.
# A neutral path: the agent sees its cwd, so it carries no case id or arm.
run_id=r-$(uuidgen | tr -d - | cut -c1-12 | tr 'A-Z' 'a-z')
live=$P6_LIVE/$run_id
mkdir -p "$live/tmp"
cp -R "$ws" "$live/ws"
chmod -R u+w "$live/ws"; chmod -R a-w "$live/ws"; chmod u+w "$live/ws/scratch"
(cd "$live/ws" && shasum -a 256 -c --quiet MANIFEST.sha256)
cp "$ws/MANIFEST.sha256" "$run_dir/workspace-MANIFEST.sha256"

# Prompt (PROTOCOL §6): the template, with product and versions from ORIENTATION.md.
orient() { sed -n "s/^- $1: //p" "$ws/ORIENTATION.md" 2>/dev/null; }
tick() { local v; v=$(orient "$1"); case "$v" in *" ("*) echo "\`${v%% (*}\` (${v#* (}" ;; *) echo "\`$v\`" ;; esac; }
python3 - "$here/prompts/c-whole.txt" "$arm" "\`$(orient Product)\`" "$(tick 'Current version')" "$(tick 'Candidate version')" "$section" >"$run_dir/prompt.txt" <<'EOF'
import sys
tpl, arm, product, frm, to, section = sys.argv[1:7]
p = open(tpl).read().strip().replace("{PRODUCT}", product).replace("{FROM}", frm).replace("{TO}", to)
whole = "Determine whether this upgrade introduces breaking changes for this specific environment."
assert whole in p
if arm == "canary":
    p = open(section).read().strip("\n")
elif arm == "C-item":
    text = open(section).read().strip("\n")
    p = p.replace(whole, "Determine whether the following upstream change, part of this upgrade, breaks this specific environment:\n\n" + text + "\n\n")
print(p)
EOF

sid=$(uuidgen | tr 'A-Z' 'a-z')
claude=$(p6_claude_bin)
p6_env "$live/tmp"
set +e
(cd "$live/ws" && python3 "$here/session.py" "$run_dir" "$P6_BUDGET_TOKENS" "$run_dir/prompt.txt" -- \
  "${P6_ENV[@]}" "$claude" -p --model "$P6_INVESTIGATOR_MODEL" --output-format stream-json --verbose \
  --session-id "$sid" "${P6_ISOLATION_ARGS[@]}" --allowedTools "${P6_ALLOWED[@]}" $(p6_ws_rules "$live/ws") "$(p6_tool_results_rule "$live/ws" "$sid")")
rc=$?
set -e

# Collect what the session wrote, and check the fixtures are unchanged.
cp -R "$live/ws/scratch" "$run_dir/scratch"
[ -f "$live/ws/scratch/assessment.md" ] && cp "$live/ws/scratch/assessment.md" "$run_dir/assessment.md"
fixtures_ok=true; (cd "$live/ws" && shasum -a 256 -c --quiet MANIFEST.sha256) || fixtures_ok=false
extra=$(cd "$live/ws" && comm -13 <(awk '{print $2}' MANIFEST.sha256 | LC_ALL=C sort) \
  <(find . -type f ! -path './scratch/*' ! -name MANIFEST.sha256 | sed 's|^\./||' | LC_ALL=C sort))
[ -z "$extra" ] || fixtures_ok=false

# Every tool call, and any that names a path outside the workspace.
python3 - "$run_dir/transcript.jsonl" "$live/ws" "$(p6_project_dir "$live/ws")/$sid/tool-results" >"$run_dir/tool-calls.tsv" <<'EOF'
import json, sys
ws, own = sys.argv[2], sys.argv[3]  # own: the session's saved oversized outputs (readable by rule)
print("tool\tinput\toutside-workspace")
for line in open(sys.argv[1]):
    try: ev = json.loads(line)
    except ValueError: continue
    if ev.get("type") != "assistant": continue
    for b in ev["message"].get("content", []):
        if b.get("type") != "tool_use": continue
        i = b.get("input", {})
        arg = i.get("command") or i.get("file_path") or i.get("path") or i.get("pattern") or json.dumps(i)
        refs = [w for w in str(arg).replace('"', ' ').replace("'", " ").split()
                if w.startswith(("/", "~", "..")) and not w.startswith((ws, own))]
        print(f"{b['name']}\t{' '.join(str(arg).split())[:300]}\t{' '.join(refs)}")
EOF

python3 - "$run_dir" "$arm" "$ws" "$live" "$sid" "$fixtures_ok" "$(git -C "$repo" rev-parse HEAD)" "$section" "$rc" <<'EOF'
import hashlib, json, os, sys
run_dir, arm, ws, live, sid, fixtures_ok, commit, section, rc = sys.argv[1:10]
s = json.load(open(os.path.join(run_dir, "session.json")))
sha = lambda p: hashlib.sha256(open(p, "rb").read()).hexdigest()
r = s["result"]
outside = sum(1 for l in open(os.path.join(run_dir, "tool-calls.tsv")).read().splitlines()[1:] if l.split("\t")[2:] and l.split("\t")[2])
meta = {
    "arm": arm, "case": os.path.basename(ws), "status": s["status"], "stop": s["stop"], "exitCode": int(rc),
    "model": {"requested": "claude-sonnet-5-5", "init": s["init"]["model"], "used": sorted((r.get("modelUsage") or {}).keys())},
    "cliVersion": s["init"]["claude_code_version"],
    "effort": "CLI default (no --effort; CLAUDE_EFFORT and other session env scrubbed; user/project/local settings not loaded)",
    "tokens": {"processed": s["tokens"], "basis": s["tokensBasis"], "budget": s["budgetTokens"], "usage": r.get("usage")},
    "costUSD": r.get("total_cost_usd"), "wallSeconds": s["wallSeconds"], "turns": r.get("num_turns"),
    "toolCalls": s["toolCalls"], "toolCallsNamingOutsidePaths": outside,
    "permissionDenials": len(r.get("permission_denials") or []),
    "workspace": {"source": ws, "manifestSha256": sha(os.path.join(ws, "MANIFEST.sha256")), "live": live + "/ws",
                  "fixturesUnchangedAfterRun": fixtures_ok == "true"},
    "sessionId": sid, "promptSha256": sha(os.path.join(run_dir, "prompt.txt")),
    "sectionFile": ({"path": section, "sha256": sha(section)} if section else None),
    "assessmentWritten": os.path.exists(os.path.join(run_dir, "assessment.md")),
    "harnessCommit": commit, "init": s["init"],
}
json.dump(meta, open(os.path.join(run_dir, "meta.json"), "w"), indent=2)
os.remove(os.path.join(run_dir, "session.json"))
print(json.dumps({k: meta[k] for k in ("status", "stop", "turns", "toolCalls", "costUSD", "wallSeconds", "assessmentWritten")}))
EOF
exit $rc
