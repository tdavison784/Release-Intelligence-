# Lane verify2 — real `-runtime` smoke run of the consumer observation (2026-10-17)

**What this is:** a mechanism check of `ri knowledge intake-rule` and of `ri verify hypotheses -runtime` with the
consumer observation, on the real cert-manager controller and the synthetic fixture
(`products/exercises/fixtures/cert-manager.yaml`, consumer `ca-trust`). **What it is not:** the scored
verification (W-0013), which the commander runs on the merged binary. Everything ran on a **scratch** copy of K0
(`scripts/phase5/snapshot.sh 03b6ff43`, digest `sha256:672a7ce0…4ff5`, 16092 files) with a scratch state dir
(fetch caches copied from the primary `.ri`, `-offline`; the shared `.ri` was not written). No evaluation
environment, `expected.yaml`, NOTES.md or `observed/` was read or used. The fixture holds only the controller, the
self-signed Issuer, and per arm a CA Certificate, a CA Issuer and a leaf Certificate in namespace `ri-verify`.

## Rules taken in

| Rule | File sha256 | Author string (verbatim) | Hypothesis | Review item | Rule digest |
|---|---|---|---|---|---|
| pre-registered `rules/rotationpolicy-ca-scoped.json` | `659b125555e9696f376598547b41cd4130a14e74a9e0eb8ce16728b5cc506e6f` (= PROTOCOL) | `commander-drafted (Claude Opus 5.5), label-aware; adopted by <reviewer> at promotion` | `hyp-8fb090745e65` | `ri-612fd8261e22` | `7089555d9e86` (with bounds `742492cb653d`) |
| deliberately wrong `verify2-smoke/rule-wrong-ca-shielded.json` (a CA with rotationPolicy unset declared shielded; non-CA unset exposed) | `c2d4fffd66638770da6af26b8175266fd5d2ea6fa47b73c4a361f0fa55072b07` | `lane verify2 (Claude Opus 5.5): deliberately wrong negative control, smoke only` | `hyp-b5ae21404d96` | `ri-8de20d2775af` | `e5ddbc0299a4` |

Both anchored on candidate `sc-0564d2273368` (cert-manager 1.18.0, holds both cited evidence ids); the pre-registered
file was encoded faithfully (strict decode + round trip, nothing refused). Intake output: `verify2-smoke/intake/`.

```
./bin/ri -v -offline -state <scratch>/state verify hypotheses -knowledge <scratch>/knowledge -store <scratch>/state -runtime -kubernetes 1.31
```

## Results

| Item | Arms built from the rule (consumer role `spec.isCA: true`) | Consumer at FROM 1.17.2 / TO 1.18.0 | Proof |
|---|---|---|---|
| `ri-612fd8261e22` pre-registered | **shielded** `condition-present`: CA + `rotationPolicy: Never`, leaf unset; **exposed** `condition-absent`: CA unset. (Overlap alternatives `isCA unset` / `isCA = false` contradict the role: listed as limitations.) | shielded verified / **verified** (the leaf's own key rotated at TO — `tls.key` changed, not decisive); exposed verified / **failed** (`x509: certificate signed by unknown authority (possibly because of "crypto/rsa: verification error" …)`; the CA's `tls.key` changed) | **confirmed** (observed) `pf-57da896bcf53` — citation check pass, exercise pass, 230 s, cluster `ri-verify-48e6ace9` |
| `ri-8de20d2775af` wrong rule | shielded `condition-present`: CA + `Never`; shielded `condition-present-2`: CA unset. No exposed arm (its exposure needs `isCA` unset: contradicts the role) — "can be refuted but not confirmed". | `condition-present` verified / verified; `condition-present-2` verified / **failed** | **refuted** (observed) `pf-97c17ca82902` — exercise fail, 219 s, cluster `ri-verify-f0fb4407` |

Public-key digests (sha256 prefixes) from `run.json` `consumers[]`, pre-registered run: the trust key captured
before re-issuance is the same at both versions on each arm (shielded `a83e8a629358`, exposed `deb4973a6493`); the
leaf key changes on every re-issuance at TO on both arms (shielded `3315d5f1ac7e` → `e5ab56591a0e`), which the
shielded consumer does not observe. Both clusters were deleted and verified absent (`cleanup.deleted/verified:
true` in each `run.json`; `kind get clusters` empty afterwards).

Gate check (`ri knowledge decide`, human reviewer, on a further scratch copy):

```
ri-8de20d2775af (refuted)   → refused: promotion gate: the rule was refuted by proof pf-97c17ca82902: a refuted rule cannot be promoted
ri-612fd8261e22 (confirmed) → refused: promotion gate: active fact(s) vf-9b9cfa5c9f5f state(s) the same subject (…); the decision must say whether the promoted rule supersedes each or coexists with it
```

The promotion itself (superseding `vf-9b9cfa5c9f5f`) is the product owner's decision in `ri review serve` and was
not made here; the end-to-end synthetic test covers a human promotion through the gate.

An earlier identical run (binary before the consumer method's producer string became `verify.exercise-consumer@v1`)
gave the same outcomes (`pf-556c32602bce` confirmed, `pf-68c3f40b2005` refuted); it is superseded by this record.

Files: `output.txt` (command output, apply/observe progress lines dropped), `proofs/*.json` (the two knowledge
records), `artifacts/<fixture>/{run,observations,exercises}.json` (the runtime artifacts the proofs cite by digest;
Secret data only as per-run HMAC digests; certificates and public keys only as sha256 digests; no PEM or key
material — checked), `intake/*.json`, `rule-wrong-ca-shielded.json`.
