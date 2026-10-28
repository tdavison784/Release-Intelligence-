# Milestone 4: formalization and independent verification (Workstreams D + E), plan

Mission §7 D1–D4, §8 E1–E6, §10 M4. Product decisions PD-1 (humans approve; approval never promotes knowledge) and
PD-2 (v13 investigations). Expected outcomes are in `GATE-M4.md` (commander only). Code references are to
`p6-decision-intel`.

## 1. Problem (from a code read)

1. **Nothing formalizes v13 output.** v13 (the default) has feature `mitigation` off, so its rich natural assessments
   (effective configuration, verified, behaviour dependencies) never become a `MitigationSpec` or a
   `SemanticAssertion`. Only the old v8 prompt produced specs, inline, which is what the mission says to stop doing
   (D1).
2. **Two applicability evaluators disagree on unset fields.**
   - The impact join (`impact/condition.go:555–616`) treats an unset field under `equals` as **false** whenever the
     manifests dimension is healthy. It never consults a schema or controller default. So it can treat an object as
     out of scope when it may be in scope: a fail-open risk.
   - The pin scope (`validate/pins.go:623–860`) resolves schema defaults and otherwise returns **unknown**, then
     not-run `applicability-undecidable`. This is the Phase 5 `isCA` problem: a condition on a field that most
     objects leave unset is undecidable, so a correct rule can never clear anything.
   - Neither evaluator can use a **controller-level default** established by upstream evidence. The default is set in
     code, not in the CRD schema.
3. **Coverage is too coarse for behaviour pins.**
   - A pin pass carries the item's subjects and paths but **no rendered changes** (`validate/override.go:149`).
   - The only way to cover a CRD change is the documentation-only check, which rejects the whole CRD when any other
     change sits beside it. This is the Phase 5 R2.2 issue.
4. **Verification checks citations, not meaning.** `verify/citations.go:289` checks that the passage exists, is in
   version bounds and names the subject. The meaning is left to the human (`verify.go:112`). In M2 an agent cited
   the correct upstream text and misread it.
5. **There is no admission-only runtime check.** Runtime exercises exist (`certificate-renewal`, the CA-trust
   consumer) but there is no server-side dry-run to settle "does the admission webhook accept this object?".

## 2. Expected behaviour (generic)

**The loop, mission M4 steps 1–7:**
1. A v13 investigation explains a mitigation naturally.
2. **Formalization** (a separate stage) extracts a cited candidate rule from it, or records why it cannot be
   represented.
3. **Independent verification** tests the rule's semantics and its boundary conditions, including omitted fields
   and defaults.
4. **A human** promotes the confirmed rule through the existing knowledge review. That is the product owner, never
   an agent.
5. The rule **applies** to an environment whose configuration satisfies it, producing a legitimate new Not Affected.
6. The rule **does not clear** an environment that does not satisfy it.
7. **Unrelated change items keep their own validation requirements.**

**Fail closed throughout.** These conditions never clear:
- an undecidable condition;
- a default without evidence;
- a rule outside its version bounds;
- a contradicted or superseded rule;
- a coverage claim beyond what the rule proves.

## 3. Lanes and minimal implementation

### Lane `formalize` (D-0021): Workstream D

| # | Change | Where |
|---|---|---|
| 1 | **Stage.** New `internal/formalize`, the `internal/synthesize` pattern: versioned prompt `formalize/v1`, a structured call through `llm.Cache` (exchange or `claude -p`), a deterministic floor, bounded retry, failure recorded. Input: one v13 `AgentAssessment` + item + evidence pool. Output: `[]domain.Hypothesis` with a `MitigationSpec` / `SemanticAssertion`, checked by the existing spec checks (lifted out of `agent/mitigation.go:28` into a pure function) | `internal/formalize/`, `internal/investigate/agent/mitigation.go` |
| 2 | **Non-invention floor:** every citation must already be in the assessment's verified, behaviour or effective-configuration evidence; every unknown, `not-checked` or `unavailable` behaviour stays, as an `OpUndecidable` leaf or an unrepresentable record; applicability is never broader than the cited sources (D4) | `internal/formalize/floor.go` |
| 3 | **Unrepresentable claims** (D3): a `Hypothesis` with `MitigationRejected` = the grammar limitation, the original claim, citations, intended conditions and a `ProposedVerification` (one additive field) | `internal/domain/` |
| 4 | **Entry points:** `ri investigate formalize <inv>` (offline, after investigation) and `-formalize` on eval. Output feeds `ExportHypotheses` unchanged. No Temporal activity in M4 | `cmd/ri/` |

