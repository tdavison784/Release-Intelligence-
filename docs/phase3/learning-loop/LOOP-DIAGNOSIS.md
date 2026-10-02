# Loop diagnosis — why 105 facts move applicability by one link

Lane `analysis` (loop-diagnosis re-tasking), 2026-10-02. Branch `p3ll/loop-diagnosis`
(base `p3-learning-loop` @ 9d5126a). Question (commander): `ri eval -knowledge
knowledge -render` gives applicability 0.514 (none/human) → 0.524 (proxy, 105 facts,
170 knowledge findings, only +2 links) — for every one of the 48 unhit affected links,
where in the loop is it lost, and why do 170 knowledge findings buy only 2 links?

## 0. Method and integrity

All numbers below come from this worktree at 9d5126a (+ the one engine fix of §5),
offline against the primary checkout's warm cache
(`-state /Users/tommydavison/repos/Release-Intelligence-/.ri`), run as

```
GITHUB_TOKEN=$(gh auth token) bin/ri -state … eval -knowledge knowledge -render -min-verification proxy -o json
```

Note `-min-verification`: it defaults to **human**; without it the run's own aggregate
reports the human row (0.514), which is how "0.524 vs 0.514" confusion arises. The
ladder inside one run (`levels`) is authoritative and needs no second invocation.

A temporary harness (`cmd/ri/zz_diag_test.go`, deliberately uncommitted) re-ran the
evaluator's exact pipeline per environment case and dumped every proxy-level impact
report and edge; the per-link traces below join case expectations → matched changes →
findings → knowledge store (candidate → proposals → review items → decisions → facts).
The harness reproduces the evaluator's link results cell for cell (27/75 at proxy).
No eval data, gates, stored results, expectations or knowledge facts were authored or
edited by this lane.

## 1. Result at a glance

Per-verification-level ladder (same run, warm cache):

| level | facts used | affected links hit | applicabilityAccuracy | not-affected violations | knowledge findings |
|---|---|---|---|---|---|
| none | 0 | 25/75 | 0.514 | 1 | 0 |
| deterministic | 0 | 25/75 | 0.514 | 1 | 0 |
| human | 0 | 25/75 | 0.514 | 1 | 0 |
| consensus | 9 | 26/75 | 0.514 | 2 | 17 |
| proxy | 105 | 27/75 | **0.524** | 2 | 170 |

Aggregate at proxy: applicabilityAccuracy 0.524 = 55/105 (affected 27/75, not-affected
cleared 28/30) **FAIL** (gate ≥ 0.80); falseActionRate 0.0588 = 1/17 **FAIL** (< 0.05);
criticalRecall 0.98, importantRecall 0.95, actionFindingEvidence 1.00, unsupported 0,
pipelineFailures 0 — PASS. classificationAccuracy 0.438, unknownRate 0.78.

Facts by level (weakest aspect governs, `VerifiedFact.Level()`): 96 proxy, 9 consensus,
**0 human, 0 deterministic** — the entire store is untrusted today.

Knowledge's whole contribution at proxy: **+2 affected hits and +1 not-affected
violation, net +1 correct decision** (55 vs 54):

- `cert-manager-1.17-1.18` E1 (review) — 3 consensus facts, `impact:knowledge-exposed`,
  the only consensus-level gain.
- `cert-manager-1.16-1.17--eu-platform` E4 (informational) — 1 proxy fact.
- the +1 violation: `cilium-1.16-1.17--plant-edge` E3 (labelled not-affected) now has
  `impact:values-default-applies` (review-required) on `bgpControlPlane.statusReport.enabled`
  — PO-3 working as decided (a changed default, unset by the customer, proven to reach
  them by render) against a label authored before PO-3. A contract-vs-label conflict for
  `groundtruth`/commander (§7), not an engine bug.

The 1 wrong ACTION is unchanged from TRUSTFIX: kyverno-1.12-1.13 `impact:values-removed`
ACTION on the customer-tuned `cleanupJobs.*` keys joins E9 (labelled review). Kyverno has
no knowledge store at all, so the PO-4 `superseded-upstream` fix cannot apply yet — and
PO-4 refinement requires a *trusted* fact, so a proxy answer alone will not fix it.

