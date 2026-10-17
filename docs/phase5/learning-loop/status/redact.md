# Lane `redact` — status

Branch `p5ll/redact` (from `p5-learning-loop` @ af89c6dd). Brief: `briefs/redact.md`. Ledger W-0027 (F1 defect fix,
exposed by `B0-K0-20261017-d262210d`). **State: done**: build, vet and test are green, and the docs are updated.

## Root cause (verified on the B0 run's own bundle + a fresh smoke bundle)
- `bundle/redact.go` `redactNode`: a credential-named key (`sensitiveKey`, unanchored `privatekey`, `secret`, `token`,
  …) holding a mapping ⇒ `redactAll` wiped every scalar below it. That covers env files (snapshot) and the
  release-scope renders (change collector). In B0 the agent read `env/platform/pki/overlays/prod/mesh-root-ca-prod.yaml`
  and saw `rotationPolicy: "<redacted sha256:6300ef800bb8>"`. That marker is `Marker("Never")`.
- The render model (`internal/render/model.go`) **never wiped mappings**: `sensitiveKey.MatchString(k) && isScalar(x)`
  only redacts scalars and recurses into maps (probe-tested). So env renders, incl. the pki overlay render, kept
  `rotationPolicy: Never`. The brief's premise for that file was inaccurate. The pin validator's effective view
  took `mesh-root-ca` from the overlay render, so it was not blinded there. The agent's raw file reads were.
- `env/resources.go` field facts also judge each leaf by its own key (`spec.privateKey.rotationPolicy` was visible).

## Design (why read-back is a separate step)
Collection runs before hashing and without a schema. In the payments envs the Certificate CRD comes only from the
cert-manager chart (env render), which the snapshot stage never sees. So:
1. **Collection** (`internal/investigate/redaction`, used by `bundle/redact.go` and by `walkFacts` for the facts
   artifact): Secret data and own-key credential scalars are unchanged (bool/null kept, as v1). A credential-named
   *list* is withheld whole (as v1). PEM private-key content is withheld under any key (new). Below a
   credential-named *mapping*, only bool/null/number are kept, and every string stays withheld.
2. **Read-back** (`redaction.Schemas.Recover`, read time, stored bytes never change): a marker is replaced by a value
   only if it sits below a credential-named mapping, its own key names no credential, the object is not a core
   Secret, and **exactly one** string enum value that the bundle's CRDs declare for that exact group/version/kind and
   field path has that marker. The output is always a schema constant, so nothing else can be re-exposed. With
   no CRD, a version mismatch, no enum, or a value outside the enum, the marker stays. Applied in:
   validate `loadView` (all objSources), dossier `view`, `read_artifact` (adds `readBack` beside the verbatim text,
   so quotes still verify).
