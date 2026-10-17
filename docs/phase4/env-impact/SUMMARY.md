# Phase 4 summary — environment-aware breaking-change investigation (2026-10-16)

Branch `p4-env-impact` (not pushed). Goal: `MISSION.md`. Design: `DESIGN.md`. Evaluation: `EVAL-REPORT.md`
(`ri eval -investigations`, frozen gates in `eval/investigations/gates.yaml`, 17 stored runs). Fleet log:
`COMMANDER.md`.

## What exists now

`ri investigate <product> <from> <to> --repo <gitops-repo> [--agent] [--runtime] [--temporal]` takes a dependency
upgrade and a GitOps environment and produces an evidence-backed, four-class finding per in-scope change item:

1. **Change evidence + environment snapshot → evidence bundle** (`internal/investigate/{bundle,store}`):
   content-addressed, reproducible from inputs (bundle ids independent of checkout/commit), Secret values redacted
   before hashing, knowledge facts stored with the bundle.
2. **Render diffs — the centerpiece** (`internal/investigate/renderdiff`): the environment's own FROM/TO renders
   (Argo CD / Flux / helmfile / Kustomize; per-release values stacks applied exactly as each tool does), stored as
   artifacts, every rendered change indexed, correlated to upstream change items, and queryable. Rendered changes
   nothing upstream explains become investigated items (`scope:rendered-unexplained`); 0 uncorrelated changes across
   the 8 evaluation environments.
3. **Relationships (secondary)** (`internal/investigate/relations`): declarative `relationships:` and `reconciles:`
   catalog rules (cited upstream docs), bounded deterministic DFS, unresolved edges recorded.
4. **Deterministic validators** (`internal/investigate/validate`): render diff (exposure is inconclusive; only
   concrete failures fail), override inspection (pinned settings from trusted, release-level knowledge only), schema
   compatibility, static references (TO vs FROM with namespace semantics), join verdicts. Cannot decide ⇒ never pass.
5. **Runtime validation** (`internal/investigate/runtime`, opt-in): throwaway kind cluster, FROM → TO, declared
   exercises with observed controller reactions, generic observations, attribution precedence; observed-level
   pass/fail; clusters always deleted.
6. **Investigation agent** (`internal/investigate/agent`, opt-in): MCP tools over the bundle (diff queries first),
   one short session per prioritised item, every citation and setting claim verified before it is recorded,
   budgets, transcripts, byte-identical offline replay. It can raise concern and request validation; it can never
   lower a class.
7. **Fail-closed four-class policy** (`internal/investigate/classify`): Affected / Not Affected / Potentially
   Affected / Insufficient Evidence; render silence never clears a controller-behaviour change; approval eligibility
   computed but never enacted in milestone 1; ACTION REQUIRED contract untouched.
8. **Temporal orchestration** (`internal/orchestrate`, `ri worker`): same pipeline in-process and on Temporal
   (parity-tested), retries, timeouts, idempotency keys, cancellation, human-review signal with fail-closed timeout,
   hash-chained audit trail.
9. **Human review** (`ri review serve`): the default view shows only the items whose human verification would let
   investigations clear environments (5 today); everything else is one link away.

## Acceptance criteria (MISSION AC1–AC8)

| AC | Status | Evidence |
|---|---|---|
| AC1 upgrade analysed against a specific environment | **met** | `ri investigate`; 8 evaluation environments, 0 pipeline failures |
| AC2 complete traceability to evidence | **met** | citationResolution 100 % in every run; content-addressed bundles; 8/8 reproducible record digests |
| AC3 traversal of dependency/config relationships | **met (secondary)** | bounded deterministic DFS + declarative relationship rules; diffs are the primary evidence (PO clarification) |
| AC4 AI investigation that inspects and requests artifacts | **met, no effect yet** | arm B: 8/8 cases, 46 concerns, 20 validation requests, 82 verified claims — 0 class changes (see below) |
| AC5 ≥1 affected and ≥1 unaffected case verified against known outcomes | **partly** | affected confirmed: kube-prometheus-stack obs-north (deterministic dangling reference + observed rejection). Unaffected: confirmed per item (2/20 deterministic, +1 observed clearing in shop-nl) but no whole case is cleared yet |
| AC6 validated conclusions vs unresolved assumptions | **met** | finding basis (deterministic / observed / none), AI records labelled, unverified prose labelled, assumptions listed |
| AC7 Temporal with retry, failure handling, audit | **met** | parity + testsuite tests, live dev-server runs, hash-chained audit |
| AC8 human review when evidence is insufficient | **met** | every non-eligible decision requires review; review UI + `ri investigate review` |

