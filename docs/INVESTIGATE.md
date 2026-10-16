# Environment-aware investigation (`ri investigate`, Phase 4)

Design: [docs/phase4/env-impact/DESIGN.md](phase4/env-impact/DESIGN.md). Each section below is owned by
one lane; edit only your own section.

## Evidence bundle

An investigation starts from an **evidence bundle**: an immutable, content-addressed record of one
upgrade (`product from → to`) against one environment. Code: `internal/investigate/bundle` (collectors,
assembler) and `internal/investigate/store` (stores). Contract: DESIGN.md §2.

| Piece | Built by | What it holds |
|---|---|---|
| `ChangeEvidenceRecord` | `bundle.NewChangeCollector(app, store)` | the `UpgradeEdge` (one `upgrade-edge` artifact), a `ChangeItem` per non-routine change and per moved compatibility constraint, the upstream documents, the release-scope (chart-default) render, and a status for **every** change source |
| `EnvironmentSnapshot` | `bundle.NewSnapshotBuilder(store)` | every environment file (secrets redacted), the parsed `env.Environment` (`environment-facts`), a status per input, the repo pinned by commit, dirty flag and a tree digest |
| `EvidenceBundle` | `bundle.NewAssembler(store)` | the two above plus the environment renders (from the `renderdiff` lane), the manifest of all artifacts and the evidence pool; `Validate()` resolves every id |

**Change sources.** Each of `release-notes`, `migration-guide`, `source-diff`, `api-schema`, `chart`,
`kustomize`, `rendered-manifests`, `compatibility`, `advisories` is stated exactly once:
`ok` (with the collected artifacts), `partial`, `unavailable` or `not-applicable`, always with a reason
unless `ok`. Upstream documents are fetched through the fetch cache (a GitHub blob URL is read as its raw
content) and compared with the digest recorded when they were ingested; a document that changed since is
`partial`. A product with no upgrade guide (ingress-nginx) is `unavailable`, one that publishes no CRD
bundle is `not-applicable`; source diffs and release-side Kustomize are not collected in milestone 1
and say so.

