#!/usr/bin/env bash
# samerun.sh — PO-1 same-model consensus measurement: two INDEPENDENT sonnet call-sets
# over the same edge. Separate exchange dirs and caches per call-set, so each request is
# answered by its own `claude -p` session (distinct call ids), both writing into one
# knowledge store. Proposal ids differ by CallID, so the store sees two separate calls.
set -u
D=/Users/tommydavison/repos/Release-Intelligence-/.ri/semantic-run-v2
OUT=$D/same-model
mkdir -p $OUT/reports
ri() { bin/ri -offline -state /Users/tommydavison/repos/Release-Intelligence-/.ri semantic propose strimzi 0.45.0 0.46.0 -model claude-sonnet-5-5 "$@"; }
for i in 1 2; do
  ri -llm-exchange $OUT/ex-$i -llm-cache $OUT/cache-$i -out $OUT/knowledge > $OUT/reports/pass$i.txt 2>&1
done
echo "requests per call-set: $(ls $OUT/ex-1/*.request.json | wc -l) / $(ls $OUT/ex-2/*.request.json | wc -l)"
scripts/semantic-exchange.sh $OUT/ex-1 "" 4
scripts/semantic-exchange.sh $OUT/ex-2 "" 4
for i in 1 2; do
  ri -llm-exchange $OUT/ex-$i -llm-cache $OUT/cache-$i -out $OUT/knowledge > $OUT/reports/report$i.txt 2>&1
done
# the two call-sets must be separate calls: no shared session id
comm -12 <(jq -r .callId $OUT/ex-1/*.response.json | sort) <(jq -r .callId $OUT/ex-2/*.response.json | sort) | wc -l
echo SAMERUN DONE
