# Milestone 6 gate (pre-registered 2026-10-28, before the blind cases exist; never edited, amendments appended)

The blind set is `eval/investigations-blind6/`: 4 affected/unaffected pairs, 8 environments, authored sealed by
`groundtruth6`. The commander reads its labels only at scoring, after every run. Arms and freeze are as in PLAN-M6.

## Scoring units

**Decisive item:** one per environment, the one named `decisive` in `expected.yaml`.

**Agent assessment outcome** at the decisive item, for arms B, C, D (per run):
- **correct:** the class equals the expected class. For an expected `insufficient-evidence`, an
  `insufficient-evidence` or `potentially-affected` that names the missing input also counts as correct.
- **hedged:** `potentially-affected` where `affected` was expected, or where `not-affected` was expected with the right
  mechanism stated.
- **false-safe (FS):** `not-affected` where `affected`, `potentially-affected` or `insufficient-evidence` was expected.
- **false-alarm (FA):** `affected` where `not-affected` was expected.

**Authoritative classification** (all arms, from the stored records): the finding class at the decisive item, and the
decision.

## Hard safety requirements (any failure fails M6)

1. **Unsafe authoritative clearances = 0** in every arm: no finding `not-affected` unless expected `not-affected`.
2. **`unsafeEligibilityRate` = 0** in every arm.
3. **Arm E promotes knowledge only through the product owner's review,** and a promoted rule never clears the
   affected side of its pair.

## Primary criteria ("meaningful improvement over the RI baseline", mission M6)

Let `correct(X)` be the mean number of correct decisive items over arm X's repeats, out of 8, and `FS(X)` the mean
false-safes.

- **P1, investigation quality.** `correct(C) ≥ correct(B) + 2` **and** `correct(C) ≥ 5`.
- **P2, safety of the assessments.** `FS(C) ≤ FS(B)` **and** `FS(C) ≤ 1`.
- **P3, consumers.** In every environment whose decisive mechanism runs through a consumer (as `NOTES.md` states),
  C names that consumer (object and how it uses the changed output) in ≥ 2 of 3 runs, in at least 75% of such
  environments.
- **P4, uncertainty.** In the incomplete-configuration environment, C and D are correct (uncertain, naming the missing
  input) in ≥ 2 of 3 runs.
- **P5, version and advisory decisions.** In the pair whose target has a security regression, D's judgement flags the
  regression and recommends a target at or above the expected safer target in ≥ 2 of 3 runs. In no pair does D
  recommend a target that RI's edge security marks as carrying a regression.

**M6 passes when every hard requirement holds and P1, P2 and P5 hold.** P3 and P4 are reported. Failing either is a
named finding, not a gate failure.

## Arm E (reported against these targets)

- **E1:** at least one legitimate new authoritative `not-affected` (after promotion), on the unaffected side of a pair,
  and none on the affected side.
- **E2:** in the environment whose controller configuration differs, no observed default from a verifier fixture
  carries over into a clearance.
- **E3:** the human review burden, in decisions and minutes, and every confirmed rule the product owner rejects, with
  the reason.

## Recommendations (D)

The product owner grades 4 D judgements, drawn by the commander before reading any label: one per pair, from repeat 1.
Each is graded as `usable as is` / `minor edits` / `materially wrong`. **Target: at most 1 materially wrong.**
Reported; not gating.

## Also reported

For every arm: tool calls, artifacts read, tokens, cost, wall time, evidence requests (made and fulfilled), knowledge
created, and occurrences of the PD-7 known limitations (a misread of a correct citation; observed-default carry-over;
auto-approval facts).

## Amendment A1, 2026-10-29 (harness only; before any blind run completed)

`ri eval -investigations` requires a `gates.yaml` in the dataset directory. `eval/investigations-blind6/gates.yaml`
is an unchanged copy of `eval/investigations-heldout/gates.yaml` (the frozen Phase 4/5 harness gates). It drives only
the harness's own pass/fail exit code. **M6 is scored by this file (GATE-M6), not by `gates.yaml`.**

## Amendment A2, 2026-10-29 (a crash fix, product owner's decision; before any agent arm ran)

**What happened.** Arm A (`M6-A`) crashed on both external-secrets environments:
`collect-change-evidence: … artifact path "upstream/external-secrets/github.com/external-secrets/external-secrets/compare/v0.18.0...v0.19.0" must be relative`.
The upstream document path builder (`bundle.docPath`) kept the `...` of a GitHub compare URL, and the domain rejects
`..` in artifact paths.

**The product owner's choice** (option 1): fix it before the agent arms, so every arm runs identical code on all 8
environments.

**The fix.** Runs of two or more dots in a URL path become `_` (`internal/investigate/bundle/change.go`, test
`TestDocPathDotRuns`). It is generic and unrelated to reasoning; stored runs are unaffected, because such paths
previously crashed.

**Records.** The new freeze tag is `m6-freeze-a2`. Arm A is re-run as `M6-A2`, and all agent arms run on
`m6-freeze-a2`. `M6-A` is kept as the record of the crash, a **blind finding of the baseline**.

**Disclosure.** The eval tool's summary of `M6-A` printed the expected classes per environment, and the commander read
it before the agent arms. Code, prompts and these thresholds were already frozen; nothing changed in response.

## Amendment A3, 2026-10-29 (sample rule; written before any arm-D judgement exists)

The labels were seen early (A2 disclosure). To remove any commander discretion, the 4 graded D judgements are fixed
mechanically: run `M6-CD2-r1`, and in each pair **the environment whose case id sorts first**:
- `argo-cd-2.14-3.0--studio-east`
- `cert-manager-1.20-1.21--treasury-de`
- `external-secrets-0.18-0.19--orders-br`
- `traefik-3.6-3.6--retail-apac`

## Amendment A4, 2026-10-29 (infrastructure failure; decided from run status only, no results read)

**What happened.** `M6-CD2-r3` hit the account's Claude usage limit partway through. In 5 of its 8 environments
(cert-manager treasury-us, both external-secrets, both traefik), the agent's transcript and the synthesis stage
end with "You've hit your 5-hour limit". The agent status is `failed` and there is no judgement. This is an
infrastructure failure, not an outcome of the design.

**Decision.**
- `M6-CD2-r3` is **void**. It stays stored, as the record.
- Repeat 3 of C+D is **re-run in full** on `m6-freeze-a2` as `M6-CD2b-r1`. No stitching of partial runs.
- C and D repeat 3 are scored from `M6-CD2b-r1`.

**Other failures stay as results.** The other agent failures (`M6-B2-r1`, `M6-B2-r3` and `M6-CD2-r1`, one
environment each) have no usage-limit text, so they remain results of their arms.

**The rule from now on:** any run whose transcripts contain the usage-limit text is void, and is re-run in full.
