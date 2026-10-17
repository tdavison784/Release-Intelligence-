#!/usr/bin/env bash
# Observed upgrade kube-prometheus-stack chart 89.2.4 -> 90.2.0 for this environment (shared by the
# token-secret pair).
# usage: observed/run.sh <case-dir> <kind-cluster-name> <charts-dir>   (output: observed/run.log)
# Flux is not installed: the Kustomization order is reproduced (policies, then monitoring), and the
# HelmRelease with helm and the values of the generated ConfigMap. Flux's helm-controller performs
# the same Helm install/upgrade (no remediation is configured, so a failed upgrade is left as is).
CASE="$(cd "$1" && pwd)"; CHARTS="$(cd "$3" && pwd)"
. "$CASE/../observed-lib.sh"
ENV="$CASE/env"; M="$ENV/infrastructure/monitoring"
exec > >(tee "$CASE/observed/run.log") 2>&1
echo "# case $(basename "$CASE"); kind $(kind version); helm $(helm version --short); $(date -u +%Y-%m-%dT%H:%M:%SZ)"
kind_up "$2"

targets() { # Prometheus' active targets per scrape pool, through the API server's service proxy
  printf '$ kubectl get --raw /api/v1/namespaces/monitoring/services/kube-prometheus-stack-prometheus:http-web/proxy/api/v1/targets | jq <per scrape pool: up/total, last error>\n'
  kubectl get --raw /api/v1/namespaces/monitoring/services/kube-prometheus-stack-prometheus:http-web/proxy/api/v1/targets |
    jq -r '.data.activeTargets | group_by(.scrapePool)[] | "  \(.[0].scrapePool)  up \([.[] | select(.health=="up")] | length)/\(length)  \([.[] | .lastError | select(. != "")] | unique | join("; "))"'
}
servicemonitors() {
  run kubectl -n monitoring get servicemonitors
  printf '$ kubectl -n monitoring get servicemonitor kube-prometheus-stack-kubelet -o jsonpath={.spec.endpoints[0]}\n'
  kubectl -n monitoring get servicemonitor kube-prometheus-stack-kubelet -o jsonpath='{.spec.endpoints[0]}' 2>&1 | cut -c1-600; echo
}

log "Kustomization policies"
apply_kustomize "$ENV/infrastructure/policies"
run kubectl get validatingadmissionpolicies,validatingadmissionpolicybindings
log "Kustomization monitoring: HelmRelease kube-prometheus-stack, chart 89.2.4, values from ConfigMap kube-prometheus-stack-values"
run kubectl create namespace monitoring
run helm install kube-prometheus-stack "$CHARTS/kube-prometheus-stack-89.2.4.tgz" -n monitoring -f "$M/values.yaml" --wait --timeout 8m
run kubectl -n monitoring rollout status statefulset/prometheus-kube-prometheus-stack-prometheus --timeout=300s
sleep 75

log "baseline on chart 89.2.4"
servicemonitors
run kubectl -n monitoring get secrets --field-selector type=kubernetes.io/service-account-token
targets

log "UPGRADE: helm upgrade to chart 90.2.0 with the same values"
try helm upgrade kube-prometheus-stack "$CHARTS/kube-prometheus-stack-90.2.0.tgz" -n monitoring -f "$M/values.yaml" --wait --timeout 8m
run helm -n monitoring history kube-prometheus-stack
run kubectl -n monitoring get secrets --field-selector type=kubernetes.io/service-account-token
servicemonitors
log "after upgrade (scrape interval 30s; waiting for the operator to reconcile and Prometheus to scrape)"
sleep 120
targets
log "prometheus-operator log lines about the control-plane ServiceMonitors"
kubectl -n monitoring logs deploy/kube-prometheus-stack-operator --tail=500 | grep -iE 'token|secret|kubelet|apiserver|coredns|skipping|invalid' | tail -20 || true
log "done"
