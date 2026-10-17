#!/usr/bin/env bash
# Observed upgrade ingress-nginx chart 4.11.5 (controller v1.11.5) -> 4.12.0 (controller v1.12.0) for
# this environment (shared by the docs-paths pair).
# usage: observed/run.sh <case-dir> <kind-cluster-name> <charts-dir>   (output: observed/run.log)
# Argo CD is not installed: the ingress-nginx Application is reproduced with helm (same chart version
# and value files, in order), the docs Application with kubectl apply --server-side (its
# ServerSideApply=true sync option; Argo CD applies every resource of the app on each sync).
# helm runs without --wait: in kind a LoadBalancer Service never gets an address; the controller
# rollout is waited for instead (the admission webhook Jobs are hooks and complete before helm returns).
CASE="$(cd "$1" && pwd)"; CHARTS="$(cd "$3" && pwd)"
. "$CASE/../observed-lib.sh"
ENV="$CASE/env"
exec > >(tee "$CASE/observed/run.log") 2>&1
echo "# case $(basename "$CASE"); kind $(kind version); helm $(helm version --short); $(date -u +%Y-%m-%dT%H:%M:%SZ)"
kind_up "$2"

VALUES=(-f "$ENV/platform/ingress-nginx/values.yaml" -f "$ENV/platform/ingress-nginx/values-prod.yaml")
sync_docs() { # what an Argo CD sync of the docs Application sends
  printf '$ kubectl kustomize apps/docs/overlays/prod | kubectl apply --server-side --field-manager=argocd-controller -f -\n'
  if kubectl kustomize "$ENV/apps/docs/overlays/prod" | kubectl apply --server-side --field-manager=argocd-controller -f - 2>&1; then
    echo "  => sync OK"; else echo "  => sync FAILED"; fi
}
get() { # get <path>: request through the controller Service with the docs host
  printf '$ curl -H "Host: docs.example.com" http://ingress-nginx-controller.ingress-nginx.svc%s  ->  ' "$1"
  kubectl exec probe -- curl -s -o /dev/null -w '%{http_code}\n' --max-time 5 -H 'Host: docs.example.com' "http://ingress-nginx-controller.ingress-nginx.svc$1" 2>&1 || true
}
checks() { get /; get /.well-known/security.txt; }

log "FROM: ingress-nginx chart 4.11.5 with platform/ingress-nginx/values.yaml + values-prod.yaml"
run helm install ingress-nginx "$CHARTS/ingress-nginx-4.11.5.tgz" -n ingress-nginx --create-namespace "${VALUES[@]}" --timeout 5m
run kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=300s
run kubectl -n ingress-nginx get deploy ingress-nginx-controller -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}{.spec.template.spec.containers[0].args}{"\n"}'
log "docs Application (sync wave 1)"
sync_docs
for d in docs-site wellknown; do run kubectl -n docs rollout status "deploy/$d" --timeout=180s; done
run kubectl run probe --image=curlimages/curl:8.10.1 --restart=Never --command -- sleep 3600
run kubectl wait pod/probe --for=condition=Ready --timeout=120s
sleep 5

log "baseline on controller v1.11.5"
checks
log "control on v1.11.5: a later sync of the docs Application, then a change to the Ingress"
sync_docs
run kubectl -n docs annotate ingress docs nginx.ingress.kubernetes.io/proxy-read-timeout=30 --overwrite
checks

log "UPGRADE: helm upgrade to chart 4.12.0 with the same value files"
run helm upgrade ingress-nginx "$CHARTS/ingress-nginx-4.12.0.tgz" -n ingress-nginx "${VALUES[@]}" --timeout 5m
run kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=300s
run kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=180s
run kubectl -n ingress-nginx get deploy ingress-nginx-controller -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}{.spec.template.spec.containers[0].args}{"\n"}'
sleep 10
log "after upgrade: requests to the existing Ingress"
checks
log "after upgrade: the next sync of the docs Application (no change in the repository)"
sync_docs
log "after upgrade: a change to the Ingress"
try kubectl -n docs annotate ingress docs nginx.ingress.kubernetes.io/proxy-read-timeout=60 --overwrite
log "after upgrade: the controller restarts"
run kubectl -n ingress-nginx rollout restart deploy/ingress-nginx-controller
run kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=180s
sleep 10
checks
log "after upgrade: the Ingress is re-created (e.g. the Application is re-synced into a rebuilt cluster)"
run kubectl -n docs delete ingress docs
sync_docs
sleep 5
checks
run kubectl -n docs get ingress
log "controller log lines about the docs Ingress"
kubectl -n ingress-nginx logs deploy/ingress-nginx-controller --tail=300 | grep -iE 'docs/docs|invalid|pathType' | tail -15 || true
log "done"
