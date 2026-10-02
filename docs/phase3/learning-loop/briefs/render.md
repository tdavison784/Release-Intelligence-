# Lane `render` — rendered-manifest diffs (Helm / Kustomize) as validation and exposure evidence

Product owner's idea (2026-10-01): validate changelog claims against what actually changes, by
templating the configs with Helm/Kustomize at the From and To versions and diffing the rendered output.

Read `_wave1-common.md` first, then DESIGN.md §1.3 (conditions), §2.3 (validators), §4 (trust ladder),
`docs/ARTIFACTS.md` (published chart packages: `helm-repo`/`oci`/`chart-tgz` channels — the bytes are
already fetched and cached by ingest; reuse them, never fetch at validation time beyond what ingest does),
`internal/helm/package.go`, `internal/ingest/contents.go`, `internal/env` (values files, `--repo` mode
detecting Helm/Argo/Flux/Helmfile/kustomization installs).

Branch `p3ll/render`, worktree `.claude/worktrees/render`. Owns `internal/render` (new),
`docs/RENDER.md` (new), `cmd/ri` flags for it, additive snapshot/domain fields (mark
`CONTRACT-CHANGE(render)`).

## What to build

1. **Renderer port + adapters** (`internal/render`): `helm template` over the cached published chart
   package at a version with a given values set; `kubectl kustomize` (kustomize is not installed
   standalone; kubectl 1.34 embeds it — detect either) over a kustomization directory/ref. Executed as
   external tools (no helm/kustomize Go SDK dependency): fixed release name/namespace, `--kube-version`
   pinned (from the environment's cluster version, else the chart's minimum), `--include-crds`, no
   cluster access (`lookup` yields empty — record that as a limitation), deterministic sort of output.
   Record tool + version, chart digest, values digest, and flags in a `RenderProvenance` so every render
   is reproducible. Missing tool or chart ⇒ an explicit `unavailable` result with reason, never a crash
   and never "no diff".
2. **Rendered-manifest diff**: decode the rendered stream (reuse env's YAML stream decoder), identify
   objects by GVK + namespace + name, and diff From→To at field level using the same path syntax as
   env/CRD paths (`spec.template.spec.containers[].args[]`, element identity by `name` where present).
   Output: added/removed objects, changed fields (before/after, JSON-encoded), each with evidence that
   points at the template file (from Helm's `# Source:` comments) and the render provenance.
3. **Two render modes, two different uses — keep them separate:**
   - **Release-level (chart defaults)**: From/To rendered with default values only. The diff is
     release-level evidence (no environment in it). Expose it to the validate lane as a validator
     `rendered-diff` (in your package, implementing `knowledge.Validator`) that confirms/refutes proposal
     subjects+changes: RBAC rules/verbs removed (ClusterRole/Role), container args/flags and feature gates
     changed, env vars added/removed (e.g. dropped LOGGING_CONFIG-style vars), images/tags changed, probes,
     ports, securityContext, webhook configs, Service/Ingress shape. Confirm only what the render proves.
   - **Environment-level (customer values)**: From/To rendered with the customer's values files (from
     `--values` / repo mode). This diff is exactly what will change in *their* cluster — deterministic
     **exposure** evidence. Add a condition op `rendered-change` (Kind/Group/Path/State: changed |
     unchanged | removed | added; optional Values) for the applicability engine, evaluated against the
     environment render. True ⇒ evidence = the rendered object field + the values line(s) that drove it;
     false only when both renders succeeded with the customer's complete values; render unavailable ⇒
     unknown (`environment-visibility-gap` or `evidence-gap`). Coordinate the evaluator binding through the
     `applicability` lane: you implement `EvaluateRenderedChange(cond, envRender) ConditionResult` in your
     package; they call it.
4. **Secrets**: rendered output contains customer values. It never goes into an LLM prompt, never into
   `knowledge/` (facts are release-level), and is rendered to text only as paths/diff summaries unless
   `--show-values`. Add a test, as envparse did.
5. **CLI**: `ri render diff <product> <from> <to> [--values …] [-o json]` for humans to inspect
   ("see the actual diffs as they pertain to the changelog"), plus an annotation step that links each
   rendered change to the UpgradeEdge changes that plausibly restate it (deterministic: same subject
   key/path/name tokens) — and lists rendered changes that NO changelog entry mentions (undocumented
   changes are valuable findings for the review queue as `evidence-sufficiency`/`semantic-mapping` items).
6. **Tests** offline: a tiny checked-in test chart at two versions (testdata) rendered for real when
   `helm` is on PATH (skip with a clear message otherwise) plus golden rendered streams so the diff logic
   is tested without tools; kustomize via a testdata overlay. Run it live on at least cert-manager
   v1.17.0→v1.18.0 and one more product with the cached chart packages and report what the rendered diff
   confirms or contradicts in the changelogs.

Model: Sonnet.

## Addendum: rendered diffs feed the proposal prompts

The `semantic` lane will include your release-level rendered changes as citable EVIDENCE in proposal
prompts. Expose `func EdgeRenderedChanges(...)` (or similar) returning per-change annotated rendered diffs
as `domain.Evidence` records (locator = object GVK/ns/name + field path, excerpt = before → after, URI =
the chart package + template source) — release-level only.
