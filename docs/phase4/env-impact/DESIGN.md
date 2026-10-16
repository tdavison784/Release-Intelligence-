# Phase 4 design — environment-aware breaking-change investigation

> Normative once merged into `p4-env-impact` (owned by the commander afterwards; lanes propose changes
> with `// CONTRACT-CHANGE(<lane>): why` plus their status file — FLEET.md). Goal: `MISSION.md`
> (§A1–§A7, P1–P6, AC1–AC8). Contract code: `internal/domain/investigation.go` (types + `Validate`),
> `internal/investigate/api.go` (ports). Generated schemas: `schemas/evidence-bundle.schema.json`,
> `schemas/investigation.schema.json`, `schemas/investigation-graph.schema.json`. Example documents:
> `internal/domain/testdata/sample-*.json`.
>
> **Product-owner clarification (binding, MISSION.md):** "DFS" in the goal was a mistype for **diffs**.
> The centerpiece evidence is the rendered before/after **diff** of the environment's own configuration
> (`internal/render`). This design makes every rendered change a first-class, citable record correlated
> to the change items it realises; the relationship graph and its bounded traversal remain, kept
> lightweight, for consumers a render cannot show (an Ingress whose annotation a controller default now
> rejects, a Deployment that mounts a Secret a Certificate produces).

## Reading order

§0 says what already exists and what Phase 4 adds. §1 fixes the first milestone's scope. §2 is the
contract (types, ids, storage). §3 is the classification and approval policy — the part that must never
be weakened. §4 is the agent, §5 the orchestration boundary, §6 evaluation, §7 the lane plan.

---

## §0 Gap map — build on, not beside

Phase 3 already answers "which upstream changes reach this environment" deterministically, with two
evidence chains, render-first exposure, verified knowledge and a tier policy. Phase 4 adds: an immutable
change- and environment-scoped **bundle**, environment renders as **first-class indexed evidence**, a
light **relationship** layer for consumers, an **agent** that can look, ask and request validation, a
**four-class per-item finding** with explicit gaps, **approval eligibility**, durable **orchestration**
and an **evaluation** against observed outcomes.

### §A1 Change evidence → `ChangeEvidenceRecord`

| Need | Exists today | Phase 4 |
|---|---|---|
| from/to, structured changes | `domain.UpgradeEdge` (`internal/domain/upgrade.go:119`), `Change` (`:38`), `UpgradeEdge.Validate` (`:169`); built by `upgrade.Build` | **Reuse.** The edge JSON is the record's `upgrade-edge` artifact; each in-scope `Change` becomes a `ChangeItem` that *references* it by id (no copying of evidence). |
| changelogs / release notes | roles `release-notes`, `changelog` (`internal/domain/release.go:32-41`); `normalize.ParseNotes` (`internal/normalize/api.go:121`) | Reuse; source kind `release-notes`. |
| migration guides | role `upgrade-guide`, parsed as notes; gap warning `internal/upgrade/build.go:321` | Reuse; source kind `migration-guide`; `MigrationNotes` cite its statements. ingress-nginx declares none → status `unavailable`, reason stated. |
| source diffs | **missing** — `git-log` gives subjects only (`internal/gitsrc/log.go:22-28`); linked PRs give file lists, no patches (`internal/github/linked.go:51`) | Not collected in milestone 1: status `unavailable` ("no source-diff collector"). Agent may `request_evidence` a linked PR (`linkedev`, docs/LINKED_EVIDENCE.md). |
| API / CRD schema changes | CRD snapshots + `upgrade.crdChanges` (`internal/upgrade/crds.go:118`), CRD attribute rules | Reuse; source kind `api-schema`; items of family `api-schema`. |
| Helm chart changes | values diff `upgrade.diffValues` (`internal/upgrade/values.go:59`); packaged-chart contents | Reuse; source kind `chart`; items of family `values`. |
| Kustomize changes (release side) | **missing** (environment side only: `render.Kustomize`, `internal/render/kustomize.go:42`) | `not-applicable` for Helm-delivered add-ons (milestone scope); stated, not silent. |
| before/after manifests | **only in the render lane**: `render.Engine.RenderPair` (`internal/render/pair.go:87`), `DefaultsTarget` (`:80`), `render.Diff` (`internal/render/diff.go:207`); not part of the edge | **Promoted.** Release-scope (chart-default) pairs in `ChangeEvidenceRecord.Renders`; environment-scope pairs in `EvidenceBundle.Renders` (§A2). Each pair indexes its `RenderedChange`s. |
| compatibility requirements | `CompatibilityConstraint` (`internal/domain/release.go:128`), `upgrade.compatChanges` (`internal/upgrade/compat.go:248`), peer bounds (`internal/impact/compat_products.go:34`) | Reuse; items anchored on `Platform`. |
| per-source collection status | `SourceStatus`/`SourceState` (`internal/domain/release.go:78-100`) per consulted source; gaps as free-text `Warnings` | **Extended**: `ChangeSourceStatus` states *every* MISSION source kind exactly once (ok / partial / unavailable / not-applicable + reason); `Validate` rejects a missing kind. |
| content addressing | HTTP cache keyed by request, not content (`internal/fetch/cache.go:28-43`), records body digest; render cache content-addressed (`internal/render/cache.go:17`) | **New**: a CAS for bundle artifacts (`cas/sha256/<hex>`), `ArtifactRef` by digest. Evidence keeps `ContentDigest` → artifact digest. |

### §A2 Environment context → `EnvironmentSnapshot`

| Need | Exists today | Phase 4 |
|---|---|---|
| inventory, GitOps defs, values, overlays, resources | `env.Load` (`internal/env/env.go:409`), repo mode (`internal/env/repo.go:70`, Argo `installed.go:164`, Flux `:217`, helmfile `repo.go:425`), resources + refs (`internal/env/resources.go:138`), inventory (`inventory.go:297`) | **Reuse** whole: the parsed `env.Environment` JSON is the snapshot's `environment-facts` artifact ("facts by reference"). |
| file digests | `File{Path, Digest}` (`internal/env/env.go:150`) on every input | Reuse; every file is also an `environment-file` artifact. |
| repo commit pinning | **missing** (`render.KustomizeInput.SourceRevision` exists, never filled: `internal/render/render.go:146`) | **New** `RepoRef{Commit, Dirty, TreeDigest}`; `TreeDigest` always set (sorted path+digest list), so a non-git directory is still pinned. |
| per-input collection status | per *dimension* health `absent/ok/partial` (`internal/env/env.go:52-67`, `Statuses` `:336`); parse problems as warnings (`:372`) | **Extended** `SnapshotInput{Kind, Path|Value, Status, Reason}` per input; dimension health still drives the existing join. |
| Kustomize overlays applied | **not built** in env repo mode (warning `internal/env/repo.go:412`) — but **built by render** (`render.Kustomize`) | Renders are the source of truth for overlays; the env facts stay what they are. |
| live observation | **none** (files only; `internal/env/env.go:1-3`) | Contract only (`LiveObservation`, input kind `live-cluster`); optional, read-only, **off by default**; not implemented in milestone 1. |
| environment renders | `ri impact --render`: `app.RenderDiffEdge` (`internal/app/render.go:103`), `EnvironmentPairs` (`:245`), targets `render.DetectTargets` (`internal/render/source.go:110`), correlation `render.Correlate` (`internal/render/correlate.go:57`), chart coverage (`internal/render/chartcoverage.go:96`), failure reasons (`internal/render/render.go:36-47`) | **Centerpiece, reused**: lane `renderdiff` stores each pair's renders + diff as artifacts and indexes every rendered change as a `RenderedChange` correlated to change items (`ChangeLink` → item). Undocumented (uncorrelated) changes are kept and block automated approval. |

### §A3 Relationships → `RelationshipGraph`, `TraversalResult` (secondary)

| Need | Exists today | Phase 4 |
|---|---|---|
| graph / traversal | **none** (no node/edge/path type anywhere) | **New, lightweight**: generic `GraphNode`/`GraphEdge` vocabulary, deterministic bounded DFS. |
| relationships | implicit, one hop: `*Ref`/`*Refs` references (`internal/env/resources.go:88`, `ResolveRef` `:260`, `References` `:319`); CR→CRD by GVK (`internal/impact/crd_gvk.go:201`); install→chart (`internal/env/installed.go:36`); kustomization→files; rendered change→changelog (`ChangeLink`) | **Reuse** as edge sources (`references`, `consumes-api`, `deploys`, …). Missing today and added only where a milestone case needs them: `volumes[].secret/configMap`, Ingress→Service/IngressClass/TLS Secret, `serviceAccountName`, selectors. |
| exposure roots | the join's matches (`ImpactMatch`, `internal/domain/impact.go:247`) | Roots = objects of environment rendered changes correlated to the item (primary) + join match subjects (secondary). |

