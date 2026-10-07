# Lane `render` — rendering validation (RENDER-MISSION.md)

> Replaces the earlier render brief (reset 2026-10-01 at the product owner's request; no earlier render
> work survives). Normative mission: `docs/phase3/learning-loop/RENDER-MISSION.md` (Goals R1–R19 below
> refer to it).

Read in order: `FLEET.md` (binding), `MISSION.md`, `RENDER-MISSION.md`, `briefs/_wave1-common.md`,
`DESIGN.md` (normative contract — especially §1.3 the `rendered-change` op, §2.3 the `rendered-diff`
validator, §4 trust ladder, §6 routing, §7 measurement), `UNKNOWN-ANALYSIS.md` §3, then the code:
`internal/domain/semantic.go`, `internal/domain/evidence.go` (`Evidence.Render` already exists — build on
it), `internal/knowledge/api.go`, `docs/ARTIFACTS.md` + `internal/helm/package.go` +
`internal/ingest/contents.go` (published chart packages are already fetched and cached by ingest — reuse
them; resolving the source/target artifact goes through the existing artifact resolution, not new
fetch logic), `internal/env` (repo mode detects Helm/Argo/Flux/Helmfile/kustomization installs; YAML
stream decoder; values files; secret withholding in `resources.go`).

Branch `p3ll/render`, worktree `.claude/worktrees/render`. Owns: `internal/render` (new),
`docs/RENDER.md` (new), `cmd/ri` render flags/commands, `eval/render/` (new, render evaluation cases),
additive domain fields (mark `// CONTRACT-CHANGE(render): why`, list in status).

## Concurrent lanes you integrate with (do not edit their packages)

| Lane | What they own | Your seam |
|---|---|---|
| `validate` | `internal/semvalidate` (CRD/values/compat/image validators) | you implement the `rendered-diff` validator in **your** package against `knowledge.Validator`; it registers alongside theirs |
| `applicability` | `internal/impact` condition evaluation | they define a `RenderedChangeEvaluator` interface in `internal/impact` (default: unknown "render unavailable"); you provide the implementation in `internal/render`; wiring in `cmd/ri`/`internal/app` behind `--render` |
| `knowledge` | store, routing, auto-verify, metrics | auto-approval policy (R10) is routing data; propose the rule + metrics fields via status, the commander routes contract changes |
| `semantic` | prompts / proposals | expose release-level render evidence they can cite in prompts (`EdgeRenderedChanges`-style function returning `domain.Evidence`) |
| `dashboard` | `internal/reviewui` | add render evidence to `knowledge.ReviewContext` (additive field); the dashboard lane renders a "Rendered delta" panel from it |
| `groundtruth` | `eval/cases` | yours are separate: `eval/render/` |

## Build

1. **Renderers (R1, R2, R13, R14).** A `Renderer` port with `helm` (`helm template`, external binary;
   v3.16 installed) and `kustomize` (`kustomize build`, or `kubectl kustomize` — kubectl 1.34 is
   installed, standalone kustomize is not) adapters. No Helm/Kustomize Go SDK dependency. Fixed release
   name/namespace (from the customer's install metadata when known: Argo Application, Flux HelmRelease,
   Helmfile), `--kube-version`/`--api-versions` from the environment where supplied, `--include-crds`, no
   cluster access (`lookup` is empty — recorded as a limitation). Full `RenderProvenance` (R1/R2/R14
   field lists) and a content-addressed render cache keyed on it. Every failure class in R13 becomes an
   explicit `RenderResult{Status: failed, Reason}` → `UNKNOWN / render-failed` evidence gap; never "no
   change". Kustomize: pick the overlay(s) tied to the environment under analysis (Argo/Flux `path`,
   repo conventions), not every directory; record the choice and why.
2. **Customer configuration first (R7, R8).** Values from `--values`, repo-mode detected sources (Argo
   CD Helm values/valuesObject, Flux HelmRelease values/valuesFrom-local, Helmfile values, repo-local
   files, Kustomize overlays). Counterfactual variants only when a specific candidate change asks for one
   (R8 example), bounded and recorded with the question they answer.
