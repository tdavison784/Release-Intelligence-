#!/usr/bin/env bash
# run4: the controlled with/without rendered-evidence measurement (README §Rendered evidence).
# The SAME 51 candidates that carried release-level rendered evidence in run3, re-proposed
# WITHOUT -render (plain semantic-full/v1 prompts), into a separate store (ids are member-derived).
# Only the 4 SUB edges have rendered candidates, so all four models answer.
# Deterministic pass 1 (this script's first loop) is offline; answering needs budget:
#   scripts/semantic-exchange.sh "$OUT/exchange-claude" "" 8 anthropic &
#   scripts/semantic-exchange.sh "$OUT/exchange-zai" "" 6 zai &   # needs the Z.AI key
# then re-run the propose loops (pass 2) and compare with run3's proposals on the same ids.
set -u
ROOT=$(cd "$(dirname "$0")/../../../../.." && pwd)
OUT=$1
ONLY=$ROOT/docs/phase3/learning-loop/runs/semantic-v3/run4-only
cd "$ROOT"
export GITHUB_TOKEN=${GITHUB_TOKEN:-$(gh auth token)}
STATE=/Users/tommydavison/repos/Release-Intelligence-/.ri
SUB="external-secrets:v0.15.0:v0.16.0 kyverno:v1.12.6:v1.13.0 prometheus-operator:v0.85.0:v0.86.2 traefik:v2.11.2:v3.0.0"
mkdir -p "$OUT/reports"
prop() { # prop <mode> <product> <from> <to> <outfile>
  local mode=$1 p=$2 f=$3 t=$4 o=$5
  case $mode in
  sh) args=(-model claude-sonnet-5-5,claude-haiku-4-5 -llm-exchange "$OUT/exchange-claude") ;;
  opus) args=(-model claude-opus-5-5 -llm-exchange "$OUT/exchange-claude") ;;
  glm) args=(-provider zai -model glm-5.3 -llm-exchange "$OUT/exchange-zai") ;;
  esac
  bin/ri -state $STATE semantic propose "$p" "$f" "$t" "${args[@]}" -only "$ONLY/$p.txt" \
    -llm-cache "$OUT/cache" -out "$OUT/knowledge" > "$o" 2>&1
}
for e in $SUB; do IFS=: read p f t <<<"$e"; prop sh $p $f $t "$OUT/reports/pass1-sh-$p.txt"; done
for e in $SUB; do IFS=: read p f t <<<"$e"; prop opus $p $f $t "$OUT/reports/pass1-opus-$p.txt"; prop glm $p $f $t "$OUT/reports/pass1-glm-$p.txt"; done
echo "requests claude $(ls $OUT/exchange-claude/*.request.json 2>/dev/null | wc -l) zai $(ls $OUT/exchange-zai/*.request.json 2>/dev/null | wc -l)"
echo "RUN4 PASS1 DONE — answer the exchanges, then re-run both loops (pass 2) to ingest"