3. Markers that stay are **undecidable**. This fixes two latent fail-opens the work exposed: pins `field`/`textLine`
   compared a marker as a literal (a scope condition on a withheld value could make an object "provably out of
   scope" ⇒ clear), and schema-compat's enum check counted an object whose enum value is withheld as compatible
   (⇒ pass). Now the pin is unknown, and schema-compat gives `not-run` with reason `schema:withheld`.
4. `redactYAML` dropped the `---` separator of a re-encoded non-first document, merging it into the previous one
   (duplicate keys ⇒ unparsable file). The separator is kept now. No dev-case env file is multi-document.

Smoke (scratch state cloned from the shared cache, `-offline`, payments-us env, no agent, K0 empty):
`mesh-root-ca-prod.yaml` is stored with `rotationPolicy: "<redacted sha256:6300ef800bb8>"`, and `checkout-tls`
keeps `size: 256`. `read_artifact` on it gives readBack `spec.privateKey.rotationPolicy = "Never"` (and
`algorithm = ECDSA` on the leaf Certificates) from the chart render's Certificate CRD (6 types indexed).

## Versions (bumped)
- `bundle.snapshot@v1→v2`, `bundle.change@v1→v2`: **every new bundle gets a new id, for every environment**, because
  the producer string is in every manifest entry. Stored bundles are immutable and keep their ids.
- `validate.override-inspection@v2→v3`, `validate.schema-compat@v1→v2` (view read-back + marker = undecidable). Run
  configs list validator names, so new investigations get new ids (expected on F2). Re-validating a stored bundle with
  v3 reads back old wholesale-wiped markers too (equally sound: same digest format).
- `investigate.dossier@v1→v2`. Agent producer and prompt unchanged. A `read_artifact` result now carries `readBack`
  when it applies, so an F1 transcript that read such a file does not replay byte-identically (F1 runs are superseded
  anyway).
- Stored runs: `cmd/ri.TestStoredRunsReverifyTheirDigests` passes, 160 case records re-verified. No `internal/domain`
  change (no schema regen).

## Tests (each would fail with the opposite / fail-open implementation; mutation-checked, 10 mutants all killed)
`redaction`: `TestKeep`, `TestRecoverReadsBackOnlyDeclaredEnumValues` (enum read back; free text, credential-named
enum field, value outside the enum, credential-named list and a marker outside a credential map are not),
`TestRecoverNeedsTheExactSchema`, duplicates, `AddCRD`. `bundle`: `TestRedactCredentialMapKeepsOnlyNonStrings`
(the brief's map: enum + free text + nested PEM + password-like `pin: "0420"` + `password`),
`TestRedactReadBackOnlySchemaEnum` (full chain redact → parse → read-back: only the enum value comes back),
`TestRedactPrivateKeyAnywhere`, `TestRedactSecretAndOwnKeyUnchanged` (v1 values pinned byte-exact),
`TestRedactKeepsDocumentSeparators`, `TestWithheldFact`. `validate`: `TestPinReadsBackWithheldEnumSetting`,
`TestPinScopeOnWithheldValue` (read back / no enum / outside enum), schema-compat `enum value withheld` cases.
`agent`: `TestReadArtifactReadsBackSchemaEnumOnly`, `TestDossierReadsBackSchemaEnumOnly`.
Mutants: keep strings under credential maps; v1 wholesale wipe; no PEM check; read back own-key credentials; read
back outside credential maps; pins literal compare; schema-compat ignores markers; no view read-back; facts by
last segment only; separators dropped.

## Files touched outside ownership (minimal, additive)
- `internal/investigate/validate/{envview,pins,schema,common}.go` (pins/coverage code): view read-back, marker ⇒
  unknown, read-back note in pin detail/evidence excerpt, validator name bumps.
- `internal/investigate/agent/{dossier,session,tools_read}.go` + new `withheld.go` (verdict code).
- `internal/investigate/api.go`: **CONTRACT-CHANGE(redact)**, additive `ReadResult.ReadBack []ReadBack`
  (`object, path, marker, value, note`, omitempty).
- `docs/INVESTIGATE.md`: new subsection "Redaction below credential-named maps".
- Proposed for DESIGN.md §2.3 (commander applies): replace "values-file keys whose last segment matches … likewise"
  with the v2 rules above, plus one sentence on read-back.

## Open questions / findings for the commander
1. **Render model is weaker than rule 2 (pre-existing, not changed).** It keeps every string below a
   credential-named map whose own key misses the render regex. A probe kept `credentials: {secretAccessKey: AKIA…}`
   in clear (`secretAccessKey` matches none of `secret$`, `secret[-_]?key`, …). Env renders are agent-readable.
   Applying rule 2 there was **not done**: it would also hide Secret *references* in renders
   (`volumes[].secret.secretName`, `secretKeyRef.name/key`, `*SecretRef.name`), which the relations graph resolves.
   That turns edges into "the render withholds the name" ⇒ traversal and coverage change in every case. It is a
   broad behaviour change, not this F1 fix. It needs a decision (user's data-sharing condition vs graph fidelity)
   before held-out runs. A narrower option: extend the render key regex for own-key credentials (`secretaccesskey`,
   `accesskey`, …) only.
2. **Dossier, not redaction (verdict code), seen in the smoke:** (a) an object-field setting derived from the item
   subject text keeps its Go casing (`Spec.PrivateKey.RotationPolicy`), and `fieldValues` is case-sensitive, so
   every object reads "unset". (b) The dossier ignores version-independent overlay renders and dedupes a kustomize
   patch file against its base (first path wins), so `mesh-root-ca` shows the base file, not the prod overlay. Both
   fail closed ("unknown"), but the B0 re-run's dossier will still not show the prod value. The agent sees it via
   `read_artifact` readBack.
3. schema-compat's `walkSchema` (relations view, `crdIndex.check`) still sees markers in enum fields as enum
   violations: a pre-existing false alarm (fail-closed), slightly reduced now that numbers are kept. Not touched.

## Integrity notes
- No `expected.yaml`/NOTES.md/observed opened. One `grep -rn validators eval …` incidentally printed six citation-URL
  lines from ingress-nginx `expected.yaml` files (URLs only, no labels or outcomes). Later greps exclude
  `eval/investigations`.
- No prompt, rule or gate edited. Fixtures are synthetic (`Gadget`/`example.test`, gizmo `Widget`).

## Cleanup
`bin/` removed; scratch state (APFS clones of the shared cache) and extracted B0 case removed from the session
scratchpad. No kind cluster or server was started.
