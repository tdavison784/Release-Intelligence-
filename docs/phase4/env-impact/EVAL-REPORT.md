# Phase 4 evaluation report — environment-aware investigation (MISSION P6, AC5)

Owner: lane `eval`. Harness: `ri eval -investigations` (docs/INVESTIGATE.md "Evaluation"; DESIGN.md §6).
Ground truth: `eval/investigations/` (8 cases, authored blind by lane groundtruth, 21 of 24 item decisions
confirmed by an observed `kind` upgrade). Gates: `eval/investigations/gates.yaml`, pre-registered and frozen;
nothing in this report moved a threshold, an expectation or a pipeline rule.

> **Read every number with n in mind.** 24 item decisions over 8 cases (3 products; 3 affected/not-affected pairs + 2 single cases); only **3** expected
> `affected` decisions and **0** expected `potentially-affected`. A single decision moves a rate by 4–33
> points. Rates here describe this dataset, not the system's general reliability.

| Arm | What runs | Stored run | State |
|---|---|---|---|
| **A** | deterministic only (no agent, no runtime) | `results/A-20261016T201359Z-294a49e7a9` (+ replicate `A-20261016T201443Z-294a49e7a9`) | **scored** (binary 294a49e7, pre-`fixes`) |
| **A′** | deterministic only, after lane `fixes` (4 routed defects fixed) | `results/A-20261016T213500Z-cf761e89e5` (+ identical dirty-tree copy `A-20261016T212406Z-13f9ba749d`, kept) | **scored** (binary cf761e89, clean tree) |
| **A″** | deterministic only, after fixes2 + values + runtime follow-up (`p4-env-impact` dec9342c; GLM layered-values fix reverted) | `results/A-20261016T231142Z-5d4da37635` (+ replicate `A-20261016T231217Z-5d4da37635`) | **scored** (binary 5d4da37635, clean tree) |
| **A‴ (reference for the ARM B decision)** | A″ + renderer fix 3b73d721 (Argo valuesObject / plain valueFiles / git-path Helm sources) | `results/A-20261016T231534Z-6b9fd9cc7c` | **scored** (binary 6b9fd9cc7c, clean tree) |
| **A⁗** | deterministic only, after lane `defaults` (fd103f0e; last pipeline fix) | `results/A-20261016T234421Z-aa06720656` | **scored** (binary aa06720656, clean tree) |
| **C′** | A⁗ + runtime (attribution precedence 1888a107) | `results/C-20261016T234455Z-aa06720656` | **scored** (binary aa06720656, clean tree) |
| **A₅** | deterministic only, after lane `refcheck` (fc0fee09) — **reference for arm B** | `results/A-20261017T003437Z-0790b00157` | **scored** (binary 0790b00157, clean tree) |
| **C₅** | A₅ + runtime | `results/C-20261017T003511Z-0790b00157` | **scored** (binary 0790b00157, clean tree) |
| **B** (aborted) | A₅ + agent; first attempt | `results/B-20261017-355df68274` | 1 case, pipeline failure (audit-ordering defect, fixed by lane auditfix); kept |
| **B** | A₅ + investigation agent: claude-cli, **claude-sonnet-5-5**, prompt **investigate-agent/v5** (digest per case in run.json) | `results/B-20261017-047cb4ea1c` | **scored** (binary 047cb4ea1c = A₅ tree + audit fix, clean; built case by case) |
| **A₆** | deterministic control on the same binary as B | `results/A-20261017T014549Z-047cb4ea1c` | **scored**; identical decisions and metrics to A₅ |
| **B-replay** | B replayed offline from its stored transcripts (no model) | `results/B-replay-20261017T014617Z-047cb4ea1c` | **scored** |
| **B+C** | — | — | **not run** (see §2f) |
| **C** | + runtime (observed) validation in throwaway kind clusters | `results/C-20261016T213533Z-cf761e89e5` (+ superseded vacuous `C-20261016T212613Z-13f9ba749d`, kept) | **scored** (binary cf761e89, clean tree) |

## 1. Headline (arm A)

- **Safety held, usefulness did not.** No expected-affected item was reported `not-affected`
  (falseNegativeRate 0/3 — but **INSUFFICIENT-DATA**: the gate needs ≥ 4) and nothing was approval-eligible
  (unsafeEligibilityRate 0/0 — **INSUFFICIENT-DATA**). Every investigation requires human review.
- **The deterministic layer cannot tell any pair apart.** All three pairs (payments-eu/us, shop-de/fr,
  obs-north/south) receive *identical* classification profiles: the setting that decides each pair is
  either a controller-binary default (cert-manager, ingress-nginx) or a render-namespace relation
  (kube-prometheus-stack) that no deterministic validator in the arm establishes.
- **notAffectedConfirmation 0/20** — no expected-not-affected item is confirmed. 15 of 23 decisive items end
  `insufficient-evidence` (unresolvedRate 0.65). Two pipeline defects (§5.1) cause part of this; the rest is
  honest fail-closed behaviour for controller-behaviour items without a trusted pin or an observed outcome.
- **Structural properties hold:** citationResolution 1954/1954, reproducibility 8/8 (re-classification of
  the stored records is byte-identical, in-process and again offline from the committed JSON), 0 pipeline
  failures; the replicate run produced the same bundles, investigations and scores.

## 2. Gates (pre-registered; INSUFFICIENT-DATA is never a pass)

| Gate | Threshold | Arm A | Arm A′ (post-`fixes`) | Arm B | Arm C |
|---|---|---|---|---|---|
| falseNegativeRate | ≤ 0.00 (n ≥ 4) | 0.000 (0/3) **INSUFFICIENT-DATA** | 0.000 (0/3) **INSUFFICIENT-DATA** | pending | 0.000 (0/3) **INSUFFICIENT-DATA** |
| unsafeEligibilityRate | ≤ 0.00 (n ≥ 1) | n/a (0/0) **INSUFFICIENT-DATA** | 0.125 (1/8) **FAIL** | | 0.125 (1/8) **FAIL** |
| falsePositiveRate | ≤ 0.25 (n ≥ 2) | 0.050 (1/20) PASS | 0.050 (1/20) PASS | | 0.050 (1/20) PASS |
| affectedDetection | ≥ 0.75 (n ≥ 2) | 0.333 (1/3) **FAIL** | 0.333 (1/3) **FAIL** | | 0.333 (1/3) **FAIL** |
| affectedConfirmation | ≥ 0.50 (n ≥ 2) | 0.000 (0/3) **FAIL** | 0.000 (0/3) **FAIL** | | 0.000 (0/3) **FAIL** |
| notAffectedConfirmation | ≥ 0.50 (n ≥ 2) | 0.000 (0/20) **FAIL** | 0.100 (2/20) **FAIL** | | 0.100 (2/20) **FAIL** |
| exposureRecall | ≥ 0.75 (n ≥ 2) | 0.286 (2/7) **FAIL** | 0.143 (1/7) **FAIL** | | 0.143 (1/7) **FAIL** |
| evidenceCoverage | ≥ 0.75 (n ≥ 4) | 0.085 (6/71) **FAIL** | 0.113 (8/71) **FAIL** | | 0.113 (8/71) **FAIL** |
| citationResolution | ≥ 1.00 (n ≥ 1) | 1.000 (1954/1954) PASS | 1.000 (2426/2426) PASS | | 1.000 (4513/4513) PASS |
| unresolvedRate | ≤ 0.50 (n ≥ 4) | 0.652 (15/23) **FAIL** | 0.565 (13/23) **FAIL** | | 0.565 (13/23) **FAIL** |
| observedAgreement | ≥ 0.75 (n ≥ 4) | 0.000 (0/21) **FAIL** | 0.000 (0/21) **FAIL** | | 0.000 (0/21) **FAIL** |
| reproducibility | ≥ 1.00 (n ≥ 1) | 1.000 (8/8) PASS | 1.000 (8/8) PASS | | 1.000 (8/8) PASS |
| pipelineFailures | ≤ 0 | 0 (of 8) PASS | 0 (of 8) PASS | | 0 (of 8) PASS |

