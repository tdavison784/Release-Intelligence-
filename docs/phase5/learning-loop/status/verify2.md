# Lane verify2 — status

Branch `p5ll/verify2`. Brief: `briefs/verify2.md` (W-0032). Mandate: PROTOCOL amendment 2026-10-17 "B0-v8 outcome;
fallback rule pre-registered". Docs: `docs/INVESTIGATE.md` "Consumer observation (lane verify2)" and
"Human-authored rules (`ri knowledge intake-rule`)".

## Done

- **Consumer observation** (runtime): `internal/investigate/runtime/consumer.go` — `FixtureSpec.Consumers
  []ConsumerCheck` (trust Secret key, re-issue order, verified Secret key, exercise), run at each version after the
  baseline observation and before the version's exercises (symmetric inside the exercise window, upgrade window
  clean). Re-issues one object after another with the declared exercise action, awaits its reaction field per
  object, then `crypto/x509` verification of the verified chain against the captured trust, in process. Records
  only outcome + sha256 digests (certificates, public keys) in `Run.Consumers`; only PEM CERTIFICATE blocks are read
  (anything else in a declared key ⇒ refused, undetermined). `failed` only for `x509.UnknownAuthorityError`;
  other x509 errors ⇒ undetermined.
- **Consumer observation** (verify): `internal/investigate/verify/consumer.go` — `SubjectFixture.Consumers`
  (role fields, consuming objects, trust, reissue, verify, references); selected when the rule's applicability
  names a role field (else the produced-object observation, unchanged); arms from the rule's alternatives in the
  role (shielded = overlap alternatives, exposed = exposure alternatives; contradictions with the role listed as
  limitations); outcome rule exactly as the protocol fixed it; `ProofArm.Consumer` carries per-version outcome +
  digests. Method producer `verify.exercise-consumer@v1` (the produced-object exercise stays `verify.exercise@v1`).
- **cert-manager fixture** (`products/exercises/fixtures/cert-manager.yaml`): consumer `ca-trust` (role
  `spec.isCA: true`; CA Issuer + leaf Certificate per arm; trust = CA Secret `ca.crt`; reissue CA then leaf; verify
  leaf `tls.crt`), upstream references quoted from cert-manager docs (ca.md, types_certificate.go v1.18.0).
- **Human-authored intake**: `internal/knowledge/intake.go` + `ri knowledge intake-rule -dir D -author A F`
  (`cmd/ri/knowledge_intake.go`). Strict decode + round-trip faithfulness check (refuses, never reinterprets);
  proposal provenance rule `human-authored:<hyp-id>` (id from the file sha256), Model = author verbatim, PromptDigest
  = CallID basis = file sha256, provider `human-authored`; whole-fact item; idempotent; reuses an existing item of the
  same rule (`itemExisted`). `knowledge.ParseHypothesisRule` accepts the kind ⇒ routing ignores it, gate/verify/UI
  treat it as a hypothesis. Verify reads its bounds from `SemanticProposal.Release`.
  The real rule file `rules/rotationpolicy-ca-scoped.json` is taken in faithfully on a scratch K0 copy: file sha256
  `659b1255…6e6f` (= the pre-registered digest), hypothesis `hyp-8fb090745e65`, candidate `sc-0564d2273368`, review
  item `ri-612fd8261e22`, rule (assertion) digest `7089555d9e86`, proposed-rule digest (with bounds) `742492cb653d`.
- Review UI: a human-authored item is headed "Human-authored rule" and its statement labelled "human-authored —
  not agent-proposed, not verified" (author, prompt version, rule file digest shown).
- Docs: INVESTIGATE.md (two subsections + tests paragraph), README command line, `ri verify` usage.

## In progress

- Real `-runtime` smoke on a scratch K0 copy → `runs/verify2-smoke.md` (see that file when present).

## Decisions (with why)

- **Selection by role field**: a rule that names the consumer's role field distinguishes subjects by that role, so
  the consumer observation tests it; every other rule keeps the old observation. Keeps the old meaning for existing
  rules with no flag, and is declarative (no product names in Go).
