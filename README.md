# Release Intelligence (prototype)

A vendor-neutral prototype that answers one question:

> **Given a software product and two versions, what does upgrading from A to B mean?**

The answer is an **UpgradeEdge**, a structured description in which every
conclusion points back to the source it came from. It covers:
- breaking changes and migration requirements
- deprecations and removals
- Helm value changes and CRD/API schema changes
- platform compatibility
- security fixes
- artifact changes

Each source citation carries a URL, a line range, a verbatim excerpt and the
digest of the exact bytes that were read. The catalog holds **28 products** —
cert-manager, Istio and Argo CD through Cilium, PostgreSQL, Vault,
kube-prometheus-stack, Traefik, Flux, the OTel Collector, Redis, Elasticsearch,
the Terraform AWS Provider, Falco, Linkerd, Prometheus Operator, Loki, MinIO
and the Go toolchain — onboarded as pure configuration
([docs/ONBOARDING.md](docs/ONBOARDING.md) measures how little code each one
needed; [docs/phase2/OUTCOMES.md](docs/phase2/OUTCOMES.md) is the phase
verdict). Nothing in the design is tied to one ecosystem.

A second question builds on it: **which of those changes matter to YOUR
environment?** `ri impact` joins the edge with local Helm values, manifests,
installed CRDs, an image list, a cluster version and a product inventory
(`--inventory`: which other products run, at which versions). It is deterministic, and
every finding cites two provenance chains — the upstream evidence of the
change and the environment evidence (file, line, excerpt) that matched
([docs/IMPACT.md](docs/IMPACT.md)).

```
$ ri upgrade cert-manager v1.17.0 v1.18.0
cert-manager v1.17.0 → v1.18.0
Path: v1.18.0  (policy minor-lineage; 4 backport releases skipped)
Summary: 5 breaking · 0 action required · 51 changes · 2 warnings

Sources:
  ✗ github-releases        github-releases  versions       unavailable: … (HTTP 403)
  ✓ git-tags               git-tags         versions       ok
  ✓ website-release-notes  repo-file        release-notes  ok
  ✓ upgrade-guide          repo-file        upgrade-guide  ok
  ✓ supported-releases     repo-file        compatibility  ok
  ✗ controller-image       oci              —              unavailable: quay.io …
  …
Breaking changes (5):
  • [v1.18.0] ACME HTTP01 challenge paths now use `PathType` `Exact` in Ingress routes: …  (declared · ev-110340e9c914)
  • [v1.18.0] The default value of `Certificate.Spec.PrivateKey.RotationPolicy` is now `Always`: …
  …
API & CRD schema changes (4):
  • [diff] Issuer v1 schema: 2 fields added: `spec.acme.profile`, `spec.vault.serverName`  (computed · …)
Helm value changes (2):
  • [diff] Default of Helm value `prometheus.servicemonitor.targetPort` changed: 9402 → "http-metrics"
Artifact changes (8 updated):
  controller-image  quay.io/jetstack/cert-manager-controller  v1.17.0 → v1.18.0  [referenced]  (ev-87e79b56b7a6, …)
  helm-chart        https://charts.jetstack.io/cert-manager  v1.17.0 → v1.18.0  [expected]   (no evidence)
Kubernetes compatibility:
  supported: 1.29–1.33 (unchanged)
Security changes (5):
  • [v1.18.0] Bump `golang.org/x/crypto` to patch `GHSA-hcg3-q754-cr77`  (security · heuristic · …)
Evidence (112 records):
  ev-110340e9c914  https://github.com/cert-manager/website/blob/master/content/docs/releases/release-notes/release-notes-1.18.md  L28-L89 (Major Themes › …)
```

`[referenced]` means quay.io was unreachable, but the exact image tag was found
in the release's install manifest, at a specific line. `[expected]` means
nothing could confirm the artifact, so it is never presented as verified.
Full examples are in [`internal/app/testdata/e2e/golden/`](internal/app/testdata/e2e/golden).

## Core ideas

- **Product first.** A product publishes many artifacts per release: VCS
  releases, Helm charts, container images, manifests, CRDs, binaries and docs.
  Their versions do not have to match. Argo CD v3.0.0, for example, ships in
  chart 8.0.x, and some app releases ship in no chart at all.
- **Declarative product definitions** (`products/*.yaml`, schema in
  `schemas/`). Release channels, artifacts, version relationships, extraction
  and classification rules, and known exceptions are data, not code. Istio and
  Argo CD were added without product-specific Go code.
- **Deterministic first.** The recurring ingestion path never calls an LLM. AI
  is used only for source discovery, ambiguity resolution and optional
  enrichment (`ri upgrade -enrich`, see [docs/ENRICHMENT.md](docs/ENRICHMENT.md)),
  and it always carries provenance.
