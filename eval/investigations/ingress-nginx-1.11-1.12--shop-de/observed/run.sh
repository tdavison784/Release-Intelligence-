#!/usr/bin/env bash
# Known-outcome run for the ingress-nginx chart 4.11.5 -> 4.12.0 (controller 1.11.5 -> 1.12.0)
# shop cases (groundtruth lane).
#
# Applies the case's environment the way its Argo CD Applications would (Argo CD itself is not
# run: each Application is resolved by hand into the equivalent helm / kubectl kustomize command),
# upgrades the chart with the same value files, and records what observably happens to each Ingress.
# Deviation from the fixture, for kind only: the controller Service has no external load balancer
# here, so requests are sent from an in-cluster client pod to the controller's Service (ClusterIP);
# helm is not run with --wait (it would wait forever for a LoadBalancer address).
#
# usage: run.sh <case-dir> <kind-cluster-name> <dir-with-chart-tgzs>
#   charts: ingress-nginx-4.11.5.tgz and ingress-nginx-4.12.0.tgz from https://kubernetes.github.io/ingress-nginx
set -euo pipefail

CASE=$(cd "$1" && pwd)
NAME=$2
CHARTS=$(cd "$3" && pwd)
ENV=$CASE/env
OUT=$CASE/observed
CTX=kind-$NAME
LOG=$OUT/run.log
: > "$LOG"
VALUES=(-f "$ENV/platform/ingress-nginx/values.yaml" -f "$ENV/platform/ingress-nginx/values-prod.yaml")
SVC=http://ingress-nginx-controller.ingress-nginx.svc.cluster.local

k() { kubectl --context "$CTX" "$@"; }
log() { echo "### $(date -u +%H:%M:%SZ) $*" | tee -a "$LOG"; }
rec() { "$@" 2>&1 | tee -a "$LOG"; }

controller_ready() {
  k rollout status -n ingress-nginx deploy/ingress-nginx-controller --timeout=300s >>"$LOG" 2>&1
  k wait -n ingress-nginx --for=condition=Ready pod -l app.kubernetes.io/component=controller --timeout=180s >>"$LOG" 2>&1
}
probe() { # one request per host through the controller, as an external client would send it
  log "requests through the controller ($1)"
  for target in "shop.example.com/" "api.shop.example.com/v1"; do
    host=${target%%/*}; path=/${target#*/}
    set +e
    res=$(k exec -n default client -- curl -s -o /dev/null -D - --max-time 5 -H "Host: $host" "$SVC$path" 2>&1)
    set -e
    echo "$host$path -> $(echo "$res" | head -1 | tr -d '\r') | $(echo "$res" | grep -i -E '^(strict-transport-security|x-frame-options):' | tr -d '\r' | tr '\n' ' ')" | tee -a "$LOG"
  done
}
rendered_servers() {
  log "server blocks in the running nginx.conf ($1)"
  pod=$(k get pod -n ingress-nginx -l app.kubernetes.io/component=controller -o jsonpath='{.items[0].metadata.name}')
  k exec -n ingress-nginx "$pod" -- sh -c "grep -E '^\s*## start server|more_set_headers' /etc/nginx/nginx.conf" 2>&1 | tee -a "$LOG" || true
}

log "case $(basename "$CASE"), cluster $NAME, $(kind version)"
kind create cluster --name "$NAME" --wait 180s >>"$LOG" 2>&1
rec k version

log "render-level diff of the ingress-nginx release (helm template 4.11.5 vs 4.12.0, same value files)"
helm template ingress-nginx "$CHARTS/ingress-nginx-4.11.5.tgz" -n ingress-nginx "${VALUES[@]}" > "$OUT/render-4.11.5.yaml"
helm template ingress-nginx "$CHARTS/ingress-nginx-4.12.0.tgz" -n ingress-nginx "${VALUES[@]}" > "$OUT/render-4.12.0.yaml"
diff -u "$OUT/render-4.11.5.yaml" "$OUT/render-4.12.0.yaml" > "$OUT/render.diff" || true
rm "$OUT/render-4.11.5.yaml" "$OUT/render-4.12.0.yaml"
log "controller ConfigMap data as rendered by 4.12.0"
helm template ingress-nginx "$CHARTS/ingress-nginx-4.12.0.tgz" -n ingress-nginx "${VALUES[@]}" -s templates/controller-configmap.yaml | tee -a "$LOG"

log "phase 0: install ingress-nginx chart 4.11.5 (Application ingress-nginx) + apps/*/overlays/prod"
rec helm --kube-context "$CTX" upgrade --install ingress-nginx "$CHARTS/ingress-nginx-4.11.5.tgz" -n ingress-nginx --create-namespace "${VALUES[@]}" --timeout 5m
controller_ready
rec k get deploy -n ingress-nginx ingress-nginx-controller -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
rec k apply -k "$ENV/apps/storefront/overlays/prod"
rec k apply -k "$ENV/apps/catalog-api/overlays/prod"
rec k rollout status -n storefront deploy/storefront --timeout=180s
rec k rollout status -n catalog-api deploy/catalog-api --timeout=180s
rec k run client -n default --image=curlimages/curl:8.10.1 --restart=Never --command -- sleep 86400
k wait -n default --for=condition=Ready pod/client --timeout=180s >>"$LOG" 2>&1
sleep 10
probe "chart 4.11.5 / controller 1.11.5"
rendered_servers "1.11.5"

log "phase 1: upgrade to chart 4.12.0 (Application targetRevision bump, same value files)"
rec helm --kube-context "$CTX" upgrade ingress-nginx "$CHARTS/ingress-nginx-4.12.0.tgz" -n ingress-nginx "${VALUES[@]}" --timeout 5m
controller_ready
rec k get deploy -n ingress-nginx ingress-nginx-controller -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
rec k get configmap -n ingress-nginx ingress-nginx-controller -o jsonpath='{.data}{"\n"}'
sleep 15
probe "chart 4.12.0 / controller 1.12.0"
rendered_servers "1.12.0"

log "phase 2: Argo CD re-sync of the storefront Ingress after the upgrade (server-side apply, dry-run=server, goes through the admission webhook)"
set +e
k apply --server-side --force-conflicts --dry-run=server -k "$ENV/apps/storefront/overlays/prod" 2>&1 | tee -a "$LOG"
k apply --server-side --force-conflicts --dry-run=server -k "$ENV/apps/catalog-api/overlays/prod" 2>&1 | tee -a "$LOG"
set -e

log "controller log lines about the Ingresses"
k logs -n ingress-nginx deploy/ingress-nginx-controller --since=10m 2>&1 | grep -i -E 'risk|storefront|catalog-api|snippet|annotation' | tail -40 | tee -a "$LOG" || true

log "done; deleting cluster"
kind delete cluster --name "$NAME" >>"$LOG" 2>&1
