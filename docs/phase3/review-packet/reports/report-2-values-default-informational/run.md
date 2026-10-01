# Report 2 — changed default correctly marked INFORMATIONAL (cert-manager v1.17.0 → v1.18.0, in-range cluster)

- **Upgrade:** cert-manager v1.17.0 → v1.18.0 (the same upgrade as report 1 — deliberately, so the only thing that changes is the environment)
- **Environment:** Kubernetes **1.31** (inside v1.18.0's supported range 1.29–1.33), one Helm values file that explicitly pins `prometheus.servicemonitor.targetPort`, two pinned images. No manifests, no CRDs supplied.
- **Expected headline:** 0 ACTION REQUIRED, 3 REVIEW REQUIRED, 2 INFORMATIONAL (cluster in range; pinned default does not apply), 3 NOT AFFECTED, 50 UNKNOWN
- **Run mode:** fully offline (recorded upstream replay). No network, no API keys, no AI step.

## Why this report is in the packet

The same upstream default change (`prometheus.servicemonitor.targetPort`:
9402 → "http-metrics") that would silently alter an un-pinned install is
classified **informational** here because this environment pins the value —
the tool claims "applies to you, you appear safe". This is the case most likely
to be wrongly marked ACTION REQUIRED, so the reviewer's judgment on it is the
key false-action datapoint.

## Files

| File | What it is |
|---|---|
| `report.txt` | the rendered report a user sees |
| `report.json` | the same report as JSON, including every evidence record |
| `../../example-env/report2-env/` | the exact environment inputs (`values.yaml`, `images.txt`) |

## Exact command

Run from the root of the repository (build first with `go build -o bin/ri ./cmd/ri`):

```
./bin/ri -offline -state <recorded-state-dir> impact cert-manager v1.17.0 v1.18.0 \
  --kubernetes 1.31 \
  --values docs/phase3/review-packet/example-env/report2-env/values.yaml \
  --images docs/phase3/review-packet/example-env/report2-env/images.txt \
  -o text
```

`<recorded-state-dir>`: see `../reproduce.sh`, which assembles it and re-runs
this report byte for byte.

## Honesty notes

- Because no manifests or CRDs are supplied here, everything that would be
  decided from them lands in UNKNOWN with `neededToDetermine` saying exactly
  that. That is intentional: the reviewer should judge whether UNKNOWN is
  honest.
- The GitHub security-advisories source is unavailable in the recording
  (see `Warnings` at the end of `report.txt`).
