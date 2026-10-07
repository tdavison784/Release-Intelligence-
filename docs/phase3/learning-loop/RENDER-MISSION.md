# Rendering Validation Goal: Prove Concrete Upgrade Effects From Customer Configuration

> Product owner's brief (2026-10-01), recorded in substance. Normative for the `render` lane.

## Mission

Upstream documentation describes behaviour abstractly; the customer needs to know **what actually
changes in the rendered deployment when moving from A to B with their configuration.** Rendering adds a
concrete evidence layer between release interpretation and environment impact:

> **Render source and target release artifacts using real customer configuration, compute semantic
> differences between those renders, and use those differences as first-class evidence for upgrade
> applicability and change validation.**

It distinguishes "vendor says something changed" from "this customer's deployment actually changes".

## Core capability

Given product, source version, target version and customer configuration, produce: source render,
target render, semantic object diff, evidence-backed interpretation — answering *what resources, fields,
permissions, images, APIs, or effective defaults change for this environment?*
(`chart vA + customer values → source manifests`; `chart vB + same values → target manifests`;
semantic render delta between them.)

## Goals

1. **Helm rendering** — deterministic `helm template <release> <chart> --version <v> -f <values>` for
   source and target with the same inputs. Capture chart identity, chart version, renderer version, values
   inputs + digests, release name, namespace, Kubernetes version/capabilities where supplied, render
   timestamp, artifact digest. Reproducible from recorded inputs.
2. **Kustomize rendering** — `kustomize build` for customer repos; recognise kustomization.yaml, bases,
   components, overlays, patches, images, replacements; render the overlays associated with the
   environment under analysis (not every directory). Record kustomize version, root/overlay path, input
   file digests, source revision, render result digest.
3. **Generic rendered resource model** — normalized semantic Kubernetes objects (apiVersion, kind,
   namespace, name, labels, annotations, spec, images, RBAC rules, service ports, volume/config refs),
   stable identity `<group>/<version>/<kind>/<namespace>/<name>`; compare objects, not text.
4. **Semantic render diffing** (no line diffs): resource-added/removed, api-version-changed,
   field-added/removed/changed, image-changed, rbac-permission-added/removed,
   service-port-added/removed/changed, container-arg-added/removed/changed, env-var-added/removed/changed,
   label/annotation changes. Example: ClusterRole cert-manager-controller, `rbac-permission-removed`,
   path `rules[2].verbs`, before `[get,list,watch,update]`, after `[get,list,watch]`.
5. **Render diffs as first-class evidence** — evidence kind `rendered-diff` with renderer type/version,
   source and target artifact + version + digest, environment values digest, object, path. Referenceable
   by semantic facts, impact findings, AI proposals and review items.
6. **Validate release-note interpretation** — rendering can confirm or contradict prose-derived
   proposals. A concrete claim absent from the render is not automatically wrong; classify the relation:
   `confirmed-by-render | not-visible-in-render | contradicted-by-render | render-not-applicable`
   (some legitimate changes are runtime-only).
7. **Customer configuration, not default-only** — the primary path uses the customer's actual
   configuration (Helm values, Argo CD Helm values, Flux HelmRelease values, Helmfile values, repo-local
   values files, Kustomize overlays). Default renders are useful but insufficient.
8. **Bounded counterfactual variants** — only when driven by a specific candidate change (actual config;
   relevant value unset; feature enabled/disabled). E.g. candidate "default foo.enabled false → true",
   customer omits foo.enabled: render source/target with customer config, and if needed source/target
   with foo.enabled=false, to attribute the difference to the default. No arbitrary combinations.
9. **Strengthen the environment join** — semantic release fact + customer-specific render delta that
   agree promote applicability confidence (e.g. "Deployment now includes sidecar X": source absent →
   target present in the customer render).
10. **Automatic knowledge approval, for suitable classes only** — two independent models agree +
    deterministic render confirms the effect + evidence chains resolve ⇒ APPROVED semantic knowledge, for
    resource added/removed, field changed, image changed, RBAC, container args/env, service changes.
    Runtime behaviour changes without rendered evidence follow normal review.
11. **Preserve ACTION REQUIRED** — a render difference alone never produces ACTION REQUIRED. Required:
    verified upstream change + rendered customer-specific change + verified consequence + environment
    exposure. Rendering proves the structural change, not the operational consequence (an RBAC
    permission removed matters only if the workload depends on it).
12. **Renderable vs non-renderable** — every change categorisable as `render-verifiable` (resources,
    RBAC, images, args, env vars, ports, labels, annotations, API versions), `partially-render-verifiable`
    (defaults, feature activation, cross-resource relationships), `not-render-verifiable` (runtime
    controller behaviour, protocol semantics, DB migrations, external services, performance, internal
    algorithms). The renderer is never universal truth.
13. **Explicit renderer failures** — chart unavailable, renderer unavailable, missing dependency, invalid
    values, missing CRD/API capability, template error, kustomize dependency failure, unsupported feature
    ⇒ `UNKNOWN / RENDER FAILED` (an evidence gap), never "no change".
14. **Reproducibility** — record renderer + version, source/target artifact digests, customer config
    digest, platform inputs, command options, output digest; cache renders by these digests.
15. **Normalize non-semantic noise** — map ordering, semantically-unordered list ordering, empty vs
    omitted where equivalent, known-noise generated annotations, timestamps, chart metadata; keep ordering
    where Kubernetes semantics depend on it. Show deployment differences, not serialization differences.
16. **Render-based evaluation cases** — RBAC, images, resources added/removed, env vars, args, Service
    ports, API-version migrations, Helm default changes, values-driven behaviour, Kustomize overlay
    differences — expected render deltas established **before** tuning.
17. **Measure rendering value** — render success rate; render diff precision (and recall where ground
    truth exists); proposals confirmed / contradicted by render; UNKNOWN → decided due to render; ACTION
    findings strengthened by render; false ACTION changes after rendering; applicability accuracy before
    vs after. Key question: **does rendering move applicability up without increasing false certainty?**
18. **Engineering review integration** — the dashboard shows render evidence inline (object, source vs
    target values) next to model proposals, with approve/reject/correct; no manual chart downloading.
19. **Learning dataset** — review records keep model proposals, render evidence, human verdict, final
    fact, to answer: how often does two-model consensus + render confirmation match human judgement?
    Which render-diff classes are safe to auto-approve? Which still need humans?

## Minimum workflow

```bash
ri impact cert-manager v1.17.0 v1.18.0 --repo ./customer-repo --kubernetes 1.31 --render
```

detect Helm/Kustomize config → resolve source and target artifacts → render both with customer config →
normalize → semantic diff → correlate with upstream changes → attach rendered evidence → environment
applicability.

## Non-goals

General-purpose Kubernetes diff product; Helm/Kustomize replacement; deployment engine; reconciliation
controller; GitOps platform; production policy engine.

## Success gates

Reproducible Helm source/target rendering with real customer values; Kustomize rendering for
representative overlays; normalized semantic resource diffing; render diffs as first-class
provenance-backed evidence; proposals confirmed or challenged by rendering; dashboard integration;
explicit renderer failures; improved applicability on the Phase 3 corpus; **no increase in false ACTION
REQUIRED**; render-confirmed knowledge participates in automatic APPROVED decisions.

> "The upstream evidence says this changed, two independent interpretations agree on what it means, and
> rendering your configuration shows the exact resource-level effect."
