# Loop diagnosis 2 — every review item decided, applicability still 0.58

Lane `analysis` (loop-diagnosis-2), 2026-10-02. Branch `p3ll/loop-diagnosis-2`, based on
`p3-learning-loop` @ 0cd9edd (proxy-2 merged). The question: with every review item decided by
the blind proxy (merged view `docs/phase3/learning-loop/proxy-shadow/eval-knowledge/`, 265 active
facts), why do 331 knowledge findings move applicability only from 0.524 to 0.581 (34/75 affected
links)? Where is each of the 41 unhit affected links lost now?

Integrity: no fact was authored, no item decided, and no eval data, gate or result edited. Three
small generic engine fixes were made with tests (§2). Each is metric-checked.

## 0. Bottom line

1. **Review volume is no longer the bottleneck. Applicability authoring is.**
   - Of the 41 unhit links, only 3 still wait on a pending item.
   - 15 died in review: the proxy closed applicability or consequence as need-more-evidence.
   - 8 have a fact on the matched change whose exposure condition does not decide.
   - Across all 331 knowledge findings, **72% of the UNKNOWN exposures come from an
     `undecidable` leaf the proposal itself wrote** (128 of 178). Many of those leaves could have
     been decided by the condition language that exists today. `text-line`, which reads config
     embedded in ConfigMaps (`loki.yaml`, `policy.csv`), is used by only 2 of 265 facts.
2. **Facts land where the dataset does not look.** 236 of the 331 findings (71%) attach to changes
   no expected item measures. Only 54 land on linked items:
   - 27 on links that are hit (including the 8 knowledge-only hits);
   - 4 on the 2 not-affected links they violate;
   - 23 on unhit links, **every one of them UNKNOWN**.
3. **The deterministic engine had three evaluation gaps.** I fixed them here (§2). One is worth
   **+2 links on its own, with no new violations**: CRD-removal/version changes now use the
   manifests' API-group evidence when `--crds` isn't supplied. Proxy-level applicability goes
   **0.581 → 0.600** (36/75); knowledge-free goes 0.524 → 0.543. The other two make conditions over
   CRD documents and boolean feature-gate keys decidable. That is metric-neutral today, but it
   removes two false-from-not-looking paths.
4. **Honest ceiling under the current contract and fixtures: ≈0.93** (§6). I measured it by
   evaluating the dataset's *own* ground-truth exposure conditions against each fixture with the
   engine:
   - 96 of 105 decide as labelled;
   - 4 affected links are honestly UNKNOWN (the fixture lacks the deciding object);
   - 4 are ground-truth authoring errors;
   - 2 are recall misses;
   - 1 is lost to evaluator attribution.

   **0.80 is reachable without new environment inputs, but not with today's proposals.** The
   remaining gap (36 → about 68 affected hits) is almost entirely applicability predicates and
   consequence evidence, not trust, volume or engine reach.

## 1. Numbers (reproducible)

```
GITHUB_TOKEN=$(gh auth token) bin/ri -state <primary>/.ri eval \
  -knowledge docs/phase3/learning-loop/proxy-shadow/eval-knowledge -render -min-verification proxy
```

| level | facts | at 0cd9edd (base) | at HEAD (this branch) |
|---|---|---|---|
| none / deterministic / human | 0 | 0.524 (26/75) | **0.543 (28/75)** |
| consensus | 31 | 0.524 (27/75) | 0.543 (29/75) |
| proxy (all decided) | 265 | **0.581 (34/75)** | **0.600 (36/75)** |

- **Facts:** 234 proxy and 31 consensus, **0 trusted**. Applicability is verified deterministically
  for 12 facts, by consensus for 28, and by proxy for 225.
- **Knowledge findings:** knowledge adds no ACTION (proxy facts are capped at review), so the ACTION
  findings stay 17 with 1 false: the kyverno E9 label dispute from TRUSTFIX.
- **Other gates:** criticalRecall 1.00, importantRecall 0.97, falseActionRate 0.059 (fails),
  everything else passes.
- **Stored results:** the plain `ri eval` (no knowledge, no render) has no regressions against
  `eval/results` at HEAD; crossplane and traefik improve.

The per-link traces come from a temporary harness (`cmd/ri/zz_diag_test.go`, never committed). It
re-runs the evaluator's exact pipeline per case and joins case expectations → matched changes →
findings → candidate → proposals → review items → decisions → facts. It reproduces 34/75 at the base
cell for cell.

