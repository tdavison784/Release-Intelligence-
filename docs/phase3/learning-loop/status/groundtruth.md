# Lane `groundtruth` — status

**State: lane brief complete** (all six steps; GLM-5.3 handoff window — see "GLM handoff log").

## Done
- Format (step 1): `internal/eval/labels.go` (new) — `semantics` per expected item, `exposure` / `overlap` /
  `environmentEvidence` per link, `environment.undecidedImpact`; strict decoding into the domain types;
  validation. `Case.TransferOf` (transfer environments, G12) in `case.go` + `transfer.go`. Tests:
  `labels_test.go`. Docs: `eval/FORMAT.md` ("Semantic labels", "Transfer environments").
- Labels + corrections (steps 2–3, commit 4ee35f7): `semantics` + `exposure` + `environmentEvidence` on
  the 21 existing links, upstream-grounded corrections recorded in `eval/CHANGELOG.md` (pre/post + why).
  D1 left as-is per brief.
- Sub-briefs for hand-off authoring: `briefs/groundtruth-newcase.md`, `briefs/groundtruth-transfer.md`.
- New cases (step 5): seven — crossplane-1.20-2.0, external-secrets-0.15-0.16, flux-2.6-2.7,
  kyverno-1.12-1.13, loki-2.9-3.0, prometheus-operator-0.85-0.86, traefik-2.11-3.0. Each: 8–10 items
  with full semantics, realistic fixture + inventory.yaml, ≥5 expectedImpact links incl. ≥2 not-affected,
  undecided links where honest. Authored blind by sub-agents per the new-case sub-brief; dataset tests
  green before commit. G22 categories covered: prose-only defaults, feature gates, cross-product version
  dependencies, RBAC, migrations, behaviour changes, deprecations, required new config, compatibility in
  prose.
- Transfer environments (step 4, G12): four sibling cases `<base>--<suffix>` with `transferOf`, authored
  blind by sub-agents per the transfer sub-brief, reviewed and committed:
  - `cert-manager-1.16-1.17--eu-platform` (EU platform staging, single-tier private PKI; 2 affected /
    2 not-affected / 1 undecided)
  - `cilium-1.16-1.17--plant-edge` (manufacturing edge; WireGuard userspaceFallback, BGP CP,
    external workloads; 4 / 2 / 1)
  - `karpenter-0.37.8-1.0.0--ci-buildfarm` (GPU CI build farm; v1beta1 manifests, old disruption taint,
    drift opt-out; 5 / 3 / 1)
  - `strimzi-0.45-0.46--edge-retail` (retail edge; KRaft, MirrorMaker 1, EnvVar provider; 4 / 3 / 1)
- One eval run (step 6), after all expectations were committed; first numbers below, nothing tuned.

## First eval numbers after this lane's dataset expansion (run once, 2026-10-01, no tuning)

28 entries (0 pipeline failures): expected 180, found 171, missed 9 (critical 1, important 5, minor 3).
recall 0.95 (critical 0.98, important 0.95), raw precision 0.73, labeled precision 0.58
(true 190 / false 140, adjudicated). classification accuracy 0.46 (112 scored), false-action rate 0.23
(3/13), unknown rate 0.80, applicability accuracy 0.39. environment (19 entries): impact links 11/73
hit (accuracy 0.15), findings 25/26, false findings 0. Gates: criticalRecall ✓ 0.980, importantRecall ✓
0.950, pipelineFailures ✓ 0, applicabilityAccuracy ✗ 0.390 < 0.80, falseActionRate ✗ 0.231, 
actionFindingEvidence ✗ 0.923, unsupported ✗ 11. Baseline for comparison (main @ c341555 per FLEET.md):
all gates passed except applicabilityAccuracy 0.095 (2/21) — so recall gates hold on the enlarged
dataset, applicability moved 0.095 → 0.39, and the falseActionRate / actionFindingEvidence /
unsupported gates now fail on the new, stricter case set (new cases score item classes and findings the
old 21-link set did not). Reported as-is per the integrity rules.

- **Regression vs stored results (finding, not hidden):** `karpenter-0.37.8-1.0.0 unsupported: 0 → 11`.
  The stored snapshot predates this lane's re-labelling of that case (4ee35f7); the run correctly refuses
  to accept silently. Commander decision needed: review, then `ri eval -update` (or leave until the
  capture lane re-snapshots). I did not run `-update` (it rewrites `eval/results/*`).
- **Base-label concerns reported by transfer authors (recorded in each transfer NOTES.md, NOT edited):**
  - cert-manager base F1 cites a "README support table" for v1.17's k8s range; no such table exists in
    the v1.17.0 README (the website Supported-Releases page is the real source).
  - strimzi base env leaves `spec.kafka.version` unset with a Reload4j-era logging ConfigMap but links
    no E8; on 0.46 the operator default moves to Kafka 4.0.0, so E8 plausibly applies there.
  - karpenter base F4 cites a "1.0.x row" of compatibility.yaml that does not exist at the v1.0.0 tag
    (matrix ends at 0.37.0/K8s 1.30); substance survives via the migration doc's 1.25 floor. E7's
    assumeRoleARN/assumeRoleDuration subjects actually live under `settings.*` in the 0.37.8 chart.

## Next (for the returning Claude agent / commander)
- Decide on the `eval/results` regression handling (`-update` after review vs capture-lane re-snapshot).
- Decide whether the four base-label concerns above warrant corrections (they are upstream-grounded
  observations from blind transfer authors; corrections would follow the CHANGELOG.md process).
- Optional: more transfer environments or new cases (brief minimums are met: ≥4 transfers, ≥6 new cases).
- The seven new cases were reviewed structurally + at NOTES level by GLM; a full quote-by-quote
  re-verification against upstream was not redone (sub-agent reports claimed blind validation; the
  loader verifies evidence line ranges and label validity).

## Decisions
- Transfer cases as sibling ids `<base>--<suffix>` with `transferOf` (least invasive; chosen in step 1).
- New cases and transfers committed individually (one commit per case) so any later correction can be
  reverted alone.
- Transfer bases chosen for diversity (cert-manager, cilium, karpenter, strimzi) among cases whose
  labels were already committed in 4ee35f7.

## Files touched outside ownership
- None beyond the brief's file list.

## Test status
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` green after all commits in this window.

## GLM handoff log
(GLM-5.3 continuing the lane during the Claude agent's usage-limit window, 2026-10-01.)
- Took over with steps 2–3 already committed (4ee35f7) and seven authored-but-uncommitted new case
  directories on disk. Reviewed them against `briefs/groundtruth-newcase.md` (structure, link mix,
  evidence, notes), ran `go test ./internal/eval` (dataset load + full package) — green — and committed
  them one per commit, plus the transfer sub-brief file that was untracked.
- Spawned four blind sub-agents per `briefs/groundtruth-transfer.md` (bases cert-manager-1.16-1.17,
  cilium-1.16-1.17, karpenter-0.37.8-1.0.0, strimzi-0.45-0.46). Each reported with tests green; I
  reviewed case.yaml + cited fixture lines + NOTES for each and committed them individually.
- Ran the single `ri eval` (step 6) with the primary checkout's warm cache (`-state
  /Users/tommydavison/repos/Release-Intelligence-/.ri`) after all expectations were committed; recorded
  the first numbers above without tuning anything and without `-update`.
- Uncertain / left for the returning agent: (a) whether more than four transfers are wanted (stopped at
  four, brief says ≥4, to fit the window); (b) the seven new cases were not re-researched
  quote-by-quote against upstream by me; (c) the `eval/results` regression and the base-label concerns
  need a commander decision (above).
