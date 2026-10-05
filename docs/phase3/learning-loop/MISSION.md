# Phase 3 Completion Goal: a Learning, Human-Verified Environment Applicability System

> The mission, recorded verbatim in substance from the product owner's brief (2026-10-01).
> Every lane of the learning-loop work traces back to a numbered goal here.

## Mission

Phase 3 established that Release Intelligence can: discover and normalize upstream release
intelligence with strong recall; perform deterministic structured environment joins; constrain action
classifications by explicit safety rules; preserve provenance and evidence; let the AI layer propose
review-worthy interpretations without letting it create mandatory engineering work; and showed that model
choice materially affects suggestion volume and precision, and that capable models disagree enough that
no single model is ground truth.

The remaining question:

> **Can Release Intelligence turn unstructured upstream changes into reliable, machine-comparable
> knowledge, join that knowledge to a customer's actual environment, and use engineering feedback to
> continuously improve applicability without weakening the safety of `ACTION REQUIRED`?**

Target loop:

```text
Upstream software change → discovery → semantic candidate extraction → model proposal(s)
→ deterministic validation where possible → engineering review where necessary
→ verified semantic knowledge → environment applicability → trusted impact classification
→ feedback stored for future learning
```

## Current measured frontier

critical recall 1.00 · important recall 1.00 · false ACTION 0.00 · unsupported 0 ·
**applicability accuracy ~0.095** · classification accuracy ~0.46 · **unknown rate ~0.73**.
Many expected actionable changes become UNKNOWN because prose-derived changes carry no
machine-comparable semantics. Central goal: **move justified findings out of UNKNOWN through better
semantic extraction and verification, not through weaker confidence rules.**

## Primary success target

`applicabilityAccuracy >= 0.80` while preserving criticalRecall >= 0.95, importantRecall >= 0.90,
falseActionRate < 0.05 (preferably ~0), ACTION evidence coverage = 1.00, unsupported = 0, pipeline
failures = 0. UNKNOWN may remain where evidence genuinely does not support a conclusion.

## Goals

1. **Structured semantic release knowledge.** Transform statements like "The default rotationPolicy is
   now Always" into machine-comparable candidates: `subject` (type crd-field, group, kind, path),
   `change` (default-changed, from Never, to Always), `applicability` (fieldState: unset),
   `consequence` (behavior-change). Subject families: Helm values, CRD fields, GVK/API versions, feature
   gates, image refs, config keys, RBAC permissions, CLI flags, env vars, compatibility boundaries,
   APIs/endpoints, protocol behavior, Terraform resources/attributes, cross-product relationships,
   migration requirements. **No product-specific Go logic; constructs stay generic and declarative.**
2. **AI is a knowledge-proposal layer.** Models propose subject, change type, before/after,
   applicability condition, consequence, migration relationship, candidate classification — citing
   evidence. Full provenance: model, provider, modelVersion, promptVersion, promptDigest,
   inputEvidenceIDs, generatedAt, confidence. A proposal stays distinguishable from a
   deterministically-verified fact and from a human-verified fact.
3. **Multi-model comparison preserved.** Run several models (GLM-5.3-Flash, Claude Sonnet/Opus/Haiku,
   future) against the same semantic candidates; store each proposal independently; store disagreement
   explicitly; never collapse outputs before review.
4. **Measure disagreement.** Agreement rate, pairwise agreement, accepted-human outcome by model,
   FP/FN rate by model, review volume by model, grounding quality by model — answer "which models are
   best at which semantic tasks" (CRD extraction, Helm applicability, migration synthesis, duplicate
   detection, cross-product compatibility, behavior-change interpretation).
5. **Deterministic validation where possible.** CRD field (source/target/published CRD, GVK, schema
   field, defaults); Helm values (values.yaml, values.schema.json, published chart, source/target diff);
   compatibility (structured tables, Helm kubeVersion, vendor matrices); images (OCI manifests, Helm
   templates, deployment artifacts). Deterministically-confirmed proposals become verified semantic
   knowledge without human review.
6. **First-class engineering review queue.** Unvalidatable proposals become durable review items
   (ambiguity, model disagreement, missing semantics, cross-product reasoning, consequence
   interpretation, classification uncertainty) — id, product, release, type, sourceChange,
   modelProposals, proposedFact, status.
7. **Engineering review dashboard.** Lightweight, review-only (not the product UI). Inbox counts
   (pending, model disagreement, needs semantic mapping, applicability questions, needs more evidence).
   Filters: product, release, subject type, review type, severity, confidence, model, disagreement,
   upstream source, status, reviewer.
8. **Fast, evidence-driven decisions.** Each item shows product/release, upstream statement, structured
   proposal, environment context, deterministic validation results, model proposals, agreement,
   upstream evidence, environment evidence, previous related decisions, and the concrete question.
   Actions: ACCEPT, REJECT, CORRECT, NEED MORE EVIDENCE, DEFER.
9. **Correction is first-class.** Preserve original proposal and final corrected value.
10. **Review question types:** semantic-mapping, applicability, consequence, classification,
    relationship, duplicate, evidence-sufficiency.
11. **Feedback becomes durable knowledge.** Accept/correct → verified semantic fact → knowledge graph →
    future joins → future ImpactReports. Reject → retained negative example. The review record holds
    candidate, model proposals, evidence, decision, correction, reviewer, timestamp, reason, resulting
    knowledge entity.
12. **Approve knowledge, not reports.** A verified release-level fact is reused across environments
    (A affected, B not affected, C unknown) without re-review. Essential for scalability.
