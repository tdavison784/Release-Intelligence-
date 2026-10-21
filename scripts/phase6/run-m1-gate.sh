#!/usr/bin/env bash
# Milestone 1 regression comparison (docs/phase6/decision-intel GATE-M1 §2, ledger D-0002).
#
#   scripts/phase6/run-m1-gate.sh <scratch-dir> [repeats=3]
#
# Runs prompt investigate-agent/v9 on the cert-manager pair, <repeats> times, on
# knowledge snapshot K0 (as B0-K0-20261017-edb21ed9-v8), offline, against an
# APFS clone of the shared .ri cache (read only), and saves each run under
# eval/investigations/results/. Run it under nohup: it outlives any monitor.
set -euo pipefail
[ $# -ge 1 ] || { echo "usage: $0 <scratch-dir> [repeats]" >&2; exit 2; }
scratch=$1
repeats=${2:-3}
repo=$(git rev-parse --show-toplevel)
shared=/Users/tommydavison/repos/Release-Intelligence-/.ri
cd "$repo"
mkdir -p "$scratch"

go build -o bin/ri ./cmd/ri
[ -d "$scratch/K0" ] || scripts/phase5/snapshot.sh 03b6ff43 "$scratch/K0"
[ -d "$scratch/state" ] || cp -c -R "$shared" "$scratch/state"

sha=$(git rev-parse --short=8 HEAD)
day=$(date -u +%Y%m%d)
for r in $(seq 1 "$repeats"); do
  id="C1-K0-$day-$sha-v9-r$r"
  [ -e "eval/investigations/results/$id" ] && { echo "skip $id (exists)"; continue; }
  echo "=== $id $(date -u +%H:%M:%SZ)"
  bin/ri -state "$scratch/state" -offline eval -investigations -arm C1 -run-id "$id" \
    -case cert-manager-1.17-1.18--payments-eu,cert-manager-1.17-1.18--payments-us \
    -knowledge "$scratch/K0" -work "$scratch/work-r$r" \
    -agent -agent-model claude-sonnet-5-5 -agent-prompt investigate-agent/v9 -save \
    >"$scratch/$id.stdout" 2>"$scratch/$id.stderr" || echo "exit $? for $id (gate checks exit non-zero; see stderr)"
  echo "=== done $id $(date -u +%H:%M:%SZ)"
done
echo "ALL DONE"
