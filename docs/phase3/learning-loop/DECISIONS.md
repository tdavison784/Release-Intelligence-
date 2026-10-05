# Product-owner decisions — learning loop

Binding decisions by the product owner. Each one records the text verbatim, the
date, and where it is implemented. DESIGN.md, ACTION_CLASSIFICATION.md and
MISSION.md cite these ids where a decision amends them.

## PO-1 — Consensus is two separate model calls agreeing (2026-10-01)

> PO-1 CONSENSUS = two separate model calls agree. Any models, including two separate calls of the same model (e.g. two independent Opus calls). Independence = distinct proposals from separate stateless calls (distinct sp- ids, distinct call/session ids, no shared context), NOT distinct families. Replace IndependentModels with a separate-call check; keep ModelFamily but use it only to label consensus as 'cross-model' vs 'same-model' (record it on the verification entry) so metrics can measure whether same-model agreement is as reliable as cross-model (correlated errors). Proposals must carry a call id (from the provider/CLI envelope) to make separateness checkable.

Implemented in contract-3:
- `domain.Provenance.CallID`, required on `SemanticProposal` and part of `ProposalID`.
- `domain.SeparateCalls` replaces `IndependentModels`.
- `domain.ConsensusScope` (`cross-model` | `same-model`), carried on
  `AspectVerification.Consensus` and `KnowledgeRef.Consensus`.
- `ValidateFactRecords`: ≥2 separate calls, no call counted twice, the label
  matches the agreeing proposals.
- Metrics: `knowledge.FactMetrics.ConsensusAgreementByScope`.

## PO-2 — Models may request ACTION REQUIRED, and agreed consensus may produce it (2026-10-01)

> PO-2 MODELS MAY REQUEST ACTION REQUIRED, AND AGREED CONSENSUS MAY PRODUCE IT. SemanticProposal.SuggestedClass may be 'action-required' (a request). Trust ladder: a knowledge finding may be ACTION REQUIRED when (a) every aspect of the fact is verified at consensus or better (deterministic/human/consensus; proxy-only still capped at review), (b) the consequence kind is action-eligible and every agreeing proposal on the consequence aspect requested action-required, (c) the exposure condition evaluates TRUE deterministically against the environment with environment evidence (both evidence chains — the existing actionFindingEvidence rule is unchanged), (d) no validator refuted any aspect. Such findings carry Knowledge.Verification = consensus visibly, render as 'ACTION REQUIRED · model consensus' (distinct from 'verified'), confidence medium-high is fine but must be labelled; and every consensus-ACTION fact is auto-sampled into human review (R19-style). Unchanged: consensus can never produce NOT AFFECTED (clearing stays trusted-only); a single model/proxy alone is still capped at review; deterministic/human path unchanged. Eval: falseActionRate and actionFindingEvidence reported per verification level including consensus; the pre-registered gates apply to the combined output.

Implemented in contract-3:
- `SemanticProposal.SuggestedClass` may be `action-required`. It is a request and
  needs an action-eligible consequence in the same proposal.
- `VerifiedFact.ConsensusAction`:
  - `Validate()` checks (a) and the eligibility half of (b).
  - `ValidateFactRecords` checks that every agreeing consequence proposal
    requested action (the rest of (b)) and that no validation refutes any
    aspect (d).
- `KnowledgeRef.ConsensusAction`, and `ActionLabel()` = "model consensus".
- `ImpactReport.Validate()` and the schema:
  - ACTION REQUIRED is allowed from a consensus fact only with `consensusAction`.
  - Condition (c) is the unchanged affected-class rule: matches plus both
    evidence chains.
  - NOT AFFECTED stays trusted-only.
  - Proxy facts stay capped at review.
  - A consensus finding may carry high confidence only on a consensus-action
    ACTION REQUIRED.
- 100% audit sampling, and the per-level eval reporting of `falseActionRate` and
  `actionFindingEvidence`, are assigned to the knowledge and applicability lanes
  (DESIGN.md §6, §7).

**Amends** MISSION Goal 21 and docs/ACTION_CLASSIFICATION.md §8. PO-2 relaxes,
by product-owner decision, the earlier rule that a model may never produce
ACTION REQUIRED.

