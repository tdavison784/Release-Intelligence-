# Status — lane `classify` (Claude Opus 5.5, agent ri-classify)

Branch `p4ei/classify`, worktree `.claude/worktrees/p4ei-classify`. Brief: `briefs/classify.md`
(+ `_wave2-common.md`, `_wave1-common.md`, DESIGN §3 / §7 "classify"). Merged `p4-env-impact` @ 359605dc
(wave 1 complete, incl. the §1.3 amendment items in `EvidenceBundle.EnvironmentItems`).
State: **DONE** (2026-10-16). Resume from: this file + brief + `git log` + `docs/INVESTIGATE.md`
"Classification and review".

## Done

Package `internal/investigate/classify/` (owned):

| File | What |
|---|---|
| `policy.go` | `New() investigate.Classifier`: R1–R4 in order; R2.2–R2.7 coverage/concern/assumption checks; R2.4 manifestation rule (behaviour/unknown items: only `ClearsBehaviour()` validations count — for coverage, answers and the ≥1-pass rule); §3.3 agent table; §3.5 action-required guard; finding fields (exposure, components, evidence, artifacts, basis, approval, provenance `classify.policy@v1`) |
| `collection.go` | R2.1 declarative table `decidingInputs` (family → change sources, environment input kinds, render-decided, validator kinds); compatibility: `kubernetes` → cluster-version, other platforms → inventory |
| `reasons.go` | deterministic reasoning (VALIDATED / NOT VALIDATED / AGENT (AI) labels), remediation, NextSteps; lexical setting extraction (flags, backticked keys, dotted fields, annotations, env vars, feature gates) from the item's upstream evidence, title, subjects and the agent's hypotheses; "observe in an isolated cluster" when upstream names none |
| `decision.go` | §3.4 decision (all items eligible ∧ agent submitted ∧ traversal complete ∧ ≥1 complete render, none incomplete ∧ no uncorrelated change ∧ tier verdict auto-pass when configured); `AutoApprovalEnabled` always false; `TierVerdict(*policy.Verdict)` |
| `review.go` | `NewReviewApplier()` (human approve/reject final, request-more-evidence needs `Requests`; proxy decisions recorded, never change state; hash-chained audit; record unchanged on error); `Apply(inv, cls, at)` records a classification (review pending, awaiting-review, audit) |
| `text.go` | `WriteText(w, inv, bundle)`: AC6 on the face of the text — class label "validated / not validated", action axis per §3.5, separate sections for validations, unvalidated assumptions, agent concerns/explanations (AI), gaps, next steps, approval |

Docs: `docs/INVESTIGATE.md` "Classification and review".

