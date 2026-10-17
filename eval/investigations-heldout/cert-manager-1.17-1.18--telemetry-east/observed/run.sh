#!/usr/bin/env bash
# Observed upgrade cert-manager v1.17.2 -> v1.18.0 for this environment (shared by the telemetry pair).
# usage: observed/run.sh <case-dir> <kind-cluster-name> <charts-dir>   (output: observed/run.log)
# Flux is not installed: its reconciliation is reproduced in dependency order (cert-manager -> pki ->
# telemetry, edge) with helm (HelmRelease spec.values) and kubectl kustomize | kubectl apply.
CASE="$(cd "$1" && pwd)"; CHARTS="$(cd "$3" && pwd)"
. "$CASE/../observed-lib.sh"
ENV="$CASE/env"; VALUES="$(mktemp -t values.XXXXXX)"
exec > >(tee "$CASE/observed/run.log") 2>&1
echo "# case $(basename "$CASE"); kind $(kind version); helm $(helm version --short); $(date -u +%Y-%m-%dT%H:%M:%SZ)"
kind_up "$2"

SECRETS=(cert-manager/telemetry-root-ca telemetry/ingest-tls edge/gateway-tls edge/rollup-tls)
checks() {
  mtls_check edge gateway https://ingest.telemetry.svc/
  mtls_check edge rollup https://ingest.telemetry.svc/
}

log "FROM: cert-manager chart v1.17.2 with the HelmRelease values"
helmrelease_values "$ENV/infrastructure/cert-manager/helmrelease.yaml" "$VALUES"
run helm install cert-manager "$CHARTS/cert-manager-v1.17.2.tgz" -n cert-manager --create-namespace -f "$VALUES" --wait --timeout 5m
log "Kustomization pki"
apply_kustomize "$ENV/infrastructure/pki"
wait_certs cert-manager
log "Kustomizations telemetry, edge"
apply_kustomize "$ENV/apps/telemetry"
apply_kustomize "$ENV/apps/edge"
wait_certs telemetry; wait_certs edge
for d in telemetry/ingest edge/gateway edge/rollup; do run kubectl -n "${d%/*}" rollout status "deploy/${d#*/}" --timeout=180s; done
run kubectl get certificates -A -o wide

log "baseline on v1.17.2"
show_keys "${SECRETS[@]}"
checks

log "control on v1.17.2: scheduled renewal of the root CA, then of ingest (and ingest restarts)"
renew cert-manager telemetry-root-ca
renew telemetry ingest-tls
restart telemetry ingest
show_keys "${SECRETS[@]}"
checks

log "UPGRADE: helm upgrade to chart v1.18.0 with the same values"
echo "CertificateRequests before:"; for ns in cert-manager telemetry edge; do count_crs $ns; done
run helm upgrade cert-manager "$CHARTS/cert-manager-v1.18.0.tgz" -n cert-manager -f "$VALUES" --wait --timeout 5m
run kubectl -n cert-manager rollout status deploy/cert-manager --timeout=120s
sleep 30
echo "CertificateRequests after (revisionHistoryLimit default 1):"; for ns in cert-manager telemetry edge; do count_crs $ns; done
run kubectl get certificates -A -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,READY:.status.conditions[0].status,REVISION:.status.revision,ROTATION:.spec.privateKey.rotationPolicy
log "after upgrade, before any renewal"
show_keys "${SECRETS[@]}"
checks

log "on v1.18.0: scheduled renewal of the root CA"
renew cert-manager telemetry-root-ca
show_keys "${SECRETS[@]}"
checks
log "on v1.18.0: scheduled renewal of ingest (and ingest restarts); gateway and rollup not renewed yet"
renew telemetry ingest-tls
restart telemetry ingest
show_keys "${SECRETS[@]}"
checks
run kubectl -n telemetry logs deploy/ingest --tail=5
log "on v1.18.0: gateway renews; rollup not yet"
renew edge gateway-tls
wait_mount edge gateway gateway-tls
show_keys "${SECRETS[@]}"
checks
log "on v1.18.0: rollup renews"
renew edge rollup-tls
wait_mount edge rollup rollup-tls
show_keys "${SECRETS[@]}"
checks
run kubectl -n cert-manager logs deploy/cert-manager --tail=400 --since=15m | grep -iE 'private key|rotation|keymanager' | tail -20 || true
rm -f "${VALUES:?}"
log "done"
