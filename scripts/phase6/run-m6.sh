#!/usr/bin/env bash
# Milestone 6 blind runs (docs/phase6/decision-intel GATE-M6, PLAN-M6 §3), from tag m6-freeze.
#
#   scripts/phase6/run-m6.sh <scratch-dir> <phase: A|B|CD> [repeats=3]
#
# A : deterministic RI, ONLINE once (records upstream data + advisories into <scratch>/state).
# B : Phase 5 agent (investigate-agent/v8), offline, on a clone of the recorded state.
# CD: consumer-aware agent (v13, the default) + upgrade synthesis, online (fetch-first), on its own clone.
# Knowledge: <scratch>/KF (frozen). Each run is saved under eval/investigations-blind6/results/. Run under nohup.
set -euo pipefail
S=$1; phase=$2; repeats=${3:-3}
repo=$(git rev-parse --show-toplevel); cd "$repo"
export DO_NOT_TRACK=1
[ -x bin/ri ] || go build -o bin/ri ./cmd/ri
DIR=eval/investigations-blind6
CASES=$(ls -d $DIR/*--*/ | xargs -n1 basename | paste -sd, -)
shared=/Users/tommydavison/repos/Release-Intelligence-/.ri
run() { # <state> <mode> <run-id> <args...>
  local st=$1 mode=$2 id=$3; shift 3
  local off=(); [ "$mode" = offline ] && off=(-offline)
  echo "=== $id ($mode) $(date -u +%H:%M:%SZ)"
  GITHUB_TOKEN=$(gh auth token) bin/ri -state "$st" ${off[@]+"${off[@]}"} eval -investigations -dir $DIR -run-id "$id" \
    -case "$CASES" -knowledge "$S/KF" -allow-live-knowledge -work "$S/work-$id" -save "$@" \
    >"$S/$id.stdout" 2>"$S/$id.stderr" || echo "exit $? for $id (gate checks exit non-zero; see stderr)"
  echo "=== done $id $(date -u +%H:%M:%SZ)"
}
case $phase in
  A)  [ -d "$S/state" ] || cp -c -R "$shared" "$S/state"
      run "$S/state" online M6-A -arm A0 ;;
  B)  [ -d "$S/state-B" ] || cp -c -R "$S/state" "$S/state-B"
      for r in $(seq 1 "$repeats"); do run "$S/state-B" offline "M6-B-r$r" -arm B0 -agent -agent-model claude-sonnet-5-5 -agent-prompt investigate-agent/v8; done ;;
  CD) [ -d "$S/state-CD" ] || cp -c -R "$S/state" "$S/state-CD"
      for r in $(seq 1 "$repeats"); do run "$S/state-CD" online "M6-CD-r$r" -arm C1 -agent -agent-model claude-sonnet-5-5 -agent-prompt investigate-agent/v13 -synthesize; done ;;
esac
echo "PHASE $phase DONE"
