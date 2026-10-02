# Deterministic validators (`internal/semvalidate`)

Part of the learning loop (docs/phase3/learning-loop/DESIGN.md §2.3, MISSION G5). A validator takes one
`SemanticAssertion` (a model proposal, a reviewer's correction, or a canonical construction) plus the ingested
From/To releases and the edge, and proves or refutes the **aspects** it can from artifacts alone. No fetches,
no models, no environment data.

```go
vs := semvalidate.Validators() // []knowledge.Validator
results, err := v.Validate(ctx, knowledge.ValidationInput{Candidate, ProposalID, Assertion, From, To, Edge, Now})
```

A validator returns no result when the assertion's family is not its own. Every `ValidationResult` passes
`ValidationResult.Validate`: a confirmation cites the artifact evidence of **both** releases (the snapshot or row
records it compared), results are content-addressed (`val-…`), and nothing environment-scoped is ever cited.

| Outcome | Meaning |
|---|---|
| `confirmed` | the artifacts prove the aspect |
| `refuted` | the artifacts contradict it: a path in neither release, a default that is not the asserted one, a key still present after being "removed", an unchanged requirement |
| `inconclusive` | the artifacts carry too little: no snapshot, a schema stating no default (it may be applied in code), a deprecation, a behaviour |

Absence of evidence refutes only where the artifact is complete for the claim: a CRD path is refuted when the
CRD of that group+kind is in the snapshot and has no such path in any version, or when the group ships no such
kind; an unknown group, a missing compatibility row and an image absent from the manifests are **inconclusive**
(prose-only boundaries and images not run by the manifests exist). A **consequence is never confirmed**, except
the canonical `none` of an `added` subject.

## Validators

| Producer | Families | Inputs | Proves |
|---|---|---|---|
| `semvalidate.values@v1` | `helm-value` | values snapshots (source tree, packaged and OCI charts) | exists; removed (a section counts as present while any key below it is); added; default / value before→after; renamed (old gone, replacement newly present); deprecated is refuted when the replacement is missing, never confirmed |
| `semvalidate.crd@v1` | `crd-field`, `gvk` | CRD snapshots incl. per-path `default/enum/required/type` | field exists / removed / added / renamed; default before→after (only when the schema states it on both sides); now-required; validation-tightened (enum value lost, newly required, type changed); enum value renamed; API version removed, unserved, added, deprecated; storage version moved |
| `semvalidate.compat@v1` | `compatibility-boundary`, `product-relationship` | compatibility rows (Kubernetes matrix, operand tables, kubeVersion) | `requirement-changed`: the target rows admit exactly the asserted range (compared line by line through `upgrade.EvaluatePlatformConstraint`, so `≥1.30` equals `>=1.30.0-0` and `1.29–1.33` is not `≥1.29`), and the source rows differ |
| `semvalidate.image@v1` | `image` | image-refs snapshots | referenced; removed; added; tag before→after (registry spellings normalised) |
| `semvalidate.restatement@v1` | `helm-value`, `crd-field`, `gvk`, `image` | the edge's **computed** changes | the same subject+change is stated by a computed diff (`values:*`, `crd:*`, `images:*`); cites the diff's two-sided evidence. Refutes only on an exclusive alternative (added vs removed vs default-changed, other before/after). Note-derived changes never restate. |
| `semvalidate.canonical@v1` | the canonical table (DESIGN §1.3) | the artifact validators above | **applicability**, when the asserted condition equals the canonical one *and* an artifact validator confirmed subject+change; otherwise inconclusive (never refuted: a narrower exposure can be right). Also the consequence `none` of an `added` subject. Rule ids `canonical:<family>/<change>`. |

`rendered-diff` belongs to the render lane (same port). `restatement` is a sixth validator beyond the five DESIGN §9
lists (UNKNOWN-ANALYSIS §3.5-6).

## What this buys

Most facts then need a human only for the consequence: subject+change are proven by an artifact or the
edge's own diff, and the exposure condition follows from the canonical table. A model proposal that names a
plausible but non-existent path, the wrong default, or an old key that was not renamed is refuted instead of
reaching a reviewer as "inconclusive".

## `ri knowledge validate`

```text
ri [-offline] [-state DIR] knowledge validate -dir knowledge -edge product:from:to[,…] [-kubernetes 1.31] [-o text|json]
```

Runs `app.Validators()` (the six above plus the render lane's `rendered-diff` over release-level renders) for every
proposal of the store against the ingested From/To releases and the edge. A stored candidate records only its target
release, so the edges it came from are named with `-edge`; a candidate that none of them produces is skipped, never
validated against a guess (the first listed edge wins when two produce the same candidate id). Results are written
through the file store and are idempotent: ids are content-derived and `CheckedAt` is the candidate's own creation
time, so a second run writes nothing. A result is a function of (candidate, validator, assertion), so proposals that
assert the same thing share one result. A validator that errors is recorded as an explicit inconclusive result with
rule `<validator>:unavailable`; a validator with no opinion on a family stores nothing and is counted as not
applicable. The report tallies confirmed / refuted / inconclusive per validator and aspect.

## Limits found by the audit (docs/phase3/learning-loop/VALIDATOR-AUDIT.md)

- **Re-rooted values.** A chart whose keys are rooted differently at the two releases (Istio 1.23 snapshots everything
  under `defaults.`) is compared with the wrapper removed when the key sets then largely coincide, and otherwise
  never confirmed or refuted. The restatement validator cannot restate a change from such a chart's computed diff.
- **Several charts.** A key present in several charts is confirmed only when every chart agrees, unless the subject
  names its chart (`Subject.Name`); an unreadable chart makes an unnamed change ambiguous.
- **Optional keys.** A key absent from both releases next to its parent section is inconclusive, not refuted: defaults
  files omit optional (commented-out) keys.
- **CRD snapshots are not complete kind lists** (runtime-generated CRDs): a missing kind is inconclusive; a missing
  path of a *present* kind is a refutation. Moves between API versions are inconclusive. Unversioned claims must hold
  for every version; attribute claims use the target storage version. A new required field withholds the canonical
  `none`. Object/array defaults sent as JSON strings are understood; `null` after a schema default is proven by its
  removal.
