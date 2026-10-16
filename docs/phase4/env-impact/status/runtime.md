# Status — lane `runtime` (Claude Opus 5.5, agent ri-runtime)

Branch `p4ei/runtime`, worktree `.claude/worktrees/p4ei-runtime`. Brief: `briefs/runtime.md` (+ `_wave2-common.md`,
`_wave1-common.md`, DESIGN §2.6 / §3.1). State: **FOLLOW-UP IN PROGRESS** (2026-10-16, commander decisions on the three open questions + the
independent-review queue; see "Follow-up 1"). Lane merged earlier as 34afd967 / 4090a4b7.
Resume from: this file + brief + `git log`.

## Done

- `exercises:` construct — `internal/catalog/exercises.go` (+ tests), `schemas/product-exercises.schema.json`,
  declarations `products/exercises/{cert-manager,ingress-nginx,prometheus-operator}.yaml` (every quote checked
  verbatim against the pinned upstream URL on 2026-10-16).

- `internal/investigate/runtime/` (validator `runtime.isolated-upgrade@v1`): `validator.go` (options, node-image
  pinning, run cache per bundle), `session.go` (renders → preflight → kind cluster → FROM / exercises / TO /
  exercises → degradations → cleanup), `observe.go` (generic observations + diff), `exercise.go`, `outcome.go`
  (per-item rules), `renders.go` (raw re-render verified against the stored artifact), `apprender.go`
  (`NewAppRenderer(app)`), `kind.go` (`KindDriver`, kubectl), `driver.go` (ports), `model.go`, `disk_*.go`.
- Unit tests with a fake driver + in-memory cluster (fictional product `gizmo`): every outcome path, quiet window
  without exercise ⇒ never pass, cleanup on every exit path (create fails, no baseline, timeout, cancellation,
  delete fails, delete unverified), observation diffing, render verification, prune/hooks, determinism.
- `docs/INVESTIGATE.md` "Runtime validation".
- Real runs on all three products (+ cert-manager payments-us), see "Real runs".

## In progress / next step

- Nothing. Real runs complete ("Real runs" below); lane done.

## Follow-up 1 (commander decisions + review queue, 2026-10-16) — branch p4ei/runtime after merging p4-env-impact @ 43303f0b

Owned now: `internal/investigate/{runtime,relations}`, `internal/catalog` (new construct), `cmd/ri/investigate.go`.

| # | Item | Done |
|---|---|---|
| 1 | data change under a consumer with no consumer degrading is not a `fail` | degradations of signature `data:*` are `Inconclusive` and name their consumers (graph successors) and whether any degraded; an item whose only attributable degradations are inconclusive gets `error` / `runtime:inconclusive-change` ("… consumers X: no degradation observed"); it fails only through a concrete degradation (then listed as context). Docs: the brief's rule is replaced (commander decision). Tests: data-only ⇒ error; consumer crash-loops after its key rotated ⇒ fail naming it. |
| 2 | exposure for controller-behaviour items | `reconciles:` construct (`internal/catalog/reconciles.go`, schema, tests; quotes verified verbatim) in `products/{ingress-nginx,cert-manager,prometheus-operator,kube-prometheus-stack}.yaml`; `relations.NewExposure(relations.WithReconcilesFrom(catalog))` roots every in-scope `controller-behaviour`/`unknown` item at every non-placeholder environment object of a reconciled kind (rule `exposure:product-reconciles`, after rendered-change and join-match roots); wired in `cmd/ri/investigate.go`. Roots, no new edge kind (no contract change): the paths from them follow established edges. Tests: render-decisive / out-of-scope / placeholder / other-product exclusions, catalog load. |
| 3 | `--runtime` in `ri investigate` | the eval branch's wiring taken verbatim (`cmd/ri/investigate.go`, `investigate_test.go`, `AppRenderer.Repo` — the snapshot is checkout-independent since 55ac1cfa, so re-renders need the checkout path) so the two branches merge cleanly, plus the `--runtime` flag in `addInvFlags` (`ri investigate` and `ri worker`). README + docs. |
| a | an accepted patch is not an exercise | catalog `reaction:` (`field-changed` path / `event` reason, cited): cert-manager renewal ⇒ `status.revision` changes (set on issuance); ingress-nginx update ⇒ the controller's `Sync` event; prometheus-operator exercises declare none (upstream documents no response to an update that leaves the generated config unchanged) ⇒ they reveal failures, never support a pass. `checkReactions` after each settle window records `Reacted`/`Unanswered` per target; an item's exercise counts only if the reaction was observed on its objects at both versions; reacted at FROM but not at TO ⇒ inconclusive `exercise-unanswered` difference. Tests: no reaction declared / none at either version / none at TO / both. |
| b | health after the TO exercises | the pass and `no-baseline` check every observation present (from, from-exercised, to, to-exercised) and the record lists them. Test asserts the text. |
| c | unsalted 64-bit data digests | `hmac-sha256:<64 hex>` under a random 32-byte per-run key, never stored or returned. Test: equal within a key, different across keys, not a plain hash. |
| d | empty families covered every family | `families` required (Go validator + schema `minItems 1`); `covers` with none is false. Tests. |
| e | unchecked subjects without a graph | rejected (`ErrRejected`), no cluster created. Test. |
| — | local, fail-closed | an item reached **only** through `exposure:product-reconciles` roots never passes (`not-run` / `runtime:reconciled-only`): a healthy exercised run shows the controller still handles its objects, not that this particular change (e.g. a removed metric) was exercised. Test. Commander may relax. |

