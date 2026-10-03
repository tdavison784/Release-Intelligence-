# Lane `groundtruth` — status

**State: done** (all six steps; GLM-5.3 hand-off window reviewed 2026-10-02 — see "Review of the GLM hand-off window").

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

## Eval numbers (post-review run, 2026-10-02; all expectations committed before any run; nothing tuned)

28 entries, 0 pipeline failures. Recall 0.95 (critical 0.98 ✓, important 0.95 ✓), labeled precision
0.58, classification accuracy 0.42 (112 scored), unknown rate 0.80, unsupported 0 ✓,
actionFindingEvidence 1.00 ✓. **applicabilityAccuracy 0.39 ✗** (105 decisions: affected links 11/73
hit, not-affected links 30/32 clean). **falseActionRate 0.15 ✗** (13 ACTION findings, 2 wrong:
crossplane-1.20-2.0, kyverno-1.12-1.13). Not-affected violations: crossplane E3, external-secrets E5.

**Read this honestly:** 0.095 → 0.39 is mostly the denominator, not the engine. The 32 new
not-affected links count as correct whenever no affected finding joins them, and the engine produces
few findings at all. The affected-link hit rate is 0.15 (11/73). Per environment: argo 0/3, cm-1.16
0/2, cm-1.17 0/2, cilium-1.15 1/2, cilium-1.16 1/2, istio 0/3, karpenter 0/3, strimzi 0/1, flux 3/5,
kyverno 2/8, external-secrets 2/5, cilium transfer 2/4, every other new case or transfer 0. The
first run (GLM window) also showed `karpenter-0.37.8-1.0.0 unsupported 0 → 11`: 11 findings joined
change ids missing from the edge `Upgrade()` returned. That was transient (an immediate re-run gave
0; this run has 0 and no stored-result regressions), so nothing should be `-update`d. It is not a
label effect, as the GLM log suggested; it looks like the edge changing between the two pipeline
calls on a cache-refreshing first run. Flagged for the pipeline owner.

## Review of the GLM hand-off window (2026-10-02)
- Seven new cases + four transfers reviewed: dataset-wide label-consistency audit (now the permanent
  `TestDatasetLabelConsistency`) and an automated verbatim check of every quote against its cited URL
  (216/216 new-case quotes verify after one URL fix).
- Fixed (eval/CHANGELOG.md, 2026-10-02): loki/traefik item classes aligned to the env-class
  convention (they predated it); crossplane E4 citation URL; the cert-manager-1.17 E3 PR-11819 quote
  made verbatim; karpenter E7 subjects → `settings.assumeRoleARN/Duration` (a valid concern from a
  transfer author); karpenter F4's source → compatibility.yaml on main.
- GLM's other two base concerns: strimzi E8 is already recorded as an observation; cert-manager F1's
  README table rolls forward on master, so it is recorded, not changed. Ten pre-existing (pre-lane)
  quotes in the old environment cases are paraphrases; they are listed in the CHANGELOG, not re-verified.
- Three further transfer agents (argo-cd, cert-manager-1.17, istio) were stopped by the user and not
  relaunched; the four committed transfers meet the brief's ≥4.

## Contract changes / requests
- None to `semantic.go`. Requests: (1) the condition path syntax needs a **map wildcard**
  (`spec.metrics[].overrides[].tagOverrides.*.value`, istio E6); (2) envinv `inventory.yaml`
  `complete: true` is still unsupported, so no label can rely on "product not installed";
  (3) scoring `undecidedImpact` links (correct = no affected and no not-affected finding) is a scoring
  change that needs pre-registration; (4) installed-CRD `status.storedVersions` (flux E2,
  external-secrets E3) is in `crds/` fixtures, which the env loader reads as CRDs, not resources.

## Follow-ups after the merge (groundtruth-2, 2026-10-02; branch merged with p3-learning-loop @ 823b44c)
- (1) Done: the ten paraphrased environment-case quotes are now verbatim (eval/CHANGELOG.md
  "2026-10-02 (b)"). Every quote of the 15 environment cases verifies. About 30 quotes in edge-only cases
  are still unverified (observation only).
- (2) Done, proposal only: docs/phase3/learning-loop/UNDECIDED-SCORING.md (reported, ungated
  `unknownHonesty`). Nothing pre-registered or implemented; the product owner decides.
- (3) Done: stored-edge fixtures for the 7 new base entries (the 4 transfers share their base edge),
  with `missedAtRecording` for the 9 recall misses. The routine gate now covers 24 entries.
