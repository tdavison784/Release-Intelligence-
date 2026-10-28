# Goal: Evidence-Driven Investigation and Upgrade Decision Intelligence

**Release Intelligence — Phase 6**

**Priority:** Core product capability  
**Foundation:** Phase 4 investigation architecture, Phase 5 learning-loop experiment, Phase 6 Phase 0 natural-agent comparison  
**Primary objective:** Improve environment-specific breaking-change reasoning and convert verified findings into actionable upgrade decisions.

---

## 1. Mission

Evolve Release Intelligence into an evidence-driven engineering investigation system capable of assessing proposed dependency upgrades against the actual infrastructure, applications, configurations, and consumers in a target environment.

The system must reproduce the useful investigative behavior demonstrated by natural Claude Code while preserving the deterministic verification, provenance, auditability, and fail-closed policy guarantees established in previous phases.

Release Intelligence must answer three related but distinct questions:

**1. What will this upgrade change?**

Identify relevant upstream behavioral changes, interface changes, configuration differences, compatibility requirements, security advisories, and concrete deployment changes.

**2. What does that mean for this specific environment?**

Determine whether the changed behavior is exercised, which consumers depend on it, whether existing configuration mitigates the change, and what evidence establishes or refutes an actual breaking impact.

**3. What should an engineer do?**

Recommend whether to proceed, modify the target version, preserve existing behavior through explicit configuration, perform additional validation, or defer the upgrade.

Produce a complete, actionable engineering recommendation suitable for a change-management process.

### Primary outcome

**Release Intelligence should produce the same kind of useful technical investigation an experienced engineer expects from Claude Code, while maintaining a strict separation between exploratory AI reasoning and verified, authoritative decisions.**

The system must be able to say not only that a change is potentially breaking, but why it is or is not breaking for a particular environment, which evidence supports that determination, and what must happen next.

---

## 2. Evidence From Previous Phases

This phase must explicitly incorporate the lessons from the previous experiments.

### Phase 4 — Investigation Infrastructure

Phase 4 established much of the technical foundation:

- Immutable evidence bundles.
- Environment-specific Helm/Kustomize rendered diffs.
- Change-to-environment correlation.
- Deterministic validators.
- Investigation agents with evidence-query tools.
- Runtime testing in isolated kind clusters.
- Temporal orchestration and audit history.
- Fail-closed classification and human review.

These capabilities should be reused rather than replaced.

### Phase 5 — Evidence-to-Decision Experiment

Phase 5 demonstrated two important limitations.

First, the investigation agent could identify relevant environmental facts but failed to produce structured mitigation hypotheses.

Second, a separately verified and human-promoted compatibility rule could not clear a known-unaffected environment because of applicability and validation-coverage limitations.

The specific findings included:

- An `isCA` predicate that could not decide whether objects with an unset field were within scope.
- A synthetic verifier that did not exercise the missing-field conditions present in the actual environment.
- A correlated CRD description change that remained uncovered despite a related verified mitigation.
- A false-safe agent verdict caused by unsupported assumptions about ingress-nginx strict path validation.

These are outstanding engineering problems, not reasons to weaken verification policy.

### Phase 6 Phase 0 — Natural Claude Code Comparison

The Phase 0 comparison established a narrow but meaningful improvement from more natural investigation.

The strongest finding was that natural Claude Code inspected relevant downstream consumers that the RI agent had access to but did not examine.

For cert-manager, this included following the relationship:

`Certificate → Secret → mounted ca.crt → application trust behavior`

However, the experiment did **not** establish that whole-upgrade investigation sessions outperform per-item sessions for change-mechanism correctness.

Per-item natural Claude performed as well or better in the tested cases.

Whole-upgrade sessions demonstrated separate value in:

- Identifying unsuitable target versions.
- Recognizing potential security regressions.
- Recommending safer patch releases.
- Planning isolated configuration and version changes.
- Producing an engineer-ready upgrade recommendation.

Both natural and structured agents remained vulnerable to unsupported behavioral assumptions.

### Architectural conclusion

**Retain focused investigation, improve consumer-aware evidence retrieval, separate investigation from formalization, and introduce whole-upgrade decision synthesis.**

