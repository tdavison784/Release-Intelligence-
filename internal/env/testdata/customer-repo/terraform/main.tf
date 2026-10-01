# Terraform manages some tooling around the cluster. Repo mode extracts only
# the trivially deterministic part — literal image assignments; HCL
# structure, variables and templating are a documented gap.
locals {
  controller_image = "quay.io/jetstack/cert-manager-controller:v1.17.0" # a local, not extracted
}

resource "kubernetes_config_map" "audit" {
  metadata {
    name = "audit"
  }
}

# The one fact repo mode takes from this file: the pinned mirror reference.
image = "quay.io/jetstack/cert-manager-webhook:v1.17.0"
