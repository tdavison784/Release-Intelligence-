# Validation report — first full dataset run

`ri eval` over the whole dataset (`eval/cases`), run live on 2026-10-01
against the real upstreams, after the ground truth was authored blind from
those upstreams (per-case research notes in each `NOTES.md`; every
expectation cites its source). No numbers below were produced by the
pipeline itself: the pipeline was the thing measured.

Method reminder: an expected item counts as **found** when at least one
generated Change matches one of the item's matchers (text regex over
title+detail, exact subject, category, release, evidence URI, flags); every
hit is recorded with the change id, the matcher that fired and a
human-openable evidence URI (`-o json`, or the text output). A **false
positive** is a Change matching a `notExpected` entry — output an operator
would not want in an upgrade brief, not merely output beyond the must-find
list. **Recall** = found/expected; **precision** = matched/(matched+false
positives). The stored snapshots of this run are committed under
`eval/results/`; the next run diffs against them and exits non-zero on any
regression.

## Bottom line first

| | |
|---|---|
| entries | 9 (8 products), 0 pipeline errors |
| expected items | 53 |
| found | **50 (recall 0.94)** — missed 0 critical, 2 important, 1 minor |
| changes generated | 1695, of which 97 covered ground truth (coverage 0.06) |
| false positives | **103 (precision 0.48)** |
| duplicate groups | 7 |
| unsupported conclusions | **0** — every conclusion's evidence resolves |
| environment entries | 2: expected findings **7/7**, false findings **0**; expected-impact links **1/4** (accuracy 0.25) |

The one-line honest summary: **the pipeline finds the upgrade work (94 %,
100 % of critical items), every conclusion is evidence-backed, and the
environment join is exact about what it can see — but a human reading the
full output wades through roughly one unit of noise per unit of signal, and
the join cannot see the ways an environment is affected by what it does NOT
set.** No cherry-picking: the worst number here (precision 0.48, link
accuracy 0.25) is as real as the best (recall 0.94, unsupported 0).

## Per entry

| entry | product | transition | recall | FP | dups | notes |
|---|---|---|---|---|---|---|
| argo-cd-3.0-3.1 | argo-cd | v3.0.6 → v3.1.0 | 5/5 | 8 | 0 | all guide items found, incl. the GHSA-driven API sanitisation |
| cert-manager-1.15-1.16 | cert-manager | v1.15.4 → v1.16.0 | 4/5 | 0 | 1 | 1 miss: OperatorHub (see below) |
| cert-manager-1.17-1.18 | cert-manager | v1.17.0 → v1.18.0 | 4/4 | 0 | 2 | env: findings 3/3, links 0/2 |
| cilium-1.16-1.17 | cilium | v1.16.1 → v1.17.0 | 11/11 | 16 | 2 | widest must-find list; env: findings 4/4, links 1/2 |
| ingress-nginx-1.11-1.12 | ingress-nginx | controller-v1.11.5 → v1.12.0 | 4/4 | 14 | 0 | all four ⚠️ release items found |
| istio-1.28-1.29 | istio | 1.28.3 → 1.29.0 | 7/7 | 2 | 1 | every upgrade-notes section found |
| kube-prometheus-stack-90-91 | kube-prometheus-stack | 90.2.0 → 91.0.0 | 3/5 | 0 | 1 | both misses are operator-level (see below) |
| postgresql-17-18 | postgresql | REL_17_4 → REL_18_0 | 6/6 | 8 | 0 | whole migration section found in the SGML notes |
| vault-1.21-2.0 | vault | v1.21.4 → v2.0.0 | 6/6 | 55 | 0 | worst precision; see FP analysis |

## The three misses

1. **cert-manager-1.15-1.16, E4 "OperatorHub packages discontinued"
   (important)** — *upstream doc shape changed, definition does not follow.*
   The item lives under `## Themes` in `release-notes-1.16.md`, but the
   definition's `website-major-themes` source extracts a section headed
   `^Major Themes$` — the heading the cert-manager website uses from 1.17
   on. `ri ingest cert-manager v1.16.0` reports the source `not-found`, so
   the whole Themes section (OperatorHub included) never became notes. This
   is a genuine, previously-unknown definition gap the dataset caught on its
   first run; the fix is a heading alternative in `products/cert-manager.yaml`
   (left to the definition owners — this branch does not touch `products/`).
