# customer-repo
A miniature customer repository fixture for `ri impact --repo` (repo mode).
It is deliberately realistic: values files per cluster, base manifests with
Helm metadata and a CRD-shaped resource, an Argo CD Application with inline
values, a Flux HelmRelease with a version range, a kustomization overlay, a
helmfile release, a vendored chart that must be skipped, a workflow with
image references, and a Terraform file with one literal image assignment.

Every fact in it is asserted by `internal/env/repo_test.go` and joined
against the recorded cert-manager edge by the app-level repo-mode e2e.
