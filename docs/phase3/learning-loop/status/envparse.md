# Lane `envparse` status

**State: done** (branch `p3ll/envparse`, from `p3-learning-loop`). `go build ./... && go vet ./... && go test ./...`
pass (incl. `TestAdversarialPack`, e2e goldens byte-identical, enrichment replay cache untouched). Full eval before/after:
output byte-identical, `no regressions against eval/results`; only the pre-existing applicabilityAccuracy gate fails.

## Done (UNKNOWN-ANALYSIS X1, X2, X3, X5 + query API)
All in `internal/env/resources.go`, stored in a NEW `Environment.Resources []Resource` (additive).
- **X1** `FieldFact.Value`: JSON-encoded leaf identical to `ValuesKey.Value` (`normalize.EncodeLeaf`, a 1-line export of
  the existing encoder); container facts for mappings/sequences; evidence = file, line, `$.<indexed path> (Lnn)`, excerpt.
- **X2** sequences descended with `[]` markers (CRD `SchemaPaths` syntax) + indexed `Element` path in the locator;
  nested sequences, empty lists, list-of-scalars, list-of-maps.
- **X3** `TextBlock`/`TextLine`: multi-line string scalars and every core ConfigMap `data` value; per-line evidence;
  `FileLine` exact for literal `|` scalars (scalar start + 1 + offset, verified against the file in a test), `Exact=false`
  for folded/quoted (documented). `---` inside block scalars does not split documents (existing decoder; tested).
- **X5** `Ref` for any mapping under a `*Ref` key (and elements under `*Refs`) with a `name`; `ResolveRef`:
  `resolved | ambiguous | unresolved`, group/kind constrain only when stated, namespace defaults to the referrer's;
  `ManifestsComplete` says whether "unresolved" can mean "not there" (unresolved is never reported as absent).
- **Query API** on `*Environment`: `ResourcesOfKind(group, kind)`, `Select(GVKSelector)`, `FieldValues(sel, path)`
  (per resource `Set` true/false), `TextBlocks(sel, pathPrefix)` + `TextBlock.Match(re)`, `References(sel, path)`,
  `ResolveRef(from, ref)`. Selector semantics: `Group ""` = core only, `"*"` = any; `Version ""` = any; `Kind "*"` = any.
  DESIGN.md had not landed on `p3-learning-loop` when I designed this, so the shape is mine; the contract lane may
  ask for renames — the surface is small.
- **Secrets**: values withheld (no `Value`, no value in the evidence excerpt, `Withheld="sensitive"`) for `Secret`
  resources, credential-named keys, and anything containing a private key; text lines assigning such keys or inside key
  blocks are withheld and never match (`TextBlock.Withheld` counts them so a negated predicate can refuse to decide).
  Oversize leaves (>4 KiB) withheld as `oversize`. Test `impactenrich.TestPromptNeverSendsManifestValues` pins that no
  manifest value/line/secret reaches a prompt.
- Docs: `docs/IMPACT.md` ("Resource facts and the query API").

## Decisions
- **Separate store, not `ManifestFields`/`GVKUsage`**: those feed report counts, the CRD matching ladder (a non-empty
  `FieldPaths` marks manifest-backed use) and the enrichment prompt (digests/caches). Extending them would change
  findings or digests; the new facts are pure additions, so goldens/eval are identical.
- Credential-key redaction is deliberately over-inclusive (`secret$`, `token`, `password`, … anywhere in the key):
  a predicate on such a field is not decidable (Withheld) rather than risking a leaked value.
- Caps (100k facts, 50k text lines, 5k per block, 200 refs per resource) warn and degrade the manifests dimension.

## Outside ownership
- `internal/normalize/helm.go`: additive exported `EncodeLeaf`.
- `internal/impactenrich/prompt_test.go`: additive test + 3 imports.
- No schema change (nothing reaches `domain`), no golden change.

## Open / for the commander
- The ConfigMap text rule keys on core `ConfigMap` `data*` paths (k8s-generic, not product logic).
- Predicates on withheld fields/lines must map to UNKNOWN(reason) in the engine (`Withheld` / `TextBlock.Withheld`).
- `FieldValues` returns container facts too (`Container=true`); engine predicates "equals/in/regex" should ignore them.
