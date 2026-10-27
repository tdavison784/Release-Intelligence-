#!/usr/bin/env bash
# Ground-truth tooling (never read by a system under test).
# usage: observed/run.sh <case-dir> <kind-cluster-name> <charts-dir>
#   <charts-dir> holds cert-manager-v1.20.4.tgz and cert-manager-v1.21.3.tgz
#   (helm pull cert-manager --repo https://charts.jetstack.io --version <v>)
#   cert-manager 1.21 supports Kubernetes >= 1.33: run with kind >= v0.31 on PATH and
#   KIND_NODE_IMAGE=kindest/node:v1.34.11@sha256:44e222ee2132dab25ff87301682f89eb82c7880ea3a1bf543bfe9708fd08d67d
#
# One throwaway kind cluster. A Vault dev server stands in for the security team's Vault (same mount
# paths, role, policy and audience as the environment describes; the role is bound to the
# ServiceAccount the ClusterIssuer names). cert-manager v1.20.4 is installed with the environment's
# values, the environment's PKI and apps are applied (server URL and CA bundle substituted for the
# stand-in Vault), the certificate is issued and renewed once; then cert-manager is upgraded to
# v1.21.3 with the same values and the certificate is renewed again.
set -euo pipefail
CASE="$(cd "$1" && pwd)"; CHARTS="$(cd "$3" && pwd)"
# shellcheck source=../../observed-lib.sh
. "$CASE/../observed-lib.sh"
ENV="$CASE/env"; OUT="$CASE/observed"; ID="$(basename "$CASE")"
exec > >(tee "$OUT/run.log") 2>&1
scratch
trap 'kind_down; rm -rf "${T:?}"' EXIT
VAULT_IMAGE=hashicorp/vault:1.20.4
SA="$(python3 -c 'import sys,yaml; d=yaml.safe_load(open(sys.argv[1])); print(d["spec"]["vault"]["auth"]["kubernetes"]["serviceAccountRef"]["name"])' "$ENV/platform/pki/vault-clusterissuer.yaml")"
echo "# $ID: cert-manager v1.20.4 -> v1.21.3; kind $(kind version | cut -d' ' -f2), node ${KIND_NODE_IMAGE%@*}; Vault stand-in $VAULT_IMAGE"
echo "# ClusterIssuer vault-pki authenticates as ServiceAccount cert-manager/$SA"

log "environment render FROM/TO (observed/render.diff)"
render_diff "$CHARTS/cert-manager-v1.20.4.tgz" "$CHARTS/cert-manager-v1.21.3.tgz" cert-manager cert-manager "$OUT/render.diff" \
  -f "$ENV/platform/cert-manager/values.yaml"

kind_up "$2"

vault() { kubectl -n vault exec deploy/vault -- env VAULT_ADDR=https://127.0.0.1:8200 VAULT_CACERT=/vault/tls/vault-ca.pem VAULT_TOKEN=root vault "$@"; }

log "stand-in Vault (dev mode, TLS) with the environment's mount, role, policy and audience"
cat > "$T/vault.yaml" <<'EOF'
apiVersion: v1
kind: Namespace
metadata: {name: vault}
---
apiVersion: v1
kind: ServiceAccount
metadata: {name: vault, namespace: vault}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: vault-auth-delegator}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: system:auth-delegator}
subjects: [{kind: ServiceAccount, name: vault, namespace: vault}]
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: vault, namespace: vault}
spec:
  selector: {matchLabels: {app: vault}}
  template:
    metadata: {labels: {app: vault}}
    spec:
      serviceAccountName: vault
      containers:
        - name: vault
          image: VAULT_IMAGE
          args: ["server", "-dev", "-dev-tls", "-dev-tls-cert-dir=/vault/tls", "-dev-root-token-id=root",
                 "-dev-listen-address=0.0.0.0:8200", "-dev-tls-san=vault.vault.svc"]
          env: [{name: SKIP_SETCAP, value: "true"}]
          ports: [{containerPort: 8200}]
          volumeMounts: [{name: tls, mountPath: /vault/tls}]
      volumes: [{name: tls, emptyDir: {}}]
