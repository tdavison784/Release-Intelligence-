#!/usr/bin/env bash
# Observed upgrade cert-manager v1.17.2 -> v1.18.0 for this environment.
# usage: observed/run.sh <case-dir> <kind-cluster-name> <charts-dir>   (output: observed/run.log)
# Argo CD is not installed: its sync waves are reproduced in order with helm (each Application's
# chart, version and value file) and kubectl kustomize | kubectl apply (each path Application).
CASE="$(cd "$1" && pwd)"; CHARTS="$(cd "$3" && pwd)"
. "$CASE/../observed-lib.sh"
ENV="$CASE/env"
exec > >(tee "$CASE/observed/run.log") 2>&1
echo "# case $(basename "$CASE"); kind $(kind version); helm $(helm version --short); $(date -u +%Y-%m-%dT%H:%M:%SZ)"
kind_up "$2"

SECRETS=(cert-manager/build-root-ca registry/artifacts-tls builders/builder-tls)
checks() { mtls_check builders builder https://artifacts.registry.svc/; }

log "wave -2: kyverno chart 3.3.7, platform/kyverno/values.yaml"
run helm install kyverno "$CHARTS/kyverno-3.3.7.tgz" -n kyverno --create-namespace -f "$ENV/platform/kyverno/values.yaml" --wait --timeout 5m
log "wave -1: cert-manager chart v1.17.2, platform/cert-manager/values.yaml"
run helm install cert-manager "$CHARTS/cert-manager-v1.17.2.tgz" -n cert-manager --create-namespace -f "$ENV/platform/cert-manager/values.yaml" --wait --timeout 5m
log "wave 0: platform/policies"
apply_kustomize "$ENV/platform/policies"
run kubectl wait clusterpolicy --all --for=condition=Ready --timeout=120s
log "wave 1: platform/pki"
apply_kustomize "$ENV/platform/pki" cert-manager
wait_certs cert-manager
log "wave 2: apps/registry, apps/builders"
apply_kustomize "$ENV/apps/registry"
apply_kustomize "$ENV/apps/builders"
wait_certs registry; wait_certs builders
run kubectl -n registry rollout status deploy/artifacts --timeout=180s
run kubectl -n builders rollout status deploy/builder --timeout=180s
log "the root CA as declared in the repository vs as stored in the cluster"
run grep -A3 privateKey "$ENV/platform/pki/build-root-ca.yaml"
run kubectl -n cert-manager get certificate build-root-ca -o jsonpath='{.spec.privateKey}{"\n"}'
run kubectl get certificates -A -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,ISCA:.spec.isCA,ROTATION:.spec.privateKey.rotationPolicy

log "baseline on v1.17.2"
show_keys "${SECRETS[@]}"
checks

log "control on v1.17.2: scheduled renewal of the root CA, then of artifacts (restarts)"
renew cert-manager build-root-ca
renew registry artifacts-tls
restart registry artifacts
show_keys "${SECRETS[@]}"
checks

log "UPGRADE: helm upgrade to chart v1.18.0 with the same values"
echo "CertificateRequests before:"; for ns in cert-manager registry builders; do count_crs $ns; done
run helm upgrade cert-manager "$CHARTS/cert-manager-v1.18.0.tgz" -n cert-manager -f "$ENV/platform/cert-manager/values.yaml" --wait --timeout 5m
run kubectl -n cert-manager rollout status deploy/cert-manager --timeout=120s
sleep 30
echo "CertificateRequests after:"; for ns in cert-manager registry builders; do count_crs $ns; done
log "after upgrade, before any renewal"
show_keys "${SECRETS[@]}"
checks

log "on v1.18.0: scheduled renewal of the root CA"
renew cert-manager build-root-ca
show_keys "${SECRETS[@]}"
checks
log "on v1.18.0: scheduled renewal of artifacts (restarts); builder not renewed yet"
renew registry artifacts-tls
restart registry artifacts
show_keys "${SECRETS[@]}"
checks
log "on v1.18.0: builder renews"
renew builders builder-tls
wait_mount builders builder builder-tls
show_keys "${SECRETS[@]}"
checks
log "on v1.18.0: the root CA renews once more"
renew cert-manager build-root-ca
show_keys "${SECRETS[@]}"
log "done"