Product-definition edits change definition digests: e2e goldens regenerated (3 `definitionDigest` lines, nothing
else); the shared warm `.ri` store re-ingests those products once after merge. My real runs used a private copy of
`.ri/store` + `.ri/cache` (render cache shared, content-addressed) so the shared store was not touched.

## Decisions (with why)

- **Exercises live in `products/exercises/<product>.yaml` (kind `ProductExercises`), not inside
  `products/<product>.yaml`.** A product definition's raw digest pins its ingested releases and upgrade edges
  (`store.LoadCurrentRelease`, `UpgradeEdge.DefinitionDigest`); editing it would invalidate the shared warm `.ri`
  store other lanes use and regenerate e2e goldens, for a validation-time trigger that says nothing about a
  release. Same catalog package, same citation rules (https upstream docs, verbatim quote ≤400 chars, never
  eval data), its own schema; `catalog.LoadDir` skips the subdirectory.
- **Exercise families**: an item counts as exercised only when its family is listed and a target object lies on
  its impact paths (cert-manager renewal: behavior/procedure/api-schema; ingress / prometheus-operator updates:
  behavior/procedure).

## Files touched outside ownership

- `products/{ingress-nginx,cert-manager,prometheus-operator,kube-prometheus-stack}.yaml` (`reconciles:`, follow-up 1
  item 2, as instructed) and the three cert-manager e2e goldens (`definitionDigest` lines only).