- (4) Done per the product-owner decision (groundtruth-3): crossplane E3 matcher narrowed to
  `\bStoreConfigs?\b`, E1 given subject matchers for spec.mode/spec.resources[]/spec.patchSets[].
  Disclosed as motivated by pipeline output (trustfix), with the gate panel before/after in
  eval/CHANGELOG.md "2026-10-02 (c)": falseActionRate 0.133 → 0.067, applicabilityAccuracy 0.467 →
  0.486, not-affected violations 1 → 0. kyverno E9 untouched (pending decision).

## groundtruth-4 (2026-10-02, branch p3ll/groundtruth-4 from p3-learning-loop @ 6eb9cd4)
- Judged the render lane's three rendered effects on not-affected links from upstream sources:
  plant-edge E4 → review (SDS/secret-sync new-cluster defaults apply: no upgradeCompatibility, a
  kube-system-backed TLS policy); kyverno E7 → informational (v2beta1 PolicyExceptions, shielded by the
  migrate hook = overlap); plant-edge E3 stands (statusReport is BGPv2 status reporting, not the
  metallb-bgp removal; reaches E3 only via the broad `(?i)bgpControlPlane` matcher — a PO decision).
  Base cilium E4/E8 gained the `tls.secretSync.enabled` subject. Disclosed as render-4-motivated, with
  before/after panels in eval/CHANGELOG.md "2026-10-02 (d)": plain applicability 0.495 → 0.476, render
  0.495 → 0.514, render not-affected violations 3 → 1.

## groundtruth-5 (2026-10-02, branch p3ll/groundtruth-5 from p3-learning-loop @ 62afad6)
- LOOP-DIAGNOSIS stage f, under the verbatim-quote policy: only karpenter E7 qualifies (its own v0.37.8
  values.yaml quote names `logConfig`), and a matcher was added (applicability 0.476 → 0.486, no new false
  action). Left unchanged, with reasons, in eval/CHANGELOG.md "2026-10-02 (e)": cilium-1.15 E6 /
  cilium-1.16 E3 (the quote names metallb-bgp, not `bgp.*`; the cited doc names the keys elsewhere, which is
  a PO decision), karpenter-ci E7 (the quote names the env var FEATURE_GATES.DRIFT, not
  `settings.featureGates.drift`; the link is review vs an ACTION finding), strimzi edge-retail E2 (the quote
  says "MirrorMaker 1", not KafkaMirrorMaker).

## groundtruth-6 (2026-10-02, branch p3ll/groundtruth-6 from p3-learning-loop @ af0d911d; GLM handoff window)
- Acted on the LOOP-DIAGNOSIS-2 findings owned by this lane (§8.1–§8.5, levers L3/L7), four commits:
  - `0db5e296` — GT exposure predicate errors (§8.1–§8.2), CHANGELOG (f): eu-platform E2 (ValidateCAA
    unset→enabled), eu-platform E3 (inverted `all`-of-disabled → `not(all ...)`), istio E6
    (tagOverrides.* → tagOverrides.peer_namespace), karpenter-ci E3/E4 (missing `group: apps` on
    Deployment/DaemonSet resource leaves). Engine-verified pre/post with a temporary harness: every
    fixed link now evaluates exactly as labelled; no other link moved.
  - `6affc2b2` — fixture completion for four honestly-UNKNOWN links (§8.3, lever L7), CHANGELOG (g):
    argo-cd E1/E2 (empty argocd-cm, verbatim v2.14.5 upstream shape, evidence lists extended) and
    cert-manager-1.17 E3 (empty ingress-nginx-controller ConfigMap + `featureGates: ""` chart default +
    exposure path fix `config.featureGates` → `featureGates`). Chosen over undecidedImpact relabel — a
    denominator change needs pre-registration, and the environment stories already commit to the
    affected state.
  - `77917162` — cilium-1.15 E1 matcher narrowed to its quoted subject (§8.5/L3), CHANGELOG (h): the
    bare `(?i)toFQDNs` alternative is gone; the quote-anchored alternative is unchanged. Plain
    applicability unchanged (0.505; matched 430→428, labelled findings 300→298, attribution only); at
    the proxy view the dnsProxy finding no longer violates the not-affected label (0.571 → 0.581, NA
    violations 2 → 1; the remaining one is the kyverno E4 umbrella, §8.4, a contract decision).
    Isolated before/after proxy panel in the CHANGELOG.
  - `26c639f1` — `ri eval -update` accepted the honest snapshot change (cilium-1.15-1.17.json only:
    matchedChanges 26 → 24) + NOTES judgement-call entry with the standing PO question.