Arm A: **4 PASS, 7 FAIL, 2 INSUFFICIENT-DATA**; arm A′: **4 PASS, 8 FAIL, 1 INSUFFICIENT-DATA** (one gate
moved from INSUFFICIENT-DATA to a real, failing denominator — that is the measurement gaining resolution,
not the system regressing; see §2a). Arm C: **4 PASS, 8 FAIL, 1 INSUFFICIENT-DATA** — numerically identical
to A′ on every gate except citationResolution's denominator (2426 → 4513: the runtime records' citations all
resolve). `ri eval -investigations` exits non-zero in every arm.

## 2c. Re-measure after fixes2 + values + runtime follow-up (arm A″)

Runs `A-20261016T231142Z-5d4da37635` and replicate `A-20261016T231217Z-5d4da37635` (clean merged tree,
`p4-env-impact` dec9342c: fixes2 — CRD documentation class, coverage echo, reproducible record digests;
values — per-release values stacks, the GLM layered-values fix reverted as fail-open; runtime follow-up).
Scores of the two runs are identical.

**Gates: 7 PASS, 5 FAIL, 1 INSUFFICIENT-DATA** (A′: 4 / 8 / 1).

| Gate | A′ | A″ |
|---|---|---|
| falseNegativeRate | 0/3 INSUFFICIENT-DATA | 0/3 **INSUFFICIENT-DATA** |
| unsafeEligibilityRate | 1/8 FAIL | 1/8 **FAIL** |
| falsePositiveRate | 1/20 PASS | 1/20 PASS |
| affectedDetection | 1/3 FAIL | **3/3 PASS** |
| affectedConfirmation | 0/3 FAIL | 0/3 **FAIL** |
| notAffectedConfirmation | 2/20 FAIL | 2/20 **FAIL** |
| exposureRecall | 1/7 FAIL | **6/7 PASS** |
| evidenceCoverage | 8/71 FAIL | 23/71 **FAIL** (file-only 34/71) |
| citationResolution | 2426/2426 PASS | 2027/2027 PASS |
| unresolvedRate | 13/23 FAIL | **0/23 PASS** |
| observedAgreement | 0/21 FAIL | 0/21 **FAIL** |
| reproducibility | 8/8 PASS | 8/8 PASS |
| pipelineFailures | 0 PASS | 0 PASS |

- **What moved:** every decision that was `insufficient-evidence` in A′ is now `potentially-affected`
  (unresolvedRate 0/23): the controller-behaviour items now carry plausible-impact signals (paths/roots,
  correlated changes) and end R3 instead of R4. That turns the three expected-affected decisions into
  detections (affectedDetection 3/3) and raises exposure reach (6/7) — but it is not confirmation:
  affectedConfirmation stays 0/3 and no not-affected was added (the same two obs B3 decisions, 2/20).
  shop-pl B1 (expected insufficient-evidence) is now potentially-affected — its declared acceptable
  alternative. Accuracy 2/24.
- **unsafeEligibilityRate is still 1/8, and it is still obs-north B1.** The four findings that were eligible
  not-affected in A′ — `ci-0f8475f72a68`, `ci-1d8a464ef1b7`, `ci-63ebfdbffcaf`, `ci-f8350fd0b224` (finding ids
  unchanged) — are **still not-affected, `classify:validated-clear`, eligible**, on the same grounds as in the
  diagnostic (`runs/unsafe-eligibility-diagnostic.md`): no correlated rendered change in either scope, no
  roots/paths, join `impact:values-unset` pass + render-diff pass. The decision is `potentially-affected`
  (the composite upgrade-note item `ci-194d5b487655` now R3), so the decision class is safe, but the
  any-matched-finding eligibility count still fires. The other eligible decisions (shop-de/fr B4, obs B2/B3)
  are all expected not-affected.
- **Investigation level:** 0/8 investigations auto-approve-eligible (agent not run), 8/8 human review.

**Record digests (reproducibility of the run record): confirmed.** The two runs produce identical bundles,
identical identified ImpactReport artifacts and identical canonical record digests (`run.ContentDigest`)
for 8/8 cases; the wall-clock times now live in `Investigation.reportStamps`, outside the identified
artifact. Note for the stored files: these two runs' `run.json` `recordDigest` fields were written by the
harness's older private digest, which predates `reportStamps` and therefore differs between the runs; the
harness now uses `run.ContentDigest` (future runs). The stored files are not edited.

## 2f. Arm B — the investigation agent, isolated (B vs A₅/A₆)

Run `B-20261017-047cb4ea1c`: claude-cli driver, model `claude-sonnet-5-5` (as reported by the CLI), prompt
`investigate-agent/v5` (per-item sessions, cap 10 items per investigation, item budget 16 calls / 14 turns /
150 000 tokens), one session set per case, run case by case and committed after each case (8 commits). Binary
047cb4ea1c = the A₅ tree plus the audit-ordering fix (5 lines in `run/pipeline.go`; no classification change). The
control A₆ on the same binary reproduces A₅'s decisions and metrics exactly, so **B − A₆ is the agent's effect**.

