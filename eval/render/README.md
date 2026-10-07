# Render evaluation cases (`eval/render/`)

Separate from `eval/cases/` (the groundtruth lane): these cases measure the
**renderer and the rendered diff**, not the whole pipeline. Same discipline (R16):
expectations are authored **before** running the renderer on the case, from upstream
material — the chart template/CRD diff between the two tags (`git diff <from> <to> --
deploy/...`), values.yaml/values.schema.json, release notes and upgrade docs — never
from Release Intelligence output. Each case's `NOTES.md` records the sources read, the
judgement calls, and any way authoring was not fully blind.

```
eval/render/<case-id>/
  case.yaml            # expectations over the rendered delta (format below)
  NOTES.md             # sources, judgement calls, blind-authoring caveats
  environment/         # optional values files for variants (values-driven behaviour)
eval/render/results/   # committed live comparisons (never touched by tools;
                       # written by hand from a live run, after authoring)
```

## case.yaml

```yaml
id: cert-manager-1.17-1.18
product: cert-manager          # products/ definition id
from: v1.17.0                  # tags as published
to: v1.18.0
researchedAt: "2026-10-01"
level: release                 # release = chart-default render (the rendered-diff
                               # validator's level); environment variants below
sources: [ ... ]               # every upstream document/tree consulted
expected:
  - id: R1
    category: image            # image | rbac | resource | port | field | arg | env |
                               # crd | helm-default | api-version | not-render-verifiable
    expect: present            # present = the renderer must show such a change;
                               # absent = it must not
    kind: Deployment           # object kind
    name: cert-manager         # object name: substring match (glob * allowed)
    class: image-changed       # optional: the diff class (see internal/render/diff.go);
                               # default = every class the category maps to
    path: ""                   # optional substring on the change path
    detail: controller image v1.17.0 → v1.18.0
    source: >-                 # the upstream fact this rests on (quote or summarize)
      chart appVersion v1.18.0
variants:                      # optional: render again with these values files
  - id: servicemonitor
    values: environment/values-servicemonitor.yaml
    expected: [ ... ]          # same shape, evaluated against that render pair only
```

A `present` expectation matches when ANY rendered change satisfies all given fields
(kind, name, class, path); `absent` when none does. Names match exactly (glob `*`
allowed), paths by substring. Expectations are authored at upstream-source
granularity — object names and field areas the templates actually contain — not at
the renderer's path spellings. An expectation the authoring sources cannot decide
marks itself `knownGap: true`: a miss is reported in the results, not failed.

## Metrics (R17, render-scoped)

Per case, from one live run: **render success rate** (pairs rendered / pairs
attempted), **precision** (rendered changes matched by some expectation / all rendered
changes, counted per change), **recall** (expectations matched / all `present`
expectations), plus the failure reasons of failed pairs. The pipeline-level metrics of
R17 (UNKNOWN → decided due to render, ACTION strengthened, false ACTION delta,
applicability before/after) need the applicability lane's wiring and are reported in
the lane status once that lane lands — the render lane records the render-scoped ones
in `results/` and `docs/RENDER.md`.

The comparison runner is `go test ./internal/render -run TestEvalRenderCases -v`
(skips cleanly offline or without helm: a case's charts must be fetchable). It reads
`eval/render/` relative to the repository root and never writes to it.
