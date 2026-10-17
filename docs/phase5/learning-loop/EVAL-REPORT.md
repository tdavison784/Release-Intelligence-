# Phase 5 evaluation report

Protocol: `PROTOCOL.md` (pre-registered 2026-10-17). This file is written by lane `eval5`; sections are appended as
arms are run. Numbers come from stored runs under `eval/investigations/results/` (append only) and are reproduced by
`ri eval -investigations -compare <run>,…`. Gates and expectations are untouched.

## §A0 — deterministic baseline, K0 (wave 1)

**Run** `A0-K0-20261017-bfebde39`: arm A0 (deterministic, no agent, no runtime), binary `bfebde39` (clean tree), 8
Phase 4 cases, knowledge = K0 = `git archive 03b6ff43 knowledge`, digest
`sha256:672a7ce08954536b4ab9e05f6093856e2746ebfbfad65674187f2c7a0cd74ff5` (recorded in `run.json` as `knowledgeDigest`;
the Go and `scripts/phase5/snapshot.sh` digests agree). Command:
`ri -state <primary>/.ri -offline eval -investigations -arm A0 -run-id … -knowledge <K0 snapshot> -save`.

**Regression check (S5 baseline).** Every one of the 24 item decisions (class, basis, eligibility) is identical to the
last Phase 4 A run (`A-20261017T020337Z-d1ed12aa8d`), so this commit's pipeline and K0 reproduce Phase 4 exactly.
Gate states are Phase 4's: 5 FAIL, 1 INSUFFICIENT-DATA; the frozen gates were not touched. The one unsafe
eligibility (`unsafeEligibilityRate` 1/8) is `kube-prometheus-stack…obs-north B1`: expected affected, classified
affected and approval-eligible (an affected finding marked eligible), a known Phase 4 result, not new.

Tables below are the A0 column of the comparison report; later arms are added next to it.

#### Runs

| label | run | arm | binary | knowledge digest |
|---|---|---|---|---|
| A0 | A0-K0-20261017-bfebde39 | A0 | bfebde398f | sha256:672a7ce08954536b4ab9e05f6093856e2746ebfbfad65674187f2c7a0cd74ff5 |

#### Policy class per item (expected → system)

| case · item | A0 | expected |
|---|---|---|
| cert-manager-1.17-1.18--payments-eu · B1 | potentially-affected [none] | affected |
| cert-manager-1.17-1.18--payments-eu · B2 | potentially-affected [none] | not-affected |
| cert-manager-1.17-1.18--payments-us · B1 | potentially-affected [none] | not-affected |
| cert-manager-1.17-1.18--payments-us · B2 | potentially-affected [none] | not-affected |
| ingress-nginx-1.11-1.12--shop-de · B1 | potentially-affected [none] | affected |
| ingress-nginx-1.11-1.12--shop-de · B2 | potentially-affected [none] | not-affected |
| ingress-nginx-1.11-1.12--shop-de · B3 | potentially-affected [none] | not-affected |
| ingress-nginx-1.11-1.12--shop-de · B4 | potentially-affected [none] | not-affected |
| ingress-nginx-1.11-1.12--shop-fr · B1 | potentially-affected [none] | not-affected |
| ingress-nginx-1.11-1.12--shop-fr · B2 | potentially-affected [none] | not-affected |
| ingress-nginx-1.11-1.12--shop-fr · B3 | potentially-affected [none] | not-affected |
| ingress-nginx-1.11-1.12--shop-fr · B4 | potentially-affected [none] | not-affected |
| ingress-nginx-1.11-1.12--shop-nl · B1 | affected [deterministic] | not-affected |
| ingress-nginx-1.11-1.12--shop-nl · B2 | potentially-affected [none] | not-affected |
| ingress-nginx-1.11-1.12--shop-pl · B1 | potentially-affected [none] | insufficient-evidence |
| ingress-nginx-1.11-1.12--shop-pl · B2 | potentially-affected [none] | not-affected |
| ingress-nginx-1.11-1.12--shop-pl · B3 | potentially-affected [none] | not-affected |
| ingress-nginx-1.11-1.12--shop-pl · B4 | potentially-affected [none] | not-affected |
| kube-prometheus-stack-89-90--obs-north · B1 | affected [deterministic] | affected |
| kube-prometheus-stack-89-90--obs-north · B2 | potentially-affected [none] | not-affected |
| kube-prometheus-stack-89-90--obs-north · B3 | not-affected [deterministic] | not-affected |
| kube-prometheus-stack-89-90--obs-south · B1 | potentially-affected [none] | not-affected |
| kube-prometheus-stack-89-90--obs-south · B2 | potentially-affected [none] | not-affected |
| kube-prometheus-stack-89-90--obs-south · B3 | not-affected [deterministic] | not-affected |