## 2. Engine fixes on this branch (generic, each with a regression test)

| commit | fix | effect |
|---|---|---|
| `impact: CRD removal/version changes use manifest group evidence without --crds` | Without the installed CRDs, `crd:removed` and `crd:version-*` returned UNKNOWN **before looking at the manifests**. A manifest of the changed CRD's group (+version, and the kind when the differ's own title names it) is positive evidence. The ladder's existing group-only rung now applies: review-required, medium confidence, "kind unconfirmed". It never yields action and never not-affected; unused subjects stay UNKNOWN. | **traefik E1** (`traefik.containo.us` IngressRoute/Middleware) and **crossplane E6** (`apiextensions.crossplane.io/v1` XRD) become hits. Kind pinning prevents a crossplane `ControllerConfig` false flag (caught in the A/B, then fixed). +2 links, 0 new violations |
| `impact/env: read CRD documents as resources; feature gates as boolean values keys` | (a) CRD documents fed only the installed-CRD inventory. A condition over a CRD's own fields (`status.storedVersions`) found "no resource of the kind" and decided **FALSE from never looking**, so a trusted fact would wrongly clear. (b) A feature-gate leaf with a values path missed `settings.featureGates.drift: false` (path names the gate's boolean key; key spelled in another case). | The ground-truth exposures of ES E3, flux E2 and karpenter-ci E7 now decide as labelled. Metric-neutral today (no fact relies on them) |

`go build ./... && go vet ./... && go test ./...` green.

## 3. The 41 unhit links

### Stage map

| stage | meaning | base (41) | HEAD (39) |
|---|---|---|---|
| 0 | item not in the edge (recall miss) | 2 | 2 |
| a | matched change never became a candidate | 6 | 5 |
| b | candidate, but proposals all abstain (evidence-sufficiency only) | 5 | 4 |
| c | a review item is still pending | 3 | 3 |
| d | decided, but an aspect closed need-more-evidence or rejected, so no fact | 15 | 15 |
| e | **fact on the matched change, exposure does not decide true** | 8 | 8 |
| f | the answer exists, the evaluator doesn't count it | 2 (+2 hidden) | 2 (+2 hidden) |

"Hidden" f: karpenter-ci E7 and strimzi-edge E2 are staged b/e by their prose change. Each also
has a **deterministic ACTION** for exactly its subject on a computed change the item's matchers
don't select:
- karpenter-ci E7: `impact:values-removed` on `settings.featureGates.drift`;
- strimzi-edge E2: `impact:crd-removed` on `kafkamirrormakers`, with the `KafkaMirrorMaker` in use.

### Per-link table

"GT exposure" is the link's own ground-truth exposure condition, evaluated by the engine against
the fixture at HEAD:
- `true` = decidable as labelled;
- `unknown` = the fixture can't decide it;
- `false` = ground-truth predicate error.

Informational links read `exposure/overlap`.