Do not assume that increasing model context, changing prompt wording, or replacing per-item sessions will independently improve correctness.

---

# 3. Target Architecture

The system should implement the following logical pipeline:

```text
Proposed Dependency Upgrade
          |
          v
Change and Environment Evidence
  - Release notes / migration guides
  - Version-specific behavioral semantics
  - Security advisories
  - GitOps configuration
  - Helm / Kustomize rendered diffs
  - Existing trusted knowledge
          |
          v
Focused Agentic Investigation
  - Per-change investigation
  - Consumer tracing
  - Effective configuration analysis
  - Natural-language reasoning
  - Evidence requests
          |
          v
Whole-Upgrade Synthesis
  - Correlate findings
  - Evaluate candidate version
  - Identify security regressions
  - Recommend version and sequencing
          |
          v
Evidence Formalization
  - Breaking mechanisms
  - Exposure conditions
  - Mitigation hypotheses
  - Consumer relationships
  - Proof obligations
          |
          v
Independent Verification
  - Semantic checks
  - Configuration validation
  - Render validation
  - Targeted runtime tests
  - Trusted knowledge / human review
          |
          v
Authoritative Classification
  - Affected
  - Not Affected
  - Potentially Affected
  - Insufficient Evidence
          |
          v
Engineering Change Recommendation
  - Proceed / modify / defer
  - Recommended target version
  - Required configuration changes
  - Validation steps
  - Rollback plan
  - Supporting evidence
```

These are logical responsibilities, not a requirement to create separate microservices or Temporal workflows for every stage.

Reuse the existing pipeline and introduce the smallest contracts necessary to maintain clear boundaries.

---

# 4. Workstream A — Improve Evidence Collection and Completeness

## Objective

Ensure that investigations can retrieve the authoritative evidence necessary to understand actual upstream behavior and identify unsuitable upgrade targets.

### A1. Version-specific behavioral evidence

Extend evidence collection beyond release-note summaries when the behavior needed for an impact determination is not established.

Support bounded retrieval of:

- Upstream migration and compatibility documentation.
- Release-specific source changes.
- Relevant implementation code.
- Configuration parsing and defaulting behavior.
- Admission or validation logic.
- CRD schema constraints.
- Controller feature-gate behavior.
- API contract changes.
- Version-specific tests that establish behavior.

The objective is not to ingest entire upstream repositories.

Evidence retrieval should be driven by unresolved technical questions.

**Example:**

An upstream release note says strict path validation becomes enabled by default.

The investigator needs to determine which path values the new validation accepts.

If the release notes do not establish that behavior, the system should retrieve the relevant implementation, validation documentation, or executable test.

The model must not treat remembered behavior as authoritative evidence.

### A2. Security advisory intelligence

Extend the security evidence pipeline to include relevant advisories from authoritative global advisory sources, including GitHub's global advisory database.

Current repository-scoped advisory collection must remain supported, but it is insufficient as the sole source.

The system should:

- Identify advisories relevant to the dependency and affected components.
- Compare the current version, proposed target, and available remediation releases.
- Evaluate affected version ranges.
- Account for prerelease and patched version semantics.
- Relate Helm chart versions to actual controller/application versions.
- Detect security regressions introduced by a proposed target.
- Identify candidate releases that address known vulnerabilities.
- Preserve source provenance, advisory timestamps, and version-range evidence.

A missing repository advisory must not be interpreted as proof that no advisory exists.

**Required regression scenario:**

Investigate the previously evaluated ingress-nginx 1.11.5 → 1.12.0 transition.

The system must identify the relevant security-regression concern and evaluate the documented remediation in 1.12.1 / chart 4.12.1 using authoritative advisory evidence.

It must not rely exclusively on the agent recalling a CVE from training.

### A3. Evidence completeness

Every investigation must distinguish:

- Evidence collected successfully.
- Evidence that is unavailable.
- Evidence that is incomplete.
- Evidence requiring interpretation.
- Evidence that directly contradicts a proposed conclusion.

Unknown behavior must remain an explicit investigation gap.

Do not silently substitute a model-generated explanation for missing upstream evidence.

