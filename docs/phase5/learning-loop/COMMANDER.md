# Phase 5 commander state (learning-loop fleet)

Live state; successor resumes from here (see `briefs/commander.md`). Goal: `MISSION.md` (scope note first).
Experiment: `PROTOCOL.md` (pre-registered 2026-10-17, never edited; amendments appended).

## Plan

- **Wave 1 (parallel):** pins, coverage, verdict, verify (Opus); eval5 (Sonnet: snapshots, scoring, A0 baseline);
  groundtruth5 (Opus, sealed branch — never merged before freeze F1).
- **Wave 2 (experiment, PROTOCOL §6 steps 1–6):** freeze F1 → A0 → B0 → export + verify hypotheses → product owner
  promotes confirmed ones in the review UI → K1 → C1 + Kbad-*.
- **Wave 3:** unseal groundtruth5 → held-out A0/B0/C1 → report (EVAL-REPORT.md, SUMMARY.md) → push to PR.

## Lanes

| Lane | Agent | Model | Branch | Worktree | Tab | State |
|---|---|---|---|---|---|---|
| pins | ri-pins | Opus | p5ll/pins | (removed) | (closed) | merged cbb192c8, cleaned up |
| coverage | ri-coverage | Opus | p5ll/coverage | (removed) | (closed) | merged ef6b8d04, cleaned up |
| verdict | ri-verdict | Opus | p5ll/verdict | (removed) | (closed) | merged, cleaned up |
| fixes5 | ri-fixes5 | Opus | p5ll/fixes5 | .claude/worktrees/p5ll-fixes5 | w1:t1W | working (W-0024, W-0025) |
| verify | ri-verify | Opus | p5ll/verify | (removed) | (closed) | merged, cleaned up |
| groundtruth5 | ri-groundtruth5 | Opus | p5ll/groundtruth5 (SEALED) | .claude/worktrees/p5ll-groundtruth5 | w1:t1T | working |
| eval5 | ri-eval5 | Sonnet | p5ll/eval5 | .claude/worktrees/p5ll-eval5 | w1:t1V | wave 1 merged; idle, kept for waves 2–3 |

## Merge log

- 2026-10-17 verdict merged (merge base 578c1a9b): item dossier (deterministic, ≤48 KiB, stored, `ri investigate dossier`), AgentVerdicts on every v6 closing entry (classify/validate never reference them — checked), MitigationSpec on record_hypothesis (shape-checked at record time), `ri investigate export-hypotheses` (proposals + review items, never facts), prompt v6 default; replaced verify's and eval5's local views with real types (eval5 stub would have dropped B0 verdicts). Live smoke (shop-de, sonnet, 137k tokens, 10 sessions): agent potentially-affected on the decisive #11819 item with the right mechanism, not-affected on 9 minor items; 0 mitigations; replay byte-identical. Not tuned. Green. Cleaned up. Follow-ups → lane fixes5 (W-0024, W-0025).

- 2026-10-17 verify merged: `ri verify hypotheses -knowledge DIR [-runtime]` → ProofRecords beside the review item; deterministic citation check (store/cache, quoted passage, versions inside bounds); synthetic both-arm fixture (`products/exercises/fixtures/`), only "condition arm unchanged + control arm changed" confirms; promotion gate on every decision path (human, whole fact, confirmed proof for the exact rule digest, any refutation permanent, bounds (from,to] cover the release, supersedes/coexists explicit); review UI hypothesis page; consensus routing ignores hypotheses. Reviewed promote.go. Green. Cleaned up. **Disclosure for the report:** the lane's smoke run (runs/verify-smoke.md) verified a hand-authored `rotationPolicy: Never` rule (confirmed 207 s) and a wrong one (refuted 194 s) in a scratch store — the mechanism was exercised on the experiment's subject before the experiment; not experiment data. Gaps: only subject-object conditions synthesise (controller-flag / other-object rules ⇒ not-run); unversioned doc URLs fail the citation check.

- 2026-10-17 cbb192c8 pins: scoped applicability (in/out/undecidable; only provably-out objects excluded; value predicates on unset fields need a CRD default or an explicit unset/set leaf in the rule — commander fail-open finding fixed in 79581777 with mutation check), effective object view through kustomize renders (base+patch = one object), release-bounds/contradiction/lifecycle tests, S3 a–h synthetic controls; Phase 4 fail-open fixed (default-change pin ignoring applicability). validate.override-inspection@v2; 152 stored investigations still validate. Green. Cleaned up. Open: reverse-reference leaves not expressible in the fact model (not needed for the dev pair).

- 2026-10-17 ef6b8d04 coverage: `namespaceFrom` on relationship rules (controller flag + documented default + citations; undecidable ⇒ edge stays inferred), cert-manager ClusterIssuer CA-secret rule declares --cluster-resource-namespace; validate.render-diff@v2 documentation-only pass; classify.policy@v2 counts it for R2.2 only (v1 still honoured for stored runs). Reviewed policy diff: doc-only excluded from counts(), re-checked against bundle. Green. Cleaned up (tab w1:t1Q, worktree, branch -d). Follow-ups: DESIGN §2.5/§2.6/§3.1 R2.4 wording proposed in its status (commander applies); stored run C-20261016T234455Z-aa06720656 re-classifies 6/8 at base too (pre-existing; W-0022).

- 2026-10-17 e5b3afa7 eval5 wave 1: knowledgeDigest/knowledgeLive in run.json, `-knowledge` mandatory (live tree refused unless -allow-live-knowledge), snapshot.sh + kbad (S3 a–h), ungated agent-verdict table (correct/false-safe/false-alarm/hedged/overstated/no-verdict; stub until verdict merges), `-compare` tables, A0-K0-20261017-bfebde39 (K0 digest sha256:672a7ce0…; 24/24 item decisions identical to Phase 4 arm A; payments-us B1 potentially-affected). Green. Worktree kept. Note: lane deleted one uncommitted earlier A0 run from a pre-commit binary (never cited).

## Decisions

- 2026-10-17 Phase 5 branches from `p4-env-impact` @ 03b6ff43 (PR #6 open, not merged into main): Phase 5 needs
  Phase 4 code + the product owner's verified knowledge.
- 2026-10-17 The payments pair is the **development** pair: the gap analysis (and so the commander) saw its labels and
  its environment differences (leaf Certificates rotate in both; only the CA's explicit setting and its consumers
  differ). Held-out ground truth is the measurement; lane briefs and prompts carry no case-shaped detail.
- 2026-10-17 No new subsystem: the rule is a normal knowledge fact (proposal → whole-fact review → human decision with a
  confirmed proof record); applicability is decided by the existing pin validator; agent verdicts are stored beside,
  never inside, policy inputs.
- 2026-10-17 Promotion is the product owner's click (human level), gated on a confirmed proof record.

- 2026-10-17 PR #6 merged into main by the product owner (merge commit bebbeb2c, history preserved: this branch's
  base 03b6ff43 is in main). Window ledger ADOPTED (product owner) → `LEDGER.yaml`; guard v4 queued (W-0008).

## Open questions for the user

- W-0019 delete superseded branch `p4ei/eval` (needs -D). W-0020 gascity/beads for the next fleet (after the report).

## Next actions

The queue is `LEDGER.yaml` (ready items, their depends and who may execute them). Guard v4 (W-0008) in
progress with ri-guard; until it merges, handoffs use guard v3 and the GLM commander reads LEDGER.yaml by hand.