| case | link | rel | stage | what blocks it | GT exposure |
|---|---|---|---|---|---|
| argo-cd-2.14-3.0 | E1 | action | e | fact vf-c87ae1cbf554: `all[not(argocd-cm flag = "false"), undecidable(policies granting update)]`. The policy grants *are* decidable (text-line on `argocd-rbac-cm` `policy.csv`), but `argocd-cm` is not in the fixture | **unknown** (argocd-cm absent): honest |
| argo-cd-2.14-3.0 | E2 | action | e | fact vf-e2a6819cf569 reads **Helm values** `configs.cm[…]`/`configs.rbac[…]`; the fixture supplies the raw ConfigMaps (manifests) and no values: "--values not supplied" | **unknown** (argocd-cm enforce flag absent): honest |
| argo-cd-2.14-3.0 | E3 | review | d | semantic-mapping closed need-more-evidence | **unknown** (argocd-cm absent): honest |
| cert-manager-1.16-1.17 | E2 | review | e | fact vf-1f4d3f06d1e3 `feature-gate ValidateCAA enabled` **without `path`**; the gate is set in values `featureGates` | true (with `path: featureGates`) |
| cert-manager-1.16-1.17 | E3 | info | d | consequence + applicability need-more-evidence | true |
| cert-manager-1.16-1.17--eu-platform | E1 | review | e | vf-b9ac6b366b2f: `all[CA/SelfSigned issuer present, undecidable(signing key size)]`; vf-72a02b5d0b5c: pure undecidable. The fixture's root Certificate is **SelfSigned with its own 4096-bit key** (the GT cites PR 7368: "the algorithm is decided by the signer"), so the leaf is decidable | true |
| cert-manager-1.17-1.18 | E3 | action | c | consequence item pending (high) | **unknown** (ingress-nginx controller ConfigMap absent): honest |
| cilium-1.16-1.17--plant-edge | E9 | review | d | applicability never asked (evidence-sufficiency closed) | true |
| crossplane-1.20-2.0 | E6 | review | a → **hit at HEAD** | computed `crd:version-deprecated`, UNKNOWN for missing `--crds`; fixed in §2 | true |
| crossplane-1.20-2.0 | E7 | info | d | consequence + applicability need-more-evidence | false / overlap — |
| crossplane-1.20-2.0 | E8 | action | a | only `lines:*` capture diffs of `core.go` match; **no candidate is generated for `lines:*` changes** | true |
| external-secrets-0.15-0.16 | E7 | review | a | the only match is a `cc:feat` note; it is not a candidate (routine/feature filter) | true |
| flux-2.6-2.7 | E5 | action | a | only a `lines:added` install.yaml diff matches; no candidate | true |
| istio-1.23-1.24 | E2 | action | d | applicability need-more-evidence (istio-csr inventory exists in the fixture) | true |
| istio-1.23-1.24 | E6 | action | d | consequence never asked (evidence-sufficiency closed) | **false**: GT path `tagOverrides.*.value` uses a map wildcard the language doesn't have |
| karpenter-0.37.8-1.0.0 | E2 | action | d | consequence need-more-evidence (semantic + applicability accepted) | true |
| karpenter-0.37.8-1.0.0--ci-buildfarm | E4 | action | d | all three aspects **rejected** by the proxy | **false**: GT `resource DaemonSet` omits `group: apps` (empty group = core) |
| karpenter-0.37.8-1.0.0--ci-buildfarm | E5 | action | b | 4 proposals, all abstain | true |
| karpenter-0.37.8-1.0.0--ci-buildfarm | E7 | review | b (+f) | env-var prose: all abstain. A deterministic ACTION on `settings.featureGates.drift` exists on an unmatched change | true (after §2b) |
| kyverno-1.12-1.13 | E3 | review | f | fact vf-d5f2c5485961 (review, exposure **true**) sits on umbrella change `chg-d645fe77075d`, which also matches E4. Last-wins attributes it to **E4 (not-affected)**: E3 misses **and** E4 is violated | true |
| kyverno-1.12-1.13 | E8 | review | a | computed `values:added` with a join rule (render: no attributable change), so not a candidate | true |
| kyverno-1.12-1.13 | E10 | info | 0 | no change carries the tested-range shift | false / overlap true |
| loki-2.9-3.0 | E1 | action | e | vf-489e08b977a9: `all[from < 3.0, undecidable(tsdb + v13 configured)]`. The schema periods are in the fixture's ConfigMap `loki.yaml` (v11/v12, no v13): decidable with `text-line` | true |
| loki-2.9-3.0 | E2 | action | e | vf-b6cadb3343c5 / vf-5a95ed0b6689: `cli-flag -*.shared-store set` ∨ undecidable("YAML path not given") ∧ undecidable(compare). Loki is configured by `-config.file`, so the flag channel is empty; `shared_store: s3` is in `loki.yaml` | true |
| loki-2.9-3.0 | E3 | action | d | applicability + consequence need-more-evidence | true |
| loki-2.9-3.0 | E4 | action | e | vf-9cfb18c65ffc, vf-ad2b1f9610ed, vf-9f98808343b3, vf-73af9e8ae138: `cli-flag` leaves (**false** or unknown: no such args) ∨ undecidable("YAML path not named"). `max_look_back_period` / `enforce_metric_name` are in `loki.yaml` | true |
| loki-2.9-3.0 | E7 | info | d | semantic + consequence need-more-evidence | false / overlap true |
| loki-2.9-3.0 | E8 | review | b | single proposal abstains | true |
| prometheus-operator-0.85-0.86 | E1 | action | d | applicability **rejected**, consequence need-more-evidence | true |
| prometheus-operator-0.85-0.86 | E2 | action | f | deterministic-validated informational fact on `chg-bc354e3f6346`, attributed by last-wins to E5. E2's own three changes have no fact | true |
| prometheus-operator-0.85-0.86 | E3 | review | 0 | no change matches (recall miss) | true |
| prometheus-operator-0.85-0.86 | E7 | info | d | semantic + consequence need-more-evidence | false / overlap true |
| prometheus-operator-0.85-0.86 | E8 | info | a | heuristic note not made a candidate | true |
| strimzi-0.45-0.46 | E5 | action | d | applicability need-more-evidence (the operand inventory exists: Kafka 3.8.0) | true |
| strimzi-0.45-0.46--edge-retail | E2 | action | e (+f) | vf-1b41a0983870: pure undecidable ("evidence names neither kind nor image"). `gvk-in-use KafkaMirrorMaker` decides it, and the deterministic `crd-removed` ACTION already exists | true |
| strimzi-0.45-0.46--edge-retail | E4 | action | b | all abstain | true |
| strimzi-0.45-0.46--edge-retail | E6 | review | c | applicability pending (normal) | true |
| strimzi-0.45-0.46--edge-retail | E8 | info | c | consequence pending (high) | true / overlap true |
| traefik-2.11-3.0 | E1 | action | b → **hit at HEAD** | prose candidate had **0 proposals**; the 9 computed `crd:removed` were UNKNOWN for missing `--crds`; fixed in §2 | true |
| traefik-2.11-3.0 | E2 | action | d | applicability need-more-evidence / semantic missing | true |
| traefik-2.11-3.0 | E8 | action | d | semantic need-more-evidence; computed `crd:added` candidate has applicability rejected | true |

