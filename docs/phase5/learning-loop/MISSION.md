# Phase 5 mission — close the agentic evidence-to-decision loop

> **Binding scope note from the product owner (2026-10-17) — read this before the goal below.**
> Do not interpret this goal as authorization to build another massive subsystem. The Phase 4 implementation
> already contains most of the infrastructure required. What is wanted next is a **controlled experiment proving
> the complete learning loop**. The strongest result is a report showing that the deterministic classifier
> couldn't establish whether a cert-manager upgrade was safe, the agent correctly identified the
> environment-specific mitigation, verification established that the mitigation was valid, and the classifier
> could then safely clear that change. Repeat the process against a known affected environment and demonstrate
> that it does not clear. Those two outcomes are the deliverable; everything else in the goal is secondary and is
> done only where the experiment needs it.
>
> Concretely: cert-manager 1.17.2 → 1.18.0, `Certificate.spec.privateKey.rotationPolicy` default Never → Always;
> payments-us (known not affected) vs payments-eu (known affected) — `eval/investigations/cert-manager-1.17-1.18--*`.

---

# Goal: Close the Agentic Evidence-to-Decision Loop (verbatim)

**Release Intelligence — Phase 5**

## Objective

Complete the agentic investigation architecture of Release Intelligence by establishing a reliable, reproducible mechanism for transforming collected change evidence, environment-specific context, and rendered configuration differences into accurate, verifiable breaking-change impact determinations.

Phase 4 demonstrated that Release Intelligence can collect extensive evidence, identify potential exposure, generate environment-specific render diffs, and enable an AI agent to investigate dependency changes.

However, evaluation exposed a critical architectural gap:

**The investigation agent can discover and verify environmental facts, but the system cannot consistently translate those findings into trusted behavioral conclusions that improve impact classifications.**

The next phase must close this gap.

The goal is to enable an investigation agent to determine not only what changed and whether an environment is exposed, but **whether the proposed dependency change will actually break the target environment, why, and what evidence supports that conclusion.**

Agent reasoning must be measurable independently of automated approval policy, while verified findings must be able to contribute to authoritative classifications without compromising safety.

## Core Principles

1. **Context before reasoning:** Provide structured, change-specific environmental context rather than requiring the agent to reconstruct an investigation from disconnected artifacts.
2. **Rendered diffs as primary evidence:** Concrete before-and-after Helm/Kustomize renders remain central. Runtime behavior and upstream semantics must also be examined when renders are insufficient.
3. **Semantic understanding:** Distinguish the existence of an environment setting from proof of what that setting does across a particular version transition.
4. **Agent reasoning is measurable:** The agent must produce an independent, provisional impact assessment that can be evaluated against known outcomes, regardless of production policy restrictions.
5. **Evidence must become actionable:** Verified agent findings must be consumable by deterministic validators, trusted knowledge, and classification policy.
6. **Knowledge should be reusable:** A behavioral compatibility rule verified for a specific release transition should be reusable across applicable environments without repeated human verification.
7. **Fail-closed production decisions:** Unsupported agent assertions, incomplete evidence, and unvalidated mitigation hypotheses must never independently authorize automatic approval.
8. **Measure usefulness, not activity:** Success is determined by improved classification correctness, verifiable safe-change determinations, and reduced unnecessary human investigation—not model agreement, token consumption, or the number of generated assertions.

---

## Architecture and Requirements

### 1. Change-Specific Impact Dossier

Introduce a structured impact dossier constructed before agent investigation.

The dossier must consolidate relevant information about a proposed upgrade and its target environment.

**Change evidence**
- Current and candidate dependency versions.
- Release notes, migration guides, and breaking-change findings.
- Available source, configuration, schema, and API differences.
- Explicit and implicit behavioral changes.
- Relevant compatibility requirements.

**Environment evidence**
- Target environment identity and immutable snapshot.
- Effective Helm values and Kustomize configurations.
- GitOps deployment definitions.
- Relevant workloads, services, consumers, and dependencies.
- Explicit overrides and inherited defaults.
- Available runtime observations.

**Concrete change evidence**
- Before-and-after rendered manifests.
- Object-level and field-level differences.
- Correlated upstream and environmental changes.
- Relevant unchanged configuration demonstrating preserved behavior.
- Changes that cannot be explained by upstream evidence.

**Existing knowledge**
- Previously verified release-specific behavioral rules.
- Known configuration mitigations.
- Historical validation outcomes.
- Provenance and verification levels.

The dossier must identify evidence that is unavailable, incomplete, or contradictory.

Avoid indiscriminately loading every artifact into the model context. Provide a focused initial investigation context with tools to retrieve additional evidence as required.

