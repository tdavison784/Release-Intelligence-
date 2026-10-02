# Transfer environment notes — strimzi 0.45.0 → 0.46.0, edge-retail

Authored blind on 2026-10-01 from upstream sources only (no `ri` run, no
pipeline output, no base-case edits). This is a second, independently designed
cluster for the same transition as `strimzi-0.45-0.46`; the expected items and
their `semantics` are inherited from the base case.

## Cluster design

A regional retail-edge platform (small single-cluster footprint, one
datacentre it mirrors from) on Kubernetes 1.31. It is deliberately a
different shape from the base case's `prod-kafka`:

| Dimension | base case (prod-kafka) | edge-retail |
|---|---|---|
| Kafka mode | KRaft, no opt-in annotations shown | KRaft, 0.45-era opt-in annotations on the Kafka CR |
| Storage | one KafkaNodePool, jbod, fast-ssd | one dual-role pool, persistent-claim, local-nvme |
| Replication | MirrorMaker 2 (dc-failover) 3.8.0 | MirrorMaker 1 (legacy-price-feed) 3.9.0 |
| Kafka version | unset (0.45 default 3.9.0), MM2 pinned 3.8.0 | pinned 3.9.0 everywhere |
| Authorization | none | OPA (`type: opa`) |
| Connect providers | none | Strimzi EnvVar provider class in `spec.config` |
| Exposure | none | nodeport external listener w/ advertised hosts |
| Logging | external log4j.properties ConfigMap | external log4j.properties ConfigMap (Reload4j-era) |

