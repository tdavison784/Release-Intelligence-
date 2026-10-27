#!/usr/bin/env bash
# Milestone 4 demo, part A (docs/phase6/decision-intel GATE-M4 §1 steps 1-3, ledger D-0024).
#
#   scripts/phase6/run-m4-demo-a.sh <scratch-dir> <recorded-state>
#
# K0 snapshot; a v13 investigation of the P-US environment (online, from a clone
# of the M2 recorded advisory state) with K0 knowledge; formalization of its
# assessments, exported into a working copy of K0 (K1); independent verification
# of the exported hypotheses with runtime exercises (kind). Stops before
# promotion: knowledge promotion is the product owner's (PD-1). Run under nohup.
set -euo pipefail
[ $# -ge 2 ] || { echo "usage: $0 <scratch-dir> <recorded-state>" >&2; exit 2; }
S=$1; recorded=$2
repo=$(git rev-parse --show-toplevel); cd "$repo"; mkdir -p "$S"
export DO_NOT_TRACK=1
go build -o bin/ri ./cmd/ri
[ -d "$S/K0" ] || scripts/phase5/snapshot.sh 03b6ff43 "$S/K0"
[ -d "$S/K1" ] || cp -R "$S/K0" "$S/K1"
[ -d "$S/state" ] || cp -c -R "$recorded" "$S/state"
CASE=eval/investigations/cert-manager-1.17-1.18--payments-us
echo "=== investigate $(date -u +%H:%M:%SZ)"
GITHUB_TOKEN=$(gh auth token) bin/ri -state "$S/state" investigate cert-manager v1.17.2 v1.18.0 \
  -repo "$CASE/env" -env-name payments-prod-us -kubernetes 1.31 -knowledge "$S/K0" \
  -agent -agent-model claude-sonnet-5-5 -o json > "$S/inv-us.json" 2> "$S/inv-us.stderr" || echo "investigate exit $?"
inv=$(python3 -c "import json,sys;d=json.load(open('$S/inv-us.json'));print(d.get('id') or d.get('investigation',{}).get('id'))")
echo "inv=$inv"
echo "=== formalize $(date -u +%H:%M:%SZ)"
bin/ri -state "$S/state" investigate formalize "$inv" -knowledge "$S/K1" > "$S/formalize.out" 2> "$S/formalize.stderr" || echo "formalize exit $?"
echo "=== verify $(date -u +%H:%M:%SZ)"
GITHUB_TOKEN=$(gh auth token) bin/ri -state "$S/state" verify hypotheses -knowledge "$S/K1" -runtime > "$S/verify.out" 2> "$S/verify.stderr" || echo "verify exit $?"
kind get clusters 2>/dev/null | sed 's/^/kind cluster left: /'
echo "PART A DONE"