**Scope and manifestation.** Every item carries `InScope` + `ScopeRule` (DESIGN §1.3) and
`Manifestation` + `ManifestationRule` (§2.2.1), decided by declared rules over the upgrade edge
(`internal/investigate/bundle/scope.go`), never by product code. A behaviour default compiled into a
controller (`manifest:declared-behaviour`, `manifest:crd-default`) is not render-decisive: a silent
render never clears it. Every release-scope rendered change that no change explains becomes its own
item, anchored on it (DESIGN §1.3, first match wins): a class the default tier policy blocks (removed
RBAC permission, removed resource, API-version or CRD storage/served move) is in scope
(`scope:rendered-structural`); the version bump of an image the product definition itself publishes
(`render.SelfArtifactBump`: same repository, exactly the edge artifact's from/to tags) is out of scope
(`scope:routine-self-artifact`) — so it is correlated, never "uncorrelated"; anything else is in scope
(`scope:rendered-unexplained`, manifestation `rendered-config`).

**Knowledge facts.** `bundle.NewChangeCollector(app, st, bundle.WithKnowledge(ks.Facts...))` (the facts
`app.LoadKnowledge` proves) stores every fact attached to a change of the edge (`impact.AttachedChanges`,
the join's rule) as a `knowledge-fact` artifact `derived/knowledge/<product>/<fact-id>.json`, related to
that change's item (`ArtifactRef.ChangeItems`, `ChangeItem.Artifacts`). Validators read facts from the
bundle, so an investigation is reproducible from its bundle alone (Principle 6); trust is still decided by
the fact's own verification level (consensus/proxy facts never pin).

**Layout** (`<state>/investigations/`, `store.New(<state>)`):

```
cas/sha256/<hex[0:2]>/<hex>   artifact bytes — immutable, written once, digest verified on every read
bundles/<bnd-id>.json         EvidenceBundle — immutable (re-writing the same bundle is a no-op)
runs/<inv-id>.json            Investigation — rewritten as it progresses; its audit trail may only grow
```

Writes are atomic (temp file + rename). Artifact paths are logical: `upstream/<product>/<host>/<path>`,
`env/<path inside the repo>` (explicit inputs: `env/inputs/<file>`), `derived/<stage>/…`.

**Redaction** happens before hashing: Kubernetes `Secret` `data`/`stringData` values and values whose key
names a credential (`password`, `token`, `apiKey`, `privateKey`, `credentials`, …) become
`"<redacted sha256:…>"` (keys kept); the artifact is marked `Redacted`. Credential values are also
withheld from the facts artifact and from evidence excerpts. Files that are not YAML/JSON are stored
verbatim.

**Reproducibility.** The bundle id is derived from content only. Wall-clock fields (`generatedAt`,
`retrievedAt`) are zeroed in the stored edge copy and `CreatedAt` is not part of the id, so two runs over
the same upstream evidence, repository state and renders yield the same bundle id; a changed file, a
different commit or a dirty tree yields a different one.

**Known gaps (milestone 1).** Live-cluster observation is not implemented (a `live` request records an
`unavailable` input). Routine changes are summarised in the edge, not itemised. Render rules that name
an artifact version bump (rule `artifact`) carry no change id; the collector attributes them to the
computed image items of that repository.

## Render diffs

The centerpiece evidence: the product's from and to releases rendered with the environment's own
configuration, diffed, and indexed so every rendered change is a citable record
(`internal/investigate/renderdiff`, DESIGN.md §2.4). `renderdiff.New(app, store)` implements the
`EnvironmentRenderer` port over `internal/render` (target detection, helm/kustomize renders, the semantic
diff, changelog correlation, chart coverage).

**One pair per render target.** Every deployment of the product the snapshot's repository (or explicit
values files) declares becomes a `ManifestPair` named by the target, with paths repo-relative so ids do
not depend on where the repository is checked out: `argocd:clusters/prod/app.yaml#0/$.spec.sources[0]`,
`flux:…`, `helmfile:…`, `values:<file>`, `kustomize:<dir>`. A repo-discovered values file that an Argo
CD / Flux / Helmfile target already layers is not rendered on its own (`render.DetectTargets`: that
configuration is not deployed anywhere); a note says so.

**Status** (what decides whether an absence of change is evidence):

| Status | When | Changes indexed |
|---|---|---|
| `ok` | both renders succeeded, values complete, and the chart (from or to, or a declared dependency) defines every top-level values key the customer sets | yes — `[]` means "rendered, nothing changed" |
| `partial` | `values-incomplete: …` (valuesFrom / value files outside the repo), `chart-coverage: …` (customer keys the chart never reads), or one side rendered and the other failed (e.g. `invalid-values: the target chart rejects…`) | **no** |
| `unavailable` | neither side rendered (`chart-unavailable`, `renderer-unavailable`, `template-error`, …), or a Kustomize overlay runs the product's images without pinning their version | **no** |
| `not-applicable` | `version-independent:` a Kustomize overlay (a consumer app) that neither pins nor runs the product: the upgrade does not change what it renders; its `from` render is kept for inspection | no |

An incomplete pair is an evidence gap, never "no change". `RenderResult.Notes` records what target
detection skipped, so "no pairs" is never silent.

**Artifacts** (`derived/render/env/<target-slug>/`): `from.yaml` / `to.yaml` are the *normalized* objects
the diff compared, one document per object headed by `# Object: <group>/<version>/<kind>/<ns>/<name>`
(Secret data and credential-named values are `<sensitive sha256:…>` digest markers; the artifact is then
`redacted`); `diff.json` is the full diff (`ri.dev/render-diff/v1alpha1`: provenance of both renders
without timestamps, every change with its id, correlation links and change items; `indexed: false` for an
incomplete pair).

**Rendered changes.** One `RenderedChange` per `render.Change` of an ok pair, id
`RenderedChangeID(environment, target, object, path, class)`. The path is `render.RenderedPath`, shared
by both render scopes: container args and RBAC rules are sets addressed by one path, so their changes
carry a member selector — `…args[=--flag]`, `rules[=apps/deployments:get]`. Each cites one `rendered-diff` evidence record whose `contentDigest` is
the diff artifact's digest and whose locator is `changes[id=<rc>]`, so it resolves to the bytes.
Sensitive changes record no values.

**Correlation** to change items (`Correlation` = the rules, comma-joined):

- a `render.Correlate` link to an edge change → that change's item (links to routine changes, which have
  no item, are dropped);
- an `artifact` link (no change id) → the computed image items (`manifest:computed-image`) whose title or
  subjects name the repository — the collector's release-scope rule;
