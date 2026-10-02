# Why is applicability UNKNOWN? — per-link analysis of the 21 environment links

> Lane `analysis` (MISSION Goals 17, 18, 22). Read-only analysis; no code, case, fact or result was
> changed. Every number below was recomputed from a fresh run of the code at `p3-learning-loop`
> (= main @ c341555 + fleet docs); the baseline reproduces exactly (applicabilityAccuracy 0.095 = 2/21,
> 47 labelled confusion cells, weighted miss 221).

## 0. Bottom line

1. **The 21 links split four ways:**

   | Bucket | Links | What closes it |
   |---|---|---|
   | Hit today | 2 | — (but both hit with the **wrong class**: INFORMATIONAL "does not apply to you" where ACTION is expected) |
   | Closable deterministically | 3 for sure (+3 if new artifact capture is built) | linking prose changes to computed diffs they restate; a `crd:storage-changed` join rule; capturing CRD enums, OCI chart values and Kafka-version tables |
   | Need a human-verified fact (env evidence already in the fixture) | 8 | release-level semantic facts + env parsing extensions (manifest scalar values, list descent, embedded text, value predicates) |
   | Need a new environment input (cross-product inventory) | 3 | the `envinv` inventory **plus fixture inventory files** recording what today lives only in descriptions/comments |
   | Not honestly reachable as a hit | 2 | nothing: the fixture contradicts the label (precondition already met; OPA not configured) |

2. **Ceiling estimates** (21-link denominator, labels unchanged, §3.3):
   (a) deterministic validation only **0.24** (5/21), or **0.38** (8/21) with new declarative artifact
   capture; (b) + human-verified facts **0.76** (16/21); (c) + cross-product inventory **0.90** (19/21).
   **0.80 is out of reach without new environment inputs**: verified knowledge alone tops out at
   16/21, one link short. With (c), 0.80 is reachable with two links to spare.
3. **Two of the 21 cannot be hit honestly** (cilium-1.15 E1, strimzi E6). The fixtures show these
   items do *not* need action, so the honest class is NOT AFFECTED. They are dataset issues (§4), not
   engine gaps.
4. **Two links are measurement artefacts, not engine misses.** In cilium-1.15 E6 and cilium-1.16 E3
   the engine **already emits `impact:values-removed` · ACTION REQUIRED · high** for the user's
   `bgp.enabled`, which is exactly the subject the `why` describes. The items' matchers
   (`metallb-bgp`, `bgpControlPlane`, …) can't see the computed change
   `Helm values section bgp.* removed (3 keys)`. The cases' own `expectedFindings` (F3 / F1
   `subject: bgp.enabled`) do find it. These are 2 of the dataset's 3 ACTION findings.
5. **None of the 5 "catastrophic" ACTION → NOT-AFFECTED cells is the engine clearing an item's own
   subject.** In 3 of them a loose matcher pinned an ACTION label onto an incidental computed change
   that is correctly not-affected. The other 2 are a real semantic hole: no *replacement
   relationship* exists, so "you don't set the new key `tls.readSecretsOnlyFromSecretsNamespace`" reads
   as clear even though the user pins the deprecated key it replaces (§2.2).
6. **The dominant cause is release-knowledge-gap: 11 of 19 unhit links** (12 if you count karpenter
   E7, where the gap comes from a missing artifact channel). The environment evidence
   that decides the link is *already parsed* for 11 of 21 links. It is present but not parsed for 6,
   mostly because `env.ManifestField` keeps no scalar value and never descends into lists. It is absent
   for 3, and in 1 case (strimzi E6) the fixture contradicts the claim.
7. **Design inputs for `contract`** (§3.5): facts must attach to *every* change that restates them, not
   to one change id; the predicate language needs about 10 operators (listed); and one deliberate
   tension to resolve: the MISSION demo puts rotationPolicy at ACTION REQUIRED, but this dataset labels
   it review. If the engine produced ACTION on that fixture today, the current evaluator would count
   it as a **false action**.

## How the data was produced (reproducible, offline)

```
go build -o bin/ri ./cmd/ri
bin/ri -offline -state /Users/tommydavison/repos/Release-Intelligence-/.ri eval -o json > eval.json
# per environment case, exactly the inputs internal/eval/run.go assembles:
bin/ri -offline -state <primary>/.ri impact -kubernetes <k> [-values …/values.yaml] [-manifests …/manifests] \
       [-crds …/crds] [-images …/images.txt] -o json <product> <from> <to>
bin/ri -offline -state <primary>/.ri upgrade -o json <product> <from> <to>
```

Scoring semantics used throughout (`internal/eval/compare.go`):

- A link is **hit** only when an AFFECTED-class finding (action / review / informational) joins a change
  that one of the item's matchers selects.
- A confusion cell is one (change × expected item) pair. Its actual value is the strongest class among
  that change's findings (order: action > review > info > unknown > not-affected).
- Each change is attributed to **one** item only: the last item in case order whose matchers select it
  (`expIDForChange`, last write wins). Three changes are shadowed this way (§4, D13).

Vocabulary in the tables: **NMCS** = the `neededToDetermine` text "no machine-comparable subject
(declared change); the deterministic join evaluates computed values/CRD/image diffs and compatibility
constraints only". **Tier** = the cheapest ladder step that yields an honest hit: `a` deterministic,
`b` + human-verified fact, `c` + cross-product inventory, `—` none.

---

## 1. Per applicability link (21)

### Summary table