## PO-3 — Unset changed defaults and new keys are decided by rendering (2026-10-02)

> When a chart default changes (or a key is new) and the customer leaves it unset, decide with
> rendering. The customer render (plus a counterfactual variant: target with the key unset vs pinned
> to the old default) shows a rendered change attributable to the key ⇒ EXPOSED (review-required by
> default; action-required only via a trusted fact or PO-2 consensus-action with an action-eligible
> consequence). Both renders succeed and nothing attributable changes ⇒ NOT AFFECTED, with the render
> check as evidence. Render unavailable ⇒ NOT AFFECTED as today (the product owner's choice), but the
> finding's check must record "render unavailable" visibly. The contract defines the check kinds,
> evidence and rule ids; the render lane implements.

(Recorded verbatim in substance from the commander relay of 2026-10-02. Motivation:
TRUSTFIX.md §5, where `impact:values-unset` read "you do not set the key" as "not affected" for
changed defaults.)

Implemented in contract-4. These apply to `values:default-changed` and `values:added` changes whose
key the customer leaves unset; `impact:values-unset` stays for removed keys only.

| Rule | Class | Required shape (`ImpactReport.Validate()` + schema) |
|---|---|---|
| `impact:values-default-applies` | review-required (stronger only through knowledge) | ≥1 `rendered-change` match whose evidence is an environment render (`Evidence.Render.scope = environment`) |
| `impact:values-default-no-effect` | not-affected | a `render`-dimension check with `render.outcome: no-attributable-change`, citing environment-render evidence |
| `impact:values-default-unrendered` | not-affected (PO choice) | a `render`-dimension check with `render.outcome: unavailable` and a `reason`: the missing render is visible on the finding |

- `ImpactCheck.Render` is `{outcome, key, counterfactual, reason}`. It is set exactly on
  `render`-dimension checks.
- Attribution means: the customer From/To renders differ, and the counterfactual (target with the
  key unset vs pinned to the old default) reproduces the difference.

## PO-4 — Kyverno E9 stays REVIEW; superseded-upstream; narrow refinement by trusted facts (2026-10-02)

> Add consequence kind `superseded-upstream` (removed/changed but replaced by an upstream mechanism)
> ⇒ review-required, not action-eligible. Add a NARROW refinement rule: a TRUSTED fact (every aspect
> human or deterministic) whose subject exactly matches a deterministic finding's subject on the same
> change may refine that finding's class — including downgrading the generic rule's ACTION to
> review-required/informational — never to not-affected, never from consensus/proxy facts; the refined
> finding keeps `refinedFrom` (original class + rule) visible and both evidence chains. Validate()
> enforces all of it.

Implemented in contract-4:
- `ConsequenceSupersededUpstream` → `ExposedClass` review-required, not action-eligible.
- Rule `impact:knowledge-refined`; `ImpactFinding.refinedFrom = {classification, rule, severity}`;
  `KnowledgeRef.subject`. `Validate()` checks:
  - the fact is trusted (consensus, consensus-action and proxy are rejected);
  - the refined class is affected (never not-affected or unknown) and differs from the original;
  - the original was an affected deterministic finding (not a knowledge rule);
  - every match lies within the fact's subject (`Subject.CoversMatch`);
  - the original finding of that rule and change is replaced, not kept;
  - the refined finding keeps the original matches, so it keeps both evidence chains.
- **"Exactly matches", defined narrowly** (`Subject.CoversMatch`):
  - `helm-value` covers values-key matches at or below its path (segment-wise; a sibling such as
    `cleanupJobsExtra` does not count);
  - `crd-field` covers manifest-field matches at or below its path;
  - `image` covers image references of exactly its repository (a mirror under another registry does
    not count);
  - every other family covers nothing.

**Amends** DESIGN.md §1.4 (consequence kinds) and §4 (composition: the one sanctioned downgrade of
a deterministic finding), and ACTION_CLASSIFICATION.md §5/§8.

## PO-5 — Dissent blocks consensus ACTION (2026-10-05)

> A fact may be consensus-ACTION only if NO separate call on that candidate's consequence aspect
> requested a lower class (review-required/informational/unknown), regardless of wording or
> consequence digest. (Expected: flux E6 → REVIEW; strimzi-edge E2 may also drop to REVIEW until
> human-verified — report it.)

(Recorded verbatim in substance from the commander relay of 2026-10-05. Motivation:
TRUST-AUDIT.md §2. On flux E6, two calls of glm-5.3 agreed migration-required and requested ACTION.
Claude Sonnet and Opus asked for REVIEW on the same candidate, but their differently worded
consequences meant they did not count as dissent. The blind proxy audit later corrected the
consequence to a deprecation.)

Implemented in contract-6:
- `domain.ConsequenceDissent(candidates, proposals)` finds the dissenting calls. A call counts when its
  task covers the consequence and its `suggestedClass` is review-required, informational or unknown.
  Calls on other candidates, mapping-only calls and calls without a class request do not count.
- The rule applies while the consequence aspect is consensus-verified. A human-verified consequence
  settles the dissent (the "until human-verified" of the decision).
- Where it is enforced:
  - `ValidateFactRecords`, over all proposals of the fact's candidates;
  - the consensus-action auto-approval policy;
  - `buildFact` (recomputed on every decision merge);
  - a new `RouteStore` refresh pass that re-derives `consensusAction` on stored facts. The file store
    now allows that derived flag to change under the same id.
- The dashboard shows "Consensus ACTION blocked by dissent" with the dissenting calls.

**Applied to the real store** (`ri knowledge route -auto-approve-general`, the loop-run-4 flags).
**All six** consensus-action facts had dissent and lost `consensusAction`. They stay active, capped
at REVIEW:

| fact | case · item | consequence (agreeing calls) | dissent (requested review) | was the ACTION right? |
|---|---|---|---|---|
| vf-d2375ef40b74 | flux E6 | migration-required, glm-5.3 ×2 (same-model) | Sonnet ×2, Opus | **no**: now REVIEW = the label |
| vf-6703cd4a7629 | flux (imagepolicies) | migration-required, glm-5.3 ×2 (same-model) | Opus ×2, Sonnet | n/a (no ACTION finding in the eval) |
| vf-cec2d4a87881 | karpenter (nodepools) | migration-required, glm-5.3 ×2 (same-model) | Opus ×2, Sonnet ×2 | n/a |
| vf-e00d920557d9 | external-secrets E3 | migration-required, 5 calls (cross-model) | Sonnet ×1 | **yes**: a true ACTION lost, now REVIEW |
| vf-22666d02a78e | strimzi-edge E2 (×2 changes) | migration-required, 5 calls (cross-model) | Sonnet ×2 | **yes**: two true ACTION findings lost, now REVIEW |
| vf-d36855dd939d | crossplane (`--registry` removed) | workload-failure, glm-5.3 + Opus (cross-model) | Sonnet ×1 | n/a |

Four new review items were opened on the two cross-model candidates that no longer auto-approve
(crossplane ×3, external-secrets ×1), so a human can settle the consequence. Per-level panels are in
[PO5-PO6-REPORT.md](PO5-PO6-REPORT.md).

## PO-6 — Consensus ACTION is labelled unaudited until a human accepts its audit (2026-10-05)

> A consensus-ACTION finding stays ACTION REQUIRED but is labelled 'ACTION REQUIRED · model
> consensus · unaudited' until a HUMAN accepts its audit item; a corrected/rejected audit supersedes
> the fact (already works). Eval reports consensus-ACTION findings audited vs unaudited separately.

Implemented in contract-6:
- `VerifiedFact.AuditedBy` is the human accept on the fact's audit item. `ValidateFactRecords` proves
  it from the records: a human, an accept, on an item of one of the fact's candidates proposing
  exactly its assertion. A proxy accept never sets it.
- `Queue.Decide` sets it, and the `RouteStore` refresh re-derives it.
- `KnowledgeRef.Audited` drives `ActionLabel()`: "model consensus · unaudited" or "model consensus".
  `ImpactReport.Validate()` allows `audited` only on consensus-action findings.
- The impact finding title and detail state UNAUDITED.
- `ri eval -knowledge` prints, per level, `N ACTION · model consensus: a audited, x false / u
  unaudited, y false`.
- After PO-5 no consensus-ACTION finding remains in either eval view, so every count is 0 today.
