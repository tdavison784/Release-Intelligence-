# Phase 5 summary — controlled learning-loop experiment (2026-10-17)

Branch `p5-learning-loop` (not pushed). Goal: `MISSION.md`, "Close the agentic evidence-to-decision loop", scoped by
the product owner to one controlled experiment, not a new subsystem. Pre-registration: `PROTOCOL.md` (criteria S1–S6,
run order, and every amendment with the run that motivated it). Numbers: `EVAL-REPORT.md`. Fleet log, merges and
decisions: `COMMANDER.md`. Work queue: `LEDGER.yaml`.

## Outcome in one paragraph

**The loop did not close.** In the experiment as run:
- the deterministic classifier could not establish whether the cert-manager 1.17.2 → 1.18.0 `rotationPolicy` change
  was safe (A0);
- the agent did **not** propose the environment-specific mitigation, under prompt v7 or v8;
- a pre-registered, commander-drafted rule took its place, passed independent verification (confirmed in a
  synthetic kind fixture) and was promoted by the product owner;
- the classifier still did **not** clear the safe environment (C1, S1 fails);
- it did not clear the known-affected environment (S2 holds) or anything on 10 blind held-out cases (S4 holds, but
  trivially, because it cleared nothing at all).

Every safety property held. Nothing was cleared that should not have been: 0 unsafe clearances in any arm. But the
experiment did not produce the positive result the mission asked for. Per the product owner's decision, it is
reported as a negative result rather than amended after seeing C1.

## Stage by stage

| Stage the mission names | Result | Where |
|---|---|---|
| 1. Deterministic classifier cannot establish safety | **Shown.** payments-us `rotationPolicy` item (ci-86548e9a2300) `potentially-affected` under A0: 12 impact paths with no clearing validation, the CRD description change not shown harmless, no pin rule that holds. | EVAL-REPORT §A0 |
| 2. Agent identifies the environment-specific mitigation | **Not shown.** 0 `record_hypothesis` calls on all runs: B0-F2 (prompt v7, 8 cases, 1.06M tokens), B0-v8 (one pre-registered shot on the dev pair, 301k tokens) and held-out B0 (v8, 10 cases, 1.32M tokens). On payments-us the agent saw that the CA pins `Never` and the leaves don't. It did not trace consumers, and it treated "set `Never`" as a remedy, not as a property of the environment. | §B0-F2, §B0-v8, §Held-out |
| 3. Verification establishes the mitigation is valid | **Shown, for a human-authored rule.** Pre-registered fallback `rules/rotationpolicy-ca-scoped.json`: citation check passed, and in the consumer observation (x509 chain check against the pre-re-issuance `ca.crt`) the shielded arm verifies at FROM and TO while the exposed arm breaks at TO → proof `pf-23e47aa9c6a9` CONFIRMED. A deliberately wrong rule was refuted in the smoke test. | runs/w0013-verify, runs/verify2-smoke.md |
| — Human promotion | **Done.** Product owner accepted the whole fact (rd-e42da65009c3) → `vf-e60a2f78af06`, superseding `vf-9b9cfa5c9f5f`. K1 = `752e030a`. | COMMANDER 752e030a |
| 4. Classifier then safely clears the change | **Not shown (S1 fails).** Two independent blockers, below. | §C1 + commander addendum |
| 5. Known-affected environment does not clear | **Shown (S2).** payments-eu stays `potentially-affected`, and the record names the unpinned in-scope CA `mesh-root-ca`. | §C1 S2 |
| Negative controls (S3) | Hold, but **do not discriminate**: C1 clears nothing new, so no mutant had a clearance to be refused. Each mutant is refused for its own reason. | §C1 S3 |
| Held-out (S4) | 0 unsafe clearances, 0 clearances gained. The promoted fact fired on no held-out case. | §Held-out |
| No regression (S5) | Holds: A0/B0/C1 safety rows identical to the Phase 4 baseline on the 8 cases. | §C1 |
| Agent direction on the dev pair (S6) | Holds at verdict level under v8: payments-eu `affected` (correct), payments-us `potentially-affected` (hedged), mechanism and citations resolve. | §B0-v8 |

## Why S1 failed

