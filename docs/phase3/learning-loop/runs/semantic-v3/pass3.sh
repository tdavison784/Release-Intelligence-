#!/usr/bin/env bash
# semantic-3 pass 3: re-run the sonnet+haiku (all 7 edges) and opus (4 smaller edges) propose
# lines to ingest every exchange answer into the knowledge store. glm (zai) was fully answered
# and ingested in pass 2; its reports are final. Same arguments as run3.sh: pass3.sh <OUT> <KN>.
set -u
OUT=$1 KN=$2
export GITHUB_TOKEN=${GITHUB_TOKEN:-$(gh auth token)}
STATE=/Users/tommydavison/repos/Release-Intelligence-/.ri
ALL="crossplane:v1.20.1:v2.0.0 external-secrets:v0.15.0:v0.16.0 flux:v2.6.4:v2.7.0 kyverno:v1.12.6:v1.13.0 loki:v2.9.6:v3.0.0 prometheus-operator:v0.85.0:v0.86.2 traefik:v2.11.2:v3.0.0"
SUB="prometheus-operator:v0.85.0:v0.86.2 kyverno:v1.12.6:v1.13.0 external-secrets:v0.15.0:v0.16.0 flux:v2.6.4:v2.7.0"
prop() { # prop <mode> <product> <from> <to> <outfile>
  local mode=$1 p=$2 f=$3 t=$4 o=$5
  case $mode in
  sh) args=(-model claude-sonnet-5-5,claude-haiku-4-5 -llm-exchange "$OUT/exchange-claude") ;;
  opus) args=(-model claude-opus-5-5 -llm-exchange "$OUT/exchange-claude") ;;
  esac
  bin/ri -state $STATE semantic propose "$p" "$f" "$t" "${args[@]}" -render -llm-cache "$OUT/cache" -out "$KN" > "$o" 2>&1
}
for e in $ALL; do IFS=: read p f t <<<"$e"; prop sh $p $f $t "$OUT/reports/sh-$p.txt"; done
for e in $SUB; do IFS=: read p f t <<<"$e"; prop opus $p $f $t "$OUT/reports/opus-$p.txt"; done
echo PASS3 DONE
