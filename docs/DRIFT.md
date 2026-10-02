# Source drift

A product definition is a snapshot of assumptions: where releases are
announced, which artifacts each release ships, how artifact versions relate to
release versions. Upstream keeps moving. `ri drift` re-validates those
assumptions against the **newest** releases — the ones published after the
definition was validated — and reports structured differences. It never
mutates `products/*.yaml`: it proposes, a human (or a reviewing agent) edits.

```
ProductDefinition → new upstream release → validate expected channels/artifacts/relationships
  → valid     → continue ingestion
  → invalid   → drift event → investigation → human edits the definition
  → unreachable → unverifiable (NOT drift)
```

## Model

`ri drift <product>` (package `internal/drift`):

1. Lists the canonical versions (same machinery as every command).
2. Determines the **baseline**: the newest release the definition's
   relationships were validated against — from the saved
   `ri check -o json` report in `docs/onboarding/checks/<id>.json` when
   present, else from `validatedAgainst` / `provenance.validatedReleases`.
3. Re-runs the exhaustive relationship validation (`ri check` semantics) on
   the newest `-n` releases (default 3) that are **newer than the baseline
   cutoff**; `-versions a,b,c` checks explicit releases instead.
4. Compares fresh outcomes with baseline outcomes and with the definition,
   and emits events.

### Event vocabulary

| kind | meaning |
|---|---|
| `relationship-broken` | a declared relationship fails on reachable channels and no more specific kind applies (tag convention change, renamed asset, dropped content) |
| `artifact-missing` | a declared artifact is absent from every reachable channel (not `optional`, not covered by `exceptions`) |
| `artifact-appeared` | an artifact deterministically discoverable through already-declared channels that the definition does not declare — today: an image repository carrying the release tag, referenced by a declared `image-refs` content snapshot, that no declared `oci` channel covers |
| `source-moved` | something vanished from its declared location: a source document that is reachable but empty, or an artifact that left its leading declared channel but is present at a **declared** fallback channel (Helm index → OCI). No new host is ever invented. |
| `availability-violated` | an artifact exists for a release outside its declared availability window (probed directly; only positive observations count) |
| `unverifiable` | could not be checked — a declared channel was unreachable. Explicitly not drift. |

Every event also carries a `status` (`drift` / `unverifiable` / `note`), a
`severity` (`high` a validated relationship broke, `medium` weaker history,
`low` informational), the affected `releases`, the `baseline` text (what held
before and where that is recorded) and `evidence` IDs that resolve inside the
report. A subject that already failed at the baseline is a `note`
("not a regression"), not drift.

### Evidence

Events cite `domain.Evidence` records from the same exhaustive run: the
registry answers that said "absent" (the oci/helm adapters attach evidence to
404s), the index entries that matched (or the only partially-matching index),
the snapshot that references an undeclared image. The one exception: when no
versions source answers at all, the event (`VersionListFailed`) describes the
failure with the source status but has nothing retrieved to cite — an
unreachable channel has no evidence by definition.

## Unverifiable vs drift

This is the honesty rule of the whole pipeline, applied with force:

- **unverifiable** = a declared channel could not be reached (blocked host,
  auth, rate limit) — and for artifacts, absence needs *every* channel to
  answer. The report says "cannot determine" and counts nothing as drift.
- **drift** = a channel *answered* and disagreed with the definition
  (reachable and absent, or reachable and present where not declared).

Consequence worth knowing: on Docker Hub a **nonexistent** repository often
answers `401 authentication required`, which is `unavailable`, not `not
found`. A renamed `docker.io` image can therefore surface as *unverifiable*
rather than `artifact-missing` unless a second declared channel (a mirror,
`ghcr`, `gcr`) answers with a clean 404. That is the honest reading of what
upstream said.

### Why it was unreachable (fetch states)

`internal/fetch` distinguishes the reasons a resource is unavailable. All of them
are still `unavailable` to callers (`errors.Is(err, fetch.ErrUnavailable)`), but
the detail, and for rate limits the source state, say why:

| Reason | Trigger | Source state | Notes |
|---|---|---|---|
| throttled | HTTP 429, 503 + `Retry-After`, GitHub quota (403 + `X-RateLimit-Remaining: 0`) | `throttled` | retried with `Retry-After` (capped at 30s, else exponential 1s/2s/4s, 3 retries), later requests to the host wait; reachable, retry later |
| authentication required | HTTP 401 | `unavailable` | detail carries the `WWW-Authenticate` challenge |
| forbidden | HTTP 403 (not a rate limit) | `unavailable` | the host answered and refused (egress policy, denied anonymous pull) |
| DNS resolution failed | name does not resolve | `unavailable` | |
| plain unreachable | connection refused, timeout, TLS | `unavailable` | |

`throttled` is treated like `unavailable` everywhere a verdict is made: never
drift, never "missing". Per-host concurrency is bounded (6 by default, 2 for
`public.ecr.aws`, which answered 429 to bursts while sequential reads were
fine) so a run does not provoke the throttling it then has to report.

## Stale baselines

`ri check -o json` records the `definitionDigest` of the definition it ran
against. `ri drift` compares it with the current definition and prints
`baseline.digestState` in the report: `current`, `stale` (the definition was
edited after the baseline was saved, so the baseline describes an older
definition: re-run `ri check` and replace it) or `unrecorded` (saved before
digests existed). It is a property of the baseline, never an event: it does not
change the verdict or the exit code.