### Deliverable A

An improved evidence collection and retrieval layer capable of answering specific upstream behavioral and security questions using citable, version-appropriate sources.

---

# 5. Workstream B — Consumer-Aware Agentic Investigation

## Objective

Improve the existing investigation agent's ability to reason about how upstream changes propagate into actual consuming applications and infrastructure.

This is the highest-priority agent improvement from Phase 0.

### B1. Preserve focused investigations

Retain change-scoped investigations as the default mechanism-analysis unit.

Do not replace the architecture with one giant whole-upgrade session.

Focused sessions should remain able to inspect related changes and shared environment context when necessary.

Their scope must not prevent them from following dependencies beyond the immediate resource where a change appears.

### B2. Investigate the entire impact mechanism

For each relevant change, the investigator must attempt to establish:

1. The upstream behavior or interface being changed.
2. The resource or configuration where the change takes effect.
3. Whether the target environment exercises the changed functionality.
4. Which objects, workloads, or applications consume the affected resource or behavior.
5. What aspect of that behavior consumers actually depend on.
6. Whether the existing configuration preserves compatible behavior.
7. Whether the upgrade could alter an observable consumer outcome.
8. What evidence remains necessary to establish the conclusion.

A model's failure to inspect relevant consumers must be recorded as an investigation gap rather than treated as evidence that there are no consumers.

### B3. Consumer tracing

Introduce a focused consumer-tracing capability using existing evidence bundles, environment artifacts, and graph relationships.

The agent should be able to investigate patterns such as:

- Certificate → Secret → mounted trust bundle → application.
- Ingress → Service → workload.
- ConfigMap → environment variable or volume → application.
- CRD → custom resource → controller behavior.
- ServiceMonitor → referenced Secret → Prometheus scraping behavior.
- Configuration or API change → downstream integration.

Relationships should be grounded in repository configuration, rendered resources, verified source behavior, or observations.

The model may propose missing relationships, but inferred relationships must remain marked as unverified.

Do not create a generalized graph engine unless a demonstrated limitation requires it.

### B4. Investigation depth and stopping conditions

Replace superficial investigation completion with evidence-based completion criteria.

The agent should not conclude that a change is supported as Not Affected unless the relevant breaking mechanisms are accounted for through:

- Verified non-exposure.
- Verified mitigation.
- Verified compatibility.
- Or appropriate observed validation.

The agent may still issue a provisional assessment before full verification, but it must explicitly label the supporting evidence and remaining proof obligations.

Do not require the agent to read every file or traverse every unrelated resource.

Investigation should be targeted, bounded, and driven by actual impact mechanisms.

### B5. Natural technical assessments

Let the investigation agent express its conclusions naturally before requiring semantic rule construction.

The per-change output should include:

- What changed.
- Potential breaking mechanism.
- Relevant consumers.
- Effective environment configuration.
- What was verified.
- What is inferred.
- What remains unknown.
- Likely environment impact.
- Recommended additional investigation or testing.

The initial investigation should not require a complete `SemanticAssertion` or `MitigationSpec`.

### B6. Iterative evidence gathering

Allow the agent to request additional context when its investigation encounters a meaningful gap.

Examples:

- A missing values file.
- An unresolved Secret consumer.
- Unknown controller validation behavior.
- An unverified feature-gate default.
- An API contract whose compatibility is unclear.

Where evidence can be retrieved, the investigation should continue against the expanded immutable evidence snapshot.

Where evidence cannot be retrieved, preserve the unresolved question and recommend an actionable next step.

### Deliverable B

A consumer-aware, focused investigation agent that can produce a strong, evidence-backed engineering assessment without simultaneously constructing a formal compatibility rule.

---

# 6. Workstream C — Whole-Upgrade Synthesis and Version Intelligence

## Objective

Introduce an upgrade-level reasoning stage that combines the individual change investigations into one actionable engineering recommendation.

This stage addresses a demonstrated gap in Release Intelligence's existing per-item output.

### C1. Upgrade-level reasoning

The synthesizer must examine:

- All relevant change-level investigations.
- Confirmed breaking impacts.
- Potentially affected consumers.
- Verified unaffected changes.
- Unresolved evidence gaps.
- Compatibility and migration dependencies.
- Security advisories.
- Available candidate versions.
- Existing environmental constraints.

It must evaluate the upgrade as an engineering change, not just a collection of independent classifications.

### C2. Candidate-version evaluation

The system must consider whether the proposed target version is appropriate.

Potential conclusions include:

- Proceed with the proposed target.
- Recommend a later patch within the same release line.
- Select an alternative supported version.
- Defer until a compatibility requirement is satisfied.
- Reject a proposed version that introduces a known security regression.
- Request additional evidence before choosing a version.

A recommendation to use another version must be backed by current, authoritative version and compatibility evidence.

Do not automatically assume the newest release is the safest or most compatible.

When proposing a replacement version, assess the new current-to-target transition rather than assuming an analysis of the original target covers it.

### C3. Change sequencing

Recommend small, independently testable changes where practical.

Prefer preserving established behavior during a version upgrade when doing so reduces unnecessary risk.

For example:

1. Explicitly configure an existing behavior that would otherwise change with the upgrade.
2. Validate that the current environment still behaves as expected.
3. Upgrade the dependency to the selected target version.
4. Validate the upgraded environment.
5. Separately investigate adopting the new upstream behavior.

Do not recommend this sequence mechanically when it introduces greater risk or conflicts with upstream migration requirements.

### C4. Cross-change interactions

Identify relationships between separately investigated changes.

Examples include:

- One version upgrade changing both a configuration default and an API requirement.
- A security fix requiring a behavior change that affects application compatibility.
- A migration whose required configuration conflicts with an existing override.
- A controller feature change affecting multiple workloads.

The synthesizer may request targeted follow-up investigation when the combination introduces a question not answered by the individual findings.

### C5. Engineering recommendation

Produce an explicit recommendation:

- **Proceed:** Available evidence supports the proposed upgrade under stated conditions.
- **Proceed with changes:** Specific configuration or sequencing changes are required.
- **Modify target:** A different version is recommended, subject to its own impact validation.
- **Validation required:** The upgrade cannot yet be cleared because defined evidence or tests are missing.
- **Do not proceed:** A known incompatibility, security regression, or other confirmed blocker makes the proposed action unacceptable under policy.

These recommendations are advisory until authoritative classification and applicable release policies have been evaluated.

### Deliverable C

A whole-upgrade synthesis capability that selects and justifies a target version, identifies cross-change risks, and recommends an actionable upgrade strategy.

---

# 7. Workstream D — Separate Investigation From Evidence Formalization

## Objective

Convert natural engineering investigations into structured, machine-verifiable claims without restricting the investigator's exploratory reasoning.

### D1. Dedicated formalization stage

Once a focused investigation is complete, introduce a separate formalization process.

The formalizer receives the completed assessment and its evidence references.

It must extract:

- Breaking mechanisms.
- Environmental exposure conditions.
- Consumer relationships.
- Applicable configuration settings.
- Potential mitigation conditions.
- Proposed behavior-preservation rules.
- Version applicability boundaries.
- Explicit uncertainties.
- Required validations.

### D2. Reuse existing domain contracts

Prefer the existing `Hypothesis`, `MitigationSpec`, `SemanticAssertion`, `ProofRecord`, and trusted-knowledge infrastructure.

Add new contract fields only where the Phase 5 and Phase 0 results demonstrate a representational gap.

The formalizer should not need to invent a new ontology or duplicate the existing knowledge store.

### D3. Preserve unrepresentable hypotheses

An otherwise useful technical hypothesis must not be discarded merely because it cannot currently be expressed in the semantic grammar.

Such cases must retain:

- The original natural-language claim.
- Supporting citations.
- Intended applicability conditions.
- The specific grammar limitation.
- A proposed verification method.

The system can then request human review or a focused contract enhancement.

### D4. Provenance and non-invention

The formalizer must never:

- Invent evidence absent from the investigation.
- Treat model assumptions as verified facts.
- Broaden applicability beyond what the sources establish.
- Remove unresolved uncertainty to make a rule syntactically valid.
- Promote its own generated rules into trusted knowledge.

