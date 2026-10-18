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
- **2026-10-17 09:10 — F1 defect fix W-0027 (redaction).** Run `B0-K0-20261017-d262210d` exposed that collection
  redacts the whole subtree of credential-named mapping keys, so `Certificate.spec.privateKey.rotationPolicy` (an
  enum, not a credential) is withheld from the agent and from the pin validator. Fixed by lane `redact` without
  weakening secret redaction; F2 = the fix merge. A0 and B0 are re-run on F2; the F1 runs are kept and reported as
  superseded, with this reason.
- **2026-10-17 09:10 — §4 wording.** "Findings byte-identical to A0" is replaced by: B0's class, rule, basis,
  approval eligibility and decision are identical to A0's for every finding; findings may differ only by the
  removal of the `agent: not run` gap and by attached, AI-labelled assertions. Why: running the agent is itself an
  investigation step (Phase 4 design), so its absence is a recorded gap; the policy-relevant fields are the
  invariant, and B0-K0-20261017-d262210d satisfied them on all 36 findings.
- **2026-10-17 10:03 — F2 = `8286e36f`** (W-0027 merged). Also closes a Phase 4 secret-redaction gap in the render model
  (credential strings under credential-named maps, e.g. `secretAccessKey`, were readable in renders that reach the
  agent). Expected effect on deterministic runs: Secret references in environment files become readable, so more
  graph edges resolve; all bundle/investigation ids change. All arms run on F2.

- **2026-10-17 10:55 — B0 on F2 proposed no mitigation hypothesis (W-0028; product owner decision).** Run
  `B0-K0-20261017-6d2ae5bf` recorded zero hypotheses on all 8 cases, so §6 step 4 has no input. On the dev pair
  the agent stated the environment facts correctly but stayed `potentially-affected` and put the overturning
  conditions only in prose (`wouldOverturn`, `proposedValidations`). Decision, made before any further run:
  1. **Prompt v8 (one revision, no iteration).** Lane `prompt8` writes v8 from the *transcript-observable* failure
     (no case labels in its brief; generic text; synthetic tests). Disclosed: the commander, who knows the dev-pair
     labels, also asked for a generic "objects that change vs objects where a consumer observes the change"
     instruction and for consumer tracing; both are domain-free but were chosen with that knowledge. It is a
     revision made after seeing in-sample v7 results and is reported as such. F3 = its merge; v8 is then frozen.
  2. **One B0-v8 run on the dev pair** (and A0 re-run on the F3 binary). No further prompt change after it,
     whatever it shows.
  3. **Fallback, pre-registered.** If B0-v8 yields no well-formed mitigation hypothesis on the payments-us
     `rotationPolicy` item(s) that §7 verification confirms, the rule is **human-authored** instead (labelled
     `human-authored` in its proposal provenance, in K1's commit and in the report) and goes through the same §7
     verification and §6 step 5 gate. The claim of §1 is then reported as holding for verification → promotion
     → classifier only; agent discovery is reported as failed.
  4. Held-out B0 (§6 step 7) uses v8 either way. S6 is scored on both the v7 and the v8 dev-pair runs.
- **2026-10-17 F3 = `89328b6f`** (W-0029, prompt `investigate-agent/v8` now the default; v1–v7 unchanged). Also
  documents `MitigationSpec.environmentBasis` as the verified evidence of the environment's state with respect to
  the rule's conditions, met or not (still non-empty; read by no validator, verifier or gate). v8 is frozen from
  here. The lane's one smoke run (ingress-nginx shop-fr, 125k tokens) used a pre-final v8 text (basis wording
  only) and made no `record_hypothesis` call, so the structured-mitigation path is covered live only by W-0030.
- **2026-10-17 — B0-v8 outcome; fallback rule pre-registered (W-0031; product owner decision).** Run
  `B0-K0-20261017-edb21ed9-v8` (dev pair, prompt v8): zero `record_hypothesis` calls; payments-eu verdict
  `affected` (correct), payments-us `potentially-affected` (hedged; consumers not traced). Agent discovery has
  failed on v7 and v8; the fallback applies. Finding that shapes it: K0 already holds the trusted fact
  `vf-9b9cfa5c9f5f` (exposure = Certificate with rotationPolicy unset; overlap = set), under which payments-us'
  two leaf Certificates stay exposed, so no rule that the current verifier can confirm (it observes only whether
  `tls.key` changes) can clear payments-us. Decision (product owner): extend §7.1's runtime exercise with a
  **consumer observation** — x509 verification of a leaf issued after a CA re-issuance against the `ca.crt`
  captured before it — on the synthetic fixture (lane `verify2`, F4), and verify the rule pre-registered here:
  `rules/rotationpolicy-ca-scoped.json` (sha256 `659b125555e9696f376598547b41cd4130a14e74a9e0eb8ce16728b5cc506e6f`),
  commander-drafted, label-aware, with its assumption stated (no consumer pins a leaf key). Its confirmation
  criterion is in the file and is fixed now, before the method exists. Its promotion must supersede
  `vf-9b9cfa5c9f5f` (two active facts on the subject ⇒ `pin:contradicting-facts`). If it is not confirmed, it is
  not promoted, C1 = K0 and S1 is reported as failed. The S4 held-out set (5 blind cert-manager cases) is the
  test of whether the rule's scope over-generalizes.
- **2026-10-17 F4 = `48a87f69`** (W-0032, lane verify2): consumer observation in the §7.1 rule exercise
  (`verify.exercise-consumer@v1`, used when a rule names the fixture's role field; every other rule keeps the
  `tls.key` observation) and `ri knowledge intake-rule` (`knowledge.intake@v1`; proposal provenance method stays
  `ai` — the rule was drafted by the commander, an AI — with rule `human-authored:<hyp>` and the author string).
  Outcome logic reviewed against the criterion pre-registered in the rule file. Disclosed limitation: the overlap
  alternatives `isCA unset` / `isCA: false` are not exercised as separate arms; they are covered only by the leaf
  present in every arm. The lane's mechanism smoke (`runs/verify2-smoke.md`) confirmed the rule (pf-57da896bcf53)
  and refuted the wrong rule (pf-97c17ca82902) on a scratch K0; the scored verification is W-0013 at F4.
