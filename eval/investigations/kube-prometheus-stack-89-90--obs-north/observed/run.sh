#!/usr/bin/env bash
# Known-outcome run for the kube-prometheus-stack chart 89.2.4 -> 90.2.0 control-plane-auth cases
# (groundtruth lane). Both chart versions ship prometheus-operator v0.93.1 and identical CRDs, so
# everything that changes comes from the chart's templates and values.
#
# Applies the case's environment the way its Argo CD Applications would (Argo CD itself is not
# run: each Application is resolved by hand into the equivalent helm / kubectl kustomize command),
# upgrades the chart with the same value files, and records what Prometheus scrapes before and
# after. Prometheus is queried from an in-cluster curl pod (the chart's images are distroless).
#
# usage: run.sh <case-dir> <kind-cluster-name> <dir-with-chart-tgzs>
#   charts: kube-prometheus-stack-89.2.4.tgz and kube-prometheus-stack-90.2.0.tgz from
#   https://prometheus-community.github.io/helm-charts
set -euo pipefail

CASE=$(cd "$1" && pwd)
NAME=$2
CHARTS=$(cd "$3" && pwd)
ENV=$CASE/env
OUT=$CASE/observed
CTX=kind-$NAME
LOG=$OUT/run.log
: > "$LOG"
VALUES=(-f "$ENV/platform/monitoring/values.yaml" -f "$ENV/platform/monitoring/values-prod.yaml")
PROM=http://kube-prometheus-stack-prometheus.monitoring.svc:9090

k() { kubectl --context "$CTX" "$@"; }
log() { echo "### $(date -u +%H:%M:%SZ) $*" | tee -a "$LOG"; }
rec() { "$@" 2>&1 | tee -a "$LOG"; }

q() { # instant query; prints "<metric labels> <value>" per series, or "(no series)"
  k exec -n default client -- curl -s --max-time 10 -G "$PROM/api/v1/query" --data-urlencode "query=$1" \
    | python3 -c 'import json,sys
d=json.load(sys.stdin)["data"]["result"]
print("\n".join("   %s %s" % ({k:v for k,v in r["metric"].items() if k in ("job","namespace","pod","service")}, r["value"][1]) for r in d) or "   (no series)")'
}
observe() {
  log "observation: $1"
  echo "-- ServiceMonitors (namespace/name)" | tee -a "$LOG"
  k get servicemonitors -A --no-headers -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name | tee -a "$LOG"
  echo "-- token Secret" | tee -a "$LOG"
  k get secret -A --field-selector type=kubernetes.io/service-account-token --no-headers -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name 2>&1 | tee -a "$LOG"
  echo "-- scrape jobs in the running Prometheus configuration" | tee -a "$LOG"
  k exec -n default client -- curl -s --max-time 10 "$PROM/api/v1/status/config" \
    | python3 -c 'import json,sys,re
y=json.load(sys.stdin)["data"]["yaml"]
print("\n".join("   "+j for j in sorted(re.findall(r"job_name: (\S+)", y))))' | tee -a "$LOG"
  echo "-- up by job" | tee -a "$LOG"
  q 'sum by (job) (up)' | tee -a "$LOG"
  echo "-- inputs of rule checkout-alerts/CheckoutMemoryNearLimit (cAdvisor via kubelet, and kube-state-metrics)" | tee -a "$LOG"
  echo "   count(container_memory_working_set_bytes{namespace=\"checkout\",container=\"checkout\"}):" | tee -a "$LOG"
  q 'count(container_memory_working_set_bytes{namespace="checkout",container="checkout"})' | tee -a "$LOG"
  echo "   count(kube_pod_container_resource_limits{namespace=\"checkout\",container=\"checkout\",resource=\"memory\"}):" | tee -a "$LOG"
  q 'count(kube_pod_container_resource_limits{namespace="checkout",container="checkout",resource="memory"})' | tee -a "$LOG"
  echo "   the rule's ratio without the threshold:" | tee -a "$LOG"
  q 'max by (pod) (container_memory_working_set_bytes{namespace="checkout", container="checkout"}) / max by (pod) (kube_pod_container_resource_limits{namespace="checkout", container="checkout", resource="memory"})' | tee -a "$LOG"
  echo "-- input of rule checkout-alerts/CheckoutRestarting (kube-state-metrics)" | tee -a "$LOG"
  q 'count(kube_pod_container_status_restarts_total{namespace="checkout"})' | tee -a "$LOG"
  echo "-- rule groups loaded" | tee -a "$LOG"
  k exec -n default client -- curl -s --max-time 10 "$PROM/api/v1/rules" \
    | python3 -c 'import json,sys
g=json.load(sys.stdin)["data"]["groups"]
print("   checkout.rules loaded:", any(x["name"]=="checkout.rules" for x in g), "| groups:", len(g))' | tee -a "$LOG"
  echo "-- operator: rejected ServiceMonitors (events + log)" | tee -a "$LOG"
  k get events -A --field-selector reason=InvalidConfiguration --no-headers -o custom-columns=NS:.metadata.namespace,OBJ:.involvedObject.name,MSG:.message 2>&1 | sort -u | tail -20 | tee -a "$LOG"
  k logs -n monitoring deploy/kube-prometheus-stack-operator --since=5m 2>&1 | grep -i -E 'skipping object|rejected' | tail -10 | cut -c1-400 | tee -a "$LOG" || true
}
wait_scrapes() { # give the operator time to reconcile and Prometheus two scrape intervals (30s each)
  k rollout status -n monitoring deploy/kube-prometheus-stack-operator --timeout=300s >>"$LOG" 2>&1
  k wait -n monitoring --for=condition=Ready pod -l app.kubernetes.io/name=prometheus --timeout=300s >>"$LOG" 2>&1
  sleep "$1"
}

