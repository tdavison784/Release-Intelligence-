# Lane `envparse` — environment parsing extensions X1–X3, X5 (follow-up for the `envinv` agent)

Read `docs/phase3/learning-loop/UNKNOWN-ANALYSIS.md` §3.4 (X1–X6) and §3.5-2 (predicate operators).
These extensions give the deterministic applicability engine the environment facts it needs to evaluate
verified release knowledge. They are generic: no product names in Go code.

Branch: continue in your worktree; first `git checkout -b p3ll/envparse p3-learning-loop` (your envinv work
is already merged there). Owns `internal/env/*` (+ tests), env-related docs, schemas regeneration.

1. **X1 — scalar values on manifest fields.** `env.ManifestField` (or equivalent) keeps the scalar value
   of each leaf the way `ValuesKey.Value` does for Helm values, with the same secret-handling care the
   values side has (check how values are kept out of LLM prompts in `internal/impactenrich` — values must
   still never reach a prompt; add a test). Keep per-fact evidence (file, line, YAML path, excerpt).
2. **X2 — descend into sequences.** Paths through lists use `[]` markers, the same syntax as CRD
   `SchemaPaths` (e.g. `spec.acme.solvers[].http01.ingress.class`), recording each element's evidence
   (element index in the locator). Existing path-only consumers must keep working (GVK inventory,
   CRD matching ladder — check `internal/impact/crd_gvk.go`; the adversarial pack
   `go test ./internal/eval -run TestAdversarialPack` must stay green).
3. **X3 — line-addressable embedded text.** For ConfigMap `data` values (and generally multi-line string
   scalars), expose lines with line-level evidence (file line = scalar start + offset) so a predicate
   "line matching regex R exists / no line matches R" can cite the exact line.
4. **X5 — cross-resource references.** Generic reference resolution for `*Ref` objects carrying
   `name`(+`kind`/`group`) (e.g. `issuerRef`), resolving to the referenced manifest object when present
   in the supplied manifests; unresolved → explicitly unresolved, never "absent".
5. A small **query API** on `env.Environment` the applicability engine can call, e.g.
   `ResourcesOfKind(group, kind)`, `FieldValues(gvk-ish selector, path) []FieldFact` (with set/unset
   distinguishable per resource), `TextLines(...)`, `ResolveRef(...)`. Name and shape it cleanly; the
   `contract` lane's DESIGN.md predicates (field unset/equals/in/regex on GVK, line regex with negation,
   cross-resource reference) will be evaluated through it. If DESIGN.md has landed on
   `p3-learning-loop` by the time you design the API, merge it and align.
6. Tests: table-driven, offline, including adversarial ones (same field name under a different kind;
   list element vs scalar; a `---` in a block scalar; a secret-looking value never logged/rendered by
   default). e2e goldens: deterministic report must stay byte-identical (these are new facts, not new
   findings) — if anything changes, explain why in status.
7. Full eval: no regressions.

Model: Sonnet. Report `LANE DONE:` as before; status file `status/envparse.md`.
