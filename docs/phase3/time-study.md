# Engineer-time-saved methodology (G14) — and a single-run pilot

Status: **methodology plus a PROXY ESTIMATE.** The pilot below is n=1,
performed by an AI agent, and contaminated in ways §3.3 spells out. It
validates the protocol (is the scenario runnable, are the artifacts
producible, does the recording sheet capture the right splits). It **does
not** estimate human time savings — no number in §3 should be quoted as one.

## 1. The two workflows being compared

**Traditional (the control):**

1. Read the release notes for the target version.
2. Read the upgrade guide (old → new).
3. Diff the Helm chart values (old → new) and reconcile against your values files.
4. Diff the CRDs (old → new).
5. Check the compatibility matrix against your cluster (and OpenShift, if any).
6. Check which images the release publishes and what you pin/mirror.
7. Search issue trackers for regressions affecting your components.
8. Walk your local config (values, manifests, GitOps apps, CI) for each touchpoint.
9. Write the upgrade plan.

**RI-assisted (the treatment):**

1. Run `ri impact <product> <from> <to>` against your environment (flags or `--repo`).
2. Read the funnel and the actionable findings (ACTION REQUIRED / REVIEW
   REQUIRED / INFORMATIONAL / NOT AFFECTED).
3. Inspect the cited evidence for findings you will act on (URL + line + excerpt).
4. Investigate the UNKNOWN list — it is mandatory reading; each row names the
   missing evidence, and (with `-enrich`) some rows carry AI review suggestions.
5. Write the upgrade plan.

The claim under test is not "RI finds things humans cannot" — it is that steps
1–4 and 8 of the traditional workflow are **performed, cited and
machine-checkable** by the tool, so human time concentrates on judgment
(steps 3–5 of the RI workflow).

## 2. Measurement protocol (for real engineers)

**Design.** Within-subject, counterbalanced. Each participant does **both**
arms of the same scenario (one traditional, one RI-assisted), with order
alternated across participants; a second scenario (different product, same
shape) is available as a washout task between arms. Within-subject because
between-subject n would need to be an order of magnitude larger to beat the
noise.

**Task.** "You are upgrading cert-manager v1.17.0 → v1.18.0 for the
environment in this repository (cluster is k8s 1.28). Produce the upgrade
plan you would open a change ticket from. Note everything you checked and
what you decided." The environment is a fixture of the same shape as
`docs/phase3/review-packet/example-env/customer-repo/` (values files,
manifests, a Certificate, image pins in CI, GitOps apps) — realistic enough
to exercise every traditional step, small enough to fit a session.

**Time accounting.** A wall-clock timer per arm, plus a self-marked split at
each transition:

| Split | Counts as | Notes |
|---|---|---|
| Source gathering (traditional) / run + funnel read (RI) | research | for RI this is minutes: locating the binary, assembling the command |
| Reading sources / reading findings | research | traditional: notes, upgrade guide, diffs; RI: findings + evidence inspection |
| Verifying a claim against its source | research | traditional: re-reading; RI: spot-checking citations (cap it: 3 citations) |
| Investigating unknowns | research | RI only, but its honest equivalent exists in the traditional arm ("what did the notes NOT tell me") |
| Writing the plan | planning | same deliverable both arms, same rubric |

Research vs planning time is reported separately because the tool targets
research time only; a tool that saves research but adds planning is a wash.

**Artifacts collected per arm:** the plan; the participant's checked-claims
list; the timer log with splits; post-arm 3-question debrief (confident?
missed-anything suspicion? would you ship this plan?).

**Plan quality rubric (scored blind by one author + one non-author):** per
category — compatibility, breaking API defaults, chart values, images,
install path, CRDs, security, observability — 2 (correct + evidenced),
1 (mentioned), 0 (absent), −1 (wrong). The categories were fixed from the
traditional workflow before any pilot ran. Coverage deltas between arms are
reported per category, not as a single score.

**Threats to validity, pre-registered:** learning effect (mitigated by
counterbalancing + washout); fixture familiarity (participants get 10 minutes
with the environment before timing starts, both arms); tool fluency
(participants get a 10-minute RI tutorial before their RI arm, excluded from
timed time); scenario fit (cert-manager is deliberately chart-heavy; a
CRD-heavy product is the second scenario).

## 3. Pilot (PROXY ESTIMATE — n=1, AI-simulated, contaminated)

### 3.1 What was run

- **Scenario:** cert-manager v1.17.0 → v1.18.0, environment =
  `docs/phase3/review-packet/example-env/customer-repo/`, cluster k8s 1.28.
- **Traditional arm:** the pilot operator (an AI agent) fetched and read the
  live upstream sources by hand — release notes, upgrade guide, supported-
  releases table, chart values diff, CRD diff — plus every file of the
  environment fixture, then wrote `pilot/traditional-plan.md`.
- **RI arm:** ran the tool offline (`ri impact cert-manager v1.17.0 v1.18.0
  --repo …/customer-repo --kubernetes 1.28`, wall clock < 1 s), read the
  rendered report, verified three citations, scanned the UNKNOWN list, wrote
  `pilot/ri-plan.md`.

