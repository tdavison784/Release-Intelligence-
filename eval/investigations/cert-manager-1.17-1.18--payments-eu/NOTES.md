# cert-manager v1.17.2 → v1.18.0 — payments-prod-eu (pair `mesh-ca`, half 1 of 2)

Authored blind on 2026-10-16 by the groundtruth lane (Claude Opus 5.5) from upstream sources and an
observed upgrade in a throwaway kind cluster. No Release Intelligence pipeline, agent or eval output
was run or read. Sibling: `cert-manager-1.17-1.18--payments-us` (identical repository except one
overlay patch).

## The environment (what the fixture models)

A GitOps repo (Argo CD app-of-apps, `env/clusters/prod`) for a payments cluster that runs its own
internal PKI with cert-manager:

- `platform/pki/base`: `ClusterIssuer/selfsigned-bootstrap` signs `Certificate cert-manager/mesh-root-ca`
  (`isCA: true`, ECDSA P-256, no `rotationPolicy`); `ClusterIssuer/mesh-ca` is a CA issuer over its
  Secret. `platform/pki/overlays/prod` patches the root CA's duration/renewBefore/subject; **it does
  not set `rotationPolicy`** (the payments-us sibling does).
- `apps/payments/base`: leaf Certificates `ledger-tls` and `checkout-tls` from `mesh-ca`; `ledger`
  (nginx) requires client certificates and verifies them with `ssl_client_certificate /tls/ca.crt`
  from its own Secret; `checkout` calls ledger with its own cert and `--cacert /tls/ca.crt` from its
  own Secret. So each service's only trust anchor is the CA copy that was current when *its own*
  leaf was last issued: the classic "ca.crt of my own Secret" pattern.
- `platform/cert-manager/values.yaml`: no `config.featureGates`.

Impact therefore has to be traced across: Application → controller default → root CA Certificate
(base + overlay patch) → CA ClusterIssuer → leaf Certificates → Secrets mounted by two Deployments
whose configs read `ca.crt` → the mTLS call between them. Nothing on the leaf Certificates or the
Deployments mentions rotation.

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

- The root CA leaves `rotationPolicy` unset after the overlay, and the feature gate is on by default,
  so under v1.18.0 the CA's next re-issuance generates a new key (source 1 L113, source 3).
- A new CA key means leaves signed afterwards do not verify against the previous CA certificate. Leaf
  re-issuance is not triggered by the CA change (source 4 L118-L121), and each service trusts only the
  ca.crt in its own Secret, so after the CA renews there is a window — until every leaf has been
  re-issued *and* every pod restarted — in which a re-issued service and a not-yet-re-issued service
  reject each other. With the old default (`Never`), the CA keeps its key and subject, so old and new
  CA certificates verify the same leaves and there is no such window.
- The leaves' own key rotation is harmless here: nothing pins a leaf key, and peers verify leaves
  through the CA chain.
- B2: nothing in the repo consumes CertificateRequest objects, so pruning history breaks nothing.

## Observed outcome (kind, 2026-10-16) — `observed/run.log`, script `observed/run.sh`

Commands: `observed/run.sh <case> ri-groundtruth-cm-eu <charts>`; kind v0.24.0, Kubernetes
v1.31.0 (kind's default node image). Argo CD itself is not run; each Application was resolved into the
equivalent `helm upgrade --install` (chart + `platform/cert-manager/values.yaml`) and
`kubectl apply -k` of its path. The renewals that would happen on schedule (root CA 720h before
expiry, leaves 240h) are forced the way `cmctl renew` does it (status condition
`Issuing=True, reason ManuallyTriggered`). Cluster deleted at the end.

| Phase | Observation (excerpts from run.log) |
|---|---|
| 0 v1.17.2 install | all Certificates Ready, `ROTATION <none>` everywhere; checkout→ledger `ledger ok: client=CN=checkout.payments.svc` |
| 1 control, v1.17.2 | root CA re-issued (revision 1→2): **public key unchanged** `ec2f2913…` → `ec2f2913…` (new serial) |
| 2 upgrade to v1.18.0 | controller `quay.io/jetstack/cert-manager-controller:v1.18.0`; Certificates unchanged (`ROTATION <none>`), call still OK — nothing breaks at upgrade time |
| 3 root CA renewal on v1.18.0 | revision 2→3: **public key rotated** `ec2f2913…` → `b4b0d35b…` |
| 4 ledger leaf renewal + ledger restart | ledger leaf key rotated too (`133fac87…` → `8bfe47f1…`); `openssl verify` with ledger's ca.crt on checkout's leaf: `error 20 ... unable to get local issuer certificate`, and vice versa; **checkout→ledger: `curl: (60) SSL certificate problem: unable to get local issuer certificate`** |
| 5 checkout leaf renewal + restart | both cross-verifications `OK`; call OK again |
| B2 | CertificateRequests on v1.17.2: `mesh-root-ca-1`, `mesh-root-ca-2`, `checkout-tls-1`, `ledger-tls-1`; on v1.18.0 only the latest revision per Certificate remains (`mesh-root-ca-3`, `checkout-tls-2`, `ledger-tls-2`) |

An earlier run (same day, same script before the call-retry tweak) gave the same result with
different keys; the retry only covers connection-level errors (curl exit 7/28) right after a
rollout and was not needed in the recorded run (every call is `attempt=1`).

`observed/render.diff` is the `helm template` v1.17.2 vs v1.18.0 diff with this case's values: the
default change is not visible in rendered manifests (no feature-gate or rotation lines; CRD text and
image tags change), i.e. a render-level comparison alone cannot show this break.

## Judgement calls

- **Affected components are the two Deployments**, not the CA Certificate itself: the CA is the
  exposed object, the consumers that break are the services whose trust anchor is a per-Secret copy
  of the CA. The leaf Certificates are listed as not-affected for B1 (their own key rotation has no
  consumer that depends on it), so a system that flags every unset rotationPolicy as affected scores
  false positives on them.
- The outage is a window, not permanent: it lasts from the first leaf re-issued after the CA's
  re-issuance until every peer holds a leaf signed by the new key and has restarted. In production
  with 720h leaves renewing 240h before expiry at different times, that window is days to weeks of
  intermittent mTLS failures. That is breaking for the environment: `affected`.
- Nothing breaks at upgrade time (phase 2). The first failure needs the CA's next scheduled renewal;
  in this repo that is 720h before the 8760h CA expires. Timing does not change the class.
- B2 is `not-affected` (feature-unused): the history pruning is real and observed, but nothing in the
  complete repository reads CertificateRequests.
- The fixture's Argo CD repo URLs (`git.example.com`) are placeholders and never fetched.
