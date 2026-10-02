# Lane `validate` status

**State: done** (branch `p3ll/validate`). `go build ./... && go vet ./... && go test ./...` pass.
The `rendered-diff` validator is skipped as instructed (render lane paused).

## Done
- `internal/semvalidate` (new): `Validators()` returns `helm-values`, `crd-schema`, `compatibility`, `image`,
  `restatement`, `canonical-applicability` (producers `semvalidate.<x>@v1`). Docs: `docs/SEMVALIDATE.md`.
  - **values**: exists / removed / added / default+value before→after / renamed / deprecated (refutes a missing
    replacement; never confirms a deprecation). Section paths count as present while any key below exists.
    Chart selection by `Subject.Name`.
  - **crd** (uses the capture lane's `CRDVersionInfo.Fields`): field exists/removed/added/renamed, default
    before→after, now-required, validation-tightened (enum loss, newly required, type change), enum rename; GVK
    removed/unserved/added/deprecated and storage-version move (V2's fact). A schema that states no default is
    **inconclusive**, never refuted (the default may be applied in code, cm-1.17 E1). Old snapshots without
    `Fields` degrade to inconclusive for attribute claims; path-list claims still work.
  - **compat**: `requirement-changed` compared as version-line sets via `upgrade.EvaluatePlatformConstraint`
    (the single semantic representation of a constraint). Works for the Kubernetes matrix, min/max columns and the
    Strimzi Kafka operand set (V5).
  - **image**: referenced/removed/added/tag change with registry-spelling normalisation.
  - **restatement**: a note-derived proposal's subject+change confirmed by a computed diff of the same edge
    (`values:*`, `crd:*` incl. the capture lane's new rules, `images:*`), citing the diff's two-sided evidence. Parses
    the fixed title/detail shapes of `internal/upgrade`; the tests build the edge with the real `upgrade.Build`, so
    drift in those shapes fails a test.
  - **canonical**: applicability confirmed when the asserted condition equals the DESIGN §1.3 table entry *and*
    an artifact validator confirmed subject+change; else inconclusive. Confirms the `none` consequence of `added`
    subjects. Never confirms any other consequence.
- Tests (table-driven, adversarial): same path different kind (`Issuer` vs `Certificate`), wrong before/after
  default, a renamed key that exists under the new name only, a plausible but non-existent path (refuted, not
  inconclusive), removed-but-still-present, replacement already existing, version-scoped removal (v1beta1 vs v1),
  unknown group (inconclusive), wrong range bounds, an unchanged range, note-derived changes never restating,
  determinism, and that confirmations cite both releases' snapshots.
- Real-data smoke (warm cache, not committed): Karpenter 0.37.8→1.0.0 `logConfig` removal confirmed from the OCI
  chart values; `NodePool v1beta1→v1` storage move confirmed; Strimzi 0.45→0.46 Kafka set `3.9 || 4.0` confirmed
  (and `3.8 || 3.9 || 4.0` refuted).

## Contract changes
- `Validators()` returns six validators, not five (`restatement` added, DESIGN §9 / UNKNOWN-ANALYSIS §3.5-6).
  Marked `CONTRACT-CHANGE(validate)` in `semvalidate.go`. No change to `domain` or `knowledge`.

## Decisions (and why)
- Refute only where the artifact is complete for the claim; otherwise inconclusive (unknown group, missing
  compat row, image not run by the manifests). Prose-only boundaries and non-run images exist.
- Canonical applicability requires an exact match (including the overlap): a narrower condition is not provable
  here, so it is inconclusive and left to a reviewer rather than refuted.
- `canonical` composes the four artifact validators in-process instead of reading other results, because a
  `Validator` sees one assertion and no sibling results.

## Notes for others
- `routing` (knowledge lane): a result can hold refuted/inconclusive checks next to confirmed ones; ids are
  `val-<candidate,validator,assertion>`, so the six validators give up to six results per proposal.
- `applicability` lane: the canonical table lives in `semvalidate/canonical.go` (`canonicalFor`); `cli-flag`,
  `env-var` and `feature-gate` have table rows or none but no artifact validator yet (the render lane's
  rendered-diff would prove them), so those canonical conditions stay inconclusive until it lands.
- No files outside `internal/semvalidate`, `docs/SEMVALIDATE.md` and this file were touched.
