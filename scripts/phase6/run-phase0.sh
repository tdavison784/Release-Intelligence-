#!/usr/bin/env bash
# Phase 0 driver (commander, X-0003): PROTOCOL §9 order — P-US, P-EU, D-EU; per case, repeats 1..3, each
# C-whole then C-item. Each run is extracted right after. A run that errors or hits the budget is kept and
# never retried (§8). Usage: scripts/phase6/run-phase0.sh [<case>...]   (default: P-US P-EU D-EU)
set -uo pipefail
cd "$(dirname "$0")/../.."
R=docs/phase6/natural-agent/runs
WS=${P6_HOME:-$HOME/ri-p6}/ws
wsdir() { case $1 in
  P-US) echo "$WS/cert-manager-1.17-1.18--payments-us" ;;
  P-EU) echo "$WS/cert-manager-1.17-1.18--payments-eu" ;;
  D-EU) echo "$WS/ingress-nginx-1.11-1.12--docs-eu" ;;
  *) echo "unknown case $1" >&2; exit 2 ;; esac; }
section() { case $1 in
  P-US|P-EU) echo docs/phase6/natural-agent/citem/cert-manager-rotationpolicy.txt ;;
  D-EU) echo docs/phase6/natural-agent/citem/ingress-nginx-11819.txt ;; esac; }
if [ $# -eq 0 ]; then set -- P-US P-EU D-EU; fi
for c in "$@"; do
  for r in 1 2 3; do
    for arm in C-whole C-item; do
      out=$R/$c/$arm-r$r
      if [ -e "$out/meta.json" ]; then echo "SKIP $c $arm r$r (exists)"; continue; fi
      if [ $arm = C-item ]; then
        scripts/phase6/run-natural.sh "$(wsdir $c)" C-item "$out" "$(section $c)"; rc=$?
      else
        scripts/phase6/run-natural.sh "$(wsdir $c)" C-whole "$out"; rc=$?
      fi
      st=$(python3 -c 'import json,sys;m=json.load(open(sys.argv[1]));print(m.get("status"),(m.get("tokens") or {}).get("total", m.get("tokens")) if isinstance(m.get("tokens"),dict) else m.get("tokens"),m.get("costUSD"),m.get("wallSeconds"),"s")' "$out/meta.json" 2>/dev/null)
      echo "RUN $c $arm r$r rc=$rc $st"
      if [ -s "$out/assessment.md" ]; then scripts/phase6/extract.sh "$out" >/dev/null 2>&1; echo "EXTRACT $c $arm r$r rc=$?"; else echo "EXTRACT $c $arm r$r skipped (no assessment)"; fi
    done
  done
done
echo "PHASE0 DONE $*"