**Cost:** 824 623 tokens, 496 tool calls, ~21 min wall clock for 8 cases (91–131 k tokens per case). 7 agent runs
`submitted`, 1 `budget-exhausted` (shop-de: the composite #11819 item's session used 17 turns of 14). 0 pipeline
failures.

**Gates: 7 PASS / 5 FAIL / 1 INSUFFICIENT-DATA — every gate identical to A₅/A₆ except:**

| Metric | A₅ / A₆ | B | |
|---|---|---|---|
| evidenceCoverage | 23/71 | **25/71** | FAIL either way |
| evidenceFileCoverage (path only, ungated) | 34/71 | **50/71** | the agent cites the environment files that matter |
| citationResolution | 1999/1999 | 2261/2261 | every agent citation resolves (quotes verified) |
| everything else | — | unchanged | incl. unsafeEligibilityRate 1/8 (same three not-rendered per-key items) |

**Decisions: 0 of 24 changed. Findings: 0 of 211 changed class, 0 changed eligibility; investigation decisions
unchanged (8/8 human-review-required).** What the agent did produce (pipeline output):
- **46 `raises-concern` assertions — all on items that were already potentially-affected.** No concern landed on a
  not-affected finding, so none blocked a clearance; 8 were answered by a passing validation.
- **20 validation requests — all `not-run`** (19 × override inspection: "no declarative pin rule names a setting
  that pins the old behaviour of this item"; 1 × static reference). The agent asks the right question, but the
  validator has no trusted pin source to answer it with.
- **82 deterministically verified setting claims** (record_claim, prompt v5): 65 `absent`, 15 `value`, 2 `present` —
  e.g. a `value` claim that `controller.metrics.enabled` is set in the environment's values. These are verified
  facts about the environment, but no policy rule consumes a verified claim as a pinned setting, so they cannot
  clear a controller-behaviour item. **This is the structural reason the agent moves no number:** under §3.3 the
  agent can only raise or request; its requests reach validators that cannot decide without trusted knowledge; its
  verified claims are not wired to R2.4. Turning verified claims (or the agent's requested settings) into
  override-inspection pins is the change that would let the agent's work count — a policy/contract decision, not an
  evaluation one.
- No agent-proposed edge changed a traversal (paths identical in every case).

**Replay (offline reproducibility of B).** `B-replay-…` re-issues every recorded tool call against fresh bundles
with no model: findings identical in 7/8 cases; decision classes and every gate identical. obs-south: one of ten
item sessions mismatched (`submit_findings` result digest differs from the recording for item `ci-194d5b487655`),
which the replay driver reports honestly (agent status `failed`, the mismatch in the reason) — classes unchanged,
only that decision's gap/next-step text differs. Routed to the agent owner (a non-deterministic `submit_findings`
result). Investigation ids differ from B by design (the driver name is part of the configuration digest);
assertions differ only in `generatedAt`.

**B+C not run.** Running the agent live with runtime would spend another ~0.8 M tokens; reusing arm C's stored
runtime records in B's investigations would stitch two different runs (not honest); replaying B's transcripts with
runtime enabled is not a faithful replay either (the runtime records change what the agent's tools return, so the
recorded calls no longer match). Given B's effect on every decision is nil and C's effect is recorded separately
(§2e: shop-nl B2 observed clearing, obs-north observed basis), B+C would add cost without isolating anything new.

## 2e. After lane `refcheck` (arms A₅ and C₅) — current reference

Runs `A-20261017T003437Z-0790b00157` and `C-20261017T003511Z-0790b00157` (clean tree, merge 0790b001 containing
fc0fee09: render-diff exposure ⇒ inconclusive, concrete failures such as dangling references ⇒ fail; an
`impact:not-joined` join verdict no longer blocks an otherwise-cleared item). Same bundles in both arms.

| Gate | A⁗ | C′ | **A₅** | **C₅** |
|---|---|---|---|---|
| falseNegativeRate | 0/3 INSUFF. | 0/3 INSUFF. | 0/3 **INSUFFICIENT-DATA** | 0/3 **INSUFFICIENT-DATA** |
| unsafeEligibilityRate | 1/8 FAIL | 1/8 FAIL | 1/8 **FAIL** | 1/9 **FAIL** |
| falsePositiveRate | 2/20 PASS | 2/20 PASS | **1/20** PASS | **1/20** PASS |
| affectedDetection | 3/3 PASS | 3/3 PASS | 3/3 PASS | 3/3 PASS |
| affectedConfirmation | 1/3 FAIL | 1/3 FAIL | 1/3 FAIL | 1/3 FAIL |
| notAffectedConfirmation | 2/20 FAIL | 2/20 FAIL | 2/20 FAIL | **3/20** FAIL |
| exposureRecall | 6/7 PASS | 6/7 PASS | 6/7 PASS | 6/7 PASS |
| evidenceCoverage | 23/71 FAIL | 23/71 FAIL | 23/71 FAIL | 23/71 FAIL |
| citationResolution | PASS | PASS | 1999/1999 PASS | 3680/3680 PASS |
| unresolvedRate | 0/23 PASS | 0/23 PASS | 0/23 PASS | 0/23 PASS |
| observedAgreement | 1/21 FAIL | 1/21 FAIL | 1/21 FAIL | **2/21** FAIL |
| reproducibility | 8/8 PASS | 8/8 PASS | 8/8 PASS | 8/8 PASS |
| pipelineFailures | 0 PASS | 0 PASS | 0 PASS | 0 PASS |

Both arms **7 PASS / 5 FAIL / 1 INSUFFICIENT-DATA**.

**Per-case decisions vs the previous runs** (A⁗ → A₅; C′ → C₅): 21 of 24 decisions are unchanged in both arms.
Changes:
- **obs-south B1** (expected not-affected): affected → **potentially-affected** in A and C. The render-diff `fail`
  on the coreDns ServiceMonitor `spec.endpoints` change is now an inconclusive exposure (`error`), so R1 no longer
  fires. The false positive from §2d is gone (falsePositiveRate 2/20 → 1/20, the remaining one being shop-nl B1,
  ACTION REQUIRED contract).
- **obs-north B1** (expected affected): stays **affected**, now for a concrete reason. render-diff and
  static-reference both `fail` on a dangling reference in the TO render: `ServiceMonitor
  kube-system/kube-prometheus-stack-coredns spec.endpoints[0].authorization.credentials.name → Secret
  kube-system/kube-prometheus-stack-prometheus-token`, which the TO render does not contain. Deterministic A now
  separates the kube-prometheus-stack pair (north affected, south potentially-affected). Basis is deterministic in
  A₅ and observed in C₅ (three runtime `fail`s, as in C′).
- **shop-nl B2** (expected not-affected, C only): potentially-affected → **not-affected, basis observed**. This is
  the first observed clearing in the evaluation: a runtime `pass` on "⚠️ Metrics: Disable by default"
  (controller-behaviour) now clears the item because the `impact:not-joined` gap no longer blocks it.
  notAffectedConfirmation 3/20, observedAgreement 2/21 in C₅. The same item also clears in shop-fr, where no
  expected item selects it.

**unsafeEligibilityRate — contributors unchanged and still only the not-rendered per-key items.** The counted
decision is obs-north B1 (system decision `affected`), in both arms, through the same three eligible not-affected
findings (bundle `bnd-3345ac2d26f6`, identical to §2d's, so the same FROM render):

| Finding | Item | Title | Rule | Component in the FROM render |
|---|---|---|---|---|
| `inf-8584d1f22d6d` | `ci-0f8475f72a68` | Helm value `kubeDns.serviceMonitor.bearerTokenFile` removed | `classify:validated-clear` | not rendered |
| `inf-8bd4c3f5b149` | `ci-1d8a464ef1b7` | Helm value `kubeProxy.serviceMonitor.bearerTokenFile` removed | `classify:validated-clear` | not rendered |
| `inf-9d2274eda209` | `ci-f8350fd0b224` | Helm value `kubeEtcd.serviceMonitor.bearerTokenFile` removed | `classify:validated-clear` | not rendered |

C₅'s denominator is 9 because shop-nl B2 is now eligible as well (expected not-affected, so not unsafe). The
granularity finding of §2d stands as reported (no re-matching).

## 2d. After lane `defaults` (arm A⁗) and with runtime (arm C′) — basis for the ARM B decision

Runs `A-20261016T234421Z-aa06720656` and `C-20261016T234455Z-aa06720656` (clean tree, merge aa067206 containing
fd103f0e and 1888a107). Both arms: **7 PASS / 5 FAIL / 1 INSUFFICIENT-DATA**, same numbers per gate.

| Gate | A‴ (ref.) | A⁗ | C′ |
|---|---|---|---|
| falseNegativeRate | 0/3 INSUFFICIENT-DATA | 0/3 INSUFFICIENT-DATA | 0/3 INSUFFICIENT-DATA |
| unsafeEligibilityRate | 1/8 FAIL | 1/8 **FAIL** | 1/8 **FAIL** |
| falsePositiveRate | 1/20 PASS | **2/20** PASS | 2/20 PASS |
| affectedDetection | 3/3 PASS | 3/3 PASS | 3/3 PASS |
| affectedConfirmation | 0/3 FAIL | **1/3** FAIL | 1/3 FAIL |
| notAffectedConfirmation | 2/20 FAIL | 2/20 FAIL | 2/20 FAIL |
| exposureRecall | 6/7 PASS | 6/7 PASS | 6/7 PASS |
| evidenceCoverage | 23/71 FAIL | 23/71 FAIL | 23/71 FAIL |
| citationResolution | PASS | 1861/1861 PASS | 3503/3503 PASS |
| unresolvedRate | 0/23 PASS | 0/23 PASS | 0/23 PASS |
| observedAgreement | 0/21 FAIL | **1/21** FAIL | 1/21 FAIL |
| reproducibility | 8/8 PASS | 8/8 PASS | 8/8 PASS |
| pipelineFailures | 0 PASS | 0 PASS | 0 PASS |

**What `defaults` changed (A‴ → A⁗).** Removed values keys whose non-empty FROM default was in effect are now
join `impact:values-removed-default` (review-required) and the rendered change where that default disappears is
correlated. Two decisions moved:
- obs-north B1 potentially-affected → **affected** (R1 `classify:validated-exposure`, deterministic): item
  `ci-63ebfdbffcaf` "Helm value `coreDns.serviceMonitor.bearerTokenFile` removed" — render-diff `fail` on the
  correlated `ServiceMonitor kube-system/kube-prometheus-stack-coredns spec.endpoints` change. First confirmed
  affected decision (affectedConfirmation 1/3, observedAgreement 1/21).
- obs-south B1 potentially-affected → **affected** — the same item, the same R1 rule, on
  `ServiceMonitor monitoring/kube-prometheus-stack-coredns spec.endpoints`. Expected not-affected: a **new
  false positive** (falsePositiveRate 2/20, still within the gate). The deterministic layer still gives the pair
  identical classes; the difference between the two environments (where the ServiceMonitors render) is not
  something render-diff evaluates.

**unsafeEligibilityRate 1/8 — the counted decision is obs-north B1** (expected affected; system decision
`affected`). The eligible not-affected findings matched to it — pipeline output only:

| Finding | Item | Title | Rule | Component in the FROM environment render |
|---|---|---|---|---|
| `inf-8584d1f22d6d` | `ci-0f8475f72a68` | Helm value `kubeDns.serviceMonitor.bearerTokenFile` removed | `classify:validated-clear` | **not rendered** — no kube-dns object in `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/from.yaml` (80 objects) |
| `inf-8bd4c3f5b149` | `ci-1d8a464ef1b7` | Helm value `kubeProxy.serviceMonitor.bearerTokenFile` removed | `classify:validated-clear` | **not rendered** — no kube-proxy object |
| `inf-9d2274eda209` | `ci-f8350fd0b224` | Helm value `kubeEtcd.serviceMonitor.bearerTokenFile` removed | `classify:validated-clear` | **not rendered** — no kube-etcd object |

(For contrast, the coreDns sibling `ci-63ebfdbffcaf`, whose `Service` and `ServiceMonitor kube-prometheus-stack-coredns`
*are* rendered, is now `affected` and not eligible.) Each of the three has join `impact:values-removed-default`
(review-required), render-diff `pass`, no correlated rendered change, no exposure root. The FROM render was
re-read from a diagnostic re-execution whose bundles are byte-identical to the stored run's (`bnd-3345ac2d26f6`,
`bnd-e7ef20b45a3d`).

**Finding (reported, not changed):** the only remaining unsafe-eligibility count comes from per-key items of
chart components this environment does not render, matched together with the deciding item to one coarse
expected item. Their not-affected is plausibly correct per component; the count fires because the metric counts
a decision eligible when *any* matched finding is. The matcher, FORMAT.md and expectations are unchanged; whether
to narrow the expected item's `match`, or to score eligibility on the deciding finding, is a groundtruth /
commander decision. The decision-level class is `affected`, and no investigation is auto-approve-eligible (0/8;
agent not run).

**Arm C′ (runtime).** Seven cases ran an isolated kind upgrade (~3–4 min each; shop-pl refused, incomplete
render by design; `kind get clusters` empty afterwards). Only one decision differs from A⁗: **obs-north B1 stays
`affected`, now with basis `observed`** — attribution precedence puts a runtime `fail` on the matched coreDns item
(plus the two `scope:rendered-unexplained` ServiceMonitor items, unmatched). Runtime outcomes: obs-north 3 fail /
1 error; **obs-south 0 fail** (23 not-run) — the runtime layer does tell the kps pair apart, but it can only raise
a class, so obs-south's deterministic false positive stays. Two runtime `pass` records (shop-fr, shop-nl, item
"Metrics: Disable by default", controller-behaviour) do not clear the item: its remaining R2 blocker is a
validation gap from the join's `impact:not-joined` verdict (no machine-comparable subject), which an observed
pass does not lift — reported for the classify owner. Errors are `Inconclusive: objects on this item's impact
paths degraded …` (payments-eu/-us 7, shop-de 14, obs-north 1).

### 2c′. Reference run on 3b73d721 (arm A‴) — the reference for the ARM B decision

Run `A-20261016T231534Z-6b9fd9cc7c` (clean tree, merge 6b9fd9cc containing 3b73d721). **Every decision,
metric and gate is identical to A″** (7 PASS / 5 FAIL / 1 INSUFFICIENT-DATA; unsafeEligibilityRate 1/8 on
obs-north B1, whose four findings `ci-0f8475f72a68`, `ci-1d8a464ef1b7`, `ci-63ebfdbffcaf`, `ci-f8350fd0b224` are
still eligible not-affected). Seven of eight bundles and investigation ids are byte-identical to A″; only
shop-pl changed: its Argo render is still `partial` (values-incomplete), the reason now names the first
missing values file (`$values/shop/common.yaml` not present) instead of `$values/shop/prod-pl.yaml`; its
classes and decision are unchanged. No other case uses the source forms the fix addresses in a way that
changes the render.

## 2a. Re-measure after lane `fixes` (arm A′)

Run `A-20261016T213500Z-cf761e89e5` (clean tree, same warm offline cache family as run A). Six of 24
decisions changed class; accuracy 3/24 (was 1/24). **Confound, stated up front:** the shared `.ri` cache was
written by other lanes between the two runs, so bundles are not byte-identical to run A's (item sets and
citation counts grew — e.g. the CRD-description items now carry their own harmless class). The deltas below
mix the four `fixes` with that drift; per-decision root causes are given so the reader can see which is
which.

