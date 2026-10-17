# Lane groundtruth5 — status

Sealed held-out ground truth (brief: `briefs/groundtruth5.md`). Owns `eval/investigations-heldout/`.
Branch `p5ll/groundtruth5` (sealed until PROTOCOL §6 step 7). This file states **no per-case labels**
(the commander must not see them before the freeze); they are only in each case's `expected.yaml`.

## Done

- 10 cases under `eval/investigations-heldout/`, same layout/format as `eval/investigations/` (FORMAT.md incl.
  its additive fields): `input.yaml`, `env/`, `expected.yaml`, `NOTES.md`, `observed/` (`run.sh`, `run.log`,
  `render.diff`, `render-index.txt`). Index: `eval/investigations-heldout/README.md`.
  - cert-manager v1.17.2 → v1.18.0: telemetry-east, telemetry-west (pair), registry-apac, billing-nordics,
    search-latam (the rotation and history settings vary in where and how they are set, across Argo CD, Flux,
    helmfile and Kustomize; details only in the cases).
  - ingress-nginx 4.11.5 → 4.12.0: docs-eu, docs-us (pair, #11819 items), partner-api (#11851, #12153).
  - kube-prometheus-stack 89.2.4 → 90.2.0: obs-emea, obs-amer (pair, the 89→90 UPGRADE.md items).
- Decisive-item class counts (all 10 cases): **5 affected, 5 not-affected** (brief: ≥4 / ≥3). Brief coverage: one
  explicit setting that looks like a mitigation but does not prevent the impact (and a second, differently
  shaped one); one setting applied through a layer the snapshot only partly sees; a behavioural break with no
  rendered counterpart; harmless render differences.
- Every case observed in kind v0.24.0 / Kubernetes v1.31.0 (`kindest/node:v1.31.0`), helm v3.16.1, charts from
  the upstream repos; both versions exercised in each run. Upstream citations with line ranges and verbatim
  quotes in each NOTES.md / expected.yaml (release notes, changelog, UPGRADE.md, source at the release tags).
- Shared tooling: `observed-lib.sh` (kind, certificate, mTLS helpers), `render-diff.py` + `render-case.sh`
  (canonical per-object FROM/TO render diff), `probes/featuregate-1.17.2.log` (cert-manager v1.17.2 refuses the
  1.18 feature gate, so no environment can pre-stage it).
- `docs/INVESTIGATE.md`: short "Held-out ground truth (Phase 5)" subsection under Evaluation (no labels).

## In progress

- Nothing. Lane done (see final message).

## Decisions

- The brief says `case.yaml`; FORMAT.md (same layout as `eval/investigations/`) names the inputs file
  `input.yaml`, which is what the harness reads (`internal/eval/investigations.go`). Cases use `input.yaml`.
- Observed procedure for cert-manager: renewals triggered like `cmctl renew` (Issuing condition) stand in for the
  scheduled renewal at `renewBefore`; control renewals on v1.17.2 before the upgrade show the old behaviour; a
  server restarts after its renewal, clients are checked once the kubelet has refreshed their Secret volume.
- GitOps controllers are not installed in kind; their order is reproduced (sync waves / dependsOn) with helm and
  `kubectl kustomize | kubectl apply` (server-side where the Application uses ServerSideApply). Stated per case.
- billing-nordics: external-secrets/Vault not reproduced; the Secret they maintain is created directly from a CA
  generated per run (cert-manager only reads that Secret). Documented in its NOTES.
- Where one item's outcome masks another's observation in a run, the masked item also cites the pair sibling's
  run (same settings for that item); stated in that case's expected.yaml and NOTES.
- A failed rollout of the declared version (the cluster keeps the old objects) is labelled at the item level by
  what the environment can no longer do; the reasoning is in the case NOTES.
- `renderedChanges` are listed for every case (grouped per logical change, noise = version labels / image tags).
- Fixture fixes made after a first failed run, never after an outcome: registry-apac sync waves (Kyverno refuses
  a policy for a kind whose CRD does not exist yet); ingress runs without `helm --wait` (a LoadBalancer Service
  never gets an address in kind); partner-api creates its namespace first; telemetry helpers wait for Secret
  volume refresh and old-pod termination (the first telemetry-east run checked too early).
- Two quotes first written from memory (Kyverno mutate docs, Kubernetes VAP docs) were replaced by verbatim text
  from the upstream repositories at pinned commits before commit of the final expectations.

## Files touched outside ownership

- `docs/phase5/learning-loop/status/groundtruth5.md` (this file).
- `docs/INVESTIGATE.md`: one additive subsection under "Evaluation".

## Contract changes

- None. No new fields beyond FORMAT.md's additive ones; `mitigation.value` is sometimes a non-string scalar
  (`true`, `3`) — the Go loader does not read `mitigation`.

## Open questions (for the commander, at unseal)

- `ri eval -investigations -dir eval/investigations-heldout` needs a `gates.yaml` in that directory
  (`LoadInvGates`). Gates are commander-owned (PROTOCOL §5): copy the frozen `eval/investigations/gates.yaml`
  byte-identical or pre-register held-out gates; this lane did not add one.

## Test status

- `go build ./... && go vet ./... && go test ./...`: green (44 packages ok, 0 FAIL; no Go code changed by this lane).
- `eval/investigations-heldout` consistency check (scope ↔ items, strongest-component rule, every referenced
  file exists, diff ranges in bounds, RE2-compatible match texts): 10/10 OK.
- Cleanup: every `ri-groundtruth5-*` kind cluster deleted by its run's trap; `kind get clusters` empty.