## Proposals

Each drift event may carry a `proposal`: an annotated YAML fragment in
definition syntax (comment explains the event; the changed keys follow). The
whole report renders as one multi-document YAML stream — printed after the
text report, or written to `-out FILE`. Deterministic proposals only:

- artifact gone from every reachable channel → end `availability` at the last
  baseline observation, or (when only some releases miss) add `exceptions`
  with a TODO reason;
- moved between declared channels → reorder `channels` so the answering
  channel leads;
- source document vanished → `exceptions` as an interim; the new location is
  research, drift never guesses hosts;
- availability violated → drop or correct the window;
- image appeared → a complete new artifact entry (id/type/name/version
  template/oci channel).

`-out` refuses to write inside the products directory, and nothing in the
tool ever edits `products/*.yaml`.

## Workflow

```
ri drift istio                     # newest 3 releases newer than the baseline
ri drift istio -n 5                # more releases
ri drift istio -versions 1.32.0    # explicit releases (e.g. after a heads-up)
ri drift istio -baseline -         # ignore the saved check report; validatedAgainst is the baseline
ri drift istio -o json             # the report as JSON
ri drift istio -out proposed.yaml  # proposal to a file instead of stdout
```

Exit code: 0 when there is no drift (including "only unverifiable" and "no
releases newer than the baseline"), non-zero when `summary.drift > 0` — so a
cron job can page on the right thing.

When a drift event fires: investigate the cited evidence (URIs open in a
browser), decide whether upstream changed (edit the definition, then re-run
`ri check` and update `docs/onboarding/checks/<id>.json`) or the release is an
anomaly (add `exceptions` with a reason). Drift events are the input;
curation stays human.

## Example

Real upstream, synthetically stale definition (istio with `pilot` pointed at
a repository that does not exist and `agentgateway-image` with a window that
ends one minor too early — live run, 2026-10-01):

```
$ ri drift istio -versions 1.31.1
istio: drift check — newest releases vs the baseline
Baseline: saved check report docs/onboarding/checks/istio.json, cutoff 1.31.1
Checked: 1.31.1

Version sources:
  ✓ git-tags                 git-tags       312 releases from 465 tags (…)
  · github-releases          github-releases not consulted: git-tags answered

Drift — reachable channels disagree with the definition: (1)
  ✗ [low] availability-violated agentgateway-image
      artifact agentgateway-image is declared with availability ">= 1.29.0, < 1.31.0" (not applicable to 1.31.1) but exists there: docker.io/istio/agentgateway:1.31.1
      releases: 1.31.1
      baseline: availability ">= 1.29.0, < 1.31.0" curated in the definition
      evidence: ev-b20e542e9125
      proposal: remove or correct the availability of agentgateway-image: …

Unverifiable — could NOT be checked (unreachable); not drift: (1)
  ? [low] unverifiable pilot-image
      cannot determine whether pilot-image still holds for 1.31.1: a declared channel could not be reached, and absence needs every channel to answer
      detail: 1.31.1: absent from gcr.io/istio-release/pilot:1.31.1; not verifiable: docker.io/istio/pilot-legacy:1.31.1 (oci: unavailable)
      baseline: passed on 1.29.0, 1.29.8, 1.30.0, 1.30.5, 1.31.0, 1.31.1 (saved check report)
      evidence: ev-d3c9c2776dd3

Summary: 1 drift · 1 unverifiable · 0 notes (1 releases checked, 47 evidence records)
error: 1 drift event(s) for istio (unverifiable events are not drift)
```

Note the two stale assumptions produce *different* statuses: the
`agentgateway` window is contradicted by a positive observation (drift), while
the renamed `pilot` repository cannot be proven absent — gcr.io says absent
but docker.io answers `unavailable` for the unknown repository (the Docker Hub
behaviour described above), so it stays unverifiable.

The proposal document (stdout after the report, or `-out`):

```yaml
# Proposal for products/istio.yaml — generated by `ri drift istio` at …
# NOT applied automatically. Review each fragment against its drift event,
# then edit the product definition by hand. Drift never guesses hosts: every
# location mentioned here is already declared in the definition.

---

# availability-violated agentgateway-image: remove or correct the availability …
artifacts:
  - id: agentgateway-image
    # availability-violated: declared ">= 1.29.0, < 1.31.0" but the artifact exists for 1.31.1
    availability: ""   # was: ">= 1.29.0, < 1.31.0"
    # or tighten the window to the real bound once known
```

Against the definitions as checked in (all baselines current as of
2026-10-01) the same command reports:

```
Baseline: saved check report docs/onboarding/checks/cert-manager.json, cutoff 1.21.2
Checked: no releases newer than the baseline; nothing to re-validate.
Summary: 0 drift · 0 unverifiable · 0 notes
```

## Scope and limits

- Deterministic only; no LLM anywhere in the path.
- Channel re-probes (moves, availability) go only through locators already
  declared in the definition.
- `artifact-appeared` detects release-tagged image references in `image-refs`
  snapshots. Other "new artifact" shapes (a new chart name in a declared
  index, a new release-asset pattern) are not detected yet; they need a
  generic "list what the channel has" comparison, which the `sources` ports
  do not expose for all kinds.
- Availability checks probe **artifacts** only; a source declared with an
  availability window that has become stale is not detected.