Use a validation-and-repair loop for malformed output, with a bounded retry policy.

A syntactically correct hypothesis is not necessarily a technically correct hypothesis.

### Deliverable D

A separate formalization stage capable of converting free-form investigation results into testable, cited hypotheses suitable for independent verification.

---

# 8. Workstream E — Complete Independent Verification

## Objective

Ensure formalized findings can be independently verified and that valid, applicable evidence can actually influence authoritative classifications.

This workstream must resolve the known Phase 5 limitations.

### E1. Rule applicability correctness

Implement robust applicability evaluation for:

- Explicit field values.
- Missing or unset fields.
- Inherited defaults.
- Schema-defined defaults.
- Controller-level defaults.
- Conditional resource predicates.
- Multiple objects with different applicability states.
- Incomplete environments.

Preserve three-valued reasoning:

- In scope.
- Out of scope.
- Undecidable.

Undecidable must not silently become out of scope.

Where the semantic grammar cannot represent a required condition, reject the rule or explicitly mark the limitation before trusted promotion.

### E2. Real-environment applicability checks

Before promoting or applying a behavioral rule, test whether its conditions can be meaningfully evaluated against representative target configurations.

Synthetic verification fixtures must include relevant boundary conditions, including omitted fields.

A rule that works only when every configuration field is explicitly present must not be represented as generally applicable.

### E3. Field-level validation coverage

Address the Phase 5 R2.2 coverage issue.

A passing validation should cover exactly the changes it proves compatible.

For example, a verified behavioral pin may support coverage of a related documentation-only change on the same field when the relevant conditions have been established.

It must not automatically clear unrelated CRD fields, resources, or changes merely because they occur in the same rendered object.

Add mutation tests that demonstrate both the intended coverage and the exclusion of unrelated changes.

### E4. Semantic verification

A verified environmental setting must not automatically be considered a verified mitigation.

The system must establish that:

- The setting has the claimed meaning in the candidate version.
- The setting is actually effective for the relevant objects.
- The relevant consumers are covered.
- The mitigation preserves the behavior needed by those consumers.
- No unaddressed assumption invalidates the conclusion.

Use authoritative upstream evidence, deterministic checks, and targeted runtime exercises as appropriate.

### E5. Targeted runtime validation

Reuse the existing kind-based runtime validation capability where it can establish behavior that static evidence cannot.

Examples include:

- Admission webhook acceptance or rejection.
- Certificate re-issuance and consumer trust behavior.
- API compatibility.
- Controller reconciliation outcomes.
- Resource-reference validity.

Tests must observe meaningful behavior rather than treating successful deployment or absence of errors as proof of compatibility.

Keep runtime validation opt-in and bounded.

Do not expand this phase into a general infrastructure service-validation platform.

### E6. Trusted knowledge

Independently verified reusable rules should flow through the existing human knowledge-review process.

Each trusted rule must retain:

- Upstream provenance.
- Version applicability.
- Required environmental conditions.
- Verified consequences.
- Known limitations.
- Supersession and contradiction relationships.
- Validation records.

A trusted fact should be reusable in another environment only when its applicability conditions are established.

### Deliverable E

An end-to-end verified-knowledge mechanism that can establish legitimate new Not Affected classifications and reject invalid mitigation claims without weakening fail-closed policy.

---

# 9. Workstream F — Authoritative Classification and Change Management

## Objective

Produce authoritative classifications and translate the verified investigation into an engineer-ready change recommendation.

### F1. Preserve classifier authority

Maintain the separation between:

**Agent assessment:** the model's provisional technical conclusion.

**Authoritative classification:** the deterministic policy result based on accepted verification evidence.

Agent confidence, model consensus, and natural-language persuasion must not directly control approval eligibility.

### F2. Evidence-backed classification

The authoritative classifier must distinguish:

- Confirmed breakage.
- Verified non-breaking behavior.
- Plausible impact without sufficient proof.
- Insufficient information to decide.

A Not Affected result requires appropriate proof within a declared scope.

Every classification should retain the exact validating records and assumptions involved.

### F3. Recommendation safety

An upgrade recommendation must not contradict authoritative policy.

