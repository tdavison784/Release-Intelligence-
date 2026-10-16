# cert-manager v1.17.2 → v1.18.0 — payments-prod-us (pair `mesh-ca`, half 2 of 2)

Authored blind on 2026-10-16 by the groundtruth lane (Claude Opus 5.5) from upstream sources and an
observed upgrade in a throwaway kind cluster. No Release Intelligence pipeline, agent or eval output
was run or read. Sibling: `cert-manager-1.17-1.18--payments-eu`.

## The environment (what the fixture models)

Identical to payments-eu (internal PKI with a cert-manager CA issuer; two payments services doing
mutual TLS, each trusting only the `ca.crt` of its own Secret) except for one line group:
`env/platform/pki/overlays/prod/mesh-root-ca-prod.yaml` also sets

```yaml
  privateKey:
    rotationPolicy: Never
```

on `Certificate cert-manager/mesh-root-ca` (a JSON-merge patch on the CRD object, so `algorithm`
and `size` from the base are kept; verified with `kubectl kustomize platform/pki/overlays/prod`).
`diff -r` of the two `env/` trees shows only that, the organisation/country, and the repo/cluster
names. The leaf Certificates still leave `rotationPolicy` unset.

## Upstream sources (all read in full for the relevant sections)

1. Release notes 1.18, cert-manager/website @ 2e32a5ef,
   `content/docs/releases/release-notes/release-notes-1.18.md`
   https://github.com/cert-manager/website/blob/2e32a5efc7a4ef284e271f54ac1b15e800e66fcc/content/docs/releases/release-notes/release-notes-1.18.md
   - L109-L113: "The default value of `Certificate.Spec.PrivateKey.RotationPolicy` is now `Always`"
     / "⚠️ Breaking change" / "We have changed the default value ... from `Never` to `Always`."
   - L120: the change is behind a feature gate enabled by default (beta).
   - L122-L136: the two ways to keep the old behaviour: explicit `rotationPolicy: Never` on the
     Certificate, or `config.featureGates.DefaultPrivateKeyRotationPolicyAlways: false` in the Helm
     values (only until 1.19, L126-L127).
   - L151-L160: `revisionHistoryLimit` default now 1; stale CertificateRequests are garbage-collected
     on upgrade (item B2).
   - L338: changelog entry, PR https://github.com/cert-manager/cert-manager/pull/7723.
2. Upgrade guide 1.17→1.18, same commit, `content/docs/releases/upgrading/upgrading-1.17-1.18.md`
   L8 and L12 (the same two default changes, nothing else).
3. Certificate usage docs, `content/docs/usage/certificate.md` L519-L541 ("Issuance behavior:
   Rotation of the private key": `>= v1.18.0` rotates by default).
4. CA issuer docs, `content/docs/configuration/ca.md` L5-L12 (CA issuers need planning "around
   rotation, trust store distribution") and L112-L124 ("Updating the secret used for the CA
   certificate won't trigger re-issuance of leaf certificates").
5. Proposal and discussion: https://github.com/cert-manager/cert-manager/issues/7601.

Chart tarballs used (from https://charts.jetstack.io): cert-manager-v1.17.2.tgz sha256
`9f8eaada785a870112c7b1874952b7e8776fd066608361c83ecee859314393ad`, cert-manager-v1.18.0.tgz sha256
`448f4a970dded49a5ceb7407d299c3d3eb5bce25c4a5199b9ce1ccb4e7df5f91`.

## Reasoning (before the run)

- The only key whose stability the services depend on is the root CA's (each side's trust anchor is
  a CA copy). The overlay pins it with the first of the two documented mitigations (source 1
  L122-L123), so v1.18.0's new default never applies to it: the CA keeps key and subject across
  re-issuance and every leaf verifies against every CA copy.
- The leaves now rotate their keys on re-issuance (default `Always`); nothing pins a leaf key, so
  that is invisible to the services.
- B2 as in payments-eu: history pruned, nothing consumes CertificateRequests.

To find the mitigation a system has to follow Application `pki` → `platform/pki/overlays/prod` →
the patch → the base object: the base file alone looks exactly like payments-eu.

## Observed outcome (kind, 2026-10-16) — `observed/run.log`, script `observed/run.sh`

Same script and procedure as payments-eu (kind v0.24.0, Kubernetes v1.31.0; Argo CD Applications
resolved into the equivalent helm / `kubectl apply -k`; scheduled renewals forced like
`cmctl renew`; cluster deleted at the end).

| Phase | Observation (excerpts from run.log) |
|---|---|
| 0 v1.17.2 install | root CA `ROTATION Never`, leaves `<none>`; checkout→ledger `ledger ok` |
| 1 control, v1.17.2 | root CA re-issued: public key unchanged `e8469314…` |
| 2 upgrade to v1.18.0 | controller v1.18.0; call OK |
| 3 root CA renewal on v1.18.0 | revision 2→3: **public key unchanged** `e8469314…` → `e8469314…` (new serial) |
| 4 ledger leaf renewal + restart | ledger leaf key rotated (`cc489874…` → `60d26f07…`, the new default at work on the leaf); both `openssl verify` cross-checks `OK`; **checkout→ledger `ledger ok`, attempt 1** |
| 5 checkout leaf renewal + restart | cross-checks `OK`; call OK |
| B2 | same pruning as payments-eu (`mesh-root-ca-1/-2` → only `mesh-root-ca-3`) |

An earlier run of the first script version gave the same key behaviour; one call in its phase 4 hit
`curl: (28) Connection timed out` immediately after the ledger rollout while both openssl trust
checks were `OK` (an endpoint-propagation race, not a TLS verdict). That is why the script now
retries connection-level errors (curl exit 7/28) only; in the recorded run no retry was needed.

`observed/render.diff`: identical to payments-eu's (same chart values); the items show up only as
CRD description text (L87-L93, L101-L102). The mitigating overlay patch is not part of the chart
render at all: it is in the environment's Kustomize output, which does not change between versions.

## Judgement calls

- B1 is `not-affected` with an established override (`kind: override`, the overlay patch), observed.
- The leaf Certificates are `not-affected` although the new default *does* change their behaviour
  (observed key rotation): no consumer depends on leaf-key stability. A system that reports them as
  affected is producing a false positive; one that says "behaviour changes, no impact" is right.
- This case and payments-eu share every upstream fact; the class differs only because of the
  environment. That is the point of the pair.
