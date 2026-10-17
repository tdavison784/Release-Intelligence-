# Lane `groundtruth5` — sealed held-out ground truth, authored blind — Opus

Branch `p5ll/groundtruth5`, worktree `.claude/worktrees/p5ll-groundtruth5`. **Your branch is sealed**: it is not
merged until the code freeze (PROTOCOL §6 step 7). Do not read Phase 5 lane branches, briefs other than this one
and `_common.md`/FLEET.md/PROTOCOL.md, or `docs/phase4/env-impact/EVAL-REPORT.md`; do not run Release
Intelligence on your cases. You own `eval/investigations-heldout/` (same layout as
`eval/investigations/`, read `eval/investigations/FORMAT.md` and README.md; you may read the existing Phase 4
cases' `env/` and `case.yaml` for format only).

## Build

Realistic GitOps environments (Argo CD / Flux / helmfile / Kustomize mixes, like the Phase 4 cases) whose
upgrade outcome you establish **from upstream sources and an observed upgrade in kind**, never from pipeline
output. At least 8 cases, at least 4 expected affected and 3 not affected at the decisive item:
- ≥ 4 for cert-manager 1.17.2 → 1.18.0, varying where and how private-key and revision-history settings are set
  (or not) and what consumes the issued material — including at least one where an explicit setting looks like
  a mitigation but does not prevent the impact, and one where the relevant setting is applied through a layer
  the environment snapshot can only partly see.
- ≥ 2 for ingress-nginx 4.11.x → 4.12.0 and ≥ 1 for kube-prometheus-stack (same transitions as Phase 4), with at
  least one behavioural break not visible in rendered manifests and one harmless render difference.
- Each case: `case.yaml`, `env/`, `expected.yaml` (per-item class + basis), `NOTES.md` (upstream citations,
  observed procedure and result, kind version), `observed/` (logs). Items at a granularity the upstream notes
  support; when one note covers several keys, say so.
Clusters `ri-groundtruth5-*`, always deleted.

## Done

Commit on your branch only; `LANE DONE:` with the case list and per-case decisive item **class counts only**
(no per-case labels in the message — the commander must not see them before the freeze).
