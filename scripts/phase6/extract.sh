#!/usr/bin/env bash
# Standardised extraction of one run's assessment (PROTOCOL §8).
#
#   scripts/phase6/extract.sh <run-dir>
#
# One stateless `claude -p --model claude-opus-5-5` call over <run-dir>/assessment.md
# only, with the fixed prompt prompts/extract.md and schema prompts/extract.schema.json;
# no tools, no settings, no MCP, in an empty directory with a scrubbed
# environment. Writes extraction.json (the structured report),
# extraction-envelope.json (the CLI's result event) and extraction-meta.json.
# assessment.md is never modified.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
. "$here/lib.sh"
[ $# -eq 1 ] || { echo "usage: $0 <run-dir>" >&2; exit 2; }
run_dir=$(cd "$1" && pwd)
a=$run_dir/assessment.md
[ -s "$a" ] || { echo "$a is missing or empty" >&2; exit 1; }
[ ! -e "$run_dir/extraction.json" ] || { echo "$run_dir/extraction.json exists" >&2; exit 1; }

tmp=$(mktemp -d "${TMPDIR:-/tmp}/p6-extract.XXXXXX")
trap 'rm -rf "${tmp:?}"' EXIT
mkdir -p "$tmp/cwd" "$tmp/tmp"
python3 - "$here/prompts/extract.md" "$a" >"$tmp/prompt.txt" <<'EOF'
import sys
print(open(sys.argv[1]).read().replace("{ASSESSMENT}", open(sys.argv[2]).read().strip("\n")))
EOF

p6_env "$tmp/tmp"
start=$(date +%s)
(cd "$tmp/cwd" && "${P6_ENV[@]}" "$(p6_claude_bin)" -p --model "$P6_EXTRACTOR_MODEL" --output-format json \
  --safe-mode --setting-sources "" --strict-mcp-config --disable-slash-commands --settings "$P6_SETTINGS" \
  --tools "" --no-session-persistence --json-schema "$(cat "$here/prompts/extract.schema.json")" \
  <"$tmp/prompt.txt" >"$run_dir/extraction-envelope.json" 2>"$tmp/stderr.txt") || true
end=$(date +%s)

python3 - "$run_dir" "$here" "$((end-start))" "$(git -C "$here" rev-parse HEAD)" "$(p6_claude_bin)" "$tmp/stderr.txt" <<'EOF'
import hashlib, json, os, subprocess, sys
run_dir, here, wall, commit, claude, stderr = sys.argv[1:7]
sha = lambda p: hashlib.sha256(open(p, "rb").read()).hexdigest()
try:
    env = json.load(open(os.path.join(run_dir, "extraction-envelope.json")))
except ValueError:
    env = {}
out = env.get("structured_output")
ok = isinstance(out, dict) and not env.get("is_error")
if ok:
    json.dump(out, open(os.path.join(run_dir, "extraction.json"), "w"), indent=2, ensure_ascii=False)
u = env.get("usage") or {}
meta = {
    "status": "ok" if ok else "error", "error": None if ok else (env.get("result") or open(stderr).read()[:500]),
    "model": {"requested": "claude-opus-5-5", "used": sorted((env.get("modelUsage") or {}).keys())},
    "cliVersion": subprocess.run([claude, "--version"], capture_output=True, text=True).stdout.split()[0],
    "input": {"assessmentSha256": sha(os.path.join(run_dir, "assessment.md"))},
    "prompt": {"path": "scripts/phase6/prompts/extract.md", "sha256": sha(os.path.join(here, "prompts/extract.md")),
               "schemaSha256": sha(os.path.join(here, "prompts/extract.schema.json"))},
    "tokens": {"processed": u.get("input_tokens", 0) + u.get("cache_creation_input_tokens", 0) + u.get("output_tokens", 0), "usage": u},
    "costUSD": env.get("total_cost_usd"), "wallSeconds": int(wall), "harnessCommit": commit,
}
json.dump(meta, open(os.path.join(run_dir, "extraction-meta.json"), "w"), indent=2)
print(json.dumps({"status": meta["status"], "mechanisms": len(out["mechanisms"]) if ok else None, "costUSD": meta["costUSD"]}))
sys.exit(0 if ok else 1)
EOF
