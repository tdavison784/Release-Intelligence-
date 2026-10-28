#!/usr/bin/env bash
# Milestone 3 gate runs (docs/phase6/decision-intel GATE-M3, ledger D-0018).
#
#   scripts/phase6/run-m3-gate.sh <scratch-dir> <recorded-state> [repeats=3]
#
# v13 investigation + synthesis (claude -p, default model) on the cert-manager pair
# and D-EU, ONLINE (upstream fetches allowed) against an APFS clone of the M2
# recorded advisory state, K0 knowledge. Each run is saved. GITHUB_TOKEN from
# `gh auth token`, in this process's environment only. Run it under nohup.
set -euo pipefail
[ $# -ge 2 ] || { echo "usage: $0 <scratch-dir> <recorded-state> [repeats]" >&2; exit 2; }
scratch=$1; recorded=$2; repeats=${3:-3}
repo=$(git rev-parse --show-toplevel); cd "$repo"; mkdir -p "$scratch"
export DO_NOT_TRACK=1
go build -o bin/ri ./cmd/ri
[ -d "$scratch/K0" ] || scripts/phase5/snapshot.sh 03b6ff43 "$scratch/K0"
[ -d "$scratch/state" ] || cp -c -R "$recorded" "$scratch/state"
sha=$(git rev-parse --short=8 HEAD); day=$(date -u +%Y%m%d)
run() { # <dir> <cases> <id>
  echo "=== $3 $(date -u +%H:%M:%SZ)"
  GITHUB_TOKEN=$(gh auth token) bin/ri -state "$scratch/state" eval -investigations -dir "$1" -arm D1 -run-id "$3" \
    -case "$2" -knowledge "$scratch/K0" -work "$scratch/work-$3" \
    -agent -agent-model claude-sonnet-5-5 -agent-prompt investigate-agent/v13 -synthesize -save \
    >"$scratch/$3.stdout" 2>"$scratch/$3.stderr" || echo "exit $? for $3 (gate checks exit non-zero; see stderr)"
}
for r in $(seq 1 "$repeats"); do
  run eval/investigations cert-manager-1.17-1.18--payments-eu,cert-manager-1.17-1.18--payments-us "D1-K0-$day-$sha-v13-synth-cm-r$r"
  run eval/investigations-heldout ingress-nginx-1.11-1.12--docs-eu "D1-K0-$day-$sha-v13-synth-deu-r$r"
done
echo "ALL DONE"
