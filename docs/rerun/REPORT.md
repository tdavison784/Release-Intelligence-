# Re-run of Phase 1 / Phase 2 checks with real network access

Lane `rerun`, branch `p3ll/rerun`, run 2026-10-02 from a machine with full network access and
`GITHUB_TOKEN=$(gh auth token)` (5000 req/h). Nothing under `docs/onboarding/`, `docs/research/`, `docs/FINDINGS.md`,
`products/` or `eval/` was modified: everything here is new under `docs/rerun/`.

## 1. Method (and what is *not* comparable)

| Artifact | Baseline (2026-10-01, sandbox) | Live re-run | Output |
|---|---|---|---|
| `ri check` x 28 | `docs/onboarding/checks/<id>.json` | same `-versions` release set, per-product fresh `-state`, `-refresh` | `docs/rerun/checks/` |
| `ri discover` x 25 | `docs/onboarding/discovery/*` | pinned to each baseline `-ref`, fresh state | `docs/rerun/discovery/` |
| **control**: baseline-era binary | | binary rebuilt at the commit that created each baseline proposal, run live | `docs/rerun/discovery-oldbinary/` |
| `ri drift` x 28 | (none stored) | vs baseline checks, vs live checks, and `-baseline -` (newest 3) | `docs/rerun/drift/` |
| `ri stats` | `ONBOARDING.md` numbers | over the live checks | `docs/rerun/stats.{json,md}` |
| Research claims (UNVERIFIED/blocked) | `docs/research/*.md` | 129 URL probes + content probes | `claims.tsv`, `raw/content-probes.txt`, `raw/registry-auth-probes.txt` |
| `ri eval` (G9) | `eval/results` | fresh state, `-refresh` (read-only, no `-update`) | `eval.live.{json,txt}` |

