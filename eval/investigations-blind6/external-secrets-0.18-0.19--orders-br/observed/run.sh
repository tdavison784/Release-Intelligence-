#!/usr/bin/env bash
# Ground-truth tooling (never read by a system under test).
# usage: observed/run.sh <case-dir> <kind-cluster-name> <charts-dir> <variant> [<log-name>]
#   <charts-dir> holds argo-cd-8.5.1.tgz (argo-helm), external-secrets-0.18.2.tgz and -0.19.2.tgz
#   <variant>: env  - the environment's own Application (clusters/prod/external-secrets.yaml)
#              csa  - an Application as an ApplicationSet would generate it, default sync options
#              ssa  - the same with ServerSideApply=true
#   (csa/ssa are the alternatives for an environment whose delivery configuration is withheld)
#
# One throwaway kind cluster with a real Argo CD (v3.1.5, chart 8.5.1). The External Secrets
# Application is synced at chart 0.18.2 with the environment's values, then its targetRevision is
# bumped to 0.19.2 (the proposed change) and Argo CD's automated sync runs. The Application has a
# single Helm source with the values file inlined (helm.valuesObject) instead of the multi-source
# $values reference, because the kind cluster has no Git server; syncPolicy and syncOptions are kept
# (csa/ssa run in the default AppProject, with a default Argo CD install). A fake-provider
# ClusterSecretStore and ExternalSecret (probe objects, not part of the environment) show whether
# the operator still reconciles.
set -euo pipefail
CASE="$(cd "$1" && pwd)"; CHARTS="$(cd "$3" && pwd)"; VARIANT="$4"; LOG="${5:-run.log}"
# shellcheck source=../../observed-lib.sh
. "$CASE/../observed-lib.sh"
ENV="$CASE/env"; OUT="$CASE/observed"; ID="$(basename "$CASE")"
exec > >(tee "$OUT/$LOG") 2>&1
scratch
trap 'kind_down; rm -rf "${T:?}"' EXIT
echo "# $ID ($VARIANT): External Secrets chart 0.18.2 -> 0.19.2 through Argo CD v3.1.5; kind $(kind version | cut -d' ' -f2), node ${KIND_NODE_IMAGE%@*}"

VALUES="$ENV/platform/external-secrets/values.yaml"
helm template external-secrets "$CHARTS/external-secrets-0.18.2.tgz" -n external-secrets -f "$VALUES" > "$T/crd-from.yaml"
helm template external-secrets "$CHARTS/external-secrets-0.19.2.tgz" -n external-secrets -f "$VALUES" > "$T/crd-to.yaml"
if [ ! -f "$OUT/render.diff" ]; then
  log "environment render FROM/TO (observed/render.diff)"
  render_diff "$CHARTS/external-secrets-0.18.2.tgz" "$CHARTS/external-secrets-0.19.2.tgz" external-secrets external-secrets "$OUT/render.diff" -f "$VALUES"
fi
log "CRD sizes in the environment's render"
python3 - "$T/crd-from.yaml" "$T/crd-to.yaml" <<'EOF'
import json, sys, yaml
for tag, f in zip(("0.18.2", "0.19.2"), sys.argv[1:]):
    for d in yaml.safe_load_all(open(f)):
        if d and d["kind"] == "CustomResourceDefinition" and len(json.dumps(d)) > 200000:
            print(f"  {tag}: {d['metadata']['name']} serialized {len(json.dumps(d, separators=(',', ':')))} bytes")
EOF
echo "  (a client-side apply stores the whole object in the last-applied-configuration annotation; annotations total at most 262144 bytes)"

cat > "$T/app_state.py" <<'EOF'
import json, sys
a = json.load(sys.stdin); st = a.get("status", {}); op = st.get("operationState", {})
print("  revision:", st.get("sync", {}).get("revision"), "| sync:", st.get("sync", {}).get("status"), "| health:", st.get("health", {}).get("status"))
print("  operation:", op.get("phase"), "|", (op.get("message") or "")[:600])
for r in op.get("syncResult", {}).get("resources", []):
    kind, name, status = r.get("kind"), r.get("name", ""), r.get("status")
    if status != "Synced" or kind == "Deployment" or "secretstores" in name:
        print(f"    {kind}/{name}: {status} {r.get('syncPhase', '')} {(r.get('message') or '')[:300]}")
