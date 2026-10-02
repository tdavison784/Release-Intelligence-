# Adversarial environment pack (G12)

Environments designed to fool a simplistic join. Each directory holds a
synthetic UpgradeEdge (declared in `adversarial.yaml`, built by
`internal/eval/adversarial_test.go`) plus the environment fixture files a
customer would supply; the assertions state what the join MUST and MUST NOT
conclude, derived from the contract in `docs/ACTION_CLASSIFICATION.md` and
`docs/IMPACT.md` — never from pipeline output.

These run as offline unit tests over the join (`go test ./internal/eval`),
not as dataset cases: they measure the join's resistance to specific
confusions, not end-to-end recall. The pack:

| directory | attack | contract |
|---|---|---|
| `values-sibling-similar-name` | removed Helm key with a similarly-named sibling set by the customer | keys compare segment-wise; a sibling must never look removed |
| `crd-same-group-different-kind` | upstream removes one CRD; the customer uses another kind of the same API group | same group is review-required at most, never action |
| `values-old-default-pinned` | customer explicitly pins the old default of a changed key | a pin keeps winning → informational, not action |
| `values-removed-source-not-supplied` | the deciding config source (values) is not supplied | UNKNOWN with `neededToDetermine`, never not-affected |
| `compat-boundary-min` | cluster version exactly at the supported minimum | boundary values admit (in-range informational) |
| `compat-boundary-max` | cluster version exactly at the supported maximum | boundary values admit |
| `image-unrelated-tag` | customer references a different repository than the changed image | not-referenced → not-affected, not image-changed |
| `crd-field-unrelated-identical-path` | unrelated resource of another API group carries an identical `spec.*` path | fields are scoped to the change's API identity, never bare-path matched |
| `dedup-three-sources` | the same fact stated by three sources | the evaluator's duplicate detection must group them |
| `runtime-dependent-unknown` | a behaviour change static config cannot prove | UNKNOWN (`impact:not-joined`), never action, never not-affected |

The `knowledge-*` fixtures attack the verified-knowledge join
(`impact.Build` with facts, DESIGN.md §4): each declares hand-built facts
(the `knowledge:` section; never derived from eval expectations) and asserts
what the trust ladder and the condition language allow the join to conclude:

| directory | attack | contract |
|---|---|---|
| `knowledge-proxy-action` | a proxy-verified fact with true exposure and an action-eligible consequence | a proxy is capped at review-required (`impact:knowledge-exposed`), never ACTION REQUIRED |
| `knowledge-proxy-clear` | the same proxy fact with false exposure, trying to clear | untrusted knowledge never clears: UNKNOWN (`impact:knowledge-undecided`), never not-affected |
| `knowledge-wrong-kind` | a fact about `Certificate` whose condition a same-group `CertificateRequest` with an identical field path could satisfy | conditions are kind-scoped; with healthy manifests and no Certificates the fact clears with checks, never exposed |
| `knowledge-withheld-secret` | the deciding condition reads a withheld ConfigMap value | a withheld value decides nothing: UNKNOWN, never exposed, never clear |
| `knowledge-partial-manifests` | the condition needs manifests supplied only partially | partial visibility never clears: UNKNOWN |
| `knowledge-chart-version-inventory` | a cross-product version condition "decided" by a chart version (Argo CD Application, chart 4.12.1) | a chart version is never the app version: UNKNOWN (`cross-product-context-gap`) |
| `knowledge-missing-product-inventory` | the product missing from an inventory that does not declare itself complete | missing from a non-complete inventory is never "not installed": UNKNOWN, never clear |
| `knowledge-deprecation-true` | exposure true, consequence a deprecation | the class follows the consequence kind (review-required), never action |

Two further attacks are covered in unit tests only (`adversarial_test.go`):

- **upstream doc unavailable** — a pipeline that cannot run is an execution
  failure (`pipelineFailures`), never a reasoning miss, and never silently
  "found nothing";
- **boundary exclusivity** — covered with the min/max boundary cases above
  (one step below/above the boundary is a different, non-adversarial
  behaviour already covered by `internal/impact`'s range tests).

Fixture vocabulary: `edge.changes[].rule` is the upstream diff rule
(`values:removed`, `crd:version-removed`, …, or `note` for declared
note-derived changes); `expect.present`/`expect.absent` are finding matchers
(`rule`, `classification`, `subject` — all set fields must match, exactly
like the dataset's finding matchers); `expect.duplicateGroups` asserts the
evaluator's duplicate grouping over the fixture's changes. A `knowledge:`
section adds hand-built verified facts (`level`, `subject`, `change`,
`exposure`/`overlap` in the §1.3 condition language, `consequence`,
`anchors` naming the fixture's own change titles, optional
`minVerification`); the harness builds valid `VerifiedFact`s from it, so a
fixture fact that would not validate fails the test.
