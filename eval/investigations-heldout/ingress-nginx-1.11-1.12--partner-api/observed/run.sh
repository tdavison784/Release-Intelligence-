#!/usr/bin/env bash
# Observed upgrade ingress-nginx chart 4.11.5 (controller v1.11.5) -> 4.12.0 (controller v1.12.0) for
# this environment.
# usage: observed/run.sh <case-dir> <kind-cluster-name> <charts-dir>   (output: observed/run.log)
# Argo CD is not installed: its sync waves are reproduced in order (memcached, ingress-nginx with
# helm and the Application's value file, partner-api).
# helm runs without --wait: in kind a LoadBalancer Service never gets an address; the controller
# rollout is waited for instead (the admission webhook Jobs are hooks and complete before helm returns).
CASE="$(cd "$1" && pwd)"; CHARTS="$(cd "$3" && pwd)"
. "$CASE/../observed-lib.sh"
ENV="$CASE/env"
exec > >(tee "$CASE/observed/run.log") 2>&1
echo "# case $(basename "$CASE"); kind $(kind version); helm $(helm version --short); $(date -u +%Y-%m-%dT%H:%M:%SZ)"
kind_up "$2"

burst() { # burst <partner-id> <n>: n requests as one partner through the controller Service; status code tally
  printf '$ %s x curl -H "Host: api.partners.example.com" -H "X-Partner-Id: %s" http://ingress-nginx-controller.ingress-nginx.svc/\n' "$2" "$1"
  kubectl exec probe -- sh -c "for i in \$(seq 1 $2); do curl -s -o /dev/null -w '%{http_code}\n' --max-time 5 -H 'Host: api.partners.example.com' -H 'X-Partner-Id: $1' http://ingress-nginx-controller.ingress-nginx.svc/; done" | sort | uniq -c | sed 's/^/  /'
}
metrics() { # the controller's Prometheus endpoint (pod annotation prometheus.io/port 10254)
  local pod; pod="$(kubectl -n ingress-nginx get pod -l app.kubernetes.io/component=controller -o jsonpath='{.items[0].status.podIP}')"
  printf '$ curl http://<controller pod>:10254/metrics | grep -c ^nginx_ingress_controller_  ->  '
  kubectl exec probe -- sh -c "curl -s --max-time 5 http://$pod:10254/metrics | grep -c '^nginx_ingress_controller_'" 2>&1 || echo "unreachable"
}

throttle_conf() { # does the rendered nginx.conf of a controller pod contain the global throttle?
  local pod; pod="$(kubectl -n ingress-nginx get pod -l app.kubernetes.io/component=controller -o jsonpath='{.items[0].metadata.name}')"
  printf '$ kubectl -n ingress-nginx exec <controller pod> -- grep -c global_throttle /etc/nginx/nginx.conf  ->  '
  kubectl -n ingress-nginx exec "$pod" -- grep -c global_throttle /etc/nginx/nginx.conf 2>&1 || true
}

log "wave -1: platform/memcached (Application option CreateNamespace=true)"
run kubectl create namespace ingress-nginx
apply_kustomize "$ENV/platform/memcached"
log "wave 0: ingress-nginx chart 4.11.5, platform/ingress-nginx/values.yaml"
run helm install ingress-nginx "$CHARTS/ingress-nginx-4.11.5.tgz" -n ingress-nginx --create-namespace -f "$ENV/platform/ingress-nginx/values.yaml" --timeout 5m
run kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=300s
run kubectl -n ingress-nginx rollout status deploy/memcached --timeout=120s
run kubectl -n ingress-nginx get deploy ingress-nginx-controller -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}{.spec.template.spec.containers[0].args}{"\n"}'
run kubectl -n ingress-nginx get configmap ingress-nginx-controller -o jsonpath='{.data}{"\n"}'
log "wave 1: apps/partner-api/overlays/prod"
apply_kustomize "$ENV/apps/partner-api/overlays/prod"
run kubectl -n partner-api rollout status deploy/partner-api --timeout=180s
run kubectl run probe --image=curlimages/curl:8.10.1 --restart=Never --command -- sleep 3600
run kubectl wait pod/probe --for=condition=Ready --timeout=120s
sleep 10

log "baseline on controller v1.11.5: 130 requests in a burst from one partner (quota 120/min)"
burst partner-a 130
burst partner-b 20
metrics
throttle_conf

log "UPGRADE: helm upgrade to chart 4.12.0 with the same value file"
run helm upgrade ingress-nginx "$CHARTS/ingress-nginx-4.12.0.tgz" -n ingress-nginx -f "$ENV/platform/ingress-nginx/values.yaml" --timeout 5m
run kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=300s
run kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=180s
run kubectl -n ingress-nginx get deploy ingress-nginx-controller -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}{.spec.template.spec.containers[0].args}{"\n"}'
run kubectl -n ingress-nginx get configmap ingress-nginx-controller -o jsonpath='{.data}{"\n"}'
sleep 10
log "after upgrade: the same burst from a partner with no requests so far"
burst partner-c 130
log "after upgrade: partner-a again (its window from the baseline burst has expired)"
sleep 60
burst partner-a 130
metrics
log "after upgrade: a change to the partner-api Ingress is admitted"
try kubectl -n partner-api annotate ingress partner-api example.com/reviewed=yes --overwrite
log "controller configuration and log lines about global rate limiting"
throttle_conf
kubectl -n ingress-nginx logs deploy/ingress-nginx-controller --tail=400 | grep -iE 'global.?rate|throttle|memcached|unknown|partner-api' | tail -15 || true
log "done"
