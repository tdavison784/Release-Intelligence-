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