## 2. The loop stage map

Stages (per the tasking), applied to each of the 48 unhit affected links; a link is
classified by the furthest stage any of its matched changes reached:

| stage | meaning | links |
|---|---|---|
| 0 | no change matches the item: the edge never proposes it (recall miss) | 6 |
| a | product absent from the knowledge store: no candidates, no proposals, no review | 20 |
| b | proposals exist but every review item is evidence-sufficiency (nothing to decide) | 4 |
| c | review items pending (high priority reserved for the human) | 10 |
| d | decided, but closed without accept → no fact (missing aspects) | 6 |
| e | fact exists but emits no affected finding | 2 |
| f | affected finding exists but the evaluator doesn't count it (matcher/attribution) | **0** |

Headline: **the loop is not leaking at the bottom.** Nothing reaches stage f — the join
is not the bottleneck. 26 of 48 links (stages 0+a) never enter the loop at all, and 20
of those because five products (loki, traefik, prometheus-operator, crossplane, kyverno)
have simply never been run through capture→semantic→knowledge. Of the 22 links inside
the loop, 10 wait on pending review items (8 high-priority = human-only, 2 normal),
6 died in review (decided needs-evidence / missing aspects), 4 never generated a
decidable question (evidence-sufficiency only), and 2 have facts that cannot land (§4).

## 3. Per-link table (all 48 unhit affected links)

The 27 hit links are listed by `ri eval` itself and not repeated. "pend hi: sem+appl+cons"
means one pending review item per named aspect, all routed high priority.

### Stage 0 — not in the edge (6)

| case | link | expected | note |
|---|---|---|---|
| crossplane-1.20-2.0 | E8 | action | fixture passes `--enable-composition-webhook-schema-validation`, but no proposed change matches the item (startup-error flag removal not captured) |
| external-secrets-0.15-0.16 | E7 | review | no change matches |
| flux-2.6-2.7 | E5 | action | no change matches |
| kyverno-1.12-1.13 | E10 | informational | k8s support-window item; no change carries the window shift |
| prometheus-operator-0.85-0.86 | E3 | review | no change matches |
| prometheus-operator-0.85-0.86 | E8 | informational | no change matches |

These are capture/ingest gaps (new change kinds or sources), exactly the UNKNOWN-ANALYSIS
"no candidate" family. Nothing the knowledge loop can do will hit them.

### Stage a — product has no knowledge store (20)

| case | link | expected |
|---|---|---|
| crossplane-1.20-2.0 | E4, E6, E7 | action, review, informational |
| kyverno-1.12-1.13 | E3, E8 | review, review |
| loki-2.9-3.0 | E1, E2, E3, E4, E7, E8, E9 | action ×4, informational, review ×2 |
| prometheus-operator-0.85-0.86 | E1, E2, E5, E7 | action ×2, review, informational |
| traefik-2.11-3.0 | E1, E2, E4, E8 | action ×4 |

Six of the dataset's 13 products have a knowledge store (argo-cd, cert-manager, cilium,
istio, karpenter, strimzi: 519 candidates, 1260 proposals, 1179 review items, 105 facts);
seven have none. Every stage-a link is in the never-run five above (external-secrets and
flux unhit links are all stage 0 or already hit).

### Stage b — proposals, but only evidence-sufficiency items (4)

| case | link | expected | candidate / note |
|---|---|---|---|
| karpenter-0.37.8-1.0.0 | E7 | action | sc-0de9594c13bd on chg-b931c62c0a10: 4 proposals, all undetermined; only evidence-sufficiency items (needs-evidence) |
| karpenter-0.37.8-1.0.0--ci-buildfarm | E5 | action | sc-11f7f1b5c61e on chg-318315d1ff6b: same shape |
| karpenter-0.37.8-1.0.0--ci-buildfarm | E7 | review | same candidate as karpenter E7 |
| strimzi-0.45-0.46--edge-retail | E4 | action | sc-b828fd2a7e6c on chg-fe58ab812a01: 3 proposals, all undetermined |

