# Prototype findings

The prototype set out to test one thesis: **can we build a continuously
maintained graph of how software products evolve, and use it to understand
what an upgrade means?** These are the findings after building the first
milestone and repeating it for cert-manager, Istio and Argo CD (October 2026,
with live upstream data).

## Milestone status

| # | Milestone criterion | Status | Where |
|---|---|---|---|
| 1 | Domain model for products, releases, artifacts, sources, evidence and upgrade edges | ✓ | `internal/domain`, `schemas/*.schema.json` |
| 2 | cert-manager has a valid product definition | ✓ | `products/cert-manager.yaml` |
| 3 | The system can validate that definition | ✓ statically (Go validator + JSON Schema) | `ri validate` |
| 4 | Historical release relationships can be checked | ✓ against 6 releases per product | `ri check <product>` |
| 5 | The system retrieves the relevant information for two versions | ✓ | `ri ingest`, `ri upgrade` |
| 6 | It produces a structured UpgradeEdge | ✓ text + JSON, with a schema | `ri upgrade … -o json` |
| 7 | Important conclusions retain source provenance | ✓ enforced by `UpgradeEdge.Validate()` and the schema | every Change → Evidence |
| 8 | Generic enough to add Istio and Argo CD without product branches | ✓ no product-specific Go code; both are pure YAML | `products/istio.yaml`, `products/argo-cd.yaml` |

`ri check` validates every reachable relationship in each definition against
six historical releases: the X.Y.0 release and the latest patch of each of
the three newest lines. No relationship fails.

| Product | Relationships validated (≥3 passes, 0 failures) | Unverifiable here | Notes |
|---|---|---|---|
| cert-manager | 14 | Helm chart, startupapicheck image (quay.io / charts.jetstack.io blocked) | 4 quay.io images verified through the install manifest |
| Istio | 26 | none | every chart, image and archive verified directly at its registry |
| Argo CD | 11 | none | chart marked optional; v3.4.0 asset exception recorded |

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** the cert-manager "Unverifiable here" cell describes the sandbox run only.
> With network access the chart and all six images validate directly (17
> validated subjects, 0 unverifiable; `docs/onboarding/checks/cert-manager.json`
> was replaced by the live run). The Argo CD "v3.4.0 asset exception" note is
> corrected below.

## What worked

- **Treating the product as the entity.** A single release fans out into 5–15
  artifacts. Their versions often differ from the app version: Argo CD v3.0.0
  ships in chart 8.0.x, and many Argo CD releases ship in no chart at all.
  The `lookup` version relation and `optional` artifacts handle this without
  special cases.
- **Declarative definitions.** Istio and Argo CD were added without touching
  Go code. Each new real-world quirk became a small, generic, declarative
  construct instead of an `if product == …` branch. The quirks were:
  - per-minor release-notes files with one section per patch
  - notes that accumulate in a directory (`baseRef`)
  - support matrices in YAML (`yaml-records`)
  - table cells that combine two platforms (`separator`)
  - an LTS vendor table that shadows the real one (`ValueColumns`)
  - chart defaults nested under a wrapper key (`stripPrefix`)
  - development placeholders in source-tree values (`ignoreKeys`)
  - alternative channels (`fallbackGroup`)
  - artifacts that only some releases ship (`optional`)
  - upstream anomalies (`exceptions`, with a mandatory reason)
- **Deterministic first, honest about gaps.** The recurring path never calls
  an LLM. Blocked hosts (api.github.com, quay.io, charts.jetstack.io,
  argoproj.github.io) are reported as `unavailable`, never as `missing`. An
  artifact the definition merely predicts is `expected`. Cross-references
  (the image tag inside the install manifest) recover `referenced` status
  with line-level evidence.
  > **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** those four hosts were blocked by the sandbox, not by upstream: all
  > answer with network access, the `referenced` stand-ins became direct `oci`
  > passes, and `covered` GitHub-release checks became `pass` (212 sandbox-artifact
  > differences across 28 products). The honesty rule itself is unchanged and
  > proved right: nothing was ever shown as `verified` that the live run later
  > contradicted. Rate limiting is now a separate `throttled` state.
- **Historical validation finds real things.** `ri check argo-cd` discovered
  that the Argo CD **v3.4.0** GitHub release has no assets at all, while
  v3.4.1 has them. (**Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** v3.4.0 is a git tag that was never published as a GitHub
  release: the release object itself returns 404, so its assets 404 too;
  v3.4.1 is the first release after v3.4.0-rc7 and has 11 assets.) It also confirmed that only about half of Argo CD app
  releases ship as a chart `appVersion`, which is why the chart is optional.
