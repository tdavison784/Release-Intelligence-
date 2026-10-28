# gitops-studio-east

Delivery repository of the `studio-prod-east` cluster (game backend). Argo CD is installed and
upgraded by the platform team with helmfile from this repository (`helmfile.yaml`); Argo CD then
syncs the game services from `apps/`.

| Path | What |
|---|---|
| `helmfile.yaml` | the `argocd` release (argo-cd chart) and the `argocd-apps` release (projects and Applications) |
| `argocd/values.yaml` | Argo CD configuration: SSO (Okta), RBAC |
| `argocd/apps.yaml` | the `games` AppProject and the `lobby` and `matchmaker` Applications |
| `apps/lobby/`, `apps/matchmaker/` | the game services |

People sign in with Okta. Members of the `studio-devs` group are the game developers: they follow
their services in the Argo CD UI, open pod logs when they debug, and sync. `studio-platform` members
administer Argo CD.
