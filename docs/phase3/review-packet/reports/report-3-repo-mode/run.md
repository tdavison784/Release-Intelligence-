# Report 3 — repository mode (`--repo`): the environment is discovered from a directory

- **Upgrade:** cert-manager v1.17.0 → v1.18.0 (the same upgrade as reports 1–2)
- **Environment:** a miniature customer repository (`../../example-env/customer-repo/`) — per-cluster values files, base manifests, an Argo CD Application with inline values, a Flux HelmRelease, a kustomization, a helmfile release, a vendored chart (which must be skipped), a GitHub workflow and a Terraform file with image references, plus one installed CRD. Cluster Kubernetes **1.28** was passed explicitly (`--kubernetes 1.28`); everything else was discovered.
- **Expected headline:** 1 ACTION REQUIRED, 3 REVIEW REQUIRED, 1 INFORMATIONAL, 3 NOT AFFECTED, 50 UNKNOWN (the discovered inputs overlap the hand-supplied ones of report 1)
- **Run mode:** fully offline (recorded upstream replay). No network, no API keys, no AI step.

## Files

| File | What it is |
|---|---|
| `report.txt` | the rendered report a user sees |
| `report.json` | the same report as JSON, including every evidence record |
| `../../example-env/customer-repo/` | the repository the environment was discovered from (a copy of the fixture; every input file is listed with its sha256 in the report header) |

## Exact command

Run from the root of the repository (build first with `go build -o bin/ri ./cmd/ri`):

```
./bin/ri -offline -state <recorded-state-dir> impact cert-manager v1.17.0 v1.18.0 \
  --repo docs/phase3/review-packet/example-env/customer-repo \
  --kubernetes 1.28 \
  -o text
```

`<recorded-state-dir>`: see `../reproduce.sh`, which assembles it and re-runs
this report byte for byte.

## Honesty notes

- Repo mode states its own limits as warnings at the end of `report.txt`: the
  vendored chart is skipped on purpose, helmfile templates are read literally,
  the kustomization is inventoried without applying patches, two values files
  are both applied (last one wins per key), and a HelmRelease referencing
  `spec.valuesFrom` (cluster objects) cannot be read from files. The reviewer
  should judge whether these disclosures are sufficient.
- GitHub security advisories are unavailable in the recording (same as the
  other reports).
