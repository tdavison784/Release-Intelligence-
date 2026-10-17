#!/usr/bin/env bash
# Known-outcome run for the cert-manager 1.17 -> 1.18 mesh-CA cases (groundtruth lane).
#
# Applies the case's environment the way its Argo CD Applications would (Argo CD itself is not
# run: each Application is resolved by hand into the equivalent helm / kubectl kustomize command),
# upgrades cert-manager, then forces the renewals that would otherwise happen on schedule
# (root CA 720h before expiry, leaves 240h before expiry) and records what observably happens.
#
# usage: run.sh <case-dir> <kind-cluster-name> <dir-with-chart-tgzs>
#   charts: cert-manager-v1.17.2.tgz and cert-manager-v1.18.0.tgz from https://charts.jetstack.io
set -euo pipefail

CASE=$(cd "$1" && pwd)
NAME=$2
CHARTS=$(cd "$3" && pwd)
ENV=$CASE/env
OUT=$CASE/observed
CTX=kind-$NAME
LOG=$OUT/run.log
: > "$LOG"

k() { kubectl --context "$CTX" "$@"; }
log() { echo "### $(date -u +%H:%M:%SZ) $*" | tee -a "$LOG"; }
rec() { "$@" 2>&1 | tee -a "$LOG"; }

pubfp() { # sha256 of the public key in a Secret's tls.crt
  k get secret -n "$1" "$2" -o jsonpath='{.data.tls\.crt}' | base64 -d \
    | openssl x509 -pubkey -noout | openssl pkey -pubin -outform DER | openssl sha256 | awk '{print $NF}'
}
fingerprints() {
  log "fingerprints ($1)"
  for s in cert-manager/mesh-root-ca payments/ledger-tls payments/checkout-tls; do
    echo "$s pubkey-sha256=$(pubfp "${s%/*}" "${s#*/}") serial=$(k get secret -n "${s%/*}" "${s#*/}" -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -serial | cut -d= -f2)" | tee -a "$LOG"
  done
}
wait_ready() { k wait --for=condition=Ready --timeout=180s certificate -n "$1" "$2" >>"$LOG" 2>&1; }
renew() { # same status change `cmctl renew` makes: add Issuing=True (ManuallyTriggered)
  local ns=$1 name=$2 rev
  rev=$(k get certificate -n "$ns" "$name" -o jsonpath='{.status.revision}')
  k patch certificate -n "$ns" "$name" --subresource=status --type=json -p "[{\"op\":\"add\",\"path\":\"/status/conditions/-\",\"value\":{\"type\":\"Issuing\",\"status\":\"True\",\"reason\":\"ManuallyTriggered\",\"message\":\"Certificate re-issuance manually triggered\",\"lastTransitionTime\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}}]" >>"$LOG"
  for _ in $(seq 1 90); do
    local now
    now=$(k get certificate -n "$ns" "$name" -o jsonpath='{.status.revision}')
    if [ "${now:-0}" -gt "${rev:-0}" ]; then break; fi
    sleep 2
  done
  wait_ready "$ns" "$name"
  log "renewed $ns/$name: revision $rev -> $(k get certificate -n "$ns" "$name" -o jsonpath='{.status.revision}')"
}
call_ledger() { # checkout -> ledger over mutual TLS, exactly as the checkout container does
  # Connection-level failures right after a rollout (curl exit 7 = connect refused, 28 = timeout)
  # are retried; any TLS verdict (or success) is final and recorded as is.
  log "checkout -> ledger call ($1)"
  local rc
  set +e
  for attempt in 1 2 3 4 5 6 7 8 9 10; do
    k exec -n payments deploy/checkout -- curl -sS --max-time 5 --cacert /tls/ca.crt --cert /tls/tls.crt --key /tls/tls.key https://ledger.payments.svc/ >>"$LOG" 2>&1
    rc=$?
    echo "attempt=$attempt exit=$rc" | tee -a "$LOG"
    if [ "$rc" != 7 ] && [ "$rc" != 28 ]; then break; fi
    sleep 3
  done
  set -e
}
cross_verify() { # does each side's trust anchor (its own ca.crt) verify the other side's leaf?
  log "openssl cross-verification ($1)"
  local d; d=$(mktemp -d)
  for s in ledger-tls checkout-tls; do
    k get secret -n payments "$s" -o jsonpath='{.data.ca\.crt}' | base64 -d > "$d/$s.ca.crt"
    k get secret -n payments "$s" -o jsonpath='{.data.tls\.crt}' | base64 -d > "$d/$s.tls.crt"
  done
  set +e
  echo "ledger trusts checkout leaf:  $(openssl verify -CAfile "$d/ledger-tls.ca.crt" "$d/checkout-tls.tls.crt" 2>&1 | tr '\n' ' ')" | tee -a "$LOG"
  echo "checkout trusts ledger leaf:  $(openssl verify -CAfile "$d/checkout-tls.ca.crt" "$d/ledger-tls.tls.crt" 2>&1 | tr '\n' ' ')" | tee -a "$LOG"
  set -e
  rm -rf "$d"
}
crs() { log "CertificateRequests ($1)"; rec k get certificaterequests -A --no-headers -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,REV:.metadata.annotations.cert-manager\\.io/certificate-revision; }