#### Safety

| row | A0 |
|---|---|
| falseNegativeRate | 0.000 (0/3) |
| unsafeEligibilityRate | 0.125 (1/8) |
| clearances (system not-affected) | 2 |
| …of which expected not-affected | 2 |
| unsafe clearances (not expected not-affected) | 0 |
| approval-eligible decisions | 8 |

#### Accuracy (class equals expected)

| metric | A0 |
|---|---|
| accuracy | 0.125 (3/24) |
| notAffectedConfirmation | 0.100 (2/20) |
| affectedConfirmation | 0.333 (1/3) |
| observedAgreement | 0.048 (1/21) |

#### Coverage and risk detection (reported separately from accuracy)

| metric | A0 |
|---|---|
| affectedDetection | 1.000 (3/3) |
| exposureRecall | 0.857 (6/7) |
| evidenceCoverage | 0.324 (23/71) |
| unresolvedRate | 0.000 (0/23) |
| humanReviewRate | 0.917 (22/24) |
| autoApprovalEligibilityRate | 0.333 (8/24) |

#### Human review and cost

| measure | A0 |
|---|---|
| item decisions needing human review | 22 |
| findings requiring human review (all items) | 118 |
| agent tokens | 0 |
| hypotheses proposed | 0 |
| wall seconds (all cases) | 30 |

### What still blocks the cert-manager `rotationPolicy` items under A0

Items: `B1` of `payments-us` (expected not-affected) and `payments-eu` (expected affected). Each matches two change
items (`ci-86548e9a2300`, the computed/CRD-side item with one correlated rendered change, and `ci-93299c596271`, the
release-note item); both are `controller-behaviour`, both land at `potentially-affected` (`classify:plausible-impact`,
basis none, human review required, not approval-eligible). Both environments produce **the same record shape**, which is
the point of the baseline: A0 does not separate the two.

| blocker (same for `-us` and `-eu` unless noted) | evidence in the run |
|---|---|
| **No pin.** Override inspection finds no rule that holds. The only K0 pin rules (from fact `vf-9b9cfa5c9f5f`: `pin:knowledge-default-changed@v1`, `pin:knowledge-exposure@v1`) require `spec.privateKey.rotationPolicy` to be set explicitly on **every** `cert-manager.io` Certificate. | `checkout-tls`, `ledger-tls` and `cert-manager/mesh-root-ca` leave it unset in both environments (`-eu` additionally lists the `mesh-root-ca-prod.yaml` overlay). A rule scoped to "every Certificate" cannot fire for either payments environment, so K0 holds no pin that could clear either item. |
| **Render silence proves nothing.** `render-diff` is `not-run`: the item manifests as controller behaviour, so a quiet render is not evidence. | `vr-c18196e31a42` / `vr-1bf958d0a0db` |
| **Unresolved relationships.** `static-reference` is `not-run`: Secrets referenced on impact paths are absent from the snapshot, and one `references` edge (Secret produced in namespace `cert-manager`, referrer namespace unknown) is only inferred. Traversal is `complete: false` (`unresolved: 1`) in both. | `vr-d1ed605167f3` / `vr-786895855fc7`; relationship gap `ge-7408a9792f35` (`environment-visibility-gap`) |
| **Every impact path is uncovered by a clearing validation** (`runtime-behavior-gap`): 12 paths + 1 rendered change + the item itself on `ci-86548e9a2300`, 8 paths + the item itself on `ci-93299c596271`. | 14 / 9 validation gaps per finding |
| **Agent not run** (arm A0): an `agent` gap on each finding. | expected; it is the capability B0 adds |

