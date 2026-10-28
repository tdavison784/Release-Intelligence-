#!/usr/bin/env bash
# Ground-truth tooling (never read by a system under test).
# usage: observed/run.sh <case-dir> <kind-cluster-name> <charts-dir>
#   <charts-dir> holds argo-cd-7.9.1.tgz (v2.14.11), argo-cd-8.2.7.tgz (v3.0.12) and argocd-apps-2.0.2.tgz
#
# One throwaway kind cluster. Argo CD v2.14.11 is installed from chart 7.9.1 with the environment's
# argocd/values.yaml (the helmfile release), plus an observation overlay that adds a local account
# `dev1` in the group `studio-devs` (stand-in for an Okta member of that group; the environment's RBAC
# lines are kept verbatim). The `games` AppProject comes from argocd/apps.yaml (argocd-apps chart); the
# `lobby` Application keeps its project and destination but its source is a public chart (podinfo),
# because the kind cluster has no Git server. dev1 signs in through the API and asks for the
# Application and for its pod logs, before and after `helm upgrade` to chart 8.2.7 (v3.0.12) with the
# same values.
set -euo pipefail
CASE="$(cd "$1" && pwd)"; CHARTS="$(cd "$3" && pwd)"
# shellcheck source=../../observed-lib.sh
. "$CASE/../observed-lib.sh"
ENV="$CASE/env"; OUT="$CASE/observed"; ID="$(basename "$CASE")"
exec > >(tee "$OUT/run.log") 2>&1
scratch
trap 'kind_down; rm -rf "${T:?}"' EXIT
echo "# $ID: Argo CD v2.14.11 (chart 7.9.1) -> v3.0.12 (chart 8.2.7); kind $(kind version | cut -d' ' -f2), node ${KIND_NODE_IMAGE%@*}"
PASS='dev1-observation-only'

log "environment render FROM/TO (observed/render.diff)"
render_diff "$CHARTS/argo-cd-7.9.1.tgz" "$CHARTS/argo-cd-8.2.7.tgz" argocd argocd "$OUT/render.diff" -f "$ENV/argocd/values.yaml"

log "observation overlay (not part of the environment)"
python3 - "$ENV/argocd/values.yaml" "$T/overlay.yaml" "$PASS" <<'EOF'
import subprocess, sys, yaml
env, out, pw = sys.argv[1:]
v = yaml.safe_load(open(env))
h = subprocess.run(["htpasswd", "-nbBC", "10", "dev1", pw], capture_output=True, text=True, check=True).stdout.strip().split(":", 1)[1]
h = "$2a$" + h[4:]
overlay = {"configs": {"cm": {"accounts.dev1": "login"},
                       "rbac": {"policy.csv": v["configs"]["rbac"]["policy.csv"] + "g, dev1, studio-devs\n"},
                       "secret": {"extra": {"accounts.dev1.password": h, "accounts.dev1.passwordMtime": "2026-10-27T00:00:00Z"}}}}
yaml.safe_dump(overlay, open(out, "w"), sort_keys=False)
print("  configs.cm.accounts.dev1: login")
print("  configs.rbac.policy.csv: <the environment's policy.csv> + 'g, dev1, studio-devs'")
print("  configs.secret.extra: accounts.dev1.password (bcrypt)")
EOF

kind_up "$2"
curl_pod

API=http://argocd-server.argocd.svc
api() { kubectl -n default exec curl -- curl -s --max-time 10 "$@"; }

as_dev1() { # as_dev1 <label>: sign in as dev1, read the Application, read the pod logs
  log "dev1 (member of studio-devs) through the API ($1)"
  local tok pod
  tok="$(api -X POST -H 'Content-Type: application/json' -d "{\"username\":\"dev1\",\"password\":\"$PASS\"}" "$API/api/v1/session" |
    python3 -c 'import json,sys; print(json.load(sys.stdin).get("token",""))' 2>/dev/null || true)"
  [ -n "$tok" ] && echo "  POST /api/v1/session -> token issued" || { echo "  POST /api/v1/session -> no token"; return 0; }
  printf '  GET /api/v1/applications/lobby -> '
  api -o /dev/null -w 'HTTP %{http_code}\n' -H "Authorization: Bearer $tok" "$API/api/v1/applications/lobby"
  printf '  GET /api/v1/account/can-i/logs/get/games/lobby -> '
  api -H "Authorization: Bearer $tok" "$API/api/v1/account/can-i/logs/get/games/lobby"; echo
  pod="$(kubectl -n games get pods -l app.kubernetes.io/name=lobby-podinfo -o jsonpath='{.items[0].metadata.name}' 2>/dev/null ||
         kubectl -n games get pods -o jsonpath='{.items[0].metadata.name}')"
  printf '  GET /api/v1/applications/lobby/logs?namespace=games&podName=%s&container=podinfo&tailLines=2 ->\n' "$pod"
  api -w '\n  HTTP %{http_code}\n' -H "Authorization: Bearer $tok" \
    "$API/api/v1/applications/lobby/logs?namespace=games&podName=$pod&container=podinfo&tailLines=2&follow=false" | cut -c1-240 | sed 's/^/    /'
}

