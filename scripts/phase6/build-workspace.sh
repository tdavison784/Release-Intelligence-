#!/usr/bin/env bash
# Build a natural-agent investigation workspace for one case (PROTOCOL §3).
#
#   scripts/phase6/build-workspace.sh <case-dir> <out>
#
# <out> (outside the repository) gets:
#   env/                 the environment repository, as RI ingested it (see P6_ENV)
#   upstream/            the upstream documents, chart values and CRDs RI fetched, by source URL path
#   rendered/from.yaml   RI's environment-scope FROM renders, concatenated
#   rendered/to.yaml     the same for TO
#   rendered/manifest.diff  diff -u of the two
#   ORIENTATION.md       product, chart, versions, Kubernetes version, environment.description (input.yaml only)
#   scratch/             the only writable directory
#   MANIFEST.sha256      sha256 of every fixture file
# Sources come from $P6_EVIDENCE/<case-id> (materialise.sh). Only input.yaml
# and env/ of the case dir are read.
#
# P6_ENV=readback (default): env files and renders carry RI's bytes, with RI's
#   redaction markers verbatim; each line holding a marker that RI's read tools
#   read back (an enum value its CRD declares) gets a YAML comment with RI's
#   note, computed by RI's own code (readback/main.go). RI's agent got the same
#   value as a separate readBack field (decision D1, X-0002).
# P6_ENV=ri: RI's bytes only, no comments (less than RI's agent had).
# P6_ENV=raw: env files as authored in the case; renders as in ri.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
. "$here/lib.sh"

[ $# -eq 2 ] || { echo "usage: $0 <case-dir> <out>" >&2; exit 2; }
case_dir=$(cd "$1" && pwd)
out=$2
mode=${P6_ENV:-readback}
case "$mode" in readback|ri|raw) ;; *) echo "P6_ENV=$mode: readback, ri or raw" >&2; exit 2 ;; esac
input=$case_dir/input.yaml
id=$(p6_input "$input" id)
product=$(p6_input "$input" product)
root=$(p6_input "$input" environment.root); root=${root:-env/}
ev=$P6_EVIDENCE/$id
[ -f "$ev/artifacts.tsv" ] || { echo "no evidence for $id under $P6_EVIDENCE (run materialise.sh)" >&2; exit 1; }
[ ! -e "$out" ] || { echo "$out exists" >&2; exit 1; }
repo=$(git -C "$here" rev-parse --show-toplevel)
mkdir -p "$(dirname "$out")"
case "$(cd "$(dirname "$out")" && pwd)/" in "$repo"/*) echo "workspace must live outside the repository" >&2; exit 1 ;; esac
mkdir -p "$out"/{env,upstream,rendered,scratch}
out=$(cd "$out" && pwd)

# files/: RI's stored bytes, or (readback) RI's bytes with read-back comments.
files=$ev/files
if [ "$mode" = readback ]; then
  tmp=$(mktemp -d "${TMPDIR:-/tmp}/p6-readback.XXXXXX")
  trap 'rm -rf "${tmp:?}"' EXIT
  (cd "$repo" && go build -o "$tmp/readback" ./scripts/phase6/readback)
  { awk -F'\t' 'NR>1 && $4=="environment" {print $1}' "$ev/artifacts.tsv"
    cut -f3,4 "$ev/renders.tsv" | tr '\t' '\n' | sed '/^$/d'; } | sort -u >"$tmp/paths"
  set --; while read -r p; do set -- "$@" "$p"; done <"$tmp/paths"
  "$tmp/readback" -evidence "$ev" -out "$tmp/files" -tsv "$tmp/markers.tsv" "$@"
  files=$tmp/files
fi

# env/: every file of the case's environment root, either as RI stored it or
# decided in env-nonartifact.tsv.
(cd "$case_dir/${root%/}" && find . -type f | sed 's|^\./||' | sort) | while read -r rel; do
  stored=$files/env/$rel
  if [ -f "$stored" ]; then
    src=$stored; [ "$mode" = raw ] && src=$case_dir/${root%/}/$rel
  else
    decision=$(awk -F'\t' -v c="$id" -v p="$rel" '$1==c && $2==p {print $3}' "$here/env-nonartifact.tsv")
    case "$decision" in
      include) src=$case_dir/${root%/}/$rel ;;
      exclude) continue ;;
      *) echo "env/$rel: not an RI artifact and not decided in env-nonartifact.tsv" >&2; exit 1 ;;
    esac
  fi
  mkdir -p "$out/env/$(dirname "$rel")"
  cp "$src" "$out/env/$rel"
done

# upstream/: by source path (upstream/<product>/<host>/<path> in RI's bundle).
awk -F'\t' 'NR>1 && $4=="upstream" {print $1}' "$ev/artifacts.tsv" | while read -r p; do
  rel=${p#upstream/"$product"/}
  mkdir -p "$out/upstream/$(dirname "$rel")"
  cp "$ev/files/$p" "$out/upstream/$rel"
done

# rendered/: each environment render target in RI's order; a target RI found
# version-independent was rendered once and appears identically on both sides.
describe() { # RI render target -> a plain description
  case "$1" in
    argocd:*) f=${1#argocd:}; echo "helm template of the Argo CD Application ${f%%#*} (${f#*#})" ;;
    kustomize:*) echo "kustomize build ${1#kustomize:}" ;;
    *) echo "$1" ;;
  esac
}
: >"$out/rendered/from.yaml"; : >"$out/rendered/to.yaml"
while IFS=$'\t' read -r target status from to; do
  [ -n "$from" ] || { echo "render $target has no FROM side ($status)" >&2; exit 1; }
  if [ -n "$to" ]; then note=""; else to=$from; note=" (rendered once: identical in from.yaml and to.yaml)"; fi
  for side in from to; do
    f=$from; [ $side = to ] && f=$to
    { echo "# ===== $(describe "$target")$note ====="; cat "$files/$f"; echo; } >>"$out/rendered/$side.yaml"
  done
done <"$ev/renders.tsv"
(cd "$out" && diff -u --label rendered/from.yaml --label rendered/to.yaml rendered/from.yaml rendered/to.yaml >rendered/manifest.diff || [ $? -eq 1 ])

# ORIENTATION.md: input.yaml fields only, no case id, no scope.
python3 - "$input" >"$out/ORIENTATION.md" <<'EOF'
import sys, yaml
c = yaml.safe_load(open(sys.argv[1]))
def ver(s):
    v, cv = s.get("version", ""), s.get("chartVersion", "")
    return v if not cv or cv == v else f"{v} (Helm chart {cv})"
e = c["environment"]
print(f"""# Orientation

- Product: {c['product']}
- Helm chart: {c['chart']['name']} from {c['chart']['repo']}
- Current version: {ver(c['from'])}
- Candidate version: {ver(c['to'])}
- Kubernetes version: {e['kubernetes']}

## Environment

{e['description'].strip()}""")
EOF

(cd "$out" && find . -type f ! -path './scratch/*' ! -name MANIFEST.sha256 | sed 's|^\./||' | LC_ALL=C sort \
  | while read -r f; do shasum -a 256 "$f"; done) >"$out/MANIFEST.sha256"
chmod -R a-w "$out"
chmod u+w "$out/scratch"
echo "$out"
