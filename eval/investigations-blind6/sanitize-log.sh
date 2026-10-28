#!/usr/bin/env bash
# Ground-truth tooling: replace machine-specific paths in observed logs (charts dir, scratch dir, checkout).
# usage: sanitize-log.sh <log>...
for f in "$@"; do
  sed -E -i '' \
    -e 's#/private/tmp/[^ ]*/charts#<charts>#g' \
    -e 's#/var/folders/[^ ]*/T/ri-gt6\.XXXXXX\.[A-Za-z0-9]+#<scratch>#g' \
    -e 's#/var/folders/[^ ]*/T/[a-z0-9-]+\.kubeconfig\.[A-Za-z0-9]+#<kubeconfig>#g' \
    -e 's#/Users/[^ ]*/eval/investigations-blind6#<blind6>#g' "$f"
done
