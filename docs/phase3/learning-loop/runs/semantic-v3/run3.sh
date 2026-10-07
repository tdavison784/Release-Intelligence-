#!/usr/bin/env bash
# semantic-3: 7 new environment-case edges. Sonnet+Haiku on all; Opus + GLM-5.3 (zai) on the
# smaller half by candidate count. -render where available (release-level chart defaults).
set -u
OUT=$1 KN=$2
export GITHUB_TOKEN=${GITHUB_TOKEN:-$(gh auth token)}
STATE=/Users/tommydavison/repos/Release-Intelligence-/.ri
ALL="crossplane:v1.20.1:v2.0.0 external-secrets:v0.15.0:v0.16.0 flux:v2.6.4:v2.7.0 kyverno:v1.12.6:v1.13.0 loki:v2.9.6:v3.0.0 prometheus-operator:v0.85.0:v0.86.2 traefik:v2.11.2:v3.0.0"
SUB="prometheus-operator:v0.85.0:v0.86.2 kyverno:v1.12.6:v1.13.0 external-secrets:v0.15.0:v0.16.0 flux:v2.6.4:v2.7.0"
mkdir -p "$OUT/reports"
prop() { # prop <mode> <product> <from> <to> <outfile> [extra]
  local mode=$1 p=$2 f=$3 t=$4 o=$5; shift 5
  case $mode in
  sh) args=(-model claude-sonnet-5-5,claude-haiku-4-5 -llm-exchange "$OUT/exchange-claude") ;;
  opus) args=(-model claude-opus-5-5 -llm-exchange "$OUT/exchange-claude") ;;
  glm) args=(-provider zai -model glm-5.3 -llm-exchange "$OUT/exchange-zai") ;;
  esac
  bin/ri -state $STATE semantic propose "$p" "$f" "$t" "${args[@]}" -render -llm-cache "$OUT/cache" -out "$KN" "$@" > "$o" 2>&1
}
for e in $ALL; do IFS=: read p f t <<<"$e"; prop sh $p $f $t "$OUT/reports/pass1-sh-$p.txt"; done
for e in $SUB; do IFS=: read p f t <<<"$e"; prop opus $p $f $t "$OUT/reports/pass1-opus-$p.txt"; prop glm $p $f $t "$OUT/reports/pass1-glm-$p.txt"; done
echo "requests claude $(ls $OUT/exchange-claude/*.request.json | wc -l) zai $(ls $OUT/exchange-zai/*.request.json | wc -l)"
scripts/semantic-exchange.sh "$OUT/exchange-claude" "" 8 anthropic &
scripts/semantic-exchange.sh "$OUT/exchange-zai" "" 6 zai &
wait
for e in $ALL; do IFS=: read p f t <<<"$e"; prop sh $p $f $t "$OUT/reports/sh-$p.txt"; done
for e in $SUB; do IFS=: read p f t <<<"$e"; prop opus $p $f $t "$OUT/reports/opus-$p.txt"; prop glm $p $f $t "$OUT/reports/glm-$p.txt"; done
echo RUN3 DONE