log "FROM: Argo CD chart 7.9.1 with argocd/values.yaml + overlay"
run helm install argocd "$CHARTS/argo-cd-7.9.1.tgz" -n argocd --create-namespace -f "$ENV/argocd/values.yaml" -f "$T/overlay.yaml" --wait --timeout 300s
run kubectl -n argocd get deploy argocd-server -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
log "argocd-cm and argocd-rbac-cm as rendered (RBAC-relevant keys)"
kubectl -n argocd get cm argocd-cm -o json | python3 -c 'import json,sys; d=json.load(sys.stdin)["data"]; [print(f"  argocd-cm {k}: {d[k]!r}") for k in sorted(d) if "rbac" in k or k.startswith("accounts") or k=="admin.enabled"]'
kubectl -n argocd get cm argocd-rbac-cm -o json | python3 -c 'import json,sys; d=json.load(sys.stdin)["data"]; [print(f"  argocd-rbac-cm {k}: {d[k]!r}") for k in sorted(d)]'

log "the games AppProject (argocd/apps.yaml) and the lobby Application (stand-in source)"
helm template argocd-apps "$CHARTS/argocd-apps-2.0.2.tgz" -n argocd -f "$ENV/argocd/apps.yaml" > "$T/apps.yaml"
python3 - "$T/apps.yaml" "$T/apps-obs.yaml" <<'EOF'
import sys, yaml
out = []
for d in yaml.safe_load_all(open(sys.argv[1])):
    if not d: continue
    if d["kind"] == "AppProject":
        d["spec"]["sourceRepos"].append("https://stefanprodan.github.io/podinfo")
        print("  AppProject/games: sourceRepos += https://stefanprodan.github.io/podinfo (stand-in source)")
    if d["kind"] == "Application":
        if d["metadata"]["name"] != "lobby": continue
        d["spec"]["source"] = {"repoURL": "https://stefanprodan.github.io/podinfo", "chart": "podinfo", "targetRevision": "6.7.1",
                               "helm": {"releaseName": "lobby"}}
        print("  Application/lobby: source -> podinfo chart 6.7.1 (project games, destination games, sync policy kept)")
    out.append(d)
yaml.safe_dump_all(out, open(sys.argv[2], "w"), sort_keys=False)
EOF
run kubectl apply -f "$T/apps-obs.yaml"
for _ in $(seq 1 60); do
  [ "$(kubectl -n argocd get application lobby -o jsonpath='{.status.health.status}' 2>/dev/null)" = Healthy ] && break; sleep 3
done
run kubectl -n argocd get application lobby -o jsonpath='{.status.sync.status} {.status.health.status}{"\n"}'
run kubectl -n games get pods
as_dev1 "FROM v2.14.11"

log "UPGRADE: helm upgrade to chart 8.2.7 (v3.0.12) with the same values"
run helm upgrade argocd "$CHARTS/argo-cd-8.2.7.tgz" -n argocd -f "$ENV/argocd/values.yaml" -f "$T/overlay.yaml" --wait --timeout 300s
run kubectl -n argocd get deploy argocd-server -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
run kubectl -n argocd rollout status statefulset/argocd-application-controller --timeout=180s
kubectl -n argocd get cm argocd-cm -o json | python3 -c 'import json,sys; d=json.load(sys.stdin)["data"]; [print(f"  argocd-cm {k}: {d[k]!r}") for k in sorted(d) if "rbac" in k]'
sleep 10
as_dev1 "TO v3.0.12"