| Decision | A | A′ | Why (deciding rule) |
|---|---|---|---|
| obs-south B3 (insecureSkipVerify values, render) | insufficient-evidence ✗ | **not-affected ✓** (deterministic) | R2 validated-clear: render-diff `pass` + join `pass`, now that layered values files no longer make the repo input `partial` (fix 1). The §5.1a prediction held. |
| obs-north B3 (same, render) | insufficient-evidence ✗ | **not-affected ✓** (deterministic) | as obs-south B3 |
| payments-eu/-us B2 (revisionHistoryLimit) | insufficient-evidence ✗ | potentially-affected ✗ | R3 plausible-impact now fires: the traversal is complete at depth 12 (fix 4), giving 4 impact paths + 1 correlated rendered change; still not cleared (controller-behaviour, 1 relationship gap). Fail-closed, differently. |
| shop-de/fr/pl B4 (enableAnnotationValidations) | potentially-affected ✗ | insufficient-evidence ✗ | the old R3 trigger (review-required `impact:values-default-applies` join finding under the `partial` input) is gone with fix 1; the item falls to R4 controller-behaviour fail-closed. Still wrong, still unresolved-class. |

- **notAffectedConfirmation 0/20 → 2/20** — the first deterministic confirmations in this evaluation, both
  on render-verification items. The two clear through R2 exactly where §5.1a said they would.
