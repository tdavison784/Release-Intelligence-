# Lane `redact` — status

Branch `p5ll/redact` (from `p5-learning-loop` @ af89c6dd). Brief: `briefs/redact.md` plus the commander's decisions
on the two open items (2026-10-17). Ledger W-0027 (F1 defect fix, exposed by `B0-K0-20261017-d262210d`; this is F2).
**State: done**: build, vet and test are green, and the docs are updated.

## Root cause (verified on the B0 run's bundle + fresh smoke bundles)
- `bundle/redact.go` wiped the whole subtree of a credential-named key that held a mapping. That covered env files
  and release-scope renders. The B0 agent read `mesh-root-ca-prod.yaml` and saw
  `rotationPolicy: "<redacted sha256:6300ef800bb8>"`, which is `Marker("Never")`.
- The render model never wiped mappings, but it kept **every** string below them whose own key missed its
  expression. A probe kept `credentials.secretAccessKey: AKIA…` in clear in a render. Renders reach the agent.
- The dossier built its own object view: it ignored version-independent overlay renders and deduplicated a
  kustomize patch against its base, and it matched upstream-text Go field names case-sensitively. So it showed
  `mesh-root-ca` from the base file and "unset" everywhere.

## What changed
1. **One rule, `internal/redaction` (redaction@v2), shared by the collector, the facts artifact and the render
   model.**
   - (a) Secret data/stringData.
   - (b) PEM private keys anywhere, by content.
   - (c) Own-key credential scalars (bool/null kept), except object-reference keys: `name`, `namespace`, or ending
     in `…Name`/`…Namespace`; `key` only beside a `name` in the same mapping (the `{name, key}` selector shape;
     commander tightening, a812d54d). A lone `key` is withheld.
   - (d) Below a credential-named map **or list**, only bool/null/number and reference keys are kept. Lists are
     walked like maps; otherwise `imagePullSecrets[].name` would vanish from renders.
   - (e) Credential-named env-var values (collector and render; the render model already had this).
   - Credential names are the union of both expressions plus `access[-_]?key`. Neither old expression matched
     `accessKey`, which the commander listed.
   - CRD documents are schemas: only (b) applies in them. Otherwise rule (d) would withhold a CRD's `privateKey`
     property descriptions, types and enum values in renders, breaking the read-back and schema defaults.
   - The render model leaves existing markers alone (a collected file parses unchanged).
   - The `---` separator of a re-encoded document is kept (v1 merged documents).
2. **Read-back** (`Schemas.Recover`, read time, stored bytes unchanged): a marker of either form below a
   credential-named map or list is read back only to the single string enum value the bundle's CRD declares for
   that exact group/version/kind and path. Own-key credentials and Secrets are never read back. It runs in the
   validators' effective view, the dossier, and `read_artifact` (`readBack` beside the verbatim text).
3. **Markers are undecidable**: pins `field`/`textLine` return unknown; schema-compat returns
   `not-run`/`schema:withheld` instead of pass. Both were latent fail-opens: comparing a marker as a literal value
   could exclude an object from a rule's scope, or pass a removed enum value.