So for `payments-us` to clear under C1 (S1) the knowledge must supply a pin whose scope can hold in that environment
(not "every Certificate"), and the remaining R2 conditions — the relationship gap, the path-coverage gaps and the
rendered change — must be closed by validators, not by the rule itself. For `payments-eu` (S2) the same K1 must leave
the item uncleared with a named failed condition; under A0 it is uncleared for the same blanket reasons, which is
**not** evidence the mechanism discriminates. Closing path coverage and the relationship gap is the `pins` / `coverage`
lanes' work; whether it is enough is a wave-2 finding, not assumed here.

### Other cases (no regression)

The six non-payments cases are unchanged from Phase 4 (see the item table). The two clearances A0 makes
(`obs-north B3`, `obs-south B3`) are expected not-affected, so there are 0 unsafe clearances; FN rate 0/3.

### Agent verdicts

Not applicable to A0. The verdict table is implemented (ungated, separate; outcomes correct / false-safe / false-alarm
/ hedged / overstated / no-verdict) and is empty until lane `verdict` merges and B0 runs.

## §A0-F1 — deterministic baseline on the frozen code (wave 2) — SUPERSEDED, kept for the record

> Superseded (commander, W-0027): collection redacts the whole `spec.privateKey` map, so `rotationPolicy` is withheld from the
> agent and the validators. A0 and B0 are re-run on F2. The F1 runs below stay stored and are not edited.

**Run** `A0-K0-20261017-d262210d`: merge `d262210d` (includes F1 = `131d6c2e`), clean tree, 8 Phase 4 cases, K0 digest
`sha256:672a7ce0…74ff5` (unchanged), fresh `-work`. Compared with `A0-K0-20261017-bfebde39` (wave 1):

- **All 24 item decisions identical** (class, basis, eligibility); all metrics, gate states and clearances identical
  (0 unsafe clearances, FN 0/3, `unsafeEligibilityRate` 1/8 as before).
- Finding-level changes only in gap lists: (1) `validate.render-diff@v1` → `@v2` (every inconclusive render-diff record has a
  new id and the validator version in its text); (2) the validator list named in the "no passing validation… could not
  decide" gap reorders (override-inspection / static-reference / render-diff swap places in the sentence); (3) on the two
  cert-manager cases the `relationship` gap on inferred edge `ge-7408a9792f35` is no longer raised once per path (net −1 gap on 16
  findings; the edge and its inference are the same, so I attribute it to the F1 gap-deduplication, not to new evidence
  — **not verified in code, commander to confirm**). No class, rule or eligibility moved.

## §B0-dev — agent arm on the development pair, K0 (wave 2, first half) — SUPERSEDED, kept for the record

> Superseded for the same reason (the `payments-us` verdict was hedged because `rotationPolicy` was redacted from its evidence).
> PROTOCOL §4 was amended 2026-10-17 09:10: the invariant is class/rule/basis/eligibility/decision identical; this run meets it.

**Run** `B0-K0-20261017-d262210d`: arm B0 (`-agent`, `claude-sonnet-5-5`, prompt `investigate-agent/v7`), same binary as
A0-F1, K0, fresh `-work`, the two payments cases only (the other six are appended later with the same binary). The agent
reads the evidence bundle only (existing guarantee; no ground-truth path is reachable).

| | payments-eu | payments-us |
|---|---|---|
| agent status | submitted | submitted |
| tokens / turns / tool calls | 175,431 / 72 / 56 | 163,392 / 71 / 55 |
| wall time | 212 s | 200 s |
| mitigation hypotheses recorded | **0** | **0** |
| assertions | 13 (raises-concern / explains) | 12 |
| validation requests | 3 (all rejected: duplicate of an existing override-inspection) | 3 (2 fulfilled → new override-inspection@v2 records, both `not-run`; 1 rejected duplicate) |

Whole run: 338,823 tokens, 412 s. The cost-table rows "human review" in a B0-vs-A0 comparison are **not comparable** (B0 holds
2 cases, A0 holds 8).

### Agent verdicts on the `rotationPolicy` items (expected: eu affected, us not-affected)

| env | item | class | confidence | scored |
|---|---|---|---|---|
| eu | `ci-86548e9a2300` (computed/CRD item, primary) | **affected** | medium | correct (B1, strongest over its two items) |
| eu | `ci-93299c596271` (release note) | potentially-affected | medium | — |
| us | `ci-86548e9a2300` | **potentially-affected** | medium | hedged |
| us | `ci-93299c596271` | potentially-affected | medium | — |