## 4. The three things the commander asked to look at

### 4a. Facts on the matched change whose exposure does not decide (8 links)

| link | leaf that fails | why | what decides it |
|---|---|---|---|
| argo E1 | `undecidable(policy grants)` + `not(argocd-cm …)` | authored undecidable, and the fixture lacks `argocd-cm` | the grants: text-line on `argocd-rbac-cm` `policy.csv`; the flag: honestly unknown without argocd-cm |
| argo E2 | `values-key configs.cm[…]` | **wrong channel**: the predicate reads Helm values, the fixture has raw ConfigMaps | field/text-line on `argocd-rbac-cm` (and argocd-cm: honestly unknown) |
| cm-1.16 E2 | `feature-gate ValidateCAA` (no `path`) | **missing path**: the leaf reads container args only | `path: featureGates` (values) |
| eu-platform E1 | `undecidable(signing key size)` | authored undecidable | `resource Certificate` with `spec.privateKey.size ≥ 3072` + `ref issuerRef → selfSigned` |
| loki E1 | `undecidable(tsdb/v13 configured)` | authored undecidable; **config lives in an embedded file** | `text-line` on ConfigMap `loki` `data["loki.yaml"]` (`schema: v13` none) |
| loki E2 | `cli-flag -…shared-store` ∨ undecidable | **wrong channel** (flags vs config file) + authored undecidable | `text-line` `shared_store:` exists in `loki.yaml` |
| loki E4 | `cli-flag -querier.engine.timeout` etc. (**false**) ∨ undecidable | **wrong channel**: the flag leaves evaluate false/unknown because Loki takes `-config.file` only | `text-line` for each removed key in `loki.yaml` |
| strimzi-edge E2 | pure `undecidable` | authored undecidable ("evidence names no kind"); the candidate cluster lacks the computed `crd:removed` member that names it | `gvk-in-use kafka.strimzi.io KafkaMirrorMaker` |

Patterns:
1. Authored `undecidable` leaves where a decidable predicate exists (5 of 8).
2. The wrong configuration channel: CLI flags or Helm values where the product reads a config
   file or ConfigMap (3 of 8).
3. One missing `path`.

Dataset-wide, 98 of 265 facts carry an `undecidable` leaf (56 are pure undecidable).
`cli-flag` leaves account for 50 of the 178 UNKNOWN exposures. The proposals see only the upstream
statement, never where the product reads its configuration, so they guess the channel or abstain.

### 4b. Facts on changes no item measures

Of the 331 knowledge findings:

| landing | unknown | informational | review |
|---|---|---|---|
| change no expected item matches | 201 | 24 | 11 |
| expected item without an env link | 24 | 2 | 7 |
| undecided links | 7 | 1 | 0 |
| linked items | 31 | 5 | 18 |