- `README.md` (one command line for `--runtime`).
- none before follow-up 1. (`internal/catalog/exercises*.go`, `schemas/product-exercises.schema.json`, `products/exercises/` and
  `docs/INVESTIGATE.md` "Runtime validation" are in the brief's ownership.)

## Contract changes

- none (no change to `internal/domain` or `internal/investigate/api.go`).

## Open questions (for the commander / user)

1. **Is a data change under a consumer alone a `fail`?** The brief lists "referenced data changed under a
   consumer" as a fail trigger, and the validator follows it: a key that the TO version changes under an
   exercise and the FROM version did not is `fail` even when no consumer degraded. In payments-us the CA key did
   **not** rotate but the leaf `tls.key`s did (key and cert rotate together, consumers stayed ready), and that is
   still a `fail` for the rotation-policy item. The alternative is `error` (inconclusive, R3) for data-only
   differences, with `fail` reserved for a degraded consumer, a rejection, a condition or a warning. Neither is
   unsafe (both block not-affected); the choice moves items between affected and potentially-affected. I kept
   the brief's rule and did not tune it to any case.
2. **Relations gap:** ingress-nginx behaviour items have no impact paths to Ingress objects, so the observed
   webhook refusal reaches them only as `runtime:unreached` (inconclusive, all 13 behaviour items). The same holds
   for the kps procedure item vs the rejected ServiceMonitors (noted below).
3. **Wiring (`--runtime`)** is the orchestrate lane's (`cmd/ri/investigate.go`): `runtime.New(WithStore,
   WithRenderer(NewAppRenderer(app)), WithExercises(FromSets(catalog.LoadExercises("products/exercises")...)...))`,
   name in `Config.Validators`, `Run` as its own heartbeated activity (docs/INVESTIGATE.md "Runtime validation").

## Test status

- `go test ./internal/catalog` green.
- `go build ./... && go vet ./...` green; `go test ./internal/investigate/runtime` green (GLM continuation,
  2026-10-16).

## Real runs (2026-10-16, GLM re-runs on current code; brief requires one per product)

Method: `RI_RUNTIME_STATE=<warm .ri> RI_RUNTIME_CASES=<id> RI_RUNTIME_TIMEOUT=<budget> go test
./internal/investigate/runtime -run Groundtruth -v -timeout 60m` — the harness builds the bundle offline
(change + snapshot + renders + graph + traversal), then the validator runs one throwaway kind cluster per case
and asserts every record conforms and the cluster is deleted.

### cert-manager payments-eu (`cert-manager-1.17-1.18--payments-eu`, v1.17.2 → v1.18.0, k8s 1.31) — 215 s

- Run `completed` (budget 900 s), node image pinned `kindest/node:v1.31.0`, cluster deleted (verified).
- Phases: FROM applied 61 objects (3 transient retries); TO applied 121, pruned 2, re-created 4 hook Jobs;
  zero apply/admission errors, nothing unserved, all workloads ready — **upgrade completed**.
- Exercise `cert-manager/certificate-renewal` (Issuing=True + ManuallyTriggered on each Certificate): 3 targets
  at FROM and at TO, all executed, none refused at either version.
- **3 degradations**, all exercise-window `data:tls.key` under a consumer: `cert-manager/mesh-root-ca`,
  `payments/checkout-tls`, `payments/ledger-tls` rotated the private key when renewed at TO only; the identical
  renewal at FROM did not. (Digests recorded, never values.)
- Outcomes over 18 in-scope items: **1 `fail`** `runtime:degraded` — the `Certificate.Spec.PrivateKey.
  RotationPolicy` default change (the only in-scope item reaching those Secrets); 6 `not-run`
  `runtime:no-baseline` (RBAC objects the release adds/removes — no FROM baseline can exist); 3 `not-run`
  `runtime:no-exercise` (rendered-family items whose paths carry no declared exercise — quiet window, never a
  pass); 7 `not-run` `runtime:no-object`; 1 `not-run` `runtime:other-families`. Zero `pass` — consistent: the
  only exercised behavior degraded, and nothing else had an exercise proving health.

### ingress-nginx shop-de (`ingress-nginx-1.11-1.12--shop-de`, controller-v1.11.5 → controller-v1.12.0, k8s 1.31) — 185 s

- Run `completed` (budget 900 s), node pinned `kindest/node:v1.31.0`, cluster deleted (verified).
- Phases: FROM applied 28; TO applied 47, re-created 7 hook Jobs; **1 apply error**:
  `networking.k8s.io/v1/Ingress/storefront/storefront` denied by the new admission webhook
  (`validate.nginx.ingress.kubernetes.io`: "annotation group ConfigurationSnippet contains risky annotation based
  on ingress configuration"). All product workloads ready ⇒ upgrade completed (the Ingress is a consumer object,
  not a product object).
- Exercise `ingress-nginx/ingress-update`: 2 targets at FROM, both executed; at TO 1 executed, **1 refused** —
  the same webhook denial on the storefront Ingress ⇒ `exercise-rejected` degradation (the 433574f7 rule doing
  what it was written for).
- **5 degradations**: upgrade window — `rejected` on the Ingress plus controller error logs naming it and (name
  boundary matching) the storefront Deployment/Service; exercise window — `exercise-rejected`.
- Outcomes over 27 in-scope items: 13 `error` `runtime:unreached` (behavior items with no cluster objects of
  their own, but the family's exercise degraded its target at TO only — a plausible impact the graph does not
  connect to any one item; never a fail, never a pass); 1 `not-run` `runtime:no-exercise` (its path objects are
  Service/Deployment, the declared exercise targets Ingress — per-object scoping stays honest); 13 `not-run`
  `runtime:no-object`. Zero `pass`, zero `fail` — the observed break stays inconclusive per item (R3).

### kube-prometheus-stack obs-north (`kube-prometheus-stack-89-90--obs-north`, chart 89.2.4 → 90.2.0, k8s 1.31) — 181 s

- Run `completed` (budget 1500 s), node pinned `kindest/node:v1.31.0`, cluster deleted (verified).
- Phases: FROM applied 85; TO applied 164, re-created 7 hook Jobs; zero apply errors, nothing unserved, all
  workloads ready — upgrade completed.
- Exercises: `prometheusrule-update` on 31 PrometheusRules and `servicemonitor-update` on 8 ServiceMonitors, at
  FROM and at TO, all executed, none refused at either version (`podmonitor-update` not selected — no PodMonitor
  in this environment).
- **3 degradations**, upgrade window `warning:InvalidConfiguration`: the operator rejects
  `default/kube-prometheus-stack-apiserver`, `kube-system/kube-prometheus-stack-coredns` and
  `kube-system/kube-prometheus-stack-kubelet` — `endpoints[0].authorization` cannot get the Bearer token:
  Secret `kube-prometheus-stack-prometheus-token` not found (the release adds that Secret in the chart's
  namespace; the Prometheus server looks for it in `monitoring`). No such warnings at FROM.
- Outcomes over 26 in-scope items: **3 `fail`** `runtime:degraded` (three rendered ServiceMonitor field-changed
  items, each uniquely reaching one rejected ServiceMonitor); 1 `not-run` `runtime:no-baseline` (the
  release-added token Secret — cannot exist at FROM); 1 `not-run` `runtime:no-exercise` (operator Deployment
  container-arg change, family rendered); 21 `not-run` `runtime:no-object`. Zero `pass`.
- Noted for the commander (graph fact, not this lane's to fix): the `procedure` item that describes the
  89→90 control-plane ServiceMonitor auth change itself has **no impact paths** to those ServiceMonitors
  (`runtime:no-object`), while the three rendered field-changed items on the same ServiceMonitors do connect
  and fail. Attribution of the observed break rides on the rendered items; the relations lane may want to know.

### cert-manager payments-us (`cert-manager-1.17-1.18--payments-us`) — 218 s (ri-runtime, code 433574f7)

- Run completed, upgrade completed, cluster deleted (verified). Renewal at FROM and TO on 3 Certificates.
- 2 degradations: exercise-window `data:tls.key` on `payments/checkout-tls` and `payments/ledger-tls`;
  `cert-manager/mesh-root-ca` did **not** rotate its key at TO (unlike payments-eu). No consumer degraded.
- Outcomes: 1 `fail` (rotation-policy item — see open question 1), 6 no-baseline, 3 no-exercise, 7 no-object,
  1 other-families; no `unreached` (no warning on the Certificates in this run).

### Run-to-run variation (same code, 433574f7; ri-runtime's run vs glm-runtime's)

- shop-de: ri-runtime's run had **0** apply errors at TO (48 applied) and 4 degradations (the exercise refusal +
  3 controller error-log lines); glm-runtime's had the reconcile pass rejected too (1 apply error). The webhook's
  check of an unchanged object on the reconcile pass is timing-dependent; per-item outcomes were identical.
- payments-eu: ri-runtime's run additionally saw 3 transient `RequestFailed` warnings ("CertificateRequest … already
  exists") on the Certificates under the TO renewal, which turned 6 path-less behaviour items into
  `runtime:unreached` (error) instead of `no-object`. Transient controller retries can therefore move items between
  not-run and error between runs (never to pass, never to fail). A follow-up could ignore a warning event whose
  object ends the window healthy with the event's count at 1 — not done, as it would also hide one-shot
  rejections such as the kps `InvalidConfiguration` events.

### Cross-run summary

Three products, three `completed` runs, 181–215 s each (budget 15–25 min), every cluster deleted and verified
absent, node image pinned per environment, zero harness failures. Every run observed real, product-generic
behaviour: cert-manager — renewal rotates `tls.key` at TO only (1 fail); ingress-nginx — the new admission
webhook rejects a FROM-valid Ingress annotation and refuses the same exercise (break observed, held
inconclusive per item by `runtime:unreached`); kube-prometheus-stack — the operator rejects three control-plane
ServiceMonitors over the token Secret (3 fails). No run produced a `pass`: each product's exercised behavior
degraded or the items lacked a declared exercise — the "quiet window is never a pass" rule held everywhere.

## Claude review of the GLM continuation (2026-10-16, ri-runtime on resume)

glm-runtime changed no code (only this file) and its conclusions hold; it re-ran the three cases on 433574f7 and
the lane was merged as 34afd967. Corrected/added here: Done list, files outside ownership, contract changes, open
questions, the payments-us run and the run-to-run variation above (glm-runtime's shop-de text reports the
reconcile-pass rejection as if deterministic). Integration (de2b5110): `go test ./internal/investigate/runtime
./internal/catalog` green.

## GLM handoff log

- 2026-10-16 (glm-runtime, GLM-5.3): took over from ri-runtime during its usage-limit window, mid "real runs".
  The paused agent had finished real run 1 (cert-manager payments-eu) and folded its findings into 433574f7 —
  but that commit postdates the run, so run 1's per-item outcomes reflect pre-fix code and were never recorded
  here (the promised "Real runs" section was missing). Decision: re-run **all three** cases (cert-manager
  payments-eu, ingress-nginx shop-de, kube-prometheus-stack obs-north) sequentially against current code so the
  summary table describes the validator as it will merge; this also exercises the 433574f7 fixes for real.
  Runs launched via `RI_RUNTIME_STATE=<warm .ri> RI_RUNTIME_CASES=<id> RI_RUNTIME_TIMEOUT=<budget> go test
  ./internal/investigate/runtime -run Groundtruth -v -timeout 60m`, one case at a time (the validator enforces
  one `ri-runtime-*` cluster at a time), logs kept in an untracked `.glm-scratch/` dir (deleted before done).
  Uncertain: whether the shared warm `.ri` cache holds everything the two new cases need offline; if a case
  skips with "change evidence not available offline" I will warm the cache with `GITHUB_TOKEN=$(gh auth token)`
  and re-run it.
- 2026-10-16 (glm-runtime): all three runs **PASS** (cert-manager 215 s, ingress-nginx 185 s, kube-prometheus-
  stack 181 s; details in "Real runs"). The offline-cache worry was unfounded — no case skipped; no cache
  warming needed. Each run's cluster was deleted and verified absent by the harness (`assertNoRuntimeClusters`).
  No code changes were needed; 433574f7's refused-exercise rule fired as designed in the ingress-nginx run.
  One observation surfaced for the commander (graph connectivity, see kube-prometheus-stack section): the
  procedure item describing the very change that degraded the ServiceMonitors has no impact paths to them;
  the three rendered ServiceMonitor items carry the fails. Cleanup done: `.glm-scratch/` removed, `kind get
  clusters` empty, no bin/ or other large untracked output, `git status --ignored` clean apart from `.lane-lead`.
  Uncertainties for the returning Claude lead: none blocking — the lane is done per its brief; the graph note
  above is a cross-lane observation, not a defect in this lane's scope.