- Blocked by policy, recorded not done: strimzi-edge E2 CRD matcher and karpenter-ci E7 matcher
  (standing verbatim-quote policy, CHANGELOG (e) — neither item's own quote names the subject);
  kyverno E3/E4 umbrella (§8.4 contract decision).
- Baseline discrepancy investigated and resolved: the plain run at the branch point measures 23/75
  affected (applicability 0.505), not the 0.543 quoted in af0d911d's message. Per-case diff against
  `eval/results/*.json` shows every case matching the stored snapshots exactly — the 0.543 figure came
  from loop-diagnosis-2's own A/B harness context, not the plain eval aggregate. No unexplained
  movement; my predicate/fixture commits moved nothing in the plain run (predicate fixes are not
  scorer-evaluated until the knowledge join, where they matter).
- Proxy-view numbers on this branch (before/after the narrowing): affected 32/75 both; NA clean
  28→29; honesty 0.90. Note the current store has drifted from loop-diagnosis-2's snapshot —
  plant-edge E3 is clean now, cilium-1.15 E1 was still violated until this narrowing.

## Open for the commander
- Decide on docs/phase3/learning-loop/UNDECIDED-SCORING.md (do not pre-register from this lane), and report applicabilityAccuracy split into
  affected-hit and not-affected-clean (the headline number is denominator-sensitive).
- The CHANGELOG corrections that tighten or soften classes (D7 → review, istio E6 → action,
  cm-1.16 E1 → undecided) are upstream-grounded; veto any you disagree with.
- Narrow the base cilium E3/E6 `(?i)bgpControlPlane` matcher (plant-edge E3 render violation; D12)?
- Candidate new links on existing cases (strimzi E8, karpenter E5) are recorded, not added.

## Decisions
- Transfer cases as sibling ids `<base>--<suffix>` with `transferOf` (least invasive; chosen in step 1).
- New cases and transfers committed individually (one commit per case) so any later correction can be
  reverted alone.
- Transfer bases chosen for diversity (cert-manager, cilium, karpenter, strimzi) among cases whose
  labels were already committed in 4ee35f7.

## Files touched outside ownership
- `internal/eval/run.go` (3 hunks: transfer entries scored environment-only; base adjudications apply),
  `internal/eval/routine_test.go` (skip cases without a recorded stored-edge fixture; ≥17 gated),
  `internal/eval/dataset_test.go` (label-consistency test). New files: `internal/eval/labels.go`,
  `transfer.go`, `labels_test.go`.

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

### GLM handoff log — groundtruth-6 window (2026-10-02, GLM-5.3)
- Took over at af0d911d with only the LOOP-DIAGNOSIS-2 doc as tasking; scoped this window to the
  findings addressed to this lane (§8.1–§8.5) and levers L3/L7, following the lane's disclosure
  patterns.
- Verified every predicate fix engine-side with a temporary harness (loaded each case through
  eval.LoadCase + env.Load, evaluated each link's exposure with impact.EvaluateCondition) — pre-fix
  states matched the diagnosis exactly; post-fix every link evaluates as labelled. The harness was
  deleted before finishing (never committed).
- Found cm-1.17 E3 had three blockers, not the one diagnosed: missing ingress-nginx ConfigMap, wrong
  exposure path (config.featureGates → featureGates), missing values key — all three fixed with
  upstream grounding (chart default `featureGates: ""`, PR 11819 deploy.yaml shape).
- Ran the plain eval twice and the proxy view twice (isolated before/after for the matcher change)
  against the primary checkout's warm cache; accepted snapshots once, after review, one file changed.
- Uncertain / left for the returning agent: (a) whether the eu-platform E2 `state: enabled` reading
  (the cluster "manages the gate list" via empty-string featureGates → gates unset → per-gate default
  enabled in 1.17) should instead be `unset` — the engine says enabled matches the label, but the
  semantic reading is arguable; (b) the argo-cm fixture deliberately carries NO data key (verbatim
  v2.14.5 shape) — if the pipeline needs a ConfigMap with an empty `data: {}` to distinguish
  "examined, empty" from "absent", that is an engine question, not a fixture one; (c) karpenter-ci E7
  and strimzi-edge E2 remain honest review/miss outcomes under the standing policy — if the PO
  relaxes (e), both get matchers in minutes; (d) the discrepancy note above: af0d911d's 0.543 vs the
  plain 0.505 — worth one line in the integration status so the commander compares like with like.