The loop looked, formed no opinion, and asked for more evidence — the proposals cite
nothing the reviewers can verify against. Fix is upstream (better evidence in proposals:
release-note URLs, migration-guide anchors), not review throughput.

### Stage c — review items pending (10)

| case | link | expected | pending items |
|---|---|---|---|
| argo-cd-2.14-3.0 | E1 | action | sc-cd46ebca5f3e: sem+appl+cons, all **high** |
| argo-cd-2.14-3.0 | E2 | action | sc-9d5516da8bb9: sem+appl+cons, all **high** |
| cert-manager-1.17-1.18 | E3 | action | sc-87d161c59ee8: sem+appl+cons, all **high** |
| istio-1.23-1.24 | E2 | action | sc-87e07dd001ae: sem+appl+cons, all **high** |
| karpenter-0.37.8-1.0.0 | E2 | action | sc-f32097415709: sem+appl+cons, all **high** |
| karpenter-0.37.8-1.0.0--ci-buildfarm | E4 | action | sc-ee5a3d56460b: sem+appl+cons, all **high** |
| strimzi-0.45-0.46 | E5 | action | sc-5851fd4cdacf: consequence **normal** (subject+change verified, applicability closed needs-evidence); plus sc-26c1a428f564 all high |
| strimzi-0.45-0.46--edge-retail | E2 | action | sc-1fdd85cb942b ×2 changes: sem+appl+cons, all **high** |
| strimzi-0.45-0.46--edge-retail | E6 | review | sc-121156407f0b: applicability **normal** (subject+change+consequence verified) |
| strimzi-0.45-0.46--edge-retail | E8 | informational | sc-b5bd8ce23049: sem+appl+cons, all **high** |

Queue-wide: 220 pending items = 208 high (77 consequence, 67 applicability, 64
semantic-mapping) + 12 normal. The high items are reserved for the human (FLEET
proxy-lane contract). Two links (strimzi E5, edge-retail E6) are blocked only on
**normal**-priority items a proxy answerer may take.

### Stage d — decided, but no fact (6)

| case | link | expected | what died |
|---|---|---|---|
| argo-cd-2.14-3.0 | E3 | review | sc-17607957b9fd: semantic-mapping closed needs-evidence; subject+change never verified |
| cert-manager-1.16-1.17 | E3 | informational | sc-b5fadc8960c1: consequence+applicability closed needs-evidence |
| cilium-1.15-1.17 | E6 | action | sc-0619f57e2843: decided without accept; applicability+consequence missing |
| cilium-1.16-1.17 | E3 | action | same candidate sc-0619f57e2843 |
| cilium-1.16-1.17--plant-edge | E9 | review | sc-f233736707d2: applicability missing |
| istio-1.23-1.24 | E6 | action | sc-621425375be9: consequence missing |

All six are "the reviewers asked for evidence the proposals did not carry" — the same
root cause as stage b, one round later. Decisions so far fleet-wide: 400 accept, 121
correct, 323 need-more-evidence, 54 defer, 42 reject.

### Stage e — fact exists, no affected finding (2)

| case | link | expected | fact | why it stalls |
|---|---|---|---|---|
| cert-manager-1.16-1.17 | E2 | review | vf-1f4d3f06d1e3 (ValidateCAA deprecation, proxy) | exposure `feature-gate ValidateCAA enabled` evaluates against container args only; the fixture sets it in **Helm values** (`featureGates: "ValidateCAA=true"`), the condition carries no `path`, so values are never read → the leaf until §5 said *false from nothing* → untrusted+false = `knowledge-undecided` · release-knowledge-gap. After the §5 fix it is honestly unknown · environment-visibility-gap. Landing it needs values visibility (§6, L6) — then the proxy cap already permits review-required, which is what E2 expects. |
| cert-manager-1.16-1.17--eu-platform | E1 | review | vf-b9ac6b366b2f, vf-72a02b5d0b5c (RSA/SHA oracles, proxy) | unknown · environment-visibility-gap: deciding needs the signing key sizes and consumer hash support — present in no supplied artifact. Honestly unreachable without a new environment input. |

### Stage f — evaluator loses an emitted affected finding (0)

