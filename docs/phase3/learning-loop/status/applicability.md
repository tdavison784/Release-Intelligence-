# Lane `applicability` — status

**State: lane work complete, pending commander review.** `go build ./... && go vet ./... && go test ./...`
green on `p3ll/applicability` (contract-3 + knowledge + dashboard merged).
No `internal/domain` type changes on this branch, so no schema regeneration was needed.

## Done

- `internal/impact/condition.go`: `EvaluateCondition` (three-valued; absence is not knowledge; every
  true carries environment evidence; every false carries an ImpactCheck against a supplied+healthy
  dimension; withheld → unknown with the leaf reason), bound to the merged env APIs (`internal/env`,
  product inventory, `InventoryComplete`). Tests: `condition_test.go` (leaves, absence, partial
  manifests, product-version table incl. chart-version refusal, never-panics).
- `unknownReason` assigned across the existing join, then mandatory (report `Validate()` + goldens
  regenerated with the reason stated).
- Deterministic joins for the capture lane's computed diffs: `crd:storage-changed`
  (`crd_attrs.go`), `crd:default-changed` / `enum-changed` / `field-required` / `field-type-changed`
  (GVK-scoped, per-resource, medium confidence demotes action to review when kind/version unpinned),
  and non-Kubernetes compatibility operands (Kafka-style support sets) decided against the product
  inventory (`compat_products.go`). Tables now in `docs/IMPACT.md` (§ The join / § compatibility).
- Knowledge pass in `impact.Build` (`knowledge.go`): trust ladder per DESIGN §4 incl. PO-1/PO-2
  (consensus = separate calls; consensus-action ACTION REQUIRED labelled "model consensus"), §2.6
  attachment (anchors + subject restatement + restatement closure, umbrella-guarded), supersession of
  the change's unknown records, never downgrading a deterministic finding, fact evidence copied into
  the pool, byte-identical output without facts (pinned by test).
- `app.LoadKnowledge` (read-only): every record validated; facts kept only when
  `impact.VerifyFacts` proves their per-aspect verification from the loaded records;
  `ReviewContexts` for the transfer subset.
- CLI: `-knowledge <dir>` / `-min-verification <level>` on `ri impact` and `ri eval`;
  `ri eval -knowledge` runs the dataset once per verification level (none/deterministic/human/
  consensus/proxy) and reports each separately (`internal/eval/levels.go`): applicability overall +
  transfer subset, classification accuracy, unknown rate, ACTION quality per level with
  model-consensus ACTION counted separately; levels with an unchanged fact set reuse the previous
  run's results; the stored-snapshot comparison and `-update` always use the knowledge-free `none`
  run, so knowledge never masks a regression of the deterministic pipeline. No regression without
  `-knowledge` (existing suite unchanged).
- Adversarial pack: eight knowledge traps (`eval/adversarial/knowledge-*`, harness in
  `adversarial_test.go`) — proxy reaching for ACTION (capped at review), proxy trying to clear
  (never clears), wrong-kind attachment (kind-scoped conditions; identical path on a same-group
  resource decides nothing), withheld secret value (decides nothing), partial manifests (never
  clear), chart-version inventory (never the app version), product missing from a non-complete
  inventory (never "not installed"), true condition + deprecation consequence (class follows the
  kind). Facts in fixtures are hand-built from the contract, never from eval expectations.
- Docs: `docs/IMPACT.md` — knowledge section (`--knowledge`, per-level eval), the CRD attribute and
  platform-operand join tables (these were claimed by commit 000eb30 but never written; added in
  this handoff).

## Decisions (and why)

- The eval's stored-snapshot diff compares the **none** level: knowledge improving a metric must
  never hide a deterministic-pipeline regression; `-update` refuses `-knowledge` for the same reason.
- Level runs are reused when the fact set is unchanged (the level sets are nested by
  `AtLeast`, so equal count ⇒ equal set; `impact.usableFacts` filters identically to `factsAt`).
- The wrong-kind trap asserts checked-clear (`impact:knowledge-clear`), not unknown: with healthy
  manifests and zero resources of the kind, the resource condition is False by the documented
  zero-resources convention (`impact:crd-field-unset`); the trap is that the sibling-kind resource
  never satisfies the condition.
- Adversarial harness: `rule: note` now builds the change as declared/note-derived
  (`MethodDeclared`), matching the vocabulary the pack README always documented — statement anchors
  (DESIGN §2.6) attach only to note-derived changes. All ten pre-existing fixtures still pass.
- Default `-min-verification` is `human` (the gate level), on both commands.

## Contract changes

- None this handoff. (Earlier lane commits follow the `CONTRACT-CHANGE` markers; none were needed
  for the eval wiring — `impact.Input.Facts`/`MinVerification` were already the contract.)

## Files outside ownership

- `internal/reviewui/demo.go` — commit ba8210a: the demo proposals lacked `provenance.callId`,
  which contract-3 made mandatory, so `TestFixturesAreValidDomainRecords` failed on the
  **integration branch itself** (dashboard fixtures predate contract-3; their status file lists the
  call-id follow-up). Minimal additive fix: the demo builder derives a distinct call id per
  (model, candidate, task). The dashboard lane should fold this into their PO-1/PO-2 follow-ups.

## Test status

- `go build ./... && go vet ./... && go test ./...` green (all packages, including
  `internal/eval`, `internal/impact`, `internal/app`, `cmd/ri`, `internal/reviewui`).
- CLI smoke: `-knowledge` with a missing dir errors cleanly; `-min-verification` without
  `-knowledge` is a usage error.
- Live `ri eval -knowledge` against a real `knowledge/` tree has **not** been run: no committed
  knowledge directory exists yet (knowledge lane open question 2). Per-level numbers are therefore
  still unmeasured.

## GLM handoff log (2026-10-01, glm-applicability continuing the paused Claude agent)

Found on arrival: six modified files + untracked `internal/eval/levels.go` (the per-level eval
wiring, half-committed), no `status/applicability.md` at all, and a pre-existing test failure on the
integration branch (`internal/reviewui` fixtures missing `callId`). Did:

1. `ba8210a` — fixed the reviewui demo fixtures (cross-lane, listed above).
2. `df564b0` — committed the in-flight eval/CLI knowledge work **plus the tests it was missing**
   (`levels_test.go`: per-level runs and reuse, baseline equality without facts, knowledge counts
   incl. consensus-action, transfer-subset math) and the `docs/IMPACT.md` `--knowledge` section.
3. `7ae7988` — the eight mandated adversarial knowledge traps: harness `knowledge:` section
   (facts must pass `VerifiedFact.Validate`), `environment.inventory` support, `rule: note` →
   declared changes, fixture README table.
4. This commit — `docs/IMPACT.md` CRD-attribute + platform-operand join tables (gap left by
   000eb30, whose message claimed them), and this status file.

Uncertain / for the commander:

- The reviewui fix is the dashboard lane's package; merge order matters only in that the
  integration branch is red without it (or their own equivalent).
- Fixture facts assert the *current* zero-resources convention (wrong-kind trap clears). If the
  contract owner later decides "no resources of the kind" should be unknown for knowledge facts
  (absence-is-not-knowledge at the kind level), `knowledge-wrong-kind` and `docs/IMPACT.md` both
  need a one-line flip.
- Per-level eval numbers (DESIGN §7 ceilings) are unmeasured until a knowledge tree is committed;
  the lane's measurement obligation then moves to a live run with the warm cache
  (`-state /Users/tommydavison/repos/Release-Intelligence-/.ri`).