1. **The rule encoding (the commander's error).** The exposure says "`rotationPolicy` unset **and** `spec.isCA equals
   true`".
   - The payments-us leaf Certificates leave `spec.isCA` unset, and no CRD declares a default for it.
   - The pin validator therefore cannot decide whether the leaves are in scope, and it fails closed: an undecidable
     object is never excluded. This is documented behaviour; lane `pins` recorded that value predicates on unset fields
     need an explicit `unset`/`set` leaf.
   - The rule's `overlap` lists `isCA unset` as shielded, but the pin validator reads only the exposure.
   - Verification could not catch this, because the synthetic fixture sets `isCA` explicitly on every arm. Confirmed in
     a fixture is not the same as decidable in a real environment.
2. **R2.2 coverage (a policy gap).** cert-manager 1.18 also rewrote the CRD description of `rotationPolicy` itself
   (`rc-e29bdc87967c`), and no validation counts that change as harmless:
   - an override-inspection pass covers subjects and paths, never rendered changes;
   - the documentation-only pass refuses, because the same CRD render also adds `spec.signatureAlgorithm`.

   This would have blocked clearance even with a perfectly encoded rule.

The product owner was offered three options: one disclosed post-hoc amendment (re-encode the rule, plus a narrow R2.2
rule letting a pin cover its own field's description change), a rule-only fix, or reporting. They chose to report.

## What the held-out set says about the rule (analysis, not a scored result)

The fact never fired on a held-out case, so S4 tested nothing about it. Reading the C1 pin records of the 5 blind
cert-manager cases statically, here is what a decidable version of the same rule would have done:

| Case | Expected | What the pin record shows | Corrected rule would… |
|---|---|---|---|
| telemetry-east | affected | root CA leaves `rotationPolicy` unset (in scope, not pinned) | not clear — correct |
| search-latam | affected | root CA leaves it unset (the intended patch never reaches it) | not clear — correct |
| telemetry-west | not-affected | CA pinned `Never`; the only miss is a leaf undecidable on `isCA` | pin holds → candidate clearance (R2.2 permitting) — correct |
| registry-apac | not-affected | CA leaves it unset in every render (a Kyverno admission mutation sets `Never`, invisible to the snapshot) | not clear — safe miss |
| billing-nordics | not-affected | environment render incomplete (templated helmfile values); no CA Certificate | not clear — safe miss |

So the rule's logic did not over-generalize on the blind cases. Both affected cases would stay exposed, and the one
case shaped like payments-us would be the only candidate clearance. This comes from reading stored records after
unsealing, by a commander who knows the labels. It is not a run and makes no claim.

## Agent verdicts (scored separately, never a policy input)

Held-out B0: 11 correct, 11 hedged, **1 false-safe** (ingress-nginx docs-eu B3: agent `not-affected`, expected
`affected`), 0 false-alarm, 3 no-verdict. The false-safe never reached policy: B0's classes are identical to A0's on all
26 items. That is the separation working, but it is the main risk of letting agent verdicts drive decisions.
The trace (§Held-out) classifies it as a **reasoning** error, not a retrieval one. The session (6 tool calls) had the
Ingress paths in hand, but never established what 1.12's new strict path validation accepts, so "valid characters" was
its own unverified judgement. It also concluded not-affected even though the Application's values files are missing from
the bundle.

## Disclosures

- **Label knowledge.** The fallback rule was drafted by the commander (Claude Opus 5.5), who had seen the dev-pair
  labels. The review UI marks it `human-authored`. Two generic prompt-v8 instructions (change vs consequence, consumer
  tracing) were shaped by the same knowledge (PROTOCOL amendment 10:55).
- **Post-hoc iterations, each pre-registered before its run:**
  - F2: redaction fix, exposed by the B0-F1 run;
  - F3: prompt v8, after B0-F2 proposed nothing;
  - F4: the verifier's consumer observation, needed because the original `tls.key` observation would refute any
    consequence-scoped rule.
- **No change after C1.** C1, Kbad and held-out ran on F4 with K1 unchanged.
- **Verifier limits:**
  - the overlap alternatives `isCA unset` and `isCA false` are not separate arms;
  - controller-flag and other-object rules are not synthesised;
  - smokes of the verify lanes exercised hand-authored rules on the experiment's subject before the scored run.
- **Data and state:**
  - Agent read path: `search_manifest` shows a redaction marker instead of the CRD enum read-back for `rotationPolicy`
    (W-0033/W-0034). Display only; the validator is unaffected.
  - Prompt8's smoke ingestion rewrote 3 ingress-nginx records in the shared `.ri/store`.
  - Kbad mutation d mutated the scope predicate's value (`isCA`), not the pinned `rotationPolicy` value.

## What would close the loop next time (not done here)

1. **Rule shape.** Intake and verification should refuse a rule whose exposure uses a value predicate on a field that
   has no CRD default and no explicit `unset`/`set` leaf. This would have caught the error before verification. The
   verifier fixture should also include an arm that leaves the role field unset.
2. **R2.2.** A passing pin should cover a correlated `crd-description-changed` on its own pinned field's path, and only
   that, with a mutation check.
3. **Agent proposal.** Three runs of evidence that the agent, as prompted, does not turn "the environment pins X on
   some objects" into a mitigation hypothesis. The next step is a tool-level affordance (for example, the dossier
   listing per-object pin states as candidate conditions), not more prompt text.

## Open items (LEDGER.yaml)

W-0019 (delete `p4ei/eval`, needs `-D`, user), W-0020 (gascity, user), W-0021 (move the :8485 review server and
ri-guard, then remove the p4-integration worktree), W-0022, W-0023, W-0026, W-0034 (search_manifest enum read-back).
