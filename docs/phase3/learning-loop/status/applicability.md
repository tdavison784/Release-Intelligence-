# Lane `applicability` — status

**State: done** (branch `p3ll/applicability`; contract, contract-2 and contract-3 merged).
`go build ./... && go vet ./... && go test ./...` green. Schemas regenerated (`go run ./internal/domain/schemagen`).

## applicability-3 (branch `p3ll/applicability-3`, from p3-learning-loop @ 62afad6) — done

Safety fix from LOOP-DIAGNOSIS.md §7.8. A `cli-flag` / `env-var` / `feature-gate` leaf naming a component
(container name) whose workload is not in the supplied manifests returned FALSE, so a trusted fact scoped to
`component: controller` could clear a change from silence (Helm-installed controllers are rarely in the
customer's manifests). Now:

- no container of that name → **unknown · environment-visibility-gap**, needed: that workload's manifests;
- false only when the manifests are **declared complete** (`ri impact --manifests-complete` →
  `env.Inputs.ManifestsComplete` → `Environment.ManifestsDeclaredComplete` + input evidence) **and** parsed
  healthily; the check cites the declaration;
- a present container that does not pass the flag is false as before (its name fact is the examined
  evidence); a feature gate stated in the values key at `path` still decides.
- Reversed tested decision documented in docs/IMPACT.md ("Decision reversed (2026-10-02)"): it treated
  *parsed completely* as *complete*. `condition_test.go` case updated; new `TestEvaluateConditionNamedComponentAbsent`.
- Adversarial `eval/adversarial/knowledge-named-component-absent`: a human fact scoped to `component:
  controller`; the old evaluator produces `impact:knowledge-clear` (verified: the trap fails on the pre-fix
  code), the fix produces `impact:knowledge-undecided`.
- Full eval, base (p3-learning-loop) vs fix, same cache: `ri eval` JSON byte-identical without knowledge and with
  `-knowledge knowledge -min-verification proxy`; the per-level text panel identical (applicability 0.476/0.495,
  ACTION 17 with 1 false — the pre-existing kyverno E9 — at every level, unknownHonesty 1.00). **No new false
  ACTION.** All 8 component-scoped facts in `knowledge/` are proxy-level (they could not clear anyway). Per
  finding (`ri impact -knowledge -min-verification proxy` over all 19 environments): 8 knowledge-undecided findings
  in 6 environments change reason release-knowledge-gap → environment-visibility-gap (cert-manager-1.16-1.17 and
  --eu-platform: CAInjectorMerging gate; cert-manager-1.17-1.18: two ingress-shim flags; cilium-1.16-1.17 and
  --plant-edge: hubble-relay `--dial-timeout`, cilium-agent `--k8s-watcher-endpoint-selector`); no class or
  summary changed. Once those facts are human-verified this is the difference between UNKNOWN and a silent
  NOT AFFECTED.
- Residual (not changed): the `ref` leaf's "unresolved ⇒ false" still keys on parse health
  (`RefResolution.ManifestsComplete`), not on a declaration — same class of issue, envparse/contract decision.

## applicability-2 (branch `p3ll/applicability-2`, from p3-learning-loop @ e7cc4e7) — done

Product-owner decision: adopt `unknownHonesty` (docs/phase3/learning-loop/UNDECIDED-SCORING.md) as a
**reported, not gated** metric. `reasonAgreement`: later. `eval/gates.yaml` untouched.

- `internal/eval/compare.go` `scoreReport`: per `environment.undecidedImpact` link, honest iff no AFFECTED and no
  NOT-AFFECTED finding joins a change the item's matchers select (same `expIDForChange` attribution as the decided
  links); `EnvMetrics.UndecidedLinks` / `UndecidedHonest` (+ `UnknownHonesty()`), per-link `EnvUndecided` audits naming
  the overclaiming findings and the facts behind them.
- `AggregateResults`: `undecidedLinks`, `undecidedHonest`, `unknownHonesty`. Undecided links never enter
  `applicabilityAccuracy` (pinned by test).
- Transfer subset (`levels.go`): undecided links whose overclaim rests on a fact reviewed with that environment are
  excluded; `TransferMetrics.UnknownHonesty`; per-level `LevelReport.UnknownHonesty`.
- Report: one line next to applicability accuracy and the affected-link hit rate, "reported, not gated", with the
  vacuity note ("vacuous — no affected link was decided …" when nothing is hit); a per-level/transfer block in the
  `-knowledge` panel.
- Test `undecided_test.go`: honest (UNKNOWN), dishonest-affected (REVIEW), dishonest-clear (NOT AFFECTED), the
  emit-nothing vacuous case, aggregation, rendering, transfer exclusion.
