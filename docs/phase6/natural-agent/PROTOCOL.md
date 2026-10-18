# Phase 6 · Phase 0 protocol — natural Claude Code vs the RI investigation agent (pre-registered)

Written by the commander (Claude Opus 5.5) on 2026-10-18, **before any natural-agent run**. `MISSION.md` holds the
product owner's plan and decisions. This file is never edited after the first scored run; amendments are appended,
dated, with the reason and the run that prompted them. The scoring rubric is `RUBRIC.md`, frozen in the same commit.

## 1. Question

> What does Claude Code successfully do during a real engineering investigation that Release Intelligence is not
> yet enabling its investigation agent to do?

Phase 0 answers this diagnostically on three known cases, before any architecture work, and ends in the decision
gate (§10). The product owner's specific suspicion is tested by an explicit contrast (§4, C-item): per-item sessions
may be what stops the agent from understanding how a CA, the certificates it issues and their consumers interact.

## 2. Cases (known, diagnostic, not blind)

| id | case dir | why |
|---|---|---|
| P-US | `eval/investigations/cert-manager-1.17-1.18--payments-us` | safe environment; RI could not clear it (Phase 5 S1) |
| P-EU | `eval/investigations/cert-manager-1.17-1.18--payments-eu` | same upgrade, affected; must be distinguished from P-US |
| D-EU | `eval/investigations-heldout/ingress-nginx-1.11-1.12--docs-eu` | the Phase 5 held-out false-safe (B3, strict path validation) |

The commander knows all three labels. P-US/P-EU are the Phase 5 development pair; D-EU was unsealed and its B0
transcript analysed by lane eval5. Results are diagnostic evidence about mechanisms, never accuracy estimates.

## 3. Evidence equivalence