2. **kube-prometheus-stack-90-91, E4 "Alertmanager zero-value duration
   fields discarded / IgnoredFields" (important)** — *upstream document the
   pipeline does not read.* The operator's v0.94.0 release notes (the
   `[CHANGE]` items) are not a declared source of the chart product; the
   chart's own UPGRADE.md section mentions the operator bump, the tightened
   RBAC and the CRD procedure, but not the operator's behavioural changes.
   The chart edge produced only 6 changes in total.
3. **kube-prometheus-stack-90-91, E5 "ScrapeConfig CRD rejects empty
   strings" (minor)** — same cause as E4.

Categories: 0 classification bugs, 0 expectation-too-strict misses, 1
source-shape gap, 2 source-coverage gaps. That distribution is the point of
the exercise: the evaluator's misses point at ingestion, not at
classification.

## False positives (103) — where the noise comes from

| product | FP | shape | cause |
|---|---|---|---|
| vault | 55 | per-plugin version bumps (28), `ui:` items (27) | the CHANGELOG is ingested whole; plugin bumps and UI churn are indistinguishable from server changes by text alone |
| cilium | 16 | metrics renames / added metrics | upstream lists them in the same "1.17 Upgrade Notes"; the dataset says (and we stand by it) that a metrics rename is not upgrade work — but the pipeline has no signal to separate them |
| ingress-nginx | 14 | `Images:`/`CI:`/`Go:`/`Docs:` commit entries | the GitHub release body is a conventional-commit firehose; the ⚠️-marked entries are extracted correctly, the rest flows through |
| postgresql | 8 | new performance features, new system-view columns | SGML release notes interleave incompatibilities with features; the ingestion keeps both |
| argo-cd | 8 | "Added Healthchecks" list entries | feature content inside the upgrade guide |
| istio | 2 | InferencePool feature notes | feature notes shipped alongside the upgrade notes |

None of these are *wrong statements* — each cites real upstream content.
They are wrong *for an upgrade brief*. The pattern: products whose notes
channels mix operator-relevant and development content (Vault, ingress-nginx,
PostgreSQL) get precision 0.17–0.64; products with dedicated upgrade-guide
channels (cert-manager, kps) get 1.00. Separating "true statement" from
"upgrade-relevant statement" without losing the 3 real misses is the open
classification problem this dataset now measures.

## Duplicate conclusions (7 groups)

Detected by identical normalized title, identical category+subject set, or
≥ 0.75 title-token overlap on the same subjects. Two shapes:

- **Multi-source restatement** (expected and mostly benign): cert-manager
  states RotationPolicy/RevisionHistoryLimit changes in both the upgrade
  guide and the release notes; cilium's metallb-bgp removal appears in the
  upgrade notes, the CHANGELOG and the values diff. A human reads the same
  fact 2–3 times.
- **Computed CRD diff fragmentation** (the actionable one): the
  subject-set detector fires on pairs of computed `crd-schema` changes that
  share giant subject sets (cert-manager `spec.acme...podTemplate.*`,
  cilium `cidrGroupSelector` trees, kps probe/lifecycle fields). The CRD
  differ splits one logical schema addition across subject subsets, which
  both inflates the change count and makes the remaining "coverage 0.06"
  look worse than the user experience (a rendered report groups these).

## Unsupported conclusions: 0

For all 1695 changes and all environment findings, every cited evidence id
resolves inside its document and findings join real changes. (This is
weaker than semantic support — the evaluator cannot judge whether an
excerpt *backs* a title — but the structural claim audit passes everywhere.)

## Environment impact (2 entries)