Scored table (B1/B2 of the two cases): correct 1, hedged 3, false-safe 0, false-alarm 0. **S6 is not met:** the verdict
for the `rotationPolicy` item differs between environments (affected vs potentially-affected, the right direction) but
`payments-us` is hedged, not not-affected.

Mechanisms and evidence (all `statementKind: unverified-ai-prose`, provenance complete, citations are the entry item's
evidence `ev-cac9c8fee493` (eu, `…2300`), `ev-661db86897e8` (eu, `…6271`), `ev-417e2b731b1e` (us, `…2300`),
`ev-448d800bffcc` (us, `…6271`); `citationsFromEntry: true`, i.e. inherited, not chosen by the agent):
- **eu / 2300, affected:** default Never→Always, each re-issuance generates a new key. `checkout-tls`, `ledger-tls`, `mesh-root-ca`
  leave it unset (claim `clm-709d931a9f6c`), gate not turned off (`clm-e566fee85694`), override inspection `vr-6f38b5064054`
  found no pin; "mesh-root-ca is a CA root", consumers might pin keys (assumption, unverified). Would overturn: effective
  Certificates set `Never`, the gate off, or no consumer depends on key stability.
- **us / 2300, potentially-affected:** same mechanism; `checkout-tls`/`ledger-tls` unset; "`mesh-root-ca` has an explicit but
  redacted value in the prod overlay, which may be Never" (stated as uncertain); assumes the controller args do not disable
  the gate (searched environment files only).
- **us / 6271:** "only mesh-root-ca sets rotationPolicy (`clm-748baf8f3a65`), value redacted"; the two payments certificates
  inherit the new default. Proposes: override-inspection with unredacted values; "add `rotationPolicy: Never` explicitly".
- The agent's own `proposedValidations` are generic (set the field and re-run); none is a structured mitigation hypothesis.

**Why no hypothesis:** the agent reached "unset on the leaf Certificates; CA value unknown/redacted" and stopped at
uncertainty. It proposed no rule of the form "the CA's setting makes the leaf rotation harmless". Nothing was
exported, so the verify→promote steps (PROTOCOL §6.4–5) have **no input from B0** on this pair as it stands.

### B0 policy invariance

Class, rule, basis, eligibility and decision of all 36 findings are identical to A0-F1. The findings are **not byte-identical**
as PROTOCOL §4 states ("must be byte-identical to A0"): the agent arm removes the `agent: not run` gap and its approval
blocker, appends the agent-assertion sentence to `reasoning`, adds `assertions`, and on payments-us adds two
override-inspection@v2 records to two findings' `validations` (both `not-run`). I read this as the intended attachment of
agent output, not as the policy consuming it; flagged for the commander because the protocol text says byte-identical.

## §A0-F2 — deterministic baseline on F2 (8286e36f + docs), K0 — current

Run `A0-K0-20261017-6d2ae5bf` (binary 6d2ae5bf, clean; K0 digest `sha256:672a7ce0…4ff5`; fresh `-work`). Supersedes §A0-F1.

**A0-F2 vs A0-F1 (`A0-K0-20261017-d262210d`), per item (24):** class, rule, basis, manifestation, eligibility, decider and
correctness are **identical on all 24 items**; so are run decisions and class counts. Accuracy 3/24, affectedDetection 3/3,
2 clearances (both correct, obs-north B3 and obs-south B3), 0 unsafe clearances, unsafeEligibilityRate 1/8 (shop-nl B1, as before).
What changed is evidence text only:

| change | where | why |
|---|---|---|
| new impact-path ids (counts unchanged: 15 and 10 gaps) | cert-manager eu/us, both items | path ids are content hashes; the redaction fix changed what is collected for the Certificates. I did not diff the facts to confirm `rotationPolicy` is the cause. |
| override-inspection v2 → v3 and render-diff id churn in gap text | kube-prometheus-stack B2/B3, ingress shop-nl B1 reasoning | validator versions moved in F2; same verdicts |
| `traversalComplete` false → true | cert-manager eu/us (vs the earliest A0 `bfebde39`) | fixes5/F1 already; unchanged between F1 and F2 |

