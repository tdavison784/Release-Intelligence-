# Shared helpers for the blind M6 observed runs (sourced by <case>/observed/run.sh). Copied from
# eval/investigations-heldout/observed-lib.sh, with helpers for the M6 products appended.
# Ground-truth tooling: never read by a system under test.
#
# Every run creates one throwaway kind cluster, installs the FROM release with the environment's own
# values, applies the environment's manifests in the order its GitOps controller would, records a
# baseline, upgrades to TO with the same values, exercises the changed behaviour and records the
# outcome. The cluster is always deleted (trap).

set -euo pipefail

KIND_NODE_IMAGE="${KIND_NODE_IMAGE:-kindest/node:v1.31.0@sha256:53df588e04085fd41ae12de0c3fe4c72f7013bba32a20e7325357a1ac94ba865}"

# log <text>: a section marker in run.log
log() { printf '\n### %s  [%s]\n' "$*" "$(date -u +%H:%M:%SZ)"; }

# run <cmd...>: echo the command, then run it (output goes to run.log)
run() { printf '$ %s\n' "$*"; "$@"; }

# try <cmd...>: like run, but a failure is recorded instead of aborting
try() { printf '$ %s\n' "$*"; if "$@"; then echo "(exit 0)"; else echo "(exit $?)"; fi; }

kind_up() { # kind_up <cluster>
  CLUSTER="$1"
  KUBECONFIG="$(mktemp -t "${CLUSTER}.kubeconfig.XXXXXX")"
  export KUBECONFIG CLUSTER
  trap kind_down EXIT
  run kind create cluster --name "$CLUSTER" --image "$KIND_NODE_IMAGE" --wait 120s --kubeconfig "$KUBECONFIG"
  run kubectl version
}

kind_down() {
  echo; echo "### cleanup  [$(date -u +%H:%M:%SZ)]"
  kind delete cluster --name "$CLUSTER" || true
  rm -f "${KUBECONFIG:?}"
}

# apply_kustomize <dir> [namespace]: what Argo CD / Flux does for a plain Kustomize source
apply_kustomize() {
  if [ -n "${2:-}" ]; then
    printf '$ kubectl kustomize %s | kubectl apply -n %s -f -\n' "$1" "$2"
    kubectl kustomize "$1" | kubectl apply -n "$2" -f -
  else
    printf '$ kubectl kustomize %s | kubectl apply -f -\n' "$1"
    kubectl kustomize "$1" | kubectl apply -f -
  fi
}

# wait_certs <namespace> [timeout]: every Certificate in the namespace Ready
wait_certs() { run kubectl -n "$1" wait certificate --all --for=condition=Ready --timeout="${2:-180s}"; }

# key_fp <namespace> <secret>: sha256 of the public key in the Secret's tls.crt
key_fp() {
  kubectl -n "$1" get secret "$2" -o jsonpath='{.data.tls\.crt}' | base64 -d |
    openssl x509 -noout -pubkey | openssl pkey -pubin -outform DER | openssl sha256 -r | cut -c1-16
}

# ca_fp <namespace> <secret>: sha256 of the public key of the Secret's ca.crt (first certificate)
ca_fp() {
  local pem; pem="$(kubectl -n "$1" get secret "$2" -o jsonpath='{.data.ca\.crt}' | base64 -d)"
  if [ -z "$pem" ]; then echo "(no ca.crt)"; return 0; fi
  printf '%s\n' "$pem" | openssl x509 -noout -pubkey | openssl pkey -pubin -outform DER | openssl sha256 -r | cut -c1-16
}

# show_keys <ns/secret>...: one line per Secret: leaf key, serial, issuer, CA key in ca.crt
show_keys() {
  local ref ns s
  printf '%-34s %-17s %-17s %s\n' SECRET TLS-KEY CA.CRT-KEY "SERIAL / ISSUER"
  for ref in "$@"; do
    ns="${ref%%/*}"; s="${ref#*/}"
    printf '%-34s %-17s %-17s %s\n' "$ref" "$(key_fp "$ns" "$s")" "$(ca_fp "$ns" "$s" 2>/dev/null || echo -)" \
      "$(kubectl -n "$ns" get secret "$s" -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -serial -issuer | tr '\n' ' ')"
  done
}

# renew <namespace> <certificate>: what `cmctl renew` does (adds the Issuing condition), then waits
# for the next revision to be Ready. Stands in for the scheduled renewal at renewBefore.
renew() {
  local ns="$1" c="$2" rev now
  rev="$(kubectl -n "$ns" get certificate "$c" -o jsonpath='{.status.revision}')"
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf '$ renew %s/%s (revision %s -> %s)\n' "$ns" "$c" "$rev" "$((rev + 1))"
  kubectl -n "$ns" patch certificate "$c" --subresource=status --type=json -p \
    "[{\"op\":\"add\",\"path\":\"/status/conditions/-\",\"value\":{\"type\":\"Issuing\",\"status\":\"True\",\"reason\":\"ManuallyTriggered\",\"message\":\"Certificate re-issuance manually triggered\",\"lastTransitionTime\":\"$now\"}}]" >/dev/null
  for _ in $(seq 1 90); do
    if [ "$(kubectl -n "$ns" get certificate "$c" -o jsonpath='{.status.revision}')" = "$((rev + 1))" ] &&
       [ "$(kubectl -n "$ns" get certificate "$c" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')" = "True" ]; then
      echo "  revision $((rev + 1)) Ready"; return 0
    fi
    sleep 2
  done
  echo "  TIMEOUT waiting for revision $((rev + 1))"; kubectl -n "$ns" describe certificate "$c" | tail -20
}