- `release-anchor` → the item anchored on the matching release-scope rendered change (same class, API
  group, kind, object name and path with the release name normalized);
- otherwise `rendered-anchor` → the change's **own environment item** (`RenderResult.Items` →
  `EvidenceBundle.EnvironmentItems`, anchored on the environment-scope rendered change):
  `scope:routine-self-artifact` (out of scope) for a self-artifact version bump, else
  `scope:rendered-unexplained` (in scope, so it gets a finding: the deployment changes and nothing
  upstream explains it — exactly what the investigation must look at).

`EvidenceBundle.Items()` / `Item()` / `InScope()` cover record and environment items alike. With every
change anchored, `Uncorrelated()` is empty for a renderdiff-built bundle; it still blocks automated
approval when non-empty (DESIGN §3.4).

**Queries** (what the agent's diff tools call): `renderdiff.QueryPage(bundle, DiffQuery)` filters by
scope (default environment), target, change item, uncorrelated, classes, kinds, namespaces (`-` =
cluster-scoped) and path prefix; results are ordered by (target, object, path, class), at most 200 per
page (default 50, ≤256 KiB), with `Next` → `DiffQuery.After` to continue. Unknown targets, items,
classes, scopes or cursors are rejected (`ErrRejected`), never answered with an empty page.
`renderdiff.Object` reads one object or field (`spec.template.spec.containers[name=x].args[=--flag]`,
`metadata.annotations["a/b"]`, `ports[port=443/TCP]`) from either side, as YAML whose text occurs in the
stored render (cite it with `read_artifact`-style quotes); an absent object or path is `found: false`; a
side that was not rendered is `ErrNotFound` with the pair's reason; objects over 64 KiB must be read by
field. `renderdiff.Summaries` lists every pair with counts by class and uncorrelated.

Coverage on the groundtruth environments (8 cases) is recorded in
`docs/phase4/env-impact/status/renderdiff.md`; reproduce with
`RI_RENDERDIFF_STATE=<warm .ri> go test ./internal/investigate/renderdiff -run Groundtruth -v`.

## Validators

Package `internal/investigate/validate` (DESIGN.md §2.6, §3.1, §7). Five `investigate.Validator`s turn the
bundle, the traversal and the deterministic join into `ValidationRecord`s. Every record is at level
`deterministic` and states its outcome relative to the **safety property** of (item, subjects): `pass` =
demonstrably not reached / not broken, `fail` = demonstrably reached **and** breaking, `error` = the validator
broke, `not-run` = it could not decide, with the reason. **A validator that cannot decide never passes.** No
validator calls a model; agent assertions are never inputs.

| Constructor | Name | Kind | Applies | Decides |
|---|---|---|---|---|
| `JoinVerdicts(report, opts…)` | `validate.join@v1` | per verdict | all | wraps the five-class join's decisive verdicts |
| `RenderDiff(opts…)` | `validate.render-diff@v1` | render-diff | all | the environment render pairs |
| `OverrideInspection(opts…)` | `validate.override-inspection@v1` | override-inspection | all | an explicit setting pinning the old behaviour |
| `SchemaCompat(opts…)` | `validate.schema-compat@v1` | schema-compat | api-schema, compatibility | cluster version range; objects vs the CRD change |
| `StaticReference(opts…)` | `validate.static-reference@v1` | static-reference | all | references along the impact paths after the upgrade |

Options: `WithStore(st)` (read artifact bytes — renders, environment files, the upgrade edge, knowledge facts;
without it a validator that needs bytes is `not-run`), `WithFacts(facts…)` (verified facts the wiring loaded from
`knowledge/`), `WithReportArtifact(id)` (cite the impact report as an input), `WithClock`. Invalid input (no
bundle, unknown item, a requested path or rendered change that is not the item's) is an error wrapping
`investigate.ErrNotFound` / `ErrRejected`, never a record.

**Coverage.** A record speaks for the requested subjects/paths, or — with none requested — for every root and
path of the item. An item-wide result (a silent complete render, a universal pin, an in-range cluster) claims all
of them; an object-level result (schema-compat, static-reference) claims the nodes and paths of the objects it
checked. Render pairs with status `not-applicable` (a target that does not deploy the product) are no render of
it and are ignored; any `partial`/`unavailable` pair makes the render incomplete.

**JoinVerdicts** — at most one record per item, for the item as a whole (no subjects): any `action-required`
finding of the item's change (incl. `impact:render-target-rejects`; a model-consensus fact is labelled as such)
→ `fail`; every finding `not-affected`, deterministic, with checks, from a trusted fact when one is involved →
`pass`; all not-affected but one rests on no render (`impact:values-default-unrendered`, or a render check
`unavailable`) → `not-run`; review-required / informational / unknown or a mix → no record. Record kind follows the
evidence that decided it (render → render-diff, CRD/API/cluster → schema-compat, else override-inspection).

**RenderDiff** — `fail` only for a *breaking* item (§1.3: declared, computed-breaking, rendered-structural)
whose environment rendered change is correlated by a rule that ties it to the item's effect (`subject`,
`values-default`, `artifact`, `release-anchor`, or the item is that change), on a path that is not CRD description
text; complete pairs decide. `pass` only for a `rendered-config` item when every pair is complete and nothing is
attributable — or only CRD description text, which is shown harmless and cited. Never a pass for
`controller-behaviour`/`unknown` items (render silence proves nothing), never a fail for `scope:category` /
`scope:rendered-unexplained` items (no concrete failure named); text-mention correlations decide nothing.

**OverrideInspection** — establishes an explicit environment setting that pins the old behaviour and records
it as the pass's `PinnedSetting` (kind, subject, path, value, the rendered object it lands in, environment
evidence) — the only deterministic validation that clears a behaviour item (`ClearsBehaviour`). Which setting
pins which behaviour is never guessed; it comes from a declarative, release-level **pin rule**, cited in the
record (rule id in the provenance, source in the detail):