**Deliverable:** A reproducible, change-scoped impact dossier that exposes both the evidence supporting an investigation and its known gaps.

### 2. Independent Agent Impact Assessment

Extend the existing investigation agent to generate an explicit, structured, provisional impact assessment.

For each relevant change, the agent must determine:

- What changed between versions?
- What behavior or interface could break?
- How does the target environment consume that behavior?
- Does the environment actually exercise the affected functionality?
- Do effective configurations preserve the previous behavior?
- What do the rendered differences establish?
- Which downstream consumers could experience an impact?
- What additional evidence is required?

The agent must produce one of four provisional classifications:

- **Affected**
- **Not Affected**
- **Potentially Affected**
- **Insufficient Evidence**

Each assessment must include:

- Proposed breaking mechanism.
- Environment-specific reasoning.
- Supporting evidence and citations.
- Verified facts versus inferred assumptions.
- Potential mitigating settings or conditions.
- Remaining uncertainty.
- Proposed validation actions.
- Explanation of what evidence would overturn the assessment.

The agent must be permitted to propose that an environment is unaffected, even when the authoritative deterministic classifier remains inconclusive.

**These provisional conclusions must remain advisory and must not directly modify production approval eligibility.**

Store both the agent assessment and authoritative policy determination independently so that disagreements can be measured and investigated.

**Deliverable:** An independently measurable agent verdict for every investigated change.

### 3. Structured Impact Hypotheses and Verification

Introduce an explicit evidence-to-proof mechanism.

The agent must be able to propose testable hypotheses describing relationships between upstream changes and environmental behavior.

A hypothesis should describe:

- The upstream behavioral change.
- The configuration or workload exposed to that change.
- The expected impact.
- A proposed mitigation or behavior-preservation condition.
- The upstream evidence supporting the claimed relationship.
- The specific validation required to confirm or refute it.

For example:

> An upstream dependency changes a controller's default behavior. The target environment explicitly configures the previous behavior. The agent hypothesizes that this setting prevents the breaking impact.

The validation system must independently evaluate the hypothesis.

Verification may include:

- Effective configuration inspection.
- Rendered configuration comparison.
- Version-specific upstream documentation.
- Source-level behavioral evidence.
- API or schema compatibility validation.
- Targeted runtime exercises.
- Human verification of semantic relationships.

Differentiate between:

1. A verified environmental fact.
2. A verified upstream behavioral rule.
3. A verified relationship between that rule and the environment.
4. An observed runtime outcome.

A verified setting alone must not be treated as proof that it preserves behavior.

Where verification succeeds, generate a structured proof record containing evidence, applicable version boundaries, environmental conditions, validation method, and limitations.

Where verification fails or remains inconclusive, preserve the uncertainty and route the hypothesis appropriately.

**Deliverable:** Agent-proposed hypotheses that can be independently confirmed, rejected, or escalated.

### 4. Complete the Trusted Knowledge Feedback Loop

Resolve the Phase 4 gap between human-verified knowledge and the classification engine.

Ensure that:

- Human review considers the complete verification state of a knowledge fact.
- Required semantic aspects are verified before a fact becomes trusted.
- Trusted facts are accessible to the relevant validators.
- Verified behavioral relationships can be applied to matching environment configurations.
- Successful validation results can legitimately change an authoritative classification.
- Superseded, contradictory, or version-inapplicable facts cannot silently clear changes.

Introduce a clear flow:

**Agent hypothesis → Evidence verification → Trusted knowledge → Environment applicability validation → Classification**

The agent may propose reusable compatibility rules, but cannot independently promote them to trusted status.

Verified knowledge must retain its provenance, version applicability, and assumptions so that future investigations can reuse it safely.

**Deliverable:** A proven end-to-end path from agent-discovered evidence to a trusted classification decision.

### 5. Improve Agent Investigation Efficiency

Use the impact dossier and existing investigation tools to reduce redundant reasoning.

The agent should prioritize changes where additional investigation could materially affect the final decision.

Prefer targeted retrieval and validation over exhaustive examination of every available artifact.

Measure:
- Tokens consumed per investigation.
- Investigation duration.
- Useful verified claims.
- Successful validation requests.
- Additional confirmed affected/unaffected classifications.
- Human review work avoided.
- Evidence retrieval and reasoning failures.

Optimize for evidence-backed decisions per unit of cost, not maximum agent activity.

Maintain reproducible records of tool use, evidence, prompts, and model outputs.

**Deliverable:** A focused investigation strategy whose reasoning quality and operational cost are measurable.

---

## Evaluation Plan

### Phase A — Establish a Controlled Baseline

