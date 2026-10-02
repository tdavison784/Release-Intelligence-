#!/usr/bin/env bash
# samerun2.sh — finish samerun.sh at higher parallelism (idempotent: answered
# responses are kept, only still-pending requests are answered). Then ingest both
# call-sets into one store and emit the two reports.
set -u
D=/Users/tommydavison/repos/Release-Intelligence-/.ri/semantic-run-v2
OUT=$D/same-model
mkdir -p $OUT/reports
# make sure ex-2's requests exist (pass2 writes them if missing)
bin/ri -offline -state /Users/tommydavison/repos/Release-Intelligence-/.ri semantic propose strimzi 0.45.0 0.46.0 \
  -model claude-sonnet-5-5 -llm-exchange $OUT/ex-2 -llm-cache $OUT/cache-2 -out $OUT/knowledge > $OUT/reports/pass2.txt 2>&1
scripts/semantic-exchange.sh $OUT/ex-1 "" 8
scripts/semantic-exchange.sh $OUT/ex-2 "" 8
for i in 1 2; do
  bin/ri -offline -state /Users/tommydavison/repos/Release-Intelligence-/.ri semantic propose strimzi 0.45.0 0.46.0 \
    -model claude-sonnet-5-5 -llm-exchange $OUT/ex-$i -llm-cache $OUT/cache-$i -out $OUT/knowledge > $OUT/reports/report$i.txt 2>&1
done
S1=$(ls $OUT/ex-1/*.response.json | wc -l | tr -d ' '); S2=$(ls $OUT/ex-2/*.response.json | wc -l | tr -d ' ')
SHARED=$(comm -12 <(jq -r .callId $OUT/ex-1/*.response.json | sort) <(jq -r .callId $OUT/ex-2/*.response.json | sort) | wc -l | tr -d ' ')
echo "answered $S1 + $S2; shared call ids: $SHARED (must be 0); reports:"; tail -4 $OUT/reports/report1.txt $OUT/reports/report2.txt
echo SAMERUN DONE
