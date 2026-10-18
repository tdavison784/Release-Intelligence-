#!/bin/bash
cd /Users/tommydavison/repos/Release-Intelligence-/.claude/worktrees/p6-integration
for r in 1 2 3; do
  out=docs/phase6/natural-agent/runs/P-US/C-raw-r$r
  [ -e $out/meta.json ] && { echo "SKIP C-raw r$r"; continue; }
  scripts/phase6/run-natural.sh ~/ri-p6/ws/cert-manager-1.17-1.18--payments-us.raw C-whole $out; rc=$?
  echo "RUN P-US C-raw r$r rc=$rc $(python3 -c 'import json,sys;m=json.load(open(sys.argv[1]));print(m["status"],m["tokens"]["processed"],m["costUSD"],m["assessmentWritten"])' $out/meta.json 2>/dev/null)"
  if [ -s $out/assessment.md ]; then scripts/phase6/extract.sh $out >/dev/null 2>&1; echo "EXTRACT P-US C-raw r$r rc=$?"; else echo "EXTRACT P-US C-raw r$r skipped (no assessment)"; fi
done
echo "CRAW DONE"
