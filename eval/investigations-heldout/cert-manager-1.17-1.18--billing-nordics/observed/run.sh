#!/usr/bin/env bash
# Observed upgrade cert-manager v1.17.2 -> v1.18.0 for this environment.
# usage: observed/run.sh <case-dir> <kind-cluster-name> <charts-dir>   (output: observed/run.log)
# The deploy pipeline is reproduced: helmfile's cert-manager release (helm with the same value files,
# in order), then `kubectl apply -k clusters/prod`. Substitution: external-secrets and the Vault it
# reads are not reproduced. The Secret they maintain (cert-manager/billing-ca-keypair, type
# kubernetes.io/tls) is created directly from a CA key pair generated for this run; cert-manager only
# ever reads that Secret.
CASE="$(cd "$1" && pwd)"; CHARTS="$(cd "$3" && pwd)"
. "$CASE/../observed-lib.sh"
ENV="$CASE/env"; TMP="$(mktemp -d -t billing-ca.XXXXXX)"
exec > >(tee "$CASE/observed/run.log") 2>&1
echo "# case $(basename "$CASE"); kind $(kind version); helm $(helm version --short); $(date -u +%Y-%m-%dT%H:%M:%SZ)"
kind_up "$2"

SECRETS=(cert-manager/billing-ca-keypair billing/invoicing-tls billing/dunning-tls billing/ledger-export-tls)
checks() {
  mtls_check billing dunning https://invoicing.billing.svc/
  mtls_check billing ledger-export https://invoicing.billing.svc/
}

log "FROM: helmfile release cert-manager, chart v1.17.2, values/cert-manager.yaml + values/cert-manager-prod.yaml"
run helm install cert-manager "$CHARTS/cert-manager-v1.17.2.tgz" -n cert-manager --create-namespace \
  -f "$ENV/values/cert-manager.yaml" -f "$ENV/values/cert-manager-prod.yaml" --wait --timeout 5m

log "stand-in for the external-secrets sync of cert-manager/billing-ca-keypair"
run openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-384 -nodes -days 3650 \
  -subj "/O=Example Billing/CN=billing-ca" -addext "basicConstraints=critical,CA:TRUE" \
  -addext "keyUsage=critical,keyCertSign,cRLSign" -keyout "$TMP/ca.key" -out "$TMP/ca.crt"
run kubectl -n cert-manager create secret tls billing-ca-keypair --cert "$TMP/ca.crt" --key "$TMP/ca.key"
rm -rf "${TMP:?}"

log "kubectl apply -k clusters/prod (without the external-secrets objects, see above)"
printf '$ kubectl kustomize clusters/prod | <drop ClusterSecretStore, ExternalSecret> | kubectl apply -f -\n'
kubectl kustomize "$ENV/clusters/prod" | python3 -c 'import sys, yaml
docs=[d for d in yaml.safe_load_all(sys.stdin) if d and not d["apiVersion"].startswith("external-secrets.io/")]
yaml.safe_dump_all(docs, sys.stdout, sort_keys=False)' | kubectl apply -f -
wait_certs billing
for d in invoicing dunning ledger-export; do run kubectl -n billing rollout status "deploy/$d" --timeout=180s; done
run kubectl get certificates -A -o wide

log "baseline on v1.17.2"
show_keys "${SECRETS[@]}"
checks

log "control on v1.17.2: scheduled renewal of invoicing (restarts) and of dunning"
renew billing invoicing-tls
restart billing invoicing
renew billing dunning-tls
wait_mount billing dunning dunning-tls
show_keys "${SECRETS[@]}"
checks

log "UPGRADE: helm upgrade to chart v1.18.0 with the same value files"
echo "CertificateRequests before:"; count_crs billing
run helm upgrade cert-manager "$CHARTS/cert-manager-v1.18.0.tgz" -n cert-manager \
  -f "$ENV/values/cert-manager.yaml" -f "$ENV/values/cert-manager-prod.yaml" --wait --timeout 5m
run kubectl -n cert-manager rollout status deploy/cert-manager --timeout=120s
sleep 30
echo "CertificateRequests after:"; count_crs billing
log "after upgrade, before any renewal"
show_keys "${SECRETS[@]}"
checks

log "on v1.18.0: scheduled renewal of invoicing (restarts); callers not renewed yet"
renew billing invoicing-tls
restart billing invoicing
show_keys "${SECRETS[@]}"
checks
log "on v1.18.0: dunning renews"
renew billing dunning-tls
wait_mount billing dunning dunning-tls
show_keys "${SECRETS[@]}"
checks
log "on v1.18.0: ledger-export renews"
renew billing ledger-export-tls
wait_mount billing ledger-export ledger-export-tls
show_keys "${SECRETS[@]}"
checks
echo "CertificateRequests at the end:"; count_crs billing
log "done"