# count_crs <namespace>: CertificateRequests per Certificate
count_crs() {
  kubectl -n "$1" get certificaterequests -o jsonpath='{range .items[*]}{.metadata.annotations.cert-manager\.io/certificate-name}{"\n"}{end}' |
    sort | uniq -c | sed 's/^/  /'
}

# helmrelease_values <HelmRelease file> <out>: the inline spec.values of a Flux HelmRelease
helmrelease_values() {
  python3 -c 'import sys, yaml
for d in yaml.safe_load_all(open(sys.argv[1])):
    if d and d.get("kind") == "HelmRelease":
        yaml.safe_dump(d["spec"].get("values") or {}, sys.stdout, sort_keys=False)' "$1" > "$2"
  printf '# values of %s (spec.values):\n' "$1"; sed 's/^/  /' "$2"
}

# mtls_check <namespace> <deployment> <url>: one call from the workload's container with the
# workload's own certificate and CA bundle; prints the response or the TLS error. A second call with
# -k (server not verified) shows whether the server accepts the workload's client certificate.
mtls_check() {
  local ns="$1" d="$2" url="$3"
  printf '$ kubectl -n %s exec deploy/%s -- curl --cacert /tls/ca.crt --cert /tls/tls.crt --key /tls/tls.key %s\n' "$ns" "$d" "$url"
  if kubectl -n "$ns" exec "deploy/$d" -- curl -sS --max-time 5 --cacert /tls/ca.crt --cert /tls/tls.crt --key /tls/tls.key "$url" 2>&1; then
    echo "  => OK"
  else
    echo "  => FAILED"
    printf '$ (same, -k: server certificate not verified) '
    kubectl -n "$ns" exec "deploy/$d" -- curl -sS -k --max-time 5 --cert /tls/tls.crt --key /tls/tls.key -o /dev/null -w 'HTTP %{http_code}\n' "$url" 2>&1 || true
  fi
}

# wait_mount <namespace> <deployment> <secret> [mount]: until the Secret's current tls.crt and ca.crt
# are what the workload's container sees (the kubelet refreshes Secret volumes with a delay)
wait_mount() {
  local ns="$1" d="$2" s="$3" m="${4:-/tls}" want got
  want="$(kubectl -n "$ns" get secret "$s" -o jsonpath='{.data.tls\.crt}{.data.ca\.crt}')"
  for _ in $(seq 1 60); do
    got="$(kubectl -n "$ns" exec "deploy/$d" -- sh -c "base64 < $m/tls.crt | tr -d '\n'; base64 < $m/ca.crt | tr -d '\n'" 2>/dev/null || true)"
    if [ "$got" = "$want" ]; then echo "  (deploy/$d sees the current $s)"; return 0; fi
    sleep 3
  done
  echo "  TIMEOUT: deploy/$d does not see the current $s"
}

# restart <namespace> <deployment>: a routine restart (reschedule, node drain, rollout), so the
# process loads the material currently in its Secret
restart() {
  run kubectl -n "$1" rollout restart "deploy/$2"
  run kubectl -n "$1" rollout status "deploy/$2" --timeout=120s
  for _ in $(seq 1 60); do
    [ "$(kubectl -n "$1" get pods -l "app=$2" --no-headers 2>/dev/null | wc -l | tr -d ' ')" = \
      "$(kubectl -n "$1" get deploy "$2" -o jsonpath='{.spec.replicas}')" ] && return 0
    sleep 2
  done
}

# --- M6 additions -------------------------------------------------------------------------------

# scratch dir for one run, removed on exit together with the cluster
scratch() { T="$(mktemp -d -t ri-gt6.XXXXXX)"; export T; }

# render_diff <chart-from.tgz> <chart-to.tgz> <release> <namespace> <out> <helm args...>: the
# environment's own FROM/TO render, canonicalised per object
render_diff() {
  local a="$1" b="$2" rel="$3" ns="$4" out="$5"; shift 5
  helm template "$rel" "$a" -n "$ns" "$@" > "$T/render-from.yaml"
  helm template "$rel" "$b" -n "$ns" "$@" > "$T/render-to.yaml"
  python3 "$(dirname "${BASH_SOURCE[0]}")/render-diff.py" "$T/render-from.yaml" "$T/render-to.yaml" "$out"
}

# curl_pod: a client pod in the default namespace for in-cluster HTTP probes
curl_pod() {
  run kubectl -n default run curl --image=curlimages/curl:8.11.1 --restart=Never --command -- sleep 86400
  run kubectl -n default wait pod/curl --for=condition=Ready --timeout=120s
}
