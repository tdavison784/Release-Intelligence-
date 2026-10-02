# ingress-nginx-like

An ingress controller fixture with two version trains: the controller train
(`controller-v1.x.y`) and the chart train (`helm-chart-4.x.z`), like
kubernetes/ingress-nginx.

## Supported Versions table

Supported versions mean E2E tests pass for the versions listed.

| Supported | Ingress-nginx version | k8s supported version |
|:---------:|-----------------------|------------------------|
| 🔄 | **v1.15.1** | 1.32, 1.33, 1.34 |
| 🔄 | **v1.15.0** | 1.32, 1.33, 1.34 |
| 🔄 | **v1.14.0** | 1.31, 1.32, 1.33 |
| 🔄 | **v1.13.0** | 1.30, 1.31, 1.32 |

## Installation

Without Helm, apply the CRD bundle and the cloud manifest:

```
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx-like/controller-v1.15.1/deploy/crds/bundle.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx-like/controller-v1.15.1/deploy/static/provider/cloud/deploy.yaml
```
