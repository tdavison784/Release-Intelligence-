#!/usr/bin/env bash
# Ground-truth tooling (never read by a system under test).
# usage: observed/run.sh <case-dir> <kind-cluster-name> <charts-dir>
#   <charts-dir> holds traefik-39.0.5.tgz and traefik-39.0.9.tgz (helm pull traefik --repo https://traefik.github.io/charts)
#
# One throwaway kind cluster: Traefik chart 39.0.5 (v3.6.10) with the environment's HelmRelease values,
# the environment's edge middlewares and apps applied as Flux would (kustomize build | apply), probes of
# every route, then the upgrade to chart 39.0.9 (v3.6.15) with the same values and CRDs replaced (the
# HelmRelease's upgrade.crds: CreateReplace), and the same probes. A probe object that references a
# middleware of the traefik namespace directly (not part of the environment, deleted after use) shows
# the effective allowCrossNamespace setting at each version. A last phase runs v3.6.25 (the safer
# target of the version decision, NOTES.md) on the same chart.
set -euo pipefail
CASE="$(cd "$1" && pwd)"; CHARTS="$(cd "$3" && pwd)"
# shellcheck source=../../observed-lib.sh
. "$CASE/../observed-lib.sh"
ENV="$CASE/env"; OUT="$CASE/observed"; ID="$(basename "$CASE")"; SLUG="${ID##*--}"
exec > >(tee "$OUT/run.log") 2>&1
scratch
trap 'kind_down; rm -rf "${T:?}"' EXIT
echo "# $ID: Traefik v3.6.10 (chart 39.0.5) -> v3.6.15 (chart 39.0.9); kind $(kind version | cut -d' ' -f2), node ${KIND_NODE_IMAGE%@*}"

log "environment render FROM/TO (observed/render.diff)"
helmrelease_values "$ENV/infrastructure/traefik/helmrelease.yaml" "$T/values.yaml"
render_diff "$CHARTS/traefik-39.0.5.tgz" "$CHARTS/traefik-39.0.9.tgz" traefik traefik "$OUT/render.diff" -f "$T/values.yaml"

kind_up "$2"
curl_pod

traefik_ip() { kubectl -n traefik get svc traefik -o jsonpath='{.spec.clusterIP}'; }

# probe <host> [curl args...]: one HTTPS request through Traefik's service; status and edge headers
probe() {
  local host="$1"; shift
  printf '$ curl -k https://%s/ %s\n' "$host" "$*"
  kubectl -n default exec curl -- curl -sk --max-time 5 --resolve "$host:443:$(traefik_ip)" "$@" \
    -o /dev/null -D - "https://$host/" 2>&1 | grep -iE '^HTTP/|^x-edge|^strict-transport|^www-authenticate' | sed 's/^/  /' || echo "  (no response)"
}

probes() {
  log "probes ($1)"
  probe "shop.$SLUG.example.com"
  probe "backoffice.$SLUG.example.com"
  probe "backoffice.$SLUG.example.com" -u 'ops:correct-horse-ops'
  log "probe object: a direct cross-namespace middleware reference ($1)"
  cat <<EOF | tee "$T/xns-probe.yaml" | sed 's/^/  /'
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: xns-probe
  namespace: storefront
spec:
  entryPoints: [websecure]
  routes:
    - kind: Rule
      match: Host(\`xns-probe.$SLUG.example.com\`)
      middlewares:
        - name: security-headers
          namespace: traefik
      services:
        - name: storefront
          port: 80
  tls: {}
EOF
  run kubectl apply -f "$T/xns-probe.yaml"
  sleep 8
  probe "xns-probe.$SLUG.example.com"
  run kubectl delete -f "$T/xns-probe.yaml"
  log "traefik log: configuration source and errors ($1)"
  kubectl -n traefik logs -l app.kubernetes.io/name=traefik --tail=-1 --prefix 2>&1 |
    sed -E $'s/\x1b\\[[0-9;]*m//g' | grep -iE 'configuration loaded|ERR|error' |
    sed -E 's/^\[pod\/[^]]*\] //; s/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z //' | sort | uniq -c | sed 's/^/  /' || true
}

log "FROM: install chart 39.0.5 with the environment's values"
run helm install traefik "$CHARTS/traefik-39.0.5.tgz" -n traefik --create-namespace -f "$T/values.yaml"
run kubectl -n traefik rollout status deploy/traefik --timeout=180s
run kubectl -n traefik get deploy traefik -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
log "the Traefik container's args and env (rendered by the chart)"
kubectl -n traefik get deploy traefik -o json | python3 -c 'import json,sys; c=json.load(sys.stdin)["spec"]["template"]["spec"]["containers"][0]; print("  args:", *c["args"], sep="\n    "); print("  env:", *[e["name"]+"="+str(e.get("value","<from field>")) for e in c.get("env",[])], sep="\n    ")'

log "FROM: apply edge middlewares and apps (Flux Kustomizations, in dependsOn order)"
apply_kustomize "$ENV/infrastructure/edge-middlewares"
apply_kustomize "$ENV/apps"
run kubectl -n storefront rollout status deploy/storefront --timeout=120s
run kubectl -n backoffice rollout status deploy/backoffice --timeout=120s
sleep 10
probes "FROM v3.6.10"

log "UPGRADE: CRDs of chart 39.0.9 (CreateReplace), then helm upgrade with the same values"
mkdir -p "$T/to" && tar -xzf "$CHARTS/traefik-39.0.9.tgz" -C "$T/to"
run kubectl apply --server-side --force-conflicts -f "$T/to/traefik/crds/"
run helm upgrade traefik "$CHARTS/traefik-39.0.9.tgz" -n traefik -f "$T/values.yaml"
run kubectl -n traefik rollout status deploy/traefik --timeout=180s
run kubectl -n traefik get deploy traefik -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
sleep 15
probes "TO v3.6.15"

log "SAFER TARGET (version decision, not the proposed upgrade): v3.6.25, the latest v3.6 patch; no chart
### ships it as appVersion, so chart 39.0.9 with image.tag=v3.6.25"
run helm upgrade traefik "$CHARTS/traefik-39.0.9.tgz" -n traefik -f "$T/values.yaml" --set image.tag=v3.6.25
run kubectl -n traefik rollout status deploy/traefik --timeout=180s
run kubectl -n traefik get deploy traefik -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
sleep 15
probes "v3.6.25"

log "resources after the upgrades"
run kubectl get middlewares.traefik.io,ingressroutes.traefik.io -A
