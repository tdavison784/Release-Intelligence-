#!/usr/bin/env bash
# Milestone 2 advisory recording and gate runs (docs/phase6/decision-intel GATE-M2 §1, ledger D-0010).
#
#   scripts/phase6/record-m2-advisories.sh <scratch-dir> [base-commit]
#
# 1. Clones the shared .ri cache (read only) into <scratch>/state and runs the
#    deterministic arm (no agent) ONLINE once on the three development cases,
#    so the state records GitHub's global and repository advisory responses.
#    GITHUB_TOKEN comes from `gh auth token`, only in this process's environment.
# 2. Replays the same cases OFFLINE from that state with this commit's binary
#    and saves the runs (A0-K0-<day>-<sha>-adv-*). Those are the stored runs that
#    carry advisory data.
# 3. Replays them OFFLINE with the base commit's binary (default: the parent
#    before the advisories merge) into <scratch>, unsaved, so classification can
#    be compared old vs new on identical bytes.
# Run it under nohup.
set -euo pipefail
[ $# -ge 1 ] || { echo "usage: $0 <scratch-dir> [base-commit]" >&2; exit 2; }
scratch=$1
repo=$(git rev-parse --show-toplevel)
cd "$repo"
base=${2:-$(git log --merges --format=%H --grep='Merge lane advisories' -1)^1}
shared=/Users/tommydavison/repos/Release-Intelligence-/.ri
mkdir -p "$scratch"
export DO_NOT_TRACK=1

go build -o bin/ri ./cmd/ri
if [ ! -x "$scratch/ri-base" ]; then
  git worktree add --detach "$scratch/base-src" "$base" >/dev/null 2>&1
  (cd "$scratch/base-src" && go build -o "$scratch/ri-base" ./cmd/ri)
  git worktree remove --force "$scratch/base-src"
fi
[ -d "$scratch/K0" ] || scripts/phase5/snapshot.sh 03b6ff43 "$scratch/K0"
[ -d "$scratch/state" ] || cp -c -R "$shared" "$scratch/state"

sha=$(git rev-parse --short=8 HEAD)
day=$(date -u +%Y%m%d)
run() { # <binary> <mode: online|offline> <dir> <cases> <run-id> <save: yes|no>
  local bin=$1 mode=$2 dir=$3 cases=$4 id=$5 save=$6 flags=()
  [ "$mode" = offline ] && flags+=(-offline)
  local extra=(); [ "$save" = yes ] && extra+=(-save)
  echo "=== $id ($mode, $(basename "$bin")) $(date -u +%H:%M:%SZ)"
  if [ "$mode" = online ]; then
    GITHUB_TOKEN=$(gh auth token) "$bin" -state "$scratch/state" eval -investigations -dir "$dir" -arm A0 -run-id "$id" \
      -case "$cases" -knowledge "$scratch/K0" -work "$scratch/work-$id" ${extra[@]+"${extra[@]}"} \
      >"$scratch/$id.stdout" 2>"$scratch/$id.stderr" || echo "exit $? for $id (gate checks exit non-zero; see stderr)"
  else
    "$bin" -state "$scratch/state" ${flags[@]+"${flags[@]}"} eval -investigations -dir "$dir" -arm A0 -run-id "$id" \
      -case "$cases" -knowledge "$scratch/K0" -work "$scratch/work-$id" ${extra[@]+"${extra[@]}"} \
      >"$scratch/$id.stdout" 2>"$scratch/$id.stderr" || echo "exit $? for $id (gate checks exit non-zero; see stderr)"
  fi
}
DEU=ingress-nginx-1.11-1.12--docs-eu
CM=cert-manager-1.17-1.18--payments-eu,cert-manager-1.17-1.18--payments-us
run bin/ri online eval/investigations-heldout "$DEU" "rec-$day-deu" no
run bin/ri online eval/investigations "$CM" "rec-$day-cm" no
run bin/ri offline eval/investigations-heldout "$DEU" "A0-K0-$day-$sha-adv-deu" yes
run bin/ri offline eval/investigations "$CM" "A0-K0-$day-$sha-adv-cm" yes
run "$scratch/ri-base" offline eval/investigations-heldout "$DEU" "base-$day-deu" no
run "$scratch/ri-base" offline eval/investigations "$CM" "base-$day-cm" no
echo "ALL DONE"