So F2 did not change any A0 decision. The `rotationPolicy` items stay `potentially-affected` (controller-behaviour, no
clearing pin rule in K0); the blocker analysis in §A0 stands.

## §B0-F2 — agent arm, all 8 cases, K0 — current

Run `B0-K0-20261017-6d2ae5bf`: `-agent -agent-model claude-sonnet-5-5`, prompt `investigate-agent/v7`, fresh `-work`, same
binary and K0 as A0-F2. All 8 cases ran the agent (status `submitted`) and each has its own investigation id (no run-key reuse).

| case | investigation | agent tokens | turns / tool calls |
|---|---|---|---|
| cert-manager payments-eu | inv-a2332d5ab309 | 165,332 | 63 / 47 |
| cert-manager payments-us | inv-b20eca4cfe98 | 161,910 | 64 / 49 |
| ingress-nginx shop-de | inv-ff6cf34928f5 | 112,016 | 62 / 48 |
| ingress-nginx shop-fr | inv-d3ddb63f76e2 | 103,612 | 53 / 42 |
| ingress-nginx shop-nl | inv-108304fbac44 | 112,322 | 60 / 45 |
| ingress-nginx shop-pl | inv-e54a22252f45 | 118,228 | 66 / 50 |
| kube-prometheus-stack obs-north | inv-b1b3a13440d8 | 146,058 | 71 / 59 |
| kube-prometheus-stack obs-south | inv-e1c0e384aeaf | 137,316 | 62 / 49 |
| **total** | | **1,056,794** | wall 1,565 s (26 min) vs A0 23 s |

**Policy invariance:** the class, rule, basis, eligibility and decision of every finding in all 8 cases equal A0-F2
(PROTOCOL §4 as amended).

### Agent verdicts (ungated, provisional)

correct 11, false-safe 0, false-alarm 0, hedged 12, overstated 0, no-verdict 1 (obs-south B3). Per case: shop-fr 4/4 correct,
shop-nl 2/2, obs-north 3/3, obs-south 2/3 (+1 no-verdict); hedged on all of shop-de, shop-pl, payments-eu, payments-us (every
item `potentially-affected`). No verdict was unsafe: the agent never said not-affected where the expectation is affected.

### rotationPolicy item (ci-86548e9a2300), both environments

- **payments-eu (expected affected): potentially-affected, confidence medium.** Mechanism: default `rotationPolicy` Never →
  Always (gate `DefaultPrivateKeyRotationPolicyAlways`, on by default); every re-issuance makes a new key. Environment: 3
  Certificates inherit the default, none explicit (claims `clm-b3137f38c077`, `clm-d99138fa1716`; override-inspection
  `vr-95cc27670c43` found no pin). Cited evidence `ev-cac9c8fee493`. Uncertainty: whether a consumer pins keys (mesh CA).
  Would overturn: explicit Never on all three, the gate disabled, or no consumer depends on key stability.
- **payments-us (expected not-affected): potentially-affected, confidence medium.** Same mechanism. checkout-tls and
  ledger-tls leave the field unset; **mesh-root-ca sets Never explicitly** (the F1 run could not see this: W-0027 is fixed).
  Cited `ev-cac9c8fee493`, claim `clm-c02505c2d6b2`, validation `vr-231a6e4b18d6`. It assumed the controller args carry no
  gate override (searched environment files only). So the verdict is hedged, not false-alarm, by the §8 scoring.

Both verdicts now rest on the real field values. Neither reached a class stronger than the deterministic one.

### Mitigation hypotheses

