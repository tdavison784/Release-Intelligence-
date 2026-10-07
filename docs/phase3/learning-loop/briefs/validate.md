# Lane `validate` — deterministic validators (G5)

Read `_wave1-common.md` first. Package `internal/semvalidate` per DESIGN.md §9 `validate`
(crd-schema, helm-values, compatibility, image, canonical-applicability). You built the CRD
default/enum/required capture in the `capture` lane — use it (`CRDVersionInfo` per-path fields) and the
values/compat/image snapshots via `internal/store`. Validators confirm only what artifacts prove and refute
what artifacts contradict; consequence is never confirmed (except the canonical `none` for `added`).
Every ValidationResult cites the artifact evidence of both sides. Table-driven tests incl. adversarial
(same path different kind, wrong default, renamed key that exists under the new name only, a
proposal with a plausible but non-existent path — must refute, not be inconclusive). Also: a
`restatement` check that confirms a note-derived proposal's subject+change against a computed diff in
the same edge (UNKNOWN-ANALYSIS §3.5-6: the cheapest tier-a win). Model: Sonnet.