### §A4 Agent

| Need | Exists today | Phase 4 |
|---|---|---|
| model port | `llm.Client.Complete` single-shot (`internal/llm/llm.go:64`); no tools, no MCP | **New** agent tool surface (`investigate.Tools`) served over MCP stdio by `ri investigate mcp`; drivers `claude-cli` / `exchange` / `replay` / `scripted`. |
| replay / no API keys | file exchange (`internal/llm/exchange.go:70`), prompt-digest cache (`internal/llm/cache.go:39`), `claude -p` style of `docs/phase3/model-comparison/raw-arm.sh` | Reuse the idea: every tool call and result is in an `agent-transcript` artifact; `replay` re-issues it. |
| AI provenance | `domain.Provenance` AI fields + `CallID` + `Provider` (`internal/domain/provenance.go:40-67`) | Reuse unchanged on every agent record. |
| searchable evidence | none | `BundleManifest` (tags, kinds, change items) + `search_manifest`/`read_artifact` + diff tools. |

### §A5 Validation

| Need | Exists today | Phase 4 |
|---|---|---|
| validator results | `domain.ValidationResult` (`internal/domain/semantic.go:1595`): release-level, candidate-scoped, upstream evidence only (`validateUpstreamEvidence`), outcomes confirmed/refuted/inconclusive, no level field | **New** `ValidationRecord`: environment-scoped, cites environment artifacts, `Level` deterministic/observed, outcome pass/fail/error/not-run. `ValidationResult` stays for the learning loop. |
| override inspection | join rules `impact:values-set-no-effect`, `impact:values-default-no-effect`, `impact:image-render-unchanged` (`internal/domain/impact.go:174-216`), set/unset evaluators (`internal/impact/setvalues.go:77`, `unset.go:35`) | **Wrapped**: lane `validate` turns decisive join verdicts into `ValidationRecord`s (`ImpactFinding` set). |
| render comparison | `render.Validator` (`internal/render/validator.go:86`, release scope), `EvaluateRenderedChange` (`internal/render/evaluate.go:71`) | Reuse at environment scope via the bundle's pairs. |
| schema / API compatibility | CRD GVK/attr joins (`internal/impact/crd_gvk.go`, `crd_attrs.go`), target rejects values (`impact:render-target-rejects`) | Wrapped / reused. |
| static reference checks | `env.ResolveRef` | New validator over graph + rendered objects. |
| runtime | none in code; groundtruth uses `kind` by hand | Contract only (`kind: runtime`, `level: observed`); a validator is optional in milestone 1. |

### §A6 Classification and policy

| Need | Exists today | Phase 4 |
|---|---|---|
| classes | five action classes per (change × matched fact) (`internal/domain/impact.go:17-49`, docs/ACTION_CLASSIFICATION.md) | **New** four MISSION classes per (change item × environment) — `FindingClass`. Both coexist; mapping in §3.5. |
| required finding fields | `ImpactFinding` has reasoning (`detail`), both evidence chains, checks, `neededToDetermine`, `unknownReason` | `InvestigationFinding` adds paths, rendered changes, validations, assertions vs assumptions, gaps, artifacts, remediation, next steps, basis, approval. |
| approval | tier policy auto-pass/review/block with safety invariants (`internal/policy/evaluate.go:106`, docs/POLICY.md) | **New** `ApprovalEligibility` + `PolicyDecision`; automated approval runs in **shadow mode** (computed, never enacted) until the gates in §6 allow it. Tier policy, when configured, must also auto-pass. |

### §A7 Orchestration

| Need | Exists today | Phase 4 |
|---|---|---|
| durable execution | none in the product; wave-0 spike `internal/orchestrate` (merged: `InvestigationWorkflow`, stub activities, review signal, query, audit) | Wave 2 replaces the spike's placeholders with these ports and `domain.Stage` names (§5). |
| audit | append-only `ReviewDecision`s and the proxy JSONL ledger only | `Investigation.Audit`: hash-chained, validated. |

### P1–P6, AC1–AC8

| Item | Delivered by |
|---|---|
| P1 contracts + bundle | this lane (contract) + `bundle` lane (`ChangeCollector`, `SnapshotBuilder`, `BundleAssembler`, stores) |
| P2 relationship discovery + impact paths | `renderdiff` (rendered changes, the primary exposure) + `relations` (graph, roots, DFS) |
| P3 agent | `agent` lane |
| P4 deterministic verification | `validate` lane |
| P5 Temporal | `orchestrate` wave 2 |
| P6 evaluation | `eval` lane against `groundtruth` cases (`eval/investigations/`) with pre-registered `eval/investigations/gates.yaml` (this lane) |
| AC1 one upgrade × one environment | `investigate.Request` → `Investigation` |
| AC2 traceability | content-derived ids end to end; `Investigation.ValidateAgainst(bundle)` resolves every id |
| AC3 traversal | `TraversalResult` (bounded DFS; secondary to rendered diffs) |
| AC4 agent inspects/requests | `Tools`: diff queries, manifest search/read, `request_evidence`, `request_validation` |
| AC5 affected + unaffected verified vs known outcomes | groundtruth pairs (cert-manager, ingress-nginx), `observed` verification, gates in §6 |
| AC6 validated vs assumptions | `Validations` vs `Assertions`/`Assumptions` on every finding; `Basis` level |
| AC7 Temporal retry/failure/audit | §5 |
| AC8 human review when evidence is insufficient | `RequiresHumanReview` on every non-eligible finding; `Investigation.Validate` forbids `review: not-required` then |

### Conflicts with existing behaviour, resolved

1. **Five classes vs four classes.** They answer different questions about different units and both
   stay. The five-class `ImpactReport` answers *what should I do about this change × fact* and owns
   ACTION REQUIRED; the four-class `InvestigationFinding` answers *does this change item affect this
   environment* and owns approval eligibility. A finding references the report's findings for its
   change (`ImpactFindings`); §3.5 fixes how they constrain each other. The investigation never writes,
   edits or reclassifies an `ImpactFinding`.
2. **"LLMs are never required for a runtime impact decision" (docs/phase3/learning-loop/FLEET.md:27-28,
   docs/ACTION_CLASSIFICATION.md:9-12) vs an agent-driven investigation.** Kept true: every class is
   computed by the deterministic policy from validation records; an agent record can only *raise*
   concern (§3.3). The agent is required for one thing only: **automated approval eligibility** requires
   the agent stage to have completed (`AgentRun.Status = submitted`). Its only power there is to block,
   so requiring that it looked can only make automated approval rarer — fail closed.
3. **"A render delta alone never yields ACTION" (R11, docs/RENDER-FIRST.md) vs Affected from a render
   validation.** `affected` is not ACTION REQUIRED. A render-diff validation fails only when a *breaking*
   change item's effect demonstrably reaches the environment (an item is breaking by upstream
   declaration or by a computed breaking rule — §1.3); the action axis stays with the join (§3.5).
4. **`domain.ValidationResult` may not cite environment evidence** (`validateUpstreamEvidence`), so
   environment validations are a new type, `ValidationRecord`. Release-level knowledge validation is
   unchanged.
5. **Model consensus (PO-2) and proxy facts.** They reach the investigation only through the join's
   findings. Only decisive join verdicts become validation records (§2.6), and the join already applies
   the trust ladder, so a consensus fact can contribute ACTION REQUIRED (unchanged) but never a
   `pass`: a consensus/proxy fact never clears.
6. **Kustomize not applied in env repo mode vs built by render.** Overlays are evaluated through the
   environment renders (`render.Kustomize`); env facts stay a file-level view. A finding that needs an
   overlay's effect cites the render, not the env facts.