3. **Resource model + semantic diff + noise normalization (R3, R4, R15).** Stable object identity
   `<group>/<version>/<kind>/<namespace>/<name>` (track api-version-changed as the same object), the R4
   change classes, list identity by `name` where Kubernetes merges by name (containers, env, ports,
   volumes), RBAC rules compared as permission sets (apiGroup × resource × verb) so reorderings are not
   changes, ordering kept where semantics depend on it (args). Paths use the env/CRD syntax
   (`spec.template.spec.containers[name=controller].args[]` or the repo's `[]` convention — choose one,
   document it, keep it compatible with `resource-field`).
4. **Evidence (R5).** `rendered-diff` evidence records referencing renderer, both artifacts + digests,
   values digest, object, path, before/after. Release-level (chart defaults) renders may back
   `VerifiedFact`s; **environment renders contain customer values and must never enter `knowledge/` or a
   prompt** (test it, like `TestPromptNeverSendsManifestValues`); render text shows paths/summaries unless
   `--show-values`.
5. **Validation of interpretations (R6, R12).** The `rendered-diff` validator maps R6 relations onto the
   contract: `confirmed-by-render` → `confirmed`, `contradicted-by-render` → `refuted`,
   `not-visible-in-render` / `render-not-applicable` → `inconclusive` with the relation recorded (additive
   field). Renderability (`render-verifiable | partially | not-render-verifiable`) per subject family ×
   change type as data, attached to candidates/validations.
6. **Applicability (R9, R11).** Implement the `rendered-change` evaluation for the applicability lane:
   true only with the customer render delta as environment evidence; false only when both renders
   succeeded with complete values; otherwise unknown with reason. A render delta alone never yields ACTION
   (R11) — that is already the trust ladder; add adversarial tests proving it (RBAC verb removed, no
   verified consequence ⇒ at most REVIEW).
7. **Auto-approval (R10) — honest version.** Render confirmation is deterministic verification of the
   *subject and change* (and canonical applicability) aspects. Two-model agreement on the remaining
   aspects is **consensus**, not human verification: the commander is adding a `consensus` verification
   level to the contract (treated like proxy by the trust ladder: ≤ REVIEW, never clears) so
   "APPROVED" knowledge can be minted automatically for the R10 classes without weakening ACTION
   REQUIRED. Write the auto-approval policy as data (classes eligible, required agreement, required render
   relation) and record every auto-approval so R19's question ("does consensus + render match human
   judgement?") can be measured by sampling auto-approved facts into human review.
8. **CLI + workflow.** `ri impact … --repo … --kubernetes … --render` (minimum workflow), and
   `ri render diff <product> <from> <to> [--values …|--repo …] [-o json]` for direct inspection, with each
   rendered change correlated to the UpgradeEdge changes that restate it and a list of rendered changes
   that **no changelog entry mentions** (undocumented changes → review items).
9. **Evaluation (R16, R17).** `eval/render/` cases with expected render deltas authored **before**
   running your renderer on them, from upstream sources (chart template diffs between tags via git/gh,
   upstream docs) — RBAC, images, resources, env vars, args, Service ports, API-version migrations, Helm
   default changes, values-driven behaviour, a Kustomize overlay. Metrics: render success rate, render diff
   precision/recall vs those cases, proposals confirmed/contradicted, UNKNOWN → decided due to render,
   ACTION strengthened, false ACTION delta, applicability before/after on the Phase 3 corpus. Report
   honestly in your status file and `docs/RENDER.md`.
10. **Tests** offline: a tiny checked-in chart at two versions + a kustomize base/overlay in testdata,
    rendered for real when the tool is on PATH (skip with a clear message otherwise), plus golden render
    streams so diff/normalization logic is tested without tools. Live runs on at least cert-manager
    v1.17.0 → v1.18.0 (with `internal/env/testdata/customer-repo` and the eval fixture values) and one more
    product, reporting what the render confirms or contradicts in the changelogs.

Commit in this order so others can integrate early: (a) renderer + model + diff + CLI, (b) evidence +
validator, (c) the `rendered-change` evaluator, (d) auto-approval policy + metrics, (e) eval cases.
After (c), tell the commander (`RENDER EVALUATOR READY`) and keep going.

Model: Opus — this lane carries design judgement (variants, normalization semantics, auto-approval
boundaries).