Expected findings: **7/7, zero false findings.** Every pinned/removed/new
values key surfaced with the right rule and classification
(cert-manager: `targetPort` pinned informational, new
`disableHTTPChallengesRole`, Kubernetes 1.31 in-range; cilium: removed
`bgp.enabled`, deprecated `tls.secretsBackend`, pinned
`certValidityDuration`, pinned `endpointMaxIpPerHostname`), and the
forbidden `crd-removed` finding correctly never appeared (CRD sets are
identical at both endpoints — verified against the repo trees).

Expected-impact links: **1/4.** The three misses are the instructive part:

- **cert-manager E1 (rotationPolicy) and E3 (PathType/ingress-nginx), review
  / action-required — the join is blind to absence and to context.** The
  fixture's Certificates do not *set* `rotationPolicy`, so nothing in the
  files matches; the HTTP01/ingress-nginx interaction depends on the
  ingress controller's version, which no cert-manager environment input
  carries. Both are real "this environment is affected" truths a human
  derives from the upgrade brief plus a glance at the repo; the join's rule
  set (values keys, apiVersions, CRDs, images, cluster version) has no rule
  that can fire. Impact accuracy 0.25 is the honest number for that.
- **cilium E3 (metallb-bgp) — expectation phrasing, not a pipeline gap.**
  The join DID fire (`impact:values-removed` on `bgp.enabled`, subject
  matched), but it joins the computed diff change ("You set 3 Helm values
  that v1.17.0 removed") whose text does not contain the upstream words
  E3's matchers look for (`metallb-bgp`, `bgpControlPlane`), so the link
  check missed while the finding-level expectation (F1) hit. Kept as
  authored — tightening the matcher after reading pipeline output would
  make the dataset tautological; recorded here instead. Link-level
  expectations in future cases should include subject matchers.

## What this run already changed

The dataset's first live run found one real definition gap (cert-manager
`Major Themes` heading, above) and one real source-coverage gap
(kube-prometheus-stack: the operator's own release notes are not a declared
source). Both are recorded here for the definition owners; `products/` is
untouched by this branch, so the misses stay measurable until they are
fixed — at which point `ri eval` will show them as improvements against
`eval/results/`.

## Reproducing

```
ri eval                 # live run, diff against eval/results, exit 1 on regression
ri eval -o json         # the same numbers, machine-readable (audit trail per hit)
ri eval -update         # after review: rewrite the snapshots, never case.yaml
```

Offline: `go test ./internal/eval` replays the recorded cert-manager entry
against the e2e fixture and re-checks the loader and scoring; the full
dataset needs network.

## Post-baseline fix round (2026-10-01)

`cert-manager-1.15-1.16` E4 (OperatorHub discontinued) is fixed by a new
`website-breaking-changes` source scoped to `>= 1.16.0, < 1.17.0`: only that
minor's notes file carries a dedicated `## Breaking changes` section, and the
item lives there — not under Themes as first reported. The section also
repeats the Helm/Venafi breaking bullets verbatim from the upgrade guide, so
duplicate groups rise 1 → 4; both facts are kept (different evidence URIs)
and cross-role consolidation is the enrichment layer's job (Goal 5), not a
reason to hide upstream redundancy. Aggregate after the fix: recall 0.96,
precision 0.48, duplicate groups 10 (was 7), env accuracy 0.25. The remaining
important miss is `kube-prometheus-stack-90-91` E4, which needs sources to
follow a pinned component's version (new construct; also wanted by Flux for
per-controller notes at their pinned tags).

## Pinned-component sources close the last misses (2026-10-01)

The standing `kube-prometheus-stack-90-91` misses (E4, E5 — the operator's
behavioural changes, which exist only in the operator's own release notes at
the pinned tag) are fixed by a new generic construct: **source locators and
extract templates can follow another artifact's resolved version** with
`{{.ArtifactVersionOf "prometheus-operator-image"}}`. The `version.strategy:
field` artifact already read the operator pin (Chart.yaml `appVersion`, the
`v`-prefixed operator tag format since 44.0.0); the construct renders that
resolved pin into a `github-releases` ref, so
`operator-release-notes` reads exactly the notes of the operator version the
chart ships (v0.94.0 at chart 91.0.0). Classify rules declare the operator's
own bracket taxonomy (`[CHANGE]` → api/breaking — the class an upgrader must
review; `[FEATURE]`/`[ENHANCEMENT]` → feature; `[BUGFIX]` → bugfix), which
the generic classifier does not know. When the pin does not resolve for a
release, the source is skipped/unavailable with the pin's reason — never a
wrong tag. Validation rejects references to unknown artifact ids, artifacts
whose version strategy is not derivable from the release (lookup,
independent), and any use outside source templates (no circularity with
artifact version resolution by construction).

