# Lane `verify2` — consumer observation for §7.1 + human-authored rule intake (F4) — Opus

Branch `p5ll/verify2`, worktree `.claude/worktrees/p5ll-verify2`. Ledger W-0032. Mandate: PROTOCOL amendment
"B0-v8 outcome; fallback rule pre-registered". Read `_common.md`, `status/verify.md` + `runs/verify-smoke.md`
(the previous verify lane), `internal/investigate/verify/`, `internal/investigate/runtime/fixture.go`,
`products/exercises/fixtures/cert-manager.yaml`, `internal/knowledge/promote.go`, the review UI hypothesis
section, and the rule you exist to test: `docs/phase5/learning-loop/rules/rotationpolicy-ca-scoped.json`.
Smallest correct extension of the existing verifier — not a new subsystem.

## Why

The verifier's runtime exercise observes one thing: whether a Certificate's `tls.key` changes across the
upgrade. That can confirm "an explicit setting preserves the old behaviour" but it cannot test a rule whose claim
is about **consequence**: that some objects change without any consumer observing it. The pre-registered rule
says only CA Certificates (`spec.isCA: true`) that leave `rotationPolicy` unset change observably; a leaf's key
rotation is invisible to a consumer verifying through the CA chain. Today's method would refute it simply because
leaf keys rotate.

## Required

1. **Consumer observation** in the fixture runtime exercise: x509 chain verification of a leaf certificate issued
   **after** a CA re-issuance against the `ca.crt` a consumer captured **before** it (Go `crypto/x509`, computed in
   process over the observed Secrets; store only the outcome and certificate/public-key digests — never key
   material; Secret data stays HMAC-digested as today). Declared in the fixture (which objects, which trust bundle,
   which re-issuance order), generic Go: no product names in code; the cert-manager specifics live in
   `products/exercises/fixtures/cert-manager.yaml`.
2. **Arms from the rule**, as the rule file's `requestedVerification` states: shielded (CA `isCA: true` +
   `rotationPolicy: Never`, leaf unset) and exposed (CA `isCA: true`, unset), at FROM 1.17.2 and TO 1.18.0.
   Outcome rule, fixed by the protocol: **confirmed** iff shielded verifies at FROM and TO and exposed verifies at
   FROM and **fails** at TO; refuted if the shielded arm fails anywhere; otherwise inconclusive (a quiet exposed arm
   is never a pass). The existing `tls.key` observation keeps its current meaning for existing rules (the
   verify-smoke proofs must still re-verify).
3. **Human-authored rule intake**: the smallest path to put a rule file into a knowledge dir as a hypothesis
   proposal + whole-fact review item with provenance `human-authored` (author string recorded verbatim, e.g.
   "commander-drafted (Claude Opus 5.5), label-aware; adopted by <reviewer> at promotion"), so `ri verify
   hypotheses` and the review UI treat it exactly like an exported agent hypothesis. Encode the rule file
   **faithfully** in the fact grammar; if any part cannot be expressed exactly (field names, bounds shape, the
   `any` inside `overlap`), stop and message the commander — do not reinterpret it. Record the rule digest.
   The promotion gate is unchanged (human, whole fact, confirmed proof of that digest, no refutation, bounds,
   explicit supersedes/coexists).
4. **Tests** (synthetic, fake runtime): confirmed / refuted / inconclusive for the consumer observation incl.
   "exposed arm quiet ⇒ not confirmed" and "shielded arm fails ⇒ refuted"; key material never stored; intake
   round-trip (proposal + item + digest); old proofs still load and re-verify.
5. **Smoke** (real `-runtime`, `ri-verify-*` kind clusters only, deleted and verified absent afterwards) on a
   **scratch** copy of K0 (`scripts/phase5/snapshot.sh`): the pre-registered rule AND a deliberately wrong rule
   (a CA with `rotationPolicy` unset declared shielded) — the wrong one must be refuted. Record
   `runs/verify2-smoke.md` like `runs/verify-smoke.md`. This smoke is a mechanism check; the scored verification
   (W-0013) is run by the commander on the merged binary.
6. Do not write to the shared `.ri` store: read-only use of its fetch cache; your own scratch state dir (prompt8
   used a scratch state dir with symlinked caches). Never touch other kind clusters, temporal on :7233,
   `./bin/worker`, or the :8484 / :8485 servers. Never read `eval/investigations/*/{expected.yaml,NOTES.md,observed/}`
   or any `env/` of an evaluation case: the fixture is synthetic.
7. Docs: INVESTIGATE.md verify section (consumer observation, intake command).

Done per `_common.md`.