### 3.2 Numbers (wall clock, AI operator)

| Arm | Source gathering / run | Reading + verifying | Plan writing | Total |
|---|---|---|---|---|
| Traditional | 61 s (7 fetches, ~120 KB of notes + 2 MB of CRD YAML) | ~67 s | 35 s | **163 s** |
| RI-assisted | < 1 s (tool run) | ~30 s (report + 3 citations + UNKNOWN scan) | ~25 s | **~55 s** |

**Do not divide these numbers by anything or extrapolate them.** An LLM reads
120 KB of release notes in seconds; a human cannot. The ratio (≈3×) says
nothing about the human ratio, which §2 exists to measure.

### 3.3 Contamination (why these numbers are protocol-validation only)

1. **Same operator both arms** — the RI arm benefited from everything the
   traditional arm taught (the operator knew to look for `rotationPolicy`).
2. **Operator wrote the packet** — the RI arm's tool output was already
   familiar; a first-time user pays a learning cost this pilot never paid.
3. **AI reading speed** — dominant distortion; see §3.2.
4. **Order effect** — traditional ran first by circumstance, not
   counterbalance.

### 3.4 What the pilot IS good for: the plans differ in instructive ways

Side by side (`pilot/traditional-plan.md` vs `pilot/ri-plan.md`):

| Category | Traditional found | RI-assisted found | Delta |
|---|---|---|---|
| Compatibility | 1.28 below 1.29–1.33; **and noticed v1.17.0 has the same range** (both rows of the table are visible when you read it by hand) | 1.28 below range (deterministic, cited) | RI **missed** the "already unsupported today" nuance — the tool compares target only |
| Breaking API defaults (`rotationPolicy`, `revisionHistoryLimit`) | Both, from the upgrade guide, with the feature-gate deadline | Both, but **only because the operator scanned the UNKNOWN list**; the tool classified them UNKNOWN (no machine-comparable subject) and the AI layer did not reach them at `-enrich-max 20` | RI's one true failure mode: the funnel alone hides the biggest breaking changes |
| Chart values | Both chart-diff items (targetPort flip; new `disableHTTPChallengesRole`) — and noticed **our values set the new key true**, which goes live | Same two items; the live-key risk (`imp-0dae408f0c17`) was a **deterministic, cited finding** | RI strictly better here: the traditional operator only caught the now-live key because they went looking; a skimming human could miss it in a 100-line diff |
| Images | All four pin sites (hand-grep) | Both image findings, with file citations | Equal coverage; RI faster and cited |
| Install path (3 tools declare cert-manager; Flux `1.*` range) | Found by walking the repo | Found via repo-mode warnings + inventory | Equal; RI's warnings carry it |
| CRDs | Additive-only, from the diff | Additive-only, from the report's CRD evidence | Equal |
| Security (5 CVE bumps) | All five, from the bugfix section | All five, via the AI suggestions in R4 (deterministic run leaves them UNKNOWN) | RI's coverage depends on the optional AI layer — the proxy reviews' shared escalation |
| Observability | New metrics found | New metrics found (AI-suggested) | Equal |

Two structural findings for the tool, both already filed by the proxy
reviews: (1) security must leave the optional AI layer; (2) declared-change
breaking defaults need a deterministic join against inventoried manifest
fields (`rotationPolicy` absent from a Certificate is decidable). Neither
blocks the packet; both would raise plan quality measurably.

### 3.5 Recording sheet (validated by the pilot)

```
participant:        scenario:            arm order:
arm start:          arm end:             total:
  research:  gather/run ____  read ____  verify ____  unknowns ____
  planning:  write ____
claims checked (list):            citations spot-checked (RI arm):
plan artifacts:                   debrief (3 answers):
```

## 4. Follow-up protocol with real engineers

- **Recruitment:** 4–8 participants, ≥3 years Kubernetes operations
  experience, currently responsible for at least one cert-manager (or
  equivalent CRD-controller) deployment; mix of platform-engineer and SRE
  roles; no authors of this repository.
- **n justification:** within-subject design; 4 participants give the
  paired-difference estimate ±~40% at this variance; 8 if schedules allow.
  This is a direction-finding study, not a powered trial.
- **Session (90 min):** 10' environment familiarization → 10' RI tutorial
  (only if RI arm is theirs) → timed arm A → 10' washout → timed arm B →
  15' debrief.
- **Compensation/consent:** sessions are consented; participants are told the
  tool is a prototype, that the fixture environment is synthetic, that
  timing failures are data not faults, and that raw notes are shared with
  them before any internal writeup quotes them (questionnaire H2 governs
  quoting).
- **Success criteria for the study** (pre-registered): research-time
  paired difference per participant with sign consistency across ≥ 3 of n;
  no arm-B plan quality regression > 1 category point vs arm-A; at least one
  RI arm per participant where a citation changed a decision.
