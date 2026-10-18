#!/usr/bin/env bash
# Observed upgrade cert-manager v1.17.2 -> v1.18.0 for this environment.
# usage: observed/run.sh <case-dir> <kind-cluster-name> <charts-dir>   (output: observed/run.log)
# Argo CD is not installed: its sync waves are reproduced in order with helm (the Application's
# chart, version and value file) and kubectl kustomize | kubectl apply (each path Application).
CASE="$(cd "$1" && pwd)"; CHARTS="$(cd "$3" && pwd)"
. "$CASE/../observed-lib.sh"
ENV="$CASE/env"
exec > >(tee "$CASE/observed/run.log") 2>&1
echo "# case $(basename "$CASE"); kind $(kind version); helm $(helm version --short); $(date -u +%Y-%m-%dT%H:%M:%SZ)"
kind_up "$2"

SECRETS=(cert-manager/search-root-ca search/search-api-tls search/indexer-tls search/query-frontend-tls)
checks() {
  mtls_check search indexer https://search-api.search.svc/
  mtls_check search query-frontend https://search-api.search.svc/
}

log "wave 0: cert-manager chart v1.17.2, platform/cert-manager/values.yaml"
run helm install cert-manager "$CHARTS/cert-manager-v1.17.2.tgz" -n cert-manager --create-namespace -f "$ENV/platform/cert-manager/values.yaml" --wait --timeout 5m
log "wave 1: platform/pki/overlays/prod"
run kubectl kustomize "$ENV/platform/pki/overlays/prod"
apply_kustomize "$ENV/platform/pki/overlays/prod" cert-manager
wait_certs cert-manager
log "wave 2: apps/search/overlays/prod"
apply_kustomize "$ENV/apps/search/overlays/prod"
wait_certs search
for d in search-api indexer query-frontend; do run kubectl -n search rollout status "deploy/$d" --timeout=180s; done
run kubectl get certificates -A -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,LABELS:.metadata.labels,ISCA:.spec.isCA,ROTATION:.spec.privateKey.rotationPolicy,HISTORY:.spec.revisionHistoryLimit

log "baseline on v1.17.2"
show_keys "${SECRETS[@]}"
checks

log "control on v1.17.2: scheduled renewal of the root CA, then of search-api (restarts)"
renew cert-manager search-root-ca
renew search search-api-tls
restart search search-api
show_keys "${SECRETS[@]}"
checks

log "UPGRADE: helm upgrade to chart v1.18.0 with the same values"
echo "CertificateRequests before:"; for ns in cert-manager search; do count_crs $ns; done
run helm upgrade cert-manager "$CHARTS/cert-manager-v1.18.0.tgz" -n cert-manager -f "$ENV/platform/cert-manager/values.yaml" --wait --timeout 5m
run kubectl -n cert-manager rollout status deploy/cert-manager --timeout=120s
sleep 30
echo "CertificateRequests after:"; for ns in cert-manager search; do count_crs $ns; done
log "after upgrade, before any renewal"
show_keys "${SECRETS[@]}"
checks

log "on v1.18.0: scheduled renewal of the root CA"
renew cert-manager search-root-ca
show_keys "${SECRETS[@]}"
checks
log "on v1.18.0: scheduled renewal of search-api (restarts); callers not renewed yet"
renew search search-api-tls
restart search search-api
show_keys "${SECRETS[@]}"
checks
log "on v1.18.0: indexer renews"
renew search indexer-tls
wait_mount search indexer indexer-tls
show_keys "${SECRETS[@]}"
checks
log "on v1.18.0: query-frontend renews"
renew search query-frontend-tls
wait_mount search query-frontend query-frontend-tls
show_keys "${SECRETS[@]}"
checks
echo "CertificateRequests at the end:"; for ns in cert-manager search; do count_crs $ns; done
log "done"
