#!/usr/bin/env bash
# Answer ri LLM-exchange requests with stateless `claude -p` calls, one isolated call per request
# (generalises docs/phase3/model-comparison/raw-arm.sh for any exchange directory and any model).
#
#   scripts/semantic-exchange.sh <exchange-dir> [model] [parallel]
#
# Each request is answered by the model named in the request (`request.model`), or by [model]
# when given. One call per prompt: system = request.system, one user message, the request's JSON
# schema as structured output, thinking off (MAX_THINKING_TOKENS=0), no tools, no project
# settings, no session persistence — nothing shared between prompts. The response file records
# the model that answered and the exact version from the CLI envelope (`modelUsage` keys), so
# provenance is `provider: anthropic`, served model as reported. The full envelope is kept next
# to the response (*.claude-p.json) for audit (usage, cost).
#
# A refused or failed call is never retried and never fabricated: the request stays pending and the
# failure is logged to <exchange-dir>/exchange.log. Re-running answers only what is still pending.
# Then re-run the ri command with the same -llm-exchange DIR to ingest the answers.
set -uo pipefail
DIR=$(cd "$1" && pwd) || { echo "usage: $0 <exchange-dir> [model] [parallel]" >&2; exit 2; }
OVERRIDE=${2:-}
PAR=${3:-4}
LOG="$DIR/exchange.log"
WORK=$(mktemp -d) # neutral cwd: no repo CLAUDE.md, no project settings
trap 'rm -rf "$WORK"' EXIT
export DIR OVERRIDE LOG WORK

answer_one() {
  req=$1
  resp="${req%.request.json}.response.json"
  [ -e "$resp" ] && return 0
  if [ "$(jq '.request.messages | length' "$req")" != 1 ] || [ "$(jq -r '.request.messages[0].role' "$req")" != user ]; then
    echo "SKIP (not a single user message): $(basename "$req")" >>"$LOG"; return 0
  fi
  model=${OVERRIDE:-$(jq -r '.request.model // empty' "$req")}
  if [ -z "$model" ]; then echo "SKIP (no model): $(basename "$req")" >>"$LOG"; return 0; fi
  out=$(cd "$WORK" && jq -r '.request.messages[0].content' "$req" | MAX_THINKING_TOKENS=0 claude -p --safe-mode \
    --model "$model" --system-prompt "$(jq -r '.request.system' "$req")" --tools "" \
    --json-schema "$(jq -c '.request.jsonSchema' "$req")" --output-format json --no-session-persistence 2>>"$LOG")
  if [ $? -ne 0 ] || [ "$(jq -r '.structured_output | type' <<<"$out" 2>/dev/null)" != object ]; then
    echo "FAIL $model $(basename "$req"): $(jq -c '{is_error,subtype,result}' <<<"$out" 2>/dev/null | head -c 400)" >>"$LOG"
    return 0
  fi
  echo "$out" >"${req%.request.json}.claude-p.json"
  version=$(jq -r '.modelUsage | keys | join(",")' <<<"$out")
  [ -n "$version" ] || version=$model
  jq -n --arg d "$(jq -r .promptDigest "$req")" --arg m "$model" --arg t "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg v "$version" --argjson o "$(jq -c .structured_output <<<"$out")" \
    '{format:"ri.dev/llm-exchange/response/v1",promptDigest:$d,model:$m,modelVersion:$v,generatedAt:$t,output:$o}' >"$resp.tmp" &&
    mv "$resp.tmp" "$resp"
  echo "OK $model $(basename "$req")" >>"$LOG"
}
export -f answer_one

n=$(find "$DIR" -maxdepth 1 -name '*.request.json' | wc -l | tr -d ' ')
find "$DIR" -maxdepth 1 -name '*.request.json' -print0 | xargs -0 -n1 -P "$PAR" bash -c 'answer_one "$0"'
ok=$(find "$DIR" -maxdepth 1 -name '*.response.json' | wc -l | tr -d ' ')
echo "semantic-exchange: $ok of $n requests answered in $DIR (log: $LOG)"
