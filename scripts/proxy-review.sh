#!/usr/bin/env bash
# Blind AI proxy review (lane `proxy`): answer `ri knowledge proxy-prompt` requests with ONE stateless
# `claude -p` call each and record every verdict with `ri knowledge decide -reviewer-kind proxy`.
#
#   scripts/proxy-review.sh <exchange-dir> <ledger.jsonl> [parallel] [knowledge-dir] [ri-binary]
#
# Per request <item>.request.json (the pattern of scripts/semantic-exchange.sh): system = request.system,
# one user message, the request's JSON schema as structured output, thinking off, no tools, no project
# settings (--safe-mode, neutral cwd), no session persistence — nothing shared between calls. The call is
# bracketed by wall-clock timestamps (the decision's StartedAt/DecidedAt); the envelope's session_id is the
# call id, its modelUsage keys the model version, total_cost_usd and duration_ms the cost and time. The full
# envelope is kept as <item>.claude-p.json for audit.
#
# Model calls run in parallel; recording is serialised (one `ri knowledge decide` at a time, so each
# decision sees the store as the previous one left it). One decision per item, never batched.
#
# Failures are recorded, never dropped, never retried blindly: a failed call writes <item>.failed and a
# ledger line (stage "call"); a refused verdict keeps its response and gets a ledger line from `ri` (stage
# "verdict"/"record"). Re-running answers only requests with neither a response nor a .failed marker.
# Create <exchange-dir>/STOP to stop launching new calls (in-flight ones finish and are recorded).
set -uo pipefail
DIR=$(cd "$1" && pwd) || { echo "usage: $0 <exchange-dir> <ledger.jsonl> [parallel] [knowledge-dir] [ri]" >&2; exit 2; }
LEDGER=$(cd "$(dirname "$2")" && pwd)/$(basename "$2")
PAR=${3:-4}
KDIR=$(cd "${4:-knowledge}" && pwd)
RI=$(cd "$(dirname "${5:-bin/ri}")" && pwd)/$(basename "${5:-bin/ri}")
[ -x "$RI" ] || { echo "ri binary $RI not found (go build -o bin/ri ./cmd/ri)" >&2; exit 2; }
LOG="$DIR/proxy-review.log"
LOCK="$DIR/.record.lock"
WORK=$(mktemp -d) # neutral cwd: no repo CLAUDE.md, no project settings
trap 'rm -rf "$WORK"' EXIT
export DIR LEDGER KDIR RI LOG LOCK WORK

now_iso() { python3 -c 'import datetime;print(datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="milliseconds").replace("+00:00","Z"))'; }
export -f now_iso

