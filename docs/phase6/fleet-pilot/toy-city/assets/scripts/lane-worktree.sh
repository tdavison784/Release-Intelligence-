#!/bin/sh
# pre_start for toy lanes: one git worktree per pool slot (detached at main; the lane creates
# lane/<bead> after claiming). SEALED.md is excluded by sparse checkout, so it is never on disk.
# gc creates the work dir (with .gc/) before pre_start, so existing entries are staged aside.
set -eu
exec 2>>"$HOME/fleetpilot/logs/lane-worktree.trace"; echo "--- $(date -u +%T) args: $* pwd: $(pwd) GIT_DIR=${GIT_DIR:-} GIT_WORK_TREE=${GIT_WORK_TREE:-}" >&2; set -x
RIG_ROOT="${1:?rig root}"; WT="${2:?work dir}"; AGENT="${3:?agent base}"
LOG="$HOME/fleetpilot/logs/lane-worktree.log"
say() { echo "$(date -u +%FT%TZ) $AGENT $*" >>"$LOG"; }
if git -C "$WT" rev-parse --is-inside-work-tree >/dev/null 2>&1 && [ -e "$WT/.git" ]; then
  say "reuse $WT ($(git -C "$WT" rev-parse --abbrev-ref HEAD))"; exit 0
fi
git -C "$RIG_ROOT" worktree prune
STAGE=""
if [ -d "$WT" ]; then STAGE="$WT.stage.$$"; mv "$WT" "$STAGE"; fi
mkdir -p "$(dirname "$WT")"
git -C "$RIG_ROOT" worktree add --no-checkout --detach "$WT" main >>"$LOG" 2>&1
git -C "$WT" sparse-checkout set --no-cone '/*' '!/SEALED.md' >>"$LOG" 2>&1
git -C "$WT" checkout --detach main >>"$LOG" 2>&1
if [ -n "$STAGE" ]; then
  for e in "$STAGE"/.[!.]* "$STAGE"/*; do [ -e "$e" ] && mv "$e" "$WT"/; done
  rmdir "$STAGE"
fi
say "created $WT (sealed absent: $([ -e "$WT/SEALED.md" ] && echo NO || echo yes))"
