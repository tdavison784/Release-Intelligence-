#!/usr/bin/env bash
# Re-materialise the raw sources RI's investigation had for a case: the
# upstream documents it fetched, the environment files it ingested and its
# environment-scope FROM/TO renders (PROTOCOL §3). Derived outputs are never
# copied.
#
#   scripts/phase6/materialise.sh <case-dir> [<arm-B bundle.json> [<extra CAS dir>...]]
#
# Runs `ri investigate` (deterministic, no agent, -offline) in a throwaway
# state dir cloned from the shared .ri cache, which is only read. With an
# arm-B bundle.json (from the stored run's cases.tar.gz), every file must have
# the digest that bundle records: bytes are taken from the fresh run's CAS or
# from the extra CAS dirs, and a source that cannot be found with that digest
# fails the run. Output: $P6_EVIDENCE/<case-id>/{files/,artifacts.tsv,renders.tsv,ORIGIN}.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
. "$here/lib.sh"

[ $# -ge 1 ] || { echo "usage: $0 <case-dir> [<arm-B bundle.json> [<extra CAS dir>...]]" >&2; exit 2; }
case_dir=$(cd "$1" && pwd); shift
armb=${1:-}; [ $# -gt 0 ] && shift
extra_cas=("$@")
repo=$(git -C "$here" rev-parse --show-toplevel)
ri=${RI_BIN:-$repo/bin/ri}
[ -x "$ri" ] || { echo "no ri binary at $ri (go build -o bin/ri ./cmd/ri, or set RI_BIN)" >&2; exit 1; }

input=$case_dir/input.yaml
id=$(p6_input "$input" id)
root=$(p6_input "$input" environment.root); root=${root:-env/}
out=$P6_EVIDENCE/$id
[ ! -e "$out" ] || { echo "$out exists; remove it first" >&2; exit 1; }

state=$(mktemp -d "${TMPDIR:-/tmp}/p6-state.XXXXXX")
trap 'rm -rf "${state:?}"' EXIT
for d in cache render store; do
  [ -d "$P6_SHARED_RI/$d" ] && cp -c -R "$P6_SHARED_RI/$d" "$state/" 2>/dev/null || cp -R "$P6_SHARED_RI/$d" "$state/"
done

# The same request the eval harness builds (internal/eval/investigations.go Request).
"$ri" -state "$state" -offline investigate \
  -repo "$case_dir/${root%/}" -env-name "$(p6_input "$input" environment.id)" \
  -kubernetes "$(p6_input "$input" environment.kubernetes)" -knowledge "$repo/knowledge" -o json \
  "$(p6_input "$input" product)" "$(p6_input "$input" from.version)" "$(p6_input "$input" to.version)" \
  >/dev/null 2>"$state/investigate.err" || { cat "$state/investigate.err" >&2; exit 1; }
bundles=("$state"/investigations/bundles/*.json)
[ ${#bundles[@]} -eq 1 ] || { echo "expected one bundle, found ${#bundles[@]}" >&2; exit 1; }
fresh=${bundles[0]}

# Raw artifacts: upstream and environment origin, and the environment-scope renders.
raw='.manifest.artifacts[] | select(.origin=="upstream" or .origin=="environment" or (.kind=="rendered-manifests" and (.path|startswith("derived/render/env/"))))'
want=$(mktemp "$state/want.XXXX")
if [ -n "$armb" ]; then
  jq -r "$raw | [.path,.digest,.kind,.origin,.uri] | @tsv" "$armb" >"$want"
  # A raw artifact of the fresh run that arm B did not have is reported, not copied.
  comm -13 <(cut -f1 "$want" | sort) <(jq -r "$raw | .path" "$fresh" | sort) | sed 's/^/not in arm-B bundle (skipped): /' >&2
else
  jq -r "$raw | [.path,.digest,.kind,.origin,.uri] | @tsv" "$fresh" >"$want"
fi

mkdir -p "$out/files"
printf 'path\tsha256\tkind\torigin\turi\tbytes-from\n' >"$out/artifacts.tsv"
missing=0
while IFS=$'\t' read -r path digest kind origin uri; do
  hex=${digest#sha256:}
  src=""; from=""
  cands=("$state/investigations/cas/sha256/${hex:0:2}/$hex")
  for x in ${extra_cas[@]+"${extra_cas[@]}"}; do cands+=("$x/$hex" "$x/sha256/${hex:0:2}/$hex"); done
  for c in "${cands[@]}"; do
    if [ -f "$c" ] && [ "$(p6_sha256 "$c")" = "$hex" ]; then src=$c; break; fi
  done
  case "$src" in
    "") echo "MISSING $path $digest" >&2; missing=$((missing+1)); continue ;;
    "$state"/*) from=fresh-run ;;
    *) from=extra-cas ;;
  esac
  mkdir -p "$out/files/$(dirname "$path")"
  cp "$src" "$out/files/$path"
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$path" "$hex" "$kind" "$origin" "$uri" "$from" >>"$out/artifacts.tsv"
done <"$want"

src_bundle=${armb:-$fresh}
jq -r '(.manifest.artifacts | map({(.id): .path}) | add) as $p
  | .renders[] | select(.scope=="environment")
  | [.target, .status, (if .from then $p[.from] else null end // ""), (if .to then $p[.to] else null end // "")] | @tsv' "$src_bundle" >"$out/renders.tsv"

{
  echo "case: $id"
  echo "arm-B bundle: ${armb:+$(jq -r .id "$armb") ($armb)}"
  echo "fresh bundle: $(jq -r .id "$fresh") (ri built from commit $(git -C "$repo" rev-parse HEAD))"
  echo "missing: $missing"
} >"$out/ORIGIN"
[ "$missing" -eq 0 ] || { echo "$missing source(s) not found with the recorded digest" >&2; exit 1; }
echo "$out"
