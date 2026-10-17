# Status — lane `eval` (Claude Opus 5.5, agent ri-eval)

Branch `p4ei/eval`. Brief: `briefs/eval.md` (+ `_wave2-common.md`, `_wave1-common.md`, FLEET.md).
Sequencing from the commander: harness + arm A now; arm B waits for agent prompt v4 (commander says when
merged — never score B with v3); arm C waits for lane runtime.

## Done
- Harness `ri eval -investigations [-case ids] [-save] [-arm X] [-run-id X] [-work DIR] [-rescore <run dir>]
  [-agent -agent-driver … -agent-model … -agent-replay-dir <run dir>] [-runtime] [-o text|json]`:
  - runs the real in-process pipeline (`run.Runner`, same wiring as `ri investigate` via `cli.pipelineAt`)
    on every case's `input.yaml` + `env/` only; the investigation store goes to `-work` (default a temp dir,
    removed) — never the shared `.ri`;
  - matcher (FORMAT.md `match`: any alternative; subject = substring of an in-scope item's subjects, text =
    RE2 over title + the edge change's detail); several matches → strongest class, all listed; none →
    insufficient-evidence + `itemMatchMisses`;
  - every DESIGN §6.2 metric + per basis / manifestation / expected-verification splits; gates.yaml
    evaluation: PASS / FAIL / INSUFFICIENT-DATA (denominator < minSample, or 0/0) — non-zero exit on any
    non-PASS; autonomy criteria (§6.3);
  - reproducibility: the stored records are re-classified with `classify.New()` and compared byte for byte
    (findings + decision); `-rescore` does it again offline from the stored JSON files;
  - `-save` stores `eval/investigations/results/<run-id>/` (`run.json` with commit/dirty/config/input,
    env and expected digests per case; `score.json`; `gates.json`; `report.txt`; per-case `{request,investigation,
    bundle,graph,impact-report,changes}.json` (+ `transcript.jsonl` with an agent) in one deterministic
    `cases.tar.gz`); refuses to overwrite.
- Tests (`internal/eval/investigations_test.go`, synthetic fixtures only): loader + malformed-case rejection,
  matching (subject, detail text, out-of-scope ignored, strongest class, miss), component reach (rendered
  change incl. release-namespace objects, path end), required evidence (local-file URI → `env/` path,
  object, field), citation resolution, run failure, every decision metric, gate states incl.
  INSUFFICIENT-DATA, autonomy, splits, report text.

- Scored run **A** stored: `eval/investigations/results/A-20261016T201359Z-294a49e7a9` (binary 294a49e7a9, clean
  tree) + replicate `A-20261016T201443Z-294a49e7a9` (same bundles/investigations/scores). `-rescore` of the stored
  run reproduces score.json exactly. Arm A gates: 4 PASS (falsePositiveRate, citationResolution, reproducibility,
  pipelineFailures), 7 FAIL (affectedDetection 1/3, affectedConfirmation 0/3, notAffectedConfirmation 0/20,
  exposureRecall 2/7, evidenceCoverage 6/71, unresolvedRate 15/23, observedAgreement 0/21), 2 INSUFFICIENT-DATA
  (falseNegativeRate 0/3 < minSample 4, unsafeEligibilityRate 0/0).
- `docs/phase4/env-impact/EVAL-REPORT.md` written for arm A (per-case table, gates per arm, splits, FN/FP root
  causes, autonomy). Harness now also reports unscored findings / unscored affected findings per run.
- Docs: `docs/INVESTIGATE.md` "Evaluation", README command line.

## Commander review (2026-10-16)
- Arm A ACCEPTED. **HOLD arm B** until an explicit "ARM B GO" message: agent prompt v5 (structured verified
  claims) is in progress (B is scored with whatever prompt the GO names — not v3/v4 unless told), and B needs
  ~1M claude-sonnet-5-5 tokens (Claude 5h window at 88 %, resets 17:30).
- Routed defects 1–3 (layered-values warning → partial, ImpactReport retrievedAt digests, CRD-description
  attribution + depth-8 truncation) → new lane `fixes`. ingress-nginx one-note-three-items = recorded known
  limitation. shop-nl B1 FP stays (ACTION REQUIRED contract).
- When `fixes` merges, a fresh arm A run (new run id; arm A results above are kept) is the natural re-measure —
  ask the commander before running it.
- Fleet guard HANDBACK / RESUME messages: follow them.

## In progress / next
0. (2026-10-16 after RESUME) **Arm B still HELD** — COMMANDER.md: `ARM B GO` text is unsubmitted in this
   pane's input box pending the Claude commander's decision; not acted on. Also open there: `.ri` cache-pinning
   decision affecting stored runs' replayability.
   **Arm B still HELD** — no "ARM B GO" received; the GLM commander also ruled arm B never runs in a GLM
   window (model under test + claude-sonnet quota). GLM continuation does not run it. On ARM B GO (Claude
   window; agent prompt v5 merged): `git merge p4-env-impact`, rebuild, run
   `./bin/ri -state <scratch state> -offline eval -investigations -agent -agent-driver claude-cli
   -agent-model claude-sonnet-5-5 -save`, verify replay with `-agent-driver replay -agent-replay-dir
   <B run dir>` (v5 per-item sessions still write ONE combined transcript artifact per case —
   `investigator.go:275-286` — replay path unchanged), fill the report's B columns, cleanup, LANE DONE.

All deterministic work is done; the lane idles awaiting ARM B GO or HANDBACK.

## GLM handoff log (2026-10-16, glm-eval window 2)

Continued the lane across two usage-limit windows. Commits f0736ae0..4e05c478 (all `[glm-handoff]`).

**Done this window:**
- Merged `p4-env-impact` into the lane (sanctioned; commander authority recorded in COMMANDER.md).
- Wired the runtime validator into the real pipeline (first wiring beyond the Go-test harness):
  `-runtime` opt-in flag, validator construction in `pipelineAt`, tool preflight, arm labels, docs,
  wiring test (13f9ba74).
- Found + minimally fixed cross-lane defect (h) (cf761e89): 55ac1cfa's checkout-independent snapshots
  broke `AppRenderer`'s re-render; fixed with an exported `Repo` override; verified with a real
  payments-eu control run before scoring. Two files outside lane ownership, both minimal and additive:
  `internal/investigate/runtime/apprender.go`, `groundtruth_test.go`.
- Scored arm A′ (post-`fixes` re-measure, clean tree): `A-20261016T213500Z-cf761e89e5` — EVAL-REPORT §2a
  (deltas, root causes, unsafeEligibility 1/8 realisation of the §5.1a caution, exposureRecall path-end
  question, cache-drift confound).
- Scored arm C (real run): `C-20261016T213533Z-cf761e89e5` — EVAL-REPORT §2b. Headline: 7/8 cases ran
  real isolated upgrades (~2 min each, clusters deleted, 0 pipeline failures); shop-pl excluded *by
  design* (`complete: false`, separate values repo; validator fail-closed refuses incomplete renders —
  correct); 3 `runtime:degraded` fails in obs-north flipped their findings potentially→affected (R1,
  basis observed) — **but no decision or gate moved, because the expected-item matcher maps obs-north B1
  to the release-note item and matches the three runtime-affected chart items to nothing.** Routed to the
  commander (groundtruth match-width or scorer sibling-items — frozen for this lane). Also routed:
  payments-eu rotationPolicy fail lost under current code (paths no longer reach the degraded Secrets);
  `runtime:unreached` errors (payments-us ×7, shop-de ×13) record as error and clear nothing — no
  observed *clearing* exists yet, so notAffectedConfirmation cannot move via runtime today.
- Kept-but-superseded runs (never edited/deleted): `A-20261016T212406Z-13f9ba749d` (dirty-tree A′ copy,
  byte-identical score), `C-20261016T212613Z-13f9ba749d` (pre-fix vacuous arm C).

**Uncertainties / for the resuming Claude agent:**
- The shared `.ri` cache drifted between arm A and A′ (other lanes write it); §2a states the confound.
- unsafeEligibilityRate 1/8 is real and expected-shaped (conservative any-matched-finding counting on
  obs-north B1); the report explains why the end-to-end approval path is still safe (§2a).
- exposureRecall's drop to 1/7 is a metric-definition consequence (path-end vs on-path at depth 12) — a
  DESIGN question, flagged in §2a, deliberately not changed here.
- Cleanup done: `.glm-scratch/`, `bin/`, /tmp scratch removed; `kind get clusters` empty; the shared
  `.ri` cache was never touched beyond reads + the validator's own artifact writes.

## Arm A″ (2026-10-16, after merging p4-env-impact dec9342c)
- Merged `p4-env-impact` (fixes2, values, runtime follow-up) at 5d4da376; build/vet/test green.
- Scored `A-20261016T231142Z-5d4da37635` + replicate `A-20261016T231217Z-5d4da37635` (identical scores).
  Gates 7 PASS / 5 FAIL / 1 INSUFFICIENT-DATA: PASS affectedDetection 3/3, exposureRecall 6/7, unresolvedRate
  0/23, falsePositiveRate 1/20, citationResolution, reproducibility 8/8, pipelineFailures 0; FAIL
  unsafeEligibilityRate 1/8 (obs-north B1 — the same four ci-0f84…/ci-1d8a…/ci-63eb…/ci-f835… findings still
  eligible not-affected), affectedConfirmation 0/3, notAffectedConfirmation 2/20, evidenceCoverage 23/71,
  observedAgreement 0/21; INSUFFICIENT-DATA falseNegativeRate 0/3.
- Record digests: canonical `run.ContentDigest` identical 8/8 between the two runs (bundles and identified
  ImpactReport artifacts byte-identical). The harness's private digest (stored in those two run.json files)
  predated `reportStamps` and differed; harness now uses `run.ContentDigest`. Stored files not edited.
- EVAL-REPORT §2c written. Arm B HELD (until ARM B GO); arm C waits for the runtime attribution-precedence
  follow-up.

## ARM B — GO received 2026-10-17 (commander)

**ARM B is claude-only (model under test claude-sonnet-5-5). A GLM continuation must NOT run, resume or re-run any arm B case - its claude -p calls would hit a different model.**

- Reference: A₅ tree (0790b001; this branch adds only docs/results on top). claude-cli driver, claude-sonnet-5-5,
  newest merged prompt (version + digest recorded per case in run.json). One run per case, case by case, commit
  after each case. If the fleet guard pauses this lane (Esc) or a HANDBACK/handoff happens: STOP arm B.
- Progress (cases done): see `eval/investigations/results/B-*/run.json` `cases`.
- **ARM B STOPPED after case 1 (2026-10-17) — pipeline defect, routed to the commander.** Run
  `B-20261017-355df68274` (binary 355df682, clean; prompt investigate-agent/v5, claude-sonnet-5-5): case
  payments-eu → pipeline failure after the agent ran (158 s): `investigate (permanent): store: invalid
  investigation: audit: event 19 goes back in time`. Cause (read-only analysis, not fixed here — run package):
  `internal/investigate/run/pipeline.go` `Pipeline.Run`, `case hasRecord:` — `done := ev("completed", …)` takes its
  `At` (`p.now()`) BEFORE `res.apply` runs; the investigate stage's apply appends one `requested` audit event per
  agent evidence/validation request at a later `p.now()`; `done` is appended after them with the earlier time ⇒
  `Investigation.Validate` rejects the record. Any agent run with ≥1 request fails; scripted runs without
  requests never hit it. Fix: stamp `done` when it is appended (after apply). The run is kept (1 case, error).
  The case's records/transcript lived in the temporary work store and are gone; next arm B attempt uses `-work`
  inside the scratchpad so a failed case keeps its transcript. Arm B restarts as a NEW run after the fix (new
  binary; -append refuses a different commit).
- Commander: stop was correct; lane `auditfix` is fixing it. **WAIT for the explicit "ARM B RESTART" message**
  (auto-suggestions in the input box are not it). On restart: merge `p4-env-impact`, rebuild clean, new run id,
  `-work <scratchpad>/bwork`, case 1 with `-save`, cases 2–8 with `-append`, commit after each case.

## Arms A₅ + C₅ after lane refcheck (2026-10-17) — current reference for arm B
- Merged `p4-env-impact` (fc0fee09 refcheck) at 0790b001; green. `A-20261017T003437Z-0790b00157`,
  `C-20261017T003511Z-0790b00157`: both 7 PASS / 5 FAIL / 1 INSUFFICIENT-DATA.
- obs-south B1 FP gone (FP 1/20); obs-north B1 affected on a concrete dangling Secret reference (A: deterministic,
  C: observed); C: shop-nl B2 not-affected with observed basis (first observed clearing; NA conf. 3/20, observed
  agreement 2/21). unsafeEligibility contributors unchanged: only the three not-rendered per-key items
  (kubeDns/kubeProxy/kubeEtcd) under obs-north B1. EVAL-REPORT §2e.
- **Waiting for ARM B GO.**

## Arm A⁗ + C′ after lane defaults (2026-10-16)
- Merged `p4-env-impact` (fd103f0e defaults, 1888a107 runtime attribution) at aa067206; green.
- `A-20261016T234421Z-aa06720656` and `C-20261016T234455Z-aa06720656`: both 7 PASS / 5 FAIL / 1 INSUFFICIENT-DATA.
  obs-north B1 now affected (R1; basis observed in C′); obs-south B1 now affected = new FP (2/20). unsafeEligibility
  1/8: obs-north B1 via ci-0f84…/ci-1d8a…/ci-f835… (kubeDns/kubeProxy/kubeEtcd per-key items; components NOT
  rendered in the FROM render) — reported as a finding, matcher/FORMAT/expectations unchanged. EVAL-REPORT §2d.
- Routed: observed runtime pass does not clear an item blocked by a join `impact:not-joined` validation gap.
- Commander: report accepted; granularity finding recorded as reported (no re-matching). **HOLD**: lane `refcheck`
  fixes RenderDiff (exposure ⇒ inconclusive; dangling references in the TO render ⇒ fail; cause of the obs-south
  FP). After it merges: `git merge p4-env-impact`, score A and C once more, then ARM B (only on the commander's go).

## Current state (2026-10-16)
- `p4ei/eval2` MERGED into `p4-env-impact` (cf2b0f29). Keep this worktree on `p4ei/eval2` for the remaining runs;
  `p4ei/eval` left as is (commander + user decide).
- **Waiting** for the commander's go after lane `defaults` merges: then `git merge p4-env-impact`, score arm A and
  arm C (runtime attribution precedence merged at 1888a107), report gates. **Arm B HELD.**
- Shell hygiene (FLEET): rm on variables only as `"${VAR:?}"/...`.

## Slim storage + branch p4ei/eval2 (2026-10-16, commander request before merge)
- `p4ei/eval` (ebfff797) left untouched. New branch `p4ei/eval2` from `p4-env-impact` (c9ba21e9): the 11 lane
  files + compressed results, so the 58 MB of uncompressed records never land in history.
- Layout: per run plain `run.json`, `score.json`, `gates.json` (new, derived from score/run), `report.txt`;
  per-case records in one deterministic `cases.tar.gz`. `-rescore`/replay read it transparently (legacy
  `cases/` still readable); `-pack <run dir>` converts legacy runs (byte-verified). Tests:
  `cmd/ri/eval_investigations_store_test.go`.
- All 9 stored runs packed: 58 MB → 7.7 MB. Verification per run: rescore from the archive is byte-identical to
  rescore from the original uncompressed layout; re-classification 8/8; equal to the stored score.json for 7
  runs; the two oldest (A-201359, A-201443) differ only by the additive unmatchedFindings/unmatchedAffected
  fields added to the harness after them (decisions/metrics/gates identical).

## Arm A‴ — reference run for the ARM B decision (2026-10-16)
- Merged `p4-env-impact` incl. 3b73d721 (renderer: Argo valuesObject/plain valueFiles/git-path sources) at
  6b9fd9cc; green. Scored `A-20261016T231534Z-6b9fd9cc7c` (clean): identical decisions/metrics/gates to A″
  (7/5/1, unsafeEligibility 1/8 obs-north B1, same four findings eligible). Only shop-pl's bundle changed
  (partial-render reason names `common.yaml`); classes unchanged. EVAL-REPORT §2c′.

## Claude review of the GLM continuation (2026-10-16, after RESUME)
- Reviewed the 9 lane commits (13f9ba74, cf761e89, 998e3349, 171384ec, 339331f9, 960f51ad, e052c0c2, 4e05c478,
  91d54d18); the other 42 in the range are the sanctioned `p4-env-impact` merge.
- `-runtime` wiring (13f9ba74): correct — tool preflight, runtime validator only when opted in, its name in
  `Config.Validators` (so the config digest/investigation id is honest), arm labels C / B+C.
- AppRenderer.Repo override (cf761e89): minimal, additive, generic; needed because checkout-independent
  snapshots (55ac1cfa) store repo ".". Kept.
- All six stored runs re-classify AND re-score byte-identically via `-rescore` (checked: A ×2, A′ ×2, C ×2).
- EVAL-REPORT §2a/§2b claims spot-checked against the records: A′ unsafeEligibility 1/8 = obs-north B1 (four
  eligible not-affected values items matched beside the insufficient composite note — the §5.1a caution);
  C's three observed `affected` findings are the `scope:rendered-unexplained` ServiceMonitor items, matched
  by no expected item. Both true. Arm B table row corrected (was "prompt v4").
- Nothing reverted. `go build/vet/test ./...` green.

## Findings routed to the commander (from arm A; none fixed here)
- (a) DEFECT: layered values files (`internal/env/env.go:428` repo-mode warning) make the snapshot's `gitops-repo`
  input `partial` ⇒ R2.1 blocks every values item even with a complete render; sole blocker for obs-north/south
  B3 and obs-north B1's four values items. Caution: fixing it alone could make those values items not-affected
  in an affected case (obs-north B1 stays insufficient only via the composite note item) — re-score after a fix.
- (b) Run-record reproducibility: the ImpactReport artifact embeds evidence `retrievedAt` wall-clock, so record
  digests differ between identical runs (findings/bundles identical).
- (c) cert-manager traversal truncated at max depth 8 (R2.3 can never hold there); 28 CRD-description rendered
  changes correlated to the rotationPolicy item.
- (d) Contract tension, not a defect: join "set-but-removed values key" ⇒ ACTION REQUIRED ⇒ affected (shop-nl B1 FP,
  observed harmless). FP gate passes; reported only.
- (e) Composite upstream note (ingress-nginx #11819) = one change item for three expected items.

## Decisions (with why)
- Unit = (case, expected item); 24 item decisions over 8 cases (README lists them; no expected class there).
- Eligibility for `unsafeEligibilityRate` counted conservatively: a decision is eligible if ANY matched finding is.
- Component reach = a cited rendered change on the object, or an impact path ENDING at it (DESIGN §6.2);
  "on a path but not its end" is reported (`onPath`), not counted. Object match = kind + name (+ group when
  the ref has an apiVersion) + namespace, where a rendered object without namespace (release namespace)
  matches any — it is the same object.
- requiredEvidence: covered ⇔ an artifact with that path is cited (finding evidence/artifacts, cited
  validations' evidence/inputs/outputs/pins, cited rendered changes' evidence, cited nodes) AND the object
  and field are named somewhere in what is cited. `evidenceFileCoverage` (path only) is reported beside it.
- INSUFFICIENT-DATA makes the command exit non-zero (it is never a pass).
- `-runtime` errors until lane runtime merges (no silent no-op).

## Files touched outside ownership
- `cmd/ri/investigate.go`: `pipeline(f)` now delegates to new `pipelineAt(f, storeDir)` (additive; behaviour
  unchanged) so the eval can keep its store out of the shared state dir.
- `cmd/ri/eval.go`: 3 lines — `ri eval -investigations` dispatches to `eval_investigations.go`.
- (GLM, 13f9ba74) `cmd/ri/investigate.go`: opt-in `invFlags.runtime` → runtime validator in `pipelineAt`;
  `cmd/ri/investigate_test.go` (wiring test).
- (GLM, cf761e89) `internal/investigate/runtime/apprender.go` (exported `Repo` override),
  `internal/investigate/runtime/groundtruth_test.go` (sets it) — lane runtime's package, minimal + additive.

## Contract changes
- none.

## Open questions
- none yet.

## Test status
- `go build ./... && go vet ./... && go test ./...` green (2026-10-16).