None. (The historical D1 cases — cilium bgp ACTIONs invisible to matchers — are now
matched in this dataset: those links sit at stage d instead.)

## 4. Where the 170 knowledge findings land

Of 170 knowledge findings across the 12 knowledge-covered cases:

- **136 (80%) are UNKNOWN** (`impact:knowledge-undecided`): 61 release-knowledge-gap
  (exposure false + untrusted — "we cannot clear without trust", honest), 50
  environment-visibility-gap (a leaf could not decide), 21 runtime-behavior-gap,
  4 evidence-gap. These are the loop *refusing*, mostly correctly.
- **34 are affected-class** (22 review-required, 12 informational; 31
  `knowledge-exposed`, 3 `knowledge-overlap`), spread over 8 cases — cilium 1.15×13,
  cert-manager 1.18×9, argo-cd×3, etc.
- Of those 34, **32 join changes no expected-item matcher selects** (dataset-FP
  changes or already-hit links) and only **2 join a previously-unhit link** (§1).
  This is the direct answer to "why only +2": the affected knowledge findings are
  real but land on subjects the dataset's items don't measure, while the items that
  *are* measured are blocked at stages 0–d above.

A second structural reason sits on top: exposure conditions dominate outcomes. Of the
105 facts, 34 have informational (never-affected) consequence classes — they can only
produce informational findings — and the 22 review-required findings cluster on a
handful of cilium/cert-manager changes. The fact population and the item population
are nearly disjoint at the affected end.

## 5. Engine bug found and fixed on this branch (commit d258ab4)

`impact: absence from an unexamined environment is unknown, not false` —
`internal/impact/condition.go` (+ test in `condition_test.go`).