review_one() {
  req=$1
  item=$(basename "$req" .request.json)
  resp="$DIR/$item.response.json"
  [ -e "$resp" ] || [ -e "$DIR/$item.failed" ] || [ -e "$DIR/$item.skipped" ] && return 0
  [ -e "$DIR/STOP" ] && return 0
  # an item decided since its prompt was built (by a human, or as a side effect) is not called for
  rel=$(jq -r '.release // ""' "$req"); [ -n "$rel" ] || rel=_endpoint
  st=$(jq -r '.reviewItem.status // "missing"' "$KDIR/$(jq -r .product "$req")/$rel/reviews/$item.json" 2>/dev/null || echo missing)
  if [ "$st" != pending ]; then
    echo "$st" >"$DIR/$item.skipped"
    jq -nc --arg i "$item" --arg st "$st" --slurpfile r "$req" \
      '{itemId:$i, candidateId:$r[0].candidateId, product:$r[0].product, release:$r[0].release, questionType:$r[0].questionType,
        priority:$r[0].priority, outcome:"skipped", stage:"call", error:("item is " + $st + ", not pending; not called"),
        selfModel:false, selfFamily:false, humanDecisionsShown:0}' \
      | { while ! mkdir "$LOCK" 2>/dev/null; do sleep 0.2; done; cat >>"$LEDGER"; rmdir "$LOCK"; }
    echo "SKIP $item: $st" >>"$LOG"
    return 0
  fi
  model=$(jq -r '.request.model' "$req")
  start=$(now_iso)
  out=$(cd "$WORK" && jq -r '.request.messages[0].content' "$req" | MAX_THINKING_TOKENS=0 claude -p --safe-mode \
    --model "$model" --system-prompt "$(jq -r '.request.system' "$req")" --tools "" \
    --json-schema "$(jq -c '.request.jsonSchema' "$req")" --output-format json --no-session-persistence 2>>"$LOG")
  rc=$?
  end=$(now_iso)
  if [ $rc -ne 0 ] || [ "$(jq -r '.structured_output | type' <<<"$out" 2>/dev/null)" != object ]; then
    why=$(jq -c '{is_error,subtype,result}' <<<"$out" 2>/dev/null | head -c 600)
    [ -n "$why" ] || why="exit $rc: $(head -c 300 <<<"$out")"
    echo "$why" >"$DIR/$item.failed"
    jq -nc --arg i "$item" --arg e "$why" --arg s "$start" --arg d "$end" --slurpfile r "$req" \
      '{itemId:$i, candidateId:$r[0].candidateId, product:$r[0].product, release:$r[0].release, questionType:$r[0].questionType,
        priority:$r[0].priority, outcome:"failed", stage:"call", error:$e, proxyModel:$r[0].request.model,
        promptDigest:$r[0].promptDigest, startedAt:$s, decidedAt:$d, selfModel:false, selfFamily:false, humanDecisionsShown:0}' \
      | { while ! mkdir "$LOCK" 2>/dev/null; do sleep 0.2; done; cat >>"$LEDGER"; rmdir "$LOCK"; }
    echo "FAIL call $item: $why" >>"$LOG"
    return 0
  fi
  echo "$out" >"$DIR/$item.claude-p.json"
  # the served version: the modelUsage key of the requested model (else all keys)
  version=$(jq -r --arg m "$model" '[.modelUsage | keys[] | select(startswith($m))] | first // empty' <<<"$out")
  [ -n "$version" ] || version=$(jq -r '.modelUsage | keys | join(",")' <<<"$out")
  [ -n "$version" ] || version=$model
  jq -n --arg i "$item" --arg d "$(jq -r .promptDigest "$req")" --arg m "$model" --arg v "$version" \
    --arg c "$(jq -r '.session_id // empty' <<<"$out")" --arg s "$start" --arg e "$end" \
    --argjson ms "$(jq '.duration_ms // 0' <<<"$out")" --argjson usd "$(jq '.total_cost_usd // 0' <<<"$out")" \
    --argjson o "$(jq -c .structured_output <<<"$out")" \
    '{format:"ri.dev/proxy-review/response/v1",itemId:$i,promptDigest:$d,model:$m,modelVersion:$v,callId:$c,
      startedAt:$s,decidedAt:$e,durationMs:$ms,costUSD:$usd,output:$o}' >"$resp.tmp" && mv "$resp.tmp" "$resp"
  # record: serialised, one decision per item
  while ! mkdir "$LOCK" 2>/dev/null; do sleep 0.2; done
  rec=$("$RI" knowledge decide -dir "$KDIR" -reviewer-kind proxy -reviewer proxy -proxy-request "$req" \
    -proxy-response "$resp" -proxy-ledger "$LEDGER" "$item" 2>&1)
  rrc=$?
  rmdir "$LOCK"
  if [ $rrc -ne 0 ]; then echo "REFUSED $item: $(head -c 400 <<<"$rec")" >>"$LOG"; else echo "OK $item: $(head -1 <<<"$rec")" >>"$LOG"; fi
}
export -f review_one

n=$(find "$DIR" -maxdepth 1 -name '*.request.json' | wc -l | tr -d ' ')
find "$DIR" -maxdepth 1 -name '*.request.json' -print0 | sort -z | xargs -0 -n1 -P "$PAR" bash -c 'review_one "$0"'
ok=$(find "$DIR" -maxdepth 1 -name '*.response.json' | wc -l | tr -d ' ')
failed=$(find "$DIR" -maxdepth 1 -name '*.failed' | wc -l | tr -d ' ')
echo "proxy-review: $n requests, $ok answered, $failed calls failed (ledger: $LEDGER, log: $LOG)"