| Rule | Source | Setting |
|---|---|---|
| `pin:join-pinned@v1` | the join's own pinned verdict on a computed default change (`impact:values-pinned`, `impact:crd-default-pinned`), with no other non-clear finding of the change | the values keys (every changed key) / the field on every resource; objects only the environment render shows must set it too |
| `pin:knowledge-default-changed@v1` | a **trusted** (deterministic/human), active verified fact attached to the item: `change.type: default-changed` of a crd-field, cli-flag, env-var, feature-gate or config-key subject | that subject, set to the old default — or to a value stated identically before and after the upgrade |
| `pin:knowledge-exposure@v1` | a trusted fact whose exposure condition is a conjunction with a settable leaf in state `unset` / `not-equals V` | the explicit setting / one of `V` |

The setting must hold **everywhere it matters**: in every complete environment render target (controller flags,
env vars, feature gates, ConfigMap keys — via `render.Inventory`) and on every visible object of the kind
(environment files and rendered objects). A setting that is absent, set only on some objects or installs, set only
after the upgrade to a non-old value, or changed across it is never a pass; neither is any rule from a consensus
or proxy fact, an incomplete render, a partially collected input, or an unparsable environment file that may
declare objects of the kind. Facts come from the bundle's `knowledge-fact` artifacts related to the item and from
`WithFacts` (attached through the report's knowledge findings or `impact.AttachedChanges`).

**SchemaCompat** — compatibility items: the snapshot's Kubernetes version against the target constraint
(`upgrade.EvaluatePlatformConstraint`; anchors `<platform>` or `<platform>/<kind>`): inside → `pass`; outside a
supported/minimum/chart-kubeVersion range → `fail`; outside a *tested* range, other platforms, no cluster version →
`not-run`. API-schema items anchored on a computed CRD change (`impact.CRDIdentity` scopes the kind exactly as
the join): every visible object of the kind (environment files + environment to-renders) against the change —
removed CRD/version still used, removed field set, removed enum value used, newly required field omitted (parent
present), value of the old type → `fail`; all compatible with complete inputs → `pass` for those objects. Default,
storage and additive changes → `not-run`; no visible object → `not-run` (absence in a repository is not proof).

**StaticReference** — on the item's paths: an edge whose provider the target render no longer renders
(`resource-removed`) while its consumer survives unchanged → `fail` (dangling reference, named); every edge
established, no placeholder, no provider removed → `pass` for those paths. Unresolved edges, placeholders, a
truncated traversal, an incomplete render or a root-only path → `not-run`. The property is references only: it
never clears a behaviour item and never passes a `scope:rendered-unexplained` item.

Tests: `go test ./internal/investigate/validate` (synthetic bundles of a fictional product; every output is
checked with `Investigation.ValidateAgainst`). Real bundles from the groundtruth inputs (bundle + renderdiff +
relations, offline): `RI_VALIDATE_STATE=<warm .ri> go test ./internal/investigate/validate -run Groundtruth -v`
(`RI_VALIDATE_VERBOSE=1` logs every record).

## Relationships

Secondary supporting context (the rendered diff is the centerpiece). Package `internal/investigate/relations`
answers one question for the agent and the policy: **which consumers reference this changed object, and how well
do we know it?** Design: DESIGN.md §2.5.

**Graph** (`relations.NewBuilder(store, opts...)`, producer `relations.builder@v1`). Nodes come from the TO
render of every complete environment pair, the environment's YAML files, and every object an environment rendered
change names (so a removed object can still be a root). Edges point in the direction impact propagates (provider →
consumer):

| Edge | From → to | Source |
|---|---|---|
| `references` | Secret / ConfigMap / ServiceAccount → workload | pod template volumes (incl. projected), `env`/`envFrom`, `serviceAccountName`, `imagePullSecrets` |
| `references` | Service / Secret / IngressClass → Ingress | backends, `defaultBackend`, `tls[].secretName`, `ingressClassName` / class annotation |
| `references` | referenced object → custom resource | generic `*Ref` fields `{name, kind[, group, namespace]}`; Flux `valuesFrom` |
| `selects` | workload → Service | `spec.selector` ⊆ pod template labels |
| `serves-api` / `consumes-api` | CRD → api-type → custom resource | CRD `spec.versions`, object `apiVersion`/`kind` |
| `produces` | custom resource → object it creates | declarative `ProducesRule`, else the `spec.secretName` convention |
| `renders-to` / `deploys` / `configures` | Application, kustomization, render source → objects; values file → Application | Argo `source.path`, `$values/…` value files, kustomize `resources`, generators, render targets |

Kustomize `namespace:` (outermost wins) and an Application's `destination.namespace` are applied so references
resolve; kustomize patch files are not objects; `configMapGenerator`/`secretGenerator` outputs are nodes.

**Status = how well a relationship is known.** `established` is an explicit name reference or resolved selector in
the snapshot. A reference to something the snapshot does not contain becomes a **placeholder node** and a `missing`
edge (reason stated) — never dropped. A name-only match (a namespace unknown on either side) and the generic
`spec.secretName` produces-convention are `inferred`. A declared `produces` relationship (below) makes the produces edge and the reference through it `established`, so a
path Certificate → Secret → Deployment can be complete.
Unresolved (inferred/missing) edges can only add paths that need validation: a path through one leaves the
traversal incomplete, which blocks `not-affected` (fail closed).

**Exposure** (`relations.NewExposure()`): the roots of an in-scope change item are the graph nodes of the
environment rendered changes correlated to it (rule `exposure:rendered-change`), then the nodes the deterministic
join's matches name (`exposure:join-match`; a subject is a node key, `Kind/name` or `namespace/Kind/name`). One
root per (item, node); an item with no signal has no root.

**Traversal** (`relations.NewTraverser()`, `graph.dfs@v1`): roots sorted by (item, node id); outgoing edges by
(kind, to-node id, edge id); simple paths only; a maximal path is emitted when no outgoing edge leads off the
path, so every edge that some simple path from a root can take lies on an emitted path (an edge that would
re-enter a node already on the path is on none). Bound `{MaxDepth 8, MaxPaths 500, MaxNodes 5000}` by default;
hitting any bound sets `Truncated` and a reason (`max depth N reached`, `max paths N reached`, `max nodes N
reached`) and the traversal is not complete. Path status is the weakest edge status; `Unresolved` lists exactly the
non-established edges on paths. Same graph, roots and bound ⇒ byte-identical result (input order does not matter).

**Declared relationships** (`relationships:` in a product definition, `internal/catalog/relationships.go`,
validated by `ri catalog validate`/`catalog.Validate`, schema `schemas/product-definition.schema.json`). Object
kinds relate in ways field semantics cannot show, so the catalog states them, each citing upstream API/CRD
documentation with a verbatim quote (never an environment or an evaluation case):

```yaml
relationships:
- id: certificate-secret
  kind: produces            # produces | references | selects
  from: {group: cert-manager.io, kind: Certificate}
  to: {kind: Secret}
  path: spec.secretName     # dotted keys; a key with dots/slashes in brackets: metadata.annotations[a.b/c]
  format: name              # or namespace/name (default name = namespace of `from`)
  scope: namespaced         # or cluster (scope of `to`)
  whenPath: spec.issuerRef.kind   # optional condition on `from`; with whenEquals
  summary: ...
  references: [{url: https://…, quote: "verbatim, ≤400 chars"}]
- id: servicemonitor-services
  kind: selects             # to: Service (metadata.labels) or Pod (pod-template labels of workloads)
  from: {group: monitoring.coreos.com, kind: ServiceMonitor}
  to: {kind: Service}
  selectorPath: spec.selector                 # a LabelSelector: matchLabels + matchExpressions
  namespaceSelectorPath: spec.namespaceSelector   # {any, matchNames}; default: the selector's own namespace
```

`produces` makes the edge from → to (status `established`; a missing `to` object becomes a placeholder explained by
its producer); `references` links `to` → `from`; `selects` links the selected object → the selector. A field that
does not hold the documented form (e.g. no `namespace/name`) yields no edge. `relations.NewBuilder(store,
relations.WithCatalog(cat))` applies the relationships of every product in the catalog (rules of products the
environment does not use match no object); `WithRelationships(product, rs...)` and `WithProducesRules` add rules
directly. Edges carry the rule id `relationship:<product>/<id>`. Shipped: cert-manager (Certificate secret,
Issuer/ClusterIssuer CA secrets, `issuerRef` to a ClusterIssuer), ingress-nginx (`auth-secret`,
`auth-tls-secret`, `proxy-ssl-secret` annotations), prometheus-operator (ServiceMonitor, PodMonitor). A ClusterIssuer
reading its CA Secret stays `inferred`: the issuer does not state the cluster resource namespace.

Limits (milestone 1): no NetworkPolicy/Gateway-API selectors, no live observation, no values-file `configures` for
Helm values outside Argo `$values/` references; cluster scope of a kind is known only for built-in kinds, declared
`scope: cluster` and CRDs in the snapshot.

## Investigation agent

The agent stage (DESIGN.md §4; code `internal/investigate/agent`, CLI `ri investigate mcp`). It is
**opt-in** (`AgentConfig.Enabled`, `--agent`). When it is on, environment configuration from the bundle is
sent to the configured model provider. Kubernetes `Secret` values and credential-named values are redacted
at collection, before anything is hashed or served (see "Evidence bundle"), and sensitive rendered values
are never recorded. Whether provider data sharing beyond that is acceptable is still an open question for
the user, so keep the stage off for real customer environments until it is answered. When the stage is off,
the run records `agent: not-run` and nothing is sent anywhere.

**What the agent can do.** It works through 13 tools, all scoped to one investigation. It never receives
the bundle itself, only the manifest it can search.

| Tool | |
|---|---|
| `list_renders`, `query_diff`, `get_rendered_object` | the rendered diffs: the primary evidence (lane renderdiff's query engine) |
| `get_change_item` | one item with its upstream change text, evidence, source documents, correlated rendered changes and join findings |
| `search_manifest`, `read_artifact` | artifacts by terms/kind/item plus a bounded RE2 grep; line windows (≤400 lines / 64 KiB) or one YAML node |
| `expand_graph`, `list_paths` | the relationship graph (depth ≤3) and impact paths |
| `record_hypothesis`, `record_assertion` | `raises-concern` / `explains` / `claims-mitigation`, optionally a proposed (always inferred) edge |
| `request_evidence` | recorded for the next collection round; live-cluster reads are rejected in milestone 1 |
| `request_validation` | runs the configured deterministic validators of that kind now; the validator's record is the result |
| `submit_findings` | one closing summary per item (recorded as `explains`); ends the run |

**What it cannot do.** Classify. Every agent record carries AI provenance (`investigate.agent@v1`, model and
version as reported, prompt version and digest, input evidence, call id). The deterministic policy
(DESIGN §3.3) only lets such records raise concern. Validations come from validators, never from the agent.

**Validation of every call.** Before anything is recorded, the tool layer checks each call:

- Arguments are decoded strictly; unknown fields are rejected.
- Every id must exist in this run: change items, artifacts, rendered changes, nodes (id or object key),
  impact paths, hypotheses.
- Every hypothesis, assertion and summary needs at least one citation `{artifact, locator, quote}`. The quote
  must occur verbatim in the artifact's bytes (`investigate.QuoteOccurs`, whitespace-insensitive). Each
  citation then becomes a `domain.Evidence` (kind by origin, `contentDigest` = artifact digest) in the run
  pool.

A failed check returns `ErrRejected` to the model as a tool error, and nothing is recorded. Every call,
accepted or not, still counts against the budget and goes into the transcript.

**Budgets** (`DefaultAgentBudget`; unset fields take the default): 40 turns, 80 tool calls, 600 000 tokens,
4 MiB returned, 1200 s.

- The tool layer counts tool calls and returned bytes. Once a budget is spent, every further call gets
  `ErrBudgetExhausted`.
- The driver checks turns, tokens and wall clock. Tokens are the envelope's input, cache-write and output
  tokens; cache reads are reported but not counted. The CLI can overshoot `--max-turns`, so turns are also
  checked after the fact. An accepted submission is allowed one closing turn.

Exhaustion is an outcome (`budget-exhausted`). It keeps every record made so far and makes automated
approval ineligible.

**Drivers** (`AgentConfig.Driver`):

| Driver | How |
|---|---|
| `claude-cli` | `claude -p --session-id <uuid> --mcp-config <tmp>/mcp.json --strict-mcp-config --tools "" --allowedTools mcp__ri --permission-mode dontAsk --max-turns N --system-prompt … --setting-sources "" --no-session-persistence --output-format json`, in a neutral temporary directory. The MCP config starts `ri investigate mcp --attach <socket> --token <t>`, a byte bridge back to the in-process session, so validation, budgets and the transcript live in one place. The session id is chosen up front and becomes every record's call id; model and version are read from the envelope's `modelUsage`. No API key: the CLI must be logged in. |
| `exchange` | the `-llm-exchange` directory protocol, one request file per tool round (`{"calls":[{"tool","arguments"}]}`). An unanswered round ends the run as `failed: exchange-pending: <request file>`; re-running replays the answered rounds (deterministic results, same digests) and continues. A response without a `callId` uses `exchange:<request digest>`. |
| `replay` | re-issues a recorded transcript's calls under their recorded call ids and compares every result digest. Any mismatch, or a different prompt digest, fails the run with the first difference named. |
| `scripted` | a fixed list of calls (tests). |

**Transcript** (`agent-transcript` artifact, `derived/agent/<tag>/transcript.jsonl`, format
`ri.dev/agent-transcript/v1`). It has three kinds of line:

- a `session` line: bundle, prompt version and digest, model, budget;
- one `call` line per tool call: call id, tool, canonical arguments, ok/error, `resultDigest` (sha256 of the
  exact bytes returned to the model), counters;
- an `envelope` line: the driver result and the provider envelope.

Results carry no wall-clock fields (`checkedAt`, `generatedAt`, … are stripped), so a replay reproduces the
transcript byte for byte.

**Prompts** are embedded, versioned files: `internal/investigate/agent/prompts/investigate-agent/v1/`
(`system.md`, `task.md` template, `tools.json` schemas). `PromptDigest = sha256(system ‖ rendered task ‖ tools.json)`.

The task lists:

- the in-scope items: id, title, family, manifestation, upstream citations, correlated rendered changes;
- the render summary;
- the uncorrelated rendered changes and unresolved edges;
- the deterministic validations already run.

The system prompt states what the agent's records can and cannot do. It also tells the agent that, for
`controller-behaviour` and `unknown` items, render silence proves nothing: it should name the setting that
pins the old behaviour and request an override inspection. A prompt change is a new version. The prompts
are generic: never tuned to a case.

**`ri investigate mcp --run <inv-id>`** serves a stored investigation read-only to any MCP client: the query
tools work, and record/request/submit tools are refused.

**Wiring (for the runner).**

- Create the stage with `agent.New(store, agent.WithValidators(...), agent.WithDriver(name, d))`;
  `claude-cli` is registered by default.
- `agent.MergeGraph(graph, outcome)` adds the agent's proposed edges (inferred) and placeholder nodes
  (`AgentOutcome.ProposedNodes`) for the post-agent re-traversal (DESIGN §5.1).
- Agent-requested validations are in `AgentOutcome.Validations`. Their evidence and artifacts are in the
  outcome's `Evidence` / `Artifacts`, alongside the transcript.

**Live run.** One `claude -p` run on a groundtruth input is recorded in
[docs/phase4/env-impact/runs/agent-live-1/](phase4/env-impact/runs/agent-live-1/README.md). It covers what
the agent did, and its replay check.
