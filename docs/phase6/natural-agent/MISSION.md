# Phase 6 mission — natural Claude Code investigation benchmark

Verbatim from the product owner (2026-10-18). Below it, the product owner's decisions on the commander's
questions. The protocol for Phase 0, the mandatory first step, is `PROTOCOL.md`.

**Product owner's decisions (2026-10-18):**
- Phase 0 includes a **single-item variant** of the natural Claude Code arm, which separates session scope
  (per-item vs whole-upgrade) from tools and output format.
- The investigator model is **Claude Sonnet 5.5** (`claude-sonnet-5-5`), the same model as RI's structured
  agent arm, so the model is not confounded.
- **Arm D (human-guided) is played by the product owner.** The commander prepares sessions and logs every
  intervention by type; the commander never writes the follow-ups, because it knows the labels.

---

I think we are going to pursue this route first:



# Plan: Mock Real-World Claude Code Breaking Change Investigations

**Release Intelligence — Agent Investigation Benchmark**

## 1. Objective

Recreate the real-world engineering workflow in which an engineer provides Claude Code with a target environment, relevant configuration files, and a proposed dependency upgrade, then asks it to investigate whether the upgrade introduces breaking changes.

Compare this natural, filesystem-driven investigation approach against Release Intelligence's existing deterministic and structured-agent implementations.

**Primary hypothesis:**

An LLM agent with appropriate environment context, direct access to relevant files, concrete rendered differences, upstream change evidence, and the freedom to investigate iteratively can produce more accurate environment-specific breaking-change assessments than the existing constrained, per-change-item investigation agent.

The experiment must determine whether the current limitations are primarily caused by:

- Incomplete or poorly presented context.
- Restricted investigation tools.
- Per-item agent sessions that lose cross-change relationships.
- Requiring formal semantic hypotheses too early.
- Inadequate environmental evidence.
- Limitations in the model's own reasoning.

**Do not assume the hypothesis is correct. Measure it.**

## 2. Non-Goals

This experiment must not:

- Replace the existing RI investigation architecture.
- Change production classification or approval policies.
- Introduce another orchestration framework.
- Require a new dependency graph engine.
- Develop a generalized autonomous agent framework.
- Modify historical Phase 4 or Phase 5 evaluation results.
- Use proprietary work repositories, configurations, credentials, or infrastructure.
- Automatically approve dependency upgrades.

The experiment is local, synthetic, reproducible, and isolated from production.

## 3. Construct Realistic Synthetic Environments

Create disposable GitOps repositories that resemble real Kubernetes environments.

Each environment should contain sufficient complexity to require meaningful investigation rather than a simple string comparison.

### Example repository structure

```text
mock-environments/
  environment-a/
    apps/
      payments/
        base/
        overlays/prod/
      checkout/
        base/
        overlays/prod/
    platform/
      cert-manager/
        values.yaml
      ingress-nginx/
        values.yaml
    clusters/
      prod/
        applications/
    upstream/
      release-notes.md
      upgrade-guide.md
      relevant-source-changes.diff
    rendered/
      from.yaml
      to.yaml
      manifest.diff
    environment.md

  environment-b/
    ...

benchmark-private/
  cases/
    case-a/
      expected.yaml
      observations/
      validation.sh
```

The benchmark-private directory must not be accessible to the investigating agent.

### Required environmental characteristics

Fixtures should include:

- Multiple namespaces.
- Helm charts with explicit and inherited defaults.
- Kustomize bases and overlays.
- Argo CD or Flux deployment definitions.
- Multiple applications consuming shared infrastructure.
- Cross-resource relationships.
- Relevant Kubernetes resources and CRDs.
- Realistic configuration overrides.
- Some intentionally irrelevant configuration.
- Dependencies whose behavior cannot be established from rendered manifests alone.

Provide the agent with a short environment orientation file, equivalent to what an engineer would explain before asking Claude Code to investigate.

Do not provide hidden expected classifications or suggested answers.

## 4. Create Breaking-Change Scenarios

