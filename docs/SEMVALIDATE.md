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