7. **"Values never sent to a model" (impactenrich) vs an agent that reads configuration.** The agent
   stage is opt-in per run (`AgentConfig.Enabled`, CLI `--agent`); environment content is then sent to
   the configured model provider, and the run says so. Secret `data`/`stringData` values are redacted at
   collection (`ArtifactRef.Redacted`), sensitive rendered values are never recorded
   (`RenderedChange.Sensitive`, mirroring `render.Change.Sensitive`). Decision for the user to confirm:
   see status file "Open questions".
8. **Brief type names that collide with existing domain names** were renamed: `ArtifactManifest` →
   `BundleManifest` (`domain.ArtifactManifest` is an `ArtifactType` constant), `ValidationResult` →
   `ValidationRecord`, `ImpactFinding` → `InvestigationFinding`.

---

## §1 First-milestone scope

### §1.1 Dependency type and products

**Helm-chart-delivered Kubernetes add-ons already in the catalog.** Products, in order:

1. **cert-manager** — richest evidence: 4 eval cases (3 with environments), the only render eval case
   (`eval/render/cert-manager-1.17-1.18`), 38 verified facts, CRD + values + chart-metadata +
   upgrade-guide sources; groundtruth pair `cert-manager-1.17-1.18--payments-{eu,us}`
   (`rotationPolicy` default; consumers through Certificate → Secret → Issuer/trust bundle).