**Tests** (`go test ./internal/investigate/classify`, ~45 cases; fictional product `gizmo`): every output goes
through `Apply` (run-record `Validate`) **and** `ValidateAgainst` its bundle, and is classified twice
(byte-identical). Per rule R1 (incl. observed basis, R1 beats passes), R2 positive, R2.1 table (8 single-input
breaks, not-applicable/unrelated gaps don't block, compatibility cluster-version, no render), R2.2, R2.3
(truncated, inferred, missing, no traversal), R2.4 (path, roots via subjects, no roots ⇒ ≥1 pass; behaviour and
unknown items: render-diff/join pass never clears, pin clears, observed clears), R2.5 (error blocks, not-run
doesn't); R3 signals, R4 gaps+steps; rendered-unexplained env item. Per §3.3 row: concern blocks and moves
R4→R3, behaviour concern needs a clearing answer, **adversarial claims-mitigation without validation never
clears** (also with a passing render), established claim leaves Assumptions, explains changes nothing, proposed
(inferred) edge only adds unresolved paths, concerns can't produce affected. §3.4: each conjunct off ⇒ human
review (agent not run/exhausted/failed, tier review/missing/mismatched/block), **uncorrelated rendered change
blocks eligibility**, incomplete render, no in-scope items, AutoApproval never enacted. §3.5 ACTION REQUIRED
contract: action-required ⇒ affected with the join ref unchanged and "action: ACTION REQUIRED"; missing wrap ⇒
error; affected below action ⇒ "action: review", text never says ACTION REQUIRED; join not-affected alone
insufficient; unknown join ⇒ gap with its unknownReason. Reproducibility: 20 shuffles of validations/assertions
⇒ identical JSON; re-classifying the stored run reproduces finding ids. NextSteps name settings (evidence,
hypotheses) or say observe. AC6 text sections. Review: human approve/reject/more-evidence, proxy never approves,
8 invalid decisions leave the record unchanged, final states final, unclassified run rejected. Integration with
the real `relations.NewTraverser()`.

Opt-in real-stage test `RI_CLASSIFY_STATE=<warm .ri> go test ./internal/investigate/classify -run Groundtruth -v`:
bundle (with knowledge) + renderdiff + relations + all 5 validators + classify over all 8 groundtruth
`input.yaml` (never `expected.yaml`): **8/8 pass** (ValidateAgainst, reproducible, nothing eligible without the
agent), 24 s offline. Class counts (diagnostic, not scored; agent not run):

| case | in scope | affected | not-affected | potentially | insufficient |
|---|---|---|---|---|---|
| cert-manager payments-eu / -us | 18 | 2 | 0 | 9 | 7 |
| ingress-nginx shop-de / -fr | 27 | 0 | 1 | 4 | 22 |
| ingress-nginx shop-nl | 31 | 1 | 1 | 7 | 22 |
| ingress-nginx shop-pl (partial render) | 27 | 0 | 0 | 0 | 27 |
| kube-prometheus-stack obs-north / -south | 26 | 0 | 5 | 6 | 15 |

Every decision is `human-review-required` (agent not run; shop-pl also: no complete render). Most
insufficient-evidence findings are behaviour items with no clearing validation, and values items blocked by the
repo input's `partial` status (note 2).

## In progress / next step

- Nothing. Commander: review + merge `p4ei/classify`.

## Decisions (with why)

- **`explains` assertions are never Assumptions.** R2.7 wording mentions explains, but §3.3 (more specific) says
  explains has no effect; the agent's `submit_findings` summary is an explains for every item, so treating it as an
  assumption would make R2 unreachable whenever the agent runs. Only unestablished `claims-mitigation` are
  Assumptions (each also an `agent` gap, so R4 always has gaps).
- **Answers by assertion id only** — `Investigation.Validate` lifts a concern only when a pass names the assertion
  id; a hypothesis id in `Answers` answers the hypothesis (removes it from NextSteps), not the assertion.
- **Which validations a finding cites:** affected → only the failing ones (basis = their level; "Cites it");
  not-affected → only the counting passes (for behaviour items: the clearing ones; basis = strongest);
  R3/R4 → every validation of the item, no basis. All of them are also listed in the text.
- **Affected findings keep only context gaps** (collection, exposure, traversal, relationship); "not cleared"
  gaps are moot once a failure is demonstrated. The agent-not-completed gap goes on R3/R4 findings only (it is a
  §3.4 conjunct, not an R2 condition).
- **Unknown join verdict ⇒ gap** (kind validation, its unknownReason, NeededToDetermine as next step) and so
  blocks R2 (fail closed; §3.5 "gap signal").
- **Action-required join without a wrapped fail ⇒ permanent error**, not a finding: no valid class exists
  (`Validate` demands affected, affected demands a failing validation), and classifying around it would hide it.
- **Invalid/duplicate records or records about unknown items ⇒ permanent error** (a broken input is never
  classified around). `Config.Policy` "" or `classify.policy@v1`; anything else is an error.
- **R2.1 table:** values/image ← chart source + (repo|values[|manifests|images]) + render; rendered ←
  rendered-manifests + render; api-schema ← api-schema source + (repo|manifests|crds) + render; compatibility ←
  compatibility source + cluster-version (kubernetes) / inventory (peer); behaviour/procedure ← none (only R2.4
  clearing validations). source-diff and release-notes are not required (source-diff is always unavailable in
  milestone 1, a stated gap).
- **Proxy review decisions never change the state** (any action): DESIGN says a proxy decision is "recorded and
  keeps waiting"; only a human decides. Approve/reject are final; request-more-evidence sets status running (new round).
- **`Apply` sets review pending for every classified run**, eligible or not (§5.3: with AutoApprovalEnabled false
  an eligible run still waits for a human). Orchestrate may use it or do the same itself.
- `joinFindings`/`platformFinding` duplicate validate's 20-line mapping (lanes never import each other's packages).

## Files touched outside ownership

- `internal/investigate/api.go` — contract change 1 below.
- `internal/domain/investigation.go` — contract change 2 below (one condition).
- `docs/INVESTIGATE.md` — new section "Classification and review" only.

## Contract changes

1. `// CONTRACT-CHANGE(classify)` (additive) `Config.TierPolicy string` (digest of the configured tier policy; ""
   = none; omitempty, so existing config digests are unchanged), `ClassifyInput.Tier *TierVerdict`, type
   `TierVerdict{Policy, Digest, Tier, Why}` — the §3.4 tier-policy conjunction had no input.
2. `// CONTRACT-CHANGE(classify)` `Investigation.Validate`: the decision check was `eligible ⇔ all findings
   eligible`; now `eligible ⇒ all findings eligible`. With "⇔" no valid decision existed when every item is
   not-affected but a §3.4 conjunct fails (agent not completed — which "⇔" plus "eligible requires a completed
   agent" made contradictory — incomplete traversal/render, uncorrelated change, tier). Only relaxes a check that
   contradicted §3.4; the existing domain test (eligible with an ineligible finding is rejected) still passes.

## Open questions / cross-lane notes (commander)

1. **validate ignores environment-scope items** (bug, one line, not mine to edit):
   `internal/investigate/validate/common.go:205` looks the item up with `in.Bundle.Change.Item`; for
   `EnvironmentItems` (scope:rendered-unexplained) every validator returns `ErrNotFound`. Fix: `in.Bundle.Item`.
   Until then those items get no validation records and can never be not-affected (fail closed). The groundtruth
   test tolerates and logs it (8–16 runs per cert-manager/shop-nl case).
2. **Repo input `partial` from kustomize-not-built warnings** blocks R2.1 for every values/image/api-schema item:
   bundle's snapshot downgrades the gitops-repo input to `partial` for env.Load's "kustomization … is not built
   with kustomize" warnings, but DESIGN §0 conflict 6 says overlays are evaluated through the renders. Safe
   (only withholds not-affected), but over-strict. Suggest lane bundle: don't downgrade the repo input for that
   warning class (or record it as a separate input with `not-applicable`), rather than the policy parsing reasons.
3. Behaviour items stay uncleared on real bundles (no trusted pin facts, no observed runs) — expected, see
   validate's note 3; their NextSteps name the settings upstream gives where the evidence excerpt contains them.
4. Agent lane: to lift a concern, the validation must list the **assertion id** in `Answers`
   (`request_validation` with `Hypothesis` alone is not enough).
5. Orchestrate: `classify.Apply` + `NewReviewApplier` + `WriteText` are ready to wire; pass `Config.TierPolicy`
   and `ClassifyInput.Tier` (via `classify.TierVerdict`) when a tier policy is configured.

## Tests

`go build ./... && go vet ./... && go test ./...` green (all packages ok). Schemas regenerated: no change. Real-stage
test 8/8 green offline. Cleanup: no kind clusters, servers or bin/ created; coverage profile only in the session
scratchpad.