A lagging 0.45 cluster still running MirrorMaker 1 and the Strimzi EnvVar
provider is exactly the population the upstream changelog warns about
("Strimzi 0.45 is the last minor Strimzi version with support for
ZooKeeper-based Apache Kafka clusters and MirrorMaker 1 deployments", and
"Strimzi 0.45 is the last Strimzi version to include the Strimzi EnvVar
Configuration Provider … and Strimzi MirrorMaker 2 Extensions"), so the
fixture is realistic, not synthetic.

## Grounding (all quotes verbatim from upstream)

- 0.46.0 CHANGELOG, "Major changes, deprecations and removals": ZooKeeper/KRaft
  migration removal ("Please make sure all your clusters are using KRaft
  before upgrading to Strimzi 0.46.0 or newer!"), MirrorMaker 1 removal,
  EnvVar provider / MM2 extensions removal from images ("Please use the
  Apache Kafka EnvVarConfigProvider … instead"), storage overrides removal,
  OPA deprecation, `statefulset.kubernetes.io/pod-name` label removal ("If
  you have any custom setup leveraging such label, please use the
  `strimzi.io/pod-name` one instead"), Kafka 4.0 Log4j2 switch.
- kafka-versions.yaml at 0.45.0 (supported: 3.8.0, 3.8.1, 3.9.0; default
  3.9.0) and at 0.46.0 (supported: 3.9.0 non-default, 4.0.0 default).
- 0.46 docs, assembly-kraft-mode.adoc: "The `Kafka` resource using KRaft mode
  must also have the annotations `strimzi.io/kraft: enabled` and
  `strimzi.io/node-pools: enabled`." — grounds the E1 evidence (the
  annotations are how a 0.45 Kafka CR declares KRaft).
- 0.45 docs, con-config-storage-zookeeper.adoc "Migrating from storage class
  overrides (deprecated)": the deprecated `overrides` lived under
  `spec.kafka.storage` in the `Kafka` resource; node pools with their own
  storage class replace them. 0.46 docs, con-config-storage-kraft.adoc:
  storage is configured through `KafkaNodePool` `storage` properties.
- 0.45 docs, proc-loading-config-from-env-vars.adoc shows the provider wired
  as `config.providers: env` + `config.providers.env.class` inside
  `spec.config`; the plugin README (kafka-env-var-config-provider) gives the
  removed Strimzi class `io.strimzi.kafka.EnvVarConfigProvider` and the
  replacement `org.apache.kafka.common.config.provider.EnvVarConfigProvider`.
- CRD diff across the tags: `045-Crd-kafkamirrormaker.yaml` exists in
  packaging/install/cluster-operator at 0.45.0 and is absent at 0.46.0 (where
  `045-Crd-kafkanodepool.yaml` takes the slot). The 0.45 Kafka CRD still
  lists `overrides` in storage and `opa` in the authorization type enum —
  schema presence is not support, which is why the links key on use.
- 0.46 docs, con-upgrade-versions-and-images.adoc: "`Kafka.spec.kafka.version`
  … defaults to the latest supported Kafka version … if not specified" — why
  this cluster pins 3.9.0 explicitly (an unpinned Kafka CR would follow the
  operator default to 4.0.0 on 0.46).
- Kubernetes 1.31: the changelog only fixes a floor ("From Strimzi 0.44.0 on,
  we support only Kubernetes 1.25 and newer"); 1.31 is comfortably in range.

## Link judgements

- **E1 not-affected.** KRaft with the 0.45 opt-in annotations, no ZooKeeper,
  no in-flight migration. Exposure is the negated kraft-enabled annotation:
  `not(resource Kafka metadata.annotations["strimzi.io/kraft"] = enabled)` —
  true only for a pre-KRaft (ZooKeeper-based) Kafka CR; here the annotation
  is present, so the exposure is false → not-affected. The pool resource is
  cited as corroboration (KafkaNodePool in use ⇒ storage/replicas managed
  outside the Kafka CR).
- **E2 action-required.** `gvk-in-use` of `KafkaMirrorMaker` (canonical shape
  for `gvk · removed`); consequence is exactly the base statement — MM1
  replication stops being operated. Migrating to MM2 is the remediation.
- **E3 not-affected.** Canonical `crd-field · removed` exposure
  (`spec.kafka.storage.overrides` set); the Kafka CR has no `spec.kafka.storage`
  at all — storage lives in the pool, single class `local-nvme`, which is
  already the suggested layout.
- **E4 action-required.** `spec.config["config.providers.env.class"] =
  io.strimzi.kafka.EnvVarConfigProvider` on the KafkaConnect — the plugin is
  gone from 0.46 images, so the config fails to load it (exposedClass
  action-required). Only the EnvVar-provider subject of the item applies; no
  KafkaMirrorMaker2 exists, so the MM2-extensions subject is untouched (and
  noted in `why`).
- **E5 not-affected.** All operand version pins are 3.9.0 and 0.46 still
  supports 3.9.0 (kafka-versions.yaml at 0.46.0). Exposure mirrors the base
  item's three-branch any (Kafka / KafkaConnect / KafkaMirrorMaker2
  `spec.version` matching `^3\.8\.`); all branches are false here (no MM2
  resource, the other two pinned 3.9.0). The MM1's own `spec.version` is also
  3.9.0, but that resource's fate is decided by E2.
- **E6 review.** `spec.kafka.authorization.type: opa` is set with a real OPA
  URL — deprecation only (the type still works in 0.46), so relevance follows
  the consequence's exposedClass `review-required`: plan the move to
  `type: custom`, nothing breaks at upgrade time.
- **E7 undecided (environment-visibility-gap).** The nodeport external
  listener means per-broker external Services that carried the removed label
  exist; consumers of `statefulset.kubernetes.io/pod-name` (LB target pools,
  firewall rules, scrape configs, scripts) live outside cluster manifests, so
  neither an affected nor a clean verdict is defensible from the fixture. A
  correct engine should emit neither.
- **E8 informational.** Custom external logging IS in use (the ConfigMap
  carries `org.apache.log4j` appender lines), so the subject is touched — but
  the brokers are pinned to 3.9.0, which remains supported, and the
  Reload4j→Log4j2 switch ships with Kafka 4.0; the overlap (version pinned
  3.9.0) shields the cluster through this operator upgrade → informational,
  "applies to you, you appear safe (until you move to Kafka 4.0)".

## Findings expectations

- F1 (`impact:crd-removed`): grounded in the same-tag CRD diff listed above;
  the fixture's `crds/` carries the 0.45 KafkaMirrorMaker CRD extract.
- F2 (`impact:image-changed`): the mirror list pins
  `quay.io/strimzi/operator`, whose tag moves 0.45.0 → 0.46.0.

## Observations on the base case (not changed)

- The base environment leaves `spec.kafka.version` unset while carrying a
  Reload4j-era logging ConfigMap, and links no E8. Per the 0.46 upgrade docs,
  an unset version follows the operator default, which becomes 4.0.0 on
  0.46 — so on that cluster the Log4j2 switch (E8) plausibly becomes
  review-required/action-required rather than unlinked. Flagged to the lane
  lead; the base case is untouched.
- E5's base classification comment ("action-required … generically
  review-required") is inherited as-is; on this environment the item is
  linked not-affected, which the base note already anticipates as generic.
