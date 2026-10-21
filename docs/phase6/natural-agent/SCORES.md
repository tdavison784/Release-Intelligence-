# Phase 0 scores (X-0004): commander's scoring against RUBRIC.md, for the product owner's audit

Per-run detail, with verbatim quotes checked by `scripts/phase6/scores.py`, is in `runs/<case>/<arm>-r<n>/score.md`
(arms A and B: `runs/<case>/{A,B}/score.md`). Abbreviations: E = evidence-backed correct, CU = correct but
unsupported, AU = appropriately uncertain, H = hedged, FS = false-safe, n/a = outside the C-item scope.
**Two judgements were marked for audit**, because each sits on a category boundary. One of them decides a reading
rule. **Audit, 2026-10-20: the product owner confirmed both as H** (see "Product-owner audit" below).

## Outcomes per item

| case · item (expected) | A | B | C-whole r1 | r2 | r3 | C-item r1 | r2 | r3 |
|---|---|---|---|---|---|---|---|---|
| P-US B1 rotationPolicy (not-affected) | H | H | E | **H ¹** | E | E | E | E |
| P-US B2 revisionHistoryLimit (not-affected) | H | H | E | E | E | n/a | n/a | n/a |
| P-EU B1 rotationPolicy (affected) | H | CU | E | E | E | E | E | E |
| P-EU B2 revisionHistoryLimit (not-affected) | H | H | E | E | E | n/a | n/a | n/a |
| D-EU B1 annotations-risk-level (not-affected) | H | E | E | E | E | E | E | E |
| D-EU B2 cross-namespace (not-affected) | H | E | E | E | E | E | E | E |
| D-EU B3 strict path validation (**affected**) | H | **FS** | **FS** | **H ²** | H | **FS** | AU | H |
| D-EU B4 annotation validation (not-affected) | H | CU | E | CU | E | E | E | E |

¹ P-US C-whole r2: the headline says "one breaking change that applies … latent". Its own consumer analysis shows the
workloads trust the CA only. Scored H, not AU.
² D-EU C-whole r2: "Probably not; needs a dry-run." By RUBRIC's wording (FS = says not affected, "safe" or "no
action") it is H, since it requires an action. **This judgement decides the "avoids the D-EU false-safe" rule.**

False-safes in total: A 0 · B 1 · C-whole 1 · C-item 1. False alarms: none in any arm.

## Pre-registered reading rules (PROTOCOL §10)

| rule | result | basis |
|---|---|---|
| Distinguishes the environments | **holds** (2/3) | repeats 1 and 3: P-US B1 E, P-EU B1 E, and both name the CA's effective rotationPolicy as decisive. Repeat 2 fails on P-US (H ¹). |
| Avoids the D-EU false-safe | **holds narrowly** (2/3) | r1 FS, r2 H ², r3 H. If ² is read as FS, it fails (1/3). |
| Meaningful advantage over RI | **holds, conditional on ²** | both rules above hold, and C-whole false-safes (1) ≤ B's (1). |
| Session scope matters | **does not hold** | No mechanism differs in ≥ 2 of 3 repeat pairs. Of 18 comparable pairs (6 mechanisms × 3), 3 differ: P-US B1 r2, D-EU B3 r2 and D-EU B4 r2. In all three, C-item is the better one, the opposite of the per-item hypothesis. |

## What the transcripts attribute the differences to (§10: model only when the same evidence is read differently)

1. **cert-manager: C over B is retrieval and effort allocation, not the model and not session scope.**
   - Arm B made 21–23 tool calls across about 18 change items. It never opened `apps/payments/base/*-deployment.yaml`
     or `ledger.conf`, both of which were bundle artifacts. It said so itself: "I did not trace what consumes the two
     certificates' Secrets (those Secrets are absent from the snapshot)".
   - Every C run, C-item included, read those files and followed the Certificate → Secret → mounted `ca.crt` →
     nginx / curl trust chain.
   - C-item, scoped to one change like a B item, did as well as C-whole. So the decisive factor is how much
     reading a single change gets, not whether the session sees the whole upgrade.