for c in st.get("conditions", []):
    print("  condition:", c.get("type"), (c.get("message") or "")[:300])
EOF
cat > "$T/crd_state.py" <<'EOF'
import hashlib, json, sys, yaml
d = json.load(sys.stdin)
def h(o): return hashlib.sha256(json.dumps([v["schema"] for v in o["spec"]["versions"]], sort_keys=True).encode()).hexdigest()[:12]
known = {}
for tag, f in (("0.18.2", sys.argv[1]), ("0.19.2", sys.argv[2])):
    for r in yaml.safe_load_all(open(f)):
        if r and r["kind"] == "CustomResourceDefinition" and r["metadata"]["name"] == d["metadata"]["name"]:
            known[h(r)] = tag
la = d["metadata"].get("annotations", {}).get("kubectl.kubernetes.io/last-applied-configuration")
print(f"  {d['metadata']['name']:45s} {known.get(h(d), 'other'):10s} {len(la) if la else 'none'}")
EOF
python3 - "$VARIANT" "$ENV" "$T/app.yaml" <<'EOF'
import sys, yaml
variant, env, out = sys.argv[1:]
values = yaml.safe_load(open(f"{env}/platform/external-secrets/values.yaml"))
if variant == "env":
    app = yaml.safe_load(open(f"{env}/clusters/prod/external-secrets.yaml"))
    src = app["spec"].pop("sources")[0]
else:
    app = {"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
           "metadata": {"name": "external-secrets", "namespace": "argocd"},
           "spec": {"project": "default",
                    "destination": {"server": "https://kubernetes.default.svc", "namespace": "external-secrets"},
                    "syncPolicy": {"automated": {"prune": True, "selfHeal": True},
                                   "syncOptions": ["CreateNamespace=true"] + (["ServerSideApply=true"] if variant == "ssa" else [])}}}
    src = {"repoURL": "https://charts.external-secrets.io", "chart": "external-secrets", "targetRevision": "0.18.2",
           "helm": {"releaseName": "external-secrets"}}
src["helm"].pop("valueFiles", None)
src["helm"]["valuesObject"] = values
app["spec"]["source"] = src
yaml.safe_dump(app, open(out, "w"), sort_keys=False)
EOF

app_state() { kubectl -n argocd get application external-secrets -o json | python3 "$T/app_state.py"; }

crd_state() {
  printf '  %-45s %-10s %s\n' CRD SCHEMA LAST-APPLIED-ANNOTATION
  for c in clustersecretstores.external-secrets.io secretstores.external-secrets.io externalsecrets.external-secrets.io; do
    kubectl get crd "$c" -o json | python3 "$T/crd_state.py" "$T/crd-from.yaml" "$T/crd-to.yaml"
  done
}

# wait_op <revision>: until the last sync operation for <revision> has finished
wait_op() {
  local p r
  for _ in $(seq 1 120); do
    p="$(kubectl -n argocd get application external-secrets -o jsonpath='{.status.operationState.phase}' 2>/dev/null || true)"
    r="$(kubectl -n argocd get application external-secrets -o jsonpath='{.status.operationState.syncResult.revision}' 2>/dev/null || true)"
    if [ "$r" = "$1" ] && { [ "$p" = Succeeded ] || [ "$p" = Failed ] || [ "$p" = Error ]; }; then return 0; fi
    sleep 3
  done
  echo "  (timeout waiting for the sync of $1)"
}