### Lane `applicability` (D-0022): Workstream E1–E3

| # | Change | Where |
|---|---|---|
| 1 | **One three-valued evaluator.** The impact join consults defaults the way pin scope does. Unset + a comparison with no established default ⇒ **unknown** (`UnknownSemanticAmbiguity`), never false. The shared helper is extracted from `pins.go:1028/1095` | `internal/impact/condition.go`, `internal/validate/pins.go`, shared pkg |
| 2 | **Established defaults:** a rule's condition may carry an explicit default for a field (`Condition` default, with a citation: CRD schema `default`, or a controller default from upstream evidence fetched at the tag via M2). An evidenced default makes an unset field decidable; without evidence it stays unknown. Verified like any other aspect | `internal/domain/semantic.go` (additive), evaluators |
| 3 | **Boundary fixtures:** verify fixtures for omitted fields and the shielded alternatives `consumer.go` already lists but never exercises | `internal/investigate/verify/`, `products/exercises/fixtures/` |
| 4 | **Exact coverage:** a passing pin covers the rendered changes whose (object kind, path) equal the pin's field, including a `crd-description-changed` on that same field path (a field-scoped documentation check). Everything else stays a gap. Mutation tests: another field's description change in the same CRD, a schema/default change, and an unrelated resource all stay gaps | `internal/validate/override.go`, `internal/domain/investigation.go`, `internal/classify/policy.go` |

### Lane `semverify` (D-0023): Workstream E4–E5 and the M2 misread finding

| # | Change | Where |
|---|---|---|
| 1 | **New proof method `semantic-check`.** Deterministic where possible: the value tokens a claim asserts (defaults, allowed values, character classes, before/after) must appear, with the same polarity, in the cited passage, or the claim is rejected as unsupported. Otherwise it is routed to a human. Applied to verify proofs and, as a report-only check, to agent behaviour entries sourced `upstream` | `internal/domain/proof.go`, `internal/investigate/verify/` |
| 2 | **Runtime exercise `admission-dry-run`:** server-side dry-run of an environment object against the target version's admission webhook in kind, recorded as `observed` (accepted or rejected, with the message). Opt-in and bounded like the existing runtime exercises | `internal/investigate/runtime/`, `products/exercises/` |
| 3 | **Tests:** a claim that adds a token the citation lacks is rejected; a polarity flip is rejected; a consistent claim passes; the dry-run is accepted and rejected on kind fixtures (skip when kind is absent) | — |

**Ownership boundaries:**
- **`formalize`:** `internal/formalize/`, the lifted spec checks.
- **`applicability`:** the evaluators and coverage.
- **`semverify`:** proof methods, runtime exercises.
- **Shared `domain` fields:** additive, each lane listing its own in its status file. Lanes merge in order
  `applicability`, then `semverify`, then `formalize`; later lanes `git merge p6-decision-intel` before finishing.

## 4. The demo and the gate (commander, after the merges)

1. **K0 knowledge snapshot.** Use the Phase 5 K0, which is before the Phase 5 human-authored rule. A legitimate *new*
   clearance has to come from this loop.
2. **Investigate.** Run a v13 investigation on the cert-manager pair.
3. **Formalize, then verify.** Run `ri verify hypotheses -runtime` (citations, semantic-check, boundary fixtures,
   renewal and CA-trust exercise).
4. **Promote.** The **product owner** promotes the confirmed rule in the review UI. That makes K1.
5. **Evaluate.** Run eval with K1 on the pair, and compare with K0.
6. **Negative controls** (mission §11): an inapplicable rule, an expired-bounds rule, a contradicted rule, and a rule
   with an unevidenced default. All fail closed.
7. **The M2 finding.** Re-check the stored M2 misread run with `semantic-check` (it must be flagged), and run
   `admission-dry-run` on the D-EU Ingress against a 1.12.x webhook (it must be observed rejected or accepted, ending
   the question).

## 5. Guardrails

- **Blinding** as before (FLEET.md). Prompts and rules stay generic; product facts live in the catalog, exercises and
  knowledge, never in Go logic.
- **The policy floor never weakens:**
  - no fail-open on unset fields;
  - `falseNegativeRate` stays 0 and `unsafeEligibilityRate` stays 0 (frozen gates);
  - knowledge is promoted by a human only (PD-1, E6).
- **Historical runs** (B0, C1, D1) must still re-score and replay.
- **Runtime checks** use kind clusters named `ri-<lane>-*`, are deleted after use, and are opt-in.
