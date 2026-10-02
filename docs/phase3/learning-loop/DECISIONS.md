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
