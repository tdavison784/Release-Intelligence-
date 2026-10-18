#!/usr/bin/env bash
# Extract the knowledge/ tree of a git commit into a scratch directory, for
# `ri eval -investigations -knowledge <out>` (PROTOCOL.md §3: K0, K1).
#
#   scripts/phase5/snapshot.sh <sha> <out>
#
# <out> must not exist (or be empty). Prints the snapshot's content digest in
# the same form as run.json's knowledgeDigest. Never touches the working tree.
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: $0 <sha> <out>" >&2
  exit 2
fi
sha=$1
out=$2
repo=$(git rev-parse --show-toplevel)

git -C "$repo" rev-parse --verify --quiet "$sha^{commit}" >/dev/null || { echo "unknown commit: $sha" >&2; exit 1; }
if [ -e "$out" ] && [ -n "$(ls -A "$out" 2>/dev/null)" ]; then
  echo "$out exists and is not empty" >&2
  exit 1
fi
mkdir -p "$out"
# git archive of the subtree `knowledge`; --strip-components drops the leading dir.
git -C "$repo" archive "$sha" knowledge | tar -x -C "$out" --strip-components=1

full=$(git -C "$repo" rev-parse "$sha^{commit}")
# Same algorithm as knowledgeTreeDigest (cmd/ri/eval_investigations_knowledge.go):
# sha256 over, per file in sorted relative-path order, "<path>\0<len>\0<bytes>".
digest=$(cd "$out" && LC_ALL=C find . -type f | sed 's|^\./||' | LC_ALL=C sort | python3 -c '
import hashlib, sys
h = hashlib.sha256()
for rel in sys.stdin.read().split("\n"):
    if not rel:
        continue
    raw = open(rel, "rb").read()
    h.update(rel.encode() + b"\0" + str(len(raw)).encode() + b"\0")
    h.update(raw)
print("sha256:" + h.hexdigest())
')
echo "snapshot of $full knowledge/ -> $out"
echo "files:  $(find "$out" -type f | wc -l | tr -d ' ')"
echo "digest: $digest"