Classification of every difference (the brief's four classes plus two the data forced):
**(a)** sandbox artifact (unreachable then, reachable and consistent now) - **(b)** genuinely unavailable upstream -
**(c)** upstream changed since - **(d)** real defect revealed - **(e)** *definition/tool changed since the baseline*
(not network at all: the baseline JSON predates later commits to `products/*.yaml` or `internal/discovery`) -
**(t)** throttling/transient (HTTP 429, one-off transport error; **not** unavailability).

Two confounders were found and controlled, because a naive baseline-vs-live diff would mis-attribute them to the network:
1. **Baselines are stale relative to the current definitions.** istio (`0bb1a33`, 2026-10-02), strimzi (`6b35dc0`),
   kube-prometheus-stack (`9c7781f`), cert-manager (`f337e8f`), karpenter (`60ac65a`) changed after their baseline check was
   saved; the new subjects/availability show up as differences. These are class (e).
2. **Discovery code evolved** (`9662626`, `468bb20`, `640e313`). Control: the baseline-era binary run live reproduces the
   cilium baseline except the `updated:` date (2-line diff, `raw/control-cilium.oldbinary.proposed.yaml`), so cilium's
   8-line diff against today's binary is tool evolution, not network.

### ENOSPC incident (disk, not network)
The first parallel batch of `ri discover` + `ri drift` filled the disk (`ENOSPC`, 2026-10-02 ~07:10 local). All its outputs
(discovery, drift, `kube-prometheus-stack` discovery stderr) were **discarded** and re-run serially (shallow/partial
clones, per-product state dir deleted after each, scratch < 120 MB). The `ri check` reports (all 28) and stats were
completed and committed before the incident and were verified as parseable JSON. Nothing that failed because of disk is
counted as unavailable anywhere in this report. Stale `*.disc.started/finished` marker files from the discarded batch
remain in `raw/` (timestamps are not meaningful); I could not delete them (cleanup `rm` was blocked by a safety check).

## 2. Inventory of contaminated artifacts

`raw/inventory-grep.txt` (337 file:line hits for `unverified live|unreachable|blocked|sandbox|egress`). Dating: baseline
checks were all committed 2026-10-01 (`df5072c` Phase 1 records/checks for cert-manager, istio, argo-cd; waves 1-4 in each
`Onboard <product>` commit); commands are in `docs/onboarding/PROTOCOL.md` (`ri discover ... -out/-report`, `ri check <id> -o json`).

| Artifact class | Contaminated content (examples, file:line in inventory) |
|---|---|
| `docs/onboarding/checks/*.json` | 12 products carry `unverifiable` (cert-manager 12, ingress-nginx 47, postgresql 15, minio 19, falco 2) and `covered` (github-release-notes etc.) = API blocked |
| `docs/onboarding/records/*.yaml` | `relationships.unverifiable` and/or `unreachableSources` for 28 products (all records mention one) (vault 17 hits, crossplane 22, postgresql 14, ingress-nginx 12) |
| `docs/onboarding/discovery/*.proposed.yaml` / `.report.md` | 7 products embed "Unverified by discovery (... HTTP 403 / unavailable)" notes: crossplane, external-secrets, falco, flux, ingress-nginx, linkerd, prometheus-operator |
| `docs/research/*.md` | `UNVERIFIED`/blocked statements in 26 of 28 docs (cert-manager 24 hits, postgresql 12, cilium 11, vault 8, istio 9, argo-cd 9) |
| `docs/FINDINGS.md`, `docs/ONBOARDING.md`, `docs/phase2/*`, README | blocked-host statements (FINDINGS:28,53,59,99; ONBOARDING:18,53,319,345; PLAN:61-67) |

## 3. `ri check`: baseline vs live (28 products, same release sets)

All 28 live runs completed with no pipeline error except one: **argo-cd** aborts on the baseline's release `3.4.0`
(`error: version "3.4.0" is not a known release of argo-cd`, `raw/argo-cd.check.first-attempt.stderr`), see D2; it was re-run
with `3.4.1`. `ri check` otherwise exits 0, i.e. no failing relationship except D1.

## Per product: check outcomes baseline -> live

| product | releases | pass | covered | n/a | unverifiable | fail | subjects validated/failing/insufficient/unverifiable (base → live) | diffs A/C/D/E/T |
|---|---|---|---|---|---|---|---|---|
| actions-runner-controller | 21 | 284 → 284 | 0 → 0 | 4 → 4 | 0 → 0 | 0 → 0 | 14/0/0/0 → 14/0/0/0 | 0/0/0/0/0 |
| argo-cd | 6 | 59 → 66 | 6 → 0 | 13 → 12 | 0 → 0 | 0 → 0 | 12/0/1/0 → 12/0/1/0 | 10/1/0/26/0 |
| cert-manager | 6 | 78 → 96 | 6 → 0 | 18 → 24 | 12 → 0 | 0 → 0 | 14/0/4/2 → 17/0/2/0 | 42/0/0/6/0 |
| cilium | 20 | 336 → 336 | 0 → 0 | 24 → 24 | 0 → 0 | 0 → 0 | 17/0/1/0 → 17/0/1/0 | 0/0/0/0/0 |
| crossplane | 12 | 86 → 98 | 14 → 2 | 56 → 56 | 0 → 0 | 0 → 0 | 8/0/5/0 → 9/0/4/0 | 12/0/0/0/0 |
| elasticsearch | 20 | 165 → 165 | 0 → 0 | 95 → 95 | 0 → 0 | 0 → 0 | 10/0/3/0 → 10/0/3/0 | 0/0/0/0/0 |
| external-secrets | 19 | 212 → 231 | 19 → 0 | 8 → 8 | 0 → 0 | 0 → 0 | 12/0/1/0 → 13/0/0/0 | 19/0/0/0/0 |
| falco | 5 | 65 → 65 | 0 → 0 | 3 → 3 | 2 → 2 | 0 → 0 | 14/0/0/2 → 14/0/0/2 | 0/0/0/0/0 |
| flux | 6 | 110 → 110 | 0 → 0 | 10 → 10 | 0 → 0 | 0 → 0 | 19/0/1/0 → 19/0/1/0 | 0/0/0/0/0 |
| golang | 6 | 54 → 54 | 0 → 0 | 12 → 12 | 0 → 0 | 0 → 0 | 10/0/1/0 → 10/0/1/0 | 0/0/0/0/0 |
| grafana | 6 | 70 → 70 | 0 → 0 | 32 → 32 | 0 → 0 | 0 → 0 | 14/0/3/0 → 14/0/3/0 | 0/0/0/0/0 |
| ingress-nginx | 34 | 663 → 727 | 18 → 0 | 115 → 115 | 47 → 0 | 0 → 1 | 25/0/2/3 → 26/1/0/0 | 92/0/1/0/0 |
| istio | 6 | 141 → 141 | 3 → 3 | 18 → 48 | 0 → 0 | 0 → 0 | 24/0/3/0 → 24/0/3/0 | 0/0/0/30/0 |
| karpenter | 6 | 63 → 65 | 3 → 3 | 24 → 24 | 0 → 10 | 0 → 0 | 11/0/4/0 → 12/0/5/4 | 0/0/0/7/10 |
| kube-prometheus-stack | 5 | 60 → 65 | 0 → 0 | 15 → 15 | 0 → 0 | 0 → 0 | 12/0/3/0 → 13/0/3/0 | 0/0/0/5/0 |
| kyverno | 6 | 117 → 117 | 0 → 0 | 9 → 9 | 0 → 0 | 0 → 0 | 20/0/1/0 → 20/0/1/0 | 0/0/0/0/0 |
| linkerd | 18 | 134 → 134 | 0 → 0 | 262 → 262 | 0 → 0 | 0 → 0 | 15/0/6/0 → 15/0/6/0 | 0/0/0/0/0 |
| loki | 6 | 73 → 73 | 2 → 2 | 39 → 39 | 0 → 0 | 0 → 0 | 13/0/6/0 → 13/0/6/0 | 0/0/0/0/0 |
| minio | 19 | 164 → 164 | 0 → 0 | 26 → 26 | 19 → 19 | 0 → 0 | 9/0/2/1 → 9/0/2/1 | 0/0/0/0/0 |
| opensearch | 17 | 153 → 153 | 3 → 3 | 31 → 31 | 0 → 0 | 0 → 0 | 11/0/0/0 → 11/0/0/0 | 0/0/0/0/0 |
| otel-collector | 65 | 848 → 847 | 0 → 0 | 192 → 192 | 0 → 1 | 0 → 0 | 15/0/1/0 → 15/0/1/1 | 0/0/0/0/1 |
| postgresql | 15 | 40 → 55 | 0 → 0 | 20 → 20 | 15 → 0 | 0 → 0 | 4/0/1/1 → 5/0/0/0 | 15/0/0/0/0 |
| prometheus-operator | 6 | 81 → 81 | 0 → 0 | 9 → 9 | 0 → 0 | 0 → 0 | 14/0/1/0 → 14/0/1/0 | 0/0/0/0/0 |
| redis | 13 | 72 → 80 | 8 → 0 | 11 → 11 | 0 → 0 | 0 → 0 | 7/0/0/0 → 7/0/0/0 | 8/0/0/0/0 |
| strimzi | 41 | 551 → 592 | 4 → 4 | 60 → 60 | 0 → 0 | 0 → 0 | 15/0/0/0 → 16/0/0/0 | 0/0/0/41/0 |
| terraform-provider-aws | 14 | 116 → 119 | 4 → 1 | 118 → 118 | 0 → 0 | 0 → 0 | 11/0/6/0 → 11/0/6/0 | 3/0/0/0/0 |
| traefik | 24 | 181 → 185 | 8 → 4 | 166 → 166 | 0 → 0 | 0 → 0 | 12/0/3/0 → 12/0/3/0 | 4/0/0/0/0 |
| vault | 32 | 236 → 236 | 0 → 0 | 244 → 244 | 0 → 0 | 0 → 0 | 14/0/1/0 → 14/0/1/0 | 7/0/0/0/0 |

Totals by class (A sandbox artifact, C upstream changed, D defect, E definition changed since baseline, T throttling/transient): {'A': 212, 'E': 115, 'C': 1, 'D': 1, 'T': 11}


Reading the table: **212 differences are (a) sandbox artifacts** - `unverifiable`/`covered` became `pass` and the
`reference:install-manifest` / `reference:deploy-cloud` stand-ins became direct `oci` passes. Subject-level, `unverifiable`
went 12→0 (cert-manager), 3→0 (ingress-nginx), 1→0 (postgresql), `insufficient` shrank for cert-manager (4→2),
crossplane (5→4), external-secrets (1→0), ingress-nginx (2→0), postgresql (1→0). Still unverifiable live: **falco** (2,
chart `0.44.0`, genuine digest mismatch, class b), **minio** (19, anonymous pulls denied at docker.io/quay.io, class b),
**karpenter** (10 checks, throttling, class t), **otel-collector** (1, transient, class t). The classifier in
`report_tables.py` is rule-based (see the code); `argo-cd`'s large (e) count includes the 3.4.0→3.4.1 release swap.

## 4. Real defects revealed (class d), with evidence

**D1. ingress-nginx: `controller-chroot-image` genuinely does not exist for v1.10.0.**
Baseline: 25 `unverifiable` ("registry.k8s.io blocked and no manifest names the chroot image"). Live: 24 pass, **1 fail**
(`1.10.0: absent from registry.k8s.io/ingress-nginx/controller-chroot:v1.10.0`). Independently confirmed
(`raw/probes.txt`): the registry redirects to `us-east5-docker.pkg.dev/.../controller-chroot/manifests/v1.10.0` → **404**,
while v1.9.6, v1.10.1, v1.10.6, v1.11.0 → 200; the tag list (205 tags) jumps from the 1.9 line to `v1.10.1`. The definition
claims chroot "since v1.2.0" (`docs/research/ingress-nginx.md:85`). **Proposal:** add an `exceptions: [{versions: [1.10.0], reason: ...}]`
entry on `controller-chroot-image` (as argo-cd does for 3.4.0). The sandbox hid a real upstream anomaly.

**D2. argo-cd: v3.4.0 is a git tag but never a GitHub release; the definition/FINDINGS explanation is wrong.**
`git ls-remote` has `v3.4.0` (+ rc1..rc7); `gh api repos/argoproj/argo-cd/releases/tags/v3.4.0` → **404**; the release list
jumps `v3.4.0-rc7` → `v3.4.1` (2026-05-06). So the "v3.4.0 release has no assets at all" finding (FINDINGS.md:59,
`products/argo-cd.yaml:211`) is a misreading: there is no release object (assets 404 *because there is no release*).
Consequences: (i) the live canonical `github-releases` source lists 451 releases without 3.4.0, so `ri check -versions 3.4.0`
errors for the whole product, while the sandbox baseline got 3.4.0 only because `github-releases` was unreachable and
`git-tags` answered - **the set of "known releases" depends on which channel answered**; (ii) the exception on
`cli-linux-amd64` is moot for the primary channel. **Proposal:** reword the exception; decide whether a version present only
in tags (never published as release) should be a known release (the definition's own canonical channel says no), and make the
reproducible-baseline release list channel-independent or record the channel used.

**D3. HTTP 429 is reported as `unavailable`, with no backoff; outcomes depend on request rate.** karpenter: 10 checks
`unverifiable` with `fetch https://public.ecr.aws/v2/karpenter/...: unavailable (HTTP 429) TOOMANYREQUESTS "Rate exceeded"`.
Re-running karpenter alone after a 2-minute pause (`checks-retry/karpenter.json`) still gave 8; sequential spaced `curl`
(`raw/probes.txt`) returned 200 for all six manifests (karpenter and karpenter-crd at 1.12.0/1.13.1/1.14.0) with a valid token. So the
registry is reachable and the answer is *throttled by burst*, not unavailable. Per the brief this is noted as throttling,
not unavailability. **Proposal:** a distinct `throttled` state (honest in drift: neither pass nor drift), honour `Retry-After`,
cap per-host concurrency for `public.ecr.aws`. Related transient: otel-collector `otelcol_0.127.0_linux_amd64.tar.gz`
`http: error` once; passes on retry (`checks-retry/otel-collector.0.127.0.json`), same class.

**D4. Baseline check JSONs no longer describe the current definitions (class e, process defect).** Strimzi +41 passes
(`kafka-versions`), istio +30 `not-applicable` (era availability), kps +5, cert-manager +6, karpenter chart-contents subjects.
`ri drift`'s default baseline (`docs/onboarding/checks/<id>.json`) therefore re-validates a definition against a baseline
produced by an older definition. **Proposal:** regenerate baselines at definition-change time, or have `ri stats` flag a
baseline whose `definitionDigest` differs from the current definition.

**D5. Eval gates (not network dependent; reported, not hidden).** `ri eval` live (fresh state, 28 entries) = warm-cache
offline = `eval/results` (`no regressions`), so G9 numbers do not depend on reachability. But **2 hard gates fail on this branch:**
`applicabilityAccuracy 0.495 < 0.80` and `falseActionRate 0.059 ≥ 0.05` (1 of 17 action findings wrong; confusion matrix in
`eval.live.txt`). FLEET.md states the baseline "passes all gates except applicabilityAccuracy 0.095"; that no longer holds
on this branch (`falseActionRate` also fails; applicability is 0.495). Pass: criticalRecall 0.98, importantRecall 0.95.

## 5. Discovery

Live vs baseline (both pinned to the same ref): 
* **Network effect (clean, via old-binary control):** the 7 products whose baseline proposals contained 403/unavailable
  notes now validate. Old binary live vs baseline differs *only* by `notes: 'Unverified by discovery (...HTTP 403...)'` →
  `validatedAgainst: [releases...]` (crossplane 6, external-secrets 4, falco 5, flux 6, linkerd 4, prometheus-operator 6,
  ingress-nginx 6+6, postgresql name only). Status summaries: crossplane (no summary in old format)→ 5 historically-validated;
  external-secrets →4 validated/1 unverified; falco 3→4 validated; flux 2→3; linkerd 0→1; prometheus-operator 6→7; ingress-nginx
  →7. Unchanged (identical but timestamp) for 14 products: elasticsearch, golang, grafana, karpenter, kyverno, loki, minio,
  opensearch, otel-collector, redis, strimzi, terraform-provider-aws, traefik, vault (2-line diff) - discovery was
  already reachable-complete for them.
* **Tool evolution (class e):** today's binary differs from old-binary-live for crossplane, external-secrets, ingress-nginx,
  postgresql, kube-prometheus-stack (491 diff lines, 74 unverified elements, `crds-chart` elements added), cilium
  (chart `strategy: template` → `lookup appVersion`), vault. Not attributable to the network.
* `kube-prometheus-stack` old-binary control did not run (not built; low priority).
* G3 headline numbers (wrong-proposal rate, share found automatically) are computed against final definitions and are not
  affected by reachability; only per-element *status* labels change.

## 6. Drift (G4)

`ri drift` vs baseline checks, vs live checks, and `-baseline -`: **0 drift, 0 unverifiable, 0 notes, 0 checked** for 26/28
(no release newer than the baseline cutoff exists: baselines were saved at upstream's tip the same day); elasticsearch and
opensearch (no baseline in definition) validated the newest 3 releases (9.5.2-9.5.4, 3.7.0-3.9.0) with 0 events. Consequence of
replacing baselines by the live versions: **none for drift** (cutoffs unchanged, argo-cd cutoff 3.5.3). Drift is simply not
exercised by same-day data; a meaningful test needs an older baseline. (OUTCOMES G4's "validated live" stale-definition
test is unaffected.)

## 7. Research claims re-probed (129 probes, `claims.tsv`; HTTP status only unless noted)

Previously "blocked/unreachable/UNVERIFIED", **now reachable (200)** (class a): api.github.com releases + security-advisories
for all 10 probed repos (advisory counts: argo-cd 52, istio 14, falco 8, redis 47, external-secrets 7, crossplane 5,
cert-manager 3; ingress-nginx/postgres/vault 0), github.com HTML (releases, advisories), quay.io (cert-manager controller +
startupapicheck manifests v1.18.0, argocd), charts.jetstack.io, argoproj.github.io, charts.external-secrets.io,
external-secrets.github.io/.io, charts/releases/cli/docs.crossplane.io + S3 buckets, registry.k8s.io (307→200), k8s.gcr.io,
kubernetes.github.io, kubernetes.io (blog + CVE feed), api.osv.dev, vuln.go.dev, blob.istio.io (Helm index), gcr.io/istio-release,
registry.istio.io (token flow → 200), www.postgresql.org (security, versioning, release docs), git.postgresql.org, apt/yum.postgresql.org,
github archive/codeload tarballs (postgres), ftp.postgresql.org (206 range), helm.releases.hashicorp.com (366 chart versions),
vaultproject.io, discuss.hashicorp.com, checkpoint-api, endoflife.date, registry.terraform.io, go.googlesource.com,
download.falco.org, artifacthub.io.

**Still unreachable / denied (class b):** `storage.googleapis.com/golang` 403 (dl.google.com 206 works);
`docs.falco.org` DNS failure (000); `git.redis.io` 000; `dl.min.io/server/minio/release` 410 Gone; docker.io and quay.io
`minio/minio` 401 even with a valid anonymous token (denied); `xpkg.crossplane.io` / `xpkg.upbound.io` / `ghcr.io/hashicorp/vault`
/ `ghcr.io/fluxcd/flux-manifests` 401 without any auth realm (so "403 egress" is now "401 auth required": reachable, not anonymous);
`docker.io/library/traefik:v3.5.0` 401 with a valid token (inconclusive: may not exist as an official-image tag);
`storage.googleapis.com/cert-manager-release` 403 (private staging bucket, as documented).
Probes that returned 404 on *my guessed path* (`blob.istio.io/.../releases/1.31.1/`, artifacthub istiod package,
`developer.hashicorp.com/vault/docs/release-notes`, `crossplane.github.io/`) are unresolved, not evidence of absence.
Content-level confirmations (`raw/content-probes.txt`): cert-manager v1.18.0 GitHub release has exactly
`cert-manager.crds.yaml` and `cert-manager.yaml` (research claim at cert-manager.md:184 **confirmed**); no `.sha256`
asset (404, confirmed); postgres has **0** GitHub releases (confirmed); istio 1.31.1 release exists, `draft=false
prerelease=false`, 34 assets (research said Releases UNVERIFIED and that bots create drafts/prereleases: humans publish, so
the flag is consistent but the claim "not a reliable discriminator" remains plausible); gcr.io/istio-release/pilot has 1.30.5 but
only 1.31.0-alpha tags, no 1.31.0/1.31.1 (**confirmed** migration claim); crossplane v2.4.2 GitHub release body is the
hand-written "Highlights" shape (claim previously UNVERIFIED, now confirmed); docs.min.io community page answers 200 with an HTML
shell (soft-404 claim not re-tested for body text).

## 8. Conclusions that no longer hold

* `docs/FINDINGS.md:26-28` - cert-manager "Unverifiable here: Helm chart, startupapicheck image (quay.io / charts.jetstack.io
  blocked)": now 17 validated / 0 unverifiable; all 6 images and the chart pass directly.
* `docs/FINDINGS.md:52-55` ("Blocked hosts (api.github.com, quay.io, charts.jetstack.io, argoproj.github.io) are reported as
  unavailable") and the README sample output (`✗ github-releases ... HTTP 403`, `controller-image unavailable quay.io`): true of the
  sandbox run only.
* `docs/FINDINGS.md:58-61` and `products/argo-cd.yaml:211-214` - "v3.4.0 GitHub release has no assets": there is no release (D2).
* `docs/FINDINGS.md:99` - "ghcr blobs and blob.istio.io were unreachable": both answer now.
* `docs/ONBOARDING.md:18,53,345` and the per-product "Unreachable sources" column / `unverifiable` counts: reflect the sandbox;
  vault's `locator:helm-git` justification ("helm.releases.hashicorp.com is blocked") no longer applies (index fetchable, 366 versions).
* `docs/phase2/PLAN.md:61-67` "anonymous Docker Hub stays rate-limited or 401": still 401 for minio; otherwise untested.
* Holds: G9 eval numbers (identical live/offline/stored), the Falco `9.0.0.tgz` re-publish (digest mismatch reproduced:
  downloaded `0224559b...` vs index `093cb7f5...`), MinIO anonymous-pull denial and `dl.min.io` 410, representability/zero-Go-code claims
  (stats unchanged; only check-derived columns moved: validated subjects, unverifiable counts).

## 9. Recommendations

1. **Replace** these baselines with the live ones: cert-manager, crossplane, external-secrets, ingress-nginx, postgresql,
   redis, terraform-provider-aws, traefik, vault (pure sandbox artifacts, no definition change), and istio, strimzi,
   kube-prometheus-stack (also current-definition-accurate). Keep falco, minio unchanged (same outcome). **Hold** karpenter
   (re-run sequentially once 429 handling exists) and argo-cd (needs D2 decision). Drift consequence: none (cutoff releases equal).
   `ri stats`/ONBOARDING.md would then show 0 unverifiable for the replaced products (see `stats.md`).
2. **Definitions:** ingress-nginx chroot exception for 1.10.0 (D1); argo-cd exception wording + known-release policy (D2);
   revisit vault `helm-git` → add the now-reachable `helm-repo` as primary (still keep git fallback); consider
   crossplane/external-secrets canonical chart channels (reachable now) as primary.
3. **Pipeline:** `throttled` state + Retry-After + per-host concurrency (D3); baseline `definitionDigest` check (D4);
   probe-time distinction 401-auth vs 403-egress vs 000-DNS in `unavailable` details.
4. **Findings/docs corrections:** section 8 list. Add a note to FLEET.md about the gate baseline (D5).
5. **Open (not done):** kube-prometheus-stack old-binary discovery control; drift against an artificially older baseline;
   body-level (not status-level) verification of every research claim.

## 10. Reproduce
`go build -o bin/ri ./cmd/ri`; per-product fresh state; `ri -state <tmp> -refresh check -versions <baseline releases> -o json <id>`;
`python3 docs/rerun/compare.py`, `python3 docs/rerun/report_tables.py`, `docs/rerun/probe-claims.sh`. Raw stderr/exit codes in `docs/rerun/raw/`.
