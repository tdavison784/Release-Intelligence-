# Report 1 — compatibility ACTION REQUIRED (cert-manager v1.17.0 → v1.18.0, cluster below the supported range)

- **Upgrade:** cert-manager v1.17.0 → v1.18.0 (51 upstream changes analyzed)
- **Environment:** Kubernetes **1.28** (below v1.18.0's supported range 1.29–1.33), a Helm values file, two workload manifests, one installed CRD, two pinned images
- **Expected headline:** 1 ACTION REQUIRED (cluster version), 3 REVIEW REQUIRED, 1 INFORMATIONAL, 3 NOT AFFECTED, 50 UNKNOWN
- **Run mode:** fully offline (recorded upstream replay). No network, no API keys, no AI step.

## Files

| File | What it is |
|---|---|
| `report.txt` | the rendered report a user sees |
| `report.json` | the same report as JSON, including every evidence record (upstream + environment) |
| `../../example-env/report1-env/` | the exact environment inputs the report was joined against |

## Exact command

Run from the root of the repository (build first with `go build -o bin/ri ./cmd/ri`):

```
./bin/ri -offline -state <recorded-state-dir> impact cert-manager v1.17.0 v1.18.0 \
  --kubernetes 1.28 \
  --values docs/phase3/review-packet/example-env/report1-env/values.yaml \
  --manifests docs/phase3/review-packet/example-env/report1-env/manifests \
  --crds docs/phase3/review-packet/example-env/report1-env/crds \
  --images docs/phase3/review-packet/example-env/report1-env/images.txt \
  -o text
```

`<recorded-state-dir>` is a directory containing the recorded upstream cache. For
this packet it was produced by `../reproduce.sh`, which assembles the state
directory from the repository's recorded offline fixtures
(`internal/app/testdata/e2e/state/cache` — HTTP responses and git results
recorded from the real upstreams) and runs every report again, byte for byte.
You do not need to run anything to review this report; the command is here so
you can.

## Honesty notes

- The run replays a recording of the real upstream sources. Two sources could
  not be recorded and are reported as unavailable (see the `Warnings` section
  at the end of `report.txt`): GitHub security advisories, so advisory
  matching is incomplete in this report.
- `generatedAt` in the JSON follows the recorder's fixed clock; re-running
  produces byte-identical output apart from that field (verified).
