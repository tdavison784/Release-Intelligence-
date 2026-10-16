# Investigation case format (Phase 4 ground truth)

Normative for `eval/investigations/` (owner of this file: the `contract` lane, then the commander;
owner of every case and its expectations: the `groundtruth` lane). Converged from the groundtruth lane's
draft (its status file, 2026-10-16) with the contract vocabulary of
`docs/phase4/env-impact/DESIGN.md` §2. Scored by the `eval` lane (DESIGN.md §6) against the
pre-registered gates in `eval/investigations/gates.yaml`.

## Integrity rules (FLEET.md "Evaluation integrity")

- Inputs and ground truth are separate files. **Everything a system under test may read is
  `input.yaml` and `env/`.** `expected.yaml`, `NOTES.md` and `observed/` are ground truth: lanes building
  the pipeline, the agent, the validators or the policy must not read them while designing, and no
  prompt, heuristic or policy may be tuned to a case.
- Expectations are authored blind from upstream sources and, wherever feasible, confirmed by an observed
  outcome (a real upgrade in a throwaway `kind` cluster), never from Release Intelligence output.
- Case ids and fixture contents must not leak the expected class (neutral environment slugs such as
  `--payments-eu`, never `--affected`; operator-style comments that never mention the upgrade).
- Only the groundtruth lane writes `expected.yaml`; `eval/investigations/results/` is written by the
  eval harness and never edited to move a number.

## Layout

```
eval/investigations/
  FORMAT.md                      # this file
  gates.yaml                     # pre-registered gates (frozen)
  README.md                      # case index: case, pair, product, from→to, verification (no expected class)
  <case-id>/
    input.yaml                   # INPUTS ONLY
    env/                         # the GitOps repository fixture (a directory treated as a repo root)
    expected.yaml                # GROUND TRUTH (blind)
    NOTES.md                     # upstream citations + how the outcome was observed
    observed/                    # raw outputs of the known-outcome run (never edited)
```

Case id: `<product>-<fromMinor>-<toMinor>--<env-slug>`. The two halves of a pair share `pair:` and
differ only in the one setting that decides impact. (The examples below are fictional and say nothing
about any dataset case.)

## `input.yaml` (readable by every lane)

```yaml
id: cert-manager-1.17-1.18--example-org     # (fictional example; not a dataset case)
pair: cert-manager-1.17-1.18--example        # optional
product: cert-manager                            # products/<id>.yaml id
chart: {repo: https://charts.jetstack.io, name: cert-manager}
from: {version: v1.17.2, chartVersion: v1.17.2}  # product tag and chart version
to:   {version: v1.18.0, chartVersion: v1.18.0}
environment:
  id: example-prod                              # → investigate.EnvironmentInputs.Name / EnvironmentSnapshot.Environment
  root: env/                                     # → EnvironmentInputs.Repo (relative to the case dir)
  entrypoints: [clusters/prod/kustomization.yaml]  # where a GitOps controller starts (informational)
  kubernetes: "1.31"                             # optional → EnvironmentInputs.KubernetesVersion
  inventory: env/inventory.yaml                  # optional → EnvironmentInputs.Inventory
  complete: true                                 # the repo is the complete declaration of what runs
  description: >-                                # what an operator would say; no hint about impact
    ...
scope:                                           # the upstream breaking-change items the case examines
  - id: B1
    title: Certificate.spec.privateKey.rotationPolicy default changed from Never to Always
    subject: spec.privateKey.rotationPolicy      # same notion of subject as eval/cases matchers
    source: <upstream URL with line anchors>     # identifies the change; not a statement about this env
withheld: []                                     # inputs deliberately absent (insufficient-evidence cases),
                                                 # described as an operator would; never the expected class
```