## Evaluation (8 cases, frozen gates) — reference runs

| Gate | A₅ deterministic | C₅ + runtime | B + agent |
|---|---|---|---|
| falseNegativeRate (affected reported not-affected) | 0/3 — INSUFFICIENT-DATA (min 4) | 0/3 | 0/3 |
| unsafeEligibilityRate | 1/8 FAIL ¹ | 1/9 FAIL ¹ | 1/8 FAIL ¹ |
| falsePositiveRate | 1/20 PASS | 1/20 PASS | 1/20 PASS |
| affectedDetection | 3/3 PASS | 3/3 PASS | 3/3 PASS |
| affectedConfirmation | 1/3 FAIL | 1/3 FAIL | 1/3 FAIL |
| notAffectedConfirmation | 2/20 FAIL | 3/20 FAIL | 2/20 FAIL |
| exposureRecall | 6/7 PASS | 6/7 PASS | 6/7 PASS |
| evidenceCoverage | 23/71 FAIL | 23/71 FAIL | 25/71 FAIL |
| unresolvedRate / citationResolution / reproducibility / pipelineFailures | PASS | PASS | PASS |
| observedAgreement | 1/21 FAIL | 2/21 FAIL | 1/21 FAIL |

¹ Only three per-key items (`kubeDns`/`kubeProxy`/`kubeEtcd` `.serviceMonitor.bearerTokenFile removed`) for
components this environment does not render — correct per key, but matched to one coarse expected item together
with the deciding (correctly affected) coreDns item. Recorded as a ground-truth granularity limitation; the matcher,
format and expectations were not changed after seeing results.

First scored run of the day for comparison: affectedDetection 1/3, exposureRecall 2/7, unresolved 15/23,
notAffectedConfirmation 0/20.

## The structural finding

The system is **safe** (no false negatives, fail-closed everywhere) but **not yet useful for clearing**: it
detects every affected case and confirms one, yet clears almost nothing. The reason is the same in every layer:
clearing a controller-behaviour change needs **trusted knowledge that a setting pins the old behaviour**. The agent
verified 82 setting facts in the environments (some are exactly the pinning settings), but by design an AI claim
can never be the trusted rule that the setting *pins* the behaviour. The lever is the learning loop: agent-proposed
pin rules → human verification once per release → reused for every environment. The review UI's 5 pin-source items
are the first instances.

## Known limitations

- Ground-truth granularity: one upstream note can cover several change items (ingress-nginx #11819; kube-prometheus-stack
  bearerTokenFile keys).
- falseNegativeRate has n=3 affected cases (< min sample 4): the safety gate cannot pass yet — more affected cases needed.
- ingress-nginx risk-level default: no pin-able fact exists yet; those environments stay fail-closed.
- Agent: one non-deterministic `submit_findings` result in obs-south replay (classes unchanged); claim nits
  (nested values, comment matches).
- Runtime: breaks on objects no item changed in the render stay inconclusive (no guessing); one cluster at a time,
  ~3–4 min per case.
- Several releases deploying one product with different values are a gap, not evaluated separately.

## Recommended next steps

1. Human-verify the 5 pin-source items (`ri review serve`, see COMMANDER.md), then re-score A/C.
2. Route agent-proposed pin rules into the review queue (the learning-loop link between §A4 and §A5).
3. Add affected ground-truth cases (≥4) so falseNegativeRate becomes measurable; refine expected-item granularity
   blind, before scoring.
4. Adopt the provider-window ledger (proposal in `proposals/`) after sign-off.
