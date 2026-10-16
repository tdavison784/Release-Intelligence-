# gitops-obs-south

GitOps repository for the `obs-prod-south` cluster (managed Kubernetes: the control plane is run by
the provider). Argo CD runs in the cluster and syncs `clusters/prod/` (app-of-apps); everything the
cluster runs is declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Argo CD `Application`s (the root app points here) |
| `platform/monitoring/` | Helm values for kube-prometheus-stack (`values.yaml` shared by all clusters, `values-prod.yaml` for this one) |
| `apps/checkout/` | the checkout service and its alerting rules |

Dashboards live in the central Grafana, which reads this cluster's Prometheus remotely, so the
chart's bundled Grafana is disabled.