For example:

- An unresolved breaking mechanism cannot become an unconditional Proceed recommendation.
- A proposed alternative version cannot be treated as validated merely because the agent recommends it.
- A passing compatibility check for one change cannot clear unrelated changes.
- Missing relevant configurations cannot be presented as proof of non-exposure.

Require explicit preconditions in the headline when the recommendation is provisional.

### F4. Engineer-ready change report

Produce one cohesive change-management artifact.

Required sections:

**Summary and recommendation**
- Dependency and versions.
- Target environment.
- Recommended action.
- Recommended target version.
- Decision status and uncertainty.

**Impact analysis**
- Affected and unaffected behavior.
- Specific impacted resources and consumers.
- Breaking mechanisms.
- Evidence and validation references.
- Outstanding risks.

**Implementation**
- Required configuration changes.
- Suggested change sequencing.
- Dependencies and prerequisites.
- Separation of behavior-preservation changes from adoption of new behavior where appropriate.

**Validation**
- Required tests.
- Expected results.
- Tests already executed.
- Conditions that must be satisfied before proceeding.

**Backout**
- Rollback strategy.
- Configuration restoration.
- Known irreversible migrations or rollback limitations.

**Audit**
- Immutable evidence identifiers.
- Agent assessment.
- Authoritative classifications.
- Human decisions.
- Applicable trusted knowledge.

The report should be suitable for adaptation into a change ticket without requiring an engineer to reconstruct the analysis.

### Deliverable F

A change recommendation that tells an engineer exactly what to change, why, what to validate, and how to back out safely.

---

# 10. Implementation Order

Do not implement every workstream simultaneously.

Use incremental delivery with independent validation gates.

## Milestone 1 — Consumer-aware investigation

**Implement Workstream B using the existing evidence system.**

Begin with the cert-manager affected/unaffected environment pair.

Demonstrate that the RI investigator now:

- Inspects the relevant consuming workloads.
- Identifies the CA trust relationship.
- Distinguishes explicit configuration from inherited defaults.
- Explains why the environments have different impact outcomes.
- Produces a natural technical assessment without structured-rule requirements.

Compare directly with the existing Phase 5 agent transcript.

**Gate:** Consumer-aware investigation reproduces the important technical reasoning observed in Phase 0.

## Milestone 2 — Behavioral and security evidence

**Implement the priority portions of Workstream A.**

Use ingress-nginx to validate:

- Retrieval of exact strict-path validation semantics.
- Reliable uncertainty handling when those semantics are unavailable.
- Global security advisory coverage.
- Identification of vulnerable candidate versions.
- Evidence-supported identification of safer patch targets.

**Gate:** The investigation no longer relies on unverified model memory to establish strict-path compatibility or a target version's security status.

## Milestone 3 — Upgrade-level synthesis

**Implement Workstream C.**

Combine existing per-item investigations into a single upgrade recommendation.

Demonstrate:

- Candidate-version analysis.
- Cross-change evaluation.
- Small, isolated change sequencing.
- Explicit test requirements.
- A coherent upgrade recommendation.

**Gate:** The synthesis produces the useful upgrade-level recommendations observed in Phase 0 without inventing evidence.

## Milestone 4 — Formalization and verification

**Implement Workstreams D and E.**

Start with a single cert-manager mitigation claim.

Demonstrate:

1. The investigator explains the mitigation naturally.
2. Formalization extracts a valid candidate rule.
3. Independent verification tests its semantics and boundary conditions.
4. Human review promotes a confirmed rule.
5. The rule applies correctly to the unaffected environment.
6. The same rule does not clear the affected environment.
7. All unrelated change items retain their own validation requirements.

**Gate:** The complete evidence-to-decision loop produces a genuine new, justified clearance.

## Milestone 5 — Authoritative recommendation integration

**Implement Workstream F.**

Connect verified investigations and upgrade synthesis to the engineering change report.

Keep existing auto-approval restrictions unchanged.

**Gate:** The system produces an auditable, actionable change recommendation consistent with authoritative classification.

## Milestone 6 — Blind comparative evaluation

Freeze the implementation, evidence contracts, prompts, and evaluation criteria.