| # | Case · link | Relevance | Hit | Dominant cause (secondary) | Subject family | Deciding env evidence | Deterministic validation? | Honest reachable class | Tier |
|---|---|---|---|---|---|---|---|---|---|
| 1 | argo-cd-2.14-3.0 · E1 | action | no | release-knowledge-gap | rbac-permission | present, **not parsed** (policy.csv text) | no — prose | ACTION (AFFECTED) | b |
| 2 | argo-cd-2.14-3.0 · E2 | action | no | release-knowledge-gap (env-visibility: argocd-cm absent) | rbac-permission + removed config key | policy.csv present not parsed; argocd-cm **absent** | no — prose | REVIEW (ACTION once argocd-cm supplied) | b |
| 3 | argo-cd-2.14-3.0 · E3 | review | no | cross-product-context-gap (env-visibility) | config-key default × product-relationship | **absent** (cert-manager only in description; argocd-cm absent) | partly (default list is in the install manifest) | UNKNOWN as supplied → REVIEW with inventory | c |
| 4 | cert-manager-1.16-1.17 · E1 | review | no | release-knowledge-gap (runtime-behavior) | protocol-behavior keyed on crd-field values | present, **not parsed** (scalar values) | no — prose | REVIEW | b |
| 5 | cert-manager-1.16-1.17 · E2 | action | no | release-knowledge-gap | feature-gate (via helm-value string) | present **and parsed** (values.yaml L2) | no — gate list lives in Go source | REVIEW honestly (deprecation; still works in 1.17), ACTION per label | b |
| 6 | cert-manager-1.16-1.17 · E3 | info | no | release-knowledge-gap | feature-gate default change | present and parsed | no | INFORMATIONAL | b |
| 7 | cert-manager-1.17-1.18 · E1 | review | no | release-knowledge-gap | crd-field default change | present and parsed (field unset) | subject yes (CRD schema path); change no | REVIEW | b |
| 8 | cert-manager-1.17-1.18 · E3 | action | no | cross-product-context-gap (env parse: lists) | compatibility-boundary (cross-product) | ingress-nginx version **absent** (YAML comment only) | no — prose | UNKNOWN as supplied → REVIEW with inventory, ACTION with ingress-nginx config | c |
| 9 | cilium-1.15-1.17 · E1 | action | no | semantic-ambiguity / **label not supported** | migration precondition (from-version) | present (edge `from` v1.15.6; images.txt) | no — prose | **NOT AFFECTED** (precondition met) | — |
| 10 | cilium-1.15-1.17 · E6 | action | no | **measurement artefact** (prose change not linked to its computed proof) | helm-value removed | present and parsed | **yes** — values diff already computed | ACTION (already emitted on the computed change) | a |
| 11 | cilium-1.15-1.17 · E8 | action | **yes** (INFO) | semantic-ambiguity (replacement relationship) | helm-value deprecated → replaced | present and parsed | partly (diff proves old key nulled + new added; "replaced-by" is prose) | REVIEW (ACTION if the key is ignored in 1.17) | hit |
| 12 | cilium-1.16-1.17 · E3 | action | no | **measurement artefact** (same as #10) | helm-value removed | present and parsed | **yes** | ACTION (already emitted) | a |
| 13 | cilium-1.16-1.17 · E4 | action | **yes** (INFO) | semantic-ambiguity (same as #11) | helm-value deprecated → replaced | present and parsed | partly | REVIEW / ACTION | hit |
| 14 | istio-1.23-1.24 · E1 | action | no | release-knowledge-gap | migration (conditioned on helm-value) | present and parsed (values.yaml L11–13) | key existence yes; procedure no | ACTION | b |
| 15 | istio-1.23-1.24 · E2 | action | no | cross-product-context-gap | compatibility-boundary (cross-product) | istio-csr version **absent** (description only) | no — prose | UNKNOWN as supplied → ACTION with inventory | c |
| 16 | istio-1.23-1.24 · E6 | review | no | release-knowledge-gap (env parse: lists + values) | api attribute (CEL) in crd-field value | present, **not parsed** (inside a list) | no — prose | REVIEW | b |
| 17 | karpenter-0.37.8-1.0.0 · E1 | action | no | release-knowledge-gap (**join-rule gap** on a computed change) | gvk / CRD storage version | present and parsed (GVK usage) | **yes** — `crd:storage-changed` is computed | REVIEW via deterministic rule; ACTION with verified migration fact | a |
| 18 | karpenter-0.37.8-1.0.0 · E2 | action | no | release-knowledge-gap (env parse: values) | crd-field enum value rename | present, **not parsed** (scalar value) | only if CRD snapshots captured `enum` | ACTION (or REVIEW) | a* / b |
| 19 | karpenter-0.37.8-1.0.0 · E7 | action | no | evidence-gap (chart values not ingested — OCI-only chart) | helm-value removed (env-var rename) | present and parsed (values.yaml L5–11) | only with an OCI chart-values channel | ACTION | a* / b |
| 20 | strimzi-0.45-0.46 · E5 | action | no | release-knowledge-gap (compat as prose; evidence-gap) | compatibility-boundary (operand versions) | present, **not parsed** (MM2 `spec.version`); image tag parsed | only with Strimzi `kafka-versions.yaml` as a compat source | ACTION | a* / b |
| 21 | strimzi-0.45-0.46 · E6 | review | no | **label not supported** (fixture has no OPA) | crd-field value deprecated | **contradicts** the claim | — | **NOT AFFECTED** | — |

`a*` = deterministic only if the named new artifact capture is built; otherwise the same link is
reachable at tier b.

### Per-link detail

Today's findings are listed per matched change (de-duplicated). In every case each listed change
carries exactly one finding.

#### 1. argo-cd-2.14-3.0 · E1 — fine-grained RBAC update/delete no longer cover sub-resources (action-required, critical)
- **Matched change → today:** `chg-d19c3b3199e4` (declared, upgrade guide) → `unknown` · `impact:not-joined` · NMCS.
- **Cause:** release-knowledge-gap. The change has no machine-comparable subject.
- **Missing structure:** subject `rbac-permission {system: argo-cd policy.csv, resource: applications, action: update|delete}`. Change `semantics-narrowed`: before = action also covers the Application's managed resources; after = covers the Application only, and `update/*` / `delete/*` are required for managed resources. Applicability: a policy line grants `applications, update|delete` **and** no line grants `update/*|delete/*`. Consequence `permission-loss` (sync-time edits and deletes of managed resources denied).
- **Env evidence:** present, not parsed. `manifests/rbac.yaml` L9–10 sit inside the `data["policy.csv"]` block scalar of ConfigMap `argocd-rbac-cm`. The env model records the path `data["policy.csv"]` but keeps no value, and nothing reads line-oriented content. Needs **X1** (manifest scalar values) + **X3** (line predicates over embedded text).
- **Validation:** none from artifacts (RBAC semantics exist only in prose) → human review.
- **Reachable:** ACTION (verified fact + both chains: L9/L10 grant, no `update/*` line) → AFFECTED, hit, class match.

#### 2. argo-cd-2.14-3.0 · E2 — logs RBAC enforced by default; `server.rbac.log.enforce.enable` removed (action-required, critical)
- **Matched change → today:** `chg-459086dd8857` → `unknown` · `impact:not-joined` · NMCS.
- **Cause:** release-knowledge-gap; secondary environment-visibility-gap (whether 2.14 already enforced logs RBAC lives in `argocd-cm`, which the fixture does not supply).
- **Missing structure:** subject `rbac-permission {resource: logs, action: get}` plus `config-key argocd-cm server.rbac.log.enforce.enable` (removed). Change `default-changed` (logs implied by `applications, get` → explicit grant required). Applicability: a role has `applications, get` **and** no `logs, get` grant **and** (2.14 flag unset/false). Consequence `permission-loss` (logs tab disappears).
- **Env evidence:** the grant side is present but not parsed (rbac.yaml L8–11; X1 + X3). The flag side is absent (argocd-cm not in the fixture).
- **Validation:** none → human.
- **Reachable:** REVIEW while argocd-cm is unsupplied (absence is not knowledge: if 2.14 already enforced, nothing changes). ACTION once argocd-cm is supplied and shows the flag unset. AFFECTED either way → hit (at worst ACTION → REVIEW, weight ×1).

#### 3. argo-cd-2.14-3.0 · E3 — default `resource.exclusions` now excludes CertificateRequest, Cilium kinds, … (review)
- **Matched changes → today:** `chg-9cbe62a32f33` → `unknown` · not-joined · NMCS. `chg-90bd9b5f3685` (computed `values:added` 9 new `configs.cm[...]` keys) → `unknown` · `impact:insufficient-visibility` · "Helm values files (--values) not supplied". The second change is attributed to E5 by last-wins shadowing (D13).
- **Cause:** cross-product-context-gap (does another product's kind exist here?). Secondary environment-visibility-gap (does the user's argocd-cm override `resource.exclusions`?).
- **Missing structure:** subject `config-key argocd-cm resource.exclusions`, change `default-changed` (unset → list incl. `cert-manager.io/CertificateRequest`, `cilium.io/CiliumIdentity|CiliumEndpoint|CiliumEndpointSlice`, `discovery.k8s.io/EndpointSlice`, …). Applicability: argocd-cm does not set `resource.exclusions` **and** the environment runs a product or kind in the list (`product-relationship: argo-cd manages kinds of cert-manager`). Consequence `visibility-loss` (objects of those kinds leave Argo CD's resource tree).
- **Env evidence:** absent. No file in the fixture shows cert-manager. Only the case description says so. argocd-cm is not supplied.
- **Validation:** partial. The default exclusion list is in the published install manifest (an artifact Argo CD ingests, but only for image refs). Whether a given cluster runs those kinds is environment knowledge.
- **Reachable:** honestly UNKNOWN on the fixture as supplied. With an inventory naming cert-manager (or a CertificateRequest-bearing GVK) and argocd-cm still unsupplied: REVIEW ("credible overlap, can't prove the override is absent") → hit, class match. NOTES.md already calls this "an expected honest miss".

#### 4. cert-manager-1.16-1.17 · E1 — RSA 3072/4096 keys now signed with SHA-384/512 (review)
- **Matched changes → today:** `chg-1cd1a2462c54`, `chg-5facf6216af6`, `chg-be5c1fe63bfd` → all `unknown` · not-joined · NMCS (3 REVIEW → UNKNOWN cells).
- **Cause:** release-knowledge-gap. Secondary runtime-behavior-gap (whether the downstream verifier accepts SHA-512 is outside any static input; this is why the honest class is review, not action).
- **Missing structure:** subject `protocol-behavior {signature hash of CA/SelfSigned-issued certificates}`. Change `behavior-changed`: before SHA-256 always; after SHA-384 for RSA 3072, SHA-512 for RSA ≥ 4096. Applicability: a `cert-manager.io/v1 Certificate` with `spec.privateKey.algorithm == RSA` and `spec.privateKey.size ∈ {3072, ≥4096}` whose `issuerRef` resolves to a CA or SelfSigned issuer. Consequence `interop-risk`.
- **Env evidence:** present, not parsed. `manifests/internal-ca.yaml` L23–25 (`algorithm: RSA`, `size: 4096`): the paths are recorded, the values are not (**X1**). The issuer type sits at L6 (`spec.ca`) on the ClusterIssuer that `issuerRef` names. That needs **X5** (cross-resource reference), or a coarser "a CA issuer is in use" condition.
- **Validation:** none → human.
- **Reachable:** REVIEW → hit, class match.

#### 5. cert-manager-1.16-1.17 · E2 — ValidateCAA feature gate deprecated, removed in 1.18 (action-required)
- **Matched changes → today:** `chg-0f79940f5f35`, `chg-fdbf4141cd23` → `unknown` · not-joined · NMCS. `chg-123447d3ab93` ("Feature Flag Promotions / Deprecations") also matches, but is shadowed to E3 (D13).
- **Cause:** release-knowledge-gap.
- **Missing structure:** subject `feature-gate {component: cert-manager-controller, name: ValidateCAA}`. Change `deprecated` (removal announced for 1.18). Applicability: the gate is enabled in the user's configuration. With the Helm convention, values key `featureGates` contains token `ValidateCAA=true`. Consequence `future-removal`.
- **Env evidence:** present **and parsed**. values.yaml L2 `featureGates: "ValidateCAA=true"`; `env.ValuesKey.Value` keeps the JSON leaf. Needs only a join-side predicate (**X4**: token in a delimited `k=v` list).
- **Validation:** none (the gate registry is Go source) → human.
- **Reachable:** honestly REVIEW. The gate still works in 1.17, and the engine treats deprecations as review everywhere else (`impact:crd-version-deprecated`). The label says ACTION. AFFECTED either way → hit (D7).

#### 6. cert-manager-1.16-1.17 · E3 — NameConstraints, UseDomainQualifiedFinalizer promoted to beta and on by default (informational)
- **Matched changes → today:** `chg-123447d3ab93`, `chg-929e3fd06ae3`, `chg-2965d6c746c8` → `unknown` · not-joined · NMCS (3 INFO → UNKNOWN cells).
- **Cause:** release-knowledge-gap.
- **Missing structure:** subject `feature-gate {NameConstraints}`, `{UseDomainQualifiedFinalizer}`; change `default-changed` false → true. Applicability: `featureGates` does **not** contain `<gate>=false` (i.e. the user doesn't opt out). Consequence `behavior-change` (no action expected).
- **Env evidence:** present and parsed (values.yaml L2 names neither gate). X4.
- **Validation:** none → human.
- **Reachable:** INFORMATIONAL → hit, class match.

#### 7. cert-manager-1.17-1.18 · E1 — Certificate `spec.privateKey.rotationPolicy` default Never → Always (review; no item class)
- **Matched changes → today:** `chg-a726b336f865`, `chg-0e1cdbe373a9`, `chg-fa4ce35f9a12` (three restatements of one statement) → all `unknown` · not-joined · NMCS.
- **Cause:** release-knowledge-gap.
- **Missing structure:** subject `crd-field {group: cert-manager.io, kind: Certificate, path: spec.privateKey.rotationPolicy}`. Change `default-changed` Never → Always. Applicability `fieldState: unset` on resources of that GVK. Consequence `behavior-change` (private keys rotate on every re-issuance).
- **Env evidence:** present and parsed. `manifests/certificate.yaml` uses `cert-manager.io/v1 Certificate` (GVK usage) and sets no `spec.privateKey.*` path. Under the precedent of `impact:crd-field-unset`, absence inside supplied manifests is a fact.
- **Validation:** the subject is provable (the path exists in the cert-manager CRD snapshot's `SchemaPaths`). The change is **not**: `CRDVersionInfo` keeps no `default:`, and cert-manager may apply the default in code rather than in the schema. → human, or a CRD-default validator (V3) if the published schema states it.
- **Reachable:** REVIEW → hit, matching the link relevance. **This is the MISSION's demonstration item.** The demo's step 10 says ACTION REQUIRED, but this fixture labels it review, and per ACTION_CLASSIFICATION §3 nothing *fails* if the operator does nothing (keys rotate). An ACTION finding here would be scored as a false action by `falseActionRate` (ground truth names a softer class). `contract` should decide this deliberately (§3.5).

#### 8. cert-manager-1.17-1.18 · E3 — HTTP01 paths now PathType Exact; breaks ingress-nginx ≥ 1.12 strict path validation (action-required)
- **Matched change → today:** `chg-e4f5f001ae5f` → `unknown` · not-joined · NMCS.
- **Cause:** cross-product-context-gap. Secondary: env parse (the HTTP01 solver lives in a list).
- **Missing structure:** subject `compatibility-boundary {product: cert-manager ≥ 1.18.0 (ACME HTTP01 ingress solver) × product: ingress-nginx}`. Change `incompatible-with`: ingress-nginx `>= 1.12.0, < 1.12.6` (and 1.13.0–1.13.1) with `strict-validate-path-type` at its default (true). Applicability: an Issuer/ClusterIssuer uses `acme.solvers[].http01.ingress` **and** inventory has ingress-nginx in range **and** the controller config does not set `strict-validate-path-type: false`. Consequence `failure` (challenges rejected → issuance stops). Workarounds: gate `ACMEHTTP01IngressPathTypeExact=false`, relax validation, or upgrade ingress-nginx.
- **Env evidence:** the ingress-nginx version is absent. It appears only in a YAML comment (certificate.yaml L1–2). The solver is present but invisible: `spec.acme.solvers` is a list, so it is a leaf path (**X2**). The ingress-nginx ConfigMap is absent.
- **Validation:** none (prose on both products) → human.
- **Reachable:** UNKNOWN as supplied (honest). With inventory, REVIEW (the ingress-nginx config is unsupplied, so its default cannot be proven) → hit at ACTION → REVIEW (×1). ACTION only if that config is supplied too.

#### 9. cilium-1.15-1.17 · E1 — toFQDNs migration requires running ≥ 1.15.6 before upgrading to 1.16 (action-required)
- **Matched changes → today:** `chg-23a43a125719` (toFQDNs overhaul), `chg-e45fbe2aa651` (dnsProxy default 50 → 1000, *incidental*), `chg-dcf178959a37` (new metric, *incidental*) → all `unknown` · not-joined · NMCS.
- **Cause:** the label is not supported by the fixture. The upgrade *starts* at v1.15.6 (the edge's `from`, also pinned in images.txt L2–3), so the precondition is met. Whether toFQDNs policies exist is also unknowable (no manifests supplied).
- **Missing structure:** subject `migration {product: cilium, crossing: 1.16.0}`, precondition `running-version >= 1.15.6` when `toFQDNs` policies exist. Applicability is evaluated against the edge's from-version (always known) and, optionally, CNP usage.
- **Env evidence:** present (edge `from`; images.txt).
- **Validation:** none (prose).
- **Reachable:** **NOT AFFECTED** (precondition satisfied; compare `impact:compatibility-satisfied` → not-affected for satisfied minimums). INFORMATIONAL at most, if modelled like `kubernetes-in-range`. Not honestly an ACTION hit (D2). Note the 2 incidental matches: an informational finding on the dnsProxy change in a future run would score this link as "hit" for the wrong reason (D12).

#### 10. cilium-1.15-1.17 · E6 — metallb-bgp integration removed; bgp.* Helm options gone (action-required, critical)
- **Matched changes → today:** `chg-cccfc2f4bcd7`, `chg-185edee7e447`, `chg-991133fe1de5` (declared; the last one literally names `bgp.enabled`, `bgp.announce.*`) → `unknown` · not-joined · NMCS. `chg-c1683026f6bb` (computed `values:added bgpControlPlane.statusReport.enabled`, matched by `(?i)bgpControlPlane`) → `not-affected` · `impact:values-unset`. This is a *matcher artefact* cell (§2.2).
- **Not matched, but decisive:** `chg-8d1c442dccb7` = computed `values:section-removed` "Helm values section `bgp.*` removed (3 keys)" → **`action-required` · `impact:values-removed` · high · confidence high** ("You set 2 Helm values that v1.17.0 removed"). The case's own `expectedFindings` F3 (`rule: impact:values-removed, subject: bgp.enabled`) finds it.
- **Cause:** a measurement artefact. In knowledge terms it is a missing *restatement link*: the prose change and the computed diff that proves it are not connected.
- **Missing structure:** subject `helm-value {chart: cilium, path: bgp.enabled, bgp.announce.loadbalancerIP, bgp.announce.podCIDR}`, change `removed` (replacement `bgpControlPlane.*`). Applicability: key set. Consequence `config-ignored` → the existing `impact:values-removed` rule. *Or*, equivalently, a relationship `restates(chg-991133fe1de5, chg-8d1c442dccb7)`.
- **Env evidence:** present and parsed (values.yaml L5–8).
- **Validation:** **yes, already done.** The values diff is the deterministic proof. A model proposing this subject for `chg-991133fe1de5` is verified without human review: the canonical tier-a path.
- **Reachable:** ACTION, already produced → hit with class match, as soon as the finding is attached to a matched change (or the matcher sees the computed change, D1).

#### 11. cilium-1.15-1.17 · E8 — `tls.secretsBackend` deprecated → `tls.readSecretsOnlyFromSecretsNamespace` (action-required) — **HIT**
- **Matched changes → today:** `chg-5d7ba30d19f8` (computed default `"local"` → `null`) → `informational` · `impact:values-pinned` · low, "You pin `tls.secretsBackend`; its default change in v1.17.0 does not apply to you" (**the hit**). `chg-37c4c95379a8` (computed 4 new `tls.*` keys) → `not-affected` · `impact:values-unset`. `chg-5630efd4738a` (TLS visibility via SDS; related prose) → `unknown` · NMCS.
- **Cause:** semantic-ambiguity. The engine sees "default changed and the user pins it" and concludes the pin keeps winning. It lacks the *deprecated-and-replaced-by* relationship that makes the pin the problem.
- **Missing structure:** subject `helm-value tls.secretsBackend`, change `deprecated` with `replacedBy: tls.readSecretsOnlyFromSecretsNamespace` (+ `tls.secretSync.*`) and value mapping `k8s → readSecretsOnlyFromSecretsNamespace: false`. Applicability: old key set **and** replacement unset. Consequence `config-migration-required` (`config-ignored` if 1.17 templates no longer read it — unverified, D8).
- **Env evidence:** present and parsed (values.yaml L9–10).
- **Validation:** partial. The diff proves the old key was nulled and the new keys added in the same release. "Replaced-by" and the value mapping are prose → human. (Good auto-verification-candidate heuristic: old key nulled + new sibling added + prose names both.)
- **Reachable:** REVIEW (ACTION if the key is ignored in 1.17) → still a hit. The class moves from ACTION → INFO (wrong, misleading text) to ACTION → REVIEW (×1) or a match.

#### 12. cilium-1.16-1.17 · E3 — metallb-bgp removed (action-required)
Identical to #10. The same changes (`chg-cccfc2f4bcd7`, `chg-185edee7e447`, `chg-991133fe1de5` → unknown; `chg-c1683026f6bb` → not-affected artefact). Unmatched `chg-8d1c442dccb7` → **`action-required` · `impact:values-removed`** ("You set 3 Helm values that v1.17.0 removed"), found by the case's own F1 (`subject: bgp.enabled`). Env evidence: values.yaml L23–27. Tier a.

#### 13. cilium-1.16-1.17 · E4 — `tls.secretsBackend` deprecated (action-required) — **HIT**
Identical to #11 (`chg-5d7ba30d19f8` → informational values-pinned = the hit; `chg-37c4c95379a8` → not-affected; `chg-5630efd4738a` → unknown). Env evidence: values.yaml L21–22. The CiliumNetworkPolicy reading a Secret from `default` (manifests L19–22, inside a list → X2) is what makes the security-relevant direction concrete. The `why` here says the key "no longer exists in the v1.17.0 chart", which contradicts the `secretsBackend: ~` citation in cilium-1.15 (D8).

#### 14. istio-1.23-1.24 · E1 — ambient upgrade with `cni.ambient.dnsCapture=true` needs the ordered procedure (action-required, critical)
- **Matched changes → today:** `chg-3ac1c957b6ce` (declared) → `unknown` · NMCS. `chg-ceb71dee9753` (computed `values:section-removed defaults.*` 46 keys, chart cni; its subjects include `defaults.cni.ambient.dnsCapture`) → `not-affected` · values-unset. That is a matcher artefact cell: the user doesn't set the chart-internal `defaults.*`, so the verdict is right for that change.
- **Cause:** release-knowledge-gap.
- **Missing structure:** subject `migration {product: istio, target: 1.24.0, procedure: CNI → restart ambient workloads → ztunnel}`. Applicability: helm-value `cni.ambient.dnsCapture == true` (optionally ∧ a namespace labelled `istio.io/dataplane-mode: ambient`). Consequence `failure` (DNS resolution failures).
- **Env evidence:** present and parsed (values.yaml L11–13, value retained). The ambient namespace label is present in `manifests/telemetry.yaml` L7, but only its path is kept (X1 if used). Caveat: the join compares the one supplied values file against every Istio chart's diff (the same file yields the cni `defaults.*` verdict and an istiod `values-new-key` finding). A value meant for the cni chart sits in an istiod-chart values file here, and values-file → chart attribution is not modelled.
- **Validation:** key existence yes (cni chart values snapshot); the procedure requirement no → human.
- **Reachable:** ACTION → hit, class match.

#### 15. istio-1.23-1.24 · E2 — istio-csr ≤ v0.12.0 breaks against 1.24's gRPC ALPN enforcement (action-required, critical)
- **Matched change → today:** `chg-ac6f2b1816e3` → `unknown` · NMCS.
- **Cause:** cross-product-context-gap.
- **Missing structure:** subject `compatibility-boundary {istio ≥ 1.24.0 × cert-manager-istio-csr <= 0.12.0}`, change `incompatible-with` (stricter gRPC/ALPN validation). Applicability: inventory has istio-csr ≤ 0.12.0 **and** values do not set `pilot.env.GRPC_ENFORCE_ALPN_ENABLED: "false"`. Consequence `failure` (CA handshakes fail).
- **Env evidence:** the istio-csr version is absent. It appears only in the case description, and as "istio-csr workaround" in a values comment. The workaround's absence *is* decidable from the supplied values (values absence is a fact).
- **Validation:** none → human.
- **Reachable:** UNKNOWN as supplied (honest). ACTION with inventory → hit, class match.

#### 16. istio-1.23-1.24 · E6 — Telemetry CEL must use standard attributes; `filter_state["wasm.*"]` → `downstream_peer` (review)
- **Matched change → today:** `chg-f772888f69da` → `unknown` · NMCS.
- **Cause:** release-knowledge-gap; secondary env parse.
- **Missing structure:** subject `api-attribute {group: telemetry.istio.io, kind: Telemetry, path: spec.metrics[].overrides[].tagOverrides.*.value, attribute: filter_state["wasm.downstream_peer"|"wasm.upstream_peer"]}`. Change `renamed` → `filter_state.downstream_peer|upstream_peer`. Applicability: a field value matches `filter_state\["wasm\.`. Consequence `silent-data-loss` (tag stops resolving).
- **Env evidence:** present, not parsed. telemetry.yaml L21 sits inside `spec.metrics` (a list → leaf path), so it needs **X2** and **X1**.
- **Validation:** none → human.
- **Reachable:** REVIEW → hit, class match.

#### 17. karpenter-0.37.8-1.0.0 · E1 — v1 APIs; v1beta1 converts via webhooks; migrate manifests before 1.1.0 (action-required, critical)
- **Matched changes → today:** `chg-52f5b6cdd25d`, `chg-ea7e715783a2`, `chg-80926f75cce1` (computed `crd:storage-changed` v1beta1 → v1 for ec2nodeclasses, nodeclaims, nodepools) → `unknown` · `impact:not-joined` · "diff rule crd:storage-changed has no join rule". `chg-9582e4b00d9c` (computed `crd:fields-added status.conditions[]`, *incidental*) → `unknown` · "crd:fields-added has no join rule". `chg-af6f523688a6` (upgrade-guide intro), `chg-c08bccbe8873` (conversion-webhook PR, supporting) → `unknown` · NMCS. That makes 6 ACTION → UNKNOWN cells. `chg-318315d1ff6b` also matches but is shadowed to E10.
- **Cause:** release-knowledge-gap (what the storage move *requires* of the user is prose), with a **join-rule gap**. The decisive change is computed and machine-comparable, but no rule joins it.
- **Missing structure:** subject `gvk {karpenter.sh/v1beta1 NodePool, NodeClaim; karpenter.k8s.aws/v1beta1 EC2NodeClass}`. Change `storage-version-changed` → v1 (still served; unserved at 1.1.0 = future-removal) + `migration` (v1 migration procedure mandatory). Applicability: GVK in manifest use. Consequence `migration-required`.
- **Env evidence:** present and parsed. GVK usage: nodepool.yaml L1 (`karpenter.sh/v1beta1 NodePool`), L20 (`karpenter.k8s.aws/v1beta1 EC2NodeClass`).
- **Validation:** **yes.** The CRD snapshots prove the storage move and the continued serving of v1beta1. A generic `crd:storage-changed` join rule (exact GVK in manifest use, whose version loses storage → review "migrate stored manifests") is deterministic and product-agnostic.
- **Reachable:** REVIEW via the deterministic rule alone → hit (ACTION → REVIEW ×1). ACTION with a human-verified migration-step fact. The 3 note-derived restatements stay UNKNOWN cells unless facts attach to every restatement (§3.5).

#### 18. karpenter-0.37.8-1.0.0 · E2 — NodePool consolidationPolicy `WhenUnderutilized` → `WhenEmptyOrUnderutilized` (action-required)
- **Matched change → today:** `chg-4dc66974bf31` → `unknown` · NMCS (`chg-318315d1ff6b` shadowed to E10).
- **Cause:** release-knowledge-gap; secondary env parse.
- **Missing structure:** subject `crd-field {karpenter.sh, NodePool, spec.disruption.consolidationPolicy}`. Change `enum-value-renamed` WhenUnderutilized → WhenEmptyOrUnderutilized (in v1). Applicability: field value == `WhenUnderutilized`. Consequence `migration-required` (the conversion webhook bridges it while v1beta1 is served).
- **Env evidence:** present, not parsed. nodepool.yaml L18: the path is recorded, the value is not (**X1**).
- **Validation:** possible only if CRD snapshots captured `enum` per path (V3). They capture paths only today. Otherwise → human.
- **Reachable:** ACTION (REVIEW if the fact's consequence notes the conversion webhook) → hit.

#### 19. karpenter-0.37.8-1.0.0 · E7 — LOGGING_CONFIG / ASSUME_ROLE_* dropped; LEADER_ELECT renamed (action-required)
- **Matched change → today:** `chg-b931c62c0a10` (heuristic) → `unknown` · NMCS.
- **Cause:** evidence-gap. Karpenter's chart is published only to OCI and the definition carries no values snapshots (products/karpenter.yaml; NOTES.md), so no values diff exists. Secondary: the item and matcher talk about env vars while the `why` talks about Helm values (D9).
- **Missing structure:** subject `helm-value {chart: karpenter, path: logConfig.*, assumeRoleARN, assumeRoleDuration}`, change `removed` (replacement `logOutputPaths`/`logErrorOutputPaths`). Relationship `env-var LOGGING_CONFIG ← helm-value logConfig`. Consequence `config-ignored` → the existing `impact:values-removed` rule. Also `controller.metrics.port` `default-changed` 8000 → 8080: the pin keeps winning (informational).
- **Env evidence:** present and parsed (values.yaml L5–11).
- **Validation:** only with a new artifact channel (pull the OCI chart and read its values.yaml, V4). Then the removal is computed and a proposal attached to `chg-b931c62c0a10` verifies deterministically. Otherwise → human.
- **Reachable:** ACTION → hit, *provided the finding attaches to `chg-b931c62c0a10`*. A computed `values:section-removed logConfig.*` change on its own would not match this item's env-var matcher.

#### 20. strimzi-0.45-0.46 · E5 — Kafka 4.0.0 supported, Kafka 3.8.x removed (link action-required; item class review-required)
- **Matched changes → today:** `chg-ddbdbee4c932` ("Add support for Kafka 4.0.0") → `unknown` · NMCS. `chg-9ed47feae8b0` (JMXReporter under Kafka 4.0, *incidental*) → `unknown` · NMCS.
- **Cause:** release-knowledge-gap (a compatibility window written as changelog prose); secondary evidence-gap (the machine-readable table is not ingested).
- **Missing structure:** subject `compatibility-boundary {operator: strimzi 0.46.0, operand: kafka}`, change `support-removed` {3.8.0, 3.8.1} / `support-added` {4.0.0}. Applicability: any `kafka.strimzi.io` resource whose `spec.version` (or `spec.kafka.version`) ∈ 3.8.x, or whose running image tag carries `kafka-3.8.*`. Consequence `failure` (the operator refuses to reconcile unsupported versions).
- **Env evidence:** present, not parsed. `KafkaMirrorMaker2.spec.version: 3.8.0` (kafka.yaml L51) and `Kafka.spec.kafka.metadataVersion: 3.8-IV0` (L26) need **X1**. The image tag `kafka:0.45.0-kafka-3.8.0` is parsed (images.txt L2), but reading the Kafka version out of a tag is product convention.
- **Validation:** possible with a new declarative compat source. Strimzi keeps a machine-readable `kafka-versions.yaml` at each tag. Not ingested today (products/strimzi.yaml carries no compatibility source).
- **Reachable:** ACTION (MM2 pins 3.8.0 explicitly) → hit, class match. The `why`'s own evidence (`metadataVersion 3.8-IV0`) is weaker than it looks (D10).

#### 21. strimzi-0.45-0.46 · E6 — OPA authorization (`type: opa`) deprecated (review)
- **Matched change → today:** `chg-e2faf5a42f72` → `unknown` · NMCS.
- **Cause:** the label is not supported. The `why` says "the fixture's authorization configuration references OPA", but no fixture file has an `authorization` block or `type: opa` (kafka.yaml has none; the CRD file is the schema).
- **Missing structure:** subject `crd-field {kafka.strimzi.io, Kafka, spec.kafka.authorization.type}`, change `value-deprecated` (`opa` → use `custom`). Applicability: value == `opa`.
- **Env evidence:** contradicts the claim. The Kafka CR is in use and sets no `spec.kafka.authorization`.
- **Reachable:** **NOT AFFECTED** (checked, clear) → not a hit (D3).

---

## 2. The confusion cells

47 labelled cells today: ACTION → UNKNOWN 28, REVIEW → UNKNOWN 9, INFO → UNKNOWN 3, ACTION → NOT-AFFECTED 5,
ACTION → INFO 2. Reproduced exactly from the per-change data.

### 2.1 ACTION → UNKNOWN (28 cells)

"Core" = the change states the item. "Incidental" = a loose matcher selects a different statement.
Every one of the 28 is `impact:not-joined`: 25 NMCS (declared/heuristic prose) and 3 "crd:storage-changed
has no join rule" (karpenter).

| Link | Cells | Changes (core / incidental) | Cause | Closes at |
|---|---|---|---|---|
| argo E1 | 1 | d19c3b31 (core) | release-knowledge | b |
| argo E2 | 1 | 459086dd (core) | release-knowledge | b |
| cm-1.16 E2 | 2 | 0f79940f, fdbf4141 (core) | release-knowledge | b |
| cm-1.17 E3 | 1 | e4f5f001 (core) | cross-product | c |
| cilium-1.15 E1 | 3 | 23a43a12 (core); e45fbe2a, dcf17895 (incidental) | label not supported | — (should be NOT AFFECTED) |
| cilium-1.15 E6 | 3 | cccfc2f4, 185edee7, 991133fe (core) | restatement link (measurement) | a |
| cilium-1.15 E8 | 1 | 5630efd4 (related prose) | semantic-ambiguity | b |
| cilium-1.16 E3 | 3 | cccfc2f4, 185edee7, 991133fe (core) | restatement link | a |
| cilium-1.16 E4 | 1 | 5630efd4 (related prose) | semantic-ambiguity | b |
| istio E1 | 1 | 3ac1c957 (core) | release-knowledge | b |
| istio E2 | 1 | ac6f2b18 (core) | cross-product | c |
| karpenter E1 | 6 | 52f5b6cd, ea7e7157, 80926f75 (computed, core); af6f5236 (core); c08bccbe (supporting); 9582e4b0 (incidental) | join-rule gap + release-knowledge | a (3 computed) / b (rest) |
| karpenter E2 | 1 | 4dc66974 (core) | release-knowledge | a* / b |
| karpenter E7 | 1 | b931c62c (core) | evidence-gap | a* / b |
| strimzi E5 | 2 | ddbdbee4 (core); 9ed47fea (incidental) | release-knowledge / evidence-gap | a* / b |

Counts: 21 core, 2 related prose (cilium `5630efd4` × 2), 1 supporting (karpenter `c08bccbe`), 4 incidental
(cilium-1.15 E1 × 2, karpenter `9582e4b0`, strimzi `9ed47fea`). **Cell-level improvement is bounded by fragmentation.** Even
when a link becomes a hit, every restatement that receives no fact stays an UNKNOWN cell. Examples:
karpenter E1 has 6 cells for one judgement; the cilium bgp items have 3 restatements each. Moving cells
requires facts that attach to whole duplicate groups (§3.5).

### 2.2 ACTION → NOT AFFECTED (5 cells) — the dangerous cells

| Case · link | Change (rule → verdict) | Is the not-affected verdict wrong for *that change*? | Reading |
|---|---|---|---|
| cilium-1.15 E6 | `chg-c1683026f6bb` `values:added bgpControlPlane.statusReport.enabled` → `impact:values-unset` | **No.** The user doesn't set the new key, and the item is not about it | matcher artefact (`(?i)bgpControlPlane` selects any bgpControlPlane change). The item's real subject (`bgp.*` removed) is **ACTION** on an unmatched change |
| cilium-1.16 E3 | same change, same verdict | No | matcher artefact (same) |
| istio E1 | `chg-ceb71dee9753` `values:section-removed defaults.*` (46 keys, chart cni) → `impact:values-unset` | No. Chart-internal `defaults.*` restructure; the user sets `cni.ambient.dnsCapture`, not `defaults.cni.ambient.dnsCapture` | matcher artefact (`cni\.ambient\.dnsCapture` matches inside the subject list) |
| cilium-1.15 E8 | `chg-37c4c95379a8` `values:added` 4 new `tls.*` keys → `impact:values-unset` | Locally correct for the `values:added` rule ("a previously ignored key becomes live"), **semantically incomplete** | **real hole.** The new key is the *replacement* for the deprecated key the user pins. "You don't set `readSecretsOnlyFromSecretsNamespace`" is exactly the migration the user still owes. Needs a `replacedBy` relationship (helm-value family) so the join evaluates old-set ∧ new-unset → REVIEW/ACTION |
| cilium-1.16 E4 | same | same | real hole (same) |

None of the 5 is the engine checking an item's own subject and wrongly clearing it. The adversarial
pack's guarantees hold. But two lessons follow:
(1) **the matrix is matcher-sensitive.** 3 of 5 "catastrophic" cells are artefacts of loose
item matchers that label incidental computed changes with the item's class.
(2) **"checked, clear" on a newly added key is only safe when no replacement relationship exists.**
That is a release-knowledge rule the contract should encode, not a join special case.

### 2.3 ACTION → INFORMATIONAL (2 cells) — the two current hits

Both are `chg-5d7ba30d19f8` · `impact:values-pinned` · informational · low, "You pin `tls.secretsBackend`;
its default change in v1.17.0 does not apply to you" (cilium-1.15 E8, cilium-1.16 E4). These count as
hits for applicabilityAccuracy, but the class is wrong and the wording tells the operator the change
doesn't apply. The pin semantics ("your value keeps winning") assume the key stays live. For a
deprecated-and-replaced key that is the wrong conclusion (§1 #11).

### 2.4 REVIEW → UNKNOWN (9) and INFO → UNKNOWN (3)

argo E3 (1 cell, cross-product), cm-1.16 E1 (3, release-knowledge), cm-1.17 E1 (3, release-knowledge,
three restatements of one sentence), istio E6 (1, release-knowledge + list parsing), strimzi E6 (1,
label not supported). INFO → UNKNOWN: cm-1.16 E3 (3, feature-gate defaults).

---

## 3. Aggregates

### 3.1 Cause classes (Goal 18), dominant per link

| Cause | Links | Which |
|---|---|---|
| release-knowledge-gap | 11 | argo E1, E2; cm-1.16 E1, E2, E3; cm-1.17 E1; istio E1, E6; karpenter E1 (join-rule sub-cause), E2; strimzi E5 |
| cross-product-context-gap | 3 | argo E3, cm-1.17 E3, istio E2 |
| semantic-ambiguity | 2 (both hit) | cilium-1.15 E8, cilium-1.16 E4 (replacement relationship) |
| evidence-gap | 1 | karpenter E7 (chart values channel) |
| measurement artefact (prose change not linked to its computed proof) | 2 | cilium-1.15 E6, cilium-1.16 E3 |
| label not supported by the fixture | 2 | cilium-1.15 E1, strimzi E6 |
| environment-visibility-gap | 0 dominant (3 secondary) | secondary in argo E2, argo E3, cm-1.17 E3 |
| runtime-behavior-gap | 0 dominant (1 secondary) | cm-1.16 E1 (why the class is review) |

Among the 19 unhit links, release-knowledge-gap dominates 11, and a 12th (karpenter E7) is a
release-knowledge gap caused by a missing artifact channel. The two "measurement artefact" links are,
in knowledge terms, release-knowledge-*linkage* gaps that a deterministic validator closes. Goal 18's
taxonomy has no slot for "the label is wrong" or "the measurement can't see a correct finding".
**Recommend `contract` keep UNKNOWN reasons to the six engine-side classes** and treat the other two as
eval/dataset issues, never as UNKNOWN reasons on findings.

### 3.2 Subject families

| Family | Links | Links |
|---|---|---|
| helm-value (removed / deprecated → replaced / gating a migration) | 6 | cilium E6/E3, cilium E8/E4, karpenter E7, istio E1 |
| compatibility-boundary (cross-product or operand versions) | 4 | argo E3 (product-relationship), cm-1.17 E3, istio E2, strimzi E5 |
| crd-field (default / enum value / deprecated value) | 3 | cm-1.17 E1, karpenter E2, strimzi E6 |
| feature-gate | 2 | cm-1.16 E2, E3 |
| rbac-permission | 2 | argo E1, E2 |
| gvk / API version | 1 | karpenter E1 |
| migration precondition (from-version) | 1 | cilium-1.15 E1 |
| api-attribute (CEL expression inside a CR) | 1 | istio E6 |
| protocol-behavior keyed on CR values | 1 | cm-1.16 E1 |

### 3.3 Ceiling estimates

Assumptions for all tiers:
- The denominator stays the current 21 links, with labels unchanged.
- A new finding counts only if it joins a change the item's matchers select. So facts must attach to
  existing change ids, or to every restatement (§3.5).
- Env parsing extensions X1–X5 are built where a tier needs them.
- The evaluator is unchanged.

| Tier | Adds | Links hit | applicabilityAccuracy |
|---|---|---|---|
| today | — | 2 (cilium E8, E4) | 0.095 |
| **(a) deterministic only, existing artifacts** | model-proposed subjects verified by artifacts already ingested: cilium-1.15 E6, cilium-1.16 E3 (values diff); karpenter E1 (generic `crd:storage-changed` join rule) | 5 | **0.24** |
| (a\*) + new declarative artifact capture | CRD `enum` capture (karpenter E2), OCI chart values (karpenter E7), Strimzi `kafka-versions.yaml` as a compat source (strimzi E5); each also needs X1 except E7 | 8 | **0.38** |
| **(b) + human-verified facts** (env evidence already in the fixtures) | argo E1, E2; cm-1.16 E1, E2, E3; cm-1.17 E1; istio E1, E6 (+ the three a\* links if a\* is not built) | 16 | **0.76** |
| **(c) + cross-product inventory** (new env input + fixture inventory files) | argo E3, cm-1.17 E3, istio E2 | 19 | **0.90** |
| unreachable with current labels | cilium-1.15 E1, strimzi E6 (honest class NOT AFFECTED) | — | ceiling 0.905 |

Readings:
- **0.80 needs 17 of 21. Tier (b) delivers 16.** Without a new environment input, the gate cannot pass on
  this dataset even with perfect verified knowledge. With the `envinv` inventory *and* fixture
  inventory files that turn the descriptions' claims ("cert-manager runs here", "istio-csr v0.12.0",
  "ingress-nginx v1.12.1") into structured evidence, the ceiling is 19/21.
- **The fixture inventory files are a dataset change.** They must be authored from the case descriptions
  and NOTES, which predate this analysis, and recorded in NOTES.md. They must not be derived from what
  makes links hit.
- If the groundtruth lane relabels the two unsupported links to `not-affected` (D2, D3), they become
  `notAffectedLinks`. They count as clean decisions when no AFFECTED finding joins them. The ceiling
  then becomes 21/21, but only by honest correction, not by tuning.
- Class quality is a separate axis. Under (b)/(c) about 4 links land on a *softer* class than the label
  (argo E2, cm-1.17 E3, karpenter E1 at REVIEW; cm-1.16 E2 REVIEW vs a label of ACTION). That costs
  classificationAccuracy, not applicabilityAccuracy.
- **unknownRate barely moves under any tier.** Its denominator is 1309 findings, and 957 UNKNOWNs are
  dominated by unrelated note-derived changes (argo-cd alone: 457). Closing all 19 links would turn roughly the 40
  UNKNOWN cells into decisions: 957 → ~917 UNKNOWN, i.e. unknownRate ≈ 0.70. The visible lever for unknownRate is UNKNOWN *reasons* plus routing, not
  link closure.

### 3.4 What unlocks the most links

**Environment parsing extensions (`internal/env`):**

| # | Extension | Links unlocked (alone / with others) | Generic? |
|---|---|---|---|
| X1 | Keep scalar **values** on `ManifestField`, the way `ValuesKey.Value` already does | cm-1.16 E1, karpenter E2, strimzi E5 alone; argo E1, E2 with X3; istio E6 with X2 | yes |
| X2 | **Descend into sequences** with `[]` markers (same syntax as CRD `SchemaPaths`, e.g. `status.conditions[]`) | istio E6, cm-1.17 E3 (solver); strengthens cilium E4 (toFQDNs CNP) | yes |
| X3 | Line-addressable **embedded text** in ConfigMap `data` values (line predicates with line-level evidence) | argo E1, E2 | yes (the predicate is data in the fact) |
| X4 | Join-side **value predicates** on existing facts: equals / in / regex / token-in-delimited-`k=v`-list | cm-1.16 E2, E3; istio E1; cilium E8/E4 (old-set ∧ new-unset) | yes |
| X5 | **Cross-resource references** (`issuerRef` → Issuer/ClusterIssuer kind) | cm-1.16 E1 (refinement) | yes |
| X6 | **Cross-product inventory** (`envinv`): declared inventory file, plus image-ref → product reverse mapping from `products/*.yaml` container-image artifacts | argo E3, cm-1.17 E3, istio E2 | yes |

Ranked by links unlocked: X1 (6) > X4 (4–6) > X6 (3) > X2 (2–3) = X3 (2) > X5 (1).
X1 + X4 + X6 are the high-value trio.

**Validators / artifact capture:**

| # | Validator | Proves | Links |
|---|---|---|---|
| V1 | Confirm a proposed helm-value subject + change against the values diff (exists) | removed / default-changed / added | cilium E6, E3 (+ partial for E8, E4) |
| V2 | Generic `crd:storage-changed` (and `crd:fields-added` → UNKNOWN reason) join rule | exact GVK in use whose version loses storage | karpenter E1 |
| V3 | CRD schema enrichment: capture `default`, `enum`, `required` per path in `CRDVersionInfo` | default-changed (when schema-defaulted), enum renames, new required fields | karpenter E2; cm-1.17 E1 only if the schema states the default |
| V4 | OCI Helm chart values ingestion (charts published only to registries) | values diffs for those charts | karpenter E7 |
| V5 | Operand-version tables as declarative compatibility sources (`yaml-records` over e.g. `kafka-versions.yaml`) | support windows | strimzi E5 |
| V6 | CRD field existence (exists via `SchemaPaths`) | subject identity only, not the change | partial for cm-1.17 E1, karpenter E2, strimzi E6 |

### 3.5 Design inputs for the `contract` lane

1. **Attachment: fact → change(s).** Hits and cells are scored per change id, so a verified fact must
   resolve to *every* change in the edge that restates it. Without that, links may hit while
   restatement cells stay UNKNOWN (cm-1.17 E1 has 3 restatements; karpenter E1 has 6 changes). Key the
   fact on release + subject (+ evidence locator), and attach it through the duplicate/restatement
   grouping rather than a single `chg-…` id. Change ids are content-derived and shift with note text.
   Also avoid attaching to multi-item "umbrella" changes (shadowing, D13).
2. **Predicate operators the 21 links need:**
   - field unset on GVK (cm-1.17 E1)
   - field equals / in (karpenter E2, strimzi E5, strimzi E6)
   - regex on a field value (istio E6)
   - helm-value equals (istio E1, cilium E8)
   - token in a delimited `k=v` list (cm-1.16 E2, E3)
   - line regex in embedded text, with **negation** "no line matches" (argo E1, E2)
   - product present / product version in range (argo E3, cm-1.17 E3, istio E2)
   - edge from-version comparison (cilium-1.15 E1)
   - cross-resource reference (cm-1.16 E1)
   - conjunction (istio E1, cilium E8)
   - an explicit "not statically decidable → UNKNOWN(reason)" for runtime-only conditions
3. **The consequence → class mapping belongs in the fact, and must be checked against the dataset's labels:**

   | Consequence | Class |
   |---|---|
   | removal or `failure` + exposure | ACTION |
   | deprecation / future-removal | REVIEW |
   | default-changed + unset | REVIEW |
   | default-changed + pinned (key still live) | INFORMATIONAL |
   | precondition satisfied | NOT AFFECTED |
   | deprecated + replacedBy, with old-set ∧ new-unset | REVIEW, or ACTION if the old key is ignored |

   cm-1.16 E2 (deprecation labelled ACTION) and the MISSION demo (default change → ACTION) both
   conflict with this mapping. Decide deliberately.
4. **The MISSION demo vs the dataset.** On cert-manager-1.17-1.18 the ground truth is `review`. A
   rotationPolicy ACTION finding there would be scored a false action. Either the demo's step 10
   becomes REVIEW REQUIRED, or the fact's consequence must name a concrete failure (e.g. consumers
   pinning the key), and the groundtruth lane would have to justify relabelling from upstream sources.
5. **A `replacedBy` relationship for helm-values (and config keys / flags).** Without it,
   `impact:values-pinned` and `impact:values-unset` give confidently wrong answers for deprecated-and-
   replaced keys (the 2 current hits and 2 of the 5 dangerous cells).
6. **Restatement links are the cheapest tier-a win.** A model proposes "this declared change's subject
   is helm-value X removed"; V1 confirms it from the values diff. That moves cilium E6/E3 with zero
   human time, and the same shape works anywhere a declared note restates a computed diff.

---

## 4. Dataset observations (for `groundtruth` and the commander — nothing was edited)

| # | Case · item | Observation | Suggested handling |
|---|---|---|---|
| D1 | cilium-1.15 E6, cilium-1.16 E3 | Matchers (`metallb-bgp`, `bgpControlPlane`, `bgp-config-path…`) never select the computed `values:section-removed bgp.*` change. The engine already classifies it ACTION, and the same cases' `expectedFindings` (F3 / F1 `subject: bgp.enabled`) find it. The cited upstream quote names `bgp.enabled` verbatim. | Integrity caution: the motive for a `subject: bgp.enabled` matcher would come from seeing pipeline output. If added, justify it from the upstream quote alone and record that in NOTES.md, or leave it and let the knowledge layer's restatement link (§3.5-6) close it. Commander's call. |
| D2 | cilium-1.15 E1 | Labelled action-required, but the edge starts at v1.15.6, which meets the "≥ 1.15.6 before 1.16" precondition (also pinned in images.txt). There are no manifests, so toFQDNs usage is unknown. The `why` itself says the env is "exactly the minimum". | The honest class is not-affected (satisfied precondition) or informational. Consider relabelling with an upstream-grounded rationale. |
| D3 | strimzi E6 | The `why` claims "the fixture's authorization configuration references OPA". No fixture file has `authorization` or `type: opa`. | Either the fixture lacks the intended block, or the link should be not-affected. Fix one side, from upstream. |
| D4 | argo-cd E3 | "cert-manager runs in the same cluster" exists only in `description`. argocd-cm (where an explicit `resource.exclusions` would override the default) is not supplied. NOTES.md already records this as an expected honest miss. | Under (c): add a fixture inventory entry for cert-manager (and optionally an argocd-cm), recorded in NOTES.md. |
| D5 | istio E2 | istio-csr v0.12.0 exists only in `description` (and a values comment). | Same: inventory entry, or an image ref `…/cert-manager-istio-csr:v0.12.0`. |
| D6 | cert-manager-1.17 E3 | ingress-nginx v1.12.1 exists only in a YAML comment (certificate.yaml L1–2). | Same: inventory entry or controller image ref. Also note the ingress-nginx ConfigMap is absent, so the honest class with inventory is REVIEW, not ACTION. |
| D7 | cert-manager-1.16 E2 | A deprecation (gate still functional in 1.17; removal in 1.18) labelled action-required. The engine's contract treats deprecations as review (`impact:crd-version-deprecated`). | Contract/groundtruth should agree on deprecation semantics. Today it can only cost class accuracy. |
| D8 | cilium E8 / E4 | The cilium-1.16 E4 `why` says the key "no longer exists in the v1.17.0 chart". The cilium-1.15 E8 citation shows `secretsBackend: ~` still present in 1.17 values. Whether `k8s` still takes effect in 1.17 templates is uncited, and it decides ACTION vs REVIEW. | Verify against the 1.17.0 chart templates and cite. |
| D9 | karpenter E7 | Item title and matcher are about env vars (LOGGING_CONFIG, …); the `why` and NOTES are about Helm values (`logConfig`, `assumeRoleARN`). A helm-values finding can only hit through the env-var change. | Consider a separate helm-values item or matcher, justified from the cited values.yaml at both tags. |
| D10 | strimzi E5 | The `why` rests on `metadataVersion 3.8-IV0`. A metadata version is not the broker software version (to our understanding, Kafka 3.9/4.0 brokers accept 3.8-IV0 — please verify upstream). The decisive evidence in the fixture is `KafkaMirrorMaker2.spec.version: 3.8.0` (and the `kafka-3.8.0` image tag), which the `why` doesn't mention. Item class (review) and link relevance (action) differ, deliberately per NOTES. | Re-ground the `why` on MM2 `spec.version`; verify the metadata-version claim upstream. |
| D11 | cert-manager-1.17 E1 | Relevance `review` vs the MISSION demo's ACTION REQUIRED (§3.5-4). | Decide at commander level before anyone builds toward the demo. |
| D12 | several | Loose matchers select incidental changes: cilium-1.15 E1 `(?i)toFQDNs` (dnsProxy default, new metric); karpenter E1 `(?i)v1beta1` (field-added, webhook PR); strimzi E5 `Kafka 4\.0\.0` (JMXReporter); istio E1 `cni\.ambient\.dnsCapture` (the `defaults.*` restructure); `(?i)bgpControlPlane` (new statusReport key); `readSecretsOnlyFromSecretsNamespace` (4 new tls keys). They produce 3 of the 5 ACTION → NOT-AFFECTED cells, and as engine reach grows they can grant **hits for the wrong reason** (e.g. a future informational on the dnsProxy change would "hit" cilium-1.15 E1). | Tighten matchers with upstream-quoted anchors where possible; the evaluator could report which change produced each hit. |
| D13 | evaluator | `expIDForChange` is last-write-wins: a change matched by two items scores only for the later one. Shadowed today: argo `chg-90bd9b5f3685` (E3 → E5), cert-manager-1.16 `chg-123447d3ab93` (E2 → E3), karpenter `chg-318315d1ff6b` (E1, E2 → E10). | Evaluator note (not a case edit). Fact attachment should avoid umbrella changes. A multi-item attribution would be a scoring change, to be pre-registered if wanted. |
| D14 | cert-manager-1.17 fixture | `manifests/certificate.yaml` puts `spec.acme.solvers` on a **Certificate** (solvers belong to Issuers). The valid evidence for E3 is the ClusterIssuer in the same file. | Cosmetic. A schema-validating env loader would warn. |