- **Arms from alternatives in the role**: the faithful reading of `any` in the overlap; for the pre-registered rule it
  yields exactly the two arms of its `requestedVerification` (shielded: CA + rotationPolicy Never; exposed: CA
  unset) and the leaf (rotationPolicy unset) is part of every arm's consumer chain, so "(b) the leaf's own key may
  change" is exercised. The deliberately wrong rule yields two shielded CA arms and no exposed arm ⇒ refutable, never
  confirmable.
- **Consumer checks run before each version's exercises**, so the produced-object degradations keep their meaning
  (the upgrade window does not see the consumer re-issuance).
- **`failed` only on an authority mismatch**: a validity/parse problem must never count as the exposed arm's
  predicted failure (that would be a fail-open confirmation).
- **Proposal method stays `ai`** for a human-authored rule: `SemanticProposal.Validate` admits only AI provenance
  and it is the least trusted label (a new Method value would make `Provenance.Deterministic()` true — fail-open).
  The human-authored label is in the provenance rule, provider and prompt version; the author string in Model.
- **Bounds on the proposal** (`SemanticProposal.Release`, only valid with a human-authored rule): the knowledge dir
  is self-contained; no run record exists to read them from.
- The trust bundle is the CA's own Secret `ca.crt` (self-signed: equals its `tls.crt`, seen in the verify-smoke
  artifacts), present as soon as the CA is issued.

## Files touched outside ownership (minimal, additive)

- `internal/domain/semantic.go` (`SemanticProposal.Release` + validation), `internal/domain/proof.go`
  (`HumanAuthoredRuleKind`, `HumanAuthoredHypothesisID`, `ProofArm.Consumer`, `ProofConsumer*`),
  `schemas/knowledge-record.schema.json` (regenerated).
- `internal/knowledge/promote.go` (`ParseHypothesisRule` accepts `human-authored:`; `HumanAuthored`).
- `internal/investigate/runtime/session.go` (`Run.Consumers`, session field, two `checkConsumers` calls),
  `fixture.go` (`FixtureSpec.Consumers`, validation), `fake_test.go` (Badge hooks for the x509 fake).
- `internal/reviewui/hypothesis.go`, `templates/detail.html` (human-authored label).
- `cmd/ri/knowledge.go` (dispatch + usage line), `cmd/ri/verify.go` (usage text), `README.md`.

## Contract changes

- Additive only (CONTRACT-CHANGE(verify2) notes in code): `SemanticProposal.release`, `ProofArm.consumer`,
  `runtime.Run.consumers`, `runtime.FixtureSpec.Consumers`, provenance rule kind `human-authored:`. No existing
  producer/policy version bumped; new producers `verify.exercise-consumer@v1`, `knowledge.intake@v1`. Stored Phase 4
  run records and the Phase 5 verify-smoke proofs load and re-verify (test `TestStoredSmokeProofsStillVerify`).

## Open questions

- None blocking. For W-0013 the commander runs `ri knowledge intake-rule -dir <K0 copy> -author "<verbatim>"
  docs/phase5/learning-loop/rules/rotationpolicy-ca-scoped.json`, then `ri verify hypotheses -knowledge <K0 copy>
  -runtime -kubernetes 1.31`, then the product owner's decision in `ri review serve` must name `vf-9b9cfa5c9f5f` as
  superseded (the gate refuses otherwise). `ri knowledge decide` has no `-supersedes` flag (UI only).

## Tests

- New: `internal/investigate/runtime/consumer_test.go` (real x509 fake: shielded verifies at both versions although
  the leaf key rotates, exposed fails at TO; no key or certificate bytes in any stored artifact; unanswered
  re-issuance and a private key in a declared key ⇒ undetermined; invalid checks refused),
  `internal/investigate/verify/consumer_test.go` (arms from the rule incl. the wrong rule; the outcome table:
  confirmed / exposed quiet / shielded fails at FROM or TO / undetermined / missing result / no baseline / no exposed
  arm / wrong rule refuted; intake → verify → gate end to end; stored smoke proofs), `internal/knowledge/intake_test.go`
  (round trip, digest, idempotence, reused item, routing unchanged, gate on human-authored items, refusals).