4. **Dossier@v3**: objects come from `validate.EffectiveView` (new export, the pin validator's own view), one
   entry per object with a render before a file. An object whose consuming render is unavailable is unknown. A
   Go-cased subject path is resolved via the view's CRD schema (case-insensitive by property); without a
   confirming CRD it is lowerCamel-cased and marked unverified, so an unset field is unknown, not the default.

Smoke (payments-us, `-offline`, no agent, empty knowledge, scratch clone of the shared cache):
- The pki overlay render stores `rotationPolicy: <sensitive …>`, keeps `size: 256` and `secretName: mesh-root-ca`.
- The env file keeps `secretName: checkout-tls`.
- The dossier for `ci-86548e9a2300` shows setting `spec.privateKey.rotationPolicy`: `mesh-root-ca` explicit
  `"Never"` from `render kustomize:platform/pki/overlays/prod (version-independent overlay)` (read back from the
  chart CRD's enum), and `checkout-tls`/`ledger-tls` inherited-default from their overlay render.
- The graph's Secret nodes now carry real names (`core/v1/Secret/payments/ledger-tls`), not markers.

## Versions
- `bundle.snapshot@v2`, `bundle.change@v2`, `renderdiff@v2`: every new bundle gets a new id, in every environment.
  Stored bundles and artifacts are immutable and keep their ids. The render cache stores raw tool output and
  re-normalizes on read, so it needs no invalidation. `render.Producer` (`render@v1`, in Phase 1–3 evidence ids)
  is unchanged.
- `validate.override-inspection@v3`, `validate.schema-compat@v2`: run configs list validator names, so new
  investigations get new ids.
- `investigate.dossier@v3`. Agent producer and prompt unchanged. `read_artifact` results carry `readBack` when it
  applies, so F1 transcripts that read such files do not replay byte-identically (F1 runs are superseded).
- Stored runs: `cmd/ri.TestStoredRunsReverifyTheirDigests` passes (160 case records). No `internal/domain` change.

## Expected behaviour changes on F2 (for the report)
- Secret references (`secretName`, `…Ref.name/key`, `secretKeyRef`, `imagePullSecrets[].name`) are now readable in
  collected env files (v1 redacted them via the unanchored `secret`), so graph edges to those Secrets resolve by
  name.
- Strings below credential-named maps are withheld in renders (read back where an enum declares them).
- `existingSecret`-style own-key strings are withheld in both places, as before in at least one of them.

## Tests (mutation-checked; every mutant killed)
New: `redaction` (`TestKeep` incl. reference keys + union names, read-back incl. render markers),
`render.TestRenderRedactsCredentialsKeepsReferences` (secretAccessKey, accessKey, clientSecret, passphrase,
bearer, existingSecret, PEM, list items, env value withheld; secretName, privateKeySecretRef.name,
tokenSecretRef.key, imagePullSecrets[].name, secretKeyRef.name, numbers/bools readable),
`TestRenderCRDKeepsSchemaUnderCredentialProperties`, `TestRenderKeepsExistingMarkers`,
`relations.TestReferencesBelowCredentialKeysResolve` (secretName + privateKeySecretRef.name resolve, edges
established, secretAccessKey withheld), `bundle.TestRedactSharedRule`, `TestWithheldFact` (shared rule, CRD),
`agent.TestDossierEffectiveViewAndJSONPath`, `TestDossierFailsClosed`, `TestLowerCamel`, plus the earlier
read-back / pins / schema-compat / separator tests.
Mutants: earlier run (10) + this round's 15. Rule (c): no reference exception; exception for every key; old
render regex. Rule (d): keep all strings below maps; no reference exception below maps; render maps not
containers; no CRD exemption; render re-redacts markers. Rule (e): render, collector. Dossier: Go casing kept;
unverified ⇒ default; unrendered ignored; overlays ignored. `key`/`name` tightening (6, all killed): key always a
reference key; never; collector, render model or facts ignoring the sibling; facts sibling from another mapping.
Tests: `credentials: {key: AKIA…}` and `privateKey: {key: …}` withheld, `tokenSecretRef: {name, key}` readable,
in `TestKeep`, `TestRenderRedactsCredentialsKeepsReferences`, `TestRedactSharedRule`, `TestWithheldFact`,
`TestRedactedFactsKeyNeedsNameSibling`.

## Files touched outside ownership (minimal)
- `internal/render/model.go` (rule now from `internal/redaction`), `internal/investigate/renderdiff/renderer.go`
  (producer bump).
- `internal/investigate/validate/{envview,pins,schema,common}.go` + new `effective.go` (exported view).
- `internal/investigate/agent/{dossier,session,tools_read}.go` + new `withheld.go`.
- `internal/investigate/relations/redaction_test.go` (new test only).
- `internal/investigate/api.go`: **CONTRACT-CHANGE(redact)**, additive `ReadResult.ReadBack []ReadBack`.
- `docs/INVESTIGATE.md` ("One redaction rule" subsection). Proposed for DESIGN.md §2.3: replace the redaction
  paragraph with rules (a)–(e) + the CRD note + read-back.

## Notes / residual risks for the commander
- Rule (c)/(d) keep `name` under a credential map by design, and `key` only in the `{name, key}` selector shape.
  A credential stored as `{name: …, key: <secret>}` would still be readable; that is the decided shape.
- The CRD exemption trusts that a CRD document holds no secret material except PEM. A `default:` containing a
  non-PEM credential in an environment-supplied CRD would stay readable.
- `env/resources.go` (the in-memory facts the impact engine reads, never sent to a model) keeps its own-key rule.
  Changing it would move A0 classifications and is not a provider-exposure issue.
- Evidence excerpts (`redactEvidence`, facts excerpts) still replace any excerpt mentioning a credential word,
  including `secretName: …` excerpts. That is stricter than the rule; unchanged.
- Dossier dedupe prefers a render's copy of an object over an unconsumed file's copy (as v1/v2 did). The pin
  validator evaluates both.

## Integrity notes
- No `expected.yaml`/NOTES.md/observed opened. Earlier, one `grep -rn validators eval …` incidentally printed six
  citation-URL lines from ingress-nginx `expected.yaml` files (URLs only, no labels or outcomes).
- During this round's mutation run, the harness's `git checkout` reverted the then-uncommitted `dossier.go`. It was
  reapplied from the recorded patch steps; the agent test cache hit confirms byte-identical content.
- No prompt, rule or gate edited. Fixtures are synthetic (`Gadget`/`example.test`, gizmo `Widget`/`Account`).

## Cleanup
`bin/` removed; scratch state clones, extracted B0 case and patch scripts removed from the session scratchpad. No
kind cluster or server was started.
