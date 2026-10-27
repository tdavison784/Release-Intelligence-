#!/usr/bin/env bash
# Milestone 2 strict-path check (docs/phase6/decision-intel GATE-M2 §2, ledger D-0010).
#
#   scripts/phase6/run-m2-upstream.sh <scratch-dir> <recorded-state> [repeats=3]
#
# <recorded-state> is the state recorded by record-m2-advisories.sh (advisories cached).
# 1. Copies it twice (APFS clones): state-abl (left as is: no upstream source fetches
#    cached) and state-live.
# 2. Live: prompt v11 on D-EU <repeats> times ONLINE against state-live, so
#    request_evidence can fetch upstream files at the tag; each run is saved.
#    GITHUB_TOKEN comes from `gh auth token`, only in this process's environment.
# 3. Ablation: the same <repeats> times OFFLINE against state-abl, so every
#    upstream fetch is an offline cache miss (behaviour unavailable); each run is saved.
# Run it under nohup.
set -euo pipefail
[ $# -ge 2 ] || { echo "usage: $0 <scratch-dir> <recorded-state> [repeats]" >&2; exit 2; }
scratch=$1
recorded=$2
repeats=${3:-3}
prompt=${P6_PROMPT:-investigate-agent/v11}
repo=$(git rev-parse --show-toplevel)
cd "$repo"
mkdir -p "$scratch"
export DO_NOT_TRACK=1
go build -o bin/ri ./cmd/ri
[ -d "$scratch/K0" ] || scripts/phase5/snapshot.sh 03b6ff43 "$scratch/K0"
[ -d "$scratch/state-abl" ] || cp -c -R "$recorded" "$scratch/state-abl"
[ -d "$scratch/state-live" ] || cp -c -R "$recorded" "$scratch/state-live"

sha=$(git rev-parse --short=8 HEAD)
day=$(date -u +%Y%m%d)
DEU=ingress-nginx-1.11-1.12--docs-eu
for r in $(seq 1 "$repeats"); do
  id="C1-K0-$day-$sha-${prompt##*/}-deu-r$r"
  echo "=== $id (online) $(date -u +%H:%M:%SZ)"
  GITHUB_TOKEN=$(gh auth token) bin/ri -state "$scratch/state-live" eval -investigations -dir eval/investigations-heldout \
    -arm C1 -run-id "$id" -case "$DEU" -knowledge "$scratch/K0" -work "$scratch/work-$id" \
    -agent -agent-model claude-sonnet-5-5 -agent-prompt "$prompt" -save \
    >"$scratch/$id.stdout" 2>"$scratch/$id.stderr" || echo "exit $? for $id (gate checks exit non-zero; see stderr)"
done
for r in $(seq 1 "$repeats"); do
  id="C1-K0-$day-$sha-${prompt##*/}-deu-abl-r$r"
  echo "=== $id (offline, no upstream cache) $(date -u +%H:%M:%SZ)"
  bin/ri -state "$scratch/state-abl" -offline eval -investigations -dir eval/investigations-heldout \
    -arm C1 -run-id "$id" -case "$DEU" -knowledge "$scratch/K0" -work "$scratch/work-$id" \
    -agent -agent-model claude-sonnet-5-5 -agent-prompt "$prompt" -save \
    >"$scratch/$id.stdout" 2>"$scratch/$id.stderr" || echo "exit $? for $id (gate checks exit non-zero; see stderr)"
done
echo "ALL DONE"