- **unsafeEligibility 0/0 → 1/8 FAIL — the important new signal.** obs-north B1 (expected **affected**) now
  matches, alongside its own insufficient-evidence finding, four `validated-clear` **eligible** not-affected
  findings (the values-removed items the layered-values fix unblocked — the §5.1a caution realised). The
  metric counts a decision eligible when *any* matched finding is (documented, deliberately conservative —
  never understate approval risk). What it means concretely: finding-level eligibility no longer implies
  safety at the decision level, because one expected item can select several system items with different
  classes. **The end-to-end approval path is still safe** — investigation-level auto-approval requires
  *every* in-scope finding eligible plus a completed agent stage (§3.4), and obs-north's composite note
  finding is insufficient-evidence with the agent not run. But the gate is doing its job: if matcher
  granularity ever lets a *deciding* finding be eligible in an affected case, this is the gate that catches
  it. Related to §5 root cause c/g (one system item standing for several expected items — here the reverse:
  several system items under one expected item).
- **exposureRecall 2/7 → 1/7** — the only metric that got worse. The `ledger` Deployment (payments-eu B1)
  is no longer a traversal path *end* at depth 12; it sits mid-path (reported `onPath`). The fixes lane's
  depth bound is measured and right (§5.1f resolved); this exposes that the §6.2 "component reach = path
  end" definition ties reach to traversal topology. Whether mid-path should count is a DESIGN/metric
  question for the commander — not changed here.
- **unresolvedRate 15/23 → 13/23, evidenceCoverage 6/71 → 8/71, citations 1954 → 2426** — more items, more
  evidence, fewer unresolved; all in the safe direction. falsePositiveRate unchanged (shop-nl B1, §5.3).

## 2b. Arm C (runtime validation, run `C-20261016T213533Z-cf761e89e5`)

Same binary and tree as A′, plus `runtime.isolated-upgrade@v1` (opt-in `-runtime`): one throwaway kind
cluster per case, installed from the case's own renders, upgraded, exercised, observed, deleted. Seven of
eight cases ran a real upgrade (~2 min each; `kind get clusters` empty afterwards — cleanup verified). The
eighth, shop-pl, is *by design* unrunnable: its fixture says `complete: false` — the ingress-nginx value
files live in a separate values repository "not part of this input" — so the render is partial
(`values-incomplete: helm.valueFiles "$values/shop/prod-pl.yaml" is not readable`) and the validator
fail-closed refuses to install an incomplete render, recording honest `runtime:run-not-run` for all 27
items. Correct behaviour, correctly recorded.

Runtime record outcomes per case (in-scope items):

| Case | fail | error | not-run (why) |
|---|---|---|---|
| payments-eu | 0 | 0 | 46: 19 no-exercise, 13 other-families, 8 no-object, 6 no-baseline |
| payments-us | 0 | 7 `runtime:unreached` | 39: 19 no-exercise, 13 other-families, 6 no-baseline, 1 no-object |
| shop-de | 0 | 13 `runtime:unreached` | 14: 13 no-object, 1 no-exercise |
| shop-fr | 0 | 0 | 27: 26 no-object, 1 no-exercise |
| shop-nl | 0 | 0 | 31: 26 no-object, 5 no-exercise |
| shop-pl | 0 | 0 | 27 `runtime:run-not-run` (incomplete render, by design) |
| obs-north | **3 `runtime:degraded`** | 0 | 23: 21 no-object, 1 no-exercise, 1 no-baseline |
| obs-south | 0 | 0 | 26: 21 no-object, 4 no-exercise, 1 no-baseline |

**The headline: the runtime evidence is real, decisive, and invisible to the score.** The three
`runtime:degraded` fails in obs-north (the ServiceMonitor objects the chart's 90.x credential change
breaks — exactly the case's expected break) drove classify R1 on their items:

> 3 findings flipped potentially-affected (R3, basis none) → **affected (`classify:validated-exposure`,
> basis observed)** vs A′.

…yet **no decision and no gate moved** (affectedDetection stays 1/3, observedAgreement stays 0/21). The
expected-item matcher maps obs-north B1 to the composite release-note item (`match:` values subjects +
note text), and the three chart ServiceMonitor items that carry the runtime fails are matched by *no*
expected item (`itemMatchMisses` counts only expected items with no match, not system items with no
expected item). The strongest signal this pipeline can produce — an observed degradation on the truly
broken case — exists in the investigation and does not reach the score. Same root-cause family as §5 c/g
(expected-item granularity vs system-item granularity), now with a concrete high-stakes instance. Routing
to the commander: either the groundtruth lane widens B1's `match` to the chart ServiceMonitor items, or
the scorer's decision rule learns a sibling-item notion. Neither is this lane's to change (expectations
are groundtruth-owned; gates and scorer semantics are frozen for this evaluation).

Also observed, routed not fixed:

- **payments-eu lost the rotationPolicy fail.** The runtime lane's earlier real run (pre-`fixes`, 18 items,
  depth 8) recorded 1 `runtime:degraded` fail on the PrivateKey.RotationPolicy item; under current code
  (depth 12, 46 items) the degradations still occur (3 in the pre-arm control run) but that item now sits
  in a not-run bucket — its traversal paths no longer reach the degraded Secrets (and no
  `runtime:unattributed` error exists, so it is not an attribution tie). The control run reproduced this
  before arm C, so it is not a wiring artefact. Owner: runtime/relations (path selection over the larger
  graph).
- **`runtime:unreached` (payments-us ×7, shop-de ×13)** — items whose traversal paths claim objects the run
  never observed — record as `error`, which classify treats as neither failing nor clearing: R4
  insufficient-evidence, decisions unchanged. Honest, but it means the observed level currently *adds*
  classes only through `fail`, never through `pass` clearing behaviour (no `runtime:confirmed`-style clear
  fired in any case: every clearing in this evaluation is still deterministic R2). observedAgreement's
  0/21 is therefore structural until passes can clear.

Arm C summary: the runtime validator is wired, safe (fail-closed, cleanup verified, zero pipeline
failures), and produces decisive evidence — but its evidence reaches the gates only through item
granularity the matcher does not model.

**Autonomy (§6.3): not eligible**, and not reachable by design in milestone 1: falseNegativeRate and
unsafeEligibilityRate are INSUFFICIENT-DATA (= FAIL here); 18 observed not-affected decisions (< 20) and 3
observed affected decisions (< 20).