- **Facts, conclusions and AI are kept apart.**
  - Facts are deterministic statements extracted from sources.
  - Changes are deterministic conclusions, marked `declared`, `computed` or
    `heuristic`.
  - Enrichments are AI-derived and marked `ai`, together with the model and
    its version, the prompt version and digest, the input evidence and the
    evidence they cite.
  - `UpgradeEdge.Validate()` and the JSON Schema both enforce this.
- **Honest about gaps.** An unreachable channel is reported as
  `unavailable`, and a predicted artifact as `expected`. Neither is ever
  shown as `verified` or `missing`. The same rule drives
  [drift detection](docs/DRIFT.md): unreachable is `unverifiable`, only a
  channel that answered differently is drift.
- **Reproducible.** All upstream reads go through a filesystem cache, and
  `-offline` replays a run byte for byte.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the model and the package
layout. See [docs/FINDINGS.md](docs/FINDINGS.md) for what the prototype taught
us about the thesis. See [docs/research/](docs/research) for the
human-verified release-channel maps the definitions were authored from.

## Usage

```
go build -o ri ./cmd/ri

ri products                                     # list definitions
ri validate                                     # static validation (Go validator)
ri versions istio                               # canonical releases (with source fallbacks)
ri check argo-cd                                # validate declared relationships against 6 historical releases
ri drift istio                                  # re-validate the definition on the newest releases; drift report + proposal (never applied)
ri ingest cert-manager v1.18.0                  # one release: sources, artifacts, notes, snapshots
ri upgrade cert-manager v1.17.0 v1.18.0         # the upgrade edge (text)
ri upgrade argo-cd v2.14.11 v3.0.6 -o json      # … as JSON (schemas/upgrade-edge.schema.json)
ri upgrade istio 1.29.2 1.30.1 -verbose         # include features, bug fixes, dependency updates
ri upgrade cert-manager v1.17.0 v1.18.0 -enrich # + AI clusters/explanations with provenance (docs/ENRICHMENT.md)
ri discover github.com/cert-manager/cert-manager -out proposed.yaml -report report.md
ri knowledge route                              # learning loop: route candidates into review items / auto-verified facts (docs/KNOWLEDGE.md)
ri knowledge decide -reviewer me <review-item>  # record a decision (proxy: -reviewer-kind proxy -proxy-* provenance)
ri knowledge metrics [-o json]                  # agreement, per-model accuracy, review cost, fact counts
ri knowledge export -o feedback.jsonl           # the human-feedback dataset
ri review serve -demo                           # engineering review UI for the learning loop (docs/REVIEW_UI.md)
ri render diff cert-manager v1.17.0 v1.18.0 --repo ./customer-repo --kubernetes 1.31   # render both releases with your configuration; semantic object diff (docs/RENDER.md)
ri impact cert-manager v1.17.0 v1.18.0 --repo ./customer-repo --kubernetes 1.31 --render # impact report + what actually changes in your rendered deployment
```

Global flags go before the command:

| Flag | Effect |
|---|---|
| `-offline` | Replay from the local cache only |
| `-refresh` | Bypass the caches |
| `-state DIR` | Set the cache and store location (default `.ri`) |
| `-v` | Log progress |

**Credentials (optional):**
- `GITHUB_TOKEN` enables the GitHub API adapters: release bodies and security
  advisories.
- `ANTHROPIC_API_KEY` enables `ri discover -llm`. AI proposals enter a
  definition only after validation against real releases.
- `ANTHROPIC_API_KEY` also enables `ri upgrade -enrich`. Alternatively,
  `-llm-exchange DIR` exchanges prompts and answers through files with any
  model. Answers are cached under the state directory and replay with
  `-offline`.

## Testing

```
go test ./...                                   # all unit + offline end-to-end tests (no network needed)
go test ./internal/app -run TestE2E -update     # refresh golden outputs after an intended change
go test ./internal/app -record -update -count=1 # re-record upstream data (network)
go run ./internal/domain/schemagen              # regenerate the UpgradeEdge/Release JSON Schemas
```

## How this was built

The prototype was built by a coordinating agent and a fleet of sub-agents,
each working in its own git worktree against contracts the coordinator
defined first:
- domain model
- catalog format
- source ports
- `api.go` stubs per package

| Role | Model |
|---|---|
| Research (release channels of each product) | Sonnet |
| Source adapters, parsing/normalisation, end-to-end tests, schemas, fixes | Sonnet |
| Ingestion pipeline, upgrade engine, discovery, code review | Opus |

The coordinator integrated each branch and validated the system against live
upstream data.

## Status

This is a learning prototype: local filesystem cache, local JSON store and a
CLI. It has no services, auth or multi-tenancy. Domain logic sits behind
ports, so storage, scheduling and multi-tenancy can be added later without
changing the model.
