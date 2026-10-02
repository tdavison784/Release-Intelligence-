#!/usr/bin/env bash
# Answer ri LLM-exchange requests with stateless `claude -p` calls, one isolated call per request
# (generalises docs/phase3/model-comparison/raw-arm.sh for any exchange directory and any model).
#
#   scripts/semantic-exchange.sh <exchange-dir> [model] [parallel] [provider]
#
# provider: anthropic (default; the CLI's own login) or zai (GLM via Z.AI's Anthropic-compatible
# gateway: ANTHROPIC_BASE_URL=https://api.z.ai/api/anthropic, ANTHROPIC_AUTH_TOKEN read from
# ~/.claude_token_zai inside each call's environment — never printed, never logged, never passed
# as an argument; ANTHROPIC_API_KEY is unset for those calls so it cannot take precedence). Run the
# matching `ri semantic propose -provider zai -model glm-5.3 …` so proposals record provider zai.
#
# Each request is answered by the model named in the request (`request.model`), or by [model]
# when given. One call per prompt: system = request.system, one user message, the request's JSON
# schema as structured output, thinking off (MAX_THINKING_TOKENS=0), no tools, no project
# settings, no session persistence — nothing shared between prompts. The response file records
# the model that answered and the exact version from the CLI envelope (`modelUsage` keys), so
# provenance is `provider: anthropic`, served model as reported. The full envelope is kept next
# to the response (*.claude-p.json) for audit (usage, cost). The envelope's session_id is the
# response's callId: every call is its own session, so two answers are checkably separate calls.
#
# A refused or failed call is never retried and never fabricated: the request stays pending and the
# failure is logged to <exchange-dir>/exchange.log. Re-running answers only what is still pending.
# Then re-run the ri command with the same -llm-exchange DIR to ingest the answers.
set -uo pipefail
DIR=$(cd "$1" && pwd) || { echo "usage: $0 <exchange-dir> [model] [parallel] [provider]" >&2; exit 2; }
OVERRIDE=${2:-}
PAR=${3:-4}
PROVIDER=${4:-anthropic}
ZAI_TOKEN_FILE=${ZAI_TOKEN_FILE:-$HOME/.claude_token_zai}
case "$PROVIDER" in
anthropic) ;;
zai) [ -s "$ZAI_TOKEN_FILE" ] || { echo "provider zai: token file $ZAI_TOKEN_FILE missing or empty" >&2; exit 2; } ;;
*) echo "unknown provider $PROVIDER (anthropic|zai)" >&2; exit 2 ;;
esac
LOG="$DIR/exchange.log"
WORK=$(mktemp -d) # neutral cwd: no repo CLAUDE.md, no project settings
trap 'rm -rf "$WORK"' EXIT
export DIR OVERRIDE LOG WORK PROVIDER ZAI_TOKEN_FILE

# call_claude runs one stateless claude -p call with the provider's environment. For zai the token
# goes into the child's environment only; stderr is scrubbed of it before reaching the log.
call_claude() {
  if [ "$PROVIDER" = zai ]; then
    # subshell: the token is exported into the environment only (never an argument, so never
    # visible in the process list), and stderr passes a literal-match scrubber before the log
    (
      unset ANTHROPIC_API_KEY
      export ANTHROPIC_BASE_URL=https://api.z.ai/api/anthropic
      ANTHROPIC_AUTH_TOKEN=$(tr -d '\n\r' <"$ZAI_TOKEN_FILE")
      export ANTHROPIC_AUTH_TOKEN
      MAX_THINKING_TOKENS=0 claude -p --safe-mode "$@" 2> >(awk 'BEGIN { t = ENVIRON["ANTHROPIC_AUTH_TOKEN"] }
        { while (t != "" && (i = index($0, t)) > 0) $0 = substr($0, 1, i - 1) "[redacted]" substr($0, i + length(t)); print }' >>"$LOG")
    )
  else
    MAX_THINKING_TOKENS=0 claude -p --safe-mode "$@" 2>>"$LOG"
  fi
}
export -f call_claude

answer_one() {
  req=$1
  resp="${req%.request.json}.response.json"
  [ -e "$resp" ] && return 0
  if [ "$(jq '.request.messages | length' "$req")" != 1 ] || [ "$(jq -r '.request.messages[0].role' "$req")" != user ]; then
    echo "SKIP (not a single user message): $(basename "$req")" >>"$LOG"; return 0
  fi
  model=${OVERRIDE:-$(jq -r '.request.model // empty' "$req")}
  if [ -z "$model" ]; then echo "SKIP (no model): $(basename "$req")" >>"$LOG"; return 0; fi
  out=$(cd "$WORK" && jq -r '.request.messages[0].content' "$req" | call_claude \
    --model "$model" --system-prompt "$(jq -r '.request.system' "$req")" --tools "" \
    --json-schema "$(jq -c '.request.jsonSchema' "$req")" --output-format json --no-session-persistence)
  if [ $? -ne 0 ] || [ "$(jq -r '.structured_output | type' <<<"$out" 2>/dev/null)" != object ]; then
    echo "FAIL $PROVIDER/$model $(basename "$req"): $(jq -c '{is_error,subtype,result}' <<<"$out" 2>/dev/null | head -c 400)" >>"$LOG"
    return 0
  fi
  echo "$out" >"${req%.request.json}.claude-p.json"
  version=$(jq -r '.modelUsage | keys | join(",")' <<<"$out")
  [ -n "$version" ] || version=$model
  # callId: the CLI session id — one fresh session per call (PO-1: separate calls are checkable)
  jq -n --arg d "$(jq -r .promptDigest "$req")" --arg m "$model" --arg t "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg v "$version" --arg c "$(jq -r '.session_id // empty' <<<"$out")" --argjson o "$(jq -c .structured_output <<<"$out")" \
    '{format:"ri.dev/llm-exchange/response/v1",promptDigest:$d,model:$m,modelVersion:$v,generatedAt:$t,callId:$c,output:$o}' >"$resp.tmp" &&
    mv "$resp.tmp" "$resp"
  echo "OK $PROVIDER/$model $(basename "$req")" >>"$LOG"
}
export -f answer_one

n=$(find "$DIR" -maxdepth 1 -name '*.request.json' | wc -l | tr -d ' ')
find "$DIR" -maxdepth 1 -name '*.request.json' -print0 | xargs -0 -n1 -P "$PAR" bash -c 'answer_one "$0"'
ok=$(find "$DIR" -maxdepth 1 -name '*.response.json' | wc -l | tr -d ' ')
echo "semantic-exchange: $ok of $n requests answered in $DIR (log: $LOG)"
