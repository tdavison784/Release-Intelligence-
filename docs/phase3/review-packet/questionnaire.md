# Reviewer questionnaire

Answer in your copy of `answer-sheet.template.md`. IDs (`imp-…`, `ev-…`,
`chg-…`) are stable identifiers used in the reports and their JSON. One-line
answers are fine; "don't know" is valid everywhere. Questions marked **[per
report]** are asked once per report (R1 = report-1-compat-action-required, R2 =
report-2-values-default-informational, R3 = report-3-repo-mode, R4 =
report-4-ai-enriched).

The five buckets the tool uses: **ACTION REQUIRED** (your environment must
change or something breaks), **REVIEW REQUIRED** (credible overlap, not
provably necessary), **INFORMATIONAL** (applies to you, you appear safe),
**NOT AFFECTED** (checked, does not apply), **UNKNOWN** (could not tell you;
states what evidence was missing).

## A. Completeness

- **A1 [per report]** Is anything important **missing** — a change you would
  have wanted to know about before this upgrade that the report does not
  surface anywhere (in any bucket)? If yes: what, and where would you look
  for it normally?
- **A2** Is anything missing across all four reports (a kind of change or
  environment input the tool never asks about)?

## B. Correctness

- **B1 [per report]** Is anything in the report **wrong** (a factual claim,
  a version, a value, a direction of a change)? Cite the finding id.
- **B2 [per report]** Is anything **falsely marked ACTION REQUIRED** — a
  finding that says your environment must change, where you believe it does
  not? This is the costliest error class, so it has its own question. (R1 and
  R3 have exactly one ACTION REQUIRED finding each: `imp-7e9a11c2834d`.)

## C. Noise

- **C1 [per report]** Is the amount of **noise** acceptable — findings you
  don't need (in any bucket), sections you'd skip, repeats? What fraction of
  the report was noise to you (rough %)?
- **C2** The UNKNOWN bucket is deliberately the largest (50 of 58 items in
  R1–R3, each naming the missing evidence). Is that volume acceptable, or
  would you want it collapsed/hidden by default?

## D. Uncertainty

- **D1 [per report]** Is **uncertainty represented correctly** — do the
  UNKNOWN items honestly state what the tool couldn't determine, and does
  anything claim more certainty than you'd grant it?
- **D2** In R4, does the AI layer stay in its lane — suggestions clearly
  weaker than deterministic verdicts, honest about what it doesn't know — or
  does it overreach anywhere?

## E. Evidence

- **E1** You were asked to spot-check **3 citations** (see packet README,
  step 4). Record for each: report, finding id, evidence id, URI, locator,
  and whether **URL + line + excerpt all held up**.
- **E2 [per report]** Can you **follow the evidence** generally: given a
  finding, could you trace why the tool concluded what it did, without
  reading source code? If not, what blocked you?

## F. Value

- **F1** Does this **save you research time** compared with how you do this
  today (release notes + upgrade guide + chart diff + CRD diff + your config,
  by hand)? Roughly: minutes saved per upgrade like this one, or "no".
- **F2** **Would you use it before a real upgrade?** (yes / no / only if …)
- **F3 [per report]** **Which findings would you act on without
  re-researching** — finding ids you'd take straight into a change ticket as
 -is?
- **F4** What single improvement would most increase the chance you'd use it?

## G. Per-finding adjudication

For every actionable finding below: do you agree with the classification? If
not, what should it be (one of the five buckets), and why (one line)?

### Report 1 (compat action-required; cluster 1.28)

| Finding | Class (as reported) | Agree? | Corrected class | Comment |
|---|---|---|---|---|
| imp-7e9a11c2834d | action-required | | | Cluster 1.28 below supported range 1.29–1.33 |
| imp-123e6594ecd5 | review-required | | | Controller image reference moves v1.17.0→v1.18.0; env pins old |
| imp-a137dded3ed6 | review-required | | | Webhook image reference moves v1.17.0→v1.18.0; env pins old |
| imp-0dae408f0c17 | review-required | | | Values set `global.rbac.disableHTTPChallengesRole`, new in v1.18.0 |
| imp-6cae808def98 | informational | | | Values pin `prometheus.servicemonitor.targetPort`; default change does not apply |

### Report 2 (informational case; cluster 1.31)

| Finding | Class (as reported) | Agree? | Corrected class | Comment |
|---|---|---|---|---|
| imp-123e6594ecd5 | review-required | | | Controller image pin |
| imp-a137dded3ed6 | review-required | | | Webhook image pin |
| imp-0dae408f0c17 | review-required | | | Values set new-in-v1.18.0 key |
| imp-e43eb9c2af2b | informational | | | Cluster 1.31 inside supported range |
| imp-6cae808def98 | informational | | | Pinned default does not apply |

### Report 3 (repo mode; cluster 1.28, environment discovered from a repository)

| Finding | Class (as reported) | Agree? | Corrected class | Comment |
|---|---|---|---|---|
| imp-7e9a11c2834d | action-required | | | Cluster 1.28 below supported range |
| imp-123e6594ecd5 | review-required | | | Controller image pin (discovered from repo) |
| imp-a137dded3ed6 | review-required | | | Webhook image pin (discovered from repo) |
| imp-0dae408f0c17 | review-required | | | Values set new-in-v1.18.0 key |
| imp-6cae808def98 | informational | | | Pinned default does not apply |

Also for R3, judging only from `example-env/customer-repo/` and the report's
warnings section: did discovery pick up anything it shouldn't, or miss
anything it shouldn't?

### Report 4 (AI-enriched; same environment as R1)

The five deterministic findings are the same as R1 (adjudicate in R1's table).
For the AI layer, adjudicate the **13 UNKNOWN findings that carry a `[suggests
review]` suggestion** — agree the suggestion is worth a reviewer's attention
(vs. dismissible without looking)?

| Finding | Suggestion topic | Worth a look? | Comment |
|---|---|---|---|
| imp-eef2b0d0d72f | ACME HTTP01 PathType → Exact (breaking) | | |
| imp-80754ea19305 | ACME profiles extension | | |
| imp-16163701185e | Label added to Let's Encrypt account keys | | |
| imp-6432f469b85a | New certificate issuance/expiration metrics | | |
| imp-2577b5434545 | `ciss` short name for ClusterIssuer | | |
| imp-62fb5ea91b14 | New helm value `global.rbac.disableHTTPChallengesRole` | | |
| imp-d09df14ed530 | Signature algorithm customization | | |
| imp-fa8df652362e | Dependency bump: golang-jwt (GHSA-mh63-6h87-95cp) | | |
| imp-50009880b265 | Dependency bump: go-jose (CVE-2025-27144) | | |
| imp-82d81560bbc8 | Dependency bump: x/crypto (GHSA-hcg3-q754-cr77) | | |
| imp-88e0a1b18609 | Dependency bump: x/oauth2 (CVE-2025-22868) | | |
| imp-4abadd43d378 | Internal ACME fork change | | |
| imp-d1ff8cfc0c11 | New `--extra-certificate-annotations` flag | | |

Also for R4: the 5 AI **notes** that do NOT suggest review (marked
`probably not applicable` or `undetermined` in the `AI enrichments` section)
— is any of them wrong or overconfident?

## H. Wrap-up

- **H1** Anything in this packet (not the tool) that blocked your review —
  missing context, confusing presentation, broken links?
- **H2** May we quote your one-line answers in internal reports? (yes/no)