Run against newly constructed blind cases.

Measure correctness, safe clearances, unsafe conclusions, human review burden, and cost.

**Gate:** Demonstrate meaningful improvement over the existing RI baseline without introducing unsafe approval behavior.

---

# 11. Evaluation Requirements

## Controlled comparison

Compare:

| Arm | Capability |
|---|---|
| A | Existing deterministic RI |
| B | Phase 5 structured investigation agent |
| C | Improved consumer-aware investigation |
| D | Consumer-aware investigation plus upgrade synthesis |
| E | Full investigation, formalization, and verification pipeline |

Do not treat additional agent-generated text or increased evidence retrieval as an accuracy improvement by itself.

## Primary quality metrics

Measure:

- Correct affected classifications.
- Correct unaffected classifications.
- False-safe agent assessments.
- Unsafe authoritative clearances.
- Confirmed breaking mechanisms.
- Relevant consumers correctly identified.
- Correct mitigation hypotheses.
- Verified mitigation hypotheses.
- Successful legitimate safe clearances.
- Unsupported assumptions presented as facts.
- Appropriate uncertainty when evidence is incomplete.
- Correct security-advisory and target-version decisions.

## Operational metrics

Measure:

- Agent tool calls.
- Files and artifacts inspected.
- Tokens and model cost.
- Investigation duration.
- Evidence requests satisfied.
- Human interventions.
- Human review decisions required.
- Reusable knowledge created.
- Change recommendations requiring correction.

## Ground-truth integrity

Previously examined Phase 4, Phase 5, and Phase 0 cases may be used for development and regression testing.

They must not be presented as independent proof of generalization.

Create new blind affected/unaffected pairs with independently observed outcomes.

Ground-truth authors should operate separately from the investigation and formalization implementation lanes.

Do not allow investigators access to expected classifications or solution transcripts.

Do not alter scored expectations to make a new implementation pass.

## Special regression requirements

The evaluation must retain coverage for:

- cert-manager payments-us: known unaffected behavior.
- cert-manager payments-eu: known affected behavior.
- ingress-nginx docs-eu: strict-path false-safe failure.
- ingress-nginx: proposed target introducing a security regression.
- The Phase 5 missing-field applicability problem.
- The Phase 5 CRD field-level validation coverage problem.
- Incomplete configuration and missing consumer information.
- Apparent mitigations that do not actually preserve compatible behavior.

Include negative controls for incorrect, expired, contradictory, and inapplicable trusted knowledge.

---

# 12. Acceptance Criteria

## Investigation

- [ ] Focused RI investigations examine relevant consuming workloads rather than stopping at the changed resource.
- [ ] The cert-manager pair is correctly distinguished through evidence-backed consumer reasoning in a controlled development test.
- [ ] The agent produces a natural technical assessment without requiring a semantic mitigation rule during investigation.
- [ ] Missing consumers, settings, or behavior documentation become explicit evidence gaps.
- [ ] Unsupported safety claims are detected and prevented from becoming authoritative decisions.

## Evidence and version intelligence

- [ ] Exact upstream behavioral semantics can be retrieved or explicitly reported unavailable.
- [ ] Global security advisories contribute to candidate-version evaluation.
- [ ] The ingress-nginx security-regression case produces an evidence-backed target-version recommendation.
- [ ] Alternative versions are analyzed as distinct candidate transitions.
- [ ] No version recommendation relies solely on unverified model knowledge.

## Upgrade synthesis

- [ ] Findings from multiple change items can be combined into one upgrade-level assessment.
- [ ] Cross-change interactions and version-selection concerns can be identified.
- [ ] Implementation sequencing, test requirements, and rollback considerations are included.
- [ ] Recommendations distinguish confirmed decisions from conditional or provisional advice.

## Formalization and verification

- [ ] A natural investigation assessment can be translated into a structured, cited mitigation hypothesis.
- [ ] The formalizer cannot silently invent evidence or eliminate uncertainty.
- [ ] Rule applicability handles explicit, unset, defaulted, and undecidable field states correctly.
- [ ] Verification fixtures cover applicable boundary conditions.
- [ ] A verified compatibility rule can produce a legitimate new Not Affected classification.
- [ ] The same rule does not clear a known affected environment.
- [ ] A verified rule covers only the change subjects and rendered differences its evidence establishes.
- [ ] Invalid or inapplicable rules fail closed.