---
apiVersion: v1
kind: Service
metadata: {name: vault, namespace: vault}
spec:
  selector: {app: vault}
  ports: [{port: 8200, targetPort: 8200}]
EOF
sed -i.bak "s#VAULT_IMAGE#$VAULT_IMAGE#" "$T/vault.yaml"
run kubectl apply -f "$T/vault.yaml"
run kubectl -n vault rollout status deploy/vault --timeout=180s
for _ in $(seq 1 30); do vault status >/dev/null 2>&1 && break; sleep 2; done
run vault secrets enable -path=pki_int pki
run vault secrets tune -max-lease-ttl=8760h pki_int
vault write -field=certificate pki_int/root/generate/internal common_name="Treasury Internal CA" ttl=8760h > /dev/null
echo "$ vault write pki_int/root/generate/internal common_name='Treasury Internal CA' ttl=8760h  (ok)"
run vault write pki_int/roles/treasury-internal allowed_domains=ledger.svc,ledger.svc.cluster.local allow_bare_domains=true allow_subdomains=true key_type=ec key_bits=256 max_ttl=720h
printf 'path "pki_int/sign/treasury-internal" { capabilities = ["create", "update"] }\n' |
  kubectl -n vault exec -i deploy/vault -- env VAULT_ADDR=https://127.0.0.1:8200 VAULT_CACERT=/vault/tls/vault-ca.pem VAULT_TOKEN=root vault policy write cert-manager-treasury -
run vault auth enable -path=kubernetes-treasury-prod kubernetes
run vault write auth/kubernetes-treasury-prod/config kubernetes_host=https://kubernetes.default.svc:443
run vault write auth/kubernetes-treasury-prod/role/cert-manager-treasury bound_service_account_names="$SA" \
  bound_service_account_namespaces=cert-manager audience=vault://vault-pki policies=cert-manager-treasury ttl=10m
kubectl -n vault exec deploy/vault -- cat /vault/tls/vault-ca.pem > "$T/vault-ca.pem"

log "FROM: install cert-manager v1.20.4 with the environment's values"
run helm install cert-manager "$CHARTS/cert-manager-v1.20.4.tgz" -n cert-manager --create-namespace \
  -f "$ENV/platform/cert-manager/values.yaml" --wait --timeout 300s
run kubectl -n cert-manager get role,rolebinding -o name

can_token() {
  printf '$ kubectl auth can-i create serviceaccounts/%s --subresource=token -n cert-manager --as=system:serviceaccount:cert-manager:cert-manager\n' "$SA"
  kubectl auth can-i create "serviceaccounts/$SA" --subresource=token -n cert-manager \
    --as=system:serviceaccount:cert-manager:cert-manager 2>&1 | sed 's/^/  /' || true
}

issuer_status() {
  log "status ($1)"
  run kubectl get clusterissuer vault-pki -o jsonpath='{range .status.conditions[*]}{.type}={.status} reason={.reason} message={.message}{"\n"}{end}'
  run kubectl -n ledger get certificate ledger-api-tls -o jsonpath='{range .status.conditions[*]}{.type}={.status} reason={.reason} message={.message}{"\n"}{end}revision={.status.revision} notAfter={.status.notAfter}{"\n"}'
  can_token
}

