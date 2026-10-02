# Lane `trustfix` — status

**State: DONE.** Branch `p3ll/trustfix` (base `p3-learning-loop` @ 823b44c). Deliverable:
`docs/phase3/learning-loop/TRUSTFIX.md`.

## Done
- Recomputed the knowledge-free eval: falseActionRate 0.133 = 2 wrong of 15 ACTION findings.
- Traced all 15 ACTIONs to rule, change, env fact, attribution and upstream evidence. 13 are
  justified. The 2 scored wrong are dataset issues, not engine bugs:
  - crossplane: E3 matcher `(?i)StoreConfig` selects a bundled schema change; the ACTION is E1's
    correct verdict;
  - kyverno: the E9 review label conflicts with the DESIGN §1.4 `setting-ignored` → ACTION contract.
- New applicability rules: only `crd-enum-value-removed` produced ACTIONs (2, both justified).
- Engine fixes, generic, each with a regression test:
  - `c804c91`: crd-field-removed why-block names only the exposed resources, each matched field once.
  - `3b42c1e`: removed CRD paths below a set list are decided element by element, with the
    CRD-definition-document exemption (finished by the GLM handoff, reviewed and kept).
- Found, not fixed (contract decision): values default-changed/added + unset → NOT AFFECTED. That
  accounts for 8 of 26 catastrophic cells (TRUSTFIX §5). Also found the registry-exact image matching
  gap.

## Gate panel (before → after)
applicabilityAccuracy 0.457 → 0.467 (FAIL); falseActionRate 0.133 → 0.133 (FAIL, needs §1
decisions; projected 0.00 if both accepted); the other five gates PASS unchanged. classificationAccuracy
0.429 → 0.438, notAffectedViolations 2 → 1, no regressions against eval/results.

## Review of the GLM handoff (3b42c1e, f566e69)
- 3b42c1e: correct and kept. It finishes my WIP. Its CRD-definition-document exemption fixes the real
  reason my WIP had no effect on the dataset (external-secrets supplies `crds/`, and the CRD document
  staged a usage entry that no resource covered). I re-ran the tests and the full eval and confirmed
  its A/B numbers.
- f566e69 (status log): one inaccuracy, corrected in TRUSTFIX §1a. The crossplane false action is not
  "via dataset-FP changes": it is E3's StoreConfig matcher attributing the bundled change to a
  not-affected item. The two crossplane ACTIONs are not both "false": only `crd-field-removed` is
  scored wrong.

## Files touched outside ownership
`internal/impact/crd_gvk.go`, `internal/impact/crd_gvk_test.go` (engine fixes the commander asked for).
No eval data, gates or results were edited.

## Open decisions
See TRUSTFIX §6 (kyverno E9; crossplane matchers; values-unset semantics; mirror matching).

## Test status
`go build ./... && go vet ./... && go test ./...` green.