## Human-verified facts (2026-10-16)

The product owner human-reviewed the five pin-source items (commit e62355fc, reviewer `tdavison784`): three
cert-manager 1.18.0 facts (rotationPolicy `Never`→`Always`; two `revisionHistoryLimit` `nil`→`1`) were recorded as
human-verified, two ingress-nginx 1.12.0 accepts produced no fact. Lane `rescore` scored arms A and C again on
`p4-env-impact` d1ed12aa with the knowledge store as input (default flags: `-knowledge knowledge`, minimum
verification `human`; no pipeline, prompt, policy, matcher, FORMAT or expectation change).

Runs (clean tree, binary d1ed12aa8d, all earlier runs kept): `A-20261017T020337Z-d1ed12aa8d`,
`C-20261017T020532Z-d1ed12aa8d`, and a **control** `A-control-prereview-20261017T020459Z-d1ed12aa8d` = the same code and
arm A with the knowledge store as it was before the review (`git archive e62355fc^ knowledge`, passed as `-knowledge`).
The control separates the effect of the review from the 27 commits between A₅/C₅ (0790b001) and this tree.

| Gate | A₅ | **A (new)** | A control (pre-review store) | C₅ | **C (new)** |
|---|---|---|---|---|---|
| falseNegativeRate | 0/3 INSUFF. | 0/3 **INSUFFICIENT-DATA** | same | 0/3 INSUFF. | 0/3 **INSUFFICIENT-DATA** |
| unsafeEligibilityRate | 1/8 FAIL | 1/8 **FAIL** | 1/8 FAIL | 1/9 FAIL | 1/9 **FAIL** (shown 0.111) |
| falsePositiveRate | 1/20 PASS | 1/20 **PASS** | 1/20 PASS | 1/20 PASS | 1/20 **PASS** |
| affectedDetection | 3/3 PASS | 3/3 **PASS** | 3/3 PASS | 3/3 PASS | 3/3 **PASS** |
| affectedConfirmation | 1/3 FAIL | 1/3 **FAIL** | 1/3 FAIL | 1/3 FAIL | 1/3 **FAIL** |
| notAffectedConfirmation | 2/20 FAIL | 2/20 **FAIL** | 2/20 FAIL | 3/20 FAIL | 3/20 **FAIL** |
| exposureRecall | 6/7 PASS | 6/7 **PASS** | 6/7 PASS | 6/7 PASS | 6/7 **PASS** |
| evidenceCoverage | 23/71 FAIL | 23/71 **FAIL** | 23/71 FAIL | 23/71 FAIL | 23/71 **FAIL** |
| citationResolution | 1999/1999 PASS | 1999/1999 **PASS** | PASS | 3680/3680 PASS | 3680/3680 **PASS** |
| unresolvedRate | 0/23 PASS | 0/23 **PASS** | 0/23 PASS | 0/23 PASS | 0/23 **PASS** |
| observedAgreement | 1/21 FAIL | 1/21 **FAIL** | 1/21 FAIL | 2/21 FAIL | 2/21 **FAIL** |
| reproducibility | 8/8 PASS | 8/8 **PASS** | 8/8 PASS | 8/8 PASS | 8/8 **PASS** |
| pipelineFailures | 0 PASS | 0 **PASS** | 0 PASS | 0 PASS | 0 **PASS** |

Both new arms: **7 PASS / 5 FAIL / 1 INSUFFICIENT-DATA**, every gate identical to A₅ / C₅.

**Decisions that moved: none.** For all 24 expected items the system class, deciding rule, deciding finding, basis,
eligibility and correctness are identical to A₅ (arm A) and C₅ (arm C), and identical to the control. The only
differences in the stored decisions are wording: citation order inside `reasoning` for three arm-A items (shop-nl B1,
obs-north B3, obs-south B3, same validations, same rules) and `gaps` / `nextSteps` / `reasoning` text for 14 arm-C items
(runtime validation ids and next-step wording from code merged since 0790b001). The three cert-manager items the
facts concern stay `potentially-affected` by `classify:plausible-impact` (R3), with no `pin:knowledge-default-changed`
validation: payments-eu B1 (expected affected, basis none), payments-eu B2, payments-us B1, payments-us B2 (expected
not-affected).

**Why the human review did not reach the classifier.** Evidence from the store and the code, not from expectations:
- A fact's level is its **weakest aspect** (`VerifiedFact.Level`, `internal/domain/semantic.go:2352`). The three facts
  are `subject: deterministic`, `change: consensus` (cross-model for rotationPolicy and one revisionHistoryLimit
  fact, same-model for the other), `applicability: consensus`, `consequence: human`. Their level is therefore
  `consensus`; commit e62355fc changed only the consequence aspect.
- The knowledge query keeps facts `AtLeast(human)` (`internal/knowledge/filestore.go:414`) and the pin rules take only
  facts whose `Level().Trusted()` (`internal/investigate/validate/override.go:344`; `join.go:222`). Consensus never
  clears a change (docs/KNOWLEDGE.md). So these facts are not loaded as input at the `human` minimum, as before.
