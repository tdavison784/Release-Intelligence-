# Phase 2 plan: does Release Intelligence scale?

> **Status: complete (2026-10-01).** 28 products onboarded across waves 0–4,
> all nine workstreams delivered. The measured outcome and the honest
> frontier are in [OUTCOMES.md](OUTCOMES.md).

Phase 1 showed that one generic, evidence-backed model can represent
cert-manager, Istio and Argo CD without product-specific code. Phase 2 has to
answer the product question:

> Can Release Intelligence become a generalised knowledge layer for
> third-party operational software, where adding products is scalable and the
> release graph can be joined with a customer's real environment to say exactly
> what an upgrade means for *them*?

Out of scope: orchestration, data lakes, controllers, SaaS, auth, UI.

## Workstreams

| # | Goal | Deliverable | Packages / commands |
|---|---|---|---|
| G1 | 20–30 diverse products | `products/*.yaml`, `docs/research/*.md` | YAML only, plus generic constructs when justified |
| G2 | Measure onboarding | per-product records + `docs/ONBOARDING.md` | `docs/onboarding/`, `ri stats` |
| G3 | Stronger discovery | richer scanners, relationship status, `-compare` coverage | `internal/discovery`, `ri discover` |
| G4 | Source drift | deterministic drift report with proposed changes, never mutating | `internal/drift`, `ri drift` |
| G5 | Semantic enrichment | AI clusters/summaries/explanations as `Enrichment`s with full provenance | `internal/enrich`, `ri upgrade -enrich` |
| G6 | Environment-aware impact | environment model + join of edge × environment | `internal/env`, `internal/impact`, `ri impact` |
| G7 | Two provenance chains | every finding cites upstream evidence and environment evidence | `domain.ImpactReport` |
| G8 | Published artifacts | Helm .tgz / OCI chart layers / OCI manifests / release assets | `internal/artifact`, catalog `contents` |
| G9 | Validation dataset | ground-truth upgrades + evaluator that detects regressions | `eval/`, `ri eval` |

## Onboarding waves

Products are onboarded in waves so that the rate of new generic constructs can
be observed over onboarding order (the convergence curve). Each wave starts
from the merged result of the previous one.

| Wave | Order | Products |
|---|---|---|
| 0 (Phase 1) | 1–3 | cert-manager, Istio, Argo CD |
| 1 | 4–10 | Cilium, PostgreSQL, kube-prometheus-stack, Vault, Crossplane, External Secrets Operator, ingress-nginx |
| 2 | 11–17 | Grafana, Strimzi, Traefik, Karpenter, Flux, OpenTelemetry Collector, Redis |
| 3 | 18–24 | Elasticsearch, Terraform AWS Provider, Actions Runner Controller, Kyverno, Falco, Linkerd, Prometheus Operator |
| 4 | 25+ | edge cases chosen from what waves 1–3 exposed |

The onboarding protocol and record format are in
[`docs/onboarding/PROTOCOL.md`](../onboarding/PROTOCOL.md).

## Rules that hold for every workstream

- **No product-specific Go code.** A new generic construct needs evidence that
  it has value beyond one product (name the other products). Otherwise the
  gap is recorded, not hacked around.
- **Deterministic first.** The recurring path (ingest, upgrade, impact, drift)
  never calls an LLM. AI output is only ever an `Enrichment` (or a discovery
  proposal), carries `method: ai`, the model, the model version, the prompt
  version and digest, the input evidence and a timestamp, and never replaces a
  deterministic fact.
- **Evidence or it did not happen.** Every conclusion cites evidence that
  resolves inside the same document (edge, impact report, drift report).
- **Honest gaps.** Unreachable is `unavailable`, never `missing`. Sandbox
  egress has varied over time: hosts blocked during Phase 1 and early wave-1
  onboarding (GitHub API, quay.io, most `*.github.io` Helm repositories,
  codeload tarballs) answered 200 by late 2026-10-01, while `registry.k8s.io`
  307-redirects and anonymous Docker Hub pulls stay rate-limited or 401.
  Verify each host when you onboard (records keep the truth of their moment),
  still declare the canonical channel first, and add reachable alternatives
  after it.
- **Trusted definitions are never mutated by tools.** Discovery and drift
  write proposals and reports; a human (or a reviewing agent) edits
  `products/*.yaml`.
- Every commit passes `go test ./...` (offline).