- **Evidence is checkable.** Spot checks of line-range evidence against the
  live documents matched in every case. Each evidence record carries the
  sha256 of the exact bytes retrieved, and the whole pipeline replays
  byte-identically from a 2.8 MiB recording (`internal/app/testdata/e2e`).
- **Discovery reaches the same answer as the humans.** The deterministic
  scanner found all 10 discovery targets for all three repositories. It
  agreed with the hand-researched channel maps in `docs/research/`, and
  validation then pruned or annotated its proposals against real releases.

## Where it is weak (and what that means)

- **Upstream structure varies widely.** Every product needed 1–3 extraction
  quirks. The constructs are generic, but onboarding a product is still
  research work. Discovery reduces that work but does not remove it.
  Expect a long tail: the first ~20 products will probably surface most
  constructs.
- **Semantic duplicates across sources.** The same fact often appears in the
  release notes, the upgrade guide and the structured notes, worded
  differently. Exact and punctuation-insensitive matching merges only some
  of them. cert-manager's `RotationPolicy` default change appears twice.
  This is the clearest place where **AI enrichment with provenance** would add
  value: clustering items into one conclusion that cites several pieces of
  evidence. The `Enrichment` type and its invariants are ready for it.
- **Classification is heuristic at the edges.** Upstream labels (headings,
  bold verbs, conventional-commit prefixes, callouts) are reliable and are
  marked `declared`. Keyword fallbacks are marked `heuristic` and are
  sometimes wrong. Provenance makes the difference visible, but a consumer
  still needs to weigh it.
- **Mutable documentation.** Upgrade guides and support matrices on `master`
  are edited after release. The cert-manager 1.18→1.19 guide now mentions a
  v1.19.6 change. Evidence pins the digest of the bytes used, but "what
  did the docs say at release time" needs history: git log of the doc, or
  periodic snapshots.
- **Source-tree vs published artifacts.** Values diffs come from the chart
  source at the tag. That is correct for cert-manager and Argo CD. Istio's
  release tooling rewrites image hub and tag, which are therefore ignored.
  Reading the published chart archive would be more faithful. Here, ghcr
  blobs and blob.istio.io were unreachable. (**Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** blob.istio.io (Helm index) and registry.istio.io answer with network access.)
- **Commit logs are a noisy substitute for release notes.** Argo CD's curated
  notes exist only behind the GitHub API. The `git-log` fallback is
  deterministic and correctly classifies `feat!`/`fix!`, but it is noisy:
  dependency bumps, docs and release mechanics. Skip rules help, and real
  release bodies would be better.

## Is the thesis viable?

**Yes, with a clear division of labour.** A continuously maintained,
evidence-backed graph of product releases is technically viable with mostly
deterministic machinery:
- version lists
- artifact resolution and verification
- structured snapshots (Helm values, CRDs, images, compatibility)
- computed diffs
- release-note extraction with upstream labels

Those pieces are reproducible, cheap to re-run and auditable. The hard
problems are two:
- **onboarding and drift**, where discovery plus historical validation is the
  right shape and AI helps with ambiguity;
- **semantic consolidation**, where AI enrichment over evidence is the
  natural next step.

Neither requires giving up provenance.

The step from "what changed" to "what this upgrade means for *my*
environment" is now mostly a join. It needs the customer's actual
configuration as input:
- the Helm values they set, intersected with removed or changed keys
- the CRD versions and fields they use, intersected with removed or unserved
  versions and pruned fields
- their cluster's Kubernetes version, checked against constraints
- the images they mirror, against added images

The edge already exposes all of these as structured `Subjects` and snapshots.

## Suggested next steps

1. **Environment-aware edges.** Accept a values file, a list of in-cluster
   CRD objects and a cluster version, and filter or prioritise Changes by
   actual exposure.
2. **AI enrichment (provenance-preserving).** Cluster semantically duplicate
   items, summarise migration steps and explain risk. Store the output as
   `Enrichment`s citing input evidence, never as Changes.
3. **Published-artifact readers.** Read files inside chart archives (tgz /
   OCI layers), so values and CRDs come from what users actually install.
4. **Drift detection.** Re-run `ri check` on new releases. Any relationship
   that stops validating triggers discovery and LLM investigation for that
   product only.
5. **Document history.** Fetch docs at the release commit, or keep snapshots,
   to answer "what did upstream say when this shipped".
6. **Credentials.** Configure tokens for the GitHub API, quay.io and similar,
   so curated release bodies and advisories replace the fallbacks.
7. **More products.** Onboard about 10 more through `ri discover` to measure
   how many new constructs the long tail needs.
