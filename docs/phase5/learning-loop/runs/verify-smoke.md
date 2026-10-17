# Lane verify — real `-runtime` smoke run (2026-10-17)

**What this is:** a mechanism check of `ri verify hypotheses -runtime` on the real cert-manager controller and the
synthetic fixture (`products/exercises/fixtures/cert-manager.yaml`). **What it is not:** an experiment result. Both
hypotheses were **hand-authored** by lane verify (provenance model `hand-authored-smoke`, never an agent), written
into a scratch knowledge directory (not K0/K1, not `knowledge/`), with a scratch run record carrying their
mitigation specs. No evaluation environment, `expected.yaml`, NOTES.md or `observed/` was read or used. The
fixture contains only the controller, a self-signed Issuer and two synthetic Certificates (`ri-arm-present`,
`ri-arm-absent`) in namespace `ri-verify`.

Rule base: the K0 release-level fact `vf-9b9cfa5c9f5f` (cert-manager 1.18.0, `Certificate.spec.privateKey.rotationPolicy`
default Never → Always) with an added mitigating (overlap) condition. Bounds (1.17.2, 1.18.0]. Citation: the fact's
upstream evidence `ev-c92478af13c4` (release-notes-1.18.md, digest `sha256:2d13485d…`, resolved in the fetch cache).

```
./bin/ri -v -state <primary .ri> verify hypotheses -knowledge <scratch>/knowledge -store <scratch>/state -runtime -kubernetes 1.31
```

| Item | Hypothesis (mitigation) | Citation check | Rule exercise | Proof |
|---|---|---|---|---|
| `ri-582b21db0eed` | `rotationPolicy: Never` preserves the previous behaviour | pass | **pass** — 207 s, cluster `ri-verify-f58409c8`: renewal answered on both arms at 1.17.2 and 1.18.0; only `ri-arm-absent-tls` changed `data:tls.key` at TO | **confirmed** (observed) `pf-c97432888ae9` |
| `ri-c30a28aa20db` | an incorrect mitigation: "an ECDSA key (`privateKey.algorithm: ECDSA`, rotationPolicy unset) is not rotated" | pass | **fail** — 194 s, cluster `ri-verify-547f4d78`: the condition arm's `ri-arm-present-tls` also changed `data:tls.key` at TO | **refuted** (observed) `pf-55e8a74ec33e` |

Both clusters were deleted and verified absent (`cleanup.deleted/verified: true` in each `run.json`).

Gate check on the stored proofs (`ri knowledge decide`, on a scratch copy):

```
ri-c30a28aa20db (refuted), human accept  → refused: promotion gate: the rule was refuted by proof pf-55e8a74ec33e
ri-582b21db0eed (confirmed), proxy accept → refused: promotion gate: only a human can promote an agent-proposed rule
ri-582b21db0eed (confirmed), human accept → fact vf-78f46e0df915 at level human   (scratch copy only)
```

Files: `verify-smoke/output.txt` (command output), `verify-smoke/proofs/*.json` (the two knowledge records),
`verify-smoke/artifacts/<fixture>/{run,observations}.json` (the runtime artifacts the proofs cite by digest;
Secret data appear only as per-run HMAC digests). Note an earlier draft of the negative control used
`rotationPolicy: Always` as "the mitigation": that rule is in fact *correct* (an explicit Always behaves the same
before and after a default change), so it was replaced by a genuinely wrong rule before the run.
