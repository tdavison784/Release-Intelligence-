#!/usr/bin/env bash
# Stateless "raw" arm: answer every ri exchange request with ONE isolated `claude -p` call,
# mirroring how ri called GLM through the Messages API (system = request.system, one user
# message, JSON-schema output, thinking off, no tools, no shared context between prompts).
#
#   raw-arm.sh <model> <bench-dir>      e.g. raw-arm.sh claude-haiku-4-5 .ri/bench/claude-haiku-4-5-raw
#
# Runs T1 (report-4) and T2 (smoke), answers, ingests, then T3 (eval). Each answer is written
# exactly as returned; a refused answer is never retried (rejections are data). A call that
# fails outright (non-zero exit, no structured output) leaves the prompt pending and is logged.
set -uo pipefail
M=$1
ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
B=$(cd "$ROOT" && mkdir -p "$2" && cd "$2" && pwd)
WORK=$(mktemp -d)   # neutral cwd: no repo CLAUDE.md, no project settings

setup() {
  for s in r4-state smoke-state; do
    mkdir -p "$B/$s/llm-cache"
    [ -e "$B/$s/cache" ] || ln -s "$ROOT/internal/app/testdata/e2e/state/cache" "$B/$s/cache"
  done
  mkdir -p "$B/r4-exchange" "$B/smoke-exchange"
}

t1() {
  (cd "$ROOT/internal/app" && ../../bin/ri -products ../../products -offline -state "$B/r4-state" impact cert-manager v1.17.0 v1.18.0 \
    --kubernetes 1.28 --values testdata/e2e/env/cert-manager/values.yaml --manifests testdata/e2e/env/cert-manager/manifests \
    --crds testdata/e2e/env/cert-manager/crds --images testdata/e2e/env/cert-manager/images.txt \
    -enrich -model "$M" -enrich-max 20 -llm-exchange "$B/r4-exchange" -o json > "$B/r4.json" 2> "$B/r4.stderr")
}

t2() {
  (cd "$ROOT" && bin/ri -offline -state "$B/smoke-state" impact cert-manager v1.17.0 v1.18.0 --repo internal/env/testdata/customer-repo \
    --kubernetes 1.28 -enrich -model "$M" -enrich-max 30 -llm-exchange "$B/smoke-exchange" -o json > "$B/smoke.json" 2> "$B/smoke.stderr")
}

answer_dir() {
  local dir=$1 n=0 fail=0
  for req in "$dir"/*.request.json; do
    local resp="${req%.request.json}.response.json"
    [ -e "$resp" ] && continue
    if [ "$(jq '.request.messages | length' "$req")" != 1 ] || [ "$(jq -r '.request.messages[0].role' "$req")" != user ]; then
      echo "SKIP (not a single user message): $req" | tee -a "$B/raw-arm.log"; fail=$((fail+1)); continue
    fi
    local out
    out=$(cd "$WORK" && jq -r '.request.messages[0].content' "$req" | MAX_THINKING_TOKENS=0 claude -p --safe-mode \
      --model "$M" --system-prompt "$(jq -r '.request.system' "$req")" --tools "" \
      --json-schema "$(jq -c '.request.jsonSchema' "$req")" --output-format json --no-session-persistence 2>>"$B/raw-arm.log")
    if [ $? -ne 0 ] || [ "$(jq -r '.structured_output | type' <<<"$out" 2>/dev/null)" != object ]; then
      echo "FAIL $(basename "$req"): $(jq -c '{is_error,result,subtype}' <<<"$out" 2>/dev/null | head -c 300)" | tee -a "$B/raw-arm.log"
      fail=$((fail+1)); continue
    fi
    echo "$out" > "${req%.request.json}.claude-p.json"   # full CLI envelope, for audit (usage, model)
    jq -n --arg d "$(jq -r .promptDigest "$req")" --arg m "$M" --arg t "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      --arg v "$(jq -r '.modelUsage | keys | join(",")' <<<"$out")" --argjson o "$(jq -c .structured_output <<<"$out")" \
      '{format:"ri.dev/llm-exchange/response/v1",promptDigest:$d,model:$m,modelVersion:$v,generatedAt:$t,output:$o}' > "$resp"
    n=$((n+1))
  done
  echo "$(basename "$dir"): answered $n, failed $fail" | tee -a "$B/raw-arm.log"
}

setup
t1; answer_dir "$B/r4-exchange"; t1; grep -m1 '^impact enrich:' "$B/r4.stderr" | tee "$B/r4.summary"
t2; answer_dir "$B/smoke-exchange"; t2; grep -m1 '^impact enrich:' "$B/smoke.stderr" | tee "$B/smoke.summary"
(cd "$ROOT" && bin/ri eval cert-manager-1.17-1.18 -enriched -model "$M" -llm-cache "$B/r4-state/llm-cache" -o json > "$B/eval.json" 2> "$B/eval.stderr")
echo "RAW-ARM DONE $M"
rm -rf "$WORK"
