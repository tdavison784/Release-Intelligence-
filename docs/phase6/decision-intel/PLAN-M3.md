# Milestone 3: upgrade-level synthesis (Workstream C), plan

Mission §10 M3 and §6 C1–C5. Product decisions PD-1 (engineer approval of the agent's judgement), PD-0a (concrete
safer target), PD-0b (latest patch; small isolated changes), PD-0d (the scoped answer lives here) and PD-2 (v13
investigations). Development cases are scored by the commander; expected outcomes are in `GATE-M3.md`, which lanes
never read. Code references are to `p6-decision-intel`.

## 1. Problem

RI produces per-item verdicts, assessments, findings, a policy decision and, since M2, an edge security section with
candidate versions. Nothing combines them into the one thing an engineer acts on: **what should we do with this
upgrade, in this environment?** In particular:
- **No upgrade-level judgement.** No recommendation (Proceed … Do not proceed), no headline, no target choice.
- **No sequencing.** Nothing names the changes, in order, as isolated steps.
- **Disagreements and hard findings are scattered across records.** Hard findings include regressions, untraced
  consumers, unavailable behaviour, and verdicts the classifier does not share. An engineer cannot see them in one
  place, and PD-1's informed approval needs exactly that.
- **No renderer shows the assessments** (code map: `AgentAssessments` are never rendered).

## 2. Expected behaviour (generic)

For each upgrade × environment, after classification, RI produces an **UpgradeJudgement**:

1. **Recommendation**, one of: `proceed`, `proceed-with-changes`, `modify-target`, `validation-required`,
   `do-not-proceed` (mission C5).
2. **Headline.** One sentence: the recommendation, the target, the scope and the preconditions. It commits
   (PD-0d), for example: "Modify target to X (chart Y); proceed with changes A then B; provided C; validation D
   required before production."
3. **Target version:**
   - the minimum safe release, from the edge's candidates (cited);
   - the recommended release. Proposed rule (product owner to confirm, see §6): the **latest patch on the chosen line**
     when it removes every regression and adds no assessed advisory (PD-0b), otherwise the minimum.
   - When the target is changed, the judgement **says the new current → target transition has not been investigated**
     and requires that investigation (mission C2), unless it was.
4. **Changes**, ordered and isolated (PD-0b): `pin-behaviour`, `version-bump`, `validate`, `adopt-new-default`,
   `other`. Each gives the file and key where known from deterministic facts (`PinnedSetting`, Argo
   `targetRevision` with file and line, values set and unset). Otherwise it is marked `proposed`, without a location.
5. **Validation:** the tests required before proceeding, each with its expected result, taken from the assessments'
   recommended tests and the gaps.
6. **Residual risks** and **disagreements and hard findings**, each with a **stable id** (PD-1). **Deterministically
   precomputed and copied verbatim into the judgement:**
   - security regressions;
   - an affected or potentially-affected classification versus an agent `not-affected`;
   - untraced consumers;
   - unavailable or not-checked behaviour;
   - a candidate still affected;
   - advisory inconsistencies.

   The model may comment on them but can never drop one.
7. **Cross-change interactions:** items that touch the same objects or settings, flagged.
8. **Verified / inferred / unknown summary**, plus a **backout** line (the version pin to restore).
9. **Provenance:** producer `synthesize@v1`, model, prompt version and digest, input evidence, call id;
   `statementKind: unverified-ai-prose`.

**Deterministic floor, enforced after the model answers:**
- `proceed` is impossible while any hard finding exists.
- A target with a regression cannot be recommended.
- Every evidence reference must resolve.
- A recommended version must be the proposed target or one of the computed candidates.
- A change location must come from a deterministic fact, or be marked `proposed`.

A violating answer is rejected and retried within bounds, then recorded as a synthesis failure. It is never silently
repaired. **The judgement never alters findings, the decision, the review or knowledge.**

## 3. Minimal implementation (one lane, `synthesis`, D-0017)

| # | Change | Where |
|---|---|---|
| 1 | `domain.UpgradeJudgement` + `Investigation.Judgement` (additive, schema regenerated) | `internal/domain/` |
| 2 | **Pre-computation**, a pure function of investigation + impact report + edge: hard findings and disagreements with stable ids, candidates plus latest-patch facts, pins, `targetRevision` location, per-item assessment digest (headline, effective config, preconditions, tests), cross-change overlaps | new `internal/synthesize/` (plain Go) |
| 3 | **One structured LLM call:** `llm.Cache{Inner: Exchange \| claude -p}` with a JSON schema; model configurable, default `claude-opus-5-5`; replay is a cache hit | `internal/synthesize/`, wiring in `cmd/ri` |
| 4 | **Deterministic floor and validation** (§2), bounded retry, failure recorded | `internal/synthesize/` |
| 5 | **Stage:** `Pipeline.Synthesize` after classify, invoked by `ri investigate --synthesize` and `ri eval -investigations -synthesize`. A Temporal activity only behind `workflow.GetVersion`, with the recorded history re-validated (`orchestrate/replay_test.go` must still pass) | `internal/investigate/run/`, `internal/orchestrate/`, `cmd/ri/` |
| 6 | **Rendering:** `RenderJudgement` as a change-ticket section (summary and recommendation, impact incl. assessments, implementation, validation, backout, disagreements), used by `ri investigate` and the reportui view/export | `internal/synthesize/render.go`, `cmd/ri/investigate.go`, reportui |
| 7 | **Scoring hook:** an ungated `ScoreJudgements` side table in eval output (recommendation, target, presence of required parts); gates unchanged | `internal/eval/` |

**Out of scope:**
- formalization and verification (M4);
- approve, deny and override recording and UI (M5). M3 only shapes the judgement for them, through the stable ids.

## 4. Tests

- **Pre-computation on fixtures:**
  - each hard-finding kind produces a stable id;
  - an agent-versus-classifier disagreement;
  - the latest patch versus the minimum;
  - pin and `targetRevision` locations with evidence.
- **Floor:**
  - `proceed` with a hard finding is rejected;
  - a target with a regression is rejected;
  - an unresolvable reference is rejected;
  - a version outside the candidates is rejected;
  - a dropped disagreement is restored verbatim, or rejected;
  - an invented location is rejected.
- **The LLM call** through a scripted exchange: replay is identical and the provenance is complete.
- **Policy isolation:** findings, decision and review are byte-identical with and without synthesis.
- **Temporal:** the replay test passes.
- **Rendering:** golden text output.

## 5. Guardrails

- **Blinding:** lanes never read `GATE-M*.md`, `M*-RESULT.md`, labels, or run reports and scores (FLEET.md).
  Synthesis prompts are generic: no product, case, file or field names.
- **No new framework.** One cached structured call. An agent session for synthesis only if M3's gate shows the
  pre-computed input is insufficient.
- **No policy change:** the judgement is advisory until M5's human decision (PD-1, mission F1).

## 6. Decision for the product owner before the lane starts

**The target rule.** M2 computes the *lowest* safe release (for example 1.12.1). PD-0b asked for the *latest patch*.
The proposal:
- **recommend the latest patch on the chosen line** when it removes every regression and adds no assessed
  advisory;
- **always show the lowest safe release as the minimum.**

The alternative is to recommend the lowest safe release.