How the harness runs a case: `investigate.Request{Product, From: from.version, To: to.version,
Environment: {Name: environment.id, Repo: <case>/env, KubernetesVersion, Inventory}, Config: <the run's
config>}`. The system decides its own change items; `scope` is **not** given to the system as a list of
items to classify (it is there so a human can read the case, and the harness uses `expected.yaml`
`match` to find the system's item). A system may read `scope` only as the operator's statement of
interest, never to restrict or seed its analysis.

## `expected.yaml` (ground truth; pipeline/agent/policy lanes must not read it)

```yaml
case: cert-manager-1.17-1.18--example-org     # (fictional example; not a dataset case)
authoredAt: "2026-10-16"
authoredBy: groundtruth lane (blind: upstream sources + observed outcome only)
items:
  - item: B1                                     # input.yaml scope id
    classification: affected                     # affected | not-affected | potentially-affected | insufficient-evidence
                                                 # (domain.FindingClass); strongest over components:
                                                 # affected > potentially-affected > insufficient-evidence > not-affected
    verification: observed                       # observed (kind upgrade) | render (helm/kustomize before/after)
                                                 # | upstream-only (no outcome could be produced; NOTES.md says why)
    observed: [observed/03-after-renewal.txt]    # raw evidence of the outcome, relative to the case dir
    citations:
      - {url: <upstream URL>, lines: "L109-L148", quote: "..."}
    match:                                       # how the system's change item for B1 is recognised
      - subject: spec.privateKey.rotationPolicy  # any element of the item's Change.subjects contains it
      - text: '(?i)rotation ?policy'             # RE2 against the item's title (and change detail)
    components:                                  # one entry per environment component the item reaches
      - ref: {file: env/infra/tls/root-ca.yaml, apiVersion: cert-manager.io/v1, kind: Certificate,
              namespace: tls, name: example-root-ca}
        classification: affected                 # FindingClass for this component
        impactPath:                              # ordered hops: exposed component → … → consumer
          - {ref: {file: env/clusters/prod/cert-manager.yaml, kind: Application, namespace: argocd, name: cert-manager}, relation: deploys}
          - {ref: {file: env/infra/tls/root-ca.yaml, kind: Certificate, namespace: tls, name: example-root-ca}, field: spec.privateKey.rotationPolicy, relation: consumes-api}
          - {ref: {kind: Secret, namespace: tls, name: example-root-ca}, relation: produces}
          - {ref: {file: env/infra/tls/ca-issuer.yaml, kind: Issuer, namespace: tls, name: example-ca}, relation: references}
        reason: one paragraph in terms of the fixture (what breaks, for whom)
        mitigation: null                         # not-affected: {ref, field, value, kind: override|feature-unused|relationship-absent}
        requiredEvidence:                        # what a correct finding must cite (evidenceCoverage)
          - {file: env/infra/tls/root-ca.yaml, object: Certificate/example-root-ca, field: spec.privateKey}
```

### Vocabulary mapping to the contract

| Case format | Contract (`internal/domain/investigation.go`) |
|---|---|
| `classification` | `FindingClass` (`affected`, `not-affected`, `potentially-affected`, `insufficient-evidence`) |
| `ref` (`apiVersion`/`kind`/`namespace`/`name`) | `GraphNode.Key` / `RenderedChange.Object` = `<group>/<version>/<kind>/<namespace>/<name>`; matching uses kind + namespace + name (group/version when given) |
| `ref.file` | the `environment-file` artifact whose `Path` is `env/<file without the env/ prefix>` |
| `relation` | `EdgeKind`, in the direction impact propagates (provider → consumer): `deploys`, `renders-to`, `configures`, `overrides`, `references` (referenced → referrer; also "mounts", "consumed-by"), `selects`, `serves-api`, `consumes-api`, `produces` (a controller creates the object, e.g. Certificate → Secret; draft "issues-secret"), `depends-on` |
| `mitigation.kind` | `override` ↔ an `overrides` edge / an override-inspection `pass`; `feature-unused` ↔ no correlated rendered change and no path; `relationship-absent` ↔ the path stops (no consumer) |
| `verification: observed` | scored as the `observed` level; `render` as render-level; `upstream-only` reported separately |
| `requiredEvidence` | satisfied when the finding (or a validation it cites) cites an artifact with that path, and — when `object`/`field` are given — evidence/rendered changes naming that object/field |

### Scoring summary (details: DESIGN.md §6)

- One **item decision** per expected item: the system finding for the change item selected by `match`
  (the in-scope item whose subjects/title match; several matching items → the strongest class among them,
  reported). No matching item → scored as `insufficient-evidence` and counted as an item-match miss.
- Component-level results (`exposureRecall`, `evidenceCoverage`) use `components`.
- Verification level of the expectation and the finding `Basis` split every metric.

## `NOTES.md`

Per item: the upstream citations (URL + line ranges + verbatim quotes: release notes, upgrade guide,
chart diff, source commit), the reasoning in terms of the fixture, and the observed outcome — commands
run, versions, the relevant output excerpts (raw files under `observed/`). If a runtime check was
impossible: why, and the render-level fallback used (labelled `verification: render`). Never an invented
outcome.
