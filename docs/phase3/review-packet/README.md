# Release Intelligence — reviewer packet (Phase 3, Goals 13–14)

**Read time / review time: about 30–45 minutes total.** Everything you need is
in this directory. You are not expected to read any source code.

## What this tool is (one paragraph)

Release Intelligence (`ri`) is a command-line tool that answers one question
for a platform team: **"If I upgrade product X from version A to version B,
which of its changes actually matter for *my* deployment — and what can I
ignore?"** You tell it your environment (cluster version, Helm values,
manifests, installed CRDs, container images — as flags or by pointing it at a
configuration repository) and it joins that environment against the upstream
release's changes (release notes, chart default changes, CRD diffs,
compatibility constraints, published images). Every claim it makes carries
citations: an upstream source (URL + line + excerpt) and, where relevant, your
own input (file + line + excerpt). It puts every analyzed change into one of
five buckets — **ACTION REQUIRED** (your environment must change or something
breaks), **REVIEW REQUIRED** (credible overlap it cannot prove),
**INFORMATIONAL** (applies to you, you appear safe), **NOT AFFECTED** (checked,
does not apply), **UNKNOWN** (it could not tell you, and it says exactly what
evidence was missing). An optional AI step may attach *suggestions* to UNKNOWN
items ("consider reviewing this") with citations — it never changes a verdict.

## What is in this packet

| Path | What it is |
|---|---|
| `questionnaire.md` | the questions you answer (with per-finding adjudication tables) |
| `answer-sheet.template.md` | where you write your answers (copy it, fill it in) |
| `SCORING.md` | how we will turn answers into metrics (transparency, not your concern) |
| `reports/report-1-compat-action-required/` | cert-manager v1.17.0→v1.18.0, cluster **1.28** (below the supported range): contains the one ACTION REQUIRED |
| `reports/report-2-values-default-informational/` | the same product and upgrade, cluster **1.31**, values pin the changed default: the "applies to you, you appear safe" case |
| `reports/report-3-repo-mode/` | the same product and upgrade, environment **discovered from a config repository** (`example-env/customer-repo/`) |
| `reports/report-4-ai-enriched/` | report 1's case plus the optional AI step (`[suggests review]` blocks on UNKNOWN findings) |
| `reports/*/report.txt` | the rendered report exactly as a user sees it (start here) |
| `reports/*/report.json` | the same report as JSON, with every evidence record |
| `reports/*/run.md` | what the report covers and the exact command that produced it |
| `example-env/` | the environment inputs each report was joined against |
| `reproduce.sh` | re-runs all four reports from the repository, byte for byte (optional; needs the repo checkout) |
| `verification.md` | the packet's own QA record: every shipped citation fetched and checked (62/62 upstream, 25/25 environment) |
| `proxy-reviews/` | two AI-simulated reviews of this packet (labelled as proxies — they are NOT human validation) |
| `../time-study.md` | the methodology for measuring engineer-time saved, plus a labelled single-run pilot estimate |

## What to do (in order)

1. Skim `reports/report-1-compat-action-required/report.txt` cold — as if a
   colleague had handed it to you before a real cert-manager upgrade (~5 min).
   Environment inputs are in `example-env/report1-env/`.
2. Read `reports/report-2-.../report.txt` and
   `reports/report-3-.../report.txt` the same way (~5 min each). For report 3
   the environment is `example-env/customer-repo/` — a miniature config repo.
3. Read `reports/report-4-ai-enriched/report.txt`, paying attention to the
   `AI enrichments` section and the `AI suggests review-required` lines (~5 min).
4. **Spot-check 3 citations** anywhere in the reports: find an evidence id
   (like `ev-94a8abc4e147`) in `report.json` (field `evidence`), open its
   `uri`, go to the `locator` (a line number or JSON path), and compare the
   `excerpt` with what is actually there. Environment citations point at files
   in `example-env/` (paths in the reports are relative to the repository root
   of the packet; strip the `docs/phase3/review-packet/` prefix). Record which
   three you checked and whether they held up.
5. Answer `questionnaire.md` in your copy of `answer-sheet.template.md`
   (~10–15 min). There are no required long-form answers; one-line answers are
   fine, and "don't know" is a valid answer everywhere.

## Ground rules for you

- Judge the reports as a **user**, not as an implementation reviewer. If you
  can't tell whether something is right without reading code, that is itself a
  finding (record it under "evidence followable?").
- The environment inputs are miniature examples, so some findings may feel
  hypothetical — judge the reasoning and the classification, not the realism.
- Everything in the reports that says "unavailable", "UNKNOWN", "not in
  cache", or carries a warning is the tool being deliberately honest about
  gaps; the question is whether that honesty is at the right level, not
  whether gaps exist.

## How your answers are recorded

Fill in `answer-sheet.template.md` (it mirrors `questionnaire.md` one-to-one
and is machine-ingestible). Return the filled copy. `SCORING.md` documents how
each answer becomes a metric — you may read it, but you don't need to.
