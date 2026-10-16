# Lane `values` — status

Branch `p4ei/values` (from `p4-env-impact` @ a3f62868). Brief: `briefs/values.md`.

## State

In progress. Next step: implement `internal/env/values.go` (per-release stacks), wire Load, scope to
the product (`Inputs.Product`), tests.

## Design (decided)

- `env` builds one `ValuesRelease` per deployment target (Argo CD chart source, Flux HelmRelease,
  helmfile release, repo values file not referenced by any release, explicit `--values` group), each with
  its ordered layers exactly as the tool applies them and the effective keys (Helm merge semantics,
  per-key provenance = file/locator + line of the layer that set it). Never merged across releases.
- `Inputs.Product` scopes `ValuesKeys` + the values dimension health to the product's release(s).
- Sources (cited): Argo CD docs "Helm Value Precedence": `parameters > valuesObject > values >
  valueFiles`, "valuesObject trumps values - therefore values will be ignored", "the last file listed has
  the highest precedence", plain paths relative to the chart root. Flux HelmRelease docs: valuesFrom
  "merged in the order given", inline values overwrite them, "When targetPath is set, it will overwrite
  everything before, including inline values".