Use a mix of known upgrade behaviors and synthetic environment configurations.

### Scenario A — Breaking default, affected environment

A dependency changes a default behavior.

The environment inherits the new default, and a downstream consumer relies on the previous behavior.

Expected outcome: Affected.

### Scenario B — Breaking default, unaffected environment

The same dependency change occurs.

The environment explicitly preserves the previous behavior or otherwise demonstrably avoids the breaking mechanism.

Expected outcome: Not Affected.

### Scenario C — Rendered configuration changes without breaking behavior

A Helm upgrade changes several deployed resources.

The differences are real, but the affected functionality remains compatible with the environment.

Expected outcome: Not Affected, when supported by sufficient validation.

### Scenario D — Breaking behavior invisible in rendered manifests

The deployment changes little beyond the controller image version.

However, the upgraded controller changes API handling, admission behavior, or runtime defaults.

Expected outcome: Affected or Potentially Affected, depending on what can be established.

### Scenario E — Incomplete environment

The agent has release documentation and a subset of deployment configurations, but decisive values or consumer files are missing.

Expected outcome: Insufficient Evidence, unless other available evidence conclusively establishes impact.

### Scenario F — Misleading apparent mitigation

The environment contains a configuration setting that appears to preserve previous behavior.

However, the setting is ignored, applies to different objects, or no longer has the expected meaning in the candidate release.

Expected outcome: Affected or Potentially Affected; never a verified safe determination based only on the apparent override.

### Initial benchmark selection

Use the existing cert-manager and ingress-nginx cases for development and debugging, including the Phase 5 false-safe scenario.

Create new, independently verified variants for the scored evaluation. The previously opened Phase 5 held-out cases must not be treated as new blind evidence.

Have a separate evaluation lane author the cases and verify outcomes using targeted tests or isolated kind runs.

The investigator must never see ground-truth labels, verification scripts that disclose those labels, or earlier analysis transcripts for the scored cases.

## 5. Provide a Real Claude Code Investigation Workspace

For each test case, create a disposable workspace containing only the allowed investigation inputs.

Claude Code should be able to:

1. Browse the repository structure.
2. Read relevant files.
3. Search across configuration and application manifests.
4. Inspect upstream release notes and migration guides.
5. Compare before-and-after Helm/Kustomize render outputs.
6. Examine relevant source or chart differences.
7. Trace consumers and configuration references.
8. Execute safe, read-only diagnostic commands.
9. Request additional evidence when something material is missing.
10. Produce a technical assessment.

Use an allowlisted tool environment with read-only fixture mounts and a separate writable scratch directory.

Prefer pinned local copies of upstream documentation for the controlled experiment. If network retrieval is tested, run it as a separate condition and record whether sources were actually reachable.

Do not permit access to the hidden evaluation directory, prior answers, unrelated repositories, credentials, or the fleet's solution documentation.

### Important architectural distinction

The agent should investigate the **entire dependency upgrade against the environment**, not an isolated change item.

It should have the freedom to connect multiple upstream changes, environment settings, and downstream consequences during one investigation.

Do not require it to create a semantic knowledge-store rule during initial analysis.

## 6. Use a Natural Engineering Prompt

Start with a prompt representative of what an engineer would actually give Claude Code:

> We are considering upgrading this dependency from the current version to the candidate version.
>
> This repository contains the Kubernetes environment that consumes it.
>
> Review the upstream release notes, migration documentation, relevant configuration files, and before-and-after rendered differences.
>
> Determine whether this upgrade introduces breaking changes for this specific environment.
>
> Investigate relevant consumers and configuration relationships. Do not assume that a documented breaking change necessarily affects this environment, or that an unchanged manifest proves runtime compatibility.
>
> Explain what is affected, what is not affected, why, and what evidence supports your conclusions.
>
> Identify missing information or tests needed before an engineer could confidently proceed.
>
> You may inspect relevant files and use the available read-only tools. Do not modify the environment.

Allow the model to choose its investigation sequence.

