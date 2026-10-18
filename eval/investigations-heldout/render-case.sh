#!/usr/bin/env bash
# Ground-truth tooling: writes <case>/observed/render.diff, the environment's own FROM/TO render of
# the upgraded release (helm template with the environment's values, in the environment's order),
# canonicalised per object by render-diff.py. Kustomize sources are identical at FROM and TO and
# have no diff. usage: render-case.sh <case-dir> <charts-dir>
set -euo pipefail
CASE="$(cd "$1" && pwd)"; CHARTS="$(cd "$2" && pwd)"; HERE="$(cd "$(dirname "$0")" && pwd)"
ENV="$CASE/env"; T="$(mktemp -d -t render.XXXXXX)"
case "$(basename "$CASE")" in
  cert-manager-1.17-1.18--telemetry-*)
    python3 -c 'import sys,yaml; d=yaml.safe_load(open(sys.argv[1])); yaml.safe_dump(d["spec"]["values"], open(sys.argv[2],"w"))' \
      "$ENV/infrastructure/cert-manager/helmrelease.yaml" "$T/values.yaml"
    REL=cert-manager NS=cert-manager FROM=cert-manager-v1.17.2 TO=cert-manager-v1.18.0 ARGS=(-f "$T/values.yaml") ;;
  cert-manager-1.17-1.18--registry-apac|cert-manager-1.17-1.18--search-latam)
    REL=cert-manager NS=cert-manager FROM=cert-manager-v1.17.2 TO=cert-manager-v1.18.0 ARGS=(-f "$ENV/platform/cert-manager/values.yaml") ;;
  cert-manager-1.17-1.18--billing-nordics)
    REL=cert-manager NS=cert-manager FROM=cert-manager-v1.17.2 TO=cert-manager-v1.18.0 ARGS=(-f "$ENV/values/cert-manager.yaml" -f "$ENV/values/cert-manager-prod.yaml") ;;
  ingress-nginx-1.11-1.12--docs-*)
    REL=ingress-nginx NS=ingress-nginx FROM=ingress-nginx-4.11.5 TO=ingress-nginx-4.12.0 ARGS=(-f "$ENV/platform/ingress-nginx/values.yaml" -f "$ENV/platform/ingress-nginx/values-prod.yaml") ;;
  ingress-nginx-1.11-1.12--partner-api)
    REL=ingress-nginx NS=ingress-nginx FROM=ingress-nginx-4.11.5 TO=ingress-nginx-4.12.0 ARGS=(-f "$ENV/platform/ingress-nginx/values.yaml") ;;
  kube-prometheus-stack-89-90--obs-*)
    REL=kube-prometheus-stack NS=monitoring FROM=kube-prometheus-stack-89.2.4 TO=kube-prometheus-stack-90.2.0 ARGS=(-f "$ENV/infrastructure/monitoring/values.yaml") ;;
  *) echo "unknown case"; exit 1 ;;
esac
helm template "$REL" "$CHARTS/$FROM.tgz" -n "$NS" "${ARGS[@]}" > "$T/from.yaml"
helm template "$REL" "$CHARTS/$TO.tgz" -n "$NS" "${ARGS[@]}" > "$T/to.yaml"
python3 "$HERE/render-diff.py" "$T/from.yaml" "$T/to.yaml" "$CASE/observed/render.diff"
rm -rf "${T:?}"