**None.** 0 hypotheses recorded on any of the 8 investigations (no well-formed, no malformed). `ri investigate
export-hypotheses` run for all 8 ids into a scratch copy of K0 (not `knowledge/`) returned `hypotheses: []` each time; the
copy is byte-identical to K0. The agent expressed its mitigation ideas only as prose `proposedValidations` ("set rotationPolicy:
Never … re-run override-inspection"). Consequence for the experiment: C1 has **no B0-derived input** from these cases; if
K1 is to be promoted it must come from somewhere else, which is a protocol question for the commander.

## §B0-v8 — agent arm on the development pair, prompt v8, K0 (F3) — the pre-registered single shot

Binary edb21ed9 (clean; F3 89328b6f merged; `go build/vet/test ./...` green). K0 digest unchanged. One run, not repeated:
`B0-K0-20261017-edb21ed9-v8` (`-case` payments-eu,payments-us; `-agent -agent-model claude-sonnet-5-5`, default prompt;
every agent record says `investigate-agent/v8`; fresh `-work`). Alongside it, A0 on all 8 cases on the same binary:
`A0-K0-20261017-edb21ed9`. (The command exited 1 because gates fail/insufficient-data, as for every run; it did not crash.)

**A0-F3 vs A0-F2:** no difference on any of the 24 items — decisions, gap and reasoning text, and every metric are identical.
**Policy invariant vs A0-F3:** class, rule, basis, eligibility and decision of every finding on the dev pair equal A0-F3.

| case | investigation | tokens | turns / calls | agent wall | items examined |
|---|---|---|---|---|---|
| payments-eu | inv-98ea0dfc2787 | 153,361 | 59 / 45 | 161.5 s | 10 of 18 |
| payments-us | inv-377edfda16af | 148,081 | 55 / 45 | 155.6 s | 10 of 18 |
| total | | 301,442 | | 324 s (A0: 41 s for 8 cases) | |

**Budget exhaustion:** none. Every examined item's session ended `submitted`; the largest item used 30,470 tokens (limit 150,000)
and 9 turns (limit 14). The only ceiling reached is the item cap: 10 of the 18 agenda items got a session and 8 are
`not-examined` in each case (budget rule `item@v1`, cap 10). Case-level token totals exceed 150,000 because the budget is per
item, not per case.

**Agent verdicts (ungated):** correct 1, hedged 3, false-safe 0, false-alarm 0, overstated 0, no-verdict 0.

### rotationPolicy item (ci-86548e9a2300)

- **payments-eu (expected affected): `affected`, confidence medium — correct.** Mechanism: default Never → Always; each
  re-issuance of a Certificate leaving the field unset generates a new key. Environment reasoning: mesh-root-ca, checkout-tls
  and ledger-tls all inherit the default; neither the field nor the gate is set in the environment. Consumers traced: **none**
  ("mesh CA trust, payments TLS endpoints … I did not trace these consumers, and the Secrets are absent from the snapshot").
  Citations `ev-cac9c8fee493`; claims `clm-44d796a9f8c6`, `clm-92f59c4780cb`; validation `vr-567a81391649`. Uncertainty: whether any
  consumer depends on a stable key. Proposed validation: check the rendered Deployment's controller args for the gate. Would
  overturn: explicit Never, or the gate false. It states "I recorded no mitigation."
- **payments-us (expected not-affected): `potentially-affected`, confidence medium — hedged.** Same mechanism. checkout-tls and
  ledger-tls inherit the default; mesh-root-ca is explicit ("redacted but is the digest of Never") and unaffected; gate not
  disabled anywhere. Consumers traced: **none** (Secrets absent from the snapshot). Citations `ev-cac9c8fee493`; claim
  `clm-861f8b91c4ad`. Proposed validations: override-inspection on the two Certificates; a runtime re-issuance check. It treats
  the gate-off condition as a controller flag and "did not record it as a mitigation".

### Hypotheses

**`record_hypothesis` was called zero times** in either transcript (tool names used: get_change_item, record_claim,
search_manifest, get_rendered_object, request_validation, record_assertion, query_diff, submit_findings). So there are **no
rejected calls and no stored mitigations** to quote. The agent said in prose, per item, that it recorded none (e.g. "I recorded no
mitigation hypothesis, because the verdict rests on ACME not being used at all"). `ri investigate export-hypotheses` for
both investigations into a scratch copy of K0 (`K0-export3`, not `knowledge/`) returned `hypotheses: []` and wrote no files; the
copy is identical to K0. `ri verify` not run.

Observation, not a conclusion: on this pair the redaction fix made the field values visible and the agent used them (it saw
mesh-root-ca's explicit setting in us), but the mitigation it identified — an explicit `rotationPolicy: Never` — it
treated as a remedy ("would overturn"), not as an existing environment property to record as a hypothesis.