Do not initially prescribe a rigid tool-call order or semantic rule grammar.

## 7. Run Controlled Comparison Arms

Use the same evidence snapshot and environment configuration for each arm.

| Arm | Investigator | Purpose |
|---|---|---|
| A — Deterministic | Existing RI validators and classifier | Establish the existing baseline |
| B — Structured agent | Current RI per-item investigation agent | Measure the existing agent architecture |
| C — Natural Claude Code | Whole-upgrade investigation with filesystem tools | Test the primary hypothesis |
| D — Human-guided Claude Code | Same as C with controlled engineer follow-ups | Measure the benefit of human guidance |
| E — Formalized investigation | C's completed assessment translated into hypotheses and independently verified | Test integration potential |

Arm E is a follow-up experiment. It must not be treated as if the agent independently produced verified knowledge.

### Human-guided experiment

For Arm D, allow a human to provide limited clarification or redirection.

Record every interaction, distinguishing:

- Clarification of the task.
- A pointer to existing evidence.
- Newly supplied missing context.
- A hint suggesting a particular conclusion.
- A correction of an agent mistake.

Do not supply expected answers.

Score both the unaided result before intervention and the final result after intervention.

This establishes whether human intervention helps because the agent lacks information, lacks investigation direction, or makes an incorrect inference.

## 8. Capture the Investigation Process

Record the complete execution for each arm.

### Required artifacts

- Initial prompt and task context.
- Exact environment snapshot and file digests.
- Model identity and version.
- Available tools and permissions.
- Tool-call and retrieval history.
- Files inspected.
- Upstream documents consulted.
- Rendered differences examined.
- Agent hypotheses and explanations.
- Missing-evidence requests.
- Human interventions, if any.
- Final technical assessment.
- Tokens, execution time, and tool calls.

### Natural investigation output

Allow Claude Code to write a normal engineering assessment.

Afterward, use a separate extraction step to produce a standardized report for evaluation.

The report should identify:

- Proposed impact classification per change mechanism.
- Affected resources and consumers.
- Breaking mechanism.
- Mitigation or lack of exposure.
- Supporting evidence.
- Assumptions and missing evidence.
- Recommended validation.
- Overall upgrade recommendation.

Preserve the original free-form assessment alongside the structured extraction.

The extraction process must not invent evidence, resolve uncertainty on the model's behalf, or upgrade provisional reasoning into verified findings.

## 9. Evaluate Against Independently Verified Ground Truth

Evaluation must distinguish identifying risk from proving safety.

### Primary metrics

- Correct affected determinations.
- Correct not-affected determinations.
- False-safe determinations.
- False-positive determinations.
- Appropriately uncertain assessments.
- Relevant breaking mechanisms discovered.
- Evidence coverage.
- Unsupported claims presented as facts.
- Relevant consumers identified.
- Correctness of proposed mitigations.

### Investigation effectiveness

Also measure:

- Percentage of cases requiring human guidance.
- Number and type of human interventions.
- Additional evidence requested.
- Ability to recover from initial incorrect hypotheses.
- Token cost per useful determination.
- Time to complete an investigation.
- Ability to distinguish multiple environments using the same dependency upgrade.

### Safety expectations

An agent should never receive credit for saying Not Affected solely because it found no apparent problems.

To count as a supported safe determination, the assessment must establish the applicable breaking mechanisms and explain why they do not reach a relevant consumer, are effectively mitigated, or have been validated as compatible within the defined scope.

A correct guess without adequate supporting evidence is recorded separately from an evidence-backed correct determination.

Every false-safe conclusion requires root-cause analysis.

## 10. Analyze Why Claude Code Performs Differently

After scoring, review the transcripts to identify which capabilities explain any difference in performance.

Pay particular attention to whether the natural agent:

- Reads information the RI agent already had but did not use.
- Discovers missing evidence through repository exploration.
- Investigates downstream consumers more effectively.
- Connects multiple changes within the same upgrade.
- Uses rendered diffs more effectively.
- Formulates mitigation hypotheses in natural language.
- Corrects mistaken assumptions through additional tool calls.
- Requests human guidance appropriately.
- Expresses confidence without adequate proof.

