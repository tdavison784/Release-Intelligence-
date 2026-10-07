# Trust audit — three anomalies in the proxy-4 eval runs

Lane `analysis` (trust-audit), 2026-10-03. Branch `p3ll/trust-audit`, based on `p3-learning-loop` @
ef40e7e (proxy-4 merged). Runs:
- `-knowledge knowledge -render` (real store);
- `-knowledge docs/phase3/learning-loop/proxy-shadow/eval-knowledge -render` (shadow view, 339 facts).

Integrity: no fact, decision or eval data was edited. Two generic fixes, each with an adversarial
test (§4).

## 0. Answers first

| # | anomaly | verdict |
|---|---|---|
| 1 | shadow proxy row `0.681 (34/62)`, every other row `/71` | **Bug, two of them.** (a) The kyverno report built with the shadow proxy facts failed `Validate()`; a rendered-change check lacked its render record. (b) The evaluator turned that error into a *silent* nil report, so the case dropped out of every denominator (9 affected links: 71 → 62) without counting a pipeline failure. Both are fixed. The row now reads **0.673 (40/71)** |
| 2 | real store consensus/proxy `ACTION 21 (2/0)`, `4 ACTION · model consensus` | False ACTION 1 is **kyverno E9**: the pre-existing deterministic `values-removed` label dispute (TRUSTFIX), not knowledge. False ACTION 2 is **flux E6**, from consensus-action fact **vf-d2375ef40b74**. PO-2 is **met to the letter**: the consequence is a **same-model** consensus (two `glm-5.3` calls); Claude Sonnet and Opus dissent on the class but are not counted. Its mandatory 100% human sample is still pending. No code violation; two contract gaps (§2) |
| 3 | shadow proxy `ACTION 19 (0/0)`: the kyverno E9 false ACTION vanished | **No fact refined or suppressed it.** There is **no `impact:knowledge-refined` finding at any level** in either view, so there is no PO-4 violation. The finding vanished because the **whole kyverno report was discarded** (anomaly 1). With the fixes the shadow proxy row reads **ACTION 20 (1/0)**: kyverno E9 is back and honest |

## 1. Anomaly 1 — the shrinking denominator

**What happened.**
1. At the proxy level, shadow fact **vf-199145427d4e** (kyverno 1.13.0, candidate `sc-194c2ca12ce9`;
   subject/applicability/consequence proxy, change deterministic) attaches to `chg-89eb7773aec8`
   (policy exceptions disabled by default).
2. Its exposure is `all[gvk-in-use kyverno.io PolicyException, rendered-change apps Deployment
   …containers[].args changed "--enablePolicyException=false"]`.
3. The rendered-change leaf **decided false**: every complete render shows no such arg change. The
   render evaluator built that false's check as `{dimension: render, …}` **without the `render`
   record**.
4. `ImpactReport.Validate()` requires a render-dimension check to carry exactly that record. So
   `impact.Build` failed with `assembled report is invalid: finding imp-3b65fa029afb: a render record
   is carried exactly by render-dimension checks`.
5. No earlier fact had a rendered-change leaf decide false, which is why this never fired before.
   The real store doesn't contain this fact.

**Why it hid.**
- `runCase` replaced the impact error with a nil report and passed no error to `ScoreEntry`.
- `ScoreEntry` gives an environment case without a report `Env{}`, i.e. **zero links**.
- The kyverno case's 9 affected links (and 1 not-affected link) left the applicability count, its
  115 findings left every other count (including the E9 false ACTION), and `pipelineFailures` stayed
  0.
- Only the proxy level was affected; the other rows didn't include the fact.
- The code comment claimed "all environment expectations miss"; the code did the opposite.

**It is not the per-level scorer.** Every level runs the same cases; the link set changed because
one level's report failed.

**Fixes (§4).** Render record on the false check; a missing report becomes a pipeline failure that
keeps its links as misses; failures are visible in the per-level panel.

| shadow proxy row | applicability | ACTION (false) |
|---|---|---|
| before | 0.681 (34/62) | 19 (0) |
| evaluator fix only | 0.614 (34/71), `✗ 1 PIPELINE FAILURE` | 19 (0) |
| both fixes | **0.673 (40/71)** | **20 (1)** |

## 2. Anomaly 2 — the false ACTIONs in the real store

Consensus level (36 facts) and proxy level (216 facts) show the same ACTION findings. Proxy facts
cannot produce ACTION.

| case · link (label) | rule | fact | verdict |
|---|---|---|---|
| external-secrets E3 (action) | `knowledge-exposed` · ACTION · model consensus | vf-e00d920557d9 (`sc-dd35e147bb75`) | correct |
| strimzi-edge E2 (action) × 2 changes | same | vf-22666d02a78e (`sc-1fdd85cb942b`) | correct |
| **flux E6 (review)** | same | **vf-d2375ef40b74** (`sc-b4d0f27d94a8`, `chg-684b2500985c`, "Storage version of `imagerepositories.image.toolkit.fluxcd.io` changes v1beta2 → v1") | **false**: over-classification |
| **kyverno E9 (review)** | `impact:values-removed` (deterministic) | — | **false**: the TRUSTFIX §1b label/contract dispute, unchanged |

### vf-d2375ef40b74 against PO-2