For each case, the natural agent's workspace holds the **raw sources of everything the RI agent's tools could reach
for that case, and nothing else**:
- the environment repository (`env/`, the case's `input.yaml` root);
- an orientation file made **only** from `input.yaml`: product, from/to versions, chart, Kubernetes version and
  `environment.description`, verbatim. The `scope` list and the case id are left out;
- the upstream documents RI fetched for the transition (release notes, upgrade guides and other pages its evidence
  cites), as pinned local files;
- the FROM/TO renders RI produced for the environment, plus a plain diff of them.

Excluded: RI's derived outputs (change items, findings, classes, validations, claims, agent records, dossiers),
knowledge facts, `expected.yaml`, `NOTES.md`, `observed/`, transcripts, `docs/`, and this repository.

Before any run, lane `ws6` writes an **evidence-equivalence table** per case: each source the RI agent could reach,
whether the workspace has it, and its path and digest. The commander reviews it (ledger X-0002). If the RI agent
could reach a source that cannot be given as a file, that is recorded as a known difference, not silently dropped.
If a source would give the natural agent more than RI had (for example upstream source code), it is left out of
Phase 0 and noted as a candidate condition for later.

The model may know these releases from training. Arm B had the same model, so this is not a difference between the
arms. It is noted in the report.

## 4. Arms

| arm | investigator | runs |
|---|---|---|
| A | RI deterministic | stored, no new run: `A0-K0-20261017-edb21ed9` (P-US, P-EU), `A0-K0-20261017-b2380765` (D-EU) |
| B | RI structured agent, prompt v8, `claude-sonnet-5-5` | stored, no new run: `B0-K0-20261017-edb21ed9-v8` (P-US, P-EU), `B0-K0-20261017-b2380765` (D-EU) |
| C-whole | Claude Code, `claude-sonnet-5-5`, whole upgrade, natural prompt (§6) | 3 independent repeats per case |
| C-item | same workspace and model; the question is scoped to one upstream change (§6) | 3 independent repeats per case |
| D | product owner continues C-whole repeat 1 interactively (§7) | 1 per case, after C is recorded |

Arm E (formalisation) is not part of Phase 0. Network access is off in every arm.

**C-item scope**, fixed now:
- P-US and P-EU: the rotationPolicy change (release-notes-1.18.md L109–L148 section text);
- D-EU: the upstream change RI investigated as one item, "Enable security features by default (#11819)", whose
  section covers B1–B4. It is the item the Phase 5 false-safe came from.

The C-item prompt quotes the upstream section verbatim, so it carries no RI-derived wording.

## 5. Isolation (verified before any scored run)

- The workspace lives outside this repository: the fixtures are read-only (`chmod -R a-w`), and `scratch/` is the
  only writable directory.
- Claude Code runs with no user or project `CLAUDE.md`, no auto-memory, no MCP servers, no skills, plugins or hooks,
  and no access to `~/.claude` project data. How this is achieved, for example a clean config directory, is lane
  ws6's choice; the result is what is verified.
- Tools: Read, Grep, Glob, and Bash limited to read-only commands (`ls`, `cat`, `head`, `tail`, `wc`, `find`,
  `grep`, `diff`, `jq`, `yq`, `kubectl kustomize` offline). Write only into `scratch/`. WebFetch, WebSearch and Task
  are denied.
- **Canary run** (ledger X-0001):
  1. Decoy files are placed beside and above the workspace and under `$HOME`.
  2. A canary session is asked to read them and to report any instructions or memory it was given.
  3. It must fail to read the decoys, and its init event must list no memory files, MCP servers or plugins.

  The canary transcript is stored. A failed canary blocks every run.

## 6. Prompts (fixed now; values filled per case, nothing else)

**C-whole**: the natural prompt from MISSION §6, verbatim, with "the current version" / "the candidate version"
replaced by the case's product and versions, and a pointer to the workspace layout:

> We are considering upgrading `<product>` from `<from>` to `<to>`. This repository contains the Kubernetes
> environment that consumes it (`env/`; orientation in `ORIENTATION.md`; upstream documents in `upstream/`;
> before-and-after renders in `rendered/`). Review the upstream release notes, migration documentation, relevant
> configuration files, and before-and-after rendered differences. Determine whether this upgrade introduces breaking
> changes for this specific environment. Investigate relevant consumers and configuration relationships. Do not
> assume that a documented breaking change necessarily affects this environment, or that an unchanged manifest
> proves runtime compatibility. Explain what is affected, what is not affected, why, and what evidence supports your
> conclusions. Identify missing information or tests needed before an engineer could confidently proceed. You may
> inspect relevant files and use the available read-only tools. Do not modify the environment. Write your final
> assessment to `scratch/assessment.md`.

**C-item**: the same text, with "Determine whether this upgrade introduces breaking changes for this specific
environment" replaced by "Determine whether the following upstream change, part of this upgrade, breaks this
specific environment:", followed by the verbatim upstream section (§4).

## 7. Arm D (human-guided)

The product owner resumes C-whole repeat 1 of each case in an interactive session with the same isolation. The
commander only logs: every turn the product owner sends is recorded verbatim, then classified by the product owner
(not the commander) as one of: task clarification, pointer to existing evidence, newly supplied context, hint
toward a conclusion, correction of a mistake. The unaided score is the C-whole repeat-1 score; the guided score is
recorded separately and never counts as autonomous.

## 8. Capture and extraction

- **Per run:**
  - the stream-json transcript (every tool call and result);
  - the init event (model id, tools, cwd);
  - the workspace digest manifest;
  - tokens, cost, wall time and turns;
  - `scratch/assessment.md`.

  Stored under `runs/<case>/<arm>-r<n>/`. A run that hits the budget (§9) or errors is kept and reported, never
  silently retried.
- **Extraction:** a separate stateless `claude -p --model claude-opus-5-5` call reads only `assessment.md`, plus a
  fixed schema prompt, and outputs a standardised report:
  - per mechanism: claimed class, resources/consumers, mechanism, mitigation or no-exposure, cited evidence,
    assumptions, missing evidence, recommended validation;
  - an overall recommendation.

  It is label-blind and may only copy or paraphrase what the assessment says; "not stated" when absent. The
  original assessment is always kept beside it.

## 9. Run parameters

- **Model:** `claude-sonnet-5-5`, pinned.
- **Effort:** the CLI default, recorded in each run's metadata. No max-turn cap.
- **Budget:** 400k total tokens per run; a run that exceeds it is stopped and reported as such.
- **Repeats:** independent fresh sessions.
- **Order:** P-US, P-EU, D-EU, alternating C-whole and C-item repeats.
- **Total:** 18 natural runs plus 1 canary. Arms A and B are not re-run.

## 10. Scoring and decision gate

- **Scoring:** the commander scores every run (A and B from their stored records; C and D from assessment +
  extraction + transcript) against `RUBRIC.md`, quoting the assessment for each criterion. The commander knows the
  labels, which the rubric exists to constrain. The product owner audits the scores.
- **Per-mechanism outcomes:**
  - evidence-backed correct;
  - correct but unsupported;
  - appropriately uncertain;
  - hedged;
  - false-safe;
  - false-alarm;
  - missed.

  Each false-safe gets a root-cause analysis in the plan's categories: missing evidence, incorrect retrieval,
  incorrect semantic interpretation, incomplete consumer analysis, reasoning error, tool restriction, policy
  restriction, unsupported conclusion.

**Pre-registered reading rules** (n = 3 per condition: these are diagnostic thresholds, not statistics):
- **"Distinguishes the environments"**: in ≥ 2 of 3 C-whole repeats, P-US B1 is evidence-backed correct or
  appropriately uncertain, P-EU B1 is evidence-backed correct, and the assessment names the CA's effective setting as
  the difference.
- **"Avoids the D-EU false-safe"**: D-EU B3 is not false-safe in ≥ 2 of 3 C-whole repeats.
- **"Meaningful advantage over RI"**: C-whole meets both rules above, **and** C-whole has no more false-safe outcomes
  in total than B (B: 1 false-safe on these items, D-EU B3).
- **"Session scope matters"**: on the same case and mechanism, C-whole and C-item outcomes differ in the same direction
  in ≥ 2 of 3 repeat pairs. If C-item is as good as C-whole, the advantage comes from tools, context or format, not
  scope. If C-item is as poor as B, scope is implicated.
- A difference between C and B is **attributed to the model** only when the transcripts show the same evidence was
  read and used differently. A difference explained by evidence one arm read and the other did not is attributed to
  retrieval or tools.

**Gate report** (`GATE.md`): it answers the six questions of the Phase 0 decision gate in MISSION, with the tables
above and transcript excerpts. It recommends proceeding to the architecture plan only if "meaningful advantage"
holds. Otherwise it recommends the evidence, semantics or reasoning investigation the mission prescribes. Nothing in
RI is changed in Phase 0.

## 11. Disclosures recorded up front

- The commander knows the labels, and wrote this protocol and the rubric. The investigator never sees either.
- D-EU's arm B transcript was analysed (EVAL-REPORT §Held-out); P-US/P-EU arm B results shaped Phase 5's prompt v8.
- Arm B's evidence came through RI's tools over a bundle; arm C's comes as files. §3's table records any difference.
- n = 3 per condition on 3 cases: no reliability claims.

## Amendments
