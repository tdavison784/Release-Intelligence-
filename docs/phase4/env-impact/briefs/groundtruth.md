# Lane `groundtruth` — known-outcome investigation cases (blind) — MISSION AC5, P6

Model: Opus. Branch `p4ei/groundtruth`, worktree `.claude/worktrees/p4ei-groundtruth`.

You build the ground truth the whole Phase 4 system is measured against. Integrity matters more than
volume. Read first: `docs/phase4/env-impact/MISSION.md`, `docs/phase4/env-impact/FLEET.md`
("Evaluation integrity" is binding for you above everything else), `eval/FORMAT.md`, and
`docs/phase3/learning-loop/briefs/groundtruth.md` + `eval/cases/*/NOTES.md` (how blind ground truth was
authored in Phase 3). Do **not** run Release Intelligence's impact/investigation pipeline on your
cases, and do not consult its output, while authoring expectations.

## Owns

- `eval/investigations/<case>/` for every case you create (fixtures + expectations + NOTES.md)
- `eval/investigations/README.md` (index of cases)
- `docs/phase4/env-impact/status/groundtruth.md`

## What a case is

A dependency upgrade (product, from → to, within the first-milestone scope: Helm-chart-delivered
Kubernetes add-ons such as cert-manager, ingress-nginx, kube-prometheus-stack, argo-cd) applied to a
**specific GitOps environment** you author as a small, realistic fixture repository:

- `env/` — a GitOps repo: Argo CD `Application` or Flux `HelmRelease` for the add-on, Helm values,
  Kustomize base + overlays (at least one overlay that patches something relevant), and the
  **consuming workloads** (e.g. `Certificate`/`Issuer`/`Ingress` objects, apps that mount generated
  Secrets, ServiceMonitors, apps relying on an annotation) so that impact has to be traced through
  relationships, not just read off the values file.
- `expected.yaml` — per breaking change item in scope and per affected component: the classification
  (`affected` / `not-affected`; use `potentially-affected` / `insufficient-evidence` only when that is
  genuinely the correct answer for the provided inputs), the **impact path** (exposed component →
  consumer, by file and object), the mitigating override when not affected, and citations.
- `NOTES.md` — how the expectation was established: upstream citations (release notes, upgrade guide,
  chart diff, source commit, with URLs + line ranges) and the **observed outcome**.

## Known outcomes (AC5: "verified against known outcomes")

Wherever feasible, confirm each expectation by actually doing the upgrade in an isolated cluster:
`kind create cluster --name ri-groundtruth-<case>`, install the `from` version with the case's
rendered environment (`helm template` / `kubectl kustomize`), apply the consumers, upgrade to `to`,
and record what observably happens (rendered diff, admission/apply errors, controller logs, resource
status). Record commands and the relevant output excerpts in NOTES.md and save raw outputs under
`<case>/observed/`. Delete clusters when done. If images can't be pulled or a runtime check is
impossible, say so and fall back to a render-level outcome (`helm template` before/after diff), labelled
as such. Never invent an outcome.

## Required set (minimum) — pairs make the test sharp

Build **pairs**: the same upgrade and the same upstream breaking change, two environments: one where it
is breaking (affected) and one where an environmental override, an unused feature, or a relationship
that isn't present makes it non-breaking (not affected). Minimum: 2 pairs across 2 different products
(4 cases). Stretch: a third pair whose impact is only visible through a transitive relationship
(e.g. a Kustomize overlay patch, or a consumer two hops away), and one case where the honest answer is
`insufficient-evidence` because a needed input is deliberately absent.

Choose breaking changes from upstream sources, not from Release Intelligence output (candidates you may
evaluate: cert-manager 1.17→1.18 private-key rotation-policy default / ACME HTTP01 PathType; ingress-nginx
annotation/snippet defaults; kube-prometheus-stack CRD or selector changes). Verify every claim against
the upstream source.

## Format

`eval/investigations/FORMAT.md` is owned by the `contract` lane, which is designing it in parallel. Start
with your own draft format and **write it at the top of your status file early (within your first hour)**
so the contract lane can converge on it; adopt FORMAT.md when it lands (the commander will tell you to
`git merge p4-env-impact`). Keep the expectations themselves yours.

Done = cases committed with NOTES.md citations and observed outcomes, README index, status file current,
`go build ./... && go vet ./... && go test ./...` still green, final message `LANE DONE:`.