Classify failures by root cause:

**Missing evidence, incorrect retrieval, incorrect semantic interpretation, incomplete consumer analysis, reasoning error, tool restriction, policy restriction, or unsupported conclusion.**

Do not attribute improvements to the model when they are explained by different available evidence.

## 11. Translate Findings Into Release Intelligence

Only after the experiment is complete should the fleet recommend architectural changes.

### If natural Claude Code significantly outperforms RI

Identify the smallest changes needed to reproduce its useful behavior.

Possible changes include:

- Whole-upgrade investigation sessions.
- More natural filesystem or repository exploration tools.
- Better initial evidence and context presentation.
- Explicit consumer-focused investigation.
- Free-form reasoning before semantic rule formalization.
- Iterative evidence gathering.
- Optional human feedback during investigations.

Retain deterministic validation and the existing authoritative policy boundaries.

### If both agents struggle

Investigate the underlying evidence problem before modifying prompts again.

Determine whether additional source evidence, runtime observations, consumer relationships, or compatibility rules are required.

### If human-guided runs significantly outperform autonomous runs

Identify which human interventions can be replaced by deterministic evidence collection, automated follow-up questions, or bounded investigation retries.

Preserve the distinction between autonomous accuracy and human-assisted accuracy.

## 12. Acceptance Criteria

- [ ] At least one complete synthetic GitOps environment is available with multiple connected workloads.
- [ ] Known affected and unaffected upgrade scenarios are independently verified.
- [ ] Claude Code can investigate the full upgrade using ordinary filesystem and read-only exploration tools.
- [ ] Ground-truth labels and earlier solutions are inaccessible to investigating agents.
- [ ] Deterministic, structured-agent, and natural-agent runs use equivalent evidence.
- [ ] The natural agent produces a standalone technical investigation report without requiring the existing semantic rule grammar.
- [ ] Human-assisted investigations are captured with explicit intervention records.
- [ ] Agent conclusions are scored separately from RI's authoritative policy determinations.
- [ ] All false-safe conclusions and unsupported assumptions are documented.
- [ ] A new blind evaluation set is completed without post-hoc prompt or rule changes.
- [ ] The final report identifies which differences arise from context, tools, reasoning structure, or human guidance.
- [ ] The fleet proposes only evidence-supported architectural changes to RI.
- [ ] No existing safety or automated approval policy is weakened.

## 13. Deliverables

The fleet must produce:

**A. Mock investigation environments**

Reproducible synthetic Kubernetes/GitOps environments, with complete allowed evidence snapshots and separate verified ground truth.

**B. Natural-agent investigation runner**

A repeatable way to launch Claude Code against a single upgrade/environment workspace and capture its evidence access, tool activity, assessment, and cost.

**C. Comparative evaluation report**

Side-by-side results for deterministic RI, structured RI agent, natural Claude Code, and human-guided Claude Code.

**D. Investigation failure analysis**

Documented examples showing where natural investigation succeeded or failed, including missed context, incorrect assumptions, and false-safe conclusions.

**E. Architecture recommendation**

A focused proposal identifying which parts of the Claude Code investigation workflow should be incorporated into Release Intelligence.

Do not automatically implement that proposal in this milestone.

---

## Definition of Success

This experiment successfully establishes whether a natural, whole-upgrade Claude Code investigation can produce more accurate, evidence-backed, environment-specific breaking-change assessments than the current Release Intelligence investigation agent.

It identifies the capabilities responsible for any improvement and the limitations that remain.

**The experiment is successful even if the natural agent fails to outperform RI, provided the results identify why.**

The desired long-term outcome is to reproduce the useful investigative behavior engineers already employ with Claude Code, while retaining the reproducibility, validation, learning loop, and safety guarantees that distinguish Release Intelligence from an unstructured LLM conversation.