- **Knowledge-only hits (proxy over none): +8.** cert-manager-1.17 E1, eu-platform E4, loki E9,
  prom-op E5, cilium-1.15 E6, cilium-1.16 E3, crossplane E4, traefik E4.
- **Knowledge-caused not-affected violations: 2.**
  - cilium-1.15 E1: the consensus dnsProxy fact, matched through the loose `(?i)toFQDNs` matcher.
  - kyverno E4: the umbrella change in 3a above.
- 240 of 265 facts produce at least one finding.
- The remainder (71% of all findings) describe real, mostly low-stakes changes that no dataset item
  measures. The loop works on everything the edge emits; the dataset measures about 75 items.

### 4c. Links that cannot be decided from the supplied environment (honest UNKNOWN)

Evaluating the ground-truth exposure itself against the fixture:

| link | missing input | note |
|---|---|---|
| argo-cd E1, E2, E3 | `argocd-cm` (the opt-in/enforce flags, explicit `resource.exclusions`) | the fixture supplies `argocd-rbac-cm` only; `not(...)` over an unsupplied ConfigMap has no evidence |
| cert-manager-1.17 E3 | the ingress-nginx controller ConfigMap (`strict-validate-path-type`) | the inventory supplies ingress-nginx 1.12.1; its config is absent |
| kyverno E1 *(already hit deterministically)* | completeness of ClusterRoles (`not(aggregating ClusterRole)`) | the fixture says "manifests are the complete set" in prose only |

The correct answer for these under absence-is-not-knowledge is UNKNOWN, and their labels claim
affected. Two routes: add the missing objects to the fixtures, or move the links to
`undecidedImpact` (groundtruth decision).

## 5. Where the 331 findings land (classes)

- **Classes:** 263 unknown, 36 review-required, 32 informational, 0 action (proxy cap).
- **Exposure evaluation:** 65 true, 88 false, 178 unknown.
- **Unknown reasons:**
  - environment-visibility-gap 127 (mostly authored or unparseable leaves);
  - release-knowledge-gap 89 (exposure false with an untrusted fact: "never clears");
  - runtime-behavior-gap 31;
  - evidence-gap 15;
  - cross-product 1.
- The 88 false exposures are the **NOT AFFECTED the contract withholds from proxy facts**. With
  trusted facts they would become clears, so their predicates matter for safety. Example: loki
  E4's `cli-flag … set` is false only because the setting lives in a file. As a trusted fact it
  would have cleared an exposed environment.

## 6. Honest ceiling for this dataset under the current contract

Method: a link is reachable when (i) its ground-truth exposure (overlap for informational)
decides as labelled on the fixture with today's condition language and engine (HEAD), (ii) the
item has a matched change, and (iii) that change is not attributed to another item.

| | links |
|---|---|
| affected links | 75 |
| − honestly UNKNOWN (§4c; kyverno E1 is already hit, so not counted) | −4 |
| − recall misses (kyverno E10, prom-op E3) | −2 |
| − evaluator attribution (kyverno E3: all its changes are attributed to E4) | −1 |
| = **reachable affected** | **68** |
| GT predicate errors (istio E6, karpenter-ci E4) | counted as reachable: a correct fact predicate decides them |

Not-affected: 30 labelled. Today 27 are clean. The 3 violations are plant-edge E3 (PO-3 vs label),
cilium-1.15 E1 (matcher) and kyverno E4 (umbrella).

| scenario | applicabilityAccuracy |
|---|---|
| today (HEAD) | 0.600 (36 + 27) |
| ceiling, violations unresolved | (68 + 27) / 105 = **0.905** |
| ceiling, the 3 violations resolved (matcher, umbrella, PO-3 relabel) | (68 + 30) / 105 = **0.933** |
| the honest-UNKNOWN 4 moved to undecidedImpact (denominator 101) | (68 + 30) / 101 = 0.970 |

**0.80 needs 84 correct decisions, about 57 affected hits with 27 NA, i.e. +21 over today.**
The reachable pool holds +32. The gap is not environment inputs; it is the quality of the facts
the loop produces.

## 7. Ranked levers (expected link gains, HEAD baseline 36/75)

