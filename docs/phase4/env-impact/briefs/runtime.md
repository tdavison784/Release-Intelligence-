# Lane `runtime` — observed-level validation in a throwaway kind cluster (MISSION §A5 last bullet, P4) — Opus

Branch `p4ei/runtime`, worktree `.claude/worktrees/p4ei-runtime`. Read `_wave2-common.md` (and
`_wave1-common.md`) first. Requested by the product owner on 2026-10-16.

## Why

Many breaking changes are behaviour defaults compiled into the new controller binary: they are invisible
in render diffs, so the only deterministic clearance is a trusted pinned setting (OverrideInspection), and
nothing deterministic can *confirm* them as affected. An isolated runtime upgrade can: it yields
`ValidationRecord`s of kind `runtime` at level **observed** (DESIGN §2.6, §3 R1/R2 already accept them).

## Owns

- `internal/investigate/runtime/` (new): an `investigate.Validator` (`runtime.New(opts) `), its cluster
  driver port + a `kind`/`kubectl` implementation, and fakes for tests.
- An additive, declarative `exercises:` construct in product definitions if needed (internal/catalog +
  schema + validator), authored only from upstream docs with a citation per entry.
- `docs/INVESTIGATE.md` section "Runtime validation"; `docs/phase4/env-impact/status/runtime.md`.

## How it must work

1. Input: an `EvidenceBundle` — the environment's complete FROM and TO renders are already stored as
   artifacts by renderdiff (apply those; no Argo CD/Flux needed), plus the relationship graph / impact
   paths and the in-scope items. Only `ok` render pairs are usable; anything else ⇒ `not-run` with reason.
2. Create `kind` cluster `ri-runtime-<short digest>` (pin the node image to the environment's Kubernetes
   version where the bundle states one), apply FROM (CRDs first), wait for readiness, take a **baseline
   observation**, apply TO (the upgrade), run declared **exercises**, wait a bounded settle window, take
   the **after observation**, delete the cluster. Delete it on every exit path (error, timeout,
   cancellation) and verify with `kind get clusters`. One cluster at a time; refuse to start when free
   disk is below a threshold (record it).
3. Observations are **generic** (no product Go code): per object on the item's impact paths (exposed
   objects + consumers): apply/admission errors, existence, status conditions, warning events, pod
   readiness/restarts, generation/observedGeneration, referenced Secret/ConfigMap data **digests** (never
   values), and controller log lines that name the object. Diff baseline vs after.
4. Exercises: some breaks only manifest when something happens (a renewal, a reconcile, a request).
   Triggers are **declared** per product (`exercises:` — e.g. a status-condition patch or annotation that
   upstream documents as the way to trigger an action, a rollout restart), linked to the change families
   / object kinds they exercise. Never inferred from eval cases.
5. Outcomes per item (all level `observed`, citing the observation artifacts by digest, stored in the
   bundle's artifact store):
   - `fail` — an object on the item's path degrades after the upgrade (rejected, condition → False,
     warning event, not ready, deleted, referenced data changed under a consumer) and nothing in the
     baseline explains it; name the concrete failure.
   - `pass` — only when the upgrade completed, every object on the item's paths was observed healthy
     before and after, **and** the exercise(s) declared for the item's family/kinds ran. A quiet window
     without the relevant exercise is `not-run` ("no exercise declared for …"), never `pass`.
   - `error` / `not-run` — image pull failures, timeouts, missing tools/docker, incomplete renders,
     unsupported workloads (e.g. needs external cloud), with reasons.
6. Opt-in only (`--runtime`); never part of default runs. Bounded wall-clock (default ≤ 10 min per
   investigation, configurable); report duration. Images pulled stay in Docker's cache (fine).

## Integrity

Never open `eval/investigations/*/{expected.yaml,NOTES.md,observed/}` — the groundtruth `observed/run.sh`
scripts contain the answers. Use only `input.yaml` and `env/`. Exercises come from upstream docs.

## Done

Unit tests with a fake cluster driver for every outcome path incl. "quiet window without exercise ⇒ never
pass", cleanup on every exit path, and observation diffing; one **real** run per product (cert-manager,
ingress-nginx, kube-prometheus-stack) on one groundtruth `input.yaml` each, results summarised in your
status file (what was observed, outcome per item — not whether it matches ground truth); all clusters
deleted; build/vet/test green; status; `LANE DONE:`.
