# Upgrade plan — cert-manager v1.17.0 → v1.18.0, customer-repo environment (cluster k8s 1.28)

*Produced by the RI-ASSISTED arm of the time-study pilot: `ri impact
cert-manager v1.17.0 v1.18.0 --repo docs/phase3/review-packet/example-env/customer-repo
--kubernetes 1.28` (offline replay, <1 s), then reading the rendered report
(`reports/report-3-repo-mode/report.txt`), verifying three citations, and
scanning the UNKNOWN list for breaking changes. Sources the tool cites were
verified, not re-derived.*

## 0. Compatibility gate (from the report's ACTION REQUIRED, `imp-7e9a11c2834d`)

- v1.18.0 supports **k8s 1.29–1.33**; cluster is **1.28** → plan cluster
  upgrade to ≥1.29 first. Evidence verified: supported-releases table L306.
- Caveat the report does not state: v1.17.0 lists the same range (read from
  the same table row below), so we are already unsupported today — check the
  row for 1.17 yourself (L307).
- The report also notes the chart's `kubeVersion ≥ 1.22` is satisfied
  (`imp-d55868f8741c`) — i.e. Helm will not block; this is a support-policy
  decision, not a hard stop.

## 1. What the report proves with environment citations (act directly)

- **Image pins** (`imp-123e6594ecd5`, `imp-a137dded3ed6`, REVIEW REQUIRED):
  controller+webhook pinned `v1.17.0` across the deployment, kustomization,
  CI workflow and Terraform — bump pins + mirror to `v1.18.0`. Verified:
  `deployment.yaml` L12 carries the pin.
- **Values pin** (`imp-6cae808def98`, INFORMATIONAL): we pin
  `prometheus.servicemonitor.targetPort: 9402`; the default change
  (9402 → "http-metrics") does not reach us. Migrate to the port name later.
- **Values key becomes live** (`imp-0dae408f0c17`, REVIEW REQUIRED): prod
  values set `global.rbac.disableHTTPChallengesRole: true`, a key v1.18.0
  introduces — previously ignored, now live. Per the release-note excerpt the
  tool cites, true drops cert-manager's pod-create permission for HTTP-01.
  Our Certificate uses an ACME issuer → **reconcile this value before
  upgrading** (likely remove it), or HTTP01 challenges break.

## 2. What the report flags for triage (from its UNKNOWN list + AI layer)

- **ACME HTTP01 `pathType` → `Exact`** (`imp-eef2b0d0d72f`, AI-suggested
  review; breaking): identify the ingress controller serving the challenge
  path; if ingress-nginx ≥1.12 with strict validation, apply one of the three
  upstream workarounds. The report's evidence block points at the release
  notes section; read it (L28–L89) for the workaround values.
- **Five dependency CVE bumps** (`imp-fa8df652362e`, `imp-50009880b265`,
  `imp-82d81560bbc8`, `imp-88e0a1b18609`, plus x/net; AI-suggested): ship
  inside the images we deploy after upgrade — no separate action; record for
  scanner exceptions.
- **Repo-mode warnings** (end of report): helmfile read literally,
  kustomization not built, vendored chart skipped, two values files
  last-wins, Flux `valuesFrom` unreadable → keep the human checks those imply
  (Flux range `1.*` can roll forward; confirm the authoritative install path).

## 3. What I found in the UNKNOWN list that the tool did NOT join (and the AI missed)

- **`Certificate.Spec.PrivateKey.RotationPolicy` default `Never`→`Always`**
  (`imp-19acd0659637`, `imp-355ebba8a382` — UNKNOWN, no AI suggestion at
  `-enrich-max 20`): our `base/certificate.yaml` sets no `rotationPolicy` →
  after upgrade, renewals rotate the private key. Decide: pin `Never` or
  accept rotation.
- **`Certificate.Spec.RevisionHistoryLimit` default `nil`→`1`**
  (`imp-3acf8d10d51e`, `imp-bbe863f802bd` — UNKNOWN): stale
  CertificateRequests will be GC'd on upgrade; acceptable, but tell anyone
  consuming CertificateRequest history.
- Lesson recorded for the protocol: **the UNKNOWN list is not optional
  reading.** In this run the two breaking API defaults appear only there
  (titles are accurate); skimming the funnel alone would have missed them.

## 4. CRDs, observability

- CRD changes are additive (short names, ACME profile fields — report's CRD
  evidence records); apply v1.18.0 CRDs with the upgrade.
- New certificate timestamp metrics (`imp-6432f469b85a`, AI-suggested): add
  to dashboards if wanted.

## Rollout order

1. Cluster to ≥1.29 (or signed-off exception).
2. Remove/reconcile `global.rbac.disableHTTPChallengesRole: true` (breaking
   interaction with our ACME issuer).
3. Decide the two Certificate default flips for `example-com`.
4. Apply CRDs; upgrade via the authoritative install path; check the Flux
   `1.*` range.
5. Bump image pins + mirror; watch challenge solves and renewals.