| # | lever | links | expected gain | owner |
|---|---|---|---|---|
| L1 | **Applicability authoring with channel context.** Give proposer and proxy reviewer, per product, where configuration is read (Helm values paths, ConfigMap-embedded files, CLI args) and the operators that read each (`values-key`, `text-line`, `field`, `gvk-in-use`). Rule: no `undecidable` leaf when the evidence names a subject a predicate can test. Re-run applicability on the 8 stage-e facts | 8 (e) | **+6** (cm-1.16 E2, eu-platform E1, loki E1/E2/E4, strimzi-edge E2; argo E1/E2 stay honestly unknown) | semantic + knowledge lanes |
| L2 | **Consequence/applicability evidence for the reviewer.** 15 stage-d links closed need-more-evidence on an aspect the upstream guide states. Attach the upgrade-guide section (not just the note line) to the candidate | 15 (d) | **+8–11** (argo E3 stays unknown) | semantic lane (evidence assembly) |
| L3 | **Measurement fixes** (groundtruth/evaluator) | kyverno E3, prom-op E2, karpenter-ci E7, strimzi-edge E2; violations cilium-1.15 E1, kyverno E4 | **+4 hits, +2 clean NA** | groundtruth (matchers, upstream-justified); evaluator multi-attribution (pre-registered) |
| L4 | **Candidate generation for `lines:*` capture diffs and filtered notes** | crossplane E8, flux E5, ES E7, prom-op E8, kyverno E8 | +3–4 | semantic lane |
| L5 | **Evidence-sufficiency abstentions** | 4 (b) | +2–3 | semantic lane |
| L6 | **The 3 still-pending items** | 3 (c) | +2 (cm-1.17 E3 is honestly unknown) | human / proxy |
| L7 | **Fixture completeness or undecided relabel** (argocd-cm, ingress-nginx ConfigMap) | 4 | +3–4, or a denominator change | groundtruth |
| L8 | **Capture recall** (kyverno E10, prom-op E3) | 2 | +2 | capture lane |

L1 + L2 + L3 at their middle estimates (+6, +9, +4 → 55, plus 2 NA) give about (55 + 29)/105 =
**0.80**. All three are needed, and none requires a trust change.

## 8. Findings for other lanes

1. **groundtruth: eu-platform E2/E3** are labelled not-affected, but their exposure conditions
   evaluate TRUE (`ValidateCAA unset`, `NameConstraints/UDQF disabled`). The conditions describe the
   *clearing* state, inverted. The labels look right; the exposures are not.
2. **groundtruth: istio E6** uses a map wildcard (`tagOverrides.*.value`) the language does not
   support; use `tagOverrides.peer_namespace.value`, or the contract adds map wildcards.
   **karpenter-ci E4** `resource DaemonSet` needs `group: apps`.
3. **groundtruth: argo E1/E2/E3, cert-manager-1.17 E3** are honestly UNKNOWN on the fixture as
   supplied (§4c).
4. **groundtruth / contract: kyverno E3/E4.** `chg-d645fe77075d` lists four deprecated settings
   inline ("… deprecated: - a - b - c - d"). `domain.IsUmbrella` counts list items only at line
   starts in `Detail`, which the normalizer flattened, so the fact attaches to an umbrella.
   Counting list lines in the evidence *excerpt* would catch it, but would reclassify 36 flattened
   changes dataset-wide (some are one statement with conditional sub-bullets). That is a contract
   decision; not changed here.
5. **groundtruth: cilium-1.15 E1** `(?i)toFQDNs` selects the dnsProxy change, and a correct
   consensus fact now violates the not-affected label through it (as predicted in UNKNOWN-ANALYSIS
   D12).
6. **semantic lane:** no candidates are made for `lines:*` capture diffs or `cc:feat` /
   heuristic-filtered notes that items depend on (L4). One prose candidate (traefik
   `sc-130acc8e1749`) has **0 proposals**.
7. **contract:** `feature-gate` without `path` reads container args only, and since d258ab4,
   unknown when there are none. A values-convention fallback (`featureGates`, `*.featureGates`)
   would decide cm-1.16 E2 even with the current fact. That is a semantic change, so left to the
   contract owner.
8. **commander:** the run's diff against stored results is unchanged from LOOP-DIAGNOSIS
   (plant-edge E3 PO-3 vs label). `eval/results` was not touched.

## 9. Artifacts

- `cmd/ri/zz_diag_test.go`: temporary dump and ground-truth-exposure harness, never committed.
- Scratch data: `…/scratchpad/ld2/` (per-case impact/edge dumps at base and HEAD, `trace*.json`,
  `rows*.json`, `gt-exposure.json`, eval texts).
