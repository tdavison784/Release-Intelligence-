#!/usr/bin/env bash
# fullrun.sh <outdir> <models> [par]
set -u
OUT=$1 M=$2 PAR=${3:-8}
mkdir -p "$OUT/reports"
EDGES="argo-cd:v2.14.5:v3.0.0 cert-manager:v1.16.0:v1.17.0 cert-manager:v1.17.0:v1.18.0 cilium:v1.15.6:v1.17.0 cilium:v1.16.1:v1.17.0 istio:1.23.4:1.24.0 karpenter:v0.37.8:v1.0.0 strimzi:0.45.0:0.46.0"
ri() { bin/ri -offline -state /Users/tommydavison/repos/Release-Intelligence-/.ri semantic propose "$1" "$2" "$3" -model "$M" -llm-exchange "$OUT/exchange" -llm-cache "$OUT/cache" -out "$OUT/knowledge" "${@:4}"; }
for e in $EDGES; do IFS=: read p f t <<<"$e"; ri $p $f $t > "$OUT/reports/$p-$f-$t.pass1.txt" 2>&1; done
echo "requests: $(ls $OUT/exchange/*.request.json | wc -l)"
scripts/semantic-exchange.sh "$OUT/exchange" "" "$PAR"
for e in $EDGES; do IFS=: read p f t <<<"$e"; ri $p $f $t > "$OUT/reports/$p-$f-$t.txt" 2>&1; ri $p $f $t -o json > "$OUT/reports/$p-$f-$t.json" 2>/dev/null; done
echo FULLRUN DONE
