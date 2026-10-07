# Discovery report: postgres

- Repository: `github.com/postgres/postgres` at `master` (commit `6f3bdadaadc4…`)
- Generated: 2026-10-02T12:37:37Z
- LLM: not used (deterministic resolver only)
- Tags: 693 tags: component tag scheme ^REL_?(?P<major>\d+)(?:_(?P<minor>\d+))?_(?P<patch>\d+)$ matching 554/693 tags, 554 stable, 139 junk; latest stable REL_18_6; lineage minor
- Strict tag pattern: `^REL_?(?P<major>\d+)(?:_(?P<minor>\d+))?_(?P<patch>\d+)$` (junk sample: PG95-1_01, PG95-1_08, PG95-1_09, REL7_1_BETA, REL7_1_BETA2, REL7_1_BETA3)
- Scanned `github.com/postgres/postgres@master` (source profile): 7699 files listed, 521 read
- Validation: done against REL_16_0, REL_16_15, REL_17_0, REL_17_11, REL_18_0, REL_18_6

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 1 | git-tags github.com/postgres/postgres |
| release-notes | not-found | 0 |  |
| changelog | not-found | 0 |  |
| helm-charts | not-found | 0 |  |
| registries | not-found | 0 |  |
| images | candidates-only | 3 |  |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 1 | github-advisories postgres/postgres |
| version-relations | not-found | 0 |  |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| source:advisories | discovered | security.github-hosted-default | https://github.com/postgres/postgres/blob/6f3bdadaadc4692050ca20dff53d3890088b9272/.github/SECURITY.md#L1 |
| source:tags | discovered | versions.git-tags | https://github.com/postgres/postgres/releases/tag/REL_18_6#refs/tags/REL_18_6 |
| versioning | inferred | tags.scheme | https://github.com/postgres/postgres/releases/tag/REL_18_6#refs/tags/REL_18_6 |

Statuses: 2 discovered, 1 inferred.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: postgresql
name: postgres
homepage: https://github.com/postgres/postgres
versioning:
  scheme: semver
  tagPattern: ^REL_?(?P<major>\d+)(?:_(?P<minor>\d+))?_(?P<patch>\d+)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/postgres/postgres
      tagPattern: ^REL_?(?P<major>\d+)(?:_(?P<minor>\d+))?_(?P<patch>\d+)$
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: postgres/postgres
    notes: 'Security policy: .github/SECURITY.md → https://www.postgresql.org/support/security/>'
artifacts: []
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - REL_16_0
    - REL_16_15
    - REL_17_0
    - REL_17_11
    - REL_18_0
    - REL_18_6
  notes: Proposed by automated discovery; relationship checks run against REL_16_0, REL_16_15, REL_17_0, REL_17_11, REL_18_0, REL_18_6.
```

## Open questions for the reviewer

- Component tag scheme inferred from the tag shapes: confirm the digit roles (a two-number tag such as REL_18_6 is read as 18.0.6).

## Validation matrix

| Element | Verdict | REL_16_0 | REL_16_15 | REL_17_0 | REL_17_11 | REL_18_0 | REL_18_6 |
|---|---|---|---|---|---|---|---|

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | no dot-separated semver family exists; component tag scheme ^REL_?(?P<major>\d+)(?:_(?P<minor>\d+))?_(?P<patch>\d+)$ mined from the tag list and tested against it (554/693 tags match, e.g. REL_18_6 = 18.0.6) |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (693 tags: component tag scheme ^REL_?(?P<major>\d+)(?:_(?P<minor>\d+))?_(?P<patch>\d+)$ matching 554/693 tags, 554 stable, 139 junk; latest stable REL_18_6; lineage… |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |

<details><summary>3 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| image `ghcr.io/anarazel/pg-vm-images/main` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/anarazel/pg-vm-images/main/linux_debian_trixie_ci` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/anarazel/pg-vm-images/main/linux_debian_trixie_ci_docs` | image.third-party | Image is not named after the product (dependency or tooling image). |

</details>

## Candidates


### image (3)

- `ghcr.io/anarazel/pg-vm-images/main` (medium; workflow.image-ref) — [postgres/postgres:.github/workflows/pg-ci.yml L125](https://github.com/postgres/postgres/blob/6f3bdadaadc4692050ca20dff53d3890088b9272/.github/workflows/pg-ci.yml#L125): `CONTAINER_REPO: ghcr.io/anarazel/pg-vm-images/main`
- `ghcr.io/anarazel/pg-vm-images/main/linux_debian_trixie_ci` (medium; workflow.image-ref) — [postgres/postgres:.github/workflows/pg-ci.yml L207](https://github.com/postgres/postgres/blob/6f3bdadaadc4692050ca20dff53d3890088b9272/.github/workflows/pg-ci.yml#L207): `container_linux_ci: ${{ env.CONTAINER_REPO }}/${{ env.CONTAINER_LINUX_CI }}`
- `ghcr.io/anarazel/pg-vm-images/main/linux_debian_trixie_ci_docs` (medium; workflow.image-ref) — [postgres/postgres:.github/workflows/pg-ci.yml L208](https://github.com/postgres/postgres/blob/6f3bdadaadc4692050ca20dff53d3890088b9272/.github/workflows/pg-ci.yml#L208): `container_linux_ci_docs: ${{ env.CONTAINER_REPO }}/${{ env.CONTAINER_LINUX_CI_DOCS }}`

### security-policy (1)

- `.github/SECURITY.md` (high; docs.security-policy) — [postgres/postgres:.github/SECURITY.md L1](https://github.com/postgres/postgres/blob/6f3bdadaadc4692050ca20dff53d3890088b9272/.github/SECURITY.md#L1): `For information about reporting security issues, see`

### tag-scheme (1)

- `^REL_?(?P<major>\d+)(?:_(?P<minor>\d+))?_(?P<patch>\d+)$` (medium; tags.ls-remote) — [postgres/postgres/releases/tag/REL_18_6 refs/tags/REL_18_6](https://github.com/postgres/postgres/releases/tag/REL_18_6#refs/tags/REL_18_6): `724edf9bde9d356724ad384a2e196edc3c9f80f7 refs/tags/REL_18_6`