**The question this experiment must answer: What does Claude Code successfully do during a real engineering investigation that Release Intelligence is not yet enabling its investigation agent to do?**






Phase 0 — Manual Investigation Comparison

This phase is mandatory and must complete before architectural implementation begins.
Purpose

Determine whether Claude Code can correctly investigate environment-specific breaking changes when given the same information currently available to Release Intelligence, without the restrictions of per-item agent sessions and mandatory semantic rule generation.

This experiment must isolate the value of natural investigation from differences in available evidence.
Initial Test Cases

Start with three existing development/regression scenarios:

Case A — cert-manager payments-us

An environment where the known upgrade behavior is not expected to break the relevant consumers.

Determine whether Claude independently identifies the effective configuration and the consumer-specific behavior that explains why the upgrade is safe within the evaluated scope.

Case B — cert-manager payments-eu

The same dependency upgrade against an environment that is affected.

Determine whether Claude identifies the actual breaking mechanism and distinguishes this environment from payments-us.

Case C — ingress-nginx docs-eu

The Phase 5 scenario where the structured agent incorrectly classified a strict-path validation change as non-breaking.

Determine whether natural investigation establishes the actual upstream validation semantics and avoids the previous unsupported safety conclusion.

These are known cases and should be treated as diagnostic experiments, not blind accuracy measurements.
Experiment Setup

For each case:

    Create an isolated, read-only investigation workspace.

    Supply the target GitOps environment and relevant dependency versions.

    Include the same upstream evidence and rendered FROM/TO artifacts available to the corresponding RI investigation.

    Permit ordinary filesystem exploration, searching, and comparison tools.

    Prevent access to expected classifications, ground-truth files, prior agent transcripts, and solution documentation.

    Use a pinned Claude model and record its exact identity.

    Preserve the full tool transcript, files inspected, reasoning summary, and final assessment.

Use the fleet to prepare the workspaces, launch investigations, and score results. The investigator itself should run as an ordinary Claude Code agent, not through RI's existing per-item session and hypothesis-generation framework.
Investigation Instructions

Give Claude Code one upgrade and one environment.

Ask it to investigate the entire upgrade, including relevant upstream behavioral changes, rendered configuration differences, effective settings, and downstream consumers.

Allow it to decide which files to inspect and which hypotheses to pursue.

Do not require structured mitigation rules, semantic assertion grammar, or knowledge-store output.

Require a normal engineering assessment that explains:

    What could break.

    Which resources and consumers are affected.

    Which changes are mitigated or irrelevant.

    Why those conclusions follow from the evidence.

    What remains unknown.

    Which validations would be needed before approving the upgrade.

Comparison

Compare the natural investigation against:

    The existing deterministic RI classification.

    The existing structured RI agent's provisional assessment.

    Independently verified expected behavior.

Score the actual technical reasoning, not just the final label.

Determine whether Claude used evidence already present in the RI bundle, discovered relationships the structured agent missed, or needed additional information.
Human-Guided Follow-Up

After the initial result is recorded, permit a second pass with limited engineer-style clarification or redirection.

Record exactly what the human contributed.

Do not count a corrected, human-guided answer as an autonomous success.
Phase 0 Decision Gate

At the end of the manual evaluation, produce a short report answering:

    Does natural Claude Code correctly distinguish the affected and unaffected environments?

    Does it explain the underlying breaking mechanism and mitigation?

    Does it trace relevant consumers more effectively?

    Does it avoid the Phase 5 false-safe reasoning error?

    What did it do differently from the RI agent?

    Would those differences justify an architectural change?

Do not proceed directly to implementation merely because the natural assessment sounds convincing.

If natural Claude Code demonstrates a meaningful advantage, proceed with the architecture below.

If it performs similarly, investigate missing environmental evidence, incomplete behavioral semantics, and reasoning limitations before redesigning the agent.




There is one thing I really want to emphasize: I suspect the per-item agent sessions may be hurting you more than we've realized. In the cert-manager example, evaluating a changed field is different from understanding how the CA, its issued certificates, and the consuming applications interact.
