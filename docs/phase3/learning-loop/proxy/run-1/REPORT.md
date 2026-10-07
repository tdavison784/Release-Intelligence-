# Proxy run-1: blind Opus review of every pending non-high item (2026-10-02)

**Scope.** 952 pending review items whose routing priority is not `high`: 684 semantic-mapping,
applicability, consequence and classification items, plus 268 evidence-sufficiency gates. The high items
are the product owner's.

**Reviewer.** `claude-opus-5-5`, one stateless `claude -p` call per item, prompt `proxy-review/v1`. The
proxy was blind: no environment, no model names, no proxy decisions, nothing from `eval/`. Decisions are
labelled `reviewerKind: proxy` and are **never** presented as human.

**Files.**
- `ledger.jsonl`: one line per attempt, recording the outcome and the proposal authorship.
- `responses/`: the verdicts with call metadata.
- `requests.tar.gz`: the exact prompts.
- `proxy-report.{txt,json}`: `ri knowledge proxy-report`.
- `metrics.txt`: `ri knowledge metrics` over the whole store after the run.

## Outcome

| | n |
|---|---|
| items | 952 |
| decisions recorded (one per item, never batched) | **940** |
| verdicts refused at recording (item stays pending, not retried) | 12 |
| calls that failed on the account session limit | 7 (they never reached the model and were decided after the reset) |
| proxy-level facts minted | **96** (59 review-required · 34 informational · 3 action-required) |

**By action:** accept 400 · correct 121 · reject 42 (1 of them a duplicate) · need-more-evidence 323 · defer 54.

| Question type | n | accept | correct | reject | need more evidence | defer |
|---|---|---|---|---|---|---|
| semantic-mapping | 219 | 120 (55%) | 47 (21%) | 24 (11%) | 26 (12%) | 2 |
| applicability | 185 | 68 (37%) | 57 (31%) | 17 (9%) | 43 (23%) | – |
| consequence | 264 | 209 (79%) | 17 (6%) | 1 | 37 (14%) | – |
| classification | 4 | 3 | – | – | 1 | – |
| evidence-sufficiency | 268 | – | – | – | 216 (81%) | 52 (19%), all "evidence sufficient" |

**Labels on corrections and rejections:**

| Label | n |
|---|---|
| wrong-applicability | 74 |
| wrong-subject | 57 |
| wrong-change-type | 32 |
| wrong-consequence | 17 |
| wrong-classification | 9 |

Applicability is where the proposals are weakest. Only 37% of the proposed exposure conditions were
accepted as-is, consistent with its 23% cross-model agreement (`metrics.txt`).

## Per proposing model (proxy decisions only)

| Model | Relation to proxy | Accepted as-is | Accepted after correction | Rejected | Accept rate (as-is) |
|---|---|---|---|---|---|
| claude-opus-5-5 | self-model | 95 | 1 | 48 | 0.67 (0.66) |
| claude-sonnet-5-5 | self-family | 252 | 32 | 112 | 0.72 (0.64) |
| claude-haiku-4-5 | self-family | 213 | 89 | 147 | 0.67 (0.47) |
| glm-5.3 | other-family | 96 | 4 | 58 | 0.63 (0.61) |

The counts are of proposals, from `knowledge.ComputeMetrics` with only the proxy's decisions. A proposal
counts as "rejected" when its aspect value lost to the decided value.

**Self-review bias (brief step 3).** The proxy accepted its own model's proposals as-is at 0.66, against
0.61 for the other family (GLM) and 0.64 for Sonnet. That gap is small, and with these sample sizes it is
not distinguishable from noise. It is the only clean self-vs-other comparison available.

The decision-level split is confounded:
- 935 of 940 decisions had at least one Claude proposal under review, so "self-family" covers almost
  everything.
- Split by who authored the *proposed* assertion, the rates differ more:

  | Proposed assertion authored by | n | accepted |
  |---|---|---|
  | includes the self-model | 38 | 89% |
  | mixed families | 75 | 92% |
  | other family only | 51 | 43% |

  But these groups are not comparable. Items proposed by GLM alone are mostly ones where every Claude
  model abstained, and 49% of them went to need-more-evidence. Read that as evidence quality, not
  self-preference. The human shadow pass (step 5) is what can measure whether the proxy's agreement is
  biased.

## Findings for the commander

1. **Prose-only corrections cannot be recorded (11 of the 12 refusals).** The proxy kept the
   consequence kind or the condition but corrected the prose: an unsupported statement or remediation, such
   as an invented `--log-format` flag or a wrong "old default". Aspect digests exclude prose, so
   `ReviewDecision.Validate` refuses these as corrections that change nothing. A human on the dashboard
   would be refused the same way. Contract question: should a correction that changes only
   `Consequence.Statement`/`Remediation` be allowed, for example with a `wrong-consequence` label and no
   digest change?
2. **One correction was over the 400-character statement limit.** The v1 prompt did not state the limits;
   the v2 prompt does (the shadow pass will use v2).
3. **Three proxy facts carry an action-eligible consequence** (`setting-ignored`):
   - `vf-e9851c5565c4`: argo-cd `ApplicationSet.spec.applyNestedSelectors`;
   - `vf-b480fd763811`: cilium hubble-relay `--dial-timeout`;
   - `vf-ab831aa869fc`: cilium `hubble.relay.dialTimeout`, with a deterministic subject and the rest proxy.

   The trust ladder caps proxy facts at REVIEW (DESIGN §4), so none of them can produce ACTION REQUIRED.
   Their consequence aspect is worth a human look.
4. **Evidence-sufficiency gates.** 216 of the 268 gates were judged insufficient. The 52 judged
   sufficient are recorded as `defer` with the reason "evidence sufficient …", so they stay open for
   re-proposal or a human.

## Cost and time

- $60.10 CLI-reported, about $0.064 per call.
- Median call 6.6 s, p90 11.4 s; 4 calls in parallel.
- Wall clock 3h21m, including a pause of about 2h45m at the account session limit; about 35 min of
  calling.
