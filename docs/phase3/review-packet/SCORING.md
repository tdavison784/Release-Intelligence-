# SCORING — how answers become metrics

This document is transparency, not instructions for reviewers: it fixes, in
advance, how a filled answer sheet turns into numbers, so the rubric cannot be
bent after the fact. n is expected to be tiny (2 proxy reviews now; a handful
of real engineers later); **every metric is reported with n** and no metric is
generalized beyond it.

## Per-question metrics

| Question | Metric | How computed |
|---|---|---|
| A1/A2 | Missing-item count | count of distinct concrete missing items named (dedupe across reviewers) |
| B1 | Wrong-claim count per report | count of finding ids cited as wrong; each is triaged: confirmed (upstream/env shows the report wrong) vs. rejected (reviewer misread) with the triage evidence recorded |
| B2 | **False-action count per report** | count of ACTION REQUIRED findings the reviewer disputes AND whose dispute survives triage. The headline false-action metric for the packet. Reported as: false-action rate = disputed-surviving / (# ACTION REQUIRED findings shown) |
| C1 | Noise acceptability | fraction answering yes; plus median reported noise % |
| C2 | UNKNOWN-volume verdict | distribution over {acceptable, collapse, hide, other-text} |
| D1 | UNKNOWN-honesty score | per report: honest=1 / mostly=0.5 / no=0; mean over reports and reviewers |
| D2 | AI-overreach count | count of concrete overreach instances named for R4 |
| E1 | **Citation survival rate** | verified citations / cited citations (a citation survives if URL resolves, locator points at matching content, excerpt matches within wording drift) |
| E2 | Evidence-followability | fraction yes/mostly; blockers listed verbatim |
| F1 | Self-reported minutes saved | per reviewer; also the pair (traditional estimate, RI estimate) if they gave both |
| F2 | Adoption intent | fraction yes; "only if…" conditions listed |
| F3 | Act-without-research set | per report: ids named; normalized as act-rate = ids named / 5 actionable findings |
| F4 | Top improvement | free text, deduped into a ranked list |
| H1 | Packet defects | free text, each becomes a packet issue |

## Adjudication agreement (section G)

For each actionable finding (5 per report × 3 deterministic reports; the R4
deterministic set is identical to R1 and is scored once, on R1):

- **Agreement rate** = agree / adjudicated, per finding and overall.
- A "no" becomes a **classification correction** only if the corrected class
  survives triage (same rule as B1): the reviewer's cited evidence is checked
  and the corrected class is one of the five.
- For R4's 13 suggestions: **suggestion-precision proxy** = worth_look=yes /
  suggestions adjudicated. (This is *not* precision of a classifier; it is
  "would this suggestion have survived a triage queue", which is what the
  suggestion layer is for.)

## Aggregation across reviewers

- All rates are means over reviewers, always with n.
- Inter-reviewer disagreement on any G row (one yes, one no) is not averaged
  away: the row is listed with both comments and becomes a discussion item,
  not a metric.
- Triage disputes: if the two proxy reviewers disagree with each other or with
  the tool, the dispute is resolved against the upstream sources (the same
  citation-spot-check procedure), and the resolution — not the vote — is what
  counts for B1/B2.

## What "good enough" means for Phase 3 (pre-registered targets)

These thresholds were fixed before the proxy reviews ran:

- false-action count across the packet = 0 (any surviving false-action is a
  release blocker for the feature);
- citation survival ≥ 10/11 spot-checked citations across reviewers (≥ 90%);
- adjudication agreement ≥ 80% of actionable findings;
- UNKNOWN honesty scored ≥ 0.5 (i.e. no reviewer calls it dishonest outright).

Failing a threshold does not fail the project; it produces a filed issue with
the evidence, which is the point of the packet.

## Known limits of this scoring

- n=2 proxy reviews are self-play: the "reviewers" share an origin with the
  authors. They validate the *packet and protocol* (are the questions
  answerable, does the scoring flow, do citations resolve), not human
  usability or real-world value.
- Self-reported minutes saved (F1) is the weakest metric; the task-based time
  study in `../time-study.md` exists to replace it with a measured pair, and
  its own pilot is equally labelled as n=1, AI-simulated.
