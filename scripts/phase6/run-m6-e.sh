#!/usr/bin/env bash
# Milestone 6 arm E (GATE-M6, PLAN-M6 §3): D repeat 1 (M6-CD2-r1) + formalize + verify -runtime.
#
#   scripts/phase6/run-m6-e.sh <scratch-dir> formalize|verify|rerun
#
# formalize: formalize/v2 on every investigation of M6-CD2-r1's eval workspace, exported into KE (a copy of KF).
# verify   : independent verification of KE's hypotheses with runtime exercises (kind, clusters deleted).
# rerun    : after the product owner's review of KE, deterministic arm A on the blind set with KE (M6-E-A).
# Promotion is the product owner's (PD-1); this script never promotes. Run under nohup.
set -euo pipefail
S=$1; step=$2
repo=$(git rev-parse --show-toplevel); cd "$repo"
export DO_NOT_TRACK=1
[ -x bin/ri ] || go build -o bin/ri ./cmd/ri
[ -d "$S/KE" ] || cp -R "$S/KF" "$S/KE"
[ -d "$S/state-E" ] || cp -c -R "$S/work-M6-CD2-r1" "$S/state-E"
case $step in
  formalize)
    for f in "$S"/state-E/investigations/runs/inv-*.json; do
      inv=$(basename "$f" .json)
      echo "=== formalize $inv $(date -u +%H:%M:%SZ)"
      bin/ri -state "$S/state-E" investigate formalize "$inv" -knowledge "$S/KE" >"$S/E-formalize-$inv.out" 2>"$S/E-formalize-$inv.stderr" || echo "formalize exit $? for $inv"
    done ;;
  verify)
    echo "=== verify $(date -u +%H:%M:%SZ)"
    GITHUB_TOKEN=$(gh auth token) bin/ri -state "$S/state-E" verify hypotheses -knowledge "$S/KE" -runtime >"$S/E-verify.out" 2>"$S/E-verify.stderr" || echo "verify exit $?"
    kind get clusters 2>/dev/null | sed 's/^/kind cluster left: /' ;;
  rerun)
    CASES=$(ls -d eval/investigations-blind6/*--*/ | xargs -n1 basename | paste -sd, -)
    GITHUB_TOKEN=$(gh auth token) bin/ri -state "$S/state" -offline eval -investigations -dir eval/investigations-blind6 -run-id M6-E-A \
      -case "$CASES" -knowledge "$S/KE" -allow-live-knowledge -work "$S/work-M6-E-A" -save -arm A0 >"$S/M6-E-A.stdout" 2>"$S/M6-E-A.stderr" || echo "eval exit $? (gate checks exit non-zero)" ;;
esac
echo "STEP $step DONE $(date -u +%H:%M:%SZ)"