An **unscoped** cli-flag, env-var or feature-gate condition decided **false from
nothing** when the environment supplied no workload container arguments at all (and,
for feature gates, no values key at the condition's path): `featureGate` fell through
to `decideFalse(examined)` with zero examined evidence — unlike its own `unset` branch —
and cli-flag/env-var reached the same unevidenced false via `valueLeaf`. Two
consequences:

- today it mis-files proxy facts as release-knowledge-gap instead of
  environment-visibility-gap (the E2 case above), and
- **once facts gain human verification it would silently clear opted-in changes**:
  `ClassifyKnowledge` on exposure false + trusted = NOT AFFECTED. A human-verified
  "X deprecated, exposed when feature-gate Y enabled" fact would have "cleared" the
  very customers who set the gate in their values.

Fix: with nothing examined and no component named, the three leaves now return
unknown · environment-visibility-gap (the unset branches' own message). A named
component absent from fully-parsed manifests still decides false (existing tested
semantics, same as an absent resource kind), and absence **with** examined evidence is
unchanged.

A/B at `-min-verification proxy` (before 9d5126a vs after d258ab4, same cache):
**every gate, every aggregate, every link identical** (27/75, 0.524, naViol 2,
falseActionRate 0.0588, unknownFindings 2017) — the change only corrects the
`unknownReason`/`neededToDetermine` of the affected undecided findings (e.g. the
ValidateCAA finding now says "no workload container arguments (or values key) to read
the feature gate from" instead of blaming proxy trust). `go build`, `go vet`,
`go test ./...` green. The run also still reports the pre-existing diff vs stored
results (plant-edge naViol 0→1 from knowledge/PO-3, §1) — not caused by this fix;
flagged, not re-baselined (that is the commander's `-update` call).

## 6. Ranked levers (with expected link gains)

Assumptions: hits count any affected-class finding joined to the item (a proxy fact
capped at review-required still hits an action-labelled link for applicabilityAccuracy,
recording the class mismatch in classificationAccuracy); na side can gain at most +2
(28→30 correct); eu-platform E1 is honestly unreachable; stage 0 needs capture work,
not loop work.

| # | lever | links addressed | realistic gain | owner |
|---|---|---|---|---|
| L1 | Run the loop (capture→semantic→knowledge) for the five never-run products | 20 (stage a) | +8–14 first pass (loki alone carries 7; realistic first-pass yield ~50% given b/c/d attrition elsewhere) | commander scheduling; semantic/knowledge lanes |
| L2 | Human decides the 208 high-priority pending items | 8 (stage c) | +6–8 (facts at any level hit; some will die in validation) | human (reserved) |
| L3 | Answer the 12 normal-priority pending items (proxy run-2) | 2 (stage c: strimzi E5, edge E6) | +1–2 | proxy/knowledge lanes |
| L4 | Richer proposal evidence (stages b+d share the root cause: proposals cite nothing decidable) | 10 (b+d) | +4–7, slower (upstream evidence in prompts/sources) | semantic lane |
| L5 | Capture the 6 stage-0 change kinds (startup-flag removals, k8s windows, …) | 6 | +3–5, bounded by UNKNOWN-ANALYSIS ceilings | capture lane |
| L6 | Feature-gate visibility in Helm values: knowledge lane sets `path` on feature-gate facts (e.g. `featureGates`), or contract decides a values-convention fallback in the leaf | 1 (E2), plus prevents future silent clears | +1 (E2), and unblocks every future feature-gate fact in values-first environments | knowledge lane (cheap) or contract (semantic change — not done unilaterally here) |
| L7 | Trust upgrades (deterministic validators / human verification of existing proxy facts) | 0 links directly | unlocks NOT-AFFECTED clears and PO-4 refinements (kyverno E9 fix lives here: needs a **trusted** superseded-upstream fact) | knowledge/validate lanes + human |

Ceiling arithmetic for the 0.80 gate (84/105): with all 30 na-correct (itself optimistic
— one is the PO-3 conflict), affected hits must reach 54/75, i.e. +27 of the 48. L2+L3+L4
+L5+L6 at their optimistic ends give +15–22; **without L1 (the five missing products)
0.80 is not reachable** — the remaining pool tops out around 0.76–0.78. With L1 at
~50–70% first-pass yield the gate becomes reachable. This matches UNKNOWN-ANALYSIS's
independent ceiling finding from the other direction.

## 7. Findings for other lanes / the commander

1. **groundtruth/commander — plant-edge E3 vs PO-3** (the +1 naViol and the run's one
   flagged diff vs stored results): `impact:values-default-applies` review-required on
   `bgpControlPlane.statusReport.enabled` follows PO-3 exactly (unset default, render-
   attributed); the not-affected label predates PO-3. Either relabel under PO-3 or the
   PO decides values-default findings don't violate not-affected labels. Do not silence
   it in the evaluator.
2. **commander — `-min-verification` default**: the run's own aggregate reports the
   human level unless `-min-verification proxy` is passed with `-knowledge`. Two runs
   quoting "0.514 vs 0.524" may be the same pipeline at different report levels.
3. **knowledge lane — feature-gate facts need `path`**: any feature-gate exposure fact
   without `path` is invisible to values-only or argless environments (E2). Cheap fix
   at authoring time; the leaf-side convention fallback is a contract decision (L6).
4. **proxy/knowledge lanes — 2 links are answerable at normal priority today**
   (strimzi E5 consequence, edge-retail E6 applicability); everything else pending is
   high and human-reserved.
5. **semantic lane — evidence-sufficiency is the loop's middle congestion**: stages b+d
   (10 links) all reduce to proposals whose citations reviewers cannot verify against.
   The decision logs (323 need-more-evidence) name the missing evidence per item.
6. **capture lane — the 6 stage-0 items** are enumerated in §3 with their change kinds.
7. **kyverno E9 false action** persists (1/17) and cannot be fixed at proxy level (PO-4
   refinement needs a trusted fact; kyverno has no store at all — it needs L1 first,
   then a human-verified superseded-upstream fact).

## 8. Artifacts

- `cmd/ri/zz_diag_test.go` — temporary dump harness, intentionally uncommitted
  (`DIAG_OUT=<dir> DIAG_STATE=<state> go test ./cmd/ri -run TestZZDiagDump`).
- Scratch data (per-case impact/edge dumps, tracer scripts, before/after eval JSON):
  `/private/tmp/claude-501/…/bb74c4e8…/scratchpad/ld/` (predecessor) and
  `/tmp/ld-after-dump/`, `/tmp/ld-after-proxy.json` (this handoff).