Numbers for `kube-prometheus-stack-90-91`: found 3/5 → **5/5** (E4 and E5
now match changes whose evidence is
`github.com/prometheus-operator/prometheus-operator/releases/tag/v0.94.0` —
exactly the citations the case was authored from), false positives 0 → 0,
duplicate groups 1 → 1 (the operator items do not duplicate the chart's own:
E2's wildcard-verbs change keeps two distinct wordings from two channels),
changes 6 → 21 (the 15 v0.94.0 release-body items, now classified instead of
unread). Aggregate over the 9 entries: recall **0.96 → 1.00** (51/53 →
53/53, zero misses at every importance), precision 0.48 → 0.49, false
positives 103 → 103 (the new source introduced none), duplicate groups 10 →
10, unsupported 0, environment accuracy 0.25 (unchanged). The dataset now
has no known miss; the open quality problem is precision (Vault,
ingress-nginx, Cilium noise), not coverage.

## Enrichment experiment: does AI consolidation move brief precision? (2026-10-01, GLM-5.3-Flash via Z.AI)

First live-model run of `ri upgrade -enrich` (Z.AI's Anthropic-compatible
endpoint, `glm-5.3-flash`, thinking disabled, ~40 prompts on the free quota;
every answer schema-validated, full provenance recorded, `.ri/llm-cache`
holds the answers for offline replay). Infrastructure: 0 failures — the
gateway needed two generic client tolerances (fenced-JSON extraction;
explicit `thinking: disabled`), both landed with tests.

**Measurement** (the three FP-dominant cases; FP = change matching a
`notExpected` list; "consolidated" = covered by ≥1 accepted enrichment's
`relatesTo`):

| case | FPs | consolidated | accepted enrichments |
|---|---|---|---|
| ingress-nginx-1.11-1.12 | 14 | 6 (43%) | 6 |
| cilium-1.16-1.17 | 16 | 0 | 6 |
| vault-1.21-2.0 | 55 | 0 | 7 |
| **total** | **85** | **6 (7%)** | 19 |

**Finding: as designed, enrichment does not move upgrade-brief precision.**
The candidate generator produces duplicate-statement clusters (the same fact
in two changelog sections — its Goal-5 job, done well), not routine-maintenance
buckets: Vault's 12 candidate groups over 167 changes are all 2–3-item
semantic pairs, while its 55 FPs are single per-plugin version bumps and
UI-only items that never become candidates. Raising `-enrich-max` cannot help
(12 << 40 cap). The precision gap is therefore a missing capability, not a
model-quality issue: it needs (a) deterministic routine-maintenance
classification (dependency-bump/UI-churn patterns as classify categories or
generic heuristics), and/or (b) candidate groups over same-category masses
with an enrichment kind that demotes them to a one-line "routine maintenance"
conclusion. The 43% on ingress-nginx shows consolidation works where FPs are
duplicates of each other.

## Deterministic routine classification closes the structural gap (2026-10-01)

The missing capability (a) above is now in the pipeline:
`internal/upgrade/routine.go` marks note-derived Changes `routine` when they
are *bare* maintenance statements — dependency bumps ("Bump x from 1.2.3 to
1.4.5", "auth/x: Update plugin to v0.23.1", SHA-prefixed `chore(deps):` firehose
entries), UI-only work (`ui:`), CI/docs/test/build churn (`docs:`, "Images:
Bump/Drop/Build …"), and observability-metric renames/deprecations. Routine
changes stay in the edge with their evidence (`routine`, `routineKind` per
change; a `routine` count+breakdown on the edge; a dim "Routine maintenance"
section in the brief) — they only leave the default
breaking/action/migration narrative. Safety first: breaking, security
(category, cited CVE/GHSA, vulnerability wording), operator directives,
feature-flag removals and non-bare bump statements ("This version upgrades
Prometheus-Operator to v0.94.0; the ClusterRole no longer grants wildcard
verbs" — caught by the recall gate before it shipped) are never routine;
computed diffs are never eligible. No product-specific patterns, no LLM.

Measured against the stored false positives (replayed offline; scoring
semantics untouched — `ri eval -offline` reproduces recall 1.00, precision
0.49, FP 103, "no regressions"):

| case | stored FPs | routine | missed, why |
|---|---|---|---|
| vault-1.21-2.0 | 55 | 53 (96%) | `ui: disable scarf analytics` — upstream files it under a "Security" heading, so the security carve-out keeps it; `secrets/azure: Update plugin to v0.25.1+ent` carries a behaviour note beyond the bump ("Improves retry handling …"), so the bare-bump rule keeps it |
| cilium-1.16-1.17 | 16 | 16 (100%) | — (all 16 are metric renames/deprecations) |
| ingress-nginx-1.11-1.12 | 14 | 14 (100%) | — |

The verdict on Cilium's metrics renames, taken deliberately: they classify
routine. A rename does not change how the upgrade runs — dashboards pointing
at old names are monitoring maintenance, which is what the dataset's
`notExpected` entry already says. The items keep their detail and their
`metrics` kind in JSON, so the loss is presentational, not informational; and
the carve-outs still hold the line where a metric change is *breaking*.

The recall-preservation gate (`go test ./internal/eval`, offline, fixtures =
the trimmed changes/evidence of the stored edges): for every one of the 9
dataset entries, no expected item has all of its matching changes classified
routine — with the argo/kps/env cases included. Zero expected loss. The
routine flag on changes is the hook for a routine-adjusted precision metric
in a later round: 83 of the 103 stored false positives are routine-marked
(vault 53, cilium 16, ingress 14), so excluding them would move brief
precision from 0.49 (100/(100+103)) to ≈ 0.87 (100/120) — but that is a
metrics decision, not this round's.

---

# Phase 3, Goals 8–12: the expanded dataset, classification scoring, honest metrics, hard gates, adversarial pack (2026-10-01)

This round grows the dataset from 9 to **17 cases (115 expected items)**, adds
the classification axis to every expectation, scores runs against it
(confusion matrix with severity weighting), replaces the single precision
number with the honest metric set, installs the pre-registered hard gates,
adds a sample-based human adjudication layer, and ships an adversarial
environment pack that tries to fool the join offline.

## Dataset composition (G8)

Eight new cases, each authored blind from the cited upstream documents (per
case: `NOTES.md` records sources and judgement calls) and, where the join
vocabulary supports it, carrying a reconstructed pre-upgrade environment
fixture:

| new case | product | transition | env fixture | expectations | change types covered |
|---|---|---|---|---|---|
| cert-manager-1.16-1.17 | cert-manager | v1.16.0 → v1.17.0 | values + manifests (RSA-4096 CA hierarchy, ValidateCAA pin) | 5 | breaking behaviour (signature hashes), feature-gate deprecation, gate promotion defaults, CRD field addition, log-format change |
| cilium-1.15-1.17 | cilium | v1.15.6 → v1.17.0 | values + images (two-hop value drift) | 9 | **multi-hop upgrade** (two upgrade notes in one edge), version-gated path, CRD field removal, API version move, Helm default change, integration removal, deprecation lifecycle across hops |
| argo-cd-2.14-3.0 | argo-cd | v2.14.5 → v3.0.0 | manifests (RBAC CM, ApplicationSet) | 9 | **mandatory migration guide** (nine breaking changes with detection/remediation), default-config change, removed metrics/repo channel, dependency (Helm 3.17.1) |
| karpenter-0.37.8-1.0.0 | karpenter | v0.37.8 → v1.0.0 | values + manifests (v1beta1 NodePool/EC2NodeClass) + images | 10 | **CRD/API migration** (v1beta1→v1 with conversion webhooks), API renames, removed annotations/taints, schema tightening (required fields), IMDS default, env-var drops, compatibility matrix |
| strimzi-0.45-0.46 | strimzi | 0.45.0 → 0.46.0 | manifests + CRDs + images | 8 | **architecture removal** (ZooKeeper, MirrorMaker 1 incl. its CRD), storage-override removal, plugin removals, Kafka support window, OPA deprecation |
| istio-1.23-1.24 | istio | 1.23.4 → 1.24.0 | values + manifests (ambient + telemetry CR) | 7 | **ordered upgrade procedure**, third-party compatibility (istio-csr/ALPN), chart replacement, Helm-managed CRDs, conflict-resolution behaviour, telemetry attribute migration |
| postgresql-17.2-17.3 | postgresql | REL_17_2 → REL_17_3 | — | 5 | **security remediation** (CVE-2025-1094), behavioural reverts, tzdata bump, upgrade-blocking extension fix |
| terraform-provider-aws-5.99-6.0 | terraform-provider-aws | v5.99.1 → v6.0.0 | — | 9 | **breaking behavioural changes** (state/output semantics), attribute removals, default change, validation tightening, provider deprecations |

Dataset totals: **17 cases, 115 expected items** (28 critical, 70 important,
17 minor), 8 with environment fixtures, 21 environment-impact links, 19
expected findings. Every expectation carries at least one upstream citation
with a quote; per-expectation `classification` follows
docs/ACTION_CLASSIFICATION.md and `notExpected` entries may carry the class
such output must NOT have carried.

## Headline numbers (deterministic run, stored under eval/results)

| metric | value |
|---|---|
| pipeline failures | **0** (execution errors counted separately; none occurred) |
| recall | **1.00** (115/115) — criticalRecall **1.00** (28/28), importantRecall **1.00** (70/70) |
| raw precision (deprecated `precision`) | 0.64 (211 matched, 117 notExpected-matched) |
| labeledPrecision (adjudicated) | **0.57** (189 true / 140 false across the adjudicated output) |
| falseActionRate | **0.00** (0 of 3 ACTION findings wrong) |
| actionFindingEvidence | **1.00** (3/3 ACTION findings with fully-resolving two-chain provenance) |
| classificationAccuracy | **0.46** (26/57 observable class expectations matched) |
| unknownRate | **0.74** (findings that honestly say "cannot tell") |
| applicabilityAccuracy | **0.10** (2 of 21 claimed environment links reach the environment) |
| evidenceCoverage | **1.00** (every matched change cites resolving evidence) |
| duplicateRate | 0.03 (47 groups; mostly the known multi-source restatement + CRD-diff fragmentation shapes) |
| unsupported | **0** |

**Recall is perfect and every conclusion is evidence-backed; the honest
failures are applicability and classification.** The join reaches this
environment for 2 of the 21 links the fixtures claim (both Cilium cases: a
removed values key and a changed image), leaves 74% of findings at UNKNOWN,
and answers "not-affected" where the fixtures say work is required — the
confusion matrix below quantifies exactly that.

## Confusion matrix (G9)

Expected class × actual class over labelled units. A unit is labelled by the
`expectedImpact` relevance of the expected item its change belongs to, or by
an `expectedFindings` classification clause; the actual class is the
strongest class among the findings joining that change (one cell per
expected-item × change pair — counting every finding would let a 25-key
values-section removal manufacture dozens of cells out of one judgement).
47 cells across 8 environment cases:

```
expected\actual   ACTION REVIEW  INFO  NOTAFF UNKNOWN
ACTION                 0      0     2      5      28
REVIEW                 0      0     0      0       9
INFORMATIONAL          0      0     0      0       3
```

Weighted miss **221** under the pre-registered severity weighting
(docs/ACTION_CLASSIFICATION.md): ACTION → NOT AFFECTED ×10 (catastrophic),
ACTION → UNKNOWN and NOT AFFECTED → ACTION ×5 (serious), ACTION → REVIEW ×1
(tolerable but imperfect); the full table is in
`internal/eval/classification.go`.

Reading: **the join never invents required action** (0 cells of
NOT-AFFECTED→ACTION; falseActionRate 0), never waves away a needed check
silently — it says UNKNOWN (28+9+3 cells) — but when it does decide, it
decides "checked, clear" (5×) where the fixtures say the operator must act.
That asymmetry is the finding: the join's conservatism is safe but
under-informative, and its UNKNOWN rate (0.74) is the honest cost.

## The gate panel (G11)

`ri eval` now prints the gate panel and exits non-zero when a gate fails.
Thresholds live in `eval/gates.yaml` and are **pre-registered**: they were
fixed from the phase plan before the expanded dataset was scored (the
dataset landed first, then gates activated on the stored results). Failures
are findings; nothing in this round tunes the pipeline against them.

| gate | threshold | actual | verdict |
|---|---|---|---|
| criticalRecall | ≥ 0.95 | 1.00 | PASS |
| importantRecall | ≥ 0.90 | 1.00 | PASS |
| applicabilityAccuracy | ≥ 0.80 | **0.10** | **FAIL** |
| falseActionRate | < 0.05 | 0.00 | PASS |
| actionFindingEvidence | ≥ 1.00 | 1.00 | PASS |
| unsupported | ≤ 0 | 0 | PASS |
| pipelineFailures | ≤ 0 | 0 | PASS |

**The applicability gate fails, and that is the round's headline finding.**
The dataset's 21 environment-impact links encode "this fixture is affected"
claims a competent operator derives from the upgrade brief plus the fixture
(a pin of a deprecated feature gate, an ingress controller below the fixed
version, a v1beta1 NodePool). The deterministic join — values keys, CRD/GVK
identity, images, cluster version — can see only 2 of the 21. The three
causes, in order of mass:

1. **Note-derived changes carry no comparable subjects** (the majority): a
   deprecated feature gate named in prose, an RBAC default flip, a taint
   rename — the join has no rule connecting them to a values key or manifest
   field, so the finding is UNKNOWN while the fixture says action-required.
2. **Cross-product context** (argo-cd's default exclusions meeting
   cert-manager, istio-csr's ALPN breakage): no input carries the other
   product's version, so applicability is genuinely undecidable from the
   supplied environment — the correct pipeline answer for these is UNKNOWN,
   and the dataset's link expectation is the *operator's* truth, not the
   join's reach. The distinction matters and the per-link `why` fields carry
   it.
3. **Compatibility as prose** (karpenter's upgrade warning, strimzi's Kafka
   window): constraint knowledge that never became a machine-readable
   compatibility record.

## Adjudications (G9)

`eval/adjudications/<case>.yaml` records 150 human verdicts: every
notExpected-matched change of the 9 FP-bearing cases plus a sample of
unmatched changes per new case, each read against the case's cited sources.
Effects, raw vs adjudicated:

- raw precision 0.64 → **labeledPrecision 0.57** (13 unmatched changes were
  adjudicated TRUE — real upgrade knowledge the must-find lists lacked, e.g.
  karpenter's "v1.0.0 is end-of-life since 2026-02", strimzi's
  upgrade-from-0.38 preconditions and the kafkaMirrorMaker values-section
  removal, aws's resiliencehub nested-block breaking change — and 9
  uncertain verdicts left both sides).
- The Vault/ingress/cilium FP masses were confirmed false positives (per
  reasons in the files); the two known Vault carve-outs remain non-routine
  and stand as FPs here.
- Every adjudication is committed data: deterministic scoring, auditable
  reasons, and the false-action rate now includes findings joined to
  adjudicated-false changes.

## Adversarial pack (G12)

`eval/adversarial/` — ten join-fooling fixtures with contract-derived
expectations, asserted offline against the real join by
`go test ./internal/eval -run TestAdversarialPack`:

| attack | verdict |
|---|---|
| removed Helm key with similarly-named sibling set by the customer | **held** — segment-wise comparison; sibling never flagged; removed key checked-and-clear |
| customer explicitly pins an old default | **held** — informational values-pinned, never action |
| deciding config source not supplied | **held** — UNKNOWN (insufficient-visibility) with neededToDetermine, never not-affected |
| cluster exactly at minimum boundary | **held** — in-range informational |
| cluster exactly at maximum boundary | **held** — in-range informational |
| image with unrelated tag / different repository | **held** — image-not-referenced, not-affected |
| unrelated resource with identical spec.* path (other API group) | **held** — GVK scoping; crd-field-unset, never crd-field-removed |
| same API group, different kind | **held at the contract line** — review-required ("kind unconfirmed"), never action-required |
| duplicated statement from three sources | **held** — duplicate detection groups all three |
| runtime-dependent behaviour static config cannot prove | **held** — UNKNOWN (not-joined), never action, never not-affected |

"Upstream doc unavailable" is pinned by `TestUpstreamUnavailableIsExecutionFailure`:
a pipeline that cannot run is an execution failure (`pipelineFailures`),
never a silent zero and never a reasoning miss. **No fixture fooled the join
into a wrong action class** — the failures the gates measure are absence of
insight, not wrong certainty.

## Enriched run: the first suggestion-precision number (G10)

`ri eval -enriched` replays the committed answer cache
(`internal/app/testdata/impact-llm-cache`, recorded from glm-5.3-flash)
against the recorded e2e fixture environment and scores the AI layer's
applicability suggestions against the fixture labels:

- **13 suggestions** on unknown findings, all review-required (the only
  class the AI may suggest) — the provenance-preserving contract holds.
- **1 of the 13 carries a fixture label** (suggestionPrecision **0.00**:
  0/1 — the labelled one joins the HTTP01/ingress-nginx item whose ground
  truth is action-required; a review suggestion there is under-escalation).
  The other 12 are unlabelled (their changes match no expectation) and are
  counted, not judged. suggestionRecall is 0/0 — the cert-manager fixture
  declares no unknown-expected finding, so the gate is vacuous.
- Caveat, documented on the flags: the committed cache replays only against
  the recorded inputs (state, clock, and the environment fixture with its
  exact path spelling — paths are part of the prompt digest), so the
  enriched run joins with kubernetes 1.28 (the recorded fixture) and F3
  (in-range at 1.31) legitimately misses. Enriched runs are an overlay;
  they are not stored into eval/results.

The measurement to act on is the 12/13 unlabelled rate: the dataset can only
score suggestions where fixtures state the correct class. Future environment
fixtures should declare `classification` on every expected finding they can
defend.

## Dataset corrections made during the round (all upstream-verified, recorded in NOTES.md)

- strimzi 0.46.0 **does** remove a CRD: `045-Crd-kafkamirrormaker.yaml`
  (MirrorMaker 1) leaves packaging/install/cluster-operator between the tags.
  The blind draft forbade crd-removed findings; the dataset now expects the
  finding (F3), and the join produces it.
- karpenter's chart artifact ships no values snapshots (chart rewritten in
  CI, OCI-only), so values-diff findings are unsupportable there; the
  fixture keeps the operator-relevance link (E7) and its honest miss.
- argo-cd/karpenter cluster-version checks: the admitted-tested/minimum
  vocabulary is `impact:compatibility-satisfied`, not
  `impact:kubernetes-in-range`.
- istio E1's matchers were tightened from a bare "Ztunnel" keyword (which
  matched 29 ambient notes) to the upstream section heading
  "Ambient upgrade with DNS proxy" — removing a false environment-link hit
  the loose matcher had granted.
- aws E4/E8 matchers extended to the v6 upgrade guide's own wording
  (cited): the CHANGELOG channel and the guide phrase the same removals
  differently, and the validation-tightening entries exist only in the
  guide's "Nullable Boolean Validation Update" section.

## Reproducing

```
ri eval                    # full dataset, gate panel, exit non-zero on regression or gate failure
ri eval -o json            # machine-readable: per-hit audit, confusion matrix, adjudications, gates
ri eval -update            # after review: rewrite eval/results (never case.yaml)
ri eval cert-manager-1.17-1.18 -enriched    # opt-in AI-suggestion scoring (offline replay)
go test ./internal/eval    # offline: replayed entry, adversarial pack, routine gate, mechanics
```