probe_es() {
  log "probe objects: fake-provider ClusterSecretStore + ExternalSecret ($1)"
  cat > "$T/probe.yaml" <<EOF
apiVersion: v1
kind: Namespace
metadata: {name: probe}
---
apiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata: {name: probe-fake}
spec:
  provider:
    fake:
      data:
        - {key: /probe/value, value: "$2"}
---
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata: {name: probe, namespace: probe}
spec:
  refreshInterval: 10s
  secretStoreRef: {kind: ClusterSecretStore, name: probe-fake}
  target: {name: probe}
  data:
    - {secretKey: value, remoteRef: {key: /probe/value}}
EOF
  local ok=""
  for _ in $(seq 1 24); do
    if kubectl apply -f "$T/probe.yaml" > "$T/probe-apply.out" 2>&1; then ok=1; break; fi
    sleep 5
  done
  printf '$ kubectl apply -f probe.yaml\n'; sed 's/^/  /' "$T/probe-apply.out"; [ -n "$ok" ] || echo "  (apply failed for 2 minutes)"
  sleep 20
  try kubectl -n probe get externalsecret probe -o jsonpath='{range .status.conditions[*]}{.type}={.status} reason={.reason} message={.message}{"\n"}{end}'
  printf '$ kubectl -n probe get secret probe -o jsonpath={.data.value} | base64 -d\n  '
  kubectl -n probe get secret probe -o jsonpath='{.data.value}' 2>/dev/null | base64 -d 2>/dev/null || printf '(no secret)'; echo
}

kind_up "$2"

log "Argo CD v3.1.5 (argo-cd chart 8.5.1)"
if [ "$VARIANT" = env ] && [ -f "$ENV/platform/argocd/values.yaml" ]; then AV=(-f "$ENV/platform/argocd/values.yaml"); else AV=(--set dex.enabled=false --set notifications.enabled=false); fi
run helm install argocd "$CHARTS/argo-cd-8.5.1.tgz" -n argocd --create-namespace "${AV[@]}" --wait --timeout 300s
run kubectl -n argocd get statefulset argocd-application-controller -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
if [ "$VARIANT" = env ]; then run kubectl apply -f "$ENV/clusters/prod/project.yaml"; fi

log "the Application under test ($VARIANT; helm.valuesObject = platform/external-secrets/values.yaml)"
python3 -c 'import sys, yaml; a = yaml.safe_load(open(sys.argv[1])); a["spec"]["source"]["helm"]["valuesObject"] = "<platform/external-secrets/values.yaml>"; print(yaml.safe_dump(a, sort_keys=False))' "$T/app.yaml" | sed 's/^/  /'
run kubectl apply -f "$T/app.yaml"

log "FROM: automated sync at 0.18.2"
wait_op 0.18.2
app_state
for d in external-secrets external-secrets-webhook external-secrets-cert-controller; do
  run kubectl -n external-secrets rollout status deploy/$d --timeout=180s
done
run kubectl -n external-secrets get deploy external-secrets -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
crd_state
probe_es "FROM 0.18.2" from-0.18.2
log "FROM: the environment's ClusterSecretStore and ExternalSecret (accepted by the API; AWS is unreachable from kind)"
apply_kustomize "$ENV/platform/secret-stores"
try kubectl create namespace orders
try kubectl -n orders apply -f "$ENV/apps/orders/externalsecret.yaml"

log "UPGRADE: targetRevision 0.18.2 -> 0.19.2 (the proposed change); Argo CD's automated sync"
run kubectl -n argocd patch application external-secrets --type merge -p '{"spec":{"source":{"targetRevision":"0.19.2"}}}'
wait_op 0.19.2
sleep 30
app_state
log "cluster state after the sync of 0.19.2"
run kubectl -n external-secrets get deploy -o 'custom-columns=NAME:.metadata.name,IMAGE:.spec.template.spec.containers[0].image,READY:.status.readyReplicas'
crd_state
probe_es "TO 0.19.2" to-0.19.2
log "external-secrets controller log (errors, deduplicated)"
kubectl -n external-secrets logs deploy/external-secrets --tail=400 2>&1 | grep -iE '"level":"error"|error' |
  sed -E 's/"ts":"?[^,]*,//' | cut -c1-300 | sort | uniq -c | sort -rn | head -12 | sed 's/^/  /' || true
log "Argo CD application controller: lines about annotation size"
kubectl -n argocd logs statefulset/argocd-application-controller --tail=-1 2>&1 | grep -iE 'too long' |
  sed -E 's/time="[^"]*" //' | cut -c1-400 | sort | uniq -c | head -6 | sed 's/^/  /' || true
