# Mission — Environment-Aware Breaking Change Impact Analysis (Phase 4)

> Verbatim goal from the product owner (2026-10-16). Normative for every lane. The fleet's design
> decisions live in DESIGN.md (contract lane); where DESIGN.md narrows scope it says so explicitly.
> Section numbers below (§A1–§A7, P1–P6, AC1–AC8) are added for cross-referencing only.

## Objective

Establish environment-aware, agent-driven breaking change impact analysis as the core intelligence
capability of Release Intelligence.

Rather than simply determining whether a dependency release introduces breaking changes, Release
Intelligence must determine whether those changes actually affect the specific infrastructure,
configurations, applications, and dependency relationships consuming that release.

The system must gather concrete evidence, construct the relevant environmental context, traverse
dependency relationships using depth-first search (DFS), and provide an AI investigation agent with the
resources necessary to produce evidence-backed impact classifications.

**The desired outcome is not simply identifying breaking changes. It is proving, where sufficient
evidence exists, whether a proposed change is breaking or non-breaking for a specific environment.**

## Core Principles

1. **Environment-specific correctness:** A globally breaking change is not necessarily breaking for an individual environment.
2. **Evidence before reasoning:** Agent conclusions must be grounded in actual configuration, dependency, source, and deployment evidence.
3. **Context completeness:** Every investigation must have access to all discoverable, relevant information about the change and its target environment.
4. **Graph-driven investigation:** Dependency and configuration relationships must be explicitly traversable, with impact paths recorded.
5. **Deterministic validation:** Where possible, agent hypotheses must be verified through rendering, comparison, static validation, or executable tests.
6. **Reproducible execution:** Investigations must be traceable to immutable evidence and environmental snapshots.
7. **Fail-closed decision making:** Missing context, incomplete traversal, and unsupported assumptions must prevent an automatic safe determination.

## Architecture

### §A1. Change Evidence Collection

Collect and normalize all relevant information about a proposed dependency upgrade:

- Current and candidate versions
- Changelogs, release notes, and migration guides
- Source changes and commit diffs
- API and schema changes
- Helm chart and Kustomize configuration changes
- Generated before-and-after deployment manifests
- Known compatibility requirements

Produce a structured breaking-change evidence record.

### §A2. Environment Context Construction

Collect relevant context for the target environment:

- Dependency inventory and versions
- GitOps repositories and deployment definitions
- Helm values and Kustomize overlays
- Kubernetes resources and configuration references
- Application-to-service relationships
- Infrastructure dependencies and relevant runtime configuration
- Live environment observations where permitted

Create an environment-specific evidence snapshot, preserving source references and collection status.

### §A3. Dependency Graph and DFS Traversal

Construct or query a graph representing dependency, configuration, and consumption relationships.

The investigation must:

- Identify the components directly exposed to the proposed change.
- Traverse downstream consumers and relevant transitive dependencies.
- Discover potentially affected workloads, configuration, services, and integrations.
- Identify environmental overrides that mitigate or eliminate potential breaking behavior.
- Track missing or unverified graph relationships.
- Record reproducible impact paths with supporting evidence.

Graph traversal should remain deterministic where possible. The AI agent should direct further
investigation when relationships cannot be established from available data.

### §A4. Context-Aware Investigation Agent

Introduce a specialized agent responsible for evaluating a proposed change against its specific environment.

The agent receives:

- Structured breaking-change findings
- Environment-specific context
- Graph traversal results and relevant dependency paths
- Before-and-after configuration artifacts
- Relevant source files and documentation
- Available investigation and validation tools

The agent must be able to request additional evidence, investigate uncertain relationships, and validate hypotheses.

Large evidence collections should be exposed through a searchable, change-scoped artifact manifest rather
than indiscriminately loading all files into the model context.

### §A5. Evidence-Based Validation

Support progressively stronger validation mechanisms:

- Configuration override inspection
- Helm and Kustomize render comparison
- Schema and API compatibility checks
- Static dependency and reference checks
- Isolated deployment or runtime validation where supported

Distinguish verified facts from inferred behavior.

### §A6. Impact Classification and Policy

Produce one of four classifications:

- **Affected:** Evidence demonstrates an incompatible or breaking impact.
- **Not Affected:** Sufficient evidence supports that the change does not affect the target environment within the defined analysis scope.
- **Potentially Affected:** A plausible impact exists but cannot yet be confirmed or dismissed.
- **Insufficient Evidence:** Missing context or validation prevents a defensible conclusion.

Every finding must include:

- Reasoning and supporting evidence
- Impacted environment and components
- Dependency traversal paths
- Validation results
- Uncertainty and evidence gaps
- References to underlying artifacts
- Recommended remediation or next investigation

Automated approvals must be governed by explicit policies and empirical validation, not by LLM confidence
scores or model agreement alone.

### §A7. Temporal Orchestration

Use Temporal to coordinate evidence collection, environment context construction, graph analysis, agent
investigation, validation, and approval.

Design activities and workflows for:

- Durable execution and retries
- Idempotent evidence collection
- Long-running investigations
- Human-in-the-loop review
- Failure recovery
- Auditability and reproducibility
- Operational observability

Temporal orchestration must remain logically separated from the reasoning and validation implementations.

## Implementation Plan

### P1 — Evidence and Context Contracts

Define the structured contracts for change evidence, environment snapshots, graph relationships, and investigation results.

Establish provenance requirements, artifact storage references, and handling of unavailable or incomplete evidence.

**Deliverable:** A reproducible, environment-scoped evidence bundle for a candidate dependency change.

### P2 — Environment Relationship Discovery

Implement the initial graph model and relationship discovery logic.

Support a bounded DFS investigation that traces how a proposed change may propagate through dependent components.

Start with GitOps and rendered configurations, then expand to observed runtime relationships.

**Deliverable:** Evidence-backed impact paths for a targeted dependency change.

### P3 — Agentic Investigation

Implement a change-scoped investigation agent with controlled access to the evidence bundle and graph traversal tools.

The agent should formulate hypotheses, retrieve relevant artifacts, inspect environmental relationships, and request validation.

**Deliverable:** Structured findings explaining why an environment is affected, unaffected, or requires further investigation.

### P4 — Deterministic Verification

Integrate Helm/Kustomize rendering and other available validators into the investigation workflow.

Separate agent assertions from confirmed validation results.

**Deliverable:** Findings incorporating concrete configuration comparisons and validation evidence.

### P5 — Temporal Production Integration

Orchestrate the complete investigation using Temporal workflows and activities.

Add retries, durable state, monitoring, cancellation, bounded execution, and human approval.

**Deliverable:** An operational, auditable breaking-change investigation pipeline.

### P6 — Reliability and Confidence Evaluation

Evaluate findings against known breaking and non-breaking upgrades.

Measure:

- False-negative rate for breaking changes
- False-positive rate
- Evidence and dependency coverage
- Unresolved or incomplete investigations
- Agreement between classifications and observed outcomes
- Human review and automated approval eligibility

**Deliverable:** Measured evidence that the system can safely support increasingly autonomous decisions.

## Initial Acceptance Criteria

The initial implementation should demonstrate:

- AC1 A dependency upgrade analyzed against a specific target environment.
- AC2 Complete traceability to collected change and environment evidence.
- AC3 DFS traversal of relevant dependency and configuration relationships.
- AC4 An AI investigation that can inspect and request additional artifacts.
- AC5 At least one reproducible affected and one unaffected test case, verified against known outcomes.
- AC6 Findings that clearly distinguish validated conclusions from unresolved assumptions.
- AC7 Temporal orchestration with basic retry, failure handling, and audit capabilities.
- AC8 Human review when sufficient evidence cannot be established.

## Scope and Delivery

The first production milestone should prioritize a narrowly defined set of dependency types and supported environments.

Broad ecosystem coverage, unrestricted autonomous approvals, and comprehensive runtime compatibility
guarantees are not requirements of the initial release.

Expansion should follow measured classification accuracy and operational reliability.

## Definition of Success

Release Intelligence can take a proposed dependency upgrade, understand what changed, investigate the
actual environment consuming it, trace relevant dependency relationships, and produce a reproducible,
evidence-backed determination of impact.

**Success is measured by the correctness, completeness, and verifiability of the impact assessment—not
simply by whether an LLM generated a classification.**
