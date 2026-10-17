# Lane `redact` — redaction blinds non-secret settings under credential-named maps (defect exposed by B0) — Opus

Branch `p5ll/redact`, worktree `.claude/worktrees/p5ll-redact`. Read `_common.md`, DESIGN §2.3 (redaction),
`internal/investigate/bundle/redact.go`, `internal/investigate/bundle/snapshot.go` (`redactedFacts`,
`redactEvidence`), `internal/render/model.go` (`sensitiveKey`, `redact*`), and how `validate/pins.go` treats
withheld values. Ledger W-0027. This is an F1 defect fix: smallest correct change, no other behaviour change.

## Defect

`redactNode` redacts the **whole subtree** of any mapping key matching `sensitiveKey` (`privatekey`, `secret`,
`token`, …). A resource's `spec.privateKey` map therefore loses non-secret settings such as `rotationPolicy`,
`algorithm`, `size`, `encoding`; the render model does the same for keys ending in `privateKey`. The agent then
cannot read an explicit setting, and the pin validator decides "values withheld (redacted)" ⇒ undecidable — so a
correct trusted rule can never apply. Generic: any CRD with settings nested under a credential-sounding map.

## Required (fail-closed for secrets — this is the user's data-sharing condition)

1. Never weaken: Secret `data`/`stringData`, PEM/private-key material anywhere (content check), and **scalar**
   values whose own key names a credential stay redacted exactly as today.
2. A credential-named key whose value is a **mapping**: do not wipe it wholesale. Recurse; each scalar below it is
   kept only when it is **provably not a credential**: a boolean / null / number, or a value constrained by a schema
   enum the bundle's CRDs declare for that exact field path. Everything else below it stays redacted. If no CRD for
   the kind is available at that point, everything below stays redacted (today's behaviour). If schema knowledge is
   not available where redaction runs (collection happens before hashing), find the narrowest correct design and
   explain it in status — e.g. a second, schema-aware un-withholding step must never re-expose a value that was
   not provably non-secret.
3. Same semantics in all three places (bundle files, env facts, render model); one shared helper if practical.
4. Producer/redaction version bump so stored bundles keep their identity and old runs still load and re-verify;
   new bundles for affected environments get new ids (expected, say which).
5. Tests: the synthetic credential-named map with an enum field + a free-text field + a nested PEM (enum kept,
   free text redacted, PEM redacted); a password-like value under such a map is redacted; Secret data unchanged;
   a mutation check that wholesale un-redaction fails a test.

Done per `_common.md`.