2. **D-EU B3: no arm is reliably right, and the cause is evidence, not architecture.**
   - The workspace and RI's bundle hold only "`strict-validate-path-type` gets enabled by default". Nothing states
     the accepted character set.
   - Five of six C runs and arm B filled it in from model knowledge, and wrongly ("." allowed). The one AU run
     (C-item r2) also took the set from memory, but stated it correctly and kept it open.
   - The C runs that avoided FS did so by requiring a dry-run, not by knowing the answer. Root cause for every
     FS: missing evidence + incorrect semantic interpretation (model knowledge).
3. **Arm A** hedges everything (all potentially-affected) and has no false-safe.

## Harness notes affecting interpretation
- H1 (P-US C-item r3): assessment recovered verbatim from the session's denied Write; scored normally.
- Every Bash `for` loop and `cd ..` was denied by the read-only allow list (1–2 per run). Each agent fell back to
  `Read`/`Grep`. This cost turns, not evidence.
- D-EU's ORIENTATION mentions `/.well-known` (A1 §4). Arm C still mis-stated the rule in 5 of 6 runs, so the hint did
  not decide B3.

## Supplementary: C-raw (PROTOCOL A1 §2; enters no reading rule)

P-US, C-whole prompt, environment files and renders as authored (no redaction markers).

| P-US B1 | r1 | r2 | r3 |
|---|---|---|---|
| C-raw | E | E | CU |
| C-whole (read-back) | E | H | E |

- r1: "platform/pki/overlays/prod/mesh-root-ca-prod.yaml sets privateKey.rotationPolicy: Never explicitly"; "Nothing in
  the repo does this" (leaf pinning).
- r2: "The root CA is protected"; "Neither ledger.conf, the Deployments nor the Services contain any key or fingerprint
  pinning."
- r3 (CU): the CA's Never and the ca.crt trust are stated, but there is no statement that nothing pins a leaf key.
  The leaf consequence is framed as a cert/key swap race.

Reading: no visible effect of the markers at n = 3, consistent with RI's read-back having given the values all
along. Redaction is not a Phase 0 cause.

## Product-owner finding: the candidate version is a security regression (outside the rubric)

D-EU's candidate, 1.12.0, predates 1.11.5. Five GitHub advisories, among them GHSA-mgvx-rpfc-9mpv (CVE-2025-1974,
critical, admission webhook RCE), affect `< 1.11.5 || >= 1.12.0-beta.0, < 1.12.1`. The upgrade as framed would
re-expose a webhook that this environment enables. The answer engineers expect: change the target to 1.12.1 (chart
4.12.1). The product owner rated this the main miss, ahead of the B3 scoring.

- RUBRIC lists only the four parts of #11819, so no arm is scored on this.
- C-whole r1 and r2 called the upgrade a security regression. r2 led with weak evidence: an entry missing from a
  changelog. Its target was "a later 1.12.x or 1.13/1.14", not a named version.
- No C-item run and not the RI agent (arm B) raised it.
- RI root cause and fix: LEDGER X-0009. RI already has rule `advisory:affects-target`, but it ingests only
  repository-level advisories. There are none for kubernetes/ingress-nginx; the advisories sit in GitHub's global
  database.

## Product-owner audit (2026-10-20)

- ¹ P-US C-whole r2: **H confirmed.**
- ² D-EU C-whole r2: **H confirmed.** "Avoids the D-EU false-safe" therefore stands at 2/3 (holds narrowly), and
  "meaningful advantage" holds.
- The product owner's reading of both: the same failure, "just not enough proper wording around them". In each run the
  verdict line did not match its own evidence and caveats. P-US r2 headlined "breaking change that applies" over an
  analysis that clears the environment. D-EU r2 led with "Probably not" over an unverified belief and a required
  dry-run. Gap to carry forward: a verdict must state its own confidence and precondition in the headline.