# renew_try <attempts>: what `cmctl renew` does, then waits up to <attempts>*2 s for the next revision
renew_try() {
  local ns=ledger c=ledger-api-tls rev now
  rev="$(kubectl -n "$ns" get certificate "$c" -o jsonpath='{.status.revision}')"
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf '$ renew %s/%s (revision %s -> %s)\n' "$ns" "$c" "$rev" "$((rev + 1))"
  kubectl -n "$ns" patch certificate "$c" --subresource=status --type=json -p \
    "[{\"op\":\"add\",\"path\":\"/status/conditions/-\",\"value\":{\"type\":\"Issuing\",\"status\":\"True\",\"reason\":\"ManuallyTriggered\",\"message\":\"Certificate re-issuance manually triggered\",\"lastTransitionTime\":\"$now\"}}]" >/dev/null
  for _ in $(seq 1 "$1"); do
    if [ "$(kubectl -n "$ns" get certificate "$c" -o jsonpath='{.status.revision}')" = "$((rev + 1))" ]; then
      echo "  revision $((rev + 1)) issued"; return 0
    fi
    sleep 2
  done
  echo "  revision $((rev + 1)) NOT issued after $(( $1 * 2 ))s"
  run kubectl -n "$ns" get certificaterequests -o 'custom-columns=NAME:.metadata.name,APPROVED:.status.conditions[?(@.type=="Approved")].status,READY:.status.conditions[?(@.type=="Ready")].status,REASON:.status.conditions[?(@.type=="Ready")].reason'
  local cr; cr="$(kubectl -n "$ns" get certificaterequests -o jsonpath='{.items[-1:].metadata.name}')"
  run kubectl -n "$ns" get certificaterequest "$cr" -o jsonpath='{range .status.conditions[*]}{.type}={.status} reason={.reason} message={.message}{"\n"}{end}'
  log "cert-manager controller log lines about the issuer"
  kubectl -n cert-manager logs -l app.kubernetes.io/component=controller --tail=-1 2>&1 |
    grep -iE 'token|forbidden|vault' | sed -E 's/^[IEW][0-9]{4} [0-9:.]+ +[0-9]+ //' | sort | uniq -c | sed 's/^/  /' | tail -15 || true
}

log "FROM: apply the PKI (server and CA bundle substituted for the stand-in Vault) and the apps"
kubectl kustomize "$ENV/platform/pki" > "$T/pki.yaml"
python3 - "$T/pki.yaml" "$T/vault-ca.pem" <<'EOF'
import sys, yaml
docs = [d for d in yaml.safe_load_all(open(sys.argv[1])) if d]
for d in docs:
    if d["kind"] == "ClusterIssuer":
        print(f'  substituted ClusterIssuer/{d["metadata"]["name"]} spec.vault.server: {d["spec"]["vault"]["server"]} -> https://vault.vault.svc:8200')
        d["spec"]["vault"]["server"] = "https://vault.vault.svc:8200"
    if d["kind"] == "Secret" and d["metadata"]["name"] == "vault-server-ca":
        print("  substituted Secret/cert-manager/vault-server-ca ca.crt -> the stand-in Vault's CA")
        d["stringData"]["ca.crt"] = open(sys.argv[2]).read()
yaml.safe_dump_all(docs, open(sys.argv[1], "w"), sort_keys=False)
EOF
run kubectl apply -f "$T/pki.yaml"
apply_kustomize "$ENV/apps/ledger/overlays/prod"
try kubectl -n ledger wait certificate/ledger-api-tls --for=condition=Ready --timeout=120s
if [ "$(kubectl -n ledger get certificate ledger-api-tls -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')" != True ]; then
  issuer_status "FROM, certificate not Ready"
  run kubectl -n ledger get certificaterequests -o yaml
  kubectl -n cert-manager logs -l app.kubernetes.io/component=controller --tail=80
  exit 1
fi
run kubectl -n ledger rollout status deploy/ledger-api --timeout=120s
issuer_status "FROM v1.20.4"
log "FROM: renewal"
renew_try 60

log "UPGRADE: helm upgrade to v1.21.3 with the same values"
run helm upgrade cert-manager "$CHARTS/cert-manager-v1.21.3.tgz" -n cert-manager \
  -f "$ENV/platform/cert-manager/values.yaml" --wait --timeout 300s
run kubectl -n cert-manager get deploy cert-manager -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
run kubectl -n cert-manager get role,rolebinding -o name
sleep 20
issuer_status "TO v1.21.3"
log "TO: renewal"
renew_try 45
issuer_status "TO v1.21.3, after the renewal attempt"
echo '$ (served certificate) kubectl -n ledger get secret ledger-api-tls -o jsonpath={.data.tls\\.crt} | base64 -d | openssl x509 -noout -serial -enddate'
kubectl -n ledger get secret ledger-api-tls -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -serial -enddate | sed 's/^/  /'
