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
evaluator's duplicate grouping over the fixture's changes.