2. **ingress-nginx** — live render results in docs/RENDER.md; groundtruth pair
   `ingress-nginx-1.11-1.12--shop-{de,fr}` (annotations risk-level default; consumers are Ingress
   objects' annotations — exactly what a controller render cannot show and a light relation can).
   No upgrade-guide or CRD source: those source kinds are `unavailable` / `not-applicable`, stated.
3. **argo-cd** (third, after 1–2 pass their gates) — 27 verified facts, an environment eval case
   (`eval/cases/argo-cd-2.14-3.0`), chart via `version.strategy: lookup`.

kube-prometheus-stack is out of milestone 1 (one eval case, no knowledge, no render case; its
consumer relations — ServiceMonitor selectors — are the natural milestone-2 extension of `relations`).

### §1.2 Environment type

A **GitOps repository** (a directory treated as a repo root): Argo CD `Application`, Flux
`HelmRelease`, helmfile releases, Helm values files, Kustomize bases + overlays, plain manifests of
consuming workloads — everything `env.Load` repo mode and `render.DetectTargets` already understand
— plus an optional product inventory and the `--kubernetes` version.

**Live-cluster observation**: optional, read-only, **off by default**. Milestone 1 defines the contract
(`LiveObservation`, `InputLiveCluster`, `RequestLiveObservation`) and implements nothing; a request for
a live read is `rejected` ("live observation disabled").

### §1.3 Which change items are in scope (`ScopeRule`)

The change record contains an item for every non-routine change and compatibility change; `InScope`
says which the investigation must classify. Rules (first match wins; recorded in `ScopeRule`):

| Rule | In scope | Items |
|---|---|---|
| `scope:routine-excluded` | no | `Change.Routine` (docs/ARCHITECTURE.md routine detector) |
| `scope:breaking` | yes | `Breaking` or `ActionRequired` (declared) |
| `scope:computed-breaking` | yes | computed diffs that remove or narrow: `values:removed`, `values:default-changed`, `crd:removed`, `crd:version-*`, `crd:fields-removed`, `crd:*-changed`/`-required`, image removed/moved, `CompatibilityChange.Narrowed` |
| `scope:rendered-structural` | yes | release-scope rendered changes no Change explains whose class the default tier policy blocks: `rbac-permission-removed`, `resource-removed`, `api-version-changed`, CRD storage/served moves (docs/POLICY.md) |
| `scope:routine-self-artifact` | no | *(commander amendment 2026-10-16)* a rendered change that is only the version bump of an artifact the product definition itself publishes (its own images / image args at the new release's version): anchored as an out-of-scope item so it is **correlated** (never "uncorrelated") |
| `scope:rendered-unexplained` | yes | *(commander amendment 2026-10-16)* every other rendered change — release scope **or** environment scope — that no Change, computed diff or structural rule explains: anchored as its own item (manifestation `rendered-config`, `RenderedAnchor` = the rendered change). Treated like `scope:category` in §3: `affected` only through a failing validation naming a concrete failure; `not-affected` only through a passing validation covering it; otherwise R3/R4. Rationale: render diffs are the centerpiece evidence (MISSION clarification) and Principle 3 (context completeness) — a change to the customer's rendered deployment that nothing upstream explains is exactly what must be investigated, not only an approval blocker. |
| `scope:category` | yes | non-breaking `removal`, `deprecation`, `migration` changes (they can still bite) |
| `scope:not-breaking` | no | everything else (features, bugfixes, security fixes — the join's `impact:security-fix` covers those) |

"Breaking" in §3 means an item in scope under `scope:breaking`, `scope:computed-breaking` or
`scope:rendered-structural`; `scope:category` and `scope:rendered-unexplained` items can be `affected` only through a failing validation
that names a concrete failure.

### §1.4 Out of scope (milestone 1)

Source diffs and release-side Kustomize collection (stated as gaps); live-cluster reads; runtime
validators beyond the groundtruth lane's manual `kind` runs; non-Helm products; non-Kubernetes
environments; multi-environment fan-out in one investigation (one investigation = one environment);
cross-product transitive upgrades (a chart pinning another chart's version — recorded as
`cross-product-context-gap`); automated approval being *enacted* (shadow only, §3.4); helmfile
templating and Flux `valuesFrom` beyond what env/render already do (they surface as `partial` inputs).

---

## §2 Contracts (`internal/domain/investigation.go`)

Every record has a content-derived id (`ShortHash` over its identity fields; timestamps are never
identity) and `Validate()`; the run record also has `ValidateAgainst(bundle)`. One test per rule:
`internal/domain/investigation_test.go`.

### §2.1 Id scheme

| Record | Prefix | Derived from |
|---|---|---|
| `ChangeEvidenceRecord` | `cer-` | product, from/to tags, edge artifact, items (+in-scope), sources, release renders |
| `ChangeItem` | `ci-` | its anchor: `changeId` \| `platform` \| `renderedChange` |
| `RenderedChange` | `rc-` | scope, render target, object, path, class |
| `EnvironmentSnapshot` | `snap-` | environment name, repo commit/dirty/tree digest, kube version, inputs (kind, path/value, status, artifacts), facts, live artifact |
| `ArtifactRef` | `art-` | kind, logical path, digest |
| `EvidenceBundle` | `bnd-` | change record id, snapshot id, manifest digest, environment renders |
| `RelationshipGraph` | `grf-` | bundle, builder, nodes, edges (+status) |
| `GraphNode` / `GraphEdge` | `gn-` / `ge-` | kind+key / kind+from+to+detail |
| `ImpactPath` | `ip-` | change item, node ids, edge ids |
| `TraversalResult` | `trv-` | graph, algorithm, bound, roots, paths, truncated |
| `ValidationRecord` | `vr-` | validator, change item, subjects, paths, rendered changes, inputs |
| `Hypothesis` / `AgentAssertion` / `EvidenceRequest` / `ValidationRequest` | `hyp-` / `asr-` / `erq-` / `vrq-` | change item, content, **model call id** (a replayed call keeps its id) |
| `InvestigationFinding` | `inf-` | environment, change item, class, rule |
| `Investigation` | `inv-` | bundle id + **config digest** — identifies the *request*, so re-submission is idempotent (Temporal workflow id) |

### §2.2 `ChangeEvidenceRecord` (§A1)

`Product`, `From`, `To`; `Edge` (artifact id of the `UpgradeEdge` JSON); `Items []ChangeItem`;
`MigrationNotes []EvidenceID`; `Renders []ManifestPair` (release scope, chart defaults);
`Sources []ChangeSourceStatus` — **every** `ChangeSourceKind` exactly once (`release-notes`,
`migration-guide`, `source-diff`, `api-schema`, `chart`, `kustomize`, `rendered-manifests`,
`compatibility`, `advisories`), status `ok` (with artifacts) / `partial` / `unavailable` /
`not-applicable` (with reason).

`ChangeItem`: exactly one anchor (`ChangeID` into the edge, `Platform` into `Edge.Compatibility`,
`RenderedChange` into a release-scope pair), `Family` (`values`, `api-schema`, `image`,
`compatibility`, `rendered`, `behavior`, `procedure`), title/flags copied for standalone rendering,
upstream `Evidence` (resolving in the bundle pool), `InScope` + `ScopeRule` (§1.3).

Family decides the deciding inputs and validators:

| Family | Deciding environment inputs | Primary validators |
|---|---|---|
| `values` | values (+ render) | render-diff, override-inspection |
| `image` | values/manifests/images (+ render) | render-diff, override-inspection |
| `rendered` | render | render-diff |
| `api-schema` | manifests / crds (+ render CRDs) | schema-compat, static-reference |
| `compatibility` | cluster-version, inventory | schema-compat (version range) |
| `behavior`, `procedure` | (none decides alone) | static-reference on consumers; otherwise no validator applies ⇒ never `not-affected` without one |

#### §2.2.1 Manifestation — where an item takes effect (decides whether render silence can clear it)

Generic design input from the groundtruth lane's `kind` runs: a whole class of breaking changes is a
**behaviour default compiled into the new controller/operator binary** (or applied server-side). In the
environment's rendered from/to diff it appears at most as CRD description text, or not at all. A complete
render that shows nothing attributable is therefore decisive only for changes that take effect in
rendered configuration. Every `ChangeItem` carries `Manifestation` + `ManifestationRule`, derived by the
collector (lane bundle) from existing evidence with the first matching declared rule — never product Go
code:

| Rule | Manifestation | Items |
|---|---|---|
| `manifest:rendered-anchor` | `rendered-config` | anchored on a release-scope rendered change |
| `manifest:computed-values` | `rendered-config` | computed `values:*` diffs (chart keys and defaults; PO-3 counterfactual renders decide unset keys) |
| `manifest:computed-image` | `rendered-config` | computed image / artifact diffs |
| `manifest:computed-compat` | `rendered-config` | compatibility items (decided by the cluster version / inventory) |
| `manifest:computed-crd-structure` | `rendered-config` | `crd:removed`, `crd:version-*`, `crd:fields-removed`, `crd:field-type-changed`, `crd:field-required`, `crd:enum-changed` (schema structure, decided against the manifests) |
| `manifest:crd-default` | `controller-behaviour` | `crd:default-changed` (server-side defaulting of objects that leave the field unset; consumers' renders are silent) |
| `manifest:rendered-correlated` | `rendered-config` | a declared/heuristic change with ≥1 release-scope rendered change correlated by rule `subject`, `values-default` or `artifact` on a path that is not description text (`…description`) |
| `manifest:declared-behaviour` | `controller-behaviour` | every other declared/heuristic (note-derived) change, including families `behavior` and `procedure` |
| `manifest:unknown` | `unknown` | nothing matched — treated as `controller-behaviour` (fail closed) |

`Manifestation.RenderDecisive()` is true only for `rendered-config`. Findings copy the item's
manifestation (checked against the bundle).

### §2.3 Artifacts, manifest, bundle on disk (P1 deliverable)

```
<state>/investigations/
  cas/sha256/<hex[0:2]>/<hex>          artifact bytes (immutable; written once; digest verified on read)
  bundles/<bnd-id>.json                EvidenceBundle (immutable)
  runs/<inv-id>.json                   Investigation (rewritten as it progresses; audit only grows)
```

`ArtifactRef{ID, Kind, MediaType, Digest, Size, Path, Origin, URI, Producer, Redacted, Tags,
ChangeItems}`. `Path` is logical and starts with the origin prefix: `upstream/<product>/<version>/…`,
`env/<path as in the repo>`, `derived/<stage>/…` (e.g. `derived/render/env/<target>/{from,to}.yaml`,
`.../diff.json`). `URI` is what a human opens (upstream URL, the repo path, `ri:<producer>`); evidence
citing the artifact uses it, with `ContentDigest` = the artifact digest.

**Redaction** (at collection, before hashing): Kubernetes `Secret` `data`/`stringData` values are
replaced by `"<redacted sha256:…>"` (keys kept); values-file keys whose last segment matches
`(?i)password|passwd|secret|token|apikey|api_key|privatekey|private_key|credentials?` likewise.
`Redacted: true` marks such artifacts. Render diffs reuse `render.Change.Sensitive`.

`BundleManifest{Artifacts}` sorted by path, unique ids and paths; its `Digest()` (canonical JSON) is part
of the bundle id. **The agent searches the manifest; it never receives the bundle.**

`EvidenceBundle{SchemaVersion, ID, Change, Snapshot, Renders (environment scope), Manifest, Evidence,
Producer, CreatedAt}`. `Evidence` is the pool every cited id resolves in (the edge's upstream records,
the environment's `local-file`/`input` records, the renders' `rendered-diff` records). Bundle
`Validate()` resolves every artifact and evidence id it contains.

### §2.4 Environment renders and `RenderedChange` — the centerpiece

`ManifestPair{Scope, Target, From, To, Diff, Status, Reason, Changes}`: one per render target
(`render.Target`: `argocd:<file>`, `flux:<file>`, `helmfile:<release>`, `kustomize:<dir>`,
`values-files`, `chart-defaults`). `Status` `ok` only for a **complete** pair: both renders succeeded,
values complete (`Target.ValuesComplete`), chart coverage holds (`internal/render/chartcoverage.go`);
otherwise `partial`/`unavailable` with the `render.FailureReason` + detail, and **no** `Changes` —
an incomplete render is an evidence gap, never "no change" (`Validate` rejects changes on a non-ok pair).

`RenderedChange{ID, Class, Object, Path, Before, After, Sensitive, ChangeItems, Correlation, Evidence}`
is one `render.Change` of the pair's `render.DiffResult`, with `ChangeItems` from `render.Correlate`
(`ChangeLink.ChangeID` → `ChangeItemID(changeID,"","")`, plus rendered-anchored items) and
`Correlation` = the `ChangeLink.Rule`. An environment-scope rendered change with **no** item is
**uncorrelated**: the customer's deployment changes and nothing known explains it. Rules:

- a finding lists **exactly** the environment rendered changes correlated to its item (exposure is never
  silently dropped) — `ValidateAgainst`;
- `not-affected` needs each of them shown harmless by a passing validation (`ValidationRecord.RenderedChanges`);
- any uncorrelated environment rendered change, or no complete environment render at all, makes
  automated approval impossible (`EvidenceBundle.Uncorrelated()`, `ValidateAgainst`).

### §2.5 Relationship graph and bounded DFS (§A3, secondary)

`GraphNode{ID, Kind, Key, Placeholder, Artifacts, Evidence}`; kinds: `product-release`,
`helm-release`, `gitops-application`, `kustomization`, `values-source`, `workload`, `service`,
`ingress`, `config-object`, `api-type`, `custom-resource`, `rbac`, `resource`, `external`. `Key` of a
Kubernetes object is `<group>/<version>/<kind>/<namespace>/<name>` (the `render.ObjectID` form) so a
rendered change's `Object` names its node. `Placeholder` marks a referenced object absent from the
snapshot.

`GraphEdge{ID, Kind, From, To, Status, Rule, Detail, Reason, Mitigates, Evidence, Artifacts,
Assertion}`. **Direction = impact propagation**: `From` is the provider (what changes), `To` the
consumer. Kinds: `deploys`, `renders-to`, `configures`, `overrides`, `references` (referenced →
referrer), `selects` (selected → selector), `serves-api`, `consumes-api`, `produces` (CR → object its
controller creates; only from a declarative relationship rule, never product Go code), `depends-on`.
Status: `established` (from evidence: an explicit name reference, a resolved selector, a render),
`inferred` (heuristic or agent-proposed), `missing` (must exist, other end unknown — e.g. `valuesFrom`
outside the repo, a `secretRef` to a Secret no input defines). **Unresolved = inferred or missing.**

`ExposureRoot{ChangeItem, Node, Rule, RenderedChange | ImpactFinding, Evidence}`: roots come from the
objects of correlated environment rendered changes first, then join match subjects.

**DFS** (`graph.dfs@v1`, lane relations): roots sorted by (change item, node id); outgoing edges sorted
by (kind, to-node id, edge id); simple paths only (a node already on the current path is not re-entered);
a path is emitted when it reaches a node with no unvisited outgoing edge (maximal simple path) — so every
reachable edge lies on some emitted path. **Bound** `DefaultTraversalBound = {MaxDepth 12 (commander amendment 2026-10-16: raised from 8 after measuring real GitOps graph depths in lane fixes; truncation still recorded and still blocks not-affected), MaxPaths 500,
MaxNodes 5000}`; hitting any bound sets `Truncated` + reason. `ImpactPath.Status` is the weakest edge
status on the path; `Unresolved` lists exactly the non-established edges on paths.

**Complete** ⇔ not truncated **and** every path established (`TraversalResult.Validate`).
`CompleteFor(item)` is the per-item form the policy uses. A root-only path (no consumers) is
established. With no relations implementation, the traversal is roots-only — honest and still
complete — so the light layer can never *weaken* a conclusion, only add paths that need validation.

### §2.6 `ValidationRecord` (§A5, P4)

`{ID, Validator (component@vN), Kind, Level, ChangeItem, Subjects, Paths, RenderedChanges, Property,
Inputs, Outputs, Outcome, Detail, Evidence, Answers, Request, ImpactFinding, Provenance, CheckedAt}`.

- **Outcome is always relative to the safety property** of (item, subjects): `pass` = demonstrably not
  reached / not broken; `fail` = demonstrably reached **and** breaking (§0 conflict 3); `error` = the
  validator broke; `not-run` = it could not run (missing binary, input, support), with the reason. A
  validator that cannot decide returns `not-run`/`error`, **never** `pass`.
- `Level`: `deterministic` (proved from artifacts) or `observed` (runtime outcome in an isolated
  cluster); `kind: runtime` ⇔ `level: observed`. Human and proxy are *reviews*, never validations; AI
  output is never a validation (`Validate` rejects AI provenance; the schema too).
- **Wrapped join verdicts** (lane validate): `impact:render-target-rejects`, action-required findings →
  `fail`; `impact:values-set-no-effect`, `impact:values-default-no-effect`,
  `impact:image-render-unchanged`, deterministic `not-affected` findings with checks → `pass`;
  `impact:values-default-unrendered` → `not-run` (no render: a gap, unlike the five-class report's
  product-owner choice, because approval needs evidence); review-required / informational / unknown →
  no record (not decisive). A knowledge finding is wrapped only when its fact is trusted
  (`deterministic` or `human`, the trust ladder's TRUSTED levels): the evaluation against the
  environment is deterministic, so the record's level is `deterministic` and its `Detail` names the
  fact and its verification. Consensus and proxy facts are never wrapped as `pass`.
- `Answers` lists agent assertion/hypothesis ids the run tested: a passing validation that answers a
  `raises-concern` assertion is the only way to lift that concern's block.
- `Pin *PinnedSetting{Kind: values-key|field|annotation|flag|env-var|config-key, Subject, Path, Value,
  RenderedObject, Evidence}`: an **override-inspection pass** that establishes, from environment evidence
  (`local-file`, `input` or an environment-scope render — checked by `ValidateAgainst`), an explicit
  setting that pins the old/safe behaviour. `RenderedObject` (with `RenderedChanges`) uses the render to
  show where the setting lands (the controller Deployment's args, its ConfigMap). `Validate` allows a pin
  only on a passing override inspection and requires its evidence. `ClearsBehaviour()` = an observed pass,
  or an override-inspection pass with a pin — the only validations that can clear a non-`rendered-config`
  item.

### §2.7 Agent records (§A4)

All carry AI provenance (`method: ai`, model, modelVersion as reported, promptVersion, promptDigest,
inputEvidence, generatedAt, **callId**, provider) and are never validation results:

- `Hypothesis{ChangeItem, Statement, Subjects, Citations}` — testable; its standing is derived from the
  validations that answer it.
- `AgentAssertion{ChangeItem, Stance, Statement, Subjects, Paths, RenderedChanges, Citations,
  ProposedEdge, Hypothesis}` — `raises-concern` | `explains` | `claims-mitigation`; a proposed edge is
  always `inferred` and names the assertion.
- `EvidenceRequest{Kind: artifact|source|environment-input|live-observation, Target, Reason, Status,
  Resolution, Fulfilled}` and `ValidationRequest{Kind, Subjects, Inputs, Hypothesis, Status,
  Resolution, Result}`.

### §2.8 `InvestigationFinding` (§A6) — the MISSION field list

| MISSION field | Field(s) |
|---|---|
| reasoning and supporting evidence | `Reasoning` (deterministic), `Evidence`, `Assertions` (agent explanations, labelled) |
| impacted environment and components | `Environment`, `Components` (node ids) |
| dependency traversal paths | `Paths`, `RenderedChanges` |
| validation results | `Validations`, `Basis` (strongest decisive level cited) |
| uncertainty and evidence gaps | `Gaps []EvidenceGap{Kind, Subject, Detail, UnknownReason}`, `Assumptions` (assertions no validation established) |
| references to artifacts | `Artifacts`, plus every id above resolving in bundle/run |
| remediation / next investigation | `Remediation`, `NextSteps` |
| (approval) | `Approval{Eligible, RequiresHumanReview, Blockers}` |

`Provenance` is deterministic (`classify.policy@vN`).

### §2.9 `Investigation` (the run record)

`{SchemaVersion, ID, Bundle, Snapshot, ChangeRecord, Environment, Config, Status, ImpactReport (artifact),
Graph (artifact), Traversal, Agent (AgentRun), Hypotheses, Assertions, EvidenceRequests,
ValidationRequests, Validations, Findings, Decision (PolicyDecision), Review (HumanReview), Artifacts
(derived), Evidence (run pool), Audit, CreatedAt}`.

- `HumanReview{State: not-required|pending|approved|rejected|more-evidence-requested, Reviewer,
  ReviewerKind, DecidedAt, Note, ProxyProvenance}`: only a **human** can approve; a proxy decision is
  recorded with AI provenance and never approves.
- `Audit []AuditEvent{Seq, At, Stage, Event, Actor, Refs, Detail, Prev}`: `Seq` contiguous from 1, `At`
  non-decreasing, `Prev` = digest of the previous event (hash chain); `AppendAudit` sets both. A
  rewritten history fails `Validate`.
- `AgentRun{Status: not-run|submitted|budget-exhausted|failed, Reason, Model, Provider, PromptVersion,
  PromptDigest, CallID, Budget, Turns, ToolCalls, Tokens, Transcript}`.

---

## §3 Classification and approval policy (`classify.policy@v1`)

Deterministic. Inputs per in-scope change item *c*: the bundle (collection statuses, environment renders
and the rendered changes correlated to *c*), the traversal (roots, paths of *c*, completeness), the
validation records of *c*, the agent records about *c*, the join findings of *c*'s change. **Never an
input:** model confidence, model agreement, the agent's opinion of the class.

### §3.1 Rules, in order (first match decides; the rule id is `InvestigationFinding.Rule`)

**R1 `classify:validated-exposure` → `affected`.** At least one validation of *c* with outcome `fail` at
level deterministic or observed. Cites it; `Basis` = its level. (A join action-required verdict arrives
here wrapped as a `fail`, §2.6.)

**R2 `classify:validated-clear` → `not-affected`.** *All* of:

1. **Collection**: every source/input that decides *c*'s family (§2.2 table) has status `ok` or
   `not-applicable`; for render-decided families, every environment render target of the product is
   `ok` (complete).
2. **Exposure accounted for**: every environment rendered change correlated to *c* is covered by a
   passing validation (`RenderedChanges`).
3. **Traversal**: `CompleteFor(c)` — not truncated, every path of *c* established (no inferred/missing
   edge reachable from *c*'s roots).
4. **Paths**: every path of *c* is covered by a passing validation (`Paths`); with no paths, every root of
   *c* is (`Subjects`); with no roots, at least one passing validation of *c* exists (e.g. a wrapped
   join `not-affected`, or — for a `rendered-config` item only — a complete render showing nothing
   attributable).
   **For a `controller-behaviour` or `unknown` item only clearing validations count** — for coverage
   here, for R2.2 and for R2.6: an override inspection that established a `PinnedSetting` (the explicit
   value/field/annotation/flag pinning the old behaviour, with the render showing where it lands), or an
   observed-level validation. A render that shows nothing never satisfies R2.4 for such an item.
5. **No contradiction**: no validation of *c* with outcome `fail` or `error`.
6. **No open concern**: every `raises-concern` assertion about *c* is answered by a passing validation.
7. **No assumption**: the finding rests on no `claims-mitigation`/`explains` assertion that no validation
   established (`Assumptions` empty).
8. **Never weakened**: no join finding of *c*'s change is `action-required`.

`Basis` = the strongest level among the cited passing validations.

**R3 `classify:plausible-impact` → `potentially-affected`.** Not R1/R2, and a plausible impact exists:
a rendered change correlated to *c*, a path of *c*, a join finding of *c* that is affected-class
(review-required/informational) or unknown-with-matches, an inconclusive (`error`) validation on an
exposed subject, or an unanswered `raises-concern` assertion. Requires `NextSteps` (what would decide it).

**R4 `classify:insufficient-evidence` → `insufficient-evidence`.** Otherwise. `Gaps` name what is
missing (collection, render, traversal truncation, unresolved edge, no applicable validator, agent not
completed); requires `NextSteps`.

Ordering of concern (for "raise"): `affected` > `potentially-affected` > `insufficient-evidence` >
`not-affected`.

### §3.2 Fail closed

Missing context (R2.1), incomplete traversal (R2.3), an unanswered concern (R2.6) or an unsupported
assumption (R2.7) each prevent `not-affected`. "No match" is never `not-affected` by itself (R2.4 needs a
passing validation even with no roots). An incomplete render never decides anything (§2.4). Render
silence never clears a behaviour change (§2.2.1, R2.4).

For a `controller-behaviour`/`unknown` item that ends in R3 or R4, `NextSteps` names the setting to look
for (the flag / values key / field / annotation the upstream text says restores the old behaviour, from
the item's evidence; or "observe the behaviour after upgrade in an isolated cluster" when upstream names
none) — produced by the classifier from the item's upstream evidence and any agent hypotheses.

### §3.3 What the agent can and cannot do

| Agent record | Effect on the class |
|---|---|
| `raises-concern` | blocks R2 until a passing validation answers it; can move R4 → R3 |
| `claims-mitigation` | none, except it is listed in `Assumptions` and therefore blocks R2 *for that finding* until a validation establishes the mitigation (then it is removed from `Assumptions`) |
| `explains` | none (attached to the finding, labelled AI) |
| proposed edge | added as `inferred` ⇒ unresolved ⇒ can only add paths / block R2 |
| `request_validation` | runs a deterministic validator; its record (not the request) is policy input |
| `request_evidence` | new artifacts in the run; re-entering collection requires a new round (§5) |

No agent record can produce `affected`, `not-affected`, or lower any class. Enforced: findings are
rejected if not deterministic (`Validate` + schema); R2 and R1 need validation records, which cannot carry
AI provenance (`Validate` + schema).

### §3.4 Approval eligibility (automated approval is shadow-only in milestone 1)

Per finding: `Eligible` ⇔ class `not-affected` (R2 held); otherwise `RequiresHumanReview` with
`Blockers` (the class, and each gap).

Per investigation, `PolicyDecision.Outcome = auto-approve-eligible` ⇔ **every** in-scope item is
eligible **and**: the agent stage completed (`submitted`); the traversal is complete; at least one
complete environment render exists and **no** environment rendered change is uncorrelated; and (when a
tier policy is configured, docs/POLICY.md) its verdict is `auto-pass`. Otherwise
`human-review-required` and `Review.State` must not be `not-required` (AC8).

`AutoApprovalEnabled` is **false** in milestone 1: eligibility is computed and measured, never enacted.
It may be enabled per dependency type only when `eval/investigations/gates.yaml`'s autonomy criteria
pass (§6.3) — a commander + user decision, recorded in COMMANDER.md. LLM confidence and model agreement
are never inputs.

### §3.5 Relation to the five-class report and ACTION REQUIRED (never weakened)

| Join (five-class) for the change | Constraint on the four-class finding |
|---|---|
| `action-required` | finding is `affected` (wrapped `fail`; `Validate` rejects anything else) |
| `review-required`, `informational` | no constraint by itself; it is a plausible-impact signal for R3 |
| `not-affected` (deterministic, with checks) | wrapped `pass` — evidence for R2, not sufficient alone (R2.1–R2.7 still apply) |
| `unknown` | gap signal (its `unknownReason` maps to `EvidenceGap.UnknownReason`) |

And the other way: the investigation never writes ACTION REQUIRED. An `affected` finding on a change
the join classifies below action stays, on the action axis, at most review-required; the renderer shows
"AFFECTED (validated) · action: review" — the action contract (dual chains + high confidence, PO-2/PO-4)
is untouched.

### §3.6 Where each rule is enforced

| Rule | `Validate` / `ValidateAgainst` | Schema | Classifier (lane classify) |
|---|---|---|---|
| findings deterministic, validations never AI, agent records AI | yes | yes | — |
| exactly four classes | yes | yes (enum) | — |
| affected cites a failing det/observed validation | yes | — | R1 |
| action-required join ⇒ affected | yes | — | R1 via wrap |
| not-affected: no gaps/assumptions, validations, basis, complete traversal, paths/roots/rendered changes covered, no fail/error, concerns answered | yes | — | R2 |
| behaviour/unknown items: only pinned-setting or observed passes cover (R2.4); finding manifestation = item manifestation; pin only on a passing override inspection, cited from environment evidence | yes (pin evidence kind against bundle) | manifestation enum, rule pattern, pin evidence minItems | R2.4 |
| R3/R4 NextSteps naming the deciding setting for behaviour items | — | — | classifier |
| finding lists exactly its correlated env rendered changes | yes (against bundle) | — | — |
| insufficient: gaps + next steps; potentially: next steps | yes | — | R3/R4 |
| one finding per in-scope item; none for out-of-scope | yes (against bundle) | — | — |
| eligibility only for not-affected; review ⇔ ¬eligible | yes | — | §3.4 |
| decision consistent with findings; agent completed; complete traversal; ≥1 complete render; no uncorrelated rendered change | yes | — | §3.4 |
| human-only approval; review required when any finding needs it | yes | — | — |
| R2.1 collection per family; tier-policy conjunction | — | — | classifier (inputs not in the run record); tested in lane classify |

---

## §4 Investigation agent (§A4, P3; lane agent)

### §4.1 Tool surface (`investigate.Tools`, MCP names)

Diff tools first — the rendered diff is the agent's primary evidence:

| Tool | Purpose |
|---|---|
| `get_change_item` | the item (family, manifestation + rule, scope) with its **upstream change text and sources**: the edge `Change` (title, detail, subjects, references) or compatibility change, the upstream evidence records (URI, locator, excerpt), the source artifacts, migration notes, the correlated release- and environment-scope rendered changes and the join findings — so the agent can name the setting that decides the item |
| `list_renders` | every render pair: scope, target, status/reason, change counts by class, uncorrelated count |
| `query_diff` | filter rendered changes (scope, target, change item, uncorrelated, classes, kinds, namespaces, path prefix) |
| `get_rendered_object` | one object/field from the from or to render (redacted), with its artifact for citation |
| `search_manifest` | artifacts by terms/kinds/change item, optional bounded RE2 grep |
| `read_artifact` | a line window (≤400 lines / 64 KiB) or one YAML node of an artifact |
| `expand_graph` | a node's neighbourhood (depth ≤3, in/out/both, edge kinds) |
| `list_paths` | impact paths (per item; unresolved only) |
| `request_evidence` | ask for an artifact/source/environment input/live read (live: rejected in milestone 1) |
| `request_validation` | run a deterministic validator now (within budget); returns request + record |
| `record_hypothesis` | a testable statement with citations |
| `record_assertion` | `raises-concern` / `explains` / `claims-mitigation`, citations, optional proposed edge |
| `submit_findings` | per-item closing summary (recorded as `explains`); ends the run |

### §4.2 Exposure to a model without API keys

`ri investigate mcp --run <inv-id> [-state …]` serves the tools over MCP stdio for one investigation. The
`claude-cli` driver runs:

```
claude -p --model <model> --mcp-config <run>/mcp.json --allowedTools 'mcp__ri__*' \
       --max-turns <budget.maxTurns> --output-format json  < task.md
```

with the system prompt in `--append-system-prompt` (or the task), and reads `session_id`, `model`, usage
and `num_turns` from the JSON envelope into `DriverResult` (model/version as reported, never assumed;
`session_id` → `CallID`). The `exchange` driver uses the existing `-llm-exchange` directory protocol
(one tool round per request/response file), so any model — or a human — can drive it; `replay` re-issues
a recorded transcript's tool calls against the same bundle; `scripted` serves tests.

### §4.3 Budgets (`domain.AgentBudget`, defaults `investigate.DefaultAgentBudget`)

`MaxTurns 40, MaxToolCalls 80, MaxTokens 600 000, MaxReadBytes 4 MiB, MaxSeconds 1200`. The tool layer
enforces tool calls and read bytes (it returns `ErrBudgetExhausted` and records the stop); the driver
enforces turns, tokens (from the envelope) and wall clock. Exhaustion is an outcome
(`AgentRun.Status = budget-exhausted`), keeps every record made so far, and makes automated approval
ineligible.

### §4.4 Prompt versioning and output validation

Prompts live as embedded files in the agent package (`prompts/investigate-agent/v1/{system,task}.md`).
`PromptVersion = "investigate-agent/v1"`; `PromptDigest = sha256(system ‖ rendered task ‖ tool
schemas JSON)`. A prompt change is a new version and a new config digest (new investigation id).

Every tool call is validated before anything is recorded: ids (artifacts, nodes, paths, rendered
changes, change items, hypotheses) must exist in this run; each `Citation{Artifact, Locator, Quote}` must
resolve to a manifest/run artifact and the quote must occur in its bytes (`investigate.QuoteOccurs`,
whitespace-normalised), then becomes a `domain.Evidence` (kind by origin, URI from the artifact,
`ContentDigest` = artifact digest) in the run pool; assertions need ≥1 citation; a proposed edge is
stored `inferred`. A failed check returns `ErrRejected` to the model as a tool error.

### §4.5 Reproducibility

The `agent-transcript` artifact (JSONL: tool name, arguments, result digest, budget counters, envelope)
makes a run replayable: `replay` re-issues the calls and must reproduce every result digest (a mismatch is
reported, not hidden). Classification is a pure function of recorded records: re-running the classifier
on a stored run must reproduce its findings byte for byte (the `reproducibility` metric, §6).

### §4.6 Task framing (prompt content requirements, not wording)

The task lists the in-scope items (id, title, family, manifestation, upstream citations), the render summary, the
correlated and uncorrelated rendered changes, the roots and unresolved edges, the deterministic
validations already run, and the policy summary of §3.3 — so the agent knows its job is to find
consumers and mechanisms the deterministic layers missed, to request validations, and that it cannot
clear anything itself. For `controller-behaviour`/`unknown` items it is told that render silence proves
nothing: its job is to read the upstream text (`get_change_item`), name the setting that pins the old
behaviour, look for it in the environment and the renders, and request an override inspection. The prompt is generic (FLEET.md: never tuned to a case; agent lanes do not read
`eval/investigations/*/expected.yaml`).

---

## §5 Orchestration boundary (§A7, P5)

Temporal stays in `internal/orchestrate` (and `cmd/ri` wiring). Activities wrap the ports in
`internal/investigate/api.go`; the same ports run in-process for `ri investigate` without Temporal
(`internal/investigate/run`, a plain sequential runner). Payloads carry ids and refs only (TEMPORAL.md:
~2 MB payload cap); records live in the `RecordStore`, bytes in the `ArtifactStore`.

### §5.1 Activities

Stage names are `domain.Stage` (the wave-0 spike's `build-env-snapshot`, `build-graph-traverse`,
`await-human-review` are renamed to these).

| Stage (`domain.Stage`) | Port | Input → output | Idempotency key | Retry | Timeout |
|---|---|---|---|---|---|
| `collect-change-evidence` | `ChangeCollector` | `ChangeRequest` → change result id (stored) | `investigate.ChangeKey(product, from, to, definitionDigest)` | transient ×5, exp. backoff 2 s→2 min | 10 min, heartbeat 30 s |
| `build-snapshot` | `SnapshotBuilder` | `EnvironmentInputs` → snapshot id | `investigate.SnapshotKey(inputs)` | transient ×3 | 5 min |
| `assemble-bundle` | `EnvironmentRenderer` + `BundleAssembler` | change + snapshot ids → bundle id | change id + snapshot id + config digest | transient ×3 (helm/chart fetch) | 15 min, heartbeat 30 s |
| `build-graph` | `GraphBuilder` | bundle id → graph artifact | bundle id + builder version | permanent on error (pure) ×1 | 2 min |
| `traverse` | `ExposureMapper` + `Traverser` | graph + bundle + report → traversal | graph id + bound | ×1 | 1 min |
| `validate` | `Validator`s | bundle, traversal, items → validation records | bundle id + validator + item | transient ×3 | 10 min per validator, heartbeat |
| `investigate` | `Investigator` | bundle, traversal, validations → agent outcome | investigation id + round | **×1** (non-idempotent, costly); transcript persisted under the key | `MaxSeconds` + 5 min, heartbeat 30 s |
| `classify` | `Classifier` | all records → findings + decision | investigation id + round + records digest | ×1 (pure) | 1 min |
| `human-review` | signal `review-decision` (`investigate.ReviewDecision`) + `ReviewApplier` | — | — | — | review timeout (default 7 d) |

The `validate` stage runs before `investigate` (deterministic pass) and again for agent-requested
validations inside `investigate` (synchronously, through `Tools.RequestValidation`). After
`investigate`, **`traverse` runs again** over the builder's graph plus the agent's proposed edges
(always `inferred`, with placeholder nodes where an end is new); that merged graph is the run's `graph`
artifact and its traversal is `Investigation.Traversal`, so a proposed relationship can only add
unresolved paths — it reaches the policy, and it can never complete a traversal. `validate` then runs
for any new path before `classify`.

### §5.2 Errors

`investigate.ClassOf(err)`: `Permanent` (invalid input, unknown product, invariant violation, missing
binary, `ErrNotFound`, `ErrRejected`, cancellation) → Temporal non-retryable application error;
`Transient` (network, registry, 429, timeouts; the default for unclassified errors) → retried within the
policy above. **Incomplete evidence is a result, not an error**: an unreachable source, a failed render,
an unreadable input are recorded statuses that flow into `insufficient-evidence`; only infrastructure
failures are errors.

### §5.3 Workflow

Workflow id = `Investigation.ID` (bundle id + config digest) — re-submitting the same request attaches to
the existing run. Query `state` returns the stage, round, decision and audit. Review: when
`Decision.Outcome = human-review-required`, the workflow waits for `review-decision`; `approve` (human
only — a proxy decision is recorded and keeps waiting), `reject` (ends), `request-more-evidence` (a new
round: collect the requested inputs, re-assemble — a new bundle id — re-run validate/investigate/classify;
at most 2 rounds); timeout ⇒ outcome `review-timed-out`, **nothing approved**. Cancellation records a
`cancelled` audit event. With `AutoApprovalEnabled` false (milestone 1) an eligible run still waits for a
human, and the eligibility is recorded for measurement.

### §5.4 Audit events

Every activity appends `started` / `completed` / `failed` / `retried` events (stage, actor `system`, refs
= the ids produced); the agent stage appends `requested` events for each evidence/validation request
(actor `agent:<model>`); review appends `decided` (actor `human:<name>` or `proxy:<model>`). The workflow
keeps its own deterministic log (spike `AuditEntry`); the run record's hash-chained `Audit` is written by
the activities through the `RecordStore`.

---

## §6 Evaluation (P6; lane eval, dataset by lane groundtruth)

### §6.1 Dataset and units

Cases under `eval/investigations/<case>/` (format: `eval/investigations/FORMAT.md`). The unit is an
**item decision**: one (case, expected scope item) pair; the system's finding for the change item that
the item's `match` selects is compared with the expected classification. Unmatched expected items are
scored as the system saying `insufficient-evidence` and counted as `itemMatchMisses`.

### §6.2 Metrics (all reported; gated ones in `eval/investigations/gates.yaml`)

| Metric | Definition |
|---|---|
| `falseNegativeRate` | expected ∈ {affected, potentially-affected} and system `not-affected`, over expected ∈ {affected, potentially-affected} — **the unsafe error, strictest gate** |
| `unsafeEligibilityRate` | system finding approval-eligible but expected ≠ `not-affected`, over eligible system findings |
| `falsePositiveRate` | expected `not-affected`, system `affected`, over expected `not-affected` |
| `affectedDetection` | expected `affected`, system ∈ {affected, potentially-affected}, over expected `affected` |
| `affectedConfirmation` | expected `affected`, system `affected` |
| `notAffectedConfirmation` | expected `not-affected`, system `not-affected` (usefulness) |
| `exposureRecall` | expected affected components reached by the finding (a correlated rendered change on the component's object, or an impact path ending at it), over expected affected components |
| `evidenceCoverage` | `requiredEvidence` entries cited by the finding (artifact path + object), over all |
| `citationResolution` | finding/assertion citations that resolve in bundle/run, over all (structural) |
| `unresolvedRate` | system `insufficient-evidence` where expected is decisive (affected / not-affected) |
| `observedAgreement` | over items with `verification: observed`: system class equals expected (potentially/insufficient count as disagreement) |
| `reproducibility` | re-classification of the stored run reproduces its findings exactly, over runs |
| `humanReviewRate`, `autoApprovalEligibilityRate` | reported, not gated (P6 "measure") |
| `pipelineFailures` | runs that errored (not classified) |

**Per manifestation**: every metric is also reported per item `Manifestation` (render-decisive vs
behaviour), so render-only blind spots are visible.

**Per verification level**: every metric is also reported per finding `Basis` (`deterministic`,
`observed`, none) and per expected `verification` (`observed`, `render`, `upstream-only`); review
outcomes per reviewer kind (`human`, `proxy` — always labelled).

### §6.3 Pre-registered gates

`eval/investigations/gates.yaml` (committed in this lane, frozen once merged). Semantics: `min` ⇒
actual ≥ threshold; `max` ⇒ actual ≤ threshold; a gate whose denominator is below its `minSample` is
`insufficient-data` — shown as such, never a silent pass, and always a FAIL for the autonomy criteria.
The autonomy criteria (when `AutoApprovalEnabled` may be considered for a dependency type) need
`falseNegativeRate` and `unsafeEligibilityRate` at 0 over at least 20 observed-verified not-affected
decisions and 20 affected/potentially-affected decisions of that type. Milestone 1 will not reach that
sample, by design.

---

## §7 Lane plan (waves 1–3)

Every implementation package lives under `internal/investigate/<lane>/`, depends on
`internal/investigate` (ports) and `internal/domain`, and may use the existing packages named; never
another lane's package (wiring happens in `internal/investigate/run` and `internal/app`). Each lane
writes its own in-memory fakes of the ports it consumes.

### Wave 1 (parallel, after this lane merges)

**`bundle`** (Sonnet) — P1 collection + stores.
- Owns `internal/investigate/bundle/`, `internal/investigate/store/`.
- Implements `store.New(root string) (*Store, error)` satisfying `investigate.ArtifactStore` and
  `investigate.RecordStore` (§2.3 layout, atomic writes, digest-verified reads, bundle immutability,
  audit-only-grows check); `bundle.NewChangeCollector(app *app.App, st investigate.ArtifactStore)
  investigate.ChangeCollector` (over `app.Upgrade` + release-scope `app.RenderDiffEdge` chart-default
  pair; §1.3 scope rules; §2.2.1 manifestation rules; every `ChangeSourceKind` stated); `bundle.NewSnapshotBuilder(st)
  investigate.SnapshotBuilder` (over `env.Load`, `RepoRef` incl. git commit/dirty/tree digest,
  redaction §2.3); `bundle.NewAssembler(st) investigate.BundleAssembler`.
- Done: a bundle for each groundtruth case's `input.yaml` that `Validate`s; determinism test (two runs →
  same bundle id).

**`renderdiff`** (Opus) — **critical path**: environment renders as indexed evidence.
- Owns `internal/investigate/renderdiff/`.
- Implements `renderdiff.New(app *app.App, st investigate.ArtifactStore) investigate.EnvironmentRenderer`
  over `app.RenderDiffEdge`/`EnvironmentPairs`, `render.Correlate`, chart coverage: renders + diffs
  stored as artifacts, one `ManifestPair` per target, `RenderedChange` per `render.Change` with
  correlation to change items (incl. rendered-anchored items), sensitive values withheld,
  incomplete pairs with reasons and no changes. Also the diff query engine the agent's `query_diff` /
  `get_rendered_object` / `list_renders` will call: `renderdiff.Query(b *domain.EvidenceBundle,
  st investigate.ArtifactStore, q investigate.DiffQuery) ([]domain.RenderedChange, error)`,
  `renderdiff.Object(...)`, `renderdiff.Summaries(...)`.
- Done: groundtruth envs render (helm/kustomize present; skip with message otherwise); the
  cert-manager and ingress-nginx pairs show the expected rendered changes correlated.

**`validate`** (Sonnet) — P4 validators.
- Owns `internal/investigate/validate/`.
- Implements `investigate.Validator`s: `validate.JoinVerdicts(report)` (wraps decisive join verdicts,
  §2.6), `validate.RenderDiff()` (env-scope: no attributable change for the item's subjects / fail when a
  breaking item's change reaches them), `validate.OverrideInspection()` (values/overlay/flag/annotation
  settings neutralising the item, via `impact` set/unset evaluators and the renders; a pass records the
  `PinnedSetting` it established, citing environment evidence — required to clear behaviour items), `validate.SchemaCompat()`
  (CRD GVK/version/field rules against manifests and rendered CRDs; kube-version ranges),
  `validate.StaticReference()` (references resolve / dangle on graph + rendered objects). Each returns
  `not-run` with a reason when it cannot decide.
- Done: unit tests per validator incl. "cannot decide ⇒ never pass".

**`relations`** (Sonnet; keep light) — secondary graph.
- Owns `internal/investigate/relations/`, the `relationships:` construct in product definitions if one is
  needed (additive catalog change, declarative only).
- Implements `relations.NewBuilder(st) investigate.GraphBuilder` (nodes from rendered objects + env
  resources; edges from `env.Resources` refs, volumes, Ingress backends/TLS/class, serviceAccountName,
  selectors, CR→api-type), `relations.NewExposure() investigate.ExposureMapper`,
  `relations.NewTraverser() investigate.Traverser` (§2.5 DFS exactly).
- Done: DFS determinism + bound tests; graph for each groundtruth env validates.

### Wave 2

**`agent`** (Opus) — after bundle + renderdiff.
- Owns `internal/investigate/agent/` (incl. embedded prompts), `cmd/ri/investigate_mcp.go`.
- Implements `investigate.Tools` (incl. `get_change_item`; validation of ids/citations, budgets, transcript), MCP stdio server
  `ri investigate mcp`, drivers `claude-cli` / `exchange` / `replay` / `scripted` (`investigate.AgentDriver`),
  `agent.New(...) investigate.Investigator`.
- Done: scripted-driver tests for every rejection path; a replay reproduces result digests; one live
  `claude -p` run on a groundtruth *input* (never reading `expected.yaml`).

**`classify`** (Opus) — after validate.
- Owns `internal/investigate/classify/`.
- Implements `classify.New() investigate.Classifier` (§3 exactly, incl. R2.1 per-family collection, R2.4
  manifestation, NextSteps naming the deciding setting, and the tier-policy conjunction) and `investigate.ReviewApplier`.
- Done: a test per rule and per §3.3 row; every output `ValidateAgainst`s its bundle; reproducibility test.

**`orchestrate`** (Sonnet) — after wave 1.
- Owns `internal/orchestrate/`, `internal/investigate/run/` (in-process runner), `cmd/ri/investigate.go`
  (`ri investigate <product> <from> <to> --repo … [--agent] [--temporal]`, `ri worker`), docs.
- Replaces the spike's placeholders with these ports, `domain.Stage` names, §5 retry/timeouts, the review
  signal → `ReviewApplier`, the run record audit.

### Wave 3

**`eval`** (Opus) — after all.
- Owns `internal/eval/investigations*.go`, `eval/investigations/results/`, report docs.
- Implements the §6 metrics and the `gates.yaml` evaluation (`ri eval -investigations`), per-level
  reporting, the matcher from `match` to change items, reproducibility re-classification.

### Dependency order

```
contract ─┬─ bundle ─────┬─ agent ───────┐
          ├─ renderdiff ─┘               ├─ eval
          ├─ validate ───── classify ────┤
          ├─ relations ──────────────────┤
          └─ (groundtruth, running) ──── orchestrate (wave 2, after wave 1)
```