13. **Human feedback dataset.** Every decision is a labelled example. Labels: accepted, rejected,
    corrected, insufficient-evidence, wrong-subject, wrong-change-type, wrong-applicability,
    wrong-consequence, wrong-classification, duplicate.
14. **Feedback before fine-tuning.** Use it first for evaluation, prompt iteration, candidate
    selection, confidence calibration, deterministic rule discovery, few-shot retrieval, model routing,
    small classifiers. No fine-tuning in this phase.
15. **Model-to-human accuracy** per model: accepted as-is / after correction / rejected / insufficient.
    Also important findings surfaced, review volume, grounding, human time.
16. **Disagreement as a routing signal (hypotheses until data supports them):** agree + validation
    succeeds → auto-verification candidate; agree + no validation → lower-priority review; disagree →
    review; all uncertain → missing-evidence queue; high-impact consequence → high-priority review.
17. **Attack ACTION→UNKNOWN failures.** Per failure: why unknown, cause class, missing structure,
    multi-model extraction, automatic validation, review, store fact, rerun eval. Track
    UNKNOWN → verified relationship → ACTION/REVIEW/INFORMATIONAL/NOT AFFECTED. Don't force genuinely
    unknowable cases.
18. **Every UNKNOWN carries a reason:** release-knowledge-gap, environment-visibility-gap,
    cross-product-context-gap, runtime-behavior-gap, evidence-gap, semantic-ambiguity. Drives routing.
19. **Cross-product environment intelligence.** Explicit environment product inventory (Kubernetes,
    cert-manager, ingress-nginx, Cilium, Argo CD, …). Semantic facts can express "requires ingress-nginx
    >= X". First-class knowledge, not special-case logic.
20. **Runtime decisions deterministic whenever possible.** verified fact + structured env facts →
    deterministic applicability → classification. Models operate at extraction/proposal/review time,
    not on every runtime decision.
21. **ACTION REQUIRED contract preserved.** Requires verified upstream fact + verified environment
    exposure + verified consequence + high confidence. A model may never directly produce it. Allowed
    path: AI proposes → automatic or human verification → trusted knowledge → deterministic join →
    ACTION REQUIRED.
    *Amended by product-owner decision PO-2 (2026-10-01, [DECISIONS.md](DECISIONS.md)):* models may
    request ACTION REQUIRED, and an agreed consensus of separate model calls (PO-1) may produce it.
    This requires every aspect at consensus or better, an action-eligible consequence that every
    agreeing call requested, deterministic exposure on both evidence chains, and no refuted aspect. The
    finding is labelled "ACTION REQUIRED · model consensus", and every such fact is sampled into human
    review. Consensus never produces NOT AFFECTED; a single model or proxy stays capped at REVIEW.
    *Narrowed by PO-5/PO-6 (2026-10-05):* any call on the consequence that requested a lower class
    blocks the consensus path (dissent counts whatever its wording), and the finding reads
    "ACTION REQUIRED · model consensus · unaudited" until a human accepts its audit item.
22. **Expand applicability ground truth.** Environment cases label expected semantic subject,
    applicability, action class, environment evidence, consequence. Prioritize prose-only defaults,
    feature gates, cross-product version dependencies, RBAC, migrations, behaviour changes,
    deprecations, required new config, compatibility hidden in prose.
23. **Real human validation.** 4–8 experienced engineers (platform, SRE, DevOps, Kubernetes, infra)
    use the dashboard. Measure correctness, time to decision, evidence clarity, proposal usefulness,
    false mandatory work, missed critical work, trust, usefulness, review fatigue. Central question:
    would an experienced engineer use this before a real upgrade and trust its mandatory findings?
24. **Human review cost.** Items per release/product, median review time, auto-validation rate,
    acceptance/correction/rejection rate, repeat-pattern rate. Desired trend: reviews/release ↓. High
    volume every release is important negative evidence.
25. **Thin end-to-end loop.** Engine → semantic candidate generator → multi-model proposal layer →
    deterministic validation → review queue → dashboard → verified knowledge store → applicability
    engine → impact report ↺ feedback dataset. Lightweight local service, database, web UI.

## Non-goals

Temporal, Iceberg, data lake, Kubernetes controllers, multi-tenancy, billing, enterprise auth,
production RBAC, fleet management, polished SaaS UI, autonomous rollout, generalized PR automation.

## Completion gates

criticalRecall >= 0.95 · importantRecall >= 0.90 · applicabilityAccuracy >= 0.80 ·
falseActionRate < 0.05 · ACTION evidence coverage = 1.00 · unsupported = 0 · pipelineFailures = 0.
Plus: no adversarial case produces a false ACTION REQUIRED; review decisions feed reusable knowledge;
model outputs separately attributable; real engineers complete the review protocol; review volume and
time measured; dashboard supports accept/reject/correct/need-more-evidence/defer; reviewed examples
retained as reusable labelled data.

## Required demonstration

1. Discover an upstream prose change. 2. GLM proposes "plausibly applies to Certificate.rotationPolicy".
3. Sonnet proposes "undetermined". 4. Artifact inspection verifies the field exists and the default
changed. 5. Applicability cannot be fully proven automatically. 6. Dashboard asks: "Does a Certificate
with rotationPolicy unset become affected by this default change?" 7. Engineer: ACCEPT. 8. The semantic
rule is stored. 9. The environment has 11 Certificates with rotationPolicy unset. 10. The deterministic
engine produces ACTION REQUIRED. 11. Future environments reuse the verified fact without re-review.

> **Machines propose. Evidence constrains. Engineers resolve ambiguity. Verified knowledge compounds.**