Preserve the existing Phase 4 evaluation results.

Use the same evidence snapshots and ground-truth cases to compare:

- **Arm A:** Deterministic investigation without an agent.
- **Arm B:** Deterministic investigation plus independent agent assessment.
- **Arm C:** Deterministic investigation plus agent-generated hypotheses and independent verification.

Ensure differences between arms are attributable to the capability under evaluation, not changes to evidence, knowledge stores, or unrelated implementation behavior.

Do not expose expected outcomes or evaluation labels to the investigating agent.

### Phase B — Validate Environment-Specific Reasoning

Begin with the existing cert-manager affected/unaffected environment pair.

The agent must examine the same upstream change under different environmental configurations and explain why the impact differs.

Then expand to ingress-nginx and kube-prometheus-stack.

Include cases involving:

- Breaking defaults neutralized by explicit configuration.
- Breaking defaults inherited by exposed workloads.
- Render differences that are harmless.
- Behavioral breaking changes not visible in rendered manifests.
- Missing environmental context.
- Incorrect or misleading mitigation hypotheses.
- Contradictory evidence.
- Incomplete dependency or consumer information.

### Phase C — Verify the Knowledge Learning Loop

Demonstrate that an agent can discover a candidate behavior-preservation rule, that the rule can be independently verified, and that the resulting trusted knowledge can be applied to another eligible environment.

Measure the authoritative classification before and after verification.

Also prove that incorrect, incomplete, or inapplicable rules cannot clear an affected change.

### Phase D — Expand Blind Ground Truth

Expand beyond the existing eight evaluation cases.

Establish additional known affected and unaffected upgrade outcomes through independent observations, including isolated runtime testing where practical.

Evaluate on held-out cases that were not used to refine prompts, validators, or rules.

Track false-safe agent assessments separately from authoritative policy false negatives.

No claims about production-level classification reliability should be made from an inadequately sized evaluation set.

---

## Acceptance Criteria

This goal is complete when:

- [ ] Every investigated change can receive a structured, reproducible impact dossier with explicit evidence gaps.
- [ ] The agent produces an independent provisional classification with cited evidence, breaking mechanisms, assumptions, and proposed validation.
- [ ] Agent verdict correctness can be evaluated independently from deterministic production policy.
- [ ] At least one known affected/unaffected environment pair is correctly distinguished and explained by the agent.
- [ ] At least one agent-discovered semantic mitigation hypothesis is independently verified and translated into trusted, reusable knowledge.
- [ ] Verified knowledge demonstrably reaches the relevant validator and changes a classification when applicable.
- [ ] An invalid or inapplicable mitigation is demonstrably rejected without generating an unsafe clearance.
- [ ] Deterministic-only, agent-only assessment, and agent-plus-verification results are compared under controlled conditions.
- [ ] Evaluation results distinguish improved classification accuracy from improved evidence coverage or risk detection.
- [ ] Investigation cost and human review requirements are reported alongside correctness.
- [ ] Existing fail-closed guarantees, evidence provenance, auditability, and reproducibility remain intact.
- [ ] Autonomous approval policy remains unchanged unless separately authorized through existing evaluation gates.

## Implementation Constraints

Build on the existing Phase 4 architecture.

Reuse the evidence bundle, rendering engine, investigation agent, MCP tools, knowledge store, validators, classifier, and Temporal orchestration.

Do not introduce a replacement orchestration framework, a new generalized graph engine, or an unrelated infrastructure platform.

Dependency traversal remains a supporting capability. **Concrete rendered diffs and environment-specific semantic analysis remain central to the investigation.**

Prioritize completing one fully verified vertical slice before broadening product coverage.

Do not weaken current deterministic safety requirements to improve benchmark scores.

Any new data contracts should integrate with existing artifacts and preserve historical run reproducibility.

---

## Definition of Success

Release Intelligence can investigate a proposed dependency upgrade, understand its semantic changes, examine the target environment, analyze concrete rendered differences, identify potential impact mechanisms, and independently reason about whether those mechanisms create a breaking change.

When the agent identifies a plausible mitigation, Release Intelligence can verify the underlying behavioral relationship and determine whether it applies to the target environment.

Verified findings can then contribute to authoritative classification through an auditable, reusable knowledge and validation process.

The system demonstrates measurable improvement over deterministic-only analysis on independent ground-truth cases without introducing unsafe approval behavior.

**The ultimate measure of success is not whether an AI model can produce a convincing explanation. It is whether Release Intelligence can consistently produce correct, environment-specific impact assessments, explain exactly why they are correct, and distinguish what has been proven from what remains uncertain.**