log "case $(basename "$CASE"), cluster $NAME, $(kind version)"
kind create cluster --name "$NAME" --wait 180s >>"$LOG" 2>&1
rec k version

log "render-level diff of the cert-manager release (helm template v1.17.2 vs v1.18.0, same values)"
helm template cert-manager "$CHARTS/cert-manager-v1.17.2.tgz" -n cert-manager -f "$ENV/platform/cert-manager/values.yaml" > "$OUT/render-v1.17.2.yaml"
helm template cert-manager "$CHARTS/cert-manager-v1.18.0.tgz" -n cert-manager -f "$ENV/platform/cert-manager/values.yaml" > "$OUT/render-v1.18.0.yaml"
diff -u --label render-v1.17.2.yaml --label render-v1.18.0.yaml "$OUT/render-v1.17.2.yaml" "$OUT/render-v1.18.0.yaml" > "$OUT/render.diff" || true
rm "$OUT/render-v1.17.2.yaml" "$OUT/render-v1.18.0.yaml"
grep -n -E '^[-+].*(feature-gates|rotation|RotationPolicy)' "$OUT/render.diff" | tee -a "$LOG" || echo "(no feature-gate/rotation lines in the render diff)" | tee -a "$LOG"

log "phase 0: install cert-manager v1.17.2 (Application cert-manager) + platform/pki/overlays/prod + apps/payments/overlays/prod"
rec helm --kube-context "$CTX" upgrade --install cert-manager "$CHARTS/cert-manager-v1.17.2.tgz" -n cert-manager --create-namespace -f "$ENV/platform/cert-manager/values.yaml" --wait --timeout 5m
rec k apply -k "$ENV/platform/pki/overlays/prod"
wait_ready cert-manager mesh-root-ca
rec k apply -k "$ENV/apps/payments/overlays/prod"
wait_ready payments ledger-tls
wait_ready payments checkout-tls
rec k rollout status -n payments deploy/ledger --timeout=180s
rec k rollout status -n payments deploy/checkout --timeout=180s
rec k get certificate -A -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,READY:.status.conditions[0].status,ROTATION:.spec.privateKey.rotationPolicy
fingerprints "v1.17.2 initial issuance"
call_ledger "v1.17.2 initial"

log "phase 1 (control, still v1.17.2): re-issue the root CA"
renew cert-manager mesh-root-ca
fingerprints "v1.17.2 after root CA re-issuance"
crs "v1.17.2"

log "phase 2: upgrade cert-manager to v1.18.0 (Application targetRevision bump, same values)"
rec helm --kube-context "$CTX" upgrade cert-manager "$CHARTS/cert-manager-v1.18.0.tgz" -n cert-manager -f "$ENV/platform/cert-manager/values.yaml" --wait --timeout 5m
rec k get deploy -n cert-manager -o custom-columns=NAME:.metadata.name,IMAGE:.spec.template.spec.containers[0].image
rec k get certificate -A -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,READY:.status.conditions[0].status,ROTATION:.spec.privateKey.rotationPolicy
call_ledger "v1.18.0 right after upgrade, nothing re-issued yet"

log "phase 3: root CA reaches its renewal window (forced) on v1.18.0"
renew cert-manager mesh-root-ca
fingerprints "v1.18.0 after root CA re-issuance"

log "phase 4: ledger's leaf reaches its renewal window (forced); ledger pods restart and load it"
renew payments ledger-tls
rec k rollout restart -n payments deploy/ledger
rec k rollout status -n payments deploy/ledger --timeout=180s
fingerprints "v1.18.0 after ledger leaf re-issuance"
cross_verify "ledger re-issued, checkout not yet"
call_ledger "ledger re-issued after CA re-issuance, checkout still on its earlier certificate"

log "phase 5: checkout's leaf is re-issued too and checkout restarts"
renew payments checkout-tls
rec k rollout restart -n payments deploy/checkout
rec k rollout status -n payments deploy/checkout --timeout=180s
cross_verify "both leaves re-issued"
call_ledger "both leaves re-issued"
crs "v1.18.0"

log "controller log lines mentioning private keys"
k logs -n cert-manager deploy/cert-manager --since=15m 2>&1 | grep -i -E 'private ?key|rotation' | tail -40 >> "$LOG" || true

log "done; deleting cluster"
kind delete cluster --name "$NAME" >>"$LOG" 2>&1