- The review UI hid the matching items because it judges an item by the aspects that item asked
  (`trustedAspects`, `internal/reviewui/triage.go`: consequence → human ⇒ "already verified at human or deterministic
  level"), not by the composed fact's level. The fact itself still has `change` and `applicability` at consensus and no
  open review item that would raise them (consensus-accepted aspects are not queued), so the UI reports nothing left
  to do while the fact stays below the gate.
- Consequently the end-to-end effect of the review is exactly zero today; the pin path (`pin:knowledge-default-changed`
  / `pin:knowledge-exposure`) was never exercised on real data. To make these facts input, the `change` and `applicability` aspects of each must reach human (or
  deterministic) level; I found no UI or command path that queues just those two aspects for a fact that is already
  composed (`review-fact` retracts or corrects a fact; it is not an aspect upgrade). Whether pins would then change a
  decision is not known from these runs: a pin only clears an item when it finds an explicit old-behaviour setting
  in the environment, and the stored gaps of the four items name none. **Open question for the commander;
  nothing in the store was changed here.**

**The two ingress-nginx accepts composed no fact.** Both belong to candidate `sc-298dac696f1a` ("⚠️ Metrics: Disable by
default", `--enable-metrics`): `rd-ff94bf6fdbf8` accepted the semantic mapping (subject `cli-flag
--enable-metrics`, change `default-changed "true"→"false"`, review item `ri-29af5ead5e5f`) and `rd-4c1f0155a7eb`
accepted the applicability (exposure `--enable-metrics` unset, review item `ri-57cfa14b0bff`). A fact needs all four
aspects; the **consequence** question `ri-246f31433fd5` ("what happens if an exposed environment does nothing…",
proposed `behavior-change`, `review-required`, severity medium) is still `pending` with route `review`, signal
`models-disagree`. Nothing else for the candidate is open (`knowledge/ingress-nginx/1.12.0/facts/` does not exist; the
release has 99 review items, 0 facts). The review UI default view ("Needs your verification") currently lists **0**
items: `ri-246f31433fd5` is not there, because a consequence-only proposal gives the pin rules no setting to look for
(`validate.CanBePinSource` false), so it is classed as "Other blocking items" (`?scope=blocking`: 206 items, of which
the UI lists the first 200 with the default `-limit 200`). Answering it there composes the fact; because the
change and applicability aspects are then human, the fact would be human at every aspect (this is the case where
the aspect-level gap above does not arise). Whether a pin then clears shop-nl B2 / shop-de / shop-fr / shop-pl
`--enable-metrics` items is a separate question the runs here cannot answer.

## 3. Metrics, split per basis, manifestation and expected verification (arm A)

Overall: accuracy 1/24 (the one correct decision is shop-pl B1, expected `insufficient-evidence`), human
review 24/24 decisions and 8/8 investigations, auto-approval eligibility 0/24, itemMatchMisses 0/24,
evidenceFileCoverage (path only) 9/71.

| Split | n | accuracy | FN rate | affected detection | NA confirmation | unresolved | exposure recall | evidence coverage |
|---|---|---|---|---|---|---|---|---|
| basis deterministic | 1 | 0/1 | — | — | 0/1 | 0/1 | — | 0/3 |
| basis none | 23 | 1/23 | 0/3 | 1/3 | 0/19 | 15/22 | 2/7 | 6/68 |
| basis observed | 0 | — | — | — | — | — | — | — |
| manifestation controller-behaviour | 15 | 1/15 | 0/2 | 1/2 | 0/12 | 11/14 | 2/3 | 6/46 |
| manifestation rendered-config | 9 | 0/9 | 0/1 | 0/1 | 0/8 | 4/9 | 0/4 | 0/25 |
| verification observed | 21 | 0/21 | 0/3 | 1/3 | 0/18 | 13/21 | 2/7 | 6/64 |
| verification observed-alternatives | 1 | 1/1 | — | — | — | — | — | 0/3 |
| verification render | 2 | 0/2 | — | — | 0/2 | 2/2 | — | 0/4 |

The only decision with a validation basis is the false positive (shop-nl B1, deterministic `fail` from a
wrapped join verdict). No finding in arm A has an observed basis (arm C will add it).

## 4. Per-case results (arm A)

`✓` correct · `✗` wrong · deciding rule and the main reason/gap of the deciding finding. "Matched" = system
change items selected by the expected item's `match` (strongest class decides).

| Case | Item | Expected (verif.) | System | Matched | Decided by / evidence gap |
|---|---|---|---|---|---|
| cert-manager payments-eu | B1 rotationPolicy default | affected (obs) | potentially-affected ✗ | 2 | R3 plausible-impact: 28 correlated env rendered changes + 11 impact paths; controller-behaviour, no clearing validation; traversal truncated (depth 8), 1 inferred edge |
| | B2 revisionHistoryLimit default | not-affected (obs) | insufficient-evidence ✗ | 2 | R4: controller-behaviour, no clearing validation (no pin, no observed outcome); traversal truncated |
| cert-manager payments-us | B1 | not-affected (obs) | potentially-affected ✗ | 2 | same as payments-eu: the CA's explicit `rotationPolicy: Never` (overlay) is not established as a pin |
| | B2 | not-affected (obs) | insufficient-evidence ✗ | 2 | same as payments-eu |
| ingress-nginx shop-de | B1 annotations-risk-level | affected (obs) | insufficient-evidence ✗ | 1 | R4: matched only the composite note "Enable security features by default (#11819)" (controller-behaviour) |
| | B2 cross-namespace | not-affected (obs) | insufficient-evidence ✗ | 1 | same composite item |
| | B3 strict path type | not-affected (obs) | insufficient-evidence ✗ | 1 | same composite item |
| | B4 enableAnnotationValidations | not-affected (obs) | potentially-affected ✗ | 2 | R3 on the computed values-default item; render-diff `pass`, but repo input `partial` (loader warning, §5.1a) |
| ingress-nginx shop-fr | B1–B3 | not-affected (obs) | insufficient-evidence ✗ | 1 | as shop-de |
| | B4 | not-affected (obs) | potentially-affected ✗ | 2 | as shop-de |
| ingress-nginx shop-nl | B1 opentelemetry values removed | not-affected (obs) | **affected** ✗ (FP) | 2 | R1: join `impact:values-removed` ACTION REQUIRED ("you set `controller.opentelemetry.enabled`") wrapped as a deterministic `fail` (§3.5) |
| | B2 metrics disabled by default | not-affected (obs) | potentially-affected ✗ | 1 | R3: correlated rendered change + 3 paths; the explicit `controller.metrics.enabled: true` is not established as a pin |
| ingress-nginx shop-pl | B1 | insufficient-evidence (obs-alternatives) | insufficient-evidence ✓ | 1 | R4 (composite item; deciding values file withheld) |
| | B2–B3 | not-affected (obs) | insufficient-evidence ✗ | 1 | composite item; no complete environment render (values file withheld) |
| | B4 | not-affected (obs) | insufficient-evidence ✗ | 2 | no complete environment render: `values-incomplete` |
| kube-prometheus-stack obs-north | B1 ServiceMonitor auth / token Secret | affected (obs) | insufficient-evidence ✗ | 5 | 4 values-removed items: join `pass` + render-diff `pass`, blocked only by the loader-warning gap (§5.1a); the composite upgrade note (controller-behaviour) R4 |
| | B2 secretFieldSelector default | not-affected (obs) | potentially-affected ✗ | 2 | R3 via an unexplained container-arg change on the operator Deployment |
| | B3 removed insecureSkipVerify values | not-affected (render) | insufficient-evidence ✗ | 4 | join `pass` + render-diff `pass`; blocked only by the loader-warning gap (§5.1a) |
| kube-prometheus-stack obs-south | B1 | not-affected (obs) | insufficient-evidence ✗ | 5 | as obs-north |
| | B2 | not-affected (obs) | potentially-affected ✗ | 2 | as obs-north |
| | B3 | not-affected (render) | insufficient-evidence ✗ | 4 | as obs-north |

Unscored findings (in-scope items no expected item selects): 14–28 per case; affected among them: 2 per
cert-manager case — `resource-removed` ClusterRole/ClusterRoleBinding `cert-manager-controller-challenges`,
which the groundtruth `renderedChanges` labels a harmless RBAC split into http01/dns01 roles. They are not
in the gated metrics but they are reviewer noise of the same kind as the FP below.

## 5. Error analysis (arm A)

23 wrong decisions. Root cause per decision (a decision can have more than one):

| Root cause | Kind | Decisions |
|---|---|---|
| a. layered values files ⇒ repo input `partial` ⇒ R2.1 blocks every values item | **pipeline defect** | shop-de/fr B4, obs-north/south B1 (values items) + B3, (+ shop-nl B1 gap list) |
| b. controller-behaviour item, no trusted pin source and no observed validation | **honest fail-closed** (missing verified knowledge / arm C) | payments-eu/us B1, B2; shop-nl B2; all composite ingress items |
| c. one upstream change item realises three expected items | **granularity** (collection) | shop-de/fr/pl B1–B3 |
| d. no complete environment render (values file deliberately withheld) | **missing input** (by design of the case) | shop-pl B2–B4 |
| e. join "you set a removed values key" ⇒ ACTION REQUIRED ⇒ `affected` | **contract consequence** (§3.5, never weakened) | shop-nl B1 (the FP) |
| f. cert-manager traversal truncated at depth 8 + 1 inferred edge | pipeline bound | payments-eu/us all items (R2.3 cannot hold) |
| g. 28 CRD-description rendered changes across all cert-manager CRDs correlated to the rotationPolicy item | correlation precision | payments-eu/us B1 (each needs a clearing validation) |

### 5.1 Pipeline defects (routed to the commander; not fixed here)

> **Status after lane `fixes` (merged, re-measured as arm A′, §2a):** (a) fixed — layered values files merge
> as Helm applies them, `partial` only for genuinely missing/unparseable; (f) fixed — default traversal
> depth 12, measured; (g) fixed — CRD description text is its own harmless render class; the
> `retrievedAt` reproducibility observation is fixed (identical runs now yield identical record digests —
> A′ reproduces 8/8). The texts below are kept as the record of what arm A measured.

a. **Loader warning on layered values files blocks R2.1.** In repo mode the env loader warns "repo mode found
2 values files; all are applied, the last one wins per key" (`internal/env/env.go:428`). The snapshot turns
that warning into `gitops-repo` status `partial`, which classify reads as a collection gap for every
values/image/rendered item — although the Argo Application names both files explicitly and the environment
render is `ok` and values-complete. It is the *only* blocker for obs-north/south B3 (join `pass` +
render-diff `pass`) and for the four values-removed items under obs-north/south B1. Owner: snapshot
(bundle package; orchestrate2 owns it now). **Caution for whoever fixes it:** with the gap gone, obs-north
B1's four values items could become `not-affected` while the case is *affected* (the break is decided by
the render namespace of the ServiceMonitors vs the token Secret, which no validator checks). The decision
would still be `insufficient-evidence` because the matched composite upgrade note stays R4, so the FN gate
would not move — but the safety then rests on that one composite item. A fix should be re-scored as a new
arm A run before anything else changes. **[2026-10-16: realised exactly as predicted — see §2a,
unsafeEligibility 1/8.]**

f/g. cert-manager traversal truncation at the default bound (max depth 8) and the breadth of CRD-description
correlation are worth a look by the relations / renderdiff owners; neither is a safety issue (both only
block `not-affected`).

h. **(found 2026-10-16 while wiring arm C; fixed minimally by lane eval, cf761e89 — outside ownership,
listed here and in the status file)** Checkout-independent snapshots (55ac1cfa) removed the repo path from
the stored snapshot, so the runtime validator's `AppRenderer` — which re-derives its inputs from the bundle
alone — could detect no render target and recorded every item `runtime:not-run` ("cannot re-render … no
longer detected in ."). The runtime lane's real runs predate 55ac1cfa, and its groundtruth harness passes
*vacuously* on current integration (a not-run run still conforms and deletes no cluster) — an assertion
sharp edge worth a look by whoever owns that test. Fix: exported `Repo` override on `AppRenderer` set by
the wiring that knows the checkout; verified by a real payments-eu isolated upgrade (215 s, 3 degradations,
cluster deleted) before scoring arm C.

### 5.2 False negatives

None (0/3). The three expected-affected decisions ended `potentially-affected` (payments-eu B1 — detected)
and `insufficient-evidence` (shop-de B1, obs-north B1 — not detected, counted in unresolvedRate). Neither
is confirmed: affectedConfirmation needs a failing deterministic or observed validation, and none of the
three breaks is visible as a breaking rendered change.

### 5.3 False positives

One (1/20): shop-nl B1. The environment sets `controller.opentelemetry.enabled: true`; chart 4.12.0 removed
the key. The Phase 3 join classifies "you set a removed key" as ACTION REQUIRED, and §3.5 requires such a
change to be `affected` (wrapped `fail`, R1). The observed upgrade shows tracing keeps working (the module
is loaded from the controller image in both versions). This is the never-weaken contract working as
designed; the FP gate passes. It is reported, not tuned.

### 5.4 Unresolved / incomplete investigations

All 8 investigations end `human-review-required`. Reasons common to all: the agent stage did not run (arm
A; it also blocks approval eligibility by design). cert-manager: traversal not complete (truncated).
shop-pl: no complete environment render (the case withholds the deciding values file; `insufficient-evidence`
is expected there for B1). No environment rendered change is uncorrelated in any case.

## 6. Human review and approval eligibility

- humanReviewRate: 24/24 decisions, 8/8 investigations (arm A).
- autoApprovalEligibilityRate: 0/24 decisions, 0/8 investigations — no finding was eligible, so
  unsafeEligibilityRate has no denominator (INSUFFICIENT-DATA, not a pass). Automated approval stays
  shadow-only (`AutoApprovalEnabled` false).
- A not-affected finding exists in 6 of 8 runs (shop-* 1–2 each, kps 5 each) but none is selected by an
  expected item; none is eligible because the agent stage did not complete.

## 7. What would have to change for autonomy (DESIGN §3.4, §6.3)

1. **Dataset size** — autonomy needs ≥ 20 observed not-affected and ≥ 20 observed affected/potentially
   decisions per dependency type, and the FN gate needs ≥ 4 affected decisions just to be evaluable. The
   dataset has 18 and 3. Milestone 1 cannot reach it (by design); more blind, kind-observed pairs are the
   prerequisite, not any pipeline change.
2. **A clearing path for controller-behaviour items** — 15 of 24 decisions are controller-behaviour. They
   can only clear through a pinned setting (needs trusted knowledge: today the cert-manager facts are
   consensus-verified and ingress-nginx / kube-prometheus-stack have none) or an observed validation (arm C).
   Without one of these, notAffectedConfirmation stays near 0 on this dataset. **Arm C measured this
   (§2b): the runtime level fired, but only as `fail` — no observed clear occurred in any case, so
   notAffectedConfirmation did not move. An observed *clearing* record (a `runtime:confirmed`-style pass
   that satisfies R2.4's ClearsBehaviour) is the missing piece; today's passes clear only rendered
   configuration.**
3. ~~**Fix §5.1a**~~ fixed by lane `fixes`; re-measured as A′ (§2a) watching obs-north B1 — the caution
   realised exactly as predicted (unsafeEligibility 1/8, decision still fail-closed).
4. **Granularity** — of composite upstream notes (ingress-nginx #11819: one change item standing for three
   different defaults cannot be cleared or confirmed per default), and now, measured by arm C, of the
   *expected-item matcher*: three runtime-affected findings in obs-north were matched by no expected item,
   so observed evidence exists but never reaches the score (§2b). Groundtruth/scorer decision, routed.
5. **Agent (arm B) must be measured** before any statement about autonomy; it cannot lower a class by
   itself (§3.3), so the agent can at most raise concerns, add paths and request validations. Runtime
   (arm C) is now measured: it raises classes (R1 on observed degradations) but currently cannot clear
   them (§2b).

## 8. Reproducing

```
go build -o bin/ri ./cmd/ri
ri -state <state with the warm upstream/render cache> -offline eval -investigations            # arm A / A′
ri … eval -investigations -runtime -save            # arm C: one throwaway kind cluster per case (deleted after each)
ri eval -investigations -rescore eval/investigations/results/A-20261016T201359Z-294a49e7a9      # offline: re-classify + re-score
```

Each stored run's `run.json` records the binary commit (`294a49e7a9` for A, `cf761e89e5` clean for A′ and C),
the configuration (arm C's lists `runtime.isolated-upgrade@v1` in `config.validators`) and per-case
input/env/expected digests.
