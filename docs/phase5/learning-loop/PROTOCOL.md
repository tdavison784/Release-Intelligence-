# Phase 5 experiment protocol (pre-registered 2026-10-17, commander-owned)

This file is written **before** any Phase 5 scored run and is never edited afterwards; amendments are appended
at the end, dated, with the reason. Lanes build only what this protocol needs (MISSION scope note).

## 1. The claim under test

> Under K0 the deterministic classifier cannot clear the cert-manager `rotationPolicy` change for an environment
> that is in fact unaffected. The investigation agent proposes a structured mitigation hypothesis; independent
> verification confirms the underlying rule; a human promotes it (never the agent); under K1 the classifier
> clears that change **for the unaffected environment only**. For the known-affected environment the same
> K1 does not clear it, and invalid / inapplicable / unverified rules never clear anything.

## 2. Cases, and what the fleet already knows

| Set | Cases | Status |
|---|---|---|
| **Development pair** | `cert-manager-1.17-1.18--payments-us` (not affected) / `--payments-eu` (affected) | **In-sample.** The commander, the planning analysis and EVAL-REPORT have seen their labels and their environment differences. Results on this pair demonstrate the mechanism; they do not measure accuracy. |
| Phase 4 regression set | the other 6 Phase 4 cases | Known labels; used for "no regression" only. |
| **Held-out set** | new cases authored blind by lane `groundtruth5` on its own branch `p5ll/groundtruth5` | **Sealed.** Not merged, not read by any pipeline lane or the commander until the code freeze in §6 step 7. This is the measurement. |

Disclosed confound: the shape of the payments distinction (which Certificate carries the explicit setting, what
consumes it) is known to the fleet. Therefore pipeline code is generic (no product names, no case names, no
case-shaped tests: fixtures are synthetic), the agent prompt is generic, and the claim is only accepted if it
also holds on the held-out set (§5 S4).

## 3. Knowledge snapshots (never the live `knowledge/` tree)

- **K0** = `git archive 03b6ff43 knowledge` — Phase 4 knowledge including the product owner's whole-fact
  verifications. Extracted to a scratch dir; its digest is recorded in every run (`knowledgeDigest`).
- **K1** = K0 + the facts promoted in §6 step 5, and nothing else, committed as one commit whose SHA is recorded.
- **Kbad-\*** = K1 with one negative-control mutation each (§5 S3), built by script from K1, never committed to
  `knowledge/`.

## 4. Arms (same bundles, same code commit, only the named capability differs)

| Arm | Pipeline | Knowledge | Agent output used by policy? |
|---|---|---|---|
| A0 | deterministic | K0 | — |
| B0 | A0 + agent provisional verdicts + mitigation hypotheses | K0 | **no**: findings must be byte-identical to A0 (tested) |
| C1 | deterministic | K1 (promoted from B0's hypotheses after verification) | only through trusted knowledge |
| A1 (ablation) | deterministic | K1 built from an *unverified* promotion attempt | must equal A0 (the promotion gate refuses) |

The agent never sees `expected.yaml`, NOTES.md, `observed/`, `docs/`, EVAL-REPORT or this file: its tools read
the evidence bundle only (existing Phase 4 guarantee, re-tested).

## 5. Pre-registered outcomes and metrics

Success criteria (all required):
- **S1** dev pair, payments-us: the in-scope `rotationPolicy` item(s) are not `not-affected` under A0, and are
  `not-affected` under C1 with basis = an override-inspection pass whose pin cites the promoted fact, and every
  remaining R2 condition satisfied (no gaps, no unvalidated assumptions, rendered changes covered).
- **S2** dev pair, payments-eu: under C1 the same item(s) are **not** `not-affected`; the record names the
  failed applicability condition(s).
- **S3** negative controls never yield `not-affected` for any item that is not `not-affected` under C1, and never
  for any expected-affected item: (a) promoted at AI/consensus level only, (b) refuted hypothesis (gate refuses
  promotion), (c) version-inapplicable (`release` outside the transition), (d) wrong pinned value,
  (e) superseded and (f) retracted rule, (g) a rule whose applicability condition cannot be decided on the
  environment, (h) a contradicting active fact for the same subject.
- **S4** held-out: C1's knowledge applied without any new human verification. Unsafe clearances (expected
  affected ⇒ `not-affected`) = 0 is required. Clearances gained are reported, whatever they are.
- **S5** no regression of the frozen Phase 4 gates' safety rows (falseNegativeRate, unsafeEligibilityRate) on the
  8 Phase 4 cases.
- **S6** agent: on the dev pair, the provisional verdicts for the `rotationPolicy` item differ between the two
  environments in the right direction, with a mechanism and citations that resolve.

