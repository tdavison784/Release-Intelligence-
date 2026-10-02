# Research notes: Grafana Loki v2.9.6 → v3.0.0 (with environment)

Researched blind on 2026-10-01. No Release Intelligence output, results, adjudications or
testdata were consulted. Every quote in case.yaml was checked verbatim against the fetched
upstream file.

## Tags

`git ls-remote --tags https://github.com/grafana/loki` lists v2.9.0–v2.9.17 and v3.0.0. I picked
**v2.9.6** as `from` because the release-please `## [3.0.0]` CHANGELOG heading compares
`v2.9.6...v3.0.0` (v2.9.6 was the newest 2.9 patch when 3.0.0 shipped on 2024-04-08; v2.9.7+ came
later). Both tags match the product's `^v(?P<version>\d+\.\d+\.\d+)$` pattern.

## Sources read

- **Upgrade guide at v3.0.0** (`docs/sources/setup/upgrade/_index.md`, section `## 3.0.0`): the main
  source. Provides the five-item shortlist (structured metadata + tsdb/v13, `shared_store`, 256KB line
  size, 15-label limit, metric renames) and the `#### …` subsections for every item except E10.
- **Upgrade guide on main** (what the product's `upgrade-guide` channel reads): same 3.0.0 content plus
  a later-added "Migrate to TSDB" caution and a `service_name` label section. Checked so the matchers
  work on both versions. Also showed that the `-ruler.alertmanager-use-v2` default flip that the task
  prompt suggested lives under `## 3.2.0`, not 3.0.0, so it is recorded as `notExpected`.
- **Release notes v3-0.md** (main): feature summary; its "Upgrade Considerations" only points at the
  guide; it confirms "Loki 3.0 defaults to using the v13 schema".
- **CHANGELOG.md on release-3.0.x**, section `[3.0.0]`: one bullet per removal (#10840 shared_store,
  #11038 max-look-back-period, #10709 max-transfer-retries, #10655 flush_shutdown, #11665
  legacy-read-mode, #11110/#11003 metrics-namespace, #10793 better defaults). The release-branch
  wording of the metrics-namespace bullet differs from main's; the quote uses the release branch.
- **Configuration reference at v2.9.6 and v3.0.0**: before/after defaults for
  `allow_structured_metadata` (false → true), `max_line_size` (0B → 256KB),
  `max_label_names_per_series` (30 → 15), the 2.9.6 YAML locations of the removed keys, and that
  `metrics_namespace` does not exist in 2.9.6. It also shows that 2.9.6's `delete_request_store`
  "Defaults to -compactor.shared-store", which is why E3 bites configs that relied on that fallback.
- **Code at the tags**: `pkg/loki/validation.go` (the CONFIG ERROR for schema < v13 / non-tsdb on
  the *active* period, `ActivePeriodConfig`); `pkg/compactor/compactor.go` (the
  delete-request-store error); `pkg/util/cfg/dynamic.go` (`ConfigFileLoader(args, "config.file",
  true)`, i.e. a strict YAML first pass, so a removed key is a startup failure and not ignored);
  `pkg/loki/loki.go` at both tags (the `-legacy-read-mode` default true → false, which the config
  reference does not document because the flag is hidden).
- **API reference v2.9.6**: `/ingester/flush_shutdown` and the different default (terminate) of the
  replacement `/ingester/shutdown`.
- **PR #10793** ("loki better defaults"): the origin of the default-change table.
- **Issue #12529**: a real report of `field shared_store not found in type compactor.Config` after
  upgrading, with a comment naming `enforce_metric_name` as another removed key that broke it.
  Issue #12506 is the upstream "Loki 3.0 Feedback and Issues" thread linked by the guide.

## Items and G22 categories

| Item | Class (item) | G22 category |
|---|---|---|
| E1 structured metadata on by default, needs tsdb + v13 | action-required (upgrade-blocked) | required new config; compatibility hidden in prose; changed default |
| E2 `shared_store` / `shared_store_key_prefix` removed (shippers + compactor) | action-required (upgrade-blocked) | removed config keys / CLI flags; migration (index location) |
| E3 `delete_request_store` required with retention | action-required (upgrade-blocked) | required new config |
| E4 already-deprecated options removed (bundle) | action-required (upgrade-blocked) | removals of deprecated keys/flags |
| E5 `max_transfer_retries` removed | action-required (upgrade-blocked) | removed config / CLI flag |
| E6 `/ingester/flush_shutdown` removed | action-required (workload-failure) | API removal; behaviour hidden in prose |
| E7 max line size default 256KB | review-required (behavior-change) | prose-only default (table) |
| E8 max label names per series 30 → 15 | review-required (behavior-change) | prose-only default |
| E9 cortex_ → loki_ metric prefix (`metrics_namespace`) | review-required (behavior-change) | behaviour change; new key with default; deprecation of the escape hatch |
| E10 `-legacy-read-mode` default true → false, deprecated | action-required (workload-failure) | changed default stated only in a table row / CHANGELOG; deprecation |

No cross-product version dependency was stated upstream for this edge (the Helm chart 6.x guide is
a separate chart concern), so there is no `product-relationship` item.

## Judgement calls

- **Removed keys are upgrade-blocked, not setting-ignored.** Loki's first config pass is strict
  (`dynamic.go`), and issue #12529 shows the resulting startup failure. A removed CLI flag fails Go
  flag parsing the same way. That is why E2, E4 and E5 use `upgrade-blocked`.
- **E2 in this environment**: `shared_store: s3` equals every period's `object_store: s3`, so the
  guide's "no additional changes are required besides removing the usage of the deleted
  configuration option" applies. The action is still required because the keys must go. There is
  no index-relocation migration in this fixture.
- **E3** is a separate item rather than part of E2: it is a *now-required* key with its own
  validation error. The guide gives it its own subsection.
- **E4 bundles 13 subjects** because upstream lists them as one numbered section. The last bullet,
  "compactor CLI flags that use the prefix `boltdb.shipper.compactor.`", names a prefix, not a
  subject, so it is not labelled. `frontend.cache-split-interval`, `ruler.wal-cleaer.period` and
  `experimental.ruler.enable-api` are labelled `cli-flag` because upstream names them only as flags.
  Upstream says `querier.worker-parallelism` "and its corresponding yaml setting", which is
  `frontend_worker.parallelism` in the 2.9.6 reference. Its exposure pattern leaves out
  `parallelism:` because the same key exists under memcached. `split_queries_by_interval` is also
  left out: the environment legitimately sets it under `limits_config`, and a line regex cannot tell
  the two parents apart.
- **E7/E8 are review, not action.** Whether a 256KB line or a 16th label occurs depends on the
  traffic. The canonical exposure for a changed default is "the key is unset". So E8 is REVIEW for
  this environment (key unset), and E7 is INFORMATIONAL (the key is pinned to 512KB, which is the
  overlap).
- **E9** is review. The PrometheusRule makes the effect concrete: the `cortex_ring_members` alert
  silently stops matching. You could argue that losing an alert is "loss of intended behavior"
  (action). I kept review because upstream frames it as a rename with an escape hatch and the rule
  still evaluates.
- **E10** is action for an exposed (two-target read/write) deployment, because backend components
  stop running. This environment runs `-target=all`, so it is **not-affected**. This is the second
  clear not-affected link, next to E5 (the ingester block exists and the WAL is on, but there is no
  `max_transfer_retries`).
- **E6 is undecided** (`runtime-behavior-gap`). The fixture proves that no in-cluster workload calls
  the endpoint (there is no preStop hook). The description states that ingester drains run from an
  external runbook script that is not part of the fixture. Whether anything still POSTs
  `/ingester/flush_shutdown` is live-traffic knowledge. The exposure is
  `any[resource-field false, undecidable]`, which is unknown under Kleene logic.
- **Item `classification` vs link `relevance`.** Following the brief, `classification` is the
  item's generic class (the consequence's exposed class), and `relevance` is the truth for this
  cluster. They differ for E5 and E10 (action-required vs not-affected), E7 (review-required vs
  informational) and E6 (action-required vs undecided). If classification scoring compares against
  the environment's observed class, a correct engine would be scored "wrong" on those four. The lane
  should decide which convention to apply.
- **Excluded**: `-ruler.alertmanager-use-v2` (3.2.0; recorded in `notExpected`). Also excluded: the
  remaining default-table rows (querier.max-concurrent, split-queries-by-interval, cache sizes and
  so on), which are tuning changes. Metric renames other than the namespace (distributor, embedded
  cache), the write-dedupe-cache deprecation, the runtime-overrides `default` section removal and
  `use_boltdb_shipper_as_backup` are real but minor, and leaving them out keeps the case at 10
  items. A tool that reports them is not wrong. They are simply not in `expected`.

## Environment fixture (grounding)

- `manifests/loki-config.yaml`: a 2.9.6-valid single-binary config. Every key was checked against
  the v2.9.6 configuration reference. S3 access uses IRSA, so there are no credential lines, and no
  line matches the sensitive-line regex. That keeps `text-line none` decidable.
  - The two schema periods (boltdb-shipper/v11, then tsdb/v12) make the active period tsdb but v12.
    That is exactly the shape the v3.0.0 `validateSchemaValues` rejects.
  - Leftovers that are common in 2.x-era configs are kept: `max_look_back_period: 0s` (next to its
    replacement `max_query_lookback`) and `enforce_metric_name: false`.
- `manifests/loki-statefulset.yaml`: image `grafana/loki:2.9.6`, args `-config.file`, `-target=all`
  and `-log.level`, and no lifecycle hooks.
- `manifests/prometheusrule.yaml`: an alert on `cortex_ring_members`, which the guide lists among
  the renamed metrics.
- `inventory.yaml`: loki 2.9.6 and prometheus-operator (no version), both stated in the description.

## Semantic labels

- **Config-file keys** use `config-key` with `component: loki` and the full YAML path from the config
  root (`limits_config.allow_structured_metadata`, `storage_config.tsdb_shipper.shared_store`), so
  identical leaf names under different parents stay distinct (the shipper vs compactor
  `shared_store`). `replacedBy` paths use `[]` for the period list
  (`schema_config.configs[].object_store`, `schema_config.configs[].index.path_prefix`).
- **CLI-only settings** use `cli-flag` with the single-dash spelling upstream uses
  (`-legacy-read-mode`, `-validation.allow-structured-metadata`). Conditions use the same spelling.
- **E1** is labelled `default-changed false → true` on `allow_structured_metadata`, not
  `now-required` on the schema. The default flip is what turns an untouched 2.9 config into a
  startup failure, and the shield (set the key to false) is expressible.
  - The exposure approximates "active period is not v13" as "no `schema: v13+` line anywhere". In
    this fixture both are true because no v13 period exists.
  - A config with a future-dated v13 period would need date reasoning (`ActivePeriodConfig`), which
    the condition language cannot express. That is noted here and not exercised.
- **E9** is `added` (`metrics_namespace`, after `"loki"`). In 2.9.6 the key does not exist, so the
  user-visible effect (cortex_ → loki_) comes from the new key's default.
- **E6** uses `api-endpoint` with method + path (`POST /ingester/flush_shutdown`; 2.9.6 registers it
  POST-only), `component: ingester`, and `replacedBy` `POST /ingester/shutdown`.
- Anchored line patterns (`^\s*key:`) keep `max_look_back_period` from matching
  `query_store_max_look_back_period`, and `shared_store(_key_prefix)?:` from matching unrelated keys.
