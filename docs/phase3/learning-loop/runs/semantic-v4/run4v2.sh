#!/usr/bin/env bash
# semantic-4 step 3: v2 re-proposal of the property-selected candidates; Sonnet + Opus + GLM-5.3.
set -u
OUT=$1 KN=$2
export GITHUB_TOKEN=${GITHUB_TOKEN:-$(gh auth token)}
STATE=/Users/tommydavison/repos/Release-Intelligence-/.ri
mkdir -p "$OUT/reports"
prop() { # prop <claude|glm> <p> <f> <t> <outfile>
  local mode=$1 p=$2 f=$3 t=$4 o=$5
  case $mode in
  claude) args=(-model claude-sonnet-5-5,claude-opus-5-5 -llm-exchange "$OUT/exchange-claude") ;;
  glm) args=(-provider zai -model glm-5.3 -llm-exchange "$OUT/exchange-zai") ;;
  esac
  bin/ri -state $STATE semantic propose "$p" "$f" "$t" "${args[@]}" -only "$OUT/edge-only/${p}__${f}__${t}.txt" -render -llm-cache "$OUT/cache" -out "$KN" > "$o" 2>&1
}
while read p f t; do prop claude $p $f $t "$OUT/reports/pass1-claude-$p-$f.txt"; prop glm $p $f $t "$OUT/reports/pass1-glm-$p-$f.txt"; done < "$OUT/edges.txt"
echo "requests claude $(ls $OUT/exchange-claude/*.request.json | wc -l) zai $(ls $OUT/exchange-zai/*.request.json | wc -l)"
scripts/semantic-exchange.sh "$OUT/exchange-claude" "" 8 anthropic &
scripts/semantic-exchange.sh "$OUT/exchange-zai" "" 6 zai &
wait
while read p f t; do prop claude $p $f $t "$OUT/reports/claude-$p-$f.txt"; prop glm $p $f $t "$OUT/reports/glm-$p-$f.txt"; done < "$OUT/edges.txt"
echo RUN4V2 DONE
