#!/usr/bin/env bash
# Reproduce every report in this packet, byte for byte, fully offline.
#
# Usage (from the root of the release-intelligence repository, on the branch
# the packet was produced from):
#
#   go build -o bin/ri ./cmd/ri
#   docs/phase3/review-packet/reproduce.sh
#
# What it does:
#   1. assembles a scratch state directory that joins the repository's
#      recorded upstream fixtures (HTTP cache + git results, recorded from the
#      real upstreams) with the recorded model answers used by report 4;
#   2. re-runs the four report commands;
#   3. diffs each output against the copy shipped in this packet.
#
# The only expected diff is `generatedAt` inside report.json (it follows the
# wall clock of the run; everything else is byte-identical). The JSON diff
# below therefore compares with that one field removed.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
RI="$ROOT/bin/ri"
PKG=docs/phase3/review-packet
STATE=$(mktemp -d /tmp/ri-review-packet-state.XXXXXX)

ln -s "$ROOT/internal/app/testdata/e2e/state/cache" "$STATE/cache"
ln -s "$ROOT/internal/app/testdata/impact-llm-cache" "$STATE/llm-cache"

cmp_text() { # cmp_text <report-dir>
  if diff -u "$ROOT/$PKG/reports/$1/report.txt" "$ROOT/$PKG/reports/$1/report.txt.tmp"; then
    echo "OK   $1/report.txt"
  else
    echo "DIFF $1/report.txt"; FAIL=1
  fi
  rm "$ROOT/$PKG/reports/$1/report.txt.tmp"
}
cmp_json() { # cmp_json <report-dir> — ignore the wall-clock generatedAt
  local a b
  a=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); d.pop("generatedAt",None); print(json.dumps(d,indent=2,sort_keys=True))' "$ROOT/$PKG/reports/$1/report.json")
  b=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); d.pop("generatedAt",None); print(json.dumps(d,indent=2,sort_keys=True))' "$ROOT/$PKG/reports/$1/report.json.tmp")
  if [ "$a" = "$b" ]; then
    echo "OK   $1/report.json (modulo generatedAt)"
  else
    diff <(echo "$a") <(echo "$b") && true
    echo "DIFF $1/report.json"; FAIL=1
  fi
  rm "$ROOT/$PKG/reports/$1/report.json.tmp"
}

FAIL=0
FAIL=0
E1=$PKG/example-env/report1-env
E2=$PKG/example-env/report2-env
EC=$PKG/example-env/customer-repo

"$RI" -offline -state "$STATE" impact cert-manager v1.17.0 v1.18.0 \
  --kubernetes 1.28 --values "$E1/values.yaml" --manifests "$E1/manifests" \
  --crds "$E1/crds" --images "$E1/images.txt" -o text \
  > "$ROOT/$PKG/reports/report-1-compat-action-required/report.txt.tmp"
cmp_text report-1-compat-action-required
"$RI" -offline -state "$STATE" impact cert-manager v1.17.0 v1.18.0 \
  --kubernetes 1.28 --values "$E1/values.yaml" --manifests "$E1/manifests" \
  --crds "$E1/crds" --images "$E1/images.txt" -o json \
  > "$ROOT/$PKG/reports/report-1-compat-action-required/report.json.tmp"
cmp_json report-1-compat-action-required

"$RI" -offline -state "$STATE" impact cert-manager v1.17.0 v1.18.0 \
  --kubernetes 1.31 --values "$E2/values.yaml" --images "$E2/images.txt" -o text \
  > "$ROOT/$PKG/reports/report-2-values-default-informational/report.txt.tmp"
cmp_text report-2-values-default-informational
"$RI" -offline -state "$STATE" impact cert-manager v1.17.0 v1.18.0 \
  --kubernetes 1.31 --values "$E2/values.yaml" --images "$E2/images.txt" -o json \
  > "$ROOT/$PKG/reports/report-2-values-default-informational/report.json.tmp"
cmp_json report-2-values-default-informational

"$RI" -offline -state "$STATE" impact cert-manager v1.17.0 v1.18.0 \
  --repo "$EC" --kubernetes 1.28 -o text \
  > "$ROOT/$PKG/reports/report-3-repo-mode/report.txt.tmp"
cmp_text report-3-repo-mode
"$RI" -offline -state "$STATE" impact cert-manager v1.17.0 v1.18.0 \
  --repo "$EC" --kubernetes 1.28 -o json \
  > "$ROOT/$PKG/reports/report-3-repo-mode/report.json.tmp"
cmp_json report-3-repo-mode

# Report 4 runs from internal/app/ so the input paths match the recorded
# model prompts (the digest covers the evidence URIs, which embed the paths).
(cd "$ROOT/internal/app" && \
  "$RI" -products ../../products -offline -state "$STATE" \
    impact cert-manager v1.17.0 v1.18.0 \
    --kubernetes 1.28 \
    --values testdata/e2e/env/cert-manager/values.yaml \
    --manifests testdata/e2e/env/cert-manager/manifests \
    --crds testdata/e2e/env/cert-manager/crds \
    --images testdata/e2e/env/cert-manager/images.txt \
    -enrich -model glm-5.3-flash -enrich-max 20 -o text) \
  > "$ROOT/$PKG/reports/report-4-ai-enriched/report.txt.tmp"
if diff -u "$ROOT/$PKG/reports/report-4-ai-enriched/report.txt" "$ROOT/$PKG/reports/report-4-ai-enriched/report.txt.tmp"; then
  echo "OK   report-4-ai-enriched/report.txt"
else
  echo "DIFF report-4-ai-enriched/report.txt"; FAIL=1
fi
rm "$ROOT/$PKG/reports/report-4-ai-enriched/report.txt.tmp"


rm -rf "$STATE"
[ "$FAIL" = 0 ] && echo "all reports reproduce byte for byte" || { echo "DIFFERENCES FOUND"; exit 1; }