## Policy and change management

- [ ] Agent verdicts remain independent of authoritative classification.
- [ ] No unverified AI claim changes automatic approval eligibility.
- [ ] The system produces an engineer-ready change recommendation with evidence, implementation steps, validation, and backout guidance.
- [ ] Existing Temporal execution, audit history, and replay guarantees are preserved.
- [ ] Historical benchmark results remain reproducible.

## Evaluation

- [ ] New blind cases demonstrate whether the improvements generalize.
- [ ] Results distinguish investigation quality, classification quality, and recommendation quality.
- [ ] Human review requirements and investigation cost are measured.
- [ ] No unsafe policy regression is introduced.
- [ ] Remaining limitations are explicitly documented rather than hidden by conservative or ambiguous metrics.

---

# 13. Constraints and Guardrails

### Reuse existing infrastructure

Do not introduce new orchestration systems, state stores, agent frameworks, or generalized graph engines without demonstrating a concrete need.

Use the existing Temporal pipeline, evidence bundles, rendering infrastructure, domain contracts, knowledge store, and validators.

### Preserve historical evidence

Existing investigation records, knowledge decisions, prompts, proof records, and benchmark results must remain immutable and reproducible.

Contract changes should be additive where feasible.

### Do not optimize for benchmark appearance

Do not increase safe classifications by relaxing validation requirements.

Do not convert ambiguous or unsupported AI assessments into authoritative Not Affected results.

Do not assume all unknown results represent failures. Measure useful uncertainty separately from actual correctness.

### Do not expand into unrelated platforms

This goal is not a general-purpose autonomous coding system or a replacement for a comprehensive infrastructure-validation service.

Runtime validation is supporting evidence for specific impact hypotheses.

### Control implementation scope

Each milestone must include:

- A defined problem and expected behavior.
- Minimal implementation scope.
- Tests demonstrating the behavior.
- A regression comparison.
- A decision on whether the milestone successfully addressed the identified gap.

Do not start the next major architectural workstream merely because implementation of the previous one is code-complete.

### Keep the human in the loop

Humans retain authority for knowledge promotion, unresolved high-risk changes, and production approval decisions.

Autonomous approval eligibility remains subject to existing evidence and reliability gates.

---

# 14. Definition of Success

Release Intelligence can investigate a proposed dependency upgrade against a specific Kubernetes/GitOps environment and produce a reliable, actionable engineering recommendation.

The investigation understands relevant upstream changes, examines concrete rendered differences, follows the affected resources into their consumers, evaluates existing mitigation settings, and establishes which behavior changes matter to the environment.

When the model discovers a potential mitigation, its natural-language reasoning can be converted into a formal hypothesis, independently verified, and incorporated into reusable trusted knowledge.

Verified evidence can then legitimately improve authoritative classification without allowing unsupported AI reasoning to bypass safety controls.

At the upgrade level, the system considers whether the selected target version is appropriate, detects relevant security regressions, recommends compatible alternatives when supported by evidence, and provides the engineer with an isolated change sequence, validation plan, and backout strategy.

**The central success criterion is measurable improvement in correct, evidence-backed, environment-specific upgrade decisions—not merely more findings, more model output, or more automated activity.**

## Final Product Vision

An engineer should be able to ask Release Intelligence:

**"I want to upgrade this dependency in this environment. What will break, what won't, and what should we do?"**

Release Intelligence should respond with:

- A defensible explanation of actual environmental impact.
- The specific resources and consumers involved.
- Verified mitigations and remaining uncertainties.
- An evidence-backed recommendation for the target version.
- A practical implementation and validation strategy.
- A clear indication of which conclusions are verified and which require human judgment.

The engineer should not have to reconstruct the investigation from changelog summaries, individual findings, and disconnected tool outputs.

**Release Intelligence should do the investigation, prove what it can, clearly identify what it cannot prove, and provide the engineer with a sound basis for the upgrade decision.**