- Docs: `eval/FORMAT.md` (scoring of undecided links); `internal/eval/case.go` comment (groundtruth-owned file,
  comment only).
- Live (offline, warm cache): **unknownHonesty 1.00 (10/10)**, next to applicability accuracy 0.47 and affected links
  18/73. No regressions. Gated numbers are identical to `p3-learning-loop` built alone; the failing
  falseActionRate gate (2/15) is pre-existing on the integration branch, not from this change.

## Done

- **Condition evaluator** (`internal/impact/condition.go`): `EvaluateCondition` / `EvaluateConditionWith`, three-valued
  Kleene logic over every op of DESIGN §1.3, bound to `internal/env` (resource facts, text blocks, refs, values keys,
  GVK usage, images, workload container args/env for `cli-flag`/`env-var`/`feature-gate`, the product inventory,
  `--kubernetes`, the edge's from-version). Absence is not knowledge (a leaf is false only against a supplied, healthy
  dimension; partial → unknown), withheld values decide nothing, scope is per resource, every true carries environment
  evidence and every false an `ImpactCheck`, `not(false)` is true only with examined evidence. `rendered-change` goes
  through a `RenderedChangeEvaluator` interface; the default `RenderUnavailable` answers unknown
  (environment-visibility-gap). `internal/render` is not imported.
- **Trust ladder** (`ClassifyKnowledge`, DESIGN §4, PO-1/PO-2): trusted → the fact's class (high); consensus/proxy →
  capped at review (medium), never clear (unknown · release-knowledge-gap); PO-2 consensus-action facts keep
  ACTION REQUIRED when the exposure is true with evidence, labelled `ACTION REQUIRED · model consensus`
  (`Knowledge.Verification=consensus`, `Consensus` scope, `ConsensusAction`).
- **Attachment** (`AttachedChanges`, §2.6): statement anchors; computed restatements by subject (per family, and only
  diff rules matching the fact's change kind — a deprecation never attaches to the later removal); closure under the
  restatement grouping for note-derived changes only; umbrellas, other products, inactive facts and facts introduced
  outside the edge never attach.
- **Build integration**: `impact.Input{Facts, MinVerification, Render}`; knowledge findings supersede the change's
  unknown records, are added next to a deterministic verdict only when stronger, never downgrade one; fact evidence is
  copied into the report pool; evaluation-created records (from-version input, values-file absence) into the
  environment pool. Byte-identical output without (applicable) facts — pinned by test and the e2e goldens.
- **`unknownReason`** assigned at every UNKNOWN emission of the existing join, then made mandatory in
  `ImpactReport.Validate()` and the schema; knowledge rules must carry their class. Goldens regenerated with the reason
  stated (JSON: added field only; text: the three collapsed UNKNOWN group lines name their reason).
- **New deterministic joins**: `crd:default-changed`, `crd:enum-changed`, `crd:field-required`,
  `crd:field-type-changed` (GVK-scoped, per resource, kind/version unpinned → medium → review at most) and
  `crd:storage-changed` (V2) in `crd_attrs.go`; operand/peer-product compatibility constraints (strimzi-style Kafka
  support sets) against the inventory in `compat_products.go`. `crd:fields-added` stays unknown
  (release-knowledge-gap).
- **`InventoryComplete`**: `inventory.yaml` mapping form `{complete: true, products: [...]}` →
  `Environment.InventoryComplete` + evidence; the list form is never complete.
- **Loading knowledge** (`app.LoadKnowledge`, `impact.VerifyFacts`, `impact.ReviewContexts`): read-only over the
  `knowledge/` record tree; a fact is used only when `VerifiedFact.Validate` and `domain.ValidateFactRecords` prove its
  verification from the loaded records (a proxy decision claimed as human, or a missing basis, is refused).
- **CLI / eval**: `-knowledge <dir>` / `-min-verification` on `ri impact` and `ri eval`. `ri eval -knowledge` runs the
  dataset per level (none, deterministic, human, consensus, proxy) with the transfer subset, ACTION quality per level
  (consensus ACTION counted separately); the stored-snapshot diff always uses the knowledge-free `none` run; gates run on
  the `-min-verification` level (default human) and the panel marks it.
- **Text output**: knowledge provenance line on each knowledge finding; `reason:` on unknown items.
- **Tests**: condition leaves/absence/partial/product-version table; ladder table; DESIGN §10 path 1 (synthetic and on
  the recorded cert-manager edge: 3 real restatements → REVIEW) and path 2 (ACTION with both chains; proxy → REVIEW;
  guards → UNKNOWN); supersession; never-downgrade; attachment guards; PO-2; CRD attribute/storage/platform joins;
  load-time verification traps. Adversarial pack: 8 knowledge traps (`eval/adversarial/knowledge-*`).
- **Docs**: `docs/IMPACT.md` (UNKNOWN reasons, knowledge in the join + condition bindings, CRD attribute and platform
  tables, complete inventories), README, `eval/FORMAT.md`.

## Eval (live dataset, offline, warm primary cache; no `-knowledge`)

No regressions against `eval/results`. Gates unchanged except the pre-existing failing one, which moves:
applicabilityAccuracy **0.095 → 0.143** (3/21; karpenter E1 via the `crd:storage-changed` join, as UNKNOWN-ANALYSIS
predicted). ACTION findings 4 → 5, **0 wrong** (new: strimzi 0.46 vs the inventory's Kafka 3.8 — outside the supported
set 3.9, 4.0). unknownRate 0.73 → 0.72. I did **not** run `-update`; the only delta is `karpenter impactLinksHit 0 → 1`
(better) — commander's call. Per-level knowledge numbers are unmeasured: no committed `knowledge/` tree yet (smoke-tested
with an empty one).