The assertion:
- subject: gvk ImageRepository v1beta2;
- change: deprecated → v1;
- exposure: `resource CustomResourceDefinition imagerepositories…[status.storedVersions[] equals "v1beta2"]`;
- consequence: `migration-required` / action-required.

| PO-2 condition | status | evidence |
|---|---|---|
| every aspect ≥ consensus, no proxy | ✓ | subject, change **deterministic** (val-5a8237c33923 restatement, val-fed517a33875 crd); applicability **consensus cross-model** (glm-5.3, claude-sonnet-5-5, claude-opus-5-5); consequence **consensus same-model** (sp-21ba3b75f0b3, sp-deb2d9954640: both `glm-5.3`, distinct call ids) |
| action-eligible consequence; every *agreeing* consequence proposal requested action-required | ✓ (letter) | both glm-5.3 proposals suggest action-required |
| exposure true deterministically, both chains, high confidence | ✓ | the flux fixture's CRD document stores v1beta2 (decidable since `3af506e6` made CRD documents resources) |
| no validator refuted any aspect | ✓ | all validations confirm or are inconclusive |

So **the fact is a valid consensus-action fact, and the false ACTION is legitimate under the
current contract**. It exposes two gaps.

1. **Class dissent is not counted.** The candidate has six proposals.
   - claude-sonnet-5-5 ×2 and claude-opus-5-5 assert the *same* consequence kind
     (`migration-required`) but **request review-required**.
   - Their consequence statements are worded differently, so their aspect digests don't "agree".
     They are not part of the basis, and PO-2's "every agreeing proposal requested action" never
     sees them.
   - The ACTION therefore rests on two calls of one model while two other models explicitly asked
     for review.
   - The consequence statement itself describes a deprecation ("…before a later release removes
     v1beta2"). DESIGN §1.4 maps that to `deprecation` → REVIEW. This is the correlated-error risk
     DESIGN §2.6 asked to measure for same-model consensus.

   *Option:* PO-2 also requires that no proposal on the fact's candidates requests a softer class
   for an action-eligible consequence (or that the consequence consensus is cross-model). Caution:
   the (correct) strimzi-edge E2 ACTION also has two dissenting Sonnet proposals, so a strict
   dissent rule costs a true ACTION too. **Commander/contract decision**; not changed here.
2. **The 100% human sample does not gate the action.** All three consensus-action facts have their
   sample item pending:
   - `ri-26a5ac4d6489` (ES);
   - `ri-ae84eec0ef2b` (flux);
   - `ri-e33989842cd6` (strimzi).

   ACTION is emitted *before* anyone looks. In the shadow view the blind proxy decided the flux
   sample: decision `rd-950ffb257e53`, **correct** ("…existing ImageRepository objects stay stored
   as v1beta2 and still work, and migration is needed only before a later release removes…"). That
   superseded the fact with vf-515cc6711c18, which is why the shadow consensus row shows 3
   consensus ACTIONs, not 4.

   *Option:* a consensus-action fact produces ACTION only once its sample item is decided, and is
   capped at review until then. **Contract decision.**

## 3. Anomaly 3 — the kyverno E9 ACTION that vanished

- Every ACTION and every `impact:knowledge-refined` finding was enumerated in both views at the
  consensus and proxy levels: **zero refined findings anywhere**.
- No proxy or consensus fact replaced, refined or downgraded the deterministic `values-removed`
  finding.
- The finding disappeared only because the kyverno report failed validation and was discarded
  (§1).
- After the fixes, the shadow proxy kyverno report validates and carries the E9 false ACTION again
  (`ACTION 20 (1/0)`).
- `validateRefinements` (PO-4) already rejects untrusted refinements. **No trust-ladder violation
  exists to fix.** The commander's hypothesis was the right thing to check; the evidence rules it
  out.

## 4. Fixes on this branch (generic, adversarial tests)

| commit | what | test |
|---|---|---|
| `render: a decided-false rendered-change leaf carries its render record` | the decisive-false check now has `render: {outcome: no-attributable-change, key: <object path>}` and cites the environment-render evidence it examined, as `Validate()` requires | `TestEvaluateRenderedChangeFalseCarriesRenderRecord` |
| `eval: a missing impact report is a pipeline failure and keeps its links` | an impact error is recorded (`pipelineFailures`); a case without a report keeps its links as misses, not-affected links unearned, undecided links unanswered, expected findings unfound; the per-level panel shows `✗ N PIPELINE FAILURE(S)` | `TestMissingReportKeepsLinksAndFails`, `TestLevelPanelShowsPipelineFailures` |

Verification:
- `go build ./... && go vet ./... && go test ./...` green.
- Plain `ri eval`: no regressions against `eval/results`.
- Real-store panel unchanged (no report there fails).
- Shadow proxy row 0.681 (34/62) → **0.673 (40/71)**, ACTION 19 (0/0) → **20 (1/0)**.

## 5. For the commander

1. Correct the shadow proxy-4 number: **0.673 (40/71)**. The published 0.681 (34/62) measured 62
   links.
2. Decide the two PO-2 gaps: count class dissent; gate consensus-action on the 100% sample.
   Until then, every "ACTION · model consensus" finding in the real store rests on unreviewed
   consensus.
3. The kyverno E9 false ACTION persists in every view (TRUSTFIX §1b, still undecided).
4. Guard: any future validation failure at a non-gate level is now visible in the level panel and
   cannot silently change a denominator.