Reported metrics (ungated, per arm; agent verdicts scored **separately** from policy classes):
agent verdict confusion matrix per item (correct / false-safe / false-alarm / hedged), policy class per item
before/after, clearances gained, unsafe clearances, human review items and decisions needed (with K0 vs K1),
tokens / wall time / sessions per investigation, verified claims, hypotheses proposed / confirmed / refuted /
inconclusive, validation requests passed, evidence-retrieval and reasoning failures. Coverage/risk-detection
changes are reported in a separate table from accuracy changes. No reliability claims from n < 4 per class.

## 6. Run order

1. Code freeze F1 on `p5-learning-loop` after wave 1 merges (SHA recorded).
2. A0 on the 8 Phase 4 cases with K0 (`ri eval -investigations -save -knowledge <K0>`).
3. B0 on the same cases with K0 (`-agent`); transcripts kept.
4. Export B0's mitigation hypotheses to proposals + review items in a copy of K0 (never facts). Run independent
   verification on each (§7). Refuted / inconclusive ones stay so and are reported.
5. The product owner reviews the confirmed ones in `ri review serve` (human); promotion is a knowledge decision.
   Commit → K1 SHA.
6. C1 and the Kbad-* controls on the 8 cases.
7. Unseal: merge `p5ll/groundtruth5` (code frozen at F1, no prompt/validator/rule edits after this point).
   A0, B0 and C1 on the held-out cases. Report.

## 7. Verification of a mitigation hypothesis (what "independent" means)

A hypothesis is confirmed only when all of the following hold, each recorded in a proof record:
1. **Upstream rule** — the hypothesis' citations resolve to upstream text at a version inside the transition and
   state the claimed behaviour (deterministic citation check), **and** a targeted runtime exercise on a
   **synthetic** fixture (never an evaluation environment) shows the behaviour preserved with the condition and
   changed without it (both arms observed; a quiet window is never a pass).
2. **Rule shape** — the proposed rule is expressible in the existing fact model (subject, change, applicability,
   consequence) with version bounds; its applicability conditions are decidable predicates.
3. **Human** — a human accepts/corrects the whole fact (all aspects) with the proof record shown. A proxy, the
   agent, or a refuted/inconclusive proof cannot promote.
Applicability to a particular environment is then decided by the deterministic validator at investigation
time (the "verified relationship"), never by the proof record.

## 8. Contract additions (additive; historical runs must still load and keep their digests)

- `Investigation.AgentVerdicts []AgentVerdict` (lane `verdict`): per item `{item, class (four classes),
  confidence, mechanism, environmentReasoning, citations, verifiedFacts (claim/validation ids), assumptions,
  mitigations (hypothesis ids), uncertainty, proposedValidations, wouldOverturn, provenance}`. Not an input to
  `classify`.
- `Hypothesis.Mitigation *MitigationSpec` (lane `verdict`): `{item, proposedRule (a SemanticAssertion-shaped
  fragment: subject, change, applicability incl. conditions, consequence, release bounds), environmentBasis
  (claim / object refs showing this environment meets the conditions), requestedVerification}`.
- `ProofRecord` (lane `verify`), stored with the review item in the knowledge store: `{id, hypothesis,
  proposedRuleDigest, methods[], outcome confirmed|refuted|inconclusive, evidence refs, versionBounds,
  conditions, limitations, level}`.
- `InvRunMeta.KnowledgeDigest` (lane `eval5`).
Field names may be adjusted by the owning lane; the shape may not shrink.

## Amendments

- **2026-10-17 08:50 (before any held-out run).** Held-out scoring gates: at unseal (§6 step 7) the commander
  copies `eval/investigations/gates.yaml` byte-identically to `eval/investigations-heldout/gates.yaml`
  (sha256 prefix `8e7a008efcde4f41`); no other gate file. Why: lane groundtruth5 reported that the held-out dir needs one, and
  gates are pre-registered — reusing the frozen Phase 4 gates unchanged is the only choice made without
  knowledge of the held-out outcomes. The held-out set is 10 cases (5 cert-manager, 3 ingress-nginx, 2
  kube-prometheus-stack; decisive items 5 affected / 5 not affected) on sealed branch `p5ll/groundtruth5` @ 2c092bdf.
- **2026-10-17 08:55 — code freeze F1 = `131d6c2e`** (all wave-1 lanes + fixes5 merged; build/vet/test green). From here
  until the report, code changes are allowed only to fix a defect a run exposes; each is appended here with the
  run that exposed it, and every arm affected is re-run on the new binary. Prompt v7 is the frozen agent prompt;
  the agent model for B0 is `claude-sonnet-5-5` (as Phase 4 arm B, for comparability).
