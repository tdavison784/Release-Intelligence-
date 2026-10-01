# Release Intelligence (prototype)

A vendor-neutral prototype that answers one question:

> **Given a software product and two versions, what does upgrading from A to B mean?**

Its output is an **UpgradeEdge**: a structured, evidence-backed description that
covers:
- breaking changes
- migration requirements
- deprecations and removals
- configuration and Helm value changes
- CRD and API changes
- platform compatibility
- security fixes
- artifact changes

Every conclusion points back to the source it came from: a URL, a line range, a
verbatim excerpt and the digest of the exact bytes retrieved.

The initial products are third-party operational software from the Kubernetes
ecosystem: **cert-manager**, **Istio** and **Argo CD**. The design is not tied
to any single ecosystem.

```
$ ri upgrade cert-manager v1.17.0 v1.18.0
```

## Core ideas

- **Product first.** A product publishes many artifacts per release: VCS
  releases, Helm charts, container images, manifests, CRDs, binaries and docs.
  Their versions do not have to match. Argo CD v3.0.0, for example, ships in
  chart 8.0.x, and some app releases ship in no chart at all.
- **Declarative product definitions** (`products/*.yaml`, schema in
  `schemas/`). Release channels, artifacts, version relationships, extraction
  and classification rules are data, not code.
- **Deterministic first.** The recurring ingestion path never calls an LLM.
  AI is reserved for source discovery, for resolving ambiguity and for
  optional enrichment, and it always carries provenance.
- **Facts ≠ conclusions ≠ AI.** Facts are deterministic statements extracted
  from sources. Changes are deterministic conclusions marked `declared`,
  `computed` or `heuristic`. Enrichments are AI-derived and marked `ai`, with
  the model, the prompt digest and the input evidence.
- **Honest about gaps.** An unreachable channel is reported as unavailable, and
  an artifact that is only predicted by the definition is shown as `expected`,
  never `verified`.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the model and package
layout. See [docs/research/](docs/research) for the verified maps of each
product's release channels that the definitions were authored from.

## Usage

```
go build -o ri ./cmd/ri

ri products                                   # list definitions
ri validate                                   # static validation of all definitions
ri versions cert-manager                      # canonical releases (with source fallbacks)
ri check cert-manager -n 3                    # validate declared relationships against 3 historical releases
ri ingest cert-manager v1.18.0                # one release: facts, artifacts, notes, snapshots
ri upgrade cert-manager v1.17.0 v1.18.0       # the upgrade edge (text)
ri upgrade cert-manager v1.17.0 v1.18.0 -o json
ri discover github.com/cert-manager/cert-manager   # propose a definition from repo inspection
```

These global flags go before the command:

| Flag | Effect |
|---|---|
| `-offline` | Replay from the local cache only |
| `-refresh` | Bypass the caches |
| `-state DIR` | Set the cache and store location (default `.ri`) |
| `-v` | Log progress |

To enable the GitHub API adapters (release bodies, advisories), set
`GITHUB_TOKEN`. To enable LLM-assisted discovery, set `ANTHROPIC_API_KEY`.

## Status

This is a learning prototype: local filesystem cache, local JSON store and a
CLI. It has no services, auth or multi-tenancy. The domain packages are
structured so those concerns can be added later without redesigning the model.