## Decisions (and why)

- Default `-min-verification` is `human` (the gate level); consensus/proxy facts are used only when asked for.
- Schema-attribute joins: a default applies to an unset field even when its plain-object parent is absent (defaulted
  parents are common; "not affected" must not rest on that guess); `required` binds only inside a stated parent. When no
  resource of the changed version is touched but resources of the kind exist at another API version → unknown
  (runtime-behavior-gap: conversion), never clear. Found live on karpenter (v1beta1 NodePools vs a v1 default).
- Operand constraints use the inventory with product-version semantics; a non-Kubernetes platform missing from the
  inventory is unknown (cross-product-context-gap) unless the inventory is declared complete.
- A credential-named ConfigMap key whose field value is withheld also has its text lines treated as withheld (see the
  envparse finding below).
- Transfer: a link is excluded from the transfer subset when one of its deciding facts was reviewed with
  `ReviewItem.Context.Label == <case id>`.

## Contract changes (`CONTRACT-CHANGE(applicability)` markers; all additive)

- `internal/domain/impact.go`: `DimensionFromVersion`; match kinds `product`, `text-line`, `reference`,
  `from-version`, `absence`; `unknownReason` mandatory on unknown findings; knowledge rule ↔ class consistency.
  `schemagen/meta.go` enums + the "unknown requires unknownReason" conditional; schemas regenerated.
- `internal/env`: `Environment.InventoryComplete` / `InventoryCompleteEvidence` and the inventory mapping form
  (DESIGN §1.3 requested this of envinv).

## Files outside ownership

`internal/domain/impact.go`, `internal/domain/schemagen/meta.go`, `schemas/*.json` (regenerated), `internal/env/env.go`,
`internal/env/inventory.go` (+ test), `internal/app/impact.go`, `internal/app/knowledge.go` (+ tests),
`internal/reviewui/demo.go` (one line: demo proposals get the `callId` contract-3 requires — the integration branch is
red without it; dashboard lane should fold it in), `README.md`, `eval/FORMAT.md`.

## Findings for other lanes / the commander

- **envparse (secret handling)**: a ConfigMap `data` key with a credential name (`password: hunter2`) has its field value
  withheld, but its text block lines are still exposed unredacted (`TextBlock.Lines[].Text`). The evaluator guards it;
  the extractor should withhold them too.
- **knowledge lane**: transfer reporting assumes `ReviewItem.Context.Label` is the eval case id when an item was reviewed
  with an environment illustration. Please confirm or tell me the convention.
- **render lane**: implement `impact.RenderedChangeEvaluator` (`EvaluateRenderedChange(c, env, edge) ConditionResult`)
  and pass it as `impact.Input.Render`; the CLI wiring is yours (`--render`).
- **groundtruth / D1**: unchanged by this lane.

## Open questions

- `-update` of `eval/results` for the karpenter improvement (commander).
- Whether gates should ever run on the consensus level ("the pre-registered gates apply to the combined output", PO-2)
  — today only when `-min-verification consensus` is passed explicitly.

## GLM handoff (2026-10-01/02) — reviewed

A GLM-5.3 agent continued the lane while the Claude window was paused (`[glm-handoff]` commits ba8210a, df564b0,
7ae7988, 7f53973). Review outcome: kept the eval/CLI commit (it is the paused work plus good tests), the reviewui
callId fix and the adversarial harness. Fixed: two trap facts that a reviewer could not have accepted (a "deprecation"
saying the value is ignored, exposure `unset`; a "workload-failure" that was a log-level default), the `crd:storage-changed`
doc row (its clear rule is `impact:crd-unused`), this status file (it claimed no domain/contract changes), gofmt.
Added what it lacked: load-time verification tests, the e2e real-data demo, UNKNOWN-reason docs, text rendering.
