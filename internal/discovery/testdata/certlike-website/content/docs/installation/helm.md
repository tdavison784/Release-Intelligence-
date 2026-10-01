# Helm

```bash
helm install certmgr oci://quay.io/examplecorp/charts/certmgr --version [[VAR::latest_version]]
```

The legacy repository:

```bash
helm repo add examplecorp https://charts.example.io --force-update
helm install certmgr examplecorp/certmgr --namespace certmgr
```
