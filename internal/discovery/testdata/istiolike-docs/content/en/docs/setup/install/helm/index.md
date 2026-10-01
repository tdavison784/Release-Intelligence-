# Helm

helm repo add mesh https://mesh-release.example.com/charts
helm install mesh-base mesh/base
helm install meshd mesh/meshd
helm install ambient oci://ghcr.io/meshproj/release/charts/ambient
