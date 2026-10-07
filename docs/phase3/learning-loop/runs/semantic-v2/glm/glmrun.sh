#!/usr/bin/env bash
set -u
OUT=$1 KN=$2
mkdir -p "$OUT/reports"
EDGES="cert-manager:v1.16.0:v1.17.0 cert-manager:v1.17.0:v1.18.0 karpenter:v0.37.8:v1.0.0 strimzi:0.45.0:0.46.0"
ri() { bin/ri -offline -state /Users/tommydavison/repos/Release-Intelligence-/.ri semantic propose "$1" "$2" "$3" -provider zai -model glm-5.3 -llm-exchange "$OUT/exchange" -llm-cache "$OUT/cache" -out "$KN" "${@:4}"; }
for e in $EDGES; do IFS=: read p f t <<<"$e"; ri $p $f $t > /dev/null 2>&1; done
echo "requests: $(ls $OUT/exchange/*.request.json | wc -l)"
scripts/semantic-exchange.sh "$OUT/exchange" "" 6 zai
for e in $EDGES; do IFS=: read p f t <<<"$e"; ri $p $f $t > "$OUT/reports/glm-$p-$f-$t.txt" 2>&1; done
echo GLM DONE
