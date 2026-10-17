#!/usr/bin/env bash
# Build the PROTOCOL.md §5 S3 negative-control knowledge trees (Kbad-*) from K1.
#
#   scripts/phase5/kbad.sh <K1-dir> <promoted-fact-id> <out-dir> [mutations...]
#
# <K1-dir> is a snapshot (scripts/phase5/snapshot.sh <K1-sha> …), never the live
# knowledge/ tree, and is left untouched: each mutant is a copy at
# <out-dir>/Kbad-<x>-<name>. Mutations (default: all), each documented in
# scripts/phase5/kbad/main.go:
#   a proxy        verified by an AI proxy reviewer only, never human
#   b refuted      promotion refused by the gate: no fact, the review item stays open
#   c release      introduced by a release outside the transition
#   d pin          the pinned value in the applicability is wrong
#   e superseded   the fact is superseded
#   f retracted    the fact is retracted
#   g undecidable  the applicability also needs a condition no environment can decide
#   h contradict   a second ACTIVE fact for the same subject says the opposite change
# Mutants that change a fact's content are minted as well-formed record chains
# (so the loader accepts them); the pipeline, not the loader, must refuse to clear.
set -euo pipefail

if [ $# -lt 3 ]; then
  sed -n '2,/^set -e/p' "$0" | sed 's/^# \{0,1\}//; /^set -e/d' >&2
  exit 2
fi
k1=$1
fact=$2
out=$3
shift 3
muts=("$@")
[ ${#muts[@]} -gt 0 ] || muts=(a b c d e f g h)

repo=$(git rev-parse --show-toplevel)
[ -d "$k1" ] || { echo "no such snapshot: $k1" >&2; exit 1; }
live=$(cd "$repo/knowledge" && pwd -P)
[ "$(cd "$k1" && pwd -P)" != "$live" ] || { echo "refusing: $k1 is the live knowledge/ tree" >&2; exit 1; }

tool=$(mktemp -d)/kbad
(cd "$repo" && go build -o "$tool" ./scripts/phase5/kbad)

name() {
  case $1 in
    a) echo proxy;; b) echo refuted;; c) echo release;; d) echo pin;;
    e) echo superseded;; f) echo retracted;; g) echo undecidable;; h) echo contradict;;
    *) echo "unknown mutation $1" >&2; return 1;;
  esac
}

mkdir -p "$out"
for m in "${muts[@]}"; do
  dst="$out/Kbad-$m-$(name "$m")"
  [ ! -e "$dst" ] || { echo "$dst exists" >&2; exit 1; }
  cp -R "$k1" "$dst"
  "$tool" -dir "$dst" -fact "$fact" -mutation "$m"
done
rm -f "${tool:?}"
rmdir "$(dirname "${tool:?}")" 2>/dev/null || true
