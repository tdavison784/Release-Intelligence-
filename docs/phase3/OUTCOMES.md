# Phase 3 outcomes

> The question this phase had to answer:
> **Can Release Intelligence accurately determine which upstream changes matter to a
> specific real-world environment, explain why, distinguish required action from noise
> or uncertainty, and produce upgrade guidance an experienced engineer would trust?**
>
> **Answer: the trust property holds — the system never invents required action
> (falseActionRate 0.00 across 47 labelled units, zero NOT-AFFECTED→ACTION errors),
> every finding is evidence-backed, and uncertainty is explicit — but applicability
> for note-derived changes is the measured frontier (applicabilityAccuracy 0.10 vs the
> ≥0.80 gate, FAILED and left failing).** The deterministic engine is safe but blind
> on prose; the GLM layer suggests review; the human decides. That division is the
> phase's product.

All numbers are computed: `ri eval` over the 17-case dataset (115 blind-researched,
cited expectations), the gate panel in `eval/REPORT.md`, the proxy reviews and pilot
under `docs/phase3/`.

## What was built (by the phase's three layers)

**Deterministic engine** (Rounds 1–2, 4):
- The five-class action contract (`docs/ACTION_CLASSIFICATION.md`, normative, enforced
  by `Validate()` and JSON-Schema conditionals): ACTION REQUIRED requires dual evidence
  chains + high confidence (below-high demotes centrally); NOT AFFECTED requires the
  deciding environment dimension to have been supplied; UNKNOWN carries
  `neededToDetermine`; severity and confidence are separate axes.
- An explicit applicability step before any action class, with environment-visibility
  scoping (no-match ≠ not-affected; absence is not knowledge — `env.Health`).
- Correctness fixes found before expanding: three compatibility bound-semantics bugs
  (minimum/maximum compared as equality; maximum ignored by the join — false
  "below-minimum" alarms for newer clusters); YAML stream decoding (block-scalar
  separators), duplicate keys, truncation warnings; GVK-scoped CRD matching with the
  identity ladder (exact GVK+field → action; kind unpinnable → review; group-only →
  review; path-only → unknown) and adversarial regression tests.
- The environment model: GVK usage inventory, installed-product detection (Helm/Argo/
  Flux/Helmfile), `--repo` discovery mode with precedence rules, per-dimension health.
- Routine-maintenance classification: 96–100% of the measured Phase-2 false positives
  (Vault/Cilium/ingress-nginx) classified deterministically with security carve-outs;
  the argo-cd firehose fell 399 → 126 in the narrative. Security items factored into
  ONE predicate (`upgrade.IsSecurityItem`) shared by the carve-out and the new
  `impact:security-fix` visibility rule.
- Collapsed UNKNOWN view, inline citations, compatibility wording honesty
  (pre-existing exclusions and kubeVersion-vs-supported divergence stated).

**GLM-5.3-Flash layer** (Round 2, live):
- `ri impact -enrich` on the user's Z.AI free quota: deterministic candidate selection
  (unknown note-derived non-routine findings; duplicate clusters; migration synthesis),
  bounded prompts (values never sent), three verdicts only, forbidden transitions
  validator-enforced (AI cannot produce action-required, cannot delete findings,
  cannot invent evidence, confidence capped at medium).
- Live: 46 candidates → 30 prompts → 28 accepted, 2 refused whole; **19 of 51 unknowns
  carried review suggestions with environment-grounded reasoning**; proxy reviewers
  scored suggestion precision 62–69% ("worth a look" quality).

**Human validation** (Round 3, honestly scoped):
- A self-contained review packet (4 reports across the classification spectrum,
  questionnaire, pre-registered scoring, reproduction script) — all 62 distinct
  upstream citations verified live (URL+locator+excerpt), 100%.
- Two blind PROXY reviews (AI-simulated personas, prominently labelled): **0 false
  ACTION REQUIREDs**, adjudication agreement 93%/100%; their filed issues became
  Round 4's fixes.
- Time-study methodology (within-subject, counterbalanced, blind plan-quality rubric)
  + an n=1 PROXY pilot, labelled as protocol validation only.

## The gate panel (pre-registered thresholds, 17-case dataset)

| Gate | Value | Threshold | Result |
|---|---|---|---|
| criticalRecall | 1.00 | ≥ 0.95 | PASS |
| importantRecall | 1.00 | ≥ 0.90 | PASS |
| falseActionRate | 0.00 | < 0.05 | PASS |
| actionFindingEvidence (both chains) | 1.00 | = 1.00 | PASS |
| unsupported conclusions | 0 | ≤ 0 | PASS |
| pipelineFailures | 0 | ≤ 0 | PASS |
| **applicabilityAccuracy** | **0.095** | **≥ 0.80** | **FAIL** |

recall 1.00 (115/115) · labeledPrecision 0.57 (raw 0.64; 150 adjudications, 13
adjudicated true positives the must-find lists lacked) · unknownRate 0.73 ·
duplicateRate 0.03 · classificationAccuracy 0.46.

## The honest reading

**What the evidence supports:** the engine is trustworthy in the direction that
matters — it never fabricates required work, every conclusion resolves to inspectable
evidence on both chains, uncertainty is explicit and actionable (`neededToDetermine`,
AI suggestions clearly labelled), and the adversarial pack (10 traps: sibling keys,
group/kind confusion, pinned defaults, missing sources, boundary versions, GVK
path-collisions, 3-source duplication, runtime-dependent behavior) fooled it into no
wrong action class. Recall is perfect on what the dataset asks; the failure mode is
**under-decision**, not wrong action.

**What fails, by design of the measurement:** applicability for note-derived changes.
28 of 35 expected-ACTION items sit in UNKNOWN because prose changes ("Default
`rotationPolicy` is now Always", "RBAC wildcard verbs removed") carry no
machine-comparable subject the join can match against environment facts. The AI layer
partially bridges this (19 suggestions on one fixture, 62–69% proxy-precision) but
cannot carry ACTION REQUIRED by contract — and should not, at 62–69%. Closing the
gap needs deterministic subject extraction from prose (a research-grade problem) and
a richer ground truth for suggestion scoring (12 of 13 suggestions were unlabelled —
a dataset gap, fixed going forward by requiring per-expectation classes, which this
phase's format now mandates).

**Proxy ≠ human.** The packet, questionnaire, scoring rubric and time-study protocol
are ready and de-risked (citation-checked, reproducible), but n=2 AI-simulated
reviews and an n=1 AI pilot validate the instruments, not the product. The
real-engineer session (4–8 participants, protocol written) is the first follow-up.

## Verdict against the phase question

The plan's success definition — identify the important changes that actually apply,
distinguish mandatory work from review/informational/non-applicable/uncertain, and
support every conclusion with inspectable evidence — is **met for computed changes**
(values, CRDs/APIs via GVK, images, compatibility: the deterministic layer's verdicts
were agreed 93–100% by reviewers with zero false actions) and **honestly incomplete
for prose changes**, where the system now says UNKNOWN instead of guessing, and the
AI layer's suggestions are usable leads rather than conclusions.

> **Engineers should trust ACTION REQUIRED enough that seeing it means something** —
> this is now true, and it is true because the system says it rarely (0.00 false rate,
> 3 ACTION findings across the labelled dataset, each dual-chain and hand-verifiable).

The remaining risk has moved from correctness to **coverage of applicability** — a
semantic-extraction problem with a working measurement loop, not an architecture
problem. Per the phase plan, that makes the next step product design around a
validated engine, with two named engineering fronts: prose-subject extraction and the
real-engineer study.