log "case $(basename "$CASE"), cluster $NAME, $(kind version)"
kind create cluster --name "$NAME" --wait 180s >>"$LOG" 2>&1
rec k version

log "render-level diff of the release (helm template 89.2.4 vs 90.2.0, same value files)"
helm template kube-prometheus-stack "$CHARTS/kube-prometheus-stack-89.2.4.tgz" -n monitoring "${VALUES[@]}" > "$OUT/render-89.2.4.yaml"
helm template kube-prometheus-stack "$CHARTS/kube-prometheus-stack-90.2.0.tgz" -n monitoring "${VALUES[@]}" > "$OUT/render-90.2.0.yaml"
diff -u --label render-89.2.4.yaml --label render-90.2.0.yaml "$OUT/render-89.2.4.yaml" "$OUT/render-90.2.0.yaml" > "$OUT/render.diff" || true
rm "$OUT/render-89.2.4.yaml" "$OUT/render-90.2.0.yaml"

log "phase 0: install chart 89.2.4 (Application kube-prometheus-stack) + apps/checkout/overlays/prod"
rec helm --kube-context "$CTX" upgrade --install kube-prometheus-stack "$CHARTS/kube-prometheus-stack-89.2.4.tgz" -n monitoring --create-namespace "${VALUES[@]}" --wait --timeout 10m
rec k apply -k "$ENV/apps/checkout/overlays/prod"
rec k rollout status -n checkout deploy/checkout --timeout=180s
rec k run client -n default --image=curlimages/curl:8.10.1 --restart=Never --command -- sleep 86400
k wait -n default --for=condition=Ready pod/client --timeout=180s >>"$LOG" 2>&1
wait_scrapes 120
observe "chart 89.2.4"

log "phase 1: upgrade to chart 90.2.0 (Application targetRevision bump, same value files)"
rec helm --kube-context "$CTX" upgrade kube-prometheus-stack "$CHARTS/kube-prometheus-stack-90.2.0.tgz" -n monitoring "${VALUES[@]}" --wait --timeout 10m
# Old series stay queryable for 5 minutes after their target disappears (staleness only applies
# to targets that are still scraped), so wait past that window before observing.
wait_scrapes 360
observe "chart 90.2.0"

log "done; deleting cluster"
kind delete cluster --name "$NAME" >>"$LOG" 2>&1
